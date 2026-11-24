package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// GitLab release support for adopters (#201). Inspection uses the GET-only
// observation client and grants nothing. Tag and release mutations are
// explicit forge operations that run only on a checked Workflow Owner
// delegation note (#201 decision 3). Garage cannot tell a human invoker from an
// agent, so every Garage tag/release mutation takes the delegated path; a human
// acting directly uses glab or the native interface instead.

const (
	releaseOpTag     = "tag"
	releaseOpCreate  = "release-create"
	releaseOpUpdate  = "release-update"
	releaseOpLink    = "link"
	markerDelegation = "[WORKFLOW OWNER — RELEASE DELEGATION]"
)

var releaseOperations = []string{releaseOpTag, releaseOpCreate, releaseOpUpdate, releaseOpLink}
var releaseLinkTypes = []string{"other", "package", "image", "runbook"}

// Asset names that carry signing or provenance evidence for a subject asset.
// Presence and durable storage are checked; content is never verified.
var signatureSuffixes = []string{".sig", ".asc", ".sigstore", ".sigstore.json", ".cosign.bundle"}
var provenanceSuffixes = []string{".intoto.jsonl", ".provenance.json", ".slsa.json"}

type glTag struct {
	Name      string `json:"name"`
	Message   string `json:"message"`
	Protected *bool  `json:"protected"`
	Commit    struct {
		ID string `json:"id"`
	} `json:"commit"`
}
type glRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Commit      struct {
		ID string `json:"id"`
	} `json:"commit"`
	Links struct {
		Self string `json:"self"`
	} `json:"_links"`
}
type glReleaseLink struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	LinkType string `json:"link_type"`
}
type glPackage struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	PackageType string `json:"package_type"`
}
type glPackageFile struct {
	ID         int    `json:"id"`
	PackageID  int    `json:"package_id"`
	FileName   string `json:"file_name"`
	FileSHA256 string `json:"file_sha256"`
}
type glTagPipeline struct {
	glPipeline
	Tag *bool `json:"tag"`
}

// releaseAsset is one release link with its checked durability.
type releaseAsset struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	LinkType string `json:"link_type"`
	Kind     string `json:"kind"`    // subject | signature | provenance
	Durable  string `json:"durable"` // passed | failed | unavailable
	SHA256   string `json:"sha256,omitempty"`
	Detail   string `json:"detail"`
}

type releaseInspection struct {
	URL    string               `json:"url"`
	Tag    string               `json:"tag"`
	Commit string               `json:"commit"`
	Reads  int                  `json:"reads"`
	Gates  []workflowPolicyGate `json:"gates"`
	Assets []releaseAsset       `json:"assets"`
	Note   string               `json:"note"`
}

const releaseInspectionNote = "Native release evidence only. It never replaces Garage release or promotion records, grants no release authority, and does not evaluate E6 conditions. Signing and provenance outcomes check asset presence and durable storage only; Garage performs no cryptographic verification."

// inspectGitlabRelease reads checked facts about one tag and its release.
// Read failures become unavailable outcomes; only invalid input is an error.
func inspectGitlabRelease(ctx context.Context, f forgeConfig, tag, expectedRef string, run glabRunner) (releaseInspection, error) {
	f = effectiveForge(f)
	if !validReleaseTag(tag) {
		return releaseInspection{}, errors.New("release inspection requires a valid tag name")
	}
	if expectedRef != "" && !objectNamePattern.MatchString(expectedRef) {
		return releaseInspection{}, errors.New("--ref must be a full commit object name")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	c := newGitlabClient(run, f)
	r := releaseReader{c: c, f: f, tag: tag, packages: map[string]packageFiles{}}
	out := releaseInspection{Tag: tag, URL: f.Endpoint + "/" + f.Project + "/-/tags/" + url.PathEscape(tag), Note: releaseInspectionNote}

	var project glProject
	if _, err := c.get(ctx, "GET", "projects/"+url.PathEscape(f.Project), &project); err == nil && project.Path == f.Project && project.ID > 0 {
		r.projectID = project.ID
	}

	tagPath := c.resource(f.Project, "repository", "tags", url.PathEscape(tag))
	var t glTag
	_, err := c.get(ctx, "GET", tagPath, &t)
	switch {
	case isGitlabStatus(err, 404):
		r.gate(&out, "tag", policyFailed, true, "the tag does not exist", tagPath)
	case err != nil:
		r.gate(&out, "tag", policyUnavailable, true, "tag: "+err.Error(), tagPath)
	case t.Name != tag || !objectNamePattern.MatchString(t.Commit.ID):
		r.gate(&out, "tag", policyUnavailable, true, "the tag response does not identify this tag and a full commit", tagPath)
	case expectedRef != "" && t.Commit.ID != expectedRef:
		out.Commit = t.Commit.ID
		r.gate(&out, "tag", policyFailed, true, "the tag points to "+t.Commit.ID+", not the expected "+expectedRef, tagPath)
	default:
		out.Commit = t.Commit.ID
		r.gate(&out, "tag", policyPassed, true, "the tag points to "+t.Commit.ID, tagPath)
	}

	switch {
	case out.Commit == "":
		r.gate(&out, "protected tag", policyUnavailable, true, "tag facts unavailable", tagPath)
	case t.Protected == nil:
		r.gate(&out, "protected tag", policyUnavailable, true, "this deployment does not report tag protection", tagPath)
	case *t.Protected:
		r.gate(&out, "protected tag", policyPassed, true, "a protected-tag rule covers the tag", tagPath)
	default:
		r.gate(&out, "protected tag", policyFailed, true, "no protected-tag rule covers the tag", tagPath)
	}

	releasePath := c.resource(f.Project, "releases", url.PathEscape(tag))
	var rel glRelease
	releaseOK := false
	_, err = c.get(ctx, "GET", releasePath, &rel)
	switch {
	case isGitlabStatus(err, 404):
		r.gate(&out, "release", policyFailed, true, "no release exists for the tag", releasePath)
	case err != nil:
		r.gate(&out, "release", policyUnavailable, true, "release: "+err.Error(), releasePath)
	case rel.TagName != tag:
		r.gate(&out, "release", policyUnavailable, true, "the release response does not identify this tag", releasePath)
	case out.Commit == "":
		r.gate(&out, "release", policyUnavailable, true, "the release exists, but its tag commit is unavailable", releasePath)
	case rel.Commit.ID != out.Commit:
		r.gate(&out, "release", policyFailed, true, "the release commit "+rel.Commit.ID+" differs from the tag commit", releasePath)
	default:
		releaseOK = true
		if rel.Links.Self != "" {
			out.URL = rel.Links.Self
		}
		r.gate(&out, "release", policyPassed, true, "the release names the tag and its commit", releasePath)
	}

	r.pipeline(ctx, &out)
	r.assets(ctx, &out, releaseOK, releasePath)
	out.Reads = c.reads
	return out, nil
}

type packageFiles struct {
	files []glPackageFile
	err   error
}

type releaseReader struct {
	c         *gitlabClient
	f         forgeConfig
	tag       string
	projectID int
	packages  map[string]packageFiles
}

func (r *releaseReader) gate(out *releaseInspection, name, state string, required bool, detail, path string) {
	source := r.f.APIBaseURL + "/" + path
	out.Gates = append(out.Gates, workflowPolicyGate{Name: name, State: state, Required: required, Detail: detail, Source: source, Sources: []string{source}})
}

// pipeline reports the latest tag pipeline for the tag commit. It is reported,
// not required: whether a release needs one is the adopter's E6 condition.
func (r *releaseReader) pipeline(ctx context.Context, out *releaseInspection) {
	listPath := r.c.resource(r.f.Project, "pipelines")
	const name = "tag pipeline"
	const scope = "; required only when an adopter release condition requires it"
	if out.Commit == "" {
		r.gate(out, name, policyUnavailable, false, "tag commit unavailable"+scope, listPath)
		return
	}
	pipelines, truncated, err := gitlabListFiltered[glPipeline](ctx, r.c, listPath, url.Values{"ref": {r.tag}, "sha": {out.Commit}})
	if err != nil || truncated {
		r.gate(out, name, policyUnavailable, false, "tag pipelines: "+errorText(err, "incomplete collection")+scope, listPath)
		return
	}
	latest := 0
	for _, p := range pipelines {
		if p.ID > latest {
			latest = p.ID
		}
	}
	if latest == 0 {
		r.gate(out, name, policyFailed, false, "no pipeline ran for the tag commit"+scope, listPath)
		return
	}
	detailPath := r.c.resource(r.f.Project, "pipelines", strconv.Itoa(latest))
	var p glTagPipeline
	if _, err := r.c.get(ctx, "GET", detailPath, &p); err != nil {
		r.gate(out, name, policyUnavailable, false, "tag pipeline: "+err.Error()+scope, detailPath)
		return
	}
	if p.ID != latest || p.Ref != r.tag || p.SHA != out.Commit || p.Tag == nil || !*p.Tag || r.projectID > 0 && p.ProjectID != 0 && p.ProjectID != r.projectID {
		r.gate(out, name, policyUnavailable, false, "the latest pipeline is not bound to this tag and commit"+scope, detailPath)
		return
	}
	switch p.Status {
	case "success":
		r.gate(out, name, policyPassed, false, "pipeline "+strconv.Itoa(p.ID)+" succeeded"+scope, detailPath)
	case "failed", "canceled", "skipped":
		r.gate(out, name, policyFailed, false, "pipeline "+strconv.Itoa(p.ID)+" "+p.Status+scope, detailPath)
	default:
		r.gate(out, name, policyUnavailable, false, "pipeline "+strconv.Itoa(p.ID)+" is "+p.Status+", not complete"+scope, detailPath)
	}
}

func (r *releaseReader) assets(ctx context.Context, out *releaseInspection, releaseOK bool, releasePath string) {
	linksPath := releasePath + "/assets/links"
	const signing = "; presence and durable storage only, no cryptographic verification; required only when an adopter release condition requires it"
	unavailable := func(detail string) {
		r.gate(out, "durable assets", policyUnavailable, true, detail, linksPath)
		r.gate(out, "signature evidence", policyUnavailable, false, detail+signing, linksPath)
		r.gate(out, "provenance evidence", policyUnavailable, false, detail+signing, linksPath)
	}
	if !releaseOK {
		unavailable("release facts unavailable or failed")
		return
	}
	links, truncated, err := gitlabList[glReleaseLink](ctx, r.c, linksPath)
	if err != nil || truncated {
		unavailable("release links: " + errorText(err, "incomplete collection"))
		return
	}
	out.Assets = []releaseAsset{}
	for _, link := range links {
		a := releaseAsset{Name: link.Name, URL: link.URL, LinkType: link.LinkType, Kind: assetKind(link.Name)}
		a.Durable, a.SHA256, a.Detail = r.durability(ctx, link.URL)
		out.Assets = append(out.Assets, a)
	}
	if len(out.Assets) == 0 {
		r.gate(out, "durable assets", policyNotRequired, false, "the release has no asset links", linksPath)
	} else {
		r.gate(out, "durable assets", worstState(out.Assets), true, "every asset link must resolve to a file in this project's package registry", linksPath)
	}

	var subjects, provenance []releaseAsset
	for _, a := range out.Assets {
		switch a.Kind {
		case "subject":
			subjects = append(subjects, a)
		case "provenance":
			provenance = append(provenance, a)
		}
	}
	if len(subjects) == 0 {
		r.gate(out, "signature evidence", policyUnavailable, false, "no signable asset"+signing, linksPath)
	} else {
		var signatures []releaseAsset
		missing := []string{}
		for _, s := range subjects {
			found := false
			for _, a := range out.Assets {
				if a.Kind == "signature" && slices.Contains(signatureSuffixes, strings.TrimPrefix(a.Name, s.Name)) && strings.HasPrefix(a.Name, s.Name) {
					signatures = append(signatures, a)
					found = true
				}
			}
			if !found {
				missing = append(missing, s.Name)
			}
		}
		if len(missing) > 0 {
			r.gate(out, "signature evidence", policyFailed, false, "no signature asset for "+strings.Join(missing, ", ")+signing, linksPath)
		} else {
			r.gate(out, "signature evidence", worstState(signatures), false, "every subject asset has a durably stored signature asset"+signing, linksPath)
		}
	}
	if len(provenance) == 0 {
		r.gate(out, "provenance evidence", policyFailed, false, "no provenance asset"+signing, linksPath)
	} else {
		r.gate(out, "provenance evidence", worstState(provenance), false, "provenance assets are durably stored"+signing, linksPath)
	}
}

// worstState combines asset durability: any failure fails, then any
// unavailable fact is unavailable; only all-passed passes.
func worstState(assets []releaseAsset) string {
	state := policyPassed
	for _, a := range assets {
		switch a.Durable {
		case policyFailed:
			return policyFailed
		case policyUnavailable:
			state = policyUnavailable
		}
	}
	return state
}

func assetKind(name string) string {
	for _, s := range signatureSuffixes {
		if strings.HasSuffix(name, s) && len(name) > len(s) {
			return "signature"
		}
	}
	for _, s := range provenanceSuffixes {
		if strings.HasSuffix(name, s) && len(name) > len(s) {
			return "provenance"
		}
	}
	return "subject"
}

// durability checks that a link resolves to one file in this project's
// generic package registry. Job artifacts are ephemeral and fail.
func (r *releaseReader) durability(ctx context.Context, raw string) (string, string, string) {
	if problem := releaseLinkURLProblem(raw); problem != "" {
		return policyFailed, "", problem
	}
	name, version, file, ok := r.genericPackageFile(raw)
	if !ok {
		return policyUnavailable, "", "outside this project's generic package registry; durability cannot be checked"
	}
	key := name + "\x00" + version
	cached, seen := r.packages[key]
	if !seen {
		cached = r.packageFiles(ctx, name, version)
		r.packages[key] = cached
	}
	if cached.err != nil {
		return policyUnavailable, "", "package files: " + cached.err.Error()
	}
	digests := map[string]bool{}
	for _, pf := range cached.files {
		if pf.FileName == file {
			digests[pf.FileSHA256] = true
		}
	}
	switch {
	case len(digests) == 0:
		return policyFailed, "", "the package file does not exist"
	case digests[""]:
		return policyUnavailable, "", "the package file has no reported SHA-256 digest"
	case len(digests) > 1:
		return policyFailed, "", "the package holds different files with this name; the downloaded content is ambiguous"
	}
	digest := ""
	for d := range digests {
		digest = d
	}
	return policyPassed, digest, "package file " + name + "/" + version + "/" + file
}

func (r *releaseReader) packageFiles(ctx context.Context, name, version string) packageFiles {
	listPath := r.c.resource(r.f.Project, "packages")
	packages, truncated, err := gitlabListFiltered[glPackage](ctx, r.c, listPath, url.Values{"package_type": {"generic"}, "package_name": {name}, "package_version": {version}})
	if err != nil || truncated {
		return packageFiles{err: errors.New(errorText(err, "incomplete package collection"))}
	}
	var result packageFiles
	for _, p := range packages {
		if p.Name != name || p.Version != version || p.PackageType != "generic" || p.ID < 1 {
			continue
		}
		files, truncated, err := gitlabList[glPackageFile](ctx, r.c, r.c.resource(r.f.Project, "packages", strconv.Itoa(p.ID), "package_files"))
		if err != nil || truncated {
			return packageFiles{err: errors.New(errorText(err, "incomplete package file collection"))}
		}
		for _, pf := range files {
			if pf.PackageID != 0 && pf.PackageID != p.ID {
				return packageFiles{err: errors.New("package file identity mismatch")}
			}
		}
		result.files = append(result.files, files...)
	}
	return result
}

// genericPackageFile parses this project's generic package download URL:
// <api>/projects/<id or path>/packages/generic/<name>/<version>/<file>.
func (r *releaseReader) genericPackageFile(raw string) (string, string, string, bool) {
	u, err := url.Parse(raw)
	base, berr := url.Parse(r.f.APIBaseURL)
	if err != nil || berr != nil || u.RawQuery != "" || u.Fragment != "" || !strings.EqualFold(canonicalHTTPSHost(u.Host), canonicalHTTPSHost(base.Host)) {
		return "", "", "", false
	}
	prefix := strings.TrimSuffix(base.EscapedPath(), "/") + "/projects/"
	rest, found := strings.CutPrefix(u.EscapedPath(), prefix)
	if !found {
		return "", "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 6 || parts[1] != "packages" || parts[2] != "generic" {
		return "", "", "", false
	}
	project, err := url.PathUnescape(parts[0])
	if err != nil || project != r.f.Project && (r.projectID < 1 || project != strconv.Itoa(r.projectID)) {
		return "", "", "", false
	}
	values := make([]string, 3)
	for i, p := range parts[3:] {
		v, err := url.PathUnescape(p)
		if err != nil || !validPackageSegment(v) {
			return "", "", "", false
		}
		values[i] = v
	}
	return values[0], values[1], values[2], true
}

// releaseLinkURLProblem refuses a link that cannot be durable release
// storage. It returns "" when the URL is acceptable to record.
func releaseLinkURLProblem(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Scheme != "https" || u.Host == "" || u.User != nil || strings.ContainsAny(raw, " \r\n\x00") {
		return "the link is not a plain HTTPS URL without credentials"
	}
	segments := strings.Split(path.Clean("/"+u.Path), "/")
	for i, s := range segments {
		if s == "jobs" && slices.Contains(segments[i+1:], "artifacts") {
			return "the link is a CI job artifact, which is ephemeral; publish the file to the package registry"
		}
	}
	return ""
}

func isGitlabStatus(err error, status int) bool {
	var re *gitlabReadError
	return errors.As(err, &re) && re.Status == status
}

func errorText(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}

// releaseOperation is one explicitly requested tag or release mutation.
type releaseOperation struct {
	Operation  string
	Tag, Ref   string
	Title      string
	Notes      string
	Message    string
	LinkName   string
	LinkURL    string
	LinkType   string
	Actor      string
	Delegation string
	Authorized bool
}

func validateReleaseOperation(o releaseOperation) error {
	if !slices.Contains(releaseOperations, o.Operation) {
		return errors.New("unsupported release operation")
	}
	if !o.Authorized {
		return errors.New("a release mutation requires the invoking workflow's explicit authorization and --authorized assertion")
	}
	if _, err := parseAuthorityMember(o.Actor); err != nil || !strings.HasPrefix(o.Actor, "gitlab:") {
		return errors.New("a release mutation requires --actor gitlab:HOST/USER")
	}
	if o.Delegation == "" {
		return errors.New("a release mutation requires --delegation with the Workflow Owner's release delegation note URL")
	}
	if !validReleaseTag(o.Tag) {
		return errors.New("the tag name is not valid")
	}
	if !objectNamePattern.MatchString(o.Ref) {
		return errors.New("--ref must be a full commit object name")
	}
	singleLine := func(s string) bool {
		return utf8.ValidString(s) && len(s) <= 255 && !strings.ContainsAny(s, "\r\n\x00")
	}
	switch o.Operation {
	case releaseOpTag:
		if !singleLine(o.Message) {
			return errors.New("a tag message must be one bounded line")
		}
	case releaseOpCreate, releaseOpUpdate:
		if strings.TrimSpace(o.Title) == "" || !singleLine(o.Title) {
			return errors.New("a release requires a nonempty single-line --title")
		}
		if !utf8.ValidString(o.Notes) || len(o.Notes) > 1_000_000 || strings.TrimSpace(o.Notes) == "" || strings.ContainsRune(o.Notes, 0) {
			return errors.New("release notes must be nonempty bounded text")
		}
	case releaseOpLink:
		if strings.TrimSpace(o.LinkName) == "" || !singleLine(o.LinkName) {
			return errors.New("an asset link requires a nonempty single-line --name")
		}
		if !slices.Contains(releaseLinkTypes, o.LinkType) {
			return errors.New("--link-type must be other, package, image or runbook")
		}
		if problem := releaseLinkURLProblem(o.LinkURL); problem != "" {
			return errors.New(problem)
		}
	}
	return nil
}

// executeGitlabRelease performs one delegated tag or release mutation and
// verifies it by read-back. It never retries and never deletes.
func executeGitlabRelease(ctx context.Context, cfg projectConfig, o releaseOperation, run glabInputRunner) (forgeOperationResult, error) {
	result := forgeOperationResult{}
	if problems := validateConfig(cfg); problems != nil {
		return result, problems
	}
	if proj(&cfg).Forge == nil || cfg.Project.Forge.Provider != "gitlab" {
		return result, errors.New("release operations require explicit GitLab configuration; GitHub retains its existing tools")
	}
	if err := validateReleaseOperation(o); err != nil {
		return result, err
	}
	f := effectiveForge(*cfg.Project.Forge)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	c := &gitlabOperations{run: run, forge: f}
	projectPath := "projects/" + url.PathEscape(f.Project)
	var project glProject
	if _, err := c.request(ctx, "GET", projectPath, nil, &project); err != nil {
		return result, err
	}
	if project.ID < 1 || project.Path != f.Project {
		return result, errors.New("GitLab project identity does not match configured target")
	}
	actor, err := c.checkActor(ctx, o.Actor)
	if err != nil {
		return result, err
	}
	var membership glMembership
	if _, err := c.request(ctx, "GET", projectPath+"/members/all/"+strconv.Itoa(actor.ID), nil, &membership); err != nil {
		return result, err
	}
	if membership.ID != actor.ID || membership.AccessLevel == nil || *membership.AccessLevel < 30 {
		return result, errors.New("release operations require checked Developer-or-higher project access")
	}
	if err := c.verifyDelegation(ctx, cfg, o, project.ID); err != nil {
		return result, err
	}

	tagPath := projectPath + "/repository/tags/" + url.PathEscape(o.Tag)
	var tag glTag
	_, tagErr := c.request(ctx, "GET", tagPath, nil, &tag)
	if o.Operation == releaseOpTag {
		if tagErr == nil {
			return result, errors.New("the tag already exists; tags are never moved or replaced")
		}
		if !isGitlabStatus(tagErr, 404) {
			return result, tagErr
		}
		fields := map[string]string{"tag_name": o.Tag, "ref": o.Ref}
		if o.Message != "" {
			fields["message"] = o.Message
		}
		var created glTag
		if _, err := c.request(ctx, "POST", projectPath+"/repository/tags", fields, &created); err != nil {
			return result, err
		}
		var read glTag
		if _, err := c.request(ctx, "GET", tagPath, nil, &read); err != nil {
			return result, err
		}
		if read.Name != o.Tag || read.Commit.ID != o.Ref || read.Message != o.Message {
			return result, errors.New("GitLab tag read-back mismatch; not verified, inspect before retrying")
		}
		return forgeOperationResult{URL: f.Endpoint + "/" + f.Project + "/-/tags/" + url.PathEscape(o.Tag), Verified: true, Complete: true}, nil
	}
	if tagErr != nil {
		return result, errors.New("the tag must already exist at the delegated commit: " + tagErr.Error())
	}
	if tag.Name != o.Tag || tag.Commit.ID != o.Ref {
		return result, errors.New("the tag does not point to the delegated commit")
	}

	releasePath := projectPath + "/releases/" + url.PathEscape(o.Tag)
	var existing glRelease
	_, relErr := c.request(ctx, "GET", releasePath, nil, &existing)
	switch o.Operation {
	case releaseOpCreate:
		if relErr == nil {
			return result, errors.New("a release already exists for the tag; use an explicitly delegated update")
		}
		if !isGitlabStatus(relErr, 404) {
			return result, relErr
		}
		// No ref is sent: release creation must never create a tag implicitly.
		fields := map[string]string{"tag_name": o.Tag, "name": o.Title, "description": o.Notes}
		var created glRelease
		if _, err := c.request(ctx, "POST", projectPath+"/releases", fields, &created); err != nil {
			return result, err
		}
		return c.readBackRelease(ctx, f, releasePath, o)
	case releaseOpUpdate:
		if relErr != nil {
			return result, relErr
		}
		if existing.TagName != o.Tag || existing.Commit.ID != o.Ref {
			return result, errors.New("the release does not belong to the delegated tag and commit")
		}
		var updated glRelease
		if _, err := c.request(ctx, "PUT", releasePath, map[string]string{"name": o.Title, "description": o.Notes}, &updated); err != nil {
			return result, err
		}
		return c.readBackRelease(ctx, f, releasePath, o)
	}

	// releaseOpLink
	if relErr != nil {
		return result, relErr
	}
	if existing.TagName != o.Tag || existing.Commit.ID != o.Ref {
		return result, errors.New("the release does not belong to the delegated tag and commit")
	}
	links, err := operationList[glReleaseLink](ctx, c, releasePath+"/assets/links", nil)
	if err != nil {
		return result, err
	}
	for _, l := range links {
		if l.Name == o.LinkName || l.URL == o.LinkURL {
			return result, errors.New("the release already has a link with this name or URL; links are never replaced")
		}
	}
	var created glReleaseLink
	if _, err := c.request(ctx, "POST", releasePath+"/assets/links", map[string]string{"name": o.LinkName, "url": o.LinkURL, "link_type": o.LinkType}, &created); err != nil {
		return result, err
	}
	if created.ID < 1 {
		return result, errors.New("GitLab link creation returned no link ID; inspect before retrying")
	}
	var read glReleaseLink
	if _, err := c.request(ctx, "GET", releasePath+"/assets/links/"+strconv.Itoa(created.ID), nil, &read); err != nil {
		return result, err
	}
	if read.ID != created.ID || read.Name != o.LinkName || read.URL != o.LinkURL || read.LinkType != o.LinkType {
		return result, errors.New("GitLab release link read-back mismatch; not verified, inspect before retrying")
	}
	return forgeOperationResult{URL: f.Endpoint + "/" + f.Project + "/-/releases/" + url.PathEscape(o.Tag), Verified: true, Complete: true}, nil
}

func (c *gitlabOperations) readBackRelease(ctx context.Context, f forgeConfig, releasePath string, o releaseOperation) (forgeOperationResult, error) {
	var read glRelease
	if _, err := c.request(ctx, "GET", releasePath, nil, &read); err != nil {
		return forgeOperationResult{}, err
	}
	if read.TagName != o.Tag || read.Name != o.Title || read.Description != o.Notes || read.Commit.ID != o.Ref {
		return forgeOperationResult{}, errors.New("GitLab release read-back mismatch; not verified, inspect before retrying")
	}
	return forgeOperationResult{URL: f.Endpoint + "/" + f.Project + "/-/releases/" + url.PathEscape(o.Tag), Verified: true, Complete: true}, nil
}

// delegationBlock is a release-delegation record's YAML block.
type delegationBlock struct {
	GarageDelegation int    `yaml:"garage_delegation"`
	Issue            int    `yaml:"issue"`
	Operation        string `yaml:"operation"`
	Tag              string `yaml:"tag"`
	Ref              string `yaml:"ref"`
	Designation      string `yaml:"designation"`
}

func parseDelegation(body string) (*delegationBlock, bool) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	first, rest, _ := strings.Cut(strings.TrimLeft(body, " \t\n"), "\n")
	if strings.TrimSpace(first) != markerDelegation {
		return nil, false
	}
	raw, ok := firstYAMLBlock(rest)
	if !ok {
		return nil, false
	}
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.KnownFields(true)
	var block delegationBlock
	if decoder.Decode(&block) != nil {
		return nil, false
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, false
	}
	if block.GarageDelegation != 1 || block.Issue < 1 || !slices.Contains(releaseOperations, block.Operation) || !validReleaseTag(block.Tag) || !objectNamePattern.MatchString(block.Ref) || block.Designation != "delegated" && block.Designation != "withdrawn" {
		return nil, false
	}
	return &block, true
}

// verifyDelegation requires that the linked note is the latest valid
// delegation record, by an E8-qualified Workflow Owner, for this exact
// operation, tag and ref, and that it delegates rather than withdraws.
func (c *gitlabOperations) verifyDelegation(ctx context.Context, cfg projectConfig, o releaseOperation, projectID int) error {
	u, err := url.Parse(o.Delegation)
	if err != nil || !strings.HasPrefix(u.Fragment, "note_") || !positiveID(strings.TrimPrefix(u.Fragment, "note_")) {
		return errors.New("--delegation must identify an exact GitLab issue note")
	}
	noteID, _ := strconv.Atoi(strings.TrimPrefix(u.Fragment, "note_"))
	u.Fragment = ""
	t, err := parseGitlabTarget([]string{u.String()}, c.forge)
	if err != nil || t.Kind != workflowIssueKind || t.Project != c.forge.Project {
		return errors.New("--delegation must be a note on a release issue in the configured project")
	}
	resource, err := c.item(ctx, t, projectID)
	if err != nil {
		return err
	}
	notes, err := operationList[operationNote](ctx, c, c.resource(t)+"/"+strconv.Itoa(t.Number)+"/notes", nil)
	if err != nil {
		return errors.New("the release issue's notes are unavailable or incomplete; delegation cannot be verified")
	}
	sort.SliceStable(notes, func(i, j int) bool {
		if notes[i].CreatedAt.Equal(notes[j].CreatedAt) {
			return notes[i].ID < notes[j].ID
		}
		return notes[i].CreatedAt.Before(notes[j].CreatedAt)
	})
	var owner *authority
	if cfg.Project.Authorities != nil {
		owner = cfg.Project.Authorities.WorkflowOwner
	}
	latestID := 0
	var latest *delegationBlock
	for _, note := range notes {
		if note.ID < 1 || note.ProjectID != projectID || note.NoteableID != resource.ID || note.NoteableIID != t.Number || note.NoteableType != "Issue" {
			return errors.New("GitLab note collection identity mismatch")
		}
		if note.System {
			continue
		}
		block, ok := parseDelegation(note.Body)
		if !ok || block.Issue != t.Number || !authorityMatches(owner, "gitlab", forgeHost(c.forge), note.Author.Username) {
			continue
		}
		if block.Operation == o.Operation && block.Tag == o.Tag && block.Ref == o.Ref {
			latestID, latest = note.ID, block
		}
	}
	if latest == nil || latestID != noteID {
		return errors.New("--delegation is not the current Workflow Owner delegation for this exact operation, tag and ref")
	}
	if latest.Designation != "delegated" {
		return errors.New("the Workflow Owner withdrew this delegation")
	}
	return nil
}

// checkActor confirms the authenticated glab identity is the expected actor.
func (c *gitlabOperations) checkActor(ctx context.Context, expectedActor string) (glUser, error) {
	var actor glUser
	if _, err := c.request(ctx, "GET", "user", nil, &actor); err != nil {
		return actor, err
	}
	expected, _ := parseAuthorityMember(expectedActor)
	if actor.ID < 1 || actor.Username == "" || !strings.EqualFold(expected.Host, canonicalHTTPSHost(forgeHost(c.forge))) || expected.User != actor.Username {
		return actor, errors.New("authenticated GitLab identity does not match expected actor")
	}
	return actor, nil
}

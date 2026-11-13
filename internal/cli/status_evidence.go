package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Workflow records, as the canonical skills write them on GitHub, and the
// garage-evidence block defined in docs/review-evidence.md.

const (
	markerImplementation = "[AGENT #1 — IMPLEMENTATION]"
	markerCorrection     = "[AGENT #1 — CORRECTION]"
	markerReview         = "[AGENT #2 — INDEPENDENT REVIEW]"
	markerReviewRequest  = "Agent #2 PR review requested"
)

// recordKind classifies a workflow record.
type recordKind int

const (
	// recordEvidence carries a well-formed garage-evidence block.
	recordEvidence recordKind = iota
	// recordLegacy has a workflow marker but no evidence block. It was
	// written before the format was adopted and is not recomputed.
	recordLegacy
	// recordInvalid has an evidence block that is not well formed.
	recordInvalid
	// recordRequest is an Agent #2 PR Review Request, which has no block.
	recordRequest
)

// countedAssociations are the GitHub author associations whose workflow
// records count (#40 decision, item 3).
var countedAssociations = []string{"OWNER", "MEMBER", "COLLABORATOR"}

var (
	objectNamePattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	changesetPattern  = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	requestHeadLine   = regexp.MustCompile("(?m)^PR head:\\s*`?([0-9a-f]{40}|[0-9a-f]{64})`?\\s*$")
)

type evidenceArtifact struct {
	Method         string `yaml:"method"`
	ObjectFormat   string `yaml:"object_format"`
	Stage          string `yaml:"stage"`
	IntegrationRef string `yaml:"integration_ref"`
	Base           string `yaml:"base"`
	Tree           string `yaml:"tree"`
	Changeset      string `yaml:"changeset"`
	Paths          *int   `yaml:"paths"`
	Commit         string `yaml:"commit,omitempty"`
}

type evidenceAnswers struct {
	AcceptanceCriteriaMet *bool `yaml:"acceptance_criteria_met"`
	NewRegression         *bool `yaml:"new_regression"`
	Works                 *bool `yaml:"works"`
}

type evidenceAsserted struct {
	Status           string           `yaml:"status,omitempty"`
	ValidationRun    *bool            `yaml:"validation_run,omitempty"`
	ExecutionContext string           `yaml:"execution_context,omitempty"`
	Inspected        string           `yaml:"inspected,omitempty"`
	Answers          *evidenceAnswers `yaml:"answers,omitempty"`
	Verdict          string           `yaml:"verdict,omitempty"`
}

type evidenceCheckable struct {
	Artifact          *evidenceArtifact `yaml:"artifact"`
	MatchesRespondsTo *bool             `yaml:"matches_responds_to,omitempty"`
}

// evidenceBlock is a garage-evidence version 1 block.
type evidenceBlock struct {
	GarageEvidence int               `yaml:"garage_evidence"`
	Record         string            `yaml:"record"`
	Issue          int               `yaml:"issue"`
	RespondsTo     string            `yaml:"responds_to,omitempty"`
	Supplements    string            `yaml:"supplements,omitempty"`
	EquivalentTo   string            `yaml:"equivalent_to,omitempty"`
	Checkable      evidenceCheckable `yaml:"checkable"`
	Asserted       *evidenceAsserted `yaml:"asserted,omitempty"`
}

func (b *evidenceBlock) artifact() *evidenceArtifact { return b.Checkable.Artifact }

func (b *evidenceBlock) verdict() string {
	if b.Asserted == nil {
		return ""
	}
	return b.Asserted.Verdict
}

func (b *evidenceBlock) status() string {
	if b.Asserted == nil {
		return ""
	}
	return b.Asserted.Status
}

// workflowRecord is one workflow-state comment on an issue or pull request.
type workflowRecord struct {
	Comment  ghComment
	Location string // "issue #33" or "PR #46"
	Marker   string
	Kind     recordKind
	Block    *evidenceBlock
	Problem  string // why an evidence block is invalid
	// RequestHead is the head an Agent #2 PR Review Request names.
	RequestHead string
	// Counted reports whether the author's association lets the record count.
	Counted bool
}

func (r *workflowRecord) author() string {
	if r.Comment.Author == nil {
		return "a deleted account"
	}
	return r.Comment.Author.Login
}

// isEvidence reports whether the record is a counted, well-formed evidence
// record of the given type.
func (r *workflowRecord) isEvidence(recordType string) bool {
	return r.Counted && r.Kind == recordEvidence && r.Block.Record == recordType
}

// parseWorkflowRecords returns the workflow records among comments, in their
// original order. Comments without a workflow marker are not records.
func parseWorkflowRecords(comments []ghComment, location string) []workflowRecord {
	var records []workflowRecord
	for _, c := range comments {
		marker, rest, ok := workflowMarker(c.Body)
		if !ok {
			continue
		}
		rec := workflowRecord{
			Comment:  c,
			Location: location,
			Marker:   marker,
			Counted:  contains(countedAssociations, c.AuthorAssociation),
		}
		if marker == markerReviewRequest {
			rec.Kind = recordRequest
			if m := requestHeadLine.FindStringSubmatch(rest); m != nil {
				rec.RequestHead = m[1]
			}
			records = append(records, rec)
			continue
		}
		raw, found := firstYAMLBlock(rest)
		if !found {
			rec.Kind = recordLegacy
			records = append(records, rec)
			continue
		}
		block, err := decodeEvidenceBlock(raw)
		if err == nil {
			err = validateEvidenceBlock(block, marker)
		}
		if err != nil {
			rec.Kind = recordInvalid
			rec.Problem = err.Error()
		} else {
			rec.Kind = recordEvidence
			rec.Block = block
		}
		records = append(records, rec)
	}
	return records
}

// workflowMarker returns the marker on the first non-empty line of body and
// the text after that line.
func workflowMarker(body string) (string, string, bool) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	for {
		line, rest, more := strings.Cut(body, "\n")
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			switch trimmed {
			case markerImplementation, markerCorrection, markerReview, markerReviewRequest:
				return trimmed, rest, true
			}
			return "", "", false
		}
		if !more {
			return "", "", false
		}
		body = rest
	}
}

// firstYAMLBlock returns the content of the first fenced yaml block in text.
func firstYAMLBlock(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "```yaml" {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "```" {
				return strings.Join(lines[i+1:j], "\n"), true
			}
		}
		return "", false
	}
	return "", false
}

// decodeEvidenceBlock strictly decodes one YAML document: unknown keys and
// trailing documents are errors.
func decodeEvidenceBlock(raw string) (*evidenceBlock, error) {
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.KnownFields(true)
	var block evidenceBlock
	if err := decoder.Decode(&block); err != nil {
		return nil, fmt.Errorf("the evidence block is not valid: %s", oneLine(err.Error()))
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("the evidence block holds more than one YAML document")
	}
	return &block, nil
}

// recordMarkers maps each record type to the marker its record carries.
var recordMarkers = map[string][]string{
	"implementation": {markerImplementation},
	"correction":     {markerCorrection},
	"equivalence":    {markerImplementation, markerCorrection},
	"review":         {markerReview},
	"pr-review":      {markerReview},
}

// validateEvidenceBlock checks the mechanically checkable form of a block.
// It never judges the asserted values beyond their allowed forms.
func validateEvidenceBlock(b *evidenceBlock, marker string) error {
	if b.GarageEvidence != 1 {
		return fmt.Errorf("garage_evidence is %d, not 1", b.GarageEvidence)
	}
	markers, ok := recordMarkers[b.Record]
	if !ok {
		return fmt.Errorf("record %q is not a known record type", b.Record)
	}
	if !contains(markers, marker) {
		return fmt.Errorf("a %s record does not belong under %s", b.Record, marker)
	}
	if b.Issue < 1 {
		return errors.New("issue is missing")
	}
	a := b.artifact()
	if a == nil {
		return errors.New("checkable.artifact is missing")
	}
	if a.Method != "garage-artifact-v1" {
		return fmt.Errorf("artifact method %q is not garage-artifact-v1", a.Method)
	}
	if a.ObjectFormat != "sha1" && a.ObjectFormat != "sha256" {
		return fmt.Errorf("object_format %q is not sha1 or sha256", a.ObjectFormat)
	}
	for name, value := range map[string]string{"integration_ref": a.IntegrationRef, "base": a.Base, "tree": a.Tree} {
		if !objectNamePattern.MatchString(value) {
			return fmt.Errorf("artifact %s is not a full object name", name)
		}
	}
	if !changesetPattern.MatchString(a.Changeset) {
		return errors.New("artifact changeset is not sha256:<64 hex>")
	}
	if a.Paths == nil || *a.Paths < 0 {
		return errors.New("artifact paths is missing")
	}
	switch a.Stage {
	case "working-tree":
		if a.Commit != "" {
			return errors.New("a working-tree artifact names a commit")
		}
	case "commit":
		if !objectNamePattern.MatchString(a.Commit) {
			return errors.New("a commit artifact does not name its commit")
		}
	default:
		return fmt.Errorf("artifact stage %q is not working-tree or commit", a.Stage)
	}

	switch b.Record {
	case "implementation", "correction":
		return checkAssertedValue("status", b.status(), "ready-for-review", "ready-for-re-review", "material-blocker")
	case "review", "pr-review":
		if b.RespondsTo == "" {
			return errors.New("responds_to is missing")
		}
		if b.Record == "pr-review" && b.Supplements == "" {
			return errors.New("a pr-review record has no supplements link")
		}
		return checkAssertedValue("verdict", b.verdict(), "approved", "changes-required")
	case "equivalence":
		if a.Stage != "commit" {
			return errors.New("an equivalence record must identify a commit")
		}
		if b.EquivalentTo == "" {
			return errors.New("equivalent_to is missing")
		}
	}
	return nil
}

func checkAssertedValue(field, value string, allowed ...string) error {
	if !contains(allowed, value) {
		return fmt.Errorf("asserted %s %q is not one of: %s", field, value, strings.Join(allowed, ", "))
	}
	return nil
}

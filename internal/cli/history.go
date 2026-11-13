package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Developer-local workflow history.
//
// History is a private, best-effort annotation of what a developer's agents
// did. It is never workflow evidence: GitHub Issue and pull request records
// are authoritative, and losing or corrupting this history must never block or
// authorize work. It lives in the user cache, outside every repository, and is
// never synchronized.

const (
	// historySchemaVersion identifies the layout of one stored event.
	historySchemaVersion = 1
	// historyEventLimit is the number of events kept per repository. Older
	// events are discarded when a new event would exceed it.
	historyEventLimit = 500
	// historyMaxFileBytes bounds how much of a history file is read. A file
	// larger than this cannot be valid history and is discarded.
	historyMaxFileBytes = 4 << 20

	historyAppDir   = "builder-garage"
	historySubDir   = "history"
	historyFileExt  = ".jsonl"
	maxURLLength    = 512
	maxArtifactLen  = 256
	maxTargetNumber = 1_000_000_000
)

// Workflow event types. Each corresponds to a canonical skill or to an
// Agent #1 / Agent #2 workflow-state record.
var historyEventTypes = []string{
	"discovery",      // engineering-discovery-workflow
	"readiness",      // implementation-readiness-workflow
	"implementation", // [AGENT #1 — IMPLEMENTATION]
	"correction",     // [AGENT #1 — CORRECTION]
	"review",         // [AGENT #2 — INDEPENDENT REVIEW], implementation review
	"pr-review",      // [AGENT #2 — INDEPENDENT REVIEW], PR review
	"finalization",   // integration-pr-review finalization by Agent #1
}

// Workflow outcomes. Each corresponds to a terminal state or verdict named by
// the canonical skills.
var historyOutcomes = []string{
	"ready",            // READY FOR AGENT #2 REVIEW / RE-REVIEW, or readiness READY
	"not-ready",        // readiness NOT READY
	"blocked",          // MATERIAL BLOCKER
	"approved",         // APPROVED / APPROVED — MERGE
	"changes-required", // CHANGES REQUIRED
	"complete",         // discovery complete, or a PR recorded with no review required
	"merged",           // the pull request was merged
}

// historyTargetKinds are the GitHub records an event can refer to.
var historyTargetKinds = []string{"issue", "pr"}

var (
	repoKeyPattern  = regexp.MustCompile(`^[0-9a-f]{32}$`)
	artifactPattern = regexp.MustCompile(`^[\x21-\x7e]+$`)
)

// errHistoryInput reports an event that was rejected before anything was
// stored.
var errHistoryInput = errors.New("invalid event")

// historyEvent is one stored event. It holds only the fields the #19 decision
// permits: no branch names, prompts, conversation text, diffs, file contents,
// environment values, or tokens.
type historyEvent struct {
	Version  int    `json:"v"`
	Time     string `json:"time"`
	Repo     string `json:"repo"`
	Kind     string `json:"kind"`
	Number   int    `json:"number"`
	Type     string `json:"type"`
	Outcome  string `json:"outcome"`
	URL      string `json:"url,omitempty"`
	Artifact string `json:"artifact,omitempty"`
}

// validate checks every field of an event against the permitted values.
func (e historyEvent) validate() error {
	if e.Version != historySchemaVersion {
		return fmt.Errorf("%w: unsupported version %d", errHistoryInput, e.Version)
	}
	if _, err := time.Parse(time.RFC3339, e.Time); err != nil {
		return fmt.Errorf("%w: time is not an RFC 3339 timestamp", errHistoryInput)
	}
	if !repoKeyPattern.MatchString(e.Repo) {
		return fmt.Errorf("%w: repository key is malformed", errHistoryInput)
	}
	if err := validateTarget(e.Kind, e.Number); err != nil {
		return err
	}
	if !contains(historyEventTypes, e.Type) {
		return fmt.Errorf("%w: event type %q is not one of: %s",
			errHistoryInput, e.Type, strings.Join(historyEventTypes, ", "))
	}
	if !contains(historyOutcomes, e.Outcome) {
		return fmt.Errorf("%w: outcome %q is not one of: %s",
			errHistoryInput, e.Outcome, strings.Join(historyOutcomes, ", "))
	}
	if err := validateEventURL(e.URL); err != nil {
		return err
	}
	return validateArtifact(e.Artifact)
}

func validateTarget(kind string, number int) error {
	if !contains(historyTargetKinds, kind) {
		return fmt.Errorf("%w: target must be an issue or a pull request", errHistoryInput)
	}
	if number < 1 || number > maxTargetNumber {
		return fmt.Errorf("%w: %s number must be a positive integer", errHistoryInput, kind)
	}
	return nil
}

// validateEventURL accepts an optional link to the durable GitHub record. It
// must be an absolute https URL without credentials or a query string, so a
// token cannot be stored through it.
func validateEventURL(raw string) error {
	if raw == "" {
		return nil
	}
	if len(raw) > maxURLLength || strings.IndexFunc(raw, isSpaceOrControl) >= 0 {
		return fmt.Errorf("%w: url must be a single https URL of at most %d characters",
			errHistoryInput, maxURLLength)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("%w: url must be an absolute https URL", errHistoryInput)
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery {
		return fmt.Errorf("%w: url must not contain credentials or a query string", errHistoryInput)
	}
	return nil
}

// validateArtifact accepts an optional artifact identity. Its format is
// pending #34, so only a length and character bound is applied here: a single
// printable token, which cannot carry a diff or free text.
func validateArtifact(raw string) error {
	if raw == "" {
		return nil
	}
	if len(raw) > maxArtifactLen || !artifactPattern.MatchString(raw) {
		return fmt.Errorf("%w: artifact must be a single printable token of at most %d characters",
			errHistoryInput, maxArtifactLen)
	}
	return nil
}

func isSpaceOrControl(r rune) bool {
	return r <= ' ' || r == 0x7f
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

// historyAppendResult describes what an append did to the stored history.
type historyAppendResult struct {
	Path      string
	Kept      int
	Discarded int
	Trimmed   int
}

// historyPath returns the history file for a repository key under cacheDir.
func historyPath(cacheDir, repoKey string) string {
	return filepath.Join(cacheDir, historyAppDir, historySubDir, repoKey+historyFileExt)
}

// appendHistoryEvent validates event, adds it to the repository's history
// under cacheDir, and keeps at most historyEventLimit events. Stored lines
// that are unreadable or no longer valid are discarded and counted, never
// treated as an error. The file is replaced through a temporary file in the
// same directory, so a failed write leaves the previous history intact.
//
// The whole read, trim, encode, and replace runs under the history's
// cross-process lock, so concurrent recordings to the same history, from any
// clone or worktree, each keep their event. If the lock stays held for the
// bounded wait, nothing is written and an error is returned.
func appendHistoryEvent(cacheDir string, event historyEvent) (historyAppendResult, error) {
	if err := event.validate(); err != nil {
		return historyAppendResult{}, err
	}

	path := historyPath(cacheDir, event.Repo)
	result := historyAppendResult{Path: path}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return result, fmt.Errorf("cannot create the history directory: %w", err)
	}

	unlock, err := lockHistory(path, historyLockTimeout)
	if err != nil {
		return result, err
	}
	defer unlock()

	existing, discarded, err := readHistory(path)
	if err != nil {
		return result, err
	}
	result.Discarded = discarded

	events := append(existing, event)
	if over := len(events) - historyEventLimit; over > 0 {
		events = events[over:]
		result.Trimmed = over
	}
	result.Kept = len(events)

	content, err := encodeHistory(events)
	if err != nil {
		return result, err
	}
	if err := writeHistory(path, content); err != nil {
		return result, err
	}
	return result, nil
}

// readHistory returns the valid events stored at path and the number of lines
// it discarded. A missing file is empty history. A symbolic link or other
// non-regular file is refused rather than followed.
func readHistory(path string) ([]historyEvent, int, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("cannot inspect the history file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("refusing to use %s: it is not a regular file", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("cannot read the history file: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, historyMaxFileBytes+1))
	if err != nil {
		return nil, 0, fmt.Errorf("cannot read the history file: %w", err)
	}
	if len(data) > historyMaxFileBytes {
		// Far larger than the event limit allows: discard it as corrupt.
		return nil, bytes.Count(data, []byte("\n")) + 1, nil
	}

	var events []historyEvent
	discarded := 0
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), historyMaxFileBytes)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		event, ok := decodeHistoryLine(line)
		if !ok {
			discarded++
			continue
		}
		events = append(events, event)
	}
	if scanner.Err() != nil {
		// The remainder is unreadable; keep what was decoded and discard the rest.
		discarded++
	}
	return events, discarded, nil
}

// decodeHistoryLine decodes one stored event. Unknown fields, trailing data,
// and invalid values all make the line invalid.
func decodeHistoryLine(line []byte) (historyEvent, bool) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	var event historyEvent
	if err := decoder.Decode(&event); err != nil {
		return historyEvent{}, false
	}
	if decoder.More() {
		return historyEvent{}, false
	}
	if event.validate() != nil {
		return historyEvent{}, false
	}
	return event, true
}

func encodeHistory(events []historyEvent) ([]byte, error) {
	var buf bytes.Buffer
	for _, event := range events {
		line, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("cannot encode the history: %w", err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// writeHistory replaces the history file with content through a private
// temporary file in the same directory.
func writeHistory(path string, content []byte) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".history.tmp-*")
	if err != nil {
		return fmt.Errorf("cannot write the history: %w", err)
	}
	tempPath := file.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	_, writeErr := file.Write(content)
	chmodErr := file.Chmod(0o600)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, chmodErr, syncErr, closeErr); err != nil {
		return fmt.Errorf("cannot write the history: %w", err)
	}

	// Refuse to replace something that is not a regular file, such as a
	// symbolic link created since the history was read.
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to replace %s: it is not a regular file", path)
	}
	if err := commitConfigReplacement(tempPath, path); err != nil {
		return fmt.Errorf("cannot write the history: %w", err)
	}
	committed = true
	return nil
}

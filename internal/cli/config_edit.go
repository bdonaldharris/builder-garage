package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// errConfigEditChanged reports that the configuration changed after a
// configuration command read it. The command stops without writing rather
// than overwrite the change.
var errConfigEditChanged = errors.New("the configuration changed while the command was running")

// errConfirmationRequired reports that a change was not confirmed: outside
// an interactive terminal, a change is written only with --yes.
var errConfirmationRequired = errors.New("the change was not confirmed")

// configChange is one requested change to the configuration.
type configChange struct {
	// apply makes the change to a candidate configuration.
	apply func(*projectConfig) error
	// retiredUnset marks "unset conditional-guidance" alone, the only change
	// that may be made to an invalid configuration, and only when the
	// result validates (#64 decision 2).
	retiredUnset bool
}

// changeConfiguration makes one change to the configuration in projectDir by
// the same pipeline init uses: snapshot, strict decode, refuse an invalid
// file, validate the complete candidate, edit the YAML nodes, check the
// round trip, confirm, and replace the file atomically if it is unchanged.
// ask is nil outside an interactive terminal; yes confirms without asking.
func changeConfiguration(projectDir string, change configChange, ask prompter, yes bool, out io.Writer) error {
	garageDir := filepath.Join(projectDir, garageDirName)
	configPath := filepath.Join(garageDir, configFileName)
	configRel := filepath.Join(garageDirName, configFileName)
	stop := func(format string, args ...any) error {
		return fmt.Errorf("%s was not changed: %s", configRel, fmt.Sprintf(format, args...))
	}

	// Identify the file before reading it. If it is replaced in between,
	// the snapshot no longer matches and the change stops before writing.
	readInfo, err := identifyFile(configPath)
	if err != nil {
		if _, readErr := readConfigFile(configPath); readErr != nil {
			return readErr
		}
		return fmt.Errorf("cannot inspect %s: %w", configPath, err)
	}
	data, err := readConfigFile(configPath)
	if err != nil {
		return err
	}
	snapshot := configSnapshot{path: configPath, info: readInfo, data: data}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return stop("it is not valid YAML (%v); correct it by hand", err)
	}
	before, err := decodeConfig(data)
	if err != nil {
		return stop("it does not match the configuration schema (%v); correct it by hand", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return stop("it is not a YAML mapping; correct it by hand")
	}
	if problems := validateConfig(before); problems != nil && !change.retiredUnset {
		return stop("it is not a valid configuration:\n%s\nCorrect it first; \"garage config validate\" checks it",
			formatProblems(problems))
	}

	// A second decode of the same bytes is an independent copy to change.
	candidate, err := decodeConfig(data)
	if err != nil {
		return stop("%v", err)
	}
	if err := change.apply(&candidate); err != nil {
		return stop("%v", err)
	}
	if problems := validateConfig(candidate); problems != nil {
		return stop("the result would not be a valid configuration:\n%s", formatProblems(problems))
	}
	if reflect.DeepEqual(before, candidate) {
		_, err := fmt.Fprintf(out, "Nothing to change; %s already has that value.\n", configRel)
		return err
	}

	removedComments, err := editDocument(&doc, before, candidate)
	if err != nil {
		return err
	}
	content, err := encodeDocument(&doc)
	if err != nil {
		return err
	}
	// The written file must decode to exactly the validated configuration.
	if check, err := decodeConfig(content); err != nil || !reflect.DeepEqual(check, candidate) {
		return fmt.Errorf("internal error: the changed configuration did not round-trip; %s was not changed", configRel)
	}

	report := describeChanges(before, candidate) + describeRemovedComments(removedComments)
	if !yes {
		if ask == nil {
			return fmt.Errorf("%w: %s was not changed; this change needs confirmation:\n\n%s\n"+
				"Run the command in an interactive terminal to confirm it, or add --yes",
				errConfirmationRequired, configRel, report)
		}
		if err := ask.Tell("\nThis will change " + configRel + ":\n\n" + report + "\n"); err != nil {
			return err
		}
		ok, err := guide{p: ask}.yesNo("Write this change?", false)
		if err != nil {
			return fmt.Errorf("%s was not changed: %w", configRel, err)
		}
		if !ok {
			_, err := fmt.Fprintf(out, "Nothing was written; %s was not changed.\n", configRel)
			return err
		}
	}

	garageInfo, err := ensureGarageDirectory(garageDir)
	if err != nil {
		return err
	}
	unchanged := func() error {
		if snapshot.verifyUnchanged() != nil {
			return fmt.Errorf("%w; %s was not changed: run the command again to apply it to the current content",
				errConfigEditChanged, configRel)
		}
		return nil
	}
	if err := replaceConfiguration(configPath, content, garageInfo,
		prepareTemporaryConfig, commitConfigReplacement, unchanged); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Updated %s:\n\n%s", configRel, report)
	return err
}

// describeChanges lists every command-line setting whose value differs, in
// the settings' own names and formats.
func describeChanges(before, after projectConfig) string {
	var b strings.Builder
	for _, o := range allOptions() {
		old, now := o.get(&before), o.get(&after)
		if slices.Equal(old, now) {
			continue
		}
		if o.kind == optionValue {
			fmt.Fprintf(&b, "  %s: %s -> %s\n", o.name, displayValue(o, old), displayValue(o, now))
			continue
		}
		for _, item := range old {
			if !slices.Contains(now, item) {
				fmt.Fprintf(&b, "  %s: remove %s\n", o.name, quoteIfNeeded(item))
			}
		}
		for _, item := range now {
			if !slices.Contains(old, item) {
				fmt.Fprintf(&b, "  %s: add %s\n", o.name, quoteIfNeeded(item))
			}
		}
		if sameItems(old, now) {
			fmt.Fprintf(&b, "  %s: reorder\n", o.name)
		}
	}
	return b.String()
}

func sameItems(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	sort.Strings(a)
	sort.Strings(b)
	return slices.Equal(a, b)
}

func displayValue(o configOption, values []string) string {
	if values == nil {
		return o.def + " (default)"
	}
	return quoteIfNeeded(values[0])
}

// quoteIfNeeded quotes a value that would otherwise be ambiguous on one line.
func quoteIfNeeded(v string) string {
	if v == "" || strings.ContainsAny(v, "\n\r\t") || strings.TrimSpace(v) != v {
		return strconv.Quote(v)
	}
	return v
}

func describeRemovedComments(comments []string) string {
	if len(comments) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nThese comments belong to removed values and will be removed with them:\n")
	for _, c := range comments {
		for _, line := range strings.Split(c, "\n") {
			b.WriteString("  " + line + "\n")
		}
	}
	return b.String()
}

// problemIdentifier matches an extension-point identifier in a validation
// problem.
var problemIdentifier = regexp.MustCompile(` \(E[0-9]+\)`)

// problemItem matches a list index in a validation problem, which is
// restated as a position counted from 1.
var problemItem = regexp.MustCompile(`\[([0-9]+)\]`)

// problemParts name the parts of a value as the flag formats do.
var problemParts = map[string]string{
	"name":              "NAME",
	"applies_when":      "WHEN",
	"required_evidence": "EVIDENCE",
	"version":           "VERSION",
	"confirmed_version": "VERSION",
	"confirmed_on":      "date",
	"harness":           "HARNESS",
	"scope":             "scope",
	"path":              "path",
	"role":              "ROLE",
	"members":           "MEMBER",
}

// problemPart matches a field of a value within a validation problem.
var problemPart = regexp.MustCompile(`\.(applies_when|required_evidence|confirmed_version|confirmed_on|name|version|harness|scope|path|role|members)\b`)

// problemRewrites restate validation wording that speaks in schema terms.
var problemRewrites = []struct{ from, to string }{
	{"discovery-trigger apply only when mode is", "discovery-trigger applies only when discovery is"},
	{"discovery-trigger must list at least one trigger when mode is \"triggers\"",
		"discovery \"triggers\" needs at least one discovery-trigger"},
	{"is required when small_corrections is present", "is required when small-corrections-without-issue is set"},
	{"applies only when small corrections are enabled", "applies only when small-corrections is enabled"},
	{"is required when small corrections are enabled", "is required when small-corrections is enabled"},
	{"; remove conditional-guidance", "; remove it with \"garage config unset conditional-guidance\""},
}

// translateProblem restates one validation problem with command-line
// setting names in place of configuration fields and extension-point
// identifiers.
func translateProblem(problem string) string {
	problem = problemIdentifier.ReplaceAllString(problem, "")
	type field struct{ field, name string }
	var fields []field
	for _, o := range allOptions() {
		for _, f := range o.fields {
			fields = append(fields, field{f, o.name})
		}
	}
	// Longer fields first, so the most specific setting name applies.
	sort.SliceStable(fields, func(i, j int) bool { return len(fields[i].field) > len(fields[j].field) })
	for _, f := range fields {
		problem = strings.ReplaceAll(problem, f.field, f.name)
	}
	problem = problemItem.ReplaceAllStringFunc(problem, func(m string) string {
		n, _ := strconv.Atoi(problemItem.FindStringSubmatch(m)[1])
		return " " + strconv.Itoa(n+1)
	})
	problem = problemPart.ReplaceAllStringFunc(problem, func(m string) string {
		return " " + problemParts[m[1:]]
	})
	for _, r := range problemRewrites {
		problem = strings.ReplaceAll(problem, r.from, r.to)
	}
	return problem
}

func formatProblems(problems configProblems) string {
	var b strings.Builder
	for _, p := range problems {
		b.WriteString("  - " + translateProblem(p) + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// editDocument changes doc from before to after, setting by setting. Every
// node of an unchanged setting is kept with its comments. A changed value is
// merged into its existing node, so its key keeps its comments. A removed
// setting or item is deleted with its comments, which are returned.
func editDocument(doc *yaml.Node, before, after projectConfig) ([]string, error) {
	root := doc.Content[0]
	var removed []string
	for _, s := range configSettings {
		old, now := s.get(&before), s.get(&after)
		if reflect.DeepEqual(old, now) {
			continue
		}
		switch {
		case now == nil:
			removed = append(removed, deleteFromMapping(root, s.path)...)
		case old == nil:
			if err := addToMapping(root, s.path, now); err != nil {
				return nil, err
			}
		default:
			node := nodeAt(root, s.path)
			if node == nil {
				return nil, fmt.Errorf("internal error: %s is missing from the document", settingName(s.path))
			}
			var replacement yaml.Node
			if err := replacement.Encode(now); err != nil {
				return nil, fmt.Errorf("cannot encode %s: %w", settingName(s.path), err)
			}
			removed = append(removed, mergeNode(node, &replacement)...)
		}
	}
	return removed, nil
}

func nodeAt(m *yaml.Node, path []string) *yaml.Node {
	for _, key := range path {
		if m == nil || m.Kind != yaml.MappingNode {
			return nil
		}
		m = mappingValue(m, key)
	}
	return m
}

// deleteFromMapping deletes the key at path inside m, then each enclosing
// mapping that the deletion leaves empty, and returns the deleted comments.
func deleteFromMapping(m *yaml.Node, path []string) []string {
	if len(path) == 0 || m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != path[0] {
			continue
		}
		if len(path) > 1 {
			child := m.Content[i+1]
			removed := deleteFromMapping(child, path[1:])
			if child.Kind != yaml.MappingNode || len(child.Content) > 0 {
				return removed
			}
		}
		removed := append(nodeComments(m.Content[i]), nodeComments(m.Content[i+1])...)
		m.Content = slices.Delete(m.Content, i, i+2)
		return removed
	}
	return nil
}

// mergeNode makes dst represent src's value, keeping dst's own comments and
// the nodes, with their comments, of every mapping key and list item that
// remains. It returns the comments of what was removed.
func mergeNode(dst, src *yaml.Node) []string {
	if dst.Kind != src.Kind {
		var removed []string
		for _, n := range dst.Content {
			removed = append(removed, nodeComments(n)...)
		}
		dst.Kind, dst.Tag, dst.Value, dst.Style, dst.Content = src.Kind, src.Tag, src.Value, src.Style, src.Content
		return removed
	}
	switch dst.Kind {
	case yaml.MappingNode:
		var removed []string
		var content []*yaml.Node
		for i := 0; i+1 < len(dst.Content); i += 2 {
			key, value := dst.Content[i], dst.Content[i+1]
			if replacement := mappingValue(src, key.Value); replacement != nil {
				removed = append(removed, mergeNode(value, replacement)...)
				content = append(content, key, value)
			} else {
				removed = append(removed, nodeComments(key)...)
				removed = append(removed, nodeComments(value)...)
			}
		}
		for i := 0; i+1 < len(src.Content); i += 2 {
			if mappingValue(dst, src.Content[i].Value) == nil {
				content = append(content, src.Content[i], src.Content[i+1])
			}
		}
		dst.Content = content
		return removed
	case yaml.SequenceNode:
		var removed []string
		used := make([]bool, len(dst.Content))
		var content []*yaml.Node
		for _, item := range src.Content {
			match := -1
			for j, existing := range dst.Content {
				if !used[j] && itemIdentity(existing) == itemIdentity(item) {
					match = j
					break
				}
			}
			if match < 0 {
				content = append(content, item)
				continue
			}
			used[match] = true
			removed = append(removed, mergeNode(dst.Content[match], item)...)
			content = append(content, dst.Content[match])
		}
		for j, existing := range dst.Content {
			if !used[j] {
				removed = append(removed, nodeComments(existing)...)
			}
		}
		dst.Content = content
		return removed
	default:
		if dst.Value != src.Value || dst.Tag != src.Tag {
			dst.Value, dst.Tag, dst.Style = src.Value, src.Tag, src.Style
		}
		return nil
	}
}

// itemIdentity identifies a list item across a change: a mapping by its name
// or harness, and a scalar by its value.
func itemIdentity(n *yaml.Node) string {
	if n.Kind == yaml.MappingNode {
		for _, key := range []string{"name", "harness"} {
			if v := mappingValue(n, key); v != nil && v.Kind == yaml.ScalarNode {
				return key + "=" + v.Value
			}
		}
		return ""
	}
	if n.Kind == yaml.ScalarNode {
		return "value=" + n.Value
	}
	return ""
}

// nodeComments returns every comment on n and the nodes beneath it.
func nodeComments(n *yaml.Node) []string {
	if n == nil {
		return nil
	}
	var comments []string
	for _, c := range []string{n.HeadComment, n.LineComment, n.FootComment} {
		if c != "" {
			comments = append(comments, c)
		}
	}
	for _, child := range n.Content {
		comments = append(comments, nodeComments(child)...)
	}
	return comments
}

// loadConfigForCommand reads and validates the configuration for show and
// get, reporting problems with setting names.
func loadConfigForCommand(projectDir string) (projectConfig, error) {
	configPath := filepath.Join(projectDir, garageDirName, configFileName)
	data, err := readConfigFile(configPath)
	if err != nil {
		return projectConfig{}, err
	}
	cfg, err := decodeConfig(data)
	if err != nil {
		return projectConfig{}, fmt.Errorf("cannot parse %s: %w", configPath, err)
	}
	if problems := validateConfig(cfg); problems != nil {
		return projectConfig{}, fmt.Errorf("%s is not a valid configuration:\n%s",
			filepath.Join(garageDirName, configFileName), formatProblems(problems))
	}
	return cfg, nil
}

func currentDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine the current directory: %w", err)
	}
	return dir, nil
}

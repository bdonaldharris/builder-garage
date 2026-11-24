package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bdonaldharris/builder-garage/skills"
)

// An installed skill must carry the same provider binding as every other
// skill; it cannot depend on the adopter having this repository's docs tree.
func TestInstalledSkillsCarryProviderOperations(t *testing.T) {
	set, err := loadSkillSet(skills.FS())
	if err != nil {
		t.Fatal(err)
	}
	var binding []byte
	for _, skill := range set {
		entry := findSkillFile(skill.Files, skillEntryFile)
		if entry == nil || !strings.Contains(string(entry.Data), "references/provider-operations.md") {
			t.Fatalf("%s has no provider binding", skill.Name)
		}
		ref := findSkillFile(skill.Files, "references/provider-operations.md")
		if ref == nil {
			t.Fatalf("%s lacks installed operations", skill.Name)
		}
		if binding == nil {
			binding = ref.Data
		} else if !bytes.Equal(binding, ref.Data) {
			t.Fatalf("%s provider contract differs", skill.Name)
		}
		for _, required := range []string{"garage forge note", "#note_ID", "Developer-or-higher", "--mr", "read-back", "separately invoked", "GitHub"} {
			if !strings.Contains(string(ref.Data), required) {
				t.Fatalf("%s lost %s", skill.Name, required)
			}
		}
	}
	// Exercise the actual installer, not only the embedding declaration.
	f := newSkillsFixture(t, "codex")
	f.Env.Skills = skills.FS()
	f.mustInstall(installOptions{})
	for _, skill := range set {
		data, err := os.ReadFile(filepath.Join(f.root("codex"), skill.Name, "references", "provider-operations.md"))
		if err != nil || !bytes.Equal(data, binding) {
			t.Fatalf("installed %s binding differs: %v", skill.Name, err)
		}
	}
	if out, err := f.verify(); err != nil {
		t.Fatalf("installed canonical assets verification failed: %v %s", err, out)
	}
}

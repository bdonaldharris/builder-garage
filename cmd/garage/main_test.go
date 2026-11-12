package main

import "testing"

// Builds that do not inject a version must report the development default.
func TestDefaultVersion(t *testing.T) {
	if version != "dev" {
		t.Fatalf("version = %q, want %q", version, "dev")
	}
}

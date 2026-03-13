package main

import (
	"testing"
)

func TestResolveKeyToPath_AbsolutePath(t *testing.T) {
	path := resolveKeyToPath("/Users/test/code/myproject")
	if path != "/Users/test/code/myproject" {
		t.Errorf("expected path returned as-is, got %q", path)
	}
}

func TestResolveKeyToPath_NonexistentSession(t *testing.T) {
	path := resolveKeyToPath("__nonexistent_session_xyz__")
	if path != "" {
		t.Errorf("expected empty for nonexistent session, got %q", path)
	}
}

func TestResolveKeyToPath_Separator(t *testing.T) {
	path := resolveKeyToPath(separator)
	// separator is not a path and not a session, should return empty
	if path != "" {
		t.Errorf("expected empty for separator, got %q", path)
	}
}

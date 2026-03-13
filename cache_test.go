package main

import (
	"testing"
)

func TestFormatSessionLine_Clean(t *testing.T) {
	r := Repo{Branch: "main", Health: HealthGreen}
	got := formatSessionLine("myproject", r)
	if got != "myproject" {
		t.Errorf("clean session should be just the name, got %q", got)
	}
}

func TestFormatSessionLine_Dirty(t *testing.T) {
	r := Repo{Branch: "feat/auth", Modified: 2, Health: HealthAmber}
	got := formatSessionLine("myproject", r)
	want := "myproject  " + ansiDimAmber + "feat/auth· 2" + ansiReset
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatDimLine_Clean(t *testing.T) {
	r := Repo{Branch: "main", Health: HealthGreen}
	got := formatDimLine("greyout", r)
	want := ansiDim + "greyout" + ansiReset
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatDimLine_Dirty(t *testing.T) {
	r := Repo{Branch: "master", Modified: 3, Untracked: 2, Health: HealthAmber}
	got := formatDimLine("riffkey", r)
	want := ansiDim + "riffkey  master · 5" + ansiReset
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormPath(t *testing.T) {
	got := normPath("/tmp/foo/bar/../baz")
	if got != "/tmp/foo/baz" && got != "/private/tmp/foo/baz" {
		t.Errorf("expected normalized path, got %q", got)
	}
}

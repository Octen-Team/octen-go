package octen

import (
	"regexp"
	"testing"
)

func TestVersion(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(Version) {
		t.Fatalf("Version %q is not in MAJOR.MINOR.PATCH form", Version)
	}
	if Version != "0.4.1" {
		t.Fatalf("Version = %q, want %q", Version, "0.4.1")
	}
}

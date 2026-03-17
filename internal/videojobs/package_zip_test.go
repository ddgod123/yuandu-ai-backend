package videojobs

import "testing"

func TestResolvePackageFormatFromGeneratedSet(t *testing.T) {
	if got := resolvePackageFormatFromGeneratedSet(nil); got != "mixed" {
		t.Fatalf("expected mixed, got %s", got)
	}
	if got := resolvePackageFormatFromGeneratedSet(map[string]struct{}{"gif": {}}); got != "gif" {
		t.Fatalf("expected gif, got %s", got)
	}
	if got := resolvePackageFormatFromGeneratedSet(map[string]struct{}{"gif": {}, "png": {}}); got != "mixed" {
		t.Fatalf("expected mixed, got %s", got)
	}
}

func TestSanitizeZipEntryComponent(t *testing.T) {
	if got := sanitizeZipEntryComponent("a/b:c*demo?"); got != "a-b-c-demo-" {
		t.Fatalf("unexpected sanitize output: %s", got)
	}
	if got := sanitizeZipEntryComponent("   "); got != "" {
		t.Fatalf("expected empty output, got %s", got)
	}
}

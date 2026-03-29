package handlers

import (
	"strings"
	"testing"

	"emoji/internal/videojobs"
)

func TestResolveVideoImageReadTables_FormatSpecific(t *testing.T) {
	tables := resolveVideoImageReadTables("png")
	if tables.Jobs != videojobs.ResolvePublicVideoImageJobsTable("png") {
		t.Fatalf("jobs table mismatch: got=%s", tables.Jobs)
	}
	if tables.Outputs != videojobs.ResolvePublicVideoImageOutputsTable("png") {
		t.Fatalf("outputs table mismatch: got=%s", tables.Outputs)
	}
	if tables.Packages != videojobs.ResolvePublicVideoImagePackagesTable("png") {
		t.Fatalf("packages table mismatch: got=%s", tables.Packages)
	}
	if tables.Events != videojobs.ResolvePublicVideoImageEventsTable("png") {
		t.Fatalf("events table mismatch: got=%s", tables.Events)
	}
	if tables.Feedback != videojobs.ResolvePublicVideoImageFeedbackTable("png") {
		t.Fatalf("feedback table mismatch: got=%s", tables.Feedback)
	}
}

func TestResolveVideoImageReadTables_AllUsesSplitUnion(t *testing.T) {
	tables := resolveVideoImageReadTables("")
	if tables.Jobs == videojobs.PublicVideoImageBaseJobsTable() {
		t.Fatalf("jobs should not fallback to base table in all-format mode")
	}
	if !strings.Contains(tables.Jobs, "UNION ALL") {
		t.Fatalf("jobs should be union expression, got=%s", tables.Jobs)
	}
	if !strings.Contains(tables.Outputs, "UNION ALL") {
		t.Fatalf("outputs should be union expression, got=%s", tables.Outputs)
	}
	for _, table := range videojobs.PublicVideoImageJobsSplitTables() {
		if !strings.Contains(tables.Jobs, table) {
			t.Fatalf("jobs union should include split table %s, got=%s", table, tables.Jobs)
		}
	}
}

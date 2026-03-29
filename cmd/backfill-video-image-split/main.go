package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"

	"emoji/internal/config"
	"emoji/internal/db"
	"emoji/internal/videojobs"

	"github.com/joho/godotenv"
)

func main() {
	apply := flag.Bool("apply", false, "apply writes into public.video_image_*_<format> tables")
	batchSize := flag.Int("batch-size", 500, "batch size per scan")
	format := flag.String("format", "", "optional format filter: gif|png|jpg|webp|live|mp4")
	fallbackFormat := flag.String("fallback-format", "gif", "fallback routed format when no format can be resolved")
	tables := flag.String("tables", "jobs,outputs,packages,events,feedbacks", "tables to backfill: jobs,outputs,packages,events,feedbacks")

	startJobID := flag.Uint64("start-job-id", 0, "start job id (exclusive)")
	startOutputID := flag.Uint64("start-output-id", 0, "start output id (exclusive)")
	startPackageID := flag.Uint64("start-package-id", 0, "start package id (exclusive)")
	startEventID := flag.Uint64("start-event-id", 0, "start event id (exclusive)")
	startFeedbackID := flag.Uint64("start-feedback-id", 0, "start feedback id (exclusive)")

	limitJobs := flag.Int("limit-jobs", 0, "max jobs to scan (0 = no limit)")
	limitOutputs := flag.Int("limit-outputs", 0, "max outputs to scan (0 = no limit)")
	limitPackages := flag.Int("limit-packages", 0, "max packages to scan (0 = no limit)")
	limitEvents := flag.Int("limit-events", 0, "max events to scan (0 = no limit)")
	limitFeedbacks := flag.Int("limit-feedbacks", 0, "max feedback rows to scan (0 = no limit)")
	flag.Parse()

	includeJobs, includeOutputs, includePackages, includeEvents, includeFeedbacks := parseTableFilter(*tables)

	_ = godotenv.Load()
	cfg := config.Load()
	dbConn, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	report, err := videojobs.BackfillPublicVideoImageSplitTables(dbConn, videojobs.PublicVideoImageSplitBackfillOptions{
		Apply:          *apply,
		BatchSize:      *batchSize,
		FormatFilter:   strings.TrimSpace(*format),
		FallbackFormat: strings.TrimSpace(*fallbackFormat),

		StartJobID:      *startJobID,
		StartOutputID:   *startOutputID,
		StartPackageID:  *startPackageID,
		StartEventID:    *startEventID,
		StartFeedbackID: *startFeedbackID,

		LimitJobs:      *limitJobs,
		LimitOutputs:   *limitOutputs,
		LimitPackages:  *limitPackages,
		LimitEvents:    *limitEvents,
		LimitFeedbacks: *limitFeedbacks,

		IncludeJobs:      includeJobs,
		IncludeOutputs:   includeOutputs,
		IncludePackages:  includePackages,
		IncludeEvents:    includeEvents,
		IncludeFeedbacks: includeFeedbacks,
	})
	if err != nil {
		log.Fatalf("backfill failed: %v", err)
	}

	payload, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(payload))
	if report.FailedTotal() > 0 {
		log.Printf("completed with failures: %d", report.FailedTotal())
	}
}

func parseTableFilter(raw string) (jobs, outputs, packages, events, feedbacks bool) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || raw == "all" || raw == "*" {
		return true, true, true, true, true
	}
	for _, item := range strings.Split(raw, ",") {
		switch strings.TrimSpace(strings.ToLower(item)) {
		case "job", "jobs":
			jobs = true
		case "output", "outputs":
			outputs = true
		case "package", "packages":
			packages = true
		case "event", "events":
			events = true
		case "feedback", "feedbacks":
			feedbacks = true
		}
	}
	return
}

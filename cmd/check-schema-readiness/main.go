package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"emoji/internal/config"
	"emoji/internal/db"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

type tableCheck struct {
	Schema string
	Table  string
	Reason string
}

type columnCheck struct {
	Schema string
	Table  string
	Column string
	Reason string
}

func main() {
	profile := flag.String("profile", "png-mainline", "readiness profile, e.g. png-mainline")
	strict := flag.Bool("strict", false, "exit non-zero when any check fails")
	flag.Parse()

	loadEnv()
	cfg := config.Load()
	dbConn, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	tables, columns, migrationHints := resolveProfile(strings.ToLower(strings.TrimSpace(*profile)))
	if len(tables) == 0 && len(columns) == 0 {
		log.Fatalf("unknown profile: %s", strings.TrimSpace(*profile))
	}

	failed := 0
	fmt.Printf("schema readiness profile: %s\n", strings.TrimSpace(*profile))
	fmt.Println("----- table checks -----")
	for _, check := range tables {
		ok, err := tableExists(dbConn, check.Schema, check.Table)
		if err != nil {
			failed++
			fmt.Printf("[FAIL] %s.%s (query error: %v)\n", check.Schema, check.Table, err)
			continue
		}
		if !ok {
			failed++
			fmt.Printf("[FAIL] %s.%s  | %s\n", check.Schema, check.Table, check.Reason)
			continue
		}
		fmt.Printf("[ OK ] %s.%s\n", check.Schema, check.Table)
	}

	fmt.Println("----- column checks -----")
	for _, check := range columns {
		ok, err := columnExists(dbConn, check.Schema, check.Table, check.Column)
		if err != nil {
			failed++
			fmt.Printf("[FAIL] %s.%s.%s (query error: %v)\n", check.Schema, check.Table, check.Column, err)
			continue
		}
		if !ok {
			failed++
			fmt.Printf("[FAIL] %s.%s.%s  | %s\n", check.Schema, check.Table, check.Column, check.Reason)
			continue
		}
		fmt.Printf("[ OK ] %s.%s.%s\n", check.Schema, check.Table, check.Column)
	}

	fmt.Println("----- summary -----")
	fmt.Printf("failed=%d\n", failed)
	if failed > 0 {
		fmt.Println("migration hints:")
		for _, hint := range migrationHints {
			fmt.Printf("  - %s\n", hint)
		}
		if *strict {
			os.Exit(2)
		}
		return
	}
	fmt.Println("all required schema checks passed.")
}

func resolveProfile(profile string) (tables []tableCheck, columns []columnCheck, migrationHints []string) {
	switch profile {
	case "png-mainline":
		tables = []tableCheck{
			{Schema: "public", Table: "video_image_jobs_png", Reason: "split read table for PNG jobs"},
			{Schema: "public", Table: "video_image_outputs_png", Reason: "split read table for PNG outputs"},
			{Schema: "public", Table: "video_image_packages_png", Reason: "split read table for PNG packages"},
			{Schema: "public", Table: "video_image_events_png", Reason: "split read table for PNG events"},
			{Schema: "public", Table: "video_image_feedback_png", Reason: "split read table for PNG feedback"},
			{Schema: "public", Table: "video_work_cards", Reason: "my works card read model"},
			{Schema: "ops", Table: "compute_redeem_codes", Reason: "compute redeem code table"},
			{Schema: "ops", Table: "compute_redeem_redemptions", Reason: "compute redeem redemption ledger"},
		}
		columns = []columnCheck{
			{Schema: "ops", Table: "compute_redeem_redemptions", Column: "clear_status", Reason: "expiry cleanup status from migration 087"},
			{Schema: "ops", Table: "compute_redeem_redemptions", Column: "cleared_points", Reason: "expiry cleanup points from migration 087"},
			{Schema: "ops", Table: "compute_redeem_redemptions", Column: "cleared_at", Reason: "expiry cleanup timestamp from migration 087"},
		}
		migrationHints = []string{
			"migrations/085_video_image_split_tables_and_work_cards.sql",
			"migrations/086_compute_redeem_codes.sql",
			"migrations/087_compute_redeem_expire_clear.sql",
		}
		return tables, columns, migrationHints
	default:
		return nil, nil, nil
	}
}

func tableExists(dbConn *gorm.DB, schema, table string) (bool, error) {
	var exists bool
	fq := fmt.Sprintf("%s.%s", strings.TrimSpace(schema), strings.TrimSpace(table))
	err := dbConn.Raw("SELECT to_regclass(?) IS NOT NULL", fq).Scan(&exists).Error
	return exists, err
}

func columnExists(dbConn *gorm.DB, schema, table, column string) (bool, error) {
	var exists bool
	err := dbConn.Raw(`
SELECT EXISTS (
	SELECT 1
	FROM information_schema.columns
	WHERE table_schema = ?
	  AND table_name = ?
	  AND column_name = ?
)`, strings.TrimSpace(schema), strings.TrimSpace(table), strings.TrimSpace(column)).
		Scan(&exists).Error
	return exists, err
}

func loadEnv() {
	paths := []string{".env", "../.env", "../../.env"}
	for _, p := range paths {
		_, _ = os.Stat(p)
		_ = godotenv.Load(p)
	}
	if cwd, err := os.Getwd(); err == nil {
		for i := 0; i < 5; i++ {
			path := filepath.Join(cwd, strings.Repeat("../", i), ".env")
			_ = godotenv.Load(path)
		}
	}
}

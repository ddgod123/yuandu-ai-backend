package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/config"
	"emoji/internal/db"
	"emoji/internal/models"
	"emoji/internal/videojobs"

	"github.com/joho/godotenv"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	pointPerCNY          = 100.0
	costMarkupMultiplier = 2.0
)

type auditRow struct {
	JobID              uint64
	UserID             uint64
	Status             string
	OutputFormats      string
	CreatedAt          time.Time
	HoldStatus         string
	ReservedPoints     int64
	SettledPoints      int64
	ExpectedPoints     int64
	Matched            bool
	BillableCostCNY    float64
	BillableCostSource string
	AICostCNY          float64
	EstimatedCost      float64
	PricingVersion     string
}

func main() {
	jobIDsArg := flag.String("job-ids", "", "comma separated job ids, e.g. 215,216")
	userID := flag.Uint64("user-id", 0, "filter by user id")
	format := flag.String("format", "png", "filter by output format when --job-ids is empty")
	limit := flag.Int("limit", 20, "max rows when --job-ids is empty")
	showAll := flag.Bool("show-all", false, "show all rows, default only mismatched/non-settled rows")
	repair := flag.Bool("repair", false, "attempt to repair mismatched/missing point settlements before printing")
	strict := flag.Bool("strict", false, "exit non-zero when mismatch exists")
	flag.Parse()

	loadEnv()
	cfg := config.Load()
	dbConn, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	jobIDs := parseJobIDs(*jobIDsArg)
	jobs, err := loadJobs(dbConn, jobIDs, *userID, strings.ToLower(strings.TrimSpace(*format)), *limit)
	if err != nil {
		log.Fatalf("load jobs failed: %v", err)
	}
	if len(jobs) == 0 {
		fmt.Println("no jobs matched")
		return
	}

	costMap, err := loadCostMap(dbConn, jobs)
	if err != nil {
		log.Fatalf("load costs failed: %v", err)
	}
	holdMap, err := loadHoldMap(dbConn, jobs)
	if err != nil {
		log.Fatalf("load point holds failed: %v", err)
	}

	rows := buildAuditRows(jobs, costMap, holdMap)
	if *repair {
		attempted, failed := repairPointClosure(dbConn, rows)
		if attempted > 0 || failed > 0 {
			fmt.Printf("repair: attempted=%d failed=%d\n", attempted, failed)
		}
		costMap, err = loadCostMap(dbConn, jobs)
		if err != nil {
			log.Fatalf("reload costs failed: %v", err)
		}
		holdMap, err = loadHoldMap(dbConn, jobs)
		if err != nil {
			log.Fatalf("reload point holds failed: %v", err)
		}
		rows = buildAuditRows(jobs, costMap, holdMap)
	}

	mismatchCount := 0
	printRows := make([]auditRow, 0, len(rows))
	for _, row := range rows {
		if !row.Matched {
			mismatchCount++
		}
		if *showAll || !row.Matched || !strings.EqualFold(strings.TrimSpace(row.HoldStatus), "settled") {
			printRows = append(printRows, row)
		}
	}

	if len(printRows) == 0 {
		printRows = rows
	}
	printAuditTable(printRows)
	fmt.Printf("\nsummary: total=%d mismatched=%d matched=%d\n", len(rows), mismatchCount, len(rows)-mismatchCount)
	if *strict && mismatchCount > 0 {
		os.Exit(2)
	}
}

func repairPointClosure(dbConn *gorm.DB, rows []auditRow) (attempted int, failed int) {
	if dbConn == nil || len(rows) == 0 {
		return 0, 0
	}
	for _, row := range rows {
		if row.Matched {
			continue
		}
		attempted++
		if err := videojobs.UpsertJobCost(dbConn, row.JobID); err != nil {
			failed++
			continue
		}
		if err := videojobs.SettleReservedPointsForJob(dbConn, row.JobID, row.Status); err != nil {
			failed++
			continue
		}
	}
	return attempted, failed
}

func parseJobIDs(raw string) []uint64 {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	out := make([]uint64, 0, len(parts))
	seen := map[uint64]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseUint(part, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func loadJobs(dbConn *gorm.DB, jobIDs []uint64, userID uint64, format string, limit int) ([]models.VideoJob, error) {
	query := dbConn.Model(&models.VideoJob{})
	if len(jobIDs) > 0 {
		query = query.Where("id IN ?", jobIDs)
	} else {
		if userID > 0 {
			query = query.Where("user_id = ?", userID)
		}
		format = strings.TrimSpace(format)
		if format != "" && format != "all" {
			query = query.Where("LOWER(output_formats) LIKE ?", "%"+strings.ToLower(format)+"%")
		}
		if limit <= 0 {
			limit = 20
		}
		if limit > 200 {
			limit = 200
		}
		query = query.Order("id DESC").Limit(limit)
	}

	var jobs []models.VideoJob
	if err := query.Find(&jobs).Error; err != nil {
		return nil, err
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID > jobs[j].ID })
	return jobs, nil
}

func loadCostMap(dbConn *gorm.DB, jobs []models.VideoJob) (map[uint64]models.VideoJobCost, error) {
	out := map[uint64]models.VideoJobCost{}
	ids := make([]uint64, 0, len(jobs))
	for _, job := range jobs {
		if job.ID > 0 {
			ids = append(ids, job.ID)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}

	var rows []models.VideoJobCost
	if err := dbConn.Where("job_id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.JobID] = row
	}
	return out, nil
}

func loadHoldMap(dbConn *gorm.DB, jobs []models.VideoJob) (map[uint64]models.ComputePointHold, error) {
	out := map[uint64]models.ComputePointHold{}
	ids := make([]uint64, 0, len(jobs))
	for _, job := range jobs {
		if job.ID > 0 {
			ids = append(ids, job.ID)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}

	var rows []models.ComputePointHold
	if err := dbConn.Where("job_id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.JobID] = row
	}
	return out, nil
}

func buildAuditRows(
	jobs []models.VideoJob,
	costMap map[uint64]models.VideoJobCost,
	holdMap map[uint64]models.ComputePointHold,
) []auditRow {
	out := make([]auditRow, 0, len(jobs))
	for _, job := range jobs {
		cost := costMap[job.ID]
		hold := holdMap[job.ID]
		billableCost, source, aiCost := resolveBillableCost(cost)
		expected := computeExpectedPoints(strings.ToLower(strings.TrimSpace(job.Status)), billableCost, hold.ReservedPoints)
		row := auditRow{
			JobID:              job.ID,
			UserID:             job.UserID,
			Status:             strings.ToLower(strings.TrimSpace(job.Status)),
			OutputFormats:      strings.TrimSpace(job.OutputFormats),
			CreatedAt:          job.CreatedAt,
			HoldStatus:         strings.TrimSpace(hold.Status),
			ReservedPoints:     hold.ReservedPoints,
			SettledPoints:      hold.SettledPoints,
			ExpectedPoints:     expected,
			BillableCostCNY:    billableCost,
			BillableCostSource: source,
			AICostCNY:          aiCost,
			EstimatedCost:      cost.EstimatedCost,
			PricingVersion:     strings.TrimSpace(cost.PricingVersion),
		}
		if strings.EqualFold(strings.TrimSpace(hold.Status), "settled") {
			row.Matched = row.SettledPoints == row.ExpectedPoints
		} else {
			row.Matched = false
		}
		out = append(out, row)
	}
	return out
}

func parseJSONMap(raw datatypes.JSON) map[string]interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func parseFloat(value interface{}) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case uint64:
		return float64(v)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func resolveBillableCost(cost models.VideoJobCost) (billableCostCNY float64, source string, aiCostCNY float64) {
	details := parseJSONMap(cost.Details)
	aiCostCNY = parseFloat(details["ai_cost_cny"])
	if aiCostCNY > 0 {
		return aiCostCNY, "ai_cost_cny", aiCostCNY
	}
	if strings.EqualFold(strings.TrimSpace(cost.Currency), "cny") && cost.EstimatedCost > 0 {
		return cost.EstimatedCost, "estimated_cost", aiCostCNY
	}
	return 0, "none", aiCostCNY
}

func pointsFromCost(cost float64) int64 {
	if cost <= 0 {
		return 0
	}
	return int64(math.Ceil(cost * pointPerCNY * costMarkupMultiplier))
}

func computeExpectedPoints(finalStatus string, billableCostCNY float64, reserved int64) int64 {
	switch finalStatus {
	case models.VideoJobStatusCancelled:
		return 0
	case models.VideoJobStatusFailed:
		if reserved <= 0 {
			return 0
		}
		return 1
	default:
		points := pointsFromCost(billableCostCNY)
		if points <= 0 {
			if reserved > 0 {
				return 1
			}
			return 0
		}
		return points
	}
}

func printAuditTable(rows []auditRow) {
	fmt.Printf("%-8s %-8s %-10s %-8s %-8s %-8s %-8s %-12s %-11s %-11s %-19s\n",
		"job_id", "status", "hold_status", "reserve", "settled", "expect", "match", "source", "billable", "ai_cost", "created_at")
	for _, row := range rows {
		match := "NO"
		if row.Matched {
			match = "YES"
		}
		fmt.Printf("%-8d %-8s %-10s %-8d %-8d %-8d %-8s %-12s %-11.4f %-11.4f %-19s\n",
			row.JobID,
			row.Status,
			row.HoldStatus,
			row.ReservedPoints,
			row.SettledPoints,
			row.ExpectedPoints,
			match,
			row.BillableCostSource,
			row.BillableCostCNY,
			row.AICostCNY,
			row.CreatedAt.Format("2006-01-02 15:04:05"),
		)
	}
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

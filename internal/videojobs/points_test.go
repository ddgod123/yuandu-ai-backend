package videojobs

import (
	"fmt"
	"strings"
	"testing"

	"emoji/internal/models"

	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResolveBillableCostCNYPrefersAICost(t *testing.T) {
	cost := models.VideoJobCost{
		EstimatedCost: 0.42,
		Currency:      "CNY",
		Details:       datatypes.JSON([]byte(`{"ai_cost_cny":0.1234}`)),
	}
	billable, source, aiCost := resolveBillableCostCNY(cost)
	if source != "ai_cost_cny" {
		t.Fatalf("expected source ai_cost_cny, got %q", source)
	}
	if billable != 0.1234 {
		t.Fatalf("expected billable 0.1234, got %v", billable)
	}
	if aiCost != 0.1234 {
		t.Fatalf("expected aiCost 0.1234, got %v", aiCost)
	}
}

func TestResolveBillableCostCNYFallbackEstimatedCost(t *testing.T) {
	cost := models.VideoJobCost{
		EstimatedCost: 0.56,
		Currency:      "CNY",
		Details:       datatypes.JSON([]byte(`{"ai_cost_cny":0}`)),
	}
	billable, source, _ := resolveBillableCostCNY(cost)
	if source != "estimated_cost" {
		t.Fatalf("expected source estimated_cost, got %q", source)
	}
	if billable != 0.56 {
		t.Fatalf("expected billable 0.56, got %v", billable)
	}
}

func TestComputeActualPointsFromCostDone(t *testing.T) {
	points := computeActualPointsFromCost(models.VideoJobStatusDone, 0.1, 80)
	if points != 20 {
		t.Fatalf("expected 20 points, got %d", points)
	}
}

func TestComputeActualPointsFromCostFailedAndCancelled(t *testing.T) {
	failed := computeActualPointsFromCost(models.VideoJobStatusFailed, 10, 50)
	if failed != 1 {
		t.Fatalf("expected failed settle points=1, got %d", failed)
	}
	cancelled := computeActualPointsFromCost(models.VideoJobStatusCancelled, 10, 50)
	if cancelled != 0 {
		t.Fatalf("expected cancelled settle points=0, got %d", cancelled)
	}
}

func TestSettleReservedPointsForJobPrefersAICostAndIsIdempotent(t *testing.T) {
	db := openPointsSettlementTestDB(t)

	account := models.ComputeAccount{
		UserID:               9001,
		AvailablePoints:      900,
		FrozenPoints:         100,
		DebtPoints:           0,
		TotalConsumedPoints:  0,
		TotalRechargedPoints: 1000,
		Status:               "active",
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}

	hold := models.ComputePointHold{
		JobID:          701,
		UserID:         9001,
		AccountID:      account.ID,
		ReservedPoints: 100,
		SettledPoints:  0,
		Status:         "held",
		Remark:         "reserve",
	}
	if err := db.Create(&hold).Error; err != nil {
		t.Fatalf("create hold: %v", err)
	}

	cost := models.VideoJobCost{
		JobID:          701,
		UserID:         9001,
		Status:         models.VideoJobStatusDone,
		EstimatedCost:  0.8,
		Currency:       "CNY",
		PricingVersion: "v1",
		Details:        datatypes.JSON([]byte(`{"ai_cost_cny":0.1234}`)),
	}
	if err := db.Create(&cost).Error; err != nil {
		t.Fatalf("create cost: %v", err)
	}

	if err := SettleReservedPointsForJob(db, 701, models.VideoJobStatusDone); err != nil {
		t.Fatalf("first settle failed: %v", err)
	}
	if err := SettleReservedPointsForJob(db, 701, models.VideoJobStatusDone); err != nil {
		t.Fatalf("second settle failed: %v", err)
	}

	var updatedAccount models.ComputeAccount
	if err := db.Where("user_id = ?", 9001).First(&updatedAccount).Error; err != nil {
		t.Fatalf("load account: %v", err)
	}
	if updatedAccount.AvailablePoints != 975 {
		t.Fatalf("expected available_points=975, got %d", updatedAccount.AvailablePoints)
	}
	if updatedAccount.FrozenPoints != 0 {
		t.Fatalf("expected frozen_points=0, got %d", updatedAccount.FrozenPoints)
	}
	if updatedAccount.TotalConsumedPoints != 25 {
		t.Fatalf("expected total_consumed_points=25, got %d", updatedAccount.TotalConsumedPoints)
	}

	var updatedHold models.ComputePointHold
	if err := db.Where("job_id = ?", 701).First(&updatedHold).Error; err != nil {
		t.Fatalf("load hold: %v", err)
	}
	if updatedHold.Status != "settled" {
		t.Fatalf("expected hold status settled, got %s", updatedHold.Status)
	}
	if updatedHold.SettledPoints != 25 {
		t.Fatalf("expected hold settled_points=25, got %d", updatedHold.SettledPoints)
	}

	var ledgers []models.ComputeLedger
	if err := db.Where("job_id = ? AND type = ?", 701, "settle").Find(&ledgers).Error; err != nil {
		t.Fatalf("load ledgers: %v", err)
	}
	if len(ledgers) != 1 {
		t.Fatalf("expected one settle ledger, got %d", len(ledgers))
	}
	if ledgers[0].Points != 25 {
		t.Fatalf("expected settle points=25, got %d", ledgers[0].Points)
	}
	meta := parseJSONMap(ledgers[0].Metadata)
	if source := strings.TrimSpace(fmt.Sprint(meta["billable_cost_source"])); source != "ai_cost_cny" {
		t.Fatalf("expected billable_cost_source=ai_cost_cny, got %q", source)
	}
	if billable := parseOptionFloat(meta, "billable_cost_cny"); billable != 0.1234 {
		t.Fatalf("expected billable_cost_cny=0.1234, got %v", billable)
	}
}

func TestSettleReservedPointsForJobFallbackEstimatedCost(t *testing.T) {
	db := openPointsSettlementTestDB(t)

	account := models.ComputeAccount{
		UserID:               9002,
		AvailablePoints:      950,
		FrozenPoints:         50,
		DebtPoints:           0,
		TotalConsumedPoints:  0,
		TotalRechargedPoints: 1000,
		Status:               "active",
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}

	hold := models.ComputePointHold{
		JobID:          702,
		UserID:         9002,
		AccountID:      account.ID,
		ReservedPoints: 50,
		SettledPoints:  0,
		Status:         "held",
		Remark:         "reserve",
	}
	if err := db.Create(&hold).Error; err != nil {
		t.Fatalf("create hold: %v", err)
	}

	cost := models.VideoJobCost{
		JobID:          702,
		UserID:         9002,
		Status:         models.VideoJobStatusDone,
		EstimatedCost:  0.056,
		Currency:       "CNY",
		PricingVersion: "v1",
		Details:        datatypes.JSON([]byte(`{"ai_cost_cny":0}`)),
	}
	if err := db.Create(&cost).Error; err != nil {
		t.Fatalf("create cost: %v", err)
	}

	if err := SettleReservedPointsForJob(db, 702, models.VideoJobStatusDone); err != nil {
		t.Fatalf("settle failed: %v", err)
	}

	var updatedHold models.ComputePointHold
	if err := db.Where("job_id = ?", 702).First(&updatedHold).Error; err != nil {
		t.Fatalf("load hold: %v", err)
	}
	// ceil(0.056 * 100 * 2) = 12
	if updatedHold.SettledPoints != 12 {
		t.Fatalf("expected settled_points=12, got %d", updatedHold.SettledPoints)
	}

	var ledger models.ComputeLedger
	if err := db.Where("job_id = ? AND type = ?", 702, "settle").First(&ledger).Error; err != nil {
		t.Fatalf("load settle ledger: %v", err)
	}
	meta := parseJSONMap(ledger.Metadata)
	if source := strings.TrimSpace(fmt.Sprint(meta["billable_cost_source"])); source != "estimated_cost" {
		t.Fatalf("expected billable_cost_source=estimated_cost, got %q", source)
	}
}

func TestSettleReservedPointsForJobWithoutHoldCreatesSyntheticSettlement(t *testing.T) {
	db := openPointsSettlementTestDB(t)

	account := models.ComputeAccount{
		UserID:               9003,
		AvailablePoints:      1000,
		FrozenPoints:         0,
		DebtPoints:           0,
		TotalConsumedPoints:  0,
		TotalRechargedPoints: 1000,
		Status:               "active",
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}

	cost := models.VideoJobCost{
		JobID:          703,
		UserID:         9003,
		Status:         models.VideoJobStatusDone,
		EstimatedCost:  0.012,
		Currency:       "CNY",
		PricingVersion: "v1",
		Details:        datatypes.JSON([]byte(`{"ai_cost_cny":0.025}`)),
	}
	if err := db.Create(&cost).Error; err != nil {
		t.Fatalf("create cost: %v", err)
	}

	if err := SettleReservedPointsForJob(db, 703, models.VideoJobStatusDone); err != nil {
		t.Fatalf("settle failed: %v", err)
	}

	var updatedHold models.ComputePointHold
	if err := db.Where("job_id = ?", 703).First(&updatedHold).Error; err != nil {
		t.Fatalf("load hold: %v", err)
	}
	if updatedHold.Status != "settled" {
		t.Fatalf("expected hold status settled, got %s", updatedHold.Status)
	}
	if updatedHold.ReservedPoints != 0 {
		t.Fatalf("expected reserved_points=0, got %d", updatedHold.ReservedPoints)
	}
	if updatedHold.SettledPoints != 5 {
		t.Fatalf("expected settled_points=5, got %d", updatedHold.SettledPoints)
	}

	var updatedAccount models.ComputeAccount
	if err := db.Where("user_id = ?", 9003).First(&updatedAccount).Error; err != nil {
		t.Fatalf("load account: %v", err)
	}
	if updatedAccount.AvailablePoints != 995 {
		t.Fatalf("expected available_points=995, got %d", updatedAccount.AvailablePoints)
	}
	if updatedAccount.TotalConsumedPoints != 5 {
		t.Fatalf("expected total_consumed_points=5, got %d", updatedAccount.TotalConsumedPoints)
	}

	var ledger models.ComputeLedger
	if err := db.Where("job_id = ? AND type = ?", 703, "settle").First(&ledger).Error; err != nil {
		t.Fatalf("load settle ledger: %v", err)
	}
	if ledger.Points != 5 {
		t.Fatalf("expected settle points=5, got %d", ledger.Points)
	}
	meta := parseJSONMap(ledger.Metadata)
	if status := strings.TrimSpace(fmt.Sprint(meta["hold_status"])); status != "missing_auto_settled" {
		t.Fatalf("expected hold_status=missing_auto_settled, got %q", status)
	}
}

func TestSettleReservedPointsForJobReconcilesSettledDelta(t *testing.T) {
	db := openPointsSettlementTestDB(t)

	account := models.ComputeAccount{
		UserID:               9004,
		AvailablePoints:      998,
		FrozenPoints:         0,
		DebtPoints:           0,
		TotalConsumedPoints:  2,
		TotalRechargedPoints: 1000,
		Status:               "active",
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}

	hold := models.ComputePointHold{
		JobID:          704,
		UserID:         9004,
		AccountID:      account.ID,
		ReservedPoints: 0,
		SettledPoints:  2,
		Status:         "settled",
		Remark:         "done",
	}
	if err := db.Create(&hold).Error; err != nil {
		t.Fatalf("create hold: %v", err)
	}

	cost := models.VideoJobCost{
		JobID:          704,
		UserID:         9004,
		Status:         models.VideoJobStatusDone,
		EstimatedCost:  0.02,
		Currency:       "CNY",
		PricingVersion: "v1",
		Details:        datatypes.JSON([]byte(`{"ai_cost_cny":0.0025}`)),
	}
	if err := db.Create(&cost).Error; err != nil {
		t.Fatalf("create cost: %v", err)
	}

	if err := SettleReservedPointsForJob(db, 704, models.VideoJobStatusDone); err != nil {
		t.Fatalf("settle reconcile failed: %v", err)
	}
	if err := SettleReservedPointsForJob(db, 704, models.VideoJobStatusDone); err != nil {
		t.Fatalf("settle reconcile retry failed: %v", err)
	}

	var updatedHold models.ComputePointHold
	if err := db.Where("job_id = ?", 704).First(&updatedHold).Error; err != nil {
		t.Fatalf("load hold: %v", err)
	}
	if updatedHold.SettledPoints != 1 {
		t.Fatalf("expected settled_points=1, got %d", updatedHold.SettledPoints)
	}

	var updatedAccount models.ComputeAccount
	if err := db.Where("user_id = ?", 9004).First(&updatedAccount).Error; err != nil {
		t.Fatalf("load account: %v", err)
	}
	if updatedAccount.AvailablePoints != 999 {
		t.Fatalf("expected available_points=999, got %d", updatedAccount.AvailablePoints)
	}
	if updatedAccount.TotalConsumedPoints != 1 {
		t.Fatalf("expected total_consumed_points=1, got %d", updatedAccount.TotalConsumedPoints)
	}

	var ledgers []models.ComputeLedger
	if err := db.Where("job_id = ? AND type = ?", 704, "settle_adjust").Find(&ledgers).Error; err != nil {
		t.Fatalf("load settle_adjust ledgers: %v", err)
	}
	if len(ledgers) != 1 {
		t.Fatalf("expected one settle_adjust ledger, got %d", len(ledgers))
	}
	if ledgers[0].Points != -1 {
		t.Fatalf("expected settle_adjust points=-1, got %d", ledgers[0].Points)
	}
	meta := parseJSONMap(ledgers[0].Metadata)
	if prev := int64(parseOptionFloat(meta, "previous_settled_points")); prev != 2 {
		t.Fatalf("expected previous_settled_points=2, got %d", prev)
	}
}

func openPointsSettlementTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.Exec(`ATTACH DATABASE ':memory:' AS ops`).Error; err != nil {
		t.Fatalf("attach ops schema: %v", err)
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS ops.compute_accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL UNIQUE,
			available_points INTEGER NOT NULL DEFAULT 0,
			frozen_points INTEGER NOT NULL DEFAULT 0,
			debt_points INTEGER NOT NULL DEFAULT 0,
			total_consumed_points INTEGER NOT NULL DEFAULT 0,
			total_recharged_points INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS ops.compute_ledgers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			job_id INTEGER,
			type TEXT NOT NULL DEFAULT '',
			points INTEGER NOT NULL DEFAULT 0,
			available_before INTEGER NOT NULL DEFAULT 0,
			available_after INTEGER NOT NULL DEFAULT 0,
			frozen_before INTEGER NOT NULL DEFAULT 0,
			frozen_after INTEGER NOT NULL DEFAULT 0,
			debt_before INTEGER NOT NULL DEFAULT 0,
			debt_after INTEGER NOT NULL DEFAULT 0,
			remark TEXT NOT NULL DEFAULT '',
			metadata JSON NOT NULL DEFAULT '{}',
			created_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS ops.compute_point_holds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id INTEGER NOT NULL UNIQUE,
			user_id INTEGER NOT NULL,
			account_id INTEGER NOT NULL,
			reserved_points INTEGER NOT NULL DEFAULT 0,
			settled_points INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'held',
			remark TEXT NOT NULL DEFAULT '',
			settled_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS ops.video_job_costs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id INTEGER NOT NULL UNIQUE,
			user_id INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT '',
			cpu_ms INTEGER NOT NULL DEFAULT 0,
			gpu_ms INTEGER NOT NULL DEFAULT 0,
			asr_seconds REAL NOT NULL DEFAULT 0,
			ocr_frames INTEGER NOT NULL DEFAULT 0,
			storage_bytes_raw INTEGER NOT NULL DEFAULT 0,
			storage_bytes_output INTEGER NOT NULL DEFAULT 0,
			output_count INTEGER NOT NULL DEFAULT 0,
			estimated_cost REAL NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'CNY',
			pricing_version TEXT NOT NULL DEFAULT '',
			details JSON NOT NULL DEFAULT '{}',
			created_at DATETIME,
			updated_at DATETIME
		)`,
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create table for test: %v", err)
		}
	}
	return db
}

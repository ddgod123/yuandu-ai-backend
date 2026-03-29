package videojobs

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"emoji/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSweepExpiredComputeRedeemPoints(t *testing.T) {
	db := openComputeRedeemExpiryTestDB(t)

	now := time.Now()
	expiredAt := now.Add(-2 * time.Hour)

	account := models.ComputeAccount{
		UserID:               1001,
		AvailablePoints:      150,
		FrozenPoints:         0,
		DebtPoints:           0,
		TotalConsumedPoints:  0,
		TotalRechargedPoints: 150,
		Status:               "active",
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create compute account: %v", err)
	}

	redemption := models.ComputeRedeemRedemption{
		CodeID:           2001,
		UserID:           1001,
		GrantedPoints:    100,
		GrantedStartsAt:  now.Add(-24 * time.Hour),
		GrantedExpiresAt: &expiredAt,
		ClearStatus:      computeRedeemClearStatusPending,
	}
	if err := db.Create(&redemption).Error; err != nil {
		t.Fatalf("create compute redemption: %v", err)
	}

	result, err := SweepExpiredComputeRedeemPoints(db, now, 10)
	if err != nil {
		t.Fatalf("sweep expired compute redeems: %v", err)
	}
	if result.Cleared != 1 {
		t.Fatalf("expected cleared=1, got %d", result.Cleared)
	}
	if result.TotalClearedPoint != 100 {
		t.Fatalf("expected total_cleared_points=100, got %d", result.TotalClearedPoint)
	}

	var updatedAccount models.ComputeAccount
	if err := db.Where("user_id = ?", 1001).First(&updatedAccount).Error; err != nil {
		t.Fatalf("load updated account: %v", err)
	}
	if updatedAccount.AvailablePoints != 50 {
		t.Fatalf("expected available_points=50, got %d", updatedAccount.AvailablePoints)
	}

	var updatedRedemption models.ComputeRedeemRedemption
	if err := db.First(&updatedRedemption, redemption.ID).Error; err != nil {
		t.Fatalf("load updated redemption: %v", err)
	}
	if updatedRedemption.ClearStatus != computeRedeemClearStatusCleared {
		t.Fatalf("expected clear_status=cleared, got %s", updatedRedemption.ClearStatus)
	}
	if updatedRedemption.ClearedPoints != 100 {
		t.Fatalf("expected cleared_points=100, got %d", updatedRedemption.ClearedPoints)
	}
	if updatedRedemption.ClearedAt == nil {
		t.Fatal("expected cleared_at to be set")
	}

	var ledgerCount int64
	if err := db.Model(&models.ComputeLedger{}).
		Where("user_id = ? AND type = ? AND points = ?", 1001, "adjust", -100).
		Count(&ledgerCount).Error; err != nil {
		t.Fatalf("count compute ledger rows: %v", err)
	}
	if ledgerCount != 1 {
		t.Fatalf("expected one adjust ledger row, got %d", ledgerCount)
	}
}

func TestSweepExpiredComputeRedeemPointsSkipsNotExpired(t *testing.T) {
	db := openComputeRedeemExpiryTestDB(t)

	now := time.Now()
	expireLater := now.Add(2 * time.Hour)

	account := models.ComputeAccount{
		UserID:               1002,
		AvailablePoints:      150,
		FrozenPoints:         0,
		DebtPoints:           0,
		TotalConsumedPoints:  0,
		TotalRechargedPoints: 150,
		Status:               "active",
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create compute account: %v", err)
	}

	redemption := models.ComputeRedeemRedemption{
		CodeID:           2002,
		UserID:           1002,
		GrantedPoints:    100,
		GrantedStartsAt:  now.Add(-1 * time.Hour),
		GrantedExpiresAt: &expireLater,
		ClearStatus:      computeRedeemClearStatusPending,
	}
	if err := db.Create(&redemption).Error; err != nil {
		t.Fatalf("create compute redemption: %v", err)
	}

	result, err := SweepExpiredComputeRedeemPoints(db, now, 10)
	if err != nil {
		t.Fatalf("sweep expired compute redeems: %v", err)
	}
	if result.Cleared != 0 {
		t.Fatalf("expected cleared=0, got %d", result.Cleared)
	}
	if result.Scanned != 0 {
		t.Fatalf("expected scanned=0, got %d", result.Scanned)
	}
}

func TestSweepExpiredComputeRedeemPointsIsIdempotent(t *testing.T) {
	db := openComputeRedeemExpiryTestDB(t)

	now := time.Now()
	expiredAt := now.Add(-2 * time.Hour)

	account := models.ComputeAccount{
		UserID:               1003,
		AvailablePoints:      200,
		FrozenPoints:         0,
		DebtPoints:           0,
		TotalConsumedPoints:  0,
		TotalRechargedPoints: 200,
		Status:               "active",
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create compute account: %v", err)
	}

	redemption := models.ComputeRedeemRedemption{
		CodeID:           2003,
		UserID:           1003,
		GrantedPoints:    30,
		GrantedStartsAt:  now.Add(-24 * time.Hour),
		GrantedExpiresAt: &expiredAt,
		ClearStatus:      computeRedeemClearStatusPending,
	}
	if err := db.Create(&redemption).Error; err != nil {
		t.Fatalf("create compute redemption: %v", err)
	}

	firstResult, err := SweepExpiredComputeRedeemPoints(db, now, 10)
	if err != nil {
		t.Fatalf("first sweep failed: %v", err)
	}
	if firstResult.Cleared != 1 {
		t.Fatalf("expected first cleared=1, got %d", firstResult.Cleared)
	}

	secondResult, err := SweepExpiredComputeRedeemPoints(db, now, 10)
	if err != nil {
		t.Fatalf("second sweep failed: %v", err)
	}
	if secondResult.Cleared != 0 {
		t.Fatalf("expected second cleared=0, got %d", secondResult.Cleared)
	}
	if secondResult.Scanned != 0 {
		t.Fatalf("expected second scanned=0, got %d", secondResult.Scanned)
	}

	var updatedAccount models.ComputeAccount
	if err := db.Where("user_id = ?", 1003).First(&updatedAccount).Error; err != nil {
		t.Fatalf("load updated account: %v", err)
	}
	if updatedAccount.AvailablePoints != 170 {
		t.Fatalf("expected available_points=170 after idempotent sweep, got %d", updatedAccount.AvailablePoints)
	}

	var ledgerCount int64
	if err := db.Model(&models.ComputeLedger{}).
		Where("user_id = ? AND type = ? AND points = ? AND remark = ?", 1003, "adjust", -30, "compute_redeem_expire_clear").
		Count(&ledgerCount).Error; err != nil {
		t.Fatalf("count clear ledgers: %v", err)
	}
	if ledgerCount != 1 {
		t.Fatalf("expected exactly one clear ledger, got %d", ledgerCount)
	}
}

func openComputeRedeemExpiryTestDB(t *testing.T) *gorm.DB {
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
		`CREATE TABLE IF NOT EXISTS ops.compute_redeem_redemptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			granted_points INTEGER NOT NULL DEFAULT 0,
			granted_starts_at DATETIME NOT NULL,
			granted_expires_at DATETIME,
			clear_status TEXT NOT NULL DEFAULT 'pending',
			cleared_points INTEGER NOT NULL DEFAULT 0,
			cleared_at DATETIME,
			ip TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			created_at DATETIME
		)`,
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create table for test: %v", err)
		}
	}
	return db
}

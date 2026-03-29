package videojobs

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	computeRedeemClearStatusPending       = "pending"
	computeRedeemClearStatusCleared       = "cleared"
	computeRedeemClearStatusNotApplicable = "not_applicable"

	defaultComputeRedeemExpireSweepLimit = 100
	maxComputeRedeemExpireSweepLimit     = 1000
)

var errComputeRedeemExpireSkip = errors.New("compute redeem expire skip")

type ComputeRedeemExpireSweepResult struct {
	Scanned           int    `json:"scanned"`
	Cleared           int    `json:"cleared"`
	Skipped           int    `json:"skipped"`
	Errors            int    `json:"errors"`
	TotalClearedPoint int64  `json:"total_cleared_points"`
	LastError         string `json:"last_error,omitempty"`
}

func NormalizeComputeRedeemClearStatus(status string, expiresAt *time.Time) string {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch normalized {
	case computeRedeemClearStatusPending, computeRedeemClearStatusCleared, computeRedeemClearStatusNotApplicable:
		return normalized
	}
	if expiresAt == nil {
		return computeRedeemClearStatusNotApplicable
	}
	return computeRedeemClearStatusPending
}

func SweepExpiredComputeRedeemPoints(db *gorm.DB, now time.Time, limit int) (ComputeRedeemExpireSweepResult, error) {
	result := ComputeRedeemExpireSweepResult{}
	if db == nil {
		return result, errors.New("invalid db")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if limit <= 0 {
		limit = defaultComputeRedeemExpireSweepLimit
	}
	if limit > maxComputeRedeemExpireSweepLimit {
		limit = maxComputeRedeemExpireSweepLimit
	}

	ids, err := listExpiredComputeRedeemCandidateIDs(db, now, limit)
	if err != nil {
		return result, err
	}
	if len(ids) == 0 {
		return result, nil
	}

	for _, redemptionID := range ids {
		result.Scanned++
		cleared, points, err := sweepOneExpiredComputeRedeem(db, redemptionID, now)
		if err != nil {
			if errors.Is(err, errComputeRedeemExpireSkip) {
				result.Skipped++
				continue
			}
			result.Errors++
			result.LastError = err.Error()
			continue
		}
		if cleared {
			result.Cleared++
			result.TotalClearedPoint += points
		} else {
			result.Skipped++
		}
	}
	return result, nil
}

func listExpiredComputeRedeemCandidateIDs(db *gorm.DB, now time.Time, limit int) ([]uint64, error) {
	ids := make([]uint64, 0, limit)
	err := db.Model(&models.ComputeRedeemRedemption{}).
		Where("granted_expires_at IS NOT NULL").
		Where("granted_expires_at <= ?", now).
		Where("(clear_status = ? OR clear_status = '' OR clear_status IS NULL)", computeRedeemClearStatusPending).
		Order("granted_expires_at ASC, id ASC").
		Limit(limit).
		Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func sweepOneExpiredComputeRedeem(db *gorm.DB, redemptionID uint64, now time.Time) (bool, int64, error) {
	if db == nil || redemptionID == 0 {
		return false, 0, errComputeRedeemExpireSkip
	}

	var deductedPoints int64
	err := db.Transaction(func(tx *gorm.DB) error {
		var redemption models.ComputeRedeemRedemption
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&redemption, redemptionID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errComputeRedeemExpireSkip
			}
			return err
		}

		clearStatus := NormalizeComputeRedeemClearStatus(redemption.ClearStatus, redemption.GrantedExpiresAt)
		if clearStatus != computeRedeemClearStatusPending {
			return errComputeRedeemExpireSkip
		}
		if redemption.GrantedExpiresAt == nil || redemption.GrantedExpiresAt.After(now) {
			return errComputeRedeemExpireSkip
		}

		alreadyCleared, err := hasComputeRedeemExpireClearLedgerTx(tx, redemption.UserID, redemption.ID)
		if err != nil {
			return err
		}
		if alreadyCleared {
			clearedPoints := redemption.ClearedPoints
			if clearedPoints <= 0 {
				clearedPoints = redemption.GrantedPoints
			}
			if clearedPoints < 0 {
				clearedPoints = 0
			}
			return tx.Model(&models.ComputeRedeemRedemption{}).
				Where("id = ?", redemption.ID).
				Updates(map[string]interface{}{
					"clear_status":   computeRedeemClearStatusCleared,
					"cleared_points": clearedPoints,
					"cleared_at":     now,
				}).Error
		}

		deductedPoints = redemption.GrantedPoints
		if deductedPoints < 0 {
			deductedPoints = 0
		}
		if deductedPoints > 0 {
			if err := AdjustComputePointsTx(tx, redemption.UserID, -deductedPoints, "compute_redeem_expire_clear", map[string]interface{}{
				"source":             "compute_redeem_expire_clear",
				"compute_redemption": redemption.ID,
				"compute_code_id":    redemption.CodeID,
				"granted_expires_at": redemption.GrantedExpiresAt.Format(time.RFC3339),
			}); err != nil {
				return err
			}
		}

		updates := map[string]interface{}{
			"clear_status":   computeRedeemClearStatusCleared,
			"cleared_points": deductedPoints,
			"cleared_at":     now,
		}
		return tx.Model(&models.ComputeRedeemRedemption{}).
			Where("id = ?", redemption.ID).
			Updates(updates).Error
	})
	if err != nil {
		if errors.Is(err, errComputeRedeemExpireSkip) {
			return false, 0, err
		}
		return false, 0, fmt.Errorf("sweep compute redemption %d: %w", redemptionID, err)
	}
	return true, deductedPoints, nil
}

func hasComputeRedeemExpireClearLedgerTx(tx *gorm.DB, userID, redemptionID uint64) (bool, error) {
	if tx == nil || userID == 0 || redemptionID == 0 {
		return false, nil
	}
	redemptionIDText := strconv.FormatUint(redemptionID, 10)
	baseQuery := tx.Model(&models.ComputeLedger{}).
		Where("user_id = ? AND type = ? AND remark = ? AND points <= 0", userID, "adjust", "compute_redeem_expire_clear")

	if tx.Dialector != nil && strings.EqualFold(strings.TrimSpace(tx.Dialector.Name()), "postgres") {
		var count int64
		if err := baseQuery.Where("metadata ->> 'compute_redemption' = ?", redemptionIDText).Count(&count).Error; err != nil {
			return false, err
		}
		return count > 0, nil
	}

	var rows []models.ComputeLedger
	if err := baseQuery.Select("metadata").Order("id DESC").Limit(200).Find(&rows).Error; err != nil {
		return false, err
	}
	for _, row := range rows {
		metadata := parseJSONMap(row.Metadata)
		if fmt.Sprint(metadata["compute_redemption"]) == redemptionIDText {
			return true, nil
		}
	}
	return false, nil
}

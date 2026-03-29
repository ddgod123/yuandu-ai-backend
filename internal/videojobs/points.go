package videojobs

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	minReservePoints     int64   = 10
	maxReservePoints     int64   = 800
	pointPerCNY          float64 = 100 // 1 point = 0.01 CNY
	costMarkupMultiplier float64 = 2.0 // 实际成本 1 元 => 用户侧计费 2 元
	initialPointsFree    int64   = 300
	initialPointsSub     int64   = 1200
	initialPointsPro     int64   = 3000
	initialPointsDefault int64   = 300
)

type InsufficientPointsError struct {
	Available int64
	Required  int64
}

func (e InsufficientPointsError) Error() string {
	return fmt.Sprintf("insufficient compute points: required=%d available=%d", e.Required, e.Available)
}

func PointPerCNY() float64 {
	return pointPerCNY
}

func CostMarkupMultiplier() float64 {
	return costMarkupMultiplier
}

func EstimateReservationPoints(sourceBytes int64, outputFormats []string, options map[string]interface{}) int64 {
	points := int64(12)
	if sourceBytes > 0 {
		sizeMB := float64(sourceBytes) / (1024 * 1024)
		points += int64(math.Ceil(sizeMB / 12))
	} else {
		points += 12
	}

	formatSet := map[string]struct{}{}
	for _, raw := range outputFormats {
		format := strings.ToLower(strings.TrimSpace(raw))
		if format == "" {
			continue
		}
		if _, ok := formatSet[format]; ok {
			continue
		}
		formatSet[format] = struct{}{}
		switch format {
		case "gif":
			points += 6
		case "webp":
			points += 5
		case "mp4":
			points += 4
		case "live":
			points += 6
		case "png":
			points += 3
		case "jpg", "jpeg":
			points += 2
		default:
			points += 2
		}
	}
	if count := int64(len(formatSet)); count > 1 {
		points += (count - 1) * 2
	}

	if parseOptionBool(options, "auto_highlight") {
		points += 3
	}
	width := parseOptionInt64(options, "width")
	if width >= 1080 {
		points += 5
	} else if width >= 720 {
		points += 3
	}
	fps := parseOptionInt64(options, "fps")
	if fps > 24 {
		points += 4
	} else if fps > 15 {
		points += 2
	}
	speed := parseOptionFloat(options, "speed")
	if speed > 0 && math.Abs(speed-1.0) > 0.001 {
		points++
	}

	if points < minReservePoints {
		return minReservePoints
	}
	if points > maxReservePoints {
		return maxReservePoints
	}
	return points
}

func EnsureComputeAccount(db *gorm.DB, userID uint64) (*models.ComputeAccount, error) {
	if db == nil || userID == 0 {
		return nil, errors.New("invalid db or user id")
	}

	var out models.ComputeAccount
	err := db.Transaction(func(tx *gorm.DB) error {
		account, err := ensureComputeAccountTx(tx, userID, false)
		if err != nil {
			return err
		}
		out = *account
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func ReservePointsForJob(tx *gorm.DB, userID, jobID uint64, points int64, remark string, metadata map[string]interface{}) error {
	if tx == nil || userID == 0 || jobID == 0 {
		return errors.New("invalid reserve points input")
	}
	if points <= 0 {
		points = minReservePoints
	}
	if points > maxReservePoints {
		points = maxReservePoints
	}

	account, err := ensureComputeAccountTx(tx, userID, true)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(account.Status), "active") {
		return errors.New("compute account is not active")
	}

	var existing models.ComputePointHold
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id = ?", jobID).First(&existing).Error
	if err == nil {
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if account.AvailablePoints < points {
		return InsufficientPointsError{
			Available: account.AvailablePoints,
			Required:  points,
		}
	}

	beforeAvail := account.AvailablePoints
	beforeFrozen := account.FrozenPoints
	beforeDebt := account.DebtPoints

	account.AvailablePoints -= points
	account.FrozenPoints += points
	if err := tx.Model(&models.ComputeAccount{}).Where("id = ?", account.ID).Updates(map[string]interface{}{
		"available_points": account.AvailablePoints,
		"frozen_points":    account.FrozenPoints,
	}).Error; err != nil {
		return err
	}

	hold := models.ComputePointHold{
		JobID:          jobID,
		UserID:         userID,
		AccountID:      account.ID,
		ReservedPoints: points,
		SettledPoints:  0,
		Status:         "held",
		Remark:         strings.TrimSpace(remark),
	}
	if err := tx.Create(&hold).Error; err != nil {
		return err
	}

	ledger := models.ComputeLedger{
		AccountID:       account.ID,
		UserID:          userID,
		JobID:           &jobID,
		Type:            "reserve",
		Points:          points,
		AvailableBefore: beforeAvail,
		AvailableAfter:  account.AvailablePoints,
		FrozenBefore:    beforeFrozen,
		FrozenAfter:     account.FrozenPoints,
		DebtBefore:      beforeDebt,
		DebtAfter:       account.DebtPoints,
		Remark:          strings.TrimSpace(remark),
		Metadata:        mustJSONOrEmpty(metadata),
	}
	return tx.Create(&ledger).Error
}

func ReleaseReservedPointsForJob(db *gorm.DB, jobID uint64, reason string) error {
	if db == nil || jobID == 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var hold models.ComputePointHold
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id = ?", jobID).First(&hold).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(hold.Status), "held") {
			return nil
		}

		var account models.ComputeAccount
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", hold.AccountID).First(&account).Error; err != nil {
			return err
		}

		release := hold.ReservedPoints
		if release < 0 {
			release = 0
		}
		if account.FrozenPoints < release {
			release = account.FrozenPoints
		}

		beforeAvail := account.AvailablePoints
		beforeFrozen := account.FrozenPoints
		beforeDebt := account.DebtPoints

		account.FrozenPoints -= release
		account.AvailablePoints += release
		if err := tx.Model(&models.ComputeAccount{}).Where("id = ?", account.ID).Updates(map[string]interface{}{
			"available_points": account.AvailablePoints,
			"frozen_points":    account.FrozenPoints,
		}).Error; err != nil {
			return err
		}

		now := time.Now()
		if err := tx.Model(&models.ComputePointHold{}).Where("id = ?", hold.ID).Updates(map[string]interface{}{
			"status":         "released",
			"settled_points": 0,
			"remark":         strings.TrimSpace(reason),
			"settled_at":     now,
		}).Error; err != nil {
			return err
		}

		ledger := models.ComputeLedger{
			AccountID:       account.ID,
			UserID:          hold.UserID,
			JobID:           &hold.JobID,
			Type:            "release",
			Points:          release,
			AvailableBefore: beforeAvail,
			AvailableAfter:  account.AvailablePoints,
			FrozenBefore:    beforeFrozen,
			FrozenAfter:     account.FrozenPoints,
			DebtBefore:      beforeDebt,
			DebtAfter:       account.DebtPoints,
			Remark:          strings.TrimSpace(reason),
			Metadata: mustJSONOrEmpty(map[string]interface{}{
				"hold_status_before": hold.Status,
			}),
		}
		return tx.Create(&ledger).Error
	})
}

func SettleReservedPointsForJob(db *gorm.DB, jobID uint64, finalStatus string) error {
	if db == nil || jobID == 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var hold models.ComputePointHold
		holdErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id = ?", jobID).First(&hold).Error
		if holdErr != nil && !errors.Is(holdErr, gorm.ErrRecordNotFound) {
			return holdErr
		}

		var cost models.VideoJobCost
		_ = tx.Where("job_id = ?", jobID).First(&cost).Error
		billableCostCNY, billableCostSource, aiCostCNY := resolveBillableCostCNY(cost)
		finalStatus = strings.ToLower(strings.TrimSpace(finalStatus))

		if errors.Is(holdErr, gorm.ErrRecordNotFound) {
			return settlePointsWithoutHoldTx(
				tx,
				jobID,
				finalStatus,
				cost,
				billableCostCNY,
				billableCostSource,
				aiCostCNY,
			)
		}

		holdStatus := strings.ToLower(strings.TrimSpace(hold.Status))
		if holdStatus == "settled" {
			return reconcileSettledPointsForHoldTx(
				tx,
				hold,
				finalStatus,
				cost,
				billableCostCNY,
				billableCostSource,
				aiCostCNY,
			)
		}
		if holdStatus != "held" {
			return nil
		}

		var account models.ComputeAccount
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", hold.AccountID).First(&account).Error; err != nil {
			return err
		}
		actual := computeActualPointsFromCost(finalStatus, billableCostCNY, hold.ReservedPoints)

		reserved := hold.ReservedPoints
		if reserved < 0 {
			reserved = 0
		}
		if account.FrozenPoints < reserved {
			reserved = account.FrozenPoints
		}

		beforeAvail := account.AvailablePoints
		beforeFrozen := account.FrozenPoints
		beforeDebt := account.DebtPoints

		account.FrozenPoints -= reserved
		if actual <= reserved {
			refund := reserved - actual
			account.AvailablePoints += refund
		} else {
			extra := actual - reserved
			if account.AvailablePoints >= extra {
				account.AvailablePoints -= extra
			} else {
				deficit := extra - account.AvailablePoints
				account.AvailablePoints = 0
				account.DebtPoints += deficit
			}
		}
		account.TotalConsumedPoints += actual
		if err := tx.Model(&models.ComputeAccount{}).Where("id = ?", account.ID).Updates(map[string]interface{}{
			"available_points":      account.AvailablePoints,
			"frozen_points":         account.FrozenPoints,
			"debt_points":           account.DebtPoints,
			"total_consumed_points": account.TotalConsumedPoints,
		}).Error; err != nil {
			return err
		}

		now := time.Now()
		if err := tx.Model(&models.ComputePointHold{}).Where("id = ?", hold.ID).Updates(map[string]interface{}{
			"status":         "settled",
			"settled_points": actual,
			"remark":         finalStatus,
			"settled_at":     now,
		}).Error; err != nil {
			return err
		}

		ledger := models.ComputeLedger{
			AccountID:       account.ID,
			UserID:          hold.UserID,
			JobID:           &hold.JobID,
			Type:            "settle",
			Points:          actual,
			AvailableBefore: beforeAvail,
			AvailableAfter:  account.AvailablePoints,
			FrozenBefore:    beforeFrozen,
			FrozenAfter:     account.FrozenPoints,
			DebtBefore:      beforeDebt,
			DebtAfter:       account.DebtPoints,
			Remark:          finalStatus,
			Metadata: mustJSONOrEmpty(map[string]interface{}{
				"reserved_points":        reserved,
				"estimated_cost":         cost.EstimatedCost,
				"billable_cost_cny":      billableCostCNY,
				"billable_cost_source":   billableCostSource,
				"ai_cost_cny":            aiCostCNY,
				"currency":               strings.TrimSpace(cost.Currency),
				"pricing_version":        strings.TrimSpace(cost.PricingVersion),
				"point_per_cny":          pointPerCNY,
				"cost_markup_multiplier": costMarkupMultiplier,
				"hold_status":            "settled",
				"final_status":           strings.ToLower(strings.TrimSpace(finalStatus)),
			}),
		}
		return tx.Create(&ledger).Error
	})
}

func settlePointsWithoutHoldTx(
	tx *gorm.DB,
	jobID uint64,
	finalStatus string,
	cost models.VideoJobCost,
	billableCostCNY float64,
	billableCostSource string,
	aiCostCNY float64,
) error {
	if tx == nil || jobID == 0 {
		return nil
	}

	userID := cost.UserID
	if userID == 0 {
		var job models.VideoJob
		if err := tx.Select("id", "user_id").Where("id = ?", jobID).First(&job).Error; err == nil {
			userID = job.UserID
		}
	}
	if userID == 0 {
		return nil
	}

	account, err := ensureComputeAccountTx(tx, userID, true)
	if err != nil {
		return err
	}

	actual := computeActualPointsFromCost(finalStatus, billableCostCNY, 0)

	beforeAvail := account.AvailablePoints
	beforeFrozen := account.FrozenPoints
	beforeDebt := account.DebtPoints

	applySettleDeltaWithoutHold(account, actual)
	account.TotalConsumedPoints += actual
	if account.TotalConsumedPoints < 0 {
		account.TotalConsumedPoints = 0
	}

	if err := tx.Model(&models.ComputeAccount{}).Where("id = ?", account.ID).Updates(map[string]interface{}{
		"available_points":      account.AvailablePoints,
		"frozen_points":         account.FrozenPoints,
		"debt_points":           account.DebtPoints,
		"total_consumed_points": account.TotalConsumedPoints,
	}).Error; err != nil {
		return err
	}

	now := time.Now()
	hold := models.ComputePointHold{
		JobID:          jobID,
		UserID:         userID,
		AccountID:      account.ID,
		ReservedPoints: 0,
		SettledPoints:  actual,
		Status:         "settled",
		Remark:         finalStatus,
		SettledAt:      &now,
	}
	if err := tx.Create(&hold).Error; err != nil {
		return err
	}

	ledger := models.ComputeLedger{
		AccountID:       account.ID,
		UserID:          userID,
		JobID:           &jobID,
		Type:            "settle",
		Points:          actual,
		AvailableBefore: beforeAvail,
		AvailableAfter:  account.AvailablePoints,
		FrozenBefore:    beforeFrozen,
		FrozenAfter:     account.FrozenPoints,
		DebtBefore:      beforeDebt,
		DebtAfter:       account.DebtPoints,
		Remark:          finalStatus,
		Metadata: mustJSONOrEmpty(map[string]interface{}{
			"reserved_points":        0,
			"estimated_cost":         cost.EstimatedCost,
			"billable_cost_cny":      billableCostCNY,
			"billable_cost_source":   billableCostSource,
			"ai_cost_cny":            aiCostCNY,
			"currency":               strings.TrimSpace(cost.Currency),
			"pricing_version":        strings.TrimSpace(cost.PricingVersion),
			"point_per_cny":          pointPerCNY,
			"cost_markup_multiplier": costMarkupMultiplier,
			"hold_status":            "missing_auto_settled",
			"final_status":           finalStatus,
		}),
	}
	return tx.Create(&ledger).Error
}

func reconcileSettledPointsForHoldTx(
	tx *gorm.DB,
	hold models.ComputePointHold,
	finalStatus string,
	cost models.VideoJobCost,
	billableCostCNY float64,
	billableCostSource string,
	aiCostCNY float64,
) error {
	if tx == nil || hold.JobID == 0 {
		return nil
	}
	target := computeActualPointsFromCost(finalStatus, billableCostCNY, hold.ReservedPoints)
	if target == hold.SettledPoints {
		return nil
	}

	var account models.ComputeAccount
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", hold.AccountID).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) && hold.UserID > 0 {
			found, ensureErr := ensureComputeAccountTx(tx, hold.UserID, true)
			if ensureErr != nil {
				return ensureErr
			}
			account = *found
		} else {
			return err
		}
	}

	beforeAvail := account.AvailablePoints
	beforeFrozen := account.FrozenPoints
	beforeDebt := account.DebtPoints

	delta := target - hold.SettledPoints
	applySettleDeltaWithoutHold(&account, delta)
	account.TotalConsumedPoints += delta
	if account.TotalConsumedPoints < 0 {
		account.TotalConsumedPoints = 0
	}

	if err := tx.Model(&models.ComputeAccount{}).Where("id = ?", account.ID).Updates(map[string]interface{}{
		"available_points":      account.AvailablePoints,
		"frozen_points":         account.FrozenPoints,
		"debt_points":           account.DebtPoints,
		"total_consumed_points": account.TotalConsumedPoints,
	}).Error; err != nil {
		return err
	}

	now := time.Now()
	if err := tx.Model(&models.ComputePointHold{}).Where("id = ?", hold.ID).Updates(map[string]interface{}{
		"account_id":      account.ID,
		"settled_points":  target,
		"remark":          finalStatus,
		"settled_at":      now,
		"updated_at":      now,
		"status":          "settled",
		"reserved_points": hold.ReservedPoints,
	}).Error; err != nil {
		return err
	}

	ledger := models.ComputeLedger{
		AccountID:       account.ID,
		UserID:          hold.UserID,
		JobID:           &hold.JobID,
		Type:            "settle_adjust",
		Points:          delta,
		AvailableBefore: beforeAvail,
		AvailableAfter:  account.AvailablePoints,
		FrozenBefore:    beforeFrozen,
		FrozenAfter:     account.FrozenPoints,
		DebtBefore:      beforeDebt,
		DebtAfter:       account.DebtPoints,
		Remark:          finalStatus,
		Metadata: mustJSONOrEmpty(map[string]interface{}{
			"previous_settled_points": hold.SettledPoints,
			"settled_points":          target,
			"reserved_points":         hold.ReservedPoints,
			"estimated_cost":          cost.EstimatedCost,
			"billable_cost_cny":       billableCostCNY,
			"billable_cost_source":    billableCostSource,
			"ai_cost_cny":             aiCostCNY,
			"currency":                strings.TrimSpace(cost.Currency),
			"pricing_version":         strings.TrimSpace(cost.PricingVersion),
			"point_per_cny":           pointPerCNY,
			"cost_markup_multiplier":  costMarkupMultiplier,
			"hold_status":             "settled_reconciled",
			"final_status":            finalStatus,
		}),
	}
	return tx.Create(&ledger).Error
}

func applySettleDeltaWithoutHold(account *models.ComputeAccount, delta int64) {
	if account == nil || delta == 0 {
		return
	}
	if delta > 0 {
		if account.AvailablePoints >= delta {
			account.AvailablePoints -= delta
			return
		}
		deficit := delta - account.AvailablePoints
		account.AvailablePoints = 0
		account.DebtPoints += deficit
		return
	}

	refund := -delta
	if account.DebtPoints > 0 {
		repay := refund
		if repay > account.DebtPoints {
			repay = account.DebtPoints
		}
		account.DebtPoints -= repay
		refund -= repay
	}
	account.AvailablePoints += refund
}

func AdjustComputePoints(db *gorm.DB, userID uint64, delta int64, reason string, metadata map[string]interface{}) error {
	if db == nil {
		return errors.New("invalid compute account adjust input")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		return AdjustComputePointsTx(tx, userID, delta, reason, metadata)
	})
}

func AdjustComputePointsTx(tx *gorm.DB, userID uint64, delta int64, reason string, metadata map[string]interface{}) error {
	if tx == nil || userID == 0 || delta == 0 {
		return errors.New("invalid compute account adjust input")
	}

	account, err := ensureComputeAccountTx(tx, userID, true)
	if err != nil {
		return err
	}

	beforeAvail := account.AvailablePoints
	beforeFrozen := account.FrozenPoints
	beforeDebt := account.DebtPoints

	if delta > 0 {
		credit := delta
		if account.DebtPoints > 0 {
			repay := credit
			if repay > account.DebtPoints {
				repay = account.DebtPoints
			}
			account.DebtPoints -= repay
			credit -= repay
		}
		account.AvailablePoints += credit
		account.TotalRechargedPoints += delta
	} else {
		cost := -delta
		if account.AvailablePoints >= cost {
			account.AvailablePoints -= cost
		} else {
			deficit := cost - account.AvailablePoints
			account.AvailablePoints = 0
			account.DebtPoints += deficit
		}
	}
	if err := tx.Model(&models.ComputeAccount{}).Where("id = ?", account.ID).Updates(map[string]interface{}{
		"available_points":       account.AvailablePoints,
		"debt_points":            account.DebtPoints,
		"total_recharged_points": account.TotalRechargedPoints,
	}).Error; err != nil {
		return err
	}

	ledger := models.ComputeLedger{
		AccountID:       account.ID,
		UserID:          userID,
		Type:            "adjust",
		Points:          delta,
		AvailableBefore: beforeAvail,
		AvailableAfter:  account.AvailablePoints,
		FrozenBefore:    beforeFrozen,
		FrozenAfter:     account.FrozenPoints,
		DebtBefore:      beforeDebt,
		DebtAfter:       account.DebtPoints,
		Remark:          strings.TrimSpace(reason),
		Metadata:        mustJSONOrEmpty(metadata),
	}
	return tx.Create(&ledger).Error
}

func computeActualPointsFromCost(finalStatus string, billableCostCNY float64, reserved int64) int64 {
	switch finalStatus {
	case models.VideoJobStatusCancelled:
		return 0
	case models.VideoJobStatusFailed:
		if reserved <= 0 {
			return 0
		}
		return 1
	default:
		points := pointsFromEstimatedCost(billableCostCNY)
		if points <= 0 {
			if reserved > 0 {
				return 1
			}
			return 0
		}
		return points
	}
}

func resolveBillableCostCNY(cost models.VideoJobCost) (billableCostCNY float64, source string, aiCostCNY float64) {
	details := parseJSONMap(cost.Details)
	aiCostCNY = parseOptionFloat(details, "ai_cost_cny")
	if aiCostCNY > 0 {
		return aiCostCNY, "ai_cost_cny", aiCostCNY
	}
	if strings.EqualFold(strings.TrimSpace(cost.Currency), "cny") && cost.EstimatedCost > 0 {
		return cost.EstimatedCost, "estimated_cost", aiCostCNY
	}
	return 0, "none", aiCostCNY
}

func pointsFromEstimatedCost(cost float64) int64 {
	if cost <= 0 {
		return 0
	}
	return int64(math.Ceil(cost * pointPerCNY * costMarkupMultiplier))
}

func ensureComputeAccountTx(tx *gorm.DB, userID uint64, forUpdate bool) (*models.ComputeAccount, error) {
	if tx == nil || userID == 0 {
		return nil, errors.New("invalid tx or user id")
	}

	query := tx.Model(&models.ComputeAccount{}).Where("user_id = ?", userID)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}

	var account models.ComputeAccount
	if err := query.First(&account).Error; err == nil {
		return &account, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	initialPoints, err := resolveInitialPointsTx(tx, userID)
	if err != nil {
		return nil, err
	}
	account = models.ComputeAccount{
		UserID:               userID,
		AvailablePoints:      initialPoints,
		FrozenPoints:         0,
		DebtPoints:           0,
		TotalConsumedPoints:  0,
		TotalRechargedPoints: initialPoints,
		Status:               "active",
	}
	if err := tx.Create(&account).Error; err != nil {
		return nil, err
	}
	if initialPoints > 0 {
		ledger := models.ComputeLedger{
			AccountID:       account.ID,
			UserID:          userID,
			Type:            "init_grant",
			Points:          initialPoints,
			AvailableBefore: 0,
			AvailableAfter:  initialPoints,
			FrozenBefore:    0,
			FrozenAfter:     0,
			DebtBefore:      0,
			DebtAfter:       0,
			Remark:          "init compute points",
			Metadata: mustJSONOrEmpty(map[string]interface{}{
				"source": "auto_init",
			}),
		}
		if err := tx.Create(&ledger).Error; err != nil {
			return nil, err
		}
	}
	return &account, nil
}

func resolveInitialPointsTx(tx *gorm.DB, userID uint64) (int64, error) {
	var user models.User
	if err := tx.Select("id", "subscription_status", "subscription_plan", "subscription_expires_at").First(&user, userID).Error; err != nil {
		return 0, err
	}
	status := strings.ToLower(strings.TrimSpace(user.SubscriptionStatus))
	plan := strings.ToLower(strings.TrimSpace(user.SubscriptionPlan))
	now := time.Now()
	isActive := status == "active" && (user.SubscriptionExpiresAt == nil || !now.After(*user.SubscriptionExpiresAt))
	if !isActive {
		return initialPointsFree, nil
	}

	switch plan {
	case "pro", "enterprise":
		return initialPointsPro, nil
	case "subscriber", "plus", "premium", "vip":
		return initialPointsSub, nil
	default:
		if plan == "" {
			return initialPointsDefault, nil
		}
		return initialPointsSub, nil
	}
}

func parseOptionInt64(options map[string]interface{}, key string) int64 {
	return int64(parseOptionFloat(options, key))
}

func parseOptionBool(options map[string]interface{}, key string) bool {
	if len(options) == 0 {
		return false
	}
	raw, ok := options[key]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		v = strings.ToLower(strings.TrimSpace(v))
		return v == "1" || v == "true" || v == "yes" || v == "on"
	case float64:
		return v > 0
	case int:
		return v > 0
	default:
		return false
	}
}

func parseOptionFloat(options map[string]interface{}, key string) float64 {
	if len(options) == 0 {
		return 0
	}
	raw, ok := options[key]
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int64:
		return float64(v)
	case int:
		return float64(v)
	case int32:
		return float64(v)
	case uint64:
		return float64(v)
	case uint:
		return float64(v)
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return 0
		}
		parsed, err := strconvParseFloat(v)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func mustJSONOrEmpty(v interface{}) datatypes.JSON {
	if v == nil {
		return datatypes.JSON([]byte("{}"))
	}
	return mustJSON(v)
}

var strconvParseFloat = func(raw string) (float64, error) {
	return strconv.ParseFloat(raw, 64)
}

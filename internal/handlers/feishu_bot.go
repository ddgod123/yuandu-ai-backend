package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"emoji/internal/feishu"
	"emoji/internal/feishujobs"
	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errFeishuBindCodeExpired      = errors.New("feishu bind code expired")
	errFeishuBindCodeUsed         = errors.New("feishu bind code already used")
	errFeishuBindCodeBoundToOther = errors.New("feishu account bound to another user")
)

type feishuEventEnvelope struct {
	Type      string             `json:"type"`
	Challenge string             `json:"challenge"`
	Token     string             `json:"token"`
	Schema    string             `json:"schema"`
	Header    feishuEventHeader  `json:"header"`
	Event     feishuEventPayload `json:"event"`
}

type feishuEventHeader struct {
	EventID   string `json:"event_id"`
	EventType string `json:"event_type"`
	TenantKey string `json:"tenant_key"`
	Token     string `json:"token"`
	AppID     string `json:"app_id"`
}

type feishuEventPayload struct {
	Sender  feishuEventSender  `json:"sender"`
	Message feishuEventMessage `json:"message"`
}

type feishuEventSender struct {
	SenderID feishuEventSenderID `json:"sender_id"`
}

type feishuEventSenderID struct {
	OpenID  string `json:"open_id"`
	UnionID string `json:"union_id"`
	UserID  string `json:"user_id"`
}

type feishuEventMessage struct {
	MessageID   string `json:"message_id"`
	RootID      string `json:"root_id"`
	ParentID    string `json:"parent_id"`
	CreateTime  string `json:"create_time"`
	ChatID      string `json:"chat_id"`
	ChatType    string `json:"chat_type"`
	MessageType string `json:"message_type"`
	Content     string `json:"content"`
}

type FeishuBindCodeConfirmRequest struct {
	Code string `json:"code"`
}

func (h *Handler) FeishuEventCallback(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db not configured"})
		return
	}
	if h.queue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "queue not configured"})
		return
	}

	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if len(rawBody) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty request body"})
		return
	}

	var event feishuEventEnvelope
	if err := json.Unmarshal(rawBody, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	if strings.EqualFold(strings.TrimSpace(event.Type), "url_verification") {
		if !h.verifyFeishuVerificationToken(event.Token, event.Header.Token) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid feishu verification token"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"challenge": event.Challenge})
		return
	}

	if !h.verifyFeishuVerificationToken(event.Header.Token, event.Token) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid feishu verification token"})
		return
	}

	eventType := strings.TrimSpace(event.Header.EventType)
	if eventType == "" {
		eventType = strings.TrimSpace(event.Type)
	}
	if eventType != "im.message.receive_v1" {
		writeFeishuOK(c)
		return
	}

	message := event.Event.Message
	messageType := strings.ToLower(strings.TrimSpace(message.MessageType))
	if !isSupportedFeishuMessageType(messageType) {
		writeFeishuOK(c)
		return
	}

	fileKey, fileName, ok := extractFeishuFileMeta(message.Content)
	if !ok {
		writeFeishuOK(c)
		return
	}

	tenantKey := strings.TrimSpace(event.Header.TenantKey)
	if tenantKey == "" {
		tenantKey = "unknown"
	}
	messageID := strings.TrimSpace(message.MessageID)
	if messageID == "" {
		writeFeishuOK(c)
		return
	}
	chatID := strings.TrimSpace(message.ChatID)
	if chatID == "" {
		writeFeishuOK(c)
		return
	}
	openID := strings.TrimSpace(event.Event.Sender.SenderID.OpenID)
	if openID == "" {
		writeFeishuOK(c)
		return
	}
	unionID := strings.TrimSpace(event.Event.Sender.SenderID.UnionID)

	eventID := strings.TrimSpace(event.Header.EventID)
	if eventID == "" {
		eventID = fallbackFeishuEventID(rawBody)
	}

	eventLog := models.FeishuEventLog{
		EventID:   eventID,
		EventType: eventType,
		TenantKey: tenantKey,
		MessageID: messageID,
		Status:    models.FeishuEventStatusReceived,
		Payload:   toJSON(event),
	}
	insertRes := h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_id"}},
		DoNothing: true,
	}).Create(&eventLog)
	if insertRes.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": insertRes.Error.Error()})
		return
	}
	if insertRes.RowsAffected == 0 {
		writeFeishuOK(c)
		return
	}

	now := time.Now()
	msgJob := models.FeishuMessageJob{}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		upsert := models.FeishuMessageJob{
			TenantKey:      tenantKey,
			ChatID:         chatID,
			MessageID:      messageID,
			MessageType:    messageType,
			FileKey:        fileKey,
			FileName:       fileName,
			OpenID:         openID,
			UnionID:        unionID,
			Status:         models.FeishuMessageJobStatusQueued,
			ErrorMessage:   "",
			NotifyAttempts: 0,
			RequestPayload: toJSON(event),
			ResultPayload:  toJSON(nil),
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_key"}, {Name: "message_id"}, {Name: "file_key"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"chat_id":         upsert.ChatID,
				"message_type":    upsert.MessageType,
				"file_name":       upsert.FileName,
				"open_id":         upsert.OpenID,
				"union_id":        upsert.UnionID,
				"request_payload": upsert.RequestPayload,
				"updated_at":      now,
			}),
		}).Create(&upsert).Error; err != nil {
			return err
		}

		if err := tx.Where("tenant_key = ? AND message_id = ? AND file_key = ?", tenantKey, messageID, fileKey).First(&msgJob).Error; err != nil {
			return err
		}
		if msgJob.Status == models.FeishuMessageJobStatusFailed || msgJob.Status == models.FeishuMessageJobStatusRetrying || msgJob.Status == models.FeishuMessageJobStatusWaitingBind {
			if err := tx.Model(&models.FeishuMessageJob{}).Where("id = ?", msgJob.ID).Updates(map[string]interface{}{
				"status":      models.FeishuMessageJobStatusQueued,
				"updated_at":  now,
				"finished_at": nil,
			}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&models.FeishuEventLog{}).Where("id = ?", eventLog.ID).Updates(map[string]interface{}{
			"status":     models.FeishuEventStatusQueued,
			"updated_at": now,
		}).Error
	})
	if err != nil {
		_ = h.db.Model(&models.FeishuEventLog{}).Where("id = ?", eventLog.ID).Updates(map[string]interface{}{
			"status":     models.FeishuEventStatusFailed,
			"updated_at": time.Now(),
		}).Error
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if enqueueErr := h.enqueueFeishuIngestTask(msgJob.ID); enqueueErr != nil {
		_ = h.db.Model(&models.FeishuEventLog{}).Where("id = ?", eventLog.ID).Updates(map[string]interface{}{
			"status":     models.FeishuEventStatusFailed,
			"updated_at": time.Now(),
		}).Error
		c.JSON(http.StatusInternalServerError, gin.H{"error": enqueueErr.Error()})
		return
	}

	writeFeishuOK(c)
}

func (h *Handler) ConfirmFeishuBindCode(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db not configured"})
		return
	}
	if h.queue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "queue not configured"})
		return
	}
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req FeishuBindCodeConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	code := normalizeFeishuBindCode(req.Code)
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	now := time.Now()
	var bindCode models.FeishuBindCode
	resumedIDs := make([]uint64, 0)
	alreadyBound := false

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("code = ?", code).First(&bindCode).Error; err != nil {
			return err
		}

		switch bindCode.Status {
		case models.FeishuBindCodeStatusUsed:
			if bindCode.UserID != nil && *bindCode.UserID == userID {
				alreadyBound = true
				return nil
			}
			return errFeishuBindCodeUsed
		case models.FeishuBindCodeStatusExpired:
			return errFeishuBindCodeExpired
		}

		if bindCode.ExpiresAt.Before(now) {
			if err := tx.Model(&models.FeishuBindCode{}).Where("id = ?", bindCode.ID).Updates(map[string]interface{}{
				"status":     models.FeishuBindCodeStatusExpired,
				"updated_at": now,
			}).Error; err != nil {
				return err
			}
			return errFeishuBindCodeExpired
		}

		var account models.ExternalAccount
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("provider = ? AND tenant_key = ? AND open_id = ?", models.ExternalAccountProviderFeishu, bindCode.TenantKey, bindCode.OpenID).
			First(&account).Error
		if err == nil {
			if account.UserID != userID {
				return errFeishuBindCodeBoundToOther
			}
			if err := tx.Model(&models.ExternalAccount{}).Where("id = ?", account.ID).Updates(map[string]interface{}{
				"union_id":   strings.TrimSpace(bindCode.UnionID),
				"status":     models.ExternalAccountStatusActive,
				"updated_at": now,
			}).Error; err != nil {
				return err
			}
		} else {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			account = models.ExternalAccount{
				Provider:  models.ExternalAccountProviderFeishu,
				TenantKey: strings.TrimSpace(bindCode.TenantKey),
				OpenID:    strings.TrimSpace(bindCode.OpenID),
				UnionID:   strings.TrimSpace(bindCode.UnionID),
				UserID:    userID,
				Status:    models.ExternalAccountStatusActive,
				Metadata:  toJSON(map[string]interface{}{"source": "bind_code"}),
			}
			if err := tx.Create(&account).Error; err != nil {
				if isDuplicateDBError(err) {
					return errFeishuBindCodeBoundToOther
				}
				return err
			}
		}

		if err := tx.Model(&models.FeishuBindCode{}).Where("id = ?", bindCode.ID).Updates(map[string]interface{}{
			"status":     models.FeishuBindCodeStatusUsed,
			"user_id":    userID,
			"used_at":    now,
			"updated_at": now,
		}).Error; err != nil {
			return err
		}

		var waiting []models.FeishuMessageJob
		if err := tx.Select("id").Where("tenant_key = ? AND open_id = ? AND status = ?", bindCode.TenantKey, bindCode.OpenID, models.FeishuMessageJobStatusWaitingBind).Find(&waiting).Error; err != nil {
			return err
		}
		if len(waiting) > 0 {
			resumedIDs = make([]uint64, 0, len(waiting))
			for _, row := range waiting {
				resumedIDs = append(resumedIDs, row.ID)
			}
			if err := tx.Model(&models.FeishuMessageJob{}).Where("id IN ?", resumedIDs).Updates(map[string]interface{}{
				"status":        models.FeishuMessageJobStatusQueued,
				"bind_code":     "",
				"error_message": "",
				"user_id":       userID,
				"finished_at":   nil,
				"updated_at":    now,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "bind code not found"})
		case errors.Is(err, errFeishuBindCodeExpired):
			c.JSON(http.StatusBadRequest, gin.H{"error": "bind code expired"})
		case errors.Is(err, errFeishuBindCodeUsed):
			c.JSON(http.StatusConflict, gin.H{"error": "bind code already used"})
		case errors.Is(err, errFeishuBindCodeBoundToOther):
			c.JSON(http.StatusConflict, gin.H{"error": "feishu account already bound to another user"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	for _, id := range resumedIDs {
		_ = h.enqueueFeishuIngestTask(id)
	}

	if !alreadyBound {
		client := feishu.NewClient(h.cfg)
		if client.Enabled() && strings.TrimSpace(bindCode.ChatID) != "" {
			_ = client.SendTextMessageToChat(c.Request.Context(), bindCode.ChatID, "账号绑定成功，已为你继续处理之前的视频任务。")
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":            true,
		"already_bound": alreadyBound,
		"resumed_jobs":  len(resumedIDs),
		"tenant_key":    bindCode.TenantKey,
		"open_id":       bindCode.OpenID,
	})
}

func (h *Handler) enqueueFeishuIngestTask(messageJobID uint64) error {
	if h.queue == nil || messageJobID == 0 {
		return nil
	}
	task, err := feishujobs.NewIngestFeishuMessageTask(messageJobID)
	if err != nil {
		return err
	}
	_, err = h.queue.Enqueue(
		task,
		asynq.Queue("default"),
		asynq.MaxRetry(8),
		asynq.Timeout(10*time.Minute),
		asynq.Retention(7*24*time.Hour),
		asynq.TaskID(fmt.Sprintf("feishu-ingest-%d", messageJobID)),
	)
	if err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
		return err
	}
	return nil
}

func verifyFeishuToken(expected string, values ...string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}

func (h *Handler) verifyFeishuVerificationToken(values ...string) bool {
	return verifyFeishuToken(h.cfg.FeishuBotVerificationToken, values...)
}

func writeFeishuOK(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
}

func isSupportedFeishuMessageType(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "file", "media", "video":
		return true
	default:
		return false
	}
}

func extractFeishuFileMeta(content string) (fileKey string, fileName string, ok bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", "", false
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return "", "", false
	}

	pickString := func(keys ...string) string {
		for _, key := range keys {
			if value, exists := parsed[key]; exists {
				s := strings.TrimSpace(fmt.Sprint(value))
				if s != "" && s != "<nil>" {
					return s
				}
			}
		}
		return ""
	}

	fileKey = pickString("file_key", "media_key", "resource_key")
	fileName = pickString("file_name", "name", "title")
	if fileName == "" {
		fileName = "source.mp4"
	}
	if fileKey == "" {
		return "", "", false
	}
	return fileKey, fileName, true
}

func fallbackFeishuEventID(rawBody []byte) string {
	sum := sha256.Sum256(rawBody)
	return "sha256_" + hex.EncodeToString(sum[:12])
}

func normalizeFeishuBindCode(raw string) string {
	code := strings.ToUpper(strings.TrimSpace(raw))
	code = strings.ReplaceAll(code, " ", "")
	code = strings.ReplaceAll(code, "-", "")
	if len(code) > 32 {
		code = code[:32]
	}
	return code
}

func isDuplicateDBError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique")
}

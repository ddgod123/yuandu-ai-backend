package handlers

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"

	"emoji/internal/models"

	"gorm.io/gorm"
)

const collectionAutoTagCodeAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
const collectionAutoTagCodeLength = 20

func normalizeCollectionAutoTagCode(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func ensureCollectionAutoTagCode(db *gorm.DB, current string) (string, error) {
	code := normalizeCollectionAutoTagCode(current)
	if code != "" {
		return code, nil
	}
	for i := 0; i < 12; i++ {
		next, err := randomCollectionAutoTagCode(collectionAutoTagCodeLength)
		if err != nil {
			return "", err
		}
		var count int64
		if err := db.Model(&models.Collection{}).Where("auto_tag_code = ?", next).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return next, nil
		}
	}
	return "", errors.New("failed to generate unique auto tag code")
}

func randomCollectionAutoTagCode(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("invalid auto tag code length")
	}
	max := big.NewInt(int64(len(collectionAutoTagCodeAlphabet)))
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = collectionAutoTagCodeAlphabet[n.Int64()]
	}
	return string(out), nil
}

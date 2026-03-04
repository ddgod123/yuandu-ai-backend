package handlers

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"

	"emoji/internal/models"

	"gorm.io/gorm"
)

const downloadCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
const downloadCodeLength = 8

func ensureCollectionDownloadCode(db *gorm.DB, current string) (string, error) {
	if strings.TrimSpace(current) != "" {
		return current, nil
	}
	for i := 0; i < 10; i++ {
		code, err := randomDownloadCode(downloadCodeLength)
		if err != nil {
			return "", err
		}
		var count int64
		if err := db.Model(&models.Collection{}).Where("download_code = ?", code).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return code, nil
		}
	}
	return "", errors.New("failed to generate unique download code")
}

func randomDownloadCode(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("invalid code length")
	}
	max := big.NewInt(int64(len(downloadCodeAlphabet)))
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = downloadCodeAlphabet[n.Int64()]
	}
	return string(out), nil
}

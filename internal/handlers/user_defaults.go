package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/url"
	"strings"
)

const defaultAvatarStyle = "adventurer"

func generateDisplayName() string {
	return fmt.Sprintf("表情用户%s", randomDigits(4))
}

func defaultAvatarURL(phone string) string {
	seed := avatarSeed(phone)
	escaped := url.QueryEscape(seed)
	return fmt.Sprintf("https://api.dicebear.com/7.x/%s/svg?seed=%s", defaultAvatarStyle, escaped)
}

func avatarSeed(phone string) string {
	trimmed := strings.TrimSpace(phone)
	if trimmed == "" {
		return randomDigits(8)
	}
	sum := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(sum[:6])
}

func randomDigits(length int) string {
	if length <= 0 {
		return "0000"
	}
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			out[i] = '0'
			continue
		}
		out[i] = byte('0' + n.Int64())
	}
	return string(out)
}

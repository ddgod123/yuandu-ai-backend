package handlers

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func parseIntQuery(c *gin.Context, key string, def int) int {
	val := strings.TrimSpace(c.Query(key))
	if val == "" {
		return def
	}
	parsed, err := strconv.Atoi(val)
	if err != nil || parsed <= 0 {
		return def
	}
	return parsed
}

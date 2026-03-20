package middleware

import (
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"
)

// RecoveryWithStack recovers from request panic and records stack/context for ops troubleshooting.
func RecoveryWithStack() gin.HandlerFunc {
	logger := log.New(os.Stderr, "[panic] ", log.LstdFlags|log.Lmicroseconds)
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		path := ""
		if c.Request != nil && c.Request.URL != nil {
			path = c.Request.URL.Path
		}
		query := ""
		if c.Request != nil && c.Request.URL != nil {
			query = c.Request.URL.RawQuery
		}
		clientIP := strings.TrimSpace(c.ClientIP())
		method := ""
		if c.Request != nil {
			method = c.Request.Method
		}
		logger.Printf("recovered panic method=%s path=%s query=%s client_ip=%s panic=%v\n%s",
			method,
			path,
			query,
			clientIP,
			recovered,
			debug.Stack(),
		)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	})
}

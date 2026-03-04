package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"emoji/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func accessTokenCookieName(cfg config.Config) string {
	name := strings.TrimSpace(cfg.AuthAccessCookieName)
	if name == "" {
		return "emoji_access_token"
	}
	return name
}

func tokenFromRequest(c *gin.Context, cfg config.Config) string {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token := strings.TrimSpace(parts[1])
			if token != "" {
				return token
			}
		}
	}

	if val, err := c.Cookie(accessTokenCookieName(cfg)); err == nil {
		token := strings.TrimSpace(val)
		if token != "" {
			return token
		}
	}
	return ""
}

func Auth(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := tokenFromRequest(c, cfg)
		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		token, err := jwt.ParseWithClaims(tokenString, jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
			return []byte(cfg.JWTSecret), nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		sub, _ := claims["sub"].(string)
		if sub != "" {
			if uid, err := strconv.ParseUint(sub, 10, 64); err == nil {
				c.Set("user_id", uid)
			}
		}
		if role, _ := claims["role"].(string); role != "" {
			c.Set("role", role)
		}

		c.Set("token", token)
		c.Next()
	}
}

// AuthOptional parses bearer token when provided and silently skips on invalid/missing token.
// Public endpoints can use role/user hints from context without forcing authentication.
func AuthOptional(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := tokenFromRequest(c, cfg)
		if tokenString == "" {
			c.Next()
			return
		}

		token, err := jwt.ParseWithClaims(tokenString, jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
			return []byte(cfg.JWTSecret), nil
		})
		if err != nil || !token.Valid {
			c.Next()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.Next()
			return
		}

		sub, _ := claims["sub"].(string)
		if sub != "" {
			if uid, err := strconv.ParseUint(sub, 10, 64); err == nil {
				c.Set("user_id", uid)
			}
		}
		if role, _ := claims["role"].(string); role != "" {
			c.Set("role", role)
		}
		c.Set("token", token)
		c.Next()
	}
}

func RequireAnyRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		val, ok := c.Get("role")
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		current, _ := val.(string)
		current = strings.ToLower(current)
		for _, role := range roles {
			if current == strings.ToLower(role) {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	}
}

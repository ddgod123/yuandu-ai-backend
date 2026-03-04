package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (h *Handler) authAccessCookieName() string {
	name := strings.TrimSpace(h.cfg.AuthAccessCookieName)
	if name == "" {
		return "emoji_access_token"
	}
	return name
}

func (h *Handler) authRefreshCookieName() string {
	name := strings.TrimSpace(h.cfg.AuthRefreshCookieName)
	if name == "" {
		return "emoji_refresh_token"
	}
	return name
}

func (h *Handler) authCookieSameSite() http.SameSite {
	switch strings.ToLower(strings.TrimSpace(h.cfg.AuthCookieSameSite)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func (h *Handler) setAuthCookies(c *gin.Context, tokens TokenResponse) {
	accessToken := strings.TrimSpace(tokens.AccessToken)
	refreshToken := strings.TrimSpace(tokens.RefreshToken)
	if accessToken == "" || refreshToken == "" {
		return
	}
	domain := strings.TrimSpace(h.cfg.AuthCookieDomain)
	secure := h.cfg.AuthCookieSecure
	accessMaxAge := int(h.cfg.AccessTokenTTL.Seconds())
	refreshMaxAge := int(h.cfg.RefreshTokenTTL.Seconds())
	if accessMaxAge < 0 {
		accessMaxAge = 0
	}
	if refreshMaxAge < 0 {
		refreshMaxAge = 0
	}
	c.SetSameSite(h.authCookieSameSite())
	c.SetCookie(h.authAccessCookieName(), accessToken, accessMaxAge, "/", domain, secure, true)
	c.SetCookie(h.authRefreshCookieName(), refreshToken, refreshMaxAge, "/", domain, secure, true)
}

func (h *Handler) clearAuthCookies(c *gin.Context) {
	domain := strings.TrimSpace(h.cfg.AuthCookieDomain)
	secure := h.cfg.AuthCookieSecure
	c.SetSameSite(h.authCookieSameSite())
	c.SetCookie(h.authAccessCookieName(), "", -1, "/", domain, secure, true)
	c.SetCookie(h.authRefreshCookieName(), "", -1, "/", domain, secure, true)
}

func (h *Handler) refreshTokenFromRequestBodyOrCookie(c *gin.Context, token string) string {
	trimmed := strings.TrimSpace(token)
	if trimmed != "" {
		return trimmed
	}
	val, err := c.Cookie(h.authRefreshCookieName())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(val)
}

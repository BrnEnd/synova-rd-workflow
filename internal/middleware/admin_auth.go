package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	adminSvc "synova-rd-workflow/internal/service/admin"
)

type AuthVerifier interface {
	Verify(token string) (adminSvc.Claims, error)
}

func AdminAuth(auth AuthVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie("session_token")
		if err != nil || strings.TrimSpace(token) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		claims, err := auth.Verify(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set("admin_email", claims.Email)
		c.Set("must_change_password", claims.MustChangePassword)
		c.Next()
	}
}

type LoginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func NewLoginRateLimiter() *LoginRateLimiter {
	return &LoginRateLimiter{attempts: make(map[string][]time.Time)}
}

func (l *LoginRateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()
		windowStart := now.Add(-time.Minute)
		l.mu.Lock()
		recent := l.attempts[ip][:0]
		for _, item := range l.attempts[ip] {
			if item.After(windowStart) {
				recent = append(recent, item)
			}
		}
		if len(recent) >= 5 {
			l.attempts[ip] = recent
			l.mu.Unlock()
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too_many_attempts", "retry_after_seconds": 60})
			return
		}
		recent = append(recent, now)
		l.attempts[ip] = recent
		l.mu.Unlock()
		c.Next()
	}
}

func AdminSecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
}

func AdminCORS(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Content-Type")
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

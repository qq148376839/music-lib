package api

import (
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Recovery returns a Gin middleware that catches panics, logs them with slog,
// and returns a 500 JSON response without exposing internals to the client.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered",
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":    -1,
					"message": "internal server error",
				})
			}
		}()
		c.Next()
	}
}

// SlogLogger returns a Gin middleware that logs each request with slog.
func SlogLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
		)
	}
}

// CORS returns a Gin middleware that sets CORS headers.
// Allowed origins are read from the CORS_ORIGINS environment variable (default: "*").
// Supports comma-separated multiple origins (e.g. "http://192.168.1.1,http://192.168.1.2").
func CORS() gin.HandlerFunc {
	raw := os.Getenv("CORS_ORIGINS")
	if raw == "" {
		raw = "*"
	}
	// Build allowed set. If "*" is present anywhere, allow all.
	allowAll := raw == "*"
	var allowed map[string]struct{}
	if !allowAll {
		parts := strings.Split(raw, ",")
		allowed = make(map[string]struct{}, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "*" {
				allowAll = true
				break
			}
			if p != "" {
				allowed[p] = struct{}{}
			}
		}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if allowAll {
			c.Header("Access-Control-Allow-Origin", "*")
		} else if _, ok := allowed[origin]; ok {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// writeOK writes a successful JSON response: {"code":0,"data":...}
func writeOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
}

// writeError writes a failure JSON response: {"code":-1,"message":...}
func writeError(c *gin.Context, httpCode int, msg string) {
	c.JSON(httpCode, gin.H{"code": -1, "message": msg})
}

// isValidPlatform returns true for the two supported login platforms.
func isValidPlatform(p string) bool {
	return p == "netease" || p == "qq"
}


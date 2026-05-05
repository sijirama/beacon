package middleware

import (
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type rateLimitEntry struct {
	count   int
	expires time.Time
}

type FixedWindowLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]rateLimitEntry
}

func NewFixedWindowLimiter(limit int, window time.Duration) *FixedWindowLimiter {
	return &FixedWindowLimiter{
		limit:   limit,
		window:  window,
		entries: make(map[string]rateLimitEntry),
	}
}

func (l *FixedWindowLimiter) Allow(key string) (bool, time.Duration) {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	for k, entry := range l.entries {
		if now.After(entry.expires) {
			delete(l.entries, k)
		}
	}

	entry, ok := l.entries[key]
	if !ok || now.After(entry.expires) {
		l.entries[key] = rateLimitEntry{
			count:   1,
			expires: now.Add(l.window),
		}
		return true, 0
	}

	if entry.count >= l.limit {
		return false, time.Until(entry.expires)
	}

	entry.count++
	l.entries[key] = entry
	return true, 0
}

func LimitJSON(l *FixedWindowLimiter, keyFn func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := keyFn(c)
		if key == "" {
			key = c.ClientIP()
		}
		ok, retryAfter := l.Allow(key)
		if ok {
			c.Next()
			return
		}
		c.Header("Retry-After", formatRetryAfter(retryAfter))
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error": "rate limit exceeded",
		})
	}
}

func LimitHTML(l *FixedWindowLimiter, keyFn func(*gin.Context) string, message string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := keyFn(c)
		if key == "" {
			key = c.ClientIP()
		}
		ok, retryAfter := l.Allow(key)
		if ok {
			c.Next()
			return
		}
		c.Header("Retry-After", formatRetryAfter(retryAfter))
		c.String(http.StatusTooManyRequests, message)
		c.Abort()
	}
}

func LimitHTMLRedirect(l *FixedWindowLimiter, keyFn func(*gin.Context) string, location, message string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := keyFn(c)
		if key == "" {
			key = c.ClientIP()
		}
		ok, retryAfter := l.Allow(key)
		if ok {
			c.Next()
			return
		}
		c.Header("Retry-After", formatRetryAfter(retryAfter))
		target := location
		if message != "" {
			target = location + "?error=" + url.QueryEscape(message)
		}
		c.Redirect(http.StatusSeeOther, target)
		c.Abort()
	}
}

func formatRetryAfter(d time.Duration) string {
	seconds := int(d.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}

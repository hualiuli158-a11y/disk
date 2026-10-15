package middleware

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"disk/pkg/app"
	"disk/pkg/errcode"

	"github.com/gin-gonic/gin"
)

type rateLimitEntry struct {
	tokens   float64
	updated  time.Time
	lastSeen time.Time
}

// RateLimiter 是进程内令牌桶。多实例部署时，网关仍需提供全局限流。
type RateLimiter struct {
	mu        sync.Mutex
	rate      float64
	burst     float64
	entries   map[string]*rateLimitEntry
	now       func() time.Time
	nextSweep time.Time
}

func NewRateLimiter(perMinute, burst int) *RateLimiter {
	return &RateLimiter{
		rate:    float64(perMinute) / float64(time.Minute),
		burst:   float64(burst),
		entries: make(map[string]*rateLimitEntry),
		now:     time.Now,
	}
}

func (l *RateLimiter) Allow(key string) (bool, time.Duration) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.nextSweep.IsZero() || !now.Before(l.nextSweep) {
		for storedKey, entry := range l.entries {
			if now.Sub(entry.lastSeen) > 10*time.Minute {
				delete(l.entries, storedKey)
			}
		}
		l.nextSweep = now.Add(time.Minute)
	}

	entry := l.entries[key]
	if entry == nil {
		entry = &rateLimitEntry{tokens: l.burst, updated: now}
		l.entries[key] = entry
	}
	elapsed := now.Sub(entry.updated)
	if elapsed > 0 {
		entry.tokens = math.Min(l.burst, entry.tokens+float64(elapsed)*l.rate)
		entry.updated = now
	}
	entry.lastSeen = now
	if entry.tokens >= 1 {
		entry.tokens--
		return true, 0
	}
	if l.rate <= 0 {
		return false, time.Minute
	}
	return false, time.Duration(math.Ceil((1 - entry.tokens) / l.rate))
}

type RateLimitKey func(*gin.Context) (string, bool)

func ByClientIP(scope string) RateLimitKey {
	return func(c *gin.Context) (string, bool) {
		return scope + ":ip:" + c.ClientIP(), true
	}
}

func ByUserID(scope string) RateLimitKey {
	return func(c *gin.Context) (string, bool) {
		value, exists := c.Get("user_id")
		userID, ok := value.(uint64)
		if !exists || !ok || userID == 0 {
			return "", false
		}
		return scope + ":user:" + strconv.FormatUint(userID, 10), true
	}
}

func RateLimit(limiter *RateLimiter, key RateLimitKey) gin.HandlerFunc {
	return func(c *gin.Context) {
		limitKey, ok := key(c)
		if !ok {
			c.Next()
			return
		}
		allowed, retryAfter := limiter.Allow(limitKey)
		if !allowed {
			seconds := int(math.Ceil(retryAfter.Seconds()))
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", strconv.Itoa(seconds))
			app.NewResponse(c).ToErrorResponse(errcode.TooManyRequests)
			c.Abort()
			return
		}
		c.Next()
	}
}

type userConcurrency struct {
	mu     sync.Mutex
	active map[uint64]int
	max    int
}

func ConcurrentUploadsPerUser(max int) gin.HandlerFunc {
	guard := &userConcurrency{active: make(map[uint64]int), max: max}
	return func(c *gin.Context) {
		value, exists := c.Get("user_id")
		userID, ok := value.(uint64)
		if !exists || !ok || userID == 0 {
			c.Next()
			return
		}
		guard.mu.Lock()
		if guard.active[userID] >= guard.max {
			guard.mu.Unlock()
			app.NewResponse(c).ToErrorResponse(errcode.TooManyRequests)
			c.Abort()
			return
		}
		guard.active[userID]++
		guard.mu.Unlock()
		defer func() {
			guard.mu.Lock()
			guard.active[userID]--
			if guard.active[userID] == 0 {
				delete(guard.active, userID)
			}
			guard.mu.Unlock()
		}()
		c.Next()
	}
}

func MaxBodyBytes(maxFileBytes, maxRequestBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if declared, ok := declaredUploadSize(c.Request); ok && declared > maxFileBytes {
			app.NewResponse(c).ToErrorResponse(errcode.ErrFileTooLarge)
			c.Abort()
			return
		}
		if c.Request.ContentLength > maxRequestBytes {
			app.NewResponse(c).ToErrorResponse(errcode.ErrFileTooLarge)
			c.Abort()
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBytes)
		c.Next()
	}
}

func declaredUploadSize(request *http.Request) (int64, bool) {
	value := request.Header.Get("X-Upload-Size")
	if value == "" {
		return 0, false
	}
	size, err := strconv.ParseInt(value, 10, 64)
	return size, err == nil && size >= 0
}

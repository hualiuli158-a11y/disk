package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type observedBody struct {
	reader *bytes.Reader
	reads  atomic.Int32
}

func (b *observedBody) Read(buffer []byte) (int, error) {
	b.reads.Add(1)
	return b.reader.Read(buffer)
}

func (*observedBody) Close() error { return nil }

func TestMaxBodyBytesRejectsKnownOversizeWithoutReading(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := &observedBody{reader: bytes.NewReader(bytes.Repeat([]byte("x"), 20))}
	router := gin.New()
	router.POST("/upload", MaxBodyBytes(8, 16), func(c *gin.Context) {
		_, _ = io.Copy(io.Discard, c.Request.Body)
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/upload", body)
	request.ContentLength = 20
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d, want 413", response.Code)
	}
	if body.reads.Load() != 0 {
		t.Fatalf("oversized body was read %d times", body.reads.Load())
	}
}

func TestRateLimitSeparatesUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewRateLimiter(1, 1)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if value := c.GetHeader("X-Test-User"); value == "one" {
			c.Set("user_id", uint64(1))
		} else {
			c.Set("user_id", uint64(2))
		}
	})
	router.GET("/search", RateLimit(limiter, ByUserID("search")), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := func(user string) int {
		req := httptest.NewRequest(http.MethodGet, "/search", nil)
		req.Header.Set("X-Test-User", user)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response.Code
	}
	if status := request("one"); status != http.StatusNoContent {
		t.Fatalf("first request status=%d", status)
	}
	if status := request("one"); status != http.StatusTooManyRequests {
		t.Fatalf("limited request status=%d", status)
	}
	if status := request("two"); status != http.StatusNoContent {
		t.Fatalf("other user status=%d", status)
	}
}

func TestConcurrentUploadsPerUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	started := make(chan struct{})
	release := make(chan struct{})
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint64(7)) })
	router.POST("/upload", ConcurrentUploadsPerUser(1), func(c *gin.Context) {
		close(started)
		<-release
		c.Status(http.StatusNoContent)
	})

	firstDone := make(chan int, 1)
	go func() {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/upload", nil))
		firstDone <- response.Code
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first upload did not start")
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/upload", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("concurrent upload status=%d, want 429", second.Code)
	}
	close(release)
	select {
	case status := <-firstDone:
		if status != http.StatusNoContent {
			t.Fatalf("first upload status=%d", status)
		}
	case <-time.After(time.Second):
		t.Fatal("first upload did not finish")
	}
}

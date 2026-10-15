package bootstrap

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"disk/pkg/logger"
	"disk/pkg/setting"

	"github.com/gin-gonic/gin"
)

func TestHTTPServerUsesConfiguredLimits(t *testing.T) {
	config := &setting.Config{Server: setting.HttpConfig{
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       4 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       6 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}}
	app := &App{config: config, router: gin.New()}

	server := app.newHTTPServer("127.0.0.1:8080")
	if server.ReadHeaderTimeout != config.Server.ReadHeaderTimeout ||
		server.ReadTimeout != config.Server.ReadTimeout ||
		server.WriteTimeout != config.Server.WriteTimeout ||
		server.IdleTimeout != config.Server.IdleTimeout ||
		server.MaxHeaderBytes != config.Server.MaxHeaderBytes {
		t.Fatalf("HTTP server limits do not match config: %+v", server)
	}
}

func TestHTTPShutdownDrainsInFlightRequestAndStopsAccepting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	started := make(chan struct{})
	release := make(chan struct{})
	router := gin.New()
	router.GET("/slow", func(c *gin.Context) {
		close(started)
		<-release
		c.String(http.StatusOK, "complete")
	})

	config := &setting.Config{Server: setting.HttpConfig{ShutdownTimeout: 2 * time.Second}}
	app := &App{config: config, router: router, logger: logger.NewLogger(io.Discard)}
	server := app.newHTTPServer("")
	serverConnection, clientConnection := net.Pipe()
	listener := newSingleConnectionListener(serverConnection)

	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)
	go func() { serveDone <- app.serveHTTP(ctx, server, listener) }()

	var dialOnce sync.Once
	client := &http.Client{Transport: &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			var connection net.Conn
			dialOnce.Do(func() { connection = clientConnection })
			if connection == nil {
				return nil, net.ErrClosed
			}
			return connection, nil
		},
	}}
	responseDone := make(chan string, 1)
	requestErr := make(chan error, 1)
	go func() {
		response, err := client.Get("http://service.test/slow")
		if err != nil {
			requestErr <- err
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			requestErr <- err
			return
		}
		responseDone <- string(body)
	}()

	select {
	case <-started:
	case err := <-requestErr:
		t.Fatalf("request failed before reaching handler: %v", err)
	case <-time.After(time.Second):
		t.Fatal("request did not reach handler")
	}
	cancel()

	select {
	case <-listener.closed:
	case <-time.After(time.Second):
		t.Fatal("listener was not closed during shutdown")
	}

	close(release)
	select {
	case body := <-responseDone:
		if body != "complete" {
			t.Fatalf("response body=%q", body)
		}
	case err := <-requestErr:
		t.Fatalf("in-flight request failed: %v", err)
	case <-time.After(time.Second):
		t.Fatal("in-flight request did not finish")
	}

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("serveHTTP returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP server did not stop")
	}
}

type singleConnectionListener struct {
	connection net.Conn
	closed     chan struct{}
	closeOnce  sync.Once
	accepted   bool
}

func newSingleConnectionListener(connection net.Conn) *singleConnectionListener {
	return &singleConnectionListener{connection: connection, closed: make(chan struct{})}
}

func (l *singleConnectionListener) Accept() (net.Conn, error) {
	if !l.accepted {
		l.accepted = true
		return l.connection, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}

func (l *singleConnectionListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

func (l *singleConnectionListener) Addr() net.Addr { return pipeAddress("memory") }

type pipeAddress string

func (a pipeAddress) Network() string { return string(a) }
func (a pipeAddress) String() string  { return string(a) }

func TestCleanupShutdownHasIndependentTimeout(t *testing.T) {
	cleanupCanceled := make(chan struct{})
	app := &App{
		config: &setting.Config{Server: setting.HttpConfig{CleanupShutdownTimeout: 20 * time.Millisecond}},
		cleanupCancel: func() {
			select {
			case <-cleanupCanceled:
			default:
				close(cleanupCanceled)
			}
		},
		cleanupDone: make(chan struct{}),
	}

	started := time.Now()
	err := app.stopCleanup()
	if err == nil {
		t.Fatal("expected cleanup shutdown timeout")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("cleanup shutdown took %s", elapsed)
	}
	select {
	case <-cleanupCanceled:
	default:
		t.Fatal("cleanup context was not canceled")
	}
}

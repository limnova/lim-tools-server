package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/limnova/lim-tools-server/internal/config"
	"github.com/limnova/lim-tools-server/internal/handler"
	"github.com/limnova/lim-tools-server/internal/service"
)

func TestPanicRequestIsLogged(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	routes := handler.New(service.NewInfoService("test", "test", "test"), nil)
	srv := New(config.Config{Env: "production"}, logger, routes)
	t.Cleanup(func() { gin.SetMode(gin.TestMode) })
	srv.http.Handler.(*gin.Engine).GET("/boom", func(*gin.Context) { panic("kaboom") })
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	req.Header.Set("X-Request-ID", "panic-test")
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	var found bool
	decoder := json.NewDecoder(&logs)
	for decoder.More() {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			t.Fatal(err)
		}
		if record["msg"] != "request" {
			continue
		}
		found = true
		if record["status"] != float64(http.StatusInternalServerError) || record["request_id"] != "panic-test" {
			t.Errorf("panic access log = %v, want status 500 and request_id panic-test", record)
		}
	}
	if !found {
		t.Fatal("panic request is missing its access log")
	}
}

func TestRunClosesConnectionsOnShutdownTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	finished := make(chan struct{})
	clientDone := make(chan struct{})
	logger := slog.New(slog.DiscardHandler)
	srv := &Server{
		cfg: config.Config{ShutdownTimeout: 20 * time.Millisecond},
		log: logger,
		http: &http.Server{
			Addr: addr,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-r.Context().Done()
				close(finished)
			}),
		},
	}
	t.Cleanup(func() {
		if err := srv.http.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- srv.Run(ctx) }()

	// Retry connection establishment while Run starts its listener.
	client := &http.Client{Timeout: 2 * time.Second}
	go func() {
		defer close(clientDone)
		deadline := time.Now().Add(2 * time.Second)
		for {
			req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr, nil)
			if reqErr != nil {
				t.Error(reqErr)
				return
			}
			response, requestErr := client.Do(req)
			if requestErr == nil {
				if closeErr := response.Body.Close(); closeErr != nil {
					t.Error(closeErr)
				}
				return
			}
			select {
			case <-started:
				return
			default:
			}
			if time.Now().After(deadline) {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not receive request")
	}
	cancel()
	select {
	case err := <-runDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Run() = %v, want shutdown deadline exceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after shutdown timeout")
	}
	select {
	case <-finished:
	case <-time.After(200 * time.Millisecond):
		t.Error("active request context was not canceled after Run returned")
	}
	if err := srv.http.Close(); err != nil {
		t.Error(err)
	}
	<-clientDone
}

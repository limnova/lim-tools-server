package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/limnova/lim-tools-server/internal/handler"
	"github.com/limnova/lim-tools-server/internal/service"
)

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)

	infoSvc := service.NewInfoService("lim-tools-server", "test", "test-version")
	r := gin.New()
	handler.New(infoSvc).Register(r)
	return r
}

func TestHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestInfo(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/info", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got service.Info
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if got.Name != "lim-tools-server" {
		t.Errorf("name = %q, want %q", got.Name, "lim-tools-server")
	}
	if got.Env != "test" {
		t.Errorf("env = %q, want %q", got.Env, "test")
	}
	if got.Version != "test-version" {
		t.Errorf("version = %q, want %q", got.Version, "test-version")
	}
	if got.StartedAt.IsZero() {
		t.Error("startedAt is zero, want the service start time")
	}
}

func TestUnknownRouteReturns404(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

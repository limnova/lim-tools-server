package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/limnova/lim-tools-server/internal/logging"
	"github.com/limnova/lim-tools-server/internal/middleware"
)

func newTestRouter(buf *bytes.Buffer) *gin.Engine {
	gin.SetMode(gin.TestMode)

	logger := slog.New(slog.NewJSONHandler(buf, nil))

	r := gin.New()
	r.Use(
		middleware.Recovery(),
		middleware.RequestID(),
		middleware.WithRequestLogger(logger),
		middleware.AccessLog(),
	)
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	r.GET("/boom", func(c *gin.Context) {
		panic("kaboom")
	})
	return r
}

// logRecords 把缓冲区里的日志按行解析成 JSON，便于断言字段。
func logRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var records []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("解析日志行失败: %v（原文 %q）", err, line)
		}
		records = append(records, record)
	}
	return records
}

func findRecord(t *testing.T, buf *bytes.Buffer, msg string) map[string]any {
	t.Helper()

	for _, record := range logRecords(t, buf) {
		if record["msg"] == msg {
			return record
		}
	}
	t.Fatalf("日志里找不到 msg=%q 的记录（原文 %q）", msg, buf.String())
	return nil
}

func TestRequestIDReusesUpstreamHeader(t *testing.T) {
	var buf bytes.Buffer
	router := newTestRouter(&buf)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(middleware.RequestIDHeader, "upstream-123")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get(middleware.RequestIDHeader); got != "upstream-123" {
		t.Errorf("响应头 %s = %q, want %q", middleware.RequestIDHeader, got, "upstream-123")
	}
	if got := findRecord(t, &buf, "request")["request_id"]; got != "upstream-123" {
		t.Errorf("日志 request_id = %v, want %q", got, "upstream-123")
	}
}

func TestRequestIDGeneratedWhenAbsent(t *testing.T) {
	var buf bytes.Buffer
	router := newTestRouter(&buf)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))

	id := rec.Header().Get(middleware.RequestIDHeader)
	if id == "" {
		t.Fatal("没带上游 ID 时应生成一个，但响应头里是空的")
	}
	if got := findRecord(t, &buf, "request")["request_id"]; got != id {
		t.Errorf("日志 request_id = %v, 响应头 = %q，两者应一致", got, id)
	}
}

func TestAccessLogOmitsClientIP(t *testing.T) {
	var buf bytes.Buffer
	router := newTestRouter(&buf)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))

	record := findRecord(t, &buf, "request")
	for _, key := range []string{"client_ip", "clientIP", "ip", "remote_addr"} {
		if _, found := record[key]; found {
			t.Errorf("访问日志不该记录 %q（属于个人数据），实际值为 %v", key, record[key])
		}
	}
	if record["status"] != float64(http.StatusOK) {
		t.Errorf("status = %v, want %d", record["status"], http.StatusOK)
	}
}

func TestRecoveryLogsPanicAndReturns500(t *testing.T) {
	var buf bytes.Buffer
	router := newTestRouter(&buf)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	record := findRecord(t, &buf, "panic recovered")
	if record["panic"] != "kaboom" {
		t.Errorf("panic = %v, want %q", record["panic"], "kaboom")
	}
	if stack, ok := record["stack"].(string); !ok || stack == "" {
		t.Error("panic 日志里应带堆栈")
	}
	if record["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", record["level"])
	}
}

func TestRequestLoggerPropagatesIntoHandlerContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.WithRequestLogger(logger))
	r.GET("/use-ctx", func(c *gin.Context) {
		// handler 里应当能取到带着 request_id 的 logger
		logging.FromContext(c.Request.Context()).InfoContext(c.Request.Context(), "from handler")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/use-ctx", nil)
	req.Header.Set(middleware.RequestIDHeader, "ctx-42")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if record := findRecord(t, &buf, "from handler"); record["request_id"] != "ctx-42" {
		t.Errorf("handler 里的日志 request_id = %v, want %q", record["request_id"], "ctx-42")
	}
}

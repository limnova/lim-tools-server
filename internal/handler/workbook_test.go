package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/limnova/lim-tools-server/internal/handler"
	"github.com/limnova/lim-tools-server/internal/service"
)

const apiSnapshot = `{"id":"client","name":"表格","sheetOrder":["s1"],"sheets":{"s1":{"id":"s1","name":"工作表1","rowCount":1000,"columnCount":26,"cellData":{"0":{"0":{"v":"hello"}}}}}}`

func workbookRouter(t *testing.T) *gin.Engine {
	t.Helper()
	store, err := service.NewWorkbookService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	router := gin.New()
	handler.New(service.NewInfoService("test", "test", "test"), store).Register(router)
	return router
}

func workbookRequest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestWorkbookHTTPCRUDAndConflict(t *testing.T) {
	router := workbookRouter(t)
	created := workbookRequest(router, http.MethodPost, "/api/v1/workbooks", `{"name":"预算","snapshot":`+apiSnapshot+`}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", created.Code, created.Body)
	}
	var book service.Workbook
	if err := json.Unmarshal(created.Body.Bytes(), &book); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/workbooks/" + book.ID
	get := workbookRequest(router, http.MethodGet, path, "")
	if get.Code != http.StatusOK || get.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("get = %d, headers = %v", get.Code, get.Header())
	}
	input := fmt.Sprintf(`{"name":"重命名","revision":%d,"snapshot":%s}`, book.Revision, apiSnapshot)
	update := workbookRequest(router, http.MethodPut, path, input)
	if update.Code != http.StatusOK || strings.Contains(update.Body.String(), `"snapshot"`) {
		t.Fatalf("update = %d: %s", update.Code, update.Body)
	}
	stale := workbookRequest(router, http.MethodPut, path, input)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), `revision_conflict`) {
		t.Fatalf("stale update = %d: %s", stale.Code, stale.Body)
	}
	staleDelete := workbookRequest(router, http.MethodDelete, path+"?revision=1", "")
	if staleDelete.Code != http.StatusConflict {
		t.Fatalf("stale delete = %d", staleDelete.Code)
	}
	list := workbookRequest(router, http.MethodGet, "/api/v1/workbooks", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "重命名") || strings.Contains(list.Body.String(), `"snapshot"`) {
		t.Fatalf("list = %d: %s", list.Code, list.Body)
	}
	deleted := workbookRequest(router, http.MethodDelete, path+"?revision=2", "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete = %d: %s", deleted.Code, deleted.Body)
	}
	if got := workbookRequest(router, http.MethodGet, path, ""); got.Code != http.StatusNotFound {
		t.Fatalf("get deleted = %d", got.Code)
	}
	if got := workbookRequest(router, http.MethodGet, "/api/v1/workbooks", ""); got.Body.String() != `{"items":[]}` {
		t.Fatalf("empty list = %s", got.Body)
	}
}

func TestWorkbookHTTPInvalidRequests(t *testing.T) {
	router := workbookRouter(t)
	tests := []struct {
		name, method, path, body string
		status                   int
	}{
		{"invalid json", "POST", "/api/v1/workbooks", `{`, 400},
		{"missing snapshot", "POST", "/api/v1/workbooks", `{"name":"表格"}`, 400},
		{"unknown field", "POST", "/api/v1/workbooks", `{"name":"表格","admin":true,"snapshot":` + apiSnapshot + `}`, 400},
		{"trailing json", "POST", "/api/v1/workbooks", `{"name":"表格","snapshot":` + apiSnapshot + `} {}`, 400},
		{"too large", "POST", "/api/v1/workbooks", `{"name":"` + strings.Repeat("a", service.MaxWorkbookBytes) + `"}`, 413},
		{"invalid id", "GET", "/api/v1/workbooks/invalid", "", 404},
		{"missing delete revision", "DELETE", "/api/v1/workbooks/invalid", "", 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := workbookRequest(router, tt.method, tt.path, tt.body)
			if got.Code != tt.status {
				t.Fatalf("status = %d, want %d: %s", got.Code, tt.status, got.Body)
			}
		})
	}
}

func TestWorkbookHTTPRejectsNonJSON(t *testing.T) {
	router := workbookRouter(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workbooks", strings.NewReader(`{"name":"表格","snapshot":`+apiSnapshot+`}`))
	request.Header.Set("Content-Type", "text/plain")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("non-json request status = %d", response.Code)
	}
}

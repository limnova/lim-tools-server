package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/limnova/lim-tools-server/internal/service"
)

const testSnapshot = `{"id":"client-id","name":"client-name","sheetOrder":["sheet1"],"sheets":{"sheet1":{"id":"sheet1","name":"工作表1","rowCount":1000,"columnCount":26,"cellData":{"0":{"0":{"v":"原始内容"},"1":{"f":"=SUM(A2:A3)","s":"header"}}}}},"styles":{"header":{"bl":1}},"resources":[{"name":"filter","data":"{}"}]}`

func newWorkbookStore(t *testing.T) *service.WorkbookService {
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
	return store
}

func createWorkbook(t *testing.T, store *service.WorkbookService) service.Workbook {
	t.Helper()
	book, err := store.Create(t.Context(), "预算", json.RawMessage(testSnapshot))
	if err != nil {
		t.Fatal(err)
	}
	return book
}

func TestWorkbookPersistenceAfterReopen(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	store, err := service.NewWorkbookService(directory)
	if err != nil {
		t.Fatal(err)
	}
	book := createWorkbook(t, store)
	updated := strings.ReplaceAll(testSnapshot, "原始内容", "修改后")
	book, err = store.Update(t.Context(), book.ID, "新名称", book.Revision, json.RawMessage(updated))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := service.NewWorkbookService(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	})
	got, err := reopened.Get(t.Context(), book.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "新名称" || got.Revision != 2 || got.ID != book.ID {
		t.Fatalf("metadata = %+v", got.WorkbookSummary)
	}
	var snapshot struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Resources []any  `json:"resources"`
	}
	if err := json.Unmarshal(got.Snapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.ID != book.ID || snapshot.Name != got.Name || len(snapshot.Resources) != 1 || !strings.Contains(string(got.Snapshot), "修改后") || !strings.Contains(string(got.Snapshot), "=SUM(A2:A3)") {
		t.Fatalf("snapshot was not preserved: %s", got.Snapshot)
	}
	list, err := reopened.List(t.Context())
	if err != nil || len(list) != 1 || list[0].Name != got.Name {
		t.Fatalf("list = %v, err = %v", list, err)
	}
}

func TestWorkbookUpdateConflictAndDelete(t *testing.T) {
	t.Parallel()
	store := newWorkbookStore(t)
	book := createWorkbook(t, store)
	if _, err := store.Update(t.Context(), book.ID, "最新名称", 1, json.RawMessage(testSnapshot)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(t.Context(), book.ID, "过期名称", 1, json.RawMessage(testSnapshot)); !errors.Is(err, service.ErrWorkbookConflict) {
		t.Fatalf("stale update error = %v", err)
	}
	if err := store.Delete(t.Context(), book.ID, 1); !errors.Is(err, service.ErrWorkbookConflict) {
		t.Fatalf("stale delete error = %v", err)
	}
	got, err := store.Get(t.Context(), book.ID)
	if err != nil || got.Name != "最新名称" {
		t.Fatalf("conflict overwrote workbook: %+v, %v", got, err)
	}
	if err := store.Delete(t.Context(), book.ID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(t.Context(), book.ID); !errors.Is(err, service.ErrWorkbookNotFound) {
		t.Fatalf("deleted file error = %v", err)
	}
}

func TestWorkbookConcurrentCompareAndSwap(t *testing.T) {
	t.Parallel()
	store := newWorkbookStore(t)
	book := createWorkbook(t, store)
	const attempts = 12
	results := make(chan error, attempts)
	var group sync.WaitGroup
	for range attempts {
		group.Go(func() {
			_, err := store.Update(t.Context(), book.ID, "并发写入", book.Revision, json.RawMessage(testSnapshot))
			results <- err
		})
	}
	group.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, service.ErrWorkbookConflict) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful writes = %d, want 1", successes)
	}
	got, err := store.Get(t.Context(), book.ID)
	if err != nil || got.Revision != 2 {
		t.Fatalf("revision after concurrent writes = %d, err = %v", got.Revision, err)
	}
}

func TestWorkbookValidation(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, title, snapshot string }{
		{"empty name", " ", testSnapshot},
		{"long name", strings.Repeat("表", 121), testSnapshot},
		{"name with newline", "bad\nname", testSnapshot},
		{"invalid json", "表", `{"sheets":`},
		{"no sheets", "表", `{"sheetOrder":[],"sheets":{}}`},
		{"duplicate id", "表", strings.Replace(testSnapshot, `["sheet1"]`, `["sheet1","sheet1"]`, 1)},
		{"missing sheet", "表", strings.Replace(testSnapshot, `["sheet1"]`, `["missing"]`, 1)},
		{"oversized dimensions", "表", strings.Replace(testSnapshot, `"rowCount":1000`, `"rowCount":100001`, 1)},
		{"negative dimensions", "表", strings.Replace(testSnapshot, `"columnCount":26`, `"columnCount":-1`, 1)},
		{"invalid coordinate", "表", strings.Replace(testSnapshot, `"cellData":{"0"`, `"cellData":{"-1"`, 1)},
		{"primitive cell", "表", strings.Replace(testSnapshot, `{"v":"原始内容"}`, `"text"`, 1)},
		{"object value", "表", strings.Replace(testSnapshot, `{"v":"原始内容"}`, `{"v":{"invalid":true}}`, 1)},
		{"invalid formula", "表", strings.Replace(testSnapshot, `"f":"=SUM(A2:A3)"`, `"f":true`, 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newWorkbookStore(t)
			if _, err := store.Create(t.Context(), tt.title, json.RawMessage(tt.snapshot)); !errors.Is(err, service.ErrInvalidWorkbook) {
				t.Fatalf("validation error = %v", err)
			}
			items, err := store.List(t.Context())
			if err != nil || len(items) != 0 {
				t.Fatalf("invalid request created data: %v, %v", items, err)
			}
		})
	}
}

func TestWorkbookRejectsPathTraversalAndCancelledWrites(t *testing.T) {
	t.Parallel()
	store := newWorkbookStore(t)
	for _, id := range []string{"../outside", "C:/file", "", strings.Repeat("g", 32)} {
		if _, err := store.Get(t.Context(), id); !errors.Is(err, service.ErrWorkbookNotFound) {
			t.Fatalf("unsafe id %q: %v", id, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.Create(ctx, "已取消", json.RawMessage(testSnapshot)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled create error = %v", err)
	}
	items, err := store.List(t.Context())
	if err != nil || len(items) != 0 {
		t.Fatalf("cancelled request created data: %v, %v", items, err)
	}
}

func TestWorkbookCorruptionIsReported(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	store, err := service.NewWorkbookService(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	book := createWorkbook(t, store)
	if err := os.WriteFile(filepath.Join(directory, book.ID+".json"), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(t.Context(), book.ID); err == nil {
		t.Fatal("corrupt file returned a successful response")
	}
	if _, err := store.List(t.Context()); err == nil {
		t.Fatal("list silently omitted a corrupt file")
	}
}

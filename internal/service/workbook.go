package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Workbook limits bound both uploads and the amount of work required to open a snapshot.
const (
	MaxWorkbookBytes = 10 << 20
	MaxWorkbookCells = 200_000
)

var (
	ErrWorkbookNotFound = errors.New("workbook not found")
	ErrWorkbookConflict = errors.New("workbook revision conflict")
	ErrInvalidWorkbook  = errors.New("invalid workbook")
	workbookIDPattern   = regexp.MustCompile(`^[a-f0-9]{32}$`)
)

// WorkbookSummary is the metadata shown in the file list.
type WorkbookSummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Revision  int64     `json:"revision"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Workbook retains the complete engine snapshot, including plugin resources.
type Workbook struct {
	WorkbookSummary
	Snapshot json.RawMessage `json:"snapshot"`
}

// WorkbookService is a durable, single-process store. One process must own the
// directory. The lock deliberately spans the read/check/replace transaction so
// concurrent HTTP requests cannot both commit the same revision.
type WorkbookService struct {
	mu   sync.Mutex
	root *os.Root
}

// NewWorkbookService opens the configured data directory, without database access.
func NewWorkbookService(directory string) (*WorkbookService, error) {
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, fmt.Errorf("create workbook directory: %w", err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open workbook directory: %w", err)
	}
	return &WorkbookService{root: root}, nil
}

// Close releases the directory handle after the HTTP server has stopped.
func (s *WorkbookService) Close() error {
	if err := s.root.Close(); err != nil {
		return fmt.Errorf("close workbook directory: %w", err)
	}
	return nil
}

// List returns metadata ordered by most recent modification.
func (s *WorkbookService) List(ctx context.Context) ([]WorkbookSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := fs.ReadDir(s.root.FS(), ".")
	if err != nil {
		return nil, fmt.Errorf("list workbooks: %w", err)
	}
	items := make([]WorkbookSummary, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || !workbookIDPattern.MatchString(id) {
			continue
		}
		book, err := s.read(id)
		if err != nil {
			return nil, err
		}
		items = append(items, book.WorkbookSummary)
	}
	slices.SortFunc(items, func(a, b WorkbookSummary) int {
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
	return items, nil
}

// Create assigns the server ID and initializes the first revision.
func (s *WorkbookService) Create(ctx context.Context, name string, snapshot json.RawMessage) (Workbook, error) {
	name, err := workbookName(name)
	if err != nil {
		return Workbook{}, err
	}
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return Workbook{}, fmt.Errorf("generate workbook id: %w", err)
	}
	id := hex.EncodeToString(entropy[:])
	snapshot, err = normalizeSnapshot(snapshot, id, name)
	if err != nil {
		return Workbook{}, err
	}
	now := time.Now().UTC()
	book := Workbook{
		WorkbookSummary: WorkbookSummary{ID: id, Name: name, Revision: 1, CreatedAt: now, UpdatedAt: now},
		Snapshot:        snapshot,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Workbook{}, err
	}
	if err := s.write(book); err != nil {
		return Workbook{}, err
	}
	return book, nil
}

// Get loads a complete workbook. Invalid IDs never reach filesystem operations.
func (s *WorkbookService) Get(ctx context.Context, id string) (Workbook, error) {
	if !workbookIDPattern.MatchString(id) {
		return Workbook{}, ErrWorkbookNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Workbook{}, err
	}
	return s.read(id)
}

// Update performs compare-and-swap; a stale revision never replaces newer data.
func (s *WorkbookService) Update(ctx context.Context, id, name string, revision int64, snapshot json.RawMessage) (Workbook, error) {
	if !workbookIDPattern.MatchString(id) {
		return Workbook{}, ErrWorkbookNotFound
	}
	if revision < 1 {
		return Workbook{}, ErrInvalidWorkbook
	}
	name, err := workbookName(name)
	if err != nil {
		return Workbook{}, err
	}
	snapshot, err = normalizeSnapshot(snapshot, id, name)
	if err != nil {
		return Workbook{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Workbook{}, err
	}
	book, err := s.read(id)
	if err != nil {
		return Workbook{}, err
	}
	if book.Revision != revision {
		return Workbook{}, ErrWorkbookConflict
	}
	book.Name, book.Snapshot = name, snapshot
	book.Revision++
	book.UpdatedAt = time.Now().UTC()
	if err := s.write(book); err != nil {
		return Workbook{}, err
	}
	return book, nil
}

// Delete refuses to remove a workbook modified since the caller last read it.
func (s *WorkbookService) Delete(ctx context.Context, id string, revision int64) error {
	if !workbookIDPattern.MatchString(id) {
		return ErrWorkbookNotFound
	}
	if revision < 1 {
		return ErrInvalidWorkbook
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	book, err := s.read(id)
	if err != nil {
		return err
	}
	if book.Revision != revision {
		return ErrWorkbookConflict
	}
	if err := s.root.Remove(id + ".json"); err != nil {
		return fmt.Errorf("delete workbook: %w", err)
	}
	return nil
}

func (s *WorkbookService) read(id string) (Workbook, error) {
	data, err := s.root.ReadFile(id + ".json")
	if errors.Is(err, fs.ErrNotExist) {
		return Workbook{}, ErrWorkbookNotFound
	}
	if err != nil {
		return Workbook{}, fmt.Errorf("read workbook: %w", err)
	}
	var book Workbook
	if err := json.Unmarshal(data, &book); err != nil {
		return Workbook{}, fmt.Errorf("decode stored workbook: %w", err)
	}
	if book.ID != id || book.Revision < 1 || len(book.Snapshot) == 0 {
		return Workbook{}, errors.New("stored workbook metadata is invalid")
	}
	return book, nil
}

func (s *WorkbookService) write(book Workbook) (err error) {
	data, err := json.Marshal(book)
	if err != nil {
		return fmt.Errorf("encode workbook: %w", err)
	}
	// The directory is server configuration, never a client-controlled path.
	file, err := os.CreateTemp(s.root.Name(), ".workbook-*")
	if err != nil {
		return fmt.Errorf("create workbook temporary file: %w", err)
	}
	temporary := filepath.Base(file.Name())
	defer func() {
		if temporary != "" {
			if cleanupErr := s.root.Remove(temporary); cleanupErr != nil && !errors.Is(cleanupErr, fs.ErrNotExist) {
				err = errors.Join(err, fmt.Errorf("remove workbook temporary file: %w", cleanupErr))
			}
		}
	}()
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return fmt.Errorf("write workbook temporary file: %w", err)
	}
	if err := s.root.Rename(temporary, book.ID+".json"); err != nil {
		return fmt.Errorf("replace workbook file: %w", err)
	}
	temporary = ""
	return nil
}

func workbookName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 120 || strings.ContainsAny(name, "\r\n\x00") {
		return "", fmt.Errorf("%w: name must contain 1 to 120 characters", ErrInvalidWorkbook)
	}
	return name, nil
}

func normalizeSnapshot(snapshot json.RawMessage, id, name string) (json.RawMessage, error) {
	if len(snapshot) == 0 || len(snapshot) > MaxWorkbookBytes {
		return nil, fmt.Errorf("%w: snapshot size exceeds limits", ErrInvalidWorkbook)
	}
	var data struct {
		SheetOrder []string                   `json:"sheetOrder"`
		Sheets     map[string]json.RawMessage `json:"sheets"`
	}
	if err := json.Unmarshal(snapshot, &data); err != nil {
		return nil, fmt.Errorf("%w: invalid snapshot json", ErrInvalidWorkbook)
	}
	if len(data.SheetOrder) < 1 || len(data.SheetOrder) > 100 || len(data.Sheets) != len(data.SheetOrder) {
		return nil, fmt.Errorf("%w: invalid sheet order", ErrInvalidWorkbook)
	}
	seenIDs, seenNames := make(map[string]bool), make(map[string]bool)
	cells := 0
	for _, sheetID := range data.SheetOrder {
		if sheetID == "" || len(sheetID) > 128 || seenIDs[sheetID] {
			return nil, fmt.Errorf("%w: invalid sheet id", ErrInvalidWorkbook)
		}
		seenIDs[sheetID] = true
		var sheet struct {
			ID          string                                `json:"id"`
			Name        string                                `json:"name"`
			RowCount    int                                   `json:"rowCount"`
			ColumnCount int                                   `json:"columnCount"`
			CellData    map[string]map[string]json.RawMessage `json:"cellData"`
		}
		if err := json.Unmarshal(data.Sheets[sheetID], &sheet); err != nil {
			return nil, fmt.Errorf("%w: invalid worksheet", ErrInvalidWorkbook)
		}
		if sheet.ID != sheetID || strings.TrimSpace(sheet.Name) == "" || utf8.RuneCountInString(sheet.Name) > 100 || seenNames[sheet.Name] {
			return nil, fmt.Errorf("%w: invalid worksheet name or id", ErrInvalidWorkbook)
		}
		seenNames[sheet.Name] = true
		if sheet.RowCount < 1 || sheet.RowCount > 100_000 || sheet.ColumnCount < 1 || sheet.ColumnCount > 1024 {
			return nil, fmt.Errorf("%w: worksheet dimensions exceed limits", ErrInvalidWorkbook)
		}
		for row, columns := range sheet.CellData {
			if !validCellIndex(row, sheet.RowCount) {
				return nil, fmt.Errorf("%w: invalid row index", ErrInvalidWorkbook)
			}
			for column, cell := range columns {
				cells++
				if cells > MaxWorkbookCells || !validCellIndex(column, sheet.ColumnCount) {
					return nil, fmt.Errorf("%w: cell limit exceeded or invalid column index", ErrInvalidWorkbook)
				}
				if len(cell) == 0 || (cell[0] != '{' && string(cell) != "null") {
					return nil, fmt.Errorf("%w: invalid cell data", ErrInvalidWorkbook)
				}
				var value struct {
					Value   any     `json:"v"`
					Formula *string `json:"f"`
				}
				if err := json.Unmarshal(cell, &value); err != nil {
					return nil, fmt.Errorf("%w: invalid cell value or formula", ErrInvalidWorkbook)
				}
				switch value.Value.(type) {
				case nil, string, float64, bool:
				default:
					return nil, fmt.Errorf("%w: cell value must be text, number or boolean", ErrInvalidWorkbook)
				}
			}
		}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(snapshot, &fields); err != nil {
		return nil, fmt.Errorf("%w: invalid snapshot object", ErrInvalidWorkbook)
	}
	encodedID, err := json.Marshal(id)
	if err != nil {
		return nil, fmt.Errorf("encode workbook id: %w", err)
	}
	encodedName, err := json.Marshal(name)
	if err != nil {
		return nil, fmt.Errorf("encode workbook name: %w", err)
	}
	fields["id"], fields["name"] = encodedID, encodedName
	normalized, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode normalized snapshot: %w", err)
	}
	return normalized, nil
}

func validCellIndex(value string, limit int) bool {
	index, err := strconv.Atoi(value)
	return err == nil && index >= 0 && index < limit && strconv.Itoa(index) == value
}

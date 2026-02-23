package repositories

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestWazzupRepository_UpsertIntegrationByOwner_CreatesWithToken(t *testing.T) {
	h := &wazzupStubHandler{}
	h.queryFn = func(query string, args []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(query, "SELECT id, webhook_token FROM wazzup_integrations"):
			return &wazzupStubRows{columns: []string{"id", "webhook_token"}, rows: nil}, nil
		case strings.Contains(query, "INSERT INTO wazzup_integrations"):
			if len(args) != 6 {
				t.Fatalf("unexpected args count: %d", len(args))
			}
			token, _ := args[3].Value.(string)
			if strings.TrimSpace(token) == "" {
				t.Fatal("expected non-empty generated webhook token")
			}
			return &wazzupStubRows{
				columns: []string{"id", "webhook_token"},
				rows:    [][]driver.Value{{int64(101), token}},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", query)
		}
	}

	db := newWazzupStubDB(t, h)
	repo := NewWazzupRepository(db)

	integrationID, webhookToken, err := repo.UpsertIntegrationByOwner(context.Background(), 7, "enc", "hash", "https://example.com/webhook", true)
	if err != nil {
		t.Fatalf("UpsertIntegrationByOwner returned error: %v", err)
	}
	if integrationID != 101 {
		t.Fatalf("unexpected integrationID: %d", integrationID)
	}
	if strings.TrimSpace(webhookToken) == "" {
		t.Fatal("expected webhook token to be returned")
	}
}

func TestWazzupRepository_UpsertIntegrationByOwner_UniqueViolationFallback(t *testing.T) {
	h := &wazzupStubHandler{}
	insertCalls := 0
	h.queryFn = func(query string, args []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(query, "SELECT id, webhook_token FROM wazzup_integrations"):
			if len(args) == 1 {
				if insertCalls == 0 {
					return &wazzupStubRows{columns: []string{"id", "webhook_token"}, rows: nil}, nil
				}
				return &wazzupStubRows{columns: []string{"id", "webhook_token"}, rows: [][]driver.Value{{int64(55), "existing-token"}}}, nil
			}
			return nil, fmt.Errorf("unexpected select args")
		case strings.Contains(query, "INSERT INTO wazzup_integrations"):
			insertCalls++
			return nil, fmt.Errorf("duplicate key value violates unique constraint")
		default:
			return nil, fmt.Errorf("unexpected query: %s", query)
		}
	}
	h.execFn = func(query string, args []driver.NamedValue) (driver.Result, error) {
		if !strings.Contains(query, "UPDATE wazzup_integrations") {
			return nil, fmt.Errorf("unexpected exec query: %s", query)
		}
		return wazzupStubResult{rowsAffected: 1}, nil
	}

	db := newWazzupStubDB(t, h)
	repo := NewWazzupRepository(db)

	integrationID, webhookToken, err := repo.UpsertIntegrationByOwner(context.Background(), 7, "enc", "hash", "https://example.com/webhook", true)
	if err != nil {
		t.Fatalf("UpsertIntegrationByOwner returned error: %v", err)
	}
	if integrationID != 55 {
		t.Fatalf("unexpected integrationID: %d", integrationID)
	}
	if webhookToken != "existing-token" {
		t.Fatalf("unexpected webhookToken: %s", webhookToken)
	}
}

func TestWazzupRepository_RegisterDedup_FirstTrueSecondFalse(t *testing.T) {
	h := &wazzupStubHandler{dedup: map[string]struct{}{}}
	h.execFn = func(query string, args []driver.NamedValue) (driver.Result, error) {
		if !strings.Contains(query, "INSERT INTO wazzup_dedup") {
			return nil, fmt.Errorf("unexpected exec query: %s", query)
		}
		key := fmt.Sprintf("%v:%v", args[0].Value, args[1].Value)
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.dedup[key]; ok {
			return wazzupStubResult{rowsAffected: 0}, nil
		}
		h.dedup[key] = struct{}{}
		return wazzupStubResult{rowsAffected: 1}, nil
	}

	db := newWazzupStubDB(t, h)
	repo := NewWazzupRepository(db)

	first, err := repo.RegisterDedup(context.Background(), 1, "event-1")
	if err != nil {
		t.Fatalf("first RegisterDedup error: %v", err)
	}
	if !first {
		t.Fatal("first RegisterDedup must be new=true")
	}

	second, err := repo.RegisterDedup(context.Background(), 1, "event-1")
	if err != nil {
		t.Fatalf("second RegisterDedup error: %v", err)
	}
	if second {
		t.Fatal("second RegisterDedup must be new=false")
	}
}

func TestWazzupRepository_CreateLeadFromInbound_SetsPhoneSourceDescription(t *testing.T) {
	h := &wazzupStubHandler{}
	h.queryFn = func(query string, args []driver.NamedValue) (driver.Rows, error) {
		if !strings.Contains(query, "INSERT INTO leads") {
			return nil, fmt.Errorf("unexpected query: %s", query)
		}
		if got, want := args[3].Value, "whatsapp"; got != want {
			t.Fatalf("unexpected source: got=%v want=%v", got, want)
		}
		if got, want := args[2].Value, "77011234567"; got != want {
			t.Fatalf("unexpected phone: got=%v want=%v", got, want)
		}
		if got, want := args[1].Value, "first inbound message"; got != want {
			t.Fatalf("unexpected description: got=%v want=%v", got, want)
		}
		return &wazzupStubRows{columns: []string{"id"}, rows: [][]driver.Value{{int64(222)}}}, nil
	}

	db := newWazzupStubDB(t, h)
	repo := NewWazzupRepository(db)

	leadID, err := repo.CreateLeadFromInbound(context.Background(), 9, " +7 (701) 123-45-67 ", "first inbound message")
	if err != nil {
		t.Fatalf("CreateLeadFromInbound returned error: %v", err)
	}
	if leadID != 222 {
		t.Fatalf("unexpected leadID: %d", leadID)
	}
}

type wazzupStubHandler struct {
	mu      sync.Mutex
	dedup   map[string]struct{}
	queryFn func(query string, args []driver.NamedValue) (driver.Rows, error)
	execFn  func(query string, args []driver.NamedValue) (driver.Result, error)
}

type wazzupStubDriver struct{ h *wazzupStubHandler }

func (d *wazzupStubDriver) Open(string) (driver.Conn, error) {
	return &wazzupStubConn{h: d.h}, nil
}

type wazzupStubConn struct{ h *wazzupStubHandler }

func (c *wazzupStubConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *wazzupStubConn) Close() error                        { return nil }
func (c *wazzupStubConn) Begin() (driver.Tx, error)           { return &wazzupStubTx{}, nil }
func (c *wazzupStubConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &wazzupStubTx{}, nil
}
func (c *wazzupStubConn) CheckNamedValue(*driver.NamedValue) error { return nil }
func (c *wazzupStubConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if c.h.queryFn == nil {
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
	return c.h.queryFn(query, args)
}
func (c *wazzupStubConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.h.execFn == nil {
		return nil, fmt.Errorf("unexpected exec: %s", query)
	}
	return c.h.execFn(query, args)
}

type wazzupStubTx struct{}

func (t *wazzupStubTx) Commit() error   { return nil }
func (t *wazzupStubTx) Rollback() error { return nil }

type wazzupStubRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r *wazzupStubRows) Columns() []string { return r.columns }
func (r *wazzupStubRows) Close() error      { return nil }
func (r *wazzupStubRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

type wazzupStubResult struct{ rowsAffected int64 }

func (r wazzupStubResult) LastInsertId() (int64, error) { return 0, nil }
func (r wazzupStubResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

func newWazzupStubDB(t *testing.T, handler *wazzupStubHandler) *sql.DB {
	t.Helper()
	driverName := fmt.Sprintf("wazzup_stub_%s", strings.ReplaceAll(t.Name(), "/", "_"))
	sql.Register(driverName, &wazzupStubDriver{h: handler})
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

package repositories

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSignatureConfirmationRepository_Approve_EmptyMetaUsesJSONAndSucceeds(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(time.Hour)

	stub := &approveQueryStub{
		t: t,
		rows: [][]driver.Value{{
			"confirmation-id",
			int64(42),
			int64(7),
			"email",
			"approved",
			nil,
			nil,
			int64(0),
			expiresAt,
			now,
			nil,
			[]byte(`{"ip":"127.0.0.1"}`),
		}},
	}

	driverName := fmt.Sprintf("approve_stub_%d", time.Now().UnixNano())
	sql.Register(driverName, &approveStubDriver{stub: stub})

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	repo := NewSignatureConfirmationRepository(db)
	confirmation, err := repo.Approve(context.Background(), "confirmation-id", nil)
	if err != nil {
		t.Fatalf("Approve returned error: %v", err)
	}
	if confirmation == nil {
		t.Fatal("Approve returned nil confirmation")
	}
	if confirmation.Status != "approved" {
		t.Fatalf("unexpected status: %s", confirmation.Status)
	}
	if atomic.LoadInt32(&stub.queryCalled) != 1 {
		t.Fatalf("expected query to be called once, got %d", stub.queryCalled)
	}
}

type approveStubDriver struct {
	stub *approveQueryStub
}

func (d *approveStubDriver) Open(string) (driver.Conn, error) {
	return &approveStubConn{stub: d.stub}, nil
}

type approveStubConn struct {
	stub *approveQueryStub
}

func (c *approveStubConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *approveStubConn) Close() error                        { return nil }
func (c *approveStubConn) Begin() (driver.Tx, error)          { return nil, driver.ErrSkip }

func (c *approveStubConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.stub.query(query, args)
}

func (c *approveStubConn) CheckNamedValue(*driver.NamedValue) error { return nil }

type approveQueryStub struct {
	t          *testing.T
	rows       [][]driver.Value
	queryCalled int32
}

func (s *approveQueryStub) query(query string, args []driver.NamedValue) (driver.Rows, error) {
	s.t.Helper()
	atomic.AddInt32(&s.queryCalled, 1)

	if !strings.Contains(query, "meta = COALESCE(meta, '{}'::jsonb) || COALESCE($2::jsonb, '{}'::jsonb)") {
		return nil, fmt.Errorf("approve query does not cast/merge jsonb meta safely: %s", query)
	}
	if len(args) != 2 {
		return nil, fmt.Errorf("unexpected args count: %d", len(args))
	}
	if args[0].Value != "confirmation-id" {
		return nil, fmt.Errorf("unexpected id arg: %v", args[0].Value)
	}
	metaArg, ok := args[1].Value.(string)
	if !ok {
		return nil, fmt.Errorf("meta arg must be string, got %T", args[1].Value)
	}
	if metaArg == "" {
		return nil, fmt.Errorf("meta arg must not be empty")
	}
	if !json.Valid([]byte(metaArg)) {
		return nil, fmt.Errorf("meta arg must be valid JSON, got %q", metaArg)
	}
	if metaArg != "{}" {
		return nil, fmt.Errorf("expected fallback meta arg {}, got %q", metaArg)
	}

	return &approveStubRows{
		columns: []string{
			"id", "document_id", "user_id", "channel", "status", "otp_hash", "token_hash",
			"attempts", "expires_at", "approved_at", "rejected_at", "meta",
		},
		rows: s.rows,
	}, nil
}

type approveStubRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r *approveStubRows) Columns() []string { return r.columns }
func (r *approveStubRows) Close() error      { return nil }
func (r *approveStubRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

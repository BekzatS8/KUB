package repositories

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSignatureConfirmationRepository_Approve_NilMetaCastsJSONBAndSucceeds(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(time.Hour)

	stub := &signatureConfirmationQueryStub{
		t: t,
		assertQuery: func(query string, args []driver.NamedValue) error {
			if !strings.Contains(query, "WHEN $2::jsonb IS NULL THEN meta") {
				return fmt.Errorf("approve query must cast nil-check to jsonb: %s", query)
			}
			if len(args) != 2 {
				return fmt.Errorf("unexpected args count: %d", len(args))
			}
			if args[0].Value != "confirmation-id" {
				return fmt.Errorf("unexpected id arg: %v", args[0].Value)
			}
			if args[1].Value != nil {
				return fmt.Errorf("meta arg must be nil for nil update, got %T", args[1].Value)
			}
			return nil
		},
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

	repo := newSignatureConfirmationStubRepo(t, stub)
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

func TestSignatureConfirmationRepository_UpdateMeta_NilMetaCastsJSONBAndSucceeds(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(time.Hour)

	stub := &signatureConfirmationQueryStub{
		t: t,
		assertQuery: func(query string, args []driver.NamedValue) error {
			if !strings.Contains(query, "WHEN $2::jsonb IS NULL THEN meta") {
				return fmt.Errorf("update meta query must cast nil-check to jsonb: %s", query)
			}
			if len(args) != 2 {
				return fmt.Errorf("unexpected args count: %d", len(args))
			}
			if args[1].Value != nil {
				return fmt.Errorf("meta arg must be nil for nil update, got %T", args[1].Value)
			}
			return nil
		},
		rows: [][]driver.Value{{
			"confirmation-id",
			int64(42),
			int64(7),
			"email",
			"pending",
			nil,
			nil,
			int64(0),
			expiresAt,
			nil,
			nil,
			[]byte(`{"opened_at":"2025-01-01T00:00:00Z"}`),
		}},
	}

	repo := newSignatureConfirmationStubRepo(t, stub)
	confirmation, err := repo.UpdateMeta(context.Background(), "confirmation-id", nil)
	if err != nil {
		t.Fatalf("UpdateMeta returned error: %v", err)
	}
	if confirmation == nil {
		t.Fatal("UpdateMeta returned nil confirmation")
	}
	if atomic.LoadInt32(&stub.queryCalled) != 1 {
		t.Fatalf("expected query to be called once, got %d", stub.queryCalled)
	}
}

func newSignatureConfirmationStubRepo(t *testing.T, stub *signatureConfirmationQueryStub) *SignatureConfirmationRepository {
	t.Helper()
	driverName := fmt.Sprintf("sign_confirmation_stub_%d", time.Now().UnixNano())
	sql.Register(driverName, &signatureConfirmationStubDriver{stub: stub})

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return NewSignatureConfirmationRepository(db)
}

type signatureConfirmationStubDriver struct {
	stub *signatureConfirmationQueryStub
}

func (d *signatureConfirmationStubDriver) Open(string) (driver.Conn, error) {
	return &signatureConfirmationStubConn{stub: d.stub}, nil
}

type signatureConfirmationStubConn struct {
	stub *signatureConfirmationQueryStub
}

func (c *signatureConfirmationStubConn) Prepare(string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}
func (c *signatureConfirmationStubConn) Close() error              { return nil }
func (c *signatureConfirmationStubConn) Begin() (driver.Tx, error) { return nil, driver.ErrSkip }

func (c *signatureConfirmationStubConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.stub.query(query, args)
}

func (c *signatureConfirmationStubConn) CheckNamedValue(*driver.NamedValue) error { return nil }

type signatureConfirmationQueryStub struct {
	t           *testing.T
	assertQuery func(query string, args []driver.NamedValue) error
	rows        [][]driver.Value
	queryCalled int32
}

func (s *signatureConfirmationQueryStub) query(query string, args []driver.NamedValue) (driver.Rows, error) {
	s.t.Helper()
	atomic.AddInt32(&s.queryCalled, 1)
	if s.assertQuery != nil {
		if err := s.assertQuery(query, args); err != nil {
			return nil, err
		}
	}

	return &signatureConfirmationStubRows{
		columns: []string{
			"id", "document_id", "user_id", "channel", "status", "otp_hash", "token_hash",
			"attempts", "expires_at", "approved_at", "rejected_at", "meta",
		},
		rows: s.rows,
	}, nil
}

type signatureConfirmationStubRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r *signatureConfirmationStubRows) Columns() []string { return r.columns }
func (r *signatureConfirmationStubRows) Close() error      { return nil }
func (r *signatureConfirmationStubRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

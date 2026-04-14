package repositories

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
)

type clientScriptedQueryResponse struct {
	queryContains string
	err           error
	rows          *clientScriptedRows
}

type clientScriptedDriver struct {
	responses []clientScriptedQueryResponse
	idx       int
}

type clientScriptedConn struct{ drv *clientScriptedDriver }

type clientScriptedRows struct {
	columns []string
	data    [][]driver.Value
	idx     int
}

func (d *clientScriptedDriver) Open(string) (driver.Conn, error) {
	return &clientScriptedConn{drv: d}, nil
}

func (c *clientScriptedConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}
func (c *clientScriptedConn) Close() error              { return nil }
func (c *clientScriptedConn) Begin() (driver.Tx, error) { return nil, errors.New("not implemented") }

func (c *clientScriptedConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if c.drv.idx >= len(c.drv.responses) {
		return nil, fmt.Errorf("unexpected query %q", query)
	}
	resp := c.drv.responses[c.drv.idx]
	c.drv.idx++
	if resp.queryContains != "" && !strings.Contains(query, resp.queryContains) {
		return nil, fmt.Errorf("expected query containing %q, got %q", resp.queryContains, query)
	}
	if resp.err != nil {
		return nil, resp.err
	}
	if resp.rows == nil {
		return &clientScriptedRows{}, nil
	}
	copyRows := *resp.rows
	return &copyRows, nil
}

func (c *clientScriptedConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, 0, len(args))
	for i, arg := range args {
		named = append(named, driver.NamedValue{Ordinal: i + 1, Value: arg})
	}
	return c.QueryContext(context.Background(), query, named)
}

func (r *clientScriptedRows) Columns() []string { return r.columns }
func (r *clientScriptedRows) Close() error      { return nil }
func (r *clientScriptedRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.data) {
		return io.EOF
	}
	row := r.data[r.idx]
	r.idx++
	for i := range dest {
		dest[i] = row[i]
	}
	return nil
}

func TestQueryManyWrapsScanError(t *testing.T) {
	drv := &clientScriptedDriver{responses: []clientScriptedQueryResponse{{
		queryContains: "SELECT 1",
		rows: &clientScriptedRows{
			columns: []string{"only_col"},
			data:    [][]driver.Value{{1}},
		},
	}}}
	driverName := "scripted_scan_wrap"
	sql.Register(driverName, drv)
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	repo := NewClientRepository(db)
	_, err = repo.queryMany("SELECT 1")
	if err == nil {
		t.Fatal("expected scan error")
	}
	if !strings.Contains(err.Error(), "scan client row:") {
		t.Fatalf("expected wrapped scan context, got: %v", err)
	}
}

func TestListByOwnerWrapsPrimaryAndFallbackErrors(t *testing.T) {
	drv := &clientScriptedDriver{responses: []clientScriptedQueryResponse{
		{
			queryContains: "client_individual_profiles",
			err:           &pq.Error{Code: pq.ErrorCode(SQLStateUndefinedTable), Message: `relation "client_individual_profiles" does not exist`},
		},
		{
			queryContains: "FROM clients c",
			err:           errors.New("fallback broken"),
		},
	}}
	driverName := "scripted_owner_wrap"
	sql.Register(driverName, drv)
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	repo := NewClientRepository(db)
	_, err = repo.ListByOwner(7, 20, 0, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "list clients by owner legacy fallback:") {
		t.Fatalf("expected fallback context, got: %v", err)
	}
	if !strings.Contains(err.Error(), "fallback broken") {
		t.Fatalf("expected wrapped fallback cause, got: %v", err)
	}
}

func TestListByOwnerSupportsMixedClientTypeRows(t *testing.T) {
	drv := &clientScriptedDriver{responses: []clientScriptedQueryResponse{{
		queryContains: "FROM clients c",
		rows: &clientScriptedRows{
			columns: testClientColumns(),
			data: [][]driver.Value{
				testClientRowValues(101, 7, "individual"),
				testClientRowValues(102, 7, "legal"),
			},
		},
	}}}
	driverName := "scripted_owner_mixed"
	sql.Register(driverName, drv)
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	repo := NewClientRepository(db)
	clients, err := repo.ListByOwner(7, 20, 0, "")
	if err != nil {
		t.Fatalf("ListByOwner returned error: %v", err)
	}
	if len(clients) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(clients))
	}
	if clients[0].ClientType != "individual" {
		t.Fatalf("expected first client type individual, got %q", clients[0].ClientType)
	}
	if clients[1].ClientType != "legal" {
		t.Fatalf("expected second client type legal, got %q", clients[1].ClientType)
	}
	if clients[0].Specialty != "Spec" || clients[0].TrustedPersonPhone != "+70000000009" {
		t.Fatalf("expected new individual fields mapped, got specialty=%q trusted_person_phone=%q", clients[0].Specialty, clients[0].TrustedPersonPhone)
	}
	if clients[0].EducationLevel != "higher" {
		t.Fatalf("expected education_level mapped, got %q", clients[0].EducationLevel)
	}
	if clients[0].IndividualProfile == nil || clients[0].IndividualProfile.VisaRefusals != "No" {
		t.Fatalf("expected nested individual profile with new fields, got %#v", clients[0].IndividualProfile)
	}
}

func TestListByOwnerHandlesNullPrimaryEmail(t *testing.T) {
	row := testClientRowValues(103, 7, "individual")
	row[5] = nil

	drv := &clientScriptedDriver{responses: []clientScriptedQueryResponse{{
		queryContains: "FROM clients c",
		rows: &clientScriptedRows{
			columns: testClientColumns(),
			data:    [][]driver.Value{row},
		},
	}}}
	driverName := "scripted_owner_null_primary_email"
	sql.Register(driverName, drv)
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	repo := NewClientRepository(db)
	clients, err := repo.ListByOwner(7, 20, 0, "")
	if err != nil {
		t.Fatalf("ListByOwner returned error: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
	if clients[0].PrimaryEmail != "" {
		t.Fatalf("expected empty primary_email, got %q", clients[0].PrimaryEmail)
	}
}

func testClientColumns() []string {
	cols := make([]string, 77)
	for i := range cols {
		cols[i] = "c" + strconv.Itoa(i+1)
	}
	return cols
}

func testClientRowValues(id int, ownerID int, clientType string) []driver.Value {
	now := time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC)
	return []driver.Value{
		id, ownerID, clientType, "Display", "+70000000000", "x@example.com", "addr", "contact", now, now, false, nil, nil, "",
		"Last", "First", "Middle", "123456789012", "ID1", "PS", "PN",
		"Reg", "Act", "KZ", "Trip", now, "BirthPlace",
		"Cit", "M", "Single", now, now,
		"Prev", "Spouse", "SpousePhone", true, []byte(`["child"]`),
		"Edu", "Job", "Trips", "Relatives", "Trusted",
		"higher", "Spec", "+70000000009", "DL123", "Uni", "UniAddr", "Manager", "USA,DE", "No",
		int64(180), int64(80), []byte(`["B"]`), "Doctor", "Clinic", "Diseases", "AddInfo",
		"Company", "123456789012", "LLP", "Director", "Contact",
		"Position", "+70000000001", "corp@example.com", "LegalAddr",
		"ActualAddr", "Bank", "IBAN", "BIK", "KBE", "Tax", "Website",
		"Industry", "Size", "LegalInfo",
	}
}

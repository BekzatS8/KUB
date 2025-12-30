package handlers_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"turcompany/internal/authz"
	"turcompany/internal/handlers"
	"turcompany/internal/middleware"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type testEnv struct {
	db           *sql.DB
	router       *gin.Engine
	cleanup      func()
	salesID      int
	sales2ID     int
	managementID int
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err = db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}

	if err = applyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	if err = seedRoles(db); err != nil {
		t.Fatalf("seed roles: %v", err)
	}

	salesID, err := createUser(db, "sales@example.com", authz.RoleSales)
	if err != nil {
		t.Fatalf("create sales user: %v", err)
	}
	sales2ID, err := createUser(db, "sales2@example.com", authz.RoleSales)
	if err != nil {
		t.Fatalf("create sales2 user: %v", err)
	}
	managementID, err := createUser(db, "manager@example.com", authz.RoleManagement)
	if err != nil {
		t.Fatalf("create management user: %v", err)
	}

	clientRepo := repositories.NewClientRepository(db)
	leadRepo := repositories.NewLeadRepository(db)
	dealRepo := repositories.NewDealRepository(db)

	clientSvc := services.NewClientService(clientRepo)
	leadSvc := services.NewLeadService(leadRepo, dealRepo, clientRepo)
	dealSvc := services.NewDealService(dealRepo)
	reportSvc := services.NewReportService(leadRepo, dealRepo)

	clientHandler := handlers.NewClientHandler(clientSvc)
	leadHandler := handlers.NewLeadHandler(leadSvc)
	dealHandler := handlers.NewDealHandler(dealSvc)
	reportHandler := handlers.NewReportHandler(reportSvc)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(testAuthMiddleware(), middleware.ReadOnlyGuard())

	clients := router.Group("/clients")
	{
		clients.POST("", clientHandler.Create)
		clients.GET("", clientHandler.List)
		clients.GET("/my", clientHandler.ListMy)
		clients.PUT("/:id", clientHandler.Update)
		clients.GET("/:id", clientHandler.GetByID)
	}

	leads := router.Group("/leads")
	{
		leads.POST("", leadHandler.Create)
		leads.GET("/:id", leadHandler.GetByID)
		leads.PUT("/:id", leadHandler.Update)
		leads.DELETE("/:id", leadHandler.Delete)
		leads.PUT("/:id/convert", leadHandler.ConvertToDeal)
		leads.GET("", leadHandler.List)
		leads.GET("/my", leadHandler.ListMy)
		leads.POST("/:id/assign", leadHandler.Assign)
		leads.POST("/:id/status", leadHandler.UpdateStatus)
	}

	deals := router.Group("/deals")
	{
		deals.POST("", dealHandler.Create)
		deals.GET("/:id", dealHandler.GetByID)
		deals.PUT("/:id", dealHandler.Update)
		deals.DELETE("/:id", dealHandler.Delete)
		deals.GET("", dealHandler.List)
		deals.GET("/my", dealHandler.ListMy)
		deals.POST("/:id/status", dealHandler.UpdateStatus)
	}

	reports := router.Group("/reports",
		middleware.RequireRoles(
			authz.RoleSales,
			authz.RoleOperations,
			authz.RoleManagement,
			authz.RoleControl,
			authz.RoleAdminStaff,
		),
	)
	{
		reports.GET("/funnel", reportHandler.GetFunnel)
		reports.GET("/leads", reportHandler.GetLeadsSummary)
		reports.GET("/revenue", reportHandler.GetRevenue)
	}

	cleanup := func() {
		_ = db.Close()
	}

	return &testEnv{
		db:           db,
		router:       router,
		cleanup:      cleanup,
		salesID:      salesID,
		sales2ID:     sales2ID,
		managementID: managementID,
	}
}

func testAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := strconv.Atoi(c.GetHeader("X-User-ID"))
		if err != nil || userID == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing user"})
			return
		}
		roleID, err := strconv.Atoi(c.GetHeader("X-Role-ID"))
		if err != nil || roleID == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing role"})
			return
		}
		c.Set("user_id", userID)
		c.Set("role_id", roleID)
		c.Next()
	}
}

func applyMigrations(db *sql.DB) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	migrationsDir := filepath.Join(wd, "..", "..", "db", "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)

	for _, name := range files {
		path := filepath.Join(migrationsDir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err = db.Exec(string(content)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
}

func seedRoles(db *sql.DB) error {
	roles := []struct {
		id   int
		name string
	}{
		{authz.RoleSales, "sales"},
		{authz.RoleOperations, "operations"},
		{authz.RoleControl, "control"},
		{authz.RoleManagement, "management"},
		{authz.RoleAdminStaff, "admin"},
	}
	for _, role := range roles {
		if _, err := db.Exec(
			`INSERT INTO roles (id, name) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
			role.id,
			role.name,
		); err != nil {
			return err
		}
	}
	return nil
}

func createUser(db *sql.DB, email string, roleID int) (int, error) {
	var id int
	err := db.QueryRow(
		`INSERT INTO users (email, password_hash, role_id) VALUES ($1, $2, $3) RETURNING id`,
		email,
		"hash",
		roleID,
	).Scan(&id)
	return id, err
}

func createClient(db *sql.DB, name string, ownerID int) (int, error) {
	var id int
	err := db.QueryRow(
		`INSERT INTO clients (name, owner_id, created_at) VALUES ($1, $2, $3) RETURNING id`,
		name,
		ownerID,
		time.Now(),
	).Scan(&id)
	return id, err
}

func requestJSON(t *testing.T, router http.Handler, method, path string, payload any, userID, roleID int) *httptest.ResponseRecorder {
	t.Helper()
	var body *bytes.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewReader(raw)
	} else {
		body = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", strconv.Itoa(userID))
	req.Header.Set("X-Role-ID", strconv.Itoa(roleID))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestLeadToDealChainAndAccess(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	leadPayload := map[string]any{
		"title":       "Lead A",
		"description": "test",
	}
	leadRes := requestJSON(t, env.router, http.MethodPost, "/leads", leadPayload, env.salesID, authz.RoleSales)
	if leadRes.Code != http.StatusCreated {
		t.Fatalf("lead create status: %d body: %s", leadRes.Code, leadRes.Body.String())
	}
	var lead struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(leadRes.Body.Bytes(), &lead); err != nil {
		t.Fatalf("parse lead: %v", err)
	}

	statusRes := requestJSON(t, env.router, http.MethodPost, fmt.Sprintf("/leads/%d/status", lead.ID), map[string]any{"to": "confirmed"}, env.salesID, authz.RoleSales)
	if statusRes.Code != http.StatusOK {
		t.Fatalf("lead status update: %d body: %s", statusRes.Code, statusRes.Body.String())
	}

	convertPayload := map[string]any{
		"amount":              1250.50,
		"currency":            "USD",
		"client_name":         "Client A",
		"client_bin_iin":      "990011223344",
		"client_address":      "Main st",
		"client_contact_info": "contact",
	}
	convertRes := requestJSON(t, env.router, http.MethodPut, fmt.Sprintf("/leads/%d/convert", lead.ID), convertPayload, env.salesID, authz.RoleSales)
	if convertRes.Code != http.StatusCreated {
		t.Fatalf("lead convert status: %d body: %s", convertRes.Code, convertRes.Body.String())
	}
	var deal struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(convertRes.Body.Bytes(), &deal); err != nil {
		t.Fatalf("parse deal: %v", err)
	}

	convertAgain := requestJSON(t, env.router, http.MethodPut, fmt.Sprintf("/leads/%d/convert", lead.ID), convertPayload, env.salesID, authz.RoleSales)
	if convertAgain.Code != http.StatusConflict {
		t.Fatalf("expected conflict on second convert, got %d body: %s", convertAgain.Code, convertAgain.Body.String())
	}

	var count int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM deals WHERE lead_id = $1`, lead.ID).Scan(&count); err != nil {
		t.Fatalf("count deals: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 deal for lead, got %d", count)
	}

	otherLeadPayload := map[string]any{
		"title":       "Lead B",
		"description": "test",
	}
	leadRes2 := requestJSON(t, env.router, http.MethodPost, "/leads", otherLeadPayload, env.salesID, authz.RoleSales)
	if leadRes2.Code != http.StatusCreated {
		t.Fatalf("lead create status: %d body: %s", leadRes2.Code, leadRes2.Body.String())
	}
	var lead2 struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(leadRes2.Body.Bytes(), &lead2); err != nil {
		t.Fatalf("parse lead2: %v", err)
	}
	statusRes2 := requestJSON(t, env.router, http.MethodPost, fmt.Sprintf("/leads/%d/status", lead2.ID), map[string]any{"to": "confirmed"}, env.salesID, authz.RoleSales)
	if statusRes2.Code != http.StatusOK {
		t.Fatalf("lead status update: %d body: %s", statusRes2.Code, statusRes2.Body.String())
	}

	errCh := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() {
			resp := requestJSON(t, env.router, http.MethodPut, fmt.Sprintf("/leads/%d/convert", lead2.ID), convertPayload, env.salesID, authz.RoleSales)
			errCh <- resp.Code
		}()
	}
	statusCodes := []int{<-errCh, <-errCh}
	sort.Ints(statusCodes)
	if statusCodes[0] != http.StatusConflict || statusCodes[1] != http.StatusCreated {
		t.Fatalf("unexpected parallel convert statuses: %v", statusCodes)
	}
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM deals WHERE lead_id = $1`, lead2.ID).Scan(&count); err != nil {
		t.Fatalf("count deals after parallel: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 deal after parallel convert, got %d", count)
	}

	clientID, err := createClient(env.db, "Client B", env.sales2ID)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	leadOwnerPayload := map[string]any{
		"title":       "Lead C",
		"description": "for management",
		"owner_id":    env.sales2ID,
	}
	leadRes3 := requestJSON(t, env.router, http.MethodPost, "/leads", leadOwnerPayload, env.managementID, authz.RoleManagement)
	if leadRes3.Code != http.StatusCreated {
		t.Fatalf("management lead create: %d body: %s", leadRes3.Code, leadRes3.Body.String())
	}
	var lead3 struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(leadRes3.Body.Bytes(), &lead3); err != nil {
		t.Fatalf("parse lead3: %v", err)
	}

	dealPayload := map[string]any{
		"lead_id":   lead3.ID,
		"client_id": clientID,
		"owner_id":  env.sales2ID,
		"amount":    500,
		"currency":  "USD",
	}
	dealRes := requestJSON(t, env.router, http.MethodPost, "/deals", dealPayload, env.managementID, authz.RoleManagement)
	if dealRes.Code != http.StatusCreated {
		t.Fatalf("deal create: %d body: %s", dealRes.Code, dealRes.Body.String())
	}
	var createdDeal struct {
		ID      int `json:"id"`
		OwnerID int `json:"owner_id"`
	}
	if err := json.Unmarshal(dealRes.Body.Bytes(), &createdDeal); err != nil {
		t.Fatalf("parse created deal: %v", err)
	}
	if createdDeal.OwnerID != env.sales2ID {
		t.Fatalf("expected management to set owner %d, got %d", env.sales2ID, createdDeal.OwnerID)
	}

	leadForbidden := requestJSON(t, env.router, http.MethodGet, fmt.Sprintf("/leads/%d", lead3.ID), nil, env.salesID, authz.RoleSales)
	if leadForbidden.Code != http.StatusForbidden {
		t.Fatalf("sales should not access other lead: %d", leadForbidden.Code)
	}
	clientForbidden := requestJSON(t, env.router, http.MethodGet, fmt.Sprintf("/clients/%d", clientID), nil, env.salesID, authz.RoleSales)
	if clientForbidden.Code != http.StatusForbidden {
		t.Fatalf("sales should not access other client: %d", clientForbidden.Code)
	}
	dealForbidden := requestJSON(t, env.router, http.MethodGet, fmt.Sprintf("/deals/%d", createdDeal.ID), nil, env.salesID, authz.RoleSales)
	if dealForbidden.Code != http.StatusForbidden {
		t.Fatalf("sales should not access other deal: %d", dealForbidden.Code)
	}

	if resp := requestJSON(t, env.router, http.MethodGet, "/clients", nil, env.salesID, authz.RoleSales); resp.Code != http.StatusForbidden {
		t.Fatalf("sales client list expected forbidden, got %d", resp.Code)
	}
	if resp := requestJSON(t, env.router, http.MethodGet, "/leads", nil, env.salesID, authz.RoleSales); resp.Code != http.StatusForbidden {
		t.Fatalf("sales lead list expected forbidden, got %d", resp.Code)
	}
	if resp := requestJSON(t, env.router, http.MethodGet, "/deals", nil, env.salesID, authz.RoleSales); resp.Code != http.StatusForbidden {
		t.Fatalf("sales deal list expected forbidden, got %d", resp.Code)
	}

	updateDealStatus := requestJSON(t, env.router, http.MethodPost, fmt.Sprintf("/deals/%d/status", deal.ID), map[string]any{"to": "won"}, env.salesID, authz.RoleSales)
	if updateDealStatus.Code != http.StatusOK {
		t.Fatalf("deal status update: %d body: %s", updateDealStatus.Code, updateDealStatus.Body.String())
	}

	from := "2000-01-01"
	to := "2100-01-01"
	funnelRes := requestJSON(t, env.router, http.MethodGet, fmt.Sprintf("/reports/funnel?from=%s&to=%s", from, to), nil, env.salesID, authz.RoleSales)
	if funnelRes.Code != http.StatusOK {
		t.Fatalf("funnel report: %d body: %s", funnelRes.Code, funnelRes.Body.String())
	}
	var funnel struct {
		Items []struct {
			Status string `json:"status"`
			Count  int64  `json:"count"`
		} `json:"items"`
	}
	if err := json.Unmarshal(funnelRes.Body.Bytes(), &funnel); err != nil {
		t.Fatalf("parse funnel: %v", err)
	}
	var wonCount int64
	for _, item := range funnel.Items {
		if item.Status == "won" {
			wonCount = item.Count
		}
	}
	if wonCount != 1 {
		t.Fatalf("expected funnel won count 1, got %d", wonCount)
	}

	revenueRes := requestJSON(t, env.router, http.MethodGet, fmt.Sprintf("/reports/revenue?from=%s&to=%s&period=month", from, to), nil, env.salesID, authz.RoleSales)
	if revenueRes.Code != http.StatusOK {
		t.Fatalf("revenue report: %d body: %s", revenueRes.Code, revenueRes.Body.String())
	}
	var revenue struct {
		Items []struct {
			TotalAmount float64 `json:"total_amount"`
		} `json:"items"`
	}
	if err := json.Unmarshal(revenueRes.Body.Bytes(), &revenue); err != nil {
		t.Fatalf("parse revenue: %v", err)
	}
	if len(revenue.Items) == 0 || revenue.Items[0].TotalAmount == 0 {
		t.Fatalf("expected revenue for sales user, got %+v", revenue.Items)
	}
}

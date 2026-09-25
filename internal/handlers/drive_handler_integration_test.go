package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"turcompany/internal/authz"
	"turcompany/internal/middleware"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
	"turcompany/internal/storage"
)

// Сквозной сценарий хранилища через HTTP: загрузка, выдача доступа, просмотр
// получателем, отдача по ссылке с Range, отзыв доступа, удаление.
//
// Нужна живая база (DRIVE_TEST_DSN, см. drive_repository_integration_test.go);
// файлы кладутся на локальный диск во временную папку — код тот же, что и
// для S3, разница только в реализации storage.Storage.
func TestDriveHTTPFlowIntegration(t *testing.T) {
	dsn := os.Getenv("DRIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("DRIVE_TEST_DSN не задан — сквозной тест хранилища пропущен")
	}
	gin.SetMode(gin.TestMode)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var userIDs []int
	defer func() {
		_, _ = db.Exec(`DELETE FROM drive_nodes WHERE name LIKE 'e2e-%'`)
		for _, id := range userIDs {
			_, _ = db.Exec(`DELETE FROM users WHERE id = $1`, id)
		}
	}()
	mkUser := func(email string, role int) int {
		var id int
		if err := db.QueryRow(`INSERT INTO users (email, first_name, last_name, role_id, password_hash, is_active)
			VALUES ($1, 'Е2Е', $1, $2, 'x', TRUE) RETURNING id`, email, role).Scan(&id); err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, id)
		return id
	}
	admin := mkUser("e2e-admin@test.local", authz.RoleSystemAdmin)
	viewer := mkUser("e2e-viewer@test.local", authz.RoleSales)
	qc := mkUser("e2e-qc@test.local", authz.RoleControl) // роль «только чтение»

	storeDir := t.TempDir()
	svc := services.NewDriveService(repositories.NewDriveRepository(db), storage.NewLocalStorage(storeDir),
		services.DriveConfig{LinkSecret: services.DriveLinkSecret([]byte("e2e-secret-e2e-secret-e2e-secret!!"))})
	h := NewDriveHandler(svc)

	// Роутер собран так же, как в routes.go: raw — до авторизации, остальное
	// после; ReadOnlyGuard стоит, как в проде, чтобы поймать блокировку ОКК.
	r := gin.New()
	r.GET("/api/v1/drive/raw/:token", h.Raw)
	r.Use(func(c *gin.Context) {
		uid, _ := strconv.Atoi(c.GetHeader("X-Test-User"))
		role, _ := strconv.Atoi(c.GetHeader("X-Test-Role"))
		c.Set("user_id", uid)
		c.Set("role_id", role)
		c.Next()
	})
	r.Use(middleware.ReadOnlyGuard())
	drive := r.Group("/api/v1/drive")
	drive.GET("/nodes", h.List)
	drive.GET("/nodes/:id/link", h.Link)
	manage := drive.Group("", middleware.RequirePermission("drive.manage", "drive"))
	manage.POST("/folders", h.CreateFolder)
	manage.POST("/files", h.Upload)
	manage.DELETE("/nodes/:id", h.Delete)
	manage.POST("/nodes/:id/shares", h.Share)
	manage.DELETE("/shares/:id", h.Unshare)

	type actor struct{ id, role int }
	as := func(a actor, method, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, body)
		req.Header.Set("X-Test-User", strconv.Itoa(a.id))
		req.Header.Set("X-Test-Role", strconv.Itoa(a.role))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	adminA, viewerA, qcA := actor{admin, authz.RoleSystemAdmin}, actor{viewer, authz.RoleSales}, actor{qc, authz.RoleControl}
	decode := func(t *testing.T, w *httptest.ResponseRecorder, v any) {
		t.Helper()
		if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
			t.Fatalf("decode %s: %v", w.Body.String(), err)
		}
	}
	upload := func(parent int64, name string, content []byte) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.WriteField("parent_id", strconv.FormatInt(parent, 10))
		fw, _ := mw.CreateFormFile("file", name)
		_, _ = fw.Write(content)
		_ = mw.Close()
		return as(adminA, http.MethodPost, "/api/v1/drive/files", &buf, mw.FormDataContentType())
	}

	// 1. Админ создаёт папку и загружает файл.
	w := as(adminA, http.MethodPost, "/api/v1/drive/folders", strings.NewReader(`{"name":"e2e-Договоры"}`), "application/json")
	if w.Code != http.StatusCreated {
		t.Fatalf("create folder: %d %s", w.Code, w.Body.String())
	}
	var folder struct{ ID int64 }
	decode(t, w, &folder)

	content := []byte("%PDF-1.4 e2e content 0123456789")
	w = upload(folder.ID, "Паспорт.pdf", content)
	if w.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	var file struct {
		ID       int64
		Name     string
		MimeType string `json:"mime_type"`
		Preview  string
	}
	decode(t, w, &file)
	if file.MimeType != "application/pdf" || file.Preview != "pdf" {
		t.Fatalf("pdf must be detected by extension, got %+v", file)
	}

	// 2. Дубль имени — переименование, а не ошибка.
	w = upload(folder.ID, "Паспорт.pdf", []byte("second"))
	var dup struct{ Name string }
	decode(t, w, &dup)
	if w.Code != http.StatusCreated || dup.Name != "Паспорт (2).pdf" {
		t.Fatalf("duplicate must be renamed, got %d %+v", w.Code, dup)
	}

	// 3. Без доступа сотрудник ничего не видит, а файл для него «не существует».
	w = as(viewerA, http.MethodGet, "/api/v1/drive/nodes", nil, "")
	var listing struct {
		Items       []struct{ ID int64 }
		Breadcrumbs []struct{ ID int64 }
		CanManage   bool `json:"can_manage"`
		SharedView  bool `json:"shared_view"`
	}
	decode(t, w, &listing)
	if len(listing.Items) != 0 || listing.CanManage || !listing.SharedView {
		t.Fatalf("viewer without shares must see empty shared view, got %+v", listing)
	}
	if w = as(viewerA, http.MethodGet, fmt.Sprintf("/api/v1/drive/nodes/%d/link", file.ID), nil, ""); w.Code != http.StatusNotFound {
		t.Fatalf("viewer without access must get 404 on link, got %d", w.Code)
	}
	if w = as(viewerA, http.MethodPost, "/api/v1/drive/folders", strings.NewReader(`{"name":"e2e-x"}`), "application/json"); w.Code != http.StatusForbidden {
		t.Fatalf("viewer must not create folders, got %d", w.Code)
	}

	// 4. Админ открывает папку сотруднику и ОКК на сутки.
	expires := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	body := fmt.Sprintf(`{"user_ids":[%d,%d],"expires_at":%q}`, viewer, qc, expires)
	w = as(adminA, http.MethodPost, fmt.Sprintf("/api/v1/drive/nodes/%d/shares", folder.ID), strings.NewReader(body), "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("share: %d %s", w.Code, w.Body.String())
	}
	var shares struct {
		Items []struct {
			ID     int64
			UserID int `json:"user_id"`
		}
	}
	decode(t, w, &shares)
	if len(shares.Items) != 2 {
		t.Fatalf("expected 2 shares, got %+v", shares)
	}

	// 5. Сотрудник видит папку в «Доступные мне» и её содержимое.
	decode(t, as(viewerA, http.MethodGet, "/api/v1/drive/nodes", nil, ""), &listing)
	if len(listing.Items) != 1 || listing.Items[0].ID != folder.ID {
		t.Fatalf("viewer must see shared folder, got %+v", listing)
	}
	decode(t, as(viewerA, http.MethodGet, fmt.Sprintf("/api/v1/drive/nodes?parent_id=%d", folder.ID), nil, ""), &listing)
	if len(listing.Items) != 2 || len(listing.Breadcrumbs) != 1 || listing.Breadcrumbs[0].ID != folder.ID {
		t.Fatalf("viewer must see folder content with path from shared root, got %+v", listing)
	}

	// 6. ОКК (роль «только чтение») тоже может открыть файл: ссылка — GET.
	if w = as(qcA, http.MethodGet, fmt.Sprintf("/api/v1/drive/nodes/%d/link", file.ID), nil, ""); w.Code != http.StatusOK {
		t.Fatalf("read-only role must be able to open shared file, got %d %s", w.Code, w.Body.String())
	}

	// 7. Ссылка → содержимое, целиком и по Range.
	w = as(viewerA, http.MethodGet, fmt.Sprintf("/api/v1/drive/nodes/%d/link?disposition=attachment", file.ID), nil, "")
	var link struct{ URL string }
	decode(t, w, &link)
	raw := httptest.NewRecorder()
	r.ServeHTTP(raw, httptest.NewRequest(http.MethodGet, link.URL, nil))
	if raw.Code != http.StatusOK || !bytes.Equal(raw.Body.Bytes(), content) {
		t.Fatalf("raw download: %d %q", raw.Code, raw.Body.String())
	}
	cd := raw.Header().Get("Content-Disposition")
	if !strings.HasPrefix(cd, "attachment") || !strings.Contains(cd, "filename*=utf-8''") {
		t.Fatalf("cyrillic file name must be RFC 5987 encoded, got %q", cd)
	}
	rangeReq := httptest.NewRequest(http.MethodGet, link.URL, nil)
	rangeReq.Header.Set("Range", "bytes=0-7")
	part := httptest.NewRecorder()
	r.ServeHTTP(part, rangeReq)
	if part.Code != http.StatusPartialContent || part.Body.String() != "%PDF-1.4" {
		t.Fatalf("range request must return 206 with 8 bytes, got %d %q", part.Code, part.Body.String())
	}

	// 8. Отзыв доступа убивает уже выданную ссылку сразу, не дожидаясь срока.
	for _, sh := range shares.Items {
		if sh.UserID == viewer {
			if w = as(adminA, http.MethodDelete, fmt.Sprintf("/api/v1/drive/shares/%d", sh.ID), nil, ""); w.Code != http.StatusOK {
				t.Fatalf("unshare: %d", w.Code)
			}
		}
	}
	revoked := httptest.NewRecorder()
	r.ServeHTTP(revoked, httptest.NewRequest(http.MethodGet, link.URL, nil))
	if revoked.Code != http.StatusNotFound {
		t.Fatalf("link must stop working after revoke, got %d", revoked.Code)
	}

	// 9. Удаление папки удаляет и объекты из хранилища.
	countFiles := func() int {
		n := 0
		_ = filepath.Walk(storeDir, func(_ string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				n++
			}
			return nil
		})
		return n
	}
	if before := countFiles(); before != 2 {
		t.Fatalf("expected 2 stored objects before delete, got %d", before)
	}
	w = as(adminA, http.MethodDelete, fmt.Sprintf("/api/v1/drive/nodes/%d", folder.ID), nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if after := countFiles(); after != 0 {
		t.Fatalf("stored objects must be removed with folder, %d left", after)
	}
}

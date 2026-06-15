package handlers

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type DocumentVersionHandler struct {
	docRepo     *repositories.DocumentRepository
	versionRepo *repositories.DocumentVersionRepository
	docService  *services.DocumentService
	filesRoot   string
}

func NewDocumentVersionHandler(
	docRepo *repositories.DocumentRepository,
	versionRepo *repositories.DocumentVersionRepository,
	docService *services.DocumentService,
	filesRoot string,
) *DocumentVersionHandler {
	return &DocumentVersionHandler{
		docRepo:     docRepo,
		versionRepo: versionRepo,
		docService:  docService,
		filesRoot:   filesRoot,
	}
}

// GET /documents/:id/versions
func (h *DocumentVersionHandler) ListVersions(c *gin.Context) {
	docID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || docID <= 0 {
		badRequest(c, "Некорректный ID документа")
		return
	}

	doc, err := h.docRepo.GetByID(docID)
	if err != nil || doc == nil {
		notFound(c, NotFoundCode, "Документ не найден")
		return
	}

	versions, err := h.versionRepo.GetVersions(c.Request.Context(), docID)
	if err != nil {
		internalError(c, "Не удалось загрузить версии")
		return
	}
	if versions == nil {
		versions = []*models.DocumentVersion{}
	}

	c.JSON(200, gin.H{
		"versions": versions,
	})
}

// POST /documents/:id/versions
func (h *DocumentVersionHandler) UploadVersion(c *gin.Context) {
	docID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || docID <= 0 {
		badRequest(c, "Некорректный ID документа")
		return
	}

	doc, err := h.docRepo.GetByID(docID)
	if err != nil || doc == nil {
		notFound(c, NotFoundCode, "Документ не найден")
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		badRequest(c, "Выберите файл для загрузки")
		return
	}

	comment := strings.TrimSpace(c.PostForm("comment"))
	userID, _ := getUserAndRole(c)

	// Get current latest version number
	latestVersion, err := h.versionRepo.GetLatestVersion(c.Request.Context(), docID)
	if err != nil {
		internalError(c, "Не удалось определить текущую версию")
		return
	}
	newVersionNum := latestVersion + 1

	// Save current document file as a version record (before overwriting)
	if doc.FilePath != "" {
		mimeType := detectMimeByPath(doc.FilePath)
		size := int64(0)
		if info, err := os.Stat(h.resolveFile(doc.FilePath)); err == nil {
			size = info.Size()
		}
		versionRecord := &models.DocumentVersion{
			DocumentID:   docID,
			Version:      latestVersion, // save current as old version
			FilePath:     doc.FilePath,
			FilePathPdf:  doc.FilePathPdf,
			FilePathDocx: doc.FilePathDocx,
			FileSize:     &size,
			MimeType:     &mimeType,
			UploadedBy:   &userID,
		}
		if _, err := h.versionRepo.CreateVersion(c.Request.Context(), versionRecord); err != nil {
			internalError(c, "Не удалось сохранить текущую версию")
			return
		}
	}

	// Save new file
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	key := filepath.ToSlash(filepath.Join(
		"versions", strconv.FormatInt(docID, 10),
		fmt.Sprintf("v%d_%d%s", newVersionNum, docID, ext),
	))

	if err := h.saveFileFromHeader(fileHeader, key); err != nil {
		internalError(c, "Не удалось сохранить файл")
		return
	}

	// Update document file paths based on extension
	doc.FilePath = key
	if ext == ".pdf" {
		doc.FilePathPdf = key
		doc.FilePathDocx = ""
	} else if ext == ".docx" {
		doc.FilePathDocx = key
		doc.FilePathPdf = ""
	} else if ext == ".xlsx" {
		doc.FilePathPdf = ""
		doc.FilePathDocx = ""
	}
	if err := h.docRepo.Update(doc); err != nil {
		h.removeStoredFile(key)
		internalError(c, "Не удалось обновить документ")
		return
	}

	// Create version record for the new file
	size := fileHeader.Size
	mimeType := detectMimeByExt(ext)
	newVersionRecord := &models.DocumentVersion{
		DocumentID: docID,
		Version:    newVersionNum,
		FilePath:   key,
		FileSize:   &size,
		MimeType:   &mimeType,
		UploadedBy: &userID,
		Comment:    &comment,
	}
	if ext == ".pdf" {
		newVersionRecord.FilePathPdf = key
	} else if ext == ".docx" {
		newVersionRecord.FilePathDocx = key
	}
	if _, err := h.versionRepo.CreateVersion(c.Request.Context(), newVersionRecord); err != nil {
		h.removeStoredFile(key)
		internalError(c, "Не удалось создать запись версии")
		return
	}

	c.JSON(200, newVersionRecord)
}

// GET /documents/:id/versions/:vid/file
func (h *DocumentVersionHandler) ServeVersionFile(c *gin.Context) {
	docID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || docID <= 0 {
		badRequest(c, "Некорректный ID документа")
		return
	}
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil || vid <= 0 {
		badRequest(c, "Некорректный ID версии")
		return
	}

	version, err := h.versionRepo.GetVersion(c.Request.Context(), vid)
	if err != nil || version == nil || version.DocumentID != docID {
		notFound(c, NotFoundCode, "Версия не найдена")
		return
	}

	filePath := version.FilePath
	if filePath == "" {
		filePath = version.FilePathPdf
	}
	if filePath == "" {
		notFound(c, NotFoundCode, "Файл версии не найден")
		return
	}

	absPath := h.resolveFile(filePath)
	if absPath == "" {
		notFound(c, NotFoundCode, "Файл версии не найден")
		return
	}
	f, err := os.Open(absPath)
	if err != nil {
		notFound(c, NotFoundCode, "Файл версии не найден")
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		notFound(c, NotFoundCode, "Файл версии не найден")
		return
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	c.Header("Content-Type", detectMimeByExt(ext))
	c.Header("Content-Disposition", "inline")
	http.ServeContent(c.Writer, c.Request, filepath.Base(filePath), stat.ModTime(), f)
}

// POST /documents/:id/versions/:vid/restore
func (h *DocumentVersionHandler) RestoreVersion(c *gin.Context) {
	docID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || docID <= 0 {
		badRequest(c, "Некорректный ID документа")
		return
	}
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil || vid <= 0 {
		badRequest(c, "Некорректный ID версии")
		return
	}

	doc, err := h.docRepo.GetByID(docID)
	if err != nil || doc == nil {
		notFound(c, NotFoundCode, "Документ не найден")
		return
	}

	version, err := h.versionRepo.GetVersion(c.Request.Context(), vid)
	if err != nil || version == nil || version.DocumentID != docID {
		notFound(c, NotFoundCode, "Версия не найдена")
		return
	}

	// Save current version before restoring
	latestVersion, err := h.versionRepo.GetLatestVersion(c.Request.Context(), docID)
	if err != nil {
		internalError(c, "Не удалось определить текущую версию")
		return
	}
	userID, _ := getUserAndRole(c)

	if doc.FilePath != "" {
		mimeType := detectMimeByPath(doc.FilePath)
		size := int64(0)
		if info, err := os.Stat(h.resolveFile(doc.FilePath)); err == nil {
			size = info.Size()
		}
		currentAsVersion := &models.DocumentVersion{
			DocumentID:   docID,
			Version:      latestVersion + 1,
			FilePath:     doc.FilePath,
			FilePathPdf:  doc.FilePathPdf,
			FilePathDocx: doc.FilePathDocx,
			FileSize:     &size,
			MimeType:     &mimeType,
			UploadedBy:   &userID,
			Comment:      stringPtr("Текущая версия перед откатом"),
		}
		if _, err := h.versionRepo.CreateVersion(c.Request.Context(), currentAsVersion); err != nil {
			internalError(c, "Не удалось сохранить текущую версию перед откатом")
			return
		}
	}

	// Restore
	doc.FilePath = version.FilePath
	doc.FilePathPdf = version.FilePathPdf
	doc.FilePathDocx = version.FilePathDocx
	if err := h.docRepo.Update(doc); err != nil {
		internalError(c, "Не удалось восстановить документ")
		return
	}

	// Create version record for the restore
	restoreComment := fmt.Sprintf("Восстановление версии v%d", version.Version)
	newVersionRecord := &models.DocumentVersion{
		DocumentID:   docID,
		Version:      latestVersion + 2,
		FilePath:     version.FilePath,
		FilePathPdf:  version.FilePathPdf,
		FilePathDocx: version.FilePathDocx,
		FileSize:     version.FileSize,
		MimeType:     version.MimeType,
		UploadedBy:   &userID,
		Comment:      &restoreComment,
	}
	if _, err := h.versionRepo.CreateVersion(c.Request.Context(), newVersionRecord); err != nil {
		internalError(c, "Не удалось создать запись восстановленной версии")
		return
	}

	c.JSON(200, gin.H{"message": fmt.Sprintf("Документ восстановлен до версии v%d", version.Version)})
}

// --- Helpers ---

func (h *DocumentVersionHandler) resolveFile(rel string) string {
	clean := filepath.Clean(strings.TrimSpace(rel))
	abs := filepath.Join(h.filesRoot, clean)

	// Prevent path traversal: verify resolved path is within filesRoot
	rel2, err := filepath.Rel(h.filesRoot, abs)
	if err != nil || strings.HasPrefix(rel2, "..") || filepath.IsAbs(abs) {
		return "" // invalid path
	}
	return abs
}

func (h *DocumentVersionHandler) saveFileFromHeader(fileHeader *multipart.FileHeader, key string) error {
	absPath := h.resolveFile(key)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	src, err := fileHeader.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(absPath)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

func (h *DocumentVersionHandler) removeStoredFile(key string) {
	if key == "" {
		return
	}
	_ = os.Remove(h.resolveFile(key))
}

func detectMimeByExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".pdf":
		return "application/pdf"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	default:
		return "application/octet-stream"
	}
}

func detectMimeByPath(path string) string {
	ext := filepath.Ext(path)
	return detectMimeByExt(ext)
}

func stringPtr(s string) *string { return &s }

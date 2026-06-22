package services

import (
	"context"
	"fmt"
	"math/rand"
	"mime"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/storage"
)

type clientAccessChecker interface {
	GetByID(id int, userID, roleID int) (*models.Client, error)
}

type clientFileStore interface {
	UpsertPrimary(ctx context.Context, clientID int64, category string, filePath string, mime *string, sizeBytes *int64, uploadedBy *int64) (*models.ClientFile, error)
	GetPrimaryByCategory(ctx context.Context, clientID int64, category string) (*models.ClientFile, error)
}

type ClientFilesService struct {
	RootDir  string
	Clients  clientAccessChecker
	FileRepo clientFileStore
	Store    storage.Storage
}

var individualClientFileCategories = []string{
	"photo35x45",
}

var legalClientFileCategories = []string{
	"photo35x45", // legacy optional compatibility
	"charter",
	"bin_certificate",
	"power_of_attorney",
	"bank_details",
	"director_id",
	"representative_id",
	"signed_contract",
	"corporate_other",
}

func NewClientFilesService(rootDir string, clients clientAccessChecker, fileRepo clientFileStore, store storage.Storage) *ClientFilesService {
	return &ClientFilesService{RootDir: rootDir, Clients: clients, FileRepo: fileRepo, Store: store}
}

func normalizeClientFileCategory(category string) string {
	return strings.ToLower(strings.TrimSpace(category))
}

func allowedClientFileCategories(clientType string) []string {
	if strings.TrimSpace(clientType) == models.ClientTypeLegal {
		return legalClientFileCategories
	}
	return individualClientFileCategories
}

func isAllowedCategoryForClient(clientType, category string) bool {
	category = normalizeClientFileCategory(category)
	for _, allowed := range allowedClientFileCategories(clientType) {
		if category == allowed {
			return true
		}
	}
	return false
}

func categoryAllowsImageOnly(category string) bool {
	return normalizeClientFileCategory(category) == "photo35x45"
}

func isAllowedClientFileExtension(category, ext string) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if categoryAllowsImageOnly(category) {
		return true
	}
	switch ext {
	case ".jpg", ".jpeg", ".png", ".pdf", ".doc", ".docx", ".xls", ".xlsx":
		return true
	default:
		return false
	}
}

func (s *ClientFilesService) UploadPrimary(ctx context.Context, userID, roleID, clientID int, category string, fileHeader *multipart.FileHeader) (*models.ClientFile, error) {
	if fileHeader == nil {
		return nil, ErrFileRequired
	}
	client, err := s.ensureClientAccess(clientID, userID, roleID)
	if err != nil {
		return nil, err
	}
	category = normalizeClientFileCategory(category)
	if !isAllowedCategoryForClient(client.ClientType, category) {
		return nil, ErrUnsupportedClientFileCategory
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !isAllowedClientFileExtension(category, ext) {
		return nil, ErrUnsupportedClientFileExtension
	}

	safeBase := filepath.Base(fileHeader.Filename)
	safeBase = strings.TrimSuffix(safeBase, filepath.Ext(safeBase))
	safeBase = strings.TrimSpace(safeBase)
	if safeBase == "" || safeBase == "." || safeBase == string(filepath.Separator) {
		safeBase = "upload"
	}
	generated := fmt.Sprintf("%s_%d_%08x%s", safeBase, time.Now().UnixNano(), rand.Uint32(), ext)

	relPath := filepath.ToSlash(filepath.Join("clients", fmt.Sprintf("%d", clientID), category, generated))

	src, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("open upload file: %w", err)
	}
	defer src.Close()

	// Detect mime before saving (read first 512 bytes then re-open or use header)
	mimeType := strings.TrimSpace(fileHeader.Header.Get("Content-Type"))

	if err := s.Store.Save(ctx, src, relPath); err != nil {
		return nil, fmt.Errorf("save client file: %w", err)
	}

	if mimeType == "" {
		mimeType = mime.TypeByExtension(ext)
	}
	if mimeType == "" {
		mimeType = mime.TypeByExtension(ext)
	}

	var mimePtr *string
	if mimeType != "" {
		mimePtr = &mimeType
	}
	size := fileHeader.Size
	uploadedBy := int64(userID)

	rec, err := s.FileRepo.UpsertPrimary(ctx, int64(clientID), category, relPath, mimePtr, &size, &uploadedBy)
	if err != nil {
		_ = s.Store.Delete(ctx, relPath)
		return nil, err
	}
	return rec, nil
}

// ResolvePrimaryPath returns the storage key, file name, and MIME type for a client file.
func (s *ClientFilesService) ResolvePrimaryPath(ctx context.Context, userID, roleID, clientID int, category string) (key string, fileName string, mimeType string, err error) {
	if _, err = s.ensureClientAccess(clientID, userID, roleID); err != nil {
		return "", "", "", err
	}
	category = normalizeClientFileCategory(category)
	rec, err := s.FileRepo.GetPrimaryByCategory(ctx, int64(clientID), category)
	if err != nil {
		return "", "", "", err
	}

	key = filepath.ToSlash(rec.FilePath)
	fileName = filepath.Base(key)
	if rec.Mime != nil {
		mimeType = *rec.Mime
	} else {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(fileName)))
	}
	return key, fileName, mimeType, nil
}

func (s *ClientFilesService) ensureClientAccess(clientID, userID, roleID int) (*models.Client, error) {
	client, err := s.Clients.GetByID(clientID, userID, roleID)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, repositories.ErrClientNotFound
	}
	return client, nil
}


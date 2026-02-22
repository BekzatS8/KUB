package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
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
}

func NewClientFilesService(rootDir string, clients clientAccessChecker, fileRepo clientFileStore) *ClientFilesService {
	return &ClientFilesService{RootDir: rootDir, Clients: clients, FileRepo: fileRepo}
}

func (s *ClientFilesService) UploadPrimary(ctx context.Context, userID, roleID, clientID int, category string, fileHeader *multipart.FileHeader) (*models.ClientFile, error) {
	if fileHeader == nil {
		return nil, ErrFileRequired
	}
	if err := s.ensureClientAccess(clientID, userID, roleID); err != nil {
		return nil, err
	}
	if category != "photo35x45" {
		return nil, ErrUnsupportedClientFileCategory
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
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
	absPath, err := s.safeAbsPath(relPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, fmt.Errorf("create client file directory: %w", err)
	}

	src, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("open upload file: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(absPath)
	if err != nil {
		return nil, fmt.Errorf("create client file: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(absPath)
		return nil, fmt.Errorf("write client file: %w", err)
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(absPath)
		return nil, fmt.Errorf("close client file: %w", err)
	}

	mimeType := strings.TrimSpace(fileHeader.Header.Get("Content-Type"))
	if mimeType == "" {
		if detected, detectErr := detectMimeFromFile(absPath); detectErr == nil {
			mimeType = detected
		}
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
		_ = os.Remove(absPath)
		return nil, err
	}
	return rec, nil
}

func (s *ClientFilesService) ResolvePrimaryPath(ctx context.Context, userID, roleID, clientID int, category string) (absPath string, fileName string, mimeType string, err error) {
	if err = s.ensureClientAccess(clientID, userID, roleID); err != nil {
		return "", "", "", err
	}
	rec, err := s.FileRepo.GetPrimaryByCategory(ctx, int64(clientID), category)
	if err != nil {
		return "", "", "", err
	}

	absPath, err = s.safeAbsPath(rec.FilePath)
	if err != nil {
		return "", "", "", err
	}
	if _, statErr := os.Stat(absPath); statErr != nil {
		return "", "", "", statErr
	}

	fileName = filepath.Base(rec.FilePath)
	if rec.Mime != nil {
		mimeType = *rec.Mime
	} else {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(fileName)))
	}
	return absPath, fileName, mimeType, nil
}

func (s *ClientFilesService) ensureClientAccess(clientID, userID, roleID int) error {
	client, err := s.Clients.GetByID(clientID, userID, roleID)
	if err != nil {
		return err
	}
	if client == nil {
		return repositories.ErrClientNotFound
	}
	return nil
}

func (s *ClientFilesService) safeAbsPath(rel string) (string, error) {
	root := filepath.Clean(s.RootDir)
	joined := filepath.Clean(filepath.Join(root, rel))
	relToRoot, err := filepath.Rel(root, joined)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(relToRoot, "..") || filepath.IsAbs(relToRoot) {
		return "", ErrClientFilePathTraversal
	}
	return joined, nil
}

func detectMimeFromFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	return http.DetectContentType(buf[:n]), nil
}

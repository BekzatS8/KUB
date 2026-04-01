package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"turcompany/internal/repositories"
)

var (
	ErrPublicSignInvalidToken  = errors.New("invalid public sign token")
	ErrPublicSignInvalidStatus = errors.New("invalid document status")
	ErrPublicSignInvalidInput  = errors.New("invalid public sign input")
)

type PublicDocumentSignPayload struct {
	SignerName  string `json:"signer_name"`
	SignerEmail string `json:"signer_email"`
	SignerPhone string `json:"signer_phone"`
	Signature   string `json:"signature"`
}

type PublicDocumentDTO struct {
	ID           int64   `json:"id"`
	DocType      string  `json:"doc_type"`
	Status       string  `json:"status"`
	FilePathDocx *string `json:"file_path_docx"`
	FilePathPDF  *string `json:"file_path_pdf"`
}

type PublicDocumentSigningConfig struct {
	BaseURL     string
	TokenPepper string
	TTLMinutes  int
	ServerTZ    *time.Location
}

type PublicDocumentSigningService struct {
	links      *repositories.PublicDocumentLinkRepository
	docService *DocumentService
	docRepo    repositories.DocumentRepository
	baseURL    string
	pepper     string
	ttlMinutes int
	serverTZ   *time.Location
	now        func() time.Time
}

func NewPublicDocumentSigningService(
	links *repositories.PublicDocumentLinkRepository,
	docService *DocumentService,
	docRepo *repositories.DocumentRepository,
	cfg PublicDocumentSigningConfig,
	now func() time.Time,
) *PublicDocumentSigningService {
	if now == nil {
		now = time.Now
	}
	ttl := cfg.TTLMinutes
	if ttl <= 0 {
		ttl = 60
	}
	serverTZ := cfg.ServerTZ
	if serverTZ == nil {
		serverTZ = time.UTC
	}
	return &PublicDocumentSigningService{
		links:      links,
		docService: docService,
		docRepo:    *docRepo,
		baseURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		pepper:     strings.TrimSpace(cfg.TokenPepper),
		ttlMinutes: ttl,
		serverTZ:   serverTZ,
		now:        now,
	}
}

func (s *PublicDocumentSigningService) GenerateSignLink(ctx context.Context, docID int64, userID, roleID, ttlMinutes int) (string, time.Time, error) {
	if s.docService == nil || s.links == nil {
		return "", time.Time{}, errors.New("service unavailable")
	}
	doc, err := s.docService.GetDocument(docID, userID, roleID)
	if err != nil || doc == nil {
		return "", time.Time{}, repositories.ErrPublicLinkNotFound
	}
	if doc.Status != "approved" {
		return "", time.Time{}, ErrPublicSignInvalidStatus
	}
	ttl := s.ttlMinutes
	if ttlMinutes > 0 {
		ttl = ttlMinutes
	}
	rawToken, tokenHash, err := generatePublicToken(s.pepper)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := s.now().Add(time.Duration(ttl) * time.Minute)
	nowUTC := s.now().UTC()
	log.Printf("[sign][public][link_created] document_id=%d server_tz=%s now_utc=%s now_local=%s expires_at=%s ttl_minutes=%d reason=public_link_created",
		docID,
		s.serverTZ.String(),
		nowUTC.Format(time.RFC3339Nano),
		nowUTC.In(s.serverTZ).Format(time.RFC3339Nano),
		expiresAt.UTC().Format(time.RFC3339Nano),
		ttl,
	)
	creator := int64(userID)
	if _, err := s.links.CreateLink(ctx, docID, &creator, tokenHash, expiresAt); err != nil {
		return "", time.Time{}, err
	}
	if s.baseURL == "" {
		return "", time.Time{}, errors.New("public base url required")
	}
	return s.baseURL + "/public/documents/" + rawToken, expiresAt, nil
}

func (s *PublicDocumentSigningService) GetPublicDocument(ctx context.Context, rawToken string) (*PublicDocumentDTO, time.Time, error) {
	if strings.TrimSpace(rawToken) == "" {
		return nil, time.Time{}, ErrPublicSignInvalidToken
	}
	tokenHash := hashPublicTokenWithPepper(rawToken, s.pepper)
	link, err := s.links.FindActiveByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, time.Time{}, err
	}
	doc, err := s.docRepo.GetByID(link.DocumentID)
	if err != nil || doc == nil {
		return nil, time.Time{}, repositories.ErrPublicLinkNotFound
	}
	res := &PublicDocumentDTO{ID: doc.ID, DocType: doc.DocType, Status: doc.Status}
	if strings.TrimSpace(doc.FilePathDocx) != "" {
		v := doc.FilePathDocx
		res.FilePathDocx = &v
	}
	if strings.TrimSpace(doc.FilePathPdf) != "" {
		v := doc.FilePathPdf
		res.FilePathPDF = &v
	}
	return res, link.ExpiresAt, nil
}

func (s *PublicDocumentSigningService) SignPublicDocument(ctx context.Context, rawToken string, payload PublicDocumentSignPayload, ip, ua string) (time.Time, string, int64, error) {
	if strings.TrimSpace(payload.SignerName) == "" || strings.TrimSpace(payload.Signature) == "" {
		return time.Time{}, "", 0, ErrPublicSignInvalidInput
	}
	tx, err := s.links.BeginTx(ctx)
	if err != nil {
		return time.Time{}, "", 0, err
	}
	defer tx.Rollback()

	tokenHash := hashPublicTokenWithPepper(rawToken, s.pepper)
	link, err := s.links.FindActiveByTokenHashForUpdate(ctx, tx, tokenHash)
	if err != nil {
		return time.Time{}, "", 0, err
	}
	doc, err := s.docRepo.GetByID(link.DocumentID)
	if err != nil || doc == nil {
		return time.Time{}, "", 0, repositories.ErrPublicLinkNotFound
	}
	if doc.Status == "signed" {
		return time.Time{}, "", 0, repositories.ErrPublicLinkUsed
	}
	if doc.Status != "approved" {
		return time.Time{}, "", 0, ErrPublicSignInvalidStatus
	}
	eventID, err := newUUID()
	if err != nil {
		return time.Time{}, "", 0, err
	}
	metaRaw, _ := json.Marshal(map[string]any{
		"signed_by": "client",
		"event_id":  eventID,
		"signer": map[string]any{
			"name":  payload.SignerName,
			"email": payload.SignerEmail,
			"phone": payload.SignerPhone,
		},
	})
	signedAt, createdEventID, err := s.links.InsertSignatureTx(ctx, tx, repositories.PublicDocumentSignatureInsert{
		DocumentID:  link.DocumentID,
		LinkID:      link.ID,
		SignerName:  strings.TrimSpace(payload.SignerName),
		SignerEmail: strings.TrimSpace(payload.SignerEmail),
		SignerPhone: strings.TrimSpace(payload.SignerPhone),
		Signature:   strings.TrimSpace(payload.Signature),
		IP:          strings.TrimSpace(ip),
		UserAgent:   strings.TrimSpace(ua),
		EventID:     eventID,
		MetaJSON:    string(metaRaw),
	})
	if err != nil {
		return time.Time{}, "", 0, err
	}
	if err := s.links.MarkUsedTx(ctx, tx, link.ID); err != nil {
		return time.Time{}, "", 0, err
	}
	if err := tx.Commit(); err != nil {
		return time.Time{}, "", 0, err
	}
	if err := s.docService.FinalizeSigning(link.DocumentID); err != nil {
		return time.Time{}, "", 0, err
	}
	if err := s.docRepo.UpdateSigningMeta(link.DocumentID, "public_link", "", "", string(metaRaw)); err != nil {
		return time.Time{}, "", 0, err
	}
	return signedAt, createdEventID, link.DocumentID, nil
}

func (s *PublicDocumentSigningService) TokenPrefixForLog(raw string) string {
	token := strings.TrimSpace(raw)
	if len(token) > 8 {
		return token[:8]
	}
	return token
}

func (s *PublicDocumentSigningService) TokenHashPrefixForLog(raw string) string {
	hash := hashPublicTokenWithPepper(raw, s.pepper)
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

func generatePublicToken(pepper string) (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("token rand: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, hashPublicTokenWithPepper(token, pepper), nil
}

func hashPublicTokenWithPepper(token, pepper string) string {
	token = strings.TrimSpace(token)
	if pepper = strings.TrimSpace(pepper); pepper != "" {
		token = token + ":" + pepper
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("uuid rand: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

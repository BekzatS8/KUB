package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/handlers"
	"turcompany/internal/middleware"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type publicVerifyRepoStub struct{}

func (s *publicVerifyRepoStub) CreatePending(context.Context, int64, int64, string, *string, *string, time.Time, []byte) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *publicVerifyRepoStub) FindPending(context.Context, int64, int64, string) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *publicVerifyRepoStub) FindPendingByTokenHash(context.Context, string, string) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *publicVerifyRepoStub) FindByTokenHash(context.Context, string, string) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *publicVerifyRepoStub) Approve(context.Context, string, []byte) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *publicVerifyRepoStub) Reject(context.Context, string, []byte) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *publicVerifyRepoStub) CancelPrevious(context.Context, int64, int64, string) (int64, error) {
	return 0, nil
}
func (s *publicVerifyRepoStub) IncrementAttempts(context.Context, string) (int, error) { return 0, nil }
func (s *publicVerifyRepoStub) Expire(context.Context, string) error                   { return nil }
func (s *publicVerifyRepoStub) HasApproved(context.Context, int64, int64, string) (bool, error) {
	return false, nil
}
func (s *publicVerifyRepoStub) GetLatestByChannel(context.Context, int64, int64, string) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *publicVerifyRepoStub) UpdateMeta(context.Context, string, []byte) (*models.SignatureConfirmation, error) {
	return nil, nil
}

type publicVerifyDocStub struct{}

func (s *publicVerifyDocStub) GetByID(int64) (*models.Document, error) { return nil, nil }

func TestSetupRoutes_PublicSigningVerifyAPIWithoutAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := services.NewDocumentSigningConfirmationService(
		&publicVerifyRepoStub{},
		nil,
		&publicVerifyDocStub{},
		nil,
		nil,
		nil,
		services.DocumentSigningConfirmationConfig{EmailTTL: 30 * time.Minute},
		time.Now,
	)
	signConfirmHandler := handlers.NewDocumentSigningConfirmationHandler(svc, nil, nil, "")
	publicSigningUIHandler, err := handlers.NewPublicSigningUIHandler()
	if err != nil {
		t.Fatalf("NewPublicSigningUIHandler error: %v", err)
	}

	r := gin.New()
	SetupRoutes(
		r,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		signConfirmHandler,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		publicSigningUIHandler,
		nil,
		middleware.NewAuthMiddleware([]byte("test-secret")),
	)

	apiReq := httptest.NewRequest(http.MethodGet, "/api/v1/sign/email/verify?token=bad-token&format=json", nil)
	apiW := httptest.NewRecorder()
	r.ServeHTTP(apiW, apiReq)
	if apiW.Code == http.StatusUnauthorized {
		t.Fatalf("verify api should be public, got 401: body=%s", apiW.Body.String())
	}
	previewReq := httptest.NewRequest(http.MethodGet, "/api/v1/sign/email/preview?token=bad-token", nil)
	previewW := httptest.NewRecorder()
	r.ServeHTTP(previewW, previewReq)
	if previewW.Code == http.StatusUnauthorized {
		t.Fatalf("preview api should be public, got 401: body=%s", previewW.Body.String())
	}

	pageReq := httptest.NewRequest(http.MethodGet, "/sign/email/verify?token=abc123", nil)
	pageW := httptest.NewRecorder()
	r.ServeHTTP(pageW, pageReq)
	if pageW.Code != http.StatusOK {
		t.Fatalf("unexpected status for signing page: got=%d want=%d", pageW.Code, http.StatusOK)
	}

	favReq := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	favW := httptest.NewRecorder()
	r.ServeHTTP(favW, favReq)
	if favW.Code != http.StatusNoContent {
		t.Fatalf("unexpected favicon status: got=%d want=%d", favW.Code, http.StatusNoContent)
	}
}

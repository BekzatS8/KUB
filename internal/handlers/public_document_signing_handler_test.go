package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"turcompany/internal/repositories"
)

func TestHandlePublicDocError_StatusMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "not found", err: repositories.ErrPublicLinkNotFound, wantStatus: http.StatusNotFound, wantCode: DocumentNotFound},
		{name: "used", err: repositories.ErrPublicLinkUsed, wantStatus: http.StatusConflict, wantCode: ConflictCode},
		{name: "expired", err: repositories.ErrPublicLinkExpired, wantStatus: http.StatusGone, wantCode: ExpiredCode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			handlePublicDocError(ctx, tc.err)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d", rec.Code, tc.wantStatus)
			}
			var payload APIError
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if payload.ErrorCode != tc.wantCode {
				t.Fatalf("error_code=%s want=%s", payload.ErrorCode, tc.wantCode)
			}
		})
	}
}

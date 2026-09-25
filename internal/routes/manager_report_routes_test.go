package routes

import (
	"testing"

	"github.com/gin-gonic/gin"

	"turcompany/internal/handlers"
)

// Порядок вкладок «Мои отчёты» живёт на PUT /reports/table/my/order рядом с
// PUT /reports/table/my/:id. Статический сегмент и wildcard на одном уровне —
// классическое место, где роутер падает при старте, поэтому проверяем, что
// маршруты регистрируются и резолвятся по отдельности.
func TestReportTableRoutes_OrderDoesNotShadowReportID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetupRoutes(r,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		handlers.NewManagerReportHandler(nil),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, // driveHandler
		func(c *gin.Context) { c.Next() },
	)

	want := map[string]bool{
		"PUT /reports/table/my/order": false,
		"PUT /reports/table/my/:id":   false,
		"GET /reports/table/my":       false,
	}
	for _, route := range r.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Fatalf("route %q is not registered", key)
		}
	}
}

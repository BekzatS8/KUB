package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	wz "turcompany/internal/integrations/wazzup"
)

// Переавторизация сломанного канала Instagram у Wazzup падает с 500 —
// отдаём окно добавления нового канала (пустой channelId по документации).
func TestInstagramConnectLink_FallsBackToNewChannelOn5xx(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		if r.URL.Path != "/iframe/generate-channels-link" || r.Header.Get("Authorization") != "Bearer child-key" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.Contains(string(b), "channelId") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"statusCode":500,"message":"Internal server error"}`))
			return
		}
		_, _ = w.Write([]byte(`{"link":"https://app.wazzup24.com/x/tech-partner/channel?token=t"}`))
	}))
	defer srv.Close()

	h := newInstagramTestHandler(&stubWazzupService{})
	h.SetWhiteLabel(wz.NewWhiteLabelClient(wz.WhiteLabelConfig{BaseURL: srv.URL, APIKey: "child-key"}, 5*time.Second))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", func(c *gin.Context) {
		c.Set("user_id", 1)
		c.Set("role_id", authz.RoleSystemAdmin)
		h.InstagramConnectLink(c)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x?channel_id=35778b22-73f9-433e-b4c8-2257794d34b2", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Link       string `json:"link"`
		NewChannel bool   `json:"new_channel"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Link == "" || !resp.NewChannel {
		t.Fatalf("ждали ссылку на новый канал: %s", w.Body.String())
	}
	if len(bodies) != 2 || !strings.Contains(bodies[0], `"channelId":"35778b22`) || strings.Contains(bodies[1], "channelId") ||
		!strings.Contains(bodies[1], `"transport":"instAPI"`) {
		t.Fatalf("неожиданные запросы к Wazzup: %v", bodies)
	}
}

// Ошибка не 5xx (например, 400) — не подменяем переподключение новым каналом.
func TestInstagramConnectLink_NoFallbackOn4xx(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad channel"}`))
	}))
	defer srv.Close()
	h := newInstagramTestHandler(&stubWazzupService{})
	h.SetWhiteLabel(wz.NewWhiteLabelClient(wz.WhiteLabelConfig{BaseURL: srv.URL, APIKey: "k"}, 5*time.Second))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", func(c *gin.Context) {
		c.Set("role_id", authz.RoleSystemAdmin)
		h.InstagramConnectLink(c)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x?channel_id=abc", nil))
	if w.Code != http.StatusBadGateway || calls != 1 {
		t.Fatalf("status=%d calls=%d", w.Code, calls)
	}
}

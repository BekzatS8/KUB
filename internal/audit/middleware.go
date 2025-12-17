package audit

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/services"
)

func AuditMiddleware(svc *services.AuditService) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next() // выполняем запрос

		// логируем только мутирующие запросы
		switch c.Request.Method {
		case "GET", "HEAD", "OPTIONS":
			return
		}
		if svc == nil {
			return
		}

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		actorID := actorFromContext(c)
		ip := c.ClientIP()
		ua := c.GetHeader("User-Agent")

		svc.Log(c.Request.Context(), services.AuditEvent{
			ActorUserID: actorID,
			Action:      fmt.Sprintf("http.%s %s", c.Request.Method, path),
			EntityType:  "http",
			EntityID:    "",
			IP:          &ip,
			UserAgent:   &ua,
			Meta: map[string]any{
				"status":     c.Writer.Status(),
				"durationMs": time.Since(start).Milliseconds(),
			},
		})
	}
}

func actorFromContext(c *gin.Context) *int {
	// Подстройка под твой проект: AuthMiddleware обычно кладёт user_id / userID
	keys := []string{"user_id", "userID", "userId", "uid"}
	for _, k := range keys {
		v, ok := c.Get(k)
		if !ok {
			continue
		}
		switch t := v.(type) {
		case int:
			if t > 0 {
				x := t
				return &x
			}
		case int64:
			if t > 0 {
				x := int(t)
				return &x
			}
		case float64:
			if t > 0 {
				x := int(t)
				return &x
			}
		case string:
			if n, err := strconv.Atoi(t); err == nil && n > 0 {
				x := n
				return &x
			}
		}
	}
	return nil
}

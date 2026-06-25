package middleware

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
)

// reDocAction matches per-document action endpoints like
// /documents/123/send-for-signature or /documents/123/archive. The numeric id
// segment distinguishes them from content-creation endpoints
// (/documents, /documents/upload, /documents/create-from-client, ...), which
// must NOT be reachable directly by read-only roles — those go through the
// admin approval feed instead.
var reDocAction = regexp.MustCompile(`^/documents/\d+(/|$)`)

func RequireRoles(allowed ...int) gin.HandlerFunc {
	allowedSet := map[int]struct{}{}
	for _, r := range allowed {
		allowedSet[r] = struct{}{}
	}
	return func(c *gin.Context) {
		v, exists := c.Get("role_id")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no role in context"})
			return
		}
		roleID, _ := v.(int)
		if _, ok := allowedSet[roleID]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}

func ReadOnlyGuard() gin.HandlerFunc {
	// запрещаем небезопасные методы для read-only ролей
	return func(c *gin.Context) {
		roleV, _ := c.Get("role_id")
		roleID, _ := roleV.(int)
		if authz.IsReadOnly(roleID) {
			switch c.Request.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				// ok
			case http.MethodPost:
				if isReadOnlyChatWriteAllowed(c.Request.URL.Path) ||
					isReadOnlyDocWriteAllowed(c.Request.URL.Path) {
					c.Next()
					return
				}
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "read-only role"})
				return
			default:
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "read-only role"})
				return
			}
		}
		c.Next()
	}
}

func isReadOnlyChatWriteAllowed(path string) bool {
	if path == "/chats/personal" {
		return true
	}
	if !strings.HasPrefix(path, "/chats/") {
		return false
	}
	return strings.HasSuffix(path, "/messages") || strings.HasSuffix(path, "/read")
}

// isReadOnlyDocWriteAllowed permits a read-only role (ОКК) to:
//   - submit an approval request to the admin feed (POST /feed-events);
//   - act on an EXISTING document of its department (archive, submit,
//     send-for-signature, sign sessions, versions, ...), matched by a numeric
//     document id segment.
//
// Document creation endpoints (POST /documents, /documents/upload,
// /documents/create-from-client, /documents/create-from-lead,
// /documents/upload-with-meta) are intentionally NOT matched here, so ОКК must
// route new documents through the admin approval feed.
func isReadOnlyDocWriteAllowed(path string) bool {
	if path == "/feed-events" || path == "/api/v1/feed-events" {
		return true
	}
	return reDocAction.MatchString(path)
}

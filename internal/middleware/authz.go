package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

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
	// запрещаем небезопасные методы для "audit"
	return func(c *gin.Context) {
		roleV, _ := c.Get("role_id")
		roleID, _ := roleV.(int)
		if roleID == 30 { // audit
			switch c.Request.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				// ok
			default:
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "read-only role"})
				return
			}
		}
		c.Next()
	}
}

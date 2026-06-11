package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
)

func RequirePermission(action, resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleV, exists := c.Get("role_id")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no role in context"})
			return
		}
		roleID, _ := roleV.(int)
		userID, _ := c.Get("user_id")
		userIDInt, _ := userID.(int)
		if !authz.Can(authz.UserContext{UserID: userIDInt, RoleID: roleID}, action, resource) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}

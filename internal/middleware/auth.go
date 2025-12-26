package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var JWTKey = []byte("your-secret-key") // TODO: вынести в конфиг

type Claims struct {
	UserID int `json:"user_id"`
	RoleID int `json:"role_id"`
	jwt.RegisteredClaims
}

func isPublicPath(path string) bool {
	switch path {
	case "/login", "/register", "/refresh", "/register/confirm", "/register/resend":
		return true
	case "/auth/forgot-password", "/auth/reset-password":
		return true
	}
	if strings.HasPrefix(path, "/swagger") ||
		strings.HasPrefix(path, "/docs") ||
		strings.HasPrefix(path, "/healthz") {
		return true
	}
	return false
}

func isWebSocketRequest(r *http.Request) bool {
	up := strings.ToLower(strings.TrimSpace(r.Header.Get("Upgrade")))
	conn := strings.ToLower(r.Header.Get("Connection"))
	return up == "websocket" && strings.Contains(conn, "upgrade")
}

func extractBearerToken(authHeader string) string {
	authHeader = strings.TrimSpace(authHeader)
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		if isPublicPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		// 1) Обычный путь: Authorization: Bearer <token>
		tokenStr := extractBearerToken(c.GetHeader("Authorization"))

		// 2) WS-путь: браузер не умеет Authorization header -> берём токен из query
		if tokenStr == "" && isWebSocketRequest(c.Request) {
			tokenStr = strings.TrimSpace(c.Query("token"))
			if tokenStr == "" {
				tokenStr = strings.TrimSpace(c.Query("access_token"))
			}
		}

		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid Authorization header"})
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrTokenSignatureInvalid
			}
			return JWTKey, nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}

		const leeway = 2 * time.Minute
		now := time.Now().Add(-leeway)
		if claims.ExpiresAt == nil || claims.ExpiresAt.Time.Before(now) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("role_id", claims.RoleID)
		c.Next()
	}
}

package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var JWTKey = []byte("your-secret-key") // TODO: вынести в конфиг/ENV

type Claims struct {
	UserID int `json:"user_id"`
	RoleID int `json:"role_id"`
	jwt.RegisteredClaims
}

// список публичных эндпоинтов, которые не требуют токена
func isPublicPath(path string) bool {
	switch path {
	case "/login", "/register":
		return true
	}
	// часто полезно пропускать документацию/healthcheck:
	if strings.HasPrefix(path, "/swagger") ||
		strings.HasPrefix(path, "/docs") ||
		strings.HasPrefix(path, "/healthz") {
		return true
	}
	return false
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1) пропускаем preflight
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		// 2) пропускаем публичные пути
		if isPublicPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		// 3) читаем Authorization
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid Authorization header"})
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid Authorization header"})
			return
		}
		tokenStr := strings.TrimSpace(parts[1])
		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid Authorization header"})
			return
		}

		// 4) парсим и валидируем токен
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			// защита: принимаем только HMAC (HS256 и т.п.)
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrTokenSignatureInvalid
			}
			return JWTKey, nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}

		// 5) проверка срока действия с небольшим leeway
		const leeway = 2 * time.Minute
		now := time.Now().Add(-leeway)
		if claims.ExpiresAt == nil || claims.ExpiresAt.Time.Before(now) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}

		// 6) прокидываем user/role в контекст
		c.Set("user_id", claims.UserID)
		c.Set("role_id", claims.RoleID)

		c.Next()
	}
}

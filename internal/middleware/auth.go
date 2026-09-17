package middleware

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID int `json:"user_id"`
	RoleID int `json:"role_id"`
	jwt.RegisteredClaims
}

func isPublicPath(path string) bool {
	switch path {
	case "/register", "/register/confirm", "/register/resend", "/auth/login", "/auth/refresh":
		return true
	case "/auth/forgot-password", "/auth/reset-password":
		return true
	case "/favicon.ico":
		return true
	}
	if strings.HasPrefix(path, "/sign/email/verify") ||
		strings.HasPrefix(path, "/api/v1/sign/email/verify") ||
		strings.HasPrefix(path, "/api/v1/sign/email/preview") {
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

// AccountStatusChecker возвращает актуальное состояние учётной записи из БД.
// Нужен потому, что JWT живёт до 2 часов, а refresh-токен — 30 дней: без
// проверки «на лету» заблокированный сотрудник или сотрудник со статусом
// «Не подтверждён» продолжал работать в системе до истечения токена
// (обратная связь заказчика 17.09.2026).
type AccountStatusChecker func(userID int) (isActive bool, isVerified bool, err error)

func NewAuthMiddleware(jwtSecret []byte, statusChecker ...AccountStatusChecker) gin.HandlerFunc {
	var checkStatus AccountStatusChecker
	for _, fn := range statusChecker {
		if fn != nil {
			checkStatus = fn
			break
		}
	}
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
			log.Printf("[auth][middleware] unauthorized: reason=missing_token path=%s method=%s", c.Request.URL.Path, c.Request.Method)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid Authorization header"})
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrTokenSignatureInvalid
			}
			return jwtSecret, nil
		})
		if err != nil || !token.Valid {
			reason := "invalid_token"
			message := "Invalid token"
			switch {
			case errors.Is(err, jwt.ErrTokenExpired):
				reason = "expired_token"
				message = "Token expired"
			case errors.Is(err, jwt.ErrTokenSignatureInvalid):
				reason = "invalid_signature"
				message = "Invalid token signature"
			}
			log.Printf("[auth][middleware] unauthorized: reason=%s path=%s method=%s err=%v", reason, c.Request.URL.Path, c.Request.Method, err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": message})
			return
		}

		const leeway = 2 * time.Minute
		now := time.Now().UTC().Add(-leeway)
		if claims.ExpiresAt == nil || claims.ExpiresAt.Before(now) {
			log.Printf("[auth][middleware] unauthorized: reason=expired_token_leeway path=%s method=%s exp=%v now=%s", c.Request.URL.Path, c.Request.Method, claims.ExpiresAt, now.Format(time.RFC3339))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token expired"})
			return
		}

		// Учётка могла быть заблокирована или переведена в «Не подтверждён»
		// уже после выдачи токена — проверяем текущее состояние в БД.
		if checkStatus != nil {
			isActive, isVerified, err := checkStatus(claims.UserID)
			switch {
			case err != nil:
				log.Printf("[auth][middleware] unauthorized: reason=account_lookup_failed user_id=%d path=%s err=%v", claims.UserID, c.Request.URL.Path, err)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Учётная запись недоступна"})
				return
			case !isActive:
				log.Printf("[auth][middleware] unauthorized: reason=account_blocked user_id=%d path=%s", claims.UserID, c.Request.URL.Path)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Пользователь отключен"})
				return
			case !isVerified:
				log.Printf("[auth][middleware] unauthorized: reason=account_not_verified user_id=%d path=%s", claims.UserID, c.Request.URL.Path)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Учётная запись не подтверждена"})
				return
			}
		}

		c.Set("user_id", claims.UserID)
		c.Set("role_id", claims.RoleID)
		c.Next()
	}
}

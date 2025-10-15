package handlers

import (
	"log"
	"net/http"
	"time"
	"turcompany/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"turcompany/internal/middleware"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type AuthHandler struct {
	userService services.UserService
	authService services.AuthService
}

func NewAuthHandler(userService services.UserService, authService services.AuthService) *AuthHandler {
	return &AuthHandler{userService: userService, authService: authService}
}

// @Summary      Вход в систему
// @Description  Аутентифицирует пользователя и возвращает токены доступа
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        login  body      models.LoginRequest  true  "Данные для входа"
// @Success      200    {object}  map[string]interface{}
// @Failure      400    {object}  map[string]string
// @Failure      401    {object}  map[string]string
// @Failure      500    {object}  map[string]string
// @Router       /login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	start := time.Now()

	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[auth][login] bad request: bind json failed: err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	email := strings.TrimSpace(req.Email)
	log.Printf("[auth][login] attempt email=%q", email)

	user, err := h.userService.GetUserByEmail(email)
	if err != nil {
		log.Printf("[auth][login] user not found by email=%q: err=%v", email, err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}
	if user == nil {
		log.Printf("[auth][login] user is nil for email=%q", email)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// Диагностика по хешу (без вывода самого хеша)
	ph := strings.TrimSpace(user.PasswordHash)
	log.Printf("[auth][login] user found: id=%d role=%d hash_len=%d bcrypt_prefix=%v",
		user.ID, user.RoleID, len(ph), strings.HasPrefix(ph, "$2"))

	if ph == "" {
		log.Printf("[auth][login] empty password_hash in DB for userID=%d email=%q", user.ID, email)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// Сравнение пароля
	pw := strings.TrimSpace(req.Password)
	if err := bcrypt.CompareHashAndPassword([]byte(ph), []byte(pw)); err != nil {
		log.Printf("[auth][login] bcrypt mismatch for userID=%d email=%q: err=%v", user.ID, email, err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}
	log.Printf("[auth][login] password OK for userID=%d", user.ID)

	// Access JWT
	accessClaims := &middleware.Claims{
		UserID: user.ID,
		RoleID: user.RoleID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString(middleware.JWTKey)
	if err != nil {
		log.Printf("[auth][login] sign access token failed for userID=%d: err=%v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}
	log.Printf("[auth][login] access token generated for userID=%d exp_in=%s",
		user.ID, time.Until(accessClaims.ExpiresAt.Time).Truncate(time.Second))

	// Refresh (opaque) -> хранится в БД
	rt, err := utils.NewRefreshToken(32)
	if err != nil {
		log.Printf("[auth][login] new refresh token failed for userID=%d: err=%v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}
	rtExp := time.Now().Add(30 * 24 * time.Hour)
	if err := h.userService.UpdateRefresh(user.ID, rt, rtExp); err != nil {
		log.Printf("[auth][login] store refresh token failed for userID=%d: err=%v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store refresh token"})
		return
	}
	log.Printf("[auth][login] refresh token stored for userID=%d exp_at=%s", user.ID, rtExp.Format(time.RFC3339))

	// Финал
	log.Printf("[auth][login] success userID=%d role=%d took=%s", user.ID, user.RoleID, time.Since(start).Truncate(time.Millisecond))

	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"user":    user, // у модели PasswordHash помечен json:"-", наружу не уйдет
		"tokens": gin.H{
			"access_token":  accessTokenString,
			"refresh_token": rt, // значение отдаём клиенту, но не логируем
		},
	})
}
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	old := strings.TrimSpace(req.RefreshToken)
	user, err := h.userService.GetByRefreshToken(old)
	if err != nil || user == nil || user.RefreshToken == nil || user.RefreshExpiresAt == nil || user.RefreshRevoked {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}
	if time.Now().After(*user.RefreshExpiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token expired"})
		return
	}

	// rotate refresh
	newRT, err := utils.NewRefreshToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to rotate refresh token"})
		return
	}
	newExp := time.Now().Add(30 * 24 * time.Hour)
	rotatedUser, err := h.userService.RotateRefresh(old, newRT, newExp)
	if err != nil || rotatedUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	// new access
	accessClaims := &middleware.Claims{
		UserID: rotatedUser.ID,
		RoleID: rotatedUser.RoleID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString(middleware.JWTKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessTokenString,
		"refresh_token": newRT, // возвращаем новый (ротация)
	})
}

package handlers

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type AuthHandler struct {
	userService          services.UserService
	authService          services.AuthService
	passwordResetService services.PasswordResetService
}

func NewAuthHandler(userService services.UserService, authService services.AuthService, passwordResetService services.PasswordResetService) *AuthHandler {
	return &AuthHandler{userService: userService, authService: authService, passwordResetService: passwordResetService}
}

func (h *AuthHandler) Login(c *gin.Context) {
	start := time.Now()

	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[auth][login] bad request: bind json failed: err=%v", err)
		badRequest(c, "Invalid login payload")
		return
	}
	email := strings.TrimSpace(req.Email)
	log.Printf("[auth][login] attempt email=%q", email)

	user, err := h.userService.GetUserByEmail(email)
	if err != nil || user == nil {
		log.Printf("[auth][login] user not found by email=%q: err=%v", email, err)
		unauthorized(c, "Invalid email or password")
		return
	}

	// Блокируем логин, если телефон не подтверждён
	if !user.IsVerified {
		log.Printf("[auth][login] user not verified id=%d", user.ID)
		forbidden(c, "Phone not verified")
		return
	}

	ph := strings.TrimSpace(user.PasswordHash)
	log.Printf("[auth][login] user found: id=%d role=%d hash_len=%d bcrypt_prefix=%v",
		user.ID, user.RoleID, len(ph), strings.HasPrefix(ph, "$2"))

	if ph == "" {
		log.Printf("[auth][login] empty password_hash in DB for userID=%d email=%q", user.ID, email)
		unauthorized(c, "Invalid email or password")
		return
	}

	pw := strings.TrimSpace(req.Password)
	if !h.authService.VerifyPassword(ph, pw) {
		log.Printf("[auth][login] bcrypt mismatch for userID=%d email=%q", user.ID, email)
		unauthorized(c, "Invalid email or password")
		return
	}
	log.Printf("[auth][login] password OK for userID=%d", user.ID)

	accessTokenString, accessExp, err := h.authService.GenerateAccessToken(user.ID, user.RoleID)
	if err != nil {
		log.Printf("[auth][login] sign access token failed for userID=%d: err=%v", user.ID, err)
		internalError(c, "Failed to generate access token")
		return
	}
	log.Printf("[auth][login] access token generated for userID=%d exp_in=%s",
		user.ID, time.Until(accessExp).Truncate(time.Second))

	rt, rtExp, err := h.authService.GenerateRefreshToken()
	if err != nil {
		log.Printf("[auth][login] new refresh token failed for userID=%d: err=%v", user.ID, err)
		internalError(c, "Failed to generate refresh token")
		return
	}
	if err := h.userService.UpdateRefresh(user.ID, rt, rtExp); err != nil {
		log.Printf("[auth][login] store refresh token failed for userID=%d: err=%v", user.ID, err)
		internalError(c, "Failed to store refresh token")
		return
	}
	log.Printf("[auth][login] refresh token stored for userID=%d exp_at=%s", user.ID, rtExp.Format(time.RFC3339))

	log.Printf("[auth][login] success userID=%d role=%d took=%s", user.ID, user.RoleID, time.Since(start).Truncate(time.Millisecond))

	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"user":    user, // PasswordHash скрыт тегом json:"-"
		"tokens": gin.H{
			"access_token":  accessTokenString,
			"refresh_token": rt,
		},
	})
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid refresh token payload")
		return
	}
	old := strings.TrimSpace(req.RefreshToken)
	user, err := h.userService.GetByRefreshToken(old)
	if err != nil || user == nil || user.RefreshToken == nil || user.RefreshExpiresAt == nil || user.RefreshRevoked {
		unauthorized(c, "Invalid refresh token")
		return
	}
	if time.Now().After(*user.RefreshExpiresAt) {
		unauthorized(c, "Refresh token expired")
		return
	}

	newRT, newExp, err := h.authService.GenerateRefreshToken()
	if err != nil {
		internalError(c, "Failed to rotate refresh token")
		return
	}
	rotatedUser, err := h.userService.RotateRefresh(old, newRT, newExp)
	if err != nil || rotatedUser == nil {
		unauthorized(c, "Invalid refresh token")
		return
	}

	accessTokenString, _, err := h.authService.GenerateAccessToken(rotatedUser.ID, rotatedUser.RoleID)
	if err != nil {
		internalError(c, "Failed to generate access token")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessTokenString,
		"refresh_token": newRT,
	})
}
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid forgot password payload")
		return
	}
	if err := h.passwordResetService.RequestReset(req.Email); err != nil {
		badRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "If the account exists, password reset instructions were sent"})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req struct {
		Token    string `json:"token" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid reset password payload")
		return
	}
	if err := h.passwordResetService.ResetPassword(req.Token, req.Password); err != nil {
		badRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Password updated"})
}

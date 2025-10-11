package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
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
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.userService.GetUserByEmail(req.Email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}
	if !h.authService.VerifyPassword(user.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// Access Token создается на 15 минут
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	// Refresh Token создается на 30 дней
	refreshClaims := &middleware.Claims{
		UserID: user.ID,
		RoleID: user.RoleID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
		},
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString(middleware.JWTKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"user":    user,
		"tokens": gin.H{
			"access_token":  accessTokenString,
			"refresh_token": refreshTokenString,
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

	token, err := jwt.ParseWithClaims(req.RefreshToken, &middleware.Claims{}, func(token *jwt.Token) (interface{}, error) {
		return middleware.JWTKey, nil
	})

	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	claims, ok := token.Claims.(*middleware.Claims)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
		return
	}

	accessClaims := &middleware.Claims{
		UserID: claims.UserID,
		RoleID: claims.RoleID,
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
		"access_token": accessTokenString,
	})
}

package handlers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type UserHandler struct {
	service             services.UserService
	verificationService *services.UserVerificationService
}

type createUserRequest struct {
	CompanyName string `json:"company_name" binding:"required"`
	BinIin      string `json:"bin_iin"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=6"`
	Phone       string `json:"phone" binding:"required"` // НОВОЕ: телефон обязателен
	RoleID      int    `json:"role_id"`                  // игнорится если создатель не админ
}

func NewUserHandler(
	service services.UserService,
	verificationService *services.UserVerificationService,
) *UserHandler {
	return &UserHandler{
		service:             service,
		verificationService: verificationService,
	}
}

func sanitizeUser(u *models.User) *models.User {
	cp := *u
	cp.PasswordHash = ""
	return &cp
}

func (h *UserHandler) CreateUser(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !authz.CanAssignRoles(roleID) {
		forbidden(c, "Only system admin can create users")
		return
	}

	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid user payload")
		return
	}

	newRole := req.RoleID
	if newRole == 0 {
		newRole = authz.RoleSales
	}
	if !authz.IsKnownRole(newRole) {
		badRequest(c, "Invalid role_id")
		return
	}

	user := &models.User{
		CompanyName: req.CompanyName,
		BinIin:      req.BinIin,
		Email:       req.Email,
		Phone:       req.Phone,
		RoleID:      newRole,
		// админ создает сразу верифицированного? оставим false, чтобы процесс был единый
		IsVerified: false,
	}

	if err := h.service.CreateUserWithPassword(user, req.Password); err != nil {
		log.Printf("CreateUser: service error: %v", err)
		internalError(c, "Failed to create user")
		return
	}

	// Отправим письмо с кодом (можно игнорить ошибку, юзер сможет переслать код публичной ручкой)
	if h.verificationService != nil {
		if err := h.verificationService.Send(user.ID, user.Email); err != nil {
			log.Printf("[users][create] send user verification email failed: %v", err)
		}
	}

	c.JSON(http.StatusCreated, sanitizeUser(user))
}

// GET /users/me
func (h *UserHandler) GetMyProfile(c *gin.Context) {
	userID, _ := getUserAndRole(c) // roleID нам здесь не нужен
	if userID == 0 {
		unauthorized(c, "Unauthorized")
		return
	}

	user, err := h.service.GetUserByID(userID)
	if err != nil || user == nil {
		notFound(c, ClientNotFoundCode, "User not found")
		return
	}

	c.JSON(http.StatusOK, sanitizeUser(user))
}

func (h *UserHandler) GetUserByID(c *gin.Context) {
	currentUserID, roleID := getUserAndRole(c)

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		badRequest(c, "Invalid user ID")
		return
	}

	// Если нет full-access/read-only, то можно смотреть только себя
	if !(authz.CanViewLeadershipData(roleID) || authz.IsReadOnly(roleID)) && currentUserID != id {
		forbidden(c, "Forbidden")
		return
	}

	user, err := h.service.GetUserByID(id)
	if err != nil || user == nil {
		notFound(c, ClientNotFoundCode, "User not found")
		return
	}
	if !authz.CanViewLeadershipData(roleID) && user.RoleID == authz.RoleManagement {
		forbidden(c, "Forbidden")
		return
	}

	c.JSON(http.StatusOK, sanitizeUser(user))
}

func (h *UserHandler) UpdateUser(c *gin.Context) {
	userID, roleID := getUserAndRole(c)

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		badRequest(c, "Invalid user ID")
		return
	}

	target, err := h.service.GetUserByID(id)
	if err != nil || target == nil {
		notFound(c, ClientNotFoundCode, "User not found")
		return
	}

	var body models.User
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Invalid user payload")
		return
	}
	body.ID = id

	// ВАЖНО: всегда сохраняем текущий хэш, чтобы не затереть его пустой строкой.
	body.PasswordHash = target.PasswordHash

	if !authz.CanAssignRoles(roleID) {
		if userID != id {
			forbidden(c, "Forbidden")
			return
		}
		// обычному пользователю нельзя менять роль/верификацию
		body.RoleID = target.RoleID
		body.IsVerified = target.IsVerified
		body.VerifiedAt = target.VerifiedAt
	}
	if authz.CanAssignRoles(roleID) && body.RoleID != 0 && !authz.IsKnownRole(body.RoleID) {
		badRequest(c, "Invalid role_id")
		return
	}

	if err := h.service.UpdateUser(&body); err != nil {
		log.Printf("UpdateUser: service error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error_code": "INTERNAL_ERROR",
			"message":    "Failed to update user",
			"details":    err.Error(), // <-- важно
		})
		return
	}

	updated, _ := h.service.GetUserByID(id)
	c.JSON(http.StatusOK, sanitizeUser(updated))
}

func (h *UserHandler) DeleteUser(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !authz.CanAssignRoles(roleID) {
		forbidden(c, "Only system admin can delete users")
		return
	}
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		badRequest(c, "Invalid user ID")
		return
	}
	if err := h.service.DeleteUser(id); err != nil {
		log.Printf("DeleteUser: service error: %v", err)
		internalError(c, "Failed to delete user")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "User deleted"})
}

func (h *UserHandler) ListUsers(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !(authz.CanViewLeadershipData(roleID) || authz.IsReadOnly(roleID)) {
		forbidden(c, "Forbidden")
		return
	}

	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "10")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	users, err := h.service.ListUsers(limit, offset)
	if err != nil {
		log.Printf("ListUsers: service error: %v", err)
		internalError(c, "Failed to list users")
		return
	}

	out := make([]*models.User, 0, len(users))
	for _, u := range users {
		if !authz.CanViewLeadershipData(roleID) && u.RoleID == authz.RoleManagement {
			continue
		}
		out = append(out, sanitizeUser(u))
	}
	c.JSON(http.StatusOK, out)
}

func (h *UserHandler) GetUserCount(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !(authz.CanViewLeadershipData(roleID) || authz.IsReadOnly(roleID)) {
		forbidden(c, "Forbidden")
		return
	}
	count, err := h.service.GetUserCount()
	if err != nil {
		log.Printf("GetUserCount: service error: %v", err)
		internalError(c, "Failed to get user count")
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (h *UserHandler) GetUserCountByRole(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !(authz.CanViewLeadershipData(roleID) || authz.IsReadOnly(roleID)) {
		forbidden(c, "Forbidden")
		return
	}

	roleIDVal, err := strconv.Atoi(c.Param("role_id"))
	if err != nil {
		badRequest(c, "Invalid role ID")
		return
	}
	if !authz.CanViewLeadershipData(roleID) && roleIDVal == authz.RoleManagement {
		forbidden(c, "Forbidden")
		return
	}

	count, err := h.service.GetUserCountByRole(roleIDVal)
	if err != nil {
		log.Printf("GetUserCountByRole: service error: %v", err)
		internalError(c, "Failed to get user count by role")
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count, "role_id": roleIDVal})
}

// Публичная регистрация: создаём sales + is_verified=false, шлём код на email
func (h *UserHandler) Register(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid registration payload")
		return
	}
	req.RoleID = authz.RoleSales

	user := &models.User{
		CompanyName: req.CompanyName,
		BinIin:      req.BinIin,
		Email:       req.Email,
		Phone:       req.Phone,
		RoleID:      req.RoleID,
		IsVerified:  false,
	}
	if err := h.service.CreateUserWithPassword(user, req.Password); err != nil {
		log.Printf("Register: service error: %v", err)
		internalError(c, "Failed to register user")
		return
	}

	verificationSent := false
	if h.verificationService != nil {
		if err := h.verificationService.Send(user.ID, user.Email); err != nil {
			log.Printf("[register] send verification email failed: %v", err)
		} else {
			verificationSent = true
		}
	}

	user.PasswordHash = ""
	c.JSON(http.StatusCreated, gin.H{
		"user":              user,
		"message":           "Registered. Email code sent.",
		"verification_sent": verificationSent,
	})
}

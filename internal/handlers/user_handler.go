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
	service    services.UserService
	smsService *services.SMS_Service
}

type createUserRequest struct {
	CompanyName string `json:"company_name" binding:"required"`
	BinIin      string `json:"bin_iin"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=6"`
	Phone       string `json:"phone" binding:"required"` // НОВОЕ: телефон обязателен
	RoleID      int    `json:"role_id"`                  // игнорится если создатель не админ
}

func NewUserHandler(service services.UserService, smsService *services.SMS_Service) *UserHandler {
	return &UserHandler{service: service, smsService: smsService}
}

// небольшое маскирование сведений о руководстве для роли Audit
func maskIfAudit(callerRole int, u *models.User) *models.User {
	if callerRole == authz.RoleAudit && u.RoleID == authz.RoleManagement {
		return &models.User{
			ID:           u.ID,
			CompanyName:  "",
			BinIin:       "",
			Email:        "",
			PasswordHash: "",
			RoleID:       u.RoleID,
		}
	}
	cp := *u
	cp.PasswordHash = ""
	return &cp
}

func (h *UserHandler) CreateUser(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if roleID != authz.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "only admin can create users"})
		return
	}

	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newRole := req.RoleID
	if newRole == 0 {
		newRole = authz.RoleSales
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	// Отправим SMS с кодом (можно игнорить ошибку, юзер сможет переслать код публичной ручкой)
	if h.smsService != nil {
		if err := h.smsService.SendUserSMS(user.ID, user.Phone); err != nil {
			log.Printf("[users][create] send user sms failed: %v", err)
		}
	}

	c.JSON(http.StatusCreated, maskIfAudit(roleID, user))
}

// GET /users/me
func (h *UserHandler) GetMyProfile(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	user, err := h.service.GetUserByID(userID)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	c.JSON(http.StatusOK, maskIfAudit(roleID, user))
}

func (h *UserHandler) GetUserByID(c *gin.Context) {
	_, roleID := getUserAndRole(c)

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	user, err := h.service.GetUserByID(id)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	c.JSON(http.StatusOK, maskIfAudit(roleID, user))
}

func (h *UserHandler) UpdateUser(c *gin.Context) {
	userID, roleID := getUserAndRole(c)

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	target, err := h.service.GetUserByID(id)
	if err != nil || target == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	var body models.User
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	body.ID = id

	// ВАЖНО: всегда сохраняем текущий хэш, чтобы не затереть его пустой строкой.
	body.PasswordHash = target.PasswordHash

	if roleID != authz.RoleAdmin {
		if userID != id {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		// обычному пользователю нельзя менять роль/верификацию
		body.RoleID = target.RoleID
		body.IsVerified = target.IsVerified
		body.VerifiedAt = target.VerifiedAt
	}

	if err := h.service.UpdateUser(&body); err != nil {
		log.Printf("UpdateUser: service error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	updated, _ := h.service.GetUserByID(id)
	c.JSON(http.StatusOK, maskIfAudit(roleID, updated))
}

func (h *UserHandler) DeleteUser(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if roleID != authz.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "only admin can delete users"})
		return
	}
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	if err := h.service.DeleteUser(id); err != nil {
		log.Printf("DeleteUser: service error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "User deleted"})
}

func (h *UserHandler) ListUsers(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !(roleID == authz.RoleManagement || roleID == authz.RoleAdmin || roleID == authz.RoleAudit) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list users"})
		return
	}

	out := make([]*models.User, 0, len(users))
	for _, u := range users {
		out = append(out, maskIfAudit(roleID, u))
	}
	c.JSON(http.StatusOK, out)
}

func (h *UserHandler) GetUserCount(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !(roleID == authz.RoleManagement || roleID == authz.RoleAdmin || roleID == authz.RoleAudit) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	count, err := h.service.GetUserCount()
	if err != nil {
		log.Printf("GetUserCount: service error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user count"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (h *UserHandler) GetUserCountByRole(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !(roleID == authz.RoleManagement || roleID == authz.RoleAdmin || roleID == authz.RoleAudit) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	roleIDVal, err := strconv.Atoi(c.Param("role_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	count, err := h.service.GetUserCountByRole(roleIDVal)
	if err != nil {
		log.Printf("GetUserCountByRole: service error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user count by role"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count, "role_id": roleIDVal})
}

// Публичная регистрация: создаём sales + is_verified=false, шлём SMS
func (h *UserHandler) Register(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user"})
		return
	}

	if h.smsService != nil {
		if err := h.smsService.SendUserSMS(user.ID, user.Phone); err != nil {
			log.Printf("[register] send sms failed: %v", err)
		}
	}

	user.PasswordHash = ""
	c.JSON(http.StatusCreated, gin.H{
		"user":    user,
		"message": "Registered. SMS code sent.",
	})
}

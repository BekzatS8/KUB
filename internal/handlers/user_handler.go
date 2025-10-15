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
	service     services.UserService
	authService services.AuthService
}

type createUserRequest struct {
	CompanyName string `json:"company_name" binding:"required"`
	BinIin      string `json:"bin_iin"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=6"`
	RoleID      int    `json:"role_id"` // будет проигнорирован, если создатель не админ
}

func NewUserHandler(service services.UserService, authService services.AuthService) *UserHandler {
	return &UserHandler{service: service, authService: authService}
}

// небольшое маскирование сведений о руководстве для роли Audit
func maskIfAudit(callerRole int, u *models.User) *models.User {
	if callerRole == authz.RoleAudit && u.RoleID == authz.RoleManagement {
		return &models.User{
			ID:           u.ID,
			CompanyName:  "", // скрываем
			BinIin:       "",
			Email:        "",
			PasswordHash: "",
			RoleID:       u.RoleID,
		}
	}
	// по умолчанию — просто не показываем password_hash
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
		newRole = authz.RoleSales // по умолчанию — продажник
	}

	user := &models.User{
		CompanyName: req.CompanyName,
		BinIin:      req.BinIin,
		Email:       req.Email,
		RoleID:      newRole,
	}

	if err := h.service.CreateUserWithPassword(user, req.Password); err != nil {
		log.Printf("CreateUser: service error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}
	c.JSON(http.StatusCreated, maskIfAudit(roleID, user))
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

	if roleID != authz.RoleAdmin {
		// не-админ — только себя и без повышения прав
		if userID != id {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		body.RoleID = target.RoleID             // нельзя менять свою роль
		body.PasswordHash = target.PasswordHash // пароль меняется отдельным флоу (если нужен)
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

func (h *UserHandler) Register(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// публичная регистрация всегда = Sales
	req.RoleID = authz.RoleSales

	user := &models.User{
		CompanyName: req.CompanyName,
		BinIin:      req.BinIin,
		Email:       req.Email,
		RoleID:      req.RoleID,
	}
	if err := h.service.CreateUserWithPassword(user, req.Password); err != nil {
		log.Printf("Register: service error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user"})
		return
	}
	// пароль не возвращаем
	user.PasswordHash = ""
	c.JSON(http.StatusCreated, user)
}

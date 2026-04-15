package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type UserHandler struct {
	service             services.UserService
	branchService       services.BranchService
	verificationService *services.UserVerificationService
}

type createUserRequest struct {
	CompanyName string `json:"company_name"`
	BinIin      string `json:"bin_iin"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	MiddleName  string `json:"middle_name"`
	Position    string `json:"position"`
	BranchID    *int   `json:"branch_id"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=6"`
	Phone       string `json:"phone" binding:"required"`
	RoleID      int    `json:"role_id"`
	IsVerified  *bool  `json:"is_verified"`
	IsActive    *bool  `json:"is_active"`
}

type updateUserRequest struct {
	CompanyName *string `json:"company_name"`
	BinIin      *string `json:"bin_iin"`
	FirstName   *string `json:"first_name"`
	LastName    *string `json:"last_name"`
	MiddleName  *string `json:"middle_name"`
	Position    *string `json:"position"`
	BranchID    *int    `json:"branch_id"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
	RoleID      *int    `json:"role_id"`
	IsVerified  *bool   `json:"is_verified"`
	IsActive    *bool   `json:"is_active"`
}

func NewUserHandler(service services.UserService, branchService services.BranchService, verificationService *services.UserVerificationService) *UserHandler {
	return &UserHandler{service: service, branchService: branchService, verificationService: verificationService}
}

type userResponse struct {
	ID         int         `json:"id"`
	FirstName  string      `json:"first_name,omitempty"`
	LastName   string      `json:"last_name,omitempty"`
	MiddleName string      `json:"middle_name,omitempty"`
	FullName   string      `json:"full_name"`
	Email      string      `json:"email"`
	Phone      string      `json:"phone"`
	Role       gin.H       `json:"role"`
	Position   string      `json:"position,omitempty"`
	Branch     interface{} `json:"branch"`
	IsActive   bool        `json:"is_active"`
	IsVerified bool        `json:"is_verified"`
	Telegram   gin.H       `json:"telegram"`
	Legacy     gin.H       `json:"legacy,omitempty"`
}

func rolePayload(roleID int) gin.H {
	meta, ok := authz.Roles[roleID]
	if !ok {
		return gin.H{"id": roleID}
	}
	return gin.H{"id": roleID, "code": meta.Code, "legacy_name": meta.LegacyName}
}

func userFullName(u *models.User) string {
	parts := []string{strings.TrimSpace(u.LastName), strings.TrimSpace(u.FirstName), strings.TrimSpace(u.MiddleName)}
	full := strings.TrimSpace(strings.Join(parts, " "))
	if full != "" {
		return full
	}
	if strings.TrimSpace(u.CompanyName) != "" {
		return strings.TrimSpace(u.CompanyName)
	}
	return u.Email
}

func (h *UserHandler) branchPayload(branchID *int) interface{} {
	if branchID == nil {
		return nil
	}
	if h.branchService == nil {
		return gin.H{"id": *branchID}
	}
	b, err := h.branchService.GetBranchByID(*branchID)
	if err != nil || b == nil {
		return gin.H{"id": *branchID}
	}
	return gin.H{"id": b.ID, "name": b.Name, "code": b.Code, "is_active": b.IsActive}
}

func (h *UserHandler) userToResponse(u *models.User) *userResponse {
	if u == nil {
		return nil
	}
	legacy := gin.H{"company_name": u.CompanyName, "bin_iin": u.BinIin}
	return &userResponse{
		ID:         u.ID,
		FirstName:  u.FirstName,
		LastName:   u.LastName,
		MiddleName: u.MiddleName,
		FullName:   userFullName(u),
		Email:      u.Email,
		Phone:      u.Phone,
		Role:       rolePayload(u.RoleID),
		Position:   u.Position,
		Branch:     h.branchPayload(u.BranchID),
		IsActive:   u.IsActive,
		IsVerified: u.IsVerified,
		Telegram: gin.H{
			"chat_id":      u.TelegramChatID,
			"notify_tasks": u.NotifyTasksTelegram,
		},
		Legacy: legacy,
	}
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
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		MiddleName:  req.MiddleName,
		Position:    req.Position,
		BranchID:    req.BranchID,
		Email:       req.Email,
		Phone:       req.Phone,
		RoleID:      newRole,
		IsVerified:  false,
		IsActive:    true,
	}
	if req.IsVerified != nil {
		user.IsVerified = *req.IsVerified
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}
	if err := h.service.CreateUserWithPassword(user, req.Password); err != nil {
		log.Printf("CreateUser: service error: %v", err)
		internalError(c, "Failed to create user")
		return
	}
	if h.verificationService != nil {
		if err := h.verificationService.Send(user.ID, user.Email); err != nil {
			log.Printf("[users][create] send user verification email failed: %v", err)
		}
	}
	c.JSON(http.StatusCreated, h.userToResponse(user))
}

func (h *UserHandler) GetMyProfile(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	if userID == 0 {
		unauthorized(c, "Unauthorized")
		return
	}
	user, err := h.service.GetUserByID(userID)
	if err != nil || user == nil {
		notFound(c, ClientNotFoundCode, "User not found")
		return
	}
	c.JSON(http.StatusOK, h.userToResponse(user))
}

func (h *UserHandler) GetUserByID(c *gin.Context) {
	currentUserID, roleID := getUserAndRole(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid user ID")
		return
	}
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
	c.JSON(http.StatusOK, h.userToResponse(user))
}

func (h *UserHandler) UpdateUser(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid user ID")
		return
	}
	target, err := h.service.GetUserByID(id)
	if err != nil || target == nil {
		notFound(c, ClientNotFoundCode, "User not found")
		return
	}
	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid user payload")
		return
	}
	body := *target
	body.ID = id
	body.PasswordHash = target.PasswordHash
	if !authz.CanAssignRoles(roleID) {
		if userID != id {
			forbidden(c, "Forbidden")
			return
		}
		req.RoleID = nil
		req.IsVerified = nil
	}
	if authz.CanAssignRoles(roleID) && req.RoleID != nil && !authz.IsKnownRole(*req.RoleID) {
		badRequest(c, "Invalid role_id")
		return
	}
	if req.CompanyName != nil {
		body.CompanyName = *req.CompanyName
	}
	if req.BinIin != nil {
		body.BinIin = *req.BinIin
	}
	if req.FirstName != nil {
		body.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		body.LastName = *req.LastName
	}
	if req.MiddleName != nil {
		body.MiddleName = *req.MiddleName
	}
	if req.Position != nil {
		body.Position = *req.Position
	}
	if req.Email != nil {
		body.Email = *req.Email
	}
	if req.Phone != nil {
		body.Phone = *req.Phone
	}
	if req.RoleID != nil {
		body.RoleID = *req.RoleID
	}
	if req.BranchID != nil {
		body.BranchID = req.BranchID
	}
	if req.IsVerified != nil {
		body.IsVerified = *req.IsVerified
	}
	if req.IsActive != nil {
		body.IsActive = *req.IsActive
	}
	if err := h.service.UpdateUser(&body); err != nil {
		log.Printf("UpdateUser: service error: %v", err)
		internalError(c, "Failed to update user")
		return
	}
	updated, _ := h.service.GetUserByID(id)
	c.JSON(http.StatusOK, h.userToResponse(updated))
}

func (h *UserHandler) DeleteUser(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !authz.CanAssignRoles(roleID) {
		forbidden(c, "Only system admin can delete users")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
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
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit
	users, err := h.service.ListUsers(limit, offset)
	if err != nil {
		log.Printf("ListUsers: service error: %v", err)
		internalError(c, "Failed to list users")
		return
	}
	out := make([]*userResponse, 0, len(users))
	for _, u := range users {
		if !authz.CanViewLeadershipData(roleID) && u.RoleID == authz.RoleManagement {
			continue
		}
		out = append(out, h.userToResponse(u))
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
		internalError(c, "Failed to get user count by role")
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count, "role_id": roleIDVal})
}

func (h *UserHandler) Register(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid registration payload")
		return
	}
	user := &models.User{
		CompanyName: req.CompanyName,
		BinIin:      req.BinIin,
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		MiddleName:  req.MiddleName,
		Position:    req.Position,
		BranchID:    req.BranchID,
		Email:       req.Email,
		Phone:       req.Phone,
		RoleID:      authz.RoleSales,
		IsVerified:  false,
		IsActive:    true,
	}
	if err := h.service.CreateUserWithPassword(user, req.Password); err != nil {
		log.Printf("Register: service error: %v", err)
		internalError(c, "Failed to register user")
		return
	}
	verificationSent := false
	if h.verificationService != nil {
		if err := h.verificationService.Send(user.ID, user.Email); err == nil {
			verificationSent = true
		}
	}
	c.JSON(http.StatusCreated, gin.H{"user": h.userToResponse(user), "message": "Registered. Email code sent.", "verification_sent": verificationSent})
}

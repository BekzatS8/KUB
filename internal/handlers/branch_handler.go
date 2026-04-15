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

type BranchHandler struct {
	branchService services.BranchService
	userService   services.UserService
}

func NewBranchHandler(branchService services.BranchService, userService services.UserService) *BranchHandler {
	return &BranchHandler{branchService: branchService, userService: userService}
}

func (h *BranchHandler) List(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !authz.IsKnownRole(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	branches, err := h.branchService.ListBranches()
	if err != nil {
		internalError(c, "Failed to list branches")
		return
	}
	c.JSON(http.StatusOK, branches)
}

func (h *BranchHandler) GetByID(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !authz.IsKnownRole(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid branch ID")
		return
	}

	if roleID != authz.RoleSystemAdmin && roleID != authz.RoleManagement {
		me, err := h.userService.GetUserByID(userID)
		if err != nil || me == nil {
			unauthorized(c, "Unauthorized")
			return
		}
		if me.BranchID != nil && *me.BranchID != id {
			forbidden(c, "Forbidden")
			return
		}
	}

	branch, err := h.branchService.GetBranchByID(id)
	if err != nil || branch == nil {
		notFound(c, ValidationFailed, "Branch not found")
		return
	}
	c.JSON(http.StatusOK, branch)
}

func (h *BranchHandler) Create(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if roleID != authz.RoleSystemAdmin {
		forbidden(c, "Only system admin can manage branches")
		return
	}
	var req struct {
		Name     string `json:"name"`
		Code     string `json:"code"`
		Address  string `json:"address"`
		Phone    string `json:"phone"`
		Email    string `json:"email"`
		IsActive *bool  `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid branch payload")
		return
	}
	if req.Name == "" || req.Code == "" {
		badRequest(c, "name and code are required")
		return
	}
	branch := &models.Branch{Name: req.Name, Code: req.Code, Address: req.Address, Phone: req.Phone, Email: req.Email, IsActive: true}
	if req.IsActive != nil {
		branch.IsActive = *req.IsActive
	}
	if err := h.branchService.CreateBranch(branch); err != nil {
		log.Printf("Create branch error: %v", err)
		internalError(c, "Failed to create branch")
		return
	}
	c.JSON(http.StatusCreated, branch)
}

func (h *BranchHandler) Update(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if roleID != authz.RoleSystemAdmin {
		forbidden(c, "Only system admin can manage branches")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid branch ID")
		return
	}
	var req models.Branch
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid branch payload")
		return
	}
	req.ID = id
	if err := h.branchService.UpdateBranch(&req); err != nil {
		internalError(c, "Failed to update branch")
		return
	}
	updated, _ := h.branchService.GetBranchByID(id)
	c.JSON(http.StatusOK, updated)
}

func (h *BranchHandler) Delete(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if roleID != authz.RoleSystemAdmin {
		forbidden(c, "Only system admin can manage branches")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid branch ID")
		return
	}
	if err := h.branchService.DeleteBranch(id); err != nil {
		internalError(c, "Failed to delete branch")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Branch deleted"})
}

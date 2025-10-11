package handlers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type RoleHandler struct {
	service services.RoleService
}

func NewRoleHandler(service services.RoleService) *RoleHandler {
	return &RoleHandler{service: service}
}

// @Summary      Создать роль
// @Description  Создает новую роль в системе
// @Tags         Roles
// @Accept       json
// @Produce      json
// @Param        role  body      models.Role  true  "Данные новой роли"
// @Success      201   {object}  models.Role
// @Failure      400   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Router       /roles [post]
func (h *RoleHandler) CreateRole(c *gin.Context) {
	var role models.Role
	if err := c.ShouldBindJSON(&role); err != nil {
		log.Println("Bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.service.CreateRole(&role); err != nil {
		log.Println("Service error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create role"})
		return
	}
	c.JSON(http.StatusCreated, role)
}

// @Summary      Получить роль по ID
// @Description  Возвращает данные одной роли
// @Tags         Roles
// @Produce      json
// @Param        id   path      int  true  "ID роли"
// @Success      200  {object}  models.Role
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /roles/{id} [get]
func (h *RoleHandler) GetRoleByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}
	role, err := h.service.GetRoleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
		return
	}
	c.JSON(http.StatusOK, role)
}

// @Summary      Обновить роль
// @Description  Обновляет данные роли по ID
// @Tags         Roles
// @Accept       json
// @Produce      json
// @Param        id    path      int           true  "ID роли"
// @Param        role  body      models.Role   true  "Обновленные данные роли"
// @Success      200   {object}  models.Role
// @Failure      400   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Router       /roles/{id} [put]
func (h *RoleHandler) UpdateRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	var role models.Role
	if err := c.ShouldBindJSON(&role); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	role.ID = id

	if err := h.service.UpdateRole(&role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update role"})
		return
	}
	c.JSON(http.StatusOK, role)
}

// @Summary      Удалить роль
// @Description  Удаляет роль по ID
// @Tags         Roles
// @Param        id   path  int  true  "ID роли"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /roles/{id} [delete]
func (h *RoleHandler) DeleteRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}
	if err := h.service.DeleteRole(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete role"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Role deleted"})
}

// @Summary      Получить список ролей
// @Description  Возвращает список всех ролей
// @Tags         Roles
// @Produce      json
// @Param        page   query     int  false  "Page number"
// @Param        limit  query     int  false  "Page size"
// @Success      200  {array}   models.Role
// @Failure      500  {object}  map[string]string
// @Router       /roles [get]
func (h *RoleHandler) ListRoles(c *gin.Context) {
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

	roles, err := h.service.ListRoles(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list roles"})
		return
	}
	c.JSON(http.StatusOK, roles)
}

// @Summary      Получить количество ролей
// @Description  Возвращает общее количество ролей в системе
// @Tags         Roles
// @Produce      json
// @Success      200  {object}  map[string]int
// @Failure      500  {object}  map[string]string
// @Router       /roles/count [get]
func (h *RoleHandler) GetRoleCount(c *gin.Context) {
	count, err := h.service.GetRoleCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get role count"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

// @Summary      Получить роли с количеством пользователей
// @Description  Возвращает список ролей с количеством пользователей для каждой роли
// @Tags         Roles
// @Produce      json
// @Success      200  {array}   object
// @Failure      500  {object}  map[string]string
// @Router       /roles/with-user-counts [get]
func (h *RoleHandler) GetRolesWithUserCounts(c *gin.Context) {
	rolesWithCounts, err := h.service.GetRolesWithUserCounts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get roles with user counts"})
		return
	}
	c.JSON(http.StatusOK, rolesWithCounts)
}

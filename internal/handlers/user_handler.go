package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type UserHandler struct {
	service     services.UserService
	authService services.AuthService
}

func NewUserHandler(service services.UserService, authService services.AuthService) *UserHandler {
	return &UserHandler{service: service, authService: authService}
}

// @Summary      Создать пользователя
// @Description  Создает нового пользователя в системе
// @Tags         Users
// @Accept       json
// @Produce      json
// @Param        user  body      models.User  true  "Данные нового пользователя"
// @Success      201   {object}  models.User
// @Failure      400   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Router       /users [post]
func (h *UserHandler) CreateUser(c *gin.Context) {
	var user models.User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.service.CreateUser(&user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}
	c.JSON(http.StatusCreated, user)
}

// @Summary      Получить пользователя по ID
// @Description  Возвращает данные одного пользователя
// @Tags         Users
// @Produce      json
// @Param        id   path      int  true  "ID пользователя"
// @Success      200  {object}  models.User
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /users/{id} [get]
func (h *UserHandler) GetUserByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	user, err := h.service.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

// @Summary      Обновить пользователя
// @Description  Обновляет данные пользователя по ID
// @Tags         Users
// @Accept       json
// @Produce      json
// @Param        id    path      int           true  "ID пользователя"
// @Param        user  body      models.User   true  "Обновленные данные пользователя"
// @Success      200   {object}  models.User
// @Failure      400   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Router       /users/{id} [put]
func (h *UserHandler) UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var user models.User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user.ID = id

	if err := h.service.UpdateUser(&user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}
	c.JSON(http.StatusOK, user)
}

// @Summary      Удалить пользователя
// @Description  Удаляет пользователя по ID
// @Tags         Users
// @Param        id   path  int  true  "ID пользователя"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /users/{id} [delete]
func (h *UserHandler) DeleteUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	if err := h.service.DeleteUser(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "User deleted"})
}

// @Summary      Получить список пользователей
// @Description  Возвращает список всех пользователей
// @Tags         Users
// @Produce      json
// @Param        page   query     int  false  "Page number"
// @Param        limit  query     int  false  "Page size"
// @Success      200  {array}   models.User
// @Failure      500  {object}  map[string]string
// @Router       /users [get]
func (h *UserHandler) ListUsers(c *gin.Context) {
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list users"})
		return
	}
	c.JSON(http.StatusOK, users)
}

// @Summary      Получить количество пользователей
// @Description  Возвращает общее количество пользователей в системе
// @Tags         Users
// @Produce      json
// @Success      200  {object}  map[string]int
// @Failure      500  {object}  map[string]string
// @Router       /users/count [get]
func (h *UserHandler) GetUserCount(c *gin.Context) {
	count, err := h.service.GetUserCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user count"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

// @Summary      Получить количество пользователей по роли
// @Description  Возвращает количество пользователей с указанной ролью
// @Tags         Users
// @Produce      json
// @Param        role_id  path  int  true  "ID роли"
// @Success      200      {object}  map[string]int
// @Failure      400      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /users/count/role/{role_id} [get]
func (h *UserHandler) GetUserCountByRole(c *gin.Context) {
	roleIDStr := c.Param("role_id")
	roleID, err := strconv.Atoi(roleIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	count, err := h.service.GetUserCountByRole(roleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user count by role"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count, "role_id": roleID})
}

// @Summary      Регистрация пользователя
// @Description  Регистрирует нового пользователя (публичный доступ)
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        user  body      models.User  true  "Данные нового пользователя"
// @Success      201   {object}  models.User
// @Failure      400   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Router       /register [post]
func (h *UserHandler) Register(c *gin.Context) {
	var user models.User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user.RoleID = 2 // Стадартное значение роли для регистрации
	if err := h.service.CreateUser(&user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user"})
		return
	}
	c.JSON(http.StatusCreated, user)
}

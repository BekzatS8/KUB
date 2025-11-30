package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type ClientHandler struct {
	Service *services.ClientService
}

type createClientRequest struct {
	Name        string `json:"name" binding:"required"`
	BinIin      string `json:"bin_iin"`
	Address     string `json:"address"`
	ContactInfo string `json:"contact_info"`
}

type updateClientRequest struct {
	Name        string `json:"name" binding:"required"`
	BinIin      string `json:"bin_iin"`
	Address     string `json:"address"`
	ContactInfo string `json:"contact_info"`
}

func NewClientHandler(service *services.ClientService) *ClientHandler {
	return &ClientHandler{Service: service}
}

func (h *ClientHandler) Create(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if authz.IsReadOnly(roleID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "read-only role"})
		return
	}

	var req createClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client := &models.Client{
		Name:        req.Name,
		BinIin:      req.BinIin,
		Address:     req.Address,
		ContactInfo: req.ContactInfo,
		CreatedAt:   time.Now(),
	}
	id, err := h.Service.Create(client)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client.ID = int(id)
	c.JSON(http.StatusCreated, client)
}

func (h *ClientHandler) Update(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if authz.IsReadOnly(roleID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "read-only role"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	existing, err := h.Service.GetByID(id)
	if err != nil || existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}

	var req updateClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing.Name = req.Name
	existing.BinIin = req.BinIin
	existing.Address = req.Address
	existing.ContactInfo = req.ContactInfo

	if err := h.Service.Update(existing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

func (h *ClientHandler) GetByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	client, err := h.Service.GetByID(id)
	if err != nil || client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}
	c.JSON(http.StatusOK, client)
}

func (h *ClientHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "100"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 100
	}
	offset := (page - 1) * size

	clients, err := h.Service.List(size, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, clients)
}

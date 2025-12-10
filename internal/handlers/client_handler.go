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
	// Для компаний
	Name string `json:"name"`

	// Физлицо — анкета
	LastName   string `json:"last_name"`
	FirstName  string `json:"first_name"`
	MiddleName string `json:"middle_name"`

	BinIin string `json:"bin_iin"`
	IIN    string `json:"iin"`

	IDNumber       string `json:"id_number"`
	PassportSeries string `json:"passport_series"`
	PassportNumber string `json:"passport_number"`

	Phone               string `json:"phone"`
	Email               string `json:"email"`
	Address             string `json:"address"`
	RegistrationAddress string `json:"registration_address"`
	ActualAddress       string `json:"actual_address"`

	ContactInfo string `json:"contact_info"`
}

type updateClientRequest struct {
	Name       string `json:"name"`
	LastName   string `json:"last_name"`
	FirstName  string `json:"first_name"`
	MiddleName string `json:"middle_name"`

	BinIin string `json:"bin_iin"`
	IIN    string `json:"iin"`

	IDNumber       string `json:"id_number"`
	PassportSeries string `json:"passport_series"`
	PassportNumber string `json:"passport_number"`

	Phone               string `json:"phone"`
	Email               string `json:"email"`
	Address             string `json:"address"`
	RegistrationAddress string `json:"registration_address"`
	ActualAddress       string `json:"actual_address"`

	ContactInfo string `json:"contact_info"`
}

func NewClientHandler(service *services.ClientService) *ClientHandler {
	return &ClientHandler{Service: service}
}

// POST /clients
func (h *ClientHandler) Create(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	var req createClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid client payload")
		return
	}

	client := &models.Client{
		Name:                req.Name,
		BinIin:              req.BinIin,
		Address:             req.Address,
		ContactInfo:         req.ContactInfo,
		LastName:            req.LastName,
		FirstName:           req.FirstName,
		MiddleName:          req.MiddleName,
		IIN:                 req.IIN,
		IDNumber:            req.IDNumber,
		PassportSeries:      req.PassportSeries,
		PassportNumber:      req.PassportNumber,
		Phone:               req.Phone,
		Email:               req.Email,
		RegistrationAddress: req.RegistrationAddress,
		ActualAddress:       req.ActualAddress,
		CreatedAt:           time.Now(),
	}

	id, err := h.Service.Create(client)
	if err != nil {
		badRequest(c, "Failed to create client")
		return
	}
	client.ID = int(id)
	c.JSON(http.StatusCreated, client)
}

// PUT /clients/:id
func (h *ClientHandler) Update(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		badRequest(c, "Invalid client ID")
		return
	}

	current, err := h.Service.GetByID(id)
	if err != nil || current == nil {
		notFound(c, ClientNotFoundCode, "Client not found")
		return
	}

	var req updateClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid client payload")
		return
	}

	current.Name = req.Name
	current.BinIin = req.BinIin
	current.Address = req.Address
	current.ContactInfo = req.ContactInfo
	current.LastName = req.LastName
	current.FirstName = req.FirstName
	current.MiddleName = req.MiddleName
	current.IIN = req.IIN
	current.IDNumber = req.IDNumber
	current.PassportSeries = req.PassportSeries
	current.PassportNumber = req.PassportNumber
	current.Phone = req.Phone
	current.Email = req.Email
	current.RegistrationAddress = req.RegistrationAddress
	current.ActualAddress = req.ActualAddress

	if err := h.Service.Update(current); err != nil {
		badRequest(c, "Failed to update client")
		return
	}
	c.JSON(http.StatusOK, current)
}

// GET /clients/:id
func (h *ClientHandler) GetByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		badRequest(c, "Invalid client ID")
		return
	}
	client, err := h.Service.GetByID(id)
	if err != nil || client == nil {
		notFound(c, ClientNotFoundCode, "Client not found")
		return
	}
	c.JSON(http.StatusOK, client)
}

// GET /clients?page=&size=
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
		internalError(c, "Failed to list clients")
		return
	}
	c.JSON(http.StatusOK, clients)
}

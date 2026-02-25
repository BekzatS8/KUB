package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

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

	Country            string `json:"country"`
	TripPurpose        string `json:"trip_purpose"`
	BirthDate          string `json:"birth_date"`
	BirthPlace         string `json:"birth_place"`
	Citizenship        string `json:"citizenship"`
	Sex                string `json:"sex"`
	MaritalStatus      string `json:"marital_status"`
	PassportIssueDate  string `json:"passport_issue_date"`
	PassportExpireDate string `json:"passport_expire_date"`

	PreviousLastName        string          `json:"previous_last_name"`
	SpouseName              string          `json:"spouse_name"`
	SpouseContacts          string          `json:"spouse_contacts"`
	HasChildren             *bool           `json:"has_children"`
	ChildrenList            json.RawMessage `json:"children_list"`
	Education               string          `json:"education"`
	Job                     string          `json:"job"`
	TripsLast5Years         string          `json:"trips_last5_years"`
	RelativesInDestination  string          `json:"relatives_in_destination"`
	TrustedPerson           string          `json:"trusted_person"`
	Height                  *int16          `json:"height"`
	Weight                  *int16          `json:"weight"`
	DriverLicenseCategories json.RawMessage `json:"driver_license_categories"`
	TherapistName           string          `json:"therapist_name"`
	ClinicName              string          `json:"clinic_name"`
	DiseasesLast3Years      string          `json:"diseases_last3_years"`
	AdditionalInfo          string          `json:"additional_info"`

	ContactInfo string `json:"contact_info"`

	ClientType string `json:"client_type"`
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

	Country            string `json:"country"`
	TripPurpose        string `json:"trip_purpose"`
	BirthDate          string `json:"birth_date"`
	BirthPlace         string `json:"birth_place"`
	Citizenship        string `json:"citizenship"`
	Sex                string `json:"sex"`
	MaritalStatus      string `json:"marital_status"`
	PassportIssueDate  string `json:"passport_issue_date"`
	PassportExpireDate string `json:"passport_expire_date"`

	PreviousLastName        *string          `json:"previous_last_name"`
	SpouseName              *string          `json:"spouse_name"`
	SpouseContacts          *string          `json:"spouse_contacts"`
	HasChildren             *bool            `json:"has_children"`
	ChildrenList            *json.RawMessage `json:"children_list"`
	Education               *string          `json:"education"`
	Job                     *string          `json:"job"`
	TripsLast5Years         *string          `json:"trips_last5_years"`
	RelativesInDestination  *string          `json:"relatives_in_destination"`
	TrustedPerson           *string          `json:"trusted_person"`
	Height                  *int16           `json:"height"`
	Weight                  *int16           `json:"weight"`
	DriverLicenseCategories *json.RawMessage `json:"driver_license_categories"`
	TherapistName           *string          `json:"therapist_name"`
	ClinicName              *string          `json:"clinic_name"`
	DiseasesLast3Years      *string          `json:"diseases_last3_years"`
	AdditionalInfo          *string          `json:"additional_info"`

	ContactInfo string `json:"contact_info"`

	ClientType *string `json:"client_type"`
}

func NewClientHandler(service *services.ClientService) *ClientHandler {
	return &ClientHandler{Service: service}
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == "23505"
	}
	return false
}

func parseDateField(field, value string, required bool) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return nil, fmt.Errorf("field %s is required and must be in YYYY-MM-DD format", field)
		}
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: expected YYYY-MM-DD", field)
	}
	return &t, nil
}

func collectMissingRedFields(req createClientRequest) []string {
	missing := make([]string, 0)
	trim := strings.TrimSpace
	if strings.ToLower(trim(req.ClientType)) == models.ClientTypeLegal {
		if trim(req.Name) == "" {
			missing = append(missing, "name")
		}
		if trim(req.BinIin) == "" {
			missing = append(missing, "bin_iin")
		}
		if trim(req.Phone) == "" {
			missing = append(missing, "phone")
		}
		return missing
	}
	if trim(req.Country) == "" {
		missing = append(missing, "country")
	}
	if trim(req.TripPurpose) == "" {
		missing = append(missing, "trip_purpose")
	}
	if trim(req.BirthDate) == "" {
		missing = append(missing, "birth_date")
	}
	if trim(req.Phone) == "" {
		missing = append(missing, "phone")
	}
	if trim(req.LastName) == "" {
		missing = append(missing, "last_name")
	}
	if trim(req.FirstName) == "" {
		missing = append(missing, "first_name")
	}
	return missing
}

func buildClientFromCreateRequest(req createClientRequest, userID int, birthDate, passportIssueDate, passportExpireDate *time.Time) *models.Client {
	return &models.Client{
		OwnerID:                 userID,
		Name:                    req.Name,
		BinIin:                  req.BinIin,
		Address:                 req.Address,
		ContactInfo:             req.ContactInfo,
		ClientType:              req.ClientType,
		LastName:                req.LastName,
		FirstName:               req.FirstName,
		MiddleName:              req.MiddleName,
		IIN:                     req.IIN,
		IDNumber:                req.IDNumber,
		PassportSeries:          req.PassportSeries,
		PassportNumber:          req.PassportNumber,
		Phone:                   req.Phone,
		Email:                   req.Email,
		RegistrationAddress:     req.RegistrationAddress,
		ActualAddress:           req.ActualAddress,
		Country:                 req.Country,
		TripPurpose:             req.TripPurpose,
		BirthDate:               birthDate,
		BirthPlace:              req.BirthPlace,
		Citizenship:             req.Citizenship,
		Sex:                     req.Sex,
		MaritalStatus:           req.MaritalStatus,
		PassportIssueDate:       passportIssueDate,
		PassportExpireDate:      passportExpireDate,
		PreviousLastName:        req.PreviousLastName,
		SpouseName:              req.SpouseName,
		SpouseContacts:          req.SpouseContacts,
		HasChildren:             req.HasChildren,
		ChildrenList:            req.ChildrenList,
		Education:               req.Education,
		Job:                     req.Job,
		TripsLast5Years:         req.TripsLast5Years,
		RelativesInDestination:  req.RelativesInDestination,
		TrustedPerson:           req.TrustedPerson,
		Height:                  req.Height,
		Weight:                  req.Weight,
		DriverLicenseCategories: req.DriverLicenseCategories,
		TherapistName:           req.TherapistName,
		ClinicName:              req.ClinicName,
		DiseasesLast3Years:      req.DiseasesLast3Years,
		AdditionalInfo:          req.AdditionalInfo,
		CreatedAt:               time.Now(),
	}
}

// POST /clients
func (h *ClientHandler) Create(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	var req createClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		missing := collectMissingRedFields(req)
		if len(missing) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error_code":     BadRequestCode,
				"message":        "Missing required fields",
				"missing_fields": missing,
			})
			return
		}
		badRequest(c, "Invalid client payload")
		return
	}

	missing := collectMissingRedFields(req)
	if len(missing) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error_code":     BadRequestCode,
			"message":        "Missing required fields",
			"missing_fields": missing,
		})
		return
	}

	requiredBirthDate := strings.ToLower(strings.TrimSpace(req.ClientType)) != models.ClientTypeLegal
	birthDate, err := parseDateField("birth_date", req.BirthDate, requiredBirthDate)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	passportIssueDate, err := parseDateField("passport_issue_date", req.PassportIssueDate, false)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	passportExpireDate, err := parseDateField("passport_expire_date", req.PassportExpireDate, false)
	if err != nil {
		badRequest(c, err.Error())
		return
	}

	client := buildClientFromCreateRequest(req, userID, birthDate, passportIssueDate, passportExpireDate)

	id, err := h.Service.Create(client, userID, roleID)
	if err != nil {
		if isUniqueViolation(err) {
			conflict(c, ConflictCode, "Client with the same BIN/IIN already exists")
			return
		}
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
		var missingErr *services.MissingFieldsError
		if errors.As(err, &missingErr) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error_code":     BadRequestCode,
				"message":        missingErr.Error(),
				"missing_fields": missingErr.Fields,
			})
			return
		}
		if strings.Contains(err.Error(), "invalid client_type") {
			badRequest(c, err.Error())
			return
		}
		badRequest(c, "Failed to create client")
		return
	}
	client.ID = int(id)
	c.JSON(http.StatusCreated, client)
}

// PUT /clients/:id
func (h *ClientHandler) Update(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		badRequest(c, "Invalid client ID")
		return
	}

	current, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || current == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, ClientNotFoundCode, "Client not found")
		return
	}

	var req updateClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid client payload")
		return
	}

	birthDate, err := parseDateField("birth_date", req.BirthDate, false)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	passportIssueDate, err := parseDateField("passport_issue_date", req.PassportIssueDate, false)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	passportExpireDate, err := parseDateField("passport_expire_date", req.PassportExpireDate, false)
	if err != nil {
		badRequest(c, err.Error())
		return
	}

	current.Name = req.Name
	current.BinIin = req.BinIin
	current.Address = req.Address
	current.ContactInfo = req.ContactInfo
	if req.ClientType != nil {
		current.ClientType = *req.ClientType
	}
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
	current.Country = req.Country
	current.TripPurpose = req.TripPurpose
	current.BirthDate = birthDate
	current.BirthPlace = req.BirthPlace
	current.Citizenship = req.Citizenship
	current.Sex = req.Sex
	current.MaritalStatus = req.MaritalStatus
	current.PassportIssueDate = passportIssueDate
	current.PassportExpireDate = passportExpireDate

	if req.PreviousLastName != nil {
		current.PreviousLastName = *req.PreviousLastName
	}
	if req.SpouseName != nil {
		current.SpouseName = *req.SpouseName
	}
	if req.SpouseContacts != nil {
		current.SpouseContacts = *req.SpouseContacts
	}
	if req.HasChildren != nil {
		v := *req.HasChildren
		current.HasChildren = &v
	}
	if req.ChildrenList != nil {
		if *req.ChildrenList == nil {
			current.ChildrenList = nil
		} else {
			current.ChildrenList = append(json.RawMessage(nil), (*req.ChildrenList)...)
		}
	}
	if req.Education != nil {
		current.Education = *req.Education
	}
	if req.Job != nil {
		current.Job = *req.Job
	}
	if req.TripsLast5Years != nil {
		current.TripsLast5Years = *req.TripsLast5Years
	}
	if req.RelativesInDestination != nil {
		current.RelativesInDestination = *req.RelativesInDestination
	}
	if req.TrustedPerson != nil {
		current.TrustedPerson = *req.TrustedPerson
	}
	if req.Height != nil {
		v := *req.Height
		current.Height = &v
	}
	if req.Weight != nil {
		v := *req.Weight
		current.Weight = &v
	}
	if req.DriverLicenseCategories != nil {
		if *req.DriverLicenseCategories == nil {
			current.DriverLicenseCategories = nil
		} else {
			current.DriverLicenseCategories = append(json.RawMessage(nil), (*req.DriverLicenseCategories)...)
		}
	}
	if req.TherapistName != nil {
		current.TherapistName = *req.TherapistName
	}
	if req.ClinicName != nil {
		current.ClinicName = *req.ClinicName
	}
	if req.DiseasesLast3Years != nil {
		current.DiseasesLast3Years = *req.DiseasesLast3Years
	}
	if req.AdditionalInfo != nil {
		current.AdditionalInfo = *req.AdditionalInfo
	}

	if err := h.Service.Update(current, userID, roleID); err != nil {
		if isUniqueViolation(err) {
			conflict(c, ConflictCode, "Client with the same BIN/IIN already exists")
			return
		}
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
		if strings.Contains(err.Error(), "invalid client_type") {
			badRequest(c, err.Error())
			return
		}
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
	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	client, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || client == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, ClientNotFoundCode, "Client not found")
		return
	}
	c.JSON(http.StatusOK, client)
}

// GET /clients?page=&size=
func (h *ClientHandler) List(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	if roleID == authz.RoleSales {
		forbidden(c, "sales cannot access full list")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "100"))

	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 100
	}
	offset := (page - 1) * size

	clientType := strings.TrimSpace(c.Query("client_type"))
	clients, err := h.Service.ListForRole(userID, roleID, size, offset, clientType)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if strings.Contains(err.Error(), "invalid client_type") {
			badRequest(c, err.Error())
			return
		}
		internalError(c, "Failed to list clients")
		return
	}
	c.JSON(http.StatusOK, clients)
}

// GET /clients/my?page=&size=
func (h *ClientHandler) ListMy(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "100"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 100
	}
	offset := (page - 1) * size

	clients, err := h.Service.ListMine(userID, size, offset, "")
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if strings.Contains(err.Error(), "invalid client_type") {
			badRequest(c, err.Error())
			return
		}
		internalError(c, "Failed to list clients")
		return
	}
	c.JSON(http.StatusOK, clients)
}

// GET /clients/:id/completeness
func (h *ClientHandler) GetCompleteness(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		badRequest(c, "Invalid client ID")
		return
	}
	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}

	missing, err := h.Service.GetMissingYellow(c.Request.Context(), id, userID, roleID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, ClientNotFoundCode, "Client not found")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"client_id":      id,
		"missing_yellow": missing,
		"yellow_ready":   len(missing) == 0,
	})
}

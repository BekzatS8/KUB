package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type clientService interface {
	Create(c *models.Client, userID, roleID int) (int64, error)
	Update(c *models.Client, userID, roleID int) error
	Delete(id int, userID, roleID int) error
	Patch(id int, updates map[string]any, userID, roleID int) (*models.Client, error)
	GetByID(id int, userID, roleID int) (*models.Client, error)
	ListForRole(userID, roleID, limit, offset int, clientType string) ([]*models.Client, error)
	ListMine(userID, limit, offset int, clientType string) ([]*models.Client, error)
	ListIndividualsForRole(userID, roleID, limit, offset int, q string) ([]*models.Client, error)
	ListCompaniesForRole(userID, roleID, limit, offset int, q string) ([]*models.Client, error)
	GetMissingYellow(ctx context.Context, clientID, userID, roleID int) ([]string, error)
	GetProfile(ctx context.Context, clientID, userID, roleID int) (*services.ClientProfilePayload, error)
}

type ClientHandler struct {
	Service clientService
}

type createClientRequest struct {
	Name       string `json:"name"`
	LastName   string `json:"last_name"`
	FirstName  string `json:"first_name"`
	MiddleName string `json:"middle_name"`
	BinIin     string `json:"bin_iin"`
	IIN        string `json:"iin"`

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

	ContactInfo       string                          `json:"contact_info"`
	ClientType        string                          `json:"client_type"`
	IndividualProfile *models.ClientIndividualProfile `json:"individual_profile"`
	LegalProfile      *models.ClientLegalProfile      `json:"legal_profile"`
}

type updateClientRequest struct {
	Name       string `json:"name"`
	LastName   string `json:"last_name"`
	FirstName  string `json:"first_name"`
	MiddleName string `json:"middle_name"`
	BinIin     string `json:"bin_iin"`
	IIN        string `json:"iin"`

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

	ContactInfo       string                          `json:"contact_info"`
	ClientType        *string                         `json:"client_type"`
	IndividualProfile *models.ClientIndividualProfile `json:"individual_profile"`
	LegalProfile      *models.ClientLegalProfile      `json:"legal_profile"`
}

type patchClientRequest struct {
	Name       *string `json:"name"`
	LastName   *string `json:"last_name"`
	FirstName  *string `json:"first_name"`
	MiddleName *string `json:"middle_name"`
	BinIin     *string `json:"bin_iin"`
	IIN        *string `json:"iin"`

	IDNumber       *string `json:"id_number"`
	PassportSeries *string `json:"passport_series"`
	PassportNumber *string `json:"passport_number"`

	Phone               *string `json:"phone"`
	Email               *string `json:"email"`
	Address             *string `json:"address"`
	RegistrationAddress *string `json:"registration_address"`
	ActualAddress       *string `json:"actual_address"`

	Country            *string `json:"country"`
	TripPurpose        *string `json:"trip_purpose"`
	BirthDate          *string `json:"birth_date"`
	BirthPlace         *string `json:"birth_place"`
	Citizenship        *string `json:"citizenship"`
	Sex                *string `json:"sex"`
	MaritalStatus      *string `json:"marital_status"`
	PassportIssueDate  *string `json:"passport_issue_date"`
	PassportExpireDate *string `json:"passport_expire_date"`
	ContactInfo        *string `json:"contact_info"`
	ClientType         *string `json:"client_type"`
}

func NewClientHandler(service *services.ClientService) *ClientHandler {
	return &ClientHandler{Service: service}
}

func parseDateField(field, value string, required bool) (*time.Time, error) {
	v, err := parseFlexibleDate(field, value, required)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	t := *v
	return &t, nil
}

func collectMissingRedFields(req createClientRequest) []string {
	missing := make([]string, 0)
	trim := strings.TrimSpace
	if strings.ToLower(trim(req.ClientType)) == models.ClientTypeLegal {
		companyName := trim(req.Name)
		bin := trim(req.BinIin)
		contactName := ""
		contactPhone := trim(req.Phone)
		legalAddress := trim(req.Address)
		if req.LegalProfile != nil {
			if trim(req.LegalProfile.CompanyName) != "" {
				companyName = trim(req.LegalProfile.CompanyName)
			}
			if trim(req.LegalProfile.BIN) != "" {
				bin = trim(req.LegalProfile.BIN)
			}
			contactName = trim(req.LegalProfile.ContactPersonName)
			if trim(req.LegalProfile.ContactPersonPhone) != "" {
				contactPhone = trim(req.LegalProfile.ContactPersonPhone)
			}
			if trim(req.LegalProfile.LegalAddress) != "" {
				legalAddress = trim(req.LegalProfile.LegalAddress)
			}
		}
		if companyName == "" {
			missing = append(missing, "company_name")
		}
		if bin == "" {
			missing = append(missing, "bin")
		}
		if contactName == "" {
			missing = append(missing, "contact_person_name")
		}
		if contactPhone == "" {
			missing = append(missing, "contact_person_phone")
		}
		if legalAddress == "" {
			missing = append(missing, "legal_address")
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
	client := &models.Client{OwnerID: userID, Name: req.Name, BinIin: req.BinIin, Address: req.Address, ContactInfo: req.ContactInfo, ClientType: req.ClientType, LastName: req.LastName, FirstName: req.FirstName, MiddleName: req.MiddleName, IIN: req.IIN, IDNumber: req.IDNumber, PassportSeries: req.PassportSeries, PassportNumber: req.PassportNumber, Phone: req.Phone, Email: req.Email, RegistrationAddress: req.RegistrationAddress, ActualAddress: req.ActualAddress, Country: req.Country, TripPurpose: req.TripPurpose, BirthDate: birthDate, BirthPlace: req.BirthPlace, Citizenship: req.Citizenship, Sex: req.Sex, MaritalStatus: req.MaritalStatus, PassportIssueDate: passportIssueDate, PassportExpireDate: passportExpireDate, PreviousLastName: req.PreviousLastName, SpouseName: req.SpouseName, SpouseContacts: req.SpouseContacts, HasChildren: req.HasChildren, ChildrenList: req.ChildrenList, Education: req.Education, Job: req.Job, TripsLast5Years: req.TripsLast5Years, RelativesInDestination: req.RelativesInDestination, TrustedPerson: req.TrustedPerson, Height: req.Height, Weight: req.Weight, DriverLicenseCategories: req.DriverLicenseCategories, TherapistName: req.TherapistName, ClinicName: req.ClinicName, DiseasesLast3Years: req.DiseasesLast3Years, AdditionalInfo: req.AdditionalInfo, CreatedAt: time.Now()}
	client.IndividualProfile = req.IndividualProfile
	client.LegalProfile = req.LegalProfile
	return client
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
		badRequest(c, "Invalid client payload")
		return
	}

	missing := collectMissingRedFields(req)
	if len(missing) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error_code": BadRequestCode, "message": "Missing required fields", "missing_fields": missing})
		return
	}

	requiredBirthDate := strings.ToLower(strings.TrimSpace(req.ClientType)) != models.ClientTypeLegal
	birthDate, err := parseDateField("birth_date", req.BirthDate, requiredBirthDate)
	if err != nil {
		writeDateError(c, err)
		return
	}
	passportIssueDate, err := parseDateField("passport_issue_date", req.PassportIssueDate, false)
	if err != nil {
		writeDateError(c, err)
		return
	}
	passportExpireDate, err := parseDateField("passport_expire_date", req.PassportExpireDate, false)
	if err != nil {
		writeDateError(c, err)
		return
	}

	client := buildClientFromCreateRequest(req, userID, birthDate, passportIssueDate, passportExpireDate)
	id, err := h.Service.Create(client, userID, roleID)
	if err != nil {
		if errors.Is(err, services.ErrClientAlreadyExists) {
			conflict(c, ClientAlreadyExists, "Client with the same BIN/IIN already exists")
			return
		}
		if errors.Is(err, services.ErrIndividualIINExists) {
			conflict(c, ConflictCode, "Individual profile with this IIN already exists")
			return
		}
		if errors.Is(err, services.ErrLegalBINExists) {
			conflict(c, ConflictCode, "Legal profile with this BIN already exists")
			return
		}
		if errors.Is(err, services.ErrInvalidEmail) {
			badRequestWithCode(c, InvalidEmailCode, "Email has invalid format")
			return
		}
		if errors.Is(err, services.ErrEmailAlreadyUsed) {
			conflict(c, EmailAlreadyUsedCode, "Email already used by another client")
			return
		}
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
		var missingErr *services.MissingFieldsError
		if errors.As(err, &missingErr) {
			c.JSON(http.StatusBadRequest, gin.H{"error_code": BadRequestCode, "message": missingErr.Error(), "missing_fields": missingErr.Fields})
			return
		}
		if errors.Is(err, services.ErrInvalidClientType) {
			badRequest(c, "invalid client_type: allowed values are individual, legal")
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
		writeDateError(c, err)
		return
	}
	passportIssueDate, err := parseDateField("passport_issue_date", req.PassportIssueDate, false)
	if err != nil {
		writeDateError(c, err)
		return
	}
	passportExpireDate, err := parseDateField("passport_expire_date", req.PassportExpireDate, false)
	if err != nil {
		writeDateError(c, err)
		return
	}
	current.Name, current.BinIin, current.Address, current.ContactInfo = req.Name, req.BinIin, req.Address, req.ContactInfo
	if req.ClientType != nil {
		current.ClientType = *req.ClientType
	}
	current.LastName, current.FirstName, current.MiddleName = req.LastName, req.FirstName, req.MiddleName
	current.IIN, current.IDNumber, current.PassportSeries, current.PassportNumber = req.IIN, req.IDNumber, req.PassportSeries, req.PassportNumber
	current.Phone, current.Email = req.Phone, req.Email
	current.RegistrationAddress, current.ActualAddress = req.RegistrationAddress, req.ActualAddress
	current.Country, current.TripPurpose, current.BirthDate = req.Country, req.TripPurpose, birthDate
	current.BirthPlace, current.Citizenship, current.Sex, current.MaritalStatus = req.BirthPlace, req.Citizenship, req.Sex, req.MaritalStatus
	current.PassportIssueDate, current.PassportExpireDate = passportIssueDate, passportExpireDate
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
	if req.IndividualProfile != nil {
		current.IndividualProfile = req.IndividualProfile
	}
	if req.LegalProfile != nil {
		current.LegalProfile = req.LegalProfile
	}
	if err := h.Service.Update(current, userID, roleID); err != nil {
		if errors.Is(err, services.ErrClientAlreadyExists) {
			conflict(c, ClientAlreadyExists, "Client with the same BIN/IIN already exists")
			return
		}
		if errors.Is(err, services.ErrIndividualIINExists) {
			conflict(c, ConflictCode, "Individual profile with this IIN already exists")
			return
		}
		if errors.Is(err, services.ErrLegalBINExists) {
			conflict(c, ConflictCode, "Legal profile with this BIN already exists")
			return
		}
		if errors.Is(err, services.ErrInvalidEmail) {
			badRequestWithCode(c, InvalidEmailCode, "Email has invalid format")
			return
		}
		if errors.Is(err, services.ErrEmailAlreadyUsed) {
			conflict(c, EmailAlreadyUsedCode, "Email already used by another client")
			return
		}
		if errors.Is(err, services.ErrClientNotFound) {
			notFound(c, ClientNotFoundCode, "Client not found")
			return
		}
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
		if errors.Is(err, services.ErrInvalidClientType) {
			badRequest(c, "invalid client_type: allowed values are individual, legal")
			return
		}
		if errors.Is(err, services.ErrClientTypeImmutable) {
			conflict(c, ConflictCode, services.ErrClientTypeImmutable.Error())
			return
		}
		badRequest(c, "Failed to update client")
		return
	}
	c.JSON(http.StatusOK, current)
}

// DELETE /clients/:id
func (h *ClientHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}

	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	err = h.Service.Delete(id, userID, roleID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
		if errors.Is(err, services.ErrClientNotFound) {
			notFound(c, ClientNotFoundCode, "Client not found")
			return
		}
		if errors.Is(err, services.ErrClientInUse) {
			conflict(c, ClientInUseCode, "Client has linked deals/documents and cannot be deleted")
			return
		}
		internalError(c, "Failed to delete client")
		return
	}

	c.Status(http.StatusNoContent)
}

// PATCH /clients/:id
func (h *ClientHandler) Patch(c *gin.Context) {
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
	var req patchClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid client payload")
		return
	}
	updates := make(map[string]any)
	addS := func(k string, v *string) {
		if v != nil {
			updates[k] = *v
		}
	}
	addS("name", req.Name)
	addS("last_name", req.LastName)
	addS("first_name", req.FirstName)
	addS("middle_name", req.MiddleName)
	addS("bin_iin", req.BinIin)
	addS("iin", req.IIN)
	addS("id_number", req.IDNumber)
	addS("passport_series", req.PassportSeries)
	addS("passport_number", req.PassportNumber)
	addS("phone", req.Phone)
	addS("email", req.Email)
	addS("address", req.Address)
	addS("registration_address", req.RegistrationAddress)
	addS("actual_address", req.ActualAddress)
	addS("country", req.Country)
	addS("trip_purpose", req.TripPurpose)
	addS("birth_place", req.BirthPlace)
	addS("citizenship", req.Citizenship)
	addS("sex", req.Sex)
	addS("marital_status", req.MaritalStatus)
	addS("contact_info", req.ContactInfo)
	addS("client_type", req.ClientType)

	dateFields := map[string]*string{"birth_date": req.BirthDate, "passport_issue_date": req.PassportIssueDate, "passport_expire_date": req.PassportExpireDate}
	for field, ptr := range dateFields {
		if ptr == nil {
			continue
		}
		t, err := parseFlexibleDate(field, *ptr, false)
		if err != nil {
			writeDateError(c, err)
			return
		}
		updates[field] = t
	}
	updated, err := h.Service.Patch(id, updates, userID, roleID)
	if err != nil {
		if errors.Is(err, services.ErrClientNotFound) {
			notFound(c, ClientNotFoundCode, "Client not found")
			return
		}
		if errors.Is(err, services.ErrInvalidEmail) {
			badRequestWithCode(c, InvalidEmailCode, "Email has invalid format")
			return
		}
		if errors.Is(err, services.ErrEmailAlreadyUsed) {
			conflict(c, EmailAlreadyUsedCode, "Email already used by another client")
			return
		}
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
		if errors.Is(err, services.ErrInvalidClientType) {
			badRequest(c, "invalid client_type: allowed values are individual, legal")
			return
		}
		if errors.Is(err, services.ErrClientTypeImmutable) {
			conflict(c, ConflictCode, services.ErrClientTypeImmutable.Error())
			return
		}
		badRequest(c, "Failed to update client")
		return
	}
	c.JSON(http.StatusOK, updated)
}

func writeDateError(c *gin.Context, err error) {
	var dErr *dateFieldError
	if errors.As(err, &dErr) {
		badRequestWithCode(c, InvalidDateFormatCode, dErr.Error())
		return
	}
	badRequest(c, err.Error())
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

func (h *ClientHandler) ListIndividuals(c *gin.Context) {
	h.listByPresetType(c, "individual")
}

func (h *ClientHandler) ListCompanies(c *gin.Context) {
	h.listByPresetType(c, "company")
}

func (h *ClientHandler) listByPresetType(c *gin.Context, kind string) {
	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	if roleID == authz.RoleSales {
		forbidden(c, "sales cannot access full list")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit < 1 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	q := strings.TrimSpace(c.Query("q"))

	var (
		clients []*models.Client
		err     error
	)
	switch kind {
	case "individual":
		clients, err = h.Service.ListIndividualsForRole(userID, roleID, limit, offset, q)
	case "company":
		clients, err = h.Service.ListCompaniesForRole(userID, roleID, limit, offset, q)
	default:
		badRequest(c, "invalid client list type")
		return
	}
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to list clients")
		return
	}
	c.JSON(http.StatusOK, clients)
}

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
	profile, err := h.Service.GetProfile(c.Request.Context(), id, userID, roleID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, ClientNotFoundCode, "Client not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"client_ref":     profile.ClientRef,
		"client_id":      profile.ClientRef.ClientID,
		"client_type":    profile.ClientRef.ClientType,
		"missing_yellow": profile.MissingYellow,
		"yellow_ready":   len(profile.MissingYellow) == 0,
	})
}

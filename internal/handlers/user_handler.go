package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
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
	filesRoot           string
}

type createUserRequest struct {
	CompanyName string `json:"company_name"`
	BinIin      string `json:"bin_iin"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	MiddleName  string `json:"middle_name"`
	Position    string `json:"position"`
	BranchID    *int   `json:"branch_id"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	Phone       string `json:"phone"`
	Address     string `json:"address"`
	ExtraInfo   string `json:"extra_info"`
	AvatarURL   string `json:"avatar_url"`
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
	Address     *string `json:"address"`
	ExtraInfo   *string `json:"extra_info"`
	RoleID      *int    `json:"role_id"`
	IsVerified  *bool   `json:"is_verified"`
	IsActive    *bool   `json:"is_active"`
}

var userPhoneE164Pattern = regexp.MustCompile(`^\+[1-9]\d{10,14}$`)

func NewUserHandler(service services.UserService, branchService services.BranchService, verificationService *services.UserVerificationService, filesRoot ...string) *UserHandler {
	root := "files"
	if len(filesRoot) > 0 && strings.TrimSpace(filesRoot[0]) != "" {
		root = strings.TrimSpace(filesRoot[0])
	}
	return &UserHandler{service: service, branchService: branchService, verificationService: verificationService, filesRoot: root}
}

type userResponse struct {
	ID         int         `json:"id"`
	FirstName  string      `json:"first_name,omitempty"`
	LastName   string      `json:"last_name,omitempty"`
	MiddleName string      `json:"middle_name,omitempty"`
	FullName   string      `json:"full_name"`
	Email      string      `json:"email"`
	Phone      string      `json:"phone"`
	IIN        string      `json:"iin,omitempty"`
	Address    string      `json:"address,omitempty"`
	ExtraInfo  string      `json:"extra_info,omitempty"`
	Avatar     gin.H       `json:"avatar"`
	AvatarURL  string      `json:"avatar_url,omitempty"`
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

func trimCreateUserRequest(req *createUserRequest) {
	req.CompanyName = strings.TrimSpace(req.CompanyName)
	req.BinIin = strings.TrimSpace(req.BinIin)
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.MiddleName = strings.TrimSpace(req.MiddleName)
	req.Position = strings.TrimSpace(req.Position)
	req.Email = strings.TrimSpace(req.Email)
	req.Phone = strings.TrimSpace(req.Phone)
	req.Address = strings.TrimSpace(req.Address)
	req.ExtraInfo = strings.TrimSpace(req.ExtraInfo)
	req.AvatarURL = strings.TrimSpace(req.AvatarURL)
}

func trimStringPtr(value **string) {
	if value != nil && *value != nil {
		trimmed := strings.TrimSpace(**value)
		*value = &trimmed
	}
}

func trimUpdateUserRequest(req *updateUserRequest) {
	trimStringPtr(&req.CompanyName)
	trimStringPtr(&req.BinIin)
	trimStringPtr(&req.FirstName)
	trimStringPtr(&req.LastName)
	trimStringPtr(&req.MiddleName)
	trimStringPtr(&req.Position)
	trimStringPtr(&req.Email)
	trimStringPtr(&req.Phone)
	trimStringPtr(&req.Address)
	trimStringPtr(&req.ExtraInfo)
}

func validateRequiredCreateUserFields(req createUserRequest) string {
	switch {
	case req.Email == "":
		return "Заполните email"
	case !validEmail(req.Email):
		return "Некорректный email"
	case req.Password == "":
		return "Введите пароль"
	case req.MiddleName == "":
		return "Укажите отчество"
	case req.Phone != "" && !userPhoneE164Pattern.MatchString(req.Phone):
		return "Телефон должен быть в международном формате, например +77001234567"
	case req.RoleID == 0:
		return "Выберите роль"
	case req.Position == "":
		return "Укажите должность"
	default:
		return ""
	}
}

func validateUpdateUserFields(req updateUserRequest) string {
	switch {
	case req.Email != nil && *req.Email != "" && !validEmail(*req.Email):
		return "Некорректный email"
	case req.Phone != nil && *req.Phone != "" && !userPhoneE164Pattern.MatchString(*req.Phone):
		return "Телефон должен быть в международном формате, например +77001234567"
	default:
		return ""
	}
}

func validEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}
func branchIDSelected(branchID *int) bool {
	return branchID != nil && *branchID > 0
}

func (h *UserHandler) validateRequiredBranch(branchID *int) string {
	if !branchIDSelected(branchID) {
		return "Выберите филиал"
	}
	if h.branchService == nil {
		return ""
	}
	branch, err := h.branchService.GetBranchByID(*branchID)
	if err != nil || branch == nil {
		return "Выбранный филиал не найден"
	}
	if !branch.IsActive {
		return "Выбранный филиал неактивен"
	}
	return ""
}

func (h *UserHandler) validateBranchForRole(roleID int, branchID *int) string {
	if roleID == authz.RoleSystemAdmin {
		return ""
	}
	return h.validateRequiredBranch(branchID)
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

func sameUserBranch(a, b *models.User) bool {
	return a != nil && b != nil && a.BranchID != nil && b.BranchID != nil && *a.BranchID == *b.BranchID
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
		IIN:        u.BinIin,
		Address:    u.Address,
		ExtraInfo:  u.ExtraInfo,
		AvatarURL:  u.AvatarURL,
		Avatar: gin.H{
			"url":                u.AvatarURL,
			"crop_x":             u.AvatarCropX,
			"crop_y":             u.AvatarCropY,
			"crop_scale":         u.AvatarCropScale,
			"crop_size":          u.AvatarCropSize,
			"has_original_image": u.AvatarOriginalPath != "",
		},
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
		forbidden(c, "Только системный администратор может создавать пользователей")
		return
	}
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Некорректные данные пользователя")
		return
	}
	trimCreateUserRequest(&req)
	if msg := validateRequiredCreateUserFields(req); msg != "" {
		badRequest(c, msg)
		return
	}
	newRole := req.RoleID
	if !authz.IsKnownRole(newRole) {
		badRequest(c, "Некорректная роль")
		return
	}
	if msg := h.validateBranchForRole(newRole, req.BranchID); msg != "" {
		badRequest(c, msg)
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
		Address:     req.Address,
		ExtraInfo:   req.ExtraInfo,
		AvatarURL:   req.AvatarURL,
		RoleID:      newRole,
		IsVerified:  false,
		IsActive:    true,
		IsActiveSet: true,
	}
	if req.IsVerified != nil {
		user.IsVerified = *req.IsVerified
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
		user.IsActiveSet = true
	}
	if err := h.service.CreateUserWithPassword(user, req.Password); err != nil {
		log.Printf("CreateUser: service error: %v", err)
		if errors.Is(err, services.ErrEmailAlreadyUsed) {
			conflict(c, ConflictCode, "Этот email уже используется")
			return
		}
		internalError(c, "Не удалось создать пользователя")
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

type updateProfileRequest struct {
	FirstName  *string `json:"first_name"`
	LastName   *string `json:"last_name"`
	MiddleName *string `json:"middle_name"`
	IIN        *string `json:"iin"`
	BinIin     *string `json:"bin_iin"`
	Phone      *string `json:"phone"`
	Address    *string `json:"address"`
	ExtraInfo  *string `json:"extra_info"`
}

type avatarCropRequest struct {
	CropX     *float64 `json:"crop_x"`
	CropY     *float64 `json:"crop_y"`
	CropScale *float64 `json:"crop_scale"`
	CropSize  *float64 `json:"crop_size"`
}

func trimUpdateProfileRequest(req *updateProfileRequest) {
	trimStringPtr(&req.FirstName)
	trimStringPtr(&req.LastName)
	trimStringPtr(&req.MiddleName)
	trimStringPtr(&req.IIN)
	trimStringPtr(&req.BinIin)
	trimStringPtr(&req.Phone)
	trimStringPtr(&req.Address)
	trimStringPtr(&req.ExtraInfo)
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	h.GetMyProfile(c)
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	if userID == 0 {
		unauthorized(c, "Unauthorized")
		return
	}
	current, err := h.service.GetUserByID(userID)
	if err != nil || current == nil {
		notFound(c, ClientNotFoundCode, "User not found")
		return
	}
	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Некорректные данные профиля")
		return
	}
	trimUpdateProfileRequest(&req)
	next := *current
	if req.FirstName != nil {
		next.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		next.LastName = *req.LastName
	}
	if req.MiddleName != nil {
		next.MiddleName = *req.MiddleName
	}
	if req.IIN != nil {
		next.BinIin = *req.IIN
	} else if req.BinIin != nil {
		next.BinIin = *req.BinIin
	}
	if req.Phone != nil {
		next.Phone = *req.Phone
	}
	if req.Address != nil {
		next.Address = *req.Address
	}
	if req.ExtraInfo != nil {
		next.ExtraInfo = *req.ExtraInfo
	}
	if err := h.service.UpdateProfile(userID, &next); err != nil {
		log.Printf("UpdateProfile: service error: %v", err)
		internalError(c, "Не удалось сохранить профиль")
		return
	}
	updated, _ := h.service.GetUserByID(userID)
	c.JSON(http.StatusOK, h.userToResponse(updated))
}

func (h *UserHandler) UploadProfileAvatar(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	if userID == 0 {
		unauthorized(c, "Unauthorized")
		return
	}
	current, err := h.service.GetUserByID(userID)
	if err != nil || current == nil {
		notFound(c, ClientNotFoundCode, "User not found")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)
	header, err := c.FormFile("file")
	if err != nil {
		badRequest(c, "Файл не указан")
		return
	}
	if header.Size > 10<<20 {
		badRequest(c, "Файл слишком большой")
		return
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedAvatarExt(ext) {
		badRequest(c, "Неверный формат файла")
		return
	}
	file, err := header.Open()
	if err != nil {
		internalError(c, "Не удалось открыть файл")
		return
	}
	defer file.Close()
	if err := validateAvatarMime(file, ext); err != nil {
		badRequest(c, err.Error())
		return
	}
	if seeker, ok := file.(io.Seeker); ok {
		_, _ = seeker.Seek(0, io.SeekStart)
	}
	name, err := randomHex(16)
	if err != nil {
		internalError(c, "Не удалось сохранить фото")
		return
	}
	key := filepath.ToSlash(filepath.Join("avatars", "users", strconv.Itoa(userID), name+ext))
	fullPath, err := h.resolveFilePath(key)
	if err != nil {
		internalError(c, "Не удалось сохранить фото")
		return
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		internalError(c, "Не удалось сохранить фото")
		return
	}
	out, err := os.Create(fullPath)
	if err != nil {
		internalError(c, "Не удалось сохранить фото")
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		_ = out.Close()
		internalError(c, "Не удалось сохранить фото")
		return
	}
	if err := out.Close(); err != nil {
		internalError(c, "Не удалось сохранить фото")
		return
	}
	avatarURL := fmt.Sprintf("/users/%d/avatar/content", userID)
	if err := h.service.UpdateAvatar(userID, avatarURL, key, key); err != nil {
		_ = os.Remove(fullPath)
		log.Printf("UploadProfileAvatar: service error: %v", err)
		internalError(c, "Не удалось сохранить фото")
		return
	}
	h.removeStoredFile(current.AvatarPath)
	updated, _ := h.service.GetUserByID(userID)
	c.JSON(http.StatusOK, h.userToResponse(updated))
}

func (h *UserHandler) UpdateProfileAvatarCrop(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	if userID == 0 {
		unauthorized(c, "Unauthorized")
		return
	}
	var req avatarCropRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Некорректные параметры кадрирования")
		return
	}
	if req.CropScale != nil && *req.CropScale <= 0 {
		badRequest(c, "Масштаб должен быть больше нуля")
		return
	}
	if req.CropSize != nil && *req.CropSize <= 0 {
		badRequest(c, "Размер должен быть больше нуля")
		return
	}
	if err := h.service.UpdateAvatarCrop(userID, req.CropX, req.CropY, req.CropScale, req.CropSize); err != nil {
		log.Printf("UpdateProfileAvatarCrop: service error: %v", err)
		internalError(c, "Не удалось сохранить аватар")
		return
	}
	updated, _ := h.service.GetUserByID(userID)
	c.JSON(http.StatusOK, h.userToResponse(updated))
}

func (h *UserHandler) DeleteProfileAvatar(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	if userID == 0 {
		unauthorized(c, "Unauthorized")
		return
	}
	current, err := h.service.GetUserByID(userID)
	if err != nil || current == nil {
		notFound(c, ClientNotFoundCode, "User not found")
		return
	}
	if err := h.service.DeleteAvatar(userID); err != nil {
		log.Printf("DeleteProfileAvatar: service error: %v", err)
		internalError(c, "Не удалось удалить аватар")
		return
	}
	h.removeStoredFile(current.AvatarPath)
	updated, _ := h.service.GetUserByID(userID)
	c.JSON(http.StatusOK, h.userToResponse(updated))
}

func (h *UserHandler) ServeMyAvatar(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	h.serveAvatarForUser(c, userID)
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
	if !authz.CanViewLeadershipData(roleID) && currentUserID != id {
		current, err := h.service.GetUserByID(currentUserID)
		if err != nil || !sameUserBranch(current, user) {
			forbidden(c, "Forbidden")
			return
		}
	}
	c.JSON(http.StatusOK, h.userToResponse(user))
}

func (h *UserHandler) ServeUserAvatar(c *gin.Context) {
	currentUserID, roleID := getUserAndRole(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid user ID")
		return
	}
	if !(authz.CanViewLeadershipData(roleID) || authz.IsReadOnly(roleID)) && currentUserID != id {
		current, currentErr := h.service.GetUserByID(currentUserID)
		target, targetErr := h.service.GetUserByID(id)
		if currentErr != nil || targetErr != nil || !sameUserBranch(current, target) {
			forbidden(c, "Forbidden")
			return
		}
	}
	h.serveAvatarForUser(c, id)
}

func (h *UserHandler) serveAvatarForUser(c *gin.Context, userID int) {
	if userID == 0 {
		unauthorized(c, "Unauthorized")
		return
	}
	user, err := h.service.GetUserByID(userID)
	if err != nil || user == nil || user.AvatarPath == "" {
		notFound(c, NotFoundCode, "Avatar not found")
		return
	}
	fullPath, err := h.resolveFilePath(user.AvatarPath)
	if err != nil {
		notFound(c, NotFoundCode, "Avatar not found")
		return
	}
	f, err := os.Open(fullPath)
	if err != nil {
		notFound(c, NotFoundCode, "Avatar not found")
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		notFound(c, NotFoundCode, "Avatar not found")
		return
	}
	c.Header("Content-Type", avatarContentType(filepath.Ext(user.AvatarPath)))
	c.Header("Content-Disposition", "inline")
	http.ServeContent(c.Writer, c.Request, filepath.Base(user.AvatarPath), stat.ModTime(), f)
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
		notFound(c, ClientNotFoundCode, "Пользователь не найден")
		return
	}
	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Некорректные данные пользователя")
		return
	}
	trimUpdateUserRequest(&req)
	if msg := validateUpdateUserFields(req); msg != "" {
		badRequest(c, msg)
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
		req.CompanyName = nil
		req.Email = nil
		req.Position = nil
		req.RoleID = nil
		req.BranchID = nil
		req.IsVerified = nil
		req.IsActive = nil
	}
	if authz.CanAssignRoles(roleID) && req.RoleID != nil && !authz.IsKnownRole(*req.RoleID) {
		badRequest(c, "Некорректная роль")
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
	if req.Address != nil {
		body.Address = *req.Address
	}
	if req.ExtraInfo != nil {
		body.ExtraInfo = *req.ExtraInfo
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
	if authz.CanAssignRoles(roleID) {
		if msg := h.validateBranchForRole(body.RoleID, body.BranchID); msg != "" {
			badRequest(c, msg)
			return
		}
	}
	if err := h.service.UpdateUser(&body); err != nil {
		log.Printf("UpdateUser: service error: %v", err)
		internalError(c, "Не удалось обновить пользователя")
		return
	}
	updated, _ := h.service.GetUserByID(id)
	c.JSON(http.StatusOK, h.userToResponse(updated))
}

func (h *UserHandler) DeleteUser(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !authz.CanAssignRoles(roleID) {
		forbidden(c, "Только системный администратор может удалять пользователей")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Некорректный ID пользователя")
		return
	}
	if err := h.service.DeleteUser(id); err != nil {
		log.Printf("DeleteUser: service error: %v", err)
		internalError(c, "Не удалось удалить пользователя")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Пользователь удален"})
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
	var current *models.User
	if !authz.CanViewLeadershipData(roleID) {
		current, err = h.service.GetUserByID(c.GetInt("user_id"))
		if err != nil || current == nil || current.BranchID == nil {
			forbidden(c, "Forbidden")
			return
		}
	}
	for _, u := range users {
		if !authz.CanViewLeadershipData(roleID) && u.RoleID == authz.RoleManagement {
			continue
		}
		if !authz.CanViewLeadershipData(roleID) && !sameUserBranch(current, u) {
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
	if !authz.CanViewLeadershipData(roleID) {
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
	if !authz.CanViewLeadershipData(roleID) {
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
	trimCreateUserRequest(&req)
	user := &models.User{
		CompanyName: req.CompanyName,
		BinIin:      req.BinIin,
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		MiddleName:  req.MiddleName,
		Position:    req.Position,
		BranchID:    nil,
		Email:       req.Email,
		Phone:       req.Phone,
		RoleID:      authz.RoleSales,
		IsVerified:  false,
		IsActive:    true,
		IsActiveSet: true,
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
	c.JSON(http.StatusCreated, gin.H{"user": h.userToResponse(user), "message": "Registered. Verification code sent.", "verification_sent": verificationSent})
}

func allowedAvatarExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	default:
		return false
	}
}

func avatarContentType(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

func validateAvatarMime(file io.Reader, ext string) error {
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("Не удалось прочитать файл")
	}
	mimeType := http.DetectContentType(buf[:n])
	expected := avatarContentType(ext)
	if expected == "application/octet-stream" || mimeType != expected {
		if !(ext == ".jpg" && mimeType == "image/jpeg") && !(ext == ".jpeg" && mimeType == "image/jpeg") {
			return fmt.Errorf("Неверный формат файла")
		}
	}
	return nil
}

func randomHex(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (h *UserHandler) resolveFilePath(key string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(key))
	if clean == "." || clean == "" || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") || strings.Contains(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid file key")
	}
	rootAbs, err := filepath.Abs(h.filesRoot)
	if err != nil {
		return "", err
	}
	fullAbs, err := filepath.Abs(filepath.Join(h.filesRoot, clean))
	if err != nil {
		return "", err
	}
	if fullAbs != rootAbs && !strings.HasPrefix(fullAbs, rootAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid file key")
	}
	return fullAbs, nil
}

func (h *UserHandler) removeStoredFile(key string) {
	if strings.TrimSpace(key) == "" {
		return
	}
	fullPath, err := h.resolveFilePath(key)
	if err != nil {
		return
	}
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		log.Printf("removeStoredFile: %v", err)
	}
}

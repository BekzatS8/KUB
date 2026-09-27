package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/authz"
	"turcompany/internal/services"
)

// Переместить, копировать, свойства и отправить — для нескольких элементов
// хранилища сразу.

type driveBatchRequest struct {
	IDs []int64 `json:"ids"`
	// TargetID — папка назначения; null — корень хранилища.
	TargetID *int64 `json:"target_id"`
}

// writeDriveOpsError — ошибки операций, которых нет в writeDriveError.
func writeDriveOpsError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, services.ErrDriveMoveIntoSelf):
		badRequestWithCode(c, "DRIVE_MOVE_INTO_SELF", "Папку нельзя переместить или скопировать в неё саму или в её подпапку")
	case errors.Is(err, services.ErrDriveNothingToDo):
		badRequestWithCode(c, "DRIVE_NOTHING_SELECTED", "Выберите файлы или папки")
	case errors.Is(err, services.ErrDriveSendFolder):
		badRequestWithCode(c, "DRIVE_SEND_FOLDER", "Отправить можно только файлы — откройте папку и выберите файлы в ней")
	case errors.Is(err, services.ErrDriveSendChannel):
		badRequestWithCode(c, "DRIVE_SEND_CHANNEL", "Выберите, куда отправить: WhatsApp, Telegram, Instagram или почта")
	case errors.Is(err, services.ErrDriveSendRecipient):
		badRequestWithCode(c, "DRIVE_SEND_RECIPIENT", "Укажите получателя")
	case errors.Is(err, services.ErrDriveSendEmail):
		badRequestWithCode(c, "DRIVE_SEND_EMAIL", "Некорректный адрес почты")
	case errors.Is(err, services.ErrDriveMailTooLarge):
		writeError(c, http.StatusRequestEntityTooLarge, "DRIVE_MAIL_TOO_LARGE", "Для почты файлы слишком большие (больше 20 МБ вместе) — отправьте их через мессенджер")
	case errors.Is(err, services.ErrDriveSendDisabled):
		writeError(c, http.StatusServiceUnavailable, "DRIVE_SEND_DISABLED", "Этот способ отправки не настроен на сервере")
	case errors.Is(err, services.ErrDriveNoPublicURL):
		writeError(c, http.StatusServiceUnavailable, "DRIVE_NO_PUBLIC_URL", "Не задан публичный адрес API (API_PUBLIC_URL) — мессенджер не сможет скачать файл")
	case errors.Is(err, services.ErrDriveSendFailed):
		writeError(c, http.StatusBadGateway, "DRIVE_SEND_FAILED", "Не удалось отправить: "+errors.Unwrap(err).Error())
	default:
		writeDriveError(c, err, fallback)
	}
}

func (h *DriveHandler) batch(c *gin.Context, op func(context.Context, services.DriveActor, []int64, *int64) (int, error), key, fallback string) {
	var req driveBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Некорректный запрос")
		return
	}
	// Копирование больших папок идёт через хранилище объектов — даём время.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()
	n, err := op(ctx, driveActor(c), req.IDs, req.TargetID)
	if err != nil {
		writeDriveOpsError(c, err, fallback)
		return
	}
	c.JSON(http.StatusOK, gin.H{key: n})
}

// POST /api/v1/drive/move {ids, target_id}
func (h *DriveHandler) Move(c *gin.Context) {
	h.batch(c, h.svc.Move, "moved", "Не удалось переместить")
}

// POST /api/v1/drive/copy {ids, target_id}
func (h *DriveHandler) Copy(c *gin.Context) {
	h.batch(c, h.svc.Copy, "copied", "Не удалось скопировать")
}

// GET /api/v1/drive/nodes/:id/properties
func (h *DriveHandler) Properties(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	props, err := h.svc.Properties(c.Request.Context(), driveActor(c), id)
	if err != nil {
		writeDriveOpsError(c, err, "Не удалось загрузить свойства")
		return
	}
	c.JSON(http.StatusOK, props)
}

type driveSendRequest struct {
	IDs           []int64 `json:"ids"`
	Channel       string  `json:"channel"`
	To            string  `json:"to"`
	ChannelID     string  `json:"channel_id"`
	Text          string  `json:"text"`
	Subject       string  `json:"subject"`
	PublicBaseURL string  `json:"api_base_url"`
}

// POST /api/v1/drive/send — файлы клиенту в WhatsApp/Telegram/Instagram
// (через мессенджер CRM) или на почту. Отправлять можно то, к чему есть
// доступ; мессенджером — только тем, у кого есть мессенджер.
func (h *DriveHandler) Send(c *gin.Context) {
	var req driveSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Некорректный запрос")
		return
	}
	actor := driveActor(c)
	if req.Channel != "email" && !authz.Can(authz.UserContext{UserID: actor.UserID, RoleID: actor.RoleID}, "messenger.view", "messenger") {
		forbidden(c, "Нет доступа к мессенджеру")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	res, err := h.svc.Send(ctx, actor, services.DriveSendRequest{
		IDs:           req.IDs,
		Channel:       req.Channel,
		To:            req.To,
		ChannelID:     req.ChannelID,
		Text:          req.Text,
		Subject:       req.Subject,
		PublicBaseURL: req.PublicBaseURL,
	})
	if err != nil {
		writeDriveOpsError(c, err, "Не удалось отправить")
		return
	}
	c.JSON(http.StatusOK, res)
}

package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"turcompany/internal/authz"
	"turcompany/internal/handlers"
	"turcompany/internal/middleware"
)

func SetupRoutes(
	r *gin.Engine,
	userHandler *handlers.UserHandler,
	clientHandler *handlers.ClientHandler,
	clientFilesHandler *handlers.ClientFilesHandler,
	clientProfileHandler *handlers.ClientProfileHandler,
	roleHandler *handlers.RoleHandler,
	leadHandler *handlers.LeadHandler,
	dealHandler *handlers.DealHandler,
	authHandler *handlers.AuthHandler,
	documentHandler *handlers.DocumentHandler,
	taskHandler *handlers.TaskHandler,
	signHandler *handlers.SignSessionHandler,
	signConfirmHandler *handlers.DocumentSigningConfirmationHandler,
	telegramSignHandler *handlers.TelegramSignWebhookHandler,
	reportHandler *handlers.ReportHandler,
	verifyHandler *handlers.VerifyHandler,
	integrationsHandler *handlers.IntegrationsHandler, // может быть nil
	chatHandler *handlers.ChatHandler,
	publicSignHandler *handlers.PublicDocumentSigningHandler,
	docPublicLinkHandler *handlers.DocumentPublicLinkHandler,
	publicSigningUIHandler *handlers.PublicSigningUIHandler,
	wazzupHandler *handlers.WazzupHandler,
	authMiddleware gin.HandlerFunc,
) *gin.Engine {

	// =====================
	// PUBLIC (no JWT)
	// =====================

	// ✅ публичный healthcheck — всегда 200
	r.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	auth := r.Group("/auth")
	{
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.RefreshToken)
		auth.POST("/forgot-password", authHandler.ForgotPassword)
		auth.POST("/reset-password", authHandler.ResetPassword)
	}

	r.POST("/register", userHandler.Register)
	r.POST("/register/confirm", verifyHandler.ConfirmUser)
	r.POST("/register/resend", verifyHandler.ResendUser)

	if signHandler != nil {
		signPublic := r.Group("/api/v1/sign/sessions")
		{
			signPublic.GET("/id/:id/page", signHandler.ServeSessionPage)
			signPublic.POST("/id/:id/sign", signHandler.SignByID)
			signPublic.POST("/token/:token/verify", signHandler.Verify)
			signPublic.POST("/token/:token/sign", signHandler.Sign)
		}
	}
	if signConfirmHandler != nil {
		if publicSigningUIHandler != nil {
			r.GET("/sign/email/verify", publicSigningUIHandler.ServeEmailVerifyPage)
		} else {
			r.GET("/sign/email/verify", signConfirmHandler.VerifyEmailToken)
		}
		r.GET("/api/v1/sign/email/verify", signConfirmHandler.VerifyEmailToken)
		r.POST("/documents/:id/sign/confirm/email", signConfirmHandler.ConfirmByEmailCode)
	}
	if telegramSignHandler != nil {
		r.POST("/telegram/webhook", telegramSignHandler.Handle)
	}
	if publicSignHandler != nil {
		publicDocs := r.Group("/public/documents")
		{
			publicDocs.GET("/:token", publicSignHandler.GetDocument)
			publicDocs.POST("/:token/sign", publicSignHandler.SignDocument)
		}
	}

	// PUBLIC: Telegram webhook (без JWT!)
	if integrationsHandler != nil {
		r.POST("/integrations/telegram/webhook", integrationsHandler.Webhook)
	}

	if wazzupHandler != nil {
		r.POST("/integrations/wazzup/webhook/:token", wazzupHandler.Webhook)
		r.GET("/integrations/wazzup/crm/:token/users", wazzupHandler.CRMUsers)
		r.GET("/integrations/wazzup/crm/:token/users/:id", wazzupHandler.CRMUserByID)
	}

	// =====================
	// PROTECTED (JWT)
	// =====================
	r.Use(authMiddleware)
	r.Use(middleware.ReadOnlyGuard())

	if signHandler != nil {
		signProtected := r.Group("/api/v1/sign/sessions")
		{
			signProtected.POST("", signHandler.Create)
		}
	}
	if gin.Mode() != gin.ReleaseMode {
		debug := r.Group("/debug", middleware.RequireRoles(authz.RoleSystemAdmin))
		{
			if signConfirmHandler != nil {
				debug.GET("/sign-confirmations/latest", signConfirmHandler.DebugLatest)
			}
			if verifyHandler != nil {
				debug.GET("/register-verification/latest", verifyHandler.DebugLatest)
			}
		}
	}

	// PRIVATE (JWT): Telegram link endpoints
	if integrationsHandler != nil {
		integr := r.Group("/integrations")
		{
			integr.GET("/telegram/link", integrationsHandler.ConfirmLink)
			integr.POST("/telegram/request-link", integrationsHandler.RequestTelegramLink)
		}
	}

	if wazzupHandler != nil {
		wazzup := r.Group("/integrations/wazzup")
		{
			wazzup.POST("/setup", wazzupHandler.Setup)
			wazzup.POST("/iframe", wazzupHandler.Iframe)
			wazzup.POST("/send", wazzupHandler.SendMessage)
		}
	}

	// USERS
	users := r.Group("/users")
	{
		users.POST("", userHandler.CreateUser)
		users.GET("/me", userHandler.GetMyProfile)
		users.GET("/count", userHandler.GetUserCount)
		users.GET("/count/role/:role_id", userHandler.GetUserCountByRole)
		users.GET("", userHandler.ListUsers)
		users.GET("/:id", userHandler.GetUserByID)
		users.PUT("/:id", userHandler.UpdateUser)
		users.DELETE("/:id", userHandler.DeleteUser)
	}

	// CLIENTS
	clients := r.Group("/clients")
	{
		clients.POST("", clientHandler.Create)
		clients.GET("", clientHandler.List)
		clients.GET("/individual", clientHandler.ListIndividuals)
		clients.GET("/company", clientHandler.ListCompanies)
		clients.GET("/my", clientHandler.ListMy)
		clients.PUT("/:id", clientHandler.Update)
		clients.PATCH("/:id", clientHandler.Patch)
		clients.DELETE("/:id", clientHandler.Delete)
		clients.GET("/:id/completeness", clientHandler.GetCompleteness)
		if clientProfileHandler != nil {
			clients.GET("/:id/profile", clientProfileHandler.GetProfile)
		}
		if clientFilesHandler != nil {
			clients.POST("/:id/files", clientFilesHandler.Upload)
			clients.GET("/:id/files/primary", clientFilesHandler.ServePrimaryInline)
			clients.GET("/:id/files/primary/download", clientFilesHandler.ServePrimaryDownload)
		}
		clients.GET("/:id", clientHandler.GetByID)
	}

	// ROLES (System admin)
	roles := r.Group("/roles", middleware.RequireRoles(authz.RoleSystemAdmin))
	{
		roles.POST("", roleHandler.CreateRole)
		roles.GET("/count", roleHandler.GetRoleCount)
		roles.GET("/with-user-counts", roleHandler.GetRolesWithUserCounts)
		roles.GET("", roleHandler.ListRoles)
		roles.GET("/:id", roleHandler.GetRoleByID)
		roles.PUT("/:id", roleHandler.UpdateRole)
		roles.DELETE("/:id", roleHandler.DeleteRole)
	}

	// LEADS
	leads := r.Group("/leads")
	{
		leads.POST("", leadHandler.Create)
		leads.GET("/:id", leadHandler.GetByID)
		leads.PUT("/:id", leadHandler.Update)
		leads.DELETE("/:id", leadHandler.Delete)
		leads.PUT("/:id/convert", leadHandler.ConvertToDeal)
		leads.PUT("/:id/convert-with-client", leadHandler.ConvertToDealWithClient)
		leads.GET("", leadHandler.List)
		leads.GET("/my", leadHandler.ListMy)
		leads.POST("/:id/assign", leadHandler.Assign)
		leads.POST("/:id/status", leadHandler.UpdateStatus)
	}

	// DEALS
	deals := r.Group("/deals")
	{
		deals.POST("", dealHandler.Create)
		deals.GET("/:id", dealHandler.GetByID)
		deals.PUT("/:id", dealHandler.Update)
		deals.DELETE("/:id", dealHandler.Delete)
		deals.GET("", dealHandler.List)
		deals.GET("/my", dealHandler.ListMy)
		deals.POST("/:id/status", dealHandler.UpdateStatus)
	}

	// DOCUMENTS
	docs := r.Group("/documents")
	{
		docs.GET("", documentHandler.ListDocuments)
		docs.GET("/types", documentHandler.ListDocumentTypes)
		docs.POST("", documentHandler.CreateDocument)
		docs.POST("/upload", documentHandler.Upload)
		docs.GET("/:id", documentHandler.GetDocument)
		docs.DELETE("/:id", documentHandler.DeleteDocument)
		docs.POST("/create-from-lead", documentHandler.CreateDocumentFromLead)
		docs.POST("/create-from-client", documentHandler.CreateDocumentFromClient)
		docs.GET("/deal/:dealid", documentHandler.ListDocumentsByDeal)
		docs.GET("/:id/file", documentHandler.ServeFile)
		docs.GET("/:id/download", documentHandler.Download)
		docs.POST("/:id/submit", documentHandler.Submit)
		docs.POST("/:id/review", documentHandler.Review)
		docs.POST("/:id/send-for-signature", documentHandler.SendForSignature)
		docs.POST("/:id/sign", documentHandler.Sign)
		if signConfirmHandler != nil {
			docs.POST("/:id/sign/start", signConfirmHandler.StartSigning)
			docs.GET("/:id/sign/status", signConfirmHandler.Status)
			if docPublicLinkHandler != nil {
				docs.POST("/:id/generate-sign-link", docPublicLinkHandler.GenerateSignLink)
			}
		}
	}

	// CHATS
	chats := r.Group("/chats")
	{
		chats.GET("/users", chatHandler.ListChatDirectoryUsers)
		chats.GET("", chatHandler.ListChats)
		chats.GET("/search", chatHandler.SearchChats)
		chats.GET("/unread", chatHandler.ListUnread)
		chats.GET("/status/:id", chatHandler.GetUserStatus)
		chats.GET("/:id/pins", chatHandler.ListPins)
		chats.GET("/:id/favorites", chatHandler.ListFavorites)

		chats.POST("/personal", chatHandler.CreatePersonalChat)
		chats.POST("/group", chatHandler.CreateGroupChat)

		chats.POST("/:id/add-members", chatHandler.AddMembers)
		chats.POST("/:id/leave", chatHandler.LeaveChat)
		chats.POST("/:id/read", chatHandler.MarkRead)
		chats.POST("/:id/upload", chatHandler.UploadAttachment)
		chats.POST("/:id/attachments", chatHandler.UploadAttachmentAlias)

		chats.DELETE("/:id", chatHandler.DeleteChat)

		chats.GET("/:id/info", chatHandler.GetChatInfo)
		chats.GET("/:id/search", chatHandler.SearchMessages)
		chats.GET("/:id/messages", chatHandler.ListMessages)
		chats.POST("/:id/messages", chatHandler.SendMessage)
		chats.PATCH("/:id/messages/:message_id", chatHandler.EditMessage)
		chats.DELETE("/:id/messages/:message_id", chatHandler.DeleteMessage)
		chats.POST("/:id/messages/:message_id/pin", chatHandler.PinMessage)
		chats.DELETE("/:id/messages/:message_id/pin", chatHandler.UnpinMessage)
		chats.POST("/:id/messages/:message_id/favorite", chatHandler.FavoriteMessage)
		chats.DELETE("/:id/messages/:message_id/favorite", chatHandler.UnfavoriteMessage)

		chats.GET("/:id/ws", chatHandler.Stream)
		r.GET("/attachments/:id/download", chatHandler.DownloadAttachment)
	}

	// TASKS
	tasks := r.Group("/tasks",
		middleware.RequireRoles(
			authz.RoleSales,
			authz.RoleOperations,
			authz.RoleControl,
			authz.RoleManagement,
			authz.RoleAdminStaff,
		),
	)
	{
		tasks.POST("", taskHandler.Create)
		tasks.GET("", taskHandler.GetAll)
		tasks.GET("/:id", taskHandler.GetByID)
		tasks.PUT("/:id", taskHandler.Update)
		tasks.DELETE("/:id", taskHandler.Delete)
		tasks.POST("/:id/status", taskHandler.ChangeStatus)
		tasks.POST("/:id/assign", taskHandler.Assign)
		tasks.POST("/:id/complete", taskHandler.Complete)
		tasks.POST("/:id/remind-later", taskHandler.RemindLater)
	}

	// REPORTS
	reports := r.Group("/reports",
		middleware.RequireRoles(
			authz.RoleSales,
			authz.RoleOperations,
			authz.RoleManagement,
			authz.RoleControl,
			authz.RoleAdminStaff,
		),
	)
	{
		reports.GET("/funnel", reportHandler.GetFunnel)
		reports.GET("/leads", reportHandler.GetLeadsSummary)
		reports.GET("/revenue", reportHandler.GetRevenue)
		reports.GET("/revenue/export", reportHandler.ExportRevenue)
	}

	return r
}

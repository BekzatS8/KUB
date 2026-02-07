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
		r.GET("/sign/:token", signHandler.ServeSignPage)

		signPublic := r.Group("/api/v1/sign/sessions")
		{
			signPublic.POST("/:token/verify", signHandler.Verify)
			signPublic.POST("/:token/sign", signHandler.Sign)
		}
	}
	if signConfirmHandler != nil {
		r.GET("/sign/email/verify", signConfirmHandler.VerifyEmailToken)
	}
	if telegramSignHandler != nil {
		r.POST("/telegram/webhook", telegramSignHandler.Handle)
	}

	// PUBLIC: Telegram webhook (без JWT!)
	if integrationsHandler != nil {
		r.POST("/integrations/telegram/webhook", integrationsHandler.Webhook)
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
		debug := r.Group("/debug")
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
		clients.GET("/my", clientHandler.ListMy)
		clients.PUT("/:id", clientHandler.Update)
		clients.GET("/:id", clientHandler.GetByID)
	}

	// ROLES (Management)
	roles := r.Group("/roles", middleware.RequireRoles(authz.RoleManagement))
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
		docs.POST("/:id/sign", documentHandler.Sign)
		if signConfirmHandler != nil {
			docs.POST("/:id/sign/start", signConfirmHandler.StartSigning)
			docs.POST("/:id/sign/confirm/email", signConfirmHandler.ConfirmByEmailCode)
			docs.GET("/:id/sign/status", signConfirmHandler.Status)
		}
	}

	// CHATS
	chats := r.Group("/chats")
	{
		chats.GET("", chatHandler.ListChats)
		chats.GET("/search", chatHandler.SearchChats)
		chats.GET("/unread", chatHandler.ListUnread)
		chats.GET("/status/:id", chatHandler.GetUserStatus)

		chats.POST("/personal", chatHandler.CreatePersonalChat)
		chats.POST("/group", chatHandler.CreateGroupChat)

		chats.POST("/:id/add-members", chatHandler.AddMembers)
		chats.POST("/:id/leave", chatHandler.LeaveChat)
		chats.POST("/:id/read", chatHandler.MarkRead)
		chats.POST("/:id/upload", chatHandler.UploadAttachment)

		chats.DELETE("/:id", chatHandler.DeleteChat)

		chats.GET("/:id/search", chatHandler.SearchMessages)
		chats.GET("/:id/messages", chatHandler.ListMessages)
		chats.POST("/:id/messages", chatHandler.SendMessage)

		chats.GET("/:id/ws", chatHandler.Stream)
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

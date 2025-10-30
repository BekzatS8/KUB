package routes

import (
	"github.com/gin-gonic/gin"
	"turcompany/internal/authz"
	"turcompany/internal/handlers"
	"turcompany/internal/middleware"
)

func SetupRoutes(
	r *gin.Engine,
	userHandler *handlers.UserHandler,
	roleHandler *handlers.RoleHandler,
	leadHandler *handlers.LeadHandler,
	dealHandler *handlers.DealHandler,
	authHandler *handlers.AuthHandler,
	documentHandler *handlers.DocumentHandler,
	taskHandler *handlers.TaskHandler,
	messageHandler *handlers.MessageHandler,
	smsHandler *handlers.SMSHandler,
	reportHandler *handlers.ReportHandler,
	verifyHandler *handlers.VerifyHandler,
	integrationsHandler *handlers.IntegrationsHandler, // ОДИН Telegram-хендлер, может быть nil
) *gin.Engine {

	// ---- public
	r.POST("/login", authHandler.Login)
	r.POST("/refresh", authHandler.RefreshToken)
	r.POST("/register", userHandler.Register)
	r.POST("/register/confirm", verifyHandler.ConfirmUser)
	r.POST("/register/resend", verifyHandler.ResendUser)

	// Telegram webhook публикуем только если есть интеграция
	if integrationsHandler != nil {
		r.POST("/integrations/telegram/webhook", integrationsHandler.Webhook)
	}

	// ---- protected
	r.Use(middleware.AuthMiddleware())
	r.Use(middleware.ReadOnlyGuard())

	// Integrations (JWT)
	if integrationsHandler != nil {
		integr := r.Group("/integrations")
		{
			integr.POST("/telegram/request-link", integrationsHandler.RequestTelegramLink)
		}
	}

	// USERS
	users := r.Group("/users")
	{
		users.POST("/", userHandler.CreateUser)
		users.GET("/count", userHandler.GetUserCount)
		users.GET("/count/role/:role_id", userHandler.GetUserCountByRole)
		users.GET("/", userHandler.ListUsers)
		users.GET("/:id", userHandler.GetUserByID)
		users.PUT("/:id", userHandler.UpdateUser)
		users.DELETE("/:id", userHandler.DeleteUser)
	}

	// ROLES (Admin)
	roles := r.Group("/roles", middleware.RequireRoles(authz.RoleAdmin))
	{
		roles.POST("/", roleHandler.CreateRole)
		roles.GET("/count", roleHandler.GetRoleCount)
		roles.GET("/with-user-counts", roleHandler.GetRolesWithUserCounts)
		roles.GET("/", roleHandler.ListRoles)
		roles.GET("/:id", roleHandler.GetRoleByID)
		roles.PUT("/:id", roleHandler.UpdateRole)
		roles.DELETE("/:id", roleHandler.DeleteRole)
	}

	// LEADS
	leads := r.Group("/leads")
	{
		leads.POST("/", leadHandler.Create)
		leads.GET("/:id", leadHandler.GetByID)
		leads.PUT("/:id", leadHandler.Update)
		leads.DELETE("/:id", leadHandler.Delete)
		leads.PUT("/:id/convert", leadHandler.ConvertToDeal)
		leads.GET("/", leadHandler.List)
		leads.POST("/:id/assign", leadHandler.Assign)
		leads.POST("/:id/status", leadHandler.UpdateStatus)
	}

	// DEALS
	deals := r.Group("/deals")
	{
		deals.POST("/", dealHandler.Create)
		deals.GET("/:id", dealHandler.GetByID)
		deals.PUT("/:id", dealHandler.Update)
		deals.DELETE("/:id", dealHandler.Delete)
		deals.GET("/", dealHandler.List)
		deals.POST("/:id/status", dealHandler.UpdateStatus)
	}

	// DOCUMENTS
	docs := r.Group("/documents")
	{
		docs.GET("/", documentHandler.ListDocuments)
		docs.POST("/", documentHandler.CreateDocument)
		docs.GET("/:id", documentHandler.GetDocument)
		docs.DELETE("/:id", documentHandler.DeleteDocument)
		docs.POST("/create-from-lead", documentHandler.CreateDocumentFromLead)
		docs.GET("/deal/:dealid", documentHandler.ListDocumentsByDeal)
		docs.GET("/:id/file", documentHandler.ServeFile)
		docs.GET("/:id/download", documentHandler.Download)
		docs.POST("/:id/submit", documentHandler.Submit)
		docs.POST("/:id/review", documentHandler.Review)
		docs.POST("/:id/sign", documentHandler.Sign)
	}

	// TASKS
	tasks := r.Group("/tasks",
		middleware.RequireRoles(authz.RoleSales, authz.RoleOperations, authz.RoleManagement, authz.RoleAdmin),
	)
	{
		tasks.POST("/", taskHandler.Create)
		tasks.GET("/", taskHandler.GetAll)
		tasks.GET("/:id", taskHandler.GetByID)
		tasks.PUT("/:id", taskHandler.Update)
		tasks.DELETE("/:id", taskHandler.Delete)
		tasks.POST("/:id/status", taskHandler.ChangeStatus)
		tasks.POST("/:id/assign", taskHandler.Assign)
	}

	// MESSAGES
	msg := r.Group("/messages",
		middleware.RequireRoles(authz.RoleSales, authz.RoleOperations, authz.RoleManagement, authz.RoleAdmin),
	)
	{
		msg.POST("/", messageHandler.Send)
		msg.GET("/conversations", messageHandler.GetConversations)
		msg.GET("/history/:partner_id", messageHandler.GetConversationHistory)
	}

	// SMS (sales/ops/mgmt/admin)
	sms := r.Group("/sms",
		middleware.RequireRoles(authz.RoleSales, authz.RoleOperations, authz.RoleManagement, authz.RoleAdmin),
	)
	{
		sms.POST("/send", smsHandler.SendSMSHandler)
		sms.POST("/resend", smsHandler.ResendSMSHandler)
		sms.POST("/confirm", smsHandler.ConfirmSMSHandler)
		sms.GET("/latest/:document_id", smsHandler.GetLatestSMSHandler)
		sms.DELETE("/:document_id", smsHandler.DeleteSMSHandler)
	}

	// REPORTS (audit/ops/mgmt/admin)
	reports := r.Group("/reports",
		middleware.RequireRoles(authz.RoleAudit, authz.RoleOperations, authz.RoleManagement, authz.RoleAdmin),
	)
	{
		reports.GET("/summary", reportHandler.GetSummary)
		reports.GET("/leads/filter", reportHandler.FilterLeads)
		reports.GET("/deals/filter", reportHandler.FilterDeals)
	}

	return r
}

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
	verifyHandler *handlers.VerifyHandler, // <= добавь параметр
) *gin.Engine {

	// === ПУБЛИЧНЫЕ ===
	r.POST("/login", authHandler.Login)
	r.POST("/refresh", authHandler.RefreshToken)           // публичный refresh
	r.POST("/register", userHandler.Register)              // регистрация
	r.POST("/register/confirm", verifyHandler.ConfirmUser) // ✅ ОСТАВИТЬ ТОЛЬКО ЗДЕСЬ
	r.POST("/register/resend", verifyHandler.ResendUser)   // ✅ ОСТАВИТЬ ТОЛЬКО ЗДЕСЬ

	// === ВСЁ НИЖЕ — ТОЛЬКО С JWT ===
	r.Use(middleware.AuthMiddleware())
	r.Use(middleware.ReadOnlyGuard()) // аудит — только чтение (режет небезопасные методы)

	// ==== USERS ====
	// читать можно Mgmt/Admin/Audit; создавать/удалять — только Admin (проверка в хендлере)
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

	// ==== ROLES ==== (только Admin)
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

	// ==== LEADS ====
	leads := r.Group("/leads")
	{
		leads.POST("/", leadHandler.Create)
		leads.GET("/:id", leadHandler.GetByID)
		leads.PUT("/:id", leadHandler.Update)
		leads.DELETE("/:id", leadHandler.Delete)
		leads.PUT("/:id/convert", leadHandler.ConvertToDeal)
		leads.GET("/", leadHandler.List)
	}

	// ==== DEALS ====
	deals := r.Group("/deals")
	{
		deals.POST("/", dealHandler.Create)
		deals.GET("/:id", dealHandler.GetByID)
		deals.PUT("/:id", dealHandler.Update)
		deals.DELETE("/:id", dealHandler.Delete)
		deals.GET("/", dealHandler.List)
	}

	// ==== DOCUMENTS ====
	documents := r.Group("/documents")
	{
		documents.GET("/", documentHandler.ListDocuments)
		documents.POST("/", documentHandler.CreateDocument)
		documents.GET("/:id", documentHandler.GetDocument)
		documents.DELETE("/:id", documentHandler.DeleteDocument)

		// спец-сценарий
		documents.POST("/create-from-lead", documentHandler.CreateDocumentFromLead)

		// по сделке
		documents.GET("/deal/:dealid", documentHandler.ListDocumentsByDeal)

		// выдача файлов
		documents.GET("/:id/file", documentHandler.ServeFile)    // inline
		documents.GET("/:id/download", documentHandler.Download) // attachment

		// статусные операции
		documents.POST("/:id/submit", documentHandler.Submit) // Sales -> under_review
		documents.POST("/:id/review", documentHandler.Review) // Ops/Mgmt/Admin -> approve|return
		documents.POST("/:id/sign", documentHandler.Sign)     // Mgmt/Admin -> signed
	}

	// ==== TASKS ==== (staff/ops/mgmt/admin)
	tasks := r.Group("/tasks",
		middleware.RequireRoles(authz.RoleStaff, authz.RoleOperations, authz.RoleManagement, authz.RoleAdmin),
	)
	{
		tasks.POST("/", taskHandler.Create)
		tasks.GET("/", taskHandler.GetAll)
		tasks.GET("/:id", taskHandler.GetByID)
		tasks.PUT("/:id", taskHandler.Update)
		tasks.DELETE("/:id", taskHandler.Delete)
	}

	// ==== MESSAGES ==== (staff/ops/mgmt/admin)
	messages := r.Group("/messages",
		middleware.RequireRoles(authz.RoleStaff, authz.RoleOperations, authz.RoleManagement, authz.RoleAdmin),
	)
	{
		messages.POST("/", messageHandler.Send)
		messages.GET("/conversations", messageHandler.GetConversations)
		messages.GET("/history/:partner_id", messageHandler.GetConversationHistory)
	}

	// ==== SMS ==== (sales/ops/mgmt/admin) — для документов
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

	// ==== REPORTS ==== (audit/ops/mgmt/admin)
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

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
	branchHandler *handlers.BranchHandler,
	clientHandler *handlers.ClientHandler,
	clientFilesHandler *handlers.ClientFilesHandler,
	clientProfileHandler *handlers.ClientProfileHandler,
	clientAvatarHandler *handlers.ClientAvatarHandler,
	clientDocsHandler *handlers.ClientDocumentsHandler,
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
	permissionHandler *handlers.PermissionHandler,
	funnelHandler *handlers.FunnelHandler,
	funnelStageHandler *handlers.FunnelStageHandler,
	funnelTransitionRuleHandler *handlers.FunnelTransitionRuleHandler,
	verifyHandler *handlers.VerifyHandler,
	integrationsHandler *handlers.IntegrationsHandler, // может быть nil
	chatHandler *handlers.ChatHandler,
	publicSignHandler *handlers.PublicDocumentSigningHandler,
	docPublicLinkHandler *handlers.DocumentPublicLinkHandler,
	publicSigningUIHandler *handlers.PublicSigningUIHandler,
	wazzupHandler *handlers.WazzupHandler,
	telephonyHandler *handlers.TelephonyHandler, // может быть nil
	orgHandler *handlers.OrganizationHandler,
	signHistoryHandler *handlers.DocumentSignHistoryHandler,
	docVersionHandler *handlers.DocumentVersionHandler,
	authMiddleware gin.HandlerFunc,
) *gin.Engine {

	// =====================
	// PUBLIC (no JWT)
	// =====================

	// ✅ публичный healthcheck — всегда 200
	r.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.GET("/favicon.ico", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
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
			r.GET("/sign/sms/verify", publicSigningUIHandler.ServeSMSVerifyPage)
		} else {
			r.GET("/sign/email/verify", signConfirmHandler.VerifyEmailToken)
			r.GET("/sign/sms/verify", signConfirmHandler.VerifySMSToken)
		}
		r.GET("/api/v1/sign/email/verify", signConfirmHandler.VerifyEmailToken)
		r.GET("/api/v1/sign/email/preview", signConfirmHandler.PreviewByEmailToken)
		r.GET("/api/v1/sign/sms/verify", signConfirmHandler.VerifySMSToken)
		r.GET("/api/v1/sign/sms/preview", signConfirmHandler.PreviewBySMSToken)
		r.POST("/documents/:id/sign/confirm/email", signConfirmHandler.ConfirmByEmailCode)
		r.POST("/documents/:id/sign/confirm/sms", signConfirmHandler.ConfirmBySMSCode)
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

	// PUBLIC: Binotel webhook (no JWT — Binotel calls this server-to-server)
	if telephonyHandler != nil {
		r.POST("/api/v1/integrations/binotel/webhook", telephonyHandler.BinotelWebhook)
	}

	// PUBLIC: Organization contacts (no JWT — for external websites/landing pages)
	if orgHandler != nil {
		r.GET("/api/v1/public/organization/contacts", orgHandler.GetPublicContacts)
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

	if permissionHandler != nil {
		r.GET("/permissions/me", permissionHandler.GetMe)
		r.GET("/api/v1/permissions/me", permissionHandler.GetMe)
	}

	if funnelHandler != nil {
		registerFunnelsRoutes(r.Group("/funnels"), funnelHandler)
		registerFunnelsRoutes(r.Group("/api/v1/funnels"), funnelHandler)
	}

	if funnelStageHandler != nil {
		registerFunnelStagesRoutes(r.Group("/funnels"), funnelStageHandler)
		registerFunnelStagesRoutes(r.Group("/api/v1/funnels"), funnelStageHandler)
		registerStagesRoutes(r.Group("/stages"), funnelStageHandler)
		registerStagesRoutes(r.Group("/api/v1/stages"), funnelStageHandler)
	}

	if funnelTransitionRuleHandler != nil {
		registerFunnelTransitionRulesRoutes(r.Group("/funnel-transition-rules"), funnelTransitionRuleHandler)
		registerFunnelTransitionRulesRoutes(r.Group("/api/v1/funnel-transition-rules"), funnelTransitionRuleHandler)
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
		// messenger.view guard: hr/legal have no messenger.view → 403
		wazzup := r.Group("/integrations/wazzup", middleware.RequirePermission("messenger.view", "messenger"))
		{
			wazzup.GET("/status", wazzupHandler.Status)
			wazzup.GET("/channels", wazzupHandler.Channels)
			wazzup.GET("/dialogs", wazzupHandler.Dialogs)
			wazzup.GET("/dialogs/:id/messages", wazzupHandler.DialogMessages)
			wazzup.POST("/dialogs/:id/messages", wazzupHandler.SendDialogMessage)
			wazzup.POST("/setup", wazzupHandler.Setup)
			wazzup.POST("/iframe", wazzupHandler.Iframe)
			wazzup.POST("/send", wazzupHandler.SendMessage)
		}
	}

	// TELEPHONY — JWT + telephony.view
	if telephonyHandler != nil {
		tGroup := r.Group("/api/v1/telephony", middleware.RequirePermission("telephony.view", "telephony"))
		{
			tGroup.GET("/calls", telephonyHandler.ListCalls)
			tGroup.GET("/calls/:id", telephonyHandler.GetCall)
		}
		// Per-entity call history (uses existing entity id param)
		clientCalls := r.Group("/api/v1/clients", middleware.RequirePermission("telephony.view", "telephony"))
		{
			clientCalls.GET("/:id/calls", telephonyHandler.ListClientCalls)
		}
		leadCalls := r.Group("/api/v1/leads", middleware.RequirePermission("telephony.view", "telephony"))
		{
			leadCalls.GET("/:id/calls", telephonyHandler.ListLeadCalls)
		}
	}

	// USERS
	profile := r.Group("/profile")
	{
		profile.GET("", userHandler.GetProfile)
		profile.PATCH("", userHandler.UpdateProfile)
		profile.POST("/avatar", userHandler.UploadProfileAvatar)
		profile.PATCH("/avatar/crop", userHandler.UpdateProfileAvatarCrop)
		profile.DELETE("/avatar", userHandler.DeleteProfileAvatar)
		profile.GET("/avatar/content", userHandler.ServeMyAvatar)
	}

	users := r.Group("/users")
	{
		users.POST("", middleware.RequirePermission("users.create", "user"), userHandler.CreateUser)
		users.GET("/me", userHandler.GetMyProfile)
		users.GET("/count", middleware.RequirePermission("users.view", "user"), userHandler.GetUserCount)
		users.GET("/count/role/:role_id", middleware.RequirePermission("users.view", "user"), userHandler.GetUserCountByRole)
		users.GET("", middleware.RequirePermission("users.view", "user"), userHandler.ListUsers)
		users.GET("/:id/avatar/content", userHandler.ServeUserAvatar)
		users.GET("/:id", middleware.RequirePermission("users.view", "user"), userHandler.GetUserByID)
		users.PUT("/:id", middleware.RequirePermission("users.update", "user"), userHandler.UpdateUser)
		users.DELETE("/:id", middleware.RequirePermission("users.delete", "user"), userHandler.DeleteUser)
	}

	// BRANCHES — read gated by branches.view (admin + management only);
	// create/update/delete are admin-only (RequirePermission + handler role check).
	branches := r.Group("/branches")
	{
		branches.GET("", middleware.RequirePermission("branches.view", "branch"), branchHandler.List)
		branches.GET("/:id", middleware.RequirePermission("branches.view", "branch"), branchHandler.GetByID)
		branches.POST("", middleware.RequirePermission("branches.create", "branch"), branchHandler.Create)
		branches.PUT("/:id", middleware.RequirePermission("branches.update", "branch"), branchHandler.Update)
		branches.DELETE("/:id", middleware.RequirePermission("branches.delete", "branch"), branchHandler.Delete)
	}

	// ORGANIZATION (singleton settings)
	if orgHandler != nil {
		r.GET("/api/v1/organization", orgHandler.Get)
		r.PUT("/api/v1/organization", orgHandler.Update)
	}

	// CLIENTS — guarded per action (mirrors /deals). Action gate = RequirePermission;
	// scope is still enforced in ClientService (getClientByIDWithScope + clientMatchesScope).
	// hr has no clients.* → 403 on all; legal has clients.view only → reads pass, writes → 403.
	// NOTE: /:id/profile and /:id/files/* stay service-enforced (ensureClientAccess) — untouched.
	clients := r.Group("/clients")
	{
		clients.POST("", middleware.RequirePermission("clients.create", "client"), clientHandler.Create)
		clients.GET("", middleware.RequirePermission("clients.view", "client"), clientHandler.List)
		clients.GET("/individual", middleware.RequirePermission("clients.view", "client"), clientHandler.ListIndividuals)
		clients.GET("/company", middleware.RequirePermission("clients.view", "client"), clientHandler.ListCompanies)
		clients.GET("/my", middleware.RequirePermission("clients.view", "client"), clientHandler.ListMy)
		clients.PUT("/:id", middleware.RequirePermission("clients.update", "client"), clientHandler.Update)
		clients.PATCH("/:id", middleware.RequirePermission("clients.update", "client"), clientHandler.Patch)
		clients.DELETE("/:id", middleware.RequirePermission("clients.delete", "client"), clientHandler.Delete)
		clients.POST("/:id/archive", middleware.RequirePermission("clients.update", "client"), clientHandler.Archive)
		clients.POST("/:id/unarchive", middleware.RequirePermission("clients.update", "client"), clientHandler.Unarchive)
		clients.GET("/:id/completeness", middleware.RequirePermission("clients.view", "client"), clientHandler.GetCompleteness)
		if clientProfileHandler != nil {
			clients.GET("/:id/profile", clientProfileHandler.GetProfile)
		}
		if clientFilesHandler != nil {
			clients.POST("/:id/files", clientFilesHandler.Upload)
			clients.GET("/:id/files/primary", clientFilesHandler.ServePrimaryInline)
			clients.GET("/:id/files/primary/download", clientFilesHandler.ServePrimaryDownload)
		}
		if clientAvatarHandler != nil {
			clients.POST("/:id/avatar", middleware.RequirePermission("clients.update", "client"), clientAvatarHandler.Upload)
			clients.PATCH("/:id/avatar/crop", middleware.RequirePermission("clients.update", "client"), clientAvatarHandler.UpdateCrop)
			clients.DELETE("/:id/avatar", middleware.RequirePermission("clients.update", "client"), clientAvatarHandler.Delete)
			clients.GET("/:id/avatar/content", clientAvatarHandler.Serve)
		}
		if clientDocsHandler != nil {
			clients.GET("/:id/documents", middleware.RequirePermission("documents.view", "client"), clientDocsHandler.ListDocuments)
			clients.POST("/:id/documents", middleware.RequirePermission("documents.create", "client"), clientDocsHandler.CreateDocument)
		}
		clients.GET("/:id", middleware.RequirePermission("clients.view", "client"), clientHandler.GetByID)
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
		leads.POST("", middleware.RequirePermission("leads.create", "lead"), leadHandler.Create)
		leads.GET("/:id", middleware.RequirePermission("leads.view", "lead"), leadHandler.GetByID)
		leads.PUT("/:id", middleware.RequirePermission("leads.update", "lead"), leadHandler.Update)
		leads.DELETE("/:id", middleware.RequirePermission("leads.delete", "lead"), leadHandler.Delete)
		leads.POST("/:id/archive", middleware.RequirePermission("leads.update", "lead"), leadHandler.Archive)
		leads.POST("/:id/unarchive", middleware.RequirePermission("leads.update", "lead"), leadHandler.Unarchive)
		// convert lead → deal is a deal-creation action: gate on deals.create
		// (visa/partner/qc/hr/legal have no deals.create → 403, same as POST /deals).
		leads.PUT("/:id/convert", middleware.RequirePermission("deals.create", "deal"), leadHandler.ConvertToDeal)
		leads.PUT("/:id/convert-with-client", middleware.RequirePermission("deals.create", "deal"), leadHandler.ConvertToDealWithClient)
		leads.GET("", middleware.RequirePermission("leads.view", "lead"), leadHandler.List)
		leads.GET("/my", middleware.RequirePermission("leads.view", "lead"), leadHandler.ListMy)
		leads.POST("/:id/assign", middleware.RequirePermission("leads.update", "lead"), leadHandler.Assign)
		leads.POST("/:id/status", middleware.RequirePermission("leads.update", "lead"), leadHandler.UpdateStatus)
		if funnelHandler != nil {
			leads.PATCH("/:id/funnel", middleware.RequirePermission(authz.ActionLeadsMoveBetweenFunnels, "lead"), funnelHandler.MoveLeadToFunnel)
		}
	}

	if funnelHandler != nil {
		apiLeads := r.Group("/api/v1/leads")
		{
			apiLeads.PATCH("/:id/funnel", middleware.RequirePermission(authz.ActionLeadsMoveBetweenFunnels, "lead"), funnelHandler.MoveLeadToFunnel)
		}
	}

	// DEALS — guarded per action; visa/partner have no deals.* permissions → 403
	deals := r.Group("/deals")
	{
		deals.POST("", middleware.RequirePermission("deals.create", "deal"), dealHandler.Create)
		deals.GET("/:id", middleware.RequirePermission("deals.view", "deal"), dealHandler.GetByID)
		deals.PUT("/:id", middleware.RequirePermission("deals.update", "deal"), dealHandler.Update)
		deals.DELETE("/:id", middleware.RequirePermission("deals.delete", "deal"), dealHandler.Delete)
		deals.POST("/:id/archive", middleware.RequirePermission("deals.update", "deal"), dealHandler.Archive)
		deals.POST("/:id/unarchive", middleware.RequirePermission("deals.update", "deal"), dealHandler.Unarchive)
		deals.GET("", middleware.RequirePermission("deals.view", "deal"), dealHandler.List)
		deals.GET("/my", middleware.RequirePermission("deals.view", "deal"), dealHandler.ListMy)
		deals.POST("/:id/status", middleware.RequirePermission("deals.update", "deal"), dealHandler.UpdateStatus)
		deals.POST("/:id/move", middleware.RequirePermission("deals.update", "deal"), dealHandler.Move)
		deals.GET("/:id/history", middleware.RequirePermission("deals.view", "deal"), dealHandler.GetHistory)
	}

	// DOCUMENTS — RequirePermission guard per endpoint; public signing routes are above (no JWT)
	docs := r.Group("/documents")
	{
		docs.GET("", middleware.RequirePermission("documents.view", "document"), documentHandler.ListDocuments)
		docs.GET("/types", middleware.RequirePermission("documents.view", "document"), documentHandler.ListDocumentTypes)
		docs.POST("", middleware.RequirePermission("documents.create", "document"), documentHandler.CreateDocument)
		docs.POST("/upload", middleware.RequirePermission("documents.create", "document"), documentHandler.Upload)
		docs.GET("/:id", middleware.RequirePermission("documents.view", "document"), documentHandler.GetDocument)
		docs.DELETE("/:id", middleware.RequirePermission("documents.delete", "document"), documentHandler.DeleteDocument)
		docs.POST("/:id/archive", middleware.RequirePermission("documents.update", "document"), documentHandler.ArchiveDocument)
		docs.POST("/:id/unarchive", middleware.RequirePermission("documents.update", "document"), documentHandler.UnarchiveDocument)
		docs.POST("/create-from-lead", middleware.RequirePermission("documents.create", "document"), documentHandler.CreateDocumentFromLead)
		docs.POST("/create-from-client", middleware.RequirePermission("documents.create", "document"), documentHandler.CreateDocumentFromClient)
		docs.GET("/deal/:dealid", middleware.RequirePermission("documents.view", "document"), documentHandler.ListDocumentsByDeal)
		docs.GET("/:id/file", middleware.RequirePermission("documents.view", "document"), documentHandler.ServeFile)
		docs.GET("/:id/download", middleware.RequirePermission("documents.download", "document"), documentHandler.Download)
		docs.POST("/:id/submit", middleware.RequirePermission("documents.update", "document"), documentHandler.Submit)
		docs.POST("/:id/review", middleware.RequirePermission("documents.update", "document"), documentHandler.Review)
		docs.POST("/:id/send-for-signature", middleware.RequirePermission("documents.send", "document"), documentHandler.SendForSignature)
		docs.POST("/:id/sign", middleware.RequirePermission("documents.update", "document"), documentHandler.Sign)
		if signConfirmHandler != nil {
			docs.POST("/:id/sign/start", middleware.RequirePermission("documents.send", "document"), signConfirmHandler.StartSigning)
			docs.POST("/:id/sign/start/email", middleware.RequirePermission("documents.send", "document"), signConfirmHandler.StartSigningEmail)
			docs.POST("/:id/sign/start/sms", middleware.RequirePermission("documents.send", "document"), signConfirmHandler.StartSigningSMS)
			docs.GET("/:id/sign/contact-options", middleware.RequirePermission("documents.view", "document"), signConfirmHandler.ContactOptions)
			docs.GET("/:id/sign/status", middleware.RequirePermission("documents.view", "document"), signConfirmHandler.Status)
			if docPublicLinkHandler != nil {
				docs.POST("/:id/generate-sign-link", middleware.RequirePermission("documents.send", "document"), docPublicLinkHandler.GenerateSignLink)
			}
		}
		if signHistoryHandler != nil {
			docs.GET("/:id/sign/history", middleware.RequirePermission("documents.view", "document"), signHistoryHandler.GetSignHistory)
		}
		if docVersionHandler != nil {
			docs.GET("/:id/versions", middleware.RequirePermission("documents.view", "document"), docVersionHandler.ListVersions)
			docs.POST("/:id/versions", middleware.RequirePermission("documents.update", "document"), docVersionHandler.UploadVersion)
			docs.GET("/:id/versions/:vid/file", middleware.RequirePermission("documents.view", "document"), docVersionHandler.ServeVersionFile)
			docs.POST("/:id/versions/:vid/restore", middleware.RequirePermission("documents.update", "document"), docVersionHandler.RestoreVersion)
		}
	}

	// CHATS — gated by chat.view; all roles currently have it, but the guard
	// blocks future roles without it and provides explicit 403 over silent 200.
	chats := r.Group("/chats", middleware.RequirePermission("chat.view", "chat"))
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

		chats.DELETE("/:id", middleware.RequirePermission("chat.delete", "chat"), chatHandler.DeleteChat)

		chats.GET("/:id/info", chatHandler.GetChatInfo)
		chats.GET("/:id/search", chatHandler.SearchMessages)
		chats.GET("/:id/messages", chatHandler.ListMessages)
		chats.POST("/:id/messages", chatHandler.SendMessage)
		chats.PATCH("/:id/messages/:message_id", chatHandler.EditMessage)
		chats.DELETE("/:id/messages/:message_id", middleware.RequirePermission("chat.delete", "chat"), chatHandler.DeleteMessage)
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
			authz.RoleControl,
			authz.RoleManagement,
			authz.RoleSystemAdmin,
			authz.RoleVisa,
			authz.RolePartner,
			authz.RoleHR,
			authz.RoleLegal,
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
		tasks.POST("/:id/archive", taskHandler.Archive)
		tasks.POST("/:id/unarchive", taskHandler.Unarchive)
	}

	// REPORTS — requires reports.view (admin, management, quality_control per permission matrix)
	reports := r.Group("/reports", middleware.RequirePermission("reports.view", "reports"))
	{
		reports.GET("/funnel", reportHandler.GetFunnel)
		reports.GET("/leads", reportHandler.GetLeadsSummary)
		reports.GET("/revenue", reportHandler.GetRevenue)
		reports.GET("/revenue/export", reportHandler.ExportRevenue)
	}

	return r
}

func registerFunnelsRoutes(group *gin.RouterGroup, funnelHandler *handlers.FunnelHandler) {
	group.GET("", middleware.RequirePermission(authz.ActionFunnelsView, "funnel"), funnelHandler.List)
	group.GET("/:id", middleware.RequirePermission(authz.ActionFunnelsView, "funnel"), funnelHandler.GetByID)
	group.POST("", middleware.RequirePermission(authz.ActionFunnelsCreate, "funnel"), funnelHandler.Create)
	group.PATCH("/reorder", middleware.RequirePermission(authz.ActionFunnelsReorder, "funnel"), funnelHandler.Reorder)
	group.PATCH("/:id", middleware.RequirePermission(authz.ActionFunnelsUpdate, "funnel"), funnelHandler.Update)
	group.DELETE("/:id", middleware.RequirePermission(authz.ActionFunnelsDelete, "funnel"), funnelHandler.Delete)
}

// registerFunnelStagesRoutes registers /:id/stages, /:id/stages/reorder and
// /:id/board under a funnels group (kanban board + stage list/create/reorder).
func registerFunnelStagesRoutes(group *gin.RouterGroup, h *handlers.FunnelStageHandler) {
	group.GET("/:id/stages", middleware.RequirePermission(authz.ActionFunnelsView, "funnel"), h.ListStages)
	group.POST("/:id/stages", middleware.RequirePermission(authz.ActionFunnelsUpdate, "funnel"), h.CreateStage)
	group.PATCH("/:id/stages/reorder", middleware.RequirePermission(authz.ActionFunnelsReorder, "funnel"), h.ReorderStages)
	group.GET("/:id/board", middleware.RequirePermission(authz.ActionFunnelsView, "funnel"), h.Board)
}

// registerStagesRoutes registers top-level /stages/:id endpoints for updating,
// deleting and duplicating individual funnel stages.
func registerStagesRoutes(group *gin.RouterGroup, h *handlers.FunnelStageHandler) {
	group.PATCH("/:id", middleware.RequirePermission(authz.ActionFunnelsUpdate, "funnel"), h.UpdateStage)
	group.DELETE("/:id", middleware.RequirePermission(authz.ActionFunnelsDelete, "funnel"), h.DeleteStage)
	group.POST("/:id/duplicate", middleware.RequirePermission(authz.ActionFunnelsCreate, "funnel"), h.DuplicateStage)
}

// registerFunnelTransitionRulesRoutes registers CRUD endpoints for admin-configured
// automatic cross-funnel transition rules.
func registerFunnelTransitionRulesRoutes(group *gin.RouterGroup, h *handlers.FunnelTransitionRuleHandler) {
	group.GET("", middleware.RequirePermission(authz.ActionFunnelsView, "funnel"), h.List)
	group.GET("/:id", middleware.RequirePermission(authz.ActionFunnelsView, "funnel"), h.Get)
	group.POST("", middleware.RequirePermission(authz.ActionFunnelsCreate, "funnel"), h.Create)
	group.PUT("/:id", middleware.RequirePermission(authz.ActionFunnelsUpdate, "funnel"), h.Update)
	group.DELETE("/:id", middleware.RequirePermission(authz.ActionFunnelsDelete, "funnel"), h.Delete)
	group.PATCH("/:id/toggle", middleware.RequirePermission(authz.ActionFunnelsUpdate, "funnel"), h.ToggleActive)
}

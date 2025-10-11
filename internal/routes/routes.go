package routes

import (
	"github.com/gin-gonic/gin"
	"turcompany/internal/handlers"
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
) *gin.Engine {

	// Аутентификация
	r.POST("/login", authHandler.Login)

	// Публичная регистрация пользователя
	r.POST("/register", userHandler.Register)

	// Маршруты для пользователей
	users := r.Group("/users")
	{
		users.POST("/", userHandler.CreateUser)                           // Создание пользователя
		users.GET("/count", userHandler.GetUserCount)                     // Количество пользователей
		users.GET("/count/role/:role_id", userHandler.GetUserCountByRole) // Количество пользователей по роли
		users.GET("/", userHandler.ListUsers)                             // Список всех пользователей
		users.GET("/:id", userHandler.GetUserByID)                        // Получение пользователя по ID
		users.PUT("/:id", userHandler.UpdateUser)                         // Обновление пользователя
		users.DELETE("/:id", userHandler.DeleteUser)                      // Удаление пользователя
	}

	// Маршруты для ролей
	roles := r.Group("/roles")
	{
		roles.POST("/", roleHandler.CreateRole)                            // Создание роли
		roles.GET("/count", roleHandler.GetRoleCount)                      // Количество ролей
		roles.GET("/with-user-counts", roleHandler.GetRolesWithUserCounts) // Роли с количеством пользователей
		roles.GET("/", roleHandler.ListRoles)                              // Список всех ролей
		roles.GET("/:id", roleHandler.GetRoleByID)                         // Получение роли по ID
		roles.PUT("/:id", roleHandler.UpdateRole)                          // Обновление роли
		roles.DELETE("/:id", roleHandler.DeleteRole)                       // Удаление роли
	}

	// Маршруты для лидов
	leads := r.Group("/leads")
	{
		leads.POST("/", leadHandler.Create)                  // Создание лида
		leads.GET("/:id", leadHandler.GetByID)               // Получение лида по ID
		leads.PUT("/:id", leadHandler.Update)                // Обновление лида
		leads.DELETE("/:id", leadHandler.Delete)             // Удаление лида
		leads.PUT("/:id/convert", leadHandler.ConvertToDeal) // Конвертация в сделку
		leads.GET("/", leadHandler.List)
	}

	// Маршруты для сделок
	deals := r.Group("/deals")
	{
		deals.POST("/", dealHandler.Create)      // Создание сделки
		deals.GET("/:id", dealHandler.GetByID)   // Получение сделки по ID
		deals.PUT("/:id", dealHandler.Update)    // Обновление сделки
		deals.DELETE("/:id", dealHandler.Delete) // Удаление сделки
		deals.GET("/", dealHandler.List)
	}

	// Маршруты для документов
	documents := r.Group("/documents")
	{
		documents.GET("/", documentHandler.ListDocuments)
		documents.POST("/", documentHandler.CreateDocument)
		documents.GET("/:id", documentHandler.GetDocument)
		documents.DELETE("/:id", documentHandler.DeleteDocument)
		documents.POST("/create-from-lead", documentHandler.CreateDocumentFromLead)
		documents.GET("/deal/:dealid", documentHandler.ListDocumentsByDeal)
	}

	// Маршруты для задач
	tasks := r.Group("/tasks")
	{
		tasks.POST("/", taskHandler.Create)      // Создание задачи
		tasks.GET("/", taskHandler.GetAll)       // Получение всех задач
		tasks.GET("/:id", taskHandler.GetByID)   // Получение задачи по ID
		tasks.PUT("/:id", taskHandler.Update)    // Обновление задачи
		tasks.DELETE("/:id", taskHandler.Delete) // Удаление задачи
	}

	// Маршруты для сообщений
	messages := r.Group("/messages")
	{
		messages.POST("/", messageHandler.Send)                                     // Отправка сообщения
		messages.GET("/conversations", messageHandler.GetConversations)             // Список бесед
		messages.GET("/history/:partner_id", messageHandler.GetConversationHistory) // История беседы
	}

	// Маршруты для SMS
	sms := r.Group("/sms")
	{
		sms.POST("/send", smsHandler.SendSMSHandler)                    // Отправка SMS
		sms.POST("/resend", smsHandler.ResendSMSHandler)                // Повторная отправка SMS
		sms.POST("/confirm", smsHandler.ConfirmSMSHandler)              // Подтверждение SMS
		sms.GET("/latest/:document_id", smsHandler.GetLatestSMSHandler) // Последняя SMS для документа
		sms.DELETE("/:document_id", smsHandler.DeleteSMSHandler)        // Удаление SMS
	}

	// Маршруты для отчетов
	reports := r.Group("/reports")
	reports.GET("/summary", reportHandler.GetSummary)
	reports.GET("/leads/filter", reportHandler.FilterLeads)
	reports.GET("/deals/filter", reportHandler.FilterDeals)

	return r
}

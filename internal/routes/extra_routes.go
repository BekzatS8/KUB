package routes

import (
	"github.com/gin-gonic/gin"

	"turcompany/internal/handlers"
)

// ExtraHandlers — разделы ТЗ «доработки CRM КУБ» (ноябрь 2026), подключаемые
// отдельно от SetupRoutes, чтобы не раздувать его длинную сигнатуру. Любой из
// обработчиков может быть nil.
type ExtraHandlers struct {
	Calendar  *handlers.CalendarHandler
	Employees *handlers.EmployeeHandler
}

// RegisterExtraRoutes регистрирует разделы, требующие входа. Вызывать СТРОГО
// после SetupRoutes: там на движок вешаются authMiddleware и ReadOnlyGuard, и
// только маршруты, добавленные после этого, их получают.
func RegisterExtraRoutes(r *gin.Engine, h ExtraHandlers) {
	if h.Calendar != nil {
		cal := r.Group("/calendar")
		{
			cal.GET("/events", h.Calendar.List)
			cal.POST("/events", h.Calendar.Create)
			cal.PUT("/events/:id", h.Calendar.Update)
			cal.DELETE("/events/:id", h.Calendar.Delete)
		}
	}
	// Сотрудники и кадровые документы (п.1–2). Доступ — админ, руководство,
	// отдел кадров; шаблоны загружают админ и кадры (проверяет сервис).
	if h.Employees != nil {
		emp := r.Group("/employees")
		{
			emp.GET("", h.Employees.List)
			emp.POST("", h.Employees.Create)
			emp.GET("/positions", h.Employees.Positions)
			emp.GET("/templates", h.Employees.ListTemplates)
			emp.POST("/templates", h.Employees.UploadTemplate)
			emp.GET("/templates/:id/file", h.Employees.TemplateFile)
			emp.DELETE("/templates/:id", h.Employees.DeleteTemplate)
			emp.GET("/:id", h.Employees.Get)
			emp.PUT("/:id", h.Employees.Update)
			emp.POST("/:id/photo", h.Employees.UploadPhoto)
			emp.GET("/:id/photo", h.Employees.Photo)
			emp.GET("/:id/documents", h.Employees.ListDocuments)
			emp.POST("/:id/documents", h.Employees.IssueDocuments)
			emp.POST("/:id/documents/:docId/send", h.Employees.SendDocument)
			emp.POST("/:id/documents/:docId/revoke", h.Employees.RevokeDocument)
			emp.GET("/:id/documents/:docId/file", h.Employees.DocumentFile)
		}
	}
}

package routes

import (
	"github.com/gin-gonic/gin"

	"turcompany/internal/handlers"
)

// ExtraHandlers — разделы ТЗ «доработки CRM КУБ» (ноябрь 2026), подключаемые
// отдельно от SetupRoutes, чтобы не раздувать его длинную сигнатуру. Любой из
// обработчиков может быть nil.
type ExtraHandlers struct {
	Calendar *handlers.CalendarHandler
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
}

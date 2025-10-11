package app

import (
	"database/sql"
	"fmt"
	"log"
	"turcompany/internal/config"
	"turcompany/internal/handlers"
	"turcompany/internal/repositories"
	"turcompany/internal/routes"
	"turcompany/internal/services"
	"turcompany/internal/utils"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq" // Подключение базы данных PostgreSQL

	swaggerFiles "github.com/swaggo/files" // Импорт файлов для Swagger с alias
	"github.com/swaggo/gin-swagger"        // Swagger middleware
	_ "turcompany/docs"                    // Сгенерированная документация Swagger
)

func Run() {
	cfg := config.LoadConfig()

	// Настройка подключения к базе данных
	db, err := sql.Open("postgres", cfg.Database.DSN)
	if err != nil {
		log.Fatal("Ошибка подключения к базе данных: ", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Ошибка закрытия базы данных: %v", err)
		}
	}()

	// Репозитории
	roleRepo := repositories.NewRoleRepository(db)
	userRepo := repositories.NewUserRepository(db)
	leadRepo := repositories.NewLeadRepository(db)
	dealRepo := repositories.NewDealRepository(db)
	documentRepo := repositories.NewDocumentRepository(db)
	taskRepo := repositories.NewTaskRepository(db)
	messageRepo := repositories.NewMessageRepository(db)
	smsRepo := repositories.NewSMSConfirmationRepository(db)

	// Сервисы
	authService := services.NewAuthService()
	emailService := services.NewEmailService(
		cfg.Email.SMTPHost,
		cfg.Email.SMTPPort,
		cfg.Email.SMTPUser,
		cfg.Email.SMTPPassword,
		cfg.Email.FromEmail,
	)
	roleService := services.NewRoleService(roleRepo)
	userService := services.NewUserService(userRepo, emailService, authService)
	leadService := services.NewLeadService(leadRepo, dealRepo)
	dealService := services.NewDealService(dealRepo)
	documentService := services.NewDocumentService(documentRepo, leadRepo, dealRepo, smsRepo, "placeholder-secret")
	taskService := services.NewTaskService(taskRepo)
	messageService := services.NewMessageService(messageRepo)
	mobizonClient := utils.NewClient("kzfaad0a91a4b498db593b78414dfdaa2c213b8b8996afa325a223543481efeb11dd11")
	smsService := services.NewSMSService(smsRepo, mobizonClient)

	// Новый сервис для отчётов
	reportService := services.NewReportService(leadRepo, dealRepo)

	// Обработчики
	authHandler := handlers.NewAuthHandler(userService, authService)
	roleHandler := handlers.NewRoleHandler(roleService)
	userHandler := handlers.NewUserHandler(userService, authService)
	leadHandler := handlers.NewLeadHandler(leadService)
	dealHandler := handlers.NewDealHandler(dealService)
	documentHandler := handlers.NewDocumentHandler(documentService)
	taskHandler := handlers.NewTaskHandler(taskService)
	messageHandler := handlers.NewMessageHandler(messageService)
	smsHandler := handlers.NewSMSHandler(smsService)

	// Новый обработчик для отчётов
	reportHandler := handlers.NewReportHandler(reportService)

	// Настройка маршрутов и middleware
	router := gin.Default()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	routes.SetupRoutes(
		router,
		userHandler,
		roleHandler,
		leadHandler,
		dealHandler,
		authHandler,
		documentHandler,
		taskHandler,
		messageHandler,
		smsHandler,
		reportHandler, // Передаём reportHandler здесь
	)

	// Swagger UI
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Запуск сервера
	listenAddr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("Сервер запущен на %s", listenAddr)
	if err := router.Run(listenAddr); err != nil {
		log.Fatal("Ошибка запуска сервера: ", err)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

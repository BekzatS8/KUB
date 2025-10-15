package app

import (
	"database/sql"
	"fmt"
	"log"

	"turcompany/internal/config"
	"turcompany/internal/handlers"
	"turcompany/internal/pdf"
	"turcompany/internal/repositories"
	"turcompany/internal/routes"
	"turcompany/internal/services"
	"turcompany/internal/utils"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	swaggerFiles "github.com/swaggo/files"
	"github.com/swaggo/gin-swagger"
	_ "turcompany/docs"
)

func Run() {
	cfg := config.LoadConfig()

	// === DB ===
	db, err := sql.Open("postgres", cfg.Database.DSN)
	if err != nil {
		log.Fatal("Ошибка подключения к БД: ", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Ошибка закрытия БД: %v", err)
		}
	}()

	// === Repos ===
	roleRepo := repositories.NewRoleRepository(db)
	userRepo := repositories.NewUserRepository(db)
	leadRepo := repositories.NewLeadRepository(db)
	dealRepo := repositories.NewDealRepository(db)
	documentRepo := repositories.NewDocumentRepository(db)
	taskRepo := repositories.NewTaskRepository(db)
	messageRepo := repositories.NewMessageRepository(db)
	smsRepo := repositories.NewSMSConfirmationRepository(db)

	// === Services ===
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

	// PDF генератор (укажи реальный путь к TTF с кириллицей)
	// например, положи DejaVuSans.ttf в assets/fonts/DejaVuSans.ttf
	pdfGen := pdf.NewDocumentGenerator(cfg.Files.RootDir, "assets/fonts/DejaVuSans.ttf")

	// DocumentService с filesRoot и pdfGen
	documentService := services.NewDocumentService(
		documentRepo,
		leadRepo,
		dealRepo,
		smsRepo,
		"placeholder-secret",
		cfg.Files.RootDir,
		pdfGen,
	)

	taskService := services.NewTaskService(taskRepo)
	messageService := services.NewMessageService(messageRepo)

	// SMS провайдер (Mobizon) из конфига
	mobizonClient := utils.NewClientWithOptions(
		cfg.Mobizon.APIKey,
		cfg.Mobizon.SenderID,
		cfg.Mobizon.DryRun,
	)
	// SMS сервис с доступом к DocumentService (чтобы подписывать после Confirm)
	smsService := services.NewSMSService(smsRepo, mobizonClient, documentService)

	// Reports
	reportService := services.NewReportService(leadRepo, dealRepo)

	// === Handlers ===
	authHandler := handlers.NewAuthHandler(userService, authService)
	roleHandler := handlers.NewRoleHandler(roleService)
	userHandler := handlers.NewUserHandler(userService, authService)
	leadHandler := handlers.NewLeadHandler(leadService)
	dealHandler := handlers.NewDealHandler(dealService)
	documentHandler := handlers.NewDocumentHandler(documentService)
	taskHandler := handlers.NewTaskHandler(taskService)
	messageHandler := handlers.NewMessageHandler(messageService)
	smsHandler := handlers.NewSMSHandler(smsService)
	reportHandler := handlers.NewReportHandler(reportService)

	// === Gin ===
	router := gin.Default()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	// Swagger
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Роуты (JWT/RBAC — внутри SetupRoutes)
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
		reportHandler,
	)

	// === Run ===
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

package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"turcompany/internal/config"
	"turcompany/internal/handlers"
	"turcompany/internal/pdf"
	"turcompany/internal/repositories"
	"turcompany/internal/routes"
	"turcompany/internal/services"
	"turcompany/internal/utils"
)

func Run() {
	cfg := config.LoadConfig()
	log.Printf("[BOOT] starting backend...")
	log.Printf("[BOOT] config: server.port=%d, telegram.enable=%v", cfg.Server.Port, cfg.Telegram.Enable)
	if cfg.Telegram.WebhookURL != "" {
		log.Printf("[BOOT] config: telegram.webhook_url=%s", cfg.Telegram.WebhookURL)
	} else {
		log.Printf("[BOOT] config: telegram.webhook_url is empty")
	}
	log.Printf("[BOOT] config: db.dsn=%s", cfg.Database.DSN)

	// === DB ===
	db, err := sql.Open("postgres", cfg.Database.DSN)
	if err != nil {
		log.Fatal("[BOOT] Ошибка подключения к БД: ", err)
	}
	// Параметры пула подключений (по желанию)
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	// Быстрая проверка соединения
	{
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			log.Fatal("[BOOT] БД недоступна: ", err)
		}
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("[BOOT] Ошибка закрытия БД: %v", err)
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
	smsRepo := repositories.NewSMSConfirmationRepository(db)    // для документов
	verifRepo := repositories.NewUserVerificationRepository(db) // для верификации пользователей
	teleLinkRepo := repositories.NewTelegramLinkRepository(db)  // для привязки Telegram

	// === Services (общие) ===
	authService := services.NewAuthService()
	emailService := services.NewEmailService(
		cfg.Email.SMTPHost,
		cfg.Email.SMTPPort,
		cfg.Email.SMTPUser,
		cfg.Email.SMTPPassword,
		cfg.Email.FromEmail,
	)

	// Подготовим переменные под Telegram и интеграционный хендлер
	var (
		tgSvc               *services.TelegramService
		integrationsHandler *handlers.IntegrationsHandler
	)

	// Telegram (если включен)
	if cfg.Telegram.Enable && cfg.Telegram.BotToken != "" {
		log.Printf("[BOOT] Telegram enabled: true (token len=%d)", len(cfg.Telegram.BotToken))
		tgSvc = services.NewTelegramService(cfg.Telegram.BotToken)

		if cfg.Telegram.WebhookURL != "" {
			log.Printf("[BOOT] setting Telegram webhook -> %s", cfg.Telegram.WebhookURL)
			if err := tgSvc.SetWebhook(cfg.Telegram.WebhookURL); err != nil {
				log.Printf("[BOOT] Telegram setWebhook error: %v", err)
			} else {
				log.Printf("[BOOT] Telegram setWebhook OK")
			}
		} else {
			log.Printf("[BOOT] Telegram webhook URL is empty — webhook will NOT be set")
		}
	} else {
		log.Printf("[BOOT] Telegram disabled or token is empty — integrations handler will be nil")
	}

	roleService := services.NewRoleService(roleRepo)
	userService := services.NewUserService(userRepo, emailService, authService)
	leadService := services.NewLeadService(leadRepo, dealRepo)
	dealService := services.NewDealService(dealRepo)

	// PDF генератор (для документов)
	pdfGen := pdf.NewDocumentGenerator(cfg.Files.RootDir, "assets/fonts/DejaVuSans.ttf")

	documentService := services.NewDocumentService(
		documentRepo,
		leadRepo,
		dealRepo,
		smsRepo,
		"placeholder-secret",
		cfg.Files.RootDir,
		pdfGen,
	)

	// --- ВАЖНО: создаём TaskService ДО сборки хендлеров, т.к. он нужен и TaskHandler, и IntegrationsHandler
	taskService := services.NewTaskService(taskRepo)
	messageService := services.NewMessageService(messageRepo)

	// SMS провайдер (Mobizon)
	mobizonClient := utils.NewClientWithOptions(
		cfg.Mobizon.APIKey,
		cfg.Mobizon.SenderID,
		cfg.Mobizon.DryRun,
	)
	log.Printf("[BOOT] Mobizon: dry_run=%v sender_id=%q", cfg.Mobizon.DryRun, cfg.Mobizon.SenderID)

	// Сервис SMS — для документов + для верификации пользователей
	smsService := services.NewSMSService(
		smsRepo,       // репозиторий подтверждений по документам
		mobizonClient, // провайдер
		documentService,
		verifRepo,   // репозиторий верификации пользователей
		userService, // чтобы отмечать is_verified
	)

	// Reports
	reportService := services.NewReportService(leadRepo, dealRepo)

	// === Handlers ===
	authHandler := handlers.NewAuthHandler(userService, authService)
	roleHandler := handlers.NewRoleHandler(roleService)
	userHandler := handlers.NewUserHandler(userService, smsService)
	leadHandler := handlers.NewLeadHandler(leadService)
	dealHandler := handlers.NewDealHandler(dealService)
	documentHandler := handlers.NewDocumentHandler(documentService)

	// ✔ TaskHandler теперь получает TelegramService и UserRepository для уведомлений
	taskHandler := handlers.NewTaskHandler(taskService, tgSvc, userRepo)

	messageHandler := handlers.NewMessageHandler(messageService)
	smsHandler := handlers.NewSMSHandler(smsService)
	verifyHandler := handlers.NewVerifyHandler(smsService)
	reportHandler := handlers.NewReportHandler(reportService)

	// ✔ IntegrationsHandler должен создаваться ПОСЛЕ taskService, и получает его в конструктор
	if tgSvc != nil {
		integrationsHandler = handlers.NewIntegrationsHandler(tgSvc, teleLinkRepo, userRepo, taskService)
	}

	// === Gin ===
	// Для продакшена можно включить gin.ReleaseMode()
	// gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), corsMiddleware())

	// === Routes ===
	log.Printf("[BOOT] mounting routes...")
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
		verifyHandler,
		integrationsHandler, // опционально, может быть nil
	)
	log.Printf("[BOOT] routes mounted. Starting server...")

	// === Run ===
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("[BOOT] HTTP listen on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatal("[BOOT] Ошибка запуска сервера: ", err)
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

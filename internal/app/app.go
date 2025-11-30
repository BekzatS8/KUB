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
	"turcompany/internal/realtime"
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
	clientRepo := repositories.NewClientRepository(db)
	documentRepo := repositories.NewDocumentRepository(db)
	taskRepo := repositories.NewTaskRepository(db)
	smsRepo := repositories.NewSMSConfirmationRepository(db)    // для документов
	verifRepo := repositories.NewUserVerificationRepository(db) // для верификации пользователей
	teleLinkRepo := repositories.NewTelegramLinkRepository(db)  // для привязки Telegram
	chatRepo := repositories.NewChatRepository(db)
	passwordResetRepo := repositories.NewPasswordResetRepository(db)
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
	clientService := services.NewClientService(clientRepo)
	leadService := services.NewLeadService(leadRepo, dealRepo, clientRepo)
	dealService := services.NewDealService(dealRepo)
	chatService := services.NewChatService(chatRepo)
	passwordResetService := services.NewPasswordResetService(userRepo, passwordResetRepo, emailService, authService)

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
	chatHub := realtime.NewChatHub()

	// === Handlers ===
	authHandler := handlers.NewAuthHandler(userService, authService, passwordResetService)
	roleHandler := handlers.NewRoleHandler(roleService)
	userHandler := handlers.NewUserHandler(userService, smsService)
	clientHandler := handlers.NewClientHandler(clientService)
	leadHandler := handlers.NewLeadHandler(leadService)
	dealHandler := handlers.NewDealHandler(dealService)
	documentHandler := handlers.NewDocumentHandler(documentService)
	chatHandler := handlers.NewChatHandler(chatService, chatHub)

	// ✔ TaskHandler теперь получает TelegramService и UserRepository для уведомлений
	taskHandler := handlers.NewTaskHandler(taskService, tgSvc, userRepo)

	smsHandler := handlers.NewSMSHandler(smsService)
	verifyHandler := handlers.NewVerifyHandler(smsService)
	reportHandler := handlers.NewReportHandler(reportService)

	// === Загружаем локаль (тайм-зону) и прокидываем в интеграции ===
	var loc *time.Location
	if tz := cfg.Server.TZ; tz != "" {
		if l, err := time.LoadLocation(tz); err != nil {
			log.Printf("[BOOT] invalid server.TZ=%q: %v — fallback to local", tz, err)
			loc = time.Local
		} else {
			loc = l
		}
	} else {
		loc = time.Local
	}
	log.Printf("[BOOT] server timezone set to: %s", loc.String())

	// ✔ IntegrationsHandler должен создаваться ПОСЛЕ taskService, и получает его в конструктор
	if tgSvc != nil {
		integrationsHandler = handlers.NewIntegrationsHandler(tgSvc, teleLinkRepo, userRepo, taskService)
		// ← прокидываем локаль
		integrationsHandler.SetLocation(loc)
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
		clientHandler,
		roleHandler,
		leadHandler,
		dealHandler,
		authHandler,
		documentHandler,
		taskHandler,
		smsHandler,
		reportHandler,
		verifyHandler,
		integrationsHandler,
		chatHandler,
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

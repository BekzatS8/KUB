package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"turcompany/internal/docx"
	"turcompany/internal/xlsx"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"turcompany/internal/audit"
	"turcompany/internal/config"
	"turcompany/internal/handlers"
	"turcompany/internal/middleware"
	"turcompany/internal/pdf"
	"turcompany/internal/realtime"
	"turcompany/internal/repositories"
	"turcompany/internal/routes"
	"turcompany/internal/services"
	"turcompany/internal/utils"
)

func Run() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("[BOOT] failed to load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("[BOOT] invalid config: %v", err)
	}
	log.Printf("[BOOT] starting backend...")
	log.Printf("[BOOT] config: server.port=%d, telegram.enable=%v", cfg.Server.Port, cfg.Telegram.Enable)
	if cfg.Telegram.WebhookURL != "" {
		log.Printf("[BOOT] config: telegram.webhook_url=%s", cfg.Telegram.WebhookURL)
	} else {
		log.Printf("[BOOT] config: telegram.webhook_url is empty")
	}
	log.Printf("[BOOT] config: db=%s", utils.MaskDSN(cfg.Database.DSN))
	cfg.Files.RootDir = filepath.Clean(cfg.Files.RootDir)
	log.Printf("[BOOT] config: files.root_dir=%s", cfg.Files.RootDir)
	log.Printf("[BOOT] config: templates docx=%s xlsx=%s txt=%s", cfg.Templates.DocxDir, cfg.Templates.XlsxDir, cfg.Templates.TxtDir)
	log.Printf("[BOOT] config: libreoffice.enable=%v binary=%s", cfg.LibreOffice.Enable, cfg.LibreOffice.Binary)
	jwtSecret := []byte(cfg.Security.JWTSecret)
	if len(jwtSecret) < 32 {
		if gin.Mode() == gin.ReleaseMode {
			log.Fatalf("[BOOT] JWT secret too short or missing (len=%d). Set security.jwt_secret or JWT_SECRET (min 32 bytes).", len(jwtSecret))
		}
		log.Printf("[BOOT] WARNING: JWT secret too short or missing (len=%d). Using insecure dev secret.", len(jwtSecret))
		jwtSecret = []byte("dev-insecure-jwt-secret-min-32-bytes")
	}
	for _, dir := range []string{
		cfg.Files.RootDir,
		filepath.Join(cfg.Files.RootDir, "pdf"),
		filepath.Join(cfg.Files.RootDir, "docx"),
		filepath.Join(cfg.Files.RootDir, "excel"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Printf("[BOOT] failed to create dir %s: %v", dir, err)
		}
	}

	// === DB ===
	db, err := sql.Open("postgres", cfg.Database.DSN)
	if err != nil {
		log.Fatal("[BOOT] Ошибка подключения к БД: ", err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

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
	smsRepo := repositories.NewSMSConfirmationRepository(db)
	verifRepo := repositories.NewUserVerificationRepository(db)
	teleLinkRepo := repositories.NewTelegramLinkRepository(db)
	chatRepo := repositories.NewChatRepository(db)
	passwordResetRepo := repositories.NewPasswordResetRepository(db)
	signSessionRepo := repositories.NewSignSessionRepository(db)
	signatureConfirmRepo := repositories.NewSignatureConfirmationRepository(db)

	// === Services (общие) ===
	authService := services.NewAuthService(jwtSecret, nil, 15*time.Minute, 30*24*time.Hour, nil)
	emailService := services.NewEmailService(
		cfg.Email.SMTPHost,
		cfg.Email.SMTPPort,
		cfg.Email.SMTPUser,
		cfg.Email.SMTPPassword,
		cfg.Email.FromEmail,
	)

	var (
		tgSvc               *services.TelegramService
		integrationsHandler *handlers.IntegrationsHandler
	)

	// Telegram
	if cfg.Telegram.Enable && cfg.Telegram.BotToken != "" {
		log.Printf("[BOOT] Telegram enabled: true (token len=%d)", len(cfg.Telegram.BotToken))
		tgSvc = services.NewTelegramService(cfg.Telegram.BotToken, teleLinkRepo, userRepo, nil, cfg.Frontend.Host)

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
	chatService := services.NewChatService(chatRepo, cfg.Files.RootDir)
	passwordResetService := services.NewPasswordResetService(userRepo, passwordResetRepo, emailService, authService, cfg.Frontend.Host)

	pdfGen := pdf.NewDocumentGenerator(cfg.Files.RootDir, cfg.Templates.TxtDir, "assets/fonts/DejaVuSans.ttf")

	docxGen := docx.NewDocxGenerator(
		cfg.Files.RootDir,
		cfg.Templates.DocxDir,
		cfg.LibreOffice.Enable,
		cfg.LibreOffice.Binary,
	)

	excelGen := xlsx.NewExcelGenerator(cfg.Files.RootDir, cfg.Templates.XlsxDir)

	documentService := services.NewDocumentService(
		documentRepo,
		leadRepo,
		dealRepo,
		clientRepo,
		smsRepo,
		"placeholder-secret",
		cfg.Files.RootDir,
		pdfGen,
		docxGen,
		excelGen,
	)

	taskService := services.NewTaskService(taskRepo, userRepo, tgSvc)
	if tgSvc != nil {
		tgSvc.SetTaskService(taskService)
	}

	// === OTP provider: WhatsApp (same interface) ===
	var otpClient services.SMSClient

	if !cfg.WhatsApp.Enabled {
		log.Fatal("[BOOT] WhatsApp is disabled but required for OTP delivery")
	}
	if !cfg.WhatsApp.DryRun {
		missing := []string{}
		if strings.TrimSpace(cfg.WhatsApp.AccessToken) == "" {
			missing = append(missing, "access_token")
		}
		if strings.TrimSpace(cfg.WhatsApp.PhoneNumberID) == "" {
			missing = append(missing, "phone_number_id")
		}
		if strings.TrimSpace(cfg.WhatsApp.TemplateCodeName) == "" {
			missing = append(missing, "template_code_name")
		}
		if len(missing) > 0 {
			log.Fatalf("[BOOT] WhatsApp config missing: %s", strings.Join(missing, ", "))
		}
	}

	otpClient = utils.NewWhatsAppClient(
		cfg.WhatsApp.AccessToken,
		cfg.WhatsApp.PhoneNumberID,
		cfg.WhatsApp.GraphBaseURL,
		cfg.WhatsApp.TemplateCodeName,
		cfg.WhatsApp.TemplateLang,
		cfg.WhatsApp.DryRun,
	)
	log.Printf("[BOOT] WhatsApp: enabled=true dry_run=%v phone_number_id=%q template_code=%q",
		cfg.WhatsApp.DryRun, cfg.WhatsApp.PhoneNumberID, cfg.WhatsApp.TemplateCodeName)

	// Сервис OTP — для документов
	smsService := services.NewSMSService(
		smsRepo,
		otpClient,
		documentRepo, // Теперь documentRepo передается как третий параметр
		nil,
	)
	smsService.SetAdditionalRepos(clientRepo, dealRepo)

	signDelivery := services.NewWhatsAppSignDelivery(services.SignDeliveryConfig{
		Enabled:          cfg.WhatsApp.Enabled,
		DryRun:           cfg.WhatsApp.DryRun,
		GraphBaseURL:     cfg.WhatsApp.GraphBaseURL,
		PhoneNumberID:    cfg.WhatsApp.PhoneNumberID,
		AccessToken:      cfg.WhatsApp.AccessToken,
		TemplateCodeName: cfg.WhatsApp.TemplateCodeName,
		TemplateLinkName: cfg.WhatsApp.TemplateLinkName,
		TemplateLang:     cfg.WhatsApp.TemplateLang,
	})
	signSessionService := services.NewSignSessionService(
		signSessionRepo,
		documentService,
		signDelivery,
		services.SignSessionConfig{
			SignBaseURL:  cfg.WhatsApp.SignBaseURL,
			DryRun:       cfg.WhatsApp.DryRun,
			TokenVisible: cfg.WhatsApp.DryRun,
		},
		nil,
	)
	signConfirmService := services.NewDocumentSigningConfirmationService(
		signatureConfirmRepo,
		userRepo,
		documentRepo,
		documentService,
		emailService,
		tgSvc,
		services.DocumentSigningConfirmationConfig{
			ConfirmPolicy: cfg.SignConfirmPolicy,
			BaseURL:       cfg.SignEmailVerifyBaseURL,
		},
		nil,
	)
	userVerificationService := services.NewUserVerificationService(
		verifRepo,
		userService,
		emailService,
		nil,
	)

	// Reports
	reportService := services.NewReportService(leadRepo, dealRepo)

	chatHub := realtime.NewChatHub(chatRepo)
	go chatHub.Run()
	defer chatHub.Stop()

	// === Handlers ===
	authHandler := handlers.NewAuthHandler(userService, authService, passwordResetService)
	roleHandler := handlers.NewRoleHandler(roleService)
	userHandler := handlers.NewUserHandler(userService, userVerificationService)
	clientHandler := handlers.NewClientHandler(clientService)
	leadHandler := handlers.NewLeadHandler(leadService)
	dealHandler := handlers.NewDealHandler(dealService)
	documentHandler := handlers.NewDocumentHandler(documentService)
	chatHandler := handlers.NewChatHandler(chatService, chatHub)
	signConfirmHandler := handlers.NewDocumentSigningConfirmationHandler(
		signConfirmService,
		documentService,
		cfg.Frontend.Host,
	)
	telegramSignHandler := handlers.NewTelegramSignWebhookHandler(tgSvc, signConfirmService)

	taskHandler := handlers.NewTaskHandler(taskService, tgSvc, userRepo)

	smsHandler := handlers.NewSMSHandler(smsService)
	verifyHandler := handlers.NewVerifyHandler(userVerificationService)
	signHandler := handlers.NewSignSessionHandler(signSessionService)
	reportHandler := handlers.NewReportHandler(reportService)

	// timezone
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

	if tgSvc != nil {
		integrationsHandler = handlers.NewIntegrationsHandler(tgSvc, teleLinkRepo, userRepo, taskService)
	}

	// === Gin ===
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), corsMiddleware(cfg))

	auditRepo := repositories.NewAuditRepository(db)
	auditSvc := services.NewAuditService(auditRepo)
	router.Use(audit.AuditMiddleware(auditSvc))

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
		signHandler,
		signConfirmHandler,
		telegramSignHandler,
		reportHandler,
		verifyHandler,
		integrationsHandler,
		chatHandler,
		middleware.NewAuthMiddleware(jwtSecret),
	)
	log.Printf("[BOOT] routes mounted. Starting server...")

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("[BOOT] HTTP listen on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatal("[BOOT] Ошибка запуска сервера: ", err)
	}
}

func corsMiddleware(cfg *config.Config) gin.HandlerFunc {
	allowedOrigins := make(map[string]struct{}, len(cfg.CORS.AllowOrigins))
	for _, origin := range cfg.CORS.AllowOrigins {
		allowedOrigins[origin] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			c.Writer.Header().Add("Vary", "Origin")
			if _, ok := allowedOrigins[origin]; ok {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			}
		}
		c.Writer.Header().Set("Access-Control-Allow-Methods", cfg.CORS.AllowMethods)
		c.Writer.Header().Set("Access-Control-Allow-Headers", cfg.CORS.AllowHeaders)
		c.Writer.Header().Set("Access-Control-Expose-Headers", cfg.CORS.ExposeHeaders)
		if c.Request.Method == "OPTIONS" {
			if origin != "" {
				if _, ok := allowedOrigins[origin]; !ok {
					c.AbortWithStatus(403)
					return
				}
			}
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

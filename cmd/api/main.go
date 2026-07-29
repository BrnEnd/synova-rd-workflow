package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"synova-rd-workflow/config"
	evoClient "synova-rd-workflow/internal/client/evolution"
	"synova-rd-workflow/internal/client/openai"
	rdClient "synova-rd-workflow/internal/client/rdstation"
	waClient "synova-rd-workflow/internal/client/whatsapp"
	adminHandler "synova-rd-workflow/internal/handler/admin"
	evoHandler "synova-rd-workflow/internal/handler/evolution"
	waHandler "synova-rd-workflow/internal/handler/whatsapp"
	adminMiddleware "synova-rd-workflow/internal/middleware"
	adminSvc "synova-rd-workflow/internal/service/admin"
	convSvc "synova-rd-workflow/internal/service/conversation"
	intentRouter "synova-rd-workflow/internal/service/intent_router"
	"synova-rd-workflow/internal/service/nlp"
	rdSvc "synova-rd-workflow/internal/service/rdstation"
	adminStore "synova-rd-workflow/internal/store/admin"
	convStore "synova-rd-workflow/internal/store/conversation"
)

// Version is injected at build time via -ldflags.
var Version = "dev"

func main() {
	logger := configureLogger("INFO")
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger = configureLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := convStore.NewDynamoDBStore(ctx, cfg.AWSRegion, cfg.DynamoDBEndpoint, cfg.DynamoDBTableName)
	if err != nil {
		logger.Error("failed to initialize dynamodb store", "error", err)
		os.Exit(1)
	}
	if cfg.DynamoDBEndpoint != "" {
		ensureCtx, ensureCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer ensureCancel()
		if err := ensureLocalTableWithRetry(ensureCtx, store, logger); err != nil {
			logger.Error("failed to ensure local dynamodb table", "error", err)
			os.Exit(1)
		}
	}

	adminData, err := adminStore.NewDynamoDBStore(ctx, cfg.AWSRegion, cfg.DynamoDBEndpoint, cfg.DynamoDBTableName)
	if err != nil {
		logger.Error("failed to initialize admin dynamodb store", "error", err)
		os.Exit(1)
	}

	schedulerCtx, schedulerCancel := context.WithCancel(context.Background())
	defer schedulerCancel()
	r := buildRouter(cfg, logger, store, adminData, schedulerCtx)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      r,
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("server starting", "port", cfg.Port, "version", Version)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server forced shutdown", "error", err)
	}

	logger.Info("server stopped")
}

func ensureLocalTableWithRetry(ctx context.Context, store *convStore.DynamoDBStore, logger *slog.Logger) error {
	var lastErr error
	for attempt := 1; attempt <= 20; attempt++ {
		if err := store.EnsureTable(ctx); err != nil {
			lastErr = err
			logger.Warn("local dynamodb not ready", "attempt", attempt, "error", err)
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		return nil
	}
	return lastErr
}

func buildRouter(cfg *config.Config, logger *slog.Logger, store *convStore.DynamoDBStore, adminData *adminStore.DynamoDBStore, schedulerCtx context.Context) *gin.Engine {
	if strings.ToUpper(cfg.LogLevel) != "DEBUG" {
		gin.SetMode(gin.ReleaseMode)
	}

	conversation := convSvc.New(store, cfg.NLPContextWindow)
	oaiClient := openai.New(cfg.OpenAIAPIKey, cfg.OpenAIModel)
	rd := rdClient.New(cfg.RDStationToken)
	rdstation := rdSvc.New(rd)
	nlpService := nlp.New(oaiClient)
	router := intentRouter.New(rdstation)
	whatsApp := waClient.New(cfg.WhatsAppAccessToken, cfg.WhatsAppPhoneNumberID)
	handler := waHandler.New(conversation, nlpService, router, whatsApp, cfg.WhatsAppVerifyToken, cfg.WhatsAppAppSecret, logger)
	evolution := evoClient.NewWithSendDelay(cfg.EvolutionBaseURL, cfg.EvolutionAPIKey, cfg.EvolutionInstance, cfg.EvolutionSendDelay)
	evolutionHandler := evoHandler.New(conversation, nlpService, router, evolution, logger, cfg.EvolutionAllowedNumbers)
	adminAuth, err := adminSvc.NewAuthService(adminData, cfg.AdminEmail, cfg.AdminInitialPasswordHash, cfg.AdminJWTPrivateKey, cfg.AdminJWTPublicKey)
	if err != nil {
		logger.Error("failed to initialize admin auth", "error", err)
		os.Exit(1)
	}
	adminResources := adminSvc.NewResourceService(adminData, evolution, whatsApp, rd)
	if err := adminResources.SeedAllowlist(context.Background(), cfg.EvolutionAllowedNumbers); err != nil {
		logger.Warn("failed to seed admin allowlist from env", "error", err)
	}
	handler.SetAllowChecker(adminResources)
	evolutionHandler.SetAllowChecker(adminResources)
	admin := adminHandler.New(adminAuth, adminResources, rd, cfg.AdminCookieSecure)
	scheduler := adminSvc.NewScheduler(adminData, rd, evolution, whatsApp, cfg.AlertCheckInterval, logger)
	go scheduler.Start(schedulerCtx)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(loggingMiddleware(logger))
	r.Use(adminMiddleware.AdminSecurityHeaders())
	r.Use(adminMiddleware.AdminCORS(cfg.AdminOrigin))
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "version": Version})
	})
	handler.RegisterRoutes(r)
	evolutionHandler.RegisterRoutes(r)
	adminGroup := r.Group("/admin")
	admin.RegisterPublicRoutes(adminGroup, adminMiddleware.NewLoginRateLimiter().Middleware())
	protectedAdmin := adminGroup.Group("")
	protectedAdmin.Use(adminMiddleware.AdminAuth(adminAuth))
	admin.RegisterProtectedRoutes(protectedAdmin)

	return r
}

func configureLogger(level string) *slog.Logger {
	logLevel := slog.LevelInfo
	switch strings.ToUpper(level) {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "WARN":
		logLevel = slog.LevelWarn
	case "ERROR":
		logLevel = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
}

func loggingMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status_code", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}

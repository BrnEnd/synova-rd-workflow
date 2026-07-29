package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"
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

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger := configureLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := convStore.NewDynamoDBStore(ctx, cfg.AWSRegion, cfg.DynamoDBEndpoint, cfg.DynamoDBTableName)
	if err != nil {
		panic(err)
	}

	adminData, err := adminStore.NewDynamoDBStore(ctx, cfg.AWSRegion, cfg.DynamoDBEndpoint, cfg.DynamoDBTableName)
	if err != nil {
		panic(err)
	}

	router := buildRouter(cfg, logger, store, adminData)
	lambda.Start(ginadapter.NewV2(router).ProxyWithContext)
}

func buildRouter(cfg *config.Config, logger *slog.Logger, store *convStore.DynamoDBStore, adminData *adminStore.DynamoDBStore) *gin.Engine {
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
		panic(err)
	}
	adminResources := adminSvc.NewResourceService(adminData, evolution, whatsApp, rd)
	if err := adminResources.SeedAllowlist(context.Background(), cfg.EvolutionAllowedNumbers); err != nil {
		logger.Warn("failed to seed admin allowlist from env", "error", err)
	}
	handler.SetAllowChecker(adminResources)
	evolutionHandler.SetAllowChecker(adminResources)
	admin := adminHandler.New(adminAuth, adminResources, rd, cfg.AdminCookieSecure)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(loggingMiddleware(logger))
	r.Use(adminMiddleware.AdminSecurityHeaders())
	r.Use(adminMiddleware.AdminCORS(cfg.AdminOrigin))
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
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

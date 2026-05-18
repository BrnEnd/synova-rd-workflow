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
	"synova-rd-workflow/internal/client/openai"
	rdClient "synova-rd-workflow/internal/client/rdstation"
	waClient "synova-rd-workflow/internal/client/whatsapp"
	waHandler "synova-rd-workflow/internal/handler/whatsapp"
	convSvc "synova-rd-workflow/internal/service/conversation"
	intentRouter "synova-rd-workflow/internal/service/intent_router"
	"synova-rd-workflow/internal/service/nlp"
	rdSvc "synova-rd-workflow/internal/service/rdstation"
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

	router := buildRouter(cfg, logger, store)
	lambda.Start(ginadapter.NewV2(router).ProxyWithContext)
}

func buildRouter(cfg *config.Config, logger *slog.Logger, store *convStore.DynamoDBStore) *gin.Engine {
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

	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	handler.RegisterRoutes(r)

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

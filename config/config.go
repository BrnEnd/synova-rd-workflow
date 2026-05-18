package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	WhatsAppAccessToken      string
	WhatsAppPhoneNumberID    string
	WhatsAppVerifyToken      string
	WhatsAppAppSecret        string
	EvolutionBaseURL         string
	EvolutionAPIKey          string
	EvolutionInstance        string
	EvolutionAllowedNumbers  []string
	OpenAIAPIKey             string
	RDStationToken           string
	DynamoDBTableName        string
	DynamoDBEndpoint         string
	AWSRegion                string
	Port                     string
	LogLevel                 string
	NLPContextWindow         int
	OpenAIModel              string
	AdminEmail               string
	AdminInitialPasswordHash string
	AdminJWTPrivateKey       string
	AdminJWTPublicKey        string
	AdminOrigin              string
	AdminCookieSecure        bool
	AlertCheckInterval       time.Duration
	EvolutionSendDelay       time.Duration
}

// Load reads .env (if present) and validates all required variables.
// It panics if any required variable is missing.
func Load() (*Config, error) {
	// Load .env file if it exists — ignore error (file optional in container envs)
	_ = godotenv.Load()

	cfg := &Config{}

	var missing []string

	cfg.WhatsAppAccessToken = os.Getenv("WHATSAPP_ACCESS_TOKEN")
	cfg.WhatsAppPhoneNumberID = os.Getenv("WHATSAPP_PHONE_NUMBER_ID")
	cfg.WhatsAppVerifyToken = os.Getenv("WHATSAPP_VERIFY_TOKEN")
	cfg.WhatsAppAppSecret = os.Getenv("WHATSAPP_APP_SECRET")
	cfg.EvolutionBaseURL = getEnvOrDefault("EVOLUTION_BASE_URL", "http://evolution-api:8080")
	cfg.EvolutionAPIKey = os.Getenv("EVOLUTION_API_KEY")
	cfg.EvolutionInstance = getEnvOrDefault("EVOLUTION_INSTANCE", "synova")
	cfg.EvolutionAllowedNumbers = parseCSV(os.Getenv("EVOLUTION_ALLOWED_NUMBERS"))
	cfg.OpenAIAPIKey = requireEnv("OPENAI_API_KEY", &missing)
	cfg.RDStationToken = requireAnyEnv([]string{"RDSTATION_TOKEN", "RDSTATION_API_KEY"}, &missing)
	cfg.DynamoDBTableName = requireEnv("DYNAMODB_TABLE_NAME", &missing)

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missing)
	}

	cfg.Port = getEnvOrDefault("APP_PORT", getEnvOrDefault("PORT", "8080"))
	cfg.LogLevel = getEnvOrDefault("LOG_LEVEL", "INFO")
	cfg.OpenAIModel = getEnvOrDefault("OPENAI_MODEL", "gpt-5.4-nano")
	cfg.AWSRegion = getEnvOrDefault("AWS_REGION_APP", getEnvOrDefault("AWS_REGION", "us-east-1"))
	cfg.DynamoDBEndpoint = os.Getenv("DYNAMODB_ENDPOINT")
	cfg.AdminEmail = getEnvOrDefault("ADMIN_EMAIL", "admin@synova.local")
	cfg.AdminInitialPasswordHash = os.Getenv("ADMIN_INITIAL_PASSWORD_HASH")
	cfg.AdminJWTPrivateKey = os.Getenv("ADMIN_JWT_PRIVATE_KEY")
	cfg.AdminJWTPublicKey = os.Getenv("ADMIN_JWT_PUBLIC_KEY")
	cfg.AdminOrigin = getEnvOrDefault("ADMIN_ORIGIN", "http://localhost:3002")
	cfg.AdminCookieSecure = parseBool(getEnvOrDefault("ADMIN_COOKIE_SECURE", "false"))
	sendDelay, err := time.ParseDuration(getEnvOrDefault("EVOLUTION_SEND_DELAY", "10s"))
	if err != nil || sendDelay < 0 {
		return nil, fmt.Errorf("EVOLUTION_SEND_DELAY must be a duration, e.g. 10s")
	}
	cfg.EvolutionSendDelay = sendDelay

	alertInterval, err := time.ParseDuration(getEnvOrDefault("ALERT_CHECK_INTERVAL", "30m"))
	if err != nil || alertInterval <= 0 {
		return nil, fmt.Errorf("ALERT_CHECK_INTERVAL must be a positive duration, e.g. 30m")
	}
	cfg.AlertCheckInterval = alertInterval

	contextWindow, err := strconv.Atoi(getEnvOrDefault("NLP_CONTEXT_WINDOW", "10"))
	if err != nil || contextWindow <= 0 {
		return nil, fmt.Errorf("NLP_CONTEXT_WINDOW must be a positive integer")
	}
	cfg.NLPContextWindow = contextWindow

	return cfg, nil
}

func requireEnv(key string, missing *[]string) string {
	v := os.Getenv(key)
	if v == "" {
		*missing = append(*missing, key)
	}
	return v
}

func requireAnyEnv(keys []string, missing *[]string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	*missing = append(*missing, keys[0])
	return ""
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func parseCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"synova-rd-workflow/config"
	evoClient "synova-rd-workflow/internal/client/evolution"
	rdClient "synova-rd-workflow/internal/client/rdstation"
	waClient "synova-rd-workflow/internal/client/whatsapp"
	adminSvc "synova-rd-workflow/internal/service/admin"
	adminStore "synova-rd-workflow/internal/store/admin"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: go run ./cmd/admin-test-alert <alert-id>")
	}
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := adminStore.NewDynamoDBStore(ctx, cfg.AWSRegion, cfg.DynamoDBEndpoint, cfg.DynamoDBTableName)
	if err != nil {
		log.Fatal(err)
	}
	evolution := evoClient.NewWithSendDelay(cfg.EvolutionBaseURL, cfg.EvolutionAPIKey, cfg.EvolutionInstance, cfg.EvolutionSendDelay)
	whatsApp := waClient.New(cfg.WhatsAppAccessToken, cfg.WhatsAppPhoneNumberID)
	rd := rdClient.New(cfg.RDStationToken)
	resources := adminSvc.NewResourceService(store, evolution, whatsApp, rd)
	result, err := resources.RunAlertNow(ctx, os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("alert run completed: deals_matched=%d messages_sent=%d skipped_dedup=%d send_errors=%d\n", result.DealsMatched, result.MessagesSent, result.SkippedDedup, result.SendErrors)
}

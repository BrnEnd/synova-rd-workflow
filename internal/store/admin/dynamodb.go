package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"synova-rd-workflow/internal/domain"
)

const (
	adminPKConfig        = "ADMIN#CONFIG"
	adminPKCollaborators = "ADMIN#COLLABORATORS"
	adminPKAlerts        = "ADMIN#ALERTS"
	adminPKAllowlist     = "ADMIN#ALLOWLIST"
	adminPKAlertSent     = "ADMIN#ALERT_SENT"
)

type DynamoDBStore struct {
	client *dynamodb.Client
	table  string
}

func NewDynamoDBStore(ctx context.Context, region, endpoint, table string) (*DynamoDBStore, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("admin dynamodb load config: %w", err)
	}
	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})
	return &DynamoDBStore{client: client, table: table}, nil
}

func (s *DynamoDBStore) GetConfig(ctx context.Context) (domain.AdminConfig, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: adminPKConfig},
			"SK": &types.AttributeValueMemberS{Value: "admin"},
		},
	})
	if err != nil {
		return domain.AdminConfig{}, fmt.Errorf("admin config get: %w", err)
	}
	if len(out.Item) == 0 {
		return domain.AdminConfig{}, nil
	}
	var item adminConfigItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return domain.AdminConfig{}, fmt.Errorf("admin config unmarshal: %w", err)
	}
	return domain.AdminConfig{Email: item.Email, PasswordHash: item.PasswordHash, MustChangePassword: item.MustChangePassword, UpdatedAt: item.UpdatedAt}, nil
}

func (s *DynamoDBStore) SaveConfig(ctx context.Context, cfg domain.AdminConfig) error {
	cfg.UpdatedAt = time.Now().UTC()
	item := adminConfigItem{PK: adminPKConfig, SK: "admin", Email: strings.ToLower(cfg.Email), PasswordHash: cfg.PasswordHash, MustChangePassword: cfg.MustChangePassword, UpdatedAt: cfg.UpdatedAt, EntityType: "admin_config"}
	return s.put(ctx, item)
}

func (s *DynamoDBStore) ListCollaborators(ctx context.Context, activeOnly bool) ([]domain.Collaborator, error) {
	var items []collaboratorItem
	if err := s.queryPK(ctx, adminPKCollaborators, &items); err != nil {
		return nil, err
	}
	out := make([]domain.Collaborator, 0, len(items))
	for _, item := range items {
		if activeOnly && !item.Active {
			continue
		}
		out = append(out, item.domain())
	}
	return out, nil
}

func (s *DynamoDBStore) GetCollaborator(ctx context.Context, id string) (domain.Collaborator, error) {
	var item collaboratorItem
	if ok, err := s.get(ctx, adminPKCollaborators, id, &item); err != nil || !ok {
		return domain.Collaborator{}, err
	}
	return item.domain(), nil
}

func (s *DynamoDBStore) SaveCollaborator(ctx context.Context, c domain.Collaborator) error {
	item := collaboratorItem{PK: adminPKCollaborators, SK: c.ID, ID: c.ID, Name: c.Name, Email: strings.ToLower(c.Email), WhatsApp: c.WhatsApp, Active: c.Active, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, EntityType: "admin_collaborator"}
	return s.put(ctx, item)
}

func (s *DynamoDBStore) FindCollaboratorByEmail(ctx context.Context, email string) (domain.Collaborator, error) {
	list, err := s.ListCollaborators(ctx, false)
	if err != nil {
		return domain.Collaborator{}, err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	for _, item := range list {
		if strings.ToLower(item.Email) == email {
			return item, nil
		}
	}
	return domain.Collaborator{}, nil
}

func (s *DynamoDBStore) ListAlerts(ctx context.Context, activeOnly bool) ([]domain.Alert, error) {
	var items []alertItem
	if err := s.queryPK(ctx, adminPKAlerts, &items); err != nil {
		return nil, err
	}
	out := make([]domain.Alert, 0, len(items))
	for _, item := range items {
		if activeOnly && !item.Active {
			continue
		}
		out = append(out, item.domain())
	}
	return out, nil
}

func (s *DynamoDBStore) GetAlert(ctx context.Context, id string) (domain.Alert, error) {
	var item alertItem
	if ok, err := s.get(ctx, adminPKAlerts, id, &item); err != nil || !ok {
		return domain.Alert{}, err
	}
	return item.domain(), nil
}

func (s *DynamoDBStore) SaveAlert(ctx context.Context, a domain.Alert) error {
	item := alertItem{PK: adminPKAlerts, SK: a.ID, ID: a.ID, Name: a.Name, DealStageID: a.DealStageID, DealStageName: a.DealStageName, TimeThresholdHours: a.TimeThresholdHours, RepeatIntervalHours: a.RepeatIntervalHours, MessageTemplate: a.MessageTemplate, RecipientIDs: a.RecipientIDs, Active: a.Active, LastCheckedAt: a.LastCheckedAt, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt, EntityType: "admin_alert"}
	return s.put(ctx, item)
}

func (s *DynamoDBStore) ListAllowlist(ctx context.Context, activeOnly bool) ([]domain.AllowlistEntry, error) {
	var items []allowlistItem
	if err := s.queryPK(ctx, adminPKAllowlist, &items); err != nil {
		return nil, err
	}
	out := make([]domain.AllowlistEntry, 0, len(items))
	for _, item := range items {
		if activeOnly && !item.Active {
			continue
		}
		out = append(out, item.domain())
	}
	return out, nil
}

func (s *DynamoDBStore) GetAllowlistEntry(ctx context.Context, id string) (domain.AllowlistEntry, error) {
	var item allowlistItem
	if ok, err := s.get(ctx, adminPKAllowlist, id, &item); err != nil || !ok {
		return domain.AllowlistEntry{}, err
	}
	return item.domain(), nil
}

func (s *DynamoDBStore) SaveAllowlistEntry(ctx context.Context, e domain.AllowlistEntry) error {
	item := allowlistItem{PK: adminPKAllowlist, SK: e.ID, ID: e.ID, PhoneNumber: e.PhoneNumber, Label: e.Label, Active: e.Active, SyncPending: e.SyncPending, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt, EntityType: "admin_allowlist"}
	return s.put(ctx, item)
}

func (s *DynamoDBStore) FindAllowlistByPhone(ctx context.Context, phone string) (domain.AllowlistEntry, error) {
	list, err := s.ListAllowlist(ctx, false)
	if err != nil {
		return domain.AllowlistEntry{}, err
	}
	phone = normalizePhone(phone)
	for _, item := range list {
		if normalizePhone(item.PhoneNumber) == phone {
			return item, nil
		}
	}
	return domain.AllowlistEntry{}, nil
}

func (s *DynamoDBStore) WasAlertSent(ctx context.Context, key string) (bool, error) {
	var item alertSentItem
	ok, err := s.get(ctx, adminPKAlertSent, key, &item)
	if err != nil || !ok {
		return false, err
	}
	return item.TTL > time.Now().Unix(), nil
}

func (s *DynamoDBStore) MarkAlertSent(ctx context.Context, key string, ttl time.Time) error {
	item := alertSentItem{PK: adminPKAlertSent, SK: key, Key: key, SentAt: time.Now().UTC(), TTL: ttl.Unix(), EntityType: "admin_alert_sent"}
	return s.put(ctx, item)
}

func (s *DynamoDBStore) get(ctx context.Context, pk, sk string, out interface{}) (bool, error) {
	res, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	})
	if err != nil {
		return false, fmt.Errorf("admin get %s/%s: %w", pk, sk, err)
	}
	if len(res.Item) == 0 {
		return false, nil
	}
	if err := attributevalue.UnmarshalMap(res.Item, out); err != nil {
		return false, fmt.Errorf("admin unmarshal %s/%s: %w", pk, sk, err)
	}
	return true, nil
}

func (s *DynamoDBStore) put(ctx context.Context, item interface{}) error {
	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("admin marshal: %w", err)
	}
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(s.table), Item: av})
	if err != nil {
		return fmt.Errorf("admin put: %w", err)
	}
	return nil
}

func (s *DynamoDBStore) queryPK(ctx context.Context, pk string, out interface{}) error {
	res, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.table),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
	})
	if err != nil {
		var notFound *types.ResourceNotFoundException
		if errors.As(err, &notFound) {
			return nil
		}
		return fmt.Errorf("admin query %s: %w", pk, err)
	}
	if err := attributevalue.UnmarshalListOfMaps(res.Items, out); err != nil {
		return fmt.Errorf("admin query unmarshal %s: %w", pk, err)
	}
	return nil
}

func normalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.TrimPrefix(phone, "+")
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, "(", "")
	phone = strings.ReplaceAll(phone, ")", "")
	return phone
}

type adminConfigItem struct {
	PK                 string    `dynamodbav:"PK"`
	SK                 string    `dynamodbav:"SK"`
	Email              string    `dynamodbav:"email"`
	PasswordHash       string    `dynamodbav:"password_hash"`
	MustChangePassword bool      `dynamodbav:"must_change_password"`
	UpdatedAt          time.Time `dynamodbav:"updated_at"`
	EntityType         string    `dynamodbav:"entity_type"`
}

type collaboratorItem struct {
	PK         string    `dynamodbav:"PK"`
	SK         string    `dynamodbav:"SK"`
	ID         string    `dynamodbav:"id"`
	Name       string    `dynamodbav:"name"`
	Email      string    `dynamodbav:"email"`
	WhatsApp   string    `dynamodbav:"whatsapp"`
	Active     bool      `dynamodbav:"active"`
	CreatedAt  time.Time `dynamodbav:"created_at"`
	UpdatedAt  time.Time `dynamodbav:"updated_at"`
	EntityType string    `dynamodbav:"entity_type"`
}

func (i collaboratorItem) domain() domain.Collaborator {
	return domain.Collaborator{ID: i.ID, Name: i.Name, Email: i.Email, WhatsApp: i.WhatsApp, Active: i.Active, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt}
}

type alertItem struct {
	PK                  string    `dynamodbav:"PK"`
	SK                  string    `dynamodbav:"SK"`
	ID                  string    `dynamodbav:"id"`
	Name                string    `dynamodbav:"name"`
	DealStageID         string    `dynamodbav:"deal_stage_id"`
	DealStageName       string    `dynamodbav:"deal_stage_name"`
	TimeThresholdHours  int       `dynamodbav:"time_threshold_hours"`
	RepeatIntervalHours int       `dynamodbav:"repeat_interval_hours"`
	MessageTemplate     string    `dynamodbav:"message_template"`
	RecipientIDs        []string  `dynamodbav:"recipient_ids"`
	Active              bool      `dynamodbav:"active"`
	LastCheckedAt       time.Time `dynamodbav:"last_checked_at"`
	CreatedAt           time.Time `dynamodbav:"created_at"`
	UpdatedAt           time.Time `dynamodbav:"updated_at"`
	EntityType          string    `dynamodbav:"entity_type"`
}

func (i alertItem) domain() domain.Alert {
	return domain.Alert{ID: i.ID, Name: i.Name, DealStageID: i.DealStageID, DealStageName: i.DealStageName, TimeThresholdHours: i.TimeThresholdHours, RepeatIntervalHours: i.RepeatIntervalHours, MessageTemplate: i.MessageTemplate, RecipientIDs: i.RecipientIDs, Active: i.Active, LastCheckedAt: i.LastCheckedAt, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt}
}

type allowlistItem struct {
	PK          string    `dynamodbav:"PK"`
	SK          string    `dynamodbav:"SK"`
	ID          string    `dynamodbav:"id"`
	PhoneNumber string    `dynamodbav:"phone_number"`
	Label       string    `dynamodbav:"label"`
	Active      bool      `dynamodbav:"active"`
	SyncPending bool      `dynamodbav:"sync_pending"`
	CreatedAt   time.Time `dynamodbav:"created_at"`
	UpdatedAt   time.Time `dynamodbav:"updated_at"`
	EntityType  string    `dynamodbav:"entity_type"`
}

func (i allowlistItem) domain() domain.AllowlistEntry {
	return domain.AllowlistEntry{ID: i.ID, PhoneNumber: i.PhoneNumber, Label: i.Label, Active: i.Active, SyncPending: i.SyncPending, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt}
}

type alertSentItem struct {
	PK         string    `dynamodbav:"PK"`
	SK         string    `dynamodbav:"SK"`
	Key        string    `dynamodbav:"key"`
	SentAt     time.Time `dynamodbav:"sent_at"`
	TTL        int64     `dynamodbav:"ttl"`
	EntityType string    `dynamodbav:"entity_type"`
}

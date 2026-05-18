package conversation

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
	"github.com/google/uuid"
	"synova-rd-workflow/internal/domain"
)

const (
	sessionSK       = "SESSION"
	messageSKPrefix = "MSG#"
	messageTTL      = 30 * 24 * time.Hour
)

// DynamoDBStore implements Store using a single DynamoDB table.
type DynamoDBStore struct {
	client *dynamodb.Client
	table  string
}

// NewDynamoDBStore loads AWS config and returns a DynamoDB-backed store.
func NewDynamoDBStore(ctx context.Context, region, endpoint, table string) (*DynamoDBStore, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("dynamodb load config: %w", err)
	}

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})

	return &DynamoDBStore{client: client, table: table}, nil
}

// EnsureTable creates the local development table when it does not exist.
func (s *DynamoDBStore) EnsureTable(ctx context.Context) error {
	_, err := s.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(s.table)})
	if err == nil {
		return nil
	}

	var notFound *types.ResourceNotFoundException
	if !errors.As(err, &notFound) {
		return fmt.Errorf("dynamodb describe table: %w", err)
	}

	_, err = s.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:   aws.String(s.table),
		BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("SK"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("PK"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("SK"), KeyType: types.KeyTypeRange},
		},
	})
	if err != nil {
		return fmt.Errorf("dynamodb create table: %w", err)
	}

	waiter := dynamodb.NewTableExistsWaiter(s.client)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(s.table)}, 30*time.Second); err != nil {
		return fmt.Errorf("dynamodb wait table: %w", err)
	}

	_, _ = s.client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(s.table),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("ttl"),
			Enabled:       aws.Bool(true),
		},
	})

	return nil
}

// NewDynamoDBStoreWithClient returns a store backed by an existing client for tests.
func NewDynamoDBStoreWithClient(client *dynamodb.Client, table string) *DynamoDBStore {
	return &DynamoDBStore{client: client, table: table}
}

func (s *DynamoDBStore) GetOrCreateSession(ctx context.Context, phoneNumber string) (domain.Session, error) {
	phoneNumber = normalizeE164(phoneNumber)
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: phonePK(phoneNumber)},
		"SK": &types.AttributeValueMemberS{Value: sessionSK},
	}

	existing, err := s.getSession(ctx, key)
	if err != nil {
		return domain.Session{}, err
	}
	if existing.PhoneNumber != "" {
		return existing, nil
	}

	now := time.Now().UTC()
	item := sessionItem{
		PK:         phonePK(phoneNumber),
		SK:         sessionSK,
		SessionID:  phoneNumber,
		Phone:      phoneNumber,
		CreatedAt:  now,
		UpdatedAt:  now,
		EntityType: "session",
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return domain.Session{}, fmt.Errorf("dynamodb marshal session: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.table),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		var conditional *types.ConditionalCheckFailedException
		if errors.As(err, &conditional) {
			return s.getSession(ctx, key)
		}
		return domain.Session{}, fmt.Errorf("dynamodb put session: %w", err)
	}

	return domain.Session{
		ID:          item.SessionID,
		PhoneNumber: item.Phone,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}, nil
}

func (s *DynamoDBStore) getSession(ctx context.Context, key map[string]types.AttributeValue) (domain.Session, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key:       key,
	})
	if err != nil {
		return domain.Session{}, fmt.Errorf("dynamodb get session: %w", err)
	}
	if len(out.Item) == 0 {
		return domain.Session{}, nil
	}

	var item sessionItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return domain.Session{}, fmt.Errorf("dynamodb unmarshal session: %w", err)
	}

	return domain.Session{
		ID:          item.SessionID,
		PhoneNumber: item.Phone,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}, nil
}

func (s *DynamoDBStore) GetRecentMessages(ctx context.Context, sessionID string, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		return nil, nil
	}

	phoneNumber := normalizeE164(sessionID)
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: phonePK(phoneNumber)},
			":prefix": &types.AttributeValueMemberS{Value: messageSKPrefix},
		},
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int32(int32(limit)),
	})
	if err != nil {
		return nil, fmt.Errorf("dynamodb query messages: %w", err)
	}

	msgs := make([]domain.Message, 0, len(out.Items))
	for _, raw := range out.Items {
		var item messageItem
		if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
			return nil, fmt.Errorf("dynamodb unmarshal message: %w", err)
		}
		msgs = append(msgs, domain.Message{
			ID:        item.MessageID,
			SessionID: item.SessionID,
			Role:      item.Role,
			Content:   item.Content,
			Intent:    item.Intent,
			CreatedAt: item.CreatedAt,
		})
	}

	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	return msgs, nil
}

func (s *DynamoDBStore) SaveMessage(ctx context.Context, msg domain.Message) error {
	createdAt := msg.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	id := msg.ID
	if id == "" {
		id = uuid.NewString()
	}
	phoneNumber := normalizeE164(msg.SessionID)

	item := messageItem{
		PK:         phonePK(phoneNumber),
		SK:         fmt.Sprintf("%s%013d#%s", messageSKPrefix, createdAt.UnixMilli(), id),
		MessageID:  id,
		SessionID:  phoneNumber,
		Role:       msg.Role,
		Content:    msg.Content,
		Intent:     msg.Intent,
		CreatedAt:  createdAt.UTC(),
		TTL:        createdAt.Add(messageTTL).Unix(),
		EntityType: "message",
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("dynamodb marshal message: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.table),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("dynamodb put message: %w", err)
	}

	return nil
}

func phonePK(phoneNumber string) string {
	return "PHONE#" + normalizeE164(phoneNumber)
}

func normalizeE164(phoneNumber string) string {
	phoneNumber = strings.TrimSpace(phoneNumber)
	if phoneNumber == "" || strings.HasPrefix(phoneNumber, "+") {
		return phoneNumber
	}
	return "+" + phoneNumber
}

type sessionItem struct {
	PK         string    `dynamodbav:"PK"`
	SK         string    `dynamodbav:"SK"`
	SessionID  string    `dynamodbav:"session_id"`
	Phone      string    `dynamodbav:"phone"`
	CreatedAt  time.Time `dynamodbav:"created_at"`
	UpdatedAt  time.Time `dynamodbav:"updated_at"`
	EntityType string    `dynamodbav:"entity_type"`
}

type messageItem struct {
	PK         string    `dynamodbav:"PK"`
	SK         string    `dynamodbav:"SK"`
	MessageID  string    `dynamodbav:"message_id"`
	SessionID  string    `dynamodbav:"session_id"`
	Role       string    `dynamodbav:"role"`
	Content    string    `dynamodbav:"content"`
	Intent     string    `dynamodbav:"intent,omitempty"`
	CreatedAt  time.Time `dynamodbav:"created_at"`
	TTL        int64     `dynamodbav:"ttl"`
	EntityType string    `dynamodbav:"entity_type"`
}

package rdstation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://crm.rdstation.com/api/v1"

// RDStationError represents a typed error from the RD Station API.
type RDStationError struct {
	Code    int
	Message string
}

func (e *RDStationError) Error() string {
	return fmt.Sprintf("rdstation error %d: %s", e.Code, e.Message)
}

// --- Request/Response types ---

type ContactResponse struct {
	ID        string `json:"_id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type ContactsListResponse struct {
	Contacts []ContactResponse `json:"contacts"`
	Total    int               `json:"total"`
}

type DealStageResponse struct {
	ID   string `json:"_id"`
	Name string `json:"name"`
}

type DealContactResponse struct {
	ID   string `json:"_id"`
	Name string `json:"name"`
}

type DealUserResponse struct {
	ID    string `json:"_id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type DealResponse struct {
	ID        string                `json:"_id"`
	Name      string                `json:"name"`
	DealStage DealStageResponse     `json:"deal_stage"`
	Contacts  []DealContactResponse `json:"contacts"`
	User      DealUserResponse      `json:"user"`
	Owner     DealUserResponse      `json:"owner"`
	DealOwner DealUserResponse      `json:"deal_owner"`
	CreatedAt string                `json:"created_at"`
	UpdatedAt string                `json:"updated_at"`
}

type DealsListResponse struct {
	Deals []DealResponse `json:"deals"`
	Total int            `json:"total"`
}

type DealContactsListResponse struct {
	Contacts []DealContactResponse `json:"contacts"`
	Total    int                   `json:"total"`
}

type ActivityResponse struct {
	ID     string `json:"_id"`
	Date   string `json:"date"`
	DealID string `json:"deal_id"`
	Text   string `json:"text"`
	UserID string `json:"user_id"`
}

type ActivitiesListResponse struct {
	Activities []ActivityResponse `json:"activities"`
	Total      int                `json:"total"`
}

type TaskDealResponse struct {
	ID   string `json:"_id"`
	Name string `json:"name"`
}

type TaskUserResponse struct {
	ID       string `json:"_id"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
}

type TaskResponse struct {
	ID        string             `json:"_id"`
	CreatedAt string             `json:"created_at"`
	Date      string             `json:"date"`
	Deal      TaskDealResponse   `json:"deal"`
	Done      bool               `json:"done"`
	Hour      string             `json:"hour"`
	Markup    string             `json:"markup"`
	Notes     string             `json:"notes"`
	Subject   string             `json:"subject"`
	Type      string             `json:"type"`
	UserIDs   []string           `json:"user_ids"`
	Users     []TaskUserResponse `json:"users"`
}

type TasksListResponse struct {
	Tasks []TaskResponse `json:"tasks"`
	Total int            `json:"total"`
}

type DealStageListResponse struct {
	DealStages []struct {
		ID             string `json:"_id"`
		Name           string `json:"name"`
		DealPipelineID string `json:"deal_pipeline_id"`
	} `json:"deal_stages"`
}

// --- Params ---

type GetContactsParams struct {
	Name  string
	Email string
	Phone string
}

type CreateContactParams struct {
	Name  string
	Email string
	Phone string
}

type GetDealsParams struct {
	Name        string
	DealStageID string
	Win         *bool
}

type GetTasksParams struct {
	Done   *bool
	UserID string
	DealID string
	Limit  int
}

type CreateTaskParams struct {
	DealID  string
	Subject string
	Type    string
	Date    string
	Hour    string
	Notes   string
	UserIDs []string
}

type CreateDealParams struct {
	Name        string
	DealStageID string
	ContactIDs  []string
	UserID      string
	Products    []DealProductParams
}

type UpdateDealParams struct {
	Name        string
	DealStageID string
	ContactIDs  []string // if non-empty, sets contacts_attributes on the deal
}

type DealProductParams struct {
	Name        string
	Description string
	Amount      float64
	BasePrice   float64
	Price       float64
}

type UpdateContactParams struct {
	Name  string
	Email string
	Phone string
}

// --- Client ---

// Client wraps the RD Station CRM REST API.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// New returns a configured RD Station API client using the default base URL.
func New(token string) *Client {
	return NewWithBaseURL(token, defaultBaseURL)
}

// NewWithBaseURL returns a client with a custom base URL (used in tests).
func NewWithBaseURL(token, baseURL string) *Client {
	return &Client{
		token:   token,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("rdstation client marshal: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	// Append token as query param
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	fullURL := c.baseURL + path + sep + "token=" + c.token

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("rdstation client new request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.doWithRetry(req)
}

func (c *Client) doWithRetry(req *http.Request) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("rdstation http do: %w", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("rdstation read body: %w", err)
			continue
		}

		switch {
		case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated:
			return body, nil

		case resp.StatusCode == 429:
			retryAfter := 60 * time.Second
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, parseErr := strconv.Atoi(ra); parseErr == nil {
					retryAfter = time.Duration(secs) * time.Second
				}
			}
			time.Sleep(retryAfter)
			// one retry after rate limit
			resp2, err2 := c.httpClient.Do(req)
			if err2 != nil {
				return nil, &RDStationError{Code: 429, Message: "rate limit exceeded"}
			}
			body2, _ := io.ReadAll(resp2.Body)
			resp2.Body.Close()
			if resp2.StatusCode == http.StatusOK || resp2.StatusCode == http.StatusCreated {
				return body2, nil
			}
			return nil, &RDStationError{Code: resp2.StatusCode, Message: string(body2)}

		case resp.StatusCode >= 400 && resp.StatusCode < 500:
			// 4xx — do not retry
			return nil, &RDStationError{Code: resp.StatusCode, Message: string(body)}

		default:
			// 5xx — retry
			lastErr = &RDStationError{Code: resp.StatusCode, Message: string(body)}
		}
	}

	return nil, lastErr
}

// GetContacts fetches contacts from RD Station filtered by the given params.
func (c *Client) GetContacts(ctx context.Context, params GetContactsParams) ([]ContactResponse, error) {
	q := url.Values{}
	if params.Name != "" {
		q.Set("name", params.Name)
	}
	if params.Email != "" {
		q.Set("email", params.Email)
	}
	if params.Phone != "" {
		q.Set("phone", params.Phone)
	}

	path := "/contacts"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	data, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var result ContactsListResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("rdstation GetContacts unmarshal: %w", err)
	}

	return result.Contacts, nil
}

// CreateContact creates a new contact in RD Station.
func (c *Client) CreateContact(ctx context.Context, params CreateContactParams) (ContactResponse, error) {
	payload := map[string]interface{}{
		"contact": map[string]interface{}{
			"name":  params.Name,
			"email": params.Email,
			"phone": params.Phone,
		},
	}

	data, err := c.do(ctx, http.MethodPost, "/contacts", payload)
	if err != nil {
		return ContactResponse{}, err
	}

	var result ContactResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return ContactResponse{}, fmt.Errorf("rdstation CreateContact unmarshal: %w", err)
	}

	return result, nil
}

// GetDeals fetches deals from RD Station.
func (c *Client) GetDeals(ctx context.Context, params GetDealsParams) ([]DealResponse, error) {
	q := url.Values{"limit": []string{"200"}}
	if params.Name != "" {
		q.Set("name", params.Name)
	}
	if params.DealStageID != "" {
		q.Set("deal_stage_id", params.DealStageID)
	}
	if params.Win != nil {
		q.Set("win", strconv.FormatBool(*params.Win))
	}

	path := "/deals"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	data, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var result DealsListResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("rdstation GetDeals unmarshal: %w", err)
	}

	return result.Deals, nil
}

// GetTasks fetches scheduled RD Station tasks.
func (c *Client) GetTasks(ctx context.Context, params GetTasksParams) ([]TaskResponse, error) {
	limit := params.Limit
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	q := url.Values{"limit": []string{strconv.Itoa(limit)}}
	if params.Done != nil {
		q.Set("done", strconv.FormatBool(*params.Done))
	}
	if params.UserID != "" {
		q.Set("user_id", params.UserID)
	}
	if params.DealID != "" {
		q.Set("deal_id", params.DealID)
	}

	data, err := c.do(ctx, http.MethodGet, "/tasks?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var result TasksListResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("rdstation GetTasks unmarshal: %w", err)
	}
	return result.Tasks, nil
}

// CreateTask creates a scheduled RD Station task linked to a deal.
func (c *Client) CreateTask(ctx context.Context, params CreateTaskParams) (TaskResponse, error) {
	task := map[string]interface{}{
		"deal_id": params.DealID,
		"subject": params.Subject,
		"type":    params.Type,
		"date":    params.Date,
		"hour":    params.Hour,
	}
	if params.Notes != "" {
		task["notes"] = params.Notes
	}
	if len(params.UserIDs) > 0 {
		task["user_ids"] = params.UserIDs
	}
	payload := map[string]interface{}{"task": task}
	data, err := c.do(ctx, http.MethodPost, "/tasks", payload)
	if err != nil {
		return TaskResponse{}, err
	}
	var result TaskResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return TaskResponse{}, fmt.Errorf("rdstation CreateTask unmarshal: %w", err)
	}
	return result, nil
}

// CreateDeal creates a new deal in RD Station.
func (c *Client) CreateDeal(ctx context.Context, params CreateDealParams) (DealResponse, error) {
	contactAttrs := make([]map[string]string, len(params.ContactIDs))
	for i, id := range params.ContactIDs {
		contactAttrs[i] = map[string]string{"_id": id}
	}

	deal := map[string]interface{}{
		"name":                params.Name,
		"contacts_attributes": contactAttrs,
	}
	if params.DealStageID != "" {
		deal["deal_stage_id"] = params.DealStageID
	}
	if params.UserID != "" {
		deal["user_id"] = params.UserID
	}
	if len(params.Products) > 0 {
		products := make([]map[string]interface{}, 0, len(params.Products))
		for _, product := range params.Products {
			item := map[string]interface{}{
				"name":        product.Name,
				"description": product.Description,
				"amount":      product.Amount,
				"base_price":  product.BasePrice,
				"price":       product.Price,
			}
			products = append(products, item)
		}
		deal["deal_products"] = products
	}

	payload := map[string]interface{}{"deal": deal}

	data, err := c.do(ctx, http.MethodPost, "/deals", payload)
	if err != nil {
		return DealResponse{}, err
	}

	var result DealResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return DealResponse{}, fmt.Errorf("rdstation CreateDeal unmarshal: %w", err)
	}

	return result, nil
}

// UpdateDeal updates fields of an existing deal.
func (c *Client) UpdateDeal(ctx context.Context, dealID string, params UpdateDealParams) (DealResponse, error) {
	deal := map[string]interface{}{}
	if params.Name != "" {
		deal["name"] = params.Name
	}
	if params.DealStageID != "" {
		deal["deal_stage_id"] = params.DealStageID
	}
	if len(params.ContactIDs) > 0 {
		attrs := make([]map[string]string, len(params.ContactIDs))
		for i, id := range params.ContactIDs {
			attrs[i] = map[string]string{"_id": id}
		}
		deal["contacts_attributes"] = attrs
	}

	payload := map[string]interface{}{"deal": deal}

	data, err := c.do(ctx, http.MethodPut, "/deals/"+dealID, payload)
	if err != nil {
		return DealResponse{}, err
	}

	var result DealResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return DealResponse{}, fmt.Errorf("rdstation UpdateDeal unmarshal: %w", err)
	}

	return result, nil
}

// UpdateContact updates fields of an existing contact.
func (c *Client) UpdateContact(ctx context.Context, contactID string, params UpdateContactParams) (ContactResponse, error) {
	contact := map[string]interface{}{}
	if params.Name != "" {
		contact["name"] = params.Name
	}
	if params.Email != "" {
		contact["email"] = params.Email
	}
	if params.Phone != "" {
		contact["phone"] = params.Phone
	}

	payload := map[string]interface{}{"contact": contact}

	data, err := c.do(ctx, http.MethodPut, "/contacts/"+contactID, payload)
	if err != nil {
		return ContactResponse{}, err
	}

	var result ContactResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return ContactResponse{}, fmt.Errorf("rdstation UpdateContact unmarshal: %w", err)
	}

	return result, nil
}

// GetDealStages retrieves all pipeline stages from RD Station.
func (c *Client) GetDealStages(ctx context.Context) ([]struct {
	ID   string
	Name string
}, error) {
	data, err := c.do(ctx, http.MethodGet, "/deal_stages", nil)
	if err != nil {
		return nil, err
	}

	var result DealStageListResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("rdstation GetDealStages unmarshal: %w", err)
	}

	stages := make([]struct {
		ID   string
		Name string
	}, len(result.DealStages))

	for i, s := range result.DealStages {
		stages[i].ID = s.ID
		stages[i].Name = s.Name
	}

	return stages, nil
}

// GetDealByID retrieves a single deal by its ID.
func (c *Client) GetDealByID(ctx context.Context, dealID string) (DealResponse, error) {
	data, err := c.do(ctx, http.MethodGet, "/deals/"+dealID, nil)
	if err != nil {
		return DealResponse{}, err
	}

	var result DealResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return DealResponse{}, fmt.Errorf("rdstation GetDealByID unmarshal: %w", err)
	}

	return result, nil
}

// GetDealContacts retrieves all contacts linked to a deal.
func (c *Client) GetDealContacts(ctx context.Context, dealID string) ([]DealContactResponse, error) {
	data, err := c.do(ctx, http.MethodGet, "/deals/"+dealID+"/contacts", nil)
	if err != nil {
		return nil, err
	}

	var result DealContactsListResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("rdstation GetDealContacts unmarshal: %w", err)
	}

	return result.Contacts, nil
}

// GetActivities returns the manual annotations registered in a deal.
func (c *Client) GetActivities(ctx context.Context, dealID string) ([]ActivityResponse, error) {
	q := url.Values{}
	q.Set("deal_id", dealID)
	q.Set("limit", "20")

	data, err := c.do(ctx, http.MethodGet, "/activities?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var result ActivitiesListResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("rdstation GetActivities unmarshal: %w", err)
	}

	return result.Activities, nil
}

// CreateActivity creates a manual annotation in a deal.
func (c *Client) CreateActivity(ctx context.Context, dealID, userID, text string) (ActivityResponse, error) {
	payload := map[string]interface{}{
		"activity": map[string]interface{}{
			"deal_id": dealID,
			"user_id": userID,
			"text":    text,
		},
	}

	data, err := c.do(ctx, http.MethodPost, "/activities", payload)
	if err != nil {
		return ActivityResponse{}, err
	}

	var result ActivityResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return ActivityResponse{}, fmt.Errorf("rdstation CreateActivity unmarshal: %w", err)
	}

	return result, nil
}

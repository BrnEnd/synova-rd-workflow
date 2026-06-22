package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultBaseURL = "https://graph.facebook.com/v20.0"
const maxDownloadedMediaBytes = 25 * 1024 * 1024

// Client sends WhatsApp Cloud API messages through Meta Graph API.
type Client struct {
	accessToken   string
	phoneNumberID string
	baseURL       string
	httpClient    *http.Client
}

// New returns a WhatsApp Cloud API client.
func New(accessToken, phoneNumberID string) *Client {
	return NewWithBaseURL(accessToken, phoneNumberID, defaultBaseURL)
}

// NewWithBaseURL returns a client with a custom base URL for tests.
func NewWithBaseURL(accessToken, phoneNumberID, baseURL string) *Client {
	return &Client{
		accessToken:   accessToken,
		phoneNumberID: phoneNumberID,
		baseURL:       strings.TrimRight(baseURL, "/"),
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}
}

// SendTextMessage sends a plain text reply. Meta caps text bodies at 4096 chars.
func (c *Client) SendTextMessage(ctx context.Context, to string, text string) error {
	if len(text) > 4096 {
		text = text[:4096]
	}

	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                strings.TrimPrefix(to, "+"),
		"type":              "text",
		"text": map[string]string{
			"body": text,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("whatsapp marshal: %w", err)
	}

	url := fmt.Sprintf("%s/%s/messages", c.baseURL, c.phoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("whatsapp new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("whatsapp http do: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("whatsapp read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("whatsapp status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

type mediaURLResponse struct {
	URL      string `json:"url"`
	MimeType string `json:"mime_type"`
}

// DownloadMedia retrieves an inbound WhatsApp media asset from Meta.
func (c *Client) DownloadMedia(ctx context.Context, mediaID string) ([]byte, string, error) {
	if strings.TrimSpace(mediaID) == "" {
		return nil, "", fmt.Errorf("whatsapp media id empty")
	}

	url := fmt.Sprintf("%s/%s", c.baseURL, mediaID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("whatsapp media url request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("whatsapp media url http do: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("whatsapp media url read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("whatsapp media url status %d: %s", resp.StatusCode, string(body))
	}

	var meta mediaURLResponse
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, "", fmt.Errorf("whatsapp media url unmarshal: %w", err)
	}
	if meta.URL == "" {
		return nil, "", fmt.Errorf("whatsapp media url empty")
	}

	downloadReq, err := http.NewRequestWithContext(ctx, http.MethodGet, meta.URL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("whatsapp media download request: %w", err)
	}
	downloadReq.Header.Set("Authorization", "Bearer "+c.accessToken)

	downloadResp, err := c.httpClient.Do(downloadReq)
	if err != nil {
		return nil, "", fmt.Errorf("whatsapp media download http do: %w", err)
	}
	defer downloadResp.Body.Close()

	downloaded, err := io.ReadAll(io.LimitReader(downloadResp.Body, maxDownloadedMediaBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("whatsapp media download read body: %w", err)
	}
	if downloadResp.StatusCode < 200 || downloadResp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("whatsapp media download status %d: %s", downloadResp.StatusCode, string(downloaded))
	}
	if len(downloaded) > maxDownloadedMediaBytes {
		return nil, "", fmt.Errorf("whatsapp media download too large")
	}

	mimeType := meta.MimeType
	if mimeType == "" {
		mimeType = downloadResp.Header.Get("Content-Type")
	}

	return downloaded, mimeType, nil
}

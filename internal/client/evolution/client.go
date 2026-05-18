package evolution

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

// Client wraps the Evolution API HTTP endpoints used by the bot.
type Client struct {
	baseURL    string
	apiKey     string
	instance   string
	httpClient *http.Client
	sendDelay  time.Duration
}

func New(baseURL, apiKey, instance string) *Client {
	return NewWithSendDelay(baseURL, apiKey, instance, 0)
}

func NewWithSendDelay(baseURL, apiKey, instance string, sendDelay time.Duration) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		instance:   instance,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		sendDelay:  sendDelay,
	}
}

func (c *Client) SendTextMessage(ctx context.Context, to string, text string) error {
	if c.sendDelay > 0 {
		timer := time.NewTimer(c.sendDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	payload := map[string]interface{}{
		"number":      normalizeNumber(to),
		"text":        text,
		"linkPreview": false,
	}
	return c.postWithRetry(ctx, fmt.Sprintf("/message/sendText/%s", c.instance), payload)
}

func (c *Client) AddToAllowlist(ctx context.Context, phone string) error {
	payload := map[string]interface{}{"number": normalizeNumber(phone)}
	return c.post(ctx, fmt.Sprintf("/instance/%s/whitelist", c.instance), payload)
}

func (c *Client) RemoveFromAllowlist(ctx context.Context, phone string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+fmt.Sprintf("/instance/%s/whitelist/%s", c.instance, normalizeNumber(phone)), nil)
	if err != nil {
		return fmt.Errorf("evolution new request: %w", err)
	}
	req.Header.Set("apikey", c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("evolution http do: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("evolution read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("evolution status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (c *Client) SetWebhook(ctx context.Context, webhookURL string) error {
	payload := map[string]interface{}{
		"enabled":           true,
		"url":               webhookURL,
		"webhook_by_events": false,
		"webhook_base64":    false,
		"events": []string{
			"MESSAGES_UPSERT",
			"CONNECTION_UPDATE",
			"QRCODE_UPDATED",
			"SEND_MESSAGE",
		},
	}
	return c.post(ctx, "/webhook/instance", payload)
}

func (c *Client) post(ctx context.Context, path string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("evolution marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("evolution new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("evolution http do: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("evolution read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("evolution status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *Client) postWithRetry(ctx context.Context, path string, payload interface{}) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		lastErr = c.post(ctx, path, payload)
		if lastErr == nil {
			return nil
		}
		if !isTransientSendError(lastErr) || attempt == 3 {
			return lastErr
		}
		timer := time.NewTimer(time.Duration(attempt) * 750 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func isTransientSendError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "Connection Closed") ||
		strings.Contains(msg, "\"1006\"") ||
		strings.Contains(msg, "status 502") ||
		strings.Contains(msg, "status 503") ||
		strings.Contains(msg, "status 504")
}

func normalizeNumber(number string) string {
	number = strings.TrimSpace(number)
	number = strings.TrimPrefix(number, "+")
	number = strings.TrimSuffix(number, "@s.whatsapp.net")
	number = strings.TrimSuffix(number, "@c.us")
	number = strings.TrimSuffix(number, "@lid")
	return number
}

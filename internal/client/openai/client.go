package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"time"
)

const baseURL = "https://api.openai.com/v1"

// --- Chat types ---

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type ChatChoice struct {
	Message struct {
		Role      string     `json:"role"`
		Content   *string    `json:"content"`
		ToolCalls []ToolCall `json:"tool_calls"`
	} `json:"message"`
	FinishReason string `json:"finish_reason"`
}

type ChatCompletionRequest struct {
	Model      string        `json:"model"`
	Messages   []ChatMessage `json:"messages"`
	Tools      []Tool        `json:"tools,omitempty"`
	ToolChoice interface{}   `json:"tool_choice,omitempty"`
}

type ChatCompletionResponse struct {
	Choices []ChatChoice `json:"choices"`
}

type transcriptionResponse struct {
	Text string `json:"text"`
}

// --- Tool / Function calling types ---

type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type Function struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// --- Client ---

// Client wraps the OpenAI Chat Completions API.
type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// New returns a configured OpenAI client.
func New(apiKey, model string) *Client {
	return &Client{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// ChatCompletion sends a chat completion request with optional function calling tools.
func (c *Client) ChatCompletion(ctx context.Context, req ChatCompletionRequest) (ChatCompletionResponse, error) {
	req.Model = c.model

	data, err := json.Marshal(req)
	if err != nil {
		return ChatCompletionResponse{}, fmt.Errorf("openai marshal: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(data))
		if err != nil {
			return ChatCompletionResponse{}, fmt.Errorf("openai new request: %w", err)
		}

		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("openai http do: %w", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("openai read body: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("openai status %d: %s", resp.StatusCode, string(body))
			continue
		}

		var result ChatCompletionResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return ChatCompletionResponse{}, fmt.Errorf("openai unmarshal: %w", err)
		}

		return result, nil
	}

	return ChatCompletionResponse{}, lastErr
}

func (c *Client) TranscribeAudio(ctx context.Context, audio []byte, filename string) (string, error) {
	if len(audio) == 0 {
		return "", fmt.Errorf("openai transcription: empty audio")
	}
	if filename == "" {
		filename = "audio.ogg"
	}
	if filepath.Ext(filename) == "" {
		filename += ".ogg"
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "whisper-1"); err != nil {
		return "", fmt.Errorf("openai transcription model field: %w", err)
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("openai transcription file field: %w", err)
	}
	if _, err := part.Write(audio); err != nil {
		return "", fmt.Errorf("openai transcription write audio: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("openai transcription close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/audio/transcriptions", &body)
	if err != nil {
		return "", fmt.Errorf("openai transcription new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai transcription http do: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("openai transcription read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai transcription status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed transcriptionResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("openai transcription unmarshal: %w", err)
	}
	return parsed.Text, nil
}

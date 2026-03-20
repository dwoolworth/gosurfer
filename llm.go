package gosurfer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LLMProvider defines the interface for language model backends.
type LLMProvider interface {
	// ChatCompletion sends messages to the LLM and returns a response.
	ChatCompletion(ctx context.Context, messages []ChatMessage, opts ...ChatOption) (*ChatResponse, error)
	// Name returns the provider/model name.
	Name() string
}

// ChatMessage represents a message in the conversation.
type ChatMessage struct {
	Role    string        `json:"role"` // "system", "user", "assistant"
	Content []ContentPart `json:"-"`
}

// ContentPart is a piece of a message (text or image).
type ContentPart struct {
	Type     string `json:"type"` // "text" or "image"
	Text     string `json:"text,omitempty"`
	ImageB64 string `json:"image_b64,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// TextMessage creates a simple text message.
func TextMessage(role, text string) ChatMessage {
	return ChatMessage{
		Role:    role,
		Content: []ContentPart{{Type: "text", Text: text}},
	}
}

// ImageMessage creates a message with text and an image.
func ImageMessage(role, text string, imageData []byte, mimeType string) ChatMessage {
	return ChatMessage{
		Role: role,
		Content: []ContentPart{
			{Type: "text", Text: text},
			{Type: "image", ImageB64: base64.StdEncoding.EncodeToString(imageData), MimeType: mimeType},
		},
	}
}

// ChatResponse is the LLM's response.
type ChatResponse struct {
	Content string     `json:"content"`
	Usage   TokenUsage `json:"usage"`
}

// TokenUsage tracks token consumption.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatOption configures a chat completion request.
type ChatOption func(*chatConfig)

type chatConfig struct {
	MaxTokens   int
	Temperature float64
	JSONMode    bool
}

// WithMaxTokens sets the maximum response tokens.
func WithMaxTokens(n int) ChatOption {
	return func(c *chatConfig) { c.MaxTokens = n }
}

// WithTemperature sets the sampling temperature.
func WithTemperature(t float64) ChatOption {
	return func(c *chatConfig) { c.Temperature = t }
}

// WithJSONMode requests JSON output from the model.
func WithJSONMode() ChatOption {
	return func(c *chatConfig) { c.JSONMode = true }
}

// --- OpenAI Provider ---

// OpenAIProvider implements LLMProvider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewOpenAI creates an OpenAI provider.
func NewOpenAI(apiKey, model string) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://api.openai.com/v1",
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

// NewOpenAICompatible creates a provider for OpenAI-compatible APIs (e.g., Ollama, vLLM).
func NewOpenAICompatible(baseURL, apiKey, model string) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OpenAIProvider) Name() string { return "openai/" + p.model }

func (p *OpenAIProvider) ChatCompletion(ctx context.Context, messages []ChatMessage, opts ...ChatOption) (*ChatResponse, error) {
	cfg := &chatConfig{MaxTokens: 4096, Temperature: 0.0}
	for _, opt := range opts {
		opt(cfg)
	}

	// Build request body
	reqBody := map[string]interface{}{
		"model":       p.model,
		"messages":    p.formatMessages(messages),
		"max_tokens":  cfg.MaxTokens,
		"temperature": cfg.Temperature,
	}
	if cfg.JSONMode {
		reqBody["response_format"] = map[string]string{"type": "json_object"}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openai error %d: %s", resp.StatusCode, string(respBody))
	}

	var result openAIResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices in response")
	}

	return &ChatResponse{
		Content: result.Choices[0].Message.Content,
		Usage: TokenUsage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		},
	}, nil
}

func (p *OpenAIProvider) formatMessages(messages []ChatMessage) []interface{} {
	result := make([]interface{}, 0, len(messages))
	for _, msg := range messages {
		if len(msg.Content) == 1 && msg.Content[0].Type == "text" {
			result = append(result, map[string]string{
				"role":    msg.Role,
				"content": msg.Content[0].Text,
			})
		} else {
			parts := make([]interface{}, 0, len(msg.Content))
			for _, part := range msg.Content {
				switch part.Type {
				case "text":
					parts = append(parts, map[string]string{
						"type": "text",
						"text": part.Text,
					})
				case "image":
					mime := part.MimeType
					if mime == "" {
						mime = "image/jpeg"
					}
					parts = append(parts, map[string]interface{}{
						"type": "image_url",
						"image_url": map[string]string{
							"url":    "data:" + mime + ";base64," + part.ImageB64,
							"detail": "low",
						},
					})
				}
			}
			result = append(result, map[string]interface{}{
				"role":    msg.Role,
				"content": parts,
			})
		}
	}
	return result
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// --- Anthropic Provider ---

// AnthropicProvider implements LLMProvider for the Anthropic Messages API.
type AnthropicProvider struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewAnthropic creates an Anthropic provider.
func NewAnthropic(apiKey, model string) *AnthropicProvider {
	return &AnthropicProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://api.anthropic.com/v1",
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *AnthropicProvider) Name() string { return "anthropic/" + p.model }

func (p *AnthropicProvider) ChatCompletion(ctx context.Context, messages []ChatMessage, opts ...ChatOption) (*ChatResponse, error) {
	cfg := &chatConfig{MaxTokens: 4096, Temperature: 0.0}
	for _, opt := range opts {
		opt(cfg)
	}

	// Separate system message
	var systemText string
	var userMsgs []ChatMessage
	for _, msg := range messages {
		if msg.Role == "system" {
			for _, part := range msg.Content {
				if part.Type == "text" {
					systemText += part.Text
				}
			}
		} else {
			userMsgs = append(userMsgs, msg)
		}
	}

	reqBody := map[string]interface{}{
		"model":       p.model,
		"max_tokens":  cfg.MaxTokens,
		"temperature": cfg.Temperature,
		"messages":    p.formatMessages(userMsgs),
	}
	if systemText != "" {
		reqBody["system"] = systemText
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("anthropic error %d: %s", resp.StatusCode, string(respBody))
	}

	var result anthropicResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	var content string
	for _, block := range result.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	return &ChatResponse{
		Content: content,
		Usage: TokenUsage{
			PromptTokens:     result.Usage.InputTokens,
			CompletionTokens: result.Usage.OutputTokens,
			TotalTokens:      result.Usage.InputTokens + result.Usage.OutputTokens,
		},
	}, nil
}

func (p *AnthropicProvider) formatMessages(messages []ChatMessage) []interface{} {
	result := make([]interface{}, 0, len(messages))
	for _, msg := range messages {
		if len(msg.Content) == 1 && msg.Content[0].Type == "text" {
			result = append(result, map[string]interface{}{
				"role": msg.Role,
				"content": []map[string]string{
					{"type": "text", "text": msg.Content[0].Text},
				},
			})
		} else {
			parts := make([]interface{}, 0, len(msg.Content))
			for _, part := range msg.Content {
				switch part.Type {
				case "text":
					parts = append(parts, map[string]string{
						"type": "text",
						"text": part.Text,
					})
				case "image":
					mime := part.MimeType
					if mime == "" {
						mime = "image/jpeg"
					}
					parts = append(parts, map[string]interface{}{
						"type": "image",
						"source": map[string]string{
							"type":       "base64",
							"media_type": mime,
							"data":       part.ImageB64,
						},
					})
				}
			}
			result = append(result, map[string]interface{}{
				"role":    msg.Role,
				"content": parts,
			})
		}
	}
	return result
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// --- Ollama Provider ---

// NewOllama creates an Ollama provider (OpenAI-compatible local inference).
func NewOllama(model string) *OpenAIProvider {
	return NewOpenAICompatible("http://localhost:11434/v1", "", model)
}

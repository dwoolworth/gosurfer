package gosurfer

import (
	"encoding/base64"
	"testing"
)

func TestTextMessage(t *testing.T) {
	msg := TextMessage("user", "hello")
	if msg.Role != "user" {
		t.Errorf("role = %q", msg.Role)
	}
	if len(msg.Content) != 1 {
		t.Fatal("expected 1 content part")
	}
	if msg.Content[0].Type != "text" {
		t.Errorf("type = %q", msg.Content[0].Type)
	}
	if msg.Content[0].Text != "hello" {
		t.Errorf("text = %q", msg.Content[0].Text)
	}
}

func TestImageMessage(t *testing.T) {
	imageData := []byte{0x89, 0x50, 0x4E, 0x47} // PNG header
	msg := ImageMessage("user", "describe this", imageData, "image/png")
	if msg.Role != "user" {
		t.Errorf("role = %q", msg.Role)
	}
	if len(msg.Content) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(msg.Content))
	}
	if msg.Content[0].Type != "text" || msg.Content[0].Text != "describe this" {
		t.Error("first part should be text")
	}
	if msg.Content[1].Type != "image" {
		t.Error("second part should be image")
	}
	if msg.Content[1].MimeType != "image/png" {
		t.Errorf("mime = %q", msg.Content[1].MimeType)
	}
	// Verify base64
	decoded, err := base64.StdEncoding.DecodeString(msg.Content[1].ImageB64)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 4 {
		t.Errorf("decoded length = %d", len(decoded))
	}
}

func TestChatOptions(t *testing.T) {
	cfg := &chatConfig{}

	WithMaxTokens(8192)(cfg)
	if cfg.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d", cfg.MaxTokens)
	}

	WithTemperature(0.7)(cfg)
	if cfg.Temperature != 0.7 {
		t.Errorf("Temperature = %f", cfg.Temperature)
	}

	WithJSONMode()(cfg)
	if !cfg.JSONMode {
		t.Error("JSONMode should be true")
	}
}

func TestOpenAI_FormatMessages_TextOnly(t *testing.T) {
	p := NewOpenAI("key", "model")
	messages := []ChatMessage{
		TextMessage("system", "be helpful"),
		TextMessage("user", "hello"),
	}
	formatted := p.formatMessages(messages)
	if len(formatted) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(formatted))
	}
	// Text-only messages should use simple format
	msg, ok := formatted[0].(map[string]string)
	if !ok {
		t.Fatal("text message should be map[string]string")
	}
	if msg["role"] != "system" {
		t.Errorf("role = %q", msg["role"])
	}
}

func TestOpenAI_FormatMessages_WithImage(t *testing.T) {
	p := NewOpenAI("key", "model")
	messages := []ChatMessage{
		ImageMessage("user", "describe", []byte{1, 2, 3}, "image/jpeg"),
	}
	formatted := p.formatMessages(messages)
	if len(formatted) != 1 {
		t.Fatalf("expected 1 message, got %d", len(formatted))
	}
	// Image message should use multipart format
	msg, ok := formatted[0].(map[string]interface{})
	if !ok {
		t.Fatal("image message should be map[string]interface{}")
	}
	content, ok := msg["content"].([]interface{})
	if !ok {
		t.Fatal("content should be array")
	}
	if len(content) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(content))
	}
}

func TestAnthropic_FormatMessages(t *testing.T) {
	p := NewAnthropic("key", "model")
	// formatMessages only handles non-system messages (system is extracted in ChatCompletion)
	messages := []ChatMessage{
		TextMessage("user", "hello"),
		TextMessage("assistant", "hi there"),
	}
	formatted := p.formatMessages(messages)
	if len(formatted) != 2 {
		t.Fatalf("expected 2 formatted messages, got %d", len(formatted))
	}
}

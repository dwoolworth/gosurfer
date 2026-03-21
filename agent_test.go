package gosurfer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseResponse_ValidJSON(t *testing.T) {
	a := &Agent{}
	thought, action, params, err := a.parseResponse(`{"thought":"click the button","action":"click","params":{"index":3}}`)
	if err != nil {
		t.Fatal(err)
	}
	if thought != "click the button" {
		t.Errorf("thought = %q", thought)
	}
	if action != "click" {
		t.Errorf("action = %q", action)
	}
	idx, _ := toInt(params["index"])
	if idx != 3 {
		t.Errorf("index = %v", params["index"])
	}
}

func TestParseResponse_MarkdownFenced(t *testing.T) {
	a := &Agent{}
	input := "```json\n{\"thought\":\"test\",\"action\":\"done\",\"params\":{\"output\":\"result\"}}\n```"
	_, action, params, err := a.parseResponse(input)
	if err != nil {
		t.Fatal(err)
	}
	if action != "done" {
		t.Errorf("action = %q", action)
	}
	if params["output"] != "result" {
		t.Errorf("output = %v", params["output"])
	}
}

func TestParseResponse_JSONInText(t *testing.T) {
	a := &Agent{}
	input := `Here's what I'll do: {"thought":"nav","action":"navigate","params":{"url":"https://example.com"}} That should work.`
	_, action, params, err := a.parseResponse(input)
	if err != nil {
		t.Fatal(err)
	}
	if action != "navigate" {
		t.Errorf("action = %q", action)
	}
	if params["url"] != "https://example.com" {
		t.Errorf("url = %v", params["url"])
	}
}

func TestParseResponse_NoJSON(t *testing.T) {
	a := &Agent{}
	_, _, _, err := a.parseResponse("I don't know what to do")
	if err == nil {
		t.Error("expected error for non-JSON response")
	}
}

func TestParseResponse_NoAction(t *testing.T) {
	a := &Agent{}
	_, _, _, err := a.parseResponse(`{"thought":"thinking","params":{}}`)
	if err == nil {
		t.Error("expected error for missing action")
	}
}

func TestParseResponse_NilParams(t *testing.T) {
	a := &Agent{}
	_, _, params, err := a.parseResponse(`{"thought":"t","action":"go_back"}`)
	if err != nil {
		t.Fatal(err)
	}
	if params == nil {
		t.Error("params should be initialized to empty map")
	}
}

func TestIsLooping_NoLoop(t *testing.T) {
	a := &Agent{recentActions: []string{"click:1", "type:2", "click:3", "scroll:up", "click:4", "done"}}
	if a.isLooping() {
		t.Error("should not detect loop with different actions")
	}
}

func TestIsLooping_Detected(t *testing.T) {
	a := &Agent{recentActions: []string{
		"click:5", "scroll:down", "click:5",
		"click:5", "scroll:down", "click:5",
	}}
	if !a.isLooping() {
		t.Error("should detect repeating 3-action pattern")
	}
}

func TestIsLooping_TooFewActions(t *testing.T) {
	a := &Agent{recentActions: []string{"click:1", "click:1"}}
	if a.isLooping() {
		t.Error("should not detect loop with fewer than 6 actions")
	}
}

func TestTrackAction(t *testing.T) {
	a := &Agent{}
	for i := 0; i < 25; i++ {
		a.trackAction("click", map[string]interface{}{"index": i})
	}
	if len(a.recentActions) != 20 {
		t.Errorf("should cap at 20 recent actions, got %d", len(a.recentActions))
	}
}

func TestNewAgent_Validation(t *testing.T) {
	_, err := NewAgent(AgentConfig{})
	if err == nil {
		t.Error("expected error for missing LLM")
	}

	_, err = NewAgent(AgentConfig{LLM: &mockLLM{}})
	if err == nil {
		t.Error("expected error for missing task")
	}

	a, err := NewAgent(AgentConfig{LLM: &mockLLM{}, Task: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if a.config.MaxSteps != 50 {
		t.Errorf("default MaxSteps should be 50, got %d", a.config.MaxSteps)
	}
	if a.config.MaxFailures != 5 {
		t.Errorf("default MaxFailures should be 5, got %d", a.config.MaxFailures)
	}
}

func TestNewAgent_WithSecrets(t *testing.T) {
	a, err := NewAgent(AgentConfig{
		LLM:     &mockLLM{},
		Task:    "test",
		Secrets: map[string]string{"user": "admin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.secrets == nil {
		t.Error("secrets should be initialized")
	}
	val, _ := a.secrets.Get("user")
	if val != "admin" {
		t.Errorf("expected admin, got %s", val)
	}
}

func TestBuildMessages_Structure(t *testing.T) {
	a := &Agent{
		config: AgentConfig{
			Task:      "find info",
			LLM:       &mockLLM{},
			MaxSteps:  10,
			MaxTokens: 4096,
		},
		actions: DefaultActions(),
	}

	state := &DOMState{
		URL:   "https://example.com",
		Title: "Example",
		Tree:  "[0]<a>Link</a>",
	}

	messages := a.buildMessages(state, 1)

	// First message should be system prompt
	if messages[0].Role != "system" {
		t.Error("first message should be system role")
	}

	// Last message should contain current state
	last := messages[len(messages)-1]
	if last.Role != "user" {
		t.Error("last message should be user role")
	}
	if !strings.Contains(last.Content[0].Text, "https://example.com") {
		t.Error("last message should contain current URL")
	}
	if !strings.Contains(last.Content[0].Text, "[0]<a>Link</a>") {
		t.Error("last message should contain DOM tree")
	}
}

func TestBuildMessages_WithTabs(t *testing.T) {
	a := &Agent{
		config:  AgentConfig{Task: "test", LLM: &mockLLM{}, MaxSteps: 10, MaxTokens: 4096},
		actions: DefaultActions(),
	}

	state := &DOMState{
		URL:   "https://example.com",
		Title: "Example",
		Tabs: []TabInfo{
			{ID: "ab12", URL: "https://example.com", Title: "Example"},
			{ID: "cd34", URL: "https://other.com", Title: "Other"},
		},
	}

	messages := a.buildMessages(state, 1)
	last := messages[len(messages)-1].Content[0].Text
	if !strings.Contains(last, "Open Tabs") {
		t.Error("should show tab list when multiple tabs exist")
	}
	if !strings.Contains(last, "ab12") || !strings.Contains(last, "cd34") {
		t.Error("should show tab IDs")
	}
}

func TestBuildMessages_LoopWarning(t *testing.T) {
	a := &Agent{
		config:  AgentConfig{Task: "test", LLM: &mockLLM{}, MaxSteps: 10, MaxTokens: 4096},
		actions: DefaultActions(),
		recentActions: []string{
			"click:5", "scroll:down", "click:5",
			"click:5", "scroll:down", "click:5",
		},
	}

	state := &DOMState{URL: "https://example.com", Title: "Example"}
	messages := a.buildMessages(state, 7)
	last := messages[len(messages)-1].Content[0].Text
	if !strings.Contains(last, "WARNING") || !strings.Contains(last, "loop") {
		t.Error("should include loop warning when stuck")
	}
}

// --- Mock LLM ---

type mockLLM struct {
	response string
}

func (m *mockLLM) Name() string { return "mock" }

func (m *mockLLM) ChatCompletion(_ context.Context, _ []ChatMessage, _ ...ChatOption) (*ChatResponse, error) {
	resp := m.response
	if resp == "" {
		resp = `{"thought":"done","action":"done","params":{"output":"mock result","success":true}}`
	}
	return &ChatResponse{Content: resp}, nil
}

// --- LLM Provider Tests with Mock HTTP ---

func TestOpenAI_ChatCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing content-type")
		}

		resp := openAIResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{{Message: struct {
				Content string `json:"content"`
			}{Content: "hello"}}},
			Usage: struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			}{10, 5, 15},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOpenAICompatible(server.URL, "test-key", "gpt-test")
	if provider.Name() != "openai/gpt-test" {
		t.Errorf("name = %q", provider.Name())
	}

	resp, err := provider.ChatCompletion(context.Background(), []ChatMessage{
		TextMessage("user", "hi"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "hello" {
		t.Errorf("content = %q", resp.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("tokens = %d", resp.Usage.TotalTokens)
	}
}

func TestAnthropic_ChatCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("missing x-api-key header")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Error("missing anthropic-version header")
		}

		resp := anthropicResponse{
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "text", Text: "world"}},
			Usage: struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			}{8, 3},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewAnthropic("test-key", "claude-test")
	provider.baseURL = server.URL
	if provider.Name() != "anthropic/claude-test" {
		t.Errorf("name = %q", provider.Name())
	}

	resp, err := provider.ChatCompletion(context.Background(), []ChatMessage{
		TextMessage("system", "be helpful"),
		TextMessage("user", "hi"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "world" {
		t.Errorf("content = %q", resp.Content)
	}
	if resp.Usage.TotalTokens != 11 {
		t.Errorf("tokens = %d", resp.Usage.TotalTokens)
	}
}

func TestOpenAI_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = fmt.Fprint(w, `{"error":"internal"}`)
	}))
	defer server.Close()

	provider := NewOpenAICompatible(server.URL, "key", "model")
	_, err := provider.ChatCompletion(context.Background(), []ChatMessage{TextMessage("user", "hi")})
	if err == nil {
		t.Error("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status code: %v", err)
	}
}

func TestNewOllama(t *testing.T) {
	p := NewOllama("llama3")
	if p.Name() != "openai/llama3" {
		t.Errorf("name = %q", p.Name())
	}
	if !strings.Contains(p.baseURL, "11434") {
		t.Error("should use default Ollama port")
	}
}

// --- recordingMockLLM records calls for assertion ---

type recordingMockLLM struct {
	response string
	err      error
	calls    [][]ChatMessage
}

func (m *recordingMockLLM) Name() string { return "recording-mock" }

func (m *recordingMockLLM) ChatCompletion(_ context.Context, msgs []ChatMessage, _ ...ChatOption) (*ChatResponse, error) {
	m.calls = append(m.calls, msgs)
	if m.err != nil {
		return nil, m.err
	}
	return &ChatResponse{
		Content: m.response,
		Usage:   TokenUsage{PromptTokens: 50, CompletionTokens: 30, TotalTokens: 80},
	}, nil
}

// --- Context Summarization Tests ---

func TestSummarizeIfNeeded_NotTriggeredUnder5Steps(t *testing.T) {
	mock := &recordingMockLLM{response: "summary"}
	a := &Agent{
		config:  AgentConfig{Task: "test", LLM: mock, MaxSteps: 50},
		actions: DefaultActions(),
		history: makeHistory(4), // only 4 steps, all fit in window
	}

	a.summarizeIfNeeded(context.Background())

	if len(mock.calls) != 0 {
		t.Error("should not call LLM when all steps fit in window")
	}
	if a.contextSummary != "" {
		t.Error("summary should remain empty")
	}
}

func TestSummarizeIfNeeded_TriggeredWhenBatchReached(t *testing.T) {
	mock := &recordingMockLLM{response: "Navigated to example.com and clicked search."}
	a := &Agent{
		config:  AgentConfig{Task: "test", LLM: mock, MaxSteps: 50},
		actions: DefaultActions(),
		history: makeHistory(8), // 3 steps outside the 5-step window
	}

	a.summarizeIfNeeded(context.Background())

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 LLM call, got %d", len(mock.calls))
	}
	if a.contextSummary != "Navigated to example.com and clicked search." {
		t.Errorf("summary = %q", a.contextSummary)
	}
	if a.summarizedUpTo != 3 {
		t.Errorf("summarizedUpTo = %d, want 3", a.summarizedUpTo)
	}
	// Token tracking
	if a.tokens.TotalTokens != 80 {
		t.Errorf("tokens = %d, want 80", a.tokens.TotalTokens)
	}
}

func TestSummarizeIfNeeded_IncrementalUpdate(t *testing.T) {
	mock := &recordingMockLLM{response: "Updated: navigated, searched, and extracted data."}
	a := &Agent{
		config:         AgentConfig{Task: "test", LLM: mock, MaxSteps: 50},
		actions:        DefaultActions(),
		history:        makeHistory(11), // 6 outside window
		contextSummary: "Previous: navigated to example.com.",
		summarizedUpTo: 3, // already summarized first 3
	}

	a.summarizeIfNeeded(context.Background())

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 LLM call, got %d", len(mock.calls))
	}

	// The prompt should include the previous summary
	prompt := mock.calls[0][1].Content[0].Text // user message
	if !strings.Contains(prompt, "Previous: navigated to example.com.") {
		t.Error("prompt should include previous summary")
	}
	// Should include steps 4-6 (indices 3-5)
	if !strings.Contains(prompt, "Step 4") {
		t.Error("prompt should include new unsummarized steps")
	}

	if a.contextSummary != "Updated: navigated, searched, and extracted data." {
		t.Errorf("summary = %q", a.contextSummary)
	}
	if a.summarizedUpTo != 6 {
		t.Errorf("summarizedUpTo = %d, want 6", a.summarizedUpTo)
	}
}

func TestSummarizeIfNeeded_DisabledByConfig(t *testing.T) {
	mock := &recordingMockLLM{response: "summary"}
	a := &Agent{
		config:  AgentConfig{Task: "test", LLM: mock, MaxSteps: 50, DisableSummary: true},
		actions: DefaultActions(),
		history: makeHistory(10),
	}

	a.summarizeIfNeeded(context.Background())

	if len(mock.calls) != 0 {
		t.Error("should not call LLM when summarization is disabled")
	}
}

func TestSummarizeIfNeeded_LLMErrorNonFatal(t *testing.T) {
	mock := &recordingMockLLM{err: fmt.Errorf("API error")}
	a := &Agent{
		config:  AgentConfig{Task: "test", LLM: mock, MaxSteps: 50},
		actions: DefaultActions(),
		history: makeHistory(8),
	}

	// Should not panic or return error
	a.summarizeIfNeeded(context.Background())

	if a.contextSummary != "" {
		t.Error("summary should remain empty on error")
	}
	if a.summarizedUpTo != 0 {
		t.Error("summarizedUpTo should not advance on error")
	}
}

func TestSummarizeIfNeeded_SkipsWhenBelowBatchThreshold(t *testing.T) {
	mock := &recordingMockLLM{response: "summary"}
	a := &Agent{
		config:         AgentConfig{Task: "test", LLM: mock, MaxSteps: 50},
		actions:        DefaultActions(),
		history:        makeHistory(7), // 2 outside window, below batch size of 3
		contextSummary: "existing summary",
		summarizedUpTo: 1, // 1 already summarized, only 1 new unsummarized
	}

	a.summarizeIfNeeded(context.Background())

	if len(mock.calls) != 0 {
		t.Error("should not call LLM when unsummarized count is below batch threshold")
	}
}

func TestBuildMessages_IncludesSummary(t *testing.T) {
	a := &Agent{
		config:         AgentConfig{Task: "test", LLM: &mockLLM{}, MaxSteps: 10},
		actions:        DefaultActions(),
		contextSummary: "Visited example.com, searched for Go, found 3 results.",
	}

	state := &DOMState{URL: "https://example.com", Title: "Example", Tree: "[0]<a>Link</a>"}
	messages := a.buildMessages(state, 1)

	systemMsg := messages[0].Content[0].Text
	if !strings.Contains(systemMsg, "Context from Earlier Steps") {
		t.Error("system prompt should contain summary header")
	}
	if !strings.Contains(systemMsg, "Visited example.com, searched for Go") {
		t.Error("system prompt should contain the summary text")
	}
}

func TestBuildMessages_NoSummaryWhenEmpty(t *testing.T) {
	a := &Agent{
		config:  AgentConfig{Task: "test", LLM: &mockLLM{}, MaxSteps: 10},
		actions: DefaultActions(),
	}

	state := &DOMState{URL: "https://example.com", Title: "Example", Tree: "[0]<a>Link</a>"}
	messages := a.buildMessages(state, 1)

	systemMsg := messages[0].Content[0].Text
	if strings.Contains(systemMsg, "Context from Earlier Steps") {
		t.Error("system prompt should not contain summary header when no summary exists")
	}
}

// makeHistory creates N fake step history entries for testing.
func makeHistory(n int) []StepInfo {
	history := make([]StepInfo, n)
	actions := []string{"navigate", "click", "type", "scroll", "extract", "search"}
	for i := range history {
		history[i] = StepInfo{
			Step:   i + 1,
			Action: actions[i%len(actions)],
			Result: fmt.Sprintf("Result of step %d", i+1),
		}
	}
	return history
}

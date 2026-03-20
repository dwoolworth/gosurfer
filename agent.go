package gosurfer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// AgentConfig configures the AI browsing agent.
type AgentConfig struct {
	// Task is the natural language description of what to accomplish.
	Task string

	// LLM is the language model provider for decision-making.
	LLM LLMProvider

	// Browser is an existing browser instance. If nil, a new headless one is created.
	Browser *Browser

	// MaxSteps is the maximum number of agent steps (default 50).
	MaxSteps int

	// MaxFailures is the max consecutive failures before stopping (default 5).
	MaxFailures int

	// UseVision includes screenshots in LLM context when true.
	UseVision bool

	// Headless controls browser visibility if a new browser is created.
	Headless bool

	// OnStep is called after each step with progress info.
	OnStep func(StepInfo)

	// Verbose enables detailed logging.
	Verbose bool

	// MaxTokens sets the max tokens per LLM call (default 4096).
	MaxTokens int

	// Temperature sets the LLM sampling temperature (default 0.0).
	Temperature float64

	// CAPTCHASolver is an optional CAPTCHA solving backend.
	// When set, the agent automatically detects and solves CAPTCHAs.
	CAPTCHASolver CAPTCHASolver

	// Secrets stores sensitive data (credentials, TOTP secrets).
	// Keys ending in "_totp" auto-generate TOTP codes on access.
	// Use {{key_name}} placeholders in typed text for auto-replacement.
	Secrets map[string]string

	// Stealth enables anti-detection mode on the browser.
	Stealth bool
}

// StepInfo provides information about a completed agent step.
type StepInfo struct {
	Step     int
	Thought  string
	Action   string
	Params   map[string]interface{}
	Result   string
	Error    error
	Duration time.Duration
	URL      string
}

// AgentResult is the final result of an agent run.
type AgentResult struct {
	// Success indicates whether the task was completed.
	Success bool

	// Output is the final answer or result text.
	Output string

	// Steps is the total number of steps taken.
	Steps int

	// History contains details of each step.
	History []StepInfo

	// TotalTokens is the cumulative token usage.
	TotalTokens TokenUsage
}

// Agent is an LLM-driven autonomous browser that completes tasks.
type Agent struct {
	config        AgentConfig
	browser       *Browser
	page          *Page
	actions       *ActionRegistry
	history       []StepInfo
	tokens        TokenUsage
	ownsBrowser   bool
	cancelDialogs func()
	recentActions []string
	secrets       *Secrets
}

// NewAgent creates a new browsing agent.
func NewAgent(config AgentConfig) (*Agent, error) {
	if config.LLM == nil {
		return nil, fmt.Errorf("gosurfer: LLM provider is required")
	}
	if config.Task == "" {
		return nil, fmt.Errorf("gosurfer: task description is required")
	}
	if config.MaxSteps == 0 {
		config.MaxSteps = 50
	}
	if config.MaxFailures == 0 {
		config.MaxFailures = 5
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = 4096
	}

	a := &Agent{
		config:  config,
		actions: DefaultActions(),
	}
	if config.Secrets != nil {
		a.secrets = NewSecrets(config.Secrets)
	}
	return a, nil
}

// Run executes the agent's task and returns the result.
func (a *Agent) Run(ctx context.Context) (*AgentResult, error) {
	// Initialize browser if needed
	if a.config.Browser != nil {
		a.browser = a.config.Browser
	} else {
		browser, err := NewBrowser(BrowserConfig{
			Headless: a.config.Headless,
			Stealth:  a.config.Stealth,
		})
		if err != nil {
			return nil, fmt.Errorf("gosurfer agent: start browser: %w", err)
		}
		a.browser = browser
		a.ownsBrowser = true
	}

	// Create initial page
	page, err := a.browser.NewPage()
	if err != nil {
		return nil, fmt.Errorf("gosurfer agent: new page: %w", err)
	}
	a.page = page

	// Auto-dismiss JS dialogs (alert, confirm, prompt)
	a.cancelDialogs = a.page.AutoDismissDialogs()

	defer func() {
		if a.cancelDialogs != nil {
			a.cancelDialogs()
		}
		if a.ownsBrowser {
			_ = a.browser.Close()
		}
	}()

	// Agent loop
	consecutiveFailures := 0
	for step := 1; step <= a.config.MaxSteps; step++ {
		select {
		case <-ctx.Done():
			return a.buildResult(false, "Task cancelled"), ctx.Err()
		default:
		}

		start := time.Now()
		info, done, err := a.step(ctx, step)
		info.Duration = time.Since(start)
		info.URL = a.page.URL()

		if err != nil {
			consecutiveFailures++
			info.Error = err
			if a.config.Verbose {
				log.Printf("[gosurfer] step %d FAILED: %v", step, err)
			}
			if consecutiveFailures >= a.config.MaxFailures {
				a.history = append(a.history, info)
				return a.buildResult(false, fmt.Sprintf("Max failures reached: %v", err)), nil
			}
		} else {
			consecutiveFailures = 0
		}

		a.history = append(a.history, info)

		if a.config.OnStep != nil {
			a.config.OnStep(info)
		}

		if a.config.Verbose {
			log.Printf("[gosurfer] step %d: %s %v -> %s", step, info.Action, info.Params, truncate(info.Result, 100))
		}

		if done {
			return a.buildResult(true, info.Result), nil
		}
	}

	return a.buildResult(false, "Max steps reached"), nil
}

// step executes a single agent step: get state -> LLM -> execute action.
func (a *Agent) step(ctx context.Context, stepNum int) (StepInfo, bool, error) {
	info := StepInfo{Step: stepNum}

	// Check for CAPTCHA and auto-solve if solver is configured
	if a.config.CAPTCHASolver != nil {
		if captchaInfo, _ := a.page.DetectCAPTCHA(); captchaInfo != nil {
			if a.config.Verbose {
				log.Printf("[gosurfer] CAPTCHA detected: %s (sitekey: %s)", captchaInfo.Type, captchaInfo.SiteKey)
			}
			if err := a.page.SolveCAPTCHA(ctx, a.config.CAPTCHASolver); err != nil {
				if a.config.Verbose {
					log.Printf("[gosurfer] CAPTCHA solve failed: %v", err)
				}
			} else {
				if a.config.Verbose {
					log.Printf("[gosurfer] CAPTCHA solved successfully")
				}
				time.Sleep(2 * time.Second) // wait for page to process token
			}
		}
	}

	// Get current DOM state
	var state *DOMState
	var err error
	if a.config.UseVision {
		state, err = a.page.DOMStateWithScreenshot()
	} else {
		state, err = a.page.DOMState()
	}
	if err != nil {
		return info, false, fmt.Errorf("get DOM state: %w", err)
	}

	// Build messages for LLM
	messages := a.buildMessages(state, stepNum)

	// Call LLM
	opts := []ChatOption{
		WithMaxTokens(a.config.MaxTokens),
		WithTemperature(a.config.Temperature),
	}
	resp, err := a.config.LLM.ChatCompletion(ctx, messages, opts...)
	if err != nil {
		return info, false, fmt.Errorf("LLM call: %w", err)
	}

	// Track tokens
	a.tokens.PromptTokens += resp.Usage.PromptTokens
	a.tokens.CompletionTokens += resp.Usage.CompletionTokens
	a.tokens.TotalTokens += resp.Usage.TotalTokens

	// Parse LLM response
	thought, actionName, params, err := a.parseResponse(resp.Content)
	if err != nil {
		return info, false, fmt.Errorf("parse LLM response: %w", err)
	}

	info.Thought = thought
	info.Action = actionName
	info.Params = params

	// Execute action
	action, ok := a.actions.Get(actionName)
	if !ok {
		return info, false, fmt.Errorf("unknown action: %s", actionName)
	}

	result, err := action.Run(ctx, ActionContext{
		Page:    a.page,
		State:   state,
		Browser: a.browser,
		Agent:   a,
	}, params)
	if err != nil {
		info.Result = fmt.Sprintf("Error: %v", err)
		return info, false, err
	}

	info.Result = result

	// Check if done
	if actionName == "done" {
		return info, true, nil
	}

	// After click/navigate, check if a new tab opened and switch to it
	if actionName == "click" || actionName == "navigate" {
		a.checkForNewTabs()
	}

	// Loop detection
	a.trackAction(actionName, params)

	return info, false, nil
}

// checkForNewTabs detects and switches to newly opened tabs after actions.
func (a *Agent) checkForNewTabs() {
	pages, err := a.browser.Pages()
	if err != nil || len(pages) <= 1 {
		return
	}
	// If there are multiple pages, use the last one (most recently opened)
	lastPage := pages[len(pages)-1]
	currentURL := a.page.URL()
	lastURL := lastPage.URL()
	if lastURL != currentURL && lastURL != "" && lastURL != "about:blank" {
		// New tab detected, switch to it
		a.page = lastPage
		// Set up dialog handling on the new page
		if a.cancelDialogs != nil {
			a.cancelDialogs()
		}
		a.cancelDialogs = a.page.AutoDismissDialogs()
		if a.config.Verbose {
			log.Printf("[gosurfer] Switched to new tab: %s", lastURL)
		}
	}
}

// trackAction records actions for loop detection.
func (a *Agent) trackAction(action string, params map[string]interface{}) {
	sig := action
	if idx, ok := params["index"]; ok {
		sig += fmt.Sprintf(":%v", idx)
	}
	if url, ok := params["url"]; ok {
		sig += fmt.Sprintf(":%v", url)
	}
	a.recentActions = append(a.recentActions, sig)
	// Keep only last 20 actions
	if len(a.recentActions) > 20 {
		a.recentActions = a.recentActions[len(a.recentActions)-20:]
	}
}

// isLooping returns true if the agent appears stuck in a repetitive loop.
func (a *Agent) isLooping() bool {
	if len(a.recentActions) < 6 {
		return false
	}
	// Check if the last 3 actions repeat the same pattern
	n := len(a.recentActions)
	last3 := a.recentActions[n-3:]
	prev3 := a.recentActions[n-6 : n-3]
	for i := 0; i < 3; i++ {
		if last3[i] != prev3[i] {
			return false
		}
	}
	return true
}

// buildMessages constructs the LLM conversation for the current step.
func (a *Agent) buildMessages(state *DOMState, stepNum int) []ChatMessage {
	messages := []ChatMessage{
		TextMessage("system", buildSystemPrompt(a.config.Task, a.actions)),
	}

	// Add history summary (last 5 steps to stay within context)
	historyStart := 0
	if len(a.history) > 5 {
		historyStart = len(a.history) - 5
	}
	for _, h := range a.history[historyStart:] {
		summary := fmt.Sprintf("Step %d: Action=%s", h.Step, h.Action)
		if h.Result != "" {
			summary += " Result=" + truncate(h.Result, 200)
		}
		if h.Error != nil {
			summary += " Error=" + h.Error.Error()
		}
		messages = append(messages, TextMessage("user", summary))
		messages = append(messages, TextMessage("assistant", fmt.Sprintf(`{"thought":"executed %s","action":"%s"}`, h.Action, h.Action)))
	}

	// Build current state message with tab info
	var tabText string
	if len(state.Tabs) > 1 {
		tabText = "\nOpen Tabs:\n"
		for _, tab := range state.Tabs {
			marker := "  "
			if tab.URL == state.URL {
				marker = "> " // current tab
			}
			tabText += fmt.Sprintf("%s[%s] %s (%s)\n", marker, tab.ID, tab.Title, truncate(tab.URL, 60))
		}
	}

	// Loop detection nudge
	var loopNudge string
	if a.isLooping() {
		loopNudge = "\n!! WARNING: You appear to be stuck in a loop repeating the same actions. Try a completely different approach. !!\n"
	}

	stateText := fmt.Sprintf("Step %d/%d\nCurrent URL: %s\nPage Title: %s\nScroll: %.0f%%%s%s\nPage DOM:\n%s",
		stepNum, a.config.MaxSteps, state.URL, state.Title, state.ScrollPosition, tabText, loopNudge, state.Tree)

	if a.config.UseVision && state.Screenshot != nil {
		messages = append(messages, ImageMessage("user", stateText, state.Screenshot, "image/jpeg"))
	} else {
		messages = append(messages, TextMessage("user", stateText))
	}

	return messages
}

// llmResponse is the expected JSON structure from the LLM.
type llmResponse struct {
	Thought string                 `json:"thought"`
	Action  string                 `json:"action"`
	Params  map[string]interface{} `json:"params"`
}

// parseResponse extracts the action from the LLM's response text.
func (a *Agent) parseResponse(content string) (thought, action string, params map[string]interface{}, err error) {
	// Strip markdown code fences if present
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) > 2 {
			lines = lines[1 : len(lines)-1]
		}
		content = strings.Join(lines, "\n")
	}

	var resp llmResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		// Try to find JSON in the response
		start := strings.Index(content, "{")
		end := strings.LastIndex(content, "}")
		if start >= 0 && end > start {
			jsonStr := content[start : end+1]
			if err2 := json.Unmarshal([]byte(jsonStr), &resp); err2 != nil {
				return "", "", nil, fmt.Errorf("no valid JSON in response: %s", truncate(content, 200))
			}
		} else {
			return "", "", nil, fmt.Errorf("no valid JSON in response: %s", truncate(content, 200))
		}
	}

	if resp.Action == "" {
		return "", "", nil, fmt.Errorf("no action in response")
	}
	if resp.Params == nil {
		resp.Params = make(map[string]interface{})
	}

	return resp.Thought, resp.Action, resp.Params, nil
}

func (a *Agent) buildResult(success bool, output string) *AgentResult {
	return &AgentResult{
		Success:     success,
		Output:      output,
		Steps:       len(a.history),
		History:     a.history,
		TotalTokens: a.tokens,
	}
}

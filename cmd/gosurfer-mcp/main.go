// gosurfer-mcp is an MCP server that exposes browser automation tools.
// AI agents connect via HTTP+SSE to browse the web, take screenshots,
// fill forms, and extract data — without managing browsers themselves.
//
// Usage:
//
//	BRAVE_API_KEY=xxx gosurfer-mcp
//
// Environment variables:
//
//	MCP_PORT           HTTP port (default 8080)
//	BRAVE_API_KEY      Brave Search API key (required for search tool)
//	GOSURFER_PROXY     HTTP/SOCKS proxy (e.g. http://sdinas02:3128)
//	GOSURFER_PROFILE   Chrome profile directory
//	GOSURFER_HUMAN     "true" for maximum anti-detection (default true)
//	GOSURFER_HEADLESS  "false" to show browser window (default true)
//	CHROME_BIN         Custom Chrome binary path
//	GOSURFER_NO_SANDBOX "true" to disable Chrome sandbox (auto-detected in containers)
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dwoolworth/gosurfer"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var browser *gosurfer.Browser

func main() {
	port := os.Getenv("MCP_PORT")
	if port == "" {
		port = "8080"
	}

	// Launch shared browser
	if err := launchBrowser(); err != nil {
		log.Fatalf("Failed to launch browser: %v", err)
	}
	defer func() { _ = browser.Close() }()

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"gosurfer",
		"0.3.0",
		server.WithToolCapabilities(true),
	)

	// Register tools
	registerTools(mcpServer)

	// Create HTTP transport
	httpServer := server.NewStreamableHTTPServer(mcpServer)

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Println("Shutting down...")
		_ = browser.Close()
		os.Exit(0)
	}()

	log.Printf("gosurfer MCP server listening on :%s", port)
	log.Printf("Endpoint: http://localhost:%s/mcp", port)
	if err := httpServer.Start(":" + port); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func launchBrowser() error {
	headless := os.Getenv("GOSURFER_HEADLESS") != "false"
	humanMode := os.Getenv("GOSURFER_HUMAN") != "false" // default ON for MCP
	profile := os.Getenv("GOSURFER_PROFILE")
	proxy := os.Getenv("GOSURFER_PROXY")
	execPath := os.Getenv("CHROME_BIN")

	// Auto-detect system Chrome
	if execPath == "" {
		paths := []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium-browser",
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				execPath = p
				break
			}
		}
	}

	// Auto-detect container for no-sandbox
	noSandbox := os.Getenv("GOSURFER_NO_SANDBOX") == "true"
	if !noSandbox {
		if _, err := os.Stat("/.dockerenv"); err == nil {
			noSandbox = true
		} else if _, err := os.Stat("/run/secrets/kubernetes.io"); err == nil {
			noSandbox = true
		}
	}

	var err error
	browser, err = gosurfer.NewBrowser(gosurfer.BrowserConfig{
		Headless:    headless,
		HumanMode:   humanMode,
		Stealth:     true,
		ExecPath:    execPath,
		UserDataDir: profile,
		Proxy:       proxy,
		NoSandbox:   noSandbox,
	})
	if err != nil {
		return fmt.Errorf("launch browser: %w", err)
	}

	log.Printf("Browser launched (human=%v headless=%v proxy=%q profile=%q)",
		humanMode, headless, proxy, profile)
	return nil
}

// withPage creates a fresh page, runs the function, and closes the page.
func withPage(ctx context.Context, fn func(page *gosurfer.Page) (*mcp.CallToolResult, error)) (*mcp.CallToolResult, error) {
	page, err := browser.NewPage()
	if err != nil {
		return mcp.NewToolResultError("failed to create page: " + err.Error()), nil
	}
	defer func() { _ = page.Close() }()

	// Wrap with a timeout
	done := make(chan struct{})
	var result *mcp.CallToolResult
	var fnErr error
	go func() {
		result, fnErr = fn(page)
		close(done)
	}()

	select {
	case <-done:
		return result, fnErr
	case <-ctx.Done():
		return mcp.NewToolResultError("request timed out"), nil
	case <-time.After(60 * time.Second):
		return mcp.NewToolResultError("request timed out after 60s"), nil
	}
}

// normalizeURL adds https:// if no scheme is present.
func normalizeURL(u string) string {
	if !strings.Contains(u, "://") {
		return "https://" + u
	}
	return u
}

// getStringArg extracts a string argument from an MCP request.
func getStringArg(req mcp.CallToolRequest, name string) string {
	if v, ok := req.GetArguments()[name]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getBoolArg extracts a boolean argument from an MCP request.
func getBoolArg(req mcp.CallToolRequest, name string) bool {
	if v, ok := req.GetArguments()[name]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// getNumberArg extracts a number argument from an MCP request.
func getNumberArg(req mcp.CallToolRequest, name string, defaultVal float64) float64 {
	if v, ok := req.GetArguments()[name]; ok {
		if n, ok := v.(float64); ok {
			return n
		}
	}
	return defaultVal
}

func registerTools(s *server.MCPServer) {
	// search
	s.AddTool(mcp.NewTool("search",
		mcp.WithDescription("Search the web using Brave Search API. Returns titles, URLs, and descriptions. No browser needed — fast and cheap."),
		mcp.WithString("query", mcp.Required(), mcp.Description("The search query")),
		mcp.WithNumber("count", mcp.Description("Number of results (default 5, max 20)")),
	), handleSearch)

	// browse
	s.AddTool(mcp.NewTool("browse",
		mcp.WithDescription("Navigate to a URL and return the focused page content (main content, stripped of nav/footer/boilerplate). Best for reading pages."),
		mcp.WithString("url", mcp.Required(), mcp.Description("The URL to navigate to")),
	), handleBrowse)

	// browse_full
	s.AddTool(mcp.NewTool("browse_full",
		mcp.WithDescription("Navigate to a URL and return the complete DOM state with all interactive elements indexed. Use when you need full page controls."),
		mcp.WithString("url", mcp.Required(), mcp.Description("The URL to navigate to")),
	), handleBrowseFull)

	// screenshot
	s.AddTool(mcp.NewTool("screenshot",
		mcp.WithDescription("Navigate to a URL and capture a screenshot. Returns a PNG image."),
		mcp.WithString("url", mcp.Required(), mcp.Description("The URL to navigate to")),
		mcp.WithBoolean("full_page", mcp.Description("Capture full scrollable page instead of viewport (default false)")),
	), handleScreenshot)

	// interact
	s.AddTool(mcp.NewTool("interact",
		mcp.WithDescription(
			"Navigate to a URL, execute browser actions (click, type, scroll, wait), and return the final page state. "+
				"Actions is a JSON array, e.g.: "+
				`[{"action":"type","selector":"#email","text":"user@test.com"},{"action":"click","selector":"#submit"},{"action":"wait","seconds":2}]. `+
				`Supported: click, type, scroll, wait, click_index, type_index.`),
		mcp.WithString("url", mcp.Required(), mcp.Description("The URL to navigate to")),
		mcp.WithString("actions", mcp.Required(), mcp.Description("JSON array of actions to execute")),
	), handleInteract)

	// extract
	s.AddTool(mcp.NewTool("extract",
		mcp.WithDescription("Navigate to a URL and evaluate JavaScript to extract data. Returns the JS result as text."),
		mcp.WithString("url", mcp.Required(), mcp.Description("The URL to navigate to")),
		mcp.WithString("js", mcp.Required(), mcp.Description("JavaScript expression to evaluate (auto-wrapped in '() =>' if needed)")),
	), handleExtract)

	// pdf
	s.AddTool(mcp.NewTool("pdf",
		mcp.WithDescription("Navigate to a URL and generate a PDF of the page. Returns base64-encoded PDF data."),
		mcp.WithString("url", mcp.Required(), mcp.Description("The URL to navigate to")),
	), handlePDF)
}

// --- Tool Handlers ---

func handleSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := getStringArg(req, "query")
	if query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}
	count := int(getNumberArg(req, "count", 5))
	if count < 1 {
		count = 1
	}
	if count > 20 {
		count = 20
	}

	result, err := braveSearch(ctx, query, count)
	if err != nil {
		return mcp.NewToolResultError("search failed: " + err.Error()), nil
	}
	return mcp.NewToolResultText(result), nil
}

func handleBrowse(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawURL := getStringArg(req, "url")
	if rawURL == "" {
		return mcp.NewToolResultError("url is required"), nil
	}

	return withPage(ctx, func(page *gosurfer.Page) (*mcp.CallToolResult, error) {
		if err := page.Navigate(normalizeURL(rawURL)); err != nil {
			return mcp.NewToolResultError("navigation failed: " + err.Error()), nil
		}
		state, err := page.FocusedDOMState()
		if err != nil {
			return mcp.NewToolResultError("DOM extraction failed: " + err.Error()), nil
		}
		text := fmt.Sprintf("URL: %s\nTitle: %s\nElements: %d\n\n%s",
			state.URL, state.Title, len(state.Elements), state.Tree)
		return mcp.NewToolResultText(text), nil
	})
}

func handleBrowseFull(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawURL := getStringArg(req, "url")
	if rawURL == "" {
		return mcp.NewToolResultError("url is required"), nil
	}

	return withPage(ctx, func(page *gosurfer.Page) (*mcp.CallToolResult, error) {
		if err := page.Navigate(normalizeURL(rawURL)); err != nil {
			return mcp.NewToolResultError("navigation failed: " + err.Error()), nil
		}
		state, err := page.DOMState()
		if err != nil {
			return mcp.NewToolResultError("DOM extraction failed: " + err.Error()), nil
		}
		text := fmt.Sprintf("URL: %s\nTitle: %s\nElements: %d\n\n%s",
			state.URL, state.Title, len(state.Elements), state.Tree)
		return mcp.NewToolResultText(text), nil
	})
}

func handleScreenshot(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawURL := getStringArg(req, "url")
	if rawURL == "" {
		return mcp.NewToolResultError("url is required"), nil
	}
	fullPage := getBoolArg(req, "full_page")

	return withPage(ctx, func(page *gosurfer.Page) (*mcp.CallToolResult, error) {
		if err := page.Navigate(normalizeURL(rawURL)); err != nil {
			return mcp.NewToolResultError("navigation failed: " + err.Error()), nil
		}

		var png []byte
		var err error
		if fullPage {
			png, err = page.FullScreenshot()
		} else {
			png, err = page.Screenshot()
		}
		if err != nil {
			return mcp.NewToolResultError("screenshot failed: " + err.Error()), nil
		}

		b64 := base64.StdEncoding.EncodeToString(png)
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.NewImageContent(b64, "image/png")},
		}, nil
	})
}

func handleInteract(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawURL := getStringArg(req, "url")
	if rawURL == "" {
		return mcp.NewToolResultError("url is required"), nil
	}
	actionsJSON := getStringArg(req, "actions")
	if actionsJSON == "" {
		return mcp.NewToolResultError("actions is required"), nil
	}

	var actions []map[string]interface{}
	if err := json.Unmarshal([]byte(actionsJSON), &actions); err != nil {
		return mcp.NewToolResultError("invalid actions JSON: " + err.Error()), nil
	}

	return withPage(ctx, func(page *gosurfer.Page) (*mcp.CallToolResult, error) {
		if err := page.Navigate(normalizeURL(rawURL)); err != nil {
			return mcp.NewToolResultError("navigation failed: " + err.Error()), nil
		}

		// Execute actions sequentially
		for i, action := range actions {
			actionType, _ := action["action"].(string)
			selector, _ := action["selector"].(string)
			text, _ := action["text"].(string)

			var err error
			switch actionType {
			case "click":
				if selector == "" {
					return mcp.NewToolResultErrorf("action %d: click requires selector", i), nil
				}
				err = page.Click(selector)
			case "type":
				if selector == "" || text == "" {
					return mcp.NewToolResultErrorf("action %d: type requires selector and text", i), nil
				}
				err = page.Type(selector, text)
			case "scroll":
				amount, _ := action["amount"].(float64)
				if amount == 0 {
					amount = 300
				}
				err = page.Scroll(0, amount)
			case "wait":
				seconds, _ := action["seconds"].(float64)
				if seconds <= 0 {
					seconds = 1
				}
				if seconds > 10 {
					seconds = 10
				}
				time.Sleep(time.Duration(seconds*1000) * time.Millisecond)
			case "click_index":
				idx, _ := action["index"].(float64)
				err = clickByIndex(page, int(idx))
			case "type_index":
				idx, _ := action["index"].(float64)
				err = typeByIndex(page, int(idx), text)
			default:
				return mcp.NewToolResultErrorf("action %d: unknown action %q", i, actionType), nil
			}

			if err != nil {
				return mcp.NewToolResultErrorf("action %d (%s) failed: %v", i, actionType, err), nil
			}
		}

		// Return final page state
		state, err := page.FocusedDOMState()
		if err != nil {
			return mcp.NewToolResultError("DOM extraction after actions failed: " + err.Error()), nil
		}
		text := fmt.Sprintf("URL: %s\nTitle: %s\nElements: %d\n\n%s",
			state.URL, state.Title, len(state.Elements), state.Tree)
		return mcp.NewToolResultText(text), nil
	})
}

func handleExtract(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawURL := getStringArg(req, "url")
	if rawURL == "" {
		return mcp.NewToolResultError("url is required"), nil
	}
	js := getStringArg(req, "js")
	if js == "" {
		return mcp.NewToolResultError("js is required"), nil
	}

	// Auto-wrap if not already a function
	if !strings.HasPrefix(js, "()") {
		js = "() => " + js
	}

	return withPage(ctx, func(page *gosurfer.Page) (*mcp.CallToolResult, error) {
		if err := page.Navigate(normalizeURL(rawURL)); err != nil {
			return mcp.NewToolResultError("navigation failed: " + err.Error()), nil
		}

		val, err := page.Eval(js)
		if err != nil {
			return mcp.NewToolResultError("JS evaluation failed: " + err.Error()), nil
		}

		// Convert result to string
		var result string
		switch v := val.(type) {
		case string:
			result = v
		case nil:
			result = "null"
		default:
			b, _ := json.Marshal(v)
			result = string(b)
		}

		return mcp.NewToolResultText(result), nil
	})
}

func handlePDF(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawURL := getStringArg(req, "url")
	if rawURL == "" {
		return mcp.NewToolResultError("url is required"), nil
	}

	return withPage(ctx, func(page *gosurfer.Page) (*mcp.CallToolResult, error) {
		if err := page.Navigate(normalizeURL(rawURL)); err != nil {
			return mcp.NewToolResultError("navigation failed: " + err.Error()), nil
		}

		pdfBytes, err := page.PDF()
		if err != nil {
			return mcp.NewToolResultError("PDF generation failed: " + err.Error()), nil
		}

		b64 := base64.StdEncoding.EncodeToString(pdfBytes)
		return mcp.NewToolResultText("data:application/pdf;base64," + b64), nil
	})
}

// --- Index-based interaction helpers ---

func clickByIndex(page *gosurfer.Page, idx int) error {
	state, err := page.DOMState()
	if err != nil {
		return fmt.Errorf("get DOM state: %w", err)
	}
	el, ok := state.Elements[idx]
	if !ok {
		return fmt.Errorf("element [%d] not found", idx)
	}
	if el.CSSSelector == "" {
		return fmt.Errorf("element [%d] has no CSS selector", idx)
	}
	return page.Click(el.CSSSelector)
}

func typeByIndex(page *gosurfer.Page, idx int, text string) error {
	state, err := page.DOMState()
	if err != nil {
		return fmt.Errorf("get DOM state: %w", err)
	}
	el, ok := state.Elements[idx]
	if !ok {
		return fmt.Errorf("element [%d] not found", idx)
	}
	if el.CSSSelector == "" {
		return fmt.Errorf("element [%d] has no CSS selector", idx)
	}
	return page.Type(el.CSSSelector, text)
}

// --- Brave Search ---

type braveResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

func braveSearch(ctx context.Context, query string, count int) (string, error) {
	apiKey := os.Getenv("BRAVE_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("BRAVE_API_KEY environment variable not set")
	}

	u := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query), count)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("brave API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("brave API returned %d: %s", resp.StatusCode, string(body))
	}

	var result braveResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parse brave response: %w", err)
	}

	var sb strings.Builder
	for i, r := range result.Web.Results {
		fmt.Fprintf(&sb, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Description)
	}

	if sb.Len() == 0 {
		return "No results found.", nil
	}
	return sb.String(), nil
}

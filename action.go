package gosurfer

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// ActionDef defines a browser action that the agent can execute.
type ActionDef struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Params      []ParamDef `json:"params"`
	Run         func(ctx context.Context, ac ActionContext, params map[string]interface{}) (string, error)
}

// ParamDef describes a parameter for an action.
type ParamDef struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // "string", "int", "float", "bool"
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ActionContext provides the browser state to action handlers.
type ActionContext struct {
	Page    *Page
	State   *DOMState
	Browser *Browser
	Agent   *Agent
}

// ActionRegistry manages available actions.
type ActionRegistry struct {
	actions map[string]*ActionDef
	order   []string // preserve insertion order for schema generation
}

// NewActionRegistry creates an empty action registry.
func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{
		actions: make(map[string]*ActionDef),
	}
}

// Register adds an action to the registry.
func (r *ActionRegistry) Register(action *ActionDef) {
	r.actions[action.Name] = action
	r.order = append(r.order, action.Name)
}

// Get returns an action by name.
func (r *ActionRegistry) Get(name string) (*ActionDef, bool) {
	a, ok := r.actions[name]
	return a, ok
}

// Actions returns all registered actions in order.
func (r *ActionRegistry) Actions() []*ActionDef {
	result := make([]*ActionDef, 0, len(r.order))
	for _, name := range r.order {
		if a, ok := r.actions[name]; ok {
			result = append(result, a)
		}
	}
	return result
}

// DefaultActions returns the built-in action set.
func DefaultActions() *ActionRegistry {
	r := NewActionRegistry()

	r.Register(&ActionDef{
		Name:        "navigate",
		Description: "Navigate to a URL.",
		Params: []ParamDef{
			{Name: "url", Type: "string", Description: "The URL to navigate to.", Required: true},
		},
		Run: actionNavigate,
	})

	r.Register(&ActionDef{
		Name:        "click",
		Description: "Click an element by its index number, OR click at viewport coordinates (x, y). Use index for interactive elements, coordinates for canvas/complex UI.",
		Params: []ParamDef{
			{Name: "index", Type: "int", Description: "The element index to click (use this OR coordinates).", Required: false},
			{Name: "x", Type: "float", Description: "Viewport X coordinate to click (use with y instead of index).", Required: false},
			{Name: "y", Type: "float", Description: "Viewport Y coordinate to click (use with x instead of index).", Required: false},
		},
		Run: actionClick,
	})

	r.Register(&ActionDef{
		Name:        "type",
		Description: "Type text into an input element. Clears existing text first.",
		Params: []ParamDef{
			{Name: "index", Type: "int", Description: "The element index to type into.", Required: true},
			{Name: "text", Type: "string", Description: "The text to type.", Required: true},
			{Name: "clear", Type: "bool", Description: "Whether to clear existing text first (default true).", Required: false},
		},
		Run: actionType,
	})

	r.Register(&ActionDef{
		Name:        "scroll",
		Description: "Scroll the page or a specific element.",
		Params: []ParamDef{
			{Name: "direction", Type: "string", Description: "Scroll direction: 'up' or 'down'.", Required: true},
			{Name: "amount", Type: "int", Description: "Number of pixels to scroll (default 500).", Required: false},
			{Name: "index", Type: "int", Description: "Element index to scroll within (optional, scrolls page if omitted).", Required: false},
		},
		Run: actionScroll,
	})

	r.Register(&ActionDef{
		Name:        "search",
		Description: "Search the web using a search engine.",
		Params: []ParamDef{
			{Name: "query", Type: "string", Description: "The search query.", Required: true},
			{Name: "engine", Type: "string", Description: "Search engine: 'google', 'duckduckgo', 'bing' (default 'google').", Required: false},
		},
		Run: actionSearch,
	})

	r.Register(&ActionDef{
		Name:        "go_back",
		Description: "Navigate back in browser history.",
		Params:      nil,
		Run:         actionGoBack,
	})

	r.Register(&ActionDef{
		Name:        "wait",
		Description: "Wait for a specified number of seconds (max 10).",
		Params: []ParamDef{
			{Name: "seconds", Type: "int", Description: "Seconds to wait (1-10).", Required: true},
		},
		Run: actionWait,
	})

	r.Register(&ActionDef{
		Name:        "screenshot",
		Description: "Take a screenshot of the current page.",
		Params:      nil,
		Run:         actionScreenshot,
	})

	r.Register(&ActionDef{
		Name:        "extract",
		Description: "Extract specific information from the current page based on a query.",
		Params: []ParamDef{
			{Name: "query", Type: "string", Description: "What information to extract from the page.", Required: true},
		},
		Run: actionExtract,
	})

	r.Register(&ActionDef{
		Name:        "send_keys",
		Description: "Send keyboard keys (e.g., 'Enter', 'Escape', 'Tab', 'Backspace').",
		Params: []ParamDef{
			{Name: "keys", Type: "string", Description: "Key name or combination (e.g., 'Enter', 'Control+a').", Required: true},
		},
		Run: actionSendKeys,
	})

	r.Register(&ActionDef{
		Name:        "select_option",
		Description: "Select an option from a dropdown/select element by visible text.",
		Params: []ParamDef{
			{Name: "index", Type: "int", Description: "The select element index.", Required: true},
			{Name: "text", Type: "string", Description: "The option text to select.", Required: true},
		},
		Run: actionSelectOption,
	})

	r.Register(&ActionDef{
		Name:        "switch_tab",
		Description: "Switch to a different browser tab by its tab ID (shown in the tab list).",
		Params: []ParamDef{
			{Name: "tab_id", Type: "string", Description: "The tab ID to switch to.", Required: true},
		},
		Run: actionSwitchTab,
	})

	r.Register(&ActionDef{
		Name:        "close_tab",
		Description: "Close a browser tab by its tab ID.",
		Params: []ParamDef{
			{Name: "tab_id", Type: "string", Description: "The tab ID to close.", Required: true},
		},
		Run: actionCloseTab,
	})

	r.Register(&ActionDef{
		Name:        "new_tab",
		Description: "Open a new browser tab with the given URL.",
		Params: []ParamDef{
			{Name: "url", Type: "string", Description: "The URL to open in the new tab.", Required: true},
		},
		Run: actionNewTab,
	})

	r.Register(&ActionDef{
		Name:        "upload_file",
		Description: "Upload a file to a file input element.",
		Params: []ParamDef{
			{Name: "index", Type: "int", Description: "The file input element index.", Required: true},
			{Name: "path", Type: "string", Description: "The file path to upload.", Required: true},
		},
		Run: actionUploadFile,
	})

	r.Register(&ActionDef{
		Name:        "done",
		Description: "Signal that the task is complete. Use this when the goal has been achieved or determined impossible.",
		Params: []ParamDef{
			{Name: "output", Type: "string", Description: "The result or answer for the task.", Required: true},
			{Name: "success", Type: "bool", Description: "Whether the task was completed successfully.", Required: true},
		},
		Run: actionDone,
	})

	return r
}

// --- Action Implementations ---

func actionNavigate(_ context.Context, ac ActionContext, params map[string]interface{}) (string, error) {
	url, _ := params["url"].(string)
	if url == "" {
		return "", fmt.Errorf("url is required")
	}
	if err := ac.Page.Navigate(url); err != nil {
		return "", err
	}
	return fmt.Sprintf("Navigated to %s", url), nil
}

func actionClick(_ context.Context, ac ActionContext, params map[string]interface{}) (string, error) {
	// Coordinate-based click
	if xVal, hasX := params["x"]; hasX {
		if yVal, hasY := params["y"]; hasY {
			x, err := toFloat(xVal)
			if err != nil {
				return "", fmt.Errorf("x must be a number")
			}
			y, err := toFloat(yVal)
			if err != nil {
				return "", fmt.Errorf("y must be a number")
			}
			if err := ac.Page.rod.Mouse.MoveTo(proto.Point{X: x, Y: y}); err != nil {
				return "", fmt.Errorf("move mouse: %w", err)
			}
			if err := ac.Page.rod.Mouse.Click(proto.InputMouseButtonLeft, 1); err != nil {
				return "", fmt.Errorf("click at (%.0f, %.0f): %w", x, y, err)
			}
			time.Sleep(300 * time.Millisecond)
			return fmt.Sprintf("Clicked at coordinates (%.0f, %.0f)", x, y), nil
		}
	}

	// Index-based click
	idx, err := toInt(params["index"])
	if err != nil {
		return "", fmt.Errorf("provide either index (int) or x,y coordinates")
	}
	el, ok := ac.State.Elements[idx]
	if !ok {
		return "", fmt.Errorf("element index %d not found in DOM state", idx)
	}

	rodEl, err := ac.Page.rod.Element(el.CSSSelector)
	if err != nil {
		return "", fmt.Errorf("find element [%d]: %w", idx, err)
	}
	wrapped := &Element{rod: rodEl, page: ac.Page, Index: idx}
	if err := wrapped.Click(); err != nil {
		return "", fmt.Errorf("click [%d]: %w", idx, err)
	}
	time.Sleep(300 * time.Millisecond)
	return fmt.Sprintf("Clicked element [%d] <%s>", idx, el.Tag), nil
}

func actionType(_ context.Context, ac ActionContext, params map[string]interface{}) (string, error) {
	idx, err := toInt(params["index"])
	if err != nil {
		return "", fmt.Errorf("index is required (int)")
	}
	text, _ := params["text"].(string)
	if text == "" {
		return "", fmt.Errorf("text is required")
	}

	// Replace secret placeholders if agent has secrets configured
	if ac.Agent != nil && ac.Agent.secrets != nil {
		text = ac.Agent.secrets.ReplaceInText(text)
	}

	el, ok := ac.State.Elements[idx]
	if !ok {
		return "", fmt.Errorf("element index %d not found", idx)
	}

	rodEl, err := ac.Page.rod.Element(el.CSSSelector)
	if err != nil {
		return "", fmt.Errorf("find element [%d]: %w", idx, err)
	}
	wrapped := &Element{rod: rodEl, page: ac.Page, Index: idx}

	// Default: clear first
	shouldClear := true
	if c, ok := params["clear"].(bool); ok {
		shouldClear = c
	}
	if shouldClear {
		_ = wrapped.Clear()
	}

	if err := wrapped.Type(text); err != nil {
		return "", fmt.Errorf("type into [%d]: %w", idx, err)
	}
	return fmt.Sprintf("Typed %q into element [%d]", text, idx), nil
}

func actionScroll(_ context.Context, ac ActionContext, params map[string]interface{}) (string, error) {
	direction, _ := params["direction"].(string)
	amount := 500
	if a, err := toInt(params["amount"]); err == nil {
		amount = a
	}

	dy := float64(amount)
	if direction == "up" {
		dy = -dy
	}

	if err := ac.Page.Scroll(0, dy); err != nil {
		return "", fmt.Errorf("scroll: %w", err)
	}
	return fmt.Sprintf("Scrolled %s by %d pixels", direction, amount), nil
}

func actionSearch(_ context.Context, ac ActionContext, params map[string]interface{}) (string, error) {
	query, _ := params["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	engine, _ := params["engine"].(string)
	if engine == "" {
		engine = "google"
	}

	var url string
	switch engine {
	case "google":
		url = "https://www.google.com/search?q=" + query
	case "duckduckgo":
		url = "https://duckduckgo.com/?q=" + query
	case "bing":
		url = "https://www.bing.com/search?q=" + query
	default:
		url = "https://www.google.com/search?q=" + query
	}

	if err := ac.Page.Navigate(url); err != nil {
		return "", err
	}
	return fmt.Sprintf("Searched %q on %s", query, engine), nil
}

func actionGoBack(_ context.Context, ac ActionContext, _ map[string]interface{}) (string, error) {
	if err := ac.Page.Back(); err != nil {
		return "", err
	}
	return "Navigated back", nil
}

func actionWait(_ context.Context, _ ActionContext, params map[string]interface{}) (string, error) {
	seconds, err := toInt(params["seconds"])
	if err != nil || seconds < 1 {
		seconds = 1
	}
	if seconds > 10 {
		seconds = 10
	}
	time.Sleep(time.Duration(seconds) * time.Second)
	return fmt.Sprintf("Waited %d seconds", seconds), nil
}

func actionScreenshot(_ context.Context, ac ActionContext, _ map[string]interface{}) (string, error) {
	_, err := ac.Page.Screenshot()
	if err != nil {
		return "", fmt.Errorf("screenshot: %w", err)
	}
	return "Screenshot taken", nil
}

func actionExtract(_ context.Context, ac ActionContext, params map[string]interface{}) (string, error) {
	query, _ := params["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	// Use the DOM tree text for extraction
	return fmt.Sprintf("[Page content for extraction query %q]\nURL: %s\nTitle: %s\n\n%s",
		query, ac.State.URL, ac.State.Title, ac.State.Tree), nil
}

func actionSendKeys(_ context.Context, ac ActionContext, params map[string]interface{}) (string, error) {
	keys, _ := params["keys"].(string)
	if keys == "" {
		return "", fmt.Errorf("keys is required")
	}
	_, err := ac.Page.rod.Eval(fmt.Sprintf(`() => {
		const event = new KeyboardEvent('keydown', {key: '%s', bubbles: true});
		document.activeElement.dispatchEvent(event);
	}`, keys))
	if err != nil {
		return "", fmt.Errorf("send keys: %w", err)
	}
	return fmt.Sprintf("Sent keys: %s", keys), nil
}

func actionSelectOption(_ context.Context, ac ActionContext, params map[string]interface{}) (string, error) {
	idx, err := toInt(params["index"])
	if err != nil {
		return "", fmt.Errorf("index is required (int)")
	}
	text, _ := params["text"].(string)
	if text == "" {
		return "", fmt.Errorf("text is required")
	}

	el, ok := ac.State.Elements[idx]
	if !ok {
		return "", fmt.Errorf("element index %d not found", idx)
	}

	rodEl, err := ac.Page.rod.Element(el.CSSSelector)
	if err != nil {
		return "", fmt.Errorf("find element [%d]: %w", idx, err)
	}
	wrapped := &Element{rod: rodEl, page: ac.Page}
	if err := wrapped.SelectOption(text); err != nil {
		return "", fmt.Errorf("select option: %w", err)
	}
	return fmt.Sprintf("Selected %q in element [%d]", text, idx), nil
}

func actionSwitchTab(_ context.Context, ac ActionContext, params map[string]interface{}) (string, error) {
	tabID, _ := params["tab_id"].(string)
	if tabID == "" {
		return "", fmt.Errorf("tab_id is required")
	}
	if ac.Browser == nil {
		return "", fmt.Errorf("no browser context for tab switching")
	}

	pages, err := ac.Browser.rod.Pages()
	if err != nil {
		return "", fmt.Errorf("list pages: %w", err)
	}

	for _, p := range pages {
		tid := string(p.TargetID)
		shortID := tid
		if len(tid) > 4 {
			shortID = tid[len(tid)-4:]
		}
		if shortID == tabID {
			info, err := p.Info()
			if err != nil {
				return "", fmt.Errorf("get page info: %w", err)
			}
			pg := &Page{rod: p, browser: ac.Browser}
			pg.dom = &DOMService{page: pg}
			if ac.Agent != nil {
				ac.Agent.page = pg
				// Re-setup dialog handling on new page
				if ac.Agent.cancelDialogs != nil {
					ac.Agent.cancelDialogs()
				}
				ac.Agent.cancelDialogs = pg.AutoDismissDialogs()
			}
			return fmt.Sprintf("Switched to tab %s: %s", tabID, info.URL), nil
		}
	}
	return "", fmt.Errorf("tab %q not found", tabID)
}

func actionCloseTab(_ context.Context, ac ActionContext, params map[string]interface{}) (string, error) {
	tabID, _ := params["tab_id"].(string)
	if tabID == "" {
		return "", fmt.Errorf("tab_id is required")
	}
	if ac.Browser == nil {
		return "", fmt.Errorf("no browser context")
	}

	pages, err := ac.Browser.rod.Pages()
	if err != nil {
		return "", fmt.Errorf("list pages: %w", err)
	}

	for _, p := range pages {
		tid := string(p.TargetID)
		shortID := tid
		if len(tid) > 4 {
			shortID = tid[len(tid)-4:]
		}
		if shortID == tabID {
			if err := p.Close(); err != nil {
				return "", fmt.Errorf("close tab: %w", err)
			}
			return fmt.Sprintf("Closed tab %s", tabID), nil
		}
	}
	return "", fmt.Errorf("tab %q not found", tabID)
}

func actionNewTab(_ context.Context, ac ActionContext, params map[string]interface{}) (string, error) {
	url, _ := params["url"].(string)
	if url == "" {
		return "", fmt.Errorf("url is required")
	}
	if ac.Browser == nil {
		return "", fmt.Errorf("no browser context")
	}

	page, err := ac.Browser.NewPage()
	if err != nil {
		return "", fmt.Errorf("new tab: %w", err)
	}
	if err := page.Navigate(url); err != nil {
		return "", fmt.Errorf("navigate new tab: %w", err)
	}

	// Switch agent to the new tab
	if ac.Agent != nil {
		ac.Agent.page = page
		if ac.Agent.cancelDialogs != nil {
			ac.Agent.cancelDialogs()
		}
		ac.Agent.cancelDialogs = page.AutoDismissDialogs()
	}

	return fmt.Sprintf("Opened new tab: %s", url), nil
}

func actionUploadFile(_ context.Context, ac ActionContext, params map[string]interface{}) (string, error) {
	idx, err := toInt(params["index"])
	if err != nil {
		return "", fmt.Errorf("index is required (int)")
	}
	path, _ := params["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	el, ok := ac.State.Elements[idx]
	if !ok {
		return "", fmt.Errorf("element index %d not found", idx)
	}

	rodEl, err := ac.Page.rod.Element(el.CSSSelector)
	if err != nil {
		return "", fmt.Errorf("find element [%d]: %w", idx, err)
	}
	if err := rodEl.SetFiles([]string{path}); err != nil {
		return "", fmt.Errorf("upload file: %w", err)
	}
	return fmt.Sprintf("Uploaded %q to element [%d]", path, idx), nil
}

func actionDone(_ context.Context, _ ActionContext, params map[string]interface{}) (string, error) {
	output, _ := params["output"].(string)
	return output, nil
}

// --- Helpers ---

func toFloat(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case string:
		return strconv.ParseFloat(val, 64)
	case json.Number:
		return val.Float64()
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

func toInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case int:
		return val, nil
	case float64:
		return int(val), nil
	case string:
		return strconv.Atoi(val)
	case json.Number:
		i, err := val.Int64()
		return int(i), err
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

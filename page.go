package gosurfer

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// Page wraps a browser tab with high-level interaction methods.
type Page struct {
	rod     *rod.Page
	browser *Browser
	dom     *DOMService
}

// defaultChallengeWaitTimeout is used by Navigate when the browser config
// does not set an explicit ChallengeWaitTimeout. Cloudflare's UAM JS
// challenge typically resolves in 5-15 seconds; 15s gives us headroom.
const defaultChallengeWaitTimeout = 15 * time.Second

// Navigate loads a URL and waits for the page to be ready. If the page is
// served a bot-protection challenge that can be auto-solved (e.g.,
// Cloudflare's "Just a moment..." JavaScript challenge), Navigate will
// poll until the challenge clears or the configured timeout elapses.
//
// Navigate does NOT return an error when the page lands on a
// non-auto-solvable challenge (Turnstile, DataDome) — the page did load,
// it just loaded a challenge. Callers who want to fail fast in that case
// should call DetectChallenge() after Navigate and check the return value.
// Only an auto-solvable challenge that fails to clear within the timeout
// produces an error.
func (p *Page) Navigate(url string) error {
	if err := p.rod.Navigate(url); err != nil {
		return fmt.Errorf("gosurfer: navigate: %w", err)
	}
	if err := p.rod.WaitLoad(); err != nil {
		return fmt.Errorf("gosurfer: wait load: %w", err)
	}

	// Determine the wait timeout. 0 means "use default", -1 means disabled.
	timeout := defaultChallengeWaitTimeout
	if p.browser != nil {
		switch t := p.browser.config.ChallengeWaitTimeout; {
		case t < 0:
			timeout = 0
		case t > 0:
			timeout = t
		}
	}

	if timeout > 0 {
		if _, _, err := p.WaitForChallenge(timeout); err != nil {
			return fmt.Errorf("gosurfer: navigate: %w", err)
		}
	}
	return nil
}

// Back navigates backward in history.
func (p *Page) Back() error {
	return p.rod.NavigateBack()
}

// Forward navigates forward in history.
func (p *Page) Forward() error {
	return p.rod.NavigateForward()
}

// Reload refreshes the current page.
func (p *Page) Reload() error {
	return p.rod.Reload()
}

// URL returns the current page URL.
func (p *Page) URL() string {
	info, err := p.rod.Info()
	if err != nil {
		return ""
	}
	return info.URL
}

// Title returns the page title.
func (p *Page) Title() (string, error) {
	info, err := p.rod.Info()
	if err != nil {
		return "", fmt.Errorf("gosurfer: title: %w", err)
	}
	return info.Title, nil
}

// HTML returns the full page HTML.
func (p *Page) HTML() (string, error) {
	return p.rod.HTML()
}

// Element finds a single element by CSS selector (waits for it to appear).
func (p *Page) Element(selector string) (*Element, error) {
	el, err := p.rod.Element(selector)
	if err != nil {
		return nil, fmt.Errorf("gosurfer: element %q: %w", selector, err)
	}
	return &Element{rod: el, page: p}, nil
}

// Elements finds all matching elements by CSS selector.
func (p *Page) Elements(selector string) ([]*Element, error) {
	els, err := p.rod.Elements(selector)
	if err != nil {
		return nil, fmt.Errorf("gosurfer: elements %q: %w", selector, err)
	}
	result := make([]*Element, len(els))
	for i, el := range els {
		result[i] = &Element{rod: el, page: p}
	}
	return result, nil
}

// ElementByXPath finds an element by XPath expression.
func (p *Page) ElementByXPath(xpath string) (*Element, error) {
	el, err := p.rod.ElementX(xpath)
	if err != nil {
		return nil, fmt.Errorf("gosurfer: xpath %q: %w", xpath, err)
	}
	return &Element{rod: el, page: p}, nil
}

// Click finds an element by selector and clicks it.
func (p *Page) Click(selector string) error {
	el, err := p.Element(selector)
	if err != nil {
		return err
	}
	return el.Click()
}

// Type finds an element by selector and types text into it.
func (p *Page) Type(selector, text string) error {
	el, err := p.Element(selector)
	if err != nil {
		return err
	}
	return el.Type(text)
}

// Text returns the text content of an element matched by selector.
func (p *Page) Text(selector string) (string, error) {
	el, err := p.Element(selector)
	if err != nil {
		return "", err
	}
	return el.Text()
}

// Screenshot captures the visible viewport as PNG bytes.
func (p *Page) Screenshot() ([]byte, error) {
	return p.rod.Screenshot(false, &proto.PageCaptureScreenshot{
		Format:  proto.PageCaptureScreenshotFormatPng,
		Quality: nil,
	})
}

// FullScreenshot captures the entire page (scrolled) as PNG bytes.
func (p *Page) FullScreenshot() ([]byte, error) {
	return p.rod.Screenshot(true, &proto.PageCaptureScreenshot{
		Format:  proto.PageCaptureScreenshotFormatPng,
		Quality: nil,
	})
}

// ScreenshotJPEG captures the viewport as JPEG bytes with the given quality (0-100).
func (p *Page) ScreenshotJPEG(quality int) ([]byte, error) {
	q := quality
	return p.rod.Screenshot(false, &proto.PageCaptureScreenshot{
		Format:  proto.PageCaptureScreenshotFormatJpeg,
		Quality: &q,
	})
}

// PDF generates a PDF of the current page.
func (p *Page) PDF() ([]byte, error) {
	reader, err := p.rod.PDF(&proto.PagePrintToPDF{
		PrintBackground: true,
	})
	if err != nil {
		return nil, fmt.Errorf("gosurfer: pdf: %w", err)
	}
	defer func() { _ = reader.Close() }()
	return io.ReadAll(reader)
}

// Eval evaluates JavaScript in the page context and returns the result.
func (p *Page) Eval(js string, args ...interface{}) (interface{}, error) {
	result, err := p.rod.Eval(js, args...)
	if err != nil {
		return nil, fmt.Errorf("gosurfer: eval: %w", err)
	}
	return result.Value.Val(), nil
}

// Scroll scrolls the page by the given number of pixels.
// Positive dy scrolls down, negative scrolls up.
// Positive dx scrolls right, negative scrolls left.
func (p *Page) Scroll(dx, dy float64) error {
	return p.rod.Mouse.Scroll(dx, dy, 0)
}

// ScrollToBottom scrolls to the bottom of the page.
func (p *Page) ScrollToBottom() error {
	_, err := p.rod.Eval(`() => window.scrollTo(0, document.body.scrollHeight)`)
	return err
}

// ScrollToTop scrolls to the top of the page.
func (p *Page) ScrollToTop() error {
	_, err := p.rod.Eval(`() => window.scrollTo(0, 0)`)
	return err
}

// WaitLoad waits for the page load event.
func (p *Page) WaitLoad() error {
	return p.rod.WaitLoad()
}

// WaitIdle waits until the page has no pending network requests.
func (p *Page) WaitIdle(timeout time.Duration) error {
	return p.rod.WaitIdle(timeout)
}

// WaitStable waits until the page DOM stops changing.
func (p *Page) WaitStable(interval time.Duration) error {
	return p.rod.WaitStable(interval)
}

// WaitSelector waits for an element matching the selector to appear.
func (p *Page) WaitSelector(selector string) (*Element, error) {
	el, err := p.rod.Element(selector) // rod.Element already waits
	if err != nil {
		return nil, fmt.Errorf("gosurfer: wait selector %q: %w", selector, err)
	}
	return &Element{rod: el, page: p}, nil
}

// DOMState extracts the current page state optimized for LLM consumption.
// This is the key method for AI agent integration.
func (p *Page) DOMState() (*DOMState, error) {
	return p.dom.GetState()
}

// FocusedDOMState extracts a pruned DOM state with boilerplate stripped.
// Removes nav, footer, cookie banners, ad containers, social links, and
// low-value links (terms, privacy, copyright, same-page anchors).
// Focuses on <main>, <article>, [role="main"] content regions.
// Typically produces 30-60% fewer tokens than DOMState.
func (p *Page) FocusedDOMState() (*DOMState, error) {
	return p.dom.GetFocusedState()
}

// DOMStateWithScreenshot extracts DOM state and captures a screenshot.
func (p *Page) DOMStateWithScreenshot() (*DOMState, error) {
	state, err := p.dom.GetState()
	if err != nil {
		return nil, err
	}
	screenshot, err := p.ScreenshotJPEG(75)
	if err != nil {
		return state, nil // return state without screenshot on error
	}
	state.Screenshot = screenshot
	return state, nil
}

// KeyPress sends a keyboard event (e.g., input.Enter, input.Escape, input.Tab).
func (p *Page) KeyPress(key input.Key) error {
	return p.rod.Keyboard.Press(key)
}

// --- Dialog Handling ---

// Dialog represents a JavaScript dialog (alert, confirm, prompt, beforeunload).
type Dialog struct {
	Type           string // "alert", "confirm", "prompt", "beforeunload"
	Message        string
	DefaultPrompt  string
	page           *rod.Page
}

// HandleDialog returns two functions: wait blocks until the next JS dialog opens,
// and handle accepts/dismisses it. Use for fine-grained dialog control.
func (p *Page) HandleDialog() (wait func() *Dialog, handle func(accept bool, promptText string) error) {
	w, h := p.rod.HandleDialog()
	wait = func() *Dialog {
		ev := w()
		return &Dialog{
			Type:          string(ev.Type),
			Message:       ev.Message,
			DefaultPrompt: ev.DefaultPrompt,
			page:          p.rod,
		}
	}
	handle = func(accept bool, promptText string) error {
		return h(&proto.PageHandleJavaScriptDialog{
			Accept:     accept,
			PromptText: promptText,
		})
	}
	return
}

// AutoDismissDialogs sets up automatic handling of JS dialogs.
// Alerts are accepted, confirms are accepted, prompts are dismissed.
// Returns a cancel function to stop auto-handling.
func (p *Page) AutoDismissDialogs() func() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			w, h := p.rod.HandleDialog()
			ev := w()
			accept := ev.Type != proto.PageDialogTypePrompt
			_ = h(&proto.PageHandleJavaScriptDialog{Accept: accept})
		}
	}()
	return cancel
}

// --- Popup / New Tab Detection ---

// WaitPopup waits for a new page/tab opened by this page (e.g., window.open,
// target="_blank"). Call before the action that triggers the popup.
func (p *Page) WaitPopup() func() (*Page, error) {
	w := p.rod.MustWaitOpen()
	return func() (*Page, error) {
		newRodPage := w()
		pg := &Page{rod: newRodPage, browser: p.browser}
		pg.dom = &DOMService{page: pg}
		return pg, nil
	}
}

// --- Iframe Access ---

// Frames returns all iframes on the page as Page instances.
func (p *Page) Frames() ([]*Page, error) {
	iframes, err := p.rod.Elements("iframe")
	if err != nil {
		return nil, err
	}
	var frames []*Page
	for _, iframe := range iframes {
		frame, err := iframe.Frame()
		if err != nil {
			continue // skip cross-origin or inaccessible iframes
		}
		pg := &Page{rod: frame, browser: p.browser}
		pg.dom = &DOMService{page: pg}
		frames = append(frames, pg)
	}
	return frames, nil
}

// --- File Dialog ---

// HandleFileDialog prepares to handle a native file chooser dialog.
// Call before the action that triggers the file dialog, then invoke the
// returned function with the file paths to select.
func (p *Page) HandleFileDialog() (func(paths []string) error, error) {
	w, err := p.rod.HandleFileDialog()
	if err != nil {
		return nil, fmt.Errorf("gosurfer: handle file dialog: %w", err)
	}
	return func(paths []string) error {
		return w(paths)
	}, nil
}

// --- Expose Go Functions to Page JS ---

// TargetID returns the CDP target ID for this page (used for tab tracking).
// Returns the last 4 chars as a short ID, matching Browser Use convention.
func (p *Page) TargetID() string {
	tid := string(p.rod.TargetID)
	if len(tid) > 4 {
		return tid[len(tid)-4:]
	}
	return tid
}

// IsIframe returns whether this page represents an iframe.
func (p *Page) IsIframe() bool {
	return p.rod.IsIframe()
}

// Close closes this page/tab.
func (p *Page) Close() error {
	return p.rod.Close()
}

// Rod returns the underlying rod.Page for advanced usage.
func (p *Page) Rod() *rod.Page {
	return p.rod
}

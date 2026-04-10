// Package gosurfer provides AI-powered browser automation in pure Go.
//
// It wraps headless Chrome via the Chrome DevTools Protocol (CDP) and provides
// an intelligent agent that can autonomously browse the web, similar to
// Python's Browser Use library but optimized for Go.
//
// Key features:
//   - Pure Go, no Node.js or Python dependency
//   - LLM-driven autonomous browsing (OpenAI, Anthropic, Ollama)
//   - Smart DOM extraction and serialization for LLM consumption
//   - Auto-waiting, network interception, screenshot/PDF
//   - Fits in a <100MB Docker container
package gosurfer

import (
	"fmt"
	"os"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
)

// BrowserConfig configures browser launch options.
type BrowserConfig struct {
	// Headless runs the browser without a visible window.
	Headless bool

	// ExecPath is the path to Chrome/Chromium. If empty, rod auto-detects.
	ExecPath string

	// UserDataDir persists browser state (cookies, localStorage, etc.).
	UserDataDir string

	// Proxy sets an HTTP/SOCKS proxy (e.g., "socks5://127.0.0.1:1080").
	Proxy string

	// WindowWidth sets the browser window width. Default: 1280.
	WindowWidth int

	// WindowHeight sets the browser window height. Default: 720.
	WindowHeight int

	// NoSandbox disables the Chromium sandbox (required in Docker).
	NoSandbox bool

	// Stealth enables anti-detection mode (patches navigator.webdriver,
	// spoofs plugins/WebGL, sets realistic user agent, etc.).
	Stealth bool

	// HumanMode enables maximum anti-detection: system Chrome, new headless mode,
	// stealth patches, and human-like behavior (random delays, mouse movement).
	// Automatically sets Stealth=true and uses --headless=new.
	HumanMode bool

	// AllowedDomains restricts navigation to these domains (glob patterns).
	AllowedDomains []string

	// BlockedDomains prevents navigation to these domains (glob patterns).
	BlockedDomains []string

	// ChallengeWaitTimeout controls how long Navigate() waits for an
	// auto-solvable bot-protection challenge (e.g., Cloudflare's "Just a
	// moment..." JS challenge) to clear. If 0, the default of 15s is used.
	// Set to -1 to disable auto-waiting entirely.
	ChallengeWaitTimeout time.Duration
}

// Browser wraps a Chrome/Chromium instance.
type Browser struct {
	rod    *rod.Browser
	config BrowserConfig
}

// NewBrowser launches a new Chrome/Chromium instance.
func NewBrowser(cfg ...BrowserConfig) (*Browser, error) {
	config := BrowserConfig{
		Headless:     true,
		WindowWidth:  1280,
		WindowHeight: 720,
	}
	if len(cfg) > 0 {
		config = cfg[0]
		if config.WindowWidth == 0 {
			config.WindowWidth = 1280
		}
		if config.WindowHeight == 0 {
			config.WindowHeight = 720
		}
	}

	// HumanMode implies stealth + system Chrome + new headless
	if config.HumanMode {
		config.Stealth = true
		if config.ExecPath == "" {
			config.ExecPath = findSystemChrome()
		}
	}

	l := launcher.New().
		Set("window-size", fmt.Sprintf("%d,%d", config.WindowWidth, config.WindowHeight)).
		Set("disable-gpu").
		Set("disable-dev-shm-usage")

	// Use new headless mode (same TLS/rendering fingerprint as regular Chrome)
	// Falls back to old headless if not in HumanMode
	if config.Headless {
		if config.HumanMode {
			// --headless=new uses the real Chrome rendering pipeline
			l = l.Headless(false).Set("headless", "new")
		} else {
			l = l.Headless(true)
		}
	} else {
		l = l.Headless(false)
	}

	// Stealth launch flags
	if config.Stealth {
		for flag, val := range stealthLaunchFlags() {
			if val == "" {
				l = l.Set(flags.Flag(flag))
			} else {
				l = l.Set(flags.Flag(flag), val)
			}
		}
	}

	// HumanMode: remove automation flags that bot detectors check
	if config.HumanMode {
		l = l.Delete("enable-automation")
		l = l.Delete("no-startup-window")
		l = l.Set("disable-features", "site-per-process,TranslateUI,AutomationControlled")
	}

	// Portable cookie encryption. By default Chrome encrypts cookies with a
	// key derived from the OS keyring (macOS Keychain, Gnome Keyring,
	// kwallet, etc.), producing cookies with a "v10" prefix that can ONLY be
	// decrypted on the same machine/user. This breaks profile portability:
	// a profile built on macOS (e.g., during an automated login run) cannot
	// be loaded by Chrome on Alpine Linux (e.g., in a production container)
	// because the encryption key is different — Chrome silently treats the
	// cookies as invalid and the login evaporates.
	//
	// --password-store=basic tells Chrome to use a portable obfuscation
	// scheme that does NOT depend on the OS keyring. --use-mock-keychain
	// additionally prevents Chrome from trying to touch the macOS Keychain.
	// Together, these flags make the cookie encryption deterministic
	// across machines, so a profile built anywhere can be loaded anywhere.
	l = l.Set("password-store", "basic")
	l = l.Set("use-mock-keychain")

	if config.ExecPath != "" {
		l = l.Bin(config.ExecPath)
	}
	if config.NoSandbox {
		l = l.NoSandbox(true)
	}
	if config.UserDataDir != "" {
		l = l.UserDataDir(config.UserDataDir)
	}
	if config.Proxy != "" {
		l = l.Proxy(config.Proxy)
	}

	u, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("gosurfer: launch browser: %w", err)
	}

	browser := rod.New().ControlURL(u)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("gosurfer: connect to browser: %w", err)
	}

	return &Browser{rod: browser, config: config}, nil
}

// ConnectBrowser connects to an existing Chrome instance via a CDP WebSocket URL.
func ConnectBrowser(wsURL string, cfg ...BrowserConfig) (*Browser, error) {
	config := BrowserConfig{WindowWidth: 1280, WindowHeight: 720}
	if len(cfg) > 0 {
		config = cfg[0]
	}

	browser := rod.New().ControlURL(wsURL)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("gosurfer: connect to browser: %w", err)
	}
	return &Browser{rod: browser, config: config}, nil
}

// NewPage creates a new browser tab.
func (b *Browser) NewPage() (*Page, error) {
	p, err := b.rod.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("gosurfer: new page: %w", err)
	}

	if err := p.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:             b.config.WindowWidth,
		Height:            b.config.WindowHeight,
		DeviceScaleFactor: 1,
	}); err != nil {
		return nil, fmt.Errorf("gosurfer: set viewport: %w", err)
	}

	page := &Page{rod: p, browser: b}
	page.dom = &DOMService{page: page}

	// Apply stealth scripts before any navigation
	if b.config.Stealth {
		if err := ApplyStealth(page); err != nil {
			return nil, fmt.Errorf("gosurfer: apply stealth: %w", err)
		}
		if err := applyStealthEmulation(page); err != nil {
			return nil, fmt.Errorf("gosurfer: stealth emulation: %w", err)
		}
	}

	return page, nil
}

// Pages returns all open pages/tabs.
func (b *Browser) Pages() ([]*Page, error) {
	pages, err := b.rod.Pages()
	if err != nil {
		return nil, err
	}
	result := make([]*Page, len(pages))
	for i, p := range pages {
		pg := &Page{rod: p, browser: b}
		pg.dom = &DOMService{page: pg}
		result[i] = pg
	}
	return result, nil
}

// Incognito creates an isolated browser context with separate cookies/storage.
func (b *Browser) Incognito() (*Browser, error) {
	incognito, err := b.rod.Incognito()
	if err != nil {
		return nil, fmt.Errorf("gosurfer: incognito: %w", err)
	}
	return &Browser{rod: incognito, config: b.config}, nil
}

// WaitDownload sets up a download handler that returns file bytes
// when the next download completes. Call before triggering the download.
func (b *Browser) WaitDownload() func() []byte {
	return b.rod.MustWaitDownload()
}

// HandleAuth sets up HTTP Basic authentication handling.
func (b *Browser) HandleAuth(username, password string) func() error {
	return b.rod.HandleAuth(username, password)
}

// PageByURL finds an open page whose URL matches the regex pattern.
func (b *Browser) PageByURL(urlRegex string) (*Page, error) {
	pages, err := b.rod.Pages()
	if err != nil {
		return nil, err
	}
	p, err := pages.FindByURL(urlRegex)
	if err != nil {
		return nil, fmt.Errorf("gosurfer: page by url %q: %w", urlRegex, err)
	}
	pg := &Page{rod: p, browser: b}
	pg.dom = &DOMService{page: pg}
	return pg, nil
}

// Rod returns the underlying rod.Browser for advanced usage.
func (b *Browser) Rod() *rod.Browser {
	return b.rod
}

// Close shuts down the browser.
func (b *Browser) Close() error {
	return b.rod.Close()
}

// findSystemChrome returns the path to the system Chrome installation.
// Prefers system Chrome over rod-downloaded Chromium for realistic TLS fingerprints.
func findSystemChrome() string {
	paths := []string{
		// macOS
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		// Linux
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/usr/bin/chromium-browser",
		"/usr/bin/chromium",
		// Windows (common locations)
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	}
	for _, p := range paths {
		if fileExists(p) {
			return p
		}
	}
	return "" // fall back to rod's auto-detection
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

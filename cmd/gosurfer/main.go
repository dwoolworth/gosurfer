// gosurfer CLI - persistent browser automation from the command line.
//
// Usage:
//
//	gosurfer open <url>              Navigate to URL
//	gosurfer click <selector>        Click an element
//	gosurfer type <selector> <text>  Type into an element
//	gosurfer screenshot [file]       Save screenshot (default: screenshot.png)
//	gosurfer pdf [file]              Save PDF (default: page.pdf)
//	gosurfer state                   Show DOM state (interactive elements)
//	gosurfer eval <js>               Evaluate JavaScript
//	gosurfer cookies                 List all cookies
//	gosurfer cookie <name> [value]   Get or set a cookie
//	gosurfer storage                 List all localStorage
//	gosurfer har <file>              Export HAR recording to file
//	gosurfer text <selector>         Get text content
//	gosurfer html                    Get full page HTML
//	gosurfer back                    Navigate back
//	gosurfer forward                 Navigate forward
//	gosurfer reload                  Reload page
//	gosurfer tabs                    List open tabs
//	gosurfer close                   Close browser and exit
//
// The browser stays running between commands for fast iteration.
// Set GOSURFER_HEADLESS=false to see the browser window.
// Set GOSURFER_STEALTH=true for anti-detection mode.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dwoolworth/gosurfer"
)

var (
	browser   *gosurfer.Browser
	page      *gosurfer.Page
	har       *gosurfer.HARRecorder
	lastState *gosurfer.DOMState // cached for index-based commands
)

func main() {
	// Handle single command from args
	if len(os.Args) > 1 {
		ensureBrowser()
		code := runCommand(os.Args[1], os.Args[2:])
		cleanup()
		os.Exit(code)
	}

	// Interactive REPL mode
	fmt.Println("gosurfer CLI - type 'help' for commands, 'quit' to exit")
	ensureBrowser()
	fmt.Println("Browser ready.")

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("gosurfer> ")
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			fmt.Print("gosurfer> ")
			continue
		}
		if line == "quit" || line == "exit" {
			break
		}

		parts := splitArgs(line)
		if len(parts) == 0 {
			fmt.Print("gosurfer> ")
			continue
		}

		runCommand(parts[0], parts[1:])
		fmt.Print("gosurfer> ")
	}

	cleanup()
}

func ensureBrowser() {
	if browser != nil {
		return
	}

	headless := os.Getenv("GOSURFER_HEADLESS") != "false"
	stealth := os.Getenv("GOSURFER_STEALTH") == "true"
	humanMode := os.Getenv("GOSURFER_HUMAN") == "true"
	profile := os.Getenv("GOSURFER_PROFILE")
	proxy := os.Getenv("GOSURFER_PROXY")

	var err error
	execPath := os.Getenv("CHROME_BIN")
	if execPath == "" {
		// Prefer system Chrome on macOS — rod-downloaded Chromium can hit Gatekeeper issues
		if _, err := os.Stat("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"); err == nil {
			execPath = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		}
	}

	// Auto-detect container environment for --no-sandbox
	noSandbox := os.Getenv("GOSURFER_NO_SANDBOX") == "true"
	if !noSandbox {
		// Detect Docker/K8s: /.dockerenv or /run/secrets/kubernetes.io
		if _, err := os.Stat("/.dockerenv"); err == nil {
			noSandbox = true
		} else if _, err := os.Stat("/run/secrets/kubernetes.io"); err == nil {
			noSandbox = true
		}
	}

	browser, err = gosurfer.NewBrowser(gosurfer.BrowserConfig{
		Headless:    headless,
		Stealth:     stealth,
		HumanMode:   humanMode,
		ExecPath:    execPath,
		UserDataDir: profile,
		Proxy:       proxy,
		NoSandbox:   noSandbox,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error launching browser: %v\n", err)
		os.Exit(1)
	}

	page, err = browser.NewPage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating page: %v\n", err)
		os.Exit(1)
	}
}

func cleanup() {
	if browser != nil {
		_ = browser.Close()
	}
}

func runCommand(cmd string, args []string) int {
	switch cmd {
	case "help":
		printHelp()

	case "open", "navigate", "goto":
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "Usage: open <url>")
			return 1
		}
		url := args[0]
		if !strings.Contains(url, "://") {
			url = "https://" + url
		}
		if err := page.Navigate(url); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		title, _ := page.Title()
		fmt.Printf("Navigated to: %s\nTitle: %s\n", page.URL(), title)

	case "click":
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "Usage: click <selector or index>")
			return 1
		}
		selector := args[0]
		// If it's a number, look up the element by index from last state
		if idx, err := strconv.Atoi(selector); err == nil {
			sel, err := selectorForIndex(idx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return 1
			}
			selector = sel
		}
		if err := page.Click(selector); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Println("Clicked:", selector)

	case "type":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: type <selector or index> <text>")
			return 1
		}
		selector := args[0]
		if idx, err := strconv.Atoi(selector); err == nil {
			sel, err := selectorForIndex(idx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return 1
			}
			selector = sel
		}
		text := strings.Join(args[1:], " ")
		if err := page.Type(selector, text); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Printf("Typed %q into %s\n", text, selector)

	case "screenshot":
		args = navigateIfURL(args)
		file := "screenshot.png"
		if len(args) > 0 {
			file = args[0]
		}
		png, err := page.Screenshot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		if err := os.WriteFile(file, png, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
			return 1
		}
		fmt.Printf("Screenshot saved: %s (%d bytes)\n", file, len(png))

	case "fullscreenshot", "fullshot":
		args = navigateIfURL(args)
		file := "fullpage.png"
		if len(args) > 0 {
			file = args[0]
		}
		png, err := page.FullScreenshot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		if err := os.WriteFile(file, png, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
			return 1
		}
		fmt.Printf("Full page screenshot saved: %s (%d bytes)\n", file, len(png))

	case "pdf":
		args = navigateIfURL(args)
		file := "page.pdf"
		if len(args) > 0 {
			file = args[0]
		}
		pdf, err := page.PDF()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		if err := os.WriteFile(file, pdf, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
			return 1
		}
		fmt.Printf("PDF saved: %s (%d bytes)\n", file, len(pdf))

	case "state":
		_ = navigateIfURL(args)
		state, err := page.DOMState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		lastState = state
		fmt.Printf("URL:   %s\nTitle: %s\nElements: %d\nScroll: %.0f%%\n\n",
			state.URL, state.Title, len(state.Elements), state.ScrollPosition)
		fmt.Println(state.Tree)

	case "focusedstate", "fstate":
		_ = navigateIfURL(args)
		state, err := page.FocusedDOMState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		lastState = state
		fmt.Printf("URL:   %s\nTitle: %s\nElements: %d (focused)\nScroll: %.0f%%\n\n",
			state.URL, state.Title, len(state.Elements), state.ScrollPosition)
		fmt.Println(state.Tree)

	case "eval":
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "Usage: eval <javascript>")
			return 1
		}
		js := strings.Join(args, " ")
		if !strings.HasPrefix(js, "()") {
			js = "() => " + js
		}
		val, err := page.Eval(js)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Println(val)

	case "text":
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "Usage: text <selector>")
			return 1
		}
		text, err := page.Text(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Println(text)

	case "html":
		html, err := page.HTML()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Println(html)

	case "cookies":
		cookies, err := page.GetCookies()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		if len(cookies) == 0 {
			fmt.Println("No cookies")
			return 0
		}
		for _, c := range cookies {
			fmt.Printf("  %-30s = %s  (domain: %s)\n", c.Name, truncateCLI(c.Value, 50), c.Domain)
		}
		fmt.Printf("\n%d cookies total\n", len(cookies))

	case "cookie":
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "Usage: cookie <name> [value]")
			return 1
		}
		if len(args) >= 2 {
			// Set cookie
			if err := page.SetCookie(args[0], args[1], "", ""); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return 1
			}
			fmt.Printf("Cookie set: %s = %s\n", args[0], args[1])
		} else {
			// Get cookie
			val, err := page.GetCookie(args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return 1
			}
			if val == "" {
				fmt.Printf("Cookie %q not found\n", args[0])
			} else {
				fmt.Printf("%s = %s\n", args[0], val)
			}
		}

	case "storage":
		items, err := page.LocalStorageAll()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		if len(items) == 0 {
			fmt.Println("localStorage is empty")
			return 0
		}
		for k, v := range items {
			fmt.Printf("  %-30s = %s\n", k, truncateCLI(v, 60))
		}
		fmt.Printf("\n%d items total\n", len(items))

	case "har":
		if har == nil {
			// Start recording
			har = page.StartHAR()
			fmt.Println("HAR recording started. Use 'har <file>' again to save.")
			return 0
		}
		if len(args) < 1 {
			fmt.Printf("HAR recording active: %d entries captured\n", har.Entries())
			return 0
		}
		data, err := har.Export()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		if err := os.WriteFile(args[0], data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Printf("HAR saved: %s (%d entries, %d bytes)\n", args[0], har.Entries(), len(data))
		har = nil

	case "back":
		if err := page.Back(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Println("Navigated back")

	case "forward":
		if err := page.Forward(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Println("Navigated forward")

	case "reload":
		if err := page.Reload(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Println("Reloaded")

	case "tabs":
		pages, err := browser.Pages()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		for i, p := range pages {
			title, _ := p.Title()
			marker := "  "
			if p.URL() == page.URL() {
				marker = "> "
			}
			fmt.Printf("%s[%d] %s - %s\n", marker, i, title, p.URL())
		}

	case "close":
		cleanup()
		fmt.Println("Browser closed")
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s (type 'help' for commands)\n", cmd)
		return 1
	}

	return 0
}

func printHelp() {
	fmt.Println(`gosurfer CLI commands:

  Navigation:
    open <url>              Navigate to URL (auto-adds https://)
    back                    Navigate back in history
    forward                 Navigate forward
    reload                  Reload current page
    tabs                    List open tabs

  Interaction:
    click <selector|index>  Click by CSS selector or state index (e.g. click 56)
    type <selector|index> <text>  Type text by selector or index
    eval <js>               Evaluate JavaScript expression

  Extraction:
    state                   Show full indexed DOM state
    focusedstate            Show content-focused DOM (no nav/footer/junk links)
    text <selector>         Get text content of element
    html                    Get full page HTML
    screenshot [file]       Save viewport screenshot (default: screenshot.png)
    fullscreenshot [file]   Save full page screenshot (default: fullpage.png)
    pdf [file]              Save PDF (default: page.pdf)

  Storage:
    cookies                 List all cookies
    cookie <name> [value]   Get or set a cookie
    storage                 List all localStorage items

  Recording:
    har                     Start HAR recording (first call)
    har <file>              Save HAR and stop recording (second call)

  Other:
    help                    Show this help
    close / quit / exit     Close browser and exit

  Environment:
    GOSURFER_HEADLESS=false   Show browser window
    GOSURFER_STEALTH=true     Enable anti-detection mode
    GOSURFER_HUMAN=true       Maximum anti-detection (system Chrome + new headless + stealth)
    GOSURFER_PROFILE=/path    Use Chrome profile directory (persists login state)
    GOSURFER_PROXY=host:port  Route traffic through HTTP/SOCKS proxy
    CHROME_BIN=/path/chrome   Custom Chrome path`)
}

func splitArgs(line string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)
	hasQuote := false

	for i := 0; i < len(line); i++ {
		c := line[i]
		if inQuote {
			if c == quoteChar {
				inQuote = false
			} else {
				current.WriteByte(c)
			}
		} else if c == '"' || c == '\'' {
			inQuote = true
			hasQuote = true
			quoteChar = c
		} else if c == ' ' || c == '\t' {
			if current.Len() > 0 || hasQuote {
				args = append(args, current.String())
				current.Reset()
				hasQuote = false
			}
		} else {
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 || hasQuote {
		args = append(args, current.String())
	}
	return args
}

// fileExtensions are common file extensions that should not be treated as URLs.
var fileExtensions = []string{
	".png", ".jpg", ".jpeg", ".gif", ".pdf", ".html", ".json", ".har",
	".txt", ".csv", ".xml", ".zip", ".tar", ".gz",
}

// navigateIfURL checks if the first arg looks like a URL, navigates to it,
// and returns the remaining args. Used by commands that support "cmd <url>".
func navigateIfURL(args []string) []string {
	if len(args) == 0 {
		return args
	}
	arg := args[0]

	// Explicit protocol — definitely a URL
	if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
		if err := page.Navigate(arg); err != nil {
			fmt.Fprintf(os.Stderr, "Error navigating: %v\n", err)
		}
		return args[1:]
	}

	// Skip file paths and extensions
	if strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, ".") {
		return args
	}
	for _, ext := range fileExtensions {
		if strings.HasSuffix(strings.ToLower(arg), ext) {
			return args
		}
	}

	// Looks like a domain (has a dot, no spaces)
	if strings.Contains(arg, ".") && !strings.Contains(arg, " ") {
		url := "https://" + arg
		if err := page.Navigate(url); err != nil {
			fmt.Fprintf(os.Stderr, "Error navigating: %v\n", err)
		}
		return args[1:]
	}

	return args
}

// selectorForIndex looks up the CSS selector for a DOM element index from the last state.
func selectorForIndex(idx int) (string, error) {
	if lastState == nil {
		return "", fmt.Errorf("run 'state' first to index page elements")
	}
	for _, el := range lastState.Elements {
		if el.Index == idx {
			if el.CSSSelector == "" {
				return "", fmt.Errorf("element [%d] has no CSS selector", idx)
			}
			return el.CSSSelector, nil
		}
	}
	return "", fmt.Errorf("element [%d] not found (max index: %d)", idx, len(lastState.Elements)-1)
}

func truncateCLI(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

# gosurfer

AI-powered browser automation in pure Go. The Go equivalent of Python's [Browser Use](https://github.com/browser-use/browser-use) — wraps headless Chrome via the Chrome DevTools Protocol with an intelligent agent that autonomously browses the web.

No Python. No Node.js. One static binary.

```go
// AI agent that completes tasks autonomously
agent, _ := gosurfer.NewAgent(gosurfer.AgentConfig{
    Task:    "Find the price of a mass produced mass driver on Alibaba",
    LLM:     gosurfer.NewOpenAI(os.Getenv("OPENAI_API_KEY"), "gpt-4o"),
    Stealth: true,
})
result, _ := agent.Run(ctx)
fmt.Println(result.Output)
```

```go
// Or use it as a direct browser automation library
browser, _ := gosurfer.NewBrowser(gosurfer.BrowserConfig{Headless: true, Stealth: true})
page, _ := browser.NewPage()
page.Navigate("https://example.com")
page.Type("#search", "query")
page.Click("#submit")
state, _ := page.DOMState() // indexed DOM optimized for LLMs
```

## Why gosurfer?

| | gosurfer | Browser Use | Playwright |
|---|---|---|---|
| Language | **Go** | Python | Node.js / Python / Java |
| Binary size | **4 MB** (UPX) | ~100 MB runtime | ~200 MB runtime |
| Docker image | **~945 MB** | ~2-3 GB | ~1.5-2 GB |
| Idle memory | **~530 MB** | ~800+ MB | ~700+ MB |
| Peak memory | **~1.1 GB** | ~2+ GB | ~1.5+ GB |
| LLM agent | Yes | Yes | No (separate layer) |
| CAPTCHA solving | Yes | Yes (cloud) | No |
| Stealth mode | Yes (12 vectors) | Yes (cloud + local) | No |
| TOTP 2FA | Yes | Yes | No |
| Dependencies | 1 (rod) | ~50+ packages | ~30+ packages |
| Startup time | **~665 ms** (container) | ~3-5 s | ~1-2 s |

## Memory Profile (Docker container benchmark)

Measured with `go run ./examples/benchmark/` inside an Alpine container:

```
Stage                                  Go Heap     Go Sys     Chrome      Total
-----------------------------------   --------   --------   --------   --------
Baseline (before browser)                0.4 MB      6.4 MB      0.0 MB      6.4 MB
After browser launch                     4.2 MB     15.5 MB    517.1 MB    532.6 MB
After navigation (HN)                    2.4 MB     15.5 MB    570.4 MB    585.9 MB
After DOM extraction                     2.5 MB     15.7 MB    577.6 MB    593.3 MB
After heavy page (Wikipedia)             6.1 MB     15.7 MB    874.5 MB    890.2 MB
After full screenshot                   13.6 MB     40.0 MB   1078.8 MB   1118.8 MB
After GC                                 0.6 MB     40.0 MB    929.9 MB    969.8 MB
```

Go itself uses **0.6-16 MB heap**. Chrome dominates, as it does in every browser automation tool.

## Installation

```bash
go get github.com/dwoolworth/gosurfer
```

Requires Chrome or Chromium. On first run, [rod](https://github.com/go-rod/rod) auto-downloads a compatible Chromium if none is found.

## Features

### AI Agent (Browser Use equivalent)

The agent takes a natural language task, launches a browser, and autonomously figures out how to complete it:

```go
agent, err := gosurfer.NewAgent(gosurfer.AgentConfig{
    Task:      "Search for 'Go programming' and summarize the top 3 results",
    LLM:       gosurfer.NewAnthropic(apiKey, "claude-sonnet-4-20250514"),
    Headless:  true,
    Stealth:   true,
    UseVision: true,  // include screenshots in LLM context
    MaxSteps:  20,
    Verbose:   true,
    OnStep: func(info gosurfer.StepInfo) {
        fmt.Printf("[Step %d] %s -> %s\n", info.Step, info.Action, info.Result)
    },
})

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()

result, err := agent.Run(ctx)
fmt.Printf("Success: %v\nOutput: %s\nSteps: %d\nTokens: %d\n",
    result.Success, result.Output, result.Steps, result.TotalTokens.TotalTokens)
```

### 21 Built-in Agent Actions

| Action | Description |
|--------|-------------|
| `navigate` | Go to a URL |
| `click` | Click element by index OR (x,y) viewport coordinates |
| `type` | Type text into inputs (with `{{secret}}` placeholder support) |
| `scroll` | Scroll page or specific element up/down |
| `search` | Web search via Google, DuckDuckGo, or Bing |
| `go_back` | Browser history back |
| `wait` | Pause 1-10 seconds |
| `screenshot` | Capture viewport |
| `extract` | Extract page content with a query |
| `send_keys` | Keyboard events (Enter, Escape, Tab) |
| `select_option` | Choose from dropdowns |
| `switch_tab` | Switch between browser tabs |
| `close_tab` | Close a tab |
| `new_tab` | Open URL in new tab |
| `upload_file` | Upload file to input |
| `get_cookies` | Retrieve all cookies for current page |
| `set_cookie` | Set a cookie (name, value, domain) |
| `get_storage` | Read localStorage values |
| `set_storage` | Write localStorage values |
| `drag` | Drag element to another element or coordinates |
| `done` | Signal task completion with result |

### LLM Providers

```go
// OpenAI
llm := gosurfer.NewOpenAI("sk-...", "gpt-4o")

// Anthropic
llm := gosurfer.NewAnthropic("sk-ant-...", "claude-sonnet-4-20250514")

// Ollama (local)
llm := gosurfer.NewOllama("llama3.1")

// Any OpenAI-compatible API (vLLM, Together, etc.)
llm := gosurfer.NewOpenAICompatible("https://api.together.xyz/v1", "key", "model")
```

### DOM Extraction for LLMs

The key innovation from Browser Use, implemented in Go. `DOMState()` extracts the page into an indexed format that LLMs can reason about:

```go
state, _ := page.DOMState()
fmt.Println(state.Tree)
```

Output:
```
[0]<a href="https://news.ycombinator.com" />
  [1]<img />
    [2]<a href="news">Hacker News</a>
  [3]<a href="newest">new</a>
  [4]<a href="front">past</a>
  [5]<input type="text" name="q" placeholder="Search..." />
  [6]<button type="submit">Search</button>
1.
  [7]<a href="https://example.com">First Story Title</a>
    (example.com)
  [8]<a href="vote?id=123">upvote</a>
```

Interactive elements get `[index]` tags. The LLM says `{"action":"click","params":{"index":7}}` and gosurfer clicks it. Non-interactive text provides context. Shadow DOM is pierced with `|SHADOW|` markers, iframes with `|IFRAME|`.

The `DOMState` struct also includes:
- Element metadata (tag, attributes, bounding box, CSS selector)
- Tab list (ID, URL, title for all open tabs)
- Scroll position, page height, viewport height
- Optional JPEG screenshot

### Stealth Mode (Anti-Detection)

12 evasion vectors ported from [puppeteer-extra-plugin-stealth](https://github.com/nicedayfor/puppeteer-extra-plugin-stealth):

```go
browser, _ := gosurfer.NewBrowser(gosurfer.BrowserConfig{
    Headless: true,
    Stealth:  true,  // enables all evasions
})
```

What it patches:
1. `navigator.webdriver` removed
2. `window.chrome` runtime emulated
3. `chrome.loadTimes` / `chrome.csi` added
4. `navigator.plugins` populated (3 realistic plugins)
5. `navigator.languages` set to `[en-US, en]`
6. Permissions API fixed (notification quirk)
7. Window outer dimensions matched to inner
8. `navigator.hardwareConcurrency` set to 4
9. `navigator.deviceMemory` set to 8GB
10. WebGL vendor/renderer spoofed (Intel Iris)
11. Media devices enumerated
12. `Function.prototype.toString` patched to return `[native code]`

Plus Chrome launch flags: `--disable-blink-features=AutomationControlled`

### CAPTCHA Detection and Solving

Detects reCAPTCHA v2/v3, hCaptcha, and Cloudflare Turnstile automatically:

```go
// Detect
info, _ := page.DetectCAPTCHA()
// info.Type: "recaptcha_v2", "recaptcha_v3", "hcaptcha", "turnstile"
// info.SiteKey: extracted from page

// Solve with 2Captcha
solver := gosurfer.NewTwoCaptchaSolver("your-2captcha-api-key")
page.SolveCAPTCHA(ctx, solver)

// Or CapSolver
solver := gosurfer.NewCapSolver("your-capsolver-api-key")

// Or custom callback
solver := &gosurfer.ManualCAPTCHASolver{
    SolveFunc: func(ctx context.Context, info gosurfer.CAPTCHAInfo) (string, error) {
        // Your custom solving logic
        return token, nil
    },
}
```

In the agent, CAPTCHAs are solved automatically:
```go
agent, _ := gosurfer.NewAgent(gosurfer.AgentConfig{
    Task:          "Login to example.com",
    LLM:           llm,
    CAPTCHASolver: gosurfer.NewTwoCaptchaSolver(apiKey),
})
```

### TOTP 2FA Auto-Generation

Secret keys ending in `_totp` automatically generate fresh TOTP codes:

```go
agent, _ := gosurfer.NewAgent(gosurfer.AgentConfig{
    Task: "Login to my account",
    LLM:  llm,
    Secrets: map[string]string{
        "username":  "admin",
        "password":  "s3cret",
        "mfa_totp":  "JBSWY3DPEHPK3PXP",  // base32 TOTP secret
    },
})
// When the agent types {{mfa_totp}}, a fresh 6-digit code is generated
```

Or use directly:
```go
code, _ := gosurfer.GenerateTOTP("JBSWY3DPEHPK3PXP")
// "482913" (changes every 30 seconds)
```

### Tab Management

The agent automatically detects new tabs and can switch between them:

```go
// Tabs are listed in DOMState
state, _ := page.DOMState()
for _, tab := range state.Tabs {
    fmt.Printf("[%s] %s - %s\n", tab.ID, tab.Title, tab.URL)
}

// Agent actions: switch_tab, close_tab, new_tab
// LLM sees tab list and can navigate between them
```

New tabs opened by `target="_blank"` links are auto-detected and switched to.

### Network Interception

```go
interceptor := page.Intercept()
interceptor.OnRequest(`.*analytics.*`, func(req *gosurfer.InterceptedRequest) {
    req.Abort() // block analytics
})
interceptor.BlockPatterns(`.*\.ads\..*`, `.*tracker.*`)
interceptor.Start()
defer interceptor.Stop()
```

### Dialog Handling

JavaScript `alert()`, `confirm()`, and `prompt()` dialogs are auto-dismissed by the agent. For manual control:

```go
// Auto-dismiss all dialogs
cancel := page.AutoDismissDialogs()
defer cancel()

// Or handle manually
wait, handle := page.HandleDialog()
go func() {
    dialog := wait()
    fmt.Println(dialog.Type, dialog.Message)
    handle(true, "") // accept
}()
```

### Cookies and Storage

Full cookie and localStorage/sessionStorage management:

```go
// Cookies
cookies, _ := page.GetCookies()
cookie, _ := page.GetCookie("session_id")
page.SetCookie("token", "abc123", ".example.com", "/")
page.DeleteCookies("token")
page.ClearCookies()

// localStorage
page.LocalStorageSet("key", "value")
val, _ := page.LocalStorageGet("key")
page.LocalStorageDelete("key")
page.LocalStorageClear()

// sessionStorage
page.SessionStorageSet("key", "value")
val, _ = page.SessionStorageGet("key")
```

### Drag and Drop

```go
// Element-to-element drag
source, _ := page.Element("#draggable")
target, _ := page.Element("#droppable")
source.DragTo(target)

// Element to coordinates
source.DragToCoordinates(300, 400)

// Coordinate-based drag
page.DragDrop(100, 200, 300, 400)
```

### HAR Recording

Record network traffic in HAR 1.2 format for debugging or analysis:

```go
recorder, _ := page.StartHAR()
page.Navigate("https://example.com")
// ... interact with page ...

data, _ := recorder.Export() // HAR 1.2 JSON bytes
fmt.Printf("Captured %d requests\n", recorder.Entries())
```

### Browser Context Isolation

```go
incognito, _ := browser.Incognito()
defer incognito.Close()
page, _ := incognito.NewPage() // isolated cookies, storage
```

## CLI

gosurfer includes an interactive command-line tool for browser automation:

```bash
# Install
go install github.com/dwoolworth/gosurfer/cmd/gosurfer@latest

# Single command
gosurfer open https://example.com
gosurfer screenshot page.png

# Interactive REPL
gosurfer
gosurfer> open https://news.ycombinator.com
gosurfer> state
gosurfer> click "a.storylink"
gosurfer> screenshot hn.png
gosurfer> cookies
gosurfer> har traffic.har
gosurfer> close
```

Commands: `open`, `click`, `type`, `screenshot`, `pdf`, `state`, `eval`, `cookies`, `cookie`, `storage`, `har`, `text`, `html`, `back`, `forward`, `reload`, `tabs`, `close`.

Set `GOSURFER_HEADLESS=false` to see the browser window, `GOSURFER_STEALTH=true` for anti-detection mode.

## Docker

```dockerfile
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git upx
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/server . \
    && upx --best --lzma /app/server

FROM alpine:3.20
RUN apk add --no-cache chromium nss freetype harfbuzz ca-certificates ttf-freefont
ENV CHROME_BIN=/usr/bin/chromium-browser
RUN adduser -D app
USER app
COPY --from=builder /app/server .
CMD ["./server"]
```

For Kubernetes, set resource limits based on the benchmark:
```yaml
resources:
  requests:
    memory: "512Mi"
    cpu: "250m"
  limits:
    memory: "1.5Gi"  # Chrome can spike during heavy pages
    cpu: "1000m"
```

## Architecture

```
gosurfer
├── browser.go      Browser lifecycle, launch, stealth flags
├── page.go         Page navigation, interaction, dialogs, popups
├── element.go      Element handles, click/type/select, shadow DOM, iframes
├── dom.go          DOM extraction + LLM serialization (the key innovation)
├── agent.go        AI agent loop with CAPTCHA auto-solve, loop detection
├── action.go       21 agent actions + custom action registry
├── llm.go          OpenAI, Anthropic, Ollama providers (raw net/http)
├── stealth.go      12-vector anti-detection (JS injection + Chrome flags)
├── captcha.go      Detection + solving (2Captcha, CapSolver, manual)
├── totp.go         RFC 6238 TOTP + secrets management
├── network.go      Request interception and blocking
├── storage.go      Cookie + localStorage/sessionStorage management
├── drag.go         Drag and drop operations
├── har.go          HAR 1.2 network traffic recording
├── prompt.go       Agent system prompt generation
└── cmd/gosurfer/   CLI entry point
```

### How the Agent Works

Each step:
1. Extract DOM state (+ optional screenshot)
2. Check for CAPTCHAs, auto-solve if solver configured
3. Build LLM prompt: system instructions + action history + current DOM
4. Call LLM, parse JSON response: `{"thought":"...","action":"click","params":{"index":5}}`
5. Execute the action via CDP
6. Detect new tabs, check for loops
7. Repeat until `done` action or max steps

The agent includes:
- **Context summarization**: For long tasks, older steps are automatically summarized by the LLM into a running narrative, so the agent retains awareness of earlier actions, extracted data, and progress even beyond the 5-step recent history window. Enabled by default; disable with `DisableSummary: true`
- **Loop detection**: Watches for repeating action patterns and nudges the LLM to try different approaches
- **Auto tab switching**: Detects `target="_blank"` clicks and follows to the new tab
- **Message compaction**: Keeps the last 5 steps verbatim, with LLM-generated summaries of older steps injected into the system prompt
- **Secret replacement**: `{{placeholder}}` in typed text is replaced with actual values (TOTP codes generated fresh)

### Built on Rod

gosurfer wraps [go-rod/rod](https://github.com/go-rod/rod), the best Go CDP library. Rod provides:
- Auto-waiting before interactions
- Chrome lifecycle management
- Network hijacking via CDP Fetch domain
- Iframe and shadow DOM traversal

gosurfer adds the AI agent layer, DOM serialization, stealth, CAPTCHA solving, and TOTP on top.

## Examples

### AI Search Agent
```bash
export OPENAI_API_KEY=sk-...
go run ./examples/search/ "What is the population of Tokyo?"
```

### Direct Scraping
```bash
go run ./examples/scrape/
```

### Memory Benchmark
```bash
# Local
go run ./examples/benchmark/

# In Docker (realistic Kubernetes numbers)
docker build -f examples/benchmark/Dockerfile -t gosurfer-bench .
docker run --rm gosurfer-bench
```

## Test Coverage

76.6% statement coverage across 14 test files (4,820 lines of tests). Integration tests use a shared headless browser with an `httptest` server:

```bash
go test -timeout 180s ./...
```

## License

MIT License. Concept and design by [Derrick Woolworth](https://github.com/dwoolworth). Implementation by [Claude](https://claude.ai) (Anthropic).

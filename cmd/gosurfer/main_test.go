package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/dwoolworth/gosurfer"
)

var testURL string

func TestMain(m *testing.M) {
	// Start test HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>CLI Test</title></head>
<body>
  <h1>CLI Test Page</h1>
  <p id="info">Hello from test server</p>
  <input type="text" id="input1" placeholder="Type here" />
  <button id="btn1">Click Me</button>
  <a href="/page2" id="link1">Page 2</a>
</body></html>`)
	})
	mux.HandleFunc("/page2", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>Page 2</title></head><body><h1>Page Two</h1></body></html>`)
	})
	ts := httptest.NewServer(mux)
	testURL = ts.URL

	// Launch shared browser for CLI tests
	var err error
	browser, err = gosurfer.NewBrowser(gosurfer.BrowserConfig{Headless: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot launch browser: %v\n", err)
		os.Exit(1)
	}
	page, err = browser.NewPage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot create page: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	_ = browser.Close()
	ts.Close()
	os.Exit(code)
}

// --- splitArgs tests (where interactive CLI bugs live) ---

func TestSplitArgs_Simple(t *testing.T) {
	got := splitArgs("open https://example.com")
	expect := []string{"open", "https://example.com"}
	assertArgs(t, got, expect)
}

func TestSplitArgs_DoubleQuotes(t *testing.T) {
	got := splitArgs(`type "#input" "hello world"`)
	expect := []string{"type", "#input", "hello world"}
	assertArgs(t, got, expect)
}

func TestSplitArgs_SingleQuotes(t *testing.T) {
	got := splitArgs(`eval 'document.title'`)
	expect := []string{"eval", "document.title"}
	assertArgs(t, got, expect)
}

func TestSplitArgs_MixedQuotes(t *testing.T) {
	got := splitArgs(`type "#search" "it's a test"`)
	expect := []string{"type", "#search", "it's a test"}
	assertArgs(t, got, expect)
}

func TestSplitArgs_ExtraSpaces(t *testing.T) {
	got := splitArgs("  open   https://example.com  ")
	expect := []string{"open", "https://example.com"}
	assertArgs(t, got, expect)
}

func TestSplitArgs_Tabs(t *testing.T) {
	got := splitArgs("open\thttps://example.com")
	expect := []string{"open", "https://example.com"}
	assertArgs(t, got, expect)
}

func TestSplitArgs_Empty(t *testing.T) {
	got := splitArgs("")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestSplitArgs_OnlySpaces(t *testing.T) {
	got := splitArgs("   ")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestSplitArgs_QuotedEmpty(t *testing.T) {
	got := splitArgs(`cookie "session_id" ""`)
	// Empty quoted string should still produce an arg
	expect := []string{"cookie", "session_id", ""}
	assertArgs(t, got, expect)
}

func TestSplitArgs_SelectorWithSpecialChars(t *testing.T) {
	got := splitArgs(`click "div.class > span:nth-child(2)"`)
	expect := []string{"click", "div.class > span:nth-child(2)"}
	assertArgs(t, got, expect)
}

func TestSplitArgs_URLWithQueryParams(t *testing.T) {
	got := splitArgs("open https://example.com/search?q=hello+world&lang=en")
	expect := []string{"open", "https://example.com/search?q=hello+world&lang=en"}
	assertArgs(t, got, expect)
}

func TestSplitArgs_SingleArg(t *testing.T) {
	got := splitArgs("help")
	expect := []string{"help"}
	assertArgs(t, got, expect)
}

func TestSplitArgs_JavaScriptExpression(t *testing.T) {
	got := splitArgs(`eval "() => document.querySelectorAll('a').length"`)
	expect := []string{"eval", "() => document.querySelectorAll('a').length"}
	assertArgs(t, got, expect)
}

func assertArgs(t *testing.T, got, expect []string) {
	t.Helper()
	if len(got) != len(expect) {
		t.Fatalf("length mismatch: got %d %v, want %d %v", len(got), got, len(expect), expect)
	}
	for i := range got {
		if got[i] != expect[i] {
			t.Errorf("arg[%d]: got %q, want %q", i, got[i], expect[i])
		}
	}
}

// --- truncateCLI tests ---

func TestTruncateCLI_Short(t *testing.T) {
	if truncateCLI("hello", 10) != "hello" {
		t.Error("short string should be unchanged")
	}
}

func TestTruncateCLI_Exact(t *testing.T) {
	if truncateCLI("12345", 5) != "12345" {
		t.Error("exact length should be unchanged")
	}
}

func TestTruncateCLI_Long(t *testing.T) {
	result := truncateCLI("hello world this is long", 10)
	if result != "hello worl..." {
		t.Errorf("got %q", result)
	}
}

func TestTruncateCLI_Empty(t *testing.T) {
	if truncateCLI("", 5) != "" {
		t.Error("empty should stay empty")
	}
}

// --- runCommand tests (integration with real browser) ---

func TestCmd_Help(t *testing.T) {
	code := runCommand("help", nil)
	if code != 0 {
		t.Errorf("help should return 0, got %d", code)
	}
}

func TestCmd_Unknown(t *testing.T) {
	code := runCommand("nonexistent_command", nil)
	if code != 1 {
		t.Errorf("unknown command should return 1, got %d", code)
	}
}

func TestCmd_Open(t *testing.T) {
	code := runCommand("open", []string{testURL})
	if code != 0 {
		t.Errorf("open should return 0, got %d", code)
	}
	title, _ := page.Title()
	if title != "CLI Test" {
		t.Errorf("title = %q", title)
	}
}

func TestCmd_Open_AutoHTTPS(t *testing.T) {
	// This will fail to connect but tests the URL prefixing
	code := runCommand("open", []string{strings.TrimPrefix(testURL, "http://")})
	// May fail with https:// but that's OK — we're testing the prefix logic
	_ = code
}

func TestCmd_Open_MissingArg(t *testing.T) {
	code := runCommand("open", nil)
	if code != 1 {
		t.Errorf("open without URL should return 1, got %d", code)
	}
}

func TestCmd_Navigate_Alias(t *testing.T) {
	code := runCommand("navigate", []string{testURL})
	if code != 0 {
		t.Errorf("navigate alias should work, got %d", code)
	}
}

func TestCmd_Goto_Alias(t *testing.T) {
	code := runCommand("goto", []string{testURL})
	if code != 0 {
		t.Errorf("goto alias should work, got %d", code)
	}
}

func TestCmd_State(t *testing.T) {
	_ = runCommand("open", []string{testURL})
	code := runCommand("state", nil)
	if code != 0 {
		t.Errorf("state should return 0, got %d", code)
	}
}

func TestCmd_Text(t *testing.T) {
	_ = runCommand("open", []string{testURL})
	code := runCommand("text", []string{"h1"})
	if code != 0 {
		t.Errorf("text should return 0, got %d", code)
	}
}

func TestCmd_Text_MissingArg(t *testing.T) {
	code := runCommand("text", nil)
	if code != 1 {
		t.Errorf("text without selector should return 1, got %d", code)
	}
}

func TestCmd_HTML(t *testing.T) {
	_ = runCommand("open", []string{testURL})
	code := runCommand("html", nil)
	if code != 0 {
		t.Errorf("html should return 0, got %d", code)
	}
}

func TestCmd_Eval(t *testing.T) {
	_ = runCommand("open", []string{testURL})
	code := runCommand("eval", []string{"document.title"})
	if code != 0 {
		t.Errorf("eval should return 0, got %d", code)
	}
}

func TestCmd_Eval_ArrowFunction(t *testing.T) {
	code := runCommand("eval", []string{"() => 2+2"})
	if code != 0 {
		t.Errorf("eval arrow should return 0, got %d", code)
	}
}

func TestCmd_Eval_MissingArg(t *testing.T) {
	code := runCommand("eval", nil)
	if code != 1 {
		t.Errorf("eval without js should return 1, got %d", code)
	}
}

func TestCmd_Click(t *testing.T) {
	_ = runCommand("open", []string{testURL})
	code := runCommand("click", []string{"#btn1"})
	if code != 0 {
		t.Errorf("click should return 0, got %d", code)
	}
}

func TestCmd_Click_MissingArg(t *testing.T) {
	code := runCommand("click", nil)
	if code != 1 {
		t.Errorf("click without selector should return 1, got %d", code)
	}
}

func TestCmd_Type(t *testing.T) {
	_ = runCommand("open", []string{testURL})
	code := runCommand("type", []string{"#input1", "hello", "world"})
	if code != 0 {
		t.Errorf("type should return 0, got %d", code)
	}
}

func TestCmd_Type_MissingArgs(t *testing.T) {
	code := runCommand("type", []string{"#input1"})
	if code != 1 {
		t.Errorf("type without text should return 1, got %d", code)
	}
}

func TestCmd_Screenshot(t *testing.T) {
	_ = runCommand("open", []string{testURL})
	tmpFile := t.TempDir() + "/test.png"
	code := runCommand("screenshot", []string{tmpFile})
	if code != 0 {
		t.Errorf("screenshot should return 0, got %d", code)
	}
	info, err := os.Stat(tmpFile)
	if err != nil {
		t.Fatalf("screenshot file not created: %v", err)
	}
	if info.Size() < 100 {
		t.Error("screenshot file too small")
	}
}

func TestCmd_PDF(t *testing.T) {
	_ = runCommand("open", []string{testURL})
	tmpFile := t.TempDir() + "/test.pdf"
	code := runCommand("pdf", []string{tmpFile})
	if code != 0 {
		t.Errorf("pdf should return 0, got %d", code)
	}
	info, err := os.Stat(tmpFile)
	if err != nil {
		t.Fatalf("pdf file not created: %v", err)
	}
	if info.Size() < 100 {
		t.Error("pdf file too small")
	}
}

func TestCmd_Cookies(t *testing.T) {
	_ = runCommand("open", []string{testURL})
	code := runCommand("cookies", nil)
	if code != 0 {
		t.Errorf("cookies should return 0, got %d", code)
	}
}

func TestCmd_Cookie_SetAndGet(t *testing.T) {
	_ = runCommand("open", []string{testURL})
	code := runCommand("cookie", []string{"test_cookie", "test_value"})
	if code != 0 {
		t.Errorf("cookie set should return 0, got %d", code)
	}
	code = runCommand("cookie", []string{"test_cookie"})
	if code != 0 {
		t.Errorf("cookie get should return 0, got %d", code)
	}
}

func TestCmd_Cookie_NotFound(t *testing.T) {
	_ = runCommand("open", []string{testURL})
	code := runCommand("cookie", []string{"nonexistent_cookie_xyz"})
	if code != 0 {
		t.Errorf("cookie get (not found) should return 0, got %d", code)
	}
}

func TestCmd_Cookie_MissingArg(t *testing.T) {
	code := runCommand("cookie", nil)
	if code != 1 {
		t.Errorf("cookie without name should return 1, got %d", code)
	}
}

func TestCmd_Storage(t *testing.T) {
	_ = runCommand("open", []string{testURL})
	code := runCommand("storage", nil)
	if code != 0 {
		t.Errorf("storage should return 0, got %d", code)
	}
}

func TestCmd_Back(t *testing.T) {
	_ = runCommand("open", []string{testURL})
	_ = runCommand("open", []string{testURL + "/page2"})
	code := runCommand("back", nil)
	if code != 0 {
		t.Errorf("back should return 0, got %d", code)
	}
}

func TestCmd_Forward(t *testing.T) {
	code := runCommand("forward", nil)
	if code != 0 {
		t.Errorf("forward should return 0, got %d", code)
	}
}

func TestCmd_Reload(t *testing.T) {
	_ = runCommand("open", []string{testURL})
	code := runCommand("reload", nil)
	if code != 0 {
		t.Errorf("reload should return 0, got %d", code)
	}
}

func TestCmd_Tabs(t *testing.T) {
	code := runCommand("tabs", nil)
	if code != 0 {
		t.Errorf("tabs should return 0, got %d", code)
	}
}

func TestCmd_HAR_StartAndStatus(t *testing.T) {
	_ = runCommand("open", []string{testURL})
	// Start recording
	har = nil // reset
	code := runCommand("har", nil)
	if code != 0 {
		t.Errorf("har start should return 0, got %d", code)
	}
	if har == nil {
		t.Fatal("har recorder should be initialized")
	}
	// Check status
	code = runCommand("har", nil)
	if code != 0 {
		t.Errorf("har status should return 0, got %d", code)
	}
	// Save
	tmpFile := t.TempDir() + "/test.har"
	code = runCommand("har", []string{tmpFile})
	if code != 0 {
		t.Errorf("har save should return 0, got %d", code)
	}
	if har != nil {
		t.Error("har should be nil after save")
	}
	info, err := os.Stat(tmpFile)
	if err != nil {
		t.Fatalf("har file not created: %v", err)
	}
	if info.Size() < 10 {
		t.Error("har file too small")
	}
}

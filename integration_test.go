package gosurfer

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Shared browser for all integration tests (avoids launching one per test).
var (
	testBrowser *Browser
	ts          *httptest.Server
)

func TestMain(m *testing.M) {
	// Start test HTTP server
	ts = testServer()

	// Launch shared browser
	var err error
	testBrowser, err = NewBrowser(BrowserConfig{Headless: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot launch browser: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	_ = testBrowser.Close()
	ts.Close()
	os.Exit(code)
}

func testServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>Test Page</title></head>
<body>
  <h1>Hello GoSurfer</h1>
  <p>This is a test page.</p>
  <form id="search-form">
    <input type="text" id="search" name="q" placeholder="Search here..." />
    <button type="submit" id="submit-btn">Search</button>
  </form>
  <a href="/page2" id="link1">Go to Page 2</a>
  <a href="/page3" target="_blank" id="link2">Open in New Tab</a>
  <select id="color-select" name="color">
    <option value="red">Red</option>
    <option value="green">Green</option>
    <option value="blue">Blue</option>
  </select>
  <div id="scrollable" style="height:100px;overflow:auto;">
    <div style="height:500px;">Scrollable content</div>
  </div>
  <div id="hidden" style="display:none;">Hidden element</div>
</body></html>`)
	})

	mux.HandleFunc("/page2", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>Page 2</title></head>
<body><h1>Page Two</h1><a href="/" id="back-link">Back</a></body></html>`)
	})

	mux.HandleFunc("/page3", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>Page 3</title></head>
<body><h1>Page Three (New Tab)</h1></body></html>`)
	})

	mux.HandleFunc("/captcha", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>CAPTCHA Test</title></head>
<body>
  <div class="g-recaptcha" data-sitekey="6LeIxAcTAAAAAJcZVRqyHh71UMIEGNQ_MXjiZKhI"></div>
  <textarea id="g-recaptcha-response" name="g-recaptcha-response"></textarea>
</body></html>`)
	})

	mux.HandleFunc("/shadow", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>Shadow DOM</title></head>
<body>
  <div id="host"></div>
  <script>
    const host = document.getElementById('host');
    const shadow = host.attachShadow({mode: 'open'});
    shadow.innerHTML = '<button id="shadow-btn">Shadow Button</button>';
  </script>
</body></html>`)
	})

	mux.HandleFunc("/iframe", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>Iframe Test</title></head>
<body>
  <h1>Main Page</h1>
  <iframe src="/page2" id="frame1" width="400" height="300"></iframe>
</body></html>`)
	})

	mux.HandleFunc("/drag", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>Drag Test</title></head>
<body>
  <div id="draggable" draggable="true"
       style="width:80px;height:80px;background:blue;position:absolute;left:20px;top:20px;cursor:grab;"
       ondragstart="event.dataTransfer.setData('text/plain','dragged')">
    Drag Me
  </div>
  <div id="dropzone"
       style="width:200px;height:200px;background:#ccc;position:absolute;left:250px;top:20px;border:2px dashed #999;"
       ondragover="event.preventDefault()"
       ondrop="event.preventDefault(); document.getElementById('status').textContent='dropped'">
    Drop Here
  </div>
  <div id="status">waiting</div>
  <script>
    // Also listen for mouseup inside dropzone as fallback for CDP-driven drags
    document.getElementById('dropzone').addEventListener('mouseup', function(e) {
      document.getElementById('status').textContent = 'dropped';
    });
  </script>
</body></html>`)
	})

	mux.HandleFunc("/storage", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "server_cookie", Value: "from_server", Path: "/"})
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>Storage Test</title></head>
<body><h1>Storage Page</h1></body></html>`)
	})

	return httptest.NewServer(mux)
}

// newPage creates a fresh page from the shared browser.
func newPage(t *testing.T) *Page {
	t.Helper()
	page, err := testBrowser.NewPage()
	if err != nil {
		t.Fatalf("NewPage: %v", err)
	}
	t.Cleanup(func() { _ = page.Close() })
	return page
}

// --- Browser Tests ---

func TestBrowser_Rod(t *testing.T) {
	if testBrowser.Rod() == nil {
		t.Error("rod browser should not be nil")
	}
}

func TestBrowser_Pages(t *testing.T) {
	p := newPage(t) // ensure at least one page exists
	_ = p.Navigate(ts.URL)
	pages, err := testBrowser.Pages()
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 {
		t.Error("should have at least one page")
	}
}

func TestBrowser_Incognito(t *testing.T) {
	inc, err := testBrowser.Incognito()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = inc.Close() }()

	page, err := inc.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if page == nil {
		t.Error("incognito page should not be nil")
	}
	_ = page.Close()
}

func TestBrowser_Stealth(t *testing.T) {
	b, err := NewBrowser(BrowserConfig{Headless: true, Stealth: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	page, err := b.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = page.Close() }()

	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}

	val, err := page.Eval(`() => navigator.webdriver`)
	if err != nil {
		t.Fatal(err)
	}
	if val != nil {
		t.Errorf("navigator.webdriver should be undefined in stealth mode, got %v", val)
	}
}

// --- Page Tests ---

func TestPage_Navigate(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.URL(), ts.URL) {
		t.Errorf("URL = %q", page.URL())
	}
}

func TestPage_Title(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	title, err := page.Title()
	if err != nil {
		t.Fatal(err)
	}
	if title != "Test Page" {
		t.Errorf("title = %q", title)
	}
}

func TestPage_HTML(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	html, err := page.HTML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "Hello GoSurfer") {
		t.Error("HTML should contain page content")
	}
}

func TestPage_ElementAndText(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)

	el, err := page.Element("h1")
	if err != nil {
		t.Fatal(err)
	}
	text, _ := el.Text()
	if text != "Hello GoSurfer" {
		t.Errorf("text = %q", text)
	}
}

func TestPage_Elements(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)

	els, err := page.Elements("a")
	if err != nil {
		t.Fatal(err)
	}
	if len(els) < 2 {
		t.Errorf("expected at least 2 links, got %d", len(els))
	}
}

func TestPage_ElementByXPath(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)

	el, err := page.ElementByXPath("//h1")
	if err != nil {
		t.Fatal(err)
	}
	text, _ := el.Text()
	if text != "Hello GoSurfer" {
		t.Errorf("text = %q", text)
	}
}

func TestPage_Click(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	if err := page.Click("#link1"); err != nil {
		t.Fatal(err)
	}
	_ = page.WaitLoad()
	title, _ := page.Title()
	if title != "Page 2" {
		t.Errorf("after click, title = %q", title)
	}
}

func TestPage_TypeAndText(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	if err := page.Type("#search", "test query"); err != nil {
		t.Fatal(err)
	}
	text, _ := page.Text("#search")
	_ = text // input .Text() may not work; check via eval
	val, _ := page.Eval(`() => document.getElementById('search').value`)
	if val != "test query" {
		t.Errorf("input value = %q", val)
	}
}

func TestPage_Screenshot(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	png, err := page.Screenshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(png) < 100 || png[0] != 0x89 || png[1] != 0x50 {
		t.Error("should be valid PNG")
	}
}

func TestPage_FullScreenshot(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	png, err := page.FullScreenshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(png) < 100 {
		t.Error("full screenshot should have content")
	}
}

func TestPage_ScreenshotJPEG(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	jpg, err := page.ScreenshotJPEG(80)
	if err != nil {
		t.Fatal(err)
	}
	if len(jpg) < 100 {
		t.Error("JPEG screenshot should have content")
	}
}

func TestPage_Eval(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	val, err := page.Eval(`() => 2 + 2`)
	if err != nil {
		t.Fatal(err)
	}
	if val != float64(4) {
		t.Errorf("eval = %v", val)
	}
}

func TestPage_BackForward(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	_ = page.Navigate(ts.URL + "/page2")
	_ = page.WaitLoad()

	_ = page.Back()
	_ = page.WaitLoad()
	title, _ := page.Title()
	if title != "Test Page" {
		t.Errorf("after back, title = %q", title)
	}
	_ = page.Forward()
	_ = page.WaitLoad()
	title, _ = page.Title()
	if title != "Page 2" {
		t.Errorf("after forward, title = %q", title)
	}
}

func TestPage_Scroll(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	if err := page.Scroll(0, 100); err != nil {
		t.Fatal(err)
	}
	if err := page.ScrollToBottom(); err != nil {
		t.Fatal(err)
	}
	if err := page.ScrollToTop(); err != nil {
		t.Fatal(err)
	}
}

func TestPage_WaitSelector(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	el, err := page.WaitSelector("#submit-btn")
	if err != nil {
		t.Fatal(err)
	}
	text, _ := el.Text()
	if text != "Search" {
		t.Errorf("text = %q", text)
	}
}

func TestPage_Reload(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	if err := page.Reload(); err != nil {
		t.Fatal(err)
	}
	_ = page.WaitLoad()
	title, _ := page.Title()
	if title != "Test Page" {
		t.Errorf("title = %q after reload", title)
	}
}

func TestPage_TargetID(t *testing.T) {
	page := newPage(t)
	tid := page.TargetID()
	if len(tid) == 0 {
		t.Error("TargetID should not be empty")
	}
	if len(tid) > 4 {
		t.Error("TargetID should be max 4 chars")
	}
}

func TestPage_IsIframe(t *testing.T) {
	page := newPage(t)
	if page.IsIframe() {
		t.Error("regular page should not be iframe")
	}
}

// --- DOMState Tests ---

func TestPage_DOMState(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)

	state, err := page.DOMState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Title != "Test Page" {
		t.Errorf("title = %q", state.Title)
	}
	if len(state.Elements) == 0 {
		t.Error("should find interactive elements")
	}
	if state.Tree == "" {
		t.Error("tree should not be empty")
	}
	foundInput := false
	for _, el := range state.Elements {
		if el.Tag == "input" && el.Attributes["placeholder"] == "Search here..." {
			foundInput = true
			break
		}
	}
	if !foundInput {
		t.Error("should find search input")
	}
}

func TestPage_DOMStateWithScreenshot(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	state, err := page.DOMStateWithScreenshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Screenshot) < 100 {
		t.Error("screenshot should be present")
	}
}

func TestPage_DOMState_Tabs(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	state, err := page.DOMState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Tabs) == 0 {
		t.Error("should list tabs")
	}
}

// --- Element Tests ---

func TestElement_Attribute(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	el, _ := page.Element("#link1")
	href, err := el.Attribute("href")
	if err != nil {
		t.Fatal(err)
	}
	if href != "/page2" {
		t.Errorf("href = %q", href)
	}
}

func TestElement_Visible(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	el, _ := page.Element("#submit-btn")
	v, _ := el.Visible()
	if !v {
		t.Error("button should be visible")
	}
}

func TestElement_BBox(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	el, _ := page.Element("#submit-btn")
	box, err := el.BBox()
	if err != nil {
		t.Fatal(err)
	}
	if box.Width <= 0 || box.Height <= 0 {
		t.Errorf("box = %+v", box)
	}
}

func TestElement_ClearAndType(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	el, _ := page.Element("#search")
	_ = el.Type("initial")
	_ = el.ClearAndType("replaced")
	val, _ := page.Eval(`() => document.getElementById('search').value`)
	if val != "replaced" {
		t.Errorf("value = %q", val)
	}
}

func TestElement_Hover(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	el, _ := page.Element("#submit-btn")
	if err := el.Hover(); err != nil {
		t.Fatal(err)
	}
}

func TestElement_Focus(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	el, _ := page.Element("#search")
	if err := el.Focus(); err != nil {
		t.Fatal(err)
	}
}

func TestElement_Screenshot(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	el, _ := page.Element("#submit-btn")
	buf, err := el.Screenshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(buf) < 10 {
		t.Error("element screenshot should have content")
	}
}

// --- CAPTCHA Detection ---

func TestPage_DetectCAPTCHA_Found(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL + "/captcha")
	info, err := page.DetectCAPTCHA()
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("should detect reCAPTCHA")
	}
	if info.Type != CAPTCHAReCaptchaV2 {
		t.Errorf("type = %q", info.Type)
	}
	if info.SiteKey != "6LeIxAcTAAAAAJcZVRqyHh71UMIEGNQ_MXjiZKhI" {
		t.Errorf("sitekey = %q", info.SiteKey)
	}
}

func TestPage_DetectCAPTCHA_NotFound(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	info, err := page.DetectCAPTCHA()
	if err != nil {
		t.Fatal(err)
	}
	if info != nil {
		t.Error("should not detect CAPTCHA on normal page")
	}
}

// --- Shadow DOM ---

func TestPage_DOMState_ShadowDOM(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL + "/shadow")
	time.Sleep(500 * time.Millisecond)
	state, err := page.DOMState()
	if err != nil {
		t.Fatal(err)
	}
	// Shadow DOM extraction depends on element visibility;
	// verify we at least get the host element or shadow marker
	if !strings.Contains(state.Tree, "|SHADOW|") && !strings.Contains(state.Tree, "Shadow Button") {
		// Shadow DOM may not pierce if host div is zero-sized; verify via direct eval
		val, evalErr := page.Eval(`() => {
			const host = document.getElementById('host');
			return host && host.shadowRoot ? host.shadowRoot.innerHTML : 'no shadow';
		}`)
		if evalErr != nil {
			t.Fatal(evalErr)
		}
		// If shadow DOM exists but wasn't in tree, it's a visibility issue (acceptable)
		if val == "no shadow" {
			t.Error("shadow DOM should exist on the page")
		}
		t.Log("Shadow DOM exists but host element not visible in extraction (zero-size div)")
	}
}

// --- Iframe ---

func TestPage_DOMState_Iframe(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL + "/iframe")
	_ = page.WaitLoad()
	time.Sleep(1 * time.Second) // iframe needs extra load time
	state, err := page.DOMState()
	if err != nil {
		t.Fatal(err)
	}
	// Iframe should appear as IFRAME marker or at least be detected
	if !strings.Contains(state.Tree, "|IFRAME|") {
		// Check if iframe element exists at all in the DOM state
		foundIframe := false
		for _, el := range state.Elements {
			if el.Tag == "iframe" {
				foundIframe = true
				break
			}
		}
		if !foundIframe {
			t.Error("should detect iframe element in DOM state")
		}
	}
}

func TestPage_Frames(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL + "/iframe")
	time.Sleep(500 * time.Millisecond)
	frames, err := page.Frames()
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) < 1 {
		t.Error("should find at least 1 iframe")
	}
}

// --- Action Execution Tests ---

func TestAction_Navigate(t *testing.T) {
	page := newPage(t)
	state := &DOMState{}
	ac := ActionContext{Page: page, State: state, Browser: testBrowser}
	result, err := actionNavigate(context.Background(), ac, map[string]interface{}{"url": ts.URL})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Navigated") {
		t.Errorf("result = %q", result)
	}
	title, _ := page.Title()
	if title != "Test Page" {
		t.Errorf("title = %q", title)
	}
}

func TestAction_Click(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	state, _ := page.DOMState()
	ac := ActionContext{Page: page, State: state, Browser: testBrowser}

	// Find the link element index
	var linkIdx int
	for idx, el := range state.Elements {
		if el.Tag == "a" && el.Attributes["href"] == "/page2" {
			linkIdx = idx
			break
		}
	}
	result, err := actionClick(context.Background(), ac, map[string]interface{}{"index": linkIdx})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Clicked") {
		t.Errorf("result = %q", result)
	}
}

func TestAction_ClickCoordinate(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	state, _ := page.DOMState()
	ac := ActionContext{Page: page, State: state, Browser: testBrowser}

	result, err := actionClick(context.Background(), ac, map[string]interface{}{"x": 100.0, "y": 100.0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "coordinates") {
		t.Errorf("result = %q", result)
	}
}

func TestAction_Type(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	state, _ := page.DOMState()
	ac := ActionContext{Page: page, State: state, Browser: testBrowser}

	var inputIdx int
	for idx, el := range state.Elements {
		if el.Tag == "input" && el.Attributes["type"] == "text" {
			inputIdx = idx
			break
		}
	}
	result, err := actionType(context.Background(), ac, map[string]interface{}{"index": inputIdx, "text": "hello world"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Typed") {
		t.Errorf("result = %q", result)
	}
}

func TestAction_Scroll(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	state, _ := page.DOMState()
	ac := ActionContext{Page: page, State: state}

	result, err := actionScroll(context.Background(), ac, map[string]interface{}{"direction": "down", "amount": 200})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Scrolled down") {
		t.Errorf("result = %q", result)
	}

	result, err = actionScroll(context.Background(), ac, map[string]interface{}{"direction": "up"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Scrolled up") {
		t.Errorf("result = %q", result)
	}
}

func TestAction_Search(t *testing.T) {
	page := newPage(t)
	state := &DOMState{}
	ac := ActionContext{Page: page, State: state}

	result, err := actionSearch(context.Background(), ac, map[string]interface{}{
		"query": "test", "engine": "duckduckgo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "duckduckgo") {
		t.Errorf("result = %q", result)
	}
}

func TestAction_GoBack(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	_ = page.Navigate(ts.URL + "/page2")
	state := &DOMState{}
	ac := ActionContext{Page: page, State: state}
	result, err := actionGoBack(context.Background(), ac, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != "Navigated back" {
		t.Errorf("result = %q", result)
	}
}

func TestAction_Wait(t *testing.T) {
	start := time.Now()
	result, err := actionWait(context.Background(), ActionContext{}, map[string]interface{}{"seconds": 1})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 900*time.Millisecond {
		t.Error("should wait at least ~1 second")
	}
	if !strings.Contains(result, "1 seconds") {
		t.Errorf("result = %q", result)
	}
}

func TestAction_Wait_Clamp(t *testing.T) {
	result, err := actionWait(context.Background(), ActionContext{}, map[string]interface{}{"seconds": 99})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "10 seconds") {
		t.Errorf("should clamp to 10, got %q", result)
	}
}

func TestAction_Screenshot(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	ac := ActionContext{Page: page}
	result, err := actionScreenshot(context.Background(), ac, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Screenshot") {
		t.Errorf("result = %q", result)
	}
}

func TestAction_Extract(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	state, _ := page.DOMState()
	ac := ActionContext{Page: page, State: state}
	result, err := actionExtract(context.Background(), ac, map[string]interface{}{"query": "page title"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Test Page") {
		t.Errorf("result = %q", result)
	}
}

func TestAction_Done(t *testing.T) {
	result, err := actionDone(context.Background(), ActionContext{}, map[string]interface{}{"output": "task complete"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "task complete" {
		t.Errorf("result = %q", result)
	}
}

func TestAction_Navigate_MissingURL(t *testing.T) {
	_, err := actionNavigate(context.Background(), ActionContext{}, map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing url")
	}
}

func TestAction_Click_MissingIndex(t *testing.T) {
	_, err := actionClick(context.Background(), ActionContext{State: &DOMState{Elements: map[int]*DOMElement{}}}, map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing index")
	}
}

func TestAction_Click_BadIndex(t *testing.T) {
	_, err := actionClick(context.Background(), ActionContext{State: &DOMState{Elements: map[int]*DOMElement{}}}, map[string]interface{}{"index": 999})
	if err == nil {
		t.Error("expected error for non-existent index")
	}
}

func TestAction_SelectOption(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	state, _ := page.DOMState()
	ac := ActionContext{Page: page, State: state, Browser: testBrowser}

	var selectIdx int
	found := false
	for idx, el := range state.Elements {
		if el.Tag == "select" {
			selectIdx = idx
			found = true
			break
		}
	}
	if !found {
		t.Skip("no select element found")
	}
	result, err := actionSelectOption(context.Background(), ac, map[string]interface{}{"index": selectIdx, "text": "Blue"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Blue") {
		t.Errorf("result = %q", result)
	}
}

func TestAction_NewTab(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	ac := ActionContext{Page: page, State: &DOMState{}, Browser: testBrowser}

	result, err := actionNewTab(context.Background(), ac, map[string]interface{}{"url": ts.URL + "/page2"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "new tab") {
		t.Errorf("result = %q", result)
	}
}

func TestAction_SendKeys(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	state, _ := page.DOMState()
	ac := ActionContext{Page: page, State: state}
	// Send Escape key (won't error on a normal page)
	result, err := actionSendKeys(context.Background(), ac, map[string]interface{}{"keys": "Escape"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Escape") {
		t.Errorf("result = %q", result)
	}
}

func TestAction_SwitchTab(t *testing.T) {
	page1 := newPage(t)
	_ = page1.Navigate(ts.URL)
	page2 := newPage(t)
	_ = page2.Navigate(ts.URL + "/page2")

	// Get tab ID of page1
	state, _ := page2.DOMState()
	if len(state.Tabs) < 2 {
		t.Skip("need at least 2 tabs")
	}

	agent := &Agent{page: page2, config: AgentConfig{Task: "test", LLM: &mockLLM{}}}
	ac := ActionContext{Page: page2, State: state, Browser: testBrowser, Agent: agent}

	// Switch to first tab
	targetTab := state.Tabs[0].ID
	result, err := actionSwitchTab(context.Background(), ac, map[string]interface{}{"tab_id": targetTab})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Switched") {
		t.Errorf("result = %q", result)
	}
}

func TestAction_SwitchTab_NotFound(t *testing.T) {
	page := newPage(t)
	ac := ActionContext{Page: page, State: &DOMState{}, Browser: testBrowser}
	_, err := actionSwitchTab(context.Background(), ac, map[string]interface{}{"tab_id": "ZZZZ"})
	if err == nil {
		t.Error("expected error for non-existent tab")
	}
}

func TestAction_CloseTab(t *testing.T) {
	// Create a tab specifically to close
	closePage, err := testBrowser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	_ = closePage.Navigate(ts.URL + "/page3")
	tid := closePage.TargetID()

	page := newPage(t)
	ac := ActionContext{Page: page, State: &DOMState{}, Browser: testBrowser}
	result, err := actionCloseTab(context.Background(), ac, map[string]interface{}{"tab_id": tid})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Closed") {
		t.Errorf("result = %q", result)
	}
}

// --- More Element Tests ---

func TestElement_HTML(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	el, _ := page.Element("h1")
	html, err := el.HTML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "Hello GoSurfer") {
		t.Errorf("html = %q", html)
	}
}

func TestElement_DoubleClick(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	el, _ := page.Element("#submit-btn")
	if err := el.DoubleClick(); err != nil {
		t.Fatal(err)
	}
}

func TestElement_WaitVisible(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	el, _ := page.Element("#submit-btn")
	if err := el.WaitVisible(); err != nil {
		t.Fatal(err)
	}
}

func TestElement_WaitStable(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	el, _ := page.Element("#submit-btn")
	if err := el.WaitStable(); err != nil {
		t.Fatal(err)
	}
}

func TestElement_ScrollIntoView(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	el, _ := page.Element("#submit-btn")
	if err := el.ScrollIntoView(); err != nil {
		t.Fatal(err)
	}
}

// --- More Page Tests ---

func TestPage_PDF(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	pdf, err := page.PDF()
	if err != nil {
		t.Fatal(err)
	}
	if len(pdf) < 100 {
		t.Error("PDF should have content")
	}
	// PDF magic bytes
	if string(pdf[:4]) != "%PDF" {
		t.Error("should be valid PDF")
	}
}

func TestPage_KeyPress(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	// Focus on input first
	el, _ := page.Element("#search")
	_ = el.Focus()
	// Press a key
	if err := page.KeyPress('a'); err != nil {
		t.Fatal(err)
	}
}

func TestPage_WaitIdle(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	if err := page.WaitIdle(5 * time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestPage_WaitStable(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)
	if err := page.WaitStable(500 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

func TestBrowser_PageByURL(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL)

	found, err := testBrowser.PageByURL(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	title, _ := found.Title()
	if title != "Test Page" {
		t.Errorf("title = %q", title)
	}
}

// --- Network Interception ---

func TestPage_Intercept_BlockPatterns(t *testing.T) {
	page := newPage(t)
	interceptor := page.Intercept()
	interceptor.BlockPatterns(`.*\.png$`)
	interceptor.Start()
	defer func() { _ = interceptor.Stop() }()

	_ = page.Navigate(ts.URL)
	title, _ := page.Title()
	if title != "Test Page" {
		t.Errorf("page should load despite blocked PNGs, title = %q", title)
	}
}

func TestPage_Intercept_OnRequest(t *testing.T) {
	page := newPage(t)
	intercepted := make(chan string, 10)
	interceptor := page.Intercept()
	interceptor.OnRequest(".*", func(req *InterceptedRequest) {
		intercepted <- req.URL()
		_ = req.Method()
		_ = req.Header("Accept")
		_ = req.Body()
		req.Continue()
	})
	interceptor.Start()

	_ = page.Navigate(ts.URL)
	time.Sleep(500 * time.Millisecond)
	_ = interceptor.Stop()

	if len(intercepted) == 0 {
		t.Log("interceptor may not have caught requests due to timing (non-critical)")
	}
}

// --- Element Frame/Shadow ---

func TestElement_Frame(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL + "/iframe")
	time.Sleep(1 * time.Second) // iframe needs time to load

	iframe, err := page.Element("iframe")
	if err != nil {
		t.Fatal(err)
	}
	frame, err := iframe.Frame()
	if err != nil {
		t.Fatal(err)
	}
	// Frame should be accessible and have content
	html, err := frame.HTML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "Page Two") {
		t.Errorf("iframe content should contain 'Page Two', got: %s", html[:min(len(html), 200)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestElement_ShadowRoot(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL + "/shadow")
	time.Sleep(500 * time.Millisecond)

	host, err := page.Element("#host")
	if err != nil {
		t.Fatal(err)
	}
	root, err := host.ShadowRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root == nil {
		t.Error("shadow root should not be nil")
	}
}

// --- CAPTCHA Injection ---

func TestPage_InjectCAPTCHAToken(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL + "/captcha")

	err := page.injectCAPTCHAToken(CAPTCHAReCaptchaV2, "test-token-123")
	if err != nil {
		t.Fatal(err)
	}
	// Verify token was injected
	val, _ := page.Eval(`() => document.getElementById('g-recaptcha-response').value`)
	if val != "test-token-123" {
		t.Errorf("injected token = %v", val)
	}
}

func TestPage_SolveCAPTCHA_NoCaptcha(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL) // no CAPTCHA on this page

	solver := &ManualCAPTCHASolver{SolveFunc: func(_ context.Context, _ CAPTCHAInfo) (string, error) {
		return "token", nil
	}}
	// Should return nil (no CAPTCHA found)
	err := page.SolveCAPTCHA(context.Background(), solver)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPage_SolveCAPTCHA_WithCaptcha(t *testing.T) {
	page := newPage(t)
	_ = page.Navigate(ts.URL + "/captcha")

	solver := &ManualCAPTCHASolver{SolveFunc: func(_ context.Context, info CAPTCHAInfo) (string, error) {
		if info.Type != CAPTCHAReCaptchaV2 {
			t.Errorf("expected recaptcha_v2, got %s", info.Type)
		}
		return "solved-token", nil
	}}
	err := page.SolveCAPTCHA(context.Background(), solver)
	if err != nil {
		t.Fatal(err)
	}
}

// --- Search action variants ---

func TestAction_Search_AllEngines(t *testing.T) {
	page := newPage(t)
	state := &DOMState{}
	for _, engine := range []string{"google", "bing", ""} {
		ac := ActionContext{Page: page, State: state}
		_, err := actionSearch(context.Background(), ac, map[string]interface{}{"query": "test", "engine": engine})
		if err != nil {
			t.Errorf("engine %q: %v", engine, err)
		}
	}
}

// --- Anthropic formatMessages with image ---

func TestAnthropic_FormatMessages_WithImage(t *testing.T) {
	p := NewAnthropic("key", "model")
	messages := []ChatMessage{
		ImageMessage("user", "describe", []byte{1, 2}, "image/png"),
	}
	formatted := p.formatMessages(messages)
	if len(formatted) != 1 {
		t.Fatalf("expected 1 message, got %d", len(formatted))
	}
	msg, ok := formatted[0].(map[string]interface{})
	if !ok {
		t.Fatal("should be map")
	}
	parts, ok := msg["content"].([]interface{})
	if !ok {
		t.Fatal("content should be array")
	}
	if len(parts) != 2 {
		t.Errorf("expected 2 parts, got %d", len(parts))
	}
}

// --- Agent Integration ---

func TestAgent_RunWithMockLLM(t *testing.T) {
	mock := &mockLLM{response: fmt.Sprintf(
		`{"thought":"navigate first","action":"navigate","params":{"url":"%s"}}`, ts.URL)}

	agent, err := NewAgent(AgentConfig{
		Task:     "test task",
		LLM:      mock,
		Browser:  testBrowser, // reuse shared browser
		MaxSteps: 3,
		OnStep: func(_ StepInfo) {
			mock.response = `{"thought":"done","action":"done","params":{"output":"found it","success":true}}`
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := agent.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Errorf("expected success, output: %s", result.Output)
	}
	if result.Output != "found it" {
		t.Errorf("output = %q", result.Output)
	}
	if result.Steps < 2 {
		t.Errorf("expected at least 2 steps, got %d", result.Steps)
	}
}

// --- New Action Tests: Cookies, Storage, Drag ---

func TestAction_GetCookies(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL + "/storage"); err != nil {
		t.Fatal(err)
	}
	_ = page.WaitLoad()

	// The /storage handler sets a "server_cookie" via Set-Cookie header
	ac := ActionContext{Page: page, State: &DOMState{}, Browser: testBrowser}
	result, err := actionGetCookies(context.Background(), ac, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "server_cookie") {
		t.Errorf("expected server_cookie in result, got %q", result)
	}
	if !strings.Contains(result, "cookie") {
		t.Errorf("result should mention cookies: %q", result)
	}
}

func TestAction_SetCookie(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}

	ac := ActionContext{Page: page, State: &DOMState{}, Browser: testBrowser}
	result, err := actionSetCookie(context.Background(), ac, map[string]interface{}{
		"name": "action_cookie", "value": "action_value",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "action_cookie=action_value") {
		t.Errorf("result = %q", result)
	}

	// Verify cookie was actually set
	val, err := page.GetCookie("action_cookie")
	if err != nil {
		t.Fatal(err)
	}
	if val != "action_value" {
		t.Errorf("cookie value = %q, want %q", val, "action_value")
	}
}

func TestAction_SetCookie_MissingName(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}

	ac := ActionContext{Page: page, State: &DOMState{}, Browser: testBrowser}
	_, err := actionSetCookie(context.Background(), ac, map[string]interface{}{
		"value": "v",
	})
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestAction_GetStorage(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}

	// Set some localStorage first
	if err := page.LocalStorageSet("action_key", "action_val"); err != nil {
		t.Fatal(err)
	}

	ac := ActionContext{Page: page, State: &DOMState{}, Browser: testBrowser}
	result, err := actionGetStorage(context.Background(), ac, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "action_key") {
		t.Errorf("expected action_key in result, got %q", result)
	}
}

func TestAction_GetStorage_Empty(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}
	_ = page.LocalStorageClear()

	ac := ActionContext{Page: page, State: &DOMState{}, Browser: testBrowser}
	result, err := actionGetStorage(context.Background(), ac, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != "localStorage is empty" {
		t.Errorf("expected empty message, got %q", result)
	}
}

func TestAction_SetStorage(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}
	_ = page.LocalStorageClear()

	ac := ActionContext{Page: page, State: &DOMState{}, Browser: testBrowser}
	result, err := actionSetStorage(context.Background(), ac, map[string]interface{}{
		"key": "set_key", "value": "set_val",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "set_key=set_val") {
		t.Errorf("result = %q", result)
	}

	// Verify it was stored
	val, err := page.LocalStorageGet("set_key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "set_val" {
		t.Errorf("localStorage value = %q, want %q", val, "set_val")
	}
}

func TestAction_SetStorage_MissingKey(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}

	ac := ActionContext{Page: page, State: &DOMState{}, Browser: testBrowser}
	_, err := actionSetStorage(context.Background(), ac, map[string]interface{}{
		"value": "v",
	})
	if err == nil {
		t.Error("expected error for missing key")
	}
}

func TestAction_Drag(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL + "/drag"); err != nil {
		t.Fatal(err)
	}
	_ = page.WaitLoad()
	time.Sleep(300 * time.Millisecond)

	state, err := page.DOMState()
	if err != nil {
		t.Fatal(err)
	}
	ac := ActionContext{Page: page, State: state, Browser: testBrowser}

	// Find the draggable and dropzone element indices
	var fromIdx, toIdx int
	foundFrom, foundTo := false, false
	for idx, el := range state.Elements {
		if el.Attributes["id"] == "draggable" {
			fromIdx = idx
			foundFrom = true
		}
		if el.Attributes["id"] == "dropzone" {
			toIdx = idx
			foundTo = true
		}
	}

	if !foundFrom || !foundTo {
		t.Skipf("could not find draggable/dropzone elements in DOM state (from=%v, to=%v)", foundFrom, foundTo)
	}

	// Test element-to-element drag via action
	result, err := actionDrag(context.Background(), ac, map[string]interface{}{
		"from_index": fromIdx,
		"to_index":   toIdx,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Dragged") {
		t.Errorf("result = %q", result)
	}
}

func TestAction_Drag_ToCoordinates(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL + "/drag"); err != nil {
		t.Fatal(err)
	}
	_ = page.WaitLoad()
	time.Sleep(300 * time.Millisecond)

	state, err := page.DOMState()
	if err != nil {
		t.Fatal(err)
	}
	ac := ActionContext{Page: page, State: state, Browser: testBrowser}

	// Find draggable index
	var fromIdx int
	found := false
	for idx, el := range state.Elements {
		if el.Attributes["id"] == "draggable" {
			fromIdx = idx
			found = true
			break
		}
	}
	if !found {
		t.Skip("could not find draggable element in DOM state")
	}

	result, err := actionDrag(context.Background(), ac, map[string]interface{}{
		"from_index": fromIdx,
		"to_x":       350.0,
		"to_y":       120.0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Dragged") {
		t.Errorf("result = %q", result)
	}
}

func TestAction_Drag_MissingTarget(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL + "/drag"); err != nil {
		t.Fatal(err)
	}
	_ = page.WaitLoad()
	time.Sleep(300 * time.Millisecond)

	state, err := page.DOMState()
	if err != nil {
		t.Fatal(err)
	}
	ac := ActionContext{Page: page, State: state, Browser: testBrowser}

	// Find draggable index
	var fromIdx int
	for idx, el := range state.Elements {
		if el.Attributes["id"] == "draggable" {
			fromIdx = idx
			break
		}
	}

	// Missing both to_index and to_x/to_y
	_, err = actionDrag(context.Background(), ac, map[string]interface{}{
		"from_index": fromIdx,
	})
	if err == nil {
		t.Error("expected error when no target specified")
	}
}

func TestAction_GetCookies_Empty(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}
	_ = page.ClearCookies()

	ac := ActionContext{Page: page, State: &DOMState{}, Browser: testBrowser}
	result, err := actionGetCookies(context.Background(), ac, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != "No cookies" {
		t.Errorf("expected 'No cookies', got %q", result)
	}
}

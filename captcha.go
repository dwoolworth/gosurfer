package gosurfer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// CAPTCHAType identifies the CAPTCHA provider.
type CAPTCHAType string

const (
	CAPTCHAReCaptchaV2 CAPTCHAType = "recaptcha_v2"
	CAPTCHAReCaptchaV3 CAPTCHAType = "recaptcha_v3"
	CAPTCHAHCaptcha    CAPTCHAType = "hcaptcha"
	CAPTCHATurnstile   CAPTCHAType = "turnstile"
)

// CAPTCHAInfo describes a detected CAPTCHA on a page.
type CAPTCHAInfo struct {
	Type    CAPTCHAType
	SiteKey string
	PageURL string
}

// CAPTCHASolver is the interface for CAPTCHA solving backends.
type CAPTCHASolver interface {
	// Solve sends the CAPTCHA to a solving service and returns the token.
	Solve(ctx context.Context, info CAPTCHAInfo) (string, error)
	// Name returns the solver name for logging.
	Name() string
}

// DetectCAPTCHA inspects the current page for known CAPTCHA providers.
// Returns nil if no CAPTCHA is found.
func (p *Page) DetectCAPTCHA() (*CAPTCHAInfo, error) {
	result, err := p.rod.Eval(captchaDetectionScript)
	if err != nil {
		return nil, fmt.Errorf("gosurfer: captcha detection: %w", err)
	}

	var info struct {
		Found   bool   `json:"found"`
		Type    string `json:"type"`
		SiteKey string `json:"siteKey"`
	}
	if err := result.Value.Unmarshal(&info); err != nil {
		return nil, fmt.Errorf("gosurfer: parse captcha info: %w", err)
	}
	if !info.Found {
		return nil, nil
	}

	pageURL := p.URL()
	return &CAPTCHAInfo{
		Type:    CAPTCHAType(info.Type),
		SiteKey: info.SiteKey,
		PageURL: pageURL,
	}, nil
}

// SolveCAPTCHA detects a CAPTCHA on the page and solves it using the provided solver.
// It injects the solution token into the page automatically.
func (p *Page) SolveCAPTCHA(ctx context.Context, solver CAPTCHASolver) error {
	info, err := p.DetectCAPTCHA()
	if err != nil {
		return err
	}
	if info == nil {
		return nil // no CAPTCHA found
	}

	token, err := solver.Solve(ctx, *info)
	if err != nil {
		return fmt.Errorf("gosurfer: solve captcha: %w", err)
	}

	return p.injectCAPTCHAToken(info.Type, token)
}

// injectCAPTCHAToken inserts the solution token into the page.
func (p *Page) injectCAPTCHAToken(captchaType CAPTCHAType, token string) error {
	var js string
	switch captchaType {
	case CAPTCHAReCaptchaV2, CAPTCHAReCaptchaV3:
		js = fmt.Sprintf(`() => {
			// Set response textarea
			const responses = document.querySelectorAll('[name="g-recaptcha-response"], textarea.g-recaptcha-response');
			responses.forEach(el => { el.innerHTML = %q; el.value = %q; });
			// Find and call the callback
			if (typeof ___grecaptcha_cfg !== 'undefined') {
				const clients = ___grecaptcha_cfg.clients || {};
				Object.values(clients).forEach(client => {
					// Walk the client object tree to find callbacks
					const findCallback = (obj, depth) => {
						if (depth > 5 || !obj) return;
						Object.values(obj).forEach(v => {
							if (typeof v === 'function') { try { v(%q); } catch(e) {} }
							else if (typeof v === 'object') findCallback(v, depth + 1);
						});
					};
					findCallback(client, 0);
				});
			}
		}`, token, token, token)
	case CAPTCHAHCaptcha:
		js = fmt.Sprintf(`() => {
			const responses = document.querySelectorAll('[name="h-captcha-response"], textarea[name="h-captcha-response"]');
			responses.forEach(el => { el.innerHTML = %q; el.value = %q; });
			// hCaptcha callback
			if (typeof hcaptcha !== 'undefined') {
				try {
					const iframe = document.querySelector('iframe[src*="hcaptcha.com"]');
					if (iframe) { iframe.setAttribute('data-hcaptcha-response', %q); }
				} catch(e) {}
			}
			// Submit the form if possible
			const form = document.querySelector('.h-captcha')?.closest('form');
			if (form) { form.dispatchEvent(new Event('submit', {bubbles: true})); }
		}`, token, token, token)
	case CAPTCHATurnstile:
		js = fmt.Sprintf(`() => {
			const responses = document.querySelectorAll('[name="cf-turnstile-response"], input[name="cf-turnstile-response"]');
			responses.forEach(el => { el.value = %q; });
			// Turnstile callback
			if (typeof turnstile !== 'undefined') {
				const widgets = document.querySelectorAll('.cf-turnstile');
				widgets.forEach(w => {
					const callbackName = w.getAttribute('data-callback');
					if (callbackName && typeof window[callbackName] === 'function') {
						window[callbackName](%q);
					}
				});
			}
		}`, token, token)
	default:
		return fmt.Errorf("unsupported CAPTCHA type: %s", captchaType)
	}

	_, err := p.rod.Eval(js)
	return err
}

// --- 2Captcha Solver ---

// TwoCaptchaSolver implements CAPTCHASolver using the 2captcha.com API.
type TwoCaptchaSolver struct {
	APIKey  string
	BaseURL string // default: https://2captcha.com
	client  *http.Client
}

// NewTwoCaptchaSolver creates a 2Captcha solver.
func NewTwoCaptchaSolver(apiKey string) *TwoCaptchaSolver {
	return &TwoCaptchaSolver{
		APIKey:  apiKey,
		BaseURL: "https://2captcha.com",
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *TwoCaptchaSolver) Name() string { return "2captcha" }

func (s *TwoCaptchaSolver) Solve(ctx context.Context, info CAPTCHAInfo) (string, error) {
	// Step 1: Submit the CAPTCHA
	params := url.Values{
		"key":     {s.APIKey},
		"pageurl": {info.PageURL},
		"json":    {"1"},
	}
	switch info.Type {
	case CAPTCHAReCaptchaV2:
		params.Set("method", "userrecaptcha")
		params.Set("googlekey", info.SiteKey)
	case CAPTCHAReCaptchaV3:
		params.Set("method", "userrecaptcha")
		params.Set("version", "v3")
		params.Set("googlekey", info.SiteKey)
		params.Set("min_score", "0.5")
	case CAPTCHAHCaptcha:
		params.Set("method", "hcaptcha")
		params.Set("sitekey", info.SiteKey)
	case CAPTCHATurnstile:
		params.Set("method", "turnstile")
		params.Set("sitekey", info.SiteKey)
	default:
		return "", fmt.Errorf("unsupported CAPTCHA type: %s", info.Type)
	}

	baseURL := s.BaseURL
	if baseURL == "" {
		baseURL = "https://2captcha.com"
	}

	submitURL := baseURL + "/in.php?" + params.Encode()
	resp, err := s.doGet(ctx, submitURL)
	if err != nil {
		return "", fmt.Errorf("submit captcha: %w", err)
	}

	var submitResult struct {
		Status  int    `json:"status"`
		Request string `json:"request"`
	}
	if err := json.Unmarshal(resp, &submitResult); err != nil {
		return "", fmt.Errorf("parse submit response: %w, body: %s", err, string(resp))
	}
	if submitResult.Status != 1 {
		return "", fmt.Errorf("2captcha submit error: %s", submitResult.Request)
	}
	taskID := submitResult.Request

	// Step 2: Poll for result (with backoff)
	pollURL := fmt.Sprintf("%s/res.php?key=%s&action=get&id=%s&json=1",
		baseURL, s.APIKey, taskID)

	for attempt := 0; attempt < 60; attempt++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}

		resp, err := s.doGet(ctx, pollURL)
		if err != nil {
			continue
		}

		var pollResult struct {
			Status  int    `json:"status"`
			Request string `json:"request"`
		}
		if err := json.Unmarshal(resp, &pollResult); err != nil {
			continue
		}

		if pollResult.Status == 1 {
			return pollResult.Request, nil
		}
		if pollResult.Request != "CAPCHA_NOT_READY" {
			return "", fmt.Errorf("2captcha error: %s", pollResult.Request)
		}
	}

	return "", fmt.Errorf("2captcha: timeout waiting for solution")
}

func (s *TwoCaptchaSolver) doGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}

// --- CapSolver ---

// CapSolver implements CAPTCHASolver using the capsolver.com API.
type CapSolver struct {
	APIKey  string
	BaseURL string // default: https://api.capsolver.com
	client  *http.Client
}

// NewCapSolver creates a CapSolver solver.
func NewCapSolver(apiKey string) *CapSolver {
	return &CapSolver{
		APIKey:  apiKey,
		BaseURL: "https://api.capsolver.com",
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *CapSolver) Name() string { return "capsolver" }

func (s *CapSolver) Solve(ctx context.Context, info CAPTCHAInfo) (string, error) {
	baseURL := s.BaseURL
	if baseURL == "" {
		baseURL = "https://api.capsolver.com"
	}

	// Map CAPTCHA type to CapSolver task type
	var taskType string
	taskParams := map[string]interface{}{
		"websiteURL": info.PageURL,
		"websiteKey": info.SiteKey,
	}
	switch info.Type {
	case CAPTCHAReCaptchaV2:
		taskType = "ReCaptchaV2TaskProxyLess"
	case CAPTCHAReCaptchaV3:
		taskType = "ReCaptchaV3TaskProxyLess"
		taskParams["pageAction"] = "verify"
	case CAPTCHAHCaptcha:
		taskType = "HCaptchaTaskProxyLess"
	case CAPTCHATurnstile:
		taskType = "AntiTurnstileTaskProxyLess"
	default:
		return "", fmt.Errorf("unsupported CAPTCHA type: %s", info.Type)
	}
	taskParams["type"] = taskType

	// Step 1: Create task
	createBody, _ := json.Marshal(map[string]interface{}{
		"clientKey": s.APIKey,
		"task":      taskParams,
	})

	resp, err := s.doPost(ctx, baseURL+"/createTask", createBody)
	if err != nil {
		return "", fmt.Errorf("create task: %w", err)
	}

	var createResult struct {
		ErrorID int    `json:"errorId"`
		ErrorDesc string `json:"errorDescription"`
		TaskID  string `json:"taskId"`
	}
	if err := json.Unmarshal(resp, &createResult); err != nil {
		return "", fmt.Errorf("parse create response: %w", err)
	}
	if createResult.ErrorID != 0 {
		return "", fmt.Errorf("capsolver: %s", createResult.ErrorDesc)
	}

	// Step 2: Poll for result
	for attempt := 0; attempt < 60; attempt++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(3 * time.Second):
		}

		pollBody, _ := json.Marshal(map[string]interface{}{
			"clientKey": s.APIKey,
			"taskId":    createResult.TaskID,
		})
		resp, err := s.doPost(ctx, baseURL+"/getTaskResult", pollBody)
		if err != nil {
			continue
		}

		var pollResult struct {
			ErrorID  int    `json:"errorId"`
			ErrorDesc string `json:"errorDescription"`
			Status   string `json:"status"`
			Solution struct {
				GRecaptchaResponse string `json:"gRecaptchaResponse"`
				Token              string `json:"token"`
			} `json:"solution"`
		}
		if err := json.Unmarshal(resp, &pollResult); err != nil {
			continue
		}
		if pollResult.ErrorID != 0 {
			return "", fmt.Errorf("capsolver: %s", pollResult.ErrorDesc)
		}
		if pollResult.Status == "ready" {
			token := pollResult.Solution.GRecaptchaResponse
			if token == "" {
				token = pollResult.Solution.Token
			}
			return token, nil
		}
	}

	return "", fmt.Errorf("capsolver: timeout waiting for solution")
}

func (s *CapSolver) doPost(ctx context.Context, rawURL string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}

// --- CAPTCHA Detection Script ---

const captchaDetectionScript = `() => {
	// reCAPTCHA v2/v3
	const recaptchaEl = document.querySelector('.g-recaptcha, #recaptcha, [data-sitekey]');
	const recaptchaIframe = document.querySelector('iframe[src*="recaptcha"], iframe[src*="google.com/recaptcha"]');
	if (recaptchaEl || recaptchaIframe) {
		let siteKey = '';
		if (recaptchaEl) {
			siteKey = recaptchaEl.getAttribute('data-sitekey') || '';
		}
		if (!siteKey && recaptchaIframe) {
			const src = recaptchaIframe.src;
			const match = src.match(/[?&]k=([^&]+)/);
			if (match) siteKey = match[1];
		}
		const isV3 = document.querySelector('.g-recaptcha[data-size="invisible"]') !== null ||
		             document.querySelector('script[src*="recaptcha/api.js?render="]') !== null;
		return { found: true, type: isV3 ? 'recaptcha_v3' : 'recaptcha_v2', siteKey: siteKey };
	}

	// hCaptcha
	const hcaptchaEl = document.querySelector('.h-captcha, [data-hcaptcha-sitekey]');
	const hcaptchaIframe = document.querySelector('iframe[src*="hcaptcha.com"]');
	if (hcaptchaEl || hcaptchaIframe) {
		let siteKey = '';
		if (hcaptchaEl) {
			siteKey = hcaptchaEl.getAttribute('data-sitekey') ||
			          hcaptchaEl.getAttribute('data-hcaptcha-sitekey') || '';
		}
		if (!siteKey && hcaptchaIframe) {
			const src = hcaptchaIframe.src;
			const match = src.match(/[?&]sitekey=([^&]+)/);
			if (match) siteKey = match[1];
		}
		return { found: true, type: 'hcaptcha', siteKey: siteKey };
	}

	// Cloudflare Turnstile
	const turnstileEl = document.querySelector('.cf-turnstile, [data-turnstile-sitekey]');
	const turnstileIframe = document.querySelector('iframe[src*="challenges.cloudflare.com"]');
	if (turnstileEl || turnstileIframe) {
		let siteKey = '';
		if (turnstileEl) {
			siteKey = turnstileEl.getAttribute('data-sitekey') ||
			          turnstileEl.getAttribute('data-turnstile-sitekey') || '';
		}
		return { found: true, type: 'turnstile', siteKey: siteKey };
	}

	// Cloudflare challenge page (interstitial)
	const cfChallenge = document.querySelector('#challenge-form, #challenge-running, .cf-browser-verification');
	if (cfChallenge) {
		return { found: true, type: 'turnstile', siteKey: '' };
	}

	return { found: false, type: '', siteKey: '' };
}`

// --- Manual/Callback Solver ---

// ManualCAPTCHASolver calls a user-provided function to solve CAPTCHAs.
// Useful for custom solving services or human-in-the-loop flows.
type ManualCAPTCHASolver struct {
	SolveFunc func(ctx context.Context, info CAPTCHAInfo) (string, error)
}

func (s *ManualCAPTCHASolver) Name() string { return "manual" }

func (s *ManualCAPTCHASolver) Solve(ctx context.Context, info CAPTCHAInfo) (string, error) {
	return s.SolveFunc(ctx, info)
}

// --- Helper ---

// Compile-time interface checks.
var (
	_ CAPTCHASolver = (*TwoCaptchaSolver)(nil)
	_ CAPTCHASolver = (*CapSolver)(nil)
	_ CAPTCHASolver = (*ManualCAPTCHASolver)(nil)
)

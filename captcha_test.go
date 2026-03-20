package gosurfer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTwoCaptchaSolver_Solve(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "/in.php") {
			// Submit - check required params
			if r.URL.Query().Get("method") != "userrecaptcha" {
				t.Error("expected userrecaptcha method")
			}
			if r.URL.Query().Get("googlekey") == "" {
				t.Error("expected googlekey param")
			}
			_, _ = fmt.Fprint(w, `{"status":1,"request":"TASK123"}`)
			return
		}

		if strings.Contains(r.URL.Path, "/res.php") {
			callCount++
			if callCount < 2 {
				_, _ = fmt.Fprint(w, `{"status":0,"request":"CAPCHA_NOT_READY"}`)
			} else {
				_, _ = fmt.Fprint(w, `{"status":1,"request":"SOLUTION_TOKEN_HERE"}`)
			}
			return
		}
	}))
	defer server.Close()

	solver := NewTwoCaptchaSolver("test-api-key")
	solver.BaseURL = server.URL

	token, err := solver.Solve(context.Background(), CAPTCHAInfo{
		Type:    CAPTCHAReCaptchaV2,
		SiteKey: "6LeIxAcTAAAAAJcZVRqyHh71UMIEGNQ_MXjiZKhI",
		PageURL: "https://example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if token != "SOLUTION_TOKEN_HERE" {
		t.Errorf("token = %q", token)
	}
}

func TestTwoCaptchaSolver_SubmitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":0,"request":"ERROR_WRONG_USER_KEY"}`)
	}))
	defer server.Close()

	solver := NewTwoCaptchaSolver("bad-key")
	solver.BaseURL = server.URL

	_, err := solver.Solve(context.Background(), CAPTCHAInfo{
		Type: CAPTCHAReCaptchaV2, SiteKey: "key", PageURL: "https://example.com",
	})
	if err == nil {
		t.Error("expected error for bad API key")
	}
	if !strings.Contains(err.Error(), "ERROR_WRONG_USER_KEY") {
		t.Errorf("error should contain API error: %v", err)
	}
}

func TestTwoCaptchaSolver_Cancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/in.php") {
			_, _ = fmt.Fprint(w, `{"status":1,"request":"TASK123"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"status":0,"request":"CAPCHA_NOT_READY"}`)
	}))
	defer server.Close()

	solver := NewTwoCaptchaSolver("key")
	solver.BaseURL = server.URL

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := solver.Solve(ctx, CAPTCHAInfo{
		Type: CAPTCHAReCaptchaV2, SiteKey: "key", PageURL: "https://example.com",
	})
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestCapSolver_Solve(t *testing.T) {
	pollCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "/createTask") {
			var req map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req["clientKey"] != "test-key" {
				t.Error("expected clientKey")
			}
			_, _ = fmt.Fprint(w, `{"errorId":0,"taskId":"task-456"}`)
			return
		}

		if strings.Contains(r.URL.Path, "/getTaskResult") {
			pollCount++
			if pollCount < 2 {
				_, _ = fmt.Fprint(w, `{"errorId":0,"status":"processing"}`)
			} else {
				_, _ = fmt.Fprint(w, `{"errorId":0,"status":"ready","solution":{"gRecaptchaResponse":"CAP_TOKEN"}}`)
			}
			return
		}
	}))
	defer server.Close()

	solver := NewCapSolver("test-key")
	solver.BaseURL = server.URL

	token, err := solver.Solve(context.Background(), CAPTCHAInfo{
		Type: CAPTCHAHCaptcha, SiteKey: "site-key", PageURL: "https://example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if token != "CAP_TOKEN" {
		t.Errorf("token = %q", token)
	}
}

func TestCapSolver_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"errorId":1,"errorDescription":"ERROR_KEY_DENIED"}`)
	}))
	defer server.Close()

	solver := NewCapSolver("bad-key")
	solver.BaseURL = server.URL

	_, err := solver.Solve(context.Background(), CAPTCHAInfo{
		Type: CAPTCHAReCaptchaV2, SiteKey: "key", PageURL: "https://example.com",
	})
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "ERROR_KEY_DENIED") {
		t.Errorf("error should contain API error: %v", err)
	}
}

func TestManualSolver(t *testing.T) {
	solver := &ManualCAPTCHASolver{
		SolveFunc: func(_ context.Context, info CAPTCHAInfo) (string, error) {
			return "manual-token-for-" + string(info.Type), nil
		},
	}
	if solver.Name() != "manual" {
		t.Errorf("name = %q", solver.Name())
	}

	token, err := solver.Solve(context.Background(), CAPTCHAInfo{Type: CAPTCHAHCaptcha})
	if err != nil {
		t.Fatal(err)
	}
	if token != "manual-token-for-hcaptcha" {
		t.Errorf("token = %q", token)
	}
}

func TestSolverNames(t *testing.T) {
	if NewTwoCaptchaSolver("k").Name() != "2captcha" {
		t.Error("wrong name")
	}
	if NewCapSolver("k").Name() != "capsolver" {
		t.Error("wrong name")
	}
}

func TestTwoCaptchaSolver_AllTypes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/in.php") {
			_, _ = fmt.Fprint(w, `{"status":1,"request":"T1"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"status":1,"request":"TOKEN"}`)
	}))
	defer server.Close()

	types := []CAPTCHAType{CAPTCHAReCaptchaV2, CAPTCHAReCaptchaV3, CAPTCHAHCaptcha, CAPTCHATurnstile}
	for _, ct := range types {
		solver := NewTwoCaptchaSolver("key")
		solver.BaseURL = server.URL
		_, err := solver.Solve(context.Background(), CAPTCHAInfo{
			Type: ct, SiteKey: "sk", PageURL: "https://example.com",
		})
		if err != nil {
			t.Errorf("type %s: %v", ct, err)
		}
	}
}

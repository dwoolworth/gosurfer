package gosurfer

import (
	"strings"
	"testing"
)

func TestStealthScript_NotEmpty(t *testing.T) {
	if len(stealthScript) < 100 {
		t.Error("stealth script should be substantial")
	}
}

func TestStealthScript_CriticalEvasions(t *testing.T) {
	checks := []struct {
		name    string
		pattern string
	}{
		{"webdriver removal", "navigator.webdriver"},
		{"chrome object", "window.chrome"},
		{"chrome.runtime", "chrome.runtime"},
		{"chrome.loadTimes", "chrome.loadTimes"},
		{"chrome.csi", "chrome.csi"},
		{"plugins array", "navigator.plugins"},
		{"languages", "navigator.languages"},
		{"permissions API", "navigator.permissions"},
		{"outer dimensions", "outerWidth"},
		{"hardware concurrency", "hardwareConcurrency"},
		{"device memory", "deviceMemory"},
		{"WebGL vendor", "UNMASKED_VENDOR_WEBGL"},
		{"WebGL renderer", "UNMASKED_RENDERER_WEBGL"},
		{"media devices", "mediaDevices"},
		{"toString patch", "nativePatterns"},
	}

	for _, check := range checks {
		if !strings.Contains(stealthScript, check.pattern) {
			t.Errorf("stealth script missing %s evasion (pattern: %q)", check.name, check.pattern)
		}
	}
}

func TestStealthLaunchFlags(t *testing.T) {
	flags := stealthLaunchFlags()
	if _, ok := flags["disable-blink-features"]; !ok {
		t.Error("should include disable-blink-features flag")
	}
	if flags["disable-blink-features"] != "AutomationControlled" {
		t.Error("should disable AutomationControlled")
	}
}

func TestDefaultUserAgent(t *testing.T) {
	ua := defaultUserAgent()
	if !strings.Contains(ua, "Chrome/") {
		t.Error("user agent should contain Chrome version")
	}
	if !strings.Contains(ua, "Mozilla/5.0") {
		t.Error("user agent should start with Mozilla/5.0")
	}
	if strings.Contains(ua, "Headless") {
		t.Error("user agent should NOT contain Headless")
	}
}

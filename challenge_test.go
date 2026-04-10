package gosurfer

import (
	"strings"
	"testing"
)

func TestChallengeType_IsAutoSolvable(t *testing.T) {
	tests := []struct {
		ct   ChallengeType
		want bool
	}{
		{ChallengeNone, false},
		{ChallengeCloudflareUAM, true},
		{ChallengeCloudflareTurnstile, false},
		{ChallengeDataDome, false},
		{ChallengeType("unknown"), false},
	}
	for _, tc := range tests {
		if got := tc.ct.IsAutoSolvable(); got != tc.want {
			t.Errorf("%q.IsAutoSolvable() = %v, want %v", tc.ct, got, tc.want)
		}
	}
}

// TestDetectChallengeJS_ContainsExpectedPatterns sanity-checks that the JS
// snippet includes the specific strings we rely on. If someone refactors
// and accidentally removes a pattern, this test catches it.
func TestDetectChallengeJS_ContainsExpectedPatterns(t *testing.T) {
	required := []string{
		// DataDome (checked FIRST)
		"captcha-delivery.com",
		"datadome",
		"var dd={",
		// Cloudflare Turnstile — only specific widget class
		"cf-turnstile",
		// Cloudflare UAM title checks
		"just a moment",
		"attention required",
		"checking if the site connection is secure",
		"checking your browser before accessing",
		// Cloudflare UAM markup — specific paths only
		"/cdn-cgi/challenge-platform/h/",
		"_cf_chl_opt",
		"cf-im-under-attack",
		// Return values
		`"cloudflare_uam"`,
		`"cloudflare_turnstile"`,
		`"datadome"`,
	}
	for _, pat := range required {
		if !strings.Contains(detectChallengeJS, pat) {
			t.Errorf("detectChallengeJS missing expected pattern: %q", pat)
		}
	}
}

// TestDetectChallengeJS_AvoidsGenericTurnstileMatch verifies that we do NOT
// use a generic html.indexOf("turnstile") check, because many Cloudflare UAM
// pages reference the turnstile JS library by name without actually being
// Turnstile-protected. Matching the generic word caused false Turnstile
// classifications on GoAnywhere in production. The only acceptable
// indexOf/check against "turnstile" strings must include the "cf-" prefix.
func TestDetectChallengeJS_AvoidsGenericTurnstileMatch(t *testing.T) {
	// Look for html.indexOf patterns — those are the actual detection checks.
	// Strip comments so we only inspect code.
	codeOnly := stripJSComments(detectChallengeJS)

	// Any indexOf that contains "turnstile" must contain "cf-turnstile" not
	// just "turnstile".
	lines := strings.Split(codeOnly, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "turnstile") {
			continue
		}
		// This line mentions turnstile. Must be either the return value
		// ("cloudflare_turnstile") or an indexOf("cf-turnstile...").
		if strings.Contains(line, `"cloudflare_turnstile"`) {
			continue // allowed: it's the return value
		}
		if strings.Contains(line, "cf-turnstile") {
			continue // allowed: it's the specific widget class
		}
		t.Errorf("line %d contains generic 'turnstile' reference: %q", i+1, strings.TrimSpace(line))
	}
}

// stripJSComments removes // line comments and /* ... */ block comments
// so tests can inspect only the executable code.
func stripJSComments(src string) string {
	var b strings.Builder
	i := 0
	for i < len(src) {
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			// Line comment — skip to end of line.
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			// Block comment — skip to closing */.
			i += 2
			for i+1 < len(src) && (src[i] != '*' || src[i+1] != '/') {
				i++
			}
			i += 2
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

// TestChallengeTypeConstants ensures the string values stay stable —
// they're part of the public API and logs, so they should not change.
func TestChallengeTypeConstants(t *testing.T) {
	tests := []struct {
		ct   ChallengeType
		want string
	}{
		{ChallengeNone, ""},
		{ChallengeCloudflareUAM, "cloudflare_uam"},
		{ChallengeCloudflareTurnstile, "cloudflare_turnstile"},
		{ChallengeDataDome, "datadome"},
	}
	for _, tc := range tests {
		if string(tc.ct) != tc.want {
			t.Errorf("ChallengeType value changed: got %q, want %q", string(tc.ct), tc.want)
		}
	}
}

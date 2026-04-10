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
		// Cloudflare UAM title checks
		"just a moment",
		"attention required",
		"checking if the site connection is secure",
		"checking your browser before accessing",
		// Cloudflare challenge markup checks
		"challenge-platform",
		"_cf_chl_opt",
		"/cdn-cgi/challenge-platform/",
		// Turnstile
		"cf-turnstile",
		"turnstile",
		// DataDome
		"captcha-delivery.com",
		"datadome",
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

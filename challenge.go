package gosurfer

import (
	"fmt"
	"strings"
	"time"
)

// ChallengeType identifies a specific bot-protection challenge.
type ChallengeType string

const (
	// ChallengeNone means the page is not currently showing a challenge.
	ChallengeNone ChallengeType = ""
	// ChallengeCloudflareUAM is Cloudflare "Under Attack Mode" — the classic
	// "Just a moment..." JavaScript challenge that resolves in ~5-15 seconds.
	ChallengeCloudflareUAM ChallengeType = "cloudflare_uam"
	// ChallengeCloudflareTurnstile is Cloudflare's newer interactive challenge.
	// These usually require user interaction and cannot be auto-solved.
	ChallengeCloudflareTurnstile ChallengeType = "cloudflare_turnstile"
	// ChallengeDataDome is captcha-delivery.com's fingerprint-based blocker.
	// Usually not auto-solvable; detection exists so callers can fail fast.
	ChallengeDataDome ChallengeType = "datadome"
)

// IsAutoSolvable reports whether a challenge type can be solved by waiting.
// Cloudflare UAM's JS challenge resolves itself; Turnstile and DataDome
// require user interaction or specialized bypass tooling.
func (c ChallengeType) IsAutoSolvable() bool {
	return c == ChallengeCloudflareUAM
}

// detectChallengeJS is a JavaScript snippet that inspects the current document
// and returns a string identifying any bot-protection challenge in progress.
// Returned as a simple string so the eval call stays cheap.
//
// Detection order matters: DataDome must be checked BEFORE Cloudflare
// challenge markers because DataDome pages legitimately include Cloudflare
// CDN attributes (data-cfasync, cdn-cgi scripts) without actually being
// protected by a Cloudflare challenge.
const detectChallengeJS = `() => {
  try {
    const title = (document.title || "").toLowerCase();
    const bodyText = (document.body && document.body.innerText || "").toLowerCase();
    const html = document.documentElement.outerHTML || "";

    // --- 1. DataDome (captcha-delivery.com) — checked first because ---
    // DataDome pages often ride on top of Cloudflare CDN infrastructure.
    if (html.indexOf("captcha-delivery.com") !== -1 ||
        html.indexOf("geo.captcha-delivery.com") !== -1 ||
        html.indexOf("datadome") !== -1 ||
        html.indexOf("window.dd=") !== -1 ||
        html.indexOf("var dd={") !== -1) {
      return "datadome";
    }

    // --- 2. Cloudflare Turnstile (interactive widget) ---
    // Only match the specific widget class, NOT the generic "turnstile"
    // word which can appear in unrelated library names or telemetry.
    if (html.indexOf("cf-turnstile") !== -1 ||
        html.indexOf("class=\"cf-turnstile") !== -1) {
      return "cloudflare_turnstile";
    }

    // --- 3. Cloudflare UAM / "Just a moment..." JS challenge ---
    // Title is the most reliable signal.
    if (title === "just a moment..." || title === "just a moment" ||
        title.startsWith("attention required") ||
        bodyText.indexOf("checking if the site connection is secure") !== -1 ||
        bodyText.indexOf("checking your browser before accessing") !== -1) {
      return "cloudflare_uam";
    }

    // Cloudflare challenge markup even if the title was already rewritten.
    // Use very specific paths that only appear on actual challenge pages.
    if (html.indexOf("/cdn-cgi/challenge-platform/h/") !== -1 ||
        html.indexOf("_cf_chl_opt") !== -1 ||
        html.indexOf("cf-im-under-attack") !== -1) {
      return "cloudflare_uam";
    }

    return "";
  } catch (e) {
    return "";
  }
}`

// DetectChallenge returns the bot-protection challenge currently shown on
// the page, or ChallengeNone if the page looks like real content.
func (p *Page) DetectChallenge() (ChallengeType, error) {
	val, err := p.Eval(detectChallengeJS)
	if err != nil {
		return ChallengeNone, fmt.Errorf("gosurfer: detect challenge: %w", err)
	}
	s, _ := val.(string)
	return ChallengeType(strings.TrimSpace(s)), nil
}

// WaitForChallenge polls the page until any auto-solvable challenge has
// cleared or the timeout elapses. It returns:
//   - the challenge type that was initially detected (ChallengeNone if no
//     challenge was present)
//   - the time spent waiting
//   - an error ONLY if an auto-solvable challenge failed to clear in time
//
// Non-auto-solvable challenges (Turnstile, DataDome) are returned without
// an error — the page did load, it just loaded a challenge. The caller
// can inspect the returned ChallengeType to decide what to do.
//
// A timeout of 0 disables waiting (no-op). Callers should pick a value
// appropriate to the challenge — Cloudflare UAM typically resolves in
// 5-15 seconds, rarely longer.
func (p *Page) WaitForChallenge(timeout time.Duration) (ChallengeType, time.Duration, error) {
	if timeout <= 0 {
		return ChallengeNone, 0, nil
	}

	const pollInterval = 500 * time.Millisecond

	start := time.Now()

	// Initial check: is there a challenge at all?
	initial, err := p.DetectChallenge()
	if err != nil {
		return ChallengeNone, time.Since(start), err
	}
	if initial == ChallengeNone {
		return ChallengeNone, time.Since(start), nil
	}
	if !initial.IsAutoSolvable() {
		// Page is on a non-auto-solvable challenge. Return the type so the
		// caller knows but without an error — the page did "load".
		return initial, time.Since(start), nil
	}

	// Poll until the auto-solvable challenge clears or timeout.
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)
		current, cerr := p.DetectChallenge()
		if cerr != nil {
			// Eval errors are common during navigation (page unloads as
			// challenge redirects). Treat as "still running" and retry.
			continue
		}
		if current == ChallengeNone {
			return initial, time.Since(start), nil
		}
	}

	return initial, time.Since(start), fmt.Errorf("gosurfer: challenge %q did not clear within %s", initial, timeout)
}

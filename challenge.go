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
const detectChallengeJS = `() => {
  try {
    const title = (document.title || "").toLowerCase();
    const bodyText = (document.body && document.body.innerText || "").toLowerCase();
    const html = document.documentElement.outerHTML || "";

    // Cloudflare "Just a moment..." / "Checking your browser"
    // Title is the most reliable signal for UAM.
    if (title === "just a moment..." || title === "just a moment" ||
        title.startsWith("attention required") ||
        bodyText.indexOf("checking if the site connection is secure") !== -1 ||
        bodyText.indexOf("checking your browser before accessing") !== -1) {
      // Distinguish UAM (auto-resolving) from Turnstile (interactive).
      if (html.indexOf("cf-turnstile") !== -1 || html.indexOf("turnstile") !== -1) {
        return "cloudflare_turnstile";
      }
      return "cloudflare_uam";
    }

    // Cloudflare challenge markup even if title isn't the usual one.
    if (html.indexOf("challenge-platform") !== -1 ||
        html.indexOf("_cf_chl_opt") !== -1 ||
        html.indexOf("/cdn-cgi/challenge-platform/") !== -1) {
      // Still prefer turnstile classification when present.
      if (html.indexOf("cf-turnstile") !== -1) {
        return "cloudflare_turnstile";
      }
      return "cloudflare_uam";
    }

    // DataDome — captcha-delivery.com.
    if (html.indexOf("captcha-delivery.com") !== -1 ||
        html.indexOf("geo.captcha-delivery.com") !== -1 ||
        html.indexOf("datadome") !== -1) {
      return "datadome";
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
//   - the challenge type that was detected and resolved (ChallengeNone if
//     no challenge was ever detected)
//   - the time spent waiting
//   - an error if the page stayed on a non-auto-solvable challenge or
//     timed out without resolving
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
		return initial, time.Since(start), fmt.Errorf("gosurfer: challenge %q cannot be auto-solved", initial)
	}

	// Poll until the challenge clears or timeout.
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
		if current != initial && !current.IsAutoSolvable() {
			// Challenge escalated to something we can't solve (e.g., UAM
			// failed and became Turnstile). Fail immediately.
			return current, time.Since(start), fmt.Errorf("gosurfer: challenge escalated to %q", current)
		}
	}

	return initial, time.Since(start), fmt.Errorf("gosurfer: challenge %q did not clear within %s", initial, timeout)
}

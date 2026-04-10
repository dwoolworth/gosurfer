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
// and returns a string identifying any bot-protection challenge currently
// blocking access to the page content.
//
// Detection philosophy: distinguish "challenge is blocking me" from
// "challenge scripts are loaded but content is rendered fine". Many
// legitimate sites load DataDome, Turnstile, or Cloudflare challenge
// scripts as background protection WITHOUT actually blocking the user —
// we must not classify those as challenges.
//
// The heuristic:
//  1. Check title for unambiguous challenge markers ("Just a moment...").
//  2. If the body has substantial text content (>250 chars), the page
//     loaded successfully — return no challenge even if protection
//     scripts are detected in HTML.
//  3. Only for empty/near-empty pages, fall back to HTML string
//     matching to identify which protection is active.
const detectChallengeJS = `() => {
  try {
    const title = (document.title || "").toLowerCase();
    const bodyEl = document.body;
    const bodyText = (bodyEl && bodyEl.innerText || "").trim();
    const bodyLen = bodyText.length;

    // --- 1. Unambiguous title markers (always blocking) ---
    if (title === "just a moment..." || title === "just a moment" ||
        title.startsWith("attention required") ||
        title === "one more step") {
      return "cloudflare_uam";
    }
    if (title === "datadome captcha" || title.indexOf("datadome") !== -1) {
      return "datadome";
    }

    // --- 2. Substantial body content = NOT blocked ---
    // If the page rendered real text, it's loaded regardless of what
    // background scripts are present. This prevents false positives on
    // legitimate sites that load Turnstile/DataDome as invisible
    // protection (Kiteworks, cloudflare.com, etc.).
    if (bodyLen > 250) {
      return "";
    }

    // --- 3. Empty/near-empty body: check HTML for specific protection ---
    const html = document.documentElement.outerHTML || "";
    const bodyTextLower = bodyText.toLowerCase();

    // Cloudflare UAM body markers (for mid-challenge state).
    if (bodyTextLower.indexOf("checking if the site connection is secure") !== -1 ||
        bodyTextLower.indexOf("checking your browser before accessing") !== -1 ||
        bodyTextLower.indexOf("verify you are human") !== -1) {
      return "cloudflare_uam";
    }

    // DataDome: specific runtime-injected signatures rather than generic
    // script src. "var dd={" is the DataDome challenge config object;
    // "geo.captcha-delivery.com" is DataDome's challenge host.
    if (html.indexOf("var dd={") !== -1 ||
        html.indexOf("geo.captcha-delivery.com") !== -1) {
      return "datadome";
    }

    // Cloudflare Turnstile: ONLY when it's the only thing on the page
    // (empty body means no surrounding content to read).
    if (html.indexOf('class="cf-turnstile') !== -1 ||
        html.indexOf("'cf-turnstile'") !== -1 ||
        html.indexOf("cf-turnstile-wrapper") !== -1) {
      return "cloudflare_turnstile";
    }

    // Cloudflare challenge-platform markup on an empty page.
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

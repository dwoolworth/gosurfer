// Structured request logging with credential-safe URL sanitization.
//
// Every tool invocation emits a single log line with:
//   - tool name
//   - sanitized URL (credentials stripped)
//   - status (ok, error, timeout, pool_exhausted)
//   - duration in milliseconds
//   - error message (if any)
//
// URL sanitization strips any embedded "user:password@" userinfo before
// logging. This prevents accidentally leaking agent-provided credentials
// into pod logs. We also guard against credentials appearing in query
// strings by redacting common sensitive parameter names.
package main

import (
	"log"
	"net/url"
	"strings"
)

// sensitiveQueryKeys are query parameter names whose values should be
// redacted when logging. Matched case-insensitively against the exact key.
var sensitiveQueryKeys = map[string]bool{
	"password":     true,
	"passwd":       true,
	"pass":         true,
	"pwd":          true,
	"secret":       true,
	"token":        true,
	"access_token": true,
	"auth":         true,
	"authorization": true,
	"api_key":      true,
	"apikey":       true,
	"key":          true,
	"session":      true,
	"sig":          true,
	"signature":    true,
}

// sanitizeURL returns a version of the URL safe to log:
//   - userinfo (user:password@) is removed
//   - common sensitive query parameters have their values redacted
//   - unparseable URLs are redacted entirely to fail safe
func sanitizeURL(raw string) string {
	if raw == "" {
		return ""
	}

	u, err := url.Parse(raw)
	if err != nil {
		// If we cannot parse it, do not risk leaking credentials — redact.
		return "[unparseable-url]"
	}

	// Strip userinfo entirely (never log user:pass@host).
	u.User = nil

	// Redact sensitive query parameters.
	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			if sensitiveQueryKeys[strings.ToLower(k)] {
				q.Set(k, "[REDACTED]")
			}
		}
		u.RawQuery = q.Encode()
	}

	// Preserve fragment as-is (fragments are client-side and rarely sensitive,
	// but if someone is passing a token in a fragment that's a bigger problem).
	return u.String()
}

// logRequest emits a structured log line for a tool invocation.
// status should be one of: "ok", "error", "timeout", "pool_exhausted".
func logRequest(tool, rawURL, status string, durationMs int64, errMsg string) {
	safeURL := sanitizeURL(rawURL)
	if errMsg != "" {
		log.Printf("tool=%s url=%q status=%s duration_ms=%d error=%q",
			tool, safeURL, status, durationMs, errMsg)
	} else {
		log.Printf("tool=%s url=%q status=%s duration_ms=%d",
			tool, safeURL, status, durationMs)
	}
}

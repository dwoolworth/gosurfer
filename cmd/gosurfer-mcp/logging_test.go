package main

import "testing"

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			name: "plain URL",
			in:   "https://example.com/path",
			want: "https://example.com/path",
		},
		{
			name: "strips user:password from userinfo",
			in:   "https://alice:s3cret@example.com/path",
			want: "https://example.com/path",
		},
		{
			name: "strips user-only userinfo",
			in:   "https://alice@example.com/path",
			want: "https://example.com/path",
		},
		{
			name: "strips credentials with port",
			in:   "https://admin:hunter2@example.com:8443/admin",
			want: "https://example.com:8443/admin",
		},
		{
			name: "strips credentials with query",
			in:   "https://bob:pw@api.example.com/v1?foo=bar",
			want: "https://api.example.com/v1?foo=bar",
		},
		{
			name: "redacts password query param",
			in:   "https://example.com/login?user=alice&password=s3cret",
			want: "https://example.com/login?password=%5BREDACTED%5D&user=alice",
		},
		{
			name: "redacts api_key query param",
			in:   "https://example.com/api?api_key=abc123&q=hello",
			want: "https://example.com/api?api_key=%5BREDACTED%5D&q=hello",
		},
		{
			name: "redacts token query param case-insensitive",
			in:   "https://example.com/?Token=xyz",
			want: "https://example.com/?Token=%5BREDACTED%5D",
		},
		{
			name: "strips userinfo AND redacts query token",
			in:   "https://dave:letmein@example.com/api?token=abc",
			want: "https://example.com/api?token=%5BREDACTED%5D",
		},
		{
			name: "unparseable URL is redacted",
			in:   "https://[badhost",
			want: "[unparseable-url]",
		},
		{
			name: "preserves normal query params",
			in:   "https://example.com/search?q=hello+world&lang=en",
			want: "https://example.com/search?lang=en&q=hello+world",
		},
		{
			name: "handles FTP scheme with creds",
			in:   "ftp://user:pass@ftp.example.com/file.txt",
			want: "ftp://ftp.example.com/file.txt",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeURL(tc.in)
			if got != tc.want {
				t.Errorf("sanitizeURL(%q)\n  want: %q\n  got:  %q", tc.in, tc.want, got)
			}
		})
	}
}

// TestSanitizeURL_NeverContainsPassword is a defensive check: no matter what
// username/password combination is provided, the sanitized output must never
// contain the password string literally.
func TestSanitizeURL_NeverContainsPassword(t *testing.T) {
	passwords := []string{
		"simplepass",
		"hunter2",
		"p@ssw0rd!",
		"TOP_SECRET_123",
		"a/b?c=d",
	}
	for _, pw := range passwords {
		raw := "https://user:" + pw + "@example.com/x?y=1"
		got := sanitizeURL(raw)
		if contains(got, pw) {
			t.Errorf("sanitized URL %q still contains password %q", got, pw)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

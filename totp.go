package gosurfer

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// GenerateTOTP generates a 6-digit TOTP code from a base32-encoded secret.
// Implements RFC 6238 with a 30-second time step and HMAC-SHA1.
func GenerateTOTP(secret string) (string, error) {
	// Clean and decode the base32 secret
	secret = strings.ToUpper(strings.TrimSpace(secret))
	secret = strings.ReplaceAll(secret, " ", "")
	secret = strings.ReplaceAll(secret, "-", "")

	// Pad to multiple of 8 for base32
	if pad := len(secret) % 8; pad != 0 {
		secret += strings.Repeat("=", 8-pad)
	}

	key, err := base32.StdEncoding.DecodeString(secret)
	if err != nil {
		return "", fmt.Errorf("gosurfer: invalid TOTP secret: %w", err)
	}

	return generateTOTPFromKey(key, time.Now()), nil
}

// GenerateTOTPAt generates a TOTP code for a specific time.
func GenerateTOTPAt(secret string, t time.Time) (string, error) {
	secret = strings.ToUpper(strings.TrimSpace(secret))
	secret = strings.ReplaceAll(secret, " ", "")
	secret = strings.ReplaceAll(secret, "-", "")
	if pad := len(secret) % 8; pad != 0 {
		secret += strings.Repeat("=", 8-pad)
	}

	key, err := base32.StdEncoding.DecodeString(secret)
	if err != nil {
		return "", fmt.Errorf("gosurfer: invalid TOTP secret: %w", err)
	}

	return generateTOTPFromKey(key, t), nil
}

func generateTOTPFromKey(key []byte, t time.Time) string {
	// Time step: 30 seconds (RFC 6238 default)
	counter := uint64(t.Unix() / 30)

	// Encode counter as big-endian 8 bytes
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	// HMAC-SHA1
	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	hash := mac.Sum(nil)

	// Dynamic truncation (RFC 4226 Section 5.4)
	offset := hash[len(hash)-1] & 0x0f
	code := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	code = code % 1000000

	return fmt.Sprintf("%06d", code)
}

// Secrets manages sensitive data (credentials, TOTP secrets) for the agent.
// Keys ending in "_totp" are automatically treated as TOTP secrets and
// generate fresh codes on each access.
type Secrets struct {
	data map[string]string
}

// NewSecrets creates a Secrets store from a key-value map.
func NewSecrets(data map[string]string) *Secrets {
	return &Secrets{data: data}
}

// Get retrieves a secret value. For keys ending in "_totp", a fresh
// TOTP code is generated from the stored secret.
func (s *Secrets) Get(key string) (string, error) {
	val, ok := s.data[key]
	if !ok {
		return "", fmt.Errorf("secret %q not found", key)
	}

	if strings.HasSuffix(key, "_totp") {
		return GenerateTOTP(val)
	}

	return val, nil
}

// Has returns whether a key exists.
func (s *Secrets) Has(key string) bool {
	_, ok := s.data[key]
	return ok
}

// Keys returns all secret key names.
func (s *Secrets) Keys() []string {
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	return keys
}

// ReplaceInText replaces {{secret_name}} placeholders in text with
// actual secret values (generating TOTP codes for _totp keys).
func (s *Secrets) ReplaceInText(text string) string {
	for key := range s.data {
		placeholder := "{{" + key + "}}"
		if strings.Contains(text, placeholder) {
			val, err := s.Get(key)
			if err == nil {
				text = strings.ReplaceAll(text, placeholder, val)
			}
		}
	}
	return text
}

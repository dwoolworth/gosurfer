package gosurfer

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateTOTP_KnownVector(t *testing.T) {
	// RFC 6238 test vector: secret "12345678901234567890" (ASCII) = base32 "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	// At Unix time 59 (time step 1), the expected TOTP is 287082
	code, err := GenerateTOTPAt(secret, time.Unix(59, 0))
	if err != nil {
		t.Fatalf("GenerateTOTPAt: %v", err)
	}
	if code != "287082" {
		t.Errorf("expected 287082, got %s", code)
	}
}

func TestGenerateTOTP_AnotherVector(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	// At Unix time 1111111109 (time step 37037037), expected is 081804
	code, err := GenerateTOTPAt(secret, time.Unix(1111111109, 0))
	if err != nil {
		t.Fatalf("GenerateTOTPAt: %v", err)
	}
	if code != "081804" {
		t.Errorf("expected 081804, got %s", code)
	}
}

func TestGenerateTOTP_CurrentTime(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP" // common test secret
	code, err := GenerateTOTP(secret)
	if err != nil {
		t.Fatalf("GenerateTOTP: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("expected 6-digit code, got %q", code)
	}
	// Verify all digits
	for _, c := range code {
		if c < '0' || c > '9' {
			t.Errorf("non-digit in TOTP code: %q", code)
			break
		}
	}
}

func TestGenerateTOTP_CleanupInput(t *testing.T) {
	// Spaces, dashes, lowercase should be handled
	code1, err := GenerateTOTP("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	code2, err := GenerateTOTP("jbsw y3dp ehpk 3pxp")
	if err != nil {
		t.Fatal(err)
	}
	code3, err := GenerateTOTP("JBSW-Y3DP-EHPK-3PXP")
	if err != nil {
		t.Fatal(err)
	}
	if code1 != code2 || code1 != code3 {
		t.Errorf("codes should match after cleanup: %s, %s, %s", code1, code2, code3)
	}
}

func TestGenerateTOTP_InvalidSecret(t *testing.T) {
	_, err := GenerateTOTP("!!!invalid!!!")
	if err == nil {
		t.Error("expected error for invalid base32 secret")
	}
}

func TestSecrets_Get(t *testing.T) {
	s := NewSecrets(map[string]string{
		"username":    "admin",
		"password":    "s3cret",
		"mfa_totp":    "JBSWY3DPEHPK3PXP",
	})

	val, err := s.Get("username")
	if err != nil {
		t.Fatal(err)
	}
	if val != "admin" {
		t.Errorf("expected admin, got %s", val)
	}

	// TOTP key should return a 6-digit code, not the secret itself
	code, err := s.Get("mfa_totp")
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 {
		t.Errorf("expected 6-digit TOTP code, got %q", code)
	}
	if code == "JBSWY3DPEHPK3PXP" {
		t.Error("should return generated TOTP, not raw secret")
	}
}

func TestSecrets_GetNotFound(t *testing.T) {
	s := NewSecrets(map[string]string{"a": "b"})
	_, err := s.Get("nonexistent")
	if err == nil {
		t.Error("expected error for missing key")
	}
}

func TestSecrets_Has(t *testing.T) {
	s := NewSecrets(map[string]string{"key": "val"})
	if !s.Has("key") {
		t.Error("expected Has to return true")
	}
	if s.Has("missing") {
		t.Error("expected Has to return false")
	}
}

func TestSecrets_Keys(t *testing.T) {
	s := NewSecrets(map[string]string{"a": "1", "b": "2"})
	keys := s.Keys()
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
}

func TestSecrets_ReplaceInText(t *testing.T) {
	s := NewSecrets(map[string]string{
		"user": "admin",
		"pass": "secret123",
	})

	result := s.ReplaceInText("login {{user}} with {{pass}}")
	if result != "login admin with secret123" {
		t.Errorf("unexpected replacement: %s", result)
	}

	// No placeholder — unchanged
	result2 := s.ReplaceInText("no placeholders here")
	if result2 != "no placeholders here" {
		t.Errorf("should be unchanged: %s", result2)
	}
}

func TestSecrets_ReplaceInText_TOTP(t *testing.T) {
	s := NewSecrets(map[string]string{
		"code_totp": "JBSWY3DPEHPK3PXP",
	})
	result := s.ReplaceInText("Enter code: {{code_totp}}")
	if strings.Contains(result, "{{code_totp}}") {
		t.Error("placeholder should have been replaced")
	}
	if strings.Contains(result, "JBSWY3DPEHPK3PXP") {
		t.Error("should contain TOTP code, not raw secret")
	}
}

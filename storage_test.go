package gosurfer

import (
	"testing"
)

func TestPage_SetAndGetCookie(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}

	if err := page.SetCookie("test_cookie", "cookie_value", "", ""); err != nil {
		t.Fatal(err)
	}

	val, err := page.GetCookie("test_cookie")
	if err != nil {
		t.Fatal(err)
	}
	if val != "cookie_value" {
		t.Errorf("cookie value = %q, want %q", val, "cookie_value")
	}
}

func TestPage_GetCookies(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}

	// Set two cookies
	if err := page.SetCookie("c1", "v1", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := page.SetCookie("c2", "v2", "", ""); err != nil {
		t.Fatal(err)
	}

	cookies, err := page.GetCookies()
	if err != nil {
		t.Fatal(err)
	}
	if len(cookies) < 2 {
		t.Errorf("expected at least 2 cookies, got %d", len(cookies))
	}

	found := map[string]bool{}
	for _, c := range cookies {
		found[c.Name] = true
	}
	if !found["c1"] || !found["c2"] {
		t.Errorf("missing expected cookies, found: %v", found)
	}
}

func TestPage_GetCookie_NotFound(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}

	// Clear to ensure clean state
	_ = page.ClearCookies()

	val, err := page.GetCookie("nonexistent_cookie")
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Errorf("expected empty string for missing cookie, got %q", val)
	}
}

func TestPage_DeleteCookies(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}

	if err := page.SetCookie("to_delete", "value", "", ""); err != nil {
		t.Fatal(err)
	}

	// Verify it exists
	val, err := page.GetCookie("to_delete")
	if err != nil {
		t.Fatal(err)
	}
	if val != "value" {
		t.Fatalf("cookie not set, got %q", val)
	}

	// Delete it
	if err := page.DeleteCookies("to_delete"); err != nil {
		t.Fatal(err)
	}

	// Verify it's gone
	val, err = page.GetCookie("to_delete")
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Errorf("cookie should be deleted, got %q", val)
	}
}

func TestPage_ClearCookies(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}

	// Set several cookies
	if err := page.SetCookie("clear1", "v1", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := page.SetCookie("clear2", "v2", "", ""); err != nil {
		t.Fatal(err)
	}

	// Clear all
	if err := page.ClearCookies(); err != nil {
		t.Fatal(err)
	}

	cookies, err := page.GetCookies()
	if err != nil {
		t.Fatal(err)
	}
	if len(cookies) != 0 {
		t.Errorf("expected 0 cookies after clear, got %d", len(cookies))
	}
}

func TestPage_SetCookies(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}
	_ = page.ClearCookies()

	batch := []Cookie{
		{Name: "bulk1", Value: "val1", Domain: "127.0.0.1", Path: "/"},
		{Name: "bulk2", Value: "val2", Domain: "127.0.0.1", Path: "/"},
		{Name: "bulk3", Value: "val3", Domain: "127.0.0.1", Path: "/"},
	}
	if err := page.SetCookies(batch); err != nil {
		t.Fatal(err)
	}

	cookies, err := page.GetCookies()
	if err != nil {
		t.Fatal(err)
	}
	if len(cookies) < 3 {
		t.Errorf("expected at least 3 cookies after bulk set, got %d", len(cookies))
	}

	found := map[string]string{}
	for _, c := range cookies {
		found[c.Name] = c.Value
	}
	for _, c := range batch {
		if found[c.Name] != c.Value {
			t.Errorf("cookie %s = %q, want %q", c.Name, found[c.Name], c.Value)
		}
	}
}

func TestPage_LocalStorageSetAndGet(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}

	if err := page.LocalStorageSet("ls_key", "ls_value"); err != nil {
		t.Fatal(err)
	}

	val, err := page.LocalStorageGet("ls_key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "ls_value" {
		t.Errorf("localStorage value = %q, want %q", val, "ls_value")
	}
}

func TestPage_LocalStorageAll(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}
	_ = page.LocalStorageClear()

	if err := page.LocalStorageSet("a", "1"); err != nil {
		t.Fatal(err)
	}
	if err := page.LocalStorageSet("b", "2"); err != nil {
		t.Fatal(err)
	}

	items, err := page.LocalStorageAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	if items["a"] != "1" || items["b"] != "2" {
		t.Errorf("items = %v", items)
	}
}

func TestPage_LocalStorageDelete(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}

	if err := page.LocalStorageSet("del_key", "del_value"); err != nil {
		t.Fatal(err)
	}

	if err := page.LocalStorageDelete("del_key"); err != nil {
		t.Fatal(err)
	}

	val, err := page.LocalStorageGet("del_key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Errorf("expected empty after delete, got %q", val)
	}
}

func TestPage_LocalStorageClear(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}

	if err := page.LocalStorageSet("x", "1"); err != nil {
		t.Fatal(err)
	}
	if err := page.LocalStorageSet("y", "2"); err != nil {
		t.Fatal(err)
	}

	if err := page.LocalStorageClear(); err != nil {
		t.Fatal(err)
	}

	items, err := page.LocalStorageAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items after clear, got %d", len(items))
	}
}

func TestPage_LocalStorageGet_NotSet(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}
	_ = page.LocalStorageClear()

	val, err := page.LocalStorageGet("never_set_key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Errorf("expected empty string for unset key, got %q", val)
	}
}

func TestPage_SessionStorageSetAndGet(t *testing.T) {
	page := newPage(t)
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}

	if err := page.SessionStorageSet("ss_key", "ss_value"); err != nil {
		t.Fatal(err)
	}

	val, err := page.SessionStorageGet("ss_key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "ss_value" {
		t.Errorf("sessionStorage value = %q, want %q", val, "ss_value")
	}
}

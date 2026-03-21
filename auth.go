package gosurfer

import (
	"encoding/json"
	"fmt"
	"os"
)

// StorageState captures the full browser storage for a page: cookies and localStorage.
// It can be serialized to JSON for reuse across sessions (e.g., preserving login state).
type StorageState struct {
	Cookies      []Cookie          `json:"cookies"`
	LocalStorage map[string]string `json:"localStorage"`
	Origin       string            `json:"origin"`
}

// SaveStorageState serializes the page's cookies and localStorage to a JSON file.
// Use LoadStorageState to restore the state in a future session.
//
//	// After logging in:
//	page.SaveStorageState("auth.json")
func (p *Page) SaveStorageState(path string) error {
	state, err := p.GetStorageState()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("gosurfer: marshal storage state: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("gosurfer: write storage state: %w", err)
	}
	return nil
}

// GetStorageState captures the page's cookies and localStorage into a StorageState.
func (p *Page) GetStorageState() (*StorageState, error) {
	cookies, err := p.GetCookies()
	if err != nil {
		return nil, fmt.Errorf("gosurfer: get cookies for state: %w", err)
	}

	localStorage, err := p.LocalStorageAll()
	if err != nil {
		// Non-fatal: some pages may not support localStorage
		localStorage = make(map[string]string)
	}

	return &StorageState{
		Cookies:      cookies,
		LocalStorage: localStorage,
		Origin:       p.URL(),
	}, nil
}

// LoadStorageState restores cookies and localStorage from a JSON file.
// The page should be navigated to the relevant origin before calling this.
//
//	page.Navigate("https://example.com")
//	page.LoadStorageState("auth.json")
func (p *Page) LoadStorageState(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("gosurfer: read storage state: %w", err)
	}

	var state StorageState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("gosurfer: parse storage state: %w", err)
	}

	return p.RestoreStorageState(&state)
}

// RestoreStorageState applies a StorageState to the current page.
func (p *Page) RestoreStorageState(state *StorageState) error {
	// Restore cookies
	if len(state.Cookies) > 0 {
		if err := p.SetCookies(state.Cookies); err != nil {
			return fmt.Errorf("gosurfer: restore cookies: %w", err)
		}
	}

	// Restore localStorage
	for key, value := range state.LocalStorage {
		if err := p.LocalStorageSet(key, value); err != nil {
			return fmt.Errorf("gosurfer: restore localStorage key %q: %w", key, err)
		}
	}

	return nil
}

package gosurfer

import (
	"encoding/json"
	"fmt"

	"github.com/go-rod/rod/lib/proto"
)

// Cookie represents a browser cookie.
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires,omitempty"`
	HTTPOnly bool    `json:"httpOnly,omitempty"`
	Secure   bool    `json:"secure,omitempty"`
	SameSite string  `json:"sameSite,omitempty"`
}

// GetCookies returns all cookies for the current page.
func (p *Page) GetCookies() ([]Cookie, error) {
	res, err := proto.NetworkGetCookies{}.Call(p.rod)
	if err != nil {
		return nil, fmt.Errorf("gosurfer: get cookies: %w", err)
	}
	cookies := make([]Cookie, len(res.Cookies))
	for i, c := range res.Cookies {
		cookies[i] = Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  float64(c.Expires),
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
			SameSite: string(c.SameSite),
		}
	}
	return cookies, nil
}

// GetCookie returns a specific cookie by name, or empty string if not found.
func (p *Page) GetCookie(name string) (string, error) {
	cookies, err := p.GetCookies()
	if err != nil {
		return "", err
	}
	for _, c := range cookies {
		if c.Name == name {
			return c.Value, nil
		}
	}
	return "", nil
}

// SetCookie sets a single cookie on the current page.
func (p *Page) SetCookie(name, value, domain, path string) error {
	if domain == "" {
		info, err := p.rod.Info()
		if err != nil {
			return fmt.Errorf("gosurfer: get page info: %w", err)
		}
		// Extract domain from URL
		domain = info.URL
	}
	if path == "" {
		path = "/"
	}

	_, err := proto.NetworkSetCookie{
		Name:   name,
		Value:  value,
		Domain: domain,
		Path:   path,
	}.Call(p.rod)
	if err != nil {
		return fmt.Errorf("gosurfer: set cookie: %w", err)
	}
	return nil
}

// SetCookies sets multiple cookies at once.
func (p *Page) SetCookies(cookies []Cookie) error {
	params := make([]*proto.NetworkCookieParam, len(cookies))
	for i, c := range cookies {
		params[i] = &proto.NetworkCookieParam{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
		}
		if c.Expires > 0 {
			params[i].Expires = proto.TimeSinceEpoch(c.Expires)
		}
	}

	return proto.NetworkSetCookies{Cookies: params}.Call(p.rod)
}

// DeleteCookies deletes cookies matching the given name.
func (p *Page) DeleteCookies(name string) error {
	return proto.NetworkDeleteCookies{Name: name}.Call(p.rod)
}

// ClearCookies deletes all cookies.
func (p *Page) ClearCookies() error {
	cookies, err := p.GetCookies()
	if err != nil {
		return err
	}
	for _, c := range cookies {
		delReq := proto.NetworkDeleteCookies{
			Name:   c.Name,
			Domain: c.Domain,
			Path:   c.Path,
		}
		if err := delReq.Call(p.rod); err != nil {
			return err
		}
	}
	return nil
}

// --- localStorage / sessionStorage ---

// LocalStorageGet retrieves a value from localStorage.
func (p *Page) LocalStorageGet(key string) (string, error) {
	result, err := p.rod.Eval(fmt.Sprintf(`() => localStorage.getItem(%q)`, key))
	if err != nil {
		return "", fmt.Errorf("gosurfer: localStorage get: %w", err)
	}
	if result.Value.Nil() {
		return "", nil
	}
	return result.Value.Str(), nil
}

// LocalStorageSet stores a value in localStorage.
func (p *Page) LocalStorageSet(key, value string) error {
	_, err := p.rod.Eval(fmt.Sprintf(`() => localStorage.setItem(%q, %q)`, key, value))
	if err != nil {
		return fmt.Errorf("gosurfer: localStorage set: %w", err)
	}
	return nil
}

// LocalStorageDelete removes a key from localStorage.
func (p *Page) LocalStorageDelete(key string) error {
	_, err := p.rod.Eval(fmt.Sprintf(`() => localStorage.removeItem(%q)`, key))
	return err
}

// LocalStorageClear clears all localStorage.
func (p *Page) LocalStorageClear() error {
	_, err := p.rod.Eval(`() => localStorage.clear()`)
	return err
}

// LocalStorageAll returns all localStorage key-value pairs.
func (p *Page) LocalStorageAll() (map[string]string, error) {
	result, err := p.rod.Eval(`() => {
		const items = {};
		for (let i = 0; i < localStorage.length; i++) {
			const key = localStorage.key(i);
			items[key] = localStorage.getItem(key);
		}
		return JSON.stringify(items);
	}`)
	if err != nil {
		return nil, fmt.Errorf("gosurfer: localStorage all: %w", err)
	}

	var items map[string]string
	if err := json.Unmarshal([]byte(result.Value.Str()), &items); err != nil {
		return nil, err
	}
	return items, nil
}

// SessionStorageGet retrieves a value from sessionStorage.
func (p *Page) SessionStorageGet(key string) (string, error) {
	result, err := p.rod.Eval(fmt.Sprintf(`() => sessionStorage.getItem(%q)`, key))
	if err != nil {
		return "", fmt.Errorf("gosurfer: sessionStorage get: %w", err)
	}
	if result.Value.Nil() {
		return "", nil
	}
	return result.Value.Str(), nil
}

// SessionStorageSet stores a value in sessionStorage.
func (p *Page) SessionStorageSet(key, value string) error {
	_, err := p.rod.Eval(fmt.Sprintf(`() => sessionStorage.setItem(%q, %q)`, key, value))
	return err
}

package gosurfer

import (
	"fmt"
	"strings"
	"time"
)

// defaultExpectTimeout is the default retry timeout for assertions.
const defaultExpectTimeout = 5 * time.Second

// expectPollInterval is how often assertions are retried.
const expectPollInterval = 100 * time.Millisecond

// Expect creates auto-retrying assertions for a page.
// Assertions retry until they pass or the timeout expires (default 5s).
//
//	expect := gosurfer.Expect(page)
//	err := expect.ToHaveTitle("Dashboard")
//	err = expect.Locator("#btn").ToBeVisible()
func Expect(page *Page, opts ...ExpectOption) *PageExpect {
	e := &PageExpect{page: page, timeout: defaultExpectTimeout}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// ExpectOption configures assertion behavior.
type ExpectOption func(*PageExpect)

// WithTimeout sets the retry timeout for assertions.
func WithTimeout(d time.Duration) ExpectOption {
	return func(e *PageExpect) { e.timeout = d }
}

// PageExpect provides auto-retrying page-level assertions.
type PageExpect struct {
	page    *Page
	timeout time.Duration
}

// retry polls a check function until it passes or the timeout expires.
func (e *PageExpect) retry(check func() error) error {
	deadline := time.Now().Add(e.timeout)
	var lastErr error
	for {
		if err := check(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("gosurfer expect: timed out after %s: %w", e.timeout, lastErr)
		}
		time.Sleep(expectPollInterval)
	}
}

// ToHaveTitle asserts the page title equals the expected string.
func (e *PageExpect) ToHaveTitle(expected string) error {
	return e.retry(func() error {
		title, err := e.page.Title()
		if err != nil {
			return fmt.Errorf("get title: %w", err)
		}
		if title != expected {
			return fmt.Errorf("expected title %q, got %q", expected, title)
		}
		return nil
	})
}

// ToHaveTitleContaining asserts the page title contains the substring.
func (e *PageExpect) ToHaveTitleContaining(substring string) error {
	return e.retry(func() error {
		title, err := e.page.Title()
		if err != nil {
			return fmt.Errorf("get title: %w", err)
		}
		if !strings.Contains(title, substring) {
			return fmt.Errorf("expected title containing %q, got %q", substring, title)
		}
		return nil
	})
}

// ToHaveURL asserts the page URL equals the expected string.
func (e *PageExpect) ToHaveURL(expected string) error {
	return e.retry(func() error {
		url := e.page.URL()
		if url != expected {
			return fmt.Errorf("expected URL %q, got %q", expected, url)
		}
		return nil
	})
}

// ToHaveURLContaining asserts the page URL contains the substring.
func (e *PageExpect) ToHaveURLContaining(substring string) error {
	return e.retry(func() error {
		url := e.page.URL()
		if !strings.Contains(url, substring) {
			return fmt.Errorf("expected URL containing %q, got %q", substring, url)
		}
		return nil
	})
}

// Locator returns a LocatorExpect for assertions on a specific element.
func (e *PageExpect) Locator(selector string) *LocatorExpect {
	return &LocatorExpect{page: e.page, selector: selector, timeout: e.timeout}
}

// LocatorExpect provides auto-retrying element-level assertions.
type LocatorExpect struct {
	page     *Page
	selector string
	timeout  time.Duration
	negate   bool
}

func (le *LocatorExpect) retry(check func() error) error {
	deadline := time.Now().Add(le.timeout)
	var lastErr error
	for {
		err := check()
		if le.negate {
			if err != nil {
				return nil // negated: error means condition is false, which is what we want
			}
			lastErr = fmt.Errorf("expected condition to be false for %q", le.selector)
		} else {
			if err == nil {
				return nil
			}
			lastErr = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("gosurfer expect: timed out after %s: %w", le.timeout, lastErr)
		}
		time.Sleep(expectPollInterval)
	}
}

// Not returns a negated LocatorExpect where all assertions are inverted.
//
//	expect.Locator("#modal").Not().ToBeVisible() // asserts element is NOT visible
func (le *LocatorExpect) Not() *LocatorExpect {
	return &LocatorExpect{
		page:     le.page,
		selector: le.selector,
		timeout:  le.timeout,
		negate:   !le.negate,
	}
}

// ToBeVisible asserts the element is visible on the page.
func (le *LocatorExpect) ToBeVisible() error {
	return le.retry(func() error {
		el, err := le.page.Element(le.selector)
		if err != nil {
			return fmt.Errorf("element %q not found", le.selector)
		}
		visible, err := el.Visible()
		if err != nil {
			return fmt.Errorf("check visibility of %q: %w", le.selector, err)
		}
		if !visible {
			return fmt.Errorf("element %q is not visible", le.selector)
		}
		return nil
	})
}

// ToBeHidden asserts the element is hidden or not in the DOM.
func (le *LocatorExpect) ToBeHidden() error {
	return le.retry(func() error {
		el, err := le.page.Element(le.selector)
		if err != nil {
			return nil // not found = hidden
		}
		visible, err := el.Visible()
		if err != nil {
			return nil // can't check = treat as hidden
		}
		if visible {
			return fmt.Errorf("element %q is visible, expected hidden", le.selector)
		}
		return nil
	})
}

// ToBeEnabled asserts the element is not disabled.
func (le *LocatorExpect) ToBeEnabled() error {
	return le.retry(func() error {
		val, err := le.page.Eval(fmt.Sprintf(
			`() => document.querySelector(%q)?.disabled ?? false`, le.selector))
		if err != nil {
			return fmt.Errorf("check disabled of %q: %w", le.selector, err)
		}
		disabled, _ := val.(bool)
		if disabled {
			return fmt.Errorf("element %q is disabled", le.selector)
		}
		return nil
	})
}

// ToBeDisabled asserts the element has the disabled attribute.
func (le *LocatorExpect) ToBeDisabled() error {
	return le.retry(func() error {
		val, err := le.page.Eval(fmt.Sprintf(
			`() => document.querySelector(%q)?.disabled ?? false`, le.selector))
		if err != nil {
			return fmt.Errorf("check disabled of %q: %w", le.selector, err)
		}
		disabled, _ := val.(bool)
		if !disabled {
			return fmt.Errorf("element %q is not disabled", le.selector)
		}
		return nil
	})
}

// ToHaveText asserts the element's text content equals the expected string.
func (le *LocatorExpect) ToHaveText(expected string) error {
	return le.retry(func() error {
		el, err := le.page.Element(le.selector)
		if err != nil {
			return fmt.Errorf("element %q not found", le.selector)
		}
		text, err := el.Text()
		if err != nil {
			return fmt.Errorf("get text of %q: %w", le.selector, err)
		}
		if strings.TrimSpace(text) != expected {
			return fmt.Errorf("expected text %q, got %q for %q", expected, strings.TrimSpace(text), le.selector)
		}
		return nil
	})
}

// ToContainText asserts the element's text content contains the substring.
func (le *LocatorExpect) ToContainText(substring string) error {
	return le.retry(func() error {
		el, err := le.page.Element(le.selector)
		if err != nil {
			return fmt.Errorf("element %q not found", le.selector)
		}
		text, err := el.Text()
		if err != nil {
			return fmt.Errorf("get text of %q: %w", le.selector, err)
		}
		if !strings.Contains(text, substring) {
			return fmt.Errorf("expected text containing %q, got %q for %q", substring, text, le.selector)
		}
		return nil
	})
}

// ToHaveValue asserts the element's value equals the expected string.
func (le *LocatorExpect) ToHaveValue(expected string) error {
	return le.retry(func() error {
		val, err := le.page.Eval(fmt.Sprintf(
			`() => document.querySelector(%q)?.value ?? ''`, le.selector))
		if err != nil {
			return fmt.Errorf("get value of %q: %w", le.selector, err)
		}
		s, _ := val.(string)
		if s != expected {
			return fmt.Errorf("expected value %q, got %q for %q", expected, s, le.selector)
		}
		return nil
	})
}

// ToHaveAttribute asserts the element has an attribute with the given value.
func (le *LocatorExpect) ToHaveAttribute(name, value string) error {
	return le.retry(func() error {
		el, err := le.page.Element(le.selector)
		if err != nil {
			return fmt.Errorf("element %q not found", le.selector)
		}
		got, err := el.Attribute(name)
		if err != nil {
			return fmt.Errorf("get attribute %q of %q: %w", name, le.selector, err)
		}
		if got != value {
			return fmt.Errorf("expected attribute %q=%q, got %q for %q", name, value, got, le.selector)
		}
		return nil
	})
}

// ToHaveCount asserts the number of elements matching the selector.
func (le *LocatorExpect) ToHaveCount(expected int) error {
	return le.retry(func() error {
		els, err := le.page.Elements(le.selector)
		if err != nil {
			return fmt.Errorf("query %q: %w", le.selector, err)
		}
		if len(els) != expected {
			return fmt.Errorf("expected %d elements matching %q, got %d", expected, le.selector, len(els))
		}
		return nil
	})
}

// ToBeChecked asserts the element (checkbox/radio) is checked.
func (le *LocatorExpect) ToBeChecked() error {
	return le.retry(func() error {
		val, err := le.page.Eval(fmt.Sprintf(
			`() => document.querySelector(%q)?.checked ?? false`, le.selector))
		if err != nil {
			return fmt.Errorf("check state of %q: %w", le.selector, err)
		}
		checked, _ := val.(bool)
		if !checked {
			return fmt.Errorf("element %q is not checked", le.selector)
		}
		return nil
	})
}

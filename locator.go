package gosurfer

import (
	"fmt"
	"strings"

	"github.com/go-rod/rod"
)

// locatorConfig holds options for semantic locator queries.
type locatorConfig struct {
	exact bool
	name  string
}

// LocatorOption configures semantic locator behavior.
type LocatorOption func(*locatorConfig)

// Exact requires an exact text match instead of substring.
func Exact() LocatorOption {
	return func(c *locatorConfig) { c.exact = true }
}

// Name filters elements by their accessible name (for GetByRole).
func Name(name string) LocatorOption {
	return func(c *locatorConfig) { c.name = name }
}

// implicitRoles maps HTML tag names to their default ARIA roles.
var implicitRoles = map[string]string{
	"a":        "link",
	"button":   "button",
	"h1":       "heading",
	"h2":       "heading",
	"h3":       "heading",
	"h4":       "heading",
	"h5":       "heading",
	"h6":       "heading",
	"input":    "textbox",
	"textarea": "textbox",
	"select":   "combobox",
	"img":      "img",
	"nav":      "navigation",
	"main":     "main",
	"header":   "banner",
	"footer":   "contentinfo",
	"aside":    "complementary",
	"form":     "form",
	"table":    "table",
	"ul":       "list",
	"ol":       "list",
	"li":       "listitem",
	"dialog":   "dialog",
	"progress": "progressbar",
	"meter":    "meter",
}

// inputTypeRoles overrides the default "textbox" role for specific input types.
var inputTypeRoles = map[string]string{
	"checkbox": "checkbox",
	"radio":    "radio",
	"button":   "button",
	"submit":   "button",
	"reset":    "button",
	"range":    "slider",
	"number":   "spinbutton",
	"search":   "searchbox",
}

// GetByRole finds the first element matching the given ARIA role.
// Supports both explicit role attributes and implicit roles from HTML semantics.
// Use Name("Submit") to filter by accessible name.
func (p *Page) GetByRole(role string, opts ...LocatorOption) (*Element, error) {
	cfg := &locatorConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	results, err := p.getAllByRole(role, cfg)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		desc := "role=" + role
		if cfg.name != "" {
			desc += ", name=" + cfg.name
		}
		return nil, fmt.Errorf("gosurfer: no element found with %s", desc)
	}
	return results[0], nil
}

// GetAllByRole finds all elements matching the given ARIA role.
func (p *Page) GetAllByRole(role string, opts ...LocatorOption) ([]*Element, error) {
	cfg := &locatorConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return p.getAllByRole(role, cfg)
}

func (p *Page) getAllByRole(role string, cfg *locatorConfig) ([]*Element, error) {
	selectors := buildRoleSelectors(role)
	selector := strings.Join(selectors, ", ")

	rodElements, err := p.rod.Elements(selector)
	if err != nil {
		return nil, fmt.Errorf("gosurfer: get by role: %w", err)
	}

	var results []*Element
	for _, el := range rodElements {
		if cfg.name != "" {
			name := getAccessibleName(el)
			if cfg.exact {
				if name != cfg.name {
					continue
				}
			} else if !strings.Contains(strings.ToLower(name), strings.ToLower(cfg.name)) {
				continue
			}
		}
		results = append(results, &Element{rod: el})
	}
	return results, nil
}

// buildRoleSelectors returns CSS selectors that match elements with the given ARIA role.
func buildRoleSelectors(role string) []string {
	selectors := []string{fmt.Sprintf("[role=%q]", role)}

	for tag, implicitRole := range implicitRoles {
		if implicitRole != role {
			continue
		}
		switch tag {
		case "input":
			for inputType, typeRole := range inputTypeRoles {
				if typeRole == role {
					selectors = append(selectors, fmt.Sprintf("input[type=%q]:not([role])", inputType))
				}
			}
			if role == "textbox" {
				selectors = append(selectors, "input:not([type]):not([role])")
				selectors = append(selectors, "input[type=text]:not([role])")
				selectors = append(selectors, "input[type=email]:not([role])")
				selectors = append(selectors, "input[type=tel]:not([role])")
				selectors = append(selectors, "input[type=url]:not([role])")
				selectors = append(selectors, "input[type=password]:not([role])")
			}
		case "a":
			if role == "link" {
				selectors = append(selectors, "a[href]:not([role])")
			}
		default:
			selectors = append(selectors, tag+":not([role])")
		}
	}

	return selectors
}

// getAccessibleName computes a simplified accessible name for a rod element.
// Follows a simplified version of the WAI-ARIA accessible name computation.
func getAccessibleName(el *rod.Element) string {
	// 1. aria-label takes priority
	ariaLabel, err := el.Attribute("aria-label")
	if err == nil && ariaLabel != nil && *ariaLabel != "" {
		return *ariaLabel
	}

	// 2. Associated <label> via id (for form controls)
	id, _ := el.Attribute("id")
	if id != nil && *id != "" {
		// Use JS to find the label for this element
		val, evalErr := el.Eval(`function() {
			const id = this.id;
			if (!id) return '';
			const label = document.querySelector('label[for="' + id + '"]');
			return label ? label.textContent.trim() : '';
		}`)
		if evalErr == nil && val.Value.Str() != "" {
			return val.Value.Str()
		}
	}

	// 3. Text content (for buttons, links, headings, etc.)
	text, err := el.Text()
	if err == nil && strings.TrimSpace(text) != "" {
		return strings.TrimSpace(text)
	}

	// 4. title attribute
	title, err := el.Attribute("title")
	if err == nil && title != nil && *title != "" {
		return *title
	}

	// 5. placeholder
	ph, err := el.Attribute("placeholder")
	if err == nil && ph != nil {
		return *ph
	}

	return ""
}

// GetByText finds the first element whose text content matches.
// By default uses case-insensitive substring matching; use Exact() for exact match.
func (p *Page) GetByText(text string, opts ...LocatorOption) (*Element, error) {
	cfg := &locatorConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	results, err := p.getAllByText(text, cfg)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("gosurfer: no element found with text %q", text)
	}
	return results[0], nil
}

// GetAllByText finds all elements whose text content matches.
func (p *Page) GetAllByText(text string, opts ...LocatorOption) ([]*Element, error) {
	cfg := &locatorConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return p.getAllByText(text, cfg)
}

func (p *Page) getAllByText(text string, cfg *locatorConfig) ([]*Element, error) {
	jsExact := "false"
	if cfg.exact {
		jsExact = "true"
	}

	rodElements, err := p.rod.ElementsByJS(rod.Eval(
		`(text, exact) => {
			const lower = text.toLowerCase();
			const results = [];
			const walk = document.createTreeWalker(document.body, NodeFilter.SHOW_ELEMENT);
			let node;
			while (node = walk.nextNode()) {
				// Only match leaf-ish elements (those with direct text nodes)
				let directText = '';
				for (const child of node.childNodes) {
					if (child.nodeType === Node.TEXT_NODE) {
						directText += child.textContent;
					}
				}
				directText = directText.trim();
				if (!directText) continue;
				if (exact === 'true') {
					if (directText === text) results.push(node);
				} else {
					if (directText.toLowerCase().includes(lower)) results.push(node);
				}
			}
			return results;
		}`, text, jsExact))
	if err != nil {
		return nil, fmt.Errorf("gosurfer: get by text: %w", err)
	}

	var results []*Element
	for _, el := range rodElements {
		results = append(results, &Element{rod: el})
	}
	return results, nil
}

// GetByLabel finds the first form element associated with a label matching the text.
// Checks <label for="...">, nested inputs inside <label>, and aria-label attributes.
func (p *Page) GetByLabel(text string, opts ...LocatorOption) (*Element, error) {
	cfg := &locatorConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	jsExact := "false"
	if cfg.exact {
		jsExact = "true"
	}

	rodElements, err := p.rod.ElementsByJS(rod.Eval(
		`(text, exact) => {
			const lower = text.toLowerCase();
			const results = [];
			// Check <label> elements
			for (const label of document.querySelectorAll('label')) {
				const lt = label.textContent.trim();
				const match = exact === 'true' ? lt === text : lt.toLowerCase().includes(lower);
				if (!match) continue;
				if (label.htmlFor) {
					const el = document.getElementById(label.htmlFor);
					if (el) { results.push(el); continue; }
				}
				const nested = label.querySelector('input, select, textarea');
				if (nested) results.push(nested);
			}
			// Check aria-label
			for (const el of document.querySelectorAll('[aria-label]')) {
				const al = el.getAttribute('aria-label');
				const match = exact === 'true' ? al === text : al.toLowerCase().includes(lower);
				if (match) results.push(el);
			}
			return results;
		}`, text, jsExact))
	if err != nil {
		return nil, fmt.Errorf("gosurfer: get by label: %w", err)
	}
	if len(rodElements) == 0 {
		return nil, fmt.Errorf("gosurfer: no element found with label %q", text)
	}
	return &Element{rod: rodElements[0]}, nil
}

// GetByPlaceholder finds the first element with a matching placeholder attribute.
func (p *Page) GetByPlaceholder(text string, opts ...LocatorOption) (*Element, error) {
	cfg := &locatorConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	jsExact := "false"
	if cfg.exact {
		jsExact = "true"
	}

	rodElements, err := p.rod.ElementsByJS(rod.Eval(
		`(text, exact) => {
			const lower = text.toLowerCase();
			const results = [];
			for (const el of document.querySelectorAll('[placeholder]')) {
				if (exact === 'true') {
					if (el.placeholder === text) results.push(el);
				} else {
					if (el.placeholder.toLowerCase().includes(lower)) results.push(el);
				}
			}
			return results;
		}`, text, jsExact))
	if err != nil {
		return nil, fmt.Errorf("gosurfer: get by placeholder: %w", err)
	}
	if len(rodElements) == 0 {
		return nil, fmt.Errorf("gosurfer: no element found with placeholder %q", text)
	}
	return &Element{rod: rodElements[0]}, nil
}

// GetByTestID finds the first element with a matching data-testid attribute.
func (p *Page) GetByTestID(id string) (*Element, error) {
	rodElements, err := p.rod.ElementsByJS(rod.Eval(
		`(id) => Array.from(document.querySelectorAll('[data-testid="' + id + '"]'))`, id))
	if err != nil {
		return nil, fmt.Errorf("gosurfer: get by test ID: %w", err)
	}
	if len(rodElements) == 0 {
		return nil, fmt.Errorf("gosurfer: no element found with test ID %q", id)
	}
	return &Element{rod: rodElements[0]}, nil
}

// GetByAltText finds the first element with a matching alt attribute.
func (p *Page) GetByAltText(text string, opts ...LocatorOption) (*Element, error) {
	cfg := &locatorConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	jsExact := "false"
	if cfg.exact {
		jsExact = "true"
	}

	rodElements, err := p.rod.ElementsByJS(rod.Eval(
		`(text, exact) => {
			const lower = text.toLowerCase();
			const results = [];
			for (const el of document.querySelectorAll('[alt]')) {
				if (exact === 'true') {
					if (el.alt === text) results.push(el);
				} else {
					if (el.alt.toLowerCase().includes(lower)) results.push(el);
				}
			}
			return results;
		}`, text, jsExact))
	if err != nil {
		return nil, fmt.Errorf("gosurfer: get by alt text: %w", err)
	}
	if len(rodElements) == 0 {
		return nil, fmt.Errorf("gosurfer: no element found with alt text %q", text)
	}
	return &Element{rod: rodElements[0]}, nil
}

package gosurfer

import (
	"fmt"
	"strings"
)

// DOMState represents the current page state, optimized for LLM consumption.
type DOMState struct {
	// URL is the current page URL.
	URL string `json:"url"`

	// Title is the page title.
	Title string `json:"title"`

	// Tree is the serialized DOM in indexed-element format for LLM consumption.
	// Interactive elements are tagged with [index] prefixes.
	Tree string `json:"tree"`

	// Elements maps element indices to their metadata for action execution.
	Elements map[int]*DOMElement `json:"elements"`

	// Tabs lists all open browser tabs.
	Tabs []TabInfo `json:"tabs,omitempty"`

	// Screenshot is an optional JPEG screenshot of the current viewport.
	Screenshot []byte `json:"-"`

	// ScrollPosition indicates current scroll percentage (0-100).
	ScrollPosition float64 `json:"scroll_position"`

	// PageHeight is the total page height in pixels.
	PageHeight float64 `json:"page_height"`

	// ViewportHeight is the visible viewport height in pixels.
	ViewportHeight float64 `json:"viewport_height"`
}

// DOMElement represents an interactive element extracted from the page.
type DOMElement struct {
	Index        int               `json:"index"`
	Tag          string            `json:"tag"`
	Text         string            `json:"text"`
	Attributes   map[string]string `json:"attributes"`
	Rect         BoundingBox       `json:"rect"`
	IsEditable   bool              `json:"is_editable"`
	IsScrollable bool              `json:"is_scrollable"`
	Depth        int               `json:"depth"`
	CSSSelector  string            `json:"css_selector"`
}

// TabInfo describes an open browser tab.
type TabInfo struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

// DOMService handles DOM extraction and serialization.
type DOMService struct {
	page      *Page
	lastState *DOMState
}

// GetState extracts the current DOM state, serialized for LLM consumption.
func (d *DOMService) GetState() (*DOMState, error) {
	// Execute the extraction script in the page
	result, err := d.page.rod.Eval(domExtractionScript)
	if err != nil {
		return nil, fmt.Errorf("gosurfer: dom extraction: %w", err)
	}

	var extracted extractedDOM
	if err := result.Value.Unmarshal(&extracted); err != nil {
		return nil, fmt.Errorf("gosurfer: parse dom: %w", err)
	}

	// Build DOMState
	elements := make(map[int]*DOMElement, len(extracted.Elements))
	for i := range extracted.Elements {
		el := &extracted.Elements[i]
		elements[el.Index] = el
	}

	state := &DOMState{
		URL:            extracted.URL,
		Title:          extracted.Title,
		Elements:       elements,
		ScrollPosition: extracted.ScrollPosition,
		PageHeight:     extracted.PageHeight,
		ViewportHeight: extracted.ViewportHeight,
	}

	// Populate tab info if browser is available
	if d.page.browser != nil {
		state.Tabs = d.getTabInfo()
	}

	// Serialize to tree format
	state.Tree = d.serialize(extracted.Nodes, elements)
	d.lastState = state
	return state, nil
}

// getTabInfo enumerates all open browser tabs.
func (d *DOMService) getTabInfo() []TabInfo {
	pages, err := d.page.browser.rod.Pages()
	if err != nil {
		return nil
	}
	tabs := make([]TabInfo, 0, len(pages))
	for _, p := range pages {
		info, err := p.Info()
		if err != nil {
			continue
		}
		tid := string(p.TargetID)
		shortID := tid
		if len(tid) > 4 {
			shortID = tid[len(tid)-4:]
		}
		tabs = append(tabs, TabInfo{
			ID:    shortID,
			URL:   info.URL,
			Title: info.Title,
		})
	}
	return tabs
}

// extractedDOM is the raw structure returned by the JS extraction script.
type extractedDOM struct {
	URL            string       `json:"url"`
	Title          string       `json:"title"`
	Elements       []DOMElement `json:"elements"`
	Nodes          []domNode    `json:"nodes"`
	ScrollPosition float64      `json:"scrollPosition"`
	PageHeight     float64      `json:"pageHeight"`
	ViewportHeight float64      `json:"viewportHeight"`
}

// domNode represents a node in the flattened DOM tree for serialization.
type domNode struct {
	Tag          string `json:"tag"`
	Text         string `json:"text"`
	Depth        int    `json:"depth"`
	ElementIndex int    `json:"elementIndex"` // -1 if not interactive
	IsScrollable bool   `json:"isScrollable"`
}

// serialize converts the DOM tree into the indexed text format for LLMs.
func (d *DOMService) serialize(nodes []domNode, elements map[int]*DOMElement) string {
	var b strings.Builder
	b.Grow(4096)

	for _, node := range nodes {
		indent := strings.Repeat("  ", node.Depth)

		if node.ElementIndex >= 0 {
			el, ok := elements[node.ElementIndex]
			if !ok {
				continue
			}
			// Interactive element with index
			prefix := ""
			if node.IsScrollable {
				prefix = "|SCROLL| "
			}
			b.WriteString(fmt.Sprintf("%s%s[%d]<%s", indent, prefix, el.Index, el.Tag))

			// Add key attributes
			for _, attr := range serializableAttrs {
				if val, ok := el.Attributes[attr]; ok && val != "" {
					b.WriteString(fmt.Sprintf(` %s=%q`, attr, val))
				}
			}

			// Add text content (truncated)
			text := truncate(el.Text, 80)
			if text != "" {
				b.WriteString(fmt.Sprintf(">%s</%s>\n", text, el.Tag))
			} else {
				b.WriteString(" />\n")
			}
		} else if node.Text != "" {
			// Non-interactive text node for context
			text := truncate(node.Text, 120)
			if text != "" {
				b.WriteString(fmt.Sprintf("%s%s\n", indent, text))
			}
		}
	}

	return b.String()
}

// actionableTags are elements that need [index] notation because the LLM can act on them.
var actionableTags = map[string]bool{
	"a": true, "button": true, "input": true, "select": true,
	"textarea": true, "details": true, "summary": true,
}

// headingLevel returns the markdown heading prefix for h1-h6 tags, or empty string.
func headingLevel(tag string) string {
	switch tag {
	case "h1":
		return "# "
	case "h2":
		return "## "
	case "h3":
		return "### "
	case "h4":
		return "#### "
	case "h5":
		return "##### "
	case "h6":
		return "###### "
	}
	return ""
}

// serializeFocused converts the DOM tree into a compact format optimized for token efficiency.
// Only actionable elements (links, buttons, inputs) get [index] tags.
// Headings become markdown. Paragraphs and other text become plain text.
// Empty elements are skipped entirely. Duplicate text is deduplicated.
func (d *DOMService) serializeFocused(nodes []domNode, elements map[int]*DOMElement) string {
	var b strings.Builder
	b.Grow(4096)

	// Track recent text for deduplication
	recentTexts := make([]string, 0, 8)
	const dedupeWindow = 5

	addRecent := func(text string) {
		normalized := strings.ToLower(strings.TrimSpace(text))
		if normalized == "" {
			return
		}
		recentTexts = append(recentTexts, normalized)
		if len(recentTexts) > dedupeWindow {
			recentTexts = recentTexts[1:]
		}
	}

	isDuplicate := func(text string) bool {
		normalized := strings.ToLower(strings.TrimSpace(text))
		if normalized == "" || len(normalized) < 10 {
			return false // don't dedupe very short text
		}
		for _, recent := range recentTexts {
			// Skip if this text is contained in a recent line or vice versa
			if strings.Contains(recent, normalized) || strings.Contains(normalized, recent) {
				return true
			}
		}
		return false
	}

	for _, node := range nodes {
		indent := strings.Repeat("  ", node.Depth)

		if node.ElementIndex >= 0 {
			el, ok := elements[node.ElementIndex]
			if !ok {
				continue
			}

			text := truncate(el.Text, 80)

			// Skip empty elements with no meaningful attributes
			if text == "" && !hasActionableAttrs(el) {
				continue
			}

			if actionableTags[el.Tag] || hasInteractiveRole(el) {
				// Actionable element: keep [index]<tag> format (never dedupe these)
				b.WriteString(fmt.Sprintf("%s[%d]<%s", indent, el.Index, el.Tag))
				for _, attr := range focusedAttrs {
					if val, ok := el.Attributes[attr]; ok && val != "" {
						b.WriteString(fmt.Sprintf(` %s=%q`, attr, val))
					}
				}
				if text != "" {
					b.WriteString(fmt.Sprintf(">%s</%s>\n", text, el.Tag))
					addRecent(text)
				} else {
					b.WriteString(" />\n")
				}
			} else if prefix := headingLevel(el.Tag); prefix != "" {
				if text != "" && !isDuplicate(text) {
					b.WriteString(fmt.Sprintf("%s%s%s\n", indent, prefix, text))
					addRecent(text)
				}
			} else if text != "" && !isDuplicate(text) {
				b.WriteString(fmt.Sprintf("%s%s\n", indent, text))
				addRecent(text)
			}
		} else if node.Text != "" {
			text := truncate(node.Text, 120)
			if text == "" || isDuplicate(text) {
				continue
			}
			if prefix := headingLevel(node.Tag); prefix != "" {
				b.WriteString(fmt.Sprintf("%s%s%s\n", indent, prefix, text))
			} else {
				b.WriteString(fmt.Sprintf("%s%s\n", indent, text))
			}
			addRecent(text)
		}
	}

	return b.String()
}

// hasActionableAttrs returns true if the element has attributes worth preserving
// (href, placeholder, value, name, aria-label).
func hasActionableAttrs(el *DOMElement) bool {
	for _, attr := range []string{"href", "placeholder", "value", "name", "aria-label"} {
		if val, ok := el.Attributes[attr]; ok && val != "" {
			return true
		}
	}
	return false
}

// hasInteractiveRole returns true if the element has an ARIA role that makes it actionable.
func hasInteractiveRole(el *DOMElement) bool {
	role, ok := el.Attributes["role"]
	if !ok {
		return false
	}
	switch role {
	case "button", "link", "textbox", "checkbox", "radio", "combobox",
		"listbox", "menuitem", "option", "searchbox", "slider",
		"spinbutton", "switch", "tab":
		return true
	}
	return false
}

// focusedAttrs is a reduced set of attributes for focused serialization.
var focusedAttrs = []string{
	"type", "name", "placeholder", "value", "href",
	"aria-label", "title", "alt", "disabled",
}

// serializableAttrs are HTML attributes included in the LLM-facing serialization.
var serializableAttrs = []string{
	"type", "name", "placeholder", "value", "href", "role",
	"aria-label", "aria-expanded", "aria-checked", "title",
	"alt", "id", "data-testid", "checked", "disabled",
	"contenteditable", "min", "max", "pattern", "required",
	"multiple", "accept", "autocomplete", "selected",
}

// GetFocusedState extracts the DOM state with boilerplate stripped.
// Removes navigation, footers, cookie banners, ad containers, and low-value
// links (terms, privacy, copyright, same-page anchors, social media).
// Focuses on <main>, <article>, [role="main"] content regions.
func (d *DOMService) GetFocusedState() (*DOMState, error) {
	result, err := d.page.rod.Eval(focusedDOMExtractionScript)
	if err != nil {
		return nil, fmt.Errorf("gosurfer: focused dom extraction: %w", err)
	}

	var extracted extractedDOM
	if err := result.Value.Unmarshal(&extracted); err != nil {
		return nil, fmt.Errorf("gosurfer: parse focused dom: %w", err)
	}

	elements := make(map[int]*DOMElement, len(extracted.Elements))
	for i := range extracted.Elements {
		el := &extracted.Elements[i]
		elements[el.Index] = el
	}

	state := &DOMState{
		URL:            extracted.URL,
		Title:          extracted.Title,
		Elements:       elements,
		ScrollPosition: extracted.ScrollPosition,
		PageHeight:     extracted.PageHeight,
		ViewportHeight: extracted.ViewportHeight,
	}

	if d.page.browser != nil {
		state.Tabs = d.getTabInfo()
	}

	state.Tree = d.serializeFocused(extracted.Nodes, elements)
	d.lastState = state
	return state, nil
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	// Collapse whitespace
	fields := strings.Fields(s)
	s = strings.Join(fields, " ")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// domExtractionScript is the JavaScript injected into the page to extract
// the DOM tree with interactive element detection.
const domExtractionScript = `() => {
	const INTERACTIVE_TAGS = new Set([
		'a', 'button', 'input', 'select', 'textarea', 'details', 'summary'
	]);

	const INTERACTIVE_ROLES = new Set([
		'button', 'link', 'textbox', 'checkbox', 'radio', 'combobox',
		'listbox', 'menu', 'menuitem', 'option', 'searchbox', 'slider',
		'spinbutton', 'switch', 'tab', 'treeitem'
	]);

	const SKIP_TAGS = new Set([
		'script', 'style', 'noscript', 'meta', 'link', 'head', 'br', 'hr'
	]);

	const ATTR_LIST = [
		'type', 'name', 'placeholder', 'value', 'href', 'role',
		'aria-label', 'aria-expanded', 'aria-checked', 'title',
		'alt', 'id', 'data-testid', 'checked', 'disabled',
		'contenteditable', 'min', 'max', 'pattern', 'required',
		'multiple', 'accept', 'autocomplete', 'selected'
	];

	function isVisible(el) {
		if (!el.offsetParent && el.tagName !== 'BODY' && el.tagName !== 'HTML') {
			const style = getComputedStyle(el);
			if (style.position !== 'fixed' && style.position !== 'sticky') return false;
		}
		const rect = el.getBoundingClientRect();
		if (rect.width === 0 && rect.height === 0) return false;
		const style = getComputedStyle(el);
		if (style.display === 'none' || style.visibility === 'hidden') return false;
		if (parseFloat(style.opacity) === 0) return false;
		return true;
	}

	function isInteractive(el) {
		const tag = el.tagName.toLowerCase();
		if (INTERACTIVE_TAGS.has(tag)) return true;
		const role = el.getAttribute('role');
		if (role && INTERACTIVE_ROLES.has(role)) return true;
		if (el.getAttribute('contenteditable') === 'true') return true;
		if (el.hasAttribute('onclick') || el.hasAttribute('onmousedown')) return true;
		if (el.tabIndex > 0) return true;
		if (el.tabIndex === 0 && !['BODY', 'HTML', 'DIV', 'SPAN'].includes(el.tagName)) return true;
		try {
			const style = getComputedStyle(el);
			if (style.cursor === 'pointer' && !['BODY', 'HTML'].includes(el.tagName)) return true;
		} catch(e) {}
		return false;
	}

	function isScrollable(el) {
		const style = getComputedStyle(el);
		const overflowY = style.overflowY;
		const overflowX = style.overflowX;
		if (overflowY === 'auto' || overflowY === 'scroll' || overflowX === 'auto' || overflowX === 'scroll') {
			return el.scrollHeight > el.clientHeight || el.scrollWidth > el.clientWidth;
		}
		return false;
	}

	function getAttributes(el) {
		const attrs = {};
		for (const name of ATTR_LIST) {
			const val = el.getAttribute(name);
			if (val !== null && val !== '') {
				attrs[name] = val;
			}
		}
		// For inputs, get the live value
		if (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT') {
			attrs['value'] = el.value || '';
		}
		return attrs;
	}

	function getCSSSelector(el) {
		if (el.id) return '#' + CSS.escape(el.id);
		let path = [];
		let current = el;
		while (current && current !== document.body) {
			let selector = current.tagName.toLowerCase();
			if (current.id) {
				path.unshift('#' + CSS.escape(current.id));
				break;
			}
			const parent = current.parentElement;
			if (parent) {
				const siblings = Array.from(parent.children).filter(c => c.tagName === current.tagName);
				if (siblings.length > 1) {
					const idx = siblings.indexOf(current) + 1;
					selector += ':nth-of-type(' + idx + ')';
				}
			}
			path.unshift(selector);
			current = current.parentElement;
		}
		return path.join(' > ');
	}

	function getDirectText(el) {
		let text = '';
		for (const child of el.childNodes) {
			if (child.nodeType === Node.TEXT_NODE) {
				text += child.textContent;
			}
		}
		return text.trim();
	}

	const elements = [];
	const nodes = [];
	let idx = 0;

	function walk(el, depth, maxDepth) {
		if (depth > (maxDepth || 50)) return;
		if (!el || SKIP_TAGS.has(el.tagName?.toLowerCase())) return;
		if (el.nodeType !== Node.ELEMENT_NODE) return;
		if (!isVisible(el)) return;

		const tag = el.tagName.toLowerCase();
		const scrollable = isScrollable(el);
		const interactive = isInteractive(el);
		const directText = getDirectText(el);

		if (interactive) {
			const rect = el.getBoundingClientRect();
			const elementData = {
				index: idx,
				tag: tag,
				text: (el.textContent || '').trim().substring(0, 200),
				attributes: getAttributes(el),
				rect: { x: rect.x, y: rect.y, width: rect.width, height: rect.height },
				is_editable: el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName),
				is_scrollable: scrollable,
				depth: depth,
				css_selector: getCSSSelector(el)
			};
			elements.push(elementData);
			nodes.push({
				tag: tag,
				text: directText,
				depth: depth,
				elementIndex: idx,
				isScrollable: scrollable
			});
			idx++;
		} else if (directText && directText.length > 1) {
			// Non-interactive text for context
			nodes.push({
				tag: tag,
				text: directText,
				depth: depth,
				elementIndex: -1,
				isScrollable: scrollable
			});
		}

		// Walk child elements
		for (const child of el.children) {
			walk(child, depth + 1, maxDepth);
		}

		// Pierce shadow DOM
		if (el.shadowRoot) {
			nodes.push({
				tag: '|SHADOW|',
				text: 'Shadow DOM content:',
				depth: depth + 1,
				elementIndex: -1,
				isScrollable: false
			});
			for (const child of el.shadowRoot.children) {
				walk(child, depth + 2, maxDepth);
			}
		}

		// Descend into same-origin iframes
		if (tag === 'iframe') {
			try {
				const iframeDoc = el.contentDocument;
				if (iframeDoc && iframeDoc.body) {
					nodes.push({
						tag: '|IFRAME|',
						text: 'Iframe: ' + (el.src || el.name || ''),
						depth: depth + 1,
						elementIndex: -1,
						isScrollable: false
					});
					walk(iframeDoc.body, depth + 2, maxDepth);
				}
			} catch(e) {
				// Cross-origin iframe - skip (handled separately via CDP)
				nodes.push({
					tag: '|IFRAME-CROSS-ORIGIN|',
					text: 'Cross-origin iframe: ' + (el.src || ''),
					depth: depth + 1,
					elementIndex: -1,
					isScrollable: false
				});
			}
		}
	}

	walk(document.body, 0, 50);

	const scrollTop = document.documentElement.scrollTop || document.body.scrollTop;
	const scrollHeight = document.documentElement.scrollHeight;
	const viewportHeight = window.innerHeight;
	const scrollPosition = scrollHeight > viewportHeight
		? (scrollTop / (scrollHeight - viewportHeight)) * 100
		: 0;

	return {
		url: location.href,
		title: document.title,
		elements: elements,
		nodes: nodes,
		scrollPosition: Math.round(scrollPosition),
		pageHeight: scrollHeight,
		viewportHeight: viewportHeight
	};
}`

// focusedDOMExtractionScript is a variant that strips boilerplate regions and
// low-value links before extraction, dramatically reducing token usage.
const focusedDOMExtractionScript = `() => {
	const INTERACTIVE_TAGS = new Set([
		'a', 'button', 'input', 'select', 'textarea', 'details', 'summary'
	]);

	const INTERACTIVE_ROLES = new Set([
		'button', 'link', 'textbox', 'checkbox', 'radio', 'combobox',
		'listbox', 'menu', 'menuitem', 'option', 'searchbox', 'slider',
		'spinbutton', 'switch', 'tab', 'treeitem'
	]);

	const SKIP_TAGS = new Set([
		'script', 'style', 'noscript', 'meta', 'link', 'head', 'br', 'hr'
	]);

	// Tags whose entire subtree is boilerplate
	const BOILERPLATE_TAGS = new Set(['nav', 'footer', 'aside']);

	// Roles whose entire subtree is boilerplate
	const BOILERPLATE_ROLES = new Set([
		'banner', 'contentinfo', 'complementary', 'navigation'
	]);

	// Classes/IDs that indicate boilerplate containers
	const BOILERPLATE_PATTERNS = /cookie|consent|gdpr|banner|newsletter|popup|modal|overlay|sidebar|widget|ad-|ads-|advert|social|share|follow/i;

	// Href patterns for low-value links
	const JUNK_HREF_PATTERNS = [
		/terms/i, /privacy/i, /cookie/i, /legal/i, /copyright/i,
		/disclaimer/i, /gdpr/i, /\/tos\b/i, /policy/i, /sitemap/i,
		/accessibility/i, /contact-us/i,
		/facebook\.com/i, /twitter\.com/i, /x\.com\/(?!.*\/status)/i,
		/instagram\.com/i, /linkedin\.com/i, /youtube\.com/i,
		/tiktok\.com/i, /pinterest\.com/i, /reddit\.com/i,
		/^javascript:/i,
		/^#$/  // bare hash (same-page anchor with no target)
		// Note: mailto: and tel: are kept — they're legitimate contact links
		// when they survive boilerplate pruning (nav/footer already stripped)
	];

	const ATTR_LIST = [
		'type', 'name', 'placeholder', 'value', 'href', 'role',
		'aria-label', 'aria-expanded', 'aria-checked', 'title',
		'alt', 'id', 'data-testid', 'checked', 'disabled',
		'contenteditable', 'min', 'max', 'pattern', 'required',
		'multiple', 'accept', 'autocomplete', 'selected'
	];

	function isVisible(el) {
		if (!el.offsetParent && el.tagName !== 'BODY' && el.tagName !== 'HTML') {
			const style = getComputedStyle(el);
			if (style.position !== 'fixed' && style.position !== 'sticky') return false;
		}
		const rect = el.getBoundingClientRect();
		if (rect.width === 0 && rect.height === 0) return false;
		const style = getComputedStyle(el);
		if (style.display === 'none' || style.visibility === 'hidden') return false;
		if (parseFloat(style.opacity) === 0) return false;
		return true;
	}

	function isBoilerplate(el) {
		const tag = el.tagName.toLowerCase();
		if (BOILERPLATE_TAGS.has(tag)) return true;
		const role = el.getAttribute('role');
		if (role && BOILERPLATE_ROLES.has(role)) return true;
		const id = el.id || '';
		const cls = el.className || '';
		const idAndClass = id + ' ' + (typeof cls === 'string' ? cls : '');
		if (BOILERPLATE_PATTERNS.test(idAndClass)) return true;
		return false;
	}

	function isJunkLink(el) {
		if (el.tagName !== 'A') return false;
		const href = el.getAttribute('href') || '';
		// Same-page anchors (just a hash)
		if (href === '#' || href === '') return true;
		// Same-page section links
		if (href.startsWith('#') && href.length > 1) return true;
		for (const pattern of JUNK_HREF_PATTERNS) {
			if (pattern.test(href)) return true;
		}
		// Links whose text is very short and generic
		const text = (el.textContent || '').trim().toLowerCase();
		const junkTexts = ['terms', 'privacy', 'legal', 'cookie', 'copyright',
			'share', 'tweet', 'follow', 'like', 'pin', 'subscribe'];
		if (junkTexts.includes(text)) return true;
		return false;
	}

	function isInteractive(el) {
		const tag = el.tagName.toLowerCase();
		if (INTERACTIVE_TAGS.has(tag)) return true;
		const role = el.getAttribute('role');
		if (role && INTERACTIVE_ROLES.has(role)) return true;
		if (el.getAttribute('contenteditable') === 'true') return true;
		if (el.hasAttribute('onclick') || el.hasAttribute('onmousedown')) return true;
		if (el.tabIndex > 0) return true;
		if (el.tabIndex === 0 && !['BODY', 'HTML', 'DIV', 'SPAN'].includes(el.tagName)) return true;
		try {
			const style = getComputedStyle(el);
			if (style.cursor === 'pointer' && !['BODY', 'HTML'].includes(el.tagName)) return true;
		} catch(e) {}
		return false;
	}

	function isScrollable(el) {
		const style = getComputedStyle(el);
		const overflowY = style.overflowY;
		const overflowX = style.overflowX;
		if (overflowY === 'auto' || overflowY === 'scroll' || overflowX === 'auto' || overflowX === 'scroll') {
			return el.scrollHeight > el.clientHeight || el.scrollWidth > el.clientWidth;
		}
		return false;
	}

	function getAttributes(el) {
		const attrs = {};
		for (const name of ATTR_LIST) {
			const val = el.getAttribute(name);
			if (val !== null && val !== '') {
				attrs[name] = val;
			}
		}
		if (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT') {
			attrs['value'] = el.value || '';
		}
		return attrs;
	}

	function getCSSSelector(el) {
		if (el.id) return '#' + CSS.escape(el.id);
		let path = [];
		let current = el;
		while (current && current !== document.body) {
			let selector = current.tagName.toLowerCase();
			if (current.id) {
				path.unshift('#' + CSS.escape(current.id));
				break;
			}
			const parent = current.parentElement;
			if (parent) {
				const siblings = Array.from(parent.children).filter(c => c.tagName === current.tagName);
				if (siblings.length > 1) {
					const idx = siblings.indexOf(current) + 1;
					selector += ':nth-of-type(' + idx + ')';
				}
			}
			path.unshift(selector);
			current = current.parentElement;
		}
		return path.join(' > ');
	}

	function getDirectText(el) {
		let text = '';
		for (const child of el.childNodes) {
			if (child.nodeType === Node.TEXT_NODE) {
				text += child.textContent;
			}
		}
		return text.trim();
	}

	const elements = [];
	const nodes = [];
	let idx = 0;

	function walk(el, depth, maxDepth) {
		if (depth > (maxDepth || 50)) return;
		if (!el || SKIP_TAGS.has(el.tagName?.toLowerCase())) return;
		if (el.nodeType !== Node.ELEMENT_NODE) return;
		if (!isVisible(el)) return;

		// Skip boilerplate subtrees entirely
		if (depth > 0 && isBoilerplate(el)) return;

		const tag = el.tagName.toLowerCase();
		const scrollable = isScrollable(el);
		const interactive = isInteractive(el);
		const directText = getDirectText(el);

		if (interactive) {
			// Skip junk links
			if (isJunkLink(el)) return;

			const rect = el.getBoundingClientRect();
			const elementData = {
				index: idx,
				tag: tag,
				text: (el.textContent || '').trim().substring(0, 200),
				attributes: getAttributes(el),
				rect: { x: rect.x, y: rect.y, width: rect.width, height: rect.height },
				is_editable: el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName),
				is_scrollable: scrollable,
				depth: depth,
				css_selector: getCSSSelector(el)
			};
			elements.push(elementData);
			nodes.push({
				tag: tag,
				text: directText,
				depth: depth,
				elementIndex: idx,
				isScrollable: scrollable
			});
			idx++;
		} else if (directText && directText.length > 1) {
			nodes.push({
				tag: tag,
				text: directText,
				depth: depth,
				elementIndex: -1,
				isScrollable: scrollable
			});
		}

		for (const child of el.children) {
			walk(child, depth + 1, maxDepth);
		}

		if (el.shadowRoot) {
			nodes.push({
				tag: '|SHADOW|',
				text: 'Shadow DOM content:',
				depth: depth + 1,
				elementIndex: -1,
				isScrollable: false
			});
			for (const child of el.shadowRoot.children) {
				walk(child, depth + 2, maxDepth);
			}
		}

		if (tag === 'iframe') {
			try {
				const iframeDoc = el.contentDocument;
				if (iframeDoc && iframeDoc.body) {
					nodes.push({
						tag: '|IFRAME|',
						text: 'Iframe: ' + (el.src || el.name || ''),
						depth: depth + 1,
						elementIndex: -1,
						isScrollable: false
					});
					walk(iframeDoc.body, depth + 2, maxDepth);
				}
			} catch(e) {
				nodes.push({
					tag: '|IFRAME-CROSS-ORIGIN|',
					text: 'Cross-origin iframe: ' + (el.src || ''),
					depth: depth + 1,
					elementIndex: -1,
					isScrollable: false
				});
			}
		}
	}

	// Prefer content regions if they exist
	const contentSelectors = [
		'main', '[role="main"]', 'article',
		'#main-content', '#content', '#page-content',
		'.main-content', '.content', '.page-content',
		'.main', '#main', '.site-content', '.page-wrapper'
	];
	let contentRoot = null;
	for (const sel of contentSelectors) {
		const el = document.querySelector(sel);
		if (el && el.children.length > 0) {
			contentRoot = el;
			break;
		}
	}

	if (contentRoot) {
		walk(contentRoot, 0, 50);
	}

	// If no content region found or it yielded too few elements,
	// walk the full body but still filter boilerplate
	if (!contentRoot || elements.length < 3) {
		elements.length = 0;
		nodes.length = 0;
		idx = 0;
		walk(document.body, 0, 50);
	}

	// If STILL too few elements, the boilerplate filter may be too aggressive.
	// Re-walk with only tag-based boilerplate detection (no class/ID patterns).
	if (elements.length < 5) {
		elements.length = 0;
		nodes.length = 0;
		idx = 0;
		// Temporarily relax boilerplate detection
		const origIsBoilerplate = isBoilerplate;
		isBoilerplate = function(el) {
			// Only strip by tag name, not class/ID patterns
			const tag = el.tagName.toLowerCase();
			if (BOILERPLATE_TAGS.has(tag)) return true;
			const role = el.getAttribute('role');
			if (role && BOILERPLATE_ROLES.has(role)) return true;
			return false;
		};
		walk(document.body, 0, 50);
		isBoilerplate = origIsBoilerplate;
	}

	// Second pass: extract internal navigation links from boilerplate regions
	// (nav, footer, aside) that were skipped in the main walk.
	// Only keeps same-origin links that aren't junk (terms, privacy, social, etc.)
	const origin = location.origin;
	const seenHrefs = new Set();
	// Track hrefs already captured in main content
	for (const el of elements) {
		if (el.attributes && el.attributes.href) {
			seenHrefs.add(el.attributes.href);
		}
	}

	const EXTERNAL_DOMAINS = /facebook\.com|twitter\.com|x\.com|instagram\.com|linkedin\.com|youtube\.com|tiktok\.com|pinterest\.com|reddit\.com|github\.com/i;

	const boilerplateRegions = document.querySelectorAll('nav, footer, aside, [role="navigation"], [role="contentinfo"], [role="banner"]');
	const siteLinks = [];
	for (const region of boilerplateRegions) {
		for (const a of region.querySelectorAll('a[href]')) {
			const href = a.getAttribute('href') || '';
			const text = (a.textContent || '').trim();
			if (!text || text.length < 2) continue;

			// Skip if already in main content
			if (seenHrefs.has(href)) continue;

			// Skip junk hrefs
			let isJunk = false;
			for (const pattern of JUNK_HREF_PATTERNS) {
				if (pattern.test(href)) { isJunk = true; break; }
			}
			if (isJunk) continue;

			// Skip junk link text
			const lowerText = text.toLowerCase();
			const junkTexts = ['terms', 'privacy', 'legal', 'cookie', 'copyright',
				'share', 'tweet', 'follow', 'like', 'pin', 'subscribe'];
			if (junkTexts.includes(lowerText)) continue;

			// Skip external links (social media, etc.)
			try {
				const resolved = new URL(href, origin);
				if (resolved.origin !== origin) {
					if (EXTERNAL_DOMAINS.test(href)) continue;
					// Keep non-social external links (could be relevant)
				}
			} catch(e) { continue; }

			// Skip same-page anchors
			if (href === '#' || (href.startsWith('#') && href.length > 1)) continue;

			seenHrefs.add(href);
			siteLinks.push({ href, text });
		}
	}

	// Add site navigation links as a compact section
	if (siteLinks.length > 0) {
		nodes.push({
			tag: 'h2',
			text: 'Site Navigation',
			depth: 0,
			elementIndex: -1,
			isScrollable: false
		});
		for (const link of siteLinks) {
			const rect = { x: 0, y: 0, width: 0, height: 0 };
			elements.push({
				index: idx,
				tag: 'a',
				text: link.text,
				attributes: { href: link.href },
				rect: rect,
				is_editable: false,
				is_scrollable: false,
				depth: 1,
				css_selector: 'a[href="' + link.href.replace(/"/g, '\\"') + '"]'
			});
			nodes.push({
				tag: 'a',
				text: link.text,
				depth: 1,
				elementIndex: idx,
				isScrollable: false
			});
			idx++;
		}
	}

	const scrollTop = document.documentElement.scrollTop || document.body.scrollTop;
	const scrollHeight = document.documentElement.scrollHeight;
	const viewportHeight = window.innerHeight;
	const scrollPosition = scrollHeight > viewportHeight
		? (scrollTop / (scrollHeight - viewportHeight)) * 100
		: 0;

	return {
		url: location.href,
		title: document.title,
		elements: elements,
		nodes: nodes,
		scrollPosition: Math.round(scrollPosition),
		pageHeight: scrollHeight,
		viewportHeight: viewportHeight
	};
}`

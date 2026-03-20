package gosurfer

import (
	"strings"
	"testing"
)

func TestSerialize_InteractiveElements(t *testing.T) {
	d := &DOMService{}
	elements := map[int]*DOMElement{
		0: {Index: 0, Tag: "button", Text: "Submit", Attributes: map[string]string{"type": "submit"}},
		1: {Index: 1, Tag: "input", Text: "", Attributes: map[string]string{"type": "text", "placeholder": "Search..."}},
		2: {Index: 2, Tag: "a", Text: "About Us", Attributes: map[string]string{"href": "/about"}},
	}
	nodes := []domNode{
		{Tag: "button", Text: "Submit", Depth: 0, ElementIndex: 0},
		{Tag: "input", Text: "", Depth: 0, ElementIndex: 1},
		{Tag: "a", Text: "About Us", Depth: 1, ElementIndex: 2},
	}

	result := d.serialize(nodes, elements)

	if !strings.Contains(result, "[0]<button") {
		t.Error("should contain indexed button element")
	}
	if !strings.Contains(result, "[1]<input") {
		t.Error("should contain indexed input element")
	}
	if !strings.Contains(result, "[2]<a") {
		t.Error("should contain indexed link element")
	}
	if !strings.Contains(result, `placeholder="Search..."`) {
		t.Error("should contain placeholder attribute")
	}
	if !strings.Contains(result, `href="/about"`) {
		t.Error("should contain href attribute")
	}
}

func TestSerialize_TextNodes(t *testing.T) {
	d := &DOMService{}
	nodes := []domNode{
		{Tag: "h1", Text: "Welcome to the site", Depth: 0, ElementIndex: -1},
		{Tag: "p", Text: "Some paragraph text", Depth: 1, ElementIndex: -1},
	}

	result := d.serialize(nodes, map[int]*DOMElement{})

	if !strings.Contains(result, "Welcome to the site") {
		t.Error("should contain h1 text")
	}
	if !strings.Contains(result, "Some paragraph text") {
		t.Error("should contain paragraph text")
	}
}

func TestSerialize_Indentation(t *testing.T) {
	d := &DOMService{}
	elements := map[int]*DOMElement{
		0: {Index: 0, Tag: "a", Text: "Link", Attributes: map[string]string{}},
	}
	nodes := []domNode{
		{Tag: "div", Text: "Container", Depth: 0, ElementIndex: -1},
		{Tag: "a", Text: "Link", Depth: 2, ElementIndex: 0},
	}

	result := d.serialize(nodes, elements)
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) < 2 {
		t.Fatal("expected at least 2 lines")
	}
	// The link at depth 2 should have more indentation than the div at depth 0
	if !strings.HasPrefix(lines[1], "    ") {
		t.Error("deeper element should be indented")
	}
}

func TestSerialize_ScrollableMarker(t *testing.T) {
	d := &DOMService{}
	elements := map[int]*DOMElement{
		0: {Index: 0, Tag: "div", Text: "Scrollable", Attributes: map[string]string{}},
	}
	nodes := []domNode{
		{Tag: "div", Text: "Scrollable", Depth: 0, ElementIndex: 0, IsScrollable: true},
	}

	result := d.serialize(nodes, elements)
	if !strings.Contains(result, "|SCROLL|") {
		t.Error("should contain SCROLL marker for scrollable elements")
	}
}

func TestSerialize_EmptyNodes(t *testing.T) {
	d := &DOMService{}
	result := d.serialize(nil, map[int]*DOMElement{})
	if result != "" {
		t.Errorf("empty nodes should produce empty string, got %q", result)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"  spaces  everywhere  ", 20, "spaces everywhere"},
		{"", 5, ""},
		{"  \t\n  ", 5, ""},
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
		}
	}
}

func TestSerializableAttrs_HasExpectedEntries(t *testing.T) {
	required := []string{"type", "name", "placeholder", "value", "href", "role", "aria-label", "id"}
	for _, attr := range required {
		found := false
		for _, a := range serializableAttrs {
			if a == attr {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("serializableAttrs missing %q", attr)
		}
	}
}

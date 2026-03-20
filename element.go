package gosurfer

import (
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// BoundingBox represents an element's position and size.
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Element wraps a DOM element with interaction methods.
type Element struct {
	rod   *rod.Element
	page  *Page
	Index int // Index in DOM serialization (set by DOMState)
}

// Click clicks the element. It waits for the element to be visible and stable.
func (e *Element) Click() error {
	if err := e.rod.ScrollIntoView(); err != nil {
		return fmt.Errorf("gosurfer: scroll into view: %w", err)
	}
	return e.rod.Click(proto.InputMouseButtonLeft, 1)
}

// DoubleClick double-clicks the element.
func (e *Element) DoubleClick() error {
	if err := e.rod.ScrollIntoView(); err != nil {
		return fmt.Errorf("gosurfer: scroll into view: %w", err)
	}
	return e.rod.Click(proto.InputMouseButtonLeft, 2)
}

// Type types text into the element. It clears existing text first if clear is true.
func (e *Element) Type(text string) error {
	return e.rod.Input(text)
}

// Clear clears the element's value by selecting all text and deleting it.
func (e *Element) Clear() error {
	return e.rod.SelectAllText()
}

// ClearAndType clears the element then types new text.
func (e *Element) ClearAndType(text string) error {
	if err := e.Clear(); err != nil {
		return err
	}
	// Delete selected text
	if err := e.page.rod.Keyboard.Press(input.Backspace); err != nil {
		return err
	}
	return e.Type(text)
}

// Text returns the visible text content of the element.
func (e *Element) Text() (string, error) {
	return e.rod.Text()
}

// HTML returns the element's outer HTML.
func (e *Element) HTML() (string, error) {
	return e.rod.HTML()
}

// Attribute returns the value of an HTML attribute.
func (e *Element) Attribute(name string) (string, error) {
	attr, err := e.rod.Attribute(name)
	if err != nil {
		return "", fmt.Errorf("gosurfer: attribute %q: %w", name, err)
	}
	if attr == nil {
		return "", nil
	}
	return *attr, nil
}

// Visible returns whether the element is visible.
func (e *Element) Visible() (bool, error) {
	visible, err := e.rod.Visible()
	if err != nil {
		return false, err
	}
	return visible, nil
}

// ScrollIntoView scrolls the element into the viewport.
func (e *Element) ScrollIntoView() error {
	return e.rod.ScrollIntoView()
}

// Screenshot captures a screenshot of just this element.
func (e *Element) Screenshot() ([]byte, error) {
	buf, err := e.rod.Screenshot(proto.PageCaptureScreenshotFormatPng, 0)
	if err != nil {
		return nil, fmt.Errorf("gosurfer: element screenshot: %w", err)
	}
	return buf, nil
}

// BBox returns the element's bounding box in viewport coordinates.
func (e *Element) BBox() (*BoundingBox, error) {
	shape, err := e.rod.Shape()
	if err != nil {
		return nil, fmt.Errorf("gosurfer: bounding box: %w", err)
	}
	box := shape.Box()
	return &BoundingBox{
		X:      box.X,
		Y:      box.Y,
		Width:  box.Width,
		Height: box.Height,
	}, nil
}

// SelectOption selects a dropdown option by its visible text.
func (e *Element) SelectOption(texts ...string) error {
	return e.rod.Select(texts, true, rod.SelectorTypeText)
}

// SelectOptionByValue selects a dropdown option by its value attribute.
func (e *Element) SelectOptionByValue(values ...string) error {
	return e.rod.Select(values, true, rod.SelectorTypeCSSSector)
}

// UploadFile sets files on a file input element.
func (e *Element) UploadFile(paths ...string) error {
	return e.rod.SetFiles(paths)
}

// Focus sets focus on the element.
func (e *Element) Focus() error {
	return e.rod.Focus()
}

// Hover moves the mouse over the element.
func (e *Element) Hover() error {
	return e.rod.Hover()
}

// WaitVisible waits until the element becomes visible.
func (e *Element) WaitVisible() error {
	return e.rod.WaitVisible()
}

// WaitStable waits until the element's position stops changing.
func (e *Element) WaitStable() error {
	return e.rod.WaitStable(300)
}

// Frame returns a Page representing the content of this iframe element.
// Panics if the element is not an iframe.
func (e *Element) Frame() (*Page, error) {
	frame, err := e.rod.Frame()
	if err != nil {
		return nil, fmt.Errorf("gosurfer: element frame: %w", err)
	}
	pg := &Page{rod: frame, browser: e.page.browser}
	pg.dom = &DOMService{page: pg}
	return pg, nil
}

// ShadowRoot returns the shadow root of this element for querying shadow DOM.
func (e *Element) ShadowRoot() (*Element, error) {
	root, err := e.rod.ShadowRoot()
	if err != nil {
		return nil, fmt.Errorf("gosurfer: shadow root: %w", err)
	}
	return &Element{rod: root, page: e.page}, nil
}

// Rod returns the underlying rod.Element for advanced usage.
func (e *Element) Rod() *rod.Element {
	return e.rod
}

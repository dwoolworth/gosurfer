package gosurfer

import (
	"fmt"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// DragTo drags this element to a target element.
func (e *Element) DragTo(target *Element) error {
	// Get source center
	srcBox, err := e.BBox()
	if err != nil {
		return fmt.Errorf("gosurfer: drag source bbox: %w", err)
	}
	srcX := srcBox.X + srcBox.Width/2
	srcY := srcBox.Y + srcBox.Height/2

	// Get target center
	tgtBox, err := target.BBox()
	if err != nil {
		return fmt.Errorf("gosurfer: drag target bbox: %w", err)
	}
	tgtX := tgtBox.X + tgtBox.Width/2
	tgtY := tgtBox.Y + tgtBox.Height/2

	return dragCoordinates(e.page, srcX, srcY, tgtX, tgtY)
}

// DragToCoordinates drags this element to specific viewport coordinates.
func (e *Element) DragToCoordinates(x, y float64) error {
	box, err := e.BBox()
	if err != nil {
		return fmt.Errorf("gosurfer: drag source bbox: %w", err)
	}
	srcX := box.X + box.Width/2
	srcY := box.Y + box.Height/2

	return dragCoordinates(e.page, srcX, srcY, x, y)
}

// DragDrop performs a drag from one coordinate to another on the page.
func (p *Page) DragDrop(fromX, fromY, toX, toY float64) error {
	return dragCoordinates(p, fromX, fromY, toX, toY)
}

// dragCoordinates performs a realistic drag operation using CDP mouse events.
// Uses move-down-move-up sequence with intermediate steps for realistic motion.
func dragCoordinates(p *Page, fromX, fromY, toX, toY float64) error {
	mouse := p.rod.Mouse

	// Move to start position
	if err := mouse.MoveTo(proto.Point{X: fromX, Y: fromY}); err != nil {
		return fmt.Errorf("gosurfer: drag move to start: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Press mouse button
	if err := mouse.Down(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("gosurfer: drag mouse down: %w", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Move linearly to target with intermediate steps (makes drag realistic)
	if err := mouse.MoveLinear(proto.Point{X: toX, Y: toY}, 10); err != nil {
		return fmt.Errorf("gosurfer: drag move to target: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Release
	if err := mouse.Up(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("gosurfer: drag mouse up: %w", err)
	}

	return nil
}

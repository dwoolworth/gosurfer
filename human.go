package gosurfer

import (
	"math"
	"math/rand"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// HumanDelay pauses for a random duration that mimics human reaction time.
// Base is the minimum delay; jitter adds randomness up to that additional amount.
//
//	gosurfer.HumanDelay(200*time.Millisecond, 300*time.Millisecond) // 200-500ms
func HumanDelay(base, jitter time.Duration) {
	delay := base + time.Duration(rand.Int63n(int64(jitter)))
	time.Sleep(delay)
}

// HumanClick clicks an element with a random offset from center and a small delay,
// mimicking natural mouse behavior instead of clicking dead center instantly.
func (p *Page) HumanClick(selector string) error {
	el, err := p.Element(selector)
	if err != nil {
		return err
	}
	return el.HumanClick()
}

// HumanClick clicks the element with random offset from center.
func (e *Element) HumanClick() error {
	box, err := e.BBox()
	if err != nil {
		return err
	}

	// Random offset within the element (not dead center)
	offsetX := box.Width*0.2 + rand.Float64()*box.Width*0.6
	offsetY := box.Height*0.2 + rand.Float64()*box.Height*0.6
	x := box.X + offsetX
	y := box.Y + offsetY

	HumanDelay(50*time.Millisecond, 150*time.Millisecond)

	return proto.InputDispatchMouseEvent{
		Type:       proto.InputDispatchMouseEventTypeMousePressed,
		X:          x,
		Y:          y,
		Button:     proto.InputMouseButtonLeft,
		ClickCount: 1,
	}.Call(e.rod.Page())
}

// HumanType types text character by character with random delays between keystrokes,
// mimicking natural typing speed (~50-150ms per character).
func (p *Page) HumanType(selector string, text string) error {
	el, err := p.Element(selector)
	if err != nil {
		return err
	}
	return el.HumanType(text)
}

// HumanType types text with random inter-keystroke delays.
func (e *Element) HumanType(text string) error {
	if err := e.Focus(); err != nil {
		return err
	}

	HumanDelay(100*time.Millisecond, 200*time.Millisecond)

	for _, ch := range text {
		if err := e.rod.Page().InsertText(string(ch)); err != nil {
			return err
		}
		// Variable typing speed: 40-180ms per character
		// Occasional longer pauses (simulates thinking)
		delay := 40 + rand.Intn(140)
		if rand.Float64() < 0.05 { // 5% chance of a "thinking" pause
			delay += 200 + rand.Intn(400)
		}
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}
	return nil
}

// HumanScroll scrolls the page with natural-feeling incremental movements.
func (p *Page) HumanScroll(deltaY float64) error {
	steps := 3 + rand.Intn(5) // 3-7 scroll steps
	perStep := deltaY / float64(steps)

	for i := 0; i < steps; i++ {
		// Add some jitter to each step
		jitter := perStep * (0.7 + rand.Float64()*0.6)
		if err := p.Scroll(0, jitter); err != nil {
			return err
		}
		time.Sleep(time.Duration(30+rand.Intn(70)) * time.Millisecond)
	}
	return nil
}

// HumanMoveMouse moves the mouse along a curved path to the target coordinates,
// simulating natural hand movement using a Bezier-like curve.
func (p *Page) HumanMoveMouse(toX, toY float64) error {
	// Get current position (default to random starting point if unknown)
	fromX := rand.Float64() * 500
	fromY := rand.Float64() * 500

	steps := 15 + rand.Intn(20) // 15-34 steps for the curve

	// Random control point for a Bezier-like curve
	ctrlX := (fromX+toX)/2 + (rand.Float64()-0.5)*200
	ctrlY := (fromY+toY)/2 + (rand.Float64()-0.5)*200

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		// Quadratic Bezier curve
		x := (1-t)*(1-t)*fromX + 2*(1-t)*t*ctrlX + t*t*toX
		y := (1-t)*(1-t)*fromY + 2*(1-t)*t*ctrlY + t*t*toY

		// Add micro-jitter (simulates hand tremor)
		x += (rand.Float64() - 0.5) * 2
		y += (rand.Float64() - 0.5) * 2

		if err := (proto.InputDispatchMouseEvent{
			Type: proto.InputDispatchMouseEventTypeMouseMoved,
			X:    math.Round(x),
			Y:    math.Round(y),
		}.Call(p.rod)); err != nil {
			return err
		}

		// Non-uniform timing (slower at start and end, faster in middle)
		speed := 5 + int(15*math.Sin(t*math.Pi))
		time.Sleep(time.Duration(speed+rand.Intn(5)) * time.Millisecond)
	}

	return nil
}

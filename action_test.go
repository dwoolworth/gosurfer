package gosurfer

import (
	"encoding/json"
	"testing"
)

func TestToInt(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected int
		wantErr  bool
	}{
		{42, 42, false},
		{float64(7), 7, false},
		{"123", 123, false},
		{json.Number("99"), 99, false},
		{"not_a_number", 0, true},
		{nil, 0, true},
		{true, 0, true},
	}

	for _, tt := range tests {
		got, err := toInt(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("toInt(%v): expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("toInt(%v): unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.expected {
			t.Errorf("toInt(%v) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestToFloat(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected float64
		wantErr  bool
	}{
		{float64(3.14), 3.14, false},
		{42, 42.0, false},
		{"2.5", 2.5, false},
		{json.Number("1.5"), 1.5, false},
		{"bad", 0, true},
		{nil, 0, true},
	}

	for _, tt := range tests {
		got, err := toFloat(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("toFloat(%v): expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("toFloat(%v): unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.expected {
			t.Errorf("toFloat(%v) = %f, want %f", tt.input, got, tt.expected)
		}
	}
}

func TestActionRegistry(t *testing.T) {
	r := NewActionRegistry()
	if len(r.Actions()) != 0 {
		t.Error("new registry should be empty")
	}

	r.Register(&ActionDef{
		Name:        "test_action",
		Description: "A test action",
		Params:      []ParamDef{{Name: "x", Type: "string", Required: true}},
	})

	a, ok := r.Get("test_action")
	if !ok {
		t.Fatal("should find registered action")
	}
	if a.Name != "test_action" {
		t.Errorf("expected test_action, got %s", a.Name)
	}
	if a.Description != "A test action" {
		t.Error("description mismatch")
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("should not find unregistered action")
	}

	if len(r.Actions()) != 1 {
		t.Errorf("expected 1 action, got %d", len(r.Actions()))
	}
}

func TestDefaultActions_Count(t *testing.T) {
	r := DefaultActions()
	actions := r.Actions()
	// We have 16 built-in actions
	if len(actions) < 15 {
		t.Errorf("expected at least 15 default actions, got %d", len(actions))
	}

	// Verify key actions exist
	required := []string{"navigate", "click", "type", "scroll", "done", "switch_tab", "upload_file"}
	for _, name := range required {
		if _, ok := r.Get(name); !ok {
			t.Errorf("missing required action: %s", name)
		}
	}
}

func TestDefaultActions_DoneAction(t *testing.T) {
	r := DefaultActions()
	done, ok := r.Get("done")
	if !ok {
		t.Fatal("done action not found")
	}
	if done.Run == nil {
		t.Fatal("done action has no Run function")
	}
}

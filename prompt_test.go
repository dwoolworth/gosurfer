package gosurfer

import (
	"strings"
	"testing"
)

func TestBuildSystemPrompt_ContainsTask(t *testing.T) {
	actions := DefaultActions()
	prompt := buildSystemPrompt("Find the weather in Tokyo", actions)

	if !strings.Contains(prompt, "Find the weather in Tokyo") {
		t.Error("prompt should contain the task description")
	}
}

func TestBuildSystemPrompt_ContainsAllActions(t *testing.T) {
	actions := DefaultActions()
	prompt := buildSystemPrompt("test task", actions)

	expectedActions := []string{
		"navigate", "click", "type", "scroll", "search",
		"go_back", "wait", "screenshot", "extract", "send_keys",
		"select_option", "switch_tab", "close_tab", "new_tab",
		"upload_file", "done",
	}

	for _, action := range expectedActions {
		if !strings.Contains(prompt, "### "+action) {
			t.Errorf("prompt missing action: %s", action)
		}
	}
}

func TestBuildSystemPrompt_ContainsJSONFormat(t *testing.T) {
	prompt := buildSystemPrompt("task", DefaultActions())
	if !strings.Contains(prompt, `"thought"`) {
		t.Error("prompt should describe JSON response format with thought field")
	}
	if !strings.Contains(prompt, `"action"`) {
		t.Error("prompt should describe JSON response format with action field")
	}
}

func TestBuildSystemPrompt_ContainsCoordinateClickInfo(t *testing.T) {
	prompt := buildSystemPrompt("task", DefaultActions())
	if !strings.Contains(prompt, "coordinate") || !strings.Contains(prompt, "viewport") {
		t.Error("prompt should mention coordinate-based clicking")
	}
}

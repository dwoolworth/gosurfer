package gosurfer

import (
	"fmt"
	"strings"
)

// buildSystemPrompt generates the system prompt for the agent's LLM.
func buildSystemPrompt(task string, actions *ActionRegistry) string {
	var b strings.Builder

	b.WriteString(`You are an AI browser automation agent. Your goal is to complete tasks by interacting with web pages.

## Your Task
`)
	b.WriteString(task)
	b.WriteString(`

## How It Works
1. You receive the current page state as a DOM tree with indexed interactive elements.
2. You choose an action to take, specifying the action name and parameters.
3. Interactive elements are shown as [index]<tag attributes>text</tag>.
4. Use the element index number to interact with specific elements.

## Rules
- Always respond with valid JSON (no markdown, no extra text).
- Use exactly one action per response.
- Think step-by-step about how to achieve the task.
- If an element is not visible, try scrolling to find it.
- If a page doesn't load correctly, try waiting or refreshing.
- If you're stuck in a loop, try a different approach.
- When the task is complete, use the "done" action with your answer.
- If the task is impossible, use "done" with success=false and explain why.

## Response Format
{
  "thought": "Brief reasoning about what to do next",
  "action": "action_name",
  "params": { "param1": "value1" }
}

## Available Actions
`)

	for _, action := range actions.Actions() {
		b.WriteString(fmt.Sprintf("\n### %s\n%s\n", action.Name, action.Description))
		if len(action.Params) > 0 {
			b.WriteString("Parameters:\n")
			for _, p := range action.Params {
				req := ""
				if p.Required {
					req = " (required)"
				}
				b.WriteString(fmt.Sprintf("- %s (%s): %s%s\n", p.Name, p.Type, p.Description, req))
			}
		}
	}

	b.WriteString(`
## Tips
- For search: Use the "search" action rather than navigating to a search engine and typing.
- For forms: Click the input field first (or use "type" with its index), then fill it in.
- For navigation: Use "navigate" with the full URL or "click" on links.
- Read the DOM tree carefully to identify the right elements.
- Element indices change after each page update, so always use the latest DOM state.
`)

	return b.String()
}

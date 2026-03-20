// Example: AI-powered web search using gosurfer.
//
// Usage:
//
//	export OPENAI_API_KEY=sk-...
//	go run ./examples/search/ "What is the current population of Tokyo?"
//
// Or with Anthropic:
//
//	export ANTHROPIC_API_KEY=sk-ant-...
//	go run ./examples/search/ --provider anthropic "What is the current population of Tokyo?"
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/dwoolworth/gosurfer"
)

func main() {
	// Parse args
	provider := "openai"
	var task string
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		if args[i] == "--provider" && i+1 < len(args) {
			provider = args[i+1]
			i++
		} else {
			task = strings.Join(args[i:], " ")
			break
		}
	}
	if task == "" {
		task = "Search Google for 'Go programming language' and tell me the first 3 results."
	}

	// Create LLM provider
	var llm gosurfer.LLMProvider
	switch provider {
	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			log.Fatal("OPENAI_API_KEY not set")
		}
		llm = gosurfer.NewOpenAI(apiKey, "gpt-4o")
	case "anthropic":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			log.Fatal("ANTHROPIC_API_KEY not set")
		}
		llm = gosurfer.NewAnthropic(apiKey, "claude-sonnet-4-20250514")
	case "ollama":
		llm = gosurfer.NewOllama("llama3.1")
	default:
		log.Fatalf("Unknown provider: %s", provider)
	}

	fmt.Printf("Task: %s\n", task)
	fmt.Printf("LLM:  %s\n\n", llm.Name())

	// Create and run agent
	agent, err := gosurfer.NewAgent(gosurfer.AgentConfig{
		Task:      task,
		LLM:       llm,
		Headless:  true,
		UseVision: false,
		MaxSteps:  20,
		Verbose:   true,
		OnStep: func(info gosurfer.StepInfo) {
			fmt.Printf("  [Step %d] %s -> %s (%s)\n",
				info.Step, info.Action, truncate(info.Result, 80), info.Duration.Round(time.Millisecond))
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := agent.Run(ctx)
	if err != nil {
		log.Fatalf("Agent error: %v", err)
	}

	fmt.Println("\n--- Result ---")
	fmt.Printf("Success: %v\n", result.Success)
	fmt.Printf("Steps:   %d\n", result.Steps)
	fmt.Printf("Tokens:  %d prompt + %d completion = %d total\n",
		result.TotalTokens.PromptTokens, result.TotalTokens.CompletionTokens, result.TotalTokens.TotalTokens)
	fmt.Printf("Output:\n%s\n", result.Output)
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

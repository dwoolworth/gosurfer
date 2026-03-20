// Example: Direct browser automation (no AI) using gosurfer.
//
// Usage:
//
//	go run ./examples/scrape/
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/dwoolworth/gosurfer"
)

func main() {
	// Launch headless browser
	browser, err := gosurfer.NewBrowser(gosurfer.BrowserConfig{
		Headless: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = browser.Close() }()

	page, err := browser.NewPage()
	if err != nil {
		log.Fatal(err)
	}

	// Navigate and interact
	if err := page.Navigate("https://news.ycombinator.com"); err != nil {
		log.Fatal(err)
	}

	title, err := page.Title()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Page: %s\n", title)
	fmt.Printf("URL:  %s\n\n", page.URL())

	// Get DOM state (useful for seeing what the LLM would see)
	state, err := page.DOMState()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Interactive elements: %d\n", len(state.Elements))
	fmt.Printf("Scroll position: %.0f%%\n\n", state.ScrollPosition)

	// Print first 20 lines of DOM tree
	lines := splitLines(state.Tree)
	limit := 20
	if len(lines) < limit {
		limit = len(lines)
	}
	fmt.Println("DOM tree (first 20 lines):")
	for _, line := range lines[:limit] {
		fmt.Println(line)
	}

	// Take a screenshot
	png, err := page.Screenshot()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("screenshot.png", png, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\nScreenshot saved to screenshot.png (%d bytes)\n", len(png))

	// Extract text from specific elements
	links, err := page.Elements(".titleline a")
	if err == nil {
		fmt.Printf("\nTop stories:\n")
		for i, link := range links {
			if i >= 10 {
				break
			}
			text, _ := link.Text()
			href, _ := link.Attribute("href")
			fmt.Printf("  %d. %s\n     %s\n", i+1, text, href)
		}
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

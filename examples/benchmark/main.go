// Benchmark: measures memory utilization of gosurfer during browser automation.
//
// Usage:
//
//	go run ./examples/benchmark/
//
// Reports Go heap, Chrome RSS, and total memory at each stage of operation.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/dwoolworth/gosurfer"
)

type memSnapshot struct {
	Label    string
	GoHeapMB float64
	GoSysMB  float64
	ChromeMB float64
}

// chromePIDs tracks which PIDs belong to our browser (not the user's Chrome).
var baselinePIDs map[int]bool

func main() {
	fmt.Println("=== gosurfer Memory Benchmark ===")
	fmt.Println()

	var snapshots []memSnapshot

	// Capture baseline Chrome PIDs (user's own browser, if any)
	baselinePIDs = getChromePIDs()
	fmt.Printf("Pre-existing Chrome processes: %d (excluded from measurement)\n", len(baselinePIDs))

	runtime.GC()
	snap(&snapshots, "Baseline (before browser)")

	// Stage 1: Launch browser
	t0 := time.Now()
	// Detect container environment
	execPath := os.Getenv("CHROME_BIN")
	noSandbox := execPath != "" // in Docker, need --no-sandbox

	browser, err := gosurfer.NewBrowser(gosurfer.BrowserConfig{
		Headless:  true,
		ExecPath:  execPath,
		NoSandbox: noSandbox,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to launch browser: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = browser.Close() }()
	time.Sleep(500 * time.Millisecond) // let processes stabilize
	fmt.Printf("Browser launched in %s\n", time.Since(t0).Round(time.Millisecond))
	snap(&snapshots, "After browser launch")

	// Stage 2: Create page
	page, err := browser.NewPage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create page: %v\n", err)
		os.Exit(1)
	}
	snap(&snapshots, "After new page")

	// Stage 3: Navigate to a real page
	t1 := time.Now()
	if err := page.Navigate("https://news.ycombinator.com"); err != nil {
		fmt.Fprintf(os.Stderr, "Navigate failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Navigation completed in %s\n", time.Since(t1).Round(time.Millisecond))
	snap(&snapshots, "After navigation (HN)")

	// Stage 4: DOM extraction
	t2 := time.Now()
	state, err := page.DOMState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "DOMState failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("DOM extraction: %d elements, %d bytes serialized in %s\n",
		len(state.Elements), len(state.Tree), time.Since(t2).Round(time.Millisecond))
	snap(&snapshots, "After DOM extraction")

	// Stage 5: Screenshot
	t3 := time.Now()
	png, err := page.Screenshot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Screenshot failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Screenshot: %d bytes in %s\n", len(png), time.Since(t3).Round(time.Millisecond))
	snap(&snapshots, "After screenshot")

	// Stage 6: Navigate to a heavier page
	t4 := time.Now()
	if err := page.Navigate("https://en.wikipedia.org/wiki/Go_(programming_language)"); err != nil {
		fmt.Fprintf(os.Stderr, "Navigate to Wikipedia failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Heavy page navigation in %s\n", time.Since(t4).Round(time.Millisecond))
	snap(&snapshots, "After heavy page (Wikipedia)")

	// Stage 7: DOM extraction on heavy page
	t5 := time.Now()
	state2, err := page.DOMState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "DOMState failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Heavy DOM extraction: %d elements, %d bytes in %s\n",
		len(state2.Elements), len(state2.Tree), time.Since(t5).Round(time.Millisecond))
	snap(&snapshots, "After heavy DOM extraction")

	// Stage 8: Full page screenshot
	t6 := time.Now()
	fullPng, err := page.FullScreenshot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Full screenshot failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Full screenshot: %d bytes in %s\n", len(fullPng), time.Since(t6).Round(time.Millisecond))
	snap(&snapshots, "After full screenshot")

	// Stage 9: Multiple tabs
	page2, err := browser.NewPage()
	if err == nil {
		_ = page2.Navigate("https://example.com")
		snap(&snapshots, "After 2nd tab open")
		_ = page2.Close()
	}

	// Stage 10: After GC
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	snap(&snapshots, "After GC")

	// Print summary table
	fmt.Println()
	fmt.Println("=== Memory Profile ===")
	fmt.Println()
	fmt.Printf("%-35s %10s %10s %10s %10s\n", "Stage", "Go Heap", "Go Sys", "Chrome", "Total")
	fmt.Printf("%-35s %10s %10s %10s %10s\n",
		strings.Repeat("-", 35), "--------", "--------", "--------", "--------")

	for _, s := range snapshots {
		total := s.GoSysMB + s.ChromeMB
		fmt.Printf("%-35s %8.1f MB %8.1f MB %8.1f MB %8.1f MB\n",
			s.Label, s.GoHeapMB, s.GoSysMB, s.ChromeMB, total)
	}

	// Peak analysis
	var peakGoHeap, peakGoSys, peakChrome, peakTotal float64
	for _, s := range snapshots {
		if s.GoHeapMB > peakGoHeap {
			peakGoHeap = s.GoHeapMB
		}
		if s.GoSysMB > peakGoSys {
			peakGoSys = s.GoSysMB
		}
		if s.ChromeMB > peakChrome {
			peakChrome = s.ChromeMB
		}
		total := s.GoSysMB + s.ChromeMB
		if total > peakTotal {
			peakTotal = total
		}
	}

	fmt.Println()
	fmt.Println("=== Peak Memory ===")
	fmt.Printf("  Go heap (alloc):   %.1f MB\n", peakGoHeap)
	fmt.Printf("  Go process (sys):  %.1f MB\n", peakGoSys)
	fmt.Printf("  Chrome (RSS):      %.1f MB\n", peakChrome)
	fmt.Printf("  Combined peak:     %.1f MB\n", peakTotal)
	fmt.Println()

	fmt.Println("=== Docker Image ===")
	fmt.Printf("  Go binary (UPX):   ~4 MB\n")
	fmt.Printf("  Chromium + deps:   ~940 MB\n")
	fmt.Printf("  Total image:       ~945 MB\n")
	fmt.Printf("  (vs Browser Use Python: ~2-3 GB, Playwright Node: ~1.5-2 GB)\n")
}

func snap(snapshots *[]memSnapshot, label string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	*snapshots = append(*snapshots, memSnapshot{
		Label:    label,
		GoHeapMB: float64(m.HeapAlloc) / 1024 / 1024,
		GoSysMB:  float64(m.Sys) / 1024 / 1024,
		ChromeMB: getOurChromeMem(),
	})
}

// getChromePIDs returns the PIDs of all current chrome/chromium processes.
func getChromePIDs() map[int]bool {
	pids := make(map[int]bool)
	out, err := exec.Command("ps", "ax", "-o", "pid,comm").Output()
	if err != nil {
		return pids
	}
	for _, line := range strings.Split(string(out), "\n") {
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "chromium") && !strings.Contains(lower, "chrome") {
			continue
		}
		if strings.Contains(lower, "chromedriver") || strings.Contains(lower, "grep") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err == nil {
			pids[pid] = true
		}
	}
	return pids
}

// getOurChromeMem returns RSS of only the Chrome processes launched by gosurfer,
// excluding the user's pre-existing browser.
func getOurChromeMem() float64 {
	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		return 0
	}

	var totalKB int64
	for _, line := range strings.Split(string(out), "\n") {
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "chromium") && !strings.Contains(lower, "chrome") {
			continue
		}
		if strings.Contains(lower, "chromedriver") || strings.Contains(lower, "grep") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		pid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		// Skip pre-existing Chrome processes
		if baselinePIDs[pid] {
			continue
		}

		rss, err := strconv.ParseInt(fields[5], 10, 64)
		if err != nil {
			continue
		}
		totalKB += rss
	}

	return float64(totalKB) / 1024
}

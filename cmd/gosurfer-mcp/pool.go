// Page pool with busy tracking, concurrency limits, and metrics.
//
// Design principles:
//  1. Pages are created on-demand, not pre-allocated. A fresh page is a clean
//     page (no leftover cookies from a previous request's cross-origin leak).
//  2. A semaphore enforces a maximum number of concurrently-open pages across
//     all tools, preventing runaway resource usage.
//  3. Acquisition is context-aware: if the caller's context is cancelled, the
//     acquisition fails fast instead of hanging.
//  4. If the pool is exhausted and the caller does not want to wait, they get
//     an immediate "pool exhausted" error rather than a hung goroutine.
//  5. Pages are always closed after use, even on error paths.
//  6. Counters track busy, total created, total closed, exhaustion events,
//     and acquisition wait time for observability.
package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dwoolworth/gosurfer"
)

// PagePool manages concurrent access to pages backed by a single shared Browser.
// It enforces a maximum concurrent page count via a buffered channel semaphore.
type PagePool struct {
	browser    *gosurfer.Browser
	sem        chan struct{} // buffered channel acts as a semaphore
	maxPages   int

	// Counters (use atomic for lock-free reads).
	busy          atomic.Int64
	totalCreated  atomic.Int64
	totalClosed   atomic.Int64
	totalExhaust  atomic.Int64
	totalTimeouts atomic.Int64

	// waitHist tracks total and count of acquisition wait times.
	waitMu          sync.Mutex
	totalWaitNanos  int64
	totalWaitCount  int64
	longestWaitNano int64
}

// ErrPoolExhausted is returned when the pool is full and the caller's context
// expires before a page slot becomes available.
var ErrPoolExhausted = errors.New("page pool exhausted: all browser slots busy")

// NewPagePool creates a pool backed by the given browser with a maximum
// concurrent page count. maxPages <= 0 defaults to 10.
func NewPagePool(browser *gosurfer.Browser, maxPages int) *PagePool {
	if maxPages <= 0 {
		maxPages = 10
	}
	return &PagePool{
		browser:  browser,
		sem:      make(chan struct{}, maxPages),
		maxPages: maxPages,
	}
}

// Acquire blocks until a page slot is available or the context expires.
// On success, returns a new page and a release function that MUST be called
// when the caller is done (defer it). The release function closes the page
// and returns the slot to the pool.
func (p *PagePool) Acquire(ctx context.Context) (*gosurfer.Page, func(), error) {
	// Fast-fail if the caller's context is already cancelled. Without this
	// check, select would non-deterministically pick between the sem and ctx
	// cases when both are ready.
	if err := ctx.Err(); err != nil {
		p.totalExhaust.Add(1)
		return nil, nil, fmt.Errorf("%w: %v", ErrPoolExhausted, err)
	}

	acquireStart := time.Now()

	// Try to grab a slot. Block until one is available or the context expires.
	select {
	case p.sem <- struct{}{}:
		// Got a slot.
	case <-ctx.Done():
		p.totalExhaust.Add(1)
		return nil, nil, fmt.Errorf("%w: %v", ErrPoolExhausted, ctx.Err())
	}

	waitElapsed := time.Since(acquireStart)
	p.recordWait(waitElapsed)

	// Create a fresh page. If creation fails, release the slot immediately.
	page, err := p.browser.NewPage()
	if err != nil {
		<-p.sem
		return nil, nil, fmt.Errorf("create page: %w", err)
	}

	p.busy.Add(1)
	p.totalCreated.Add(1)

	release := func() {
		if closeErr := page.Close(); closeErr != nil {
			// Log but don't block release — we still need to return the slot.
			fmt.Printf("pool: page close error: %v\n", closeErr)
		}
		p.busy.Add(-1)
		p.totalClosed.Add(1)
		<-p.sem
	}

	return page, release, nil
}

// recordWait updates the wait-time histogram. Safe for concurrent use.
func (p *PagePool) recordWait(d time.Duration) {
	p.waitMu.Lock()
	defer p.waitMu.Unlock()
	p.totalWaitNanos += d.Nanoseconds()
	p.totalWaitCount++
	if d.Nanoseconds() > p.longestWaitNano {
		p.longestWaitNano = d.Nanoseconds()
	}
}

// RecordTimeout increments the timeout counter. Called by callers that
// cancel a page operation mid-flight due to their own timeout.
func (p *PagePool) RecordTimeout() {
	p.totalTimeouts.Add(1)
}

// Stats is a snapshot of pool health for observability.
type Stats struct {
	MaxPages        int    `json:"max_pages"`
	Busy            int64  `json:"busy"`
	Available       int    `json:"available"`
	TotalCreated    int64  `json:"total_created"`
	TotalClosed     int64  `json:"total_closed"`
	TotalExhausted  int64  `json:"total_exhausted"`
	TotalTimeouts   int64  `json:"total_timeouts"`
	AvgWaitMs       int64  `json:"avg_wait_ms"`
	LongestWaitMs   int64  `json:"longest_wait_ms"`
}

// Stats returns a snapshot of current pool state.
func (p *PagePool) Stats() Stats {
	p.waitMu.Lock()
	avg := int64(0)
	if p.totalWaitCount > 0 {
		avg = (p.totalWaitNanos / p.totalWaitCount) / int64(time.Millisecond)
	}
	longest := p.longestWaitNano / int64(time.Millisecond)
	p.waitMu.Unlock()

	busy := p.busy.Load()
	return Stats{
		MaxPages:       p.maxPages,
		Busy:           busy,
		Available:      p.maxPages - int(busy),
		TotalCreated:   p.totalCreated.Load(),
		TotalClosed:    p.totalClosed.Load(),
		TotalExhausted: p.totalExhaust.Load(),
		TotalTimeouts:  p.totalTimeouts.Load(),
		AvgWaitMs:      avg,
		LongestWaitMs:  longest,
	}
}

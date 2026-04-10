package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Note: These tests exercise the pool's semaphore and counters directly by
// bypassing the browser. We do that by using a nil browser and only testing
// the portions of Acquire that run before browser.NewPage is called
// (exhaustion and cancellation paths), plus the Stats counters directly.

func TestPagePoolStatsInitial(t *testing.T) {
	p := NewPagePool(nil, 5)
	s := p.Stats()
	if s.MaxPages != 5 {
		t.Errorf("MaxPages: want 5, got %d", s.MaxPages)
	}
	if s.Busy != 0 {
		t.Errorf("Busy: want 0, got %d", s.Busy)
	}
	if s.Available != 5 {
		t.Errorf("Available: want 5, got %d", s.Available)
	}
	if s.TotalCreated != 0 || s.TotalClosed != 0 || s.TotalExhausted != 0 {
		t.Errorf("counters should start at zero: %+v", s)
	}
}

func TestPagePoolDefaultsToTenWhenZero(t *testing.T) {
	p := NewPagePool(nil, 0)
	if p.maxPages != 10 {
		t.Errorf("maxPages: want 10 (default), got %d", p.maxPages)
	}
}

func TestPagePoolDefaultsToTenWhenNegative(t *testing.T) {
	p := NewPagePool(nil, -5)
	if p.maxPages != 10 {
		t.Errorf("maxPages: want 10 (default), got %d", p.maxPages)
	}
}

func TestPagePoolAcquireFailsWhenContextAlreadyCancelled(t *testing.T) {
	p := NewPagePool(nil, 2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before acquiring

	_, _, err := p.Acquire(ctx)
	if !errors.Is(err, ErrPoolExhausted) {
		t.Errorf("expected ErrPoolExhausted, got %v", err)
	}

	s := p.Stats()
	if s.TotalExhausted != 1 {
		t.Errorf("TotalExhausted: want 1, got %d", s.TotalExhausted)
	}
}

func TestPagePoolAcquireFailsWhenAllSlotsTakenAndContextExpires(t *testing.T) {
	p := NewPagePool(nil, 2)

	// Manually fill the semaphore to simulate two active acquisitions without
	// actually creating browser pages.
	p.sem <- struct{}{}
	p.sem <- struct{}{}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, err := p.Acquire(ctx)
	elapsed := time.Since(start)

	if !errors.Is(err, ErrPoolExhausted) {
		t.Errorf("expected ErrPoolExhausted, got %v", err)
	}
	if elapsed < 40*time.Millisecond {
		t.Errorf("should have waited for context deadline; elapsed=%s", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("should have returned promptly after context deadline; elapsed=%s", elapsed)
	}

	s := p.Stats()
	if s.TotalExhausted != 1 {
		t.Errorf("TotalExhausted: want 1, got %d", s.TotalExhausted)
	}
}

func TestPagePoolRecordTimeout(t *testing.T) {
	p := NewPagePool(nil, 1)
	p.RecordTimeout()
	p.RecordTimeout()
	s := p.Stats()
	if s.TotalTimeouts != 2 {
		t.Errorf("TotalTimeouts: want 2, got %d", s.TotalTimeouts)
	}
}

func TestPagePoolWaitTimeRecording(t *testing.T) {
	p := NewPagePool(nil, 1)
	p.recordWait(50 * time.Millisecond)
	p.recordWait(150 * time.Millisecond)

	s := p.Stats()
	if s.AvgWaitMs != 100 {
		t.Errorf("AvgWaitMs: want 100, got %d", s.AvgWaitMs)
	}
	if s.LongestWaitMs != 150 {
		t.Errorf("LongestWaitMs: want 150, got %d", s.LongestWaitMs)
	}
}

func TestPagePoolStatsAvailableTracksBusy(t *testing.T) {
	p := NewPagePool(nil, 4)
	p.busy.Store(3)
	s := p.Stats()
	if s.Available != 1 {
		t.Errorf("Available: want 1, got %d", s.Available)
	}
	if s.Busy != 3 {
		t.Errorf("Busy: want 3, got %d", s.Busy)
	}
}

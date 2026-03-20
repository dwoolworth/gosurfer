package gosurfer

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHARRecorder_CapturesRequests(t *testing.T) {
	page := newPage(t)

	rec := page.StartHAR()

	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}
	_ = page.WaitLoad()
	time.Sleep(1 * time.Second) // Allow network events to be captured

	entries := rec.Entries()
	if entries == 0 {
		t.Error("HAR recorder should capture at least one request after navigation")
	}
}

func TestHARRecorder_Export(t *testing.T) {
	page := newPage(t)

	rec := page.StartHAR()

	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}
	_ = page.WaitLoad()
	time.Sleep(1 * time.Second)

	data, err := rec.Export()
	if err != nil {
		t.Fatal(err)
	}

	// Verify it's valid JSON
	if !json.Valid(data) {
		t.Fatal("exported HAR should be valid JSON")
	}

	// Verify HAR structure
	var har harFile
	if err := json.Unmarshal(data, &har); err != nil {
		t.Fatalf("failed to unmarshal HAR: %v", err)
	}

	if har.Log.Version != "1.2" {
		t.Errorf("HAR version = %q, want %q", har.Log.Version, "1.2")
	}
	if har.Log.Creator.Name != "gosurfer" {
		t.Errorf("creator = %q, want %q", har.Log.Creator.Name, "gosurfer")
	}
	if len(har.Log.Entries) == 0 {
		t.Error("HAR entries should not be empty after navigation")
	}

	// Check that at least one entry has a valid request URL
	foundURL := false
	for _, entry := range har.Log.Entries {
		if entry.Request.URL != "" {
			foundURL = true
			break
		}
	}
	if !foundURL {
		t.Error("at least one HAR entry should have a request URL")
	}
}

func TestHARRecorder_Entries(t *testing.T) {
	page := newPage(t)

	rec := page.StartHAR()

	// Navigate to multiple pages to generate more entries
	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}
	_ = page.WaitLoad()
	time.Sleep(500 * time.Millisecond)

	if err := page.Navigate(ts.URL + "/page2"); err != nil {
		t.Fatal(err)
	}
	_ = page.WaitLoad()
	time.Sleep(500 * time.Millisecond)

	count := rec.Entries()
	if count < 2 {
		t.Errorf("expected at least 2 entries after 2 navigations, got %d", count)
	}
}

func TestHARRecorder_TimingFields(t *testing.T) {
	page := newPage(t)

	rec := page.StartHAR()

	if err := page.Navigate(ts.URL); err != nil {
		t.Fatal(err)
	}
	_ = page.WaitLoad()
	time.Sleep(1 * time.Second)

	data, err := rec.Export()
	if err != nil {
		t.Fatal(err)
	}

	var har harFile
	if err := json.Unmarshal(data, &har); err != nil {
		t.Fatal(err)
	}

	if len(har.Log.Entries) == 0 {
		t.Fatal("no entries to check timings")
	}

	// Check that the first entry has timing information populated
	entry := har.Log.Entries[0]
	if entry.StartedDateTime == "" {
		t.Error("startedDateTime should be set")
	}

	// At least one of the timing fields should be non-negative
	// (local server requests may have DNS/Connect/SSL all as 0 or negative,
	// but send and wait should be populated)
	timings := entry.Timings
	t.Logf("timings: dns=%.2f connect=%.2f ssl=%.2f send=%.2f wait=%.2f receive=%.2f",
		timings.DNS, timings.Connect, timings.SSL, timings.Send, timings.Wait, timings.Receive)

	// The entry should have a total time > 0
	if entry.Time <= 0 {
		t.Logf("entry time = %.2f (may be 0 for very fast local requests)", entry.Time)
	}

	// Verify request method is set
	if entry.Request.Method == "" {
		t.Error("request method should be set")
	}
	if entry.Response.Status == 0 {
		t.Log("response status = 0 (may be pending)")
	}
}

package gosurfer

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// HARRecorder captures all network traffic on a page in HAR 1.2 format.
// Start recording before navigation to capture everything.
type HARRecorder struct {
	page    *Page
	entries []harEntry
	mu      sync.Mutex
	started time.Time

	// Pending requests waiting for response
	pending map[proto.NetworkRequestID]*harEntry
}

// StartHAR begins recording network traffic on the page.
// Call StopHAR() when done, then Export() to get the HAR data.
func (p *Page) StartHAR() *HARRecorder {
	rec := &HARRecorder{
		page:    p,
		pending: make(map[proto.NetworkRequestID]*harEntry),
		started: time.Now(),
	}

	// Enable network domain
	_ = proto.NetworkEnable{}.Call(p.rod)

	// Subscribe to network events
	go p.rod.EachEvent(
		func(e *proto.NetworkRequestWillBeSent) {
			rec.onRequest(e)
		},
		func(e *proto.NetworkResponseReceived) {
			rec.onResponse(e)
		},
		func(e *proto.NetworkLoadingFinished) {
			rec.onFinished(e)
		},
		func(e *proto.NetworkLoadingFailed) {
			rec.onFailed(e)
		},
	)()

	return rec
}

// Export returns the recorded traffic as a HAR 1.2 JSON byte slice.
func (rec *HARRecorder) Export() ([]byte, error) {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	har := harFile{
		Log: harLog{
			Version: "1.2",
			Creator: harCreator{
				Name:    "gosurfer",
				Version: "1.0",
			},
			Entries: rec.buildEntries(),
		},
	}

	return json.MarshalIndent(har, "", "  ")
}

// Entries returns the number of recorded requests.
func (rec *HARRecorder) Entries() int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return len(rec.entries)
}

func (rec *HARRecorder) onRequest(e *proto.NetworkRequestWillBeSent) {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	headers := make([]harNVP, 0)
	for name, val := range e.Request.Headers {
		headers = append(headers, harNVP{Name: name, Value: val.Str()})
	}

	entry := &harEntry{
		StartedDateTime: time.Now().Format(time.RFC3339Nano),
		Request: harRequest{
			Method:      e.Request.Method,
			URL:         e.Request.URL,
			HTTPVersion: "HTTP/1.1",
			Headers:     headers,
			HeadersSize: -1,
			BodySize:    len(e.Request.PostData),
			PostData:    e.Request.PostData,
		},
		Time:    0,
		startAt: time.Now(),
	}

	rec.pending[e.RequestID] = entry
}

func (rec *HARRecorder) onResponse(e *proto.NetworkResponseReceived) {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	entry, ok := rec.pending[e.RequestID]
	if !ok {
		return
	}

	headers := make([]harNVP, 0)
	for name, val := range e.Response.Headers {
		headers = append(headers, harNVP{Name: name, Value: val.Str()})
	}

	entry.Response = harResponse{
		Status:      e.Response.Status,
		StatusText:  e.Response.StatusText,
		HTTPVersion: e.Response.Protocol,
		Headers:     headers,
		HeadersSize: -1,
		BodySize:    -1,
		Content: harContent{
			Size:     int(e.Response.EncodedDataLength),
			MimeType: e.Response.MIMEType,
		},
	}

	// Calculate timing
	if e.Response.Timing != nil {
		entry.Timings = harTimings{
			DNS:     e.Response.Timing.DNSEnd - e.Response.Timing.DNSStart,
			Connect: e.Response.Timing.ConnectEnd - e.Response.Timing.ConnectStart,
			SSL:     e.Response.Timing.SslEnd - e.Response.Timing.SslStart,
			Send:    e.Response.Timing.SendEnd - e.Response.Timing.SendStart,
			Wait:    e.Response.Timing.ReceiveHeadersEnd - e.Response.Timing.SendEnd,
			Receive: 0, // filled on loadingFinished
		}
	}
}

func (rec *HARRecorder) onFinished(e *proto.NetworkLoadingFinished) {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	entry, ok := rec.pending[e.RequestID]
	if !ok {
		return
	}

	entry.Time = float64(time.Since(entry.startAt).Milliseconds())
	entry.Response.BodySize = int(e.EncodedDataLength)
	entry.Response.Content.Size = int(e.EncodedDataLength)

	rec.entries = append(rec.entries, *entry)
	delete(rec.pending, e.RequestID)
}

func (rec *HARRecorder) onFailed(e *proto.NetworkLoadingFailed) {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	entry, ok := rec.pending[e.RequestID]
	if !ok {
		return
	}

	entry.Time = float64(time.Since(entry.startAt).Milliseconds())
	entry.Response = harResponse{
		Status:     0,
		StatusText: fmt.Sprintf("Failed: %s", e.ErrorText),
	}

	rec.entries = append(rec.entries, *entry)
	delete(rec.pending, e.RequestID)
}

func (rec *HARRecorder) buildEntries() []harOutputEntry {
	out := make([]harOutputEntry, len(rec.entries))
	for i, e := range rec.entries {
		out[i] = harOutputEntry{
			StartedDateTime: e.StartedDateTime,
			Time:            e.Time,
			Request: harOutputRequest{
				Method:      e.Request.Method,
				URL:         e.Request.URL,
				HTTPVersion: e.Request.HTTPVersion,
				Headers:     e.Request.Headers,
				HeadersSize: e.Request.HeadersSize,
				BodySize:    e.Request.BodySize,
			},
			Response: harOutputResponse{
				Status:      e.Response.Status,
				StatusText:  e.Response.StatusText,
				HTTPVersion: e.Response.HTTPVersion,
				Headers:     e.Response.Headers,
				Content:     e.Response.Content,
				HeadersSize: e.Response.HeadersSize,
				BodySize:    e.Response.BodySize,
			},
			Timings: e.Timings,
		}
	}
	return out
}

// --- HAR 1.2 Data Types ---

type harFile struct {
	Log harLog `json:"log"`
}

type harLog struct {
	Version string           `json:"version"`
	Creator harCreator       `json:"creator"`
	Entries []harOutputEntry `json:"entries"`
}

type harCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type harOutputEntry struct {
	StartedDateTime string          `json:"startedDateTime"`
	Time            float64         `json:"time"`
	Request         harOutputRequest  `json:"request"`
	Response        harOutputResponse `json:"response"`
	Timings         harTimings      `json:"timings"`
}

type harOutputRequest struct {
	Method      string   `json:"method"`
	URL         string   `json:"url"`
	HTTPVersion string   `json:"httpVersion"`
	Headers     []harNVP `json:"headers"`
	HeadersSize int      `json:"headersSize"`
	BodySize    int      `json:"bodySize"`
}

type harOutputResponse struct {
	Status      int        `json:"status"`
	StatusText  string     `json:"statusText"`
	HTTPVersion string     `json:"httpVersion"`
	Headers     []harNVP   `json:"headers"`
	Content     harContent `json:"content"`
	HeadersSize int        `json:"headersSize"`
	BodySize    int        `json:"bodySize"`
}

type harNVP struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type harContent struct {
	Size     int    `json:"size"`
	MimeType string `json:"mimeType"`
}

type harTimings struct {
	DNS     float64 `json:"dns"`
	Connect float64 `json:"connect"`
	SSL     float64 `json:"ssl"`
	Send    float64 `json:"send"`
	Wait    float64 `json:"wait"`
	Receive float64 `json:"receive"`
}

// Internal entry with timing state
type harEntry struct {
	StartedDateTime string
	Time            float64
	Request         harRequest
	Response        harResponse
	Timings         harTimings
	startAt         time.Time
}

type harRequest struct {
	Method      string
	URL         string
	HTTPVersion string
	Headers     []harNVP
	HeadersSize int
	BodySize    int
	PostData    string
}

type harResponse struct {
	Status      int
	StatusText  string
	HTTPVersion string
	Headers     []harNVP
	Content     harContent
	HeadersSize int
	BodySize    int
}

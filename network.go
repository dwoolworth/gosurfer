package gosurfer

import (
	"encoding/json"
	"net/http"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// NetworkRoute defines a rule for intercepting network requests.
type NetworkRoute struct {
	// Pattern matches request URLs (regex).
	Pattern string

	// Handler processes matching requests.
	Handler func(req *InterceptedRequest)
}

// InterceptedRequest wraps a hijacked network request.
type InterceptedRequest struct {
	hijack *rod.Hijack
}

// URL returns the request URL.
func (r *InterceptedRequest) URL() string {
	return r.hijack.Request.URL().String()
}

// Method returns the HTTP method.
func (r *InterceptedRequest) Method() string {
	return r.hijack.Request.Method()
}

// Header returns a single request header value.
func (r *InterceptedRequest) Header(key string) string {
	return r.hijack.Request.Header(key)
}

// Body returns the request body.
func (r *InterceptedRequest) Body() string {
	return r.hijack.Request.Body()
}

// Continue allows the request to proceed normally.
func (r *InterceptedRequest) Continue() {
	r.hijack.ContinueRequest(&proto.FetchContinueRequest{})
}

// Abort blocks the request.
func (r *InterceptedRequest) Abort() {
	r.hijack.Response.Fail(proto.NetworkErrorReasonAborted)
}

// Respond returns a custom response without hitting the real server.
//
//	req.Respond(200, "OK", "Content-Type", "text/plain")
func (r *InterceptedRequest) Respond(status int, body string, headerPairs ...string) {
	r.hijack.Response.Payload().ResponseCode = status
	if len(headerPairs) > 0 {
		r.hijack.Response.SetHeader(headerPairs...)
	}
	r.hijack.Response.SetBody(body)
}

// RespondJSON returns a JSON response without hitting the real server.
// The data parameter is marshaled to JSON automatically.
//
//	req.RespondJSON(200, map[string]any{"status": "ok", "count": 42})
func (r *InterceptedRequest) RespondJSON(status int, data interface{}) {
	r.hijack.Response.Payload().ResponseCode = status
	r.hijack.Response.SetHeader("Content-Type", "application/json")
	body, err := json.Marshal(data)
	if err != nil {
		r.hijack.Response.SetBody(`{"error":"marshal failed"}`)
		return
	}
	r.hijack.Response.SetBody(body)
}

// LoadResponse fetches the real response from the server.
// Call this before reading or modifying the response.
func (r *InterceptedRequest) LoadResponse() error {
	return r.hijack.LoadResponse(&http.Client{}, true)
}

// ResponseStatus returns the response status code (after LoadResponse).
func (r *InterceptedRequest) ResponseStatus() int {
	return r.hijack.Response.Payload().ResponseCode
}

// ResponseBody returns the response body (after LoadResponse).
func (r *InterceptedRequest) ResponseBody() string {
	return r.hijack.Response.Body()
}

// SetResponseBody replaces the response body (after LoadResponse).
func (r *InterceptedRequest) SetResponseBody(body string) {
	r.hijack.Response.SetBody(body)
}

// SetResponseHeader sets a response header (after LoadResponse).
func (r *InterceptedRequest) SetResponseHeader(pairs ...string) {
	r.hijack.Response.SetHeader(pairs...)
}

// NetworkInterceptor manages request interception for a page.
type NetworkInterceptor struct {
	page   *Page
	router *rod.HijackRouter
	routes []NetworkRoute
}

// Intercept sets up network request interception on the page.
func (p *Page) Intercept() *NetworkInterceptor {
	router := p.rod.HijackRequests()
	return &NetworkInterceptor{
		page:   p,
		router: router,
	}
}

// OnRequest adds a route that intercepts matching requests.
// The pattern uses CDP URL glob syntax: * matches any characters.
// Example: "*api/users*" matches any URL containing "api/users".
func (ni *NetworkInterceptor) OnRequest(pattern string, handler func(req *InterceptedRequest)) *NetworkInterceptor {
	ni.routes = append(ni.routes, NetworkRoute{Pattern: pattern, Handler: handler})
	ni.router.MustAdd(pattern, func(ctx *rod.Hijack) {
		handler(&InterceptedRequest{hijack: ctx})
	})
	return ni
}

// MockJSON intercepts requests matching the pattern and returns a JSON response.
// This is the simplest way to mock an API endpoint:
//
//	interceptor.MockJSON(`/api/users`, 200, map[string]any{
//	    "users": []map[string]any{{"id": 1, "name": "Alice"}},
//	})
func (ni *NetworkInterceptor) MockJSON(pattern string, status int, data interface{}) *NetworkInterceptor {
	return ni.OnRequest(pattern, func(req *InterceptedRequest) {
		req.RespondJSON(status, data)
	})
}

// MockText intercepts requests matching the pattern and returns a text response.
func (ni *NetworkInterceptor) MockText(pattern string, status int, body string, headerPairs ...string) *NetworkInterceptor {
	return ni.OnRequest(pattern, func(req *InterceptedRequest) {
		req.Respond(status, body, headerPairs...)
	})
}

// BlockPatterns blocks all requests matching the given URL glob patterns.
// Useful for blocking ads, trackers, and large resources.
// Example: "*analytics*", "*.ads.*", "*tracker*"
func (ni *NetworkInterceptor) BlockPatterns(patterns ...string) *NetworkInterceptor {
	for _, pattern := range patterns {
		ni.router.MustAdd(pattern, func(ctx *rod.Hijack) {
			ctx.Response.Fail(proto.NetworkErrorReasonAborted)
		})
	}
	return ni
}

// Start begins intercepting requests. Call Stop() when done.
func (ni *NetworkInterceptor) Start() {
	go ni.router.Run()
}

// Stop stops intercepting requests.
func (ni *NetworkInterceptor) Stop() error {
	return ni.router.Stop()
}

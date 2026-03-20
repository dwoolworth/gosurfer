package gosurfer

import (
	"regexp"

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

// Respond returns a custom response with the given body.
func (r *InterceptedRequest) Respond(status int, body string, headerPairs ...string) {
	if len(headerPairs) > 0 {
		r.hijack.Response.SetHeader(headerPairs...)
	}
	r.hijack.Response.SetBody(body)
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
func (ni *NetworkInterceptor) OnRequest(pattern string, handler func(req *InterceptedRequest)) *NetworkInterceptor {
	ni.routes = append(ni.routes, NetworkRoute{Pattern: pattern, Handler: handler})
	ni.router.MustAdd(regexp.MustCompile(pattern).String(), func(ctx *rod.Hijack) {
		handler(&InterceptedRequest{hijack: ctx})
	})
	return ni
}

// BlockPatterns blocks all requests matching the given URL patterns.
// Useful for blocking ads, trackers, and large resources.
func (ni *NetworkInterceptor) BlockPatterns(patterns ...string) *NetworkInterceptor {
	for _, pattern := range patterns {
		ni.router.MustAdd(regexp.MustCompile(pattern).String(), func(ctx *rod.Hijack) {
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

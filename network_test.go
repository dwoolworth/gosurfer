package gosurfer

import "testing"

// NetworkInterceptor requires a live browser page, so we test the struct/types only.

func TestNetworkRoute_Fields(t *testing.T) {
	route := NetworkRoute{
		Pattern: `.*\.png$`,
		Handler: func(req *InterceptedRequest) {
			req.Abort()
		},
	}
	if route.Pattern != `.*\.png$` {
		t.Error("pattern mismatch")
	}
	if route.Handler == nil {
		t.Error("handler should not be nil")
	}
}

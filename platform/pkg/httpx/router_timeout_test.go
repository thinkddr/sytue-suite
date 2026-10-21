// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRouteTimeoutKeepsDefaultAndCallerCancellation(t *testing.T) {
	r := NewRouter(RouterOptions{
		RequestTimeout: time.Minute,
		RequestTimeoutFor: func(r *http.Request) time.Duration {
			if r.URL.Path == "/onboarding" {
				return 2 * time.Minute
			}
			return -1 // An accidental negative override cannot remove the default.
		},
	})
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		deadline, ok := req.Context().Deadline()
		if !ok {
			t.Error("request has no deadline")
		}
		want := time.Minute
		if req.URL.Path == "/onboarding" {
			want *= 2
		}
		if remaining := time.Until(deadline); remaining < want-time.Second || remaining > want {
			t.Errorf("%s deadline = %s, want %s", req.URL.Path, remaining, want)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	for _, path := range []string{"/ordinary", "/onboarding"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/onboarding", http.NoBody).WithContext(ctx)
	requestTimeout(time.Minute, func(*http.Request) time.Duration { return 2 * time.Minute })(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		if req.Context().Err() != context.Canceled {
			t.Error("onboarding lost caller cancellation")
		}
	})).ServeHTTP(httptest.NewRecorder(), req)
}

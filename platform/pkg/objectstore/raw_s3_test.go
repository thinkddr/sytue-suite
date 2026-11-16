// SPDX-License-Identifier: Apache-2.0
package objectstore

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// A real net/http endpoint emulates the S3 response boundary and records the
// methods. A separate disposable S3 integration test is still required.
func TestRawS3LazyGETDoesNotTrustHeadForEmptyOrMissing(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", "Mon, 02 Nov 2026 12:00:00 GMT")
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path+" "+r.Header.Get("Accept-Encoding"))
		mu.Unlock()
		if r.Header.Get("Accept-Encoding") != "identity" {
			t.Error("request did not require identity")
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/empty"):
			w.Header().Set("Content-Length", "0")
			w.Header().Set("ETag", `"fixture"`)
		case strings.HasSuffix(r.URL.Path, "/missing"):
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>")
		case strings.HasSuffix(r.URL.Path, "/forbidden"):
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "<Error><Code>AccessDenied</Code><Message>forbidden</Message></Error>")
		case strings.HasSuffix(r.URL.Path, "/encoded"):
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = io.WriteString(w, "opaque")
		case strings.HasSuffix(r.URL.Path, "/chunked"):
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			_, _ = io.WriteString(w, "non-empty")
		case strings.HasSuffix(r.URL.Path, "/partial"):
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Length", "20")
			_, _ = io.WriteString(w, "opaque")
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	client, err := New(Config{Endpoint: strings.TrimPrefix(server.URL, "http://"), Region: "us-east-1", Bucket: "fixture-bucket", AccessKey: "fixture", SecretKey: "fixture", Transport: &http.Transport{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"empty", "missing", "forbidden", "encoded", "chunked", "partial"} {
		t.Run(name, func(t *testing.T) {
			body, err := client.GetRaw(t.Context(), tenancy.Tenant{Slug: "acme"}, name)
			if err != nil {
				t.Fatal(err)
			}
			data, readErr := io.ReadAll(body)
			closeErr := body.Close()
			if closeErr != nil || len(data) != 0 {
				t.Fatal("unexpected raw result", closeErr)
			}
			switch name {
			case "empty":
				if readErr != nil {
					t.Fatal("empty object failed", readErr)
				}
			case "missing":
				if !errors.Is(readErr, ErrNotFound) {
					t.Fatal("missing object not identified", readErr)
				}
			case "forbidden":
				if readErr == nil || errors.Is(readErr, ErrNotFound) {
					t.Fatal("forbidden treated as missing", readErr)
				}
			case "encoded":
				if !errors.Is(readErr, ErrRawUnsupported) {
					t.Fatal("encoded S3 response accepted", readErr)
				}
			case "chunked":
				if !errors.Is(readErr, ErrRawUnsupported) {
					t.Fatal("unknown-length S3 response became clean EOF", readErr)
				}
			case "partial":
				if readErr == nil || errors.Is(readErr, io.EOF) {
					t.Fatal("partial response was accepted", readErr)
				}
			}
		})
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 6 {
		t.Fatal("unexpected number of provider requests", calls)
	}
	for _, call := range calls {
		if !strings.HasPrefix(call, "GET ") {
			t.Fatal("raw read trusted a HEAD instead of performing GET", calls)
		}
	}
}

func TestRawS3CloseCancelsBlockedNetworkRead(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", "Mon, 02 Nov 2026 12:00:00 GMT")
		w.Header().Set("Content-Length", "10")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := New(Config{Endpoint: strings.TrimPrefix(server.URL, "http://"), Region: "us-east-1", Bucket: "fixture-bucket", AccessKey: "fixture", SecretKey: "fixture", Transport: &http.Transport{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	body, err := client.GetRaw(ctx, tenancy.Tenant{Slug: "acme"}, "blocked")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(body); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("GET did not start")
	}
	closed := make(chan error, 1)
	go func() { closed <- body.Close() }()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not interrupt GET")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled read succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Read remained blocked")
	}
}

func TestRawS3RejectsUnknownTransportBeforeProviderIO(t *testing.T) {
	transport := dropboxTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("unsupported transport was used")
		return nil, ErrRawUnsupported
	})
	client, err := New(Config{Endpoint: "fixture.example", Region: "us-east-1", Bucket: "fixture-bucket", AccessKey: "fixture", SecretKey: "fixture", Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	body, err := client.GetRaw(t.Context(), tenancy.Tenant{Slug: "acme"}, "blob")
	if body != nil || !errors.Is(err, ErrRawUnsupported) {
		t.Fatal("unknown S3 transport accepted", err)
	}
}

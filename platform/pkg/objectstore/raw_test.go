// SPDX-License-Identifier: Apache-2.0
package objectstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// Keep net/http's real Transport, while sending the fixture's provider hosts
// to an isolated TLS server; no external credentials or provider are used.
func rawFixtureClient(server *httptest.Server) *http.Client {
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.ServerName = "127.0.0.1"
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	return &http.Client{Transport: transport}
}

func rawAdapter(t *testing.T, name string, client *http.Client) RawBlobReader {
	t.Helper()
	token := func(context.Context) (string, error) { return "fixture-token", nil }
	switch name {
	case "gdrive":
		g := NewGoogleDrive(token, externalIndexFixture{"acme:blob": "fixture-object"})
		g.http = client
		return g
	case "onedrive":
		o := NewOneDrive(token)
		o.http = client
		return o
	case "dropbox":
		d := NewDropbox(token)
		d.http = client
		return d
	case "webdav":
		w, err := NewWebDAV("https://fixture.example/", "Sytue", "fixture", "fixture")
		if err != nil {
			t.Fatal(err)
		}
		w.http = client
		return w
	default:
		t.Fatal("unknown adapter")
		return nil
	}
}

func TestRawHTTPAdaptersRequestIdentityAndRejectEncodedResponses(t *testing.T) {
	for _, name := range []string{"gdrive", "onedrive", "dropbox", "webdav"} {
		for _, encoded := range []bool{false, true} {
			t.Run(name+"/"+strconv.FormatBool(encoded), func(t *testing.T) {
				seen := make(chan string, 1)
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					seen <- r.Header.Get("Accept-Encoding")
					if encoded {
						w.Header().Add("Content-Encoding", "identity")
						w.Header().Add("Content-Encoding", "gzip")
					}
					_, _ = io.WriteString(w, "opaque")
				}))
				defer server.Close()
				adapter := rawAdapter(t, name, rawFixtureClient(server))
				body, err := adapter.GetRaw(t.Context(), tenancy.Tenant{Slug: "acme"}, "blob")
				if encoded {
					if !errors.Is(err, ErrRawUnsupported) || body != nil {
						t.Fatal("encoded response was accepted", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				select {
				case header := <-seen:
					if header != "identity" {
						t.Fatal("raw request did not require identity")
					}
				case <-time.After(5 * time.Second):
					t.Fatal("provider request was not observed")
				}
				if encoded {
					return
				}
				data, readErr := io.ReadAll(body)
				closeErr := body.Close()
				if readErr != nil || closeErr != nil || !bytes.Equal(data, []byte("opaque")) {
					t.Fatal("raw bytes changed", readErr, closeErr)
				}
			})
		}
	}
}

func TestRawHTTPAdaptersRejectUnknownTransportBeforeRequest(t *testing.T) {
	for _, name := range []string{"gdrive", "onedrive", "dropbox", "webdav"} {
		t.Run(name, func(t *testing.T) {
			client := &http.Client{Transport: dropboxTransport(func(*http.Request) (*http.Response, error) {
				t.Fatal("unsupported transport was used")
				return nil, ErrRawUnsupported
			})}
			body, err := rawAdapter(t, name, client).GetRaw(t.Context(), tenancy.Tenant{Slug: "acme"}, "blob")
			if body != nil || !errors.Is(err, ErrRawUnsupported) {
				t.Fatal("unknown transport accepted", err)
			}
		})
	}
}

func TestRawHTTPAdaptersCloseInterruptsBlockedRead(t *testing.T) {
	for _, name := range []string{"gdrive", "onedrive", "dropbox", "webdav"} {
		t.Run(name, func(t *testing.T) {
			started := make(chan struct{})
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
			}))
			defer server.Close()
			adapter := rawAdapter(t, name, rawFixtureClient(server))
			body, err := adapter.GetRaw(t.Context(), tenancy.Tenant{Slug: "acme"}, "blob")
			if err != nil {
				t.Fatal(err)
			}
			<-started
			done := make(chan error, 1)
			go func() { _, err := io.ReadAll(body); done <- err }()
			if err := body.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("canceled read succeeded")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("close did not interrupt read")
			}
		})
	}
}

func TestRawEncodingCheckRejectsTransparentDecompression(t *testing.T) {
	closed := false
	res := &http.Response{Header: make(http.Header), Uncompressed: true, Body: &rawTestCloser{Reader: strings.NewReader("x"), closed: &closed}}
	body, err := acceptRawHTTP(res, func() {})
	if body != nil || !errors.Is(err, ErrRawUnsupported) || !closed {
		t.Fatal("transparent decode was not rejected and closed")
	}
}

func TestOneDriveRawRedirectRetainsIdentityWithoutLeakingAuthorization(t *testing.T) {
	seen := make(chan string, 2)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("Accept-Encoding") + "/" + r.Header.Get("Authorization")
		if r.Host == "graph.microsoft.com" {
			http.Redirect(w, r, "https://content.example/opaque", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "opaque")
	}))
	defer server.Close()
	adapter := rawAdapter(t, "onedrive", rawFixtureClient(server))
	body, err := adapter.GetRaw(t.Context(), tenancy.Tenant{Slug: "acme"}, "blob")
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(body)
	closeErr := body.Close()
	if readErr != nil || closeErr != nil || string(data) != "opaque" {
		t.Fatal("redirect changed body", readErr, closeErr)
	}
	if first, second := <-seen, <-seen; first != "identity/Bearer fixture-token" || second != "identity/" {
		t.Fatal("redirect changed identity or leaked authorization", first, second)
	}
}

type rawTestCloser struct {
	io.Reader
	closed *bool
}

func (r *rawTestCloser) Close() error { *r.closed = true; return nil }

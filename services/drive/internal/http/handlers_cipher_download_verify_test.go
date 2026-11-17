// SPDX-License-Identifier: Apache-2.0
package drivehttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

type cipherDownloadReader struct {
	io.Reader
	closeErr error
	closed   bool
}

type cipherDeadlineRecorder struct{ *httptest.ResponseRecorder }

func (*cipherDeadlineRecorder) SetWriteDeadline(time.Time) error { return nil }

type captureCipherDeadline struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
}

func (w *captureCipherDeadline) SetWriteDeadline(deadline time.Time) error {
	w.deadlines = append(w.deadlines, deadline)
	return nil
}

func (r *cipherDownloadReader) Close() error { r.closed = true; return r.closeErr }

type cipherErrorReader struct{ sent bool }

type cipherMissingReader struct{}

func (cipherMissingReader) Read([]byte) (int, error) { return 0, objectstore.ErrNotFound }

type cipherEOFReader struct{}

func (cipherEOFReader) Read(p []byte) (int, error) { return copy(p, "opaque"), io.EOF }

func (r *cipherErrorReader) Read(p []byte) (int, error) {
	if r.sent {
		return 0, errors.New("secret provider failure")
	}
	r.sent = true
	return copy(p, "opaque"), errors.New("secret provider failure")
}

func cipherDownloadRequest(ctx context.Context, t *testing.T, fixture *cipherHTTPFixture) *httptest.ResponseRecorder {
	t.Helper()
	h := newHandlers(&fakeSvc{})
	h.cipher = fixture
	router := chi.NewRouter()
	router.Get("/cipher/{product}/{id}", h.DownloadCipherBlob)
	ctx = store.WithOwner(tenancy.Into(ctx, tenancy.Tenant{Slug: "current"}), "private-owner")
	req := httptest.NewRequest(http.MethodGet, "/cipher/drive/"+strings.Repeat("A", 26), http.NoBody).WithContext(ctx)
	header, err := storage.EncodeScopeHeader(storage.Scope{Tenant: "current", Subject: "private-owner", Product: "drive"})
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(storage.ScopeHeader, header)
	response := httptest.NewRecorder()
	router.ServeHTTP(&cipherDeadlineRecorder{response}, req)
	return response
}

func cipherDownloadReceipt() domain.CipherBlob {
	sum := sha256.Sum256([]byte("opaque"))
	return domain.CipherBlob{BlobReceipt: storage.BlobReceipt{ID: strings.Repeat("A", 26), Product: "drive", SizeBytes: 6, SHA256: hex.EncodeToString(sum[:])}}
}

func TestCipherDownloadVerifiesCompleteProviderObjectBeforeResponse(t *testing.T) {
	for _, scenario := range []string{"valid", "empty", "maximum", "eof-with-data", "missing", "missing-lazy", "forbidden", "short", "long", "corrupt", "read-error", "close-error", "invalid-receipt", "oversized-receipt"} {
		t.Run(scenario, func(t *testing.T) {
			reader := &cipherDownloadReader{Reader: strings.NewReader("opaque")}
			fixture := &cipherHTTPFixture{blob: cipherDownloadReceipt(), openBody: reader}
			wantBody := "opaque"
			switch scenario {
			case "empty", "maximum":
				wantBody = ""
				if scenario == "maximum" {
					wantBody = strings.Repeat("z", int(storage.MaxCipherBlockBytes))
				}
				reader.Reader = strings.NewReader(wantBody)
				fixture.blob.SizeBytes = int64(len(wantBody))
				sum := sha256.Sum256([]byte(wantBody))
				fixture.blob.SHA256 = hex.EncodeToString(sum[:])
			case "eof-with-data":
				reader.Reader = cipherEOFReader{}
			case "missing":
				fixture.openBody = nil
				fixture.openErr = domain.ErrNotFound
			case "missing-lazy":
				reader.Reader = cipherMissingReader{}
			case "forbidden":
				fixture.openBody = nil
				fixture.openErr = domain.ErrPermission
			case "short":
				reader.Reader = strings.NewReader("opaq")
			case "long":
				reader.Reader = strings.NewReader("opaque-secret-suffix")
			case "corrupt":
				reader.Reader = strings.NewReader("opaqUe")
			case "read-error":
				reader.Reader = &cipherErrorReader{}
			case "close-error":
				reader.closeErr = errors.New("secret close failure")
			case "invalid-receipt":
				fixture.blob.SHA256 = "invalid"
			case "oversized-receipt":
				fixture.blob.SizeBytes = storage.MaxCipherBlockBytes + 1
			}
			response := cipherDownloadRequest(t.Context(), t, fixture)
			if scenario == "valid" || scenario == "eof-with-data" || scenario == "empty" || scenario == "maximum" {
				if response.Code != http.StatusOK || response.Body.String() != wantBody || response.Header().Get("Content-Length") != strconv.Itoa(len(wantBody)) || response.Header().Get("Content-Type") != "application/octet-stream" {
					t.Fatal("verified object was not returned correctly", response.Code)
				}
			} else {
				want := http.StatusServiceUnavailable
				if scenario == "missing" || scenario == "missing-lazy" {
					want = http.StatusNotFound
				}
				if scenario == "forbidden" {
					want = http.StatusForbidden
				}
				if response.Code != want || response.Header().Get("Content-Type") == "application/octet-stream" || response.Header().Get("Content-Length") == "6" || strings.Contains(response.Body.String(), "opaque") || strings.Contains(response.Body.String(), "secret") {
					t.Fatal("invalid provider object escaped", response.Code, response.Body.String())
				}
			}
			if scenario != "missing" && scenario != "forbidden" && !reader.closed {
				t.Fatal("provider body was not closed")
			}
		})
	}
}

type blockedCipherDownload struct {
	started chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (r *blockedCipherDownload) Read([]byte) (int, error) {
	close(r.started)
	<-r.closed
	return 0, errors.New("closed")
}
func (r *blockedCipherDownload) Close() error { r.once.Do(func() { close(r.closed) }); return nil }

func TestCipherDownloadCancellationClosesProviderAndNeverSucceeds(t *testing.T) {
	reader := &blockedCipherDownload{started: make(chan struct{}), closed: make(chan struct{})}
	fixture := &cipherHTTPFixture{blob: cipherDownloadReceipt(), openBody: reader}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- cipherDownloadRequest(ctx, t, fixture) }()
	select {
	case <-reader.started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider read did not start")
	}
	cancel()
	select {
	case response := <-done:
		if response.Code != http.StatusServiceUnavailable {
			t.Fatal("cancelled download succeeded", response.Code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not interrupt provider read")
	}
	select {
	case <-reader.closed:
	default:
		t.Fatal("provider body was not closed")
	}
}

func TestCipherDownloadInheritedDeadlineInterruptsBlockedProvider(t *testing.T) {
	reader := &blockedCipherDownload{started: make(chan struct{}), closed: make(chan struct{})}
	fixture := &cipherHTTPFixture{blob: cipherDownloadReceipt(), openBody: reader}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- cipherDownloadRequest(ctx, t, fixture) }()
	select {
	case <-reader.started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider read did not start")
	}
	select {
	case response := <-done:
		if response.Code != http.StatusServiceUnavailable {
			t.Fatal("timed-out download succeeded", response.Code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deadline did not interrupt provider read")
	}
	select {
	case <-reader.closed:
	default:
		t.Fatal("timeout did not close provider body")
	}
}

func TestCipherDownloadSlotWaitHonorsCancellation(t *testing.T) {
	for i := 0; i < cap(cipherDownloadSlots); i++ {
		cipherDownloadSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(cipherDownloadSlots); i++ {
			<-cipherDownloadSlots
		}
	}()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	fixture := &cipherHTTPFixture{blob: cipherDownloadReceipt()}
	response := cipherDownloadRequest(ctx, t, fixture)
	if response.Code != http.StatusServiceUnavailable || fixture.calls != 0 {
		t.Fatal("cancelled waiter reached provider", response.Code, fixture.calls)
	}
}

type blockedCipherResponse struct {
	header  http.Header
	entered chan<- struct{}
	release <-chan struct{}
}

func (w *blockedCipherResponse) Header() http.Header            { return w.header }
func (*blockedCipherResponse) WriteHeader(int)                  {}
func (*blockedCipherResponse) SetWriteDeadline(time.Time) error { return nil }
func (w *blockedCipherResponse) Write(p []byte) (int, error) {
	w.entered <- struct{}{}
	<-w.release
	return len(p), nil
}

func TestCipherDownloadSlotsCoverSlowResponseWrites(t *testing.T) {
	entered := make(chan struct{}, cap(cipherDownloadSlots))
	release := make(chan struct{})
	releaseAll := sync.OnceFunc(func() { close(release) })
	defer releaseAll()
	done := make(chan struct{}, cap(cipherDownloadSlots))
	header, err := storage.EncodeScopeHeader(storage.Scope{Tenant: "current", Subject: "private-owner", Product: "drive"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < cap(cipherDownloadSlots); i++ {
		fixture := &cipherHTTPFixture{blob: cipherDownloadReceipt(), data: "opaque"}
		h := newHandlers(&fakeSvc{})
		h.cipher = fixture
		router := chi.NewRouter()
		router.Get("/cipher/{product}/{id}", h.DownloadCipherBlob)
		ctx := store.WithOwner(tenancy.Into(t.Context(), tenancy.Tenant{Slug: "current"}), "private-owner")
		req := httptest.NewRequest(http.MethodGet, "/cipher/drive/"+strings.Repeat("A", 26), http.NoBody).WithContext(ctx)
		req.Header.Set(storage.ScopeHeader, header)
		writer := &blockedCipherResponse{header: make(http.Header), entered: entered, release: release}
		go func() { router.ServeHTTP(writer, req); done <- struct{}{} }()
	}
	for i := 0; i < cap(cipherDownloadSlots); i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("response did not reach blocked write")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	fixture := &cipherHTTPFixture{blob: cipherDownloadReceipt()}
	response := cipherDownloadRequest(ctx, t, fixture)
	if response.Code != http.StatusServiceUnavailable || fixture.calls != 0 {
		t.Fatal("slow writers did not hold the download slots")
	}
	releaseAll()
	for i := 0; i < cap(cipherDownloadSlots); i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("download did not release slot")
		}
	}
}

type deadlineCipherResponse struct {
	header  http.Header
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *deadlineCipherResponse) Header() http.Header { return w.header }
func (*deadlineCipherResponse) WriteHeader(int)       {}
func (w *deadlineCipherResponse) SetWriteDeadline(deadline time.Time) error {
	if !deadline.IsZero() && !deadline.After(time.Now()) {
		w.once.Do(func() { close(w.release) })
	}
	return nil
}

func (w *deadlineCipherResponse) Write([]byte) (int, error) {
	close(w.entered)
	<-w.release
	return 0, context.Canceled
}

func TestCipherDownloadCancellationInterruptsBlockedClient(t *testing.T) {
	fixture := &cipherHTTPFixture{blob: cipherDownloadReceipt(), data: "opaque"}
	h := newHandlers(&fakeSvc{})
	h.cipher = fixture
	router := chi.NewRouter()
	router.Get("/cipher/{product}/{id}", h.DownloadCipherBlob)
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := store.WithOwner(tenancy.Into(parent, tenancy.Tenant{Slug: "current"}), "private-owner")
	req := httptest.NewRequest(http.MethodGet, "/cipher/drive/"+strings.Repeat("A", 26), http.NoBody).WithContext(ctx)
	header, err := storage.EncodeScopeHeader(storage.Scope{Tenant: "current", Subject: "private-owner", Product: "drive"})
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(storage.ScopeHeader, header)
	writer := &deadlineCipherResponse{header: make(http.Header), entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan struct{})
	go func() { router.ServeHTTP(writer, req); close(done) }()
	select {
	case <-writer.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("client write did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("client write did not respect cancellation")
	}
}

func TestCipherDownloadFailsClosedWhenWriteDeadlineUnsupported(t *testing.T) {
	fixture := &cipherHTTPFixture{blob: cipherDownloadReceipt(), data: "opaque"}
	h := newHandlers(&fakeSvc{})
	h.cipher = fixture
	router := chi.NewRouter()
	router.Get("/cipher/{product}/{id}", h.DownloadCipherBlob)
	ctx := store.WithOwner(tenancy.Into(t.Context(), tenancy.Tenant{Slug: "current"}), "private-owner")
	req := httptest.NewRequest(http.MethodGet, "/cipher/drive/"+strings.Repeat("A", 26), http.NoBody).WithContext(ctx)
	header, err := storage.EncodeScopeHeader(storage.Scope{Tenant: "current", Subject: "private-owner", Product: "drive"})
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(storage.ScopeHeader, header)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "opaque") {
		t.Fatal("unsupported writer returned ciphertext", response.Code)
	}
}

func TestCipherDownloadHonorsEarlierDeadline(t *testing.T) {
	fixture := &cipherHTTPFixture{blob: cipherDownloadReceipt(), data: "opaque"}
	h := newHandlers(&fakeSvc{})
	h.cipher = fixture
	router := chi.NewRouter()
	router.Get("/cipher/{product}/{id}", h.DownloadCipherBlob)
	expected := time.Now().Add(10 * time.Second)
	parent, cancel := context.WithDeadline(t.Context(), expected)
	defer cancel()
	ctx := store.WithOwner(tenancy.Into(parent, tenancy.Tenant{Slug: "current"}), "private-owner")
	req := httptest.NewRequest(http.MethodGet, "/cipher/drive/"+strings.Repeat("A", 26), http.NoBody).WithContext(ctx)
	header, err := storage.EncodeScopeHeader(storage.Scope{Tenant: "current", Subject: "private-owner", Product: "drive"})
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(storage.ScopeHeader, header)
	response := &captureCipherDeadline{ResponseRecorder: httptest.NewRecorder()}
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK || len(response.deadlines) != 2 || !response.deadlines[0].Equal(expected) || !response.deadlines[1].IsZero() {
		t.Fatal("write deadline did not preserve and clear inherited deadline", response.Code, response.deadlines)
	}
}

// Exercise actual net/http TLS transports through the same logging, metrics,
// timeout and security middleware used by Drive. The fixture is not S3.
func TestCipherDownloadWithHTTPMiddlewareAndRealTransports(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			fixture := &cipherHTTPFixture{blob: cipherDownloadReceipt(), data: "opaque"}
			h := newHandlers(&fakeSvc{})
			h.cipher = fixture
			router := httpx.NewRouter(httpx.RouterOptions{RequestTimeout: time.Minute})
			router.Get("/cipher/{product}/{id}", h.DownloadCipherBlob)
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := store.WithOwner(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "current"}), "private-owner")
				router.ServeHTTP(w, r.WithContext(ctx))
			})
			server := httptest.NewUnstartedServer(handler)
			server.EnableHTTP2 = version == 2
			server.StartTLS()
			defer server.Close()
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/cipher/drive/"+strings.Repeat("A", 26), http.NoBody)
			if err != nil {
				t.Fatal(err)
			}
			header, err := storage.EncodeScopeHeader(storage.Scope{Tenant: "current", Subject: "private-owner", Product: "drive"})
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set(storage.ScopeHeader, header)
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			data, err := io.ReadAll(response.Body)
			if err != nil || response.ProtoMajor != version || response.StatusCode != http.StatusOK || string(data) != "opaque" {
				t.Fatal("transport rejected verified content", version, response.Proto, response.StatusCode, err)
			}
		})
	}
}

// SPDX-License-Identifier: Apache-2.0
package httpx_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

var errHTTPSourceIngress = errors.New("verified continuing ingress is not installed")

type httpSourcePort struct {
	scope              storage.Scope
	epoch              uuid.UUID
	state              storage.AppMigrationState
	page               storage.StreamSourcePage
	row                storage.StreamSourceRow
	body               func() io.ReadCloser
	stateErr, beginErr error
	calls              atomic.Int64
}

func (p *httpSourcePort) check(scope storage.Scope) error {
	p.calls.Add(1)
	if scope != p.scope {
		return errors.New("unverified scope reached port")
	}
	return nil
}

func (p *httpSourcePort) State(_ context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	if err := p.check(scope); err != nil {
		return storage.AppMigrationState{}, err
	}
	return p.state, p.stateErr
}

func (p *httpSourcePort) Begin(_ context.Context, scope storage.Scope, _ uuid.UUID) (storage.AppMigrationState, error) {
	if err := p.check(scope); err != nil {
		return storage.AppMigrationState{}, err
	}
	return p.state, p.beginErr
}

func (p *httpSourcePort) Page(_ context.Context, scope storage.Scope, _ uuid.UUID, _ storage.LegacySourceCursor, _ int) (storage.StreamSourcePage, error) {
	if err := p.check(scope); err != nil {
		return storage.StreamSourcePage{}, err
	}
	return p.page, nil
}

func (p *httpSourcePort) Open(_ context.Context, scope storage.Scope, _ uuid.UUID, _, _ string) (storage.StreamSourceRow, io.ReadCloser, error) {
	if err := p.check(scope); err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	return p.row, p.body(), nil
}

type httpSourceBody struct {
	*bytes.Reader
	closes, reads atomic.Int64
}

func (b *httpSourceBody) Read(p []byte) (int, error) { b.reads.Add(1); return b.Reader.Read(p) }
func (b *httpSourceBody) Close() error               { b.closes.Add(1); return nil }

func httpSourceRow(data []byte) storage.StreamSourceRow {
	sum := sha256.Sum256(data)
	return storage.StreamSourceRow{Kind: "object", ID: "original", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
}

func newHTTPSourcePort() *httpSourcePort {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}
	epoch := uuid.New()
	data := []byte(`{"future_integer":9007199254740993,"original":"ICS"}`)
	row := httpSourceRow(data)
	return &httpSourcePort{scope: scope, epoch: epoch, state: storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}, page: storage.StreamSourcePage{Scope: scope, Epoch: epoch.String(), Rows: []storage.StreamSourceRow{row}}, row: row, body: func() io.ReadCloser { return &httpSourceBody{Reader: bytes.NewReader(data)} }, beginErr: errHTTPSourceIngress}
}

func newHTTPSourceRouter(t *testing.T, p *httpSourcePort) http.Handler {
	t.Helper()
	handlers, err := httpx.NewPrivateSourceHandlers(httpx.PrivateSourceHTTPConfig{Product: p.scope.Product, Port: p, MaxBodyBytes: 32 << 20, Kind: func(kind string) bool { return kind == "object" || kind == "mime_message" }, Identity: func(ctx context.Context) (httpx.PrivateSourceIdentity, bool) {
		claims, ok := authn.FromContext(ctx)
		return httpx.PrivateSourceIdentity{Tenant: claims.Tenant, Subject: claims.Subject, Delegado: claims.Delegado}, ok
	}, MapError: func(err error) *httpx.Error {
		if errors.Is(err, errHTTPSourceIngress) {
			return httpx.ErrUnavailable.WithCode("private_calendar_ingress_required")
		}
		return httpx.ErrInternal.WithMessage("secret original content").WithParams(map[string]any{"secret": "secret original content"}).WithDetails("secret original content").Wrap(err)
	}})
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/migration", handlers.State)
	router.Post("/migration", handlers.Begin)
	router.Get("/migration/{epoch}/rows", handlers.Page)
	router.Get("/migration/{epoch}/source/{kind}/{source}", handlers.Body)
	return router
}

func httpSourceRequest(t *testing.T, p *httpSourcePort, method, path, body string) *http.Request {
	t.Helper()
	ctx := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: p.scope.Tenant}), authn.Claims{Tenant: p.scope.Tenant, Subject: p.scope.Subject, Email: "alice@example.invalid"})
	r := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
	header, err := storage.EncodeScopeHeader(p.scope)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set(storage.ScopeHeader, header)
	return r
}

func serveHTTPSource(router http.Handler, w http.ResponseWriter, r *http.Request) (recovered any) {
	defer func() { recovered = recover() }()
	router.ServeHTTP(w, r)
	return nil
}

func TestPrivateSourceHTTPRequiresAuthoritativeContextBeforePort(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*http.Request)
	}{
		{"missing_claims", func(r *http.Request) {
			*r = *r.WithContext(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}))
		}},
		{"delegated", func(r *http.Request) {
			*r = *r.WithContext(authn.Into(r.Context(), authn.Claims{Tenant: "acme", Subject: "alice", Delegado: true}))
		}},
		{"tenant_context_changed", func(r *http.Request) { *r = *r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "foreign"})) }},
		{"forged_header", func(r *http.Request) {
			value, _ := storage.EncodeScopeHeader(storage.Scope{Tenant: "foreign", Subject: "bob", Product: "calendar"})
			r.Header.Set(storage.ScopeHeader, value)
		}},
		{"duplicate_header", func(r *http.Request) { r.Header["x-sytue-storage-scope"] = r.Header.Values(storage.ScopeHeader) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := newHTTPSourcePort()
			router := newHTTPSourceRouter(t, p)
			r := httpSourceRequest(t, p, http.MethodGet, "/migration?subject=bob&tenant=foreign", "")
			test.change(r)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code < 400 || p.calls.Load() != 0 {
				t.Fatal("request routing overrode verified identity", w.Code, p.calls.Load())
			}
		})
	}
	p := newHTTPSourcePort()
	w := httptest.NewRecorder()
	newHTTPSourceRouter(t, p).ServeHTTP(w, httpSourceRequest(t, p, http.MethodGet, "/migration?subject=bob&tenant=foreign", ""))
	if w.Code != http.StatusOK || p.calls.Load() != 1 {
		t.Fatal("URL fields changed authoritative context", w.Code)
	}
}

func TestPrivateSourceHTTPRejectsBodyMetadataBeforeReadingAndStillCloses(t *testing.T) {
	for _, change := range []func(*httpSourcePort){
		func(p *httpSourcePort) { p.row.ID = "another-owner-row" },
		func(p *httpSourcePort) { p.row.Kind = "unknown" },
		func(p *httpSourcePort) { p.row.Size = 64 << 20 },
		func(p *httpSourcePort) { p.row.SHA256 = "invalid" },
	} {
		p := newHTTPSourcePort()
		body := &httpSourceBody{Reader: bytes.NewReader([]byte("original"))}
		p.body = func() io.ReadCloser { return body }
		change(p)
		w := httptest.NewRecorder()
		newHTTPSourceRouter(t, p).ServeHTTP(w, httpSourceRequest(t, p, http.MethodGet, "/migration/"+p.epoch.String()+"/source/object/original", ""))
		if w.Code != http.StatusInternalServerError || body.reads.Load() != 0 || body.closes.Load() != 1 {
			t.Fatal("invalid descriptor reached source content", w.Code, body.reads.Load(), body.closes.Load())
		}
	}
}

func TestPrivateSourceHTTPBeginDoesNotInferIngressFromFrozenHistory(t *testing.T) {
	p := newHTTPSourcePort()
	started := time.Now().UTC()
	p.state.Mode = "frozen"
	p.state.Epoch = p.epoch.String()
	p.state.StartedAt = &started
	p.state.Rows = 1
	router := newHTTPSourceRouter(t, p)
	for _, body := range []string{`{"epoch":"` + p.epoch.String() + `","scope":{"subject":"bob"}}`, `{"epoch":"00000000-0000-0000-0000-000000000000"}`} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httpSourceRequest(t, p, http.MethodPost, "/migration", body))
		if w.Code != http.StatusBadRequest || p.calls.Load() != 0 {
			t.Fatal("unverified migration body reached domain", w.Code, p.calls.Load())
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpSourceRequest(t, p, http.MethodPost, "/migration", `{"epoch":"`+p.epoch.String()+`"}`))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "private_calendar_ingress_required") {
		t.Fatal("frozen history opened continuing ingress", w.Code, w.Body.String())
	}
	p.beginErr = nil
	p.state.Mode = "legacy"
	p.state.Epoch = ""
	p.state.StartedAt = nil
	p.state.Rows = 0
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httpSourceRequest(t, p, http.MethodPost, "/migration", `{"epoch":"`+p.epoch.String()+`"}`))
	if w.Code != http.StatusInternalServerError {
		t.Fatal("empty legacy success was accepted as frozen", w.Code)
	}
}

func TestPrivateSourceHTTPValidatesEpochCursorPageAndResponseScope(t *testing.T) {
	p := newHTTPSourcePort()
	router := newHTTPSourceRouter(t, p)
	for _, path := range []string{"/migration/00000000-0000-0000-0000-000000000000/rows", "/migration/" + p.epoch.String() + "/rows?after=invalid!", "/migration/" + p.epoch.String() + "/rows?limit=0", "/migration/" + p.epoch.String() + "/rows?limit=1&limit=2", "/migration/" + p.epoch.String() + "/source/unknown/original"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httpSourceRequest(t, p, http.MethodGet, path, ""))
		if w.Code != http.StatusBadRequest || p.calls.Load() != 0 {
			t.Fatal("invalid source request reached port", path, w.Code, p.calls.Load())
		}
	}
	p.page.Scope.Subject = "bob"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpSourceRequest(t, p, http.MethodGet, "/migration/"+p.epoch.String()+"/rows", ""))
	if w.Code != http.StatusInternalServerError {
		t.Fatal("foreign page scope accepted", w.Code)
	}
	p.page.Scope = p.scope
	p.page.Epoch = uuid.NewString()
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httpSourceRequest(t, p, http.MethodGet, "/migration/"+p.epoch.String()+"/rows", ""))
	if w.Code != http.StatusInternalServerError {
		t.Fatal("foreign page epoch accepted", w.Code)
	}
	p.page.Epoch = p.epoch.String()
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httpSourceRequest(t, p, http.MethodGet, "/migration/"+p.epoch.String()+"/rows", ""))
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("valid bounded source page rejected", w.Code)
	}
}

func TestPrivateSourceHTTPErrorsNeverExposeRawMessagesOrRouting(t *testing.T) {
	p := newHTTPSourcePort()
	p.stateErr = errors.New("postgresql://SECRET-DSN secret original content")
	router := newHTTPSourceRouter(t, p)
	r := httpSourceRequest(t, p, http.MethodGet, "/migration", "")
	var logs bytes.Buffer
	r = r.WithContext(log.Into(r.Context(), slog.New(slog.NewJSONHandler(&logs, nil))))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	for _, secret := range []string{"SECRET-DSN", "secret original content", "postgresql://"} {
		if strings.Contains(w.Body.String()+logs.String(), secret) {
			t.Fatal("source error escaped safe status/code mapping")
		}
	}
	if w.Header().Get("Location") != "" || w.Code != http.StatusInternalServerError {
		t.Fatal("source error redirected or changed status", w.Code)
	}
}

func TestPrivateSourceHTTPStreamsExactBytesAndClosesOnce(t *testing.T) {
	p := newHTTPSourcePort()
	data := bytes.Repeat([]byte("original Unicode / 漢字 \n"), 5000)
	p.row = httpSourceRow(data)
	body := &httpSourceBody{Reader: bytes.NewReader(data)}
	p.body = func() io.ReadCloser { return body }
	w := httptest.NewRecorder()
	if recovered := serveHTTPSource(newHTTPSourceRouter(t, p), w, httpSourceRequest(t, p, http.MethodGet, "/migration/"+p.epoch.String()+"/source/object/original", "")); recovered != nil {
		t.Fatal("valid source stream aborted", recovered)
	}
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), data) || w.Header().Get("X-Sytue-Source-SHA256") != p.row.SHA256 || body.closes.Load() != 1 {
		t.Fatal("exact source stream or cleanup changed", w.Code, body.closes.Load())
	}
}

func TestPrivateSourceHTTPWithholdsFinalChunkOnHashOrLengthMismatch(t *testing.T) {
	for _, test := range []struct {
		name     string
		declared int
		hash     bool
	}{{"corrupt_hash", 100000, true}, {"short_body", 120000, false}, {"extra_byte", 99999, false}} {
		t.Run(test.name, func(t *testing.T) {
			p := newHTTPSourcePort()
			data := bytes.Repeat([]byte("x"), 100000)
			p.row = httpSourceRow(data)
			p.row.Size = int64(test.declared)
			if test.hash {
				p.row.SHA256 = strings.Repeat("0", 64)
			}
			body := &httpSourceBody{Reader: bytes.NewReader(data)}
			p.body = func() io.ReadCloser { return body }
			w := httptest.NewRecorder()
			recovered := serveHTTPSource(newHTTPSourceRouter(t, p), w, httpSourceRequest(t, p, http.MethodGet, "/migration/"+p.epoch.String()+"/source/object/original", ""))
			failure, ok := recovered.(error)
			if !ok || !errors.Is(failure, http.ErrAbortHandler) || w.Body.Len() >= test.declared || body.closes.Load() != 1 {
				t.Fatal("invalid stream delivered complete framing", test.name, recovered, w.Body.Len(), body.closes.Load())
			}
		})
	}
	p := newHTTPSourcePort()
	p.row.SHA256 = strings.Repeat("0", 64)
	w := httptest.NewRecorder()
	recovered := serveHTTPSource(newHTTPSourceRouter(t, p), w, httpSourceRequest(t, p, http.MethodGet, "/migration/"+p.epoch.String()+"/source/object/original", ""))
	if recovered != nil || w.Code != http.StatusInternalServerError || w.Header().Get("Content-Length") != "" {
		t.Fatal("small corrupt source escaped as successful bytes", w.Code, recovered)
	}
}

type blockedHTTPSourceBody struct {
	started, closed      chan struct{}
	startOnce, closeOnce sync.Once
	closes               atomic.Int64
}

func (b *blockedHTTPSourceBody) Read([]byte) (int, error) {
	b.startOnce.Do(func() { close(b.started) })
	<-b.closed
	return 0, context.Canceled
}

func (b *blockedHTTPSourceBody) Close() error {
	b.closes.Add(1)
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}

func TestPrivateSourceHTTPCancellationClosesBlockedStream(t *testing.T) {
	p := newHTTPSourcePort()
	body := &blockedHTTPSourceBody{started: make(chan struct{}), closed: make(chan struct{})}
	p.body = func() io.ReadCloser { return body }
	defer func() {
		if body.closes.Load() == 0 {
			_ = body.Close()
		}
	}()
	r := httpSourceRequest(t, p, http.MethodGet, "/migration/"+p.epoch.String()+"/source/object/original", "")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(ctx)
	done := make(chan any, 1)
	router := newHTTPSourceRouter(t, p)
	go func() { done <- serveHTTPSource(router, httptest.NewRecorder(), r) }()
	select {
	case <-body.started:
	case <-time.After(3 * time.Second):
		t.Fatal("source stream did not start")
	}
	cancel()
	select {
	case recovered := <-done:
		if recovered != nil {
			t.Fatal("unwritten cancelled body panicked", recovered)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation left source Read blocked")
	}
	if body.closes.Load() != 1 {
		t.Fatal("source stream was not closed exactly once", body.closes.Load())
	}
}

type generatedHTTPSource struct {
	remaining       int64
	maxRead, closes atomic.Int64
}

func (r *generatedHTTPSource) Read(p []byte) (int, error) {
	if int64(len(p)) > r.maxRead.Load() {
		r.maxRead.Store(int64(len(p)))
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > r.remaining {
		n = r.remaining
	}
	clear(p[:int(n)])
	r.remaining -= n
	return int(n), nil
}
func (r *generatedHTTPSource) Close() error { r.closes.Add(1); return nil }

type countingHTTPSourceWriter struct {
	header   http.Header
	size     int64
	maxChunk int
	status   int
}

func (w *countingHTTPSourceWriter) Header() http.Header    { return w.header }
func (w *countingHTTPSourceWriter) WriteHeader(status int) { w.status = status }
func (w *countingHTTPSourceWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.size += int64(len(p))
	if len(p) > w.maxChunk {
		w.maxChunk = len(p)
	}
	return len(p), nil
}

func TestPrivateSourceHTTPKeepsLargeMIMEStreamingBounded(t *testing.T) {
	const size = 16 << 20
	sum := sha256.New()
	if _, err := io.Copy(sum, &generatedHTTPSource{remaining: size}); err != nil {
		t.Fatal(err)
	}
	p := newHTTPSourcePort()
	p.scope.Product = "mail"
	p.row = storage.StreamSourceRow{Kind: "mime_message", ID: "original", Size: size, SHA256: hex.EncodeToString(sum.Sum(nil))}
	body := &generatedHTTPSource{remaining: size}
	p.body = func() io.ReadCloser { return body }
	w := &countingHTTPSourceWriter{header: make(http.Header)}
	recovered := serveHTTPSource(newHTTPSourceRouter(t, p), w, httpSourceRequest(t, p, http.MethodGet, "/migration/"+p.epoch.String()+"/source/mime_message/original", ""))
	if recovered != nil || w.status != http.StatusOK || w.size != size || w.maxChunk > 32<<10 || body.maxRead.Load() > 32<<10 || body.closes.Load() != 1 {
		t.Fatal("MIME was materialized or unbounded", recovered, w.status, w.size, w.maxChunk, body.maxRead.Load(), body.closes.Load())
	}
}

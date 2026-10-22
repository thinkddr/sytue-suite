// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package policy_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

// fakeSink captura las filas de egress auditadas.
type fakeSink struct {
	mu      sync.Mutex
	records []domain.EgressAudit
}

func (s *fakeSink) RecordEgress(_ context.Context, rec domain.EgressAudit) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, rec)
	return nil
}

// fakeLimiter permite/deniega y captura la última clave consultada.
type fakeLimiter struct {
	allow   bool
	lastKey string
}

func (l *fakeLimiter) Allow(_ context.Context, key string) (httpx.RateLimitResult, error) {
	l.lastKey = key
	return httpx.RateLimitResult{Allowed: l.allow}, nil
}

// allowLoopback relaja la barrera SSRF SOLO en loopback, que es donde escucha
// httptest, y deja el resto de la política tal como en producción. Se restaura al
// terminar el test (var de proceso, no concurrente).
//
// Antes devolvía `false` para CUALQUIER IP, es decir, desactivaba la barrera entera:
// con eso, todos los tests que la usan pasarían igual con la defensa quitada —el test
// que no sirve para nada—. Ahora un destino privado o de metadatos sigue bloqueado
// dentro del test, igual que en producción.
func allowLoopback(t *testing.T) {
	t.Helper()
	restore := ssrfhttp.SetIPPolicyForTest(func(ip net.IP) bool {
		if ip != nil && ip.IsLoopback() {
			return false // permitida: es el servidor del test
		}
		return ssrfhttp.IsBlockedIP(ip) // el resto, como en producción
	})
	t.Cleanup(restore)
}

func TestPolicyInjectsAuthAndAuditsRedacted(t *testing.T) {
	allowLoopback(t)

	const token = "super-secret-token"
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sink := &fakeSink{}
	limiter := &fakeLimiter{allow: true}
	rt := policy.BuildRuntime(policy.RuntimeParams{
		OrgID:      "acme",
		InstanceID: "inst-1",
		Bearer:     token,
		Limiter:    limiter,
		Audit:      sink,
		Log:        slog.Default(),
	})

	resp, err := rt.HTTP.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", resp.StatusCode)
	}

	// (5) El servidor recibió el Bearer inyectado por la pasarela (el driver no lo fija).
	if gotAuth != "Bearer "+token {
		t.Fatalf("Authorization recibido = %q, quiero %q", gotAuth, "Bearer "+token)
	}

	// (2) El limitador se consultó con la clave org:instance.
	if limiter.lastKey != "acme:inst-1" {
		t.Fatalf("clave de rate-limit = %q, quiero acme:inst-1", limiter.lastKey)
	}

	// (3) Se auditó exactamente una fila, con Authorization REDACTADO y sin token.
	if len(sink.records) != 1 {
		t.Fatalf("filas de egress_audit = %d, quiero 1", len(sink.records))
	}
	rec := sink.records[0]
	if rec.Auth != "REDACTED" {
		t.Fatalf("auth auditado = %q, quiero REDACTED", rec.Auth)
	}
	if rec.OrgID != "acme" || rec.InstanceID != "inst-1" || rec.Method != http.MethodGet || rec.Status != 200 {
		t.Fatalf("fila de auditoría inesperada: %+v", rec)
	}
	// El token NO debe aparecer en NINGÚN campo textual de la auditoría.
	blob := strings.Join([]string{rec.Auth, rec.Host, rec.Method, rec.Err}, "|")
	if strings.Contains(blob, token) {
		t.Fatalf("el token se filtró en egress_audit: %q", blob)
	}
}

func TestPolicyBlocksWhenRateLimited(t *testing.T) {
	allowLoopback(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sink := &fakeSink{}
	rt := policy.BuildRuntime(policy.RuntimeParams{
		OrgID:      "acme",
		InstanceID: "inst-1",
		Limiter:    &fakeLimiter{allow: false},
		Audit:      sink,
		Log:        slog.Default(),
	})

	if _, err := rt.HTTP.Get(srv.URL); !errors.Is(err, policy.ErrRateLimited) {
		t.Fatalf("error = %v, quiero ErrRateLimited", err)
	}
	// Una petición denegada no llega a egresar: no se audita.
	if len(sink.records) != 0 {
		t.Fatalf("una petición limitada no debe auditarse; hay %d filas", len(sink.records))
	}
}

func TestPolicyBlocksPrivateDestination(t *testing.T) {
	// SIN relajar la política: la barrera SSRF de producción bloquea loopback.
	sink := &fakeSink{}
	rt := policy.BuildRuntime(policy.RuntimeParams{
		OrgID:      "acme",
		InstanceID: "inst-1",
		Limiter:    &fakeLimiter{allow: true},
		Audit:      sink,
		Log:        slog.Default(),
	})

	// 127.0.0.1 es loopback: destino privado no permitido.
	if _, err := rt.HTTP.Get("http://127.0.0.1:1/"); !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("error = %v, quiero ssrfhttp.ErrBlocked", err)
	}
	if len(sink.records) != 0 {
		t.Fatalf("un destino bloqueado no debe egresar ni auditarse; hay %d filas", len(sink.records))
	}
}

// TestRoundTrip_driverAuthWins: si el driver ya compuso su cabecera (Jira usa Basic con
// correo + API token, ServiceNow igual), la pasarela NO la pisa con el bearer del vault.
// Antes la pisaba, así que en cuanto una instancia guardaba un `bearer` para otra de sus
// capacidades, sus consultas empezaban a devolver 401.
func TestRoundTrip_driverAuthWins(t *testing.T) {
	allowLoopback(t)

	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rt := policy.BuildRuntime(policy.RuntimeParams{
		OrgID:      "acme",
		InstanceID: "inst-1",
		Bearer:     "token-de-la-pasarela",
		Limiter:    &fakeLimiter{allow: true},
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Basic ZGV2OnNlY3JldA==")
	resp, err := rt.HTTP.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if got != "Basic ZGV2OnNlY3JldA==" {
		t.Errorf("Authorization = %q; la cabecera del driver debe sobrevivir al bearer de la pasarela", got)
	}
}

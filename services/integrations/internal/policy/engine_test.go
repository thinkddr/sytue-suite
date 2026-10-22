// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package policy_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

// fakeStore satisface policy.DecisionLoader: sirve un conjunto fijo de reglas y
// captura las decisiones registradas. loads cuenta las cargas (para probar la caché).
type fakeStore struct {
	mu    sync.Mutex
	pols  []domain.Policy
	loads int
	recs  []domain.PolicyDecision
}

func (f *fakeStore) LoadPolicies(_ context.Context, _ string) ([]domain.Policy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loads++
	return append([]domain.Policy(nil), f.pols...), nil
}

func (f *fakeStore) RecordDecision(_ context.Context, d domain.PolicyDecision) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recs = append(f.recs, d)
	return nil
}

func (f *fakeStore) decisions() []domain.PolicyDecision {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.PolicyDecision(nil), f.recs...)
}

func (f *fakeStore) loadCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loads
}

// pol construye una política habilitada.
func pol(id, name string, action domain.PolicyAction, cond string) domain.Policy {
	return domain.Policy{ID: id, Name: name, Action: action, Condition: json.RawMessage(cond), Enabled: true}
}

// egressServer devuelve un servidor que cuenta cuántas veces lo golpean: si una
// regla `block` funciona, el contador debe quedar en 0 (el request nunca salió).
func egressServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// buildRT arma un Runtime con el motor de gobernanza cableado (y captura el egress
// auditado en sink para comprobar que un block NO deja fila de auditoría).
func buildRT(fs *fakeStore, apiFamily string, sink *fakeSink) connector.Runtime {
	eng := policy.NewEngine(fs, time.Minute, slog.Default())
	return policy.BuildRuntime(policy.RuntimeParams{
		OrgID:      "acme",
		InstanceID: "inst-1",
		APIFamily:  apiFamily,
		Policy:     eng,
		Audit:      sink,
		Log:        slog.Default(),
	})
}

func TestPolicyBlockDoesNotEgress(t *testing.T) {
	allowLoopback(t)
	srv, hits := egressServer(t)

	fs := &fakeStore{pols: []domain.Policy{
		pol("r1", "bloquear salesforce", domain.PolicyBlock, `{"api_family":"salesforce"}`),
	}}
	sink := &fakeSink{}
	rt := buildRT(fs, "salesforce", sink)

	_, err := rt.HTTP.Get(srv.URL)
	if !errors.Is(err, policy.ErrBlockedByPolicy) {
		t.Fatalf("error = %v, quiero ErrBlockedByPolicy", err)
	}
	// El transporte subyacente NUNCA se invocó: el servidor no recibió nada.
	if got := hits.Load(); got != 0 {
		t.Fatalf("un request bloqueado no debe salir; golpes al servidor = %d", got)
	}
	// La decisión de bloqueo quedó registrada.
	recs := fs.decisions()
	if len(recs) != 1 || recs[0].Action != domain.PolicyBlock || recs[0].PolicyID != "r1" {
		t.Fatalf("decisión de bloqueo inesperada: %+v", recs)
	}
	// Un request bloqueado no llega a auditarse como egress (nunca salió).
	if len(sink.records) != 0 {
		t.Fatalf("un bloqueo no debe dejar fila de egress_audit; hay %d", len(sink.records))
	}
}

func TestPolicyWarnEgressesAndMarks(t *testing.T) {
	allowLoopback(t)
	srv, hits := egressServer(t)

	fs := &fakeStore{pols: []domain.Policy{
		pol("r1", "marcar salesforce", domain.PolicyWarn, `{"api_family":"salesforce"}`),
	}}
	sink := &fakeSink{}
	rt := buildRT(fs, "salesforce", sink)

	resp, err := rt.HTTP.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	// warn deja salir: el servidor recibió el request y se auditó el egress.
	if got := hits.Load(); got != 1 {
		t.Fatalf("warn debe dejar salir; golpes al servidor = %d", got)
	}
	if len(sink.records) != 1 {
		t.Fatalf("warn debe auditarse como egress; filas = %d", len(sink.records))
	}
	// …y quedó marcado en policy_decisions.
	recs := fs.decisions()
	if len(recs) != 1 || recs[0].Action != domain.PolicyWarn {
		t.Fatalf("decisión de warn inesperada: %+v", recs)
	}
}

func TestPolicyAllowRuleEgressesWithoutRecord(t *testing.T) {
	allowLoopback(t)
	srv, hits := egressServer(t)

	// Un `allow` que casa por host (exacto) ANTES de un `block` por api_family: el
	// allow gana (excepción) y no se registra decisión.
	fs := &fakeStore{pols: []domain.Policy{
		pol("r1", "permitir loopback", domain.PolicyAllow, `{"host":"127.0.0.1"}`),
		pol("r2", "bloquear salesforce", domain.PolicyBlock, `{"api_family":"salesforce"}`),
	}}
	sink := &fakeSink{}
	rt := buildRT(fs, "salesforce", sink)

	resp, err := rt.HTTP.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if got := hits.Load(); got != 1 {
		t.Fatalf("allow debe dejar salir; golpes = %d", got)
	}
	if recs := fs.decisions(); len(recs) != 0 {
		t.Fatalf("un allow no registra decisión; hay %d", len(recs))
	}
}

func TestPolicyNoRuleEgressesAndCaches(t *testing.T) {
	allowLoopback(t)
	srv, hits := egressServer(t)

	fs := &fakeStore{} // sin reglas.
	sink := &fakeSink{}
	// Reusar el MISMO runtime/engine en dos requests: la segunda carga debe salir de
	// caché (una sola carga), no una query por request.
	eng := policy.NewEngine(fs, time.Minute, slog.Default())
	rt := policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "acme", InstanceID: "inst-1", APIFamily: "salesforce",
		Policy: eng, Audit: sink, Log: slog.Default(),
	})

	for i := 0; i < 2; i++ {
		resp, err := rt.HTTP.Get(srv.URL)
		if err != nil {
			t.Fatalf("GET %d: %v", i, err)
		}
		resp.Body.Close()
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("sin reglas ambos requests salen; golpes = %d", got)
	}
	if len(fs.decisions()) != 0 {
		t.Fatal("sin reglas no hay decisiones")
	}
	if n := fs.loadCount(); n != 1 {
		t.Fatalf("las reglas se cargan una vez y se cachean; cargas = %d", n)
	}

	// Invalidate fuerza recarga en el siguiente egress.
	eng.Invalidate("acme")
	resp, err := rt.HTTP.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET tras invalidar: %v", err)
	}
	resp.Body.Close()
	if n := fs.loadCount(); n != 2 {
		t.Fatalf("tras Invalidate se recarga; cargas = %d", n)
	}
}

func TestPolicyBodyContains(t *testing.T) {
	allowLoopback(t)

	rule := []domain.Policy{
		pol("r1", "bloquear IBAN en el cuerpo", domain.PolicyBlock, `{"body_contains":"IBAN"}`),
	}

	// (a) el cuerpo contiene el patrón ⇒ bloqueado, no sale.
	t.Run("match bloquea", func(t *testing.T) {
		srv, hits := egressServer(t)
		fs := &fakeStore{pols: rule}
		rt := buildRT(fs, "odoo", &fakeSink{})
		_, err := rt.HTTP.Post(srv.URL, "text/plain", strings.NewReader("pago a IBAN ES91..."))
		if !errors.Is(err, policy.ErrBlockedByPolicy) {
			t.Fatalf("error = %v, quiero ErrBlockedByPolicy", err)
		}
		if got := hits.Load(); got != 0 {
			t.Fatalf("el cuerpo casó el patrón: no debe salir; golpes = %d", got)
		}
	})

	// (b) el cuerpo NO contiene el patrón ⇒ sale normal.
	t.Run("sin match sale", func(t *testing.T) {
		srv, hits := egressServer(t)
		fs := &fakeStore{pols: rule}
		rt := buildRT(fs, "odoo", &fakeSink{})
		resp, err := rt.HTTP.Post(srv.URL, "text/plain", strings.NewReader("pago rutinario"))
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		resp.Body.Close()
		if got := hits.Load(); got != 1 {
			t.Fatalf("sin coincidencia debe salir; golpes = %d", got)
		}
		if len(fs.decisions()) != 0 {
			t.Fatal("sin coincidencia no hay decisión")
		}
	})
}

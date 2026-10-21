// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package powerbi_test

// Tests OFFLINE del EmbedBroker de Power BI (L4) contra un httptest que imita GET del
// informe + POST GenerateToken (sin red ni Power BI real). Egress por el Runtime real
// (policy.BuildRuntime → ssrfhttp + inyección del Bearer del service principal). El
// camino feliz relaja la barrera SSRF para loopback (allowLoopback, definido en
// powerbi_test.go) y afirma: mint server-side, TTL, Bearer inyectado y que el
// artefacto NO aparece en egress_audit (Authorization REDACTADO, cuerpos nunca
// auditados). El test de URL privada NO relaja la barrera y afirma ErrBlocked.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/powerbi"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

// captureAudit acumula las filas de egress_audit para inspeccionarlas (¿se filtró el
// token?). Satisface policy.AuditSink.
type captureAudit struct{ recs []domain.EgressAudit }

func (c *captureAudit) RecordEgress(_ context.Context, rec domain.EgressAudit) error {
	c.recs = append(c.recs, rec)
	return nil
}

// runtimeAudit es como runtime() (powerbi_test.go) pero con un AuditSink capturador.
func runtimeAudit(bearer string, audit policy.AuditSink) connector.Runtime {
	return policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "org-1", InstanceID: "inst-1", Bearer: bearer, Audit: audit, Log: slog.Default(),
	})
}

// pbiEmbedServer imita la API de embed: GET .../reports/{id} devuelve embedUrl +
// datasetId; POST .../reports/{id}/GenerateToken devuelve el token acotado. Captura
// el Authorization visto.
func pbiEmbedServer(t *testing.T, token string) (*httptest.Server, *string) {
	t.Helper()
	var lastAuth string
	handle := func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/GenerateToken") {
			_, _ = w.Write([]byte(`{"token":"` + token + `","tokenId":"tid-1","expiration":"2026-07-23T14:00:00Z"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"rep-1","name":"Ventas",` +
			`"embedUrl":"https://app.powerbi.com/reportEmbed?reportId=rep-1&groupId=grp-1","datasetId":"ds-1"}`))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/myorg/groups/", handle)  // informes con workspace
	mux.HandleFunc("/v1.0/myorg/reports/", handle) // «Mi área de trabajo»
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &lastAuth
}

// TestEmbedTokenMintsServerSide: mint app-owns-data server-side. Afirma token/tipo/
// caducidad, embedUrl canónica del informe, Bearer del service principal inyectado y
// que el artefacto NO se filtra a egress_audit (redactado).
func TestEmbedTokenMintsServerSide(t *testing.T) {
	allowLoopback(t)
	const secret = "super-secret-embed-token-xyz"
	srv, lastAuth := pbiEmbedServer(t, secret)
	audit := &captureAudit{}

	req := []byte(`{"report_id":"rep-1","group_id":"grp-1"}`)
	emb, err := powerbi.New().EmbedToken(t.Context(), instance(srv.URL, ""), runtimeAudit("sp-bearer", audit), req)
	if err != nil {
		t.Fatalf("EmbedToken: %v", err)
	}

	// Minted server-side: el token sale de GenerateToken, no del navegador.
	if emb.Token != secret {
		t.Fatalf("token = %q, quiero el minted server-side %q", emb.Token, secret)
	}
	if emb.TokenType != "Embed" {
		t.Fatalf("tokenType = %q, quiero Embed (app-owns-data)", emb.TokenType)
	}
	const wantURL = "https://app.powerbi.com/reportEmbed?reportId=rep-1&groupId=grp-1"
	if emb.URL != wantURL {
		t.Fatalf("embedUrl = %q, quiero la canónica del informe %q", emb.URL, wantURL)
	}
	// TTL corto: la caducidad del fixture (no un valor inventado por el cliente).
	if want := time.Date(2026, 7, 23, 14, 0, 0, 0, time.UTC); !emb.ExpiresAt.Equal(want) {
		t.Fatalf("expiry = %v, quiero %v", emb.ExpiresAt, want)
	}
	// Bearer del service principal inyectado por el Runtime (el driver nunca lo fija).
	if *lastAuth != "Bearer sp-bearer" {
		t.Fatalf("Authorization = %q, quiero Bearer sp-bearer", *lastAuth)
	}

	// Dos egress auditados (GET informe + POST GenerateToken): en NINGUNO aparece el
	// artefacto y el Authorization va REDACTADO (nunca el token en claro).
	if len(audit.recs) != 2 {
		t.Fatalf("egress auditado = %d filas, quiero 2 (GET + GenerateToken)", len(audit.recs))
	}
	for i, rec := range audit.recs {
		if rec.Auth != "REDACTED" {
			t.Fatalf("fila %d: Auth = %q, quiero REDACTED", i, rec.Auth)
		}
		blob, _ := json.Marshal(rec)
		if strings.Contains(string(blob), secret) {
			t.Fatalf("el embed token se filtró a egress_audit: %s", blob)
		}
	}
}

// TestEmbedTokenMyWorkspace: sin group_id, el informe va por «Mi área de trabajo».
func TestEmbedTokenMyWorkspace(t *testing.T) {
	allowLoopback(t)
	srv, _ := pbiEmbedServer(t, "tok")
	emb, err := powerbi.New().EmbedToken(t.Context(), instance(srv.URL, ""), runtime("sp"), []byte(`{"report_id":"rep-1"}`))
	if err != nil {
		t.Fatalf("EmbedToken sin group: %v", err)
	}
	if emb.Token == "" {
		t.Fatal("se esperaba un token minted")
	}
}

// TestEmbedTokenBadReq: sin report_id ⇒ ErrBadQuery (400), sin red.
func TestEmbedTokenBadReq(t *testing.T) {
	_, err := powerbi.New().EmbedToken(t.Context(), instance("https://api.powerbi.com", ""), runtime("tok"), []byte(`{}`))
	if !errors.Is(err, connector.ErrBadQuery) {
		t.Fatalf("err = %v, quiero connector.ErrBadQuery (falta report_id)", err)
	}
}

// TestEmbedTokenPrivateURLBlocked: SIN relajar la barrera, api_base apunta a loopback
// (privado) ⇒ el egress se bloquea antes de conectar con ssrfhttp.ErrBlocked.
func TestEmbedTokenPrivateURLBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embedUrl":"x"}`))
	}))
	defer srv.Close()

	_, err := powerbi.New().EmbedToken(t.Context(), instance(srv.URL, ""), runtime("tok"), []byte(`{"report_id":"rep-1","group_id":"grp-1"}`))
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("err = %v, quiero ssrfhttp.ErrBlocked (URL privada)", err)
	}
}

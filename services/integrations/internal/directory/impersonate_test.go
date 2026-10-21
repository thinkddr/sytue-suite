// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package directory_test

// Tests OFFLINE del minteo de tokens de IMPERSONACIÓN (canje RFC 8693): un Sytue
// ID simulado con httptest y el reloj INYECTADO — la renovación se comprueba
// moviendo el reloj, nunca durmiendo (un test que duerme 5 minutos no se ejecuta).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/integrations/internal/directory"
)

// impTTL es la vida del token de impersonación que anuncia Sytue ID
// (domain.ImpersonationTokenTTL). Corta a propósito: por eso hay que renovar.
const impTTL = 5 * time.Minute

// mockIDP es un Sytue ID mínimo: client_credentials para el token propio del
// client y token-exchange para el canje. Modela lo que de verdad importa aquí —
// que el client sea el DELEGABLE DE SU ORGANIZACIÓN y que el sujeto pedido sea
// miembro de ESA organización— y cuenta los canjes.
type mockIDP struct {
	// clients: client_id → secreto.
	clients map[string]string
	// members: client_id → usuarios a los que ese client puede impersonar.
	members   map[string][]string
	exchanges int
	minted    int
}

func (m *mockIDP) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		id, secret := r.FormValue("client_id"), r.FormValue("client_secret")
		if want, ok := m.clients[id]; !ok || want != secret {
			oauthError(w, "invalid_client", "autenticación de client fallida")
			return
		}
		switch r.FormValue("grant_type") {
		case "client_credentials":
			writeJSON(w, map[string]any{"access_token": "own:" + id, "expires_in": 3600})
		case "urn:ietf:params:oauth:grant-type:token-exchange":
			m.exchanges++
			// El subject_token del canje es el token PROPIO del client: es él quien
			// presenta su identidad (y no se encadena una impersonación previa).
			if got := r.FormValue("subject_token"); got != "own:"+id {
				oauthError(w, "invalid_grant", "subject_token inválido o expirado")
				return
			}
			subject := r.FormValue("requested_subject")
			if !contains(m.members[id], subject) {
				oauthError(w, "invalid_target", "el sujeto solicitado no es impersonable por este client")
				return
			}
			m.minted++
			// El token lleva el sujeto para que el destino pueda comprobar EN NOMBRE
			// DE QUIÉN se escribe, y un contador para distinguir una renovación.
			writeJSON(w, map[string]any{
				"access_token":      impToken(subject, m.minted),
				"issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
				"token_type":        "Bearer",
				"expires_in":        int(impTTL.Seconds()),
			})
		default:
			oauthError(w, "unsupported_grant_type", "grant_type no soportado")
		}
	}
}

func impToken(subject string, n int) string {
	return "imp:" + subject + ":" + strconv.Itoa(n)
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func oauthError(w http.ResponseWriter, code, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`{"error":"` + code + `","error_description":"` + desc + `"}`))
}

// newIDP levanta el IdP simulado con un client delegable por organización.
func newIDP(t *testing.T) (*mockIDP, string) {
	t.Helper()
	m := &mockIDP{
		clients: map[string]string{
			"sytue-migrate-acme":  "secreto-acme",
			"sytue-migrate-otros": "secreto-otros",
		},
		members: map[string][]string{
			"sytue-migrate-acme":  {"grace@acme.test", "bob@acme.test"},
			"sytue-migrate-otros": {"zoe@otros.test"},
		},
	}
	srv := httptest.NewServer(m.handler(t))
	t.Cleanup(srv.Close)
	return m, srv.URL
}

// clock es un reloj movible para inyectar en la delegación.
type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }

func newDelegation(base, clientID, secret, subject string, c *clock) *directory.Delegation {
	return directory.NewDelegation(directory.DelegationConfig{
		BaseURL:      base,
		ClientID:     clientID,
		ClientSecret: secret,
		Subject:      subject,
		Audience:     "sytue-drive",
		Now:          c.now,
	})
}

// TestCanjePorUsuarioObjetivo: cada persona tiene SU token, y el canje pide
// exactamente a esa persona. Es lo que hace que el contenido aterrice en la cuenta
// de cada cual y no en la del admin.
func TestCanjePorUsuarioObjetivo(t *testing.T) {
	m, base := newIDP(t)
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	ctx := context.Background()

	grace, err := newDelegation(base, "sytue-migrate-acme", "secreto-acme", "grace@acme.test", c).Token(ctx)
	if err != nil {
		t.Fatalf("canje de grace: %v", err)
	}
	bob, err := newDelegation(base, "sytue-migrate-acme", "secreto-acme", "bob@acme.test", c).Token(ctx)
	if err != nil {
		t.Fatalf("canje de bob: %v", err)
	}
	if !strings.HasPrefix(grace, "imp:grace@acme.test:") {
		t.Fatalf("el token de grace no es suyo: %q", grace)
	}
	if !strings.HasPrefix(bob, "imp:bob@acme.test:") {
		t.Fatalf("el token de bob no es suyo: %q", bob)
	}
	if grace == bob {
		t.Fatal("dos personas distintas recibieron el MISMO token")
	}
	if m.exchanges != 2 {
		t.Fatalf("canjes = %d; se esperaba uno por persona", m.exchanges)
	}
}

// TestTokenSeRenuevaAlCaducar es el fallo obvio de esta ola: el token de
// impersonación vive 5 minutos y una migración dura mucho más. Con el reloj
// inyectado se comprueba que se cachea mientras vale y se RENUEVA cuando le queda
// menos que el margen — sin dormir ni un milisegundo.
func TestTokenSeRenuevaAlCaducar(t *testing.T) {
	m, base := newIDP(t)
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	d := newDelegation(base, "sytue-migrate-acme", "secreto-acme", "grace@acme.test", c)
	ctx := context.Background()

	first, err := d.Token(ctx)
	if err != nil {
		t.Fatalf("primer canje: %v", err)
	}
	// Mientras el token vale, NO se vuelve a canjear (ni se martillea el audit_log).
	for i := 0; i < 3; i++ {
		c.add(time.Minute)
		got, err := d.Token(ctx)
		if err != nil {
			t.Fatalf("token a los %d min: %v", i+1, err)
		}
		if got != first {
			t.Fatalf("el token se renovó antes de tiempo (minuto %d)", i+1)
		}
	}
	if m.exchanges != 1 {
		t.Fatalf("canjes = %d antes de caducar; se esperaba 1", m.exchanges)
	}

	// A 4m30s entra el margen de seguridad: se renueva ANTES de que caduque, no
	// después de comerse un 401 a mitad de una subida.
	c.add(90 * time.Second)
	renewed, err := d.Token(ctx)
	if err != nil {
		t.Fatalf("renovación: %v", err)
	}
	if renewed == first {
		t.Fatal("el token NO se renovó al entrar en el margen: la migración moriría a los 5 minutos")
	}
	if !strings.HasPrefix(renewed, "imp:grace@acme.test:") {
		t.Fatalf("el token renovado no es del usuario objetivo: %q", renewed)
	}
	if m.exchanges != 2 {
		t.Fatalf("canjes = %d tras renovar; se esperaban 2", m.exchanges)
	}
}

// TestCanjeFallidoDaMotivoLegible: si una persona no es impersonable (baja, otra
// organización, no existe), el error tiene que explicarlo EN CASTELLANO y nombrar
// a quién afecta — acaba en el informe del trabajo, que lo lee un admin.
func TestCanjeFallidoDaMotivoLegible(t *testing.T) {
	_, base := newIDP(t)
	c := &clock{t: time.Unix(1_800_000_000, 0)}

	_, err := newDelegation(base, "sytue-migrate-acme", "secreto-acme", "fantasma@acme.test", c).Token(context.Background())
	if err == nil {
		t.Fatal("el canje de una persona no impersonable debería fallar")
	}
	msg := err.Error()
	if !strings.Contains(msg, "fantasma@acme.test") {
		t.Errorf("el motivo no dice a quién afecta: %q", msg)
	}
	if !strings.Contains(msg, "no es impersonable") {
		t.Errorf("el motivo no explica la causa que dio Sytue ID: %q", msg)
	}
}

// TestAislamientoPorOrganizacion: las credenciales delegables son POR
// ORGANIZACIÓN. El client de una org no puede actuar sobre una persona de otra —
// y esa es la razón de que no exista un client delegable global.
func TestAislamientoPorOrganizacion(t *testing.T) {
	_, base := newIDP(t)
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	ctx := context.Background()

	// El client de «otros» pidiendo una persona de «acme».
	if _, err := newDelegation(base, "sytue-migrate-otros", "secreto-otros", "grace@acme.test", c).Token(ctx); err == nil {
		t.Fatal("un client de otra organización pudo impersonar a grace")
	}
	// Y con el secreto equivocado no hay canje que valga.
	if _, err := newDelegation(base, "sytue-migrate-acme", "secreto-que-no-es", "grace@acme.test", c).Token(ctx); err == nil {
		t.Fatal("el canje prosperó con un secreto incorrecto")
	}
}

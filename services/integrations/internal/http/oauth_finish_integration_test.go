// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package integrationshttp

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
)

// terminar pide el canje como lo pide la consola: con la sesión de quien vuelve.
func (c *consentimiento) terminar(t *testing.T, sub, org, state, code string) oauthFinishResp {
	t.Helper()
	resp := c.pedir(t, http.MethodPost, "/oauth/finish", sub, org, oauthFinishReq{State: state, Code: code})
	var out oauthFinishResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("finish = %d (%v)", resp.StatusCode, err)
	}
	return out
}

// NEG-10. El canje se hace solo para la persona y la organización que sellaron el
// estado, una sola vez, y solo con el estado del último arranque.
func TestSoloQuienEmpezoElConsentimientoLoTermina(t *testing.T) {
	c := montarConsentimiento(t, func(h *Handlers, r chi.Router) {
		r.Post("/oauth/finish", h.OAuthFinish)
	})
	antes := len(canjesDelProveedor())
	state := c.arrancar(t, "ana")

	rechazos := map[string]struct{ sub, org, state string }{
		// Bea administra la misma organización, pero no empezó este consentimiento.
		"otra persona": {"bea", "acme", state},
		// El mismo sujeto en otra organización (su cuenta personal, por ejemplo).
		"otra organización": {"ana", "otra", state},
		"sin estado":        {"ana", "acme", ""},
	}
	for nombre, caso := range rechazos {
		if out := c.terminar(t, caso.sub, caso.org, caso.state, "codigo-de-"+caso.sub); out.Error != "state" {
			t.Errorf("%s: desenlace %+v, quiero error «state»", nombre, out)
		}
	}
	// Un estado caducado, sellado con la misma clave para la misma persona.
	caducado, err := consent.Begin(c.cif, consent.Flow{AuthorizeURL: "https://login.proveedor.example/authorize", ClientID: "app-de-acme"},
		"acme", "ana", c.inst, "https://integraciones.ejemplo/api/v1/oauth/callback", "", time.Now().Add(-11*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if out := c.terminar(t, "ana", "acme", stateDe(t, caducado.AuthorizeURL), "codigo-de-ana"); out.Error != "state" {
		t.Errorf("estado caducado: desenlace %+v, quiero error «state»", out)
	}
	if tok, n := c.token(t), len(canjesDelProveedor())-antes; tok != "" || n != 0 {
		t.Fatalf("tras los rechazos hay token %q y %d canjes; no tiene que haber ninguno", tok, n)
	}

	// Ana, que lo empezó, sí lo termina (los intentos ajenos no le gastan el estado).
	out := c.terminar(t, "ana", "acme", state, "codigo-de-ana")
	if out.Error != "" || out.Instance != c.inst || out.Section != "apps" {
		t.Fatalf("Ana: desenlace %+v, quiero conectada", out)
	}
	if tok := c.token(t); tok != "token-de-codigo-de-ana" {
		t.Fatalf("Ana: token guardado %q", tok)
	}
	canjes := canjesDelProveedor()[antes:]
	if len(canjes) != 1 || canjes[0].Get("code") != "codigo-de-ana" || canjes[0].Get("code_verifier") == "" {
		t.Fatalf("canjes en el proveedor: %v", canjes)
	}

	// El mismo estado otra vez: ya se usó. No se canjea ni se toca el token.
	if out := c.terminar(t, "ana", "acme", state, "otro-codigo"); out.Error != "state" {
		t.Errorf("estado repetido: desenlace %+v, quiero error «state»", out)
	}
	// Dos arranques: solo vale el último.
	viejo, nuevo := c.arrancar(t, "ana"), c.arrancar(t, "ana")
	if out := c.terminar(t, "ana", "acme", viejo, "codigo-viejo"); out.Error != "state" {
		t.Errorf("estado de un arranque anterior: desenlace %+v, quiero error «state»", out)
	}
	if n := len(canjesDelProveedor()) - antes; n != 1 {
		t.Errorf("hubo %d canjes; el repetido y el viejo no tienen que llegar al proveedor", n)
	}
	if out := c.terminar(t, "ana", "acme", nuevo, "codigo-nuevo"); out.Error != "" || c.token(t) != "token-de-codigo-nuevo" {
		t.Errorf("el último arranque: desenlace %+v, token %q", out, c.token(t))
	}
}

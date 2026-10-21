// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/config"
	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
)

func TestWhatsAppEmbeddedSignupCanjeaElCodigoComoDocumentaMeta(t *testing.T) {
	rt := roomsTestRuntime(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Host != "graph.facebook.com" || r.URL.Path != "/v26.0/oauth/access_token" || q.Get("client_id") != "app" || q.Get("client_secret") != "secreto-app" || q.Get("code") != "codigo-meta" || len(q) != 3 {
			t.Fatalf("protocolo Meta incorrecto: %s %s", r.Method, r.URL.Path)
		}
		return roomsTestResponse(http.StatusOK, `{"access_token":"token-meta","expires_in":3600}`), nil
	})
	tok, err := exchangeWhatsAppCode(context.Background(), rt, whatsAppGraphOrigin+"/v26.0", config.OAuthClient{ClientID: "app", ClientSecret: "secreto-app"}, "codigo-meta")
	if err != nil || tok.AccessToken != "token-meta" {
		t.Fatalf("no canjeó el código: %v", err)
	}
}

func TestWhatsAppNuncaAceptaUnGrantDeOtraAppOSinMensajeria(t *testing.T) {
	const good = `{"app_id":"app","is_valid":true,"expires_at":0,"scopes":["whatsapp_business_management","whatsapp_business_messaging"],"granular_scopes":[{"scope":"whatsapp_business_management","target_ids":["123","123","https://evil.example"]}]}`
	var grant whatsAppDebugGrant
	if err := json.Unmarshal([]byte(good), &grant); err != nil {
		t.Fatal(err)
	}
	accounts, expires, err := validateWhatsAppGrant(grant, "app")
	if err != nil || len(accounts) != 1 || accounts[0] != "123" || !expires.IsZero() {
		t.Fatalf("grant válido: %v %v", accounts, err)
	}
	for _, tc := range []struct {
		name string
		edit func(*whatsAppDebugGrant)
	}{
		{"otra app", func(g *whatsAppDebugGrant) { g.AppID = "otra" }},
		{"revocado", func(g *whatsAppDebugGrant) { g.Valid = false }},
		{"sin mensajes", func(g *whatsAppDebugGrant) { g.Scopes = []string{"whatsapp_business_management"} }},
		{"caducado", func(g *whatsAppDebugGrant) { g.Expires = time.Now().Add(-time.Minute).Unix() }},
		{"sin cuentas", func(g *whatsAppDebugGrant) { g.Grants = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var candidate whatsAppDebugGrant
			_ = json.Unmarshal([]byte(good), &candidate)
			tc.edit(&candidate)
			if _, _, err := validateWhatsAppGrant(candidate, "app"); err == nil {
				t.Fatal("se aceptó grant no autorizado")
			}
		})
	}
}

func TestWhatsAppListaNumerosSoloDentroDeLosWABAAutorizados(t *testing.T) {
	calls := 0
	rt := roomsTestRuntime(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "graph.facebook.com" || r.URL.Path != "/v26.0/123/phone_numbers" {
			t.Fatalf("salió del WABA concedido: %s", r.URL.Path)
		}
		if calls == 1 {
			return roomsTestResponse(http.StatusOK, `{"data":[{"id":"111","display_phone_number":"+34111","verified_name":"Empresa","status":"CONNECTED"}],"paging":{"cursors":{"after":"pagina2"},"next":"https://evil.example/robar-token"}}`), nil
		}
		if r.URL.Query().Get("after") != "pagina2" {
			t.Fatal("perdió el cursor de Meta")
		}
		return roomsTestResponse(http.StatusOK, `{"data":[{"id":"222","display_phone_number":"+34222","verified_name":"Empresa2","status":"PENDING"},{"id":"333","status":"PENDING"}]}`), nil
	})
	numbers, err := listWhatsAppNumbers(context.Background(), rt, whatsAppGraphOrigin+"/v26.0", "123", []string{"333"})
	if err != nil || len(numbers) != 3 || calls != 2 {
		t.Fatalf("lista incompleta: %+v %v", numbers, err)
	}
	if numbers[0].RegistrationRequired || !numbers[1].RegistrationRequired || numbers[2].RegistrationRequired {
		t.Fatalf("registro y avance perdido: %+v", numbers)
	}
	public, _ := json.Marshal(numbers)
	if strings.Contains(string(public), "123") || strings.Contains(string(public), "token") {
		t.Fatalf("filtró metadatos privados: %s", public)
	}
}

func TestWhatsAppCallbackSoloPuedeSerElHandlerFirmadoDeRooms(t *testing.T) {
	const nonce = "abcdefghijklmnopqrstuvwxyz0123456789"
	const origin = "https://rooms.example.eu"
	callback := origin + "/webhooks/whatsapp/" + nonce
	if !validWhatsAppCallback(origin, callback, nonce) {
		t.Fatal("se rechazó el callback fijo")
	}
	for _, target := range []string{"https://evil.example/webhooks/whatsapp/" + nonce, "http://rooms.example.eu/webhooks/whatsapp/" + nonce, callback + "?token=otro", origin + "/otra/" + nonce, "https://user:pass@rooms.example.eu/webhooks/whatsapp/" + nonce} {
		if validWhatsAppCallback(origin, target, nonce) {
			t.Fatalf("se aceptó un callback arbitrario: %s", target)
		}
	}
}

func TestWhatsAppConsentimientoSoloPuedeTerminarLaMismaSesion(t *testing.T) {
	c, err := keycrypto.New(make([]byte, keycrypto.KeyLen))
	if err != nil {
		t.Fatal(err)
	}
	state, _, err := consent.BeginEmbedded(c, "acme", "ana", "instancia", whatsAppStateSection, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	h := &Handlers{oauthCipher: c}
	// Store nil deliberado: ningún estado ajeno puede llegar a tocarlo o hacer egress.
	for _, tc := range []struct{ sub, tenant, state string }{{"bea", "acme", state}, {"ana", "otra", state}, {"ana", "acme", "manipulado"}} {
		ctx := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: tc.tenant}), authn.Claims{Subject: tc.sub, Tenant: tc.tenant})
		if _, err := h.consumeWhatsAppState(ctx, whatsAppFinishRequest{State: tc.state, Code: "codigo-meta"}); err == nil {
			t.Fatal("se aceptó una sesión ajena")
		}
	}
}

func TestWhatsAppSinConfiguracionNoCreaUnaInstancia(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest(http.MethodPost, "/whatsapp/signup/start", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.WhatsAppSignupStart(w, r)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "whatsapp_not_configured") {
		t.Fatalf("debe informar indisponibilidad: %d %s", w.Code, w.Body)
	}
	info := httptest.NewRecorder()
	h.WhatsAppSignupInfo(info, httptest.NewRequest(http.MethodGet, "/whatsapp/signup", http.NoBody))
	if info.Code != http.StatusOK || !strings.Contains(info.Body.String(), `"available":false`) {
		t.Fatalf("inventó una conexión disponible: %d %s", info.Code, info.Body)
	}
}

func TestWhatsAppSuscribeYConfiguraElCallbackPorNumero(t *testing.T) {
	const callback = "https://rooms.example.eu/webhooks/whatsapp/abcdefghijklmnopqrstuvwxyz0123456789"
	calls := 0
	rt := roomsTestRuntime(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodPost || r.URL.Host != "graph.facebook.com" {
			t.Fatalf("destino incorrecto: %s %s", r.Method, r.URL.Host)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if calls == 1 {
			if r.URL.Path != "/v26.0/123/subscribed_apps" || len(body) != 0 {
				t.Fatal("no suscribió la app al WABA primero")
			}
		} else {
			hook, ok := body["webhook_configuration"].(map[string]any)
			if r.URL.Path != "/v26.0/111" || !ok || hook["override_callback_uri"] != callback || hook["verify_token"] != "abcdefghijklmnopqrstuvwxyz0123456789" {
				t.Fatalf("no configuró override oficial por número: %v", body)
			}
		}
		return roomsTestResponse(http.StatusOK, `{"success":true}`), nil
	})
	if err := configureWhatsAppWebhooks(context.Background(), rt, whatsAppGraphOrigin+"/v26.0", whatsAppNumber{ID: "111", waba: "123"}, callback, "abcdefghijklmnopqrstuvwxyz0123456789"); err != nil || calls != 2 {
		t.Fatalf("no activó webhooks: calls=%d err=%v", calls, err)
	}
	calls = 0
	rejected := roomsTestRuntime(func(*http.Request) (*http.Response, error) {
		calls++
		return roomsTestResponse(http.StatusOK, `{"success":false}`), nil
	})
	if err := configureWhatsAppWebhooks(context.Background(), rejected, whatsAppGraphOrigin+"/v26.0", whatsAppNumber{ID: "111", waba: "123"}, callback, "abcdefghijklmnopqrstuvwxyz0123456789"); err == nil || calls != 1 {
		t.Fatalf("el fallo de suscripción no puede activar el número: calls=%d err=%v", calls, err)
	}
}

func TestWhatsAppNoRevelaCodigoNiSecretoSiMetaRechazaElCanje(t *testing.T) {
	rt := roomsTestRuntime(func(*http.Request) (*http.Response, error) {
		return roomsTestResponse(http.StatusBadRequest, `{"error":"codigo-meta secreto-app"}`), nil
	})
	_, err := exchangeWhatsAppCode(context.Background(), rt, whatsAppGraphOrigin+"/v26.0", config.OAuthClient{ClientID: "app", ClientSecret: "secreto-app"}, "codigo-meta")
	if err == nil || strings.Contains(err.Error(), "codigo-meta") || strings.Contains(err.Error(), "secreto-app") {
		t.Fatalf("el error expuso credenciales: %v", err)
	}
}

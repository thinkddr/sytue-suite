// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// hookProbe monta el handler del webhook entrante con la publicación interceptada
// (el servicio real exige BD) y devuelve el texto publicado.
func hookProbe() (*intHandlers, *string) {
	var got string
	ih := &intHandlers{postIncomingFn: func(_ context.Context, _, text string) error {
		got = text
		return nil
	}}
	return ih, &got
}

func doHook(t *testing.T, ih *intHandlers, h http.HandlerFunc, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := withToken(httptest.NewRequest(http.MethodPost, "/hooks/tok", strings.NewReader(body)), "tok")
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h(rec, r)
	return rec
}

// TestPostIncomingDialects: el MISMO endpoint acepta el formato nativo, el de Slack
// y el de Teams, y responde a cada cliente el ack que espera ver.
func TestPostIncomingDialects(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		body        string
		wantText    string
		wantBody    string
	}{
		{
			name:     "nativo intacto",
			body:     `{"text":"hola mundo"}`,
			wantText: "hola mundo",
			wantBody: `{"ok":true}`,
		},
		{
			name:     "slack attachments",
			body:     `{"text":"Aviso","attachments":[{"title":"Build 42","title_link":"https://ci/42","text":"verde","fields":[{"title":"Entorno","value":"prod"}]}]}`,
			wantText: "Aviso\n\n**Build 42** (https://ci/42)\nverde\n**Entorno:** prod",
			wantBody: "ok",
		},
		{
			name:     "slack blocks",
			body:     `{"blocks":[{"type":"header","text":{"type":"plain_text","text":"Incidencia"}},{"type":"section","text":{"type":"mrkdwn","text":"*Sev1* en <https://x.io|API>"}}]}`,
			wantText: "## Incidencia\n**Sev1** en API (https://x.io)",
			wantBody: "ok",
		},
		{
			name:        "slack form-urlencoded",
			contentType: "application/x-www-form-urlencoded",
			body:        "payload=" + url.QueryEscape(`{"text":"desde *form*"}`),
			wantText:    "desde **form**",
			wantBody:    "ok",
		},
		{
			name:     "teams MessageCard",
			body:     `{"@type":"MessageCard","themeColor":"0076D7","summary":"s","title":"CPU alta","text":"nodo 3","sections":[{"facts":[{"name":"Valor","value":"93%"}]}]}`,
			wantText: "## CPU alta\nnodo 3\n**Valor:** 93%",
			wantBody: "1",
		},
		{
			name:     "teams Adaptive Card",
			body:     `{"type":"message","attachments":[{"contentType":"application/vnd.microsoft.card.adaptive","content":{"type":"AdaptiveCard","body":[{"type":"TextBlock","weight":"Bolder","text":"Pedido 991"},{"type":"FactSet","facts":[{"title":"Cliente","value":"ACME"}]}]}}]}`,
			wantText: "**Pedido 991**\n**Cliente:** ACME",
			wantBody: "1",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ih, got := hookProbe()
			ct := c.contentType
			if ct == "" {
				ct = "application/json"
			}
			rec := doHook(t, ih, ih.PostIncoming, ct, c.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("quiero 200, got %d (%s)", rec.Code, rec.Body.String())
			}
			if *got != c.wantText {
				t.Fatalf("texto publicado =\n%q\nquiero\n%q", *got, c.wantText)
			}
			if body := strings.TrimSpace(rec.Body.String()); body != c.wantBody {
				t.Fatalf("cuerpo de respuesta = %q, quiero %q", body, c.wantBody)
			}
		})
	}
}

// TestPostIncomingAliasesForceDialect: los alias fuerzan el dialecto aunque el
// cuerpo sea ambiguo ({"text":…} a secas se trataría como nativo).
func TestPostIncomingAliasesForceDialect(t *testing.T) {
	ih, got := hookProbe()
	rec := doHook(t, ih, ih.PostIncomingSlack, "application/json", `{"text":"esto es *importante*"}`)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Fatalf("alias slack: %d %q", rec.Code, rec.Body.String())
	}
	if *got != "esto es **importante**" {
		t.Fatalf("alias slack debe traducir mrkdwn, got %q", *got)
	}

	ih, got = hookProbe()
	rec = doHook(t, ih, ih.PostIncomingTeams, "application/json", `{"text":"linea1<br>linea2"}`)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "1" {
		t.Fatalf("alias teams: %d %q", rec.Code, rec.Body.String())
	}
	if *got != "linea1\nlinea2" {
		t.Fatalf("alias teams debe normalizar <br>, got %q", *got)
	}
}

// TestPostIncomingIgnoresSlackChannel es la prueba de SEGURIDAD: el token fija el
// canal; el `channel` (y la identidad) del payload de Slack no cambian nada.
func TestPostIncomingIgnoresSlackChannel(t *testing.T) {
	var seenToken string
	var seenText string
	ih := &intHandlers{postIncomingFn: func(_ context.Context, token, text string) error {
		seenToken, seenText = token, text
		return nil
	}}
	rec := doHook(t, ih, ih.PostIncoming, "application/json",
		`{"channel":"#privado-direccion","username":"suplantador","icon_emoji":":x:","text":"hola"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("quiero 200, got %d", rec.Code)
	}
	if seenToken != "tok" {
		t.Fatalf("el destino lo fija el token de la ruta, got %q", seenToken)
	}
	if seenText != "hola" {
		t.Fatalf("channel/username no deben influir en el mensaje, got %q", seenText)
	}
}

// TestPostIncomingDegradesNever500: cuerpos raros ⇒ 4xx, jamás 500 ni pánico.
func TestPostIncomingDegradesNever500(t *testing.T) {
	cases := []struct{ name, contentType, body string }{
		{"json roto", "application/json", `{roto`},
		{"vacío", "application/json", ``},
		{"campo desconocido (nativo estricto, como antes)", "application/json", `{"text":"x","desconocido":1}`},
		{"slack sin nada legible", "application/json", `{"attachments":[]}`},
		{"teams sin nada legible", "application/json", `{"@type":"MessageCard"}`},
		{"form sin payload", "application/x-www-form-urlencoded", `foo=bar`},
		{"array", "application/json", `[1,2,3]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ih, _ := hookProbe()
			rec := doHook(t, ih, ih.PostIncoming, c.contentType, c.body)
			if rec.Code < 400 || rec.Code >= 500 {
				t.Fatalf("quiero 4xx, got %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestPostIncomingRejectsHugeBody: el cuerpo se acota ANTES de mirarlo.
func TestPostIncomingRejectsHugeBody(t *testing.T) {
	ih, _ := hookProbe()
	huge := `{"text":"` + strings.Repeat("a", maxHookBodyBytes+1024) + `"}`
	rec := doHook(t, ih, ih.PostIncoming, "application/json", huge)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("quiero 400 por cuerpo enorme, got %d", rec.Code)
	}
}

// TestNewRouterMountsHookAliases: los alias conviven con /hooks/{token} sin
// conflicto de rutas en chi (segmentos distintos).
func TestNewRouterMountsHookAliases(t *testing.T) {
	r := NewRouter(Deps{})
	for _, path := range []string{"/hooks/slack/abc", "/hooks/teams/abc", "/hooks/abc"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, http.NoBody) // GET ⇒ 405 si la ruta existe
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: quiero 405 (ruta montada solo para POST), got %d", path, rec.Code)
		}
	}
}

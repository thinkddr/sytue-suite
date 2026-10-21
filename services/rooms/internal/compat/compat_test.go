// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package compat

import (
	"net/url"
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		body        string
		want        string
	}{
		{"nativo text", "application/json", `{"text":"hola"}`, Native},
		{"nativo vacío", "application/json", `{}`, Native},
		{"json roto", "application/json", `{nope`, Native}, // que falle como siempre
		{"slack attachments", "application/json", `{"text":"x","attachments":[{"text":"y"}]}`, Slack},
		{"slack blocks", "application/json", `{"blocks":[{"type":"divider"}]}`, Slack},
		{"slack channel/username", "application/json", `{"channel":"#ops","username":"bot","text":"x"}`, Slack},
		{"slack form", "application/x-www-form-urlencoded", `payload=%7B%22text%22%3A%22x%22%7D`, Slack},
		{"slack form con charset", "application/x-www-form-urlencoded; charset=utf-8", `payload=%7B%7D`, Slack},
		{"teams messagecard", "application/json", `{"@type":"MessageCard","text":"x"}`, Teams},
		{"teams themeColor", "application/json", `{"themeColor":"0076D7","text":"x"}`, Teams},
		{"teams sections", "application/json", `{"summary":"s","sections":[{"text":"x"}]}`, Teams},
		{"teams adaptive wrapper", "application/json", `{"type":"message","attachments":[{"contentType":"application/vnd.microsoft.card.adaptive","content":{}}]}`, Teams},
		{"teams adaptive pelada", "application/json", `{"type":"AdaptiveCard","body":[]}`, Teams},
		{"google chat cardsV2", "application/json", `{"cardsV2":[{"cardId":"x","card":{"header":{"title":"t"}}}]}`, GoogleChat},
		{"google chat cards heredado", "application/json", `{"text":"x","cards":[{"sections":[]}]}`, GoogleChat},
		{"google chat solo texto es nativo", "application/json", `{"text":"*hola*"}`, Native},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Detect(c.contentType, []byte(c.body)); got != c.want {
				t.Fatalf("Detect = %q, quiero %q", got, c.want)
			}
		})
	}
}

func TestTranslateGoogleChat(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{
			name: "texto con formato de Chat",
			body: `{"text":"Despliegue *ok* en <https://ci.sytue.com/42|build 42>"}`,
			want: "Despliegue **ok** en build 42 (https://ci.sytue.com/42)",
		},
		{
			name: "cardsV2 con cabecera, párrafo, decorado y clave-valor",
			body: `{"cardsV2":[{"cardId":"c","card":{"header":{"title":"Incidencia","subtitle":"Sev1"},
			       "sections":[{"header":"Detalle","widgets":[{"textParagraph":{"text":"API <b>caída</b><br>desde las 10:00"}},
			       {"decoratedText":{"topLabel":"Entorno","text":"prod","bottomLabel":"eu-west"}},
			       {"keyValue":{"topLabel":"Duración","content":"12m"}},{"buttonList":{"buttons":[]}}]}]}}]}`,
			want: "**Incidencia**\nSev1\n**Detalle**\nAPI **caída**\ndesde las 10:00\n**Entorno:** prod\n_eu-west_\n**Duración:** 12m",
		},
		{
			name: "texto y tarjeta heredada",
			body: `{"text":"aviso","cards":[{"header":{"title":"Build"}}]}`,
			want: "aviso\n\n**Build**",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Translate(GoogleChat, "application/json", []byte(c.body))
			if err != nil {
				t.Fatalf("Translate: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %q\nquiero %q", got, c.want)
			}
		})
	}
	if _, err := Translate(GoogleChat, "application/json", []byte(`{"cardsV2":[]}`)); err == nil {
		t.Fatal("una tarjeta sin nada legible debe ser ErrUnsupported")
	}
	if ct, b := Ack(GoogleChat); ct != "application/json" || b != "{}" {
		t.Fatalf("Google Chat espera JSON: %q %q", ct, b)
	}
}

func TestAck(t *testing.T) {
	if _, b := Ack(Slack); b != "ok" {
		t.Fatalf("Slack espera el cuerpo literal «ok», got %q", b)
	}
	if _, b := Ack(Teams); b != "1" {
		t.Fatalf("Teams espera el cuerpo literal «1», got %q", b)
	}
	if _, b := Ack(Native); b != "" {
		t.Fatalf("el nativo no lleva ack de texto, got %q", b)
	}
}

func TestNormalizeOutgoing(t *testing.T) {
	for _, c := range []struct {
		in   string
		want string
		ok   bool
	}{
		{"", Native, true}, // por defecto: comportamiento histórico
		{"sytue", Native, true},
		{"slack", Slack, true},
		{" SLACK ", Slack, true},
		{"teams", Teams, true},
		{"basura", "", false},
	} {
		got, ok := NormalizeOutgoing(c.in)
		if got != c.want || ok != c.ok {
			t.Fatalf("NormalizeOutgoing(%q) = (%q,%v), quiero (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestMrkdwn(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"enlace con texto", "mira <https://sytue.com|la web>", "mira la web (https://sytue.com)"},
		{"enlace pelado", "ve a <https://sytue.com>", "ve a https://sytue.com"},
		{"mención usuario", "hola <@U123|ana>", "hola @ana"},
		{"mención sin nombre", "hola <@U123>", "hola @U123"},
		{"canal", "en <#C1|general>", "en #general"},
		{"here", "<!here> atentos", "@here atentos"},
		{"mailto", "escribe a <mailto:a@b.com|a@b.com>", "escribe a a@b.com"},
		{"negrita", "esto es *importante*", "esto es **importante**"},
		{"negrita ya doble", "esto es **importante**", "esto es **importante**"},
		{"cursiva sin cambio", "esto es _sutil_", "esto es _sutil_"},
		{"tachado", "esto ~ya no~ vale", "esto ~~ya no~~ vale"},
		{"código intacto", "usa `a*b*c` ahora", "usa `a*b*c` ahora"},
		{"bloque intacto", "```\n*x* <a|b>\n```", "```\n*x* <a|b>\n```"},
		{"multiplicación no es negrita", "2 * 3 * 4", "2 * 3 * 4"},
		{"entidades", "A &amp; B &lt;ok&gt;", "A & B <ok>"},
		{"sin marcas", "texto llano", "texto llano"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Mrkdwn(c.in); got != c.want {
				t.Fatalf("Mrkdwn(%q) = %q, quiero %q", c.in, got, c.want)
			}
		})
	}
}

func TestTranslateSlack(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		body        string
		want        string
	}{
		{
			name: "attachments completo",
			body: `{"text":"Despliegue *ok*","attachments":[{"color":"#36a64f","pretext":"aviso","title":"Build 42",
			       "title_link":"https://ci.sytue.com/42","text":"Todo verde","fields":[{"title":"Entorno","value":"prod","short":true},
			       {"title":"Duración","value":"2m","short":true}],"footer":"CI"}]}`,
			want: "Despliegue **ok**\n\naviso\n**Build 42** (https://ci.sytue.com/42)\nTodo verde\n**Entorno:** prod\n**Duración:** 2m\n_CI_",
		},
		{
			name: "blocks",
			body: `{"blocks":[{"type":"header","text":{"type":"plain_text","text":"Incidencia"}},
			       {"type":"section","text":{"type":"mrkdwn","text":"*Sev1* en <https://x.io|API>"}},
			       {"type":"divider"},
			       {"type":"context","elements":[{"type":"mrkdwn","text":"hace 2m"},{"type":"plain_text","text":"guardia: ana"}]}]}`,
			want: "## Incidencia\n**Sev1** en API (https://x.io)\n---\n_hace 2m · guardia: ana_",
		},
		{
			name: "section con fields",
			body: `{"blocks":[{"type":"section","fields":[{"type":"mrkdwn","text":"*A*"},{"type":"mrkdwn","text":"*B*"}]}]}`,
			want: "**A**\n**B**",
		},
		{
			name: "bloque no soportado degrada",
			body: `{"text":"hola","blocks":[{"type":"image","image_url":"https://x/y.png","alt_text":"ignorada"},{"type":"actions","text":{"type":"plain_text","text":"pulsa"}}]}`,
			want: "hola\npulsa",
		},
		{
			name: "solo fallback",
			body: `{"attachments":[{"fallback":"resumen plano"}]}`,
			want: "resumen plano",
		},
		{
			name:        "form payload",
			contentType: "application/x-www-form-urlencoded",
			body:        "payload=" + url.QueryEscape(`{"text":"desde *form*"}`),
			want:        "desde **form**",
		},
		{
			name:        "form text (comando de barra)",
			contentType: "application/x-www-form-urlencoded",
			body:        "token=abc&text=" + url.QueryEscape("hola ~mundo~"),
			want:        "hola ~~mundo~~",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ct := c.contentType
			if ct == "" {
				ct = "application/json"
			}
			got, err := Translate(Slack, ct, []byte(c.body))
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if got != c.want {
				t.Fatalf("Translate =\n%q\nquiero\n%q", got, c.want)
			}
		})
	}
}

// TestTranslateSlackIgnoresChannel es la prueba de SEGURIDAD del entrante: el campo
// `channel` del payload no puede redirigir el mensaje — el canal lo fija el webhook,
// así que ni se lee ni aparece en el texto.
func TestTranslateSlackIgnoresChannel(t *testing.T) {
	got, err := Translate(Slack, "application/json", []byte(`{"channel":"#privado-direccion","text":"hola","username":"suplantador","icon_emoji":":x:"}`))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if got != "hola" {
		t.Fatalf("el payload solo debe aportar el texto, got %q", got)
	}
	if strings.Contains(got, "privado") || strings.Contains(got, "suplantador") {
		t.Fatalf("channel/username no deben influir en el mensaje: %q", got)
	}
}

func TestTranslateTeams(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{
			name: "MessageCard",
			body: `{"@type":"MessageCard","@context":"https://schema.org/extensions","themeColor":"0076D7",
			        "summary":"Alerta","title":"CPU alta","text":"El nodo 3 supera el 90%<br>desde las 10:00",
			        "sections":[{"activityTitle":"monitor","activitySubtitle":"prod","facts":[{"name":"Nodo","value":"3"},
			        {"name":"Valor","value":"93%"}],"text":"Ver [panel](https://grafana.sytue.com/d/1)"}]}`,
			want: "## CPU alta\nEl nodo 3 supera el 90%\ndesde las 10:00\n**monitor**\n_prod_\nVer panel (https://grafana.sytue.com/d/1)\n**Nodo:** 3\n**Valor:** 93%",
		},
		{
			name: "Adaptive Card",
			body: `{"type":"message","attachments":[{"contentType":"application/vnd.microsoft.card.adaptive",
			        "content":{"type":"AdaptiveCard","body":[{"type":"TextBlock","size":"Large","weight":"Bolder","text":"Pedido 991"},
			        {"type":"TextBlock","text":"Enviado hoy"},{"type":"FactSet","facts":[{"title":"Cliente","value":"ACME"}]},
			        {"type":"Container","items":[{"type":"TextBlock","text":"anidado"}]},
			        {"type":"Image","url":"https://x/y.png"}]}}]}`,
			want: "**Pedido 991**\nEnviado hoy\n**Cliente:** ACME\nanidado",
		},
		{
			name: "solo summary degrada",
			body: `{"@type":"MessageCard","summary":"Nada más que contar"}`,
			want: "Nada más que contar",
		},
		{
			name: "ColumnSet",
			body: `{"type":"AdaptiveCard","body":[{"type":"ColumnSet","columns":[{"items":[{"type":"TextBlock","text":"izq"}]},{"items":[{"type":"TextBlock","text":"der"}]}]}]}`,
			want: "izq\nder",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Translate(Teams, "application/json", []byte(c.body))
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if got != c.want {
				t.Fatalf("Translate =\n%q\nquiero\n%q", got, c.want)
			}
		})
	}
}

// TestTranslateBasuraDegrada: nada de pánico ni 500 — o texto legible, o ErrUnsupported
// (que el handler convierte en 400).
func TestTranslateBasuraDegrada(t *testing.T) {
	cases := []struct{ dialect, contentType, body string }{
		{Slack, "application/json", `{no es json`},
		{Slack, "application/json", `{}`},
		{Slack, "application/json", `{"attachments":[]}`},
		{Slack, "application/json", `{"blocks":[{"type":"actions"}]}`},
		{Slack, "application/x-www-form-urlencoded", `%zz`},
		{Slack, "application/x-www-form-urlencoded", `otra=cosa`},
		{Slack, "application/json", `[]`},
		{Teams, "application/json", `{"@type":"MessageCard"}`},
		{Teams, "application/json", `nada`},
		{Teams, "application/json", `{"sections":[{}]}`},
		{"desconocido", "application/json", `{"text":"x"}`},
	}
	for _, c := range cases {
		got, err := Translate(c.dialect, c.contentType, []byte(c.body))
		if err == nil && strings.TrimSpace(got) == "" {
			t.Fatalf("%s/%q: sin error pero texto vacío", c.dialect, c.body)
		}
	}
}

func TestSlackOutgoingForm(t *testing.T) {
	form := SlackOutgoing{
		Token: "tok", TeamDomain: "acme", ChannelID: "c-1", ChannelName: "c-1",
		UserID: "u-1", UserName: "Ana", Text: "deploy ya", TriggerWord: "deploy", Timestamp: 1700000000,
	}.Form()
	vals, err := url.ParseQuery(form)
	if err != nil {
		t.Fatalf("form inválido: %v", err)
	}
	for k, want := range map[string]string{
		"token": "tok", "team_domain": "acme", "channel_id": "c-1", "channel_name": "c-1",
		"user_id": "u-1", "user_name": "Ana", "text": "deploy ya", "trigger_word": "deploy",
		"timestamp": "1700000000",
	} {
		if got := vals.Get(k); got != want {
			t.Fatalf("%s = %q, quiero %q", k, got, want)
		}
	}
}

func TestParseSlackReply(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"in_channel", `{"response_type":"in_channel","text":"hecho *ya*"}`, "hecho **ya**"},
		{"ephemeral se publica igual", `{"response_type":"ephemeral","text":"solo para ti"}`, "solo para ti"},
		{"con attachments", `{"text":"resultado","attachments":[{"title":"detalle"}]}`, "resultado\n\n**detalle**"},
		{"sin texto", `{"response_type":"in_channel"}`, ""},
		{"basura", `no json`, ""},
		{"vacío", ``, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ParseSlackReply([]byte(c.in)); got != c.want {
				t.Fatalf("ParseSlackReply(%q) = %q, quiero %q", c.in, got, c.want)
			}
		})
	}
}

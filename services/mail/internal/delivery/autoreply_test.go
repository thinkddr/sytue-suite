// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"strings"
	"testing"
)

// msg compone un .eml de prueba a partir de cabeceras + cuerpo (CRLF).
func msg(headers, body string) []byte {
	return []byte(strings.ReplaceAll(headers, "\n", "\r\n") + "\r\n\r\n" + body)
}

func TestAutoReplyEligible(t *testing.T) {
	normal := "From: bob@x.com\nTo: ada@sytue.eu\nSubject: hola"

	cases := []struct {
		name         string
		headers      string
		envelopeFrom string
		want         bool
	}{
		{"correo normal", normal, "bob@x.com", true},
		{"remitente de sobre vacío (rebote)", normal, "", false},
		{"remitente de sobre <> (rebote)", normal, "<>", false},
		{"Auto-Submitted auto-generated", normal + "\nAuto-Submitted: auto-generated", "bob@x.com", false},
		{"Auto-Submitted auto-replied", normal + "\nAuto-Submitted: auto-replied", "bob@x.com", false},
		{"Auto-Submitted: no (permitido)", normal + "\nAuto-Submitted: no", "bob@x.com", true},
		{"Precedence bulk", normal + "\nPrecedence: bulk", "bob@x.com", false},
		{"Precedence list", normal + "\nPrecedence: list", "bob@x.com", false},
		{"Precedence junk", normal + "\nPrecedence: junk", "bob@x.com", false},
		{"List-Id (lista)", normal + "\nList-Id: <news.x.com>", "bounce@x.com", false},
		{"List-Unsubscribe (lista)", normal + "\nList-Unsubscribe: <mailto:u@x.com>", "bob@x.com", false},
		{"X-Auto-Response-Suppress All", normal + "\nX-Auto-Response-Suppress: All", "bob@x.com", false},
		{"X-Auto-Response-Suppress OOF", normal + "\nX-Auto-Response-Suppress: DR, OOF", "bob@x.com", false},
	}
	for _, c := range cases {
		if got := autoReplyEligible(msg(c.headers, "cuerpo"), c.envelopeFrom); got != c.want {
			t.Errorf("%s: autoReplyEligible = %v, quiero %v", c.name, got, c.want)
		}
	}
}

func TestBuildAutoReply(t *testing.T) {
	raw := string(buildAutoReply("ada@sytue.eu", "bob@x.com", "Estoy de vacaciones", "Vuelvo el lunes.", "orig-123@x.com"))

	// RFC 3834 CRÍTICO: la propia respuesta se marca como auto-replied para romper
	// bucles con otro auto-respondedor.
	if !strings.Contains(raw, "Auto-Submitted: auto-replied") {
		t.Errorf("falta Auto-Submitted: auto-replied\n%s", raw)
	}
	for _, want := range []string{
		"From: ada@sytue.eu",
		"To: bob@x.com",
		"Subject: Estoy de vacaciones",
		"In-Reply-To: <orig-123@x.com>",
		"References: <orig-123@x.com>",
		"Precedence: bulk",
		"Content-Transfer-Encoding: base64",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("falta %q en la respuesta\n%s", want, raw)
		}
	}
}

func TestBuildAutoReplyDefaultSubjectAndUnicode(t *testing.T) {
	// Asunto vacío ⇒ por defecto («Respuesta automática», con acento no-ASCII);
	// sin Message-ID original ⇒ sin In-Reply-To.
	raw := string(buildAutoReply("ada@sytue.eu", "bob@x.com", "", "cuerpo", ""))
	if strings.Contains(raw, "In-Reply-To:") {
		t.Errorf("sin Message-ID original no debe haber In-Reply-To\n%s", raw)
	}
	// El asunto por defecto lleva acento ⇒ va como encoded-word (RFC 2047).
	if !strings.Contains(raw, "=?utf-8?B?") {
		t.Errorf("el asunto no-ASCII debe ir codificado (RFC 2047)\n%s", raw)
	}
}

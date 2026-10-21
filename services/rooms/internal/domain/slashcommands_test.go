// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// TestNormalizeSlashName: el nombre acaba en una restricción de la tabla y en el
// autocompletado. Si aquí pasara algo que la base rechaza, el error saldría como un 500
// en vez de como «ese nombre no vale».
func TestNormalizeSlashName(t *testing.T) {
	for in, want := range map[string]string{
		"desplegar":    "desplegar",
		"/desplegar":   "desplegar",
		"  /Desplegar": "desplegar",
		"build_prod":   "build_prod",
		"a-b-c":        "a-b-c",
		"":             "",
		"/":            "",
		"1empieza-mal": "", // debe empezar por letra
		"con espacio":  "",
		"con.punto":    "",
		"MAYÚSCULAS":   "",
		"demasiado-largo-para-un-comando-de-barra": "", // > 32
	} {
		if got := NormalizeSlashName(in); got != want {
			t.Fatalf("NormalizeSlashName(%q) = %q, quiero %q", in, got, want)
		}
	}
}

// TestNoSePuedePisarUnComandoIntegrado: el cliente resuelve los integrados ANTES de
// preguntar al servidor, así que un «/help» registrado no se ejecutaría jamás. Quien lo
// registrara vería un comando muerto sin ninguna pista de por qué.
func TestNoSePuedePisarUnComandoIntegrado(t *testing.T) {
	for _, n := range []string{"help", "me", "topic", "invite", "msg", "dm", "search", "leave"} {
		if got := NormalizeSlashName(n); got != "" {
			t.Fatalf("se aceptó pisar el comando integrado /%s", n)
		}
	}
}

// TestSplitSlashInput: los argumentos van TAL CUAL al comando. Recortarlos o partirlos
// cambiaría lo que su autor escribió.
func TestSplitSlashInput(t *testing.T) {
	for _, c := range []struct{ in, name, args string }{
		{"/desplegar prod", "desplegar", "prod"},
		{"/desplegar", "desplegar", ""},
		{"  /desplegar   prod  --forzar  ", "desplegar", "prod  --forzar"},
		{"/desplegar\nprod", "desplegar", "prod"},
		{"desplegar prod", "", ""}, // sin barra no es un comando
		{"/", "", ""},
		{"/1malo x", "", ""},
		{"", "", ""},
	} {
		name, args := SplitSlashInput(c.in)
		if name != c.name || args != c.args {
			t.Fatalf("SplitSlashInput(%q) = (%q,%q), quiero (%q,%q)", c.in, name, args, c.name, c.args)
		}
	}
}

// TestRespuestaPorDefectoEsEfimera: publicar en el canal es lo irreversible de las dos
// opciones, así que un `response_type` que no se entienda no puede acabar ahí.
func TestRespuestaPorDefectoEsEfimera(t *testing.T) {
	if got := NormalizeSlashResponseType("in_channel"); got != SlashResponseInChannel {
		t.Fatalf("in_channel = %q", got)
	}
	for _, raw := range []string{"", "ephemeral", "basura", "IN CHANNEL", "público"} {
		if got := NormalizeSlashResponseType(raw); got != SlashResponseEphemeral {
			t.Fatalf("%q acabó publicándose en el canal (= %q)", raw, got)
		}
	}
	// Insensible a mayúsculas y espacios: es lo que manda un bot real.
	if got := NormalizeSlashResponseType(" In_Channel "); got != SlashResponseInChannel {
		t.Fatalf(" In_Channel  = %q", got)
	}
}

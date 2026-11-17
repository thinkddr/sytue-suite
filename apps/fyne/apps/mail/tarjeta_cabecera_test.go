// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLaTarjetaDesplegadaEnseñaElCCYLaCCO: como `ThreadMessage` de la web, el CC
// tiene su línea bajo el «Para» —sin él no se sabe a quién llega «Responder a
// todos»— y la CCO solo sale si el sobre la trae. Cada línea suma un `text-sm` al
// alto de la cabecera; una lista vacía no deja hueco.
func TestLaTarjetaDesplegadaEnseñaElCCYLaCCO(t *testing.T) {
	tr := traductor(t)
	l := NuevoLector(tr, "es")
	m := MensajeDe(api.MailMessage{
		ID: "m1", From: "ana@ejemplo.eu", To: []string{"yo@ejemplo.eu"},
		Cc: []string{"luis@ejemplo.eu", "eva@ejemplo.eu"}, Bcc: []string{"jefa@ejemplo.eu"},
	})
	con := nuevaTarjetaMensaje(l, m, true)
	quiero := []string{
		"ana@ejemplo.eu",
		"Para: yo@ejemplo.eu",
		"CC: luis@ejemplo.eu, eva@ejemplo.eu",
		"CCO: jefa@ejemplo.eu",
	}
	if got := con.lineasCabecera(); !slices.Equal(got, quiero) {
		t.Errorf("la cabecera desplegada pinta %q; quiero %q", got, quiero)
	}

	m.CC, m.CCO = nil, nil
	sin := nuevaTarjetaMensaje(l, m, true)
	if got := sin.lineasCabecera(); len(got) != 2 || strings.HasPrefix(got[len(got)-1], "CC") {
		t.Errorf("sin CC ni CCO la cabecera pinta %q", got)
	}
	if d := con.altoCabecera() - sin.altoCabecera(); d != 2*tema.TextoSM.Linea {
		t.Errorf("CC y CCO suman %v al alto de la cabecera; deberían sumar dos líneas text-sm (%v)", d, 2*tema.TextoSM.Linea)
	}

	// Plegada sigue siendo el extracto, con o sin destinatarios en copia.
	m.CC = []Direccion{{Correo: "luis@ejemplo.eu"}}
	m.Extracto = "Hola"
	if got := nuevaTarjetaMensaje(l, m, false).lineasCabecera(); !slices.Equal(got, []string{"Hola"}) {
		t.Errorf("plegada pinta %q", got)
	}
}

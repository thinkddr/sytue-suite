// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

// Lo que guarda una persona no lo lee otra, y sin nadie dentro no se guarda nada.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el sujeto en la clave, Beto lee lo que
// plegó Ana.
func TestPorPersonaSeparaALasPersonas(t *testing.T) {
	prefs := test.NewApp().Preferences()
	quien := "ana"
	p := NuevoPorPersona(prefs, func() string { return quien })
	p.Poner("plegadas", `["dms"]`)
	if p.Leer("plegadas") != `["dms"]` {
		t.Fatalf("Ana no lee lo suyo: %q", p.Leer("plegadas"))
	}
	quien = "beto"
	if got := p.Leer("plegadas"); got != "" {
		t.Errorf("Beto lee lo de Ana: %q", got)
	}
	quien = ""
	p.Poner("plegadas", "x")
	if got := p.Leer("plegadas"); got != "" {
		t.Errorf("sin nadie dentro se guarda %q", got)
	}
	quien = "ana"
	p.Poner("plegadas", "")
	if got := p.Leer("plegadas"); got != "" {
		t.Errorf("poner vacío no lo borra: %q", got)
	}
}

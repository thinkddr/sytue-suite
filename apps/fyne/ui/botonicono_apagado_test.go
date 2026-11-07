// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElBotonIconoApagadoSigueEnElTabulador: `Apagado` es `aria-disabled` —no
// responde y se ve atenuado como uno Inerte—, pero NO sale del tabulador, que es
// lo que lo distingue de `disabled`. VISTO EN ROJO dejando que Tapped llame y sin
// atenuar el glifo.
func TestElBotonIconoApagadoSigueEnElTabulador(t *testing.T) {
	pulsado := 0
	b := NuevoBotonIcono(iconos.Imagen, "Es la portada del álbum", func() { pulsado++ })
	b.Apagado = true
	b.Tapped(nil)
	b.TypedRune(' ')
	if pulsado != 0 {
		t.Errorf("apagado, el botón respondió %d veces", pulsado)
	}
	if b.Disabled() {
		t.Error("apagado no es inerte: tiene que seguir siendo parada del tabulador")
	}
	_, apagado := b.colores(tema.Claro)
	inerte := NuevoBotonIcono(iconos.Imagen, "x", nil)
	inerte.Inerte = true
	if _, quiero := inerte.colores(tema.Claro); apagado != quiero {
		t.Errorf("apagado el glifo va en %v y uno inerte en %v: tiene que verse igual", apagado, quiero)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// El punto de presencia es el de Avatar.tsx con `box-sizing: border-box`: el aro de
// `surface` mide `dot` y el disco de color, `dot − 4`. Y no va solo en el color: cada
// estado lleva su forma (la luna, la raya, el hueco), y en línea es el disco lleno.
//
// VISTO EN ROJO (2026-09-26, en el frontal, saboteado a mano): con el aro de `dot + 4`
// de antes, el de un avatar `sm` mide 15 y no 11; y sin la forma, los cuatro estados son
// el mismo disco.
func TestElPuntoDePresenciaTieneForma(t *testing.T) {
	a := NuevoAvatar("Ada Lovelace").Con(AvatarSM)
	a.Presencia = EnLinea
	r := test.WidgetRenderer(a).(*avatarRenderer)
	a.Resize(MedidaAvatar(AvatarSM))
	r.Layout(a.Size())
	// sm = 32: dot = max(10, round(32 × 0,34)) = 11, a 1,75 fuera por la derecha y abajo.
	if r.aro.Size() != fyne.NewSize(11, 11) || r.aro.Position() != fyne.NewPos(23, 23) {
		t.Errorf("el aro mide %v en %v", r.aro.Size(), r.aro.Position())
	}
	if r.punto.Size() != fyne.NewSize(7, 7) || r.punto.Position() != fyne.NewPos(25, 25) {
		t.Errorf("el disco mide %v en %v", r.punto.Size(), r.punto.Position())
	}
	if r.forma.Visible() {
		t.Error("en línea lleva forma y es el disco lleno")
	}
	formas := map[fyne.Size]Presencia{}
	for _, p := range []Presencia{Ausente, NoMolestar, Desconectado} {
		a.Presencia = p
		a.Refresh()
		if !r.forma.Visible() {
			t.Errorf("%v va sin forma", p)
		}
		formas[r.forma.Size()] = p
	}
	if len(formas) != 3 {
		t.Errorf("las tres formas no se distinguen: %v", formas)
	}
	a.Presencia = SinPresencia
	a.Refresh()
	if r.forma.Visible() || r.punto.Visible() {
		t.Error("sin presencia queda algo del punto")
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElGrupoDeCampoEsElFieldGroupDeLaWeb: la etiqueta ocupa su caja de línea de
// `text-sm` (15,75), el `gap-1.5` (5,25) y el campo debajo a todo el ancho. Con la caja
// de la FUENTE en vez de la de línea el campo subiría un par de píxeles.
func TestElGrupoDeCampoEsElFieldGroupDeLaWeb(t *testing.T) {
	campo := NuevaEntrada("")
	g := GrupoCampo("Nombre", false, campo, "", "")
	quiero := tema.TextoSM.Linea + HuecoGrupoCampo + campo.MinSize().Height
	if got := g.MinSize().Height; got != quiero {
		t.Errorf("el grupo mide %v de alto y FieldGroup %v", got, quiero)
	}
	g.Resize(fyne.NewSize(300, quiero))
	if y := campo.Position().Y; y != tema.TextoSM.Linea+HuecoGrupoCampo {
		t.Errorf("el campo empieza en y=%v", y)
	}
	if w := campo.Size().Width; w != 300 {
		t.Errorf("el campo mide %v de ancho y tiene que llenar el grupo", w)
	}
	rotulo, ok := g.Objects[0].(*Rotulo)
	if !ok || rotulo.Texto != "Nombre" {
		t.Error("la etiqueta no es la primera pieza del grupo")
	}
}

// TestElHuecoDeUnBotonSeRespeta: un disparador escrito a mano con `gap-1.5` mide 1,75
// menos que el `gap-2` de la talla, y el glifo se coloca con ese hueco.
func TestElHuecoDeUnBotonSeRespeta(t *testing.T) {
	normal := NuevoBoton("Nuevo", nil).Con(Secundario, TallaSM)
	normal.IconoDetras, normal.TamIcono = iconos.ChevronAbajo, 14
	estrecho := NuevoBoton("Nuevo", nil).Con(Secundario, TallaSM)
	estrecho.IconoDetras, estrecho.TamIcono, estrecho.Hueco = iconos.ChevronAbajo, 14, 1.5*0.25*tema.Rem
	if d := normal.MinSize().Width - estrecho.MinSize().Width; d != 2 {
		t.Errorf("el gap-1.5 ahorra %v y tiene que ahorrar 1,75", d)
	}
}

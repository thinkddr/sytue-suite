// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package pantalla es la pantalla mínima de punta a punta de los cimientos: tema,
// widgets propios y una medida verificada contra el número de producción.
//
// No es una app de la suite ni la va a ser. Existe para dos cosas: que `cmd/demo` se
// pueda abrir en una máquina con pantalla y se VEA, y que `demo_test.go` la capture sin
// servidor gráfico y compruebe los píxeles. Cuando el catálogo de widgets crezca, esta
// pantalla crece con él y sigue siendo el sitio donde se mira si algo se ha torcido.
package pantalla

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Ancho y Alto son los del lienzo de la demostración. Fijos, porque la captura se
// compara píxel a píxel y una ventana de tamaño variable no se puede comparar.
const (
	Ancho = 520
	Alto  = 260
)

// Demo construye la pantalla: un fondo de papel, una tarjeta con título y, dentro, las
// cuatro variantes del botón.
func Demo(v tema.Variante) fyne.CanvasObject {
	fondo := canvas.NewRectangle(tema.Color("bg", v))
	fondo.Resize(fyne.NewSize(Ancho, Alto))

	botones := []*ui.Boton{
		ui.NuevoBoton("Guardar cambios", nil),
		ui.NuevoBoton("Cancelar", nil).Con(ui.Secundario, ui.TallaMD),
		ui.NuevoBoton("Más opciones", nil).Con(ui.Fantasma, ui.TallaMD),
		ui.NuevoBoton("Eliminar", nil).Con(ui.Peligro, ui.TallaMD),
	}
	// `gap-2` entre botones, con el layout propio de la suite.
	hijos := make([]fyne.CanvasObject, len(botones))
	for i, b := range botones {
		hijos[i] = b
	}
	fila := container.New(ui.Fila{Hueco: tema.Espacio(2)}, hijos...)

	tarjeta := ui.NuevaTarjeta("Cambios sin guardar", fila)
	tarjeta.Resize(tarjeta.MinSize())
	tarjeta.Move(fyne.NewPos(tema.Espacio(6), tema.Espacio(6)))

	inerte := ui.NuevoBoton("Enviando…", nil)
	inerte.Inerte = true
	inerte.Resize(inerte.MinSize())
	inerte.Move(fyne.NewPos(tema.Espacio(6), tarjeta.Position().Y+tarjeta.Size().Height+tema.Espacio(4)))

	return container.NewWithoutLayout(fondo, tarjeta, inerte)
}

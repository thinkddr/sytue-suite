// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaFranjaSoloLecturaTieneSalida es el contrato de `ReadOnlyBar.tsx`: la franja
// lleva el botón que la desbloquea, el botón funciona, y con la petición ya enviada el
// botón deja paso a la nota y no se puede volver a pedir.
//
// Los altos son los de la hoja, no los que calcula el widget: `py-1` (3,5 + 3,5) más el
// botón `h-6` con su suelo de 24 más el `border-b` = 32; sin botón, la línea de
// `text-sm` de la hoja de la suite (15,75, `tema.TextoSM`) en vez del botón = 23,75.
func TestLaFranjaSoloLecturaTieneSalida(t *testing.T) {
	pedida := 0
	f := NuevaFranjaSoloLectura("Solo lectura").Con("Pedir acceso de edición", func() { pedida++ })
	w := test.NewTempWindow(t, f)
	w.Resize(fyne.NewSize(800, 60))
	if got := f.MinSize().Height; got != 33 {
		t.Errorf("con botón mide %v de alto y la web 32", got)
	}
	test.Tap(f.Accion)
	if pedida != 1 {
		t.Fatalf("pulsar «Pedir acceso de edición» no pidió nada (%d)", pedida)
	}
	if p := f.Accion.Position(); p.X <= 400 || !f.Accion.Visible() {
		t.Errorf("el botón no está a la derecha de la franja: %v", p)
	}

	f.Nota = "Petición enviada"
	f.Refresh()
	if len(f.botones()) != 0 {
		t.Error("con la petición enviada la franja sigue ofreciendo el botón")
	}
	if got := f.MinSize().Height; got != 27 {
		t.Errorf("sin botón mide %v de alto y la web 23,75", got)
	}
}

// TestEnElTelefonoLaFranjaLlevaElSueloTactilYDosLineas: con el dedo el botón `xs` mide 44
// (`pointer-coarse:min-h-[max(44px,2.75rem)]` de Button.tsx), y la franja, medida para el
// de 24, lo dejaba salirse por arriba y por abajo; y el mensaje, en una línea al lado de
// «Pedir acceso de edición», se quedaba en cuatro palabras (auditoría del 2026-11-24, en
// Calendar, el editor y `peticion`). En el teléfono la franja crece con el botón, el botón
// cabe entero y el mensaje se reparte en dos líneas. VISTO EN ROJO sin `SueloTactil`.
func TestEnElTelefonoLaFranjaLlevaElSueloTactilYDosLineas(t *testing.T) {
	restaurar := TactilDePrueba(true)
	t.Cleanup(func() { restaurar() })
	f := NuevaFranjaSoloLectura("Este documento es de solo lectura: pide permiso para editarlo.").
		Con("Pedir acceso de edición", nil)
	w := test.NewTempWindow(t, f)
	w.Resize(fyne.NewSize(390, 120))
	f.Resize(fyne.NewSize(390, f.MinSize().Height))
	alto := f.MinSize().Height
	if quiero := 2*rellenoFranjaY + SueloTactilBarra + tema.Borde; alto != quiero {
		t.Errorf("en el teléfono mide %v de alto y con el botón de 44 son %v", alto, quiero)
	}
	if b := f.Accion; b.Position().Y < 0 || b.Position().Y+b.Size().Height > alto-tema.Borde || b.Size().Height < SueloTactilBarra {
		t.Errorf("el botón (%v, %v) no cabe en la franja de %v", b.Position(), b.Size(), alto)
	}
	r := test.WidgetRenderer(f).(*franjaSoloLecturaRenderer)
	if r.texto2.Hidden || r.texto2.Text == "" {
		t.Fatalf("el mensaje sigue en una línea: «%s»", r.texto.Text)
	}
	if r.texto.Position().Y < 0 || r.texto2.Position().Y+tema.TextoSM.Linea > alto {
		t.Error("las dos líneas del mensaje no caben en la franja")
	}
	guardar(t, "franja-solo-lectura-telefono.png", w.Canvas().Capture())

	// En escritorio, como siempre: una línea y el botón de 24.
	TactilDePrueba(false)
	f.Refresh()
	if got := f.MinSize().Height; got != 2*rellenoFranjaY+medidas[TallaXS].alto+tema.Borde {
		t.Errorf("en escritorio mide %v", got)
	}
}

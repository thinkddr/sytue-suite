// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/png"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestBuscarYReemplazar: se encuentra también lo que cruza un cambio de formato, se
// reemplaza la activa y luego todas, y deshacer lo devuelve todo de un paso.
func TestBuscarYReemplazar(t *testing.T) {
	const antes = "<p>ho<b>la</b> y hola</p><h2>Hola</h2>"
	f := folioCon(t, antes)
	if n := f.Buscar("hola", false, false); n != 3 {
		t.Fatalf("hay 3 «hola» (una con media negrita) y se encuentran %d", n)
	}
	f.IrACoincidencia(1)
	f.Reemplazar("adiós", "hola", false, false)
	if got := f.Serializar(); got != "<p>ho<b>la</b> y adiós</p><h2>Hola</h2>" {
		t.Errorf("reemplazar la activa: %s", got)
	}
	if len(f.resaltes) != 2 || f.resaltes[f.activa].b != 1 {
		t.Errorf("tras reemplazar, la activa es la siguiente: %+v (activa %d)", f.resaltes, f.activa)
	}
	f.ReemplazarTodo("X", "hola", false, false)
	if got := f.Serializar(); got != "<p>X y adiós</p><h2>X</h2>" {
		t.Errorf("reemplazar todo: %s", got)
	}
	f.deshacer()
	f.deshacer()
	if got := f.Serializar(); got != antes {
		t.Errorf("deshacer los dos reemplazos: %s", got)
	}
}

// TestCerrarBuscarQuitaElAmarillo: como en la web y en Calcus, al cerrar el panel el
// folio deja de pintar las coincidencias; al volver a abrirlo busca otra vez lo tecleado
// y se queda en la que se estaba viendo.
func TestCerrarBuscarQuitaElAmarillo(t *testing.T) {
	f := folioCon(t, "<p>hola y hola</p><p>hola</p>")
	amarillos := func() int {
		n := 0
		for _, o := range pintar(f, fyne.NewPos(0, 0), tema.Claro) {
			if r, ok := o.(*canvas.Rectangle); ok && (r.FillColor == editor.ResalteBuscar || r.FillColor == editor.ResalteActivo) {
				n++
			}
		}
		return n
	}
	f.Buscar("hola", false, false)
	f.IrACoincidencia(2)
	if amarillos() != 3 {
		t.Fatalf("con el panel abierto se pintan %d coincidencias", amarillos())
	}
	f.OcultarResaltes()
	if n := amarillos(); n != 0 {
		t.Errorf("con el panel cerrado siguen pintadas %d", n)
	}
	f.MostrarResaltes()
	if amarillos() != 3 || f.Activa() != 2 {
		t.Errorf("al volver a abrirlo se pintan %d y la activa es %d", amarillos(), f.Activa())
	}
}

// TestElEsquemaSonLosTitulos: h1–h3, sangrados por nivel, y el mensaje de la web cuando
// no hay ninguno.
func TestElEsquemaSonLosTitulos(t *testing.T) {
	f := folioCon(t, "<h1>Uno</h1><p>x</p><h2>Dos</h2><h3></h3><h4>Cuatro</h4>")
	var filas []*filaEsquema
	var buscar func(o fyne.CanvasObject)
	buscar = func(o fyne.CanvasObject) {
		if fe, ok := o.(*filaEsquema); ok {
			filas = append(filas, fe)
		}
		if c, ok := o.(*fyne.Container); ok {
			for _, h := range c.Objects {
				buscar(h)
			}
		}
		if s, ok := o.(*container.Scroll); ok {
			buscar(s.Content)
		}
	}
	buscar(f.PanelEsquema(nil))
	if len(filas) != 3 || filas[0].Texto != "Uno" || filas[2].Texto != "outline.untitled" || filas[1].nivel != 2 {
		t.Fatalf("el esquema: %d filas %+v", len(filas), filas)
	}
	if sangriaEsquema(1) != 8 || sangriaEsquema(3) != 24 {
		t.Errorf("las sangrías son `pl-2`, `pl-4`, `pl-6`: %v y %v", sangriaEsquema(1), sangriaEsquema(3))
	}
}

// TestLosPanelesSePintan monta el editor con «Buscar» abierto y con resaltes, y lo pinta:
// un token de color inventado entra en pánico AL PINTAR, y aquí se pinta todo.
func TestLosPanelesSePintan(t *testing.T) {
	ctx := &editor.Contexto{Tipo: editor.Documento, Zoom: 100, Regla: true, Estado: editor.Listo, Panel: editor.PanelBuscar}
	f := NuevoFolio(ctx)
	f.Programar = func(time.Duration, func()) {} // sin relojes vivos: la búsqueda se lanza a mano
	ctx.Lienzo = f
	ctx.Paneles = panelesDe(f, nil, "es")
	f.Cargar(muestraReal(t))
	m := editor.NuevoMarco(ctx, registroDocus("es"), nil)
	f.PanelBuscar(nil)
	f.buscador.Consultar("Sytue")
	if f.buscador.Cuenta() != 2 {
		t.Errorf("«Sytue» sale dos veces en la demo y el contador dice %d", f.buscador.Cuenta())
	}
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(m)
	c.Resize(fyne.NewSize(1280, 744))
	img := c.Capture()
	if img.Bounds().Dx() == 0 {
		t.Fatal("no se ha pintado nada")
	}
	if ruta := os.Getenv("SYTUE_CAPTURA_PANEL"); ruta != "" {
		fh, err := os.Create(ruta)
		if err != nil {
			t.Fatal(err)
		}
		defer fh.Close()
		_ = png.Encode(fh, img)
	}
	ctx.Panel = editor.PanelEsquema
	m.Recomponer()
	c.Capture()
}

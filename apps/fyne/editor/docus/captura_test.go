// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/software"
)

// TestLaPantallaSePinta monta Docus entero con el documento real y lo pinta con el
// pintor de software, sin CGO y sin servidor gráfico.
//
// Con `SYTUE_CAPTURA=/ruta.png` deja además la imagen, que es como se MIRA lo que se
// pinta en vez de describirlo de memoria; con `SYTUE_CAPTURA_DOC=/doc.html`, de ese
// documento. Sin la variable, sigue siendo un guardián: un token de color inventado entra
// en pánico AL PINTAR, y aquí se pinta todo.
func TestLaPantallaSePinta(t *testing.T) {
	o := Pantalla(nil, nil)
	doc := muestraReal(t)
	if ruta := os.Getenv("SYTUE_CAPTURA_DOC"); ruta != "" {
		b, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}
		doc = string(b)
	}
	if f, ok := buscarFolio(o); ok {
		f.Cargar(doc)
	}
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(o)
	c.SetScale(1)
	c.Resize(fyne.NewSize(1280, 744)) // 800 menos la barra superior de la web
	img := c.Capture()
	if ruta := os.Getenv("SYTUE_CAPTURA"); ruta != "" {
		f, err := os.Create(ruta)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
	if img.Bounds().Dx() == 0 {
		t.Fatal("la pantalla no ha pintado nada")
	}
}

// buscarFolio saca el folio del árbol montado; Pantalla no lo devuelve porque el
// binario no lo necesita.
func buscarFolio(o fyne.CanvasObject) (*Folio, bool) {
	if f, ok := o.(*Folio); ok {
		return f, true
	}
	type conHijos interface{ Objects() []fyne.CanvasObject }
	if c, ok := o.(conHijos); ok {
		for _, h := range c.Objects() {
			if f, ok := buscarFolio(h); ok {
				return f, true
			}
		}
	}
	if w, ok := o.(fyne.Widget); ok {
		for _, h := range w.CreateRenderer().Objects() {
			if f, ok := buscarFolio(h); ok {
				return f, true
			}
		}
	}
	return nil, false
}

// TestLoQueLlegaDeDriveSeVeEnSoloLectura: el camino del enlace entrante. Lo que se carga
// es una CONVERSIÓN del fichero, no un documento de Docs, así que tiene que verse —el
// folio lleva el contenido— y no dejarse editar, porque no hay dónde guardarlo.
func TestLoQueLlegaDeDriveSeVeEnSoloLectura(t *testing.T) {
	o, cargar := PantallaConCarga(nil, nil)
	cargar("Propuesta de servicio", muestraReal(t))
	f, ok := buscarFolio(o)
	if !ok {
		t.Fatal("la pantalla no tiene folio")
	}
	if len(f.Documento().Bloques()) == 0 {
		t.Fatal("el folio sigue vacío después de cargar")
	}
	if f.ctx.Titulo != "Propuesta de servicio" || !f.ctx.SoloLectura {
		t.Errorf("título %q, solo lectura %v", f.ctx.Titulo, f.ctx.SoloLectura)
	}
}

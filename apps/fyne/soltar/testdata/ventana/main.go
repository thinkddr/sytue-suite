// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command ventana es la ventana de soltar_x11_test.go: una ventana de Fyne que escribe
// en la salida, una línea JSON por evento, lo que se suelta sobre ella. Vive en
// testdata para que `go build ./...` no la vea.
package main

import (
	"encoding/json"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver"

	"git.sytue.eu/sytue/one/apps/fyne/soltar"
)

// Evento es una línea de la salida.
type Evento struct {
	Tipo     string   `json:"tipo"`
	Texto    string   `json:"texto,omitempty"`
	URLs     []string `json:"urls,omitempty"`
	Ficheros []string `json:"ficheros,omitempty"`
	X        float32  `json:"x"`
	Y        float32  `json:"y"`
	Ventana  uintptr  `json:"ventana,omitempty"`
	Escala   float32  `json:"escala,omitempty"`
}

func main() {
	a := app.New()
	w := a.NewWindow("prueba-soltar")
	w.SetContent(canvas.NewRectangle(nil))
	w.Resize(fyne.NewSize(400, 300))
	salida := json.NewEncoder(os.Stdout)
	escribir := func(e Evento) { _ = salida.Encode(e) }

	soltar.AlSoltar(w, func(s soltar.Soltado) {
		escribir(Evento{Tipo: "texto", Texto: s.Texto, URLs: s.URLs, X: s.Posicion.X, Y: s.Posicion.Y})
	})
	soltar.AlArrastrar(w, func(encima bool) {
		if encima {
			escribir(Evento{Tipo: "entra"})
		} else {
			escribir(Evento{Tipo: "sale"})
		}
	})
	w.SetOnDropped(func(p fyne.Position, uris []fyne.URI) {
		e := Evento{Tipo: "ficheros", X: p.X, Y: p.Y}
		for _, u := range uris {
			e.Ficheros = append(e.Ficheros, u.String())
		}
		escribir(e)
	})
	// «lista» cuando el bucle ya corre, con la escala del lienzo, que la prueba necesita
	// para saber dónde tiene que caer lo soltado.
	a.Lifecycle().SetOnStarted(func() {
		e := Evento{Tipo: "lista", Escala: w.Canvas().Scale()}
		if nw, ok := w.(driver.NativeWindow); ok {
			nw.RunNative(func(c any) {
				if c, ok := c.(driver.X11WindowContext); ok {
					e.Ventana = c.WindowHandle
				}
			})
		}
		escribir(e)
	})
	w.ShowAndRun()
}

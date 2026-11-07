// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las cajas con `border` de Rooms dejan ver su filete y su interior por separado: el
// píxel del borde de arriba no es del color del de justo debajo.
//
// VISTO EN ROJO (2026-11-06): con container.NewStack, el rectángulo de dentro de la
// ui.Caja tapaba el borde (la previa de reenviar, la tarjeta de encuesta y la del
// fichero adjunto salían sin filete), y con relleno transparente sin ConTrazo el de fuera
// quedaba macizo (la lista de quién lo ha visto, cada opción de la encuesta y la casilla
// sin marcar de «Nuevo mensaje directo» eran bloques del color del borde).
func TestLasCajasDeRoomsEnsenanElBordeYElInterior(t *testing.T) {
	tr := traductor(t)
	lectores := func() fyne.CanvasObject {
		col := ListaLectores(tr, i18n.Locale("es"), []api.RoomsReader{
			{Subject: "ana", Name: "Ana", ReadAt: time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)},
			{Subject: "beto", Name: "Beto", ReadAt: time.Date(2026, 11, 6, 10, 5, 0, 0, time.UTC)},
		}, 3)
		return col.(*fyne.Container).Objects[1] // la lista, sin el «Visto por 2 de 3»
	}
	encuesta := func() fyne.CanvasObject {
		v := &VistaTarjeta{T: tr, Locale: "es", Ahora: time.Now, Tarjeta: &Tarjeta{Tipo: "poll", Opciones: []string{"Viernes", "Lunes"}}}
		return v.construir()
	}
	casos := []struct {
		nombre string
		objeto func() fyne.CanvasObject
		// x es la columna (en px de CSS) donde se mira; borde, la fila del filete de
		// arriba; dentro, una fila del interior sin nada encima.
		x, borde, dentro float32
		tam              fyne.Size
	}{
		{"previa de reenviar", func() fyne.CanvasObject {
			return NuevoReenviar(tr, Mensaje{Cuerpo: "hola", Autor: Autor{Nombre: "Ana"}}, nil)
		}, 200, 0, 3, fyne.NewSize(400, 300)},
		{"quién lo ha visto", lectores, 200, 0, 3, fyne.NewSize(400, 0)},
		{"tarjeta de encuesta", encuesta, 200, 0, 3, fyne.NewSize(400, 0)},
		{"opción de encuesta", func() fyne.CanvasObject {
			return NuevaOpcionEncuesta("Viernes", "1 · 50 %", false, 0, false, func() {})
		}, 100, 0, 3, fyne.NewSize(300, 0)},
		{"fichero adjunto", func() fyne.CanvasObject {
			return (&VistaAdjuntos{T: tr}).fichero(Adjunto{ID: "1", Nombre: "acta.pdf", Tipo: "application/pdf"})
		}, 150, 0, 3, fyne.NewSize(300, 0)},
		{"casilla sin marcar", func() fyne.CanvasObject {
			return NuevaFilaPersona(Persona{ID: "ana", Nombre: "Ana"}, false, func() {})
		}, 0, 0, 0, fyne.NewSize(300, 0)},
	}
	for _, c := range casos {
		o := c.objeto()
		tam := c.tam
		if tam.Height == 0 {
			tam.Height = o.MinSize().Height
		}
		o.Resize(tam)
		x, yb, yd := c.x, c.borde, c.dentro
		if c.nombre == "casilla sin marcar" {
			// La casilla de 16 va en `px-2`, centrada en el alto: se mira su filete
			// de la izquierda y su centro.
			arriba := (tam.Height - 16) / 2
			img := pinta(t, tam.Width, tam.Height, o)
			filete, centro := px(img, int(8*escala), int((arriba+8)*escala)), px(img, int(16*escala), int((arriba+8)*escala))
			if cerca(filete, centro) {
				t.Errorf("%s: el filete %v es del color del centro %v", c.nombre, filete, centro)
			}
			continue
		}
		img := pinta(t, tam.Width, tam.Height, o)
		filete, interior := px(img, int(x*escala), int(yb*escala)), px(img, int(x*escala), int(yd*escala))
		if cerca(filete, interior) {
			t.Errorf("%s: el filete %v es del color del interior %v", c.nombre, filete, interior)
		}
	}
}

// Los botones de la tarjeta de un fichero («Abrir» y la nube) van centrados en alto,
// como la flecha de descargar del enlace: es el `items-center` de Attachments.tsx.
//
// VISTO EN ROJO (2026-11-06): con ui.Fila directamente en el lado derecho de un
// Border, los botones se quedaban arriba de la tarjeta y la flecha, más baja.
func TestLosBotonesDelFicheroVanCentradosEnAlto(t *testing.T) {
	o := (&VistaAdjuntos{T: traductor(t)}).fichero(Adjunto{ID: "1", Nombre: "acta.pdf", Tipo: "application/pdf"})
	alto := o.MinSize().Height
	o.Resize(fyne.NewSize(300, alto))
	n := 0
	var mirar func(c fyne.CanvasObject, y float32)
	mirar = func(c fyne.CanvasObject, y float32) {
		y += c.Position().Y
		if b, ok := c.(*ui.BotonIcono); ok {
			n++
			if centro := y + b.Size().Height/2; centro < alto/2-0.5 || centro > alto/2+0.5 {
				t.Errorf("%q: centro en %v, la tarjeta en %v", b.Rotulo, centro, alto/2)
			}
			return
		}
		if k, ok := c.(*fyne.Container); ok {
			for _, h := range k.Objects {
				mirar(h, y)
			}
		}
	}
	mirar(o, -o.Position().Y)
	if n != 2 {
		t.Fatalf("hay %d botones, quiero «Abrir» y la nube", n)
	}
}

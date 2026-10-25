// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestElCanalYElHiloConAdjuntosSeRecorrenAlGirar(t *testing.T) {
	t.Cleanup(ui.TactilDePrueba(true))
	for _, hilo := range []bool{false, true} {
		t.Run(fmt.Sprintf("hilo=%v", hilo), func(t *testing.T) {
			tr := traductor(t)
			var objeto fyne.CanvasObject
			var compositor *Compositor
			var flujo *columnaConversacion
			if hilo {
				p := NuevoPanelHilo(tr, "es")
				p.Estado, p.Raiz = Listo, ejemplo()[0]
				objeto, compositor = p, p.Compositor
				flujo = &test.WidgetRenderer(p).(*panelHiloRenderer).flujo
			} else {
				c := nuevoCanal(tr, "es")
				c.sinCanal = false
				c.Cabecera.Canal = Canal{ID: "equipo", Nombre: "equipo"}
				c.Historial.Estado, c.Historial.Mensajes = Listo, ejemplo()
				objeto, compositor = c, c.Compositor
				flujo = &test.WidgetRenderer(c).(*canalRenderer).flujo
			}
			w := test.NewTempWindow(t, container.NewWithoutLayout(objeto))
			w.SetPadded(false)
			compositor.FormatoVisible = true
			compositor.PonerTexto("Una respuesta con todos sus adjuntos")
			compositor.PonerAvisoComando("Revisa los adjuntos antes de enviar")
			pendientes := make([]Pendiente, 6)
			for i := range pendientes {
				pendientes[i] = Pendiente{Local: fmt.Sprint(i), Nombre: "revisión-del-proyecto.pdf", Subido: &api.RoomsAttachment{}}
			}
			compositor.PonerPendientes(pendientes)
			area := compositor.area
			enviado := ""
			compositor.AlEnviar = func(s string) { enviado = s }
			for _, tam := range []fyne.Size{
				{Width: 320, Height: 700},
				{Width: 568, Height: 320},
				{Width: 320, Height: 700},
				{Width: 844, Height: 390},
				{Width: 390, Height: 844},
				{Width: 1024, Height: 768},
				{Width: 768, Height: 1024},
				{Width: 568, Height: 160},
				{Width: 320, Height: 700},
			} {
				w.Resize(tam)
				objeto.Resize(tam)
				objeto.Refresh()
				if tam == fyne.NewSize(568, 320) && !flujo.desbordando {
					t.Fatal("el caso no obliga a desplazar la columna horizontal")
				}
				if flujo.desbordando {
					if flujo.desborde.Size() != tam {
						t.Fatalf("el scroll no respeta el viewport: %v frente a %v", flujo.desborde.Size(), tam)
					}
					flujo.desborde.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -1e6)})
				}
				donde := fyne.CurrentApp().Driver().AbsolutePositionForObject
				enviar := compositor.enviar
				p := donde(enviar)
				if p.X < 0 || p.Y < 0 || p.X+enviar.Size().Width > tam.Width+0.01 || p.Y+enviar.Size().Height > tam.Height+0.01 {
					t.Fatalf("a %v Enviar no se alcanza con el gesto: %v/%v", tam, p, enviar.Size())
				}
				if enviar.Size().Width < 44 || enviar.Size().Height < 44 {
					t.Fatalf("a %v Enviar pierde el tamaño táctil: %v", tam, enviar.Size())
				}
				if flujo.desbordando {
					flujo.desborde.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, 1e6)})
					if flujo.desborde.Offset.Y != 0 {
						t.Fatal("el gesto no devuelve la cabecera al principio")
					}
				}
				if compositor.area != area || compositor.Texto() != "Una respuesta con todos sus adjuntos" || len(compositor.Pendientes) != len(pendientes) || !compositor.FormatoVisible {
					t.Fatal("girar reconstruyó el redactor o perdió el borrador, los adjuntos o el formato")
				}
			}
			test.Tap(compositor.enviar)
			if enviado != "Una respuesta con todos sus adjuntos" {
				t.Fatal("Enviar perdió su acción tras girar")
			}
		})
	}
}

func TestLaCabeceraUsaElAnchoDelCanalTrasAbrirElPanel(t *testing.T) {
	c := nuevoCanal(traductor(t), "es")
	c.sinCanal = false
	c.Ventana = func() float32 { return 1280 }
	c.Cabecera.Canal = Canal{ID: "equipo", Nombre: "equipo"}
	c.Derecho = NuevoPanelHilo(c.T, "es")
	for _, ancho := range []float32{900, 1280, 900} {
		c.Resize(fyne.NewSize(ancho, 700))
		c.Refresh()
		if want := c.Cabecera.Size().Width < ui.Estrecho; c.Cabecera.Estrecha != want {
			t.Errorf("ventana1280, canal%g: cabecera estrecha=%v, debe ser%v", c.Cabecera.Size().Width, c.Cabecera.Estrecha, want)
		}
	}
}

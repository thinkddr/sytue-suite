// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"fmt"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Carril a 768 conserva sus dos laterales: sólo quedan 156 para el canal. Las
// fichas deben ceder y los mandos de la cabecera se alcanzan con gesto y foco.
func TestElPanelAbiertoNoExpulsaLosMandosDelCanalAlGirar(t *testing.T) {
	t.Cleanup(ui.TactilDePrueba(true))
	original := "marco"
	if !tema.EsMarco() {
		original = "carril"
	}
	t.Cleanup(func() { tema.ElegirForma(original) })
	for _, forma := range []string{"marco", "carril"} {
		t.Run(forma, func(t *testing.T) {
			tema.ElegirForma(forma)
			sa, _, w := salaEn(t, servicioConHilo(), 1280)
			sa.Arrancar(context.Background())
			sa.AbrirHilo(sa.P.Canal.Historial.Mensajes[3])
			c := sa.P.Canal
			for _, comp := range []*Compositor{c.Compositor, sa.hilo.Compositor} {
				comp.FormatoVisible = true
				comp.PonerTexto("Borrador que sobrevive al giro")
				comp.PonerPendientes([]Pendiente{
					{Local: "1", Nombre: "revisión-del-proyecto.pdf", Subido: &api.RoomsAttachment{}},
					{Local: "2", Nombre: "presupuesto-completo.ods", Subido: &api.RoomsAttachment{}},
				})
			}
			for _, tam := range []fyne.Size{
				{Width: 768, Height: 1024},
				{Width: 1024, Height: 768},
				{Width: 844, Height: 390},
				{Width: 568, Height: 320},
				{Width: 1024, Height: 768},
				{Width: 768, Height: 1024},
			} {
				w.Resize(tam)
				sa.P.Objeto().Resize(tam)
				sa.P.Refrescar()
				if tam.Width == 1024 && (!c.Compositor.Visible() || !sa.hilo.Visible() || sa.hilo.Size().Width != AnchoPanelDerecho) {
					t.Fatal("a 1024 se perdió la convivencia del canal y el panel")
				}
				if forma == "carril" && tam.Width == 768 && c.Compositor.Size().Width != 156 {
					t.Fatalf("el caso dejó de reproducir el canal entre los dos laterales: %v", c.Compositor.Size())
				}
				for _, comp := range []*Compositor{c.Compositor, sa.hilo.Compositor} {
					if comp.Texto() != "Borrador que sobrevive al giro" || len(comp.Pendientes) != 2 {
						t.Fatal("el giro perdió el borrador o los adjuntos")
					}
					if !comp.Visible() {
						continue
					}
					for _, o := range comp.objPendientes {
						f := o.(*FichaPendiente)
						if f.Position().X < 0 || f.Position().X+f.Size().Width > comp.Size().Width {
							t.Fatalf("a %v una ficha sale del canal de %v: %v/%v", tam, comp.Size(), f.Position(), f.Size())
						}
						q := f.quitar
						if q.Position().X < 0 || q.Position().X+q.Size().Width > f.Size().Width || q.Size().Width < 44 || q.Size().Height < 44 {
							t.Fatalf("a %v Quitar no cabe o pierde tamaño táctil: %v/%v en %v", tam, q.Position(), q.Size(), f.Size())
						}
					}
				}
				if c.Compositor.Visible() {
					comprobarCabeceraReducida(t, w, c.Cabecera)
					if c.Cabecera.menu != nil {
						t.Fatal("el canal estrecho añadió una hamburguesa aunque el menú lateral ya está visible")
					}
				}
				if dir := os.Getenv("SYTUE_ROTACION_CAPTURAS"); dir != "" && tam.Width == 1024 {
					capturarCompositorEn(t, w, dir, fmt.Sprintf("native-rotacion-rooms-panel-%s-%gx%g", forma, tam.Width, tam.Height))
				}
			}
			comp := c.Compositor
			test.Tap(comp.objPendientes[0].(*FichaPendiente).quitar)
			if len(comp.Pendientes) != 1 {
				t.Fatal("Quitar no responde tras volver al canal estrecho")
			}
		})
	}
}

func comprobarCabeceraReducida(t *testing.T, w fyne.Window, c *Cabecera) {
	t.Helper()
	if !c.reducidaVisible {
		return
	}
	if c.reducida.Size().Width <= 0 {
		t.Fatal("la cabecera no tiene área para desplazar sus controles")
	}
	donde := fyne.CurrentApp().Driver().AbsolutePositionForObject
	for _, o := range c.reducida.Objetos {
		f, ok := o.(fyne.Focusable)
		if !ok {
			continue
		}
		w.Canvas().Focus(f)
		x := donde(o).X - donde(c.reducida).X
		if x < -0.01 || x+o.Size().Width > c.reducida.Size().Width+0.01 {
			t.Fatalf("un mando de cabecera no se alcanza con foco: %T %v/%v", o, o.Position(), o.Size())
		}
		if o.Size().Width < 44 || o.Size().Height < 44 {
			t.Fatalf("un mando de cabecera pierde su blanco táctil: %T %v", o, o.Size())
		}
	}
	c.reducida.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(1e6, 0)})
	if c.reducida.Desplazamiento() != 0 {
		t.Fatal("el gesto no devuelve la cabecera al principio")
	}
}

func TestElTemaEnEdicionSobreviveAlCanalEstrecho(t *testing.T) {
	t.Cleanup(ui.TactilDePrueba(true))
	c := NuevaCabecera(traductor(t), "es")
	c.Canal = Canal{ID: "equipo", Nombre: "equipo", Tema: "Tema anterior", Clase: Privado}
	c.EditandoTema = true
	c.Resize(fyne.NewSize(500, AltoCabecera))
	c.Refresh()
	ed := c.editorTema
	ed.PonerTexto("Tema que todavía no se ha guardado")
	for _, ancho := range []float32{156, 412, 156, 500} {
		c.Resize(fyne.NewSize(ancho, AltoCabecera))
		c.Refresh()
		if c.editorTema != ed || ed.Texto() != "Tema que todavía no se ha guardado" {
			t.Fatal("estrechar el canal recreó el campo y perdió lo escrito")
		}
		if c.reducidaVisible {
			if c.tituloReducido.Icono != iconos.Candado {
				t.Fatal("el canal privado perdió su indicador al estrecharse")
			}
			p, s := c.temaReducido.Position(), c.temaReducido.Size()
			if p.X < 0 || p.X+s.Width > c.reducida.Size().Width+0.01 {
				t.Fatal("el tema en edición queda fuera de la cabecera desplazable")
			}
		}
	}
	c.Canal = Canal{ID: "directo", Nombre: "Otra persona", Clase: Directo}
	c.EditandoTema = false
	c.Resize(fyne.NewSize(156, AltoCabecera))
	c.Refresh()
	if c.editorTema != nil || c.temaReducido != nil {
		t.Fatal("el directo conserva el tema del canal anterior")
	}
}

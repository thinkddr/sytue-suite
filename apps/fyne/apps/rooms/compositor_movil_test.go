// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestElCompositorConFormatoMantieneEnviarVisible(t *testing.T) {
	antes := tactilEnCompositor
	t.Cleanup(func() { tactilEnCompositor = antes })
	for _, tactil := range []bool{false, true} {
		for _, ancho := range []float32{320, 375, 390, 412} {
			t.Run(fmt.Sprintf("tactil=%v/ancho=%g", tactil, ancho), func(t *testing.T) {
				tactilEnCompositor = func() bool { return tactil }
				c := NuevoCompositor(traductor(t), "es")
				c.NombreCanal, c.FormatoVisible = "equipo", true
				enviado := ""
				c.AlEnviar = func(s string) { enviado = s }
				w := test.NewTempWindow(t, container.NewWithoutLayout(c))
				w.SetPadded(false)
				w.Resize(fyne.NewSize(ancho, 180))
				c.Resize(fyne.NewSize(ancho, c.Medida().Height))
				c.PonerTexto("Hola, equipo")
				if len(c.botones) != 5+len(marcasDeBarra) {
					t.Fatal("el caso no enseña todas las opciones de formato")
				}
				comprobarBarraCompositor(t, c, tactil)
				if c.barraLado.AnchoContenido() <= c.barraLado.Size().Width {
					t.Fatal("el caso no fuerza el desbordamiento de la tira")
				}
				d := fyne.CurrentApp().Driver()
				for _, b := range c.botones {
					// El foco real debe revelar el control, sin llamar al helper de
					// geometría desde el guardián como sustituto de Tab.
					f, ok := b.(fyne.Focusable)
					if !ok {
						t.Fatal("una acción no se puede enfocar con Tab")
					}
					w.Canvas().Focus(f)
					vista := d.AbsolutePositionForObject(b).X - d.AbsolutePositionForObject(c.barraLado).X
					if vista < -0.01 || vista+b.Size().Width > c.barraLado.Size().Width+0.01 {
						t.Errorf("un control no se alcanza desplazando: x=%v, ancho=%v, hueco=%v", vista, b.Size().Width, c.barraLado.Size().Width)
					}
				}
				if c.barraLado.Desplazamiento() <= 0 {
					t.Fatal("las últimas acciones no han desplazado la tira")
				}
				c.barraLado.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(1e5, 0)})
				if c.barraLado.Desplazamiento() != 0 {
					t.Fatal("el arrastre táctil no devuelve las primeras acciones")
				}
				c.barraLado.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(-1e5, 0)})
				if c.barraLado.Desplazamiento() <= 0 {
					t.Fatal("el arrastre táctil no alcanza las últimas acciones")
				}
				w.Canvas().Unfocus()
				comprobarBarraCompositor(t, c, tactil)
				capturarCompositor(t, w, fmt.Sprintf("compositor-formato-%v-%g", tactil, ancho))
				test.Tap(c.enviar)
				if enviado != "Hola, equipo" {
					t.Fatal("Enviar deja de responder al desplazar las acciones")
				}
			})
		}
	}
}

// A 1024 el canal queda entre el menú y el hilo. Marco deja además 10 a la
// derecha de la hoja; Carril va a sangre. Los dos compositores deben reservar la
// altura táctil real dentro de sus padres, conservando ambos laterales visibles.
func TestElCanalEntreDosLateralesReservaElCompositor(t *testing.T) {
	antes := tactilEnCompositor
	tactilEnCompositor = func() bool { return true }
	t.Cleanup(func() { tactilEnCompositor = antes })
	for _, forma := range []struct {
		nombre string
		borde  float32
	}{{"marco", 10}, {"carril", 0}} {
		t.Run(forma.nombre, func(t *testing.T) {
			tema.ElegirForma(forma.nombre)
			t.Cleanup(tema.FormaDeLosGuardianes)
			sa, _, w := salaEn(t, servicioConHilo(), 1024)
			sa.Arrancar(context.Background())
			sa.P.Canal.Compositor.FormatoVisible = true
			sa.P.Canal.Compositor.Refresh()
			sa.AbrirHilo(sa.P.Canal.Historial.Mensajes[3])
			c, h := sa.P.Canal.Compositor, sa.hilo.Compositor
			sa.P.Canal.Refresh()
			marco, canal, menu, hilo := sa.P.armazon, sa.P.Canal, sa.P.armazon.Menu, sa.hilo
			if !menu.Visible() || !hilo.Visible() || !c.Visible() || !h.Visible() || canal.Derecho != hilo || menu.Size().Width != tema.AnchoLateral || hilo.Size().Width != AnchoPanelDerecho {
				t.Fatal("el caso perdió uno de los dos laterales o de los compositores")
			}
			if canal.Position().X != menu.Position().X+menu.Size().Width || canal.Position().X+canal.Size().Width != marco.Size().Width-forma.borde {
				t.Fatalf("la hoja no ocupa lo que deja el menú y el borde: menú=%v/%v, canal=%v/%v, marco=%v", menu.Position(), menu.Size(), canal.Position(), canal.Size(), marco.Size())
			}
			if got, quiero := c.Size().Width, canal.Size().Width-hilo.Size().Width; got != quiero || hilo.Position().X != got || hilo.Position().X+hilo.Size().Width != canal.Size().Width {
				t.Fatalf("el canal y el hilo no comparten el ancho de su padre: compositor=%v, hilo=%v/%v, padre=%v", c.Size(), hilo.Position(), hilo.Size(), canal.Size())
			}
			for _, comp := range []*Compositor{c, h} {
				if comp.Size().Height != comp.Medida().Height {
					t.Errorf("el padre reserva %v y el compositor necesita %v", comp.Size().Height, comp.Medida().Height)
				}
				comprobarBarraCompositor(t, comp, true)
			}
			antesAlto := h.Size().Height
			h.PonerAvisoComando("Un aviso del comando")
			if h.Size().Height <= antesAlto || h.Size().Height != h.Medida().Height {
				t.Errorf("el hilo no reacciona a AlCambiarAlto: antes=%v, ahora=%v, medida=%v", antesAlto, h.Size().Height, h.Medida().Height)
			}
			capturarCompositor(t, w, "compositor-dos-laterales-1024-"+forma.nombre)
		})
	}
}

func comprobarBarraCompositor(t *testing.T, c *Compositor, tactil bool) {
	t.Helper()
	barra, enviar := c.barraLado, c.enviar
	if barra.Position().X+barra.Size().Width+HuecoBarra > enviar.Position().X+0.01 {
		t.Fatal("la tira desplazable invade el botón Enviar")
	}
	if enviar.Position().X < 0 || enviar.Position().X+enviar.Size().Width > c.Size().Width || enviar.Position().Y+enviar.Size().Height > c.Size().Height-AbajoFranja {
		t.Errorf("Enviar está recortado: %v/%v en %v", enviar.Position(), enviar.Size(), c.Size())
	}
	if tactil && (enviar.Size().Width < 44 || enviar.Size().Height < 44) {
		t.Errorf("Enviar pierde su objetivo táctil: %v", enviar.Size())
	}
	for _, b := range c.botones {
		if b.Position().Y < 0 || b.Position().Y+b.Size().Height > barra.Size().Height {
			t.Errorf("un control se recorta verticalmente: %v/%v, barra=%v", b.Position(), b.Size(), barra.Size())
		}
		if tactil && (b.Size().Width < 44 || b.Size().Height < 44) {
			t.Errorf("un control pierde su objetivo táctil: %v", b.Size())
		}
	}
}

func capturarCompositor(t *testing.T, w fyne.Window, nombre string) {
	t.Helper()
	capturarCompositorEn(t, w, os.Getenv("SYTUE_CAPTURAS"), nombre)
}

func capturarCompositorEn(t *testing.T, w fyne.Window, dir, nombre string) {
	t.Helper()
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, nombre+".png"))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, w.Canvas().Capture())
	if cerrar := f.Close(); err != nil || cerrar != nil {
		t.Fatalf("guardar compositor: %v / %v", err, cerrar)
	}
}

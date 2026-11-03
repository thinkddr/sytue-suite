// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

func TestCancelarElRellenoNoEscribeCeldas(t *testing.T) {
	c := lienzoDePrueba()
	r := c.Rejilla
	c.Libro.Escribir(0, 0, "1")
	r.Seleccionar(0, 0, false)
	pos, lado := r.asa()
	r.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: pos.AddXY(lado/2, lado/2)}})
	ev := &fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(AnchoCabecera+10, AltoCelda*5+5)}}
	r.Dragged(ev)
	if r.destRell == nil {
		t.Fatal("no se mostró el destino del relleno")
	}
	antes := c.Serializar()
	r.CancelarArrastre()
	r.Dragged(ev)
	r.DragEnd()
	if c.Serializar() != antes || r.destRell != nil || r.rellenando {
		t.Fatal("la cancelación escribió el relleno o dejó su previa")
	}
	// Cancelar el gesto no equivale a cancelar una edición de fórmula o celda.
	r.Editar("pendiente")
	r.CancelarArrastre()
	if !r.editando {
		t.Fatal("cancelar un arrastre cerró el editor de celda")
	}
	r.Cancelar()
}

func TestCancelarTarjetasNoMueveNiRedimensionaObjetos(t *testing.T) {
	for _, caso := range []struct {
		nombre string
		tamano bool
	}{{"mover gráfico", false}, {"redimensionar gráfico", true}} {
		t.Run(caso.nombre, func(t *testing.T) {
			c := lienzoDePrueba()
			if !c.Cargar(libroConGrafico) {
				t.Fatal("no se abrió el libro")
			}
			tarjeta := nuevaTarjetaGrafico(c, "g1")
			tarjeta.Resize(fyne.NewSize(400, 300))
			pos := fyne.NewPos(20, 10)
			if caso.tamano {
				pos = fyne.NewPos(399, 299)
			}
			ev := &fyne.DragEvent{PointEvent: fyne.PointEvent{Position: pos}, Dragged: fyne.NewDelta(10, 10)}
			antes := c.Serializar()
			tarjeta.Dragged(ev)
			if tarjeta.modo == gestoNada {
				t.Fatal("el gesto no empezó")
			}
			tarjeta.CancelarArrastre()
			tarjeta.Dragged(ev)
			tarjeta.DragEnd()
			if c.Serializar() != antes || tarjeta.modo != gestoNada {
				t.Fatal("cancelar guardó la posición o el tamaño")
			}
			tarjeta.Dragged(ev)
			tarjeta.DragEnd()
			if c.Serializar() == antes {
				t.Fatal("el gesto posterior quedó bloqueado")
			}
		})
	}
	c := lienzoDePrueba()
	c.Cargar(libroConGrafico)
	tarjeta := nuevaTarjetaSegmentacion(c, "s1")
	ev := &fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(20, 10)}, Dragged: fyne.NewDelta(10, 5)}
	antes := c.Serializar()
	tarjeta.Dragged(ev)
	tarjeta.CancelarArrastre()
	tarjeta.Dragged(ev)
	tarjeta.DragEnd()
	if c.Serializar() != antes || tarjeta.moviendo {
		t.Fatal("cancelar movió la segmentación")
	}
}

func TestCancelarUnaPestanaConservaElOrden(t *testing.T) {
	defer ui.TactilDePrueba(false)()
	c := NuevoLienzo(NuevoLibroCon([]string{"Uno", "Dos", "Tres"}, 0))
	c.Resize(fyne.NewSize(560, 300))
	pinta(t, 560, 300, c)
	tira := c.Pestanas.tira
	ph, destino := tira.pestanas[0], tira.pestanas[2]
	x := destino.Position().X + destino.Size().Width/2 - ph.Position().X
	ev := &fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(x, 10)}, Dragged: fyne.NewDelta(10, 0)}
	antes := slices.Clone(c.Libro.Hojas())
	ph.Dragged(ev)
	ph.CancelarArrastre()
	ph.Dragged(ev)
	ph.DragEnd()
	if !slices.Equal(c.Libro.Hojas(), antes) {
		t.Fatal("cancelar reordenó las hojas")
	}
	ph.Dragged(ev)
	ph.DragEnd()
	if slices.Equal(c.Libro.Hojas(), antes) {
		t.Fatal("el gesto nuevo quedó bloqueado")
	}
}

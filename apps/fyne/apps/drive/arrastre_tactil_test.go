// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

func TestElAsaTactilConservaElOrigenAlReciclarFilaYCancelaOtraVista(t *testing.T) {
	t.Cleanup(ui.TactilDePrueba(true))
	e := NuevoExplorador(traductor(t))
	e.Fase = Exito
	origen := Elemento{ID: "archivo", Nombre: "Contrato.pdf", Ruta: "/Contrato.pdf"}
	destino := Elemento{ID: "carpeta", Nombre: "Clientes", Carpeta: true, Ruta: "/Clientes"}
	e.Elementos = []Elemento{origen, destino}
	var movidos []Elemento
	e.AlMoverA = func(els []Elemento, d Elemento) {
		if d.ID != destino.ID {
			t.Fatal("se ha cambiado el destino")
		}
		movidos = append(movidos, els...)
	}
	revision := uint64(1)
	e.Refresco.Revision = func() uint64 { return revision }
	fila := e.nuevaFilaTactil()
	fila.el = origen
	tarjeta := NuevaTarjetaDrive(destino.Nombre, "", true)
	fila.Resize(fyne.NewSize(200, 60))
	tarjeta.Resize(fyne.NewSize(180, 180))
	tarjeta.Move(fyne.NewPos(220, 0))
	e.tarjetas = []tarjetaPintada{{el: destino, objeto: tarjeta, t: tarjeta}}
	w := test.NewWindow(container.NewWithoutLayout(fila, tarjeta))
	t.Cleanup(w.Close)
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(tarjeta).AddXY(50, 70)
	drag := &fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: pos}, Dragged: fyne.NewDelta(20, 10)}
	fila.asa.Dragged(drag)
	fila.el = Elemento{ID: "fila-reciclada"}
	fila.asa.Dragged(drag)
	fila.asa.DragEnd()
	if len(movidos) != 1 || movidos[0].ID != origen.ID {
		t.Fatalf("se ha movido otra fila durante el desplazamiento: %+v", movidos)
	}
	fila.el = origen
	fila.asa.Dragged(drag)
	revision++
	fila.asa.DragEnd()
	if len(movidos) != 1 || len(e.arrastrando) != 0 || e.destino != "" {
		t.Fatal("un gesto de otra vista mueve datos o deja el destino marcado")
	}
}

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

// En táctil la fila lleva el asa de arrastrar a la derecha y el nombre se queda el
// resto del ancho. Con `ui.FilaAuto` cada hijo se quedaba en su mínimo y el nombre del
// fichero no salía en ningún aparato táctil (auditoría estética del 2026-11-24).
//
// VISTO EN ROJO con el `FilaAuto{Derecha: 1}` de antes: la fila mide su mínimo.
func TestLaFilaTactilDeDriveDejaElAnchoAlNombre(t *testing.T) {
	t.Cleanup(ui.TactilDePrueba(true))
	e := NuevoExplorador(traductor(t))
	e.Fase = Exito
	fila := e.nuevaFilaTactil()
	fila.el = Elemento{ID: "a", Nombre: "Contrato de arrendamiento firmado.pdf", Ruta: "/Contrato.pdf"}
	w := test.NewWindow(fila)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 60))
	// Menos el asa y el hueco que deja el borde entre las dos (el relleno del tema).
	asa := fila.asa.Size().Width
	if got, quiero := fila.fila.Size().Width, float32(390)-asa; got < quiero-12 {
		t.Fatalf("la fila del nombre mide %v de %v (el asa, %v)", got, quiero, asa)
	}
	if fila.asa.Position().X+asa > 390+0.5 {
		t.Fatalf("el asa se sale por la derecha: x=%v ancho=%v", fila.asa.Position().X, asa)
	}
}

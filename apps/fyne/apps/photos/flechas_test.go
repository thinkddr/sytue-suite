// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

// TestLasFlechasRecorrenLaGaleria: las flechas van de foto en foto por su sitio en
// pantalla, ↓ desde la última fila de un mes baja al siguiente, y Fin lleva a la última
// foto y desplaza la página hasta ella. Es el `alTeclearEnGaleria` de la web
// (`galeria-flechas.test.tsx`).
func TestLasFlechasRecorrenLaGaleria(t *testing.T) {
	fotos := fotosDePrueba(40) // del 14 de septiembre hacia atrás: dos meses
	teca := tecaCargada(fotos)
	v := vistaDePrueba(t, teca)
	scroll := container.NewVScroll(container.NewStack())
	v.Desplaza = scroll
	var meses [][]fyne.CanvasObject
	grupos := PorMes(fotos, v.T)
	if len(grupos) < 2 {
		t.Fatalf("las fotos de prueba caen en %d meses; hacen falta dos", len(grupos))
	}
	scroll.Content = PantallaGaleria(v, Galeria{
		Grupos: grupos,
		AlMes: func(_, rej fyne.CanvasObject, _ GrupoMes) {
			meses = append(meses, rej.(*fyne.Container).Objects)
		},
	})
	w := test.NewWindow(scroll)
	defer w.Close()
	w.Resize(fyne.NewSize(ventanaAncho, ventanaAlto))

	todas := append(append([]fyne.CanvasObject{}, meses[0]...), meses[1]...)
	tesela := func(i int) *Miniatura { return todas[i].(*Miniatura) }
	foco := func() fyne.Focusable { return w.Canvas().Focused() }

	w.Canvas().Focus(tesela(0))
	tesela(0).TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if foco() != tesela(1) {
		t.Fatal("→ desde la primera foto no pasó a la segunda")
	}
	ultimaDelMes := len(meses[0]) - 1
	w.Canvas().Focus(tesela(ultimaDelMes))
	tesela(ultimaDelMes).TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	enfocada, _ := foco().(*Miniatura)
	if enfocada == nil || !contiene(meses[1], enfocada) {
		t.Fatal("↓ desde la última fila de un mes no bajó al siguiente")
	}
	enfocada.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	if foco() != tesela(len(todas)-1) {
		t.Fatal("Fin no fue a la última foto")
	}
	if scroll.Offset.Y <= 0 {
		t.Error("la última foto está fuera de la vista y la página no se desplazó")
	}
}

func contiene(objs []fyne.CanvasObject, o fyne.CanvasObject) bool {
	for _, x := range objs {
		if x == o {
			return true
		}
	}
	return false
}

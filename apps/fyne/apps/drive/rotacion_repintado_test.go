// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Una notificación de alto pendiente pertenece a la cabecera anterior. Su pila
// ya no está en la ventana, pero listaFranja se conserva en la nueva pantalla.
func TestLaRotacionNoDejaQueUnaCabeceraRetiradaRedimensioneLaLista(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	t.Cleanup(tema.FormaDeLosGuardianes)
	tema.ElegirForma("marco")
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana = Exito, 568
	e.Elementos = []Elemento{
		{ID: "c1", Nombre: "Actas", Carpeta: true, Tamano: -1},
		{ID: "f1", Nombre: "acta.md", Tamano: 10},
		{ID: "f2", Nombre: "b.pdf", Tamano: 2048},
	}
	anterior := e.Objeto(tema.Claro).(*fyne.Container)
	w := test.NewTempWindow(t, anterior)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(568, 320))
	// Guarda el aviso que ya podía estar en la cola antes de retirar la vista.
	cabecera := anterior.Objects[0].(*fyne.Container).Layout.(*CabeceraVista)
	notificacionPendiente := cabecera.AlCambiarAlto
	scroll := e.listaFranja
	if scroll == nil {
		t.Fatal("el fixture no montó el viewport de carpetas")
	}

	e.Ventana = 844
	actual := e.Objeto(tema.Claro)
	w.SetContent(actual)
	w.Resize(fyne.NewSize(844, 390))
	fyne.DoAndWait(func() {})
	if e.listaFranja != scroll {
		t.Fatal("repintar sustituyó el viewport que conserva el desplazamiento")
	}
	objetoDentroDeDrive(t, scroll, actual)
	tamano, posicion := scroll.Size(), scroll.Position()
	// No se espera al planificador: se reproduce exactamente el aviso de la vista
	// retirada después de que la vista nueva haya colocado el mismo viewport.
	if notificacionPendiente != nil {
		notificacionPendiente()
	}
	if scroll.Size() != tamano || scroll.Position() != posicion {
		t.Fatalf("la cabecera retirada cambió el viewport vivo: %v/%v; era %v/%v", scroll.Position(), scroll.Size(), posicion, tamano)
	}
	objetoDentroDeDrive(t, scroll, actual)
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func TestLaRotacionRetiraElLayoutAnteriorSinPerderElDesplazamiento(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	t.Cleanup(tema.FormaDeLosGuardianes)
	tema.ElegirForma("marco")
	tr, err := i18n.New("es", i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	barra, restaurar := ui.BarraSuiteDePrueba(tr, iconos.Drive)
	t.Cleanup(restaurar)
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana = Exito, 568
	for i := range 12 {
		e.Elementos = append(e.Elementos, Elemento{ID: fmt.Sprintf("c%d", i), Nombre: fmt.Sprintf("Carpeta %d", i), Carpeta: true, Tamano: -1})
	}
	for i := range 20 {
		e.Elementos = append(e.Elementos, Elemento{ID: fmt.Sprintf("f%d", i), Nombre: fmt.Sprintf("Archivo %d.txt", i)})
	}
	e.Seleccion["f0"] = true
	anterior := e.Objeto(tema.Claro)
	// El cromo real reserva la barra de la suite y el margen de la hoja. Sin
	// ellos el cuerpo de 844x390 mide 274: la franja no es compacta y el scroll
	// exterior no tiene nada que desplazar, aunque haya muchas carpetas.
	w := test.NewTempWindow(t, barra.Montar(ui.NuevoArmazon(anterior)))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(568, 320))

	e.Ventana = 844
	actual := e.Objeto(tema.Claro)
	w.SetContent(barra.Montar(ui.NuevoArmazon(actual)))
	w.Resize(fyne.NewSize(844, 390))
	fyne.DoAndWait(func() {})
	scroll := e.listaFranja
	if scroll == nil || e.filas == nil {
		t.Fatal("el fixture no montó los dos desplazamientos de la lista")
	}
	scroll.ScrollToBottom()
	e.filas.ScrollToBottom()
	if scroll.Offset.Y <= 0 || e.filas.GetScrollOffset() <= 0 {
		t.Fatalf("el fixture debe tener desplazamiento exterior e interior que conservar: exterior=%v interior=%g viewport=%v contenido=%v minimo=%v lista=%v", scroll.Offset, e.filas.GetScrollOffset(), scroll.Size(), scroll.Content.Size(), scroll.Content.MinSize(), e.filas.Size())
	}
	objetoDentroDeDrive(t, scroll, actual)
	tamano, posicion := scroll.Size(), scroll.Position()
	offset, offsetLista := scroll.Offset, e.filas.GetScrollOffset()
	e.buscador.PonerTexto("Informe trimestral")
	buscadorTam, buscadorPos := e.buscador.Size(), e.buscador.Position()

	// Una referencia anterior puede recibir todavía una recolocación. Su
	// cabecera conserva el modo de 568 px y pide 95, frente a los 55 de la viva.
	// El árbol retirado no debe prestar esa geometría al scroll actual.
	anterior.Resize(fyne.NewSize(844, 390))
	anterior.Refresh()
	if scroll.Size() != tamano || scroll.Position() != posicion {
		t.Fatalf("el árbol retirado redimensionó el viewport vivo: %v/%v; era %v/%v", scroll.Position(), scroll.Size(), posicion, tamano)
	}
	if scroll.Offset != offset || e.filas.GetScrollOffset() != offsetLista {
		t.Fatal("el árbol retirado alteró la posición de lectura de la lista")
	}
	if e.Marcados() != 1 || !e.Seleccion["f0"] {
		t.Fatal("el repintado perdió la selección de la persona")
	}
	if e.buscador.Texto() != "Informe trimestral" || e.buscador.Size() != buscadorTam || e.buscador.Position() != buscadorPos {
		t.Fatal("el árbol retirado alteró el texto o la geometría del buscador vivo")
	}
	objetoDentroDeDrive(t, scroll, actual)
}

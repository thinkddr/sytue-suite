// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// lienzoConMenu es el lienzo traducido en una ventana con sitio para el menú, con A1..A3
// rellenas.
func lienzoConMenu(t *testing.T) (*Lienzo, fyne.Window) {
	t.Helper()
	c := lienzoTraducido(t)
	for f, v := range []string{"uno", "dos", "tres"} {
		c.Libro.Escribir(0, f, v)
	}
	w := test.NewTempWindow(t, c)
	w.Resize(fyne.NewSize(900, 640))
	c.Rejilla.Refresh()
	return c, w
}

// puntoCelda, puntoCabCol y puntoCabFila son un punto dentro de la celda, de la cabecera
// de la columna y de la cabecera de la fila.
func puntoCelda(r *Rejilla, col, fila int) fyne.Position {
	vw := r.vista()
	return fyne.NewPos(vw.x(col)+5, vw.y(fila)+5)
}

func puntoCabCol(r *Rejilla, col int) fyne.Position {
	vw := r.vista()
	return fyne.NewPos(vw.x(col)+5, vw.cy*vw.z+AltoCelda*vw.z/2)
}

func puntoCabFila(r *Rejilla, fila int) fyne.Position {
	vw := r.vista()
	return fyne.NewPos(vw.cx*vw.z+AnchoCabecera*vw.z/2, vw.y(fila)+5)
}

// textosDe son los rótulos del menú en orden, con «—» por cada separador.
func textosDe(m *ui.Menu) []string {
	var out []string
	for _, o := range m.Elementos {
		if e, ok := o.(*ui.ElementoMenu); ok {
			out = append(out, e.Texto)
		} else {
			out = append(out, "—")
		}
	}
	return out
}

// TestElClicDerechoEnUnaCeldaAbreSuMenu: el clic derecho (y mantener pulsado, que Fyne
// entrega igual) fuera del rango selecciona la celda y abre el menú de las celdas, con
// las opciones de la cinta; una opción manda su comando y el foco vuelve a la rejilla.
func TestElClicDerechoEnUnaCeldaAbreSuMenu(t *testing.T) {
	c, w := lienzoConMenu(t)
	r := c.Rejilla
	test.TapSecondaryAt(r, puntoCelda(r, 0, 1))
	m := c.MenuContextual()
	if m == nil {
		t.Fatal("el clic derecho sobre una celda no abre el menú")
	}
	if m.Rotulo != "Opciones de la celda" {
		t.Errorf("el menú se anuncia como %q", m.Rotulo)
	}
	if r.Seleccion() != (Celda{C: 0, F: 1}) {
		t.Errorf("el clic derecho no selecciona la celda pulsada: %+v", r.Seleccion())
	}
	want := []string{
		"Cortar", "Copiar", "Pegar", "Pegar valores", "—",
		"Insertar fila encima", "Insertar fila debajo", "Insertar columna a la izquierda", "Insertar columna a la derecha",
		"Eliminar filas", "Eliminar columnas", "Borrar contenido", "—",
		"Ordenar de menor a mayor", "Ordenar de mayor a menor", "—", "Congelar hasta aquí",
	}
	if got := textosDe(m); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("las opciones de la celda son\n%v\ny se esperaban\n%v", got, want)
	}
	guarda(t, w.Canvas().Capture()) // con `SYTUE_CAPTURA`, el menú abierto
	test.Tap(elementoMenu(t, m, "Insertar fila encima"))
	if c.MenuContextual() != nil {
		t.Error("el menú sigue abierto tras elegir una opción")
	}
	if got := c.Libro.Texto(0, 2); got != "dos" {
		t.Errorf("«Insertar fila encima» no baja la fila: A3 = %q", got)
	}
	if got := c.Libro.Texto(0, 1); got != "" {
		t.Errorf("la fila insertada no está vacía: A2 = %q", got)
	}
	if !c.Libro.PuedeDeshacer() {
		t.Error("lo hecho desde el menú no se puede deshacer")
	}
	if w.Canvas().Focused() != r {
		t.Error("el foco no vuelve a la rejilla al cerrar el menú")
	}
}

// TestElClicDerechoDentroDelRangoLoConserva: como en Excel, el clic derecho dentro del
// rango no lo deshace, y «Borrar contenido» vacía el rango entero.
func TestElClicDerechoDentroDelRangoLoConserva(t *testing.T) {
	c, _ := lienzoConMenu(t)
	r := c.Rejilla
	r.Seleccionar(0, 0, false)
	r.Seleccionar(0, 2, true)
	test.TapSecondaryAt(r, puntoCelda(r, 0, 1))
	if c1, f1, c2, f2 := r.Rango(); c1 != 0 || f1 != 0 || c2 != 0 || f2 != 2 {
		t.Fatalf("el clic derecho dentro del rango lo cambia a %d,%d:%d,%d", c1, f1, c2, f2)
	}
	test.Tap(elementoMenu(t, c.MenuContextual(), "Borrar contenido"))
	for f := range 3 {
		if got := c.Libro.Texto(0, f); got != "" {
			t.Errorf("«Borrar contenido» deja A%d = %q", f+1, got)
		}
	}
}

// TestLasCabecerasTienenSuVariante: la cabecera de columna selecciona la columna entera
// y ofrece solo lo de columnas (con ordenar); la de fila, solo lo de filas (sin ordenar).
func TestLasCabecerasTienenSuVariante(t *testing.T) {
	c, _ := lienzoConMenu(t)
	r := c.Rejilla
	test.TapSecondaryAt(r, puntoCabCol(r, 1))
	m := c.MenuContextual()
	if m == nil || m.Rotulo != "Opciones de la columna" {
		t.Fatalf("la cabecera de columna no abre su menú: %v", m)
	}
	if c1, f1, c2, f2 := r.Rango(); c1 != 1 || c2 != 1 || f1 != 0 || f2 != r.Fils-1 {
		t.Errorf("no selecciona la columna B entera: %d,%d:%d,%d", c1, f1, c2, f2)
	}
	cols := strings.Join(textosDe(m), "|")
	if strings.Contains(cols, "fila") || !strings.Contains(cols, "Eliminar columnas") || !strings.Contains(cols, "Ordenar") {
		t.Errorf("el menú de la columna lleva %s", cols)
	}
	test.Tap(elementoMenu(t, m, "Insertar columna a la izquierda"))
	if got := c.Libro.Texto(0, 0); got != "uno" {
		t.Errorf("insertar a la izquierda de B mueve la A: A1 = %q", got)
	}

	test.TapSecondaryAt(r, puntoCabFila(r, 2))
	m = c.MenuContextual()
	if m == nil || m.Rotulo != "Opciones de la fila" {
		t.Fatalf("la cabecera de fila no abre su menú: %v", m)
	}
	if c1, f1, c2, f2 := r.Rango(); f1 != 2 || f2 != 2 || c1 != 0 || c2 != r.Cols-1 {
		t.Errorf("no selecciona la fila 3 entera: %d,%d:%d,%d", c1, f1, c2, f2)
	}
	filas := strings.Join(textosDe(m), "|")
	if strings.Contains(filas, "columna") || strings.Contains(filas, "Ordenar") || !strings.Contains(filas, "Eliminar filas") {
		t.Errorf("el menú de la fila lleva %s", filas)
	}
	test.Tap(elementoMenu(t, m, "Eliminar filas"))
	if got := c.Libro.Texto(0, 2); got != "" {
		t.Errorf("«Eliminar filas» no quita la fila 3: A3 = %q", got)
	}
}

// TestLaHojaProtegidaDejaLasOpcionesInertesConSuMotivo: lo que la protección veta sale
// en el menú, pero inerte y diciendo por qué; copiar sigue vivo.
func TestLaHojaProtegidaDejaLasOpcionesInertesConSuMotivo(t *testing.T) {
	c, _ := lienzoConMenu(t)
	c.Libro.Proteger(&Proteccion{Sort: true}, "")
	r := c.Rejilla
	test.TapSecondaryAt(r, puntoCelda(r, 0, 0))
	m := c.MenuContextual()
	if m == nil {
		t.Fatal("con la hoja protegida el menú no se abre")
	}
	if elementoMenu(t, m, "Copiar").Inerte {
		t.Error("copiar se puede siempre")
	}
	if elementoMenu(t, m, "Ordenar de menor a mayor").Inerte {
		t.Error("ordenar tiene su permiso concedido y sale inerte")
	}
	for _, texto := range []string{"Cortar", "Pegar", "Insertar fila encima", "Eliminar columnas", "Borrar contenido"} {
		e := elementoMenu(t, m, texto)
		if !e.Inerte {
			t.Errorf("«%s» no sale inerte con la hoja protegida", texto)
		}
		if want := texto + " (no disponible: Hoja protegida)"; e.Rotulo != want {
			t.Errorf("«%s» se anuncia como %q", texto, e.Rotulo)
		}
	}
	m.Cerrar()
	r.SoloLectura = true
	test.TapSecondaryAt(r, puntoCelda(r, 0, 0))
	if e := elementoMenu(t, c.MenuContextual(), "Congelar hasta aquí"); !e.Inerte || !strings.Contains(e.Rotulo, "Solo lectura") {
		t.Errorf("en solo lectura «Congelar» queda vivo o sin motivo: %q", e.Rotulo)
	}
}

// TestMayusF10YLaTeclaMenuAbrenElMenu: el teclado llega al mismo menú (WCAG 2.1.1),
// colgando de la celda activa; Escape lo cierra y deja el foco en la rejilla.
func TestMayusF10YLaTeclaMenuAbrenElMenu(t *testing.T) {
	c, w := lienzoConMenu(t)
	r := c.Rejilla
	w.Canvas().Focus(r)
	r.Seleccionar(1, 2, false)
	r.TypedKey(&fyne.KeyEvent{Name: fyne.KeyF10})
	if c.MenuContextual() != nil {
		t.Fatal("F10 sin Mayús abre el menú")
	}
	r.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	r.TypedKey(&fyne.KeyEvent{Name: fyne.KeyF10})
	r.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	m := c.MenuContextual()
	if m == nil {
		t.Fatal("Mayús+F10 no abre el menú")
	}
	if r.Seleccion() != (Celda{C: 1, F: 2}) {
		t.Errorf("abrir con el teclado mueve la selección a %+v", r.Seleccion())
	}
	vw := r.vista()
	if got := r.anclaMenu.Position(); got != fyne.NewPos(vw.x(1), vw.y(2)) {
		t.Errorf("el menú no cuelga de la celda activa: ancla en %v", got)
	}
	foco, ok := w.Canvas().Focused().(*ui.ElementoMenu)
	if !ok {
		t.Fatalf("el foco no entra en el menú: %T", w.Canvas().Focused())
	}
	foco.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if c.MenuContextual() != nil {
		t.Error("Escape no cierra el menú")
	}
	if w.Canvas().Focused() != r {
		t.Errorf("tras Escape el foco está en %T, no en la rejilla", w.Canvas().Focused())
	}

	r.SeleccionarTodo()
	r.Seleccionar(0, 0, false)
	r.pulsar(zonaCabFila, Celda{F: 1})
	r.TypedKey(&fyne.KeyEvent{Name: desktop.KeyMenu})
	if m := c.MenuContextual(); m == nil || m.Rotulo != "Opciones de la fila" {
		t.Fatalf("la tecla Menú con una fila entera no abre el menú de la fila: %v", m)
	}
}

// TestCongelarHastaAquiYDescongelar: «Congelar hasta aquí» inmoviliza por encima y a la
// izquierda de la celda, como el botón de la cinta; con algo inmovilizado, el menú
// ofrece descongelar. En A1 congela la primera fila, como el botón; en la cabecera de
// la columna A no hay nada que congelar y sale inerte.
func TestCongelarHastaAquiYDescongelar(t *testing.T) {
	c, _ := lienzoConMenu(t)
	r := c.Rejilla
	test.TapSecondaryAt(r, puntoCabCol(r, 0))
	if e := elementoMenu(t, c.MenuContextual(), "Congelar hasta aquí"); !e.Inerte || e.Rotulo != "" {
		t.Errorf("en la columna A «Congelar» sale vivo o con un motivo que no hay: %q", e.Rotulo)
	}
	c.MenuContextual().Cerrar()
	test.TapSecondaryAt(r, puntoCelda(r, 0, 0))
	test.Tap(elementoMenu(t, c.MenuContextual(), "Congelar hasta aquí"))
	if got := c.Libro.Fijas(); got != (Fijas{Rows: 1}) {
		t.Fatalf("en A1 congela %+v y no la primera fila, como el botón de la cinta", got)
	}
	c.Mandar(CmdFijar, "0,0")
	test.TapSecondaryAt(r, puntoCelda(r, 2, 3))
	test.Tap(elementoMenu(t, c.MenuContextual(), "Congelar hasta aquí"))
	if got := c.Libro.Fijas(); got != (Fijas{Cols: 2, Rows: 3}) {
		t.Fatalf("congela %+v y no las dos columnas y tres filas de encima", got)
	}
	test.TapSecondaryAt(r, puntoCelda(r, 2, 3))
	test.Tap(elementoMenu(t, c.MenuContextual(), "Descongelar paneles"))
	if got := c.Libro.Fijas(); got != (Fijas{}) {
		t.Errorf("«Descongelar paneles» deja %+v", got)
	}
}

// TestPegarValoresDesdeElMenu: copiar y pegar solo valores llegan al portapapeles por el
// mismo camino que Ctrl+C y Ctrl+Mayús+V.
func TestPegarValoresDesdeElMenu(t *testing.T) {
	c, _ := lienzoConMenu(t)
	r := c.Rejilla
	c.Libro.Escribir(1, 0, "=1+1")
	test.TapSecondaryAt(r, puntoCelda(r, 1, 0))
	test.Tap(elementoMenu(t, c.MenuContextual(), "Copiar"))
	test.TapSecondaryAt(r, puntoCelda(r, 2, 0))
	test.Tap(elementoMenu(t, c.MenuContextual(), "Pegar valores"))
	if got := c.Libro.HTML(2, 0); got != "2" {
		t.Errorf("«Pegar valores» deja C1 = %q y no el valor 2", got)
	}
}

// TestMayusF10NoDejaLaMayusculaPegada: con Mayús+F10 el foco salta al menú antes de que
// se suelte Mayús, así que la rejilla no ve ese KeyUp; al volver, las flechas no pueden
// seguir extendiendo el rango como si Mayús siguiera pulsada.
func TestMayusF10NoDejaLaMayusculaPegada(t *testing.T) {
	c, w := lienzoConMenu(t)
	r := c.Rejilla
	w.Canvas().Focus(r)
	r.Seleccionar(1, 1, false)
	r.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	r.TypedKey(&fyne.KeyEvent{Name: fyne.KeyF10})
	foco, ok := w.Canvas().Focused().(*ui.ElementoMenu)
	if !ok {
		t.Fatalf("el foco no entra en el menú: %T", w.Canvas().Focused())
	}
	// Mayús se suelta ya en el menú, que no tiene KeyUp: la rejilla no se entera.
	foco.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if w.Canvas().Focused() != r {
		t.Fatalf("tras Escape el foco está en %T", w.Canvas().Focused())
	}
	r.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	if c1, f1, c2, f2 := r.Rango(); c1 != 1 || f1 != 2 || c2 != 1 || f2 != 2 {
		t.Errorf("la flecha tras el menú extiende el rango a %d,%d:%d,%d, como con Mayús", c1, f1, c2, f2)
	}
}

// TestMayusClicDerechoNoDejaLaMayusculaPegada: Mayús+clic derecho no deja la Mayús del
// ratón puesta para la flecha siguiente.
func TestMayusClicDerechoNoDejaLaMayusculaPegada(t *testing.T) {
	c, w := lienzoConMenu(t)
	r := c.Rejilla
	w.Canvas().Focus(r)
	r.Seleccionar(1, 1, false)
	p := puntoCelda(r, 1, 1)
	r.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: p}, Button: desktop.MouseButtonSecondary, Modifier: fyne.KeyModifierShift})
	r.MouseUp(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: p}, Button: desktop.MouseButtonSecondary, Modifier: fyne.KeyModifierShift})
	r.TappedSecondary(&fyne.PointEvent{Position: p})
	c.MenuContextual().Cerrar()
	r.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	if c1, f1, c2, f2 := r.Rango(); c1 != 1 || f1 != 2 || c2 != 1 || f2 != 2 {
		t.Errorf("la flecha tras Mayús+clic derecho extiende el rango a %d,%d:%d,%d", c1, f1, c2, f2)
	}
}

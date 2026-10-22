// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestOrdenarEsElDeLaWeb: por la columna de la celda ancla, las vacías al final, las
// filas enteras se mueven con su formato, y una celda suelta ordena lo usado.
func TestOrdenarEsElDeLaWeb(t *testing.T) {
	c := lienzoDePrueba()
	l := c.Libro
	for f, fila := range [][2]string{{"c", "3"}, {"a", ""}, {"b", "1"}, {"d", "texto"}} {
		l.Escribir(0, f, fila[0])
		l.Escribir(1, f, fila[1])
	}
	l.CambiarEstilo(conRelleno("#FEF08A"), 0, 2, 0, 2) // la fila de «b»
	c.Rejilla.Seleccionar(1, 0, false)                 // ancla en la columna B, celda suelta
	c.Mandar(CmdOrdenar, "asc")
	var got string
	for f := range 4 {
		got += l.Crudo(0, f)
	}
	// Números antes que textos y las vacías al final: 1 (b), 3 (c), «texto» (d), vacía (a).
	if got != "bcda" {
		t.Errorf("ordenar por B deja la columna A como %q y la web deja «bcda»", got)
	}
	if l.Estilo(0, 0).Relleno != "#FEF08A" {
		t.Error("el relleno de la fila de «b» no ha viajado con ella")
	}
	c.Mandar(CmdOrdenar, "desc")
	if l.Crudo(0, 0) != "d" {
		t.Errorf("descendente empieza por %q", l.Crudo(0, 0))
	}
	c.Deshacer(false)
	if l.Crudo(0, 0) != "b" {
		t.Error("ordenar no se deshace en un paso")
	}
}

// TestInsertarFilaMueveLasFormulas: la regla que más importa de la pestaña Datos. Una
// fila insertada DENTRO del rango de una suma lo estira; borrar lo que señalaba una
// fórmula da #¡REF!, no un cero.
func TestInsertarFilaMueveLasFormulas(t *testing.T) {
	c := lienzoDePrueba()
	l := c.Libro
	for f := range 5 {
		l.Escribir(0, f, "1")
	}
	l.Escribir(1, 0, "=SUM(A1:A5)")
	l.PonerFormatoNumero("currency", "", 0, 4, 0, 4)
	c.Rejilla.Seleccionar(0, 2, false)
	c.Mandar(CmdInsFila, "above")
	if got := l.Crudo(1, 0); got != "=SUM(A1:A6)" {
		t.Errorf("insertar una fila dentro del rango deja %q", got)
	}
	if l.Crudo(0, 2) != "" || l.Crudo(0, 5) != "1" || l.FormatoNumero(0, 5).ID != "currency" {
		t.Errorf("la fila no se ha abierto: A3=%q A6=%q (formato %q)", l.Crudo(0, 2), l.Crudo(0, 5), l.FormatoNumero(0, 5).ID)
	}
	l.Escribir(2, 0, "=A6*2")
	c.Rejilla.Seleccionar(0, 5, false)
	c.Mandar(CmdBorrarFilas, "")
	if got := l.Crudo(2, 0); got != "=#¡REF!*2" {
		t.Errorf("borrar la fila que señalaba una fórmula la deja %q", got)
	}
	// El extremo de un rango que cae en lo borrado se pega al borde de lo que queda
	// (`mover(…, 'sujeta')` en refshift.ts): A6 borrada deja el rango en A6, que ahora es
	// la fila que subió. Es lo que hace la web, y el mismo documento no puede dar dos
	// sumas distintas.
	if got := l.Crudo(1, 0); got != "=SUM(A1:A6)" {
		t.Errorf("borrar la última fila del rango lo deja %q y la web deja A1:A6", got)
	}
	c.Rejilla.Seleccionar(0, 0, false)
	c.Mandar(CmdInsCol, "left")
	if got := l.Crudo(2, 0); got != "=SUM(B1:B6)" {
		t.Errorf("insertar una columna a la izquierda deja la suma (ahora en C1) como %q", got)
	}
}

// TestQuitarDuplicadosYTextoEnColumnas: se queda la primera de cada grupo y se avisa
// de cuántas se han quitado; el texto se reparte hacia la derecha sin tocar fórmulas.
func TestQuitarDuplicadosYTextoEnColumnas(t *testing.T) {
	c := lienzoDePrueba()
	l := c.Libro
	for f, v := range []string{"x", "y", "x", "z", "y"} {
		l.Escribir(0, f, v)
	}
	var aviso string
	c.AlAviso = func(_ ui.TonoAviso, _, detalle string) { aviso = detalle }
	c.Rejilla.Seleccionar(0, 0, false)
	c.Rejilla.Seleccionar(0, 4, true)
	c.Mandar(CmdDuplicados, "")
	var got string
	for f := range 5 {
		got += l.Crudo(0, f) + "|"
	}
	if got != "x|y|z|||" {
		t.Errorf("quitar duplicados deja %q", got)
	}
	if aviso == "" {
		t.Error("no se ha avisado de cuántas filas se han quitado")
	}
	if _, f1, _, f2 := c.Rejilla.Rango(); f1 != 0 || f2 != 2 {
		t.Errorf("queda seleccionado %d..%d y tienen que ser las tres que quedan", f1, f2)
	}

	l.Escribir(3, 0, "uno; dos ;tres")
	l.Escribir(3, 1, "=A1&\";\"")
	c.Rejilla.Seleccionar(3, 0, false)
	c.Rejilla.Seleccionar(3, 1, true)
	c.Mandar(CmdTextoCols, ";")
	if l.Crudo(3, 0) != "uno" || l.Crudo(4, 0) != "dos" || l.Crudo(5, 0) != "tres" {
		t.Errorf("texto en columnas deja %q %q %q", l.Crudo(3, 0), l.Crudo(4, 0), l.Crudo(5, 0))
	}
	if l.Crudo(3, 1) != "=A1&\";\"" {
		t.Error("texto en columnas ha partido una fórmula")
	}
}

// TestElAsaRellenaEnMosaico: arrastrar el asa copia el origen en mosaico hacia donde se
// arrastra —sin series—, desplaza las referencias relativas y deja seleccionado todo.
func TestElAsaRellenaEnMosaico(t *testing.T) {
	c := lienzoDePrueba()
	r := c.Rejilla
	c.Libro.Escribir(0, 0, "1")
	c.Libro.Escribir(0, 1, "=A1*2")
	c.Libro.PonerFormatoNumero("currency", "", 0, 1, 0, 1)
	r.Seleccionar(0, 0, false)
	r.Seleccionar(0, 1, true) // A1:A2
	pos, lado := r.asa()
	r.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: pos.AddXY(lado/2, lado/2)}})
	r.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(AnchoCabecera+10, AltoCelda+5*AltoCelda+5)}})
	if r.destRell == nil || *r.destRell != [4]int{0, 2, 0, 5} {
		t.Fatalf("arrastrar el asa hasta A6 marca %v", r.destRell)
	}
	r.DragEnd()
	got := make([]string, 0, 6)
	for f := range 6 {
		got = append(got, c.Libro.Crudo(0, f))
	}
	if strings.Join(got, "|") != "1|=A1*2|1|=A3*2|1|=A5*2" {
		t.Errorf("el relleno deja %v", got)
	}
	if c.Libro.FormatoNumero(0, 3).ID != "currency" {
		t.Error("el formato de número no ha viajado con el relleno")
	}
	if c1, f1, c2, f2 := r.Rango(); c1 != 0 || f1 != 0 || c2 != 0 || f2 != 5 {
		t.Errorf("queda seleccionado %d,%d..%d,%d y tiene que ser A1:A6", c1, f1, c2, f2)
	}
	c.Deshacer(false)
	if c.Libro.Crudo(0, 3) != "" {
		t.Error("el relleno no se deshace en un paso")
	}
}

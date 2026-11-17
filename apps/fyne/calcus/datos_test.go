// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
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
	// El FIN de un rango que cae en lo borrado se pega a la última fila que queda antes
	// del corte (`sujetaFin` en refshift.ts y reescribir.go): A6 borrada deja el rango en
	// A1:A5, como Excel. Antes quedaba en A6, que ya era la fila que subió, un dato ajeno.
	if got := l.Crudo(1, 0); got != "=SUM(A1:A5)" {
		t.Errorf("borrar la última fila del rango lo deja %q y la web deja A1:A5", got)
	}
	c.Rejilla.Seleccionar(0, 0, false)
	c.Mandar(CmdInsCol, "left")
	if got := l.Crudo(2, 0); got != "=SUM(B1:B5)" {
		t.Errorf("insertar una columna a la izquierda deja la suma (ahora en C1) como %q", got)
	}
}

// libroConReglas tiene de todo lo que cuelga de las filas y las columnas: formato
// condicional (con fórmula anclada a su primera celda), validaciones, esquema, anchos,
// altos y combinadas.
const libroConReglas = `{"active":0,"sheets":[{"name":"H","cells":{"B2":"1"},` +
	`"condFormats":[{"range":"B2:B10","kind":"formula","formula":"=B2>$D$1","fill":"#FFEB9C"}],` +
	`"validations":[{"range":"B2:B10","values":["Sí","No"]}],` +
	`"rowGroups":[{"start":1,"end":4,"level":1}],"colGroups":[{"start":1,"end":2,"level":1}],` +
	`"colWidths":{"1":120},"rowHeights":{"4":30},"merges":["A1:C1","B5:B6"]}]}`

// TestLaEstructuraMueveReglasEsquemaYCombinadas: insertar o borrar filas y columnas mueve
// el formato condicional, las validaciones, el esquema, los anchos, los altos y las
// combinadas con las reglas de las fórmulas, como la web; antes se quedaban en las celdas
// de antes, pintando y vetando filas ajenas sin avisar.
func TestLaEstructuraMueveReglasEsquemaYCombinadas(t *testing.T) {
	l := libroDePrueba(t, libroConReglas).Libro
	l.Estructura("insrow", "above", 0, 0, 0, 0)
	h := l.h()
	if c := h.condiciones[0]; c.Rango != "B3:B11" || c.Formula != "=B3>$D$2" {
		t.Errorf("tras insertar una fila encima, la regla es %+v", c)
	}
	if v := h.validaciones[0]; v.Rango != "B3:B11" {
		t.Errorf("tras insertar una fila encima, la validación es %+v", v)
	}
	if g := h.grupFilas[0]; g.Start != 2 || g.End != 5 {
		t.Errorf("el grupo de filas no baja: %+v", g)
	}
	if h.altos["5"] != 30 || len(h.altos) != 1 || h.fusiones[0] != "A2:C2" || h.fusiones[1] != "B6:B7" {
		t.Errorf("altos %v y combinadas %v no siguen a sus filas", h.altos, h.fusiones)
	}
	// Borrar su primera fila: la fórmula pasa a la nueva primera celda, no a #¡REF!.
	l.Estructura("delrow", "", 0, 2, 0, 2)
	if c := l.h().condiciones[0]; c.Rango != "B3:B10" || c.Formula != "=B3>$D$2" {
		t.Errorf("tras borrar su primera fila, la regla es %+v", c)
	}
	// Borrar todas sus filas la elimina.
	l.Estructura("delrow", "", 0, 2, 0, 9)
	if h := l.h(); len(h.condiciones) != 0 || len(h.validaciones) != 0 {
		t.Errorf("borrar todas sus filas no las elimina: %+v %+v", h.condiciones, h.validaciones)
	}
	l.Deshacer()
	l.Deshacer()
	l.Deshacer()
	l.Estructura("inscol", "left", 0, 0, 0, 0)
	h = l.h()
	if g := h.grupCols[0]; g.Start != 2 || g.End != 3 || h.anchos["2"] != 120 || h.fusiones[0] != "B1:D1" {
		t.Errorf("tras insertar una columna: grupo %+v, anchos %v, combinadas %v", g, h.anchos, h.fusiones)
	}
	if c := h.condiciones[0]; c.Rango != "C2:C10" || c.Formula != "=C2>$E$1" {
		t.Errorf("tras insertar una columna, la regla es %+v", c)
	}
}

// TestLaEstructuraNoTocaLasReglasDeUnaHojaProtegida: sin el permiso `objects`, el servidor
// veta cambiar el formato condicional y las validaciones; moverlos haría que rechazara el
// guardado entero. El esquema y las combinadas, que no gobierna ningún permiso, sí se mueven.
func TestLaEstructuraNoTocaLasReglasDeUnaHojaProtegida(t *testing.T) {
	l := libroDePrueba(t, libroConReglas).Libro
	l.Proteger(&Proteccion{InsertRows: true}, "")
	l.Estructura("insrow", "above", 0, 0, 0, 0)
	h := l.h()
	if h.condiciones[0].Rango != "B2:B10" || h.validaciones[0].Rango != "B2:B10" {
		t.Errorf("con la hoja protegida se han movido: %+v %+v", h.condiciones, h.validaciones)
	}
	if h.grupFilas[0].Start != 2 {
		t.Errorf("el esquema no gobernado por permisos no baja: %+v", h.grupFilas)
	}
	l.Proteger(&Proteccion{InsertRows: true, Objects: true}, "")
	l.Estructura("insrow", "above", 0, 0, 0, 0)
	if c := l.h().condiciones[0]; c.Rango != "B3:B11" {
		t.Errorf("con el permiso de objetos concedido no se mueve: %+v", c)
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

// TestLaEstructuraMueveLasFormulasDeLasOtrasHojas: insertar o borrar filas y columnas en
// «Ventas» reescribe las fórmulas de «Resumen» que la citan —celdas, rangos, nombres con
// comillas, `$`, formato condicional y validaciones—, como la web (refshift.ts); y en
// «Ventas», una fórmula que cita «Resumen» se queda. Antes «Resumen» seguía mirando la
// celda vieja sin avisar, y la hoja activa movía también las referencias a otras hojas.
func TestLaEstructuraMueveLasFormulasDeLasOtrasHojas(t *testing.T) {
	l := libroDePrueba(t, `{"active":0,"sheets":[`+
		`{"name":"Ventas","cells":{"B4":"7","C1":"=B4+Resumen!A1"}},`+
		`{"name":"Resumen","cells":{"A1":"=Ventas!B4","A2":"=SUM(Ventas!A1:B9)","A3":"=ventas!$B$4*B4","A4":"='Hoja 2'!C3+Hoja2!C3"},`+
		`"condFormats":[{"range":"A1:A3","kind":"formula","formula":"=A1>Ventas!$B$4","fill":"#FFEB9C"}],`+
		`"validations":[{"range":"C1","type":"formula","formula":"=COUNTIF(Ventas!$A$1:$A$9,C1)>0"}]},`+
		`{"name":"Hoja 2","cells":{"A1":"='Ventas'!B4+Ventas!A1"},"protect":{},"styles":{}}]}`).Libro
	celda := func(hoja int, ref string) string {
		r, _ := formula.ParseRangeRef(ref)
		return textoPlano(l.f.hojas[hoja].celdas[Celda{C: r.C1, F: r.R1}])
	}
	l.Estructura("insrow", "above", 0, 1, 0, 1) // una fila encima de la 2
	for _, c := range []struct {
		hoja        int
		ref, quiero string
	}{
		{0, "C1", "=B5+Resumen!A1"},
		{1, "A1", "=Ventas!B5"},
		{1, "A2", "=SUM(Ventas!A1:B10)"},
		{1, "A3", "=ventas!$B$5*B4"},
		// «Hoja 2» está protegida y su celda, bloqueada: sigue a su dato igual, porque no
		// es editarla y el servidor lo acepta (proteccion_ajenas.go).
		{2, "A1", "='Ventas'!B5+Ventas!A1"},
	} {
		if got := celda(c.hoja, c.ref); got != c.quiero {
			t.Errorf("tras insertar una fila, %d!%s es %q y la web da %q", c.hoja, c.ref, got, c.quiero)
		}
	}
	r := l.f.hojas[1]
	if r.condiciones[0].Rango != "A1:A3" || r.condiciones[0].Formula != "=A1>Ventas!$B$5" {
		t.Errorf("la regla de «Resumen» es %+v", r.condiciones[0])
	}
	if f := r.validaciones[0].Formula; f != "=COUNTIF(Ventas!$A$1:$A$10,C1)>0" {
		t.Errorf("la validación de «Resumen» es %q", f)
	}
	// Borrar la fila del total: quien la citaba pasa a #¡REF!, como en la propia hoja.
	l.Estructura("delrow", "", 0, 4, 0, 4)
	if got := celda(1, "A1"); got != "="+formula.RefRota {
		t.Errorf("tras borrar la fila referida, Resumen!A1 es %q", got)
	}
	if got := celda(1, "A2"); got != "=SUM(Ventas!A1:B9)" {
		t.Errorf("tras borrar una fila del rango, Resumen!A2 es %q", got)
	}
	// Deshacer lo devuelve todo, también en las otras hojas.
	l.Deshacer()
	l.Deshacer()
	if got := celda(1, "A1"); got != "=Ventas!B4" {
		t.Errorf("deshacer no devuelve Resumen!A1: %q", got)
	}
	// Una columna en una hoja con espacio en el nombre: se mueve lo que la cita con sus
	// comillas, y no lo que cita «Ventas».
	l.Activar(2)
	l.Estructura("inscol", "left", 1, 0, 1, 0)
	if got := celda(1, "A4"); got != "='Hoja 2'!D3+Hoja2!C3" {
		t.Errorf("insertar una columna en «Hoja 2»: Resumen!A4 es %q", got)
	}
	if got := celda(1, "A1"); got != "=Ventas!B4" {
		t.Errorf("insertar en «Hoja 2» mueve lo que cita «Ventas»: %q", got)
	}
}

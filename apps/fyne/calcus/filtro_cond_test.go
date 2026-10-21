// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"strings"
	"testing"
)

// libroFiltrado es una hoja con cabecera en la fila 1 y un autofiltro CON CONDICIÓN en la
// columna B («mayor que 15»), como lo guarda la web (`SheetAutoFilter.cond`).
const libroFiltrado = `{"active":0,"sheets":[{"name":"H","cells":{"A1":"Mes","B1":"Importe","A2":"Ene","B2":"10","A3":"Feb","B3":"20","A4":"Mar","B4":"30","A5":"Abr","B5":"x"},` +
	`"autofilter":{"row":0,"c1":0,"c2":1,"cond":{"1":{"op":"gt","a":"15"}}}}]}`

// TestElFiltroPorCondicionNoSePierdeNiSeIgnora: un libro de la web con un filtro por
// condición se abría EDITABLE en Calcus nativo, enseñaba las filas que la web oculta y, al
// guardar, escribía el autofiltro SIN `cond`: la condición se perdía para todos.
//
// VISTO EN ROJO (2026-10-05, en el frontal, sobre main): «el guardado pierde la condición»
// y «filas ocultas: []».
func TestElFiltroPorCondicionNoSePierdeNiSeIgnora(t *testing.T) {
	lz := libroDePrueba(t, libroFiltrado)
	if s := lz.Serializar(); !strings.Contains(s, `"cond":{"1":{"op":"gt","a":"15"}}`) {
		t.Errorf("el guardado pierde la condición del filtro: %s", s)
	}
	var ocultas []int
	for f, o := range lz.Libro.FilasOcultas() {
		if o {
			ocultas = append(ocultas, f)
		}
	}
	// B2 = 10 no pasa «> 15»; B5 = «x» es texto contra número: tampoco. B3 y B4 se ven.
	if len(ocultas) != 2 || ocultas[0] != 1 || ocultas[1] != 4 {
		t.Errorf("filas ocultas: %v, y la web oculta la 2 y la 5", ocultas)
	}
}

// cuerpoDe arma el cuerpo de una columna: float64 es un número; lo demás, texto.
func cuerpoDe(valores ...any) []celdaFiltro {
	out := make([]celdaFiltro, len(valores))
	for i, v := range valores {
		out[i] = celdaFiltro{v: v}
	}
	return out
}

func pasan(pasa []bool) string {
	var b strings.Builder
	for _, p := range pasa {
		if p {
			b.WriteString("1")
		} else {
			b.WriteString("0")
		}
	}
	return b.String()
}

// TestEvaluarFiltro: los operadores de `evalFilterColumn` con sus casos de borde: texto
// contra número no se ordena, los empates entran en los N superiores, y una celda sin
// número no está «fuera» de un intervalo.
func TestEvaluarFiltro(t *testing.T) {
	nums := cuerpoDe(10.0, 20.0, 30.0, 30.0, "x", "")
	casos := []struct {
		cond   CondFiltro
		cuerpo []celdaFiltro
		quiere string
	}{
		{CondFiltro{Op: "gt", A: "15"}, nums, "011100"},
		{CondFiltro{Op: "ge", A: "20"}, nums, "011100"},
		{CondFiltro{Op: "lt", A: "20"}, nums, "100000"},
		{CondFiltro{Op: "le", A: "20"}, nums, "110000"},
		{CondFiltro{Op: "eq", A: "30"}, nums, "001100"},
		{CondFiltro{Op: "ne", A: "30"}, nums, "110011"},
		{CondFiltro{Op: "between", A: "30", B: "15"}, nums, "011100"}, // el orden de los extremos da igual
		{CondFiltro{Op: "notBetween", A: "15", B: "25"}, nums, "101100"},
		{CondFiltro{Op: "blank"}, nums, "000001"},
		{CondFiltro{Op: "notBlank"}, nums, "111110"},
		{CondFiltro{Op: "top", N: 1}, nums, "001100"}, // el empate entra
		{CondFiltro{Op: "bottom", N: 2}, nums, "110000"},
		{CondFiltro{Op: "top", N: 50, Percent: true}, nums, "001100"}, // la mitad de 4 números: 2
		{CondFiltro{Op: "aboveAvg"}, nums, "001100"},                  // media 22,5
		{CondFiltro{Op: "belowAvg"}, nums, "110000"},
		{CondFiltro{Op: "contains", A: " AN "}, cuerpoDe("Ana", "Juan", "Luis", 5.0), "1100"},
		{CondFiltro{Op: "notContains", A: "an"}, cuerpoDe("Ana", "Juan", "Luis", 5.0), "0011"},
		{CondFiltro{Op: "begins", A: "ju"}, cuerpoDe("Ana", "Juan", "Luis"), "010"},
		{CondFiltro{Op: "ends", A: "IS"}, cuerpoDe("Ana", "Juan", "Luis"), "001"},
		{CondFiltro{Op: "gt", A: "b"}, cuerpoDe("a", "c", 5.0, ""), "0100"}, // texto con texto; el número y el vacío, no
		{CondFiltro{Op: "eq", A: "ana"}, cuerpoDe("Ana", " ANA ", "Ana María"), "110"},
		{CondFiltro{Op: "lt", A: "fila 10"}, cuerpoDe("fila 2", "fila 10", "fila 11"), "100"}, // orden numérico, como la web
	}
	for _, c := range casos {
		if got := pasan(evaluarFiltro(c.cond, c.cuerpo)); got != c.quiere {
			t.Errorf("%+v: %s, quiere %s", c.cond, got, c.quiere)
		}
	}
	// Por color: el relleno o la tinta MANUAL de la celda, sin distinguir mayúsculas.
	color := []celdaFiltro{{estilo: Estilo{Relleno: "#FFEE00"}}, {estilo: Estilo{Tinta: "#ffee00"}}, {}}
	if got := pasan(evaluarFiltro(CondFiltro{Op: "fill", Color: "#ffee00"}, color)); got != "100" {
		t.Errorf("fill: %s", got)
	}
	if got := pasan(evaluarFiltro(CondFiltro{Op: "fontColor", Color: "#ffee00"}, color)); got != "010" {
		t.Errorf("fontColor: %s", got)
	}
}

// leerAutofiltro carga una hoja con esa condición cruda en la columna B y devuelve su filtro.
func leerAutofiltro(t *testing.T, cond string) *Autofiltro {
	t.Helper()
	lz := libroDePrueba(t, `{"active":0,"sheets":[{"name":"H","cells":{"A1":"x"},"autofilter":{"row":0,"c1":0,"c2":1,"cond":{"1":`+cond+`}}}]}`)
	af := lz.Libro.Autofiltro()
	if af == nil {
		t.Fatal("la hoja tiene autofiltro")
	}
	return af
}

// TestSanearCondFiltro: lo que no vale no entra, y lo que vale sale normalizado.
func TestSanearCondFiltro(t *testing.T) {
	buenos := map[string]CondFiltro{
		`{"op":"gt","a":" 15 "}`:                       {Op: "gt", A: " 15 "}, // el operando, tal cual se tecleó
		`{"op":"between","a":"1","b":"2"}`:             {Op: "between", A: "1", B: "2"},
		`{"op":"top","n":3.9}`:                         {Op: "top", N: 3},
		`{"op":"top","n":"10","percent":true}`:         {Op: "top", N: 10, Percent: true},
		`{"op":"bottom","n":500,"percent":true}`:       {Op: "bottom", N: 500}, // un porcentaje no pasa de 100
		`{"op":"fill","color":"#FFEE00"}`:              {Op: "fill", Color: "#ffee00"},
		`{"op":"blank","a":"sobra","n":7,"color":"x"}`: {Op: "blank"},
	}
	for crudo, quiere := range buenos {
		a := leerAutofiltro(t, crudo)
		if got, ok := a.Cond["1"]; !ok || got != quiere {
			t.Errorf("%s: %+v %v, quiere %+v", crudo, got, ok, quiere)
		}
	}
	for _, malo := range []string{
		`{"op":"inventado","a":"1"}`, `{"op":"gt"}`, `{"op":"gt","a":"   "}`, `{"op":"between","a":"1"}`,
		`{"op":"top"}`, `{"op":"top","n":0}`, `{"op":"top","n":100001}`, `{"op":"fill","color":"rojo"}`, `"gt"`, `null`,
	} {
		if a := leerAutofiltro(t, malo); len(a.Cond) != 0 {
			t.Errorf("%s no es una condición y entró: %+v", malo, a.Cond)
		}
	}
	// Una condición de una columna que el filtro no cubre, o de una clave que no es un
	// entero, no entra.
	lz := libroDePrueba(t, `{"active":0,"sheets":[{"name":"H","cells":{"A1":"x"},"autofilter":{"row":0,"c1":0,"c2":1,"cond":{"5":{"op":"blank"},"1.5":{"op":"blank"}}}}]}`)
	if af := lz.Libro.Autofiltro(); af == nil || len(af.Cond) != 0 {
		t.Errorf("fuera de [c1, c2] no hay condición: %+v", af)
	}
}

// TestValoresYCondicionSeExcluyenEnLaMismaColumna: excluir un valor quita la condición de
// esa columna y poner una condición quita sus valores excluidos (`setExcluded`, `setCond`);
// las demás columnas no se tocan, el comando `filtercond` hace lo mismo y deshacer vuelve.
func TestValoresYCondicionSeExcluyenEnLaMismaColumna(t *testing.T) {
	lz := libroDePrueba(t, libroFiltrado)
	l := lz.Libro
	if !l.Filtrar(0, "Ene", true) {
		t.Fatal("se puede excluir un valor de A")
	}
	if af := l.Autofiltro(); len(af.Excl["0"]) != 1 || af.Cond["1"].Op != "gt" {
		t.Fatalf("excluir en A no toca la condición de B: %+v", af)
	}
	l.Filtrar(1, "20", true)
	if af := l.Autofiltro(); len(af.Cond) != 0 || len(af.Excl["1"]) != 1 {
		t.Fatalf("excluir un valor en B quita su condición: %+v", af)
	}
	lz.Mandar(CmdCondFiltro, `{"col":1,"cond":{"op":"contains","a":"3"}}`)
	if af := l.Autofiltro(); af.Cond["1"].Op != "contains" || len(af.Excl["1"]) != 0 || len(af.Excl["0"]) != 1 {
		t.Fatalf("la condición en B quita sus excluidos y deja los de A: %+v", af)
	}
	if !l.Autofiltro().filtrada(1) || l.Autofiltro().filtrada(5) {
		t.Error("el desplegable de B se enciende con la condición, y el de una columna sin criterio no")
	}
	lz.Mandar(CmdCondFiltro, `{"col":1,"cond":{"op":"inventado"}}`)
	if af := l.Autofiltro(); af.Cond["1"].Op != "contains" {
		t.Errorf("una condición inválida no toca nada: %+v", af)
	}
	lz.Mandar(CmdCondFiltro, `{"col":1}`)
	if af := l.Autofiltro(); af.Cond["1"].Op != "contains" {
		t.Errorf("sin `cond` no se quita nada (solo un null explícito): %+v", af)
	}
	lz.Mandar(CmdCondFiltro, `{"col":1,"cond":null}`)
	if af := l.Autofiltro(); len(af.Cond) != 0 {
		t.Errorf("null quita la condición: %+v", af)
	}
	if !l.Deshacer() || l.Autofiltro().Cond["1"].Op != "contains" {
		t.Errorf("deshacer devuelve la condición: %+v", l.Autofiltro())
	}
}

// TestLaCondicionSigueASuColumna: insertar una columna delante mueve la condición con ella.
func TestLaCondicionSigueASuColumna(t *testing.T) {
	l := libroDePrueba(t, libroFiltrado).Libro
	l.Estructura("inscol", "left", 0, 0, 0, 0)
	af := l.Autofiltro()
	if af == nil || af.Cond["2"].Op != "gt" || len(af.Cond) != 1 {
		t.Fatalf("la condición de B pasa a C: %+v", af)
	}
}

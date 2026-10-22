// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"fmt"
	"strings"
	"testing"
)

// Los vectores de este fichero son los de `packages/office/test/formula-edit.test.ts` y
// `packages/office/test/sheets-refshift.test.ts`, uno a uno.

func TestTokenizeFxEsElDeLaWeb(t *testing.T) {
	toksSum := TokenizeFx("=SUM(A1;B2)")
	got := make([]string, 0, len(toksSum))
	for _, tk := range toksSum {
		got = append(got, fmt.Sprintf("%d:%s@%d", tk.T, tk.V, tk.Inicio))
	}
	quiero := []string{"4:=@0", "3:SUM@1", "5:(@4", "3:A1@5", "7:;@7", "3:B2@8", "6:)@10"}
	if strings.Join(got, " ") != strings.Join(quiero, " ") {
		t.Errorf("tokenizeFx da %v y la web %v", got, quiero)
	}
	toks := TokenizeFx(`="hola"&A1`)
	if toks[1].V != `"hola"` || toks[3].V != "A1" {
		t.Errorf("la cadena o la palabra no salen enteras: %+v", toks)
	}
	_ = TokenizeFx(`="A1`) // una cadena sin cerrar no rompe
}

func TestLoQueHayBajoElCursor(t *testing.T) {
	for _, c := range []struct {
		texto    string
		cursor   int
		ok       bool
		prefijo  string
		ini, fin int
	}{
		{"=SU", 3, true, "SU", 1, 3},
		{"=1+AV", 5, true, "AV", 3, 5},
		{"=NORM.DI", 8, true, "NORM.DI", 1, 8},
		{"=SUM(", 5, false, "", 0, 0},
		{"=", 1, false, "", 0, 0},
		{"=Hoja1!A", 8, false, "", 0, 0},
		{"SUM", 3, false, "", 0, 0},
	} {
		f, ok := FuncTokenAt(c.texto, c.cursor)
		if ok != c.ok || (ok && (f.Prefijo != c.prefijo || f.Inicio != c.ini || f.Fin != c.fin)) {
			t.Errorf("funcTokenAt(%q, %d) = %+v %v", c.texto, c.cursor, f, ok)
		}
	}
	for _, c := range []struct {
		texto        string
		cursor       int
		ok           bool
		nombre       string
		arg, abierto int
	}{
		{"=SUM(A1;B2", 10, true, "SUM", 1, 4},
		{"=SUM(A1;B2", 6, true, "SUM", 0, 4},
		{"=IF(A1>2;SUM(B1;C1", 18, true, "SUM", 1, 12},
		{"=IF(A1>2;SUM(B1;C1);", 20, true, "IF", 2, 3},
		{"=1+2", 4, false, "", 0, 0},
		{"=(A1+B1)", 5, false, "", 0, 0},
	} {
		h, ok := ArgHintAt(c.texto, c.cursor)
		if ok != c.ok || (ok && (h.Nombre != c.nombre || h.Arg != c.arg || h.Apertura != c.abierto)) {
			t.Errorf("argHintAt(%q, %d) = %+v %v", c.texto, c.cursor, h, ok)
		}
	}
	for texto, quiero := range map[string]bool{
		"=|": true, "=A1+|": true, "=SUM(|": true, "=SUM(A1;|": true,
		"=A1|": false, "=SUM(A1|": false, "=1+2|": false, "hola|": false,
	} {
		cursor := strings.Index(texto, "|")
		if got := ExpectsRef(strings.Replace(texto, "|", "", 1), cursor); got != quiero {
			t.Errorf("expectsRef(%q) = %v", texto, got)
		}
	}
}

func TestF4CiclaElAnclaje(t *testing.T) {
	for ref, quiero := range map[string]string{
		"A1": "$A$1", "$A$1": "A$1", "A$1": "$A1", "$A1": "A1", "Hoja1!A1": "Hoja1!$A$1",
		"A1:B2": "$A$1:$B$2", "$A$1:$B$2": "A$1:B$2", "no-es-ref": "no-es-ref",
	} {
		if got := CycleRefAbs(ref); got != quiero {
			t.Errorf("cycleRefAbs(%q) = %q y la web da %q", ref, got, quiero)
		}
	}
	if texto, cursor, ok := CycleRefAt("=A1+B2", 2); !ok || texto != "=$A$1+B2" || cursor != 5 {
		t.Errorf("cycleRefAt en A1 da %q, %d", texto, cursor)
	}
	if texto, cursor, ok := CycleRefAt("=A1+B2", 5); !ok || texto != "=A1+$B$2" || cursor != 8 {
		t.Errorf("cycleRefAt en B2 da %q, %d", texto, cursor)
	}
	if _, _, ok := CycleRefAt("=1+2", 2); ok {
		t.Error("cycleRefAt sin referencia bajo el cursor ha hecho algo")
	}
}

func TestExtractRefsEsElDeLaWeb(t *testing.T) {
	var got []string
	for _, r := range ExtractRefs("=A1+B2*A1") {
		got = append(got, fmt.Sprintf("%s@%d:%d#%d", r.Texto, r.Inicio, r.Fin, r.Color))
	}
	if strings.Join(got, " ") != "A1@1:3#0 B2@4:6#1 A1@7:9#0" {
		t.Errorf("extractRefs da %v", got)
	}
	if r := ExtractRefs("=A1+$A$1"); r[0].Color != r[1].Color {
		t.Error("el anclaje cambia el color de la misma celda")
	}
	if r := ExtractRefs("=SUM(A1:B3)"); len(r) != 1 || r[0].Texto != "A1:B3" || r[0].Rect != (Rect{0, 0, 1, 2}) {
		t.Errorf("un rango sale %+v", r)
	}
	if r := ExtractRefs("=Hoja2!A1+A1"); len(r) != 2 || r[0].Texto != "Hoja2!A1" || r[0].Hoja != "Hoja2" || r[1].Color != 1 {
		t.Errorf("una referencia con hoja sale %+v", r)
	}
	for texto, quiero := range map[string]string{
		`="A1"+B1`: "B1", "=LOG10(2)+A1": "A1", "=SUM($A:$B)": "$A:$B", "=A+1": "", "=3+A1": "A1",
		"=SUM(Hoja1!A:A)": "Hoja1!A:A", "=SUM(1:1)": "1:1", "=Ventas[T1]+B1": "B1",
	} {
		var textos []string
		for _, r := range ExtractRefs(texto) {
			textos = append(textos, r.Texto)
		}
		if got := strings.Join(textos, ","); got != quiero {
			t.Errorf("extractRefs(%q) = %q y la web da %q", texto, got, quiero)
		}
	}
	if r := ExtractRefs("=SUM(3:5)"); r[0].Rect.R1 != 2 || r[0].Rect.R2 != 4 || r[0].Rect.C2 < 1000 {
		t.Errorf("la fila entera 3:5 sale %+v", r[0].Rect)
	}
	for _, c := range []struct {
		texto  string
		cursor int
		quiero string
	}{
		{"=SUM(A:A)", 7, "A:A"},
		{"=SUM(1:3)", 8, "1:3"},
		{"=A1", 3, "A1"},
		{"=A1", 1, "A1"},
		{"=A1+B2", 3, "A1"},
		{"=A1+B2", 5, "B2"},
		{"=SUM(A1:B2)", 7, "A1:B2"},
		{"=SUM(A1:B2)", 10, "A1:B2"},
		{"=1+2", 2, ""},
		{"=A1", 0, ""},
		{"A1", 1, ""},
	} {
		r, _ := RefAtCaret(c.texto, c.cursor)
		if r.Texto != c.quiero {
			t.Errorf("refAtCaret(%q, %d) = %q y la web da %q", c.texto, c.cursor, r.Texto, c.quiero)
		}
	}
}

func TestInsertarYBorrarReescribeLasFormulas(t *testing.T) {
	insFila := func(desde int) Desplazamiento {
		return Desplazamiento{Filas: true, Desde: desde, Cantidad: 1, Tope: 100}
	}
	delFila := func(desde, n int) Desplazamiento {
		return Desplazamiento{Filas: true, Desde: desde, Cantidad: -n, Tope: 100}
	}
	insCol := func(desde int) Desplazamiento { return Desplazamiento{Desde: desde, Cantidad: 1, Tope: 26} }
	delCol := func(desde, n int) Desplazamiento { return Desplazamiento{Desde: desde, Cantidad: -n, Tope: 26} }
	for _, c := range []struct {
		d             Desplazamiento
		antes, quiero string
	}{
		{insFila(2), "A1", "A1"},
		{insFila(2), "A2", "A2"},
		{insFila(2), "A3", "A4"},
		{insFila(2), "A10", "A11"},
		{insFila(2), "=SUMA(A1:A5)", "=SUMA(A1:A6)"},
		{insFila(0), "=SUMA(A3:A5)", "=SUMA(A4:A6)"},
		{insFila(20), "=SUMA(A1:A5)", "=SUMA(A1:A5)"},
		{delFila(2, 1), "A3", RefRota},
		{delFila(2, 1), "=A3*2", "=" + RefRota + "*2"},
		{delFila(2, 3), "A9", "A6"},
		{delFila(2, 3), "=SUMA(A1:A10)", "=SUMA(A1:A7)"},
		{delFila(2, 3), "=SUMA(A4:A8)", "=SUMA(A3:A5)"},
		{delFila(2, 3), "=SUMA(A3:A5)", "=SUMA(" + RefRota + ")"},
		{insFila(0), "$A$3", "$A$4"},
		{insFila(0), "A$3", "A$4"},
		{insFila(3), "=$B$2+$B$7", "=$B$2+$B$8"},
		{insCol(1), "C1", "D1"},
		{insCol(1), "A1", "A1"},
		{insCol(2), "=SUMA(B1:D1)", "=SUMA(B1:E1)"},
		{delCol(2, 1), "C1", RefRota},
		{delCol(2, 2), "E1", "C1"},
		{insFila(3), "A:A", "A:A"},
		{insCol(1), "3:3", "3:3"},
		{insCol(0), "B:B", "C:C"},
		{insFila(0), "Hoja1!B4", "Hoja1!B5"},
		{insFila(0), "'Mi hoja'!B4:C9", "'Mi hoja'!B5:C10"},
		{insFila(0), "b4", "b5"},
		{insFila(0), "A100", RefRota},
		{insCol(0), "Z1", RefRota},
		{insFila(0), "123", "123"},
		{insFila(0), "texto suelto", "texto suelto"},
		{insFila(0), "=SUMA(1;2)", "=SUMA(1;2)"},
		{insFila(0), `="A3 es el total"`, `="A3 es el total"`},
		{insFila(0), "=A5+B5+C5", "=A6+B6+C6"},
	} {
		got := c.d.ReescribirFormula(c.antes)
		if !strings.HasPrefix(c.antes, "=") {
			got = c.d.MoverReferencia(c.antes)
			if c.antes == "123" || c.antes == "texto suelto" {
				got = c.d.ReescribirFormula(c.antes)
			}
		}
		if got != c.quiero {
			t.Errorf("%+v sobre %q da %q y la web da %q", c.d, c.antes, got, c.quiero)
		}
	}
}

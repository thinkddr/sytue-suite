// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package numfmt

import (
	"strings"
	"testing"
)

// TestLasMascarasSonLasDeLaWeb son los vectores de
// `packages/office/test/sheets-numfmt-mask.test.ts`, uno a uno.
func TestLasMascarasSonLasDeLaWeb(t *testing.T) {
	const h = 1.0 / 24
	casos := []struct {
		m      string
		n      float64
		quiero string
	}{
		{"0.00", 1234.5, "1234,50"},
		{"#,##0.00", 1234.5, "1.234,50"},
		{"#,##0", 1234.5, "1.235"},
		{"#,##0", 1234.4, "1.234"},
		{"0", 0, "0"},
		{"#", 0, ""},
		{"00000", 42, "00042"},
		{"00", 123, "123"},
		{"#.##", 5, "5"},
		{"#.##", 5.5, "5,5"},
		{"0.##", 5.5, "5,5"},
		{"0.00", 5.5, "5,50"},
		{".00", 12.5, "12,50"},
		{"#,##0.00", -1234.5, "-1.234,50"},
		{"0.00;0.00", -5, "5,00"},
		{"#,##0.00;(#,##0.00)", -1234.5, "(1.234,50)"},
		{"#,##0;-#,##0", -1234, "-1.234"},
		{`0.00;-0.00;"cero"`, 0, "cero"},
		{"0.00;-0.00;", 0, ""},
		{"0;(0)", 0, "0"},
		{"0%", 0.5, "50%"},
		{"0.0%", 0.1234, "12,3%"},
		{"#,##0,", 1234567, "1.235"},
		{"0,,", 12345678, "12"},
		{"$#,##0.00", 1234.5, "$1.234,50"},
		{"#,##0.00 €", 1234.5, "1.234,50 €"},
		{"[$€-C0A]#,##0.00", 1234.5, "€1.234,50"},
		{`"USD "#,##0.00`, 1234.5, "USD 1.234,50"},
		{`\$0`, 5, "$5"},
		{"General", 1234.5, "1234.5"},
		{"General", -5, "-5"},
		{"dd/mm/yyyy", 45292, "01/01/2024"},
		{"d/m/yyyy", 45292, "1/1/2024"},
		{"dd/mmm/yyyy", 45292, "01/ene/2024"},
		{"mmmm yyyy", 45292, "enero 2024"},
		{"yyyy-mm-dd", 45658, "2025-01-01"},
		{"dddd", 45292, "lunes"},
		{`ddd d "de" mmmm`, 45292, "lun 1 de enero"},
		{"hh:mm:ss", 0.5, "12:00:00"},
		{"hh:mm", 0.75, "18:00"},
		{"h:mm", 0.25, "6:00"},
		{"h:mm AM/PM", 0.75, "6:00 PM"},
		{"h:mm am/pm", 0.5, "12:00 pm"},
		{"dd/mm/yyyy hh:mm", 45292.5, "01/01/2024 12:00"},
		{"dd/mm/yyyy", -3, "-3"},
		{"dd/mm/yyyy", 60, "29/02/1900"},
		{"[h]:mm:ss", 26.5 * h, "26:30:00"},
		{"[h]:mm", 1.5 * h, "1:30"},
		{"[hh]:mm", 1.5 * h, "01:30"},
		{"[m]", 90 * 60 * (1.0 / 86400), "90"},
		{"[mm]:ss", 90.5 * 60 * (1.0 / 86400), "90:30"},
		{"[s]", 3661.0 / 86400, "3661"},
		{"[h]:mm", -1.5 * h, "-1:30"},
		{`[h] "h"`, 2 * h, "2 h"},
		{`[>=100]"alto";[<0]"neg";0.00`, 150, "alto"},
		{`[>=100]"alto";[<0]"neg";0.00`, -5, "-neg"},
		{`[>=100]"alto";[<0]"neg";0.00`, 42, "42,00"},
		{"[>5]0;0", 3, "3"},
		{`[<>0]0;"cero"`, 0, "cero"},
		{"[>100]0", 5, "5"},
		{"00000-0000", 123456789, "12345-6789"},
		{"00-00", 1234, "12-34"},
		{"00-00", 123, "01-23"},
		{"000-000-000", 123456789, "123-456-789"},
	}
	for _, c := range casos {
		if got, _ := ConMascara(c.m, c.n); got != c.quiero {
			t.Errorf("formatWithMask(%q, %v) = %q y la web da %q", c.m, c.n, got, c.quiero)
		}
	}
	for _, c := range []struct{ m, texto, color string }{
		{"[Red]0.00", "5,00", "#DC2626"}, {"0.00;[Rojo]-0.00", "-5,00", "#DC2626"}, {"[Blue]0", "5", "#2563EB"}, {"0.00", "5,00", ""},
	} {
		n := 5.0
		if strings.Contains(c.m, "Rojo") {
			n = -5
		}
		if texto, color := ConMascara(c.m, n); texto != c.texto || color != c.color {
			t.Errorf("formatWithMask(%q, %v) = %q/%q y la web da %q/%q", c.m, n, texto, color, c.texto, c.color)
		}
	}
	// Más de cien decimales se acotan a cien, sin romper.
	forzado, _ := ConMascara("0."+strings.Repeat("0", 101), 0.5)
	if !strings.HasPrefix(forzado, "0,5") || len(forzado)-strings.Index(forzado, ",")-1 != 100 {
		t.Errorf("una máscara de 101 decimales da %q", forzado)
	}
	if got := secciones(`0.00;"a;b";0`); len(got) != 3 || got[1] != `"a;b"` {
		t.Errorf("splitSections parte dentro de las comillas: %q", got)
	}
}

// TestUnaMascaraQueSeSaleDeFloatNoRevienta (FYE-01): cinco «%» con 1e300 dan +Inf
// y 160 con 0 dan NaN; antes, pánico al pintar la celda.
func TestUnaMascaraQueSeSaleDeFloatNoRevienta(t *testing.T) {
	for _, c := range []struct {
		mascara string
		n       float64
	}{{"0%%%%%", 1e300}, {"0" + strings.Repeat("%", 160), 0}, {"0" + strings.Repeat(",", 200), 1}} {
		if texto, _ := Aplicar("custom", 0, c.mascara, c.n); texto == "" {
			t.Errorf("%q con %v no da texto", c.mascara, c.n)
		}
	}
}

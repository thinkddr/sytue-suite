// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// sinNBSP es el `plain()` del test de la web: Intl separa la moneda y el % con un
// espacio duro, y el vector se escribe con uno normal.
func sinNBSP(s string) string { return strings.ReplaceAll(s, "\u00a0", " ") }

func ent(n int) *int { return &n }

// TestLosPresetsSonLosDeLaWeb son los vectores de `applyNumFmt` en
// `packages/office/test/sheets-format.test.ts`, más los casos de redondeo y de millares que
// un `strconv` a secas haría mal.
func TestLosPresetsSonLosDeLaWeb(t *testing.T) {
	for _, c := range []struct {
		f      FormatoNum
		n      float64
		quiero string
	}{
		{FormatoNum{ID: "currency"}, 1234.5, "1.234,50 €"},
		{FormatoNum{ID: "percent"}, 0.5, "50 %"},
		{FormatoNum{ID: "number"}, 1234.5, "1234,50"},
		{FormatoNum{ID: "thousands"}, 1234.5, "1.234,50"},
		{FormatoNum{ID: "number", Decimales: ent(0)}, 1234.4, "1234"},
		{FormatoNum{ID: "percent", Decimales: ent(1)}, 0.1234, "12,3 %"},
		{FormatoNum{ID: "general"}, 1234.5, "1234.5"},
		{FormatoNum{ID: "custom", Mascara: "#,##0.00;(#,##0.00)"}, -1234.5, "(1.234,50)"},
		{FormatoNum{ID: "custom"}, 1234.5, "1234.5"},
		{FormatoNum{ID: "date"}, 1, "1/1/1900"},
		{FormatoNum{ID: "date"}, 60, "29/2/1900"},
		{FormatoNum{ID: "date"}, 61, "1/3/1900"},
		{FormatoNum{ID: "date"}, 45292, "1/1/2024"},
		{FormatoNum{ID: "date"}, -3, "-3"},
		// El empate EXACTO: 0,125 a dos decimales es 0,13 en Intl y en toFixed; el
		// redondeo al par de strconv daría 0,12.
		{FormatoNum{ID: "number"}, 0.125, "0,13"},
		// Y el que no es empate: 1,005 es 1,00499999… en binario.
		{FormatoNum{ID: "number"}, 1.005, "1,00"},
		// Los negativos llevan el guion, también el cero negativo, como Intl.
		{FormatoNum{ID: "thousands"}, -1234567.891, "-1.234.567,89"},
		{FormatoNum{ID: "number"}, math.Copysign(0, -1), "-0,00"},
		// El porcentaje agrupa con el mínimo de es-ES: 1234 % junto, 12.345 % separado.
		{FormatoNum{ID: "percent"}, 12.34, "1234 %"},
		{FormatoNum{ID: "percent"}, 123.45, "12.345 %"},
	} {
		if got, _ := aplicarFormato(c.f, c.n); sinNBSP(got) != c.quiero {
			t.Errorf("applyNumFmt(%+v, %v) = %q y la web da %q", c.f, c.n, got, c.quiero)
		}
	}
	// La moneda y el porcentaje van con espacio DURO: que no se partan en dos líneas.
	if got, _ := aplicarFormato(FormatoNum{ID: "currency"}, 1); !strings.Contains(got, "\u00a0€") {
		t.Errorf("la moneda sale %q, sin el espacio duro de Intl", got)
	}
	// formatCellValue: lo que no es número pasa de largo.
	cur := &FormatoNum{ID: "currency"}
	for v, quiero := range map[formula.Value]string{formula.ErrDiv0: "#DIV/0!", true: "TRUE", "hola": "hola"} {
		if got, _ := textoConFormato(v, cur); got != quiero {
			t.Errorf("formatCellValue(%v, currency) = %q y debería ser %q", v, got, quiero)
		}
	}
}

// TestElEstiloSeSaneaComoEnLaWeb: `sanitizeCellStyle` deja pasar solo lo que conoce, y
// lo que se queda vacío desaparece.
func TestElEstiloSeSaneaComoEnLaWeb(t *testing.T) {
	var c estiloCrudo
	if err := json.Unmarshal([]byte(`{"font":"Comic Sans","size":200.7,"color":"#12345G","fill":"#FEF08A",
		"u":true,"s":"sí","valign":"arriba","wrap":true,"border":{"t":true,"x":true},"locked":false,"hidden":1}`), &c); err != nil {
		t.Fatal(err)
	}
	e, ok := c.sanear()
	f := false
	quiero := Estilo{Tam: 96, Relleno: "#FEF08A", Subraya: true, Ajustar: true, Bordes: Bordes{T: true}, Bloqueada: &f}
	if !ok || e.Tam != quiero.Tam || e.Relleno != quiero.Relleno || !e.Subraya || e.Tacha || !e.Ajustar ||
		e.Bordes != quiero.Bordes || e.Bloqueada == nil || *e.Bloqueada || e.Oculta || e.Fuente != "" || e.Tinta != "" || e.Vertical != "" {
		t.Errorf("el estilo saneado es %+v y la web deja %+v", e, quiero)
	}
	b, _ := json.Marshal(e)
	if got := string(b); got != `{"size":96,"fill":"#FEF08A","u":true,"wrap":true,"border":{"t":true},"locked":false}` {
		t.Errorf("el estilo se escribe %s", got)
	}
	if _, ok := (estiloCrudo{Subraya: false}).sanear(); ok {
		t.Error("un estilo que se queda vacío tiene que desaparecer del mapa")
	}
	if e, ok := fusionar(Estilo{Subraya: true, Relleno: "#FEF08A"}, conSubraya(false)); !ok || e.Subraya || e.Relleno == "" {
		t.Errorf("quitar el subrayado deja %+v", e)
	}
}

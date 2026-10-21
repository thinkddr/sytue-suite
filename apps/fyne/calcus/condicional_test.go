// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"image/color"
	"math"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los vectores son los de `packages/office/test/sheets-advanced.test.ts` y
// `sheets-format.test.ts`.

func celdas(m map[string]float64) func(c, f int) formula.Value {
	return func(c, f int) formula.Value {
		if v, ok := m[Ref(c, f)]; ok {
			return v
		}
		return ""
	}
}

func nunca(string, int, int) bool { return false }

func valor(s string) *string { return &s }

func TestElFormatoCondicionalEsElDeLaWeb(t *testing.T) {
	g := celdas(map[string]float64{"A1": 10, "A2": 3, "A3": 20})
	m := decorar([]ReglaCond{{Rango: "A1:A3", Op: "gt", Valor: valor("5"), Relleno: "#FEF08A"}}, g, nunca)
	if m[Celda{0, 0}].relleno != "#FEF08A" || m[Celda{0, 1}] != nil || m[Celda{0, 2}].relleno != "#FEF08A" {
		t.Error("la regla de celda no rellena solo las que cumplen")
	}
	verde := ReglaCond{Rango: "A1", Op: "gt", Valor: valor("0"), Relleno: "#BBF7D0"}
	amarillo := ReglaCond{Rango: "A1", Op: "gt", Valor: valor("0"), Relleno: "#FEF08A"}
	if decorar([]ReglaCond{verde, amarillo}, g, nunca)[Celda{}].relleno != "#BBF7D0" {
		t.Error("no gana la PRIMERA regla de relleno")
	}

	g = celdas(map[string]float64{"A1": 10, "A2": 20, "A3": 10, "A4": 5})
	m = decorar([]ReglaCond{{Rango: "A1:A4", Clase: "top", Puesto: 2, Relleno: "#FED7AA"}}, g, nunca)
	if m[Celda{0, 0}] == nil || m[Celda{0, 1}] == nil || m[Celda{0, 2}] == nil || m[Celda{0, 3}] != nil {
		t.Error("los dos superiores no incluyen los empates del umbral")
	}
	m = decorar([]ReglaCond{{Rango: "A1:A4", Clase: "dup", Relleno: "#FBCFE8"}}, g, nunca)
	if m[Celda{0, 0}] == nil || m[Celda{0, 1}] != nil {
		t.Error("los duplicados no se marcan bien")
	}
	m = decorar([]ReglaCond{{Rango: "A1:A4", Clase: "unique", Relleno: "#BFDBFE"}}, g, nunca)
	if m[Celda{0, 1}] == nil || m[Celda{0, 0}] != nil {
		t.Error("los únicos no se marcan bien")
	}

	g = celdas(map[string]float64{"A1": 5, "A2": 20, "A3": 12.5})
	m = decorar([]ReglaCond{{Rango: "A1:A3", Clase: "colorScale", Escala: []string{"#000000", "#ffffff"}}}, g, nunca)
	if m[Celda{0, 0}].relleno != "#000000" || m[Celda{0, 1}].relleno != "#ffffff" || m[Celda{0, 2}].relleno != "#808080" {
		t.Errorf("la escala de colores da %q %q %q", m[Celda{0, 0}].relleno, m[Celda{0, 1}].relleno, m[Celda{0, 2}].relleno)
	}
	m = decorar([]ReglaCond{{Rango: "A1:A3", Clase: "iconSet", Iconos: "arrows"}}, g, nunca)
	if m[Celda{0, 0}].icono != "arrows:0" || m[Celda{0, 1}].icono != "arrows:2" || m[Celda{0, 2}].icono != "arrows:1" {
		t.Error("los iconos no van por tercios")
	}
	if got := colorEnEscala([]string{"#000000", "#ff0000", "#ffffff"}, 0.25); got != "#800000" {
		t.Errorf("colorScaleAt de tres colores da %q", got)
	}

	g = celdas(map[string]float64{"A1": 10, "A2": 20, "A3": -20})
	m = decorar([]ReglaCond{{Rango: "A1:A3", Clase: "dataBar", Color: "#2563EB"}}, g, nunca)
	if b := m[Celda{0, 1}].barra; b.frac != 1 || b.neg {
		t.Errorf("la barra de 20 es %+v", b)
	}
	if b := m[Celda{0, 2}].barra; !b.neg {
		t.Error("la barra de −20 no es negativa")
	}
	if b := m[Celda{0, 0}].barra; math.Abs(b.frac-0.5) > 1e-9 {
		t.Errorf("la barra de 10 mide %v", b.frac)
	}

	m = decorar([]ReglaCond{{Rango: "A1:A2", Clase: "formula", Formula: "=X", Relleno: "#E9D5FF"}}, g,
		func(_ string, _, df int) bool { return df == 0 })
	if m[Celda{0, 0}] == nil || m[Celda{0, 1}] != nil {
		t.Error("la regla de fórmula no usa el evaluador con la fila relativa")
	}

	// condMatches: miles y coma es-ES, el texto sin distinguir mayúsculas, las vacías
	// y los errores nunca.
	for _, c := range []struct {
		r    ReglaCond
		v    formula.Value
		sale bool
	}{
		{ReglaCond{Op: "le", Valor: valor("1.000,5")}, 1000.5, true},
		{ReglaCond{Op: "gt", Valor: valor("100")}, "texto", false},
		{ReglaCond{Op: "gt", Valor: valor("100")}, formula.ErrDiv0, false},
		{ReglaCond{Op: "ne", Valor: valor("x")}, "", false},
		{ReglaCond{Op: "contains", Valor: valor("HOLA")}, "dijo hola ayer", true},
		{ReglaCond{Op: "eq", Valor: valor("sí")}, "SÍ", true},
		{ReglaCond{Op: "eq", Valor: valor("5")}, 5.0, true},
	} {
		if got := c.r.cumple(c.v); got != c.sale {
			t.Errorf("condMatches(%s %s, %v) = %v", c.r.Op, *c.r.Valor, c.v, got)
		}
	}
}

// TestLaReglaDeFormulaSeEvaluaEnElMotor: la regla «=A1>2» sobre A1:A3 se desplaza a
// cada celda y la evalúa el motor del libro, en una celda de apoyo que después se vacía.
func TestLaReglaDeFormulaSeEvaluaEnElMotor(t *testing.T) {
	c := lienzoDePrueba()
	for f, v := range []string{"1", "5", "3"} {
		c.Libro.Escribir(0, f, v)
	}
	c.Rejilla.Seleccionar(0, 0, false)
	c.Mandar(CmdCondAnadir, `{"range":"A1:A3","kind":"formula","formula":"=A1>2","fill":"#E9D5FF"}`)
	if d := c.Libro.decoracionDe(0, 0); d != nil && d.relleno != "" {
		t.Error("A1 (1) no es mayor que 2 y se ha rellenado")
	}
	if d := c.Libro.decoracionDe(0, 1); d == nil || d.relleno != "#E9D5FF" {
		t.Error("A2 (5) es mayor que 2 y no se ha rellenado")
	}
	if v, _ := c.Libro.motor.GetValue(c.Libro.Hoja(), celdaApoyo); v != "" {
		t.Errorf("la celda de apoyo se ha quedado con %v", v)
	}
	// Y se pinta: el relleno condicional es el fondo de la celda.
	img := pinta(t, 560, 300, c)
	if got := px(img, AnchoCabecera+AnchoCelda/2, AltoBarra+2*AltoCelda+4); !cerca(got, color.NRGBA{R: 0xE9, G: 0xD5, B: 0xFF, A: 0xff}) {
		t.Errorf("A2 pinta %v con el relleno condicional", got)
	}
	// Cambiar A2 lo recalcula.
	c.Libro.Escribir(0, 1, "0")
	if d := c.Libro.decoracionDe(0, 1); d != nil && d.relleno != "" {
		t.Error("el formato condicional no se ha recalculado al cambiar la celda")
	}
}

// TestLasEtiquetasDeLasReglasSeTraducenComoEnLaWeb: el gestor de reglas y el de
// validaciones componen la etiqueta con las mismas claves que `condRuleLabel` y
// `validationLabel`, en el idioma de la interfaz. Los mismos casos que los tests de la
// web (`sheets-advanced.test.ts` y `sheets-validation.test.ts`).
func TestLasEtiquetasDeLasReglasSeTraducenComoEnLaWeb(t *testing.T) {
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	c := &editor.Contexto{T: tr.T}
	cinco := "5"
	for _, caso := range []struct {
		r      ReglaCond
		quiero string
	}{
		{ReglaCond{Clase: "dataBar"}, "Barra de datos"},
		{ReglaCond{Clase: "top", Puesto: 5, Porcien: true}, "5 % superiores"},
		{ReglaCond{Op: "gt", Valor: &cinco}, "Mayor que 5"},
		{ReglaCond{Clase: "colorScale", Escala: []string{"#fff", "#000"}}, "Escala de 2 colores"},
	} {
		if got := etiquetaRegla(c, caso.r); got != caso.quiero {
			t.Errorf("etiquetaRegla(%+v) = %q y la web dice %q", caso.r, got, caso.quiero)
		}
	}
	for _, caso := range []struct {
		v      Validacion
		quiero string
	}{
		{Validacion{Valores: []string{"a", "b"}}, "a, b"},
		{Validacion{Tipo: "number", Op: "between", A: "1", B: "5"}, "Número entre 1 y 5"},
		{Validacion{Tipo: "date", Op: "gt", A: "1/1/2020"}, "Fecha > 1/1/2020"},
		{Validacion{Tipo: "textLength", Op: "le", A: "5"}, "Longitud del texto ≤ 5"},
		{Validacion{Tipo: "formula", Formula: "=A1>0"}, "Fórmula: =A1>0"},
	} {
		if got := etiquetaValidacion(c, caso.v); got != caso.quiero {
			t.Errorf("etiquetaValidacion(%+v) = %q y la web dice %q", caso.v, got, caso.quiero)
		}
	}
}

// TestUnaReglaDeRangoEnormeNoRecorreFueraDeLaRejilla (FYE-03): A1:A9999999 se
// queda en las filas de la hoja.
func TestUnaReglaDeRangoEnormeNoRecorreFueraDeLaRejilla(t *testing.T) {
	leidas := 0
	g := func(c, f int) formula.Value { leidas++; return float64(1) }
	m := decorar([]ReglaCond{{Rango: "A1:A9999999", Op: "gt", Valor: valor("0"), Relleno: "#FEF08A"}}, g, nunca)
	if leidas > Filas || len(m) > Filas {
		t.Fatalf("leídas %d celdas y decoradas %d con una hoja de %d filas", leidas, len(m), Filas)
	}
}

// TestLaReglaPuedePintarSoloLaLetra: una regla de un libro importado puede pintar solo la
// letra («en rojo si es negativo») o la letra y el relleno; antes se perdía al sanear. Cada
// canal es el de la primera regla que lo da, como `buildCondDecor`.
func TestLaReglaPuedePintarSoloLaLetra(t *testing.T) {
	var cruda reglaCruda
	if err := json.Unmarshal([]byte(`{"range":"A1:A2","op":"lt","value":"0","textColor":"#C00000"}`), &cruda); err != nil {
		t.Fatal(err)
	}
	r, ok := cruda.sanear()
	if !ok || r.Tinta != "#C00000" || r.Relleno != "" {
		t.Fatalf("la regla de solo letra se pierde al sanear: %+v %v", r, ok)
	}
	g := celdas(map[string]float64{"A1": -1, "A2": 2})
	ambos := ReglaCond{Rango: "A1:A2", Op: "gt", Valor: valor("-5"), Relleno: "#FEF08A", Tinta: "#1D4ED8"}
	m := decorar([]ReglaCond{r, ambos}, g, nunca)
	if a1 := m[Celda{0, 0}]; a1 == nil || a1.tinta != "#C00000" || a1.relleno != "#FEF08A" {
		t.Errorf("A1 = %+v: la letra de la primera y el relleno de la segunda", a1)
	}
	if a2 := m[Celda{0, 1}]; a2 == nil || a2.tinta != "#1D4ED8" {
		t.Errorf("A2 = %+v", a2)
	}
	if b, err := json.Marshal(r); err != nil || !strings.Contains(string(b), `"textColor":"#C00000"`) {
		t.Errorf("al guardar: %s", b)
	}
}

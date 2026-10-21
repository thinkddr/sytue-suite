// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"
	"sort"
	"testing"
	"time"
)

// Los casos de `multiDayLayout.test.ts` y `timeLayout.test.ts`, uno a uno: la misma
// entrada tiene que dar la misma geometría que en la web.

var semanaJulio = func() []time.Time {
	d := make([]time.Time, 7)
	for i := range d {
		d[i] = time.Date(2026, 7, 6+i, 0, 0, 0, 0, time.UTC)
	}
	return d
}()

func diaJul(d int) time.Time { return time.Date(2026, 7, d, 0, 0, 0, 0, time.UTC) }

func TestTramarEsBarSpan(t *testing.T) {
	for _, c := range []struct {
		nombre     string
		ini, fin   time.Time
		desde, has int
		ok         bool
	}{
		{"tres días, fin exclusivo", diaJul(6), diaJul(9), 0, 2, true},
		{"un solo día", diaJul(8), diaJul(9), 2, 2, true},
		{"sin fin", diaJul(10), time.Time{}, 4, 4, true},
		{"empezó antes", diaJul(4), diaJul(8), 0, 1, true},
		{"acaba después", diaJul(11), diaJul(20), 5, 6, true},
		{"después del rango", diaJul(20), diaJul(22), 0, 0, false},
		{"antes del rango", diaJul(1), diaJul(3), 0, 0, false},
	} {
		d, h, ok := Tramar(semanaJulio, c.ini, c.fin, time.UTC)
		if ok != c.ok || (ok && (d != c.desde || h != c.has)) {
			t.Errorf("%s: sale [%d,%d] ok=%v y la web da [%d,%d] ok=%v", c.nombre, d, h, ok, c.desde, c.has, c.ok)
		}
	}
}

func filasPorID(bs []Barra[string]) map[string]int {
	m := map[string]int{}
	for _, b := range bs {
		m[b.Item] = b.Fila
	}
	return m
}

func TestApilarEsAssignRows(t *testing.T) {
	b := func(d, h int, id string) Barra[string] { return Barra[string]{Item: id, Desde: d, Hasta: h} }
	if f := filasPorID(Apilar([]Barra[string]{b(0, 3, "a"), b(2, 5, "b")})); f["a"] == f["b"] {
		t.Error("dos solapadas comparten fila")
	}
	for _, x := range Apilar([]Barra[string]{b(0, 1, "a"), b(3, 4, "b")}) {
		if x.Fila != 0 {
			t.Errorf("las disjuntas no reutilizan la fila 0: %s en %d", x.Item, x.Fila)
		}
	}
	if f := filasPorID(Apilar([]Barra[string]{b(0, 2, "a"), b(2, 4, "b")})); f["a"] == f["b"] {
		t.Error("tocarse en el mismo día es solaparse (inclusivo)")
	}
	if f := filasPorID(Apilar([]Barra[string]{b(1, 2, "corta"), b(0, 5, "larga"), b(3, 4, "media")})); f["larga"] != 0 {
		t.Errorf("la larga no va arriba: fila %d", f["larga"])
	}
	filas := map[int]bool{}
	for _, x := range Apilar([]Barra[string]{b(0, 4, "a"), b(1, 5, "b"), b(2, 6, "c")}) {
		filas[x.Fila] = true
	}
	if len(filas) != 3 {
		t.Errorf("tres solapadas ocupan %d filas y no 3", len(filas))
	}
}

func TestSemanasEsSliceWeeks(t *testing.T) {
	sem := Semanas(Apilar([]Barra[string]{{Item: "x", Desde: 5, Hasta: 8}}), 6, 7)
	if len(sem[0]) != 1 || len(sem[1]) != 1 {
		t.Fatalf("el evento que cruza de semana no se parte en dos: %d y %d", len(sem[0]), len(sem[1]))
	}
	if a := sem[0][0]; a.Col != 5 || a.Ancho != 2 || a.SigueIzq || !a.SigueDer {
		t.Errorf("semana 0: %+v", a)
	}
	if b := sem[1][0]; b.Col != 0 || b.Ancho != 2 || !b.SigueIzq || b.SigueDer {
		t.Errorf("semana 1: %+v", b)
	}
	sem = Semanas(Apilar([]Barra[string]{{Item: "y", Desde: 8, Hasta: 10}}), 6, 7)
	if len(sem[0]) != 0 || len(sem[1]) != 1 || sem[1][0].Col != 1 || sem[1][0].Ancho != 3 {
		t.Errorf("dentro de una semana: %+v", sem[1])
	}
}

func TestLimitarEsLimitWeekRows(t *testing.T) {
	tr := func(col, ancho, fila int) Tramo[string] {
		return Tramo[string]{Barra: Barra[string]{Fila: fila}, Col: col, Ancho: ancho}
	}
	vis, ocultas := Limitar([]Tramo[string]{tr(0, 2, 0), tr(0, 2, 1), tr(0, 2, 2), tr(0, 2, 3)}, 2, 7)
	if len(vis) != 2 || ocultas[0] != 2 || ocultas[1] != 2 || ocultas[3] != 0 {
		t.Errorf("visibles %d, ocultas %v", len(vis), ocultas)
	}
	if n := Filas([]Tramo[string]{tr(0, 1, 0), tr(2, 1, 2)}); n != 3 {
		t.Errorf("rowCount da %d y no 3", n)
	}
	if n := Filas[string](nil); n != 0 {
		t.Errorf("rowCount de nada da %d", n)
	}
}

func TestTintaSobreEsBarTextColor(t *testing.T) {
	for _, hex := range []string{"#b0470f", "#334155", "#146a47", "#6b3e9e", "#1e50a8"} {
		if colorDe(hex) == AcentoPorDefecto && hex != "#1a6fbb" {
			t.Fatalf("no se leyó %s", hex)
		}
		if got := TintaSobre(colorDe(hex)); got != TintaClaraBarra {
			t.Errorf("sobre %s la web pone blanco y aquí sale %v", hex, got)
		}
	}
	if got := TintaSobre(colorDe("#ffe08a")); got != TintaOscuraBarra {
		t.Errorf("sobre #ffe08a la web pone #111827 y aquí sale %v", got)
	}
	if got := TintaSobre(color.NRGBA{R: 255, G: 255, B: 255, A: 255}); got != TintaOscuraBarra {
		t.Errorf("sobre blanco sale %v", got)
	}
}

func TestMinutosDelDiaEsDayMinutes(t *testing.T) {
	dia := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	a := func(h, m int) time.Time { return time.Date(2026, 7, 10, h, m, 0, 0, time.UTC) }
	for _, c := range []struct {
		nombre   string
		ini, fin time.Time
		quiero   Intervalo
		ok       bool
	}{
		{"09–10", a(9, 0), a(10, 0), Intervalo{540, 600}, true},
		{"sin fin", a(9, 0), time.Time{}, Intervalo{540, 570}, true},
		{"fin igual", a(9, 0), a(9, 0), Intervalo{540, 570}, true},
		{"desde la víspera", time.Date(2026, 7, 9, 23, 0, 0, 0, time.UTC), a(1, 0), Intervalo{0, 60}, true},
		{"hacia mañana", a(23, 0), time.Date(2026, 7, 11, 2, 0, 0, 0, time.UTC), Intervalo{1380, MinutosDia}, true},
		{"otro día", time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC), time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC), Intervalo{}, false},
	} {
		got, ok := MinutosDelDia(c.ini, c.fin, dia, time.UTC)
		if ok != c.ok || got != c.quiero {
			t.Errorf("%s: %v ok=%v y la web da %v ok=%v", c.nombre, got, ok, c.quiero, c.ok)
		}
	}
}

func TestImantarEsSnapMin(t *testing.T) {
	for _, c := range []struct {
		min        float64
		paso, dice int
	}{
		{7, 15, 0},
		{8, 15, 15},
		{532, 15, 525},
		{-30, 15, 0},
		{5000, 15, MinutosDia},
		{70, 30, 60},
		{100, 30, 90},
		{105, 30, 120},
	} {
		if got := Imantar(c.min, c.paso); got != c.dice {
			t.Errorf("snapMin(%v, %d) da %d en la web y %d aquí", c.min, c.paso, c.dice, got)
		}
	}
}

func TestColumnasEsLayoutTimed(t *testing.T) {
	iv := func(d, h int, id string) Bloque[string] { return Bloque[string]{Item: id, Intervalo: Intervalo{d, h}} }
	if out := Columnas([]Bloque[string]{iv(540, 600, "")}); out[0].Col != 0 || out[0].Cols != 1 {
		t.Errorf("uno solo: %+v", out[0])
	}
	out := Columnas([]Bloque[string]{iv(540, 600, "a"), iv(570, 630, "b")})
	ids := make([]string, 0, len(out))
	for _, e := range out {
		if e.Cols != 2 {
			t.Errorf("solapados con %d columnas", e.Cols)
		}
		ids = append(ids, e.Item+":"+string(rune('0'+e.Col)))
	}
	sort.Strings(ids)
	if ids[0] != "a:0" || ids[1] != "b:1" {
		t.Errorf("reparto %v", ids)
	}
	for _, e := range Columnas([]Bloque[string]{iv(540, 600, "a"), iv(660, 720, "b")}) {
		if e.Cols != 1 || e.Col != 0 {
			t.Errorf("disjuntos: %+v", e)
		}
	}
	por := map[string]Bloque[string]{}
	for _, e := range Columnas([]Bloque[string]{iv(540, 660, "A"), iv(570, 600, "B"), iv(600, 660, "C")}) {
		por[e.Item] = e
	}
	if por["A"].Col != 0 || por["B"].Col != 1 || por["C"].Col != 1 || por["A"].Cols != 2 {
		t.Errorf("la columna liberada no se reutiliza: %+v", por)
	}
	if n := len(Columnas([]Bloque[string]{iv(540, 600, "x"), iv(700, 760, "y"), iv(545, 590, "z")})); n != 3 {
		t.Errorf("se pierden eventos: %d de 3", n)
	}
}

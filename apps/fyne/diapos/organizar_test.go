// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"slices"
	"testing"
)

// Los casos son los de `test/slides-organize.test.ts` de la web, uno a uno.

func caja4(id string, x, y, w, h float64, grupo string) Caja {
	return Caja{ID: id, X: x, Y: y, W: w, H: h, Grupo: grupo}
}

func ids(cs []Caja) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func porID(cs []Caja, id string) Caja {
	for _, c := range cs {
		if c.ID == id {
			return c
		}
	}
	return Caja{}
}

func abcd() []Caja {
	return []Caja{
		caja4("a", 0, 0, 10, 10, ""), caja4("b", 0, 0, 10, 10, ""),
		caja4("c", 0, 0, 10, 10, ""), caja4("d", 0, 0, 10, 10, ""),
	}
}

// TestReordenarZEsElDeLaWeb. VISTO EN ROJO (2026-09-23, en el frontal): cambiando el vecino de
// «forward» por el de «backward», el primer caso de avance deja b donde estaba.
func TestReordenarZEsElDeLaWeb(t *testing.T) {
	casos := []struct {
		sel    []string
		op     string
		quiero []string
	}{
		{[]string{"b"}, "front", []string{"a", "c", "d", "b"}},
		{[]string{"c"}, "back", []string{"c", "a", "b", "d"}},
		{[]string{"b"}, "forward", []string{"a", "c", "b", "d"}},
		{[]string{"c"}, "backward", []string{"a", "c", "b", "d"}},
		{[]string{"d"}, "forward", []string{"a", "b", "c", "d"}},
		{[]string{"a"}, "backward", []string{"a", "b", "c", "d"}},
		{[]string{"b", "c"}, "forward", []string{"a", "d", "b", "c"}},
		{[]string{"a", "c"}, "front", []string{"b", "d", "a", "c"}},
	}
	for _, c := range casos {
		if got := ids(ReordenarZ(abcd(), c.sel, c.op)); !slices.Equal(got, c.quiero) {
			t.Errorf("ReordenarZ(%v, %s) = %v, quiero %v", c.sel, c.op, got, c.quiero)
		}
	}
}

func TestAlinearYDistribuirSonLosDeLaWeb(t *testing.T) {
	bs := []Caja{caja4("a", 30, 5, 20, 10, ""), caja4("b", 60, 40, 10, 10, "")}
	for _, c := range []struct {
		borde string
		a, b  float64
	}{{"left", 0, 0}, {"right", 80, 90}, {"hcenter", 40, 45}} {
		out := Alinear(bs, []string{"a", "b"}, c.borde, true)
		if porID(out, "a").X != c.a || porID(out, "b").X != c.b {
			t.Errorf("%s al lienzo: a=%v b=%v, quiero %v %v", c.borde, porID(out, "a").X, porID(out, "b").X, c.a, c.b)
		}
	}
	if bs[0].X != 30 {
		t.Error("alinear ha mutado el original")
	}
	vs := []Caja{caja4("a", 10, 10, 20, 20, ""), caja4("b", 50, 40, 10, 10, "")}
	for _, c := range []struct {
		borde string
		a, b  float64
	}{{"top", 10, 10}, {"bottom", 30, 40}, {"vmiddle", 20, 25}} {
		out := Alinear(vs, []string{"a", "b"}, c.borde, false)
		if porID(out, "a").Y != c.a || porID(out, "b").Y != c.b {
			t.Errorf("%s a la selección: a=%v b=%v, quiero %v %v", c.borde, porID(out, "a").Y, porID(out, "b").Y, c.a, c.b)
		}
	}
	h := []Caja{caja4("a", 0, 0, 10, 10, ""), caja4("b", 30, 0, 20, 10, ""), caja4("c", 80, 0, 10, 10, "")}
	if out := Distribuir(h, []string{"a", "b", "c"}, "h"); porID(out, "b").X != 35 || porID(out, "c").X != 80 {
		t.Errorf("distribuir en horizontal deja b en %v y c en %v (35 y 80)", porID(out, "b").X, porID(out, "c").X)
	}
	if out := Distribuir(h[:2], []string{"a", "b"}, "h"); porID(out, "b").X != 30 {
		t.Error("con menos de tres no se distribuye")
	}
}

func TestLosGruposSonLosDeLaWeb(t *testing.T) {
	g := []Caja{
		caja4("a", 0, 0, 10, 10, "g1"), caja4("b", 0, 0, 10, 10, "g1"), caja4("c", 0, 0, 10, 10, ""),
		caja4("d", 0, 0, 10, 10, "g2"), caja4("e", 0, 0, 10, 10, "g2"),
	}
	for _, c := range []struct{ sel, quiero []string }{
		{[]string{"a"}, []string{"a", "b"}},
		{[]string{"c"}, []string{"c"}},
		{[]string{"a", "d"}, []string{"a", "b", "d", "e"}},
		{[]string{"c", "zzz"}, []string{"c"}},
	} {
		if got := ExpandirGrupos(g, c.sel); !slices.Equal(got, c.quiero) {
			t.Errorf("ExpandirGrupos(%v) = %v, quiero %v", c.sel, got, c.quiero)
		}
	}
	out := Agrupar(abcd(), []string{"b", "d"}, "G")
	if !slices.Equal(ids(out), []string{"a", "c", "b", "d"}) || porID(out, "b").Grupo != "G" || porID(out, "a").Grupo != "" {
		t.Errorf("agrupar b y d = %v", out)
	}
	if out := Desagrupar(g, []string{"a"}); porID(out, "b").Grupo != "" || porID(out, "d").Grupo != "g2" {
		t.Error("desagrupar quita el grupo entero tocado y solo ese")
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package office

import "testing"

var tipos = []string{Documento, Hoja, Presentacion}

func TestRegistroDeComandos(t *testing.T) {
	cs := Comandos()
	if len(cs) < 150 {
		t.Fatalf("solo hay %d comandos: el registro ha dejado de leerse", len(cs))
	}
	negrita, ok := BuscarComando("bold")
	if !ok || negrita.ID != "bold" || !negrita.Edits || negrita.LabelKey == "" {
		t.Fatalf("bold: %+v", negrita)
	}
	for _, k := range tipos {
		if !negrita.Existe(k) {
			t.Errorf("bold debería existir en %s", k)
		}
	}
	if !negrita.Bloqueado(true) || negrita.Bloqueado(false) {
		t.Error("un comando que edita se apaga en solo lectura y solo ahí")
	}
	if _, ok := BuscarComando("inventado"); ok {
		t.Error("un id que no está en el registro no se encuentra")
	}
	// «Cortar» es `copy` con valor: el registro lo da por lectura.
	cp, _ := BuscarComando("copy")
	if cp.Edits {
		t.Error("copy no edita; cortar se trata aparte")
	}
}

func TestRegistroDeComandosEsUnaCopia(t *testing.T) {
	a, _ := BuscarComando("bold")
	a.Kinds[0] = "mutado"
	b, _ := BuscarComando("bold")
	if b.Kinds[0] == "mutado" {
		t.Fatal("el registro compartido se pudo alterar desde fuera")
	}
	m := Comandos()
	delete(m, "bold")
	if _, ok := BuscarComando("bold"); !ok {
		t.Fatal("borrar de la copia borró del registro")
	}
}

func TestTaxonomiaDelRoadmap(t *testing.T) {
	ids := func(k string) []string {
		pest := PestanasDe(k)
		out := make([]string, 0, len(pest))
		for _, p := range pest {
			out = append(out, p.ID)
		}
		return out
	}
	tiene := func(k, id string) bool {
		for _, x := range ids(k) {
			if x == id {
				return true
			}
		}
		return false
	}
	for _, k := range tipos {
		if !tiene(k, "inicio") || !tiene(k, "revisar") {
			t.Errorf("%s: Inicio y Revisar van siempre: %v", k, ids(k))
		}
		if got := ids(k); got[0] != "inicio" || got[len(got)-1] != "vista" {
			t.Errorf("%s: Inicio abre la tira y Vista la cierra: %v", k, got)
		}
	}
	for _, id := range []string{"insertar", "diseno"} {
		if !tiene(Documento, id) || !tiene(Presentacion, id) || tiene(Hoja, id) {
			t.Errorf("%s: documento y presentación, no la hoja", id)
		}
	}
	for _, id := range []string{"datos", "formulas"} {
		if !tiene(Hoja, id) || tiene(Documento, id) || tiene(Presentacion, id) {
			t.Errorf("%s: solo la hoja", id)
		}
	}
	if !tiene(Presentacion, "presentar") || tiene(Documento, "presentar") || tiene(Hoja, "presentar") {
		t.Error("presentar: solo las diapositivas")
	}
}

// TestNingunComandoSinPestana: la pestaña de cada comando se ve en TODOS los tipos en que
// el comando existe. Es lo que impide perder un comando al mover una pestaña.
func TestNingunComandoSinPestana(t *testing.T) {
	for id, c := range Comandos() {
		p, _, ok := BuscarPestana(c.Tab)
		if !ok {
			t.Errorf("%s: la pestaña %q no está en la taxonomía", id, c.Tab)
			continue
		}
		for _, k := range c.Kinds {
			if !p.Visible(k) {
				t.Errorf("%s existe en %s pero su pestaña %q no se ve ahí", id, k, c.Tab)
			}
		}
	}
}

func TestTaxonomiaEsUnaCopia(t *testing.T) {
	a := Pestanas()
	a[0].Kinds[0] = "mutado"
	a[0].ID = "mutado"
	if b := Pestanas(); b[0].ID == "mutado" || b[0].Kinds[0] == "mutado" {
		t.Fatal("la taxonomía compartida se pudo alterar desde fuera")
	}
}

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

// Los atajos de Google Docs del registro (los de Word chocan con el navegador), y lo que no
// tienen que pisar: guardar, buscar, imprimir y el zoom son del marco, no de un comando.
func TestComandoDelAtajo(t *testing.T) {
	casos := []struct{ teclas, kind, id, valor string }{
		{"Ctrl+B", Documento, "bold", ""},
		{"Ctrl+Shift+X", Presentacion, "strike", ""},
		{"Ctrl+Shift+L", Documento, "alignLeft", ""},
		{"Ctrl+Shift+e", Documento, "alignCenter", ""}, // la tecla no distingue mayúsculas
		{"Ctrl+Shift+R", Documento, "alignRight", ""},
		{"Ctrl+Shift+J", Documento, "alignJustify", ""},
		{"Ctrl+Shift+7", Documento, "listNumber", ""},
		{"Ctrl+Shift+8", Documento, "listBullet", ""},
		{"Ctrl+Alt+0", Documento, "heading", "p"},
		{"Ctrl+Alt+2", Documento, "heading", "h2"},
		{"Ctrl+]", Documento, "indent", ""},
		{"Ctrl+[", Documento, "outdent", ""},
		{"Ctrl+.", Documento, "superscript", ""},
		{"Ctrl+,", Documento, "subscript", ""},
		{`Ctrl+\`, Documento, "clearFormat", ""},
		{"Ctrl+K", Documento, "insertLink", ""},
		{"Ctrl+Shift+V", Hoja, "paste", "values"},
		// En la hoja, la misma combinación es el autofiltro de Excel y Sheets: el registro
		// solo prohíbe repetirla dentro de un tipo.
		{"Ctrl+Shift+L", Hoja, "autofilter", ""},
	}
	for _, c := range casos {
		id, valor, ok := ComandoDelAtajo(c.teclas, c.kind)
		if !ok || id != c.id || valor != c.valor {
			t.Errorf("%s en %s = %q %q %v; se esperaba %q %q", c.teclas, c.kind, id, valor, ok, c.id, c.valor)
		}
	}
	nada := []struct{ teclas, kind string }{
		{"Ctrl+Z", Documento},                // `canvas`: lo atiende el historial del editor
		{"Ctrl+Shift+B", Documento},          // un modificador de más no es negrita
		{"Ctrl+Alt+B", Documento},            // ni uno de otro tipo
		{"Ctrl+Shift+L", Presentacion + "x"}, // un tipo que no existe no tiene atajos
		{"Shift+L", Documento},               // sin Ctrl es escribir
		{"Ctrl+Shift+Mayús+L", Documento},
	}
	for _, c := range nada {
		if id, _, ok := ComandoDelAtajo(c.teclas, c.kind); ok {
			t.Errorf("%s en %s no debería disparar nada y dispara %s", c.teclas, c.kind, id)
		}
	}
	for _, reservado := range []string{"Ctrl+S", "Ctrl+F", "Ctrl+P", "Ctrl+0", "Ctrl+=", "Ctrl+-", "Ctrl+Shift+=", "Ctrl+H", "Ctrl+Shift+Z"} {
		for _, k := range tipos {
			if id, _, ok := ComandoDelAtajo(reservado, k); ok {
				t.Errorf("%s es del marco y en %s lo pisa %s", reservado, k, id)
			}
		}
	}
}

func TestLosIdsEnOrdenSonLosDelRegistroEnElOrdenDelJSON(t *testing.T) {
	ids := IDsEnOrden()
	if len(ids) != len(Comandos()) {
		t.Fatalf("%d ids en orden y %d filas: falta o sobra alguno", len(ids), len(Comandos()))
	}
	for _, id := range ids {
		if _, ok := BuscarComando(id); !ok {
			t.Errorf("%q no está en el registro", id)
		}
	}
	// Las primeras filas del JSON, tal cual están escritas (no en orden alfabético).
	if ids[0] != "undo" || ids[1] != "redo" || ids[2] != "copy" {
		t.Errorf("el orden no es el del JSON: %v", ids[:3])
	}
	ids[0] = "mutado"
	if IDsEnOrden()[0] != "undo" {
		t.Error("IDsEnOrden tiene que devolver una copia")
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import "testing"

const (
	listaIdx = `<ul style="list-style:none;padding-left:0;margin:.25em 0">`
	subIdx   = `<ul style="list-style:none;padding-left:1.5em;margin:0">`
)

func TestElTerminoDelIndice(t *testing.T) {
	for crudo, quiero := range map[string][3]string{
		"café": {"café", "", "1"}, " Casa : puerta ": {"Casa", "puerta", "1"}, "A:B:C": {"A", "B:C", "1"},
		":x": {"", "x", ""}, "   ": {"", "", ""},
	} {
		termino, sub, ok := TerminoIndice(crudo)
		if ok != (quiero[2] == "1") || ok && (termino != quiero[0] || sub != quiero[1]) {
			t.Errorf("TerminoIndice(%q) = %q, %q, %v", crudo, termino, sub, ok)
		}
	}
}

// TestElIndiceAlfabeticoDeLaWeb: lo que dejan `markIndexEntry` y `regenerarIndice` se
// entiende y vuelve igual; las listas del índice no llevan viñeta y las subentradas van
// sangradas; marcar envuelve lo seleccionado (o deja el marcador vacío, que no se pierde al
// seguir escribiendo); y generar ordena como `localeCompare('es')` —la «á» con la «a»— y
// numera las apariciones en orden. Se vio en ROJO ordenando por bytes: «árbol» iba el
// último.
func TestElIndiceAlfabeticoDeLaWeb(t *testing.T) {
	web := `<p>El <span data-index-entry="té" data-index-sub="verde" id="idx-1">té</span></p>` +
		`<section data-index=""><p><b>Índice alfabético</b></p>` + listaIdx + `<li data-index-term="">té` + subIdx +
		`<li data-index-sub="">verde <a href="#idx-1">1</a></li></ul></li></ul></section>`
	d := Parse(web)
	if !d.Editable() || d.HTML() != web {
		t.Fatalf("la web no vuelve igual o no se edita (%v): %s", d.NoEntendido(), d.HTML())
	}
	bs := d.Bloques()
	if n := len(bs); bs[n-2].Texto() != "té" || bs[n-2].Sangria != 0 || bs[n-1].Sangria != 1 || bs[n-1].Marca != "" {
		t.Errorf("las entradas: %q/%d, %q/%d/%q", bs[n-2].Texto(), bs[n-2].Sangria, bs[n-1].Texto(), bs[n-1].Sangria, bs[n-1].Marca)
	}

	d = Parse(`<p>Zapato, casa y árbol.</p>`)
	e := abrir(t, d, 0)
	e.MarcarIndice(0, 6, "Zapato", "")
	abrir(t, d, 0).MarcarIndice(8, 12, "Casa", "puerta")
	abrir(t, d, 0).MarcarIndice(15, 15, "árbol", "")
	abrir(t, d, 0).Sustituir(21, 21, "!")
	marcado := `<p><span data-index-entry="Zapato">Zapato</span>, <span data-index-entry="Casa" data-index-sub="puerta">casa</span> y ` +
		`<span data-index-entry="árbol"></span>árbol!.</p>`
	despues(t, "marcar", d, marcado)
	if _, ok := d.GenerarIndiceAlfa(d.Bloques(), 0, "Índice alfabético", "Vacío"); !ok {
		t.Fatal("no se ha generado el índice")
	}
	despues(t, "generar", d, `<p><span data-index-entry="Zapato" id="idx-1">Zapato</span>, `+
		`<span data-index-entry="Casa" data-index-sub="puerta" id="idx-2">casa</span> y <span data-index-entry="árbol" id="idx-3"></span>árbol!.</p>`+
		`<section data-index=""><p><b>Índice alfabético</b></p>`+listaIdx+
		`<li data-index-term="">árbol <a href="#idx-3">3</a></li>`+
		`<li data-index-term="">Casa`+subIdx+`<li data-index-sub="">puerta <a href="#idx-2">2</a></li></ul></li>`+
		`<li data-index-term="">Zapato <a href="#idx-1">1</a></li></ul></section>`)
	if abrir(t, d, len(d.Bloques())-1).MarcarIndice(0, 1, "x", "") {
		t.Error("dentro del propio índice no se marca")
	}
	d = Parse(`<p>nada</p>`)
	d.GenerarIndiceAlfa(d.Bloques(), 0, "Índice", "No hay entradas.")
	despues(t, "sin entradas", d, `<p>nada</p><section data-index=""><p><b>Índice</b></p><p style="color:#64748b"><i>No hay entradas.</i></p></section>`)
}

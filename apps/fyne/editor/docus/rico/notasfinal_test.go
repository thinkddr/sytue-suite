// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import "testing"

const (
	listaFinal = `<ol style="list-style:none;padding-left:0;margin:.25em 0">`
	notaFinal1 = `<li id="en-1"><a href="#enref-1" data-endnote-back="" contenteditable="false" title="Volver">i.</a> `
	notaFinal2 = `<li id="en-2"><a href="#enref-2" data-endnote-back="" contenteditable="false" title="Volver">ii.</a> `
	marcaFinal = `<sup data-endnote-ref="1" id="enref-1"><a href="#en-1">i</a></sup>`
)

func TestLosRomanosDeLaWeb(t *testing.T) {
	for n, quiero := range map[int]string{0: "i", 1: "i", 4: "iv", 9: "ix", 14: "xiv", 1994: "mcmxciv"} {
		if got := Romano(n); got != quiero {
			t.Errorf("Romano(%d) = %q, y `romanNumeral` da %q", n, got, quiero)
		}
	}
}

// TestLasNotasAlFinalDeLaWeb: lo que deja `insertarNotaAlFinal` se entiende, vuelve igual y
// se pinta sin número ni sangría, con el enlace de vuelta como UNA runa que no se edita; una
// nota nueva delante de otra la renumera (la de delante es la «i»), y quitar el marcador
// quita su nota. Se vio en ROJO sin `sinMarca`: la nota salía con «1.» delante.
func TestLasNotasAlFinalDeLaWeb(t *testing.T) {
	web := `<p>a` + marcaFinal + `</p><section data-endnotes=""><hr>` + listaFinal + notaFinal1 + `Texto</li></ol></section>`
	d := Parse(web)
	if !d.Editable() || d.HTML() != web {
		t.Fatalf("la web no vuelve igual o no se edita (%v): %s", d.NoEntendido(), d.HTML())
	}
	nota := d.Bloques()[len(d.Bloques())-1]
	if nota.Marca != "" || nota.Sangria != 0 || nota.Texto() != string(ObjetoRuna)+" Texto" || nota.Tramos[0].Rotulo != "i." {
		t.Errorf("la nota: marca %q, sangría %d, texto %q, %+v", nota.Marca, nota.Sangria, nota.Texto(), nota.Tramos[0])
	}

	d = Parse(`<p>hola</p>`)
	l, ok := d.InsertarNotaFinal(d.Bloques(), 0, 4, "Texto", "Volver")
	if !ok {
		t.Fatal("no se ha insertado la nota al final")
	}
	despues(t, "la primera", d, `<p>hola`+marcaFinal+`</p><section data-endnotes=""><hr>`+listaFinal+notaFinal1+`Texto</li></ol></section>`)
	if i := indice(d.Bloques(), l); i < 0 || d.Bloques()[i].Texto() != string(ObjetoRuna)+" Texto" {
		t.Error("el cursor va a la nota nueva")
	}
	abrir(t, d, len(d.Bloques())-1).Sustituir(2, 7, "Primera")
	d.InsertarNotaFinal(d.Bloques(), 0, 0, "Texto", "Volver")
	despues(t, "la segunda, delante", d, `<p>`+marcaFinal+`hola<sup data-endnote-ref="2" id="enref-2"><a href="#en-2">ii</a></sup></p>`+
		`<section data-endnotes=""><hr>`+listaFinal+notaFinal1+`Texto</li>`+notaFinal2+`Primera</li></ol></section>`)
	e := abrir(t, d, 0)
	e.Sustituir(0, 1, "") // se va el primer marcador
	if !d.RenumerarNotasFinal("Texto", "Volver") {
		t.Fatal("renumerar dice si había algo")
	}
	despues(t, "sin el primer marcador", d, `<p>hola`+marcaFinal+`</p><section data-endnotes=""><hr>`+listaFinal+notaFinal1+`Primera</li></ol></section>`)
	abrir(t, d, 0).Sustituir(4, 5, "")
	d.RenumerarNotasFinal("Texto", "Volver")
	despues(t, "sin marcadores", d, `<p>hola</p>`)
}

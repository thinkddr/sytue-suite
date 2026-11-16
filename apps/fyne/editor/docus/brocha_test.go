// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
)

// TestLaBrocha es el ciclo de «Copiar formato», el mismo que `docs-brocha.test.ts`: se
// arma, un clic suelto no la gasta, al soltar sobre una selección pinta en UN paso de
// deshacer, y Esc la desarma sin pintar.
func TestLaBrocha(t *testing.T) {
	const antes = `<p><b>hola</b> mundo cruel</p>`
	f := folioCon(t, antes)
	f.anc, f.cur, f.conCursor = punto{0, 0}, punto{0, 4}, true // «hola»
	f.Mandar(editor.CopiarFormato, "")
	if !f.Formato().Brocha {
		t.Fatal("pulsada la brocha, el botón tiene que salir pulsado")
	}

	f.mover(punto{0, 8}, false) // un clic: el cursor solo
	f.pag.MouseUp(&desktop.MouseEvent{})
	if f.Serializar() != antes || !f.Formato().Brocha {
		t.Fatalf("un clic suelto no pinta ni la gasta: %s", f.Serializar())
	}

	f.anc, f.cur = punto{0, 5}, punto{0, 10} // arrastrar sobre «mundo»
	f.pag.MouseUp(&desktop.MouseEvent{})
	if got := f.Serializar(); got != `<p><b>hola</b> <b>mundo</b> cruel</p>` {
		t.Errorf("al soltar sobre «mundo» se pinta en negrita: %s", got)
	}
	if f.Formato().Brocha {
		t.Error("tras pintar, la brocha se desarma")
	}
	f.deshacer()
	if got := f.Serializar(); got != antes {
		t.Errorf("pintar es un solo paso de deshacer: %s", got)
	}

	f.anc, f.cur = punto{0, 0}, punto{0, 4}
	f.Mandar(editor.CopiarFormato, "")
	f.pag.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	f.anc, f.cur = punto{0, 11}, punto{0, 16}
	f.pag.MouseUp(&desktop.MouseEvent{})
	if f.Formato().Brocha || f.Serializar() != antes {
		t.Errorf("Esc desarma sin pintar: %s", f.Serializar())
	}
}

// TestLaBrochaConElTeclado: otra pulsación pinta lo seleccionado; sobre el mismo origen,
// se arrepiente.
func TestLaBrochaConElTeclado(t *testing.T) {
	f := folioCon(t, `<p><i>uno</i> dos</p>`)
	f.anc, f.cur, f.conCursor = punto{0, 0}, punto{0, 3}, true
	f.Mandar(editor.CopiarFormato, "")
	f.Mandar(editor.CopiarFormato, "")
	if f.Formato().Brocha || f.Serializar() != `<p><i>uno</i> dos</p>` {
		t.Fatalf("sobre el origen la segunda pulsación la desarma: %s", f.Serializar())
	}
	f.Mandar(editor.CopiarFormato, "")
	f.anc, f.cur = punto{0, 4}, punto{0, 7} // Mayús+flechas hasta «dos»
	f.Mandar(editor.CopiarFormato, "")
	if got := f.Serializar(); got != `<p><i>uno</i> <i>dos</i></p>` || f.Formato().Brocha {
		t.Errorf("la segunda pulsación pinta «dos»: %s", got)
	}
}

// TestCambiarMayusculasEnElFolio: cruza bloques, cada uno empieza frase, la selección
// sigue abarcando lo mismo y se deshace de una vez.
func TestCambiarMayusculasEnElFolio(t *testing.T) {
	const antes = `<p>HOLA <b>MUNDO</b>. ADIÓS</p><h2>otra COSA</h2>`
	f := folioCon(t, antes)
	f.anc, f.cur, f.conCursor = punto{0, 0}, punto{1, 4}, true
	f.Mandar(editor.Mayusculas, "sentence")
	if got := f.Serializar(); got != `<p>Hola <b>mundo</b>. Adiós</p><h2>Otra COSA</h2>` {
		t.Errorf("tipo oración: %s", got)
	}
	if f.anc != (punto{0, 0}) || f.cur != (punto{1, 4}) {
		t.Errorf("la selección se ha movido: %v → %v", f.anc, f.cur)
	}
	f.Mandar(editor.Mayusculas, "inventado")
	f.deshacer()
	if got := f.Serializar(); got != antes {
		t.Errorf("deshacer devuelve lo de antes: %s", got)
	}
}

// TestLaCintaTieneBrochaYMayusculas: los dos botones están en Fuente, la brocha sale
// pulsada con su rótulo de «armada» y el menú lleva los cuatro modos.
func TestLaCintaTieneBrochaYMayusculas(t *testing.T) {
	f := folioCon(t, `<p>hola</p>`)
	f.anc, f.cur, f.conCursor = punto{0, 0}, punto{0, 4}, true
	f.Mandar(editor.CopiarFormato, "")
	b := botonBrocha(f.ctx, f.Formato().Brocha)
	if !b.Activo || b.Rotulo != "format.painterOn" {
		t.Errorf("armada: activo %v, rótulo %q", b.Activo, b.Rotulo)
	}
	if b := botonBrocha(f.ctx, false); b.Activo || b.Rotulo != "format.painter" {
		t.Errorf("desarmada: activo %v, rótulo %q", b.Activo, b.Rotulo)
	}
	if len(rotulosMayusculas) != 4 {
		t.Errorf("el menú lleva los cuatro modos: %v", rotulosMayusculas)
	}
}

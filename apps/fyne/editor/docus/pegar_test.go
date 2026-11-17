// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
)

// conPortapapeles hace que el folio lea ese HTML del portapapeles del sistema, en el
// acto, y deja `texto` en el de Fyne. Cuenta las lecturas.
func conPortapapeles(f *Folio, h, texto string, err error) *int {
	leidas := 0
	f.Lanzar = func(hacer func()) { hacer() }
	f.LeerHTML = func() (string, error) { leidas++; return h, err }
	fyne.CurrentApp().Clipboard().SetContent(texto)
	return &leidas
}

func fixturePegar(t *testing.T, nombre string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("rico", "testdata", "pegar", nombre))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestPegarDeWordConFormato: ⌘/Ctrl + V con lo que copia Word pega el título, la negrita,
// el enlace, la lista, la tabla y la imagen en el cursor, el documento sigue entendiéndose
// entero (editable) y un solo deshacer lo quita todo.
func TestPegarDeWordConFormato(t *testing.T) {
	const antes = "<p>Antes</p><p>Después</p>"
	f := folioCon(t, antes)
	conPortapapeles(f, fixturePegar(t, "word.html"), "Informe trimestral", nil)
	f.mover(punto{0, 5}, false) // al final de «Antes»
	f.pag.TypedShortcut(&fyne.ShortcutPaste{})
	h := f.Serializar()
	for _, quiero := range []string{
		"<p>Antes</p><h1>Informe", "<b>crecieron un 12 %</b>", `<a href="https://sytue.eu/informe">Sytue</a>`,
		"<ul><li>", "<table><tbody>", `<img src="data:image/png;base64,`, "<p>Después</p>",
	} {
		if !strings.Contains(h, quiero) {
			t.Errorf("no está %q en %s", quiero, h)
		}
	}
	for _, fuera := range []string{"Mso", "mso-", "onclick", "file:"} {
		if strings.Contains(h, fuera) {
			t.Errorf("entra %q: %s", fuera, h)
		}
	}
	if d := rico.Parse(h); !d.Editable() {
		t.Errorf("tras pegar el documento no se entiende: %v", d.NoEntendido())
	}
	// El cursor queda al final de lo pegado: en el último párrafo, el de la imagen.
	if c := f.bloques[f.cur.b]; f.cur.p != f.largo(f.cur.b) || c.Texto() != string(rico.ObjetoRuna) {
		t.Errorf("el cursor queda en %v (%q) y no al final de lo pegado", f.cur, c.Texto())
	}
	f.pag.TypedShortcut(&fyne.ShortcutUndo{})
	if got := f.Serializar(); got != antes {
		t.Errorf("un deshacer no quita lo pegado de una vez: %s", got)
	}
}

// TestPegarEnMedioDeUnaFrase: lo copiado de dentro de un párrafo (Chrome lo da sin
// bloque) entra en la línea, sobre la selección, sin partir el párrafo y con su formato.
func TestPegarEnMedioDeUnaFrase(t *testing.T) {
	f := folioCon(t, "<p>Hola XX mundo</p>")
	conPortapapeles(f, `<meta charset='utf-8'><span style="color: rgb(32, 33, 36);">muy <b>buen</b></span>`, "muy buen", nil)
	f.anc, f.cur = punto{0, 5}, punto{0, 7} // «XX»
	f.pag.TypedShortcut(&fyne.ShortcutPaste{})
	if got := f.Serializar(); got != `<p>Hola <span style="color: rgb(32, 33, 36)">muy <b>buen</b></span> mundo</p>` {
		t.Errorf("pegar en la línea: %s", got)
	}
	if f.cur != (punto{0, 13}) || f.anc != f.cur {
		t.Errorf("el cursor queda en %v y no tras lo pegado", f.cur)
	}
}

// TestPegarSinFormato: sin HTML en el portapapeles se pega el texto; con ⌘/Ctrl + Mayús + V
// también, sin leer siquiera el HTML; y donde el formato no cabe (bloques en una celda,
// el control de cambios) se pega el texto.
func TestPegarSinFormato(t *testing.T) {
	f := folioCon(t, "<p>a</p>")
	conPortapapeles(f, "", "uno", errors.New("sin HTML"))
	f.mover(punto{0, 1}, false)
	f.pag.TypedShortcut(&fyne.ShortcutPaste{})
	if got := f.Serializar(); got != "<p>auno</p>" {
		t.Errorf("sin HTML: %s", got)
	}

	leidas := conPortapapeles(f, "<h2>título</h2>", " plano", nil)
	f.pag.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyV, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift})
	if got := f.Serializar(); got != "<p>auno plano</p>" || *leidas != 0 {
		t.Errorf("con Mayús: %s (HTML leído %d veces)", got, *leidas)
	}

	f = folioCon(t, "<table><tbody><tr><td>x</td></tr></tbody></table>")
	conPortapapeles(f, "<p>uno</p><p>dos</p>", "uno\ndos", nil)
	f.mover(punto{0, 1}, false)
	f.pag.TypedShortcut(&fyne.ShortcutPaste{})
	if got := f.Serializar(); strings.Contains(got, "</table><p>") || !strings.Contains(got, "uno") {
		t.Errorf("dos párrafos en una celda: %s", got)
	}

	f = folioCon(t, rico.Sentinela+"<p>a</p>")
	conPortapapeles(f, "<b>negrita</b>", "negrita", nil)
	f.mover(punto{0, 1}, false)
	f.pag.TypedShortcut(&fyne.ShortcutPaste{})
	if got := f.Serializar(); strings.Contains(got, "<b>") || !strings.Contains(got, "negrita") {
		t.Errorf("con control de cambios se pega el texto, marcado: %s", got)
	}
}

// TestPegarBloquesEnMedioDeUnParrafo: varios bloques pegados en medio de un párrafo lo
// parten, y el cursor queda al final de lo pegado, no al final de la cola del párrafo.
func TestPegarBloquesEnMedioDeUnParrafo(t *testing.T) {
	f := folioCon(t, "<p>Hola mundo</p>")
	conPortapapeles(f, "<p>uno</p><p>dos</p>", "uno\ndos", nil)
	f.mover(punto{0, 4}, false)
	f.pag.TypedShortcut(&fyne.ShortcutPaste{})
	if got := f.Serializar(); !strings.Contains(got, "<p>uno</p><p>dos</p>") {
		t.Fatalf("no se pegan los dos párrafos: %s", got)
	}
	if c := f.bloques[f.cur.b]; c.Texto() != "dos" || f.cur.p != 3 || f.anc != f.cur {
		t.Errorf("el cursor queda en %v (%q) y no tras «dos»", f.cur, c.Texto())
	}
}

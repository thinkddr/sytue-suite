// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
)

// casoMd es un caso de `testdata/markdown.json`, el que lee también la web
// (`client/markdown.test.ts`): el mismo texto pegado tiene que dar la misma nota.
type casoMd struct {
	Nombre string `json:"nombre"`
	Md     string `json:"md"`
	Parece bool   `json:"parece"`
	HTML   string `json:"html"`
}

func TestMarkdownCasosComunesConLaWeb(t *testing.T) {
	crudo, err := os.ReadFile("testdata/markdown.json")
	if err != nil {
		t.Fatal(err)
	}
	var casos []casoMd
	if err := json.Unmarshal(crudo, &casos); err != nil {
		t.Fatal(err)
	}
	if len(casos) < 10 {
		t.Fatalf("solo %d casos", len(casos))
	}
	for _, c := range casos {
		t.Run(c.Nombre, func(t *testing.T) {
			if got := pareceMarkdown(c.Md); got != c.Parece {
				t.Errorf("pareceMarkdown = %v, quiero %v", got, c.Parece)
			}
			if got := markdownAHTML(c.Md); got != c.HTML {
				t.Errorf("markdownAHTML:\n tengo %s\nquiero %s", got, c.HTML)
			}
		})
	}
}

// El HTML generado tiene que ser una nota que este editor entiende entera: si `Leer` no
// lo diera por fiel, la nota pegada se abriría en solo lectura.
func TestMarkdownPegadoEsUnaNotaFiel(t *testing.T) {
	d := documentoDePegado("## Plan\n- [x] Cajas\n> cita\n```\ncódigo\n```\n1. **uno** [a](https://sytue.eu)")
	if d == nil {
		t.Fatal("no se ha tomado por Markdown")
	}
	tipos := make([]TipoBloque, 0, len(d.Bloques))
	for _, b := range d.Bloques {
		tipos = append(tipos, b.Tipo)
	}
	quiero := []TipoBloque{Titulo, Tarea, Cita, Codigo, Numerada}
	if len(tipos) != len(quiero) {
		t.Fatalf("bloques %v, quiero %v", tipos, quiero)
	}
	for i := range quiero {
		if tipos[i] != quiero[i] {
			t.Fatalf("bloques %v, quiero %v", tipos, quiero)
		}
	}
	if !d.Bloques[1].Marcada {
		t.Error("la tarea hecha llega sin marcar")
	}
	if documentoDePegado("Hola, quedamos el martes.\n- y nada más") != nil {
		t.Error("la prosa con una sola marca se ha tomado por Markdown")
	}
}

// editorParaPegar monta un editor vacío con el foco, listo para recibir atajos.
func editorParaPegar(t *testing.T) *EditorNota {
	t.Helper()
	ed := NuevoEditorNota(trInterno(t), Leer(""), "")
	w := test.NewWindow(ed)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(500, 400))
	w.Canvas().Focus(ed.Zona())
	return ed
}

// El caso del plan: pegar con ⌘V da título, tarea y viñeta con negrita.
func TestPegarMarkdownLlegaConFormato(t *testing.T) {
	ed := editorParaPegar(t)
	pp := test.NewClipboard()
	pp.SetContent("## Plan\n- [ ] Cajas\n- **Camión**")
	ed.zona.TypedShortcut(&fyne.ShortcutPaste{Clipboard: pp})
	html := ed.HTML()
	for _, quiero := range []string{
		"<h2>Plan</h2>",
		`<div class="task-row"><input type="checkbox"/> <span>Cajas</span></div>`,
		"<ul><li><b>Camión</b></li></ul>",
	} {
		if !strings.Contains(html, quiero) {
			t.Fatalf("falta %s en %s", quiero, html)
		}
	}
	if strings.Contains(html, "##") || strings.Contains(html, "**") {
		t.Fatalf("quedan marcas literales: %s", html)
	}
}

// Pegar prosa normal no cambia: entra como texto, con sus asteriscos.
func TestPegarProsaNoEsMarkdown(t *testing.T) {
	ed := editorParaPegar(t)
	pp := test.NewClipboard()
	pp.SetContent("Hola, *quedamos* el martes.")
	ed.zona.TypedShortcut(&fyne.ShortcutPaste{Clipboard: pp})
	if got := ed.Documento().Bloques[0].Texto(); got != "Hola, *quedamos* el martes." {
		t.Fatalf("la prosa ha llegado como %q (%s)", got, ed.HTML())
	}
}

// Ctrl+Mayús+V pega el Markdown sin formato, como en la web.
func TestPegarSinFormatoConMayusculas(t *testing.T) {
	ed := editorParaPegar(t)
	fyne.CurrentApp().Clipboard().SetContent("## Plan\n- [ ] Cajas")
	ed.zona.TypedShortcut(&desktop.CustomShortcut{
		KeyName: fyne.KeyV, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift,
	})
	html := ed.HTML()
	if strings.Contains(html, "<h2>") || strings.Contains(html, "task-row") || !strings.Contains(html, "## Plan") {
		t.Fatalf("Ctrl+Mayús+V ha pegado con formato: %s", html)
	}
}

// En un bloque de código el Markdown pegado es código: entra en letras, como en la web.
func TestPegarMarkdownEnCodigoEntraLiteral(t *testing.T) {
	ed := NuevoEditorNota(trInterno(t), Leer("<pre>x</pre>"), "")
	w := test.NewWindow(ed)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(500, 400))
	w.Canvas().Focus(ed.Zona())
	pp := test.NewClipboard()
	pp.SetContent("## Plan\n- [ ] Cajas")
	ed.zona.TypedShortcut(&fyne.ShortcutPaste{Clipboard: pp})
	html := ed.HTML()
	if strings.Contains(html, "<h2>") || strings.Contains(html, "task-row") || !strings.Contains(html, "## Plan") {
		t.Fatalf("el Markdown pegado en un bloque de código se ha interpretado: %s", html)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
)

func zipDe(t *testing.T, ficheros map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for nombre, cuerpo := range ficheros {
		w, err := zw.Create(nombre)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(cuerpo)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMarkdownAHTMLCubreLoQueExportanObsidianYBear(t *testing.T) {
	md := strings.Join([]string{
		"## Compra",
		"Leche **entera** y *pan*",
		"segunda línea con `**literal**`",
		"",
		"- [ ] huevos",
		"- [x] fruta",
		"- uno",
		"- dos",
		"1. primero",
		"",
		"> una cita",
		"",
		"```",
		"<script>x</script>",
		"```",
		"",
		"| A | B |",
		"|---|---|",
		"| 1 | 2 |",
		"",
		"---",
		"Ver [[Otra nota|la otra]], [web](https://ejemplo.eu) y [local](adjunto.pdf) ~~no~~ ==sí==",
	}, "\n")
	got := MarkdownAHTML(md)
	quiero := []string{
		"<h2>Compra</h2>",
		"<p>Leche <strong>entera</strong> y <em>pan</em><br>segunda línea con <code>**literal**</code></p>",
		`<div class="task-row"><input type="checkbox" class="task-checkbox"> <span>huevos</span></div>`,
		`<input type="checkbox" class="task-checkbox" checked> <span>fruta</span>`,
		"<ul><li>uno</li><li>dos</li></ul><ol><li>primero</li></ol>",
		"<blockquote><p>una cita</p></blockquote>",
		"<pre><code>&lt;script&gt;x&lt;/script&gt;</code></pre>",
		"<table><thead><tr><th>A</th><th>B</th></tr></thead><tbody><tr><td>1</td><td>2</td></tr></tbody></table>",
		"<hr>",
		`Ver la otra, <a href="https://ejemplo.eu">web</a> y local <del>no</del> <mark>sí</mark>`,
	}
	for _, q := range quiero {
		if !strings.Contains(got, q) {
			t.Errorf("falta %q en\n%s", q, got)
		}
	}
	if strings.Contains(got, "<script>") {
		t.Errorf("el HTML del Markdown pasó sin escapar: %s", got)
	}
}

func TestLeerImportableMarkdownSuelto(t *testing.T) {
	md := "\uFEFF---\ntags: [a]\n---\n# Receta\nHarina\n"
	nb, notas, err := LeerImportable([]byte(md), `C:\Users\x\Receta.md`)
	if err != nil {
		t.Fatal(err)
	}
	if nb.Title != "Receta" || len(notas) != 1 {
		t.Fatalf("cuaderno %q con %d notas", nb.Title, len(notas))
	}
	n := notas[0]
	// El `# Receta` repetido se quita: el título ya lo dice, y el front matter no se pinta.
	if n.Title != "Receta" || n.Content != "<p>Harina</p>" || n.Kind != KindText {
		t.Fatalf("nota %+v", n)
	}
	in := nb.AImportar()
	if err := in.Normalize(); err != nil {
		t.Fatalf("el cuaderno importado no pasa la validación: %v", err)
	}
	ni := n.AImportar()
	if err := ni.Normalize(false); err != nil {
		t.Fatalf("la nota importada no pasa la validación: %v", err)
	}
}

func TestLeerImportableZipDeObsidian(t *testing.T) {
	datos := zipDe(t, map[string]string{
		"Bóveda/b.md":                "Segunda",
		"Bóveda/Sub/a.markdown":      "# a\nPrimera",
		"Bóveda/.obsidian/app.json":  "{}",
		"Bóveda/.obsidian/nota.md":   "oculta",
		"__MACOSX/Bóveda/._b.md":     "basura",
		"Bóveda/adjuntos/imagen.png": "\x89PNG",
	})
	nb, notas, err := LeerImportable(datos, "Bóveda.zip")
	if err != nil {
		t.Fatal(err)
	}
	if nb.Title != "Bóveda" || len(notas) != 2 {
		t.Fatalf("cuaderno %q con %d notas: %+v", nb.Title, len(notas), notas)
	}
	// Orden de carpeta: «Sub/a» antes que «b».
	if notas[0].Title != "a" || notas[0].Content != "<p>Primera</p>" || notas[0].Position != 0 {
		t.Errorf("primera nota %+v", notas[0])
	}
	if notas[1].Title != "b" || notas[1].Position != 1 {
		t.Errorf("segunda nota %+v", notas[1])
	}
}

func TestLeerImportableZipSinNotasNoEsImportable(t *testing.T) {
	datos := zipDe(t, map[string]string{"foto.jpg": "x"})
	if _, _, err := LeerImportable(datos, "fotos.zip"); !errors.Is(err, ErrFormatoNoReconocido) {
		t.Fatalf("err = %v, quiero ErrFormatoNoReconocido", err)
	}
}

func TestLeerImportableZipFrenaLaBomba(t *testing.T) {
	// Un ZIP pequeño que descomprime por encima del tope se rechaza entero.
	datos := zipDe(t, map[string]string{"a.md": strings.Repeat("a", maxDescomprimidoZip+1)})
	if len(datos) > 1<<20 {
		t.Fatalf("el ZIP de prueba no comprime: %d bytes", len(datos))
	}
	if _, _, err := LeerImportable(datos, "bomba.zip"); !errors.Is(err, ErrFormatoNoReconocido) {
		t.Fatalf("err = %v, quiero ErrFormatoNoReconocido", err)
	}
}

func TestLeerImportableEvernote(t *testing.T) {
	enex := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE en-export SYSTEM "http://xml.evernote.com/pub/evernote-export4.dtd">
<en-export export-date="20240101T120000Z" application="Evernote">
  <note>
    <title>Viaje</title>
    <content><![CDATA[<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE en-note SYSTEM "http://xml.evernote.com/pub/enml2.dtd"><en-note><div><en-todo checked="true"/>Billetes</div><div><en-todo/>Hotel</div><en-media hash="abc" type="image/png"/><div>&nbsp;</div></en-note>]]></content>
    <tag>vacaciones</tag>
    <tag>dos palabras</tag>
  </note>
  <note><title>Otra</title><content><![CDATA[<en-note>Hola</en-note>]]></content></note>
</en-export>`
	nb, notas, err := LeerImportable([]byte(enex), "Recetas.enex")
	if err != nil {
		t.Fatal(err)
	}
	if nb.Title != "Recetas" || len(notas) != 2 {
		t.Fatalf("cuaderno %q con %d notas", nb.Title, len(notas))
	}
	c := notas[0].Content
	for _, q := range []string{
		`<div class="task-row"><input type="checkbox" class="task-checkbox" checked> <span>Billetes</span></div>`,
		`<div class="task-row"><input type="checkbox" class="task-checkbox"> <span>Hotel</span></div>`,
		"#vacaciones #dos-palabras",
	} {
		if !strings.Contains(c, q) {
			t.Errorf("falta %q en %q", q, c)
		}
	}
	if strings.Contains(c, "en-media") || strings.Contains(c, "en-note") || strings.Contains(c, "<?xml") {
		t.Errorf("quedan etiquetas de Evernote: %q", c)
	}
	if notas[1].Title != "Otra" || notas[1].Content != "Hola" || notas[1].Position != 1 {
		t.Errorf("segunda nota %+v", notas[1])
	}
}

func TestLeerImportableSigueLeyendoElNativoYRechazaLoDemas(t *testing.T) {
	nativo := `{"formato":"sytue.notepads","version":1,"notebook":{"title":"X"},"notas":[]}`
	if nb, _, err := LeerImportable([]byte(nativo), "x.onex"); err != nil || nb.Title != "X" {
		t.Fatalf("el .onex dejó de leerse: %v", err)
	}
	// Un binario con extensión .md no se convierte en una nota de basura.
	if _, _, err := LeerImportable([]byte("\xff\xfe\x00binario"), "x.md"); !errors.Is(err, ErrFormatoNoReconocido) {
		t.Fatalf("err = %v", err)
	}
	// Ni un texto cualquiera sin extensión de Markdown.
	if _, _, err := LeerImportable([]byte("hola"), "x.pdf"); !errors.Is(err, ErrFormatoNoReconocido) {
		t.Fatalf("err = %v", err)
	}
}

// Evernote 10 ya no usa `<en-todo>`: escribe las listas de tareas con
// propiedades CSS propias. Tienen que llegar como tareas, con su marca.
func TestEvernote10ConvierteSusListasDeTareas(t *testing.T) {
	enml := `<en-note><ul style="--en-todo:true;"><li style="--en-checked:true;"><div>Pan</div></li>` +
		`<li style="--en-checked:false;"><div>Sal</div></li></ul></en-note>`
	got := enmlAHTML(enml, nil)
	quiero := `<div class="task-row"><input type="checkbox" class="task-checkbox" checked> <span>Pan</span></div>` +
		`<div class="task-row"><input type="checkbox" class="task-checkbox"> <span>Sal</span></div>`
	if got != quiero {
		t.Errorf("got %q", got)
	}
}

// Notion pega un identificador al nombre de cada página, y Bear con adjuntos
// guarda cada nota como «Título.textbundle/text.md»: el título es el de verdad.
func TestElTituloDeUnaEntradaQuitaLoQuePoneElPrograma(t *testing.T) {
	casos := map[string]string{
		"Export/Ideas 0123456789abcdef0123456789abcdef.md": "Ideas",
		"Bear/Receta.textbundle/text.markdown":             "Receta",
		"Bóveda/text.md":                                   "text",
		"Bóveda/Sub/Diario.md":                             "Diario",
	}
	for ruta, quiero := range casos {
		if got := tituloDeEntrada(ruta); got != quiero {
			t.Errorf("tituloDeEntrada(%q) = %q, quiero %q", ruta, got, quiero)
		}
	}
}

func TestMarkdownAHTMLNoFormateaDentroDeLosEnlacesNiInventaTablas(t *testing.T) {
	got := MarkdownAHTML("[doc](https://ejemplo.eu/_a_/b**c**) y <https://ejemplo.eu/~~x~~>")
	if !strings.Contains(got, `<a href="https://ejemplo.eu/_a_/b**c**">doc</a>`) ||
		!strings.Contains(got, `<a href="https://ejemplo.eu/~~x~~">https://ejemplo.eu/~~x~~</a>`) {
		t.Errorf("el formato entró en el enlace: %s", got)
	}
	if got := MarkdownAHTML("a | b\n---"); got != "<p>a | b</p><hr>" {
		t.Errorf("un párrafo con «|» y una raya salió %q", got)
	}
}

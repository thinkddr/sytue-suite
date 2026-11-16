// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/saneado"
)

// pegadoDe es lo que queda de un fichero de testdata/pegar tal como llega del portapapeles.
func pegadoDe(t *testing.T, nombre string) Pegado {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "pegar", nombre))
	if err != nil {
		t.Fatal(err)
	}
	return ReducirPegado(saneado.HTML(string(b)))
}

// TestPegarDeWordConservaElFormato: lo que copia Word (con sus clases, sus `mso-*`, sus
// `<o:p>`, sus comentarios condicionales y su lista hecha de párrafos) queda en un título,
// un párrafo con negrita, cursiva y su enlace seguro, una lista de verdad, una tabla y su
// imagen `data:`; y se entiende entero, así que el documento sigue siendo editable.
func TestPegarDeWordConservaElFormato(t *testing.T) {
	p := pegadoDe(t, "word.html")
	if p.EnLinea {
		t.Error("varios bloques no se pegan en la línea")
	}
	if d := Parse(p.HTML); !d.Editable() {
		t.Fatalf("lo pegado de Word no se entiende (%v):\n%s", d.NoEntendido(), p.HTML)
	}
	for _, quiero := range []string{
		"<h1>Informe\ntrimestral</h1>", "<b>crecieron un 12 %</b>", "<i>panel</i>",
		`<a href="https://sytue.eu/informe">Sytue</a>`, `<ul><li><span style="color: #C00000">Riesgo</span> alto</li></ul>`,
		"<table><tbody><tr><td><p><b>Región</b></p></td>", "<td><p>1.200</p></td>",
		`<img src="data:image/png;base64,`, `alt="Logo" width="2" height="2">`,
	} {
		if !strings.Contains(p.HTML, quiero) {
			t.Errorf("falta %q en:\n%s", quiero, p.HTML)
		}
	}
	for _, fuera := range []string{"Mso", "mso-", "o:p", "onclick", "robar", "file:", "·", "Symbol", "class=", "width=301", "<!--"} {
		if strings.Contains(p.HTML, fuera) {
			t.Errorf("entra %q:\n%s", fuera, p.HTML)
		}
	}
}

// TestPegarDeChromeConservaElFormato: lo que copia un navegador (estilos calculados en
// cada elemento, `<div>` anidados, listas anidadas, cita, regla y código) entra con su
// forma; lo que no se entiende ni es seguro —un botón, un SVG, un iframe, una imagen
// `https:` que delataría a quien abre el documento— no entra.
func TestPegarDeChromeConservaElFormato(t *testing.T) {
	p := pegadoDe(t, "chrome.html")
	if d := Parse(p.HTML); !d.Editable() {
		t.Fatalf("lo pegado de Chrome no se entiende (%v):\n%s", d.NoEntendido(), p.HTML)
	}
	for _, quiero := range []string{
		`<h2 style="color: rgb(32, 33, 36);`, "font-size: 22px", "<p>Asistentes: <strong>Ana</strong>, <em>Luis</em> y ",
		`<span style="text-decoration: underline">Marta</span>`, `<li>Revisar el <a href="https://example.org/plan">plan</a></li>`,
		"<li>Enviar presupuesto<ul><li>antes del viernes</li></ul></li>", "<ol><li>Primero</li><li>Segundo</li></ol>",
		"<blockquote><p>La calidad no es un acto.</p></blockquote><hr>", "<p>go test ./...<br>make ci</p>",
	} {
		if !strings.Contains(p.HTML, quiero) {
			t.Errorf("falta %q en:\n%s", quiero, p.HTML)
		}
	}
	for _, fuera := range []string{"letter-spacing", "text-transform", "margin", "padding", "target", "rel=", "class", "<div", "<pre", "<code", "<button", "<svg", "circle", "<iframe", "rastreo.gif", "<img", "onclick"} {
		if strings.Contains(p.HTML, fuera) {
			t.Errorf("entra %q:\n%s", fuera, p.HTML)
		}
	}
}

// TestNadaNoEntendidoEntra: cada cosa que el folio no sabe pintar se desenvuelve y deja
// su texto; lo que solo agrupa no deja nada; una tabla en una celda se aplana.
func TestNadaNoEntendidoEntra(t *testing.T) {
	for entrada, quiero := range map[string]Pegado{
		`<span>uno <code>dos</code> <mark>tres</mark></span>`:                           {`<p>uno dos tres</p>`, true},
		`<p class="x" id="y" style="margin: 0; mso-x: 1">a</p>`:                         {`<p>a</p>`, true},
		`<b>negrita</b> y <a>sin enlace</a>`:                                            {`<p><b>negrita</b> y sin enlace</p>`, true},
		`<del>fuera</del> <ins>dentro</ins>`:                                            {`<p><s>fuera</s> <u>dentro</u></p>`, true},
		`<div><div><p>uno</p></div><div>dos</div></div>`:                                {`<p>uno</p><p>dos</p>`, false},
		`<h3 style="text-align: center; float: left">T</h3>`:                            {`<h3 style="text-align: center">T</h3>`, false},
		`<table><tr><td><table><tr><td>a</td><td>b</td></tr></table></td></tr></table>`: {`<table><tbody><tr><td><p>a b</p></td></tr></tbody></table>`, false},
		`<table><caption>Cifras</caption><tr><td></td></tr></table>`:                    {`<p>Cifras</p><table><tbody><tr><td><br></td></tr></tbody></table>`, false},
		`<font color="#c00000" face="Arial">rojo</font><font color="url(x)">no</font>`:  {`<p><font color="#c00000">rojo</font>no</p>`, true},
		`<img src="data:image/svg+xml;base64,PHN2Zz4=">texto`:                           {`<p>texto</p>`, true},
		"\n  \n": {"", false},
	} {
		got := ReducirPegado(saneado.HTML(entrada))
		if got != quiero {
			t.Errorf("ReducirPegado(%q)\n  = %+v\n  y quiero %+v", entrada, got, quiero)
		}
		if got.HTML != "" && !Parse(got.HTML).Editable() {
			t.Errorf("%q deja algo que no se entiende: %v", entrada, Parse(got.HTML).NoEntendido())
		}
	}
}

// TestPegarEnLineaNoParteElParrafo: un trozo de una frase pegado en medio de otra entra
// con su formato sobre el de alrededor, sin partir el párrafo; un enlace pegado dentro de
// otro no se anida.
func TestPegarEnLineaNoParteElParrafo(t *testing.T) {
	d := Parse(`<p>Hola <i>mundo</i></p>`)
	e := d.Bloques()[0].Editar()
	if n := e.PegarEnLinea(7, `<p><b>gran</b> de</p>`); n != 7 {
		t.Fatalf("entran %d runas y son 7", n)
	}
	if got := d.HTML(); got != `<p>Hola <i>mu<b>gran</b> dendo</i></p>` {
		t.Errorf("pegar en medio: %s", got)
	}
	d = Parse(`<p><a href="https://a.example">ab</a></p>`)
	e = d.Bloques()[0].Editar()
	e.PegarEnLinea(1, `<p><a href="https://b.example">X</a></p>`)
	if got := d.HTML(); got != `<p><a href="https://a.example">a</a><a href="https://b.example">X</a><a href="https://a.example">b</a></p>` {
		t.Errorf("un enlace en otro: %s", got)
	}
	if n := Parse(`<p>a</p>`).Bloques()[0].Editar().PegarEnLinea(0, `<h2>no</h2>`); n != 0 {
		t.Errorf("un título no se pega en la línea, y entran %d", n)
	}
}

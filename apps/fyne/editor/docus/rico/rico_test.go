// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// literales junta las cadenas de un trozo de código (`"a" + "b"`, o con comillas graves):
// así los documentos de muestra se leen de DONDE VIVEN y no de una copia que envejece.
func literales(t *testing.T, fuente, desde, hasta string) string {
	t.Helper()
	_, resto, ok := strings.Cut(fuente, desde)
	trozo, _, ok2 := strings.Cut(resto, hasta)
	if !ok || !ok2 {
		t.Fatalf("no encuentro el documento de muestra entre %q y %q: ¿se ha movido?", desde, hasta)
	}
	var b strings.Builder
	for _, lit := range regexp.MustCompile("\"(?:[^\"\\\\]|\\\\.)*\"|`[^`]*`").FindAllString(trozo, -1) {
		s, err := strconv.Unquote(lit)
		if err != nil {
			t.Fatalf("%s: %v", lit, err)
		}
		b.WriteString(s)
	}
	return b.String()
}

// nordika es el documento que siembra la demo (el único de Docus con contenido) y
// fidelidad el que usa el servicio para probar sus exportaciones.
func nordika(t *testing.T) string {
	return literales(t, webref.Read(t, "test/e2e/sembrar-demo.mjs"), `title: "Propuesta de servicio — Nordika",`+"\n    content:", "\n  },")
}

func fidelidad(t *testing.T) string {
	return literales(t, webref.Read(t, "services/docs/internal/domain/export_fidelity_test.go"), "const richDocHTML = ", "\n\n")
}

// La regla de la casa: cargar → guardar sin tocar → IDÉNTICO, byte a byte.
func TestIdaYVueltaSinTocar(t *testing.T) {
	muestra := nordika(t)
	if !strings.HasPrefix(muestra, "<h1>Propuesta de servicio") || !strings.HasSuffix(muestra, "entre semana.</p>") {
		t.Fatalf("la muestra no es la que era: %q", muestra)
	}
	casos := map[string]string{
		"la demo":         muestra,
		"vacío de la web": "<p><br></p>",
		"entidades":       `<p>l'équipe &amp; «co» a&nbsp;b &lt;x&gt; "comillas"</p>`,
		"lo que escribe Chrome": `<p style="text-align: center;"><span style="font-weight: bold; color: rgb(255, 0, 0);">rojo</span> ` +
			`<span style="font-style: italic;">y</span> <span style="text-decoration-line: underline;">sub</span></p>`,
		"enlace y atributos": `<p><a href="https://sytue.eu/?a=1&amp;b=&quot;2&quot;" target="_blank" rel="noopener">x</a></p>`,
		"listas anidadas":    "<ul><li>uno<ul><li>dentro</li></ul></li><li>dos</li></ul><ol><li>a</li></ol>",
		// Los <li> seguidos vuelven dentro de UN <ul>, no cada uno en el suyo.
		"listas entre párrafos": "<p>x</p><ul><li>a</li><li>b</li></ul><p>y</p><ul><li>a</li></ul><ol><li>b</li></ol>",
		"línea suelta":          "suelto<p>párrafo</p>cola<br>",
		// Lo que NO se entiende también vuelve intacto: leer no estropea.
		"control de cambios": `<!--sytue-track--><p>a<ins data-track="1" data-author-id="u1">b</ins></p>`,
		"objetos":            `<p><img src="data:image/png;base64,AAAA" data-sytue-image="1" style="width: 40px;"><svg viewBox="0 0 1 1"><foreignObject></foreignObject></svg></p>`,
		"datos":              `<script type="application/json">{"a":"<b>&"}</script><p>x</p>`,
	}
	for nombre, fuente := range casos {
		if got := Parse(fuente).HTML(); got != fuente {
			t.Errorf("%s:\n  leído    %q\n  escrito  %q", nombre, fuente, got)
		}
	}
	if d := Parse(muestra); !d.Editable() || !d.Guardable() {
		t.Errorf("la demo se tiene que poder editar: %v", d.NoEntendido())
	}
}

// Lo que no se entiende se lee, se dice por qué, y no se toca.
func TestLoQueNoSeEntiendeSeLeeYNoSeToca(t *testing.T) {
	for fuente, motivo := range map[string]string{
		`<p>a</p><!--sytue-track-->`:                        "<!-- -->",
		`<p class="x">a</p>`:                                "p[class]",
		`<p style="margin: 0 0 0 40px;">a</p>`:              "style: margin",
		`<p><span style="color: red;">a</span></p>`:         "style: color",
		`<p><span style="text-align: center;">a</span></p>`: "style: text-align",
		`<p><svg viewBox="0 0 1 1"></svg></p>`:              "<svg>",
		`<table><tbody><tr><td><table><tbody><tr><td>x</td></tr></tbody></table></td></tr></tbody></table>`: "<table> en <td>",
		`<table><colgroup><col></colgroup><tbody><tr><td>x</td></tr></tbody></table>`:                       "<colgroup>",
		`<p><mark>a</mark></p>`:              "<mark>",
		`<p><sup data-note="1">1</sup></p>`:  "sup[data-note]",
		`<p><font face="Arial">a</font></p>`: "font[face]",
		`<p>a<br style="clear: both;"></p>`:  "br[style]",
		`<p>a</p><script>x</script>`:         "<script>",
		// Lo que la web escribe en el folio y aquí no se modela (venían de documento_test).
		`<p id="h-1">x</p>`:                          "p[id]",
		`<nav><p>Índice</p></nav>`:                   "<nav>",
		`<section><p>n</p></section>`:                "<section>",
		`<p><span data-formfield="text"></span></p>`: "span[data-formfield]",
		`<p><ins>x</ins></p>`:                        "<ins>",
	} {
		if d := Parse(fuente); d.Editable() || !slices.Contains(d.NoEntendido(), motivo) {
			t.Errorf("%s: quería %q entre los motivos, hay %v", fuente, motivo, d.NoEntendido())
		}
	}
	for _, fuente := range []string{
		"", "<p><br></p>", `<h2 style="text-align: right; line-height: 1.15;">a</h2>`, `<div>a</div>`,
		`<p><font color="#ff0000">a</font><span style="font-size: x-large; font-family: Georgia; background-color: rgb(255, 255, 0);">b</span></p>`,
		`<p><strong><em><u><s><strike>a</strike></s></u></em></strong></p>`, `<blockquote>a</blockquote><hr>`,
		`<p style="margin-left: 40px; text-indent: -12px;">a<sup>2</sup><sub>i</sub></p>`,
		`<p><span style="font-family: &quot;Times New Roman&quot;;">a</span></p>`,
		`<!--sytue-track--><p>a<ins data-track="1">b</ins></p>`,
	} {
		if d := Parse(fuente); !d.Editable() {
			t.Errorf("%s se entiende entero y dice que no: %v", fuente, d.NoEntendido())
		}
	}
}

func resumen(bloques []Bloque) string {
	var b strings.Builder
	for _, bl := range bloques {
		fmt.Fprintf(&b, "%d/%d/%d/%s|%s\n", bl.Tipo, bl.Nivel, bl.Sangria, bl.Marca, bl.Texto())
	}
	return b.String()
}

func TestBloques(t *testing.T) {
	quiero := `1/1/0/|Propuesta de servicio — Nordika
0/0/0/|Migración del correo y el almacenamiento de la organización a Sytue One, con continuidad del dominio propio.
1/2/0/|Alcance
0/0/1/•|40 buzones con su historial completo
0/0/1/•|Migración de ficheros compartidos a Sytue Drive
0/0/1/•|Calendario y contactos del equipo
1/2/0/|Calendario
0/0/0/|Ventana de migración: dos fines de semana, sin corte del servicio entre semana.
`
	bloques := Parse(nordika(t)).Bloques()
	if got := resumen(bloques); got != quiero {
		t.Errorf("la demo:\n%s\nquería:\n%s", got, quiero)
	}
	ultimo := bloques[len(bloques)-1]
	if len(ultimo.Tramos) != 3 || !ultimo.Tramos[1].Estilo.Negrita() || ultimo.Tramos[0].Estilo.Negrita() || ultimo.Tramos[1].Texto != "dos fines de semana" {
		t.Errorf("la negrita del último párrafo: %+v", ultimo.Tramos)
	}
	if bloques[0].Base.Tamano != 32 || bloques[0].Base.Peso != 700 || bloques[2].Base.Peso != 600 {
		t.Errorf("los títulos miden lo que en editor.css: %+v", bloques[0].Base)
	}

	for fuente, quiero := range map[string]string{
		// Blancos como en un navegador: rachas a un espacio, ninguno en los bordes, el duro se queda.
		"<p>  hola \n  <b> mundo </b>\t! a\u00a0 b</p>":                                                    "0/0/0/|hola mundo ! a\u00a0 b\n",
		"<p>a<br>b<br></p><p><br></p><p></p>":                                                              "0/0/0/|a\nb\n0/0/0/|\n0/0/0/|\n",
		"suelto\n<p>p</p>\n  \n<div>d<p>dentro</p>cola</div>":                                              "0/0/0/|suelto\n0/0/0/|p\n0/0/0/|d\n0/0/0/|dentro\n0/0/0/|cola\n",
		"<ol start=\"3\"><li>a</li><li>b<ul><li>c</li></ul></li></ol><ul><li><p>p1</p><p>p2</p></li></ul>": "0/0/1/3.|a\n0/0/1/4.|b\n0/0/2/•|c\n0/0/1/•|p1\n0/0/1/|p2\n",
		"<blockquote>cita<p>más</p></blockquote><hr><h4>h</h4>":                                            "2/0/0/|cita\n2/0/0/|más\n3/0/0/|\n1/4/0/|h\n",
		"<p>a<img src=x>b</p><script>no</script><style>p{}</style>":                                        "0/0/0/|a\uFFFCb\n",
	} {
		if got := resumen(Parse(fuente).Bloques()); got != quiero {
			t.Errorf("%q:\n%q\nquería:\n%q", fuente, got, quiero)
		}
	}
	if tr := Parse("<p>a<img src=x>b</p>").Bloques()[0].Tramos; len(tr) != 3 || tr[1].Objeto != "img" {
		t.Errorf("lo que no es texto se nombra: %+v", tr)
	}
}

func TestEstilos(t *testing.T) {
	tramo := func(fuente string) Estilo { return Parse(fuente).Bloques()[0].Tramos[0].Estilo }
	e := tramo(`<p style="text-align: center; line-height: 2;"><span style="font-weight: 600; font-style: italic; text-decoration: underline line-through; color: #f00; background-color: rgba(0, 0, 255, 0.5); font-size: 18pt;">x</span></p>`)
	if !e.Negrita() || !e.Cursiva || !e.Subrayado || !e.Tachado || e.Color.R != 255 || e.Fondo.B != 255 || e.Fondo.A != 128 || e.Tamano != 24 || e.Alineacion != Centro || e.Interlineado != 2 {
		t.Errorf("estilo: %+v", e)
	}
	if e := tramo(`<h1><span style="font-weight: normal; font-size: 50%;">x</span></h1>`); e.Negrita() || e.Tamano != 16 {
		t.Errorf("dentro de un título se puede quitar la negrita y achicar: %+v", e)
	}
	if e := tramo(`<p><a href="https://sytue.eu">x</a></p>`); e.Enlace != "https://sytue.eu" || !e.Subrayado || e.Color != tintaEnlace {
		t.Errorf("enlace: %+v", e)
	}
	if e := tramo(`<blockquote>x</blockquote>`); !e.Cursiva || e.Color != tintaCita {
		t.Errorf("cita: %+v", e)
	}
}

// abrir deja el bloque i listo para editar.
func abrir(t *testing.T, d *Documento, i int) *Edicion {
	t.Helper()
	bloques := d.Bloques()
	if i >= len(bloques) {
		t.Fatalf("no hay bloque %d en %s", i, d.HTML())
	}
	return bloques[i].Editar()
}

// despues comprueba lo que queda y que se puede guardar: que un navegador lo leería igual.
func despues(t *testing.T, caso string, d *Documento, quiero string) {
	t.Helper()
	if got := d.HTML(); got != quiero {
		t.Errorf("%s:\n  hay     %s\n  quería  %s", caso, got, quiero)
	}
	if !d.Guardable() {
		t.Errorf("%s: lo que queda no se puede guardar: %s (%v)", caso, d.HTML(), d.NoEntendido())
	}
}

func TestEscribir(t *testing.T) {
	// Editar UN bloque deja los demás byte a byte como estaban.
	muestra := nordika(t)
	d := Parse(muestra)
	e := abrir(t, d, 1)
	e.Cambiar(strings.Replace(e.Texto(), "Migración del correo", "Traslado del correo", 1))
	despues(t, "la demo", d, strings.Replace(muestra, "<p>Migración del correo", "<p>Traslado del correo", 1))

	for _, c := range []struct{ caso, fuente, quito, pongo, quiero string }{
		{"dentro de una negrita sigue la negrita", "<p>a<b>bc</b>d</p>", "abcd", "abXcd", "<p>a<b>bXc</b>d</p>"},
		{"al final hereda lo de la izquierda", `<p>a<span style="color: rgb(255, 0, 0);">b</span></p>`, "ab", "abc", `<p>a<span style="color: rgb(255, 0, 0);">bc</span></p>`},
		{"al principio, lo de la derecha", "<p><i>a</i>b</p>", "ab", "Xab", "<p><i>Xa</i>b</p>"},
		{"borrar un tramo entero se lleva su envoltorio", "<p>a<b>b</b>c</p>", "abc", "ac", "<p>ac</p>"},
		{"escribir sobre una selección hereda de lo que sustituye", "<p><b>Ada</b> y Grace.</p>", "Ada y Grace.", "Ada, Grace y Hedy.", "<p><b>Ada</b>, Grace y Hedy.</p>"},
		{"también si lo sustituido iba en negrita", "<p>a <b>bb</b> c</p>", "a bb c", "a XYZ c", "<p>a <b>XYZ</b> c</p>"},
		{"lo que no se pinta tampoco se pierde", `<p><span style="font-family: Georgia;">hola</span></p>`, "hola", "hola!", `<p><span style="font-family: Georgia;">hola!</span></p>`},
		{"un enlace sigue siendo el mismo enlace", `<p><a href="https://sytue.eu/?a=1&amp;b=2" target="_blank">sytue</a></p>`, "sytue", "Sytue", `<p><a href="https://sytue.eu/?a=1&amp;b=2" target="_blank">Sytue</a></p>`},
		{"vaciar deja el párrafo vacío de la web", "<p>a</p>", "a", "", "<p><br></p>"},
		{"escribir en el vacío", "<p><br></p>", "", "a", "<p>a</p>"},
		{"los espacios que un navegador se comería van duros", "<p>a</p>", "a", " a  b ", "<p>&nbsp;a &nbsp;b&nbsp;</p>"},
		{"un salto de línea es un <br>", "<p>ab</p>", "ab", "a\nb", "<p>a<br>b</p>"},
		{"lo que rompería el HTML se escapa", "<p>a</p>", "a", `<b> & "x"`, `<p>&lt;b&gt; &amp; "x"</p>`},
		{"ni retornos de carro ni nulos", "<p>a</p>", "a", "a\r\x00b", "<p>ab</p>"},
		{"una línea suelta se edita donde está", "suelto<p>p</p>", "suelto", "suelta", "suelta<p>p</p>"},
	} {
		d := Parse(c.fuente)
		e := abrir(t, d, 0)
		if e.Texto() != c.quito {
			t.Errorf("%s: el campo enseña %q y quería %q", c.caso, e.Texto(), c.quito)
		}
		e.Cambiar(c.pongo)
		despues(t, c.caso, d, c.quiero)
	}
}

func TestMarcas(t *testing.T) {
	for _, c := range []struct {
		caso, fuente string
		desde, hasta int
		m            Marca
		quiero       string
	}{
		{"poner negrita", "<p>hola</p>", 2, 4, Negrita, "<p>ho<b>la</b></p>"},
		{"poner cursiva dentro de una negrita", "<p><b>hola</b></p>", 0, 2, Cursiva, "<p><b><i>ho</i>la</b></p>"},
		{"quitar parte de una negrita", "<p><b>hola</b></p>", 1, 3, Negrita, "<p><b>h</b>ol<b>a</b></p>"},
		{"quitar la de Chrome", `<p><span style="font-weight: bold;">hola</span></p>`, 0, 4, Negrita, "<p>hola</p>"},
		{"quitarla sin llevarse el color", `<p><span style="font-weight: bold; color: rgb(255, 0, 0);">hola</span></p>`, 0, 4, Negrita, `<p><span style="font-weight: bold; color: rgb(255, 0, 0);"><span style="font-weight: normal;">hola</span></span></p>`},
		{"en un título la negrita se niega", "<h1>hola</h1>", 0, 2, Negrita, `<h1><span style="font-weight: normal;">ho</span>la</h1>`},
		{"y se vuelve a poner quitando la negación", `<h1><span style="font-weight: normal;">ho</span>la</h1>`, 0, 2, Negrita, "<h1>hola</h1>"},
		{"en una cita la cursiva se niega", "<blockquote>hola</blockquote>", 0, 4, Cursiva, `<blockquote><span style="font-style: normal;">hola</span></blockquote>`},
		{"mezcla: si no la lleva todo, se pone a todo", "<p>a<b>b</b>c</p>", 0, 3, Negrita, "<p><b>a</b><b>b</b><b>c</b></p>"},
		{"el enlace se queda por fuera", `<p><a href="x">hola</a></p>`, 0, 4, Negrita, `<p><a href="x"><b>hola</b></a></p>`},
	} {
		d := Parse(c.fuente)
		abrir(t, d, 0).Alternar(c.desde, c.hasta, c.m)
		despues(t, c.caso, d, c.quiero)
	}

	// Con el cursor quieto: «negrita» y a escribir.
	d := Parse("<p>hola</p>")
	e := abrir(t, d, 0)
	if e.Tiene(4, 4, Negrita) {
		t.Error("al final de un texto corriente no hay negrita")
	}
	e.Alternar(4, 4, Negrita)
	if !e.Tiene(4, 4, Negrita) {
		t.Error("pedida la negrita, el botón tiene que decirlo")
	}
	despues(t, "pedirla no cambia nada", d, "<p>hola</p>")
	e.Cambiar("hola!")
	e.Cambiar("hola!!")
	despues(t, "lo escrito va en negrita, y lo siguiente la hereda", d, "<p>hola<b>!!</b></p>")
	if !e.Tiene(4, 6, Negrita) || e.Tiene(3, 6, Negrita) {
		t.Error("Tiene es de TODO el tramo")
	}
}

func TestPartirYFundir(t *testing.T) {
	for _, c := range []struct {
		caso, fuente string
		bloque, pos  int
		quiero       string
	}{
		{"en medio de un párrafo", `<p style="text-align: center;">ho<b>la</b></p>`, 0, 3, `<p style="text-align: center;">ho<b>l</b></p><p style="text-align: center;"><b>a</b></p>`},
		{"al final: uno vacío detrás", "<p>a</p><p>b</p>", 0, 1, "<p>a</p><p><br></p><p>b</p>"},
		{"al principio: uno vacío delante", "<p>a</p>", 0, 0, "<p><br></p><p>a</p>"},
		{"tras un título se sigue en texto corriente", "<h2>Título</h2>", 0, 6, "<h2>Título</h2><p><br></p>"},
		{"en medio de un título, dos títulos", "<h2>ab</h2>", 0, 1, "<h2>a</h2><h2>b</h2>"},
		{"un elemento de lista más", "<ul><li>ab</li></ul>", 0, 1, "<ul><li>a</li><li>b</li></ul>"},
		{"Intro en el último elemento vacío sale de la lista", "<ul><li>a</li><li><br></li></ul><p>z</p>", 1, 0, "<ul><li>a</li></ul><p><br></p><p>z</p>"},
		{"en una línea suelta es un salto", "suelto<p>p</p>", 0, 3, "sue<br>lto<p>p</p>"},
	} {
		d := Parse(c.fuente)
		abrir(t, d, c.bloque).Partir(c.pos)
		despues(t, c.caso, d, c.quiero)
	}

	for _, c := range []struct {
		caso, fuente string
		bloque       int
		quiero       string
		en           int
	}{
		{"un párrafo se une al de arriba", "<p>ho</p><p><b>la</b></p><p>z</p>", 1, "<p>ho<b>la</b></p><p>z</p>", 2},
		{"un título vuelve a ser texto", "<p>a</p><h1>b</h1>", 1, "<p>a</p><p>b</p>", 0},
		{"un elemento sale de su lista, que se parte", "<ul><li>a</li><li>b</li><li>c</li></ul>", 1, "<ul><li>a</li></ul><p>b</p><ul><li>c</li></ul>", 0},
		{"el primero sale por arriba", "<ol><li>a</li><li>b</li></ol>", 0, "<p>a</p><ol><li>b</li></ol>", 0},
		{"y si era el único, la lista se va", "<ul><li>a</li></ul>", 0, "<p>a</p>", 0},
		{"sobre una regla, se lleva la regla", "<p>a</p><hr><p>b</p>", 2, "<p>a</p><p>b</p>", 0},
		{"un párrafo se une al último elemento de la lista", "<ul><li>a</li></ul><p>b</p>", 1, "<ul><li>ab</li></ul>", 1},
		{"al vaciarse, el contenedor se va con él", "<p>a</p><div><p>b</p></div>", 1, "<p>ab</p>", 1},
		{"el primero del documento no tiene con quién", "<p>a</p>", 0, "<p>a</p>", -1},
	} {
		d := Parse(c.fuente)
		lugar, en, ok := d.Fundir(d.Bloques(), c.bloque)
		despues(t, c.caso, d, c.quiero)
		if !ok {
			en = -1
		}
		if en != c.en {
			t.Errorf("%s: el cursor queda en %d y quería %d", c.caso, en, c.en)
		}
		if ok && indice(d.Bloques(), lugar) < 0 {
			t.Errorf("%s: el lugar devuelto no es ningún bloque", c.caso)
		}
	}
}

func TestTiposYListas(t *testing.T) {
	d := Parse(`<p style="text-align: center;">ho<b>la</b></p>`)
	if !abrir(t, d, 0).CambiarTipo("h2") {
		t.Fatal("un párrafo se puede hacer título")
	}
	despues(t, "a título", d, `<h2 style="text-align: center;">ho<b>la</b></h2>`)
	abrir(t, d, 0).CambiarTipo("blockquote")
	despues(t, "a cita", d, `<blockquote style="text-align: center;">ho<b>la</b></blockquote>`)
	if abrir(t, d, 0).CambiarTipo("script") || abrir(t, Parse("<ul><li>a</li></ul>"), 0).CambiarTipo("h1") || abrir(t, Parse("suelto<p>p</p>"), 0).CambiarTipo("h1") {
		t.Error("ni etiquetas de fuera de la lista, ni elementos de lista, ni líneas sueltas")
	}

	for _, c := range []struct {
		caso, fuente string
		bloque       int
		ordenada     bool
		quiero       string
	}{
		{"un párrafo a viñetas", "<p>a</p><p>b</p>", 1, false, "<p>a</p><ul><li>b</li></ul>"},
		{"sigue la lista que tiene encima", "<ul><li>a</li></ul><p><b>b</b></p>", 1, false, "<ul><li>a</li><li><b>b</b></li></ul>"},
		{"pero no una de la otra clase", "<ul><li>a</li></ul><p>b</p>", 1, true, "<ul><li>a</li></ul><ol><li>b</li></ol>"},
		{"de viñetas a numerada cambia la lista", "<ul><li>a</li><li>b</li></ul>", 0, true, "<ol><li>a</li><li>b</li></ol>"},
		{"otra vez lo mismo lo saca", "<ol><li>a</li><li>b</li></ol>", 1, true, "<ol><li>a</li></ol><p>b</p>"},
	} {
		d := Parse(c.fuente)
		lugar, ok := abrir(t, d, c.bloque).AlternarLista(c.ordenada)
		despues(t, c.caso, d, c.quiero)
		if !ok || indice(d.Bloques(), lugar) < 0 {
			t.Errorf("%s: el lugar devuelto no es ningún bloque (%v)", c.caso, ok)
		}
	}
}

// Miles de <div> anidados no tumban nada: se leen hasta donde se llega, y no se editan.
func TestAnidamientoPatologico(t *testing.T) {
	d := Parse(strings.Repeat("<div>", 400) + "hondo" + strings.Repeat("</div>", 400))
	if d.Editable() || !slices.Contains(d.NoEntendido(), "anidamiento") {
		t.Errorf("motivos: %v", d.NoEntendido())
	}
	_ = d.Bloques()
}

// Lo que el folio de Fyne necesita además de lo que ya tenía Gio: sustituir sabiendo dónde
// está el cursor, las otras cuatro marcas, las propiedades de <span style> y las de párrafo.
func TestLoQueMandaLaCinta(t *testing.T) {
	for _, c := range []struct {
		caso, fuente string
		hacer        func(*Edicion)
		quiero       string
	}{
		{"sustituir sin adivinar", "<p>aa<b>a</b></p>", func(e *Edicion) { e.Sustituir(2, 2, "a") }, "<p>aaa<b>a</b></p>"},
		{"sustituir hereda lo que sustituye", "<p>a<b>bc</b>d</p>", func(e *Edicion) { e.Sustituir(1, 3, "X") }, "<p>a<b>X</b>d</p>"},
		{"subrayar", "<p>hola</p>", func(e *Edicion) { e.Alternar(0, 2, Subrayado) }, "<p><u>ho</u>la</p>"},
		{"tachar como Chrome", "<p>hola</p>", func(e *Edicion) { e.Alternar(0, 4, Tachado) }, "<p><strike>hola</strike></p>"},
		{"quitar el subrayado", "<p><u>hola</u></p>", func(e *Edicion) { e.Alternar(0, 4, Subrayado) }, "<p>hola</p>"},
		{
			"el subrayado de fuera no se niega", `<p><span style="text-decoration-line: underline; color: rgb(1, 2, 3);">hola</span></p>`,
			func(e *Edicion) { e.Alternar(0, 4, Subrayado) }, `<p><span style="text-decoration-line: underline; color: rgb(1, 2, 3);">hola</span></p>`,
		},
		{"superíndice quita el subíndice", "<p>x<sub>2</sub></p>", func(e *Edicion) { e.Alternar(1, 2, Superindice) }, "<p>x<sup>2</sup></p>"},
		{
			"color como Chrome", "<p>hola</p>", func(e *Edicion) { e.PonerPropiedad(0, 2, "color", "rgb(192, 57, 43)") },
			`<p><span style="color: rgb(192, 57, 43);">ho</span>la</p>`,
		},
		{
			"cambiar de color no apila spans", `<p><span style="color: rgb(1, 2, 3);">hola</span></p>`,
			func(e *Edicion) { e.PonerPropiedad(0, 4, "color", "rgb(4, 5, 6)") }, `<p><span style="color: rgb(4, 5, 6);">hola</span></p>`,
		},
		{
			"quitar el resalte", `<p><span style="background-color: rgb(1, 2, 3);">hola</span></p>`,
			func(e *Edicion) { e.PonerPropiedad(0, 4, "background-color", "") }, "<p>hola</p>",
		},
		{
			"quitar formato deja el enlace", `<p><a href="x"><b><span style="color: rgb(1, 2, 3);">hola</span></b></a></p>`,
			func(e *Edicion) { e.QuitarFormato(0, 4) }, `<p><a href="x">hola</a></p>`,
		},
		{"alinear", "<p>a</p>", func(e *Edicion) { e.PonerBloque("text-align", "center") }, `<p style="text-align: center;">a</p>`},
		{
			"cambiar la alineación respeta lo demás", `<p style="line-height: 2; text-align: right;">a</p>`,
			func(e *Edicion) { e.PonerBloque("text-align", "center") }, `<p style="line-height: 2; text-align: center;">a</p>`,
		},
		{
			"quitar la última propiedad quita el atributo", `<p style="text-align: center;">a</p>`,
			func(e *Edicion) { e.PonerBloque("text-align", "") }, "<p>a</p>",
		},
	} {
		d := Parse(c.fuente)
		c.hacer(abrir(t, d, 0))
		despues(t, c.caso, d, c.quiero)
	}

	// Pedir un color con el cursor quieto lo lleva lo siguiente que se escriba.
	d := Parse("<p>ab</p>")
	e := abrir(t, d, 0)
	e.PonerPropiedad(2, 2, "color", "rgb(1, 2, 3)")
	e.Sustituir(2, 2, "c")
	despues(t, "color pendiente", d, `<p>ab<span style="color: rgb(1, 2, 3);">c</span></p>`)
	if f := e.Formato(2, 3); f.Color.B != 3 {
		t.Errorf("el formato de lo escrito: %+v", f)
	}
	if f := Parse(`<p><span style="font-family: Georgia; font-size: x-large;">a</span></p>`).Bloques()[0].Tramos[0].Estilo; f.Familia != "Georgia" || f.Tamano != 24 {
		t.Errorf("fuente y tamaño: %+v", f)
	}
}

func TestUnirYQuitar(t *testing.T) {
	for _, c := range []struct {
		caso, fuente string
		bloque       int
		quiero       string
		en           int
	}{
		{"un título se une sin dejar de serlo el de arriba", "<p>ab</p><h1>cd</h1>", 1, "<p>abcd</p>", 2},
		{"un elemento de lista se une al párrafo", "<p>a</p><ul><li>b</li></ul>", 1, "<p>ab</p>", 1},
		{"una regla debajo se va", "<p>a</p><hr><p>b</p>", 1, "<p>a</p><p>b</p>", 1},
		{"con una tabla no se une", "<p>a</p><table><tbody><tr><td>x</td></tr></tbody></table>", 1, "<p>a</p><table><tbody><tr><td>x</td></tr></tbody></table>", -1},
	} {
		d := Parse(c.fuente)
		_, en, ok := d.Unir(d.Bloques(), c.bloque)
		if got := d.HTML(); got != c.quiero {
			t.Errorf("%s:\n  hay     %s\n  quería  %s", c.caso, got, c.quiero)
		}
		if !ok {
			en = -1
		}
		if en != c.en {
			t.Errorf("%s: el cursor queda en %d y quería %d", c.caso, en, c.en)
		}
	}
	d := Parse("<p>a</p><ul><li>b</li></ul><p>c</p>")
	d.Quitar(d.Bloques(), 1)
	despues(t, "quitar se lleva la lista vacía", d, "<p>a</p><p>c</p>")
	v := Parse("")
	if !v.Asegurar() || v.HTML() != "<p><br></p>" || v.Asegurar() {
		t.Errorf("un documento vacío recibe UN párrafo donde escribir: %s", v.HTML())
	}
}

// Las tablas que escribe la web —la de «Insertar tabla», con y sin fila de encabezado, y
// las que dejan combinar celdas, el color de celda y la alineación vertical— vuelven byte a
// byte y se editan. Cada celda es su bloque, en su sitio de la rejilla. Se vio en ROJO
// quitando atom.Td de lo que se entiende, y contando las columnas sin los rowspan de arriba.
func TestLasTablasVanYVuelven(t *testing.T) {
	casos := []string{
		// `tableHtml` de canvas/exec.ts, 2 × 3, y el de Insertar con la fila de encabezado.
		`<table><tbody><tr><td><br></td><td><br></td><td><br></td></tr><tr><td><br></td><td><br></td><td><br></td></tr></tbody></table><p><br></p>`,
		`<table><tbody><tr><th><br></th><th><br></th></tr><tr><td><br></td><td><br></td></tr></tbody></table><p><br></p>`,
		// Combinar (mergeCells), color y alineación (setCellProp), y texto con formato dentro.
		`<table><tbody><tr><td rowspan="2" style="background-color: rgb(254, 240, 138); vertical-align: middle;">a</td><td colspan="2">b <b>c</b></td></tr>` +
			`<tr><td><p>d</p><p>e</p></td><td style="vertical-align: bottom;">f</td></tr></tbody></table>`,
	}
	for _, fuente := range casos {
		d := Parse(fuente)
		if got := d.HTML(); got != fuente || !d.Guardable() {
			t.Errorf("no vuelve igual o no se edita (%v):\n  leído    %s\n  escrito  %s", d.NoEntendido(), fuente, got)
		}
	}
	bl := Parse(casos[2]).Bloques()
	type sitio struct {
		texto                string
		fila, col, fs, cs    int
		vertical             string
		fondo, mismaQueAntes bool
	}
	got := make([]sitio, 0, len(bl))
	for i, b := range bl {
		c := b.Celda
		got = append(got, sitio{b.Texto(), c.Fila, c.Col, c.Filas, c.Cols, c.Vertical, c.Fondo.A > 0, i > 0 && bl[i-1].Celda == c})
	}
	quiero := []sitio{
		{"a", 0, 0, 2, 1, "middle", true, false},
		{"b c", 0, 1, 1, 2, "top", false, false},
		{"d", 1, 1, 1, 1, "top", false, false}, // la columna 0 la ocupa la «a» de arriba
		{"e", 1, 1, 1, 1, "top", false, true},
		{"f", 1, 2, 1, 1, "bottom", false, false},
	}
	if !slices.Equal(got, quiero) {
		t.Errorf("las celdas:\n  hay     %+v\n  quería  %+v", got, quiero)
	}
	if tb := bl[0].Celda.Tabla; tb.Filas != 2 || tb.Cols != 3 || len(tb.Celdas) != 4 {
		t.Errorf("la rejilla es de %d × %d con %d celdas", tb.Filas, tb.Cols, len(tb.Celdas))
	}
	if bl[0].Tramos[0].Estilo.Fondo.A != 0 {
		t.Error("el fondo es de la celda, no del texto: no se hereda")
	}
	if th := Parse(casos[1]).Bloques()[0]; !th.Celda.Cabecera || !th.Base.Negrita() {
		t.Error("un <th> es cabecera y va en negrita")
	}

	// La tabla del servicio no lleva <tbody>: un navegador se lo pone al leerla, así que no
	// vuelve byte a byte —tampoco en la web— pero sí como el mismo árbol, y se edita.
	f := Parse(fidelidad(t))
	if !f.Guardable() {
		t.Errorf("el documento del servicio se tiene que poder editar: %v", f.NoEntendido())
	}
	var celdas []string
	for _, b := range f.Bloques() {
		if b.Celda != nil {
			celdas = append(celdas, fmt.Sprintf("%s@%d,%d", b.Texto(), b.Celda.Fila, b.Celda.Col))
		}
	}
	if strings.Join(celdas, " ") != "c1@0,0 c2@0,1 c3@1,0 c4@1,1" {
		t.Errorf("las celdas del documento del servicio: %v", celdas)
	}
}

// Escribir en una celda cambia esa celda y nada más; Retroceso al principio de una celda y
// Suprimir al final no la funden con la de al lado; vaciar una celda deja el `<br>` de la
// web y no se lleva el <td>; e Intro en el texto suelto de una celda lo parte en párrafos.
// Se vio en ROJO quitando la comprobación de celda de Fundir: «b» se unía a «a».
func TestUnaCeldaSeQuedaEnSuSitio(t *testing.T) {
	const tabla = `<table><tbody><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></tbody></table><p>fuera</p>`
	d := Parse(tabla)
	e := abrir(t, d, 1)
	e.Sustituir(1, 1, "!")
	despues(t, "escribir en una celda", d, `<table><tbody><tr><td>a</td><td>b!</td></tr><tr><td>c</td><td>d</td></tr></tbody></table><p>fuera</p>`)

	d = Parse(tabla)
	if _, _, ok := d.Fundir(d.Bloques(), 1); ok {
		t.Error("Retroceso al principio de «b» la ha fundido con «a»")
	}
	if _, _, ok := d.Fundir(d.Bloques(), 4); ok {
		t.Error("Retroceso al principio de lo de debajo se ha metido en la tabla")
	}
	if _, _, ok := d.Unir(d.Bloques(), 2); ok {
		t.Error("Suprimir al final de «b» se ha traído la «c»")
	}
	despues(t, "nada se ha movido", d, tabla)

	d.Quitar(d.Bloques(), 3)
	despues(t, "vaciar una celda", d, `<table><tbody><tr><td>a</td><td>b</td></tr><tr><td>c</td><td><br></td></tr></tbody></table><p>fuera</p>`)

	d = Parse(`<table><tbody><tr><td>abc</td></tr></tbody></table>`)
	if _, ok := abrir(t, d, 0).Partir(2); !ok {
		t.Fatal("Intro en una celda no ha partido")
	}
	despues(t, "Intro en una celda", d, `<table><tbody><tr><td><p>ab</p><p>c</p></td></tr></tbody></table>`)
	if bl := d.Bloques(); len(bl) != 2 || bl[0].Celda != bl[1].Celda {
		t.Errorf("los dos párrafos son de la misma celda: %+v", bl)
	}
	d = Parse(`<table><tbody><tr><td>abc</td></tr></tbody></table>`)
	if !abrir(t, d, 0).CambiarTipo("h2") {
		t.Fatal("una celda se puede hacer título")
	}
	despues(t, "título en una celda", d, `<table><tbody><tr><td><h2>abc</h2></td></tr></tbody></table>`)
}

// png3x2 es una data: URL de imagen. El modelo no la lee: la maqueta, sí (docus).
const png3x2 = "data:image/png;base64,iVBORw0KGgo="

// Las imágenes que escribe la web —la de `insertImage`, sin nada más, y la que ya ha
// tocado el manejo de imagen, con sus data-*, su estilo y contenteditable— vuelven byte a
// byte y se editan. Ocupan UNA runa, y la maqueta sabe cuánto miden.
func TestLasImagenesVanYVuelven(t *testing.T) {
	gestionada := `<img src="` + png3x2 + `" data-w="120" data-h="80" data-wrap="topBottom" data-align="center" data-sytue-image="" alt="Logo" contenteditable="false" style="width:120px;height:80px;max-width:none;display:block;clear:both;margin-left:auto;margin-right:auto;margin-top:14px;margin-bottom:14px">`
	for _, fuente := range []string{
		`<p>a<img src="` + png3x2 + `">b</p>`,
		`<p>` + gestionada + `</p>`,
		`<table><tbody><tr><td><img src="` + png3x2 + `"></td></tr></tbody></table>`,
	} {
		d := Parse(fuente)
		if got := d.HTML(); got != fuente || !d.Guardable() {
			t.Errorf("no vuelve igual o no se edita (%v):\n  leído    %s\n  escrito  %s", d.NoEntendido(), fuente, got)
		}
	}
	b := Parse(`<p>a<img src="` + png3x2 + `">b</p>`).Bloques()[0]
	if b.Texto() != "a"+string(ObjetoRuna)+"b" || b.Tramos[1].Imagen == nil || b.Tramos[1].Imagen.Gestionada {
		t.Errorf("la imagen es la runa de en medio: %q %+v", b.Texto(), b.Tramos)
	}
	im := *Parse(`<p>` + gestionada + `</p>`).Bloques()[0].Tramos[0].Imagen
	im.nodo = nil
	if im != (Imagen{Src: png3x2, Alt: "Logo", Ajuste: "topBottom", Alinear: "center", Ancho: 120, Alto: 80, Gestionada: true}) {
		t.Errorf("lo leído de la imagen: %+v", im)
	}
}

// Escribir, dar formato, partir y fundir junto a una imagen no la toca: vuelve el MISMO
// <img>, fuera de lo que se escribe. Se vio en ROJO quitando `sinObjeto` de `hereda`: lo
// tecleado detrás de la imagen se metía DENTRO del <img>.
func TestEscribirJuntoAUnaImagenNoLaToca(t *testing.T) {
	img := `<img src="` + png3x2 + `" data-sytue-image="" style="width:30px">`
	d := Parse(`<p>a` + img + `b</p>`)
	e := abrir(t, d, 0)
	e.Sustituir(2, 2, "X")
	despues(t, "tras la imagen", d, `<p>a`+img+`Xb</p>`)
	e.Sustituir(1, 1, "Y")
	despues(t, "delante", d, `<p>aY`+img+`Xb</p>`)
	e.Marcar(0, e.Largo(), Negrita, true)
	despues(t, "negrita con la imagen dentro", d, `<p><b>aY`+img+`Xb</b></p>`)
	e.QuitarFormato(0, e.Largo())
	despues(t, "quitar el formato", d, `<p>aY`+img+`Xb</p>`)
	if _, ok := e.Partir(3); !ok {
		t.Fatal("Intro tras la imagen no parte")
	}
	despues(t, "Intro tras la imagen", d, `<p>aY`+img+`</p><p>Xb</p>`)
	if _, _, ok := d.Unir(d.Bloques(), 1); !ok {
		t.Fatal("Suprimir al final no une")
	}
	despues(t, "y volver a unir", d, `<p>aY`+img+`Xb</p>`)
	abrir(t, d, 0).Sustituir(0, 0, "pegado ￼")
	despues(t, "una runa de objeto pegada no se escribe", d, `<p>pegado aY`+img+`Xb</p>`)
	e = abrir(t, d, 0)
	e.Sustituir(8, 10, "")
	despues(t, "borrar la imagen con lo de al lado", d, `<p>pegado aXb</p>`)
}

// «Insertar tabla» deja el HTML de la web (`tableHtml`: celdas `<br>`, <th> en la fila de
// encabezado y el párrafo de escape detrás) donde está el cursor: en un párrafo vacío lo
// sustituye, al principio va delante, al final detrás, en medio lo parte y en una celda va
// detrás de la tabla. El cursor queda en la primera celda. Se vio en ROJO insertando
// siempre al final del documento.
func TestInsertarTablaComoLaWeb(t *testing.T) {
	tabla := HTMLTabla(2, 2, true)
	if tabla != `<table><tbody><tr><th><br></th><th><br></th></tr><tr><td><br></td><td><br></td></tr></tbody></table><p><br></p>` {
		t.Fatalf("la tabla de la web: %s", tabla)
	}
	for _, c := range []struct {
		caso, fuente string
		bloque, pos  int
		quiero       string
	}{
		{"en un párrafo vacío", `<p>x</p><p><br></p>`, 1, 0, `<p>x</p>` + tabla},
		{"al final", `<p>ab</p><p>z</p>`, 0, 2, `<p>ab</p>` + tabla + `<p>z</p>`},
		{"al principio", `<p>ab</p>`, 0, 0, tabla + `<p>ab</p>`},
		{"en medio", `<p>a<b>b</b></p>`, 0, 1, `<p>a</p>` + tabla + `<p><b>b</b></p>`},
		{"en una lista", `<ul><li>a</li><li>b</li></ul>`, 0, 1, `<ul><li>a</li><li>b</li></ul>` + tabla},
		{"en una celda", `<table><tbody><tr><td>c</td></tr></tbody></table>`, 0, 1, `<table><tbody><tr><td>c</td></tr></tbody></table>` + tabla},
	} {
		d := Parse(c.fuente)
		l, ok := d.InsertarHTML(d.Bloques(), c.bloque, c.pos, tabla)
		despues(t, c.caso, d, c.quiero)
		if i := indice(d.Bloques(), l); !ok || i < 0 || d.Bloques()[i].Celda == nil || !d.Bloques()[i].Celda.Cabecera {
			t.Errorf("%s: el cursor no va a la primera celda (%v)", c.caso, ok)
		}
	}
	// El salto de página y la regla son de la web tal cual, y el salto se entiende: se
	// edita alrededor y Suprimir lo quita como a una regla.
	d := Parse(`<p>a</p>`)
	d.InsertarHTML(d.Bloques(), 0, 1, HTMLSaltoPagina)
	despues(t, "salto de página", d, `<p>a</p>`+HTMLSaltoPagina)
	if bl := d.Bloques(); len(bl) != 3 || bl[1].Tipo != SaltoPagina || bl[1].Arriba != 24 {
		t.Errorf("el salto de página no es su bloque: %+v", bl)
	}
	if _, _, ok := d.Unir(d.Bloques(), 1); !ok {
		t.Error("Suprimir antes del salto no lo quita")
	}
	despues(t, "sin el salto", d, `<p>a</p><p><br></p>`)
}

// Las órdenes de tabla hacen lo que `tableOp`, `mergeCells`, `splitCell` y `setCellProp` de
// la web. Se vio en ROJO combinando sin comprobar el rectángulo: dos celdas en diagonal se
// fundían y la tabla quedaba rota.
func TestLasOrdenesDeTabla(t *testing.T) {
	const base = `<table><tbody><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></tbody></table>`
	celda := func(d *Documento, texto string) *Celda {
		for _, b := range d.Bloques() {
			if b.Texto() == texto {
				return b.Celda
			}
		}
		t.Fatalf("no hay celda %q en %s", texto, d.HTML())
		return nil
	}
	for _, c := range []struct {
		orden  OrdenTabla
		quiero string
	}{
		{FilaEncima, `<table><tbody><tr><td><br></td><td><br></td></tr><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></tbody></table>`},
		{FilaDebajo, `<table><tbody><tr><td>a</td><td>b</td></tr><tr><td><br></td><td><br></td></tr><tr><td>c</td><td>d</td></tr></tbody></table>`},
		{ColIzquierda, `<table><tbody><tr><td><br></td><td>a</td><td>b</td></tr><tr><td><br></td><td>c</td><td>d</td></tr></tbody></table>`},
		{ColDerecha, `<table><tbody><tr><td>a</td><td><br></td><td>b</td></tr><tr><td>c</td><td><br></td><td>d</td></tr></tbody></table>`},
		{BorrarFila, `<table><tbody><tr><td>c</td><td>d</td></tr></tbody></table>`},
		{BorrarColumna, `<table><tbody><tr><td>b</td></tr><tr><td>d</td></tr></tbody></table>`},
		{BorrarTabla, `<p><br></p>`},
	} {
		d := Parse(base)
		if _, ok := d.Tabla(celda(d, "a"), c.orden); !ok {
			t.Errorf("orden %d: no ha hecho nada", c.orden)
		}
		despues(t, fmt.Sprint("orden ", c.orden), d, c.quiero)
	}
	// La última fila y la última columna se llevan la tabla, como en la web.
	d := Parse(`<p>x</p><table><tbody><tr><td>a</td></tr></tbody></table>`)
	d.Tabla(celda(d, "a"), BorrarFila)
	despues(t, "borrar la última fila", d, `<p>x</p>`)

	d = Parse(base)
	if _, ok := d.Combinar([]*Celda{celda(d, "a"), celda(d, "d")}); ok {
		t.Error("dos celdas en diagonal no son un rectángulo")
	}
	despues(t, "sin combinar", d, base)
	if _, ok := d.Combinar([]*Celda{celda(d, "a"), celda(d, "b")}); !ok {
		t.Fatal("dos celdas de una fila se combinan")
	}
	despues(t, "combinar", d, `<table><tbody><tr><td rowspan="1" colspan="2">a b</td></tr><tr><td>c</td><td>d</td></tr></tbody></table>`)
	if _, ok := d.Dividir(celda(d, "a b")); !ok {
		t.Fatal("una celda combinada se divide")
	}
	despues(t, "dividir", d, `<table><tbody><tr><td rowspan="1" colspan="1">a b</td><td><br></td></tr><tr><td>c</td><td>d</td></tr></tbody></table>`)

	d = Parse(base)
	d.PonerCeldas([]*Celda{celda(d, "a")}, "background-color", "#FEF08A")
	d.PonerCeldas([]*Celda{celda(d, "a")}, "vertical-align", "middle")
	despues(t, "color y alineación", d, `<table><tbody><tr><td style="background-color: rgb(254, 240, 138); vertical-align: middle;">a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></tbody></table>`)
	d.PonerCeldas([]*Celda{celda(d, "a")}, "background-color", "transparent")
	despues(t, "sin color", d, `<table><tbody><tr><td style="vertical-align: middle;">a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></tbody></table>`)
}

// «Imagen» deja el `<img src>` de `insertImage` en el cursor, y «Enlace» el `<a href>` de
// EnlacePanel en lugar de la selección, sin meter un enlace dentro de otro.
func TestInsertarImagenYEnlace(t *testing.T) {
	d := Parse(`<p>ab</p>`)
	abrir(t, d, 0).InsertarImagen(1, "data:image/png;base64,AA")
	despues(t, "imagen", d, `<p>a<img src="data:image/png;base64,AA">b</p>`)

	d = Parse(`<p>hola mundo</p>`)
	abrir(t, d, 0).InsertarEnlace(5, 10, "https://sytue.eu", "Sytue")
	despues(t, "enlace sobre la selección", d, `<p>hola <a href="https://sytue.eu">Sytue</a></p>`)
	d = Parse(`<p><a href="https://a.eu">ab</a></p>`)
	abrir(t, d, 0).InsertarEnlace(1, 1, "https://b.eu", "X")
	despues(t, "enlace dentro de otro", d, `<p><a href="https://a.eu">a</a><a href="https://b.eu">X</a><a href="https://a.eu">b</a></p>`)
}

// El ancla de un comentario es el `<mark data-comment-id>` de la web: se entiende (y se
// pinta con su fondo), se pone en un tramo aunque cruce formatos, se encuentra y se quita
// dejando el documento como estaba. Un <mark> sin id no se entiende. Se vio en ROJO
// quitando `fundirTextos` de QuitarAncla: «ho» y «la » quedaban en dos textos y lo escrito
// ya no se leía como el mismo árbol.
func TestElAnclaDeUnComentario(t *testing.T) {
	const web = `<p>ho<mark data-comment-id="m1">la <b>mun</b></mark><b>do</b></p>`
	if d := Parse(web); d.HTML() != web || !d.Guardable() {
		t.Errorf("el ancla de la web no vuelve igual o no se edita: %v", d.NoEntendido())
	}
	if b := Parse(web).Bloques()[0]; b.Tramos[1].Estilo.Fondo != FondoAncla {
		t.Errorf("lo comentado se pinta con %v y la web con %v", b.Tramos[1].Estilo.Fondo, FondoAncla)
	}
	if d := Parse(`<p><mark>x</mark></p>`); d.Editable() {
		t.Error("un <mark> sin id es el amarillo del navegador, que no se pinta")
	}
	d := Parse(`<p>hola <b>mundo</b></p>`)
	abrir(t, d, 0).Anclar(2, 8, "m2")
	despues(t, "anclar", d, `<p>ho<mark data-comment-id="m2">la </mark><b><mark data-comment-id="m2">mun</mark>do</b></p>`)
	if desde, hasta, ok := d.Bloques()[0].Ancla("m2"); !ok || desde != 2 || hasta != 8 {
		t.Errorf("el ancla está en [%d, %d) (%v) y es [2, 8)", desde, hasta, ok)
	}
	if !d.QuitarAncla("m2") || d.QuitarAncla("m2") {
		t.Error("quitar el ancla dice si la había")
	}
	despues(t, "quitar el ancla", d, `<p>hola <b>mundo</b></p>`)
}

// «Tabla de contenidos» deja el `<nav data-toc>` de la web: a cada h1–h3 sin id le pone
// `h-<n>` saltando los ocupados, y enlaza cada entrada a su título con la sangría de su
// nivel en em; «Actualizar» lo rehace en su sitio. «Nota al pie» deja el marcador
// `<sup data-footnote-ref>` en el cursor y la nota en la `<section data-footnotes>` del
// final, que crea si falta. Se vio en ROJO numerando los ids sin saltar los ocupados: dos
// títulos acababan con `h-1`.
func TestElIndiceYLasNotasComoLaWeb(t *testing.T) {
	d := Parse(`<h1>Uno</h1><p>a</p><h2>Dos</h2><h3 id="h-1">Tres</h3>`)
	if _, ok := d.InsertarIndice(d.Bloques(), 1, 1, "Índice"); !ok {
		t.Fatal("no se ha insertado el índice")
	}
	nav := `<nav data-toc=""><p><b>Índice</b></p><ol>` +
		`<li data-level="1" style="margin-left:0em"><a href="#h-2">Uno</a></li>` +
		`<li data-level="2" style="margin-left:1.5em"><a href="#h-3">Dos</a></li>` +
		`<li data-level="3" style="margin-left:3em"><a href="#h-1">Tres</a></li></ol></nav>`
	despues(t, "índice", d, `<h1 id="h-2">Uno</h1><p>a</p>`+nav+`<h2 id="h-3">Dos</h2><h3 id="h-1">Tres</h3>`)
	var enNav []string
	for _, b := range d.Bloques() {
		if b.Recuadro != nil {
			enNav = append(enNav, fmt.Sprintf("%s@%g", b.Texto(), b.Base.MargenIzq))
		}
	}
	if strings.Join(enNav, " ") != "Índice@0 Uno@0 Dos@24 Tres@48" {
		t.Errorf("los bloques del índice: %v", enNav)
	}
	abrir(t, d, 0).Sustituir(0, 3, "Primero")
	if !d.ActualizarIndice("Índice") {
		t.Fatal("con índice, «Actualizar» lo rehace")
	}
	if got := d.HTML(); !strings.Contains(got, `<a href="#h-2">Primero</a>`) || strings.Count(got, "<nav") != 1 {
		t.Errorf("el índice actualizado: %s", got)
	}

	d = Parse(`<p>hola</p>`)
	l, ok := d.InsertarNotaPie(d.Bloques(), 0, 4, "Texto de la nota")
	if !ok {
		t.Fatal("no se ha insertado la nota")
	}
	despues(t, "nota al pie", d, `<p>hola<sup data-footnote-ref="1" id="fnref-1"><a href="#fn-1">1</a></sup></p>`+
		`<section data-footnotes=""><hr><ol><li id="fn-1" value="1">Texto de la nota</li></ol></section>`)
	if i := indice(d.Bloques(), l); i < 0 || d.Bloques()[i].Texto() != "Texto de la nota" || d.Bloques()[i].Marca != "1." {
		t.Errorf("el cursor no va a la nota")
	}
	// La segunda, desde dentro de las notas: el marcador va al final del cuerpo.
	d.InsertarNotaPie(d.Bloques(), 2, 0, "Otra")
	if got := d.HTML(); !strings.HasPrefix(got, `<p>hola<sup data-footnote-ref="1" id="fnref-1"><a href="#fn-1">1</a></sup>`+
		`<sup data-footnote-ref="2" id="fnref-2"><a href="#fn-2">2</a></sup></p>`) || !strings.Contains(got, `<li id="fn-2" value="2">Otra</li></ol>`) {
		t.Errorf("la segunda nota: %s", got)
	}
}

// Lo que pone el importador de .docx (services/docs, import_docx_formato.go): el espaciado, el
// interlineado y los tabuladores del párrafo se pintan; la presentación de las celdas y de las
// letras se deja sin pintar y no impide editar.
func TestLoImportadoDeUnDocx(t *testing.T) {
	fuente := `<p style="font-family:'Times New Roman','Liberation Serif',Tinos,serif;font-size:15pt;margin-top:8px;margin-bottom:0;line-height:18pt">a</p>` +
		`<table style="width:300px;margin-left:auto;margin-right:auto"><tbody><tr><td style="border:1px solid #000000;background-color:#CED7E7;padding:0 7.2px;width:100px">` +
		`<p style="margin-bottom:10.67px;line-height:1.3"><span style="letter-spacing:1pt;text-transform:uppercase">b</span></p></td></tr></tbody></table>` +
		`<p><sup data-footnote-ref="1" data-note-class="endnote" id="fnref-1"><a href="#fn-1">1</a></sup></p>`
	d := Parse(fuente)
	if !d.Editable() {
		t.Fatalf("no se edita: %v", d.NoEntendido())
	}
	for _, p := range []string{"style: border", "style: padding", "style: width", "style: letter-spacing", "style: text-transform", "style: margin-left"} {
		if !slices.Contains(d.Ignorado(), p) {
			t.Errorf("falta %q en lo ignorado: %v", p, d.Ignorado())
		}
	}
	b := d.Bloques()
	if b[0].Arriba != 8 || b[0].Abajo != 0 || b[0].Base.Familia != "Times New Roman" || b[0].Base.Interlineado < 1.19 || b[0].Base.Interlineado > 1.21 {
		t.Errorf("párrafo: arriba %v abajo %v familia %q interlineado %v", b[0].Arriba, b[0].Abajo, b[0].Base.Familia, b[0].Base.Interlineado)
	}
	if c := b[1]; c.Celda == nil || c.Abajo < 10.6 || c.Abajo > 10.7 {
		t.Errorf("celda: %+v", c)
	}
	if d := Parse(`<p style="white-space:pre-wrap;tab-size:47.2px">a	b  c</p>`); !d.Editable() || d.Bloques()[0].Texto() != "a\tb  c" || d.Bloques()[0].Base.Tabulador != 47.2 {
		t.Errorf("los tabuladores de un párrafo importado: editable %v, %q", d.Editable(), d.Bloques()[0].Texto())
	}
	for _, c := range []struct {
		css  string
		want float32
	}{
		{"line-height:0.01", 0.5},
		{"line-height:2pt", 0.5},
		{"line-height:8", 5},
	} {
		d := Parse(`<p style="` + c.css + `">a</p>`)
		if !d.Editable() || d.Bloques()[0].Base.Interlineado != c.want {
			t.Errorf("%s: editable %v (%v), interlineado %v (quería %v)", c.css, d.Editable(), d.NoEntendido(), d.Bloques()[0].Base.Interlineado, c.want)
		}
	}
	if d := Parse(`<p style="line-height:-1">a</p>`); d.Editable() {
		t.Error("un interlineado negativo no se entiende")
	}
}

// El marco de texto de un .docx importado (div[data-marco]: un framePr o un cuadro anclado) no
// impide editar: sus párrafos se editan en el flujo, y su `float`, su ancho y sus márgenes se dejan
// sin pintar. Antes el `float` y el atributo del div dejaban el documento en solo lectura.
func TestElMarcoDeUnDocxSeEdita(t *testing.T) {
	fuente := `<p>Antes</p><div data-marco="" style="float:left;width:189.33px;margin-left:-0.67px;margin-right:12px;margin-bottom:4px">` +
		`<p style="text-align:right">APTITUDES</p></div><p>Después</p>`
	d := Parse(fuente)
	if !d.Editable() {
		t.Fatalf("no se edita: %v", d.NoEntendido())
	}
	textos := make([]string, 0, len(d.Bloques()))
	for _, b := range d.Bloques() {
		textos = append(textos, b.Texto())
	}
	if strings.Join(textos, "|") != "Antes|APTITUDES|Después" {
		t.Errorf("los bloques son %q", textos)
	}
	if !slices.Contains(d.Ignorado(), "style: float") {
		t.Errorf("falta el float en lo ignorado: %v", d.Ignorado())
	}
	if got := d.HTML(); got != fuente {
		t.Errorf("leerlo lo cambia:\n%s", got)
	}
}

// La viñeta propia de una lista importada de Pages (`list-style-type:'- '`) se pinta y no
// impide editar.
func TestLaVinetaDeUnaListaImportada(t *testing.T) {
	d := Parse(`<ul style="list-style-type:'- '"><li>uno</li></ul><ul><li>dos</li></ul>`)
	if !d.Editable() {
		t.Fatalf("no se edita: %v", d.NoEntendido())
	}
	if b := d.Bloques(); b[0].Marca != "-" || b[1].Marca != "•" {
		t.Errorf("marcas %q y %q", b[0].Marca, b[1].Marca)
	}
}

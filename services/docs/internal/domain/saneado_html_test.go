// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// LO QUE EL EDITOR ESCRIBE NO SE TOCA, NI UN BYTE. Cada caso es una cadena de las que el
// editor guarda en el modelo (con el fichero que la escribe), más las tres trampas del
// contrato: el texto de las formas en <foreignObject>, el comentario del control de
// cambios al principio y la marca de agua en `style` con `url(data:image/…)`.
var htmlDelEditor = []struct{ nombre, html string }{
	{"párrafos y formato (exec.ts, styleWithCSS)", `<h1 style="text-align: center;">Informe</h1><p>Texto <strong>fuerte</strong>, <em>énfasis</em>, <u>subrayado</u>, <s>tachado</s>, x<sup>2</sup>, <span style="color: rgb(26, 53, 65); background-color: rgb(252, 241, 217);">color</span> y <font face="Georgia" size="4" color="#1a3541">fuente</font>.</p><ul><li>uno</li><li>dos</li></ul><ol start="3"><li>tres</li></ol><blockquote>cita</blockquote><hr>`},
	{"control de cambios (trackchanges.ts)", `<!--sytue-track--><p>Antes <ins data-track="ins" data-author="Ada" data-author-id="u1" data-ts="1759700000000">nuevo</ins><del data-track="del" data-author="Ada" data-author-id="u1" data-ts="1759700000001">viejo</del></p>`},
	{"forma con texto (docblocks.ts)", `<figure data-shape="rect" data-shape-type="rect" contenteditable="false" style="width:240px"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 120" width="240" height="120"><rect x="1" y="1" width="238" height="118" rx="6" fill="#d9e9f8" stroke="#1a3541" stroke-width="2"/><foreignObject x="0" y="0" width="240" height="120"><div xmlns="http://www.w3.org/1999/xhtml" style="box-sizing:border-box;width:100%;height:100%;display:flex;align-items:center;justify-content:center;font-family:var(--font-sans,Inter,sans-serif);font-size:14px">Texto de la forma</div></foreignObject></svg></figure>`},
	{"SmartArt (smartart.ts)", `<figure data-smartart="process" data-items="[&quot;Uno&quot;,&quot;Dos&quot;]" data-layout="process" contenteditable="false"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 100"><g><path d="M0 0 L100 0 L100 60 Z" fill="currentColor" fill-opacity="0.12"/><foreignObject x="4" y="4" width="92" height="52"><div xmlns="http://www.w3.org/1999/xhtml" style="display:flex;gap:0.4em"><span aria-hidden="true" style="opacity:0.55;flex:0 0 auto">&#8226;</span><span style="min-width:0">Uno</span></div></foreignObject></g></svg></figure>`},
	{"marca de agua de imagen (pagedecor.ts)", `<div data-sytue-watermark="image" data-kind="image" data-opacity="0.2" style="background-image:url(data:image/png;base64,iVBORw0KGgo=);background-repeat:no-repeat;opacity:0.2"><svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 10 10"><image href="data:image/png;base64,iVBORw0KGgo=" x="0" y="0" width="10" height="10" preserveAspectRatio="xMidYMid meet"/></svg></div>`},
	{"campo de formulario (formfields.ts)", `<p>Nombre: <span data-formfield="text" data-name="nombre" data-placeholder="Tu nombre" contenteditable="false" class="ff">&nbsp;</span> <span data-formfield="checkbox" data-checked="true" contenteditable="false">&#9745;</span></p>`},
	{"tabla, imagen y enlaces", `<table style="width: 100%;"><tbody><tr><th colspan="2" style="width: 50%;">A &amp; B</th></tr><tr><td rowspan="1">1 &lt; 2</td><td><img src="data:image/png;base64,iVBORw0KGgo=" alt="logo" width="40" height="40" data-sytue-image="1" data-wrap="inline" loading="lazy"><a href="https://sytue.eu/a?b=1&amp;c=2" target="_blank" rel="noopener">web</a> <a href="mailto:ada@example.eu">correo</a> <a href="#nota-1">ancla</a> <a href="/ruta">ruta</a></td></tr></tbody></table>`},
	{"notas, índice y salto de página", `<p>Texto<sup data-footnote-ref="1"><a href="#fn-1">1</a></sup></p><div data-page-break="true"></div><nav data-toc="true"><p data-level="1"><a href="#h-1">Uno</a></p></nav><section data-footnotes="true"><p id="fn-1">Nota</p></section>`},
	{"medio con subtítulos (slides/subtitulos.ts)", `<video src="data:video/mp4;base64,AAAA" poster="data:image/png;base64,iVBORw0KGgo=" controls><track kind="subtitles" srclang="es" src="data:text/vtt;base64,V0VCVlRU"></video><audio src="https://media.example.eu/voz.mp3" controls></audio>`},
	{"escrito a mano, con otra forma de lo mismo", "<P STYLE='text-align:right'>Madrid,<BR>hoy</P>\n<p class=nota>sin comillas</p>"},
}

func TestSanearHTMLNoTocaLoQueElEditorEscribe(t *testing.T) {
	for _, c := range htmlDelEditor {
		if got := SanearHTML(c.html); got != c.html {
			t.Errorf("%s: el saneador cambió contenido legítimo\n   era: %s\n queda: %s", c.nombre, c.html, got)
		}
	}
}

// Lo que ejecuta código, o lo trae de fuera, no se guarda. Cada caso dice qué queda: el
// contenido que no era el problema se conserva.
var htmlQueNoEntra = []struct{ nombre, entra, queda string }{
	{"manejador de evento", `<p onclick="x()">a</p>`, `<p>a</p>`},
	{"manejador en una imagen", `<img src="/a.png" onerror="x()" alt="a">`, `<img src="/a.png" alt="a">`},
	{"manejador en mayúsculas y sin comillas", `<p ONCLICK=x()>a</p>`, `<p>a</p>`},
	{"manejador pegado al atributo anterior", `<img src="/a.png"onerror="x()">`, `<img src="/a.png">`},
	{"manejador en SVG", `<svg onload="x()"><circle r="1"/></svg>`, `<svg><circle r="1"/></svg>`},
	{"enlace con otro esquema", `<a href="javascript:x()">a</a>`, `<a>a</a>`},
	{"el mismo, con espacios y mayúsculas", `<a href="  JaVaScRiPt:x()">a</a>`, `<a>a</a>`},
	{"el mismo, con entidades", `<a href="&#106;avascript:x()">a</a>`, `<a>a</a>`},
	{"el mismo, en xlink:href", `<svg><a xlink:href="javascript:x()"><text>a</text></a></svg>`, `<svg><a><text>a</text></a></svg>`},
	{"data: que no es imagen en un enlace", `<a href="data:text/html,x">a</a>`, `<a>a</a>`},
	{"data: que no es imagen en un recurso", `<img src="data:text/html,x">`, `<img>`},
	{"data: en un <use>", `<svg><use href="data:image/svg+xml,x"/></svg>`, `<svg><use/></svg>`},
	{"srcdoc", `<img srcdoc="x" src="/a.png">`, `<img src="/a.png">`},
	{"script, con su contenido", `<script>x()</script><p>a</p>`, `<p>a</p>`},
	{"script autocerrado", `<script/>x()</script><p>a</p>`, `<p>a</p>`},
	{"script dentro de un SVG", `<svg><script>x()</script><circle r="1"/></svg>`, `<svg><circle r="1"/></svg>`},
	{"style, con su contenido", `<style>p{color:red}</style><p>a</p>`, `<p>a</p>`},
	{"iframe", `<iframe src="https://example.org"></iframe>b`, `b`},
	{"object con contenido de reserva", `<object data="x"><p>dentro</p></object>b`, `b`},
	{"MathML entero", `<math><mi>x</mi><annotation-xml encoding="text/html"><p>y</p></annotation-xml></math>b`, `b`},
	{"animación que cambia un atributo", `<svg><a><animate attributeName="href" to="javascript:x()"/><text>t</text></a></svg>`, `<svg><a><text>t</text></a></svg>`},
	{"meta no se lleva el resto", `<meta http-equiv="refresh" content="0;url=https://example.org"><p>a</p>`, `<p>a</p>`},
	{"base y link no se llevan el resto", `<base href="https://example.org/"><link rel="stylesheet" href="x.css"><p>a</p>`, `<p>a</p>`},
	{"comentario", `<p>a</p><!-- oculto --><p>b</p>`, `<p>a</p><p>b</p>`},
	{"la marca del control de cambios solo vale al principio", `<p>a</p><!--sytue-track-->`, `<p>a</p>`},
	{"instrucción de proceso y doctype", `<!doctype html><?xml version="1.0"?><p>a</p>`, `<p>a</p>`},
	{"etiqueta desconocida: se desenvuelve", `<marquee><b>a</b></marquee>`, `<b>a</b>`},
	{"formulario: se desenvuelve", `<form action="https://example.org"><input name="q"><button formaction="javascript:x()">ir</button></form>`, `ir`},
	{"textarea: queda su texto, escapado", `<textarea><img src=x onerror=x()></textarea>`, `&lt;img src=x onerror=x()>`},
	{"title: se conserva con el contenido aplanado", `<svg><title><b>t</b></title></svg>`, `<svg><title>&lt;b>t&lt;/b></title></svg>`},
	{"style con url de otro esquema", `<p style="background:url(javascript:x())">a</p>`, `<p>a</p>`},
	{"style con expression", `<p style="width:EXPRESSION(x())">a</p>`, `<p>a</p>`},
	{"etiqueta sin cerrar al final", `<p>a</p><img src="/a.png" onerror="x()"`, `<p>a</p>`},
	{"un < suelto junto a una etiqueta quitada", `<<x>img src=x onerror=x()>`, `&lt;img src=x onerror=x()>`},
	{"lo que se conserva alrededor no cambia", `<p style="color: red;">a</p><p onclick="x()" class="b">c</p><p>d &amp; e</p>`, `<p style="color: red;">a</p><p class="b">c</p><p>d &amp; e</p>`},
}

func TestSanearHTMLQuitaLoQueEjecuta(t *testing.T) {
	for _, c := range htmlQueNoEntra {
		got := SanearHTML(c.entra)
		if got != c.queda {
			t.Errorf("%s:\n  entra: %s\n  queda: %s\n quería: %s", c.nombre, c.entra, got, c.queda)
		}
		// Sanear lo ya saneado no cambia nada: es lo que hace estable el guardado.
		if otra := SanearHTML(got); otra != got {
			t.Errorf("%s: no es estable: %s -> %s", c.nombre, got, otra)
		}
	}
}

// Una hoja lleva HTML en sus celdas y una presentación en el `html` de sus cajas; lo demás
// del modelo no se toca. Un modelo sin nada que quitar sale idéntico.
func TestSanearContenidoDeHojasYPresentaciones(t *testing.T) {
	// Como en `hojaDeMuestra` (editor/dev/superficies-fixtures.ts), con formato en celdas.
	hoja := `{"active":0,"sheets":[{"name":"Ventas","cells":{"A1":"<b>Mes</b>","B1":"<div style=\"text-align: right;\"><i>Unidades</i></div>","A2":"Ene","B2":"120","B3":"=SUM(B2:B2)","C3":"=IF(B2&lt;200,&quot;bajo&quot;,&quot;alto&quot;)","D3":"=A2&amp;B2"},"formats":{"B2":{"id":"thousands"}},"styles":{"A1":{"fill":"#d9e9f8"}},"freeze":{"cols":1,"rows":1},"charts":[{"id":"g1","type":"bar","range":"A1:B13","title":"Unidades <por> mes","x":700.5,"y":70}]},{"name":"Notas","cells":{}}]}`
	// Como en `presentacionDeMuestra`, con patrón y diseño.
	diapos := `{"slides":[{"id":"s1","boxes":[{"id":"t1","x":8,"y":28,"w":84,"h":20,"kind":"text","html":"<h1>Resultados</h1>"},{"id":"a3","x":56,"y":28,"w":36,"h":52,"kind":"shape","shape":"rect","html":"","fill":"#d9e9f8"}],"notes":"Empezar por el margen <no> por el volumen."}],"master":{"boxes":[{"id":"pie","x":5,"y":92,"w":30,"h":6,"kind":"text","html":"<p style=\"font-size: 12px;\">Cooperativa</p>"}],"layouts":[{"id":"portada","name":"Portada","boxes":[{"id":"t","x":10,"y":35,"w":80,"h":20,"kind":"text","html":"<h1>Título</h1>"}]}]}}`
	for clave, modelo := range map[string]string{claveHoja: hoja, claveDiapos: diapos, "doc:html": htmlDelEditor[1].html} {
		if got := SanearContenido(clave, modelo); got != modelo {
			t.Errorf("%s: el saneador cambió un modelo legítimo:\n%s", clave, got)
		}
	}

	leer := func(s string) map[string]any {
		t.Helper()
		var m map[string]any
		dec := json.NewDecoder(strings.NewReader(s))
		dec.UseNumber()
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("lo saneado no es JSON: %v\n%s", err, s)
		}
		return m
	}
	// Una celda con un manejador, y el mismo `<` escrito como escape de JSON en otra.
	sucia := strings.Replace(hoja, `"A2":"Ene"`, `"A2":"<img src=\"/a.png\" onerror=\"x()\">Ene","A9":"\u003cscript\u003ex()\u003c/script\u003eFeb","n":12345678901234567890`, 1)
	limpia := SanearContenido(claveHoja, sucia)
	if strings.Contains(limpia, "onerror") || strings.Contains(limpia, "script") {
		t.Fatalf("la hoja saneada conserva lo que ejecuta:\n%s", limpia)
	}
	celdas := leer(limpia)["sheets"].([]any)[0].(map[string]any)["cells"].(map[string]any)
	if celdas["A2"] != `<img src="/a.png">Ene` || celdas["A9"] != "Feb" || celdas["C3"] != `=IF(B2&lt;200,&quot;bajo&quot;,&quot;alto&quot;)` || celdas["A1"] != "<b>Mes</b>" {
		t.Errorf("celdas tras sanear: %v", celdas)
	}
	if !strings.Contains(limpia, `12345678901234567890`) || !strings.Contains(limpia, `700.5`) || !strings.Contains(limpia, `Unidades <por> mes`) {
		t.Errorf("lo que no es HTML de celda cambió:\n%s", limpia)
	}

	sucias := strings.NewReplacer(
		`"html":"<h1>Resultados</h1>"`, `"html":"<h1 onclick=\"x()\">Resultados</h1>"`,
		`"html":"<p style=\"font-size: 12px;\">Cooperativa</p>"`, `"html":"<p>Cooperativa</p><iframe src=\"https://example.org\"></iframe>"`,
		`"html":"<h1>Título</h1>"`, `"html":"<a href=\"javascript:x()\">Título</a>"`,
	).Replace(diapos)
	limpias := SanearContenido(claveDiapos, sucias)
	for _, resto := range []string{"onclick", "iframe", "javascript:"} {
		if strings.Contains(limpias, resto) {
			t.Errorf("la presentación saneada conserva %q:\n%s", resto, limpias)
		}
	}
	m := leer(limpias)
	if h := m["slides"].([]any)[0].(map[string]any)["boxes"].([]any)[0].(map[string]any)["html"]; h != "<h1>Resultados</h1>" {
		t.Errorf("caja de la diapositiva: %v", h)
	}
	if n := m["slides"].([]any)[0].(map[string]any)["notes"]; n != "Empezar por el margen <no> por el volumen." {
		t.Errorf("las notas, que no son HTML, cambiaron: %v", n)
	}
}

// El almacén sanea los `set` de los tres registros de contenido, y solo esos; el valor
// de un lote sin nada que quitar queda como llegó.
func TestSanearOps(t *testing.T) {
	set := func(clave, contenido string) Op {
		return Op{Kind: OpSet, Key: clave, Value: jsonString(contenido)}
	}
	legitimo := set("doc:html", htmlDelEditor[0].html)
	antes := bytes.Clone(legitimo.Value)
	ops := []Op{
		legitimo,
		set("doc:html", `<p onclick="x()">a</p>`),
		set(claveDiapos, `{"slides":[{"id":"s1","boxes":[{"id":"b1","kind":"text","html":"<p onclick=\"x()\">a</p>"}]}]}`),
		set("doc:title", `<p onclick="x()">no es contenido</p>`),
		{Kind: OpInsert, Dot: Dot{Actor: "a", Counter: 1}, Value: json.RawMessage(`"<"`)},
	}
	SanearOps(ops)
	if !bytes.Equal(ops[0].Value, antes) {
		t.Errorf("el valor legítimo cambió: %s", ops[0].Value)
	}
	var html1, diapos string
	_ = json.Unmarshal(ops[1].Value, &html1)
	_ = json.Unmarshal(ops[2].Value, &diapos)
	if html1 != `<p>a</p>` || strings.Contains(diapos, "onclick") || !strings.Contains(diapos, `<p>a</p>`) {
		t.Errorf("saneado: %q / %s", html1, diapos)
	}
	if !strings.Contains(string(ops[3].Value), "onclick") || string(ops[4].Value) != `"<"` {
		t.Errorf("se tocó lo que no es un registro de contenido: %s / %s", ops[3].Value, ops[4].Value)
	}
}

// Lo guardado antes de sanear al guardar puede no estarlo: el HTML que se exporta —el
// que se descarga y el que enseña un enlace público— sale saneado igualmente.
func TestElHTMLExportadoSaleSaneado(t *testing.T) {
	estado := func(clave, contenido string) *State {
		st := NewState()
		if _, err := st.Apply(Op{Kind: OpSet, Key: clave, Value: jsonString(contenido), Dot: Dot{Actor: "a", Counter: 1}, Lamport: 1}); err != nil {
			t.Fatal(err)
		}
		return st
	}
	for _, c := range []struct {
		kind  Kind
		st    *State
		queda string
	}{
		{KindDocument, estado("doc:html", `<p onclick="x()">hola</p><script>x()</script>`), `<p>hola</p>`},
		{KindPresentation, estado(claveDiapos, `{"slides":[{"id":"s1","boxes":[{"id":"b1","x":1,"y":1,"w":50,"h":50,"kind":"text","html":"<p onclick=\"x()\">hola</p>"}]}]}`), `<p>hola</p>`},
	} {
		pagina, _, _, err := Export(c.kind, "T", c.st, FormatHTML)
		if err != nil {
			t.Fatal(err)
		}
		if s := string(pagina); strings.Contains(s, "onclick") || strings.Contains(s, "x()") || !strings.Contains(s, c.queda) {
			t.Errorf("%s: el HTML exportado no sale saneado:\n%s", c.kind, s)
		}
	}
}

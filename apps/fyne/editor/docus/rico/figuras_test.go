// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"slices"
	"strings"
	"testing"
)

// formaDeLaWeb es una forma como la deja el navegador en `innerHTML` tras `shapeFigureHtml`.
const formaDeLaWeb = `<p>Antes</p><figure data-shape="" data-shape-type="ellipse" data-w="300" data-h="160" data-fill="#2563EB" ` +
	`data-stroke="#0A0A0A" data-text="Etiqueta" contenteditable="false" style="display:block;margin:1em auto;width:300px;max-width:100%">` +
	`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 300 160" width="100%" preserveAspectRatio="none" ` +
	`style="display:block;width:100%;height:auto" role="img" aria-label="Forma elipse — Etiqueta">` +
	`<ellipse cx="150" cy="80" rx="149" ry="79" fill="#2563EB" stroke="#0A0A0A" stroke-width="2"></ellipse>` +
	`<foreignObject x="0" y="0" width="300" height="160"><div xmlns="http://www.w3.org/1999/xhtml" style="display:flex">Etiqueta</div>` +
	`</foreignObject></svg></figure><p><br></p>`

// TestLaFiguraDeLaWebVuelveTalCual: una forma escrita en la web se entiende, vuelve byte a
// byte sin leer su SVG y es UN bloque que no es de texto, con su configuración. Se vio en
// ROJO sin saltarse los hijos de la figura al entender: el `<svg>` la dejaba en solo lectura.
func TestLaFiguraDeLaWebVuelveTalCual(t *testing.T) {
	d := Parse(formaDeLaWeb)
	if !d.Editable() || d.HTML() != formaDeLaWeb || !d.Guardable() {
		t.Fatalf("no se entiende (%v) o no vuelve igual:\n%s", d.NoEntendido(), d.HTML())
	}
	bs := d.Bloques()
	if len(bs) != 3 || bs[1].Tipo != Figura || bs[1].Tipo.EsTexto() || bs[1].Ilustracion.Forma == nil {
		t.Fatalf("bloques: %+v", bs)
	}
	if got := *bs[1].Ilustracion.Forma; got != (Forma{Tipo: "ellipse", W: 300, H: 160, Relleno: "#2563EB", Trazo: "#0A0A0A", Texto: "Etiqueta"}) {
		t.Errorf("la forma leída: %+v", got)
	}
	if Parse(`<ul><li>` + strings.TrimPrefix(strings.TrimSuffix(formaDeLaWeb, `<p><br></p>`), `<p>Antes</p>`) + `</li></ul>`).Editable() {
		t.Error("una figura dentro de una lista no se entiende")
	}
	// Borrar hacia atrás desde el párrafo de detrás se la lleva, como una regla.
	if _, _, ok := d.Fundir(bs, 2); !ok || strings.Contains(d.HTML(), "<figure") {
		t.Errorf("Retroceso tras la figura: %s", d.HTML())
	}
}

// TestLasFormasSonLasDeLaWeb son los vectores de `docs-blocks.test.ts`: el saneado, el
// HTML con sus `data-*` y el SVG, sin capa de texto en la línea y la flecha, y escapado.
func TestLasFormasSonLasDeLaWeb(t *testing.T) {
	if f := SanearForma("inventada", "10", "99999", "#123456", "transparent", ""); f.Tipo != "rect" || f.W != 24 || f.H != 1600 ||
		f.Relleno != "transparent" || f.Trazo != "#0A0A0A" {
		t.Errorf("saneado: %+v", f)
	}
	if f := SanearForma("star", "abc", "240", "#2563EB", "#DC2626", "x"); f.W != AnchoForma || f.H != 240 || f.Relleno != "#2563EB" || f.Trazo != "#DC2626" {
		t.Errorf("saneado: %+v", f)
	}
	f := Forma{Tipo: "ellipse", W: 300, H: 160, Relleno: "#2563EB", Trazo: "#0A0A0A", Texto: "Etiqueta"}
	h := f.HTML("Forma elipse")
	for _, quiero := range []string{`data-shape-type="ellipse"`, `data-w="300"`, `data-h="160"`, `data-fill="#2563EB"`, `contenteditable="false"`, "<ellipse", "Etiqueta", "foreignObject"} {
		if !strings.Contains(h, quiero) {
			t.Errorf("el HTML no lleva %q: %s", quiero, h)
		}
	}
	for tipo, quiero := range map[string]string{"star": "<polygon", "triangle": "<polygon", "cylinder": "<path", "rect": "<rect", "arrow": "<line"} {
		f.Tipo = tipo
		if s := f.HTML(""); !strings.Contains(s, quiero) || (tipo == "arrow") == strings.Contains(s, "foreignObject") {
			t.Errorf("%s: %s", tipo, s)
		}
	}
	f.Tipo, f.Texto = "rect", `<script>alert(1)</script> & co`
	if s := f.HTML(""); strings.Contains(s, "<script>") || !strings.Contains(s, "&lt;script&gt;") || !strings.Contains(s, "&amp;") {
		t.Errorf("sin escapar: %s", s)
	}
	f.Relleno = "transparent"
	if s := string(f.SVG()); !strings.Contains(s, `fill="none"`) || strings.Contains(s, "foreignObject") {
		t.Errorf("el SVG para pintar: %s", s)
	}
}

// TestElGraficoEsElDeLaWeb: el saneado de la rejilla, que va y vuelve por `data-chart-grid`,
// y el gráfico de arranque, con el eje de 0 a 15 y las cuatro categorías.
func TestElGraficoEsElDeLaWeb(t *testing.T) {
	if g := SanearGrafico("scatter", "no es json"); g.Tipo != "bar" || !slices.EqualFunc(g.Rejilla, RejillaInicial(), slices.Equal) {
		t.Errorf("saneado: %+v", g)
	}
	if g := SanearGrafico("pie", `[["a",1],["b"],5]`); g.Tipo != "pie" || !slices.EqualFunc(g.Rejilla, [][]string{{"a", "1"}, {"b", ""}, {"", ""}}, slices.Equal) {
		t.Errorf("rectangularizada: %+v", g.Rejilla)
	}
	g := Grafico{Tipo: "bar", Rejilla: RejillaInicial()}
	g.Rejilla[1][0] = `Ene <&>`
	d := Parse(`<p>a</p>`)
	if _, ok := d.InsertarHTML(d.Bloques(), 0, 1, g.HTML()+`<p><br></p>`); !ok || !d.Guardable() {
		t.Fatalf("no se inserta o no se guarda: %v", d.NoEntendido())
	}
	otra := Parse(d.HTML())
	bs := otra.Bloques()
	if !otra.Editable() || otra.HTML() != d.HTML() || len(bs) != 3 || bs[1].Ilustracion == nil || bs[1].Ilustracion.Grafico == nil {
		t.Fatalf("no vuelve: %s", d.HTML())
	}
	if got := *bs[1].Ilustracion.Grafico; got.Tipo != "bar" || !slices.EqualFunc(got.Rejilla, g.Rejilla, slices.Equal) {
		t.Errorf("releído: %+v", got)
	}
	web := g.HTML()
	for _, quiero := range []string{`>15</text>`, `>Ene &lt;&amp;&gt;</text>`, `aria-label="Gráfico de barras"`, "<title>"} {
		if !strings.Contains(web, quiero) {
			t.Errorf("el SVG horneado no lleva %q", quiero)
		}
	}
	svg, rotulos, vacio := g.SVG()
	textos := make([]string, 0, len(rotulos))
	for _, r := range rotulos {
		textos = append(textos, r.Texto)
	}
	if vacio || strings.Contains(string(svg), "<text") || !slices.Equal(textos, []string{"0", "5", "10", "15", "Ene <&>", "Feb", "Mar", "Abr"}) {
		t.Errorf("para pintar: vacío %v, rótulos %v", vacio, textos)
	}
	if _, r, vacio := (Grafico{Tipo: "pie", Rejilla: [][]string{{"", "S"}, {"a", "0"}}}).SVG(); !vacio || len(r) != 1 || r[0].Texto != AvisoSinDatos {
		t.Errorf("una tarta sin positivos es el aviso: %v %v", vacio, r)
	}
}

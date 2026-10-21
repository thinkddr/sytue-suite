// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"git.sytue.eu/sytue/one/apps/fyne/editor/figuras"
)

// Las formas y los gráficos del documento (`editor/docblocks.ts`). Como en la web, el
// HTML del folio ES el modelo: cada uno es un `<figure contenteditable="false">` con su
// configuración en `data-*` —la fuente de verdad— y el SVG HORNEADO dentro, que viaja al
// export. Aquí el SVG de dentro no se lee: la figura se pinta desde sus `data-*` y vuelve
// al árbol tal cual. Al insertarla o cambiarla se hornea el mismo SVG que la web.

// Ilustracion es lo que lleva un bloque [Figura]: una forma, un gráfico o un diagrama,
// uno de los tres.
type Ilustracion struct {
	Forma   *Forma
	Grafico *Grafico
	Smart   *SmartArt
	nodo    *html.Node
}

// Clase es la de la figura, la de su `data-*`: "shape", "chart" o "smartart".
func (il *Ilustracion) Clase() string {
	switch {
	case il.Grafico != nil:
		return "chart"
	case il.Smart != nil:
		return "smartart"
	}
	return "shape"
}

// ClaseDe es la clase del HTML de una figura, el que dan los `HTML` de este paquete.
func ClaseDe(fragmento string) string {
	c, _, _ := strings.Cut(strings.TrimPrefix(fragmento, "<figure data-"), " ")
	return c
}

// Forma es `DocShapeConfig`.
type Forma struct {
	Tipo           string
	W, H           int
	Relleno, Trazo string // Relleno puede ser 'transparent'
	Texto          string
}

// Grafico es el tipo y la rejilla de datos de un gráfico: 1.ª fila la cabecera y 1.ª
// columna los rótulos.
type Grafico struct {
	Tipo    string
	Rejilla [][]string
}

// Medidas de una forma, en px de maqueta.
const (
	AnchoForma = 320 // DEFAULT_SHAPE_W
	AltoForma  = 180 // DEFAULT_SHAPE_H
	MinForma   = 24
	MaxForma   = 1600
)

// Medidas del gráfico: el viewBox; la figura se pinta al 100 % de su ancho.
const (
	AnchoGrafico = 480
	AltoGrafico  = 300
	MaxFilasGraf = 60
	MaxColsGraf  = 12
)

// TiposForma son los ids de DOC_SHAPES, en su orden.
var TiposForma = []string{"rect", "ellipse", "line", "arrow", "triangle", "diamond", "pentagon", "hexagon", "star", "cylinder", "callout"}

// TiposGrafico son los de DOC_CHART_TYPES, con su rótulo, que la web no traduce.
var TiposGrafico = []struct{ ID, Rotulo string }{{"bar", "Barras"}, {"line", "Líneas"}, {"pie", "Circular"}}

// SanearForma es `sanitizeShapeConfig` sobre los valores crudos de sus `data-*`.
func SanearForma(tipo, w, h, relleno, trazo, texto string) Forma {
	f := Forma{Tipo: "rect", W: sanearMedida(w, AnchoForma), H: sanearMedida(h, AltoForma), Relleno: "transparent", Trazo: figuras.Paleta[0], Texto: texto}
	if slices.Contains(TiposForma, tipo) {
		f.Tipo = tipo
	}
	if relleno == "transparent" || slices.Contains(figuras.Paleta, relleno) {
		f.Relleno = relleno
	}
	if slices.Contains(figuras.Paleta, trazo) {
		f.Trazo = trazo
	}
	return f
}

// sanearMedida es `sanitizeSize`: entero acotado; lo que no es un número, el de por defecto.
func sanearMedida(crudo string, porDefecto int) int {
	n, err := strconv.ParseFloat(strings.TrimSpace(crudo), 64)
	if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
		return porDefecto
	}
	return int(figuras.Acotar(math.Round(n), MinForma, MaxForma))
}

// ConTexto dice si la forma lleva la capa de texto: la línea y la flecha, no.
func (f Forma) ConTexto() bool { return f.Tipo != "line" && f.Tipo != "arrow" }

// RejillaInicial es DEFAULT_CHART_GRID, con los rótulos que la web no traduce.
func RejillaInicial() [][]string {
	return [][]string{{"", "Serie 1"}, {"Ene", "10"}, {"Feb", "15"}, {"Mar", "7"}, {"Abr", "12"}}
}

// SanearGrafico es `sanitizeChartType` + `parseChartGrid`: lo que no se lee cae a las
// barras y a la rejilla de arranque; la rejilla se rectangulariza y se acota.
func SanearGrafico(tipo, rejilla string) Grafico {
	g := Grafico{Tipo: "bar", Rejilla: RejillaInicial()}
	if slices.ContainsFunc(TiposGrafico, func(t struct{ ID, Rotulo string }) bool { return t.ID == tipo }) {
		g.Tipo = tipo
	}
	var datos []any
	if err := json.Unmarshal([]byte(rejilla), &datos); err != nil || len(datos) == 0 {
		return g
	}
	filas := make([][]string, 0, min(len(datos), MaxFilasGraf))
	cols := 1
	for _, d := range datos[:min(len(datos), MaxFilasGraf)] {
		celdas, ok := d.([]any)
		if !ok {
			filas = append(filas, []string{""})
			continue
		}
		fila := make([]string, 0, min(len(celdas), MaxColsGraf))
		for _, c := range celdas[:min(len(celdas), MaxColsGraf)] {
			fila = append(fila, cadenaJS(c))
		}
		filas = append(filas, fila)
		cols = max(cols, len(fila))
	}
	for i, f := range filas {
		for len(f) < cols {
			f = append(f, "")
		}
		filas[i] = f
	}
	g.Rejilla = filas
	return g
}

// cadenaJS es `String(c ?? ”)` para lo que trae un JSON.
func cadenaJS(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	return "[object Object]" // ponytail: un objeto o una lista dentro de una celda, como `String({})`
}

// esFigura dice si n es la figura de una forma o de un gráfico.
func esFigura(n *html.Node) bool {
	return n.Type == html.ElementNode && n.DataAtom == atom.Figure && n.Namespace == "" &&
		(tieneAtributo(n, "data-shape") || tieneAtributo(n, "data-chart") || tieneAtributo(n, "data-smartart"))
}

// entenderFigura: la figura va suelta en el folio o en unas columnas, que es donde la deja
// `insertHTML`; en una lista o una celda se pintaría de otra forma y no se entiende.
func entenderFigura(n *html.Node, motivos map[string]bool) {
	if p := n.Parent; p == nil || p.DataAtom != atom.Body && !esColumnas(p) {
		motivos["<figure> dentro de otro"] = true
	}
}

func figuraDe(n *html.Node) *Ilustracion {
	a := func(k string) string { return atributo(n, k) }
	if tieneAtributo(n, "data-shape") {
		f := SanearForma(a("data-shape-type"), a("data-w"), a("data-h"), a("data-fill"), a("data-stroke"), a("data-text"))
		return &Ilustracion{Forma: &f, nodo: n}
	}
	if tieneAtributo(n, "data-smartart") {
		sa := SmartArt{Disposicion: SanearDisposicion(a("data-layout")), Elementos: LeerElementos(a("data-items"))}
		return &Ilustracion{Smart: &sa, nodo: n}
	}
	g := SanearGrafico(a("data-chart-type"), a("data-chart-grid"))
	return &Ilustracion{Grafico: &g, nodo: n}
}

// PonerFigura cambia la figura por la del HTML dado (el de [Forma.HTML] o [Grafico.HTML]):
// el `rebake` de la web, que reescribe sus `data-*` y el SVG.
func (d *Documento) PonerFigura(f *Ilustracion, fragmento string) bool {
	if f == nil || f.nodo == nil || f.nodo.Parent == nil {
		return false
	}
	nodos, err := html.ParseFragment(strings.NewReader(fragmento), cuerpo())
	if err != nil || len(nodos) != 1 || !esFigura(nodos[0]) {
		return false
	}
	f.nodo.Parent.InsertBefore(nodos[0], f.nodo)
	f.nodo.Parent.RemoveChild(f.nodo)
	f.nodo = nodos[0]
	return true
}

var escapar = html.EscapeString

// HTML es `shapeFigureHtml`: la figura con su configuración y el SVG horneado. `aria` es
// `shape.aria` ya traducido, con el nombre de la forma.
func (f Forma) HTML(aria string) string {
	return fmt.Sprintf(`<figure data-shape data-shape-type="%s" data-w="%d" data-h="%d" data-fill="%s" data-stroke="%s" data-text="%s" `+
		`contenteditable="false" style="display:block;margin:1em auto;width:%dpx;max-width:100%%">%s</figure>`,
		escapar(f.Tipo), f.W, f.H, escapar(f.Relleno), escapar(f.Trazo), escapar(f.Texto), f.W, f.svgWeb(aria))
}

// svgWeb es `shapeSvg`.
func (f Forma) svgWeb(aria string) string {
	w, h := figuras.N2(float64(f.W)), figuras.N2(float64(f.H))
	if t := strings.TrimSpace(f.Texto); t != "" {
		aria += " — " + t
	}
	texto := ""
	if f.ConTexto() && strings.TrimSpace(f.Texto) != "" {
		texto = fmt.Sprintf(`<foreignObject x="0" y="0" width="%s" height="%s"><div xmlns="http://www.w3.org/1999/xhtml" style="`+
			`box-sizing:border-box;width:100%%;height:100%%;display:flex;align-items:center;justify-content:center;text-align:center;`+
			`padding:6px 10px;font-family:var(--font-sans,Inter,sans-serif);font-size:%spx;font-weight:500;line-height:1.2;`+
			`color:%s;overflow:hidden;word-break:break-word">%s</div></foreignObject>`,
			w, h, figuras.N2(f.LetraTexto()), figuras.Paleta[0], escapar(f.Texto))
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" width="100%%" preserveAspectRatio="none" `+
		`style="display:block;width:100%%;height:auto" role="img" aria-label="%s">%s%s</svg>`, w, h, escapar(aria), f.geometria(true), texto)
}

// LetraTexto es el `font-size` del texto de la forma.
func (f Forma) LetraTexto() float64 {
	return figuras.Acotar(min(float64(f.H)*0.16, float64(f.W)*0.1), 9, 28)
}

// SVG es la forma sola, sin el texto, para pintarla: `oksvg` no sabe de foreignObject.
func (f Forma) SVG() []byte {
	w, h := figuras.N2(float64(f.W)), figuras.N2(float64(f.H))
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` + w + " " + h + `" width="` + w + `" height="` + h + `">` +
		f.geometria(false) + `</svg>`)
}

// geometria es `shapeGeometrySvg`. Para pintarla, 'transparent' va como `none`: es lo
// mismo para el navegador y lo único que entiende `oksvg`.
func (f Forma) geometria(web bool) string {
	w, h := float64(f.W), float64(f.H)
	fill, stroke := f.Relleno, f.Trazo
	if fill == "transparent" && !web {
		fill = "none"
	}
	n2 := figuras.N2
	var b strings.Builder
	if pts := figuras.Vertices(f.Tipo, w, h); len(pts) > 0 {
		fmt.Fprintf(&b, `<polygon points="%s" fill="%s" stroke="%s" stroke-width="2" stroke-linejoin="round"/>`, figuras.PuntosSVG(pts), fill, stroke)
		return b.String()
	}
	switch f.Tipo {
	case "cylinder":
		d := figuras.MedidasCilindro(w, h)
		fmt.Fprintf(&b, `<path d="%s" fill="%s" stroke="%s" stroke-width="2" stroke-linejoin="round"/>`+
			`<path d="%s" fill="none" stroke="%s" stroke-width="2" stroke-linecap="round"/>`, d.Cuerpo(), fill, stroke, d.Costura(), stroke)
	case "ellipse":
		fmt.Fprintf(&b, `<ellipse cx="%s" cy="%s" rx="%s" ry="%s" fill="%s" stroke="%s" stroke-width="2"/>`,
			n2(w/2), n2(h/2), n2(max(1, w/2-1)), n2(max(1, h/2-1)), fill, stroke)
	case "line", "arrow":
		y := n2(h / 2)
		fmt.Fprintf(&b, `<line x1="2" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="3" stroke-linecap="round"/>`, y, n2(w-2), y, stroke)
		if f.Tipo == "arrow" {
			fmt.Fprintf(&b, `<path d="M%s %s L%s %s L%s %s" fill="none" stroke="%s" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/>`,
				n2(w-14), n2(h/2-7), n2(w-2), y, n2(w-14), n2(h/2+7), stroke)
		}
	default:
		fmt.Fprintf(&b, `<rect x="1" y="1" width="%s" height="%s" rx="%s" fill="%s" stroke="%s" stroke-width="2"/>`,
			n2(max(1, w-2)), n2(max(1, h-2)), n2(figuras.Acotar(min(w, h)*0.04, 0, 8)), fill, stroke)
	}
	return b.String()
}

// HTML es `chartFigureHtml`.
func (g Grafico) HTML() string {
	var rejilla bytes.Buffer
	enc := json.NewEncoder(&rejilla)
	enc.SetEscapeHTML(false)  // JSON.stringify no escapa `<`, `>` ni `&`
	_ = enc.Encode(g.Rejilla) //nolint:errcheck // una [][]string siempre se codifica
	svg, _, _ := g.componer(true)
	return fmt.Sprintf(`<figure data-chart data-chart-type="%s" data-chart-grid="%s" contenteditable="false" `+
		`style="display:block;margin:1em auto;width:480px;max-width:100%%">%s</figure>`,
		escapar(g.Tipo), escapar(strings.TrimSuffix(rejilla.String(), "\n")), svg)
}

// SVG es el gráfico para pintarlo: las marcas en SVG y los rótulos aparte, que `oksvg`
// no pinta texto. Vacío dice que no hay datos que pintar: se enseña el aviso.
func (g Grafico) SVG() ([]byte, []figuras.Rotulo, bool) {
	svg, rotulos, vacio := g.componer(false)
	return []byte(svg), rotulos, vacio
}

// AvisoSinDatos es el texto del gráfico vacío, que la web no traduce.
const AvisoSinDatos = "Sin datos numéricos: edita los datos del gráfico"

// componer es `chartSvg`. Con `web` escribe el SVG que se hornea, con sus `<text>` y
// `<title>`; sin él, el que se pinta, con los rótulos aparte.
func (g Grafico) componer(web bool) (string, []figuras.Rotulo, bool) {
	rotulos, series := figuras.Datos(g.Rejilla)
	var tarta []float64
	if g.Tipo == "pie" && len(series) > 0 {
		tarta = series[0].Valores
	}
	vacio := len(rotulos) == 0 || len(series) == 0 || g.Tipo == "pie" && !slices.ContainsFunc(tarta, func(v float64) bool { return v > 0 })
	var b strings.Builder
	if web {
		etiqueta := map[string]string{"bar": "barras", "line": "líneas", "pie": "sectores"}[g.Tipo]
		fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="100%%" style="display:block;width:100%%;height:auto" `+
			`role="img" aria-label="Gráfico de %s">`, AnchoGrafico, AltoGrafico, etiqueta)
	} else {
		fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`, AnchoGrafico, AltoGrafico, AnchoGrafico, AltoGrafico)
	}
	var fuera []figuras.Rotulo
	switch {
	case vacio && web:
		fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" dominant-baseline="middle" fill="%s" font-family="var(--font-sans,Inter,sans-serif)" `+
			`font-size="14">%s</text>`, AnchoGrafico/2, AltoGrafico/2, figuras.Apagado, AvisoSinDatos)
	case vacio: // `dominant-baseline="middle"`: la línea base, a un tercio de la letra bajo el centro
		fuera = []figuras.Rotulo{{Texto: AvisoSinDatos, X: AnchoGrafico / 2, Y: AltoGrafico/2 + 14*0.35, Tam: 14, Ancla: figuras.AnclaMedio, Color: figuras.Apagado}}
	case g.Tipo == "pie":
		tartaSVG(&b, rotulos, tarta, web)
	default:
		fuera = cartesianoSVG(&b, g.Tipo, rotulos, series, web)
	}
	b.WriteString(`</svg>`)
	return b.String(), fuera, vacio
}

// cartesianoSVG es `cartesianBody`: barras o líneas sobre un eje Y «bonito».
func cartesianoSVG(b *strings.Builder, tipo string, rotulos []string, series []figuras.Serie, web bool) []figuras.Rotulo {
	n2 := figuras.N2
	series = series[:min(len(series), len(figuras.Series))]
	const padL, padR, padT, padB, letra = 44, 10, 12, 26, 12
	plotW, plotH := float64(AnchoGrafico-padL-padR), float64(AltoGrafico-padT-padB)
	var todos []float64
	for _, s := range series {
		todos = append(todos, s.Valores...)
	}
	var escala figuras.Escala
	if tipo == "bar" {
		escala = figuras.Marcas(min(0, slices.Min(todos)), max(0, slices.Max(todos)))
	} else {
		escala = figuras.Marcas(slices.Min(todos), slices.Max(todos))
	}
	rango := escala.Max - escala.Min
	if rango == 0 {
		rango = 1
	}
	yDe := func(v float64) float64 { return padT + (escala.Max-v)/rango*plotH }
	n := max(1, len(rotulos))
	hueco := plotW / float64(n)
	paso := int(math.Ceil(float64(n) / max(1, math.Floor(plotW/(letra*4)))))
	var fuera []figuras.Rotulo
	rotulo := func(r figuras.Rotulo, dy string) {
		if !web {
			fuera = append(fuera, r)
			return
		}
		ancla := map[figuras.Ancla]string{figuras.AnclaFin: "end", figuras.AnclaMedio: "middle"}[r.Ancla]
		fmt.Fprintf(b, `<text x="%s" y="%s"%s text-anchor="%s" font-size="%d" fill="%s">%s</text>`, n2(r.X), n2(r.Y), dy, ancla, letra, r.Color, escapar(r.Texto))
	}
	for _, m := range escala.Marcas {
		y := n2(yDe(m))
		fmt.Fprintf(b, `<line x1="%d" x2="%d" y1="%s" y2="%s" stroke="%s" stroke-width="1" shape-rendering="crispEdges"/>`, padL, AnchoGrafico-padR, y, y, figuras.Reticula)
		r := figuras.Rotulo{Texto: figuras.Numero(m), X: padL - 4, Y: math.Round(yDe(m)*100) / 100, Tam: letra, Ancla: figuras.AnclaFin, Color: figuras.Apagado}
		if web {
			rotulo(r, ` dy="0.32em"`)
		} else {
			r.Y += 0.32 * letra
			rotulo(r, "")
		}
	}
	if tipo == "bar" {
		barrasSVG(b, rotulos, series, hueco, yDe, web)
	} else {
		lineasSVG(b, series, hueco, yDe)
	}
	base := padT + plotH
	if tipo == "bar" {
		base = yDe(0)
	}
	fmt.Fprintf(b, `<line x1="%d" x2="%d" y1="%s" y2="%s" stroke="%s" stroke-width="1" shape-rendering="crispEdges"/>`, padL, AnchoGrafico-padR, n2(base), n2(base), figuras.Eje)
	for i, r := range rotulos {
		if i%paso != 0 || r == "" {
			continue
		}
		if len([]rune(r)) > 9 {
			r = string([]rune(r)[:8]) + "…"
		}
		rotulo(figuras.Rotulo{
			Texto: r, X: math.Round((padL+hueco*(float64(i)+0.5))*100) / 100, Y: math.Round((AltoGrafico-padB/2+letra*0.3)*100) / 100,
			Tam: letra, Ancla: figuras.AnclaMedio, Color: figuras.Apagado,
		}, "")
	}
	return fuera
}

// barrasSVG son las barras de `cartesianBody`: un grupo por categoría, del 72 % del hueco.
func barrasSVG(b *strings.Builder, rotulos []string, series []figuras.Serie, hueco float64, yDe func(float64) float64, web bool) {
	const padL = 44
	n2 := figuras.N2
	grupo := hueco * 0.72
	bw := grupo / float64(max(1, len(series)))
	y0 := yDe(0)
	for k, s := range series {
		for i, v := range s.Valores {
			yv := yDe(v)
			titulo := ""
			if web {
				cat := ""
				if i < len(rotulos) {
					cat = rotulos[i]
				}
				titulo = "<title>" + escapar(cat+" — "+s.Nombre+": "+figuras.Numero(v)) + "</title>"
			}
			fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="%s" fill="%s" shape-rendering="crispEdges">%s</rect>`,
				n2(padL+float64(i)*hueco+(hueco-grupo)/2+float64(k)*bw+1), n2(min(yv, y0)), n2(max(1, bw-2)), n2(math.Abs(yv-y0)), figuras.Series[k], titulo)
		}
	}
}

// lineasSVG son las líneas de `cartesianBody`: un trazo por serie, por el centro de cada hueco.
func lineasSVG(b *strings.Builder, series []figuras.Serie, hueco float64, yDe func(float64) float64) {
	const padL = 44
	n2 := figuras.N2
	for k, s := range series {
		var d strings.Builder
		for i, v := range s.Valores {
			orden := "L"
			if i == 0 {
				orden = "M"
			}
			d.WriteString(orden + n2(padL+hueco*(float64(i)+0.5)) + " " + n2(yDe(v)))
		}
		fmt.Fprintf(b, `<path d="%s" fill="none" stroke="%s" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>`, d.String(), figuras.Series[k])
	}
}

// tartaSVG es `pieBody`: sectores de la primera serie, los valores > 0.
func tartaSVG(b *strings.Builder, rotulos []string, valores []float64, web bool) {
	n2 := figuras.N2
	type parte struct {
		nombre, color string
		valor         float64
	}
	var partes []parte
	total := 0.0
	for i, nombre := range rotulos {
		v := 0.0
		if i < len(valores) {
			v = max(0, valores[i])
		}
		if v > 0 {
			partes = append(partes, parte{nombre: nombre, valor: v, color: figuras.Series[i%len(figuras.Series)]})
			total += v
		}
	}
	if total == 0 {
		total = 1
	}
	const cx, cy = AnchoGrafico / 2, AltoGrafico / 2
	r := max(1, min(AnchoGrafico, AltoGrafico)/2-12.0)
	a := -math.Pi / 2
	for _, p := range partes {
		frac := p.valor / total
		a1 := a + frac*2*math.Pi
		titulo := ""
		if web {
			titulo = "<title>" + escapar(p.nombre+": "+figuras.Numero(p.valor)+" ("+figuras.Numero(frac*100)+" %)") + "</title>"
		}
		if frac > 0.9995 {
			fmt.Fprintf(b, `<circle cx="%s" cy="%s" r="%s" fill="%s" stroke="#FFFFFF" stroke-width="1.5">%s</circle>`, n2(cx), n2(cy), n2(r), p.color, titulo)
		} else {
			grande := 0
			if frac > 0.5 {
				grande = 1
			}
			fmt.Fprintf(b, `<path d="M%s %sL%s %sA%s %s 0 %d 1 %s %sZ" fill="%s" stroke="#FFFFFF" stroke-width="1.5" stroke-linejoin="round">%s</path>`,
				n2(cx), n2(cy), n2(cx+r*math.Cos(a)), n2(cy+r*math.Sin(a)), n2(r), n2(r), grande, n2(cx+r*math.Cos(a1)), n2(cy+r*math.Sin(a1)), p.color, titulo)
		}
		a = a1
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// La DECORACIÓN de la página de «Disposición» (`pagedecor.ts` y `setColumns` de exec.ts),
// con el mismo HTML: la marca de agua y el borde son capas `<div>` al principio del folio,
// con sus ajustes en `data-*` y su dibujo horneado en `style`; las columnas envuelven un
// tramo de bloques en un `<div class="sytue-cols">` con `column-count`.

// MarcaAgua es `WatermarkSpec`: un texto o una imagen, con su color, su opacidad, su tamaño
// (px de la letra; % del ancho, la imagen) y si va en diagonal.
type MarcaAgua struct {
	Imagen   bool
	Texto    string
	Src      string
	Color    string
	Opacidad float64
	Tamano   int
	Diagonal bool
}

// BordePagina es `PageBorderSpec`: el estilo (solid, dashed, dotted, double), el grosor y
// el color.
type BordePagina struct {
	Estilo string
	Grosor int
	Color  string
}

// ColorMarcaAgua y ColorBorde son los de por defecto de la web.
const (
	ColorMarcaAgua = "#94A3B8"
	ColorBorde     = "#475569"
)

var (
	reHex          = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	reImagenSegura = regexp.MustCompile(`(?i)^data:image/(png|jpe?g|gif|webp|avif);base64,[a-z0-9+/=\s]+$`)
)

func hexSeguro(crudo, reserva string) string {
	if s := strings.TrimSpace(crudo); reHex.MatchString(s) {
		return strings.ToUpper(s)
	}
	return reserva
}

func numero64(crudo string, reserva float64) float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(crudo), 64); err == nil && !math.IsInf(v, 0) && !math.IsNaN(v) {
		return v
	}
	return reserva
}

func acotar(n, lo, hi float64) float64 { return math.Min(hi, math.Max(lo, n)) }

// ImagenMarcaSegura es `isSafeWatermarkImage`: solo un data: URI ráster.
func ImagenMarcaSegura(src string) bool { return reImagenSegura.MatchString(strings.TrimSpace(src)) }

// SanearMarcaAgua es `sanitizeWatermarkSpec`: nil si no hay ni texto ni imagen.
func SanearMarcaAgua(imagen bool, texto, src, color, opacidad, tamano, diagonal string) *MarcaAgua {
	m := &MarcaAgua{
		Color:    hexSeguro(color, ColorMarcaAgua),
		Opacidad: redondeo(acotar(numero64(opacidad, 0.15), 0.02, 1), 100),
		Diagonal: diagonal == "1" || diagonal == "true",
	}
	if imagen && ImagenMarcaSegura(src) {
		m.Imagen, m.Src = true, strings.TrimSpace(src)
		m.Tamano = int(math.Round(acotar(numero64(tamano, 60), 10, 100)))
		return m
	}
	if m.Texto = strings.TrimSpace(texto); m.Texto == "" {
		return nil
	}
	m.Tamano = int(math.Round(acotar(numero64(tamano, 120), 24, 400)))
	return m
}

// SanearBorde es `sanitizePageBorderSpec`: nil con grosor 0.
func SanearBorde(estilo, grosor, color string) *BordePagina {
	g := int(math.Round(acotar(numero64(grosor, 0), 0, 12)))
	if g <= 0 {
		return nil
	}
	if !slices.Contains([]string{"solid", "dashed", "dotted", "double"}, estilo) {
		estilo = "solid"
	}
	return &BordePagina{Estilo: estilo, Grosor: g, Color: hexSeguro(color, ColorBorde)}
}

// El folio de la web mide 794 × 1123: es la baldosa de la marca de agua.
const (
	baldosaAncho = 794
	baldosaAlto  = 1123
)

var escXML = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")

// svgMarca es `watermarkSvg`.
func svgMarca(m *MarcaAgua) string {
	cx, cy := fnum(baldosaAncho/2.0), fnum(baldosaAlto/2.0) // 397 y 561.5
	rot := ""
	if m.Diagonal {
		rot = ` transform="rotate(-45 ` + cx + ` ` + cy + `)"`
	}
	cab := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` + strconv.Itoa(baldosaAncho) + ` ` + strconv.Itoa(baldosaAlto) + `" ` +
		`preserveAspectRatio="xMidYMid meet">`
	if m.Imagen {
		esc := acotar(float64(m.Tamano), 10, 100) / 100
		bw, bh := redondeo(baldosaAncho*esc, 100), redondeo(baldosaAlto*esc, 100)
		bx, by := redondeo((baldosaAncho-bw)/2, 100), redondeo((baldosaAlto-bh)/2, 100)
		return cab + `<image href="` + escXML.Replace(m.Src) + `" x="` + fnum(bx) + `" y="` + fnum(by) + `" width="` + fnum(bw) +
			`" height="` + fnum(bh) + `" preserveAspectRatio="xMidYMid meet"` + rot + `/></svg>`
	}
	fs := acotar(float64(m.Tamano), 24, 400)
	return cab + `<text x="` + cx + `" y="` + cy + `" fill="` + m.Color + `" font-family="Inter, system-ui, sans-serif" ` +
		`font-size="` + fnum(fs) + `" font-weight="700" letter-spacing="0.06em" text-anchor="middle" ` +
		`dominant-baseline="central"` + rot + `>` + escXML.Replace(m.Texto) + `</text></svg>`
}

func fnum(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// cssSVG es `encodeSvgForCss`.
var cssSVG = strings.NewReplacer("%", "%25", "<", "%3C", ">", "%3E", "#", "%23", `"`, "%22", "&", "%26", "+", "%2B", "\n", "%20")

// estiloMarca es `watermarkStyle`.
func estiloMarca(m *MarcaAgua) string {
	uri := "data:image/svg+xml," + cssSVG.Replace(svgMarca(m))
	return "position:absolute;inset:0;z-index:-1;pointer-events:none;user-select:none;" +
		"print-color-adjust:exact;-webkit-print-color-adjust:exact;" +
		"opacity:" + fnum(m.Opacidad) + ";" +
		"background-image:url('" + uri + "');" +
		"background-repeat:repeat-y;background-position:center top;" +
		"background-size:100% " + strconv.Itoa(baldosaAlto) + "px;"
}

// estiloBorde es `pageBorderStyle`.
func estiloBorde(b *BordePagina) string {
	inset := "calc(var(--doc-page-margin, 96px) / 2)"
	return "position:absolute;pointer-events:none;z-index:0;" +
		"top:" + inset + ";left:" + inset + ";right:" + inset + ";bottom:" + inset + ";" +
		"border:" + strconv.Itoa(b.Grosor) + "px " + b.Estilo + " " + b.Color + ";"
}

func esMarcaAgua(n *html.Node) bool {
	return n != nil && n.DataAtom == atom.Div && n.Namespace == "" && tieneAtributo(n, "data-sytue-watermark")
}

func esBordePagina(n *html.Node) bool {
	return n != nil && n.DataAtom == atom.Div && n.Namespace == "" && tieneAtributo(n, "data-sytue-page-border")
}

func esPaginaDoc(n *html.Node) bool {
	return n != nil && n.DataAtom == atom.Div && n.Namespace == "" && tieneAtributo(n, "data-sytue-page")
}

// capaDecorativa dice si n es una de las capas: datos del folio, que no son bloques.
func capaDecorativa(n *html.Node) bool { return esMarcaAgua(n) || esBordePagina(n) || esPaginaDoc(n) }

// Los atributos de cada capa, los de `watermarkHtml` y `pageBorderHtml`.
var (
	atributosMarca = []string{
		"data-sytue-watermark", "data-kind", "data-color", "data-opacity", "data-size", "data-diagonal",
		"data-text", "data-image", "contenteditable", "aria-hidden", "style",
	}
	atributosBorde  = []string{"data-sytue-page-border", "data-style", "data-width", "data-color", "contenteditable", "aria-hidden", "style"}
	atributosPagina = []string{"data-sytue-page", "data-width", "data-height", "data-margins", "data-color", "contenteditable", "aria-hidden"}
)

// entenderCapa apunta lo que impide editar una capa: que no cuelgue del folio, que lleve
// algo dentro o un atributo que no es de los suyos.
func entenderCapa(n *html.Node, motivos map[string]bool) {
	propios := atributosBorde
	switch {
	case esMarcaAgua(n):
		propios = atributosMarca
	case esPaginaDoc(n):
		propios = atributosPagina
	}
	if n.Parent == nil || n.Parent.DataAtom != atom.Body || n.FirstChild != nil {
		motivos["<div> de fondo de página"] = true
	}
	for _, a := range n.Attr {
		if a.Namespace != "" || !slices.Contains(propios, a.Key) {
			motivos["div["+a.Key+"]"] = true
		}
	}
}

func (d *Documento) capa(es func(*html.Node) bool) *html.Node {
	for h := d.raiz.FirstChild; h != nil; h = h.NextSibling {
		if es(h) {
			return h
		}
	}
	return nil
}

// MarcaAguaDe es `readWatermark`: la marca del folio, releída de sus `data-*`, o nil.
func (d *Documento) MarcaAguaDe() *MarcaAgua {
	n := d.capa(esMarcaAgua)
	if n == nil {
		return nil
	}
	return SanearMarcaAgua(atributo(n, "data-kind") == "image", atributo(n, "data-text"), atributo(n, "data-image"),
		atributo(n, "data-color"), atributo(n, "data-opacity"), atributo(n, "data-size"), atributo(n, "data-diagonal"))
}

// BordeDe es `readBorder`.
func (d *Documento) BordeDe() *BordePagina {
	n := d.capa(esBordePagina)
	if n == nil {
		return nil
	}
	return SanearBorde(atributo(n, "data-style"), atributo(n, "data-width"), atributo(n, "data-color"))
}

// ponerCapa es `applyLayer`: quita la capa que haya y, si hay otra, la pone la primera.
func (d *Documento) ponerCapa(es func(*html.Node) bool, nueva *html.Node) {
	if vieja := d.capa(es); vieja != nil {
		d.raiz.RemoveChild(vieja)
	}
	if nueva != nil {
		d.raiz.InsertBefore(nueva, d.raiz.FirstChild)
	}
}

// PonerMarcaAgua es `watermarkHtml` puesta en el folio (o quitada, con nil).
func (d *Documento) PonerMarcaAgua(m *MarcaAgua) {
	if m == nil {
		d.ponerCapa(esMarcaAgua, nil)
		return
	}
	kind := "text"
	if m.Imagen {
		kind = "image"
	}
	diag := "0"
	if m.Diagonal {
		diag = "1"
	}
	attrs := []html.Attribute{
		{Key: "data-sytue-watermark"},
		{Key: "data-kind", Val: kind},
		{Key: "data-color", Val: m.Color},
		{Key: "data-opacity", Val: fnum(m.Opacidad)},
		{Key: "data-size", Val: strconv.Itoa(m.Tamano)},
		{Key: "data-diagonal", Val: diag},
	}
	if m.Imagen {
		attrs = append(attrs, html.Attribute{Key: "data-image", Val: m.Src})
	} else {
		attrs = append(attrs, html.Attribute{Key: "data-text", Val: m.Texto})
	}
	attrs = append(attrs, html.Attribute{Key: "contenteditable", Val: "false"}, html.Attribute{Key: "aria-hidden", Val: "true"},
		html.Attribute{Key: "style", Val: estiloMarca(m)})
	d.ponerCapa(esMarcaAgua, elemento("div", attrs...))
}

// PonerBorde es `pageBorderHtml` puesto en el folio (o quitado, con nil).
func (d *Documento) PonerBorde(b *BordePagina) {
	if b == nil {
		d.ponerCapa(esBordePagina, nil)
		return
	}
	d.ponerCapa(esBordePagina, elemento("div",
		html.Attribute{Key: "data-sytue-page-border"}, html.Attribute{Key: "data-style", Val: b.Estilo},
		html.Attribute{Key: "data-width", Val: strconv.Itoa(b.Grosor)}, html.Attribute{Key: "data-color", Val: b.Color},
		html.Attribute{Key: "contenteditable", Val: "false"}, html.Attribute{Key: "aria-hidden", Val: "true"},
		html.Attribute{Key: "style", Val: estiloBorde(b)}))
}

// PaginaDoc es la página que guarda el documento (la capa `data-sytue-page` de `pagedecor.ts`,
// la que escribe el importador de .docx): tamaño y márgenes (arriba, derecha, abajo,
// izquierda) en px, y el color del papel («#RRGGBB», o "" para el blanco).
type PaginaDoc struct {
	Ancho, Alto float32
	Margenes    [4]float32
	Color       string
}

// PaginaDe es la página del documento, o nil si no la guarda. Lo que no se entiende vale lo
// del folio: A4 y 96 px.
func (d *Documento) PaginaDe() *PaginaDoc {
	n := d.capa(esPaginaDoc)
	if n == nil {
		return nil
	}
	num := func(v string, reserva, lo, hi float64) float32 {
		f := numero64(v, reserva)
		if f < lo || f > hi {
			f = reserva
		}
		return float32(f)
	}
	p := &PaginaDoc{Ancho: num(atributo(n, "data-width"), 794, 96, 4000), Alto: num(atributo(n, "data-height"), 1123, 96, 4000)}
	m := strings.Fields(atributo(n, "data-margins"))
	for i := range p.Margenes {
		p.Margenes[i] = 96
		if i < len(m) {
			p.Margenes[i] = num(m[i], 96, 0, 2000)
		}
	}
	if c := atributo(n, "data-color"); c != "" {
		p.Color = hexSeguro(c, "")
	}
	return p
}

// PonerPaginaDoc guarda la página en el documento (o la quita, con nil).
func (d *Documento) PonerPaginaDoc(p *PaginaDoc) {
	if p == nil {
		d.ponerCapa(esPaginaDoc, nil)
		return
	}
	m := make([]string, 4)
	for i, v := range p.Margenes {
		m[i] = fnum(math.Round(float64(v)*100) / 100)
	}
	attrs := []html.Attribute{
		{Key: "data-sytue-page"},
		{Key: "data-width", Val: fnum(math.Round(float64(p.Ancho)*100) / 100)},
		{Key: "data-height", Val: fnum(math.Round(float64(p.Alto)*100) / 100)},
		{Key: "data-margins", Val: strings.Join(m, " ")},
	}
	if p.Color != "" {
		attrs = append(attrs, html.Attribute{Key: "data-color", Val: p.Color})
	}
	attrs = append(attrs, html.Attribute{Key: "contenteditable", Val: "false"}, html.Attribute{Key: "aria-hidden", Val: "true"})
	d.ponerCapa(esPaginaDoc, elemento("div", attrs...))
}

// ---- columnas ----

// reColumnas es el `style` que deja `setColumns`, tal cual lo escribe el navegador.
var reColumnas = regexp.MustCompile(`^column-count: ([123]);(?: column-gap: 2em;)?$`)

func esColumnas(n *html.Node) bool {
	return n != nil && n.DataAtom == atom.Div && n.Namespace == "" && atributo(n, "class") == "sytue-cols"
}

// columnasDe es cuántas columnas tiene un envoltorio (su `column-count`), 1 si no se lee.
func columnasDe(n *html.Node) int {
	if m := reColumnas.FindStringSubmatch(atributo(n, "style")); m != nil {
		v, _ := strconv.Atoi(m[1])
		return v
	}
	return 1
}

// entenderColumnas: el envoltorio va en el folio, con su clase, su `data-cols` y el `style`
// de `setColumns`.
//
// ponytail: una tabla o un índice dentro de las columnas no se editan —la vista los coloca
// desde el borde del folio, no desde el de su columna—.
func entenderColumnas(n *html.Node, motivos map[string]bool) {
	if n.Parent == nil || n.Parent.DataAtom != atom.Body {
		motivos["div.sytue-cols dentro de otro"] = true
	}
	for _, a := range []atom.Atom{atom.Table, atom.Nav} {
		if len(elementosDe(n, a)) > 0 {
			motivos["<"+a.String()+"> en columnas"] = true
		}
	}
	for _, a := range n.Attr {
		switch {
		case a.Namespace != "":
		case a.Key == "class" || a.Key == "data-cols":
			continue
		case a.Key == "style" && reColumnas.MatchString(a.Val):
			continue
		}
		motivos["div.sytue-cols["+a.Key+"]"] = true
	}
}

// Columnas es `setColumns(n)` sobre los bloques [i, j]: los hijos del folio que tocan se
// envuelven en n columnas; si ya son un envoltorio, se ajusta, y con 1 se deshace. Devuelve
// el lugar donde queda el cursor y si ha cambiado algo.
func (d *Documento) Columnas(bloques []Bloque, i, j, n int) (Lugar, bool) {
	n = min(3, max(1, n))
	if i < 0 || j >= len(bloques) || i > j {
		return Lugar{}, false
	}
	arriba := func(b Bloque) *html.Node {
		x := b.primero
		if b.cont != nil && b.cont.DataAtom != atom.Body {
			x = b.cont
		}
		for x != nil && x.Parent != nil && x.Parent != d.raiz {
			x = x.Parent
		}
		return x
	}
	a, z := arriba(bloques[i]), arriba(bloques[j])
	if a == nil || z == nil {
		return Lugar{}, false
	}
	if a == z && esColumnas(a) {
		if n == 1 {
			primero := a.FirstChild
			for h := a.FirstChild; h != nil; h = a.FirstChild {
				a.RemoveChild(h)
				d.raiz.InsertBefore(h, a)
			}
			d.raiz.RemoveChild(a)
			if primero == nil {
				return Lugar{}, true
			}
			return d.primeroDesde(primero)
		}
		ponerAtributo(a, "data-cols", strconv.Itoa(n))
		ponerDeclaracion(a, "column-count", strconv.Itoa(n))
		return d.primeroDesde(a)
	}
	if n == 1 {
		return Lugar{}, false
	}
	div := elemento("div", html.Attribute{Key: "class", Val: "sytue-cols"}, html.Attribute{Key: "data-cols", Val: strconv.Itoa(n)},
		html.Attribute{Key: "style", Val: "column-count: " + strconv.Itoa(n) + "; column-gap: 2em;"})
	d.raiz.InsertBefore(div, a)
	for h := a; h != nil; {
		sig := h.NextSibling
		d.raiz.RemoveChild(h)
		div.AppendChild(h)
		if h == z {
			break
		}
		h = sig
	}
	return d.primeroDesde(div)
}

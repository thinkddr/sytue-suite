// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"cmp"
	"encoding/xml"
	"html"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Diseños de una presentación: dentro del patrón (`master.layouts`), cada uno con su
// nombre y sus marcadores de posición —cajas de texto con su muestra— de los que parte
// una diapositiva nueva. Cada diapositiva dice cuál lleva (`layout`). Es lo que el editor
// guarda en `SlideLayout` (slides/disenos.ts); aquí se sanean igual, y se escriben como
// los diseños (`slideLayout`) de un .pptx y se leen de ellos.

// slideLayout es un diseño.
type slideLayout struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Boxes []slideBox `json:"boxes"`
	// Decor son las formas del diseño que no son marcadores —un logo, una franja, un cuadro
	// de texto fijo— y Connectors, sus líneas: se guardan una vez aquí y se pintan debajo de
	// cada diapositiva que lleva el diseño, encima del patrón, como en PowerPoint. HideMaster
	// es que el diseño oculta las cajas del patrón (`showMasterSp="0"`). Los tres son de
	// 2026-10 y opcionales: un diseño de antes no los lleva y se pinta como siempre.
	Decor      []slideBox       `json:"decor,omitempty"`
	Connectors []slideConnector `json:"connectors,omitempty"`
	HideMaster bool             `json:"hideMaster,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
}

const (
	// maxDisenos y maxCajasDiseno son los techos del editor (MAX_LAYOUTS y MAX_PLACEHOLDERS).
	maxDisenos      = 24
	maxCajasDiseno  = 12
	maxNombreDiseno = 60
	// maxDecorDiseno y maxLineasDiseno acotan las formas y las líneas de un diseño
	// (MAX_LAYOUT_DECOR y MAX_LAYOUT_CONNECTORS).
	maxDecorDiseno  = 200
	maxLineasDiseno = 100
)

// nsRelaciones es el espacio de nombres de los atributos de relación de OOXML (`r:id`), y
// nsRelacionesEstricto, el mismo en OOXML estricto (ISO 29500 Strict).
const (
	nsRelaciones         = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsRelacionesEstricto = "http://purl.oclc.org/ooxml/officeDocument/relationships"
)

var reIDDiseno = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

// disenosValidos rehace una lista de diseños: ids válidos y únicos, el nombre acotado y,
// de cada uno, solo las cajas de texto (los marcadores), hasta el tope y dentro del lienzo.
func disenosValidos(in []slideLayout) []slideLayout {
	var out []slideLayout
	vistos := map[string]bool{}
	for _, l := range in {
		if len(out) >= maxDisenos {
			break
		}
		if !reIDDiseno.MatchString(l.ID) || vistos[l.ID] {
			continue
		}
		vistos[l.ID] = true
		name := strings.TrimSpace(l.Name)
		if utf8.RuneCountInString(name) > maxNombreDiseno {
			name = string([]rune(name)[:maxNombreDiseno])
		}
		nl := slideLayout{ID: l.ID, Name: name, Boxes: []slideBox{}, HideMaster: l.HideMaster}
		nl.Decor = l.Decor[:min(len(l.Decor), maxDecorDiseno)]
		nl.Connectors = l.Connectors[:min(len(l.Connectors), maxLineasDiseno)]
		for _, b := range l.Boxes {
			if len(nl.Boxes) >= maxCajasDiseno {
				break
			}
			if b.Kind != "" && b.Kind != "text" {
				continue
			}
			nl.Boxes = append(nl.Boxes, slideBox{
				ID: b.ID, Kind: "text", HTML: b.HTML,
				X: clampPct(b.X), Y: clampPct(b.Y), W: clampPct(b.W), H: clampPct(b.H),
			})
		}
		out = append(out, nl)
	}
	return out
}

func clampPct(v float64) float64 {
	return min(100, max(0, v))
}

// --- Exportación a .pptx ---

// pptxLayoutsOf son los diseños que se escriben en el .pptx —el primero, en blanco, es el
// de las diapositivas sin diseño— y el índice de cada diseño del modelo entre ellos.
func pptxLayoutsOf(m slidesModel) ([]pptxLayout, map[string]int) {
	index := map[string]int{}
	if m.Master == nil || len(m.Master.Layouts) == 0 {
		// Los tres del editor, tal cual: el modelo que no los toca no los lleva.
		for i, id := range []string{"titulo", "titulo-cuerpo", "dos-columnas"} {
			index[id] = i + 1
		}
		return pptxLayouts, index
	}
	out := []pptxLayout{pptxLayouts[0]}
	for i, l := range disenosValidos(m.Master.Layouts) {
		out = append(out, pptxLayoutOf(l))
		index[l.ID] = i + 1
	}
	return out, index
}

// pptxLayoutOf es un diseño del modelo como diseño de .pptx: la primera caja con un
// título es el título, y las demás, marcadores de contenido por orden.
func pptxLayoutOf(l slideLayout) pptxLayout {
	pl := pptxLayout{typ: "cust", name: l.Name, decor: l.Decor, conectores: l.Connectors, ocultaPatron: l.HideMaster}
	haTitulo := false
	idx := 0
	for i, b := range l.Boxes {
		ph := pptxPh{x: b.X, y: b.Y, w: b.W, h: b.H, text: plainParas(b.HTML)}
		if i == 0 && esTitulo(b.HTML) {
			ph.typ, haTitulo = "title", true
		} else {
			idx++
			ph.idx = idx
		}
		pl.phs = append(pl.phs, ph)
	}
	if !haTitulo && len(pl.phs) == 0 {
		pl.typ = "blank"
	}
	return pl
}

// esTitulo dice si el HTML de una caja empieza por un título.
func esTitulo(htmlStr string) bool {
	for _, blk := range parseHTMLBlocks(htmlStr) {
		return blk.para != nil && strings.HasPrefix(blk.para.style, "Heading")
	}
	return false
}

// plainParas son los párrafos de una caja como DrawingML sin formato: el texto de la muestra
// de un marcador, que el diseño enseña como indicación.
func plainParas(htmlStr string) string {
	var b strings.Builder
	for _, blk := range parseHTMLBlocks(htmlStr) {
		if blk.para == nil {
			continue
		}
		var t strings.Builder
		for _, r := range blk.para.runs {
			if !r.brk && r.image == nil {
				t.WriteString(r.text)
			}
		}
		if t.Len() > 0 {
			b.WriteString(`<a:p><a:r><a:rPr lang="es-ES"/><a:t>` + xmlEscape(t.String()) + `</a:t></a:r></a:p>`)
		}
	}
	if b.Len() == 0 {
		return `<a:p><a:endParaRPr lang="es-ES"/></a:p>`
	}
	return b.String()
}

// --- Importación de .pptx ---

var reEtiquetas = regexp.MustCompile(`<[^>]*>`)

// tituloDe es el HTML de una caja de título con el texto de otra: un `<h1>` con su texto.
func tituloDe(htmlStr string) string {
	t := strings.TrimSpace(html.UnescapeString(reEtiquetas.ReplaceAllString(htmlStr, "")))
	return "<h1>" + html.EscapeString(t) + "</h1>"
}

// muestraDeMarcador es la muestra de un marcador de un diseño como caja del editor: un
// título para el título, y para el resto solo el primer párrafo de la indicación (que en
// PowerPoint trae un párrafo por nivel).
func muestraDeMarcador(typ, htmlStr string) string {
	if typ == "title" || typ == "ctrTitle" {
		return tituloDe(htmlStr)
	}
	for _, b := range parseHTMLBlocks(htmlStr) {
		if b.para != nil {
			p := *b.para
			p.list, p.level = "", 0
			return blocksToHTML([]xblock{{para: &p}})
		}
	}
	return ""
}

// readPptxLayouts lee los diseños del patrón —sus marcadores de contenido con su sitio y su
// muestra— en el orden en que el patrón los lista, y el id de cada parte de diseño. El
// diseño en blanco (sin marcadores) no es un diseño del modelo: es no tener ninguno.
func readPptxLayouts(masterPath string, files map[string][]byte, wEMU, hEMU float64, pt *pptxTheme) ([]slideLayout, map[string]string) {
	raw := files[masterPath]
	rels := parseRels(files[relsPath(masterPath)])
	var root xnode
	if len(raw) == 0 || xml.Unmarshal(raw, &root) != nil {
		return nil, nil
	}
	ids := map[string]string{} // ruta de la parte → id del diseño
	var out []slideLayout
	for _, e := range root.kids("sldLayoutIdLst") {
		target := rels[cmp.Or(e.attrNS(nsRelaciones, "id"), e.attrNS(nsRelacionesEstricto, "id"))]
		if target == "" || len(out) >= maxDisenos {
			continue
		}
		name := path.Clean(path.Dir(masterPath) + "/" + target)
		l, ok := readPptxLayout(name, "layout-"+strconv.Itoa(len(out)+1), files, wEMU, hEMU, pt)
		if ok {
			ids[name] = l.ID
			out = append(out, l)
		}
	}
	return out, ids
}

// readPptxLayout lee un diseño: sus marcadores de contenido y sus formas. Lo devuelve también
// en blanco (sin nada), con false.
func readPptxLayout(name, id string, files map[string][]byte, wEMU, hEMU float64, pt *pptxTheme) (slideLayout, bool) {
	raw := files[name]
	if len(raw) == 0 {
		return slideLayout{}, false
	}
	var head struct {
		CSld struct {
			Name string `xml:"name,attr"`
		} `xml:"cSld"`
	}
	_ = xml.Unmarshal(raw, &head)
	saved := *pt
	pt.layout, pt.soloMarcadores = phGeoms{}, true // la geometría que falte, la del patrón
	sl := parseSlide(id, raw, path.Dir(name), parseRels(files[relsPath(name)]), files, wEMU, hEMU, pt, false)
	*pt = saved
	decor, lineas := formasDeParte(name, id+"d", files, wEMU, hEMU, pt)
	l := slideLayout{
		ID: id, Name: cmp.Or(strings.TrimSpace(head.CSld.Name), id), Boxes: sl.Boxes, Decor: decor,
		Connectors: lineas, HideMaster: sl.HideMaster,
	}
	return l, len(sl.Boxes) > 0 || len(decor) > 0 || len(lineas) > 0
}

// formasDeParte son las formas de una parte de diseño o de patrón que no son marcadores, y sus
// líneas: se lee como un patrón (que se salta los marcadores), con los extremos de las líneas
// sueltos, porque no hay caja de diapositiva a la que anclarlos. Su fondo no: lo hereda cada
// diapositiva (heredarFondo).
func formasDeParte(parte, id string, files map[string][]byte, wEMU, hEMU float64, pt *pptxTheme) ([]slideBox, []slideConnector) {
	saved := *pt
	pt.soloMarcadores, pt.extremosSueltos = false, true
	deco := parseSlide(id, files[parte], path.Dir(parte), parseRels(files[relsPath(parte)]), files, wEMU, hEMU, pt, true)
	*pt = saved
	var formas []slideBox
	for _, b := range deco.Boxes {
		if b.ID != deco.ID+"bg" {
			formas = append(formas, b)
		}
	}
	return formas, deco.Connectors
}

// disenoDeOtroPatron pone en el modelo, la primera vez que una diapositiva lo usa, un diseño
// de otro patrón que el del modelo (decisión del dueño, 2026-10-16): sus marcadores y, en sus
// formas, debajo de las suyas, las de su patrón (`patron`, ya en uso en pt), porque el modelo
// tiene un solo patrón y el diseño lo tapa. Si otro diseño ya se llama igual, el nombre lleva
// el del patrón detrás («Título · Oscuro»). Devuelve si la diapositiva tiene que tapar ella el
// patrón del modelo: si es de otro y su diseño no entra.
func (m *slidesModel) disenoDeOtroPatron(ids map[string]string, diseno, patron string, pa *patronPptx, files map[string][]byte, wEMU, hEMU float64, pt *pptxTheme) bool {
	if _, ok := ids[diseno]; pa == nil || ok {
		return false
	}
	if ids == nil || diseno == "" || m.Master != nil && len(m.Master.Layouts) >= maxDisenos {
		return true
	}
	if m.Master == nil {
		m.Master = &slideMaster{Boxes: []slideBox{}}
	}
	// Los del primer patrón son layout-1…layout-N, seguidos: el siguiente número está libre.
	id := "layout-" + strconv.Itoa(len(m.Master.Layouts)+1)
	l, ok := readPptxLayout(diseno, id, files, wEMU, hEMU, pt)
	if !l.HideMaster {
		formas, lineas := formasDeParte(patron, id+"p", files, wEMU, hEMU, pt)
		l.Decor, l.Connectors = append(formas, l.Decor...), append(lineas, l.Connectors...)
		ok = ok || len(formas) > 0 || len(lineas) > 0
	}
	if !ok {
		return true
	}
	l.HideMaster = true // el patrón del modelo no es el suyo
	for _, o := range m.Master.Layouts {
		if o.Name == l.Name {
			l.Name += " · " + cmp.Or(pa.nombre, "otro patrón")
			break
		}
	}
	m.Master.Layouts = append(m.Master.Layouts, l)
	ids[diseno] = id
	return false
}

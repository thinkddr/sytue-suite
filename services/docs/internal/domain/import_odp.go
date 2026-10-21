// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Importador .odp (OASIS ODF 1.3) -> doc:slides: el reverso de export_odp.go. Lee el
// contenido de cada diapositiva (texto, formas, imágenes, medios, conectores, notas) y
// lo que la rodea: el tema y el patrón de la página maestra, el fondo, la transición y
// el patrón visible u oculto de cada una y su árbol de animaciones.

// odpState es lo que el lector de una presentación lleva además de lo común de ODF.
type odpState struct {
	look   odpLook
	ink    string         // tinta del tema (RRGGBB), para no tomarla por un color propio
	master bool           // se están leyendo las cajas de la página maestra
	ids    map[string]int // draw:id → índice de caja de la diapositiva en curso
	// layoutIDs: nombre del estilo de diseño de ODF → id del diseño del modelo.
	layoutIDs map[string]string
	conns     []odpConn // conectores de la diapositiva en curso
	timing    *xnode    // árbol de animaciones de la diapositiva en curso
}

func importOdp(data []byte, p *presupuesto) (Kind, string, error) {
	files, err := readZip(data)
	if err != nil {
		return "", "", err
	}
	content := files["content.xml"]
	if len(content) == 0 {
		return "", "", wrapFormat("odp: falta content.xml")
	}
	styles := map[string]odfStyleInfo{}
	if styXML := files["styles.xml"]; len(styXML) > 0 {
		for k, v := range parseODFStyles(styXML) {
			styles[k] = v
		}
	}
	for k, v := range parseODFStyles(content) {
		styles[k] = v
	}
	ancho, alto := tamanoPaginaODP(content, files["styles.xml"])
	look := odpLook{primaria: readOdpStyles(content), otra: readOdpStyles(files["styles.xml"])}
	model := &slidesModel{}
	_, ink := readODPMaster(files, content, styles, look, ancho, alto, model)
	layoutIDs := readODPLayouts(files["styles.xml"], ancho, alto, model)
	leerFormasDeDisenoODP(files, styles, look, ink, ancho, alto, model)
	parseODPBody(model, content, styles, files, look, ink, layoutIDs, ancho, alto, p)
	model.quitarFondosDelTema()
	model.Size = tamanoDiapositiva(ancho, alto)
	if len(model.Slides) == 0 {
		model.Slides = []slideT{{ID: "s1", Boxes: []slideBox{}}}
	}
	for i := range model.Slides {
		if model.Slides[i].Boxes == nil {
			model.Slides[i].Boxes = []slideBox{}
		}
	}
	return KindPresentation, marshalSlidesModel(model), nil
}

// readODPMaster lee la página maestra de la primera diapositiva: el tema (su fondo, y la
// tinta y la fuente del estilo «standard») y las cajas que no son marcadores de
// posición. Devuelve el fondo del patrón y la tinta del tema (RRGGBB).
func readODPMaster(files map[string][]byte, content []byte, styles map[string]odfStyleInfo, look odpLook, ancho, alto float64, model *slidesModel) (bg, ink string) {
	sty := files["styles.xml"]
	if len(sty) == 0 {
		return "", ""
	}
	look = odpLook{primaria: look.otra, otra: look.primaria}
	d := &odfDec{styles: styles, listSty: parseODFListStyles(sty), files: files, paginaW: ancho, paginaH: alto}
	d.look, d.master, d.ids = look, true, map[string]int{}
	d.dec = xml.NewDecoder(bytes.NewReader(sty))
	want := primerAtributo(content, "page", "master-page-name")
	if strings.HasPrefix(want, "M_") && bytes.Contains(sty, []byte(`style:name="`+want+`" `)) && bytes.Contains(sty, []byte(odpNSSytue)) {
		want = "" // la de un diseño con formas (la escribe export_odp.go): la del patrón es la primera
	}
	var page xml.StartElement
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return "", ""
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "master-page" && (want == "" || attrLocal(se, "name") == want) {
			page = se
			break
		}
	}
	bg = look.resolver(attrLocal(page, "style-name")).fondoPropio()
	std := look.resolver("standard")
	ink = normalizeColor(std.color)
	d.ink = ink
	sl := slideT{ID: "m", Boxes: []slideBox{}}
	d.readODPElements("master-page", &sl, files)
	if bg != "" {
		model.temaAjeno(bg, ink, std.font, attrLocal(page, "display-name"))
	}
	if len(sl.Boxes) > 0 {
		model.Master = &slideMaster{Boxes: sl.Boxes}
	}
	return bg, ink
}

// leerFormasDeDisenoODP devuelve a los diseños del modelo las formas que export_odp.go
// escribió en su página maestra: el grupo marcado `sytue:decor` de cada página con
// `sytue:layout`, y si el diseño ocultaba el patrón.
func leerFormasDeDisenoODP(files map[string][]byte, styles map[string]odfStyleInfo, look odpLook, ink string, ancho, alto float64, model *slidesModel) {
	sty := files["styles.xml"]
	if model.Master == nil || len(model.Master.Layouts) == 0 || !bytes.Contains(sty, []byte(`decor="true"`)) {
		return
	}
	porID := map[string]*slideLayout{}
	for i := range model.Master.Layouts {
		porID[model.Master.Layouts[i].ID] = &model.Master.Layouts[i]
	}
	d := &odfDec{styles: styles, listSty: parseODFListStyles(sty), files: files, paginaW: ancho, paginaH: alto}
	d.look, d.ink = odpLook{primaria: look.otra, otra: look.primaria}, ink
	d.dec = xml.NewDecoder(bytes.NewReader(sty))
	var l *slideLayout
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return
		}
		se, ok := tok.(xml.StartElement)
		switch {
		case !ok:
		case se.Name.Local == "master-page":
			l = porID[attrSytue(se, "layout")]
			if l != nil {
				l.HideMaster = attrSytue(se, "hideMaster") == "true"
			}
		case se.Name.Local == "g" && l != nil && attrSytue(se, "decor") == "true":
			sl := slideT{ID: l.ID + "d", Boxes: []slideBox{}}
			d.ids, d.conns = map[string]int{}, nil
			d.readODPElements("g", &sl, files)
			d.resolveODPConnectors(&sl, d.conns)
			l.Decor, l.Connectors = sl.Boxes, sl.Connectors
		}
	}
}

// leerGrupo lee un `draw:g`. En una página maestra, el de las formas de un diseño se salta:
// es del diseño, no del patrón (leerFormasDeDisenoODP).
func (d *odfDec) leerGrupo(t xml.StartElement, sl *slideT, files map[string][]byte) {
	if d.master && attrSytue(t, "decor") == "true" {
		_ = d.dec.Skip()
		return
	}
	d.descend(func() { d.readODPElements("g", sl, files) })
}

// parseODPBody recorre <office:presentation> reconstruyendo diapositivas.
func parseODPBody(model *slidesModel, content []byte, styles map[string]odfStyleInfo, files map[string][]byte, look odpLook, ink string, layoutIDs map[string]string, ancho, alto float64, p *presupuesto) {
	dec := p.decodificador(content)
	d := &odfDec{dec: dec, styles: styles, listSty: parseODFListStyles(content), files: files, paginaW: ancho, paginaH: alto}
	d.look, d.ink, d.layoutIDs = look, ink, layoutIDs
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "page" {
			model.Slides = append(model.Slides, d.readODPPage(se, len(model.Slides)+1, files))
		}
	}
}

// readODPElements procesa recursivamente los elementos dentro de una página o grupo ODF.
func (d *odfDec) readODPElements(stopTag string, sl *slideT, files map[string][]byte) {
	for {
		tok, err := d.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			id := attrLocal(t, "id")
			add := func(b slideBox) {
				b.ID = sl.ID + "b" + strconv.Itoa(len(sl.Boxes)+1)
				if id != "" {
					d.ids[id] = len(sl.Boxes)
				}
				sl.Boxes = append(sl.Boxes, b)
			}
			switch t.Name.Local {
			case "g":
				d.leerGrupo(t, sl, files)
			case "frame":
				if b, ok := d.readODPFrame(t, files); ok {
					add(b)
				}
			case "rect", "ellipse", "circle", "line", "custom-shape", "polygon", "polyline", "path":
				add(d.readODPShape(t))
			case "connector":
				k := d.readODPConnector(t)
				if !d.master {
					d.conns = append(d.conns, k)
				}
			case "par":
				var n xnode
				if d.dec.DecodeElement(&n, &t) == nil && !d.master {
					d.timing = &n
				}
			case "notes":
				sl.Notes = d.readODPNotes()
			default:
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == stopTag {
				return
			}
		}
	}
}

// readODPPage lee una <draw:page> a slideT: sus cajas, conectores y animaciones, las
// notas, el fondo propio, el patrón visible u oculto y la transición.
func (d *odfDec) readODPPage(se xml.StartElement, n int, files map[string][]byte) slideT {
	sl := slideT{ID: "s" + strconv.Itoa(n), Boxes: []slideBox{}}
	st := d.look.resolver(attrLocal(se, "style-name"))
	sl.HideMaster = st.bgObjects == "false"
	if bg := st.fondoPropio(); bg != "" {
		sl.Bg = "#" + bg
	}
	sl.Transition = odpTransition(st)
	sl.Layout = d.layoutIDs[attrLocal(se, "presentation-page-layout-name")]
	d.ids, d.conns, d.timing = map[string]int{}, nil, nil
	d.readODPElements("page", &sl, files)
	d.resolveODPConnectors(&sl, d.conns)
	if d.timing != nil {
		readOdpTiming(d.timing, d.ids, sl.Boxes)
	}
	return sl
}

// odpMediaSrc es el origen de un medio: su fichero del paquete como data:URL, o su URL
// si es externa; "" si no se puede llevar (fichero ajeno al paquete, tipo desconocido o
// demasiado grande).
func odpMediaSrc(href string, files map[string][]byte) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	name := strings.TrimPrefix(strings.TrimPrefix(href, "./"), "../")
	raw := files[name]
	mime := mediaExts[strings.ToLower(strings.TrimPrefix(pathExt(name), "."))]
	if len(raw) == 0 || mime == "" || base64.StdEncoding.EncodedLen(len(raw)) > maxMedioImportado {
		return ""
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
}

func pathExt(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i:]
	}
	return ""
}

// odpMediaBox completa la caja de un marco con un <draw:plugin>: un vídeo o un audio con
// su póster; false si el marco no lleva un medio que se pueda llevar.
func odpMediaBox(b slideBox, plug odpPlugin, imgHref, img string, files map[string][]byte) (slideBox, bool) {
	href := plug.href
	src := ""
	if href != "" {
		src = odpMediaSrc(href, files)
	}
	if src == "" {
		return b, false
	}
	b.Kind, b.Src, b.Loop, b.MediaType = "media", src, plug.loop, "video"
	b.Tracks = odpTracks(plug.tracks, files)
	if len(b.Tracks) == 0 && !strings.Contains(href, "://") {
		b.Tracks = pistasJunto(files, strings.TrimPrefix(strings.TrimPrefix(href, "./"), "../"), "")
	}
	if strings.HasPrefix(mediaExts[strings.ToLower(strings.TrimPrefix(pathExt(href), "."))], "audio/") {
		b.MediaType = "audio"
	}
	// El póster liso que pone el exportador cuando el medio no traía el suyo no es del
	// usuario: no se importa.
	if img != "" && !bytes.Equal(files[strings.TrimPrefix(imgHref, "./")], posterPNG()) {
		b.Poster = img
	}
	return b, true
}

// odpPlugin es un <draw:plugin> leído: el origen del medio, si va en bucle y las pistas de
// subtítulos que el exportador escribe dentro (`sytue:track`).
type odpPlugin struct {
	href   string
	loop   bool
	tracks []odpTrackRef
}

type odpTrackRef struct {
	href, lang, label string
	def               bool
}

// readODPPlugin lee un <draw:plugin>.
func (d *odfDec) readODPPlugin(se xml.StartElement) odpPlugin {
	pl := odpPlugin{href: attrLocal(se, "href")}
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return pl
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Local == "param" && attrLocal(t, "name") == "Loop":
				pl.loop = attrLocal(t, "value") == "true"
			case t.Name.Local == "track":
				pl.tracks = append(pl.tracks, odpTrackRef{
					href: attrLocal(t, "href"), lang: attrLocal(t, "lang"), label: attrLocal(t, "label"),
					def: attrLocal(t, "default") == "true",
				})
			}
			_ = d.dec.Skip()
		case xml.EndElement:
			if t.Name.Local == "plugin" {
				return pl
			}
		}
	}
}

// odpTracks lee los .vtt de las pistas de un medio y los pasa por el saneado del editor.
func odpTracks(refs []odpTrackRef, files map[string][]byte) []slideTrack {
	var in []slideTrack
	for _, r := range refs {
		raw := files[strings.TrimPrefix(r.href, "./")]
		if len(raw) > 4*maxVTT {
			continue
		}
		in = append(in, slideTrack{Lang: r.lang, Label: r.label, VTT: string(raw), Default: r.def})
	}
	return pistasValidas(in)
}

// readODPFrame lee un <draw:frame>: caja de texto, imagen embebida o medio (un
// <draw:plugin> con su póster). En la página maestra, un marco de presentación
// (título, esquema, pie…) es un marcador de posición, no una caja del patrón.
func (d *odfDec) readODPFrame(se xml.StartElement, files map[string][]byte) (slideBox, bool) {
	if d.master && attrLocal(se, "class") != "" {
		_ = d.dec.Skip()
		return slideBox{}, false
	}
	b := slideBox{
		X: odpPct(attrLocal(se, "x"), d.paginaW), Y: odpPct(attrLocal(se, "y"), d.paginaH),
		W: odpPct(attrLocal(se, "width"), d.paginaW), H: odpPct(attrLocal(se, "height"), d.paginaH),
	}
	ok := false
	var directBlocks []xblock
	var plug odpPlugin
	var imgHref string
	img := ""
fin:
	for {
		tok, err := d.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "text-box":
				b.Kind = "text"
				b.HTML = blocksToHTML(d.readTextBlocks("text-box", "", 0))
				ok = true
			case "p":
				runs := d.readRuns("p", xrun{})
				directBlocks = append(directBlocks, xblock{para: &xpara{runs: runs}})
			case "image":
				imgHref = attrLocal(t, "href")
				img = dataURLFromZip(imgHref, files)
				_ = d.dec.Skip()
			case "plugin":
				plug = d.readODPPlugin(t)
			default:
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == "frame" {
				break fin
			}
		}
	}
	if m, isMedia := odpMediaBox(b, plug, imgHref, img, files); isMedia {
		b, ok = m, true
	} else if img != "" {
		b.Kind, b.Src = "image", img
		ok = true
	} else if !ok && len(directBlocks) > 0 {
		b.Kind = "text"
		b.HTML = blocksToHTML(directBlocks)
		ok = true
	}
	if b.W <= 0 {
		b.W = 30
	}
	if b.H <= 0 {
		b.H = 10
	}
	return b, ok
}

// odpShapeOf es la forma del editor de una geometría de ODF (`draw:type` de su
// `draw:enhanced-geometry`): el inverso de odpShapeTypes, y el cilindro del exportador
// (por su trazado propio) o el de LibreOffice. Cualquier otra es un rectángulo.
func odpShapeOf(typ, path string) string {
	switch typ {
	case "ellipse", "circle":
		return "ellipse"
	case "can":
		return "cylinder"
	case "round-rectangular-callout":
		return "callout"
	case "non-primitive":
		if strings.Contains(path, "C 0 -1200 21600 -1200") {
			return "cylinder"
		}
	}
	for shape, t := range odpShapeTypes {
		if t == typ {
			return shape
		}
	}
	return "rect"
}

// readODPShapeBody lee el contenido de una forma hasta su cierre: su texto (párrafos,
// títulos, listas y cuadros de texto) y su geometría.
func (d *odfDec) readODPShapeBody(tag string, b *slideBox) []xblock {
	var blocks []xblock
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return blocks
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				blocks = append(blocks, xblock{para: &xpara{runs: d.readRuns("p", xrun{})}})
			case "h":
				lvl := min(max(atoiDefault(attrLocal(t, "outline-level"), 1), 1), 6)
				p := xpara{style: "Heading" + strconv.Itoa(lvl), align: d.paraAlign(t)}
				p.runs = d.readRuns("h", xrun{})
				blocks = append(blocks, xblock{para: &p})
			case "list":
				kind := d.listKind(attrLocal(t, "style-name"), 1, "")
				d.descend(func() { blocks = append(blocks, d.readTextBlocks("list", kind, 1)...) })
			case "text-box":
				blocks = append(blocks, d.readTextBlocks("text-box", "", 0)...)
			case "enhanced-geometry":
				b.Shape = odpShapeOf(attrLocal(t, "type"), attrLocal(t, "enhanced-path"))
				_ = d.dec.Skip()
			default:
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == tag {
				return blocks
			}
		}
	}
}

// readODPShape lee una forma (draw:rect/ellipse/line/custom-shape) a un slideBox.
func (d *odfDec) readODPShape(se xml.StartElement) slideBox {
	st := d.look.resolver(attrLocal(se, "style-name"))
	b := slideBox{Kind: "shape", Shape: "rect", Fill: st.relleno(), Stroke: st.trazo()}
	switch se.Name.Local {
	case "ellipse", "circle":
		b.Shape = "ellipse"
	case "line":
		b.Shape = "line"
	}
	if se.Name.Local == "line" {
		x1, y1 := odpPct(attrLocal(se, "x1"), d.paginaW), odpPct(attrLocal(se, "y1"), d.paginaH)
		x2, y2 := odpPct(attrLocal(se, "x2"), d.paginaW), odpPct(attrLocal(se, "y2"), d.paginaH)
		b.X, b.Y, b.W, b.H = x1, y1, x2-x1, y2-y1
	} else {
		b.X = odpPct(attrLocal(se, "x"), d.paginaW)
		b.Y = odpPct(attrLocal(se, "y"), d.paginaH)
		b.W = odpPct(attrLocal(se, "width"), d.paginaW)
		b.H = odpPct(attrLocal(se, "height"), d.paginaH)
	}

	shapeBlocks := d.readODPShapeBody(se.Name.Local, &b)
	visible := func(c string) bool { return c != "" && c != "transparent" }
	if len(shapeBlocks) > 0 {
		b.HTML = blocksToHTML(shapeBlocks)
		// Sin relleno ni trazo que ver es un cuadro de texto; y una forma personalizada que
		// no dice su relleno (solo los trae heredados) es el título de una plantilla.
		explicito := st.fillType == "solid" || st.strokeType == "solid"
		if !visible(b.Fill) && !visible(b.Stroke) || se.Name.Local == "custom-shape" && !explicito {
			b.Kind = "text"
		}
	}

	if b.W <= 0 {
		b.W = 20
	}
	if b.H <= 0 {
		b.H = 20
	}
	return b
}

// readODPNotes extrae el texto de <presentation:notes> (párrafos unidos por \n).
func (d *odfDec) readODPNotes() string {
	var lines []string
	for {
		tok, err := d.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			// Descender por frame/text-box hasta los <text:p> (sin saltar el
			// subárbol, o se perdería el texto de las notas).
			if t.Name.Local == "p" {
				runs := d.readRuns("p", xrun{})
				var b strings.Builder
				for _, r := range runs {
					if r.brk {
						b.WriteByte('\n')
					} else {
						b.WriteString(r.text)
					}
				}
				if s := strings.TrimSpace(b.String()); s != "" {
					lines = append(lines, s)
				}
			}
		case xml.EndElement:
			if t.Name.Local == "notes" {
				return strings.Join(lines, "\n")
			}
		}
	}
	return strings.Join(lines, "\n")
}

// odpPct convierte una longitud ODF ("3.048cm") a % del lienzo sobre la dimensión
// dada: la de la página de la diapositiva (ver tamanoPaginaODP).
func odpPct(length string, dimCm float64) float64 {
	cm := parseLenCm(length)
	if dimCm <= 0 {
		return 0
	}
	return roundPct(cm / dimCm * 100)
}

// parseLenCm parsea una longitud ODF a centímetros (cm/mm/in).
func parseLenCm(s string) float64 {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasSuffix(s, "cm"):
		return atof(strings.TrimSuffix(s, "cm"))
	case strings.HasSuffix(s, "mm"):
		return atof(strings.TrimSuffix(s, "mm")) / 10
	case strings.HasSuffix(s, "in"):
		return atof(strings.TrimSuffix(s, "in")) * 2.54
	}
	return atof(s)
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

// dataURLFromZip lee un medio embebido del paquete y lo devuelve como data:URL.
func dataURLFromZip(href string, files map[string][]byte) string {
	if href == "" {
		return ""
	}
	href = strings.TrimPrefix(href, "./")
	raw := files[href]
	if len(raw) == 0 {
		return ""
	}
	mime := mediaTypeOf(strings.ToLower(href))
	if !strings.HasPrefix(mime, "image/") {
		return ""
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
}

// colorHash antepone '#' a un color "RRGGBB" ya normalizado; "" si vacío.
func colorHash(c string) string {
	if c == "" {
		return ""
	}
	return "#" + c
}

// Lo que mide la página de la diapositiva que escribe el exportador (export_slides.go):
// la de un fichero sin página declarada.
const (
	anchoPaginaODP = 25.4
	altoPaginaODP  = 14.288
)

// tamanoPaginaODP es el ancho y el alto, en cm, de la página de la PRIMERA diapositiva:
// su `draw:master-page-name` lleva al `style:master-page`, este a su `style:page-layout`
// y ahí están `fo:page-width` y `fo:page-height`.
//
// Hace falta leerla: Impress escribe 28 × 15,75 (su 16:9) o 28 × 21 (4:3), y con el
// lienzo del exportador fijo las cajas de un .odp de LibreOffice se salían de la
// diapositiva —hasta un 113 % de alto—. Sin página que leer, la del exportador.
func tamanoPaginaODP(content, estilos []byte) (ancho, alto float64) {
	patron := primerAtributo(content, "page", "master-page-name")
	disenos := map[string][2]float64{}
	maestras := map[string]string{}
	for _, parte := range [][]byte{estilos, content} {
		leerPaginas(parte, disenos, maestras)
	}
	if d, ok := disenos[maestras[patron]]; ok && d[0] > 0 && d[1] > 0 {
		return d[0], d[1]
	}
	return anchoPaginaODP, altoPaginaODP
}

// primerAtributo es el atributo `attr` del primer elemento `elem` de la parte.
func primerAtributo(parte []byte, elem, attr string) string {
	dec := xml.NewDecoder(bytes.NewReader(parte))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == elem {
			return attrLocal(se, attr)
		}
	}
}

// leerPaginas apunta los `style:page-layout` (nombre → ancho y alto en cm) y los
// `style:master-page` (nombre → su page-layout) de una parte del paquete.
func leerPaginas(parte []byte, disenos map[string][2]float64, maestras map[string]string) {
	dec := xml.NewDecoder(bytes.NewReader(parte))
	diseno := ""
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "page-layout":
			diseno = attrLocal(se, "name")
		case "page-layout-properties":
			if diseno != "" {
				disenos[diseno] = [2]float64{parseLenCm(attrLocal(se, "page-width")), parseLenCm(attrLocal(se, "page-height"))}
			}
		case "master-page":
			maestras[attrLocal(se, "name")] = attrLocal(se, "page-layout-name")
		}
	}
}

// tamanoDiapositiva es el `size` del modelo de la web para una página de ese ancho y
// alto: nada si es 16:9 (el de por defecto), el preset 4:3, o uno propio en cm.
func tamanoDiapositiva(ancho, alto float64) *outSize {
	p := ancho / alto
	switch {
	case math.Abs(p-16.0/9) < 0.01:
		return nil
	case math.Abs(p-4.0/3) < 0.01:
		return &outSize{Preset: "4:3"}
	}
	return &outSize{Preset: "custom", W: roundPct(ancho), H: roundPct(alto), Unit: "cm"}
}

// attrSytue es el atributo `local` del espacio de nombres de Sytue (odpNSSytue); "" si no está.
func attrSytue(se xml.StartElement, local string) string {
	for _, a := range se.Attr {
		if a.Name.Space == odpNSSytue && a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// readODPLayouts lee los diseños de styles.xml (`style:presentation-page-layout`) y los pone
// en el patrón del modelo; devuelve el id de cada uno por el nombre de su estilo. Los que
// escribe el exportador llevan el id y el nombre propios. Los esquemas de página de
// LibreOffice (AL1T0, AL2T1…) no los llevan: se mapean a un diseño de Sytue con su
// geometría y un nombre por su composición (odpDisenoLibreOffice).
func readODPLayouts(styles []byte, ancho, alto float64, model *slidesModel) map[string]string {
	dec := xml.NewDecoder(bytes.NewReader(styles))
	l := &odpLectorDisenos{ids: map[string]string{}, ancho: ancho, alto: alto}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			l.inicio(t)
		case xml.EndElement:
			if t.Name.Local == "presentation-page-layout" {
				l.fin()
			}
		}
	}
	if len(l.out) > 0 {
		if model.Master == nil {
			model.Master = &slideMaster{Boxes: []slideBox{}}
		}
		model.Master.Layouts = disenosValidos(l.out)
	}
	return l.ids
}

// odpLectorDisenos es el estado de la lectura de styles.xml: los diseños ya leídos, el
// propio (cur) o el esquema de LibreOffice (lo) que se está leyendo, y el id por estilo.
type odpLectorDisenos struct {
	ids         map[string]string
	out         []slideLayout
	cur         *slideLayout
	lo          *odpDisenoLO
	ancho, alto float64
}

// inicio atiende un elemento de apertura: el diseño nuevo o uno de sus marcadores.
func (l *odpLectorDisenos) inicio(t xml.StartElement) {
	switch t.Name.Local {
	case "presentation-page-layout":
		l.cur, l.lo = nil, nil
		if id := attrSytue(t, "id"); id != "" && len(l.out) < maxDisenos {
			l.out = append(l.out, slideLayout{ID: id, Name: attrSytue(t, "name"), Boxes: []slideBox{}})
			l.cur = &l.out[len(l.out)-1]
			l.ids[attrLocal(t, "name")] = id
		} else if id == "" {
			l.lo = &odpDisenoLO{estilo: attrLocal(t, "name")}
		}
	case "placeholder":
		if l.lo != nil {
			l.lo.marcador(t, l.ancho, l.alto)
			return
		}
		if l.cur == nil || len(l.cur.Boxes) >= maxCajasDiseno {
			return
		}
		html := attrSytue(t, "html")
		if html == "" && attrLocal(t, "object") == "title" {
			html = "<h1></h1>"
		}
		l.cur.Boxes = append(l.cur.Boxes, slideBox{
			ID: l.cur.ID + "b" + strconv.Itoa(len(l.cur.Boxes)+1), Kind: "text", HTML: html,
			X: odpPct(attrLocal(t, "x"), l.ancho), Y: odpPct(attrLocal(t, "y"), l.alto),
			W: odpPct(attrLocal(t, "width"), l.ancho), H: odpPct(attrLocal(t, "height"), l.alto),
		})
	}
}

// fin cierra el diseño en curso; el esquema de LibreOffice se vuelve un diseño de Sytue.
func (l *odpLectorDisenos) fin() {
	if d, ok := l.lo.diseno(l.out); ok && len(l.out) < maxDisenos {
		l.out = append(l.out, d)
		l.ids[l.lo.estilo] = d.ID
	}
	l.cur, l.lo = nil, nil
}

// odpDisenoLO es un esquema de página de LibreOffice leído: sus marcadores de contenido.
type odpDisenoLO struct {
	estilo string
	cajas  []slideBox
	tipos  []string
}

// marcador apunta un marcador del esquema. Solo los de la diapositiva: el título, el
// subtítulo y los de contenido; las notas, el folleto y el pie no son cajas de ella.
func (d *odpDisenoLO) marcador(se xml.StartElement, ancho, alto float64) {
	obj := attrLocal(se, "object")
	html := "<p>Haz clic para añadir texto</p>"
	switch obj {
	case "title":
		html = "<h1>Haz clic para añadir título</h1>"
	case "subtitle", "outline", "text", "object", "graphic", "chart", "table", "orgchart":
	default:
		return
	}
	w, h := odpPct(attrLocal(se, "width"), ancho), odpPct(attrLocal(se, "height"), alto)
	if w <= 0 || h <= 0 || len(d.cajas) >= maxCajasDiseno {
		return
	}
	d.tipos = append(d.tipos, obj)
	d.cajas = append(d.cajas, slideBox{
		Kind: "text", HTML: html, W: w, H: h,
		X: odpPct(attrLocal(se, "x"), ancho), Y: odpPct(attrLocal(se, "y"), alto),
	})
}

var reNoIDDiseno = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// diseno es el esquema como diseño de Sytue: id `lo-<estilo>` y el nombre de su composición
// (el mismo idioma que los diseños de serie del editor). false si no trae ningún marcador.
func (d *odpDisenoLO) diseno(ya []slideLayout) (slideLayout, bool) {
	if d == nil || len(d.cajas) == 0 {
		return slideLayout{}, false
	}
	id := "lo-" + reNoIDDiseno.ReplaceAllString(d.estilo, "")
	if len(id) > 40 {
		id = id[:40]
	}
	titulo, subs, contenidos := false, 0, 0
	for _, t := range d.tipos {
		switch t {
		case "title":
			titulo = true
		case "subtitle":
			subs++
		default:
			contenidos++
		}
	}
	name := nombreDisenoLO(titulo, subs, contenidos)
	for n := 2; ; n++ {
		repetido := false
		for _, l := range ya {
			repetido = repetido || l.Name == name
		}
		if !repetido {
			break
		}
		name = nombreDisenoLO(titulo, subs, contenidos) + " " + strconv.Itoa(n)
	}
	for i := range d.cajas {
		d.cajas[i].ID = id + "b" + strconv.Itoa(i+1)
	}
	return slideLayout{ID: id, Name: name, Boxes: d.cajas}, true
}

// nombreDisenoLO nombra un esquema por su composición, con los nombres de los diseños de
// serie del editor («Título», «Título y contenido», «Dos columnas»).
func nombreDisenoLO(titulo bool, subs, contenidos int) string {
	switch {
	case titulo && subs == 0 && contenidos == 0:
		return "Título"
	case titulo && subs == 1 && contenidos == 0:
		return "Título y subtítulo"
	case titulo && subs == 0 && contenidos == 1:
		return "Título y contenido"
	case titulo && subs == 0 && contenidos == 2:
		return "Dos columnas"
	case titulo:
		return "Título y " + strconv.Itoa(subs+contenidos) + " marcadores"
	case subs+contenidos == 1:
		return "Contenido"
	}
	return strconv.Itoa(subs+contenidos) + " marcadores"
}

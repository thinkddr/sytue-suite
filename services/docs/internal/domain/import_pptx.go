// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"cmp"
	"encoding/base64"
	"encoding/xml"
	"math"
	"path"
	"strconv"
	"strings"
)

// Importador OOXML PresentationML (.pptx) -> SlidesModel -> doc:slides. Reverso
// de export_slides.go: lee ppt/presentation.xml (orden y tamaño), el patrón (sus
// cajas, con los campos de número de diapositiva) y su tema (que se reconoce como uno
// de los del editor por su fondo, su tinta y su fuente), y cada ppt/slides/slideN.xml:
// fondo propio, cajas de texto con formato, imágenes, formas, vídeo y audio,
// conectores, transición, animaciones (pptx_anim.go) y notas del ponente. Posiciones
// EMU -> % del lienzo. Clean-room sobre ECMA-376 (PresentationML/DrawingML) con
// encoding/xml; el texto de cada caja se reconvierte a HTML con blocksToHTML.

// importPptx convierte los bytes de un .pptx en (KindPresentation, JSON SlidesModel).
func importPptx(data []byte, p *presupuesto) (Kind, string, error) {
	files, err := readZip(data)
	if err != nil {
		return "", "", err
	}
	if len(files["ppt/presentation.xml"]) == 0 {
		return "", "", wrapFormat("pptx: falta ppt/presentation.xml")
	}
	var pres struct {
		SldSz struct {
			CX string `xml:"cx,attr"`
			CY string `xml:"cy,attr"`
		} `xml:"sldSz"`
		Masters []struct {
			RID string `xml:"id,attr"`
		} `xml:"sldMasterIdLst>sldMasterId"`
		SldIDs []struct {
			RID string `xml:"id,attr"`
		} `xml:"sldIdLst>sldId"`
		Defecto xnode `xml:"defaultTextStyle"`
	}
	_ = xml.Unmarshal(files["ppt/presentation.xml"], &pres)

	wEMU := float64(atoiDefault(pres.SldSz.CX, slideWEMU))
	hEMU := float64(atoiDefault(pres.SldSz.CY, slideHEMU))
	if wEMU <= 0 {
		wEMU = slideWEMU
	}
	if hEMU <= 0 {
		hEMU = slideHEMU
	}
	presRels := parseRels(files["ppt/_rels/presentation.xml.rels"])

	model := &slidesModel{Size: tamanoDiapositiva(wEMU/360000, hEMU/360000)}
	pt := &pptxTheme{scheme: map[string]string{}, tablas: estilosDeTabla(files["ppt/tableStyles.xml"]), presupuesto: p, defecto: nivelesDe(&pres.Defecto)}
	layouts := map[string]phGeoms{}
	var layoutIDs map[string]string // ruta de la parte de diseño → id del diseño del modelo
	fondoPatron := ""
	patrones := map[string]*patronPptx{} // los patrones ya leídos, por su parte
	patronActual, primero := "", ""
	if len(pres.Masters) > 0 {
		if target := presRels[pres.Masters[0].RID]; target != "" {
			masterPath := path.Clean("ppt/" + target)
			if pa := readPptxMaster(masterPath, files, wEMU, hEMU, pt, model); pa != nil {
				patrones[masterPath], patronActual, primero, fondoPatron = pa, masterPath, masterPath, pa.fondo
			}
			var found []slideLayout
			found, layoutIDs = readPptxLayouts(masterPath, files, wEMU, hEMU, pt)
			model.ponerDisenos(found)
		}
	}
	for i, s := range pres.SldIDs {
		if p.agotado() {
			break // ya no cabe: importar devuelve ErrDemasiadoGrande
		}
		target := presRels[s.RID]
		if target == "" {
			target = "slides/slide" + strconv.Itoa(i+1) + ".xml"
		}
		slidePath := path.Clean("ppt/" + target)
		raw := files[slidePath]
		if len(raw) == 0 {
			continue
		}
		rels := parseRels(files[relsPath(slidePath)])
		pt.layout = layoutGeoms(slidePath, rels, files, layouts)
		// El fondo y el mapa de colores de su diseño, que la diapositiva hereda si no trae los suyos.
		diseno := disenoDe(slidePath, rels)
		// Su patrón, que no tiene por qué ser el primero: sus colores, sus marcadores y su fondo.
		patronActual = pt.usarPatronDe(diseno, files, patrones, patronActual)
		pt.mapa = mapaDeColores(files[diseno])
		otro := patrones[patronActual] // el patrón de la diapositiva, si no es el del modelo
		if patronActual == primero {
			otro = nil
		}
		// Su diseño, si es de otro patrón, entra en los del modelo con las formas de ese patrón.
		tapa := model.disenoDeOtroPatron(layoutIDs, diseno, patronActual, otro, files, wEMU, hEMU, pt)
		fondoDiseno, imagenDiseno := fondoQueHereda(diseno, files, pt, otro)
		pt.fondo = fondoDiseno
		sl := parseSlide("s"+strconv.Itoa(i+1), raw, path.Dir(slidePath), rels, files, wEMU, hEMU, pt, false)
		pt.mapa, pt.fondo = nil, ""
		heredarFondo(&sl, fondoDiseno, imagenDiseno, fondoPatron)
		// El patrón del modelo es el primero: sus cajas no son las de una diapositiva de otro,
		// que lo tapa ella si su diseño no lo tapa.
		sl.HideMaster = sl.HideMaster || tapa
		// Notas del ponente: parte notesSlide referenciada en las relaciones; y su diseño.
		for _, target := range rels {
			switch {
			case strings.Contains(target, "notesSlide"):
				sl.Notes = extractNotes(files[path.Clean(path.Dir(slidePath)+"/"+target)])
			case strings.Contains(target, "slideLayout"):
				sl.Layout = layoutIDs[path.Clean(path.Dir(slidePath)+"/"+target)]
			}
		}
		model.Slides = append(model.Slides, sl)
	}
	if len(model.Slides) == 0 {
		model.Slides = []slideT{{ID: "s1", Boxes: []slideBox{}}}
	}
	model.quitarFondosDelTema()
	return KindPresentation, marshalSlidesModel(model), nil
}

// pptxTheme es lo que una diapositiva necesita del patrón para leerse: los colores del
// esquema ya pasados por el mapa del patrón (bg1, tx1, accent1…), para resolver los
// `schemeClr`.
type pptxTheme struct {
	scheme map[string]string
	// presupuesto corta la lectura de las diapositivas en cuanto no caben (import_presupuesto.go).
	presupuesto *presupuesto
	// rellenos, fondos y lineas son las listas de estilos del tema (`a:fmtScheme`): el XML de
	// cada relleno, fondo y trazo, al que apuntan `fillRef`, `bgRef` y `lnRef` por su índice.
	rellenos, fondos, lineas [][]byte
	// tablas son los estilos de tabla de la presentación (ppt/tableStyles.xml) por su id.
	tablas map[string]estiloTabla
	// enSmartArt: se está leyendo el dibujo de un SmartArt, que no puede llevar otro dentro
	// (un fichero hostil que se cite a sí mismo no da vueltas sin fin).
	enSmartArt bool
	// fondo y mapa son, de la parte que se lee, el fondo que hereda («RRGGBB») y su mapa de
	// colores (`overrideClrMapping`, de la diapositiva o de su diseño).
	fondo string
	mapa  map[string]string
	// soloMarcadores: se está leyendo un diseño, del que solo cuentan los marcadores de
	// posición de contenido (readPptxLayouts). extremosSueltos: los conectores no se anclan
	// a cajas (las formas de un diseño, que no son de ninguna diapositiva).
	soloMarcadores, extremosSueltos bool
	// master y layout son la geometría de los marcadores de posición del patrón y del
	// diseño de la diapositiva en curso: un marcador sin `a:xfrm` hereda la suya.
	master, layout phGeoms
	// tintaTema es el color del texto del tema del modelo (tx1 del primer patrón): el que el
	// editor pinta en el texto que no dice el suyo.
	tintaTema string
	// txStyles son las listas de estilos de texto del patrón por clase (`titleStyle`,
	// `bodyStyle`, `otherStyle`) y defecto la de la presentación (`p:defaultTextStyle`): lo
	// último que hereda el texto (pptx_marcadores.go).
	txStyles map[string]nivelesTexto
	defecto  nivelesTexto
}

// patronPptx es lo que una diapositiva necesita de su patrón: los colores de su tema con el mapa
// del patrón, las listas de estilos del tema, la geometría y las listas de texto de sus
// marcadores y su fondo. Una presentación puede tener varios patrones, y cada diapositiva usa
// el de su diseño: el primero es además el tema y el patrón del modelo.
type patronPptx struct {
	scheme                   map[string]string
	rellenos, fondos, lineas [][]byte
	geoms                    phGeoms
	txStyles                 map[string]nivelesTexto
	fuente, nombre           string // la fuente del cuerpo y el nombre del tema
	fondo, imagen            string // el fondo del patrón: su color y, si es una imagen, su data URL
}

// leerPatronPptx lee un patrón: su tema (colores, fuente, listas de estilos) y, del patrón, el
// mapa de colores, los marcadores y las listas de texto. El fondo lo pone quien lo usa, que
// necesita sus colores para leerlo (fondoDeParte).
func leerPatronPptx(masterPath string, files map[string][]byte) *patronPptx {
	raw := files[masterPath]
	pa := &patronPptx{scheme: map[string]string{}, geoms: readPhGeoms(raw), txStyles: leerTxStyles(raw)}
	for _, target := range parseRels(files[relsPath(masterPath)]) {
		if strings.Contains(target, "theme") {
			tema := files[path.Clean(path.Dir(masterPath)+"/"+target)]
			pa.fuente, pa.nombre = readThemeColors(tema, pa.scheme)
			pa.rellenos, pa.fondos, pa.lineas = listasDeEstilos(tema)
			break
		}
	}
	var clrMap struct {
		Attrs []xml.Attr `xml:",any,attr"`
	}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "clrMap" {
			_ = dec.DecodeElement(&clrMap, &se)
			break
		}
	}
	mapped := map[string]string{}
	for _, a := range clrMap.Attrs {
		if c := pa.scheme[a.Value]; c != "" {
			mapped[a.Name.Local] = c
		}
	}
	if len(mapped) == 0 { // sin clrMap: el de por defecto
		mapped["bg1"], mapped["tx1"], mapped["bg2"], mapped["tx2"] = pa.scheme["lt1"], pa.scheme["dk1"], pa.scheme["lt2"], pa.scheme["dk2"]
	}
	for k, v := range mapped {
		pa.scheme[k] = v
	}
	return pa
}

// usar pone el patrón como el de las partes que se leen a continuación.
func (t *pptxTheme) usar(pa *patronPptx) {
	t.scheme, t.rellenos, t.fondos, t.lineas = pa.scheme, pa.rellenos, pa.fondos, pa.lineas
	t.master, t.txStyles = pa.geoms, pa.txStyles
}

// usarPatronDe pone como patrón el del diseño, si no es el que ya se usa (`actual`), leyéndolo
// la primera vez (`leidos`). Devuelve el que queda en uso.
func (t *pptxTheme) usarPatronDe(diseno string, files map[string][]byte, leidos map[string]*patronPptx, actual string) string {
	mp := patronDe(diseno, files)
	if mp == "" || mp == actual || len(files[mp]) == 0 {
		return actual
	}
	pa := leidos[mp]
	if pa == nil {
		pa = leerPatronPptx(mp, files)
		t.usar(pa)
		pa.fondo, pa.imagen = fondoDeParte(mp, files, t)
		leidos[mp] = pa
	}
	t.usar(pa)
	return mp
}

// patronDe es la parte del patrón de un diseño; "" si no lo dice.
func patronDe(diseno string, files map[string][]byte) string {
	for _, target := range parseRels(files[relsPath(diseno)]) {
		if strings.Contains(target, "slideMaster") {
			return path.Clean(path.Dir(diseno) + "/" + target)
		}
	}
	return ""
}

// readPptxMaster lee el tema y el patrón: el esquema de colores (con el mapa del
// patrón), el fondo, la fuente, y las cajas que no son marcadores de posición —las que
// se ven en todas las diapositivas—. Con ellos reconoce el tema del editor o, si no es
// ninguno, lo guarda como tema propio.
func readPptxMaster(masterPath string, files map[string][]byte, wEMU, hEMU float64, pt *pptxTheme, model *slidesModel) *patronPptx {
	raw := files[masterPath]
	if len(raw) == 0 {
		return nil
	}
	pa := leerPatronPptx(masterPath, files)
	pt.usar(pa)
	pa.fondo, pa.imagen = fondoDeParte(masterPath, files, pt)
	pt.tintaTema = pt.scheme["tx1"]
	rels := parseRels(files[relsPath(masterPath)])
	font, themeName := pa.fuente, pa.nombre
	m := parseSlide("m", raw, path.Dir(masterPath), rels, files, wEMU, hEMU, pt, true)
	bg := strings.TrimPrefix(m.Bg, "#")
	if bg == "" {
		bg = pt.scheme["bg1"]
	}
	model.temaAjeno(bg, pt.scheme["tx1"], font, themeName)
	if len(m.Boxes) > 0 {
		model.Master = &slideMaster{Boxes: m.Boxes}
	}
	return pa
}

// readThemeColors apunta los colores del esquema del tema (dk1, lt1, accent1…) y
// devuelve la fuente del cuerpo y el nombre del tema.
func readThemeColors(raw []byte, scheme map[string]string) (font, name string) {
	var th struct {
		Name string `xml:"name,attr"`
		Clr  struct {
			Colors []struct {
				XMLName xml.Name
				Srgb    struct {
					Val string `xml:"val,attr"`
				} `xml:"srgbClr"`
				Sys struct {
					Last string `xml:"lastClr,attr"`
				} `xml:"sysClr"`
			} `xml:",any"`
		} `xml:"themeElements>clrScheme"`
		Minor struct {
			Latin struct {
				Typeface string `xml:"typeface,attr"`
			} `xml:"latin"`
		} `xml:"themeElements>fontScheme>minorFont"`
	}
	if xml.Unmarshal(raw, &th) != nil {
		return "", ""
	}
	for _, c := range th.Clr.Colors {
		v := c.Srgb.Val
		if v == "" {
			v = c.Sys.Last
		}
		if v = normalizeHexColor(v); v != "" {
			scheme[c.XMLName.Local] = v
		}
	}
	return th.Minor.Latin.Typeface, th.Name
}

// relsPath deriva la ruta del fichero de relaciones de una parte OPC:
// "ppt/slides/slide1.xml" -> "ppt/slides/_rels/slide1.xml.rels".
func relsPath(part string) string {
	dir, file := path.Split(part)
	return path.Clean(dir + "_rels/" + file + ".rels")
}

// parseSlide convierte el árbol de una diapositiva (o del patrón, con `master`) en
// slideT: fondo, cajas, conectores, transición y animaciones. `dir` es la carpeta de
// la parte, contra la que se resuelven sus relaciones. Del patrón no se toman los
// marcadores de posición: son plantillas, no se ven en las diapositivas.
func parseSlide(id string, raw []byte, dir string, rels map[string]string, files map[string][]byte, wEMU, hEMU float64, pt *pptxTheme, master bool) slideT {
	sl := slideT{ID: id, Boxes: []slideBox{}}
	dec := pt.presupuesto.decodificador(raw)
	p := &pptxDec{dec: dec, rels: rels, files: files, dir: dir, wEMU: wEMU, hEMU: hEMU, theme: pt, master: master, idioma: idiomaDeTexto(raw)}
	p.colores(raw)
	fondoImg := ""
	spidBox := map[string]int{}
	type cxnRaw struct {
		c        slideConnector
		from, to struct{ spid, idx string }
		a, b     cpt
	}
	var cxns []cxnRaw
	var grupos []*grupoDML // los grupos abiertos, del de fuera al de dentro
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if p.grupos(tok, &grupos) {
			continue
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		var (
			box  slideBox
			spid string
			got  bool
		)
		switch se.Name.Local {
		case "sld", "sldLayout":
			sl.HideMaster = attrLocal(se, "showMasterSp") == "0"
			continue
		case "bg":
			fondoImg = p.fondoPropio(&sl)
			continue
		case "graphicFrame":
			cajas, id := p.readGraphicFrame()
			sl.ponerTodas(cajas, id, grupos, spidBox, wEMU, hEMU)
			continue
		case "sp":
			box, spid, got = p.readSp()
		case "pic":
			box, spid, got = p.readPic()
		case "cxnSp":
			c, from, to, a, b := p.readCxnSp()
			if !pt.soloMarcadores {
				a, b = enGrupos(grupos, a), enGrupos(grupos, b)
				cxns = append(cxns, cxnRaw{c: c, from: from, to: to, a: a, b: b})
			}
			continue
		case "transition":
			sl.Transition = cmp.Or(morphSiYa(sl.Transition), p.readTransition())
			continue
		case "timing":
			var t xnode
			if dec.DecodeElement(&t, &se) == nil {
				readTiming(&t, spidBox, sl.Boxes)
			}
			continue
		default:
			continue
		}
		if got {
			sl.poner(box, spid, grupos, spidBox, wEMU, hEMU)
		}
	}
	if sl.Transition == "none" {
		sl.Transition = ""
	}
	p.pistasSueltas(&sl)
	// Los conectores, al final: sus extremos apuntan a cajas por su id de forma.
	tol := math.Max(wEMU, hEMU) / 100 // un 1 %: lo que redondean los % del modelo
	end := func(ref struct{ spid, idx string }, p cpt) connectorEnd {
		if i, ok := spidBox[ref.spid]; ok && !pt.extremosSueltos { // "" no es clave: poner no la guarda
			b := sl.Boxes[i]
			r := crect{b.X / 100 * wEMU, b.Y / 100 * hEMU, b.W / 100 * wEMU, b.H / 100 * hEMU}
			return connectorEnd{BoxID: b.ID, Anchor: cxnAnchor(b, r, ref.idx, p, tol)}
		}
		x := math.Min(100, math.Max(0, pctOf(p.x, wEMU)))
		y := math.Min(100, math.Max(0, pctOf(p.y, hEMU)))
		return connectorEnd{X: &x, Y: &y}
	}
	for i, c := range cxns {
		c.c.ID = sl.ID + "c" + strconv.Itoa(i+1)
		c.c.From, c.c.To = end(c.from, c.a), end(c.to, c.b)
		sl.Connectors = append(sl.Connectors, c.c)
	}
	// Una imagen de fondo: el modelo solo tiene fondo de color, así que va como imagen a
	// sangre debajo de todo (al final: las animaciones y los conectores ya tienen sus cajas).
	ponerImagenDeFondo(&sl, fondoImg)
	return sl
}

// poner añade una caja a la diapositiva, en su sitio fuera de los grupos que la contienen y con
// su id; `spid` es su id de forma, el que citan animaciones y conectores.
func (sl *slideT) poner(box slideBox, spid string, grupos []*grupoDML, spidBox map[string]int, wEMU, hEMU float64) {
	box.X, box.Y, box.W, box.H = cajaEnGrupos(grupos, box, wEMU, hEMU)
	box.ID = sl.ID + "b" + strconv.Itoa(len(sl.Boxes)+1)
	if spid != "" {
		spidBox[spid] = len(sl.Boxes)
	}
	sl.Boxes = append(sl.Boxes, box)
}

// ponerTodas añade las cajas de un mismo marco (las formas de un SmartArt comparten su id).
func (sl *slideT) ponerTodas(cajas []slideBox, spid string, grupos []*grupoDML, spidBox map[string]int, wEMU, hEMU float64) {
	for _, b := range cajas {
		sl.poner(b, spid, grupos, spidBox, wEMU, hEMU)
	}
}

// ponerImagenDeFondo pone debajo de todo la imagen de fondo de la diapositiva, si tiene.
func ponerImagenDeFondo(sl *slideT, src string) {
	if src != "" {
		sl.Boxes = append([]slideBox{imagenDeFondo(sl.ID+"bg", src)}, sl.Boxes...)
	}
}

// disenoDe es la parte del diseño de una diapositiva; "" si no tiene.
func disenoDe(slidePath string, rels map[string]string) string {
	for _, target := range rels {
		if strings.Contains(target, "slideLayout") {
			return path.Clean(path.Dir(slidePath) + "/" + target)
		}
	}
	return ""
}

// ponerDisenos pone en el patrón del modelo los diseños leídos, si hay alguno.
func (m *slidesModel) ponerDisenos(found []slideLayout) {
	if len(found) == 0 {
		return
	}
	if m.Master == nil {
		m.Master = &slideMaster{Boxes: []slideBox{}}
	}
	m.Master.Layouts = found
}

// fondoQueHereda es el fondo del diseño de una diapositiva o, si no dice ninguno y la
// diapositiva es de otro patrón que el del modelo (`otro`), el de ese patrón. El del primero
// ya está: su color es el del tema y su imagen va en las cajas del patrón del modelo.
func fondoQueHereda(diseno string, files map[string][]byte, pt *pptxTheme, otro *patronPptx) (color, imagen string) {
	color, imagen = fondoDeParte(diseno, files, pt)
	if color == "" && imagen == "" && otro != nil {
		return otro.fondo, otro.imagen
	}
	return color, imagen
}

// heredarFondo da a una diapositiva sin fondo propio el de su diseño: su imagen, o su color si
// no es el del patrón (ese ya es el del tema de la presentación).
func heredarFondo(sl *slideT, color, imagen, fondoPatron string) {
	if sl.Bg != "" || len(sl.Boxes) > 0 && sl.Boxes[0].ID == sl.ID+"bg" {
		return
	}
	switch {
	case imagen != "":
		ponerImagenDeFondo(sl, imagen)
	case color != "" && color != fondoPatron:
		sl.Bg = "#" + color
	}
}

// fondoDeParte es el fondo que declara una parte de diseño o de patrón: su color («RRGGBB») y,
// si es una imagen, su data URL. Vacíos si no declara ninguno.
func fondoDeParte(parte string, files map[string][]byte, pt *pptxTheme) (color, imagen string) {
	raw := files[parte]
	if len(raw) == 0 {
		return "", ""
	}
	p := &pptxDec{dec: xml.NewDecoder(bytes.NewReader(raw)), rels: parseRels(files[relsPath(parte)]), files: files, dir: path.Dir(parte), theme: pt, mapa: mapaDeColores(raw)}
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return "", ""
		}
		if se, ok := tok.(xml.StartElement); ok {
			switch se.Name.Local {
			case "bg":
				c, img := p.readBg()
				if img != "" {
					return c, p.dataURLFor(img)
				}
				return c, ""
			case "spTree":
				return "", "" // el fondo va antes que las formas: no hay
			}
		}
	}
}

// imagenDeFondo es la caja de una imagen de fondo: a sangre, debajo de las demás.
func imagenDeFondo(id, src string) slideBox {
	return slideBox{ID: id, Kind: "image", Src: src, X: 0, Y: 0, W: 100, H: 100}
}

// mapaDeColores es el `a:overrideClrMapping` de una diapositiva o de un diseño (bg1="dk1"…);
// nil si sigue el del patrón.
func mapaDeColores(raw []byte) map[string]string {
	i := bytes.Index(raw, []byte("overrideClrMapping"))
	if i < 0 {
		return nil
	}
	j := bytes.LastIndexByte(raw[:i], '<')
	if j < 0 {
		return nil
	}
	dec := xml.NewDecoder(bytes.NewReader(raw[j:]))
	tok, err := dec.RawToken()
	se, ok := tok.(xml.StartElement)
	if err != nil || !ok {
		return nil
	}
	m := map[string]string{}
	for _, a := range se.Attr {
		m[a.Name.Local] = a.Value
	}
	return m
}

type pptxDec struct {
	dec        *xml.Decoder
	rels       map[string]string
	files      map[string][]byte
	dir        string
	wEMU, hEMU float64
	theme      *pptxTheme
	master     bool
	idioma     string // idioma del texto de la diapositiva (subtítulos sin idioma)
	// tintaForma es el color de letra del estilo de la forma que se lee (`fontRef`).
	tintaForma string
	vtt        map[string]bool // relaciones de subtítulos ya reclamadas por un medio
	// mapa es el `overrideClrMapping` de la diapositiva o de su diseño (bg1 → dk1…).
	mapa map[string]string
	// fondo es el fondo de la diapositiva en curso: sobre él se mezcla un relleno translúcido.
	fondo rgbDML
	// tinta es el color del texto que no dice el suyo cuando el mapa de colores cambia el de
	// la presentación (una diapositiva oscura en un tema claro); "" = el del tema.
	tinta string
	// propia y cadena son las listas de estilos del texto de la forma que se lee: la suya
	// (`a:lstStyle`) y las que hereda (pptx_marcadores.go). Vacías fuera de una forma.
	propia nivelesTexto
	cadena cadenaTexto
	// escalaFuente es el `fontScale` del ajuste automático del cuerpo que se lee
	// (`a:normAutofit`, en milésimas de por ciento): lo que PowerPoint encogió su texto para que
	// cupiera. 0 = sin encoger.
	escalaFuente int
}

// colores prepara los colores de la parte que se lee: su mapa (el suyo o el de su diseño), el
// fondo sobre el que se mezcla lo translúcido y, si el mapa cambia la tinta (tx1 → lt1, la
// diapositiva oscura de un tema claro), el color del texto que no dice el suyo: el editor
// pintaría la tinta del tema.
func (p *pptxDec) colores(raw []byte) {
	p.mapa = p.theme.mapa
	if m := mapaDeColores(raw); m != nil {
		p.mapa = m
	}
	p.fondo = rgbDML{1, 1, 1, 1}
	if c, ok := rgbDeHex(p.theme.fondo); ok {
		p.fondo = c
	} else if c, ok := rgbDeHex(p.esquema("bg1")); ok {
		p.fondo = c
	}
	if t := p.esquema("tx1"); t != "" && t != p.theme.tintaTema {
		p.tinta = t // otro mapa de colores, u otro patrón con otro tema
	}
}

// fondoPropio lee el `p:bg` de la parte: su color va a la diapositiva (y es el fondo sobre el
// que se mezcla lo translúcido) y su imagen, si la tiene, se devuelve como data URL.
func (p *pptxDec) fondoPropio(sl *slideT) string {
	c, img := p.readBg()
	sl.Bg = hexColor(c)
	if x, ok := rgbDeHex(c); ok {
		p.fondo = x
	}
	if p.theme.soloMarcadores {
		return "" // de un diseño solo cuentan sus marcadores
	}
	return p.dataURLFor(img)
}

// esquema resuelve un color del esquema (bg1, tx1, accent1, dk1…) a «RRGGBB» con el mapa de
// colores de la diapositiva.
func (p *pptxDec) esquema(n string) string {
	if p.theme == nil {
		return ""
	}
	if m := p.mapa[n]; m != "" {
		n = m
	}
	return p.theme.scheme[n]
}

// hexRelleno es el color de un relleno para el modelo («RRGGBB»), con su transparencia ya
// mezclada sobre el fondo de la diapositiva; "" si no tiene color.
func (p *pptxDec) hexRelleno(r pinturaDML) string {
	if !r.hay {
		return ""
	}
	return r.color.sobre(p.fondo).hex()
}

// rellenoDeRef es el relleno al que apunta una referencia a las listas de estilos del tema
// (`fillRef`, `bgRef`, `lnRef`), con el color de la referencia en el lugar de phClr. `idx`
// por encima de 1000 es de la lista de fondos (ECMA-376 §20.1.4.2.1).
func (p *pptxDec) rellenoDeRef(lista [][]byte, idx int, ph rgbDML) (pinturaDML, bool) {
	if idx > 1000 && p.theme != nil {
		lista, idx = p.theme.fondos, idx-1000
	}
	return rellenoDeEstilo(lista, idx, ph, p.esquema)
}

// relleno lee un relleno de DrawingML ya abierto (solidFill, gradFill, pattFill, blipFill,
// noFill) hasta su cierre, con los colores del esquema de la diapositiva.
func (p *pptxDec) relleno(se xml.StartElement) pinturaDML {
	r := rellenoDML(p.dec, se, p.esquema)
	if r.hay && r.color.a < 0.01 {
		return pinturaDML{nada: true} // del todo transparente: no tiene relleno
	}
	return r
}

// readBg lee el fondo de una parte (`p:bg`): `p:bgPr` con su relleno, o `p:bgRef` con un
// relleno del tema y el color que ocupa el lugar de phClr (ECMA-376 §19.3.1.2). Devuelve el
// color («RRGGBB»; el medio si es un degradado, la mezcla si es una trama) y, si es una
// imagen, su relación.
func (p *pptxDec) readBg() (color, imagen string) {
	var r pinturaDML
	for fin := false; !fin; {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Local == "bgRef":
				ph, _ := colorHijo(p.dec, t, p.esquema)
				var ok bool
				if p.theme != nil {
					r, ok = p.rellenoDeRef(p.theme.rellenos, atoiDefault(attrLocal(t, "idx"), 0), ph)
				}
				if !ok {
					r = pinturaDML{color: ph, hay: true}
				}
			case esRellenoDML(t.Name.Local):
				r = p.relleno(t)
			}
		case xml.EndElement:
			fin = t.Name.Local == "bg"
		}
	}
	if r.hay {
		color = r.color.sobre(rgbDML{1, 1, 1, 1}).hex()
	}
	return color, r.imagen
}

// morphSiYa es «morph» si la diapositiva ya lo tiene (una transición que venga detrás no lo
// sustituye) y "" si no.
func morphSiYa(t string) string {
	if t == "morph" {
		return t
	}
	return ""
}

// readTransition lee una `p:transition`: el morph, un fundido, un desplazamiento (los
// que mueven la diapositiva) o, cualquier otro, un fundido —hay transición, y es la
// más discreta del editor—.
func (p *pptxDec) readTransition() string {
	t := "none"
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return t
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "morph":
				t = "morph"
			case "push", "cover", "pull", "wipe", "split", "strips", "pan", "reveal", "uncover":
				t = "slide"
			case "cut":
			default:
				t = "fade"
			}
			_ = p.dec.Skip()
		case xml.EndElement:
			if e.Name.Local == "transition" {
				return t
			}
		}
	}
}

// readCxnSp lee un `p:cxnSp`: tipo, color y puntas, y sus extremos (las formas a las
// que se ancla y los puntos de su recuadro).
//
//nolint:gocyclo // traductor de un formato a otro: un caso por elemento del conector
func (p *pptxDec) readCxnSp() (c slideConnector, from, to struct{ spid, idx string }, a, b cpt) {
	c.Kind = "straight"
	var off, ext cpt
	var rot float64
	var flipH, flipV bool
	var trazoEstilo pinturaDML
	trazoDicho := false
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "stCxn":
				from.spid, from.idx = attrLocal(t, "id"), attrLocal(t, "idx")
			case "endCxn":
				to.spid, to.idx = attrLocal(t, "id"), attrLocal(t, "idx")
			case "xfrm":
				rot = float64(atoiDefault(attrLocal(t, "rot"), 0))
				flipH, flipV = attrLocal(t, "flipH") == "1", attrLocal(t, "flipV") == "1"
				pos := p.readXfrm()
				off, ext = cpt{pos.x, pos.y}, cpt{pos.cx, pos.cy}
				continue
			case "prstGeom":
				switch prst := attrLocal(t, "prst"); {
				case strings.HasPrefix(prst, "bentConnector"):
					c.Kind = "elbow"
				case strings.HasPrefix(prst, "curvedConnector"):
					c.Kind = "curved"
				}
			case "ln":
				continue // descender: color y puntas
			case "solidFill", "gradFill", "pattFill":
				// La tinta del tema es el trazo por defecto del editor (currentColor): no se fija.
				if hx := p.hexRelleno(p.relleno(t)); hx != "" && hx != p.esquema("tx1") {
					c.Stroke = "#" + hx
				}
				trazoDicho = true
				continue
			case "noFill":
				trazoDicho = true
			case "style":
				_, trazoEstilo, _ = p.readStyle()
				continue
			case "headEnd":
				c.StartArrow = attrLocal(t, "type") != "" && attrLocal(t, "type") != "none"
			case "tailEnd":
				c.EndArrow = attrLocal(t, "type") != "" && attrLocal(t, "type") != "none"
			case "nvCxnSpPr", "cNvCxnSpPr", "spPr":
				continue
			}
			_ = p.dec.Skip()
		case xml.EndElement:
			if t.Name.Local == "cxnSp" {
				if hx := p.hexRelleno(trazoEstilo); !trazoDicho && hx != "" {
					c.Stroke = "#" + hx // el trazo de su estilo, si no dice el suyo
				}
				a, b = cxnEnds(off, ext, rot, flipH, flipV)
				return c, from, to, a, b
			}
		}
	}
	a, b = cxnEnds(off, ext, rot, flipH, flipV)
	return c, from, to, a, b
}

// readSp lee un <p:sp>: geometría, tipo (texto/forma), relleno/trazo y texto. Devuelve
// también su id de forma (lo que usan animaciones y conectores). En el patrón, un
// marcador de posición no cuenta.
func (p *pptxDec) readSp() (slideBox, string, bool) {
	var nv nvProps
	sp := spPrLeido{prst: "rect"}
	var boxHTML string
	// El relleno y el trazo de la forma, si los dice ella; si no, los de su estilo
	// (`p:style`), que es como pinta PowerPoint las formas que se insertan sin tocarlas.
	var rellenoEstilo, trazoEstilo pinturaDML
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "nvSpPr":
				nv = p.readNv("nvSpPr")
				p.cadena = p.theme.cadenaDe(nv)
			case "spPr":
				sp = p.readSpPr()
				if sp.prst == "" {
					sp.prst = "rect"
				}
			case "style":
				rellenoEstilo, trazoEstilo, p.tintaForma = p.readStyle()
			case "txBody":
				boxHTML = p.readTxBody()
			default:
				_ = p.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == "sp" {
				p.tintaForma, p.propia, p.cadena = "", nivelesTexto{}, nil
				return p.pintarSp(nv, sp, rellenoEstilo, trazoEstilo, boxHTML)
			}
		}
	}
	p.tintaForma, p.propia, p.cadena = "", nivelesTexto{}, nil
	if boxHTML != "" {
		return p.pintarSp(nv, sp, rellenoEstilo, trazoEstilo, boxHTML)
	}
	return slideBox{}, "", false
}

// pintarSp compone la caja de un <p:sp> con su relleno y su trazo, o los de su estilo si no
// dice los suyos. Una forma rellena de una imagen y sin texto es una imagen.
func (p *pptxDec) pintarSp(nv nvProps, sp spPrLeido, rellenoEstilo, trazoEstilo pinturaDML, boxHTML string) (slideBox, string, bool) {
	relleno, trazo := sp.relleno, sp.trazo
	if !sp.rellenoDicho {
		relleno = rellenoEstilo
	}
	if !sp.trazoDicho {
		trazo = trazoEstilo
	}
	stroke := p.hexRelleno(trazo)
	if trazo.nada {
		stroke = "transparent"
	}
	b, id, ok := p.closeSp(nv, sp.pos, sp.prst, p.hexRelleno(relleno), stroke, boxHTML, relleno.nada)
	if src := p.dataURLFor(relleno.imagen); ok && src != "" && strings.TrimSpace(stripHTML(boxHTML)) == "" {
		b = slideBox{X: b.X, Y: b.Y, W: b.W, H: b.H, Kind: "image", Src: src}
	}
	return b, id, ok
}

// readStyle lee `p:style`: las referencias de la forma a las listas de estilos del tema, cada
// una con el color que ocupa el lugar de phClr (ECMA-376 §20.1.4.2). De la letra solo cuenta
// el color: el del texto que no dice el suyo.
func (p *pptxDec) readStyle() (relleno, trazo pinturaDML, tinta string) {
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return relleno, trazo, tinta
		}
		switch t := tok.(type) {
		case xml.StartElement:
			idx := atoiDefault(attrLocal(t, "idx"), 0)
			switch t.Name.Local {
			case "fillRef", "lnRef":
				ph, ok := colorHijo(p.dec, t, p.esquema)
				if !ok || idx == 0 || p.theme == nil {
					continue
				}
				if t.Name.Local == "fillRef" {
					relleno, _ = p.rellenoDeRef(p.theme.rellenos, idx, ph)
				} else {
					trazo, _ = p.rellenoDeRef(p.theme.lineas, idx, ph)
				}
			case "fontRef":
				if c, ok := colorHijo(p.dec, t, p.esquema); ok {
					tinta = c.hex()
				}
			default:
				_ = p.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == "style" {
				return relleno, trazo, tinta
			}
		}
	}
}

// closeSp cierra la lectura de un <p:sp>: decide si cuenta (no, si está oculta, si es un
// marcador del patrón o algo que no es un marcador de contenido de un diseño), le da la
// geometría que hereda un marcador sin la suya y compone la caja.
func (p *pptxDec) closeSp(nv nvProps, pos xfrmPos, prst, fill, stroke, boxHTML string, noFill bool) (slideBox, string, bool) {
	if nv.oculta || p.master && nv.ph {
		return slideBox{}, "", false
	}
	if nv.ph && pos.cx == 0 && pos.cy == 0 {
		pos = p.theme.placeholder(nv.phType, nv.phIdx, pos)
	}
	if p.theme != nil && p.theme.soloMarcadores { // un diseño: solo sus marcadores de contenido
		if !nv.ph || nv.phType == "dt" || nv.phType == "ftr" || nv.phType == "sldNum" {
			return slideBox{}, "", false
		}
		boxHTML = muestraDeMarcador(nv.phType, boxHTML)
	}
	b := buildShapeBox(pos, nv.txBox, prst, fill, stroke, boxHTML, p.wEMU, p.hEMU)
	if b.Kind == "shape" && noFill && fill == "" {
		b.Fill = "transparent"
	}
	return b, nv.id, true
}

// buildShapeBox decide si el <p:sp> es una caja de texto o una forma y compone
// el slideBox correspondiente.
func buildShapeBox(pos xfrmPos, txBox bool, prst, fill, stroke, boxHTML string, wEMU, hEMU float64) slideBox {
	b := slideBox{
		X: pctOf(pos.x, wEMU), Y: pctOf(pos.y, hEMU),
		W: pctOf(pos.cx, wEMU), H: pctOf(pos.cy, hEMU),
		HTML: boxHTML,
	}
	shape := prstToShape(prst)
	// Una caja de texto con relleno o con borde es, para el editor, una forma con texto: su caja
	// de texto no pinta ni lo uno ni lo otro, y el relleno se perdía.
	pinta := fill != "" || (stroke != "" && stroke != "transparent")
	isShape := pinta || !txBox && shape != ""
	if isShape {
		b.Kind = "shape"
		b.Shape = shape
		if b.Shape == "" {
			b.Shape = "rect"
		}
		b.Fill = hexColor(fill)
		b.Stroke = hexColor(stroke)
		if stroke == "transparent" {
			b.Stroke = stroke
		}
	} else {
		b.Kind = "text"
	}
	if b.W <= 0 {
		b.W = 30
	}
	if b.H <= 0 {
		b.H = 10
	}
	return b
}

// readPic lee un <p:pic>: geometría e imagen embebida (a dataURL base64) o, si lleva
// un vídeo o un audio, la caja de medio con su fichero y su póster.
func (p *pptxDec) readPic() (slideBox, string, bool) {
	var pos xfrmPos
	var embed string
	var nv nvProps
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "nvPicPr":
				nv = p.readNv("nvPicPr")
			case "blip":
				embed = attrLocal(t, "embed")
				_ = p.dec.Skip()
			case "blipFill":
				// Descender (contiene <a:blip>); NO saltar el subárbol.
			case "spPr":
				pos = p.readSpPr().pos
			default:
				_ = p.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == "pic" {
				b, ok := p.picBox(pos, embed, nv)
				return b, nv.id, ok
			}
		}
	}
	return slideBox{}, "", false
}

// picBox compone la caja de un <p:pic>: un medio con su póster o una imagen.
func (p *pptxDec) picBox(pos xfrmPos, embed string, nv nvProps) (slideBox, bool) {
	if nv.oculta || p.master && nv.ph || p.theme != nil && p.theme.soloMarcadores {
		return slideBox{}, false
	}
	b := slideBox{
		X: pctOf(pos.x, p.wEMU), Y: pctOf(pos.y, p.hEMU),
		W: pctOf(pos.cx, p.wEMU), H: pctOf(pos.cy, p.hEMU),
	}
	if b.W <= 0 {
		b.W = 40
	}
	if b.H <= 0 {
		b.H = 30
	}
	poster := p.dataURLFor(embed)
	if nv.media != "" {
		if src := p.mediaSrc(nv); src != "" {
			b.Kind, b.MediaType, b.Src = "media", nv.media, src
			b.Tracks = p.readTracks(nv.tracks)
			if len(b.Tracks) == 0 {
				b.Tracks = p.pistasJuntoAlMedio(nv)
			}
			if poster != "" && !p.isReservePoster(embed) {
				b.Poster = poster
			}
			return b, true
		}
	}
	if poster == "" {
		return slideBox{}, false
	}
	b.Kind, b.Src = "image", poster
	return b, true
}

// Tope de un medio incrustado al importar: lo convertido se siembra con una operación
// de menos de 1 MiB (import_presupuesto.go), y un vídeo no se puede encoger como una
// foto. ponytail: por encima se queda su póster como imagen; el arreglo de verdad es
// subir el medio a Drive y enlazarlo.
const maxMedioImportado = 600_000

// mediaSrc es el origen de un medio: su fichero del paquete como data:URL (el
// incrustado de p14:media, o el del enlace si es interno) o su URL si es externa.
func (p *pptxDec) mediaSrc(nv nvProps) string {
	for _, rid := range []string{nv.mediaEmbed, nv.mediaLink} {
		target := p.rels[rid]
		if target == "" {
			continue
		}
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			return target
		}
		name := path.Clean(p.dir + "/" + target)
		raw := p.files[name]
		ext := strings.TrimPrefix(path.Ext(name), ".")
		mime := mediaExts[strings.ToLower(ext)]
		if len(raw) == 0 || mime == "" || base64.StdEncoding.EncodedLen(len(raw)) > maxMedioImportado {
			continue
		}
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
	}
	return ""
}

// isReservePoster dice si el póster es el liso que pone el exportador cuando el medio
// no traía el suyo (no es del usuario: no se importa).
func (p *pptxDec) isReservePoster(rID string) bool {
	return path.Base(p.rels[rID]) == "poster.png" && bytes.Equal(p.files[path.Clean(p.dir+"/"+p.rels[rID])], posterPNG())
}

// dataURLFor resuelve un r:embed a un data:URL base64 leyendo el medio del paquete.
func (p *pptxDec) dataURLFor(rID string) string {
	if rID == "" {
		return ""
	}
	target := p.rels[rID]
	if target == "" {
		return ""
	}
	return dataURLFromZip(path.Clean(p.dir+"/"+target), p.files)
}

// nvProps es lo que importa de las propiedades no visuales de una forma: su id, si es
// caja de texto o marcador de posición, y su medio (tipo y relaciones).
type nvProps struct {
	id                    string
	txBox, ph             bool
	oculta                bool // `hidden`: PowerPoint no la pinta ni al editar ni al presentar
	phType, phIdx         string
	media                 string // video, audio
	mediaLink, mediaEmbed string
	tracks                []pptxTrackRef // subtítulos del medio (pptx_subtitulos.go)
}

// readNv lee <p:nvSpPr>/<p:nvPicPr> hasta su cierre.
func (p *pptxDec) readNv(end string) nvProps {
	var nv nvProps
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return nv
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local != "track" {
				// Una pista de subtítulos citada por cualquier elemento del medio (la de
				// PowerPoint), sin idioma: lo pone readTracks.
				for _, a := range t.Attr {
					if (a.Name.Local == "id" || a.Name.Local == "embed" || a.Name.Local == "link") && esRelacionVTT(p.rels[a.Value]) {
						nv.tracks = append(nv.tracks, pptxTrackRef{rid: a.Value})
					}
				}
			}
			switch t.Name.Local {
			case "cNvPr":
				nv.id, nv.oculta = attrLocal(t, "id"), verdadero(attrLocal(t, "hidden"))
			case "cNvSpPr":
				nv.txBox = attrLocal(t, "txBox") == "1"
			case "ph":
				nv.ph, nv.phType, nv.phIdx = true, attrLocal(t, "type"), attrLocal(t, "idx")
			case "videoFile":
				nv.media, nv.mediaLink = "video", attrLocal(t, "link")
			case "audioFile":
				nv.media, nv.mediaLink = "audio", attrLocal(t, "link")
			case "media":
				nv.mediaEmbed = attrLocal(t, "embed")
			case "track":
				nv.tracks = append(nv.tracks, pptxTrackRef{
					lang: attrLocal(t, "lang"), label: attrLocal(t, "label"),
					def: attrLocal(t, "default") == "1", rid: attrLocal(t, "id"),
				})
			case "nvPr", "extLst", "ext":
				continue // descender
			}
			_ = p.dec.Skip()
		case xml.EndElement:
			if t.Name.Local == end {
				return nv
			}
		}
	}
}

// xfrmPos son las coordenadas EMU de un <a:xfrm> (offset y extensión).
type xfrmPos struct{ x, y, cx, cy float64 }

// spPrLeido es lo que cuenta de un <p:spPr>: geometría, forma, relleno y trazo, y si los dice
// (una forma que no los dice toma los de su estilo).
type spPrLeido struct {
	pos                      xfrmPos
	prst                     string
	relleno, trazo           pinturaDML
	rellenoDicho, trazoDicho bool
}

// readSpPr lee <p:spPr>. El relleno de nivel spPr es el de la forma; el de dentro de <a:ln>
// es el del trazo.
func (p *pptxDec) readSpPr() spPrLeido {
	var r spPrLeido
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return r
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Local == "xfrm":
				r.pos = p.readXfrm()
			case t.Name.Local == "prstGeom":
				r.prst = attrLocal(t, "prst")
				_ = p.dec.Skip()
			case esRellenoDML(t.Name.Local):
				r.relleno, r.rellenoDicho = p.relleno(t), true
			case t.Name.Local == "ln":
				r.trazo, r.trazoDicho = p.readLn()
			default:
				_ = p.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == "spPr" {
				return r
			}
		}
	}
}

// grupoDML es la transformación de un grupo (`p:grpSpPr/a:xfrm`, ECMA-376 §20.1.7.5): lleva
// el espacio de sus hijas (chOff, chExt) a su sitio (off, ext). Sin ella, lo agrupado y
// movido o escalado después de agruparlo salía donde estaba al agruparlo.
type grupoDML struct{ off, ext, chOff, chExt cpt }

func (g grupoDML) punto(c cpt) cpt {
	sx, sy := 1.0, 1.0
	if g.chExt.x != 0 && g.ext.x != 0 {
		sx = g.ext.x / g.chExt.x
	}
	if g.chExt.y != 0 && g.ext.y != 0 {
		sy = g.ext.y / g.chExt.y
	}
	return cpt{g.off.x + (c.x-g.chOff.x)*sx, g.off.y + (c.y-g.chOff.y)*sy}
}

// enGrupos lleva un punto (EMU) del espacio del grupo más interior al de la diapositiva.
func enGrupos(grupos []*grupoDML, c cpt) cpt {
	for i := len(grupos) - 1; i >= 0; i-- {
		c = grupos[i].punto(c)
	}
	return c
}

// cajaEnGrupos es la caja (en % del lienzo) llevada al espacio de la diapositiva.
func cajaEnGrupos(grupos []*grupoDML, b slideBox, wEMU, hEMU float64) (x, y, w, h float64) {
	if len(grupos) == 0 {
		return b.X, b.Y, b.W, b.H
	}
	a := enGrupos(grupos, cpt{b.X / 100 * wEMU, b.Y / 100 * hEMU})
	z := enGrupos(grupos, cpt{(b.X + b.W) / 100 * wEMU, (b.Y + b.H) / 100 * hEMU})
	return pctOf(a.x, wEMU), pctOf(a.y, hEMU), pctOf(z.x-a.x, wEMU), pctOf(z.y-a.y, hEMU)
}

// coordenada lee una coordenada de DrawingML (ST_Coordinate: entero CON signo; una forma que asoma
// por la izquierda tiene x negativa y salía pegada al borde); 0 si no es un número.
func coordenada(s string) float64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return float64(v)
}

// grupos lleva la pila de grupos abiertos: abre uno con su transformación y lo cierra al
// terminar. Dice si el elemento era de los grupos (y ya se ha leído).
func (p *pptxDec) grupos(tok xml.Token, pila *[]*grupoDML) bool {
	switch t := tok.(type) {
	case xml.EndElement:
		if t.Name.Local == "grpSp" && len(*pila) > 0 {
			*pila = (*pila)[:len(*pila)-1]
		}
		return true
	case xml.StartElement:
		switch t.Name.Local {
		case "grpSp":
			*pila = append(*pila, &grupoDML{})
			return true
		case "grpSpPr":
			if len(*pila) > 0 {
				*(*pila)[len(*pila)-1] = p.readGrpSpPr()
			}
			return true
		}
	}
	return false
}

// readGrpSpPr lee la transformación de un <p:grpSpPr> hasta su cierre.
func (p *pptxDec) readGrpSpPr() grupoDML {
	var g grupoDML
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return g
		}
		switch t := tok.(type) {
		case xml.StartElement:
			x, y, cx, cy := coordenada(attrLocal(t, "x")), coordenada(attrLocal(t, "y")), coordenada(attrLocal(t, "cx")), coordenada(attrLocal(t, "cy"))
			switch t.Name.Local {
			case "off":
				g.off = cpt{x, y}
			case "ext":
				g.ext = cpt{cx, cy}
			case "chOff":
				g.chOff = cpt{x, y}
			case "chExt":
				g.chExt = cpt{cx, cy}
			case "xfrm":
				continue
			}
			_ = p.dec.Skip()
		case xml.EndElement:
			if t.Name.Local == "grpSpPr" {
				return g
			}
		}
	}
}

// readXfrm lee <a:off>/<a:ext> de un <a:xfrm>.
func (p *pptxDec) readXfrm() xfrmPos {
	var pos xfrmPos
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return pos
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "off":
				pos.x, pos.y = coordenada(attrLocal(t, "x")), coordenada(attrLocal(t, "y"))
			case "ext":
				pos.cx, pos.cy = coordenada(attrLocal(t, "cx")), coordenada(attrLocal(t, "cy"))
			}
			_ = p.dec.Skip()
		case xml.EndElement:
			if t.Name.Local == "xfrm" {
				return pos
			}
		}
	}
}

// readLn lee un <a:ln> hasta su cierre: su relleno (un <a:noFill> es «sin trazo») y si lo dice.
func (p *pptxDec) readLn() (pinturaDML, bool) {
	var r pinturaDML
	dicho := false
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return r, dicho
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if esRellenoDML(t.Name.Local) {
				r, dicho = p.relleno(t), true
			} else {
				_ = p.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == "ln" {
				return r, dicho
			}
		}
	}
}

// readTxBody parsea el texto de <p:txBody> a HTML (blocksToHTML).
func (p *pptxDec) readTxBody() string {
	var blocks []xblock
	defer func() { p.escalaFuente = 0 }()
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				blocks = append(blocks, xblock{para: p.readDrawPara()})
			case "lstStyle": // la lista de la forma, delante de las que hereda
				var n xnode
				if p.dec.DecodeElement(&n, &t) == nil {
					p.propia = nivelesDe(&n)
				}
			case "bodyPr":
				var n xnode
				if p.dec.DecodeElement(&n, &t) == nil && n.kid("normAutofit") != nil {
					p.escalaFuente = atoiDefault(n.kid("normAutofit").attr("fontScale"), 0)
				}
			default:
				_ = p.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == "txBody" {
				return blocksToHTML(blocks)
			}
		}
	}
	return blocksToHTML(blocks)
}

// readDrawPara lee un <a:p>: alineación, viñeta/numeración y runs.
func (p *pptxDec) readDrawPara() *xpara {
	para := &xpara{}
	maxSize := 0
	var tamanos []int // el tamaño propio de cada tramo (0 = lo hereda)
	alineado := false // dice su alineación, también «l», que el modelo escribe como ""
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return para
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "pPr":
				alineado = attrLocal(t, "algn") != ""
				p.readDrawPPr(para, t)
			case "r", "fld":
				if run, sz, ok := p.readDrawRunOrField(t); ok {
					para.runs = append(para.runs, run)
					tamanos = append(tamanos, sz)
					maxSize = max(maxSize, sz)
				}
			case "br":
				para.runs = append(para.runs, xrun{brk: true})
				tamanos = append(tamanos, 0)
				_ = p.dec.Skip()
			default:
				_ = p.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == "p" {
				headingFromSize(para, p.aEscenario(maxSize))
				p.heredar(para, tamanos, alineado)
				return para
			}
		}
	}
}

// anchoEscenario es el ancho del escenario del editor en sus px (STAGE_W de SlidesCanvas.tsx).
const anchoEscenario = 960

// escalaTexto son los px del escenario por punto de la diapositiva. El editor pinta el texto
// en px de un escenario de 960 de ancho, sea cual sea la diapositiva, así que un tamaño en
// puntos se escala con el ancho, igual que la geometría (en %): 1 en una de 13,33 pulgadas
// (960 pt, la 16:9 de PowerPoint) y 4/3 en una de 10 (720 pt: la 4:3 y la de Google). El
// exportador hace la cuenta al revés (export_slides.go).
func escalaTexto(wEMU float64) float64 {
	if wEMU <= 0 {
		return 1
	}
	return anchoEscenario / (wEMU / emuPorPunto)
}

const emuPorPunto = 12700

// aEscenario pasa un tamaño de la diapositiva (centésimas de punto) a centésimas de px del
// escenario, con lo que el ajuste automático encogió el cuerpo, si lo encogió.
func (p *pptxDec) aEscenario(sz int) int {
	f := escalaTexto(p.wEMU)
	if p.escalaFuente > 0 && p.escalaFuente < 100000 {
		f *= float64(p.escalaFuente) / 100000
	}
	return int(math.Round(float64(sz) * f))
}

// tamCuerpo es el tamaño de un párrafo del editor en px del escenario (`BOX_CSS`): el del
// texto que no hace falta decir.
const tamCuerpo = 20

// heredar completa el párrafo con lo que hereda de sus listas de estilos (pptx_marcadores.go):
// la alineación, si no dice la suya; el color de los tramos que no dicen el suyo; y el tamaño de todos,
// el suyo o el heredado, en px del escenario, salvo en un título del editor (que lo lleva su
// estilo) o si es el de un párrafo.
//
// El color: la lista de la forma gana al estilo de la forma (`p:style`), y este a lo que se
// hereda del diseño, del patrón y de la presentación. Lo heredado que es la tinta del tema no
// se escribe: el editor ya la pinta.
func (p *pptxDec) heredar(para *xpara, tamanos []int, alineado bool) {
	propia := cadenaTexto{p.propia}.nivel(para.level)
	resto := p.cadena.nivel(para.level)
	if !alineado {
		para.align = drawAlign(cmp.Or(propia.algn, resto.algn))
	}
	tinta := cmp.Or(p.colorDeLetra(propia.letra), p.tintaForma)
	if tinta == "" {
		if tinta = p.colorDeLetra(resto.letra); p.theme != nil && tinta == p.theme.tintaTema {
			tinta = ""
		}
	}
	tinta = cmp.Or(tinta, p.tinta)
	heredado := cmp.Or(propia.sz, resto.sz)
	for i := range para.runs {
		r := &para.runs[i]
		if r.brk {
			continue
		}
		if r.color == "" {
			r.color = tinta
		}
		sz := heredado
		if i < len(tamanos) && tamanos[i] > 0 {
			sz = tamanos[i]
		}
		if para.style != "" || sz <= 0 {
			continue
		}
		if px := float64(p.aEscenario(sz)) / 100; math.Abs(px-tamCuerpo) >= 0.05 {
			r.css = conDeclaracion(r.css, "font-size", strconv.FormatFloat(math.Round(px*10)/10, 'f', -1, 64)+"px")
		}
	}
	subirComun(para) // el tamaño que comparten todos, al párrafo: como el importador de .docx
}

// colorDeLetra es el color («RRGGBB») del relleno que lleva un `a:defRPr`; "" si no lleva. Solo
// el relleno: el `a:ln` que puede ir delante es el contorno de la letra.
func (p *pptxDec) colorDeLetra(n *xnode) string {
	if n == nil {
		return ""
	}
	for i := range n.Kids {
		if !esRellenoDML(n.Kids[i].XMLName.Local) {
			continue
		}
		if dec, se, ok := decDeNodo(&n.Kids[i]); ok {
			return p.hexRelleno(rellenoDML(dec, se, p.esquema))
		}
	}
	return ""
}

// readDrawRunOrField lee un <a:r> o un <a:fld>. Los campos del patrón vuelven a ser
// {n} y {total}; en una diapositiva queda lo que muestran.
func (p *pptxDec) readDrawRunOrField(se xml.StartElement) (xrun, int, bool) {
	run, sz, ok := p.readDrawRun(se.Name.Local)
	if ok && p.master && se.Name.Local == "fld" {
		switch attrLocal(se, "type") {
		case fldSlideNum:
			run.text = "{n}"
		case fldSlideTotal:
			run.text = "{total}"
		}
	}
	return run, sz, ok
}

// headingFromSize: DrawingML no tiene estilos de título: el exportador codifica el
// nivel con el tamaño de fuente; lo revertimos para el round-trip. Y el título va en
// negrita por serlo: no es formato propio.
func headingFromSize(para *xpara, sz int) {
	if para.list != "" {
		return
	}
	if st := slideSizeToStyle(sz); st != "" && (!slideSizeLegacy(sz) || paraTodaNegrita(para)) {
		para.style = st
		for i := range para.runs {
			para.runs[i].b = false
		}
	}
}

// slideSizeLegacy: los tamaños que solo emitían las versiones anteriores del exportador
// (32 y 28 pt) son también de uso corriente en presentaciones ajenas.
func slideSizeLegacy(sz int) bool { return sz == 3200 || sz == 2800 }

// paraTodaNegrita dice si todos los tramos con texto del párrafo van en negrita, que es
// como los escribía el exportador: un 32 pt con un solo tramo en negrita es énfasis, no
// un título.
func paraTodaNegrita(para *xpara) bool {
	hay := false
	for _, r := range para.runs {
		if r.brk {
			continue
		}
		if !r.b {
			return false
		}
		hay = true
	}
	return hay
}

// readDrawPPr lee la alineación y el tipo de viñeta del párrafo DrawingML.
func (p *pptxDec) readDrawPPr(para *xpara, se xml.StartElement) {
	para.align = drawAlign(attrLocal(se, "algn"))
	para.level = atoiDefault(attrLocal(se, "lvl"), 0) // sublistas: a:pPr lvl="N"
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "buChar":
				para.list = "bullet"
			case "buAutoNum":
				para.list = "number"
			}
			_ = p.dec.Skip()
		case xml.EndElement:
			if t.Name.Local == "pPr" {
				return
			}
		}
	}
}

// readDrawRun lee un <a:r>: formato (rPr) y texto (<a:t>). Devuelve también el
// tamaño de fuente (sz, centésimas de punto) para reconstruir títulos.
func (p *pptxDec) readDrawRun(end string) (xrun, int, bool) {
	r := xrun{}
	sz := 0
	got := false
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return r, sz, got
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "rPr":
				sz = atoiDefault(attrLocal(t, "sz"), 0)
				p.readDrawRPr(&r, t)
			case "t":
				r.text = p.readCharData("t")
				got = r.text != ""
			default:
				_ = p.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == end {
				return r, sz, got // el color que no dice lo pone heredar
			}
		}
	}
}

// slideSizeToStyle revierte el tamaño de fuente de un run al estilo de título del
// editor (inverso de slideRunSize en export_slides.go). Solo reconoce los tamaños
// EXACTOS que emite el exportador —los de ahora y los de antes (32/28 pt)—; cualquier
// otro es texto normal (conservador para no convertir texto grande de presentaciones
// ajenas en títulos).
func slideSizeToStyle(sz int) string {
	switch sz {
	case 4400, 3200:
		return "Heading1"
	case 3000, 2800:
		return "Heading2"
	case 2400:
		return "Heading3"
	default:
		return ""
	}
}

// readDrawRPr lee los atributos de formato (b/i/u/strike), color y enlace del run.
func (p *pptxDec) readDrawRPr(r *xrun, se xml.StartElement) {
	// `b` e `i` son xsd:boolean: «1» o «true» (Keynote y Google escriben «true»).
	r.b = verdadero(attrLocal(se, "b"))
	r.i = verdadero(attrLocal(se, "i"))
	if u := attrLocal(se, "u"); u != "" && u != "none" {
		r.u = true
	}
	if s := attrLocal(se, "strike"); s != "" && s != "noStrike" {
		r.strike = true
	}
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "solidFill", "gradFill", "pattFill":
				if c := p.hexRelleno(p.relleno(t)); c != "" {
					r.color = c
				}
			case "highlight":
				// El resaltado (a:highlight): el fondo del tramo.
				if c, ok := colorHijo(p.dec, t, p.esquema); ok {
					r.css = strings.TrimPrefix(r.css+";background-color:#"+c.hex(), ";")
				}
				continue
			case "hlinkClick":
				if href := p.rels[attrLocal(t, "id")]; href != "" {
					r.link = href
				}
				_ = p.dec.Skip()
			default:
				_ = p.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == "rPr" {
				return
			}
		}
	}
}

// readCharData acumula el texto de un elemento hasta su cierre.
func (p *pptxDec) readCharData(local string) string {
	return readElementText(p.dec, local)
}

// extractNotes extrae el texto de una parte notesSlide (párrafos unidos por \n).
func extractNotes(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var lines []string
	var cur strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				cur.WriteString(readElementText(dec, "t"))
			}
		case xml.EndElement:
			if t.Name.Local == "p" {
				lines = append(lines, cur.String())
				cur.Reset()
			}
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// readElementText lee el CharData de un elemento ya abierto hasta su cierre.
func readElementText(dec *xml.Decoder, local string) string {
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return b.String()
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			if t.Name.Local == local {
				return b.String()
			}
		}
	}
}

// pctOf convierte una medida EMU a porcentaje del lienzo (0..100).
func pctOf(v, dim float64) float64 {
	if dim <= 0 {
		return 0
	}
	return roundPct(v / dim * 100)
}

// roundPct redondea a 3 decimales (suficiente para posiciones de caja).
func roundPct(v float64) float64 {
	return math.Round(v*1000) / 1000
}

// drawAlign mapea algn DrawingML a la alineación del editor.
func drawAlign(algn string) string {
	switch algn {
	case "ctr":
		return "center"
	case "r":
		return "right"
	case "just":
		return "justify"
	}
	return ""
}

// prstToShape mapea un preset DrawingML a la forma del editor (el inverso de
// pptxShapePresets, más los parecidos); "" si no tiene equivalente.
func prstToShape(prst string) string {
	switch prst {
	case "line", "straightConnector1":
		return "line"
	case "leftArrow":
		return "arrow"
	case "roundRect":
		return "rect"
	case "flowChartMagneticDisk":
		return "cylinder"
	case "wedgeRoundRectCallout":
		return "callout"
	}
	for shape, p := range pptxShapePresets {
		if p == prst && shape != "rect" {
			return shape
		}
	}
	return ""
}

// hexColor normaliza un color DrawingML ("FF0000") a "#RRGGBB" para el modelo del
// editor; "" si no hay color.
func hexColor(v string) string {
	if c := normalizeHexColor(v); c != "" {
		return "#" + c
	}
	return ""
}

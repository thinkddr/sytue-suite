// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"cmp"
	"encoding/xml"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Formato de WordprocessingML: propiedades de carácter (rPr, ECMA-376 Parte 1 §17.3.2), de
// párrafo (pPr, §17.3.1), de tabla y de celda (§17.4), y la herencia que las reparte
// (§17.7): docDefaults, basedOn y los estilos de tabla, de párrafo y de carácter. Casi todo
// el formato de un documento real vive en sus estilos, no en el texto.
//
// Cada propiedad tiene tres estados: no se dice (nil, 0 o ""), se activa o se desactiva. Un
// nivel más concreto pisa al de debajo solo en lo que dice. Lo que ningún nivel dice no se
// escribe en el HTML: manda el folio del editor, como mandaría en un HTML sin estilos.
//
// Clean-room sobre la especificación y la documentación pública de Microsoft; nada se ha
// portado de otra implementación.

// docxRPr son propiedades de carácter.
type docxRPr struct {
	// Interruptores (§17.7.3): entre estilos de tipos distintos se alternan; ver docxAlternar.
	b, i, strike, caps, smallCaps, vanish *bool
	u                                     *bool
	color, hl, shd                        *string // RRGGBB; "" es automático / sin resalte / sin sombreado
	vert                                  string  // superscript | subscript | baseline
	sz                                    float64 // puntos; 0 es que no se dice
	font                                  string
	espaciado                             *float64 // w:spacing (§17.3.2.35), en puntos
	posicion                              *float64 // w:position (§17.3.2.24), en puntos; positivo sube
	borde                                 *string  // w:bdr (§17.3.2.4): CSS de border o "none"
}

// sobre pisa r con lo que dice o.
func (r *docxRPr) sobre(o docxRPr) {
	for _, p := range [][2]**bool{
		{&r.b, &o.b},
		{&r.i, &o.i},
		{&r.strike, &o.strike},
		{&r.caps, &o.caps},
		{&r.smallCaps, &o.smallCaps},
		{&r.vanish, &o.vanish},
		{&r.u, &o.u},
	} {
		if *p[1] != nil {
			*p[0] = *p[1]
		}
	}
	for _, p := range [][2]**string{{&r.color, &o.color}, {&r.hl, &o.hl}, {&r.shd, &o.shd}} {
		if *p[1] != nil {
			*p[0] = *p[1]
		}
	}
	if o.vert != "" {
		r.vert = o.vert
	}
	if o.sz > 0 {
		r.sz = o.sz
	}
	if o.font != "" {
		r.font = o.font
	}
	if o.espaciado != nil {
		r.espaciado = o.espaciado
	}
	if o.posicion != nil {
		r.posicion = o.posicion
	}
	if o.borde != nil {
		r.borde = o.borde
	}
}

// docxPPr son propiedades de párrafo.
type docxPPr struct {
	jc                       string
	izq, der, primera        *float64 // sangrías en twips (§17.3.1.12); primera negativa = francesa
	antes, despues, linea    *float64 // twips; linea en 240 avos con lineRule auto
	reglaLinea               string   // auto | exact | atLeast
	contextual               *bool
	shd                      *string
	bordes                   [4]*string // top, right, bottom, left: CSS de border o "none"
	numID                    *string
	ilvl                     *int
	pStyle                   string
	outline                  *int
	marca                    docxRPr // rPr de la marca de párrafo (§17.3.1.29)
	tieneMarca, tieneBordeAb bool
	tabs                     []docxTab // paradas propias (w:tabs, §17.3.1.38), por posición
	marco                    *string   // CSS del marco de texto (framePr); "" es que no lo es
	marcoAncla               docxAncla // posición del marco cuando va respecto a la página/margen
	antesPag, finPag         bool      // pageBreakBefore y sectPr en pPr
}

// docxTab es una parada de tabulación: posición en twips desde el margen, alineación y relleno.
type docxTab struct {
	pos         float64
	val, leader string // val: left, center, right, decimal o clear (que quita la heredada)
}

// mezclarTabs pone las paradas de o sobre las de base: entre niveles se acumulan, y una
// «clear» quita la que había en su posición (§17.3.1.37).
func mezclarTabs(base, o []docxTab) []docxTab {
	out := slices.Clone(base)
	for _, t := range o {
		out = slices.DeleteFunc(out, func(b docxTab) bool { return math.Abs(b.pos-t.pos) < 1 })
		if t.val != "clear" {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b docxTab) int { return cmp.Compare(a.pos, b.pos) })
	return out
}

func (p *docxPPr) sobre(o docxPPr) {
	if o.jc != "" {
		p.jc = o.jc
	}
	if o.tieneMarca {
		// La marca de párrafo: sin pasarla, un párrafo vacío no medía lo que dice (cerrarParrafo).
		p.marca, p.tieneMarca = o.marca, true
	}
	for _, f := range [][2]**float64{
		{&p.izq, &o.izq},
		{&p.der, &o.der},
		{&p.primera, &o.primera},
		{&p.antes, &o.antes},
		{&p.despues, &o.despues},
		{&p.linea, &o.linea},
	} {
		if *f[1] != nil {
			*f[0] = *f[1]
		}
	}
	if o.reglaLinea != "" {
		p.reglaLinea = o.reglaLinea
	}
	if o.contextual != nil {
		p.contextual = o.contextual
	}
	if o.shd != nil {
		p.shd = o.shd
	}
	for i := range p.bordes {
		if o.bordes[i] != nil {
			p.bordes[i] = o.bordes[i]
		}
	}
	if o.numID != nil {
		p.numID = o.numID
	}
	if o.ilvl != nil {
		p.ilvl = o.ilvl
	}
	if o.outline != nil {
		p.outline = o.outline
	}
	if len(o.tabs) > 0 {
		p.tabs = mezclarTabs(p.tabs, o.tabs)
	}
	if o.marco != nil {
		p.marco, p.marcoAncla = o.marco, o.marcoAncla
	}
	if o.antesPag {
		p.antesPag = true
	}
	if o.finPag {
		p.finPag = true
	}
}

// docxCeldaPr son propiedades de celda (tcPr, §17.4.66) o las que una tabla da a sus
// celdas (tblPr: bordes, sombreado y márgenes por defecto).
type docxCeldaPr struct {
	shd     *string
	bordes  [6]*string  // top, right, bottom, left, insideH, insideV
	margen  [4]*float64 // top, right, bottom, left, en twips
	ancho   float64     // px (tcW en dxa)
	valign  string      // top | center | bottom
	tblAnch float64     // px: tblW de una tabla
	tblInd  *float64    // px: tblInd
	tblJc   string      // jc de una tabla
	estilo  string      // tblStyle
	span    int         // gridSpan de una celda
	vm      string      // vMerge de una celda: "restart", "continue" o ""
	look    *int        // tblLook de una tabla: qué formato condicional de su estilo se aplica
	bandaF  int         // tblStyleRowBandSize: filas por banda (0 es 1)
	bandaC  int         // tblStyleColBandSize
}

// Bits de tblLook (§17.4.56), los de su forma antigua en hexadecimal.
const (
	lookPrimeraFila = 0x20
	lookUltimaFila  = 0x40
	lookPrimeraCol  = 0x80
	lookUltimaCol   = 0x100
	lookSinBandasF  = 0x200
	lookSinBandasC  = 0x400
)

// docxCond es el formato condicional de una parte de un estilo de tabla (tblStylePr, §17.7.6).
type docxCond struct {
	r     docxRPr
	p     docxPPr
	celda docxCeldaPr
}

func (c *docxCeldaPr) sobre(o docxCeldaPr) {
	if o.shd != nil {
		c.shd = o.shd
	}
	for i := range c.bordes {
		if o.bordes[i] != nil {
			c.bordes[i] = o.bordes[i]
		}
	}
	for i := range c.margen {
		if o.margen[i] != nil {
			c.margen[i] = o.margen[i]
		}
	}
	if o.ancho > 0 {
		c.ancho = o.ancho
	}
	if o.valign != "" {
		c.valign = o.valign
	}
	if o.bandaF > 0 {
		c.bandaF = o.bandaF
	}
	if o.bandaC > 0 {
		c.bandaC = o.bandaC
	}
}

// --- La hoja: docDefaults, estilos, tema y familias de las fuentes ---

type docxEstilo struct {
	tipo, base string
	r          docxRPr
	p          docxPPr
	celda      docxCeldaPr          // de un estilo de tabla: tblPr y tcPr
	cond       map[string]*docxCond // de un estilo de tabla: tblStylePr por w:type
}

type docxHoja struct {
	rDef         docxRPr
	pDef         docxPPr
	estilos      map[string]*docxEstilo
	defecto      map[string]string // tipo -> styleId con w:default="1"
	tema         docxTema
	familias     map[string]string // nombre de fuente -> familia genérica de CSS (fontTable)
	tabulador    float64           // w:defaultTabStop de settings.xml, en twips
	margenIzq    float64           // margen izquierdo de la última sección, en twips (seccionDe)
	margenArriba float64           // margen superior de la última sección, en twips
	anchoTexto   float64           // ancho de página menos sus márgenes laterales, en twips; 0 si no se sabe
	altoPag      float64           // alto de página de la última sección, en twips; 0 si no se sabe
	exportado    bool              // el paquete trae el styles.xml que escribe la exportación de Docus
	// descartes cuenta lo que se lee y no se traduce (ver corpus_docx_test.go); nil no cuenta.
	descartes map[string]int
}

type docxTema struct {
	colores      map[string]string // dk1, lt1, accent1… -> RRGGBB
	mayor, menor string            // tipografías latinas del tema
}

// descarta apunta una propiedad leída y no traducida, con su contenedor («rPr/w», «tcPr/noWrap»).
func (h *docxHoja) descarta(donde, que string) {
	if h != nil && h.descartes != nil {
		h.descartes[donde+"/"+que]++
	}
}

// descartaContenido apunta un elemento del cuerpo, de un párrafo o de un tramo que no se lee,
// salvo las marcas que no llevan contenido (marcadores, revisiones de ortografía, permisos).
func (h *docxHoja) descartaContenido(donde, que string) {
	switch que {
	case "bookmarkStart", "bookmarkEnd", "proofErr", "permStart", "permEnd", "lastRenderedPageBreak",
		"commentReference", "annotationRef", "moveFromRangeStart", "moveFromRangeEnd", "moveToRangeStart",
		"moveToRangeEnd", "rPr", "pPr", "separator", "continuationSeparator", "footnoteRef", "endnoteRef",
		"customXmlInsRangeStart", "customXmlInsRangeEnd", "customXmlDelRangeStart", "customXmlDelRangeEnd",
		"sdtPr", "sdtEndPr", "smartTagPr", "customXmlPr":
		return
	}
	h.descarta(donde, que)
}

// docxDescartes, si no es nil, recoge lo que cada importación lee y no traduce. Solo lo enciende
// la medida del corpus (corpus_docx_test.go), que importa los ficheros de uno en uno.
var docxDescartes map[string]int

func parseDocxHoja(files map[string][]byte) *docxHoja {
	h := &docxHoja{estilos: map[string]*docxEstilo{}, defecto: map[string]string{}, descartes: docxDescartes}
	h.tema = parseDocxTema(files["word/theme/theme1.xml"])
	h.familias = parseDocxFamilias(files["word/fontTable.xml"])
	h.tabulador = parseDocxTabulador(files["word/settings.xml"])
	data := files["word/styles.xml"]
	if len(data) == 0 || string(data) == docxStyles || slices.Contains(docxStylesAnteriores, string(data)) {
		// Sin estilos, o con los que escribe la exportación de Docus: esos son el reflejo de los
		// del editor (Calibri 11 para Word, la cursiva de la cita), no formato del autor, y
		// aplicarlos al volver a abrir el fichero cambiaría el documento que se exportó. De ellos
		// solo cuenta la negrita de los títulos 1 a 3, que es la del editor: así un título que
		// se exportó sin negrita (<w:b w:val="0"/>) vuelve sin ella.
		if len(data) > 0 {
			h.exportado = true
			for _, id := range []string{"Heading1", "Heading2", "Heading3"} {
				si := true
				h.estilos[id] = &docxEstilo{tipo: "paragraph", r: docxRPr{b: &si}}
			}
		}
		return h
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return h
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "rPrDefault", "pPrDefault": // contenedores: se entra
		case "rPr":
			h.leerRPr(dec, &h.rDef)
		case "pPr":
			h.leerPPr(dec, &h.pDef)
		case "style":
			h.leerEstilo(dec, se)
		case "styles", "docDefaults":
		default:
			_ = dec.Skip() // latentStyles
		}
	}
}

func (h *docxHoja) leerEstilo(dec *xml.Decoder, se xml.StartElement) {
	e := &docxEstilo{tipo: attrLocal(se, "type")}
	id := attrLocal(se, "styleId")
	if v := attrLocal(se, "default"); v == "1" || v == "true" || v == "on" {
		if _, ya := h.defecto[e.tipo]; !ya {
			h.defecto[e.tipo] = id
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if ee, ok := tok.(xml.EndElement); ok && ee.Name.Local == "style" {
			break
		}
		t, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch t.Name.Local {
		case "basedOn":
			e.base = attrLocal(t, "val")
			_ = dec.Skip()
		case "rPr":
			h.leerRPr(dec, &e.r)
		case "pPr":
			h.leerPPr(dec, &e.p)
		case "tblPr", "tcPr":
			h.leerCeldaPr(dec, t.Name.Local, &e.celda)
		case "tblStylePr":
			if e.cond == nil {
				e.cond = map[string]*docxCond{}
			}
			c := &docxCond{}
			h.leerCond(dec, c)
			e.cond[attrLocal(t, "type")] = c
		default:
			_ = dec.Skip()
		}
	}
	if id != "" {
		h.estilos[id] = e
	}
}

// leerCond lee un tblStylePr: lo que da a la letra, al párrafo y a las celdas de su parte.
func (h *docxHoja) leerCond(dec *xml.Decoder, c *docxCond) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if t.Name.Local == "tblStylePr" {
				return
			}
		case xml.StartElement:
			switch t.Name.Local {
			case "rPr":
				h.leerRPr(dec, &c.r)
			case "pPr":
				h.leerPPr(dec, &c.p)
			case "tblPr", "tcPr":
				h.leerCeldaPr(dec, t.Name.Local, &c.celda)
			default: // trPr: alto de fila y cabecera repetida, que el editor no tiene
				_ = dec.Skip()
			}
		}
	}
}

// cadena devuelve los estilos de id hacia arriba por basedOn, el más general primero.
// Iterativa y acotada: el fichero viene de fuera y puede traer ciclos.
func (h *docxHoja) cadena(id string) []*docxEstilo {
	var out []*docxEstilo
	visto := map[string]bool{}
	for cur := id; cur != "" && !visto[cur] && len(out) < 64; {
		visto[cur] = true
		e, ok := h.estilos[cur]
		if !ok {
			break
		}
		out = append(out, e)
		cur = e.base
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// estiloP es el pPr y el rPr del estilo de párrafo id (o del de por defecto), ya heredados.
func (h *docxHoja) estiloP(id string) (docxPPr, docxRPr) {
	if id == "" || h.estilos[id] == nil {
		id = h.defecto["paragraph"]
	}
	var p docxPPr
	var r docxRPr
	for _, e := range h.cadena(id) {
		p.sobre(e.p)
		r.sobre(e.r)
	}
	return p, r
}

func (h *docxHoja) estiloR(id string) docxRPr {
	if id == "" || h.estilos[id] == nil {
		id = h.defecto["character"]
	}
	var r docxRPr
	for _, e := range h.cadena(id) {
		r.sobre(e.r)
	}
	return r
}

func (h *docxHoja) estiloT(id string) (docxCeldaPr, docxPPr, docxRPr, map[string]docxCond) {
	if id == "" || h.estilos[id] == nil {
		id = h.defecto["table"]
	}
	var c docxCeldaPr
	var p docxPPr
	var r docxRPr
	cond := map[string]docxCond{}
	for _, e := range h.cadena(id) {
		c.sobre(e.celda)
		p.sobre(e.p)
		r.sobre(e.r)
		for tipo, o := range e.cond {
			v := cond[tipo]
			v.celda.sobre(o.celda)
			v.p.sobre(o.p)
			v.r.sobre(o.r)
			cond[tipo] = v
		}
	}
	return c, p, r, cond
}

// docxAlternar resuelve un interruptor (§17.7.3): lo directo manda; si no, entre los estilos de
// tabla, de párrafo y de carácter que lo activan se alterna (dos activados lo apagan), y si
// ninguno lo dice, vale lo de docDefaults. ponytail: un solo estilo que lo dice manda tal cual.
func docxAlternar(def *bool, estilos []*bool, directo *bool) *bool {
	if directo != nil {
		return directo
	}
	var dichos, activos int
	var ultimo *bool
	for _, v := range estilos {
		if v != nil {
			dichos++
			ultimo = v
			if *v {
				activos++
			}
		}
	}
	switch {
	case activos >= 2:
		v := activos%2 == 1
		return &v
	case dichos > 0:
		return ultimo
	}
	return def
}

// resolverR junta las propiedades de carácter de un tramo: docDefaults, estilo de tabla, de
// párrafo y de carácter, y lo directo (§17.7.2).
func (h *docxHoja) resolverR(tabla, parrafo, caracter, directo docxRPr) docxRPr {
	out := h.rDef
	out.sobre(tabla)
	out.sobre(parrafo)
	out.sobre(caracter)
	out.sobre(directo)
	niveles := []docxRPr{tabla, parrafo, caracter}
	for _, f := range []func(*docxRPr) **bool{
		func(r *docxRPr) **bool { return &r.b }, func(r *docxRPr) **bool { return &r.i },
		func(r *docxRPr) **bool { return &r.strike }, func(r *docxRPr) **bool { return &r.caps },
		func(r *docxRPr) **bool { return &r.smallCaps }, func(r *docxRPr) **bool { return &r.vanish },
	} {
		est := make([]*bool, 0, len(niveles))
		for i := range niveles {
			est = append(est, *f(&niveles[i]))
		}
		*f(&out) = docxAlternar(*f(&h.rDef), est, *f(&directo))
	}
	return out
}

// --- Lectores (sobre el decodificador ya situado dentro del elemento) ---

func (h *docxHoja) leerRPr(dec *xml.Decoder, r *docxRPr) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if t.Name.Local == "rPr" {
				return
			}
		case xml.StartElement:
			h.propiedadR(t, r)
			_ = dec.Skip()
		}
	}
}

//nolint:gocyclo,gocognit // una rama por propiedad de rPr: la ramificación ES la tabla
func (h *docxHoja) propiedadR(t xml.StartElement, r *docxRPr) {
	si := func() *bool { v := boolAttr(t); return &v }
	switch t.Name.Local {
	case "b":
		r.b = si()
	case "i":
		r.i = si()
	case "strike", "dstrike":
		if v := si(); *v || r.strike == nil {
			r.strike = v
		}
	case "caps":
		r.caps = si()
	case "smallCaps":
		r.smallCaps = si()
	case "vanish":
		r.vanish = si()
	case "u":
		// Sin w:val no hay subrayado: CT_Underline solo trae el color, y Pages lo escribe así
		// en cada tramo. Subraya cualquier valor de ST_Underline salvo «none».
		v := attrLocal(t, "val")
		u := v != "" && v != "none"
		r.u = &u
	case "color":
		c := h.colorTema(attrLocal(t, "val"), attrLocal(t, "themeColor"), attrLocal(t, "themeTint"), attrLocal(t, "themeShade"))
		r.color = &c
	case "highlight":
		c := docxResaltes[attrLocal(t, "val")]
		r.hl = &c
	case "shd":
		r.shd = h.sombreado(t)
	case "sz":
		if n, err := strconv.ParseFloat(attrLocal(t, "val"), 64); err == nil && n > 0 && n <= 3276 {
			r.sz = n / 2
		}
	case "rFonts":
		r.font = h.fuente(t)
	case "vertAlign":
		r.vert = attrLocal(t, "val")
	case "spacing":
		if n, err := strconv.ParseFloat(attrLocal(t, "val"), 64); err == nil && math.Abs(n) <= 31680 {
			pt := n / 20
			r.espaciado = &pt
		}
	case "position":
		if n, err := strconv.ParseFloat(attrLocal(t, "val"), 64); err == nil && math.Abs(n) <= 3168 {
			pt := n / 2
			r.posicion = &pt
		}
	case "bdr":
		r.borde = h.borde(t)
	case "rStyle", "lang", "szCs", "bCs", "iCs", "rtl", "cs", "noProof", "webHidden", "specVanish", "kern",
		"snapToGrid", "ins", "del", "moveFrom", "moveTo", "rPrChange":
		// Sin efecto en lo que se pinta: rStyle lo lee el tramo; lang, el corrector; los *Cs son
		// del texto complejo; rtl solo invierte el orden de ese texto; kern es el umbral del
		// interletraje, que el navegador ya aplica; snapToGrid, la rejilla de Asia oriental; ins,
		// del y rPrChange, las revisiones de la marca de párrafo y del formato.
	case "w":
		if attrLocal(t, "val") != "100" {
			h.descarta("rPr", "w") // escala horizontal: CSS no la tiene para texto en línea
		}
	case "outline", "emboss", "imprint", "shadow":
		if boolAttr(t) {
			h.descarta("rPr", t.Name.Local) // activado: relieve que el editor no pinta
		}
	default:
		h.descarta("rPr", t.Name.Local)
	}
}

// docxResaltes son los colores de ST_HighlightColor (§17.18.40).
var docxResaltes = map[string]string{
	"black": "000000", "blue": "0000FF", "cyan": "00FFFF", "green": "00FF00", "magenta": "FF00FF",
	"red": "FF0000", "yellow": "FFFF00", "white": "FFFFFF", "darkBlue": "000080", "darkCyan": "008080",
	"darkGreen": "008000", "darkMagenta": "800080", "darkRed": "800000", "darkYellow": "808000",
	"darkGray": "808080", "lightGray": "C0C0C0", "none": "",
}

func (h *docxHoja) leerPPr(dec *xml.Decoder, p *docxPPr) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if t.Name.Local == "pPr" {
				return
			}
		case xml.StartElement:
			h.propiedadP(dec, t, p)
		}
	}
}

//nolint:gocyclo,gocognit // una rama por propiedad de pPr: la ramificación ES la tabla
func (h *docxHoja) propiedadP(dec *xml.Decoder, t xml.StartElement, p *docxPPr) {
	twips := func(local string) *float64 {
		if n, err := strconv.ParseFloat(attrLocal(t, local), 64); err == nil && math.Abs(n) <= 31680 {
			return &n
		}
		return nil
	}
	switch t.Name.Local {
	case "rPr":
		p.tieneMarca = true
		h.leerRPr(dec, &p.marca)
		return
	case "numPr":
		h.leerNumPr(dec, p)
		return
	case "tabs":
		h.leerTabs(dec, p)
		return
	case "framePr":
		v, a := h.marcoCSS(t)
		p.marco, p.marcoAncla = &v, a
	case "pBdr":
		h.leerBordes(dec, "pBdr", p.bordes[:])
		p.tieneBordeAb = p.bordes[2] != nil && *p.bordes[2] != "none"
		return
	case "pStyle":
		p.pStyle = attrLocal(t, "val")
	case "jc":
		p.jc = attrLocal(t, "val")
	case "ind":
		if v := cmpPtr(twips("left"), twips("start")); v != nil {
			p.izq = v
		}
		if v := cmpPtr(twips("right"), twips("end")); v != nil {
			p.der = v
		}
		if v := twips("firstLine"); v != nil {
			p.primera = v
		}
		if v := twips("hanging"); v != nil {
			n := -*v
			p.primera = &n
		}
	case "spacing":
		if v := twips("before"); v != nil {
			p.antes = v
		}
		if v := twips("after"); v != nil {
			p.despues = v
		}
		// beforeAutospacing/afterAutospacing: el espaciado «automático» de HTML, 14 pt.
		auto := 280.0
		if a := attrLocal(t, "beforeAutospacing"); a != "" && a != "0" && a != "false" && a != "off" {
			p.antes = &auto
		}
		if a := attrLocal(t, "afterAutospacing"); a != "" && a != "0" && a != "false" && a != "off" {
			p.despues = &auto
		}
		if v := twips("line"); v != nil && *v > 0 {
			p.linea = v
			p.reglaLinea = attrLocal(t, "lineRule")
			if p.reglaLinea == "" {
				p.reglaLinea = "auto"
			}
		}
	case "contextualSpacing":
		v := boolAttr(t)
		p.contextual = &v
	case "shd":
		p.shd = h.sombreado(t)
	case "outlineLvl":
		if n, err := strconv.Atoi(attrLocal(t, "val")); err == nil {
			p.outline = &n
		}
	case "keepNext", "keepLines", "widowControl", "suppressAutoHyphens", "suppressLineNumbers",
		"snapToGrid", "adjustRightInd", "autoSpaceDE", "autoSpaceDN", "kinsoku", "wordWrap",
		"overflowPunct", "topLinePunct", "textAlignment", "bidi", "mirrorIndents", "suppressOverlap",
		"pPrChange", "rPrChange", "cnfStyle", "divId":
		// Paginación, partición de palabras y tipografía de Asia oriental: el editor no pagina
		// así; cnfStyle es la caché del formato condicional de una tabla y divId, el div de un
		// HTML pegado. bidi=0 es el valor por defecto; activado sí se pierde y se cuenta.
		if t.Name.Local == "bidi" && boolAttr(t) {
			h.descarta("pPr", "bidi")
		}
	case "sectPr":
		p.finPag = true
	case "pageBreakBefore":
		if boolAttr(t) {
			p.antesPag = true
			h.descarta("pPr", "pageBreakBefore")
		}
	default:
		h.descarta("pPr", t.Name.Local)
	}
	_ = dec.Skip()
}

// leerTabs lee w:tabs. Las de alineación «num» son las de una lista, que pinta el editor; las
// «bar» (una raya vertical en la posición) no se pintan y se cuentan.
func (h *docxHoja) leerTabs(dec *xml.Decoder, p *docxPPr) {
	var tabs []docxTab
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if t, ok := tok.(xml.EndElement); ok && t.Name.Local == "tabs" {
			break
		}
		t, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		_ = dec.Skip()
		pos, err := strconv.ParseFloat(attrLocal(t, "pos"), 64)
		if t.Name.Local != "tab" || err != nil || math.Abs(pos) > 31680 {
			continue
		}
		val := attrLocal(t, "val")
		switch val {
		case "start":
			val = "left"
		case "end":
			val = "right"
		case "left", "right", "center", "decimal", "clear":
		case "bar":
			h.descarta("tabs", "bar")
			continue
		default: // num
			continue
		}
		tabs = append(tabs, docxTab{pos: pos, val: val, leader: docxRellenos[attrLocal(t, "leader")]})
	}
	p.tabs = append(p.tabs, tabs...) // con sus «clear»: se aplican al ponerlo sobre otro nivel
}

// docxRellenos son los de ST_TabTlc (§17.18.85), con el nombre que lee el editor.
var docxRellenos = map[string]string{"dot": "dot", "hyphen": "hyphen", "underscore": "underscore", "heavy": "heavy", "middleDot": "middledot"}

func (h *docxHoja) leerNumPr(dec *xml.Decoder, p *docxPPr) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if t.Name.Local == "numPr" {
				return
			}
		case xml.StartElement:
			switch t.Name.Local {
			case "ilvl":
				n := atoiDefault(attrLocal(t, "val"), 0)
				p.ilvl = &n
			case "numId":
				v := attrLocal(t, "val")
				p.numID = &v
			}
			_ = dec.Skip()
		}
	}
}

// leerLook lee tblLook: sus atributos o, en la forma de Word 2007, el hexadecimal de w:val.
func leerLook(t xml.StartElement) *int {
	n64, _ := strconv.ParseInt(attrLocal(t, "val"), 16, 32)
	n := int(n64)
	for attr, bit := range map[string]int{
		"firstRow": lookPrimeraFila, "lastRow": lookUltimaFila, "firstColumn": lookPrimeraCol,
		"lastColumn": lookUltimaCol, "noHBand": lookSinBandasF, "noVBand": lookSinBandasC,
	} {
		switch v := attrLocal(t, attr); v {
		case "":
		case "1", "true", "on":
			n |= bit
		default:
			n &^= bit
		}
	}
	return &n
}

// docxLados es el orden de los bordes: el de CSS (top, right, bottom, left) y los interiores.
var docxLados = map[string]int{"top": 0, "right": 1, "end": 1, "bottom": 2, "left": 3, "start": 3, "insideH": 4, "insideV": 5}

// leerBordes lee pBdr, tblBorders o tcBorders hasta su cierre; los lados que no caben en
// dst (los interiores de un párrafo, between, bar) se cuentan como descartados.
func (h *docxHoja) leerBordes(dec *xml.Decoder, fin string, dst []*string) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if t.Name.Local == fin {
				return
			}
		case xml.StartElement:
			if i, ok := docxLados[t.Name.Local]; ok && i < len(dst) {
				dst[i] = h.borde(t)
			} else if v := attrLocal(t, "val"); v != "nil" && v != "none" {
				h.descarta(fin, t.Name.Local)
			}
			_ = dec.Skip()
		}
	}
}

//nolint:gocyclo,gocognit // una rama por propiedad de tblPr/tcPr: la ramificación ES la tabla
func (h *docxHoja) leerCeldaPr(dec *xml.Decoder, fin string, c *docxCeldaPr) {
	px := func(t xml.StartElement) float64 {
		n, err := strconv.ParseFloat(attrLocal(t, "w"), 64)
		if err != nil || n <= 0 || n > 31680 {
			return 0
		}
		switch attrLocal(t, "type") {
		case "dxa", "":
			return n / 15
		}
		return 0 // pct y auto: lo decide la maqueta
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if t.Name.Local == fin {
				return
			}
		case xml.StartElement:
			switch t.Name.Local {
			case "tblBorders", "tcBorders":
				h.leerBordes(dec, t.Name.Local, c.bordes[:])
				continue
			case "tblCellMar", "tcMar":
				h.leerMargenes(dec, t.Name.Local, c)
				continue
			case "shd":
				c.shd = h.sombreado(t)
			case "tcW":
				c.ancho = px(t)
			case "tblW":
				c.tblAnch = px(t)
			case "tblInd":
				if n := px(t); n > 0 {
					c.tblInd = &n
				}
			case "jc":
				c.tblJc = attrLocal(t, "val")
			case "vAlign":
				c.valign = attrLocal(t, "val")
			case "tblStyle":
				c.estilo = attrLocal(t, "val")
			case "gridSpan":
				c.span = atoiDefault(attrLocal(t, "val"), 1)
			case "vMerge":
				c.vm = "continue"
				if attrLocal(t, "val") == "restart" {
					c.vm = "restart"
				}
			case "tblLook":
				c.look = leerLook(t)
			case "tblStyleRowBandSize":
				c.bandaF = atoiDefault(attrLocal(t, "val"), 1)
			case "tblStyleColBandSize":
				c.bandaC = atoiDefault(attrLocal(t, "val"), 1)
			case "tblLayout", "tblCellSpacing", "tcPrChange",
				"tblPrChange", "tblCaption", "tblDescription", "noWrap", "tcFitText", "hideMark", "cnfStyle":
				// Maquetación que la rejilla del editor no tiene y no cambia lo que se ve.
			default:
				h.descarta(fin, t.Name.Local)
			}
			_ = dec.Skip()
		}
	}
}

func (h *docxHoja) leerMargenes(dec *xml.Decoder, fin string, c *docxCeldaPr) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if t.Name.Local == fin {
				return
			}
		case xml.StartElement:
			if i, ok := docxLados[t.Name.Local]; ok && i < 4 {
				if n, err := strconv.ParseFloat(attrLocal(t, "w"), 64); err == nil && n >= 0 && n <= 31680 &&
					(attrLocal(t, "type") == "dxa" || attrLocal(t, "type") == "") {
					c.margen[i] = &n
				}
			}
			_ = dec.Skip()
		}
	}
}

// --- Valores: colores, sombreado, bordes y fuentes ---

// colorTema resuelve un color de WordprocessingML: el del tema si lo nombra (§17.3.2.6: con
// themeColor se ignora val), con su tinte o su sombra; si no, val. "" es automático.
func (h *docxHoja) colorTema(val, tema, tinte, sombra string) string {
	if c, ok := h.tema.colores[docxNombreTema[tema]]; ok && c != "" {
		return docxTinteSombra(c, tinte, sombra)
	}
	return docxHex(val)
}

func docxHex(v string) string {
	v = strings.TrimSpace(v)
	if len(v) != 6 || strings.EqualFold(v, "auto") {
		return ""
	}
	if _, err := strconv.ParseUint(v, 16, 32); err != nil {
		return ""
	}
	return strings.ToUpper(v)
}

// docxNombreTema traduce ST_ThemeColor a la entrada de a:clrScheme (con el clrSchemeMapping
// por defecto: el texto en dk y el fondo en lt).
var docxNombreTema = map[string]string{
	"dark1": "dk1", "text1": "dk1", "light1": "lt1", "background1": "lt1",
	"dark2": "dk2", "text2": "dk2", "light2": "lt2", "background2": "lt2",
	"accent1": "accent1", "accent2": "accent2", "accent3": "accent3", "accent4": "accent4",
	"accent5": "accent5", "accent6": "accent6", "hyperlink": "hlink", "followedHyperlink": "folHlink",
}

// docxTinteSombra aplica themeTint o themeShade (un byte en hexadecimal) a la luminancia HSL
// del color: el tinte aclara hacia el blanco y la sombra oscurece hacia el negro.
func docxTinteSombra(c, tinte, sombra string) string {
	f := func(s string) (float64, bool) {
		n, err := strconv.ParseUint(s, 16, 8)
		return float64(n) / 255, err == nil && s != ""
	}
	t, hayT := f(tinte)
	s, hayS := f(sombra)
	if !hayT && !hayS {
		return c
	}
	n, _ := strconv.ParseUint(c, 16, 32)
	r, g, b := float64(n>>16&0xff)/255, float64(n>>8&0xff)/255, float64(n&0xff)/255
	hh, ss, l := rgbAHSL(r, g, b)
	if hayT {
		l = l*t + (1 - t)
	}
	if hayS {
		l *= s
	}
	r, g, b = hslARGB(hh, ss, l)
	return docxRGB(r, g, b)
}

func docxRGB(r, g, b float64) string {
	byte8 := func(x float64) uint64 { return uint64(math.Round(math.Max(0, math.Min(1, x)) * 255)) }
	return strings.ToUpper(strconv.FormatUint(1<<24|byte8(r)<<16|byte8(g)<<8|byte8(b), 16)[1:])
}

func rgbAHSL(r, g, b float64) (h, s, l float64) {
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case r:
		h = math.Mod((g-b)/d+6, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

func hslARGB(h, s, l float64) (r, g, b float64) {
	if s == 0 {
		return l, l, l
	}
	q := l * (1 + s)
	if l >= 0.5 {
		q = l + s - l*s
	}
	p := 2*l - q
	canal := func(t float64) float64 {
		t = math.Mod(t+1, 1)
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		}
		return p
	}
	return canal(h + 1.0/3), canal(h), canal(h - 1.0/3)
}

// sombreado resuelve un w:shd (§17.3.5): con «clear» manda el relleno (fill); con «solid», el
// color del patrón; con un porcentaje, la mezcla de los dos. «nil» es sin sombreado, y un
// relleno «auto» no pinta nada (nunca es negro ni blanco). ponytail: las tramas de rayas y
// cuadrícula se pintan como su mezcla al 50 %.
func (h *docxHoja) sombreado(t xml.StartElement) *string {
	val := attrLocal(t, "val")
	relleno := h.colorTema(attrLocal(t, "fill"), attrLocal(t, "themeFill"), attrLocal(t, "themeFillTint"), attrLocal(t, "themeFillShade"))
	patron := h.colorTema(attrLocal(t, "color"), attrLocal(t, "themeColor"), attrLocal(t, "themeTint"), attrLocal(t, "themeShade"))
	out := ""
	switch val {
	case "nil":
	case "clear", "":
		out = relleno
	case "solid":
		out = cmpOr(patron, "000000")
	default:
		pct := 0.5
		if n, err := strconv.Atoi(strings.TrimPrefix(val, "pct")); err == nil && strings.HasPrefix(val, "pct") && n >= 0 && n <= 100 {
			pct = float64(n) / 100
		}
		out = docxMezcla(cmpOr(patron, "000000"), cmpOr(relleno, "FFFFFF"), pct)
	}
	return &out
}

func cmpPtr(a, b *float64) *float64 {
	if a != nil {
		return a
	}
	return b
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func docxMezcla(a, b string, pct float64) string {
	x, _ := strconv.ParseUint(a, 16, 32)
	y, _ := strconv.ParseUint(b, 16, 32)
	m := func(s uint) float64 {
		return (float64(x>>s&0xff)*pct + float64(y>>s&0xff)*(1-pct)) / 255
	}
	return docxRGB(m(16), m(8), m(0))
}

// borde traduce un borde de párrafo, tabla o celda (ST_Border, §17.18.2) a CSS. El grosor
// viene en octavos de punto.
func (h *docxHoja) borde(t xml.StartElement) *string {
	val := attrLocal(t, "val")
	if val == "" || val == "nil" || val == "none" {
		n := "none"
		return &n
	}
	estilo := "solid"
	switch {
	case val == "double" || strings.HasPrefix(val, "triple") || strings.HasPrefix(val, "thinThick") || strings.HasPrefix(val, "thickThin"):
		estilo = "double"
	case val == "dotted":
		estilo = "dotted"
	case strings.HasPrefix(val, "dash") || strings.HasPrefix(val, "dot"):
		estilo = "dashed"
	}
	ancho := 1.0
	if n, err := strconv.ParseFloat(attrLocal(t, "sz"), 64); err == nil && n > 0 && n <= 96 {
		ancho = math.Max(1, math.Round(n/8*4/3))
	}
	if estilo == "double" {
		ancho = math.Max(ancho, 3)
	}
	c := cmpOr(h.colorTema(attrLocal(t, "color"), attrLocal(t, "themeColor"), attrLocal(t, "themeTint"), attrLocal(t, "themeShade")), "000000")
	out := strconv.FormatFloat(ancho, 'f', -1, 64) + "px " + estilo + " #" + c
	return &out
}

// fuente lee w:rFonts: la latina (ascii, o hAnsi), o la del tema si la nombra.
func (h *docxHoja) fuente(t xml.StartElement) string {
	for _, a := range []string{"asciiTheme", "hAnsiTheme"} {
		switch v := attrLocal(t, a); {
		case strings.HasPrefix(v, "major") && h.tema.mayor != "":
			return h.tema.mayor
		case strings.HasPrefix(v, "minor") && h.tema.menor != "":
			return h.tema.menor
		}
	}
	return cmpOr(attrLocal(t, "ascii"), attrLocal(t, "hAnsi"))
}

func parseDocxTema(data []byte) docxTema {
	tema := docxTema{colores: map[string]string{}}
	if len(data) == 0 {
		return tema
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	var enEsquema, enMayor bool
	var actual string
	for {
		tok, err := dec.Token()
		if err != nil {
			return tema
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "clrScheme":
				enEsquema = true
			case "majorFont":
				enMayor = true
			case "minorFont":
				enMayor = false
			case "srgbClr", "sysClr":
				if enEsquema && actual != "" {
					tema.colores[actual] = docxHex(cmpOr(attrLocal(t, "lastClr"), attrLocal(t, "val")))
				}
			case "latin":
				if enMayor && tema.mayor == "" {
					tema.mayor = attrLocal(t, "typeface")
				} else if !enMayor && tema.menor == "" {
					tema.menor = attrLocal(t, "typeface")
				}
			default:
				if enEsquema {
					actual = t.Name.Local
				}
			}
		case xml.EndElement:
			if t.Name.Local == "clrScheme" {
				enEsquema = false
			}
		}
	}
}

// parseDocxFamilias lee fontTable.xml: la familia de cada fuente (w:family, §17.8.3.10) da la
// genérica de CSS a la que caer si quien abre el documento no tiene esa fuente.
func parseDocxFamilias(data []byte) map[string]string {
	out := map[string]string{}
	if len(data) == 0 {
		return out
	}
	var doc struct {
		Fonts []struct {
			Name   string `xml:"name,attr"`
			Family struct {
				Val string `xml:"val,attr"`
			} `xml:"family"`
			Pitch struct {
				Val string `xml:"val,attr"`
			} `xml:"pitch"`
		} `xml:"font"`
	}
	if xml.Unmarshal(data, &doc) != nil {
		return out
	}
	gen := map[string]string{"roman": "serif", "swiss": "sans-serif", "modern": "monospace", "script": "cursive", "decorative": "fantasy"}
	for _, f := range doc.Fonts {
		// Pages declara «roman» con pitch «default» para toda fuente que no conoce (Calibri,
		// Helvetica Neue): esa familia no informa.
		if g := gen[f.Family.Val]; g != "" && f.Pitch.Val != "default" {
			out[f.Name] = g
		}
	}
	return out
}

// parseDocxTabulador lee w:defaultTabStop (§17.15.1.25); sin él, media pulgada (720 twips).
func parseDocxTabulador(data []byte) float64 {
	var doc struct {
		Tab struct {
			Val string `xml:"val,attr"`
		} `xml:"defaultTabStop"`
	}
	if xml.Unmarshal(data, &doc) == nil {
		if n, err := strconv.ParseFloat(doc.Tab.Val, 64); err == nil && n >= 60 && n <= 31680 {
			return n
		}
	}
	return 720
}

// --- A CSS ---

// docxCSS acumula declaraciones de CSS con valores que el importador ha calculado o saneado.
type docxCSS struct{ b strings.Builder }

func (c *docxCSS) add(prop, val string) {
	if val == "" {
		return
	}
	if c.b.Len() > 0 {
		c.b.WriteByte(';')
	}
	c.b.WriteString(prop + ":" + val)
}

func (c *docxCSS) String() string { return c.b.String() }

func docxPx(twips float64) string {
	if twips == 0 {
		return "0"
	}
	return docxNum(twips/15) + "px"
}

func docxNum(n float64) string { return strconv.FormatFloat(math.Round(n*100)/100, 'f', -1, 64) }

// familiaCSS es el font-family de una fuente del documento, con su genérica detrás. El nombre
// viene del fichero: solo pasan letras, cifras, espacios y guiones, así que no puede cerrar la
// declaración ni el atributo.
func (h *docxHoja) familiaCSS(nombre string) string {
	limpio := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '_' {
			return r
		}
		return -1
	}, nombre)
	limpio = strings.TrimSpace(limpio)
	if limpio == "" {
		return ""
	}
	if strings.ContainsAny(limpio, " 0123456789") {
		limpio = "'" + limpio + "'" // sin comillas, un nombre con espacios o cifras no es válido en CSS
	}
	if s := docxSustitutas[strings.ToLower(nombre)]; s != "" {
		return limpio + "," + s
	}
	if g := h.familias[nombre]; g != "" {
		return limpio + "," + g
	}
	return limpio
}

// docxSustitutas son, para las fuentes de Office y de Apple más comunes, su equivalente libre
// con las mismas métricas y su familia genérica: sin ellas, quien no tiene Calibri vería el
// documento en la letra por defecto del navegador, que suele ser con serifa.
var docxSustitutas = map[string]string{
	"calibri": "Carlito,sans-serif", "calibri light": "Carlito,sans-serif", "cambria": "Caladea,serif",
	"arial": "'Liberation Sans',Arimo,sans-serif", "helvetica": "'Liberation Sans',Arimo,sans-serif",
	"helvetica neue": "'Liberation Sans',Arimo,sans-serif", "times new roman": "'Liberation Serif',Tinos,serif",
	"times": "'Liberation Serif',Tinos,serif", "courier new": "'Liberation Mono',Cousine,monospace",
	"aptos": "sans-serif", "segoe ui": "sans-serif", "verdana": "sans-serif", "tahoma": "sans-serif",
	"century gothic": "sans-serif", "georgia": "serif", "garamond": "serif", "book antiqua": "serif",
}

// cssBase son las propiedades de carácter que un bloque da a todo su texto: tipografía,
// tamaño y color. Solo las que algún nivel dice.
//
// El tamaño de 12 pt y el negro no se escriben en un párrafo: son los del folio del editor
// (16 px; su tinta, #0a0a0a). Un título sí lleva su tamaño, que el editor pone mayor.
func (h *docxHoja) cssBase(css *docxCSS, r docxRPr, titulo bool) {
	if f := h.familiaCSS(r.font); f != "" {
		css.add("font-family", f)
	}
	if r.sz > 0 && (r.sz != 12 || titulo) {
		css.add("font-size", docxNum(r.sz)+"pt")
	}
	if r.color != nil && *r.color != "" && *r.color != "000000" {
		css.add("color", "#"+*r.color)
	}
}

// fondoTramo es el fondo de un tramo: el resaltado, que se pinta encima del sombreado.
func fondoTramo(r docxRPr) string {
	if r.hl != nil && *r.hl != "" {
		return *r.hl
	}
	if r.shd != nil {
		return *r.shd
	}
	return ""
}

// cssTramo es el estilo de un tramo frente al de su bloque: solo lo que cambia.
func (h *docxHoja) cssTramo(r, base docxRPr) string {
	var css docxCSS
	if r.font != "" && r.font != base.font {
		if f := h.familiaCSS(r.font); f != "" {
			css.add("font-family", f)
		}
	}
	if r.sz > 0 && r.sz != base.sz {
		css.add("font-size", docxNum(r.sz)+"pt")
	}
	if fondo := fondoTramo(r); fondo != "" {
		css.add("background-color", "#"+fondo)
	}
	if r.caps != nil && *r.caps {
		css.add("text-transform", "uppercase")
	} else if r.smallCaps != nil && *r.smallCaps {
		css.add("font-variant", "small-caps")
	}
	if r.espaciado != nil && *r.espaciado != 0 {
		css.add("letter-spacing", docxNum(*r.espaciado)+"pt")
	}
	if r.posicion != nil && *r.posicion != 0 && r.vert == "" {
		css.add("vertical-align", docxNum(*r.posicion)+"pt")
	}
	if r.borde != nil && *r.borde != "none" {
		css.add("border", *r.borde)
	}
	if r.vanish != nil && *r.vanish {
		css.add("display", "none")
	}
	return css.String()
}

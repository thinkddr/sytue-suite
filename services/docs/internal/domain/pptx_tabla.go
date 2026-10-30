// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"cmp"
	"encoding/xml"
	"html"
	"path"
	"strings"
)

// Tablas y SmartArt de Diapos: lo que va en un `p:graphicFrame`. Se perdían enteros —el texto
// de las tablas era el 12 % del texto de las presentaciones del corpus de terceros—.
//
//   - Una tabla (`a:tbl`, ECMA-376 §21.1.3.13) llega como caja de texto con una tabla HTML:
//     sus celdas combinadas, su relleno, sus bordes y el color y la negrita de su letra, los
//     suyos o los de su estilo de tabla (`ppt/tableStyles.xml`, §20.1.4.2), por partes:
//     toda la tabla, bandas de filas y columnas, primera y última fila y columna, y esquinas.
//   - Un SmartArt llega con las formas de su dibujo ya compuesto (la parte `diagramDrawing`
//     que guarda PowerPoint): cada forma, su caja.
//   - Un gráfico (`c:chart`) llega como tabla con su título, categorías y series; un objeto
//     incrustado (`p:oleObj`), con su imagen de reemplazo o su nombre.

// estiloTabla es un `a:tblStyle`: sus partes por nombre (wholeTbl, band1H, firstRow…).
type estiloTabla map[string]*xnode

// estilosDeTabla lee ppt/tableStyles.xml: cada estilo por su id.
func estilosDeTabla(raw []byte) map[string]estiloTabla {
	var lst xnode
	if len(raw) == 0 || xml.Unmarshal(raw, &lst) != nil {
		return nil
	}
	out := map[string]estiloTabla{}
	for _, st := range lst.Kids {
		if st.XMLName.Local != "tblStyle" {
			continue
		}
		e := estiloTabla{}
		for i := range st.Kids {
			e[st.Kids[i].XMLName.Local] = &st.Kids[i]
		}
		out[st.attr("styleId")] = e
	}
	return out
}

// xmlDeNodo vuelve a escribir un nodo con sus nombres locales, para leerlo con los lectores de
// color y de relleno, que van sobre un decodificador.
func xmlDeNodo(b *bytes.Buffer, n *xnode) {
	b.WriteString("<" + n.XMLName.Local)
	for _, a := range n.Attrs {
		b.WriteString(" " + a.Name.Local + `="`)
		_ = xml.EscapeText(b, []byte(a.Value))
		b.WriteString(`"`)
	}
	b.WriteString(">")
	for i := range n.Kids {
		xmlDeNodo(b, &n.Kids[i])
	}
	b.WriteString("</" + n.XMLName.Local + ">")
}

// decDeNodo es un decodificador sobre el nodo y su elemento de apertura.
func decDeNodo(n *xnode) (*xml.Decoder, xml.StartElement, bool) {
	var b bytes.Buffer
	xmlDeNodo(&b, n)
	dec := xml.NewDecoder(&b)
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, xml.StartElement{}, false
		}
		if se, ok := tok.(xml.StartElement); ok {
			return dec, se, true
		}
	}
}

// rellenoDeNodo es el relleno de un nodo que lo contiene (`a:fill` con su relleno, o un
// `a:fillRef` al tema) o el de un `a:ln`.
func (p *pptxDec) rellenoDeNodo(n *xnode) (pinturaDML, bool) {
	if n == nil {
		return pinturaDML{}, false
	}
	for i := range n.Kids {
		k := &n.Kids[i]
		switch {
		case esRellenoDML(k.XMLName.Local):
			dec, se, ok := decDeNodo(k)
			if !ok {
				return pinturaDML{}, false
			}
			return rellenoDML(dec, se, p.esquema), true
		case k.XMLName.Local == "fillRef" || k.XMLName.Local == "lnRef":
			ph, _ := p.colorDeNodo(k)
			if p.theme == nil {
				return pinturaDML{}, false
			}
			lista := p.theme.rellenos
			if k.XMLName.Local == "lnRef" {
				lista = p.theme.lineas
			}
			return p.rellenoDeRef(lista, atoiDefault(k.attr("idx"), 0), ph)
		case k.XMLName.Local == "ln":
			return p.rellenoDeNodo(k)
		}
	}
	return pinturaDML{}, false
}

// colorDeNodo es el primer color hijo de un nodo.
func (p *pptxDec) colorDeNodo(n *xnode) (rgbDML, bool) {
	for i := range n.Kids {
		if esColorDML(n.Kids[i].XMLName.Local) {
			if dec, se, ok := decDeNodo(&n.Kids[i]); ok {
				return colorDML(dec, se, p.esquema)
			}
		}
	}
	return rgbDML{}, false
}

// celdaPptx es una `a:tc` leída: su celda del modelo de tabla y lo que dice de su aspecto.
type celdaPptx struct {
	xcell
	hMerge, vMerge bool
	relleno        pinturaDML
	rellenoDicho   bool
	bordes         map[string]pinturaDML // lnL, lnR, lnT, lnB que dice la celda
	anchor         string
}

// tablaPptx es una `a:tbl` leída.
type tablaPptx struct {
	filas  [][]celdaPptx
	estilo string          // tableStyleId
	partes map[string]bool // firstRow, bandRow… que la tabla activa
}

// readTbl lee una `a:tbl` hasta su cierre.
func (p *pptxDec) readTbl() tablaPptx {
	t := tablaPptx{partes: map[string]bool{}}
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return t
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "tblPr":
				for _, a := range e.Attr {
					t.partes[a.Name.Local] = verdadero(a.Value)
				}
				continue // descender: tableStyleId
			case "tableStyleId":
				t.estilo = strings.TrimSpace(readElementText(p.dec, "tableStyleId"))
				continue
			case "tr":
				t.filas = append(t.filas, nil)
				continue
			case "tc":
				if len(t.filas) > 0 && len(t.filas[len(t.filas)-1]) < maxColumnasTablaPptx {
					t.filas[len(t.filas)-1] = append(t.filas[len(t.filas)-1], p.readTc(e))
					continue
				}
			}
			_ = p.dec.Skip()
		case xml.EndElement:
			if e.Name.Local == "tbl" {
				return t
			}
		}
	}
}

// maxColumnasTablaPptx acota lo que se lee de una fila (un fichero hostil puede declarar
// cualquier cosa); PowerPoint no pasa de 75.
const maxColumnasTablaPptx = 200

// readTc lee una `a:tc` hasta su cierre: su texto, sus celdas combinadas y su aspecto.
func (p *pptxDec) readTc(se xml.StartElement) celdaPptx {
	c := celdaPptx{bordes: map[string]pinturaDML{}}
	c.colspan = min(maxColumnasTablaPptx, atoiDefault(attrLocal(se, "gridSpan"), 1))
	c.rowspan = min(1000, atoiDefault(attrLocal(se, "rowSpan"), 1))
	c.hMerge, c.vMerge = verdadero(attrLocal(se, "hMerge")), verdadero(attrLocal(se, "vMerge"))
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return c
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Local == "txBody":
				continue
			case t.Name.Local == "p":
				c.paras = append(c.paras, *p.readDrawPara())
				continue
			case t.Name.Local == "tcPr":
				c.anchor = attrLocal(t, "anchor")
				continue
			case strings.HasPrefix(t.Name.Local, "ln") && len(t.Name.Local) == 3:
				r, _ := p.readLnHasta(t.Name.Local)
				c.bordes[t.Name.Local] = r
				continue
			case esRellenoDML(t.Name.Local):
				c.relleno, c.rellenoDicho = p.relleno(t), true
				continue
			}
			_ = p.dec.Skip()
		case xml.EndElement:
			if t.Name.Local == "tc" {
				return c
			}
		}
	}
}

// readLnHasta es readLn para los trazos de una celda (`a:lnL`, `a:lnR`…), que cierran con su
// propio nombre.
func (p *pptxDec) readLnHasta(fin string) (pinturaDML, bool) {
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
			if t.Name.Local == fin {
				return r, dicho
			}
		}
	}
}

// partesDeCelda son las partes del estilo de tabla que tocan a la celda (fila r, columna k
// de nf×nc), de menos a más precedencia (ECMA-376 §20.1.4.2).
func (t tablaPptx) partesDeCelda(r, k, nf, nc int) []string {
	out := []string{"wholeTbl"}
	primeraF, primeraC := t.partes["firstRow"], t.partes["firstCol"]
	// Las bandas se cuentan sin la fila (o la columna) de cabecera.
	banda := func(i int, sinPrimera bool, impar, par string) string {
		if sinPrimera {
			i--
		}
		if i%2 == 0 {
			return impar
		}
		return par
	}
	if t.partes["bandRow"] {
		out = append(out, banda(r, primeraF, "band1H", "band2H"))
	}
	if t.partes["bandCol"] {
		out = append(out, banda(k, primeraC, "band1V", "band2V"))
	}
	ultimaF, ultimaC := t.partes["lastRow"] && r == nf-1, t.partes["lastCol"] && k == nc-1
	primeraF, primeraC = primeraF && r == 0, primeraC && k == 0
	for _, x := range []struct {
		si    bool
		parte string
	}{
		{ultimaC, "lastCol"},
		{primeraC, "firstCol"},
		{ultimaF, "lastRow"},
		{primeraF, "firstRow"},
		{ultimaF && ultimaC, "seCell"},
		{ultimaF && primeraC, "swCell"},
		{primeraF && ultimaC, "neCell"},
		{primeraF && primeraC, "nwCell"},
	} {
		if x.si {
			out = append(out, x.parte)
		}
	}
	return out
}

// tablaHTML compone la tabla del modelo con el aspecto de cada celda en su CSS.
func (p *pptxDec) tablaHTML(t tablaPptx) string {
	estilo := estiloTabla(nil)
	if p.theme != nil {
		estilo = p.theme.tablas[t.estilo]
	}
	nf, nc := len(t.filas), 0
	for _, f := range t.filas {
		nc = max(nc, len(f)) // una a:tc por columna de la rejilla, también las combinadas
	}
	tab := &xtable{cols: nc, css: "border-collapse:collapse;width:100%;font:inherit"} // la letra de la caja, no la del navegador
	for r, f := range t.filas {
		fila := make([]xcell, 0, len(f))
		for k, c := range f {
			x := c.xcell
			x.covered = c.hMerge || c.vMerge
			a := p.aspectoDeCelda(estilo, t.partesDeCelda(r, k, nf, nc), r, k, k+max(x.colspan, 1)-1, nf, nc)
			if c.rellenoDicho {
				a.relleno = c.relleno
			}
			for lado, b := range c.bordes {
				a.bordes[lado] = b
			}
			x.css = p.cssDeCelda(a.relleno, a.bordes, c.anchor)
			for i := range x.paras {
				for j := range x.paras[i].runs {
					run := &x.paras[i].runs[j]
					run.color = cmp.Or(run.color, a.tinta)
					run.b = run.b || a.negrita
				}
			}
			fila = append(fila, x)
		}
		tab.rows = append(tab.rows, fila)
	}
	return tableToHTML(tab)
}

// aspectoCelda es lo que el estilo de tabla da a una celda.
type aspectoCelda struct {
	relleno pinturaDML
	bordes  map[string]pinturaDML
	tinta   string
	negrita bool
}

// aspectoDeCelda recorre las partes del estilo que tocan a la celda, de menos a más
// precedencia: cada una sustituye lo que diga.
func (p *pptxDec) aspectoDeCelda(estilo estiloTabla, partes []string, r, k, kFin, nf, nc int) aspectoCelda {
	a := aspectoCelda{bordes: map[string]pinturaDML{}}
	for _, nombre := range partes {
		parte := estilo[nombre]
		if parte == nil {
			continue
		}
		if tc := parte.kid("tcStyle"); tc != nil {
			if r2, ok := p.rellenoDeNodo(tc.kid("fill")); ok {
				a.relleno = r2
			} else if r2, ok := p.rellenoDeNodo(tc); ok {
				a.relleno = r2
			}
			p.bordesDeEstilo(tc.kid("tcBdr"), r, k, kFin, nf, nc, a.bordes)
		}
		if tx := parte.kid("tcTxStyle"); tx != nil {
			a.tinta = cmp.Or(p.tintaDeEstilo(tx), a.tinta)
			if b := tx.attr("b"); b != "" {
				a.negrita = b == "on"
			}
		}
	}
	return a
}

// tintaDeEstilo es el color de letra de un `a:tcTxStyle`: el suyo o, si no, el de su fontRef.
func (p *pptxDec) tintaDeEstilo(tx *xnode) string {
	if c, ok := p.colorDeNodo(tx); ok {
		return c.hex()
	}
	if fr := tx.kid("fontRef"); fr != nil {
		if c, ok := p.colorDeNodo(fr); ok {
			return c.hex()
		}
	}
	return ""
}

// bordesDeEstilo apunta los bordes que una parte del estilo da a la celda: los de fuera de la
// tabla (left, right, top, bottom) o los de dentro (insideH, insideV), según dónde caiga.
func (p *pptxDec) bordesDeEstilo(bdr *xnode, r, k, kFin, nf, nc int, bordes map[string]pinturaDML) {
	if bdr == nil {
		return
	}
	lados := map[string]string{"lnL": "insideV", "lnR": "insideV", "lnT": "insideH", "lnB": "insideH"}
	if k == 0 {
		lados["lnL"] = "left"
	}
	if kFin == nc-1 {
		lados["lnR"] = "right"
	}
	if r == 0 {
		lados["lnT"] = "top"
	}
	if r == nf-1 {
		lados["lnB"] = "bottom"
	}
	for lado, nombre := range lados {
		if n := bdr.kid(nombre); n != nil {
			if b, ok := p.rellenoDeNodo(n); ok {
				bordes[lado] = b
			}
		}
	}
}

// cssDeCelda es el CSS de una celda: relleno, bordes, márgenes (los de PowerPoint: 0,1 y 0,05
// pulgadas) y alineación vertical.
func (p *pptxDec) cssDeCelda(relleno pinturaDML, bordes map[string]pinturaDML, anchor string) string {
	var b strings.Builder
	if hx := p.hexRelleno(relleno); hx != "" {
		b.WriteString("background:#" + hx + ";")
	}
	for _, l := range [][2]string{{"lnL", "left"}, {"lnR", "right"}, {"lnT", "top"}, {"lnB", "bottom"}} {
		if hx := p.hexRelleno(bordes[l[0]]); hx != "" {
			b.WriteString("border-" + l[1] + ":1px solid #" + hx + ";")
		}
	}
	b.WriteString("padding:5px 10px")
	if v := map[string]string{"ctr": "middle", "b": "bottom"}[anchor]; v != "" {
		b.WriteString(";vertical-align:" + v)
	}
	return b.String()
}

// readGraphicFrame lee un <p:graphicFrame>: una tabla, un gráfico (como tabla con sus series
// y valores), un objeto incrustado (su imagen de reemplazo o su etiqueta) o un SmartArt.
// Devuelve también el id de forma.
func (p *pptxDec) readGraphicFrame() ([]slideBox, string) {
	var nv nvProps
	var pos xfrmPos
	var tabla, dibujo, datos, imgOle string
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "nvGraphicFramePr":
				nv = p.readNv("nvGraphicFramePr")
			case "xfrm":
				pos = p.readXfrm()
			case "graphic", "graphicData", "AlternateContent", "Choice", "Fallback":
				continue
			case "tbl":
				tabla = p.tablaHTML(p.readTbl())
			case "chart":
				if rel := p.rels[attrLocal(t, "id")]; rel != "" {
					tabla = tablaDeGraficoPPTX(p.files[path.Clean(p.dir+"/"+rel)])
				}
				_ = p.dec.Skip()
			case "oleObj":
				p.leerOleMarco(t, &imgOle, &tabla)
			case "relIds":
				dibujo = p.dibujoDeSmartArt(attrLocal(t, "dm"))
				if rel := p.rels[attrLocal(t, "dm")]; rel != "" {
					datos = path.Clean(p.dir + "/" + rel)
				}
				_ = p.dec.Skip()
			default:
				_ = p.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == "graphicFrame" {
				return p.cajasDeMarco(nv, pos, tabla, dibujo, datos, imgOle), nv.id
			}
		}
	}
	return nil, ""
}

func (p *pptxDec) leerOleMarco(se xml.StartElement, imgOle, tabla *string) {
	var ole xnode
	if p.dec.DecodeElement(&ole, &se) != nil {
		return
	}
	if src := p.imagenDeOle(&ole); src != "" && *imgOle == "" {
		*imgOle = src
	} else if nom := cmp.Or(ole.attr("name"), ole.attr("progId")); nom != "" && *tabla == "" {
		*tabla = "<p>" + html.EscapeString(nom) + "</p>"
	}
}

func (p *pptxDec) imagenDeOle(n *xnode) string {
	if n == nil {
		return ""
	}
	for _, k := range []string{"embed", "id", "relid"} {
		if rid := n.attr(k); rid != "" && p.rels[rid] != "" {
			if src := dataURLFromZip(path.Clean(p.dir+"/"+p.rels[rid]), p.files); src != "" {
				return src
			}
		}
	}
	for i := range n.Kids {
		if src := p.imagenDeOle(&n.Kids[i]); src != "" {
			return src
		}
	}
	return ""
}

func textoNodoChart(n *xnode) string {
	if n == nil {
		return ""
	}
	var partes []string
	var rec func(*xnode)
	rec = func(x *xnode) {
		if x.XMLName.Local == "t" || x.XMLName.Local == "v" {
			if s := strings.TrimSpace(x.Text); s != "" {
				partes = append(partes, s)
			}
			return
		}
		for i := range x.Kids {
			rec(&x.Kids[i])
		}
	}
	rec(n)
	return strings.Join(partes, " ")
}

func puntosCacheChart(n *xnode) []string {
	if n == nil {
		return nil
	}
	var out []string
	var rec func(*xnode)
	rec = func(x *xnode) {
		if x.XMLName.Local == "pt" {
			if v := x.kid("v"); v != nil {
				out = append(out, strings.TrimSpace(v.Text))
			}
			return
		}
		for i := range x.Kids {
			rec(&x.Kids[i])
		}
	}
	rec(n)
	return out
}

func tablaDeGraficoPPTX(raw []byte) string {
	var root xnode
	if len(raw) == 0 || xml.Unmarshal(raw, &root) != nil {
		return ""
	}
	ch := root.kid("chart")
	if ch == nil {
		ch = &root
	}
	titulo := textoNodoChart(ch.kid("title"))
	plot := ch.kid("plotArea")
	if plot == nil {
		return ""
	}
	var nombresSeries, cats []string
	var series [][]celda09
	for i := range plot.Kids {
		ck := &plot.Kids[i]
		if !strings.HasSuffix(ck.XMLName.Local, "Chart") {
			continue
		}
		for j := range ck.Kids {
			ser := &ck.Kids[j]
			if ser.XMLName.Local != "ser" {
				continue
			}
			nombresSeries = append(nombresSeries, textoNodoChart(ser.kid("tx")))
			if len(cats) == 0 {
				cats = puntosCacheChart(cmp.Or(ser.kid("cat"), ser.kid("xVal")))
			}
			vals := puntosCacheChart(cmp.Or(ser.kid("val"), ser.kid("yVal")))
			fila := make([]celda09, len(vals))
			for k, v := range vals {
				fila[k] = celda09{texto: v}
			}
			series = append(series, fila)
		}
	}
	nr := len(cats)
	for _, s := range series {
		nr = max(nr, len(s))
	}
	datos := make([][]celda09, nr)
	for r := range datos {
		datos[r] = make([]celda09, len(series))
		for c, s := range series {
			if r < len(s) {
				datos[r][c] = s[r]
			}
		}
	}
	t := armarTablaGrafico(titulo, cats, nombresSeries, datos)
	if t == nil {
		return ""
	}
	return blocksToHTML([]xblock{{table: t.xtable()}})
}

// cajasDeMarco son las cajas de un marco: la de su tabla o las de su SmartArt, en su sitio.
func (p *pptxDec) cajasDeMarco(nv nvProps, pos xfrmPos, tabla, dibujo, datos, imgOle string) []slideBox {
	if nv.oculta || p.master && nv.ph || p.theme != nil && p.theme.soloMarcadores {
		return nil
	}
	if imgOle != "" {
		return []slideBox{{
			Kind: "image", Src: imgOle,
			X: pctOf(pos.x, p.wEMU), Y: pctOf(pos.y, p.hEMU),
			W: max(pctOf(pos.cx, p.wEMU), 10), H: max(pctOf(pos.cy, p.hEMU), 10),
		}}
	}
	if tabla != "" {
		return []slideBox{buildShapeBox(pos, true, "rect", "", "", tabla, p.wEMU, p.hEMU)}
	}
	raw := p.files[dibujo]
	if len(raw) == 0 {
		// Sin dibujo compuesto (los de PowerPoint 2007 y los de otras herramientas): su texto,
		// nodo a nodo, como lista en el sitio del marco. La disposición no se reproduce.
		if lista := listaDeSmartArt(p.files[datos]); lista != "" {
			return []slideBox{buildShapeBox(pos, true, "rect", "", "", lista, p.wEMU, p.hEMU)}
		}
		return nil
	}
	if p.theme == nil || p.theme.enSmartArt {
		return nil
	}
	// Las formas del dibujo van en el espacio del marco: se llevan a su sitio.
	p.theme.enSmartArt = true
	sl := parseSlide("d", raw, path.Dir(dibujo), relsDe(p.files, dibujo), p.files, p.wEMU, p.hEMU, p.theme, false)
	p.theme.enSmartArt = false
	for i := range sl.Boxes {
		sl.Boxes[i].X += pctOf(pos.x, p.wEMU)
		sl.Boxes[i].Y += pctOf(pos.y, p.hEMU)
		sl.Boxes[i].ID = ""
	}
	return sl.Boxes
}

// dibujoDeSmartArt es la parte del dibujo ya compuesto de un SmartArt: la que cita su parte
// de datos (`dsp:dataModelExt relId`, una relación de la diapositiva) o, si no, la única de la
// diapositiva.
func (p *pptxDec) dibujoDeSmartArt(rDatos string) string {
	datos := p.files[path.Clean(p.dir+"/"+p.rels[rDatos])]
	if i := bytes.Index(datos, []byte("dataModelExt")); i >= 0 {
		var e struct {
			RelID string `xml:"relId,attr"`
		}
		j := bytes.LastIndexByte(datos[:i], '<')
		if dec := xml.NewDecoder(bytes.NewReader(datos[j:])); j >= 0 && dec.Decode(&e) == nil && p.rels[e.RelID] != "" {
			return path.Clean(p.dir + "/" + p.rels[e.RelID])
		}
	}
	unico := ""
	for _, t := range p.rels {
		if strings.Contains(t, "diagrams/drawing") {
			if unico != "" {
				return "" // varios SmartArt y ninguno dice el suyo
			}
			unico = path.Clean(p.dir + "/" + t)
		}
	}
	return unico
}

// listaDeSmartArt es el texto de los nodos de la parte de datos de un SmartArt (`dgm:pt` sin
// tipo o de tipo «node»; los demás son el documento, las conexiones y la presentación), como
// lista HTML; "" si no hay.
func listaDeSmartArt(raw []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var b strings.Builder
	nodo, enT := false, false
	var par strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "pt":
				tipo := attrLocal(t, "type")
				nodo = tipo == "" || tipo == "node"
			case "t":
				enT = nodo
			}
		case xml.CharData:
			if enT {
				par.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				enT = false
			case "p":
				if s := strings.TrimSpace(par.String()); s != "" && nodo {
					b.WriteString("<li>" + html.EscapeString(s) + "</li>")
				}
				par.Reset()
			}
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return "<ul>" + b.String() + "</ul>"
}

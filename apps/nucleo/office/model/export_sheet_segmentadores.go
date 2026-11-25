// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Exportación de los segmentadores (slicers) a la forma de Excel: lo que la importación
// (import_sheet_segmentadores.go) lee, escrito con las mismas referencias, sin tomar código
// de nadie:
//
//   - [MS-XLSX] «Extensions to SpreadsheetML»: CT_SlicerCacheDefinition (`name`,
//     `sourceName`; `pivotTables` y `data/tabular` para los de tabla dinámica; la
//     extensión {2F2917AC-EB37-4324-AD4E-5DD8C200BD13} con CT_TableSlicerCache para los de
//     tabla), CT_SlicerCaches en el extLst del libro ({BBE1A952-AA13-448e-AADC-164F8A28A991}
//     los de dinámica, {46BE6895-7355-4a93-B00E-2C351335B9C9} los de tabla), CT_Slicers y
//     CT_SlicerList en el extLst de la hoja ({A8765BA9-456A-4dab-B4F3-ACF838C121DE} los de
//     dinámica, {3A4CF648-6AED-40f4-86FF-DC5316D8AED3} los de tabla) y
//     x14:pivotCacheDefinition@pivotCacheId ({725AE2AE-9491-48be-B2B4-4EB974FC3084}), que es
//     el que cita `tabular@pivotCacheId`.
//   - ECMA-376 Parte 1 §18.2.5 (un `definedName` `Slicer_*` con `#N/A` por caché, como los
//     escribe Excel), §18.5.1.2 (la tabla y el `id` de sus columnas) y §20.5.2 (el ancla del
//     dibujo, aquí dentro de un mc:AlternateContent que solo entiende Excel 2010+).
//
// Un segmentador de Sytue filtra el autofiltro de su hoja (o una tabla dinámica). Excel solo
// sabe segmentar una TABLA o una dinámica, así que sale a Excel el de una columna que cae
// dentro de una tabla con cabecera, y el de una dinámica que se exporta; los demás siguen
// viajando solo en la extensión de Sytue (`sy:slicers`), como hasta ahora, que además lleva
// su id. La extensión va siempre: con ella la importación los reconoce sin mirar estos.
//
// ponytail: escrito contra la especificación y la importación de Sytue; falta abrirlo en
// Excel (lo hará el dueño). Si Excel lo repara, el sitio a mirar es la caché de la dinámica
// (sus elementos y el x14:pivotCacheDefinition).

const (
	nsX15        = "http://schemas.microsoft.com/office/spreadsheetml/2010/11/main"
	nsMC         = "http://schemas.openxmlformats.org/markup-compatibility/2006"
	nsSle        = "http://schemas.microsoft.com/office/drawing/2010/slicer"
	nsSle15      = "http://schemas.microsoft.com/office/drawing/2012/slicer"
	relSlicer    = "http://schemas.microsoft.com/office/2007/relationships/slicer"
	relSlicerCch = "http://schemas.microsoft.com/office/2007/relationships/slicerCache"
	ctSlicer     = "application/vnd.ms-excel.slicer+xml"
	ctSlicerCch  = "application/vnd.ms-excel.slicerCache+xml"

	uriCachesDinamica = "{BBE1A952-AA13-448e-AADC-164F8A28A991}"
	uriCachesTabla    = "{46BE6895-7355-4a93-B00E-2C351335B9C9}"
	uriListaDinamica  = "{A8765BA9-456A-4dab-B4F3-ACF838C121DE}"
	uriListaTabla     = "{3A4CF648-6AED-40f4-86FF-DC5316D8AED3}"
	uriCacheDeTabla   = "{2F2917AC-EB37-4324-AD4E-5DD8C200BD13}"
	uriPivotCacheX14  = "{725AE2AE-9491-48be-B2B4-4EB974FC3084}"

	// La tarjeta de un segmentador nuevo en Excel: 1,83 × 2,52 cm a 96 ppp (en px).
	anchoTarjetaPx = 192.0
	altoTarjetaPx  = 265.0
	// rowHeight de Excel para cada botón: 0,67 cm en EMU.
	altoBotonEMU = 241300
)

// segmentadorXlsx es un segmentador que sale a Excel, con todo lo que sus partes citan.
type segmentadorXlsx struct {
	nombre, cache, fuente string // la tarjeta, su caché `Slicer_*` y la columna o el campo
	x, y                  float64
	// De tabla: el `id` de la tabla exportada y el de su columna (1-based).
	tablaID, columna int
	// De dinámica: la dinámica, su campo, la hoja (sheetId), el pivotCacheId y, por elemento
	// compartido del campo, si está marcado.
	dinamica      string
	campo         int
	tabID         int
	pivotCacheID  int
	seleccionados []bool
}

func (s segmentadorXlsx) deTabla() bool { return s.dinamica == "" }

// planSegmentadores decide qué segmentadores de cada hoja salen a Excel, con los nombres
// únicos en el libro. Necesita las dinámicas tal como las exporta addXlsxPivots (su caché)
// y numera las tablas como objetosXlsx (el `id` de cada tabla).
func planSegmentadores(m *sheetModel, pivotes []pivotePlaneado) [][]segmentadorXlsx {
	out := make([][]segmentadorXlsx, len(m.Sheets))
	nombres, caches := map[string]bool{}, map[string]bool{}
	tablaID := 0
	for i, sd := range m.Sheets {
		ids := make([]int, len(sd.Tables))
		for k, t := range sd.Tables {
			if _, ok := leerRango(t.Range); ok {
				tablaID++
				ids[k] = tablaID
			}
		}
		for _, sl := range sd.Slicers {
			if len(out[i]) >= maxSegmentadores {
				break
			}
			var s segmentadorXlsx
			var ok bool
			if sl.Pivot == "" {
				s, ok = segmentadorDeTablaXlsx(sd, sl, ids)
			} else {
				s, ok = segmentadorDeDinamicaXlsx(i, sl, pivotes)
			}
			if !ok {
				continue
			}
			s.nombre = unico(s.fuente, nombres, " ")
			s.cache = unico("Slicer_"+nombreDefinible(s.fuente), caches, "")
			if sl.X != nil && sl.Y != nil {
				s.x, s.y = max(*sl.X, 0), max(*sl.Y, 0)
			}
			out[i] = append(out[i], s)
		}
	}
	return out
}

// segmentadorDeTablaXlsx es el de una columna dentro de una tabla con cabecera, o nada.
func segmentadorDeTablaXlsx(sd sheetData, sl sheetSlicer, ids []int) (segmentadorXlsx, bool) {
	for k, t := range sd.Tables {
		r, ok := leerRango(t.Range)
		col := sl.Col + 1
		if !ok || ids[k] == 0 || col < r.c1 || col > r.c2 || (t.Headers != nil && !*t.Headers) {
			continue
		}
		cols := columnasDeTabla(sd, t, r)
		s := segmentadorXlsx{fuente: cols[col-r.c1], tablaID: ids[k], columna: col - r.c1 + 1}
		// Sin posición, a la derecha de la tabla, como la deja Excel.
		s.x, s.y = float64(r.c2+1)*anchoCeldaPx, float64(r.r1-1)*altoCeldaPx
		return s, true
	}
	return segmentadorXlsx{}, false
}

// segmentadorDeDinamicaXlsx es el de un campo de una dinámica de la hoja que se exporta.
func segmentadorDeDinamicaXlsx(hoja int, sl sheetSlicer, pivotes []pivotePlaneado) (segmentadorXlsx, bool) {
	for _, pv := range pivotes {
		if pv.hoja != hoja || pv.p.Name != sl.Pivot || sl.Col < 0 || sl.Col >= len(pv.names) {
			continue
		}
		_, keys := distinctItems(pv.cols[sl.Col])
		var ocultos []string
		for _, f := range slices.Concat(pv.p.Rows, pv.p.Cols, pv.p.Filters, pv.p.HiddenFilters) {
			if f.Field == sl.Col {
				ocultos = append(ocultos, f.Hidden...)
			}
		}
		marcados := make([]bool, len(keys))
		for x, k := range keys {
			marcados[x] = !containsStr(ocultos, k)
		}
		s := segmentadorXlsx{
			fuente: pv.names[sl.Col], dinamica: pv.p.Name, campo: sl.Col, tabID: hoja + 1,
			pivotCacheID: pv.cacheID, seleccionados: marcados,
		}
		if r1, _, _, c2, ok := parseA1Range(strings.ToUpper(pv.p.Range)); ok {
			s.x, s.y = float64(c2+1)*anchoCeldaPx, float64(r1-1)*altoCeldaPx
		}
		return s, true
	}
	return segmentadorXlsx{}, false
}

// unico es `base` o, si ya está, con un número detrás (`sep` entre los dos): Excel exige
// nombres de tarjeta y de caché únicos en el libro.
func unico(base string, vistos map[string]bool, sep string) string {
	if base == "" {
		base = "Segmentador"
	}
	n, k := base, 1
	for vistos[strings.ToUpper(n)] {
		n = base + sep + strconv.Itoa(k)
		k++
	}
	vistos[strings.ToUpper(n)] = true
	return n
}

// nombreDefinible deja una columna en algo que vale como nombre definido: letras, cifras,
// `_` y `.`; lo demás, `_`.
func nombreDefinible(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// listasSegmentadoresXlsx añade a la hoja las partes de sus segmentadores (una para los de
// tabla y otra para los de dinámica, como Excel), sus relaciones y sus <ext> del extLst.
func listasSegmentadoresXlsx(segs []segmentadorXlsx, cuenta *contadorObjetos, p *partesObjetos, rid func(tipo, destino string) string) (extDinamica, extTabla string) {
	for _, deTabla := range []bool{false, true} {
		var lista strings.Builder
		for _, s := range segs {
			if s.deTabla() != deTabla {
				continue
			}
			lista.WriteString(`<slicer name="` + xmlEscape(s.nombre) + `" cache="` + xmlEscape(s.cache) +
				`" caption="` + xmlEscape(s.fuente) + `" rowHeight="` + strconv.Itoa(altoBotonEMU) + `"/>`)
		}
		if lista.Len() == 0 {
			continue
		}
		cuenta.segmentadores++
		n := strconv.Itoa(cuenta.segmentadores)
		ruta := "xl/slicers/slicer" + n + ".xml"
		p.partes[ruta] = xmlHeader + `<slicers xmlns="` + nsX14 + `" xmlns:mc="` + nsMC + `" mc:Ignorable="x" xmlns:x="` + nsMainX + `">` +
			lista.String() + `</slicers>`
		p.overrides = append(p.overrides, `<Override PartName="/`+ruta+`" ContentType="`+ctSlicer+`"/>`)
		id := rid(relSlicer, "../slicers/slicer"+n+".xml")
		if deTabla {
			extTabla = `<ext uri="` + uriListaTabla + `" xmlns:x15="` + nsX15 + `"><x15:slicerList xmlns:x14="` + nsX14 +
				`"><x14:slicer r:id="` + id + `"/></x15:slicerList></ext>`
		} else {
			extDinamica = `<ext uri="` + uriListaDinamica + `" xmlns:x14="` + nsX14 + `"><x14:slicerList><x14:slicer r:id="` +
				id + `"/></x14:slicerList></ext>`
		}
	}
	return extDinamica, extTabla
}

// anclasSegmentadoresXlsx son las anclas del dibujo de la hoja: una tarjeta por
// segmentador en su posición, dentro del mc:AlternateContent que escribe Excel.
func anclasSegmentadoresXlsx(segs []segmentadorXlsx, primerID int) string {
	var b strings.Builder
	for i, s := range segs {
		requiere, ns := "sle15", nsSle15
		if !s.deTabla() {
			requiere, ns = "sle", nsSle
		}
		col, fila := int(s.x)/anchoCeldaPx, int(s.y)/altoCeldaPx
		b.WriteString(`<mc:AlternateContent xmlns:mc="` + nsMC + `"><mc:Choice xmlns:` + requiere + `="` + ns + `" Requires="` + requiere + `">` +
			`<xdr:oneCellAnchor><xdr:from><xdr:col>` + strconv.Itoa(col) + `</xdr:col><xdr:colOff>` +
			strconv.FormatInt(pxAEmu(s.x-float64(col*anchoCeldaPx)), 10) + `</xdr:colOff><xdr:row>` + strconv.Itoa(fila) +
			`</xdr:row><xdr:rowOff>` + strconv.FormatInt(pxAEmu(s.y-float64(fila*altoCeldaPx)), 10) + `</xdr:rowOff></xdr:from>` +
			`<xdr:ext cx="` + strconv.FormatInt(pxAEmu(anchoTarjetaPx), 10) + `" cy="` + strconv.FormatInt(pxAEmu(altoTarjetaPx), 10) + `"/>` +
			`<xdr:graphicFrame macro=""><xdr:nvGraphicFramePr><xdr:cNvPr id="` + strconv.Itoa(primerID+i) + `" name="` +
			xmlEscape(s.nombre) + `"/><xdr:cNvGraphicFramePr/></xdr:nvGraphicFramePr>` +
			`<xdr:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></xdr:xfrm><a:graphic><a:graphicData uri="` + nsSle + `">` +
			`<sle:slicer xmlns:sle="` + nsSle + `" name="` + xmlEscape(s.nombre) + `"/></a:graphicData></a:graphic>` +
			`</xdr:graphicFrame><xdr:clientData/></xdr:oneCellAnchor></mc:Choice><mc:Fallback/></mc:AlternateContent>`)
	}
	return b.String()
}

// cachesSegmentadoresXlsx añade al libro las cachés de los segmentadores: sus partes, sus
// relaciones, sus tipos, sus `Slicer_*` y las listas del extLst del libro. Va DESPUÉS de
// addXlsxPivots: el extLst cierra el libro, tras las <pivotCaches>.
func cachesSegmentadoresXlsx(files map[string]string, plan [][]segmentadorXlsx) {
	var rels, tipos, nombres, deDinamica, deTabla strings.Builder
	n := 0
	for _, segs := range plan {
		for _, s := range segs {
			n++
			k := strconv.Itoa(n)
			ruta := "xl/slicerCaches/slicerCache" + k + ".xml"
			files[ruta] = cacheSegmentadorXlsx(s)
			id := "rIdSc" + k
			rels.WriteString(`<Relationship Id="` + id + `" Type="` + relSlicerCch + `" Target="slicerCaches/slicerCache` + k + `.xml"/>`)
			tipos.WriteString(`<Override PartName="/` + ruta + `" ContentType="` + ctSlicerCch + `"/>`)
			nombres.WriteString(`<definedName name="` + xmlEscape(s.cache) + `">#N/A</definedName>`)
			if s.deTabla() {
				deTabla.WriteString(`<x14:slicerCache r:id="` + id + `"/>`)
			} else {
				deDinamica.WriteString(`<x14:slicerCache r:id="` + id + `"/>`)
			}
		}
	}
	if n == 0 {
		return
	}
	var ext strings.Builder
	if deDinamica.Len() > 0 {
		ext.WriteString(`<ext uri="` + uriCachesDinamica + `" xmlns:x14="` + nsX14 + `"><x14:slicerCaches>` + deDinamica.String() + `</x14:slicerCaches></ext>`)
	}
	if deTabla.Len() > 0 {
		ext.WriteString(`<ext uri="` + uriCachesTabla + `" xmlns:x15="` + nsX15 + `"><x15:slicerCaches xmlns:x14="` + nsX14 + `">` +
			deTabla.String() + `</x15:slicerCaches></ext>`)
	}
	wb := files["xl/workbook.xml"]
	if strings.Contains(wb, "</definedNames>") {
		wb = strings.Replace(wb, "</definedNames>", nombres.String()+"</definedNames>", 1)
	} else {
		wb = strings.Replace(wb, "<calcPr ", "<definedNames>"+nombres.String()+"</definedNames><calcPr ", 1)
	}
	files["xl/workbook.xml"] = strings.Replace(wb, "</workbook>", "<extLst>"+ext.String()+"</extLst></workbook>", 1)
	files["xl/_rels/workbook.xml.rels"] = strings.Replace(files["xl/_rels/workbook.xml.rels"], "</Relationships>", rels.String()+"</Relationships>", 1)
	files["[Content_Types].xml"] = strings.Replace(files["[Content_Types].xml"], "</Types>", tipos.String()+"</Types>", 1)
}

// cacheSegmentadorXlsx es la parte de la caché de un segmentador.
func cacheSegmentadorXlsx(s segmentadorXlsx) string {
	var b strings.Builder
	b.WriteString(xmlHeader + `<slicerCacheDefinition xmlns="` + nsX14 + `" xmlns:mc="` + nsMC + `" mc:Ignorable="x x15" xmlns:x="` + nsMainX +
		`" xmlns:x15="` + nsX15 + `" name="` + xmlEscape(s.cache) + `" sourceName="` + xmlEscape(s.fuente) + `">`)
	if s.deTabla() {
		b.WriteString(`<extLst><x:ext uri="` + uriCacheDeTabla + `"><x15:tableSlicerCache tableId="` + strconv.Itoa(s.tablaID) +
			`" column="` + strconv.Itoa(s.columna) + `"/></x:ext></extLst></slicerCacheDefinition>`)
		return b.String()
	}
	b.WriteString(`<pivotTables><pivotTable tabId="` + strconv.Itoa(s.tabID) + `" name="` + xmlEscape(s.dinamica) + `"/></pivotTables>` +
		`<data><tabular pivotCacheId="` + strconv.Itoa(s.pivotCacheID) + `"><items count="` + strconv.Itoa(len(s.seleccionados)) + `">`)
	for x, sel := range s.seleccionados {
		b.WriteString(`<i x="` + strconv.Itoa(x) + `"`)
		if sel {
			b.WriteString(` s="1"`)
		}
		b.WriteString(`/>`)
	}
	b.WriteString(`</items></tabular></data></slicerCacheDefinition>`)
	return b.String()
}

// camposSegmentados son, por caché de dinámica, los campos que segmenta algún segmentador:
// la caché y la dinámica los escriben con sus elementos, que son los que cita la caché del
// segmentador.
func camposSegmentados(plan [][]segmentadorXlsx) map[int]map[int]bool {
	out := map[int]map[int]bool{}
	for _, segs := range plan {
		for _, s := range segs {
			if s.deTabla() {
				continue
			}
			if out[s.pivotCacheID] == nil {
				out[s.pivotCacheID] = map[int]bool{}
			}
			out[s.pivotCacheID][s.campo] = true
		}
	}
	return out
}

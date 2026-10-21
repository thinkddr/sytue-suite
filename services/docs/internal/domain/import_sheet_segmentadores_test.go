// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

// Fixtures ORIGINALES escritos a mano con la forma que fijan [MS-XLSX] y ECMA-376 para un
// libro con un segmentador de tabla y otro de tabla dinámica (ver import_sheet_segmentadores.go).
const (
	sgMain = `http://schemas.openxmlformats.org/spreadsheetml/2006/main`
	sgRel  = `http://schemas.openxmlformats.org/officeDocument/2006/relationships`
	sgX14  = `http://schemas.microsoft.com/office/spreadsheetml/2009/9/main`
	sgX15  = `http://schemas.microsoft.com/office/spreadsheetml/2010/11/main`
	sgTipo = `http://schemas.openxmlformats.org/officeDocument/2006/relationships/`
)

func relsSeg(pares ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i := 0; i+2 < len(pares)+0; i += 3 {
		b.WriteString(`<Relationship Id="` + pares[i] + `" Type="` + pares[i+1] + `" Target="` + pares[i+2] + `"/>`)
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

func celdaTexto(ref, t string) string {
	return `<c r="` + ref + `" t="inlineStr"><is><t>` + t + `</t></is></c>`
}

// libroConSegmentadores: hoja «Datos» con la tabla Ventas (A1:C4, id 1), la dinámica
// «Dinamica1» (E1:F3) y dos segmentadores: «Producto» (tabla, columna de id 2) y «Zona»
// (dinámica, campo 0).
func libroConSegmentadores(mod func(map[string]string)) map[string]string {
	f := map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`,
		"_rels/.rels":         relsSeg("rId1", sgTipo+"officeDocument", "xl/workbook.xml"),
		"xl/workbook.xml": `<workbook xmlns="` + sgMain + `" xmlns:r="` + sgRel + `"><sheets><sheet name="Datos" sheetId="1" r:id="rId1"/></sheets>` +
			`<definedNames><definedName name="Slicer_Producto">#N/A</definedName><definedName name="Slicer_Zona">#N/A</definedName><definedName name="Total">Datos!$C$2</definedName></definedNames>` +
			`<extLst><ext uri="{BBE1A952-AA13-448e-AADC-164F8A28A991}" xmlns:x14="` + sgX14 + `"><x14:slicerCaches><x14:slicerCache r:id="rId10"/><x14:slicerCache r:id="rId11"/></x14:slicerCaches></ext></extLst></workbook>`,
		"xl/_rels/workbook.xml.rels": relsSeg("rId1", sgTipo+"worksheet", "worksheets/sheet1.xml",
			"rId10", "http://schemas.microsoft.com/office/2007/relationships/slicerCache", "slicerCaches/slicerCache1.xml",
			"rId11", "http://schemas.microsoft.com/office/2007/relationships/slicerCache", "slicerCaches/slicerCache2.xml"),
		"xl/worksheets/sheet1.xml": `<worksheet xmlns="` + sgMain + `" xmlns:r="` + sgRel + `"><sheetData>` +
			`<row r="1">` + celdaTexto("A1", "Zona") + celdaTexto("B1", "Producto") + celdaTexto("C1", "Importe") + `</row>` +
			`<row r="2">` + celdaTexto("A2", "Norte") + celdaTexto("B2", "Sillas") + `<c r="C2"><v>10</v></c></row>` +
			`<row r="3">` + celdaTexto("A3", "Sur") + celdaTexto("B3", "Mesas") + `<c r="C3"><v>20</v></c></row>` +
			`<row r="4">` + celdaTexto("A4", "Sur") + celdaTexto("B4", "Sillas") + `<c r="C4"><v>30</v></c></row></sheetData>` +
			`<drawing r:id="rId2"/><tableParts count="1"><tablePart r:id="rId3"/></tableParts>` +
			`<extLst><ext uri="{A8765BA9-456A-4dab-B4F3-ACF838C121DE}" xmlns:x14="` + sgX14 + `"><x14:slicerList><x14:slicer r:id="rId4"/></x14:slicerList></ext>` +
			`<ext uri="{3A4CF648-6AED-40f4-86FF-DC5316D8AED3}" xmlns:x15="` + sgX15 + `" xmlns:x14="` + sgX14 + `"><x15:slicerList><x14:slicer r:id="rId5"/></x15:slicerList></ext></extLst></worksheet>`,
		"xl/worksheets/_rels/sheet1.xml.rels": relsSeg("rId2", sgTipo+"drawing", "../drawings/drawing1.xml",
			"rId3", sgTipo+"table", "../tables/table1.xml",
			"rId4", "http://schemas.microsoft.com/office/2007/relationships/slicer", "../slicers/slicer1.xml",
			"rId5", "http://schemas.microsoft.com/office/2007/relationships/slicer", "../slicers/slicer2.xml",
			"rId6", sgTipo+"pivotTable", "../pivotTables/pivotTable1.xml"),
		"xl/tables/table1.xml": `<table xmlns="` + sgMain + `" id="1" name="Ventas" displayName="Ventas" ref="A1:C4"><autoFilter ref="A1:C4"/>` +
			`<tableColumns count="3"><tableColumn id="1" name="Zona"/><tableColumn id="2" name="Producto"/><tableColumn id="3" name="Importe"/></tableColumns></table>`,
		"xl/pivotTables/pivotTable1.xml": `<pivotTableDefinition xmlns="` + sgMain + `" name="Dinamica1" cacheId="1"><location ref="E1:F3" firstHeaderRow="1" firstDataRow="1" firstDataCol="1"/>` +
			`<pivotFields count="3"><pivotField axis="axisRow" showAll="0"><items count="3"><item x="0"/><item x="1"/><item t="default"/></items></pivotField><pivotField showAll="0"/><pivotField dataField="1" showAll="0"/></pivotFields>` +
			`<rowFields count="1"><field x="0"/></rowFields><dataFields count="1"><dataField name="Suma" fld="2"/></dataFields></pivotTableDefinition>`,
		"xl/pivotTables/_rels/pivotTable1.xml.rels": relsSeg("rId1", sgTipo+"pivotCacheDefinition", "../pivotCache/pivotCacheDefinition1.xml"),
		"xl/pivotCache/pivotCacheDefinition1.xml": `<pivotCacheDefinition xmlns="` + sgMain + `"><cacheSource type="worksheet"><worksheetSource ref="A1:C4" sheet="Datos"/></cacheSource>` +
			`<cacheFields count="3"><cacheField name="Zona"><sharedItems count="2"><s v="Norte"/><s v="Sur"/></sharedItems></cacheField><cacheField name="Producto"><sharedItems/></cacheField><cacheField name="Importe"><sharedItems/></cacheField></cacheFields></pivotCacheDefinition>`,
		"xl/slicers/slicer1.xml": `<slicers xmlns="` + sgX14 + `"><slicer name="Producto" cache="Slicer_Producto" caption="Producto" rowHeight="241300"/></slicers>`,
		"xl/slicers/slicer2.xml": `<slicers xmlns="` + sgX14 + `"><slicer name="Zona" cache="Slicer_Zona" caption="Zona" rowHeight="241300"/></slicers>`,
		"xl/slicerCaches/slicerCache1.xml": `<slicerCacheDefinition xmlns="` + sgX14 + `" xmlns:x15="` + sgX15 + `" name="Slicer_Producto" sourceName="Producto">` +
			`<extLst><ext uri="{2F2917AC-EB37-4324-AD4E-5DD8C200BD13}"><x15:tableSlicerCache tableId="1" column="2"/></ext></extLst></slicerCacheDefinition>`,
		"xl/slicerCaches/slicerCache2.xml": `<slicerCacheDefinition xmlns="` + sgX14 + `" name="Slicer_Zona" sourceName="Zona"><pivotTables><pivotTable tabId="1" name="Dinamica1"/></pivotTables>` +
			`<data><tabular pivotCacheId="1"><items count="2"><i x="0" s="1"/><i x="1" s="1"/></items></tabular></data></slicerCacheDefinition>`,
		"xl/drawings/drawing1.xml": `<xdr:wsDr xmlns:xdr="http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">` +
			`<mc:AlternateContent xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"><mc:Choice xmlns:sle="http://schemas.microsoft.com/office/drawing/2010/slicer" Requires="sle">` +
			`<xdr:twoCellAnchor><xdr:from><xdr:col>8</xdr:col><xdr:colOff>0</xdr:colOff><xdr:row>2</xdr:row><xdr:rowOff>0</xdr:rowOff></xdr:from><xdr:to><xdr:col>10</xdr:col><xdr:colOff>0</xdr:colOff><xdr:row>12</xdr:row><xdr:rowOff>0</xdr:rowOff></xdr:to>` +
			`<xdr:graphicFrame macro=""><xdr:nvGraphicFramePr><xdr:cNvPr id="2" name="Producto"/><xdr:cNvGraphicFramePr/></xdr:nvGraphicFramePr><xdr:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></xdr:xfrm>` +
			`<a:graphic><a:graphicData uri="http://schemas.microsoft.com/office/drawing/2010/slicer"><sle:slicer name="Producto"/></a:graphicData></a:graphic></xdr:graphicFrame><xdr:clientData/></xdr:twoCellAnchor></mc:Choice></mc:AlternateContent></xdr:wsDr>`,
	}
	if mod != nil {
		mod(f)
	}
	return f
}

type hojaSeg struct {
	Slicers []sheetSlicer `json:"slicers"`
	Names   []definedName `json:"-"`
}

func importarSegmentadores(t *testing.T, mod func(map[string]string)) (hojaSeg, string) {
	t.Helper()
	_, doc, err := importXlsx(zipStore(t, libroConSegmentadores(mod)), nil)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Sheets []hojaSeg `json:"sheets"`
	}
	if err := json.Unmarshal([]byte(doc), &m); err != nil || len(m.Sheets) != 1 {
		t.Fatalf("modelo ilegible: %v\n%s", err, doc)
	}
	return m.Sheets[0], doc
}

func TestImportaSegmentadoresDeExcel(t *testing.T) {
	h, doc := importarSegmentadores(t, nil)
	if len(h.Slicers) != 2 {
		t.Fatalf("quiero 2 segmentadores, hay %d: %+v\n%s", len(h.Slicers), h.Slicers, doc)
	}
	tabla, din := h.Slicers[0], h.Slicers[1]
	if tabla.Col != 1 || tabla.Pivot != "" {
		t.Errorf("segmentador de tabla = %+v, quiero col 1 (Producto)", tabla)
	}
	if tabla.X == nil || *tabla.X != 8*anchoCeldaPx || *tabla.Y != 2*altoCeldaPx {
		t.Errorf("posición de la tarjeta = %v,%v", tabla.X, tabla.Y)
	}
	if din.Col != 0 || din.Pivot != "Dinamica1" {
		t.Errorf("segmentador de dinámica = %+v, quiero campo 0 de Dinamica1", din)
	}
	if !strings.Contains(doc, `"autofilter":{"row":0,"c1":0,"c2":2`) {
		t.Errorf("el autofiltro debe cubrir la tabla segmentada:\n%s", doc)
	}
	if strings.Contains(doc, "Slicer_") || !strings.Contains(doc, `"Total"`) {
		t.Errorf("los Slicer_* (#N/A) no son nombres del usuario y Total sí:\n%s", doc)
	}
}

// Entradas rotas u hostiles: el libro se importa igual y el segmentador roto se descarta.
func TestSegmentadoresHostilesNoRompenElLibro(t *testing.T) {
	grande := strings.Repeat("<x>", 1)
	casos := map[string]func(map[string]string){
		"caché sin tabla": func(f map[string]string) { f["xl/tables/table1.xml"] = `<table id="9" ref="A1:C4"/>` },
		"caché que no existe": func(f map[string]string) {
			f["xl/slicers/slicer1.xml"] = strings.ReplaceAll(f["xl/slicers/slicer1.xml"], "Slicer_Producto", "Slicer_Nada")
		},
		"relación rota": func(f map[string]string) { delete(f, "xl/slicerCaches/slicerCache1.xml") },
		"columna fuera de tabla": func(f map[string]string) {
			f["xl/slicerCaches/slicerCache1.xml"] = strings.NewReplacer(`column="2"`, `column="99"`, `sourceName="Producto"`, `sourceName="Otra"`).Replace(f["xl/slicerCaches/slicerCache1.xml"])
		},
		"dinámica inexistente": func(f map[string]string) {
			f["xl/slicerCaches/slicerCache2.xml"] = strings.ReplaceAll(f["xl/slicerCaches/slicerCache2.xml"], "Dinamica1", "Fantasma")
		},
		"XML truncado": func(f map[string]string) { f["xl/slicerCaches/slicerCache1.xml"] = grande },
		"parte desmesurada": func(f map[string]string) {
			f["xl/slicerCaches/slicerCache1.xml"] = strings.Repeat(" ", maxParteSegmentador+1)
		},
		"anidado profundo": func(f map[string]string) {
			f["xl/slicers/slicer1.xml"] = strings.Repeat("<a>", 50000) + strings.Repeat("</a>", 50000)
		},
		"entidades (XML bomb)": func(f map[string]string) {
			f["xl/slicers/slicer2.xml"] = `<!DOCTYPE l [<!ENTITY a "aaaaaaaaaa"><!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;">]><slicers xmlns="` + sgX14 + `"><slicer name="&b;&b;&b;" cache="Slicer_Zona"/></slicers>`
		},
		"definedName sin valor": func(f map[string]string) {
			f["xl/workbook.xml"] = strings.ReplaceAll(f["xl/workbook.xml"], `>#N/A<`, `><`)
		},
	}
	for nombre, mod := range casos {
		t.Run(nombre, func(t *testing.T) {
			h, _ := importarSegmentadores(t, mod)
			if len(h.Slicers) > 2 {
				t.Fatalf("más segmentadores de los posibles: %+v", h.Slicers)
			}
		})
	}
	// Los tres primeros casos pierden exactamente el segmentador roto, no el otro.
	for _, n := range []string{"caché sin tabla", "caché que no existe", "relación rota", "columna fuera de tabla"} {
		h, _ := importarSegmentadores(t, casos[n])
		if len(h.Slicers) != 1 || h.Slicers[0].Pivot != "Dinamica1" {
			t.Errorf("%s: quiero solo el de la dinámica, hay %+v", n, h.Slicers)
		}
	}
	if h, _ := importarSegmentadores(t, casos["dinámica inexistente"]); len(h.Slicers) != 1 || h.Slicers[0].Pivot != "" {
		t.Errorf("dinámica inexistente: quiero solo el de la tabla, hay %+v", h.Slicers)
	}
}

// Quinientos segmentadores de la misma columna se funden en uno.
func TestSegmentadoresRepetidosSeFunden(t *testing.T) {
	h, _ := importarSegmentadores(t, func(f map[string]string) {
		var b strings.Builder
		b.WriteString(`<slicers xmlns="` + sgX14 + `">`)
		for range 500 {
			b.WriteString(`<slicer name="Producto" cache="Slicer_Producto"/>`)
		}
		b.WriteString(`</slicers>`)
		f["xl/slicers/slicer1.xml"] = b.String()
	})
	if len(h.Slicers) > 2 { // los repetidos de la misma columna se funden
		t.Fatalf("hay %d segmentadores", len(h.Slicers))
	}
}

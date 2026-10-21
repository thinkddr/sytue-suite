// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

// El color y el trazo de los bordes y el color de letra de una regla condicional llegan de
// Excel y vuelven a .xlsx y a .ods. Antes solo llegaba que había borde (fino y del color del
// editor), y una regla que solo cambiaba la letra («en rojo si es negativo») se perdía entera.
func TestBordesYLetraCondicional(t *testing.T) {
	files := map[string]string{
		"xl/workbook.xml": `<workbook ` + nsHoja + `><sheets><sheet name="A" sheetId="1" r:id="r1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="r1" Type="` + tipoR + `worksheet" Target="worksheets/sheet1.xml"/>` +
			`<Relationship Id="r2" Type="` + tipoR + `styles" Target="styles.xml"/></Relationships>`,
		"xl/styles.xml": `<styleSheet ` + nsHoja + `><fonts count="1"><font><sz val="11"/></font></fonts>` +
			`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
			`<borders count="4"><border><left/><right/><top/><bottom/><diagonal/></border>` +
			`<border><left style="thin"><color auto="1"/></left><right style="medium"><color rgb="FFFF0000"/></right>` +
			`<top style="mediumDashDot"><color theme="4"/></top><bottom style="double"><color rgb="FF000000"/></bottom><diagonal/></border>` +
			`<border><left/><right/><top/><bottom style="hair"><color indexed="12"/></bottom><diagonal/></border>` +
			`<border><start style="medium"><color rgb="FF00B050"/></start><end style="thin"/><top/><bottom/><diagonal/></border></borders>` +
			`<cellXfs count="4"><xf fontId="0" fillId="0" borderId="0"/><xf fontId="0" fillId="0" borderId="1" applyBorder="1"/><xf fontId="0" fillId="0" borderId="2" applyBorder="1"/>` +
			`<xf fontId="0" fillId="0" borderId="3" applyBorder="1"/></cellXfs>` +
			`<dxfs count="2"><dxf><font><color rgb="FFC00000"/></font></dxf>` +
			`<dxf><font><b/><color theme="1"/></font><fill><patternFill><bgColor rgb="FFFFEB9C"/></patternFill></fill></dxf></dxfs></styleSheet>`,
		"xl/worksheets/sheet1.xml": `<worksheet ` + nsHoja + `><sheetData><row r="1"><c r="A1" s="1"><v>-1</v></c><c r="B1" s="2"><v>2</v></c><c r="C1" s="3"><v>3</v></c></row></sheetData>` +
			`<conditionalFormatting sqref="A1:B1"><cfRule type="cellIs" dxfId="0" priority="1" operator="lessThan"><formula>0</formula></cfRule>` +
			`<cfRule type="cellIs" dxfId="1" priority="2" operator="greaterThan"><formula>1</formula></cfRule></conditionalFormatting></worksheet>`,
	}
	m := parseSheetModelJSON(t, importar1(t, "bordes.xlsx", files))
	quiero := cellBorder{T: true, R: true, B: true, L: true, TS: "dashed", TC: "#4472C4", RS: "medium", RC: "#FF0000", BS: "double"}
	if b := m.Sheets[0].Styles["A1"].Border; b == nil || *b != quiero {
		t.Errorf("bordes de A1 = %+v, quiero %+v (el negro y «auto» son el color de serie)", b, quiero)
	}
	if b := m.Sheets[0].Styles["B1"].Border; b == nil || *b != (cellBorder{B: true, BS: "dotted", BC: "#0000FF"}) {
		t.Errorf("borde «hair» indexado de B1 = %+v", b)
	}
	if b := m.Sheets[0].Styles["C1"].Border; b == nil || *b != (cellBorder{L: true, LS: "medium", LC: "#00B050", R: true}) {
		t.Errorf("borde con start y end de C1 = %+v: son la izquierda y la derecha", b)
	}
	reglas := m.Sheets[0].CondFormats
	if len(reglas) != 2 || reglas[0].TextColor != "#C00000" || reglas[0].Fill != "" || reglas[1].Fill != "#FFEB9C" || reglas[1].TextColor == "" {
		t.Fatalf("reglas = %+v: la de solo letra roja tiene que llegar, y la otra con su letra", reglas)
	}
	// La vuelta: a .xlsx y a .ods, y otra vez al modelo.
	for _, formato := range []BlobFormat{FormatOOXML, FormatODF} {
		data, _, ext, err := Export(KindSpreadsheet, "x", stateWithContent(t, "doc:sheet", marshalSheetModel(m)), formato)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Import("x."+ext, data)
		if err != nil {
			t.Fatal(err)
		}
		v := parseSheetModelJSON(t, res.Content)
		if b := v.Sheets[0].Styles["A1"].Border; b == nil || *b != quiero {
			t.Errorf("%s: bordes de A1 a la vuelta = %+v", ext, b)
		}
		if r := v.Sheets[0].CondFormats; len(r) != 2 || r[0].TextColor != "#C00000" || !strings.EqualFold(r[1].Fill, "#FFEB9C") {
			t.Errorf("%s: reglas a la vuelta = %+v", ext, r)
		}
	}
}

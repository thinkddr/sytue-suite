// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"reflect"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
)

// hojaConOcultas lleva de todo lo que esconde filas y columnas a la vez, para que cada cosa
// vuelva a su sitio y no a la de al lado: filas y columnas ocultas a mano (una suelta, un
// tramo y otro pegado a un grupo), un grupo colapsado y un autofiltro que esconde una fila.
const hojaConOcultas = `{"active":0,"sheets":[{"name":"O","cells":{
	"A1":"Zona","B1":"Kilos","C1":"c","D1":"d","E1":"e","F1":"f","G1":"g","H1":"h",
	"A2":"Norte","B2":"1","A3":"Sur","B3":"2","A4":"Norte","B4":"3",
	"D7":"x","D8":"y","D9":"z","D12":"fin"},
	"autofilter":{"row":0,"c1":0,"c2":1,"excl":{"0":["Sur"]}},
	"rowGroups":[{"start":7,"end":8,"level":1,"collapsed":true}],
	"hiddenRows":[{"start":5,"end":6},{"start":10,"end":10}],
	"colGroups":[{"start":5,"end":5,"level":1,"collapsed":true}],
	"hiddenCols":[{"start":2,"end":2},{"start":6,"end":7}]}]}`

func igualOcultas(t *testing.T, donde string, quiero, tengo sheetData) {
	t.Helper()
	if !reflect.DeepEqual(quiero.HiddenRows, tengo.HiddenRows) {
		t.Errorf("%s: filas ocultas\n tengo  %+v\n quiero %+v", donde, tengo.HiddenRows, quiero.HiddenRows)
	}
	if !reflect.DeepEqual(quiero.HiddenCols, tengo.HiddenCols) {
		t.Errorf("%s: columnas ocultas\n tengo  %+v\n quiero %+v", donde, tengo.HiddenCols, quiero.HiddenCols)
	}
	igualGrupos(t, donde+" filas", sanearGrupos(quiero.RowGroups, maxFilasHoja), tengo.RowGroups)
	igualGrupos(t, donde+" columnas", sanearGrupos(quiero.ColGroups, maxColumnasHoja), tengo.ColGroups)
}

// TestOcultasIdaYVuelta: lo oculto a mano vuelve oculto a mano, sin convertirse en un grupo
// ni tragarse las filas del grupo colapsado o las que esconde el autofiltro.
func TestOcultasIdaYVuelta(t *testing.T) {
	quiero := hojaDe(t, hojaConOcultas)
	for _, f := range []BlobFormat{FormatOOXML, FormatODF} {
		data, ext := exportarHoja(t, hojaConOcultas, f)
		igualOcultas(t, ext, quiero, importarHoja(t, "o."+ext, data))
	}
}

// TestOcultaDentroDelFiltro: en un .xlsx la fila que esconde el filtro y la que se ocultó a
// mano dentro de su rango se escriben igual (`hidden="1"`); al leerlo, la que el filtro no
// escondería es la oculta a mano. (LibreOffice da por filtradas todas las del rango, así que
// esto no pasa por él.)
func TestOcultaDentroDelFiltro(t *testing.T) {
	modelo := `{"active":0,"sheets":[{"name":"F","cells":{"A1":"Zona","B1":"Kilos",
		"A2":"Norte","B2":"1","A3":"Sur","B3":"2","A4":"Este","B4":"3","A5":"Norte","B5":"4"},
		"autofilter":{"row":0,"c1":0,"c2":1,"excl":{"0":["Sur"]}},
		"hiddenRows":[{"start":3,"end":3}]}]}`
	data, ext := exportarHoja(t, modelo, FormatOOXML)
	sd := importarHoja(t, "f."+ext, data)
	if quiero := []analisis.Tramo{{Start: 3, End: 3}}; !reflect.DeepEqual(sd.HiddenRows, quiero) {
		t.Errorf("filas ocultas = %+v, quiero %+v (la de «Sur» es del filtro)", sd.HiddenRows, quiero)
	}
	if sd.AutoFilter == nil || !reflect.DeepEqual(sd.AutoFilter.Excl, map[string][]string{"0": {"Sur"}}) {
		t.Errorf("el autofiltro no volvió igual: %+v", sd.AutoFilter)
	}
}

func TestOcultasPorLibreOffice(t *testing.T) {
	quiero := hojaDe(t, hojaConOcultas)
	for _, f := range []BlobFormat{FormatOOXML, FormatODF} {
		data, ext := exportarHoja(t, hojaConOcultas, f)
		for _, d := range []struct{ ext, formato string }{{"ods", "ods"}, {"xlsx", "xlsx:Calc MS Excel 2007 XML"}} {
			igualOcultas(t, ext+"→"+d.ext, quiero, importarHoja(t, "lo."+d.ext, sofficeConvertir(t, data, ext, d.formato)))
		}
	}
}

// TestOcultasEnElXml: lo que lee Excel. Una fila oculta a mano es `hidden="1"` sin
// `outlineLevel` (un grupo la plegaría con un botón que nadie puso), y la fila vacía oculta
// se escribe aunque no tenga celdas.
func TestOcultasEnElXml(t *testing.T) {
	data, _ := exportarHoja(t, hojaConOcultas, FormatOOXML)
	hoja := unzipPart(t, data, "xl/worksheets/sheet1.xml")
	for _, quiero := range []string{`<row r="6" hidden="1"`, `<row r="11" hidden="1"`, `<col min="3" max="3"`} {
		if !strings.Contains(hoja, quiero) {
			t.Errorf("falta %q en la hoja:\n%s", quiero, hoja)
		}
	}
	if i := strings.Index(hoja, `<row r="6"`); i >= 0 && strings.Contains(hoja[i:i+60], "outlineLevel") {
		t.Errorf("una fila oculta a mano lleva nivel de esquema: %s", hoja[i:i+60])
	}
}

// TestOcultasDeExcelSinEsquema: un .xlsx de Excel con filas y columnas ocultas a mano, que
// antes se abría con todo a la vista.
func TestOcultasDeExcelSinEsquema(t *testing.T) {
	hoja := `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
	<cols><col min="2" max="3" width="9" hidden="1"/><col min="5" max="16384" width="9" hidden="true"/></cols><sheetData>
	<row r="1"><c r="A1"><v>1</v></c></row>
	<row r="2" hidden="1"><c r="A2"><v>2</v></c></row>
	<row r="3" hidden="1"/>
	<row r="5" hidden="1"/></sheetData></worksheet>`
	pkg, err := zipPackage(map[string]string{
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="H" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   hoja,
	})
	if err != nil {
		t.Fatal(err)
	}
	sd := importarHoja(t, "h.xlsx", pkg)
	if quiero := []analisis.Tramo{{Start: 1, End: 2}, {Start: 4, End: 4}}; !reflect.DeepEqual(sd.HiddenRows, quiero) {
		t.Errorf("filas ocultas = %+v, quiero %+v", sd.HiddenRows, quiero)
	}
	if quiero := []analisis.Tramo{{Start: 1, End: 2}, {Start: 4, End: maxColumnasHoja - 1}}; !reflect.DeepEqual(sd.HiddenCols, quiero) {
		t.Errorf("columnas ocultas = %+v, quiero %+v", sd.HiddenCols, quiero)
	}
	if len(sd.RowGroups)+len(sd.ColGroups) != 0 {
		t.Errorf("lo oculto a mano se convirtió en esquema: %+v %+v", sd.RowGroups, sd.ColGroups)
	}
}

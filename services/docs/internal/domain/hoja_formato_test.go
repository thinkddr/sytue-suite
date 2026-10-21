// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Pruebas del formato de una hoja (estilos de celda, formato condicional y validaciones de
// datos) en .xlsx y .ods: lo que se exporta vuelve igual al importarlo, y lo abre y lo
// conserva LibreOffice donde hay `soffice` (el contenedor de los agentes).

// hojaConFormato es una hoja de Calcus con un ejemplo de cada cosa que el editor sabe
// guardar de formato.
func hojaConFormato() string {
	return `{"active":0,"sheets":[{"name":"Ventas","cells":{
		"A1":"<b>Zona</b>","B1":"<div style=\"text-align: right;\"><i>Kilos</i></div>","C1":"Margen",
		"A2":"Norte","B2":"1200","C2":"0,25","A3":"Sur","B3":"800","C3":"0,5","A4":"Este","B4":"1500","C4":"x",
		"A5":"Oeste","B5":"700","D2":"=B2*2"},
	"formats":{"C2":{"id":"percent","decimals":0}},
	"styles":{
		"A1":{"fill":"#DDEBF7","color":"#1F3864","size":14,"font":"Arial","u":true,"s":true,"wrap":true,"valign":"top",
			"border":{"t":true,"b":true,"l":true,"r":true}},
		"E9":{"fill":"#FFF2CC"}},
	"condFormats":[
		{"range":"B2:B4","op":"gt","value":"1000","fill":"#FDE9D9"},
		{"range":"B2:B5","op":"contains","value":"7","fill":"#E2EFDA"},
		{"range":"A2:A5","kind":"formula","formula":"=$B2>1000","fill":"#FFE699"},
		{"range":"B2:B5","kind":"top","rank":2,"fill":"#C6E0B4"},
		{"range":"B2:B5","kind":"bottom","rank":20,"percent":true,"fill":"#F8CBAD"},
		{"range":"A2:A5","kind":"dup","fill":"#BDD7EE"},
		{"range":"A2:A5","kind":"unique","fill":"#D9D9D9"},
		{"range":"B2:B5","kind":"dataBar","color":"#2563EB"},
		{"range":"C2:C3","kind":"colorScale","scale":["#F8696B","#63BE7B"]},
		{"range":"B2:B5","kind":"colorScale","scale":["#F8696B","#FFEB84","#63BE7B"]},
		{"range":"B2:B5","kind":"iconSet","icons":"traffic"}],
	"validations":[
		{"range":"A2:A5","values":["Norte","Sur","Este"],"error":"Zona no válida","strict":true},
		{"range":"B2:B5","type":"number","op":"gt","a":"0","integer":true,"strict":true},
		{"range":"C2:C3","type":"number","op":"between","a":"0","b":"1,5"},
		{"range":"E2:E3","type":"date","op":"ge","a":"01/01/2026","strict":true},
		{"range":"F2","type":"textLength","op":"le","a":"10","strict":true},
		{"range":"G2","type":"formula","formula":"=G2>0","strict":true}]
	}]}`
}

// sofficeConvertir convierte con LibreOffice y devuelve el fichero; salta la prueba si no hay.
func sofficeConvertir(t *testing.T, data []byte, ext, destino string) []byte {
	t.Helper()
	soffice, err := exec.LookPath("soffice")
	if err != nil {
		t.Skip("sin soffice")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "hoja."+ext)
	if err := os.WriteFile(in, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(soffice, "--headless", "-env:UserInstallation=file://"+filepath.Join(dir, "perfil"),
		"--convert-to", destino, "--outdir", filepath.Join(dir, "fuera"), in)
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("soffice %s: %v\n%s", destino, err, o)
	}
	out, err := os.ReadFile(filepath.Join(dir, "fuera", "hoja."+strings.SplitN(destino, ":", 2)[0]))
	if err != nil {
		t.Fatalf("LibreOffice no produjo el %s: %v", destino, err)
	}
	return out
}

func exportarHoja(t *testing.T, modelo string, formato BlobFormat) ([]byte, string) {
	t.Helper()
	data, _, ext, err := Export(KindSpreadsheet, "", stateWithContent(t, "doc:sheet", modelo), formato)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	return data, ext
}

func importarHoja(t *testing.T, nombre string, data []byte) sheetData {
	t.Helper()
	res, err := Import(nombre, data)
	if err != nil {
		t.Fatalf("import %s: %v", nombre, err)
	}
	var m sheetModel
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil || len(m.Sheets) == 0 {
		t.Fatalf("modelo ilegible: %v", err)
	}
	return m.Sheets[0]
}

func hojaDe(t *testing.T, modelo string) sheetData {
	t.Helper()
	var m sheetModel
	if err := json.Unmarshal([]byte(modelo), &m); err != nil {
		t.Fatal(err)
	}
	return m.Sheets[0]
}

// igualFormato compara el formato de dos hojas, regla a regla, con un mensaje que dice cuál.
func igualFormato(t *testing.T, donde string, quiero, tengo sheetData) {
	t.Helper()
	for ref, st := range quiero.Styles {
		if !reflect.DeepEqual(st, tengo.Styles[ref]) {
			t.Errorf("%s: estilo de %s = %+v, quiero %+v", donde, ref, tengo.Styles[ref], st)
		}
	}
	if len(tengo.Styles) != len(quiero.Styles) {
		t.Errorf("%s: %d estilos, quiero %d", donde, len(tengo.Styles), len(quiero.Styles))
	}
	for ref, c := range quiero.Cells {
		if tengo.Cells[ref] != c {
			t.Errorf("%s: celda %s = %q, quiero %q", donde, ref, tengo.Cells[ref], c)
		}
	}
	if !reflect.DeepEqual(quiero.CondFormats, tengo.CondFormats) {
		t.Errorf("%s: formato condicional\n tengo  %+v\n quiero %+v", donde, tengo.CondFormats, quiero.CondFormats)
	}
	if !reflect.DeepEqual(quiero.Validations, tengo.Validations) {
		t.Errorf("%s: validaciones\n tengo  %+v\n quiero %+v", donde, tengo.Validations, quiero.Validations)
	}
}

func TestFormatoXlsxIdaYVuelta(t *testing.T) {
	quiero := hojaDe(t, hojaConFormato())
	data, ext := exportarHoja(t, hojaConFormato(), FormatOOXML)
	igualFormato(t, "xlsx", quiero, importarHoja(t, "x."+ext, data))
}

func TestFormatoXlsxSeAbreEnLibreOffice(t *testing.T) {
	data, _ := exportarHoja(t, hojaConFormato(), FormatOOXML)
	if pdf := sofficeConvertir(t, data, "xlsx", "pdf"); !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatal("LibreOffice no produjo un PDF válido")
	}
	// Lo que LibreOffice guarda de vuelta conserva el formato (lo que escribe Calc se lee).
	quiero := hojaDe(t, hojaConFormato())
	vuelta := importarHoja(t, "lo.xlsx", sofficeConvertir(t, data, "xlsx", "xlsx:Calc MS Excel 2007 XML"))
	if len(vuelta.Styles) < 2 || len(vuelta.CondFormats) < len(quiero.CondFormats)-1 || len(vuelta.Validations) < len(quiero.Validations)-1 {
		t.Errorf("LibreOffice perdió formato: %d estilos, %d reglas, %d validaciones", len(vuelta.Styles), len(vuelta.CondFormats), len(vuelta.Validations))
	}
	t.Logf("LibreOffice: reglas %+v\nvalidaciones %+v", vuelta.CondFormats, vuelta.Validations)
}

func TestFormatoOdsIdaYVuelta(t *testing.T) {
	quiero := hojaDe(t, hojaConFormato())
	data, ext := exportarHoja(t, hojaConFormato(), FormatODF)
	igualFormato(t, "ods", quiero, importarHoja(t, "x."+ext, data))
}

func TestFormatoOdsSeAbreEnLibreOffice(t *testing.T) {
	data, _ := exportarHoja(t, hojaConFormato(), FormatODF)
	if pdf := sofficeConvertir(t, data, "ods", "pdf"); !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatal("LibreOffice no produjo un PDF válido")
	}
	// Lo que LibreOffice guarda de vuelta (ods y xlsx) conserva el formato.
	quiero := hojaDe(t, hojaConFormato())
	for _, destino := range []struct{ ext, formato string }{{"ods", "ods"}, {"xlsx", "xlsx:Calc MS Excel 2007 XML"}} {
		vuelta := importarHoja(t, "lo."+destino.ext, sofficeConvertir(t, data, "ods", destino.formato))
		if len(vuelta.Styles) < 2 || len(vuelta.CondFormats) != len(quiero.CondFormats) || len(vuelta.Validations) < len(quiero.Validations)-1 {
			t.Errorf("%s: LibreOffice perdió formato: %d estilos, %d reglas, %d validaciones", destino.ext, len(vuelta.Styles), len(vuelta.CondFormats), len(vuelta.Validations))
		}
		if !reflect.DeepEqual(vuelta.CondFormats, quiero.CondFormats) {
			t.Logf("%s: reglas tras LibreOffice\n tengo  %+v\n quiero %+v", destino.ext, vuelta.CondFormats, quiero.CondFormats)
		}
		if !reflect.DeepEqual(vuelta.Validations, quiero.Validations) {
			t.Logf("%s: validaciones tras LibreOffice\n tengo  %+v\n quiero %+v", destino.ext, vuelta.Validations, quiero.Validations)
		}
	}
}

// TestEstilosXlsxDeExcel lee una hoja de estilos como la escribe Excel: colores de tema con
// tinte, colores indexados, rellenos por defecto y bordes; la negrita y la cursiva van al
// HTML de la celda y el resto de estilo, al mapa `styles`.
func TestEstilosXlsxDeExcel(t *testing.T) {
	styles := `<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
	<fonts count="3"><font><sz val="11"/><color theme="1"/><name val="Aptos Narrow"/></font>
	<font><b/><i/><sz val="14"/><color indexed="10"/><name val="Arial"/></font>
	<font><sz val="11"/><color rgb="FF00B050"/><name val="Aptos Narrow"/></font></fonts>
	<fills count="4"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill>
	<fill><patternFill patternType="solid"><fgColor theme="4" tint="0.79998168889431442"/><bgColor indexed="64"/></patternFill></fill>
	<fill><patternFill patternType="solid"><fgColor indexed="64"/></patternFill></fill></fills>
	<borders count="2"><border><left/><right/><top/><bottom/><diagonal/></border><border><left style="thin"><color auto="1"/></left><right/><top/><bottom style="medium"/><diagonal/></border></borders>
	<cellXfs count="4"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/>
	<xf numFmtId="0" fontId="1" fillId="2" borderId="1" applyFont="1"><alignment horizontal="center" vertical="top" wrapText="1"/></xf>
	<xf numFmtId="0" fontId="2" fillId="0" borderId="0"/>
	<xf numFmtId="0" fontId="0" fillId="3" borderId="0"/></cellXfs></styleSheet>`
	e := parseXlsxStyles([]byte(styles), nil)
	if e == nil || len(e.xfs) != 4 {
		t.Fatalf("xfs mal leídos: %+v", e)
	}
	x := e.xfs[1]
	if !x.negrita || !x.cursiva || x.alinear != "center" {
		t.Errorf("negrita/cursiva/alineación: %+v", x)
	}
	st := x.estilo
	// «Azul, Énfasis 1, Claro 80 %» de Office es #D9E1F2; Excel redondea el HLS a enteros de 0 a 240.
	if st.Fill != "#D9E1F2" && st.Fill != "#DAE3F3" {
		t.Errorf("relleno de tema con tinte = %q, quiero cerca de #D9E1F2", st.Fill)
	}
	if st.Color != "#FF0000" || st.Size != 14 || st.Font != "Arial" || st.Valign != "top" || !st.Wrap {
		t.Errorf("estilo = %+v", st)
	}
	if st.Border == nil || !st.Border.L || !st.Border.B || st.Border.T || st.Border.R {
		t.Errorf("bordes = %+v", st.Border)
	}
	if e.xfs[2].estilo.Color != "#00B050" {
		t.Errorf("color rgb = %q", e.xfs[2].estilo.Color)
	}
	if e.xfs[3].estilo.Fill != "" {
		t.Errorf("el relleno de sistema (indexed 64) no es un color: %q", e.xfs[3].estilo.Fill)
	}
	if !e.xfs[0].estilo.vacio() {
		t.Errorf("el estilo por defecto debe quedar vacío: %+v", e.xfs[0])
	}
}

// TestFormatoHostil: rangos fuera de la rejilla, repeticiones absurdas y miles de reglas no
// hacen crecer el modelo ni tumban la importación.
func TestFormatoHostil(t *testing.T) {
	var cf, dv strings.Builder
	for i := 0; i < 5000; i++ {
		cf.WriteString(`<conditionalFormatting sqref="A1:A9"><cfRule type="duplicateValues" dxfId="0" priority="1"/></conditionalFormatting>`)
		dv.WriteString(`<dataValidation type="list" sqref="B1:B3"><formula1>"a,b"</formula1></dataValidation>`)
	}
	hoja := `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>x</t></is></c><c r="XFE1" t="inlineStr"><is><t>fuera</t></is></c></row></sheetData>` +
		cf.String() + `<dataValidations count="1">` + dv.String() + `<dataValidation type="list" sqref="A1:ZZZZ99999999"><formula1>"a"</formula1></dataValidation></dataValidations></worksheet>`
	pkg, err := zipPackage(map[string]string{
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="H" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   hoja,
		"xl/styles.xml":              `<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><dxfs count="1"><dxf><fill><patternFill><bgColor rgb="FFFF0000"/></patternFill></fill></dxf></dxfs></styleSheet>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	sd := importarHoja(t, "hostil.xlsx", pkg)
	if len(sd.CondFormats) > maxReglasHoja || len(sd.Validations) > maxValidacionesHoja {
		t.Errorf("sin tope: %d reglas, %d validaciones", len(sd.CondFormats), len(sd.Validations))
	}
	if _, ok := sd.Cells["XFE1"]; ok {
		t.Error("una celda fuera de la rejilla de Excel no entra")
	}

	ods := `<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
	<office:automatic-styles><style:style style:name="s" style:family="table-cell"><style:table-cell-properties fo:background-color="#ff0000"/></style:style></office:automatic-styles>
	<office:body><office:spreadsheet><table:table table:name="H"><table:table-row table:number-rows-repeated="1000000000"><table:table-cell table:style-name="s" table:number-columns-repeated="1000000000"/><table:table-cell table:style-name="s" table:number-columns-repeated="3"/></table:table-row></table:table></office:spreadsheet></office:body></office:document-content>`
	pkg, err = buildODFPackage("application/vnd.oasis.opendocument.spreadsheet", ods, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	sd = importarHoja(t, "hostil.ods", pkg)
	if len(sd.Styles) > 8 {
		t.Errorf("las repeticiones absurdas no se expanden: %d estilos", len(sd.Styles))
	}
}

const hojaConMedidas = `{"active":0,"sheets":[{"name":"M","cells":{"A1":"Zona","B1":"x","C3":"y"},
	"colWidths":{"0":150,"1":150,"2":200,"5":64,"7":300},"rowHeights":{"0":40,"4":60,"6":18}}]}`

func igualMedidas(t *testing.T, donde string, quiero, tengo map[string]float64, tol float64) {
	t.Helper()
	if len(quiero) != len(tengo) {
		t.Errorf("%s: %d medidas, quiero %d (%v)", donde, len(tengo), len(quiero), tengo)
	}
	for k, q := range quiero {
		if g := tengo[k]; g < q-tol || g > q+tol {
			t.Errorf("%s: medida %s = %v, quiero %v", donde, k, g, q)
		}
	}
}

func TestMedidasIdaYVuelta(t *testing.T) {
	quiero := hojaDe(t, hojaConMedidas)
	for _, f := range []BlobFormat{FormatOOXML, FormatODF} {
		data, ext := exportarHoja(t, hojaConMedidas, f)
		sd := importarHoja(t, "m."+ext, data)
		igualMedidas(t, ext+" columnas", quiero.ColWidths, sd.ColWidths, 0.5)
		igualMedidas(t, ext+" filas", quiero.RowHeights, sd.RowHeights, 0.5)
	}
}

func TestMedidasSeAbrenEnLibreOffice(t *testing.T) {
	quiero := hojaDe(t, hojaConMedidas)
	for _, f := range []BlobFormat{FormatOOXML, FormatODF} {
		data, ext := exportarHoja(t, hojaConMedidas, f)
		if pdf := sofficeConvertir(t, data, ext, "pdf"); !bytes.HasPrefix(pdf, []byte("%PDF")) {
			t.Fatal("LibreOffice no produjo un PDF válido")
		}
		for _, d := range []struct{ ext, formato string }{{"ods", "ods"}, {"xlsx", "xlsx:Calc MS Excel 2007 XML"}} {
			sd := importarHoja(t, "lo."+d.ext, sofficeConvertir(t, data, ext, d.formato))
			// El ancho de Excel son caracteres de la fuente base (Calibri 11, 7 px): LibreOffice los
			// mide con la fuente que tenga, y en el contenedor no es Calibri. Desde .xlsx solo se
			// exige el orden entre columnas; desde .ods, el valor (con 6 % de holgura al volver a .xlsx).
			if ext == "xlsx" {
				w := sd.ColWidths
				if !(w["0"] == w["1"] && w["2"] > w["0"] && w["5"] < w["0"] && w["7"] > w["2"]) {
					t.Errorf("xlsx→%s: el orden de los anchos no se conserva: %v", d.ext, w)
				}
			} else {
				tol := 2.0
				if d.ext == "xlsx" {
					tol = 0.06 * 300
				}
				for k, q := range quiero.ColWidths {
					if g := sd.ColWidths[k]; g < q-tol || g > q+tol {
						t.Errorf("%s→%s: columna %s = %v, quiero %v", ext, d.ext, k, g, q)
					}
				}
			}
			for k, q := range quiero.RowHeights {
				if k == "6" {
					continue // 18 px: LibreOffice lo da por casi igual a la fila de serie del libro
				}
				if g := sd.RowHeights[k]; g < q-2 || g > q+2 {
					t.Errorf("%s→%s: fila %s = %v, quiero %v", ext, d.ext, k, g, q)
				}
			}
		}
	}
}

const hojaConFormatosNumero = `{"active":0,"sheets":[{"name":"N","cells":{"A1":"1234.5","A2":"0.25","A3":"46096","A4":"3.14159","A5":"1234567","A6":"99"},
	"formats":{"A1":{"id":"currency","decimals":2},"A2":{"id":"percent","decimals":1},"A3":{"id":"date"},
		"A4":{"id":"number","decimals":3},"A5":{"id":"thousands","decimals":0},"A6":{"id":"number","decimals":0}}}]}`

func igualFormatos(t *testing.T, donde string, quiero, tengo sheetData) {
	t.Helper()
	for ref, q := range quiero.Formats {
		g, ok := tengo.Formats[ref]
		if !ok || g.ID != q.ID || fmtDecimals(g) != fmtDecimals(q) {
			t.Errorf("%s: formato de %s = %+v, quiero %+v", donde, ref, g, q)
		}
		if tengo.Cells[ref] != quiero.Cells[ref] {
			t.Errorf("%s: valor de %s = %q, quiero %q", donde, ref, tengo.Cells[ref], quiero.Cells[ref])
		}
	}
}

func TestFormatosDeNumeroIdaYVuelta(t *testing.T) {
	quiero := hojaDe(t, hojaConFormatosNumero)
	for _, f := range []BlobFormat{FormatOOXML, FormatODF} {
		data, ext := exportarHoja(t, hojaConFormatosNumero, f)
		igualFormatos(t, ext, quiero, importarHoja(t, "n."+ext, data))
	}
}

func TestFormatosDeNumeroPorLibreOffice(t *testing.T) {
	quiero := hojaDe(t, hojaConFormatosNumero)
	for _, f := range []BlobFormat{FormatOOXML, FormatODF} {
		data, ext := exportarHoja(t, hojaConFormatosNumero, f)
		for _, d := range []struct{ ext, formato string }{{"ods", "ods"}, {"xlsx", "xlsx:Calc MS Excel 2007 XML"}} {
			igualFormatos(t, ext+"→"+d.ext, quiero, importarHoja(t, "lo."+d.ext, sofficeConvertir(t, data, ext, d.formato)))
		}
	}
}

// TestFormulaCompartidaDeExcel: la maestra lleva el texto y las demás lo heredan con las
// referencias relativas desplazadas; una que se saldría de la hoja queda sin fórmula.
func TestFormulaCompartidaDeExcel(t *testing.T) {
	hoja := `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>
	<row r="2"><c r="B2"><v>1</v></c><c r="C2"><f t="shared" ref="C2:C4" si="0">B2*2+$A$1</f><v>2</v></c></row>
	<row r="3"><c r="B3"><v>2</v></c><c r="C3"><f t="shared" si="0"/><v>4</v></c></row>
	<row r="4"><c r="B4"><v>3</v></c><c r="C4"><f t="shared" si="0"/><v>6</v></c></row></sheetData></worksheet>`
	pkg, err := zipPackage(map[string]string{
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="H" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   hoja,
	})
	if err != nil {
		t.Fatal(err)
	}
	sd := importarHoja(t, "c.xlsx", pkg)
	for ref, quiero := range map[string]string{"C2": "=B2*2+$A$1", "C3": "=B3*2+$A$1", "C4": "=B4*2+$A$1"} {
		if sd.Cells[ref] != quiero {
			t.Errorf("%s = %q, quiero %q", ref, sd.Cells[ref], quiero)
		}
	}
}

const hojaConEsquema = `{"active":0,"sheets":[{"name":"E","cells":{"A1":"t","A2":"a","A3":"b","A4":"c","A5":"sub","A6":"d","A7":"e","A8":"sub2","A9":"total","D1":"x","E1":"y","F1":"z"},
	"rowGroups":[{"start":1,"end":3,"level":1},{"start":2,"end":2,"level":2,"collapsed":true},{"start":5,"end":6,"level":1,"collapsed":true}],
	"colGroups":[{"start":3,"end":4,"level":1,"collapsed":true}]}]}`

func igualGrupos(t *testing.T, donde string, quiero, tengo []sheetGroup) {
	t.Helper()
	if !reflect.DeepEqual(quiero, tengo) {
		t.Errorf("%s: grupos\n tengo  %+v\n quiero %+v", donde, tengo, quiero)
	}
}

func TestEsquemaIdaYVuelta(t *testing.T) {
	quiero := hojaDe(t, hojaConEsquema)
	for _, f := range []BlobFormat{FormatOOXML, FormatODF} {
		data, ext := exportarHoja(t, hojaConEsquema, f)
		sd := importarHoja(t, "e."+ext, data)
		igualGrupos(t, ext+" filas", sanearGrupos(quiero.RowGroups, maxFilasHoja), sd.RowGroups)
		igualGrupos(t, ext+" columnas", sanearGrupos(quiero.ColGroups, maxColumnasHoja), sd.ColGroups)
	}
}

func TestEsquemaPorLibreOffice(t *testing.T) {
	quiero := hojaDe(t, hojaConEsquema)
	for _, f := range []BlobFormat{FormatOOXML, FormatODF} {
		data, ext := exportarHoja(t, hojaConEsquema, f)
		for _, d := range []struct{ ext, formato string }{{"ods", "ods"}, {"xlsx", "xlsx:Calc MS Excel 2007 XML"}} {
			sd := importarHoja(t, "lo."+d.ext, sofficeConvertir(t, data, ext, d.formato))
			igualGrupos(t, ext+"→"+d.ext+" filas", sanearGrupos(quiero.RowGroups, maxFilasHoja), sd.RowGroups)
			igualGrupos(t, ext+"→"+d.ext+" columnas", sanearGrupos(quiero.ColGroups, maxColumnasHoja), sd.ColGroups)
		}
	}
}

const hojaConFiltro = `{"active":0,"sheets":[{"name":"F","cells":{"A1":"Zona","B1":"Kilos","C1":"Nota","D1":"Top",
	"A2":"Norte","B2":"1200","C2":"alfa","D2":"5","A3":"Sur","B3":"800","C3":"beta","D3":"6","A4":"Este","B4":"1500","C4":"gamma","D4":"7","A5":"Oeste","B5":"700","C5":"alfa2","D5":"8","A6":"Norte","B6":"900","C6":"x","D6":"9"},
	"autofilter":{"row":0,"c1":0,"c2":3,"excl":{"0":["Sur","Este"]},"cond":{"1":{"op":"between","a":"800","b":"1300"},"2":{"op":"contains","a":"a"},"3":{"op":"top","n":3}}}}]}`

func TestAutofiltroOdsIdaYVuelta(t *testing.T) {
	quiero := hojaDe(t, hojaConFiltro)
	data, ext := exportarHoja(t, hojaConFiltro, FormatODF)
	sd := importarHoja(t, "f."+ext, data)
	if !reflect.DeepEqual(sd.AutoFilter, quiero.AutoFilter) {
		t.Errorf("autofiltro\n tengo  %+v\n quiero %+v", sd.AutoFilter, quiero.AutoFilter)
	}
}

// El «N superiores» no sobrevive a LibreOffice leyendo un .ods ajeno (lo lee como «mayor o
// igual»), así que esa prueba va sin él.
func TestAutofiltroOdsPorLibreOffice(t *testing.T) {
	modelo := strings.Replace(hojaConFiltro, `,"3":{"op":"top","n":3}`, "", 1)
	quiero := hojaDe(t, modelo)
	data, _ := exportarHoja(t, modelo, FormatODF)
	for _, d := range []struct{ ext, formato string }{{"ods", "ods"}, {"xlsx", "xlsx:Calc MS Excel 2007 XML"}} {
		sd := importarHoja(t, "lo."+d.ext, sofficeConvertir(t, data, "ods", d.formato))
		if sd.AutoFilter == nil || sd.AutoFilter.Row != 0 || sd.AutoFilter.C2 != 3 {
			t.Errorf("ods→%s: LibreOffice perdió el autofiltro: %+v", d.ext, sd.AutoFilter)
			continue
		}
		if !reflect.DeepEqual(sd.AutoFilter, quiero.AutoFilter) {
			t.Errorf("ods→%s: autofiltro\n tengo  %+v\n quiero %+v", d.ext, sd.AutoFilter, quiero.AutoFilter)
		}
	}
}

const hojaConEscenarios = `{"active":0,"sheets":[{"name":"Plan","cells":{"A1":"Ventas","B1":"100","A2":"Coste","B2":"60","A3":"Margen","B3":"=B1-B2"},
	"scenarios":[{"name":"Optimista","values":{"B1":"150","B2":"55"}},{"name":"Pesimista","values":{"B1":"80","B2":"70"}}]}]}`

func TestEscenariosXlsxIdaYVuelta(t *testing.T) {
	quiero := hojaDe(t, hojaConEscenarios)
	data, ext := exportarHoja(t, hojaConEscenarios, FormatOOXML)
	sd := importarHoja(t, "e."+ext, data)
	if !reflect.DeepEqual(sd.Scenarios, quiero.Scenarios) {
		t.Errorf("escenarios\n tengo  %+v\n quiero %+v", sd.Scenarios, quiero.Scenarios)
	}
}

func TestEscenariosOdsIdaYVuelta(t *testing.T) {
	quiero := hojaDe(t, hojaConEscenarios)
	data, ext := exportarHoja(t, hojaConEscenarios, FormatODF)
	sd := importarHoja(t, "e."+ext, data)
	if !reflect.DeepEqual(sd.Scenarios, quiero.Scenarios) {
		t.Errorf("escenarios\n tengo  %+v\n quiero %+v", sd.Scenarios, quiero.Scenarios)
	}
	var m sheetModel
	res, _ := Import("e.ods", data)
	_ = json.Unmarshal([]byte(res.Content), &m)
	if len(m.Sheets) != 1 {
		t.Errorf("las hojas de escenario no son hojas del editor: %d hojas", len(m.Sheets))
	}
}

func TestEscenariosPorLibreOffice(t *testing.T) {
	quiero := hojaDe(t, hojaConEscenarios)
	for _, f := range []BlobFormat{FormatOOXML, FormatODF} {
		data, ext := exportarHoja(t, hojaConEscenarios, f)
		for _, d := range []struct{ ext, formato string }{{"ods", "ods"}, {"xlsx", "xlsx:Calc MS Excel 2007 XML"}} {
			sd := importarHoja(t, "lo."+d.ext, sofficeConvertir(t, data, ext, d.formato))
			if !reflect.DeepEqual(sd.Scenarios, quiero.Scenarios) {
				t.Errorf("%s→%s: escenarios\n tengo  %+v\n quiero %+v", ext, d.ext, sd.Scenarios, quiero.Scenarios)
			}
		}
	}
}

const hojaConCombinadas = `{"active":0,"sheets":[{"name":"C","cells":{"A1":"Título","A3":"x","B3":"y","D4":"z","E5":"w"},
	"merges":["A1:C1","A3:A5","D4:E5"]}]}`

func TestCombinadasIdaYVuelta(t *testing.T) {
	quiero := hojaDe(t, hojaConCombinadas)
	for _, f := range []BlobFormat{FormatOOXML, FormatODF} {
		data, ext := exportarHoja(t, hojaConCombinadas, f)
		sd := importarHoja(t, "c."+ext, data)
		if !reflect.DeepEqual(sd.Merges, quiero.Merges) {
			t.Errorf("%s: combinadas\n tengo  %v\n quiero %v", ext, sd.Merges, quiero.Merges)
		}
		if sd.Cells["A1"] != "Título" {
			t.Errorf("%s: el ancla perdió su texto: %q", ext, sd.Cells["A1"])
		}
	}
}

func TestCombinadasPorLibreOffice(t *testing.T) {
	quiero := hojaDe(t, hojaConCombinadas)
	for _, f := range []BlobFormat{FormatOOXML, FormatODF} {
		data, ext := exportarHoja(t, hojaConCombinadas, f)
		for _, d := range []struct{ ext, formato string }{{"ods", "ods"}, {"xlsx", "xlsx:Calc MS Excel 2007 XML"}} {
			sd := importarHoja(t, "lo."+d.ext, sofficeConvertir(t, data, ext, d.formato))
			if !reflect.DeepEqual(sd.Merges, quiero.Merges) {
				t.Errorf("%s→%s: combinadas\n tengo  %v\n quiero %v", ext, d.ext, sd.Merges, quiero.Merges)
			}
		}
	}
}

// TestCombinadasHostiles: rangos de una celda, fuera de la rejilla o solapados se descartan.
func TestCombinadasHostiles(t *testing.T) {
	got := combinadasDeRangos(sanearCombinadas([]string{"A1", "A1:B2", "B2:C3", "A1:XFE5", "C3:D4", "basura"}))
	if !reflect.DeepEqual(got, []string{"A1:B2", "C3:D4"}) {
		t.Errorf("combinadas saneadas = %v", got)
	}
}

// hojaConSubtotales es lo que deja «Datos > Subtotal» del editor (sheets/outline.ts): un
// SUBTOTAL por bloque y otro general, con el esquema de dos niveles que los pliega.
const hojaConSubtotales = `{"active":0,"sheets":[{"name":"S","cells":{"A1":"Zona","B1":"Importe",
	"A2":"Norte","B2":"10","A3":"Norte","B3":"20","A4":"Total Norte","B4":"=SUBTOTAL(9;B2:B3)",
	"A5":"Sur","B5":"5","A6":"Total Sur","B6":"=SUBTOTAL(9;B5:B5)","A7":"Total general","B7":"=SUBTOTAL(9;B2:B6)"},
	"rowGroups":[{"start":1,"end":5,"level":1},{"start":1,"end":2,"level":2},{"start":4,"end":4,"level":2,"collapsed":true}]}]}`

// TestSubtotalesDeEsquemaIdaYVuelta: las fórmulas SUBTOTAL y los grupos que las pliegan
// vuelven iguales de .xlsx y de .ods.
func TestSubtotalesDeEsquemaIdaYVuelta(t *testing.T) {
	quiero := hojaDe(t, hojaConSubtotales)
	for _, f := range []BlobFormat{FormatOOXML, FormatODF} {
		data, ext := exportarHoja(t, hojaConSubtotales, f)
		sd := importarHoja(t, "s."+ext, data)
		igualGrupos(t, ext+" filas", sanearGrupos(quiero.RowGroups, maxFilasHoja), sd.RowGroups)
		for ref, v := range quiero.Cells {
			if sd.Cells[ref] != v {
				t.Errorf("%s: %s = %q, quiero %q", ext, ref, sd.Cells[ref], v)
			}
		}
		if len(sd.Cells) != len(quiero.Cells) {
			t.Errorf("%s: vuelven %d celdas de %d", ext, len(sd.Cells), len(quiero.Cells))
		}
	}
}

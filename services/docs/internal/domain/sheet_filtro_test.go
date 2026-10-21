// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// libroConFiltro: seis filas de ventas con un autofiltro «Importe > 100» y la región «Sur»
// excluida. Solo la fila 5 (Este, 400) cumple las dos cosas.
const libroConFiltro = `{"active":0,"sheets":[{"name":"Ventas","cells":{` +
	`"A1":"Región","B1":"Importe","C1":"Nota",` +
	`"A2":"Norte","B2":"100","C2":"ok","A3":"Sur","B3":"250","C3":"revisar",` +
	`"A4":"Norte","B4":"80","C4":"ok","A5":"Este","B5":"400","C5":"",` +
	`"A6":"Sur","B6":"150","C6":"revisar","A7":"Oeste","B7":"90","C7":"ok"},` +
	`"autofilter":{"row":0,"c1":0,"c2":2,"excl":{"0":["Sur"]},"cond":{"1":{"op":"gt","a":"100"}}}}]}`

func filtroDe(t *testing.T, doc string) *sheetAutoFilter {
	t.Helper()
	var m sheetModel
	if err := json.Unmarshal([]byte(doc), &m); err != nil {
		t.Fatalf("el libro no se lee: %v\n%s", err, doc)
	}
	return m.Sheets[0].AutoFilter
}

func xlsxDeFiltro(t *testing.T, libro string) (map[string][]byte, []byte) {
	t.Helper()
	data, err := buildXlsx(stateWithContent(t, "doc:sheet", libro))
	if err != nil {
		t.Fatal(err)
	}
	if destino := os.Getenv("SYTUE_FICHERO"); destino != "" {
		_ = os.WriteFile(destino+".xlsx", data, 0o600) // para abrirlo con LibreOffice
	}
	files, err := readZip(data)
	if err != nil {
		t.Fatal(err)
	}
	return files, data
}

// TestElXlsxLlevaElAutofiltroYLasFilasOcultas: el <autoFilter> va tras sheetData con sus
// columnas, y las filas que el filtro esconde llegan con hidden="1" (Excel no reaplica
// el filtro al abrir).
func TestElXlsxLlevaElAutofiltroYLasFilasOcultas(t *testing.T) {
	files, _ := xlsxDeFiltro(t, libroConFiltro)
	hoja := string(files["xl/worksheets/sheet1.xml"])
	if err := xml.Unmarshal([]byte(hoja), new(struct{})); err != nil {
		t.Fatalf("la hoja no es XML válido: %v", err)
	}
	for _, quiero := range []string{
		`</sheetData><autoFilter ref="A1:C7">`,
		`<filterColumn colId="0"><filters><filter val="Norte"/><filter val="Este"/><filter val="Oeste"/></filters></filterColumn>`,
		`<filterColumn colId="1"><customFilters><customFilter operator="greaterThan" val="100"/></customFilters></filterColumn>`,
	} {
		if !strings.Contains(hoja, quiero) {
			t.Errorf("la hoja no lleva %s:\n%s", quiero, hoja)
		}
	}
	for fila, oculta := range map[int]bool{1: false, 2: true, 3: true, 4: true, 5: false, 6: true, 7: true} {
		if got := strings.Contains(hoja, `<row r="`+strconv.Itoa(fila)+`" hidden="1">`); got != oculta {
			t.Errorf("fila %d oculta=%v, quiero %v", fila, got, oculta)
		}
	}
}

// TestLasCondicionesVanYVuelvenDeXlsx: cada operador con equivalente en Excel sale como su
// <filterColumn> y vuelve igual.
func TestLasCondicionesVanYVuelvenDeXlsx(t *testing.T) {
	casos := []struct {
		cond filterCond
		xml  string
	}{
		{filterCond{Op: "gt", A: "100"}, `<customFilter operator="greaterThan" val="100"/>`},
		{filterCond{Op: "ge", A: "100"}, `<customFilter operator="greaterThanOrEqual" val="100"/>`},
		{filterCond{Op: "lt", A: "100"}, `<customFilter operator="lessThan" val="100"/>`},
		{filterCond{Op: "le", A: "100"}, `<customFilter operator="lessThanOrEqual" val="100"/>`},
		{filterCond{Op: "eq", A: "250"}, `<customFilter val="250"/>`},
		{filterCond{Op: "ne", A: "250"}, `<customFilter operator="notEqual" val="250"/>`},
		{filterCond{Op: "between", A: "80", B: "150"}, `<customFilters and="1"><customFilter operator="greaterThanOrEqual" val="80"/><customFilter operator="lessThanOrEqual" val="150"/>`},
		{filterCond{Op: "notBetween", A: "80", B: "150"}, `<customFilters><customFilter operator="lessThan" val="80"/><customFilter operator="greaterThan" val="150"/>`},
		{filterCond{Op: "contains", A: "ev"}, `<customFilter val="*ev*"/>`},
		{filterCond{Op: "notContains", A: "ev"}, `<customFilter operator="notEqual" val="*ev*"/>`},
		{filterCond{Op: "begins", A: "re"}, `<customFilter val="re*"/>`},
		{filterCond{Op: "ends", A: "ar"}, `<customFilter val="*ar"/>`},
		{filterCond{Op: "blank"}, `<filters blank="1"/>`},
		{filterCond{Op: "notBlank"}, `<customFilter operator="notEqual" val=" "/>`},
		{filterCond{Op: "top", N: 2}, `<top10 top="1" percent="0" val="2"/>`},
		{filterCond{Op: "bottom", N: 50, Percent: true}, `<top10 top="0" percent="1" val="50"/>`},
		{filterCond{Op: "aboveAvg"}, `<dynamicFilter type="aboveAverage"/>`},
		{filterCond{Op: "belowAvg"}, `<dynamicFilter type="belowAverage"/>`},
	}
	for _, c := range casos {
		t.Run(c.cond.Op, func(t *testing.T) {
			m := sheetModel{}
			if err := json.Unmarshal([]byte(libroConFiltro), &m); err != nil {
				t.Fatal(err)
			}
			m.Sheets[0].AutoFilter = &sheetAutoFilter{Row: 0, C1: 0, C2: 2, Cond: map[string]filterCond{"2": c.cond}}
			libro, _ := json.Marshal(m)
			files, data := xlsxDeFiltro(t, string(libro))
			hoja := string(files["xl/worksheets/sheet1.xml"])
			if !strings.Contains(hoja, `<filterColumn colId="2">`) || !strings.Contains(hoja, c.xml) {
				t.Fatalf("no sale %s:\n%s", c.xml, hoja)
			}
			_, vuelta, err := importXlsx(data, nil)
			if err != nil {
				t.Fatal(err)
			}
			af := filtroDe(t, vuelta)
			if af == nil || !reflect.DeepEqual(af.Cond["2"], c.cond) {
				t.Errorf("vuelve como %+v, quiero %+v", af, c.cond)
			}
		})
	}
}

// TestLosValoresExcluidosVuelvenDeXlsx: Excel guarda los valores que SE VEN; el editor, los
// que se esconden. Salen y vuelven iguales.
func TestLosValoresExcluidosVuelvenDeXlsx(t *testing.T) {
	_, data := xlsxDeFiltro(t, libroConFiltro)
	_, vuelta, err := importXlsx(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	af := filtroDe(t, vuelta)
	if af == nil || !reflect.DeepEqual(af.Excl, map[string][]string{"0": {"Sur"}}) {
		t.Errorf("las exclusiones vuelven como %+v", af)
	}
	if af.Row != 0 || af.C1 != 0 || af.C2 != 2 {
		t.Errorf("el rango vuelve como %+v", af)
	}
}

// TestUnFiltroPorColorNoRompeElXlsx: el color no tiene equivalente (pide un dxf); la
// columna sale sin criterio y el libro se abre.
func TestUnFiltroPorColorNoRompeElXlsx(t *testing.T) {
	libro := strings.Replace(libroConFiltro, `"cond":{"1":{"op":"gt","a":"100"}}`, `"cond":{"1":{"op":"fill","color":"#ff0000"}}`, 1)
	files, _ := xlsxDeFiltro(t, libro)
	hoja := string(files["xl/worksheets/sheet1.xml"])
	if strings.Contains(hoja, `colId="1"`) {
		t.Errorf("el color no debería exportarse:\n%s", hoja)
	}
	if !strings.Contains(hoja, `<autoFilter ref="A1:C7">`) {
		t.Errorf("el autofiltro debería salir igual:\n%s", hoja)
	}
}

// TestElAutofiltroDeUnaTablaVaEnLaTabla: sobre una tabla no hay un segundo <autoFilter> en la
// hoja (Excel «repararía» el libro): las condiciones van en el de la tabla.
func TestElAutofiltroDeUnaTablaVaEnLaTabla(t *testing.T) {
	libro := strings.Replace(libroConFiltro, `"autofilter":`, `"tables":[{"name":"Ventas","range":"A1:C7"}],"autofilter":`, 1)
	files, data := xlsxDeFiltro(t, libro)
	if hoja := string(files["xl/worksheets/sheet1.xml"]); strings.Contains(hoja, "<autoFilter") {
		t.Errorf("la hoja no debe llevar autoFilter propio sobre una tabla:\n%s", hoja)
	}
	tabla := string(files["xl/tables/table1.xml"])
	if !strings.Contains(tabla, `<autoFilter ref="A1:C7"><filterColumn colId="0">`) ||
		!strings.Contains(tabla, `<customFilter operator="greaterThan" val="100"/>`) {
		t.Errorf("el filtro no va en la tabla:\n%s", tabla)
	}
	_, vuelta, err := importXlsx(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if af := filtroDe(t, vuelta); af == nil || af.Cond["1"].Op != "gt" {
		t.Errorf("el filtro de la tabla no vuelve: %+v", af)
	}
}

// TestUnLibroSinFiltroNoCambia: sin autofiltro, ni <autoFilter> ni filas ocultas.
func TestUnLibroSinFiltroNoCambia(t *testing.T) {
	libro := strings.Replace(libroConFiltro, `,"autofilter":{"row":0,"c1":0,"c2":2,"excl":{"0":["Sur"]},"cond":{"1":{"op":"gt","a":"100"}}}`, "", 1)
	files, _ := xlsxDeFiltro(t, libro)
	if hoja := string(files["xl/worksheets/sheet1.xml"]); strings.Contains(hoja, "autoFilter") || strings.Contains(hoja, "hidden") {
		t.Errorf("sin filtro, la hoja no debe llevar nada de eso:\n%s", hoja)
	}
}

// TestPasaCondicion: la semántica de cada operador sobre una columna mixta. Es el espejo
// de packages/office/test/sheets-filtro.test.ts: si una cambia, cambia la otra.
func TestPasaCondicion(t *testing.T) {
	col := func(vs ...formula.Value) []celdaFiltro {
		out := make([]celdaFiltro, len(vs))
		for i, v := range vs {
			out[i] = celdaFiltro{v: v, texto: formula.Display(v)}
		}
		return out
	}
	mixta := col(10.0, 20.0, 30.0, "Sur", "", 40.0)
	casos := []struct {
		nombre string
		cond   filterCond
		quiero []bool
	}{
		{"mayor que", filterCond{Op: "gt", A: "20"}, []bool{false, false, true, false, false, true}},
		{"mayor o igual", filterCond{Op: "ge", A: "20"}, []bool{false, true, true, false, false, true}},
		{"igual a texto", filterCond{Op: "eq", A: "sur"}, []bool{false, false, false, true, false, false}},
		{"distinto de", filterCond{Op: "ne", A: "20"}, []bool{true, false, true, true, true, true}},
		{"entre (invertido)", filterCond{Op: "between", A: "30", B: "10"}, []bool{true, true, true, false, false, false}},
		{"no entre ignora el texto", filterCond{Op: "notBetween", A: "15", B: "35"}, []bool{true, false, false, false, false, true}},
		{"contiene", filterCond{Op: "contains", A: "u"}, []bool{false, false, false, true, false, false}},
		{"vacías", filterCond{Op: "blank"}, []bool{false, false, false, false, true, false}},
		{"2 superiores", filterCond{Op: "top", N: 2}, []bool{false, false, true, false, false, true}},
		{"50 % inferiores (con empates)", filterCond{Op: "bottom", N: 50, Percent: true}, []bool{true, true, false, false, false, false}},
		{"sobre la media", filterCond{Op: "aboveAvg"}, []bool{false, false, true, false, false, true}},
		{"bajo la media", filterCond{Op: "belowAvg"}, []bool{true, true, false, false, false, false}},
	}
	for _, c := range casos {
		if got := pasaCondicion(c.cond, mixta); !reflect.DeepEqual(got, c.quiero) {
			t.Errorf("%s: %v, quiero %v", c.nombre, got, c.quiero)
		}
	}
	// «A1» no es un número: «igual a A1» compara texto y no se cuela con el 1.
	if got := pasaCondicion(filterCond{Op: "eq", A: "A1"}, col(1.0, "a1")); !reflect.DeepEqual(got, []bool{false, true}) {
		t.Errorf("A1 se leyó como número: %v", got)
	}
}

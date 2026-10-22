// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// libroConDinamica es una hoja de ventas con una tabla dinámica que usa todo lo que
// solo Sytue sabe decir (agrupar fechas, intervalos, formato del valor) y lo que
// cualquier hoja entiende (filas, columnas, valores, filtros y ocultos).
const libroConDinamica = `{"active":0,"sheets":[{"name":"Ventas","cells":{` +
	`"A1":"Zona","B1":"Producto","C1":"Fecha","D1":"Importe",` +
	`"A2":"Norte","B2":"Sillas","C2":"45292","D2":"150","A3":"Norte","B3":"Mesas","C3":"45323","D3":"300",` +
	`"A4":"Sur","B4":"Mesas","C4":"45413","D4":"600","A5":"Sur","B5":"Sillas","C5":"45474","D5":"200",` +
	`"A6":"Este","B6":"Sillas","C6":"45658","D6":"100",` +
	`"G1":"Zona","H1":"(Todos)","G3":"<b>sum de Importe</b>"},` +
	`"pivots":[{"name":"TablaDinámica1","source":"A1:D6","anchor":"G1","range":"G1:J8",` +
	`"rows":[{"field":0,"hidden":["Este"]}],"cols":[{"field":2,"date":"years"}],` +
	`"values":[{"field":3,"fn":"avg","show":"pctGrand","fmt":{"id":"currency"}},{"field":3,"fn":"count","name":"Pedidos"}],` +
	`"filters":[{"field":1}],"grandCols":false}]}]}`

// dinamicasDe son las tablas dinámicas del modelo importado, como JSON genérico.
func dinamicasDe(t *testing.T, modelo string) []any {
	t.Helper()
	var m struct {
		Sheets []struct {
			Pivots []any `json:"pivots"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal([]byte(modelo), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Sheets) == 0 {
		t.Fatal("sin hojas")
	}
	return m.Sheets[0].Pivots
}

func original(t *testing.T) []any {
	t.Helper()
	return dinamicasDe(t, libroConDinamica)
}

// TestLaDinamicaVaYVuelveEnXlsxSinPerder: exportada a .xlsx lleva las partes que Excel
// y LibreOffice leen, y al volver a importarla sale la MISMA definición.
func TestLaDinamicaVaYVuelveEnXlsxSinPerder(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", libroConDinamica)
	data, err := buildXlsx(s)
	if err != nil {
		t.Fatal(err)
	}
	parts := zipParts(t, data) // además, todo el XML bien formado
	guardarMuestra(t, "dinamica.xlsx", data)
	partes := []string{
		"xl/pivotCache/pivotCacheDefinition1.xml", "xl/pivotTables/pivotTable1.xml",
		"xl/worksheets/_rels/sheet1.xml.rels", "xl/pivotTables/_rels/pivotTable1.xml.rels",
	}
	for _, p := range partes {
		if parts[p] == "" {
			t.Fatalf("falta %s", p)
		}
	}
	tabla := parts["xl/pivotTables/pivotTable1.xml"]
	quiero := []string{
		`<rowFields count="1"><field x="0"/>`, `<field x="-2"/>`, `<pageField fld="1"`,
		`subtotal="average"`, `showDataAs="percentOfTotal"`, `colGrandTotals="0"`, `<item x="2" h="1"/>`, `location ref="G3:J8"`,
	}
	for _, quiero := range quiero {
		if !strings.Contains(tabla, quiero) {
			t.Errorf("la tabla no lleva %s:\n%s", quiero, tabla)
		}
	}
	if !strings.Contains(parts["xl/workbook.xml"], `<pivotCache cacheId="1"`) ||
		!strings.Contains(parts["[Content_Types].xml"], "pivotTable+xml") {
		t.Error("el libro no declara la caché o los tipos")
	}
	_, modelo, err := importXlsx(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := dinamicasDe(t, modelo); !reflect.DeepEqual(got, original(t)) {
		t.Errorf("vuelve distinta:\n%v\n%v", got, original(t))
	}
}

// TestLaDinamicaDeOtraHojaDeCalculoSeEntiende: un .xlsx sin la extensión de Sytue —lo
// que escribe Excel— se reconstruye de sus partes normalizadas.
func TestLaDinamicaDeOtraHojaDeCalculoSeEntiende(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", libroConDinamica)
	data, err := buildXlsx(s)
	if err != nil {
		t.Fatal(err)
	}
	files, err := readZip(data)
	if err != nil {
		t.Fatal(err)
	}
	sinExt := regexp.MustCompile(`<extLst>.*</extLst>`)
	partes := map[string]string{}
	for k, v := range files {
		partes[k] = string(v)
	}
	partes["xl/pivotTables/pivotTable1.xml"] = sinExt.ReplaceAllString(partes["xl/pivotTables/pivotTable1.xml"], "")
	otro, err := zipPackage(partes)
	if err != nil {
		t.Fatal(err)
	}
	_, modelo, err := importXlsx(otro, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := dinamicasDe(t, modelo)
	if len(got) != 1 {
		t.Fatalf("salen %d tablas", len(got))
	}
	b, _ := json.Marshal(got[0])
	var p pivotSpec
	_ = json.Unmarshal(b, &p)
	if p.Name != "TablaDinámica1" || p.Source != "A1:D6" || p.Anchor != "G1" || len(p.Rows) != 1 || p.Rows[0].Field != 0 ||
		!reflect.DeepEqual(p.Rows[0].Hidden, []string{"Este"}) || len(p.Cols) != 1 || p.Cols[0].Field != 2 ||
		len(p.Filters) != 1 || p.Filters[0].Field != 1 || len(p.Values) != 2 || p.Values[0].Fn != "avg" ||
		p.Values[0].Show != "pctGrand" || p.Values[1].Fn != "count" || p.GrandCols == nil || *p.GrandCols {
		t.Errorf("se reconstruye mal: %s", b)
	}
}

// TestLaDinamicaVaYVuelveEnOdsSinPerder: igual con el piloto de datos de ODF.
func TestLaDinamicaVaYVuelveEnOdsSinPerder(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", libroConDinamica)
	data, _, _, err := exportODF(KindSpreadsheet, "Ventas", s, nil)
	if err != nil {
		t.Fatal(err)
	}
	parts := zipParts(t, data)
	guardarMuestra(t, "dinamica.ods", data)
	content := parts["content.xml"]
	quiero := []string{
		`<table:data-pilot-table table:name="TablaDinámica1"`, `table:target-range-address="Ventas.G1:Ventas.J8"`,
		`table:cell-range-address="Ventas.A1:Ventas.D6"`, `table:orientation="page"`, `table:function="average"`,
		`table:data-pilot-member table:name="Este" table:display="false"`, `table:grand-total="row"`,
	}
	for _, quiero := range quiero {
		if !strings.Contains(content, quiero) {
			t.Errorf("content.xml no lleva %s", quiero)
		}
	}
	_, modelo, err := importOds(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := dinamicasDe(t, modelo); !reflect.DeepEqual(got, original(t)) {
		t.Errorf("vuelve distinta:\n%v\n%v", got, original(t))
	}
	// Sin el atributo de Sytue, se reconstruye.
	sinDef := regexp.MustCompile(` xmlns:sy="[^"]*" sy:pivot="[^"]*"`)
	partes := map[string]string{}
	files, _ := readZip(data)
	for k, v := range files {
		partes[k] = string(v)
	}
	partes["content.xml"] = sinDef.ReplaceAllString(partes["content.xml"], "")
	otro, err := zipPackage(partes)
	if err != nil {
		t.Fatal(err)
	}
	_, modelo, err = importOds(otro, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := dinamicasDe(t, modelo)
	b, _ := json.Marshal(got)
	if len(got) != 1 || !strings.Contains(string(b), `"hidden":["Este"]`) || !strings.Contains(string(b), `"fn":"avg"`) ||
		!strings.Contains(string(b), `"filters":[{"field":1}]`) {
		t.Errorf("de un ODS ajeno sale %s", b)
	}
}

// guardarMuestra deja el fichero donde diga SYTUE_MUESTRAS, para abrirlo con
// LibreOffice o Excel.
func guardarMuestra(t *testing.T, nombre string, data []byte) {
	t.Helper()
	dir := os.Getenv("SYTUE_MUESTRAS")
	if dir == "" {
		return
	}
	if err := os.WriteFile(dir+"/"+nombre, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// libroConFiltroOculto es una tabla dinámica filtrada por una segmentación de un campo
// que no está en ella (Producto): el filtro oculto.
const libroConFiltroOculto = `{"active":0,"sheets":[{"name":"Ventas","cells":{` +
	`"A1":"Zona","B1":"Producto","C1":"Importe","A2":"Norte","B2":"Sillas","C2":"150",` +
	`"A3":"Norte","B3":"Mesas","C3":"300","A4":"Sur","B4":"Mesas","C4":"600","A5":"Sur","B5":"Sillas","C5":"200"},` +
	`"pivots":[{"name":"TablaDinámica1","source":"A1:C5","anchor":"E1","rows":[{"field":0}],"cols":[],` +
	`"values":[{"field":2,"fn":"sum"}],"hiddenFilters":[{"field":1,"hidden":["Mesas"]}]}]}]}`

// TestElFiltroOcultoVaYVuelve: el filtro oculto sale como lo dicen Excel (pivotField
// sin eje con elementos ocultos) y Calc (campo «hidden» con sus miembros), y se
// reconstruye de ahí cuando el fichero no trae la definición de Sytue.
func TestElFiltroOcultoVaYVuelve(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", libroConFiltroOculto)
	quiero := dinamicasDe(t, libroConFiltroOculto)
	reconstruida := func(t *testing.T, modelo string) {
		t.Helper()
		got := dinamicasDe(t, modelo)
		b, _ := json.Marshal(got)
		if len(got) != 1 || !strings.Contains(string(b), `"hiddenFilters":[{"field":1,"hidden":["Mesas"]}]`) {
			t.Errorf("se reconstruye sin el filtro oculto: %s", b)
		}
	}
	quitar := func(t *testing.T, data []byte, parte string, re *regexp.Regexp) []byte {
		t.Helper()
		files, err := readZip(data)
		if err != nil {
			t.Fatal(err)
		}
		partes := map[string]string{}
		for k, v := range files {
			partes[k] = string(v)
		}
		partes[parte] = re.ReplaceAllString(partes[parte], "")
		otro, err := zipPackage(partes)
		if err != nil {
			t.Fatal(err)
		}
		return otro
	}

	xlsx, err := buildXlsx(s)
	if err != nil {
		t.Fatal(err)
	}
	guardarMuestra(t, "filtro-oculto.xlsx", xlsx)
	tabla := zipParts(t, xlsx)["xl/pivotTables/pivotTable1.xml"]
	if !regexp.MustCompile(`<pivotField compact="0" outline="0" showAll="0"><items count="3"><item x="0"/><item x="1" h="1"/>`).MatchString(tabla) &&
		!regexp.MustCompile(`<pivotField compact="0" outline="0" showAll="0"><items count="3"><item x="0" h="1"/><item x="1"/>`).MatchString(tabla) {
		t.Errorf("Producto no va como campo sin eje con Mesas oculto:\n%s", tabla)
	}
	_, modelo, err := importXlsx(xlsx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := dinamicasDe(t, modelo); !reflect.DeepEqual(got, quiero) {
		t.Errorf("vuelve distinta de .xlsx:\n%v\n%v", got, quiero)
	}
	_, modelo, err = importXlsx(quitar(t, xlsx, "xl/pivotTables/pivotTable1.xml", regexp.MustCompile(`<extLst>.*</extLst>`)), nil)
	if err != nil {
		t.Fatal(err)
	}
	reconstruida(t, modelo)

	ods, _, _, err := exportODF(KindSpreadsheet, "Ventas", s, nil)
	if err != nil {
		t.Fatal(err)
	}
	guardarMuestra(t, "filtro-oculto.ods", ods)
	if !strings.Contains(zipParts(t, ods)["content.xml"], `table:orientation="hidden"><table:data-pilot-level table:show-empty="false"><table:data-pilot-members>`) ||
		!strings.Contains(zipParts(t, ods)["content.xml"], `table:name="Mesas" table:display="false"`) {
		t.Error("el .ods no lleva Producto como campo oculto con Mesas fuera")
	}
	_, modelo, err = importOds(ods, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := dinamicasDe(t, modelo); !reflect.DeepEqual(got, quiero) {
		t.Errorf("vuelve distinta de .ods:\n%v\n%v", got, quiero)
	}
	_, modelo, err = importOds(quitar(t, ods, "content.xml", regexp.MustCompile(` xmlns:sy="[^"]*" sy:pivot="[^"]*"`)), nil)
	if err != nil {
		t.Fatal(err)
	}
	reconstruida(t, modelo)
}

// TestElFiltroOcultoDeCalcSeLee: Calc 7.4 entiende el filtro oculto de nuestro .xlsx y lo
// guarda como campo «hidden» con sus miembros (testdata/calcus/filtro-oculto-de-xlsx.ods
// es el .xlsx de libroConFiltroOculto abierto y guardado en .ods por Calc), y de ahí se
// lee. Al guardar en .xlsx, Calc lo tira por su cuenta: solo escribe los elementos de
// los campos que van en un eje.
func TestElFiltroOcultoDeCalcSeLee(t *testing.T) {
	datos, err := os.ReadFile("testdata/calcus/filtro-oculto-de-xlsx.ods")
	if err != nil {
		t.Fatal(err)
	}
	_, modelo, err := importOds(datos, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(dinamicasDe(t, modelo))
	if !strings.Contains(string(b), `"hiddenFilters":[{"field":1,"hidden":["Mesas"]}]`) {
		t.Errorf("el filtro oculto no llega de Calc: %s", b)
	}
}

// libroSinSubtotales es una dinámica con dos campos de fila y los subtotales apagados: lo que
// el editor llama «sin subtotales» tiene que sobrevivir a los dos formatos (defaultSubtotal="0"
// en Excel, ausencia de data-pilot-subtotal en Calc) y no volver a encenderse.
const libroSinSubtotales = `{"active":0,"sheets":[{"name":"Ventas","cells":{` +
	`"A1":"Zona","B1":"Producto","C1":"Importe","A2":"Norte","B2":"Sillas","C2":"150",` +
	`"A3":"Norte","B3":"Mesas","C3":"300","A4":"Sur","B4":"Mesas","C4":"600","A5":"Sur","B5":"Sillas","C5":"200"},` +
	`"pivots":[{"name":"TablaDinámica1","source":"A1:C5","anchor":"E1","range":"E1:G9","rows":[{"field":0},{"field":1}],"cols":[],` +
	`"values":[{"field":2,"fn":"sum"}],"subtotals":false}]}]}`

// TestLosSubtotalesDeLaDinamicaVanYVuelven: apagados, siguen apagados tras .xlsx y .ods, y
// encendidos (el defecto) no aparece la clave.
func TestLosSubtotalesDeLaDinamicaVanYVuelven(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", libroSinSubtotales)
	xlsx, err := buildXlsx(s)
	if err != nil {
		t.Fatal(err)
	}
	ods, _, _, err := exportODF(KindSpreadsheet, "Ventas", s, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, deXlsx, err := importXlsx(xlsx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, deOds, err := importOds(ods, nil)
	if err != nil {
		t.Fatal(err)
	}
	for nombre, modelo := range map[string]string{"xlsx": deXlsx, "ods": deOds} {
		b, _ := json.Marshal(dinamicasDe(t, modelo))
		if !strings.Contains(string(b), `"subtotals":false`) {
			t.Errorf("%s: los subtotales apagados no vuelven apagados: %s", nombre, b)
		}
	}
	if !strings.Contains(zipParts(t, xlsx)["xl/pivotTables/pivotTable1.xml"], `defaultSubtotal="0"`) {
		t.Error("la dinámica de Excel no lleva defaultSubtotal=\"0\"")
	}
	// El defecto: nada que apagar.
	con := stateWithContent(t, "doc:sheet", strings.Replace(libroSinSubtotales, `,"subtotals":false`, "", 1))
	data, err := buildXlsx(con)
	if err != nil {
		t.Fatal(err)
	}
	_, modelo, err := importXlsx(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(dinamicasDe(t, modelo)); strings.Contains(string(b), `subtotals`) {
		t.Errorf("con subtotales no debería llevar la clave: %s", b)
	}
}

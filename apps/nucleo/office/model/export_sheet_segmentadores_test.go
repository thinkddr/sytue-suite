// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// libroConSegmentadoresSytue: la hoja «Datos» con la tabla Ventas (A1:C4), la dinámica
// Dinamica1 sobre ella (Mesas fuera por su segmentador) y tres segmentadores: el de la
// columna Producto de la tabla, el del campo Producto de la dinámica y uno del autofiltro
// en la columna H, que no cae en ninguna tabla y Excel no sabe pintar.
const libroConSegmentadoresSytue = `{"active":0,"sheets":[{"name":"Datos","cells":{` +
	`"A1":"Zona","B1":"Producto","C1":"Importe",` +
	`"A2":"Norte","B2":"Sillas","C2":"10","A3":"Sur","B3":"Mesas","C3":"20","A4":"Sur","B4":"Sillas","C4":"30",` +
	`"H1":"Otra","H2":"x"},` +
	`"tables":[{"name":"Ventas","range":"A1:C4"}],` +
	`"autofilter":{"row":0,"c1":0,"c2":7},` +
	`"pivots":[{"name":"Dinamica1","source":"A1:C4","anchor":"E1","range":"E1:F3",` +
	`"rows":[{"field":0}],"values":[{"field":2,"fn":"sum"}],"hiddenFilters":[{"field":1,"hidden":["Mesas"]}]}],` +
	`"slicers":[{"id":"s1","col":1,"x":768,"y":48},{"id":"s2","col":1,"pivot":"Dinamica1","x":960,"y":48},{"id":"s3","col":7}]` +
	`}]}`

// segmentadoresDe son los segmentadores de la primera hoja de un modelo.
func segmentadoresDe(t *testing.T, modelo string) []sheetSlicer {
	t.Helper()
	var m struct {
		Sheets []struct {
			Slicers []sheetSlicer `json:"slicers"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal([]byte(modelo), &m); err != nil || len(m.Sheets) == 0 {
		t.Fatalf("modelo ilegible: %v", err)
	}
	return m.Sheets[0].Slicers
}

// sinExtensionSytue es el .xlsx sin la extensión `sy:slicers` de sus hojas: lo que queda es
// lo que leería Excel.
func sinExtensionSytue(t *testing.T, data []byte) []byte {
	t.Helper()
	files, err := readZip(data)
	if err != nil {
		t.Fatal(err)
	}
	partes := map[string]string{}
	re := regexp.MustCompile(`<ext uri="` + regexp.QuoteMeta(uriSytue) + `"[^>]*><sy:slicers>.*?</sy:slicers></ext>`)
	for k, v := range files {
		partes[k] = string(v)
		if strings.HasPrefix(k, "xl/worksheets/sheet") {
			partes[k] = re.ReplaceAllString(partes[k], "")
		}
	}
	otro, err := zipPackage(partes)
	if err != nil {
		t.Fatal(err)
	}
	return otro
}

// TestLosSegmentadoresSalenComoLosDeExcel: los de una tabla y los de una dinámica salen con
// las partes de [MS-XLSX] —caché, lista, ancla del dibujo, `Slicer_*` y extLst del libro y de
// la hoja—; el del autofiltro sin tabla, no (Excel no sabe segmentar eso).
func TestLosSegmentadoresSalenComoLosDeExcel(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", libroConSegmentadoresSytue)
	data, err := buildXlsx(s)
	if err != nil {
		t.Fatal(err)
	}
	partes := zipParts(t, data) // además, todo el XML bien formado
	guardarMuestra(t, "segmentadores.xlsx", data)

	libro := partes["xl/workbook.xml"]
	for _, quiero := range []string{
		`<definedName name="Slicer_Producto">#N/A</definedName>`,
		`<definedName name="Slicer_Producto1">#N/A</definedName>`,
		`<ext uri="` + uriCachesDinamica + `"`, `<ext uri="` + uriCachesTabla + `"`,
	} {
		if !strings.Contains(libro, quiero) {
			t.Errorf("el libro no lleva %s:\n%s", quiero, libro)
		}
	}
	// El extLst cierra el libro, tras las <pivotCaches> (CT_Workbook).
	if !regexp.MustCompile(`</pivotCaches><extLst>.*</extLst></workbook>$`).MatchString(libro) {
		t.Errorf("el extLst no va al final del libro:\n%s", libro)
	}
	cacheTabla, cacheDinamica := partes["xl/slicerCaches/slicerCache1.xml"], partes["xl/slicerCaches/slicerCache2.xml"]
	if !strings.Contains(cacheTabla, `sourceName="Producto"`) || !strings.Contains(cacheTabla, `<x15:tableSlicerCache tableId="1" column="2"/>`) {
		t.Errorf("caché del de tabla:\n%s", cacheTabla)
	}
	// Los elementos compartidos de Producto son Sillas (0) y Mesas (1): Mesas, fuera.
	if !strings.Contains(cacheDinamica, `<pivotTable tabId="1" name="Dinamica1"/>`) ||
		!strings.Contains(cacheDinamica, `<tabular pivotCacheId="1"><items count="2"><i x="0" s="1"/><i x="1"/></items>`) {
		t.Errorf("caché del de dinámica:\n%s", cacheDinamica)
	}
	// La caché de la dinámica dice su pivotCacheId y escribe los elementos de Producto.
	pc := partes["xl/pivotCache/pivotCacheDefinition1.xml"]
	if !strings.Contains(pc, `<x14:pivotCacheDefinition pivotCacheId="1"/>`) || !strings.Contains(pc, `<cacheField name="Producto" numFmtId="0"><sharedItems count="2">`) {
		t.Errorf("la caché de la dinámica no lleva lo que cita el segmentador:\n%s", pc)
	}
	hoja := partes["xl/worksheets/sheet1.xml"]
	for _, uri := range []string{uriListaDinamica, uriListaTabla, uriSytue} {
		if !strings.Contains(hoja, `<ext uri="`+uri+`"`) {
			t.Errorf("la hoja no lleva la lista %s", uri)
		}
	}
	if strings.Count(partes["xl/drawings/drawing1.xml"], "<sle:slicer ") != 2 {
		t.Errorf("el dibujo no ancla las dos tarjetas:\n%s", partes["xl/drawings/drawing1.xml"])
	}
	tipos := partes["[Content_Types].xml"]
	if strings.Count(tipos, ctSlicerCch) != 2 || strings.Count(tipos, ctSlicer) != 2 {
		t.Errorf("faltan los tipos de las partes:\n%s", tipos)
	}
	// El del autofiltro (columna H) no sale a Excel.
	for ruta, contenido := range partes {
		if strings.HasPrefix(ruta, "xl/slicer") && strings.Contains(contenido, "Otra") {
			t.Errorf("el segmentador del autofiltro sin tabla sale en %s", ruta)
		}
	}
}

// TestLosSegmentadoresVanYVuelvenPorExcel: sin la extensión de Sytue (lo que deja Excel), la
// importación los reconstruye de las partes de Excel con su columna, su dinámica y su
// posición; con ella, vuelven idénticos y sin repetirse.
func TestLosSegmentadoresVanYVuelvenPorExcel(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", libroConSegmentadoresSytue)
	data, err := buildXlsx(s)
	if err != nil {
		t.Fatal(err)
	}
	_, modelo, err := importXlsx(sinExtensionSytue(t, data), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := segmentadoresDe(t, modelo)
	if len(got) != 2 {
		t.Fatalf("de las partes de Excel salen %d segmentadores, y son 2: %+v", len(got), got)
	}
	// Las listas van por clase (la de dinámica, luego la de tabla): el orden no se conserva.
	tabla, din := got[0], got[1]
	if tabla.Pivot != "" {
		tabla, din = din, tabla
	}
	if tabla.Col != 1 || tabla.Pivot != "" || tabla.X == nil || *tabla.X != 768 || *tabla.Y != 48 {
		t.Errorf("el de tabla vuelve como %+v", tabla)
	}
	if din.Col != 1 || din.Pivot != "Dinamica1" || din.X == nil || *din.X != 960 || *din.Y != 48 {
		t.Errorf("el de dinámica vuelve como %+v", din)
	}
	if strings.Contains(modelo, "Slicer_") {
		t.Errorf("los Slicer_* (#N/A) vuelven como nombres del usuario:\n%s", modelo)
	}

	_, modelo, err = importXlsx(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, quiero := segmentadoresDe(t, modelo), segmentadoresDe(t, libroConSegmentadoresSytue); !reflect.DeepEqual(got, quiero) {
		t.Errorf("con la extensión de Sytue vuelven distintos:\n%+v\n%+v", got, quiero)
	}
}

// TestSegmentadoresConNombresRepetidosYSinSitio: dos de columnas con el mismo nombre dan
// nombres de tarjeta y de caché únicos, y uno sin posición se ancla a la derecha de su tabla.
func TestSegmentadoresConNombresRepetidosYSinSitio(t *testing.T) {
	libro := strings.Replace(libroConSegmentadoresSytue, `"C1":"Importe"`, `"C1":"Producto"`, 1)
	libro = strings.Replace(libro, `{"id":"s3","col":7}`, `{"id":"s3","col":2}`, 1)
	s := stateWithContent(t, "doc:sheet", libro)
	data, err := buildXlsx(s)
	if err != nil {
		t.Fatal(err)
	}
	partes := zipParts(t, data)
	var lista strings.Builder
	for ruta, c := range partes {
		if strings.HasPrefix(ruta, "xl/slicers/") {
			lista.WriteString(c)
		}
	}
	nombres := regexp.MustCompile(` name="([^"]+)" cache="([^"]+)"`).FindAllStringSubmatch(lista.String(), -1)
	vistos, caches := map[string]bool{}, map[string]bool{}
	for _, n := range nombres {
		if vistos[n[1]] || caches[n[2]] {
			t.Errorf("nombre repetido: %v", n)
		}
		vistos[n[1]], caches[n[2]] = true, true
	}
	if len(nombres) != 3 {
		t.Fatalf("salen %d segmentadores y son 3: %v", len(nombres), nombres)
	}
	_, modelo, err := importXlsx(sinExtensionSytue(t, data), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, sg := range segmentadoresDe(t, modelo) {
		if sg.Col == 2 && (sg.X == nil || *sg.X != 4*anchoCeldaPx || *sg.Y != 0) {
			t.Errorf("el que no tenía sitio va a %v,%v y no a la derecha de la tabla", sg.X, sg.Y)
		}
	}
}

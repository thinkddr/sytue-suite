// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

const espacios09 = `xmlns:sf="http://developer.apple.com/namespaces/sf" xmlns:sfa="http://developer.apple.com/namespaces/sfa" ` +
	`xmlns:sl="http://developer.apple.com/namespaces/sl" xmlns:ls="http://developer.apple.com/namespaces/ls" ` +
	`xmlns:key="http://developer.apple.com/namespaces/keynote2"`

// tabla09Prueba: 3 filas × 3 columnas. La primera celda salta con sf:ct a (0,1); luego
// avanzan de una en una, y un sf:ct de 254 pasa a la fila siguiente una columna atrás.
const tabla09Prueba = `<sf:tabular-model sf:name="Gastos"><sf:grid sf:numrows="3" sf:numcols="3"><sf:datasource>` +
	`<sf:t sf:ct="1" sf:s="cs-cab"><sf:ct sfa:s="Concepto"/></sf:t>` + // (0,1)
	`<sf:t><sf:ct sfa:s="Importe &amp; más"/></sf:t>` + // (0,2)
	`<sf:t sf:ct="254"><sf:ct sfa:s="Luz"/></sf:t>` + // (1,0)
	`<sf:n sf:v="-97.400000000000006" sf:s="cs-eur"/>` + // (1,1)
	`<sf:d sf:cell-date="276048000"/>` + // (1,2): 2009-10-01
	`<sf:pm sf:ct="254"><sf:menu-choices><sf:t sfa:ID="op-1"><sf:ct sfa:s="Sí"/></sf:t><sf:t sfa:ID="op-2"><sf:ct sfa:s="No"/></sf:t></sf:menu-choices><sf:proxied-cell-ref sfa:IDREF="op-2"/></sf:pm>` + // (2,0)
	`<sf:f><sf:fo sf:fs="=SUM(B)"/><sf:r><sf:rn sf:v="12.5"><sf:cf><sf:number-format sf:format-type="2" sf:format-decimal-places="1"/></sf:cf></sf:rn></sf:r></sf:f>` + // (2,1)
	`<sf:g/>` + // (2,2)
	`</sf:datasource></sf:grid></sf:tabular-model>`

const estilosCelda09Prueba = `<sf:stylesheet>` +
	`<sf:cell-style sfa:ID="cs-cab"><sf:property-map>` +
	`<sf:fill><sf:color sfa:r="1" sfa:g="0.5" sfa:b="0"/></sf:fill>` +
	`</sf:property-map></sf:cell-style>` +
	`<sf:cell-style sfa:ID="cs-eur"><sf:property-map>` +
	`<sf:SFTCellStylePropertyFormatType><sf:number sfa:number="257"/></sf:SFTCellStylePropertyFormatType>` +
	`<sf:SFTCellStylePropertyNumberFormat><sf:number-format sf:format-type="1" sf:format-decimal-places="2" sf:format-show-thousands-separator="true"/></sf:SFTCellStylePropertyNumberFormat>` +
	`</sf:property-map></sf:cell-style>` +
	`</sf:stylesheet>`

func TestNumbers09ConvierteLasCeldas(t *testing.T) {
	tablaConCb := strings.Replace(tabla09Prueba, `<sf:g/>`, `<sf:cb sf:v="1"/>`, 1)
	indice := `<?xml version="1.0"?><ls:document ` + espacios09 + `>` + estilosCelda09Prueba + `<ls:workspace-array>` +
		`<ls:workspace ls:workspace-name="Hoja 1"><ls:page-info><sf:layers><sf:layer><sf:drawables><sf:tabular-info>` + tablaConCb + `</sf:tabular-info>` +
		`<sf:chart-info/></sf:drawables></sf:layer></sf:layers></ls:page-info></ls:workspace>` +
		`</ls:workspace-array><ls:tabular-prototypes>` + strings.Replace(tabla09Prueba, "Concepto", "PLANTILLA", 1) + `</ls:tabular-prototypes></ls:document>`
	res, err := ImportLocalized("cuentas.numbers", pages09(t, indice, nil), false, "es")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindSpreadsheet {
		t.Fatalf("kind %q:\n%.300s", res.Kind, res.Content)
	}
	var m sheetModel
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Sheets) != 2 || m.Sheets[0].Name != "Aviso" || m.Sheets[1].Name != "Hoja 1" {
		t.Fatalf("hojas: %+v", m.Sheets)
	}
	aviso := m.Sheets[0].Cells
	if !strings.Contains(aviso["A1"], "Conversión parcial · hoja .numbers") ||
		!strings.Contains(aviso["A2"], "Las fórmulas") || !strings.Contains(aviso["A3"], "gráficos") {
		t.Errorf("aviso: %v", aviso)
	}
	quiero := map[string]string{
		"B1": "Concepto", "C1": "Importe &amp; más", "A2": "Luz", "B2": "-97.4", "C2": "40087",
		"A3": "No", "B3": "12.5", "C3": "TRUE",
	}
	celdas := m.Sheets[1].Cells
	for ref, v := range quiero {
		if celdas[ref] != v {
			t.Errorf("%s = %q, quiero %q", ref, celdas[ref], v)
		}
	}
	if len(celdas) != len(quiero) {
		t.Errorf("sobran celdas: %v", celdas)
	}
	if m.Sheets[1].Formats["C2"].ID != "date" {
		t.Errorf("C2 sin formato de fecha: %v", m.Sheets[1].Formats)
	}
	if f := m.Sheets[1].Formats["B2"]; f.ID != "currency" || f.Decimals == nil || *f.Decimals != 2 {
		t.Errorf("B2 formato %+v, quiero currency con 2 decimales", f)
	}
	if f := m.Sheets[1].Formats["B3"]; f.ID != "percent" || f.Decimals == nil || *f.Decimals != 1 {
		t.Errorf("B3 formato %+v, quiero percent con 1 decimal", f)
	}
	if st := m.Sheets[1].Styles["B1"]; st.Fill != "#FF8000" {
		t.Errorf("B1 estilo %+v, quiero relleno #FF8000", st)
	}
}

// Una hoja sin fórmulas ni nada que se quede fuera no lleva aviso; dos tablas en la
// misma hoja salen como dos hojas con el nombre de cada una.
func TestNumbers09SinAvisoYVariasTablas(t *testing.T) {
	sinFormula := strings.Replace(tabla09Prueba, `<sf:f><sf:fo sf:fs="=SUM(B)"/><sf:r><sf:rn sf:v="12.5"><sf:cf><sf:number-format sf:format-type="2" sf:format-decimal-places="1"/></sf:cf></sf:rn></sf:r></sf:f>`, `<sf:g/>`, 1)
	otra := strings.Replace(sinFormula, `sf:name="Gastos"`, `sf:name="Ingresos"`, 1)
	indice := `<?xml version="1.0"?><ls:document ` + espacios09 + `><ls:workspace-array>` +
		`<ls:workspace ls:workspace-name="Casa">` + sinFormula + otra + `</ls:workspace>` +
		`</ls:workspace-array></ls:document>`
	res, err := Import("cuentas.numbers", pages09(t, indice, nil))
	if err != nil {
		t.Fatal(err)
	}
	var m sheetModel
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Sheets) != 2 || m.Sheets[0].Name != "Casa · Gastos" || m.Sheets[1].Name != "Casa · Ingresos" {
		t.Errorf("hojas: %+v", m.Sheets)
	}
}

// Una tabla anclada en el texto de un .pages es una tabla del documento, en su sitio.
func TestPages09ConTablaEnElTexto(t *testing.T) {
	indice := `<?xml version="1.0"?><sl:document ` + espacios09 + `><sf:text-storage sf:kind="body">` +
		`<sf:attachments><sf:attachment sfa:ID="tabla-0"><sf:tabular-info>` + tabla09Prueba + `</sf:tabular-info></sf:attachment></sf:attachments>` +
		`<sf:text-body><sf:p>Antes<sf:attachment-ref sfa:IDREF="tabla-0"/>después<sf:br/></sf:p></sf:text-body></sf:text-storage></sl:document>`
	res, err := Import("informe.pages", pages09(t, indice, nil))
	if err != nil {
		t.Fatal(err)
	}
	const quiero = `<p>Antes</p><table><tbody><tr><td></td><td>Concepto</td><td>Importe &amp; más</td></tr>` +
		`<tr><td>Luz</td><td>-97.4</td><td>2009-10-01</td></tr><tr><td>No</td><td>12.5</td><td></td></tr></tbody></table><p>después</p>`
	if res.Content != quiero {
		t.Errorf("tabla en el texto:\n%s\nquiero:\n%s", res.Content, quiero)
	}
}

func TestKeynote09ConvierteLasDiapositivas(t *testing.T) {
	indice := `<?xml version="1.0"?><key:presentation ` + espacios09 + `><key:size sfa:w="1024" sfa:h="768"/>` +
		`<key:theme-list><key:theme><key:master-slides><key:master-slide><key:title-placeholder><key:text><sf:text-storage><sf:text-body><sf:p>MAESTRO</sf:p></sf:text-body></sf:text-storage></key:text></key:title-placeholder></key:master-slide></key:master-slides></key:theme></key:theme-list>` +
		`<key:slide-list><key:slide>` +
		`<key:title-placeholder><key:text><sf:text-storage><sf:text-body><sf:p sf:list-level="1">Título</sf:p></sf:text-body></sf:text-storage></key:text></key:title-placeholder>` +
		`<key:body-placeholder><key:text><sf:text-storage><sf:text-body>` + "\n      " +
		`<sf:p sf:list-level="1">Uno<sf:br/></sf:p>` + "\n      " + `<sf:p sf:list-level="2">Uno.a</sf:p></sf:text-body></sf:text-storage></key:text></key:body-placeholder>` +
		`<key:page><sf:layers><sf:layer><sf:drawables>` +
		`<sf:shape><sf:geometry><sf:size sfa:w="512" sfa:h="76.8"/><sf:position sfa:x="256" sfa:y="384"/></sf:geometry><sf:text><sf:text-storage><sf:text-body><sf:p>Cuadro</sf:p></sf:text-body></sf:text-storage></sf:text></sf:shape>` +
		`<sf:sticky-note><sf:text-storage><sf:text-body><sf:p>COMENTARIO</sf:p></sf:text-body></sf:text-storage></sf:sticky-note>` +
		`<sf:image/>` +
		`</sf:drawables></sf:layer></sf:layers></key:page>` +
		`<key:notes><sf:text-storage><sf:text-body><sf:p>Nota 1</sf:p><sf:p>Nota 2</sf:p></sf:text-body></sf:text-storage></key:notes>` +
		`</key:slide><key:slide><key:title-placeholder><key:text><sf:text-storage><sf:text-body><sf:p/></sf:text-body></sf:text-storage></key:text></key:title-placeholder></key:slide>` +
		`</key:slide-list></key:presentation>`
	paquete := zipDe(t, map[string][]byte{"index.apxl": []byte(indice), "QuickLook/Thumbnail.jpg": jpegDe(t, 40, 30)})
	res, err := ImportLocalized("charla.key", paquete, false, "es")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindPresentation {
		t.Fatalf("kind %q:\n%.300s", res.Kind, res.Content)
	}
	var m slidesModel
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatal(err)
	}
	if m.Size == nil || m.Size.Preset != "4:3" {
		t.Errorf("tamaño: %+v", m.Size)
	}
	// Dos diapositivas y, porque la imagen no se convierte, una tercera con el aviso.
	if len(m.Slides) != 3 {
		t.Fatalf("%d diapositivas:\n%s", len(m.Slides), res.Content)
	}
	s := m.Slides[0]
	if len(s.Boxes) != 3 {
		t.Fatalf("cajas: %+v", s.Boxes)
	}
	if s.Boxes[0].HTML != "<p>Título</p>" || s.Boxes[1].HTML != "<ul><li>Uno<ul><li>Uno.a</li></ul></li></ul>" {
		t.Errorf("título y cuerpo: %q, %q", s.Boxes[0].HTML, s.Boxes[1].HTML)
	}
	if b := s.Boxes[2]; b.HTML != "<p>Cuadro</p>" || b.X != 25 || b.Y != 50 || b.W != 50 || b.H != 10 {
		t.Errorf("cuadro: %+v", b)
	}
	if s.Notes != "Nota 1\nNota 2" {
		t.Errorf("notas: %q", s.Notes)
	}
	if strings.Contains(res.Content, "MAESTRO") || strings.Contains(res.Content, "COMENTARIO") {
		t.Errorf("sobra el maestro o la nota adhesiva:\n%s", res.Content)
	}
	ultima := m.Slides[2]
	if len(ultima.Boxes) != 2 || !strings.Contains(ultima.Boxes[0].HTML, "Conversión parcial · presentación .key") ||
		ultima.Boxes[1].Kind != "image" || !strings.HasPrefix(ultima.Boxes[1].Src, "data:image/jpeg;base64,") {
		t.Errorf("la última diapositiva no es el aviso con la vista previa: %+v", ultima.Boxes)
	}
}

func TestKeynote09YNumbers09ImagenesYDuracion(t *testing.T) {
	indiceKey := `<?xml version="1.0"?><key:presentation ` + espacios09 + `><key:size sfa:w="1024" sfa:h="768"/>` +
		`<key:slide-list><key:slide>` +
		`<key:page><sf:layers><sf:layer><sf:drawables>` +
		`<sf:image><sf:geometry><sf:size sfa:w="512" sfa:h="384"/><sf:position sfa:x="128" sfa:y="192"/></sf:geometry><sf:binary><sf:data sf:path="Data/diapo.jpg"/></sf:binary></sf:image>` +
		`</sf:drawables></sf:layer></sf:layers></key:page>` +
		`</key:slide></key:slide-list></key:presentation>`
	paqKey := zipDe(t, map[string][]byte{
		"index.apxl":        []byte(indiceKey),
		"Data/diapo.jpg":    jpegDe(t, 64, 48),
		"Data/mt-diapo.jpg": jpegDe(t, 4, 4),
	})
	res, err := ImportLocalized("charla.key", paqKey, false, "es")
	if err != nil {
		t.Fatal(err)
	}
	var mk slidesModel
	if err := json.Unmarshal([]byte(res.Content), &mk); err != nil {
		t.Fatal(err)
	}
	if len(mk.Slides) != 1 || len(mk.Slides[0].Boxes) != 1 {
		t.Fatalf("diapositivas: %+v", mk.Slides)
	}
	if b := mk.Slides[0].Boxes[0]; b.Kind != "image" || !strings.HasPrefix(b.Src, "data:image/jpeg;base64,") || b.X != 12.5 || b.Y != 25 || b.W != 50 || b.H != 50 {
		t.Errorf("imagen en diapositiva: %+v", b)
	}

	indiceNum := `<?xml version="1.0"?><ls:document ` + espacios09 + `><ls:workspace-array>` +
		`<ls:workspace ls:workspace-name="Tiempos"><sf:tabular-info><sf:tabular-model sf:name="T"><sf:grid sf:numrows="1" sf:numcols="2"><sf:datasource>` +
		`<sf:du sf:v="3661"/>` +
		`<sf:f><sf:fo sf:fs="=A1"/><sf:r><sf:rt><sf:ct sfa:s="ok"/></sf:rt></sf:r></sf:f>` +
		`</sf:datasource></sf:grid></sf:tabular-model></sf:tabular-info></ls:workspace>` +
		`</ls:workspace-array></ls:document>`
	resNum, err := Import("tiempos.numbers", pages09(t, indiceNum, nil))
	if err != nil {
		t.Fatal(err)
	}
	var mn sheetModel
	if err := json.Unmarshal([]byte(resNum.Content), &mn); err != nil {
		t.Fatal(err)
	}
	if c := mn.Sheets[len(mn.Sheets)-1].Cells; c["A1"] != "1h 1m 1s" || c["B1"] != "ok" {
		t.Errorf("celdas de duración y resultado de texto: %v", c)
	}
}

func TestIWork09GraficosYCombinadasEnNumbersPagesYKeynote(t *testing.T) {
	const grafico09 = `<sf:chart-info><sf:geometry><sf:size sfa:w="512" sfa:h="384"/><sf:position sfa:x="102.4" sfa:y="76.8"/></sf:geometry>` +
		`<sf:chart-model-object><sf:formula-chart-model sf:chart-direction="0">` +
		`<sf:chart-name sfa:string="Ingresos 2026"/>` +
		`<sf:chart-column_names><sf:string sfa:string="T1"/><sf:string sfa:string="T2"/></sf:chart-column_names>` +
		`<sf:chart-row_names><sf:string sfa:string="Norte"/><sf:string sfa:string="Sur"/></sf:chart-row_names>` +
		`<sf:cached-data>` +
		`<sf:mutable-array><sf:number sfa:number="15"/><sf:number sfa:number="25"/></sf:mutable-array>` +
		`<sf:mutable-array><sf:number sfa:number="35"/><sf:number sfa:number="45"/></sf:mutable-array>` +
		`</sf:cached-data></sf:formula-chart-model></sf:chart-model-object></sf:chart-info>`

	indiceNum := `<?xml version="1.0"?><ls:document ` + espacios09 + `><ls:workspace-array>` +
		`<ls:workspace ls:workspace-name="Anual">` +
		`<sf:tabular-info><sf:tabular-model sf:name="Datos"><sf:grid sf:numrows="2" sf:numcols="3"><sf:datasource>` +
		`<sf:t sf:col-span="2"><sf:ct sfa:s="Cabecera"/></sf:t><sf:g/><sf:t><sf:ct sfa:s="C1"/></sf:t>` +
		`<sf:n sf:v="1"/><sf:n sf:v="2"/><sf:n sf:v="3"/>` +
		`</sf:datasource></sf:grid></sf:tabular-model></sf:tabular-info>` +
		grafico09 +
		`</ls:workspace></ls:workspace-array></ls:document>`
	resNum, err := ImportLocalized("anual.numbers", pages09(t, indiceNum, nil), false, "es")
	if err != nil {
		t.Fatal(err)
	}
	var mn sheetModel
	if err := json.Unmarshal([]byte(resNum.Content), &mn); err != nil {
		t.Fatal(err)
	}
	if len(mn.Sheets) != 2 || mn.Sheets[0].Name == "Aviso" {
		t.Fatalf("hojas (sin Aviso): %+v", mn.Sheets)
	}
	if len(mn.Sheets[0].Merges) != 1 || mn.Sheets[0].Merges[0] != "A1:B1" {
		t.Errorf("combinadas = %v, quiero [A1:B1]", mn.Sheets[0].Merges)
	}
	if g := mn.Sheets[1]; !strings.Contains(g.Name, "Ingresos 2026") || g.Cells["B1"] != "T1" || g.Cells["A2"] != "Norte" || g.Cells["C3"] != "45" {
		t.Errorf("hoja de gráfico '09: %+v", g)
	}

	indicePages := `<?xml version="1.0"?><sl:document ` + espacios09 + `><sf:text-storage sf:kind="body">` +
		`<sf:attachments><sf:attachment sfa:ID="graf-0">` + grafico09 + `</sf:attachment></sf:attachments>` +
		`<sf:text-body><sf:p>Resumen:<sf:attachment-ref sfa:IDREF="graf-0"/></sf:p></sf:text-body></sf:text-storage></sl:document>`
	resPages, err := ImportLocalized("anual.pages", pages09(t, indicePages, nil), false, "es")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resPages.Content, "Conversión parcial") || !strings.Contains(resPages.Content, "<table>") ||
		!strings.Contains(resPages.Content, "Ingresos 2026") || !strings.Contains(resPages.Content, "<td>45</td>") {
		t.Errorf("gráfico en .pages '09:\n%s", resPages.Content)
	}

	indiceKey := `<?xml version="1.0"?><key:presentation ` + espacios09 + `><key:size sfa:w="1024" sfa:h="768"/>` +
		`<key:slide-list><key:slide><key:page><sf:layers><sf:layer><sf:drawables>` + grafico09 +
		`</sf:drawables></sf:layer></sf:layers></key:page></key:slide></key:slide-list></key:presentation>`
	resKey, err := ImportLocalized("anual.key", pages09(t, indiceKey, nil), false, "es")
	if err != nil {
		t.Fatal(err)
	}
	var mk slidesModel
	if err := json.Unmarshal([]byte(resKey.Content), &mk); err != nil {
		t.Fatal(err)
	}
	if len(mk.Slides) != 1 || len(mk.Slides[0].Boxes) != 1 || !strings.Contains(mk.Slides[0].Boxes[0].HTML, "<table>") ||
		!strings.Contains(mk.Slides[0].Boxes[0].HTML, "Norte") || mk.Slides[0].Boxes[0].X != 10 || mk.Slides[0].Boxes[0].Y != 10 {
		t.Errorf("gráfico en .key '09: %+v", mk.Slides)
	}
}

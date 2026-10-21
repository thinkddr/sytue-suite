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
	`<sf:t sf:ct="1"><sf:ct sfa:s="Concepto"/></sf:t>` + // (0,1)
	`<sf:t><sf:ct sfa:s="Importe &amp; más"/></sf:t>` + // (0,2)
	`<sf:t sf:ct="254"><sf:ct sfa:s="Luz"/></sf:t>` + // (1,0)
	`<sf:n sf:v="-97.400000000000006"/>` + // (1,1)
	`<sf:d sf:cell-date="276048000"/>` + // (1,2): 2009-10-01
	`<sf:pm sf:ct="254"><sf:menu-choices><sf:t sfa:ID="op-1"><sf:ct sfa:s="Sí"/></sf:t><sf:t sfa:ID="op-2"><sf:ct sfa:s="No"/></sf:t></sf:menu-choices><sf:proxied-cell-ref sfa:IDREF="op-2"/></sf:pm>` + // (2,0)
	`<sf:f><sf:fo sf:fs="=SUM(B)"/><sf:r><sf:rn sf:v="12.5"/></sf:r></sf:f>` + // (2,1)
	`<sf:g/>` + // (2,2)
	`</sf:datasource></sf:grid></sf:tabular-model>`

func TestNumbers09ConvierteLasCeldas(t *testing.T) {
	indice := `<?xml version="1.0"?><ls:document ` + espacios09 + `><ls:workspace-array>` +
		`<ls:workspace ls:workspace-name="Hoja 1"><ls:page-info><sf:layers><sf:layer><sf:drawables><sf:tabular-info>` + tabla09Prueba + `</sf:tabular-info>` +
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
		"A3": "No", "B3": "12.5",
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
}

// Una hoja sin fórmulas ni nada que se quede fuera no lleva aviso; dos tablas en la
// misma hoja salen como dos hojas con el nombre de cada una.
func TestNumbers09SinAvisoYVariasTablas(t *testing.T) {
	sinFormula := strings.Replace(tabla09Prueba, `<sf:f><sf:fo sf:fs="=SUM(B)"/><sf:r><sf:rn sf:v="12.5"/></sf:r></sf:f>`, `<sf:g/>`, 1)
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

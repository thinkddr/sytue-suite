// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

// El color de la pestaña de una hoja va y vuelve por .ods: antes solo llegaba de un .xlsx, y
// guardar en .ods lo perdía (el exportador no escribía estilos de tabla) y abrir uno de
// LibreOffice no lo leía.
func TestPestanaEnODS(t *testing.T) {
	m := &sheetModel{Sheets: []sheetData{
		{Name: "Ventas", Cells: map[string]string{"A1": "1"}, TabColor: "#C00000"},
		{Name: "Gastos", Cells: map[string]string{"A1": "2"}},
		{Name: "Otra", Cells: map[string]string{"A1": "3"}, TabColor: "#C00000"},
	}}
	data, _, _, err := Export(KindSpreadsheet, "x", stateWithContent(t, "doc:sheet", marshalSheetModel(m)), FormatODF)
	if err != nil {
		t.Fatal(err)
	}
	contenido := unzipPart(t, data, "content.xml")
	if n := strings.Count(contenido, `tableooo:tab-color="#c00000"`); n != 1 {
		t.Errorf("estilos de tabla con la pestaña = %d, quiero 1 (lo comparten las dos hojas)", n)
	}
	res, err := Import("x.ods", data)
	if err != nil {
		t.Fatal(err)
	}
	hojas := parseSheetModelJSON(t, res.Content).Sheets
	pestanas := make([]string, 0, len(hojas))
	for _, h := range hojas {
		pestanas = append(pestanas, h.TabColor)
	}
	if strings.Join(pestanas, ",") != "#C00000,,#C00000" {
		t.Errorf("pestañas a la vuelta = %q", pestanas)
	}
}

// LibreOffice, como caja negra: ve la pestaña del .ods que escribimos (la conserva al pasarlo a
// .xlsx), y la que escribe él en un .ods se lee aquí.
func TestPestanaPorLibreOffice(t *testing.T) {
	modelo := `{"sheets":[{"name":"Ventas","cells":{"A1":"1"},"tabColor":"#C00000"}]}`
	ods, _ := exportarHoja(t, modelo, FormatODF)
	if h := importarHoja(t, "lo.xlsx", sofficeConvertir(t, ods, "ods", "xlsx:Calc MS Excel 2007 XML")); h.TabColor != "#C00000" {
		t.Errorf("LibreOffice no ve la pestaña de nuestro .ods: %q", h.TabColor)
	}
	xlsx, _ := exportarHoja(t, modelo, FormatOOXML)
	if h := importarHoja(t, "lo.ods", sofficeConvertir(t, xlsx, "xlsx", "ods")); h.TabColor != "#C00000" {
		t.Errorf("no se lee la pestaña de un .ods de LibreOffice: %q", h.TabColor)
	}
}

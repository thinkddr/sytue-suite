// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// libroConNombres tiene los cuatro casos de nombre que usa una hoja de verdad: un rango
// del libro con una hoja que lleva espacio, una constante del libro, la misma constante
// con otro valor y ámbito de HOJA (tapa a la del libro en «Datos») y una fórmula.
const libroConNombres = `{"active":0,"sheets":[` +
	`{"name":"Ventas 2026","cells":{"A1":"Mes","B1":"Ventas","B2":"100","B3":"200","B4":"120","B5":"125",` +
	`"D1":"=SUM(Total)","D2":"=D1*Tasa"}},` +
	`{"name":"Datos","cells":{"A1":"7","B1":"=Tasa","B2":"=Doble"}}],` +
	`"names":[{"name":"Total","ref":"'Ventas 2026'!$B$2:$B$5"},{"name":"Tasa","ref":"0.21"},` +
	`{"name":"Tasa","ref":"0.1","sheet":"Datos"},{"name":"Doble","ref":"Datos!$A$1*2","comment":"El doble de A1"}]}`

func nombresDe(t *testing.T, doc string) []definedName {
	t.Helper()
	var m sheetModel
	if err := json.Unmarshal([]byte(doc), &m); err != nil {
		t.Fatalf("el libro no se lee: %v\n%s", err, doc)
	}
	return m.Names
}

// sinComentarios es la lista sin el comentario: ODF no tiene dónde guardarlo.
func sinComentarios(ns []definedName) []definedName {
	out := make([]definedName, len(ns))
	for i, n := range ns {
		n.Comment = ""
		out[i] = n
	}
	return out
}

// TestLosNombresVanYVuelvenDeXlsx: exportar a .xlsx e importar de vuelta da los MISMOS
// nombres, con su ámbito (localSheetId) y su comentario.
func TestLosNombresVanYVuelvenDeXlsx(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", libroConNombres)
	xlsx, err := buildXlsx(s)
	if err != nil {
		t.Fatal(err)
	}
	if destino := os.Getenv("SYTUE_FICHERO"); destino != "" {
		_ = os.WriteFile(destino+".xlsx", xlsx, 0o600) // para abrirlo con LibreOffice
	}
	files, _ := readZip(xlsx)
	wb := string(files["xl/workbook.xml"])
	if !strings.Contains(wb, `<definedName name="Tasa" localSheetId="1">0.1</definedName>`) ||
		!strings.Contains(wb, "</sheets><definedNames>") || !strings.Contains(wb, "</definedNames><calcPr") {
		t.Errorf("workbook.xml no lleva los nombres donde Excel los busca:\n%s", wb)
	}
	_, vuelta, err := importXlsx(xlsx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := nombresDe(t, vuelta), nombresDe(t, libroConNombres); !reflect.DeepEqual(got, want) {
		t.Errorf("los nombres vuelven como\n%+v\nquiero\n%+v", got, want)
	}
}

// TestLosNombresVanYVuelvenDeOds: lo mismo con .ods, donde los de hoja van dentro de su
// tabla y un rango es un named-range.
func TestLosNombresVanYVuelvenDeOds(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", libroConNombres)
	ods, _, _, err := exportODF(KindSpreadsheet, "Nombres", s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if destino := os.Getenv("SYTUE_FICHERO"); destino != "" {
		_ = os.WriteFile(destino+".ods", ods, 0o600)
	}
	files, _ := readZip(ods)
	content := string(files["content.xml"])
	for _, quiero := range []string{
		`<table:named-range table:name="Total" table:base-cell-address="$&apos;Ventas 2026&apos;.$A$1" table:cell-range-address="$&apos;Ventas 2026&apos;.$B$2:.$B$5"/>`,
		`<table:named-expression table:name="Tasa" table:base-cell-address="$&apos;Datos&apos;.$A$1" table:expression="0.1"/></table:named-expressions></table:table>`,
	} {
		if !strings.Contains(content, quiero) {
			t.Errorf("content.xml no lleva %s", quiero)
		}
	}
	_, vuelta, err := importOds(ods, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := nombresDe(t, vuelta)
	// Los de hoja se leen al recorrer su tabla, antes que los del libro.
	want := sinComentarios(nombresDe(t, libroConNombres))
	want = []definedName{want[2], want[0], want[1], want[3]}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("los nombres vuelven como\n%+v\nquiero\n%+v", got, want)
	}
}

// TestLosNombresQueEscribeCalc: lo que guarda LibreOffice tras abrir nuestros .xlsx y
// .ods (testdata/calcus/nombres-*, generados con LibreOffice 7.4 en el contenedor de agentes)
// se lee con los mismos nombres. Es la prueba de que el formato no es solo nuestro.
func TestLosNombresQueEscribeCalc(t *testing.T) {
	want := sinComentarios(nombresDe(t, libroConNombres))
	for _, f := range []string{"nombres-lo-de-xlsx.xlsx", "nombres-lo-de-xlsx.ods", "nombres-lo-de-ods.xlsx", "nombres-lo-de-ods.ods"} {
		datos, err := os.ReadFile("testdata/calcus/" + f)
		if err != nil {
			t.Fatal(err)
		}
		var doc string
		if strings.HasSuffix(f, ".xlsx") {
			_, doc, err = importXlsx(datos, nil)
		} else {
			_, doc, err = importOds(datos, nil)
		}
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		got := sinComentarios(nombresDe(t, doc))
		porClave := map[string]definedName{}
		for _, n := range got {
			porClave[n.Sheet+"!"+n.Name] = n
		}
		for _, n := range want {
			if g, ok := porClave[n.Sheet+"!"+n.Name]; !ok || g.Ref != n.Ref {
				t.Errorf("%s: %s!%s llega como %+v (quiero ref %q)", f, n.Sheet, n.Name, g, n.Ref)
			}
		}
		if len(got) != len(want) {
			t.Errorf("%s: llegan %d nombres, quiero %d: %+v", f, len(got), len(want), got)
		}
	}
}

// TestNombresDeExcel: los que Excel se reserva (`_xlnm.*`) no son de la persona y no se
// importan, y un localSheetId que no apunta a ninguna hoja tampoco.
func TestNombresDeExcel(t *testing.T) {
	uno, nueve := 1, 9
	got := nombresDeXlsx([]xlsxDefinedName{
		{Name: "_xlnm._FilterDatabase", Local: &uno, Ref: "Datos!$A$1:$C$9"},
		{Name: "_xlnm.Print_Area", Ref: "Hoja1!$A$1:$F$40"},
		{Name: "Huérfano", Local: &nueve, Ref: "1"},
		{Name: "IVA", Ref: " =0.21 "},
		{Name: "Local", Local: &uno, Ref: "$B$2"},
	}, []string{"Hoja1", "Datos"})
	want := []definedName{{Name: "IVA", Ref: "0.21"}, {Name: "Local", Ref: "$B$2", Sheet: "Datos"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("quiero %+v, da %+v", want, got)
	}
}

// TestLasDescargasCalculanConNombres: Markdown, HTML y PDF salen del motor; sin los
// nombres, toda fórmula que los usa saldría como #NAME?.
func TestLasDescargasCalculanConNombres(t *testing.T) {
	var m sheetModel
	if err := json.Unmarshal([]byte(libroConNombres), &m); err != nil {
		t.Fatal(err)
	}
	g := sheetGrids(&m)
	// Ventas 2026: D1 = SUM(Total) = 545 y D2 = D1 × 0,21 (la Tasa del libro).
	if d1, d2 := g[0].rows[0][3], g[0].rows[1][3]; d1 != "545" || d2 != "114.45" {
		t.Errorf("Ventas 2026: D1=%q D2=%q, quiero 545 y 114.45", d1, d2)
	}
	// Datos: B1 = la Tasa de la hoja (0,1, no 0,21) y B2 = Doble = A1 × 2.
	if b1, b2 := g[1].rows[0][1], g[1].rows[1][1]; b1 != "0.1" || b2 != "14" {
		t.Errorf("Datos: B1=%q B2=%q, quiero 0.1 y 14", b1, b2)
	}
}

// TestRefOdfAA1: la hoja absoluta y la citada de ODF llegan como las escribe el editor.
func TestRefOdfAA1(t *testing.T) {
	for in, want := range map[string]string{
		".A1":                 "A1",
		"Hoja1.B2":            "Hoja1!B2",
		"$Hoja1.$A$1":         "Hoja1!$A$1",
		"$'Ventas 2026'.$B$2": "'Ventas 2026'!$B$2",
		"'O''Brien'.C3":       "'O''Brien'!C3",
	} {
		if got := odfRefToA1(in); got != want {
			t.Errorf("odfRefToA1(%q) = %q, quiero %q", in, got, want)
		}
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

// libroConObjetos es una hoja de Calcus con todo lo que el editor pone encima de las
// celdas: dos tablas (una con totales y estilo), minigráficos, gráficos de cada forma
// con sus opciones y una segmentación. Es lo que tiene que salir IGUAL tras ir a .xlsx
// y volver.
const libroConObjetos = `{"active":0,"sheets":[{"name":"Ventas 2026","cells":{` +
	`"A1":"Mes","B1":"Ventas","C1":"Coste","D1":"Margen",` +
	`"A2":"Enero","B2":"120","C2":"80","D2":"40","A3":"Febrero","B3":"95.5","C3":"70","D3":"25.5",` +
	`"A4":"Marzo","B4":"150","C4":"90","D4":"-12","A5":"Abril","B5":"180","C5":"100","D5":"80",` +
	`"F1":"Región","G1":"Total","F2":"Norte","G2":"10","F3":"Sur","G3":"20"},` +
	`"tables":[{"name":"Ventas","range":"A1:D6","style":"green","totalsRow":true,"totals":{"1":"sum","2":"average","3":"counta"}},` +
	`{"name":"Regiones","range":"F1:G3","style":"slate"}],` +
	`"sparklines":[{"cell":"E2","range":"B2:D2","type":"line"},{"cell":"E3","range":"B3:D3","type":"column","color":"#16A34A"},` +
	`{"cell":"E4","range":"B4:D4","type":"winloss"}],` +
	`"charts":[` +
	`{"id":"0f3c6a1e-2b4d-4c8e-9a1f-5d6e7f8091a2","type":"bar","range":"A1:C5","title":"Ventas y coste","x":480,"y":24,"labels":true,"xTitle":"Mes","yTitle":"Euros"},` +
	`{"id":"1a2b3c4d-5e6f-4a1b-8c2d-3e4f5a6b7c8d","type":"hbar","range":"A1:D5","stacked":true,"legend":"top","x":480,"y":260,"w":360,"h":200},` +
	`{"id":"2b3c4d5e-6f7a-4b2c-9d3e-4f5a6b7c8d9e","type":"combo","range":"A1:D5","lineSeries":[1,2],"secondaryAxis":[2],"grid":false,"x":900,"y":24},` +
	`{"id":"3c4d5e6f-7a8b-4c3d-8e4f-5a6b7c8d9e0f","type":"donut","range":"A1:B5","legend":"right","colors":["#7C3AED","","#0D9488"],"x":900,"y":260},` +
	`{"id":"4d5e6f7a-8b9c-4d4e-9f5a-6b7c8d9e0f1a","type":"line","range":"A1:D3","byRow":true,"legend":"none","x":10,"y":400},` +
	`{"id":"5e6f7a8b-9c0d-4e5f-8a6b-7c8d9e0f1a2b","type":"scatter","range":"B1:C5","x":300,"y":400},` +
	`{"id":"6f7a8b9c-0d1e-4f6a-9b7c-8d9e0f1a2b3c","type":"area","range":"A1:C5","stacked":true,"x":600,"y":400},` +
	`{"id":"7a8b9c0d-1e2f-4a7b-8c8d-9e0f1a2b3c4d","type":"pie","range":"A1:B5","labels":true,"x":900,"y":400}],` +
	`"slicers":[{"id":"s1","col":0,"x":700,"y":20},{"id":"s2","col":1,"pivot":"TablaDinámica1","x":700,"y":260}]}]}`

type soloObjetos struct {
	Sheets []struct {
		Name  string            `json:"name"`
		Cells map[string]string `json:"cells"`
		objetosHoja
	} `json:"sheets"`
}

func leerObjetos(t *testing.T, doc string) soloObjetos {
	t.Helper()
	var m soloObjetos
	if err := json.Unmarshal([]byte(doc), &m); err != nil {
		t.Fatalf("el libro no se lee: %v\n%s", err, doc)
	}
	return m
}

// TestLosObjetosVanYVuelvenDeXlsx: exportar a .xlsx e importar de vuelta da las MISMAS
// tablas, minigráficos, gráficos (con su id, su posición y todas sus opciones) y
// segmentaciones, y las celdas de siempre.
func TestLosObjetosVanYVuelvenDeXlsx(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", libroConObjetos)
	xlsx, err := buildXlsx(s)
	if err != nil {
		t.Fatal(err)
	}
	if destino := os.Getenv("SYTUE_FICHERO"); destino != "" {
		_ = os.WriteFile(destino+".xlsx", xlsx, 0o600) // para abrirlo con LibreOffice
	}
	_, vuelta, err := importXlsx(xlsx, nil)
	if err != nil {
		t.Fatal(err)
	}
	compararObjetos(t, vuelta)
	// Y el fichero lleva lo que Excel necesita: la fila de totales con SUBTOTAL y las
	// partes de tabla, dibujo y gráfico declaradas.
	files, _ := readZip(xlsx)
	hoja := string(files["xl/worksheets/sheet1.xml"])
	for _, quiero := range []string{"SUBTOTAL(109,Ventas[Ventas])", "SUBTOTAL(101,Ventas[Coste])", "SUBTOTAL(103,Ventas[Margen])", "<tableParts count=\"2\">", "<drawing r:id="} {
		if !strings.Contains(hoja, quiero) {
			t.Errorf("la hoja no lleva %q", quiero)
		}
	}
	tipos := string(files["[Content_Types].xml"])
	for _, quiero := range []string{"/xl/tables/table2.xml", "/xl/drawings/drawing1.xml", "/xl/charts/chart8.xml"} {
		if !strings.Contains(tipos, quiero) {
			t.Errorf("[Content_Types].xml no declara %s", quiero)
		}
	}
}

// TestLosObjetosVanYVuelvenDeOds: lo mismo con .ods.
func TestLosObjetosVanYVuelvenDeOds(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", libroConObjetos)
	ods, _, _, err := exportODF(KindSpreadsheet, "Ventas", s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if destino := os.Getenv("SYTUE_FICHERO"); destino != "" {
		_ = os.WriteFile(destino+".ods", ods, 0o600)
	}
	_, vuelta, err := importOds(ods, nil)
	if err != nil {
		t.Fatal(err)
	}
	compararObjetos(t, vuelta)
	files, _ := readZip(ods)
	manifiesto := string(files["META-INF/manifest.xml"])
	if !strings.Contains(manifiesto, `manifest:full-path="Object 8/" manifest:version="1.3" manifest:media-type="application/vnd.oasis.opendocument.chart"`) {
		t.Error("el manifiesto no declara los gráficos incrustados como documentos de gráfico")
	}
	if !strings.Contains(string(files["content.xml"]), "of:=SUBTOTAL(109;[.B2:.B5])") {
		t.Error("la fila de totales del .ods no lleva su SUBTOTAL en OpenFormula")
	}
}

// compararObjetos exige que el libro de vuelta tenga los objetos y las celdas de
// libroConObjetos.
func compararObjetos(t *testing.T, vuelta string) {
	t.Helper()
	antes, despues := leerObjetos(t, libroConObjetos), leerObjetos(t, vuelta)
	a, d := antes.Sheets[0], despues.Sheets[0]
	if !reflect.DeepEqual(a.Tables, d.Tables) {
		t.Errorf("las tablas no vuelven iguales:\n  antes:   %+v\n  después: %+v", a.Tables, d.Tables)
	}
	if !reflect.DeepEqual(a.Sparklines, d.Sparklines) {
		t.Errorf("los minigráficos no vuelven iguales:\n  antes:   %+v\n  después: %+v", a.Sparklines, d.Sparklines)
	}
	if !reflect.DeepEqual(a.Slicers, d.Slicers) {
		t.Errorf("las segmentaciones no vuelven iguales:\n  antes:   %+v\n  después: %+v", a.Slicers, d.Slicers)
	}
	if len(a.Charts) != len(d.Charts) {
		t.Fatalf("salen %d gráficos y entraron %d", len(d.Charts), len(a.Charts))
	}
	for i := range a.Charts {
		ja, _ := json.Marshal(a.Charts[i])
		jd, _ := json.Marshal(d.Charts[i])
		if !bytes.Equal(ja, jd) {
			t.Errorf("el gráfico %d no vuelve igual:\n  antes:   %s\n  después: %s", i, ja, jd)
		}
	}
	// Las celdas: las mismas, salvo la fila de totales, que en el fichero son SUBTOTAL y
	// al volver se quitan (el editor la calcula).
	for ref, v := range a.Cells {
		if d.Cells[ref] != v {
			t.Errorf("la celda %s era %q y vuelve %q", ref, v, d.Cells[ref])
		}
	}
	for ref := range d.Cells {
		if _, ok := a.Cells[ref]; !ok {
			t.Errorf("aparece la celda %s = %q, que no estaba", ref, d.Cells[ref])
		}
	}
}

// TestLosFicherosDeCalcSeLeen lee ficheros REALES de LibreOffice Calc 7.4: nuestro
// libro exportado a .xlsx y a .ods, abierto y vuelto a guardar por Calc en los dos
// formatos (testdata/calcus). Lo que Calc conserva tiene que llegar entero; lo que Calc
// tira por su cuenta está dicho abajo, caso a caso, para que no parezca culpa nuestra.
func TestLosFicherosDeCalcSeLeen(t *testing.T) {
	antes := leerObjetos(t, libroConObjetos).Sheets[0]
	for _, c := range []struct {
		fichero string
		// Lo que Calc pierde: el id del gráfico al guardar en .xlsx, y el estilo y las
		// funciones de totales de las tablas (la fila de totales solo sobrevive en .xlsx).
		conID, conTotales bool
	}{
		{"lo-de-xlsx.xlsx", false, true},
		{"lo-de-xlsx.ods", true, false},
		{"lo-de-ods.xlsx", false, false},
		{"lo-de-ods.ods", true, false},
	} {
		datos, err := os.ReadFile("testdata/calcus/" + c.fichero)
		if err != nil {
			t.Fatal(err)
		}
		var doc string
		if strings.HasSuffix(c.fichero, ".xlsx") {
			_, doc, err = importXlsx(datos, nil)
		} else {
			_, doc, err = importOds(datos, nil)
		}
		if err != nil {
			t.Fatalf("%s: %v", c.fichero, err)
		}
		d := leerObjetos(t, doc).Sheets[0]
		if !reflect.DeepEqual(antes.Sparklines, d.Sparklines) {
			t.Errorf("%s: los minigráficos llegan como %+v", c.fichero, d.Sparklines)
		}
		if len(d.Tables) != 2 {
			t.Fatalf("%s: llegan %d tablas", c.fichero, len(d.Tables))
		}
		for _, tb := range d.Tables {
			want := map[string]string{"Ventas": "A1:D6", "Regiones": "F1:G3"}[tb.Name]
			if tb.Range != want || (tb.Name == "Ventas" && tb.TotalsRow != c.conTotales) {
				t.Errorf("%s: la tabla %s llega como %+v", c.fichero, tb.Name, tb)
			}
		}
		if len(d.Charts) != len(antes.Charts) {
			t.Fatalf("%s: llegan %d gráficos de %d", c.fichero, len(d.Charts), len(antes.Charts))
		}
		for i, g := range d.Charts {
			a := antes.Charts[i]
			if c.conID != (g.ID == a.ID) {
				t.Errorf("%s: el gráfico %d llega con el id %q", c.fichero, i, g.ID)
			}
			// Calc vuelve a anclar con sus propias alturas de fila: la y se mueve unos
			// píxeles (260 → 267) al pasar por .xlsx.
			if g.Y == nil || math.Abs(*g.Y-*a.Y) > 8 {
				t.Errorf("%s: el gráfico %d llega a y=%v y estaba en %v", c.fichero, i, g.Y, *a.Y)
			}
			g.ID, g.Y, a.Y = a.ID, nil, nil
			ja, _ := json.Marshal(a)
			jd, _ := json.Marshal(g)
			if !bytes.Equal(ja, jd) {
				t.Errorf("%s: el gráfico %d no llega igual:\n  antes:   %s\n  después: %s", c.fichero, i, ja, jd)
			}
		}
	}
}

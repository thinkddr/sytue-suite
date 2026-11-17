// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
)

// De punta a punta: Calcus de Fyne inserta o borra filas y columnas en una hoja protegida
// con ese permiso, reescribe las fórmulas bloqueadas de la PROPIA hoja (y las de la otra
// hoja protegida que la cita), y el servidor acepta el guardado de quien no es el
// propietario (apps/nucleo/office/model/proteccion_propia.go).
//
// VISTO EN ROJO sin proteccion_propia.go: los cuatro guardados daban ErrProtegido.

// libroPropiaT es «Datos», protegida con insertar y borrar filas y columnas, con una suma
// y dos fórmulas bloqueadas de la propia hoja, y «Resumen», protegida sin permisos, que
// cita la suma.
const libroPropiaT = `{"active":0,"sheets":[` +
	`{"name":"Datos","cells":{"A1":"1","A2":"2","A3":"3","A4":"4","A5":"5",` +
	`"A10":"<b>=SUM(A1:A5)</b>","B1":"=A3*2","B2":"=Datos!A4+$A$5"},` +
	`"protect":{"insertRows":true,"deleteRows":true,"insertCols":true,"deleteCols":true}},` +
	`{"name":"Resumen","cells":{"A1":"=Datos!A10"},"protect":{}}]}`

func TestCambiarLaEstructuraDeLaHojaProtegidaLoAceptaElServidor(t *testing.T) {
	casos := []struct {
		nombre     string
		col, fila  int
		cmd        editor.Comando
		lado       string
		suma, otra string // la suma de «Datos» (celda y fórmula) y lo que queda en «Resumen»
	}{
		{"insertar una fila encima de la 3", 0, 2, CmdInsFila, "above", "A11 <b>=SUM(A1:A6)</b>", "=Datos!A11"},
		{"borrar la fila 2", 0, 1, CmdBorrarFilas, "", "A9 <b>=SUM(A1:A4)</b>", "=Datos!A9"},
		{"borrar la fila 3, que cita B1", 0, 2, CmdBorrarFilas, "", "A9 <b>=SUM(A1:A4)</b>", "=Datos!A9"},
		{"insertar una columna a la izquierda", 0, 0, CmdInsCol, "left", "B10 <b>=SUM(B1:B5)</b>", "=Datos!B10"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			lz := libroDePrueba(t, libroPropiaT)
			lz.Rejilla.Seleccionar(c.col, c.fila, false)
			lz.Mandar(c.cmd, c.lado)
			guardado := lz.Serializar()
			datos, resumen := celdasGuardadasT(t, guardado, 0), celdasGuardadasT(t, guardado, 1)
			ref, suma, _ := strings.Cut(c.suma, " ")
			if datos[ref] != suma || resumen["A1"] != c.otra {
				t.Fatalf("Fyne no reescribe las fórmulas: %s=%q, Resumen!A1=%q", ref, datos[ref], resumen["A1"])
			}
			if err := aceptaElServidor(t, libroPropiaT, guardado); err != nil {
				t.Fatalf("el servidor rechaza el cambio de estructura de la propia hoja: %v", err)
			}
			// Y el servidor no lo acepta a ciegas: la misma suma sin reescribir se rechaza.
			sinReescribir := conCeldaT(t, guardado, ref, "<b>=SUM(A1:A5)</b>")
			if err := aceptaElServidor(t, libroPropiaT, sinReescribir); !errors.Is(err, model.ErrProtegido) {
				t.Errorf("la suma sin reescribir: quiero ErrProtegido y es %v", err)
			}
		})
	}
}

// conCeldaT es el modelo guardado con la celda `ref` de «Datos» cambiada.
func conCeldaT(t *testing.T, modelo, ref, valor string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(modelo), &m); err != nil {
		t.Fatal(err)
	}
	m["sheets"].([]any)[0].(map[string]any)["cells"].(map[string]any)[ref] = valor
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Y lo mismo que la web: con las entradas del vector `propiaHoja` de la web
// (packages/office/test/vectores/calcus.json, `desplazarHojas`), Calcus de Fyne deja
// cada hoja igual —celdas, estilos, protección con su rango editable movido y tablas
// dinámicas— y el servidor lo acepta.
//
// VISTO EN ROJO sin los rangos en `Proteccion` (PEND-CALC-FYNE-RANGOS): Fyne quitaba el
// rango editable del vector al guardar, la protección no era la de la web y el servidor
// lo rechazaba, con o sin desplazamiento.
func TestVectoresDeLaPropiaHojaComoLaWeb(t *testing.T) {
	b, err := os.ReadFile(rutaVectores)
	if err != nil {
		t.Fatalf("no se leen los vectores de la web: %v", err)
	}
	var v struct {
		PropiaHoja struct {
			Hojas []json.RawMessage
			Casos []struct {
				Hoja int
				D    struct {
					Eje             string
					Desde, Cantidad int
				}
				Sale []json.RawMessage
			}
		} `json:"propiaHoja"` //nolint:tagliatelle // el nombre lo fija la web
	}
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("los vectores no se entienden: %v", err)
	}
	if len(v.PropiaHoja.Casos) == 0 {
		t.Fatal("calcus.json no trae la sección «propiaHoja»")
	}
	libro := func(hojas []json.RawMessage, activa int) string {
		b, _ := json.Marshal(map[string]any{"active": activa, "sheets": hojas})
		return string(b)
	}
	for _, c := range v.PropiaHoja.Casos {
		inicial := libro(v.PropiaHoja.Hojas, c.Hoja)
		lz := libroDePrueba(t, inicial)
		d := c.D
		fin := d.Desde - d.Cantidad - 1 // el último que se borra
		switch {
		case d.Eje == "fila" && d.Cantidad > 0:
			lz.Libro.Estructura(string(CmdInsFila), "above", 0, d.Desde, 0, d.Desde)
		case d.Eje == "fila":
			lz.Libro.Estructura(string(CmdBorrarFilas), "", 0, d.Desde, 0, fin)
		case d.Cantidad > 0:
			lz.Libro.Estructura(string(CmdInsCol), "left", d.Desde, 0, d.Desde, 0)
		default:
			lz.Libro.Estructura(string(CmdBorrarCols), "", d.Desde, 0, fin, 0)
		}
		guardado := lz.Serializar()
		var m struct{ Sheets []map[string]json.RawMessage }
		if err := json.Unmarshal([]byte(guardado), &m); err != nil || len(m.Sheets) != len(c.Sale) {
			t.Fatalf("%+v: lo guardado no es el libro: %v", d, err)
		}
		for i, raw := range c.Sale {
			var web map[string]json.RawMessage
			_ = json.Unmarshal(raw, &web)
			for _, campo := range []string{"cells", "styles", "protect", "pivots"} {
				if aqui, alli := crudoJSON(m.Sheets[i][campo]), crudoJSON(web[campo]); !reflect.DeepEqual(aqui, alli) {
					t.Errorf("%+v, hoja %d, %s:\n  aquí:   %v\n  la web: %v", d, i, campo, aqui, alli)
				}
			}
		}
		if err := aceptaElServidor(t, inicial, guardado); err != nil {
			t.Errorf("%+v: el servidor rechaza lo que guarda Fyne: %v", d, err)
		}
	}
}

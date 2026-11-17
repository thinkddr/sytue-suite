// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
)

// Los rangos editables por persona o grupo de una hoja protegida (`protect.ranges`):
// Calcus de Fyne los lee, los conserva y los escribe como la web, deja escribir en sus
// celdas a quien se le concedieron y los mueve con sus filas y columnas. El servidor
// acepta el guardado de quien no es el propietario
// (apps/nucleo/office/model/proteccion_rangos.go).
//
// VISTO EN ROJO sin los rangos en `Proteccion`: la ida y vuelta los perdía, B1 no se
// podía escribir y el servidor rechazaba el guardado con ErrProtegido aunque solo
// tocara lo permitido.

// libroRangosT es «Cuentas», protegida sin permisos, con B1:B2 para luis y C1:C3 para
// ana o el grupo de contabilidad. Tal como lo guarda la web.
const libroRangosT = `{"active":0,"sheets":[{"name":"Cuentas",` +
	`"cells":{"A1":"Concepto","B1":"10","B2":"20","C1":"x"},` +
	`"protect":{"ranges":[{"ref":"B1:B2","users":["luis"]},` +
	`{"ref":"C1:C3","users":["ana"],"groups":["contabilidad"]}]}}]}`

func TestLosRangosEditablesVanYVuelvenComoEnLaWeb(t *testing.T) {
	lz := libroDePrueba(t, libroRangosT)
	if !igual(t, libroRangosT, lz.Serializar()) {
		t.Fatal("sin tocar nada, los rangos editables no salen como entraron")
	}
	// Y saneados como `rangosSaneados`: la referencia en mayúsculas y sin «$», solo las
	// personas que son texto, y fuera lo que no se lee —una columna entera, que la web no
	// sabe pintar— o no concede nada a nadie.
	lz = libroDePrueba(t, `{"active":0,"sheets":[{"name":"H","cells":{},"protect":{"ranges":[`+
		`{"ref":" $b$2:c3 ","users":["ana","",7],"title":"Ventas"},{"ref":"B:C","users":["ana"]},`+
		`{"ref":"A1"},{"ref":"A1","users":[]},{"ref":5,"users":["ana"]},null,{"ref":"D4","groups":["g"]}]}}]}`)
	quiero := []model.RangoEditable{{Ref: "B2:C3", Users: []string{"ana"}}, {Ref: "D4", Groups: []string{"g"}}}
	if got := lz.Libro.Proteccion().Ranges; !reflect.DeepEqual(got, quiero) {
		t.Errorf("los rangos se sanean como %+v", got)
	}
	// Sin ninguno que valga, `ranges` no se escribe.
	lz = libroDePrueba(t, `{"active":0,"sheets":[{"name":"H","cells":{},"protect":{"sort":true,"ranges":[{"ref":"A1"}]}}]}`)
	if !igual(t, `{"active":0,"sheets":[{"name":"H","cells":{},"protect":{"sort":true}}]}`, lz.Serializar()) {
		t.Error("sin rangos que valgan, `ranges` sigue saliendo")
	}
}

// abrirConRangosT abre libroRangosT desde el servicio como `quien` (sin nadie si es "").
func abrirConRangosT(t *testing.T, quien string) (*servicioEstructura, *App) {
	t.Helper()
	s := &servicioEstructura{hoja: libroRangosT, lamport: 4, dot: `{"a":"web","c":2}`}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	a := NuevaApp(traductorPlay(t), "es", api.NewDocs(srv.URL, sesionFija{}, srv.Client()))
	a.lanzar = func(f func()) { f() }
	a.portada.Lanzar = a.lanzar
	if quien != "" {
		a.Reclamos = func(context.Context) (auth.Reclamos, error) { return auth.Reclamos{Sub: quien}, nil }
	}
	a.AbrirID("h1", "")
	if a.abierto.ctx.SoloLectura || a.abierto.auto == nil {
		t.Fatal("el libro se abre en solo lectura")
	}
	return s, a
}

// De punta a punta: luis, que no es el propietario, abre la hoja desde el servicio,
// escribe en SU rango y no fuera, y el servidor acepta lo que guarda Fyne; lo mismo con
// una celda de fuera, no.
func TestQuienTieneUnRangoLoEditaYElServidorAceptaSuGuardado(t *testing.T) {
	s, a := abrirConRangosT(t, "luis")
	ab := a.abierto
	r := ab.lz.Rejilla
	escribir := func(c, f int, texto string) bool {
		r.Seleccionar(c, f, false)
		return r.Escribir(texto)
	}
	if !escribir(1, 0, "15") || ab.lz.Libro.Crudo(1, 0) != "15" {
		t.Fatal("B1 es del rango de luis y no la deja escribir")
	}
	if escribir(2, 0, "y") || escribir(0, 0, "otro") || escribir(1, 2, "fuera") {
		t.Error("luis escribe fuera de su rango: en el de ana, en lo bloqueado o debajo del suyo")
	}
	// Las ediciones de varias celdas (borrar, rellenar) siguen la misma regla.
	if !ab.lz.Libro.Editable(1, 1) || ab.lz.Libro.rangoBloqueado(1, 0, 1, 1) || !ab.lz.Libro.rangoBloqueado(1, 0, 2, 0) {
		t.Error("el rango de luis no cuenta como desbloqueado en las ediciones de varias celdas")
	}
	ab.auto.Parar()
	if err := a.guardar(ab); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	guardados := append([]string(nil), s.guardados...)
	s.mu.Unlock()
	if len(guardados) != 1 {
		t.Fatalf("se guarda %d veces", len(guardados))
	}
	var m struct {
		Sheets []struct {
			Protect json.RawMessage `json:"protect"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal([]byte(guardados[0]), &m); err != nil {
		t.Fatal(err)
	}
	var web struct {
		Sheets []struct{ Protect json.RawMessage }
	}
	_ = json.Unmarshal([]byte(libroRangosT), &web)
	if !reflect.DeepEqual(crudoJSON(m.Sheets[0].Protect), crudoJSON(web.Sheets[0].Protect)) {
		t.Errorf("la protección se guarda como %s", m.Sheets[0].Protect)
	}
	if err := aceptaElServidor(t, libroRangosT, guardados[0]); err != nil {
		t.Fatalf("el servidor rechaza el guardado de luis dentro de su rango: %v", err)
	}
	// Fuera de su rango, el servidor no lo acepta: ni en el de ana ni en lo bloqueado.
	for _, ref := range []string{"C1", "A1", "B3"} {
		if err := aceptaElServidor(t, libroRangosT, conCeldaT(t, guardados[0], ref, "otro")); !errors.Is(err, model.ErrProtegido) {
			t.Errorf("luis escribe %s: quiero ErrProtegido y es %v", ref, err)
		}
	}
}

// Sin saber quién edita, ningún rango concede nada (deny-by-default), como en la web.
func TestSinSaberQuienEditaNingunRangoConcedeNada(t *testing.T) {
	_, a := abrirConRangosT(t, "")
	if a.abierto.lz.Libro.Editable(1, 0) {
		t.Error("sin sesión, B1 se puede escribir")
	}
	_, a = abrirConRangosT(t, "eva") // de un grupo que el cliente no ve: como en la web
	if a.abierto.lz.Libro.Editable(2, 0) || a.abierto.lz.Libro.Editable(1, 0) {
		t.Error("eva no tiene ningún rango por su sub y escribe")
	}
}

// Los rangos viajan con sus celdas al insertar o borrar filas y columnas —crecen, encogen
// o desaparecen—, deshacer los devuelve, y el servidor acepta el desplazamiento de quien
// no es el propietario.
func TestLosRangosSiguenASusCeldas(t *testing.T) {
	const inicial = `{"active":0,"sheets":[{"name":"Datos",` +
		`"cells":{"A1":"1","A2":"2","A3":"3","A4":"4","A5":"5","C1":"x","D8":"fin"},` +
		`"protect":{"insertRows":true,"deleteRows":true,"insertCols":true,"deleteCols":true,` +
		`"ranges":[{"ref":"A4:A5","users":["luis"]},{"ref":"C1","users":["ana"]}]}}]}`
	casos := []struct {
		nombre string
		cmd    editor.Comando
		c1, f1 int
		c2, f2 int
		quiero string // los rangos que quedan en `protect`
	}{
		{
			"insertar una fila encima de la 3", CmdInsFila, 0, 2, 0, 2,
			`[{"ref":"A5:A6","users":["luis"]},{"ref":"C1","users":["ana"]}]`,
		},
		{
			"borrar la fila 5, que encoge el rango", CmdBorrarFilas, 0, 4, 0, 4,
			`[{"ref":"A4","users":["luis"]},{"ref":"C1","users":["ana"]}]`,
		},
		{
			"borrar la fila 1, que se lleva el de ana", CmdBorrarFilas, 0, 0, 0, 0,
			`[{"ref":"A3:A4","users":["luis"]}]`,
		},
		{"borrar de la 1 a la 5, que se los lleva todos", CmdBorrarFilas, 0, 0, 0, 4, ""},
		{
			"insertar una columna a la izquierda", CmdInsCol, 0, 0, 0, 0,
			`[{"ref":"B4:B5","users":["luis"]},{"ref":"D1","users":["ana"]}]`,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			lz := libroDePrueba(t, inicial)
			lado := map[editor.Comando]string{CmdInsFila: "above", CmdInsCol: "left"}[c.cmd]
			lz.Libro.Estructura(string(c.cmd), lado, c.c1, c.f1, c.c2, c.f2)
			guardado := lz.Serializar()
			var m struct {
				Sheets []struct {
					Protect json.RawMessage `json:"protect"`
				} `json:"sheets"`
			}
			if err := json.Unmarshal([]byte(guardado), &m); err != nil {
				t.Fatal(err)
			}
			// Los permisos siguen igual; sin rangos que queden, `ranges` no sale.
			quiero := `{"insertRows":true,"deleteRows":true,"insertCols":true,"deleteCols":true}`
			if c.quiero != "" {
				quiero = quiero[:len(quiero)-1] + `,"ranges":` + c.quiero + `}`
			}
			if !reflect.DeepEqual(crudoJSON(m.Sheets[0].Protect), crudoJSON(json.RawMessage(quiero))) {
				t.Errorf("la protección queda %s;\n quiero %s", m.Sheets[0].Protect, quiero)
			}
			if err := aceptaElServidor(t, inicial, guardado); err != nil {
				t.Errorf("el servidor rechaza el desplazamiento de los rangos: %v", err)
			}
			// La foto de deshacer comparte la protección: moverla no puede tocar la de antes.
			lz.Deshacer(false)
			if !igual(t, inicial, lz.Serializar()) {
				t.Error("deshacer no devuelve los rangos donde estaban")
			}
		})
	}
}

// Con la hoja protegida, «Proteger hoja» enseña sus rangos editables, como la web.
func TestElPanelEnsenaLosRangosEditables(t *testing.T) {
	lz := libroDePrueba(t, libroRangosT)
	ctx := &editor.Contexto{Tipo: editor.Hoja, Lienzo: lz}
	cuerpo := lz.cuerpoDatos(ctx)
	w := test.NewTempWindow(t, cuerpo)
	w.Resize(fyne.NewSize(1400, 500))
	botonDeCinta(cuerpo, "data.protectSheet").Tapped(nil)
	var textos []string
	for _, o := range todos(w.Canvas().Overlays().Top()) {
		if p, ok := o.(*ui.Parrafo); ok {
			textos = append(textos, p.Texto)
		}
	}
	quiero := []string{"data.protect.sheetProtected", "data.protect.ranges: B1:B2, C1:C3"}
	if len(textos) < 2 || !reflect.DeepEqual(textos[len(textos)-2:], quiero) {
		t.Errorf("el panel de la hoja protegida dice %q", textos)
	}
	guarda(t, w.Canvas().Capture()) // con `SYTUE_CAPTURA`, el panel abierto
}

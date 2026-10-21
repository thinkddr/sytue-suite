// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestTraeTusContactosVaEnElPie es `footer={<ImportData domains={['contacts']} />}`: con
// Google conectado en la organización, el pie del menú lleva «Trae tus contactos» (la
// pieza común `ui.ImportarDatos`); sin ningún origen, el pie se queda vacío.
func TestTraeTusContactosVaEnElPie(t *testing.T) {
	pie := func(instancias string) fyne.CanvasObject {
		t.Helper()
		var pedidas atomic.Int32
		gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer pedidas.Add(1)
			switch r.URL.Path {
			case "/api/v1/instances":
				_, _ = io.WriteString(w, instancias)
			case "/api/v1/migrations/self":
				_, _ = io.WriteString(w, `{"migrations":[]}`)
			}
		}))
		t.Cleanup(gw.Close)
		a := appDePrueba(t, &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}})
		a.ConImportar(api.NewIntegraciones(gw.URL, nil, nil))
		a.Cargar()
		// Hasta que la pasarela ha contestado a las dos preguntas, y lo que eso manda
		// pintar ha corrido (en la rutina del test): así «vacío» no es «aún no ha llegado».
		for i := 0; i < 200 && pedidas.Load() < 2; i++ {
			time.Sleep(10 * time.Millisecond)
		}
		for range 5 {
			fyne.DoAndWait(func() {})
			time.Sleep(10 * time.Millisecond)
		}
		return a.Pantalla.menu().(*ui.Lateral).Pie
	}
	if o := pie(`{"instances":[{"id":"g","name":"G","template_key":"google","status":"active"}]}`); o == nil {
		t.Fatal("con Google conectado no hay pie")
	}
	// Sin origen NO hay pie: el 2026-09-28 se veía un filete con nada debajo. VISTO EN
	// ROJO con el rectángulo transparente de antes.
	if o := pie(`{"instances":[]}`); o != nil {
		t.Fatalf("sin ningún origen hay pie, y con él un filete sin nada debajo: %T", o)
	}
}

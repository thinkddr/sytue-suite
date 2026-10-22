// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

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

// TestTraeTusCalendariosVaEnElPie es `footer={<ImportData domains={['calendar']} />}`:
// con Google conectado en la organización, el pie del menú lleva «Trae tus datos» (la
// pieza común `ui.ImportarDatos`) y, sin ningún origen, NO hay pie: el 2026-09-28 se
// veía un filete con nada debajo. VISTO EN ROJO con el relleno transparente de antes.
func TestTraeTusCalendariosVaEnElPie(t *testing.T) {
	pie := func(instancias string) fyne.CanvasObject {
		t.Helper()
		var pedidas atomic.Int32
		gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer pedidas.Add(1)
			switch r.URL.Path {
			case "/api/v1/instances":
				_, _ = io.WriteString(w, instancias)
			case "/api/v1/migrations/self":
				_, _ = io.WriteString(w, `{"migrations":[{"id":"j","domain":"mail","source":"google","status":"running"}]}`)
			}
		}))
		t.Cleanup(gw.Close)
		a := appDePrueba(t, &servicioFalso{})
		a.ConImportar(api.NewIntegraciones(gw.URL, nil, nil))
		a.Arrancar()
		for i := 0; i < 200 && pedidas.Load() < 2; i++ {
			time.Sleep(10 * time.Millisecond)
		}
		for range 5 {
			fyne.DoAndWait(func() {})
			time.Sleep(10 * time.Millisecond)
		}
		return a.P.menu().(*ui.Lateral).Pie
	}
	if o := pie(`{"instances":[{"id":"g","name":"G","template_key":"google","status":"active"}]}`); o == nil {
		t.Fatal("con Google conectado no hay pie")
	}
	// Un trabajo de OTRO dominio (mail) no cuenta: sin origen, nada que ofrecer.
	if o := pie(`{"instances":[]}`); o != nil {
		t.Fatalf("sin ningún origen hay pie, y con él un filete sin nada debajo: %T", o)
	}
}

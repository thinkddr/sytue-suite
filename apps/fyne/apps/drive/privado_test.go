// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type datosDePrueba struct{ api.DriveData }

func (datosDePrueba) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "org", Subject: "owner", Product: "drive"}
}

func TestLaCuotaPrivadaUsaLaCuentaSeparadaSinAbrirDatosAntiguos(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/quota" {
			t.Errorf("unexpected old data endpoint: %s", r.URL.Path)
		}
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(api.DriveQuota{UsedBytes: 13, LimitBytes: 40})
	}))
	defer server.Close()
	account := api.NewDrive(server.URL, nil, nil)
	data, err := account.WithPrivateData(datosDePrueba{})
	if err != nil {
		t.Fatal(err)
	}
	p := NuevaPantalla(traductor(t), data, nil, nil)
	p.Cuenta = account
	defer p.Cerrar()
	p.cargarCuota()
	if calls.Load() != 1 {
		t.Fatal("private data facade intercepted account quota", calls.Load())
	}
}

func TestCerrarLaPantallaPrivadaBorraVistasYNoPermiteRefrescos(t *testing.T) {
	w := test.NewTempWindow(t, nil)
	w.Resize(fyne.NewSize(320, 640))
	p := NuevaPantalla(traductor(t), nil, nil, w)
	w.SetContent(p.Objeto())
	d := ui.NuevoDialogo("Private account", ui.NuevaEntrada("Private value"))
	d.NoDescartable = true
	d.Mostrar(w.Canvas())
	if w.Canvas().Overlays().Top() == nil {
		t.Fatal("private fixture did not show")
	}
	p.explor.Elementos = []Elemento{{ID: "private", Nombre: "private data"}}
	p.papelera.Elementos = []Elemento{{Nombre: "private trash"}}
	p.sec.recientes.Elementos = []Elemento{{Nombre: "private recent"}}
	p.sec.documentos.Documentos = []Documento{{}}
	refresh := 0
	p.RefrescarDatos = func(func()) { refresh++ }
	p.Cerrar()
	p.Cerrar()
	p.Recargar()
	p.Repintar()
	if refresh != 0 || len(p.explor.Elementos) != 0 || len(p.papelera.Elementos) != 0 || len(p.sec.recientes.Elementos) != 0 || len(p.sec.documentos.Documentos) != 0 || len(p.raiz.Objects) != 0 {
		t.Fatal("closed private presenter retained or rebuilt its view")
	}
	if w.Canvas().Overlays().Top() != nil || w.Canvas().Focused() != nil {
		t.Fatal("closed private presenter retained an account dialog or focus")
	}
	d.Cerrar()
}

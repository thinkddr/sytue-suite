// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"

	"git.sytue.eu/sytue/one/apps/fyne/agenda"
)

// TestSinRedSeQuedaEnSoloLectura es el efecto de `offline` en ContactsApp.tsx: al irse
// la red se apagan las escrituras, se cierra el formulario abierto y se suelta la
// selección; al volver, se puede escribir otra vez.
func TestSinRedSeQuedaEnSoloLectura(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana"), persona("bruno", "Bruno")}}
	a := appDePrueba(t, s)
	red := &offline.Monitor{}
	a.Conexion(red)
	a.Cargar()
	a.Pantalla.AlMarcar("ana")
	if a.abrirFormulario(nil) == nil || a.lienzo().Overlays().Top() == nil {
		t.Fatal("no se ha abierto el formulario")
	}
	red.Report(offline.Offline)
	if !a.Pantalla.SinConexion || a.lienzo().Overlays().Top() != nil || len(a.marcados) != 0 {
		t.Fatalf("sin red: solo lectura %v, encima %v, marcados %v",
			a.Pantalla.SinConexion, a.lienzo().Overlays().Top(), a.marcados)
	}
	if a.Pantalla.Estado != agenda.Exito || len(a.Pantalla.Contactos) != 2 {
		t.Errorf("lo ya cargado tiene que seguir viéndose: %v, %d", a.Pantalla.Estado, len(a.Pantalla.Contactos))
	}
	red.Report(offline.Online)
	if a.Pantalla.SinConexion {
		t.Error("con la red de vuelta sigue en solo lectura")
	}
}

// TestSinRedYSinCopiaDiceSinConexionYVuelveSolo: lo que no llega sin red se queda en
// «sin conexión», que no tiene «Reintentar», así que al volver la red se pide solo.
func TestSinRedYSinCopiaDiceSinConexionYVuelveSolo(t *testing.T) {
	var pedidas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pedidas.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	a := NuevaApp(traductor(t), api.NewContacts(srv.URL, nil, nil))
	a.enFondo = func(f func()) { f() }
	red := &offline.Monitor{}
	a.Conexion(red)
	red.Report(offline.Offline)
	a.Cargar()
	if a.Pantalla.Estado != agenda.SinConexion {
		t.Fatalf("sin red y sin copia: estado %v", a.Pantalla.Estado)
	}
	antes := pedidas.Load()
	red.Report(offline.Online)
	if pedidas.Load() == antes || a.Pantalla.Estado != agenda.Error {
		t.Errorf("al volver la red no se ha vuelto a pedir (%d → %d, estado %v)", antes, pedidas.Load(), a.Pantalla.Estado)
	}
}

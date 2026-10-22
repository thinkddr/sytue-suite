// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Un cuaderno público de otra persona se lee y no se escribe: la pantalla no deja
// escribir, su franja ofrece «Pedir acceso de edición», y cuando el Hub dice que se ha
// concedido se vuelven a leer los cuadernos. Uno propio no lleva franja.
func TestUnCuadernoAjenoDeLecturaPideAccesoYSeDesbloqueaEnVivo(t *testing.T) {
	var listados atomic.Int32
	srv := servidorDeCuadernos(t, &listados)
	p := NuevaPantalla(trInterno(t), "es", api.NewNotes(srv, nil, nil), nil, nil)
	ajeno := DeCuaderno(api.Notebook{ID: "c1", Title: "Plan", ReadOnly: true})
	p.activo = ajeno
	p.engancharPeticion(ajeno)
	if p.puedeEscribir() {
		t.Error("se deja escribir en un cuaderno ajeno de solo lectura")
	}
	f, ok := p.franjaPeticion().(*ui.FranjaSoloLectura)
	if !ok || f.Accion == nil {
		t.Fatal("el cuaderno ajeno no tiene franja con «Pedir acceso de edición»")
	}
	ui.RepartirAviso(api.HubNotification{ID: "g", Type: "notes.access.granted", Params: map[string]string{"resource": "c1"}})
	fyne.DoAndWait(func() {})
	esperar(t, func() bool { return listados.Load() > 0 })
	propio := DeCuaderno(api.Notebook{ID: "c2", Title: "Mío"})
	p.activo = propio
	p.engancharPeticion(propio)
	if p.franjaPeticion() != nil {
		t.Error("un cuaderno propio lleva franja de solo lectura")
	}
}

// servidorDeCuadernos cuenta las veces que se pide la lista de cuadernos.
func servidorDeCuadernos(t *testing.T, listados *atomic.Int32) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/notebooks" {
			listados.Add(1)
		}
		_, _ = w.Write([]byte(`{"notebooks":[],"notes":[],"tags":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

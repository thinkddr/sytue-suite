// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
)

// Crear una hoja de cálculo manda la señal de cambios solo al widget de Calcus: al
// crear se sabe el tipo, y los de Docus y Diapos no tienen por qué volver a leer.
// VISTO EN ROJO sin el CambioEn de createDocument (llegaban docs, sheets y slides).
func TestCrearUnaHojaSoloAvisaAlWidgetDeCalcus(t *testing.T) {
	var mu sync.Mutex
	var llegan []string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e struct {
			Data struct {
				Producto string `json:"producto"`
			} `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&e)
		mu.Lock()
		llegan = append(llegan, e.Data.Producto)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()

	h := newHandler(&fakeStore{})
	senal := hubnotify.New(hub.URL, "s").TrasEscribir(nil, "docs", "sheets", "slides")
	rec := httptest.NewRecorder()
	senal(http.HandlerFunc(h.createDocument)).ServeHTTP(rec,
		req(t, "POST", "/v1/documents", ownerClaims(), nil, `{"kind":"spreadsheet","title":"Cuentas"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("crear: %d %s", rec.Code, rec.Body.String())
	}
	time.Sleep(300 * time.Millisecond) // el envío es asíncrono: que lleguen también las que sobran
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(llegan, []string{"sheets"}) {
		t.Fatalf("la señal va a %v, quiero [sheets]", llegan)
	}
}

// Lo que la papelera de Drive le hace al fichero de un documento llega por la puerta
// interna, sin petición de Docs con TrasEscribir: desdeDrive manda la señal del tipo de
// cada documento a su dueño, y nada de lo que no aplica. VISTO EN ROJO sin el
// AvisarCambio de desdeDrive.
func TestDesdeDriveAvisaAlWidgetDeSuTipo(t *testing.T) {
	var mu sync.Mutex
	var llegan []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e struct {
			Subject string `json:"subject"`
			Data    struct {
				Producto string `json:"producto"`
			} `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&e)
		mu.Lock()
		llegan = append(llegan, e.Data.Producto+" "+e.Subject)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	recibido := func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := slices.Clone(llegan)
		slices.Sort(out)
		return out
	}

	h := &Handler{papelera: &papeleraFalsa{}, hubNotifier: hubnotify.New(srv.URL, "s")}
	cuerpo := `{"tenant":"acme","files":[
		{"id":"` + ficheroID + `","state":"trashed","owner_sub":"ana","name":"Informe.odt"},
		{"id":"` + ficheroID + `","state":"gone","owner_sub":"bea","name":"Charla.odp"},
		{"id":"` + ficheroID + `","state":"trashed","owner_sub":"carla","name":"Informe.docx"}]}`
	rec := httptest.NewRecorder()
	h.desdeDrive(rec, req(t, "POST", "/internal/drive/files", nil, nil, cuerpo))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código %d", rec.Code)
	}
	quiero := []string{"docs ana", "slides bea"}
	for limite := time.Now().Add(5 * time.Second); !slices.Equal(recibido(), quiero); {
		if time.Now().After(limite) {
			t.Fatalf("llega %v, quiero %v", recibido(), quiero)
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond) // que lleguen también las que sobran
	if got := recibido(); !slices.Equal(got, quiero) {
		t.Fatalf("llega %v, quiero %v", got, quiero)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package syerphttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// El widget del ERP son «mis actividades pendientes»: la señal de cambios llega a su
// responsable cuando otra persona se la pone o la cierra, no solo a quien escribe.
// Cada escritura usa su propio cliente del Hub, porque la ventana de agrupado es por
// cliente. Para verlo en rojo: quitar los AvisarTambien de CrearActividad y
// CerrarActividad. Escrito el 2026-11-07 sin un Postgres 18 a mano: compila (go vet
// -tags integration), sin correr aún.
func TestLaSenalDelERPLlegaAQuienTieneLaActividad(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	hs := manejadoresCompletos(t, store.New(harness.Pool))
	var mu sync.Mutex
	var llegan []string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e struct {
			Subject string `json:"subject"`
			Event   string `json:"event"`
		}
		_ = json.NewDecoder(r.Body).Decode(&e)
		if e.Event == hubnotify.EventoDatos {
			mu.Lock()
			llegan = append(llegan, e.Subject)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()
	hs.cfg.PublicURL = "https://erp.sytue.eu"
	directorio := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"user_id":"luis","name":"Luis","email":"luis@acme.test"}`))
	}))
	defer directorio.Close()
	hs.cfg.IDBaseURL = directorio.URL

	esperar := func(quiero ...string) {
		t.Helper()
		slices.Sort(quiero)
		limite := time.Now().Add(5 * time.Second)
		for {
			mu.Lock()
			got := slices.Clone(llegan)
			mu.Unlock()
			slices.Sort(got)
			if slices.Equal(got, quiero) {
				mu.Lock()
				llegan = nil
				mu.Unlock()
				return
			}
			if time.Now().After(limite) {
				t.Fatalf("la señal llega a %v, quiero %v", got, quiero)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	conSenal := func(h http.HandlerFunc) http.Handler {
		hs.hub = hubnotify.New(hub.URL, "s")
		return hs.hub.TrasEscribir(nil, "erp")(h)
	}

	w := comoPersona(t, "jefa", []string{"owner"}, http.MethodPost, "/api/v1/actividades",
		`{"tipo":"tarea","resumen":"Pedir el IBAN","vence":"2026-10-01","responsable_sub":"luis"}`, nil,
		conSenal(hs.CrearActividad))
	if w.Code != http.StatusCreated {
		t.Fatalf("crear: %d %s", w.Code, w.Body.String())
	}
	esperar("jefa", "luis")
	var creada struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &creada); err != nil || creada.ID == "" {
		t.Fatalf("actividad creada: %v %s", err, w.Body.String())
	}

	w = comoPersona(t, "jefa", []string{"owner"}, http.MethodPost, "/api/v1/actividades/"+creada.ID+"/hecha", "",
		map[string]string{"id": creada.ID}, conSenal(hs.CerrarActividad("hecha")))
	if w.Code != http.StatusOK {
		t.Fatalf("cerrar: %d %s", w.Code, w.Body.String())
	}
	esperar("jefa", "luis")
}

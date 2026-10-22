// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package syerphttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// Una tarea que te pone otra persona te llega a la campana, dice quién y abre las
// tareas; la que te pones tú, no.
func TestUnaTareaAsignadaLlegaALaCampanaDeQuienLaTiene(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	hs := manejadoresCompletos(t, store.New(harness.Pool))
	avisos := make(chan hubnotify.Notification, 4)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n hubnotify.Notification
		_ = json.NewDecoder(r.Body).Decode(&n)
		avisos <- n
		w.WriteHeader(http.StatusCreated)
	}))
	defer hub.Close()
	hs.cfg.PublicURL = "https://erp.sytue.eu"
	hs.hub = hubnotify.New(hub.URL, "s")
	// Sytue ID resuelve a quien es de la organización y a nadie más (directory:resolve).
	directorio := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			UserID string `json:"user_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		if r.URL.Path != "/api/v1/directory:resolve" || q.UserID != "luis" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"user_id":"luis","name":"Luis","email":"luis@acme.test"}`))
	}))
	defer directorio.Close()
	hs.cfg.IDBaseURL = directorio.URL

	crear := func(quien, cuerpo string) string {
		t.Helper()
		w := comoPersona(t, quien, []string{"owner"}, http.MethodPost, "/api/v1/actividades", cuerpo, nil,
			http.HandlerFunc(hs.CrearActividad))
		if w.Code != http.StatusCreated {
			t.Fatalf("crear: %d %s", w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	crear("jefa", `{"tipo":"tarea","resumen":"Pedir el IBAN","vence":"2026-10-01"}`)
	if b := crear("jefa", `{"tipo":"llamada","resumen":"Llamar a Anthropic","vence":"2026-10-02","responsable_sub":"luis","responsable_nombre":"Otro nombre"}`); !strings.Contains(b, `"responsable_nombre":"Luis"`) {
		t.Fatalf("el nombre del responsable es el del directorio, no el del cliente: %s", b)
	}

	// Alguien que no está en el directorio no puede llevar una tarea.
	w := comoPersona(t, "jefa", []string{"owner"}, http.MethodPost, "/api/v1/actividades",
		`{"tipo":"tarea","resumen":"x","vence":"2026-10-01","responsable_sub":"intruso"}`, nil, http.HandlerFunc(hs.CrearActividad))
	if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("responsable fuera del directorio: %d %s", w.Code, w.Body.String())
	}

	select {
	case n := <-avisos:
		if n.Subject != "luis" || n.Type != "erp.task.assigned" || n.Tenant != "acme" || n.Source != "erp" {
			t.Fatalf("aviso = %+v", n)
		}
		if n.URL != "https://erp.sytue.eu/erp/acme/?vista=tareas" || n.Params["actor"] != "JEFA" || n.Params["title"] != "Llamar a Anthropic" {
			t.Fatalf("enlace o variables: %+v", n)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("la tarea asignada no llega a la campana")
	}
	select {
	case n := <-avisos:
		t.Fatalf("la tarea que una se pone no avisa: %+v", n)
	case <-time.After(300 * time.Millisecond):
	}
}

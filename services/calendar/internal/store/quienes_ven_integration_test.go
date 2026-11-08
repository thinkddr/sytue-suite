// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// Lo que escribe en un calendario compartido quien lo recibe lo ve su dueño, y lo que
// escribe el dueño, quienes lo tienen: a todos les llega la señal de cambios para su
// widget, y a nadie más. Fuera de una escritura con señal no se consulta nada. VISTO
// EN ROJO sin la rama de los shares en QuienesVen (solo llegaba al dueño).
func TestLaSenalDeUnCalendarioLlegaAQuienesLoVen(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "dueña", Name: "equipo", DisplayName: "Equipo"})
	if err != nil {
		t.Fatal(err)
	}
	otro, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "otra", Name: "suyo", DisplayName: "Suyo"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"bob@sytue.eu", "carol@sytue.eu"} {
		if _, err := st.CreateCalendarShare(ctx, cal.ID, "dueña", c, domain.PermWriter); err != nil {
			t.Fatal(err)
		}
	}
	quienes, err := st.QuienesVen(ctx, cal.ID)
	slices.Sort(quienes)
	if err != nil || !slices.Equal(quienes, []string{"dueña", "email:bob@sytue.eu", "email:carol@sytue.eu"}) {
		t.Fatalf("QuienesVen = %v %v", quienes, err)
	}

	var mu sync.Mutex
	var llegan []string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e struct {
			Subject string `json:"subject"`
		}
		_ = json.NewDecoder(r.Body).Decode(&e)
		mu.Lock()
		llegan = append(llegan, e.Subject)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()
	escribir := hubnotify.New(hub.URL, "s").TrasEscribir(nil, "calendar")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st.AvisarQuienesVen(r.Context(), cal.ID)
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/calendars/x/events", http.NoBody)
	req = req.WithContext(authn.Into(ctx, authn.Claims{Subject: "bob-sub", Tenant: "acme"}))
	escribir.ServeHTTP(httptest.NewRecorder(), req)

	quiero := []string{"bob-sub", "dueña", "email:bob@sytue.eu", "email:carol@sytue.eu"}
	limite := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		got := slices.Clone(llegan)
		mu.Unlock()
		slices.Sort(got)
		if slices.Equal(got, quiero) {
			break
		}
		if time.Now().After(limite) {
			t.Fatalf("la señal llega a %v, quiero %v", got, quiero)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Sin escritura con señal (CalDAV de un worker, un test) no consulta ni avisa.
	st.AvisarQuienesVen(ctx, otro.ID)
}

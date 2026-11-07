// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service

import (
	"context"
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
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
	"git.sytue.eu/sytue/one/services/calendar/internal/subscriptions"
)

// feedFijo sirve siempre el mismo resultado, sin red.
type feedFijo struct{ res subscriptions.FetchResult }

func (f feedFijo) Fetch(context.Context, string, subscriptions.Conditional) (subscriptions.FetchResult, error) {
	return f.res, nil
}

const feedDeHoy = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\n" +
	"BEGIN:VEVENT\r\nUID:festivo-1\r\nDTSTAMP:20261107T000000Z\r\n" +
	"DTSTART:20261107T090000Z\r\nDTEND:20261107T100000Z\r\nSUMMARY:Festivo\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

// El worker refresca las suscripciones fuera de cualquier petición: cuando el feed
// cambia, la señal de cambios llega al dueño y a quienes lo tienen compartido; un 304
// no la manda. Para verlo en rojo: quitar avisarRefresco de refreshJob (no llega nada)
// o la comprobación de `Status == "ok"` (el 304 avisa). Escrito el 2026-11-07 sin un
// Postgres 18 a mano: compila (go vet -tags integration), sin correr aún.
func TestUnRefrescoConCambiosAvisaYUn304No(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour, SubscriptionRefreshInterval: time.Hour}, st, nil)

	var mu sync.Mutex
	var llegan []string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e struct {
			Subject string `json:"subject"`
			Event   string `json:"event"`
		}
		_ = json.NewDecoder(r.Body).Decode(&e)
		mu.Lock()
		llegan = append(llegan, e.Event+" "+e.Subject)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()
	vistos := func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := slices.Clone(llegan)
		slices.Sort(out)
		return out
	}

	dueña := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}),
		authn.Claims{Subject: "ana", Email: "ana@acme.test"})
	cal, err := st.CreateSubscription(dueña, "festivos", store.NewSubscription{
		OwnerSub: "ana", URL: "https://feeds.example.eu/festivos.ics", DisplayName: "Festivos",
		RefreshInterval: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateCalendarShare(dueña, cal.ID, "ana", "luis@acme.test", domain.PermReader); err != nil {
		t.Fatal(err)
	}
	job, err := st.SubscriptionForCalendar(dueña, cal.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Lo que corre el worker: solo el tenant, sin claims ni TrasEscribir.
	worker := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})

	// Un 304 (con su propio cliente del Hub: la ventana de agrupado es por cliente).
	svc.SetHubNotifier(hubnotify.New(hub.URL, "s"))
	svc.subClient = feedFijo{subscriptions.FetchResult{NotModified: true}}
	svc.refreshJob(worker, job)
	time.Sleep(300 * time.Millisecond)
	if got := vistos(); len(got) != 0 {
		t.Fatalf("un feed sin cambios (304) manda la señal: %v", got)
	}

	svc.SetHubNotifier(hubnotify.New(hub.URL, "s"))
	svc.subClient = feedFijo{subscriptions.FetchResult{Body: feedDeHoy, ETag: `"v2"`}}
	svc.refreshJob(worker, job)
	quiero := []string{"datos ana", "datos email:luis@acme.test"}
	limite := time.Now().Add(5 * time.Second)
	for !slices.Equal(vistos(), quiero) {
		if time.Now().After(limite) {
			t.Fatalf("tras un refresco con cambios llega %v, quiero %v", vistos(), quiero)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

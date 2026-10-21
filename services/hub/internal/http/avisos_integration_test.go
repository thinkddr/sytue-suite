// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package hubhttp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	"git.sytue.eu/sytue/one/services/hub/internal/config"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
	"git.sytue.eu/sytue/one/services/hub/internal/notify"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

// La campana, de punta a punta y contra un Postgres de verdad: un servicio deja un
// aviso por la puerta interna, llega por el flujo SSE a quien lo mira, cuenta como
// sin leer y deja de contar al leerlo.
//
// Es el guardián de las dos causas de «Todo al día» con cosas pasando (2026-09-24):
//   - el CHECK de `source` de 0001 rechazaba seis de los once orígenes, así que aquí
//     se manda uno de CADA origen válido;
//   - el flujo moría con el WriteTimeout del servidor, así que el servidor lleva
//     plazos (de décimas, para no esperar los 30 s de producción) y los avisos se
//     crean cuando ya han vencido.
func TestUnAvisoCreadoLlegaAlFlujoYBajaAlLeerlo(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := store.New(h.Pool)
	hub := notify.NewHub(32)
	go func() { _ = notify.NewRelay(h.Pool, hub).Run(ctx) }()
	esperarListen(t, h)

	hs := &Handlers{cfg: config.Config{InternalSecret: "s"}, notify: notify.NewService(st, hub), hub: hub, store: st}
	r := chi.NewRouter()
	r.With(hs.internalAuth).Post("/internal/notifications", hs.CreateInternalNotification)
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c := authn.Claims{Subject: "ana", Tenant: "acme", Mailbox: "Ana@Acme.es"}
				next.ServeHTTP(w, r.WithContext(tenancy.Into(authn.Into(r.Context(), c), tenancy.Tenant{Slug: "acme"})))
			})
		})
		api.Get("/notifications", hs.ListNotifications)
		api.Get("/notifications/unread_count", hs.UnreadCount)
		api.Post("/notifications/{id}/read", hs.MarkRead)
		api.Get("/notifications/stream", hs.Stream)
	})
	const plazo = 200 * time.Millisecond
	srv := httptest.NewUnstartedServer(r)
	srv.Config.ReadTimeout, srv.Config.WriteTimeout = plazo, plazo
	srv.Start()
	defer srv.Close()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/notifications/stream", http.NoBody)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	eventos := make(chan domain.Notification, 32)
	go leerFlujo(res.Body, eventos)

	time.Sleep(2 * plazo) // los plazos del servidor, vencidos
	fuentes := domain.NotificationSources()
	for _, s := range fuentes {
		cuerpo := `{"tenant":"acme","subject":"ana","source":"` + string(s) + `","type":"` + string(s) + `.prueba",` +
			`"title":"Aviso de ` + string(s) + `","url":"https://corporate.sytue.eu/x","params":{"actor":"Bea"}}`
		pr, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/internal/notifications", strings.NewReader(cuerpo))
		pr.Header.Set("Authorization", "Bearer s")
		pr.Header.Set("Content-Type", "application/json")
		pres, err := srv.Client().Do(pr)
		if err != nil {
			t.Fatal(err)
		}
		_ = pres.Body.Close()
		if pres.StatusCode != http.StatusCreated {
			t.Errorf("origen %q: la base lo rechaza (HTTP %d)", s, pres.StatusCode)
		}
	}

	// Y uno para quien el emisor solo conoce por su dirección (un invitado de Calendar):
	// llega a quien la tenga en su token.
	cuerpo := `{"tenant":"acme","subject":"` + hubnotify.PorCorreo("ana@acme.es") + `","source":"calendar",` +
		`"type":"calendar.invitation","title":"Invitación"}`
	pr, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/internal/notifications", strings.NewReader(cuerpo))
	pr.Header.Set("Authorization", "Bearer s")
	pr.Header.Set("Content-Type", "application/json")
	if pres, err := srv.Client().Do(pr); err != nil || pres.StatusCode != http.StatusCreated {
		t.Fatalf("aviso por dirección: %v", err)
	} else {
		_ = pres.Body.Close()
	}

	llegados := map[domain.Source]domain.Notification{}
	limite := time.After(5 * time.Second)
	porCorreo := false
	for len(llegados) < len(fuentes) || !porCorreo {
		select {
		case n := <-eventos:
			if n.Type == "calendar.invitation" {
				porCorreo = true
				continue
			}
			llegados[n.Source] = n
		case <-limite:
			t.Fatalf("por el flujo llegaron %d de %d avisos (el de la dirección: %v)", len(llegados), len(fuentes), porCorreo)
		}
	}
	uno := llegados[domain.SourceRooms]
	if uno.URL != "https://corporate.sytue.eu/x" || uno.Params["actor"] != "Bea" {
		t.Fatalf("el aviso llega sin su enlace o sus variables: %+v", uno)
	}

	if n := sinLeer(t, srv); n != len(fuentes)+1 {
		t.Fatalf("sin leer = %d, quiero %d", n, len(fuentes)+1)
	}
	lr, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/api/v1/notifications/"+uno.ID.String()+"/read", http.NoBody)
	lres, err := srv.Client().Do(lr)
	if err != nil {
		t.Fatal(err)
	}
	_ = lres.Body.Close()
	if n := sinLeer(t, srv); n != len(fuentes) {
		t.Fatalf("tras leer uno, sin leer = %d, quiero %d", n, len(fuentes))
	}
}

// esperarListen espera a que el relay esté escuchando: un aviso creado antes no lo
// oye nadie y el test fallaría por la carrera, no por lo que guarda.
func esperarListen(t *testing.T, h *pgtest.Harness) {
	t.Helper()
	for range 100 {
		var n int
		_ = h.Pool.QueryRow(context.Background(),
			`SELECT count(*) FROM pg_stat_activity WHERE query LIKE 'LISTEN%' AND datname = current_database()`).Scan(&n)
		if n > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("el relay no llegó a escuchar")
}

func leerFlujo(cuerpo io.Reader, eventos chan<- domain.Notification) {
	sc := bufio.NewScanner(cuerpo)
	evento := ""
	for sc.Scan() {
		l := sc.Text()
		switch {
		case strings.HasPrefix(l, "event: "):
			evento = strings.TrimPrefix(l, "event: ")
		case strings.HasPrefix(l, "data: ") && evento == "notification":
			var n domain.Notification
			if json.Unmarshal([]byte(strings.TrimPrefix(l, "data: ")), &n) == nil {
				eventos <- n
			}
		}
	}
}

func sinLeer(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/v1/notifications/unread_count", http.NoBody)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var v struct {
		Unread int `json:"unread"`
	}
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v.Unread
}

// Un evento de ESTADO —Sytue ID: «cambió su foto»— llega por el mismo flujo con su
// nombre y sus datos, y no deja rastro: ni en la lista de la campana ni en el
// contador. Y el Hub no reparte nombres que no conoce: `notification` en manos de un
// emisor se haría pasar por un aviso.
func TestUnEventoDeEstadoLlegaAlFlujoYNoSeGuarda(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := store.New(h.Pool)
	hub := notify.NewHub(32)
	go func() { _ = notify.NewRelay(h.Pool, hub).Run(ctx) }()
	esperarListen(t, h)

	hs := &Handlers{cfg: config.Config{InternalSecret: "s"}, notify: notify.NewService(st, hub), hub: hub, store: st}
	r := chi.NewRouter()
	r.With(hs.internalAuth).Post("/internal/events", hs.CreateInternalEvent)
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c := authn.Claims{Subject: "ana", Tenant: "acme"}
				next.ServeHTTP(w, r.WithContext(tenancy.Into(authn.Into(r.Context(), c), tenancy.Tenant{Slug: "acme"})))
			})
		})
		api.Get("/notifications", hs.ListNotifications)
		api.Get("/notifications/unread_count", hs.UnreadCount)
		api.Get("/notifications/stream", hs.Stream)
	})
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/notifications/stream", http.NoBody)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	llegan := make(chan [2]string, 8)
	go func() {
		sc := bufio.NewScanner(res.Body)
		evento := ""
		for sc.Scan() {
			l := sc.Text()
			if v, ok := strings.CutPrefix(l, "event: "); ok {
				evento = v
			} else if v, ok := strings.CutPrefix(l, "data: "); ok && evento != "ready" {
				llegan <- [2]string{evento, v}
			}
		}
	}()

	mandar := func(evento string) int {
		cuerpo := `{"tenant":"acme","subject":"ana","event":"` + evento + `","data":{"version":"lq3x9"}}`
		pr, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/internal/events", strings.NewReader(cuerpo))
		pr.Header.Set("Authorization", "Bearer s")
		pr.Header.Set("Content-Type", "application/json")
		pres, err := srv.Client().Do(pr)
		if err != nil {
			t.Fatal(err)
		}
		_ = pres.Body.Close()
		return pres.StatusCode
	}
	if c := mandar("notification"); c != http.StatusBadRequest {
		t.Fatalf("un evento `notification` por la puerta de estado: HTTP %d, quiero 400", c)
	}
	if c := mandar(hubnotify.EventoAvatar); c != http.StatusAccepted {
		t.Fatalf("el evento de la foto: HTTP %d, quiero 202", c)
	}
	select {
	case e := <-llegan:
		if e[0] != "avatar" || e[1] != `{"version":"lq3x9"}` {
			t.Fatalf("llega %q %s, quiero avatar {\"version\":\"lq3x9\"}", e[0], e[1])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("el evento de la foto no llegó por el flujo")
	}
	lr, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/notifications", http.NoBody)
	lres, err := srv.Client().Do(lr)
	if err != nil {
		t.Fatal(err)
	}
	lista, _ := io.ReadAll(lres.Body)
	_ = lres.Body.Close()
	if n := sinLeer(t, srv); n != 0 || strings.Contains(string(lista), "avatar") {
		t.Fatalf("el evento de estado dejó rastro en la campana: %d sin leer, lista %s", n, lista)
	}
}

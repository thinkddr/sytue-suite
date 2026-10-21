// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package hubhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

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

// La reconstrucción de avisos (hubnotify.Historico), de punta a punta: el cliente de
// los servicios contra la ruta interna y lo que ve después la campana de cada cual.
// Guarda las cinco cosas que se pidieron: un aviso por elemento aunque se lance dos
// veces, la fecha real, la marca, que no suene y que nadie vea el de otro.
func TestAvisosHistoricos(t *testing.T) {
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
	r.With(hs.internalAuth).Post("/internal/notifications/historic", hs.CreateInternalHistoricNotification)
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sub := r.Header.Get("X-Prueba-Sub")
				c := authn.Claims{Subject: sub, Tenant: "acme"}
				next.ServeHTTP(w, r.WithContext(tenancy.Into(authn.Into(r.Context(), c), tenancy.Tenant{Slug: "acme"})))
			})
		})
		api.Get("/notifications", hs.ListNotifications)
		api.Get("/notifications/stream", hs.Stream)
	})
	srv := httptest.NewServer(r)
	defer srv.Close()
	cli := hubnotify.New(srv.URL, "s")

	// La campana de Ana, abierta: lo reconstruido no puede llegar por aquí.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/notifications/stream", http.NoBody)
	req.Header.Set("X-Prueba-Sub", "ana")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	eventos := make(chan domain.Notification, 32)
	go leerFlujo(res.Body, eventos)

	// Fechas relativas a hoy y los techos movidos a hoy: la prueba tiene que valer
	// también después del cierre de la ruta y de que pasen los 90 días de la ventana.
	hoy := time.Now().UTC().Truncate(time.Hour)
	hasta, cierre := domain.HistoricoHasta, domain.HistoricoCierre
	domain.HistoricoHasta, domain.HistoricoCierre = hoy, hoy.Add(24*time.Hour)
	defer func() { domain.HistoricoHasta, domain.HistoricoCierre = hasta, cierre }()
	dia := func(n int) time.Time { return hoy.Add(time.Duration(n) * 24 * time.Hour) }
	cuando := dia(-24).Add(10*time.Hour + 30*time.Minute)
	aviso := func(para, clave string, fecha time.Time) hubnotify.Historico {
		return hubnotify.Historico{
			Notification: hubnotify.Notification{
				Tenant: "acme", Subject: para, Source: "mail", Type: "mail.message.received",
				Title: "Nuevo correo de Bea", Body: "asunto", URL: "https://mail.sytue.eu/x",
				Params: map[string]string{"from": "Bea"},
			},
			Clave: clave, Fecha: fecha,
		}
	}
	lote := []hubnotify.Historico{aviso("ana", "mail:1", cuando), aviso("bea", "mail:1", cuando), aviso("bea", "mail:2", cuando)}
	for vuelta := range 2 {
		var out bytes.Buffer
		if err := hubnotify.Reconstruir(ctx, cli, &out, lote); err != nil {
			t.Fatalf("vuelta %d: %v", vuelta, err)
		}
		quiero := "mail.message.received creado 3\n"
		if vuelta == 1 {
			quiero = "mail.message.received repetido 3\n"
		}
		if out.String() != quiero {
			t.Fatalf("vuelta %d: recuento %q, quiero %q", vuelta, out.String(), quiero)
		}
	}

	t.Run("uno por elemento y persona, con su fecha, leído y marcado", func(t *testing.T) {
		de := campana(t, srv, "ana")
		if len(de) != 1 {
			t.Fatalf("Ana tiene %d avisos, quiero 1: %+v", len(de), de)
		}
		n := de[0]
		if !n.CreatedAt.Equal(cuando) {
			t.Errorf("fecha %v, quiero la real %v", n.CreatedAt, cuando)
		}
		if !n.Read || n.ReadAt == nil {
			t.Errorf("tiene que nacer leído: %+v", n)
		}
		if !n.Historic {
			t.Errorf("sin la marca de anterior al registro: %+v", n)
		}
		if len(campana(t, srv, "bea")) != 2 {
			t.Errorf("Bea tiene que tener los suyos dos")
		}
	})

	t.Run("dos del mismo instante no se pierden al pasar de página", func(t *testing.T) {
		primera := campanaCon(t, srv, "bea", "?limit=1")
		if len(primera) != 1 {
			t.Fatalf("primera página: %+v", primera)
		}
		u := primera[0]
		resto := campanaCon(t, srv, "bea", "?limit=1&before="+u.CreatedAt.Format(time.RFC3339Nano)+"&before_id="+u.ID.String())
		if len(resto) != 1 || resto[0].ID == u.ID || !resto[0].CreatedAt.Equal(u.CreatedAt) {
			t.Fatalf("la segunda página pierde el del mismo instante: %+v", resto)
		}
	})

	t.Run("nadie ve el de otro", func(t *testing.T) {
		for _, n := range campana(t, srv, "carla") {
			t.Errorf("Carla ve un aviso que no es suyo: %+v", n)
		}
	})

	t.Run("no suena", func(t *testing.T) {
		// Uno en vivo después: si el flujo trae ESTE y no los de antes, lo reconstruido
		// no pasó por el tiempo real.
		if err := cli.Send(ctx, aviso("ana", "", time.Time{}).Notification); err != nil {
			t.Fatal(err)
		}
		select {
		case n := <-eventos:
			if n.Historic || n.CreatedAt.Equal(cuando) {
				t.Fatalf("un aviso reconstruido sonó en la campana: %+v", n)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("ni siquiera llegó el aviso en vivo: el flujo no funciona y la prueba no prueba nada")
		}
	})

	t.Run("de cuando el registro ya funcionaba, no", func(t *testing.T) {
		// El primer aviso en vivo de docs.shared fue hace 17 días: lo de hace 15 ya se avisó,
		// y lo de unos minutos antes también (el elemento que lo causó va antes que su aviso).
		if err := h.Pool.InTenantTx(tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"}), func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `INSERT INTO notifications (tenant, subject, source, type, title, created_at)
				VALUES ('acme', 'dani', 'docs', 'docs.shared', 'en vivo', $1)`, dia(-17))
			return e
		}); err != nil {
			t.Fatal(err)
		}
		compartido := func(fecha time.Time) hubnotify.Historico {
			a := aviso("ana", "docs:shared:"+fecha.Format(time.DateOnly), fecha)
			a.Source, a.Type = "docs", "docs.shared"
			return a
		}
		for fecha, quiero := range map[time.Time]string{
			dia(-15):                             domain.HistoricoPosterior,
			dia(-17).Add(-5 * time.Minute):       domain.HistoricoPosterior,
			dia(-19):                             domain.HistoricoCreado,
			domain.HistoricoHasta.Add(time.Hour): domain.HistoricoPosterior,
			time.Now().Add(time.Hour):            domain.HistoricoPosterior,
			time.Now().Add(-domain.VentanaHistorica - time.Hour): domain.HistoricoAntiguo,
		} {
			if got, err := cli.SendHistorico(ctx, compartido(fecha)); err != nil || got != quiero {
				t.Errorf("fechado %v: %q (%v), quiero %q", fecha, got, err, quiero)
			}
		}
	})

	t.Run("lo que la persona calla, callado", func(t *testing.T) {
		if err := st.SetNotificationPref(tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"}), "ana",
			domain.NotificationPref{Source: domain.SourceRooms, Muted: true}); err != nil {
			t.Fatal(err)
		}
		a := aviso("ana", "rooms:1", cuando)
		a.Source, a.Type = "rooms", "rooms.message"
		if got, err := cli.SendHistorico(ctx, a); err != nil || got != domain.HistoricoSilenciado {
			t.Fatalf("%q (%v), quiero %q", got, err, domain.HistoricoSilenciado)
		}
	})

	t.Run("la marca no se quita", func(t *testing.T) {
		err := h.Pool.InTenantTx(tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"}), func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `UPDATE notifications SET clave_origen = NULL WHERE clave_origen IS NOT NULL`)
			return e
		})
		if err == nil {
			t.Fatal("se pudo quitar la marca de un aviso reconstruido")
		}
	})

	t.Run("una dirección retirada no hereda nada de antes", func(t *testing.T) {
		vieja := hubnotify.PorCorreo("vieja@acme.es")
		if _, err := st.OlvidarBuzon(ctx, vieja); err != nil {
			t.Fatal(err)
		}
		if got, err := cli.SendHistorico(ctx, aviso(vieja, "calendar:invitation:1", cuando)); err != nil || got != domain.HistoricoOlvidado {
			t.Fatalf("%q (%v), quiero %q", got, err, domain.HistoricoOlvidado)
		}
	})

	t.Run("nadie puede llenar la campana de otro", func(t *testing.T) {
		if err := h.Pool.InTenantTx(tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"}), func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `INSERT INTO notifications (tenant, subject, source, type, title, read, created_at, clave_origen)
				SELECT 'acme', 'fer', 'mail', 'mail.message.received', 'x', true, $2, 'relleno:' || g
				FROM generate_series(1, $1) g`, domain.TopeHistoricosPorPersona, dia(-30))
			return e
		}); err != nil {
			t.Fatal(err)
		}
		if got, err := cli.SendHistorico(ctx, aviso("fer", "mail:uno-mas", cuando)); err != nil || got != domain.HistoricoTope {
			t.Fatalf("pasado el tope de reconstruidos por persona: %q (%v), quiero %q", got, err, domain.HistoricoTope)
		}
		// Desde otra organización —inventada: el Hub no sabe cuáles existen— tampoco.
		otra := aviso("fer", "mail:de-otra", cuando)
		otra.Tenant = "inventada"
		if got, err := cli.SendHistorico(ctx, otra); err != nil || got != domain.HistoricoTope {
			t.Fatalf("el tope por persona se rodea desde otra organización: %q (%v)", got, err)
		}
		// Y en vivo: avisos de la última hora repartidos entre organizaciones inventadas.
		if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
			if _, e := tx.Exec(ctx, "SELECT set_config('app.maintenance', 'on', true)"); e != nil {
				return e
			}
			_, e := tx.Exec(ctx, `INSERT INTO notifications (tenant, subject, source, type, title)
				SELECT 'org' || (g % 20), 'gil', 'mail', 't' || g, 'x' FROM generate_series(1, $1) g`, notify.TopeAvisosPersonaPorHora)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		if err := cli.Send(ctx, hubnotify.Notification{Tenant: "acme", Subject: "gil", Source: "mail", Type: "otro", Title: "x"}); err == nil {
			t.Fatal("en vivo, repartir entre organizaciones inventadas rodea el tope por persona")
		}
	})

	t.Run("la ruta cierra", func(t *testing.T) {
		antes := domain.HistoricoCierre
		domain.HistoricoCierre = time.Now().Add(-time.Minute)
		defer func() { domain.HistoricoCierre = antes }()
		if _, err := cli.SendHistorico(ctx, aviso("ana", "mail:tarde", cuando)); err == nil || !strings.Contains(err.Error(), "410") {
			t.Fatalf("pasado el cierre, la ruta histórica sigue abierta: %v", err)
		}
	})

	t.Run("sin el secreto, nada", func(t *testing.T) {
		if _, err := hubnotify.New(srv.URL, "otro").SendHistorico(ctx, aviso("ana", "mail:3", cuando)); err == nil {
			t.Fatal("la ruta histórica aceptó un secreto que no es")
		}
	})
}

func campana(t *testing.T, srv *httptest.Server, sub string) []domain.Notification {
	t.Helper()
	return campanaCon(t, srv, sub, "")
}

func campanaCon(t *testing.T, srv *httptest.Server, sub, consulta string) []domain.Notification {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/v1/notifications"+consulta, http.NoBody)
	req.Header.Set("X-Prueba-Sub", sub)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var v struct {
		Notifications []domain.Notification `json:"notifications"`
	}
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v.Notifications
}

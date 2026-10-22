// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package casthttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/clientip"
	"git.sytue.eu/sytue/one/platform/pkg/operador"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/cast/db"
	"git.sytue.eu/sytue/one/services/cast/internal/config"
	"git.sytue.eu/sytue/one/services/cast/internal/media"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
	"git.sytue.eu/sytue/one/services/cast/internal/social"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// La denuncia sin sesión pasa por el router de verdad: sin la prueba de trabajo no
// entra, con ella sí y una sola vez, y la respuesta no lleva nada.
func TestDenunciaSinSesionExigeLaPruebaDeTrabajo(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	st := store.New(h.Pool)
	svc := service.New(service.Deps{Store: st, Firmante: media.New("clave-de-prueba")})
	clave := []byte("clave-de-las-denuncias")
	soc := social.New(h.Pool, svc.AccesoSocial()).ConModeracion(social.Moderacion{Secreto: clave})
	// Dos réplicas: cada una con su verificador, las dos sobre la misma tabla de
	// soluciones usadas (migración 0012), como en cmd/cast.
	replica := func() http.Handler {
		return NewRouter(Deps{Store: st, Servicio: svc, Social: soc,
			Altcha: altcha.New(clave, altcha.ConRepeticiones(altcha.EnTabla(h.Pool, "altcha_used")))})
	}
	router, otraReplica := replica(), replica()

	var id string
	if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO videos (tenant, owner_sub, kind, status, visibility, title)
			VALUES ('acme', 'duenya', 'upload', 'ready', 'public', 'Público')
			RETURNING id::text`).Scan(&id)
	}); err != nil {
		t.Fatal(err)
	}

	pedir := func(metodo, ruta, cuerpo string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
		req.Header.Set("Content-Type", "application/json")
		// Un token cualquiera no convierte la ruta pública en una con sesión.
		req.Header.Set("Authorization", "Bearer no-vale-nada")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	abiertas := func() (n int) {
		t.Helper()
		if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
			if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
				return err
			}
			return tx.QueryRow(ctx, `SELECT count(*) FROM reports WHERE reporter_sub = ''`).Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	ruta := "/api/v1/public/videos/" + id + "/reports"

	// Sin prueba de trabajo, o con una inventada: 400 y nada guardado.
	for _, cuerpo := range []string{`{"kind":"video","reason":"spam"}`, `{"kind":"video","reason":"spam","altcha":"aW52ZW50YWRh"}`} {
		if rec := pedir(http.MethodPost, ruta, cuerpo); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "altcha_invalid") {
			t.Fatalf("sin prueba de trabajo: %d %s", rec.Code, rec.Body)
		}
	}
	if n := abiertas(); n != 0 {
		t.Fatalf("se guardaron %d denuncias sin prueba de trabajo", n)
	}

	// Con el reto resuelto: 204, sin cuerpo, y la fila está.
	rec := pedir(http.MethodGet, "/api/v1/public/reports/altcha", "")
	var reto altcha.Challenge
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &reto) != nil {
		t.Fatalf("reto: %d %s", rec.Code, rec.Body)
	}
	solucion, err := altcha.SolveChallenge(reto)
	if err != nil {
		t.Fatal(err)
	}
	cuerpo, _ := json.Marshal(map[string]string{"kind": "video", "reason": "violence", "details": "se ve una pelea", "altcha": solucion})
	if rec := pedir(http.MethodPost, ruta, string(cuerpo)); rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("denuncia con la prueba resuelta: %d %s", rec.Code, rec.Body)
	}
	if n := abiertas(); n != 1 {
		t.Fatalf("denuncias anónimas guardadas: %d", n)
	}
	// La misma solución no vale dos veces, tampoco en otra réplica.
	if rec := pedir(http.MethodPost, ruta, string(cuerpo)); rec.Code != http.StatusBadRequest {
		t.Fatalf("solución repetida: %d %s", rec.Code, rec.Body)
	}
	router = otraReplica
	if rec := pedir(http.MethodPost, ruta, string(cuerpo)); rec.Code != http.StatusBadRequest {
		t.Fatalf("solución repetida en otra réplica: %d %s", rec.Code, rec.Body)
	}
}

// La cola del operador por HTTP: solo con un token de la consola de plataforma y
// el sí de Sytue ID —que recibe qué se va a hacer y sobre qué organización—; un
// administrador de organización no entra, y quien no es operador no sabe si una
// denuncia existe.
func TestColaDelOperadorPorHTTP(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	st := store.New(h.Pool)
	svc := service.New(service.Deps{Store: st, Firmante: media.New("clave-de-prueba")})
	soc := social.New(h.Pool, svc.AccesoSocial()).ConModeracion(social.Moderacion{Secreto: []byte("k")})

	var rid string
	if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO reports (tenant, kind, target_id, target_sub, reason, reporter_sub, reporter_key)
			VALUES ('acme', 'person', 'bea', 'bea', 'spam', 'zoe', 'sub:zoe') RETURNING id::text`).Scan(&rid)
	}); err != nil {
		t.Fatal(err)
	}

	var pedidos []operador.Acceso
	respuesta := error(nil)
	hd := &Handlers{svc: svc, social: soc, operador: func(_ context.Context, token string, a operador.Acceso) error {
		if token != "tok" {
			t.Errorf("a Sytue ID no le llega el token de quien llama: %q", token)
		}
		pedidos = append(pedidos, a)
		return respuesta
	}}
	llamar := func(aud []string, roles []string, metodo, id, cuerpo string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(metodo, "/x", strings.NewReader(cuerpo))
		req.Header.Set("Authorization", "Bearer tok")
		rc := chi.NewRouteContext()
		rc.URLParams.Add("rid", id)
		c := authn.Claims{Subject: "operadora", Tenant: "corporate", Audience: aud, Roles: roles}
		req = req.WithContext(context.WithValue(authn.Into(req.Context(), c), chi.RouteCtxKey, rc))
		rec := httptest.NewRecorder()
		if metodo == http.MethodGet {
			hd.DenunciasOperador(rec, req)
		} else {
			hd.ResolverDenunciaOperador(rec, req)
		}
		return rec
	}
	consola := []string{"https://id.sytue.eu", "sytue-platform"}

	// Un token de la web de Cast —aunque sea de alguien que administra su
	// organización— no llega ni a preguntar.
	for _, metodo := range []string{http.MethodGet, http.MethodPost} {
		if rec := llamar([]string{"sytue-cast"}, []string{"owner"}, metodo, rid, `{"action":"dismiss"}`); rec.Code != http.StatusForbidden {
			t.Fatalf("%s con token de Cast: %d", metodo, rec.Code)
		}
	}
	if len(pedidos) != 0 {
		t.Fatalf("se preguntó a Sytue ID con un token que no es de la consola: %+v", pedidos)
	}

	// Sytue ID dice que no: 403 y nada en el cuerpo; no sabe contestar: 503.
	respuesta = operador.ErrNoEsOperador
	if rec := llamar(consola, nil, http.MethodGet, "", ""); rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), rid) {
		t.Fatalf("no operador: %d %s", rec.Code, rec.Body)
	}
	if rec := llamar(consola, nil, http.MethodPost, "00000000-0000-4000-8000-000000000000", `{"action":"dismiss"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("no operador, denuncia que no existe: %d (no debe saber que no existe)", rec.Code)
	}
	respuesta = errors.New("sin red")
	if rec := llamar(consola, nil, http.MethodGet, "", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Sytue ID sin contestar: %d", rec.Code)
	}

	// Sytue ID dice que sí.
	respuesta, pedidos = nil, nil
	rec := llamar(consola, nil, http.MethodGet, "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), rid) || !strings.Contains(rec.Body.String(), `"tenant":"acme"`) {
		t.Fatalf("cola: %d %s", rec.Code, rec.Body)
	}
	if len(pedidos) != 1 || pedidos[0].Servicio != "cast" || pedidos[0].Accion != "reports.list" {
		t.Fatalf("lo que se dijo a Sytue ID al listar: %+v", pedidos)
	}
	if rec := llamar(consola, nil, http.MethodPost, "00000000-0000-4000-8000-000000000000", `{"action":"dismiss"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("operadora, denuncia que no existe: %d", rec.Code)
	}
	pedidos = nil
	if rec := llamar(consola, nil, http.MethodPost, rid, `{"action":"block"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("resolver: %d %s", rec.Code, rec.Body)
	}
	if len(pedidos) != 1 || pedidos[0].Accion != "report.resolve" || pedidos[0].Org != "acme" ||
		pedidos[0].Objetivo != rid || pedidos[0].Detalle["action"] != "block" {
		t.Fatalf("lo que se dijo a Sytue ID al resolver: %+v", pedidos)
	}
	var desde, por string
	if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT resolved_from, resolved_by FROM reports WHERE id = $1`, rid).Scan(&desde, &por)
	}); err != nil || desde != "platform" || por != "operadora" {
		t.Fatalf("resuelta desde %q por %q: %v", desde, por, err)
	}
}

// La IP de quien denuncia sin cuenta (tope y deduplicación) sale del
// X-Forwarded-For SOLO si la petición llega desde un proxy de confianza —el BFF de
// la web o Traefik, dentro del clúster—. Un cliente que llama directo con su
// propia cabecera se queda con la IP desde la que conecta: no puede hacerse pasar
// por muchas personas para denunciar lo mismo una y otra vez.
func TestLaIPDeLaDenunciaSoloSeCreeDesdeElBFF(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	st := store.New(h.Pool)
	svc := service.New(service.Deps{Store: st, Firmante: media.New("clave-de-prueba")})
	clave := []byte("clave-de-las-denuncias")
	soc := social.New(h.Pool, svc.AccesoSocial()).ConModeracion(social.Moderacion{Secreto: clave})
	verificador := altcha.New(clave)

	var id string
	if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO videos (tenant, owner_sub, kind, status, visibility, title)
			VALUES ('acme', 'duenya', 'upload', 'ready', 'public', 'Público')
			RETURNING id::text`).Scan(&id)
	}); err != nil {
		t.Fatal(err)
	}
	// Dos denuncias iguales, cada una con su X-Forwarded-For, desde `desde`; devuelve
	// cuántas filas distintas dejaron (una por IP que el servicio cree).
	filas := func(cfg config.Config, desde string) int {
		t.Helper()
		router := NewRouter(Deps{Cfg: cfg, Store: st, Servicio: svc, Social: soc, Altcha: verificador})
		// Distintas por la izquierda y por la derecha: ninguna lectura ingenua de la
		// cabecera las junta.
		for _, xff := range []string{"203.0.113.1", "203.0.113.9, 203.0.113.2"} {
			reto, err := verificador.CrearReto(ambitoDenuncia, dificultadDenuncia, vidaRetoDenuncia)
			if err != nil {
				t.Fatal(err)
			}
			sol, err := altcha.SolveChallenge(reto)
			if err != nil {
				t.Fatal(err)
			}
			cuerpo, _ := json.Marshal(map[string]string{"kind": "video", "reason": "spam", "altcha": sol})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/public/videos/"+id+"/reports", strings.NewReader(string(cuerpo)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Forwarded-For", xff)
			req.RemoteAddr = desde
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusNoContent {
				t.Fatalf("denuncia desde %s con %q: %d %s", desde, xff, rec.Code, rec.Body)
			}
		}
		var n int
		if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
			if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM reports WHERE reporter_sub = ''`).Scan(&n); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `DELETE FROM reports`)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Desde el BFF (red privada): la cabecera manda, son dos personas.
	if n := filas(config.Config{}, "10.42.0.9:40000"); n != 2 {
		t.Fatalf("desde el BFF: %d filas, quiero 2 (una por cliente)", n)
	}
	// Un cliente directo desde internet inventándose la cabecera: es una sola persona.
	if n := filas(config.Config{}, "198.51.100.7:40000"); n != 1 {
		t.Fatalf("un cliente directo falsifica su IP: %d filas, quiero 1", n)
	}
	// Con TRUSTED_PROXIES=none, ni siquiera el BFF: todo cuenta como su IP.
	if n := filas(config.Config{TrustedProxies: clientip.NoTrustedProxies()}, "10.42.0.9:40000"); n != 1 {
		t.Fatalf("sin proxies de confianza: %d filas, quiero 1", n)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package pageshttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// WEB-05. Una denuncia sin reto ALTCHA no entra (antes bastaba con no mandarlo), una
// solución no vale dos veces, y el buzón de abuso recibe un aviso por sitio y hora: el
// store dice cuántas lleva el sitio en la última hora y solo la primera avisa.
func TestUnaDenunciaExigeSuRetoYAvisaUnaVezPorHora(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	var sitio, otro uuid.UUID
	for slug, id := range map[string]*uuid.UUID{"denunciada": &sitio, "otra": &otro} {
		if err := super.QueryRow(ctx, `INSERT INTO sites (tenant, slug, name, draft, created_by)
			VALUES ('org', $1, 'Web', '{}', 'sub') RETURNING id`, slug).Scan(id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := super.Exec(ctx, `INSERT INTO domains (tenant, site_id, host, token, status, created_by)
		VALUES ('org', $1, 'denunciada.example.test', '0123456789abcdef0123456789abcdef', 'active', 'sub')`, sitio); err != nil {
		t.Fatal(err)
	}
	st := store.New(super)
	verificador := altcha.New([]byte("clave-de-pruebas-de-32-bytes!!!!"), altcha.ConRepeticiones(st.RegistrarAltcha))
	r := chi.NewRouter()
	g := grupos{publico: r}
	hd := &Handlers{Deps: Deps{Store: st, Altcha: verificador}}
	montarPublicoTienda(g, hd)
	montarDenuncias(g, hd)

	denunciar := func(reto string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/reports", strings.NewReader(
			`{"reason":"phishing","details":"Suplanta a un banco","path":"/","altcha":"`+reto+`"}`))
		req.Host = "denunciada.example.test"
		req.Header.Set("Origin", "https://denunciada.example.test")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}
	resuelto := func() string {
		t.Helper()
		ch, err := verificador.CreateChallenge(500, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		sol, err := altcha.SolveChallenge(ch)
		if err != nil {
			t.Fatal(err)
		}
		return sol
	}
	guardadas := func() int {
		t.Helper()
		var n int
		if err := super.QueryRow(ctx, `SELECT count(*) FROM abuse_reports WHERE site_id = $1`, sitio).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	if code := denunciar(""); code != http.StatusUnprocessableEntity || guardadas() != 0 {
		t.Fatalf("denuncia sin reto: %d y %d guardadas; quiero 422 y ninguna", code, guardadas())
	}
	reto := resuelto()
	if code := denunciar(reto); code != http.StatusNoContent || guardadas() != 1 {
		t.Fatalf("denuncia con reto: %d y %d guardadas; quiero 204 y una", code, guardadas())
	}
	if code := denunciar(reto); code != http.StatusUnprocessableEntity || guardadas() != 1 {
		t.Fatalf("la misma solución otra vez: %d y %d guardadas; quiero 422 y una", code, guardadas())
	}

	// El aviso: la primera de la hora de cada sitio, y solo ella.
	for i, quiero := range []int{1, 2} {
		n, err := st.CrearDenunciaAbuso(ctx, &domain.AbuseReport{SiteID: otro, Host: "otra.example.test", Reason: domain.AbuseReasonOther})
		if err != nil || n != quiero {
			t.Fatalf("denuncia %d del otro sitio: %d en la hora (%v); quiero %d", i+1, n, err, quiero)
		}
	}
}

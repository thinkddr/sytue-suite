// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de los logs de runtime buscables (WS4/O1) contra un
// PostgreSQL real (build-tag `integration`). Aquí vive lo que NO se puede
// demostrar con una función pura:
//   - SearchRuntimeLogs: recorrido keyset completo (sin huecos ni repeticiones),
//     filtro por texto (con metacaracteres LIKE literales) y por rango;
//   - RLS: el tenant vecino no ve ni una línea, ni siquiera con SQL crudo;
//   - AppendRuntimeLogs: guarda bytes que PostgreSQL rechazaría (NUL, UTF-8 roto)
//     y adelanta el corte de recolección al último ts guardado;
//   - ClaimLogCollection: arrienda (una segunda reclamación no repite) y solo
//     toma deployments vivos;
//   - PurgeExpired: caduca los logs pasados RuntimeLogRetention.
package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// seedReadyDeployment crea un deployment y lo deja 'ready' (que es el estado que
// mira el recolector de logs).
func seedReadyDeployment(t *testing.T, ctx context.Context, st *store.Store, tenant string, projectID uuid.UUID) domain.Deployment {
	t.Helper()
	d, err := st.CreateDeployment(ctx, store.NewDeploymentRecord{
		ProjectID: projectID, Target: domain.TargetProduction, GitRef: "main",
		URL: "https://web.apps.sytue.eu",
	})
	if err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}
	if err := st.SetDeploymentStatus(ctx, tenant, d.ID, domain.DeployReady, d.URL, ""); err != nil {
		t.Fatalf("SetDeploymentStatus: %v", err)
	}
	return d
}

func lines(base time.Time, texts ...string) []domain.LogLine {
	out := make([]domain.LogLine, 0, len(texts))
	for i, s := range texts {
		out = append(out, domain.LogLine{TS: base.Add(time.Duration(i) * time.Second), Text: s})
	}
	return out
}

// TestSearchRuntimeLogsKeyset recorre TODA la búsqueda página a página y exige
// orden estricto (ts, id) DESC, cobertura completa y fin de cursor. Es la
// garantía de que la paginación es keyset de verdad: sin OFFSET, un recorrido con
// escrituras concurrentes no puede saltarse ni repetir líneas.
func TestSearchRuntimeLogsKeyset(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	p := seedProject(t, ctx, st, "web")
	d := seedReadyDeployment(t, ctx, st, "acme", p.ID)

	base := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	const total = 15
	texts := make([]string, 0, total)
	for i := 0; i < total; i++ {
		// Mismo segundo cada 3 líneas: fuerza EMPATES de ts, que es justo lo que
		// tiene que desempatar el id del cursor.
		texts = append(texts, fmt.Sprintf("línea %02d", i))
	}
	batch := make([]domain.LogLine, 0, total)
	for i, s := range texts {
		batch = append(batch, domain.LogLine{TS: base.Add(time.Duration(i/3) * time.Second), Text: s})
	}
	if n, err := st.AppendRuntimeLogs(context.Background(), "acme", p.ID, d.ID, batch); err != nil || n != total {
		t.Fatalf("AppendRuntimeLogs = %d, %v; quiero %d, nil", n, err, total)
	}

	seen := map[uuid.UUID]bool{}
	cursor := store.PageCursor{}
	prevTS, prevID := time.Time{}, uuid.Nil
	pages := 0
	for {
		page, err := st.SearchRuntimeLogs(ctx, p.ID, store.RuntimeLogFilter{}, cursor, 4)
		if err != nil {
			t.Fatalf("SearchRuntimeLogs(página %d): %v", pages, err)
		}
		pages++
		for _, l := range page.Lines {
			if seen[l.ID] {
				t.Fatalf("línea repetida en el recorrido keyset: %s", l.ID)
			}
			seen[l.ID] = true
			if !prevTS.IsZero() && !pageBefore(prevTS, prevID, l.TS, l.ID) {
				t.Fatalf("orden roto: (%s,%s) no precede a (%s,%s)", prevTS, prevID, l.TS, l.ID)
			}
			prevTS, prevID = l.TS, l.ID
		}
		if page.Next == nil {
			break
		}
		cursor = *page.Next
		if pages > total {
			t.Fatal("el cursor no termina: bucle infinito")
		}
	}
	if len(seen) != total {
		t.Errorf("el recorrido vio %d líneas de %d", len(seen), total)
	}
}

// TestSearchRuntimeLogsFilters comprueba el filtro de texto (subcadena
// case-insensitive), que los metacaracteres de LIKE se tratan como TEXTO (un '%'
// tecleado no devuelve la tabla entera) y el acotado por rango de tiempo.
func TestSearchRuntimeLogsFilters(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	p := seedProject(t, ctx, st, "web")
	d := seedReadyDeployment(t, ctx, st, "acme", p.ID)

	base := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	batch := lines(base,
		"GET /api/v1/health 200",          // +0s
		"ERROR timeout conectando con db", // +1s
		"warn: disco al 100% de uso",      // +2s
		"GET /api/v1/pay 500",             // +3s
	)
	if _, err := st.AppendRuntimeLogs(context.Background(), "acme", p.ID, d.ID, batch); err != nil {
		t.Fatalf("AppendRuntimeLogs: %v", err)
	}

	got := func(f store.RuntimeLogFilter) []string {
		t.Helper()
		page, err := st.SearchRuntimeLogs(ctx, p.ID, f, store.PageCursor{}, 100)
		if err != nil {
			t.Fatalf("SearchRuntimeLogs(%+v): %v", f, err)
		}
		out := make([]string, 0, len(page.Lines))
		for _, l := range page.Lines {
			out = append(out, l.Text)
		}
		return out
	}

	// Subcadena, case-insensitive.
	if res := got(store.RuntimeLogFilter{Query: "error"}); len(res) != 1 || res[0] != batch[1].Text {
		t.Errorf("q=error dio %v", res)
	}
	// '%' es TEXTO, no comodín: sin escapar casaría con las 4 líneas.
	if res := got(store.RuntimeLogFilter{Query: "100%"}); len(res) != 1 || res[0] != batch[2].Text {
		t.Errorf("q=100%% dio %v (¿comodín sin escapar?)", res)
	}
	// '_' tampoco es comodín (casaría cualquier carácter).
	if res := got(store.RuntimeLogFilter{Query: "GET_"}); len(res) != 0 {
		t.Errorf("q=GET_ dio %v; quiero 0 (el _ es literal)", res)
	}
	// Rango [+1s, +3s): deja fuera la primera y la última.
	since, until := base.Add(time.Second), base.Add(3*time.Second)
	res := got(store.RuntimeLogFilter{Since: &since, Until: &until})
	if len(res) != 2 || res[0] != batch[2].Text || res[1] != batch[1].Text {
		t.Errorf("rango dio %v", res)
	}
	// Texto Y rango combinan en AND.
	if res := got(store.RuntimeLogFilter{Query: "GET", Since: &since, Until: &until}); len(res) != 0 {
		t.Errorf("q+rango dio %v; quiero 0", res)
	}
}

// TestRuntimeLogsTenantIsolation exige que el aislamiento lo ponga la BASE DE
// DATOS, no el predicado del store: la comprobación se hace con SQL CRUDO y sin
// filtro de tenant, con el rol de aplicación (NOBYPASSRLS) del harness. Si la
// política de la migración 0024 desapareciera, este test se pone rojo.
func TestRuntimeLogsTenantIsolation(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctxA, ctxB := tenantCtx("acme"), tenantCtx("globex")
	pA := seedProject(t, ctxA, st, "web-a")
	dA := seedReadyDeployment(t, ctxA, st, "acme", pA.ID)
	pB := seedProject(t, ctxB, st, "web-b")
	dB := seedReadyDeployment(t, ctxB, st, "globex", pB.ID)

	base := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	if _, err := st.AppendRuntimeLogs(context.Background(), "acme", pA.ID, dA.ID,
		lines(base, "secreto de acme")); err != nil {
		t.Fatalf("AppendRuntimeLogs: %v", err)
	}
	// El vecino TIENE logs propios: así el 0 de abajo es aislamiento y no una
	// tabla vacía.
	if _, err := st.AppendRuntimeLogs(context.Background(), "globex", pB.ID, dB.ID,
		lines(base, "secreto de globex")); err != nil {
		t.Fatalf("AppendRuntimeLogs(globex): %v", err)
	}

	// Por la puerta de delante: el vecino no ve el proyecto ajeno.
	page, err := st.SearchRuntimeLogs(ctxB, pA.ID, store.RuntimeLogFilter{}, store.PageCursor{}, 100)
	if err != nil {
		t.Fatalf("SearchRuntimeLogs(globex): %v", err)
	}
	if len(page.Lines) != 0 {
		t.Errorf("globex vio %d líneas de acme", len(page.Lines))
	}
	// Por la puerta de atrás: SQL crudo sin WHERE tenant, con app.tenant=globex.
	var n int
	err = h.Pool.InTx(context.Background(), func(tx pgx.Tx) error {
		if err := pg.SetTenant(context.Background(), tx, "globex"); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM runtime_logs`).Scan(&n)
	})
	if err != nil {
		t.Fatalf("consulta cruda: %v", err)
	}
	if n != 1 {
		t.Errorf("con app.tenant=globex se ven %d filas de runtime_logs; quiero 1 (solo la suya)", n)
	}
	// Y el propio dueño sí las ve (la política no es un 'deny all' disfrazado).
	page, err = st.SearchRuntimeLogs(ctxA, pA.ID, store.RuntimeLogFilter{}, store.PageCursor{}, 100)
	if err != nil || len(page.Lines) != 1 || page.Lines[0].Text != "secreto de acme" {
		t.Fatalf("acme ve %d líneas (err=%v); quiero la suya", len(page.Lines), err)
	}
}

// TestAppendRuntimeLogsSanitiza guarda lo que un contenedor puede escribir de
// verdad y PostgreSQL NO admite en `text`: un byte NUL y UTF-8 roto. Sin
// saneado, el INSERT del lote entero falla y se pierden todos los logs de la
// pasada, no solo la línea envenenada.
func TestAppendRuntimeLogsSanitiza(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	p := seedProject(t, ctx, st, "web")
	d := seedReadyDeployment(t, ctx, st, "acme", p.ID)

	base := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	batch := lines(base,
		"antes\x00después",       // NUL: PostgreSQL lo rechaza en text
		"roto \xff\xfe fin",      // UTF-8 inválido
		"línea sana con acentos", // la que NO debe perderse por culpa de las otras
	)
	n, err := st.AppendRuntimeLogs(context.Background(), "acme", p.ID, d.ID, batch)
	if err != nil {
		t.Fatalf("AppendRuntimeLogs: %v", err)
	}
	if n != 3 {
		t.Fatalf("se guardaron %d líneas de 3", n)
	}
	page, err := st.SearchRuntimeLogs(ctx, p.ID, store.RuntimeLogFilter{Query: "antes"}, store.PageCursor{}, 10)
	if err != nil || len(page.Lines) != 1 {
		t.Fatalf("búsqueda tras saneado: %d líneas, err=%v", len(page.Lines), err)
	}
	if page.Lines[0].Text != "antesdespués" {
		t.Errorf("línea guardada = %q; quiero el NUL fuera", page.Lines[0].Text)
	}
}

// TestClaimLogCollectionLease comprueba las dos propiedades del reparto: solo se
// reclaman deployments VIVOS y la reclamación ARRIENDA (una segunda pasada
// inmediata no vuelve a darlos), que es lo que impide que dos réplicas lean el
// mismo pod y dupliquen sus logs.
func TestClaimLogCollectionLease(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	p := seedProject(t, ctx, st, "web")
	ready := seedReadyDeployment(t, ctx, st, "acme", p.ID)
	// Uno que sigue construyéndose: no tiene pod del que leer.
	queued, err := st.CreateDeployment(ctx, store.NewDeploymentRecord{
		ProjectID: p.ID, Target: domain.TargetProduction, GitRef: "main", URL: "https://x",
	})
	if err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	claimed, err := st.ClaimLogCollection(context.Background(), now, time.Minute, 10)
	if err != nil {
		t.Fatalf("ClaimLogCollection: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != ready.ID {
		t.Fatalf("reclamados %+v; quiero solo el ready %s (no %s)", claimed, ready.ID, queued.ID)
	}
	if !claimed[0].Since.IsZero() {
		t.Errorf("primera reclamación con corte %v; quiero cero (nunca recolectado)", claimed[0].Since)
	}
	if claimed[0].Tenant != "acme" || claimed[0].ProjectID != p.ID {
		t.Errorf("reclamado sin tenant/proyecto: %+v", claimed[0])
	}

	// Arriendo: dentro de la ventana no se vuelve a dar.
	again, err := st.ClaimLogCollection(context.Background(), now.Add(30*time.Second), time.Minute, 10)
	if err != nil {
		t.Fatalf("ClaimLogCollection (2ª): %v", err)
	}
	if len(again) != 0 {
		t.Errorf("segunda reclamación devolvió %d; quiero 0 (arriendo vivo)", len(again))
	}

	// Pasada la ventana vuelve a tocar, y el corte que se entrega es el de la
	// ÚLTIMA LÍNEA guardada (no el reloj de la reclamación): sin eso, las líneas
	// escritas mientras se leía el pod se volverían a leer y a guardar.
	lastTS := now.Add(90 * time.Second)
	if _, err := st.AppendRuntimeLogs(context.Background(), "acme", p.ID, ready.ID,
		[]domain.LogLine{{TS: lastTS, Text: "hola"}}); err != nil {
		t.Fatalf("AppendRuntimeLogs: %v", err)
	}
	next, err := st.ClaimLogCollection(context.Background(), now.Add(5*time.Minute), time.Minute, 10)
	if err != nil {
		t.Fatalf("ClaimLogCollection (3ª): %v", err)
	}
	if len(next) != 1 || !next[0].Since.Equal(lastTS) {
		t.Fatalf("tercera reclamación = %+v; quiero corte en %v", next, lastTS)
	}
}

// TestPurgeRuntimeLogsRetention comprueba que los logs CADUCAN solos: el cron de
// mantenimiento (cross-tenant, sin BYPASSRLS) borra lo anterior a la retención y
// no toca lo reciente.
func TestPurgeRuntimeLogsRetention(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	p := seedProject(t, ctx, st, "web")
	d := seedReadyDeployment(t, ctx, st, "acme", p.ID)

	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	old := now.Add(-store.RuntimeLogRetention - time.Hour)
	if _, err := st.AppendRuntimeLogs(context.Background(), "acme", p.ID, d.ID, []domain.LogLine{
		{TS: old, Text: "caducada 1"},
		{TS: old.Add(time.Second), Text: "caducada 2"},
		{TS: now.Add(-time.Hour), Text: "reciente"},
	}); err != nil {
		t.Fatalf("AppendRuntimeLogs: %v", err)
	}

	stats, err := st.PurgeExpired(context.Background(), now, store.KeepBuildsPerProject)
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if stats.RuntimeLogs != 2 {
		t.Errorf("purgadas %d líneas; quiero 2", stats.RuntimeLogs)
	}
	if n := maintCount(t, h.Pool, `SELECT count(*) FROM runtime_logs`); n != 1 {
		t.Errorf("quedan %d líneas; quiero 1 (la reciente)", n)
	}

	// Idempotente: una segunda pasada no borra nada más.
	stats, err = st.PurgeExpired(context.Background(), now, store.KeepBuildsPerProject)
	if err != nil {
		t.Fatalf("PurgeExpired (2ª): %v", err)
	}
	if stats.RuntimeLogs != 0 {
		t.Errorf("segunda purga borró %d líneas; quiero 0", stats.RuntimeLogs)
	}
}

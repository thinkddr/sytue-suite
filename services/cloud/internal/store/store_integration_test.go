// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de Sytue Cloud contra un PostgreSQL real (efímero, vía
// platform/pkg/pgtest sobre testcontainers-go). Detrás del build-tag
// `integration`: no corren en el build/CI por defecto (sin Docker). Verifican:
//   - PAGINACIÓN KEYSET de builds y deployments (orden estable por
//     (created_at, id) DESC, desempate por id, recorrido completo por cursor),
//   - PURGA/ROLLUP de retención (PurgeExpired): respeta retenciones, conserva
//     lo reciente y las K últimas builds por proyecto, rollup mensual de uso
//     idempotente,
//   - políticas RLS de mantenimiento de la migración 0002 (deny-by-default sin
//     flag, cross-tenant con `app.maintenance='on'`, incluido el INSERT del
//     rollup) con un rol de aplicación sin BYPASSRLS.
package store_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

func tenantCtx(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

// maintExec ejecuta SQL dentro de una transacción de mantenimiento (cross-
// tenant), para sembrar/backdatear filas de cualquier tenant en los tests.
func maintExec(t *testing.T, pool *pg.Pool, sql string, args ...any) {
	t.Helper()
	err := pool.InMaintenanceTx(context.Background(), func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), sql, args...)
		return err
	})
	if err != nil {
		t.Fatalf("maintExec(%q): %v", sql, err)
	}
}

// maintCount cuenta filas con una consulta cross-tenant de mantenimiento.
func maintCount(t *testing.T, pool *pg.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	err := pool.InMaintenanceTx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), sql, args...).Scan(&n)
	})
	if err != nil {
		t.Fatalf("maintCount(%q): %v", sql, err)
	}
	return n
}

func seedProject(t *testing.T, ctx context.Context, st *store.Store, slug string) domain.Project {
	t.Helper()
	p, err := st.CreateProject(ctx, store.NewProjectRecord{
		Slug: slug, Name: slug, GitProvider: domain.GitGeneric,
		ProductionRef: "main", Framework: domain.FrameworkDetect,
	}, 0)
	if err != nil {
		t.Fatalf("CreateProject(%q): %v", slug, err)
	}
	return p
}

func seedBuild(t *testing.T, ctx context.Context, st *store.Store, projectID uuid.UUID) domain.Build {
	t.Helper()
	b, err := st.EnqueueBuild(ctx, store.NewBuildRecord{ProjectID: projectID, GitRef: "main"})
	if err != nil {
		t.Fatalf("EnqueueBuild: %v", err)
	}
	return b
}

// pageBefore comprueba orden estricto (created_at, id) DESC entre filas
// consecutivas del recorrido keyset.
func pageBefore(prevAt time.Time, prevID uuid.UUID, at time.Time, id uuid.UUID) bool {
	if at.Before(prevAt) {
		return true
	}
	return at.Equal(prevAt) && bytes.Compare(id[:], prevID[:]) < 0
}

// TestListBuildsKeyset verifica que la paginación keyset de builds recorre
// TODAS las filas en orden (created_at, id) DESC estable — con EMPATES de
// created_at desempatados por id — sin repetir ni saltar, y que el cursor
// termina en nil.
func TestListBuildsKeyset(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	p := seedProject(t, ctx, st, "web")

	const total = 23
	seeded := map[uuid.UUID]bool{}
	for i := 0; i < total; i++ {
		seeded[seedBuild(t, ctx, st, p.ID).ID] = true
	}
	// Fuerza empates: 5 grupos de created_at idéntico (desempate por id).
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	maintExec(t, h.Pool, `
		WITH ranked AS (SELECT id, row_number() OVER (ORDER BY id) AS rn FROM builds)
		UPDATE builds b SET created_at = $1::timestamptz + make_interval(mins => (r.rn % 5)::int)
		FROM ranked r WHERE r.id = b.id`, base)

	var walked []domain.Build
	cursor := store.PageCursor{}
	pages := 0
	for {
		page, err := st.ListBuilds(ctx, p.ID, cursor, 7)
		if err != nil {
			t.Fatalf("ListBuilds(page %d): %v", pages, err)
		}
		pages++
		walked = append(walked, page.Builds...)
		if page.Next == nil {
			break
		}
		if len(page.Builds) != 7 {
			t.Fatalf("página intermedia con %d filas, quiero 7", len(page.Builds))
		}
		cursor = *page.Next
		if pages > total {
			t.Fatal("el cursor no termina (bucle)")
		}
	}
	if len(walked) != total || pages != 4 {
		t.Fatalf("recorrido=%d filas en %d páginas, quiero %d en 4", len(walked), pages, total)
	}
	seen := map[uuid.UUID]bool{}
	for i, b := range walked {
		if seen[b.ID] {
			t.Fatalf("build repetida en el recorrido: %s", b.ID)
		}
		seen[b.ID] = true
		if !seeded[b.ID] {
			t.Fatalf("build no sembrada en el recorrido: %s", b.ID)
		}
		if i > 0 && !pageBefore(walked[i-1].CreatedAt, walked[i-1].ID, b.CreatedAt, b.ID) {
			t.Fatalf("orden keyset roto en posición %d: (%v,%s) tras (%v,%s)",
				i, b.CreatedAt, b.ID, walked[i-1].CreatedAt, walked[i-1].ID)
		}
	}
}

// TestListDeploymentsKeyset verifica el recorrido keyset de deployments (orden,
// desempate por id, cobertura completa, fin de cursor).
func TestListDeploymentsKeyset(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	p := seedProject(t, ctx, st, "web")

	const total = 15
	seeded := map[uuid.UUID]bool{}
	for i := 0; i < total; i++ {
		d, err := st.CreateDeployment(ctx, store.NewDeploymentRecord{
			ProjectID: p.ID, Target: domain.TargetProduction, GitRef: "main",
			URL: fmt.Sprintf("https://web-%d.apps.sytue.eu", i),
		})
		if err != nil {
			t.Fatalf("CreateDeployment(%d): %v", i, err)
		}
		seeded[d.ID] = true
	}
	// Fuerza empates: 3 grupos de created_at idéntico.
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	maintExec(t, h.Pool, `
		WITH ranked AS (SELECT id, row_number() OVER (ORDER BY id) AS rn FROM deployments)
		UPDATE deployments d SET created_at = $1::timestamptz + make_interval(mins => (r.rn % 3)::int)
		FROM ranked r WHERE r.id = d.id`, base)

	var walked []domain.Deployment
	cursor := store.PageCursor{}
	pages := 0
	for {
		page, err := st.ListDeployments(ctx, p.ID, cursor, 4)
		if err != nil {
			t.Fatalf("ListDeployments(page %d): %v", pages, err)
		}
		pages++
		walked = append(walked, page.Deployments...)
		if page.Next == nil {
			break
		}
		cursor = *page.Next
		if pages > total {
			t.Fatal("el cursor no termina (bucle)")
		}
	}
	if len(walked) != total || pages != 4 {
		t.Fatalf("recorrido=%d filas en %d páginas, quiero %d en 4", len(walked), pages, total)
	}
	seen := map[uuid.UUID]bool{}
	for i, d := range walked {
		if seen[d.ID] || !seeded[d.ID] {
			t.Fatalf("recorrido con repetidos o filas ajenas: %s", d.ID)
		}
		seen[d.ID] = true
		if i > 0 && !pageBefore(walked[i-1].CreatedAt, walked[i-1].ID, d.CreatedAt, d.ID) {
			t.Fatalf("orden keyset roto en posición %d", i)
		}
	}
}

// finishOK marca una build como succeeded (terminal).
func finishOK(t *testing.T, st *store.Store, tenant string, id uuid.UUID) {
	t.Helper()
	if err := st.FinishBuild(context.Background(), tenant, id, store.BuildResult{Status: domain.BuildSucceeded}); err != nil {
		t.Fatalf("FinishBuild(%s): %v", id, err)
	}
}

// TestPurgeExpired verifica la pasada de purga/rollup completa: respeta las
// retenciones (no borra lo reciente ni lo no terminal), conserva las K últimas
// builds por proyecto, purga webhooks/outbox/previews caducados y agrega el
// detalle de uso antiguo en filas mensuales sin alterar los totales. La segunda
// pasada es un no-op (idempotencia).
func TestPurgeExpired(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	now := time.Now().UTC()
	old := now.Add(-40 * 24 * time.Hour) // fuera de las retenciones de 30/7 días
	recent := now.Add(-24 * time.Hour)   // dentro de todas las retenciones
	backdate := func(table string, at time.Time, col string, ids []uuid.UUID) {
		maintExec(t, h.Pool, `UPDATE `+table+` SET `+col+`=$1 WHERE id = ANY($2)`, at, ids)
	}

	pa := seedProject(t, ctx, st, "alpha")
	pb := seedProject(t, ctx, st, "beta")

	// --- Builds. Proyecto alpha: 6 terminales viejas (purgables), 2 queued
	// viejas (no terminales: se conservan), 2 terminales recientes (se
	// conservan y son las 2 últimas del proyecto → keepBuilds=2 las cubre).
	var oldTerm, oldQueued, recentTerm []uuid.UUID
	for i := 0; i < 6; i++ {
		b := seedBuild(t, ctx, st, pa.ID)
		finishOK(t, st, "acme", b.ID)
		oldTerm = append(oldTerm, b.ID)
	}
	for i := 0; i < 2; i++ {
		oldQueued = append(oldQueued, seedBuild(t, ctx, st, pa.ID).ID)
	}
	for i := 0; i < 2; i++ {
		b := seedBuild(t, ctx, st, pa.ID)
		finishOK(t, st, "acme", b.ID)
		recentTerm = append(recentTerm, b.ID)
	}
	backdate("builds", old, "created_at", oldTerm)
	backdate("builds", old.Add(-time.Hour), "created_at", oldQueued)

	// Proyecto beta: 5 terminales viejas; keepBuilds=2 conserva las 2 más
	// recientes aunque estén caducadas (objetivos de rollback).
	var betaBuilds []uuid.UUID
	for i := 0; i < 5; i++ {
		b := seedBuild(t, ctx, st, pb.ID)
		finishOK(t, st, "acme", b.ID)
		betaBuilds = append(betaBuilds, b.ID)
		backdate("builds", old.Add(time.Duration(i)*time.Minute), "created_at", []uuid.UUID{b.ID})
	}

	// --- Webhooks: uno caducado, uno reciente.
	if _, err := st.RecordWebhook(ctx, "acme", pa.ID, "github", "d-old", "push", nil); err != nil {
		t.Fatalf("RecordWebhook: %v", err)
	}
	if _, err := st.RecordWebhook(ctx, "acme", pa.ID, "github", "d-new", "push", nil); err != nil {
		t.Fatalf("RecordWebhook: %v", err)
	}
	maintExec(t, h.Pool, `UPDATE git_webhooks SET received_at=$1 WHERE delivery_id='d-old'`, old)

	// --- Outbox (sin RLS): procesado viejo (purga), procesado reciente y no
	// procesado viejo (se conservan).
	for _, row := range []struct {
		id        uuid.UUID
		processed *time.Time
	}{
		{uuid.New(), &old},
		{uuid.New(), &recent},
		{uuid.New(), nil},
	} {
		if _, err := h.Pool.Exec(context.Background(), `
			INSERT INTO events_outbox (id, channel, type, tenant, created_at, processed_at)
			VALUES ($1, 'cloud', 'test', 'acme', $2, $3)`, row.id, old, row.processed); err != nil {
			t.Fatalf("sembrar outbox: %v", err)
		}
	}

	// --- Previews: retirada vieja (purga), retirada reciente, ready vieja y
	// producción retirada vieja (se conservan).
	mkDep := func(target domain.DeployTarget, pr *int, status domain.DeployStatus, updatedAt time.Time) uuid.UUID {
		t.Helper()
		d, err := st.CreateDeployment(ctx, store.NewDeploymentRecord{
			ProjectID: pa.ID, Target: target, GitRef: "main", PRNumber: pr, URL: "https://x",
		})
		if err != nil {
			t.Fatalf("CreateDeployment: %v", err)
		}
		if err := st.SetDeploymentStatus(context.Background(), "acme", d.ID, status, "", ""); err != nil {
			t.Fatalf("SetDeploymentStatus: %v", err)
		}
		backdate("deployments", updatedAt, "updated_at", []uuid.UUID{d.ID})
		return d.ID
	}
	pr1, pr2, pr3 := 1, 2, 3
	purgedPrev := mkDep(domain.TargetPreview, &pr1, domain.DeployRetired, old)
	keptPrevRecent := mkDep(domain.TargetPreview, &pr2, domain.DeployRetired, recent)
	keptPrevReady := mkDep(domain.TargetPreview, &pr3, domain.DeployReady, old)
	keptProdRetired := mkDep(domain.TargetProduction, nil, domain.DeployRetired, old)

	// --- Uso: detalle de enero (>90d) en dos métricas → 2 rollups mensuales;
	// un detalle reciente se conserva tal cual.
	jan := func(day int) time.Time { return time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC) }
	usage := []store.UsageInput{
		{Tenant: "acme", ProjectID: &pa.ID, Metric: "build_minutes", Quantity: 1, Unit: "min", PeriodFrom: jan(3), PeriodTo: jan(4)},
		{Tenant: "acme", ProjectID: &pa.ID, Metric: "build_minutes", Quantity: 2, Unit: "min", PeriodFrom: jan(10), PeriodTo: jan(11)},
		{Tenant: "acme", ProjectID: &pa.ID, Metric: "build_minutes", Quantity: 3, Unit: "min", PeriodFrom: jan(20), PeriodTo: jan(21)},
		{Tenant: "acme", ProjectID: &pa.ID, Metric: "egress_gb", Quantity: 5, Unit: "GB", PeriodFrom: jan(5), PeriodTo: jan(6)},
		{Tenant: "acme", ProjectID: &pa.ID, Metric: "build_minutes", Quantity: 7, Unit: "min", PeriodFrom: now.Add(-48 * time.Hour), PeriodTo: recent},
	}
	for i, u := range usage {
		if err := st.RecordUsage(context.Background(), u); err != nil {
			t.Fatalf("RecordUsage(%d): %v", i, err)
		}
	}

	// --- Primera pasada.
	stats, err := st.PurgeExpired(context.Background(), now, 2)
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	want := store.PurgeStats{Builds: 9, Webhooks: 1, OutboxEvents: 1, Previews: 1, UsageDetail: 4, UsageRollups: 2}
	if stats != want {
		t.Fatalf("stats=%+v, quiero %+v", stats, want)
	}

	// Builds restantes de alpha: 2 queued viejas + 2 terminales recientes.
	remainA := map[uuid.UUID]bool{}
	pageA, err := st.ListBuilds(ctx, pa.ID, store.PageCursor{}, 50)
	if err != nil {
		t.Fatalf("ListBuilds(alpha): %v", err)
	}
	for _, b := range pageA.Builds {
		remainA[b.ID] = true
	}
	if len(remainA) != 4 {
		t.Fatalf("alpha conserva %d builds, quiero 4", len(remainA))
	}
	for _, id := range append(oldQueued, recentTerm...) {
		if !remainA[id] {
			t.Fatalf("build %s debía conservarse (no terminal o reciente)", id)
		}
	}
	// Beta conserva exactamente sus 2 más recientes (keepBuilds).
	pageB, err := st.ListBuilds(ctx, pb.ID, store.PageCursor{}, 50)
	if err != nil {
		t.Fatalf("ListBuilds(beta): %v", err)
	}
	if len(pageB.Builds) != 2 {
		t.Fatalf("beta conserva %d builds, quiero 2 (keepBuilds)", len(pageB.Builds))
	}
	keep := map[uuid.UUID]bool{betaBuilds[4]: true, betaBuilds[3]: true}
	for _, b := range pageB.Builds {
		if !keep[b.ID] {
			t.Fatalf("beta conserva %s, quería las 2 últimas %v", b.ID, betaBuilds[3:])
		}
	}

	if n := maintCount(t, h.Pool, `SELECT count(*) FROM git_webhooks`); n != 1 {
		t.Fatalf("git_webhooks=%d, quiero 1 (solo d-new)", n)
	}
	// 2 sembrados que sobreviven (procesado reciente + no procesado viejo) + 1
	// aviso cloud.deploy.ready que emite keptPrevReady al pasar a ready (outbox
	// transaccional; no procesado, así que la purga no lo toca).
	if n := maintCount(t, h.Pool, `SELECT count(*) FROM events_outbox`); n != 3 {
		t.Fatalf("events_outbox=%d, quiero 3", n)
	}
	for _, tc := range []struct {
		id   uuid.UUID
		want int
		name string
	}{
		{purgedPrev, 0, "preview retirada vieja"},
		{keptPrevRecent, 1, "preview retirada reciente"},
		{keptPrevReady, 1, "preview ready"},
		{keptProdRetired, 1, "producción retirada"},
	} {
		if n := maintCount(t, h.Pool, `SELECT count(*) FROM deployments WHERE id=$1`, tc.id); n != tc.want {
			t.Fatalf("%s: filas=%d, quiero %d", tc.name, n, tc.want)
		}
	}

	// Rollup: totales intactos y fila mensual alineada al mes natural UTC.
	sum, err := st.SummarizeUsage(ctx, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("SummarizeUsage: %v", err)
	}
	totals := map[string]float64{}
	for _, a := range sum {
		totals[a.Metric] = a.Total
	}
	if totals["build_minutes"] != 13 || totals["egress_gb"] != 5 {
		t.Fatalf("totales tras rollup=%v, quiero build_minutes=13 egress_gb=5", totals)
	}
	if n := maintCount(t, h.Pool, `
		SELECT count(*) FROM usage_records
		WHERE metric='build_minutes' AND quantity=6
		  AND period_from=$1 AND period_to=$2`,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)); n != 1 {
		t.Fatalf("fila mensual build_minutes=6 de enero: %d, quiero 1", n)
	}
	if n := maintCount(t, h.Pool, `SELECT count(*) FROM usage_records`); n != 3 {
		t.Fatalf("usage_records=%d, quiero 3 (2 mensuales + 1 detalle reciente)", n)
	}

	// --- Segunda pasada: no-op (idempotencia; el rollup mensual no se re-agrega).
	again, err := st.PurgeExpired(context.Background(), now, 2)
	if err != nil {
		t.Fatalf("PurgeExpired (2ª): %v", err)
	}
	if again != (store.PurgeStats{}) {
		t.Fatalf("segunda pasada no idempotente: %+v", again)
	}
	if n := maintCount(t, h.Pool, `SELECT count(*) FROM usage_records`); n != 3 {
		t.Fatalf("usage_records tras 2ª pasada=%d, quiero 3", n)
	}
}

// TestMaintenancePoliciesRLS verifica con un rol de aplicación SIN BYPASSRLS
// que (1) sin app.tenant ni app.maintenance no se ve ninguna fila (deny-by-
// default), (2) con app.maintenance='on' la política PERMISSIVE de la
// migración 0002 habilita lectura cross-tenant y el INSERT del rollup, y (3)
// sin el flag ese INSERT viola RLS.
//
// El rol SIN BYPASSRLS ya lo aporta el harness (pgtest conecta el Pool como
// `sytue_app`, NOSUPERUSER + NOBYPASSRLS), así que este test dejó de crearse el
// suyo a mano con `CREATE ROLE cloud_app` + `SET LOCAL ROLE`: ese andamio existía
// solo porque el Pool era superusuario y ninguna política se evaluaba. Ahora
// ejercita EL MISMO rol que el resto de la suite.
func TestMaintenancePoliciesRLS(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	p := seedProject(t, ctx, st, "web")
	seedBuild(t, ctx, st, p.ID)

	bg := context.Background()

	errRollback := errors.New("rollback intencional")
	insertUsage := `INSERT INTO usage_records (tenant, metric, quantity, unit, period_from, period_to)
		VALUES ('acme', 'm', 1, 'u', now(), now())`

	err := h.Pool.InTx(bg, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(bg, `SELECT count(*) FROM builds`).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("sin tenant ni maintenance se ven %d builds, quiero 0 (deny-by-default)", n)
		}
		// projects es la tabla que JOINan ListReadyProductions/ListPendingDeployments
		// del reconciliador; sin su política de mantenimiento (0003) el JOIN daría 0
		// filas incluso bajo app.maintenance='on' (blocker Ola A #1).
		if err := tx.QueryRow(bg, `SELECT count(*) FROM projects`).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("sin maintenance se ven %d projects, quiero 0 (deny-by-default)", n)
		}
		if _, err := tx.Exec(bg, `SELECT set_config('app.maintenance', 'on', true)`); err != nil {
			return err
		}
		if err := tx.QueryRow(bg, `SELECT count(*) FROM builds`).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("con maintenance se ven %d builds, quiero 1", n)
		}
		if err := tx.QueryRow(bg, `SELECT count(*) FROM projects`).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("con maintenance se ven %d projects, quiero 1 (política 0003)", n)
		}
		if _, err := tx.Exec(bg, insertUsage); err != nil {
			t.Errorf("INSERT de rollup bajo maintenance rechazado: %v", err)
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("tx de mantenimiento: %v", err)
	}

	err = h.Pool.InTx(bg, func(tx pgx.Tx) error {
		if _, err := tx.Exec(bg, insertUsage); err == nil {
			t.Error("INSERT cross-tenant sin maintenance debería violar RLS")
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("tx sin maintenance: %v", err)
	}
}

// TestDeleteProjectRevokesConfinedTokens comprueba el guarantee de 0017: un token
// confinado a un proyecto "muere con su proyecto". Como DeleteProject es soft-delete
// (el ON DELETE CASCADE de la FK no se dispara), la revocación la hace un DELETE
// explícito en la tx. Un token tenant-wide (project_id NULL) NO debe verse afectado.
func TestDeleteProjectRevokesConfinedTokens(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	p := seedProject(t, ctx, st, "web")

	confined, err := st.CreateAPIToken(ctx, store.NewAPITokenRecord{
		Name: "ci", TokenHash: "hash-confined", Scope: domain.ScopeDeploy,
		CreatedBy: "u1", ProjectID: &p.ID,
	})
	if err != nil {
		t.Fatalf("CreateAPIToken confinado: %v", err)
	}
	wide, err := st.CreateAPIToken(ctx, store.NewAPITokenRecord{
		Name: "wide", TokenHash: "hash-wide", Scope: domain.ScopeDeploy, CreatedBy: "u1",
	})
	if err != nil {
		t.Fatalf("CreateAPIToken tenant-wide: %v", err)
	}

	if err := st.DeleteProject(ctx, p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	if _, err := st.ResolveAPIToken(context.Background(), "hash-confined"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("token confinado %s resoluble tras borrar su proyecto (quiero ErrNotFound): %v", confined.ID, err)
	}
	if _, err := st.ResolveAPIToken(context.Background(), "hash-wide"); err != nil {
		t.Errorf("token tenant-wide %s revocado por error al borrar un proyecto: %v", wide.ID, err)
	}
}

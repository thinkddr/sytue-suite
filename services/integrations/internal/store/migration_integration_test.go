// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration
// +build integration

// Tests de integracion de la MIGRACION DE CONTENIDO contra un PostgreSQL real y
// efimero (platform/pkg/pgtest). Detras del build-tag `integration`. Verifican:
//   - que 0005/0006 aplican con RLS ENABLE+FORCE en migration_jobs, migration_map
//     y migration_delegates,
//   - AISLAMIENTO por org: org B no ve, ni cancela, ni contamina el trabajo de A,
//     ni puede leer el client delegable de A (seria impersonar en otro tenant),
//   - que el secreto del client delegable se sella (jamas viaja en claro),
//   - idempotencia del mapa (upsert por (job, source_id)) y el informe de omitidos.
package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func TestMigrationTablesRLS(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	ctx := context.Background()

	for _, tbl := range []string{"migration_jobs", "migration_map", "migration_delegates"} {
		var enable, force bool
		if err := h.Pool.QueryRow(ctx,
			`SELECT relrowsecurity, relforcerowsecurity FROM pg_class WHERE relname = $1 AND relkind = 'r'`, tbl).
			Scan(&enable, &force); err != nil {
			t.Fatalf("tabla %q no encontrada tras migrar: %v", tbl, err)
		}
		if !enable || !force {
			t.Fatalf("tabla %q sin RLS ENABLE+FORCE (enable=%v force=%v)", tbl, enable, force)
		}
	}
}

// newJob crea instancia + trabajo de migracion en la org dada.
func newJob(ctx context.Context, t *testing.T, st *store.Store, org string) domain.MigrationJob {
	t.Helper()
	inst, err := st.CreateInstance(ctx, "microsoft365", "M365 de "+org, nil, nil)
	if err != nil {
		t.Fatalf("%s CreateInstance: %v", org, err)
	}
	job, err := st.CreateMigrationJob(ctx, store.NewMigrationJob{
		InstanceID: inst.ID,
		TargetUser: "grace@" + org + ".test",
		SourceUser: "alice@old.example",
		Source:     domain.SourceM365,
		Domain:     domain.DomainDrive,
	})
	if err != nil {
		t.Fatalf("%s CreateMigrationJob: %v", org, err)
	}
	return job
}

func TestMigrationJobOrgIsolation(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	seedTemplate(t, h, "microsoft365")

	ctxA, ctxB := tenantCtx("org-a"), tenantCtx("org-b")
	jobA := newJob(ctxA, t, st, "org-a")

	// A ve su trabajo; B no (RLS deny-by-default), ni por id.
	if _, err := st.GetMigrationJob(ctxA, jobA.ID); err != nil {
		t.Fatalf("A no ve su propio trabajo: %v", err)
	}
	if _, err := st.GetMigrationJob(ctxB, jobA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B lee el trabajo de A (err=%v)", err)
	}
	if jobs, err := st.ListMigrationJobs(ctxB, 50); err != nil || len(jobs) != 0 {
		t.Fatalf("aislamiento roto: B lista %d trabajos de A (err=%v)", len(jobs), err)
	}
	// Ni puede cancelarlo, ni reanudarlo, ni abrir su token.
	if err := st.CancelMigrationJob(ctxB, jobA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B cancela el trabajo de A (err=%v)", err)
	}
	if err := st.ResumeMigrationJob(ctxB, jobA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B reanuda el trabajo de A (err=%v)", err)
	}
	// A conserva su trabajo intacto y encolado.
	got, err := st.GetMigrationJob(ctxA, jobA.ID)
	if err != nil || got.Status != domain.MigrationQueued {
		t.Fatalf("el trabajo de A cambio: %+v (err=%v)", got, err)
	}
}

// TestDelegatedClientSealedYAisladoPorOrg: el secreto del client DELEGABLE (con el
// que se canjean tokens en nombre de cualquier miembro de la org) se guarda sellado
// y es invisible para otra organizacion. Filtrarlo seria entregar impersonacion de
// todo un tenant, asi que el aislamiento lo impone RLS, no la aplicacion.
func TestDelegatedClientSealedYAisladoPorOrg(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))

	ctxA, ctxB := tenantCtx("org-a"), tenantCtx("org-b")
	if err := st.SaveDelegatedClient(ctxA, store.Delegate{
		ClientID: "sytue-migrate-org-a", Secret: "secreto-de-org-a",
	}); err != nil {
		t.Fatalf("SaveDelegatedClient: %v", err)
	}

	// El valor almacenado NO es el secreto en claro. La lectura cruda va DENTRO de
	// InTenantTx porque el Pool del harness ya no tiene BYPASSRLS: sin el GUC
	// `app.tenant` la política esconde la fila y esto fallaría con "no rows". Sigue
	// siendo SQL crudo (no pasa por el store), que es lo que da valor a la
	// comprobación: lee la columna tal cual está en disco.
	var stored string
	if err := h.Pool.InTenantTx(ctxA, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT client_secret_enc FROM migration_delegates WHERE org_id = 'org-a'`).Scan(&stored)
	}); err != nil {
		t.Fatalf("leer secreto sellado: %v", err)
	}
	if strings.Contains(stored, "secreto-de-org-a") {
		t.Fatal("el secreto del client delegable se guardo en claro")
	}

	// A lo abre; B ni siquiera ve que exista.
	got, err := st.DelegatedClient(ctxA)
	if err != nil {
		t.Fatalf("DelegatedClient(A): %v", err)
	}
	if got.ClientID != "sytue-migrate-org-a" || got.Secret != "secreto-de-org-a" {
		t.Fatalf("credenciales descifradas = %+v", got)
	}
	if _, err := st.DelegatedClient(ctxB); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B lee el client delegable de A (err=%v)", err)
	}
	// Y B no puede pisar la fila de A (RLS WITH CHECK acota lo que escribe).
	if err := st.SaveDelegatedClient(ctxB, store.Delegate{
		ClientID: "impostor", Secret: "otro",
	}); err != nil {
		t.Fatalf("SaveDelegatedClient(B): %v", err)
	}
	again, err := st.DelegatedClient(ctxA)
	if err != nil || again.ClientID != "sytue-migrate-org-a" {
		t.Fatalf("aislamiento roto: B piso el client delegable de A (%+v, err=%v)", again, err)
	}
}

func TestMigrationMapIdempotenteYInforme(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	seedTemplate(t, h, "microsoft365")

	ctxA, ctxB := tenantCtx("org-a"), tenantCtx("org-b")
	jobA := newJob(ctxA, t, st, "org-a")
	if err := st.StartMigrationJob(ctxA, jobA.ID); err != nil {
		t.Fatalf("StartMigrationJob: %v", err)
	}

	items := []domain.MigrationItem{
		{SourceID: "f-1", Kind: domain.KindFile, SytueID: "sy-1", Path: "/a.txt", Hash: "h1"},
		{SourceID: "g-doc", Kind: domain.KindFile, Path: "/Acta", SkipReason: "documento nativo de Google: requiere exportacion"},
	}
	if err := st.RecordMigrationItems(ctxA, jobA.ID, items); err != nil {
		t.Fatalf("RecordMigrationItems: %v", err)
	}
	// Reescribir el MISMO lote no duplica (PK job+source_id).
	if err := st.RecordMigrationItems(ctxA, jobA.ID, items); err != nil {
		t.Fatalf("RecordMigrationItems (2.a vez): %v", err)
	}
	mapped, err := st.LoadMigrationMap(ctxA, jobA.ID)
	if err != nil {
		t.Fatalf("LoadMigrationMap: %v", err)
	}
	if len(mapped) != 2 {
		t.Fatalf("el mapa duplico entradas: %d filas", len(mapped))
	}

	// El informe solo trae lo OMITIDO, con su motivo.
	skipped, err := st.ListMigrationSkipped(ctxA, jobA.ID, 50)
	if err != nil {
		t.Fatalf("ListMigrationSkipped: %v", err)
	}
	if len(skipped) != 1 || skipped[0].SourceID != "g-doc" || skipped[0].SkipReason == "" {
		t.Fatalf("informe de omitidos incorrecto: %+v", skipped)
	}
	if skipped[0].SytueID != "" {
		t.Fatal("una entidad omitida no puede llevar id en Sytue")
	}

	// B no ve el mapa de A ni puede escribir en el (RLS).
	if got, err := st.LoadMigrationMap(ctxB, jobA.ID); err != nil || len(got) != 0 {
		t.Fatalf("aislamiento roto: B ve %d filas del mapa de A (err=%v)", len(got), err)
	}
	if err := st.RecordMigrationItems(ctxB, jobA.ID, items); err == nil {
		if got, _ := st.LoadMigrationMap(ctxA, jobA.ID); len(got) != 2 {
			t.Fatalf("aislamiento roto: B contamino el mapa de A (%d filas)", len(got))
		}
	}

	// El progreso avanza y devuelve el estado vivo; una cancelacion se ve al vuelo.
	st1, err := st.SaveMigrationProgress(ctxA, jobA.ID, `{"queue":[]}`, domain.MigrationCounts{Migrated: 1, Skipped: 1})
	if err != nil || st1 != domain.MigrationRunning {
		t.Fatalf("SaveMigrationProgress = %q (err=%v)", st1, err)
	}
	if err := st.CancelMigrationJob(ctxA, jobA.ID); err != nil {
		t.Fatalf("CancelMigrationJob: %v", err)
	}
	st2, err := st.SaveMigrationProgress(ctxA, jobA.ID, "", domain.MigrationCounts{})
	if err != nil || st2 != domain.MigrationCanceled {
		t.Fatalf("tras cancelar, SaveMigrationProgress = %q (err=%v)", st2, err)
	}
	// La cancelacion no borro el progreso ya guardado (se puede reanudar).
	after, err := st.GetMigrationJob(ctxA, jobA.ID)
	if err != nil {
		t.Fatalf("GetMigrationJob: %v", err)
	}
	if after.Counts.Migrated != 1 || after.Counts.Skipped != 1 {
		t.Fatalf("la cancelacion piso los contadores: %+v", after.Counts)
	}
}

// TestLaColaDeMigracionesTieneTope (NEG-20): el mismo trabajo vivo no se encola
// dos veces, y una organización no pasa de MaxMigracionesVivas; la otra sigue
// pudiendo encolar.
func TestLaColaDeMigracionesTieneTope(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	seedTemplate(t, h, "microsoft365")
	ctxA := tenantCtx("org-a")
	job := newJob(ctxA, t, st, "org-a")

	repetido := store.NewMigrationJob{InstanceID: job.InstanceID, TargetUser: "grace@org-a.test", SourceUser: "alice@old.example", Source: domain.SourceM365, Domain: domain.DomainDrive}
	if _, err := st.CreateMigrationJob(ctxA, repetido); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("el mismo trabajo vivo otra vez: %v, quiero ErrConflict", err)
	}
	if err := h.Pool.InTenantTx(ctxA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctxA, `
			INSERT INTO migration_jobs (org_id, instance_id, target_user, source_user, source, domain, direction)
			SELECT 'org-a', $1, 'p' || g || '@org-a.test', '', 'm365', 'drive', 'in' FROM generate_series(2, $2::int) g`,
			job.InstanceID, store.MaxMigracionesVivas)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	otro := repetido
	otro.TargetUser = "nuevo@org-a.test"
	if _, err := st.CreateMigrationJob(ctxA, otro); !errors.Is(err, store.ErrDemasiadasMigraciones) {
		t.Fatalf("pasado el tope: %v, quiero ErrDemasiadasMigraciones", err)
	}
	newJob(tenantCtx("org-b"), t, st, "org-b")
}

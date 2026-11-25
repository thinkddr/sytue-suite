// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration
// +build integration

package store_test

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// selfFixture monta dos conexiones de la MISMA cuenta de Google en org-a y devuelve
// el alta de una importación de correo de Ana con la primera.
func selfFixture(t *testing.T) (*store.Store, *pgtest.Harness, context.Context, store.SelfInbound, string) {
	t.Helper()
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	seedTemplate(t, h, "google-workspace")
	ctx := tenantCtx("org-a")
	a, err := st.CreateInstance(ctx, "google-workspace", "Google", []byte(`{"domain":"acme.com"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateInstance(ctx, "google-workspace", "Google (otra vez)", []byte(`{"domain":"acme.com"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	in := store.SelfInbound{
		Job: store.NewMigrationJob{
			InstanceID: a.ID, TargetUser: "sub-ana", SourceUser: "ana@acme.com",
			Source: domain.SourceGoogle, Domain: domain.DomainMail, Direction: domain.DirectionIn,
		},
		Email:       "ana@acme.com",
		SameAccount: []string{a.ID, b.ID},
	}
	return st, h, ctx, in, b.ID
}

func contarTrabajos(ctx context.Context, t *testing.T, h *pgtest.Harness) int {
	t.Helper()
	var n int
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM migration_jobs`).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestLanzarImportacionPropiaNoDuplica es el corazón del arreglo: lanzar dos veces la
// misma cuenta y el mismo contenido —con la misma conexión o con otra de la misma
// cuenta— no crea un segundo trabajo, que copiaría el buzón entero otra vez.
func TestLanzarImportacionPropiaNoDuplica(t *testing.T) {
	st, h, ctx, in, otra := selfFixture(t)

	primero, err := st.LaunchSelfInboundJob(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	// En cola: lanzar otra vez, ahora con la otra conexión, devuelve el mismo.
	conOtra := in
	conOtra.Job.InstanceID = otra
	segundo, err := st.LaunchSelfInboundJob(ctx, conOtra)
	if err != nil || segundo.ID != primero.ID || segundo.Status != domain.MigrationQueued {
		t.Fatalf("relanzar en cola: %+v, %v; quiero el mismo trabajo %s", segundo, err, primero.ID)
	}
	if n := contarTrabajos(ctx, t, h); n != 1 {
		t.Fatalf("hay %d trabajos, quiero 1", n)
	}

	// Falló a medias: relanzar SIGUE donde se quedó (cursor y contadores intactos) y
	// pasa a la conexión elegida.
	if err := st.StartMigrationJob(ctx, primero.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveMigrationProgress(ctx, primero.ID, "pagina-7", domain.MigrationCounts{Migrated: 40, Skipped: 2}); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishMigrationJob(ctx, primero.ID, domain.MigrationError, "fallo"); err != nil {
		t.Fatal(err)
	}
	reintento, err := st.LaunchSelfInboundJob(ctx, conOtra)
	if err != nil || reintento.ID != primero.ID {
		t.Fatalf("reintentar: %+v, %v", reintento, err)
	}
	job, err := st.GetMigrationJob(ctx, primero.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != domain.MigrationQueued || job.Cursor != "pagina-7" || job.Counts.Migrated != 40 ||
		job.InstanceID != otra || job.LastError != "" {
		t.Fatalf("tras reintentar: %+v; quiero en cola, con su cursor y sus cuentas, y con la otra conexión", job)
	}

	// Terminado: relanzar trae lo nuevo desde el principio, con el mismo mapa.
	if err := st.StartMigrationJob(ctx, primero.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishMigrationJob(ctx, primero.ID, domain.MigrationDone, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.LaunchSelfInboundJob(ctx, in); err != nil {
		t.Fatal(err)
	}
	job, _ = st.GetMigrationJob(ctx, primero.ID)
	if job.Status != domain.MigrationQueued || job.Cursor != "" || job.Counts.Migrated != 40 {
		t.Fatalf("tras relanzar uno terminado: %+v; quiero en cola desde el principio y con lo ya traído", job)
	}
	if n := contarTrabajos(ctx, t, h); n != 1 {
		t.Fatalf("hay %d trabajos, quiero 1", n)
	}

	// Otro contenido sí es otro trabajo.
	contactos := in
	contactos.Job.Domain = domain.DomainContacts
	if c, err := st.LaunchSelfInboundJob(ctx, contactos); err != nil || c.ID == primero.ID {
		t.Fatalf("contactos: %+v, %v; quiero un trabajo nuevo", c, err)
	}
}

// TestLanzarImportacionPropiaReutilizaLaDelAdministrador: si quien administra ya trajo
// el correo de Ana con el asistente automático (la nombra por su correo de Sytue), que
// Ana pulse «Traerlo de Google» no lo vuelve a copiar entero; y lo ve como suyo.
func TestLanzarImportacionPropiaReutilizaLaDelAdministrador(t *testing.T) {
	st, h, ctx, in, _ := selfFixture(t)
	admin, err := st.CreateMigrationJob(ctx, store.NewMigrationJob{
		InstanceID: in.Job.InstanceID, TargetUser: "ana@acme.com", SourceUser: "Ana@Acme.com",
		Source: domain.SourceGoogle, Domain: domain.DomainMail,
	})
	if err != nil {
		t.Fatal(err)
	}
	propia, err := st.LaunchSelfInboundJob(ctx, in)
	if err != nil || propia.ID != admin.ID {
		t.Fatalf("debía reutilizar el trabajo del administrador %s: %+v, %v", admin.ID, propia, err)
	}
	if n := contarTrabajos(ctx, t, h); n != 1 {
		t.Fatalf("hay %d trabajos, quiero 1", n)
	}
	mios, err := st.ListMigrationJobsForTarget(ctx, []string{"sub-ana", "ana@acme.com"}, 50)
	if err != nil || len(mios) != 1 || mios[0].ID != admin.ID {
		t.Fatalf("Ana debía ver la importación que le lanzaron: %+v, %v", mios, err)
	}

	// La de OTRA persona no se reutiliza nunca, aunque sea la misma cuenta del origen.
	otra := in
	otra.Job.TargetUser, otra.Email = "sub-luis", "luis@acme.com"
	if j, err := st.LaunchSelfInboundJob(ctx, otra); err != nil || j.ID == admin.ID {
		t.Fatalf("Luis no puede heredar el trabajo de Ana: %+v, %v", j, err)
	}
}

// TestLanzarImportacionPropiaALaVez: dos lanzamientos simultáneos (dos clics, la web y
// la app nativa) dejan UN trabajo. Sin el cerrojo, los dos veían «no hay» y creaban uno.
func TestLanzarImportacionPropiaALaVez(t *testing.T) {
	st, h, ctx, in, otra := selfFixture(t)
	var wg sync.WaitGroup
	ids := make([]string, 8)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			alta := in
			if i%2 == 1 {
				alta.Job.InstanceID = otra
			}
			j, err := st.LaunchSelfInboundJob(ctx, alta)
			if err != nil {
				t.Error(err)
				return
			}
			ids[i] = j.ID
		}(i)
	}
	wg.Wait()
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("lanzamientos simultáneos crearon trabajos distintos: %v", ids)
		}
	}
	if n := contarTrabajos(ctx, t, h); n != 1 {
		t.Fatalf("hay %d trabajos, quiero 1", n)
	}
}

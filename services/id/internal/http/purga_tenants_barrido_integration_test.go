// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/purgatenant"
	drivedb "git.sytue.eu/sytue/one/services/drive/db"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// TestBarrerPurgasDeTenant: de la cola al cierre de la purga, con dos servicios
// reales (uno con objetos) y los demás ausentes del despliegue. Lo que se
// comprueba es el ciclo en la cola de Sytue ID: espera a los objetos, se cierra
// cuando el servicio los borra y lo deja en la auditoría.
func TestBarrerPurgasDeTenant(t *testing.T) {
	l := newLiveServer(t)
	ctx := l.ctx
	_, org, err := l.srv.crearCuentaPersonal(ctx, domain.User{Email: "eva-purga@id.test", EmailVerified: true, Name: "Eva"})
	if err != nil {
		t.Fatal(err)
	}
	hub := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	drive := pgtest.New(t, drivedb.Migrations, drivedb.MigrationsDir)
	for svc, h := range map[string]*pgtest.Harness{"hub": hub, "drive": drive} {
		for _, q := range esquemasPurga[svc].siembra {
			if _, err := h.PoolSinRLS(t).Exec(ctx, q, org.Slug); err != nil {
				t.Fatalf("sembrar %s: %v", svc, err)
			}
		}
	}
	l.srv.basesDeServicio = func(_ context.Context, svc string) (pg.Database, func(), error) {
		switch svc {
		case "hub":
			return hub.Pool, func() {}, nil
		case "drive":
			return drive.Pool, func() {}, nil
		}
		return nil, nil, &pgconn.PgError{Code: "3D000", Message: "database does not exist"}
	}
	if err := l.st.EncolarPurgaDeTenant(ctx, nil, org.ID, store.PurgaCuentaBorrada); err != nil {
		t.Fatal(err)
	}
	// Encolar dos veces no duplica ni cambia el motivo.
	if err := l.st.EncolarPurgaDeTenant(ctx, nil, org.ID, store.PurgaSupresionRGPD); err != nil {
		t.Fatal(err)
	}
	if err := l.srv.BarrerPurgasDeTenant(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := l.st.PurgaDeTenantDe(ctx, org.ID)
	if err != nil || p == nil || p.Completada != nil || p.Motivo != store.PurgaCuentaBorrada {
		t.Fatalf("tras el primer barrido: %+v %v", p, err)
	}
	if !p.Progreso["hub"].Hecho || !p.Progreso["drive"].BaseVacia || !p.Progreso["drive"].EsperaObjetos || !p.Progreso["bases"].SinBase {
		t.Fatalf("progreso: %+v", p.Progreso)
	}
	// Mientras espera, otro barrido no la toca: su turno es dentro de diez minutos.
	if err := l.srv.BarrerPurgasDeTenant(ctx); err != nil {
		t.Fatal(err)
	}
	if otra, _ := l.st.PurgaDeTenantDe(ctx, org.ID); otra.Intentos != 1 {
		t.Fatalf("una purga en espera se reclamó antes de su turno: %d intentos", otra.Intentos)
	}
	// Drive borra sus objetos; llega el turno; se cierra.
	if n, err := (purgatenant.Trabajador{DB: drive.Pool, Almacen: &almacenAnotado{}}).Pasada(ctx); err != nil || n != 1 {
		t.Fatalf("trabajador de Drive: %d %v", n, err)
	}
	if _, err := l.st.DB().Exec(ctx, `UPDATE tenant_purges SET next_attempt_at = now() WHERE org_id = $1`, org.ID); err != nil {
		t.Fatal(err)
	}
	if err := l.srv.BarrerPurgasDeTenant(ctx); err != nil {
		t.Fatal(err)
	}
	p, _ = l.st.PurgaDeTenantDe(ctx, org.ID)
	if p.Completada == nil || p.UltimoError != "" {
		t.Fatalf("la purga no se cerró: %+v", p)
	}
	if n := l.cuenta(t, `SELECT count(*) FROM audit_log WHERE action = 'org.content.purged' AND target = $1`, org.ID); n != 1 {
		t.Fatalf("auditoría de la purga: %d filas", n)
	}
}

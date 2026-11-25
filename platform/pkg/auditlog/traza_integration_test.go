// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package auditlog_test

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func harnessTraza(t *testing.T) *pgtest.Harness {
	t.Helper()
	return pgtest.New(t, pg.WithAuditLogDDL(fstest.MapFS{
		"migrations/0001_traza.up.sql": {Data: []byte("-- sytue:audit-log-v1\nSELECT 'audit_log'::regclass;")},
	}), "migrations")
}

func TestTrazaSeEscribeEnLotesYSeLeePorTenant(t *testing.T) {
	h := harnessTraza(t)
	ctx, cancel := context.WithCancel(context.Background())
	reg := auditlog.NuevoRegistro(h.Pool, nil)
	hecho := make(chan struct{})
	go func() { reg.Ejecutar(ctx); close(hecho) }()

	// Por el camino de un handler: el middleware deja el registro en el contexto.
	escribirDirecto(t, reg, "acme", "ana", "file.download", "f-1")
	escribirDirecto(t, reg, "acme", "ana", "file.download", "f-2")
	escribirDirecto(t, reg, "otra", "eva", "file.download", "f-3")
	// Cancelar vacía lo pendiente antes de salir.
	cancel()
	<-hecho

	admin := h.PoolSinRLS(t)
	var n int
	if err := admin.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'acceso.file.download'`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("filas escritas: %d %v", n, err)
	}
	// Leída por tenant, con filtros en la consulta y keyset.
	pag, next, err := auditlog.Listar(context.Background(), h.Pool, "acme", auditlog.Consulta{Action: "ACCESO.file", Limit: 1})
	if err != nil || len(pag) != 1 || next == "" || pag[0].Target != "f-2" {
		t.Fatalf("primera página: %+v %q %v", pag, next, err)
	}
	pos, _ := auditlog.LeerCursor(next)
	pag2, next2, err := auditlog.Listar(context.Background(), h.Pool, "acme", auditlog.Consulta{Limit: 1, After: pos})
	if err != nil || len(pag2) != 1 || next2 != "" || pag2[0].Target != "f-1" {
		t.Fatalf("segunda página: %+v %q %v", pag2, next2, err)
	}
	if otra, _, _ := auditlog.Listar(context.Background(), h.Pool, "acme", auditlog.Consulta{Actor: "eva", Limit: 10}); len(otra) != 0 {
		t.Fatal("la traza de otra organización se ve desde acme")
	}
}

// escribirDirecto encola como lo haría AnotarComo dentro de una petición.
func escribirDirecto(t *testing.T, reg *auditlog.Registro, tenant, actor, accion, objetivo string) {
	t.Helper()
	ctx := auditlog.ConRegistro(context.Background(), reg)
	auditlog.AnotarComo(ctx, tenant, actor, accion, objetivo, map[string]any{"via": "web"})
}

func TestTrazaSoloDeAnadirYRetencion(t *testing.T) {
	h := harnessTraza(t)
	ctx := context.Background()
	admin := h.PoolSinRLS(t)
	viejo := time.Now().Add(-auditlog.RetencionAcceso - 24*time.Hour)
	if _, err := admin.Exec(ctx, `INSERT INTO audit_log (occurred_at, tenant, actor, action, target) VALUES
		($1, 'acme', 'ana', 'acceso.file.download', 'viejo'),
		(now(), 'acme', 'ana', 'acceso.file.download', 'nuevo'),
		($1, 'acme', 'ana', 'ediscovery.export', 'caso-1')`, viejo); err != nil {
		t.Fatal(err)
	}
	acme := tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"})
	// Ni editar ni borrar desde la aplicación.
	for _, sql := range []string{
		`UPDATE audit_log SET target = 'otro' WHERE tenant = 'acme'`,
		`DELETE FROM audit_log WHERE tenant = 'acme'`,
	} {
		err := h.Pool.InTenantTx(acme, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, sql); return err })
		if err == nil {
			t.Fatalf("la traza admitió %q", sql)
		}
	}
	// La retención: solo acceso.* y solo lo vencido, aunque se le pida todo.
	n, err := auditlog.PurgarAccesoVencido(ctx, h.Pool, time.Now().Add(-auditlog.RetencionAcceso))
	if err != nil || n != 1 {
		t.Fatalf("retención: %d %v", n, err)
	}
	if err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM audit_log`)
		if tag.RowsAffected() != 1 {
			t.Errorf("el mantenimiento alcanzó %d filas; solo debía ver la de acceso", tag.RowsAffected())
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var quedan []string
	rows, _ := admin.Query(ctx, `SELECT target FROM audit_log ORDER BY id`)
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		quedan = append(quedan, s)
	}
	rows.Close()
	if len(quedan) != 1 || quedan[0] != "caso-1" {
		t.Fatalf("tras la retención quedan %v; la traza de cumplimiento no se toca", quedan)
	}
	// La fusión reescribe el tenant, y nada más.
	if err := h.Pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE audit_log SET tenant = 'destino' WHERE tenant = 'acme'`)
		return err
	}); err != nil {
		t.Fatalf("la fusión no pudo mover la traza: %v", err)
	}
	if err := h.Pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE audit_log SET actor = 'otro'`)
		return err
	}); err == nil {
		t.Fatal("la fusión pudo reescribir algo más que el tenant")
	}
}

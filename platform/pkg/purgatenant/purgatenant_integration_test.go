// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package purgatenant_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/fstest"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/purgatenant"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type almacenFalso struct {
	mu        sync.Mutex
	tenants   []string
	prefijos  []string
	claves    []string
	fallarHay bool
}

func (a *almacenFalso) BorrarTenant(_ context.Context, t tenancy.Tenant) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fallarHay {
		return 0, errors.New("almacén caído")
	}
	a.tenants = append(a.tenants, t.Slug)
	return 2, nil
}

func (a *almacenFalso) BorrarPrefijo(_ context.Context, p string) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prefijos = append(a.prefijos, p)
	return 1, nil
}

func (a *almacenFalso) BorrarClave(_ context.Context, k string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.claves = append(a.claves, k)
	return nil
}

func TestTrabajadorCumpleLasOrdenesYSoloCuandoLaBaseEstaVacia(t *testing.T) {
	h := pgtest.New(t, pg.WithTenantPurgeDDL(fstest.MapFS{
		"migrations/0001_cola.up.sql": {Data: []byte("-- sytue:tenant-purge-v1\nSELECT 'platform_tenant_purges'::regclass;")},
	}), "migrations")
	ctx := context.Background()
	admin := h.PoolSinRLS(t)
	// Una orden lista, otra que la purga aún no ha cerrado (sin db_purged_at): un
	// gancho la creó para sus claves, pero las filas siguen ahí.
	if _, err := admin.Exec(ctx, `
		INSERT INTO platform_tenant_purges (slug, db_purged_at) VALUES ('p-lista', now());
		INSERT INTO platform_tenant_purges (slug) VALUES ('p-a-medias');
		INSERT INTO platform_tenant_purge_keys VALUES ('p-lista', 'shared/mensajes/legado.eml');`); err != nil {
		t.Fatal(err)
	}
	alm := &almacenFalso{fallarHay: true}
	w := purgatenant.Trabajador{
		DB: h.Pool, Almacen: alm,
		Prefijos: func(slug string) []string { return []string{"shared/mensajes/" + slug + "/"} },
	}
	// Con el almacén caído la orden se queda abierta con su error.
	if n, err := w.Pasada(ctx); err != nil || n != 0 {
		t.Fatalf("pasada con fallo: %d, %v", n, err)
	}
	e, err := pg.ConsultarPurgaDeObjetos(ctx, h.Pool, "p-lista")
	if err != nil || e.Hecho() || e.UltimoErr == "" || e.Intentos != 1 {
		t.Fatalf("tras fallar: %+v, %v", e, err)
	}
	// Lo que sí se pudo borrar en el intento fallido se borró (mejor esfuerzo);
	// la orden se repite entera, porque borrar lo que no existe no es un error.
	if len(alm.prefijos) != 1 || len(alm.claves) != 1 {
		t.Fatalf("el intento fallido no siguió con el resto: %+v", alm)
	}
	// Vuelve el almacén; el reintento espera su turno, así que se adelanta.
	alm.fallarHay, alm.prefijos, alm.claves = false, nil, nil
	if _, err := admin.Exec(ctx, `UPDATE platform_tenant_purges SET last_attempt_at = now() - interval '1 day'`); err != nil {
		t.Fatal(err)
	}
	if n, err := w.Pasada(ctx); err != nil || n != 1 {
		t.Fatalf("pasada: %d, %v", n, err)
	}
	if !slices.Equal(alm.tenants, []string{"p-lista"}) || !slices.Equal(alm.prefijos, []string{"shared/mensajes/p-lista/"}) ||
		!slices.Equal(alm.claves, []string{"shared/mensajes/legado.eml"}) {
		t.Fatalf("el almacén recibió %+v", alm)
	}
	e, err = pg.ConsultarPurgaDeObjetos(ctx, h.Pool, "p-lista")
	if err != nil || !e.Hecho() || e.UltimoErr != "" {
		t.Fatalf("cerrada: %+v, %v", e, err)
	}
	var claves int
	_ = admin.QueryRow(ctx, `SELECT count(*) FROM platform_tenant_purge_keys`).Scan(&claves)
	if claves != 0 {
		t.Fatal("las claves de una orden cumplida sobran")
	}
	// La que no tiene la base vacía no se ha tocado.
	if e, _ := pg.ConsultarPurgaDeObjetos(ctx, h.Pool, "p-a-medias"); e.Hecho() || !e.Pendiente {
		t.Fatalf("la orden a medias: %+v", e)
	}
	// Una transacción de negocio (ni fusión ni mantenimiento) no ve la cola.
	var visibles int
	if err := h.Pool.QueryRow(ctx, `SELECT count(*) FROM platform_tenant_purges`).Scan(&visibles); err != nil || visibles != 0 {
		t.Fatalf("la cola es visible sin modo: %d, %v", visibles, err)
	}
}

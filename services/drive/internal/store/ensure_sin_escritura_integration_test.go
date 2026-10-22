// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Service.Root() pasa por EnsureQuota en CADA resolución de ruta de WebDAV y en
// cada GET /folders/root, que es el arranque de la app de Drive y el de Rooms.
// Mientras lo resolvía con un `INSERT ... ON CONFLICT DO UPDATE SET tenant =
// EXCLUDED.tenant`, cada una de esas peticiones escribía una versión nueva de la
// fila sin cambiar ni un valor: tupla muerta, WAL y un bloqueo sobre la ÚNICA
// fila de cuota del tenant, por la que pasan todas sus peticiones a la vez.
//
// La señal es `xmin`, el id de transacción que escribió la versión visible de la
// fila: si Postgres la reescribe, cambia.
package store_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// xminCuota devuelve el xmin de la fila de cuota del tenant.
func xminCuota(t *testing.T, ctx context.Context, h *pgtest.Harness, slug string) string {
	t.Helper()
	var x string
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT xmin::text FROM quotas WHERE tenant = $1`, slug).Scan(&x)
	})
	if err != nil {
		t.Fatalf("leer xmin de la cuota: %v", err)
	}
	return x
}

func TestEnsureQuotaNoEscribeCuandoYaExiste(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	const limite = int64(5 << 30)
	q, err := st.EnsureQuota(ctx, limite)
	if err != nil {
		t.Fatalf("EnsureQuota (alta): %v", err)
	}
	if q.Tenant != "acme" || q.LimitBytes != limite {
		t.Fatalf("cuota recién creada = %+v", q)
	}
	antes := xminCuota(t, ctx, h, "acme")

	// Diez resoluciones de ruta, que es lo que hace un cliente WebDAV sincronizando.
	for i := range 10 {
		got, err := st.EnsureQuota(ctx, limite)
		if err != nil {
			t.Fatalf("EnsureQuota #%d: %v", i, err)
		}
		if got.Tenant != "acme" || got.LimitBytes != limite {
			t.Fatalf("EnsureQuota #%d devolvió %+v", i, got)
		}
	}
	if despues := xminCuota(t, ctx, h, "acme"); despues != antes {
		t.Fatalf("xmin %s -> %s: EnsureQuota reescribe la fila de cuota en cada llamada. "+
			"Son una tupla muerta y un bloqueo de fila por CADA petición de WebDAV y por cada GET /folders/root", antes, despues)
	}
}

// xminRaiz devuelve el xmin de la carpeta raíz del tenant.
func xminRaiz(t *testing.T, ctx context.Context, h *pgtest.Harness, slug string) string {
	t.Helper()
	var x string
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT xmin::text FROM folders WHERE tenant = $1 AND is_root`, slug).Scan(&x)
	}); err != nil {
		t.Fatalf("leer xmin de la raíz: %v", err)
	}
	return x
}

// EnsureBase es por donde pasa AHORA Service.Root(), o sea cada resolución de
// ruta de WebDAV. La garantía de arriba —leer no escribe— tiene que seguir
// valiendo por esta vía: si no, se habría perdido justo al unificar las dos
// consultas en una transacción, que es lo contrario de lo que se pretendía.
//
// Se vigilan las DOS filas, cuota y raíz, porque ahora las toca la misma
// transacción y una escritura en cualquiera de ellas bloquearía igual.
func TestEnsureBaseNoEscribeCuandoYaExiste(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	const limite = int64(4096)

	raiz, cuota, err := st.EnsureBase(ctx, limite, false)
	if err != nil {
		t.Fatalf("EnsureBase (alta): %v", err)
	}
	if cuota.LimitBytes != limite || !raiz.IsRoot {
		t.Fatalf("alta inesperada: cuota=%+v raiz=%+v", cuota, raiz)
	}

	antesCuota, antesRaiz := xminCuota(t, ctx, h, "acme"), xminRaiz(t, ctx, h, "acme")
	for i := range 5 {
		r, q, err := st.EnsureBase(ctx, limite, false)
		if err != nil {
			t.Fatalf("EnsureBase #%d: %v", i, err)
		}
		if q.LimitBytes != limite || r.ID != raiz.ID {
			t.Fatalf("EnsureBase #%d devolvió cuota=%+v raiz=%+v", i, q, r)
		}
	}
	if x := xminCuota(t, ctx, h, "acme"); x != antesCuota {
		t.Fatalf("la cuota se reescribió: xmin %s -> %s", antesCuota, x)
	}
	if x := xminRaiz(t, ctx, h, "acme"); x != antesRaiz {
		t.Fatalf("la raíz se reescribió: xmin %s -> %s", antesRaiz, x)
	}
}

// Y lo que devuelve tiene que ser lo MISMO que devolvían las dos por separado:
// el objetivo era ahorrar una transacción, no cambiar la respuesta.
func TestEnsureBaseCoincideConLasDosPorSeparado(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	const limite = int64(8192)

	qSuelta, err := st.EnsureQuota(ctx, limite)
	if err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	rSuelta, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}

	rJunta, qJunta, err := st.EnsureBase(ctx, limite, false)
	if err != nil {
		t.Fatalf("EnsureBase: %v", err)
	}
	if qJunta.Tenant != qSuelta.Tenant || qJunta.LimitBytes != qSuelta.LimitBytes || qJunta.UsedBytes != qSuelta.UsedBytes {
		t.Fatalf("cuota distinta: suelta=%+v junta=%+v", qSuelta, qJunta)
	}
	if rJunta.ID != rSuelta.ID || !rJunta.IsRoot {
		t.Fatalf("raíz distinta: suelta=%+v junta=%+v", rSuelta, rJunta)
	}
}

// Con el límite dictado por el plan (`fijar`), la fila se actualiza cuando el
// plan cambia —es como una cuenta personal pasa a 1 TB sin que nadie avise a
// Drive— y, si no cambia, sigue sin escribirse nada: es la misma garantía de
// arriba con la puerta de escritura abierta solo para el caso que la necesita.
func TestEnsureBaseFijaLaCuotaSoloCuandoCambia(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("personal")

	if _, q, err := st.EnsureBase(ctx, 5<<30, true); err != nil || q.LimitBytes != 5<<30 {
		t.Fatalf("alta con 5 GiB: cuota=%+v err=%v", q, err)
	}
	antes := xminCuota(t, ctx, h, "personal")
	// Mismo plan, cinco veces: ni una escritura.
	for i := range 5 {
		if _, q, err := st.EnsureBase(ctx, 5<<30, true); err != nil || q.LimitBytes != 5<<30 {
			t.Fatalf("EnsureBase #%d: cuota=%+v err=%v", i, q, err)
		}
	}
	if x := xminCuota(t, ctx, h, "personal"); x != antes {
		t.Fatalf("con el mismo límite no debe reescribir: xmin %s -> %s", antes, x)
	}
	// Cambia el plan: la fila sigue al token.
	_, q, err := st.EnsureBase(ctx, 1<<40, true)
	if err != nil || q.LimitBytes != 1<<40 {
		t.Fatalf("subir a 1 TiB: cuota=%+v err=%v", q, err)
	}
	// Y un valor por defecto (no fijado) NO pisa lo que hay: es lo que protege la
	// cuota que un administrador ajustó a mano en una organización.
	_, q, err = st.EnsureBase(ctx, 10<<30, false)
	if err != nil || q.LimitBytes != 1<<40 {
		t.Fatalf("el defecto no debe pisar el límite fijado: cuota=%+v err=%v", q, err)
	}
}

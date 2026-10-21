// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

func TestFacturasPendientesProcesoAcotaYPaginaAntesDelLimite(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	// Más de cien facturas recientes de otro período no deben ocultar ninguna
	// del trimestre. Confirmado/pagado tampoco implica que estén en el libro.
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO records (tenant,model,estado,data,owner_sub,created_at)
			SELECT 'acme', 'factura_venta', 'borrador', '{"fecha":"2026-10-01"}', 'ada', '2026-10-01'::timestamptz
			FROM generate_series(1,120)`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO records (tenant,model,estado,data,owner_sub,created_at)
			SELECT 'acme', CASE WHEN n%2=0 THEN 'factura_compra' ELSE 'factura_venta' END,
			CASE n%3 WHEN 0 THEN 'confirmado' WHEN 1 THEN 'pagado' ELSE 'borrador' END,
			'{"fecha":"2026-09-12"}', 'ada', '2026-09-12'::timestamptz FROM generate_series(1,103) n`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	desde, hasta := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	modelos := []string{"factura_venta", "factura_compra"}
	var cursor string
	vistos := map[string]bool{}
	estados := map[string]bool{}
	for {
		p, err := st.FacturasPendientesProceso(ctx, desde, hasta, modelos, cursor, 30)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range p.Registros {
			if vistos[r.ID] || r.Data["fecha"] != "2026-09-12" {
				t.Fatalf("página repetida o de otro período: %+v", r)
			}
			vistos[r.ID], estados[r.Estado] = true, true
		}
		if p.Siguiente == "" {
			break
		}
		cursor = p.Siguiente
	}
	pendientes, err := st.Pendientes(ctx, desde, hasta)
	if err != nil || len(vistos) != 103 || pendientes["facturas"]["2026-09"] != len(vistos) || len(estados) != 3 {
		t.Fatalf("listado y contador difieren: %d %v %v %v", len(vistos), pendientes, estados, err)
	}
	if otra, err := st.FacturasPendientesProceso(tenantCtx("globex"), desde, hasta, modelos, "", 30); err != nil || len(otra.Registros) != 0 {
		t.Fatalf("aislamiento: %+v %v", otra, err)
	}
	if _, err := st.FacturasPendientesProceso(ctx, desde, hasta, modelos, "cursor-invalido", 30); !errors.Is(err, domain.ErrDatos) {
		t.Fatalf("cursor inválido: %v", err)
	}
}

func TestBorrarUltimoJustificanteReabreSoloSuProceso(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	docs := []string{"justificante", "pago"}
	for fase := 1; fase <= 3; fase++ {
		if _, err := st.AvanzarProceso(ctx, "es_303", "2026-T3", fase, "", docs, "ada"); err != nil {
			t.Fatal(err)
		}
	}
	guardar := func(doc string) store.Adjunto {
		t.Helper()
		a, _, err := st.GuardarAdjunto(ctx, "obligacion", "es_303:2026-T3:"+doc, "documento.pdf", "application/pdf", []byte("%PDF-1.7 prueba"), "ada")
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	uno, otro, pago, adicional := guardar("justificante"), guardar("justificante"), guardar("pago"), guardar("adicional")
	if _, err := st.AvanzarProceso(ctx, "es_303", "2026-T3", 4, "CSV-303", docs, "ada"); err != nil {
		t.Fatal(err)
	}
	if err := st.BorrarAdjunto(tenantCtx("globex"), pago.ID); !errors.Is(err, store.ErrNoEncontrado) {
		t.Fatalf("otro tenant pudo borrar: %v", err)
	}
	for _, a := range []store.Adjunto{uno, adicional} {
		if err := st.BorrarAdjunto(ctx, a.ID); err != nil {
			t.Fatal(err)
		}
		if p, err := st.ProcesoObligacion(ctx, "es_303", "2026-T3"); err != nil || p.Fase != 4 {
			t.Fatalf("borrar copia o documento adicional reabre: %+v %v", p, err)
		}
	}
	if err := st.BorrarAdjunto(ctx, otro.ID); err != nil {
		t.Fatal(err)
	}
	p, err := st.ProcesoObligacion(ctx, "es_303", "2026-T3")
	if err != nil || p.Fase != 3 || p.Referencia != "CSV-303" {
		t.Fatalf("último justificante no reabre: %+v %v", p, err)
	}
	if _, err := st.AvanzarProceso(ctx, "es_303", "2026-T3", 4, "CSV-303", docs, "ada"); !errors.Is(err, domain.ErrDatos) {
		t.Fatalf("volver a terminar sin archivo: %v", err)
	}
	_, procesos, err := st.PreparacionObligaciones(ctx, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), []string{"2026-T3"})
	if err != nil || procesos["es_303:2026-T3"].Fase != 3 {
		t.Fatalf("estado bulk: %+v %v", procesos, err)
	}
	guardar("justificante")
	if p, err := st.AvanzarProceso(ctx, "es_303", "2026-T3", 4, "CSV-CORREGIDO", docs, "ada"); err != nil || p.Fase != 4 || p.Referencia != "CSV-CORREGIDO" {
		t.Fatalf("reponer justificante: %+v %v", p, err)
	}
}

func TestTerminarYBorrarJustificanteConcurrentes(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	for i := range 8 {
		clave := fmt.Sprintf("es_303_%d", i)
		for fase := 1; fase <= 3; fase++ {
			if _, err := st.AvanzarProceso(ctx, clave, "2026-T3", fase, "", nil, "ada"); err != nil {
				t.Fatal(err)
			}
		}
		a, _, err := st.GuardarAdjunto(ctx, "obligacion", clave+":2026-T3:justificante", "archivo.pdf", "application/pdf", []byte("%PDF-1.7 prueba"), "ada")
		if err != nil {
			t.Fatal(err)
		}
		inicio := make(chan struct{})
		fallos := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-inicio
			_, err := st.AvanzarProceso(ctx, clave, "2026-T3", 4, "CSV", nil, "ada")
			if err != nil && !errors.Is(err, domain.ErrDatos) {
				fallos <- err
			}
		}()
		go func() {
			defer wg.Done()
			<-inicio
			if err := st.BorrarAdjunto(ctx, a.ID); err != nil {
				fallos <- err
			}
		}()
		close(inicio)
		wg.Wait()
		close(fallos)
		for err := range fallos {
			t.Fatal(err)
		}
		if p, err := st.ProcesoObligacion(ctx, clave, "2026-T3"); err != nil || p.Fase != 3 {
			t.Fatalf("terminó sin justificante en carrera: %+v %v", p, err)
		}
	}
}

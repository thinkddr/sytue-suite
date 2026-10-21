// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del VISOR de auditoría contra un PostgreSQL real.
//
// Aquí no se prueba lógica de Go: se prueban las garantías que solo da la base de
// datos y que sostienen una consulta de compliance —que es la que se hace cuando
// algo ya ha pasado y hay que demostrar quién lo hizo—:
//   - el recorrido por CURSOR no pierde ni repite filas (keyset, sin OFFSET),
//   - los filtros los aplica el SERVIDOR (si no, "buscar" solo miraría la 1.ª página),
//   - un '%' en el filtro es texto, no un comodín que devuelve la tabla entera,
//   - la traza de una organización NO se ve desde otra,
//   - el export CSV lleva el mismo conjunto y neutraliza fórmulas de hoja de cálculo.
package store

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
)

func auditCtx(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

// seedAudit inserta un evento con un occurred_at CONCRETO (RecordAudit siempre
// sella con now(), y para probar rangos y orden hace falta controlar el reloj).
func seedAudit(t *testing.T, st *Store, slug, actor, action, resource string, at time.Time) {
	t.Helper()
	ctx := auditCtx(slug)
	err := st.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_events (tenant, actor_sub, action, resource, detail_json, occurred_at)
			VALUES ($1, $2, $3, $4, '{"k":"v"}'::jsonb, $5)`, slug, actor, action, resource, at)
		return err
	})
	if err != nil {
		t.Fatalf("sembrar auditoría: %v", err)
	}
}

func TestAuditVisorIntegration(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := New(h.Pool)
	ctx := auditCtx("acme")
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	// 10 eventos de acme, todos con el MISMO occurred_at a propósito: es el caso
	// que rompe un keyset mal hecho (sin desempate por id se repite o se salta una
	// fila al cambiar de página), y en auditoría los eventos llegan a ráfagas.
	for range 10 {
		seedAudit(t, st, "acme", "ana@acme.eu", "token.create", "ci", base)
	}
	seedAudit(t, st, "acme", "bruno@acme.eu", "billing.set_plan", "pro", base.Add(-48*time.Hour))
	seedAudit(t, st, "acme", "bruno@acme.eu", "token.revoke", "ci", base.Add(-96*time.Hour))
	seedAudit(t, st, "otra", "espia@otra.eu", "project.delete", "secreto", base)

	t.Run("el cursor recorre TODO sin repetir ni perder filas", func(t *testing.T) {
		seen := map[string]int{}
		var after PageCursor
		for pages := 0; ; pages++ {
			if pages > 20 {
				t.Fatal("el recorrido por cursor no termina")
			}
			page, err := st.ListAudit(ctx, AuditFilter{}, after, 5)
			if err != nil {
				t.Fatalf("ListAudit: %v", err)
			}
			for _, e := range page.Events {
				seen[e.ID.String()]++
			}
			if page.Next == nil {
				break
			}
			after = *page.Next
		}
		if len(seen) != 12 {
			t.Errorf("%d eventos distintos, quiero 12 (los de acme, ninguno de otra)", len(seen))
		}
		for id, n := range seen {
			if n != 1 {
				t.Errorf("el evento %s salió %d veces", id, n)
			}
		}
	})

	t.Run("orden estable descendente", func(t *testing.T) {
		page, err := st.ListAudit(ctx, AuditFilter{}, PageCursor{}, 50)
		if err != nil {
			t.Fatalf("ListAudit: %v", err)
		}
		for i := 1; i < len(page.Events); i++ {
			if page.Events[i].OccurredAt.After(page.Events[i-1].OccurredAt) {
				t.Fatalf("orden roto en %d: %v > %v", i,
					page.Events[i].OccurredAt, page.Events[i-1].OccurredAt)
			}
		}
	})

	t.Run("filtra por actor (subcadena), acción (prefijo) y rango", func(t *testing.T) {
		cases := []struct {
			name string
			f    AuditFilter
			want int
		}{
			{"actor por subcadena", AuditFilter{Actor: "bruno"}, 2},
			{"actor ignora mayúsculas", AuditFilter{Actor: "BRUNO@ACME.EU"}, 2},
			{"prefijo de acción", AuditFilter{Action: "token."}, 11},
			{"acción exacta", AuditFilter{Action: "token.revoke"}, 1},
			{"desde", AuditFilter{Since: ptr(base.Add(-time.Hour))}, 10},
			{"hasta", AuditFilter{Until: ptr(base.Add(-time.Hour))}, 2},
			{"combinado", AuditFilter{Actor: "bruno", Action: "token."}, 1},
			// Un '%' es TEXTO: si se colara como comodín, este filtro devolvería
			// los 12 eventos en vez de ninguno (fuga de la tabla entera).
			{"comodín literal", AuditFilter{Actor: "%"}, 0},
			{"guion bajo literal", AuditFilter{Action: "token_"}, 0},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				page, err := st.ListAudit(ctx, c.f, PageCursor{}, 100)
				if err != nil {
					t.Fatalf("ListAudit: %v", err)
				}
				if len(page.Events) != c.want {
					t.Errorf("%d eventos, quiero %d", len(page.Events), c.want)
				}
			})
		}
	})

	t.Run("la traza de otra organización no se ve", func(t *testing.T) {
		page, err := st.ListAudit(ctx, AuditFilter{Action: "project.delete"}, PageCursor{}, 100)
		if err != nil {
			t.Fatalf("ListAudit: %v", err)
		}
		if len(page.Events) != 0 {
			t.Fatalf("acme ve %d eventos de otra organización", len(page.Events))
		}
		// Y desde la suya sí, para descartar que el test pase por estar vacía.
		otra, err := st.ListAudit(auditCtx("otra"), AuditFilter{}, PageCursor{}, 100)
		if err != nil {
			t.Fatalf("ListAudit(otra): %v", err)
		}
		if len(otra.Events) != 1 {
			t.Errorf("otra ve %d eventos propios, quiero 1", len(otra.Events))
		}
	})

	t.Run("el export CSV lleva el conjunto filtrado", func(t *testing.T) {
		var buf bytes.Buffer
		if err := st.ExportAudit(ctx, AuditFilter{Actor: "bruno"}, &buf); err != nil {
			t.Fatalf("ExportAudit: %v", err)
		}
		recs, err := csv.NewReader(&buf).ReadAll()
		if err != nil {
			t.Fatalf("CSV ilegible: %v", err)
		}
		if len(recs) != 3 { // cabecera + 2 eventos de bruno
			t.Fatalf("%d filas, quiero 3 (cabecera + 2)", len(recs))
		}
		if got := strings.Join(recs[0], ","); got != "ts,actor,action,target,detail" {
			t.Errorf("cabecera = %q", got)
		}
		if recs[1][1] != "bruno@acme.eu" {
			t.Errorf("actor exportado = %q", recs[1][1])
		}
	})

	t.Run("el CSV no ejecuta fórmulas de hoja de cálculo", func(t *testing.T) {
		// El actor lo elige quien se autentica: si llega a Excel como fórmula, abrir
		// el export es ejecución de código (CWE-1236).
		seedAudit(t, st, "acme", `=cmd|'/c calc'!A1`, "project.create", "x", base)
		var buf bytes.Buffer
		if err := st.ExportAudit(ctx, AuditFilter{Actor: "cmd"}, &buf); err != nil {
			t.Fatalf("ExportAudit: %v", err)
		}
		recs, err := csv.NewReader(&buf).ReadAll()
		if err != nil {
			t.Fatalf("CSV ilegible: %v", err)
		}
		if len(recs) != 2 {
			t.Fatalf("%d filas, quiero 2", len(recs))
		}
		if !strings.HasPrefix(recs[1][1], "'=") {
			t.Errorf("la celda %q debería ir neutralizada con apóstrofo", recs[1][1])
		}
	})
}

func ptr[T any](v T) *T { return &v }

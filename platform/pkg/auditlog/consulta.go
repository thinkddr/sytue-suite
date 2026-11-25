// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auditlog

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// MaxCSV acota la exportación CSV de una vez.
const MaxCSV = 5000

// Listar devuelve una página de la traza del tenant con los filtros EN LA
// CONSULTA. Era lo que el comentario de VentanaMax pedía «si la tabla creciera»:
// con la traza de acceso en la misma tabla crece, y filtrar en memoria las 500
// últimas filas habría dejado la de cumplimiento enterrada bajo las descargas de
// una mañana. Va en InTenantTx con el tenant también en el WHERE (RLS + defensa
// en profundidad). Con q.CSV devuelve hasta MaxCSV filas y sin cursor.
func Listar(ctx context.Context, db pg.Database, tenant string, q Consulta) ([]Entrada, string, error) {
	if !tenancy.ValidSlug(tenant) {
		return nil, "", fmt.Errorf("auditlog: tenant inválido")
	}
	limite := q.Limit
	if q.CSV {
		limite = MaxCSV
	}
	if limite <= 0 || limite > MaxCSV {
		limite = 100
	}
	var out []Entrada
	err := db.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		sql, args := consultaSQL(tenant, q, limite+1)
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return fmt.Errorf("auditlog: listar: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var e Entrada
			var detail []byte
			if err := rows.Scan(&e.ID, &e.OccurredAt, &e.Tenant, &e.Actor, &e.Action, &e.Target, &detail); err != nil {
				return err
			}
			e.Detail = detail
			out = append(out, e)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, "", err
	}
	if q.CSV {
		if len(out) > MaxCSV {
			out = out[:MaxCSV]
		}
		return out, "", nil
	}
	next := ""
	if len(out) > limite {
		out = out[:limite]
		last := out[limite-1]
		next = FormatearCursor(last.OccurredAt, last.ID)
	}
	return out, next, nil
}

// consultaSQL arma la consulta con los filtros que traiga q. Los valores van
// SIEMPRE como parámetros; lo único que se compone es qué condiciones hay.
func consultaSQL(tenant string, q Consulta, limite int) (string, []any) {
	var w strings.Builder
	args := []any{tenant}
	w.WriteString(`SELECT id, occurred_at, tenant, actor, action, target, detail FROM audit_log WHERE tenant = $1`)
	param := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	// Ni la acción ni el actor son texto de nadie: la acción es un identificador
	// ASCII en minúsculas («acceso.file.download») y el actor, un sub o
	// «link:<id>». Por eso no pasan por plegar(), que es para lo que escribe una
	// persona: basta con comparar en minúsculas, sin comodines que escapar.
	if q.Action != "" {
		fmt.Fprintf(&w, ` AND starts_with(action, %s)`, param(strings.ToLower(q.Action)))
	}
	if q.Actor != "" {
		fmt.Fprintf(&w, ` AND strpos(actor, %s) > 0`, param(strings.ToLower(q.Actor)))
	}
	if !q.Since.IsZero() {
		fmt.Fprintf(&w, ` AND occurred_at >= %s`, param(q.Since))
	}
	if !q.Until.IsZero() {
		fmt.Fprintf(&w, ` AND occurred_at < %s`, param(q.Until))
	}
	if q.After != nil && !q.CSV {
		fmt.Fprintf(&w, ` AND (occurred_at, id) < (%s, %s)`, param(q.After.OccurredAt), param(q.After.ID))
	}
	fmt.Fprintf(&w, ` ORDER BY occurred_at DESC, id DESC LIMIT %s`, param(limite))
	return w.String(), args
}

// Servir es el GET /admin/audit-log de un servicio: lee la consulta, lista y
// contesta en JSON ({items, nextCursor}) o en CSV. Quién puede llamar lo decide
// el montaje de cada servicio (owner/admin); el tenant sale de la sesión.
func Servir(w http.ResponseWriter, r *http.Request, db pg.Database, servicio string) {
	tn, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("The token has no active organisation."))
		return
	}
	q, err := LeerConsulta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	entradas, next, err := Listar(r.Context(), db, tn.Slug, q)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if q.CSV {
		EscribirCSV(w, r, servicio, entradas)
		return
	}
	if entradas == nil {
		entradas = []Entrada{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": entradas, "nextCursor": next})
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
)

// auditExportCap acota el CSV de auditoría: tope duro de filas por descarga, para
// que un export no se lleve por delante la memoria del cliente ni una conexión
// eternamente abierta.
//
// ponytail: cota conocida; acotar por rango de fechas es lo que reduce el conjunto.
// Para archivado íntegro haría falta un job a object storage, no una descarga HTTP.
const auditExportCap = 100_000

// AuditEntry es un evento de auditoría de acciones admin.
type AuditEntry struct {
	ActorSub string
	Action   string
	Resource string
	Detail   map[string]any
}

// AuditRecord es un evento ya persistido, tal y como se sirve al cliente (las
// etiquetas JSON SON el contrato de la API; el dashboard las consume).
type AuditRecord struct {
	ID       uuid.UUID `json:"id"`
	ActorSub string    `json:"actor_sub"`
	Action   string    `json:"action"`
	Resource string    `json:"resource"`
	// Detail se pasa VERBATIM desde jsonb: ni se deserializa ni se re-serializa
	// (evita reordenar claves y perder números grandes por el paso por float64).
	Detail     json.RawMessage `json:"detail"`
	OccurredAt time.Time       `json:"occurred_at"`
}

// AuditFilter acota una consulta de auditoría. Todos los campos son opcionales.
// La organización NO va aquí: la fija el tenant del contexto (RLS + predicado
// explícito), nunca el llamante.
type AuditFilter struct {
	// Actor casa por SUBCADENA (case-insensitive): quien consulta recuerda un
	// trozo del correo o del sub, no el identificador entero.
	Actor string
	// Action casa por PREFIJO: las acciones son jerárquicas ("billing.", "token."),
	// así que un prefijo es justo "todo lo de facturación".
	Action string
	Since  *time.Time // occurred_at >= Since
	Until  *time.Time // occurred_at <  Until
}

// AuditPage es una página de auditoría con el cursor para pedir la siguiente.
type AuditPage struct {
	Events []AuditRecord
	// Next es el cursor de continuación; nil si no quedan más resultados.
	Next *PageCursor
}

const auditSelect = `SELECT id, actor_sub, action, resource, detail_json, occurred_at FROM audit_events`

// RecordAudit inserta un evento de auditoría del tenant activo.
func (s *Store) RecordAudit(ctx context.Context, in AuditEntry) error {
	t := tenancy.MustFrom(ctx)
	detail, err := json.Marshal(in.Detail)
	if err != nil {
		detail = []byte("{}")
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_events (tenant, actor_sub, action, resource, detail_json, occurred_at)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			t.Slug, in.ActorSub, in.Action, in.Resource, detail, time.Now().UTC())
		if err != nil {
			return fmtErr("registrar auditoría", err)
		}
		return nil
	})
}

// auditWhere construye el WHERE parametrizado (tenant + filtros + keyset) y sus
// args. El predicado de tenant es EXPLÍCITO además de RLS (defensa en profundidad,
// igual que el resto de listados del store).
func auditWhere(tenant string, f AuditFilter, after PageCursor) (string, []any) {
	args := []any{}
	ph := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	var b strings.Builder
	b.WriteString(` WHERE tenant = ` + ph(tenant))
	if f.Actor != "" {
		b.WriteString(` AND plegar(actor_sub) LIKE plegar(` + ph("%"+escapeLike(f.Actor)+"%") + `) ESCAPE '\'`)
	}
	if f.Action != "" {
		b.WriteString(` AND plegar(action) LIKE plegar(` + ph(escapeLike(f.Action)+"%") + `) ESCAPE '\'`)
	}
	if f.Since != nil {
		b.WriteString(` AND occurred_at >= ` + ph(*f.Since))
	}
	if f.Until != nil {
		b.WriteString(` AND occurred_at < ` + ph(*f.Until))
	}
	if !after.IsZero() {
		// Keyset, NUNCA OFFSET: el predicado compuesto recorre el índice
		// audit_events_tenant_time_idx y su coste no crece con la profundidad.
		b.WriteString(` AND (occurred_at, id) < (` + ph(after.CreatedAt) + `, ` + ph(after.ID) + `)`)
	}
	return b.String(), args
}

func scanAudit(row pgx.Row) (AuditRecord, error) {
	var e AuditRecord
	err := row.Scan(&e.ID, &e.ActorSub, &e.Action, &e.Resource, &e.Detail, &e.OccurredAt)
	return e, err
}

// ListAudit devuelve una página de eventos del tenant activo (más recientes
// primero) filtrada por actor/acción/rango, con paginación keyset por
// (occurred_at, id) DESC. Pide limit+1 para saber si hay página siguiente sin un
// COUNT extra.
func (s *Store) ListAudit(ctx context.Context, f AuditFilter, after PageCursor, limit int) (AuditPage, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var page AuditPage
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		where, args := auditWhere(t.Slug, f, after)
		args = append(args, limit+1)
		rows, err := tx.Query(ctx,
			auditSelect+where+` ORDER BY occurred_at DESC, id DESC LIMIT $`+strconv.Itoa(len(args)), args...)
		if err != nil {
			return fmtErr("listar auditoría", err)
		}
		defer rows.Close()
		for rows.Next() {
			e, err := scanAudit(rows)
			if err != nil {
				return err
			}
			page.Events = append(page.Events, e)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(page.Events) > limit {
			last := page.Events[limit-1]
			page.Events = page.Events[:limit]
			page.Next = &PageCursor{CreatedAt: last.OccurredAt, ID: last.ID}
		}
		return nil
	})
	if err != nil {
		return AuditPage{}, err
	}
	return page, nil
}

// ExportAudit escribe en w la auditoría filtrada del tenant activo como CSV, en
// streaming fila a fila (no materializa la tabla en memoria). Acotado por
// auditExportCap. Mismo esquema de columnas que el resto de exports de la suite,
// para que un SIEM ingiera todos igual.
func (s *Store) ExportAudit(ctx context.Context, f AuditFilter, w io.Writer) error {
	t := tenancy.MustFrom(ctx)
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		where, args := auditWhere(t.Slug, f, PageCursor{})
		args = append(args, auditExportCap)
		rows, err := tx.Query(ctx,
			auditSelect+where+` ORDER BY occurred_at DESC, id DESC LIMIT $`+strconv.Itoa(len(args)), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		cw := csv.NewWriter(w)
		if err := cw.Write([]string{"ts", "actor", "action", "target", "detail"}); err != nil {
			return err
		}
		for rows.Next() {
			e, err := scanAudit(rows)
			if err != nil {
				return err
			}
			if err := cw.Write([]string{
				e.OccurredAt.UTC().Format(time.RFC3339), validation.CSVSafe(e.ActorSub),
				validation.CSVSafe(e.Action), validation.CSVSafe(e.Resource), validation.CSVSafe(string(e.Detail)),
			}); err != nil {
				return err
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	})
	if err != nil {
		return fmtErr("exportar auditoría", err)
	}
	return nil
}

// escapeLike escapa los metacaracteres de LIKE (\ % _) para tratar la entrada del
// usuario como texto literal; la consulta declara ESCAPE '\'. Sin esto, un actor
// con un '%' devolvería la tabla entera.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
)

// AuditEntry es un registro de auditoría inmutable (append-only).
type AuditEntry struct {
	ActorID string
	OrgID   string
	Action  string
	Target  string
	IP      string
	Detail  map[string]any
}

// Audit inserta un registro de auditoría. No debe contener datos personales
// sensibles en claro (docs/CONVENTIONS.md §0.7, §10).
func (s *Store) Audit(ctx context.Context, q pg.Querier, e AuditEntry) error {
	if q == nil {
		q = s.DB()
	}
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return errf("Audit.marshal", err)
	}
	const sql = `INSERT INTO audit_log (actor_id, org_id, action, target, ip, detail)
		VALUES (nullif($1,'')::uuid, nullif($2,'')::uuid, $3, nullif($4,''),
		        nullif($5,'')::inet, $6)`
	_, err = q.Exec(ctx, sql, e.ActorID, e.OrgID, e.Action, e.Target, e.IP, raw)
	return errf("Audit", err)
}

// AuditFilter acota una consulta del log. Todos los campos son opcionales; la
// organización SIEMPRE la fija el llamante (aislamiento por tenant), no va aquí.
type AuditFilter struct {
	Action  string     // prefijo de acción (LIKE 'x%'); vacío = todas
	ActorID string     // uuid exacto del actor; vacío = todos
	Since   *time.Time // occurred_at >= Since
	Until   *time.Time // occurred_at <  Until
}

// AuditCursor es la posición keyset (occurred_at, id) DESC de la última fila
// devuelta. nil ⇒ primera página. id es el bigint monótono del audit_log, que
// desempata filas con el mismo occurred_at.
type AuditCursor struct {
	OccurredAt time.Time
	ID         int64
}

// AuditRow es una entrada de auditoría para lectura admin. Incluye el email del
// actor (JOIN a users) para no exponer solo UUIDs en la Console. detail va como
// JSON crudo (la tabla lo muestra opcionalmente; el CSV lo exporta tal cual).
type AuditRow struct {
	ID         int64
	OccurredAt time.Time
	ActorID    string
	ActorEmail string
	Action     string
	Target     string
	IP         string
	// Detail son los bytes JSON crudos de la columna jsonb. []byte (no
	// json.RawMessage) para acertar el caso *[]byte del códec JSONB de pgx, que
	// copia los bytes sin re-parsear.
	Detail []byte
	// OrgSlug solo lo rellena el registro de PLATAFORMA, que cruza todas las
	// organizaciones: en el de una sola, la organización es el contexto y
	// repetirla en cada fila no dice nada.
	OrgSlug string
}

// auditExportCap acota el CSV de auditoría: un tope duro de filas por descarga.
// ponytail: cota conocida; para archivado íntegro de una tabla que crece sin
// límite, un job a object storage. Filtrar por rango de fechas reduce el set.
const auditExportCap = 100_000

// auditSelect proyecta las columnas de AuditRow con el email del actor resuelto.
// El LEFT JOIN por PK (users.id) es un nested loop barato; el índice
// audit_log_org_occurred_idx (0008) sirve el WHERE org + el ORDER BY/keyset.
const auditSelect = `SELECT l.id, l.occurred_at, coalesce(l.actor_id::text,''),
	coalesce(a.email,''), l.action, coalesce(l.target,''),
	coalesce(host(l.ip),''), l.detail
	FROM audit_log l LEFT JOIN users a ON a.id = l.actor_id`

// escapeLike escapa los metacaracteres LIKE (\ % _) de una entrada de usuario
// para tratarla como texto literal, sea como prefijo o como infijo. Se combina
// con ESCAPE '\' en la consulta. Sin esto, un solo '%' devuelve la tabla entera.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// auditWhere construye la cláusula WHERE parametrizada (org + filtros + keyset)
// y sus argumentos. $1 es siempre org_id; el resto se añade solo si aplica, con
// placeholders correlativos. Devuelve el número de placeholders usados en args.
func auditWhere(orgID string, f AuditFilter, after *AuditCursor) (string, []any) {
	args := []any{orgID}
	var b strings.Builder
	b.WriteString(` WHERE l.org_id = $1`)
	ph := func(v any) string {
		args = append(args, v)
		return "$" + itoa(len(args))
	}
	if f.Action != "" {
		b.WriteString(` AND l.action LIKE ` + ph(escapeLike(f.Action)+"%") + ` ESCAPE '\'`)
	}
	if f.ActorID != "" {
		b.WriteString(` AND l.actor_id = ` + ph(f.ActorID) + `::uuid`)
	}
	if f.Since != nil {
		b.WriteString(` AND l.occurred_at >= ` + ph(*f.Since))
	}
	if f.Until != nil {
		b.WriteString(` AND l.occurred_at < ` + ph(*f.Until))
	}
	if after != nil {
		// Keyset DESC: filas estrictamente "anteriores" a la última vista.
		lo := ph(after.OccurredAt)
		hi := ph(after.ID)
		b.WriteString(` AND (l.occurred_at, l.id) < (` + lo + `, ` + hi + `)`)
	}
	return b.String(), args
}

// ListAuditPage devuelve una página del log de una organización con paginación
// keyset (occurred_at, id) DESC — sin OFFSET, estable a una tabla que solo crece.
// more=true si queda al menos una página más (consulta limit+1).
func (s *Store) ListAuditPage(ctx context.Context, orgID string, f AuditFilter, after *AuditCursor, limit int) ([]AuditRow, bool, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	where, args := auditWhere(orgID, f, after)
	args = append(args, limit+1)
	sql := auditSelect + where + ` ORDER BY l.occurred_at DESC, l.id DESC LIMIT $` + itoa(len(args))
	rows, err := s.DB().Query(ctx, sql, args...)
	if err != nil {
		return nil, false, errf("ListAuditPage", err)
	}
	defer rows.Close()
	out := make([]AuditRow, 0, limit)
	for rows.Next() {
		var a AuditRow
		if err := rows.Scan(&a.ID, &a.OccurredAt, &a.ActorID, &a.ActorEmail,
			&a.Action, &a.Target, &a.IP, &a.Detail); err != nil {
			return nil, false, errf("ListAuditPage.scan", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, false, errf("ListAuditPage.iter", err)
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

// platformAuditWhere acota a lo que hace un admin GLOBAL: las acciones
// `platform.*`, de cualquier organización o de ninguna. El literal es el mismo
// que el predicado del índice parcial de la mig 0045 —PostgreSQL solo usa un
// índice parcial si puede demostrar que el WHERE de la consulta implica el
// suyo—, así que cambiarlo aquí sin cambiarlo allí devuelve un seq scan sobre
// una tabla que solo crece.
func platformAuditWhere(f AuditFilter, after *AuditCursor) (string, []any) {
	var args []any
	var b strings.Builder
	b.WriteString(` WHERE l.action LIKE 'platform.%'`)
	ph := func(v any) string {
		args = append(args, v)
		return "$" + itoa(len(args))
	}
	if f.Action != "" {
		b.WriteString(` AND l.action LIKE ` + ph(escapeLike(f.Action)+"%") + ` ESCAPE '\'`)
	}
	if f.ActorID != "" {
		b.WriteString(` AND l.actor_id = ` + ph(f.ActorID) + `::uuid`)
	}
	if f.Since != nil {
		b.WriteString(` AND l.occurred_at >= ` + ph(*f.Since))
	}
	if f.Until != nil {
		b.WriteString(` AND l.occurred_at < ` + ph(*f.Until))
	}
	if after != nil {
		b.WriteString(` AND (l.occurred_at, l.id) < (` + ph(after.OccurredAt) + `, ` + ph(after.ID) + `)`)
	}
	return b.String(), args
}

// ListPlatformAuditPage devuelve una página del registro de PLATAFORMA: lo que
// han hecho los admin globales, de todas las organizaciones a la vez.
//
// Existe porque ese registro estaba escrito pero no se podía leer: la mitad
// lleva el org_id de la organización tocada —y solo se veía desde la consola de
// ESA organización, donde un admin global no entra— y la otra mitad (conceder
// alcance global, elevarse, retirar plazas) no lleva ninguno, así que no
// aparecía en ninguna parte. El slug sale por LEFT JOIN: sin él, «suspendida»
// es un uuid y no dice sobre quién se actuó.
func (s *Store) ListPlatformAuditPage(ctx context.Context, f AuditFilter, after *AuditCursor, limit int) ([]AuditRow, bool, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	const sel = `SELECT l.id, l.occurred_at, coalesce(l.actor_id::text,''),
		coalesce(a.email,''), l.action, coalesce(l.target,''),
		coalesce(host(l.ip),''), l.detail, coalesce(o.slug,'')
		FROM audit_log l
		LEFT JOIN users a ON a.id = l.actor_id
		LEFT JOIN organizations o ON o.id = l.org_id`
	where, args := platformAuditWhere(f, after)
	args = append(args, limit+1)
	sql := sel + where + ` ORDER BY l.occurred_at DESC, l.id DESC LIMIT $` + itoa(len(args))
	rows, err := s.DB().Query(ctx, sql, args...)
	if err != nil {
		return nil, false, errf("ListPlatformAuditPage", err)
	}
	defer rows.Close()
	out := make([]AuditRow, 0, limit)
	for rows.Next() {
		var a AuditRow
		if err := rows.Scan(&a.ID, &a.OccurredAt, &a.ActorID, &a.ActorEmail,
			&a.Action, &a.Target, &a.IP, &a.Detail, &a.OrgSlug); err != nil {
			return nil, false, errf("ListPlatformAuditPage.scan", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, false, errf("ListPlatformAuditPage.iter", err)
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

// ExportAudit escribe el log filtrado de una organización como CSV en w,
// haciendo streaming fila a fila (no materializa toda la tabla en memoria).
// Acotado por auditExportCap; el orden es cronológico inverso, como la vista.
func (s *Store) ExportAudit(ctx context.Context, orgID string, f AuditFilter, w io.Writer) error {
	where, args := auditWhere(orgID, f, nil)
	args = append(args, auditExportCap)
	sql := auditSelect + where + ` ORDER BY l.occurred_at DESC, l.id DESC LIMIT $` + itoa(len(args))
	rows, err := s.DB().Query(ctx, sql, args...)
	if err != nil {
		return errf("ExportAudit", err)
	}
	defer rows.Close()
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"occurred_at", "action", "actor_id", "actor_email", "target", "ip", "detail"}); err != nil {
		return errf("ExportAudit.header", err)
	}
	for rows.Next() {
		var a AuditRow
		if err := rows.Scan(&a.ID, &a.OccurredAt, &a.ActorID, &a.ActorEmail,
			&a.Action, &a.Target, &a.IP, &a.Detail); err != nil {
			return errf("ExportAudit.scan", err)
		}
		if err := cw.Write([]string{
			a.OccurredAt.UTC().Format(time.RFC3339), validation.CSVSafe(a.Action), validation.CSVSafe(a.ActorID),
			validation.CSVSafe(a.ActorEmail), validation.CSVSafe(a.Target), validation.CSVSafe(a.IP), validation.CSVSafe(string(a.Detail)),
		}); err != nil {
			return errf("ExportAudit.write", err)
		}
	}
	if err := rows.Err(); err != nil {
		return errf("ExportAudit.iter", err)
	}
	cw.Flush()
	return errf("ExportAudit.flush", cw.Error())
}

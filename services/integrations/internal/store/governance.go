// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Gobernanza L5 del Sytue Integration Gateway: persistencia de las reglas de
// política de egress (policies), del registro de decisiones (policy_decisions) y
// lectura admin del registro de egress (egress_audit). Dos vías de acceso, ambas
// aisladas por org:
//   - Desde una petición admin (CRUD, visores): tenantTx fija el tenant del contexto.
//   - Desde la cadena de egress (LoadPolicies/RecordDecision, fuera de una tx de
//     petición): fija el tenant EXPLÍCITAMENTE desde el orgID recibido, igual que
//     RecordEgress. Ambas activan el rol integrations_app ⇒ RLS aísla siempre.
package store

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// governanceExportCap acota el CSV de egress: tope duro de filas por descarga.
// ponytail: cota conocida; para archivado íntegro, un job a object storage.
// Filtrar por rango de fechas reduce el set.
const governanceExportCap = 100_000

// --- Motor de política: carga de reglas + registro de decisiones ---------------
// Ambos entran desde la cadena de egress con un contexto propio (sin tenant),
// así que fijan el tenant desde el orgID recibido. Satisfacen policy.DecisionLoader.

// LoadPolicies devuelve las reglas HABILITADAS de una org en orden estable
// (created_at ASC): el motor las evalúa en ese orden. Cumple policy.PolicyLoader.
func (s *Store) LoadPolicies(ctx context.Context, orgID string) ([]domain.Policy, error) {
	out := make([]domain.Policy, 0, 16)
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, orgID); err != nil {
			return err
		}
		// Solo si Resolve comprobó que el rol se PUEDE asumir; si no, el
		// aislamiento lo sostiene la RLS forzada (ver platform/pkg/pg.AppRole).
		if s.rol.Activo() {
			if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+appRole); err != nil {
				return err
			}
		}
		rows, err := tx.Query(ctx, `
			SELECT id, org_id, name, condition, action, enabled, created_at, updated_at
			  FROM policies WHERE org_id = current_setting('app.tenant', true) AND enabled = true ORDER BY created_at ASC, id ASC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			p, err := scanPolicy(rows)
			if err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: cargar políticas: %w", err)
	}
	return out, nil
}

// RecordDecision inserta una decisión (block/warn) en policy_decisions. Cumple
// policy.DecisionSink. Fija el tenant desde d.OrgID (no del contexto).
func (s *Store) RecordDecision(ctx context.Context, d domain.PolicyDecision) error {
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, d.OrgID); err != nil {
			return err
		}
		// Solo si Resolve comprobó que el rol se PUEDE asumir; si no, el
		// aislamiento lo sostiene la RLS forzada (ver platform/pkg/pg.AppRole).
		if s.rol.Activo() {
			if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+appRole); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO policy_decisions
			    (org_id, instance_id, policy_id, policy_name, action, request_id, host, method, occurred_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			d.OrgID, nullUUID(d.InstanceID), nullUUID(d.PolicyID), d.PolicyName,
			string(d.Action), d.RequestID, d.Host, d.Method, d.OccurredAt)
		return err
	})
	if err != nil {
		return fmt.Errorf("store: registrar decisión de política: %w", err)
	}
	return nil
}

// --- CRUD de políticas (desde peticiones admin, tenant del contexto) -----------

const policyCols = `id, org_id, name, condition, action, enabled, created_at, updated_at`

func scanPolicy(row pgx.Row) (domain.Policy, error) {
	var p domain.Policy
	var action string
	if err := row.Scan(&p.ID, &p.OrgID, &p.Name, &p.Condition, &action, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return domain.Policy{}, err
	}
	p.Action = domain.PolicyAction(action)
	return p, nil
}

// ListPolicies devuelve las reglas de la org activa (habilitadas o no), más
// recientes primero, para el panel de gobernanza.
func (s *Store) ListPolicies(ctx context.Context) ([]domain.Policy, error) {
	out := make([]domain.Policy, 0, 16)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+policyCols+` FROM policies WHERE org_id = current_setting('app.tenant', true) ORDER BY created_at DESC, id DESC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			p, err := scanPolicy(rows)
			if err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar políticas: %w", err)
	}
	return out, nil
}

// CreatePolicy inserta una regla en la org activa y la devuelve.
func (s *Store) CreatePolicy(ctx context.Context, name string, condition json.RawMessage, action domain.PolicyAction, enabled bool) (*domain.Policy, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, err
	}
	var p domain.Policy
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		p, err = scanPolicy(tx.QueryRow(ctx, `
			INSERT INTO policies (org_id, name, condition, action, enabled)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING `+policyCols,
			t.Slug, name, coalesceJSON(condition), string(action), enabled))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("store: crear política: %w", err)
	}
	return &p, nil
}

// UpdatePolicy sustituye nombre/condición/acción/enabled de una regla y la
// devuelve. ErrNotFound si no existe en la org.
func (s *Store) UpdatePolicy(ctx context.Context, id, name string, condition json.RawMessage, action domain.PolicyAction, enabled bool) (*domain.Policy, error) {
	var p domain.Policy
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		p, err = scanPolicy(tx.QueryRow(ctx, `
			UPDATE policies
			   SET name = $2, condition = $3, action = $4, enabled = $5, updated_at = now()
			 WHERE org_id = current_setting('app.tenant', true) AND id = $1
			RETURNING `+policyCols,
			id, name, coalesceJSON(condition), string(action), enabled))
		return err
	})
	if isNoRows(err) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: actualizar política: %w", err)
	}
	return &p, nil
}

// DeletePolicy borra una regla. ErrNotFound si no existe en la org.
func (s *Store) DeletePolicy(ctx context.Context, id string) error {
	return s.execAffecting(ctx, "borrar política", `DELETE FROM policies WHERE org_id = current_setting('app.tenant', true) AND id = $1`, id)
}

// --- Visor del registro de egress (egress_audit) -------------------------------

// EgressRow es una fila del registro de egress para lectura admin.
type EgressRow struct {
	ID         int64
	InstanceID string
	Host       string
	Method     string
	Status     int
	ReqBytes   int64
	RespBytes  int64
	LatencyMS  int64
	Auth       string
	Err        string
	OccurredAt time.Time
}

// EgressFilter acota el registro de egress. Todos opcionales; la org la fija el
// llamante (aislamiento por tenant), no va aquí.
type EgressFilter struct {
	Host   string     // prefijo de host (LIKE 'x%')
	Status *int       // status HTTP exacto; nil = cualquiera (0 = errores de red)
	Since  *time.Time // occurred_at >= Since
	Until  *time.Time // occurred_at <  Until
}

// Cursor es la posición keyset (occurred_at, id) DESC compartida por los visores.
type Cursor struct {
	OccurredAt time.Time
	ID         int64
}

const egressSelect = `SELECT id, coalesce(instance_id::text,''), host, method, status,
	req_bytes, resp_bytes, latency_ms, auth, err, occurred_at FROM egress_audit WHERE org_id = current_setting('app.tenant', true)`

func scanEgress(row pgx.Row) (EgressRow, error) {
	var e EgressRow
	err := row.Scan(&e.ID, &e.InstanceID, &e.Host, &e.Method, &e.Status,
		&e.ReqBytes, &e.RespBytes, &e.LatencyMS, &e.Auth, &e.Err, &e.OccurredAt)
	return e, err
}

// egressWhere construye los filtros parametrizados (filtros + keyset) y sus args,
// cada uno con su AND: la organización ya va en egressSelect (NEG-22).
func egressWhere(f EgressFilter, after *Cursor) (string, []any) {
	args := []any{}
	var b strings.Builder
	ph := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if f.Host != "" {
		b.WriteString(` AND host LIKE ` + ph(escapeLikePrefix(f.Host)+"%") + ` ESCAPE '\'`)
	}
	if f.Status != nil {
		b.WriteString(` AND status = ` + ph(*f.Status))
	}
	if f.Since != nil {
		b.WriteString(` AND occurred_at >= ` + ph(*f.Since))
	}
	if f.Until != nil {
		b.WriteString(` AND occurred_at < ` + ph(*f.Until))
	}
	if after != nil {
		b.WriteString(` AND (occurred_at, id) < (` + ph(after.OccurredAt) + `, ` + ph(after.ID) + `)`)
	}
	return b.String(), args
}

// ListEgressPage devuelve una página del registro de egress de la org activa con
// keyset (occurred_at, id) DESC. more=true si queda otra página (consulta limit+1).
func (s *Store) ListEgressPage(ctx context.Context, f EgressFilter, after *Cursor, limit int) ([]EgressRow, bool, error) {
	limit = clampLimit(limit)
	out := make([]EgressRow, 0, limit)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		where, args := egressWhere(f, after)
		args = append(args, limit+1)
		rows, err := tx.Query(ctx, egressSelect+where+` ORDER BY occurred_at DESC, id DESC LIMIT $`+strconv.Itoa(len(args)), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			e, err := scanEgress(rows)
			if err != nil {
				return err
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, false, fmt.Errorf("store: listar egress: %w", err)
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

// ExportEgress escribe el registro de egress filtrado de la org como CSV en w,
// en streaming fila a fila (no materializa la tabla). Acotado por governanceExportCap.
func (s *Store) ExportEgress(ctx context.Context, f EgressFilter, w io.Writer) error {
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		where, args := egressWhere(f, nil)
		args = append(args, governanceExportCap)
		rows, err := tx.Query(ctx, egressSelect+where+` ORDER BY occurred_at DESC, id DESC LIMIT $`+strconv.Itoa(len(args)), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		cw := csv.NewWriter(w)
		if err := cw.Write([]string{"occurred_at", "host", "method", "status", "req_bytes", "resp_bytes", "latency_ms", "auth", "error", "instance_id"}); err != nil {
			return err
		}
		for rows.Next() {
			e, err := scanEgress(rows)
			if err != nil {
				return err
			}
			if err := cw.Write([]string{
				e.OccurredAt.UTC().Format(time.RFC3339), validation.CSVSafe(e.Host), validation.CSVSafe(e.Method),
				strconv.Itoa(e.Status), strconv.FormatInt(e.ReqBytes, 10), strconv.FormatInt(e.RespBytes, 10),
				strconv.FormatInt(e.LatencyMS, 10), validation.CSVSafe(e.Auth), validation.CSVSafe(e.Err), e.InstanceID,
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
		return fmt.Errorf("store: exportar egress: %w", err)
	}
	return nil
}

// --- Visor de decisiones de política (policy_decisions) ------------------------

// DecisionRow es una decisión (block/warn) para lectura admin.
type DecisionRow struct {
	ID         int64
	InstanceID string
	PolicyID   string
	PolicyName string
	Action     string
	RequestID  string
	Host       string
	Method     string
	OccurredAt time.Time
}

const decisionSelect = `SELECT id, coalesce(instance_id::text,''), coalesce(policy_id::text,''),
	policy_name, action, request_id, host, method, occurred_at FROM policy_decisions WHERE org_id = current_setting('app.tenant', true)`

// ListDecisionsPage devuelve una página de decisiones de la org activa (keyset
// occurred_at,id DESC), opcionalmente filtrada por acción (warn|block).
func (s *Store) ListDecisionsPage(ctx context.Context, action string, after *Cursor, limit int) ([]DecisionRow, bool, error) {
	limit = clampLimit(limit)
	out := make([]DecisionRow, 0, limit)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		args := []any{}
		var b strings.Builder
		b.WriteString(decisionSelect) // la organización va en decisionSelect, y la RLS detrás.
		ph := func(v any) string {
			args = append(args, v)
			return "$" + strconv.Itoa(len(args))
		}
		if action != "" {
			b.WriteString(` AND action = ` + ph(action))
		}
		if after != nil {
			b.WriteString(` AND (occurred_at, id) < (` + ph(after.OccurredAt) + `, ` + ph(after.ID) + `)`)
		}
		args = append(args, limit+1)
		b.WriteString(` ORDER BY occurred_at DESC, id DESC LIMIT $` + strconv.Itoa(len(args)))
		rows, err := tx.Query(ctx, b.String(), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d DecisionRow
			if err := rows.Scan(&d.ID, &d.InstanceID, &d.PolicyID, &d.PolicyName,
				&d.Action, &d.RequestID, &d.Host, &d.Method, &d.OccurredAt); err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, false, fmt.Errorf("store: listar decisiones: %w", err)
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

// PrunePolicyDecisions borra las decisiones de la org activa anteriores a `before`
// (retención). Devuelve cuántas borró.
//
// ponytail: retención POR ORG bajo RLS (testeable, consistente con el aislamiento).
// El barrido periódico CROSS-org es un job de servicio con BYPASSRLS o un CronJob
// por org — mismo diferido que el tick de sync (ver cmd/integrations/main.go).
func (s *Store) PrunePolicyDecisions(ctx context.Context, before time.Time) (int64, error) {
	var n int64
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM policy_decisions WHERE org_id = current_setting('app.tenant', true) AND occurred_at < $1`, before)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("store: purgar decisiones: %w", err)
	}
	return n, nil
}

// --- helpers de gobernanza -----------------------------------------------------

// clampLimit acota el tamaño de página (1..500, 100 por defecto).
func clampLimit(limit int) int {
	if limit <= 0 || limit > 500 {
		return 100
	}
	return limit
}

// EncodeCursor codifica la posición keyset como token opaco URL-safe.
func EncodeCursor(ts time.Time, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(ts.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(id, 10)))
}

// DecodeCursor invierte EncodeCursor. Token vacío ⇒ nil (primera página); un token
// corrupto es entrada inválida del cliente (ErrInvalidInput), nunca un 500.
func DecodeCursor(token string) (*Cursor, error) {
	if token == "" {
		return nil, nil //nolint:nilnil // contrato: token vacío ⇒ primera página.
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, domain.ErrInvalidInput
	}
	tsRaw, idRaw, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, domain.ErrInvalidInput
	}
	ts, err := time.Parse(time.RFC3339Nano, tsRaw)
	if err != nil {
		return nil, domain.ErrInvalidInput
	}
	id, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil {
		return nil, domain.ErrInvalidInput
	}
	return &Cursor{OccurredAt: ts, ID: id}, nil
}

// escapeLikePrefix escapa los metacaracteres LIKE (\ % _) para usar la entrada como
// prefijo literal seguido de '%' (con ESCAPE '\' en la consulta).
func escapeLikePrefix(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

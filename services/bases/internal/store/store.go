// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store es la persistencia de Sytue Bases sobre PostgreSQL (pgx). Una
// petición, una transacción (CONVENTIONS §13.3): dentro de la MISMA transacción se
// fija el tenant, se resuelve el rol del actor y se hace el trabajo, de modo que la
// autorización no puede quedar vieja entre la comprobación y la escritura.
//
// Tres capas de defensa, y los tests quitan cada una para ver que falla:
//  1. La RLS por organización (`app.tenant`), forzada en todas las tablas.
//  2. Las claves foráneas compuestas (tenant, id): un id ajeno no se puede enlazar.
//  3. `autorizar`: el rol del actor sobre la base/tabla que toca (domain.Permite).
//
// Consultas siempre parametrizadas; la configuración de una vista nunca se
// concatena (ver filtros.go).
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// Store agrupa el acceso a datos de Sytue Bases.
type Store struct {
	pool *pg.Pool
	// externos recuerda, por persona, en qué organizaciones es invitada (externos.go).
	externos recuerdoAnfitrionas
}

// New construye un Store sobre el pool dado.
func New(pool *pg.Pool) *Store { return &Store{pool: pool} }

// Healthz delega en el pool (para /readyz).
func (s *Store) Healthz(ctx context.Context) error { return s.pool.Healthz(ctx) }

// en abre la transacción de la petición con el tenant del actor fijado.
func (s *Store) en(ctx context.Context, a domain.Actor, fn func(tx pgx.Tx) error) error {
	if s.pool == nil || a.Tenant == "" || a.Sub == "" {
		return domain.ErrNoExiste
	}
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, a.Tenant); err != nil {
			return err
		}
		return fn(tx)
	})
	// Las claves de los enlaces se comprueban al confirmar (diferidas, ver la
	// migración): una violación de clave foránea es siempre «lo referencia otro registro».
	if esFK(err) {
		return domain.ErrReferenciado
	}
	return err
}

// esFK indica un 23503 (violación de clave foránea): lo que enlaza todavía existe.
func esFK(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23503"
}

// aplicaAlActor es el filtro de `base_permissions` (alias p) con las entradas que
// aplican a un actor: a su nombre, a uno de sus grupos o a toda su organización; y, si
// viene de otra organización, SOLO las `external` a su nombre. Los parámetros son
// SIEMPRE $1 = tenant, $2 = sub, $3 = grupos y $4 = externo (ver argsDelActor).
const aplicaAlActor = `(NOT $4::boolean AND (p.principal_kind = 'user' AND p.principal_id = $2
	OR p.principal_kind = 'tenant' AND p.principal_id = $1
	OR p.principal_kind = 'group' AND p.principal_id = ANY($3::text[]))
	OR $4::boolean AND p.principal_kind = 'external' AND p.principal_id = $2)`

// argsDelActor son los cuatro primeros parámetros de una consulta con aplicaAlActor.
func argsDelActor(a domain.Actor) []any {
	grupos := a.Groups
	if len(grupos) > domain.MaxGruposDelActor {
		grupos = grupos[:domain.MaxGruposDelActor]
	}
	if grupos == nil || a.Externo {
		grupos = []string{}
	}
	return []any{a.Tenant, a.Sub, grupos, a.Externo}
}

// cargarACL lee el propietario de la base y las entradas de ACL que aplican al
// actor (suyas, de sus grupos o de toda su organización). ErrNoExiste si la base no existe.
func cargarACL(ctx context.Context, tx pgx.Tx, a domain.Actor, baseID string) (string, []domain.EntradaACL, error) {
	var owner string
	if err := tx.QueryRow(ctx, `SELECT owner_sub FROM bases WHERE id = $1 AND tenant = $2`, baseID, a.Tenant).Scan(&owner); err != nil {
		return "", nil, errNoFilas(err)
	}
	if owner == a.Sub && !a.Externo {
		return owner, nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT coalesce(p.table_id::text, ''), p.principal_kind, p.principal_id, p.role
		  FROM base_permissions p
		 WHERE p.tenant = $1 AND p.base_id = $5 AND `+aplicaAlActor+`
		 LIMIT $6`, append(argsDelActor(a), baseID, domain.MaxPermisosPorBase)...)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var entradas []domain.EntradaACL
	for rows.Next() {
		var e domain.EntradaACL
		if err := rows.Scan(&e.TableID, &e.PrincipalKind, &e.PrincipalID, &e.Role); err != nil {
			return "", nil, err
		}
		entradas = append(entradas, e)
	}
	return owner, entradas, rows.Err()
}

// rolEn resuelve el rol efectivo del actor sobre una tabla de una base (tabla ""
// = nivel base). ErrNoExiste si no hay base o no hay NINGÚN acceso: no se revela
// que existe.
func rolEn(ctx context.Context, tx pgx.Tx, a domain.Actor, baseID, tabla string) (domain.Rol, error) {
	owner, entradas, err := cargarACL(ctx, tx, a, baseID)
	if err != nil {
		return "", err
	}
	rol, ok := domain.ResolverRol(owner, entradas, a, tabla)
	if !ok {
		return "", domain.ErrNoExiste
	}
	return rol, nil
}

// autorizar exige que el actor pueda hacer `acc` sobre la base (y la tabla, si se
// indica). Sin acceso, 404; con acceso pero insuficiente, 403.
func autorizar(ctx context.Context, tx pgx.Tx, a domain.Actor, baseID, tabla string, acc domain.Accion) error {
	rol, err := rolEn(ctx, tx, a, baseID, tabla)
	if err != nil {
		return err
	}
	if !domain.Permite(rol, acc) {
		return domain.ErrSinPermiso
	}
	return nil
}

// baseDeTabla resuelve la base de una tabla. Un id de tabla de otro tenant no
// existe para la RLS, y uno de otra base no da acceso a esta: el rol se calcula
// siempre sobre la base REAL de la tabla, nunca sobre la que diga el cliente.
func baseDeTabla(ctx context.Context, tx pgx.Tx, a domain.Actor, tabla string) (string, error) {
	var base string
	err := tx.QueryRow(ctx, `SELECT base_id::text FROM base_tables WHERE id = $1 AND tenant = $2`, tabla, a.Tenant).Scan(&base)
	return base, errNoFilas(err)
}

// autorizarTabla es autorizar partiendo de una tabla; devuelve su base.
func autorizarTabla(ctx context.Context, tx pgx.Tx, a domain.Actor, tabla string, acc domain.Accion) (string, error) {
	base, err := baseDeTabla(ctx, tx, a, tabla)
	if err != nil {
		return "", err
	}
	return base, autorizar(ctx, tx, a, base, tabla, acc)
}

func errNoFilas(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNoExiste
	}
	return err
}

// auditar escribe una entrada de la traza dentro de la transacción de la operación.
func auditar(ctx context.Context, tx pgx.Tx, a domain.Actor, baseID string, tabla, registro, campo *string, accion string, antes, despues json.RawMessage) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO base_audit (tenant, base_id, table_id, record_id, field_id, actor, action, before, after)
		VALUES ($1, $2, $3::uuid, $4::uuid, $5::uuid, $6, $7, $8::jsonb, $9::jsonb)`,
		a.Tenant, baseID, tabla, registro, campo, a.Sub, accion, nilSiVacio(domain.Acotar(antes)), nilSiVacio(domain.Acotar(despues)))
	if err != nil {
		return fmt.Errorf("auditoría: %w", err)
	}
	return nil
}

func nilSiVacio(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return []byte(b)
}

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func jsonDe(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// ---------------------------------------------------------------------------
// Bases
// ---------------------------------------------------------------------------

// CrearBase crea una base; quien la crea es su propietario. Cualquier miembro de la
// organización puede crear (la verja de organización la pone el router).
func (s *Store) CrearBase(ctx context.Context, a domain.Actor, nombre, descripcion string) (domain.Base, error) {
	nombre, err := domain.LimpiarNombre(nombre)
	if err != nil {
		return domain.Base{}, err
	}
	descripcion = strings.TrimSpace(descripcion)
	if len([]rune(descripcion)) > 1000 {
		return domain.Base{}, domain.ErrInvalido
	}
	var b domain.Base
	err = s.en(ctx, a, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM bases WHERE tenant = $1`, a.Tenant).Scan(&n); err != nil {
			return err
		}
		if n >= domain.MaxBasesPorOrg {
			return domain.ErrLimite
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO bases (tenant, name, description, owner_sub) VALUES ($1, $2, $3, $4)
			RETURNING id, name, description, owner_sub, created_at, updated_at`,
			a.Tenant, nombre, descripcion, a.Sub).
			Scan(&b.ID, &b.Name, &b.Description, &b.OwnerSub, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return err
		}
		b.Role = domain.RolPropietario
		return auditar(ctx, tx, a, b.ID, nil, nil, nil, "base.create", nil, jsonDe(map[string]string{"name": nombre}))
	})
	return b, err
}

// ListarBases devuelve las bases a las que el actor tiene acceso: las de su
// organización (propias, o con alguna entrada de ACL que le aplique) y, detrás, las que
// le han compartido desde OTRAS organizaciones, con `host_org`. Role vacío = solo accede
// a alguna de sus tablas.
func (s *Store) ListarBases(ctx context.Context, a domain.Actor) ([]domain.Base, error) {
	out, err := s.listarBasesEn(ctx, a)
	if err != nil || a.Externo {
		return out, err
	}
	fuera, err := s.basesDeFuera(ctx, a)
	return append(out, fuera...), err
}

// listarBasesEn son las bases de UNA organización (a.Tenant) que el actor puede ver.
func (s *Store) listarBasesEn(ctx context.Context, a domain.Actor) ([]domain.Base, error) {
	out := []domain.Base{}
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT p.base_id::text, coalesce(p.table_id::text, ''), p.principal_kind, p.principal_id, p.role
			  FROM base_permissions p
			 WHERE p.tenant = $1 AND `+aplicaAlActor+`
			 LIMIT 5000`, argsDelActor(a)...)
		if err != nil {
			return err
		}
		acl := map[string][]domain.EntradaACL{}
		var ids []string
		for rows.Next() {
			var base string
			var e domain.EntradaACL
			if err := rows.Scan(&base, &e.TableID, &e.PrincipalKind, &e.PrincipalID, &e.Role); err != nil {
				rows.Close()
				return err
			}
			if _, ok := acl[base]; !ok {
				ids = append(ids, base)
			}
			acl[base] = append(acl[base], e)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		rows, err = tx.Query(ctx, `
			SELECT id::text, name, description, owner_sub, created_at, updated_at
			  FROM bases WHERE tenant = $1 AND (owner_sub = $2 AND NOT $5::boolean OR id = ANY($3::uuid[]))
			 ORDER BY updated_at DESC LIMIT $4`, a.Tenant, a.Sub, ids, domain.MaxBasesPorOrg, a.Externo)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b domain.Base
			if err := rows.Scan(&b.ID, &b.Name, &b.Description, &b.OwnerSub, &b.CreatedAt, &b.UpdatedAt); err != nil {
				return err
			}
			b.Role, _ = domain.ResolverRol(b.OwnerSub, acl[b.ID], a, "")
			out = append(out, b)
		}
		return rows.Err()
	})
	return out, err
}

// BuscarBases es la búsqueda federada del Hub: las bases a las que el actor tiene acceso
// cuyo nombre o descripción contienen `q` (sin término, las más recientes). Pasa por
// ListarBases, así que la ACL es la misma que la de «Mis bases».
//
// ponytail: filtra en memoria sobre las ≤ 200 bases de la organización.
func (s *Store) BuscarBases(ctx context.Context, a domain.Actor, q string, limite int) ([]domain.Base, error) {
	if limite <= 0 || limite > 50 {
		limite = 20
	}
	todas, err := s.ListarBases(ctx, a)
	if err != nil {
		return nil, err
	}
	q = strings.TrimSpace(q)
	out := []domain.Base{}
	for _, b := range todas {
		if len(out) == limite {
			break
		}
		if buscar.Contiene(b.Name, q) || buscar.Contiene(b.Description, q) {
			out = append(out, b)
		}
	}
	return out, nil
}

// ObtenerBase devuelve una base con el rol del actor.
func (s *Store) ObtenerBase(ctx context.Context, a domain.Actor, id string) (domain.Base, error) {
	var b domain.Base
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		rol, err := rolEn(ctx, tx, a, id, "")
		if err != nil {
			return err
		}
		b.Role = rol
		return errNoFilas(tx.QueryRow(ctx, `
			SELECT id::text, name, description, owner_sub, created_at, updated_at FROM bases WHERE id = $1 AND tenant = $2`, id, a.Tenant).
			Scan(&b.ID, &b.Name, &b.Description, &b.OwnerSub, &b.CreatedAt, &b.UpdatedAt))
	})
	return b, err
}

// ActualizarBase cambia nombre y descripción (propietario).
func (s *Store) ActualizarBase(ctx context.Context, a domain.Actor, id, nombre, descripcion string) error {
	nombre, err := domain.LimpiarNombre(nombre)
	descripcion = strings.TrimSpace(descripcion)
	if err != nil || len([]rune(descripcion)) > 1000 {
		return domain.ErrInvalido
	}
	return s.en(ctx, a, func(tx pgx.Tx) error {
		if err := autorizar(ctx, tx, a, id, "", domain.AccBorrarBase); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bases SET name = $3, description = $4, updated_at = now() WHERE id = $1 AND tenant = $2`, id, a.Tenant, nombre, descripcion); err != nil {
			return err
		}
		return auditar(ctx, tx, a, id, nil, nil, nil, "base.update", nil, jsonDe(map[string]string{"name": nombre}))
	})
}

// BorrarBase borra la base y todo lo que cuelga de ella (propietario).
func (s *Store) BorrarBase(ctx context.Context, a domain.Actor, id string) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		if err := autorizar(ctx, tx, a, id, "", domain.AccBorrarBase); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM bases WHERE id = $1 AND tenant = $2`, id, a.Tenant); err != nil {
			return err
		}
		return auditar(ctx, tx, a, id, nil, nil, nil, "base.delete", nil, nil)
	})
}

// ---------------------------------------------------------------------------
// Permisos
// ---------------------------------------------------------------------------

// ListarPermisos devuelve la ACL de la base (propietario).
func (s *Store) ListarPermisos(ctx context.Context, a domain.Actor, base string) ([]domain.Permiso, error) {
	out := []domain.Permiso{}
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		if err := autorizar(ctx, tx, a, base, "", domain.AccPermisos); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id::text, coalesce(table_id::text, ''), principal_kind, principal_id, principal_label, role, created_by, created_at
			  FROM base_permissions WHERE base_id = $1 AND tenant = $2 ORDER BY created_at, id LIMIT $3`, base, a.Tenant, domain.MaxPermisosPorBase)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p domain.Permiso
			if err := rows.Scan(&p.ID, &p.TableID, &p.PrincipalKind, &p.PrincipalID, &p.PrincipalLabel, &p.Role, &p.CreatedBy, &p.CreatedAt); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// principalValido dice si (clase, id) es un principal que la ACL puede guardar: una
// persona o un grupo de la organización, o TODA la organización —solo la propia—.
func principalValido(a domain.Actor, clase, id string) bool {
	if id == "" || len(id) > 200 || strings.ContainsFunc(id, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return false
	}
	switch clase {
	case domain.PrincipalUsuario, domain.PrincipalGrupo, domain.PrincipalExterno:
		return true
	case domain.PrincipalOrganizacion:
		return id == a.Tenant
	}
	return false
}

// ConcederPermiso añade o cambia una entrada de la ACL (propietario). El
// propietario no se concede (es el de la base) y `tenant` solo puede ser la propia
// organización. Un grupo se guarda por su nombre: aplica a quien lo traiga en su token.
func (s *Store) ConcederPermiso(ctx context.Context, a domain.Actor, base string, p domain.Permiso) (domain.Permiso, error) {
	p.PrincipalLabel = strings.TrimSpace(p.PrincipalLabel)
	if !p.Role.Concedible() || !principalValido(a, p.PrincipalKind, p.PrincipalID) || len([]rune(p.PrincipalLabel)) > 200 {
		return p, domain.ErrInvalido
	}
	if p.PrincipalKind != domain.PrincipalUsuario && p.PrincipalKind != domain.PrincipalExterno {
		p.PrincipalLabel = ""
	}
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		if err := autorizar(ctx, tx, a, base, "", domain.AccPermisos); err != nil {
			return err
		}
		if p.TableID != "" {
			b, err := baseDeTabla(ctx, tx, a, p.TableID)
			if err != nil || b != base {
				return domain.ErrNoExiste
			}
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM base_permissions WHERE base_id = $1 AND tenant = $2`, base, a.Tenant).Scan(&n); err != nil {
			return err
		}
		if n >= domain.MaxPermisosPorBase {
			return domain.ErrLimite
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO base_permissions (tenant, base_id, table_id, principal_kind, principal_id, role, created_by, principal_label)
			VALUES ($1, $2, $3::uuid, $4, $5, $6, $7, $8)
			ON CONFLICT (base_id, coalesce(table_id, '00000000-0000-0000-0000-000000000000'::uuid), principal_kind, principal_id)
			DO UPDATE SET role = EXCLUDED.role,
			              principal_label = CASE WHEN EXCLUDED.principal_label = '' THEN base_permissions.principal_label ELSE EXCLUDED.principal_label END
			RETURNING id::text, created_by, created_at, principal_label`,
			a.Tenant, base, ptr(p.TableID), p.PrincipalKind, p.PrincipalID, p.Role, a.Sub, p.PrincipalLabel).Scan(&p.ID, &p.CreatedBy, &p.CreatedAt, &p.PrincipalLabel)
		if err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, ptr(p.TableID), nil, nil, "permission.grant", nil,
			jsonDe(map[string]string{"kind": p.PrincipalKind, "principal": p.PrincipalID, "role": string(p.Role)}))
	})
	if err == nil && p.PrincipalKind == domain.PrincipalExterno {
		s.externos.olvidar(p.PrincipalID)
	}
	return p, err
}

// RevocarPermiso quita una entrada de la ACL (propietario).
func (s *Store) RevocarPermiso(ctx context.Context, a domain.Actor, base, permiso string) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		if err := autorizar(ctx, tx, a, base, "", domain.AccPermisos); err != nil {
			return err
		}
		var kind, id, rol string
		err := tx.QueryRow(ctx, `DELETE FROM base_permissions WHERE id = $1 AND base_id = $2 AND tenant = $3 RETURNING principal_kind, principal_id, role`,
			permiso, base, a.Tenant).Scan(&kind, &id, &rol)
		if err != nil {
			return errNoFilas(err)
		}
		return auditar(ctx, tx, a, base, nil, nil, nil, "permission.revoke", jsonDe(map[string]string{"kind": kind, "principal": id, "role": rol}), nil)
	})
}

// TransferirBase da la propiedad de la base a `nuevo`, otra persona de la organización
// (propietario). Quien la tenía se queda como EDITOR —no pierde de golpe su trabajo,
// pero ya no decide esquema, permisos ni conectores— y la entrada que el nuevo
// propietario tuviera a su nombre sobra y se quita. Que `nuevo` es de la organización
// lo garantiza quien llama (la capa HTTP lo resuelve en Sytue ID con la sesión del
// propietario; nunca acepta un `sub` del cliente).
func (s *Store) TransferirBase(ctx context.Context, a domain.Actor, base, nuevo string) (nombre string, err error) {
	if nuevo == "" || len(nuevo) > 200 || nuevo == a.Sub {
		return "", domain.ErrInvalido
	}
	err = s.en(ctx, a, func(tx pgx.Tx) error {
		// La fila bloqueada serializa dos transferencias a la vez: la segunda ya no es propietaria.
		if err := tx.QueryRow(ctx, `SELECT name FROM bases WHERE id = $1 AND tenant = $2 FOR UPDATE`, base, a.Tenant).Scan(&nombre); err != nil {
			return errNoFilas(err)
		}
		if err := autorizar(ctx, tx, a, base, "", domain.AccTransferir); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bases SET owner_sub = $3, updated_at = now() WHERE id = $1 AND tenant = $2`, base, a.Tenant, nuevo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM base_permissions WHERE tenant = $1 AND base_id = $2 AND principal_kind = 'user' AND principal_id = $3`, a.Tenant, base, nuevo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO base_permissions (tenant, base_id, principal_kind, principal_id, role, created_by)
			VALUES ($1, $2, 'user', $3, 'editor', $3)
			ON CONFLICT (base_id, coalesce(table_id, '00000000-0000-0000-0000-000000000000'::uuid), principal_kind, principal_id)
			DO UPDATE SET role = 'editor'`, a.Tenant, base, a.Sub); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, nil, nil, nil, "base.transfer", jsonDe(map[string]string{"owner": a.Sub}), jsonDe(map[string]string{"owner": nuevo}))
	})
	return nombre, err
}

// ---------------------------------------------------------------------------
// Auditoría
// ---------------------------------------------------------------------------

// ListarAuditoria devuelve la traza de la base, de la más reciente a la más antigua,
// paginada por id (antes = 0 desde el final). Solo el propietario.
func (s *Store) ListarAuditoria(ctx context.Context, a domain.Actor, base string, antes int64, limite int) ([]domain.Entrada, error) {
	limite = min(max(limite, 1), 200)
	out := []domain.Entrada{}
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		if err := autorizar(ctx, tx, a, base, "", domain.AccAuditoria); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, coalesce(table_id::text, ''), coalesce(record_id::text, ''), coalesce(field_id::text, ''), actor, action, before, after, at
			  FROM base_audit WHERE tenant = $1 AND base_id = $2 AND ($3::bigint = 0 OR id < $3)
			 ORDER BY id DESC LIMIT $4`, a.Tenant, base, antes, limite)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e domain.Entrada
			if err := rows.Scan(&e.ID, &e.TableID, &e.RecordID, &e.FieldID, &e.Actor, &e.Action, &e.Before, &e.After, &e.At); err != nil {
				return err
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}

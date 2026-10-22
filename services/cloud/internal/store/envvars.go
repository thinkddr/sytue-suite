// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// EnvVarRecord son los datos para crear/actualizar una variable. Para secretos,
// ValueEnc lleva el valor ya cifrado (la capa de servicio lo cifra).
type EnvVarRecord struct {
	ProjectID  uuid.UUID
	Key        string
	ValuePlain string
	ValueEnc   []byte
	IsSecret   bool
	Scope      domain.EnvScope
}

// CloneEnvVars copia TODAS las variables de un proyecto a otro del mismo tenant
// y devuelve cuántas copió.
//
// Es un INSERT ... SELECT a propósito: los secretos viajan por su columna cifrada
// sin pasar por la memoria del proceso ni por el sobre de cifrado. Leerlos para
// volver a sellarlos daría el mismo resultado y añadiría una ventana en la que
// los valores en claro existen, para nada.
//
// El tenant sale del contexto y se usa en las DOS puntas de la consulta: no hay
// forma de copiar variables desde el proyecto de otra organización aunque se
// pasara su id (RLS lo cortaría igual, esto es defensa en profundidad).
//
// ON CONFLICT DO NOTHING: un destino que ya tenga esa clave conserva la suya. El
// clonado no pisa lo que alguien haya escrito.
func (s *Store) CloneEnvVars(ctx context.Context, from, to uuid.UUID) (int, error) {
	t := tenancy.MustFrom(ctx)
	n := 0
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO env_vars (tenant, project_id, key, value_plain, value_enc, is_secret, scope)
			SELECT tenant, $3, key, value_plain, value_enc, is_secret, scope
			  FROM env_vars WHERE tenant=$1 AND project_id=$2
			ON CONFLICT (tenant, project_id, key, scope) DO NOTHING`, t.Slug, from, to)
		if err != nil {
			return fmtErr("clonar variables de entorno", err)
		}
		n = int(tag.RowsAffected())
		return nil
	})
	return n, err
}

// UpsertEnvVar inserta o actualiza una variable (clave única por scope).
func (s *Store) UpsertEnvVar(ctx context.Context, rec EnvVarRecord) (domain.EnvVar, error) {
	t := tenancy.MustFrom(ctx)
	var v domain.EnvVar
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		v, e = upsertEnvVarTx(ctx, tx, t.Slug, rec)
		return e
	})
	return v, err
}

// upsertEnvVarTx es el cuerpo transaccional de UpsertEnvVar (ver createProjectTx:
// lo comparte el alta atómica desde plantilla).
func upsertEnvVarTx(ctx context.Context, tx pgx.Tx, tenant string, rec EnvVarRecord) (domain.EnvVar, error) {
	if rec.Scope == "" {
		rec.Scope = domain.ScopeAll
	}
	var ok bool
	if err := tx.QueryRow(ctx,
		`SELECT true FROM projects WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
		tenant, rec.ProjectID).Scan(&ok); err != nil {
		return domain.EnvVar{}, normalizeNoRows(err, domain.ErrNotFound)
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO env_vars (tenant, project_id, key, value_plain, value_enc, is_secret, scope)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant, project_id, key, scope) DO UPDATE SET
			value_plain = EXCLUDED.value_plain,
			value_enc   = EXCLUDED.value_enc,
			is_secret   = EXCLUDED.is_secret,
			updated_at  = now()
		RETURNING id, tenant, project_id, key, value_plain, is_secret, scope, updated_at`,
		tenant, rec.ProjectID, rec.Key, rec.ValuePlain, rec.ValueEnc, rec.IsSecret, string(rec.Scope))
	v, err := scanEnvVar(row, tenant)
	if checkViolation(err) {
		return domain.EnvVar{}, domain.ErrInvalidInput
	}
	return v, err
}

func scanEnvVar(row pgx.Row, t string) (domain.EnvVar, error) {
	var v domain.EnvVar
	if err := row.Scan(&v.ID, &v.Tenant, &v.ProjectID, &v.Key, &v.Value,
		&v.IsSecret, &v.Scope, &v.UpdatedAt); err != nil {
		return domain.EnvVar{}, err
	}
	v.Tenant = t
	if v.IsSecret {
		v.Value = "" // nunca devolver el valor de un secreto
	}
	return v, nil
}

// ListEnvVars lista las variables de un proyecto (sin valores de secretos).
func (s *Store) ListEnvVars(ctx context.Context, projectID uuid.UUID) ([]domain.EnvVar, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.EnvVar
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, project_id, key, value_plain, is_secret, scope, updated_at
			FROM env_vars WHERE tenant=$1 AND project_id=$2 ORDER BY key, scope`, t.Slug, projectID)
		if err != nil {
			return fmtErr("listar env vars", err)
		}
		defer rows.Close()
		for rows.Next() {
			var v domain.EnvVar
			if err := rows.Scan(&v.ID, &v.Tenant, &v.ProjectID, &v.Key, &v.Value,
				&v.IsSecret, &v.Scope, &v.UpdatedAt); err != nil {
				return err
			}
			v.Tenant = t.Slug
			if v.IsSecret {
				v.Value = ""
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

// EnvVarScope devuelve el ámbito de una variable del tenant activo.
func (s *Store) EnvVarScope(ctx context.Context, id uuid.UUID) (domain.EnvScope, error) {
	t := tenancy.MustFrom(ctx)
	var scope domain.EnvScope
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT scope FROM env_vars WHERE tenant=$1 AND id=$2`, t.Slug, id).Scan(&scope)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return scope, err
}

// DeleteEnvVar borra una variable por ID.
func (s *Store) DeleteEnvVar(ctx context.Context, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM env_vars WHERE tenant=$1 AND id=$2`, t.Slug, id)
		if err != nil {
			return fmtErr("borrar env var", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// ResolvedEnvVar es una variable con su valor en claro (secretos descifrados),
// para inyección en un deployment. Solo la usa el controller.
type ResolvedEnvVar struct {
	Key      string
	Value    string // ya en claro (para variables no secretas)
	ValueEnc []byte // valor cifrado si IsSecret (lo descifra el controller)
	IsSecret bool
	Scope    domain.EnvScope
}

// ResolveEnvForDeploy devuelve las variables aplicables a un target (production
// o preview), incluyendo el value_enc de los secretos para que el controller los
// descifre e inyecte. Cross-tenant (lo invoca el controller con el tenant).
//
// Qué recibe cada destino:
//
//   - PRODUCCIÓN: lo de ámbito 'all' y lo de 'production'.
//   - VISTA PREVIA: lo de ámbito 'preview', y de 'all' solo lo que NO es secreto.
//
// La asimetría es la decisión del dueño del 2026-10-06 («sin producción»): un
// secreto de ámbito 'all' ES un secreto de producción —la clave de la pasarela
// de pago, la del proveedor de correo— y una vista previa es el código de una
// rama que nadie ha revisado todavía. Si una vista previa necesita un secreto,
// se le da uno suyo con ámbito 'preview'; lo que no se hace es prestarle el de
// producción (CLOUD-01). La configuración que no es secreta sí se comparte: no
// protege nada y sin ella casi ninguna app arranca.
//
// El ORDEN importa: primero 'all', después lo específico del destino. Quien
// recorre el resultado se queda con el último valor de cada clave, así que el
// valor propio del destino gana al compartido. Sin `ORDER BY` ganaba el que la
// base devolviera último.
func (s *Store) ResolveEnvForDeploy(ctx context.Context, tenant string, projectID uuid.UUID, target domain.DeployTarget) ([]ResolvedEnvVar, error) {
	scope := domain.ScopeProduction
	if target == domain.TargetPreview {
		scope = domain.ScopePreview
	}
	var out []ResolvedEnvVar
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT key, value_plain, value_enc, is_secret, scope
			FROM env_vars
			WHERE tenant=$1 AND project_id=$2
			  AND (scope = $3 OR (scope = 'all' AND ($3 <> 'preview' OR NOT is_secret)))
			ORDER BY (scope <> 'all'), key`,
			tenant, projectID, string(scope))
		if err != nil {
			return fmtErr("resolver env de deploy", err)
		}
		defer rows.Close()
		for rows.Next() {
			var v ResolvedEnvVar
			if err := rows.Scan(&v.Key, &v.Value, &v.ValueEnc, &v.IsSecret, &v.Scope); err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

// PreviewWithheld es lo que una vista previa NO recibe de producción y para lo
// que el proyecto no ha definido un equivalente propio.
type PreviewWithheld struct {
	// SecretKeys son las claves de los secretos de producción (ámbito 'all' o
	// 'production') sin un valor de ámbito 'preview'. Solo los nombres.
	SecretKeys []string
	// ServiceKinds son los tipos de servicio gestionado que producción tiene
	// listos y para los que no hay ninguno de vista previa.
	ServiceKinds []string
}

// Empty dice si no falta nada.
func (w PreviewWithheld) Empty() bool {
	return len(w.SecretKeys) == 0 && len(w.ServiceKinds) == 0
}

// PreviewWithheld calcula lo que a una vista previa del proyecto le va a faltar
// por no heredar lo de producción. Existe para poder DECIRLO: una app que no
// arranca sin su clave tiene que encontrarse un mensaje que nombre la clave, no
// un despliegue colgado en «desplegando» sin explicación. Cross-tenant.
func (s *Store) PreviewWithheld(ctx context.Context, tenant string, projectID uuid.UUID) (PreviewWithheld, error) {
	var out PreviewWithheld
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		// Las dos consultas devuelven una sola columna de texto.
		texto := func(sql string) ([]string, error) {
			rows, err := tx.Query(ctx, sql, tenant, projectID)
			if err != nil {
				return nil, err
			}
			return pgx.CollectRows(rows, pgx.RowTo[string])
		}
		keys, err := texto(`
			SELECT DISTINCT e.key FROM env_vars e
			WHERE e.tenant=$1 AND e.project_id=$2 AND e.is_secret AND e.scope IN ('all', 'production')
			  AND NOT EXISTS (
			      SELECT 1 FROM env_vars p
			      WHERE p.tenant=e.tenant AND p.project_id=e.project_id AND p.key=e.key AND p.scope='preview')
			ORDER BY e.key`)
		if err != nil {
			return fmtErr("secretos sin valor de vista previa", err)
		}
		kinds, err := texto(`
			SELECT DISTINCT m.kind FROM managed_services m
			WHERE m.tenant=$1 AND m.project_id=$2 AND m.target='production'
			  AND m.status='ready' AND m.deleted_at IS NULL AND m.creds_enc IS NOT NULL
			  AND NOT EXISTS (
			      SELECT 1 FROM managed_services v
			      WHERE v.tenant=m.tenant AND v.project_id=m.project_id AND v.kind=m.kind
			        AND v.target='preview' AND v.deleted_at IS NULL)
			ORDER BY m.kind`)
		if err != nil {
			return fmtErr("servicios sin equivalente de vista previa", err)
		}
		out = PreviewWithheld{SecretKeys: keys, ServiceKinds: kinds}
		return nil
	})
	return out, err
}

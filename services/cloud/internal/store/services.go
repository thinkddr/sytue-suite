// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

func scanManagedService(row pgx.Row, t string) (domain.ManagedService, error) {
	var m domain.ManagedService
	var cfg []byte
	if err := row.Scan(&m.ID, &m.Tenant, &m.ProjectID, &m.Kind, &m.Name, &m.Status,
		&cfg, &m.CreatedAt, &m.UpdatedAt, &m.Target); err != nil {
		return domain.ManagedService{}, err
	}
	// En contextos tenant-scoped se pasa el slug para reforzar; en listados
	// cross-tenant se pasa "" y se conserva el valor real de la columna.
	if t != "" {
		m.Tenant = t
	}
	if len(cfg) > 0 {
		_ = json.Unmarshal(cfg, &m.Config)
	}
	return m, nil
}

const managedCols = `id, tenant, project_id, kind, name, status, config_json, created_at, updated_at, target`

// NewServiceRecord son los datos para aprovisionar un servicio gestionado.
type NewServiceRecord struct {
	ProjectID uuid.UUID
	Kind      domain.ServiceKind
	Name      string
	Config    map[string]string
	// Target dice a qué despliegues entrega sus credenciales el servicio:
	// producción (vacío o 'production') o vistas previas ('preview').
	Target domain.DeployTarget
}

// CountManagedServices cuenta los servicios gestionados vivos del tenant.
func (s *Store) CountManagedServices(ctx context.Context) (int, error) {
	t := tenancy.MustFrom(ctx)
	var n int
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM managed_services WHERE tenant=$1 AND deleted_at IS NULL`,
			t.Slug).Scan(&n)
	})
	return n, err
}

// ProvisionService crea un servicio gestionado en estado 'provisioning' (lo
// completa el controller, que rellena config_json y creds_enc).
func (s *Store) ProvisionService(ctx context.Context, rec NewServiceRecord, maxManaged int) (domain.ManagedService, error) {
	t := tenancy.MustFrom(ctx)
	var m domain.ManagedService
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		m, e = provisionServiceTx(ctx, tx, t.Slug, rec, maxManaged)
		return e
	})
	return m, err
}

// provisionServiceTx es el cuerpo transaccional de ProvisionService (ver
// createProjectTx: lo comparte el alta atómica desde plantilla). Cuenta los
// servicios DENTRO de la transacción, así que un alta que aprovisiona varios
// también topa con la cuota del plan.
func provisionServiceTx(
	ctx context.Context, tx pgx.Tx, tenant string, rec NewServiceRecord, maxManaged int,
) (domain.ManagedService, error) {
	cfg, _ := json.Marshal(rec.Config)
	if len(cfg) == 0 {
		cfg = []byte(`{}`)
	}
	if maxManaged > 0 {
		var count int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM managed_services WHERE tenant=$1 AND deleted_at IS NULL`,
			tenant).Scan(&count); err != nil {
			return domain.ManagedService{}, fmtErr("contar servicios gestionados", err)
		}
		if count >= maxManaged {
			return domain.ManagedService{}, domain.ErrQuotaExceeded
		}
	}
	var ok bool
	if err := tx.QueryRow(ctx,
		`SELECT true FROM projects WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
		tenant, rec.ProjectID).Scan(&ok); err != nil {
		return domain.ManagedService{}, normalizeNoRows(err, domain.ErrNotFound)
	}
	target := rec.Target
	if target == "" {
		target = domain.TargetProduction
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO managed_services (tenant, project_id, kind, name, config_json, status, target)
		VALUES ($1, $2, $3, $4, $5, 'provisioning', $6)
		RETURNING `+managedCols,
		tenant, rec.ProjectID, string(rec.Kind), rec.Name, cfg, string(target))
	m, err := scanManagedService(row, tenant)
	switch {
	case uniqueViolation(err):
		return domain.ManagedService{}, domain.ErrConflict
	case checkViolation(err):
		return domain.ManagedService{}, domain.ErrInvalidInput
	case err != nil:
		return domain.ManagedService{}, err
	}
	return m, nil
}

// GetService devuelve un servicio gestionado por ID.
func (s *Store) GetService(ctx context.Context, id uuid.UUID) (domain.ManagedService, error) {
	t := tenancy.MustFrom(ctx)
	var m domain.ManagedService
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+managedCols+` FROM managed_services WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, id)
		var e error
		m, e = scanManagedService(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ManagedService{}, domain.ErrNotFound
	}
	return m, err
}

// ListServices lista los servicios gestionados de un proyecto.
func (s *Store) ListServices(ctx context.Context, projectID uuid.UUID) ([]domain.ManagedService, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.ManagedService
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+managedCols+` FROM managed_services
			 WHERE tenant=$1 AND project_id=$2 AND deleted_at IS NULL ORDER BY created_at`,
			t.Slug, projectID)
		if err != nil {
			return fmtErr("listar servicios", err)
		}
		defer rows.Close()
		for rows.Next() {
			m, err := scanManagedService(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

// ReprovisionServices devuelve servicios YA aprovisionados al estado
// 'provisioning' para que el controller vuelva a pasar por ellos.
//
// Existe por la asimetría que se lleva por delante cualquier cambio de
// aprovisionamiento: `reconcileServices` solo llama a Provision en estado
// 'provisioning', así que un servicio 'ready' NO SE RECONCILIA JAMÁS. Lo que
// cambia en el entorno de un servicio ya levantado —que aparezca la app con la
// que se integra, o que desaparezca— no le llega nunca si nadie lo devuelve a la
// cola. Es idempotente por construcción: provisionApp reutiliza los secretos
// generados y hace upsert del resto.
//
// Los que están en 'deleting' se quedan fuera: resucitarlos a medio borrar
// dejaría recursos huérfanos que ya nadie retira.
func (s *Store) ReprovisionServices(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE managed_services SET status='provisioning', updated_at=now()
			 WHERE tenant=$1 AND id = ANY($2) AND status <> 'deleting' AND deleted_at IS NULL`,
			t.Slug, ids)
		if err != nil {
			return fmtErr("reencolar servicios para reaprovisionar", err)
		}
		return nil
	})
}

// UpdateServiceConfig cambia el nombre y/o mezcla claves en la configuración de
// un servicio gestionado, y lo devuelve a la cola de aprovisionamiento para que
// el cambio llegue al clúster.
//
// # Mezcla, no reemplazo
//
// `config_json` lleva mezcladas dos cosas: lo que pidió el cliente (schedule,
// imagen, comando) y lo que ESCRIBIÓ el aprovisionamiento al terminar (host,
// bucket, url pública, credenciales no sensibles). Sustituir el objeto entero por
// lo que manda el cliente borraría la segunda mitad y dejaría al panel sin poder
// enseñar a dónde responde una función. Por eso `||`, que en jsonb es «mezcla con
// prioridad a la derecha».
//
// # Reencolar es parte de la operación, no un extra
//
// `reconcileServices` solo aprovisiona lo que está en `provisioning`; sin volver
// a ese estado, el cambio se guardaría en la base y NUNCA llegaría al CronJob ni
// al Deployment. Por eso va en la MISMA transacción: guardar y no reencolar es el
// caso que se lee como «no funciona» sin que nada falle.
func (s *Store) UpdateServiceConfig(
	ctx context.Context, id uuid.UUID, name *string, config map[string]string,
) (domain.ManagedService, error) {
	t := tenancy.MustFrom(ctx)
	// Un mapa nil se serializa como `null`, y `config_json || 'null'::jsonb` es un
	// error de tipos en Postgres: la petición que solo cambia el NOMBRE llegaría
	// aquí con el mapa vacío y reventaría. Se normaliza al objeto vacío, que es
	// además lo que significa «no toco ninguna clave».
	if config == nil {
		config = map[string]string{}
	}
	patch, err := json.Marshal(config)
	if err != nil {
		return domain.ManagedService{}, err
	}
	var m domain.ManagedService
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE managed_services SET
				name        = COALESCE($3, name),
				config_json = COALESCE(config_json, '{}'::jsonb) || $4::jsonb,
				status      = 'provisioning',
				updated_at  = now()
			WHERE tenant=$1 AND id=$2 AND status <> 'deleting' AND deleted_at IS NULL
			RETURNING `+managedCols,
			t.Slug, id, name, patch)
		var e error
		m, e = scanManagedService(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ManagedService{}, domain.ErrNotFound
	}
	return m, err
}

// DeleteService marca un servicio gestionado para eliminación (lo retira el
// controller, liberando recursos: drop de DB, borrado de bucket, etc.).
func (s *Store) DeleteService(ctx context.Context, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE managed_services SET status='deleting', updated_at=now()
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`, t.Slug, id)
		if err != nil {
			return fmtErr("marcar servicio para borrado", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// MarkServiceReady fija el servicio como listo y guarda config pública +
// credenciales cifradas. Cross-tenant (lo invoca el controller).
func (s *Store) MarkServiceReady(ctx context.Context, tenant string, id uuid.UUID, config map[string]string, credsEnc []byte) error {
	cfg, _ := json.Marshal(config)
	if len(cfg) == 0 {
		cfg = []byte(`{}`)
	}
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE managed_services SET status='ready', config_json=$3, creds_enc=$4, updated_at=now()
			WHERE tenant=$1 AND id=$2`, tenant, id, cfg, credsEnc)
		if err != nil {
			return fmtErr("marcar servicio ready", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// FinalizeServiceDeletion borra definitivamente (soft) un servicio en estado
// 'deleting' tras liberar sus recursos. Cross-tenant.
func (s *Store) FinalizeServiceDeletion(ctx context.Context, tenant string, id uuid.UUID) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`UPDATE managed_services SET deleted_at=now(), updated_at=now()
			 WHERE tenant=$1 AND id=$2`, tenant, id)
		return err
	})
}

// ListPendingServices devuelve servicios en provisioning/deleting (cross-tenant)
// para el reconciliador del controller.
func (s *Store) ListPendingServices(ctx context.Context, limit int) ([]domain.ManagedService, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []domain.ManagedService
	// Cross-tenant: InMaintenanceTx habilita la política de mantenimiento de
	// managed_services (0003); con FORCE RLS sería deny-by-default (0 filas).
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		// Por «el que lleva más sin mirarse» y no por updated_at, que un fallo no
		// cambia: cincuenta servicios que fallan siempre ya no paran a los demás
		// (CLOUD-06).
		rows, err := tx.Query(ctx,
			`UPDATE managed_services SET checked_at = now() WHERE id IN (
				SELECT id FROM managed_services
				WHERE status IN ('provisioning','deleting') AND deleted_at IS NULL
				ORDER BY checked_at NULLS FIRST, updated_at LIMIT $1 FOR UPDATE SKIP LOCKED)
			 RETURNING `+managedCols, limit)
		if err != nil {
			return fmtErr("listar servicios pendientes", err)
		}
		defer rows.Close()
		for rows.Next() {
			m, err := scanManagedService(rows, "")
			if err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

// ResolveServiceCreds devuelve las credenciales cifradas de los servicios ready
// de un proyecto para que el controller las descifre e inyecte en el deployment.
// Cross-tenant.
//
// SOLO las de los servicios de ESE destino: una vista previa recibe las de los
// servicios creados para vistas previas y nunca las de producción, y al revés.
// El filtro va en el SQL, que es por donde pasan todos los que piden
// credenciales (el despliegue, las funciones, los workers y DB Studio): hacerlo
// en cada llamante es como se llegó a que una rama sin revisar arrancara con la
// base de datos de producción (CLOUD-01).
func (s *Store) ResolveServiceCreds(
	ctx context.Context, tenant string, projectID uuid.UUID, target domain.DeployTarget,
) (map[string][]byte, error) {
	// Cualquier valor que no sea 'preview' es producción: un destino vacío o
	// desconocido no puede abrir la puerta de las vistas previas, ni al revés.
	if target != domain.TargetPreview {
		target = domain.TargetProduction
	}
	out := map[string][]byte{}
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT kind, creds_enc FROM managed_services
			WHERE tenant=$1 AND project_id=$2 AND target=$3
			  AND status='ready' AND deleted_at IS NULL AND creds_enc IS NOT NULL`,
			tenant, projectID, string(target))
		if err != nil {
			return fmtErr("resolver credenciales de servicios", err)
		}
		defer rows.Close()
		for rows.Next() {
			var kind string
			var enc []byte
			if err := rows.Scan(&kind, &enc); err != nil {
				return err
			}
			out[kind] = enc
		}
		return rows.Err()
	})
	return out, err
}

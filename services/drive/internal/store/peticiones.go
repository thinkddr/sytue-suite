// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// «Pedir acceso de edición» de algo compartido en lectura (0025), con el contrato común
// de platform/pkg/peticiones. Pide quien lo tiene en lectura; concede quien lo compartió
// (o el dueño del fichero o la carpeta).
var tablaPeticiones = peticiones.Tabla{Nombre: "drive_peticiones_edicion", Recurso: "recurso"}

// PedirEdicion deja la petición. true si es nueva.
func (s *Store) PedirEdicion(ctx context.Context, recurso uuid.UUID, p peticiones.Peticion) (bool, error) {
	t := tenancy.MustFrom(ctx)
	p.Recurso = recurso.String()
	nueva := false
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		nueva, e = tablaPeticiones.Pedir(ctx, tx, t.Slug, p)
		return e
	})
	return nueva, err
}

// puedeConceder: es el dueño del fichero o la carpeta, o ha compartido `recurso` con
// alguien (una compartición viva suya: ni revocada ni caducada, como en todas las demás
// verjas de Drive). ErrNotFound si no, sin decir si existe.
func puedeConceder(ctx context.Context, tx pgx.Tx, tenant string, recurso uuid.UUID, sub string) error {
	var puede bool
	err := conTodaLaOrganizacion(ctx, tx, func() error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM files   WHERE tenant = $1 AND id = $2 AND owner_sub = $3)
			    OR EXISTS (SELECT 1 FROM folders WHERE tenant = $1 AND id = $2 AND owner_sub = $3)
			    OR EXISTS (SELECT 1 FROM shares  WHERE tenant = $1 AND (file_id = $2 OR folder_id = $2)
			                                       AND created_by = $3 AND revoked_at IS NULL
			                                       AND (expires_at IS NULL OR expires_at > now()))`,
			tenant, recurso, sub).Scan(&puede)
	})
	if err != nil {
		return err
	}
	if sub == "" || !puede {
		return domain.ErrNotFound
	}
	return nil
}

// PeticionesEdicion son las pendientes de `recurso`. Solo quien puede conceder.
func (s *Store) PeticionesEdicion(ctx context.Context, recurso uuid.UUID, sub string) ([]peticiones.Peticion, error) {
	t := tenancy.MustFrom(ctx)
	if OwnerFrom(ctx) == "" {
		ctx = WithOwner(ctx, sub)
	}
	var out []peticiones.Peticion
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := puedeConceder(ctx, tx, t.Slug, recurso, sub); err != nil {
			return err
		}
		var e error
		out, e = tablaPeticiones.Listar(ctx, tx, recurso.String())
		return e
	})
	return out, err
}

// ResolverPeticionEdicion borra la petición y, si se concede, sube a escritura la
// compartición de lectura de quien la pidió, en la MISMA transacción. Solo quien puede
// conceder, y solo a quien lo pidió (ErrNotFound si no). Si lo tenía por un grupo y no
// tiene compartición propia, se le da una de escritura.
func (s *Store) ResolverPeticionEdicion(ctx context.Context, recurso uuid.UUID, sub, sujeto string, concede bool) error {
	t := tenancy.MustFrom(ctx)
	if OwnerFrom(ctx) == "" {
		ctx = WithOwner(ctx, sub)
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if err := puedeConceder(ctx, tx, t.Slug, recurso, sub); err != nil {
			return err
		}
		habia, err := tablaPeticiones.Quitar(ctx, tx, recurso.String(), sujeto)
		if err != nil {
			return err
		}
		if !habia {
			return domain.ErrNotFound
		}
		if !concede {
			return nil
		}
		tag, err := tx.Exec(ctx, `
			UPDATE shares SET permission = 'write'
			 WHERE tenant = $1 AND (file_id = $2 OR folder_id = $2) AND kind = 'user'
			   AND subject = $3 AND permission = 'read' AND revoked_at IS NULL`,
			t.Slug, recurso, sujeto)
		if err != nil || tag.RowsAffected() > 0 {
			return err
		}
		return conTodaLaOrganizacion(ctx, tx, func() error {
			_, err := tx.Exec(ctx, `
				INSERT INTO shares (tenant, file_id, folder_id, kind, subject, permission, created_by)
				SELECT $1, (SELECT id FROM files WHERE tenant = $1 AND id = $2),
				           (SELECT id FROM folders WHERE tenant = $1 AND id = $2), 'user', $3, 'write', $4
				 WHERE EXISTS (SELECT 1 FROM files WHERE tenant = $1 AND id = $2)
				    OR EXISTS (SELECT 1 FROM folders WHERE tenant = $1 AND id = $2)`,
				t.Slug, recurso, sujeto, sub)
			return err
		})
	})
}

// PedidasPor son, de `recursos`, los que `sub` tiene pedidos: «Compartido conmigo» lo
// dice en la franja al volver. Una consulta para toda la lista.
func (s *Store) PedidasPor(ctx context.Context, sub string, recursos []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := map[uuid.UUID]bool{}
	if len(recursos) == 0 {
		return out, nil
	}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT recurso FROM drive_peticiones_edicion WHERE tenant = current_setting('app.tenant', true) AND sujeto = $1 AND recurso = ANY ($2)`, sub, recursos)
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		for _, id := range ids {
			out[id] = true
		}
		return err
	})
	return out, err
}

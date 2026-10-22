// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// «Pedir acceso» de un buzón compartido o delegado en `read` (0018), con el contrato
// común de platform/pkg/peticiones. Concede quien lo gestiona: `manage` en su ACL, o el
// dueño de la cuenta si es un buzón personal delegado. Conceder lo sube a `send`.
var tablaPeticiones = peticiones.Tabla{Nombre: "mailbox_peticiones_edicion", Recurso: "account_id"}

// gestores son quienes pueden conceder sobre `cuenta`: su ACL `manage` y, si es un
// buzón personal, su dueño.
const gestores = `
	SELECT grantee_subject FROM mailbox_acl WHERE account_id = $1 AND tenant = $2 AND permission = 'manage'
	UNION
	SELECT subject FROM accounts WHERE id = $1 AND tenant = $2 AND kind = 'user'`

// PedirEdicion deja la petición y devuelve a quién avisar. nueva=false si ya estaba.
func (s *Store) PedirEdicion(ctx context.Context, cuenta uuid.UUID, p peticiones.Peticion) (nueva bool, avisar []string, err error) {
	t := tenantOf(ctx)
	p.Recurso = cuenta.String()
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		if nueva, e = tablaPeticiones.Pedir(ctx, tx, t, p); e != nil || !nueva {
			return e
		}
		filas, e := tx.Query(ctx, gestores, cuenta, t)
		if e != nil {
			return e
		}
		avisar, e = pgx.CollectRows(filas, pgx.RowTo[string])
		return e
	})
	return nueva, avisar, err
}

// puedeConceder: `sub` gestiona `cuenta`. ErrNotFound si no.
func puedeConceder(ctx context.Context, tx pgx.Tx, cuenta uuid.UUID, sub string) error {
	var puede bool
	if err := tx.QueryRow(ctx, `SELECT $3::text IN (`+gestores+`)`, cuenta, tenantOf(ctx), sub).Scan(&puede); err != nil {
		return err
	}
	if sub == "" || !puede {
		return domain.ErrNotFound
	}
	return nil
}

// PeticionesEdicion son las pendientes de un buzón. Solo quien lo gestiona.
func (s *Store) PeticionesEdicion(ctx context.Context, cuenta uuid.UUID, sub string) ([]peticiones.Peticion, error) {
	var out []peticiones.Peticion
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := puedeConceder(ctx, tx, cuenta, sub); err != nil {
			return err
		}
		var e error
		out, e = tablaPeticiones.Listar(ctx, tx, cuenta.String())
		return e
	})
	return out, err
}

// ResolverPeticionEdicion borra la petición y, si se concede, pasa a quien la pidió de
// `read` a `send`, en la MISMA transacción. Solo quien gestiona el buzón, y solo a
// quien lo pidió (ErrNotFound si no). Nunca baja un permiso.
func (s *Store) ResolverPeticionEdicion(ctx context.Context, cuenta uuid.UUID, sub, sujeto string, concede bool) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := puedeConceder(ctx, tx, cuenta, sub); err != nil {
			return err
		}
		habia, err := tablaPeticiones.Quitar(ctx, tx, cuenta.String(), sujeto)
		if err != nil {
			return err
		}
		if !habia {
			return domain.ErrNotFound
		}
		if !concede {
			return nil
		}
		_, err = tx.Exec(ctx, `
			UPDATE mailbox_acl SET permission = 'send', updated_at = now()
			 WHERE account_id = $1 AND tenant = $2 AND grantee_subject = $3 AND permission = 'read'`,
			cuenta, tenantOf(ctx), sujeto)
		return err
	})
}

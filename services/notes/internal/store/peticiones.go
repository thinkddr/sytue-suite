// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// «Pedir acceso de edición» de un cuaderno ajeno que se lee pero no se escribe (0009),
// con el contrato común de platform/pkg/peticiones. Concede su DUEÑO, y conceder da de
// alta a quien lo pidió en `notebook_editores`.
var tablaPeticiones = peticiones.Tabla{Nombre: "notebook_peticiones_edicion", Recurso: "notebook_id"}

// PedirEdicion deja la petición y devuelve el dueño, a quien avisar. yaPuede si ya lo
// escribe; ErrNotFound si no lo ve.
func (s *Store) PedirEdicion(ctx context.Context, nb uuid.UUID, p peticiones.Peticion) (nueva, yaPuede bool, dueno string, err error) {
	t := tenancy.MustFrom(ctx)
	p.Recurso = nb.String()
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `
			SELECT nb.owner_sub,
			       nb.owner_sub = $3 OR nb.visibility = 'organization'
			       OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $3)
			  FROM notebooks nb
			 WHERE nb.tenant = $1 AND nb.id = $2 AND nb.deleted_at IS NULL
			   AND (nb.owner_sub = $3 OR nb.visibility IN ('organization','public')
			        OR EXISTS (SELECT 1 FROM notebook_lectores l WHERE l.notebook_id = nb.id AND l.sujeto = $3)
			        OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $3))`,
			t.Slug, nb, p.Sujeto).Scan(&dueno, &yaPuede)
		if isNoRows(e) {
			return domain.ErrNotFound
		}
		if e != nil || yaPuede {
			return e
		}
		nueva, e = tablaPeticiones.Pedir(ctx, tx, t.Slug, p)
		return e
	})
	return nueva, yaPuede, dueno, err
}

// esDueno: `sub` es el dueño de `nb`. ErrNotFound si no.
func esDueno(ctx context.Context, tx pgx.Tx, tenant string, nb uuid.UUID, sub string) error {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT true FROM notebooks WHERE tenant = $1 AND id = $2 AND owner_sub = $3 AND deleted_at IS NULL`,
		tenant, nb, sub).Scan(&ok)
	if isNoRows(e) || sub == "" {
		return domain.ErrNotFound
	}
	return e
}

// PeticionesEdicion son las pendientes de un cuaderno. Solo su dueño.
func (s *Store) PeticionesEdicion(ctx context.Context, nb uuid.UUID, sub string) ([]peticiones.Peticion, error) {
	t := tenancy.MustFrom(ctx)
	var out []peticiones.Peticion
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if e := esDueno(ctx, tx, t.Slug, nb, sub); e != nil {
			return e
		}
		var e error
		out, e = tablaPeticiones.Listar(ctx, tx, nb.String())
		return e
	})
	return out, err
}

// ResolverPeticionEdicion borra la petición y, si se concede, hace editora de las
// notas del cuaderno a quien la pidió, en la MISMA transacción. Solo el dueño, y solo
// a quien lo pidió (ErrNotFound si no).
func (s *Store) ResolverPeticionEdicion(ctx context.Context, nb uuid.UUID, sub, sujeto string, concede bool) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if e := esDueno(ctx, tx, t.Slug, nb, sub); e != nil {
			return e
		}
		habia, e := tablaPeticiones.Quitar(ctx, tx, nb.String(), sujeto)
		if e != nil {
			return e
		}
		if !habia {
			return domain.ErrNotFound
		}
		if !concede {
			return nil
		}
		_, e = tx.Exec(ctx, `
			INSERT INTO notebook_editores (notebook_id, tenant, sujeto, concedido_por) VALUES ($1, $2, $3, $4)
			ON CONFLICT (notebook_id, sujeto) DO NOTHING`, nb, t.Slug, sujeto, sub)
		return e
	})
}

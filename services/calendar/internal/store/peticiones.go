// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// «Pedir acceso de edición» de un calendario compartido (0013), con el contrato común
// de platform/pkg/peticiones. Pide quien lo tiene de lector; concede el DUEÑO.
var tablaPeticiones = peticiones.Tabla{Nombre: "calendar_peticiones_edicion", Recurso: "calendar_id"}

// PedirEdicion deja la petición y devuelve el dueño, a quien avisar. nueva=false si ya
// estaba pedida (no se vuelve a avisar).
func (s *Store) PedirEdicion(ctx context.Context, calendarID uuid.UUID, p peticiones.Peticion) (nueva bool, dueno string, err error) {
	t := tenancy.MustFrom(ctx)
	p.Recurso = calendarID.String()
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `SELECT owner_sub FROM calendars WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, calendarID).Scan(&dueno)
		if isNoRows(e) {
			return domain.ErrNotFound
		}
		if e != nil {
			return e
		}
		nueva, e = tablaPeticiones.Pedir(ctx, tx, t.Slug, p)
		return e
	})
	return nueva, dueno, err
}

// PeticionesEdicion son las pendientes de un calendario. Solo el dueño.
func (s *Store) PeticionesEdicion(ctx context.Context, calendarID uuid.UUID, ownerSub string) ([]peticiones.Peticion, error) {
	t := tenancy.MustFrom(ctx)
	var out []peticiones.Peticion
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireOwnedCalendar(ctx, tx, t.Slug, calendarID, ownerSub); err != nil {
			return err
		}
		var e error
		out, e = tablaPeticiones.Listar(ctx, tx, calendarID.String())
		return e
	})
	return out, err
}

// ResolverPeticionEdicion borra la petición y, si se concede, pasa a quien la pidió de
// lector a editor, en la MISMA transacción. Solo el dueño, y solo a quien lo pidió
// (ErrNotFound si no). Nunca baja un permiso: el UPDATE solo toca a los lectores.
func (s *Store) ResolverPeticionEdicion(ctx context.Context, calendarID uuid.UUID, ownerSub, sujeto string, concede bool) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireOwnedCalendar(ctx, tx, t.Slug, calendarID, ownerSub); err != nil {
			return err
		}
		var correo string
		e := tx.QueryRow(ctx, `SELECT correo FROM calendar_peticiones_edicion WHERE tenant=$3 AND calendar_id=$1 AND sujeto=$2`,
			calendarID, sujeto, t.Slug).Scan(&correo)
		if isNoRows(e) {
			return domain.ErrNotFound
		}
		if e != nil {
			return e
		}
		if _, e := tablaPeticiones.Quitar(ctx, tx, calendarID.String(), sujeto); e != nil {
			return e
		}
		if !concede {
			return nil
		}
		correo = domain.NormalizeEmail(correo)
		tag, e := tx.Exec(ctx,
			`UPDATE calendar_shares SET permission = 'writer', updated_at = now()
			  WHERE tenant=$1 AND calendar_id=$2 AND permission = 'reader'
			    AND ((grantee_sub <> '' AND grantee_sub = $3) OR (grantee_email <> '' AND grantee_email = $4))`,
			t.Slug, calendarID, sujeto, correo)
		if e != nil || tag.RowsAffected() > 0 {
			return e
		}
		// Sin fila de lector que subir (la tenía por otra vía): se le da la suya.
		_, e = tx.Exec(ctx,
			`INSERT INTO calendar_shares (tenant, calendar_id, grantee_email, grantee_sub, permission, created_by)
			 VALUES ($1, $2, $3, $4, 'writer', $5)
			 ON CONFLICT (tenant, calendar_id, grantee_email, grantee_sub) DO NOTHING`,
			t.Slug, calendarID, correo, sujeto, ownerSub)
		return e
	})
}

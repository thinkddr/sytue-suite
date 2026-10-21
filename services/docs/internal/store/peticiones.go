// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// tablaPeticiones es la de «Pedir acceso de edición» (0008), con el contrato común de
// platform/pkg/peticiones.
var tablaPeticiones = peticiones.Tabla{Nombre: "doc_peticiones_edicion", Recurso: "document_id"}

// PedirEdicion deja (o renueva) la petición. true si es NUEVA.
func (s *Store) PedirEdicion(ctx context.Context, p peticiones.Peticion) (bool, error) {
	nueva := false
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		t, _ := tenancy.From(ctx)
		var err error
		nueva, err = tablaPeticiones.Pedir(ctx, tx, t.Slug, p)
		return err
	})
	return nueva, err
}

// PeticionesEdicion son las pendientes de un documento.
func (s *Store) PeticionesEdicion(ctx context.Context, docID string) ([]peticiones.Peticion, error) {
	var out []peticiones.Peticion
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = tablaPeticiones.Listar(ctx, tx, docID)
		return err
	})
	return out, err
}

// HayPeticionEdicion dice si esta persona ya lo pidió.
func (s *Store) HayPeticionEdicion(ctx context.Context, docID, sujeto string) (bool, error) {
	hay := false
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		hay, err = tablaPeticiones.Hay(ctx, tx, docID, sujeto)
		return err
	})
	return hay, err
}

// ResolverPeticionEdicion borra la petición y, si se concede, da el rol de editor
// en la MISMA transacción: una concesión a medias (permiso sin borrar la petición, o
// al revés) dejaría la franja del dueño pidiéndole algo ya hecho. ErrNotFound si no
// había petición: no se concede a quien no lo pidió.
func (s *Store) ResolverPeticionEdicion(ctx context.Context, docID, sujeto, concede string) error {
	return s.documentTx(ctx, docID, func(tx pgx.Tx) error {
		habia, err := tablaPeticiones.Quitar(ctx, tx, docID, sujeto)
		if err != nil {
			return err
		}
		if !habia {
			return domain.ErrNotFound
		}
		if concede == "" {
			return nil
		}
		t, _ := tenancy.From(ctx)
		// Nunca BAJA un rol: si entretanto alguien le dio más, se queda con más.
		if _, err := tx.Exec(ctx, `
			INSERT INTO doc_permissions (document_id, tenant, principal_kind, principal_id, role, granted_by)
			VALUES ($1, $2, 'user', $3, 'editor', $4)
			ON CONFLICT (document_id, principal_kind, principal_id)
			DO UPDATE SET role = 'editor', granted_by = EXCLUDED.granted_by, via_link = NULL
			WHERE doc_permissions.role IN ('viewer', 'commenter') OR doc_permissions.via_link IS NOT NULL`,
			docID, t.Slug, sujeto, concede); err != nil {
			return fmt.Errorf("store: conceder edición: %w", err)
		}
		return nil
	})
}

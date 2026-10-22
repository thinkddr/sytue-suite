// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

// SyncAvatarByEmail refleja el avatar propio de Sytue ID en TODAS las fichas de
// contacto del tenant cuyo correo coincida, sin importar quién las tenga en su
// agenda (owner_sub) ni en qué libreta — a diferencia de SetContactPhoto/
// RemoveContactPhoto, que son owner-scoped por UID. Es lo que la agenda de
// «Organización» (POST /api/directorio) NO cubre: esa solo crea lo que falta y
// nunca reescribe lo que ya hay (para no pisar lo que cada quien anotó a mano);
// el avatar es la única excepción deliberada, porque es la misma persona vista
// desde la agenda de otro.
//
// dataURI vacío QUITA la foto (el usuario quitó su avatar propio). Devuelve
// cuántas fichas se tocaron.
func (s *Store) SyncAvatarByEmail(ctx context.Context, emails []string, dataURI string) (int, error) {
	if len(emails) == 0 {
		return 0, nil
	}
	lower := make([]string, len(emails))
	for i, e := range emails {
		lower[i] = strings.ToLower(strings.TrimSpace(e))
	}
	t := tenancy.MustFrom(ctx)
	tocadas := 0
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT c.object_id, c.address_book_id
			FROM contacts c
			JOIN contact_emails ce ON ce.tenant = c.tenant AND ce.contact_id = c.id
			WHERE c.tenant = $1 AND lower(ce.email) = ANY($2) AND c.deleted_at IS NULL`,
			t.Slug, lower)
		if err != nil {
			return fmt.Errorf("store: buscar fichas por correo: %w", err)
		}
		type objetivo struct {
			objectID, bookID uuid.UUID
		}
		var objetivos []objetivo
		for rows.Next() {
			var o objetivo
			if err := rows.Scan(&o.objectID, &o.bookID); err != nil {
				rows.Close()
				return fmt.Errorf("store: leer ficha a sincronizar: %w", err)
			}
			objetivos = append(objetivos, o)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, o := range objetivos {
			var data string
			if err := tx.QueryRow(ctx,
				`SELECT data FROM card_objects WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`,
				t.Slug, o.objectID).Scan(&data); err != nil {
				if isNoRows(err) {
					continue // borrada entre el SELECT y aquí: no hay nada que sincronizar
				}
				return fmt.Errorf("store: leer tarjeta a sincronizar: %w", err)
			}
			var card string
			var err error
			if dataURI == "" {
				card, err = domain.RemoveCardPhoto(data)
			} else {
				card, err = domain.SetCardPhoto(data, dataURI)
			}
			if err != nil {
				return err
			}
			if err := persistCard(ctx, tx, t.Slug, o.objectID, o.bookID, card); err != nil {
				return err
			}
			tocadas++
		}
		return nil
	})
	return tocadas, err
}

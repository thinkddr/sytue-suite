// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// maxQuienesVen acota a quiénes se lleva la señal de cambios de una escritura: el dueño
// y las personas con quien se compartió, que en la práctica son unas pocas.
const maxQuienesVen = 200

// QuienesVenCuaderno son quienes ven lo que se escribe en un cuaderno: su dueño y las
// personas con quien lo compartió, para leer o para editar. Es a quien llega la señal de
// cambios (hubnotify.AvisarQuienesVen) cuando otra persona escribe en él: hasta el
// 2026-11-25 solo la recibía quien escribía (ADR 0078). Un cuaderno de toda la
// organización no se avisa persona a persona.
func (s *Store) QuienesVenCuaderno(ctx context.Context, nb uuid.UUID) ([]string, error) {
	return s.quienesVen(ctx, `
		SELECT owner_sub FROM notebooks WHERE tenant = $1 AND id = $2
		UNION SELECT sujeto FROM notebook_editores WHERE tenant = $1 AND notebook_id = $2
		UNION SELECT sujeto FROM notebook_lectores WHERE tenant = $1 AND notebook_id = $2
		LIMIT $3`, nb)
}

// QuienesVenNota son quienes ven una nota: los de su cuaderno, quien la escribió y las
// personas con quien la compartió suelta.
func (s *Store) QuienesVenNota(ctx context.Context, nota uuid.UUID) ([]string, error) {
	return s.quienesVen(ctx, `
		SELECT n.owner_sub FROM notes n WHERE n.tenant = $1 AND n.id = $2
		UNION SELECT nb.owner_sub FROM notes n JOIN notebooks nb ON nb.tenant = n.tenant AND nb.id = n.notebook_id
		       WHERE n.tenant = $1 AND n.id = $2
		UNION SELECT e.sujeto FROM notes n JOIN notebook_editores e ON e.tenant = n.tenant AND e.notebook_id = n.notebook_id
		       WHERE n.tenant = $1 AND n.id = $2
		UNION SELECT l.sujeto FROM notes n JOIN notebook_lectores l ON l.tenant = n.tenant AND l.notebook_id = n.notebook_id
		       WHERE n.tenant = $1 AND n.id = $2
		UNION SELECT sujeto FROM note_personas WHERE tenant = $1 AND note_id = $2
		LIMIT $3`, nota)
}

func (s *Store) quienesVen(ctx context.Context, consulta string, id uuid.UUID) ([]string, error) {
	t := tenancy.MustFrom(ctx)
	var out []string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, consulta, t.Slug, id, maxQuienesVen)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("store: quiénes ven: %w", err)
	}
	return out, nil
}

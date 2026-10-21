// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PurgeExpiredTrash destruye lo que lleve en la papelera más de `retention` y
// devuelve qué se fue, con la organización y la huella de cada foto: es lo que
// hace falta para borrar después sus derivadas del almacén.
//
// RECORRE TODAS LAS ORGANIZACIONES. Es la única consulta del servicio que lo
// hace, y por eso va en una transacción de MANTENIMIENTO: sin `app.tenant` que
// fijar, la política de aislamiento no dejaría ver ni borrar nada y el worker
// diría «purgadas 0» para siempre sin que nada fallara.
//
// Se lee antes de borrar, en dos sentencias de la misma transacción, por lo
// mismo que la purga a mano: las recetas se van en cascada con la foto, y
// depender de si el disparador de la cascada ya corrió dentro de la misma
// sentencia es apostar sobre un detalle del motor.
func (s *Store) PurgeExpiredTrash(ctx context.Context, retention time.Duration) ([]Purgada, error) {
	if retention <= 0 {
		return nil, nil
	}
	corte := time.Now().Add(-retention)
	var out []Purgada
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.id, a.tenant, coalesce(e.fingerprint, '')
			  FROM assets a
			  LEFT JOIN edits e
			    ON e.tenant = a.tenant AND e.asset_id = a.id AND e.owner_sub = a.owner_sub
			 WHERE a.deleted_at IS NOT NULL AND a.deleted_at < $1
			 -- El tope por vuelta acota lo que una purga tiene bloqueado a la
			 -- vez. Lo que sobre se recoge en la siguiente, dentro de una hora.
			 LIMIT 5000`, corte)
		if err != nil {
			return err
		}
		defer rows.Close()
		out = out[:0]
		for rows.Next() {
			var p Purgada
			if err := rows.Scan(&p.ID, &p.Tenant, &p.Huella); err != nil {
				return err
			}
			out = append(out, p)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(out) == 0 {
			return nil
		}
		// Se borran POR IDENTIFICADOR y no repitiendo el `deleted_at < corte`:
		// entre las dos sentencias alguien puede haber tirado otra foto, y
		// borrarla sería purgar algo que acaba de entrar en la papelera.
		_, err = tx.Exec(ctx, `DELETE FROM assets WHERE id = ANY($1)`, idsDe(out))
		return err
	})
	return out, err
}

// idsDe saca los identificadores para el `= ANY($1)` del borrado.
func idsDe(ps []Purgada) []uuid.UUID {
	ids := make([]uuid.UUID, len(ps))
	for i, p := range ps {
		ids[i] = p.ID
	}
	return ids
}

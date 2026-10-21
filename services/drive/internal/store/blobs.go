// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Candidatos a huérfano de los blobs COMPARTIDOS (migración 0022). Sin tenant:
// la tabla no tiene datos de ninguna organización y la lee el mantenimiento.

// AnotarBlobCandidato apunta una clave compartida que una fila dejó de usar.
// Idempotente; conserva la fecha de la primera anotación.
func (s *Store) AnotarBlobCandidato(ctx context.Context, key string) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO blobs_candidatos (storage_key) VALUES ($1) ON CONFLICT DO NOTHING`, key)
		return err
	})
}

// OlvidarBlobCandidato retira una clave de los candidatos: alguien la vuelve a
// usar. Si el barrido la está procesando en ese momento, ESPERA a que acabe
// (la fila está bloqueada) y así quien reutiliza sabe, al volver, si el objeto
// sobrevivió: por eso se comprueba la existencia DESPUÉS de llamar aquí.
func (s *Store) OlvidarBlobCandidato(ctx context.Context, key string) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM blobs_candidatos WHERE storage_key = $1`, key)
		return err
	})
}

// PurgarBlobsCandidatos recorre los candidatos anotados antes de `antesDe` y, para
// cada uno, decide con la referencia GLOBAL —todas las organizaciones, bajo el
// mantenimiento— si sigue en uso. Los que nadie usa se entregan a `borrar` (el
// objeto de S3) DENTRO de la transacción que bloquea la fila, para que nadie
// pueda reutilizar la clave mientras se está borrando. Devuelve cuántos borró.
func (s *Store) PurgarBlobsCandidatos(ctx context.Context, antesDe time.Time, limit int,
	borrar func(ctx context.Context, key string) error,
) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	borrados := 0
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT storage_key FROM blobs_candidatos
			 WHERE desde < $1
			 ORDER BY desde
			 LIMIT $2
			 FOR UPDATE SKIP LOCKED`, antesDe, limit)
		if err != nil {
			return fmt.Errorf("store: candidatos: %w", err)
		}
		var claves []string
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				rows.Close()
				return err
			}
			claves = append(claves, k)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, k := range claves {
			var enUso bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS (SELECT 1 FROM files         WHERE storage_key = $1)
				    OR EXISTS (SELECT 1 FROM file_versions WHERE storage_key = $1)`, k).Scan(&enUso); err != nil {
				return fmt.Errorf("store: referencia global: %w", err)
			}
			if !enUso {
				if err := borrar(ctx, k); err != nil {
					// Se deja el candidato: lo intentará el siguiente barrido.
					continue
				}
				borrados++
			}
			if _, err := tx.Exec(ctx, `DELETE FROM blobs_candidatos WHERE storage_key = $1`, k); err != nil {
				return err
			}
		}
		return nil
	})
	return borrados, err
}

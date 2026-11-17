// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// GuardarMiniatura anota la miniatura de la portada dibujada con la secuencia `seq` del
// documento y devuelve la clave de la que sustituye, para que quien llama la borre de
// Object Storage.
//
// Solo AVANZA: si ya hay una dibujada con una secuencia igual o posterior —dos réplicas
// que la dibujaron a la vez, o una que se retrasó—, no cambia nada y `guardada` es
// false; entonces sobra la que se acaba de subir. Tampoco toca `updated_at`: dibujarla
// no es editar el documento, y la portada se ordena por esa fecha.
func (s *Store) GuardarMiniatura(ctx context.Context, docID string, seq int64, clave string) (anterior string, guardada bool, err error) {
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		var vieja *string
		var vieSeq int64
		err := tx.QueryRow(ctx, `
			SELECT thumbnail_key, thumbnail_seq FROM documents
			WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, docID).Scan(&vieja, &vieSeq)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if vieSeq >= seq {
			return nil
		}
		if _, err := tx.Exec(ctx, `
			UPDATE documents SET thumbnail_key = $2, thumbnail_seq = $3 WHERE id = $1`,
			docID, clave, seq); err != nil {
			return err
		}
		if vieja != nil {
			anterior = *vieja
		}
		guardada = true
		return nil
	})
	if err != nil {
		return "", false, fmt.Errorf("store: guardar miniatura: %w", err)
	}
	return anterior, guardada, nil
}

// MiniaturaDe es la clave y la secuencia de la miniatura de un documento; clave vacía =
// sin miniatura todavía.
func (s *Store) MiniaturaDe(ctx context.Context, docID string) (clave string, seq int64, err error) {
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		var k *string
		e := tx.QueryRow(ctx, `
			SELECT thumbnail_key, thumbnail_seq FROM documents
			WHERE id = $1 AND deleted_at IS NULL`, docID).Scan(&k, &seq)
		if isNoRows(e) {
			return domain.ErrNotFound
		}
		if k != nil {
			clave = *k
		}
		return e
	})
	if err != nil {
		return "", 0, fmt.Errorf("store: leer miniatura: %w", err)
	}
	return clave, seq, nil
}

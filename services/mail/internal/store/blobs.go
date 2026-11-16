// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
)

// Contabilidad de los blobs COMPARTIDOS (migración 0017). Las tres operaciones
// van sin tenant: `blob_refs` no tiene columna de organización. Desde el
// 2026-10-16 el `hash` de las filas nuevas es `<organización>/<sha256>` y su
// clave lleva la organización (blobs.Clave): solo se comparte dentro de ella.
// Las de antes, con el hash a secas, pueden estar referenciadas desde varias.

// ReservarBlobCompartido anota que un mensaje con ese hash va a existir bajo
// `key`. Devuelve si ya había fila (otro buzón de la organización ya lo trajo)
// y si su objeto está subido entero. Quien la crea es quien sube.
func (s *Store) ReservarBlobCompartido(ctx context.Context, hash, key string, size int64) (exists, ready bool, err error) {
	err = s.pool.InTx(ctx, func(tx pgx.Tx) error { exists, ready, err = reserveSharedBlobTx(ctx, tx, hash, key, size); return err })
	return exists, ready, err
}

func reserveSharedBlobTx(ctx context.Context, tx pgx.Tx, hash, key string, size int64) (bool, bool, error) {
	tag, err := tx.Exec(ctx, `INSERT INTO blob_refs(hash,key,size_bytes) VALUES($1,$2,$3) ON CONFLICT(hash) DO NOTHING`, hash, key, size)
	if err != nil {
		return false, false, fmt.Errorf("store: reservar blob: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return false, false, nil
	}
	var ready bool
	err = tx.QueryRow(ctx, `SELECT listo FROM blob_refs WHERE hash=$1`, hash).Scan(&ready)
	return true, ready, err
}

// MarcarBlobListo confirms the complete uploaded object in its caller transaction.
func (s *Store) MarcarBlobListo(ctx context.Context, hash string) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error { return markSharedBlobReadyTx(ctx, tx, hash) })
}

func markSharedBlobReadyTx(ctx context.Context, tx pgx.Tx, hash string) error {
	_, err := tx.Exec(ctx, `UPDATE blob_refs SET listo=true WHERE hash=$1`, hash)
	return err
}

// LiberarBlobCompartido preserves blobs already referenced by another mailbox.
func (s *Store) LiberarBlobCompartido(ctx context.Context, hash string) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error { return releaseSharedBlobTx(ctx, tx, hash) })
}

func releaseSharedBlobTx(ctx context.Context, tx pgx.Tx, hash string) error {
	_, err := tx.Exec(ctx, `DELETE FROM blob_refs WHERE hash=$1 AND refs=0 AND NOT listo`, hash)
	return err
}

// BlobCompartido dice si una clave de mensaje vive en el espacio compartido.
func BlobCompartido(key string) bool {
	return strings.HasPrefix(key, objectstore.PrefijoCompartido)
}

// sumarReferencia cuenta una fila más apuntando al blob compartido. Va en la
// MISMA transacción que el insert del mensaje: si el insert no entra, tampoco
// la referencia.
func sumarReferencia(ctx context.Context, tx pgx.Tx, key string) error {
	if !BlobCompartido(key) {
		return nil
	}
	tag, err := tx.Exec(ctx, `UPDATE blob_refs SET refs = refs + 1 WHERE key = $1`, key)
	if err != nil {
		return fmt.Errorf("store: sumar referencia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Una fila de mensaje que apunta a un blob sin contabilidad sería un
		// objeto que nadie borra nunca: mejor no insertar el mensaje.
		return fmt.Errorf("store: blob compartido %q sin reserva", key)
	}
	return nil
}

// soltarBlobs decide, para las claves de mensajes ya borrados, cuáles se pueden
// borrar de S3: las compartidas cuando su contador llega a cero; las propias
// del tenant (anteriores a la deduplicación) cuando ninguna otra fila de la
// organización —una copia entre buzones comparte clave— las referencia.
func soltarBlobs(ctx context.Context, tx pgx.Tx, claves []string) ([]string, error) {
	vistas := map[string]bool{}
	var huerfanas []string
	for _, key := range claves {
		if key == "" || vistas[key] {
			continue
		}
		vistas[key] = true
		var retained bool
		if err := tx.QueryRow(ctx, `SELECT mail_private_storage_retains_blob($1)`, key).Scan(&retained); err != nil {
			return nil, err
		}
		if retained {
			continue
		}
		if BlobCompartido(key) {
			var refs int
			err := tx.QueryRow(ctx, `
				UPDATE blob_refs SET refs = GREATEST(refs - 1, 0) WHERE key = $1 RETURNING refs`, key).Scan(&refs)
			if errors.Is(err, pgx.ErrNoRows) {
				continue // sin contabilidad: no se toca, lo mirará el barrido de huérfanos
			}
			if err != nil {
				return nil, fmt.Errorf("store: restar referencia: %w", err)
			}
			if refs == 0 {
				if _, err := tx.Exec(ctx, `DELETE FROM blob_refs WHERE key = $1 AND refs = 0`, key); err != nil {
					return nil, fmt.Errorf("store: cerrar blob: %w", err)
				}
				huerfanas = append(huerfanas, key)
			}
			continue
		}
		var referenciada bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM messages WHERE blob_key = $1)`, key).Scan(&referenciada); err != nil {
			return nil, fmt.Errorf("store: comprobar referencia: %w", err)
		}
		if !referenciada {
			huerfanas = append(huerfanas, key)
		}
	}
	return huerfanas, nil
}

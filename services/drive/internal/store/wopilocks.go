// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// WopiLock es el bloqueo que un editor externo mantiene sobre un fichero mientras lo
// tiene abierto. El `LockID` lo elige el editor y es OPACO: solo se compara por igualdad.
type WopiLock struct {
	FileID    uuid.UUID
	LockID    string
	HolderSub string
	ExpiresAt time.Time
}

// ErrLockConflict indica que el fichero lo tiene otro editor con OTRO identificador de
// bloqueo. WOPI exige responder 409 y devolver el lock actual en la cabecera X-WOPI-Lock,
// que es lo que permite al editor decir «lo tiene abierto alguien» en vez de fallar mudo.
var ErrLockConflict = errors.New("store: fichero bloqueado por otro editor")

// AcquireWopiLock toma o REFRESCA el bloqueo de un fichero.
//
// Es una sola sentencia a propósito. Comprobar y luego insertar en dos pasos deja una
// ventana entre ambos: dos editores que abren el mismo fichero a la vez pasarían los dos
// la comprobación y el segundo pisaría al primero. El `ON CONFLICT ... WHERE` mueve la
// decisión dentro del índice único, que es donde PostgreSQL la serializa de verdad.
//
// Se concede si: no había lock, el que había CADUCÓ, o es el MISMO lock_id (refresco).
func (s *Store) AcquireWopiLock(ctx context.Context, fileID uuid.UUID, lockID, holder string, ttl time.Duration) (WopiLock, error) {
	t := tenancy.MustFrom(ctx)
	var got WopiLock
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO wopi_locks (tenant, file_id, lock_id, holder_sub, expires_at)
			VALUES ($1, $2, $3, $4, now() + make_interval(secs => $5))
			ON CONFLICT (file_id) DO UPDATE
			   SET lock_id    = EXCLUDED.lock_id,
			       holder_sub = EXCLUDED.holder_sub,
			       expires_at = EXCLUDED.expires_at
			 WHERE wopi_locks.expires_at <= now()
			    OR wopi_locks.lock_id = EXCLUDED.lock_id
			RETURNING file_id, lock_id, holder_sub, expires_at`,
			t.Slug, fileID, lockID, holder, ttl.Seconds())
		return row.Scan(&got.FileID, &got.LockID, &got.HolderSub, &got.ExpiresAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// El WHERE del DO UPDATE no se cumplió: hay un lock vivo y es de otro.
		return WopiLock{}, ErrLockConflict
	}
	return got, err
}

// GetWopiLock devuelve el bloqueo VIVO de un fichero. Un lock caducado se trata como
// inexistente: es justo lo que permite recuperar un fichero cuyo editor murió sin soltarlo.
func (s *Store) GetWopiLock(ctx context.Context, fileID uuid.UUID) (WopiLock, error) {
	t := tenancy.MustFrom(ctx)
	var l WopiLock
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT file_id, lock_id, holder_sub, expires_at FROM wopi_locks
			  WHERE tenant=$1 AND file_id=$2 AND expires_at > now()`,
			t.Slug, fileID).Scan(&l.FileID, &l.LockID, &l.HolderSub, &l.ExpiresAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return WopiLock{}, domain.ErrNotFound
	}
	return l, err
}

// ReleaseWopiLock suelta el bloqueo SOLO si el lock_id coincide.
//
// La comparación va en el DELETE y no antes: sin ella, un editor podría soltar el bloqueo
// de otro —el suyo caducado, el ajeno vivo— y dejar el fichero abierto a la sobreescritura
// que el bloqueo existía para impedir.
func (s *Store) ReleaseWopiLock(ctx context.Context, fileID uuid.UUID, lockID string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`DELETE FROM wopi_locks
			  WHERE tenant=$1 AND file_id=$2 AND (lock_id=$3 OR expires_at <= now())`,
			t.Slug, fileID, lockID)
		if err != nil {
			return fmt.Errorf("store: soltar bloqueo wopi: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// O no había (nada que soltar, no es un error para el llamante) o es de
			// otro. Se distingue mirando si queda uno vivo.
			var vivo bool
			if err := tx.QueryRow(ctx,
				`SELECT EXISTS (SELECT 1 FROM wopi_locks
				   WHERE tenant=$1 AND file_id=$2 AND expires_at > now())`,
				t.Slug, fileID).Scan(&vivo); err != nil {
				return fmt.Errorf("store: comprobar bloqueo wopi: %w", err)
			}
			if vivo {
				return ErrLockConflict
			}
		}
		return nil
	})
}

// PurgeExpiredWopiLocks borra los bloqueos vencidos. Lo llama el mismo barrido periódico
// que la papelera: sin él la tabla crece con locks que ya no significan nada.
func (s *Store) PurgeExpiredWopiLocks(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM wopi_locks WHERE expires_at <= now()`)
		if err != nil {
			return fmt.Errorf("store: purgar bloqueos wopi: %w", err)
		}
		n = tag.RowsAffected()
		return nil
	})
	return n, err
}

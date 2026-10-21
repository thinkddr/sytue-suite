// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// Lo que usa el barrido periódico (service/barrido.go). Cruza organizaciones, así
// que todo va en mantenimiento, como la cola; la política de borrado es de 0006.

// claveDelBarrido es el advisory lock que hace del barrido un trabajo de un solo
// ejecutor entre réplicas. El número es arbitrario pero fijo: lo comparten todas
// las réplicas de Cast (y ningún otro servicio usa esta base).
const claveDelBarrido int64 = 0x0CA57_5EEB

// ConCerrojoDelBarrido corre fn solo si esta réplica se queda el cerrojo del
// barrido; si lo tiene otra, devuelve (false, nil) sin llamar a fn.
//
// El cerrojo es de SESIÓN (pg_try_advisory_lock), sobre una conexión directa
// (pg.Pool.AcquireDirect, pensada para esto: el pooler transaccional no guarda
// estado de sesión). Dura lo que dura fn, que abre sus propias transacciones
// cortas; si el proceso muere, la sesión se cierra y el cerrojo se suelta solo.
func (s *Store) ConCerrojoDelBarrido(ctx context.Context, fn func(ctx context.Context) error) (bool, error) {
	conn, err := s.pool.AcquireDirect(ctx)
	if err != nil {
		return false, fmt.Errorf("cast: conexión para el cerrojo del barrido: %w", err)
	}
	var mio bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, claveDelBarrido).Scan(&mio); err != nil {
		conn.Release()
		return false, fmt.Errorf("cast: cerrojo del barrido: %w", err)
	}
	if !mio {
		conn.Release()
		return false, nil
	}
	defer func() {
		// Soltarlo aunque ctx ya esté cancelado (apagado). Si no se puede, la
		// conexión no vuelve al pool con el cerrojo puesto: se cierra.
		fin, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(fin, `SELECT pg_advisory_unlock($1)`, claveDelBarrido); err != nil {
			_ = conn.Hijack().Close(fin)
			return
		}
		conn.Release()
	}()
	return true, fn(ctx)
}

// Borrado es un vídeo borrado pendiente de retirar: lo justo para purgarlo y
// seguir la pasada.
type Borrado struct {
	ID        uuid.UUID
	Tenant    string
	DeletedAt time.Time
}

// Cursor es la posición de la pasada, para pedir la tanda siguiente.
func (b Borrado) Cursor() *Cursor { return &Cursor{Fecha: b.DeletedAt, ID: b.ID} }

// BorradosCaducados lista, del más antiguo al más reciente, hasta `limite` vídeos
// borrados hace más de `gracia`, a partir de `desde` (exclusivo; nil = desde el
// principio). El cursor deja avanzar la pasada aunque la purga de alguno falle y
// su fila siga ahí.
func (s *Store) BorradosCaducados(ctx context.Context, gracia time.Duration, desde *Cursor, limite int) ([]Borrado, error) {
	fecha, id := argsCursor(desde)
	var out []Borrado
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, deleted_at FROM videos
			 WHERE deleted_at IS NOT NULL AND deleted_at < now() - $1::interval
			   AND ($2::timestamptz IS NULL OR (deleted_at, id) > ($2::timestamptz, $3::uuid))
			 ORDER BY deleted_at, id
			 LIMIT $4`, gracia, fecha, id, limite)
		if err != nil {
			return fmt.Errorf("cast: listar borrados caducados: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var b Borrado
			if err := rows.Scan(&b.ID, &b.Tenant, &b.DeletedAt); err != nil {
				return fmt.Errorf("cast: leer borrado caducado: %w", err)
			}
			out = append(out, b)
		}
		return rows.Err()
	})
	return out, err
}

// Eliminar retira del todo la fila de un vídeo ya borrado y, por cascada, lo que
// cuelga de él (personas, trabajos, espectadores, comentarios, chat). Un vídeo
// que no está borrado no se toca: domain.ErrNoExiste.
func (s *Store) Eliminar(ctx context.Context, id uuid.UUID) error {
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM videos WHERE id = $1 AND deleted_at IS NOT NULL`, id)
		if err != nil {
			return fmt.Errorf("cast: eliminar vídeo borrado: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		return nil
	})
}

// MotivoSubidaCaducada es lo que ve quien subía cuando el barrido da su subida
// por perdida.
const MotivoSubidaCaducada = "la subida no llegó a completarse; bórralo y vuelve a intentarlo"

// CaducarSubidas pasa a `failed` hasta `limite` vídeos que llevan en `uploading`
// más de `plazo` sin cambiar, y los devuelve (uploading → failed es una
// transición válida: domain.PuedePasar). Una fila que otra transacción tiene
// cogida (una subida que termina justo ahora) se salta.
func (s *Store) CaducarSubidas(ctx context.Context, plazo time.Duration, limite int) ([]domain.Video, error) {
	var out []domain.Video
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			UPDATE videos
			   SET status = 'failed', failure = $3, updated_at = now()
			 WHERE id IN (SELECT id FROM videos
			               WHERE status = 'uploading' AND deleted_at IS NULL
			                 AND updated_at < now() - $1::interval
			               ORDER BY updated_at
			               FOR UPDATE SKIP LOCKED
			               LIMIT $2)
			 RETURNING `+columnas, plazo, limite, MotivoSubidaCaducada)
		if err != nil {
			return fmt.Errorf("cast: caducar subidas: %w", err)
		}
		out, err = leerVideos(rows)
		return err
	})
	return out, err
}

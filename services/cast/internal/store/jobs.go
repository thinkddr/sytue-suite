// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// La cola de transcodificación (CAST-ENCARGO §4 `transcode_jobs`, §7). El servicio
// encola en la organización del vídeo; el transcodificador reclama y resuelve en
// mantenimiento, porque cruza organizaciones, y en la MISMA transacción mueve el
// vídeo: así un trabajo hecho y un vídeo que sigue en `processing` no pueden
// darse a la vez.
//
// Un trabajo de póster NO toca el ciclo de vida: el vídeo sigue `ready` (y
// visible) mientras se cuece, y si falla solo se pierde el póster nuevo.

const columnasJob = `id, tenant, video_id, kind, status, attempts, claimed_at, claimed_by, error, payload, created_at, updated_at`

func leerJob(row pgx.Row) (domain.Job, error) {
	var j domain.Job
	err := row.Scan(&j.ID, &j.Tenant, &j.VideoID, &j.Kind, &j.Status, &j.Attempts, &j.ClaimedAt, &j.ClaimedBy, &j.Error, &j.Payload, &j.CreatedAt, &j.UpdatedAt)
	if esNoRows(err) {
		return domain.Job{}, domain.ErrNoExiste
	}
	if err != nil {
		return domain.Job{}, fmt.Errorf("cast: leer trabajo: %w", err)
	}
	return j, nil
}

// encolar inserta el trabajo dentro de la transacción dada. Si ya hay uno activo
// (pending o running) de esa clase para el vídeo, devuelve ese: pedir dos veces
// lo mismo no lo duplica (índice `transcode_jobs_uno_activo`).
func encolar(ctx context.Context, tx pgx.Tx, tenant string, videoID uuid.UUID, kind domain.JobKind, payload []byte) (domain.Job, error) {
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	j, err := leerJob(tx.QueryRow(ctx, `
		INSERT INTO transcode_jobs (tenant, video_id, kind, payload)
		VALUES ($1, $2, $3, $4::jsonb)
		ON CONFLICT (video_id, kind) WHERE status IN ('pending', 'running') DO NOTHING
		RETURNING `+columnasJob, tenant, videoID, kind, payload))
	if !errors.Is(err, domain.ErrNoExiste) {
		return j, err
	}
	return leerJob(tx.QueryRow(ctx, `
		SELECT `+columnasJob+` FROM transcode_jobs
		 WHERE video_id = $1 AND kind = $2 AND tenant = $3 AND status IN ('pending', 'running')`, videoID, kind, tenant))
}

// Encolar crea un trabajo para el vídeo. La subida y la grabación de un directo
// pasan además el vídeo a `processing`; el póster no lo toca. `payload` es JSON;
// nil = {}.
func (s *Store) Encolar(ctx context.Context, tenant string, videoID uuid.UUID, kind domain.JobKind, payload []byte) (domain.Job, error) {
	if !kind.Valida() {
		return domain.Job{}, fmt.Errorf("%w: clase de trabajo %q", domain.ErrEntradaInvalida, kind)
	}
	var j domain.Job
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		v, err := leerVideo(tx.QueryRow(ctx, `SELECT `+columnas+` FROM videos WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL FOR UPDATE`, videoID, tenant))
		if err != nil {
			return err
		}
		if kind.MueveElVideo() && v.Status != domain.StatusProcessing {
			if _, err := cambiarEstado(ctx, tx, tenant, videoID, Estado{A: domain.StatusProcessing}); err != nil {
				return err
			}
		}
		j, err = encolar(ctx, tx, tenant, videoID, kind, payload)
		return err
	})
	return j, err
}

// RecibirOriginal registra el original ya guardado (claves y bytes), pasa el vídeo
// a `processing` y encola su transcodificación, TODO en una transacción: o queda
// el vídeo procesándose con su trabajo, o no cambia nada.
func (s *Store) RecibirOriginal(ctx context.Context, tenant string, videoID uuid.UUID, clave string, bytes int64) (domain.Video, error) {
	var v domain.Video
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var err error
		v, err = cambiarEstado(ctx, tx, tenant, videoID, Estado{
			A: domain.StatusProcessing, OriginalKey: &clave, BytesOriginal: &bytes, BytesTotal: &bytes,
		})
		if err != nil {
			return err
		}
		_, err = encolar(ctx, tx, tenant, videoID, domain.JobUpload, nil)
		return err
	})
	return v, err
}

// Reclamar toma el trabajo pendiente más antiguo que ya pueda intentarse, para
// `quien` (el hostname de la réplica), saltando los que otra réplica tiene
// cogidos. Antes devuelve a la cola los que llevan más de domain.Huerfano en
// `running`: su réplica murió sin terminarlos. ErrNoExiste si no hay nada.
func (s *Store) Reclamar(ctx context.Context, quien string) (domain.Job, error) {
	var j domain.Job
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE transcode_jobs
			   SET status = 'pending', claimed_by = '', error = 'réplica perdida: se reintenta', updated_at = now()
			 WHERE status = 'running' AND claimed_at < now() - $1::interval`, domain.Huerfano); err != nil {
			return fmt.Errorf("cast: recuperar trabajos huérfanos: %w", err)
		}
		var err error
		j, err = leerJob(tx.QueryRow(ctx, `
			UPDATE transcode_jobs
			   SET status = 'running', attempts = attempts + 1, claimed_at = now(), claimed_by = $1, updated_at = now()
			 WHERE id = (SELECT id FROM transcode_jobs
			              WHERE status = 'pending' AND next_attempt_at <= now()
			              ORDER BY next_attempt_at, created_at
			              FOR UPDATE SKIP LOCKED
			              LIMIT 1)
			 RETURNING `+columnasJob, quien))
		return err
	})
	return j, err
}

// VideoDelTrabajo lee el vídeo de un trabajo reclamado, en mantenimiento (el
// transcodificador no tiene organización propia).
func (s *Store) VideoDelTrabajo(ctx context.Context, j domain.Job) (domain.Video, error) {
	var v domain.Video
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		var err error
		v, err = leerVideo(tx.QueryRow(ctx, `SELECT `+columnas+` FROM videos WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL`, j.VideoID, j.Tenant))
		return err
	})
	return v, err
}

// ErrVideoCambio: el trabajo se cerró, pero el vídeo ya no estaba donde el
// trabajo lo dejó (lo borraron o cambió mientras). No es un fallo del trabajo;
// quien lo recibe solo lo registra.
var ErrVideoCambio = errors.New("cast: el vídeo cambió mientras se procesaba")

// Terminar marca el trabajo como hecho y guarda su resultado en la misma
// transacción: una subida o una grabación dejan el vídeo `ready` con sus
// derivadas; un póster solo cambia `poster_key`. Si el vídeo ya no está (borrado)
// o no está en `processing`, el trabajo se cierra igual y se devuelve
// ErrVideoCambio.
func (s *Store) Terminar(ctx context.Context, jobID uuid.UUID, resultado Estado) (domain.Video, error) {
	var v domain.Video
	var cambio bool
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		j, err := leerJob(tx.QueryRow(ctx, `SELECT `+columnasJob+` FROM transcode_jobs WHERE id = $1 AND status = 'running' FOR UPDATE`, jobID))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE transcode_jobs SET status = 'done', error = '', updated_at = now() WHERE id = $1`, jobID); err != nil {
			return fmt.Errorf("cast: terminar trabajo: %w", err)
		}
		if !j.Kind.MueveElVideo() {
			// Póster y calidades: el vídeo no cambia de estado, solo lo que el
			// trabajo produjo y lo que ahora ocupa.
			v, err = leerVideo(tx.QueryRow(ctx, `
				UPDATE videos
				   SET poster_key     = COALESCE($2, poster_key),
				       webm_key       = COALESCE($3, webm_key),
				       hls_qualities  = COALESCE($4, hls_qualities),
				       webm_qualities = COALESCE($5, webm_qualities),
				       bytes_total    = COALESCE($6, bytes_total),
				       updated_at     = now()
				 WHERE id = $1 AND tenant = $7 AND deleted_at IS NULL
				 RETURNING `+columnas, j.VideoID, resultado.PosterKey, resultado.WebmKey,
				resultado.HlsQualities, resultado.WebmQualities, resultado.BytesTotal, j.Tenant))
		} else {
			resultado.A = domain.StatusReady
			v, err = cambiarEstado(ctx, tx, j.Tenant, j.VideoID, resultado)
		}
		if errors.Is(err, domain.ErrNoExiste) || errors.Is(err, domain.ErrConflicto) {
			cambio = true
			return nil
		}
		if err != nil {
			return err
		}
		// La elección cambió mientras el trabajo se cocía: lo producido es de la
		// anterior, así que otra pasada la pone al día.
		if j.Kind != domain.JobPoster && !slices.Equal(v.Qualities, resultado.Elegidas) {
			_, err = encolar(ctx, tx, j.Tenant, j.VideoID, domain.JobQualities, nil)
		}
		return err
	})
	if err == nil && cambio {
		return domain.Video{}, ErrVideoCambio
	}
	return v, err
}

// esperaTrasFallo es cuánto espera un trabajo antes de reintentarse tras su
// intento n.º `intentos`: 30 s, 2 min, 8 min.
func esperaTrasFallo(intentos int32) time.Duration {
	espera := 30 * time.Second
	for i := int32(1); i < intentos; i++ {
		espera *= 4
	}
	return espera
}

// Fallar anota el error. Con intentos por debajo del tope, el trabajo vuelve a
// `pending` con una espera creciente y el vídeo sigue `processing`; al agotarlos,
// el trabajo queda `failed` y, si es una subida o una grabación, el vídeo también,
// con el motivo, para que la interfaz lo diga. Un póster fallido no toca el vídeo.
func (s *Store) Fallar(ctx context.Context, jobID uuid.UUID, motivo string) (reintenta bool, err error) {
	err = s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		j, err := leerJob(tx.QueryRow(ctx, `SELECT `+columnasJob+` FROM transcode_jobs WHERE id = $1 AND status = 'running' FOR UPDATE`, jobID))
		if err != nil {
			return err
		}
		// Tres reintentos: los intentos 1, 2 y 3 vuelven a la cola; el cuarto no.
		reintenta = j.Attempts <= domain.MaxIntentos
		if reintenta {
			_, err = tx.Exec(ctx, `
				UPDATE transcode_jobs
				   SET status = 'pending', error = $2, next_attempt_at = now() + $3::interval, updated_at = now()
				 WHERE id = $1`, jobID, motivo, esperaTrasFallo(j.Attempts))
			if err != nil {
				return fmt.Errorf("cast: fallar trabajo: %w", err)
			}
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE transcode_jobs SET status = 'failed', error = $2, updated_at = now() WHERE id = $1`, jobID, motivo); err != nil {
			return fmt.Errorf("cast: fallar trabajo: %w", err)
		}
		if !j.Kind.MueveElVideo() {
			return nil
		}
		_, err = cambiarEstado(ctx, tx, j.Tenant, j.VideoID, Estado{A: domain.StatusFailed, Failure: &motivo})
		// Un vídeo que ya no está en processing (lo borraron mientras) no es un fallo del trabajo.
		if errors.Is(err, domain.ErrNoExiste) || errors.Is(err, domain.ErrConflicto) {
			return nil
		}
		return err
	})
	return reintenta, err
}

// TrabajosDelVideo lista los trabajos de un vídeo, en su organización, el más
// reciente primero: lo que la ficha enseña de «qué se está cociendo».
func (s *Store) TrabajosDelVideo(ctx context.Context, tenant string, videoID uuid.UUID) ([]domain.Job, error) {
	var out []domain.Job
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+columnasJob+` FROM transcode_jobs WHERE video_id = $1 AND tenant = $2 ORDER BY created_at DESC`, videoID, tenant)
		if err != nil {
			return fmt.Errorf("cast: listar trabajos: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			j, err := leerJob(rows)
			if err != nil {
				return err
			}
			out = append(out, j)
		}
		return rows.Err()
	})
	return out, err
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// ElegirCalidades guarda la elección de calidades del vídeo y, si `siEncola` lo
// pide viendo la fila ya bloqueada, encola en la misma transacción el trabajo que
// pone lo generado al día. Así la elección y su trabajo no se separan: o quedan
// los dos, o ninguno.
func (s *Store) ElegirCalidades(ctx context.Context, tenant string, id uuid.UUID, qs []int32, siEncola func(domain.Video) bool) (domain.Video, error) {
	var v domain.Video
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var err error
		v, err = leerVideo(tx.QueryRow(ctx, `
			UPDATE videos SET qualities = $2, updated_at = now()
			 WHERE id = $1 AND tenant = $3 AND deleted_at IS NULL
			 RETURNING `+columnas, id, qs, tenant))
		if err != nil {
			return err
		}
		if siEncola != nil && siEncola(v) {
			_, err = encolar(ctx, tx, tenant, id, domain.JobQualities, nil)
		}
		return err
	})
	if esCheck(err) {
		return domain.Video{}, fmt.Errorf("%w: %w", domain.ErrEntradaInvalida, err)
	}
	return v, err
}

// EncolarCalidadesPendientes encola, en todas las organizaciones, el trabajo de
// calidades de los vídeos listos que aún tienen la escalera WebM de antes
// (webm_qualities NULL: solo 720p), los más recientes primero y como mucho
// `limite`. Devuelve cuántos encoló. Lo corren `cast encolar-calidades` y el
// barrido (RellenarCalidades).
//
// Un vídeo cuyo trabajo de calidades ya falló (agotó sus reintentos) no se vuelve
// a encolar: sin esto, el barrido lo reintentaría cada hora para siempre,
// gastando CPU en un original que ffmpeg no sabe leer.
func (s *Store) EncolarCalidadesPendientes(ctx context.Context, limite int) (int, error) {
	var n int
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO transcode_jobs (tenant, video_id, kind)
			SELECT tenant, id, 'qualities' FROM videos
			 WHERE deleted_at IS NULL AND status = 'ready'
			   AND webm_key IS NOT NULL AND webm_qualities IS NULL
			   AND NOT EXISTS (SELECT 1 FROM transcode_jobs j
			                    WHERE j.video_id = videos.id AND j.kind = 'qualities' AND j.status = 'failed')
			 ORDER BY created_at DESC
			 LIMIT $1
			ON CONFLICT (video_id, kind) WHERE status IN ('pending', 'running') DO NOTHING`, limite)
		if err != nil {
			return fmt.Errorf("cast: encolar calidades pendientes: %w", err)
		}
		n = int(tag.RowsAffected())
		return nil
	})
	return n, err
}

// CalidadesEnCola cuenta los trabajos de calidades pendientes o en marcha, en
// todas las organizaciones: lo que el barrido mira para no llenar la cola de golpe.
func (s *Store) CalidadesEnCola(ctx context.Context) (int, error) {
	var n int
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM transcode_jobs
			 WHERE kind = 'qualities' AND status IN ('pending', 'running')`).Scan(&n)
	})
	if err != nil {
		return 0, fmt.Errorf("cast: contar trabajos de calidades: %w", err)
	}
	return n, nil
}

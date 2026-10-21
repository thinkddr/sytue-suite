// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package social

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// RetencionChat es cuánto se conserva el chat de un directo.
const RetencionChat = 30 * 24 * time.Hour

// RetencionDenuncias es cuánto se conserva una denuncia ya resuelta o descartada,
// y con ella la copia de lo denunciado que guarda (extracto, nombre, texto de
// quien denuncia). Las abiertas no caducan: se conservan hasta que alguien las
// resuelve. Rooms usa el mismo plazo (services/rooms/internal/store/operador.go).
// La traza (`moderation_audit`) no se purga: no lleva contenido, y
// platform/pkg/auditlog no fija plazo para ninguna traza de la suite.
const RetencionDenuncias = 365 * 24 * time.Hour

// Limpiar borra los mensajes de chat más antiguos que RetencionChat, de todas
// las organizaciones, en una transacción de mantenimiento (`app.maintenance`).
//
// ponytail: por `created_at` y no por `live_ended_at` del vídeo: la diferencia
// es la duración del directo, y así no hay que abrir `videos` en mantenimiento.
func (s *Servicio) Limpiar(ctx context.Context) (int64, error) {
	n, err := s.porLotes(ctx, `
		DELETE FROM chat_messages
		 WHERE id IN (SELECT id FROM chat_messages
		               WHERE created_at < now() - $1::interval
		               ORDER BY created_at LIMIT 5000)`, RetencionChat)
	if err != nil {
		return n, fmt.Errorf("social: limpiar el chat: %w", err)
	}
	return n, nil
}

// PurgarDenuncias borra, de todas las organizaciones, las denuncias resueltas o
// descartadas hace más de RetencionDenuncias (política de mantenimiento,
// migración 0010).
func (s *Servicio) PurgarDenuncias(ctx context.Context) (int64, error) {
	n, err := s.porLotes(ctx, `
		DELETE FROM reports
		 WHERE id IN (SELECT id FROM reports
		               WHERE status <> 'open' AND resolved_at < now() - $1::interval
		               ORDER BY resolved_at LIMIT 5000)`, RetencionDenuncias)
	if err != nil {
		return n, fmt.Errorf("social: purgar denuncias: %w", err)
	}
	return n, nil
}

// porLotes repite un DELETE de hasta 5000 filas, cada uno en su transacción de
// mantenimiento, hasta que borre menos: así no se retiene un bloqueo largo si se
// acumuló mucho.
func (s *Servicio) porLotes(ctx context.Context, sql string, args ...any) (int64, error) {
	var total int64
	for {
		var n int64
		err := s.store.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, sql, args...)
			n = tag.RowsAffected()
			return err
		})
		total += n
		if err != nil || n < 5000 {
			return total, err
		}
	}
}

// BucleDeLimpieza corre Limpiar y PurgarDenuncias al arrancar y luego cada
// `cada`, hasta que el contexto termine. Para `cmd/cast`.
func (s *Servicio) BucleDeLimpieza(ctx context.Context, cada time.Duration) {
	tic := time.NewTicker(cada)
	defer tic.Stop()
	for {
		if n, err := s.Limpiar(ctx); err != nil {
			log.From(ctx).Warn("cast/chat: la limpieza falló", "error", err)
		} else if n > 0 {
			log.From(ctx).Info("cast/chat: mensajes retirados por antigüedad", "n", n)
		}
		if n, err := s.PurgarDenuncias(ctx); err != nil {
			log.From(ctx).Warn("cast/denuncias: la purga falló", "error", err)
		} else if n > 0 {
			log.From(ctx).Info("cast/denuncias: resueltas purgadas por antigüedad", "n", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-tic.C:
		}
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package directo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// datos son las consultas del directo: las claves de emisión, los espectadores
// y los pocos campos de `videos` que solo el directo toca (room, egress,
// grabación). Cada función es UNA transacción en la organización del vídeo,
// salvo dos que cruzan organizaciones a propósito: resolver de quién es un
// ingress (`enIngesta`, la única que fija `app.cast_ingest`) y listar los
// directos en curso para el sondeo (mantenimiento, solo lectura).
type datos struct {
	pool *pg.Pool
}

func (s *datos) enTenant(ctx context.Context, tenant string, fn func(tx pgx.Tx) error) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		return fn(tx)
	})
}

// enIngesta es la ÚNICA función que fija `app.cast_ingest`: solo lectura de
// `stream_keys` por ingress_id, para saber de qué organización es un aviso de
// LiveKit antes de abrir la transacción en ella.
func (s *datos) enIngesta(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.cast_ingest', 'on', true)"); err != nil {
			return fmt.Errorf("cast: set_config app.cast_ingest: %w", err)
		}
		return fn(tx)
	})
}

func esNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// ── Claves de emisión ───────────────────────────────────────────────────────

// Clave es la clave de emisión de una persona en una organización (fila de
// `stream_keys`), con el ingress de LiveKit que hay detrás.
type Clave struct {
	Tenant    string
	OwnerSub  string
	OwnerName string
	Key       string
	IngressID string
	CreatedAt time.Time
	RotatedAt *time.Time
}

const columnasClave = `tenant, owner_sub, owner_name, key, ingress_id, created_at, rotated_at`

func leerClave(row pgx.Row) (Clave, error) {
	var c Clave
	err := row.Scan(&c.Tenant, &c.OwnerSub, &c.OwnerName, &c.Key, &c.IngressID, &c.CreatedAt, &c.RotatedAt)
	if esNoRows(err) {
		return Clave{}, domain.ErrNoExiste
	}
	if err != nil {
		return Clave{}, fmt.Errorf("cast: leer clave de emisión: %w", err)
	}
	return c, nil
}

// clave lee la de una persona en su organización.
func (s *datos) clave(ctx context.Context, tenant, sub string) (Clave, error) {
	var c Clave
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var err error
		c, err = leerClave(tx.QueryRow(ctx, `SELECT `+columnasClave+` FROM stream_keys WHERE tenant = $1 AND owner_sub = $2`, tenant, sub))
		return err
	})
	return c, err
}

// guardarClave inserta la clave si la persona aún no tiene. Devuelve la que
// queda en la base y si es la que se pasó: dos peticiones a la vez crean dos
// ingresos, y solo uno cabe en la fila; quien llama borra el otro.
func (s *datos) guardarClave(ctx context.Context, c Clave) (Clave, bool, error) {
	var out Clave
	nueva := true
	err := s.enTenant(ctx, c.Tenant, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO stream_keys (tenant, owner_sub, owner_name, key, ingress_id)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (tenant, owner_sub) DO NOTHING
			RETURNING `+columnasClave, c.Tenant, c.OwnerSub, c.OwnerName, c.Key, c.IngressID)
		var err error
		out, err = leerClave(row)
		if errors.Is(err, domain.ErrNoExiste) {
			nueva = false
			out, err = leerClave(tx.QueryRow(ctx, `SELECT `+columnasClave+` FROM stream_keys WHERE tenant = $1 AND owner_sub = $2`, c.Tenant, c.OwnerSub))
		}
		return err
	})
	return out, nueva, err
}

// rotarClave sustituye la clave y el ingress de una persona y devuelve los
// anteriores, para que quien llama borre el ingress viejo en LiveKit.
func (s *datos) rotarClave(ctx context.Context, c Clave) (Clave, error) {
	var anterior Clave
	err := s.enTenant(ctx, c.Tenant, func(tx pgx.Tx) error {
		var err error
		anterior, err = leerClave(tx.QueryRow(ctx, `SELECT `+columnasClave+` FROM stream_keys WHERE tenant = $1 AND owner_sub = $2 FOR UPDATE`, c.Tenant, c.OwnerSub))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE stream_keys SET key = $2, ingress_id = $3, owner_name = $4, rotated_at = now()
			 WHERE owner_sub = $1 AND tenant = $5`, c.OwnerSub, c.Key, c.IngressID, c.OwnerName, c.Tenant)
		if err != nil {
			return fmt.Errorf("cast: rotar clave de emisión: %w", err)
		}
		return nil
	})
	return anterior, err
}

// clavePorIngress resuelve de quién es un ingress, en la organización que sea.
func (s *datos) clavePorIngress(ctx context.Context, ingressID string) (Clave, error) {
	var c Clave
	err := s.enIngesta(ctx, func(tx pgx.Tx) error {
		var err error
		c, err = leerClave(tx.QueryRow(ctx, `SELECT `+columnasClave+` FROM stream_keys WHERE ingress_id = $1`, ingressID))
		return err
	})
	return c, err
}

// ── Lo del vídeo que solo toca el directo ───────────────────────────────────

// fijarRoom apunta un directo recién programado a su room de LiveKit. Solo
// mientras está `scheduled`: el room de un directo en curso no se cambia.
func (s *datos) fijarRoom(ctx context.Context, tenant string, id uuid.UUID, room string) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE videos SET live_room = $2, updated_at = now()
			 WHERE id = $1 AND tenant = $3 AND deleted_at IS NULL AND kind = 'live' AND status = 'scheduled'`, id, room, tenant)
		if err != nil {
			return fmt.Errorf("cast: fijar room: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		return nil
	})
}

// videoActivoPorRoom es el directo programado o en curso que publica en esa
// room; el más reciente si hubiera más de uno (un room se reutiliza cuando OBS
// emite sin haber programado nada).
func (s *datos) videoActivoPorRoom(ctx context.Context, tenant, room string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id FROM videos
			 WHERE live_room = $1 AND tenant = $2 AND deleted_at IS NULL AND status IN ('scheduled', 'live')
			 ORDER BY created_at DESC LIMIT 1`, room, tenant).Scan(&id)
		if esNoRows(err) {
			return domain.ErrNoExiste
		}
		if err != nil {
			return fmt.Errorf("cast: vídeo por room: %w", err)
		}
		return nil
	})
	return id, err
}

// videoPorEgress es el vídeo cuya grabación es ese egress.
func (s *datos) videoPorEgress(ctx context.Context, tenant, egressID string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id FROM videos WHERE live_egress_id = $1 AND tenant = $2 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1`, egressID, tenant).Scan(&id)
		if esNoRows(err) {
			return domain.ErrNoExiste
		}
		if err != nil {
			return fmt.Errorf("cast: vídeo por egress: %w", err)
		}
		return nil
	})
	return id, err
}

// fijarEgress anota qué egress graba el directo (para pararlo y para reconocer
// su webhook al terminar).
func (s *datos) fijarEgress(ctx context.Context, tenant string, id uuid.UUID, egressID string) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE videos SET live_egress_id = $2, updated_at = now() WHERE id = $1 AND tenant = $3 AND deleted_at IS NULL`, id, egressID, tenant)
		if err != nil {
			return fmt.Errorf("cast: fijar egress: %w", err)
		}
		return nil
	})
}

// fijarGrabacion deja la playlist HLS grabada como `hls_key` del vídeo: el
// transcodificador leerá sus objetos y generará el WebM y el póster antes de publicar.
func (s *datos) fijarGrabacion(ctx context.Context, tenant string, id uuid.UUID, hlsKey string) (domain.Status, error) {
	var estado domain.Status
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		// El estado viene de la MISMA escritura: si el cierre ganó la fila, quien
		// termina de anotar el HLS debe encolarlo aunque antes hubiese leído live.
		err := tx.QueryRow(ctx, `UPDATE videos SET hls_key = $2, updated_at = now() WHERE id = $1 AND tenant = $3 AND deleted_at IS NULL AND kind = 'live' RETURNING status`, id, hlsKey, tenant).Scan(&estado)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNoExiste
		}
		if err != nil {
			return fmt.Errorf("cast: fijar grabación: %w", err)
		}
		return nil
	})
	return estado, err
}

// fallarGrabacion anota un resultado terminal para no sondearlo cada cinco segundos.
// Sigue ended y nunca inventa un HLS ni publica un VOD sin objetos.
func (s *datos) fallarGrabacion(ctx context.Context, tenant string, id uuid.UUID, motivo string) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE videos SET failure = $3, updated_at = now()
			WHERE id = $1 AND tenant = $2 AND kind = 'live' AND status = 'ended'
			  AND deleted_at IS NULL AND COALESCE(hls_key, '') = ''`, id, tenant, motivo)
		return err
	})
}

// activo es un directo programado o en curso, tal como lo ve el sondeo.
type activo struct {
	Tenant   string
	ID       uuid.UUID
	Room     string
	Status   domain.Status
	EgressID string
}

// directosEnCurso incluye también las grabaciones ended aún sin resolver. Un fallo
// terminal ya anotado no vuelve a consultarse; las encoladas pasan a processing.
func (s *datos) directosEnCurso(ctx context.Context) ([]activo, error) {
	var out []activo
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT tenant, id, live_room, status, COALESCE(live_egress_id, '')
			  FROM videos
			 WHERE deleted_at IS NULL AND kind = 'live' AND live_room IS NOT NULL
			   AND (status IN ('scheduled', 'live') OR
			        (status = 'ended' AND COALESCE(live_egress_id, '') <> '' AND failure = ''))`)
		if err != nil {
			return fmt.Errorf("cast: listar directos en curso: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var a activo
			if err := rows.Scan(&a.Tenant, &a.ID, &a.Room, &a.Status, &a.EgressID); err != nil {
				return fmt.Errorf("cast: leer directo en curso: %w", err)
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

// ── Espectadores ────────────────────────────────────────────────────────────

// recontar fija `viewers_now` con lo que hay en `live_viewers` y sube el pico,
// dentro de la transacción que acaba de tocar la lista: una petición, una
// transacción (por eso no se llama a Store.Espectadores, que abriría otra).
func recontar(ctx context.Context, tx pgx.Tx, tenant string, videoID uuid.UUID) (int32, error) {
	var n int32
	err := tx.QueryRow(ctx, `
		WITH n AS (SELECT count(*)::integer AS c FROM live_viewers WHERE video_id = $1 AND tenant = $2)
		UPDATE videos SET viewers_now = n.c, viewers_peak = GREATEST(viewers_peak, n.c)
		  FROM n WHERE videos.id = $1 AND videos.tenant = $2 AND videos.deleted_at IS NULL
		RETURNING n.c`, videoID, tenant).Scan(&n)
	if esNoRows(err) {
		return 0, domain.ErrNoExiste
	}
	if err != nil {
		return 0, fmt.Errorf("cast: recontar espectadores: %w", err)
	}
	return n, nil
}

// espectador anota que alguien entra (o sale) del directo y devuelve cuántos
// quedan. Idempotente: entrar dos veces es una fila; salir sin haber entrado, nada.
func (s *datos) espectador(ctx context.Context, tenant string, videoID uuid.UUID, identity string, entra bool) (int32, error) {
	var n int32
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var err error
		if entra {
			_, err = tx.Exec(ctx, `INSERT INTO live_viewers (video_id, tenant, identity) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, videoID, tenant, identity)
		} else {
			_, err = tx.Exec(ctx, `DELETE FROM live_viewers WHERE video_id = $1 AND identity = $2 AND tenant = $3`, videoID, identity, tenant)
		}
		if err != nil {
			return fmt.Errorf("cast: anotar espectador: %w", err)
		}
		n, err = recontar(ctx, tx, tenant, videoID)
		return err
	})
	return n, err
}

// espectadores deja la lista exactamente como la ve LiveKit (el sondeo): entra
// quien falta, sale quien ya no está.
func (s *datos) espectadores(ctx context.Context, tenant string, videoID uuid.UUID, identidades []string) (int32, error) {
	if identidades == nil {
		identidades = []string{}
	}
	var n int32
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM live_viewers WHERE video_id = $1 AND tenant = $3 AND identity <> ALL($2::text[])`, videoID, identidades, tenant); err != nil {
			return fmt.Errorf("cast: retirar espectadores: %w", err)
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO live_viewers (video_id, tenant, identity)
			SELECT $1, $2, unnest($3::text[])
			ON CONFLICT DO NOTHING`, videoID, tenant, identidades)
		if err != nil {
			return fmt.Errorf("cast: anotar espectadores: %w", err)
		}
		n, err = recontar(ctx, tx, tenant, videoID)
		return err
	})
	return n, err
}

// vaciarEspectadores retira a todos al terminar el directo; el pico se queda.
func (s *datos) vaciarEspectadores(ctx context.Context, tenant string, videoID uuid.UUID) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM live_viewers WHERE video_id = $1 AND tenant = $2`, videoID, tenant); err != nil {
			return fmt.Errorf("cast: vaciar espectadores: %w", err)
		}
		_, err := recontar(ctx, tx, tenant, videoID)
		if errors.Is(err, domain.ErrNoExiste) {
			return nil
		}
		return err
	})
}

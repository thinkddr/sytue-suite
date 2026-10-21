// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// columnas es la proyección de `videos` que escanea leerVideo. Va en una
// constante para que todas las consultas devuelvan lo mismo en el mismo orden.
const columnas = `id, tenant, owner_sub, owner_name, kind, status, visibility, title, description,
       duration_ms, width, height,
       COALESCE(original_key, ''), COALESCE(hls_key, ''), COALESCE(webm_key, ''), COALESCE(poster_key, ''),
       bytes_original, bytes_total, comments_enabled, chat_enabled, download_enabled,
       COALESCE(live_room, ''), COALESCE(live_source, ''), live_started_at, live_ended_at, COALESCE(live_egress_id, ''),
       viewers_now, viewers_peak, views, failure, published_at, created_at, updated_at,
       qualities, hls_qualities, webm_qualities`

func leerVideo(row pgx.Row) (domain.Video, error) {
	var v domain.Video
	err := row.Scan(&v.ID, &v.Tenant, &v.OwnerSub, &v.OwnerName, &v.Kind, &v.Status, &v.Visibility, &v.Title, &v.Description,
		&v.DurationMS, &v.Width, &v.Height,
		&v.OriginalKey, &v.HlsKey, &v.WebmKey, &v.PosterKey,
		&v.BytesOriginal, &v.BytesTotal, &v.CommentsEnabled, &v.ChatEnabled, &v.DownloadEnabled,
		&v.LiveRoom, &v.LiveSource, &v.LiveStartedAt, &v.LiveEndedAt, &v.LiveEgressID,
		&v.ViewersNow, &v.ViewersPeak, &v.Views, &v.Failure, &v.PublishedAt, &v.CreatedAt, &v.UpdatedAt,
		&v.Qualities, &v.HlsQualities, &v.WebmQualities)
	if esNoRows(err) {
		return domain.Video{}, domain.ErrNoExiste
	}
	if err != nil {
		return domain.Video{}, fmt.Errorf("cast: leer vídeo: %w", err)
	}
	return v, nil
}

func leerVideos(rows pgx.Rows) ([]domain.Video, error) {
	defer rows.Close()
	var out []domain.Video
	for rows.Next() {
		v, err := leerVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Nuevo es lo que hace falta para dar de alta un vídeo.
type Nuevo struct {
	Tenant, OwnerSub, OwnerName string
	Kind                        domain.Kind
	Visibility                  domain.Visibility
	Title, Description          string
	// Solo para un directo: por dónde entra y en qué room de LiveKit.
	LiveSource domain.LiveSource
	LiveRoom   string
	// Qualities es la elección de calidades (nil = la recomendada).
	Qualities []int32
}

// Crear da de alta el vídeo en su estado inicial.
func (s *Store) Crear(ctx context.Context, n Nuevo) (domain.Video, error) {
	var v domain.Video
	err := s.enTenant(ctx, n.Tenant, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO videos (tenant, owner_sub, owner_name, kind, status, visibility, title, description, live_source, live_room, qualities)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), NULLIF($10, ''), $11)
			RETURNING `+columnas,
			n.Tenant, n.OwnerSub, n.OwnerName, n.Kind, domain.EstadoInicial(n.Kind), n.Visibility, n.Title, n.Description,
			string(n.LiveSource), n.LiveRoom, n.Qualities)
		var err error
		v, err = leerVideo(row)
		return err
	})
	if esCheck(err) {
		return domain.Video{}, fmt.Errorf("%w: %w", domain.ErrEntradaInvalida, err)
	}
	return v, err
}

// EnOrganizacion lee un vídeo de la organización dada (no borrado). Cualquier
// estado: es la lectura de quien lo edita; quién más lo ve lo decide `acceso`.
func (s *Store) EnOrganizacion(ctx context.Context, tenant string, id uuid.UUID) (domain.Video, error) {
	var v domain.Video
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var err error
		v, err = leerVideo(tx.QueryRow(ctx, `SELECT `+columnas+` FROM videos WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL`, id, tenant))
		return err
	})
	return v, err
}

// abierto y visible son, escritas en el SQL, las condiciones de las políticas
// `videos_publico` y `videos_persona`: lo que se puede leer cruzando
// organizaciones. Van AQUÍ además de en la política porque una conexión que se
// salte la RLS (superusuario, BYPASSRLS) no evalúa ninguna, y entonces lo único
// que separa un vídeo de organización de la puerta pública es esta consulta.
const (
	visible = `deleted_at IS NULL AND status IN ('ready', 'live')`
	abierto = visible + ` AND visibility IN ('public', 'unlisted')`
	// invitado pide el sub de la persona en $2.
	invitado = visible + ` AND visibility = 'people'
	   AND EXISTS (SELECT 1 FROM video_people p WHERE p.tenant = videos.tenant AND p.video_id = videos.id AND p.user_id = $2)`
)

// Publico lee un vídeo por la política pública: solo public/unlisted ya visibles,
// de cualquier organización.
func (s *Store) Publico(ctx context.Context, id uuid.UUID) (domain.Video, error) {
	var v domain.Video
	err := s.enPublico(ctx, func(tx pgx.Tx) error {
		var err error
		v, err = leerVideo(tx.QueryRow(ctx, `SELECT `+columnas+` FROM videos WHERE id = $1 AND `+abierto, id))
		return err
	})
	return v, err
}

// Invitado lee un vídeo `people` en el que está esa persona, de cualquier organización.
func (s *Store) Invitado(ctx context.Context, sub string, id uuid.UUID) (domain.Video, error) {
	var v domain.Video
	err := s.enPersona(ctx, sub, func(tx pgx.Tx) error {
		var err error
		v, err = leerVideo(tx.QueryRow(ctx, `SELECT `+columnas+` FROM videos WHERE id = $1 AND `+invitado, id, sub))
		return err
	})
	return v, err
}

// Filtro acota un listado con sesión (§5.2 `GET /videos`).
type Filtro struct {
	// Solo los del dueño (`scope=mine`). Vacío = la organización (`scope=org`).
	OwnerSub string
	// Status vacío = todos.
	Status domain.Status
	// Q busca en el título (contiene, sin distinguir mayúsculas).
	Q string
	// Desde y Limite paginan por (created_at, id) descendente.
	Desde  *Cursor
	Limite int
}

// Biblioteca lista lo que ve alguien de la organización que NO es el dueño:
// public, unlisted y org ya visibles, más los `people` en los que está, más todo
// lo suyo en cualquier estado (y todo, para quien administra). Es la regla de
// `acceso` en SQL, para no traer la organización entera y filtrarla en Go.
func (s *Store) Biblioteca(ctx context.Context, tenant, sub string, admin bool, f Filtro) ([]domain.Video, error) {
	fecha, id := argsCursor(f.Desde)
	var out []domain.Video
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+columnas+`
			  FROM videos v
			 WHERE v.tenant = $9 AND v.deleted_at IS NULL
			   AND ($1 = '' OR v.owner_sub = $1)
			   AND ($2 = '' OR v.status = $2)
			   AND ($3 = '' OR plegar(v.title) LIKE '%' || plegar($3) || '%')
			   AND (v.owner_sub = $4 OR $5
			        OR (v.status IN ('ready', 'live') AND (
			              v.visibility IN ('public', 'unlisted', 'org')
			           OR (v.visibility = 'people' AND EXISTS (
			                 SELECT 1 FROM video_people p WHERE p.tenant = v.tenant AND p.video_id = v.id AND p.user_id = $4)))))
			   AND ($6::timestamptz IS NULL OR (v.created_at, v.id) < ($6::timestamptz, $7::uuid))
			 ORDER BY v.created_at DESC, v.id DESC
			 LIMIT $8`,
			f.OwnerSub, string(f.Status), escaparLike(f.Q), sub, admin, fecha, id, f.Limite, tenant)
		if err != nil {
			return fmt.Errorf("cast: listar biblioteca: %w", err)
		}
		out, err = leerVideos(rows)
		return err
	})
	return out, err
}

// escaparLike deja el texto listo para ir dentro de un ILIKE como literal.
func escaparLike(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(q)
}

// Explore es la portada pública (§5.3): primero los directos `live` públicos,
// luego los vídeos `public` listos por publicación descendente, de TODAS las
// organizaciones. El cursor recorre solo la segunda parte: los directos en curso
// son pocos y van siempre delante, sin paginar.
func (s *Store) Explore(ctx context.Context, desde *Cursor, limite int) (directos, videos []domain.Video, err error) {
	fecha, id := argsCursor(desde)
	err = s.enPublico(ctx, func(tx pgx.Tx) error {
		if desde == nil {
			rows, err := tx.Query(ctx, `
				SELECT `+columnas+` FROM videos
				 WHERE visibility = 'public' AND status = 'live' AND deleted_at IS NULL
				 ORDER BY live_started_at DESC NULLS LAST, id DESC
				 LIMIT 50`)
			if err != nil {
				return fmt.Errorf("cast: listar directos: %w", err)
			}
			if directos, err = leerVideos(rows); err != nil {
				return err
			}
		}
		rows, err := tx.Query(ctx, `
			SELECT `+columnas+` FROM videos
			 WHERE visibility = 'public' AND status = 'ready' AND deleted_at IS NULL
			   AND ($1::timestamptz IS NULL OR (published_at, id) < ($1::timestamptz, $2::uuid))
			 ORDER BY published_at DESC, id DESC
			 LIMIT $3`, fecha, id, limite)
		if err != nil {
			return fmt.Errorf("cast: listar portada: %w", err)
		}
		videos, err = leerVideos(rows)
		return err
	})
	return directos, videos, err
}

// Canal lista lo público ya visible (live y ready) de UNA organización, lo más
// reciente primero (§5.3 `GET /public/c/{org}`).
func (s *Store) Canal(ctx context.Context, tenant string, desde *Cursor, limite int) ([]domain.Video, error) {
	fecha, id := argsCursor(desde)
	var out []domain.Video
	err := s.enPublico(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+columnas+` FROM videos
			 WHERE visibility = 'public' AND tenant = $1 AND `+visible+`
			   AND ($2::timestamptz IS NULL OR (published_at, id) < ($2::timestamptz, $3::uuid))
			 ORDER BY published_at DESC, id DESC
			 LIMIT $4`, tenant, fecha, id, limite)
		if err != nil {
			return fmt.Errorf("cast: listar canal: %w", err)
		}
		out, err = leerVideos(rows)
		return err
	})
	return out, err
}

// Compartidos lista los vídeos `people` de OTRAS organizaciones en los que está
// esa persona (los de la suya salen ya en Biblioteca).
func (s *Store) Compartidos(ctx context.Context, sub, tenantPropio string, desde *Cursor, limite int) ([]domain.Video, error) {
	fecha, id := argsCursor(desde)
	var out []domain.Video
	err := s.enPersona(ctx, sub, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+columnas+` FROM videos
			 WHERE tenant <> $1
			   AND `+visible+` AND visibility = 'people'
			   AND EXISTS (SELECT 1 FROM video_people p WHERE p.tenant = videos.tenant AND p.video_id = videos.id AND p.user_id = $5)
			   AND ($2::timestamptz IS NULL OR (created_at, id) < ($2::timestamptz, $3::uuid))
			 ORDER BY created_at DESC, id DESC
			 LIMIT $4`, tenantPropio, fecha, id, limite, sub)
		if err != nil {
			return fmt.Errorf("cast: listar compartidos: %w", err)
		}
		out, err = leerVideos(rows)
		return err
	})
	return out, err
}

// Actualizar aplica los cambios del dueño. COALESCE deja lo que no viene.
func (s *Store) Actualizar(ctx context.Context, tenant string, id uuid.UUID, c domain.Cambios) (domain.Video, error) {
	var v domain.Video
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE videos
			   SET title            = COALESCE($2, title),
			       description      = COALESCE($3, description),
			       visibility       = COALESCE($4, visibility),
			       comments_enabled = COALESCE($5, comments_enabled),
			       chat_enabled     = COALESCE($6, chat_enabled),
			       download_enabled = COALESCE($7, download_enabled),
			       updated_at       = now()
			 WHERE id = $1 AND tenant = $8 AND deleted_at IS NULL
			 RETURNING `+columnas,
			id, c.Title, c.Description, (*string)(c.Visibility), c.CommentsEnabled, c.ChatEnabled, c.DownloadEnabled, tenant)
		var err error
		v, err = leerVideo(row)
		return err
	})
	if esCheck(err) {
		return domain.Video{}, fmt.Errorf("%w: %w", domain.ErrEntradaInvalida, err)
	}
	return v, err
}

// Estado es un cambio de ciclo de vida. Lo que lo acompaña es opcional: el
// transcodificador rellena medidas, claves y bytes; el directo, room y egress; un
// fallo, su motivo.
type Estado struct {
	A             domain.Status
	DurationMS    *int64
	Width, Height *int32
	OriginalKey   *string
	HlsKey        *string
	WebmKey       *string
	PosterKey     *string
	BytesOriginal *int64
	BytesTotal    *int64
	LiveRoom      *string
	LiveEgressID  *string
	Failure       *string
	// Las calidades que existen en cada escalera tras el trabajo (nil = no cambian).
	HlsQualities  []int32
	WebmQualities []int32
	// Elegidas es la elección que el trabajo vio al empezar (nil = la
	// recomendada): si al terminar ya es otra —alguien la cambió mientras—,
	// Terminar encola otra pasada.
	Elegidas []int32
}

const sqlCambiarEstado = `
	UPDATE videos
	   SET status          = $2,
	       duration_ms     = COALESCE($3, duration_ms),
	       width           = COALESCE($4, width),
	       height          = COALESCE($5, height),
	       original_key    = COALESCE($6, original_key),
	       hls_key         = COALESCE($7, hls_key),
	       webm_key        = COALESCE($8, webm_key),
	       poster_key      = COALESCE($9, poster_key),
	       bytes_original  = COALESCE($10, bytes_original),
	       bytes_total     = COALESCE($11, bytes_total),
	       live_room       = COALESCE($12, live_room),
	       live_egress_id  = COALESCE($13, live_egress_id),
	       hls_qualities   = COALESCE($15, hls_qualities),
	       webm_qualities  = COALESCE($16, webm_qualities),
	       failure         = CASE WHEN $2 = 'failed' THEN COALESCE($14, failure) ELSE '' END,
	       live_started_at = CASE WHEN $2 = 'live'  AND live_started_at IS NULL THEN now() ELSE live_started_at END,
	       live_ended_at   = CASE WHEN $2 = 'ended' AND live_ended_at   IS NULL THEN now() ELSE live_ended_at   END,
	       published_at    = CASE WHEN $2 IN ('ready', 'live') AND published_at IS NULL THEN now() ELSE published_at END,
	       updated_at      = now()
	 WHERE id = $1 AND tenant = $17
	 RETURNING ` + columnas

func argsEstado(tenant string, id uuid.UUID, e Estado) []any {
	return []any{
		id, e.A, e.DurationMS, e.Width, e.Height, e.OriginalKey, e.HlsKey, e.WebmKey, e.PosterKey,
		e.BytesOriginal, e.BytesTotal, e.LiveRoom, e.LiveEgressID, e.Failure, e.HlsQualities, e.WebmQualities,
		tenant,
	}
}

// cambiarEstado comprueba la transición y la aplica dentro de la transacción
// dada (la lectura con FOR UPDATE, para que dos trabajadores no se pisen). La
// organización va SIEMPRE, también desde mantenimiento, que la saca del trabajo.
func cambiarEstado(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID, e Estado) (domain.Video, error) {
	actual, err := leerVideo(tx.QueryRow(ctx, `SELECT `+columnas+` FROM videos WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL FOR UPDATE`, id, tenant))
	if err != nil {
		return domain.Video{}, err
	}
	if !domain.PuedePasar(actual.Status, e.A) {
		return domain.Video{}, fmt.Errorf("%w: de %s a %s", domain.ErrConflicto, actual.Status, e.A)
	}
	return leerVideo(tx.QueryRow(ctx, sqlCambiarEstado, argsEstado(tenant, id, e)...))
}

// CambiarEstado pasa el vídeo a otro estado en su organización, si la
// transición es válida, y fija `published_at` la primera vez que se puede ver.
func (s *Store) CambiarEstado(ctx context.Context, tenant string, id uuid.UUID, e Estado) (domain.Video, error) {
	var v domain.Video
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var err error
		v, err = cambiarEstado(ctx, tx, tenant, id, e)
		return err
	})
	return v, err
}

// Borrar retira el vídeo (borrado blando) y devuelve lo borrado, con sus claves,
// para que quien llama purgue los objetos y recalcule la cuota. Lo que cuelga
// —personas, comentarios, chat— sigue en la base; los listados y las políticas
// públicas ya no lo enseñan.
func (s *Store) Borrar(ctx context.Context, tenant string, id uuid.UUID) (domain.Video, error) {
	var v domain.Video
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var err error
		v, err = leerVideo(tx.QueryRow(ctx, `
			UPDATE videos SET deleted_at = now(), updated_at = now(), bytes_total = 0
			 WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL
			 RETURNING `+columnas, id, tenant))
		if err != nil {
			return err
		}
		// Lo que quedaba en cola ya no tiene a quién servir. Lo que está en marcha
		// lo cierra su réplica al terminar (Terminar tolera el vídeo borrado).
		if _, err := tx.Exec(ctx, `
			UPDATE transcode_jobs SET status = 'failed', error = 'vídeo borrado', updated_at = now()
			 WHERE video_id = $1 AND tenant = $2 AND status = 'pending'`, id, tenant); err != nil {
			return fmt.Errorf("cast: cancelar trabajos del vídeo: %w", err)
		}
		return nil
	})
	return v, err
}

// RegistrarVista suma una visualización. Quién la cuenta y cuándo (una por vídeo,
// IP y hora) lo decide el servicio; aquí solo se suma, en la organización del
// vídeo, sin tocar `updated_at` (una vista no es una edición).
func (s *Store) RegistrarVista(ctx context.Context, tenant string, id uuid.UUID) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE videos SET views = views + 1 WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL`, id, tenant)
		if err != nil {
			return fmt.Errorf("cast: registrar vista: %w", err)
		}
		return nil
	})
}

// Espectadores fija el recuento del directo y sube el pico si hace falta. Lo llama
// el directo desde el webhook o el sondeo de LiveKit.
func (s *Store) Espectadores(ctx context.Context, tenant string, id uuid.UUID, ahora int32) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE videos SET viewers_now = $2, viewers_peak = GREATEST(viewers_peak, $2)
			 WHERE id = $1 AND tenant = $3 AND deleted_at IS NULL`, id, ahora, tenant)
		if err != nil {
			return fmt.Errorf("cast: espectadores: %w", err)
		}
		return nil
	})
}

// BytesDeOrganizacion suma lo que ocupa Cast en una organización: lo que se declara
// a Drive como uso externo (§8). Los borrados ya tienen bytes_total = 0.
func (s *Store) BytesDeOrganizacion(ctx context.Context, tenant string) (int64, error) {
	var n int64
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT COALESCE(SUM(bytes_total), 0) FROM videos WHERE tenant = $1 AND deleted_at IS NULL`, tenant).Scan(&n)
	})
	if err != nil {
		return 0, fmt.Errorf("cast: sumar bytes: %w", err)
	}
	return n, nil
}

// ── Personas ────────────────────────────────────────────────────────────────

// Personas lista con quién está compartido el vídeo.
func (s *Store) Personas(ctx context.Context, tenant string, id uuid.UUID) ([]domain.Persona, error) {
	var out []domain.Persona
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT user_id, name, username, added_at FROM video_people
			 WHERE video_id = $1 AND tenant = $2 ORDER BY added_at, user_id`, id, tenant)
		if err != nil {
			return fmt.Errorf("cast: listar personas: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var p domain.Persona
			if err := rows.Scan(&p.UserID, &p.Name, &p.Username, &p.AddedAt); err != nil {
				return fmt.Errorf("cast: leer persona: %w", err)
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// AnadirPersona comparte el vídeo con alguien. Repetirlo refresca su nombre.
// Devuelve ErrEntradaInvalida al pasar de MaxPersonas.
func (s *Store) AnadirPersona(ctx context.Context, tenant string, id uuid.UUID, p domain.Persona, porSub string) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM video_people WHERE video_id = $1 AND tenant = $3 AND user_id <> $2`, id, p.UserID, tenant).Scan(&n); err != nil {
			return fmt.Errorf("cast: contar personas: %w", err)
		}
		if n >= domain.MaxPersonas {
			return fmt.Errorf("%w: un vídeo se comparte con %d personas como máximo", domain.ErrEntradaInvalida, domain.MaxPersonas)
		}
		// La fila sale del vídeo DE ESA organización: sin él no se inserta nada. Con
		// VALUES bastaba la clave ajena para colgar una persona del vídeo de otra.
		tag, err := tx.Exec(ctx, `
			INSERT INTO video_people (tenant, video_id, user_id, name, username, added_by)
			SELECT v.tenant, v.id, $3, $4, $5, $6 FROM videos v
			 WHERE v.id = $2 AND v.tenant = $1 AND v.deleted_at IS NULL
			ON CONFLICT (video_id, user_id) DO UPDATE SET name = EXCLUDED.name, username = EXCLUDED.username`,
			tenant, id, p.UserID, p.Name, p.Username, porSub)
		if err != nil {
			return fmt.Errorf("cast: añadir persona: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		return nil
	})
}

// QuitarPersona deja de compartir el vídeo con alguien.
func (s *Store) QuitarPersona(ctx context.Context, tenant string, id uuid.UUID, userID string) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM video_people WHERE video_id = $1 AND user_id = $2 AND tenant = $3`, id, userID, tenant)
		if err != nil {
			return fmt.Errorf("cast: quitar persona: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		return nil
	})
}

// EsInvitado dice si esa persona está en la lista del vídeo. Lee por la política
// de persona, así que vale para alguien de otra organización.
func (s *Store) EsInvitado(ctx context.Context, sub string, id uuid.UUID) (bool, error) {
	var ok bool
	err := s.enPersona(ctx, sub, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM video_people WHERE video_id = $1 AND user_id = $2)`, id, sub).Scan(&ok)
	})
	return ok, err
}

// FijarPoster apunta el vídeo a un póster propio ya guardado en el almacén.
func (s *Store) FijarPoster(ctx context.Context, tenant string, id uuid.UUID, clave string) (domain.Video, error) {
	var v domain.Video
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var err error
		v, err = leerVideo(tx.QueryRow(ctx, `
			UPDATE videos SET poster_key = $2, updated_at = now()
			 WHERE id = $1 AND tenant = $3 AND deleted_at IS NULL
			 RETURNING `+columnas, id, clave, tenant))
		return err
	})
	return v, err
}

// PorID lee un vídeo sabiendo solo su id, en mantenimiento: es lo que sirve /media,
// cuyo token firmado prueba que quien lo tiene pasó por `play`. Solo vídeos que
// ya tienen derivadas que servir (ready, live, o processing con original), no lo
// que aún se está subiendo.
func (s *Store) PorID(ctx context.Context, id uuid.UUID) (domain.Video, error) {
	var v domain.Video
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		var err error
		v, err = leerVideo(tx.QueryRow(ctx, `
			SELECT `+columnas+` FROM videos
			 WHERE id = $1 AND deleted_at IS NULL AND status <> 'uploading'`, id))
		return err
	})
	return v, err
}

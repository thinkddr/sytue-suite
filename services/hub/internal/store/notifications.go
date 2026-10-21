// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

const notificationCols = `tenant, id, subject, source, type, level, title, body, url, params, read, created_at, read_at, clave_origen IS NOT NULL`

func scanNotification(row pgx.Row) (domain.Notification, error) {
	var n domain.Notification
	var source, level string
	if err := row.Scan(&n.Tenant, &n.ID, &n.Subject, &source, &n.Type, &level,
		&n.Title, &n.Body, &n.URL, &n.Params, &n.Read, &n.CreatedAt, &n.ReadAt, &n.Historic); err != nil {
		return domain.Notification{}, err
	}
	if len(n.Params) == 0 {
		n.Params = nil // `{}` en la base; en el JSON, sin campo
	}
	n.Source = domain.Source(source)
	n.Level = domain.NotificationLevel(level)
	return n, nil
}

// CreateNotification inserta una notificación para un usuario y devuelve el
// registro persistido. La publicación del NOTIFY de tiempo real la hace el
// servicio (notify) en la misma transacción vía InsertNotificationTx.
func (s *Store) CreateNotification(ctx context.Context, rec domain.NewNotification) (domain.Notification, error) {
	t := tenancy.MustFrom(ctx)
	var n domain.Notification
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		n, e = InsertNotificationTx(ctx, tx, t.Slug, rec)
		return e
	})
	return n, err
}

// InsertNotificationTx inserta una notificación dentro de la transacción dada
// (para que el NOTIFY de tiempo real viaje atómicamente con la escritura).
func InsertNotificationTx(ctx context.Context, tx pgx.Tx, tenant string, rec domain.NewNotification) (domain.Notification, error) {
	params := rec.Params
	if params == nil {
		params = map[string]string{} // la columna es NOT NULL: nil sería un NULL
	}
	row := tx.QueryRow(ctx,
		`INSERT INTO notifications (tenant, subject, source, type, level, title, body, url, params)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING `+notificationCols,
		tenant, rec.Subject, string(rec.Source), rec.Type, string(rec.Level),
		rec.Title, rec.Body, rec.URL, params)
	return scanNotification(row)
}

// InsertHistoricoTx guarda un aviso reconstruido (domain.NewHistorico): con su fecha,
// ya leído y con su clave de origen. Si esa clave ya la tiene su destinatario no hace
// nada y devuelve false: lanzar la reconstrucción dos veces no duplica nada.
func InsertHistoricoTx(ctx context.Context, tx pgx.Tx, tenant string, h domain.NewHistorico) (bool, error) {
	params := h.Params
	if params == nil {
		params = map[string]string{}
	}
	tag, err := tx.Exec(ctx,
		`INSERT INTO notifications (tenant, subject, source, type, level, title, body, url, params,
		                            read, read_at, created_at, clave_origen)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, true, now(), $10, $11)
		 ON CONFLICT (tenant, subject, clave_origen) WHERE clave_origen IS NOT NULL DO NOTHING`,
		tenant, h.Subject, string(h.Source), h.Type, string(h.Level),
		h.Title, h.Body, h.URL, params, h.Fecha, h.Clave)
	return tag.RowsAffected() == 1, err
}

// PrimerAvisoEnVivo es cuándo dejó la organización su primer aviso EN VIVO de ese
// tipo: desde ahí el registro ya funcionaba, y lo que pasó después no se reconstruye
// (lo tiene, o lo calló su destinatario). Nil si todavía no hay ninguno.
func PrimerAvisoEnVivo(ctx context.Context, tx pgx.Tx, tenant, tipo string) (*time.Time, error) {
	var t *time.Time
	err := tx.QueryRow(ctx,
		`SELECT min(created_at) FROM notifications
		 WHERE tenant = $1 AND type = $2 AND clave_origen IS NULL`, tenant, tipo).Scan(&t)
	return t, err
}

// BuzonOlvidado dice si Sytue ID retiró alguna vez esa dirección (OlvidarBuzon).
func BuzonOlvidado(ctx context.Context, tx pgx.Tx, subject string) (bool, error) {
	var si bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM buzones_olvidados WHERE buzon = $1)`, subject).Scan(&si)
	return si, err
}

// DeLaPersona cuenta los avisos de `subject` en TODAS las organizaciones: los de la
// última hora o, con `historicos`, los reconstruidos. Ve las demás organizaciones con la
// política de destinatario (0005), fijando `app.destinatarios` SOLO a ese buzón mientras
// cuenta y dejando después el valor que tenía la transacción.
//
// Antes, un cerrojo por destinatario hasta el final de la transacción: sin él, varias
// creaciones a la vez leían el mismo recuento y pasaban todas el tope.
func DeLaPersona(ctx context.Context, tx pgx.Tx, subject string, historicos bool) (int, error) {
	if strings.Contains(subject, separadorDestinatarios) {
		return 0, domain.ErrInvalidInput
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('hub:persona:' || $1, 0))`, subject); err != nil {
		return 0, err
	}
	var previo string
	if err := tx.QueryRow(ctx, `SELECT coalesce(current_setting('app.destinatarios', true), '')`).Scan(&previo); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.destinatarios', $1, true)", subject); err != nil {
		return 0, err
	}
	// Dos consultas literales, no un CASE: con plan genérico, el CASE impedía usar
	// el índice por (subject, created_at) en la ruta caliente de cada aviso en vivo.
	consulta := `SELECT count(*) FROM notifications WHERE subject = $1 AND created_at > now() - interval '1 hour'`
	if historicos {
		consulta = `SELECT count(*) FROM notifications WHERE subject = $1 AND clave_origen IS NOT NULL`
	}
	var n int
	if err := tx.QueryRow(ctx, consulta, subject).Scan(&n); err != nil {
		return 0, err
	}
	_, err := tx.Exec(ctx, "SELECT set_config('app.destinatarios', $1, true)", previo)
	return n, err
}

// ListNotifications lista las notificaciones de un usuario con paginación keyset
// descendente por (created_at, id). before es opcional (nil = primera página).
//
// `subjects` son los buzones de la persona: su `sub` y sus direcciones
// (`hubnotify.PorCorreo`), porque quien solo conoce un correo —un invitado de
// Calendar— avisa a la dirección (ver domain.Destinatarios).
func (s *Store) ListNotifications(ctx context.Context, subjects []string, onlyUnread bool, before *domain.Antes, limit int) ([]domain.Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	// Sin ID, uuid.Nil deja fuera todo el instante En: lo de siempre (`created_at < En`).
	var antesEn *time.Time
	antesID := uuid.Nil
	if before != nil {
		antesEn, antesID = &before.En, before.ID
	}
	var out []domain.Notification
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		d, err := conDestinatarios(ctx, tx, subjects)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT `+notificationCols+` FROM notifications
			 WHERE ((tenant = $6 AND subject = ANY($1)) OR subject = ANY($7))
			   AND ($2::boolean = false OR read = false)
			   AND ($3::timestamptz IS NULL OR (created_at, id) < ($3, $5::uuid))
			 ORDER BY created_at DESC, id DESC
			 LIMIT $4`,
			d.enLaOrganizacion, onlyUnread, antesEn, limit, antesID, d.organizacion, d.deLaPersona)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			n, err := scanNotification(rows)
			if err != nil {
				return err
			}
			out = append(out, n)
		}
		return rows.Err()
	})
	return out, err
}

// UnreadCount devuelve el número de notificaciones sin leer de un usuario.
func (s *Store) UnreadCount(ctx context.Context, subjects []string) (int, error) {
	var count int
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		d, err := conDestinatarios(ctx, tx, subjects)
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM notifications
			 WHERE ((tenant = $2 AND subject = ANY($1)) OR subject = ANY($3)) AND read = false`,
			d.enLaOrganizacion, d.organizacion, d.deLaPersona).Scan(&count)
	})
	return count, err
}

// MarkRead marca una notificación como leída (idempotente). Falla con
// ErrNotFound si no existe o no pertenece al usuario.
func (s *Store) MarkRead(ctx context.Context, id uuid.UUID, subjects []string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		d, err := conDestinatarios(ctx, tx, subjects)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`UPDATE notifications SET read = true, read_at = now()
			 WHERE id=$1 AND ((tenant = $3 AND subject = ANY($2)) OR subject = ANY($4)) AND read = false`,
			id, d.enLaOrganizacion, d.organizacion, d.deLaPersona)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			// Distinguir "no existe" de "ya leída": comprobar existencia.
			var exists bool
			if err := tx.QueryRow(ctx,
				`SELECT true FROM notifications
				 WHERE id=$1 AND ((tenant = $3 AND subject = ANY($2)) OR subject = ANY($4))`,
				id, d.enLaOrganizacion, d.organizacion, d.deLaPersona).Scan(&exists); errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			} else if err != nil {
				return err
			}
		}
		return nil
	})
}

// MarkAllRead marca todas las notificaciones del usuario como leídas y devuelve
// cuántas se han actualizado.
func (s *Store) MarkAllRead(ctx context.Context, subjects []string) (int64, error) {
	var affected int64
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		d, err := conDestinatarios(ctx, tx, subjects)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`UPDATE notifications SET read = true, read_at = now()
			 WHERE ((tenant = $2 AND subject = ANY($1)) OR subject = ANY($3)) AND read = false`,
			d.enLaOrganizacion, d.organizacion, d.deLaPersona)
		if err != nil {
			return err
		}
		affected = tag.RowsAffected()
		return nil
	})
	return affected, err
}

// PurgeReadBefore borra notificaciones leídas anteriores al instante dado
// (retención). Devuelve el número de filas eliminadas. No usa tenant del
// contexto: opera sobre TODOS los tenants (tarea de mantenimiento), por lo que
// desactiva RLS dentro de una transacción de servicio.
func (s *Store) PurgeReadBefore(ctx context.Context, before time.Time) (int64, error) {
	var affected int64
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := enableMaintenance(ctx, tx); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`DELETE FROM notifications WHERE read = true AND read_at < $1`, before)
		if err != nil {
			return err
		}
		affected = tag.RowsAffected()
		return nil
	})
	return affected, err
}

// separadorDestinatarios separa los buzones en `app.destinatarios`: no puede ir en
// un `sub` ni en una dirección de correo.
const separadorDestinatarios = "\x1f"

// destinatarios es A QUIÉN se le leen los avisos en una petición, en las dos mitades
// que el SQL necesita por separado:
//
//	(tenant = organizacion AND subject = ANY(enLaOrganizacion)) OR subject = ANY(deLaPersona)
//
// Lo que es de la PERSONA —su `sub` y los buzones que Sytue ID firma en su token— se
// lee en todas sus organizaciones (ADR 0053). Todo lo demás, y en particular su correo
// de entrar, solo en la organización de la sesión.
//
// POR QUÉ VA EN EL SQL. El corte lo hacían dos políticas de fila (la de organización
// y la de destinatario, 0005) y la consulta decía solo `subject = ANY(...)`. Una
// política no se evalúa para un superusuario ni para un rol con BYPASSRLS, y el
// despliegue ha llegado a conectar así: con ese rol, un token DELEGADO —al que el
// handler no da buzones personales a propósito— leía igualmente los avisos de la
// persona en sus otras organizaciones, porque su `sub` iba en la lista y nada cortaba
// por organización. Las políticas siguen y dicen lo mismo; ya no son lo único.
type destinatarios struct {
	organizacion     string
	enLaOrganizacion []string
	deLaPersona      []string
}

// conDestinatarios reparte los buzones de quien llama (ver destinatarios) y fija LOCAL
// `app.destinatarios` con los PERSONALES (domain.ConBuzonesPersonales: su `sub` y sus
// direcciones de TODAS sus organizaciones, que el handler saca de SU token —claims
// `mailbox` y `mailboxes`, que Sytue ID deriva en cada emisión—, nunca de la
// petición), para la política `notifications_destinatario`.
//
// Una dirección que Sytue ID retiró DESPUÉS de emitir el token no cuenta
// (`buzones_olvidados`): el token de quien la tenía sigue vivo unos minutos.
func conDestinatarios(ctx context.Context, q pg.Querier, subjects []string) (destinatarios, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return destinatarios{}, err
	}
	persona := domain.BuzonesPersonalesDe(ctx)
	personales := []string{}
	for _, s := range persona.Buzones {
		if strings.TrimSpace(s) != "" && !strings.Contains(s, separadorDestinatarios) && !slices.Contains(personales, s) {
			personales = append(personales, s)
		}
	}
	if len(personales) > 1 {
		olvidados, err := buzonesOlvidadosDesde(ctx, q, personales[1:], persona.Emitido)
		if err != nil {
			return destinatarios{}, err
		}
		personales = slices.DeleteFunc(personales, func(b string) bool { return slices.Contains(olvidados, b) })
		subjects = slices.DeleteFunc(slices.Clone(subjects), func(b string) bool { return slices.Contains(olvidados, b) })
	}
	if _, err := q.Exec(ctx, "SELECT set_config('app.destinatarios', $1, true)",
		strings.Join(personales, separadorDestinatarios)); err != nil {
		return destinatarios{}, err
	}
	enOrg := make([]string, 0, len(subjects))
	for _, s := range subjects {
		if strings.TrimSpace(s) != "" && !slices.Contains(enOrg, s) {
			enOrg = append(enOrg, s)
		}
	}
	return destinatarios{organizacion: t.Slug, enLaOrganizacion: enOrg, deLaPersona: personales}, nil
}

// buzonesOlvidadosDesde son, de `buzones`, los que Sytue ID retiró después de `emitido`.
func buzonesOlvidadosDesde(ctx context.Context, q pg.Querier, buzones []string, emitido time.Time) ([]string, error) {
	rows, err := q.Query(ctx,
		`SELECT buzon FROM buzones_olvidados WHERE buzon = ANY($1) AND olvidado >= $2`, buzones, emitido)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// enableMaintenance fija LOCAL `app.maintenance='on'` para que la política
// PERMISSIVE de mantenimiento habilite operaciones cross-tenant (purga). El
// ajuste vive solo en la transacción actual.
func enableMaintenance(ctx context.Context, q pg.Querier) error {
	_, err := q.Exec(ctx, "SELECT set_config('app.maintenance', 'on', true)")
	return err
}

// ---------------------------------------------------------------------------
// Preferencias de notificación (por origen y tipo, por usuario)
// ---------------------------------------------------------------------------

// ListNotificationPrefs devuelve las EXCEPCIONES del usuario. La ausencia de reglas
// significa «que llegue todo»: no hay que sembrar una fila por cada origen.
func (s *Store) ListNotificationPrefs(ctx context.Context, subject string) ([]domain.NotificationPref, error) {
	// La organización va en el SQL: la política de fila dice lo mismo, pero no se
	// evalúa con un rol que se la salte (ver destinatarios).
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, err
	}
	var out []domain.NotificationPref
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT source, kind, muted FROM notification_prefs
			 WHERE tenant = $2 AND subject = $1 ORDER BY source, kind`, subject, t.Slug)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p domain.NotificationPref
			var src string
			if err := rows.Scan(&src, &p.Kind, &p.Muted); err != nil {
				return err
			}
			p.Source = domain.Source(src)
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// SetNotificationPref guarda (o quita) una excepción. `muted=false` sobre un origen sin
// regla previa BORRA la fila en vez de guardar un «no silenciado»: el estado por defecto
// ya es ese, y persistirlo llenaría la tabla de filas que no dicen nada. La excepción es
// cuando hace de contra-regla de una del origen entero, que sí hay que guardar.
func (s *Store) SetNotificationPref(ctx context.Context, subject string, p domain.NotificationPref) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if !p.Muted && p.Kind == "" {
			_, err := tx.Exec(ctx,
				`DELETE FROM notification_prefs WHERE tenant = $3 AND subject = $1 AND source = $2 AND kind = ''`,
				subject, string(p.Source), t.Slug)
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO notification_prefs (tenant, subject, source, kind, muted)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (tenant, subject, source, kind)
			 DO UPDATE SET muted = EXCLUDED.muted, updated_at = now()`,
			t.Slug, subject, string(p.Source), p.Kind, p.Muted)
		return err
	})
}

// PrefsDeLaPersona son las reglas de silencio de `subject` en TODAS sus organizaciones,
// una lista por organización (ADR 0053): lo que calla en una calla también lo que le
// llega de otra. Solo para decidir si se entrega un aviso; no sale hacia fuera.
func (s *Store) PrefsDeLaPersona(ctx context.Context, subject string) ([][]domain.NotificationPref, error) {
	porOrg := map[string][]domain.NotificationPref{}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if strings.Contains(subject, separadorDestinatarios) {
			return domain.ErrInvalidInput
		}
		if _, err := tx.Exec(ctx, "SELECT set_config('app.destinatarios', $1, true)", subject); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT tenant, source, kind, muted FROM notification_prefs WHERE subject = $1`, subject)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t, src string
			var p domain.NotificationPref
			if err := rows.Scan(&t, &src, &p.Kind, &p.Muted); err != nil {
				return err
			}
			p.Source = domain.Source(src)
			porOrg[t] = append(porOrg[t], p)
		}
		return rows.Err()
	})
	out := make([][]domain.NotificationPref, 0, len(porOrg))
	for _, ps := range porOrg {
		out = append(out, ps)
	}
	return out, err
}

// AvisosRecientes cuenta los avisos de un MISMO tipo que una organización ha dejado a
// `subject` en la última hora: el tope por emisor (ADR 0053). Por tipo, para que quien
// agote uno (cien menciones) no calle los demás de esa organización (una invitación).
// Con origen y tipo vacíos, todos: el tope total.
func AvisosRecientes(ctx context.Context, tx pgx.Tx, tenant, subject string, source domain.Source, tipo string) (int, error) {
	var n int
	err := tx.QueryRow(ctx,
		`SELECT count(*) FROM notifications
		 WHERE tenant = $1 AND subject = $2
		   AND ($3 = '' OR (source = $3 AND type = $4))
		   AND created_at > now() - interval '1 hour'`,
		tenant, subject, string(source), tipo).Scan(&n)
	return n, err
}

// OlvidarBuzon borra, en TODAS las organizaciones, los avisos a un buzón que Sytue ID
// ha retirado: si la dirección se reasigna, quien la reciba no hereda los avisos de
// quien la tuvo (ADR 0053). Tarea de mantenimiento, como la purga.
func (s *Store) OlvidarBuzon(ctx context.Context, subject string) (int64, error) {
	var n int64
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := enableMaintenance(ctx, tx); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM notifications WHERE subject = $1`, subject)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()
		_, err = tx.Exec(ctx, `
			INSERT INTO buzones_olvidados (buzon) VALUES ($1)
			ON CONFLICT (buzon) DO UPDATE SET olvidado = now()`, subject)
		return err
	})
	return n, err
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Integraciones (Ola 15): persistencia de webhooks entrantes/salientes y bots.
// Gestión por tenant (InTenantTx, RLS por tenant); los lookups por token del
// endpoint público y del auth de bot son CROSS-TENANT vía InMaintenanceTx (política
// PERMISSIVE de mantenimiento, sin BYPASSRLS). Ver mig. 0014.

// ---------------------------------------------------------------------------
// Webhooks entrantes
// ---------------------------------------------------------------------------

const incomingCols = `id, channel_id, name, icon, created_by, created_at`

func scanIncoming(row pgx.Row, tenant string) (domain.IncomingWebhook, error) {
	var w domain.IncomingWebhook
	if err := row.Scan(&w.ID, &w.ChannelID, &w.Name, &w.Icon, &w.CreatedBy, &w.CreatedAt); err != nil {
		return domain.IncomingWebhook{}, err
	}
	w.Tenant = tenant
	return w, nil
}

// CreateIncomingWebhook crea un webhook entrante ligado a un canal (solo un admin
// del canal). Guarda el HASH del token, nunca el token. El canal debe existir/vivo.
func (s *Store) CreateIncomingWebhook(ctx context.Context, channelID uuid.UUID, actor, name, icon, tokenHash string) (domain.IncomingWebhook, error) {
	t := tenancy.MustFrom(ctx)
	var w domain.IncomingWebhook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAdmin(ctx, tx, t.Slug, ch, actor); err != nil {
			return err
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO incoming_webhooks (tenant, channel_id, token_hash, name, icon, created_by)
			 VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+incomingCols,
			t.Slug, channelID, tokenHash, name, icon, actor)
		w, err = scanIncoming(row, t.Slug)
		return err
	})
	return w, err
}

// ListIncomingWebhooks lista los webhooks entrantes de un canal (admin del canal).
func (s *Store) ListIncomingWebhooks(ctx context.Context, channelID uuid.UUID, actor string) ([]domain.IncomingWebhook, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.IncomingWebhook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAdmin(ctx, tx, t.Slug, ch, actor); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT `+incomingCols+` FROM incoming_webhooks
			 WHERE tenant=$1 AND channel_id=$2 ORDER BY created_at DESC`, t.Slug, channelID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			w, err := scanIncoming(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, w)
		}
		return rows.Err()
	})
	return out, err
}

// incomingAdmin lee un webhook entrante y exige que el actor sea admin de su canal.
func incomingAdmin(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID, actor string) (uuid.UUID, error) { //nolint:unparam // channelID expuesto para autorizacion; los callers actuales solo comprueban el error
	var channelID uuid.UUID
	err := tx.QueryRow(ctx,
		`SELECT channel_id FROM incoming_webhooks WHERE tenant=$1 AND id=$2`, tenant, id).Scan(&channelID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, domain.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	ch, err := getChannelTx(ctx, tx, tenant, channelID)
	if err != nil {
		return uuid.Nil, err
	}
	return channelID, requireChannelAdmin(ctx, tx, tenant, ch, actor)
}

// DeleteIncomingWebhook borra un webhook entrante (admin de su canal).
func (s *Store) DeleteIncomingWebhook(ctx context.Context, id uuid.UUID, actor string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := incomingAdmin(ctx, tx, t.Slug, id, actor); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM incoming_webhooks WHERE tenant=$1 AND id=$2`, t.Slug, id)
		return err
	})
}

// RotateIncomingWebhook cambia el token de un webhook entrante (admin de su canal):
// persiste el nuevo hash y devuelve el webhook. El claro lo genera el servicio.
func (s *Store) RotateIncomingWebhook(ctx context.Context, id uuid.UUID, actor, newHash string) (domain.IncomingWebhook, error) {
	t := tenancy.MustFrom(ctx)
	var w domain.IncomingWebhook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := incomingAdmin(ctx, tx, t.Slug, id, actor); err != nil {
			return err
		}
		row := tx.QueryRow(ctx,
			`UPDATE incoming_webhooks SET token_hash=$3 WHERE tenant=$1 AND id=$2 RETURNING `+incomingCols,
			t.Slug, id, newHash)
		var err error
		w, err = scanIncoming(row, t.Slug)
		return err
	})
	return w, err
}

// LookupIncomingWebhookByTokenHash busca un webhook entrante por el hash de su
// token, CROSS-TENANT (endpoint público sin JWT: el token es la autorización). Usa
// InMaintenanceTx (política PERMISSIVE) sin BYPASSRLS. Devuelve el tenant en el
// campo Tenant para que el llamante fije el contexto antes de publicar.
func (s *Store) LookupIncomingWebhookByTokenHash(ctx context.Context, tokenHash string) (domain.IncomingWebhook, error) {
	var w domain.IncomingWebhook
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		var tenant string
		row := tx.QueryRow(ctx,
			`SELECT tenant, `+incomingCols+` FROM incoming_webhooks WHERE token_hash=$1`, tokenHash)
		if err := row.Scan(&tenant, &w.ID, &w.ChannelID, &w.Name, &w.Icon, &w.CreatedBy, &w.CreatedAt); err != nil {
			return err
		}
		w.Tenant = tenant
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.IncomingWebhook{}, domain.ErrNotFound
	}
	return w, err
}

// CreateHookMessage publica un mensaje en un canal como autor sintético (hook:<id>),
// SIN exigir membresía (el webhook es la autorización) pero validando que el canal
// exista y no esté archivado. Requiere app.tenant fijado en el contexto (el servicio
// lo construye del tenant del webhook). No toca read_state ni auto-follow (las
// integraciones no leen ni siguen hilos). Devuelve el mensaje insertado.
func (s *Store) CreateHookMessage(ctx context.Context, channelID uuid.UUID, authorSub, authorName, body string) (domain.Message, error) {
	t := tenancy.MustFrom(ctx)
	var m domain.Message
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if ch.Archived {
			return domain.ErrConflict
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO messages (tenant, channel_id, author_sub, author_name, body, mentions_all, type)
			 VALUES ($1, $2, $3, $4, $5, $6, 'user') RETURNING `+messageCols,
			t.Slug, channelID, authorSub, authorName, body, domain.HasChannelMention(body))
		m, err = scanMessage(row, t.Slug)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`UPDATE channels SET last_message_at=now() WHERE tenant=$1 AND id=$2`, t.Slug, channelID)
		return err
	})
	return m, err
}

// ---------------------------------------------------------------------------
// Webhooks salientes
// ---------------------------------------------------------------------------

const outgoingCols = `id, channel_id, trigger_words, callback_url, token, dialect, signing_secret, created_by, created_at`

func (s *Store) scanOutgoing(row pgx.Row, tenant string) (domain.OutgoingWebhook, error) {
	var w domain.OutgoingWebhook
	if err := row.Scan(&w.ID, &w.ChannelID, &w.TriggerWords, &w.CallbackURL, &w.Token, &w.Dialect, &w.SigningSecret, &w.CreatedBy, &w.CreatedAt); err != nil {
		return domain.OutgoingWebhook{}, err
	}
	// El secreto de firma se guarda cifrado; aquí vuelve a texto para poder firmar.
	var err error
	if w.SigningSecret, err = s.openSecret(w.SigningSecret); err != nil {
		return domain.OutgoingWebhook{}, err
	}
	w.Tenant = tenant
	return w, nil
}

// CreateOutgoingWebhook crea un webhook saliente. Si está ligado a un canal, exige
// admin de ese canal; si no (tenant-wide), admin del workspace. El servicio ya
// validó la URL, las palabras y generó el token de verificación.
func (s *Store) CreateOutgoingWebhook(ctx context.Context, channelID *uuid.UUID, actor, callbackURL, token, dialect, signingSecret string, triggerWords []string) (domain.OutgoingWebhook, error) {
	t := tenancy.MustFrom(ctx)
	var w domain.OutgoingWebhook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := authorizeOutgoingScope(ctx, tx, t.Slug, channelID, actor); err != nil {
			return err
		}
		sealed, err := s.sealSecret(signingSecret)
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO outgoing_webhooks (tenant, channel_id, trigger_words, callback_url, token, dialect, signing_secret, created_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+outgoingCols,
			t.Slug, channelID, triggerWords, callbackURL, token, dialect, sealed, actor)
		w, err = s.scanOutgoing(row, t.Slug)
		return err
	})
	return w, err
}

// authorizeOutgoingScope exige admin del canal (si el saliente está ligado) o del
// workspace (si es tenant-wide).
func authorizeOutgoingScope(ctx context.Context, tx pgx.Tx, tenant string, channelID *uuid.UUID, actor string) error {
	if channelID != nil {
		ch, err := getChannelTx(ctx, tx, tenant, *channelID)
		if err != nil {
			return err
		}
		return requireChannelAdmin(ctx, tx, tenant, ch, actor)
	}
	if !esAdminDeLaOrganizacion(ctx, actor) {
		return domain.ErrForbidden
	}
	return nil
}

// ListOutgoingWebhooks lista los salientes del tenant que el actor puede gestionar:
// admin del workspace ve todos; si no, solo los de canales que administra.
//
// ponytail: filtro de gestión en Go sobre el conjunto (pocos salientes por tenant,
// los crea un admin). Si crecieran, empujar el filtro de canales-administrados a SQL.
func (s *Store) ListOutgoingWebhooks(ctx context.Context, actor string) ([]domain.OutgoingWebhook, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.OutgoingWebhook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		wsAdmin := esAdminDeLaOrganizacion(ctx, actor)
		rows, err := tx.Query(ctx,
			`SELECT `+outgoingCols+` FROM outgoing_webhooks WHERE tenant=$1 ORDER BY created_at DESC`, t.Slug)
		if err != nil {
			return err
		}
		defer rows.Close()
		var all []domain.OutgoingWebhook
		for rows.Next() {
			w, err := s.scanOutgoing(rows, t.Slug)
			if err != nil {
				return err
			}
			all = append(all, w)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, w := range all {
			if wsAdmin {
				out = append(out, w)
				continue
			}
			if w.ChannelID == nil {
				// Los de toda la organización los ve quien los gestiona: llevan la URL
				// de destino y el token con el que el receptor reconoce nuestras llamadas.
				continue
			}
			ch, err := getChannelTx(ctx, tx, t.Slug, *w.ChannelID)
			if err != nil {
				continue
			}
			if requireChannelAdmin(ctx, tx, t.Slug, ch, actor) == nil {
				out = append(out, w)
			}
		}
		return nil
	})
	return out, err
}

// DeleteOutgoingWebhook borra un saliente (admin de su canal, o del workspace si es
// tenant-wide).
func (s *Store) DeleteOutgoingWebhook(ctx context.Context, id uuid.UUID, actor string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var channelID *uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT channel_id FROM outgoing_webhooks WHERE tenant=$1 AND id=$2`, t.Slug, id).Scan(&channelID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := authorizeOutgoingScope(ctx, tx, t.Slug, channelID, actor); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM outgoing_webhooks WHERE tenant=$1 AND id=$2`, t.Slug, id)
		return err
	})
}

// OutgoingWebhooksForChannel devuelve los salientes del tenant candidatos para un
// canal (ligados a él o tenant-wide). El consumidor filtra por palabra en Go
// (domain.MatchesOutgoing). Requiere app.tenant fijado. Coste acotado: pocos
// salientes por tenant.
func (s *Store) OutgoingWebhooksForChannel(ctx context.Context, channelID uuid.UUID) ([]domain.OutgoingWebhook, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.OutgoingWebhook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Un saliente SIN canal es de toda la organización, y «toda la organización»
		// son sus canales PÚBLICOS: nunca un privado ni un mensaje directo. Quien
		// quiera uno en un canal privado lo crea atado a ese canal, con la verja de
		// ese canal.
		rows, err := tx.Query(ctx,
			`SELECT `+outgoingCols+` FROM outgoing_webhooks
			 WHERE tenant=$1 AND (channel_id=$2 OR (channel_id IS NULL AND EXISTS (
			         SELECT 1 FROM channels c
			         WHERE c.tenant=$1 AND c.id=$2 AND c.kind='public' AND c.deleted_at IS NULL)))`,
			t.Slug, channelID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			w, err := s.scanOutgoing(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, w)
		}
		return rows.Err()
	})
	return out, err
}

// ---------------------------------------------------------------------------
// Bots
// ---------------------------------------------------------------------------

const botCols = `id, name, owner_sub, created_at`

func scanBot(row pgx.Row, tenant string) (domain.Bot, error) {
	var b domain.Bot
	if err := row.Scan(&b.ID, &b.Name, &b.OwnerSub, &b.CreatedAt); err != nil {
		return domain.Bot{}, err
	}
	b.Tenant = tenant
	return b, nil
}

// CreateBot crea un bot del tenant (solo admin del workspace). Guarda el HASH del
// token, nunca el token. El servicio ya validó el nombre y generó el token.
func (s *Store) CreateBot(ctx context.Context, name, tokenHash, ownerSub string) (domain.Bot, error) {
	t := tenancy.MustFrom(ctx)
	var b domain.Bot
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if !esAdminDeLaOrganizacion(ctx, ownerSub) {
			return domain.ErrForbidden
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO bots (tenant, name, token_hash, owner_sub)
			 VALUES ($1, $2, $3, $4) RETURNING `+botCols,
			t.Slug, name, tokenHash, ownerSub)
		var err error
		b, err = scanBot(row, t.Slug)
		return err
	})
	return b, err
}

// ListBots lista los bots del tenant (admin del workspace).
func (s *Store) ListBots(ctx context.Context, actor string) ([]domain.Bot, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Bot
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if !esAdminDeLaOrganizacion(ctx, actor) {
			return domain.ErrForbidden
		}
		rows, err := tx.Query(ctx,
			`SELECT `+botCols+` FROM bots WHERE tenant=$1 ORDER BY created_at DESC`, t.Slug)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			b, err := scanBot(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, b)
		}
		return rows.Err()
	})
	return out, err
}

// DeleteBot borra un bot (su dueño o un admin del workspace). Sus filas de
// pertenencia en channel_members caen al no tener FK (subject sintético); se
// limpian aquí por el subject bot:<id>.
func (s *Store) DeleteBot(ctx context.Context, id uuid.UUID, actor string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var ownerSub string
		err := tx.QueryRow(ctx,
			`SELECT owner_sub FROM bots WHERE tenant=$1 AND id=$2`, t.Slug, id).Scan(&ownerSub)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if actor != ownerSub {
			if !esAdminDeLaOrganizacion(ctx, actor) {
				return domain.ErrForbidden
			}
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM channel_members WHERE tenant=$1 AND subject=$2`,
			t.Slug, domain.BotAuthorSub(id)); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM bots WHERE tenant=$1 AND id=$2`, t.Slug, id)
		return err
	})
}

// LookupBotByTokenHash busca un bot por el hash de su token Bearer, CROSS-TENANT
// (middleware de auth de bot, sin JWT). InMaintenanceTx sin BYPASSRLS. Devuelve el
// tenant en el campo Tenant.
func (s *Store) LookupBotByTokenHash(ctx context.Context, tokenHash string) (domain.Bot, error) {
	var b domain.Bot
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		var tenant string
		row := tx.QueryRow(ctx,
			`SELECT tenant, `+botCols+` FROM bots WHERE token_hash=$1`, tokenHash)
		if err := row.Scan(&tenant, &b.ID, &b.Name, &b.OwnerSub, &b.CreatedAt); err != nil {
			return err
		}
		b.Tenant = tenant
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Bot{}, domain.ErrNotFound
	}
	return b, err
}

// AddBotToChannel añade un bot como miembro de un canal (admin del canal), para que
// pueda publicar en él (reusa channel_members con subject bot:<id>). El bot debe
// existir en el tenant. Idempotente.
func (s *Store) AddBotToChannel(ctx context.Context, botID, channelID uuid.UUID, actor string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var botName string
		err := tx.QueryRow(ctx,
			`SELECT name FROM bots WHERE tenant=$1 AND id=$2`, t.Slug, botID).Scan(&botName)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAdmin(ctx, tx, t.Slug, ch, actor); err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO channel_members (tenant, channel_id, subject, name, role)
			 VALUES ($1, $2, $3, $4, 'member')
			 ON CONFLICT (tenant, channel_id, subject) DO UPDATE SET name=EXCLUDED.name`,
			t.Slug, channelID, domain.BotAuthorSub(botID), botName)
		return err
	})
}

// RemoveBotFromChannel quita a un bot de un canal (admin del canal).
func (s *Store) RemoveBotFromChannel(ctx context.Context, botID, channelID uuid.UUID, actor string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAdmin(ctx, tx, t.Slug, ch, actor); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`DELETE FROM channel_members WHERE tenant=$1 AND channel_id=$2 AND subject=$3`,
			t.Slug, channelID, domain.BotAuthorSub(botID))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

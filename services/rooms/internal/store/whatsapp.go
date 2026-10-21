// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// WhatsApp Business (mig. 0034). Gestión por admin del workspace; el aviso público
// busca la cuenta por token CROSS-TENANT y el resto va con el tenant de la cuenta.

const whatsappCols = `id, team_id, phone_number_id, display_phone, verified_name, access_token, app_secret, created_by, created_at, ready, instance_id`

// scanWhatsApp lee whatsappCols; `antes` son destinos de columnas que la consulta
// pone DELANTE (el tenant en la búsqueda cross-tenant).
func (s *Store) scanWhatsApp(row pgx.Row, tenant string, antes ...any) (domain.WhatsAppAccount, error) {
	var a domain.WhatsAppAccount
	if err := row.Scan(append(antes, &a.ID, &a.TeamID, &a.PhoneNumberID, &a.DisplayPhone, &a.VerifiedName,
		&a.AccessToken, &a.AppSecret, &a.CreatedBy, &a.CreatedAt, &a.Ready, &a.InstanceID)...); err != nil {
		return domain.WhatsAppAccount{}, err
	}
	var err error
	if a.AccessToken, err = s.openSecret(a.AccessToken); err != nil {
		return domain.WhatsAppAccount{}, err
	}
	if a.AppSecret, err = s.openSecret(a.AppSecret); err != nil {
		return domain.WhatsAppAccount{}, err
	}
	a.Tenant = tenant
	return a, nil
}

// requireWorkspaceAdmin es la verja de gestión de WhatsApp: conectar un número
// decide qué equipo lee las conversaciones con clientes.
func requireWorkspaceAdmin(ctx context.Context, actor string) error {
	if !esAdminDeLaOrganizacion(ctx, actor) {
		return domain.ErrForbidden
	}
	return nil
}

// CreateWhatsAppAccount conecta un número (admin del workspace que además es
// miembro del equipo: los canales de clientes se crean en su nombre). ErrConflict
// si el número ya está conectado.
func (s *Store) CreateWhatsAppAccount(ctx context.Context, a domain.WhatsAppAccount, tokenHash string) (domain.WhatsAppAccount, error) {
	a.Ready = true
	return s.createWhatsAppAccount(ctx, a, tokenHash)
}

// RequireWhatsAppTeam checks local access before any credentials or provider calls.
func (s *Store) RequireWhatsAppTeam(ctx context.Context, actor string, teamID uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireWorkspaceAdmin(ctx, actor); err != nil {
			return err
		}
		return requireTeamMember(ctx, tx, t.Slug, teamID, actor)
	})
}

// PrepareWhatsAppAccount allows Meta's challenge, but cannot receive conversations.
// Only an expired setup can be retried. Its new ID fences late completions from
// the previous attempt; an active number is never replaced.
func (s *Store) PrepareWhatsAppAccount(ctx context.Context, a domain.WhatsAppAccount, tokenHash string) (domain.WhatsAppAccount, error) {
	if a.InstanceID == "" {
		return domain.WhatsAppAccount{}, domain.ErrInvalidInput
	}
	a.Ready = false
	return s.createWhatsAppAccount(ctx, a, tokenHash)
}

func (s *Store) createWhatsAppAccount(ctx context.Context, a domain.WhatsAppAccount, tokenHash string) (domain.WhatsAppAccount, error) {
	t := tenancy.MustFrom(ctx)
	var out domain.WhatsAppAccount
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireWorkspaceAdmin(ctx, a.CreatedBy); err != nil {
			return err
		}
		if err := requireTeamMember(ctx, tx, t.Slug, a.TeamID, a.CreatedBy); err != nil {
			return err
		}
		token, err := s.sealSecret(a.AccessToken)
		if err != nil {
			return err
		}
		secret, err := s.sealSecret(a.AppSecret)
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO whatsapp_accounts (tenant, team_id, phone_number_id, display_phone, verified_name,
			                                access_token, app_secret, token_hash, created_by, ready, instance_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			 ON CONFLICT (tenant, phone_number_id) DO UPDATE SET
			   id=EXCLUDED.id, created_at=now(), team_id=EXCLUDED.team_id, display_phone=EXCLUDED.display_phone,
			   verified_name=EXCLUDED.verified_name, access_token=EXCLUDED.access_token,
			   app_secret=EXCLUDED.app_secret, token_hash=EXCLUDED.token_hash, created_by=EXCLUDED.created_by
			 WHERE NOT whatsapp_accounts.ready AND NOT EXCLUDED.ready
			   AND whatsapp_accounts.instance_id=EXCLUDED.instance_id AND EXCLUDED.instance_id<>''
			   AND whatsapp_accounts.created_at < now() - interval '2 minutes'
			 RETURNING `+whatsappCols,
			t.Slug, a.TeamID, a.PhoneNumberID, a.DisplayPhone, a.VerifiedName, token, secret, tokenHash, a.CreatedBy, a.Ready, a.InstanceID)
		out, err = s.scanWhatsApp(row, t.Slug)
		if uniqueViolation(err) || errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrConflict
		}
		return err
	})
	return out, err
}

// FinishWhatsAppAccount publishes the number only after Meta accepts its callback.
func (s *Store) FinishWhatsAppAccount(ctx context.Context, id uuid.UUID, actor string) (domain.WhatsAppAccount, error) {
	t := tenancy.MustFrom(ctx)
	var out domain.WhatsAppAccount
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireWorkspaceAdmin(ctx, actor); err != nil {
			return err
		}
		var err error
		out, err = s.scanWhatsApp(tx.QueryRow(ctx,
			`UPDATE whatsapp_accounts SET ready=true WHERE tenant=$1 AND id=$2 AND NOT ready RETURNING `+whatsappCols,
			t.Slug, id), t.Slug)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	})
	return out, err
}

// AbortWhatsAppAccount erases only an unfinished setup, preserving any active one.
func (s *Store) AbortWhatsAppAccount(ctx context.Context, id uuid.UUID, actor string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireWorkspaceAdmin(ctx, actor); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM whatsapp_accounts WHERE tenant=$1 AND id=$2 AND NOT ready`, t.Slug, id)
		return err
	})
}

// ListWhatsAppAccounts lista los números conectados (admin del workspace).
func (s *Store) ListWhatsAppAccounts(ctx context.Context, actor string) ([]domain.WhatsAppAccount, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.WhatsAppAccount
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireWorkspaceAdmin(ctx, actor); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT `+whatsappCols+` FROM whatsapp_accounts WHERE tenant=$1 AND ready ORDER BY created_at`, t.Slug)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := s.scanWhatsApp(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

// DeleteWhatsAppAccount desconecta un número (admin del workspace). Los canales de
// clientes se quedan: son el historial de la atención.
func (s *Store) DeleteWhatsAppAccount(ctx context.Context, id uuid.UUID, actor string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireWorkspaceAdmin(ctx, actor); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM whatsapp_accounts WHERE tenant=$1 AND id=$2`, t.Slug, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// LookupWhatsAppAccountByTokenHash busca la cuenta del aviso público, sin tenant.
func (s *Store) LookupWhatsAppAccountByTokenHash(ctx context.Context, tokenHash string) (domain.WhatsAppAccount, error) {
	var a domain.WhatsAppAccount
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		var tenant string
		row := tx.QueryRow(ctx, `SELECT tenant, `+whatsappCols+` FROM whatsapp_accounts WHERE token_hash=$1`, tokenHash)
		var err error
		if a, err = s.scanWhatsApp(row, "", &tenant); err != nil {
			return err
		}
		a.Tenant = tenant
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WhatsAppAccount{}, domain.ErrNotFound
	}
	return a, err
}

// WhatsAppSeen dice si un id de mensaje de Meta ya se publicó. Meta entrega al
// menos una vez: lo repetido no se vuelve a publicar.
func (s *Store) WhatsAppSeen(ctx context.Context, messageID string) (bool, error) {
	t := tenancy.MustFrom(ctx)
	var visto bool
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM whatsapp_seen WHERE tenant=$1 AND message_id=$2)`, t.Slug, messageID).Scan(&visto)
	})
	return visto, err
}

// MarkWhatsAppSeen anota un id de mensaje DESPUÉS de publicarlo: si algo falla antes,
// el reintento de Meta lo vuelve a intentar en vez de darlo por visto y perderlo. De
// paso purga lo que pasó de 7 días, trabajo acotado por el índice de fecha.
func (s *Store) MarkWhatsAppSeen(ctx context.Context, messageID string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`DELETE FROM whatsapp_seen WHERE tenant=$1 AND seen_at < now() - interval '7 days'`, t.Slug); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO whatsapp_seen (tenant, message_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, t.Slug, messageID)
		return err
	})
}

// EnsureWhatsAppChat devuelve el canal del cliente, creándolo si es el primer
// mensaje (o si el suyo se archivó o borró). El canal es PÚBLICO en el equipo de la
// cuenta y nace con todo el equipo dentro: lo que escribe un cliente tiene que
// verse sin ir a buscarlo.
func (s *Store) EnsureWhatsAppChat(ctx context.Context, a domain.WhatsAppAccount, waID, name string, at time.Time) (domain.WhatsAppChat, error) {
	t := tenancy.MustFrom(ctx)
	c := domain.WhatsAppChat{AccountID: a.ID, WaID: waID, ContactName: name, LastInboundAt: at}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var vivo bool
		err := tx.QueryRow(ctx,
			`SELECT w.id, w.channel_id, (ch.id IS NOT NULL AND NOT ch.archived AND ch.deleted_at IS NULL)
			   FROM whatsapp_chats w
			   LEFT JOIN channels ch ON ch.tenant = w.tenant AND ch.id = w.channel_id
			  WHERE w.tenant=$1 AND w.account_id=$2 AND w.wa_id=$3`,
			t.Slug, a.ID, waID).Scan(&c.ID, &c.ChannelID, &vivo)
		switch {
		case err == nil && vivo:
			_, err = tx.Exec(ctx,
				`UPDATE whatsapp_chats SET last_inbound_at=$2,
				        contact_name = CASE WHEN $3 <> '' THEN $3 ELSE contact_name END
				  WHERE tenant=$4 AND id=$1`, c.ID, at, name, t.Slug)
			return err
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return err
		}

		slug := domain.WhatsAppChannelSlug(a.ID, waID)
		if err == nil {
			// Había canal, pero archivado o borrado: uno nuevo, con otro slug para no
			// chocar con el archivado.
			slug += "-" + at.UTC().Format("20060102150405")
		}
		titulo := "WhatsApp · " + strings.TrimSpace(name)
		if strings.TrimSpace(name) == "" {
			titulo = "WhatsApp · +" + waID
		}
		if len([]rune(titulo)) > 100 {
			titulo = string([]rune(titulo)[:100])
		}
		if err := tx.QueryRow(ctx,
			`INSERT INTO channels (tenant, team_id, slug, name, topic, kind, created_by)
			 VALUES ($1, $2, $3, $4, $5, 'public', $6) RETURNING id`,
			t.Slug, a.TeamID, slug, titulo, "+"+waID+" · "+a.DisplayPhone, a.CreatedBy).Scan(&c.ChannelID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO channel_members (tenant, channel_id, subject, role)
			 SELECT tenant, $2, subject, 'member' FROM team_members WHERE tenant=$1 AND team_id=$3
			 ON CONFLICT (tenant, channel_id, subject) DO NOTHING`, t.Slug, c.ChannelID, a.TeamID); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`INSERT INTO whatsapp_chats (tenant, account_id, wa_id, channel_id, contact_name, last_inbound_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (account_id, wa_id) DO UPDATE SET channel_id = EXCLUDED.channel_id,
			        contact_name = EXCLUDED.contact_name, last_inbound_at = EXCLUDED.last_inbound_at
			 RETURNING id`, t.Slug, a.ID, waID, c.ChannelID, name, at).Scan(&c.ID)
	})
	return c, err
}

// WhatsAppChatForChannel devuelve el cliente y la cuenta de un canal de WhatsApp, o
// ErrNotFound si el canal no es de ningún cliente (el caso de casi todos).
func (s *Store) WhatsAppChatForChannel(ctx context.Context, channelID uuid.UUID) (domain.WhatsAppChat, domain.WhatsAppAccount, error) {
	t := tenancy.MustFrom(ctx)
	var c domain.WhatsAppChat
	var a domain.WhatsAppAccount
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var tenant string
		row := tx.QueryRow(ctx,
			`SELECT w.id, w.wa_id, w.contact_name, w.last_inbound_at,
			        a.tenant, a.id, a.team_id, a.phone_number_id, a.display_phone, a.verified_name,
			        a.access_token, a.app_secret, a.created_by, a.created_at
			   FROM whatsapp_chats w JOIN whatsapp_accounts a ON a.id = w.account_id AND a.tenant = w.tenant
			  WHERE w.tenant=$1 AND w.channel_id=$2 AND a.ready`, t.Slug, channelID)
		if err := row.Scan(&c.ID, &c.WaID, &c.ContactName, &c.LastInboundAt,
			&tenant, &a.ID, &a.TeamID, &a.PhoneNumberID, &a.DisplayPhone, &a.VerifiedName,
			&a.AccessToken, &a.AppSecret, &a.CreatedBy, &a.CreatedAt); err != nil {
			return err
		}
		var err error
		if a.AccessToken, err = s.openSecret(a.AccessToken); err != nil {
			return err
		}
		a.AppSecret = "" // no hace falta para enviar
		a.Tenant = tenant
		c.AccountID, c.ChannelID = a.ID, channelID
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return c, a, domain.ErrNotFound
	}
	return c, a, err
}

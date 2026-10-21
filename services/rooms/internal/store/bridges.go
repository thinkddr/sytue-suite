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

// Puentes de canal (mig. 0033). Gestión por admin del canal, como los webhooks
// entrantes; el consumidor del outbox los lee por canal con el tenant del evento.

const bridgeCols = `id, channel_id, platform, webhook_url, target, last_error, last_error_at, created_by, created_at, instance_id, remote_id`

func (s *Store) scanBridge(row pgx.Row, tenant string) (domain.Bridge, error) {
	var b domain.Bridge
	if err := row.Scan(&b.ID, &b.ChannelID, &b.Platform, &b.WebhookURL, &b.Target, &b.LastError, &b.LastErrorAt, &b.CreatedBy, &b.CreatedAt, &b.InstanceID, &b.RemoteID); err != nil {
		return domain.Bridge{}, err
	}
	var err error
	if b.WebhookURL, err = s.openSecret(b.WebhookURL); err != nil {
		return domain.Bridge{}, err
	}
	b.Tenant = tenant
	return b, nil
}

// ChannelForAdmin devuelve el canal si el actor lo administra. El servicio lo usa
// para probar el puente ANTES de guardarlo sin dejar que cualquier miembro haga
// que el servidor publique en un webhook ajeno.
func (s *Store) ChannelForAdmin(ctx context.Context, channelID uuid.UUID, actor string) (domain.Channel, error) {
	t := tenancy.MustFrom(ctx)
	var ch domain.Channel
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		if ch, err = getChannelTx(ctx, tx, t.Slug, channelID); err != nil {
			return err
		}
		return requireChannelAdmin(ctx, tx, t.Slug, ch, actor)
	})
	return ch, err
}

// CreateBridge guarda un puente (admin del canal) con la URL cifrada. Rechaza con
// ErrConflict el que supere domain.MaxBridgesPerChannel.
func (s *Store) CreateBridge(ctx context.Context, channelID uuid.UUID, actor, platform, webhookURL string) (domain.Bridge, error) {
	return s.createBridge(ctx, channelID, actor, domain.Bridge{Platform: platform, WebhookURL: webhookURL, Target: domain.MaskBridgeURL(webhookURL)})
}

// CreateOAuthBridge stores only account/destination identifiers, never provider secrets.
func (s *Store) CreateOAuthBridge(ctx context.Context, channelID uuid.UUID, actor, platform, instanceID, remoteID, target string) (domain.Bridge, error) {
	return s.createBridge(ctx, channelID, actor, domain.Bridge{Platform: platform, InstanceID: instanceID, RemoteID: remoteID, Target: target})
}

func (s *Store) createBridge(ctx context.Context, channelID uuid.UUID, actor string, input domain.Bridge) (domain.Bridge, error) {
	t := tenancy.MustFrom(ctx)
	var b domain.Bridge
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAdmin(ctx, tx, t.Slug, ch, actor); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM channel_bridges WHERE tenant=$1 AND channel_id=$2`, t.Slug, channelID).Scan(&n); err != nil {
			return err
		}
		if n >= domain.MaxBridgesPerChannel {
			return domain.ErrConflict
		}
		sealed, err := s.sealSecret(input.WebhookURL)
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO channel_bridges (tenant, channel_id, platform, webhook_url, target, created_by, instance_id, remote_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+bridgeCols,
			t.Slug, channelID, input.Platform, sealed, input.Target, actor, input.InstanceID, input.RemoteID)
		b, err = s.scanBridge(row, t.Slug)
		return err
	})
	return b, err
}

// ListBridges lista los puentes de un canal (admin del canal).
func (s *Store) ListBridges(ctx context.Context, channelID uuid.UUID, actor string) ([]domain.Bridge, error) {
	if _, err := s.ChannelForAdmin(ctx, channelID, actor); err != nil {
		return nil, err
	}
	return s.BridgesForChannel(ctx, channelID)
}

// DeleteBridge borra un puente (admin de su canal).
func (s *Store) DeleteBridge(ctx context.Context, id uuid.UUID, actor string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var channelID uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT channel_id FROM channel_bridges WHERE tenant=$1 AND id=$2`, t.Slug, id).Scan(&channelID)
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
		_, err = tx.Exec(ctx, `DELETE FROM channel_bridges WHERE tenant=$1 AND id=$2`, t.Slug, id)
		return err
	})
}

// BridgesForChannel devuelve los puentes de un canal, con la URL en claro, para
// entregar. Requiere app.tenant fijado; sin comprobación de actor (lo llama el
// consumidor del outbox).
func (s *Store) BridgesForChannel(ctx context.Context, channelID uuid.UUID) ([]domain.Bridge, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Bridge
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+bridgeCols+` FROM channel_bridges
			 WHERE tenant=$1 AND channel_id=$2 ORDER BY created_at`, t.Slug, channelID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			b, err := s.scanBridge(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, b)
		}
		return rows.Err()
	})
	return out, err
}

// SetBridgeError anota el resultado de la última entrega. Solo escribe cuando el
// estado CAMBIA: un canal con mucho tráfico no convierte cada mensaje en un UPDATE.
func (s *Store) SetBridgeError(ctx context.Context, id uuid.UUID, lastError string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE channel_bridges
			    SET last_error = $3,
			        last_error_at = CASE WHEN $3 = '' THEN last_error_at ELSE now() END
			  WHERE tenant=$1 AND id=$2 AND last_error <> $3`, t.Slug, id, lastError)
		return err
	})
}

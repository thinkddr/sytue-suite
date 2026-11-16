// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	roomsearch "git.sytue.eu/sytue/one/apps/nucleo/rooms/search"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// searchAccess is shared by plaintext compatibility search and opaque metadata
// scanning. Query syntax never decides who can read a conversation.
func searchAccess(subj string, external bool) string {
	member := "EXISTS (SELECT 1 FROM channel_members m WHERE m.tenant=c.tenant AND m.channel_id=c.id AND m.subject=" + subj
	if external {
		member += " AND m.externo"
	}
	member += " AND (m.externo OR c.kind='direct' OR EXISTS (SELECT 1 FROM team_members team_access WHERE team_access.tenant=c.tenant AND team_access.team_id=c.team_id AND team_access.subject=" + subj + ")))"
	if !external {
		member += " OR (c.kind='public' AND EXISTS (SELECT 1 FROM team_members tm WHERE tm.tenant=c.tenant AND tm.team_id=c.team_id AND tm.subject=" + subj + "))"
	}
	return " AND (" + member + ") AND NOT EXISTS (SELECT 1 FROM matrix_tenant_access denied WHERE denied.tenant=c.tenant AND denied.subject=" + subj + " AND denied.status<>'active')" + filtroVaciado("msg", subj) + " AND NOT EXISTS (SELECT 1 FROM channel_members hidden WHERE hidden.tenant=c.tenant AND hidden.channel_id=c.id AND hidden.subject=" + subj + " AND hidden.hidden_at IS NOT NULL)"
}

func readMessageMetadataTx(ctx context.Context, tx pgx.Tx, tenant, subject string, before *roomsearch.Position, limit int, ids []uuid.UUID, external bool) ([]roomsearch.Metadata, error) {
	var qb sqlArgs
	org, actor := qb.add(tenant), qb.add(subject)
	sql := `SELECT msg.id,msg.channel_id,msg.parent_id,msg.author_sub,msg.author_name,msg.created_at,msg.edited_at,msg.matrix_room_id,msg.matrix_event_id,msg.type,c.slug,c.name,
 EXISTS (SELECT 1 FROM message_saves saved WHERE saved.tenant=msg.tenant AND saved.message_id=msg.id AND saved.subject=` + actor + `),
 EXISTS (SELECT 1 FROM message_pins pinned WHERE pinned.tenant=msg.tenant AND pinned.message_id=msg.id)
 FROM messages msg JOIN channels c ON c.tenant=msg.tenant AND c.id=msg.channel_id
 WHERE msg.tenant=` + org + ` AND msg.deleted_at IS NULL AND c.deleted_at IS NULL` + searchAccess(actor, external)
	if before != nil {
		sql += " AND (msg.created_at,msg.id,msg.tenant) < (" + qb.add(before.CreatedAt) + "," + qb.add(before.ID) + "::uuid," + qb.add(before.Tenant) + ")"
	}
	if ids != nil {
		sql += " AND msg.id = ANY(" + qb.add(ids) + "::uuid[])"
	}
	sql += " ORDER BY msg.created_at DESC,msg.id DESC,msg.tenant DESC LIMIT " + qb.add(limit)
	rows, err := tx.Query(ctx, sql, qb.vals...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []roomsearch.Metadata{}
	for rows.Next() {
		var m roomsearch.Metadata
		var id, channel uuid.UUID
		var parent *uuid.UUID
		var room, event string
		if err := rows.Scan(&id, &channel, &parent, &m.AuthorSub, &m.AuthorName, &m.CreatedAt, &m.EditedAt, &room, &event, &m.Type, &m.ChannelSlug, &m.ChannelName, &m.Saved, &m.Pinned); err != nil {
			return nil, err
		}
		m.ID, m.ChannelID, m.Tenant = id.String(), channel.String(), tenant
		if parent != nil {
			v := parent.String()
			m.ParentID = &v
		}
		if event != "" {
			m.MatrixEvent = &storage.Reference{RoomID: room, EventID: event}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) MessageMetadata(ctx context.Context, subject string, before *roomsearch.Position, limit int, ids []uuid.UUID) ([]roomsearch.Metadata, error) {
	tenant := tenancy.MustFrom(ctx)
	var out []roomsearch.Metadata
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = readMessageMetadataTx(ctx, tx, tenant.Slug, subject, before, limit, ids, false)
		return err
	})
	return out, err
}

// ExternalMessageMetadata uses the existing external-membership tenant discovery
// and RLS switch; a host's public teams are not granted by an external channel.
func (s *Store) ExternalMessageMetadata(ctx context.Context, subject, own string, before *roomsearch.Position, limit int, ids map[string][]uuid.UUID) ([]roomsearch.Metadata, error) {
	out := []roomsearch.Metadata{}
	err := s.enCadaAnfitrion(ctx, subject, own, func(tx pgx.Tx, tenant string) (bool, error) {
		var selected []uuid.UUID
		if ids != nil {
			selected = ids[tenant]
			if len(selected) == 0 {
				return false, nil
			}
		}
		rows, err := readMessageMetadataTx(ctx, tx, tenant, subject, before, limit, selected, true)
		out = append(out, rows...)
		return false, err
	})
	return out, err
}

// SearchIdentityActive reads only the global lifecycle tombstone. Message data
// stays behind ordinary tenant RLS; a maintenance transaction returns one bool.
func (s *Store) SearchIdentityActive(ctx context.Context, subject string) (bool, error) {
	var active bool
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM matrix_identity_access WHERE subject=$1 AND status<>'active')`, subject).Scan(&active)
	})
	return active, err
}

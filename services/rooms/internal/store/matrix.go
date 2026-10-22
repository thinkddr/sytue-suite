// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// MatrixGate only controls access metadata; it has no encryption keys.
type MatrixGate interface {
	Reconcile(context.Context, string, []string, bool, bool) error
	Redact(context.Context, domain.MatrixEventRef) error
}

func (s *Store) EditMatrixMessage(ctx context.Context, id uuid.UUID, actor string, ref domain.MatrixEventRef, validate func(context.Context, MatrixChannel) error) (domain.Message, error) {
	t := tenancy.MustFrom(ctx)
	var result domain.Message
	err := s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		current, err := getMessageTx(ctx, tx, t.Slug, id)
		if err != nil {
			return err
		}
		if err := lockMatrixChannelTx(ctx, tx, t.Slug, current.ChannelID); err != nil {
			return err
		}
		current, err = getMessageTx(ctx, tx, t.Slug, id)
		if err != nil {
			return err
		}
		if current.AuthorSub != actor {
			return domain.ErrForbidden
		}
		if current.MatrixEvent == nil {
			return domain.ErrEncryptedRequired
		}
		state, err := matrixChannelTx(ctx, tx, t.Slug, current.ChannelID)
		if err != nil {
			return err
		}
		if err := requireWriteAccess(ctx, tx, t.Slug, state.Channel, actor, current.AuthorName); err != nil {
			return err
		}
		if state.RoomID != ref.RoomID || validate == nil {
			return domain.ErrForbidden
		}
		if err := validate(matrixContext(ctx, tx), state); err != nil {
			return err
		}
		result, err = scanMessage(tx.QueryRow(ctx, `UPDATE messages SET matrix_event_id=$3,edited_at=now() WHERE tenant=$1 AND id=$2 RETURNING `+messageCols, t.Slug, id, ref.EventID), t.Slug)
		return err
	})
	return result, err
}

func (s *Store) redactMatrixMessageTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) error {
	var ref domain.MatrixEventRef
	err := tx.QueryRow(ctx, `SELECT matrix_room_id,matrix_event_id FROM messages WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`, tenant, id).Scan(&ref.RoomID, &ref.EventID)
	if err != nil {
		return err
	}
	if ref.EventID == "" {
		return nil
	}
	if s.matrixGate == nil {
		return domain.ErrMatrixUnavailable
	}
	return s.matrixGate.Redact(ctx, ref)
}

func (s *Store) SetMatrixGate(g MatrixGate) { s.matrixGate = g }

type MatrixChannel struct {
	ChannelID uuid.UUID      `json:"channel_id"`
	RoomID    string         `json:"matrix_room_id"`
	Channel   domain.Channel `json:"-"`
	Subjects  []string       `json:"-"`
	ReadOnly  bool           `json:"-"`
}

func lockMatrixChannelTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) error {
	var locked uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM channels WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, tenant, id).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func matrixSubjectsTx(ctx context.Context, tx pgx.Tx, tenant string, ch domain.Channel) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT subject FROM (SELECT subject FROM channel_members cm WHERE cm.tenant=$1 AND cm.channel_id=$2
 AND (externo OR $3='direct' OR EXISTS(SELECT 1 FROM team_members tm WHERE tm.tenant=$1 AND tm.team_id=$4 AND tm.subject=cm.subject))
 UNION SELECT subject FROM team_members WHERE tenant=$1 AND team_id=$4 AND $3='public') allowed WHERE NOT EXISTS(SELECT 1 FROM matrix_tenant_access d WHERE d.tenant=$1 AND d.subject=allowed.subject AND d.status<>'active') AND NOT EXISTS(SELECT 1 FROM matrix_identity_access g WHERE g.subject=allowed.subject AND g.status<>'active') ORDER BY subject`, tenant, ch.ID, string(ch.Kind), ch.TeamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subjects []string
	for rows.Next() {
		var sub string
		if err := rows.Scan(&sub); err != nil {
			return nil, err
		}
		if !domain.IsBotAuthor(sub) {
			subjects = append(subjects, sub)
		}
	}
	return subjects, rows.Err()
}

func matrixChannelTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) (MatrixChannel, error) {
	ch, err := getChannelTx(ctx, tx, tenant, id)
	if err != nil {
		return MatrixChannel{}, err
	}
	state := MatrixChannel{ChannelID: id, Channel: ch, ReadOnly: ch.Archived}
	state.Subjects, err = matrixSubjectsTx(ctx, tx, tenant, ch)
	if err != nil {
		return MatrixChannel{}, err
	}
	err = tx.QueryRow(ctx, `SELECT room_id FROM matrix_channel_rooms WHERE tenant=$1 AND channel_id=$2`, tenant, id).Scan(&state.RoomID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	if err == nil && ch.Kind == domain.ChannelDirect && len(state.Subjects) == 2 {
		blocked := bloqueoEnDirectoTx(ctx, tx, tenant, id, state.Subjects[0])
		if errors.Is(blocked, domain.ErrBlocked) {
			state.ReadOnly = true
		} else if blocked != nil {
			return MatrixChannel{}, blocked
		}
	}
	return state, err
}

// MatrixChannelIDs lists only channels this actor can read under the existing ACL.
func (s *Store) MatrixChannelIDs(ctx context.Context, actor string) ([]uuid.UUID, error) {
	t := tenancy.MustFrom(ctx)
	var ids []uuid.UUID
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT c.id FROM channels c WHERE c.tenant=$1 AND c.deleted_at IS NULL AND (
 (c.kind='public' AND EXISTS(SELECT 1 FROM team_members tm WHERE tm.tenant=c.tenant AND tm.team_id=c.team_id AND tm.subject=$2))
 OR EXISTS(SELECT 1 FROM channel_members cm WHERE cm.tenant=c.tenant AND cm.channel_id=c.id AND cm.subject=$2
 AND (cm.externo OR c.kind='direct' OR EXISTS(SELECT 1 FROM team_members tm WHERE tm.tenant=c.tenant AND tm.team_id=c.team_id AND tm.subject=$2)))) ORDER BY c.id`, t.Slug, actor)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	return ids, err
}

// EnsureMatrixChannel serializes provisioning and access changes on the channel
// row. Every invited subject comes from an ACL already committed before bootstrap.
func (s *Store) EnsureMatrixChannel(ctx context.Context, id uuid.UUID, actor string, provision func(context.Context, MatrixChannel) (string, error)) (MatrixChannel, error) {
	t := tenancy.MustFrom(ctx)
	var state MatrixChannel
	err := s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		if err := lockMatrixChannelTx(ctx, tx, t.Slug, id); err != nil {
			return err
		}
		var err error
		state, err = matrixChannelTx(ctx, tx, t.Slug, id)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, state.Channel, actor); err != nil {
			return err
		}
		if !slices.Contains(state.Subjects, actor) {
			return domain.ErrForbidden
		}
		room, err := provision(matrixContext(ctx, tx), state)
		if err != nil {
			return err
		}
		state.RoomID = room
		_, err = tx.Exec(ctx, `INSERT INTO matrix_channel_rooms(tenant,channel_id,room_id) VALUES($1,$2,$3) ON CONFLICT(tenant,channel_id) DO NOTHING`, t.Slug, id, room)
		return err
	})
	return state, err
}

// revokeMatrixTx runs after the local change but before its transaction commits.
// A remote error rolls the local change back. No invitations happen here.
func (s *Store) revokeMatrixTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) error {
	state, err := matrixChannelTx(ctx, tx, tenant, id)
	if err != nil {
		return err
	}
	if err := s.revokeChannelCallsTx(ctx, tx, tenant, id); err != nil {
		return err
	}
	if state.RoomID == "" {
		return nil
	}
	if s.matrixGate == nil {
		return domain.ErrMatrixUnavailable
	}
	return s.matrixGate.Reconcile(ctx, state.RoomID, state.Subjects, state.ReadOnly, false)
}

func (s *Store) lockTeamMatrixChannelsTx(ctx context.Context, tx pgx.Tx, tenant string, team uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT c.id FROM channels c WHERE c.tenant=$1 AND c.team_id=$2 AND c.kind<>'direct' AND (EXISTS(SELECT 1 FROM matrix_channel_rooms m WHERE m.tenant=c.tenant AND m.channel_id=c.id) OR EXISTS(SELECT 1 FROM rooms r WHERE r.tenant=c.tenant AND r.status<>'ended' AND r.metadata->'source'->>'kind'='chat' AND r.metadata->'source'->>'ref_id'=c.id::text)) ORDER BY c.id FOR UPDATE OF c`, tenant, team)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

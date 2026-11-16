// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"database/sql"
	"errors"

	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/id"
)

const outboundPolicyDDL = `CREATE TABLE IF NOT EXISTS sytue_outbound_recipient_policy_v1 (
 account_id TEXT NOT NULL,
 device_id TEXT NOT NULL,
 room_id TEXT NOT NULL,
 policy TEXT NOT NULL,
 session_id TEXT NOT NULL,
 PRIMARY KEY(account_id,device_id,room_id)
)`

// No value in this table is a credential, SSSS secret or recipient authority.
// It records only which SDK-created outbound was persisted through this exact
// installed controller/owner-only policy. A server or old binary cannot mark a
// different session by changing a version flag or a public backup payload.
func (s controllerStore) recordOutboundPolicy(ctx context.Context, room id.RoomID, session id.SessionID) error {
	if s.sqlStore == nil || s.sqlStore.AccountID == "" || s.sqlStore.DeviceID == "" || s.policy == "" || room == "" {
		return ErrSecureStore
	}
	_, err := s.sqlStore.DB.Exec(ctx, `INSERT INTO sytue_outbound_recipient_policy_v1(account_id,device_id,room_id,policy,session_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT(account_id,device_id,room_id) DO UPDATE SET policy=excluded.policy,session_id=excluded.session_id`, s.sqlStore.AccountID, s.sqlStore.DeviceID, room, s.policy, session)
	return err
}

// The maintained SDK owns session creation, pickling and sharing. This single
// store boundary persists its ordinary write and public migration stamp in the
// same transaction, so a crash/failure can never stamp an uncommitted session.
func (s controllerStore) AddOutboundGroupSession(ctx context.Context, session *mxcrypto.OutboundGroupSession) error {
	if s.sqlStore == nil {
		return s.Store.AddOutboundGroupSession(ctx, session)
	}
	if session == nil || s.policy == "" {
		return ErrSecureStore
	}
	return s.sqlStore.DB.DoTxn(ctx, nil, func(ctx context.Context) error {
		if _, err := s.sqlStore.DB.Exec(ctx, outboundPolicyDDL); err != nil {
			return err
		}
		if err := s.Store.AddOutboundGroupSession(ctx, session); err != nil {
			return err
		}
		return s.recordOutboundPolicy(ctx, session.RoomID, session.ID())
	})
}

func (s controllerStore) ensureOutboundRecipientPolicy(ctx context.Context, rooms map[id.RoomID]struct{}) error {
	if s.sqlStore == nil || s.policy == "" {
		return ErrSecureStore
	}
	return s.sqlStore.DB.DoTxn(ctx, nil, func(ctx context.Context) error {
		if _, err := s.sqlStore.DB.Exec(ctx, outboundPolicyDDL); err != nil {
			return err
		}
		for room := range rooms {
			var previous, sessionID string
			err := s.sqlStore.DB.QueryRow(ctx, `SELECT policy,session_id FROM sytue_outbound_recipient_policy_v1 WHERE account_id=$1 AND device_id=$2 AND room_id=$3`, s.sqlStore.AccountID, s.sqlStore.DeviceID, room).Scan(&previous, &sessionID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			outbound, err := s.GetOutboundGroupSession(ctx, room)
			if err != nil {
				return err
			}
			current := ""
			if outbound != nil {
				current = outbound.ID().String()
			}
			if previous == s.policy && sessionID == current {
				continue
			}
			if err := s.RemoveOutboundGroupSession(ctx, room); err != nil {
				return err
			}
			if err := s.recordOutboundPolicy(ctx, room, ""); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *Client) currentOutboundRecipientPolicy() (controllerStore, error) {
	if c == nil || c.crypto == nil {
		return controllerStore{}, ErrSecureStore
	}
	policy, ok := c.crypto.Machine().CryptoStore.(controllerStore)
	if !ok || policy.sqlStore == nil || policy.sqlStore.AccountID != c.account || policy.sqlStore.DeviceID != c.session.DeviceID {
		return controllerStore{}, ErrSecureStore
	}
	return policy, nil
}

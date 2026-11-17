//go:build cgo

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/id"
)

func TestOutboundPolicyMigrationPreservesOnlyCurrentSDKSessionAndRollsBackFailure(t *testing.T) {
	ctx := t.Context()
	raw, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "sdk.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	db, err := dbutil.NewWithDB(raw, "sqlite3")
	if err != nil {
		t.Fatal(err)
	}
	newStore := func(account string, device id.DeviceID) controllerStore {
		store := mxcrypto.NewSQLCryptoStore(db, dbutil.ZeroLogger(zerolog.Nop()), account, device, []byte("synthetic-policy-pickle-key"))
		if err := store.DB.Upgrade(ctx); err != nil {
			t.Fatal(err)
		}
		return controllerStore{Store: store, sqlStore: store, controller: "@rooms_controller:fixture.test", ownerOnly: "@owner:fixture.test", policy: strings.Repeat("1", 64)}
	}
	room := id.RoomID("!local-policy-fixture")
	rooms := map[id.RoomID]struct{}{room: {}}
	fresh := func() *mxcrypto.OutboundGroupSession {
		group, err := mxcrypto.NewOutboundGroupSession(room, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return group
	}
	sessionIs := func(store controllerStore, wanted *mxcrypto.OutboundGroupSession) {
		got, err := store.GetOutboundGroupSession(ctx, room)
		if err != nil {
			t.Fatal(err)
		}
		if wanted == nil && got != nil || wanted != nil && (got == nil || got.ID() != wanted.ID()) {
			t.Fatal("policy migration retained or retired the wrong SDK session")
		}
	}
	first := newStore("policy-account", "DEVICE_A")
	legacy := fresh()
	if err := first.Store.AddOutboundGroupSession(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if err := first.ensureOutboundRecipientPolicy(ctx, rooms); err != nil {
		t.Fatal(err)
	}
	sessionIs(first, nil)
	current := fresh()
	if err := first.AddOutboundGroupSession(ctx, current); err != nil {
		t.Fatal(err)
	}
	if err := first.ensureOutboundRecipientPolicy(ctx, rooms); err != nil {
		t.Fatal(err)
	}
	sessionIs(first, current)
	// A later old binary can write a DIFFERENT outbound in the same database.
	// The old policy stamp must not authorize that replacement merely by version.
	replacement := fresh()
	if err := first.Store.AddOutboundGroupSession(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	if err := first.ensureOutboundRecipientPolicy(ctx, rooms); err != nil {
		t.Fatal(err)
	}
	sessionIs(first, nil)
	for _, other := range []controllerStore{newStore("policy-other-account", "DEVICE_A"), newStore("policy-account", "DEVICE_B")} {
		unmarked := fresh()
		if err := other.Store.AddOutboundGroupSession(ctx, unmarked); err != nil {
			t.Fatal(err)
		}
		if err := other.ensureOutboundRecipientPolicy(ctx, rooms); err != nil {
			t.Fatal(err)
		}
		sessionIs(other, nil)
	}
	failing := newStore("policy-failure", "FAILURE_DEVICE")
	retained := fresh()
	if err := failing.Store.AddOutboundGroupSession(ctx, retained); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `CREATE TRIGGER reject_policy_stamp BEFORE INSERT ON sytue_outbound_recipient_policy_v1 WHEN NEW.account_id='policy-failure' BEGIN SELECT RAISE(ABORT,'synthetic policy stamp failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := failing.ensureOutboundRecipientPolicy(ctx, rooms); err == nil {
		t.Fatal("failed stamp was accepted")
	}
	sessionIs(failing, retained)
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM sytue_outbound_recipient_policy_v1 WHERE account_id=$1 AND device_id=$2`, failing.sqlStore.AccountID, failing.sqlStore.DeviceID).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed migration marked success", err)
	}
	candidate := fresh()
	if err := failing.AddOutboundGroupSession(ctx, candidate); err == nil {
		t.Fatal("failed SDK stamp committed")
	}
	sessionIs(failing, retained)
	if _, err := db.Exec(ctx, `DROP TRIGGER reject_policy_stamp`); err != nil {
		t.Fatal(err)
	}
	if err := failing.AddOutboundGroupSession(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	if err := failing.ensureOutboundRecipientPolicy(ctx, rooms); err != nil {
		t.Fatal(err)
	}
	sessionIs(failing, candidate)
	upgraded := failing
	upgraded.policy = strings.Repeat("2", 64)
	if err := upgraded.ensureOutboundRecipientPolicy(ctx, rooms); err != nil {
		t.Fatal(err)
	}
	sessionIs(upgraded, nil)
}

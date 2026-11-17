// SPDX-License-Identifier: Apache-2.0
package sessionproof

import (
	"bytes"
	"encoding/base64"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func fixture() (storage.Scope, Session) {
	key := func(v byte) string { return base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{v}, 32)) }
	return storage.Scope{Tenant: "acme", Subject: "alice", Product: "rooms"}, Session{RoomID: "!session:fixture", SessionID: key(1), SenderKey: key(2), SenderUser: "@alice:fixture", SigningKey: key(3)}
}

func TestSessionProofBindsEveryOwnerAndSDKField(t *testing.T) {
	scope, session := fixture()
	encoded, err := Encode(scope, session)
	if err != nil || !Matches(scope, session, encoded.Payload) {
		t.Fatal("rooms scope failed", err)
	}
	owners := []storage.Scope{scope, scope, scope}
	owners[0].Tenant = "other"
	owners[1].Subject = "bob"
	owners[2].Product = "notes"
	for _, owner := range owners {
		changed, err := Encode(owner, session)
		if err != nil || changed.ID == encoded.ID || Matches(owner, session, encoded.Payload) {
			t.Fatal("owner binding lost", err)
		}
	}
	sessions := []Session{session, session, session, session, session}
	sessions[0].RoomID = "!other:fixture"
	sessions[1].SessionID = session.SenderKey
	sessions[2].SenderKey = session.SessionID
	sessions[3].SenderUser = "@bob:fixture"
	sessions[4].SigningKey = session.SenderKey
	for _, candidate := range sessions {
		changed, err := Encode(scope, candidate)
		if err != nil || changed.ID == encoded.ID || Matches(scope, candidate, encoded.Payload) {
			t.Fatal("SDK identity binding lost", err)
		}
	}
	for _, bad := range [][]byte{append(bytes.Clone(encoded.Payload), ' '), append(bytes.Clone(encoded.Payload), encoded.Payload...), bytes.Replace(encoded.Payload, []byte(Kind), []byte("legacy"), 1), bytes.Replace(encoded.Payload, []byte(`"scope":`), []byte(`"unknown":`), 1)} {
		if Matches(scope, session, bad) {
			t.Fatal("ambiguous or foreign provenance accepted")
		}
	}
}

func TestSessionProofNeverPrunesOtherSessions(t *testing.T) {
	scope, session := fixture()
	first, err := Encode(scope, session)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{first.ID: true}
	for i := 0; i < 1024; i++ {
		key := make([]byte, 32)
		key[0], key[1] = byte(i), byte(i>>8)
		session.SessionID = base64.RawStdEncoding.EncodeToString(key)
		encoded, err := Encode(scope, session)
		if err != nil {
			t.Fatal(err)
		}
		seen[encoded.ID] = true
	}
	scope, session = fixture()
	again, err := Encode(scope, session)
	if err != nil || again.ID != first.ID || !Matches(scope, session, first.Payload) || len(seen) < 1024 {
		t.Fatal("independent history was pruned", err)
	}
}

func TestSessionProofRejectsInvalidKeysAndUnknownInput(t *testing.T) {
	scope, session := fixture()
	session.SenderKey = "not-a-key"
	if _, err := Encode(scope, session); err == nil {
		t.Fatal("invalid SDK key accepted")
	}
	for _, raw := range []string{`{}`, `{"private":"ignored"}`, `null`} {
		if _, err := DecodeSession([]byte(raw)); err == nil {
			t.Fatal("invalid SDK tuple accepted")
		}
	}
}

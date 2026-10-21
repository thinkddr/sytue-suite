// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"maunium.net/go/mautrix"
	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/olm"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// This exercises the real SDK sharing and Olm/Megolm decryptors, with a hostile
// maximally-trusted controller device. Only the verified human receives a key.
func TestSDKOnlyVerifiedHumanDecryptsWithControllerPresent(t *testing.T) {
	const controller id.UserID = "@rooms_controller:matrix.test"
	const human id.UserID = "@sytue_human:matrix.test"
	const room id.RoomID = "!artificial:matrix.test"
	var delivered mautrix.ReqSendToDevice
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/keys/query") {
			var request mautrix.ReqQueryKeys
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if _, requested := request.DeviceKeys[controller]; requested {
				t.Error("controller bypassed empty inventory through key fetching")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"device_keys":{}}`))
			return
		}
		if !strings.Contains(r.URL.Path, "/sendToDevice/m.room.encrypted/") {
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&delivered); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	cli, err := mautrix.NewClient(server.URL, "@sytue_sender:matrix.test", "artificial")
	if err != nil {
		t.Fatal(err)
	}
	cli.Client, cli.DeviceID = server.Client(), "SENDER"
	store := mxcrypto.NewMemoryStore(nil)
	state := mautrix.NewMemoryStateStore()
	if err = state.SetEncryptionEvent(t.Context(), room, &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}); err != nil {
		t.Fatal(err)
	}
	if err = state.SetHistoryVisibility(t.Context(), room, &event.HistoryVisibilityEventContent{HistoryVisibility: event.HistoryVisibilityJoined}); err != nil {
		t.Fatal(err)
	}
	cryptoState, ok := state.(mxcrypto.StateStore)
	if !ok {
		t.Fatal("SDK memory state lacks crypto interface")
	}
	mach := mxcrypto.NewOlmMachine(cli, nil, controllerStore{Store: store, controller: controller}, cryptoState)
	if err = mach.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	mach.SendKeysMinTrust = id.TrustStateCrossSignedVerified
	receiver := mxcrypto.NewOlmAccount()
	if err = receiver.Internal.GenOneTimeKeys(1); err != nil {
		t.Fatal(err)
	}
	oneTime, err := receiver.Internal.OneTimeKeys()
	if err != nil {
		t.Fatal(err)
	}
	var otk id.Curve25519
	for _, key := range oneTime {
		otk = key
	}
	session, err := mach.GetAccount().Internal.NewOutboundSession(receiver.IdentityKey(), otk)
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []id.UserID{controller, human} {
		device := &id.Device{UserID: user, DeviceID: "VERIFIED", IdentityKey: receiver.IdentityKey(), SigningKey: receiver.SigningKey(), Trust: id.TrustStateVerified}
		if err = store.PutDevice(t.Context(), user, device); err != nil {
			t.Fatal(err)
		}
	}
	if err = store.AddSession(t.Context(), receiver.IdentityKey(), &mxcrypto.OlmSession{Internal: session, ExpirationMixin: mxcrypto.ExpirationMixin{TimeMixin: mxcrypto.TimeMixin{CreationTime: time.Now(), LastEncryptedTime: time.Now(), LastDecryptedTime: time.Now()}}}); err != nil {
		t.Fatal(err)
	}
	if err = mach.ShareGroupSession(t.Context(), room, []id.UserID{controller, human}); err != nil {
		t.Fatal(err)
	}
	if len(delivered.Messages) != 1 || len(delivered.Messages[human]) != 1 || len(delivered.Messages[controller]) != 0 {
		t.Fatal("SDK sent key outside verified human recipient")
	}
	encoded, err := json.Marshal(delivered.Messages[human]["VERIFIED"])
	if err != nil {
		t.Fatal(err)
	}
	var encrypted event.EncryptedEventContent
	if err = json.Unmarshal(encoded, &encrypted); err != nil {
		t.Fatal(err)
	}
	message, ok := encrypted.OlmCiphertext[receiver.IdentityKey()]
	if !ok {
		t.Fatal("human has no Olm ciphertext")
	}
	incoming, err := receiver.NewInboundSessionFrom(mach.GetAccount().IdentityKey(), message.Body)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := incoming.Decrypt(message.Body, message.Type)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Content event.RoomKeyEventContent `json:"content"`
	}
	if err = json.Unmarshal(plaintext, &envelope); err != nil {
		t.Fatal(err)
	}
	group, err := olm.NewInboundGroupSession([]byte(envelope.Content.SessionKey))
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := mach.EncryptMegolmEvent(t.Context(), room, event.EventMessage, &event.MessageEventContent{MsgType: event.MsgText, Body: "artificial verified-human payload"})
	if err != nil {
		t.Fatal(err)
	}
	cleartext, _, err := group.Decrypt(cipher.MegolmCiphertext)
	if err != nil || !strings.Contains(string(cleartext), "artificial verified-human payload") {
		t.Fatal("verified human could not decrypt SDK message", err)
	}
}

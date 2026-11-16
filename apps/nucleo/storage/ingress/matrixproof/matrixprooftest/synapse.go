// SPDX-License-Identifier: Apache-2.0
package matrixprooftest

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// Register creates only a synthetic account on the caller's disposable Synapse.
// SDK callers retain their own client setup, real crypto and recovery lifecycle.
func Register(ctx context.Context, t testing.TB, client *mautrix.Client, username, password string, device id.DeviceID) *mautrix.RespRegister {
	t.Helper()
	request := &mautrix.ReqRegister[any]{Username: username, Password: password, DeviceID: device}
	registered, challenge, err := client.Register(ctx, request)
	if err == nil && challenge != nil {
		request.Auth = map[string]any{"type": "m.login.dummy", "session": challenge.Session}
		registered, _, err = client.Register(ctx, request)
	}
	if err != nil || registered == nil {
		t.Fatal("disposable Synapse registration failed", err)
	}
	client.UserID, client.DeviceID, client.AccessToken = registered.UserID, registered.DeviceID, registered.AccessToken
	return registered
}

// StoragePowers is the real Matrix write fence used by historical cutover tests.
// Freeze changes only standard power levels, never an SDK verification flag.
func StoragePowers(frozen bool) map[string]any {
	encrypted := 0
	if frozen {
		encrypted = 100
	}
	return map[string]any{"users": map[string]int{}, "users_default": 0, "events_default": 100, "state_default": 100, "invite": 100, "kick": 100, "ban": 100, "redact": 100, "events": map[string]int{"m.room.encrypted": encrypted, "m.room.message": 100}}
}

func EncryptedStorageRoom(ctx context.Context, t testing.TB, controller *mautrix.Client, member id.UserID) id.RoomID {
	t.Helper()
	var room mautrix.RespCreateRoom
	body := map[string]any{
		"visibility": "private", "preset": "private_chat", "room_version": "12",
		"creation_content": map[string]bool{"m.federate": false}, "invite": []string{member.String()}, "power_level_content_override": StoragePowers(false),
		"initial_state": []map[string]any{
			{"type": "m.room.encryption", "state_key": "", "content": map[string]string{"algorithm": string(id.AlgorithmMegolmV1)}},
			{"type": "m.room.history_visibility", "state_key": "", "content": map[string]string{"history_visibility": "joined"}},
		},
	}
	if _, err := controller.MakeRequest(ctx, http.MethodPost, controller.BuildClientURL("v3", "createRoom"), body, &room); err != nil {
		t.Fatal("disposable encrypted room", err)
	}
	return room.RoomID
}

// PublishSigningChain uploads disposable keys using only maintained SDK APIs.
// installRoot is used for a new fixture account; additional devices preserve
// the existing account root/self-signing keys and upload only their own keys.
func PublishSigningChain(ctx context.Context, t testing.TB, client *mautrix.Client, chain *Fixture, password string, installRoot bool) {
	t.Helper()
	user, device := chain.Origin.UserID, chain.Origin.DeviceID
	if client.UserID != user || client.DeviceID != device {
		t.Fatal("fixture SDK identity mismatch")
	}
	if installRoot {
		private := NewSigning(t)
		userSigning := mautrix.CrossSigningKeys{UserID: user, Usage: []id.CrossSigningUsage{id.XSUsageUserSigning}, Keys: map[id.KeyID]id.Ed25519{id.NewKeyID(id.KeyAlgorithmEd25519, private.PublicKey().String()): private.PublicKey()}}
		userSigning.Signatures = Sign(t, chain.Master, userSigning, user, chain.Master.PublicKey().String())
		request := &mautrix.UploadCrossSigningKeysReq[any]{Master: chain.Origin.Keys.MasterKeys[user], SelfSigning: chain.Origin.Keys.SelfSigningKeys[user], UserSigning: userSigning}
		if err := client.UploadCrossSigningKeys(ctx, request, func(challenge *mautrix.RespUserInteractive) any {
			return map[string]any{"type": "m.login.password", "identifier": map[string]string{"type": "m.id.user", "user": user.String()}, "password": password, "session": challenge.Session}
		}); err != nil {
			t.Fatal("fixture SDK cross-signing upload", err)
		}
	}
	keys := chain.Origin.Keys.DeviceKeys[user][device]
	if _, err := client.UploadKeys(ctx, &mautrix.ReqUploadKeys{DeviceKeys: &keys}); err != nil {
		t.Fatal("fixture SDK device upload", err)
	}
	raw, err := json.Marshal(keys)
	if err != nil {
		t.Fatal(err)
	}
	var signed mautrix.ReqKeysSignatures
	if err := json.Unmarshal(raw, &signed); err != nil {
		t.Fatal(err)
	}
	request := mautrix.ReqUploadSignatures{user: map[string]mautrix.ReqKeysSignatures{device.String(): signed}}
	result, err := client.UploadSignatures(ctx, &request)
	if err != nil || result == nil || len(result.Failures) != 0 {
		t.Fatal("fixture SDK self-signature upload", err)
	}
}

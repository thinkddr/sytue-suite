// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// emitInteropVector is opt-in test tooling for the independent RustCrypto↔Go Olm
// fixture. All keys belong to an artificial httptest device, never an account.
func emitInteropVector(t *testing.T, c *Client, ciphertext []byte, plaintext string, descriptor event.EncryptedFileInfo, media []byte, fileText string) {
	t.Helper()
	output := os.Getenv("SYTUE_MATRIX_VECTOR_OUT")
	if output == "" {
		return
	}
	var encrypted event.EncryptedEventContent
	if err := json.Unmarshal(ciphertext, &encrypted); err != nil {
		t.Fatal(err)
	}
	session, err := c.crypto.Machine().CryptoStore.GetGroupSession(t.Context(), "!room:matrix.test", encrypted.SessionID)
	if err != nil || session == nil {
		t.Fatalf("interop session: %v", err)
	}
	exported, err := session.Internal.Export(session.Internal.FirstKnownIndex())
	if err != nil {
		t.Fatal(err)
	}
	defer clear(exported)
	vector := map[string]any{
		"artificial": true,
		"generator":  "mautrix-go v0.32.0/goolm; ARTIFICIAL TEST DEVICE ONLY",
		"plaintext":  plaintext,
		"sessions":   []mxcrypto.ExportedSession{{Algorithm: id.AlgorithmMegolmV1, RoomID: session.RoomID, SenderKey: session.SenderKey, SenderClaimedKeys: mxcrypto.SenderClaimedKeys{Ed25519: session.SigningKey}, SessionID: session.ID(), SessionKey: string(exported), ForwardingChains: &[]string{}}},
		"events":     []*event.Event{{RoomID: session.RoomID, ID: "$event", Timestamp: 1792152000000, Sender: c.cli.UserID, Type: event.EventEncrypted, Content: event.Content{VeryRaw: ciphertext}}},
		"attachment": map[string]any{"descriptor": descriptor, "ciphertext": base64.StdEncoding.EncodeToString(media), "plaintext": base64.StdEncoding.EncodeToString([]byte(fileText))},
	}
	encoded, err := json.MarshalIndent(vector, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

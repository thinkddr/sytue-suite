// SPDX-License-Identifier: Apache-2.0
package publication_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

func TestPublicationAdmissionAuthenticatesExactCipherAndProducer(t *testing.T) {
	scope := storage.Scope{Tenant: "fixture", Subject: "owner", Product: "mail"}
	f := matrixprooftest.New(t, scope, "matrix.test", "DEVICE")
	device, err := matrixproof.Verify(t.Context(), f.Origin, f.Master.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	request := publication.Request{Version: 1, Domain: "org.sytue.publication.admission.v1", Namespace: "storage:mail", Scope: scope, Target: storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "DEVICE"}, TransactionID: "retained-intent.0001", CipherSHA256: strings.Repeat("a", 64), AuthorityGeneration: 9223372036854775807}
	signed := publication.SignedRequest{Request: request}
	signed.Signatures = matrixprooftest.Sign(t, f.Device, signed, f.Origin.UserID, f.Origin.DeviceID.String())
	verified, err := publication.VerifyRequest(t.Context(), signed, request, device)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Value() != request || verified.SHA256() == "" {
		t.Fatal("verified public request changed")
	}
	raw, err := json.Marshal(signed)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "\"authority_generation\":\"9223372036854775807\"") {
		t.Fatal("int64 generation lost canonical decimal wire")
	}
	evidence, err := verified.PublicEvidence()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = matrixproof.VerifyHistoricalDevice(t.Context(), evidence, device.Identity()); err != nil {
		t.Fatal("verified admission evidence did not preserve SDK origin", err)
	}
	for _, fault := range []string{"scope", "namespace", "room", "device", "transaction", "cipher", "generation", "unsigned"} {
		t.Run(fault, func(t *testing.T) {
			altered := signed
			switch fault {
			case "scope":
				altered.Scope.Product = "docs"
			case "namespace":
				altered.Namespace = "resources:mail"
			case "room":
				altered.Target.RoomID = "!other:matrix.test"
			case "device":
				altered.Target.DeviceID = "OTHER"
			case "transaction":
				altered.TransactionID += ".other"
			case "cipher":
				altered.CipherSHA256 = strings.Repeat("b", 64)
			case "generation":
				altered.AuthorityGeneration--
			case "unsigned":
				altered.Signatures = nil
			}
			// Even when a request's fields are supplied as expected, the maintained SDK
			// signature must still authenticate that exact public transcript.
			if _, err := publication.VerifyRequest(t.Context(), altered, altered.Request, device); err == nil {
				t.Fatal("changed admission authorized", fault)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = publication.VerifyRequest(ctx, signed, request, device); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = (publication.VerifiedRequest{}).PublicEvidence(); err == nil {
		t.Fatal("zero request exposed verified evidence")
	}
}

func TestWireCommitmentExcludesOnlyExactOpaqueAdmissionMarker(t *testing.T) {
	raw := []byte(`{"session_id":"synthetic-session","ciphertext":"synthetic-ciphertext","algorithm":"m.megolm.v1.aes-sha2","m.relates_to":{"event_id":"$original","rel_type":"m.thread"}}`)
	before, _, err := publication.WireDigest(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	marker := publication.Marker{Version: 1, ID: base64.RawURLEncoding.EncodeToString(make([]byte, 32))}
	var content map[string]any
	if err = json.Unmarshal(raw, &content); err != nil {
		t.Fatal(err)
	}
	content[publication.MarkerName] = marker
	marked, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	after, got, err := publication.WireDigest(marked, true)
	if err != nil || before != after || got != marker {
		t.Fatal("marker changed ciphertext commitment", err)
	}
	for _, field := range []string{"ciphertext", "session_id", "m.relates_to"} {
		t.Run(field, func(t *testing.T) {
			var changed map[string]any
			if err := json.Unmarshal(marked, &changed); err != nil {
				t.Fatal(err)
			}
			if field == "m.relates_to" {
				changed[field] = map[string]any{"event_id": "$another", "rel_type": "m.thread"}
			} else {
				changed[field] = "changed"
			}
			raw, _ := json.Marshal(changed)
			hash, _, err := publication.WireDigest(raw, true)
			if err == nil && hash == before {
				t.Fatal("changed SDK wire not covered", field)
			}
		})
	}
	for _, invalid := range []string{
		string(raw), "{}", "[]", "null", "{",
		`{"algorithm":"plaintext","ciphertext":"x","session_id":"s"}`,
		`{"algorithm":"m.megolm.v1.aes-sha2","ciphertext":"x","session_id":"s","org.sytue.publication.admission":{"v":1,"id":"invalid"}}`,
	} {
		if _, _, err := publication.WireDigest([]byte(invalid), true); err == nil {
			t.Fatal("invalid required marker accepted")
		}
	}
	content[publication.MarkerName] = map[string]any{"v": 1, "id": marker.ID, "extra": true}
	invalid, _ := json.Marshal(content)
	if _, _, err = publication.WireDigest(invalid, true); err == nil {
		t.Fatal("unchecked marker metadata was excluded from commitment")
	}
}

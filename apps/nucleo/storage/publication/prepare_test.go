// SPDX-License-Identifier: Apache-2.0
package publication_test

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

func TestPublicationRequestBuilderKeepsSDKWireAndExactDecimalIntent(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}
	f := matrixprooftest.New(t, scope, "matrix.test", "DEVICE")
	authority := matrixproof.Authority{Version: 1, Scope: scope, UserID: f.Origin.UserID.String(), RootKey: f.Master.PublicKey().String(), Generation: 2, Status: "active"}
	target := storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "DEVICE"}
	wire := []byte(`{"algorithm":"m.megolm.v1.aes-sha2","session_id":"synthetic-builder","ciphertext":"opaque","m.relates_to":{"event_id":"$thread"}}`)
	prepared, err := publication.BuildStorageRequest(scope, target, "fixed.1", wire, authority)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Request.Scope != scope || prepared.Request.Target != target || prepared.Request.TransactionID != "fixed.1" || prepared.Request.Namespace != "storage:mail" || prepared.Request.AuthorityGeneration != 2 {
		t.Fatal("builder changed original intent")
	}
	hash, _, err := publication.WireDigest(wire, false)
	if err != nil || prepared.Request.CipherSHA256 != hash {
		t.Fatal("SDK cipher not covered", err)
	}
	canonical, err := matrixproof.CanonicalBytes(prepared.Request)
	if err != nil || !bytes.Equal(canonical, prepared.Payload) {
		t.Fatal("canonical payload changed", err)
	}
	digest, err := matrixproof.RequestDigest(prepared.Request)
	if err != nil || prepared.SHA256 != digest {
		t.Fatal("request commitment changed", err)
	}
	signature := matrixprooftest.Sign(t, f.Device, prepared.Request, f.Origin.UserID, f.Origin.DeviceID.String())
	device, err := matrixproof.Verify(t.Context(), f.Origin, f.Master.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publication.VerifyRequest(t.Context(), publication.SignedRequest{Request: prepared.Request, Signatures: signature}, prepared.Request, device); err != nil {
		t.Fatal("same SDK cannot authenticate canonical builder request", err)
	}
	changed := bytes.Replace(wire, []byte("$thread"), []byte("$other"), 1)
	other, err := publication.BuildStorageRequest(scope, target, "fixed.1", changed, authority)
	if err != nil || prepared.SHA256 == other.SHA256 {
		t.Fatal("changed wire reused admission", err)
	}
	high, err := publication.BuildRequest(scope, "resources:mail", target, "fixed.1", math.MaxInt64, wire)
	if err != nil || !bytes.Contains(high.Payload, []byte(`"authority_generation":"9223372036854775807"`)) {
		t.Fatal("int64 lost in common constructor", err)
	}
	for _, fault := range []string{"scope", "inactive", "generation", "plaintext", "marker", "namespace", "transaction"} {
		t.Run(fault, func(t *testing.T) {
			a := authority
			actual := scope
			raw := wire
			txn := "fixed.1"
			switch fault {
			case "scope":
				actual.Product = "docs"
			case "inactive":
				a.Status = "retired"
			case "generation":
				a.Generation = 0
			case "plaintext":
				raw = []byte(`{"body":"cleartext"}`)
			case "marker":
				raw, err = publication.AttachMarker(raw, publication.Marker{Version: 1, ID: strings.Repeat("A", 43)})
				if err != nil {
					t.Fatal(err)
				}
			case "transaction":
				txn = "bad\nintent"
			case "namespace":
				if _, err := publication.BuildRequest(scope, "storage:docs", target, txn, 2, raw); err == nil {
					t.Fatal("foreign namespace built")
				}
				return
			}
			if _, err := publication.BuildStorageRequest(actual, target, txn, raw, a); err == nil {
				t.Fatal("invalid builder input accepted", fault)
			}
		})
	}
	var decoded publication.Request
	if err := json.Unmarshal(prepared.Payload, &decoded); err != nil || decoded != prepared.Request {
		t.Fatal("canonical payload did not preserve request", err)
	}
}

// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"encoding/json"
	"os"
	"testing"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// The optional vector contains disposable PUBLIC Rust SDK evidence only.
func TestBootstrapRustSDKCanonicalContext(t *testing.T) {
	path := os.Getenv("SYTUE_RESOURCE_BOOTSTRAP_VECTOR")
	if path == "" {
		t.Skip("opt-in actual Rust SDK vector")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		Context resource.BootstrapContext `json:"context"`
		Signed  json.RawMessage           `json:"signed_context"`
		Origin  json.RawMessage           `json:"origin"`
		Hash    string                    `json:"context_sha256"`
	}
	if err = json.Unmarshal(raw, &vector); err != nil || !vector.Context.Valid() {
		t.Fatal("invalid Rust context", err)
	}
	if vector.Context.SourceCutover.Seq != "9223372036854775807" {
		t.Fatal("Rust JSON lost SQL int64 precision")
	}
	hash, err := resource.BootstrapContextSHA256(vector.Context)
	if err != nil || hash != vector.Hash {
		t.Fatal("Go/Wasm and native context hashes differ", err)
	}
	var signedContext resource.BootstrapContext
	if err = json.Unmarshal(vector.Signed, &signedContext); err != nil || signedContext != vector.Context {
		t.Fatal("Rust signed another context", err)
	}
	origin, err := sessionproof.DecodeCertificateOrigin(vector.Origin)
	if err != nil {
		t.Fatal("shared public SDK origin codec", err)
	}
	device, err := matrixproof.VerifyUser(t.Context(), origin, id.UserID(vector.Context.Creator.UserID), id.Ed25519(vector.Context.Creator.RootKey))
	if err != nil {
		t.Fatal("maintained Rust cross-signing chain", err)
	}
	target := storage.PublicationTarget{RoomID: vector.Context.RoomID, DeviceID: vector.Context.Creator.DeviceID}
	if err = device.VerifyJSON(t.Context(), vector.Context.CreatorScope, target, vector.Signed); err != nil {
		t.Fatal("native Go rejected actual Rust context signature", err)
	}
	t.Log("actual Rust SDK signs canonical Go/Wasm bootstrap context; native Go verifies SDK chain and max int64 string")
}

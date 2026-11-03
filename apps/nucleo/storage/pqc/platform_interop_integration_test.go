//go:build integration && !js

// SPDX-License-Identifier: Apache-2.0
package pqc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

type platformInteropFixture struct {
	Scope     storage.Scope `json:"scope"`
	Room      string        `json:"room"`
	Identity  []byte        `json:"identity"`
	Recipient string        `json:"recipient"`
	Record    []byte        `json:"record"`
	Proof     []byte        `json:"proof"`
	Envelope  pqc.Envelope  `json:"envelope"`
}

// The configured bundle must be built from this snapshot's cmd/pqc-wasm on the
// remote test host. This checks real process/JS byte boundaries, not physical OS
// keychains, product activation or Matrix authority. All keys are disposable.
func TestPQCHybridNativeWasmInteroperabilityEveryProduct(t *testing.T) {
	bundle := os.Getenv("SYTUE_PQC_INTEROP_WASM")
	if bundle == "" {
		t.Skip("remote native/Wasm interoperability bundle required")
	}
	if !filepath.IsAbs(bundle) {
		t.Fatal("absolute remote bundle required")
	}
	if _, err := os.Stat(bundle); err != nil {
		t.Fatal("missing remote bundle")
	}
	dir := t.TempDir()
	fixtures := make([]platformInteropFixture, 0, len(storage.Products()))
	for _, product := range storage.Products() {
		scope := storage.Scope{Tenant: "artificial-interop", Subject: "fixture-owner", Product: product}
		identity, err := pqc.GenerateIdentity()
		if err != nil {
			t.Fatal(err)
		}
		record := storage.Record{Version: 1, Scope: scope, ObjectID: "object", RevisionID: "revision", Role: "content", Previous: []string{"$first", "$second"}, Payload: []byte(`{"integer":900719925474099312345,"unicode":"á漢🙂","spacing": [ 1, 2 ]}`)}
		proof := []byte(`{"artificial_owner_proof":"not-authority"}`)
		envelope, err := pqc.SealStorageRecord(identity.Recipient(), scope, "!private:matrix.test", record, proof)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		fixtures = append(fixtures, platformInteropFixture{Scope: scope, Room: "!private:matrix.test", Identity: []byte(identity.String()), Recipient: identity.Recipient().String(), Record: raw, Proof: proof, Envelope: envelope})
	}
	defer func() {
		for i := range fixtures {
			clear(fixtures[i].Identity)
			clear(fixtures[i].Record)
			clear(fixtures[i].Proof)
			clear(fixtures[i].Envelope.Ciphertext)
		}
	}()
	raw, err := json.Marshal(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	input, output := filepath.Join(dir, "artificial-input.json"), filepath.Join(dir, "output.json")
	if err := os.WriteFile(input, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeBytes, err := os.ReadFile(filepath.Join(runtime.GOROOT(), "lib", "wasm", "wasm_exec.js"))
	if err != nil {
		t.Fatal(err)
	}
	runtimeFile := filepath.Join(dir, "wasm_exec.cjs")
	if err := os.WriteFile(runtimeFile, runtimeBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("missing source path")
	}
	runner := filepath.Join(filepath.Dir(source), "..", "..", "..", "..", "test", "e2e", "pqc-native-wasm.cjs")
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", runner, bundle, runtimeFile, input, output) //nolint:gosec // Fixed repo runner; data and bundle are disposable remote test fixtures, never product input.
	cmd.Env = []string{"GOMAXPROCS=1"}
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Run(); err != nil {
		t.Fatal("native/Wasm process interoperability failed; child output suppressed", err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	native := exec.CommandContext(ctx, executable, "-test.run=^TestPQCHybridNativeWasmReturnProcess$", "-test.v") //nolint:gosec // This test's own compiled binary and fixed child test; artificial fixtures only.
	native.Env = []string{"SYTUE_PQC_INTEROP_INPUT=" + input, "SYTUE_PQC_INTEROP_OUTPUT=" + output, "GOMAXPROCS=1"}
	native.Stdout, native.Stderr = io.Discard, io.Discard
	if err := native.Run(); err != nil {
		t.Fatal("fresh native return process rejected Wasm ciphertext", err)
	}
	t.Log("same maintained Go hybrid profile: native → real Go/Wasm RPC → fresh native for every product; scope/key/room substitutions rejected")
}

// Separate OS process: no live native identity, decoder or Go heap crosses from
// the producer. Only disposable fixture bytes and returned ciphertext are read.
func TestPQCHybridNativeWasmReturnProcess(t *testing.T) {
	input, output := os.Getenv("SYTUE_PQC_INTEROP_INPUT"), os.Getenv("SYTUE_PQC_INTEROP_OUTPUT")
	if input == "" || output == "" {
		t.Skip("fresh native child only")
	}
	raw, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	var fixtures []platformInteropFixture
	if json.Unmarshal(raw, &fixtures) != nil || len(fixtures) != len(storage.Products()) {
		t.Fatal("incomplete artificial native fixtures")
	}
	defer func() {
		for i := range fixtures {
			clear(fixtures[i].Identity)
			clear(fixtures[i].Record)
			clear(fixtures[i].Proof)
			clear(fixtures[i].Envelope.Ciphertext)
		}
	}()
	result, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(result)
	var envelopes []pqc.Envelope
	if json.Unmarshal(result, &envelopes) != nil || len(envelopes) != len(fixtures) {
		t.Fatal("incomplete Wasm return path")
	}
	for i, envelope := range envelopes {
		fixture := fixtures[i]
		identity, err := pqc.ParseIdentity(string(fixture.Identity))
		if err != nil {
			t.Fatal(err)
		}
		actual, proof, err := pqc.OpenStorageRecord(identity, fixture.Scope, fixture.Room, envelope)
		if err != nil {
			t.Fatal("Wasm ciphertext cannot be opened natively", fixture.Scope.Product)
		}
		var original storage.Record
		if json.Unmarshal(fixture.Record, &original) != nil || !original.MatchesExact(actual) || !bytes.Equal(proof, fixture.Proof) {
			t.Fatal("Wasm changed original record or opaque proof", fixture.Scope.Product)
		}
		clear(actual.Payload)
		clear(original.Payload)
		clear(proof)
		clear(envelope.Ciphertext)
	}
}

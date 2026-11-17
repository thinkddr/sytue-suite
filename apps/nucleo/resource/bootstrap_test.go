// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"filippo.io/age"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

func TestBootstrapContextExactSequenceHybridDescriptorAndDomainBinding(t *testing.T) {
	f := authority(t, true)
	context, err := resource.NewBootstrapContext(resource.BootstrapBaseline, f.epoch.Value().Resource, f.epoch.Value().RoomID, f.epoch.Value().ID, f.epoch.Value().Recipient, resource.Position{Seq: "9223372036854775807"}, f.scope, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	if context.SourceCutover.Valid() {
		t.Fatal("pre-cipher baseline exception escaped into a general position")
	}
	raw, err := resource.BootstrapContextBytes(context)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"seq":"9223372036854775807"`)) {
		t.Fatal("canonical context lost full SQL int64")
	}
	sum := sha256.Sum256(raw)
	hash, err := resource.BootstrapContextSHA256(context)
	if err != nil || hash != hex.EncodeToString(sum[:]) {
		t.Fatal("context evidence is not exact canonical bytes", err)
	}
	payload := []byte(`{"descriptor":{"cipher_file":"opaque-resource-file","key":"PRIVATE-DESCRIPTOR-KEY"},"future":18446744073709551615}`)
	sealed, err := resource.SealBootstrapDescriptor(f.epochIdentity.Recipient(), context, payload)
	if err != nil || sealed.Profile != pqc.HybridProfile {
		t.Fatal("hybrid seal", err)
	}
	opened, err := resource.OpenBootstrapDescriptor(f.epochIdentity, context, sealed)
	if err != nil || !bytes.Equal(opened, payload) {
		t.Fatal("same Go hybrid descriptor roundtrip", err)
	}
	clear(opened)
	wire, _ := json.Marshal(sealed)
	if bytes.Contains(wire, []byte("PRIVATE-DESCRIPTOR-KEY")) || bytes.Contains(raw, []byte("PRIVATE-DESCRIPTOR-KEY")) {
		t.Fatal("private descriptor leaked into public transcript")
	}
	for _, fault := range []string{"resource", "epoch", "source", "creator", "snapshot"} {
		t.Run(fault, func(t *testing.T) {
			changed := context
			switch fault {
			case "resource":
				changed.Resource.ID = "another"
			case "epoch":
				changed.Epoch = "epoch-other"
			case "source":
				changed.SourceCutover.Seq = "9223372036854775806"
			case "creator":
				changed.CreatorScope.Subject = "another"
			case "snapshot":
				changed.SnapshotKind = resource.SnapshotCurrentState
			}
			if _, err := resource.OpenBootstrapDescriptor(f.epochIdentity, changed, sealed); err == nil {
				t.Fatal("substituted bootstrap context opened")
			}
		})
	}
	legacy, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = resource.SealBootstrapDescriptor(legacy.Recipient(), context, payload); err == nil {
		t.Fatal("new bootstrap used classical-only recipient")
	}
	if _, err = resource.SealBootstrapDescriptor(nil, context, payload); err == nil {
		t.Fatal("nil recipient accepted")
	}
	if _, err = resource.SealBootstrapDescriptor(f.epochIdentity.Recipient(), context, make([]byte, resource.MaxBootstrapDescriptorBytes+1)); err == nil {
		t.Fatal("whole archive permitted in a descriptor event")
	}
}

func TestBootstrapInitialAnchorsZeroOpsAndIndependentCheckpoint(t *testing.T) {
	f := authority(t, true)
	epoch := f.epoch.Value()
	if err := resource.ValidateInitialBaseline(epoch, f.anchor); err != nil {
		t.Fatal("initial binding", err)
	}
	if epoch.Baseline.Reference == epoch.Checkpoint.Reference {
		t.Fatal("history archive reused as current state")
	}
	// A zero-op document still has title/comments/metadata that need encryption.
	zero := epoch
	zero.Baseline.Position.Seq = "0"
	zero.Cutover = zero.Baseline.Position
	zero.Checkpoint.SourceCutover = zero.Cutover
	b, err := resource.NewBootstrapContext(resource.BootstrapBaseline, zero.Resource, zero.RoomID, zero.ID, zero.Recipient, resource.Position{Seq: "0"}, f.scope, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	zero.Baseline.ContextSHA256, _ = resource.BootstrapContextSHA256(b)
	c, err := resource.NewBootstrapContext(resource.BootstrapCheckpoint, zero.Resource, zero.RoomID, zero.ID, zero.Recipient, zero.Cutover, f.scope, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	zero.Checkpoint.ContextSHA256, _ = resource.BootstrapContextSHA256(c)
	anchor := f.anchor
	anchor.Baseline = zero.Cutover
	anchor.PolicySHA256, _ = resource.EpochDigest(zero)
	if !zero.Cutover.Valid() || resource.ValidateInitialBaseline(zero, anchor) != nil {
		t.Fatal("zero-op encrypted state cannot be recovered")
	}
	for _, fault := range []string{"context", "checkpoint-context", "room", "same-reference", "baseline-seq", "source-head"} {
		t.Run(fault, func(t *testing.T) {
			changed := epoch
			switch fault {
			case "context":
				changed.Baseline.ContextSHA256 = strings.Repeat("9", 64)
			case "checkpoint-context":
				changed.Checkpoint.ContextSHA256 = strings.Repeat("9", 64)
			case "room":
				changed.Checkpoint.Reference.RoomID = "!another:matrix.test"
			case "same-reference":
				changed.Checkpoint.Reference = changed.Baseline.Reference
			case "baseline-seq":
				changed.Baseline.Position.Seq = "1"
			case "source-head":
				changed.Checkpoint.SourceCutover.HeadCipherSHA256 = strings.Repeat("9", 64)
			}
			if err := resource.ValidateInitialBaseline(changed, f.anchor); err == nil {
				t.Fatal("substituted initial anchor accepted")
			}
		})
	}
}

func TestBootstrapNextEpochPreservesHistoryAndRequiresFreshKeyAndSnapshot(t *testing.T) {
	f := authority(t, true)
	previous := f.epoch.Value()
	manager, err := resource.Authorize(f.epoch, f.cap, f.scope.Subject, resource.Manage)
	if err != nil {
		t.Fatal(err)
	}
	nextKey, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	next := previous
	next.ID = "epoch2"
	next.Generation = "2"
	next.Recipient = nextKey.Recipient().String()
	next.PreviousEpoch = previous.ID
	next.PreviousPolicySHA256 = f.epoch.SHA256()
	next.Cutover = resource.Position{Seq: "9007199254740999", HeadCipherSHA256: strings.Repeat("7", 64)}
	context, err := resource.PrepareCheckpointContext(manager, next.ID, next.Recipient, next.Cutover)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := resource.BootstrapContextSHA256(context)
	next.Checkpoint = resource.Checkpoint{SourceCutover: next.Cutover, Reference: storage.Reference{RoomID: next.RoomID, EventID: "$checkpoint2"}, CipherSHA256: strings.Repeat("8", 64), ContextSHA256: hash, SessionCertificateID: "checkpoint2-certificate"}
	if err = resource.ValidateBaselineTransition(previous, next); err != nil {
		t.Fatal("valid new epoch", err)
	}
	if err = resource.ValidateCheckpointContext(next, f.scope, f.actor); err != nil {
		t.Fatal("manager scope binding", err)
	}
	if next.Cutover.HeadCipherSHA256 == next.Checkpoint.CipherSHA256 {
		t.Fatal("checkpoint replaced logical operation head")
	}
	if _, err = resource.PrepareCheckpointContext(resource.VerifiedAccess{}, next.ID, next.Recipient, next.Cutover); err == nil {
		t.Fatal("raw JSON became a verified manager")
	}
	commenter, err := resource.Authorize(f.epoch, f.cap, f.scope.Subject, resource.Comment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = resource.PrepareCheckpointContext(commenter, next.ID, next.Recipient, next.Cutover); err == nil {
		t.Fatal("comment action became manager authority")
	}
	for _, fault := range []string{"same-recipient", "baseline", "same-checkpoint", "cutover", "creator"} {
		t.Run(fault, func(t *testing.T) {
			changed := next
			switch fault {
			case "same-recipient":
				changed.Recipient = previous.Recipient
			case "baseline":
				changed.Baseline.Reference.EventID = "$changed-history"
			case "same-checkpoint":
				changed.Checkpoint.Reference = previous.Checkpoint.Reference
			case "cutover":
				changed.Checkpoint.SourceCutover = previous.Cutover
			case "creator":
				changed.Issuer.DeviceID = "OTHER_DEVICE"
			}
			// Even an otherwise valid original SDK manager signature is insufficient
			// to reuse a recipient on rotation or substitute the immutable history.
			signed := resource.SignedEpoch{Epoch: changed}
			signed.Signatures = matrixprooftest.Sign(t, f.chain.Device, signed, f.chain.Origin.UserID, f.actor.DeviceID)
			if err := f.device.VerifyJSON(t.Context(), f.scope, storage.PublicationTarget{RoomID: previous.RoomID, DeviceID: f.actor.DeviceID}, signed); err != nil {
				t.Fatal("fixture manager did not sign", err)
			}
			if fault == "creator" {
				if err := resource.ValidateCheckpointContext(changed, f.scope, f.actor); err == nil {
					t.Fatal("checkpoint creator substitution accepted")
				}
			} else if err := resource.ValidateBaselineTransition(previous, changed); err == nil {
				t.Fatal("manager signature bypassed immutable/fresh-key binding")
			}
		})
	}
}

func TestBootstrapStrictContextDecoder(t *testing.T) {
	f := authority(t, true)
	value, err := resource.NewBootstrapContext(resource.BootstrapBaseline, f.epoch.Value().Resource, f.epoch.Value().RoomID, f.epoch.Value().ID, f.epoch.Value().Recipient, resource.Position{Seq: "9007199254740993"}, f.scope, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := resource.BootstrapContextBytes(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := resource.DecodeBootstrapContext(raw)
	if err != nil || decoded != value {
		t.Fatal("strict canonical context roundtrip", err)
	}
	numeric := bytes.Replace(raw, []byte(`"seq":"9007199254740993"`), []byte(`"seq":9007199254740993`), 1)
	unknown := append([]byte(`{"unknown":true,`), raw[1:]...)
	for _, invalid := range [][]byte{numeric, unknown, append(append([]byte(nil), raw...), []byte(" {}")...)} {
		if _, err := resource.DecodeBootstrapContext(invalid); err == nil {
			t.Fatal("ambiguous public routing was accepted")
		}
	}
}

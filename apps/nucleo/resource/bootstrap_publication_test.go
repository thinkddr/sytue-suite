// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

// Public verifier fixture: SDK signatures are real; ciphertext here is opaque
// synthetic Megolm wire. Actual crypto transport runs in the Synapse fixture.
func bootstrapCandidateFixture(t testing.TB, f bootstrapPlanCase, kind resource.BootstrapKind, baseline *resource.VerifiedBootstrapAcceptance) resource.BootstrapCandidate {
	return f.bootstrap.OpaqueCandidate(t, kind, baseline)
}

func bootstrapAcceptanceFixture(candidate resource.BootstrapCandidate, reference storage.Reference) resource.AcceptanceSource {
	return acceptanceFixture(func(ref storage.Reference) (*event.Event, error) {
		if ref != reference {
			return nil, resource.ErrInvalid
		}
		return &event.Event{ID: id.EventID(ref.EventID), RoomID: id.RoomID(ref.RoomID), Sender: id.UserID(candidate.Proof.Actor.UserID), Type: event.EventEncrypted, Content: event.Content{VeryRaw: candidate.EncryptedEvent}}, nil
	})
}

func TestResourceBootstrapPublicationAndActivationRequireBothExactSDKACKs(t *testing.T) {
	fixture := prepareBootstrapPlanFixture(t)
	baseline := bootstrapCandidateFixture(t, fixture, resource.BootstrapBaseline, nil)
	verified, err := resource.VerifyBootstrapCandidate(t.Context(), baseline, fixture.verified, nil)
	if err != nil {
		t.Fatal("baseline", err)
	}
	if _, err = fixture.verified.CheckpointContext(resource.VerifiedBootstrapAcceptance{}); err == nil {
		t.Fatal("checkpoint invented missing baseline ACK")
	}
	ref := storage.Reference{RoomID: fixture.plan.RoomID, EventID: "$baseline-accepted"}
	accepted, err := resource.ConfirmBootstrapAcceptance(t.Context(), verified, ref, bootstrapAcceptanceFixture(baseline, ref))
	if err != nil {
		t.Fatal(err)
	}
	cp := bootstrapCandidateFixture(t, fixture, resource.BootstrapCheckpoint, &accepted)
	cpVerified, err := resource.VerifyBootstrapCandidate(t.Context(), cp, fixture.verified, &accepted)
	if err != nil {
		t.Fatal("checkpoint", err)
	}
	cpRef := storage.Reference{RoomID: fixture.plan.RoomID, EventID: "$checkpoint-accepted"}
	cpAccepted, err := resource.ConfirmBootstrapAcceptance(t.Context(), cpVerified, cpRef, bootstrapAcceptanceFixture(cp, cpRef))
	if err != nil {
		t.Fatal(err)
	}
	b, err := accepted.Baseline()
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := cpAccepted.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	epoch := resource.Epoch{Version: 1, Domain: "org.sytue.resource.epoch.v1", Resource: fixture.plan.Source.Resource, RoomID: fixture.plan.RoomID, ID: fixture.plan.Epoch, Generation: "1", Recipient: fixture.plan.Recipient, MembershipSHA256: fixture.plan.MembershipSHA256, History: fixture.plan.History, Cutover: b.Position, Baseline: b, Checkpoint: checkpoint, Validation: fixture.plan.Validation, Issuer: fixture.plan.Creator}
	sign := func(value resource.Epoch) resource.SignedEpoch {
		return resource.SignedEpoch{Epoch: value, Signatures: matrixprooftest.Sign(t, fixture.f.Chain.Device, value, fixture.f.Chain.Origin.UserID, fixture.f.Actor.DeviceID)}
	}
	activation, err := resource.VerifyBootstrapActivation(t.Context(), sign(epoch), fixture.verified, accepted, cpAccepted)
	if err != nil || activation.Epoch().Value() != epoch || activation.Anchor().Baseline.Seq != "9007199254740993" || activation.PlanSHA256() != fixture.verified.SHA256() {
		t.Fatal("exact final activation", err)
	}
	for _, fault := range []string{"baseline", "checkpoint", "recipient", "history", "source-seq", "issuer", "membership", "signature"} {
		t.Run(fault, func(t *testing.T) {
			changed := epoch
			switch fault {
			case "baseline":
				changed.Baseline.Reference.EventID = "$other"
			case "checkpoint":
				changed.Checkpoint.CipherSHA256 = fixture.f.Epoch.SHA256()
			case "recipient":
				changed.Recipient = fixture.f.RecipientIdentity.Recipient().String()
			case "history":
				changed.History = resource.HistoryJoined
			case "source-seq":
				changed.Cutover.Seq = "1"
			case "issuer":
				changed.Issuer.DeviceID = "OTHER"
			case "membership":
				changed.MembershipSHA256 = fixture.f.Epoch.SHA256()
			}
			candidate := sign(changed)
			if fault == "signature" {
				candidate.Signatures = nil
			}
			if _, err := resource.VerifyBootstrapActivation(t.Context(), candidate, fixture.verified, accepted, cpAccepted); err == nil {
				t.Fatal("final epoch substituted plan/ACK", fault)
			}
		})
	}
	if _, err = resource.VerifyBootstrapActivation(t.Context(), sign(epoch), fixture.verified, accepted, resource.VerifiedBootstrapAcceptance{}); err == nil {
		t.Fatal("missing checkpoint became active epoch")
	}
	fixture.source.withdrawAt = fixture.source.checks + 1
	if err = activation.Revalidate(t.Context()); err == nil {
		t.Fatal("activation lost original source fence")
	}
}

func TestResourceBootstrapPublicationRejectsForgedPreEpochClaims(t *testing.T) {
	f := prepareBootstrapPlanFixture(t)
	baseline := bootstrapCandidateFixture(t, f, resource.BootstrapBaseline, nil)
	for _, fault := range []string{"plan", "context", "actor", "certificate", "cipher", "signature", "checkpoint-before-baseline"} {
		t.Run(fault, func(t *testing.T) {
			raw, _ := json.Marshal(baseline)
			var candidate resource.BootstrapCandidate
			if json.Unmarshal(raw, &candidate) != nil {
				t.Fatal("fixture clone")
			}
			switch fault {
			case "plan":
				candidate.Proof.PlanSHA256 = f.f.Epoch.SHA256()
			case "context":
				candidate.Proof.ContextSHA256 = f.f.Epoch.SHA256()
			case "actor":
				candidate.Proof.Actor.DeviceID = "OTHER"
			case "certificate":
				candidate.SessionCertificate = []byte("{}")
			case "cipher":
				candidate.EncryptedEvent = f.f.Candidate.EncryptedEvent
			case "checkpoint-before-baseline":
				candidate.Proof.Kind = resource.BootstrapCheckpoint
			}
			candidate.Proof.Signatures = matrixprooftest.Sign(t, f.f.Chain.Device, candidate.Proof.BootstrapPublication, f.f.Chain.Origin.UserID, f.f.Actor.DeviceID)
			if fault == "signature" {
				candidate.Proof.Signatures = nil
			}
			if _, err := resource.VerifyBootstrapCandidate(t.Context(), candidate, f.verified, nil); err == nil {
				t.Fatal("forged bootstrap accepted", fault)
			}
		})
	}
	verified, err := resource.VerifyBootstrapCandidate(t.Context(), baseline, f.verified, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref := storage.Reference{RoomID: f.plan.RoomID, EventID: "$original"}
	wrong := ref
	wrong.EventID = "$wrong"
	if _, err = resource.ConfirmBootstrapAcceptance(t.Context(), verified, wrong, bootstrapAcceptanceFixture(baseline, ref)); err == nil {
		t.Fatal("unfetched SDK ref accepted")
	}
	replaced := baseline
	replaced.EncryptedEvent = f.f.Candidate.EncryptedEvent
	if _, err = resource.ConfirmBootstrapAcceptance(t.Context(), verified, ref, bootstrapAcceptanceFixture(replaced, ref)); err == nil {
		t.Fatal("replaced ciphertext ACK accepted")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = resource.ConfirmBootstrapAcceptance(cancelled, verified, ref, bootstrapAcceptanceFixture(baseline, ref)); err == nil {
		t.Fatal("cancelled publication ACK accepted")
	}
}

func TestResourceBootstrapCaptureRetainsOnlyPublicOriginalEvidence(t *testing.T) {
	fixture := prepareBootstrapPlanFixture(t)
	capture, err := resource.CaptureBootstrapPlan(t.Context(), fixture.verified)
	if err != nil || !capture.Valid() || capture.SHA256() != fixture.verified.SHA256() {
		t.Fatal("capture", err)
	}
	for _, pair := range []struct {
		raw  []byte
		hash string
	}{{capture.Bytes(), capture.SHA256()}, {capture.EvidenceBytes(), capture.EvidenceSHA256()}} {
		hash := sha256.Sum256(pair.raw)
		if hex.EncodeToString(hash[:]) != pair.hash {
			t.Fatal("SQL evidence bytehash mismatch")
		}
	}
	var session resource.SessionRecord
	if json.Unmarshal(fixture.current.PublicRecordBytes(), &session) != nil {
		t.Fatal("fixture session")
	}
	for _, record := range capture.Records() {
		hash := sha256.Sum256(record.Value)
		if hex.EncodeToString(hash[:]) != record.SHA256 {
			t.Fatal("public record hash mismatch")
		}
		for _, private := range []string{fixture.f.RecipientIdentity.String(), fixture.f.EpochIdentity.String(), session.Proof.Nonce} {
			if private != "" && bytes.Contains(record.Value, []byte(private)) {
				t.Fatal("private key or consumed enrollment nonce leaked into resource evidence")
			}
		}
	}
	raw := capture.EvidenceBytes()
	raw[0] = 0
	if bytes.Equal(raw, capture.EvidenceBytes()) {
		t.Fatal("capture exposed mutable descriptor")
	}
	fixture.registry.Retire()
	if _, err = resource.CaptureBootstrapPlan(t.Context(), fixture.verified); err == nil {
		t.Fatal("retired device captured new plan")
	}
}

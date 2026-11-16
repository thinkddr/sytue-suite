// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// Shared metadata witness; original SQL/RLS is exercised in the PG integration.
type committedFixture = resourcetest.CommittedSource

func committedCapture(t testing.TB, capture resource.CapturedReservation, candidate resource.Candidate, evidence *heldFixture) *committedFixture {
	t.Helper()
	evidence.Records = map[string]json.RawMessage{}
	for _, hash := range capture.EvidenceHashes() {
		record, err := capture.Evidence(hash)
		if err != nil {
			t.Fatal(err)
		}
		evidence.Records[hash] = record.Value
	}
	source := resourcetest.NewCommittedSource(t, capture, candidate)
	source.Evidence = evidence
	return source
}

func readerSession(t testing.TB, f resourcetest.Fixture) (resource.VerifiedAccess, *resourcetest.SessionSource) {
	t.Helper()
	chain := matrixprooftest.NewDevice(t, f.Chain, id.DeviceID("CURRENT_READER_"+uuid.NewString()))
	key, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	current, source := resourcetest.EnrolledSession(t, chain, key)
	reader, err := resource.AuthorizeSession(t.Context(), f.Epoch, f.Cap, current, resource.Read)
	if err != nil {
		t.Fatal(err)
	}
	return reader, source
}

func TestResourceCommittedReadUsesCurrentBAndOnlyCapturedProducerA(t *testing.T) {
	for _, privateRules := range []bool{false, true} {
		t.Run(map[bool]string{false: "capability", true: "validated"}[privateRules], func(t *testing.T) {
			f, held, capture := prepareHeld(t, privateRules)
			source := committedCapture(t, capture, held.Held.Candidate, held)
			reader, current := readerSession(t, f)
			f.Chain.Origin.DeviceActive = false
			if _, err := matrixproof.VerifyUser(t.Context(), f.Chain.Origin, f.Chain.Origin.UserID, f.Chain.Master.PublicKey()); err == nil {
				t.Fatal("A regained a live Device")
			}
			proof, err := resource.VerifyCommittedRead(t.Context(), source.Stored.Resource, source.Stored.OpID, reader, resourceFence(activeResourceFence), source)
			if err != nil {
				t.Fatal(err)
			}
			if proof.Value().Actor != f.Actor || proof.Receipt() != source.Stored.Receipt || proof.CandidateSHA256() != capture.Value().CandidateSHA256 || proof.SHA256() != capture.Value().PublicationSHA256 || source.Checks != 1 || held.Gets == 0 {
				t.Fatal("historical receipt or public SDK chain was replaced")
			}
			if err := proof.RevalidateReader(t.Context(), reader, resourceFence(activeResourceFence)); err != nil {
				t.Fatal(err)
			}
			otherReader, _ := readerSession(t, f)
			if err := proof.RevalidateReader(t.Context(), otherReader, resourceFence(activeResourceFence)); err == nil {
				t.Fatal("another enrolled physical reader adopted the captured proof")
			}
			raw, _ := json.Marshal(proof) //nolint:staticcheck // SA9005: intentionally testing opaque struct marshaling
			var empty resource.VerifiedCommittedRead
			if json.Unmarshal(raw, &empty) != nil { //nolint:staticcheck // SA9005: intentionally testing opaque struct unmarshaling
				t.Fatal("opaque proof JSON")
			}
			if err := empty.RevalidateReader(t.Context(), reader, resourceFence(activeResourceFence)); err == nil {
				t.Fatal("JSON minted committed read authority")
			}
			current.Retire()
			if err := proof.RevalidateReader(t.Context(), reader, resourceFence(activeResourceFence)); !errors.Is(err, resource.ErrRevoked) {
				t.Fatal("retired B retained a read", err)
			}
		})
	}
}

func TestResourceCommittedReadRejectsNonCommittedOrChangedIndependentBindings(t *testing.T) {
	f, held, capture := prepareHeld(t, true)
	baseline := committedCapture(t, capture, held.Held.Candidate, held)
	reader, _ := readerSession(t, f)
	cases := map[string]func(*resource.CommittedPublication){
		"sending":          func(v *resource.CommittedPublication) { v.State = resource.Sending },
		"ambiguous":        func(v *resource.CommittedPublication) { v.State = resource.Ambiguous },
		"prepared":         func(v *resource.CommittedPublication) { v.State = resource.Prepared },
		"cancelled":        func(v *resource.CommittedPublication) { v.State = resource.Cancelled },
		"foreign-resource": func(v *resource.CommittedPublication) { v.Resource.ID = "other" },
		"attempt":          func(v *resource.CommittedPublication) { v.AttemptID = "00000000-0000-4000-8000-000000000099" },
		"generation":       func(v *resource.CommittedPublication) { v.Generation = "2" },
		"candidate":        func(v *resource.CommittedPublication) { v.Candidate.Proof.OpID = "other" },
		"receipt-room":     func(v *resource.CommittedPublication) { v.Receipt.Reference.RoomID = "!other:matrix.test" },
		"receipt-event":    func(v *resource.CommittedPublication) { v.Receipt.Reference.EventID = "" },
		"receipt-op":       func(v *resource.CommittedPublication) { v.Receipt.OpID = "other" },
		"receipt-hash":     func(v *resource.CommittedPublication) { v.Receipt.PublicationSHA256 = v.CandidateSHA256 },
		"receipt-base":     func(v *resource.CommittedPublication) { v.Receipt.Base.Seq = "0" },
		"receipt-end":      func(v *resource.CommittedPublication) { v.Receipt.Head.Seq = "9007199254740994" },
		"receipt-cipher":   func(v *resource.CommittedPublication) { v.Receipt.Head.HeadCipherSHA256 = v.CandidateSHA256 },
		"historical-cert":  func(v *resource.CommittedPublication) { v.Candidate.SessionCertificate = json.RawMessage(`{}`) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(baseline.Stored)
			var changed resource.CommittedPublication
			if json.Unmarshal(raw, &changed) != nil {
				t.Fatal("clone")
			}
			change(&changed)
			source := &committedFixture{Stored: changed, Evidence: held}
			if _, err := resource.VerifyCommittedRead(t.Context(), baseline.Stored.Resource, baseline.Stored.OpID, reader, resourceFence(activeResourceFence), source); err == nil {
				t.Fatal("changed committed transcript accepted")
			}
		})
	}
	source := *baseline
	source.CheckError = resource.ErrFork
	if _, err := resource.VerifyCommittedRead(t.Context(), source.Stored.Resource, source.Stored.OpID, reader, resourceFence(activeResourceFence), &source); !errors.Is(err, resource.ErrFork) {
		t.Fatal("late original CAS mismatch ignored", err)
	}
	rawReader, err := resource.Authorize(f.Epoch, f.Cap, f.Scope.Subject, resource.Read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resource.VerifyCommittedRead(t.Context(), source.Stored.Resource, source.Stored.OpID, rawReader, resourceFence(activeResourceFence), baseline); err == nil {
		t.Fatal("historical role metadata substituted for current B session")
	}
	writer, err := resource.Authorize(f.Epoch, f.Cap, f.Scope.Subject, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resource.VerifyCommittedRead(t.Context(), source.Stored.Resource, source.Stored.OpID, writer, resourceFence(activeResourceFence), baseline); err == nil {
		t.Fatal("Publish substituted for Read")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := resource.VerifyCommittedRead(ctx, source.Stored.Resource, source.Stored.OpID, reader, resourceFence(activeResourceFence), baseline); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled reader returned a proof", err)
	}
}

func TestResourceCommittedReadOpensMaintainedHybridAfterProducerRetirement(t *testing.T) {
	f, held, _ := prepareHeld(t, false)
	window, err := resource.CurrentEpoch(f.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := resource.BindOwnedEpoch(window, f.EpochIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Close()
	writer, err := resource.Authorize(f.Epoch, f.Cap, f.Scope.Subject, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	record := storage.Record{Version: 1, Scope: f.Scope, ObjectID: "object", RevisionID: "revision", Role: "content", Payload: []byte(`{"seq":9007199254740993,"extensions":{"keep":900719925474099312345}}`)}
	payload, err := resource.EncodeRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := owned.SealFrame(t.Context(), f.Candidate.Proof.Publication, payload, writer, resourceFence(activeResourceFence))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	inner, err := resource.CipherJSONDigest(encoded)
	if err != nil {
		t.Fatal(err)
	}
	candidate := f.Candidate
	candidate.Proof.InnerCipherSHA256 = inner
	candidate.Proof.Signatures = matrixprooftest.Sign(t, f.Chain.Device, candidate.Proof, f.Chain.Origin.UserID, f.Actor.DeviceID)
	full, err := resource.VerifyCandidate(t.Context(), candidate, writer, f.Device, nil)
	if err != nil {
		t.Fatal(err)
	}
	capture, err := resource.CaptureReservation(t.Context(), candidate, full, candidate.Proof.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	source := committedCapture(t, capture, candidate, held)
	reader, current := readerSession(t, f)
	f.Chain.Origin.DeviceActive = false
	proof, err := resource.VerifyCommittedRead(t.Context(), source.Stored.Resource, source.Stored.OpID, reader, resourceFence(activeResourceFence), source)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := owned.OpenCommittedFrame(t.Context(), proof, envelope, reader, resourceFence(activeResourceFence))
	if err != nil || !bytes.Equal(actual, payload) {
		t.Fatal("maintained hybrid changed committed original bytes", err)
	}
	clear(actual)
	if proof.ProducerScope() != f.Scope {
		t.Fatal("historical producer scope replaced by reader or JSON")
	}
	sdkCalls := 0
	sdk := committedRecordSDK(func(ctx context.Context, p resource.VerifiedCommittedRead) ([]byte, error) {
		sdkCalls++
		return owned.OpenCommittedFrame(ctx, p, envelope, reader, resourceFence(activeResourceFence))
	})
	decoded, err := resource.ReadCommittedRecord(t.Context(), sdk, proof, reader, resourceFence(activeResourceFence), "object", "revision", "content")
	if err != nil || !record.MatchesExact(decoded) {
		t.Fatal("same private record lost authenticated original context or precise bytes", err)
	}
	clear(decoded.Payload)
	for _, target := range [][3]string{{"other", "revision", "content"}, {"object", "other", "content"}, {"object", "revision", "manifest"}} {
		if out, err := resource.ReadCommittedRecord(t.Context(), sdk, proof, reader, resourceFence(activeResourceFence), target[0], target[1], target[2]); err == nil || len(out.Payload) != 0 {
			t.Fatal("foreign record context released plaintext", target)
		}
	}
	before := sdkCalls
	if out, err := resource.ReadCommittedRecord(t.Context(), sdk, resource.VerifiedCommittedRead{}, reader, resourceFence(activeResourceFence), "object", "revision", "content"); err == nil || len(out.Payload) != 0 || sdkCalls != before {
		t.Fatal("JSON/empty read proof reached SDK record")
	}
	envelope.Ciphertext[0] ^= 1
	if plain, err := owned.OpenCommittedFrame(t.Context(), proof, envelope, reader, resourceFence(activeResourceFence)); err == nil || plain != nil {
		t.Fatal("changed inner cipher released plaintext")
	}
	envelope.Ciphertext[0] ^= 1
	late := committedRecordSDK(func(ctx context.Context, p resource.VerifiedCommittedRead) ([]byte, error) {
		raw, err := sdk.ReadCommitted(ctx, p)
		current.Retire()
		return raw, err
	})
	if out, err := resource.ReadCommittedRecord(t.Context(), late, proof, reader, resourceFence(activeResourceFence), "object", "revision", "content"); !errors.Is(err, resource.ErrRevoked) || len(out.Payload) != 0 {
		t.Fatal("late retired reader released private record", err)
	}
	if plain, err := owned.OpenCommittedFrame(t.Context(), proof, envelope, reader, resourceFence(activeResourceFence)); !errors.Is(err, resource.ErrRevoked) || plain != nil {
		t.Fatal("retired B opened committed bytes", err)
	}
}

func TestResourceCommittedReadAnchorsHistoricalEpochToCurrentSignedSuccessor(t *testing.T) {
	f, held, capture := prepareHeld(t, false)
	source := committedCapture(t, capture, held.Held.Candidate, held)
	_, directory := readerSession(t, f)
	provider, err := resource.NewSessionProvider(directory, directory)
	if err != nil {
		t.Fatal(err)
	}
	current, err := provider.Resolve(t.Context(), f.Scope, directory.Record.Binding.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	original := f.SignedEpoch
	next, nextCap, _ := rotateSessionEpoch(t, f.Epoch, f.Cap, original, current, directory, source.Stored.Receipt.Head)
	reader, err := resource.AuthorizeSession(t.Context(), next, nextCap, current, resource.Read)
	if err != nil {
		t.Fatal(err)
	}
	f.Chain.Origin.DeviceActive = false
	proof, err := resource.VerifyCommittedRead(t.Context(), source.Stored.Resource, source.Stored.OpID, reader, resourceFence(activeResourceFence), source)
	if err != nil || proof.Epoch().SHA256() != f.Epoch.SHA256() || proof.Receipt() != source.Stored.Receipt {
		t.Fatal("current B lost the anchored historical epoch", err)
	}
	unrelated := resourcetest.New(t, false)
	foreignReader, _ := readerSession(t, unrelated)
	if _, err := resource.VerifyCommittedRead(t.Context(), source.Stored.Resource, source.Stored.OpID, foreignReader, resourceFence(activeResourceFence), source); err == nil {
		t.Fatal("a captured historical pin replaced the independent current policy chain")
	}
}

// The body is opened through maintained hybrid crypto in these shared-core tests.
// Actual Matrix GET/Megolm is exercised separately in the native integration.
type committedRecordSDK func(context.Context, resource.VerifiedCommittedRead) ([]byte, error)

func (f committedRecordSDK) ReadCommitted(ctx context.Context, p resource.VerifiedCommittedRead) ([]byte, error) {
	return f(ctx, p)
}

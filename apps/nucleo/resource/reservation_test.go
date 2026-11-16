// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

type heldFixture struct {
	Held    resource.HeldReservation
	Records map[string]json.RawMessage
	Gets    int
}

func (s *heldFixture) HeldReservation(_ context.Context, r resource.Resource, opID string) (resource.HeldReservation, error) {
	if s.Held.Resource != r || s.Held.OpID != opID {
		return resource.HeldReservation{}, resource.ErrInvalid
	}
	return s.Held, nil
}

func (s *heldFixture) PublicEvidence(_ context.Context, r resource.Resource, hash string) (json.RawMessage, error) {
	s.Gets++
	if s.Held.Resource != r {
		return nil, resource.ErrInvalid
	}
	raw, ok := s.Records[hash]
	if !ok {
		return nil, resource.ErrInvalid
	}
	return bytes.Clone(raw), nil
}

func cloneHeld(t *testing.T, source *heldFixture) *heldFixture {
	t.Helper()
	raw, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var copied heldFixture
	if json.Unmarshal(raw, &copied) != nil {
		t.Fatal("held fixture could not survive restart")
	}
	return &copied
}

func rewriteDescriptor(t *testing.T, source *heldFixture, change func(*resource.Reservation)) {
	t.Helper()
	var descriptor resource.Reservation
	if json.Unmarshal(source.Held.Descriptor, &descriptor) != nil {
		t.Fatal("descriptor fixture invalid")
	}
	change(&descriptor)
	raw, err := matrixproof.CanonicalBytes(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := resource.CipherJSONDigest(raw)
	if err != nil {
		t.Fatal(err)
	}
	source.Held.Descriptor = raw
	source.Held.DescriptorSHA256 = hash
}

func prepareHeld(t *testing.T, privateRules bool) (resourcetest.Fixture, *heldFixture, resource.CapturedReservation) {
	t.Helper()
	f := resourcetest.New(t, privateRules)
	candidate := f.Candidate
	writer, err := resource.Authorize(f.Epoch, f.Cap, f.Scope.Subject, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	var validator *resource.ValidationEvidence
	if privateRules {
		pending, err := resource.VerifyProducerCandidate(t.Context(), candidate, writer, f.Device)
		if err != nil {
			t.Fatal(err)
		}
		validate, err := resource.Authorize(f.Epoch, f.Cap, f.Scope.Subject, resource.Validate)
		if err != nil {
			t.Fatal(err)
		}
		cert := &resource.ValidationCertificate{Version: 1, Domain: "org.sytue.resource.validation.v1", PublicationSHA256: pending.SHA256(), ValidatorCapabilitySHA256: f.Cap.SHA256(), RulesVersion: f.Epoch.Value().Validation.RulesVersion, Allow: true, Issuer: f.Actor}
		cert.Signatures = matrixprooftest.Sign(t, f.Chain.Device, cert, f.Chain.Origin.UserID, f.Actor.DeviceID)
		candidate.Validation = cert
		validator = &resource.ValidationEvidence{Access: validate, Device: f.Device}
	}
	verified, err := resource.VerifyCandidate(t.Context(), candidate, writer, f.Device, validator)
	if err != nil {
		t.Fatal(err)
	}
	capture, err := resource.CaptureReservation(t.Context(), candidate, verified, candidate.Proof.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	value := capture.Value()
	source := &heldFixture{Held: resource.HeldReservation{State: resource.Sending, Resource: value.Resource, OpID: value.OpID, AttemptID: value.AttemptID, Epoch: value.Epoch, Generation: value.Generation, PolicySHA256: value.PolicySHA256, CandidateSHA256: value.CandidateSHA256, PublicationSHA256: value.PublicationSHA256, Base: value.Base, End: value.End, DescriptorSHA256: capture.SHA256(), Descriptor: capture.Bytes(), Candidate: candidate}, Records: map[string]json.RawMessage{}}
	for _, hash := range capture.EvidenceHashes() {
		record, err := capture.Evidence(hash)
		if err != nil {
			t.Fatal(err)
		}
		actual := sha256.Sum256(record.Value)
		if hex.EncodeToString(actual[:]) != hash {
			t.Fatal("public evidence hash does not match exact PostgreSQL bytes")
		}
		if bytes.Contains(record.Value, []byte(f.RecipientIdentity.String())) || bytes.Contains(record.Value, []byte(f.EpochIdentity.String())) {
			t.Fatal("public evidence exposed a private hybrid identity")
		}
		source.Records[hash] = record.Value
	}
	if !capture.Valid() {
		t.Fatal("verified capture lost its immutable descriptor")
	}
	return f, source, capture
}

func TestResourceReservationRestartAfterProducerRevocationOnlyAcknowledgesExactHold(t *testing.T) {
	for _, privateRules := range []bool{false, true} {
		name := "capability"
		if privateRules {
			name = "validated"
		}
		t.Run(name, func(t *testing.T) {
			f, source, capture := prepareHeld(t, privateRules)
			origin := f.Chain.Origin
			origin.DeviceActive = false
			if _, err := matrixproof.VerifyUser(t.Context(), origin, origin.UserID, f.Chain.Master.PublicKey()); err == nil {
				t.Fatal("revoked producer regained live publication authority")
			}
			// Only public stored bytes survive this relay restart. No active Device,
			// VerifiedPublication, SDK user token, epoch identity or SSSS is restored.
			restarted := cloneHeld(t, source)
			value := capture.Value()
			restored, err := resource.RestoreReservation(t.Context(), value.Resource, value.OpID, restarted)
			if err != nil {
				t.Fatal("accepted hold could not recover without producer online", err)
			}
			if restored.SHA256() != capture.SHA256() || restored.Value().AttemptID != source.Held.Candidate.Proof.TransactionID {
				t.Fatal("reconciliation replaced immutable original attempt")
			}
			reference := storage.Reference{RoomID: source.Held.Candidate.Proof.RoomID, EventID: "$accepted-original"}
			calls := 0
			accepted, err := resource.ConfirmReservedAcceptance(t.Context(), restored, reference, acceptanceFixture(func(requested storage.Reference) (*event.Event, error) {
				calls++
				if requested != reference {
					t.Fatal("reconciliation fetched another endpoint")
				}
				var content event.EncryptedEventContent
				if json.Unmarshal(restarted.Held.Candidate.EncryptedEvent, &content) != nil {
					t.Fatal("invalid SDK fixture")
				}
				return &event.Event{ID: id.EventID(reference.EventID), RoomID: id.RoomID(reference.RoomID), Sender: id.UserID(value.Actor.UserID), Type: event.EventEncrypted, Content: event.Content{Parsed: &content}}, nil
			}))
			if err != nil || calls != 1 || accepted.ReservationSHA256() != capture.SHA256() || accepted.PublicationSHA256() != value.PublicationSHA256 || accepted.Reference() != reference {
				t.Fatal("existing accepted ciphertext was not acknowledged exactly", err)
			}
			if _, err := resource.ConfirmReservedAcceptance(t.Context(), resource.VerifiedReservation{}, reference, nil); err == nil {
				t.Fatal("raw or zero reservation became acceptance")
			}
		})
	}
}

func TestResourceReservationRejectsFakeHoldsAndPublicBackupClaims(t *testing.T) {
	_, original, _ := prepareHeld(t, true)
	for _, fault := range []string{"prepared", "committed", "attempt", "generation", "base", "descriptor-sha", "missing-record", "corrupt-record", "missing-pub-signature", "missing-policy-signature", "changed-pin", "validation", "different-cipher"} {
		t.Run(fault, func(t *testing.T) {
			source := cloneHeld(t, original)
			switch fault {
			case "prepared":
				source.Held.State = resource.Prepared
			case "committed":
				source.Held.State = resource.Committed
			case "attempt":
				source.Held.AttemptID = "new-attempt"
			case "generation":
				source.Held.Generation = "2"
			case "base":
				source.Held.Base.Seq = "1"
			case "descriptor-sha":
				source.Held.DescriptorSHA256 = source.Held.PublicationSHA256
			case "missing-record":
				for hash := range source.Records {
					delete(source.Records, hash)
					break
				}
			case "corrupt-record":
				for hash := range source.Records {
					source.Records[hash] = json.RawMessage("{}")
					break
				}
			case "different-cipher":
				source.Held.Candidate.EncryptedEvent = json.RawMessage("{}")
			case "validation":
				source.Held.Candidate.Validation.Allow = false
			case "missing-pub-signature":
				source.Held.Candidate.Proof.Signatures = nil
				hash, err := resource.CandidateDigest(source.Held.Candidate)
				if err != nil {
					t.Fatal(err)
				}
				source.Held.CandidateSHA256 = hash
				rewriteDescriptor(t, source, func(d *resource.Reservation) { d.CandidateSHA256 = hash })
			case "changed-pin":
				rewriteDescriptor(t, source, func(d *resource.Reservation) { d.Producer.Pin.SigningKey = matrixprooftest.NewSigning(t).PublicKey() })
			case "missing-policy-signature":
				var descriptor resource.Reservation
				_ = json.Unmarshal(source.Held.Descriptor, &descriptor)
				var epoch resource.SignedEpoch
				_ = json.Unmarshal(source.Records[descriptor.Policy.StatementSHA256], &epoch)
				epoch.Signatures = nil
				raw, err := matrixproof.CanonicalBytes(epoch)
				if err != nil {
					t.Fatal(err)
				}
				hash, err := resource.CipherJSONDigest(raw)
				if err != nil {
					t.Fatal(err)
				}
				source.Records[hash] = raw
				rewriteDescriptor(t, source, func(d *resource.Reservation) { d.Policy.StatementSHA256 = hash })
			}
			if _, err := resource.RestoreReservation(t.Context(), source.Held.Resource, source.Held.OpID, source); err == nil {
				t.Fatal("unaccepted/fabricated hold or unsigned backup became authority", fault)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := resource.RestoreReservation(ctx, original.Held.Resource, original.Held.OpID, original); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestResourceReservationRejectsTransactionChosenAfterSignature(t *testing.T) {
	f := resourcetest.New(t, false)
	access, err := resource.Authorize(f.Epoch, f.Cap, f.Scope.Subject, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := resource.VerifyCandidate(t.Context(), f.Candidate, access, f.Device, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, attempt := range []string{"attempt-original", "00000000-0000-4000-8000-000000000002"} {
		if _, err := resource.CaptureReservation(t.Context(), f.Candidate, verified, attempt); err == nil {
			t.Fatal("relay replaced SDK-signed client transaction", attempt)
		}
	}
}

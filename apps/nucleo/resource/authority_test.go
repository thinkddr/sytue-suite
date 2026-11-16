// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

type authorityFixture struct {
	chain             *matrixprooftest.Fixture
	recipientIdentity *pqc.Identity
	epochIdentity     *pqc.Identity
	enrolled          resource.VerifiedRecipient
	device            matrixproof.Device
	scope             storage.Scope
	actor             resource.Actor
	roster            resource.VerifiedRoster
	epoch             resource.VerifiedEpoch
	signedEpoch       resource.SignedEpoch
	cap               resource.VerifiedCapability
	signedCap         resource.SignedCapability
	candidate         resource.Candidate
	anchor            resource.Anchor
}

func authority(t *testing.T, privateRules bool) authorityFixture {
	t.Helper()
	f := resourcetest.New(t, privateRules)
	return authorityFixture{chain: f.Chain, recipientIdentity: f.RecipientIdentity, epochIdentity: f.EpochIdentity, enrolled: f.Enrolled, device: f.Device, scope: f.Scope, actor: f.Actor, roster: f.Roster, epoch: f.Epoch, signedEpoch: f.SignedEpoch, cap: f.Cap, signedCap: f.SignedCap, candidate: f.Candidate, anchor: f.Anchor}
}

func TestResourceAuthorityCannotBeConstructedFromServerRows(t *testing.T) {
	f := authority(t, false)
	access, err := resource.Authorize(f.epoch, f.cap, f.scope.Subject, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := resource.VerifyCandidate(t.Context(), f.candidate, access, f.device, nil)
	if err != nil || verified.SHA256() == "" || verified.Value().ExpectedBase.Seq != "9007199254740993" {
		t.Fatal("authorized candidate rejected", err)
	}
	for _, fault := range []string{"empty-epoch", "empty-cap", "wrong-subject", "new-pin", "resource", "role", "actions", "recipient", "new-device", "cross-product", "ciphertext", "plaintext", "session-cert", "operation", "previous-head"} {
		t.Run(fault, func(t *testing.T) {
			epoch, capability := f.epoch, f.cap
			subject := f.scope.Subject
			candidate := f.candidate
			device := f.device
			switch fault {
			case "empty-epoch":
				epoch = resource.VerifiedEpoch{}
			case "empty-cap":
				capability = resource.VerifiedCapability{}
			case "wrong-subject":
				subject = "another"
			case "new-pin":
				anchor := f.anchor
				anchor.PolicySHA256 = strings.Repeat("4", 64)
				_, err = resource.VerifyInitialEpoch(t.Context(), f.signedEpoch, f.roster, device, anchor)
				if err == nil {
					t.Fatal("server pin accepted")
				}
				return
			case "resource", "role", "actions", "recipient":
				changed := f.signedCap
				switch fault {
				case "resource":
					changed.Resource.ID = "another"
				case "role":
					changed.Role = "viewer"
				case "actions":
					changed.Actions = []resource.Action{resource.Read}
				case "recipient":
					changed.ProductKeyID = strings.Repeat("5", 64)
				}
				changed.Signatures = matrixprooftest.Sign(t, f.chain.Device, changed, f.chain.Origin.UserID, f.actor.DeviceID)
				_, err = resource.VerifyCapability(t.Context(), changed, epoch, device)
				if err == nil {
					t.Fatal("grant not in signed roster accepted")
				}
				return
			case "new-device":
				changed := candidate.Proof
				changed.Actor.DeviceID = "NEW_DEVICE"
				candidate.Proof = changed
			case "cross-product":
				origin := f.chain.Origin
				origin.Scope.Product = "rooms"
				device, err = matrixproof.VerifyUser(t.Context(), origin, origin.UserID, f.chain.Master.PublicKey())
				if err != nil {
					t.Fatal(err)
				}
			case "ciphertext":
				candidate.EncryptedEvent = append(append([]byte(nil), candidate.EncryptedEvent[:len(candidate.EncryptedEvent)-1]...), []byte(",\"sender_key\":\"changed\"}")...)
			case "plaintext":
				candidate.EncryptedEvent = append(append([]byte(nil), candidate.EncryptedEvent[:len(candidate.EncryptedEvent)-1]...), []byte(",\"body\":\"private data\"}")...)
			case "session-cert":
				candidate.SessionCertificate = []byte("{}")
			case "operation":
				candidate.Proof.OpID = "another"
			case "previous-head":
				candidate.Proof.ExpectedBase.HeadCipherSHA256 = strings.Repeat("6", 64)
			}
			access, err := resource.Authorize(epoch, capability, subject, resource.Publish)
			if err == nil {
				_, err = resource.VerifyCandidate(t.Context(), candidate, access, device, nil)
			}
			if err == nil {
				t.Fatal("unverified row or changed publication accepted", fault)
			}
		})
	}
	// Accessors return copies; callers cannot mutate the verified permission set.
	public := f.cap.Value()
	public.Actions[0] = resource.Read
	if f.cap.Value().Actions[0] != resource.Comment {
		t.Fatal("capability alias mutated verified authority")
	}
}

func TestResourcePendingValidationCannotBecomeCommitThroughEditorSignature(t *testing.T) {
	f := authority(t, true)
	access, err := resource.Authorize(f.epoch, f.cap, f.scope.Subject, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := resource.VerifyProducerCandidate(t.Context(), f.candidate, access, f.device)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resource.CompleteCandidate(t.Context(), f.candidate, pending, nil); !errors.Is(err, resource.ErrValidatorUnavailable) {
		t.Fatal("missing private validation accepted", err)
	}
	validationAccess, err := resource.Authorize(f.epoch, f.cap, f.scope.Subject, resource.Validate)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &resource.ValidationCertificate{Version: 1, Domain: "org.sytue.resource.validation.v1", PublicationSHA256: pending.SHA256(), ValidatorCapabilitySHA256: f.cap.SHA256(), RulesVersion: f.epoch.Value().Validation.RulesVersion, Allow: true, Issuer: f.actor}
	certificate.Signatures = matrixprooftest.Sign(t, f.chain.Device, certificate, f.chain.Origin.UserID, f.actor.DeviceID)
	candidate := f.candidate
	candidate.Validation = certificate
	completed, err := resource.CompleteCandidate(t.Context(), candidate, pending, &resource.ValidationEvidence{Access: validationAccess, Device: f.device})
	if err != nil || completed.SHA256() != pending.SHA256() {
		t.Fatal("manager validation did not resume exact durable candidate", err)
	}
	if _, err := resource.CompleteCandidate(t.Context(), candidate, pending, &resource.ValidationEvidence{Access: access, Device: f.device}); err == nil {
		t.Fatal("producer publish role substituted for validator grant")
	}
	changed := candidate
	changed.Proof.OpCount = "3"
	if _, err := resource.CompleteCandidate(t.Context(), changed, pending, &resource.ValidationEvidence{Access: validationAccess, Device: f.device}); err == nil {
		t.Fatal("validation certified a different immutable candidate")
	}
}

type acceptanceFixture func(storage.Reference) (*event.Event, error)

func (f acceptanceFixture) AcceptedEvent(_ context.Context, reference storage.Reference) (*event.Event, error) {
	return f(reference)
}

func TestResourceAcceptanceRequiresExactFetchedSDKCipherEvent(t *testing.T) {
	f := authority(t, false)
	access, err := resource.Authorize(f.epoch, f.cap, f.scope.Subject, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := resource.VerifyCandidate(t.Context(), f.candidate, access, f.device, nil)
	if err != nil {
		t.Fatal(err)
	}
	reference := storage.Reference{RoomID: f.candidate.Proof.RoomID, EventID: "$accepted"}
	source := func(fault string) acceptanceFixture {
		return func(requested storage.Reference) (*event.Event, error) {
			if requested != reference {
				t.Fatal("acceptance fetched different endpoint")
			}
			if fault == "transport" {
				return nil, errors.New("synthetic transport unavailable")
			}
			var content event.EncryptedEventContent
			if json.Unmarshal(f.candidate.EncryptedEvent, &content) != nil {
				t.Fatal("SDK fixture invalid")
			}
			wire := &event.Event{ID: id.EventID(reference.EventID), RoomID: id.RoomID(reference.RoomID), Sender: id.UserID(f.actor.UserID), Type: event.EventEncrypted, Content: event.Content{Parsed: &content}}
			switch fault {
			case "event":
				wire.ID = "$other"
			case "room":
				wire.RoomID = "!other:matrix.test"
			case "sender":
				wire.Sender = "@other:matrix.test"
			case "type":
				wire.Type = event.EventMessage
			case "cipher":
				content.MegolmCiphertext = []byte("differentcipher")
			}
			return wire, nil
		}
	}
	accepted, err := resource.ConfirmAcceptance(t.Context(), publication, reference, source(""))
	if err != nil || accepted.Reference() != reference || accepted.PublicationSHA256() != publication.SHA256() || accepted.CipherSHA256() != publication.Value().OuterCipherSHA256 {
		t.Fatal("exact accepted ciphertext rejected", err)
	}
	for _, fault := range []string{"event", "room", "sender", "type", "cipher", "transport"} {
		t.Run(fault, func(t *testing.T) {
			if _, err := resource.ConfirmAcceptance(t.Context(), publication, reference, source(fault)); err == nil {
				t.Fatal("different/unaccepted event committed", fault)
			}
		})
	}
	if _, err := resource.ConfirmAcceptance(t.Context(), resource.VerifiedPublication{}, reference, source("")); err == nil {
		t.Fatal("zero verified publication accepted")
	}
}

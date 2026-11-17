// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

type resourceFence func(context.Context, resource.VerifiedAccess) error

func (f resourceFence) Revalidate(ctx context.Context, access resource.VerifiedAccess) error {
	return f(ctx, access)
}
func activeResourceFence(context.Context, resource.VerifiedAccess) error { return nil }

func TestResourceEpochGrantUsesMaintainedHybridAndNeverExportsKeys(t *testing.T) {
	f := authority(t, false)
	window, err := resource.CurrentEpoch(f.epoch)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := resource.BindOwnedEpoch(window, f.epochIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Close()
	manager, err := resource.Authorize(f.epoch, f.cap, f.scope.Subject, resource.Manage)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := resource.Authorize(f.epoch, f.cap, f.scope.Subject, resource.Read)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := resource.Authorize(f.epoch, f.cap, f.scope.Subject, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	fence := resourceFence(activeResourceFence)
	grant, err := owned.PrepareGrant(t.Context(), manager, f.cap, fence)
	if err != nil {
		t.Fatal(err)
	}
	signed := resource.SignedEpochGrant{EpochGrant: grant}
	signed.Signatures = matrixprooftest.Sign(t, f.chain.Device, signed, f.chain.Origin.UserID, f.actor.DeviceID)
	opened, err := resource.OpenGrant(t.Context(), signed, window, manager, reader, f.recipientIdentity, f.device, fence)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	payload := bytes.Repeat([]byte("{\"seq\":9223372036854775807,\"value\":\"original Office bytes\"}\n"), 600)
	envelope, err := opened.SealFrame(t.Context(), f.candidate.Proof.Publication, payload, writer, fence)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	innerHash, err := resource.CipherJSONDigest(raw)
	if err != nil {
		t.Fatal(err)
	}
	candidate := f.candidate
	candidate.Proof.InnerCipherSHA256 = innerHash
	candidate.Proof.Signatures = matrixprooftest.Sign(t, f.chain.Device, candidate.Proof, f.chain.Origin.UserID, f.actor.DeviceID)
	pending, err := resource.VerifyProducerCandidate(t.Context(), candidate, writer, f.device)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := opened.OpenFrame(t.Context(), pending, envelope, reader, fence)
	if err != nil || !bytes.Equal(actual, payload) {
		t.Fatal("shared hybrid epoch changed original domain bytes", err)
	}
	clear(actual)
	wrongIdentity, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resource.OpenGrant(t.Context(), signed, window, manager, reader, wrongIdentity, f.device, fence); err == nil {
		t.Fatal("public cap/roster released key without recipient private identity")
	}
	changed := signed
	changed.Context.RecipientScope.Product = "rooms"
	changed.Signatures = matrixprooftest.Sign(t, f.chain.Device, changed, f.chain.Origin.UserID, f.actor.DeviceID)
	if _, err := resource.OpenGrant(t.Context(), changed, window, manager, reader, f.recipientIdentity, f.device, fence); err == nil {
		t.Fatal("manager signature bypassed exact product context")
	}
	if _, err := owned.PrepareGrant(t.Context(), reader, f.cap, fence); err == nil {
		t.Fatal("reader role granted epoch key")
	}
	if _, err := opened.SealFrame(t.Context(), f.candidate.Proof.Publication, payload, reader, fence); err == nil {
		t.Fatal("reader sealed writable publication")
	}
	checks := 0
	lateClose := resourceFence(func(context.Context, resource.VerifiedAccess) error {
		checks++
		if checks == 2 {
			opened.Close()
		}
		return nil
	})
	if content, err := opened.OpenFrame(t.Context(), pending, envelope, reader, lateClose); !errors.Is(err, resource.ErrRevoked) || content != nil {
		t.Fatal("retired handle released plaintext", err)
	}
}

func rotateFixture(t *testing.T, f authorityFixture, history resource.HistoryPolicy) (resource.VerifiedEpoch, resource.VerifiedCapability, *pqc.Identity) {
	t.Helper()
	nextKey, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	next := f.signedEpoch
	next.ID = "epoch2"
	next.Generation = "2"
	next.PreviousEpoch = f.epoch.Value().ID
	next.PreviousPolicySHA256 = f.epoch.SHA256()
	next.History = history
	next.Recipient = nextKey.Recipient().String()
	end, err := f.candidate.Proof.End()
	if err != nil {
		t.Fatal(err)
	}
	next.Cutover = resource.Position{Seq: end, HeadCipherSHA256: f.candidate.Proof.OuterCipherSHA256}
	prepareFixtureCheckpoint(t, f, &next, next.Cutover)
	next.Signatures = matrixprooftest.Sign(t, f.chain.Device, next, f.chain.Origin.UserID, f.actor.DeviceID)
	epoch, err := resource.VerifyNextEpoch(t.Context(), next, f.roster, f.device, f.epoch, f.cap, next.Cutover)
	if err != nil {
		t.Fatal(err)
	}
	capability := f.signedCap
	capability.Epoch = next.ID
	capability.Generation = next.Generation
	capability.PolicySHA256 = epoch.SHA256()
	capability.Signatures = matrixprooftest.Sign(t, f.chain.Device, capability, f.chain.Origin.UserID, f.actor.DeviceID)
	verified, err := resource.VerifyCapability(t.Context(), capability, epoch, f.device)
	if err != nil {
		t.Fatal(err)
	}
	return epoch, verified, nextKey
}

func TestResourceRotationPreservesAuthenticatedHistoryAndStopsOldEpochWrites(t *testing.T) {
	f := authority(t, false)
	next, nextCap, _ := rotateFixture(t, f, resource.HistoryJoined)
	oldWindow, err := resource.ClosedEpoch(f.epoch, next)
	if err != nil {
		t.Fatal(err)
	}
	old, err := resource.BindOwnedEpoch(oldWindow, f.epochIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	manager, err := resource.Authorize(next, nextCap, f.scope.Subject, resource.Manage)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := resource.Authorize(next, nextCap, f.scope.Subject, resource.Read)
	if err != nil {
		t.Fatal(err)
	}
	fence := resourceFence(activeResourceFence)
	grant, err := old.PrepareGrant(t.Context(), manager, nextCap, fence)
	if err != nil {
		t.Fatal("existing member lost already authorized history", err)
	}
	signed := resource.SignedEpochGrant{EpochGrant: grant}
	signed.Signatures = matrixprooftest.Sign(t, f.chain.Device, signed, f.chain.Origin.UserID, f.actor.DeviceID)
	restored, err := resource.OpenGrant(t.Context(), signed, oldWindow, manager, reader, f.recipientIdentity, f.device, fence)
	if err != nil {
		t.Fatal("fresh own product handle could not recover old epoch", err)
	}
	defer restored.Close()
	writer, err := resource.Authorize(next, nextCap, f.scope.Subject, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.SealFrame(t.Context(), f.candidate.Proof.Publication, []byte("late old epoch"), writer, fence); err == nil {
		t.Fatal("closed historical epoch accepted new publication")
	}
	// Even a correctly signed successor cannot mutate the pinned previous head.
	changed := resource.SignedEpoch{Epoch: next.Value()}
	changed.Signatures = matrixprooftest.Sign(t, f.chain.Device, changed, f.chain.Origin.UserID, f.actor.DeviceID)
	wrong := changed.Cutover
	wrong.HeadCipherSHA256 = f.epoch.Value().Cutover.HeadCipherSHA256
	if _, err := resource.VerifyNextEpoch(t.Context(), changed, f.roster, f.device, f.epoch, f.cap, wrong); err == nil {
		t.Fatal("server-selected cutover replaced independently pinned head")
	}
}

func TestResourceNewMemberHistoryRequiresSignedFullGrantAndJoinedRotation(t *testing.T) {
	for _, history := range []resource.HistoryPolicy{resource.HistoryFull, resource.HistoryJoined} {
		t.Run(string(history), func(t *testing.T) {
			f := authority(t, false)
			scope := f.scope
			scope.Subject = "viewer"
			peer := matrixprooftest.NewUser(t, scope, "matrix.test", "@viewer:matrix.test", "VIEWER_DOCS")
			peerDevice, err := matrixproof.VerifyUser(t.Context(), peer.Origin, peer.Origin.UserID, peer.Master.PublicKey())
			if err != nil {
				t.Fatal(err)
			}
			actor := resource.Actor{UserID: peer.Origin.UserID.String(), DeviceID: peer.Origin.DeviceID.String(), RootKey: peer.Master.PublicKey().String()}
			key, err := pqc.GenerateIdentity()
			if err != nil {
				t.Fatal(err)
			}
			keyID, _ := pqc.RecipientID(key.Recipient())
			enrolled := resource.SignedRecipient{RecipientEnrollment: resource.RecipientEnrollment{Version: 1, Domain: "org.sytue.resource.recipient.v1", Scope: scope, UserID: actor.UserID, Recipient: key.Recipient().String(), KeyID: keyID, Issuer: actor}}
			enrolled.Signatures = matrixprooftest.Sign(t, peer.Device, enrolled, peer.Origin.UserID, actor.DeviceID)
			verifiedRecipient, err := resource.VerifyRecipient(t.Context(), enrolled, peerDevice, storage.PublicationTarget{RoomID: f.epoch.Value().RoomID, DeviceID: actor.DeviceID})
			if err != nil {
				t.Fatal(err)
			}
			end, _ := f.candidate.Proof.End()
			cutover := resource.Position{Seq: end, HeadCipherSHA256: f.candidate.Proof.OuterCipherSHA256}
			from := f.epoch.Value().Cutover
			if history == resource.HistoryJoined {
				from = cutover
			}
			members := f.roster.Members()
			members = append(members, resource.Member{Scope: scope, UserID: actor.UserID, RootKey: actor.RootKey, Recipient: key.Recipient().String(), ProductKeyID: keyID, EnrollmentSHA256: verifiedRecipient.SHA256(), Role: "viewer", Actions: []resource.Action{resource.Read}, From: from})
			roster, err := resource.VerifyRoster(f.epoch.Value().Resource, members, []resource.VerifiedRecipient{f.enrolled, verifiedRecipient})
			if err != nil {
				t.Fatal(err)
			}
			epochKey, err := pqc.GenerateIdentity()
			if err != nil {
				t.Fatal(err)
			}
			signed := f.signedEpoch
			signed.ID = "epoch2"
			signed.Generation = "2"
			signed.PreviousEpoch = f.epoch.Value().ID
			signed.PreviousPolicySHA256 = f.epoch.SHA256()
			signed.Cutover = cutover
			signed.Recipient = epochKey.Recipient().String()
			signed.History = history
			signed.MembershipSHA256 = roster.SHA256()
			prepareFixtureCheckpoint(t, f, &signed, signed.Cutover)
			signed.Signatures = matrixprooftest.Sign(t, f.chain.Device, signed, f.chain.Origin.UserID, f.actor.DeviceID)
			epoch, err := resource.VerifyNextEpoch(t.Context(), signed, roster, f.device, f.epoch, f.cap, cutover)
			if err != nil {
				t.Fatal(err)
			}
			ownerCap := f.signedCap
			ownerCap.Epoch = signed.ID
			ownerCap.Generation = signed.Generation
			ownerCap.PolicySHA256 = epoch.SHA256()
			ownerCap.MembershipSHA256 = roster.SHA256()
			ownerCap.Signatures = matrixprooftest.Sign(t, f.chain.Device, ownerCap, f.chain.Origin.UserID, f.actor.DeviceID)
			owner, err := resource.VerifyCapability(t.Context(), ownerCap, epoch, f.device)
			if err != nil {
				t.Fatal(err)
			}
			peerCap := ownerCap
			peerCap.Subject = scope.Subject
			peerCap.UserID = actor.UserID
			peerCap.ProductKeyID = keyID
			peerCap.Role = "viewer"
			peerCap.Actions = []resource.Action{resource.Read}
			peerCap.From = from
			peerCap.Signatures = matrixprooftest.Sign(t, f.chain.Device, peerCap, f.chain.Origin.UserID, f.actor.DeviceID)
			viewer, err := resource.VerifyCapability(t.Context(), peerCap, epoch, f.device)
			if err != nil {
				t.Fatal(err)
			}
			manager, err := resource.Authorize(epoch, owner, f.scope.Subject, resource.Manage)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := resource.Authorize(epoch, viewer, scope.Subject, resource.Read)
			if err != nil {
				t.Fatal(err)
			}
			fence := resourceFence(activeResourceFence)
			oldWindow, err := resource.ClosedEpoch(f.epoch, epoch)
			if err != nil {
				t.Fatal(err)
			}
			old, err := resource.BindOwnedEpoch(oldWindow, f.epochIdentity)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Close()
			grant, err := old.PrepareGrant(t.Context(), manager, viewer, fence)
			if history == resource.HistoryJoined {
				if err == nil {
					t.Fatal("new joined-only member received a pre-join epoch key")
				}
			} else {
				if err != nil {
					t.Fatal("signed full-history member could not recover history", err)
				}
				signedGrant := resource.SignedEpochGrant{EpochGrant: grant}
				signedGrant.Signatures = matrixprooftest.Sign(t, f.chain.Device, signedGrant, f.chain.Origin.UserID, f.actor.DeviceID)
				restored, err := resource.OpenGrant(t.Context(), signedGrant, oldWindow, manager, reader, key, f.device, fence)
				if err != nil {
					t.Fatal(err)
				}
				restored.Close()
			}
			currentWindow, _ := resource.CurrentEpoch(epoch)
			current, err := resource.BindOwnedEpoch(currentWindow, epochKey)
			if err != nil {
				t.Fatal(err)
			}
			defer current.Close()
			currentGrant, err := current.PrepareGrant(t.Context(), manager, viewer, fence)
			if err != nil {
				t.Fatal("new member could not receive current epoch", err)
			}
			signedGrant := resource.SignedEpochGrant{EpochGrant: currentGrant}
			signedGrant.Signatures = matrixprooftest.Sign(t, f.chain.Device, signedGrant, f.chain.Origin.UserID, f.actor.DeviceID)
			joined, err := resource.OpenGrant(t.Context(), signedGrant, currentWindow, manager, reader, key, f.device, fence)
			if err != nil {
				t.Fatal(err)
			}
			joined.Close()
		})
	}
}

func prepareFixtureCheckpoint(t *testing.T, f authorityFixture, next *resource.SignedEpoch, cutover resource.Position) {
	t.Helper()
	manager, err := resource.Authorize(f.epoch, f.cap, f.scope.Subject, resource.Manage)
	if err != nil {
		t.Fatal(err)
	}
	checkpointCtx, err := resource.PrepareCheckpointContext(manager, next.ID, next.Recipient, cutover)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := resource.BootstrapContextSHA256(checkpointCtx)
	if err != nil {
		t.Fatal(err)
	}
	next.Checkpoint = resource.Checkpoint{SourceCutover: cutover, Reference: storage.Reference{RoomID: next.RoomID, EventID: "$checkpoint-" + next.ID}, CipherSHA256: strings.Repeat("9", 64), ContextSHA256: hash, SessionCertificateID: "sessioncert-checkpoint-" + next.ID}
}

// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// This is a public metadata/registry witness with actual SDK signatures. Its
// accepted ciphertext is synthetic; SQL and actual SDK readback are separate.
type policySourceFixture struct {
	Retained resource.RetainedBootstrapPolicy
	Records  map[string]json.RawMessage
	withdraw bool
}

func (s *policySourceFixture) RetainedBootstrapPolicy(ctx context.Context, key resource.Resource, plan string) (resource.RetainedBootstrapPolicy, error) {
	if s.withdraw || key != s.Retained.Capture.Resource || plan != s.Retained.Capture.PlanID {
		return resource.RetainedBootstrapPolicy{}, resource.ErrRevoked
	}
	return s.Retained, ctx.Err()
}

func (s *policySourceFixture) PublicEvidence(ctx context.Context, key resource.Resource, sha string) (json.RawMessage, error) {
	if s.withdraw || key != s.Retained.Capture.Resource || s.Records[sha] == nil {
		return nil, resource.ErrRevoked
	}
	return bytes.Clone(s.Records[sha]), ctx.Err()
}

// A separate port models an authenticated private vault pin. No value returned
// by policySourceFixture is ever used as this independent trust decision.
type policyAnchorFixture struct{ anchor resource.Anchor }

func (s policyAnchorFixture) BootstrapPolicyAnchor(ctx context.Context, key resource.Resource, _ string) (resource.Anchor, error) {
	if key != s.anchor.Resource {
		return resource.Anchor{}, resource.ErrRevoked
	}
	return s.anchor, ctx.Err()
}

type policyRestoreCase struct {
	source     *policySourceFixture
	anchors    policyAnchorFixture
	capability resource.CapabilityEvidence
	current    resource.VerifiedSession
	registry   *resourcetest.SessionSource
	backup     []byte
	anchorRaw  []byte
	envelopes  map[resource.BootstrapKind]pqc.Envelope
	payloads   map[resource.BootstrapKind][]byte
}

func preparePolicyRestoreCase(t testing.TB) policyRestoreCase {
	t.Helper()
	c, captureSource := captureHistoricalCase(t)
	plan, err := resource.CompleteBootstrapRosterCapture(t.Context(), c.pending, capturePages{inputs: c.inputs}, c.provider)
	if err != nil {
		t.Fatal(err)
	}
	captured, err := resource.CaptureBootstrapPlan(t.Context(), plan)
	if err != nil {
		t.Fatal(err)
	}
	b := c.base.bootstrap
	b.Verified, b.Signed = plan, c.plan
	baseline := b.OpaqueCandidate(t, resource.BootstrapBaseline, nil)
	envelopes := map[resource.BootstrapKind]pqc.Envelope{}
	payloads := map[resource.BootstrapKind][]byte{}
	sealDescriptor := func(candidate resource.BootstrapCandidate, contextValue resource.BootstrapContext) resource.BootstrapCandidate {
		t.Helper()
		payload := []byte("original private " + string(contextValue.Kind) + " descriptor")
		envelope, err := resource.SealBootstrapDescriptor(c.base.f.EpochIdentity.Recipient(), contextValue, payload)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		hash, err := resource.CipherJSONDigest(raw)
		clear(raw)
		if err != nil {
			t.Fatal(err)
		}
		candidate.Proof.InnerCipherSHA256 = hash
		candidate.Proof.Signatures = b.Sign(candidate.Proof)
		envelopes[contextValue.Kind], payloads[contextValue.Kind] = envelope, payload
		return candidate
	}
	baselineContext, err := plan.BaselineContext()
	if err != nil {
		t.Fatal(err)
	}
	baseline = sealDescriptor(baseline, baselineContext)
	verified, err := resource.VerifyBootstrapCandidate(t.Context(), baseline, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref := storage.Reference{RoomID: c.plan.RoomID, EventID: "$fresh-policy-baseline"}
	accepted, err := resource.ConfirmBootstrapAcceptance(t.Context(), verified, ref, bootstrapAcceptanceFixture(baseline, ref))
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := b.OpaqueCandidate(t, resource.BootstrapCheckpoint, &accepted)
	checkpointContext, err := plan.CheckpointContext(accepted)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint = sealDescriptor(checkpoint, checkpointContext)
	cpVerified, err := resource.VerifyBootstrapCandidate(t.Context(), checkpoint, plan, &accepted)
	if err != nil {
		t.Fatal(err)
	}
	cpRef := storage.Reference{RoomID: c.plan.RoomID, EventID: "$fresh-policy-checkpoint"}
	cpAccepted, err := resource.ConfirmBootstrapAcceptance(t.Context(), cpVerified, cpRef, bootstrapAcceptanceFixture(checkpoint, cpRef))
	if err != nil {
		t.Fatal(err)
	}
	signed := b.FinalEpoch(t, accepted, cpAccepted)
	activation, err := resource.VerifyBootstrapActivation(t.Context(), signed, plan, accepted, cpAccepted)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := resource.EncodeBootstrapRecovery(t.Context(), plan, c.base.f.EpochIdentity)
	if err != nil {
		t.Fatal(err)
	}
	source := &policySourceFixture{Records: map[string]json.RawMessage{}}
	for _, record := range captured.Records() {
		source.Records[record.SHA256] = record.Value
	}
	for _, sha := range captureSource.records.EvidenceHashes() {
		record, err := captureSource.records.Evidence(sha)
		if err != nil {
			t.Fatal(err)
		}
		source.Records[sha] = record.Value
	}
	var descriptor resource.BootstrapPlanEvidence
	if json.Unmarshal(captured.EvidenceBytes(), &descriptor) != nil {
		t.Fatal("original plan descriptor")
	}
	var members []resource.Member
	if json.Unmarshal(source.Records[descriptor.MembersSHA256], &members) != nil {
		t.Fatal("original members")
	}
	var owner resource.Member
	var recipient resource.StatementEvidence
	for _, m := range members {
		if m.Scope == c.plan.CreatorScope {
			owner = m
		}
	}
	for _, r := range descriptor.Recipients {
		var enrolled resource.SignedRecipient
		if json.Unmarshal(source.Records[r.StatementSHA256], &enrolled) == nil && enrolled.Scope == owner.Scope {
			recipient = r
		}
	}
	ownerCap := resource.Capability{Version: 1, Domain: "org.sytue.resource.capability.v1", Resource: c.plan.Source.Resource, RoomID: c.plan.RoomID, Epoch: c.plan.Epoch, Generation: "1", MembershipSHA256: c.plan.MembershipSHA256, PolicySHA256: activation.Anchor().PolicySHA256, Subject: owner.Scope.Subject, UserID: owner.UserID, ProductKeyID: owner.ProductKeyID, Role: owner.Role, Actions: owner.Actions, From: owner.From, Issuer: c.plan.Creator}
	signedCap := resource.SignedCapability{Capability: ownerCap, Signatures: b.Sign(ownerCap)}
	anchorRaw, err := resource.EncodeBootstrapPolicyAnchor(t.Context(), activation, signedCap)
	if err != nil {
		t.Fatal(err)
	}
	capRaw, err := matrixproof.CanonicalBytes(signedCap)
	if err != nil {
		t.Fatal(err)
	}
	capSHA, err := resource.CipherJSONDigest(capRaw)
	if err != nil {
		t.Fatal(err)
	}
	source.Records[capSHA] = capRaw
	capability := resource.CapabilityEvidence{Capability: resource.StatementEvidence{StatementSHA256: capSHA, Signer: descriptor.Plan.Signer}, Recipient: recipient, IssuerEnrollment: recipient}
	capture := captureSource.held
	capture.State = "complete"
	source.Retained = resource.RetainedBootstrapPolicy{Capture: capture, State: "active", Epoch: signed.ID, Generation: signed.Generation, PolicySHA256: activation.Anchor().PolicySHA256, MembershipSHA256: signed.MembershipSHA256, PlanEvidenceSHA256: captured.EvidenceSHA256(), PlanEvidence: captured.EvidenceBytes(), SignedEpoch: activation.PublicEpochBytes(), Baseline: resource.RetainedBootstrapSlot{State: "accepted", Kind: resource.BootstrapBaseline, CandidateSHA256: verified.SHA256(), Candidate: baseline, Reference: ref}, Checkpoint: resource.RetainedBootstrapSlot{State: "accepted", Kind: resource.BootstrapCheckpoint, CandidateSHA256: cpVerified.SHA256(), Candidate: checkpoint, Reference: cpRef}}
	bChain := matrixprooftest.NewDevice(t, c.base.f.Chain, "FRESH_POLICY_B")
	leaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	current, registry := resourcetest.EnrolledSession(t, bChain, leaf)
	c.base.registry.Retire()
	c.base.f.Chain.Origin.DeviceActive = false
	if _, err := matrixproof.VerifyUser(t.Context(), c.base.f.Chain.Origin, c.base.f.Chain.Origin.UserID, c.base.f.Chain.Master.PublicKey()); err == nil {
		t.Fatal("retired A still constructed a live device")
	}
	// Retain only durable public bytes, actual B and the private-vault witness.
	// No VerifiedEpoch/VerifiedCapability survives this helper into restoration.
	return policyRestoreCase{source: source, anchors: policyAnchorFixture{activation.Anchor()}, capability: capability, current: current, registry: registry, backup: backup, anchorRaw: anchorRaw, envelopes: envelopes, payloads: payloads}
}

func restorePolicyCase(ctx context.Context, c policyRestoreCase) (resource.RestoredBootstrapPolicy, error) {
	return resource.RestoreBootstrapPolicy(ctx, c.source.Retained.Capture.Resource, c.source.Retained.Capture.PlanID, c.capability, c.current, c.anchors, c.source)
}

func TestBootstrapFreshPolicyRestoresFromPublicSourceAndIndependentOwnerPin(t *testing.T) {
	c := preparePolicyRestoreCase(t)
	defer clear(c.backup)
	// Force a fresh JSON process boundary for all ORIGINAL public evidence.
	raw, err := json.Marshal(c.source)
	if err != nil {
		t.Fatal(err)
	}
	var restarted policySourceFixture
	if json.Unmarshal(raw, &restarted) != nil {
		t.Fatal("public source restart")
	}
	c.source = &restarted
	policy, err := restorePolicyCase(t.Context(), c)
	if err != nil {
		t.Fatal("fresh B policy restoration required a held epoch/capability", err)
	}
	reader, err := policy.Access(t.Context(), resource.Read)
	if err != nil || reader.Actor() != c.current.Actor() || reader.Capability().Enrollment().Issuer == reader.Actor() {
		t.Fatal("restored current B impersonated A", err)
	}
	if _, err := resource.Authorize(reader.Epoch(), reader.Capability(), c.current.Scope().Subject, resource.Read); err == nil {
		t.Fatal("legacy authorization revived retired A from recovered metadata")
	}
	window, err := policy.Window(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	handle, err := resource.RestoreBootstrapRecovery(t.Context(), c.backup, c.source.Retained.Capture.PlanSHA256, c.current.Scope(), window)
	if err != nil {
		t.Fatal("fresh policy did not close SSSS window circularity", err)
	}
	defer handle.Close()
	manager, err := policy.Access(t.Context(), resource.Manage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.PrepareGrant(t.Context(), manager, manager.Capability(), resourceFence(activeResourceFence)); err == nil {
		t.Fatal("restored policy issued an epoch grant as historical A")
	}
	c.source.withdraw = true
	if err := reader.RevalidateSession(t.Context()); err == nil {
		t.Fatal("source withdrawal did not fence retained current B access")
	}
	if _, err := resource.Reauthorize(t.Context(), reader, c.current.Scope().Subject, resource.Read); err == nil {
		t.Fatal("reauthorization dropped ORIGINAL source fence")
	}
	if h, err := resource.RestoreBootstrapRecovery(t.Context(), c.backup, c.source.Retained.Capture.PlanSHA256, c.current.Scope(), window); err == nil {
		h.Close()
		t.Fatal("retained window bypassed withdrawn source")
	}
	c.source.withdraw = false
	replacementRegistry := &resourcetest.SessionSource{Record: c.registry.Record, Chain: c.registry.Chain}
	replacementProvider, err := resource.NewSessionProvider(replacementRegistry, replacementRegistry)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := replacementProvider.Resolve(t.Context(), c.current.Scope(), c.current.Actor().DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	c.registry.Retire()
	if _, err := resource.AuthorizeSession(t.Context(), reader.Epoch(), reader.Capability(), replacement, resource.Read); err == nil {
		t.Fatal("identical public binding replaced original consumed Provider witness")
	}
	if _, err := policy.Access(t.Context(), resource.Read); err == nil {
		t.Fatal("retired B retained restored authority")
	}
}

func TestBootstrapFreshPolicyRejectsSubstitutedSourceAnchorAndAcceptedSlots(t *testing.T) {
	c := preparePolicyRestoreCase(t)
	defer clear(c.backup)
	raw, err := json.Marshal(c.source)
	if err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"capture", "preparing", "generation", "root-generation", "source", "anchor", "pin", "evidence", "baseline-pending", "checkpoint-missing", "candidate", "reference", "cipher", "swapped", "signature", "capability"} {
		t.Run(fault, func(t *testing.T) {
			changed := c
			changed.source = &policySourceFixture{}
			if json.Unmarshal(raw, changed.source) != nil {
				t.Fatal("fixture clone")
			}
			v := &changed.source.Retained
			switch fault {
			case "capture":
				v.Capture.State = "pending"
			case "preparing":
				v.State = "preparing"
			case "generation":
				v.Generation = "2"
			case "root-generation":
				v.Capture.RootGeneration = "2"
			case "source":
				v.Capture.Source.ACLVersion = "1"
			case "anchor":
				changed.anchors.anchor.PolicySHA256 = v.MembershipSHA256
			case "pin":
				v.Capture.OwnerPin.SigningKey = "another"
			case "evidence":
				v.PlanEvidenceSHA256 = v.PolicySHA256
			case "baseline-pending":
				v.Baseline.State = "sending"
			case "checkpoint-missing":
				v.Checkpoint = resource.RetainedBootstrapSlot{}
			case "candidate":
				v.Baseline.CandidateSHA256 = v.PolicySHA256
			case "reference":
				v.Checkpoint.Reference.EventID = "$substituted"
			case "cipher":
				v.Checkpoint.Candidate.Proof.OuterCipherSHA256 = v.PolicySHA256
			case "swapped":
				v.Baseline, v.Checkpoint = v.Checkpoint, v.Baseline
			case "signature":
				v.Checkpoint.Candidate.Proof.Signatures = nil
			case "capability":
				changed.capability.IssuerEnrollment = resource.StatementEvidence{}
			}
			if _, err := restorePolicyCase(t.Context(), changed); err == nil {
				t.Fatal("substituted historical metadata became authority", fault)
			}
		})
	}
}

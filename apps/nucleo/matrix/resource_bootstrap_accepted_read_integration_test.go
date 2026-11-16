//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// Immutable PUBLIC metadata captured through actual SDK signatures and both
// actual Synapse ACKs. The Source/Registry lifecycle witness is synthetic here;
// ORIGINAL Docs SQL/RLS/freeze is independently exercised by the store tests.
// No private epoch, held VerifiedEpoch/Capability or original SDK is retained.
type nativeBootstrapPolicyWitness struct {
	Value       resource.RetainedBootstrapPolicy
	Records     map[string]json.RawMessage
	ownerIntent resource.Capability
	withdraw    atomic.Bool
}

func (s *nativeBootstrapPolicyWitness) RetainedBootstrapPolicy(ctx context.Context, key resource.Resource, plan string) (resource.RetainedBootstrapPolicy, error) {
	if s.withdraw.Load() || s.Value.Capture.Resource != key || s.Value.Capture.PlanID != plan {
		return resource.RetainedBootstrapPolicy{}, resource.ErrRevoked
	}
	return s.Value, ctx.Err()
}

func (s *nativeBootstrapPolicyWitness) PublicEvidence(ctx context.Context, key resource.Resource, sha string) (json.RawMessage, error) {
	if s.withdraw.Load() || key != s.Value.Capture.Resource || s.Records[sha] == nil {
		return nil, resource.ErrRevoked
	}
	return bytes.Clone(s.Records[sha]), ctx.Err()
}

func captureNativeBootstrapPolicyWitness(ctx context.Context, t testing.TB, plan resource.VerifiedBootstrapPlan, roster resource.CapturedBootstrapRosterEvidence, baseline, checkpoint resource.VerifiedBootstrapAcceptance, activation resource.VerifiedBootstrapActivation, journal *offline.Store) *nativeBootstrapPolicyWitness {
	t.Helper()
	captured, err := resource.CaptureBootstrapPlan(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	s := &nativeBootstrapPolicyWitness{Records: map[string]json.RawMessage{}}
	for _, record := range captured.Records() {
		s.Records[record.SHA256] = record.Value
	}
	for _, sha := range roster.EvidenceHashes() {
		record, err := roster.Evidence(sha)
		if err != nil {
			t.Fatal(err)
		}
		s.Records[sha] = record.Value
	}
	var captureDescriptor resource.BootstrapCaptureEvidence
	var planDescriptor resource.BootstrapPlanEvidence
	if json.Unmarshal(roster.Bytes(), &captureDescriptor) != nil || json.Unmarshal(captured.EvidenceBytes(), &planDescriptor) != nil || len(planDescriptor.Recipients) != 1 {
		t.Fatal("original public plan/capture transcript")
	}
	var members []resource.Member
	if json.Unmarshal(s.Records[planDescriptor.MembersSHA256], &members) != nil || len(members) != 1 {
		t.Fatal("original owner membership")
	}
	p := plan.Value()
	m := members[0]
	// This is an explicit ORIGINAL owner intent supplied to the actual SDK
	// backup orchestrator. No signed capability is put in Source/normal-op rows.
	// The unexported field does not survive the public Source JSON restart.
	s.ownerIntent = resource.Capability{Version: 1, Domain: "org.sytue.resource.capability.v1", Resource: p.Source.Resource, RoomID: p.RoomID, Epoch: p.Epoch, Generation: "1", MembershipSHA256: p.MembershipSHA256, PolicySHA256: activation.Anchor().PolicySHA256, Subject: m.Scope.Subject, UserID: m.UserID, ProductKeyID: m.ProductKeyID, Role: m.Role, Actions: m.Actions, From: m.From, Issuer: p.Creator}
	binding := roster.SessionBinding()
	s.Value = resource.RetainedBootstrapPolicy{Capture: resource.RetainedBootstrapCapture{State: "complete", Resource: p.Source.Resource, PlanID: p.ID, Source: p.Source, Creator: p.Creator, RoomID: p.RoomID, SessionBindingSHA256: p.SessionBindingSHA256, SessionGeneration: binding.Generation, RootGeneration: binding.RootGeneration, OwnerPin: roster.OwnerPin(), PlanSHA256: plan.SHA256(), ManifestSHA256: captureDescriptor.ManifestSHA256, EvidenceSHA256: roster.SHA256(), Evidence: roster.Bytes()}, State: "active", Epoch: p.Epoch, Generation: "1", PolicySHA256: activation.Anchor().PolicySHA256, MembershipSHA256: p.MembershipSHA256, PlanEvidenceSHA256: captured.EvidenceSHA256(), PlanEvidence: captured.EvidenceBytes(), SignedEpoch: activation.PublicEpochBytes()}
	owner, err := p.CreatorScope.JournalOwner()
	if err != nil {
		t.Fatal(err)
	}
	for _, accepted := range []resource.VerifiedBootstrapAcceptance{baseline, checkpoint} {
		var local bootstrapLocalPublication
		found, err := journal.Read(owner, storage.ResourceBootstrapPublicationNamespace, p.ID+"/"+string(accepted.Kind()), &local)
		if err != nil || !found || local.Candidate == nil || local.Binding != binding || local.PlanSHA256 != plan.SHA256() {
			t.Fatal("original encrypted bootstrap intent missing", err)
		}
		clear(local.Payload)
		sha, err := resource.BootstrapCandidateSHA256(*local.Candidate)
		if err != nil || sha != accepted.CandidateSHA256() {
			t.Fatal("actual SDK ACK changed original bootstrap candidate", err)
		}
		slot := resource.RetainedBootstrapSlot{State: "accepted", Kind: accepted.Kind(), CandidateSHA256: sha, Candidate: *local.Candidate, Reference: accepted.Reference()}
		if accepted.Kind() == resource.BootstrapBaseline {
			s.Value.Baseline = slot
		} else {
			s.Value.Checkpoint = slot
		}
	}
	return s
}

// A CURRENT synthetic Registry witness still consumes the production Provider,
// actual B SDK query/signatures and fresh hybrid leaf/challenge. No historical A
// flags or Device are relabelled into B; production Registry SQL is separate.
func nativePolicySession(ctx context.Context, t testing.TB, c *Client, scope storage.Scope, expectedRoot string, generation resource.Decimal) (resource.VerifiedSession, *nativeResourceRoomFixtureSource) {
	t.Helper()
	roots, err := c.crypto.Machine().GetOwnCrossSigningPublicKeys(ctx)
	if err != nil || roots == nil || roots.MasterKey.String() != expectedRoot {
		t.Fatal("fresh canonical recovery changed original account root", err)
	}
	leaf, err := c.resourceLeaf(true)
	if err != nil {
		t.Fatal(err)
	}
	source := &nativeResourceRoomFixtureSource{client: c}
	source.active.Store(true)
	device, err := source.ResourceSessionDevice(ctx, scope, c.DeviceID(), roots.MasterKey.String())
	if err != nil {
		t.Fatal(err)
	}
	keyID, err := pqc.RecipientID(leaf.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	actor := resource.Actor{UserID: c.session.UserID.String(), DeviceID: c.DeviceID(), RootKey: roots.MasterKey.String()}
	recipient := resource.SignedRecipient{RecipientEnrollment: resource.RecipientEnrollment{Version: 1, Domain: "org.sytue.resource.recipient.v1", Scope: scope, UserID: actor.UserID, Recipient: leaf.Recipient().String(), KeyID: keyID, Issuer: actor}}
	recipient.Signatures, err = c.signResourceJSON(ctx, recipient)
	if err != nil {
		t.Fatal(err)
	}
	enrolled, err := resource.VerifyAccountRecipient(ctx, recipient, device)
	if err != nil {
		t.Fatal(err)
	}
	binding := resource.SessionBinding{Scope: scope, Namespace: "resources:" + scope.Product, UserID: actor.UserID, DeviceID: actor.DeviceID, RootKey: actor.RootKey, Recipient: recipient.Recipient, ProductKeyID: keyID, EnrollmentSHA256: enrolled.SHA256(), Generation: generation, RootGeneration: "1", AccountVersion: "1", TenantVersion: "1", ConsentID: uuid.NewString()}
	now := time.Now()
	challenge, err := resource.NewSessionChallenge(ctx, binding, false, now.Add(5*time.Minute), now)
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := resource.OpenSessionChallenge(ctx, challenge, leaf, now)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := resource.PrepareSessionEnrollment(challenge, nonce, "retain_confirmed_root", now)
	if err != nil {
		t.Fatal(err)
	}
	proof := resource.SignedSessionEnrollment{SessionEnrollment: intent}
	proof.Signatures, err = c.signResourceJSON(ctx, proof)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := resource.VerifySessionEnrollment(ctx, proof, recipient, challenge, device, now)
	if err != nil {
		t.Fatal(err)
	}
	source.record = resource.SessionRecord{Binding: binding, PinnedRoot: actor.RootKey, RootGeneration: "1", Recipient: recipient, Challenge: challenge, Proof: proof, ProofSHA256: verified.SHA256(), ConfirmedAt: now}
	provider, err := resource.NewSessionProvider(source, source)
	if err != nil {
		t.Fatal(err)
	}
	current, err := provider.Resolve(ctx, scope, c.DeviceID())
	if err != nil {
		t.Fatal(err)
	}
	return current, source
}

func exerciseNativeFreshBootstrapPolicy(ctx context.Context, t *testing.T, old *Client, cfg Config, password, recovery string, vault *Client, public *nativeBootstrapPolicyWitness, lease offline.WorkLease, journal *offline.Store) {
	t.Helper()
	encoded, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	source := &nativeBootstrapPolicyWitness{}
	if json.Unmarshal(encoded, source) != nil {
		t.Fatal("fresh Source public evidence restart")
	}
	loginClient, err := mautrix.NewClient(cfg.Bootstrap.HomeserverURL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	loginClient.Client = cfg.HTTPClient
	login, err := loginClient.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: cfg.Bootstrap.MatrixUserID}, Password: password, DeviceID: "RESOURCE_POLICY_FRESH_B"})
	if err != nil {
		t.Fatal(err)
	}
	freshCfg := cfg
	freshCfg.StoreDir, freshCfg.Secrets = t.TempDir(), &memorySecrets{values: map[string][]byte{}}
	fresh := storageSynapseSession(ctx, t, freshCfg, credentials{Homeserver: cfg.Bootstrap.HomeserverURL, UserID: login.UserID, DeviceID: login.DeviceID, AccessToken: login.AccessToken, Subject: cfg.Subject, Tenant: cfg.Tenant, ResourceProduct: cfg.ResourceProduct})
	defer func() { _ = fresh.Close() }()
	go func() { _ = fresh.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := fresh.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fresh.Recover(ctx, recovery); err != nil {
		t.Fatal("actual canonical B root recovery", err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	creator := source.Value.Capture.Creator
	if err := fresh.RevokeDevice(ctx, creator.DeviceID, storageFixtureUIA(login.UserID, password)); err != nil {
		t.Fatal("actual producer A retirement", err)
	}
	keys, err := fresh.cli.QueryKeys(ctx, &mautrix.ReqQueryKeys{DeviceKeys: map[id.UserID]mautrix.DeviceIDList{login.UserID: {id.DeviceID(creator.DeviceID)}}})
	if err != nil || len(keys.DeviceKeys[login.UserID]) != 0 {
		t.Fatal("old A remains in actual SDK directory", err)
	}
	current, registry := nativePolicySession(ctx, t, fresh, source.Value.Capture.Source.OwnerScope, creator.RootKey, "2")
	key, planID := source.Value.Capture.Resource, source.Value.Capture.PlanID
	policy, err := fresh.RecoverResourceBootstrapPolicy(ctx, current, key, planID, source, vault, lease)
	if err != nil {
		t.Fatal("actual B fresh policy restoration needed held A authority", err)
	}
	guard, requirement, err := fresh.RecoveredResourceBootstrapRecovery(ctx, current, key, planID, policy, vault, lease)
	if err != nil {
		t.Fatal("actual B cannot reconstruct bootstrap-only recovery prerequisite", err)
	}
	if err := guard.CheckBootstrapRecoveryReady(ctx, requirement); err != nil {
		t.Fatal("actual B independent signed owner capability/anchor readback", err)
	}
	window, err := policy.Window(ctx)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := fresh.RecoverResourceBootstrapEpoch(ctx, current, source.Value.Capture.PlanSHA256, window, vault, lease)
	if err != nil {
		t.Fatal("actual B independent SSSS epoch recovery", err)
	}
	defer handle.Close()
	access, err := policy.Access(ctx, resource.Read)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := fresh.BindResourceRoom(ctx, current, access, handle, registry, lease, journal)
	if err != nil {
		t.Fatal("actual current B read-only resource binding", err)
	}
	defer reader.Close()
	if err := reader.RestoreBackup(ctx); err != nil {
		t.Fatal("actual B scoped old SDK session backup", err)
	}
	owner, err := current.Scope().JournalOwner()
	if err != nil {
		t.Fatal(err)
	}
	var baselineProof resource.VerifiedBootstrapRead
	for _, kind := range []resource.BootstrapKind{resource.BootstrapBaseline, resource.BootstrapCheckpoint} {
		proof, err := policy.Accepted(ctx, kind, access, registry)
		if err != nil {
			t.Fatal(err)
		}
		var original bootstrapLocalPublication
		found, err := journal.Read(owner, storage.ResourceBootstrapPublicationNamespace, planID+"/"+string(kind), &original)
		if err != nil || !found || original.Candidate == nil {
			t.Fatal("original private bootstrap intent missing", err)
		}
		actual, err := reader.ReadBootstrapAccepted(ctx, proof)
		exact := bytes.Equal(actual, original.Payload)
		clear(actual)
		clear(original.Payload)
		if err != nil || !exact {
			t.Fatal("actual fresh B SDK/hybrid original descriptor readback", kind, err)
		}
		if kind == resource.BootstrapBaseline {
			baselineProof = proof
		}
	}
	if count, err := journal.CountOwner(owner, storage.ResourceBootstrapPublicationNamespace); err != nil || count != 2 {
		t.Fatal("descriptor reads fabricated a bootstrap work ACK", count, err)
	}
	source.withdraw.Store(true)
	if out, err := reader.ReadBootstrapAccepted(ctx, baselineProof); err == nil || len(out) != 0 {
		t.Fatal("retained SDK bootstrap read bypassed withdrawn Original Source")
	}
	source.withdraw.Store(false)
	registry.active.Store(false)
	if out, err := reader.ReadBootstrapAccepted(ctx, baselineProof); err == nil || len(out) != 0 {
		t.Fatal("retired B retained accepted descriptor authority")
	}
	t.Log("FRESH_CANONICAL_B_RETIRED_A_PRIVATE_VAULT_PIN_ORIGINAL_BOTH_SDK_DESCRIPTORS_GREEN_NO_ACK")
}

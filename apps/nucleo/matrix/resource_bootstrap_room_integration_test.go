//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type nativeBootstrapSource struct {
	snapshot resource.BootstrapSourceSnapshot
}

func (s nativeBootstrapSource) BootstrapSource(ctx context.Context, key resource.Resource) (resource.BootstrapSourceSnapshot, error) {
	if key != s.snapshot.Resource {
		return resource.BootstrapSourceSnapshot{}, resource.ErrRevoked
	}
	return s.snapshot, ctx.Err()
}

func (s nativeBootstrapSource) CheckBootstrapSource(ctx context.Context, value resource.BootstrapSourceSnapshot) error {
	if value != s.snapshot {
		return resource.ErrRevoked
	}
	return ctx.Err()
}

type nativeBootstrapCapturePages struct {
	inputs []resource.BootstrapRosterInput
}

func (p nativeBootstrapCapturePages) BootstrapRosterPage(ctx context.Context, offset resource.Decimal) (resource.BootstrapRosterPage, error) {
	if offset != "0" {
		return resource.BootstrapRosterPage{}, resource.ErrPending
	}
	return resource.BootstrapRosterPage{Offset: offset, Inputs: p.inputs}, ctx.Err()
}

type nativeBootstrapAcceptance struct{ client *mautrix.Client }

func (s nativeBootstrapAcceptance) AcceptedEvent(ctx context.Context, ref storage.Reference) (*event.Event, error) {
	wire, err := s.client.GetEvent(ctx, id.RoomID(ref.RoomID), id.EventID(ref.EventID))
	if err != nil {
		return nil, err
	}
	if wire != nil && wire.RoomID == "" {
		wire.RoomID = id.RoomID(ref.RoomID)
	}
	return wire, nil
}

// Uses the existing REAL SDK/Synapse session; only the original source approval
// and Registry witness are synthetic, as documented on that shared fixture.
// This is crypto/WAL proof, not the still-required 0073 Origin composition proof.
type nativeBootstrapResume struct {
	plan                  resource.VerifiedBootstrapPlan
	vault                 *Client
	policy                *nativeBootstrapPolicyWitness
	key                   resource.Resource
	planID, transaction   string
	payload, candidateRaw []byte
}

func exerciseNativeResourceBootstrap(ctx context.Context, t *testing.T, c *Client, current resource.VerifiedSession, fixture resourcetest.Fixture, provider *resource.SessionProvider, lease offline.WorkLease, journal *offline.Store) *nativeBootstrapResume {
	t.Helper()
	input := nativeBootstrapInput{key: fixture.Epoch.Value().Resource, room: fixture.Epoch.Value().RoomID, validation: fixture.Epoch.Value().Validation, roster: fixture.Roster, enrolled: fixture.Enrolled}
	return exerciseNativeBootstrapInput(ctx, t, c, current, input, provider, lease, journal)
}

// The bootstrap-only fixture needs only original routing/current enrollment;
// it never creates a previous normal-operation epoch or capability.
type nativeBootstrapInput struct {
	key        resource.Resource
	room       string
	validation resource.ValidationPolicy
	roster     resource.VerifiedRoster
	enrolled   resource.VerifiedRecipient
}

func exerciseNativeBootstrapInput(ctx context.Context, t *testing.T, c *Client, current resource.VerifiedSession, input nativeBootstrapInput, provider *resource.SessionProvider, lease offline.WorkLease, journal *offline.Store) *nativeBootstrapResume {
	t.Helper()
	key := input.key
	planID := uuid.NewString()
	recipient, err := c.ResourceBootstrapRecipient(ctx, current, key, planID, lease, journal)
	if err != nil {
		t.Fatal("bootstrap recipient", err)
	}
	members := input.roster.Members()
	for i := range members {
		members[i].From = resource.Position{Seq: "0"}
	}
	roster, err := resource.VerifyRoster(key, members, []resource.VerifiedRecipient{input.enrolled})
	if err != nil {
		t.Fatal(err)
	}
	source := nativeBootstrapSource{snapshot: resource.BootstrapSourceSnapshot{Resource: key, OwnerScope: current.Scope(), SourceEpoch: uuid.NewString(), History: resource.HistoryFull, SourceSeq: "9007199254740993", ACLVersion: "1"}}
	bindingHash, err := matrixproof.RequestDigest(current.Binding())
	if err != nil {
		t.Fatal(err)
	}
	plan := resource.BootstrapPlan{Version: 1, Domain: "org.sytue.resource.bootstrap.plan.v1", ID: planID, Source: source.snapshot, RoomID: input.room, Epoch: "bootstrap-" + uuid.NewString(), Recipient: recipient, MembershipSHA256: roster.SHA256(), History: resource.HistoryFull, Validation: input.validation, CreatorScope: current.Scope(), Creator: current.Actor(), SessionBindingSHA256: bindingHash}
	var record resource.SessionRecord
	if json.Unmarshal(current.PublicRecordBytes(), &record) != nil {
		t.Fatal("current public enrollment")
	}
	manifest, inputs, err := resource.PrepareBootstrapRosterManifest(plan, []resource.BootstrapRosterInput{{Member: members[0], Recipient: record.Recipient}})
	if err != nil {
		t.Fatal(err)
	}
	signed, signedManifest, err := c.SignResourceBootstrapCapture(ctx, current, plan, manifest, lease)
	if err != nil {
		t.Fatal("native capture SDK signatures", err)
	}
	pending, err := resource.VerifyBootstrapRosterManifest(ctx, signed, signedManifest, current, source)
	if err != nil {
		t.Fatal("native capture signed plan/count/digest", err)
	}
	tampered := signedManifest
	tampered.MemberCount = "2"
	if _, err := resource.VerifyBootstrapRosterManifest(ctx, signed, tampered, current, source); err == nil {
		t.Fatal("native SDK manifest did not bind complete count")
	}
	if _, _, err := c.SignResourceBootstrapCapture(ctx, current, plan, manifest, nil); err == nil {
		t.Fatal("native capture signing dropped original lease")
	}
	verifiedPlan, err := resource.CompleteBootstrapRosterCapture(ctx, pending, nativeBootstrapCapturePages{inputs: inputs}, provider)
	if err != nil || len(verifiedPlan.CurrentRosterBindings()) != 1 {
		t.Fatal("native capture lost current roster/plan", err)
	}
	capturedRoster, err := resource.CaptureBootstrapRosterEvidence(ctx, pending)
	if err != nil {
		t.Fatal("original PRE-network public roster evidence", err)
	}
	vault, vaultFault := newBootstrapRecoveryTestVault(ctx, t, c, current.Scope())
	t.Cleanup(func() { _ = vault.Client.Close() })
	if _, err := c.BindResourceBootstrapRoom(ctx, current, verifiedPlan, c, lease, journal); err == nil {
		t.Fatal("canonical/global vault accepted for resource recovery")
	}
	exerciseNativeOwnerOnlyBootstrapCaptureDenial(t, ctx, c, current, signed, roster, source, vault.Client, lease, journal)
	room, err := c.BindResourceBootstrapRoom(ctx, current, verifiedPlan, vault.Client, lease, journal)
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	transaction := uuid.NewString()
	payload := []byte("original private bootstrap descriptor with precise source 9007199254740993")
	prepared, err := room.PrepareDescriptor(ctx, resource.BootstrapBaseline, transaction, payload, nil)
	if err != nil {
		t.Fatal("prepare baseline descriptor", err)
	}
	defer prepared.Close()
	candidate, err := prepared.Candidate()
	if err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(candidate)
	if bytes.Contains(original, payload) {
		t.Fatal("bootstrap candidate exposed plaintext")
	}
	request, err := prepared.SignAdmission(ctx)
	if err != nil || request.Namespace != "resources:docs" || request.TransactionID != transaction {
		t.Fatal("bootstrap request lost original purpose/txn", err)
	}
	if _, err := prepared.prepared.Send(ctx, nil); !errors.Is(err, ErrPublicationAdmissionRequired) {
		t.Fatal("bootstrap bypassed shared durable relay", err)
	}
	checked, err := resource.VerifyBootstrapCandidate(ctx, candidate, verifiedPlan, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture accesses the maintained SDK's transport directly. Production
	// PreparedBootstrapPublication deliberately has NO Send entry point.
	response, err := c.cli.SendMessageEvent(ctx, id.RoomID(plan.RoomID), event.EventEncrypted, json.RawMessage(candidate.EncryptedEvent), mautrix.ReqSendEvent{TransactionID: transaction})
	if err != nil {
		t.Fatal(err)
	}
	ref := storage.Reference{RoomID: plan.RoomID, EventID: response.EventID.String()}
	ack, err := resource.ConfirmBootstrapAcceptance(ctx, checked, ref, nativeBootstrapAcceptance{client: c.cli})
	if err != nil {
		t.Fatal("actual baseline ACK", err)
	}
	actual, err := c.cli.GetEvent(ctx, id.RoomID(ref.RoomID), id.EventID(ref.EventID))
	if err != nil {
		t.Fatal(err)
	}
	actual.RoomID = id.RoomID(ref.RoomID)
	if err = actual.Content.ParseRaw(actual.Type); err != nil {
		t.Fatal(err)
	}
	clearEvent, err := c.crypto.Decrypt(ctx, actual)
	if err != nil {
		t.Fatal("real bootstrap SDK decrypt", err)
	}
	frame, err := eventContentBytes(clearEvent)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(frame)
	envelope, err := resource.DecodeFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(envelope.Ciphertext)
	identity, err := c.bootstrapKey(ctx, current, key, planID, lease, journal, false)
	if err != nil {
		t.Fatal(err)
	}
	cleartext, err := resource.OpenBootstrapDescriptor(identity, checked.Context(), envelope)
	if err != nil || !bytes.Equal(cleartext, payload) {
		t.Fatal("real hybrid bootstrap descriptor did not roundtrip", err)
	}
	clear(cleartext)
	prepared.Close()
	room.Close()
	// Reconstruct the adapter only from persistent local public witness, encrypted
	// WAL and the SAME OS keychain entry; no re-encryption under its transaction.
	resumed, err := c.BindResourceBootstrapRoom(ctx, current, verifiedPlan, vault.Client, lease, journal)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	again, err := resumed.PrepareDescriptor(ctx, resource.BootstrapBaseline, transaction, payload, nil)
	if err != nil {
		t.Fatal("resume prepared baseline", err)
	}
	defer again.Close()
	repeated, _ := again.Candidate()
	repeatedRaw, _ := json.Marshal(repeated)
	if !bytes.Equal(original, repeatedRaw) {
		t.Fatal("baseline resume replaced exact SDK wire or provenance")
	}
	if _, err = resumed.PrepareDescriptor(ctx, resource.BootstrapBaseline, transaction, append(bytes.Clone(payload), '!'), nil); !errors.Is(err, resource.ErrConflict) {
		t.Fatal("same txn accepted changed original descriptor", err)
	}
	checkpoint, err := resumed.PrepareDescriptor(ctx, resource.BootstrapCheckpoint, uuid.NewString(), []byte("current-state descriptor"), &ack)
	if err != nil {
		t.Fatal("prepare checkpoint after actual baseline", err)
	}
	recoveredVault, policy := exerciseNativeBootstrapRecovery(ctx, t, c, current, verifiedPlan, capturedRoster, ack, checkpoint, vault, vaultFault, lease, journal)
	checkpoint.Close()
	return &nativeBootstrapResume{vault: recoveredVault, policy: policy, plan: verifiedPlan, key: key, planID: planID, transaction: transaction, payload: bytes.Clone(payload), candidateRaw: original}
}

func exerciseNativeBootstrapAfterSDKRestart(ctx context.Context, t *testing.T, c *Client, current resource.VerifiedSession, lease offline.WorkLease, journal *offline.Store, saved *nativeBootstrapResume) {
	t.Helper()
	defer clear(saved.payload)
	room, err := c.BindResourceBootstrapRoom(ctx, current, saved.plan, saved.vault, lease, journal)
	if err != nil {
		t.Fatal("bootstrap bind after SDK/SQLite reopen", err)
	}
	defer room.Close()
	prepared, err := room.PrepareDescriptor(ctx, resource.BootstrapBaseline, saved.transaction, saved.payload, nil)
	if err != nil {
		t.Fatal("bootstrap WAL after SDK/SQLite reopen", err)
	}
	defer prepared.Close()
	candidate, err := prepared.Candidate()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(candidate)
	if !bytes.Equal(saved.candidateRaw, raw) {
		t.Fatal("SDK restart replaced immutable bootstrap wire")
	}
	secret := c.bootstrapKeyName(current.Actor().DeviceID, saved.planID)
	if err := c.cfg.Secrets.Write(secret, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = c.ResourceBootstrapRecipient(ctx, current, saved.key, saved.planID, lease, journal); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("lost witnessed private epoch key was silently regenerated", err)
	}
	remaining, err := c.cfg.Secrets.Read(secret)
	if err != nil || len(remaining) != 0 {
		clear(remaining)
		t.Fatal("failed witnessed-key recovery wrote a replacement private identity", err)
	}
}

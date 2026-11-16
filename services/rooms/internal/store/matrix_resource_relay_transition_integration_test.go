//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
	docsdb "git.sytue.eu/sytue/one/services/docs/db"
	matrixDirectory "git.sytue.eu/sytue/one/services/rooms/internal/matrix"
)

var errRegistryRelaySessionLost = errors.New("relay discarded current Provider device B")

type registryRelayGate struct {
	tb            testing.TB
	provider      *resource.SessionProvider
	currentDevice string
	initial       resourcetest.Fixture
	next          resource.VerifiedEpoch
	nextCap       resource.VerifiedCapability
	nextRoster    resource.VerifiedRoster
	chainB        *matrixprooftest.Fixture
}

func (g *registryRelayGate) Access(ctx context.Context, key resource.Resource, action resource.Action, raw json.RawMessage) (resource.VerifiedAccess, error) {
	var signed resource.SignedEpoch
	if json.Unmarshal(raw, &signed) != nil {
		return resource.VerifiedAccess{}, resource.ErrInvalid
	}
	hash, err := resource.EpochDigest(signed.Epoch)
	if err != nil {
		return resource.VerifiedAccess{}, err
	}
	epoch, cap := g.initial.Epoch, g.initial.Cap
	if hash != epoch.SHA256() {
		epoch, cap = g.next, g.nextCap
	}
	if key != epoch.Value().Resource || hash != epoch.SHA256() {
		return resource.VerifiedAccess{}, resource.ErrInvalid
	}
	session, err := g.provider.Resolve(ctx, g.initial.Scope, g.currentDevice)
	if err != nil {
		return resource.VerifiedAccess{}, err
	}
	return resource.AuthorizeSession(ctx, epoch, cap, session, action)
}

func (g *registryRelayGate) InitialEpoch(ctx context.Context, value resource.SignedEpoch) (resource.VerifiedEpoch, error) {
	return resource.VerifyInitialEpoch(ctx, value, g.initial.Roster, g.initial.Device, g.initial.Anchor)
}

func (g *registryRelayGate) NextEpoch(ctx context.Context, value resource.SignedEpoch, previous resource.VerifiedAccess, cutover resource.Position) (resource.VerifiedEpoch, error) {
	current, err := g.provider.Resolve(ctx, g.initial.Scope, g.currentDevice)
	if err != nil {
		return resource.VerifiedEpoch{}, err
	}
	binding, ok := previous.CurrentSessionBinding()
	if !ok || binding != current.Binding() || previous.Actor() != current.Actor() {
		return resource.VerifiedEpoch{}, errRegistryRelaySessionLost
	}
	next, err := resource.VerifyNextEpochWithSession(ctx, value, g.nextRoster, current.Device(), previous.Epoch(), previous.Capability(), cutover, current)
	if err != nil {
		return resource.VerifiedEpoch{}, err
	}
	signed := g.initial.SignedCap
	signed.Epoch = value.ID
	signed.Generation = value.Generation
	signed.PolicySHA256 = next.SHA256()
	signed.MembershipSHA256 = value.MembershipSHA256
	signed.ProductKeyID = current.Binding().ProductKeyID
	signed.Issuer = current.Actor()
	signed.Signatures = matrixprooftest.Sign(g.tb, g.chainB.Device, signed, g.chainB.Origin.UserID, current.Actor().DeviceID)
	cap, err := resource.VerifyCapability(ctx, signed, next, current.Device())
	if err != nil {
		return resource.VerifiedEpoch{}, err
	}
	g.next, g.nextCap = next, cap
	return next, nil
}

func (*registryRelayGate) Producer(context.Context, resource.Candidate, resource.VerifiedAccess) (resource.VerifiedPendingPublication, error) {
	return resource.VerifiedPendingPublication{}, resource.ErrInvalid
}

func (*registryRelayGate) Complete(context.Context, resource.Candidate, resource.VerifiedPendingPublication) (resource.VerifiedPublication, error) {
	return resource.VerifiedPublication{}, resource.ErrInvalid
}

func (*registryRelayGate) VerifyAcceptance(context.Context, resource.Candidate, resource.VerifiedPublication, storage.Reference) (resource.VerifiedAcceptance, error) {
	return resource.VerifiedAcceptance{}, resource.ErrInvalid
}

func (*registryRelayGate) VerifyReservedAcceptance(context.Context, resource.VerifiedReservation, storage.Reference) (resource.VerifiedAcceptance, error) {
	return resource.VerifiedAcceptance{}, resource.ErrInvalid
}

func (*registryRelayGate) VerifyNotAccepted(context.Context, resource.JournalEntry, json.RawMessage) error {
	return resource.ErrInvalid
}

type registryRelayFence struct{ session resource.VerifiedSession }

func (f registryRelayFence) Revalidate(ctx context.Context, a resource.VerifiedAccess) error {
	binding, ok := a.CurrentSessionBinding()
	if !ok || binding != f.session.Binding() {
		return resource.ErrRevoked
	}
	return f.session.Revalidate(ctx)
}

// This composes the real Registry/Directory with a SECOND real product database
// and the ONE resourcepg Relay. The checkpoint bytes are a device fixture and
// encrypted locally; no application plaintext or private key enters either DB.
// Matrix message publication/native UI and Office source migration remain their
// separate required integration proofs; this specifically verifies durable Rotate.
func prepareRegistryRelayFreshB(t *testing.T, ctx context.Context, directory *matrixDirectory.Client, provider *resource.SessionProvider, a, b resource.VerifiedSession, chainA, chainB *matrixprooftest.Fixture, aLeaf, bLeaf *pqc.Identity, retireB func() error) func() {
	t.Helper()
	origin, err := directory.ResourceSessionOrigin(ctx, a.Scope(), a.Actor().DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(&event.EncryptedEventContent{Algorithm: id.AlgorithmMegolmV1, SessionID: id.SessionID(chainA.Master.PublicKey()), SenderKey: id.Curve25519(chainA.Self.PublicKey()), DeviceID: origin.DeviceID, MegolmCiphertext: []byte("opaque relay metadata fixture")})
	if err != nil {
		t.Fatal(err)
	}
	f := resourcetest.NewWithSDKRecipient(t, true, origin, id.Ed25519(a.Actor().RootKey), func(value any) signatures.Signatures {
		return matrixprooftest.Sign(t, chainA.Device, value, origin.UserID, origin.DeviceID.String())
	}, "!resource-registry-relay", raw, aLeaf)
	if f.Enrolled.SHA256() != a.Binding().EnrollmentSHA256 {
		t.Fatal("fixture did not use A actual Registry leaf")
	}
	h := pgtest.New(t, docsdb.Migrations, docsdb.MigrationsDir)
	gate := &registryRelayGate{tb: t, provider: provider, currentDevice: a.Actor().DeviceID, initial: f, chainB: chainB}
	relay, err := resourcepg.New(resourcepg.Dependencies{TenantTx: h.Pool.InTenantTx, ValidateResource: func(ctx context.Context, key resource.Resource) error {
		if key != f.Epoch.Value().Resource {
			return resource.ErrInvalid
		}
		return ctx.Err()
	}, Notify: func(ctx context.Context, _ pgx.Tx, _ resource.Receipt) error { return ctx.Err() }, RequirePrivateRules: true}, gate, f.Scope.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if err := relay.Bootstrap(ctx, f.SignedEpoch); err != nil {
		t.Fatal("resource product bootstrap", err)
	}
	return func() {
		gate.currentDevice = b.Actor().DeviceID
		if err := relay.Drain(ctx, f.Epoch.Value().Resource); err != nil {
			t.Fatal("B could not drain historical role A", err)
		}
		manager, err := resource.AuthorizeSession(ctx, f.Epoch, f.Cap, b, resource.Manage)
		if err != nil {
			t.Fatal(err)
		}
		epochKey, err := pqc.GenerateIdentity()
		if err != nil {
			t.Fatal(err)
		}
		next := f.SignedEpoch
		next.ID = "epoch_device_b"
		next.Generation = "2"
		next.PreviousEpoch = f.Epoch.Value().ID
		next.PreviousPolicySHA256 = f.Epoch.SHA256()
		next.Issuer = b.Actor()
		next.Recipient = epochKey.Recipient().String()
		member := f.Cap.Member()
		member.Recipient = b.Binding().Recipient
		member.ProductKeyID = b.Binding().ProductKeyID
		member.EnrollmentSHA256 = b.Recipient().SHA256()
		gate.nextRoster, err = resource.VerifyRoster(next.Resource, []resource.Member{member}, []resource.VerifiedRecipient{b.Recipient()})
		if err != nil {
			t.Fatal(err)
		}
		next.MembershipSHA256 = gate.nextRoster.SHA256()
		checkpoint, err := resource.PrepareCheckpointContext(manager, next.ID, next.Recipient, next.Cutover)
		if err != nil {
			t.Fatal(err)
		}
		if checkpoint.Creator != b.Actor() {
			t.Fatal("checkpoint impersonated historical A")
		}
		payload := []byte("{\"seq\":9007199254740993,\"content\":\"original authorized checkpoint fixture\"}")
		envelope, err := resource.SealBootstrapDescriptor(epochKey.Recipient(), checkpoint, payload)
		if err != nil {
			t.Fatal(err)
		}
		contextSHA, err := resource.BootstrapContextSHA256(checkpoint)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		cipherSHA, err := resource.CipherJSONDigest(encoded)
		if err != nil {
			t.Fatal(err)
		}
		next.Checkpoint = resource.Checkpoint{SourceCutover: next.Cutover, Reference: storage.Reference{RoomID: next.RoomID, EventID: "$checkpoint-b"}, CipherSHA256: cipherSHA, ContextSHA256: contextSHA, SessionCertificateID: "checkpoint-b-certificate"}
		next.Signatures = matrixprooftest.Sign(t, chainB.Device, next, chainB.Origin.UserID, b.Actor().DeviceID)
		if err := relay.Rotate(ctx, next); err != nil {
			t.Fatal("durable Rotate lost current Provider B", err)
		}
		var stored []byte
		var seq, baseline int64
		if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT e.signed_policy,r.seq,r.baseline_seq FROM resource_e2e_resources r JOIN resource_e2e_epochs e USING(tenant,product,kind,resource_id,epoch) WHERE r.tenant=$1 AND e.tenant=$1 AND r.product=$2 AND r.kind=$3 AND r.resource_id=$4`, next.Resource.Tenant, next.Resource.Product, next.Resource.Kind, next.Resource.ID).Scan(&stored, &seq, &baseline)
		}); err != nil {
			t.Fatal(err)
		}
		var policy resource.SignedEpoch
		if json.Unmarshal(stored, &policy) != nil || policy.Issuer != b.Actor() || policy.Checkpoint != next.Checkpoint || policy.Baseline != f.SignedEpoch.Baseline || seq != 9007199254740993 || baseline != seq {
			t.Fatal("durable epoch relabelled actor or lost exact history anchor")
		}
		actual, err := resource.OpenBootstrapDescriptor(epochKey, checkpoint, envelope)
		if err != nil || !bytes.Equal(actual, payload) {
			t.Fatal("new checkpoint changed original device bytes", err)
		}
		clear(actual)
		epochWindow, err := resource.CurrentEpoch(gate.next)
		if err != nil {
			t.Fatal(err)
		}
		handle, err := resource.BindOwnedEpoch(epochWindow, epochKey)
		if err != nil {
			t.Fatal(err)
		}
		defer handle.Close()
		nextManager, err := resource.AuthorizeSession(ctx, gate.next, gate.nextCap, b, resource.Manage)
		if err != nil {
			t.Fatal(err)
		}
		nextReader, err := resource.AuthorizeSession(ctx, gate.next, gate.nextCap, b, resource.Read)
		if err != nil {
			t.Fatal(err)
		}
		grant, err := handle.PrepareGrant(ctx, nextManager, gate.nextCap, registryRelayFence{b})
		if err != nil {
			t.Fatal(err)
		}
		contextBytes, err := matrixproof.CanonicalBytes(grant.Context)
		if err != nil {
			t.Fatal(err)
		}
		if leaked, err := pqc.Open(aLeaf, contextBytes, grant.Envelope); err == nil || leaked != nil {
			clear(leaked)
			t.Fatal("retired A private leaf opened B epoch grant")
		}
		if chainA.Master != chainB.Master || chainA.Self != chainB.Self {
			t.Fatal("fixture lost A knowledge of global Matrix secrets")
		}
		signedGrant := resource.SignedEpochGrant{EpochGrant: grant}
		signedGrant.Signatures = matrixprooftest.Sign(t, chainB.Device, signedGrant, chainB.Origin.UserID, b.Actor().DeviceID)
		opened, err := resource.OpenGrant(ctx, signedGrant, epochWindow, nextManager, nextReader, bLeaf, b.Device(), registryRelayFence{b})
		if err != nil {
			t.Fatal(err)
		}
		opened.Close()
		if err := retireB(); err != nil {
			t.Fatal(err)
		}
		if err := relay.Drain(ctx, next.Resource); err == nil {
			t.Fatal("withdrawn B still managed durable relay")
		}
		if strings.Contains(string(stored), string(payload)) {
			t.Fatal("relay persisted checkpoint plaintext")
		}
		t.Log("real Registry Provider B survives resourcepg reauthorization and commits signed B epoch/checkpoint under exact >2^53 baseline; retired A cannot decrypt next grant despite global Matrix secrets")
	}
}

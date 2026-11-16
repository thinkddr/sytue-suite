// SPDX-License-Identifier: Apache-2.0
package resourcetest

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// SessionSource is an artificial registry witness using real Go SDK signatures.
// It does not replace the PostgreSQL/consumed-consent/current-directory proofs.
type SessionSource struct {
	Record  resource.SessionRecord
	Chain   *matrixprooftest.Fixture
	retired atomic.Bool
}

func (s *SessionSource) CurrentResourceSession(ctx context.Context, scope storage.Scope, device string) (resource.SessionRecord, error) {
	if s.retired.Load() || scope != s.Record.Binding.Scope || device != s.Record.Binding.DeviceID {
		return resource.SessionRecord{}, resource.ErrRevoked
	}
	return s.Record, ctx.Err()
}

func (s *SessionSource) CheckResourceSession(ctx context.Context, b resource.SessionBinding) error {
	if s.retired.Load() || b != s.Record.Binding {
		return resource.ErrRevoked
	}
	return ctx.Err()
}

func (s *SessionSource) ResourceSessionDevice(ctx context.Context, scope storage.Scope, device, root string) (matrixproof.Device, error) {
	if s.retired.Load() || scope != s.Chain.Origin.Scope || device != s.Chain.Origin.DeviceID.String() {
		return matrixproof.Device{}, resource.ErrRevoked
	}
	return matrixproof.VerifyUser(ctx, s.Chain.Origin, s.Chain.Origin.UserID, id.Ed25519(root))
}

// A test registry witness exercises only the public production Provider path.
// Actual durable purpose/lifecycle/RLS is verified separately against PostgreSQL.
func EnrolledSession(t testing.TB, chain *matrixprooftest.Fixture, key *pqc.Identity) (resource.VerifiedSession, *SessionSource) {
	t.Helper()
	scope := chain.Origin.Scope
	actor := resource.Actor{UserID: chain.Origin.UserID.String(), DeviceID: chain.Origin.DeviceID.String(), RootKey: chain.Master.PublicKey().String()}
	keyID, _ := pqc.RecipientID(key.Recipient())
	recipient := resource.SignedRecipient{RecipientEnrollment: resource.RecipientEnrollment{Version: 1, Domain: "org.sytue.resource.recipient.v1", Scope: scope, UserID: actor.UserID, Recipient: key.Recipient().String(), KeyID: keyID, Issuer: actor}}
	recipient.Signatures = matrixprooftest.Sign(t, chain.Device, recipient, chain.Origin.UserID, actor.DeviceID)
	device, err := matrixproof.VerifyUser(t.Context(), chain.Origin, chain.Origin.UserID, chain.Master.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	verified, err := resource.VerifyAccountRecipient(t.Context(), recipient, device)
	if err != nil {
		t.Fatal(err)
	}
	binding := resource.SessionBinding{Scope: scope, Namespace: "resources:" + scope.Product, UserID: actor.UserID, DeviceID: actor.DeviceID, RootKey: actor.RootKey, Recipient: recipient.Recipient, ProductKeyID: keyID, EnrollmentSHA256: verified.SHA256(), Generation: "1", RootGeneration: "1", AccountVersion: "0", TenantVersion: "0", ConsentID: uuid.NewString()}
	now := time.Now()
	challenge, err := resource.NewSessionChallenge(t.Context(), binding, false, now.Add(time.Minute), now)
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := resource.OpenSessionChallenge(t.Context(), challenge, key, now)
	if err != nil {
		t.Fatal(err)
	}
	unsigned, err := resource.PrepareSessionEnrollment(challenge, nonce, "retain_confirmed_root", now)
	if err != nil {
		t.Fatal(err)
	}
	proof := resource.SignedSessionEnrollment{SessionEnrollment: unsigned}
	proof.Signatures = matrixprooftest.Sign(t, chain.Device, proof, chain.Origin.UserID, actor.DeviceID)
	enrollment, err := resource.VerifySessionEnrollment(t.Context(), proof, recipient, challenge, device, now)
	if err != nil {
		t.Fatal(err)
	}
	source := &SessionSource{Record: resource.SessionRecord{Binding: binding, PinnedRoot: actor.RootKey, RootGeneration: "1", Recipient: recipient, Challenge: challenge, Proof: proof, ProofSHA256: enrollment.SHA256(), ConfirmedAt: now}, Chain: chain}
	provider, err := resource.NewSessionProvider(source, source)
	if err != nil {
		t.Fatal(err)
	}
	current, err := provider.Resolve(t.Context(), scope, actor.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	return current, source
}

func (s *SessionSource) Retire() { s.retired.Store(true) }

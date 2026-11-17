// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

func validResourceIdentity(cfg Config) bool {
	subject, err := uuid.Parse(cfg.Subject)
	local, _, parseErr := id.UserID(cfg.Bootstrap.MatrixUserID).Parse()
	b := cfg.Bootstrap
	return err == nil && subject != uuid.Nil && subject.String() == cfg.Subject && parseErr == nil && local == "sytue_"+cfg.Subject && storage.Product(cfg.ResourceProduct) && cfg.NativeConsumer == "" && b.StorageProduct == "" && b.StorageIdentityVersion == 0 && b.LegacyStorage == nil && b.StorageRoomID == "" && len(b.StorageRoomIDs) == 0 && b.DraftRoomID == "" && len(b.DraftRoomIDs) == 0 && len(b.Channels) == 0
}

func (c *Client) resourceScope() storage.Scope {
	return storage.Scope{Tenant: c.cfg.Tenant, Subject: c.cfg.Subject, Product: c.cfg.ResourceProduct}
}

func (c *Client) resourceLeafKey(device id.DeviceID) string {
	return "matrix-resource-device-leaf-" + matrixAccountName(c.account+"\x00"+device.String())
}

// Product leaf keys live ONLY in the OS keychain for this exact SDK device.
// Global Matrix SSSS secrets cannot reveal another device's hybrid private leaf.
func (c *Client) resourceLeaf(create bool) (*pqc.Identity, error) {
	c.resourceLeafMu.Lock()
	defer c.resourceLeafMu.Unlock()
	if c.cfg.ResourceProduct == "" || c.session.DeviceID == "" {
		return nil, ErrIdentity
	}
	key := c.resourceLeafKey(c.session.DeviceID)
	raw, err := c.cfg.Secrets.Read(key)
	if err != nil {
		return nil, ErrSecureStore
	}
	defer clear(raw)
	if len(raw) == 0 {
		if !create {
			return nil, ErrRecoveryRequired
		}
		identity, err := pqc.GenerateIdentity()
		if err != nil {
			return nil, err
		}
		encoded := []byte(identity.String())
		defer clear(encoded)
		if err := c.cfg.Secrets.Write(key, encoded); err != nil {
			return nil, ErrSecureStore
		}
		return identity, nil
	}
	identity, err := pqc.ParseIdentity(string(raw))
	if err != nil || identity.Profile() != pqc.HybridProfile {
		return nil, ErrSecureStore
	}
	return identity, nil
}

func resourceLease(ctx context.Context, lease offline.WorkLease) (func(), error) {
	if lease == nil {
		return nil, offline.ErrWorkLease
	}
	release, err := lease(ctx)
	if err != nil {
		return nil, err
	}
	if release == nil {
		return nil, offline.ErrWorkLease
	}
	if err := ctx.Err(); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

// The caller holds stateMu. Root equality is checked against the maintained
// SDK's verified account, independently of any server-returned record or pin.
func (c *Client) resourceSigningActor(ctx context.Context) (resource.Actor, error) {
	if !c.ready.Load() {
		return resource.Actor{}, ErrNotReady
	}
	if c.closed.Load() || c.crypto == nil || c.cfg.ResourceProduct == "" || c.session.ResourceProduct != c.cfg.ResourceProduct || !c.resourceScope().Valid() {
		return resource.Actor{}, ErrIdentity
	}
	status, err := c.verificationStatus(ctx)
	if err != nil {
		return resource.Actor{}, err
	}
	if !status.Verified {
		return resource.Actor{}, ErrUnverified
	}
	keys, err := c.crypto.Machine().GetOwnCrossSigningPublicKeys(ctx)
	if err != nil {
		return resource.Actor{}, err
	}
	if keys == nil {
		return resource.Actor{}, ErrUnverified
	}
	return resource.Actor{UserID: c.session.UserID.String(), DeviceID: c.session.DeviceID.String(), RootKey: keys.MasterKey.String()}, nil
}

func (c *Client) signResourceJSON(ctx context.Context, value any) (signatures.Signatures, error) {
	signature, err := c.crypto.Machine().GetAccount().SignJSON(value)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return signatures.NewSingleSignature(c.session.UserID, id.KeyAlgorithmEd25519, c.session.DeviceID.String(), signature), nil
}

type resourceHostSession struct {
	client *Client
	rooms  *api.Rooms
	lease  offline.WorkLease
}

func (s resourceHostSession) CurrentResourceSession(ctx context.Context, scope storage.Scope, device string) (resource.SessionRecord, error) {
	release, err := resourceLease(ctx, s.lease)
	if err != nil {
		return resource.SessionRecord{}, err
	}
	defer release()
	if s.client.closed.Load() {
		return resource.SessionRecord{}, ErrClosed
	}
	if !s.client.ready.Load() {
		return resource.SessionRecord{}, ErrNotReady
	}
	if scope != s.client.resourceScope() || device != s.client.DeviceID() {
		return resource.SessionRecord{}, ErrIdentity
	}
	record, err := s.rooms.MatrixResourceSession(ctx, scope.Product, device)
	if err != nil {
		return resource.SessionRecord{}, err
	}
	if s.client.closed.Load() {
		return resource.SessionRecord{}, ErrClosed
	}
	if !s.client.ready.Load() {
		return resource.SessionRecord{}, ErrNotReady
	}
	return record, nil
}

func (s resourceHostSession) CheckResourceSession(ctx context.Context, b resource.SessionBinding) error {
	value, err := s.CurrentResourceSession(ctx, b.Scope, b.DeviceID)
	if err != nil {
		return err
	}
	if value.Binding != b {
		return ErrIdentity
	}
	return ctx.Err()
}

func (s resourceHostSession) ResourceSessionDevice(ctx context.Context, scope storage.Scope, device, root string) (matrixproof.Device, error) {
	release, err := resourceLease(ctx, s.lease)
	if err != nil {
		return matrixproof.Device{}, err
	}
	defer release()
	c := s.client
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	actor, err := c.resourceSigningActor(ctx)
	if err != nil {
		return matrixproof.Device{}, err
	}
	if scope != c.resourceScope() || actor.DeviceID != device || actor.RootKey != root {
		return matrixproof.Device{}, ErrIdentity
	}
	who, err := c.cli.Whoami(ctx)
	if err != nil {
		return matrixproof.Device{}, err
	}
	if who.UserID != c.session.UserID || who.DeviceID != c.session.DeviceID {
		return matrixproof.Device{}, ErrIdentity
	}
	keys, err := c.cli.QueryKeys(ctx, &mautrix.ReqQueryKeys{DeviceKeys: mautrix.DeviceKeysRequest{c.session.UserID: mautrix.DeviceIDList{c.session.DeviceID}}})
	if err != nil {
		return matrixproof.Device{}, err
	}
	_, server, err := c.session.UserID.Parse()
	if err != nil {
		return matrixproof.Device{}, ErrIdentity
	}
	origin := matrixproof.Origin{Scope: scope, ServerName: server, UserID: c.session.UserID, DeviceID: c.session.DeviceID, AccountActive: true, DeviceActive: true, Keys: *keys}
	return matrixproof.VerifyUser(ctx, origin, c.session.UserID, id.Ed25519(root))
}

func (c *Client) ResourceSessionProvider(rooms *api.Rooms, lease offline.WorkLease) (*resource.SessionProvider, error) {
	if c == nil || rooms == nil || lease == nil || c.cfg.ResourceProduct == "" {
		return nil, ErrIdentity
	}
	source := resourceHostSession{client: c, rooms: rooms, lease: lease}
	return resource.NewSessionProvider(source, source)
}

// ConfirmResourceRoot must present this SDK root/product/device to the user.
// Returning true is an explicit decision; silently confirming is not consent.
type ConfirmResourceRoot func(context.Context, resource.SessionBinding) (bool, error)

// EnrollResourceSession consumes only the purpose retained by LoginNative.
// It generates a fresh local leaf, signs the exact Go nonce transcript with the
// verified SDK, and resolves the final Provider again before exposing a session.
func resourceWithLease(ctx context.Context, lease offline.WorkLease, fn func() error) error {
	release, err := resourceLease(ctx, lease)
	if err != nil {
		return err
	}
	defer release()
	return fn()
}

//nolint:gocyclo // enrollment is an ordered chain of sequential fail-closed verification steps of security-critical code; splitting them would scatter the check order
func (c *Client) EnrollResourceSession(ctx context.Context, rooms *api.Rooms, lease offline.WorkLease, confirmRoot ConfirmResourceRoot) (resource.VerifiedSession, error) {
	provider, err := c.ResourceSessionProvider(rooms, lease)
	if err != nil {
		return resource.VerifiedSession{}, err
	}
	var actor resource.Actor
	err = resourceWithLease(ctx, lease, func() error {
		c.stateMu.RLock()
		defer c.stateMu.RUnlock()
		var err error
		actor, err = c.resourceSigningActor(ctx)
		return err
	})
	if err != nil {
		return resource.VerifiedSession{}, err
	}
	if existing, err := provider.Resolve(ctx, c.resourceScope(), actor.DeviceID); err == nil {
		if err := c.verifyExistingResourceSession(ctx, lease, actor, existing); err != nil {
			return resource.VerifiedSession{}, err
		}
		return existing, nil
	} else {
		var missing *api.Error
		if !errors.As(err, &missing) || (missing.Status != http.StatusForbidden && missing.Status != http.StatusNotFound) {
			return resource.VerifiedSession{}, err
		}
	}
	c.mu.Lock()
	grant := c.resourceGrant
	c.mu.Unlock()
	if grant == nil || grant.Product != c.cfg.ResourceProduct || grant.Namespace != "resources:"+c.cfg.ResourceProduct || grant.Tenant != c.cfg.Tenant || grant.LoginToken != "" || len(grant.Grant) < 32 {
		return resource.VerifiedSession{}, ErrLoginRequired
	}
	identity, recipient, err := c.prepareSignedRecipient(ctx, lease, actor)
	if err != nil {
		return resource.VerifiedSession{}, err
	}
	var challenge resource.SessionChallenge
	err = resourceWithLease(ctx, lease, func() error {
		if c.closed.Load() {
			return ErrClosed
		}
		var err error
		challenge, err = rooms.MatrixResourceSessionChallenge(ctx, c.cfg.ResourceProduct, grant.Grant, recipient)
		return err
	})
	if err != nil {
		return resource.VerifiedSession{}, err
	}
	if challenge.Binding.Scope != c.resourceScope() || challenge.Binding.Actor() != actor || challenge.Binding.Recipient != recipient.Recipient || challenge.Binding.ProductKeyID != recipient.KeyID || challenge.Binding.ConsentID != grant.ConsentID {
		return resource.VerifiedSession{}, ErrIdentity
	}
	decision, err := confirmRootDecision(ctx, challenge, confirmRoot)
	if err != nil {
		return resource.VerifiedSession{}, err
	}
	proof, err := c.signSessionEnrollmentProof(ctx, lease, actor, challenge, identity, decision)
	if err != nil {
		return resource.VerifiedSession{}, err
	}
	var record resource.SessionRecord
	err = resourceWithLease(ctx, lease, func() error {
		if c.closed.Load() {
			return ErrClosed
		}
		var err error
		record, err = rooms.ConfirmMatrixResourceSession(ctx, c.cfg.ResourceProduct, proof)
		return err
	})
	if err != nil {
		return resource.VerifiedSession{}, err
	}
	if record.Binding != challenge.Binding {
		return resource.VerifiedSession{}, ErrIdentity
	}
	verified, err := provider.Resolve(ctx, c.resourceScope(), actor.DeviceID)
	if err != nil {
		return resource.VerifiedSession{}, err
	}
	c.mu.Lock()
	if c.resourceGrant == grant {
		c.resourceGrant = nil
	}
	c.mu.Unlock()
	return verified, nil
}

func (c *Client) verifyExistingResourceSession(ctx context.Context, lease offline.WorkLease, actor resource.Actor, existing resource.VerifiedSession) error {
	return resourceWithLease(ctx, lease, func() error {
		c.stateMu.RLock()
		defer c.stateMu.RUnlock()
		current, err := c.resourceSigningActor(ctx)
		if err != nil {
			return err
		}
		if current != actor {
			return ErrIdentity
		}
		identity, err := c.resourceLeaf(false)
		if err != nil {
			return err
		}
		keyID, err := pqc.RecipientID(identity.Recipient())
		if err != nil || keyID != existing.Binding().ProductKeyID || identity.Recipient().String() != existing.Binding().Recipient {
			return ErrRecoveryRequired
		}
		return nil
	})
}

func (c *Client) prepareSignedRecipient(ctx context.Context, lease offline.WorkLease, actor resource.Actor) (*pqc.Identity, resource.SignedRecipient, error) {
	var identity *pqc.Identity
	var recipient resource.SignedRecipient
	err := resourceWithLease(ctx, lease, func() error {
		c.stateMu.RLock()
		defer c.stateMu.RUnlock()
		current, err := c.resourceSigningActor(ctx)
		if err != nil {
			return err
		}
		if current != actor {
			return ErrIdentity
		}
		identity, err = c.resourceLeaf(true)
		if err != nil {
			return err
		}
		keyID, err := pqc.RecipientID(identity.Recipient())
		if err != nil {
			return err
		}
		recipient = resource.SignedRecipient{RecipientEnrollment: resource.RecipientEnrollment{Version: 1, Domain: "org.sytue.resource.recipient.v1", Scope: c.resourceScope(), UserID: actor.UserID, Recipient: identity.Recipient().String(), KeyID: keyID, Issuer: actor}}
		recipient.Signatures, err = c.signResourceJSON(ctx, recipient)
		return err
	})
	return identity, recipient, err
}

func confirmRootDecision(ctx context.Context, challenge resource.SessionChallenge, confirmRoot ConfirmResourceRoot) (string, error) {
	if !challenge.RequiresRootConfirmation {
		return "retain_confirmed_root", nil
	}
	if confirmRoot == nil {
		return "", ErrUnverified
	}
	approved, err := confirmRoot(ctx, challenge.Binding)
	if err != nil {
		return "", err
	}
	if !approved {
		return "", api.ErrOAuthDenied
	}
	return "confirm_current_root", nil
}

func (c *Client) signSessionEnrollmentProof(ctx context.Context, lease offline.WorkLease, actor resource.Actor, challenge resource.SessionChallenge, identity *pqc.Identity, decision string) (resource.SignedSessionEnrollment, error) {
	var proof resource.SignedSessionEnrollment
	err := resourceWithLease(ctx, lease, func() error {
		c.stateMu.RLock()
		defer c.stateMu.RUnlock()
		current, err := c.resourceSigningActor(ctx)
		if err != nil {
			return err
		}
		if current != actor {
			return ErrIdentity
		}
		nonce, err := resource.OpenSessionChallenge(ctx, challenge, identity, time.Now())
		if err != nil {
			return err
		}
		unsigned, err := resource.PrepareSessionEnrollment(challenge, nonce, decision, time.Now())
		if err != nil {
			return err
		}
		proof = resource.SignedSessionEnrollment{SessionEnrollment: unsigned}
		proof.Signatures, err = c.signResourceJSON(ctx, proof)
		return err
	})
	return proof, err
}

var (
	_ resource.SessionRegistry  = resourceHostSession{}
	_ resource.SessionDirectory = resourceHostSession{}
)

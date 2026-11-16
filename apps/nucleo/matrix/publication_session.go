// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"

	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

// PublicationSession has no public constructor from JSON. The isolated-storage
// factory verifies the SDK/root/scope. A resources factory must be supplied by
// the independently enrolled Product SessionProvider; legacy identity is not a
// fallback. Namespace is never selected from a request passed to a signer.
type PublicationSession struct {
	owner      *Client
	scope      storage.Scope
	namespace  string
	generation int64
	user       id.UserID
	device     id.DeviceID
	root       id.Ed25519
	lease      offline.WorkLease
	validate   func(context.Context) error
}

func (s *PublicationSession) Scope() storage.Scope {
	if s == nil {
		return storage.Scope{}
	}
	return s.scope
}

func (s *PublicationSession) Namespace() string {
	if s == nil {
		return ""
	}
	return s.namespace
}

func (s *PublicationSession) Generation() int64 {
	if s == nil {
		return 0
	}
	return s.generation
}

// StoragePublicationSession retains a persistent scope witness and requires the
// exact isolated storage identity plus an authority root equal to the SDK's own
// verified root. A token-only or canonical human legacy session cannot mint it.
func (c *Client) StoragePublicationSession(ctx context.Context, expected storage.Scope, authority matrixproof.Authority, lease offline.WorkLease) (*PublicationSession, error) {
	if lease == nil || !expected.Valid() || authority.RequireActive(expected, authority.Generation) != nil {
		return nil, ErrIdentity
	}
	release, err := lease(ctx)
	if err != nil {
		return nil, err
	}
	if release == nil {
		return nil, offline.ErrWorkLease
	}
	defer release()
	if _, err := c.PrepareStorage(ctx, expected); err != nil {
		return nil, err
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.cfg.Bootstrap.StorageIdentityVersion != storage.IsolatedIdentity || c.storageScope() != expected || c.session.UserID.String() != authority.UserID {
		return nil, ErrIdentity
	}
	status, err := c.verificationStatus(ctx)
	if err != nil {
		return nil, err
	}
	if !status.Verified {
		return nil, ErrUnverified
	}
	keys, err := c.crypto.Machine().GetOwnCrossSigningPublicKeys(ctx)
	if err != nil {
		return nil, err
	}
	if keys == nil || keys.MasterKey.String() != authority.RootKey {
		return nil, ErrIdentity
	}
	return &PublicationSession{owner: c, scope: expected, namespace: "storage:" + expected.Product, generation: authority.Generation, user: c.session.UserID, device: c.session.DeviceID, root: keys.MasterKey, lease: lease}, nil
}

// ResourcePublicationSession accepts only the current Provider token and this
// resource product's own verified SDK root/device plus persistent local lease.
func (c *Client) ResourcePublicationSession(ctx context.Context, current resource.VerifiedSession, lease offline.WorkLease) (*PublicationSession, error) {
	if c == nil || lease == nil || c.cfg.ResourceProduct == "" {
		return nil, ErrIdentity
	}
	binding := current.Binding()
	if !binding.Valid() || binding.Scope != c.resourceScope() || binding.Namespace != "resources:"+c.cfg.ResourceProduct || binding.UserID != c.cfg.Bootstrap.MatrixUserID || binding.DeviceID != c.DeviceID() {
		return nil, ErrIdentity
	}
	if err := current.Revalidate(ctx); err != nil {
		return nil, err
	}
	generation, err := binding.Generation.Int64()
	if err != nil || generation < 1 {
		return nil, ErrIdentity
	}
	release, err := resourceLease(ctx, lease)
	if err != nil {
		return nil, err
	}
	var session *PublicationSession
	err = func() error {
		defer release()
		c.stateMu.RLock()
		defer c.stateMu.RUnlock()
		actor, err := c.resourceSigningActor(ctx)
		if err != nil {
			return err
		}
		if current.Scope() != c.resourceScope() || current.Actor() != actor || binding.Namespace != "resources:"+c.cfg.ResourceProduct {
			return ErrIdentity
		}
		session = &PublicationSession{owner: c, scope: binding.Scope, namespace: binding.Namespace, generation: generation, user: c.session.UserID, device: c.session.DeviceID, root: id.Ed25519(actor.RootKey), lease: lease, validate: current.Revalidate}
		return nil
	}()
	if err != nil {
		return nil, err
	}
	if err := current.Revalidate(ctx); err != nil {
		return nil, err
	}
	return session, nil
}

func (p *PreparedEncryptedPublication) withPublicationSession(ctx context.Context, session *PublicationSession, fn func(context.Context) error) error {
	if !p.validPublicationSession(session) {
		return ErrIdentity
	}
	// A provider may acquire its OWN persistent work lease and use HTTP/SDK.
	// Calling it under ours can deadlock behind a waiting logout writer.
	if session.validate != nil {
		if err := session.validate(ctx); err != nil {
			return err
		}
	}
	if err := p.Revalidate(ctx); err != nil {
		return err
	}
	release, err := resourceLease(ctx, session.lease)
	if err != nil {
		return err
	}
	err = func() error {
		defer release()
		unlock, err := p.lockReady(p.operationContext(ctx))
		if err != nil {
			return err
		}
		defer unlock()
		if err := p.verifyPublicationSessionScopeAndRoot(ctx, session); err != nil {
			return err
		}
		if err := fn(ctx); err != nil {
			return err
		}
		if !p.sameClient() {
			return ErrIdentity
		}
		return ctx.Err()
	}()
	if err != nil {
		return err
	}
	if session.validate != nil {
		if err := session.validate(ctx); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (p *PreparedEncryptedPublication) validPublicationSession(session *PublicationSession) bool {
	return p != nil && session != nil && session.owner == p.owner && session.lease != nil &&
		publication.ValidNamespace(session.scope, session.namespace) && session.generation >= 1 &&
		session.scope.Subject == p.binding.Subject && session.scope.Tenant == p.binding.Tenant &&
		session.user.String() == p.binding.MatrixUserID && session.device.String() == p.binding.DeviceID
}

func (p *PreparedEncryptedPublication) verifyPublicationSessionScopeAndRoot(ctx context.Context, session *PublicationSession) error {
	if session.namespace == "storage:"+session.scope.Product {
		if p.owner.cfg.Bootstrap.StorageIdentityVersion != storage.IsolatedIdentity || p.owner.storageScope() != session.scope || p.owner.cfg.ResourceProduct != "" {
			return ErrIdentity
		}
	} else if session.validate == nil || session.namespace != "resources:"+session.scope.Product || p.owner.cfg.ResourceProduct != session.scope.Product || p.owner.cfg.Bootstrap.StorageProduct != "" {
		return ErrIdentity
	}
	status, err := p.owner.verificationStatus(ctx)
	if err != nil {
		return err
	}
	if !status.Verified {
		return ErrUnverified
	}
	keys, err := p.machine.Machine().GetOwnCrossSigningPublicKeys(ctx)
	if err != nil {
		return err
	}
	if keys == nil || keys.MasterKey != session.root {
		return ErrIdentity
	}
	return ctx.Err()
}

// SignAdmission derives every wire/target/transaction field locally. Scope,
// namespace and generation come only from a verified PublicationSession.
func (p *PreparedEncryptedPublication) SignAdmission(ctx context.Context, session *PublicationSession) (publication.SignedRequest, error) {
	var signed publication.SignedRequest
	err := p.withPublicationSession(ctx, session, func(ctx context.Context) error {
		prepared, err := publication.BuildRequest(session.scope, session.namespace, storage.PublicationTarget{RoomID: p.binding.RoomID, DeviceID: p.binding.DeviceID}, p.binding.TransactionID, session.generation, p.raw)
		if err != nil {
			return err
		}
		defer clear(prepared.Payload)
		request := prepared.Request
		signature, err := p.machine.Machine().GetAccount().SignJSON(request)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		signed = publication.SignedRequest{Request: request, Signatures: signatures.NewSingleSignature(session.user, id.KeyAlgorithmEd25519, session.device.String(), signature)}
		return nil
	})
	if err != nil {
		return publication.SignedRequest{}, err
	}
	return signed, nil
}

// SignSessionCertificate proves only this actual prepared SDK session and exact
// context. Its public signature grants no role; the Resource authority still
// verifies the independently pinned epoch/capability and before-HTTP ACK.
func (p *PreparedEncryptedPublication) SignSessionCertificate(ctx context.Context, session *PublicationSession, expected sessionproof.CertificateExpectation) (sessionproof.Certificate, error) {
	var certificate sessionproof.Certificate
	err := p.withPublicationSession(ctx, session, func(ctx context.Context) error {
		if !expected.Valid() || expected.Scope != session.scope || expected.Session != p.provenance || expected.DeviceID != p.binding.DeviceID || expected.RootKey != session.root.String() {
			return ErrIdentity
		}
		certificate = sessionproof.Certificate{Version: 1, Kind: sessionproof.CertificateKind, CertificateExpectation: expected}
		signature, err := p.machine.Machine().GetAccount().SignJSON(certificate)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		certificate.Signatures = signatures.NewSingleSignature(session.user, id.KeyAlgorithmEd25519, session.device.String(), signature)
		return nil
	})
	if err != nil {
		return sessionproof.Certificate{}, err
	}
	return certificate, nil
}

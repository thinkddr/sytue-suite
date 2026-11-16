// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"errors"
	"io"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/ssss"
	"maunium.net/go/mautrix/event"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/keyring"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// RequireRecoverableIngress retains the isolated-only ingress contract while
// sharing the recoverable-publication capability used by all private storage.
func (c *Client) RequireRecoverableIngress(ctx context.Context, scope storage.Scope, target storage.PublicationTarget) (context.Context, error) {
	c.stateMu.RLock()
	isolated := c.cfg.Bootstrap.StorageIdentityVersion == storage.IsolatedIdentity
	c.stateMu.RUnlock()
	if !isolated {
		return nil, ErrIdentity
	}
	bound, err := c.RequireRecoverableStorage(ctx, scope, target)
	if err != nil {
		return nil, err
	}
	proof, ok := bound.Value(storagePublicationContext{}).(storagePublication)
	if !ok || proof.identity != storage.IsolatedIdentity {
		return nil, ErrIdentity
	}
	return bound, nil
}

// CreateIngressKey saves and re-reads the private recipient in maintained SSSS
// before exposing its public enrollment. Each candidate has its own key-ID
// secret name: racing devices cannot overwrite the winner's recovery material.
func (c *Client) CreateIngressKey(ctx context.Context, scope storage.Scope, epoch string, generation int64) (ingress.KeyPlan, error) {
	if !storage.MigrationEpoch(epoch) || generation < 1 || generation >= 1<<53 {
		return ingress.KeyPlan{}, ingress.ErrContract
	}
	if _, err := c.PrepareStorage(ctx, scope); err != nil {
		return ingress.KeyPlan{}, err
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.storageScope() != scope || c.cfg.Bootstrap.StorageIdentityVersion != storage.IsolatedIdentity {
		return ingress.KeyPlan{}, ErrIdentity
	}
	owner, err := c.storageAuthKey(ctx, "")
	if err != nil {
		return ingress.KeyPlan{}, err
	}
	defer clear(owner.Key)
	plan, err := keyring.Create(ctx, nativeIngressSecrets{client: c, owner: owner}, scope, epoch, generation)
	return plan, ingressKeyError(err)
}

// RecoverIngressKeyPlan lets another verified device enroll in the immutable
// existing recipient. SSSS stays inside the maintained SDK; only the public age
// recipient is returned, and missing recovery data never generates a new key.
func (c *Client) RecoverIngressKeyPlan(ctx context.Context, task ingress.Task) (ingress.KeyPlan, error) {
	if task.Validate(task.Scope, task.Epoch) != nil {
		return ingress.KeyPlan{}, ingress.ErrContract
	}
	return c.recoverIngressKeyPlan(ctx, task.Scope, task.Epoch, task.KeyGeneration, task.KeyID)
}

// RecoverIngressEnrollment authenticates the exact public recipient against its
// SSSS secret. A missing secret never creates another candidate or generation.
func (c *Client) RecoverIngressEnrollment(ctx context.Context, expected ingress.KeyPlan) (ingress.KeyPlan, error) {
	if agecipher.ValidateKeyPlan(expected) != nil {
		return ingress.KeyPlan{}, ingress.ErrContract
	}
	actual, err := c.recoverIngressKeyPlan(ctx, expected.Scope, expected.Epoch, expected.Generation, expected.KeyID)
	if err != nil {
		return ingress.KeyPlan{}, err
	}
	if actual != expected {
		return ingress.KeyPlan{}, ingress.ErrContract
	}
	return actual, nil
}

func (c *Client) recoverIngressKeyPlan(ctx context.Context, scope storage.Scope, epoch string, generation int64, keyID string) (ingress.KeyPlan, error) {
	if _, err := c.PrepareStorage(ctx, scope); err != nil {
		return ingress.KeyPlan{}, err
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.storageScope() != scope || c.cfg.Bootstrap.StorageIdentityVersion != storage.IsolatedIdentity {
		return ingress.KeyPlan{}, ErrIdentity
	}
	status, err := c.verificationStatus(ctx)
	if err != nil {
		return ingress.KeyPlan{}, err
	}
	if !status.Verified {
		return ingress.KeyPlan{}, ErrUnverified
	}
	plan, err := keyring.RecoverTuple(ctx, nativeIngressSecrets{client: c}, scope, epoch, generation, keyID)
	return plan, ingressKeyError(err)
}

// Caller holds stateMu. Nothing is generated when recovery data is missing.
func (c *Client) readIngressIdentity(ctx context.Context, scope storage.Scope, epoch string, generation int64, keyID string) (*pqc.Identity, error) {
	if scope != c.storageScope() || c.cfg.Bootstrap.StorageIdentityVersion != storage.IsolatedIdentity {
		return nil, ErrIdentity
	}
	identity, err := keyring.ReadIdentity(ctx, nativeIngressSecrets{client: c}, scope, epoch, generation, keyID)
	return identity, ingressKeyError(err)
}

// OpenIngressBody keeps the private age identity inside the authorized SDK
// client. The caller supplies an immutable ciphertext spool, never a URL.
func (c *Client) OpenIngressBody(ctx context.Context, task ingress.Task, source io.ReaderAt) ([]byte, *io.SectionReader, error) {
	if task.Validate(task.Scope, task.Epoch) != nil {
		return nil, nil, ingress.ErrContract
	}
	if _, err := c.PrepareStorage(ctx, task.Scope); err != nil {
		return nil, nil, err
	}
	c.stateMu.RLock()
	identity, err := c.readIngressIdentity(ctx, task.Scope, task.Epoch, task.KeyGeneration, task.KeyID)
	c.stateMu.RUnlock()
	if err != nil {
		return nil, nil, err
	}
	return agecipher.Open(ctx, source, task, identity)
}

type nativeIngressSecrets struct {
	client *Client
	owner  *ssss.Key
}

func (s nativeIngressSecrets) Write(ctx context.Context, name string, raw []byte) error {
	if s.owner == nil {
		return ErrIdentity
	}
	return s.client.crypto.Machine().SSSS.SetEncryptedAccountData(ctx, event.Type{Type: name, Class: event.AccountDataEventType}, raw, s.owner)
}

func (s nativeIngressSecrets) Read(ctx context.Context, name string) ([]byte, error) {
	owner, err := s.client.storageAuthKey(ctx, "")
	if err != nil {
		return nil, err
	}
	defer clear(owner.Key)
	raw, err := s.client.crypto.Machine().SSSS.GetDecryptedAccountData(ctx, event.Type{Type: name, Class: event.AccountDataEventType}, owner)
	if errors.Is(err, mautrix.MNotFound) {
		return nil, ErrRecoveryRequired
	}
	if err != nil {
		return nil, ErrUnverified
	}
	return raw, nil
}

func ingressKeyError(err error) error {
	if errors.Is(err, keyring.ErrUnverified) {
		return ErrUnverified
	}
	if errors.Is(err, keyring.ErrEncoding) {
		return ErrSecureStore
	}
	return err
}

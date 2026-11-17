// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"strings"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Both identities belong to the configured Sytue Matrix server. A same-looking
// scope/account on another homeserver is not this product's recovery vault.
func bootstrapRecoveryVaultUser(scope storage.Scope, canonical, vault id.UserID) bool {
	_, canonicalServer, canonicalErr := canonical.Parse()
	_, vaultServer, vaultErr := vault.Parse()
	prefix := storage.IsolatedMatrixUserPrefix(scope)
	return scope.Valid() && canonicalErr == nil && vaultErr == nil && canonical != vault && canonicalServer == vaultServer && strings.HasPrefix(vault.String(), prefix) && len(vault.String()) > len(prefix)
}

type bootstrapRecoveryWork struct {
	Version               int
	Scope                 storage.Scope
	PlanSHA256            string
	PolicySHA256          string
	OwnerCapabilitySHA256 string
}

// withBootstrapRecoveryVault keeps canonical resource enrollment separate from
// the product-isolated storage identity and its recovery root. The work lease is
// the caller's original product consent, not a cached OIDC token or JSON grant.
// Provider/network validation is outside the lease to avoid recursively locking
// a retained read lease while an account writer is queued.
func (c *Client) withBootstrapRecoveryVault(ctx context.Context, current resource.VerifiedSession, vault *Client, lease offline.WorkLease, operation func(context.Context) error) error {
	if c == nil || vault == nil || c == vault || operation == nil {
		return ErrIdentity
	}
	if err := current.Revalidate(ctx); err != nil {
		return err
	}
	release, err := resourceLease(ctx, lease)
	if err != nil {
		return err
	}
	err = func() error {
		defer release()
		if !c.validBootstrapRecoveryVaultPair(current, vault) {
			return ErrIdentity
		}
		binding, err := vault.PrepareStorage(ctx, current.Scope())
		if err != nil {
			return err
		}
		if !binding.Valid() || binding.Scope != current.Scope() {
			return ErrIdentity
		}
		bound, err := vault.RequireRecoverableStorage(ctx, current.Scope(), binding.Target)
		if err != nil {
			return err
		}
		if err := operation(bound); err != nil {
			return err
		}
		after, err := vault.StorageBindings(ctx)
		if err != nil {
			return err
		}
		if !binding.Same(after) {
			return ErrIdentity
		}
		return ctx.Err()
	}()
	if err != nil {
		return err
	}
	return current.Revalidate(ctx)
}

func (c *Client) validBootstrapRecoveryVaultPair(current resource.VerifiedSession, vault *Client) bool {
	c.stateMu.RLock()
	valid := !c.closed.Load() && c.cfg.ResourceProduct != "" && current.Scope() == c.resourceScope() && current.Binding().UserID == c.session.UserID.String() && current.Binding().DeviceID == c.session.DeviceID.String()
	c.stateMu.RUnlock()
	if !valid {
		return false
	}
	vault.stateMu.RLock()
	valid = !vault.closed.Load() && vault.cfg.ResourceProduct == "" && vault.storageScope() == current.Scope() && vault.cfg.Bootstrap.StorageIdentityVersion == storage.IsolatedIdentity && bootstrapRecoveryVaultUser(current.Scope(), id.UserID(current.Binding().UserID), vault.session.UserID)
	vault.stateMu.RUnlock()
	return valid
}

// BackupResourceBootstrapKey writes and reads back the exact private epoch key
// through this product's existing SDK2 SSSS. It must finish before bootstrap
// ciphertext is treated as safely retained. Accepted-but-lost writes are read
// back first on retry; a missing secret never causes another identity.
func (c *Client) BackupResourceBootstrapKey(ctx context.Context, current resource.VerifiedSession, plan resource.VerifiedBootstrapPlan, vault *Client, lease offline.WorkLease, journal *offline.Store) error {
	value := plan.Value()
	if c == nil || vault == nil || c == vault || !journal.Encrypted() || current.Binding() != plan.SessionBinding() || current.Scope() != value.CreatorScope || current.Actor() != value.Creator {
		return ErrIdentity
	}
	vault.stateMu.RLock()
	validVault := !vault.closed.Load() && vault.cfg.ResourceProduct == "" && vault.storageScope() == current.Scope() && vault.cfg.Bootstrap.StorageIdentityVersion == storage.IsolatedIdentity && bootstrapRecoveryVaultUser(current.Scope(), id.UserID(current.Binding().UserID), vault.session.UserID)
	vault.stateMu.RUnlock()
	if !validVault {
		return ErrIdentity
	}
	identity, err := c.bootstrapKey(ctx, current, value.Source.Resource, value.ID, lease, journal, false)
	if err != nil {
		return err
	}
	raw, err := resource.EncodeBootstrapRecovery(ctx, plan, identity)
	if err != nil {
		return err
	}
	defer clear(raw)
	name, err := resource.BootstrapRecoverySecretID(plan.SHA256())
	if err != nil {
		return err
	}
	return c.backupBootstrapVaultValue(ctx, current, vault, lease, journal, name, bootstrapRecoveryWork{Version: 1, Scope: current.Scope(), PlanSHA256: plan.SHA256()}, raw, plan.Revalidate)
}

// The pre-network key and final accepted-policy anchor use the SAME immutable
// authenticated SDK vault read/write/readback and retained work ordering.
func (c *Client) backupBootstrapVaultValue(ctx context.Context, current resource.VerifiedSession, vault *Client, lease offline.WorkLease, journal *offline.Store, name string, expected bootstrapRecoveryWork, raw []byte, revalidate func(context.Context) error) error {
	if journal == nil || !journal.Encrypted() || revalidate == nil {
		return ErrIdentity
	}
	owner := current.Scope().Tenant + "\x00" + current.Scope().Subject + "\x00" + current.Scope().Product
	namespace := storage.ResourceBootstrapRecoveryNamespace
	return journal.Exclusive(ctx, owner, namespace, name, func() error {
		var retained bootstrapRecoveryWork
		found, err := journal.Read(owner, namespace, name, &retained)
		if err != nil {
			return err
		}
		if found && retained != expected {
			return ErrIdentity
		}
		if !found {
			if err := journal.Write(owner, namespace, name, expected); err != nil {
				return err
			}
		}
		err = c.withBootstrapRecoveryVault(ctx, current, vault, lease, func(ctx context.Context) error {
			return writeAndVerifyBootstrapVaultSecret(ctx, vault, name, raw)
		})
		if err != nil {
			return err
		}
		if err := revalidate(ctx); err != nil {
			return err
		}
		release, err := resourceLease(ctx, lease)
		if err != nil {
			return err
		}
		defer release()
		c.stateMu.RLock()
		same := !c.closed.Load() && c.cfg.ResourceProduct != "" && c.resourceScope() == current.Scope() && c.session.UserID.String() == current.Actor().UserID && c.session.DeviceID.String() == current.Actor().DeviceID
		c.stateMu.RUnlock()
		if !same {
			return ErrIdentity
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return journal.Delete(owner, namespace, name)
	})
}

func writeAndVerifyBootstrapVaultSecret(ctx context.Context, vault *Client, name string, raw []byte) error {
	existing, revision, found, err := vault.ReadSecret(ctx, name, 1)
	defer clear(existing)
	if err != nil {
		return err
	}
	if found {
		if revision != 1 || !bytes.Equal(existing, raw) {
			return ErrRecoveryExists
		}
		return nil
	}
	if err := vault.PutSecret(ctx, name, raw, 1); err != nil {
		return err
	}
	accepted, revision, found, err := vault.ReadSecret(ctx, name, 1)
	defer clear(accepted)
	if err != nil {
		return err
	}
	if !found || revision != 1 || !bytes.Equal(accepted, raw) {
		return ErrRecoveryRequired
	}
	return nil
}

// RecoverResourceBootstrapEpoch lets current device B recover an epoch handle from
// its independently recovered product SDK2, without the OS store or live SDK of
// producer A. The independently pinned epoch window and old captured signature
// still apply. This does not re-enroll A, restore publication WAL or grant access.
func (c *Client) RecoverResourceBootstrapEpoch(ctx context.Context, current resource.VerifiedSession, planSHA256 string, window resource.EpochWindow, vault *Client, lease offline.WorkLease) (*resource.EpochHandle, error) {
	name, err := resource.BootstrapRecoverySecretID(planSHA256)
	if err != nil {
		return nil, err
	}
	var handle *resource.EpochHandle
	err = c.withBootstrapRecoveryVault(ctx, current, vault, lease, func(ctx context.Context) error {
		raw, revision, found, err := vault.ReadSecret(ctx, name, 1)
		defer clear(raw)
		if err != nil {
			return err
		}
		if !found || revision != 1 {
			return ErrRecoveryRequired
		}
		handle, err = resource.RestoreBootstrapRecovery(ctx, raw, planSHA256, current.Scope(), window)
		return err
	})
	if err != nil {
		if handle != nil {
			handle.Close()
		}
		return nil, err
	}
	return handle, nil
}

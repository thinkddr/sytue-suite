// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// BackupResourceBootstrapPolicy durably writes and reads back the final policy
// pin through the SAME authenticated product SDK2 vault as the epoch backup.
// Call it before SourceComplete/retiring original bootstrap work. A failed
// write/readback keeps recovery work pending; retry first reconciles exact bytes.
// It neither replaces the pre-network key secret nor treats SQL active as ACK.
func (c *Client) BackupResourceBootstrapPolicy(ctx context.Context, current resource.VerifiedSession, activation resource.VerifiedBootstrapActivation, ownerIntent resource.Capability, vault *Client, lease offline.WorkLease, journal *offline.Store) error {
	_, err := c.backupResourceBootstrapPolicy(ctx, current, activation, ownerIntent, vault, lease, journal)
	return err
}

func (c *Client) backupResourceBootstrapPolicy(ctx context.Context, current resource.VerifiedSession, activation resource.VerifiedBootstrapActivation, ownerIntent resource.Capability, vault *Client, lease offline.WorkLease, journal *offline.Store) (resource.BootstrapRecoveryRequirement, error) {
	var zero resource.BootstrapRecoveryRequirement
	p := activation.Plan()
	bindingSHA, err := matrixproof.RequestDigest(current.Binding())
	if err != nil || current.Scope() != p.CreatorScope || current.Actor() != p.Creator || bindingSHA != p.SessionBindingSHA256 {
		return zero, ErrIdentity
	}
	if err := resource.CheckBootstrapOwnerCapabilityIntent(ctx, ownerIntent, activation); err != nil {
		return zero, err
	}
	// The caller supplies its original explicit owner intent. Only the CURRENT
	// SDK signs it under the retained product lease; restore never infers a grant.
	signed := resource.SignedCapability{Capability: ownerIntent}
	signed.Signatures, err = c.signCurrentResourceMetadata(ctx, current, ownerIntent, lease)
	if err != nil {
		return zero, err
	}
	raw, err := resource.EncodeBootstrapPolicyAnchor(ctx, activation, signed)
	if err != nil {
		return zero, err
	}
	defer clear(raw)
	name, err := resource.BootstrapPolicyAnchorSecretID(p.Source.Resource, p.ID)
	if err != nil {
		return zero, err
	}
	capSHA, err := resource.CapabilityDigest(ownerIntent)
	if err != nil {
		return zero, err
	}
	rawSHA, err := resource.CipherJSONDigest(raw)
	if err != nil {
		return zero, err
	}
	want := resource.BootstrapRecoveryRequirement{Resource: p.Source.Resource, PlanID: p.ID, PlanSHA256: activation.PlanSHA256(), PolicySHA256: activation.Anchor().PolicySHA256, OwnerCapabilitySHA256: capSHA, VaultEvidenceSHA256: rawSHA}
	expected := bootstrapRecoveryWork{Version: 1, Scope: current.Scope(), PlanSHA256: want.PlanSHA256, PolicySHA256: want.PolicySHA256, OwnerCapabilitySHA256: want.OwnerCapabilitySHA256}
	port := bootstrapVaultPolicyAnchors{owner: c, current: current, vault: vault, lease: lease}
	err = c.backupBootstrapVaultValue(ctx, current, vault, lease, journal, name, expected, raw, func(ctx context.Context) error {
		if err := activation.Revalidate(ctx); err != nil {
			return err
		}
		// Re-authenticate and decode the complete signed capability/chain before
		// the shared helper may delete its pending recovery work under the lease.
		evidence, err := port.BootstrapPolicyAuthority(ctx, p.Source.Resource, p.ID)
		if err != nil {
			return err
		}
		if evidence.Requirement() != want {
			return resource.ErrInvalid
		}
		return activation.Revalidate(ctx)
	})
	if err != nil {
		return zero, err
	}
	return want, nil
}

type bootstrapVaultPolicyAnchors struct {
	owner   *Client
	current resource.VerifiedSession
	vault   *Client
	lease   offline.WorkLease
}

var (
	_ resource.BootstrapPolicyAnchorSource    = bootstrapVaultPolicyAnchors{}
	_ resource.BootstrapPolicyAuthoritySource = bootstrapVaultPolicyAnchors{}
)

// ResourceBootstrapPolicyAnchors creates an actual authenticated SDK2 vault
// trust port for CURRENT B, without any policy supplied by Source. Every read
// repeats vault recoverability, scope/user/server and current session checks.
// Token-only or global/cross-product SSSS cannot serve as an anchor source.
func (c *Client) ResourceBootstrapPolicyAnchors(ctx context.Context, current resource.VerifiedSession, vault *Client, lease offline.WorkLease) (resource.BootstrapPolicyAnchorSource, error) {
	if err := c.withBootstrapRecoveryVault(ctx, current, vault, lease, func(context.Context) error { return nil }); err != nil {
		return nil, err
	}
	return bootstrapVaultPolicyAnchors{owner: c, current: current, vault: vault, lease: lease}, nil
}

func (v bootstrapVaultPolicyAnchors) BootstrapPolicyAnchor(ctx context.Context, key resource.Resource, planID string) (resource.Anchor, error) {
	var anchor resource.Anchor
	err := v.withPolicySecret(ctx, key, planID, func(ctx context.Context, raw []byte) error {
		var err error
		anchor, err = resource.DecodeBootstrapPolicyAnchor(ctx, raw, key, planID, v.current.Binding())
		return err
	})
	if err != nil {
		return resource.Anchor{}, err
	}
	return anchor, nil
}

func (v bootstrapVaultPolicyAnchors) BootstrapPolicyAuthority(ctx context.Context, key resource.Resource, planID string) (resource.BootstrapPolicyAuthorityEvidence, error) {
	var evidence resource.BootstrapPolicyAuthorityEvidence
	err := v.withPolicySecret(ctx, key, planID, func(ctx context.Context, raw []byte) error {
		var err error
		evidence, err = resource.DecodeBootstrapPolicyAuthority(ctx, raw, key, planID, v.current.Binding())
		return err
	})
	if err != nil {
		return resource.BootstrapPolicyAuthorityEvidence{}, err
	}
	return evidence, nil
}

func (v bootstrapVaultPolicyAnchors) withPolicySecret(ctx context.Context, key resource.Resource, planID string, operation func(context.Context, []byte) error) error {
	name, err := resource.BootstrapPolicyAnchorSecretID(key, planID)
	if err != nil {
		return err
	}
	return v.owner.withBootstrapRecoveryVault(ctx, v.current, v.vault, v.lease, func(ctx context.Context) error {
		raw, revision, found, err := v.vault.ReadSecret(ctx, name, 1)
		defer clear(raw)
		if err != nil {
			return err
		}
		if !found || revision != 1 {
			return ErrRecoveryRequired
		}
		return operation(ctx, raw)
	})
}

func (c *Client) ResourceBootstrapPolicyAuthority(ctx context.Context, current resource.VerifiedSession, vault *Client, lease offline.WorkLease) (resource.BootstrapPolicyAuthoritySource, error) {
	if err := c.withBootstrapRecoveryVault(ctx, current, vault, lease, func(context.Context) error { return nil }); err != nil {
		return nil, err
	}
	return bootstrapVaultPolicyAnchors{owner: c, current: current, vault: vault, lease: lease}, nil
}

// RecoverResourceBootstrapPolicy reads the independently authenticated final
// capsule, including A's signed owner capability, before the shared historical
// verifier checks ORIGINAL Source and B. No prior normal operation is needed.
func (c *Client) RecoverResourceBootstrapPolicy(ctx context.Context, current resource.VerifiedSession, key resource.Resource, planID string, source resource.TrustedBootstrapPolicySource, vault *Client, lease offline.WorkLease) (resource.RestoredBootstrapPolicy, error) {
	port, err := c.ResourceBootstrapPolicyAuthority(ctx, current, vault, lease)
	if err != nil {
		return resource.RestoredBootstrapPolicy{}, err
	}
	return resource.RestoreBootstrapPolicyFromVault(ctx, key, planID, current, port, source)
}

type bootstrapRecoveryGuard struct {
	source     resource.BootstrapPolicyAuthoritySource
	want       resource.BootstrapRecoveryRequirement
	revalidate func(context.Context) error
}

func (g bootstrapRecoveryGuard) CheckBootstrapRecoveryReady(ctx context.Context, want resource.BootstrapRecoveryRequirement) error {
	if g.source == nil || g.revalidate == nil || !want.Valid() || want != g.want {
		return resource.ErrRevoked
	}
	if err := g.revalidate(ctx); err != nil {
		return err
	}
	evidence, err := g.source.BootstrapPolicyAuthority(ctx, want.Resource, want.PlanID)
	if err != nil {
		return err
	}
	if evidence.Requirement() != want {
		return resource.ErrInvalid
	}
	return g.revalidate(ctx)
}

// PrepareResourceBootstrapRecovery returns only a per-bootstrap prerequisite.
// It always signs, persists and re-reads the exact original owner capability and
// anchor first. It cannot call product-wide SourceComplete or ACK/delete work.
func (c *Client) PrepareResourceBootstrapRecovery(ctx context.Context, current resource.VerifiedSession, activation resource.VerifiedBootstrapActivation, ownerIntent resource.Capability, vault *Client, lease offline.WorkLease, journal *offline.Store) (resource.BootstrapRecoveryPrecondition, resource.BootstrapRecoveryRequirement, error) {
	want, err := c.backupResourceBootstrapPolicy(ctx, current, activation, ownerIntent, vault, lease, journal)
	if err != nil {
		return nil, resource.BootstrapRecoveryRequirement{}, err
	}
	port := bootstrapVaultPolicyAnchors{owner: c, current: current, vault: vault, lease: lease}
	return bootstrapRecoveryGuard{source: port, want: want, revalidate: activation.Revalidate}, want, nil
}

// RecoveredResourceBootstrapRecovery keeps the prerequisite usable only with
// the SAME actual current B and ORIGINAL Source-bound recovered policy.
func (c *Client) RecoveredResourceBootstrapRecovery(ctx context.Context, current resource.VerifiedSession, key resource.Resource, planID string, policy resource.RestoredBootstrapPolicy, vault *Client, lease offline.WorkLease) (resource.BootstrapRecoveryPrecondition, resource.BootstrapRecoveryRequirement, error) {
	port, err := c.ResourceBootstrapPolicyAuthority(ctx, current, vault, lease)
	if err != nil {
		return nil, resource.BootstrapRecoveryRequirement{}, err
	}
	evidence, err := port.BootstrapPolicyAuthority(ctx, key, planID)
	if err != nil {
		return nil, resource.BootstrapRecoveryRequirement{}, err
	}
	want, err := policy.RecoveryRequirement(ctx, evidence)
	if err != nil || want.Resource != key || want.PlanID != planID {
		return nil, resource.BootstrapRecoveryRequirement{}, resource.ErrRevoked
	}
	check := func(ctx context.Context) error {
		access, err := policy.Access(ctx, resource.Manage)
		if err != nil {
			return err
		}
		binding, ok := access.CurrentSessionBinding()
		if !ok || binding != current.Binding() {
			return resource.ErrRevoked
		}
		return current.Revalidate(ctx)
	}
	guard := bootstrapRecoveryGuard{source: port, want: want, revalidate: check}
	if err := guard.CheckBootstrapRecoveryReady(ctx, want); err != nil {
		return nil, resource.BootstrapRecoveryRequirement{}, err
	}
	return guard, want, nil
}

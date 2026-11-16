// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// This port is an authenticated-vault witness for codec/core tests. Actual
// product SDK2 reads/writes are exercised by the native Synapse test.
type policyAuthorityFixture struct {
	raw     []byte
	current resource.VerifiedSession
}

func (s policyAuthorityFixture) BootstrapPolicyAuthority(ctx context.Context, key resource.Resource, planID string) (resource.BootstrapPolicyAuthorityEvidence, error) {
	if err := s.current.Revalidate(ctx); err != nil {
		return resource.BootstrapPolicyAuthorityEvidence{}, err
	}
	return resource.DecodeBootstrapPolicyAuthority(ctx, s.raw, key, planID, s.current.Binding())
}

func TestBootstrapVaultRestoresOriginalSignedOwnerCapabilityWithoutNormalOperation(t *testing.T) {
	c := preparePolicyRestoreCase(t)
	defer clear(c.backup)
	defer clear(c.anchorRaw)
	key, planID := c.source.Retained.Capture.Resource, c.source.Retained.Capture.PlanID
	// No normal operation exists in this Source contract, and the ONLY signed
	// capability statement is removed from its public evidence ledger entirely.
	delete(c.source.Records, c.capability.Capability.StatementSHA256)
	if _, err := restorePolicyCase(t.Context(), c); err == nil {
		t.Fatal("Source fabricated an unretained signed capability")
	}
	c.capability = resource.CapabilityEvidence{}
	vault := policyAuthorityFixture{raw: bytes.Clone(c.anchorRaw), current: c.current}
	defer clear(vault.raw)
	policy, err := resource.RestoreBootstrapPolicyFromVault(t.Context(), key, planID, c.current, vault, c.source)
	if err != nil {
		t.Fatal("bootstrap-only B required a previous normal operation/capability in Source", err)
	}
	access, err := policy.Access(t.Context(), resource.Read)
	if err != nil || access.Actor() != c.current.Actor() {
		t.Fatal("vault signed grant lost current B", err)
	}
	evidence, err := vault.BootstrapPolicyAuthority(t.Context(), key, planID)
	if err != nil {
		t.Fatal(err)
	}
	requirement, err := policy.RecoveryRequirement(t.Context(), evidence)
	if err != nil || !requirement.Valid() || requirement.OwnerCapabilitySHA256 != access.Capability().SHA256() {
		t.Fatal("original signed capability hash was not pinned", err)
	}
	// Simulate process restart with only durable public Source bytes and this
	// separately authenticated capsule. No VerifiedEpoch/Capability survives.
	encoded, err := json.Marshal(c.source)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &policySourceFixture{}
	if json.Unmarshal(encoded, restarted) != nil {
		t.Fatal("Source public restart")
	}
	policy, err = resource.RestoreBootstrapPolicyFromVault(t.Context(), key, planID, c.current, vault, restarted)
	if err != nil {
		t.Fatal("bootstrap-only restart lost original owner grant", err)
	}
	restarted.withdraw = true
	if _, err := policy.Access(t.Context(), resource.Read); err == nil {
		t.Fatal("vault owner signature bypassed changed original Source")
	}
}

func TestBootstrapVaultMissingAlteredCapabilityNeverBecomesVerifiedGrant(t *testing.T) {
	c := preparePolicyRestoreCase(t)
	defer clear(c.backup)
	defer clear(c.anchorRaw)
	key, planID := c.source.Retained.Capture.Resource, c.source.Retained.Capture.PlanID
	delete(c.source.Records, c.capability.Capability.StatementSHA256)
	for _, fault := range []string{"absent", "capability-hash", "missing-records", "statement-pin", "signature", "record-hash"} {
		t.Run(fault, func(t *testing.T) {
			var value map[string]any
			if json.Unmarshal(c.anchorRaw, &value) != nil {
				t.Fatal("private capsule fixture")
			}
			switch fault {
			case "capability-hash":
				value["owner_capability_sha256"] = c.source.Retained.PolicySHA256
			case "missing-records":
				delete(value, "capability_records")
			case "statement-pin":
				value["owner_capability"].(map[string]any)["capability"].(map[string]any)["signer"].(map[string]any)["pin"].(map[string]any)["SigningKey"] = "another"
			case "signature":
				refs := value["owner_capability"].(map[string]any)["capability"].(map[string]any)
				for _, item := range value["capability_records"].([]any) {
					record := item.(map[string]any)
					if record["sha256"] == refs["statement_sha256"] {
						record["value"].(map[string]any)["signatures"] = map[string]any{}
						raw, err := matrixproof.CanonicalBytes(record["value"])
						if err != nil {
							t.Fatal(err)
						}
						sha, err := resource.CipherJSONDigest(raw)
						if err != nil {
							t.Fatal(err)
						}
						record["sha256"], refs["statement_sha256"] = sha, sha
					}
				}
				items := value["capability_records"].([]any)
				sort.Slice(items, func(i, j int) bool {
					return items[i].(map[string]any)["sha256"].(string) < items[j].(map[string]any)["sha256"].(string)
				})
			case "record-hash":
				value["capability_records"].([]any)[0].(map[string]any)["sha256"] = c.source.Retained.PolicySHA256
			}
			raw, err := matrixproof.CanonicalBytes(value)
			if err != nil {
				t.Fatal(err)
			}
			defer clear(raw)
			if fault == "absent" {
				raw = nil
			}
			if _, err := resource.DecodeBootstrapPolicyAuthority(t.Context(), raw, key, planID, c.current.Binding()); err == nil {
				t.Fatal("private vault codec accepted missing/altered signed capability", fault)
			}
			vault := policyAuthorityFixture{raw: raw, current: c.current}
			if _, err := resource.RestoreBootstrapPolicyFromVault(t.Context(), key, planID, c.current, vault, c.source); err == nil {
				t.Fatal("missing/altered capsule manufactured verified owner capability", fault)
			}
		})
	}
}

// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

func TestBootstrapFinalPolicyAnchorHasSeparateImmutablePrivateVaultBinding(t *testing.T) {
	c := preparePolicyRestoreCase(t)
	defer clear(c.backup)
	defer clear(c.anchorRaw)
	key, planID := c.source.Retained.Capture.Resource, c.source.Retained.Capture.PlanID
	name, err := resource.BootstrapPolicyAnchorSecretID(key, planID)
	if err != nil {
		t.Fatal(err)
	}
	keyName, err := resource.BootstrapRecoverySecretID(c.source.Retained.Capture.PlanSHA256)
	if err != nil || name == keyName {
		t.Fatal("final policy overwrote the pre-network plan/key", err)
	}
	anchor, err := resource.DecodeBootstrapPolicyAnchor(t.Context(), c.anchorRaw, key, planID, c.current.Binding())
	if err != nil || anchor != c.anchors.anchor {
		t.Fatal("current B could not open original vault anchor without held policy", err)
	}
	for _, fault := range []string{"domain", "plan", "plan-hash", "plan-signature", "root-generation", "owner", "policy", "signature", "signer", "unknown"} {
		t.Run(fault, func(t *testing.T) {
			var value map[string]any
			if json.Unmarshal(c.anchorRaw, &value) != nil {
				t.Fatal("private fixture")
			}
			switch fault {
			case "domain":
				value["domain"] = "org.sytue.another"
			case "plan":
				value["plan_id"] = "00000000-0000-4000-8000-000000000001"
			case "plan-hash":
				value["plan_sha256"] = c.source.Retained.MembershipSHA256
			case "plan-signature":
				value["signed_plan"].(map[string]any)["signatures"] = map[string]any{}
			case "root-generation":
				value["original_binding"].(map[string]any)["root_generation"] = "2"
			case "owner":
				value["anchor"].(map[string]any)["OwnerScope"].(map[string]any)["subject"] = "another"
			case "policy":
				value["anchor"].(map[string]any)["PolicySHA256"] = c.source.Retained.MembershipSHA256
			case "signature":
				value["signed_epoch"].(map[string]any)["signatures"] = map[string]any{}
			case "signer":
				value["owner_pin"].(map[string]any)["SigningKey"] = "another"
			case "unknown":
				value["future"] = true
			}
			raw, err := matrixproof.CanonicalBytes(value)
			if err != nil {
				t.Fatal(err)
			}
			defer clear(raw)
			if _, err := resource.DecodeBootstrapPolicyAnchor(t.Context(), raw, key, planID, c.current.Binding()); err == nil {
				t.Fatal("substituted private policy anchor accepted", fault)
			}
		})
	}
}

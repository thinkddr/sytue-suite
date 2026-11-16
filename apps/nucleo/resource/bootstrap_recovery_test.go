// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// Metadata fixtures retain real SDK signatures; ciphertext acceptance is opaque
// synthetic wire. Independent actual SSSS custody runs in the Synapse test.
func recoveryWindowFixture(t *testing.T) (bootstrapPlanCase, resource.EpochWindow) {
	t.Helper()
	f := prepareBootstrapPlanFixture(t)
	baseline := bootstrapCandidateFixture(t, f, resource.BootstrapBaseline, nil)
	verified, err := resource.VerifyBootstrapCandidate(t.Context(), baseline, f.verified, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref := storage.Reference{RoomID: f.plan.RoomID, EventID: "$recovery-baseline"}
	accepted, err := resource.ConfirmBootstrapAcceptance(t.Context(), verified, ref, bootstrapAcceptanceFixture(baseline, ref))
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := bootstrapCandidateFixture(t, f, resource.BootstrapCheckpoint, &accepted)
	cpVerified, err := resource.VerifyBootstrapCandidate(t.Context(), checkpoint, f.verified, &accepted)
	if err != nil {
		t.Fatal(err)
	}
	cpRef := storage.Reference{RoomID: f.plan.RoomID, EventID: "$recovery-checkpoint"}
	cpAccepted, err := resource.ConfirmBootstrapAcceptance(t.Context(), cpVerified, cpRef, bootstrapAcceptanceFixture(checkpoint, cpRef))
	if err != nil {
		t.Fatal(err)
	}
	activation, err := resource.VerifyBootstrapActivation(t.Context(), f.bootstrap.FinalEpoch(t, accepted, cpAccepted), f.verified, accepted, cpAccepted)
	if err != nil {
		t.Fatal(err)
	}
	window, err := resource.CurrentEpoch(activation.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	return f, window
}

func TestBootstrapRecoveryUsesOriginalSignedPlanAndNeverRevivesProducer(t *testing.T) {
	f, window := recoveryWindowFixture(t)
	raw, err := resource.EncodeBootstrapRecovery(t.Context(), f.verified, f.f.EpochIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	name, err := resource.BootstrapRecoverySecretID(f.verified.SHA256())
	if err != nil || !storage.Identifier(name) {
		t.Fatal("immutable plan address", err)
	}
	other, _ := resource.BootstrapRecoverySecretID(strings.Repeat("a", 64))
	if name == other {
		t.Fatal("plans share backup address")
	}
	f.registry.Retire()
	if _, err := resource.EncodeBootstrapRecovery(t.Context(), f.verified, f.f.EpochIdentity); err == nil {
		t.Fatal("retired producer captured another backup")
	}
	// Recovery checks the captured signature and independently pinned epoch only;
	// it must not call the now-retired creator's live SessionProvider.
	handle, err := resource.RestoreBootstrapRecovery(t.Context(), raw, f.verified.SHA256(), f.plan.CreatorScope, window)
	if err != nil || handle == nil {
		t.Fatal("retired producer prevented owner recovery", err)
	}
	defer handle.Close()
	if _, err := handle.OpenFrame(t.Context(), resource.VerifiedPendingPublication{}, pqc.Envelope{}, resource.VerifiedAccess{}, nil); err == nil {
		t.Fatal("a recovered key created read authority")
	}
	for _, fault := range []string{"kind", "plan-hash", "source", "creator", "membership", "recipient", "signature", "identity", "unknown"} {
		t.Run(fault, func(t *testing.T) {
			var value map[string]any
			if json.Unmarshal(raw, &value) != nil {
				t.Fatal("private fixture decode")
			}
			plan := value["plan"].(map[string]any)
			switch fault {
			case "kind":
				value["domain"] = "org.sytue.another"
			case "plan-hash":
				value["plan_sha256"] = strings.Repeat("b", 64)
			case "source":
				plan["source"].(map[string]any)["source_epoch"] = "00000000-0000-4000-8000-000000000001"
			case "creator":
				plan["creator"].(map[string]any)["device_id"] = "ANOTHER_DEVICE"
			case "membership":
				plan["membership_sha256"] = strings.Repeat("c", 64)
			case "recipient":
				plan["recipient"] = f.f.RecipientIdentity.Recipient().String()
			case "signature":
				plan["signatures"] = map[string]any{}
			case "identity":
				value["identity"] = f.f.RecipientIdentity.String()
			case "unknown":
				value["future"] = true
			}
			modified, err := matrixproof.CanonicalBytes(value)
			if err != nil {
				t.Fatal(err)
			}
			defer clear(modified)
			if h, err := resource.RestoreBootstrapRecovery(t.Context(), modified, f.verified.SHA256(), f.plan.CreatorScope, window); err == nil {
				h.Close()
				t.Fatal("substituted recovery binding accepted", fault)
			}
		})
	}
	wrongScope := f.plan.CreatorScope
	wrongScope.Product = "mail"
	if _, err := resource.RestoreBootstrapRecovery(t.Context(), raw, f.verified.SHA256(), wrongScope, window); err == nil {
		t.Fatal("other product recovered a resource epoch")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := resource.RestoreBootstrapRecovery(cancelled, raw, f.verified.SHA256(), f.plan.CreatorScope, window); err == nil {
		t.Fatal("cancelled recovery returned a live handle")
	}
	if _, err := resource.RestoreBootstrapRecovery(t.Context(), raw, f.verified.SHA256(), f.plan.CreatorScope, resource.EpochWindow{}); err == nil {
		t.Fatal("public metadata replaced pinned epoch")
	}
	if _, err := resource.BootstrapRecoverySecretID("not-a-digest"); err == nil {
		t.Fatal("invalid public address accepted")
	}
}

// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"bytes"
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// Real hybrid envelopes and SDK public signatures; the outer Megolm/accepted
// Source remain the explicitly synthetic metadata witness of this fixture.
func TestBootstrapAcceptedReadRequiresCurrentBOriginalSourceAndBothExactDescriptors(t *testing.T) {
	c := preparePolicyRestoreCase(t)
	defer clear(c.backup)
	defer clear(c.anchorRaw)
	policy, err := restorePolicyCase(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := policy.Access(t.Context(), resource.Read)
	if err != nil {
		t.Fatal(err)
	}
	fence := resourceFence(activeResourceFence)
	window, err := policy.Window(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	handle, err := resource.RestoreBootstrapRecovery(t.Context(), c.backup, c.source.Retained.Capture.PlanSHA256, c.current.Scope(), window)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	var baselineProof resource.VerifiedBootstrapRead
	for _, kind := range []resource.BootstrapKind{resource.BootstrapBaseline, resource.BootstrapCheckpoint} {
		proof, err := policy.Accepted(t.Context(), kind, reader, fence)
		if err != nil || proof.Publication().Actor == c.current.Actor() || proof.Context().CreatorScope != c.current.Scope() {
			t.Fatal("accepted read impersonated A or lost original context", err)
		}
		actual, err := handle.OpenBootstrapFrame(t.Context(), proof, c.envelopes[kind], reader, fence)
		if err != nil || !bytes.Equal(actual, c.payloads[kind]) {
			t.Fatal("current B did not open exact original accepted descriptor", kind, err)
		}
		clear(actual)
		if kind == resource.BootstrapBaseline {
			baselineProof = proof
		}
	}
	if out, err := handle.OpenBootstrapFrame(t.Context(), baselineProof, c.envelopes[resource.BootstrapCheckpoint], reader, fence); err == nil || len(out) != 0 {
		t.Fatal("baseline proof opened a checkpoint envelope")
	}
	if out, err := handle.OpenBootstrapFrame(t.Context(), resource.VerifiedBootstrapRead{}, c.envelopes[resource.BootstrapBaseline], reader, fence); err == nil || len(out) != 0 {
		t.Fatal("public/empty accepted proof opened a private descriptor")
	}
	public, err := pqc.ParseRecipient(pqc.HybridProfile, reader.Epoch().Value().Recipient)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := resource.SealBootstrapDescriptor(public, baselineProof.Context(), []byte("replacement private intent under same context"))
	if err != nil {
		t.Fatal(err)
	}
	if out, err := handle.OpenBootstrapFrame(t.Context(), baselineProof, replacement, reader, fence); err == nil || len(out) != 0 {
		t.Fatal("accepted proof opened a freshly encrypted replacement descriptor")
	}
	calls := 0
	postFence := resourceFence(func(context.Context, resource.VerifiedAccess) error {
		calls++
		if calls == 2 {
			c.source.withdraw = true
		}
		return nil
	})
	if out, err := handle.OpenBootstrapFrame(t.Context(), baselineProof, c.envelopes[resource.BootstrapBaseline], reader, postFence); err == nil || len(out) != 0 || calls != 2 {
		t.Fatal("post-decrypt original Source fence did not stop descriptor release", calls, err)
	}
	c.source.withdraw = false
	c.source.Retained.Checkpoint.State = "sending"
	if _, err := policy.Accepted(t.Context(), resource.BootstrapBaseline, reader, fence); err == nil {
		t.Fatal("one accepted slot replaced both original accepted descriptors")
	}
	if out, err := handle.OpenBootstrapFrame(t.Context(), baselineProof, c.envelopes[resource.BootstrapBaseline], reader, fence); err == nil || len(out) != 0 {
		t.Fatal("retained accepted proof bypassed changed original Source")
	}
}

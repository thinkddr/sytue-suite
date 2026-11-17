// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
)

func TestBootstrapPeerRequiresWholeSignedStreamAndCurrentLeaf(t *testing.T) {
	c := prepareCaptureCase(t)
	query := resource.BootstrapPeerQuery{Plan: c.plan, Manifest: c.manifest, Scope: c.reader.Scope(), DeviceID: c.reader.Actor().DeviceID}
	if !query.Valid() {
		t.Fatal("valid public selector rejected")
	}
	mismatchedPlan := c.plan
	mismatchedPlan.MembershipSHA256 = c.base.plan.MembershipSHA256
	mismatchedPlan.Signatures = c.base.bootstrap.Sign(mismatchedPlan.BootstrapPlan)
	mismatchedManifest, _, err := resource.PrepareBootstrapRosterManifest(mismatchedPlan.BootstrapPlan, c.inputs)
	if err != nil {
		t.Fatal(err)
	}
	mismatchedSigned := resource.SignedBootstrapRosterManifest{BootstrapRosterManifest: mismatchedManifest, Signatures: c.base.bootstrap.Sign(mismatchedManifest)}
	mismatchedPending, err := resource.VerifyBootstrapRosterManifest(t.Context(), mismatchedPlan, mismatchedSigned, c.base.current, c.base.source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resource.BootstrapRosterPeer(t.Context(), mismatchedPending, capturePages{inputs: c.inputs}, query.Scope, query.DeviceID); err == nil {
		t.Fatal("manifest ignored independent plan membership commitment")
	}
	input, err := resource.BootstrapRosterPeer(t.Context(), c.pending, capturePages{inputs: c.inputs}, query.Scope, query.DeviceID)
	if err != nil || input.Member.Scope != c.reader.Scope() {
		t.Fatal("whole manifest did not select peer", err)
	}
	if err := resource.CheckBootstrapPeerSession(t.Context(), input, c.reader); err != nil {
		t.Fatal(err)
	}
	if _, err := resource.BootstrapRosterPeer(t.Context(), c.pending, capturePages{inputs: c.inputs[:1]}, query.Scope, query.DeviceID); err == nil {
		t.Fatal("partial stream disclosed peer")
	}
	changed := capturePages{inputs: c.inputs, change: func(p *resource.BootstrapRosterPage) {
		if p.Offset == "1" {
			p.Inputs[0].Member.Role = "different-role"
		}
	}}
	if _, err := resource.BootstrapRosterPeer(t.Context(), c.pending, changed, query.Scope, query.DeviceID); err == nil {
		t.Fatal("unsigned stream disclosed peer")
	}
	if _, err := resource.BootstrapRosterPeer(t.Context(), c.pending, capturePages{inputs: c.inputs}, query.Scope, "absent-device"); err == nil {
		t.Fatal("unlisted peer disclosed")
	}
	if err := resource.CheckBootstrapPeerSession(t.Context(), input, c.base.current); err == nil {
		t.Fatal("different current session replaced peer leaf")
	}
	for _, mutate := range []func(*resource.BootstrapRosterInput){
		func(p *resource.BootstrapRosterInput) { p.Member.RootKey = c.base.current.Actor().RootKey },
		func(p *resource.BootstrapRosterInput) { p.Member.UserID = c.base.current.Actor().UserID },
		func(p *resource.BootstrapRosterInput) { p.Member.ProductKeyID = c.base.current.Binding().ProductKeyID },
		func(p *resource.BootstrapRosterInput) {
			p.Member.EnrollmentSHA256 = c.base.current.Binding().EnrollmentSHA256
		},
	} {
		forged := input
		mutate(&forged)
		if err := resource.CheckBootstrapPeerSession(t.Context(), forged, c.reader); err == nil {
			t.Fatal("roster replaced independent current binding")
		}
	}
	c.readerSource.Retire()
	if err := resource.CheckBootstrapPeerSession(t.Context(), input, c.reader); err == nil {
		t.Fatal("retired peer remained current")
	}
}

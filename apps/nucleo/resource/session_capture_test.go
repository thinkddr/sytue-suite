// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"context"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

type atomicCurrentAuthority struct {
	source                               *resourcetest.SessionSource
	captures, reads, checks, directories int
	change                               func(*resource.SessionRecord)
	deviceOverride                       *matrixproof.Device
}

func (s *atomicCurrentAuthority) CaptureCurrentResourceSession(ctx context.Context, scope storage.Scope, device string) (resource.SessionRecord, matrixproof.Device, error) {
	s.captures++
	record, err := s.source.CurrentResourceSession(ctx, scope, device)
	if err != nil {
		return resource.SessionRecord{}, matrixproof.Device{}, err
	}
	verified, err := s.source.ResourceSessionDevice(ctx, scope, device, record.PinnedRoot)
	if err != nil {
		return resource.SessionRecord{}, matrixproof.Device{}, err
	}
	if s.change != nil {
		s.change(&record)
	}
	if s.deviceOverride != nil {
		verified = *s.deviceOverride
	}
	return record, verified, nil
}

func (s *atomicCurrentAuthority) CurrentResourceSession(ctx context.Context, scope storage.Scope, device string) (resource.SessionRecord, error) {
	s.reads++
	return s.source.CurrentResourceSession(ctx, scope, device)
}

func (s *atomicCurrentAuthority) CheckResourceSession(ctx context.Context, binding resource.SessionBinding) error {
	s.checks++
	return s.source.CheckResourceSession(ctx, binding)
}

func (s *atomicCurrentAuthority) ResourceSessionDevice(ctx context.Context, scope storage.Scope, device, root string) (matrixproof.Device, error) {
	s.directories++
	return s.source.ResourceSessionDevice(ctx, scope, device, root)
}

func TestAtomicCurrentResolveVerifiesOneCaptureAndFutureChecksStayLive(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "atomic-current", Product: "docs"}
	chain := matrixprooftest.NewUser(t, scope, "matrix.test", "@sytue_atomic-current:matrix.test", "ATOMIC_CURRENT")
	leaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	_, registry := resourcetest.EnrolledSession(t, chain, leaf)
	authority := &atomicCurrentAuthority{source: registry}
	provider, err := resource.NewCurrentSessionProvider(authority)
	if err != nil {
		t.Fatal(err)
	}
	current, err := provider.Resolve(t.Context(), scope, registry.Record.Binding.DeviceID)
	if err != nil || current.Binding() != registry.Record.Binding {
		t.Fatal("single capture did not pass actual enrollment/SDK verifier", err)
	}
	if authority.captures != 1 || authority.reads != 0 || authority.checks != 0 || authority.directories != 0 {
		t.Fatal("Resolve repeated live requests", authority)
	}
	if err := current.Revalidate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if authority.captures != 1 || authority.checks != 2 || authority.directories != 1 {
		t.Fatal("future Revalidate reused captured authority", authority)
	}
	for name, change := range map[string]func(*resource.SessionRecord){
		"scope":     func(r *resource.SessionRecord) { r.Binding.Scope.Subject = "other-scope" },
		"device":    func(r *resource.SessionRecord) { r.Binding.DeviceID = "other-device" },
		"pin":       func(r *resource.SessionRecord) { r.PinnedRoot = chain.Self.PublicKey().String() },
		"proof":     func(r *resource.SessionRecord) { r.ProofSHA256 = strings.Repeat("f", 64) },
		"signature": func(r *resource.SessionRecord) { r.Proof.Signatures = nil },
	} {
		t.Run(name, func(t *testing.T) {
			authority.change = change
			if _, err := provider.Resolve(t.Context(), scope, registry.Record.Binding.DeviceID); err == nil {
				t.Fatal("captured JSON bypassed current SDK/enrollment/scope verifier")
			}
		})
	}
	otherScope := scope
	otherScope.Subject = "different-human"
	otherChain := matrixprooftest.NewUser(t, otherScope, "matrix.test", "@sytue_different-human:matrix.test", "OTHER_CURRENT")
	otherLeaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	other, otherRegistry := resourcetest.EnrolledSession(t, otherChain, otherLeaf)
	otherDevice := other.Device()
	authority.change = func(r *resource.SessionRecord) { *r = otherRegistry.Record }
	authority.deviceOverride = &otherDevice
	if _, err := provider.Resolve(t.Context(), scope, registry.Record.Binding.DeviceID); err == nil {
		t.Fatal("independently valid cross-scope SDK capture replaced requested identity")
	}
	authority.deviceOverride = nil
	authority.change = nil
	separate, err := resource.NewSessionProvider(authority, authority)
	if err != nil {
		t.Fatal(err)
	}
	before := authority.captures
	if _, err := separate.Resolve(t.Context(), scope, registry.Record.Binding.DeviceID); err != nil {
		t.Fatal(err)
	}
	if authority.captures != before || authority.reads != 1 {
		t.Fatal("registry silently replaced separately configured directory")
	}
	registry.Retire()
	if err := current.Revalidate(t.Context()); err == nil {
		t.Fatal("returned session retained request-local permission witness")
	}
}

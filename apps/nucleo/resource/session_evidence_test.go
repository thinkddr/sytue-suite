// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

type publicSessionEvidence struct {
	registry *resourcetest.SessionSource
	chain    *matrixprooftest.Fixture
	calls    int
	mutate   func(*resource.SessionEvidence)
}

func (s *publicSessionEvidence) CurrentResourceSessionEvidence(ctx context.Context, scope storage.Scope, device string) (resource.SessionEvidence, error) {
	s.calls++
	record, err := s.registry.CurrentResourceSession(ctx, scope, device)
	if err != nil {
		return resource.SessionEvidence{}, err
	}
	out := resource.SessionEvidence{Record: record, Origin: s.chain.Origin}
	if s.mutate != nil {
		s.mutate(&out)
	}
	return out, nil
}

func TestPublicSessionEvidenceUsesSameProviderAndIndependentCanonicalOrigin(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "public-peer", Product: "docs"}
	chain := matrixprooftest.NewUser(t, scope, "matrix.test", "@sytue_public-peer:matrix.test", "PUBLIC_PEER_B")
	leaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	original, registry := resourcetest.EnrolledSession(t, chain, leaf)
	source := &publicSessionEvidence{registry: registry, chain: chain}
	provider, err := resource.NewSessionEvidenceProvider(source, "matrix.test")
	if err != nil {
		t.Fatal(err)
	}
	current, err := provider.Resolve(t.Context(), scope, original.Actor().DeviceID)
	if err != nil || current.Binding() != original.Binding() {
		t.Fatal("public evidence did not use existing verifier", err)
	}
	if source.calls != 1 {
		t.Fatal("Resolve fetched identical public evidence repeatedly", source.calls)
	}
	before := source.calls
	if err := current.Revalidate(t.Context()); err != nil || source.calls-before != 3 {
		t.Fatal("revalidation reused public JSON", err)
	}
	for name, mutate := range map[string]func(*resource.SessionEvidence){
		"record-root":     func(v *resource.SessionEvidence) { v.Record.PinnedRoot = chain.Self.PublicKey().String() },
		"server":          func(v *resource.SessionEvidence) { v.Origin.ServerName = "attacker.test" },
		"human-prefix":    func(v *resource.SessionEvidence) { v.Record.Binding.UserID = "@public-peer:matrix.test" },
		"sdk-signatures":  func(v *resource.SessionEvidence) { v.Origin.Keys.DeviceKeys = nil },
		"root-generation": func(v *resource.SessionEvidence) { v.Record.RootGeneration = "42" },
	} {
		t.Run(name, func(t *testing.T) {
			source.mutate = mutate
			if _, err := provider.Resolve(t.Context(), scope, original.Actor().DeviceID); err == nil {
				t.Fatal("public response replaced independent canonical verification")
			}
		})
	}
	source.mutate = nil
	if _, err := resource.NewSessionEvidenceProvider(source, "attacker.test"); err != nil {
		t.Fatal(err)
	}
	wrongServer, _ := resource.NewSessionEvidenceProvider(source, "attacker.test")
	if _, err := wrongServer.Resolve(t.Context(), scope, original.Actor().DeviceID); err == nil {
		t.Fatal("response selected its own homeserver")
	}
	registry.Retire()
	if err := current.Revalidate(t.Context()); err == nil {
		t.Fatal("retired session revived from public evidence")
	}
}

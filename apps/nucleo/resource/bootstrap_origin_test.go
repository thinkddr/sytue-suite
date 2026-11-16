// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

type bootstrapOriginFixture struct {
	hold       resource.BootstrapOriginHold
	reads      int
	withdrawAt int
}

func (s *bootstrapOriginFixture) BootstrapOriginHold(ctx context.Context, key resource.Resource, plan string, kind resource.BootstrapKind) (resource.BootstrapOriginHold, error) {
	s.reads++
	if ctx.Err() != nil {
		return resource.BootstrapOriginHold{}, ctx.Err()
	}
	if (s.withdrawAt > 0 && s.reads >= s.withdrawAt) || key != s.hold.Resource || plan != s.hold.PlanID || kind != s.hold.Kind {
		return resource.BootstrapOriginHold{}, resource.ErrRevoked
	}
	return s.hold, nil
}

func TestResourceBootstrapOriginRequiresOriginalDurableHoldAndCurrentProducer(t *testing.T) {
	f := prepareBootstrapPlanFixture(t)
	c := bootstrapCandidateFixture(t, f, resource.BootstrapBaseline, nil)
	verified, err := resource.VerifyBootstrapCandidate(t.Context(), c, f.verified, nil)
	if err != nil {
		t.Fatal(err)
	}
	value := f.verified.Value()
	source := &bootstrapOriginFixture{hold: resource.BootstrapOriginHold{Resource: value.Source.Resource, PlanID: value.ID, PlanSHA256: f.verified.SHA256(), Phase: "preparing", Source: value.Source, SessionBindingSHA256: value.SessionBindingSHA256, SessionGeneration: f.current.Binding().Generation, RoomID: value.RoomID, Actor: value.Creator, Kind: resource.BootstrapBaseline, State: "sending", TransactionID: c.Proof.TransactionID, CandidateSHA256: verified.SHA256(), ContextSHA256: c.Proof.ContextSHA256, OuterCipherSHA256: c.Proof.OuterCipherSHA256}}
	generation, _ := f.current.Binding().Generation.Int64()
	unsigned := publication.Request{Version: 1, Domain: "org.sytue.publication.admission.v1", Namespace: "resources:" + value.Source.Resource.Product, Scope: value.CreatorScope, Target: storage.PublicationTarget{RoomID: value.RoomID, DeviceID: value.Creator.DeviceID}, TransactionID: c.Proof.TransactionID, CipherSHA256: c.Proof.OuterCipherSHA256, AuthorityGeneration: generation}
	sign := func(v publication.Request) publication.SignedRequest {
		return publication.SignedRequest{Request: v, Signatures: matrixprooftest.Sign(t, f.f.Chain.Device, v, f.f.Chain.Origin.UserID, value.Creator.DeviceID)}
	}
	signed := sign(unsigned)
	permit, err := resource.VerifyBootstrapOriginRequest(t.Context(), verified, f.current, signed, source)
	if err != nil || permit.Request().Value() != unsigned || permit.SessionBinding() != f.current.Binding() || permit.PlanSHA256() != f.verified.SHA256() || permit.CandidateSHA256() != verified.SHA256() {
		t.Fatal("original durable bootstrap denied", err)
	}
	original := source.hold
	for _, fault := range []string{"prepared", "accepted", "finalized", "resource", "plan", "plan-digest", "source-epoch", "source-version", "binding", "generation", "room", "actor", "kind", "transaction", "candidate", "context", "cipher"} {
		t.Run(fault, func(t *testing.T) {
			changed := original
			switch fault {
			case "prepared":
				changed.State = "prepared"
			case "accepted":
				changed.State = "accepted"
			case "finalized":
				changed.Phase = "active"
			case "resource":
				changed.Resource.Tenant = "other"
			case "plan":
				changed.PlanID = "00000000-0000-4000-8000-000000000001"
			case "plan-digest":
				changed.PlanSHA256 = f.f.Epoch.SHA256()
			case "source-epoch":
				changed.Source.SourceEpoch = "00000000-0000-4000-8000-000000000001"
			case "source-version":
				changed.Source.ACLVersion = "9"
			case "binding":
				changed.SessionBindingSHA256 = f.f.Epoch.SHA256()
			case "generation":
				changed.SessionGeneration = "2"
			case "room":
				changed.RoomID = "!foreign:example.test"
			case "actor":
				changed.Actor.DeviceID = "OTHER"
			case "kind":
				changed.Kind = resource.BootstrapCheckpoint
			case "transaction":
				changed.TransactionID = "00000000-0000-4000-8000-000000000001"
			case "candidate":
				changed.CandidateSHA256 = f.f.Epoch.SHA256()
			case "context":
				changed.ContextSHA256 = f.f.Epoch.SHA256()
			case "cipher":
				changed.OuterCipherSHA256 = f.f.Epoch.SHA256()
			}
			source.hold = changed
			if _, err := resource.VerifyBootstrapOriginRequest(t.Context(), verified, f.current, signed, source); err == nil {
				t.Fatal("changed original slot accepted", fault)
			}
			source.hold = original
		})
	}
	for _, fault := range []string{"namespace", "scope", "room", "device", "transaction", "cipher", "generation", "signature"} {
		t.Run("request-"+fault, func(t *testing.T) {
			changed := unsigned
			switch fault {
			case "namespace":
				changed.Namespace = "storage:" + value.Source.Resource.Product
			case "scope":
				changed.Scope.Subject = "other"
			case "room":
				changed.Target.RoomID = "!foreign:example.test"
			case "device":
				changed.Target.DeviceID = "OTHER"
			case "transaction":
				changed.TransactionID = "00000000-0000-4000-8000-000000000001"
			case "cipher":
				changed.CipherSHA256 = f.f.Epoch.SHA256()
			case "generation":
				changed.AuthorityGeneration++
			}
			bad := sign(changed)
			if fault == "signature" {
				bad.Signatures = nil
			}
			if _, err := resource.VerifyBootstrapOriginRequest(t.Context(), verified, f.current, bad, source); err == nil {
				t.Fatal("changed request accepted", fault)
			}
		})
	}
	source.hold.State = "ambiguous"
	if err := permit.Revalidate(t.Context()); err != nil {
		t.Fatal("retained ambiguous hold rejected", err)
	}
	source.withdrawAt = source.reads + 2
	if _, err := resource.VerifyBootstrapOriginRequest(t.Context(), verified, f.current, signed, source); err == nil {
		t.Fatal("late source withdrawal accepted")
	}
	source.withdrawAt = 0
	f.registry.Retire()
	if err := permit.Revalidate(t.Context()); err == nil {
		t.Fatal("retained bootstrap revived withdrawn producer")
	}
	if _, err := resource.VerifyBootstrapOriginRequest(t.Context(), verified, resource.VerifiedSession{}, signed, source); err == nil {
		t.Fatal("zero live session accepted")
	}
	if _, err := resource.VerifyBootstrapOriginRequest(t.Context(), resource.VerifiedBootstrapCandidate{}, f.current, signed, source); err == nil {
		t.Fatal("zero bootstrap accepted")
	}
}

// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
)

type bootstrapSourceFixture struct {
	snapshot   resource.BootstrapSourceSnapshot
	checks     int
	withdrawAt int
}

func (s *bootstrapSourceFixture) BootstrapSource(ctx context.Context, key resource.Resource) (resource.BootstrapSourceSnapshot, error) {
	if key != s.snapshot.Resource {
		return resource.BootstrapSourceSnapshot{}, resource.ErrRevoked
	}
	return s.snapshot, ctx.Err()
}

func (s *bootstrapSourceFixture) CheckBootstrapSource(ctx context.Context, expected resource.BootstrapSourceSnapshot) error {
	s.checks++
	if expected != s.snapshot || s.withdrawAt > 0 && s.checks >= s.withdrawAt {
		return resource.ErrRevoked
	}
	return ctx.Err()
}

func TestResourceBootstrapPlanRequiresOriginalFrozenOwnerAndCurrentDevice(t *testing.T) {
	fixture := prepareBootstrapPlanFixture(t)
	f, source, current, registry, plan, verified, sign := fixture.f, fixture.source, fixture.current, fixture.registry, fixture.plan, fixture.verified, fixture.sign
	baselineCtx, err := verified.BaselineContext()
	if err != nil || baselineCtx.SourceCutover.Seq != plan.Source.SourceSeq || baselineCtx.SourceCutover.HeadCipherSHA256 != "" || baselineCtx.Creator != current.Actor() {
		t.Fatal("baseline invented policy/reference or lost int64", err)
	}
	for _, fault := range []string{"owner", "source-epoch", "source-seq", "acl", "binding", "signature", "roster", "current"} {
		t.Run(fault, func(t *testing.T) {
			changed := plan
			roster := f.Roster
			session := current
			switch fault {
			case "owner":
				changed.Source.OwnerScope.Subject = "another"
				changed.CreatorScope = changed.Source.OwnerScope
			case "source-epoch":
				changed.Source.SourceEpoch = uuid.NewString()
			case "source-seq":
				changed.Source.SourceSeq = "1"
			case "acl":
				changed.Source.ACLVersion = "1"
			case "binding":
				changed.SessionBindingSHA256 = f.Epoch.SHA256()
			case "roster":
				roster = resource.VerifiedRoster{}
			case "current":
				session = resource.VerifiedSession{}
			}
			candidate := sign(changed)
			if fault == "signature" {
				candidate.Signatures = nil
			}
			if _, err := resource.VerifyBootstrapPlan(t.Context(), candidate, roster, session, source); err == nil {
				t.Fatal("bootstrap metadata became authority", fault)
			}
		})
	}
	source.withdrawAt = source.checks + 1
	if err := verified.Revalidate(t.Context()); err == nil {
		t.Fatal("source withdrawal did not fence bootstrap")
	}
	source.withdrawAt = 0
	registry.Retire()
	if err := verified.Revalidate(t.Context()); err == nil {
		t.Fatal("retired device continued bootstrap")
	}
	if _, err := resource.VerifyBootstrapPlan(t.Context(), sign(plan), f.Roster, current, source); err == nil {
		t.Fatal("public plan revived producer")
	}
}

type bootstrapPlanCase struct {
	bootstrap resourcetest.BootstrapFixture
	f         resourcetest.Fixture
	source    *bootstrapSourceFixture
	current   resource.VerifiedSession
	registry  *resourcetest.SessionSource
	plan      resource.BootstrapPlan
	verified  resource.VerifiedBootstrapPlan
	sign      func(resource.BootstrapPlan) resource.SignedBootstrapPlan
}

func prepareBootstrapPlanFixture(t testing.TB) bootstrapPlanCase {
	t.Helper()
	f := resourcetest.New(t, false)
	current, registry := resourcetest.EnrolledSession(t, f.Chain, f.RecipientIdentity)
	source := &bootstrapSourceFixture{snapshot: resource.BootstrapSourceSnapshot{Resource: f.Epoch.Value().Resource, OwnerScope: f.Scope, History: resource.HistoryFull, SourceEpoch: uuid.NewString(), SourceSeq: "9007199254740993", ACLVersion: "9007199254740995"}}
	bootstrap := resourcetest.NewBootstrap(t, t.Context(), f, current, source, source.snapshot.Resource)
	f.Roster = bootstrap.Roster
	sign := func(value resource.BootstrapPlan) resource.SignedBootstrapPlan {
		return resource.SignedBootstrapPlan{BootstrapPlan: value, Signatures: bootstrap.Sign(value)}
	}
	return bootstrapPlanCase{bootstrap: bootstrap, f: f, source: source, current: current, registry: registry, plan: bootstrap.Signed.BootstrapPlan, verified: bootstrap.Verified, sign: sign}
}

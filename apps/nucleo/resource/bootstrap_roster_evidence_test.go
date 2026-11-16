// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

type retainedCaptureFixture struct {
	held         resource.RetainedBootstrapCapture
	records      resource.CapturedBootstrapRosterEvidence
	changeRecord bool
}

func (s *retainedCaptureFixture) RetainedBootstrapCapture(ctx context.Context, key resource.Resource, planID string) (resource.RetainedBootstrapCapture, error) {
	if key != s.held.Resource || planID != s.held.PlanID {
		return resource.RetainedBootstrapCapture{}, resource.ErrRevoked
	}
	return s.held, ctx.Err()
}

func (s *retainedCaptureFixture) PublicEvidence(ctx context.Context, key resource.Resource, sha string) (json.RawMessage, error) {
	if key != s.held.Resource {
		return nil, resource.ErrRevoked
	}
	record, err := s.records.Evidence(sha)
	if err != nil {
		return nil, err
	}
	if s.changeRecord {
		record.Value = []byte("{}")
	}
	return record.Value, ctx.Err()
}

func captureHistoricalCase(t testing.TB) (captureCase, *retainedCaptureFixture) {
	t.Helper()
	c := prepareCaptureCase(t)
	captured, err := resource.CaptureBootstrapRosterEvidence(t.Context(), c.pending)
	if err != nil {
		t.Fatal(err)
	}
	plan := c.pending.Plan()
	binding := captured.SessionBinding()
	held := resource.RetainedBootstrapCapture{State: "pending", Resource: plan.Source.Resource, PlanID: plan.ID, Source: plan.Source, Creator: plan.Creator, RoomID: plan.RoomID, SessionBindingSHA256: plan.SessionBindingSHA256, SessionGeneration: binding.Generation, RootGeneration: binding.RootGeneration, OwnerPin: captured.OwnerPin(), PlanSHA256: c.pending.Value().PlanSHA256, ManifestSHA256: c.pending.SHA256(), EvidenceSHA256: captured.SHA256(), Evidence: captured.Bytes()}
	return c, &retainedCaptureFixture{held: held, records: captured}
}

func TestBootstrapPendingCaptureHistoricalEvidenceNeverRevivesA(t *testing.T) {
	c, source := captureHistoricalCase(t)
	raw := source.records.Bytes()
	raw[0] = 'x'
	if bytes.Equal(raw, source.records.Bytes()) {
		t.Fatal("pending capture descriptor retained caller mutation")
	}
	for _, sha := range source.records.EvidenceHashes() {
		record, err := source.records.Evidence(sha)
		if err != nil {
			t.Fatal(err)
		}
		record.Value[0] = 'x'
		again, err := source.records.Evidence(sha)
		if err != nil || bytes.Equal(record.Value, again.Value) {
			t.Fatal("public evidence returned shared mutable bytes", err)
		}
	}
	// A is gone, but its earlier SDK evidence remains historical only.
	c.base.registry.Retire()
	if err := c.pending.Revalidate(t.Context()); err == nil {
		t.Fatal("retired producer remained current")
	}
	old, err := resource.RestoreBootstrapCapture(t.Context(), source.held.Resource, source.held.PlanID, source)
	if err != nil || old.Plan() != c.pending.Plan() {
		t.Fatal("historical pending SDK proof required live A", err)
	}
	bChain := matrixprooftest.NewDevice(t, c.base.f.Chain, "DOCS_CAPTURE_B")
	leaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	current, bSource := resourcetest.EnrolledSession(t, bChain, leaf)
	if current.Actor() == old.Plan().Creator || current.Binding().ProductKeyID == c.base.current.Binding().ProductKeyID {
		t.Fatal("fixture cloned old actor or hybrid leaf")
	}
	query, err := resource.PrepareBootstrapCaptureCancellation(t.Context(), current, old)
	if err != nil {
		t.Fatal(err)
	}
	signed := resource.SignedBootstrapCaptureCancellation{BootstrapCaptureCancellation: query, Signatures: matrixprooftest.Sign(t, bChain.Device, query, bChain.Origin.UserID, current.Actor().DeviceID)}
	verified, err := resource.VerifyBootstrapCaptureCancellation(t.Context(), signed, current, old, c.base.source)
	if err != nil || verified.Value().Actor != current.Actor() || verified.Value().Actor == old.Plan().Creator {
		t.Fatal("current B cancellation impersonated historical A", err)
	}
	if verified.OriginalEvidenceSHA256() != source.held.EvidenceSHA256 || verified.OriginalOwnerPin() != source.held.OwnerPin {
		t.Fatal("cancellation lost immutable original pin")
	}
	// An unrelated root at the same public user/name is not recovery.
	changedRoot := matrixprooftest.NewUser(t, current.Scope(), bChain.Origin.ServerName, bChain.Origin.UserID, "ROOT_RESET")
	reset, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	other, _ := resourcetest.EnrolledSession(t, changedRoot, reset)
	if _, err := resource.PrepareBootstrapCaptureCancellation(t.Context(), other, old); err == nil {
		t.Fatal("root reset silently authorized historical cancellation")
	}
	unsigned := signed
	unsigned.Signatures = nil
	if _, err := resource.VerifyBootstrapCaptureCancellation(t.Context(), unsigned, current, old, c.base.source); err == nil {
		t.Fatal("current public record replaced SDK B signature")
	}
	impersonated := signed
	impersonated.Actor = old.Plan().Creator
	impersonated.Target.DeviceID = old.Plan().Creator.DeviceID
	if _, err := resource.VerifyBootstrapCaptureCancellation(t.Context(), impersonated, current, old, c.base.source); err == nil {
		t.Fatal("current B could claim old producer A")
	}
	c.base.source.withdrawAt = c.base.source.checks + 1
	if err := verified.Revalidate(t.Context()); err == nil {
		t.Fatal("source freeze/ACL withdrawal did not fence cancellation")
	}
	c.base.source.withdrawAt = 0
	bSource.Retire()
	if err := verified.Revalidate(t.Context()); err == nil {
		t.Fatal("withdrawn B kept cancellation authority")
	}
}

func TestBootstrapPendingCaptureRejectsTamperedHistoricalSource(t *testing.T) {
	c, source := captureHistoricalCase(t)
	for _, fault := range []string{"state", "owner", "device", "root", "binding", "root-generation", "manifest", "descriptor", "record"} {
		t.Run(fault, func(t *testing.T) {
			changed := *source
			changed.held = source.held
			switch fault {
			case "state":
				changed.held.State = "complete"
			case "owner":
				changed.held.Source.OwnerScope.Subject = "another"
			case "device":
				changed.held.Creator.DeviceID = "ANOTHER_DEVICE"
			case "root":
				changed.held.OwnerPin.RootKey = id.Ed25519(c.reader.Actor().RootKey)
			case "binding":
				changed.held.SessionBindingSHA256 = c.plan.MembershipSHA256
			case "root-generation":
				changed.held.RootGeneration = "2"
			case "manifest":
				changed.held.ManifestSHA256 = c.plan.MembershipSHA256
			case "descriptor":
				changed.held.Evidence = append([]byte(nil), source.held.Evidence...)
				changed.held.Evidence[0] = 'x'
			case "record":
				changed.changeRecord = true
			}
			if _, err := resource.RestoreBootstrapCapture(t.Context(), changed.held.Resource, changed.held.PlanID, &changed); err == nil {
				t.Fatal("historical caller-selected metadata became authority", fault)
			}
		})
	}
}

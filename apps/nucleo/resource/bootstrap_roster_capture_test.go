// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

type capturePages struct {
	inputs []resource.BootstrapRosterInput
	onRead func(int)
	change func(*resource.BootstrapRosterPage)
}

func (p capturePages) BootstrapRosterPage(ctx context.Context, offset resource.Decimal) (resource.BootstrapRosterPage, error) {
	if err := ctx.Err(); err != nil {
		return resource.BootstrapRosterPage{}, err
	}
	n, err := offset.Int64()
	if err != nil || n >= int64(len(p.inputs)) {
		return resource.BootstrapRosterPage{}, resource.ErrPending
	}
	if p.onRead != nil {
		p.onRead(int(n))
	}
	var input resource.BootstrapRosterInput
	raw, _ := json.Marshal(p.inputs[n])
	_ = json.Unmarshal(raw, &input)
	page := resource.BootstrapRosterPage{Offset: offset, Inputs: []resource.BootstrapRosterInput{input}}
	if p.change != nil {
		p.change(&page)
	}
	return page, nil
}

type captureCase struct {
	base         bootstrapPlanCase
	reader       resource.VerifiedSession
	readerSource *resourcetest.SessionSource
	provider     *resource.SessionProvider
	plan         resource.SignedBootstrapPlan
	manifest     resource.SignedBootstrapRosterManifest
	pending      resource.VerifiedBootstrapRosterManifest
	inputs       []resource.BootstrapRosterInput
}

func prepareCaptureCase(t testing.TB) captureCase {
	t.Helper()
	f := prepareBootstrapPlanFixture(t)
	scope := f.current.Scope()
	scope.Subject = "reader"
	chain := matrixprooftest.NewUser(t, scope, "matrix.test", "@reader:matrix.test", "READER_DEVICE")
	leaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	reader, readerSource := resourcetest.EnrolledSession(t, chain, leaf)
	b := reader.Binding()
	member := resource.Member{Scope: b.Scope, UserID: b.UserID, RootKey: b.RootKey, Recipient: b.Recipient, ProductKeyID: b.ProductKeyID, EnrollmentSHA256: b.EnrollmentSHA256, Role: "viewer", Actions: []resource.Action{resource.Read}, From: resource.Position{Seq: "0"}}
	owner := f.f.Roster.Members()[0]
	members := []resource.Member{owner, member}
	membership, err := resource.MembershipDigest(f.plan.Source.Resource, members)
	if err != nil {
		t.Fatal(err)
	}
	plan := f.plan
	plan.MembershipSHA256 = membership
	signed := f.sign(plan)
	manifest, inputs, err := resource.PrepareBootstrapRosterManifest(plan, []resource.BootstrapRosterInput{{Member: member, Recipient: readerSource.Record.Recipient}, {Member: owner, Recipient: f.registry.Record.Recipient}})
	if err != nil {
		t.Fatal(err)
	}
	proof := resource.SignedBootstrapRosterManifest{BootstrapRosterManifest: manifest, Signatures: f.bootstrap.Sign(manifest)}
	pending, err := resource.VerifyBootstrapRosterManifest(t.Context(), signed, proof, f.current, f.source)
	if err != nil {
		t.Fatal(err)
	}
	directory := resourcetest.SessionSources{f.registry, readerSource}
	provider, err := resource.NewSessionProvider(directory, directory)
	if err != nil {
		t.Fatal(err)
	}
	return captureCase{base: f, reader: reader, readerSource: readerSource, provider: provider, plan: signed, manifest: proof, pending: pending, inputs: inputs}
}

func TestBootstrapRosterCaptureUsesEveryCurrentConsentAndSamePlan(t *testing.T) {
	c := prepareCaptureCase(t)
	plan, err := resource.CompleteBootstrapRosterCapture(t.Context(), c.pending, capturePages{inputs: c.inputs}, c.provider)
	if err != nil || plan.SHA256() != c.pending.Value().PlanSHA256 || plan.Value() != c.plan.BootstrapPlan || len(plan.CurrentRosterBindings()) != 2 {
		t.Fatal("complete current roster did not produce same plan", err)
	}
	bindings := plan.CurrentRosterBindings()
	bindings[0] = resource.SessionBinding{}
	if len(plan.CurrentRosterBindings()) != 2 || !plan.CurrentRosterBindings()[0].Valid() {
		t.Fatal("public bindings retained caller mutation")
	}
	raw, err := resource.BootstrapRosterPageBytes(c.plan.Source.Resource, resource.BootstrapRosterPage{Offset: "0", Inputs: c.inputs[:1]})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := resource.DecodeBootstrapRosterPage(c.plan.Source.Resource, raw)
	if err != nil || decoded.Offset != "0" || len(decoded.Inputs) != 1 {
		t.Fatal("canonical page roundtrip", err)
	}
	if _, err := resource.DecodeBootstrapRosterPage(c.plan.Source.Resource, append(raw, []byte(" {}")...)); err == nil {
		t.Fatal("noncanonical/trailing page accepted")
	}
	// Retiring a non-owner MUST invalidate both a future completion and an already
	// completed plan. Owner SDK, source freeze and all public signatures stay valid.
	c.readerSource.Retire()
	if err := c.pending.Revalidate(t.Context()); err != nil {
		t.Fatal("reader withdrawal unexpectedly withdrew owner intent", err)
	}
	if err := plan.Revalidate(t.Context()); err == nil {
		t.Fatal("retired non-owner remained authorized after completed capture")
	}
	if _, err := resource.CompleteBootstrapRosterCapture(t.Context(), c.pending, capturePages{inputs: c.inputs}, c.provider); err == nil {
		t.Fatal("retired non-owner became verified from retained public pages")
	}
}

func TestBootstrapRosterCaptureRejectsPartialMismatchAndReplays(t *testing.T) {
	c := prepareCaptureCase(t)
	for _, fault := range []string{"count", "digest", "order", "duplicate", "offset", "unsigned-manifest", "unsigned-plan", "source-drift", "cancel"} {
		t.Run(fault, func(t *testing.T) {
			signed := c.manifest
			plan := c.plan
			pages := capturePages{inputs: c.inputs}
			ctx := t.Context()
			switch fault {
			case "count":
				signed.MemberCount = "3"
				signed.Signatures = c.base.bootstrap.Sign(signed.BootstrapRosterManifest)
			case "digest":
				signed.OrderedInputsSHA256 = c.plan.MembershipSHA256
				signed.Signatures = c.base.bootstrap.Sign(signed.BootstrapRosterManifest)
			case "order":
				pages.inputs = []resource.BootstrapRosterInput{c.inputs[1], c.inputs[0]}
			case "duplicate":
				pages.inputs = []resource.BootstrapRosterInput{c.inputs[0], c.inputs[0]}
			case "offset":
				pages.change = func(p *resource.BootstrapRosterPage) { p.Offset = "1" }
			case "unsigned-manifest":
				signed.Signatures = nil
			case "unsigned-plan":
				plan.Signatures = nil
			case "source-drift":
				old := c.base.source.snapshot
				defer func() { c.base.source.snapshot = old }()
				pages.onRead = func(n int) {
					if n == 1 {
						c.base.source.snapshot.ACLVersion = "1"
					}
				}
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				pages.onRead = func(n int) {
					if n == 1 {
						cancel()
					}
				}
				defer cancel()
			}
			pending, err := resource.VerifyBootstrapRosterManifest(ctx, plan, signed, c.base.current, c.base.source)
			if err == nil {
				_, err = resource.CompleteBootstrapRosterCapture(ctx, pending, pages, c.provider)
			}
			if err == nil {
				t.Fatal("partial/mismatched public capture became verified plan", fault)
			}
			if fault == "count" && !errors.Is(err, resource.ErrPending) {
				t.Fatal("missing immutable page must remain pending", err)
			}
			if fault == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost original context", err)
			}
		})
	}
	// A restored complete source resumes with identical signed plan and pages.
	if _, err := resource.CompleteBootstrapRosterCapture(t.Context(), c.pending, capturePages{inputs: c.inputs}, c.provider); err != nil {
		t.Fatal("complete capture could not resume after partial attempt", err)
	}
}

func TestBootstrapRosterStreamIsCanonicalWithoutTotalMemberCap(t *testing.T) {
	c := prepareCaptureCase(t)
	stream, err := resource.NewBootstrapRosterStream(c.plan.Source.Resource)
	if err != nil {
		t.Fatal(err)
	}
	var inputs []resource.BootstrapRosterInput
	// This is a structural digest vector, deliberately NOT a verified roster or
	// fake SDK permission. Signature verification remains mandatory at completion.
	for i := 0; i < 1025; i++ {
		input := c.inputs[0]
		input.Member.Scope.Subject = fmt.Sprintf("member-%04d", i)
		input.Member.UserID = "@" + input.Member.Scope.Subject + ":matrix.test"
		input.Recipient.Scope = input.Member.Scope
		input.Recipient.UserID = input.Member.UserID
		input.Recipient.Issuer.UserID = input.Member.UserID
		input.Member.EnrollmentSHA256, err = matrixproof.RequestDigest(input.Recipient.RecipientEnrollment)
		if err != nil {
			t.Fatal(err)
		}
		if err := stream.Add(input); err != nil {
			t.Fatal("stream imposed total member cap", i, err)
		}
		inputs = append(inputs, input)
	}
	count, sha, err := stream.Finish()
	if err != nil || count != "1025" {
		t.Fatal("stream lost complete member count", count, err)
	}
	expected, err := matrixproof.RequestDigest(struct {
		Domain   string                          `json:"domain"`
		Inputs   []resource.BootstrapRosterInput `json:"inputs"`
		Resource resource.Resource               `json:"resource"`
	}{Domain: "org.sytue.resource.bootstrap.members.v1", Inputs: inputs, Resource: c.plan.Source.Resource})
	if err != nil || sha != expected {
		t.Fatal("stream differs from canonical Go JSON digest", err)
	}
	if err := stream.Add(c.inputs[0]); err == nil {
		t.Fatal("sealed stream accepted extra input")
	}
	if _, _, err := stream.Finish(); err == nil {
		t.Fatal("stream finished twice")
	}
}

// A live public session record is insufficient to query ORIGINAL source state.
// The owner intent must carry its maintained SDK signature first.
type captureSourceReadCounter struct {
	original resource.OriginalBootstrapSource
	reads    int
}

func (s *captureSourceReadCounter) BootstrapSource(ctx context.Context, key resource.Resource) (resource.BootstrapSourceSnapshot, error) {
	s.reads++
	return s.original.BootstrapSource(ctx, key)
}

func (s *captureSourceReadCounter) CheckBootstrapSource(ctx context.Context, value resource.BootstrapSourceSnapshot) error {
	return s.original.CheckBootstrapSource(ctx, value)
}

func TestBootstrapCaptureRejectsUnsignedPlanBeforeOriginalSourceLookup(t *testing.T) {
	c := prepareCaptureCase(t)
	source := &captureSourceReadCounter{original: c.base.source}
	unsigned := c.plan
	unsigned.Signatures = nil
	if _, err := resource.VerifyBootstrapRosterManifest(t.Context(), unsigned, c.manifest, c.base.current, source); err == nil || source.reads != 0 {
		t.Fatal("unsigned capture queried original source metadata", source.reads, err)
	}
}

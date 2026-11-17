//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

func TestOfficeResourceRosterCaptureStoresOriginalPagesAndCurrentRecipients(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	f := resourcetest.New(t, true)
	ctx := tenantCtx(f.Scope.Tenant)
	current, ownerRegistry := resourcetest.EnrolledSession(t, f.Chain, f.RecipientIdentity)
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "retained original", OwnerSub: f.Scope.Subject})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE documents SET seq=9007199254740993 WHERE tenant=$1 AND id=$2`, f.Scope.Tenant, doc.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	original, err := store.NewPrivateSource(st, officeObjects{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := original.Begin(ctx, f.Scope, uuid.New()); err != nil {
		t.Fatal(err)
	}
	key := resource.Resource{Tenant: f.Scope.Tenant, Product: "docs", Kind: "document", ID: doc.ID}
	source, err := store.NewResourceBootstrapSource(st, f.Scope)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := resourcetest.NewBootstrap(t, ctx, f, current, source, key)
	readerScope := f.Scope
	readerScope.Subject = "reader"
	readerChain := matrixprooftest.NewUser(t, readerScope, "matrix.test", "@reader:matrix.test", "READER_DOCS")
	readerLeaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	reader, readerRegistry := resourcetest.EnrolledSession(t, readerChain, readerLeaf)
	b := reader.Binding()
	member := resource.Member{Scope: b.Scope, UserID: b.UserID, RootKey: b.RootKey, Recipient: b.Recipient, ProductKeyID: b.ProductKeyID, EnrollmentSHA256: b.EnrollmentSHA256, Role: "viewer", Actions: []resource.Action{resource.Read}, From: resource.Position{Seq: "0"}}
	owner := bootstrap.Roster.Members()[0]
	membership, err := resource.MembershipDigest(key, []resource.Member{owner, member})
	if err != nil {
		t.Fatal(err)
	}
	value := bootstrap.Signed.BootstrapPlan
	value.MembershipSHA256 = membership
	build := func(plan resource.BootstrapPlan) (resource.VerifiedBootstrapRosterManifest, []resource.BootstrapRosterInput) {
		t.Helper()
		manifest, inputs, err := resource.PrepareBootstrapRosterManifest(plan, []resource.BootstrapRosterInput{{Member: member, Recipient: readerRegistry.Record.Recipient}, {Member: owner, Recipient: ownerRegistry.Record.Recipient}})
		if err != nil {
			t.Fatal(err)
		}
		signed := resource.SignedBootstrapPlan{BootstrapPlan: plan, Signatures: bootstrap.Sign(plan)}
		signedManifest := resource.SignedBootstrapRosterManifest{BootstrapRosterManifest: manifest, Signatures: bootstrap.Sign(manifest)}
		pending, err := resource.VerifyBootstrapRosterManifest(ctx, signed, signedManifest, current, source)
		if err != nil {
			t.Fatal(err)
		}
		return pending, inputs
	}
	pending, inputs := build(value)
	capture, err := store.NewResourceBootstrapCaptureStore(st, f.Scope.Subject)
	if err != nil {
		t.Fatal(err)
	}
	directory := resourcetest.SessionSources{ownerRegistry, readerRegistry}
	provider, err := resource.NewSessionProvider(directory, directory)
	if err != nil {
		t.Fatal(err)
	}
	status, err := capture.Begin(ctx, pending)
	if err != nil || status.State != "pending" || status.ReceivedCount != "0" || status.MemberCount != "2" {
		t.Fatal("pending capture acquired premature authority", err, status)
	}
	if again, err := capture.Begin(ctx, pending); err != nil || again != status {
		t.Fatal("capture begin lost exact idempotence", err)
	}
	first := resource.BootstrapRosterPage{Offset: "0", Inputs: inputs[:1]}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			got, err := capture.Append(ctx, pending, first)
			if err == nil && got.ReceivedCount != "1" {
				err = errors.New("duplicate page advanced progress twice")
			}
			results <- err
		})
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal("concurrent immutable append", err)
		}
	}
	if _, err := capture.Finalize(ctx, pending, provider); !errors.Is(err, resource.ErrPending) {
		t.Fatal("partial capture became a plan", err)
	}
	bChain := matrixprooftest.NewDevice(t, f.Chain, "DOCS_CAPTURE_RECOVERY_B")
	bLeaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	currentB, _ := resourcetest.EnrolledSession(t, bChain, bLeaf)
	cancelB := prepareOriginalCaptureBCancellation(t, ctx, capture, pending, currentB, bChain, source)
	if err := capture.CancelRecovered(ctx, cancelB); err != nil {
		t.Fatal(err)
	}
	legacyRelay, err := store.NewResourceRelay(st, &officeResourceGate{f: f}, f.Scope.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacyRelay.BeginBootstrap(ctx, completeOriginalBootstrapCurrent(t, ctx, bootstrap, source, ownerRegistry)); err == nil {
		t.Fatal("legacy BeginBootstrap escaped cancelled original capture fence")
	}
	if _, err := capture.Status(ctx, pending); !errors.Is(err, resource.ErrRevoked) {
		t.Fatal("cancelled capture became reusable", err)
	}
	if _, err := capture.Append(ctx, pending, resource.BootstrapRosterPage{Offset: "1", Inputs: inputs[1:]}); err == nil {
		t.Fatal("cancelled capture accepted another page")
	}
	// Start another immutable plan, retaining the cancelled evidence and source.
	value.ID = uuid.NewString()
	value.Epoch = "capture-" + uuid.NewString()
	pending, inputs = build(value)
	if _, err := capture.Begin(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if _, err := capture.Append(ctx, pending, resource.BootstrapRosterPage{Offset: "0", Inputs: inputs[:1]}); err != nil {
		t.Fatal(err)
	}
	changed := inputs[0]
	changed.Member.Role = "editor"
	if _, err := capture.Append(ctx, pending, resource.BootstrapRosterPage{Offset: "0", Inputs: []resource.BootstrapRosterInput{changed}}); !errors.Is(err, resource.ErrConflict) {
		t.Fatal("immutable existing page silently changed", err)
	}
	if _, err := capture.Append(ctx, pending, resource.BootstrapRosterPage{Offset: "2", Inputs: inputs[1:]}); err == nil {
		t.Fatal("noncontiguous capture accepted gap")
	}
	if _, err := capture.Append(ctx, pending, resource.BootstrapRosterPage{Offset: "1", Inputs: inputs[:1]}); err == nil {
		t.Fatal("SQL accepted duplicate member across page boundary")
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE resource_e2e_roster_captures SET received_count=2 WHERE tenant=$1 AND product='docs' AND kind='document' AND resource_id=$2 AND plan_id=$3`, key.Tenant, key.ID, pending.Plan().ID)
		return err
	}); err == nil {
		t.Fatal("SQL progress advanced without original pages")
	}
	if _, err := capture.Append(ctx, pending, resource.BootstrapRosterPage{Offset: "1", Inputs: inputs[1:]}); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE resource_e2e_roster_captures SET state='complete',finished_at=now() WHERE tenant=$1 AND product='docs' AND kind='document' AND resource_id=$2 AND plan_id=$3`, key.Tenant, key.ID, pending.Plan().ID)
		return err
	}); err == nil {
		t.Fatal("SQL page count replaced the shared VerifiedPlan")
	}
	// Crash AFTER the real shared plan/evidence insertion and capture transition.
	// The outer transaction must roll BOTH back; no synthetic verified value.
	interrupted := errors.New("synthetic interruption before database commit")
	rollback, err := resourcepg.NewBootstrapCaptureStore(resourcepg.Dependencies{
		ValidateResource: func(_ context.Context, v resource.Resource) error {
			if v != key {
				return resource.ErrInvalid
			}
			return nil
		},
		RequirePrivateRules:  true,
		CheckBootstrapSource: source.CheckBootstrapSourceTx,
		TenantTx: func(ctx context.Context, fn func(pgx.Tx) error) error {
			return h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
				if err := fn(tx); err != nil {
					return err
				}
				var n int
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM resource_e2e_bootstraps WHERE tenant=$1 AND product='docs' AND kind='document' AND resource_id=$2 AND plan_id=$3`, key.Tenant, key.ID, pending.Plan().ID).Scan(&n); err != nil {
					return err
				}
				if n > 0 {
					return interrupted
				}
				return nil
			})
		},
	}, f.Scope.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rollback.Finalize(ctx, pending, provider); !errors.Is(err, interrupted) {
		t.Fatal("commit-boundary fault was not reached", err)
	}
	status, err = capture.Status(ctx, pending)
	if err != nil || status.State != "pending" || status.ReceivedCount != "2" {
		t.Fatal("commit interruption left false completion", err, status)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM resource_e2e_bootstraps WHERE tenant=$1 AND product='docs' AND kind='document' AND resource_id=$2`, key.Tenant, key.ID).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Fatal("interrupted capture retained committed bootstrap")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cancelBeforeFinalize := prepareOriginalCaptureBCancellation(t, ctx, capture, pending, currentB, bChain, source)
	verified, err := capture.Finalize(ctx, pending, provider)
	if err != nil || verified.SHA256() != pending.Value().PlanSHA256 || len(verified.CurrentRosterBindings()) != 2 {
		t.Fatal("same current verified plan not committed atomically", err)
	}
	status, err = capture.Status(ctx, pending)
	if err != nil || status.State != "complete" {
		t.Fatal("capture completion missing", err)
	}
	if again, err := capture.Begin(ctx, pending); err != nil || again != status {
		t.Fatal("completed capture begin retry changed original status", err)
	}
	if err := capture.CancelRecovered(ctx, cancelBeforeFinalize); err == nil {
		t.Fatal("complete bootstrap with zero slots became metadata-only cancellation")
	}
	if _, err := capture.Finalize(ctx, pending, provider); err != nil {
		t.Fatal("completed capture cannot reverify/reconcile", err)
	}
	if _, err := capture.Status(tenantCtx("foreign"), pending); err == nil {
		t.Fatal("capture escaped original tenant")
	}
	other, err := store.NewResourceBootstrapCaptureStore(st, "foreign-owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Status(ctx, pending); err == nil {
		t.Fatal("capture escaped original owner")
	}
	// Completed status is NOT permanent recipient consent or routing permission.
	readerRegistry.Retire()
	if _, err := capture.Finalize(ctx, pending, provider); err == nil {
		t.Fatal("completed status replaced current recipient consent")
	}
	if err := verified.Revalidate(ctx); err == nil {
		t.Fatal("completed plan kept retired reader")
	}
	if err := source.CheckBootstrapSource(ctx, pending.Plan().Source); err != nil {
		t.Fatal("public capture retired/thawed retained original", err)
	}
}

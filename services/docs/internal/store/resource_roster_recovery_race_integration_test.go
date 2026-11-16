//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

type originalCaptureRecoveryCase struct {
	ctx          context.Context
	key          resource.Resource
	pending      resource.VerifiedBootstrapRosterManifest
	capture      *resourcepg.BootstrapCaptureStore
	provider     *resource.SessionProvider
	cancellation resource.VerifiedBootstrapCaptureCancellation
	registry     *resourcetest.SessionSource
	currentB     resource.VerifiedSession
	bChain       *matrixprooftest.Fixture
	original     resource.OriginalBootstrapSource
	inspect      func(func(pgx.Tx) error) error
}

func prepareOriginalCaptureRecoveryCase(t *testing.T) originalCaptureRecoveryCase {
	t.Helper()
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	f := resourcetest.New(t, true)
	ctx := tenantCtx(f.Scope.Tenant)
	current, registry := resourcetest.EnrolledSession(t, f.Chain, f.RecipientIdentity)
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "actual original race", OwnerSub: f.Scope.Subject})
	if err != nil {
		t.Fatal(err)
	}
	original, err := store.NewPrivateSource(st, officeObjects{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := original.Begin(ctx, f.Scope, uuid.New()); err != nil {
		t.Fatal(err)
	}
	key := resource.Resource{Tenant: f.Scope.Tenant, Product: f.Scope.Product, Kind: "document", ID: doc.ID}
	source, err := store.NewResourceBootstrapSource(st, f.Scope)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := resourcetest.NewBootstrap(t, ctx, f, current, source, key)
	manifest, inputs, err := resource.PrepareBootstrapRosterManifest(bootstrap.Signed.BootstrapPlan, []resource.BootstrapRosterInput{{Member: bootstrap.Roster.Members()[0], Recipient: registry.Record.Recipient}})
	if err != nil {
		t.Fatal(err)
	}
	signedManifest := resource.SignedBootstrapRosterManifest{BootstrapRosterManifest: manifest, Signatures: bootstrap.Sign(manifest)}
	pending, err := resource.VerifyBootstrapRosterManifest(ctx, bootstrap.Signed, signedManifest, current, source)
	if err != nil {
		t.Fatal(err)
	}
	capture, err := store.NewResourceBootstrapCaptureStore(st, f.Scope.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := capture.Begin(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if _, err := capture.Append(ctx, pending, resource.BootstrapRosterPage{Offset: "0", Inputs: inputs}); err != nil {
		t.Fatal(err)
	}
	provider, err := resource.NewSessionProvider(registry, registry)
	if err != nil {
		t.Fatal(err)
	}
	bChain := matrixprooftest.NewDevice(t, f.Chain, "DOCS_RACE_B")
	bLeaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	currentB, _ := resourcetest.EnrolledSession(t, bChain, bLeaf)
	cancellation := prepareOriginalCaptureBCancellation(t, ctx, capture, pending, currentB, bChain, source)
	return originalCaptureRecoveryCase{ctx: ctx, key: key, pending: pending, capture: capture, provider: provider, cancellation: cancellation, registry: registry, currentB: currentB, bChain: bChain, original: source, inspect: func(fn func(pgx.Tx) error) error { return h.Pool.InTenantTx(ctx, fn) }}
}

func TestOfficeResourcePendingCaptureFinalizeAndCurrentBCancelHaveOneWinner(t *testing.T) {
	c := prepareOriginalCaptureRecoveryCase(t)
	ctx, key, pending, capture, provider, cancellation := c.ctx, c.key, c.pending, c.capture, c.provider, c.cancellation
	start := make(chan struct{})
	finalResult := make(chan error, 1)
	cancelResult := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Go(func() { <-start; _, err := capture.Finalize(ctx, pending, provider); finalResult <- err })
	wg.Go(func() { <-start; cancelResult <- capture.CancelRecovered(ctx, cancellation) })
	close(start)
	wg.Wait()
	finalErr, cancelErr := <-finalResult, <-cancelResult
	winners := 0
	if finalErr == nil {
		winners++
	}
	if cancelErr == nil {
		winners++
	}
	if winners != 1 {
		t.Fatal("original Source Finalize/Cancel did not have exactly one winner", finalErr, cancelErr)
	}
	if err := c.inspect(func(tx pgx.Tx) error {
		var state string
		var bootstraps, slots int
		if err := tx.QueryRow(ctx, `SELECT state FROM resource_e2e_roster_captures WHERE tenant=$1 AND product=$2 AND kind=$3 AND resource_id=$4 AND plan_id=$5`, key.Tenant, key.Product, key.Kind, key.ID, pending.Plan().ID).Scan(&state); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*)FROM resource_e2e_bootstraps WHERE tenant=$1 AND product=$2 AND kind=$3 AND resource_id=$4`, key.Tenant, key.Product, key.Kind, key.ID).Scan(&bootstraps); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*)FROM resource_e2e_bootstrap_slots WHERE tenant=$1 AND product=$2 AND kind=$3 AND resource_id=$4`, key.Tenant, key.Product, key.Kind, key.ID).Scan(&slots); err != nil {
			return err
		}
		if slots != 0 {
			t.Fatal("metadata race manufactured a publication slot")
		}
		if finalErr == nil && (state != "complete" || bootstraps != 1) {
			t.Fatal("Finalize won without atomic bootstrap")
		}
		if cancelErr == nil && (state != "cancelled" || bootstraps != 0) {
			t.Fatal("Cancel won but bootstrap escaped source fence")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOfficeResourcePendingCaptureCurrentBCancelsAfterProducerRetirement(t *testing.T) {
	c := prepareOriginalCaptureRecoveryCase(t)
	c.registry.Retire()
	if err := c.pending.Revalidate(c.ctx); err == nil {
		t.Fatal("fixture old producer did not retire")
	}
	cancellation := prepareOriginalCaptureBCancellation(t, c.ctx, c.capture, c.pending, c.currentB, c.bChain, c.original)
	if err := c.capture.CancelRecovered(c.ctx, cancellation); err != nil {
		t.Fatal("current B required retired producer A to cancel pre-epoch capture", err)
	}
	if err := c.original.CheckBootstrapSource(c.ctx, c.pending.Plan().Source); err != nil {
		t.Fatal("metadata cancellation retired original source", err)
	}
}

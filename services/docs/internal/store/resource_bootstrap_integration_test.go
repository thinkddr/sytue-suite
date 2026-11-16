//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
	"git.sytue.eu/sytue/one/services/docs/db"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// This combines ORIGINAL Office source + real RLS/SQL + shared typed SDK
// verifiers. Megolm wire and AcceptedEvent source are synthetic here; the real
// origin/homeserver transport is a separate required activation proof.
func TestOfficeResourceBootstrapDurablePlanSlotsAndAtomicActivation(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	f := resourcetest.New(t, true)
	ctx := tenantCtx(f.Scope.Tenant)
	current, currentRegistry := resourcetest.EnrolledSession(t, f.Chain, f.RecipientIdentity)
	kind, _ := model.ProductKind(f.Scope.Product)
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: kind, Title: "retained original", OwnerSub: f.Scope.Subject})
	if err != nil {
		t.Fatal(err)
	}
	if err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE documents SET seq=9007199254740993 WHERE tenant=$1 AND id=$2`, f.Scope.Tenant, doc.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	original, err := store.NewPrivateSource(st, officeObjects{})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := original.Begin(ctx, f.Scope, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.NewResourceBootstrapSource(st, f.Scope)
	if err != nil {
		t.Fatal(err)
	}
	key := resource.Resource{Tenant: f.Scope.Tenant, Product: f.Scope.Product, Kind: string(kind), ID: doc.ID}
	bootstrap := resourcetest.NewBootstrap(t, ctx, f, current, source, key)
	gate := &officeResourceGate{f: f}
	relay, err := store.NewResourceRelay(st, gate, f.Scope.Subject)
	if err != nil {
		t.Fatal(err)
	}
	historicalPlan := bootstrap.Verified
	if _, err := relay.BeginBootstrap(ctx, historicalPlan); err == nil {
		t.Fatal("owner-only historical plan reached live backend Begin")
	}
	if _, err := relay.StageBootstrap(ctx, historicalPlan, resource.BootstrapCandidate{}, resource.VerifiedBootstrapCandidate{}); err == nil {
		t.Fatal("owner-only historical plan reached live backend Stage")
	}
	if _, err := relay.BootstrapOriginSource(ctx, historicalPlan); err == nil {
		t.Fatal("owner-only historical plan reached Origin source")
	}
	plan, capture := persistOriginalBootstrapCurrent(t, ctx, st, bootstrap, source, currentRegistry)
	bootstrap.Verified = plan
	value := plan.Value()
	initial, err := relay.BeginBootstrap(ctx, plan)
	if err != nil || initial.State != "preparing" || initial.PlanID != value.ID {
		t.Fatal("begin", err)
	}
	var initialized int
	countActive := func() {
		t.Helper()
		if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM resource_e2e_resources WHERE tenant=$1 AND resource_id=$2`, key.Tenant, key.ID).Scan(&initialized)
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := capture.AcceptedPolicySource(ctx, current, key, value.ID); err == nil {
		t.Fatal("complete capture with zero slots became an accepted policy")
	}
	countActive()
	if initialized != 0 {
		t.Fatal("plan fabricated a final epoch")
	}
	for range 2 {
		again, err := relay.BeginBootstrap(ctx, plan)
		if err != nil || again != initial {
			t.Fatal("plan retry replaced source/epoch", err)
		}
	}
	other := resourcetest.NewBootstrap(t, ctx, f, current, source, key)
	if _, err = relay.BeginBootstrap(ctx, completeOriginalBootstrapCurrent(t, ctx, other, source, currentRegistry)); !errors.Is(err, resource.ErrConflict) {
		t.Fatal("concurrent plan silently replaced original", err)
	}
	baseline := bootstrap.OpaqueCandidate(t, resource.BootstrapBaseline, nil)
	baselineVerified, err := resource.VerifyBootstrapCandidate(ctx, baseline, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	historicalCandidate, historicalCandidateErr := resource.VerifyBootstrapCandidate(ctx, baseline, historicalPlan, nil)
	if historicalCandidateErr != nil {
		t.Fatal("historical candidate proof unavailable", historicalCandidateErr)
	}
	if _, err := relay.StageBootstrap(ctx, historicalPlan, baseline, historicalCandidate); err == nil {
		t.Fatal("owner-only historical plan reached live backend Stage with valid candidate")
	}
	slot, err := relay.StageBootstrap(ctx, plan, baseline, baselineVerified)
	if err != nil || slot.State != "prepared" {
		t.Fatal("stage", err)
	}
	originSource := bootstrapOriginPreparedDenied(t, ctx, relay, f, plan, current, baseline, baselineVerified)
	// Rebuild the product relay exactly as after a crash. No local slot state is
	// needed to recover the original immutable SDK bytes or transaction.
	relay, err = store.NewResourceRelay(st, gate, f.Scope.Subject)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := relay.PendingBootstrap(ctx, plan, resource.BootstrapBaseline)
	if err != nil || retained.SHA256 != slot.SHA256 || string(retained.Candidate.EncryptedEvent) != string(baseline.EncryptedEvent) || retained.Candidate.Proof.TransactionID != baseline.Proof.TransactionID {
		t.Fatal("restart changed staged SDK intent", err)
	}
	type claim struct {
		slot  resourcepg.BootstrapSlot
		fresh bool
		err   error
	}
	claims := make(chan claim, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Go(func() {
			slot, fresh, err := relay.AcquireBootstrap(ctx, plan, resource.BootstrapBaseline)
			claims <- claim{slot, fresh, err}
		})
	}
	group.Wait()
	close(claims)
	freshCount := 0
	for result := range claims {
		if result.err != nil || result.slot.State != "sending" {
			t.Fatal("acquire", result.err)
		}
		if result.fresh {
			freshCount++
		}
	}
	if freshCount != 1 {
		t.Fatal("multiple dispatchers acquired same original send", freshCount)
	}
	if err = relay.MarkBootstrapAmbiguous(ctx, key, value.ID, plan.SHA256(), resource.BootstrapBaseline); err != nil {
		t.Fatal(err)
	}
	held, fresh, err := relay.AcquireBootstrap(ctx, plan, resource.BootstrapBaseline)
	if err != nil || fresh || held.State != "ambiguous" {
		t.Fatal("ambiguous attempt became fresh send", err)
	}
	originPermit := bootstrapOriginAmbiguousAllowed(t, ctx, originSource, f, plan, current, baseline, baselineVerified)
	for _, query := range []string{`UPDATE resource_e2e_bootstraps SET source_seq=1 WHERE tenant=$1 AND resource_id=$2`, `UPDATE resource_e2e_bootstrap_slots SET state='prepared' WHERE tenant=$1 AND resource_id=$2`, `UPDATE resource_e2e_bootstrap_slots SET encrypted_event=convert_to('{}','UTF8')WHERE tenant=$1 AND resource_id=$2`, `DELETE FROM resource_e2e_bootstrap_slots WHERE tenant=$1 AND resource_id=$2`} {
		if err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, query, key.Tenant, key.ID); return err }); !e2eFenceBlocked(err) {
			t.Fatal("bootstrap evidence mutable/released without proof", err)
		}
	}
	baselineRef := storage.Reference{RoomID: value.RoomID, EventID: "$baseline-sdk-accepted"}
	gate.accepted = &event.Event{ID: id.EventID(baselineRef.EventID), RoomID: id.RoomID(baselineRef.RoomID), Sender: id.UserID(value.Creator.UserID), Type: event.EventEncrypted, Content: event.Content{VeryRaw: baseline.EncryptedEvent}}
	baselineACK, err := resource.ConfirmBootstrapAcceptance(ctx, baselineVerified, baselineRef, gate)
	if err != nil {
		t.Fatal(err)
	}
	if err = relay.CommitBootstrap(ctx, key, value.ID, baselineACK); err != nil {
		t.Fatal("baseline ACK", err)
	}
	if err = relay.CommitBootstrap(ctx, key, value.ID, baselineACK); err != nil {
		t.Fatal("baseline lost ACK retry", err)
	}
	if err = originPermit.Revalidate(ctx); err == nil {
		t.Fatal("accepted bootstrap became new origin authority")
	}
	checkpoint := bootstrap.OpaqueCandidate(t, resource.BootstrapCheckpoint, &baselineACK)
	checkpointVerified, err := resource.VerifyBootstrapCandidate(ctx, checkpoint, plan, &baselineACK)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = relay.StageBootstrap(ctx, plan, checkpoint, checkpointVerified); err != nil {
		t.Fatal("checkpoint stage", err)
	}
	if _, fresh, err = relay.AcquireBootstrap(ctx, plan, resource.BootstrapCheckpoint); err != nil || !fresh {
		t.Fatal("checkpoint acquire", err)
	}
	cpRef := storage.Reference{RoomID: value.RoomID, EventID: "$checkpoint-sdk-accepted"}
	gate.accepted = &event.Event{ID: id.EventID(cpRef.EventID), RoomID: id.RoomID(cpRef.RoomID), Sender: id.UserID(value.Creator.UserID), Type: event.EventEncrypted, Content: event.Content{VeryRaw: checkpoint.EncryptedEvent}}
	cpACK, err := resource.ConfirmBootstrapAcceptance(ctx, checkpointVerified, cpRef, gate)
	if err != nil {
		t.Fatal(err)
	}
	signed := bootstrap.FinalEpoch(t, baselineACK, cpACK)
	historicalActivation, historicalErr := resource.VerifyBootstrapActivation(ctx, signed, historicalPlan, baselineACK, cpACK)
	if historicalErr != nil {
		t.Fatal("historical activation proof unavailable", historicalErr)
	}
	activation, err := resource.VerifyBootstrapActivation(ctx, signed, plan, baselineACK, cpACK)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = relay.FinalizeBootstrap(ctx, activation); !errors.Is(err, resource.ErrPending) {
		t.Fatal("active routing before exact SQL checkpoint ACK", err)
	}
	countActive()
	if initialized != 0 {
		t.Fatal("pending checkpoint fabricated active epoch")
	}
	if err = relay.CommitBootstrap(ctx, key, value.ID, cpACK); err != nil {
		t.Fatal(err)
	}
	if _, err := relay.FinalizeBootstrap(ctx, historicalActivation); err == nil {
		t.Fatal("owner-only historical plan reached live backend Complete")
	}
	// An older epoch-only caller must not bypass the pending phase transaction.
	gate.f.Roster = bootstrap.Roster
	gate.f.Anchor = activation.Anchor()
	gate.f.SignedEpoch = signed
	gate.f.Epoch = activation.Epoch()
	member := bootstrap.Roster.Members()[0]
	cap := resource.Capability{Version: 1, Domain: "org.sytue.resource.capability.v1", Resource: key, RoomID: value.RoomID, Epoch: value.Epoch, Generation: "1", MembershipSHA256: bootstrap.Roster.SHA256(), PolicySHA256: activation.Epoch().SHA256(), Subject: member.Scope.Subject, UserID: member.UserID, ProductKeyID: member.ProductKeyID, Role: member.Role, Actions: member.Actions, From: member.From, Issuer: value.Creator}
	gate.f.SignedCap = resource.SignedCapability{Capability: cap, Signatures: bootstrap.Sign(cap)}
	if err = relay.Bootstrap(ctx, signed); !e2eFenceBlocked(err) {
		t.Fatal("epoch-only path bypassed pre-reference phase", err)
	}
	active, err := relay.FinalizeBootstrap(ctx, activation)
	if err != nil || active.State != "active" {
		t.Fatal("atomic activation", err)
	}
	again, err := relay.FinalizeBootstrap(ctx, activation)
	if err != nil || again != active {
		t.Fatal("activation retry", err)
	}
	countActive()
	if initialized != 1 {
		t.Fatal("multiple active resources", initialized)
	}
	snapshot, err := source.BootstrapSource(ctx, key)
	if err != nil || snapshot.SourceEpoch != frozen.Epoch || snapshot.SourceSeq != "9007199254740993" {
		t.Fatal("source was retired before client content verification", err)
	}
	var foreign int
	if err = h.Pool.InTenantTx(tenantCtx("other"), func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM resource_e2e_bootstraps`).Scan(&foreign)
	}); err != nil || foreign != 0 {
		t.Fatal("cross tenant phase visible", err)
	}
	down, err := fs.ReadFile(db.Migrations, "migrations/0037_resource_bootstrap.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = h.PoolSinRLS(t).InTx(ctx, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, string(down)); return err }); !e2eFenceBlocked(err) {
		t.Fatal("rollback discarded retained bootstrap", err)
	}
	if _, err = relay.BootstrapOriginSource(ctx, plan); err == nil {
		t.Fatal("finalized plan became origin authority")
	}
	exerciseAcceptedPolicyRecovery(t, ctx, h.Pool, capture, bootstrap, activation, gate.f.SignedCap, currentRegistry)
	currentRegistry.Retire()
	if err = originPermit.Revalidate(ctx); err == nil {
		t.Fatal("retired original producer revived from hold")
	}
	if json.Valid(baseline.EncryptedEvent) == false {
		t.Fatal("fixture SDK event invalid")
	}
}

//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

type officeResourceGate struct {
	f               resourcetest.Fixture
	empty           bool
	accepted        *event.Event
	gets            int
	producerBlocked bool
}

func (g *officeResourceGate) Access(ctx context.Context, key resource.Resource, action resource.Action, raw json.RawMessage) (resource.VerifiedAccess, error) {
	if g.empty {
		return resource.VerifiedAccess{}, nil
	}
	var signed resource.SignedEpoch
	if err := json.Unmarshal(raw, &signed); err != nil {
		return resource.VerifiedAccess{}, err
	}
	epoch, err := resource.VerifyInitialEpoch(ctx, signed, g.f.Roster, g.f.Device, g.f.Anchor)
	if err != nil {
		return resource.VerifiedAccess{}, err
	}
	cap, err := resource.VerifyCapability(ctx, g.f.SignedCap, epoch, g.f.Device)
	if err != nil {
		return resource.VerifiedAccess{}, err
	}
	return resource.Authorize(epoch, cap, g.f.Scope.Subject, action)
}

func (g *officeResourceGate) Producer(ctx context.Context, c resource.Candidate, a resource.VerifiedAccess) (resource.VerifiedPendingPublication, error) {
	if g.producerBlocked {
		return resource.VerifiedPendingPublication{}, resource.ErrRevoked
	}
	return resource.VerifyProducerCandidate(ctx, c, a, g.f.Device)
}

func (g *officeResourceGate) Complete(ctx context.Context, c resource.Candidate, p resource.VerifiedPendingPublication) (resource.VerifiedPublication, error) {
	access, err := resource.Authorize(g.f.Epoch, g.f.Cap, g.f.Scope.Subject, resource.Validate)
	if err != nil {
		return resource.VerifiedPublication{}, err
	}
	return resource.CompleteCandidate(ctx, c, p, &resource.ValidationEvidence{Access: access, Device: g.f.Device})
}

func (g *officeResourceGate) AcceptedEvent(context.Context, storage.Reference) (*event.Event, error) {
	g.gets++
	return g.accepted, nil
}

func (g *officeResourceGate) VerifyAcceptance(ctx context.Context, c resource.Candidate, p resource.VerifiedPublication, ref storage.Reference) (resource.VerifiedAcceptance, error) {
	return resource.ConfirmAcceptance(ctx, p, ref, g)
}

func (g *officeResourceGate) VerifyReservedAcceptance(ctx context.Context, p resource.VerifiedReservation, ref storage.Reference) (resource.VerifiedAcceptance, error) {
	return resource.ConfirmReservedAcceptance(ctx, p, ref, g)
}

func (g *officeResourceGate) VerifyNotAccepted(context.Context, resource.JournalEntry, json.RawMessage) error {
	return resource.ErrInvalid
}

func (g *officeResourceGate) InitialEpoch(ctx context.Context, e resource.SignedEpoch) (resource.VerifiedEpoch, error) {
	return resource.VerifyInitialEpoch(ctx, e, g.f.Roster, g.f.Device, g.f.Anchor)
}

func (g *officeResourceGate) NextEpoch(ctx context.Context, e resource.SignedEpoch, a resource.VerifiedAccess, p resource.Position) (resource.VerifiedEpoch, error) {
	return resource.VerifyNextEpoch(ctx, e, g.f.Roster, g.f.Device, a.Epoch(), a.Capability(), p)
}

func officeValidate(t testing.TB, f resourcetest.Fixture, c resource.Candidate) resource.Candidate {
	t.Helper()
	hash, err := resource.PublicationDigest(c.Proof.CipherPublication)
	if err != nil {
		t.Fatal(err)
	}
	certificate := resource.ValidationCertificate{Version: 1, Domain: "org.sytue.resource.validation.v1", PublicationSHA256: hash, ValidatorCapabilitySHA256: f.Cap.SHA256(), RulesVersion: f.Epoch.Value().Validation.RulesVersion, Allow: true, Issuer: f.Actor}
	certificate.Signatures = matrixprooftest.Sign(t, f.Chain.Device, certificate, f.Chain.Origin.UserID, f.Actor.DeviceID)
	c.Validation = &certificate
	return c
}

// This verifies the opaque database relay with real SDK signatures and public
// verifier types. Cipher content is synthetic and never decrypted here; actual
// Megolm/PQ streams have their separate SDK/homeserver interoperability proofs.
func TestOfficeResourceRelayTypedAuthorityPendingAndLostACK(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	f := resourcetest.New(t, true)
	ctx := tenantCtx(f.Scope.Tenant)
	gate := &officeResourceGate{f: f}
	relay, err := store.NewResourceRelay(st, gate, f.Scope.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if err = relay.Bootstrap(ctx, f.SignedEpoch); err != nil {
		t.Fatal("bootstrap", err)
	}
	pending, err := relay.Stage(ctx, f.Candidate)
	if err != nil || pending.State != resource.WaitingValidation {
		t.Fatal("unvalidated operation not durably pending", err, pending.State)
	}
	if pending.AttemptID != f.Candidate.Proof.TransactionID || pending.Receipt != nil {
		t.Fatal("stage fabricated a receipt or omitted durable identity")
	}
	if _, err = relay.AcquireSending(ctx, f.Candidate.Proof.Resource, f.Candidate.Proof.OpID); !errors.Is(err, resource.ErrValidatorUnavailable) {
		t.Fatal("unvalidated operation became sendable", err)
	}
	if gate.gets != 0 {
		t.Fatal("pending validation contacted acceptance source")
	}
	candidate := officeValidate(t, f, f.Candidate)
	prepared, err := relay.Stage(ctx, candidate)
	if err != nil || prepared.State != resource.Prepared || prepared.AttemptID != pending.AttemptID {
		t.Fatal("validation replaced immutable attempt", err, prepared.State)
	}
	sending, err := relay.AcquireSending(ctx, candidate.Proof.Resource, candidate.Proof.OpID)
	if err != nil || sending.State != resource.Sending {
		t.Fatal("acquire", err, sending.State)
	}
	var mismatchTxn *pgconn.PgError
	mismatchErr := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO resource_e2e_candidates SELECT (jsonb_populate_record(null::resource_e2e_candidates,to_jsonb(c)||jsonb_build_object('op_id','wrong-signed-transaction','attempt_id',gen_random_uuid()))).* FROM resource_e2e_candidates c WHERE op_id=$1`, candidate.Proof.OpID)
		return e
	})
	if !errors.As(mismatchErr, &mismatchTxn) || mismatchTxn.Code != "23514" {
		t.Fatal("database accepted attempt different from signed transaction", mismatchErr)
	}
	var duplicateTxn *pgconn.PgError
	replayErr := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO resource_e2e_candidates SELECT (jsonb_populate_record(null::resource_e2e_candidates,to_jsonb(c)||jsonb_build_object('op_id','different-op'))).* FROM resource_e2e_candidates c WHERE op_id=$1`, candidate.Proof.OpID)
		return e
	})
	if !errors.As(replayErr, &duplicateTxn) || duplicateTxn.Code != "23505" || duplicateTxn.ConstraintName != "resource_e2e_candidates_attempt_id_key" {
		t.Fatal("same SDK transaction reused for a different operation", replayErr)
	}
	var evidenceBefore, evidenceAfter int
	var reservationSHA string
	if err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if e := tx.QueryRow(ctx, `SELECT count(*) FROM resource_e2e_public_evidence`).Scan(&evidenceBefore); e != nil {
			return e
		}
		return tx.QueryRow(ctx, `SELECT reservation_sha256 FROM resource_e2e_candidates WHERE op_id=$1`, candidate.Proof.OpID).Scan(&reservationSHA)
	}); err != nil || evidenceBefore < 3 || len(reservationSHA) != 64 {
		t.Fatal("public SDK evidence/reservation not persisted before send", err, evidenceBefore)
	}
	if _, err = relay.AcquireSending(ctx, candidate.Proof.Resource, candidate.Proof.OpID); err != nil {
		t.Fatal("repeated acquire", err)
	}
	if err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM resource_e2e_public_evidence`).Scan(&evidenceAfter)
	}); err != nil || evidenceAfter != evidenceBefore {
		t.Fatal("publication recopied content addressed evidence", err, evidenceBefore, evidenceAfter)
	}
	for _, q := range []string{
		`UPDATE resource_e2e_candidates SET reservation=convert_to('{}','UTF8'),reservation_sha256=encode(digest(convert_to('{}','UTF8'),'sha256'),'hex')`,
		`UPDATE resource_e2e_public_evidence SET value=convert_to('{}','UTF8'),sha256=encode(digest(convert_to('{}','UTF8'),'sha256'),'hex')`,
	} {
		if e := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error { _, e := tx.Exec(ctx, q); return e }); !e2eFenceBlocked(e) {
			t.Fatal("captured SDK evidence was replaceable", e)
		}
	}
	if err = relay.Cancel(ctx, candidate.Proof.Resource, candidate.Proof.OpID); !errors.Is(err, resource.ErrPending) {
		t.Fatal("cancel discarded potentially accepted send", err)
	}
	if err = relay.Drain(ctx, candidate.Proof.Resource); err != nil {
		t.Fatal("drain", err)
	}
	if err = relay.MarkAmbiguous(ctx, sending); err != nil {
		t.Fatal("ambiguous", err)
	}
	var content map[string]json.RawMessage
	if err = json.Unmarshal(candidate.EncryptedEvent, &content); err != nil {
		t.Fatal(err)
	}
	ref := storage.Reference{RoomID: candidate.Proof.RoomID, EventID: "$accepted"}
	gate.accepted = &event.Event{ID: id.EventID(ref.EventID), RoomID: id.RoomID(ref.RoomID), Sender: id.UserID(candidate.Proof.Actor.UserID), Type: event.EventEncrypted, Content: event.Content{VeryRaw: candidate.EncryptedEvent}}
	wrong := ref
	wrong.EventID = "$wrong"
	if _, err = relay.Commit(ctx, sending, wrong); err == nil {
		t.Fatal("unconfirmed reference committed")
	}
	if _, e := relay.ReconcileAccepted(ctx, candidate.Proof.Resource, candidate.Proof.OpID, wrong); e == nil {
		t.Fatal("reserved ACK accepted wrong reference")
	}
	gate.producerBlocked = true
	if _, e := relay.Commit(ctx, sending, ref); !errors.Is(e, resource.ErrRevoked) {
		t.Fatal("normal commit revived retired producer callback", e)
	}
	receipt, err := relay.ReconcileAccepted(ctx, candidate.Proof.Resource, candidate.Proof.OpID, ref)
	if err != nil {
		t.Fatal("commit", err)
	}
	if receipt.Head.Seq != "9007199254740995" || receipt.Base != f.Candidate.Proof.ExpectedBase || receipt.Head.HeadCipherSHA256 != candidate.Proof.OuterCipherSHA256 {
		t.Fatal("receipt lost precise original sequence or cipher head")
	}
	gets := gate.gets
	again, err := relay.ReconcileAccepted(ctx, candidate.Proof.Resource, candidate.Proof.OpID, ref)
	if err != nil || again != receipt || gate.gets != gets {
		t.Fatal("lost ACK did not return original durable receipt", err)
	}
	page, err := relay.Page(ctx, candidate.Proof.Resource, resource.Cursor{Position: receipt.Base}, 1)
	if err != nil || len(page.Records) != 1 || page.Records[0] != receipt || page.Next.Position != receipt.Head {
		t.Fatal("committed chain page", err)
	}
	badCursor := resource.Cursor{Position: receipt.Base}
	badCursor.Position.HeadCipherSHA256 = candidate.Proof.OuterCipherSHA256
	if _, err = relay.Page(ctx, candidate.Proof.Resource, badCursor, 1); !errors.Is(err, resource.ErrFork) {
		t.Fatal("forked cursor accepted", err)
	}
	other, err := store.NewResourceRelay(st, gate, "other-subject")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = other.Pending(ctx, candidate.Proof.Resource, candidate.Proof.OpID); err == nil {
		t.Fatal("public stored policy granted another subject")
	}
	gate.empty = true
	if _, err = relay.Pending(ctx, candidate.Proof.Resource, candidate.Proof.OpID); err == nil {
		t.Fatal("zero VerifiedAccess treated as permission")
	}
}

// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

type officeOriginGate struct {
	officeResourceGate
	current resource.VerifiedSession
}

func (g *officeOriginGate) Access(ctx context.Context, key resource.Resource, action resource.Action, raw json.RawMessage) (resource.VerifiedAccess, error) {
	a, err := g.officeResourceGate.Access(ctx, key, action, raw)
	if err != nil {
		return a, err
	}
	return resource.AuthorizeSession(ctx, a.Epoch(), a.Capability(), g.current, action)
}

func TestOfficeResourceOriginSourceReadsOriginalHoldAndRequiresCurrentProvider(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	f := resourcetest.New(t, true)
	current, registry := resourcetest.EnrolledSession(t, f.Chain, f.RecipientIdentity)
	ctx := tenantCtx(f.Scope.Tenant)
	gate := &officeOriginGate{officeResourceGate: officeResourceGate{f: f}, current: current}
	relay, err := store.NewResourceRelay(st, gate, f.Scope.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if err = relay.Bootstrap(ctx, f.SignedEpoch); err != nil {
		t.Fatal(err)
	}
	candidate := officeValidate(t, f, f.Candidate)
	if _, err = relay.Stage(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	key, op := candidate.Proof.Resource, candidate.Proof.OpID
	if _, err = relay.OriginHoldSource(ctx, key, op); !errors.Is(err, resource.ErrPending) {
		t.Fatal("prepared row became durable Sending hold", err)
	}
	if _, err = relay.AcquireSending(ctx, key, op); err != nil {
		t.Fatal(err)
	}
	source, err := relay.OriginHoldSource(ctx, key, op)
	if err != nil {
		t.Fatal(err)
	}
	held, err := source.HeldReservation(ctx, key, op)
	if err != nil || held.Resource != key || held.OpID != op || held.State != resource.Sending || held.AttemptID != candidate.Proof.TransactionID || held.Base != candidate.Proof.ExpectedBase || held.Candidate.Proof.OuterCipherSHA256 != candidate.Proof.OuterCipherSHA256 {
		t.Fatal("source replaced original SQL bindings", err)
	}
	if _, err = source.HeldReservation(ctx, resource.Resource{Tenant: "other", Product: key.Product, Kind: key.Kind, ID: key.ID}, op); err == nil {
		t.Fatal("cross tenant hold read")
	}
	request := publication.Request{Version: 1, Domain: "org.sytue.publication.admission.v1", Namespace: current.Binding().Namespace, Scope: current.Scope(), Target: storage.PublicationTarget{RoomID: candidate.Proof.RoomID, DeviceID: current.Actor().DeviceID}, TransactionID: candidate.Proof.TransactionID, CipherSHA256: candidate.Proof.OuterCipherSHA256, AuthorityGeneration: 1}
	signed := publication.SignedRequest{Request: request, Signatures: matrixprooftest.Sign(t, f.Chain.Device, request, f.Chain.Origin.UserID, f.Actor.DeviceID)}
	verified, err := source.VerifyRequest(ctx, current, signed)
	if err != nil || verified.Request().Value() != request || verified.Reservation().SHA256() != held.DescriptorSHA256 {
		t.Fatal("typed original hold failed", err)
	}
	legacy, err := store.NewResourceRelay(st, &gate.officeResourceGate, f.Scope.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = legacy.OriginHoldSource(ctx, key, op); err == nil {
		t.Fatal("historical capability without current Provider became origin authority")
	}
	if err = relay.MarkAmbiguous(ctx, resource.JournalEntry{Candidate: candidate, State: resource.Sending, AttemptID: held.AttemptID}); err != nil {
		t.Fatal(err)
	}
	if err = source.Revalidate(ctx); err != nil {
		t.Fatal("ambiguous original hold disappeared", err)
	}
	registry.Retire()
	if err = source.Revalidate(ctx); err == nil {
		t.Fatal("retained source survived current producer revocation")
	}
	if _, err = source.VerifyRequest(ctx, current, signed); err == nil {
		t.Fatal("retained reservation revived producer")
	}
	// Historical ACK restoration is separate and does not create a live source.
	if _, err = relay.OriginHoldSource(ctx, key, op); err == nil {
		t.Fatal("revoked producer got a new origin source")
	}
}

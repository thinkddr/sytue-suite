//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"sync/atomic"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

type officeCipherTransport func(context.Context, resource.Candidate, resource.VerifiedPublication, publication.Marker) (storage.Reference, error)

func (f officeCipherTransport) Send(ctx context.Context, c resource.Candidate, p resource.VerifiedPublication, m publication.Marker) (storage.Reference, error) {
	return f(ctx, c, p, m)
}

func TestOfficeResourceDispatchClaimsExactlyOnceAndNeverResendsAmbiguous(t *testing.T) {
	for _, mode := range []string{"concurrent", "lost-ack", "admission-denied", "cancelled", "commit-failed", "validation-pending"} {
		t.Run(mode, func(t *testing.T) {
			h := harnessComoEnProduccion(t)
			fixture := resourcetest.New(t, true)
			ctx := tenantCtx(fixture.Scope.Tenant)
			gate := &officeResourceGate{f: fixture}
			relay, err := store.NewResourceRelay(store.New(h.Pool), gate, fixture.Scope.Subject)
			if err != nil {
				t.Fatal(err)
			}
			if err := relay.Bootstrap(ctx, fixture.SignedEpoch); err != nil {
				t.Fatal(err)
			}
			candidate := officeValidate(t, fixture, fixture.Candidate)
			if mode == "validation-pending" {
				candidate = fixture.Candidate
			}
			ref := storage.Reference{RoomID: candidate.Proof.RoomID, EventID: "$dispatch-accepted"}
			gate.accepted = &event.Event{ID: id.EventID(ref.EventID), RoomID: id.RoomID(ref.RoomID), Sender: id.UserID(candidate.Proof.Actor.UserID), Type: event.EventEncrypted, Content: event.Content{VeryRaw: candidate.EncryptedEvent}}
			marker := publication.Marker{Version: 1, ID: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{19}, 32))}
			var admits, sends atomic.Int64
			admitted := make(chan struct{})
			release := make(chan struct{})
			failure := errors.New("synthetic dispatch transport uncertainty")
			runctx, cancel := context.WithCancel(ctx)
			defer cancel()
			admit := func(ctx context.Context, entry resource.JournalEntry, verified resource.VerifiedPublication) (publication.Marker, error) {
				claimCount := admits.Add(1)
				if entry.State != resource.Sending || entry.AttemptID != candidate.Proof.TransactionID || verified.Value() != candidate.Proof.CipherPublication {
					t.Error("admission did not receive exact durable sending claim")
				}
				if mode == "concurrent" && claimCount == 1 {
					close(admitted)
					<-release
				}
				if mode == "admission-denied" {
					return publication.Marker{}, failure
				}
				return marker, nil
			}
			transport := officeCipherTransport(func(ctx context.Context, c resource.Candidate, p resource.VerifiedPublication, m publication.Marker) (storage.Reference, error) {
				sends.Add(1)
				if m != marker || p.Value() != candidate.Proof.CipherPublication {
					t.Error("transport was rebound after durable admission")
				}
				if mode == "lost-ack" {
					return storage.Reference{}, failure
				}
				if mode == "cancelled" {
					cancel()
					return storage.Reference{}, context.Canceled
				}
				return ref, nil
			})
			if mode == "commit-failed" {
				gate.accepted.ID = "$different-event"
			}
			var receipt resource.Receipt
			if mode == "concurrent" {
				type result struct {
					r resource.Receipt
					e error
				}
				resultCh := make(chan result, 1)
				go func() { r, e := relay.Dispatch(runctx, candidate, admit, transport); resultCh <- result{r, e} }()
				select {
				case <-admitted:
				case early := <-resultCh:
					t.Fatal("first dispatcher failed before claim", early.e)
				case <-t.Context().Done():
					t.Fatal("first dispatcher did not claim")
				}
				_, secondErr := relay.Dispatch(ctx, candidate, admit, transport)
				close(release)
				got := <-resultCh
				receipt, err = got.r, got.e
				if !errors.Is(secondErr, resource.ErrPending) {
					t.Fatal("second dispatcher acquired existing sending hold", secondErr)
				}
			} else {
				receipt, err = relay.Dispatch(runctx, candidate, admit, transport)
			}
			if mode == "validation-pending" {
				if !errors.Is(err, resource.ErrValidatorUnavailable) || admits.Load() != 0 || sends.Load() != 0 {
					t.Fatal("private rules bypassed before sending", err)
				}
				entry, e := relay.Pending(ctx, candidate.Proof.Resource, candidate.Proof.OpID)
				if e != nil || entry.State != resource.WaitingValidation {
					t.Fatal("unvalidated candidate not durable", e)
				}
				return
			}
			if mode == "concurrent" {
				if err != nil || receipt.Reference != ref || admits.Load() != 1 || sends.Load() != 1 {
					t.Fatal("fresh claim sent more than once or lost commit", err)
				}
				again, e := relay.Dispatch(ctx, candidate, admit, transport)
				if e != nil || again != receipt || sends.Load() != 1 {
					t.Fatal("committed retry sent again", e)
				}
				return
			}
			var pending *resourcepg.DispatchError
			if !errors.As(err, &pending) {
				t.Fatal("uncertain operation falsely committed", err)
			}
			if mode == "commit-failed" && pending.AcceptedReference() != ref {
				t.Fatal("failed commit lost exact accepted reference")
			}
			entry, e := relay.Pending(ctx, candidate.Proof.Resource, candidate.Proof.OpID)
			if e != nil || entry.State != resource.Ambiguous || entry.Receipt != nil {
				t.Fatal("failed dispatch did not persist ambiguous hold", e, entry.State)
			}
			before := sends.Load()
			if _, e := relay.Dispatch(ctx, candidate, admit, transport); !errors.Is(e, resource.ErrPending) || sends.Load() != before {
				t.Fatal("ambiguous retry blindly sent again", e)
			}
			if mode == "admission-denied" && before != 0 {
				t.Fatal("origin denial reached transport")
			}
			if e := relay.Cancel(ctx, candidate.Proof.Resource, candidate.Proof.OpID); e == nil {
				t.Fatal("cancel discarded ambiguous hold")
			}
			if mode == "lost-ack" || mode == "commit-failed" {
				gate.accepted.ID = id.EventID(ref.EventID)
				gate.producerBlocked = true
				restored, e := relay.ReconcileAccepted(ctx, candidate.Proof.Resource, candidate.Proof.OpID, ref)
				if e != nil || restored.Reference != ref || sends.Load() != before {
					t.Fatal("ACK-only recovery revived or resent producer", e)
				}
			}
		})
	}
}

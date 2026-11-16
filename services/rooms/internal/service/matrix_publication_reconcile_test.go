// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

type reconciliationFixture struct {
	pending     []store.MatrixPublicationPending
	next        *store.MatrixPublicationCursor
	acks        []publication.OriginEvent
	sourceFault string
	storeErr    error
}

func (f *reconciliationFixture) PendingMatrixPublicationOrigins(_ context.Context, _ *store.MatrixPublicationCursor, limit int) ([]store.MatrixPublicationPending, *store.MatrixPublicationCursor, error) {
	if limit != 16 {
		return nil, nil, errors.New("unbounded reconciliation")
	}
	return f.pending, f.next, f.storeErr
}

func (f *reconciliationFixture) AcceptMatrixPublicationOrigin(_ context.Context, scope storage.Scope, user string, e publication.OriginEvent) error {
	if scope != f.pending[0].Cursor.Scope || user != e.Sender {
		return errors.New("changed acknowledgement scope")
	}
	f.acks = append(f.acks, e)
	return nil
}

func (f *reconciliationFixture) RetainedPublicationOrigin(ctx context.Context, scope storage.Scope, ref storage.Reference, retained string) (publication.OriginEvent, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil || tenant.Slug != scope.Tenant || retained != "@owner:matrix.test" {
		return publication.OriginEvent{}, errors.New("missing trusted routing")
	}
	value := publication.OriginEvent{Version: 1, Reference: ref, Sender: "@owner:matrix.test", Type: "m.room.encrypted", Content: json.RawMessage(`{"algorithm":"m.megolm.v1.aes-sha2","session_id":"synthetic","ciphertext":"opaque"}`)}
	switch f.sourceFault {
	case "missing", "unavailable", "cancelled":
		return publication.OriginEvent{}, errors.New("origin unavailable")
	case "event":
		value.Reference.EventID = "$other"
	case "room":
		value.Reference.RoomID = "!other:matrix.test"
	case "plaintext":
		value.Type = "m.room.message"
		value.Content = nil
	case "sender":
		value.Sender = "@other:matrix.test"
	case "invalid":
		value.Version = 2
	}
	return value, nil
}

func TestPublicationReconcileAcknowledgesOnlyPositiveExactReadback(t *testing.T) {
	for _, fault := range []string{"valid", "missing", "unavailable", "cancelled", "event", "room", "plaintext", "invalid", "sender", "database"} {
		t.Run(fault, func(t *testing.T) {
			row := store.MatrixPublicationPending{Cursor: store.MatrixPublicationCursor{Scope: storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}, ID: uuid.New()}, Reference: storage.Reference{RoomID: "!room:matrix.test", EventID: "$event"}, MatrixUserID: "@owner:matrix.test"}
			f := &reconciliationFixture{pending: []store.MatrixPublicationPending{row}, next: &row.Cursor, sourceFault: fault}
			if fault == "database" {
				f.storeErr = errors.New("database unavailable")
			}
			next, count, err := reconcileMatrixPublications(t.Context(), f, f, nil)
			if fault == "valid" {
				if err != nil || count != 1 || len(f.acks) != 1 || f.acks[0].Reference != row.Reference || next != f.next {
					t.Fatal("positive exact readback lost", count, err)
				}
			} else {
				if count != 0 || len(f.acks) != 0 {
					t.Fatal("absence/error fabricated acceptance", fault, count)
				}
				if fault == "database" && err == nil {
					t.Fatal("database failure hidden")
				}
			}
		})
	}
}

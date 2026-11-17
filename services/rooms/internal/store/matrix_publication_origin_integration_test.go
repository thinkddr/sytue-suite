//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Ciphertext is synthetic here: real SDK encryption/backup and the homeserver
// callback boundary are tested separately. Signatures use actual SDK keys.
func reservedOriginFixture(t *testing.T, f *authorityFixture, txn string) (publication.OriginEvent, publication.Receipt) {
	t.Helper()
	raw := json.RawMessage(`{"algorithm":"m.megolm.v1.aes-sha2","session_id":"synthetic-session","ciphertext":"opaque-sdk-ciphertext"}`)
	digest, _, err := publication.WireDigest(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	signed := signedPublicationFixture(t, f, txn)
	signed.CipherSHA256 = digest
	signed.Signatures = matrixprooftest.Sign(t, f.gate.keys.Device, signed, f.gate.keys.Origin.UserID, f.gate.keys.Origin.DeviceID.String())
	receipt, err := f.st.ReserveMatrixStoragePublication(tenantCtx(f.scope.Tenant), f.scope.Subject, f.scope.Product, signed)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := publication.AttachMarker(raw, receipt.Marker)
	if err != nil {
		t.Fatal(err)
	}
	return publication.OriginEvent{Version: 1, Reference: storage.Reference{RoomID: f.target.RoomID, EventID: "$" + txn}, Sender: f.gate.keys.Origin.UserID.String(), Type: "m.room.encrypted", Content: wire}, receipt
}

func TestPublicationOriginClaimsOnlyExactReservedWire(t *testing.T) {
	f := enrolledPublicationFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	event, receipt := reservedOriginFixture(t, f, "origin-immutable")
	scope, found, err := f.st.MatrixPublicationRoomScope(ctx, event.Reference.RoomID)
	if err != nil || !found || scope != f.scope {
		t.Fatal("independent room routing", err)
	}
	if _, found, err = f.st.MatrixPublicationRoomScope(ctx, "!unknown:matrix.test"); err != nil || found {
		t.Fatal("unknown room manufactured scope", err)
	}
	for _, fault := range []string{"cipher", "marker", "room", "sender", "scope", "inactive"} {
		t.Run(fault, func(t *testing.T) {
			changed := event
			actual := f.scope
			switch fault {
			case "cipher":
				var content map[string]any
				if json.Unmarshal(changed.Content, &content) != nil {
					t.Fatal("fixture")
				}
				content["ciphertext"] = "changed"
				changed.Content, _ = json.Marshal(content)
			case "marker":
				changed.Content = json.RawMessage(`{"algorithm":"m.megolm.v1.aes-sha2","session_id":"synthetic-session","ciphertext":"opaque-sdk-ciphertext"}`)
			case "room":
				changed.Reference.RoomID = "!other:matrix.test"
			case "sender":
				changed.Sender = "@other:matrix.test"
			case "scope":
				actual.Subject = "other"
			case "inactive":
				f.gate.keys.Origin.DeviceActive = false
			}
			err := f.st.AdmitMatrixPublicationOrigin(ctx, actual, event.Sender, changed, true)
			f.gate.keys.Origin.DeviceActive = true
			if err == nil {
				t.Fatal("changed origin was admitted")
			}
		})
	}
	for range 2 {
		if err := f.st.AdmitMatrixPublicationOrigin(ctx, f.scope, event.Sender, event, true); err != nil {
			t.Fatal("admission/retry", err)
		}
	}
	pending, err := f.st.ReadMatrixStoragePublication(ctx, f.scope.Subject, f.scope.Product, event.Reference.RoomID, "origin-immutable")
	if err != nil || pending.Status != "sending" || pending.Marker != receipt.Marker || pending.Reference != nil {
		t.Fatal("origin did not retain sending hold", err)
	}
	changed := event
	changed.Reference.EventID = "$second-event"
	if err := f.st.AdmitMatrixPublicationOrigin(ctx, f.scope, event.Sender, changed, true); err == nil {
		t.Fatal("marker claimed by another event")
	}
	changed.Content = json.RawMessage(`{"algorithm":"m.megolm.v1.aes-sha2","session_id":"synthetic-session","ciphertext":"changed"}`)
	if err := f.st.AcceptMatrixPublicationOrigin(ctx, f.scope, event.Sender, changed); err == nil {
		t.Fatal("wrong origin accepted")
	}
	for range 2 {
		if err := f.st.AcceptMatrixPublicationOrigin(ctx, f.scope, event.Sender, event); err != nil {
			t.Fatal("acknowledgement/retry", err)
		}
	}
	accepted, err := f.st.ReadMatrixStoragePublication(ctx, f.scope.Subject, f.scope.Product, event.Reference.RoomID, "origin-immutable")
	if err != nil || accepted.Status != "accepted" || accepted.Reference == nil || *accepted.Reference != event.Reference {
		t.Fatal("accepted reference changed", err)
	}
}

func TestPublicationOriginAcknowledgementSurvivesProducerWithdrawal(t *testing.T) {
	f := enrolledPublicationFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	event, _ := reservedOriginFixture(t, f, "origin-after-withdrawal")
	if err := f.st.AdmitMatrixPublicationOrigin(ctx, f.scope, event.Sender, event, true); err != nil {
		t.Fatal(err)
	}
	if err := f.st.ApplyMatrixAccountAccess(ctx, f.scope.Subject, "suspended", 3, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	f.gate.keys.Origin.DeviceActive = false
	if err := f.st.AdmitMatrixPublicationOrigin(ctx, f.scope, event.Sender, event, true); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("withdrawn identity admitted work", err)
	}
	for range 2 {
		if err := f.st.AcceptMatrixPublicationOrigin(ctx, f.scope, event.Sender, event); err != nil {
			t.Fatal("old acknowledgement lost", err)
		}
	}
	if err := publicationTx(f, true, f.scope.Tenant, f.scope.Subject, func(tx pgx.Tx) error {
		var state string
		if err := tx.QueryRow(ctx, `SELECT status FROM matrix_storage_publications WHERE event_id=$1`, event.Reference.EventID).Scan(&state); err != nil {
			return err
		}
		if state != "accepted" {
			t.Fatal("withdrawal stranded accepted event")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationOriginSendingHoldBlocksFenceBeforePrivilegedIO(t *testing.T) {
	f, proof := deviceFenceFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	event, _ := reservedOriginFixture(t, f, "origin-before-fence")
	if err := f.st.AdmitMatrixPublicationOrigin(ctx, f.scope, event.Sender, event, true); err != nil {
		t.Fatal(err)
	}
	old := f.gate.keys
	next := matrixprooftest.New(t, f.scope, "matrix.test", id.DeviceID("NEXT_DEVICE"))
	next.Master, next.Self = old.Master, old.Self
	next.Origin.Keys.MasterKeys = old.Origin.Keys.MasterKeys
	next.Origin.Keys.SelfSigningKeys = old.Origin.Keys.SelfSigningKeys
	user := next.Origin.UserID
	deviceID := next.Origin.DeviceID
	dev := next.Origin.Keys.DeviceKeys[user][deviceID]
	dev.Signatures = matrixprooftest.Sign(t, old.Self, dev, user, old.Self.PublicKey().String())
	own := matrixprooftest.Sign(t, next.Device, dev, user, deviceID.String())
	for name, value := range own[user] {
		dev.Signatures[user][name] = value
	}
	next.Origin.Keys.DeviceKeys[user][deviceID] = dev
	f.gate.keys = next
	previous := f.target
	f.target.DeviceID = deviceID.String()
	request := proof.Request
	handoff := *request.Handoff
	handoff.Previous.Target = previous
	handoff.Previous.SigningKey = old.Device.PublicKey().String()
	request.Handoff = &handoff
	request.Target = f.target
	proof = signDeviceFence(t, f, request)
	if _, err := f.st.PlanMatrixStorageDeviceFence(ctx, proof); err != nil {
		t.Fatal(err)
	}
	calls := 0
	verify := func(context.Context, storage.Scope, ingress.Assignment, bool) error { calls++; return nil }
	if _, err := f.st.ConfirmMatrixStorageDeviceFence(ctx, proof, verify); !errors.Is(err, domain.ErrConflict) || calls != 0 {
		t.Fatal("pending origin reached revoke", err, calls)
	}
	if err := f.st.AcceptMatrixPublicationOrigin(ctx, f.scope, event.Sender, event); err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.ConfirmMatrixStorageDeviceFence(ctx, proof, verify); err != nil || calls != 1 {
		t.Fatal("accepted origin failed to drain", err, calls)
	}
}

func TestPublicationRecoveryRetainsOriginalProducerAfterDeviceWithdrawal(t *testing.T) {
	f := enrolledPublicationFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	event, _ := reservedOriginFixture(t, f, "retained-public-recovery")
	selection := publication.RecoverySelection{Scope: f.scope, RoomID: event.Reference.RoomID, TransactionID: "retained-public-recovery"}
	assertState := func(status string) {
		t.Helper()
		state, err := f.st.ReadMatrixStoragePublicationRecovery(ctx, f.scope.Subject, f.scope.Product, selection.RoomID, selection.TransactionID)
		if err != nil || !state.Valid(selection) || state.Status != status {
			t.Fatal("lost retained public state", status, err)
		}
		if state.Original.Target != f.target || len(state.Original.Signatures) == 0 {
			t.Fatal("producer replaced by current device")
		}
		raw, err := json.Marshal(state)
		if err != nil || strings.Contains(string(raw), "marker") {
			t.Fatal("recovery produced send marker", err)
		}
		hint, err := publication.DecodeRecoveryHint(raw, selection)
		if err != nil {
			t.Fatal(err)
		}
		ref, accepted := hint.AcceptedReference()
		if accepted != (status == "accepted") || (accepted && ref != event.Reference) {
			t.Fatal("wrong ACK-only reference")
		}
	}
	assertState("reserved")
	if err := f.st.AdmitMatrixPublicationOrigin(ctx, f.scope, event.Sender, event, true); err != nil {
		t.Fatal(err)
	}
	assertState("sending")
	if err := f.st.AcceptMatrixPublicationOrigin(ctx, f.scope, event.Sender, event); err != nil {
		t.Fatal(err)
	}
	f.gate.keys.Origin.DeviceActive = false
	assertState("accepted")
	for _, bad := range []struct{ tenant, subject, product, room, txn string }{{"foreign", f.scope.Subject, f.scope.Product, selection.RoomID, selection.TransactionID}, {f.scope.Tenant, "other", f.scope.Product, selection.RoomID, selection.TransactionID}, {f.scope.Tenant, f.scope.Subject, "docs", selection.RoomID, selection.TransactionID}, {f.scope.Tenant, f.scope.Subject, f.scope.Product, "!other:matrix.test", selection.TransactionID}, {f.scope.Tenant, f.scope.Subject, f.scope.Product, selection.RoomID, "other"}} {
		if _, err := f.st.ReadMatrixStoragePublicationRecovery(tenantCtx(bad.tenant), bad.subject, bad.product, bad.room, bad.txn); err == nil {
			t.Fatal("crossed retained history boundary")
		}
	}
	if err := f.st.ApplyMatrixAccountAccess(ctx, f.scope.Subject, "suspended", 3, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.ReadMatrixStoragePublicationRecovery(ctx, f.scope.Subject, f.scope.Product, selection.RoomID, selection.TransactionID); err == nil {
		t.Fatal("suspended account read recovery")
	}
}

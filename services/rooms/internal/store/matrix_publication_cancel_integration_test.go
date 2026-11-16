//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

func originSignedRequest(t *testing.T, f *authorityFixture, event publication.OriginEvent, txn string) publication.SignedRequest {
	t.Helper()
	signed := signedPublicationFixture(t, f, txn)
	digest, _, err := publication.WireDigest(event.Content, true)
	if err != nil {
		t.Fatal(err)
	}
	signed.CipherSHA256 = digest
	signed.Signatures = matrixprooftest.Sign(t, f.gate.keys.Device, signed, f.gate.keys.Origin.UserID, f.gate.keys.Origin.DeviceID.String())
	return signed
}

func TestPublicationCancelCommitsNegativeTombstoneWithoutGuessingAbsence(t *testing.T) {
	f := enrolledPublicationFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	signed := signedPublicationFixture(t, f, "cancel-unseen-reserve")
	first, err := f.st.CancelMatrixStoragePublication(ctx, f.scope.Subject, f.scope.Product, signed)
	if err != nil || !first.Valid() || first.Status != "cancelled" || first.Reference != nil {
		t.Fatal("negative tombstone was not committed", err)
	}
	for range 2 {
		retry, err := f.st.ReserveMatrixStoragePublication(ctx, f.scope.Subject, f.scope.Product, signed)
		if err != nil || retry.Marker != first.Marker || retry.Status != "cancelled" || retry.RequestSHA256 != first.RequestSHA256 {
			t.Fatal("cancelled transaction revived", err)
		}
	}
	changed := signed
	changed.CipherSHA256 = strings.Repeat("c", 64)
	changed.Signatures = matrixprooftest.Sign(t, f.gate.keys.Device, changed, f.gate.keys.Origin.UserID, f.gate.keys.Origin.DeviceID.String())
	if _, err := f.st.CancelMatrixStoragePublication(ctx, f.scope.Subject, f.scope.Product, changed); err == nil {
		t.Fatal("negative evidence changed its ciphertext")
	}
	event, reserved := reservedOriginFixture(t, f, "cancel-lost-reply")
	signed = originSignedRequest(t, f, event, "cancel-lost-reply")
	cancelled, err := f.st.CancelMatrixStoragePublication(ctx, f.scope.Subject, f.scope.Product, signed)
	if err != nil || cancelled.Status != "cancelled" || cancelled.Marker != reserved.Marker {
		t.Fatal("lost reserve reply could not be cancelled", err)
	}
	if err := f.st.AdmitMatrixPublicationOrigin(ctx, f.scope, event.Sender, event, true); err == nil {
		t.Fatal("late cached marker published after cancellation")
	}
	read, err := f.st.ReadMatrixStoragePublication(ctx, f.scope.Subject, f.scope.Product, f.target.RoomID, signed.TransactionID)
	if err != nil || read.Status != "cancelled" || read.RequestSHA256 != cancelled.RequestSHA256 {
		t.Fatal("durable negative evidence lost", err)
	}
}

func TestPublicationCancelNeverConvertsSendingOrAcceptedIntoAbsence(t *testing.T) {
	f := enrolledPublicationFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	event, _ := reservedOriginFixture(t, f, "cancel-inflight")
	signed := originSignedRequest(t, f, event, "cancel-inflight")
	if err := f.st.AdmitMatrixPublicationOrigin(ctx, f.scope, event.Sender, event, true); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"sending", "accepted"} {
		if state == "accepted" {
			if err := f.st.AcceptMatrixPublicationOrigin(ctx, f.scope, event.Sender, event); err != nil {
				t.Fatal(err)
			}
		}
		receipt, err := f.st.CancelMatrixStoragePublication(ctx, f.scope.Subject, f.scope.Product, signed)
		if err != nil || receipt.Status != state {
			t.Fatal("origin hold changed into negative evidence", state, err)
		}
	}
}

func TestPublicationCancelAndOriginAdmissionHaveOneDurableWinner(t *testing.T) {
	f := enrolledPublicationFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	event, _ := reservedOriginFixture(t, f, "cancel-origin-race")
	signed := originSignedRequest(t, f, event, "cancel-origin-race")
	ready := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	var receipt publication.Receipt
	var cancelErr, admitErr error
	go func() {
		defer wait.Done()
		<-ready
		receipt, cancelErr = f.st.CancelMatrixStoragePublication(ctx, f.scope.Subject, f.scope.Product, signed)
	}()
	go func() {
		defer wait.Done()
		<-ready
		admitErr = f.st.AdmitMatrixPublicationOrigin(ctx, f.scope, event.Sender, event, true)
	}()
	close(ready)
	wait.Wait()
	if cancelErr != nil {
		t.Fatal(cancelErr)
	}
	read, err := f.st.ReadMatrixStoragePublication(ctx, f.scope.Subject, f.scope.Product, f.target.RoomID, signed.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	switch read.Status {
	case "cancelled":
		if admitErr == nil || receipt.Status != "cancelled" {
			t.Fatal("cancel winner still allowed origin")
		}
	case "sending":
		if admitErr != nil || receipt.Status != "sending" {
			t.Fatal("admitted origin was cancelled", admitErr)
		}
	default:
		t.Fatal("race manufactured another state", read.Status)
	}
}

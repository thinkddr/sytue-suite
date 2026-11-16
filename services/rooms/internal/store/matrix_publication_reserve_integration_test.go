//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func signedPublicationFixture(t *testing.T, f *authorityFixture, transaction string) publication.SignedRequest {
	t.Helper()
	value := publication.SignedRequest{Request: publication.Request{Version: 1, Domain: "org.sytue.publication.admission.v1", Namespace: "storage:" + f.scope.Product, Scope: f.scope, Target: f.target, TransactionID: transaction, CipherSHA256: strings.Repeat("b", 64), AuthorityGeneration: 1}}
	value.Signatures = matrixprooftest.Sign(t, f.gate.keys.Device, value, f.gate.keys.Origin.UserID, f.gate.keys.Origin.DeviceID.String())
	return value
}

func enrolledPublicationFixture(t *testing.T) *authorityFixture {
	t.Helper()
	f := newAuthorityFixture(t)
	challenge := f.challenge(t)
	if _, err := f.st.ConfirmMatrixRoot(tenantCtx(f.scope.Tenant), f.scope.Subject, f.scope.Product, f.sign(t, challenge.Action)); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestPublicationReserveCommitsIdenticalMarkerAfterLostResponse(t *testing.T) {
	f := enrolledPublicationFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	signed := signedPublicationFixture(t, f, "stable-sdk-transaction")
	first, err := f.st.ReserveMatrixStoragePublication(ctx, f.scope.Subject, f.scope.Product, signed)
	if err != nil || !first.Valid() || first.Status != "reserved" {
		t.Fatal("reserve", err)
	}
	second, err := f.st.ReserveMatrixStoragePublication(ctx, f.scope.Subject, f.scope.Product, signed)
	if err != nil || second.Marker != first.Marker || second.RequestSHA256 != first.RequestSHA256 {
		t.Fatal("lost reply changed reservation", err)
	}
	read, err := f.st.ReadMatrixStoragePublication(ctx, f.scope.Subject, f.scope.Product, f.target.RoomID, signed.TransactionID)
	if err != nil || read.Marker != first.Marker || read.Status != "reserved" {
		t.Fatal("committed reservation was not visible", err)
	}
	if err := publicationTx(f, true, f.scope.Tenant, f.scope.Subject, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_publications`).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Fatal("reservation duplicated", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	changed := signedPublicationFixture(t, f, signed.TransactionID)
	changed.CipherSHA256 = strings.Repeat("c", 64)
	changed.Signatures = matrixprooftest.Sign(t, f.gate.keys.Device, changed, f.gate.keys.Origin.UserID, f.gate.keys.Origin.DeviceID.String())
	if _, err = f.st.ReserveMatrixStoragePublication(ctx, f.scope.Subject, f.scope.Product, changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("transaction ciphertext changed", err)
	}
}

func TestPublicationReserveRejectsUnsignedForeignAndStaleAuthority(t *testing.T) {
	f := enrolledPublicationFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	for _, fault := range []string{"unsigned", "bad-signature", "foreign-owner", "foreign-product", "resources-namespace", "foreign-room", "generation", "inactive-device"} {
		t.Run(fault, func(t *testing.T) {
			signed := signedPublicationFixture(t, f, "denied-"+fault)
			switch fault {
			case "unsigned":
				signed.Signatures = nil
			case "bad-signature":
				for user, keys := range signed.Signatures {
					for name := range keys {
						keys[name] = "not-a-signature"
					}
					signed.Signatures[user] = keys
				}
			case "foreign-owner":
				signed.Scope.Subject = "other"
			case "foreign-product":
				signed.Scope.Product = "docs"
			case "resources-namespace":
				signed.Namespace = "resources:" + f.scope.Product
			case "foreign-room":
				signed.Target.RoomID = "!foreign:matrix.test"
			case "generation":
				signed.AuthorityGeneration++
			case "inactive-device":
				f.gate.keys.Origin.DeviceActive = false
			}
			if _, err := f.st.ReserveMatrixStoragePublication(ctx, f.scope.Subject, f.scope.Product, signed); err == nil {
				t.Fatal("unauthorized reservation accepted")
			}
			f.gate.keys.Origin.DeviceActive = true
		})
	}
	if err := publicationTx(f, true, f.scope.Tenant, f.scope.Subject, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_publications`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Fatal("failed authorization retained an admission", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

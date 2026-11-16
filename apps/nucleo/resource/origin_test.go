// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

func TestResourceOriginRequestRequiresOriginalCurrentProducerAndExactHold(t *testing.T) {
	f, source, captured := prepareHeld(t, false)
	retained, err := resource.RestoreReservation(t.Context(), source.Held.Resource, source.Held.OpID, source)
	if err != nil {
		t.Fatal(err)
	}
	current, registry := resourcetest.EnrolledSession(t, f.Chain, f.RecipientIdentity)
	p := f.Candidate.Proof
	unsigned := publication.Request{Version: 1, Domain: "org.sytue.publication.admission.v1", Namespace: "resources:" + f.Scope.Product, Scope: f.Scope, Target: storage.PublicationTarget{RoomID: p.RoomID, DeviceID: p.Actor.DeviceID}, TransactionID: p.TransactionID, CipherSHA256: p.OuterCipherSHA256, AuthorityGeneration: 1}
	sign := func(value publication.Request) publication.SignedRequest {
		return publication.SignedRequest{Request: value, Signatures: matrixprooftest.Sign(t, f.Chain.Device, value, f.Chain.Origin.UserID, f.Actor.DeviceID)}
	}
	signed := sign(unsigned)
	verified, err := resource.VerifyOriginRequest(t.Context(), retained, current, signed)
	if err != nil || verified.Reservation().SHA256() != captured.SHA256() || verified.Request().Value() != unsigned || verified.SessionBinding() != current.Binding() {
		t.Fatal("exact original hold rejected", err)
	}
	for _, fault := range []string{"namespace", "scope", "room", "device", "transaction", "cipher", "generation", "signature"} {
		t.Run(fault, func(t *testing.T) {
			changed := unsigned
			switch fault {
			case "namespace":
				changed.Namespace = "storage:" + f.Scope.Product
			case "scope":
				changed.Scope.Tenant = "other"
			case "room":
				changed.Target.RoomID = "!other:example.test"
			case "device":
				changed.Target.DeviceID = "OTHER"
			case "transaction":
				changed.TransactionID = "00000000-0000-4000-8000-000000000002"
			case "cipher":
				changed.CipherSHA256 = f.Epoch.SHA256()
			case "generation":
				changed.AuthorityGeneration = 2
			}
			candidate := sign(changed)
			if fault == "signature" {
				candidate.Signatures = nil
			}
			if _, err := resource.VerifyOriginRequest(t.Context(), retained, current, candidate); err == nil {
				t.Fatal("changed origin request accepted", fault)
			}
		})
	}
	if _, err := resource.VerifyOriginRequest(t.Context(), resource.VerifiedReservation{}, current, signed); err == nil {
		t.Fatal("zero retained hold accepted")
	}
	if _, err := resource.VerifyOriginRequest(t.Context(), retained, resource.VerifiedSession{}, signed); err == nil {
		t.Fatal("public JSON became current device")
	}
	registry.Retire()
	if err := verified.Revalidate(t.Context()); err == nil {
		t.Fatal("origin request survived producer withdrawal")
	}
	if _, err := resource.VerifyOriginRequest(t.Context(), retained, current, signed); err == nil {
		t.Fatal("historical hold revived producer")
	}
}

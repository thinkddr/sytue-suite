// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"bytes"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

func TestResourceBootstrapSourceQueryRequiresCurrentSDKSignatureAndBinding(t *testing.T) {
	f := prepareBootstrapPlanFixture(t)
	key := f.plan.Source.Resource
	unsigned, err := resource.PrepareBootstrapSourceQuery(t.Context(), f.current, key, f.plan.RoomID)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(q resource.BootstrapSourceQuery) resource.SignedBootstrapSourceQuery {
		return resource.SignedBootstrapSourceQuery{BootstrapSourceQuery: q, Signatures: matrixprooftest.Sign(t, f.f.Chain.Device, q, f.f.Chain.Origin.UserID, f.current.Actor().DeviceID)}
	}
	signed := sign(unsigned)
	verified, err := resource.VerifyBootstrapSourceQuery(t.Context(), signed, f.current)
	if err != nil || verified.Value() != unsigned || verified.SessionBinding() != f.current.Binding() {
		t.Fatal("current source query denied", err)
	}
	raw := verified.PublicQueryBytes()
	raw[0] = 0
	if bytes.Equal(raw, verified.PublicQueryBytes()) {
		t.Fatal("source query exposed mutable signed bytes")
	}
	for _, fault := range []string{"namespace", "resource", "owner", "room", "device", "root", "binding", "domain", "signature"} {
		t.Run(fault, func(t *testing.T) {
			changed := unsigned
			switch fault {
			case "namespace":
				changed.OwnerScope.Product = "mail"
				changed.Resource.Product = "mail"
			case "resource":
				changed.Resource.Tenant = "other"
			case "owner":
				changed.OwnerScope.Subject = "other"
			case "room":
				changed.Target.RoomID = "invalid"
			case "device":
				changed.Target.DeviceID = "OTHER"
				changed.Actor.DeviceID = "OTHER"
			case "root":
				changed.Actor.RootKey = f.f.Chain.Self.PublicKey().String()
			case "binding":
				changed.SessionBindingSHA256 = f.f.Epoch.SHA256()
			case "domain":
				changed.Domain = "org.sytue.resource.other"
			}
			forged := sign(changed)
			if fault == "signature" {
				forged.Signatures = nil
			}
			if _, err := resource.VerifyBootstrapSourceQuery(t.Context(), forged, f.current); err == nil {
				t.Fatal("source query accepted another authority", fault)
			}
		})
	}
	f.registry.Retire()
	if err := verified.Revalidate(t.Context()); err == nil {
		t.Fatal("source query revived retired session")
	}
	if _, err := resource.VerifyBootstrapSourceQuery(t.Context(), signed, f.current); err == nil {
		t.Fatal("historical query became current authorization")
	}
	if _, err := resource.PrepareBootstrapSourceQuery(t.Context(), resource.VerifiedSession{}, key, f.plan.RoomID); err == nil {
		t.Fatal("zero session prepared query")
	}
}

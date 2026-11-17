// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"maunium.net/go/mautrix/crypto/signatures"

	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
)

func TestSequencePreservesSQLInt64AndRejectsAmbiguousWireValues(t *testing.T) {
	for _, n := range []int64{0, 1, 1 << 53, math.MaxInt64} {
		seq, err := resource.Sequence(n)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(seq)
		if err != nil {
			t.Fatal(err)
		}
		var decoded resource.Decimal
		if json.Unmarshal(raw, &decoded) != nil {
			t.Fatal("sequence not a decimal string")
		}
		actual, err := decoded.Int64()
		if err != nil || actual != n {
			t.Fatal("sequence precision lost", n, actual, err)
		}
	}
	for _, value := range []resource.Decimal{"", "-1", "+1", "00", "01", "1.0", "1e2", " 1", "9223372036854775808"} {
		if _, err := value.Int64(); err == nil {
			t.Fatal("ambiguous sequence accepted", value)
		}
	}
	var seq resource.Decimal
	if json.Unmarshal([]byte("9007199254740993"), &seq) == nil {
		t.Fatal("JavaScript number accepted")
	}
	if _, err := resource.Decimal("9223372036854775807").Next(1); err == nil {
		t.Fatal("sequence overflow accepted")
	}
	if _, err := resource.Decimal("0").Next(0); err == nil {
		t.Fatal("zero-op commit accepted")
	}
}

func TestPublicationTransactionIsCanonicalNonNilUUID(t *testing.T) {
	f := resourcetest.New(t, false)
	for _, value := range []string{"", "00000000-0000-0000-0000-000000000000", "attempt-original", "00000000000040008000000000000001", "urn:uuid:00000000-0000-4000-8000-000000000001", strings.ToUpper("abcdef00-0000-4000-8000-000000000001")} {
		p := f.Candidate.Proof.Publication
		p.TransactionID = value
		if p.Valid() {
			t.Fatal("ambiguous transaction accepted", value)
		}
	}
	p := f.Candidate.Proof.Publication
	p.TransactionID = "abcdef00-0000-4000-8000-000000000001"
	if !p.Valid() {
		t.Fatal("canonical client WAL transaction rejected")
	}
}

func TestResourceAuthorityAcceptsRoomVersion12OpaqueID(t *testing.T) {
	base := resourcetest.New(t, false)
	room := "!Nhcu5BS-UMnFX7hBVfVSoXiD7OgH6iRT-xyIuqDnpYQ"
	fixture := resourcetest.NewWithSDK(t, false, base.Chain.Origin, base.Chain.Master.PublicKey(), func(value any) signatures.Signatures {
		return matrixprooftest.Sign(t, base.Chain.Device, value, base.Chain.Origin.UserID, base.Actor.DeviceID)
	}, room, base.Candidate.EncryptedEvent)
	access, err := resource.Authorize(fixture.Epoch, fixture.Cap, fixture.Scope.Subject, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resource.VerifyCandidate(t.Context(), fixture.Candidate, access, fixture.Device, nil); err != nil {
		t.Fatal("v12 resource rejected", err)
	}
	changed := fixture.Candidate
	changed.Proof.RoomID = room + "different"
	if _, err := resource.VerifyCandidate(t.Context(), changed, access, fixture.Device, nil); err == nil {
		t.Fatal("opaque room ID bypassed exact SDK signature binding")
	}
}

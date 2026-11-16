// SPDX-License-Identifier: Apache-2.0
package matrixproof_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

func TestHumanDeviceProofKeepsIsolatedBoundaryAndIndependentExpectedUser(t *testing.T) {
	scope := storage.Scope{Tenant: "fixture", Subject: "owner", Product: "rooms"}
	f := matrixprooftest.NewUser(t, scope, "matrix.test", "@human:matrix.test", "ROOMS_DEVICE")
	if _, err := matrixproof.Verify(t.Context(), f.Origin, f.Master.PublicKey()); err == nil {
		t.Fatal("human identity crossed isolated storage boundary")
	}
	device, err := matrixproof.VerifyUser(t.Context(), f.Origin, "@human:matrix.test", f.Master.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	target := storage.PublicationTarget{RoomID: "!resource:matrix.test", DeviceID: "ROOMS_DEVICE"}
	value := struct {
		Product    string `json:"product"`
		Signatures any    `json:"signatures,omitempty"`
	}{Product: "rooms"}
	value.Signatures = matrixprooftest.Sign(t, f.Device, value, f.Origin.UserID, f.Origin.DeviceID.String())
	if err := device.VerifyJSON(t.Context(), scope, target, value); err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"user", "server", "account", "device", "root", "product", "tenant", "producer", "value"} {
		t.Run(fault, func(t *testing.T) {
			origin := f.Origin
			expected := id.UserID("@human:matrix.test")
			root := f.Master.PublicKey()
			s := scope
			p := target
			object := value
			switch fault {
			case "user":
				expected = "@other:matrix.test"
			case "server":
				origin.ServerName = "untrusted.test"
			case "account":
				origin.AccountActive = false
			case "device":
				origin.DeviceActive = false
			case "root":
				root = matrixprooftest.NewSigning(t).PublicKey()
			case "product":
				s.Product = "mail"
			case "tenant":
				s.Tenant = "another"
			case "producer":
				p.DeviceID = "OTHER_DEVICE"
			case "value":
				object.Product = "mail"
			}
			verified, err := matrixproof.VerifyUser(t.Context(), origin, expected, root)
			if err == nil {
				err = verified.VerifyJSON(t.Context(), s, p, object)
			}
			if err == nil {
				t.Fatal("changed binding accepted", fault)
			}
		})
	}
}

func TestCanonicalBytesUseActualJSONWireContract(t *testing.T) {
	value := struct {
		Absent   []string `json:"absent,omitempty"`
		Sequence string   `json:"sequence"`
		Text     string   `json:"text"`
	}{Sequence: "9223372036854775807", Text: "á<"}
	raw, err := matrixproof.CanonicalBytes(value)
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte("{\"sequence\":\"9223372036854775807\",\"text\":\"á<\"}")
	if !bytes.Equal(raw, expected) {
		t.Fatalf("wire canonical JSON changed: %s", raw)
	}
	var decoded map[string]any
	if json.Unmarshal(raw, &decoded) != nil || decoded["sequence"] != "9223372036854775807" {
		t.Fatal("int64 precision changed")
	}
}

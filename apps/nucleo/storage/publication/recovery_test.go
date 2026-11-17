// SPDX-License-Identifier: Apache-2.0
package publication_test

import (
	"encoding/json"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

func recoveryStateFixture(t *testing.T, status string) (publication.RecoveryState, publication.RecoverySelection) {
	t.Helper()
	scope := storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}
	f := matrixprooftest.New(t, scope, "matrix.test", "PRODUCER_A")
	prepared, err := publication.BuildRequest(scope, "storage:mail", storage.PublicationTarget{RoomID: "!original:matrix.test", DeviceID: "PRODUCER_A"}, "original.1", 2, []byte(`{"algorithm":"m.megolm.v1.aes-sha2","session_id":"synthetic-recovery","ciphertext":"opaque"}`))
	if err != nil {
		t.Fatal(err)
	}
	original := publication.SignedRequest{Request: prepared.Request, Signatures: matrixprooftest.Sign(t, f.Device, prepared.Request, f.Origin.UserID, f.Origin.DeviceID.String())}
	receipt := publication.Receipt{Version: 1, Marker: publication.Marker{Version: 1, ID: strings.Repeat("A", 43)}, Status: status, RequestSHA256: prepared.SHA256}
	if status == "accepted" {
		receipt.Reference = &storage.Reference{RoomID: original.Target.RoomID, EventID: "$accepted"}
	}
	state, err := publication.RecoveryFromStored(original, receipt)
	if err != nil {
		t.Fatal(err)
	}
	return state, publication.RecoverySelection{Scope: scope, RoomID: original.Target.RoomID, TransactionID: original.TransactionID}
}

func TestPublicationRecoveryRetainsADataButNeverReturnsAnAdmissionMarker(t *testing.T) {
	for _, status := range []string{"reserved", "sending", "accepted", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			state, selection := recoveryStateFixture(t, status)
			raw, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), `"marker"`) {
				t.Fatal("read-only recovery leaked send marker")
			}
			hint, err := publication.DecodeRecoveryHint(raw, selection)
			if err != nil || hint.Status() != status || hint.Original() != state.Original.Request || hint.RequestSHA256() != state.RequestSHA256 {
				t.Fatal("retained transaction changed", err)
			}
			reference, accepted := hint.AcceptedReference()
			if accepted != (status == "accepted") {
				t.Fatal("nonaccepted hint became an event reference")
			}
			if accepted && (reference.RoomID != selection.RoomID || reference.EventID != "$accepted") {
				t.Fatal("accepted reference changed")
			}
			if hint.Original().Target.DeviceID != "PRODUCER_A" {
				t.Fatal("reader B replaced the historical producer")
			}
		})
	}
	if _, accepted := (publication.RecoveryHint{}).AcceptedReference(); accepted {
		t.Fatal("empty hint became accepted")
	}
}

func TestPublicationRecoveryRejectsCrossedSelectorOrReceiptWithoutLiveDeviceA(t *testing.T) {
	for _, fault := range []string{"tenant", "subject", "product", "room", "transaction", "hash", "reference", "missing-reference", "status", "unsigned", "namespace", "version", "unknown-marker", "oversize", "trailing"} {
		t.Run(fault, func(t *testing.T) {
			value, selection := recoveryStateFixture(t, "accepted")
			switch fault {
			case "tenant":
				selection.Scope.Tenant = "other"
			case "subject":
				selection.Scope.Subject = "reader-other-person"
			case "product":
				selection.Scope.Product = "docs"
			case "room":
				selection.RoomID = "!new-room:matrix.test"
			case "transaction":
				selection.TransactionID = "different.1"
			case "hash":
				value.RequestSHA256 = strings.Repeat("0", 64)
			case "reference":
				value.Reference = &storage.Reference{RoomID: "!other:matrix.test", EventID: "$accepted"}
			case "missing-reference":
				value.Reference = nil
			case "status":
				value.Status = "ready"
			case "unsigned":
				value.Original.Signatures = nil
			case "namespace":
				value.Original.Namespace = "resources:mail"
			case "version":
				value.Version = 2
			}
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if fault == "unknown-marker" {
				var object map[string]json.RawMessage
				_ = json.Unmarshal(raw, &object)
				object["marker"] = json.RawMessage(`{"v":1,"id":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`)
				raw, _ = json.Marshal(object)
			}
			if fault == "oversize" {
				raw = append(raw, []byte(strings.Repeat(" ", 32769))...)
			}
			if fault == "trailing" {
				raw = append(raw, []byte("{}")...)
			}
			if _, err := publication.DecodeRecoveryHint(raw, selection); err == nil {
				t.Fatal("crossed recovery data accepted", fault)
			}
		})
	}
}

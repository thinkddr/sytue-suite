// SPDX-License-Identifier: Apache-2.0
package publication_test

import (
	"encoding/json"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

func receiptRequest() publication.Request {
	return publication.Request{Version: 1, Domain: "org.sytue.publication.admission.v1", Namespace: "storage:mail", Scope: storage.Scope{Tenant: "fixture", Subject: "owner", Product: "mail"}, Target: storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "DEVICE"}, TransactionID: "retained-intent.0001", CipherSHA256: strings.Repeat("a", 64), AuthorityGeneration: 9223372036854775807}
}

func TestPublicationReceiptBindsExactCanonicalRequestAndOnlyReservedReturnsMarker(t *testing.T) {
	request := receiptRequest()
	hash, err := matrixproof.RequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"reserved", "sending", "accepted", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			receipt := publication.Receipt{Version: 1, Marker: publication.Marker{Version: 1, ID: strings.Repeat("A", 43)}, Status: status, RequestSHA256: hash}
			if status == "accepted" {
				receipt.Reference = &storage.Reference{RoomID: request.Target.RoomID, EventID: "$opaque"}
			}
			raw, err := json.Marshal(receipt)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := publication.DecodeReceipt(raw, request)
			if err != nil || parsed.Status != status || parsed.Marker != receipt.Marker {
				t.Fatal("matching receipt rejected", err)
			}
			marker, err := parsed.ReservedFor(request)
			if status == "reserved" {
				if err != nil || marker != receipt.Marker {
					t.Fatal("reserved marker lost", err)
				}
			} else if err == nil || marker.Valid() {
				t.Fatal("nonreserved state became a sending permit", status)
			}
			for _, field := range []string{"scope", "namespace", "room", "device", "txn", "generation", "cipher"} {
				expected := request
				switch field {
				case "scope":
					expected.Scope.Subject = "other"
				case "namespace":
					expected.Namespace = "resources:mail"
				case "room":
					expected.Target.RoomID = "!other:matrix.test"
				case "device":
					expected.Target.DeviceID = "OTHER"
				case "txn":
					expected.TransactionID = "other"
				case "generation":
					expected.AuthorityGeneration--
				case "cipher":
					expected.CipherSHA256 = strings.Repeat("b", 64)
				}
				if _, err := publication.DecodeReceipt(raw, expected); err == nil {
					t.Fatal("receipt crossed request", field)
				}
			}
		})
	}
}

func TestPublicationReceiptRejectsMalformedAndCrossedAcceptance(t *testing.T) {
	request := receiptRequest()
	hash, err := matrixproof.RequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	base := publication.Receipt{Version: 1, Marker: publication.Marker{Version: 1, ID: strings.Repeat("A", 43)}, Status: "reserved", RequestSHA256: hash}
	for _, fault := range []string{"hash", "marker", "status", "version", "reserved-reference", "accepted-without-reference", "accepted-other-room", "unknown-field", "trailing", "oversized"} {
		t.Run(fault, func(t *testing.T) {
			value := base
			switch fault {
			case "hash":
				value.RequestSHA256 = strings.Repeat("b", 64)
			case "marker":
				value.Marker.ID = "not-canonical"
			case "status":
				value.Status = "ready"
			case "version":
				value.Version = 2
			case "reserved-reference":
				value.Reference = &storage.Reference{RoomID: request.Target.RoomID, EventID: "$opaque"}
			case "accepted-without-reference":
				value.Status = "accepted"
			case "accepted-other-room":
				value.Status = "accepted"
				value.Reference = &storage.Reference{RoomID: "!other:matrix.test", EventID: "$opaque"}
			}
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			switch fault {
			case "unknown-field":
				raw = append(raw[:len(raw)-1], []byte(`,"allow":true}`)...)
			case "trailing":
				raw = append(raw, []byte(" {}")...)
			case "oversized":
				raw = append(raw, []byte(strings.Repeat(" ", 16384))...)
			}
			if _, err := publication.DecodeReceipt(raw, request); err == nil {
				t.Fatal("invalid receipt accepted", fault)
			}
		})
	}
}

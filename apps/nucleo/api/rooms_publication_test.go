// SPDX-License-Identifier: Apache-2.0
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

func publicationAPIFixture(product string) publication.SignedRequest {
	return publication.SignedRequest{Request: publication.Request{Version: 1, Domain: "org.sytue.publication.admission.v1", Namespace: "storage:" + product, Scope: storage.Scope{Tenant: "acme", Subject: "owner", Product: product}, Target: storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "DEVICE"}, TransactionID: "fixed.1", CipherSHA256: strings.Repeat("a", 64), AuthorityGeneration: 2}, Signatures: signatures.NewSingleSignature(id.UserID("@owner:matrix.test"), id.KeyAlgorithmEd25519, "DEVICE", "artificial-public-fixture-signature")}
}

func TestPublicationAPIAllProductsKeepsOriginalIntentAndNeverUpgradesSending(t *testing.T) {
	for _, product := range storage.Products() {
		t.Run(product, func(t *testing.T) {
			proof := publicationAPIFixture(product)
			hash, err := matrixproof.RequestDigest(proof.Request)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				scope, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader))
				if err != nil || scope != proof.Scope {
					t.Error("API changed original scope", err)
				}
				root := "/api/v1/matrix/storage/" + product + "/publications"
				status := "reserved"
				switch calls {
				case 1, 2:
					suffix := "/reserve"
					if calls == 2 {
						suffix = "/cancel"
						status = "cancelled"
					}
					if r.Method != http.MethodPost || r.URL.Path != root+suffix || r.URL.RawQuery != "" {
						t.Error("wrong fixed mutation route")
					}
					var got publication.SignedRequest
					if json.NewDecoder(r.Body).Decode(&got) != nil || got.Request != proof.Request || len(got.Signatures) == 0 {
						t.Error("API changed exact signed request")
					}
				case 3:
					status = "sending"
					if r.Method != http.MethodGet || r.URL.Path != root || len(r.URL.Query()) != 2 || r.URL.Query().Get("room_id") != proof.Target.RoomID || r.URL.Query().Get("transaction_id") != proof.TransactionID {
						t.Error("wrong ACK-only readback route")
					}
				}
				if err := json.NewEncoder(w).Encode(publication.Receipt{Version: 1, Marker: publication.Marker{Version: 1, ID: strings.Repeat("A", 43)}, Status: status, RequestSHA256: hash}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client := NewRooms(server.URL, nil, server.Client())
			reserved, err := client.ReserveMatrixStoragePublication(t.Context(), proof)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reserved.ReservedFor(proof.Request); err != nil {
				t.Fatal("reserved exact receipt lost", err)
			}
			cancelled, err := client.CancelMatrixStoragePublication(t.Context(), proof)
			if err != nil || cancelled.Status != "cancelled" {
				t.Fatal("cancel receipt lost", err)
			}
			sending, err := client.ReadMatrixStoragePublication(t.Context(), proof.Request)
			if err != nil || sending.Status != "sending" {
				t.Fatal("ACK-only status changed", err)
			}
			if _, err := sending.ReservedFor(proof.Request); err == nil {
				t.Fatal("sending status authorized another message HTTP")
			}
			if calls != 3 {
				t.Fatal("missing API request", calls)
			}
		})
	}
}

func TestPublicationAPIRejectsForeignReceiptAndMalformedRequestBeforeHTTP(t *testing.T) {
	proof := publicationAPIFixture("mail")
	for _, fault := range []string{"hash", "room", "oversize"} {
		t.Run(fault, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if fault == "oversize" {
					_, _ = w.Write([]byte(strings.Repeat(" ", 16385)))
					return
				}
				digest, _ := matrixproof.RequestDigest(proof.Request)
				ref := storage.Reference{RoomID: proof.Target.RoomID, EventID: "$accepted"}
				if fault == "hash" {
					digest = strings.Repeat("b", 64)
				} else {
					ref.RoomID = "!foreign:matrix.test"
				}
				_ = json.NewEncoder(w).Encode(publication.Receipt{Version: 1, Marker: publication.Marker{Version: 1, ID: strings.Repeat("A", 43)}, Status: "accepted", RequestSHA256: digest, Reference: &ref})
			}))
			defer server.Close()
			if _, err := NewRooms(server.URL, nil, server.Client()).ReserveMatrixStoragePublication(t.Context(), proof); err == nil {
				t.Fatal("foreign receipt accepted", fault)
			}
		})
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	client := NewRooms(server.URL, nil, server.Client())
	for _, fault := range []string{"namespace", "unsigned", "scope", "transaction"} {
		bad := proof
		switch fault {
		case "namespace":
			bad.Namespace = "resources:mail"
		case "unsigned":
			bad.Signatures = nil
		case "scope":
			bad.Scope.Product = "docs"
		case "transaction":
			bad.TransactionID = "bad\nintent"
		}
		if _, err := client.ReserveMatrixStoragePublication(t.Context(), bad); err == nil {
			t.Fatal("invalid proof reached HTTP", fault)
		}
	}
	if calls != 0 {
		t.Fatal("malformed request reached network", calls)
	}
}

func TestPublicationAPIRecoveryAllProductsKeepsOriginalProducerWithoutSendMarker(t *testing.T) {
	for _, product := range storage.Products() {
		t.Run(product, func(t *testing.T) {
			proof := publicationAPIFixture(product)
			digest, err := matrixproof.RequestDigest(proof.Request)
			if err != nil {
				t.Fatal(err)
			}
			state, err := publication.RecoveryFromStored(proof, publication.Receipt{Version: 1, Marker: publication.Marker{Version: 1, ID: strings.Repeat("A", 43)}, Status: "accepted", RequestSHA256: digest, Reference: &storage.Reference{RoomID: proof.Target.RoomID, EventID: "$original"}})
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				scope, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader))
				if err != nil || scope != proof.Scope {
					t.Error("recovery lost owner scope", err)
				}
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/matrix/storage/"+product+"/publications/recovery" || len(r.URL.Query()) != 2 || r.URL.Query().Get("room_id") != proof.Target.RoomID || r.URL.Query().Get("transaction_id") != proof.TransactionID {
					t.Error("recovery changed original selector")
				}
				_ = json.NewEncoder(w).Encode(state)
			}))
			defer server.Close()
			selection := publication.RecoverySelection{Scope: proof.Scope, RoomID: proof.Target.RoomID, TransactionID: proof.TransactionID}
			hint, err := NewRooms(server.URL, nil, server.Client()).ReadMatrixStoragePublicationRecovery(t.Context(), selection)
			ref, accepted := hint.AcceptedReference()
			if err != nil || calls != 1 || hint.Original() != proof.Request || !accepted || ref.EventID != "$original" {
				t.Fatal("recovery changed public history", err)
			}
		})
	}
}

func TestPublicationAPIRecoveryRejectsSubstitutionAndInvalidSelector(t *testing.T) {
	proof := publicationAPIFixture("mail")
	digest, _ := matrixproof.RequestDigest(proof.Request)
	original, _ := publication.RecoveryFromStored(proof, publication.Receipt{Version: 1, Marker: publication.Marker{Version: 1, ID: strings.Repeat("A", 43)}, Status: "sending", RequestSHA256: digest})
	selection := publication.RecoverySelection{Scope: proof.Scope, RoomID: proof.Target.RoomID, TransactionID: proof.TransactionID}
	for _, fault := range []string{"scope", "hash", "room", "marker", "oversize"} {
		t.Run(fault, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				value := original
				switch fault {
				case "scope":
					value.Original.Scope.Subject = "foreign"
				case "hash":
					value.RequestSHA256 = strings.Repeat("b", 64)
				case "room":
					value.Original.Target.RoomID = "!other:matrix.test"
				case "marker":
					raw, _ := json.Marshal(value)
					_, _ = w.Write(append(raw[:len(raw)-1], []byte(`,"marker":{"v":1,"id":"forbidden"}}`)...))
					return
				case "oversize":
					_, _ = w.Write([]byte(strings.Repeat(" ", 32769)))
					return
				}
				_ = json.NewEncoder(w).Encode(value)
			}))
			defer server.Close()
			if _, err := NewRooms(server.URL, nil, server.Client()).ReadMatrixStoragePublicationRecovery(t.Context(), selection); err == nil {
				t.Fatal("invalid public hint accepted")
			}
		})
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	bad := selection
	bad.TransactionID = "bad\nintent"
	if _, err := NewRooms(server.URL, nil, server.Client()).ReadMatrixStoragePublicationRecovery(t.Context(), bad); err == nil || calls != 0 {
		t.Fatal("invalid selector reached network", err, calls)
	}
}

//go:build integration

// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

func TestLiveMailPostAuditExactBrowserCasePreservesEveryNativeWorkerCheck(t *testing.T) {
	expected := errors.New("original worker receipt assertion")
	for _, name := range []string{"TestPrivateIngressLiveNativeMailSharedOutbound", "TestPrivateIngressLiveNativeMailApplication", "TestPrivateIngressLiveNativeMailPostgresSource", "TestPrivateIngressLiveNativeMailBrowserFullSource/child", "unknown"} {
		t.Run(name, func(t *testing.T) {
			browser, worker := 0, 0
			err := runLiveMailPostAudit(name, func() error { browser++; return nil }, func() error { worker++; return expected })
			if !errors.Is(err, expected) || browser != 0 || worker != 1 {
				t.Fatal("a non-browser proof omitted the original worker assertions")
			}
		})
	}
	browser, worker := 0, 0
	if err := runLiveMailPostAudit("TestPrivateIngressLiveNativeMailBrowserFullSource", func() error { browser++; return nil }, func() error { worker++; return expected }); err != nil || browser != 1 || worker != 0 {
		t.Fatal("exact BrowserFull inherited an unrelated worker claim", err)
	}
}

func TestBrowserPublicationObserverRetainsOnlyExactAcceptedPublicRouting(t *testing.T) {
	wire := []byte("{\"algorithm\":\"m.megolm.v1.aes-sha2\",\"ciphertext\":\"artificial-sdk-ciphertext\",\"session_id\":\"artificial-session\"}")
	hash, _, err := publication.WireDigest(wire, false)
	if err != nil {
		t.Fatal(err)
	}
	marker := publication.Marker{Version: 1, ID: strings.Repeat("A", 43)}
	marked, err := publication.AttachMarker(wire, marker)
	if err != nil {
		t.Fatal(err)
	}
	request := publication.Request{Version: 1, Domain: "org.sytue.publication.admission.v1", Namespace: "storage:mail", Scope: storage.Scope{Tenant: "tenant", Subject: "owner", Product: "mail"}, Target: storage.PublicationTarget{RoomID: "!room:test", DeviceID: "DEVICE"}, TransactionID: "immutable-txn", CipherSHA256: hash, AuthorityGeneration: 1}
	sha, err := matrixproof.RequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	receipt := publication.Receipt{Version: 1, Marker: marker, Status: "reserved", RequestSHA256: sha}
	event := publication.OriginEvent{Version: 1, Reference: storage.Reference{RoomID: request.Target.RoomID, EventID: "$accepted"}, Sender: "@owner:test", Type: "m.room.encrypted", Content: marked}
	origin := &mailBrowserOrigin{}
	origin.observeReserved(request, receipt)
	origin.observeAccepted(event)
	origin.observeAccepted(event)
	route := origin.routes[marker.ID]
	if origin.auditError != nil || route.Reference == nil || *route.Reference != event.Reference || route.Room != request.Target.RoomID || route.TransactionID != request.TransactionID || route.RequestSHA256 != sha || route.CipherSHA256 != hash {
		t.Fatal("accepted routing changed or lost its exact commitment")
	}
	event.Reference.EventID = "$other"
	origin.observeAccepted(event)
	if origin.auditError == nil {
		t.Fatal("an accepted routing reference changed silently")
	}
	origin = &mailBrowserOrigin{}
	origin.observeReserved(request, receipt)
	event.Reference.EventID = "$accepted"
	changed, err := publication.AttachMarker([]byte("{\"algorithm\":\"m.megolm.v1.aes-sha2\",\"ciphertext\":\"different-artificial-ciphertext\",\"session_id\":\"artificial-session\"}"), marker)
	if err != nil {
		t.Fatal(err)
	}
	event.Content = changed
	origin.observeAccepted(event)
	if origin.auditError == nil {
		t.Fatal("accepted ciphertext commitment changed silently")
	}
	origin = &mailBrowserOrigin{}
	origin.observeAccepted(event)
	if origin.auditError == nil {
		t.Fatal("unknown marker became accepted public evidence")
	}
	origin = &mailBrowserOrigin{}
	origin.observeReserved(request, receipt)
	event.Content = []byte("{}")
	origin.strictPhase.Store(true)
	origin.observeAccepted(event)
	if origin.auditError == nil {
		t.Fatal("strict accepted event without marker became evidence")
	}
}

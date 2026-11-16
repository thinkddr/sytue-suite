// SPDX-License-Identifier: Apache-2.0
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

func TestMatrixResourceBootstrapSourceRetainsSignedIntentAndExactOwner(t *testing.T) {
	f := resourcetest.New(t, false)
	current, _ := resourcetest.EnrolledSession(t, f.Chain, f.RecipientIdentity)
	key := f.Epoch.Value().Resource
	query, err := resource.PrepareBootstrapSourceQuery(t.Context(), current, key, f.Epoch.Value().RoomID)
	if err != nil {
		t.Fatal(err)
	}
	signed := resource.SignedBootstrapSourceQuery{BootstrapSourceQuery: query, Signatures: matrixprooftest.Sign(t, f.Chain.Device, query, f.Chain.Origin.UserID, f.Actor.DeviceID)}
	snapshot := resource.BootstrapSourceSnapshot{Resource: key, OwnerScope: f.Scope, SourceEpoch: uuid.NewString(), SourceSeq: "9007199254740993", ACLVersion: "0", History: resource.HistoryFull}
	var calls atomic.Int32
	var wrong atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/v1/matrix/resources/docs/bootstrap/source" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer current-app-session" {
			t.Error("source request lost current API session or selected another route")
		}
		var received resource.SignedBootstrapSourceQuery
		if json.NewDecoder(r.Body).Decode(&received) != nil || received.BootstrapSourceQuery != query || len(received.Signatures) == 0 {
			t.Error("signed source intent changed")
		}
		out := snapshot
		if wrong.Load() {
			out.OwnerScope.Subject = "other"
		}
		json.NewEncoder(w).Encode(out)
	}))
	defer server.Close()
	client := NewRooms(server.URL, sesionFija("current-app-session"), server.Client())
	got, err := client.MatrixResourceBootstrapSource(t.Context(), "docs", signed)
	if err != nil || got != snapshot {
		t.Fatal("exact source tuple rejected", err)
	}
	wrong.Store(true)
	if _, err = client.MatrixResourceBootstrapSource(t.Context(), "docs", signed); !errors.Is(err, ErrMatrixSSO) {
		t.Fatal("another owner accepted from backend", err)
	}
	before := calls.Load()
	if _, err = client.MatrixResourceBootstrapSource(t.Context(), "mail", signed); !errors.Is(err, ErrMatrixSSO) || calls.Load() != before {
		t.Fatal("source request crossed product boundary", err)
	}
}

// SPDX-License-Identifier: Apache-2.0
package resourcepg_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
)

func TestBootstrapHTTPSourceRequiresFreshMACAndExactOriginalContext(t *testing.T) {
	f := resourcetest.New(t, false)
	current, registry := resourcetest.EnrolledSession(t, f.Chain, f.RecipientIdentity)
	key := f.Epoch.Value().Resource
	unsigned, err := resource.PrepareBootstrapSourceQuery(t.Context(), current, key, f.Epoch.Value().RoomID)
	if err != nil {
		t.Fatal(err)
	}
	signed := resource.SignedBootstrapSourceQuery{BootstrapSourceQuery: unsigned, Signatures: matrixprooftest.Sign(t, f.Chain.Device, unsigned, f.Chain.Origin.UserID, f.Actor.DeviceID)}
	verified, err := resource.VerifyBootstrapSourceQuery(t.Context(), signed, current)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := resource.BootstrapSourceSnapshot{Resource: key, OwnerScope: f.Scope, SourceEpoch: uuid.NewString(), SourceSeq: "9007199254740993", ACLVersion: "0", History: resource.HistoryFull}
	secret := strings.Repeat("a", 64)
	var mode atomic.Value
	mode.Store("valid")
	var trapCalls atomic.Int32
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { trapCalls.Add(1); w.WriteHeader(http.StatusNoContent) }))
	defer trap.Close()
	var previous atomic.Value
	server := httptest.NewServer(httpx.SoloDesdeDentro(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q httpx.ResourceBootstrapSourceRequest
		if r.URL.Path != httpx.ResourceBootstrapSourcePath || httpx.DecodeJSONLimit(w, r, &q, 32768) != nil || q.Version != 1 || !httpx.ValidMetadataNonce(q.Nonce) {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		selected := mode.Load().(string)
		if selected == "redirect" {
			http.Redirect(w, r, trap.URL, http.StatusTemporaryRedirect)
			return
		}
		if selected == "replay" {
			httpx.JSON(w, r, http.StatusOK, previous.Load().(httpx.ResourceBootstrapSourceResponse))
			return
		}
		response := httpx.ResourceBootstrapSourceResponse{Version: 1, Nonce: q.Nonce, Snapshot: snapshot}
		switch selected {
		case "wrong-owner":
			response.Snapshot.OwnerScope.Subject = "other"
		case "wrong-resource":
			response.Snapshot.Resource.ID = "another"
		case "wrong-nonce":
			response.Nonce = strings.Repeat("b", 64)
		case "wrong-version":
			response.Version = 2
		}
		domain := httpx.ResourceBootstrapSourceProofDomain
		if selected == "wrong-domain" {
			domain = "sytue-resource-session-origin-v1"
		}
		mac, macErr := httpx.MetadataMAC(secret, domain, q, response)
		if macErr != nil {
			t.Error(macErr)
			httpx.Fail(w, r, httpx.ErrUnavailable)
			return
		}
		response.MAC = mac
		if selected == "tampered" {
			response.Snapshot.ACLVersion = "99"
		}
		if selected == "valid" {
			previous.Store(response)
		}
		w.Header().Set("Content-Type", "application/json")
		if selected == "unknown" {
			raw, _ := json.Marshal(response)
			raw = append(raw[:len(raw)-1], []byte(",\"extra\":true}")...)
			w.Write(raw)
			return
		}
		httpx.JSON(w, r, http.StatusOK, response)
		if selected == "extra-JSON" {
			w.Write([]byte("{}"))
		}
	})))
	defer server.Close()
	endpoint, err := resourcepg.NewOriginSourceEndpoint("docs", server.URL, secret, server.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	source, err := endpoint.BootstrapSource(verified)
	if err != nil {
		t.Fatal(err)
	}
	got, err := source.BootstrapSource(t.Context(), key)
	if err != nil || got != snapshot {
		t.Fatal("valid exact source tuple denied", err)
	}
	for _, selected := range []string{"replay", "wrong-owner", "wrong-resource", "wrong-nonce", "wrong-version", "wrong-domain", "tampered", "unknown", "extra-JSON", "redirect"} {
		mode.Store(selected)
		if _, err = source.BootstrapSource(t.Context(), key); err == nil {
			t.Fatal("untrusted source response accepted", selected)
		}
	}
	if trapCalls.Load() != 0 {
		t.Fatal("internal source credential followed redirect")
	}
	mode.Store("valid")
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = source.BootstrapSource(cancelled, key); !errors.Is(err, context.Canceled) {
		t.Fatal("source lost original caller cancellation", err)
	}
	registry.Retire()
	if _, err = source.BootstrapSource(t.Context(), key); err == nil {
		t.Fatal("old verified query survived producer withdrawal")
	}
}

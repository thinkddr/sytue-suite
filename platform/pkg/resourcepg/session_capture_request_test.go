// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

func TestCurrentEvidenceRequestIsOneUseAndFutureChecksRemainLive(t *testing.T) {
	f := resourcetest.New(t, false)
	_, registry := resourcetest.EnrolledSession(t, f.Chain, f.RecipientIdentity)
	secret := strings.Repeat("a", 64)
	var reads atomic.Int64
	remote := httptest.NewServer(httpx.SoloDesdeDentro(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q SessionOriginRequest
		if httpx.DecodeJSONLimit(w, r, &q, 4096) != nil || !q.ValidForService("docs") {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		reads.Add(1)
		record, err := registry.CurrentResourceSession(r.Context(), q.Scope, q.DeviceID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrForbidden)
			return
		}
		response, err := SignSessionOriginResponse(secret, q, SessionOriginResponse{Version: 1, Record: record, Origin: f.Chain.Origin})
		if err != nil {
			httpx.Fail(w, r, httpx.ErrUnavailable)
			return
		}
		httpx.JSON(w, r, http.StatusOK, response)
	})))
	defer remote.Close()
	authority, err := NewSessionAuthorityClient("docs", remote.URL, secret, remote.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	request := &currentEvidenceRequest{authority: authority, scope: f.Scope, device: f.Actor.DeviceID}
	provider, err := resource.NewCurrentSessionProvider(request)
	if err != nil {
		t.Fatal(err)
	}
	current, err := provider.Resolve(t.Context(), f.Scope, f.Actor.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := request.take()
	if err != nil || evidence.Record.Binding != current.Binding() || request.value != nil {
		t.Fatal("request witness retained captured public data after consumption", err)
	}
	if _, err := request.take(); err == nil {
		t.Fatal("one-use witness returned public evidence twice")
	}
	if _, _, err := request.CaptureCurrentResourceSession(t.Context(), f.Scope, f.Actor.DeviceID); err == nil {
		t.Fatal("one-use current request witness accepted second capture")
	}
	if reads.Load() != 1 {
		t.Fatal("Resolve reused request nonce or repeated authority", reads.Load())
	}
	if err := current.Revalidate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 4 {
		t.Fatal("future live checks reused request witness", reads.Load())
	}
	registry.Retire()
	if err := current.Revalidate(t.Context()); err == nil {
		t.Fatal("retirement bypassed consumed request witness")
	}
}

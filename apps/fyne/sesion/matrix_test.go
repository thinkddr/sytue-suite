// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package sesion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestMatrixStorageReturnBelongsToTheNativeAppAndPendingFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"product": input["product"], "state": "bound-state", "authorize_url": "https://rooms.sytue.eu/rooms/matrix/native/start?state=bound-state"})
	}))
	defer server.Close()
	client := api.NewRooms(server.URL, nil, server.Client())
	for _, tc := range []struct{ product, native string }{{"hub", "one"}, {"drive", "drive"}, {"mail", "mail"}, {"rooms", "rooms"}} {
		start, err := client.MatrixNativeStartFor(t.Context(), tc.product)
		if err != nil {
			t.Fatal(err)
		}
		flow := &matrixAuthorization{start: start, arrived: make(chan api.MatrixNativeCallback, 1)}
		s := Suite{apex: "sytue.eu", app: consentApp{metadata: fyne.AppMetadata{Custom: map[string]string{"esquema": api.Esquema(tc.native)}}}, matrixConsent: flow}
		own, _ := url.Parse(api.Esquema(tc.native) + "://matrix/callback?state=bound-state&ticket=opaque-ticket")
		other, _ := url.Parse("sytue-unregistered://matrix/callback?state=bound-state&ticket=opaque-ticket")
		wrong, _ := url.Parse(api.Esquema(tc.native) + "://matrix/callback?state=other&ticket=opaque-ticket")
		credential, _ := url.Parse(own.String() + "&loginToken=credential")
		if s.recibirMatrix(other) || !s.recibirMatrix(wrong) || !s.recibirMatrix(credential) || len(flow.arrived) != 0 {
			t.Fatal("foreign or credential-bearing callback delivered", tc.product)
		}
		if !s.recibirMatrix(own) || len(flow.arrived) != 1 {
			t.Fatal("own app return was lost", tc.product)
		}
		callback := <-flow.arrived
		if !start.Accepts(callback) {
			t.Fatal("callback lost original product", tc.product)
		}
		s.matrixConsent = nil
		if !s.recibirMatrix(own) {
			t.Fatal("unsolicited callback was retained for navigation")
		}
	}
}

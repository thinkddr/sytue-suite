// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestMatrixNativeConsumerPinsProductAppAndPKCEAtBothExchanges(t *testing.T) {
	for _, phase := range []string{"valid", "start-missing", "start-foreign", "finish-missing", "finish-foreign"} {
		t.Run(phase, func(t *testing.T) {
			challenge := ""
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input map[string]any
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if input["product"] != "drive" || input["native_consumer"] != "pdf" || input["identity_version"] != float64(storage.IsolatedIdentity) {
					t.Error("native handoff lost the data/app pair")
				}
				consumer := "pdf"
				start := strings.HasSuffix(r.URL.Path, "/start")
				if start && phase == "start-missing" || !start && phase == "finish-missing" {
					consumer = ""
				}
				if start && phase == "start-foreign" || !start && phase == "finish-foreign" {
					consumer = "docs"
				}
				if start {
					challenge, _ = input["code_challenge"].(string)
					_ = json.NewEncoder(w).Encode(map[string]any{"product": "drive", "native_consumer": consumer, "identity_version": storage.IsolatedIdentity, "tenant": "acme", "state": "consumer-state", "authorize_url": "https://rooms.sytue.eu/rooms/matrix/native/start?state=consumer-state"})
				} else {
					verifier, _ := input["code_verifier"].(string)
					digest := sha256.Sum256([]byte(verifier))
					if base64.RawURLEncoding.EncodeToString(digest[:]) != challenge {
						t.Error("consumer exchange lost PKCE")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"login_token": "synthetic-consumer-login-token", "native_consumer": consumer, "identity_version": storage.IsolatedIdentity})
				}
			}))
			defer server.Close()
			client := NewRooms(server.URL, nil, server.Client())
			flow, err := client.MatrixNativeStartForConsumer(t.Context(), "drive", "pdf", storage.IsolatedIdentity)
			if strings.HasPrefix(phase, "start-") {
				if !errors.Is(err, ErrMatrixSSO) {
					t.Fatal("unbound consumer accepted at start", err)
				}
				return
			}
			if err != nil || flow.Product() != "drive" || flow.NativeConsumer() != "pdf" || flow.ReturnScheme() != "sytue-pdf" || !flow.ForConsumer("sytue.eu", "drive", "pdf") || flow.ForConsumer("sytue.eu", "drive", "") {
				t.Fatal("native consumer changed data identity or return app", err)
			}
			uri, _ := url.Parse("sytue-pdf://matrix/callback?state=consumer-state&ticket=consumer-ticket")
			callback, ok := ParseMatrixNativeCallbackForConsumer(uri, "drive", "pdf")
			if !ok || !flow.Accepts(callback) {
				t.Fatal("own consumer return rejected")
			}
			if _, ok := ParseMatrixNativeCallbackFor(uri, "drive"); ok {
				t.Fatal("Drive adopted another native app return")
			}
			foreign := callback
			foreign.nativeConsumer = "docs"
			if flow.Accepts(foreign) {
				t.Fatal("same state crossed consumer binding")
			}
			token, err := client.MatrixNativeFinish(t.Context(), flow, callback)
			if strings.HasPrefix(phase, "finish-") {
				if !errors.Is(err, ErrMatrixSSO) {
					t.Fatal("unbound consumer accepted at finish", err)
				}
				return
			}
			if err != nil || token != "synthetic-consumer-login-token" {
				t.Fatal("exact consumer exchange failed", err)
			}
		})
	}
}

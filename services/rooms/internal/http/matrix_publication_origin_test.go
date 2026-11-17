// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

type originServiceFixture struct {
	calls    int
	last     publication.OriginEvent
	accepted bool
}

func (f *originServiceFixture) BeforeMatrixPublicationOrigin(_ context.Context, e publication.OriginEvent) (publication.OriginDecision, error) {
	f.calls++
	f.last = e
	return publication.OriginDecision{Version: 1, Managed: true, Allow: true}, nil
}

func (f *originServiceFixture) AcceptedMatrixPublicationOrigin(ctx context.Context, e publication.OriginEvent) (publication.OriginDecision, error) {
	f.accepted = true
	return f.BeforeMatrixPublicationOrigin(ctx, e)
}

func TestPublicationOriginHTTPRejectsMissingSecretAndPlaintextBodies(t *testing.T) {
	for _, fault := range []string{"before", "accepted", "missing-auth", "wrong-auth", "empty-config", "plaintext-body", "missing-cipher", "foreign-type", "unknown-field", "oversized"} {
		t.Run(fault, func(t *testing.T) {
			secret := "synthetic-test-secret"
			configured := secret
			if fault == "empty-config" {
				configured = ""
			}
			f := &originServiceFixture{}
			r := chi.NewRouter()
			mountPublicationOrigin(r, configured, f)
			event := publication.OriginEvent{Version: 1, Reference: storage.Reference{RoomID: "!room:matrix.test", EventID: "$event"}, Sender: "@owner:matrix.test", Type: "m.room.encrypted", Content: json.RawMessage(`{"algorithm":"m.megolm.v1.aes-sha2","session_id":"synthetic","ciphertext":"opaque"}`)}
			if fault == "plaintext-body" {
				event.Type = "m.room.message"
				event.Content = json.RawMessage(`{"body":"must-not-reach-service"}`)
			}
			if fault == "missing-cipher" {
				event.Content = nil
			}
			if fault == "foreign-type" {
				event.Type = "m.room.member"
				event.Content = nil
			}
			if fault == "oversized" {
				event.Content = json.RawMessage(`{"ciphertext":"` + strings.Repeat("a", 128<<10) + `"}`)
			}
			raw, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			if fault == "unknown-field" {
				raw = append(raw[:len(raw)-1], []byte(`,"user_token":"forbidden"}`)...)
			}
			suffix := "before"
			if fault == "accepted" {
				suffix = "accepted"
			}
			req := httptest.NewRequest(http.MethodPost, "/internal/matrix/publications/"+suffix, strings.NewReader(string(raw)))
			req.Header.Set("Content-Type", "application/json")
			if fault != "missing-auth" {
				token := secret
				if fault == "wrong-auth" {
					token = "wrong"
				}
				req.Header.Set("Authorization", "Bearer "+token)
			}
			response := httptest.NewRecorder()
			r.ServeHTTP(response, req)
			if fault == "before" || fault == "accepted" {
				if response.Code != http.StatusOK || f.calls != 1 || f.last.Sender != event.Sender || f.accepted != (fault == "accepted") {
					t.Fatal("authenticated callback failed", response.Code, f.calls)
				}
				if response.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("callback cached")
				}
			} else if response.Code < 400 || f.calls != 0 {
				t.Fatal("untrusted callback reached service", response.Code, f.calls)
			}
			if strings.Contains(response.Body.String(), "must-not-reach-service") || strings.Contains(response.Body.String(), secret) {
				t.Fatal("private input reflected")
			}
		})
	}
}

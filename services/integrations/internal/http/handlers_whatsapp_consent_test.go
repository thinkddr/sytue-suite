// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetaConsentOriginIsFixedByServerConfiguration(t *testing.T) {
	h := &Handlers{roomsPublicURL: "https://rooms.sytue.example/rooms"}
	w := httptest.NewRecorder()
	h.WhatsAppConsent(w, httptest.NewRequest(http.MethodGet, "/consent/whatsapp?origin=https://attacker.invalid&state=must-not-render&code=never-render", http.NoBody))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	body := w.Body.String()
	for _, required := range []string{"https://rooms.sytue.example", "https://connect.facebook.net/en_US/sdk.js", "event.source !== opener", "event.origin !== roomsOrigin", "state: config.state", "response_type: 'code'"} {
		if !strings.Contains(body, required) {
			t.Errorf("missing isolation guard %q", required)
		}
	}
	for _, absent := range []string{"attacker.invalid", "must-not-render", "never-render", "localStorage", "sessionStorage", "document.cookie", "access_token"} {
		if strings.Contains(body, absent) {
			t.Errorf("consent document contains %q", absent)
		}
	}
	for key, want := range map[string]string{"Cache-Control": "no-store", "Referrer-Policy": "no-referrer", "Cross-Origin-Opener-Policy": "unsafe-none", "Cross-Origin-Embedder-Policy": "unsafe-none"} {
		if got := w.Header().Get(key); got != want {
			t.Errorf("%s=%q want %q", key, got, want)
		}
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'nonce-") || !strings.Contains(csp, "frame-ancestors 'none'") || strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
		t.Errorf("unexpected CSP %q", csp)
	}
}

func TestMetaConsentRejectsMissingOrUnsafeRoomsOrigin(t *testing.T) {
	for _, origin := range []string{"", "http://rooms.sytue.example", "https://user@rooms.sytue.example", "https://rooms.sytue.example?origin=other", "https://rooms.sytue.example#fragment"} {
		h := &Handlers{roomsPublicURL: origin}
		w := httptest.NewRecorder()
		h.WhatsAppConsent(w, httptest.NewRequest(http.MethodGet, "/consent/whatsapp", http.NoBody))
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("origin=%q status=%d", origin, w.Code)
		}
	}
}

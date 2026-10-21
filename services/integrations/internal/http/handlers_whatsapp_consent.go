// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"crypto/rand"
	_ "embed"
	"html/template"
	"net/http"
	"net/url"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

//go:embed consent/whatsapp.html
var whatsAppConsentHTML string

var whatsAppConsentPage = template.Must(template.New("whatsapp-consent").Parse(whatsAppConsentHTML))

// WhatsAppConsent keeps Meta's SDK off the Rooms origin and its local encryption keys.
// The opener is pinned to server configuration; neither URLs nor query parameters choose it.
func (h *Handlers) WhatsAppConsent(w http.ResponseWriter, r *http.Request) {
	rooms, err := url.Parse(h.roomsPublicURL)
	if err != nil || rooms.Scheme != "https" || rooms.Host == "" || rooms.User != nil || rooms.RawQuery != "" || rooms.Fragment != "" {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	nonce := rand.Text()
	headers := w.Header()
	headers.Set("Content-Type", "text/html; charset=utf-8")
	headers.Set("Cache-Control", "no-store")
	headers.Set("Referrer-Policy", "no-referrer")
	// This one public consent document intentionally keeps its cross-origin opener.
	headers.Set("Cross-Origin-Opener-Policy", "unsafe-none")
	headers.Set("Cross-Origin-Embedder-Policy", "unsafe-none")
	headers.Set("Content-Security-Policy", "default-src 'none'; "+
		"script-src 'nonce-"+nonce+"' https://connect.facebook.net; "+
		"style-src 'nonce-"+nonce+"'; "+
		"connect-src https://graph.facebook.com https://www.facebook.com https://connect.facebook.net; "+
		"frame-src https://www.facebook.com https://web.facebook.com https://facebook.com; "+
		"img-src data: https://www.facebook.com https://facebook.com; "+
		"base-uri 'none'; object-src 'none'; "+
		"form-action https://www.facebook.com https://facebook.com; frame-ancestors 'none'")
	_ = whatsAppConsentPage.Execute(w, struct{ Nonce, RoomsOrigin string }{
		Nonce:       nonce,
		RoomsOrigin: rooms.Scheme + "://" + rooms.Host,
	})
}

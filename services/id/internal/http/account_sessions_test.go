// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	// Sec-Fetch-Site manda cuando viene. Los dos primeros casos son la pareja
	// MEDIDA con Chrome contra una página que sirve «Referrer-Policy: no-referrer»
	// como hace este servicio: el POST propio y el de una página ajena llegan los
	// dos con Origin: null, y solo Sec-Fetch-Site los separa.
	s := &Server{cfg: Config{Issuer: "https://id.sytue.eu", RPOrigins: []string{"https://id.sytue.eu"}}}
	cases := []struct {
		name      string
		origin    string
		fetchSite string
		want      bool
	}{
		{"form propio con no-referrer", "null", "same-origin", true},
		{"página ajena con no-referrer", "null", "cross-site", false},
		{"subdominio hermano (mismo site, cookie Lax incluida)", "null", "same-site", false},
		{"subdominio hermano con su Origin real", "https://cliente.apps.sytue.eu", "same-site", false},
		{"navegación del usuario (barra/marcador)", "", "none", true},
		{"Origin propio y fetch-site propio", "https://id.sytue.eu", "same-origin", true},
		// Sin Sec-Fetch-Site (Safari < 16.4, curl, proxy del BFF): contraste por Origin.
		{"sin fetch-site, Origin propio", "https://id.sytue.eu", "", true},
		{"sin fetch-site, sin Origin", "", "", true},
		{"sin fetch-site, Origin opaco", "null", "", true},
		{"sin fetch-site, subdominio hermano", "https://mail.sytue.eu", "", false},
		{"sin fetch-site, esquema degradado", "http://id.sytue.eu", "", false},
		{"sin fetch-site, sufijo tramposo", "https://id.sytue.eu.evil", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/account/sessions/revoke", http.NoBody)
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			if c.fetchSite != "" {
				r.Header.Set("Sec-Fetch-Site", c.fetchSite)
			}
			if got := s.sameOrigin(r); got != c.want {
				t.Errorf("sameOrigin(Origin=%q, Sec-Fetch-Site=%q) = %v, quiero %v", c.origin, c.fetchSite, got, c.want)
			}
		})
	}
}

func TestDeviceES(t *testing.T) {
	cases := []struct{ ua, want string }{
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36", "Chrome · macOS"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:127.0) Gecko/20100101 Firefox/127.0", "Firefox · Windows"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15", "Safari · macOS"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Edg/126.0.0.0", "Edge · Windows"},
		// Android antes que Linux (el UA de Android contiene "Linux").
		{"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36", "Chrome · Android"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/126.0.0.0 Mobile/15E148 Safari/604.1", "Chrome · iOS"},
		{"", "Dispositivo desconocido"},
		{"SytueE2E-Dispositivo-Victima", "SytueE2E-Dispositivo-Victima"}, // fallback literal
	}
	for _, c := range cases {
		if got := deviceES("es", c.ua); got != c.want {
			t.Errorf("deviceES(%q) = %q, quiero %q", c.ua, got, c.want)
		}
	}
	// Fallback largo: truncado con elipsis, nunca el UA entero.
	long := deviceES("es", "x-"+string(make([]byte, 100)))
	if len(long) > 60 {
		t.Errorf("fallback sin truncar: %d bytes", len(long))
	}
}

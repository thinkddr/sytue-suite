// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package subscriptions

import (
	"context"
	"strings"
	"testing"
)

// TestValidateURLRejectsInternalAndBadScheme es la BARRERA 1 (pre-petición): IPs
// literales internas y esquemas no http/https se rechazan; una IP pública literal
// pasa (sin DNS). localhost se rechaza por RESOLUCIÓN a loopback.
func TestValidateURLRejectsInternalAndBadScheme(t *testing.T) {
	ctx := context.Background()
	reject := []string{
		"http://127.0.0.1/holidays.ics",
		"http://10.1.2.3/team.ics",
		"http://169.254.169.254/latest/meta-data/", // metadata cloud
		"http://[::1]/x.ics",
		"https://[fd00::1]/x.ics",
		"http://100.64.0.1/x.ics",
		"ftp://example.com/x.ics",   // esquema no permitido
		"file:///etc/passwd",        // nunca file://
		"javascript:alert(1)",       // esquema peligroso
		"http://user:pw@host/x",     // credenciales embebidas
		"http://localhost/team.ics", // resuelve a loopback
		"",
	}
	for _, u := range reject {
		if err := validateURL(ctx, u); err == nil {
			t.Errorf("validateURL(%q) = nil, want error (debe rechazarse)", u)
		}
	}
	// IP pública literal: pasa la barrera 1 sin necesitar DNS.
	if err := validateURL(ctx, "http://1.1.1.1/holidays.ics"); err != nil {
		t.Errorf("validateURL(IP pública literal) = %v, want nil", err)
	}
}

// TestDialerBlocksInternal es la BARRERA 2 (en el connect, cierra el TOCTOU): el
// dialer re-resuelve y rechaza el destino ANTES de conectar. Cubre IP literal y
// resolución (localhost). Nunca llega a abrir la conexión al destino interno.
func TestDialerBlocksInternal(t *testing.T) {
	tr := ssrfSafeTransport()
	ctx := context.Background()
	for _, addr := range []string{"127.0.0.1:80", "169.254.169.254:80", "10.0.0.1:443", "localhost:80"} {
		conn, err := tr.DialContext(ctx, "tcp", addr)
		if err == nil {
			conn.Close()
			t.Errorf("DialContext(%q) conectó, want error SSRF", addr)
			continue
		}
		if !strings.Contains(err.Error(), "no permitido") {
			t.Errorf("DialContext(%q) err = %v, want «destino no permitido»", addr, err)
		}
	}
}

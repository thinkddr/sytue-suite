// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import "testing"

// TestSafeDialControl ejercita la última barrera anti-SSRF del emisor de
// webhooks: corre tras resolver DNS, con la IP concreta del destino, y debe
// rechazar destinos internos (loopback/RFC1918/link-local/metadata/unspecified)
// y aceptar los públicos. Cierra el rebinding de un hostname público a una IP
// interna que la validación por host no puede ver.
func TestSafeDialControl(t *testing.T) {
	blocked := []string{
		"127.0.0.1:443",      // loopback
		"169.254.169.254:80", // metadata cloud (link-local)
		"10.0.0.5:443",       // privada RFC1918
		"192.168.1.1:80",     // privada
		"[::1]:443",          // loopback IPv6
		"0.0.0.0:80",         // unspecified
		"100.64.5.7:443",     // CGNAT RFC6598: la CIDR de pods del clúster
		"100.127.255.254:80", // CGNAT, límite superior del /10
	}
	for _, addr := range blocked {
		if err := safeDialControl("tcp", addr, nil); err == nil {
			t.Errorf("safeDialControl(%q) = nil, want error (SSRF bloqueado)", addr)
		}
	}
	allowed := []string{
		"93.184.216.34:443", // pública
		"1.1.1.1:443",
	}
	for _, addr := range allowed {
		if err := safeDialControl("tcp", addr, nil); err != nil {
			t.Errorf("safeDialControl(%q) = %v, want nil", addr, err)
		}
	}
}

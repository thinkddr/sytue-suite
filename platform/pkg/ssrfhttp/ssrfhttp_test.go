// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ssrfhttp

import (
	"context"
	"errors"
	"net"
	"testing"
)

// TestIsBlockedIP cubre el denylist SSRF: loopback, RFC1918, link-local/metadata,
// CGNAT, unspecified, ULA IPv6 y ::1 se bloquean; una IP pública se permite.
func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1",       // loopback
		"10.0.0.1",        // RFC1918
		"10.255.1.2",      // RFC1918
		"172.16.5.4",      // RFC1918
		"192.168.1.1",     // RFC1918
		"169.254.169.254", // link-local (metadata cloud)
		"100.64.0.1",      // CGNAT RFC6598
		"100.127.255.1",   // CGNAT borde superior
		"0.0.0.0",         // unspecified
		"::1",             // loopback IPv6
		"fc00::1",         // ULA IPv6
		"fd00::1",         // ULA IPv6, mitad alta de fc00::/7
		"fe80::1",         // link-local IPv6
		"ff02::1",         // multicast IPv6
		"::",              // unspecified IPv6
		"224.0.0.1",       // multicast IPv4 link-local
		// Multicast fuera del rango link-local: es lo único que cubre la rama
		// IsMulticast() de la función, y sin estos dos casos quitarla no rompía
		// ningún test de la suite.
		"239.255.255.250", // multicast IPv4 administrativo (SSDP)
		"ff05::1",         // multicast IPv6 site-local
	}
	for _, s := range blocked {
		if !IsBlockedIP(net.ParseIP(s)) {
			t.Errorf("IsBlockedIP(%s) = false; se esperaba bloqueada", s)
		}
	}
	allowed := []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "203.0.113.10", "2606:2800:220:1::1", "2606:4700:4700::1111"}
	for _, s := range allowed {
		if IsBlockedIP(net.ParseIP(s)) {
			t.Errorf("IsBlockedIP(%s) = true; se esperaba permitida", s)
		}
	}
	if !IsBlockedIP(nil) {
		t.Error("IsBlockedIP(nil) debe ser true (sin IP usable)")
	}
}

// TestValidateHostLiteralIP valida hosts que son literales IP (sin DNS): las
// privadas/loopback/link-local se rechazan con ErrBlocked; una pública pasa.
func TestValidateHostLiteralIP(t *testing.T) {
	ctx := context.Background()
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "::1", "fc00::1"} {
		if err := ValidateHost(ctx, s); !errors.Is(err, ErrBlocked) {
			t.Errorf("ValidateHost(%s) = %v; se esperaba ErrBlocked", s, err)
		}
	}
	if err := ValidateHost(ctx, "8.8.8.8"); err != nil {
		t.Errorf("ValidateHost(8.8.8.8) = %v; se esperaba nil", err)
	}
	if err := ValidateHost(ctx, ""); !errors.Is(err, ErrInvalidURL) {
		t.Errorf("ValidateHost(\"\") = %v; se esperaba ErrInvalidURL", err)
	}
}

// TestValidateURLScheme rechaza esquemas no http/https (file, gopher, etc.) y
// URLs con IP privada literal; acepta http(s) hacia una IP pública literal.
func TestValidateURLScheme(t *testing.T) {
	ctx := context.Background()
	for _, s := range []string{
		"file:///etc/passwd",
		"gopher://127.0.0.1:70/",
		"ftp://example.com/",
		"http://127.0.0.1/admin",
		"https://10.0.0.5/internal",
		"http://169.254.169.254/latest/meta-data/",
	} {
		if err := ValidateURL(ctx, s); err == nil {
			t.Errorf("ValidateURL(%s) = nil; se esperaba error", s)
		}
	}
	if err := ValidateURL(ctx, "https://8.8.8.8/path?q=1"); err != nil {
		t.Errorf("ValidateURL(https://8.8.8.8) = %v; se esperaba nil", err)
	}
}

// TestSafeDialContextBlocksLiteral comprueba que el dialer (barrera 2) rechaza un
// literal IP interno sin abrir socket.
func TestSafeDialContextBlocksLiteral(t *testing.T) {
	_, err := SafeDialContext(context.Background(), "tcp", net.JoinHostPort("127.0.0.1", "5432"))
	if !errors.Is(err, ErrBlocked) {
		t.Errorf("SafeDialContext(127.0.0.1) = %v; se esperaba ErrBlocked", err)
	}
}

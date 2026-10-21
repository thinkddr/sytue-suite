// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

func TestSanitizeDisplayName(t *testing.T) {
	// El vector XSS del hallazgo 7: un nombre con </script> no debe conservar
	// caracteres de control; los ángulos se dejan al escape del punto de salida,
	// pero los separadores de línea y controles se eliminan aquí.
	cases := []struct {
		in   string
		want string
	}{
		{"  Ada Lovelace  ", "Ada Lovelace"},
		{"a\u2028b", "ab"},       // separador de línea U+2028
		{"a\u2029b", "ab"},       // separador de párrafo U+2029
		{"a\x00b\tc\nd", "abcd"}, // NUL, TAB, LF (controles)
	}
	for _, c := range cases {
		if got := SanitizeDisplayName(c.in); got != c.want {
			t.Errorf("SanitizeDisplayName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// Los separadores de línea nunca deben sobrevivir (romperían <script>).
	if got := SanitizeDisplayName("x\u2028\u2029y"); strings.ContainsAny(got, "\u2028\u2029") {
		t.Errorf("SanitizeDisplayName dejó separadores de línea: %q", got)
	}
	// Límite de longitud.
	if got := SanitizeDisplayName(strings.Repeat("a", 300)); len(got) != 256 {
		t.Errorf("SanitizeDisplayName no truncó a 256: len=%d", len(got))
	}
}

func TestClientValidRedirectURI_ExactMatch(t *testing.T) {
	c := Client{RedirectURIs: []string{
		"https://one.sytue.eu/auth/callback",
		"http://localhost:4321/auth/callback",
	}}
	cases := []struct {
		uri  string
		want bool
	}{
		{"https://one.sytue.eu/auth/callback", true},
		{"http://localhost:4321/auth/callback", true},
		// Sin comodines ni prefijos: variaciones deben fallar (anti open-redirect).
		{"https://one.sytue.eu/auth/callback/", false},
		{"https://one.sytue.eu/auth/callback?x=1", false},
		{"https://one.sytue.eu.evil.com/auth/callback", false},
		{"https://evil.com/auth/callback", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := c.ValidRedirectURI(tc.uri); got != tc.want {
			t.Errorf("ValidRedirectURI(%q) = %v, quería %v", tc.uri, got, tc.want)
		}
	}
}

func TestClientIsPublic(t *testing.T) {
	if !(Client{Type: ClientPublic}).IsPublic() {
		t.Error("un client tipo public debe ser público")
	}
	if !(Client{TokenEndpointAuthMethod: "none"}).IsPublic() {
		t.Error("token_endpoint_auth_method=none implica público")
	}
	if (Client{Type: ClientConfidential, TokenEndpointAuthMethod: "client_secret_basic"}).IsPublic() {
		t.Error("un confidencial con secreto no es público")
	}
}

func TestRoleRequiresMFA(t *testing.T) {
	if !RoleOwner.RequiresMFA() || !RoleAdmin.RequiresMFA() {
		t.Error("owner/admin deben requerir MFA")
	}
	if RoleMember.RequiresMFA() || RoleBilling.RequiresMFA() {
		t.Error("member/billing no requieren MFA")
	}
}

// El cliente aplica «el de cada app» si no hay claim. Petróleo necesita viajar
// explícito para conservar la elección de una organización tras cambiar el defecto.
func TestColorClaimConservaLasEleccionesExplicitas(t *testing.T) {
	for _, c := range []struct{ color, claim string }{
		{"", ""},
		{"app", ""},
		{"petroleo", "petroleo"},
		{"cobalto", "cobalto"},
	} {
		if got := (Organization{Color: c.color}).ColorClaim(); got != c.claim {
			t.Errorf("color %q: claim %q, quiero %q", c.color, got, c.claim)
		}
	}
}

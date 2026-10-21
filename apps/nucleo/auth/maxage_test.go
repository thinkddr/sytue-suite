// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"net/url"
	"testing"
)

// TestElLoginLlevaMaxAgeSoloSiSePide: el bloqueo sin verificador pide la contraseña
// de Sytue con `max_age`; el login de siempre no lo manda.
func TestElLoginLlevaMaxAgeSoloSiSePide(t *testing.T) {
	o := &OIDC{Issuer: "https://id.example"}
	o.endpoints.Authorization = "https://id.example/oauth2/authorize"
	destino, _, _ := o.autorizar("http://127.0.0.1:1/callback", "")
	u, _ := url.Parse(destino)
	if u.Query().Has("max_age") {
		t.Fatalf("sin pedirlo no va max_age: %s", destino)
	}
	o.MaxAge = 60
	destino, _, _ = o.autorizar("http://127.0.0.1:1/callback", "")
	u, _ = url.Parse(destino)
	if u.Query().Get("max_age") != "60" {
		t.Fatalf("con el bloqueo, max_age=60: %s", destino)
	}
}

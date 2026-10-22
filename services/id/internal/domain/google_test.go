// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// Una librería de Google pide la identidad con sus nombres largos: tienen que
// llegar al authorize como los de OpenID, sin duplicados y sin tocar el resto.
func TestNormalizeGoogleScopes(t *testing.T) {
	got := NormalizeGoogleScopes("openid https://www.googleapis.com/auth/userinfo.email email " +
		"https://www.googleapis.com/auth/userinfo.profile https://www.googleapis.com/auth/calendar.readonly")
	if want := "openid email profile https://www.googleapis.com/auth/calendar.readonly"; got != want {
		t.Fatalf("NormalizeGoogleScopes = %q, quiero %q", got, want)
	}
	if !HasGoogleAPIScope("openid https://www.googleapis.com/auth/drive") {
		t.Error("drive es un scope de API de Google")
	}
	// La identidad no abre ninguna API: un token solo con ella no lleva la audiencia.
	if HasGoogleAPIScope("openid email https://www.googleapis.com/auth/userinfo.email") {
		t.Error("los scopes de identidad no son de API")
	}
}

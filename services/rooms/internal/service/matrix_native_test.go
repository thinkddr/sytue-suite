// SPDX-License-Identifier: Apache-2.0
package service

import (
	"net/url"
	"testing"

	"git.sytue.eu/sytue/one/services/rooms/internal/config"
)

func TestMatrixNativePKCEVectorAndFixedPublicRoutes(t *testing.T) {
	challenge, err := matrixPKCEChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	if err != nil || challenge != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatalf("RFC7636 vector %q %v", challenge, err)
	}
	if !matrixOpaque(challenge) || matrixOpaque(challenge+"=") {
		t.Fatal("S256 challenge canonical encoding not enforced")
	}
	for _, verifier := range []string{"short", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/"} {
		if _, err := matrixPKCEChallenge(verifier); err == nil {
			t.Fatal("invalid verifier accepted")
		}
	}
	s := &Service{cfg: config.Config{PublicURL: "https://rooms.sytue.eu/ignored"}}
	target, err := s.matrixNativePublicURL("callback", challenge)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != "rooms.sytue.eu" || parsed.Path != "/rooms/matrix/native/callback" || parsed.Query().Get("state") != challenge {
		t.Fatalf("unsafe callback %s", target)
	}
	s.cfg.PublicURL = "https://rooms.sytue.eu/?return_to=https://attacker.example"
	if _, err := s.matrixNativePublicURL("start", challenge); err == nil {
		t.Fatal("configured URL query accepted")
	}
}

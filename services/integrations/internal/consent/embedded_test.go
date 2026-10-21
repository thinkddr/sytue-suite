// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package consent_test

import (
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
)

func TestEmbeddedSignupConservaSesionNonceYVencimiento(t *testing.T) {
	c := cipher(t)
	now := time.Now()
	state, nonce, err := consent.BeginEmbedded(c, "acme", "ana", "instancia", "rooms-whatsapp", now)
	if err != nil {
		t.Fatal(err)
	}
	rd, err := consent.Callback(c, state, "codigo-meta", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if rd.Tenant != "acme" || rd.Sub != "ana" || rd.Instance != "instancia" || rd.Section != "rooms-whatsapp" || rd.Challenge() != nonce || rd.RedirectURI != "" {
		t.Fatalf("perdió la vinculación: %+v", rd)
	}
	if _, err := consent.Callback(c, state, "codigo-meta", "", now.Add(11*time.Minute)); !errors.Is(err, consent.ErrState) {
		t.Fatalf("se aceptó un state caducado: %v", err)
	}
	if _, _, err := consent.BeginEmbedded(c, "acme", "", "instancia", "rooms-whatsapp", now); err == nil {
		t.Fatal("no puede empezar sin persona")
	}
}

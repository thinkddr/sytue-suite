// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// El buzón de una cuenta personal cuelga del dominio padre, no de un subdominio
// con su hash: es la única dirección que una persona puede dictar por teléfono.
func TestElBuzonPersonalCuelgaDelDominioPadre(t *testing.T) {
	if got := OrgBaseMailDomain(PersonalSlug("carla@ejemplo.com"), "Sytue.eu"); got != "sytue.eu" {
		t.Fatalf("personal: %q", got)
	}
	if got := OrgBaseMailDomain("acme", "sytue.eu"); got != "acme.sytue.eu" {
		t.Fatalf("organización: %q", got)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import "testing"

// TestElNASNoNaceEnElDominioDeLaSuite (FILE-09, NEG-24): sin dominio de contenido
// de clientes y su id en el proxy no hay túnel —antes caía en sytue.eu, el de la
// suite—, y con ellos los nombres nacen allí.
func TestElNASNoNaceEnElDominioDeLaSuite(t *testing.T) {
	for _, c := range []struct{ id, dominio string }{{"", ""}, {"domain2", ""}, {"", "clientes.example"}} {
		if newTunnelClient("https://proxy.example/v1", "llave", "org", c.id, c.dominio, "https://proxy.example") != nil {
			t.Errorf("id %q, dominio %q: hay túnel", c.id, c.dominio)
		}
	}
	tc := newTunnelClient("https://proxy.example/v1", "llave", "org", "domain2", "Clientes.Example", "https://proxy.example")
	if tc == nil || tc.domain != "clientes.example" || tc.domainID != "domain2" {
		t.Fatalf("con dominio de clientes: %+v", tc)
	}
}

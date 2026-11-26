// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import "testing"

// Una entrada de auditoría con el slug de la organización (lo que trae `c.Tenant`) no
// puede llegar al INSERT como uuid: hacía fallar la fila entera y la acción no quedaba
// registrada. El id va como id; el slug, a resolver; vacío, sin organización.
func TestOrgDeAuditoriaSeparaIDYSlug(t *testing.T) {
	casos := []struct{ org, id, slug string }{
		{"", "", ""},
		{"0b7f3c2e-1d4a-4c5b-9e8f-7a6b5c4d3e2f", "0b7f3c2e-1d4a-4c5b-9e8f-7a6b5c4d3e2f", ""},
		{"0B7F3C2E-1D4A-4C5B-9E8F-7A6B5C4D3E2F", "0B7F3C2E-1D4A-4C5B-9E8F-7A6B5C4D3E2F", ""},
		{"acme", "", "acme"},
		{"p-e965a545e7a16e53", "", "p-e965a545e7a16e53"},
		{"{0b7f3c2e-1d4a-4c5b-9e8f-7a6b5c4d3e2f}", "", "{0b7f3c2e-1d4a-4c5b-9e8f-7a6b5c4d3e2f}"},
	}
	for _, c := range casos {
		if id, slug := orgDeAuditoria(c.org); id != c.id || slug != c.slug {
			t.Errorf("orgDeAuditoria(%q) = (%q, %q), quiero (%q, %q)", c.org, id, slug, c.id, c.slug)
		}
	}
}

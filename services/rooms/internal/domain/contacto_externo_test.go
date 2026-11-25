// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

func TestPoliticaContactoExternoAdmite(t *testing.T) {
	abierta := PoliticaContactoExterno{}
	cerrada := PoliticaContactoExterno{Modo: ContactoCerrado}
	confianza := PoliticaContactoExterno{Modo: ContactoConfianza, Confianza: []string{"otra"}}
	for _, c := range []struct {
		p      PoliticaContactoExterno
		org    string
		orgs   []string
		uno    bool
		varias bool
	}{
		{abierta, "otra", nil, true, true},
		{cerrada, "otra", []string{"otra"}, false, false},
		{confianza, "otra", []string{"tercera", "otra"}, true, true},
		{confianza, "tercera", []string{"tercera"}, false, false},
		// Sin organizaciones conocidas (solo cuenta personal): la de confianza no la admite.
		{confianza, "", nil, false, false},
	} {
		if got := c.p.Admite(c.org); got != c.uno {
			t.Errorf("%+v.Admite(%q) = %v", c.p, c.org, got)
		}
		if got := c.p.AdmiteAlguna(c.orgs); got != c.varias {
			t.Errorf("%+v.AdmiteAlguna(%v) = %v", c.p, c.orgs, got)
		}
	}
}

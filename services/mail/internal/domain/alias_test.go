// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"testing"
	"time"
)

func TestVacationActiveAt(t *testing.T) {
	start := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	before := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	after := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		v    Vacation
		at   time.Time
		want bool
	}{
		{"desactivada", Vacation{Enabled: false, StartsAt: &start, EndsAt: &end}, mid, false},
		{"en rango", Vacation{Enabled: true, StartsAt: &start, EndsAt: &end}, mid, true},
		{"antes del inicio", Vacation{Enabled: true, StartsAt: &start, EndsAt: &end}, before, false},
		{"tras el fin", Vacation{Enabled: true, StartsAt: &start, EndsAt: &end}, after, false},
		{"sin límites (siempre)", Vacation{Enabled: true}, mid, true},
		{"solo inicio, después", Vacation{Enabled: true, StartsAt: &start}, after, true},
		{"solo inicio, antes", Vacation{Enabled: true, StartsAt: &start}, before, false},
		{"solo fin, antes", Vacation{Enabled: true, EndsAt: &end}, before, true},
		{"solo fin, después", Vacation{Enabled: true, EndsAt: &end}, after, false},
	}
	for _, c := range cases {
		if got := c.v.ActiveAt(c.at); got != c.want {
			t.Errorf("%s: ActiveAt = %v, quiero %v", c.name, got, c.want)
		}
	}
}

func TestAliasDomainAllowed(t *testing.T) {
	cases := []struct {
		name          string
		alias         string
		primary       string
		tenant        string
		primaryDomain string
		rootTenant    string
		orgDomains    []string
		want          bool
	}{
		// Sigue valiendo, y es deliberado: si alguien YA tiene su dirección primaria
		// en el raíz, puede hacerse alias ahí. Esa dirección solo la reparte quien
		// administra la suite, así que el privilegio ya estaba concedido antes; lo
		// que se cierra abajo es pedirlo por las bravas desde otra organización.
		{"mismo dominio que la primaria", "ventas@sytue.eu", "ada@sytue.eu", "acme", "sytue.eu", "corporate", nil, true},
		// Antes esto era `true` para cualquiera: el dominio raíz se repartía por
		// orden de llegada y `acme` podía quedarse con `security@sytue.eu`.
		{"raíz desde una organización cualquiera", "ada@sytue.eu", "ada@acme.eu", "acme", "sytue.eu", "corporate", nil, false},
		{"raíz desde la organización institucional", "security@sytue.eu", "ada@corporate.sytue.eu", "corporate", "sytue.eu", "corporate", nil, true},
		{"raíz sin institucional configurado: de nadie", "ada@sytue.eu", "ada@acme.eu", "acme", "sytue.eu", "", nil, false},
		{"la institucional sigue teniendo su subdominio", "ada@corporate.sytue.eu", "ada@corporate.sytue.eu", "corporate", "sytue.eu", "corporate", nil, true},
		{"dominio de la propia cuenta", "otro@acme.eu", "ada@acme.eu", "acme", "sytue.eu", "corporate", nil, true},
		{"dominio ajeno", "yo@gmail.com", "ada@sytue.eu", "acme", "sytue.eu", "corporate", nil, false},
		{"case-insensitive", "Ventas@SYTUE.eu", "ada@sytue.eu", "acme", "sytue.eu", "corporate", nil, true},
		{"dirección inválida", "no-arroba", "ada@sytue.eu", "acme", "sytue.eu", "corporate", nil, false},
		// El caso que dejaba al reconciliador de buzones reintentando un 400 para
		// siempre: la dirección del subdominio de la organización, que Sytue ID
		// reparte también cuando la principal vive en el dominio propio del cliente.
		{"subdominio de la organización", "ada@acme.sytue.eu", "ada@acme.eu", "acme", "sytue.eu", "corporate", nil, true},
		{"subdominio de OTRA organización", "ada@otra.sytue.eu", "ada@acme.eu", "acme", "sytue.eu", "corporate", nil, false},
		{"subdominio sin tenant en la cuenta", "ada@acme.sytue.eu", "ada@acme.eu", "", "sytue.eu", "corporate", nil, false},
		// El SEGUNDO dominio de la organización (mig 0052): no se puede deducir de
		// la dirección de nadie, llega en el token y sin él una empresa con dos
		// marcas no podía tener una dirección en la segunda.
		{
			"otro dominio de la organización", "ada@acme.es", "ada@acme.eu", "acme", "sytue.eu",
			"corporate",
			[]string{"acme.eu", "acme.es"},
			true,
		},
		{
			"dominio de otra organización aunque venga en la lista vacía", "ada@otra.com", "ada@acme.eu",
			"acme", "sytue.eu",
			"corporate",
			[]string{"acme.eu", "acme.es"},
			false,
		},
		{
			"la lista también es case-insensitive", "ADA@Acme.ES", "ada@acme.eu", "acme", "sytue.eu",
			"corporate",
			[]string{" ACME.es "},
			true,
		},
	}
	for _, c := range cases {
		acc := Account{Address: c.primary, Tenant: c.tenant}
		if got := AliasDomainAllowed(c.alias, acc, c.primaryDomain, c.rootTenant, c.orgDomains); got != c.want {
			t.Errorf("%s: AliasDomainAllowed(%q,%q/%q,%q,%v) = %v, quiero %v",
				c.name, c.alias, c.primary, c.tenant, c.primaryDomain, c.orgDomains, got, c.want)
		}
	}
}

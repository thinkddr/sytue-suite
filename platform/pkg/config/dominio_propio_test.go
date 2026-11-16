// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config

import (
	"strings"
	"testing"
)

func TestDominioPropioSustituyeSytueEnProduccion(t *testing.T) {
	t.Setenv(BaseDomainEnvVar, "Suite.MiEmpresa.eu")
	web := FirstPartyWebOrigins(EnvProd)
	for _, o := range []string{"https://id.suite.miempresa.eu", "https://one.suite.miempresa.eu", "https://mail.suite.miempresa.eu"} {
		if !strings.Contains(web, o) {
			t.Errorf("falta %s en %s", o, web)
		}
	}
	if strings.Contains(web, "sytue.eu") {
		t.Errorf("en prod con dominio propio no deben quedar orígenes de sytue.eu: %s", web)
	}
	if cors := FirstPartyCORSOrigins(EnvProd); !strings.HasPrefix(cors, "https://*.suite.miempresa.eu,") {
		t.Errorf("CORS sin el comodín del dominio propio: %s", cors)
	}
	if op := SytueOperatedWebOrigins(EnvProd); strings.Contains(op, "sytue.eu") || !strings.Contains(op, "https://platform.suite.miempresa.eu") {
		t.Errorf("orígenes operados mal derivados: %s", op)
	}
}

func TestSinDominioPropioLasListasNoCambian(t *testing.T) {
	t.Setenv(BaseDomainEnvVar, "")
	if FirstPartyWebOrigins(EnvProd) != firstPartyWebOriginsProdCSV {
		t.Fatal("sin SYTUE_BASE_DOMAIN la lista de producción debe ser la de siempre")
	}
}

func TestDominioPropioRechazaValoresQueAbririanLaCSP(t *testing.T) {
	for _, malo := range []string{"*.evil.eu", "https://x.eu", "x.eu/ruta", "localhost", ".x.eu", "x.eu.", "a b.eu", "x.eu,https://evil.eu"} {
		t.Setenv(BaseDomainEnvVar, malo)
		if dominioPropio() != "" {
			t.Errorf("%q no debería aceptarse como dominio", malo)
		}
		if FirstPartyWebOrigins(EnvProd) != firstPartyWebOriginsProdCSV {
			t.Errorf("con %q inválido la lista debe quedar como siempre", malo)
		}
	}
}

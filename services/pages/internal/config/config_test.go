// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config

import (
	"encoding/base64"
	"strings"
	"testing"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
)

// entorno deja lo obligatorio de un entorno y vacía lo que el test decide.
func entorno(t *testing.T, env string) {
	t.Helper()
	t.Setenv("ENV", env)
	t.Setenv("DATABASE_URL", "postgres://x@127.0.0.1/pages")
	t.Setenv("S3_ACCESS_KEY", "a")
	t.Setenv("S3_SECRET_KEY", "b")
	for _, v := range []string{
		"PORT", "HTTP_ADDR", "HUB_BASE_URL", "PAGES_IP_APEX", "SECRET_ENC_KEY",
		"PAGES_PUBLICADOR_SECRET", "ID_ESTADO_ORGS_SECRET", "AI_PAGES_SECRET",
		"SYERP_PEDIDOS_SECRET", "ANALYTICS_PAGES_SECRET",
	} {
		t.Setenv(v, "")
	}
}

// En dev arranca sin configurar nada más, con los secretos de desarrollo que
// también importa el otro extremo de cada puerta.
func TestDesarrolloArrancaConLosValoresDeDesarrollo(t *testing.T) {
	entorno(t, "dev")
	cfg, err := Cargar()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8100" || cfg.PublicadorSecret != pcfg.DevPagesPublicadorSecret ||
		cfg.IDEstadoOrgsSecret != pcfg.DevIDEstadoOrgsSecret || cfg.IPApex != "127.0.0.1" ||
		cfg.SecretEncKey != nil || cfg.SitesDomain != "sites.sytue.localhost" {
		t.Fatalf("configuración de dev inesperada: %+v", cfg)
	}
}

// Fuera de dev, sin publicador, sin estado de organizaciones o sin KEK no arranca;
// los demás secretos vacíos solo apagan su función.
func TestFueraDeDesarrolloExigeLoImprescindible(t *testing.T) {
	entorno(t, "prod")
	_, err := Cargar()
	for _, v := range []string{"PAGES_PUBLICADOR_SECRET", "ID_ESTADO_ORGS_SECRET", "SECRET_ENC_KEY"} {
		if err == nil || !strings.Contains(err.Error(), v) {
			t.Errorf("Cargar = %v; quiero el error de %s", err, v)
		}
	}

	t.Setenv("PAGES_PUBLICADOR_SECRET", "p")
	t.Setenv("ID_ESTADO_ORGS_SECRET", "e")
	t.Setenv("SECRET_ENC_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	cfg, err := Cargar()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AISecret != "" || cfg.IPApex != "" || len(cfg.SecretEncKey) != 32 {
		t.Fatalf("fuera de dev, lo opcional vacío se queda vacío: %+v", cfg)
	}

	// El secreto de desarrollo es público: fuera de dev se rechaza.
	t.Setenv("AI_PAGES_SECRET", pcfg.DevAIPagesSecret)
	if _, err := Cargar(); err == nil || !strings.Contains(err.Error(), "AI_PAGES_SECRET") {
		t.Fatalf("Cargar = %v; quiero el rechazo del secreto de desarrollo", err)
	}
}

// La IP del ápex se enseña a clientes para su registro A: fuera de dev, solo una
// IPv4 pública.
func TestIPDelApex(t *testing.T) {
	for _, c := range []struct {
		v, env string
		ok     bool
	}{
		{"51.158.110.232", "prod", true},
		{"", "prod", true},
		{"10.0.0.1", "prod", false},
		{"127.0.0.1", "staging", false},
		{"2001:db8::1", "prod", false},
		{"no-es-ip", "prod", false},
		{"127.0.0.1", "dev", true},
	} {
		_, err := ipDelApex(c.v, pcfg.Environment(c.env))
		if (err == nil) != c.ok {
			t.Errorf("ipDelApex(%q, %s) = %v", c.v, c.env, err)
		}
	}
}

// El servidor de las webs no pide base, OIDC ni secretos.
func TestSitiosNoPideBaseNiSecretos(t *testing.T) {
	entorno(t, "prod")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PORT", "9000")
	s, err := CargarSitios()
	if err != nil {
		t.Fatal(err)
	}
	if s.HTTPAddr != ":9000" || s.S3.Bucket != "sytue-dev-pages-sitios" {
		t.Fatalf("sitios: %+v", s)
	}
}

// WEB-09. Con el dominio de contenido de clientes, las webs salen del dominio registrable
// de la suite: `sites.<clientes>`, en la API y en el servidor de las webs. La dirección de
// hoy queda como la anterior, que solo redirige.
func TestLasWebsVanAlDominioDeClientes(t *testing.T) {
	entorno(t, "dev")
	t.Setenv("PAGES_SITES_DOMAIN", "sites.sytue.eu")
	t.Setenv("SYTUE_CLIENT_CONTENT_DOMAIN", "")
	cfg, err := Cargar()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SitesDomain != "sites.sytue.eu" || cfg.SitesDomainAnterior != "" || cfg.DominioDeClientes != "" {
		t.Fatalf("sin dominio de clientes: %q, anterior %q, clientes %q", cfg.SitesDomain, cfg.SitesDomainAnterior, cfg.DominioDeClientes)
	}

	t.Setenv("SYTUE_CLIENT_CONTENT_DOMAIN", " Clientes.Example. ")
	cfg, err = Cargar()
	if err != nil {
		t.Fatal(err)
	}
	sitios, err := CargarSitios()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []Sitios{cfg.Sitios, sitios} {
		if s.SitesDomain != "sites.clientes.example" || s.SitesDomainAnterior != "sites.sytue.eu" || s.DominioDeClientes != "clientes.example" {
			t.Fatalf("con dominio de clientes: %q, anterior %q, clientes %q", s.SitesDomain, s.SitesDomainAnterior, s.DominioDeClientes)
		}
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build tunelreal

package service

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Prueba el cliente del túnel CONTRA EL PROXY REAL: crea un sitio, un recurso
// sin SSO y un destino, comprueba el sitio y lo borra todo. Es la única forma
// de saber que las formas de las peticiones siguen siendo las del proxy
// (su esquema es estricto: un campo de más es un 400).
//
//	TUNNEL_API_URL=https://pangolin-api.sytue.eu/v1 TUNNEL_API_KEY_FILE=~/.config/sytue/pangolin-api-key \
//	  go test -tags tunelreal -run TestTunelContraElProxyReal ./services/drive/internal/service/
func TestTunelContraElProxyReal(t *testing.T) {
	base := os.Getenv("TUNNEL_API_URL")
	raw, err := os.ReadFile(os.Getenv("TUNNEL_API_KEY_FILE"))
	if base == "" || err != nil {
		t.Skip("sin TUNNEL_API_URL o TUNNEL_API_KEY_FILE")
	}
	c := newTunnelClient(base, strings.TrimSpace(string(raw)), "sytue-projects", "domain1", "sytue.eu", "https://pangolin.sytue.eu")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	nombre := "nas-prueba-conector"
	siteID, newtID, secret, err := c.crearSitio(ctx, nombre)
	if err != nil {
		t.Fatalf("crear sitio: %v", err)
	}
	resourceID := 0
	defer func() {
		if err := c.borrar(context.Background(), siteID, resourceID); err != nil {
			t.Errorf("limpiar sitio %d / recurso %d: %v", siteID, resourceID, err)
		}
	}()
	if newtID == "" || secret == "" {
		t.Fatal("el proxy no devolvió credenciales del agente")
	}
	resourceID, err = c.crearRecurso(ctx, siteID, nombre, "192.168.1.250", 5005, "http")
	if err != nil {
		t.Fatalf("crear recurso: %v", err)
	}
	enLinea, err := c.sitioEnLinea(ctx, siteID)
	if err != nil || enLinea {
		t.Fatalf("un sitio sin agente no está en línea: online=%v err=%v", enLinea, err)
	}
	if !strings.Contains(c.comandoAgente(newtID, secret), "--endpoint https://pangolin.sytue.eu") {
		t.Fatal("el comando del agente no apunta al proxy")
	}
}

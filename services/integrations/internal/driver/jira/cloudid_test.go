// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jira

// Test INTERNO (no jira_test) de la elección del cloudid: la raíz 3LO vive en
// api.atlassian.com y no se puede levantar en un httptest, pero la parte que de verdad
// importa es a QUÉ sitio se apunta. Un token de consentimiento puede abarcar varios
// sitios de Atlassian, así que quedarse con el primero leería incidencias de otra
// organización sin que nadie lo notara.

import (
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

func TestPickCloudID(t *testing.T) {
	t.Parallel()
	res := []accessibleResource{
		{ID: "otro-sitio", URL: "https://otra-empresa.atlassian.net"},
		{ID: "cloud-123", URL: "https://acme.atlassian.net/"}, // barra final: no debe estorbar
	}

	got, err := pickCloudID(res, "https://acme.atlassian.net")
	if err != nil {
		t.Fatalf("pickCloudID: %v", err)
	}
	if got != "cloud-123" {
		t.Fatalf("cloudid = %q, quiero el del sitio configurado (cloud-123)", got)
	}

	// Sitio no autorizado ⇒ error de configuración (400), no una consulta a ciegas.
	_, err = pickCloudID(res, "https://tercera.atlassian.net")
	if !errors.Is(err, connector.ErrBadQuery) || !strings.Contains(err.Error(), "tercera.atlassian.net") {
		t.Fatalf("err = %v, quiero ErrBadQuery nombrando el sitio", err)
	}
}

// TestRestBase: solo el consentimiento contra un sitio Cloud cambia de raíz. Todo lo
// demás —auth básica, Data Center— sigue hablando con el sitio, que es como funcionan
// las conexiones ya creadas.
func TestRestBase(t *testing.T) {
	t.Parallel()
	for nombre, tc := range map[string]struct {
		site      string
		hasBearer bool
	}{
		"cloud sin bearer":    {"https://acme.atlassian.net", false},
		"data center":         {"https://jira.acme.com", true},
		"sin bearer ni cloud": {"https://jira.acme.com", false},
	} {
		got, err := restBase(t.Context(), connector.Runtime{}, connector.Instance{HasBearer: tc.hasBearer}, tc.site)
		if err != nil || got != tc.site {
			t.Errorf("%s: restBase = (%q, %v), quiero el sitio sin tocar", nombre, got, err)
		}
	}
}

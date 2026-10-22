// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)

package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// CLOUD-03: el host de una app del catálogo no puede ser nunca la URL de
// producción de otro proyecto (`<slug>.<dominio>`), sea cual sea el slug.
func TestHostDeAppNoEsElDeNingunProyecto(t *testing.T) {
	const dom = "apps.sytue.eu"
	for _, app := range []string{"gitea", "grafana", "umami", "woodpecker"} {
		for _, slug := range []string{"foo", "acme", "gitea-foo"} {
			h := HostnameDeApp(uuid.New(), app, dom)
			etiqueta := strings.TrimSuffix(h, "."+dom)
			if ValidSlug(etiqueta) {
				t.Fatalf("%q lo puede reclamar el proyecto %q", h, etiqueta)
			}
			if h == slug+"."+dom || h == app+"-"+slug+"."+dom {
				t.Fatalf("colisión: %q", h)
			}
		}
	}
}

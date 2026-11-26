// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

// TestLauncherTilesUnaPorAplicacion: la misma aplicación conectada dos veces con la
// misma dirección de lanzamiento salía dos veces en el lanzador de todas las apps. Una
// dirección distinta sí es otra tesela (otro dominio, otra cuenta).
func TestLauncherTilesUnaPorAplicacion(t *testing.T) {
	byKey := map[string]*manifest.Manifest{"google-workspace": {Name: "Google Workspace"}}
	acme := map[string]string{manifest.KeyLaunchURL: "https://www.google.com/a/acme.com/ServiceLogin"}
	beta := map[string]string{manifest.KeyLaunchURL: "https://www.google.com/a/beta.com/ServiceLogin"}
	tiles := launcherTiles([]domain.AppInstance{
		conexion("nueva", "google-workspace", domain.StatusActive, true, acme, t0.Add(time.Hour)),
		conexion("vieja", "google-workspace", domain.StatusActive, true, acme, t0),
		conexion("beta", "google-workspace", domain.StatusActive, true, beta, t0),
		conexion("apagada", "google-workspace", domain.StatusDisabled, true, beta, t0),
		conexion("sin-url", "google-workspace", domain.StatusActive, true, nil, t0),
	}, byKey)
	if len(tiles) != 2 || tiles[0].Key != "nueva" || tiles[1].Key != "beta" {
		t.Fatalf("teselas = %+v; quiero una por dirección, la más reciente primero", tiles)
	}
}

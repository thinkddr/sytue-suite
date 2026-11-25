// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package widgets

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLoDelRepositorioPrivadoDiceQueSeActualizaConLaApp: lo que se publica desde el
// repositorio privado (Drive, CertiPDF, una app de registros en Matrix) sale con
// `app_only`, se lee igual y, ya rancio, el lector lo dice (PideAbrir). Cambiar solo
// la marca vuelve a escribir: si no, un snapshot del servidor seguiría diciendo que
// hace falta la app. Lo del servidor no lleva el campo.
func TestLoDelRepositorioPrivadoDiceQueSeActualizaConLaApp(t *testing.T) {
	scope := fixture(t)
	ctx := context.Background()
	privado := Detail{Counters: []Counter{{Kind: "files", Value: 2}}, SoloConLaApp: true}
	if err := PublishDetail(ctx, "eu.sytue.drive", "drive", scope, privado, Vigencia); err != nil {
		t.Fatal(err)
	}
	s, err := Read("eu.sytue.drive")
	if err != nil || !s.SoloConLaApp {
		t.Fatalf("leído: %+v, %v", s, err)
	}
	ahora := time.Unix(s.UpdatedAt, 0)
	if s.PideAbrir(ahora) || !s.PideAbrir(ahora.Add(Fresco)) {
		t.Fatal("PideAbrir no es «privado y rancio»")
	}
	ruta, err := path("eu.sytue.drive")
	if err != nil {
		t.Fatal(err)
	}
	crudo, err := os.ReadFile(ruta)
	if err != nil || !strings.Contains(string(crudo), `"app_only":true`) {
		t.Fatalf("en disco: %s, %v", crudo, err)
	}
	del := privado
	del.SoloConLaApp = false
	if err := PublishDetail(ctx, "eu.sytue.drive", "drive", scope, del, Vigencia); err != nil {
		t.Fatal(err)
	}
	if s, err := Read("eu.sytue.drive"); err != nil || s.SoloConLaApp {
		t.Fatalf("la misma lista desde el servidor sigue marcada: %+v, %v", s, err)
	}
	if crudo, _ := os.ReadFile(ruta); strings.Contains(string(crudo), "app_only") {
		t.Fatalf("lo del servidor lleva app_only: %s", crudo)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"os"
	"strings"
	"testing"
)

// TestLoDeInternetLlevaZoneIdentifier (FYS-02): un adjunto guardado con
// GuardarDeInternet lleva el flujo Zone.Identifier de la zona de Internet.
func TestLoDeInternetLlevaZoneIdentifier(t *testing.T) {
	ruta, err := GuardarDeInternet(t.TempDir(), "factura.pdf", strings.NewReader("%PDF"))
	if err != nil {
		t.Fatal(err)
	}
	marca, err := os.ReadFile(ruta + ":Zone.Identifier")
	if err != nil || !strings.Contains(string(marca), "ZoneId=3") {
		t.Fatalf("sin marca: %q %v", marca, err)
	}
}

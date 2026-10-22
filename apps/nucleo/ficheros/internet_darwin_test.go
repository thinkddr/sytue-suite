// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// TestLoDeInternetLlevaLaCuarentena (FYS-02): un adjunto guardado con
// GuardarDeInternet lleva `com.apple.quarantine`, y lo guardado con Guardar no.
func TestLoDeInternetLlevaLaCuarentena(t *testing.T) {
	dir := t.TempDir()
	ruta, err := GuardarDeInternet(dir, "factura.pdf", strings.NewReader("%PDF"))
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 128)
	n, err := unix.Getxattr(ruta, "com.apple.quarantine", buf)
	if err != nil || !strings.HasPrefix(string(buf[:n]), "0081;") {
		t.Fatalf("sin cuarentena: %q %v", buf[:max(n, 0)], err)
	}
	propio, err := Guardar(dir, "propio.pdf", strings.NewReader("%PDF"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unix.Getxattr(propio, "com.apple.quarantine", buf); err == nil {
		t.Fatal("Guardar sin más también marca")
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package voz

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSinSpeechDispatcherNoHayVoz: sin `spd-say` en el PATH, Leer dice ErrNoDisponible
// y no un fallo cualquiera, que es lo que hace que la app diga «no sabe leer en voz alta».
func TestSinSpeechDispatcherNoHayVoz(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := Leer("hola", "es", nil); !errors.Is(err, ErrNoDisponible) {
		t.Fatalf("sin spd-say Leer devuelve %v", err)
	}
}

// TestSpdSayRecibeElTextoYElIdioma: con un `spd-say` fingido que apunta lo que le llega,
// el texto va entero por la entrada, en UNA línea, con `-e -w -l es`; al acabar se llama
// a alAcabar; y callar pide a Speech Dispatcher que pare (`-S`).
func TestSpdSayRecibeElTextoYElIdioma(t *testing.T) {
	dir := t.TempDir()
	apunte := filepath.Join(dir, "apunte")
	// Solo órdenes internas del intérprete: el PATH del test no tiene ni `cat`.
	guion := "#!/bin/sh\necho \"$*\" >> " + apunte + "\nwhile IFS= read -r l; do echo \"$l\" >> " + apunte + "; done\n"
	if err := os.WriteFile(filepath.Join(dir, "spd-say"), []byte(guion), 0o755); err != nil { //nolint:gosec // G306: tiene que poder ejecutarse
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	acabo := make(chan struct{})
	callar, err := Leer("Estimado\ncliente:  adiós", "es-ES", func() { close(acabo) })
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-acabo:
	case <-time.After(10 * time.Second):
		t.Fatal("no se ha llamado a alAcabar")
	}
	callar()
	datos, _ := os.ReadFile(apunte) //nolint:gosec // G304: fichero del propio test
	lineas := strings.Split(strings.TrimSpace(string(datos)), "\n")
	if len(lineas) != 3 || lineas[0] != "-e -w -l es" || lineas[1] != "Estimado cliente: adiós" || lineas[2] != "-S" {
		t.Errorf("spd-say recibió %q", lineas)
	}
}

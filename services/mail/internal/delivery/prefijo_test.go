// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

// El MX guarda el mensaje con su Authentication-Results delante sin copiarlo a
// memoria: el lector concatenado tiene que leerse igual a trozos y por saltos.
func TestConPrefijoAnteponeLaCabecera(t *testing.T) {
	cuerpo := "From: a@ejemplo.org\r\nSubject: hola\r\n\r\ncuerpo\r\n"
	cab := "Authentication-Results: mx.sytue.eu; spf=pass\r\n"
	r := conPrefijo(cab, io.NewSectionReader(strings.NewReader(cuerpo), 0, int64(len(cuerpo))))
	if r.Size() != int64(len(cab)+len(cuerpo)) {
		t.Fatalf("tamaño %d", r.Size())
	}
	if err := iotest.TestReader(r, []byte(cab+cuerpo)); err != nil {
		t.Fatal(err)
	}
	trozo := make([]byte, 10)
	n, err := r.ReadAt(trozo, int64(len(cab))-4)
	if err != nil || string(trozo[:n]) != "ss\r\nFrom: " {
		t.Fatalf("lectura que cruza el límite: %q %v", trozo[:n], err)
	}
}

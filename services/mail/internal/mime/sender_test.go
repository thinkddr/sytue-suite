// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mime

import (
	"strings"
	"testing"
)

func TestEnsureSenderHeader(t *testing.T) {
	// Añade Sender cuando falta; From y cuerpo intactos.
	in := "From: Ada <ada@sytue.eu>\r\nSubject: Hola\r\n\r\nCuerpo del mensaje"
	out := string(EnsureSenderHeader([]byte(in), "asistente@sytue.eu"))
	if strings.Count(out, "Sender:") != 1 {
		t.Fatalf("debe haber exactamente un Sender:\n%s", out)
	}
	if !strings.Contains(out, "Sender: asistente@sytue.eu\r\n") {
		t.Fatalf("Sender no sellado:\n%s", out)
	}
	if !strings.Contains(out, "From: Ada <ada@sytue.eu>") {
		t.Fatalf("From alterado:\n%s", out)
	}
	if !strings.HasSuffix(out, "Cuerpo del mensaje") {
		t.Fatalf("cuerpo alterado:\n%s", out)
	}

	// Reemplaza un Sender previo del cliente (no se fía de él), incl. continuación.
	in2 := "Sender: viejo@sytue.eu\r\n\t(plegado)\r\nFrom: dir@sytue.eu\r\n\r\nx"
	out2 := string(EnsureSenderHeader([]byte(in2), "real@sytue.eu"))
	if strings.Contains(out2, "viejo@sytue.eu") || strings.Contains(out2, "plegado") {
		t.Fatalf("Sender previo (o su continuación) no eliminado:\n%s", out2)
	}
	if strings.Count(out2, "Sender:") != 1 || !strings.Contains(out2, "Sender: real@sytue.eu") {
		t.Fatalf("Sender no reemplazado limpio:\n%s", out2)
	}
	if !strings.Contains(out2, "From: dir@sytue.eu") {
		t.Fatalf("From perdido:\n%s", out2)
	}

	// Dirección vacía => sin cambios (enviar-como de buzón compartido no lleva Sender).
	if got := string(EnsureSenderHeader([]byte(in), "")); got != in {
		t.Fatalf("addr vacío debe dejar el mensaje intacto")
	}

	// Tolera terminadores LF (no solo CRLF).
	in3 := "From: a@b.eu\n\nhola"
	out3 := string(EnsureSenderHeader([]byte(in3), "s@b.eu"))
	if !strings.Contains(out3, "Sender: s@b.eu") || !strings.HasSuffix(out3, "hola") {
		t.Fatalf("LF no soportado:\n%s", out3)
	}
}

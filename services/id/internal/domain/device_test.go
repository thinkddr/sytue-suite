// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

var devKey = []byte("test-device-key-32-bytes-长度不重要-hmac")

func TestDeviceCookie_RoundTrip(t *testing.T) {
	b, err := GenerateToken(32)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	cookie := SignDeviceCookie(devKey, b)
	got, ok := ParseDeviceCookie(devKey, cookie)
	if !ok || got != b {
		t.Fatalf("round-trip falló: ok=%v got=%q want=%q", ok, got, b)
	}
}

func TestDeviceCookie_RejectsTamperAndForgery(t *testing.T) {
	b, _ := GenerateToken(32)
	cookie := SignDeviceCookie(devKey, b)

	// Firma con otra clave ⇒ no verifica.
	if _, ok := ParseDeviceCookie([]byte("otra-clave-distinta-000000000000"), cookie); ok {
		t.Fatal("una firma con otra clave NO debe verificar")
	}
	// Manipular el secreto (mantener la firma) ⇒ no verifica.
	if _, ok := ParseDeviceCookie(devKey, "x"+cookie); ok {
		t.Fatal("un secreto manipulado NO debe verificar")
	}
	// Formas malformadas ⇒ no verifican (sin pánico).
	for _, bad := range []string{"", ".", "sinpunto", b, "." + b, b + "."} {
		if _, ok := ParseDeviceCookie(devKey, bad); ok {
			t.Fatalf("cookie malformada %q NO debe verificar", bad)
		}
	}
}

func TestDeriveDeviceID_StablePerUserAndSecret(t *testing.T) {
	b, _ := GenerateToken(32)
	b2, _ := GenerateToken(32)

	id := DeriveDeviceID(devKey, "user-a", b)
	if id == "" {
		t.Fatal("device_id vacío")
	}
	// Determinista: mismo (usuario, secreto) ⇒ mismo id (estable entre logins).
	if DeriveDeviceID(devKey, "user-a", b) != id {
		t.Fatal("el device_id debe ser estable para (usuario, secreto)")
	}
	// El usuario entra en el MAC: mismo navegador, otra cuenta ⇒ device_id distinto
	// (la confianza es POR USUARIO; dos cuentas en un navegador no la comparten).
	if DeriveDeviceID(devKey, "user-b", b) == id {
		t.Fatal("distinto usuario debe dar distinto device_id con el mismo secreto")
	}
	// Otro secreto (otro navegador) ⇒ device_id distinto.
	if DeriveDeviceID(devKey, "user-a", b2) == id {
		t.Fatal("distinto secreto debe dar distinto device_id")
	}
	// El device_id no revela el secreto de la cookie (es un HMAC).
	if id == b {
		t.Fatal("el device_id no debe coincidir con el secreto de la cookie")
	}
}

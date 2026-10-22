// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package stripe

import "testing"

func TestCheckoutURLNoAceptaOtroHost(t *testing.T) {
	if !checkoutURL("https://checkout.stripe.com/c/pay/cs_test_123") {
		t.Fatal("checkout oficial rechazado")
	}
	if checkoutURL("https://checkout.stripe.com.ejemplo.test/c/pay/cs_test_123") || checkoutURL("https://evil.example/") {
		t.Fatal("host no confiable aceptado")
	}
}

func TestCheckoutID(t *testing.T) {
	if !checkoutID("cs_test_123456") || checkoutID("pi_123456") || checkoutID("cs_test_foo/../../bar") {
		t.Fatal("validación de id de sesión incorrecta")
	}
}

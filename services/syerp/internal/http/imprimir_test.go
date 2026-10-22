// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import "testing"

func TestPrecioSinIVA(t *testing.T) {
	for _, c := range []struct {
		precio, tipo int64
		want         string
	}{
		{1210, 21, "10,00 €"},
		{250, 4, "2,4038 €"},
		{-250, 4, "-2,4038 €"},
		{123456789, 21, "1.020.304,0413 €"},
		{5000, 0, "50,00 €"},
	} {
		if got := precioSinIVA(c.precio, c.tipo); got != c.want {
			t.Errorf("precioSinIVA(%d, %d) = %q, se esperaba %q", c.precio, c.tipo, got, c.want)
		}
	}
}

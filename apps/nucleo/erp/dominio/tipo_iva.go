// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dominio

import (
	"fmt"
	"strings"
)

// TiposIVAHistoricos son los tipos de IVA que ha habido desde 2023, en centésimas
// de punto: lo que se reconoce en una factura que solo trae base y cuota.
var TiposIVAHistoricos = []int64{2100, 1000, 750, 500, 400, 200}

// TipoDeCuota deduce el tipo de IVA de una base y su cuota, en centésimas de punto.
// La AEAT admite 10 céntimos de diferencia entre la cuota y base × tipo, por
// redondeos; más allá, la cuota no corresponde a ningún tipo.
func TipoDeCuota(base, cuota int64) (int64, bool) {
	mejor, dif := int64(0), int64(-1)
	for _, t := range TiposIVAHistoricos {
		esperada := Redondear(base*t, 10000)
		d := max(esperada-cuota, cuota-esperada)
		if dif < 0 || d < dif {
			mejor, dif = t, d
		}
	}
	return mejor, dif >= 0 && dif <= 10
}

// Redondear divide redondeando al entero más cercano, también con negativos.
func Redondear(n, d int64) int64 {
	if n < 0 {
		return -((-n + d/2) / d)
	}
	return (n + d/2) / d
}

// Porcentaje escribe un tipo en centésimas de punto (2100 = 21 %) sin decimales
// que sobren: «21», «5.5», «0».
func Porcentaje(centesimas int64) string {
	if centesimas%100 == 0 {
		return fmt.Sprintf("%d", centesimas/100)
	}
	s := fmt.Sprintf("%d.%02d", centesimas/100, centesimas%100)
	return strings.TrimSuffix(s, "0")
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import "time"

// # El período del libro y del balance (`periodo.ts`)
//
// Los cortes que pide quien lleva la contabilidad —este mes, el anterior, el
// trimestre, el año— sin teclear fechas. El servicio acota por `desde`/`hasta`.

// Periodos son los de la web, en su orden: el del `<select>`.
var Periodos = []string{"todo", "mes", "mesAnterior", "trimestre", "anio", "anioAnterior"}

// rotuloPeriodo es `ROTULO_PERIODO`: la clave del catálogo de cada uno.
var rotuloPeriodo = map[string]string{
	"todo":         "ledger.periodAll",
	"mes":          "ledger.periodMonth",
	"mesAnterior":  "ledger.periodLastMonth",
	"trimestre":    "ledger.periodQuarter",
	"anio":         "ledger.periodYear",
	"anioAnterior": "ledger.periodLastYear",
}

// iso es el `YYYY-MM-DD` de un día del calendario LOCAL de `hoy`. `time.Date`
// normaliza como `Date` de JS: el día 0 de un mes es el último del anterior.
func iso(hoy time.Time, anio int, mes time.Month, dia int) string {
	return time.Date(anio, mes, dia, 12, 0, 0, 0, hoy.Location()).Format(time.DateOnly)
}

// RangoDePeriodo es `rangoDePeriodo`: el rango con los dos extremos INCLUIDOS; «todo»
// (o uno que no se conozca) no acota.
func RangoDePeriodo(p string, hoy time.Time) (desde, hasta string) {
	a, m := hoy.Year(), hoy.Month()
	switch p {
	case "mes":
		return iso(hoy, a, m, 1), iso(hoy, a, m+1, 0)
	case "mesAnterior":
		return iso(hoy, a, m-1, 1), iso(hoy, a, m, 0)
	case "trimestre":
		t := m - (m-1)%3
		return iso(hoy, a, t, 1), iso(hoy, a, t+3, 0)
	case "anio":
		return iso(hoy, a, time.January, 1), iso(hoy, a, time.December, 31)
	case "anioAnterior":
		return iso(hoy, a-1, time.January, 1), iso(hoy, a-1, time.December, 31)
	}
	return "", ""
}

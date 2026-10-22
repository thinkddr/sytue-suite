// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "time"

// ---------------------------------------------------------------------------
// Alertas de coste (paridad DigitalOcean billing alerts)
// ---------------------------------------------------------------------------
//
// El formulario del límite de gasto PROMETÍA «Te avisaremos al acercarte a X €» y
// no había nada detrás. Esto lo cumple.
//
// El aviso se ancla al LÍMITE DE GASTO que el cliente ya configura, no a un
// importe aparte: es el número que él eligió como techo, así que es el que da
// sentido a «te estás acercando». Sin límite configurado no hay nada con que
// comparar y no se avisa — no se inventa un techo.

// SpendThresholds son los porcentajes del límite que disparan aviso, en orden
// ascendente. Dos y no cinco: avisar demasiado enseña a ignorar los avisos, y el
// 100 % ya lo frena el choke point de Deploy (ErrSpendLimitExceeded), así que el
// del 100 % es «te acabas de quedar sin desplegar», no una sorpresa posterior.
var SpendThresholds = []int{80, 100}

// SpendAlertPeriod es el periodo al que se ancla el dedupe: el mes natural, el
// mismo que factura BuildInvoiceLines. Cruzar el 80 % avisa UNA vez en el mes;
// al entrar el mes siguiente el contador de gasto vuelve a cero y los avisos
// también, sin ninguna tarea de limpieza.
func SpendAlertPeriod(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// CrossedSpendThresholds devuelve los umbrales que el gasto ya ha superado, en
// orden ascendente. Predicado PURO.
//
// Devuelve TODOS los cruzados, no solo el mayor, a propósito: si el gasto pega un
// salto grande entre dos evaluaciones (un despliegue caro, o el evaluador estuvo
// caído), el cliente debe recibir igualmente el aviso del 80 % además del de
// 100 %. Quien llama deduplica por periodo, así que no hay riesgo de repetirlos.
//
// Un límite <= 0 no es un techo (es «sin límite»): no cruza nada.
func CrossedSpendThresholds(spendEur, limitEur float64) []int {
	if limitEur <= 0 || spendEur <= 0 {
		return nil
	}
	var out []int
	for _, pct := range SpendThresholds {
		if spendEur >= limitEur*float64(pct)/100 {
			out = append(out, pct)
		}
	}
	return out
}

// SpendAlertPayload es el cuerpo del aviso de coste que viaja al webhook. Lleva
// las cifras además del porcentaje para que el receptor no tenga que consultarlas.
type SpendAlertPayload struct {
	ThresholdPct int       `json:"threshold_pct"`
	SpendEur     float64   `json:"spend_eur"`
	LimitEur     float64   `json:"limit_eur"`
	PeriodStart  time.Time `json:"period_start"`
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package verifactu

import (
	"net/url"
	"time"
)

// Servicio de cotejo al que apunta el QR tributario (especificación del QR v0.5.0).
const (
	qrPruebas    = "https://prewww2.aeat.es/wlpl/TIKE-CONT/ValidarQR"
	qrProduccion = "https://www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR"
)

// Leyendas que acompañan al QR en la factura: encima y debajo.
const (
	LeyendaQR        = "QR tributario:"
	LeyendaVerifactu = "VERI*FACTU"
)

// URLQR es la dirección que codifica el QR de una factura (se pinta con
// platform/pkg/qr, nivel M): con ella el cliente comprueba en la sede de la AEAT
// que la factura está registrada. El importe es el total de la factura.
func URLQR(produccion bool, nif, numSerie string, fecha time.Time, importeTotal int64) string {
	base := qrPruebas
	if produccion {
		base = qrProduccion
	}
	// El orden de los parámetros es el de la especificación; url.Values los
	// ordenaría alfabéticamente.
	return base + "?nif=" + url.QueryEscape(nif) +
		"&numserie=" + url.QueryEscape(numSerie) +
		"&fecha=" + url.QueryEscape(Fecha(fecha)) +
		"&importe=" + url.QueryEscape(Importe(importeTotal))
}

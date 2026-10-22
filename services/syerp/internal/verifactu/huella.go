// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package verifactu genera, encadena y remite a la Agencia Tributaria los
// registros de facturación del Reglamento de sistemas informáticos de facturación
// (RD 1007/2023 y Orden HAC/1177/2024) en la modalidad VERI*FACTU: cada factura
// emitida deja un registro de alta (o de anulación) con la huella del anterior, se
// envía a la AEAT en el momento y la factura lleva el QR con el que el cliente la
// comprueba en la sede electrónica.
//
// Se implementa al pie de la letra de los documentos técnicos de la AEAT: huella
// (v0.1.2), QR (v0.5.0) y servicio web (v1.0.3), con los ejemplos de esos
// documentos como pruebas. Nada de aquí sabe de la base de datos ni de HTTP del
// ERP: recibe datos y devuelve XML, huellas y respuestas.
package verifactu

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // la hora de Madrid también en una imagen sin zoneinfo
)

// HuellaAlta es la huella de un registro de alta: SHA-256, en hexadecimal y en
// mayúsculas, de «IDEmisorFactura=…&NumSerieFactura=…&…» con los valores tal y
// como van en el XML. La del primer registro de la cadena lleva «Huella=» vacío.
func HuellaAlta(nif, numSerie, fecha, tipoFactura, cuotaTotal, importeTotal, huellaAnterior, generado string) string {
	return huella(
		"IDEmisorFactura", nif,
		"NumSerieFactura", numSerie,
		"FechaExpedicionFactura", fecha,
		"TipoFactura", tipoFactura,
		"CuotaTotal", cuotaTotal,
		"ImporteTotal", importeTotal,
		"Huella", huellaAnterior,
		"FechaHoraHusoGenRegistro", generado,
	)
}

// HuellaAnulacion es la huella de un registro de anulación.
func HuellaAnulacion(nif, numSerie, fecha, huellaAnterior, generado string) string {
	return huella(
		"IDEmisorFacturaAnulada", nif,
		"NumSerieFacturaAnulada", numSerie,
		"FechaExpedicionFacturaAnulada", fecha,
		"Huella", huellaAnterior,
		"FechaHoraHusoGenRegistro", generado,
	)
}

// huella concatena los pares nombre=valor con «&», sin espacios en los extremos de
// cada valor, y calcula el SHA-256 de la cadena en UTF-8.
func huella(pares ...string) string {
	var b strings.Builder
	for i := 0; i+1 < len(pares); i += 2 {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(pares[i])
		b.WriteByte('=')
		b.WriteString(strings.TrimSpace(pares[i+1]))
	}
	suma := sha256.Sum256([]byte(b.String()))
	return strings.ToUpper(hex.EncodeToString(suma[:]))
}

// Importe escribe céntimos como los pide la AEAT: punto decimal, dos decimales y
// sin ceros a la izquierda («241.40», «-12.35», «0.00»).
func Importe(centimos int64) string {
	signo := ""
	if centimos < 0 {
		signo, centimos = "-", -centimos
	}
	return fmt.Sprintf("%s%d.%02d", signo, centimos/100, centimos%100)
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

// Fecha escribe una fecha como la AEAT: dd-mm-aaaa.
func Fecha(t time.Time) string { return t.Format("02-01-2006") }

// madrid es la hora de la AEAT. El huso va en el propio valor, así que sirve
// cualquiera, pero el de los ejemplos y el de quien lo lee es este.
var madrid = func() *time.Location {
	l, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		return time.FixedZone("CET", 3600)
	}
	return l
}()

// FechaHora escribe el momento de generación del registro con su huso
// («2024-01-01T19:20:30+01:00»), en hora de Madrid.
func FechaHora(t time.Time) string { return t.In(madrid).Format("2006-01-02T15:04:05-07:00") }

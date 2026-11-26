// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package obligaciones

import (
	"context"
	"strings"
	"time"
)

// Saldo es lo asentado en una cuenta entre dos fechas, en céntimos.
type Saldo struct {
	Cuenta      string
	Debe, Haber int64
}

// ResultadoCalculo es la cifra que sale del libro para una obligación y un periodo:
// la forma JSON de `GET /api/v1/obligaciones/calculo`.
type ResultadoCalculo struct {
	Clave             string  `json:"clave"`
	Periodo           string  `json:"periodo"`
	Desde             string  `json:"desde"`
	Hasta             string  `json:"hasta"`
	Tipo              string  `json:"tipo"`
	Porcentaje        float64 `json:"porcentaje"`
	HaberCentimos     int64   `json:"haber_centimos"`
	DebeCentimos      int64   `json:"debe_centimos"`
	ResultadoCentimos int64   `json:"resultado_centimos"`
	// Asentado: hay asientos en el periodo. Sin ellos la cifra no dice nada.
	Asentado bool `json:"asentado"`
}

// Calcular saca del libro la cifra de la obligación en un periodo ya leído
// (RangoDePeriodo): el IVA a ingresar del trimestre, la base del pago fraccionado.
// balance da los saldos asentados entre dos fechas; lo pone cada almacén. Es la parte
// de «preparar la presentación» que el ERP puede hacer solo; la presentación en sí
// se hace en el portal.
func (c Calculo) Calcular(ctx context.Context, clave, periodo string, desde, hasta time.Time, balance func(ctx context.Context, desde, hasta time.Time) ([]Saldo, error)) (ResultadoCalculo, error) {
	// Acumulado (el 130): el resultado desde el 1 de enero al cierre de cada
	// periodo del año hasta este, para restar lo que ya se pagó en los anteriores.
	cierres := []time.Time{hasta}
	if c.Acumulado {
		cierres = cierres[:0]
		// El día 0 de un mes es el último del anterior: 31 de marzo, 30 de junio…
		for q := 1; !time.Date(desde.Year(), time.Month(3*q+1), 0, 0, 0, 0, 0, time.UTC).After(hasta); q++ {
			cierres = append(cierres, time.Date(desde.Year(), time.Month(3*q+1), 0, 0, 0, 0, 0, time.UTC))
		}
		desde = time.Date(desde.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	}
	var haber, debe int64
	acumulados := make([]int64, 0, len(cierres))
	asentado := false
	for _, cierre := range cierres {
		saldos, err := balance(ctx, desde, cierre)
		if err != nil {
			return ResultadoCalculo{}, err
		}
		// Saldo acreedor de las cuentas «haber» menos saldo deudor de las «debe»:
		// para el IVA, 477 (repercutido) menos 472 (soportado). Sobre ingresos y
		// gastos (7 y 6) es el resultado del periodo.
		haber, debe = 0, 0
		for _, s := range saldos {
			if tienePrefijo(s.Cuenta, c.Haber) {
				haber += s.Haber - s.Debe
			}
			if tienePrefijo(s.Cuenta, c.Debe) {
				debe += s.Debe - s.Haber
			}
		}
		acumulados = append(acumulados, haber-debe)
		asentado = len(saldos) > 0
	}
	resultado := haber - debe
	switch {
	case c.Acumulado:
		resultado = PagoFraccionado(acumulados, c.Porcentaje)
	case c.Tipo == "porcentaje_resultado":
		resultado = int64(float64(resultado) * c.Porcentaje / 100)
	}
	return ResultadoCalculo{
		Clave: clave, Periodo: periodo, Desde: desde.Format(time.DateOnly), Hasta: hasta.Format(time.DateOnly),
		Tipo: c.Tipo, Porcentaje: c.Porcentaje, HaberCentimos: haber, DebeCentimos: debe, ResultadoCentimos: resultado,
		Asentado: asentado,
	}, nil
}

func tienePrefijo(cuenta string, prefijos []string) bool {
	for _, p := range prefijos {
		if strings.HasPrefix(cuenta, p) {
			return true
		}
	}
	return false
}

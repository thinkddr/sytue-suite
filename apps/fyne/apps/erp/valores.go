// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"encoding/json"
	"math"
	"strconv"
	"time"
)

// Lo que llega en `data` de un registro es JSON sin tipo (`map[string]any`), así que
// antes de formatear hay que saber qué es. Está aparte de `Celda` porque es lo único de
// este paquete que se puede equivocar en silencio —un céntimo— y así se prueba solo.

// entero devuelve el valor como entero de 64 bits. Es lo que hace falta para el dinero,
// que viaja en CÉNTIMOS: `encoding/json` los entrega como `float64`, y hasta
// 2^53 céntimos (90 billones de euros) eso es exacto, así que redondear al entero más
// cercano recupera el número que mandó el servicio sin perder nada.
//
// `json.Number` se contempla porque un decodificador con `UseNumber` los da así, y
// entonces la cadena es el dato exacto y no hay por qué pasar por un flotante.
func entero(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > math.MaxInt64 {
			return 0, false
		}
		return int64(math.Round(n)), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}

// texto es el valor como cadena. Un número se escribe SIN notación científica y sin
// cola de ceros: `fmt.Sprint` de un float64 grande da «1.23456789e+08», que en una
// celda de un ERP se lee como un error de datos.
func texto(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case json.Number:
		return s.String()
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	case bool:
		if s {
			return "✓"
		}
		return SinValor
	case nil:
		return ""
	}
	return ""
}

// parsearEntero lee la parte entera de un decimal canónico. Vacía es cero, que es lo
// que significa «.5».
func parsearEntero(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

// fechaISO lee una fecha `AAAA-MM-DD` en UTC, que es como la guarda el servicio.
//
// Se fija UTC a propósito: la web construye `new Date(iso + 'T00:00:00')`, que es hora
// LOCAL, y con eso una factura del día 1 emitida en Canarias puede salir escrita como
// del 31. Aquí una fecha sin hora es un día del calendario y no un instante.
func fechaISO(s string) (time.Time, bool) {
	t, err := time.ParseInLocation(time.DateOnly, s, time.UTC)
	return t, err == nil
}

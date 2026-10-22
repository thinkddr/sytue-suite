// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Zonas horarias IANA para la expansión de recurrencias con DST correcto
// (RFC 5545 3.3.5: DATE-TIME con TZID). La tzdata la aporta la stdlib (embebida
// vía time/tzdata en el binario, ver cmd/calendar); aquí solo cacheamos las
// *time.Location resueltas para no repetir la I/O de time.LoadLocation.
package domain

import (
	"sync"
	"time"
)

// locCache memoiza TZID IANA -> *time.Location. La expansión de una serie resuelve
// MUCHAS ocurrencias con la MISMA zona; cargar tzdata por ocurrencia dispararía el
// coste a escala. sync.Map: lecturas concurrentes sin contención, escritura rara.
var locCache sync.Map // string -> *time.Location

// LoadLocationCached resuelve un TZID IANA (p. ej. "Europe/Madrid") a su
// *time.Location, cacheado. Un TZID vacío o desconocido degrada a UTC (nunca
// devuelve nil ni rompe la expansión). Cachea también los fallos (como UTC) para
// no reintentar la carga de una zona inexistente en cada llamada.
func LoadLocationCached(tzid string) *time.Location {
	if tzid == "" || tzid == "UTC" {
		return time.UTC
	}
	if v, ok := locCache.Load(tzid); ok {
		if loc, ok := v.(*time.Location); ok {
			return loc
		}
	}
	loc, err := time.LoadLocation(tzid)
	if err != nil {
		loc = time.UTC // TZID desconocido: floating/UTC en vez de fallar.
	}
	locCache.Store(tzid, loc)
	return loc
}

// EventLocation es la zona en la que se generan las ocurrencias de un evento: la
// de su TZID para eventos con hora; UTC (FLOTANTE) para los de día completo, que
// caen el mismo día natural en cualquier zona y no deben desplazarse por DST.
func EventLocation(tzid string, allDay bool) *time.Location {
	if allDay {
		return time.UTC
	}
	return LoadLocationCached(tzid)
}

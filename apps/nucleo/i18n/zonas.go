// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package i18n

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Las zonas horarias IANA que ofrece el selector de zona de un evento de Sytue
// Calendar: `Intl.supportedValuesOf('timeZone')` del motor de la web, volcado por
// gen-i18n.ts. No es texto de interfaz —es la misma lista en los 26 idiomas— pero Go no
// sabe listarlas, así que viaja con los formatos.
//
//go:embed zonas.json
var zonasJSON []byte

var zonas = func() []string {
	var z []string
	if err := json.Unmarshal(zonasJSON, &z); err != nil {
		panic("i18n: zonas.json: " + err.Error())
	}
	return z
}()

// ZonasHorarias es `listTimeZones()`.
func ZonasHorarias() []string { return zonas }

// EtiquetaZona es `zoneLabel`: «Europe/Madrid (GMT+02:00)», con el desfase de la zona en
// el instante `en`. Una zona que el sistema no conoce sale sin desfase.
func EtiquetaZona(zona string, en time.Time) string {
	nombre := strings.ReplaceAll(zona, "_", " ")
	loc, err := time.LoadLocation(zona)
	if err != nil {
		return nombre
	}
	_, off := en.In(loc).Zone()
	signo := "+"
	if off < 0 {
		signo, off = "-", -off
	}
	return fmt.Sprintf("%s (GMT%s%02d:%02d)", nombre, signo, off/3600, off%3600/60)
}

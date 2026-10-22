// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Cuando es `formatWhen` de lib/search.ts: la línea de «cuándo» de un evento, la ÚNICA,
// que usan la vista rápida y el resultado de la búsqueda. Cada rama es una plantilla
// COMPLETA del catálogo —el separador, el orden y la preposición cambian entre
// lenguas—, con la fecha y la hora de Intl.
//
// El fin de un día completo es EXCLUSIVO: el último día es el anterior al fin.
func (p *Pantalla) Cuando(inicio, fin time.Time, todoElDia bool) string {
	l := p.locale()
	fecha := func(t time.Time) string { return i18n.FormatDayMonthLong(l, t) }
	hora := func(t time.Time) string { return i18n.FormatTimeDigits(l, t) }
	if todoElDia {
		ultimo := inicio
		if !fin.IsZero() && fin.After(inicio) {
			ultimo = fin.AddDate(0, 0, -1)
		}
		if mismoDia(inicio, ultimo) {
			return p.t("when.allDay", i18n.Vars{"date": fecha(inicio)})
		}
		return p.t("when.allDayRange", i18n.Vars{"start": fecha(inicio), "end": fecha(ultimo)})
	}
	if fin.IsZero() {
		return p.t("when.at", i18n.Vars{"date": fecha(inicio), "time": hora(inicio)})
	}
	if mismoDia(inicio, fin) {
		return p.t("when.range", i18n.Vars{"date": fecha(inicio), "start": hora(inicio), "end": hora(fin)})
	}
	return p.t("when.rangeAcross", i18n.Vars{
		"startDate": fecha(inicio), "startTime": hora(inicio),
		"endDate": fecha(fin), "endTime": hora(fin),
	})
}

// cuandoDe es Cuando para una ocurrencia, en la zona de visualización.
func (p *Pantalla) cuandoDe(o Ocurrencia) string {
	ini, fin := ocurrenciaInstantes(o, p.zona())
	return p.Cuando(ini, fin, o.Event.AllDay)
}

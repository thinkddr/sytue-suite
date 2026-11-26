// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package obligaciones

import (
	"fmt"
	"strings"
	"time"
)

// EventosICS son los eventos del calendario descargable: la revisión semanal de los
// lunes y un evento por obligación en «tu fecha», con su aviso. uid distingue la
// organización en los UID («<uid>-<clave>-<periodo>@erp.sytue.eu»); rotulo traduce las
// claves del catálogo (`obligacion.*`, `ambito.*`, `aviso.*`).
func EventosICS(inst []Instancia, uid string, rotulo func(string) string, ahora time.Time) []Evento {
	eventos := make([]Evento, 0, 1+len(inst))
	eventos = append(eventos, Evento{
		UID:         uid + "-revision-semanal@erp.sytue.eu",
		Fecha:       proximoLunes(ahora.UTC()),
		Resumen:     rotulo("aviso.revision_semanal"),
		Descripcion: rotulo("aviso.revision_semanal_pasos"),
		Repetir:     "FREQ=WEEKLY;BYDAY=MO",
	})
	for _, i := range inst {
		ambito := rotulo("ambito." + i.Ambito)
		eventos = append(eventos, Evento{
			UID:         fmt.Sprintf("%s-%s-%s@erp.sytue.eu", uid, i.Clave, i.Periodo),
			Fecha:       i.Recomendada,
			Resumen:     i.Nombre + " · " + EtiquetaPeriodo(i.Periodo),
			Descripcion: descripcionICS(i, rotulo("obligacion."+i.Concepto), ambito, rotulo),
			URL:         i.Portal.URL,
			Categoria:   ambito,
			AvisoDias:   7,
		})
	}
	return eventos
}

// descripcionICS es lo que se lee al abrir el evento: qué es, las fechas que
// cuentan, qué hay que tener y dónde se presenta.
func descripcionICS(i Instancia, concepto, ambito string, rotulo func(string) string) string {
	var b strings.Builder
	b.WriteString(concepto + " · " + ambito)
	b.WriteString("\n" + rotulo("aviso.tu_fecha") + ": " + i.Recomendada.Format(time.DateOnly))
	if i.LimiteDomiciliacion != nil {
		b.WriteString("\n" + rotulo("aviso.limite_domiciliacion") + ": " + i.LimiteDomiciliacion.Format(time.DateOnly))
	}
	b.WriteString("\n" + rotulo("aviso.limite_legal") + ": " + i.Limite.Format(time.DateOnly))
	if g := i.Guia; g != nil {
		b.WriteString("\n\n" + g.Que)
		for _, n := range g.Necesitas {
			b.WriteString("\n• " + n)
		}
	}
	b.WriteString("\n\n" + i.Portal.Nombre + ": " + i.Portal.URL)
	if i.Sancion != nil && i.Sancion.Nota != "" {
		b.WriteString("\n" + i.Sancion.Nota)
	}
	return b.String()
}

// proximoLunes es el lunes de esta semana si hoy es lunes, o el siguiente.
func proximoLunes(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return d.AddDate(0, 0, (8-int(d.Weekday()))%7)
}

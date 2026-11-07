// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// colorCalendarioPorDefecto es el acento de un calendario sin color válido: el
// `AcentoPorDefecto` de la pantalla de Calendar.
const colorCalendarioPorDefecto = "#1a6fbb"

// Calendar lee TODOS los calendarios (propios y compartidos) y sus ocurrencias del mes
// en curso, hasta al menos ocho días vista: el widget enseña hoy y lo próximo aunque
// la app esté mirando otro mes, otro día o con calendarios ocultos, y a medianoche ya
// tiene el «hoy» nuevo. Peticiones: 2 + una por calendario.
func Calendar(c Comunes, cal *api.Calendars) widgets.Fuente {
	return func(ctx context.Context) (widgets.Detail, error) {
		var compartidos []api.Calendar
		var errC error
		var lecturas sync.WaitGroup
		lecturas.Go(func() { compartidos, errC = cal.SharedWithMe(ctx) })
		propios, err := cal.List(ctx)
		lecturas.Wait()
		if err != nil {
			return widgets.Detail{}, err
		}
		if errC != nil {
			// Sin los compartidos el contador de calendarios mentiría.
			return widgets.Detail{}, errC
		}
		todos := unicos(propios, compartidos)
		ahora := c.ahora()
		zona := ahora.Location()
		inicioMes := time.Date(ahora.Year(), ahora.Month(), 1, 0, 0, 0, 0, zona)
		hasta := inicioMes.AddDate(0, 1, 0)
		if semana := inicioDelDia(ahora).AddDate(0, 0, 8); semana.After(hasta) {
			hasta = semana
		}
		porCal, err := instanciasDe(ctx, cal, todos, inicioMes, hasta)
		if err != nil {
			return widgets.Detail{}, err
		}
		return detalleCalendar(c, todos, porCal, ahora), nil
	}
}

func unicos(listas ...[]api.Calendar) []api.Calendar {
	visto := map[string]bool{}
	var out []api.Calendar
	for _, l := range listas {
		for _, c := range l {
			if c.ID == "" || visto[c.ID] {
				continue
			}
			visto[c.ID] = true
			out = append(out, c)
		}
	}
	return out
}

// instanciasDe pide las ocurrencias de cada calendario a la vez. Un 403/404 de UN
// calendario (le han retirado la compartición) lo quita; otro error de uno tumba la
// ronda: publicar sin él enseñaría un día vacío que no lo está.
func instanciasDe(ctx context.Context, cal *api.Calendars, cals []api.Calendar, desde, hasta time.Time) (map[string][]api.Instance, error) {
	type par struct {
		id   string
		inst []api.Instance
		err  error
	}
	pares := make(chan par, len(cals))
	for _, c := range cals {
		go func() {
			inst, err := cal.Instances(ctx, c.ID, desde, hasta)
			pares <- par{c.ID, inst, err}
		}()
	}
	out := make(map[string][]api.Instance, len(cals))
	var primerError error
	for range cals {
		r := <-pares
		switch {
		case r.err == nil:
			out[r.id] = r.inst
		case sinAcceso(r.err):
		case primerError == nil:
			primerError = r.err
		}
	}
	return out, primerError
}

type ocurrencia struct {
	api.Instance
	color string
}

func ocurrenciasDe(cals []api.Calendar, porCal map[string][]api.Instance) []ocurrencia {
	var out []ocurrencia
	for _, c := range cals {
		for _, inst := range porCal[c.ID] {
			if strings.EqualFold(inst.Event.Status, "CANCELLED") {
				continue
			}
			color := widgets.SanitizeColor(inst.Event.Color)
			if color == "" {
				color = widgets.SanitizeColor(c.Color)
			}
			if color == "" {
				color = colorCalendarioPorDefecto
			}
			out = append(out, ocurrencia{Instance: inst, color: color})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Start.Equal(out[j].Start) {
			return out[i].Start.Before(out[j].Start)
		}
		return out[i].End.Before(out[j].End)
	})
	return out
}

// detalleCalendar es el widget: el número de calendarios, los eventos que quedan de
// hoy en adelante (como mucho MaxItems en la lista) y la rejilla del mes de `ahora`.
func detalleCalendar(c Comunes, cals []api.Calendar, porCal map[string][]api.Instance, ahora time.Time) widgets.Detail {
	zona := ahora.Location()
	inicioMes := time.Date(ahora.Year(), ahora.Month(), 1, 0, 0, 0, 0, zona)
	finMes := inicioMes.AddDate(0, 1, 0)
	hoy := inicioDelDia(ahora)
	var ocupados []int
	colores := map[int]string{}
	var items []widgets.Item
	var proximos int64
	for _, oc := range ocurrenciasDe(cals, porCal) {
		ini, fin := oc.Start.In(zona), oc.End.In(zona)
		if oc.Event.AllDay {
			ini, fin = flotante(oc.Start, zona), flotante(oc.End, zona)
		}
		finInc := finInclusivo(ini, fin, oc.Event.AllDay)
		ocupados = marcarDias(ocupados, colores, ini, finInc, inicioMes, finMes, oc.color)
		if !vigente(oc.Event.AllDay, ini, fin, finInc, ahora, hoy) {
			continue
		}
		proximos++
		if !llenos(items) {
			items = append(items, itemDeEvento(c, oc, ini))
		}
	}
	contadores := []widgets.Counter{{Kind: "calendars", Value: int64(len(cals))}}
	if proximos > 0 {
		contadores = append(contadores, widgets.Counter{Kind: "events", Value: proximos})
	}
	return widgets.Detail{
		Counters: contadores,
		Items:    widgets.SanitizeItems(items),
		Calendar: widgets.BuildCalendarGrid(ahora.Year(), ahora.Month(), ocupados, colores),
	}
}

func inicioDelDia(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// flotante es el día de un evento de día completo en la zona local: el servicio lo
// guarda a medianoche UTC y cae el mismo día en cualquier zona (api.Instance.Day).
// Pasarlo sin más a la local lo corría una hora en Madrid —el final caía a la 01:00 del
// día siguiente y ese día salía ocupado y el evento, vigente— y a la víspera al oeste.
// Es la regla de widgets.Vigentes, con la que el lector recorta las filas.
func flotante(t time.Time, zona *time.Location) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, zona)
}

// finInclusivo descuenta el final exclusivo de un evento de día completo.
func finInclusivo(ini, fin time.Time, diaCompleto bool) time.Time {
	if diaCompleto && fin.After(ini) && fin.Hour() == 0 && fin.Minute() == 0 && fin.Second() == 0 {
		return fin.Add(-time.Nanosecond)
	}
	return fin
}

// vigente: un evento de día completo vale hasta el final de su último día; el resto,
// mientras no haya terminado.
func vigente(diaCompleto bool, ini, fin, finInc, ahora, hoy time.Time) bool {
	if diaCompleto {
		return !finInc.Before(hoy)
	}
	return fin.After(ahora) || !ini.Before(ahora)
}

// marcarDias añade a la cuadrícula del mes los días que ocupa [ini, finInc].
func marcarDias(ocupados []int, colores map[int]string, ini, finInc, inicioMes, finMes time.Time, color string) []int {
	if finInc.Before(inicioMes) || !ini.Before(finMes) {
		return ocupados
	}
	primero, ultimo := ini, finInc
	if primero.Before(inicioMes) {
		primero = inicioMes
	}
	if limite := finMes.Add(-time.Nanosecond); ultimo.After(limite) {
		ultimo = limite
	}
	for d := inicioDelDia(primero).Day(); d <= inicioDelDia(ultimo).Day(); d++ {
		ocupados = append(ocupados, d)
		if _, ok := colores[d]; !ok && color != "" {
			colores[d] = color
		}
	}
	return ocupados
}

func itemDeEvento(c Comunes, oc ocurrencia, ini time.Time) widgets.Item {
	titulo := strings.TrimSpace(oc.Event.Summary)
	if titulo == "" {
		titulo = c.t("event.untitled")
	}
	insignia := ini.Format("15:04")
	if oc.Event.AllDay {
		insignia = ""
	}
	return widgets.Item{
		Title:    titulo,
		Subtitle: oc.Event.Location,
		Badge:    insignia,
		Color:    oc.color,
		StartAt:  max(oc.Start.Unix(), 0),
		EndAt:    max(oc.End.Unix(), 0),
		AllDay:   oc.Event.AllDay,
	}
}

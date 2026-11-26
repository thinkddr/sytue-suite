// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # «Compartir mi disponibilidad» (`Disponibilidad.tsx`)
//
// Los huecos libres de los próximos días dentro de la jornada de los ajustes, escritos
// como texto para pegarlos en un correo o un chat: lo que se hace a mano cuando alguien
// de fuera pregunta «¿cuándo te viene bien?». Los huecos son los de «buscar hueco» del
// servicio, así que cuenta lo mismo que el free/busy. El texto es el de
// `lib/disponibilidad.ts`, con la zona en la cabecera.

// Las duraciones mínimas y los días hacia delante que se ofrecen.
var (
	DuracionesDisponibilidad = []int{30, 60}
	VentanasDisponibilidad   = []int{7, 14}
)

// DiaDisponible son los tramos libres de un día: «lun, 30 nov» y «09:00–10:30».
type DiaDisponible struct {
	Dia    string
	Tramos []string
}

// DiasDisponibles es `diasDisponibles`: los huecos agrupados por día natural en `loc` y
// en orden. Un tramo al revés no se escribe.
func DiasDisponibles(huecos []api.Hueco, loc *time.Location, l i18n.Locale) []DiaDisponible {
	ordenados := slices.Clone(huecos)
	slices.SortStableFunc(ordenados, func(a, b api.Hueco) int { return a.Start.Compare(b.Start) })
	var out []DiaDisponible
	for _, h := range ordenados {
		if !h.End.After(h.Start) {
			continue
		}
		ini, fin := h.Start.In(loc), h.End.In(loc)
		dia := i18n.FormatWeekdayDayMonthShort(l, ini)
		tramo := i18n.FormatTimeDigits(l, ini) + "–" + i18n.FormatTimeDigits(l, fin)
		if n := len(out); n > 0 && out[n-1].Dia == dia {
			out[n-1].Tramos = append(out[n-1].Tramos, tramo)
			continue
		}
		out = append(out, DiaDisponible{Dia: dia, Tramos: []string{tramo}})
	}
	return out
}

// TextoDisponibilidad es `textoDisponibilidad`: la cabecera con la zona y una línea por
// día. Sin huecos, nada (no se copia una cabecera vacía).
func (p *Pantalla) TextoDisponibilidad(dias []DiaDisponible, zona string) string {
	if len(dias) == 0 {
		return ""
	}
	lineas := []string{p.t("availability.textHeader", i18n.Vars{"zone": zona})}
	for _, d := range dias {
		lineas = append(lineas, p.t("availability.textLine", i18n.Vars{"day": d.Dia, "slots": strings.Join(d.Tramos, ", ")}))
	}
	return strings.Join(lineas, "\n")
}

// compartirDisponibilidad abre el diálogo: duración y días arriba, lo que se va a copiar
// debajo, y «Copiar» en el pie, apagado mientras no haya nada.
func (a *App) compartirDisponibilidad() {
	lienzo := a.lienzo()
	if lienzo == nil || a.Cal == nil {
		return
	}
	p := a.P
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	duracion, dias := DuracionesDisponibilidad[0], VentanasDisponibilidad[0]
	zona := p.nombreZona()
	texto := ""
	resultado := container.New(ui.Columna{})
	var d *ui.Dialogo
	parrafo := func(clave, token string) fyne.CanvasObject {
		return ui.NuevoParrafo(p.t(clave, nil), tema.TextoSM, ui.Normal, token, v)
	}
	poner := func(o fyne.CanvasObject) {
		resultado.Objects = []fyne.CanvasObject{o}
		resultado.Refresh()
		if d != nil {
			d.Recolocar()
		}
	}
	pedir := func() {
		texto = ""
		if d != nil {
			d.SinConfirmar(true)
		}
		poner(parrafo("findTime.loading", "text-muted"))
		aj := p.ajustes()
		desde := p.ahora()
		hasta := desde.Add(time.Duration(dias) * 24 * time.Hour)
		dur, n := duracion, dias
		a.lanzar(func(vida context.Context) {
			ctx, parar := context.WithTimeout(vida, plazo)
			defer parar()
			l, err := a.Cal.SuggestSlots(ctx, desde, hasta, dur, zona,
				fmt.Sprintf("%02d:00", aj.InicioLaboral), fmt.Sprintf("%02d:00", aj.FinLaboral), !aj.FinesDeSemana)
			a.enUI(func() {
				// Una respuesta de otra elección (se cambió el selector mientras tanto) no vale.
				if dur != duracion || n != dias {
					return
				}
				switch {
				case err != nil:
					poner(parrafo("findTime.error", "danger-text"))
				default:
					texto = p.TextoDisponibilidad(DiasDisponibles(l, p.zona(), p.locale()), zona)
					if texto == "" {
						poner(parrafo("findTime.empty", "text-muted"))
						return
					}
					poner(ui.NuevoParrafo(texto, tema.TextoSM, ui.Normal, "text", v))
					d.SinConfirmar(false)
				}
			})
		})
	}

	opcDur := make([]string, len(DuracionesDisponibilidad))
	for i, m := range DuracionesDisponibilidad {
		opcDur[i] = p.etiquetaDuracion(m)
	}
	selDur := ui.NuevoSelector(opcDur, func(i int) { duracion = DuracionesDisponibilidad[i]; pedir() })
	selDur.Rotulo = p.t("availability.duration", nil)
	opcDias := make([]string, len(VentanasDisponibilidad))
	for i, n := range VentanasDisponibilidad {
		opcDias[i] = p.t("findTime.windowDays", i18n.Vars{"count": n})
	}
	selDias := ui.NuevoSelector(opcDias, func(i int) { dias = VentanasDisponibilidad[i]; pedir() })
	selDias.Rotulo = p.t("findTime.window", nil)

	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(4)},
		parrafo("availability.description", "text-muted"),
		container.New(&dosColumnas{Hueco: tema.Espacio(4), Corte: CorteSM, ventana: func() float32 { return lienzo.Size().Width }},
			ui.GrupoCampo(p.t("availability.duration", nil), false, selDur, "", ""),
			ui.GrupoCampo(p.t("findTime.window", nil), false, selDias, "", "")),
		resultado)
	d = ui.Confirmacion(p.t("availability.title", nil), p.t("common.cancel", nil), p.t("availability.copy", nil),
		false, cuerpo, nil, func() {
			if texto == "" {
				return
			}
			fyne.CurrentApp().Clipboard().SetContent(texto)
			d.Cerrar()
			a.avisar(ui.AvisoExito, "availability.copied")
		})
	a.mostrarDialogo(d, lienzo)
	pedir()
}

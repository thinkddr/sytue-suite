// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"math"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Los totales de la vista (`lib/resumen.ts` y el pie de Rejilla.tsx)
//
// Lo que el servicio cuenta sobre TODO lo que pasa la vista (`GET /tables/{id}/summary`),
// no sobre la ventana cargada: sumar aquí las cien filas que se ven daría un total que
// parece cierto y no lo es. Se enciende y se apaga desde el menú «⋯» de la tabla, porque
// cada cuenta es una pasada por la tabla entera.
//
// La web lo pinta como la última fila de la tabla; aquí va en una línea bajo la rejilla,
// columna a columna con su nombre delante, porque la rejilla nativa es un solo widget que
// mide y pinta sus filas y un pie dentro de él tendría que repartir sus anchos.

// pieDeColumna es `pieDeColumna` de la web: lo que se dice de una columna, y su detalle
// (media, mínimo y máximo). `ok` falso si no hay nada que decir: una columna sin ningún
// valor no dice «0 con valor».
func pieDeColumna(c api.BasesField, r *api.BasesFieldSummary, t func(string, ...i18n.Vars) string, idioma i18n.Locale) (texto, detalle string, ok bool) {
	if r == nil || r.Filled == 0 {
		return "", "", false
	}
	if c.Kind == api.BasesKindRating && r.Avg != nil {
		// Sumar estrellas no dice nada: de una valoración se quiere la media.
		return t("summary.average", i18n.Vars{"value": cifra(c, *r.Avg, idioma)}), "", true
	}
	tipo := efectivo(c).Kind
	if tipo == api.BasesKindNumber && r.Sum != nil {
		texto = t("summary.sum", i18n.Vars{"value": cifra(c, *r.Sum, idioma)})
		if r.Avg != nil && r.Min != nil && r.Max != nil {
			detalle = t("summary.detail", i18n.Vars{
				"avg": cifra(c, *r.Avg, idioma), "min": cifra(c, *r.Min, idioma), "max": cifra(c, *r.Max, idioma),
			})
		}
		return texto, detalle, true
	}
	if tipo == api.BasesKindCheckbox {
		return t("summary.checked", i18n.Vars{"n": r.Filled}), "", true
	}
	return t("summary.filled", i18n.Vars{"n": r.Filled}), "", true
}

// cifra es una cifra como la escribe su columna: en su moneda si la tiene.
func cifra(c api.BasesField, n float64, idioma i18n.Locale) string {
	if c.Kind == api.BasesKindCurrency {
		return textoVisible(c, n, idioma)
	}
	// Hasta dos decimales y sin ceros de relleno: el `maximumFractionDigits: 2` de la web.
	return i18n.FormatNumber(idioma, math.Round(n*100)/100)
}

// enRejilla dice si lo que se mira es la rejilla: el tablero, la galería y el formulario
// no llevan totales.
func (b *Base) enRejilla() bool {
	v := b.vista()
	return v == nil || v.Kind == api.BasesGrid
}

// alternarPie enciende o apaga los totales; al encenderlos se piden.
func (p *Pantalla) alternarPie() {
	p.conPie = !p.conPie
	if b := p.abierta; b != nil {
		b.resumen = nil
	}
	p.pedirResumen()
	p.Repintar()
}

// pedirResumen pide los totales de lo que se mira, con la vista, la búsqueda y el día de
// la rejilla. Un fallo deja el pie sin cifras: la rejilla sigue sirviendo y no se
// enseñan cifras viejas.
func (p *Pantalla) pedirResumen() {
	b := p.abierta
	if b == nil || !p.conPie || b.Activa == "" || p.svc == nil || !b.enRejilla() {
		return
	}
	tabla, vista, l := b.Activa, b.VistaID, b.lectura
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		r, err := p.svc.Summary(ctx, tabla, vista, l.api())
		enHilo(func() {
			if p.abierta != b || b.Activa != tabla || b.VistaID != vista || b.lectura != l {
				return
			}
			b.resumen = nil
			if err == nil {
				b.resumen = &r
			}
			p.Repintar()
		})
	})
}

// lineaResumen es el pie: «Registros: 120 · Importe — Suma: 2.500,00 € · …». nil sin
// totales que enseñar.
func (p *Pantalla) lineaResumen(v tema.Variante) fyne.CanvasObject {
	b := p.abierta
	if !p.conPie || b == nil || b.resumen == nil || !b.enRejilla() {
		return nil
	}
	partes := []string{p.t("summary.count", i18n.Vars{"n": b.resumen.Count})}
	for _, c := range b.Campos {
		r, hay := b.resumen.Fields[c.ID]
		if !hay {
			continue
		}
		texto, detalle, ok := pieDeColumna(c, &r, p.t, p.tr.Locale())
		if !ok {
			continue
		}
		if detalle != "" {
			texto += " (" + detalle + ")"
		}
		partes = append(partes, c.Name+" — "+texto)
	}
	return ui.NuevoParrafo(strings.Join(partes, " · "), tema.TextoXS, ui.Normal, "text-muted", v).A(p.anchoContenido())
}

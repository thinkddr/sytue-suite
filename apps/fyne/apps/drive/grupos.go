// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Los tramos de «Recientes»
//
// `lib/grupos-fecha.ts` de la web: «Hoy», «Ayer», «Esta semana» y «Antes». Una lista de
// cien ficheros con la fecha en letra pequeña obliga a leer fecha por fecha para saber
// dónde acaba lo de hoy; con los tramos se ve de un vistazo.
//
// Las cuentas son en la hora LOCAL de `ahora` (la zona de quien mira): «hoy» es su día
// de calendario y no las últimas 24 horas. La semana empieza el lunes (ISO), así que un
// lunes «Esta semana» no tiene nada: lo del domingo es «Ayer» y lo de antes, «Antes».

// GrupoFecha es un tramo, en el orden en que se pintan.
type GrupoFecha int

const (
	GrupoHoy GrupoFecha = iota
	GrupoAyer
	GrupoEstaSemana
	GrupoAntes
)

// claveGrupo es el rótulo de cada tramo (`RECENT_GROUP_TITLE`).
var claveGrupo = [...]string{
	GrupoHoy:        "recent.group.today",
	GrupoAyer:       "recent.group.yesterday",
	GrupoEstaSemana: "recent.group.thisWeek",
	GrupoAntes:      "recent.group.earlier",
}

// medianoche es el comienzo del día de `t` en su zona, desplazado `dias` días. Con
// time.Date y no restando horas, para que un cambio de hora no corra la frontera.
func medianoche(t time.Time, dias int) time.Time {
	a, m, d := t.Date()
	return time.Date(a, m, d+dias, 0, 0, 0, 0, t.Location())
}

// GrupoDeFecha es el tramo de `cuando` visto desde `ahora`. Sin fecha va a «Antes», que
// es lo único que no promete nada falso; una fecha futura (el reloj del servidor por
// delante) cuenta como «Hoy».
func GrupoDeFecha(cuando, ahora time.Time) GrupoFecha {
	if cuando.IsZero() {
		return GrupoAntes
	}
	switch {
	case !cuando.Before(medianoche(ahora, 0)):
		return GrupoHoy
	case !cuando.Before(medianoche(ahora, -1)):
		return GrupoAyer
	}
	// Weekday: 0 es domingo. Días desde el lunes de esta semana.
	desdeLunes := (int(ahora.Weekday()) + 6) % 7
	if !cuando.Before(medianoche(ahora, -desdeLunes)) {
		return GrupoEstaSemana
	}
	return GrupoAntes
}

// TramoDeFecha es un tramo con sus elementos, en el orden en que llegaron.
type TramoDeFecha struct {
	Grupo     GrupoFecha
	Elementos []Elemento
}

// AgruparPorFecha parte la lista en tramos SIN reordenarla: si dos elementos de un
// mismo tramo llegan separados se juntan en el suyo, conservando su orden relativo, y
// los tramos vacíos no salen (un rótulo sin nada debajo es ruido).
func AgruparPorFecha(els []Elemento, ahora time.Time) []TramoDeFecha {
	var por [GrupoAntes + 1][]Elemento
	for _, el := range els {
		g := GrupoDeFecha(el.Modificado, ahora)
		por[g] = append(por[g], el)
	}
	var out []TramoDeFecha
	for g, lista := range por {
		if len(lista) > 0 {
			out = append(out, TramoDeFecha{Grupo: GrupoFecha(g), Elementos: lista})
		}
	}
	return out
}

// Medidas del rótulo de tramo: `sytue-overline text-text-subtle px-3 pt-4 pb-1.5`.
const (
	ArribaRotuloTramo float32 = 4 * 0.25 * tema.Rem
	AbajoRotuloTramo  float32 = 1.5 * 0.25 * tema.Rem
)

// AltoRotuloTramo es lo que ocupa el rótulo: su caja de línea más el `pt-4 pb-1.5`.
var AltoRotuloTramo = ArribaRotuloTramo + tema.TextoXS.Linea + AbajoRotuloTramo

// rotuloTramo es el `<h2>` de un tramo, alineado con el texto de las filas (el `px-3`
// de la fila, dentro del de la lista).
func rotuloTramo(texto string, v tema.Variante) fyne.CanvasObject {
	ver := ui.NuevoVersal(texto, tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v))
	return container.New(&enTramo{v: ver}, ver.Objetos()...)
}

type enTramo struct{ v *ui.Versal }

var _ fyne.Layout = (*enTramo)(nil)

func (e *enTramo) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(CanalonLista+e.v.Ancho(), AltoRotuloTramo)
}

func (e *enTramo) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	e.v.Recortar(max(t.Width-2*CanalonLista, 0))
	e.v.Colocar(CanalonLista, ArribaRotuloTramo)
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Filtros rápidos (`FILTROS_RAPIDOS` de utils.ts)
//
// No leídos, destacados y con adjuntos, bajo la búsqueda: conmutadores, uno a la vez,
// y pulsar el activo vuelve a «todas». Filtran lo YA cargado —la lista sigue pidiendo
// páginas—, sin otra consulta al servicio.

// FiltroRapido es el filtro de la lista. El cero es «todas».
type FiltroRapido int

// Los filtros, en el orden de la franja.
const (
	FiltroTodas FiltroRapido = iota
	FiltroNoLeidas
	FiltroDestacadas
	FiltroConAdjuntos
)

// rotuloFiltro es `ROTULO_FILTRO` de MessageList.tsx.
var rotuloFiltro = map[FiltroRapido]string{
	FiltroTodas:       "filter.showAll",
	FiltroNoLeidas:    "filter.unread",
	FiltroDestacadas:  "filter.starred",
	FiltroConAdjuntos: "filter.attachments",
}

// FiltrarConversaciones es `filtrarConversaciones`: lo que deja ver un filtro. La
// conversación ABIERTA se queda aunque deje de cumplirlo —abrir una sin leer la marca
// leída, y que desaparezca bajo el cursor es perder el sitio—; sale al abrir otra.
func FiltrarConversaciones(cs []Conversacion, f FiltroRapido, abierta string) []Conversacion {
	if f == FiltroTodas {
		return cs
	}
	out := make([]Conversacion, 0, len(cs))
	for _, c := range cs {
		cumple := (f == FiltroNoLeidas && c.SinLeer) || (f == FiltroDestacadas && c.Destacada) ||
			(f == FiltroConAdjuntos && c.ConAdjunto)
		if cumple || (abierta != "" && c.HiloID == abierta) {
			out = append(out, c)
		}
	}
	return out
}

// Las medidas de la franja: `flex items-center gap-1 border-b px-4 py-1.5` con botones
// `size="xs"` (24 de alto).
const (
	rellenoFiltros float32 = 1.5 * 0.25 * tema.Rem
	huecoFiltros   float32 = 1 * 0.25 * tema.Rem
)

// AltoFiltros es lo que ocupa la franja con su filete.
var AltoFiltros = 2*rellenoFiltros + ui.MedidaBoton("", ui.TallaXS, 0).Height + tema.Borde

// filtrosDeLaFranja son los tres conmutadores, en su orden.
var filtrosDeLaFranja = []FiltroRapido{FiltroNoLeidas, FiltroDestacadas, FiltroConAdjuntos}

// botonesFiltro monta los tres conmutadores de la franja. Pulsar el activo vuelve a
// «todas»; cuál está activo se lee al pulsar, no al montar.
func (b *Bandeja) botonesFiltro() []*ui.Boton {
	out := make([]*ui.Boton, 0, len(filtrosDeLaFranja))
	for _, f := range filtrosDeLaFranja {
		out = append(out, ui.NuevoBoton(b.T.T(rotuloFiltro[f], nil), func() {
			if b.AlFiltrar == nil {
				return
			}
			if b.Filtro == f {
				b.AlFiltrar(FiltroTodas)
				return
			}
			b.AlFiltrar(f)
		}).Con(ui.Fantasma, ui.TallaXS))
	}
	return out
}

// pintarFiltros pone el activo en `secondary` y el resto en `ghost`, como la web.
func (r *bandejaRenderer) pintarFiltros() {
	for i, f := range filtrosDeLaFranja {
		r.filtros[i].Variante = ui.Fantasma
		if r.b.Filtro == f {
			r.filtros[i].Variante = ui.Secundario
		}
	}
}

// ponerFiltros coloca la franja desde la altura dada y devuelve dónde acaba.
func (r *bandejaRenderer) ponerFiltros(y, ancho float32) float32 {
	x := MargenBusqueda
	for _, f := range r.filtros {
		m := f.MinSize()
		f.Move(fyne.NewPos(x, y+rellenoFiltros))
		f.Resize(m)
		x += m.Width + huecoFiltros
	}
	r.lineaFiltros.Move(fyne.NewPos(0, y+AltoFiltros-tema.Borde))
	r.lineaFiltros.Resize(fyne.NewSize(ancho, tema.Borde))
	return y + AltoFiltros
}

// TopePaginasFiltro son las páginas que un filtro sin coincidencias pide por su cuenta
// antes de parar y ofrecer «Cargar más»: sin tope, «Destacados» en un buzón de decenas
// de miles de correos sin estrella se traería la carpeta entera, página a página.
const TopePaginasFiltro = 5

// ElegirFiltro pone un filtro rápido y vuelve a pintar la lista. Suelta la selección:
// marcar lo que ya no se ve y archivarlo después sería mover a ciegas.
//
// Solo REPINTA la lista si había una: en error, sin red, sin permiso o cargando, el
// estado de la lista es lo que hay que seguir diciendo, y pintar daría por vacía una
// carpeta que no se pudo leer (la web pone la rama de error antes que el vacío).
func (b *Buzon) ElegirFiltro(f FiltroRapido) {
	b.P.Bandeja.Filtro = f
	b.P.Bandeja.Marcadas, b.ancla = map[string]bool{}, -1
	b.cursorAtajo = 0
	b.paginasFiltro = 0
	if e := b.P.Bandeja.Estado; e != Listo && e != Vacio {
		b.P.Bandeja.Refresh()
		return
	}
	b.pintarLista()
}

// seguirFiltrando es «Cargar más» del vacío de un filtro que ya agotó su tope: otra
// tanda de páginas automáticas.
func (b *Buzon) seguirFiltrando() {
	b.paginasFiltro = 0
	b.pintarLista() // vuelve a pedir, contando desde cero
}

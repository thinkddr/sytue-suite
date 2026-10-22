// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"strings"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// Compartir es `ShareLanding`: lo que otra app de la suite manda a Rooms por su esquema
// (`sytue-rooms:/share?…`, `api.RutaCompartir`). Tapa la sala con la pantalla de
// compartir, con el mensaje ya escrito; elegir un canal lo publica y lo abre.
func (sa *Sala) Compartir(c api.Compartir) {
	t := sa.P.T
	p := NuevoCompartir(t, sa.P.Locale, c.Origen)
	r := []rune(c.Texto)
	p.Texto = string(r[:min(len(r), TopeCompartir)])
	p.Estado = Cargando
	var todos []Canal
	filtrar := func(q string) {
		q = buscar.Plegar(strings.TrimSpace(q))
		p.Canales = p.Canales[:0]
		for _, k := range todos {
			if q == "" || strings.Contains(buscar.Plegar(k.Nombre), q) {
				p.Canales = append(p.Canales, k)
			}
		}
		p.Refresh()
	}
	cargar := func() {
		p.Estado = Cargando
		p.Refresh()
		pedir(sa, sa.pedirLista, func(l lista, err error) {
			if err != nil {
				p.Estado = estadoDe(err)
				p.Refresh()
				return
			}
			todos = todos[:0]
			for _, k := range l.canales {
				if !k.Archived {
					todos = append(todos, sa.canalDe(k))
				}
			}
			p.Estado = Listo
			if len(todos) == 0 {
				p.Estado = Vacio
			}
			filtrar("")
		})
	}
	p.AlBuscar, p.AlReintentar = filtrar, cargar
	p.AlCompartir = func(k Canal) {
		texto := strings.TrimSpace(p.Texto)
		if texto == "" {
			return // la lista ya está apagada y lo dice
		}
		p.Enviando = k.ID
		p.Refresh()
		pedir(sa, func(ctx context.Context) (api.RoomsMessage, error) { return sa.S.Post(ctx, k.ID, texto) },
			func(_ api.RoomsMessage, err error) {
				p.Enviando = ""
				p.Refresh()
				if err != nil {
					sa.P.Avisar(ui.AvisoPeligro, t.T("share.failed", nil))
					return
				}
				sa.P.PonerPagina(nil)
				sa.compartir = nil
				if i := slicesIndex(sa.canales, k.ID); i >= 0 {
					sa.Elegir(sa.canalDe(sa.canales[i]))
				} else {
					sa.cargarCanales() //nolint:contextcheck // como en Arrancar: el contexto viaja en sa.ctx
				}
			})
	}
	sa.compartir = p
	sa.P.PonerPagina(p)
	cargar()
}

// slicesIndex es el índice del canal con ese id, o −1.
func slicesIndex(cs []api.RoomsChannel, id string) int {
	for i, c := range cs {
		if c.ID == id {
			return i
		}
	}
	return -1
}

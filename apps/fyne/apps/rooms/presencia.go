// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"slices"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// En la web: `presenceSubs`, `refrescarPresencia` y `dmPeerSub` de ChatApp.tsx.
//
// La presencia de los demás la sabe el Hub. Se pide de golpe la de todos los que tienen
// un directo con quien mira y, con el panel de miembros abierto, la de la gente del
// canal —una petición por cada 200, no una por persona— al cargar los canales, al abrir
// el panel y cada minuto; un fallo conserva lo último que se supo.

// CadaPresencia es el `useSondeo(refrescarPresencia, 60_000)`.
const CadaPresencia = time.Minute

// Presencias es lo que Rooms le pide al Hub; lo cumple `(*api.Hub).Presencias`.
type Presencias func(ctx context.Context, subjects ...string) ([]api.Presencia, error)

// subsDePresencia son los de TODOS los directos, 1:1 y de grupo, y los del roster con el
// panel de miembros abierto, sin repetir.
func (sa *Sala) subsDePresencia() []string {
	var out []string
	for _, c := range sa.canales {
		if c.Direct() {
			out = append(out, c.PeerSubs...)
		}
	}
	if sa.miembros != nil {
		for _, m := range sa.roster {
			out = append(out, m.ID)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// presenciaRoster pone en el roster —y en la gente del equipo del panel— la presencia
// que ya se sabe.
func (sa *Sala) presenciaRoster() {
	poner := func(ms []Miembro) {
		for i := range ms {
			if p, ok := sa.presencias[ms[i].ID]; ok {
				ms[i].Presencia = PresenciaDe(p.Estado)
			}
		}
	}
	poner(sa.roster)
	if sa.miembros != nil {
		poner(sa.miembros.Equipo)
	}
}

// refrescarPresencia pide la presencia de los directos y se vuelve a programar.
func (sa *Sala) refrescarPresencia() {
	if sa.pararPresencia != nil {
		sa.pararPresencia()
		sa.pararPresencia = nil
	}
	subs := sa.subsDePresencia()
	if sa.Presencias == nil || len(subs) == 0 {
		return
	}
	pedir(sa, func(ctx context.Context) ([]api.Presencia, error) { return sa.Presencias(ctx, subs...) },
		func(ps []api.Presencia, err error) {
			if err == nil {
				for _, p := range ps {
					sa.presencias[p.Subject] = p
				}
				sa.pintarCanales()
				if i := slices.IndexFunc(sa.canales, func(c api.RoomsChannel) bool { return c.ID == sa.abierto }); i >= 0 {
					sa.P.Canal.Cabecera.Canal = sa.canalDe(sa.canales[i])
				}
				if p := sa.miembros; p != nil {
					sa.presenciaRoster()
					p.Roster = sa.roster
					p.Refresh()
				}
				sa.P.Refrescar()
			}
			sa.pararPresencia = sa.Esperar(CadaPresencia, sa.refrescarPresencia)
		})
}

// presenciaDe pone en el canal la presencia del otro de un directo 1:1 (`dmPeerSub`:
// por su `sub`, no por el nombre) o cuántos son en uno de grupo.
func (sa *Sala) presenciaDe(c api.RoomsChannel, out *Canal) {
	if !c.Direct() {
		return
	}
	if len(c.PeerSubs) > 1 {
		out.Personas = len(c.PeerSubs) + 1
		return
	}
	if len(c.PeerSubs) == 1 {
		if p, ok := sa.presencias[c.PeerSubs[0]]; ok {
			out.Presencia, out.EstadoPar = PresenciaDe(p.Estado), p.Mensaje
		}
	}
}

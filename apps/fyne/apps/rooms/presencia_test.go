// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// La presencia de los directos se pide de golpe al Hub y cada minuto; el punto va en el
// avatar del carril y de la cabecera, y bajo el nombre sale «Ausente · Comiendo». Un
// directo de grupo dice cuántos son.
//
// VISTO EN ROJO (2026-09-23, en el frontal): casando por NOMBRE y no por `peer_subs`, el
// directo con «Ana Ruiz» se queda sin punto.
func TestLaPresenciaDeLosDirectos(t *testing.T) {
	s := servicioConDos()
	s.canales[1].PeerSubs = []string{"ana"}
	s.canales = append(s.canales, api.RoomsChannel{ID: "g1", Name: "grupo", Kind: "direct", Peers: []string{"B", "C"}, PeerSubs: []string{"b", "c"}})
	sa, relojes := salaDePrueba(t, s)
	var pedidos [][]string
	sa.Presencias = func(_ context.Context, subs ...string) ([]api.Presencia, error) {
		pedidos = append(pedidos, subs)
		return []api.Presencia{{Subject: "ana", Estado: api.Ausente, Mensaje: "Comiendo"}}, nil
	}
	sa.Arrancar(context.Background())
	if len(pedidos) != 1 || !slices.Equal(pedidos[0], []string{"ana", "b", "c"}) {
		t.Fatalf("se piden %v", pedidos)
	}
	if relojes[CadaPresencia] == nil {
		t.Error("no se vuelve a pedir al minuto")
	}
	var d1, g1 Canal
	for _, c := range sa.P.Lateral.Canales {
		switch c.ID {
		case "d1":
			d1 = c
		case "g1":
			g1 = c
		}
	}
	if d1.Presencia != ui.Ausente || d1.EstadoPar != "Comiendo" || g1.Personas != 3 {
		t.Fatalf("el directo sale %+v y el grupo %+v", d1, g1)
	}
	sa.Elegir(d1)
	cab := sa.P.Canal.Cabecera
	if got := cab.lineaDeDebajo(); got != sa.P.T.T("presence.away", nil)+" · Comiendo" {
		t.Errorf("bajo el nombre dice %q", got)
	}
	if a, ok := cab.marca.(*ui.Avatar); !ok || a.Presencia != ui.Ausente {
		t.Errorf("el avatar de la cabecera va sin punto: %T", cab.marca)
	}
}

// Con el panel de miembros abierto, la presencia de la gente del canal se pide al Hub
// al abrirlo —junto a la de los directos— y cada fila la lleva en su avatar; cerrado,
// ya no se pide.
//
// VISTO EN ROJO (2026-09-26, en el frontal, saboteado a mano): sin el roster en
// subsDePresencia, «Grace» sale sin punto.
func TestLaPresenciaEnElPanelDeMiembros(t *testing.T) {
	s := servicioConDos()
	s.canales[1].PeerSubs = []string{"ana"}
	s.roster = []api.RoomsMember{{Subject: "ana", Name: "Ana Ruiz"}, {Subject: "gr", Name: "Grace"}}
	s.roster = append(s.roster, api.RoomsMember{Subject: "yo", Name: "Yo"}, api.RoomsMember{Subject: "be", Name: "Beto"})
	sa, _, w := salaEn(t, s, 1280)
	var pedidos [][]string
	sa.Presencias = func(_ context.Context, subs ...string) ([]api.Presencia, error) {
		pedidos = append(pedidos, subs)
		return []api.Presencia{
			{Subject: "ana", Estado: api.Ausente},
			{Subject: "gr", Estado: api.NoMolestar},
			{Subject: "yo", Estado: api.EnLinea},
			{Subject: "be", Estado: api.Desconectado},
		}, nil
	}
	sa.Arrancar(context.Background())
	sa.AbrirPanel(PanelMiembros)
	if n := len(pedidos); n < 2 || !slices.Equal(pedidos[n-1], []string{"ana", "be", "gr", "yo"}) {
		t.Fatalf("se piden %v", pedidos)
	}
	if dir := os.Getenv("SYTUE_CAPTURA_SALA"); dir != "" {
		f, err := os.Create(filepath.Join(dir, "sala-miembros-presencia-1280.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, w.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	}
	p := sa.miembros
	quien := map[string]ui.Presencia{}
	for _, m := range p.Roster {
		quien[m.ID] = m.Presencia
	}
	if quien["gr"] != ui.NoMolestar || quien["ana"] != ui.Ausente {
		t.Fatalf("el roster lleva %v", quien)
	}
	f := p.fila(Miembro{ID: "gr", Nombre: "Grace", Presencia: ui.NoMolestar}).(*FilaMiembro)
	test.WidgetRenderer(f)
	if f.avatar.Presencia != ui.NoMolestar {
		t.Errorf("el avatar de la fila va con %v", f.avatar.Presencia)
	}
	sa.cerrarDerecho()
	if slices.Contains(sa.subsDePresencia(), "gr") {
		t.Error("con el panel cerrado se sigue pidiendo la del canal")
	}
}

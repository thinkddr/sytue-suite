// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"context"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ia"
)

// TestLasDiapositivasDeLaIASonLasDeLaWeb es `diapositivasDe`: cada título abre una
// lámina, un apartado de más de seis puntos sigue en otra con el mismo título, y el texto
// del modelo entra ESCAPADO (lo único que es marcado es lo que escribe `htmlDeTrozos`).
func TestLasDiapositivasDeLaIASonLasDeLaWeb(t *testing.T) {
	texto := "Intro suelta\n# Plan <b>\n- uno\n- **dos**\n# Largo\n" + strings.Repeat("- p\n", 8)
	ds := DiapositivasDe(ia.BloquesDe(texto))
	if len(ds) != 4 {
		t.Fatalf("salen %d láminas y son cuatro (sin título, Plan, Largo y su continuación)", len(ds))
	}
	if len(ds[0].Titulo) != 0 || htmlDeTrozos(ds[1].Titulo) != "Plan &lt;b&gt;" {
		t.Errorf("títulos: %q y %q", htmlDeTrozos(ds[0].Titulo), htmlDeTrozos(ds[1].Titulo))
	}
	if got := htmlDeCuerpo(ds[1].Cuerpo); got != "<ul><li>uno</li><li><strong>dos</strong></li></ul>" {
		t.Errorf("el cuerpo de «Plan» es %q", got)
	}
	if len(ds[2].Cuerpo) != PuntosPorDiapo || len(ds[3].Cuerpo) != 2 || htmlDeTrozos(ds[3].Titulo) != "Largo" {
		t.Errorf("el apartado largo se parte en %d y %d puntos", len(ds[2].Cuerpo), len(ds[3].Cuerpo))
	}
	if got := htmlDeCuerpo(ds[0].Cuerpo); got != "<p>Intro suelta</p>" {
		t.Errorf("lo que no es lista va en párrafos: %q", got)
	}
}

// TestLaIACreaLaminasDetrasDeLaActual: con Sytue AI detrás, el carril ofrece el
// «Asistente» sobre la presentación, e «Insertar» crea las láminas detrás de la actual
// —la primera, elegida—, con título y cuerpo; la lámina sin título lleva su cuerpo.
//
// VISTO EN ROJO (2026-09-26, en el frontal): llenando la caja única con el título vacío,
// como hace la web, la primera lámina salía vacía y «Intro suelta» se perdía.
func TestLaIACreaLaminasDetrasDeLaActual(t *testing.T) {
	a := NuevaApp(traductor(t), "es", nil, nil)
	a.lanzar = func(f func()) { f() }
	a.Asistir = func(context.Context, api.PeticionIA) (string, error) { return "ok", nil }
	test.NewTempWindow(t, a.Objeto()).Resize(fyne.NewSize(1280, 800))
	a.AbrirContenido("Charla", `{"slides":[{"id":"s0","boxes":[]},{"id":"s9","boxes":[]}]}`, true, func(string) error { return nil })
	ctx, e := a.Contexto(), a.Editor()
	if !ctx.IA || ctx.Paneles == nil || ctx.Paneles(editor.PanelIA) == nil {
		t.Fatal("con Sytue AI, el carril ofrece el «Asistente»")
	}
	if tx, sel := e.ContextoIA(); tx != "" || sel {
		t.Errorf("sin caja abierta no hay contexto: %q", tx)
	}
	if clave := e.AplicarIA("Intro suelta\n# Plan\n- uno\n- dos", false); clave != "ai.slidesAdded" {
		t.Fatalf("«Insertar» devuelve %q", clave)
	}
	m := e.Modelo()
	if len(m.Diapos) != 4 || m.Diapos[0].ID != "s0" || m.Diapos[3].ID != "s9" || e.Activa() != 1 {
		t.Fatalf("hay %d láminas y la activa es la %d", len(m.Diapos), e.Activa())
	}
	if h := m.Diapos[1].Cajas[0].HTML; h != "<p>Intro suelta</p>" {
		t.Errorf("la lámina sin título lleva %q", h)
	}
	if h := m.Diapos[2].Cajas[0].HTML + "|" + m.Diapos[2].Cajas[1].HTML; h != "Plan|<ul><li>uno</li><li>dos</li></ul>" {
		t.Errorf("la lámina de «Plan» lleva %q", h)
	}
	if ctx.Guardado != editor.Sucio {
		t.Error("crear láminas es un cambio que se guarda")
	}
}

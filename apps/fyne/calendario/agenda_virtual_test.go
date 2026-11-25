// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLaAgendaSoloMontaLosDiasQueSeVen: con un mes lleno la agenda monta los días que
// caen en la vista (y uno más por cada lado), mide como si estuvieran todos y monta los
// que entran al desplazar. VISTO EN ROJO con la columna de antes, que los montaba todos.
func TestLaAgendaSoloMontaLosDiasQueSeVen(t *testing.T) {
	p := &Pantalla{
		T: traductor(t), Vista: VistaAgenda, Estado: ui.EstadoExito, Zona: time.UTC,
		Cursor: cursorDeBanco, Ahora: func() time.Time { return cursorDeBanco },
	}
	cals := []api.Calendar{{ID: "c1"}, {ID: "c2"}, {ID: "c3"}}
	p.Eventos = Fundir(cals, nil, instanciasDeBanco(), false)
	ordenar(p.Eventos)
	w := test.NewWindow(p.Construir())
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))
	a, _ := buscarObjeto(w.Content(), func(o fyne.CanvasObject) bool {
		c, ok := o.(*fyne.Container)
		if !ok {
			return false
		}
		_, es := c.Layout.(*agendaVirtual)
		return es
	}).(*fyne.Container)
	if a == nil {
		t.Fatal("la agenda no es virtual")
	}
	v, _ := a.Layout.(*agendaVirtual)
	if n := len(a.Objects); n == 0 || n >= len(v.grupos) {
		t.Fatalf("montados %d días de %d", n, len(v.grupos))
	}
	var alto float32
	for i, g := range v.grupos {
		alto += MedidaGrupoAgenda(len(g.dentro), i > 0)
	}
	if got := a.MinSize().Height; got != alto {
		t.Errorf("la agenda mide %v y con todos los días mediría %v", got, alto)
	}
	v.s.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -alto)})
	if _, ok := v.hechos[len(v.grupos)-1]; !ok {
		t.Error("al bajar hasta el final no se monta el último día")
	}
}

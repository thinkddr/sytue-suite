// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Jueves 12 de noviembre de 2026 a las 10:30 en Madrid: los mismos casos que
// `lib/grupos-fecha.test.ts` de la web.
func jueves(t *testing.T) (time.Time, func(dia, h, m int) time.Time) {
	t.Helper()
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skip("sin zona Europe/Madrid:", err)
	}
	local := func(dia, h, m int) time.Time { return time.Date(2026, 11, dia, h, m, 0, 0, madrid) }
	return local(12, 10, 30), local
}

func TestLosTramosSonDiasDeCalendarioYLaSemanaEmpiezaElLunes(t *testing.T) {
	ahora, local := jueves(t)
	casos := []struct {
		cuando time.Time
		quiero GrupoFecha
	}{
		{local(12, 0, 1), GrupoHoy},
		{local(11, 23, 50), GrupoAyer},
		{local(11, 0, 0), GrupoAyer},
		{local(10, 12, 0), GrupoEstaSemana},
		{local(9, 0, 0), GrupoEstaSemana},
		{local(8, 23, 59), GrupoAntes},
		{local(1, 12, 0), GrupoAntes},
		{local(13, 9, 0), GrupoHoy},
		{time.Time{}, GrupoAntes},
		// El servicio manda UTC: cuenta el instante, no la zona en que llega. Las 23:30
		// UTC del miércoles son ya las 00:30 del jueves en Madrid.
		{time.Date(2026, 11, 11, 23, 30, 0, 0, time.UTC), GrupoHoy},
	}
	for _, c := range casos {
		if got := GrupoDeFecha(c.cuando, ahora); got != c.quiero {
			t.Errorf("%s: tramo %d, quiero %d", c.cuando, got, c.quiero)
		}
	}
	// Pasada la medianoche, lo de hace diez minutos ya es de ayer.
	if got := GrupoDeFecha(local(11, 23, 59), local(12, 0, 9)); got != GrupoAyer {
		t.Errorf("las 23:59 de anoche vistas a las 00:09: tramo %d", got)
	}
	// Un lunes, lo del domingo es «Ayer» y lo del sábado, «Antes».
	lunes := local(9, 9, 0)
	if GrupoDeFecha(local(8, 18, 0), lunes) != GrupoAyer || GrupoDeFecha(local(7, 18, 0), lunes) != GrupoAntes {
		t.Error("un lunes, la semana anterior no es «Esta semana»")
	}
	// Y un domingo, la semana va del lunes al viernes.
	domingo := local(15, 9, 0)
	if GrupoDeFecha(local(9, 8, 0), domingo) != GrupoEstaSemana || GrupoDeFecha(local(8, 23, 0), domingo) != GrupoAntes {
		t.Error("un domingo, el lunes ya no cuenta como «Esta semana»")
	}
}

func TestAgruparConservaElOrdenYOmiteLosTramosVacios(t *testing.T) {
	ahora, local := jueves(t)
	el := func(id string, cuando time.Time) Elemento { return Elemento{ID: id, Modificado: cuando} }
	tramos := AgruparPorFecha([]Elemento{
		el("a", local(12, 9, 0)), el("b", local(11, 12, 0)), el("c", local(1, 12, 0)),
		el("d", local(12, 8, 0)), el("e", time.Time{}),
	}, ahora)
	got := make([]string, 0, len(tramos))
	for _, tr := range tramos {
		got = append(got, strings.Join(append([]string{claveGrupo[tr.Grupo]}, ids(tr.Elementos)...), " "))
	}
	quiero := []string{"recent.group.today a d", "recent.group.yesterday b", "recent.group.earlier c e"}
	if !slices.Equal(got, quiero) {
		t.Errorf("tramos %q, quiero %q", got, quiero)
	}
	if AgruparPorFecha(nil, ahora) != nil {
		t.Error("sin elementos no debe haber tramos")
	}
}

// rotulosDeTramo son los textos de los rótulos de tramo pintados, en orden.
func rotulosDeTramo(o fyne.CanvasObject) []string {
	var out []string
	var andar func(fyne.CanvasObject)
	andar = func(o fyne.CanvasObject) {
		if s, ok := o.(*container.Scroll); ok {
			andar(s.Content)
			return
		}
		c, ok := o.(*fyne.Container)
		if !ok {
			return
		}
		if _, ok := c.Layout.(*enTramo); ok {
			var b strings.Builder
			for _, l := range c.Objects {
				b.WriteString(l.(*canvas.Text).Text)
			}
			out = append(out, b.String())
			return
		}
		for _, h := range c.Objects {
			andar(h)
		}
	}
	andar(o)
	return out
}

// TestRecientesPintaLosRotulosDeSusTramos: «Recientes» lleva un rótulo por tramo con
// algo, en versalitas y en el orden de los tramos; «Destacados», ninguno.
func TestRecientesPintaLosRotulosDeSusTramos(t *testing.T) {
	ahora, local := jueves(t)
	tr := traductor(t)
	els := []Elemento{
		{ID: "f1", Nombre: "acta.md", Tamano: 10, Modificado: local(12, 9, 0), Ruta: "/acta.md"},
		{ID: "f2", Nombre: "plano.pdf", Tamano: 10, Modificado: local(10, 9, 0), Ruta: "/plano.pdf"},
		{ID: "f3", Nombre: "viejo.txt", Tamano: 10, Modificado: local(2, 9, 0), Ruta: "/viejo.txt"},
	}
	for _, s := range []Seccion{Recientes, Favoritos} {
		vp := NuevaVistaPlana(tr, s, map[string]bool{})
		vp.Fase, vp.Ventana, vp.Ahora, vp.Elementos = Exito, 1280, ahora, els
		w := test.NewWindow(vp.Objeto(tema.Claro))
		w.Resize(fyne.NewSize(1020, 744))
		got := rotulosDeTramo(w.Content())
		w.Close()
		var quiero []string
		if s == Recientes {
			for _, k := range []string{"recent.group.today", "recent.group.thisWeek", "recent.group.earlier"} {
				quiero = append(quiero, strings.ToUpper(tr.T(k, nil)))
			}
		}
		if !slices.Equal(got, quiero) {
			t.Errorf("sección %d: rótulos %q, quiero %q", s, got, quiero)
		}
	}
}

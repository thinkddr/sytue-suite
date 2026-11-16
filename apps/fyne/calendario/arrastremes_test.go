// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El arrastre a otro día en el MES (`arrastremes.go`). Vistos en ROJO sin él: la píldora
// y la barra no eran `fyne.Draggable` y no había `AlMover` que llamar.

const anchoMesPrueba, altoMesPrueba = 1280, 800

type movimientoMes struct {
	o        Ocurrencia
	ini, fin time.Time
}

// mesArrastrable monta la rejilla de septiembre de 2026 (arranca el lunes 31 de agosto)
// con sus eventos y apunta lo que se mueve y lo que se abre.
func mesArrastrable(t *testing.T, eventos ...Ocurrencia) (*Mes, *[]movimientoMes, *[]string) {
	t.Helper()
	m := NuevoMes(i18n.Locale("es"), time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC))
	m.Zona = time.UTC
	m.Eventos = eventos
	movidos, abiertos := &[]movimientoMes{}, &[]string{}
	m.AlMover = func(o Ocurrencia, ini, fin time.Time) { *movidos = append(*movidos, movimientoMes{o, ini, fin}) }
	m.AlAbrir = func(o Ocurrencia, _ fyne.CanvasObject) { *abiertos = append(*abiertos, o.Event.ID) }
	w := test.NewWindow(m)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(anchoMesPrueba, altoMesPrueba))
	w.Canvas().Capture()
	return m, movidos, abiertos
}

// centroCelda es el centro de la celda `i` de la rejilla, en coordenadas de la rejilla.
func centroCelda(m *Mes, i int) fyne.Position {
	t := m.Size()
	alto := AltoSemana(t.Height)
	return fyne.NewPos((float32(i%7)+0.5)*t.Width/7, AltoDiasSemana+tema.Borde+(float32(i/7)+0.5)*alto)
}

// arrastrarEnMes pulsa sobre `o` en el punto `desde` de la rejilla y pasa por `pasos`,
// como lo entrega el driver: cada evento trae la posición de ahora, relativa al objeto,
// y lo recorrido desde el anterior. No suelta.
func arrastrarEnMes(o fyne.Draggable, desde fyne.Position, pasos ...fyne.Position) {
	d := fyne.CurrentApp().Driver()
	base := d.AbsolutePositionForObject(o.(fyne.CanvasObject))
	antes := desde
	for _, p := range pasos {
		e := &fyne.DragEvent{Dragged: fyne.NewDelta(p.X-antes.X, p.Y-antes.Y)}
		e.Position = p.Subtract(base)
		o.Dragged(e)
		antes = p
	}
}

func pildoraArrastrableDe(t *testing.T, m *Mes, id string) *pildoraArrastrable {
	t.Helper()
	p, _ := buscarObjeto(m, func(o fyne.CanvasObject) bool {
		a, ok := o.(*pildoraArrastrable)
		return ok && a.o.Event.ID == id
	}).(*pildoraArrastrable)
	if p == nil {
		t.Fatalf("la píldora %q no se puede arrastrar", id)
	}
	return p
}

// TestArrastrarMesLaPildoraDelMartesAlJueves: soltada dos días después, conserva la hora
// y la duración; mientras se arrastra, la celda de destino se resalta.
func TestArrastrarMesLaPildoraDelMartesAlJueves(t *testing.T) {
	ini := time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC) // martes, celda 8
	m, movidos, abiertos := mesArrastrable(t, ocurrencia("d", "Dentista", ini, ini.Add(90*time.Minute), false))
	p := pildoraArrastrableDe(t, m, "d")

	arrastrarEnMes(p, centroCelda(m, 8), centroCelda(m, 9), centroCelda(m, 10))
	r := m.renderer
	if !r.celdas[10].destino || r.celdas[8].destino || r.celdas[9].destino {
		t.Error("mientras se arrastra, solo la celda del jueves debería resaltarse como destino")
	}
	if got, quiero := test.WidgetRenderer(r.celdas[10]).(*celdaRenderer).fondo.FillColor,
		mezclar(tema.Color("surface", tema.Claro), tema.Color("primary", tema.Claro), 0.1); got != quiero {
		t.Errorf("la celda de destino se pinta %v y `bg-primary/10` es %v", got, quiero)
	}
	p.DragEnd()

	if len(*movidos) != 1 {
		t.Fatalf("se guardaron %d movimientos y quiero uno", len(*movidos))
	}
	mv := (*movidos)[0]
	if quiero := time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC); !mv.ini.Equal(quiero) || !mv.fin.Equal(quiero.Add(90*time.Minute)) {
		t.Errorf("se guardó %v–%v y quiero el jueves de 9:00 a 10:30", mv.ini, mv.fin)
	}
	if len(*abiertos) != 0 {
		t.Error("un arrastre no abre la vista rápida")
	}
	for i, c := range m.renderer.celdas {
		if c.destino {
			t.Errorf("tras soltar, la celda %d sigue resaltada", i)
		}
	}
	// Se pinta ya en el jueves mientras se guarda.
	if !m.Eventos[0].Start.Equal(mv.ini) {
		t.Errorf("la píldora sigue en %v mientras se guarda", m.Eventos[0].Start)
	}
}

// TestArrastrarMesSinMoverEsUnClic: un temblor de un píxel no es un arrastre; abre la
// vista rápida, que es lo que habría hecho el `Tapped`.
func TestArrastrarMesSinMoverEsUnClic(t *testing.T) {
	ini := time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC)
	m, movidos, abiertos := mesArrastrable(t, ocurrencia("d", "Dentista", ini, ini.Add(time.Hour), false))
	p := pildoraArrastrableDe(t, m, "d")
	c := centroCelda(m, 8)
	arrastrarEnMes(p, c, c.Add(fyne.NewPos(1, 1)))
	p.DragEnd()
	if len(*movidos) != 0 || len(*abiertos) != 1 {
		t.Errorf("movidos %d, abiertos %v: quiero ninguno movido y el evento abierto", len(*movidos), *abiertos)
	}
}

// TestArrastrarMesUnaSuscripcionNoSeMueve: la píldora de solo lectura no es arrastrable.
func TestArrastrarMesUnaSuscripcionNoSeMueve(t *testing.T) {
	ini := time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC)
	o := ocurrencia("f", "Festivo", ini, ini.Add(time.Hour), false)
	o.SoloLectura = true
	m, _, _ := mesArrastrable(t, o)
	if a := buscarObjeto(m, func(x fyne.CanvasObject) bool { _, ok := x.(fyne.Draggable); return ok }); a != nil {
		t.Errorf("una suscripción no se arrastra y hay %T arrastrable", a)
	}
	if buscarObjeto(m, func(x fyne.CanvasObject) bool { _, ok := x.(*pildora); return ok }) == nil {
		t.Error("la píldora de la suscripción tiene que seguir ahí, pulsable")
	}
}

// TestArrastrarMesUnaBarraCogidaPorSuSegundoDia: el día completo del 8 al 10 cogido por
// el 9 y soltado en el 10 pasa al 9–11, y sigue siendo de día completo (flotante, UTC).
func TestArrastrarMesUnaBarraCogidaPorSuSegundoDia(t *testing.T) {
	m, movidos, _ := mesArrastrable(t, ocurrencia("c", "Congreso",
		time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC), true))
	b, _ := buscarObjeto(m, func(o fyne.CanvasObject) bool { _, ok := o.(*barraArrastrable); return ok }).(*barraArrastrable)
	if b == nil {
		t.Fatal("la barra del congreso no se puede arrastrar")
	}
	arrastrarEnMes(b, centroCelda(m, 9), centroCelda(m, 10))
	n := 0
	for _, c := range m.renderer.celdas {
		if c.destino {
			n++
		}
	}
	if n != 3 {
		t.Errorf("el destino resalta %d celdas y la barra cubre tres días", n)
	}
	b.DragEnd()
	if len(*movidos) != 1 {
		t.Fatalf("se guardaron %d movimientos y quiero uno", len(*movidos))
	}
	mv := (*movidos)[0]
	if !mv.ini.Equal(time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)) || !mv.fin.Equal(time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("se guardó %v–%v y quiero del 9 al 12 (exclusivo) a las 00:00 UTC", mv.ini, mv.fin)
	}
}

// TestArrastrarMesConservaLaHoraConCambioDeHora: del sábado 24 al lunes 26 de octubre
// de 2026 cambia la hora en Madrid; la reunión sigue a las 9:00 de pared.
func TestArrastrarMesConservaLaHoraConCambioDeHora(t *testing.T) {
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skip("sin tzdata")
	}
	ini := time.Date(2026, 10, 24, 9, 0, 0, 0, madrid)
	i, f := DesplazarDias(ocurrencia("r", "Reunión", ini.UTC(), ini.Add(time.Hour).UTC(), false), 2, madrid)
	if quiero := time.Date(2026, 10, 26, 9, 0, 0, 0, madrid); !i.Equal(quiero) || !f.Equal(quiero.Add(time.Hour)) {
		t.Errorf("movida dos días queda %v–%v y quiero las 9:00 del lunes", i, f)
	}
}

// TestArrastrarMesLaPantallaGuardaPorElMismoCamino: la rejilla del mes de la pantalla
// guarda por `AlMoverEvento`, el de la rejilla horaria (`App.mover`: alcance de serie,
// «Deshacer»).
func TestArrastrarMesLaPantallaGuardaPorElMismoCamino(t *testing.T) {
	p := pantallaDeMuestra(t)
	var llamado bool
	p.AlMoverEvento = func(Ocurrencia, time.Time, time.Time) { llamado = true }
	w := test.NewWindow(p.Construir())
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(anchoMesPrueba, altoMesPrueba))
	m, _ := buscarObjeto(w.Content(), func(o fyne.CanvasObject) bool { _, ok := o.(*Mes); return ok }).(*Mes)
	if m == nil || m.AlMover == nil {
		t.Fatal("la rejilla del mes no recibe con qué guardar un arrastre")
	}
	m.AlMover(Ocurrencia{}, time.Time{}, time.Time{})
	if !llamado {
		t.Error("el arrastre del mes no llega a `AlMoverEvento`")
	}
}

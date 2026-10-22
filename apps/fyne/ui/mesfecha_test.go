// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestElCalendarioEscribeLaFecha: el mes empieza en lunes, pulsar un día lo escribe en el
// campo como `LeerFecha` lo lee, las flechas cambian de mes, y el icono del campo lo
// abre (el resto del campo sigue siendo para teclear).
//
// VISTO EN ROJO (2026-09-24, en el frontal): con `(Weekday()+7)%7` la rejilla de octubre
// de 2026 empezaba en domingo (27 de septiembre) y no en lunes.
func TestElCalendarioEscribeLaFecha(t *testing.T) {
	m := NuevoMesFecha("es", time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), time.UTC)
	dias := m.Dias()
	if dias[0].Weekday() != time.Monday || dias[3] != time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("la rejilla empieza en %v y el día 1 cae en %v", dias[0], dias[3])
	}
	m.Desplazar(1)
	if m.Mes.Month() != time.November {
		t.Errorf("la flecha lleva a %v", m.Mes)
	}

	e := CampoFecha(CampoMD)
	e.Ancho = 200
	ConCalendario(e, "es", time.UTC)
	w := test.NewWindow(container.NewWithoutLayout(e))
	defer w.Close()
	w.Resize(fyne.NewSize(400, 500))
	e.Resize(e.MinSize())
	e.PonerTexto("15/10/2026")
	e.Tapped(&fyne.PointEvent{Position: fyne.NewPos(e.Size().Width-5, 5)})
	if w.Canvas().Overlays().Top() != nil {
		t.Error("pulsar el texto abre el calendario: tiene que abrirlo el icono")
	}
	e.Tapped(&fyne.PointEvent{Position: fyne.NewPos(12, 12)})
	if w.Canvas().Overlays().Top() == nil {
		t.Fatal("el icono no abre el calendario")
	}
	var dia *diaFecha
	objs := w.Canvas().Overlays().Top().(fyne.Widget)
	var buscar func(o fyne.CanvasObject)
	buscar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *diaFecha:
			if x.dia.Day() == 20 && x.dia.Month() == time.October {
				dia = x
			}
		case *fyne.Container:
			for _, h := range x.Objects {
				buscar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				buscar(h)
			}
		}
	}
	buscar(objs)
	if dia == nil {
		t.Fatal("no está el 20 de octubre en el calendario de la fecha escrita")
	}
	dia.Tapped(nil)
	if e.Texto() != "20/10/2026" {
		t.Errorf("elegir el 20 escribe %q", e.Texto())
	}
	w.Canvas().Capture()
}

// TestElCalendarioSePintaEnLosDosTemas: un token mal escrito entra en pánico AL PINTAR.
func TestElCalendarioSePintaEnLosDosTemas(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, nombre := range []string{"claro", "oscuro"} {
		tema.Elegir(nombre)
		m := NuevoMesFecha("es", time.Now(), time.Local)
		w := test.NewWindow(m.Objeto())
		w.Resize(fyne.NewSize(240, 260))
		w.Canvas().Capture()
		w.Close()
	}
}

// La semana de Notes cruza mes/año con días civiles y todos sus destinos llegan
// con ratón o teclado al mismo manejador del calendario compartido.
func TestSemanaFechaCruzaElAnoYRespetaInerte(t *testing.T) {
	elegida := time.Date(2026, 12, 31, 12, 0, 0, 0, time.Local)
	var recibido time.Time
	semana := NuevaSemanaFecha("es", elegida, elegida, false, func(d time.Time) { recibido = d }, nil)
	if len(semana.Objects) != 7 {
		t.Fatal("no hay siete días")
	}
	for i, o := range semana.Objects {
		c := o.(*diaFecha)
		esperado := elegida.AddDate(0, 0, i-3)
		if c.Nombre != i18n.FormatFullDate("es", esperado) {
			t.Fatalf("destino %d: %s", i, c.Nombre)
		}
		c.Tapped(nil)
		if !recibido.Equal(esperado) {
			t.Fatalf("pulsar el día %d no navega", i)
		}
	}
	bloqueada := NuevaSemanaFecha("es", elegida, elegida, true, func(time.Time) { t.Fatal("navegó estando inerte") }, nil)
	bloqueada.Objects[0].(*diaFecha).Tapped(nil)
}

// Los puntos de la semana: cada día lleva la marca que se le da y la anuncia en su nombre.
func TestSemanaFechaLlevaSusMarcas(t *testing.T) {
	elegida := time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local)
	semana := NuevaSemanaFecha("es", elegida, elegida, false, nil, func(d time.Time) (int, string) {
		if d.Day() == 30 {
			return 2, "pendientes"
		}
		return 0, ""
	})
	for i, o := range semana.Objects {
		c := o.(*diaFecha)
		quiero := 0
		if i == 2 {
			quiero = 2
		}
		if c.marca != quiero || (quiero > 0) != strings.HasSuffix(c.Nombre, " · pendientes") {
			t.Fatalf("día %d: marca %d, nombre %q", i, c.marca, c.Nombre)
		}
	}
}

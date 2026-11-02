// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"strconv"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La captura de iPhone a393 enseñaba «LUN21MAR22MIÉ23…»: las letras de cada día
// invadían el siguiente. Se comprueban las cajas de los objetos ya pintados, tanto
// al estrechar como al volver al mismo ancho, con abreviaturas de distintas longitudes.
func TestLaCabeceraSemanalMantieneCadaDiaEnSuColumnaAlGirar(t *testing.T) {
	t.Cleanup(ui.TactilDePrueba(true))
	for _, locale := range []i18n.Locale{"es", "lv", "pt", "fi"} {
		t.Run(string(locale), func(t *testing.T) {
			tr, err := i18n.New(locale, i18n.Common(), catalogos.Calendar())
			if err != nil {
				t.Fatal(err)
			}
			p := pantallaDeMuestra(t)
			p.T, p.Vista = tr, VistaSemana
			comite := p.Cursor.AddDate(0, 0, 2).Add(-2 * time.Hour)
			p.Eventos = []Ocurrencia{
				ocurrencia("vacaciones", "Vacaciones",
					p.Cursor.AddDate(0, 0, 2), p.Cursor.AddDate(0, 0, 5), true),
				ocurrencia("comite", "Comité", comite, comite.Add(time.Hour), false),
			}
			var creado, fin time.Time
			p.AlCrearRango = func(a, b time.Time) { creado, fin = a, b }
			vista := p.vistaHoraria().(*fyne.Container)
			reparto := vista.Objects[1].(*fyne.Container)
			cab := reparto.Layout.(*bajoCabeceraHoraria).cab
			w := test.NewWindow(vista)
			defer w.Close()
			w.SetPadded(false)
			for _, ancho := range []float32{393, 720, 1024, 320, 393} {
				w.Resize(fyne.NewSize(ancho, 700))
				w.Canvas().Capture()
				r := test.WidgetRenderer(cab).(*cabeceraHorariaRenderer)
				comprobarDiasDentroDeColumnas(t, r)
				if fin := r.fileteFranja.Position().Y + r.fileteFranja.Size().Height; !cerca(fin, cab.Size().Height, 0.1) {
					t.Errorf("a%g la franja acaba en%g fuera de la cabecera de%g", ancho, fin, cab.Size().Height)
				}
				cuerpo := reparto.Objects[1]
				if !cerca(cuerpo.Position().Y, cab.Size().Height, 0.1) ||
					cuerpo.Position().Y+cuerpo.Size().Height > reparto.Size().Height+0.1 {
					t.Errorf("a%g el cuerpo no sigue a la cabecera: cabecera=%v, cuerpo=%v/%v", ancho,
						cab.Size(), cuerpo.Position(), cuerpo.Size())
				}
				if r.iman.Size().Width < 44 || r.iman.Size().Height < 44 || r.iman.Position().Y < 0 ||
					r.iman.Position().Y+r.iman.Size().Height > r.filete.Position().Y+0.1 {
					t.Errorf("a%g el conmutador no conserva sus44 completos: %v/%v", ancho, r.iman.Position(), r.iman.Size())
				}
				test.Tap(r.iman)
				if p.paso() != PasoIman30 {
					t.Fatal("el conmutador ha perdido su acción al girar")
				}
				test.Tap(r.iman)
				// Pulsar la línea PINTADA de las10 crea a las10 también cuando la
				// cabecera cambia de alto. La hora del evento visible queda en ella.
				c := p.cuerpoHorario
				abs := fyne.CurrentApp().Driver().AbsolutePositionForObject(c)
				martes := r.cols[1].fondo
				linea10 := c.renderer.lineas[10].Position().Y
				test.TapCanvas(w.Canvas(), abs.Add(fyne.NewPos(martes.Position().X+martes.Size().Width/2, linea10)))
				quiero := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
				if !creado.Equal(quiero) || !fin.Equal(quiero.Add(time.Hour)) {
					t.Errorf("a%g pulsar las10 crea%v..%v", ancho, creado, fin)
				}
				if bloque := c.renderer.bloques[0]; !cerca(bloque.Position().Y, linea10, 0.1) {
					t.Errorf("a%g el comité de las10 se separa de su línea horaria", ancho)
				}
			}
		})
	}
}

func comprobarDiasDentroDeColumnas(t *testing.T, r *cabeceraHorariaRenderer) {
	t.Helper()
	for i, col := range r.cols {
		izq, der := col.fondo.Position().X, col.fondo.Position().X+col.fondo.Size().Width
		letras := 0
		for _, o := range append(col.semana.Objetos(), col.inicial.Objetos()...) {
			if o.Visible() {
				letras++
				comprobarRotuloEnColumna(t, o, izq, der, r.filete.Position().Y)
			}
		}
		if letras == 0 || !col.numero.Visible() || col.numero.Text != strconv.Itoa(r.c.d.dias[i].Day()) {
			t.Errorf("el día%d ha perdido su nombre o número", i)
		}
		comprobarRotuloEnColumna(t, col.numero, izq, der, r.filete.Position().Y)
	}
}

func comprobarRotuloEnColumna(t *testing.T, o fyne.CanvasObject, izq, der, abajo float32) {
	t.Helper()
	p, s := o.Position(), o.Size()
	if p.X < izq-0.1 || p.X+s.Width > der+0.1 || p.Y < 0 || p.Y+s.Height > abajo+0.1 {
		t.Errorf("el rótulo%T en%v/%v sale de su columna X%g..%g Y0..%g", o, p, s, izq, der, abajo)
	}
}

func TestLaCabeceraDeUnSoloDiaConservaLaAbreviaturaCompleta(t *testing.T) {
	t.Cleanup(ui.TactilDePrueba(true))
	p := pantallaDeMuestra(t)
	p.Vista = VistaDia
	p.Cursor = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	cab := nuevaCabeceraHoraria(p, p.disponerHorario(p.diasHorarios()))
	cab.Resize(fyne.NewSize(393, cab.AltoPara(393)))
	r := test.WidgetRenderer(cab).(*cabeceraHorariaRenderer)
	comprobarDiasDentroDeColumnas(t, r)
	if r.cols[0].inicial.Objetos()[0].Visible() {
		t.Error("un día con sitio pierde su abreviatura de escritorio")
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// masDia es el «+N más» de la celda del mes: el `<button>` de CalendarGrid.tsx que
// despliega en un Popover todos los eventos del día. Antes era un texto pintado sobre la
// celda, y pulsarlo creaba un evento (el clic caía a la celda). Al ser un widget propio
// recibe él la pulsación, y Fyne no se la pasa a la celda de debajo.
//
// Puntero, pulsación, teclado y rol de botón son los de `ui.Pulsable`.
type masDia struct {
	ui.Pulsable

	rotulo string
}

var _ fyne.Widget = (*masDia)(nil)

func nuevoMasDia(m *Mes, dia time.Time, rotulo string) *masDia {
	b := &masDia{rotulo: rotulo}
	b.AlPulsar = func() {
		if m.AlMas != nil {
			m.AlMas(dia, b) // «+N más» es el ancla del Popover
		}
	}
	b.ExtendBaseWidget(b)
	return b
}

// AccessibilityLabel es el texto visible, como el del botón de la web.
func (b *masDia) AccessibilityLabel() string { return b.rotulo }

func (b *masDia) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &masDiaRenderer{
		b:     b,
		caja:  ui.NuevaCaja(tema.Color("surface-sunken", v), tema.Color("surface-sunken", v), tema.RadioSM),
		texto: ui.Texto(b.rotulo, tema.TextoXS, ui.Medio, tema.Color("text-muted", v)),
		// Abierto: en reposo no hay caja que tape el centro de un aro relleno.
		aro: ui.NuevoAro(tema.RadioSM, 0, v).Abierto(),
	}
	r.objetos = append(append(r.caja.Objetos(), r.texto), r.aro.Objetos()...)
	r.aro.Ver(false)
	r.Refresh()
	return r
}

type masDiaRenderer struct {
	b       *masDia
	caja    *ui.Caja
	texto   *canvas.Text
	aro     *ui.Aro
	objetos []fyne.CanvasObject
}

// AltoMasDia es el alto del «+N más»: `min-h-[max(24px,1.5rem)] py-1`, el objetivo de
// WCAG 2.5.8, como las píldoras. Hasta el 2026-11-25 medía su línea de `text-xs` (16).
var AltoMasDia = max(24, 2*espacio1+tema.TextoXS.Linea)

// MinSize: `px-1.5 py-1 text-xs`, de AltoMasDia de alto.
func (r *masDiaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(2*espacio1p5+ui.Medir(r.b.rotulo, tema.TextoXS, ui.Medio).Width, AltoMasDia)
}
func (r *masDiaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *masDiaRenderer) Destroy()                     {}

func (r *masDiaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	ui.Truncar(r.texto, r.b.rotulo, tema.TextoXS, ui.Medio, max(t.Width-2*espacio1p5, 0))
	ui.ColocarTexto(r.texto, tema.TextoXS, espacio1p5, (t.Height-tema.TextoXS.Linea)/2)
	r.aro.Colocar(t)
}

// Refresh pinta `hover:bg-surface-sunken hover:text-text`; en reposo, sin caja.
func (r *masDiaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if r.b.Encima() {
		r.caja.Pintar(tema.Color("surface-sunken", v), tema.Color("surface-sunken", v))
		r.texto.Color = tema.Color("text", v)
	} else {
		r.caja.Pintar(color.NRGBA{}, color.NRGBA{})
		r.texto.Color = tema.Color("text-muted", v)
	}
	r.texto.Refresh()
	r.aro.Ver(r.b.Enfocado())
}

// AnchoListaDia es el `w-72` de la lista que despliega «+N más».
var AnchoListaDia float32 = 18 * tema.Rem

// mostrarDiaMes es el Popover de «+N más»: la fecha y TODOS los eventos del día con la
// fila de la agenda. Pulsar una fila cierra la lista y abre su vista rápida, anclada al
// «+N más» (la fila ya no está).
func (p *Pantalla) mostrarDiaMes(dia time.Time, ancla fyne.CanvasObject) *ui.Emergente {
	fecha := i18n.FormatDayMonthLong(p.locale(), dia)
	dias := EventosDelDia(dia, p.zona(), p.Eventos)
	filas := make([]fyne.CanvasObject, 0, 1+len(dias))
	// Alineada con el texto de las filas, como el `-mx-2` de la lista en la web.
	filas = append(filas, renglon(fecha, tema.TextoSM, ui.SemiNegrita, "text", ui.Margen{Izq: tema.Borde + espacio2, Abj: espacio2}))
	var e *ui.Emergente
	for _, f := range dias {
		fila := p.nuevaFilaAgenda(*f.O)
		fila.alAbrir = func(o Ocurrencia, _ fyne.CanvasObject) {
			if e != nil {
				e.Cerrar()
			}
			if p.AlAbrirEvento != nil {
				p.AlAbrirEvento(o, ancla)
			}
		}
		filas = append(filas, fila)
	}
	lista := ui.AlAncho(AnchoListaDia, container.New(ui.Columna{Hueco: espacio0p5}, filas...))
	e = ui.MostrarPopover(ancla, lista)
	e.Rotulo = p.t("grid.dayEvents", i18n.Vars{"date": fecha, "count": len(dias)})
	return e
}

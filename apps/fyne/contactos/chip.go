// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La etiqueta de un contacto en su ficha (`GroupChips` de ContactDetail.tsx): una
// píldora con el nombre y, si se puede escribir, el aspa que la quita.
//
//	píldora `inline-flex gap-1 rounded-full border bg-bg-subtle py-0.5 pr-1 pl-2.5` hoja
//	nombre  `text-sm text-text max-w-48 truncate`                                  hoja
//	aspa    `h-5 w-5 rounded-full text-text-muted`, al pasar `bg-border text-text`  hoja
const (
	// LadoAspaChip es el `h-5 w-5` del aspa y GlifoAspaChip su `<Icon size={12}>`.
	LadoAspaChip  = 5 * 0.25 * tema.Rem
	GlifoAspaChip = float32(12)
	// MaxNombreChip es el `max-w-48` del nombre.
	MaxNombreChip = 48 * 0.25 * tema.Rem
)

const (
	izqChip   = 2.5 * 0.25 * tema.Rem
	derChip   = 0.25 * tema.Rem
	vertChip  = 0.5 * 0.25 * tema.Rem
	huecoChip = 0.25 * tema.Rem
)

// MedidaChip es lo que ocupa una píldora: el `py-0.5` y el borde alrededor de lo más
// alto de dentro, que es el aspa (17,5) si la hay y la línea de `text-sm` si no.
func MedidaChip(nombre string, conAspa bool) fyne.Size {
	ancho := izqChip + min(ui.Medir(nombre, tema.TextoSM, ui.Normal).Width, MaxNombreChip) + derChip + 2*tema.Borde
	alto := tema.TextoSM.Linea
	if conAspa {
		ancho += huecoChip + LadoAspaChip
		alto = max(alto, LadoAspaChip)
	}
	return fyne.NewSize(ancho, alto+2*vertChip+2*tema.Borde)
}

// nuevoChip arma la píldora. `alQuitar` nil: sin aspa, que es lo que la web pinta sin
// red.
func nuevoChip(nombre, rotuloQuitar string, alQuitar func()) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	caja := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), 999)
	texto := ui.Texto(nombre, tema.TextoSM, ui.Normal, tema.Color("text", v))
	objs := append(caja.Objetos(), texto)
	l := &chipLayout{caja: caja, texto: texto, nombre: nombre}
	if alQuitar != nil {
		l.aspa = nuevaAspa(rotuloQuitar, alQuitar)
		objs = append(objs, l.aspa)
	}
	return container.New(l, objs...)
}

type chipLayout struct {
	caja   *ui.Caja
	texto  *canvas.Text
	nombre string
	aspa   *aspaChip
}

func (l *chipLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return MedidaChip(l.nombre, l.aspa != nil)
}

func (l *chipLayout) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	l.caja.Redimensionar(t)
	x := tema.Borde + izqChip
	ui.Truncar(l.texto, l.nombre, tema.TextoSM, ui.Normal, MaxNombreChip)
	ui.ColocarTexto(l.texto, tema.TextoSM, x, (t.Height-tema.TextoSM.Linea)/2)
	if l.aspa != nil {
		l.aspa.Move(fyne.NewPos(t.Width-tema.Borde-derChip-LadoAspaChip, (t.Height-LadoAspaChip)/2))
		l.aspa.Resize(fyne.NewSize(LadoAspaChip, LadoAspaChip))
	}
}

// aspaChip es el `<button>` que quita la etiqueta, con la esquina compartida.
type aspaChip struct{ ui.Pulsable }

func nuevaAspa(rotulo string, alPulsar func()) *aspaChip {
	a := &aspaChip{}
	a.Nombre, a.AlPulsar = rotulo, alPulsar
	a.ExtendBaseWidget(a)
	return a
}

func (a *aspaChip) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &aspaRenderer{
		a:     a,
		fondo: canvas.NewRectangle(color.NRGBA{}),
		glifo: iconos.Trazo(iconos.Cerrar, GlifoAspaChip, "text-muted", v),
		aro:   ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v),
	}
	r.Refresh()
	return r
}

type aspaRenderer struct {
	a     *aspaChip
	fondo *canvas.Rectangle
	glifo *iconos.Dibujo
	aro   *ui.Aro
}

func (r *aspaRenderer) MinSize() fyne.Size { return fyne.NewSize(LadoAspaChip, LadoAspaChip) }
func (r *aspaRenderer) Objects() []fyne.CanvasObject {
	return r.aro.Antes(r.fondo, r.glifo.Objeto())
}
func (r *aspaRenderer) Destroy() {}

func (r *aspaRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	r.aro.Colocar(t)
	r.glifo.Colocar((t.Width-GlifoAspaChip)/2, (t.Height-GlifoAspaChip)/2, GlifoAspaChip)
}

func (r *aspaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = color.NRGBA{}
	radio := tema.Forma("radius-control")
	r.fondo.CornerRadius = radio
	r.aro.CambiarRadio(radio)
	r.glifo.Tenir("text-muted", v)
	if r.a.Encima() {
		r.fondo.FillColor = tema.Color("border", v)
		r.glifo.Tenir("text", v)
	}
	r.aro.Pintar(v)
	r.aro.Ver(r.a.Enfocado())
	r.fondo.Refresh()
	r.glifo.Objeto().Refresh()
}

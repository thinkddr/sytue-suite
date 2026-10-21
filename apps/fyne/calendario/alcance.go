// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// DialogoAlcance es ScopeDialog.tsx: al editar, mover o borrar una ocurrencia de una
// serie, «solo este», «este y los siguientes» o «todos». Cada apertura arranca en
// «solo este», la opción menos destructiva.
//
//	Modal sm · fieldset `flex-col gap-1` · legend `text-muted mb-2 text-sm`
//	Radio `hover:bg-bg-subtle w-full rounded-md px-2 py-2 text-sm`
//	pie: Cancelar (secondary) · Guardar (primary) o Eliminar (danger)
func (p *Pantalla) DialogoAlcance(borrar bool, alConfirmar func(alcance string), alCancelar func()) *ui.Dialogo {
	elegido := AlcanceEste
	filas := make([]*filaAlcance, 0, 3)
	for _, o := range []struct{ alcance, clave string }{
		{AlcanceEste, "scope.this"}, {AlcanceSiguientes, "scope.following"}, {AlcanceTodos, "scope.all"},
	} {
		f := nuevaFilaAlcance(p.t(o.clave, nil), o.alcance == elegido)
		alcance := o.alcance
		f.alElegir = func() {
			elegido = alcance
			for _, g := range filas {
				g.radio.Marcada = g == f
				g.radio.Refresh()
			}
		}
		filas = append(filas, f)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	leyenda := ui.NuevoParrafo(p.t("scope.legend", nil), tema.TextoSM, ui.Normal, "text-muted", v)
	cuerpo := make([]fyne.CanvasObject, 0, 1+len(filas))
	cuerpo = append(cuerpo, container.New(&margenAbajo{Abj: espacio2}, leyenda))
	for _, f := range filas {
		cuerpo = append(cuerpo, f)
	}

	titulo, rotulo, variante := "scope.editTitle", "common.save", ui.Primario
	if borrar {
		titulo, rotulo, variante = "scope.deleteTitle", "common.delete", ui.Peligro
	}
	var d *ui.Dialogo
	cancelar := ui.NuevoBoton(p.t("common.cancel", nil), func() { d.Cerrar(); alCancelar() }).Con(ui.Secundario, ui.TallaMD)
	confirmar := ui.NuevoBoton(p.t(rotulo, nil), func() { d.Cerrar(); alConfirmar(elegido) }).Con(variante, ui.TallaMD)
	d = ui.NuevoDialogo(p.t(titulo, nil), container.New(ui.Columna{Hueco: tema.Espacio(1)}, cuerpo...)).
		Con(ui.DialogoSM, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, confirmar))
	d.RotuloCerrar = p.t("common.close", nil)
	d.AlCerrar = func() { d.Cerrar(); alCancelar() }
	return d
}

// margenAbajo es un `mb-*` sobre un único hijo.
type margenAbajo struct{ Abj float32 }

func (m *margenAbajo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return objs[0].MinSize().AddWidthHeight(0, m.Abj)
}

func (m *margenAbajo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, max(t.Height-m.Abj, 0)))
}

// filaAlcance es un `<Radio>` de ScopeDialog: el radio del catálogo, su etiqueta en
// `text-sm` y la fila entera pulsable, con el `hover:bg-bg-subtle`.
type filaAlcance struct {
	widget.BaseWidget
	etiqueta string
	radio    *ui.Eleccion
	alElegir func()
	encima   bool
}

var (
	_ fyne.Tappable     = (*filaAlcance)(nil)
	_ desktop.Hoverable = (*filaAlcance)(nil)
)

func nuevaFilaAlcance(etiqueta string, marcada bool) *filaAlcance {
	f := &filaAlcance{etiqueta: etiqueta}
	f.radio = ui.NuevoRadio(etiqueta, func(bool) { f.Tapped(nil) })
	f.radio.SinEtiqueta, f.radio.Marcada = true, marcada
	f.ExtendBaseWidget(f)
	return f
}

func (f *filaAlcance) Tapped(*fyne.PointEvent) {
	if f.alElegir != nil {
		f.alElegir()
	}
}
func (f *filaAlcance) MouseIn(*desktop.MouseEvent)    { f.encima = true; f.Refresh() }
func (f *filaAlcance) MouseMoved(*desktop.MouseEvent) {}
func (f *filaAlcance) MouseOut()                      { f.encima = false; f.Refresh() }

// MinSize: `py-2` alrededor de lo más alto entre el radio y la línea de `text-sm`.
func (f *filaAlcance) MinSize() fyne.Size {
	return fyne.NewSize(0, 2*espacio2+max(ui.LadoEleccion, tema.TextoSM.Linea))
}

func (f *filaAlcance) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaAlcanceRenderer{
		f:     f,
		fondo: canvas.NewRectangle(color.NRGBA{}),
		texto: ui.Texto(f.etiqueta, tema.TextoSM, ui.Normal, tema.Color("text", v)),
	}
	r.fondo.CornerRadius = tema.RadioMD
	r.Refresh()
	return r
}

type filaAlcanceRenderer struct {
	f     *filaAlcance
	fondo *canvas.Rectangle
	texto *canvas.Text
}

func (r *filaAlcanceRenderer) MinSize() fyne.Size { return r.f.MinSize() }
func (r *filaAlcanceRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fondo, r.f.radio, r.texto}
}
func (r *filaAlcanceRenderer) Destroy() {}

func (r *filaAlcanceRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	lado := r.f.radio.MinSize()
	r.f.radio.Move(fyne.NewPos(espacio2, (t.Height-lado.Height)/2))
	r.f.radio.Resize(lado)
	ui.ColocarTexto(r.texto, tema.TextoSM, espacio2+lado.Width+ui.HuecoEleccion, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *filaAlcanceRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = color.NRGBA{}
	if r.f.encima {
		r.fondo.FillColor = tema.Color("bg-subtle", v)
	}
	r.fondo.Refresh()
	r.texto.Color = tema.Color("text", v)
	r.texto.Refresh()
	r.f.radio.Refresh()
}

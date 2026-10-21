// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/ReadOnlyBar.tsx

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// FranjaSoloLectura es `ReadOnlyBar.tsx`: el aviso de solo lectura que va entre la
// cinta y el lienzo, con el botón que lo desbloquea.
//
// Va ahí y no en un diálogo porque sin ella quien abre algo que no puede editar pulsa
// negrita, no pasa nada y concluye que la aplicación está rota. Y lleva botón porque un
// aviso que solo informa deja a la persona sin salida (petición del dueño, 2026-09-25):
// «Editar» si el bloqueo es suyo, «Pedir acceso de edición» si le falta el permiso. Qué
// botón es lo decide la app, que es quien sabe de dónde sale su permiso; enviada la
// petición, la app pone `Nota` («Petición enviada») y el botón desaparece.
//
// `flex items-center gap-x-3 border-b bg-bg-subtle px-4 py-1 text-sm text-text-muted`,
// con el candado de 14, el texto que se estira y los botones `xs` a la derecha.
//
// ponytail: una sola línea; la web salta de línea con `flex-wrap` y aquí el texto se
// recorta con puntos suspensivos. Basta en el ancho de un editor de escritorio.
type FranjaSoloLectura struct {
	widget.BaseWidget

	// Texto es por qué es de solo lectura, YA traducido.
	Texto string
	// Nota sustituye a los botones cuando ya no hay nada que pulsar.
	Nota string
	// Accion es el botón que la desbloquea; Secundaria, una segunda salida discreta
	// («Rechazar» en la franja del dueño). Los crea NuevaFranjaSoloLectura.Con.
	Accion, Secundaria *Boton
}

var (
	_ fyne.Widget     = (*FranjaSoloLectura)(nil)
	_ fyne.Accessible = (*FranjaSoloLectura)(nil)
)

var (
	rellenoFranjaX = tema.Espacio(4) // px-4
	rellenoFranjaY = tema.Espacio(1) // py-1
	huecoFranja    = tema.Espacio(3) // gap-x-3
	ladoCandado    = float32(14)     // <Icon name="lock" size={14} />
)

// NuevaFranjaSoloLectura crea el aviso con el texto YA traducido y sin botón.
func NuevaFranjaSoloLectura(texto string) *FranjaSoloLectura {
	f := &FranjaSoloLectura{Texto: texto}
	f.ExtendBaseWidget(f)
	return f
}

// Con le pone el botón principal (variante secundaria, talla xs) y devuelve la franja.
func (f *FranjaSoloLectura) Con(rotulo string, alPulsar func()) *FranjaSoloLectura {
	f.Accion = NuevoBoton(rotulo, alPulsar).Con(Secundario, TallaXS)
	return f
}

// ConSecundaria le pone la segunda salida (fantasma, xs), que va delante de la principal.
func (f *FranjaSoloLectura) ConSecundaria(rotulo string, alPulsar func()) *FranjaSoloLectura {
	f.Secundaria = NuevoBoton(rotulo, alPulsar).Con(Fantasma, TallaXS)
	return f
}

// AccessibilityRole la anuncia como texto (`role="status"`); lo que se pulsa son sus botones.
func (f *FranjaSoloLectura) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleText
}

// AccessibilityLabel es el propio aviso.
func (f *FranjaSoloLectura) AccessibilityLabel() string { return f.Texto }

// botones son los que se pintan: ninguno si hay nota.
func (f *FranjaSoloLectura) botones() []*Boton {
	if f.Nota != "" {
		return nil
	}
	var bs []*Boton
	for _, b := range []*Boton{f.Secundaria, f.Accion} {
		if b != nil {
			bs = append(bs, b)
		}
	}
	return bs
}

// MedidaFranjaSoloLectura es su alto: el `py-1` de los dos lados, lo más alto que lleva
// dentro —el botón `xs` si hay botón, la línea de `text-sm` si no— y el `border-b`.
func MedidaFranjaSoloLectura(f *FranjaSoloLectura) fyne.Size {
	dentro := tema.TextoSM.Linea
	if len(f.botones()) > 0 {
		dentro = max(dentro, medidas[TallaXS].alto)
	}
	return fyne.NewSize(0, 2*rellenoFranjaY+dentro+tema.Borde)
}

// MinSize es la medida declarada.
func (f *FranjaSoloLectura) MinSize() fyne.Size {
	f.ExtendBaseWidget(f)
	return MedidaFranjaSoloLectura(f)
}

// CreateRenderer compone el fondo, su filete, el candado, el texto y los botones.
func (f *FranjaSoloLectura) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &franjaSoloLecturaRenderer{
		f:      f,
		fondo:  canvas.NewRectangle(tema.Color("bg-subtle", v)),
		filete: canvas.NewRectangle(tema.Color("border", v)),
		glifo:  iconos.Trazo(iconos.Candado, ladoCandado, "text-muted", v),
		texto:  Texto(f.Texto, tema.TextoSM, Normal, tema.Color("text-muted", v)),
		nota:   Texto(f.Nota, tema.TextoSM, Medio, tema.Color("text", v)),
	}
	r.Refresh()
	return r
}

type franjaSoloLecturaRenderer struct {
	f             *FranjaSoloLectura
	fondo, filete *canvas.Rectangle
	glifo         *iconos.Dibujo
	texto, nota   *canvas.Text
}

func (r *franjaSoloLecturaRenderer) MinSize() fyne.Size { return MedidaFranjaSoloLectura(r.f) }
func (r *franjaSoloLecturaRenderer) Destroy()           {}

func (r *franjaSoloLecturaRenderer) Objects() []fyne.CanvasObject {
	bs := r.f.botones()
	objs := make([]fyne.CanvasObject, 0, 5+len(bs))
	objs = append(objs, r.fondo, r.filete, r.glifo.Objeto(), r.texto, r.nota)
	for _, b := range bs {
		objs = append(objs, b)
	}
	return objs
}

// Layout: candado y texto a la izquierda, los botones (o la nota) empujados a la derecha.
func (r *franjaSoloLecturaRenderer) Layout(t fyne.Size) {
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	r.filete.Move(fyne.NewPos(0, t.Height-tema.Borde))
	r.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	linea := t.Height - tema.Borde

	derecha := t.Width - rellenoFranjaX
	if r.f.Nota != "" {
		w := Medir(r.f.Nota, tema.TextoSM, Medio).Width
		derecha -= w
		ColocarTexto(r.nota, tema.TextoSM, derecha, (linea-tema.TextoSM.Linea)/2)
		derecha -= huecoFranja
	}
	bs := r.f.botones()
	for i := len(bs) - 1; i >= 0; i-- {
		m := bs[i].MinSize()
		derecha -= m.Width
		bs[i].Move(fyne.NewPos(derecha, (linea-m.Height)/2))
		bs[i].Resize(m)
		derecha -= huecoFranja
	}

	r.glifo.Colocar(rellenoFranjaX, (linea-ladoCandado)/2, ladoCandado)
	x := rellenoFranjaX + ladoCandado + huecoFranja
	Truncar(r.texto, r.f.Texto, tema.TextoSM, Normal, max(derecha-x, 0))
	ColocarTexto(r.texto, tema.TextoSM, x, (linea-tema.TextoSM.Linea)/2)
}

// Refresh repinta con los tokens del tema actual y el texto de ahora.
func (r *franjaSoloLecturaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("bg-subtle", v)
	r.filete.FillColor = tema.Color("border", v)
	r.glifo.Tenir("text-muted", v)
	r.texto.Color = tema.Color("text-muted", v)
	r.nota.Text, r.nota.Color = r.f.Nota, tema.Color("text", v)
	r.nota.Hidden = r.f.Nota == ""
	r.Layout(r.f.Size())
	for _, o := range []fyne.CanvasObject{r.fondo, r.filete, r.texto, r.nota} {
		o.Refresh()
	}
	for _, b := range r.f.botones() {
		b.Refresh()
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

/*
Las FRANJAS de debajo de la cinta (`PdfApp.tsx`): el «hecho» en verde, el fallo en rojo,
y —cuando los hay— los campos y las zonas de censura marcados y sin aplicar.

	<div class="flex flex-wrap items-center justify-between gap-3 px-4 py-2 text-sm …">
	  <span>{texto}</span>
	  <span class="flex items-center gap-3">{acciones}</span>

`flex-wrap` con `justify-between`: si el texto y las acciones no caben en una línea, las
acciones bajan a la siguiente, pegadas a la izquierda, con `gap-3` entre las dos filas.
*/

// Las medidas de una franja.
var (
	franjaPX    = tema.Espacio(4)
	franjaPY    = tema.Espacio(2)
	franjaHueco = tema.Espacio(3)
)

// franja es una de las franjas: fondo y tinta por token, el texto y sus acciones.
type franja struct {
	widget.BaseWidget
	fondo, tinta string
	texto        string
	acciones     []fyne.CanvasObject
	// entreAcciones es el `gap-*` del grupo de acciones: `gap-3` en la del aviso y
	// `gap-2` en las de campos y censura. Cero es `gap-3`.
	entreAcciones float32
	// ancho es el de la ventana, que decide si las acciones bajan de línea. Lo sabe la
	// pantalla (el marco lo apunta al colocar), no la franja.
	ancho func() float32
}

func nuevaFranja(fondo, tinta, texto string, ancho func() float32, acciones ...fyne.CanvasObject) *franja {
	f := &franja{fondo: fondo, tinta: tinta, texto: texto, acciones: acciones, ancho: ancho}
	f.ExtendBaseWidget(f)
	return f
}

// AccessibilityLabel es el texto: un fallo interrumpe (`role="alert"`) y un «hecho» no
// (`role="status"`); Fyne no distingue los dos roles, así que al menos se anuncia.
func (f *franja) AccessibilityLabel() string { return f.texto }

// reparto devuelve las líneas del texto, si las acciones van en la misma línea y el
// alto de la línea de acciones.
func (f *franja) reparto(ancho float32) (lineas []string, juntas bool, altoAcciones float32) {
	util := max(ancho-2*franjaPX, 0)
	var acciones float32
	for i, a := range f.acciones {
		m := a.MinSize()
		if i > 0 {
			acciones += f.hueco()
		}
		acciones += m.Width
		altoAcciones = max(altoAcciones, m.Height)
	}
	texto := ui.Medir(f.texto, tema.TextoSM, ui.Normal).Width
	juntas = len(f.acciones) == 0 || texto+franjaHueco+acciones <= util
	if juntas && len(f.acciones) > 0 {
		return []string{f.texto}, true, altoAcciones
	}
	return ui.Repartir(f.texto, tema.TextoSM, ui.Normal, util), juntas, altoAcciones
}

func (f *franja) hueco() float32 {
	if f.entreAcciones > 0 {
		return f.entreAcciones
	}
	return franjaHueco
}

// medidaFranja es la ÚNICA cuenta del alto.
func medidaFranja(f *franja, ancho float32) fyne.Size {
	lineas, juntas, acciones := f.reparto(ancho)
	texto := float32(len(lineas)) * tema.TextoSM.Linea
	alto := max(texto, acciones)
	if !juntas {
		alto = texto
		if len(f.acciones) > 0 {
			alto += franjaHueco + acciones
		}
	}
	return fyne.NewSize(0, alto+2*franjaPY)
}

func (f *franja) MinSize() fyne.Size {
	f.ExtendBaseWidget(f)
	return medidaFranja(f, f.ancho())
}

func (f *franja) CreateRenderer() fyne.WidgetRenderer {
	r := &franjaRenderer{f: f, fondo: canvas.NewRectangle(color.Transparent)}
	r.Refresh()
	return r
}

type franjaRenderer struct {
	f      *franja
	fondo  *canvas.Rectangle
	lineas []*canvas.Text
}

func (r *franjaRenderer) MinSize() fyne.Size { return r.f.MinSize() }
func (r *franjaRenderer) Destroy()           {}

func (r *franjaRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.fondo}
	for _, l := range r.lineas {
		objs = append(objs, l)
	}
	return append(objs, r.f.acciones...)
}

func (r *franjaRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	lineas, juntas, altoAcciones := r.f.reparto(t.Width)
	r.lineas = r.lineas[:0]
	for _, l := range lineas {
		r.lineas = append(r.lineas, ui.Texto(l, tema.TextoSM, ui.Normal, tema.Color(r.f.tinta, v)))
	}
	texto := float32(len(lineas)) * tema.TextoSM.Linea
	fila := max(texto, altoAcciones)
	y := franjaPY
	if juntas {
		y += (fila - texto) / 2
	}
	for i, l := range r.lineas {
		ui.ColocarTexto(l, tema.TextoSM, franjaPX, y+float32(i)*tema.TextoSM.Linea)
	}
	// Las acciones: a la derecha en la misma línea, o debajo y a la izquierda.
	x, ya := t.Width-franjaPX, franjaPY+(fila-altoAcciones)/2
	if !juntas {
		x, ya = franjaPX, franjaPY+texto+franjaHueco
		for _, a := range r.f.acciones {
			m := a.MinSize()
			a.Move(fyne.NewPos(x, ya+(altoAcciones-m.Height)/2))
			a.Resize(m)
			x += m.Width + r.f.hueco()
		}
		return
	}
	for i := len(r.f.acciones) - 1; i >= 0; i-- {
		a := r.f.acciones[i]
		m := a.MinSize()
		x -= m.Width
		a.Move(fyne.NewPos(x, ya+(altoAcciones-m.Height)/2))
		a.Resize(m)
		x -= r.f.hueco()
	}
}

func (r *franjaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color(r.f.fondo, v)
	r.fondo.Refresh()
	r.Layout(r.f.Size())
	canvas.Refresh(r.f)
}

// accionDeFranja es un botón de franja: `rounded px-2 py-0.5 text-xs underline`, con la
// tinta de la franja.
func accionDeFranja(texto, tinta string, medio bool, alPulsar func()) *Accion {
	a := NuevaAccion(texto, alPulsar)
	a.Peso = ui.Normal
	if medio {
		a.Peso = ui.Medio
	}
	a.PY, a.Radio = tema.Espacio(0.5), 0.25*tema.Rem
	a.Tinta, a.FondoEncima, a.Subrayado = tinta, "", true
	return a
}

// franjaAviso es la del «hecho» o el fallo, con «Abrir la copia» si la operación dejó
// una y «Descartar».
func (p *Pantalla) franjaAviso() fyne.CanvasObject {
	if p.Error == "" && p.Aviso == "" {
		return nil
	}
	fondo, tinta, texto := "success-bg", "success-text", p.Aviso
	if p.Error != "" {
		fondo, tinta, texto = "danger-bg", "danger-text", p.Error
	}
	var acciones []fyne.CanvasObject
	if p.Copia != "" {
		ruta := p.Copia
		acciones = append(acciones, accionDeFranja(p.t("pdf.saved.open"), tinta, true, func() {
			if p.AlAbrirRuta != nil {
				p.AlAbrirRuta(ruta)
			}
		}))
	}
	acciones = append(acciones, accionDeFranja(p.t("pdf.dismiss"), tinta, false, func() {
		p.Error, p.Aviso, p.Copia = "", "", ""
		p.Recomponer()
	}))
	return nuevaFranja(fondo, tinta, texto, p.anchoVentana, acciones...)
}

// anchoVentana es el ancho con el que se colocó la pantalla la última vez.
func (p *Pantalla) anchoVentana() float32 {
	if p.ancho > 0 {
		return p.ancho
	}
	return 1280
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Skeleton.tsx

import (
	"image/color"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// DelanteroEsqueleto es el `lead` de SkeletonRows: lo que va delante del texto.
type DelanteroEsqueleto int

const (
	// SinDelantero es `lead="none"`.
	SinDelantero DelanteroEsqueleto = iota
	// DelanteroAvatar es `lead="avatar"`: el círculo de `Avatar size="sm"`.
	DelanteroAvatar
	// DelanteroTesela es `lead="tile"`: la tesela de tipo de Drive.
	DelanteroTesela
)

// LadoDelanteroEsqueleto es el `LADO_DELANTERO` de la web: 32 el avatar y 36 la tesela.
func LadoDelanteroEsqueleto(d DelanteroEsqueleto) float32 {
	switch d {
	case DelanteroAvatar:
		return MedidaAvatar(AvatarSM).Width
	case DelanteroTesela:
		return TeselaDrive
	case SinDelantero:
	}
	return 0
}

// Las rayas de cada fila, en rem de la suite: el título de `0.85rem`, el subtítulo de
// `0.7rem` con el `gap-1.5` entre los dos, y la meta de `0.7rem` por defecto separada por
// `gap-8`.
const (
	AltoTituloEsqueleto float32 = 0.85 * tema.Rem
	AltoSubEsqueleto    float32 = 0.7 * tema.Rem
	huecoLineasEsq      float32 = 1.5 * 0.25 * tema.Rem
	huecoMetaEsq        float32 = 8 * 0.25 * tema.Rem
)

// AltoFilaEsqueleto es `altoFilaEsqueleto` de la web: `py-2` arriba y abajo más lo más
// alto de sus hijos —la caja de línea de `text-base`, más la de `text-sm` con dos líneas,
// o la pieza delantera si es más alta—. El filete, si lo hay, va DENTRO de ese alto, como
// el `border-b` de un `<li>` con `box-sizing: border-box`.
func AltoFilaEsqueleto(d DelanteroEsqueleto, lineas int) float32 {
	texto := tema.TextoBase.Linea
	if lineas >= 2 {
		texto += tema.TextoSM.Linea
	}
	return 2*AltoFila + max(texto, LadoDelanteroEsqueleto(d))
}

// MetaEsqueleto es una columna de la derecha (`SkeletonRowsMeta`). Oculta es la columna
// que la web esconde por debajo de un corte (`hidden sm:block`): quien la monta sabe el
// ancho de su ventana.
type MetaEsqueleto struct {
	Ancho, Alto float32
	Oculta      bool
}

// FilasEsqueleto es `SkeletonRows`: las filas de carga de una lista, ya montadas.
//
// Cada app se hacía las suyas —Drive tenía tres, con tres geometrías—, todas con un
// `Esqueleto` por bloque, o sea una animación por bloque. Aquí late la lista entera A UNA,
// con una sola animación que tiñe todos los rectángulos, que es además lo que se ve en la
// web: todos los `.sytue-skeleton` arrancan juntos y pulsan en fase.
//
// Para el lector de pantalla es un único «Cargando» (`role="status"` una vez, §3.18): los
// bloques son rectángulos de lienzo y no llegan a él.
type FilasEsqueleto struct {
	widget.BaseWidget

	Filas     int
	Delantero DelanteroEsqueleto
	// Lineas es 1 (título) o 2 (título y subtítulo).
	Lineas int
	Meta   []MetaEsqueleto
	// Margen es el relleno horizontal del contenedor (el `px-3` de una lista), por fuera
	// del `px-3` de cada fila.
	Margen float32
	// Filete es el `border-b` de cada fila (`rowClass`).
	Filete bool
	// SinUltimoFilete es el `last:border-b-0`: dentro de una caja con borde, el filete
	// de la última fila se sumaría al de la caja.
	SinUltimoFilete bool
	// Rotulo es el `label`; vacío es «Cargando» (`ui.loading`).
	Rotulo string

	mu   sync.Mutex
	fase float32
	anim *fyne.Animation
}

var (
	_ fyne.Widget     = (*FilasEsqueleto)(nil)
	_ fyne.Accessible = (*FilasEsqueleto)(nil)
)

// NuevasFilasEsqueleto crea `filas` filas de carga de `lineas` líneas.
func NuevasFilasEsqueleto(filas int, d DelanteroEsqueleto, lineas int) *FilasEsqueleto {
	f := &FilasEsqueleto{Filas: filas, Delantero: d, Lineas: lineas}
	f.ExtendBaseWidget(f)
	return f
}

// AccessibilityLabel es la región `role="status"`: se dice una vez para toda la lista.
func (f *FilasEsqueleto) AccessibilityLabel() string {
	if f.Rotulo != "" {
		return f.Rotulo
	}
	return textoComun("ui.loading")
}

// AccessibilityRole: como la Rueda, Fyne no tiene «status» y lo que llega es el texto.
func (f *FilasEsqueleto) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleText
}

// parteTitulo y parteSub son los anchos de la web (`38 + (i*11)%42` y `50 + (i*17)%35`
// por ciento): iguales, el esqueleto parece una tabla vacía en vez de algo que llega.
func parteTitulo(i int) float32 { return float32(38+(i*11)%42) / 100 }
func parteSub(i int) float32    { return float32(50+(i*17)%35) / 100 }

// filaEsq son los bloques de una fila; los nil no se pintan.
type filaEsq struct {
	delantero, titulo, sub, filete *canvas.Rectangle
	meta                           []*canvas.Rectangle
}

// CreateRenderer monta los rectángulos y arranca el latido.
func (f *FilasEsqueleto) CreateRenderer() fyne.WidgetRenderer {
	r := &filasEsqueletoRenderer{f: f}
	nuevo := func() *canvas.Rectangle {
		b := canvas.NewRectangle(color.NRGBA{})
		r.bloques = append(r.bloques, b)
		r.objetos = append(r.objetos, b)
		return b
	}
	for range max(f.Filas, 0) {
		var fila filaEsq
		if f.Delantero != SinDelantero {
			fila.delantero = nuevo()
		}
		fila.titulo = nuevo()
		if f.Lineas >= 2 {
			fila.sub = nuevo()
		}
		for range f.Meta {
			fila.meta = append(fila.meta, nuevo())
		}
		if f.Filete && (!f.SinUltimoFilete || len(r.filas) != f.Filas-1) {
			// El filete es borde, no esqueleto: no late.
			fila.filete = canvas.NewRectangle(color.NRGBA{})
			r.objetos = append(r.objetos, fila.filete)
		}
		r.filas = append(r.filas, fila)
	}
	f.arrancar(r)
	r.Refresh()
	return r
}

// arrancar lanza el único latido. Con «menos movimiento» los bloques se quedan opacos,
// como el Esqueleto suelto.
func (f *FilasEsqueleto) arrancar(r *filasEsqueletoRenderer) {
	if MenosMovimiento() {
		return
	}
	a := fyne.NewAnimation(DuracionPulso, func(p float32) {
		f.mu.Lock()
		f.fase = p
		f.mu.Unlock()
		r.tenir()
	})
	a.Curve = fyne.AnimationLinear
	a.RepeatCount = fyne.AnimationRepeatForever
	f.mu.Lock()
	f.anim = a
	f.mu.Unlock()
	a.Start()
}

// Parar detiene el latido. Lo llama el renderer al destruirse.
func (f *FilasEsqueleto) Parar() {
	f.mu.Lock()
	a := f.anim
	f.anim = nil
	f.mu.Unlock()
	if a != nil {
		a.Stop()
	}
}

type filasEsqueletoRenderer struct {
	f       *FilasEsqueleto
	filas   []filaEsq
	bloques []*canvas.Rectangle
	objetos []fyne.CanvasObject
}

func (r *filasEsqueletoRenderer) alto() float32 {
	return AltoFilaEsqueleto(r.f.Delantero, r.f.Lineas)
}

func (r *filasEsqueletoRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, float32(len(r.filas))*r.alto())
}

func (r *filasEsqueletoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filasEsqueletoRenderer) Destroy()                     { r.f.Parar() }

// ponerBloque coloca un bloque del esqueleto; un ancho negativo se queda en cero.
func ponerBloque(b *canvas.Rectangle, x, y, w, h float32) {
	b.Move(fyne.NewPos(x, y))
	b.Resize(fyne.NewSize(max(w, 0), h))
}

func (r *filasEsqueletoRenderer) Layout(t fyne.Size) {
	alto := r.alto()
	izq := r.f.Margen + MargenFila
	der := t.Width - r.f.Margen - MargenFila
	lado := LadoDelanteroEsqueleto(r.f.Delantero)
	for i, fila := range r.filas {
		y := float32(i) * alto
		caja := alto
		if fila.filete != nil {
			caja -= tema.Borde
			ponerBloque(fila.filete, r.f.Margen, y+caja, t.Width-2*r.f.Margen, tema.Borde)
		}
		centro := func(h float32) float32 { return y + (caja-h)/2 }
		x := izq
		if fila.delantero != nil {
			ponerBloque(fila.delantero, x, centro(lado), lado, lado)
			fila.delantero.CornerRadius = tema.RadioMD
			if r.f.Delantero == DelanteroAvatar {
				fila.delantero.CornerRadius = lado / 2
			}
			x += lado + HuecoFila
		}
		finTexto := r.colocarMeta(fila, der, centro)
		util := max(finTexto-x, 0)
		texto := AltoTituloEsqueleto
		if fila.sub != nil {
			texto += huecoLineasEsq + AltoSubEsqueleto
		}
		ty := centro(texto)
		ponerBloque(fila.titulo, x, ty, float32(int(util*parteTitulo(i)+0.5)), AltoTituloEsqueleto)
		fila.titulo.CornerRadius = tema.RadioMD
		if fila.sub != nil {
			ponerBloque(fila.sub, x, ty+AltoTituloEsqueleto+huecoLineasEsq, float32(int(util*parteSub(i)+0.5)), AltoSubEsqueleto)
			fila.sub.CornerRadius = tema.RadioMD
		}
	}
}

// colocarMeta pone la meta de una fila de derecha a izquierda (la oculta no ocupa
// sitio) y devuelve dónde acaba el texto.
func (r *filasEsqueletoRenderer) colocarMeta(fila filaEsq, der float32, centro func(float32) float32) float32 {
	xm := der
	hayMeta := false
	for j := len(fila.meta) - 1; j >= 0; j-- {
		m, b := r.f.Meta[j], fila.meta[j]
		b.CornerRadius = tema.RadioMD
		if m.Oculta {
			b.Hide()
			continue
		}
		b.Show()
		h := m.Alto
		if h <= 0 {
			h = AltoSubEsqueleto
		}
		if hayMeta {
			xm -= huecoMetaEsq
		}
		xm -= m.Ancho
		ponerBloque(b, xm, centro(h), m.Ancho, h)
		hayMeta = true
	}
	if !hayMeta {
		return der
	}
	return xm - HuecoFila
}

// tenir pinta todos los bloques con el instante del latido.
func (r *filasEsqueletoRenderer) tenir() {
	r.f.mu.Lock()
	fase := r.f.fase
	r.f.mu.Unlock()
	c := tintaEsqueleto("", fase)
	for _, b := range r.bloques {
		b.FillColor = c
		b.Refresh()
	}
}

// Refresh vuelve a tenir (el tema puede haber cambiado) y recoloca.
func (r *filasEsqueletoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	for _, fila := range r.filas {
		if fila.filete != nil {
			fila.filete.FillColor = tema.Color("border", v)
			fila.filete.Refresh()
		}
	}
	r.tenir()
	r.Layout(r.f.Size())
}

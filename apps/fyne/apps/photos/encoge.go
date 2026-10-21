// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"image/color"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Lo que encoge y se parte en líneas
//
// En un teléfono la web no deja que la cabecera del mes ni el rótulo de la descripción
// se salgan: sus hijos son elementos de un `flex` con `flex-shrink: 1` —el valor por
// defecto—, así que cuando no caben ENCOGEN a la vez, cada uno en proporción a lo que
// medía, sin bajar de su palabra más larga, y su texto se parte en las líneas que le
// quepan. El relleno y los bordes cuentan al ocupar espacio, pero no al pesar
// cuánto encoge cada hijo: CSS Flexbox §9.7 usa la base interior.

// PiezaFlex es un hijo de FilaEncoge: lo que mide su contenido en una línea (el
// `max-content`), su palabra más larga (el `min-content`, de donde no baja) y su alto a
// un ancho dado (nil: el de su mínimo). Fija es `shrink-0`: no encoge.
type PiezaFlex struct {
	Max, Min float32
	// RellenoX es la suma del relleno y los bordes horizontales. CSS reparte
	// flex-shrink según la base INTERIOR: no encoge el padding (CSS Flexbox §9.7).
	RellenoX float32
	Alto     func(ancho float32) float32
	Fija     bool
}

// FilaEncoge es `flex items-center gap-*` con hijos que encogen, y RellenoY su `py-*`.
// Cada hijo se centra en alto en la fila, que mide lo que el más alto a su ancho. Con
// Separar es además `justify-between`: lo que sobra va entre los hijos.
type FilaEncoge struct {
	Hueco, RellenoY float32
	Piezas          []PiezaFlex
	Separar         bool
}

var _ fyne.Layout = FilaEncoge{}

// Anchos es lo que se lleva cada hijo a ese ancho: el reparto de CSS del espacio que
// falta (§9.7 de css-flexbox). Se le quita a cada uno en proporción a su medida interior; el que
// llegaría por debajo de su mínimo se queda en él, deja de encoger, y lo que falta se
// vuelve a repartir entre los demás.
func (f FilaEncoge) Anchos(ancho float32) []float32 {
	n := len(f.Piezas)
	a := make([]float32, n)
	quieta := make([]bool, n)
	for i, p := range f.Piezas {
		a[i], quieta[i] = p.Max, p.Fija
	}
	disponible := ancho - f.Hueco*float32(max(n-1, 0))
	for {
		ocupado, pesos := float32(0), float32(0)
		for i, p := range f.Piezas {
			if quieta[i] {
				ocupado += a[i]
				continue
			}
			ocupado += p.Max
			pesos += max(p.Max-p.RellenoX, 0)
		}
		falta := ocupado - disponible
		if falta <= 0 || pesos == 0 {
			return a
		}
		topado := false
		for i, p := range f.Piezas {
			if quieta[i] {
				continue
			}
			if a[i] = p.Max - falta*max(p.Max-p.RellenoX, 0)/pesos; a[i] < p.Min {
				a[i], quieta[i], topado = p.Min, true, true
			}
		}
		if !topado {
			return a
		}
	}
}

func (f FilaEncoge) alto(i int, o fyne.CanvasObject, ancho float32) float32 {
	if p := f.Piezas[i]; p.Alto != nil {
		return p.Alto(ancho)
	}
	return o.MinSize().Height
}

// AltoCon es el alto de la fila a ese ancho.
func (f FilaEncoge) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	alto := float32(0)
	for i, w := range f.Anchos(ancho) {
		alto = max(alto, f.alto(i, objs[i], w))
	}
	return alto + 2*f.RellenoY
}

// MinSize es el `min-content` de la fila: cada hijo en su palabra más larga.
func (f FilaEncoge) MinSize(objs []fyne.CanvasObject) fyne.Size {
	ancho := f.Hueco * float32(max(len(objs)-1, 0))
	for _, p := range f.Piezas {
		ancho += p.Min
		if p.Fija {
			ancho += p.Max - p.Min
		}
	}
	return fyne.NewSize(ancho, f.AltoCon(objs, ancho))
}

func (f FilaEncoge) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	anchos := f.Anchos(t.Width)
	hueco := f.Hueco
	if f.Separar && len(anchos) > 1 {
		libre := t.Width - hueco*float32(len(anchos)-1)
		for _, w := range anchos {
			libre -= w
		}
		hueco += max(libre, 0) / float32(len(anchos)-1)
	}
	x := float32(0)
	for i, w := range anchos {
		h := f.alto(i, objs[i], w)
		objs[i].Move(fyne.NewPos(x, (t.Height-h)/2))
		objs[i].Resize(fyne.NewSize(w, h))
		x += w + hueco
	}
}

// PiezaParrafo es la pieza de un párrafo de una sola cara: su línea entera y su palabra
// más larga.
func PiezaParrafo(p *ui.Parrafo) PiezaFlex {
	medir := func(s string) float32 { return ui.Medir(s, p.Escalon, p.Peso).Width }
	return PiezaFlex{
		Max: medir(p.Texto), Min: palabraMasLarga(p.Texto, medir),
		Alto: func(ancho float32) float32 { return ui.AltoParrafo(p, ancho) },
	}
}

// palabraMasLarga es el `min-content` de un texto que se parte por espacios.
func palabraMasLarga(texto string, medir func(string) float32) float32 {
	m := float32(0)
	for _, w := range strings.Fields(texto) {
		m = max(m, medir(w))
	}
	return m
}

// repartir es el reparto codicioso por espacios con la medida que se le pase: el de
// `ui.Repartir`, pero para lo que no se mide con una cara sola (las versalitas con su
// tracking).
func repartir(texto string, cabe float32, medir func(string) float32) []string {
	palabras := strings.Fields(texto)
	if len(palabras) == 0 {
		return nil
	}
	lineas := []string{palabras[0]}
	for _, w := range palabras[1:] {
		i := len(lineas) - 1
		if cand := lineas[i] + " " + w; cabe <= 0 || medir(cand) <= cabe {
			lineas[i] = cand
			continue
		}
		lineas = append(lineas, w)
	}
	return lineas
}

// ---- Las versalitas que se parten ----

// VersalRepartido es un rótulo en versalitas con su `letter-spacing` que, si no cabe,
// se parte en líneas como cualquier texto del navegador. `ui.Versal` es UNA línea —cada
// letra es su propio `canvas.Text`, porque Fyne no sabe de tracking—, así que aquí va
// uno por renglón.
type VersalRepartido struct {
	widget.BaseWidget
	Texto   string
	Escalon tema.Escalon
	Peso    ui.Peso
	Tinta   color.NRGBA
	// Tracking es el `letter-spacing` en em (ui.TrackingOverline, ui.TrackingWider).
	Tracking float32
}

var _ fyne.Widget = (*VersalRepartido)(nil)

// NuevoVersalRepartido crea el rótulo.
func NuevoVersalRepartido(texto string, e tema.Escalon, p ui.Peso, c color.NRGBA, tracking float32) *VersalRepartido {
	v := &VersalRepartido{Texto: texto, Escalon: e, Peso: p, Tinta: c, Tracking: tracking}
	v.ExtendBaseWidget(v)
	return v
}

func (v *VersalRepartido) medir(s string) float32 {
	return ui.MedirVersalCon(s, v.Escalon, v.Peso, v.Tracking)
}

// Lineas es el rótulo partido a ese ancho.
func (v *VersalRepartido) Lineas(ancho float32) []string { return repartir(v.Texto, ancho, v.medir) }

// Pieza es lo que el rótulo pone en una FilaEncoge.
func (v *VersalRepartido) Pieza() PiezaFlex {
	return PiezaFlex{
		Max: v.medir(v.Texto), Min: palabraMasLarga(v.Texto, v.medir),
		Alto: func(ancho float32) float32 { return float32(max(len(v.Lineas(ancho)), 1)) * v.Escalon.Linea },
	}
}

// CreateRenderer arma el rótulo. Las líneas se hacen al colocarlo, que es cuando se
// sabe el ancho.
func (v *VersalRepartido) CreateRenderer() fyne.WidgetRenderer {
	return &versalRepartidoRenderer{v: v}
}

type versalRepartidoRenderer struct {
	v       *VersalRepartido
	hechas  []string
	lineas  []*ui.Versal
	objetos []fyne.CanvasObject
}

func (r *versalRepartidoRenderer) MinSize() fyne.Size {
	p := r.v.Pieza()
	return fyne.NewSize(p.Max, r.v.Escalon.Linea)
}

func (r *versalRepartidoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *versalRepartidoRenderer) Destroy()                     {}

func (r *versalRepartidoRenderer) Layout(t fyne.Size) {
	if l := r.v.Lineas(t.Width); !slices.Equal(l, r.hechas) {
		r.hechas, r.lineas, r.objetos = l, nil, nil
		for _, s := range l {
			vs := ui.NuevoVersalCon(s, r.v.Escalon, r.v.Peso, r.v.Tinta, r.v.Tracking)
			r.lineas = append(r.lineas, vs)
			r.objetos = append(r.objetos, vs.Objetos()...)
		}
	}
	for i, vs := range r.lineas {
		vs.Colocar(0, float32(i)*r.v.Escalon.Linea)
	}
}

func (r *versalRepartidoRenderer) Refresh() {
	r.hechas = nil
	r.Layout(r.v.Size())
	for _, vs := range r.lineas {
		vs.Pintar(r.v.Tinta)
	}
}

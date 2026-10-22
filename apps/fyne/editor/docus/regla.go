// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// AltoRegla es `--editor-ruler-h`, en píxeles literales de la hoja.
const AltoRegla float32 = 20

// AnchoA4Cm es el ancho del A4 en centímetros. El paso de la regla es
// `anchoPágina / 21`, que es lo que hace `Ruler.tsx`.
const AnchoA4Cm = 21

// Las tintas de la regla son locales de `editor.css` y NO tokens del tema, por lo
// mismo que las del folio: la regla se pinta sobre el papel, que es blanco en los dos
// temas, así que una tinta que siguiera al tema saldría a 3,05:1 en oscuro.
var (
	// TintaRegla es `--editor-ruler-ink`: rgba(40, 36, 30, 0.8).
	TintaRegla = color.NRGBA{R: 40, G: 36, B: 30, A: 204}
	// BordeRegla es `--editor-ruler-border`: rgba(40, 36, 30, 0.16).
	BordeRegla = color.NRGBA{R: 40, G: 36, B: 30, A: 41}
	// MargenRegla es `--editor-ruler-margin`: rgba(15, 23, 42, 0.08), la zona
	// sombreada de los márgenes.
	MargenRegla = color.NRGBA{R: 15, G: 23, B: 42, A: 20}
)

// TamRotuloRegla es el `font-size="8"` de los números de la regla.
const TamRotuloRegla float32 = 8

// Regla es la regla horizontal del folio, como la dibuja `Ruler.tsx`:
//
//   - las zonas de margen sombreadas;
//   - los números cada centímetro contados DESDE EL MARGEN IZQUIERDO —el 0 es el
//     principio del texto y no se rotula; en el margen se rotula la distancia, «2 1»—,
//     con `text-anchor="middle"` y la línea base en y = 13;
//   - una rayita en cada medio centímetro, de y = 8 a 12, al 70 %;
//   - los tres marcadores de sangría del párrafo del cursor: primera línea arriba
//     (▼) y sangría izquierda y derecha abajo (▲).
//
// ponytail: los marcadores se pintan donde dice el párrafo, pero no se ARRASTRAN; en la
// web se arrastran y se mueven con las flechas. Relevar con un `Draggable` que escriba
// `margin-left`, `text-indent` y `margin-right` como `moveMarker`.
type Regla struct {
	widget.BaseWidget

	// Margen es el de la página; Izq, Primera y Der las sangrías del párrafo del cursor
	// (`margin-left`, `text-indent`, `margin-right`), en px de maqueta.
	Margen, Izq, Primera, Der float32
	// Ancho es el de la página en px de maqueta (cero: el A4 vertical).
	Ancho float32
}

// anchoYCm son el ancho de la página y los centímetros que mide. La web los saca de
// `pageW / (landscape ? 29.7 : 21)`, que es 96 dpi; aquí también, y para CUALQUIER
// tamaño: allí Carta y Legal dejan la regla midiendo un A4 (su propio `ponytail`).
func (r *Regla) anchoYCm() (float32, float32) {
	ancho := r.Ancho
	if ancho <= 0 {
		ancho = AnchoPagina
	}
	return ancho, ancho / (AnchoPagina / AnchoA4Cm)
}

var _ fyne.Widget = (*Regla)(nil)

// NuevaRegla crea la regla con el margen `Normal`.
func NuevaRegla() *Regla {
	r := &Regla{Margen: MargenPagina}
	r.ExtendBaseWidget(r)
	return r
}

// AccessibilityRole la anuncia como contenedor con nombre, el `role="group"` de la web.
func (r *Regla) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// MinSize es el folio de ancho por los 20 de alto.
func (r *Regla) MinSize() fyne.Size {
	r.ExtendBaseWidget(r)
	ancho, _ := r.anchoYCm()
	return fyne.NewSize(ancho, AltoRegla)
}

// marcaRegla es una marca de la regla en px de maqueta: un número o una rayita.
type marcaRegla struct {
	x      float32
	numero string // vacío: rayita de medio centímetro
}

// marcasRegla es el bucle de `Ruler.tsx`: desde el primer medio centímetro que cabe a
// la izquierda del margen hasta el borde derecho, sin el 0.
func marcasRegla(margen, ancho, cm float32) []marcaRegla {
	paso := ancho / cm / 2
	var out []marcaRegla
	i := ceilInt((2 - margen) / paso)
	for ; ; i++ {
		x := margen + float32(i)*paso
		if x > ancho-2 {
			break
		}
		switch {
		case i == 0: // la frontera del margen: ya la marca el sombreado
		case i%2 == 0:
			out = append(out, marcaRegla{x: x, numero: strconv.Itoa(absInt(i / 2))})
		default:
			out = append(out, marcaRegla{x: x})
		}
	}
	return out
}

func ceilInt(f float32) int {
	i := int(f)
	if float32(i) < f {
		i++
	}
	return i
}

func absInt(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// CreateRenderer compone el fondo, las zonas de margen, las marcas y los marcadores.
func (r *Regla) CreateRenderer() fyne.WidgetRenderer {
	return &reglaRenderer{r: r}
}

type reglaRenderer struct {
	r    *Regla
	objs []fyne.CanvasObject
}

func (rr *reglaRenderer) MinSize() fyne.Size           { return rr.r.MinSize() }
func (rr *reglaRenderer) Objects() []fyne.CanvasObject { return rr.objs }
func (rr *reglaRenderer) Destroy()                     {}

// Layout reconstruye la regla entera: son unas cincuenta primitivas y cambian con el
// párrafo del cursor.
func (rr *reglaRenderer) Layout(t fyne.Size) {
	r := rr.r
	ancho, cm := r.anchoYCm()
	z := t.Width / ancho
	rect := func(c color.Color, x, y, w, h float32) {
		o := canvas.NewRectangle(c)
		o.Move(fyne.NewPos(x*z, y*z))
		o.Resize(fyne.NewSize(max(w*z, 1), max(h*z, 1)))
		rr.objs = append(rr.objs, o)
	}
	rr.objs = nil // un slice nuevo: Fyne puede estar recorriendo el anterior
	rect(PapelBlanco, 0, 0, ancho, AltoRegla)
	rect(MargenRegla, 0, 0, r.Margen, AltoRegla)
	rect(MargenRegla, ancho-r.Margen, 0, r.Margen, AltoRegla)
	// `border-b` de 1 px DENTRO de los 20 (`box-sizing: border-box`).
	rect(BordeRegla, 0, AltoRegla-1, ancho, 1)

	rayita := TintaRegla
	rayita.A = uint8(float32(rayita.A) * 0.7) // `opacity={0.7}` del grupo de rayas
	for _, m := range marcasRegla(r.Margen, ancho, cm) {
		if m.numero == "" {
			rect(rayita, m.x-0.5, 8, 1, 4)
			continue
		}
		n := canvas.NewText(m.numero, TintaRegla)
		n.TextSize = TamRotuloRegla * z
		n.FontSource = ui.Normal.Cara()
		ancho := n.MinSize().Width
		// La línea base del SVG en y = 13; la caja de la letra sube 0,95 em desde ella.
		n.Move(fyne.NewPos(m.x*z-ancho/2, (13-ascFuente*TamRotuloRegla)*z))
		n.Resize(n.MinSize())
		rr.objs = append(rr.objs, n)
	}

	// Los marcadores: una caja de 14 × 9 centrada en su x, arriba o abajo, con el
	// triángulo de 10 × 7 dentro.
	marcador := func(x float32, arriba bool) {
		// Las rutas de la web (`viewBox="0 0 10 7"`) llevadas a la rejilla de 24 de
		// `iconos`: ×2,4. El trazo de 2 en esa rejilla es el de 1 de la web a 0,83.
		d := "M12 2.4l9.6 12H2.4Z" // M5 1l4 5H1Z
		y := AltoRegla - 9
		if arriba {
			d, y = "M2.4 2.4h19.2L12 14.4Z", 0 // M1 1h8L5 6Z
		}
		g := iconos.TrazoDeRutaRelleno(d, 10, TintaRegla)
		g.Colocar((x-5)*z, (y+1)*z, 10*z)
		rr.objs = append(rr.objs, g.Objeto())
	}
	marcador(r.Margen+r.Izq+r.Primera, true)
	marcador(r.Margen+r.Izq, false)
	marcador(ancho-r.Margen-r.Der, false)
}

func (rr *reglaRenderer) Refresh() {
	rr.Layout(rr.r.Size())
	canvas.Refresh(rr.r)
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Medidas del grupo segmentado, de `AccountMenu.tsx`:
//
//	carril  `bg-bg-subtle flex rounded-md p-0.5`      → relleno 2 y `rounded-md`; dentro de
//	        un menú, `bg-subtle` es `menu-hover` (ver EnMenu)
//	opción  `rounded-control px-2 py-1 text-xs font-medium` → 24 de alto, radio 4
//	activa  `bg-surface text-text shadow-[var(--shadow-xs)]`
//	resto   `text-text-muted hover:text-text`
//	muestra `size-2.5 rounded-full`, con el `gap-1.5` de la opción hasta su rótulo
const (
	// LadoMuestra es el `size-2.5` del círculo de color de una opción y HuecoMuestra el
	// `gap-1.5` que lo separa del rótulo.
	LadoMuestra  float32 = 2.5 * 0.25 * tema.Rem
	HuecoMuestra float32 = 1.5 * 0.25 * tema.Rem
	// RellenoSegmentado es el `p-0.5` del carril.
	RellenoSegmentado float32 = 0.5 * 0.25 * tema.Rem
	// MargenSegmento es el `px-2` de cada opción.
	MargenSegmento float32 = 2 * 0.25 * tema.Rem
)

// RadioSegmento es la esquina `rounded-control` de cada opción.
var RadioSegmento = tema.Forma("radius-control")

// EscalonSegmento es el `text-xs` de las opciones. Con nombre propio para que el
// guardián pueda nombrar de dónde sale el alto de 24.
var EscalonSegmento = tema.TextoXS

// AltoSegmento es `py-1` sobre la caja de línea de `text-xs`: 4 + 16 + 4.
var AltoSegmento float32 = 2*(1*0.25*tema.Rem) + tema.TextoXS.Linea

// Segmentado es el grupo de botones con UNO elegido: el `role="radiogroup"` de la
// apariencia en el menú de cuenta (Sistema · Claro · Oscuro).
//
// Es un grupo y no tres botones sueltos, y eso decide el teclado: se entra con el
// tabulador UNA vez —al que está elegido— y dentro se cambia con las flechas, que es
// lo que hace un grupo de radios y lo que esperan los lectores de pantalla. Tres
// botones seguidos obligarían a tabular tres veces para pasar de largo.
type Segmentado struct {
	widget.BaseWidget

	// Opciones son los rótulos, YA traducidos.
	Opciones []string
	// Elegida es el índice activo.
	Elegida int
	// Rotulo es el nombre accesible del grupo (`aria-label`).
	Rotulo   string
	AlElegir func(int)
	// Muestras son los círculos de color que acompañan a los rótulos (`muestra` en
	// AccountMenu.tsx), una por opción; nil, no hay. Cada una es un color o, con
	// varios, los sectores iguales de un `conic-gradient` sin fundido, que es como se
	// enseña «el de cada app».
	Muestras [][]color.NRGBA
	// Ancho es el tope del grupo: las opciones que no caben bajan a otra fila, que es su
	// `flex-wrap`, y el carril ocupa entonces todo ese ancho. Con 0 van en una sola.
	Ancho float32
	// EnMenu dice que el grupo va DENTRO de un menú. La web no lo dice en el componente:
	// `.sytue-menu-surface` reasigna `--sytue-bg-subtle` a `--sytue-menu-hover`
	// (appearance.css) y el carril lo hereda. Aquí no hay cascada, así que lo dice quien
	// monta el menú. Sin esto, en traslúcido el carril es un velo blanco al 26 % sobre un
	// menú casi blanco y no se ve; en oscuro, tampoco.
	EnMenu bool

	encima   int // índice bajo el puntero, o -1
	enfocado bool
}

var (
	_ fyne.Widget       = (*Segmentado)(nil)
	_ fyne.Tappable     = (*Segmentado)(nil)
	_ fyne.Focusable    = (*Segmentado)(nil)
	_ fyne.Accessible   = (*Segmentado)(nil)
	_ desktop.Hoverable = (*Segmentado)(nil)
)

// NuevoSegmentado crea el grupo con sus opciones.
func NuevoSegmentado(rotulo string, opciones []string, alElegir func(int)) *Segmentado {
	s := &Segmentado{Opciones: opciones, Rotulo: rotulo, AlElegir: alElegir, encima: -1}
	s.ExtendBaseWidget(s)
	return s
}

// pista es el token del carril: `bg-subtle`, que dentro de un menú es `menu-hover`.
func (s *Segmentado) pista() string {
	if s.EnMenu {
		return "menu-hover"
	}
	return "bg-subtle"
}

// AccessibilityLabel es el `aria-label` del grupo con la opción activa, porque el
// papel de Fyne no sabe decirla.
func (s *Segmentado) AccessibilityLabel() string {
	if s.Elegida >= 0 && s.Elegida < len(s.Opciones) {
		return s.Rotulo + ": " + s.Opciones[s.Elegida]
	}
	return s.Rotulo
}

// AccessibilityRole: Fyne solo distingue cuatro papeles y el que le corresponde a un
// grupo de opciones es `container`; el que está elegido se dice en el nombre, que es
// lo que lee el lector de pantalla.
func (s *Segmentado) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// AnchoSegmento es lo que ocupa una opción: su rótulo con el `px-2` a cada lado.
func AnchoSegmento(texto string) float32 {
	return Medir(texto, EscalonSegmento, Medio).Width + 2*MargenSegmento
}

// anchoOpcion es AnchoSegmento más, si el grupo lleva muestras, la suya y su hueco.
func (s *Segmentado) anchoOpcion(i int) float32 {
	ancho := AnchoSegmento(s.Opciones[i])
	if s.Muestras != nil {
		ancho += LadoMuestra + HuecoMuestra
	}
	return ancho
}

// reparto dice dónde va cada opción y lo que ocupa el grupo. Es la ÚNICA cuenta: la
// usan el MinSize, el Layout y el clic. Sin Ancho, una fila; con él, las que hagan falta.
func (s *Segmentado) reparto() (pos []fyne.Position, total fyne.Size) {
	dentro := float32(math.Inf(1))
	if s.Ancho > 0 {
		dentro = s.Ancho - 2*RellenoSegmentado
	}
	var x, y, ancho float32
	pos = make([]fyne.Position, 0, len(s.Opciones))
	for i := range s.Opciones {
		w := s.anchoOpcion(i)
		if x > 0 && x+w > dentro {
			x, y = 0, y+AltoSegmento
		}
		pos = append(pos, fyne.NewPos(RellenoSegmentado+x, RellenoSegmentado+y))
		x += w
		ancho = max(ancho, x)
	}
	ancho += 2 * RellenoSegmentado
	if y > 0 {
		ancho = s.Ancho // un flex que envuelve ocupa su línea entera
	}
	return pos, fyne.NewSize(ancho, y+AltoSegmento+2*RellenoSegmentado)
}

// MedidaSegmentado es lo que ocupa el grupo entero.
func MedidaSegmentado(opciones []string) fyne.Size {
	var ancho float32
	for _, o := range opciones {
		ancho += AnchoSegmento(o)
	}
	return fyne.NewSize(ancho+2*RellenoSegmentado, AltoSegmento+2*RellenoSegmentado)
}

// Elegir cambia la opción activa y avisa. No hace nada si ya era esa: un radio que ya
// está marcado no dispara `change`.
func (s *Segmentado) Elegir(i int) {
	if i < 0 || i >= len(s.Opciones) || i == s.Elegida {
		return
	}
	s.Elegida = i
	s.Refresh()
	if s.AlElegir != nil {
		s.AlElegir(i)
	}
}

// Tapped elige la opción sobre la que se ha pulsado.
func (s *Segmentado) Tapped(e *fyne.PointEvent) { s.Elegir(s.enQue(e.Position)) }

// enQue es qué opción cae bajo ese punto. Fuera de todas, -1.
func (s *Segmentado) enQue(p fyne.Position) int {
	pos, _ := s.reparto()
	for i, o := range pos {
		if p.X >= o.X && p.X < o.X+s.anchoOpcion(i) && p.Y >= o.Y && p.Y < o.Y+AltoSegmento {
			return i
		}
	}
	return -1
}

// MouseIn, MouseMoved y MouseOut mantienen el `hover:text-text` de las inactivas.
func (s *Segmentado) MouseIn(e *desktop.MouseEvent) { s.MouseMoved(e) }

func (s *Segmentado) MouseMoved(e *desktop.MouseEvent) {
	if i := s.enQue(e.Position); i != s.encima {
		s.encima = i
		s.Refresh()
	}
}

func (s *Segmentado) MouseOut() {
	s.encima = -1
	s.Refresh()
}

// FocusGained enciende el anillo.
func (s *Segmentado) FocusGained() { s.enfocado = true; s.Refresh() }

// FocusLost lo apaga.
func (s *Segmentado) FocusLost() { s.enfocado = false; s.Refresh() }

// TypedRune: el espacio confirma, como en un radio.
func (s *Segmentado) TypedRune(r rune) {
	if r == ' ' && s.AlElegir != nil {
		s.AlElegir(s.Elegida)
	}
}

// TypedKey mueve con las flechas, que es la navegación de un `radiogroup`. Da la
// vuelta por los dos extremos, como la lista de resultados de la búsqueda.
func (s *Segmentado) TypedKey(e *fyne.KeyEvent) {
	var paso int
	switch e.Name {
	case fyne.KeyLeft, fyne.KeyUp:
		paso = -1
	case fyne.KeyRight, fyne.KeyDown:
		paso = 1
	case fyne.KeyReturn, fyne.KeyEnter:
		if s.AlElegir != nil {
			s.AlElegir(s.Elegida)
		}
		return
	default:
		SubirEscape(s, e)
		return
	}
	n := len(s.Opciones)
	if n > 0 {
		s.Elegir((s.Elegida + paso + n) % n)
	}
}

// CreateRenderer construye el carril, la pastilla de la activa y los rótulos.
//
// La sombra de la pastilla va SinInterior: un `box-shadow` no pasa por debajo de su
// caja, y `surface` es un velo en traslúcido que la dejaría ver y oscurecería lo elegido.
func (s *Segmentado) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &segmentadoRenderer{
		s:       s,
		aro:     NuevoAro(tema.RadioMD, ConHueco, v),
		carril:  NuevaCaja(tema.Color(s.pista(), v), color.NRGBA{}, tema.RadioMD),
		activa:  NuevaCaja(tema.Color("surface", v), color.NRGBA{}, RadioSegmento),
		sombra:  NuevaSombra(tema.SombraXS, RadioSegmento).SinInterior(),
		rotulos: make([]*canvas.Text, 0, len(s.Opciones)),
	}
	r.objetos = r.aro.Antes(r.carril.Objetos()...)
	r.objetos = append(r.objetos, r.sombra.Objeto())
	r.objetos = append(r.objetos, r.activa.Objetos()...)
	for i, o := range s.Opciones {
		t := Texto(o, EscalonSegmento, Medio, tema.Color("text-muted", v))
		r.rotulos = append(r.rotulos, t)
		r.objetos = append(r.objetos, t)
		if i < len(s.Muestras) {
			m := nuevaMuestra(s.Muestras[i])
			r.muestras = append(r.muestras, m)
			r.objetos = append(r.objetos, m)
		}
	}
	r.Refresh() // regla 7
	return r
}

// nuevaMuestra pinta el círculo de color de una opción: liso o, con varios colores, en
// sectores iguales desde las doce y en el sentido del reloj, que es un `conic-gradient`
// de paradas duras. El borde va suavizado a un píxel.
func nuevaMuestra(colores []color.NRGBA) *canvas.Raster {
	return canvas.NewRaster(func(w, h int) image.Image {
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		if len(colores) == 0 {
			return img
		}
		cx, cy, radio := float64(w)/2, float64(h)/2, float64(min(w, h))/2
		for y := range h {
			for x := range w {
				dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
				cubre := min(max(radio-math.Hypot(dx, dy)+0.5, 0), 1)
				if cubre == 0 {
					continue
				}
				// El ángulo desde las doce, en vueltas: de 0 a 1.
				vuelta := math.Mod(math.Atan2(dx, -dy)/(2*math.Pi)+1, 1)
				c := colores[min(int(vuelta*float64(len(colores))), len(colores)-1)]
				c.A = uint8(float64(c.A) * cubre)
				img.SetNRGBA(x, y, c)
			}
		}
		return img
	})
}

type segmentadoRenderer struct {
	s              *Segmentado
	aro            *Aro
	carril, activa *Caja
	sombra         *Sombra
	rotulos        []*canvas.Text
	muestras       []*canvas.Raster
	objetos        []fyne.CanvasObject
}

func (r *segmentadoRenderer) MinSize() fyne.Size {
	_, total := r.s.reparto()
	return total
}
func (r *segmentadoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *segmentadoRenderer) Destroy()                     {}

func (r *segmentadoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.carril.Redimensionar(t)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())

	sitios, _ := r.s.reparto()
	for i, rotulo := range r.rotulos {
		pos := sitios[i]
		if i == r.s.Elegida {
			tam := fyne.NewSize(r.s.anchoOpcion(i), AltoSegmento)
			MoverCaja(r.activa, pos, tam)
			r.sombra.Ajustar(pos, tam, v)
		}
		x := pos.X + MargenSegmento
		if i < len(r.muestras) {
			r.muestras[i].Move(fyne.NewPos(x, pos.Y+(AltoSegmento-LadoMuestra)/2))
			r.muestras[i].Resize(fyne.NewSize(LadoMuestra, LadoMuestra))
			x += LadoMuestra + HuecoMuestra
		}
		ColocarTexto(rotulo, EscalonSegmento, x, pos.Y+(AltoSegmento-EscalonSegmento.Linea)/2)
	}
}

func (r *segmentadoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.carril.Pintar(tema.Color(r.s.pista(), v), color.NRGBA{})
	r.activa.Pintar(tema.Color("surface", v), color.NRGBA{})
	r.aro.Pintar(v)
	r.aro.Ver(r.s.enfocado)
	for i, rotulo := range r.rotulos {
		tinta := tema.Color("text-muted", v)
		if i == r.s.Elegida || i == r.s.encima {
			tinta = tema.Color("text", v)
		}
		rotulo.Color = tinta
		rotulo.Refresh()
	}
	r.Layout(r.s.Size())
}

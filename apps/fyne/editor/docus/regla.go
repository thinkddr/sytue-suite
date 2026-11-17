// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/color"
	"math"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
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
// Los marcadores se arrastran y, con el foco, se mueven con ←/→ a pasos de 0,25 cm,
// con los topes de `withMarkerAt`; cada cambio llega a quien monta la regla por
// `Cambiar`, que es quien lo escribe en el párrafo.
type Regla struct {
	widget.BaseWidget

	// Margen es el de la página; Izq, Primera y Der las sangrías del párrafo del cursor
	// (`margin-left`, `text-indent`, `margin-right`), en px de maqueta.
	Margen, Izq, Primera, Der float32
	// Ancho es el de la página en px de maqueta (cero: el A4 vertical).
	Ancho float32

	// Cambiar recibe las sangrías nuevas tras mover un marcador y dice si las ha escrito
	// (sin párrafo de texto bajo el cursor no hay dónde, como el `selectedBlock()` nulo
	// de la web, y el marcador no se mueve). `nuevoPaso` es falso en los movimientos que
	// siguen a otro ya escrito del MISMO arrastre: todo el gesto es un paso de deshacer,
	// como el único `input` que emite la web al soltar.
	Cambiar func(izq, primera, der float32, nuevoPaso bool) bool
	// Traducir da los nombres de los marcadores para la accesibilidad (`ruler.*`).
	Traducir func(clave string) string

	marcadores [3]*marcadorRegla
}

// Topes y paso de `Ruler.tsx`.
const (
	// MinTextoRegla es `MIN_TEXT_W`: el ancho mínimo de columna que dejan los topes.
	MinTextoRegla float32 = 24
	// PasoReglaCm es `STEP_CM`, lo que mueve cada flecha.
	PasoReglaCm float32 = 0.25
)

// pxPorCm (ajustes.go) es el de la web, 96 dpi en cualquier papel.

// Los tres marcadores, en el orden de `markers` de la web.
const (
	marcaPrimera = iota
	marcaIzquierda
	marcaDerecha
)

// clavesMarcador son sus nombres accesibles, los `aria-label` de la web.
var clavesMarcador = [3]string{"ruler.firstLine", "ruler.hanging", "ruler.right"}

// sangrias son `Indents`: margin-left, text-indent y margin-right en px de maqueta.
type sangrias struct{ izq, pri, der float32 }

func acotar(v, lo, hi float32) float32 { return min(max(v, lo), max(lo, hi)) }

func redondear(v float32) float32 { return float32(math.Round(float64(v))) }

// xMarcador es `markerX`: dónde cae cada marcador sobre el folio.
func xMarcador(id int, margen, ancho float32, s sangrias) float32 {
	switch id {
	case marcaPrimera:
		return margen + s.izq + s.pri
	case marcaIzquierda:
		return margen + s.izq
	}
	return ancho - margen - s.der
}

// conMarcadorEn es `withMarkerAt`: las sangrías de llevar un marcador a x, con topes.
// Mover la izquierda conserva la posición visual de la primera línea, como la web.
func conMarcadorEn(id int, x, margen, ancho float32, cur sangrias) sangrias {
	textoW := ancho - 2*margen
	switch id {
	case marcaPrimera:
		cur.pri = redondear(acotar(x-margen-cur.izq, -cur.izq, textoW-cur.izq-cur.der-MinTextoRegla))
	case marcaIzquierda:
		izq := redondear(acotar(x-margen, 0, textoW-cur.der-MinTextoRegla))
		cur.pri = redondear(acotar(cur.pri+cur.izq-izq, -izq, textoW-izq-cur.der-MinTextoRegla))
		cur.izq = izq
	default:
		cur.der = redondear(acotar(ancho-margen-x, 0, textoW-cur.izq-MinTextoRegla))
	}
	return cur
}

// llevar mueve un marcador a x (px de maqueta) y avisa si algo cambia.
func (r *Regla) llevar(id int, x float32, nuevoPaso bool) bool {
	ancho, _ := r.anchoYCm()
	cur := sangrias{r.Izq, r.Primera, r.Der}
	nuevo := conMarcadorEn(id, x, r.Margen, ancho, cur)
	if nuevo == cur {
		return false
	}
	if r.Cambiar != nil && !r.Cambiar(nuevo.izq, nuevo.pri, nuevo.der, nuevoPaso) {
		return false
	}
	r.Izq, r.Primera, r.Der = nuevo.izq, nuevo.pri, nuevo.der
	r.Refresh()
	return true
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
	for i := range r.marcadores {
		m := &marcadorRegla{r: r, id: i}
		m.ExtendBaseWidget(m)
		r.marcadores[i] = m
	}
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

	// Los marcadores: una caja de 14 × 9 centrada en su x, arriba o abajo.
	s := sangrias{r.Izq, r.Primera, r.Der}
	for i, m := range r.marcadores {
		x := acotar(xMarcador(i, r.Margen, ancho, s), 0, ancho)
		y := AltoRegla - altoMarcador
		if i == marcaPrimera {
			y = 0
		}
		m.Move(fyne.NewPos((x-anchoMarcador/2)*z, y*z))
		m.Resize(fyne.NewSize(anchoMarcador*z, altoMarcador*z))
		m.Refresh()
		rr.objs = append(rr.objs, m)
	}
}

// La caja de cada marcador, `w-[14px] h-[9px]` de la web.
const (
	anchoMarcador float32 = 14
	altoMarcador  float32 = 9
)

// marcadorRegla es uno de los tres triángulos: se arrastra y, enfocado, se mueve con
// las flechas. Es el `<div role="slider">` de `Ruler.tsx`.
type marcadorRegla struct {
	widget.BaseWidget
	r         *Regla
	id        int
	foco      bool
	enGesto   bool // `acumulado` vale: el gesto ya empezó
	arrastre  bool // ya hay un paso de deshacer abierto para este gesto
	acumulado float32
}

var (
	_ fyne.Draggable = (*marcadorRegla)(nil)
	_ fyne.Focusable = (*marcadorRegla)(nil)
	_ fyne.Tappable  = (*marcadorRegla)(nil)
)

func (m *marcadorRegla) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleButton // Fyne no tiene «slider»
}

func (m *marcadorRegla) AccessibilityLabel() string {
	if m.r.Traducir == nil {
		return clavesMarcador[m.id]
	}
	return m.r.Traducir(clavesMarcador[m.id])
}

// escala son los px de pantalla por px de maqueta.
func (m *marcadorRegla) escala() float32 {
	ancho, _ := m.r.anchoYCm()
	if w := m.r.Size().Width; w > 0 {
		return w / ancho
	}
	return 1
}

// Dragged sigue al puntero sumando lo arrastrado: el marcador se recoloca a cada paso,
// y la posición relativa a él dejaría de valer en cuanto se mueve.
func (m *marcadorRegla) Dragged(ev *fyne.DragEvent) {
	ancho, _ := m.r.anchoYCm()
	// El origen se toma UNA vez por gesto: rehacerlo mientras nada se ha escrito tiraría
	// los movimientos de menos de medio px (un arrastre lento con zoom no movería nada).
	if !m.enGesto {
		m.enGesto = true
		m.acumulado = xMarcador(m.id, m.r.Margen, ancho, sangrias{m.r.Izq, m.r.Primera, m.r.Der})
	}
	m.acumulado += ev.Dragged.DX / m.escala()
	if m.r.llevar(m.id, m.acumulado, !m.arrastre) {
		m.arrastre = true
	}
}

func (m *marcadorRegla) DragEnd() { m.enGesto, m.arrastre = false, false }

// Tapped le da el foco, para que las flechas lo muevan después.
func (m *marcadorRegla) Tapped(*fyne.PointEvent) {
	if c := fyne.CurrentApp().Driver().CanvasForObject(m); c != nil {
		c.Focus(m)
	}
}

func (m *marcadorRegla) FocusGained() {
	m.foco = true
	m.Refresh()
}

func (m *marcadorRegla) FocusLost() {
	m.foco = false
	m.Refresh()
}

func (m *marcadorRegla) TypedRune(rune) {}

// TypedKey: ← y → mueven el marcador EN PANTALLA 0,25 cm; para la derecha, → reduce la
// sangría, como en la web. Cada pulsación es un paso de deshacer.
func (m *marcadorRegla) TypedKey(ev *fyne.KeyEvent) {
	if ui.SubirEscape(m, ev) { // Escape y «atrás» son del diálogo de encima
		return
	}
	var dir float32
	switch ev.Name {
	case fyne.KeyLeft:
		dir = -1
	case fyne.KeyRight:
		dir = 1
	default:
		return
	}
	ancho, _ := m.r.anchoYCm()
	x := xMarcador(m.id, m.r.Margen, ancho, sangrias{m.r.Izq, m.r.Primera, m.r.Der})
	m.r.llevar(m.id, x+dir*PasoReglaCm*pxPorCm, true)
}

func (m *marcadorRegla) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return &marcadorRenderer{m: m, aro: ui.NuevoAro(tema.RadioSM, ui.SinHueco, v).Abierto()}
}

type marcadorRenderer struct {
	m    *marcadorRegla
	aro  *ui.Aro
	objs []fyne.CanvasObject
}

func (mr *marcadorRenderer) MinSize() fyne.Size           { return fyne.NewSize(anchoMarcador, altoMarcador) }
func (mr *marcadorRenderer) Objects() []fyne.CanvasObject { return mr.objs }
func (mr *marcadorRenderer) Destroy()                     {}

// Layout pinta el triángulo de 10 × 7 dentro de la caja y el aro si tiene el foco.
func (mr *marcadorRenderer) Layout(t fyne.Size) {
	z := t.Width / anchoMarcador
	// Las rutas de la web (`viewBox="0 0 10 7"`) llevadas a la rejilla de 24 de
	// `iconos`: ×2,4. El trazo de 2 en esa rejilla es el de 1 de la web a 0,83.
	d := "M12 2.4l9.6 12H2.4Z" // M5 1l4 5H1Z
	if mr.m.id == marcaPrimera {
		d = "M2.4 2.4h19.2L12 14.4Z" // M1 1h8L5 6Z
	}
	g := iconos.TrazoDeRutaRelleno(d, 10, TintaRegla)
	g.Colocar(2*z, z, 10*z)
	mr.aro.Colocar(t)
	mr.aro.Ver(mr.m.foco)
	mr.objs = mr.aro.Antes(g.Objeto())
}

func (mr *marcadorRenderer) Refresh() {
	mr.aro.Pintar(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	mr.Layout(mr.m.Size())
	canvas.Refresh(mr.m)
}

func (rr *reglaRenderer) Refresh() {
	rr.Layout(rr.r.Size())
	canvas.Refresh(rr.r)
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/SegmentedControl.tsx

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
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

// TallaSegmento es el `size` de `SegmentedControl.tsx`. La pequeña es la de siempre y
// el valor cero, para que los grupos que ya había no cambien.
type TallaSegmento uint8

const (
	// SegmentoSM es `size="sm"`: 24 de alto, `text-xs` medio, la elegida en `text`.
	// Menús y preferencias.
	SegmentoSM TallaSegmento = iota
	// SegmentoMD es `size="md"`: `h-7` (28), `text-sm` seminegrita, `px-2.5`, un
	// `gap-0.5` entre opciones, la elegida en `primary-text` y las demás con
	// `hover:bg-surface`. Cabeceras: la vista del calendario, la de Drive.
	SegmentoMD
)

// medidaSegmento es lo que cambia de una talla a otra.
type medidaSegmento struct {
	alto, margen, hueco, icono float32
	escalon                    tema.Escalon
	peso                       Peso
	radio                      float32
	// activa es la tinta de la elegida; fondoEncima, el relleno de la opción bajo el
	// puntero (vacío: solo cambia la tinta).
	activa, fondoEncima string
}

func (t TallaSegmento) medida() medidaSegmento {
	if t == SegmentoMD {
		// `primary-text` y no `primary`: el relleno no se reasigna en oscuro y sobre
		// `surface` oscuro daba 2,96:1; el rol de texto sí invierte.
		return medidaSegmento{
			alto: 7 * 0.25 * tema.Rem, margen: 2.5 * 0.25 * tema.Rem, hueco: HuecoRejillaSegmento,
			icono: 16, escalon: tema.TextoSM, peso: SemiNegrita, radio: tema.RadioSM,
			activa: "primary-text", fondoEncima: "surface",
		}
	}
	return medidaSegmento{
		alto: AltoSegmento, margen: MargenSegmento, icono: 14, escalon: EscalonSegmento,
		peso: Medio, radio: RadioSegmento, activa: "text",
	}
}

const (
	// HuecoRejillaSegmento es el `gap-0.5` entre opciones de la talla mediana y de la
	// rejilla, y MargenRejillaSegmento el `px-1.5` de cada celda de la rejilla, que
	// trunca antes que desbordar.
	HuecoRejillaSegmento  float32 = 0.5 * 0.25 * tema.Rem
	MargenRejillaSegmento float32 = 1.5 * 0.25 * tema.Rem
	// HuecoIconoSegmento es el `gap-1.5` entre el icono y el rótulo.
	HuecoIconoSegmento float32 = 1.5 * 0.25 * tema.Rem
)

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
	// AlConfirmar avisa cuando la persona da por buena una opción: al pulsarla (aunque ya
	// fuera la elegida), con Intro o con el espacio. Las flechas, Inicio y Fin solo
	// mueven la elección (AlElegir) y no confirman: así un grupo dentro del cajón del
	// teléfono cambia la vista sin cerrarlo a cada flecha, y lo cierra al confirmar,
	// que es el clic sobre el `<aside>` de la web. Opcional.
	AlConfirmar func(int)
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
	// Talla es el `size` de la web; el valor cero es la pequeña. Se fija antes de
	// pintar: el radio de las cajas se decide al crear el renderer.
	Talla TallaSegmento
	// Iconos van delante de cada rótulo, uno por opción (`iconos.Ninguno`, sin icono);
	// nil, ninguno. Con SoloIcono los rótulos no se pintan y quedan como nombre
	// accesible, que es el `soloIcono` de la web (lista y cuadrícula en Drive).
	Iconos    []iconos.Icono
	SoloIcono bool
	// Rejilla reparte las opciones en esas columnas iguales a TODO el ancho que le den,
	// con un `gap-0.5`, y cada celda trunca su rótulo antes que desbordar: el cajón del
	// teléfono del calendario (2×2). Con 0, las opciones van a su ancho natural.
	Rejilla int
	// AlEnfocar avisa al ganar el foco: una tira desplazable revela así el grupo al
	// tabular, como hace con `BotonIcono`.
	AlEnfocar func()
	// SubeTeclas deja que las teclas y letras que el grupo no usa lleguen a la ventana
	// ([SubirTecla], [SubirRuna]), como en [Eleccion]: en Drive el clic en «lista /
	// cuadrícula» le da el foco, y sin esto «v», «?», Supr o F2 justo después no
	// llegaban a nadie. Opcional, por lo mismo que allí.
	SubeTeclas bool

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

// icono dice qué icono lleva la opción i, si lleva.
func (s *Segmentado) icono(i int) (iconos.Icono, bool) {
	if i < len(s.Iconos) && s.Iconos[i] != iconos.Ninguno {
		return s.Iconos[i], true
	}
	return iconos.Ninguno, false
}

// contenido es lo que ocupa lo pintado de la opción i: muestra, icono y rótulo con sus
// huecos, sin el margen.
func (s *Segmentado) contenido(i int) float32 {
	m := s.Talla.medida()
	var ancho float32
	if s.Muestras != nil {
		ancho += LadoMuestra + HuecoMuestra
	}
	if _, si := s.icono(i); si {
		ancho += m.icono
		if !s.SoloIcono {
			ancho += HuecoIconoSegmento
		}
	}
	if !s.SoloIcono {
		ancho += Medir(s.Opciones[i], m.escalon, m.peso).Width
	}
	return ancho
}

// anchoOpcion es lo que ocupa la opción i a su ancho natural: su contenido con el
// margen a cada lado o, con SoloIcono, un cuadrado del alto de la talla.
func (s *Segmentado) anchoOpcion(i int) float32 {
	m := s.Talla.medida()
	if s.SoloIcono {
		return m.alto
	}
	return s.contenido(i) + 2*m.margen
}

// reparto dice dónde va cada opción, lo que mide y lo que ocupa el grupo. Es la ÚNICA
// cuenta: la usan el MinSize, el Layout y el clic. Sin Ancho, una fila; con él, las que
// hagan falta; con Rejilla, columnas iguales en el ancho que tenga el widget.
func (s *Segmentado) reparto() (cajas []fyne.Position, tams []fyne.Size, total fyne.Size) {
	m := s.Talla.medida()
	cajas = make([]fyne.Position, 0, len(s.Opciones))
	tams = make([]fyne.Size, 0, len(s.Opciones))
	if s.Rejilla > 0 {
		cols := s.Rejilla
		filas := (len(s.Opciones) + cols - 1) / cols
		ancho := max((s.Size().Width-2*RellenoSegmentado-float32(cols-1)*HuecoRejillaSegmento)/float32(cols), 0)
		for i := range s.Opciones {
			col, fila := float32(i%cols), float32(i/cols)
			cajas = append(cajas, fyne.NewPos(RellenoSegmentado+col*(ancho+HuecoRejillaSegmento),
				RellenoSegmentado+fila*(m.alto+HuecoRejillaSegmento)))
			tams = append(tams, fyne.NewSize(ancho, m.alto))
		}
		// Como `min-w-0` en la web: el ancho lo pone quien lo coloca, no los rótulos.
		alto := float32(filas)*m.alto + float32(max(filas-1, 0))*HuecoRejillaSegmento
		return cajas, tams, fyne.NewSize(2*RellenoSegmentado+float32(cols-1)*HuecoRejillaSegmento, alto+2*RellenoSegmentado)
	}
	dentro := float32(math.Inf(1))
	if s.Ancho > 0 {
		dentro = s.Ancho - 2*RellenoSegmentado
	}
	var x, y, ancho float32
	for i := range s.Opciones {
		w := s.anchoOpcion(i)
		if x > 0 && x+w > dentro {
			x, y = 0, y+m.alto+m.hueco
		}
		cajas = append(cajas, fyne.NewPos(RellenoSegmentado+x, RellenoSegmentado+y))
		tams = append(tams, fyne.NewSize(w, m.alto))
		x += w
		ancho = max(ancho, x)
		x += m.hueco
	}
	ancho += 2 * RellenoSegmentado
	if y > 0 {
		ancho = s.Ancho // un flex que envuelve ocupa su línea entera
	}
	return cajas, tams, fyne.NewSize(ancho, y+m.alto+2*RellenoSegmentado)
}

// CabeRotulo dice si el rótulo de la opción i entra entero tal como está colocada: en
// la rejilla, con su `px-1.5`. Es el `scrollWidth <= clientWidth` que el banco e2e
// afirma en la web.
func (s *Segmentado) CabeRotulo(i int) bool {
	if i < 0 || i >= len(s.Opciones) {
		return false
	}
	_, tams, _ := s.reparto()
	margen := s.Talla.medida().margen
	if s.Rejilla > 0 {
		margen = MargenRejillaSegmento
	}
	return s.contenido(i) <= tams[i].Width-2*margen
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

// Tapped elige la opción sobre la que se ha pulsado y la confirma.
func (s *Segmentado) Tapped(e *fyne.PointEvent) {
	i := s.enQue(e.Position)
	s.Elegir(i)
	if i >= 0 && i < len(s.Opciones) {
		s.confirmar()
	}
}

// confirmar es la segunda mitad de pulsar, Intro y el espacio: AlConfirmar con la
// opción activa.
func (s *Segmentado) confirmar() {
	if s.AlConfirmar != nil {
		s.AlConfirmar(s.Elegida)
	}
}

// enQue es qué opción cae bajo ese punto. Fuera de todas, -1.
func (s *Segmentado) enQue(p fyne.Position) int {
	pos, tams, _ := s.reparto()
	for i, o := range pos {
		if p.X >= o.X && p.X < o.X+tams[i].Width && p.Y >= o.Y && p.Y < o.Y+tams[i].Height {
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
func (s *Segmentado) FocusGained() {
	s.enfocado = true
	s.Refresh()
	if s.AlEnfocar != nil {
		s.AlEnfocar()
	}
}

// FocusLost lo apaga.
func (s *Segmentado) FocusLost() { s.enfocado = false; s.Refresh() }

// TypedRune: el espacio confirma, como en un radio.
func (s *Segmentado) TypedRune(r rune) {
	if r == ' ' {
		if s.AlElegir != nil {
			s.AlElegir(s.Elegida)
		}
		s.confirmar()
		return
	}
	if s.SubeTeclas {
		SubirRuna(lienzoDe(s), r)
	}
}

// TypedKey mueve con las flechas, que es la navegación de un `radiogroup`. Da la
// vuelta por los dos extremos, como la lista de resultados de la búsqueda.
func (s *Segmentado) TypedKey(e *fyne.KeyEvent) {
	// En la rejilla, ↑/↓ saltan de fila; en una fila son lo mismo que ←/→.
	fila := max(s.Rejilla, 1)
	var paso int
	switch e.Name {
	case fyne.KeyLeft:
		paso = -1
	case fyne.KeyRight:
		paso = 1
	case fyne.KeyUp:
		paso = -fila
	case fyne.KeyDown:
		paso = fila
	case fyne.KeyHome:
		s.Elegir(0)
		return
	case fyne.KeyEnd:
		s.Elegir(len(s.Opciones) - 1)
		return
	case fyne.KeyReturn, fyne.KeyEnter:
		if s.AlElegir != nil {
			s.AlElegir(s.Elegida)
		}
		s.confirmar()
		return
	default:
		if !SubirEscape(s, e) && s.SubeTeclas {
			SubirTecla(lienzoDe(s), e)
		}
		return
	}
	n := len(s.Opciones)
	if n > 0 {
		s.Elegir(((s.Elegida+paso)%n + n) % n)
	}
}

// CreateRenderer construye el carril, la pastilla de la activa y los rótulos.
//
// La sombra de la pastilla va SinInterior: un `box-shadow` no pasa por debajo de su
// caja, y `surface` es un velo en traslúcido que la dejaría ver y oscurecería lo elegido.
func (s *Segmentado) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	m := s.Talla.medida()
	r := &segmentadoRenderer{
		s:       s,
		aro:     NuevoAro(tema.RadioMD, ConHueco, v),
		carril:  NuevaCaja(tema.Color(s.pista(), v), color.NRGBA{}, tema.RadioMD),
		encima:  NuevaCaja(color.NRGBA{}, color.NRGBA{}, m.radio),
		activa:  NuevaCaja(tema.Color("surface", v), color.NRGBA{}, m.radio),
		sombra:  NuevaSombra(tema.SombraXS, m.radio).SinInterior(),
		rotulos: make([]*canvas.Text, 0, len(s.Opciones)),
	}
	r.objetos = r.aro.Antes(r.carril.Objetos()...)
	r.objetos = append(r.objetos, r.encima.Objetos()...)
	r.objetos = append(r.objetos, r.sombra.Objeto())
	r.objetos = append(r.objetos, r.activa.Objetos()...)
	for i, o := range s.Opciones {
		t := Texto(o, m.escalon, m.peso, tema.Color("text-muted", v))
		t.Hidden = s.SoloIcono
		r.rotulos = append(r.rotulos, t)
		r.objetos = append(r.objetos, t)
		var mu *canvas.Raster
		if i < len(s.Muestras) {
			mu = nuevaMuestra(s.Muestras[i])
			r.objetos = append(r.objetos, mu)
		}
		r.muestras = append(r.muestras, mu)
		var ic *iconos.Dibujo
		if cual, si := s.icono(i); si {
			ic = iconos.Trazo(cual, m.icono, "text-muted", v)
			r.objetos = append(r.objetos, ic.Objeto())
		}
		r.iconos = append(r.iconos, ic)
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
	s                      *Segmentado
	aro                    *Aro
	carril, encima, activa *Caja
	sombra                 *Sombra
	rotulos                []*canvas.Text
	// muestras e iconos van por opción; nil donde esa opción no lleva.
	muestras []*canvas.Raster
	iconos   []*iconos.Dibujo
	objetos  []fyne.CanvasObject
}

func (r *segmentadoRenderer) MinSize() fyne.Size {
	_, _, total := r.s.reparto()
	return total
}
func (r *segmentadoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *segmentadoRenderer) Destroy()                     {}

func (r *segmentadoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.carril.Redimensionar(t)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	m := r.s.Talla.medida()

	sitios, tams, _ := r.s.reparto()
	r.encima.Ver(false)
	for i, rotulo := range r.rotulos {
		pos, tam := sitios[i], tams[i]
		if i == r.s.Elegida {
			MoverCaja(r.activa, pos, tam)
			r.sombra.Ajustar(pos, tam, v)
		} else if i == r.s.encima && m.fondoEncima != "" {
			MoverCaja(r.encima, pos, tam)
			r.encima.Ver(true)
		}
		// Lo pintado va centrado en su opción: a su ancho natural es lo mismo que
		// empezar en el margen, y en la rejilla es el `text-center` de la celda.
		margen := m.margen
		if r.s.Rejilla > 0 {
			margen = MargenRejillaSegmento
		}
		fijo := r.s.contenido(i)
		if !r.s.SoloIcono {
			fijo -= Medir(r.s.Opciones[i], m.escalon, m.peso).Width
		}
		var ancho float32
		if !r.s.SoloIcono {
			ancho = Truncar(rotulo, r.s.Opciones[i], m.escalon, m.peso, max(tam.Width-2*margen-fijo, 0))
		}
		x := pos.X + max((tam.Width-fijo-ancho)/2, margen)
		if r.s.SoloIcono {
			x = pos.X + (tam.Width-fijo)/2
		}
		if mu := r.muestras[i]; mu != nil {
			mu.Move(fyne.NewPos(x, pos.Y+(m.alto-LadoMuestra)/2))
			mu.Resize(fyne.NewSize(LadoMuestra, LadoMuestra))
			x += LadoMuestra + HuecoMuestra
		}
		if ic := r.iconos[i]; ic != nil {
			ic.Colocar(x, pos.Y+(m.alto-m.icono)/2, m.icono)
			x += m.icono + HuecoIconoSegmento
		}
		ColocarTexto(rotulo, m.escalon, x, pos.Y+(m.alto-m.escalon.Linea)/2)
	}
}

func (r *segmentadoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	m := r.s.Talla.medida()
	r.carril.Pintar(tema.Color(r.s.pista(), v), color.NRGBA{})
	r.activa.Pintar(tema.Color("surface", v), color.NRGBA{})
	if m.fondoEncima != "" {
		r.encima.Pintar(tema.Color(m.fondoEncima, v), color.NRGBA{})
	}
	r.aro.Pintar(v)
	r.aro.Ver(r.s.enfocado)
	for i, rotulo := range r.rotulos {
		token := "text-muted"
		switch i {
		case r.s.Elegida: // la elegida gana al puntero
			token = m.activa
		case r.s.encima:
			token = "text"
		}
		rotulo.Color = tema.Color(token, v)
		rotulo.Refresh()
		if ic := r.iconos[i]; ic != nil {
			ic.Tenir(token, v)
		}
	}
	r.Layout(r.s.Size())
}

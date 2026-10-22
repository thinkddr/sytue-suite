// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"  // los tres formatos que la web mete en un dataURL
	_ "image/jpeg" //
	_ "image/png"  //
	"math"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus"
	"git.sytue.eu/sytue/one/apps/fyne/editor/figuras"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/externo"
)

// # Escalar el escenario, que es LO específico de una presentación
//
// Una diapositiva es geometría libre sobre un lienzo de proporción fija: las cajas van
// en % y el escenario entero se encoge o se estira para caber en el hueco. La web lo
// resuelve con `transform: scale(k)` sobre un `div` de 960 px, y Fyne no tiene eso:
// no sabe escalar un subárbol ya compuesto.
//
// Y no hace falta que lo sepa. El truco no es escalar un árbol, es no construirlo a
// tamaño fijo: este renderer calcula CADA posición y CADA tamaño en coordenadas del
// escenario y los multiplica por `k` al colocarlos. Un `transform` de CSS es
// exactamente eso, solo que el navegador lo hace después y nosotros antes.
//
// Lo que se gana, medido (TestElEscenarioEscalaSinRasterizar): el texto se CONFORMA al
// tamaño final —44 px × k— en vez de rasterizarse a 44 y estirarse, así que una
// miniatura de 128 px de ancho y el escenario a pantalla completa salen los dos
// limpios. Es lo contrario de lo que le pasa a `canvas.Image`, que rasteriza y luego
// estira el mapa de bits con ApproxBiLinear —el motivo de que los iconos vayan con
// `canvas.Raster`—.
//
// Lo que cuesta: una re-composición entera cada vez que cambia `k`, porque no hay
// subárbol que reutilizar. Es aritmética sobre las cajas de UNA diapositiva; con las
// nueve de la muestra no se mide.

// Colores de CONTENIDO del lienzo. No son tokens de interfaz: son los hex que el
// usuario aplica a su documento, fijados en `editor/ribbon/primitives.tsx` (§5), y por
// eso van literales y no por `tema.Color`.
var (
	tinta      = color.NRGBA{R: 0x0A, G: 0x0A, B: 0x0A, A: 255} // CONTENT_PALETTE[0]
	rellenoDef = color.NRGBA{R: 0x47, G: 0x55, B: 0x69, A: 255} // CONTENT_PALETTE[1], SHAPE_FILL
	papel      = color.NRGBA{R: 255, G: 255, B: 255, A: 255}    // --editor-page-bg
)

// Visuales es `SlideVisuals`: el fondo y la tinta efectivos de una diapositiva.
type Visuales struct {
	Fondo, Tinta color.NRGBA
	// Familia es la letra del tema («Georgia» en «arena»; vacía, la de la suite): el
	// `--font-sans` que `visualsStyle` pone en el escenario y leen las cajas de texto.
	Familia string
}

// temas son SLIDE_THEMES. La letra de «arena» es Georgia, que la suite no puede llevar
// dentro (su licencia no lo deja): se pinta con la del equipo, como el navegador, y si no
// la tiene con la que el sistema dé por ella (ver tema.CaraDelSistema).
var temas = map[string]Visuales{
	"carbon":  {Fondo: tinta, Tinta: papel},
	"pizarra": {Fondo: rellenoDef, Tinta: papel},
	"arena":   {Fondo: color.NRGBA{R: 0xFE, G: 0xD7, B: 0xAA, A: 255}, Tinta: tinta, Familia: "Georgia"},
	"menta":   {Fondo: color.NRGBA{R: 0xBB, G: 0xF7, B: 0xD0, A: 255}, Tinta: tinta},
	"cielo":   {Fondo: color.NRGBA{R: 0xBF, G: 0xDB, B: 0xFE, A: 255}, Tinta: tinta},
}

// Visuales resuelve fondo y tinta de la diapositiva `i`: el fondo propio manda sobre
// el del tema, la tinta viene del tema. Es `slideVisuals`.
func (m *Modelo) Visuales(i int) Visuales {
	v := Visuales{Fondo: papel, Tinta: tinta}
	if t, ok := temas[m.Tema]; ok {
		v = t
	} else if t, ok := m.temaPropio(m.Tema); ok {
		v = t
	}
	if i >= 0 && i < len(m.Diapos) {
		if f := m.Diapos[i].Fondo; f != "" && f != "transparent" {
			if c, ok := editor.Hex(f); ok {
				v.Fondo = c
			}
		}
	}
	return v
}

// temaPropio resuelve el tema propio con ese id: su fondo y su tinta (que han de ser un
// `#RRGGBB`; si no, el tema no vale y se pinta Papel) y su fuente.
func (m *Modelo) temaPropio(id string) (Visuales, bool) {
	for _, t := range m.Temas {
		if t.ID != id {
			continue
		}
		fondo, okF := editor.Hex(t.Fondo)
		tinta, okT := editor.Hex(t.Tinta)
		if !okF || !okT {
			return Visuales{}, false
		}
		return Visuales{Fondo: fondo, Tinta: tinta, Familia: t.Fuente}, true
	}
	return Visuales{}, false
}

// FIT_GAP de SlidesCanvas.tsx: el aire que el escenario deja alrededor dentro del área
// de desplazamiento.
const HuecoEncaje float32 = 64

// Encaje es la escala del escenario: auto-encaje al hueco × zoom. Es el cálculo de
// `scale` en SlidesCanvas.tsx, con su regla, que no es la obvia.
//
// El ANCHO manda siempre; el alto solo si tiene hueco. Tomar el mínimo de los dos y
// caer a un valor fijo cuando sale ≤ 0 es lo que hacía la web antes, y en un teléfono
// cuyo hueco medía ~130 px la diapositiva se salía +372 px (medido por el barrido móvil
// en un iPhone 12). Sin medida no se inventa un tamaño: se pinta al mínimo y se encaja
// en cuanto haya una.
func (m *Modelo) Encaje(area fyne.Size, hueco, zoom float32) float32 {
	anchoK := (area.Width - hueco) / AnchoEscenario
	altoK := (area.Height - hueco) / m.AltoEscenario()
	var k float32
	if anchoK > 0 {
		k = anchoK
		if altoK > 0 {
			k = min(anchoK, altoK)
		}
	}
	return max(0.08, k*zoom/100)
}

// Lienzo pinta UNA diapositiva, a la escala que le dé su tamaño.
//
// El mismo widget es el escenario del editor, la miniatura del carril y la diapositiva
// a pantalla completa: lo único que cambia entre los tres es qué tamaño se le da y
// cuánto aire deja (Hueco). Tener tres no daría ni un píxel distinto y sí tres sitios
// donde se pinta un `h1` con tamaños que se van separando.
type Lienzo struct {
	widget.BaseWidget

	M      *Modelo
	Indice int
	// Hueco es el aire alrededor del escenario dentro del widget: HuecoEncaje en el
	// escenario del editor, 0 en la miniatura y en la presentación.
	Hueco float32
	// Zoom en % (100 = encajar). Solo el escenario del editor lo mueve.
	Zoom float32
	// ConPasos pinta solo las cajas presentes en el paso Paso de la presentación; sin
	// él se pintan todas, que es lo que enseñan el escenario y las miniaturas.
	ConPasos bool
	Paso     int
	// Escala, si no es cero, manda sobre el encaje: es la del escenario del editor, que
	// se calcula con el hueco VISIBLE del área y no con el tamaño del propio lienzo
	// (con zoom, el lienzo es más grande que lo que se ve y se desplaza).
	Escala float32
	// Rotulo traduce una clave del catálogo; sin él, las cajas que no se pintan salen
	// con su clave, que es lo que hace `i18n.Translator.T` con una clave suelta.
	Rotulo   func(clave string) string
	Variante tema.Variante
	// Oculta es la caja de texto que se está escribiendo: su texto lo pintan los
	// editores de encima, y pintarlo también aquí lo doblaría.
	Oculta string
	// AlinearDefecto es el `text-align` que HEREDAN los bloques que no declaran el suyo.
	// Vacío en el escenario y en la presentación; "center" en la miniatura, porque la
	// web la pinta dentro de un `<button>` y el navegador centra el texto de un botón:
	// en el carril, lo que en el escenario va a la izquierda sale centrado.
	AlinearDefecto string
	// Radio redondea las esquinas del fondo: el `border-radius: 4px` de las
	// diapositivas de la vista del moderador.
	Radio float32
	// Patron pinta, sobre el fondo de la diapositiva, las cajas del patrón tal cual —con
	// sus campos {n} y {total} sin sustituir— y nada más: es el escenario de «Editar
	// patrón», donde esas cajas son las que se editan.
	Patron bool
	// Conector es el conector elegido en el escenario, que lleva el halo del acento.
	Conector string
	// Saliendo son las cajas que hacen su salida ANIMADA en este paso: aún se pintan,
	// y su efecto las deja invisibles (el `fill both` de la web).
	Saliendo map[string]bool

	r *lienzoRenderer
}

var _ fyne.Widget = (*Lienzo)(nil)

// NuevoLienzo crea el lienzo de una diapositiva, encajado y sin zoom.
func NuevoLienzo(m *Modelo, i int) *Lienzo {
	l := &Lienzo{M: m, Indice: i, Zoom: 100, Variante: tema.Claro}
	l.ExtendBaseWidget(l)
	return l
}

// MedidaEscenario es el tamaño que ocupa el escenario dentro de un widget de `t`: la
// caja de proporción fija más grande que cabe, y el aire de `Hueco` alrededor.
func (l *Lienzo) MedidaEscenario(t fyne.Size) fyne.Size {
	k := l.escala(t)
	return fyne.NewSize(AnchoEscenario*k, l.M.AltoEscenario()*k)
}

func (l *Lienzo) escala(t fyne.Size) float32 {
	if l.Escala > 0 {
		return l.Escala
	}
	z := l.Zoom
	if z == 0 {
		z = 100
	}
	return l.M.Encaje(t, l.Hueco, z)
}

// AccessibilityLabel anuncia qué diapositiva es. Fyne no tiene rol de «imagen», así
// que el contenedor se anuncia como texto con el número, que es lo que la web pone en
// el `aria-label` del carril (`slides.aria.slideOf`).
func (l *Lienzo) AccessibilityLabel() string {
	if l.Rotulo == nil {
		return ""
	}
	if l.Patron {
		return l.Rotulo("slides.master.aria")
	}
	return l.Rotulo("slides.aria.slideOf")
}

// AccessibilityRole es contenedor: la diapositiva agrupa sus cajas.
func (l *Lienzo) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// CreateRenderer compone la diapositiva.
func (l *Lienzo) CreateRenderer() fyne.WidgetRenderer {
	r := &lienzoRenderer{l: l, fondo: canvas.NewRectangle(l.M.Visuales(l.Indice).Fondo)}
	l.r = r
	r.rehacer(l.Size())
	return r
}

type lienzoRenderer struct {
	l       *Lienzo
	fondo   *canvas.Rectangle
	objetos []fyne.CanvasObject
	// El tamaño con el que se compuso. No hay subárbol que reutilizar —esa es la
	// contrapartida de multiplicar al colocar—, así que se recompone cuando cambia,
	// y solo cuando cambia.
	compuesto fyne.Size
	// porCaja son las primitivas de cada caja tal como se compusieron, para que los
	// efectos de la presentación las muevan y las atenúen sin recomponer (ver Efecto).
	porCaja map[string][]primitiva
	// centros y esquinas son el centro y la esquina de arriba a la izquierda de cada caja:
	// los orígenes de sus transformaciones.
	centros, esquinas map[string]fyne.Position
}

// primitiva es un objeto de una caja con su posición, su tamaño y sus colores de reposo.
// Una `local` va dentro del recorte de su caja (ver `recortada`): su posición es relativa
// a él, que ya se mueve y se escala con la caja, así que ella se escala desde su esquina.
type primitiva struct {
	o      fyne.CanvasObject
	pos    fyne.Position
	tam    fyne.Size
	letra  float32
	c1, c2 color.NRGBA
	local  bool
}

// Efecto pone la caja `id` a la opacidad `alfa`, desplazada `d` px de su sitio y a la
// escala `s` alrededor de su centro (el `transform-origin` por defecto): un fotograma
// de su animación. Con (1, 0, 1) vuelve al reposo. Solo con ConPasos, que es la
// presentación.
func (l *Lienzo) Efecto(id string, alfa float32, d fyne.Delta, s float32) {
	l.Transformar(id, alfa, d, s, s, true)
}

// Transformar es Efecto con una escala por eje y el origen en el centro de la caja o,
// con `centro` false, en su esquina de arriba a la izquierda (el `transform-origin: 0 0`
// del morph).
//
// ponytail: la letra no se estira por ejes (un `canvas.Text` tiene un solo tamaño): toma
// la escala vertical. Solo se ve en un morph entre cajas de proporción distinta.
func (l *Lienzo) Transformar(id string, alfa float32, d fyne.Delta, sx, sy float32, centro bool) {
	if l.r == nil {
		return
	}
	for _, p := range l.r.porCaja[id] {
		c, m := l.r.centros[id], d
		if !centro {
			c = l.r.esquinas[id]
		}
		if p.local {
			c, m = fyne.Position{}, fyne.Delta{}
		}
		pos := fyne.NewPos(c.X+(p.pos.X-c.X)*sx+m.DX, c.Y+(p.pos.Y-c.Y)*sy+m.DY)
		p.o.Move(pos)
		if tam := fyne.NewSize(p.tam.Width*sx, p.tam.Height*sy); tam != p.o.Size() {
			p.o.Resize(tam)
		}
		switch o := p.o.(type) {
		case *canvas.Text:
			o.Color, o.TextSize = conAlfa(p.c1, alfa), p.letra*sy
		case *canvas.Raster:
			o.Translucency = float64(1 - alfa)
		case *canvas.Image:
			o.Translucency = float64(1 - alfa)
		case *canvas.Rectangle:
			o.FillColor, o.StrokeColor = conAlfa(p.c1, alfa), conAlfa(p.c2, alfa)
		}
		canvas.Refresh(p.o)
	}
}

func nrgba(c color.Color) color.NRGBA {
	if c == nil {
		return color.NRGBA{}
	}
	n, _ := color.NRGBAModel.Convert(c).(color.NRGBA)
	return n
}

func primitivas(objs []fyne.CanvasObject) []primitiva {
	return primitivasEn(objs, false)
}

func primitivasEn(objs []fyne.CanvasObject, local bool) []primitiva {
	fuera := make([]primitiva, 0, len(objs))
	for _, o := range objs {
		p := primitiva{o: o, pos: o.Position(), tam: o.Size(), local: local}
		if r, ok := o.(*container.Clip); ok {
			fuera = append(fuera, p)
			if dentro, ok := r.Content.(*fyne.Container); ok {
				fuera = append(fuera, primitivasEn(dentro.Objects, true)...)
			}
			continue
		}
		switch x := o.(type) {
		case *canvas.Text:
			p.c1, p.letra = nrgba(x.Color), x.TextSize
		case *canvas.Rectangle:
			p.c1 = nrgba(x.FillColor)
			p.c2 = nrgba(x.StrokeColor)
		}
		fuera = append(fuera, p)
	}
	return fuera
}

func (r *lienzoRenderer) MinSize() fyne.Size           { return fyne.NewSize(1, 1) }
func (r *lienzoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *lienzoRenderer) Destroy()                     {}

func (r *lienzoRenderer) Layout(t fyne.Size) {
	if t != r.compuesto {
		r.rehacer(t)
	}
}

func (r *lienzoRenderer) Refresh() {
	r.rehacer(r.l.Size())
	canvas.Refresh(r.l)
}

// rehacer compone las cajas a la escala que toque, con su posición ABSOLUTA ya puesta.
// Es lo que en la web hace el `transform: scale(k)` del navegador; aquí se multiplica
// al colocar.
//
// Las posiciones son absolutas y no relativas al escenario a propósito: un `Layout`
// que desplazara lo ya colocado acumularía el desplazamiento en cada pasada, que es un
// fallo que solo se ve al redimensionar dos veces.
func (r *lienzoRenderer) rehacer(t fyne.Size) {
	r.compuesto = t
	k := r.l.escala(t)
	v := r.l.M.Visuales(r.l.Indice)
	sh := r.l.M.AltoEscenario()

	esc := fyne.NewSize(AnchoEscenario*k, sh*k)
	// Centrado en el hueco, como el `m-auto` del área de la web.
	o := fyne.NewPos((t.Width-esc.Width)/2, (t.Height-esc.Height)/2)
	r.fondo.FillColor = v.Fondo
	r.fondo.CornerRadius = r.l.Radio
	r.fondo.Resize(esc)
	r.fondo.Move(o)

	r.objetos = []fyne.CanvasObject{r.fondo}
	r.porCaja, r.centros, r.esquinas = map[string][]primitiva{}, map[string]fyne.Position{}, map[string]fyne.Position{}
	// Las líneas de las formas del diseño van con el patrón: encima de sus cajas y debajo de
	// las de la diapositiva, como la `ConnectorLayer` de `layoutConnectorsFor`.
	visibles := r.l.cajasVisibles()
	patron := 0
	if !r.l.Patron && r.l.Indice >= 0 && r.l.Indice < len(r.l.M.Diapos) {
		patron = len(r.l.M.cajasPatron(r.l.Indice))
	}
	for n, c := range visibles {
		if n == patron && patron > 0 {
			r.lineasDelDiseno(o, sh, esc, v)
		}
		if c.ID == r.l.Oculta && r.l.Oculta != "" {
			continue
		}
		pos := o.AddXY(float32(c.X)/100*AnchoEscenario*k, float32(c.Y)/100*sh*k)
		tam := fyne.NewSize(float32(c.W)/100*AnchoEscenario*k, float32(c.H)/100*sh*k)
		objs := r.l.pintarCaja(c, pos, tam, k, v)
		if r.l.ConPasos {
			r.porCaja[c.ID] = primitivas(objs)
			r.centros[c.ID] = pos.AddXY(tam.Width/2, tam.Height/2)
			r.esquinas[c.ID] = pos
		}
		r.objetos = append(r.objetos, objs...)
	}
	if patron > 0 && patron >= len(visibles) {
		r.lineasDelDiseno(o, sh, esc, v) // una diapositiva sin cajas propias
	}
	// Los conectores van ENCIMA de las cajas y se calculan con las de la diapositiva
	// (no las del patrón), también en la presentación, como `ConnectorLayer`.
	if i := r.l.Indice; !r.l.Patron && i >= 0 && i < len(r.l.M.Diapos) {
		d := r.l.M.Diapos[i]
		if svg := svgConectores(d.Conectores, d.Cajas, float64(sh), hexDe(v.Tinta), r.l.Conector,
			hexDe(tema.Color("primary", r.l.Variante))); svg != nil {
			r.objetos = append(r.objetos, colocado(iconos.DeSVG(svg, esc), o))
		}
	}
}

// lineasDelDiseno pinta las líneas de las formas del diseño de la diapositiva en curso.
func (r *lienzoRenderer) lineasDelDiseno(o fyne.Position, sh float32, esc fyne.Size, v Visuales) {
	con := r.l.M.ConectoresDiseno(r.l.Indice)
	if len(con) == 0 {
		return
	}
	if svg := svgConectores(con, nil, float64(sh), hexDe(v.Tinta), "", ""); svg != nil {
		r.objetos = append(r.objetos, colocado(iconos.DeSVG(svg, esc), o))
	}
}

// cajasVisibles son las cajas que se pintan: las del patrón siempre y, con ConPasos,
// de las propias solo las presentes en ese paso (`boxPresentAtStep`).
func (l *Lienzo) cajasVisibles() []Caja {
	if l.Patron {
		return l.M.cajas(IndicePatron)
	}
	todas := l.M.CajasResueltas(l.Indice)
	if !l.ConPasos || l.Indice < 0 || l.Indice >= len(l.M.Diapos) {
		return todas
	}
	d := l.M.Diapos[l.Indice]
	patron := len(todas) - len(d.Cajas)
	por := PorCaja(Plan(d))
	fuera := todas[:patron:patron]
	for _, c := range d.Cajas {
		if PresenteEn(por[c.ID], l.Paso) || l.Saliendo[c.ID] {
			// Tras una trayectoria, la caja se queda donde la dejó (la traslación
			// estática de `motionRestOffset`).
			r := DesfaseReposo(por[c.ID], l.Paso)
			c.X, c.Y = c.X+r.DX, c.Y+r.DY
			fuera = append(fuera, c)
		}
	}
	return fuera
}

// pintarCaja devuelve las primitivas de UNA caja, ya colocadas en coordenadas del
// escenario escaladas. Es `renderBox`: el mismo camino para el escenario, las
// miniaturas y la presentación.
func (l *Lienzo) pintarCaja(c Caja, pos fyne.Position, tam fyne.Size, k float32, v Visuales) []fyne.CanvasObject {
	sh := float64(l.M.AltoEscenario())
	vw, vh := c.W/100*float64(AnchoEscenario), c.H/100*sh
	switch c.Tipo {
	case "", "text":
		return l.pintarTexto(c, pos, tam, k, v)
	case "shape":
		return []fyne.CanvasObject{colocado(iconos.DeSVG(svgForma(c, vw, vh), tam), pos)}
	case "chart":
		if c.Grafico == nil {
			return nil
		}
		return l.pintarGrafico(c.Grafico, vw, vh, pos, tam, k)
	case "smartart":
		if c.Diagrama == nil {
			return nil
		}
		return l.pintarSmart(c.Diagrama, vw, vh, pos, tam, k, v)
	case "media":
		return l.pintarMedio(c, vw, vh, pos, tam, k, v)
	case "image":
		if img, ok := imagenDe(c.Src); ok {
			i := canvas.NewImageFromImage(img)
			i.FillMode = canvas.ImageFillContain // `object-contain`
			i.Resize(tam)
			i.Move(pos)
			return []fyne.CanvasObject{i}
		}
	}
	return l.marcador(c, pos, tam, k)
}

// colocado pone el dibujo en `pos` y devuelve su objeto.
func colocado(d *iconos.Dibujo, pos fyne.Position) fyne.CanvasObject {
	d.Objeto().Move(pos)
	return d.Objeto()
}

// pintarTexto es el `div` con TEXT_BOX_CLASS: el HTML de la caja con la hoja del lienzo,
// maquetado y pintado por el folio de Docus (ver `texto.go`), con la tinta de la
// diapositiva y recortado a la caja, que es el `overflow-hidden` de la web: una línea que
// no cabe entera se corta a mitad, no desaparece.
func (l *Lienzo) pintarTexto(c Caja, pos fyne.Position, tam fyne.Size, k float32, v Visuales) []fyne.CanvasObject {
	objs := docus.PintarCaja(c.HTML, cajaFolio(tam, k, hojaCaja(v.Tinta, v.Familia, l.AlinearDefecto)), fyne.NewPos(0, 0))
	if len(objs) == 0 {
		return nil
	}
	return []fyne.CanvasObject{recortada(objs, pos, tam)}
}

// recortada mete los objetos (en coordenadas de la caja) en un recorte del tamaño de la
// caja, colocado en `pos`: lo que se salga no se pinta.
func recortada(objs []fyne.CanvasObject, pos fyne.Position, tam fyne.Size) fyne.CanvasObject {
	r := container.NewClip(container.NewWithoutLayout(objs...))
	r.Move(pos)
	r.Resize(tam)
	return r
}

// pintarGrafico es `SlideChartView`: las marcas en SVG y los rótulos encima; sin datos,
// `slides.chart.noData` centrado en el gris del gráfico.
func (l *Lienzo) pintarGrafico(g *Grafico, vw, vh float64, pos fyne.Position, tam fyne.Size, k float32) []fyne.CanvasObject {
	gc := componerGrafico(g, vw, vh)
	gris, _ := editor.Hex(apagadoGrafico)
	if gc.vacio {
		return l.textoCentrado(l.rotulo("slides.chart.noData"), float32(gc.letra+1), 1.5, gris, ui.Normal,
			pos, tam, k, 2*tema.Espacio(2))
	}
	fuera := []fyne.CanvasObject{colocado(iconos.DeSVG(gc.svg, tam), pos)}
	for _, r := range gc.rotulos {
		c, _ := editor.Hex(r.Color)
		fuera = append(fuera, editor.RotuloEnBase(r, pos, k, c, ui.Normal))
	}
	return fuera
}

// textoCentrado es el `grid place-items-center px-2 text-center` de los vacíos del
// gráfico y del diagrama: el texto repartido y centrado en los dos ejes. `tamLetra` va
// en px del escenario; `hueco` es el relleno horizontal total, ya en px de pantalla.
func (l *Lienzo) textoCentrado(texto string, tamLetra, interlinea float32, c color.NRGBA, peso ui.Peso,
	pos fyne.Position, tam fyne.Size, k, hueco float32,
) []fyne.CanvasObject {
	e := tema.Escalon{Tam: tamLetra * k, Linea: tamLetra * interlinea * k}
	lineas := ui.Lineas(texto, e, peso, tam.Width-hueco)
	y := pos.Y + (tam.Height-float32(len(lineas))*e.Linea)/2
	fuera := make([]fyne.CanvasObject, 0, len(lineas))
	for _, ln := range lineas {
		t := ui.Texto(ln, e, peso, c)
		ui.ColocarTexto(t, e, pos.X+(tam.Width-ui.Medir(ln, e, peso).Width)/2, y)
		fuera = append(fuera, t)
		y += e.Linea
	}
	return fuera
}

// pintarSmart es `SmartArtView`: nodos y conectores en SVG con la tinta de la
// diapositiva, y el texto de cada nodo repartido y centrado (a la izquierda, con su
// viñeta, en la lista), recortado a lo que cabe en el nodo.
func (l *Lienzo) pintarSmart(d *Diagrama, vw, vh float64, pos fyne.Position, tam fyne.Size, k float32, v Visuales) []fyne.CanvasObject {
	disp := d.Disposicion
	if !disposicionesSmart[disp] {
		disp = "list"
	}
	nodos, con := geometriaSmart(disp, d.Elementos, vw, vh)
	if len(nodos) == 0 {
		apagada := v.Tinta
		apagada.A = uint8(float32(apagada.A) * 0.6)
		return l.textoCentrado(l.rotulo("smartart.empty"), float32(acotar(vh*0.06, 8, 13)), 1.5, apagada,
			ui.Normal, pos, tam, k, 2*tema.Espacio(2))
	}
	fuera := []fyne.CanvasObject{colocado(iconos.DeSVG(figuras.SVGSmart(nodos, con, vw, vh, hexDe(v.Tinta)), tam), pos)}
	vineta := disp == "list"
	for _, nd := range nodos {
		fuera = append(fuera, editor.TextoNodoSmart(nd, d.Elementos[nd.I].Texto, vineta, pos, k, v.Tinta)...)
	}
	return fuera
}

// pintarMedio es `MediaPlaceholder`: el editor y las miniaturas no reproducen; enseñan
// el cartel del vídeo si lo hay y, encima, el glifo del tipo, o sin cartel el glifo
// sobre un velo de la tinta al 8 %.
func (l *Lienzo) pintarMedio(c Caja, vw, vh float64, pos fyne.Position, tam fyne.Size, k float32, v Visuales) []fyne.CanvasObject {
	audio := c.TipoMedio == "audio"
	lado := float32(math.Round(acotar(min(vw, vh)*0.3, 12, 72)))
	relleno := float32(math.Round(float64(lado) * 0.24))
	var fuera []fyne.CanvasObject
	var cartel image.Image
	if !audio && c.Cartel != "" {
		cartel, _ = imagenDe(c.Cartel)
	}
	icono := iconos.EdVideo
	if audio {
		icono = iconos.EdAudio
	}
	centro := pos.AddXY(tam.Width/2, tam.Height/2)
	ladoK := lado * k
	if cartel != nil {
		i := canvas.NewImageFromImage(cartel)
		i.FillMode = canvas.ImageFillContain
		i.Resize(tam)
		i.Move(pos)
		disco := canvas.NewCircle(color.NRGBA{A: 115}) // el negro al 45 % de la web
		r := ladoK/2 + relleno*k
		disco.Resize(fyne.NewSize(2*r, 2*r))
		disco.Move(centro.SubtractXY(r, r))
		g := iconos.TrazoEdConTinta(icono, ladoK, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		g.Colocar(centro.X-ladoK/2, centro.Y-ladoK/2, ladoK)
		return append(fuera, i, disco, g.Objeto())
	}
	velo := v.Tinta
	velo.A = uint8(float32(velo.A) * 0.08)
	fondo := canvas.NewRectangle(velo)
	fondo.Resize(tam)
	fondo.Move(pos)
	tenue := v.Tinta
	tenue.A = uint8(float32(tenue.A) * 0.72)
	g := iconos.TrazoEdConTinta(icono, ladoK, tenue)
	g.Colocar(centro.X-ladoK/2, centro.Y-ladoK/2, ladoK)
	return append(fuera, fondo, g.Objeto())
}

// marcador es lo que se pinta donde hay algo que este lienzo NO sabe dibujar —hoy, una
// imagen que no llega como `data:`—: un borde y el nombre de lo que va ahí, con la misma
// clave de catálogo que usa la web (`slides.box.*`). Ocupa el sitio exacto del objeto
// para que el resto de la diapositiva no se lea desplazado, y `NoPintado` lo cuenta.
func (l *Lienzo) marcador(c Caja, pos fyne.Position, tam fyne.Size, k float32) []fyne.CanvasObject {
	caja := ui.NuevaCaja(tema.Color("bg-subtle", l.Variante), tema.Color("border", l.Variante), tema.RadioSM)
	caja.Redimensionar(tam)
	fuera := caja.Objetos()
	for _, o := range fuera {
		o.Move(o.Position().Add(pos))
	}
	e := tema.Escalon{Tam: tamCuerpo * k, Linea: tamCuerpo * interlineado * k}
	t := ui.Texto(l.rotulo(claveDe(c)), e, ui.Medio, tema.Color("text-muted", l.Variante))
	m := ui.Medir(t.Text, e, ui.Medio)
	ui.ColocarTexto(t, e, pos.X+(tam.Width-m.Width)/2, pos.Y+(tam.Height-e.Linea)/2)
	return append(fuera, t)
}

// hexDe escribe un color como `#RRGGBB` para meterlo en un SVG.
func hexDe(c color.NRGBA) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }

func (l *Lienzo) rotulo(clave string) string {
	if l.Rotulo == nil {
		return clave
	}
	return l.Rotulo(clave)
}

// claveDe es `boxLabel`: la clave de catálogo con la que la web nombra cada tipo de
// objeto.
func claveDe(c Caja) string {
	switch c.Tipo {
	case "image":
		return "slides.box.image"
	case "chart":
		return "slides.box.chart"
	case "smartart":
		return "slides.box.smartart"
	case "shape":
		return "slides.box.shape"
	case "media":
		if c.TipoMedio == "audio" {
			return "slides.box.audio"
		}
		return "slides.box.video"
	}
	return "slides.box.shape"
}

// imagenDe decodifica un `data:` URL. Es el caso real: `Insertar.tsx` reescala la
// imagen en el cliente y la guarda como dataURL de menos de ~700 KiB dentro del propio
// modelo, así que la presentación se pinta entera sin pedir nada a la red.
//
// Una URL http(s) NO se descarga aquí: un widget que hace peticiones al pintar es un
// widget que se cuelga al pintar. Sale como marcador.
func imagenDe(src string) (image.Image, bool) {
	coma := strings.IndexByte(src, ',')
	if !strings.HasPrefix(src, "data:") || coma < 0 || !strings.Contains(src[:coma], ";base64") {
		return nil, false
	}
	datos, err := base64.StdEncoding.DecodeString(src[coma+1:])
	if err != nil {
		return nil, false
	}
	// Con la barrera común de tamaño (FYE-04): una imagen incrustada de 20000×20000
	// comprime a pocos KiB y pedía 1,6 GiB al pintarla.
	img, err := externo.Decodificar(datos)
	if err != nil {
		return nil, false
	}
	return img, true
}

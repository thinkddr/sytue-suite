// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"crypto/rand"
	"fmt"
	"image/color"
	"math"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Las muestras de color de Calendar: `ColorSwatches` de CalendarModal.tsx, un grupo de
// radios con un círculo por tono, que usan el formulario de un calendario, el de un
// evento (con «color del calendario» delante) y el de una suscripción.
//
//	grid h-7 w-7 place-items-center rounded-full border border-border
//	elegido: shadow 0 0 0 2px surface, 0 0 0 4px border-strong · tic de 14
//	«sin color»: borde DISCONTINUO y el tic en `text`
//
// NO es `ui.MuestraColor`, que es la de CertiPDF (`h-5`, sin anillo, escalada al
// elegirla): la paridad manda sobre la reutilización.
var (
	// LadoMuestra es el `h-7 w-7`.
	LadoMuestra = tema.Espacio(7)
	// HuecoMuestras es el `gap-2`.
	HuecoMuestras = tema.Espacio(2)
)

// celdaMuestra y huecoMuestras son la caja de cada tono y su separación. Con el dedo, el
// blanco es de 44 con el círculo de 28 en su centro y sin hueco entre blancos: la web
// hace lo mismo con `pointer-coarse:after:-inset-2 gap-4` (28 + 2×8 = 44 de paso), y
// caben siete por fila en la hoja de 390.
func celdaMuestra() float32 {
	if ui.EsTactil() {
		return ui.SueloTactilBarra
	}
	return LadoMuestra
}

func huecoMuestras() float32 {
	if ui.EsTactil() {
		return 0
	}
	return HuecoMuestras
}

// grupoMuestras es el `role="radiogroup"`: las flechas mueven la elección.
type grupoMuestras struct {
	widget.BaseWidget
	muestras []*muestraCal
	elegida  int
	alElegir func(hex string)
}

// nuevasMuestras arma el grupo. Con `ninguno`, la primera es «color del calendario»
// (valor vacío).
func nuevasMuestras(p *Pantalla, valor string, ninguno bool, alElegir func(hex string)) *grupoMuestras {
	g := &grupoMuestras{alElegir: alElegir, elegida: -1}
	items := PaletaCalendario
	if ninguno {
		items = append([]struct{ Clave, Hex string }{{"colour.inherit", ""}}, items...)
	}
	for i, it := range items {
		m := &muestraCal{g: g, i: i, hex: it.Hex, nombre: p.t(it.Clave, nil)}
		m.ExtendBaseWidget(m)
		if strings.EqualFold(it.Hex, valor) {
			g.elegida = i
		}
		g.muestras = append(g.muestras, m)
	}
	g.ExtendBaseWidget(g)
	return g
}

func (g *grupoMuestras) elegir(i int) {
	g.elegida = (i + len(g.muestras)) % len(g.muestras)
	for _, m := range g.muestras {
		m.Refresh()
	}
	if g.alElegir != nil {
		g.alElegir(g.muestras[g.elegida].hex)
	}
}

// MinSize deja encoger el grupo. El alto depende del ancho que tenga el diálogo.
func (g *grupoMuestras) MinSize() fyne.Size {
	return fyne.NewSize(celdaMuestra(), celdaMuestra())
}

func (g *grupoMuestras) AltoPara(ancho float32) float32 {
	return (&envolver{Hueco: huecoMuestras(), Ancho: ancho}).MinSize(g.objetos()).Height
}

func (g *grupoMuestras) objetos() []fyne.CanvasObject {
	o := make([]fyne.CanvasObject, len(g.muestras))
	for i, m := range g.muestras {
		o[i] = m
	}
	return o
}

func (g *grupoMuestras) CreateRenderer() fyne.WidgetRenderer {
	return &grupoMuestrasRenderer{g: g, c: container.New(&envolver{Hueco: huecoMuestras()}, g.objetos()...)}
}

type grupoMuestrasRenderer struct {
	g *grupoMuestras
	c *fyne.Container
}

func (r *grupoMuestrasRenderer) MinSize() fyne.Size           { return r.g.MinSize() }
func (r *grupoMuestrasRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.c} }
func (r *grupoMuestrasRenderer) Destroy()                     {}
func (r *grupoMuestrasRenderer) Refresh()                     { r.c.Refresh() }
func (r *grupoMuestrasRenderer) Layout(t fyne.Size)           { r.c.Resize(t) }

// muestraCal es un tono: un radio que se pinta como un círculo.
type muestraCal struct {
	widget.BaseWidget
	g      *grupoMuestras
	i      int
	hex    string
	nombre string

	enfocada bool
}

var (
	_ fyne.Tappable   = (*muestraCal)(nil)
	_ fyne.Focusable  = (*muestraCal)(nil)
	_ fyne.Accessible = (*muestraCal)(nil)
)

func (m *muestraCal) Tapped(*fyne.PointEvent) { m.g.elegir(m.i) }
func (m *muestraCal) FocusGained()            { m.enfocada = true; m.Refresh() }
func (m *muestraCal) FocusLost()              { m.enfocada = false; m.Refresh() }
func (m *muestraCal) TypedRune(r rune) {
	if r == ' ' {
		m.g.elegir(m.i)
	}
}

// TypedKey: las flechas eligen el siguiente o el anterior y llevan el foco con ellos,
// que es el `roving tabindex` de la web.
func (m *muestraCal) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(m, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	paso := 0
	switch e.Name {
	case fyne.KeyRight, fyne.KeyDown:
		paso = 1
	case fyne.KeyLeft, fyne.KeyUp:
		paso = -1
	case fyne.KeyReturn, fyne.KeyEnter:
		m.g.elegir(m.i)
		return
	default:
	}
	if paso == 0 {
		return
	}
	m.g.elegir(m.i + paso)
	if c := fyne.CurrentApp().Driver().CanvasForObject(m); c != nil {
		c.Focus(m.g.muestras[m.g.elegida])
	}
}

// AccessibilityLabel es el nombre del tono, que es texto de interfaz y no el hex.
func (m *muestraCal) AccessibilityLabel() string             { return m.nombre }
func (m *muestraCal) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }
func (m *muestraCal) MinSize() fyne.Size                     { return fyne.NewSize(celdaMuestra(), celdaMuestra()) }
func (m *muestraCal) Cursor() desktop.Cursor                 { return desktop.PointerCursor }

func (m *muestraCal) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &muestraRenderer{
		m:       m,
		anillo:  canvas.NewCircle(tema.Color("border-strong", v)),
		hueco:   canvas.NewCircle(tema.Color("surface", v)),
		borde:   canvas.NewCircle(tema.Color("border", v)),
		relleno: canvas.NewCircle(color.NRGBA{}),
		aro:     ui.NuevoAro(LadoMuestra/2, 0, v),
	}
	tinta := color.NRGBA{R: 255, G: 255, B: 255, A: 255} // `text-neutral-0`
	if m.hex == "" {
		tinta = tema.Color("text", v)
		// El «sin color» lleva el borde DISCONTINUO: catorce trazos a lo largo del
		// círculo, que es lo que hace el navegador con `border-dashed` en un círculo.
		for range 14 {
			l := canvas.NewLine(tema.Color("border", v))
			l.StrokeWidth = tema.Borde
			r.trazos = append(r.trazos, l)
		}
	}
	r.tic = iconos.TrazoConTinta(iconos.Tic, 14, tinta)
	r.objetos = []fyne.CanvasObject{r.anillo, r.hueco, r.borde, r.relleno}
	for _, l := range r.trazos {
		r.objetos = append(r.objetos, l)
	}
	r.objetos = append(r.objetos, r.tic.Objeto())
	r.objetos = r.aro.Antes(r.objetos...)
	r.Refresh()
	return r
}

type muestraRenderer struct {
	m                             *muestraCal
	anillo, hueco, borde, relleno *canvas.Circle
	trazos                        []*canvas.Line
	tic                           *iconos.Dibujo
	aro                           *ui.Aro
	objetos                       []fyne.CanvasObject
}

func (r *muestraRenderer) MinSize() fyne.Size           { return r.m.MinSize() }
func (r *muestraRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *muestraRenderer) Destroy()                     {}

func (r *muestraRenderer) Layout(celda fyne.Size) {
	// El círculo es de 28 en el centro de su celda (44 con el dedo).
	o := fyne.NewPos((celda.Width-LadoMuestra)/2, (celda.Height-LadoMuestra)/2)
	t := fyne.NewSize(LadoMuestra, LadoMuestra)
	circulo := func(c *canvas.Circle, fuera float32) {
		c.Move(fyne.NewPos(o.X-fuera, o.Y-fuera))
		c.Resize(fyne.NewSize(t.Width+2*fuera, t.Height+2*fuera))
	}
	circulo(r.anillo, 4)
	circulo(r.hueco, 2)
	circulo(r.borde, 0)
	circulo(r.relleno, -tema.Borde)
	radio := float64(t.Width/2 - tema.Borde/2)
	for i, l := range r.trazos {
		// Cada trazo ocupa la mitad de su catorceavo de vuelta.
		a0 := 2 * math.Pi * float64(i) / float64(len(r.trazos))
		a1 := a0 + math.Pi/float64(len(r.trazos))
		cx, cy := float64(o.X+t.Width/2), float64(o.Y+t.Height/2)
		l.Position1 = fyne.NewPos(float32(cx+radio*math.Cos(a0)), float32(cy+radio*math.Sin(a0)))
		l.Position2 = fyne.NewPos(float32(cx+radio*math.Cos(a1)), float32(cy+radio*math.Sin(a1)))
	}
	r.tic.Colocar(o.X+(t.Width-14)/2, o.Y+(t.Height-14)/2, 14)
	r.aro.ColocarEn(o, t)
}

func (r *muestraRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	m := r.m
	elegida := m.g.elegida == m.i
	r.anillo.Hidden, r.hueco.Hidden = !elegida, !elegida
	r.anillo.FillColor, r.hueco.FillColor = tema.Color("border-strong", v), tema.Color("surface", v)
	r.borde.Hidden = m.hex == ""
	r.borde.FillColor = tema.Color("border", v)
	r.relleno.FillColor = color.NRGBA{}
	if m.hex != "" {
		r.relleno.FillColor = colorDe(m.hex)
	} else {
		// Sin borde lleno, el interior es el del panel.
		r.relleno.FillColor = tema.Color("surface-raised", v)
	}
	if elegida {
		r.tic.Objeto().Show()
	} else {
		r.tic.Objeto().Hide()
	}
	for _, o := range r.objetos {
		canvas.Refresh(o)
	}
	r.aro.Ver(m.enfocada)
}

// ---- Las píldoras de los invitados ----

// pildoraInvitado es un `<li>` de la lista de invitados:
//
//	rounded-pill border border-border bg-bg-subtle text-sm py-0.5 pr-1 pl-2.5 gap-1
//	<span class="max-w-[16rem] truncate">{email}</span>
//	<button class="h-5 w-5 rounded-full text-text-muted hover:bg-danger-bg
//	               hover:text-danger-text"><Icon close 14/></button>
func nuevaPildoraInvitado(p *Pantalla, correo string, alQuitar func()) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	quitar := ui.NuevoBotonIcono(iconos.Cerrar, p.t("event.guestRemove", i18n.Vars{"email": correo}), alQuitar)
	quitar.Lado, quitar.TamIcono, quitar.Radio = tema.Espacio(5), 14, tema.Forma("radius-control")
	caja := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioPill)
	texto := ui.Texto(correo, tema.TextoSM, ui.Normal, tema.Color("text", v))
	return container.New(&pildoraLayout{caja: caja, texto: texto, correo: correo, boton: quitar},
		append(caja.Objetos(), texto, quitar)...)
}

type pildoraLayout struct {
	caja   *ui.Caja
	texto  *canvas.Text
	correo string
	boton  *ui.BotonIcono
}

// anchoTexto es el del correo con su `max-w-[16rem]`.
func (l *pildoraLayout) anchoTexto() float32 {
	return min(ui.Medir(l.correo, tema.TextoSM, ui.Normal).Width, tema.Espacio(64))
}

func (l *pildoraLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	b := l.boton.MinSize()
	return fyne.NewSize(2*tema.Borde+espacio2p5+l.anchoTexto()+tema.Espacio(1)+b.Width+tema.Espacio(1),
		2*tema.Borde+2*espacio0p5+max(tema.TextoSM.Linea, b.Height))
}

func (l *pildoraLayout) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	l.caja.Redimensionar(t)
	x := tema.Borde + espacio2p5
	b := l.boton.MinSize()
	w := min(l.anchoTexto(), max(t.Width-x-tema.Espacio(1)-b.Width-tema.Espacio(1)-tema.Borde, 0))
	ui.Truncar(l.texto, l.correo, tema.TextoSM, ui.Normal, w)
	ui.ColocarTexto(l.texto, tema.TextoSM, x, (t.Height-tema.TextoSM.Linea)/2)
	l.boton.Move(fyne.NewPos(x+w+tema.Espacio(1), (t.Height-b.Height)/2))
	l.boton.Resize(b)
}

// uuidV4 es `crypto.randomUUID()`.
func uuidV4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

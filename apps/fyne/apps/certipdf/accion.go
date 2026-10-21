// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	temafyne "fyne.io/fyne/v2/theme"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Accion es un `<button>` que CertiPDF escribe a mano con clases de Tailwind y no con el
// `Button` de la suite: `px-2 py-1.5 text-xs font-medium rounded-md border`, con fondo,
// borde y tinta por token. La web de este producto tiene más de cuarenta así —los de
// la cinta, los de las franjas, los de los paneles—, cada uno con su combinación, y
// `ui.Boton` es otra pieza (`h-8 px-3`, semibold, cuatro variantes).
//
// Es de ESTA app a propósito: son las clases de `Paneles.tsx` y de `PdfApp.tsx`, no un
// componente de la suite, y la paridad manda sobre la reutilización.
type Accion struct {
	ui.Pulsable

	Texto   string
	Escalon tema.Escalon
	Peso    ui.Peso
	// PX y PY son el relleno (`px-*`, `py-*`).
	PX, PY float32
	Radio  float32
	// Fondo, FondoEncima, Tinta, TintaEncima y Borde son tokens; vacío es transparente
	// (o la tinta de reposo, en TintaEncima).
	Fondo, FondoEncima, Tinta, TintaEncima, Borde string
	// BordeAlfa es la opacidad del borde (`border-danger-text/30`); cero es opaco. En CSS
	// el fondo llega por debajo del borde (`background-clip: border-box`), así que se
	// mezcla con el fondo del PROPIO botón y no con lo que hay detrás.
	BordeAlfa float32
	// Subrayado es `underline`.
	Subrayado bool
	// Ancho fija el ancho (`w-full`, `flex-1`); 0 es el del rótulo.
	Ancho float32
	// Envuelto reparte el rótulo en líneas centradas a lo que deje Ancho, como un
	// `<button>` al que una fila `flex` encoge por debajo de su texto (ver [repartirFila]).
	Envuelto bool
	// Alto fija el alto (el `align-items: stretch` de una fila); 0 es el del rótulo.
	Alto float32
	// Icono va delante del rótulo, con `Hueco` entre los dos. IconoEd es lo mismo con
	// un glifo del juego del editor.
	Icono    iconos.Icono
	IconoEd  iconos.Ed
	TamIcono float32
	Hueco    float32
	// Opacidad es la de `disabled:opacity-*` cuando está Inerte. 0 vale 0,4, la de
	// este producto.
	Opacidad float32
	// Sobre es el token de lo que hay DEBAJO del botón: donde un fondo transparente o
	// translúcido se aplana. Vacío es `surface`, el de los paneles; en un diálogo es
	// `surface-raised`, que en oscuro es otro color (#1a2c35 frente a #14232a).
	Sobre string
}

// NuevaAccion crea el botón con las clases más repetidas de la app: `text-text
// hover:bg-bg-subtle rounded-control px-2 py-1.5 text-xs font-medium`.
func NuevaAccion(texto string, alPulsar func()) *Accion {
	a := &Accion{
		Texto: texto, Escalon: tema.TextoXS, Peso: ui.Medio,
		PX: tema.Espacio(2), PY: tema.Espacio(1.5), Radio: tema.Forma("radius-control"),
		Tinta: "text", FondoEncima: "bg-subtle",
	}
	a.Nombre, a.AlPulsar = texto, alPulsar
	a.ExtendBaseWidget(a)
	return a
}

func (a *Accion) conIcono() bool { return a.Icono != iconos.Ninguno || a.IconoEd != iconos.EdNinguno }

func (a *Accion) sobre() string {
	if a.Sobre == "" {
		return "surface"
	}
	return a.Sobre
}

// ConBorde le pone `border-border`.
func (a *Accion) ConBorde() *Accion { a.Borde = "border"; return a }

// bordeAccion es el grosor del borde, que en CSS va por fuera del relleno.
func bordeAccion(a *Accion) float32 {
	if a.Borde != "" {
		return tema.Borde
	}
	return 0
}

// lineasAccion es el rótulo repartido: una sola línea salvo que vaya Envuelto.
func lineasAccion(a *Accion) []string {
	if !a.Envuelto || a.Ancho <= 0 {
		return []string{a.Texto}
	}
	return ui.Repartir(a.Texto, a.Escalon, a.Peso, a.Ancho-2*a.PX-2*bordeAccion(a))
}

// MedidaAccion es la ÚNICA cuenta: rótulo (e icono) más relleno, más el borde si lo
// lleva, que en CSS va por fuera del relleno.
func MedidaAccion(a *Accion) fyne.Size {
	b := bordeAccion(a)
	alto := float32(len(lineasAccion(a)))*a.Escalon.Linea + 2*a.PY + 2*b
	if a.Alto > 0 {
		alto = a.Alto
	}
	if a.Ancho > 0 {
		return fyne.NewSize(a.Ancho, alto)
	}
	ancho := ui.Medir(a.Texto, a.Escalon, a.Peso).Width
	if a.conIcono() {
		ancho += a.TamIcono
		if a.Texto != "" {
			ancho += a.Hueco
		}
	}
	return fyne.NewSize(ancho+2*a.PX+2*b, alto)
}

// MinSize es [MedidaAccion].
func (a *Accion) MinSize() fyne.Size { a.ExtendBaseWidget(a); return MedidaAccion(a) }

// CreateRenderer arma caja, glifo, rótulo, subrayado y anillo.
func (a *Accion) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &accionRenderer{
		a:    a,
		aro:  ui.NuevoAro(a.Radio, ui.SinHueco, v).Abierto(),
		caja: ui.NuevaCaja(color.NRGBA{}, color.NRGBA{A: 1}, a.Radio),
	}
	switch {
	case a.IconoEd != iconos.EdNinguno:
		r.glifo = iconos.TrazoEd(a.IconoEd, a.TamIcono, a.Tinta, v)
	case a.Icono != iconos.Ninguno:
		r.glifo = iconos.Trazo(a.Icono, a.TamIcono, a.Tinta, v)
	}
	r.Refresh()
	return r
}

type accionRenderer struct {
	a     *Accion
	aro   *ui.Aro
	caja  *ui.Caja
	glifo *iconos.Dibujo
	// rotulos es una línea por renglón del rótulo; casi siempre, una.
	rotulos []*canvas.Text
}

func (r *accionRenderer) MinSize() fyne.Size { return MedidaAccion(r.a) }
func (r *accionRenderer) Destroy()           {}

func (r *accionRenderer) Objects() []fyne.CanvasObject {
	objs := append([]fyne.CanvasObject{}, r.caja.Objetos()...)
	if r.glifo != nil {
		objs = append(objs, r.glifo.Objeto())
	}
	for _, t := range r.rotulos {
		objs = append(objs, t)
	}
	return r.aro.Antes(objs...)
}

func (r *accionRenderer) Layout(t fyne.Size) {
	a := r.a
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	if len(r.rotulos) != 1 {
		// Envuelto: cada renglón centrado, y el bloque centrado en el alto.
		y := (t.Height - float32(len(r.rotulos))*a.Escalon.Linea) / 2
		for i, x := range r.rotulos {
			ui.ColocarTexto(x, a.Escalon, (t.Width-ui.Medir(x.Text, a.Escalon, a.Peso).Width)/2, y+float32(i)*a.Escalon.Linea)
		}
		return
	}
	m := ui.Medir(a.Texto, a.Escalon, a.Peso)
	contenido := m.Width
	if r.glifo != nil {
		contenido += a.TamIcono
		if a.Texto != "" {
			contenido += a.Hueco
		}
	}
	// Centrado, que es lo que hace un `<button>` con su texto cuando le dan más ancho.
	x := (t.Width - contenido) / 2
	if r.glifo != nil {
		r.glifo.Colocar(x, (t.Height-a.TamIcono)/2, a.TamIcono)
		x += a.TamIcono + a.Hueco
	}
	ui.ColocarTexto(r.rotulos[0], a.Escalon, x, (t.Height-a.Escalon.Linea)/2)
}

func (r *accionRenderer) Refresh() {
	a := r.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	token := func(nombre string) color.NRGBA {
		if nombre == "" {
			return color.NRGBA{}
		}
		return tema.Color(nombre, v)
	}
	fondo, tinta := token(a.Fondo), token(a.Tinta)
	if a.Encima() && !a.Inerte {
		if a.FondoEncima != "" {
			fondo = token(a.FondoEncima)
		}
		if a.TintaEncima != "" {
			tinta = token(a.TintaEncima)
		}
	}
	borde := token(a.Borde)
	if a.BordeAlfa > 0 && a.Borde != "" {
		borde.A = uint8(float32(borde.A)*a.BordeAlfa + 0.5)
		if fondo.A == 255 {
			borde = tokens.SobreSRGB(borde, fondo)
		}
	}
	if a.Inerte {
		o := a.Opacidad
		if o == 0 {
			o = 0.4
		}
		sup := tema.Color(a.sobre(), v)
		fondo, tinta, borde = opaco(fondo, o, sup), opaco(tinta, o, sup), opaco(borde, o, sup)
	}
	switch {
	case a.Borde == "":
		// Sin borde, la caja es UN rectángulo del color del fondo: el de fuera con él y el
		// de dentro vacío. Con los dos pintados, un fondo translúcido (`bg-selected`) se
		// sumaría dos veces y saldría más oscuro que en la web.
		borde, fondo = fondo, color.NRGBA{}
	case fondo.A > 0 && fondo.A < 255:
		// Con borde, un fondo translúcido (`bg-selected`) se aplana sobre la superficie:
		// la caja es el rectángulo del borde con el relleno encima, y a través de un
		// relleno translúcido se vería el borde entero.
		fondo = tokens.SobreSRGB(fondo, tema.Color(a.sobre(), v))
	case fondo.A == 0:
		// Con borde y sin fondo, la caja se pinta como un rectángulo del color del borde
		// con el relleno encima: un relleno transparente dejaría ver el borde ENTERO,
		// como un botón gris. Se pinta del color de lo que hay debajo.
		fondo = tema.Color(a.sobre(), v)
	}
	r.caja.Pintar(fondo, borde)
	lineas := lineasAccion(a)
	if len(r.rotulos) != len(lineas) {
		r.rotulos = r.rotulos[:0]
		for range lineas {
			r.rotulos = append(r.rotulos, ui.Texto("", a.Escalon, a.Peso, tinta))
		}
	}
	for i, x := range r.rotulos {
		x.Text, x.Color, x.TextStyle.Underline = lineas[i], tinta, a.Subrayado
		x.Refresh()
	}
	if r.glifo != nil {
		r.glifo.TenirCon(tinta)
	}
	r.aro.Pintar(v)
	r.aro.Ver(a.Enfocado())
	r.Layout(a.Size())
}

// deslizable es `overflow-x-auto`: la fila se desliza de lado y mide de alto lo que su
// contenido. El `container.Scroll` de Fyne declara un alto mínimo propio (medido: 32 para
// una fila de 24,5), que descolocaba la tira de pestañas 7 píxeles.
//
// Sin barra ni sombra de Fyne: la web desliza estas tiras sin enseñar barra (en el
// navegador es fina y superpuesta, y en la lámina no se ve).
func deslizable(o fyne.CanvasObject) fyne.CanvasObject {
	d, _ := deslizableCon(o)
	return d
}

// deslizableCon es [deslizable] y además su desplazador, para enmascararlo.
func deslizableCon(o fyne.CanvasObject) (fyne.CanvasObject, *container.Scroll) {
	s := container.NewHScroll(o)
	return container.New(deslizaH{}, container.NewThemeOverride(s, temaDeslizable{})), s
}

// anchoMascara es el `calc(100% - 24px)` de `MASCARA_SCROLL_X`: lo que se difumina.
const anchoMascara float32 = 24

// conMascara es `MASCARA_SCROLL_X`: mientras en la tira `s` queda contenido a la
// derecha, los últimos 24 px de `caja` —el elemento que la web enmascara, con su fondo
// y su borde— se desvanecen hasta dejar ver lo de detrás, que es `fondo`. Una máscara de
// alfa sobre un fondo liso es lo mismo que un degradado de ese fondo por encima, que es
// lo que se pinta: Fyne no enmascara.
//
// Se apaga al llegar al final para no anunciar lo que ya no hay.
func conMascara(caja fyne.CanvasObject, s *container.Scroll, fondo string) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c := tema.Color(fondo, v)
	transparente := c
	transparente.A = 0
	d := canvas.NewHorizontalGradient(transparente, c)
	m := &mascara{s: s, degradado: d}
	antes := s.OnScrolled
	s.OnScrolled = func(p fyne.Position) {
		if antes != nil {
			antes(p)
		}
		m.poner()
	}
	return container.New(m, caja, d)
}

type mascara struct {
	s         *container.Scroll
	degradado *canvas.LinearGradient
}

// queda es `Math.ceil(scrollLeft + clientWidth) < scrollWidth`.
func (m *mascara) queda() bool {
	return float32(math.Ceil(float64(m.s.Offset.X+m.s.Size().Width))) < m.s.Content.MinSize().Width
}

func (m *mascara) poner() {
	if m.queda() == m.degradado.Visible() {
		return
	}
	esconder(m.degradado, !m.queda())
}

func (m *mascara) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

func (m *mascara) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	objs[1].Move(fyne.NewPos(max(t.Width-anchoMascara, 0), 0))
	objs[1].Resize(fyne.NewSize(min(anchoMascara, t.Width), t.Height))
	esconder(objs[1], !m.queda())
}

// temaDeslizable es el de la suite sin la barra ni la sombra del desplazamiento.
type temaDeslizable struct{ tema.Sytue }

// Color devuelve transparente para la barra y la sombra.
func (t temaDeslizable) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case temafyne.ColorNameScrollBar, temafyne.ColorNameShadow, temafyne.ColorNameScrollBarBackground:
		return color.Transparent
	}
	return t.Sytue.Color(n, v)
}

type deslizaH struct{}

func (deslizaH) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, contenidoDe(objs[0]).Height)
}

func (deslizaH) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

// contenidoDe es el tamaño mínimo de lo que va dentro de un [deslizable].
func contenidoDe(o fyne.CanvasObject) fyne.Size {
	if c, ok := o.(*fyne.Container); ok && len(c.Objects) == 1 {
		o = c.Objects[0]
	}
	if t, ok := o.(*container.ThemeOverride); ok {
		o = t.Content
	}
	if s, ok := o.(*container.Scroll); ok {
		return s.Content.MinSize()
	}
	return o.MinSize()
}

// opaco es `opacity-*` de grupo aplicado a un color: mezclado con la superficie, como
// hace `ui` con sus controles inertes. Un color transparente se queda transparente.
func opaco(c color.NRGBA, o float32, sobre color.NRGBA) color.NRGBA {
	if c.A == 0 || o >= 1 {
		return c
	}
	c.A = uint8(float32(c.A) * o)
	return tokens.SobreSRGB(c, sobre)
}

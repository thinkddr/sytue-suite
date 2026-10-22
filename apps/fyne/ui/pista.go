// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Tooltip.tsx

import (
	"image/color"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Las medidas de Tooltip.tsx, en px de CSS.
const (
	// RetardoPista es el retardo del ratón (spec §Tooltip). El foco de teclado abre
	// al instante: quien navega con Tab ya ha decidido mirar ahí.
	RetardoPista = 300 * time.Millisecond
	// SeparacionPista es el `calc(100%+6px)`: el hueco entre el control y la burbuja.
	SeparacionPista float32 = 6
	// AnchoMaxPista es `max-w-[240px]`.
	AnchoMaxPista float32 = 240
)

var (
	margenXPista = tema.Espacio(2)   // px-2: 7
	margenYPista = tema.Espacio(1)   // py-1: 3,5
	ladoFlecha   = tema.Espacio(1.5) // h-1.5 w-1.5 del rombo: 5,25
	// La flecha va `-bottom-[3px]` / `-top-[3px]`: tres píxeles fuera de la burbuja.
	salidaFlecha float32 = 3
)

// MedidaPista es lo que ocupa la burbuja: `px-2 py-1 text-xs`, con el tope de 240. Un
// rótulo que no cabe pasa de línea dentro de ese tope, centrado (`text-center`).
func MedidaPista(rotulo string) fyne.Size {
	ancho := Medir(rotulo, tema.TextoXS, Medio).Width + 2*margenXPista
	lineas := len(lineasPista(rotulo))
	return fyne.NewSize(min(ancho, AnchoMaxPista), float32(max(lineas, 1))*tema.TextoXS.Linea+2*margenYPista)
}

// lineasPista es el rótulo repartido en el ancho útil de la burbuja.
func lineasPista(rotulo string) []string {
	return Repartir(rotulo, tema.TextoXS, Medio, AnchoMaxPista-2*margenXPista)
}

// Pista es Tooltip.tsx: ayuda contextual breve que envuelve a un control.
//
// # Por qué envuelve en vez de apilarse
//
// Como los avisos, y por el mismo motivo medido: una capa de Fyne se queda con TODO el
// puntero (`FindObjectAtPositionMatching` ni visita las raíces cuando hay capa), y una
// pista que bloquea la aplicación es peor que no tener pista. Así que la burbuja es un
// HERMANO del control dentro de este envoltorio, exactamente como el `<span
// class="relative inline-flex">` de la web, y se pinta FUERA de la caja del envoltorio
// —Fyne no recorta salvo que un renderer lo pida—.
//
// El límite es el mismo que en la web: dentro de algo que recorta (`overflow-hidden`,
// un desplazamiento, el panel de un diálogo) la burbuja se corta. La web lo resuelve
// con un portal cuando le hace falta, y aquí también ([Pista.Portal]): no es una capa
// sino un contenedor sin layout que la pantalla pone ENCIMA de todo lo suyo, y como
// solo lleva primitivas —caja, texto, flecha, sombra— no se queda con el puntero.
//
// No debe llevar información esencial única: es ayuda, no contenido.
type Pista struct {
	widget.BaseWidget
	// Rotulo es el texto de la burbuja, YA traducido.
	Rotulo string
	// Hijo es el control al que acompaña.
	Hijo fyne.CanvasObject
	// Portal, si lo hay, es donde se pinta la burbuja en vez de junto al control: un
	// contenedor SIN layout que cubre la pantalla por encima de lo que recorta (la cinta
	// de CertiPDF va en un desplazador). Se coloca con la posición absoluta de los dos.
	// ponytail: con portal la burbuja entra sin el fundido de 150 ms.
	Portal *fyne.Container
	// Abajo pone la burbuja DEBAJO del control. El valor cero es arriba, como
	// `placement = 'top'` en la web.
	Abajo bool
	// Derecha la pone a la DERECHA, centrada en alto y con la flecha a la izquierda. No
	// es una colocación de Tooltip.tsx: es donde se ve el `title` de una fila del carril
	// PLEGADO (64 px), que centrada encima se saldría de la ventana por la izquierda.
	Derecha bool

	abierta bool
	reloj   *time.Timer
	r       *pistaRenderer
}

var (
	_ fyne.Widget       = (*Pista)(nil)
	_ desktop.Hoverable = (*Pista)(nil)
)

// NuevaPista envuelve el control con su ayuda. Si el control es de este paquete, la
// pista se abre también con el TECLADO: al enfocarlo, al instante, y se cierra al perder
// el foco y con Escape (el `onFocusCapture`/`onBlurCapture` de Tooltip.tsx, que Fyne no
// tiene porque no hay fase de captura: se lo avisa el propio control).
func NuevaPista(rotulo string, hijo fyne.CanvasObject) *Pista {
	p := &Pista{Rotulo: rotulo, Hijo: hijo}
	if c, es := hijo.(conPista); es {
		c.ponerPista(p)
	}
	p.ExtendBaseWidget(p)
	return p
}

// conPista es el control que avisa a su pista del foco y de Escape.
type conPista interface{ ponerPista(*Pista) }

// alFoco es lo que el control le avisa: abrir al ganar el foco, cerrar al perderlo.
func (p *Pista) alFoco(enfocado bool) {
	if p == nil {
		return
	}
	if enfocado {
		p.Abrir()
		return
	}
	p.Cerrar()
}

// alEscape la cierra si está abierta. No se queda la tecla: en la web el tooltip la oye
// en `document` y el diálogo de debajo, también.
func (p *Pista) alEscape() {
	if p != nil {
		p.Cerrar()
	}
}

// MouseIn arranca el retardo de 300 ms.
func (p *Pista) MouseIn(*desktop.MouseEvent) { p.alPuntero(true) }

// alPuntero es el `onMouseEnter`/`onMouseLeave` del envoltorio, avisado por el CONTROL.
//
// Hace falta porque el driver le da el `MouseIn` al objeto «pasable» MÁS PROFUNDO bajo
// el puntero (`FindObjectAtPositionMatching` se queda con la última coincidencia), y un
// botón lo es: con la pista envolviéndolo, el `MouseIn` era del botón y la burbuja no
// se abría nunca con el ratón. Solo con el teclado, y con el driver de pruebas llamando
// a mano a `Pista.MouseIn`, que es como lo miraban los guardianes.
func (p *Pista) alPuntero(dentro bool) {
	if p == nil {
		return
	}
	if !dentro {
		p.Cerrar()
		return
	}
	if p.reloj != nil || p.abierta {
		return
	}
	p.reloj = time.AfterFunc(RetardoPista, func() { fyne.Do(func() { p.Abrir() }) })
}

// MouseMoved no hace nada: el retardo ya está en marcha.
func (p *Pista) MouseMoved(*desktop.MouseEvent) {}

// MouseOut cancela el retardo y cierra.
func (p *Pista) MouseOut() { p.Cerrar() }

// Abrir enseña la burbuja al instante. Es lo que hace el foco de teclado.
func (p *Pista) Abrir() {
	if p.abierta {
		return
	}
	p.abierta = true
	p.Refresh()
	if p.r != nil {
		p.r.fundirEntrada()
	}
}

// Cerrar la esconde. Lo llaman el ratón al salir y Escape: una ayuda que tapa lo que
// hay debajo y no se quita con el teclado es una trampa (WAI-ARIA APG).
func (p *Pista) Cerrar() {
	if p.reloj != nil {
		p.reloj.Stop()
		p.reloj = nil
	}
	if !p.abierta {
		return
	}
	p.abierta = false
	p.Refresh()
}

// Abierta dice si la burbuja está a la vista.
func (p *Pista) Abierta() bool { return p.abierta }

// CreateRenderer construye el envoltorio: el control, y detrás la burbuja y su flecha.
func (p *Pista) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// Superficie INVERTIDA por tokens de rol, como `sytue-tooltip-surface`: así
	// contrasta igual en claro y en oscuro sin heredar el color de la tarjeta.
	tinta := tema.Color("text", v)
	r := &pistaRenderer{
		p:       p,
		sombra:  NuevaSombra(tema.SombraMD, tema.RadioMD), // `shadow-[var(--shadow-md)]`
		burbuja: NuevaCaja(tinta, color.NRGBA{}, tema.RadioMD),
		tinta:   tinta,
	}
	r.flecha = rombo(ladoFlecha, &r.tinta)
	p.r = r
	r.rehacer()
	return r
}

type pistaRenderer struct {
	p       *Pista
	sombra  *Sombra
	burbuja *Caja
	lineas  []*canvas.Text
	flecha  *canvas.Raster
	tinta   color.NRGBA // la de la burbuja y la flecha, que la lee al pintar
	objetos []fyne.CanvasObject
	// fundido es la imagen de la burbuja mientras entra con `sytue-fade-in`.
	fundido *canvas.Image
	// enPortal es lo que esta pista tiene puesto en su [Pista.Portal].
	enPortal []fyne.CanvasObject
}

// burbujaYFlecha son las primitivas de la burbuja: la caja, las líneas y la flecha.
func (r *pistaRenderer) burbujaYFlecha() []fyne.CanvasObject {
	out := append([]fyne.CanvasObject{}, r.burbuja.Objetos()...)
	for _, l := range r.lineas {
		out = append(out, l)
	}
	return append(out, r.flecha)
}

// fundirEntrada es `sytue-fade-in` (150 ms) con la opacidad de GRUPO de la burbuja: se
// pinta una vez a una imagen y se funde esa imagen, como las capas (fundido.go). La burbuja
// son varias primitivas y no un objeto, así que se meten un momento en un contenedor en
// el origen para pintarlas y se devuelven a su sitio.
func (r *pistaRenderer) fundirEntrada() {
	c := lienzoDe(r.p)
	if c == nil || SinMovimiento || r.fundido != nil || r.p.Portal != nil {
		return
	}
	objs := r.burbujaYFlecha()
	var ini, fin fyne.Position
	for i, o := range objs {
		p, s := o.Position(), o.Size()
		if i == 0 {
			ini, fin = p, p.Add(fyne.NewPos(s.Width, s.Height))
			continue
		}
		ini = fyne.NewPos(min(ini.X, p.X), min(ini.Y, p.Y))
		fin = fyne.NewPos(max(fin.X, p.X+s.Width), max(fin.Y, p.Y+s.Height))
	}
	tam := fyne.NewSize(fin.X-ini.X, fin.Y-ini.Y)
	if tam.IsZero() {
		return
	}
	for _, o := range objs {
		o.Move(o.Position().Subtract(ini))
	}
	grupo := container.NewWithoutLayout(objs...)
	grupo.Resize(tam)
	img := canvas.NewImageFromImage(instantanea(grupo, c.Scale()))
	for _, o := range objs {
		o.Move(o.Position().Add(ini))
		o.Hide()
	}
	img.FillMode = canvas.ImageFillStretch
	img.Move(ini)
	img.Resize(tam)
	img.Translucency = 1
	r.fundido = img
	r.objetos = append(r.objetos, img)
	canvas.Refresh(r.p)
	a := fyne.NewAnimation(DuracionFundido, func(t float32) {
		img.Translucency = float64(1 - t)
		r.sombra.img.Translucency = float64(1 - t)
		canvas.Refresh(img)
		canvas.Refresh(r.sombra.img)
		if t >= 1 {
			r.fundido = nil
			r.rehacer()
			canvas.Refresh(r.p)
		}
	})
	a.Curve = CurvaEstandar
	a.Start()
}

// rehacer arma los objetos: el control, la sombra, la burbuja, una línea de texto por
// cada una del reparto y la flecha. Escondida si no está abierta.
func (r *pistaRenderer) rehacer() {
	papel := tema.Color("bg", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.lineas = r.lineas[:0]
	for _, l := range lineasPista(r.p.Rotulo) {
		r.lineas = append(r.lineas, Texto(l, tema.TextoXS, Medio, papel))
	}
	r.objetos = append([]fyne.CanvasObject{r.p.Hijo, r.sombra.Objeto()}, r.burbuja.Objetos()...)
	for _, l := range r.lineas {
		r.objetos = append(r.objetos, l)
	}
	r.objetos = append(r.objetos, r.flecha)
	for _, o := range r.objetos[1:] {
		if r.p.abierta {
			o.Show()
		} else {
			o.Hide()
		}
	}
	if portal := r.p.Portal; portal != nil {
		r.alPortal(portal, r.objetos[1:])
		r.objetos = r.objetos[:1]
	}
	if r.fundido != nil { // entrando: la burbuja la pinta su imagen
		if !r.p.abierta {
			r.fundido = nil
			return
		}
		for _, o := range r.burbujaYFlecha() {
			o.Hide()
		}
		r.objetos = append(r.objetos, r.fundido)
	}
}

// alPortal cambia en el portal lo que esta pista tenía puesto por `flotantes`: solo lo
// suyo, porque el portal lo comparten todas las pistas de la pantalla.
func (r *pistaRenderer) alPortal(portal *fyne.Container, flotantes []fyne.CanvasObject) {
	mias := map[fyne.CanvasObject]bool{}
	for _, o := range r.enPortal {
		mias[o] = true
	}
	quedan := portal.Objects[:0:0]
	for _, o := range portal.Objects {
		if !mias[o] {
			quedan = append(quedan, o)
		}
	}
	r.enPortal = nil
	if r.p.abierta {
		r.enPortal = append(r.enPortal, flotantes...)
	}
	quedan = append(quedan, r.enPortal...)
	portal.Objects = quedan
	portal.Refresh()
}

// rombo dibuja el `rotate-45` de la flecha con un ráster.
//
// Se hace con un ráster y no con un rectángulo girado porque Fyne no gira primitivas,
// y no con líneas porque el rombo va RELLENO. La cuenta es la de un cuadrado girado
// 45°: `|dx| + |dy| ≤ lado/√2`, que a este tamaño —5,25 px— cabe en tres líneas y no
// necesita ni antialias.
func rombo(lado float32, tinta *color.NRGBA) *canvas.Raster {
	d := lado / float32(math.Sqrt2)
	return canvas.NewRasterWithPixels(func(x, y, w, h int) color.Color {
		c := *tinta
		cx, cy := float32(w)/2, float32(h)/2
		// El ráster trabaja en píxeles de DISPOSITIVO, así que el radio se escala con
		// el ancho pedido: la diagonal del rombo ocupa la caja entera.
		k := float32(w) / (2 * d)
		if abs(float32(x)+0.5-cx)+abs(float32(y)+0.5-cy) <= d*k {
			return c
		}
		return color.NRGBA{}
	})
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// MinSize es la del CONTROL y solo la del control: la burbuja se pinta fuera, como el
// `absolute` de la web, así que no empuja a nadie.
func (r *pistaRenderer) MinSize() fyne.Size { return r.p.Hijo.MinSize() }

func (r *pistaRenderer) Objects() []fyne.CanvasObject { return r.objetos }

func (r *pistaRenderer) Destroy() {}

// Layout coloca la burbuja centrada sobre el control (o bajo él) a 6 px, y la flecha
// asomando 3 px por el lado que toca.
func (r *pistaRenderer) Layout(t fyne.Size) {
	r.p.Hijo.Resize(t)
	r.p.Hijo.Move(fyne.NewPos(0, 0))

	m := MedidaPista(r.p.Rotulo)
	x := (t.Width - m.Width) / 2 // `left-1/2 -translate-x-1/2`
	y := -m.Height - SeparacionPista
	switch {
	case r.p.Derecha:
		x, y = t.Width+SeparacionPista, (t.Height-m.Height)/2
	case r.p.Abajo:
		y = t.Height + SeparacionPista
	}
	if portal := r.p.Portal; portal != nil {
		// En el portal, lo mismo contado desde su esquina.
		d := fyne.CurrentApp().Driver()
		o := d.AbsolutePositionForObject(r.p).Subtract(d.AbsolutePositionForObject(portal))
		x, y = x+o.X, y+o.Y
	}
	r.burbuja.Redimensionar(m)
	for _, o := range r.burbuja.Objetos() {
		o.Move(o.Position().Add(fyne.NewPos(x, y)))
	}
	if r.p.abierta { // `Ajustar` la vuelve a enseñar: cerrada, no se toca
		r.sombra.Ajustar(fyne.NewPos(x, y), m, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	}
	for i, l := range r.lineas {
		// `text-center`: cada línea centrada en la burbuja.
		ancho := Medir(l.Text, tema.TextoXS, Medio).Width
		ColocarTexto(l, tema.TextoXS, x+(m.Width-ancho)/2, y+margenYPista+float32(i)*tema.TextoXS.Linea)
	}

	// La caja del rombo es su DIAGONAL, no su lado: un cuadrado de 5,25 girado 45°
	// ocupa 7,42.
	diag := ladoFlecha * float32(math.Sqrt2)
	fx, fy := x+(m.Width-diag)/2, y+m.Height+salidaFlecha-diag/2-ladoFlecha/2
	switch {
	case r.p.Derecha:
		fx, fy = x-salidaFlecha-diag/2+ladoFlecha/2, y+(m.Height-diag)/2
	case r.p.Abajo:
		fy = y - salidaFlecha - diag/2 + ladoFlecha/2
	}
	r.flecha.Resize(fyne.NewSize(diag, diag))
	r.flecha.Move(fyne.NewPos(fx, fy))
	if r.p.Portal != nil && r.p.abierta {
		r.p.Portal.Refresh()
	}
}

// Refresh enseña o esconde la burbuja. `sytue-fade-in` sobre una capa de primitivas
// pediría animar el alfa de tres objetos; a 150 ms y sobre un rótulo de ayuda, no lo
// vale.
func (r *pistaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.tinta = tema.Color("text", v)
	r.burbuja.Pintar(r.tinta, color.NRGBA{})
	r.rehacer()
	r.Layout(r.p.Size())
	r.flecha.Refresh()
	canvas.Refresh(r.p)
}

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
	// MargenPista es el aire mínimo con el borde de la ventana: el `MARGEN` de
	// Tooltip.tsx, que es el mismo de `useFitInViewport`.
	MargenPista float32 = 8
	// GrupoPista es el retardo de GRUPO: si otra pista se acaba de cerrar, la siguiente
	// abre sin esperar. Recorrer una barra de iconos con el ratón no debe costar 300 ms
	// por icono.
	GrupoPista = 500 * time.Millisecond
)

// ultimoCierrePista es cuándo se cerró la última pista abierta, de cualquier pantalla:
// es lo que hace «de grupo» al retardo. Solo se toca en la rutina de la interfaz.
var ultimoCierrePista time.Time

var (
	margenXPista = tema.Espacio(2)   // px-2: 8
	margenYPista = tema.Espacio(1)   // py-1: 4
	ladoFlecha   = tema.Espacio(1.5) // h-1.5 w-1.5 del rombo: 6
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

	// ancla, si la hay, es la caja del control cuando la pista NO lo envuelve: la que un
	// BotonIcono lleva de serie va en una caja de lado cero dentro del propio botón
	// (para que el puntero no la encuentre nunca y el `:hover` siga siendo del botón), y
	// la burbuja se coloca con la medida del botón y no con la suya.
	ancla func() fyne.Size

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

// portalesPista es, para cada ventana, el portal de su pantalla: el que la barra de la
// suite pone por encima de todo lo suyo (ver [BarraSuite]). Es el `createPortal(…,
// document.body)` de Tooltip.tsx para las pistas que un control trae de serie.
var portalesPista porLienzo[fyne.Container]

// portalDePistas es el portal de la pantalla en la que está `o`, o nil si no hay o si
// hay una capa encima: el portal va DEBAJO de las capas, así que la pista de un botón
// de un diálogo se quedaría tapada por su propio diálogo. Con una capa arriba, además,
// lo de debajo no recibe ni puntero ni foco, así que la pista que se abra es de la capa.
func portalDePistas(o fyne.CanvasObject) *fyne.Container {
	c := lienzoDe(o)
	if c == nil || c.Overlays().Top() != nil {
		return nil
	}
	return portalesPista.de(c)
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
	if time.Since(ultimoCierrePista) < GrupoPista {
		p.Abrir()
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
	ultimoCierrePista = time.Now()
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
	if c == nil || MenosMovimiento() || r.fundido != nil || r.p.Portal != nil {
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
			// `rehacer` arma líneas de texto NUEVAS, que nacen en el origen: sin volver
			// a colocarlas, el rótulo saltaba a la esquina del control al acabar de
			// entrar la burbuja.
			r.rehacer()
			r.Layout(r.p.Size())
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

// ladoPista es por dónde sale la burbuja, ya volteada si no cabía.
type ladoPista int

const (
	pistaArriba ladoPista = iota
	pistaAbajo
	pistaDerecha
	pistaIzquierda
)

// preferido es el lado que pide la pista; el que acaba teniendo lo decide
// [colocarPista].
func (p *Pista) preferido() ladoPista {
	switch {
	case p.Derecha:
		return pistaDerecha
	case p.Abajo:
		return pistaAbajo
	}
	return pistaArriba
}

// colocarPista es `colocarTooltip` de Tooltip.tsx: la burbuja de medida `m` junto a un
// control de medida `ancla`, VOLTEADA al lado opuesto si por el preferido no cabe y
// acotada a la ventana con `MargenPista` de aire. Todo en coordenadas del control, que
// está en `abs` de una ventana de medida `ventana` (cero: sin ventana, sin límites).
//
// Devuelve la esquina de la burbuja, el lado en que queda y dónde va la flecha a lo
// largo de su borde, que sigue apuntando al centro del control aunque la burbuja se
// haya corrido para caber.
func colocarPista(ancla, m fyne.Size, abs fyne.Position, ventana fyne.Size, preferido ladoPista) (fyne.Position, ladoPista, float32) {
	const sinLimite float32 = 1e9
	lo, hi := fyne.NewPos(-sinLimite, -sinLimite), fyne.NewPos(sinLimite, sinLimite)
	if !ventana.IsZero() {
		lo = fyne.NewPos(MargenPista-abs.X, MargenPista-abs.Y)
		hi = fyne.NewPos(ventana.Width-MargenPista-abs.X, ventana.Height-MargenPista-abs.Y)
	}
	acotar := func(v, a, b float32) float32 { return max(a, min(v, b)) }
	if preferido == pistaDerecha || preferido == pistaIzquierda {
		cabeDerecha := ancla.Width+SeparacionPista+m.Width <= hi.X
		cabeIzquierda := -SeparacionPista-m.Width >= lo.X
		lado, x := pistaDerecha, ancla.Width+SeparacionPista
		if !cabeDerecha && cabeIzquierda {
			lado, x = pistaIzquierda, -SeparacionPista-m.Width
		}
		x = acotar(x, lo.X, hi.X-m.Width)
		centro := ancla.Height / 2
		y := acotar(centro-m.Height/2, lo.Y, hi.Y-m.Height)
		return fyne.NewPos(x, y), lado, acotar(centro-y, 6, m.Height-6)
	}
	cabeArriba := -SeparacionPista-m.Height >= lo.Y
	cabeAbajo := ancla.Height+SeparacionPista+m.Height <= hi.Y
	lado := preferido
	if preferido == pistaArriba && !cabeArriba && cabeAbajo {
		lado = pistaAbajo
	}
	if preferido == pistaAbajo && !cabeAbajo && cabeArriba {
		lado = pistaArriba
	}
	y := -m.Height - SeparacionPista // `bottom-[calc(100%+6px)]`
	if lado == pistaAbajo {
		y = ancla.Height + SeparacionPista
	}
	centro := ancla.Width / 2
	x := acotar(centro-m.Width/2, lo.X, hi.X-m.Width) // `left-1/2 -translate-x-1/2`
	return fyne.NewPos(x, y), lado, acotar(centro-x, 6, m.Width-6)
}

// Layout coloca la burbuja centrada sobre el control (o bajo él, o a su lado) a 6 px,
// dentro de la ventana, y la flecha asomando 3 px por el lado que toca.
func (r *pistaRenderer) Layout(t fyne.Size) {
	r.p.Hijo.Resize(t)
	r.p.Hijo.Move(fyne.NewPos(0, 0))
	// Cerrada, la burbuja está escondida y no hay nada que colocar: Abrir la vuelve a
	// colocar. Y no se le pregunta al driver la posición absoluta: lo hace recorriendo
	// el árbol y pidiendo renderers, y si el antepasado que está creando el suyo aún no
	// lo tiene en la caché, lo vuelve a crear y vuelve aquí sin fin (Rooms: la fila de
	// mensaje con reacciones, la cabecera del panel del hilo).
	if !r.p.abierta {
		return
	}
	ancla := t
	if r.p.ancla != nil {
		ancla = r.p.ancla()
	}

	m := MedidaPista(r.p.Rotulo)
	var abs fyne.Position
	var ventana fyne.Size
	d := fyne.CurrentApp().Driver()
	if c := lienzoDe(r.p); c != nil {
		// El driver da (0, 0) cuando no encuentra el objeto en el lienzo (y el de
		// pruebas, que hace trampa, devuelve siempre la última ventana): entonces no
		// hay ventana que valga y la burbuja va sin acotar, como en la v1. El precio es
		// un control pegado EXACTAMENTE a la esquina, que se queda sin volteo.
		if abs = d.AbsolutePositionForObject(r.p); !abs.IsZero() {
			ventana = c.Size()
		}
	}
	pos, lado, flecha := colocarPista(ancla, m, abs, ventana, r.p.preferido())
	x, y := pos.X, pos.Y
	if portal := r.p.Portal; portal != nil {
		// En el portal, lo mismo contado desde su esquina.
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
	// ocupa 7,42. Su centro va a `flecha` a lo largo del borde que mira al control.
	diag := ladoFlecha * float32(math.Sqrt2)
	var fx, fy float32
	switch lado {
	case pistaArriba:
		fx, fy = x+flecha-diag/2, y+m.Height+salidaFlecha-diag/2-ladoFlecha/2
	case pistaAbajo:
		fx, fy = x+flecha-diag/2, y-salidaFlecha-diag/2+ladoFlecha/2
	case pistaDerecha:
		fx, fy = x-salidaFlecha-diag/2+ladoFlecha/2, y+flecha-diag/2
	case pistaIzquierda:
		fx, fy = x+m.Width+salidaFlecha-diag/2-ladoFlecha/2, y+flecha-diag/2
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

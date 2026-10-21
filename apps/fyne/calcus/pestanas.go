// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image/color"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La barra de hojas de SheetCanvas.tsx:
//
//	border-border bg-surface flex h-8 shrink-0 items-center gap-1 border-t px-2 text-xs
//
// y cada pestaña `relative h-full max-w-[12rem] shrink-0 truncate px-2`, con el
// subrayado `bg-indicator absolute inset-x-1 bottom-0 h-0.5 rounded-full` en la activa.
// Todo rem: con el de 14, `h-8` son 28, `px-2` 7, `gap-1` 3,5 y `max-w-[12rem]` 168.
var (
	AltoPestanas      = tema.Espacio(8)
	MargenPestanas    = tema.Espacio(2)
	HuecoPestanas     = tema.Espacio(1)
	MargenPestana     = tema.Espacio(2)
	AnchoMaxPestana   = float32(12 * tema.Rem)
	SangriaIndicador  = tema.Espacio(1)
	GrosorIndicador   = tema.Espacio(0.5)
	EscalonPestanas   = tema.TextoXS
	altoContenidoTira = AltoPestanas - tema.Borde // `h-full` dentro del `border-t`
)

// MedidaPestanaHoja es lo que ocupa una pestaña: su nombre en `text-xs` (seminegrita
// media si es la activa) con `px-2`, hasta el tope de `max-w-[12rem]`.
func MedidaPestanaHoja(nombre string, activa bool) fyne.Size {
	ancho := ui.Medir(nombre, EscalonPestanas, pesoPestana(activa)).Width + 2*MargenPestana
	return fyne.NewSize(min(ancho, AnchoMaxPestana), altoContenidoTira)
}

func pesoPestana(activa bool) ui.Peso {
	if activa {
		return ui.Medio // `font-medium`
	}
	return ui.Normal
}

// Pestanas es la barra de hojas: las pestañas, los tres botones (añadir, duplicar y
// eliminar hoja) y el resumen del rango seleccionado.
type Pestanas struct {
	widget.BaseWidget

	lz                  *Lienzo
	tira                *tiraHojas
	mas, copiar, borrar *editor.BotonCinta
}

var _ fyne.Widget = (*Pestanas)(nil)

func nuevasPestanas(lz *Lienzo) *Pestanas {
	p := &Pestanas{lz: lz}
	p.ExtendBaseWidget(p)
	p.tira = &tiraHojas{p: p, arrastre: -1}
	p.tira.ExtendBaseWidget(p.tira)
	// Los rótulos se ponen al pintar: el traductor llega DESPUÉS de construir el lienzo.
	p.mas = editor.NuevoBotonCinta(iconos.EdMas, "", lz.AnadirHoja)
	p.copiar = editor.NuevoBotonCinta(iconos.EdCopiar, "", lz.DuplicarHoja)
	p.borrar = editor.NuevoBotonCinta(iconos.EdPapelera, "", lz.PedirBorrarHoja)
	return p
}

// AccessibilityRole la anuncia como contenedor.
func (p *Pestanas) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// MinSize es el `h-8`; el ancho lo pone quien la coloca.
func (p *Pestanas) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	return fyne.NewSize(2*MargenPestanas+3*editor.LadoBotonCinta, AltoPestanas)
}

// Resumen es el texto del `<output>`: «Recuento: 3 · Suma: 6 · Promedio: 2». Vacío
// sin rango o sin ningún número, que es cuando la web no lo pinta.
func (p *Pestanas) Resumen() string {
	r := p.lz.Rejilla
	if r.ext == nil {
		return ""
	}
	n, suma, media, ok := p.lz.Libro.Resumen(r.Rango())
	if !ok {
		return ""
	}
	return p.lz.t("sheet.stats.count", i18n.Vars{"count": n}) + " · " +
		p.lz.t("sheet.stats.sum", i18n.Vars{"value": formula.FormatNumber(suma)}) + " · " +
		p.lz.t("sheet.stats.average", i18n.Vars{"value": formula.FormatNumber(media)})
}

// CreateRenderer compone el fondo, la línea de arriba, la tira, los botones y el resumen.
func (p *Pestanas) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pestanasRenderer{
		p:       p,
		fondo:   canvas.NewRectangle(tema.Color("surface", v)),
		linea:   canvas.NewRectangle(tema.Color("border", v)),
		resumen: ui.Texto("", EscalonPestanas, ui.Normal, tema.Color("text-muted", v)),
	}
	r.objetos = []fyne.CanvasObject{r.fondo, r.linea, p.tira, p.mas, p.copiar, p.borrar, r.resumen}
	r.Refresh()
	return r
}

type pestanasRenderer struct {
	p            *Pestanas
	fondo, linea *canvas.Rectangle
	resumen      *canvas.Text
	objetos      []fyne.CanvasObject
}

func (r *pestanasRenderer) MinSize() fyne.Size           { return r.p.MinSize() }
func (r *pestanasRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pestanasRenderer) Destroy()                     {}

func (r *pestanasRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	// `border-t`, dentro del `h-8` por `box-sizing: border-box`.
	r.linea.Move(fyne.NewPos(0, 0))
	r.linea.Resize(fyne.NewSize(t.Width, tema.Borde))

	alto := t.Height - tema.Borde
	der := t.Width - MargenPestanas
	if r.resumen.Text != "" {
		ancho := ui.Medir(r.resumen.Text, EscalonPestanas, ui.Normal).Width
		der -= ancho
		ui.ColocarTexto(r.resumen, EscalonPestanas, der, tema.Borde+(alto-EscalonPestanas.Linea)/2)
		der -= HuecoPestanas
	}
	// `items-center` centra los botones de la cinta en el alto útil de la barra.
	lado := editor.LadoBotonCinta
	for _, b := range []*editor.BotonCinta{r.p.borrar, r.p.copiar, r.p.mas} {
		der -= lado
		b.Move(fyne.NewPos(der, tema.Borde+(alto-lado)/2))
		b.Resize(fyne.NewSize(lado, lado))
		der -= HuecoPestanas
	}
	r.p.tira.Move(fyne.NewPos(MargenPestanas, tema.Borde))
	r.p.tira.Resize(fyne.NewSize(max(der-MargenPestanas, 0), alto))
}

func (r *pestanasRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	lz := r.p.lz
	r.fondo.FillColor = tema.Color("surface", v)
	r.linea.FillColor = tema.Color("border", v)
	r.resumen.Color = tema.Color("text-muted", v)
	r.resumen.Text = r.p.Resumen()
	r.p.mas.Rotulo = lz.t("sheet.add", nil)
	r.p.copiar.Rotulo = lz.t("sheet.duplicate", nil)
	r.p.borrar.Rotulo = lz.t("sheet.delete", nil)
	// Con el libro protegido los tres botones se apagan, como en la web (`disabled`).
	estructura := !lz.Rejilla.SoloLectura && lz.Libro.ProteccionLibro() == nil
	r.p.mas.Inerte = !estructura
	r.p.copiar.Inerte = !estructura
	r.p.borrar.Inerte = !estructura || len(lz.Libro.Hojas()) <= 1
	r.p.tira.sincronizar()
	// Con el tamaño del WIDGET y no con el del último Layout: si el renderer nace
	// después del Resize —que es lo normal—, Fyne no vuelve a colocarlo y todo se
	// quedaría en 0×0. Se vio en el guardián de píxeles: la barra salía en blanco.
	r.Layout(r.p.Size())
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// ---- la tira ----

// tiraHojas es el `overflow-x-auto` de las pestañas: se desplaza con la rueda y RECORTA
// lo que no cabe, que es lo que da implementar `fyne.Scrollable` (ver el comentario de
// la Rejilla). Con seis hojas o en un teléfono, las últimas pestañas no se saldrían de
// la barra: quedarían detrás de los botones sin forma de llegar a ellas.
//
// Con el puntero encima y pestañas que no caben, la barra de desplazamiento: en la web
// es la del sistema con `scrollbar-width: thin` y `scrollbar-color: border-strong
// transparent`, que en macOS se superpone y sale al pasar por encima. El pulgar es el
// medido en Chromium (2026-09-24, ocho pestañas en 200 px): 6 px de alto a 2 del borde
// de abajo, redondeado y en `border-strong`, de largo lo que se ve de la tira.
//
// Donde la barra del sistema NO se superpone —Linux y Windows, que Chromium pinta con
// barras clásicas— la tira que no cabe le reserva 10 px debajo: las pestañas se quedan
// en lo de arriba (su `h-full` es el alto que queda) y la barra lleva el pulgar siempre
// visible y una flecha en cada punta, que corre la tira 40 px. Medido en el Chromium de
// Linux del frontal (2026-09-26, ocho pestañas en 284 px, a ×1 y ×2): botones de 12 px,
// triángulos de 3,5 × 6 a 2,5 px de la punta y el pulgar de 6 px a 2 del borde.
type tiraHojas struct {
	widget.BaseWidget

	p        *Pestanas
	desX     float32
	encima   bool // el puntero, sobre la tira (sobre una pestaña lo dice la pestaña)
	pestanas []*pestanaHoja
	arrastre int // la pestaña que se está arrastrando, o -1
	soltarX  float32
	// sobrePulgar es el puntero sobre el pulgar; arrastrandoPulgar, que se le está
	// arrastrando, y arrastreSuelto, un arrastre que no empezó en él.
	sobrePulgar, arrastrandoPulgar, arrastreSuelto bool
}

var (
	_ fyne.Widget       = (*tiraHojas)(nil)
	_ fyne.Scrollable   = (*tiraHojas)(nil)
	_ fyne.Tappable     = (*tiraHojas)(nil)
	_ fyne.Draggable    = (*tiraHojas)(nil)
	_ desktop.Hoverable = (*tiraHojas)(nil)
)

// MouseIn, MouseMoved y MouseOut siguen el puntero para enseñar la barra.
func (t *tiraHojas) MouseIn(*desktop.MouseEvent) { t.encima = true; t.Refresh() }

// MouseMoved sigue si el puntero está sobre el pulgar, que cambia de color.
func (t *tiraHojas) MouseMoved(e *desktop.MouseEvent) {
	if encima := t.enPulgar(e.Position); encima != t.sobrePulgar {
		t.sobrePulgar = encima
		t.Refresh()
	}
}

// MouseOut esconde la barra.
func (t *tiraHojas) MouseOut() { t.encima, t.sobrePulgar = false, false; t.Refresh() }

// conPuntero dice si el puntero está sobre la tira o sobre alguna de sus pestañas.
func (t *tiraHojas) conPuntero() bool {
	if t.encima {
		return true
	}
	for _, ph := range t.pestanas {
		if ph.encima {
			return true
		}
	}
	return false
}

// Medidas del pulgar de la barra (`scrollbar-width: thin`, medido en Chromium), y de la
// barra clásica: su alto, sus dos botones, lo que corre cada flecha
// (`kPixelsPerLineStep`) y dónde va el triángulo dentro del botón.
const (
	AltoPulgarTira   float32 = 6
	MargenPulgarTira float32 = 2

	AltoBarraClasica  float32 = 10
	LadoBotonClasico  float32 = 12
	PasoFlechaClasica float32 = 40
	ladoFlecha        float32 = 6 // la caja del triángulo, de 3,5 × 6 a 2,5 de la punta
)

// barraClasica es que el sistema pinta las barras de desplazamiento clásicas, que no se
// superponen (lo que hace Chromium en Linux y en Windows). Es una variable para probar
// las dos en cualquier sistema.
var barraClasica = runtime.GOOS == "linux" || runtime.GOOS == "windows"

// clasica dice si la tira lleva ahora la barra clásica: la del sistema y que no quepa.
func (t *tiraHojas) clasica() bool {
	return barraClasica && t.Size().Width > 0 && t.anchoContenido() > t.Size().Width
}

// FraccionPagina es lo que corre un clic en la pista de la barra clásica, sobre lo que
// se ve: `kFractionToStepWhenPaging` de Chromium.
const FraccionPagina float32 = 0.875

// PulgarEncima conserva el color de reposo, como la barra clásica de Chromium en
// Linux: la captura con los tokens actuales del 2026-10-04 no cambia al pasar el
// puntero. `scrollbar-color` solo recibe `border-strong` y una pista transparente.
func PulgarEncima(v tema.Variante) color.NRGBA {
	return tema.Color("border-strong", v)
}

// PulgarArrastrando es el color al pulsar el pulgar clásico, medido en Chromium
// de Linux el 2026-10-04 con el `border-strong` de la paleta petróleo (#515b5d y
// #adb3b4). Chromium oscurece el claro y aclara el oscuro; el guardián comprueba
// también los tokens de reposo para detectar cambios de paleta.
func PulgarArrastrando(v tema.Variante) color.NRGBA {
	if v == tema.Oscuro {
		return color.NRGBA{R: 173, G: 179, B: 180, A: 255}
	}
	return color.NRGBA{R: 81, G: 91, B: 93, A: 255}
}

// pulgar es dónde va el pulgar —x, y, largo—, o false si no se ve: en la
// clásica, siempre que no quepa, en la pista entre los dos botones; superpuesto, solo
// con el puntero encima y a lo ancho de la tira.
func (t *tiraHojas) pulgar() (x, y, largo float32, ok bool) {
	tam, total := t.Size(), t.anchoContenido()
	if tam.Width <= 0 || total <= tam.Width {
		return 0, 0, 0, false
	}
	if t.clasica() {
		pista := tam.Width - 2*LadoBotonClasico
		return LadoBotonClasico + t.desX/total*pista, tam.Height - AltoBarraClasica + MargenPulgarTira, pista * tam.Width / total, true
	}
	if !t.conPuntero() {
		return 0, 0, 0, false
	}
	return t.desX / total * tam.Width, tam.Height - MargenPulgarTira - AltoPulgarTira, tam.Width * tam.Width / total, true
}

// enPulgar dice si el punto cae en la columna del pulgar, a todo el alto de la barra
// (como el navegador: el pulgar se coge también por sus dos márgenes).
func (t *tiraHojas) enPulgar(p fyne.Position) bool {
	x, _, largo, ok := t.pulgar()
	return ok && p.X >= x && p.X < x+largo && p.Y >= t.Size().Height-AltoBarraClasica
}

// correr mueve la tira `d` hacia la derecha, acotada.
func (t *tiraHojas) correr(d float32) {
	t.desX = clampF(t.desX+d, 0, max(t.anchoContenido()-t.Size().Width, 0))
	t.Refresh()
}

// Tapped en la barra clásica: una flecha corre 40 px, y la pista, una página hacia el
// lado del clic.
func (t *tiraHojas) Tapped(e *fyne.PointEvent) {
	if !t.clasica() || e.Position.Y < t.Size().Height-AltoBarraClasica || t.enPulgar(e.Position) {
		return
	}
	x, _, _, _ := t.pulgar()
	pagina := FraccionPagina * t.Size().Width
	switch {
	case e.Position.X < LadoBotonClasico:
		t.correr(-PasoFlechaClasica)
	case e.Position.X >= t.Size().Width-LadoBotonClasico:
		t.correr(PasoFlechaClasica)
	case e.Position.X < x:
		t.correr(-pagina)
	default:
		t.correr(pagina)
	}
}

// Dragged arrastra el pulgar: la tira corre lo que corre el puntero, a escala de lo que
// mide entera sobre la pista. Un arrastre que no empieza en el pulgar no hace nada.
func (t *tiraHojas) Dragged(e *fyne.DragEvent) {
	if !t.arrastrandoPulgar {
		if t.arrastreSuelto || !t.enPulgar(e.Position.SubtractXY(e.Dragged.DX, e.Dragged.DY)) {
			t.arrastreSuelto = true
			return
		}
		t.arrastrandoPulgar = true
	}
	pista := t.Size().Width
	if t.clasica() {
		pista -= 2 * LadoBotonClasico
	}
	if pista > 0 {
		t.correr(e.Dragged.DX * t.anchoContenido() / pista)
	}
}

// DragEnd suelta el pulgar.
func (t *tiraHojas) DragEnd() {
	t.arrastrandoPulgar, t.arrastreSuelto = false, false
	t.Refresh()
}

// sincronizar deja una pestaña por hoja, con su nombre y su estado.
func (t *tiraHojas) sincronizar() {
	hojas := t.p.lz.Libro.Hojas()
	for len(t.pestanas) < len(hojas) {
		ph := &pestanaHoja{tira: t, i: len(t.pestanas)}
		ph.ExtendBaseWidget(ph)
		t.pestanas = append(t.pestanas, ph)
	}
	t.pestanas = t.pestanas[:len(hojas)]
	for i, ph := range t.pestanas {
		ph.nombre, ph.activa = hojas[i], i == t.p.lz.Libro.Activa()
		ph.color = t.p.lz.Libro.ColorPestana(i)
	}
	t.Refresh()
}

// Scrolled corre la tira; la rueda vertical también, como hace el navegador con una
// fila que solo se desplaza en horizontal.
func (t *tiraHojas) Scrolled(e *fyne.ScrollEvent) {
	d := e.Scrolled.DX
	if d == 0 {
		d = e.Scrolled.DY
	}
	t.correr(-d)
}

func (t *tiraHojas) anchoContenido() float32 {
	var x float32
	for i, ph := range t.pestanas {
		if i > 0 {
			x += HuecoPestanas
		}
		x += MedidaPestanaHoja(ph.nombre, ph.activa).Width
	}
	return x
}

// MinSize es cero de ancho: `min-w-0 flex-1`, se lleva lo que sobre.
func (t *tiraHojas) MinSize() fyne.Size {
	t.ExtendBaseWidget(t)
	return fyne.NewSize(0, altoContenidoTira)
}

// CreateRenderer coloca las pestañas en fila.
func (t *tiraHojas) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	pulgar := canvas.NewRectangle(tema.Color("border-strong", v))
	pulgar.CornerRadius = AltoPulgarTira / 2
	pulgar.Hidden = true
	// Las flechas de la barra clásica, en la rejilla de 24 de los iconos.
	izq := iconos.RellenoDeRuta("M10 12L24 0V24Z", ladoFlecha, tema.Color("border-strong", v))
	der := iconos.RellenoDeRuta("M0 0L14 12L0 24Z", ladoFlecha, tema.Color("border-strong", v))
	izq.Ver(false)
	der.Ver(false)
	return &tiraRenderer{t: t, pulgar: pulgar, flechas: [2]*iconos.Dibujo{izq, der}}
}

type tiraRenderer struct {
	t       *tiraHojas
	pulgar  *canvas.Rectangle
	flechas [2]*iconos.Dibujo
}

func (r *tiraRenderer) MinSize() fyne.Size { return r.t.MinSize() }
func (r *tiraRenderer) Destroy()           {}

func (r *tiraRenderer) Objects() []fyne.CanvasObject {
	o := make([]fyne.CanvasObject, len(r.t.pestanas), len(r.t.pestanas)+1)
	for i, ph := range r.t.pestanas {
		o[i] = ph
	}
	return append(o, r.pulgar, r.flechas[0].Objeto(), r.flechas[1].Objeto()) // encima de las pestañas
}

func (r *tiraRenderer) Layout(tam fyne.Size) {
	clasica := r.t.clasica()
	alto := tam.Height
	if clasica {
		alto -= AltoBarraClasica // el `h-full` de las pestañas es lo que queda encima
	}
	x := -r.t.desX
	for _, ph := range r.t.pestanas {
		m := MedidaPestanaHoja(ph.nombre, ph.activa)
		m.Height = min(m.Height, alto)
		ph.Move(fyne.NewPos(x, (alto-m.Height)/2))
		ph.Resize(m)
		x += m.Width + HuecoPestanas
	}
	r.flechas[0].Ver(clasica)
	r.flechas[1].Ver(clasica)
	if clasica {
		y := alto + (AltoBarraClasica-ladoFlecha)/2
		r.flechas[0].Colocar(0, y, ladoFlecha) // el triángulo va de 2,5 a 6 de la punta
		r.flechas[1].Colocar(tam.Width-ladoFlecha, y, ladoFlecha)
	}
	x, y, largo, ok := r.t.pulgar()
	r.pulgar.Hidden = !ok
	if ok {
		r.pulgar.Move(fyne.NewPos(x, y))
		r.pulgar.Resize(fyne.NewSize(largo, AltoPulgarTira))
	}
}

func (r *tiraRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.pulgar.FillColor = tema.Color("border-strong", v)
	if r.t.clasica() && r.t.arrastrandoPulgar {
		r.pulgar.FillColor = PulgarArrastrando(v)
	} else if r.t.clasica() && r.t.sobrePulgar {
		r.pulgar.FillColor = PulgarEncima(v)
	}
	r.flechas[0].Tenir("border-strong", v)
	r.flechas[1].Tenir("border-strong", v)
	r.Layout(r.t.Size())
	r.pulgar.Refresh()
	for _, ph := range r.t.pestanas {
		ph.Refresh()
	}
}

// soltar es el `onDrop`: la pestaña arrastrada va a la posición de la que cae debajo.
func (t *tiraHojas) soltar() {
	de := t.arrastre
	t.arrastre = -1
	if de < 0 {
		return
	}
	for i, ph := range t.pestanas {
		if t.soltarX >= ph.Position().X && t.soltarX < ph.Position().X+ph.Size().Width {
			t.p.lz.MoverHoja(de, i)
			return
		}
	}
}

// ---- una pestaña ----

// pestanaHoja es un `<button>` de la tira: clic cambia de hoja, doble clic renombra y
// arrastrar reordena. `aria-pressed` en la activa.
type pestanaHoja struct {
	widget.BaseWidget

	tira             *tiraHojas
	i                int
	nombre           string
	activa           bool
	encima, enfocado bool
	// color es el de la pestaña (`tabColor`): una banda debajo del nombre, como la web.
	color string
}

var (
	_ fyne.Tappable       = (*pestanaHoja)(nil)
	_ fyne.DoubleTappable = (*pestanaHoja)(nil)
	_ fyne.Focusable      = (*pestanaHoja)(nil)
	_ fyne.Draggable      = (*pestanaHoja)(nil)
	_ fyne.Accessible     = (*pestanaHoja)(nil)
	_ desktop.Hoverable   = (*pestanaHoja)(nil)
)

func (ph *pestanaHoja) Tapped(*fyne.PointEvent)        { ph.tira.p.lz.CambiarHoja(ph.i) }
func (ph *pestanaHoja) DoubleTapped(*fyne.PointEvent)  { ph.tira.p.lz.PedirRenombrarHoja(ph.i) }
func (ph *pestanaHoja) FocusGained()                   { ph.enfocado = true; ph.Refresh() }
func (ph *pestanaHoja) FocusLost()                     { ph.enfocado = false; ph.Refresh() }
func (ph *pestanaHoja) MouseMoved(*desktop.MouseEvent) {}

func (ph *pestanaHoja) MouseIn(*desktop.MouseEvent) {
	ph.encima = true
	ph.Refresh()
	ph.tira.Refresh()
}
func (ph *pestanaHoja) MouseOut() { ph.encima = false; ph.Refresh(); ph.tira.Refresh() }

// TypedRune y TypedKey: Espacio e Intro pulsan, como en cualquier `<button>`.
func (ph *pestanaHoja) TypedRune(r rune) {
	if r == ' ' {
		ph.Tapped(nil)
	}
}

func (ph *pestanaHoja) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(ph, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		ph.Tapped(nil)
	}
}

// Dragged sigue el puntero en coordenadas de la TIRA, que es donde se decide sobre qué
// pestaña cae.
func (ph *pestanaHoja) Dragged(e *fyne.DragEvent) {
	if lz := ph.tira.p.lz; lz.Rejilla.SoloLectura || lz.Libro.ProteccionLibro() != nil { // `draggable={!protectedWorkbook}`
		return
	}
	ph.tira.arrastre = ph.i
	ph.tira.soltarX = ph.Position().X + e.Position.X
}

// DragEnd suelta.
func (ph *pestanaHoja) DragEnd() { ph.tira.soltar() }

// AccessibilityLabel es el nombre de la hoja, que es el texto del botón.
func (ph *pestanaHoja) AccessibilityLabel() string { return ph.nombre }

// AccessibilityRole es botón.
func (ph *pestanaHoja) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// MinSize es su medida declarada.
func (ph *pestanaHoja) MinSize() fyne.Size {
	ph.ExtendBaseWidget(ph)
	return MedidaPestanaHoja(ph.nombre, ph.activa)
}

// CreateRenderer compone el aro, el nombre y el subrayado.
func (ph *pestanaHoja) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pestanaHojaRenderer{
		ph:        ph,
		aro:       ui.NuevoAro(0, ui.SinHueco, v), // `sytue-focus-ring` sin redondeo
		texto:     ui.Texto("", EscalonPestanas, ui.Normal, color.NRGBA{}),
		indicador: canvas.NewRectangle(tema.Color("indicator", v)),
		banda:     canvas.NewRectangle(color.NRGBA{}),
	}
	r.indicador.CornerRadius = GrosorIndicador / 2 // `rounded-full`
	r.banda.CornerRadius = GrosorBandaColor / 2
	r.objetos = r.aro.Antes(r.texto, r.banda, r.indicador)
	r.Refresh()
	return r
}

type pestanaHojaRenderer struct {
	ph        *pestanaHoja
	aro       *ui.Aro
	texto     *canvas.Text
	indicador *canvas.Rectangle
	banda     *canvas.Rectangle
	objetos   []fyne.CanvasObject
}

// GrosorBandaColor es el alto de la banda del color de la pestaña (`h-1`).
const GrosorBandaColor = 4

func (r *pestanaHojaRenderer) MinSize() fyne.Size           { return r.ph.MinSize() }
func (r *pestanaHojaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pestanaHojaRenderer) Destroy()                     {}

func (r *pestanaHojaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	peso := pesoPestana(r.ph.activa)
	ui.Truncar(r.texto, r.ph.nombre, EscalonPestanas, peso, t.Width-2*MargenPestana)
	ui.ColocarTexto(r.texto, EscalonPestanas, MargenPestana, (t.Height-EscalonPestanas.Linea)/2)
	r.indicador.Move(fyne.NewPos(SangriaIndicador, t.Height-GrosorIndicador))
	r.indicador.Resize(fyne.NewSize(max(t.Width-2*SangriaIndicador, 0), GrosorIndicador))
	r.banda.Move(fyne.NewPos(SangriaIndicador, t.Height-GrosorBandaColor))
	r.banda.Resize(fyne.NewSize(max(t.Width-2*SangriaIndicador, 0), GrosorBandaColor))
}

func (r *pestanaHojaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ph := r.ph
	// `text-text font-medium` la activa; `text-text-muted hover:text-text` el resto.
	token := "text-muted"
	if ph.activa || ph.encima {
		token = "text"
	}
	r.texto.Color = tema.Color(token, v)
	r.texto.FontSource = pesoPestana(ph.activa).Cara()
	r.indicador.FillColor = tema.Color("indicator", v)
	if ph.activa {
		r.indicador.Show()
	} else {
		r.indicador.Hide()
	}
	if c, ok := editor.Hex(ph.color); ok {
		r.banda.FillColor = c
		r.banda.Show()
	} else {
		r.banda.Hide()
	}
	r.aro.Pintar(v)
	r.aro.Ver(ph.enfocado)
	r.Layout(ph.Size())
	r.texto.Refresh()
	r.banda.Refresh()
	r.indicador.Refresh()
}

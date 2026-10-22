// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Medidas de la vista del moderador (`PresenterView` de SlidesCanvas.tsx). Son px
// literales de sus estilos en línea, no la escala de la suite, y están medidas en su
// ventana de 1040 × 720 con Playwright.
const (
	// AnchoModerador y AltoModerador son el `window.open(…, 'width=1040,height=720')`.
	AnchoModerador float32 = 1040
	AltoModerador  float32 = 720
	// Los `padding` de la cabecera (12 20), del centro (18 20), de las notas (12 20) y
	// del pie (10 20); los huecos entre columnas (24), bajo un rótulo (8) y en la
	// cabecera (14).
	margenModerador       float32 = 20
	arribaCabecera        float32 = 12
	arribaCentro          float32 = 18
	arribaNotas           float32 = 12
	arribaPie             float32 = 10
	huecoColumnas         float32 = 24
	huecoRotulo           float32 = 8
	huecoCabecera         float32 = 14
	huecoBotonesPie       float32 = 8
	margenNotas           float32 = 6 // el `margin: 6px 0 0` del párrafo
	altoMinNotas          float32 = 92
	fraccionNotas         float32 = 0.26 // `flex: 0 0 26%`
	radioVistaPrevia      float32 = 4
	trackingRotuloModerad float32 = 0.08 // `letterSpacing: 0.08em`
	trackingTiempoModerad float32 = 0.02 // `letterSpacing: 0.02em`
)

// Tintas de la ventana: `CONTENT_PALETTE[3]`, `[2]` y el `color-mix(in srgb, white 16%,
// transparent)` de las rayas.
var (
	blancoModerador = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	grisModerador   = color.NRGBA{R: 0x94, G: 0xA3, B: 0xB8, A: 255}
	rayaModerador   = color.NRGBA{R: 255, G: 255, B: 255, A: 41}
	// Los botones siguen la esquina compartida también en la ventana del moderador.
	radioBotonModerador = tema.Forma("radius-control")
)

// Tipografía: la ventana hereda el `line-height` del cuerpo (22 con rem 16) y solo cambia el
// tamaño de letra, salvo en el párrafo de notas, que lleva 1,5.
var (
	letraCuerpo     = tema.TextoBase.Linea
	escTiempo       = tema.Escalon{Tam: tema.Texto2XL.Tam, Linea: letraCuerpo}
	escBase         = tema.Escalon{Tam: tema.TextoBase.Tam, Linea: letraCuerpo}
	escPequeno      = tema.Escalon{Tam: tema.TextoXS.Tam, Linea: letraCuerpo}
	escRotulo       = tema.Escalon{Tam: tema.Texto2XS.Tam, Linea: letraCuerpo}
	escNotasModerad = tema.Escalon{Tam: tema.TextoMD.Tam, Linea: tema.TextoMD.Tam * 1.5}
)

// cadaSegundo llama a `f` en el hilo de la interfaz una vez por segundo hasta que se
// llama a lo que devuelve. Es una variable para que las pruebas no dejen relojes vivos.
var cadaSegundo = func(f func()) (parar func()) {
	fin := make(chan struct{})
	go func() {
		tic := time.NewTicker(time.Second)
		defer tic.Stop()
		for {
			select {
			case <-tic.C:
				fyne.Do(f)
			case <-fin:
				return
			}
		}
	}()
	return func() { close(fin) }
}

// Moderador es la vista del moderador: la segunda ventana con el cronómetro y el reloj,
// la diapositiva actual en su paso y la siguiente, las notas y los mandos. No tiene
// estado propio de navegación: pinta la posición de la presentación y le manda los
// gestos, como hace la web con sus `onPrev`/`onNext`.
//
// ponytail: las notas que no caben en su franja se cortan (`overflow: auto` no está).
type Moderador struct {
	widget.BaseWidget

	p      *Presentacion
	idioma i18n.Locale
	inicio time.Time
	ahora  func() time.Time
	// AlVolver, si no es nil, es tocar la diapositiva actual: en un teléfono, donde la
	// vista va en la misma ventana que la presentación, es volver a ella.
	AlVolver func()
	r        *moderadorRenderer
}

var (
	_ fyne.Widget   = (*Moderador)(nil)
	_ fyne.Tappable = (*Moderador)(nil)
)

// Tapped vuelve a la presentación si el toque cae en la diapositiva actual.
func (m *Moderador) Tapped(e *fyne.PointEvent) {
	if m.AlVolver == nil || m.r == nil {
		return
	}
	a := m.r.actual
	if d := e.Position.Subtract(a.Position()); d.X >= 0 && d.Y >= 0 && d.X <= a.Size().Width && d.Y <= a.Size().Height {
		m.AlVolver()
	}
}

// NuevoModerador monta la vista de la presentación `p`, con el cronómetro a cero.
func NuevoModerador(p *Presentacion, idioma i18n.Locale) *Moderador {
	m := &Moderador{p: p, idioma: idioma, ahora: time.Now}
	m.inicio = m.ahora()
	m.ExtendBaseWidget(m)
	return m
}

// Reiniciar pone el cronómetro a cero (el botón «Reiniciar»).
func (m *Moderador) Reiniciar() {
	m.inicio = m.ahora()
	m.Refresh()
}

// hora es el reloj de la cabecera: `toLocaleTimeString(lang, { hour: '2-digit',
// minute: '2-digit' })`, que es la hora corta del idioma con la hora a dos cifras.
func (m *Moderador) hora() string {
	s := []rune(i18n.FormatTime(m.idioma, m.ahora()))
	if len(s) > 1 && unicode.IsDigit(s[0]) && !unicode.IsDigit(s[1]) {
		return "0" + string(s)
	}
	return string(s)
}

// AnchoVistaPrevia es `max(220, min(560, floor((ancho − 72) / 2)))`.
func AnchoVistaPrevia(ancho float32) float32 {
	return max(220, min(560, float32(int((ancho-72)/2))))
}

func (m *Moderador) CreateRenderer() fyne.WidgetRenderer {
	p := m.p
	r := &moderadorRenderer{
		m:        m,
		fondo:    canvas.NewRectangle(tinta),
		reloj:    ui.Texto("", escBase, ui.Normal, grisModerador),
		posicion: ui.Texto("", escBase, ui.Normal, grisModerador),
		pista:    ui.Texto(p.t("slides.present.hint", nil), escPequeno, ui.Normal, grisModerador),
		rotActual: ui.NuevoVersalCon(p.t("slides.presenter.current", nil), escRotulo, ui.Normal,
			grisModerador, trackingRotuloModerad),
		rotSiguiente: ui.NuevoVersalCon(p.t("nav.nextItem", nil), escRotulo, ui.Normal,
			grisModerador, trackingRotuloModerad),
		rotNotas: ui.NuevoVersalCon(p.t("slides.notes", nil), escRotulo, ui.Normal,
			grisModerador, trackingRotuloModerad),
		fin:      ui.Texto(p.t("slides.presenter.endOfDeck", nil), escBase, ui.Normal, grisModerador),
		sombraA:  ui.NuevaSombraCon(editor.CapasSombraPagina, radioVistaPrevia),
		sombraS:  ui.NuevaSombraCon(editor.CapasSombraPagina, radioVistaPrevia),
		diapo:    -1,
		reinicia: nuevoBotonModerador(p.t("slides.presenter.reset", nil), true, m.Reiniciar),
		anterior: nuevoBotonModerador("← "+p.t("nav.previous", nil), false, func() { p.Avanzar(-1) }),
		siguien:  nuevoBotonModerador(p.t("nav.nextItem", nil)+" →", false, func() { p.Avanzar(1) }),
		terminar: nuevoBotonModerador(p.t("slides.presenter.end", nil), true, p.salir),
	}
	for i := range r.rayas {
		r.rayas[i] = canvas.NewRectangle(rayaModerador)
	}
	r.actual = NuevoLienzo(p.M, p.Diapo)
	r.siguiente = NuevoLienzo(p.M, p.Diapo)
	for _, l := range []*Lienzo{r.actual, r.siguiente} {
		l.Hueco, l.ConPasos, l.Radio = 0, true, radioVistaPrevia
		l.Rotulo = func(c string) string { return p.t(c, nil) }
	}
	m.r = r
	r.Refresh()
	return r
}

type moderadorRenderer struct {
	m                           *Moderador
	fondo                       *canvas.Rectangle
	rayas                       [3]*canvas.Rectangle
	reloj, posicion             *canvas.Text
	tiempo                      *ui.TextoTabular
	tiempoObjeto                fyne.CanvasObject
	tiempoTexto                 string
	pista, fin                  *canvas.Text
	rotActual, rotSiguiente     *ui.Versal
	rotNotas                    *ui.Versal
	actual, siguiente           *Lienzo
	sombraA, sombraS            *ui.Sombra
	marcoFin                    fyne.CanvasObject
	tamFin                      fyne.Size
	notas                       []*canvas.Text
	reinicia, anterior, siguien *botonModerador
	terminar                    *botonModerador
	diapo, paso                 int
	haySiguiente, listo         bool
}

func (r *moderadorRenderer) MinSize() fyne.Size { return fyne.NewSize(1, 1) }
func (r *moderadorRenderer) Destroy()           {}

func (r *moderadorRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{
		r.fondo, r.rayas[0], r.rayas[1], r.rayas[2], r.tiempoObjeto, r.reinicia,
		r.reloj, r.posicion, r.sombraA.Objeto(), r.actual,
	}
	objs = append(objs, r.rotActual.Objetos()...)
	objs = append(objs, r.rotSiguiente.Objetos()...)
	if r.haySiguiente {
		objs = append(objs, r.sombraS.Objeto(), r.siguiente)
	} else if r.marcoFin != nil {
		objs = append(objs, r.marcoFin, r.fin)
	}
	objs = append(objs, r.rotNotas.Objetos()...)
	for _, t := range r.notas {
		objs = append(objs, t)
	}
	return append(objs, r.anterior, r.siguien, r.pista, r.terminar)
}

// Refresh repinta los textos que cambian con el reloj y, solo si la presentación se ha
// movido, las dos diapositivas: el reloj pasa cada segundo y rasterizarlas en cada
// pasada sería el grueso del trabajo de la ventana.
func (r *moderadorRenderer) Refresh() {
	m, p := r.m, r.m.p
	transcurrido := Transcurrido(int(m.ahora().Sub(m.inicio) / time.Second))
	if r.tiempo == nil || transcurrido != r.tiempoTexto {
		r.tiempoTexto = transcurrido
		r.tiempo = ui.NuevoTextoTabular(transcurrido, escTiempo, ui.Negrita, blancoModerador).
			ConEspaciado(trackingTiempoModerad)
		r.tiempoObjeto = r.tiempo.Objeto()
	}
	r.reloj.Text = m.hora()
	r.posicion.Text = p.t("slides.aria.slideOf", i18nVars{"n": p.Diapo + 1, "total": len(p.M.Diapos)})
	if !r.listo || p.Diapo != r.diapo || p.Paso != r.paso {
		r.listo, r.diapo, r.paso = true, p.Diapo, p.Paso
		r.actual.Indice, r.actual.Paso = p.Diapo, p.Paso
		r.actual.Refresh()
		r.haySiguiente = p.Diapo < p.ultima()
		if r.haySiguiente {
			r.siguiente.Indice, r.siguiente.Paso = p.Diapo+1, 0
			r.siguiente.Refresh()
		}
	}
	r.Layout(m.Size())
	for _, t := range []*canvas.Text{r.reloj, r.posicion} {
		t.Refresh()
	}
	canvas.Refresh(m)
}

func (r *moderadorRenderer) Layout(t fyne.Size) {
	p := r.m.p
	ui.FondoHastaElBorde(r.m, r.fondo, t)
	x0, ancho := margenModerador, t.Width-2*margenModerador

	// `align-items: baseline`: cada pieza aporta lo que ocupa por encima y debajo de
	// su línea base; el botón incluye su borde y padding, los spans su caja de línea.
	// Blink usa métricas enteras para el baseline del flex layout. El medio
	// píxel de LineaBase al apretar una línea sirve al pintado a ×2, no a esa fila.
	baseTiempo := float32(math.Round(float64(ui.LineaBase(escTiempo, ui.Negrita))))
	baseReloj := ui.LineaBase(escBase, ui.Normal)
	eBoton, _, _, pyBoton := r.reinicia.estilo()
	baseBoton := 1 + pyBoton + ui.LineaBase(eBoton, ui.Normal)
	base := max(baseTiempo, baseBoton, baseReloj)
	fila := base + max(escTiempo.Linea-baseTiempo,
		r.reinicia.MinSize().Height-baseBoton, escBase.Linea-baseReloj)
	altoCab := arribaCabecera + fila + arribaCabecera + 1
	r.tiempoObjeto.Move(fyne.NewPos(x0, arribaCabecera+base-baseTiempo))
	r.tiempoObjeto.Resize(fyne.NewSize(r.tiempo.Ancho(), escTiempo.Linea))
	x := x0 + r.tiempo.Ancho() + huecoCabecera
	r.reinicia.Move(fyne.NewPos(x, arribaCabecera+base-baseBoton))
	r.reinicia.Resize(r.reinicia.MinSize())
	x += r.reinicia.MinSize().Width + huecoCabecera
	ui.ColocarTexto(r.reloj, escBase, x, arribaCabecera+base-baseReloj)
	w := ui.Medir(r.posicion.Text, escBase, ui.Normal).Width
	ui.ColocarTexto(r.posicion, escBase, t.Width-margenModerador-w, arribaCabecera+(fila-letraCuerpo)/2)
	raya(r.rayas[0], 0, altoCab-1, t.Width)

	// Pie y notas se miden desde abajo; el centro se queda con lo que sobra.
	altoPie := arribaPie + r.anterior.MinSize().Height + arribaPie + 1
	yPie := t.Height - altoPie
	altoNotas := max(altoMinNotas, t.Height*fraccionNotas)
	yNotas := yPie - altoNotas

	// Centro: dos columnas de `vista previa` centradas, con su rótulo encima.
	vw := AnchoVistaPrevia(t.Width)
	vh := vw * p.M.AltoEscenario() / AnchoEscenario
	xc := x0 + (ancho-(2*vw+huecoColumnas))/2
	yc := altoCab + arribaCentro
	yv := yc + letraCuerpo + huecoRotulo
	r.rotActual.Colocar(xc, yc)
	r.rotSiguiente.Colocar(xc+vw+huecoColumnas, yc)
	previa(r.actual, r.sombraA, fyne.NewPos(xc, yv), fyne.NewSize(vw, vh))
	xs := xc + vw + huecoColumnas
	if r.haySiguiente {
		previa(r.siguiente, r.sombraS, fyne.NewPos(xs, yv), fyne.NewSize(vw, vh))
	} else {
		if r.marcoFin == nil || r.tamFin != fyne.NewSize(vw, vh) {
			r.marcoFin, r.tamFin = marcoFinal(fyne.NewSize(vw, vh)), fyne.NewSize(vw, vh)
		}
		r.marcoFin.Move(fyne.NewPos(xs, yv))
		fw := ui.Medir(r.fin.Text, escBase, ui.Normal).Width
		ui.ColocarTexto(r.fin, escBase, xs+(vw-fw)/2, yv+(vh-letraCuerpo)/2)
	}

	// Notas: el rótulo y el párrafo, o «Sin notas…» en gris y más pequeño.
	raya(r.rayas[1], 0, yNotas, t.Width)
	yr := yNotas + 1 + arribaNotas
	r.rotNotas.Colocar(x0, yr)
	r.colocarNotas(x0, yr+letraCuerpo+margenNotas, ancho, yPie)

	// Pie: los dos botones, la pista y «Terminar», repartidos con `space-between`.
	raya(r.rayas[2], 0, yPie, t.Width)
	yb := yPie + 1 + arribaPie
	alto := r.anterior.MinSize().Height
	r.anterior.Move(fyne.NewPos(x0, yb))
	r.anterior.Resize(r.anterior.MinSize())
	xs2 := x0 + r.anterior.MinSize().Width + huecoBotonesPie
	r.siguien.Move(fyne.NewPos(xs2, yb))
	r.siguien.Resize(r.siguien.MinSize())
	izq := xs2 + r.siguien.MinSize().Width
	tm := r.terminar.MinSize()
	r.terminar.Move(fyne.NewPos(t.Width-margenModerador-tm.Width, yb+(alto-tm.Height)/2))
	r.terminar.Resize(tm)
	pw := ui.Medir(r.pista.Text, escPequeno, ui.Normal).Width
	libre := t.Width - margenModerador - tm.Width - izq - pw
	ui.ColocarTexto(r.pista, escPequeno, izq+libre/2, yb+(alto-letraCuerpo)/2)
}

// colocarNotas reparte las notas en líneas del ancho de la franja y pinta las que
// caben por encima del pie.
func (r *moderadorRenderer) colocarNotas(x, y, ancho, tope float32) {
	d := r.m.p.M.Diapos[r.m.p.Diapo]
	r.notas = r.notas[:0]
	e, c, lineas := escNotasModerad, blancoModerador, ui.Lineas(d.Notas, escNotasModerad, ui.Normal, ancho)
	if d.Notas == "" || len(lineas) == 0 || (len(lineas) == 1 && lineas[0] == "") {
		e, c, lineas = escBase, grisModerador, []string{r.m.p.t("slides.presenter.noNotes", nil)}
	}
	for _, l := range lineas {
		if y+e.Linea > tope {
			break
		}
		txt := ui.Texto(l, e, ui.Normal, c)
		ui.ColocarTexto(txt, e, x, y)
		r.notas = append(r.notas, txt)
		y += e.Linea
	}
}

func raya(r *canvas.Rectangle, x, y, ancho float32) {
	r.Move(fyne.NewPos(x, y))
	r.Resize(fyne.NewSize(ancho, 1))
}

// previa coloca una diapositiva de la vista del moderador (`PresenterSlide`) con la
// sombra de página debajo.
func previa(l *Lienzo, s *ui.Sombra, pos fyne.Position, tam fyne.Size) {
	l.Move(pos)
	l.Resize(tam)
	s.Ajustar(pos, tam, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
}

// marcoFinal es el hueco de la siguiente cuando ya no hay: `1px dashed` de la raya, con
// su radio de 4. La raya se da ya mezclada con la tinta del fondo, que es lisa.
func marcoFinal(tam fyne.Size) fyne.CanvasObject {
	w, h := float64(tam.Width), float64(tam.Height)
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" width="%s" height="%s">`+
		`<rect x="0.5" y="0.5" width="%s" height="%s" rx="%s" fill="none" stroke="#313131"`+
		` stroke-width="1" stroke-dasharray="3 3"/></svg>`,
		n2(w), n2(h), n2(w), n2(h), n2(w-1), n2(h-1), n2(float64(radioVistaPrevia)))
	return iconos.DeSVG([]byte(svg), tam).Objeto()
}

// ---- botones ----

// botonModerador es el `<button>` con estilo en línea de la ventana: borde de la raya,
// radio 6, sin fondo; `padding: 8px 14px` y letra `text-base` en blanco, o el fantasma
// (`4px 10px`, `text-xs`, gris) para «Reiniciar» y «Terminar».
type botonModerador struct {
	widget.BaseWidget
	texto    string
	fantasma bool
	alPulsar func()
}

var (
	_ fyne.Tappable      = (*botonModerador)(nil)
	_ desktop.Cursorable = (*botonModerador)(nil)
)

func nuevoBotonModerador(texto string, fantasma bool, f func()) *botonModerador {
	b := &botonModerador{texto: texto, fantasma: fantasma, alPulsar: f}
	b.ExtendBaseWidget(b)
	return b
}

func (b *botonModerador) estilo() (e tema.Escalon, c color.NRGBA, px, py float32) {
	if b.fantasma {
		return escPequeno, grisModerador, 10, 4
	}
	return escBase, blancoModerador, 14, 8
}

func (b *botonModerador) Tapped(*fyne.PointEvent) {
	if b.alPulsar != nil {
		b.alPulsar()
	}
}

func (b *botonModerador) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (b *botonModerador) AccessibilityLabel() string             { return b.texto }
func (b *botonModerador) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// La cara web de Figtree está limitada por unicode-range: ← y → los pinta la
// reserva (Noto Sans Math en Chromium/Linux, comprobado por CDP el 2026-10-04).
// La TTF completa sí los contiene, así que se separan del rótulo para no usar sus
// avances distintos. El recurso compartido fija Noto Sans Math 3.000 también en
// equipos sin esa familia instalada o con una versión anterior.
func (b *botonModerador) partes() (rotulo, flecha string, primero bool, cara fyne.Resource) {
	rotulo = b.texto
	switch {
	case strings.HasPrefix(rotulo, "←"):
		rotulo, flecha, primero = strings.TrimPrefix(rotulo, "←"), "←", true
	case strings.HasSuffix(rotulo, "→"):
		rotulo, flecha = strings.TrimSuffix(rotulo, "→"), "→"
	}
	if flecha != "" {
		cara = tema.Matematica
	}
	return rotulo, flecha, primero, cara
}

func (b *botonModerador) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	e, _, px, py := b.estilo()
	rotulo, flecha, _, cara := b.partes()
	ancho := ui.Medir(rotulo, e, ui.Normal).Width
	if flecha != "" {
		ancho += ui.MedirConCara(flecha, e, cara).Width
	}
	return fyne.NewSize(ancho+2*px+2, e.Linea+2*py+2)
}

func (b *botonModerador) CreateRenderer() fyne.WidgetRenderer {
	e, c, _, _ := b.estilo()
	borde := canvas.NewRectangle(color.Transparent)
	borde.StrokeColor, borde.StrokeWidth, borde.CornerRadius = rayaModerador, 1, radioBotonModerador
	rotulo, flecha, primero, cara := b.partes()
	r := &botonModeradorRenderer{b: b, borde: borde, texto: ui.Texto(rotulo, e, ui.Normal, c), primero: primero}
	if flecha != "" {
		r.flecha = ui.TextoConCara(flecha, e, cara, c)
	}
	return r
}

type botonModeradorRenderer struct {
	b       *botonModerador
	borde   *canvas.Rectangle
	texto   *canvas.Text
	flecha  *canvas.Text
	primero bool
}

func (r *botonModeradorRenderer) Layout(t fyne.Size) {
	e, _, px, py := r.b.estilo()
	r.borde.Resize(t)
	x, y := px+1, py+1
	colocarFlecha := func() {
		// El glifo de reserva comparte la línea base del rótulo Figtree, como un
		// fallback dentro del mismo run CSS; su propia caja de línea no manda.
		desfase := ui.LineaBase(e, ui.Normal) - ui.LineaBaseConCara(e, r.flecha.FontSource)
		ui.ColocarTexto(r.flecha, e, x, y+desfase)
		x += ui.MedirConCara(r.flecha.Text, e, r.flecha.FontSource).Width
	}
	if r.flecha != nil && r.primero {
		colocarFlecha()
	}
	ui.ColocarTexto(r.texto, e, x, y)
	x += ui.Medir(r.texto.Text, e, ui.Normal).Width
	if r.flecha != nil && !r.primero {
		colocarFlecha()
	}
}

func (r *botonModeradorRenderer) MinSize() fyne.Size { return r.b.MinSize() }
func (r *botonModeradorRenderer) Objects() []fyne.CanvasObject {
	objetos := []fyne.CanvasObject{r.borde, r.texto}
	if r.flecha != nil {
		objetos = append(objetos, r.flecha)
	}
	return objetos
}
func (r *botonModeradorRenderer) Refresh() { canvas.Refresh(r.b) }
func (r *botonModeradorRenderer) Destroy() {}

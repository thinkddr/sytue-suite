// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

const (
	bordeMenuMovil             float32 = 24
	decidirGesto               float32 = 12
	abrirMenuMovil             float32 = 64
	cerrarMenuMovil            float32 = 48
	actualizarListaMovil       float32 = 72
	dominioGesto               float32 = 1.5
	ladoIndicadorRefresco      float32 = 36
	iconoIndicadorRefresco     float32 = 18
	recorridoIndicadorRefresco float32 = 52
)

// RefrescoLista describes a list that can be refreshed. Arriba reads its actual
// scroll offset; Puede also checks loading, selection, connectivity and lifetime.
// The callback is the app's existing refresh action and runs on the UI thread.
type RefrescoLista struct {
	Arriba, Puede func() bool
	Actualizar    func()
	// Revision changes on account, folder, filter or view changes. A gesture
	// started on a previous revision never refreshes its replacement.
	Revision func() uint64
}

// RevisionLista adapts an app's non-negative request sequence without allowing a
// negative or wrapped signed counter to become an unrelated unsigned revision.
func RevisionLista(sequence int) uint64 {
	if sequence < 0 {
		return 0
	}
	return uint64(sequence)
}

// RefrescarAlTirar marks a list without intercepting its rows, taps or scrolling.
// The mobile canvas arbitrates a single touch sequence before dispatching it.
// Desktop input and accessibility continue to use the original controls.
func RefrescarAlTirar(content fyne.CanvasObject, options *RefrescoLista) fyne.CanvasObject {
	if !esMovil() {
		return content
	}
	w := &listaActualizable{content: content, options: options}
	w.ExtendBaseWidget(w)
	return w
}

type listaActualizable struct {
	widget.BaseWidget
	content fyne.CanvasObject
	options *RefrescoLista
}

func (l *listaActualizable) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(l.content)
}

func (l *listaActualizable) disponible() bool {
	return l.Visible() && l.options != nil && l.options.Actualizar != nil && l.options.Arriba != nil && l.options.Arriba() &&
		(l.options.Puede == nil || l.options.Puede())
}

// raizGestos owns one filter per canvas. It is part of the content tree, never a
// transparent input overlay; it has no Touchable, Tappable or Draggable methods.
type raizGestos struct {
	widget.BaseWidget
	content     fyne.CanvasObject
	canvas      fyne.Canvas
	gestos      gestosMoviles
	primerPlano bool
}

func conGestosMoviles(content fyne.CanvasObject) fyne.CanvasObject {
	if !esMovil() {
		return content
	}
	w := &raizGestos{content: content, primerPlano: true}
	w.ExtendBaseWidget(w)
	return w
}

func (w *raizGestos) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	sombra := NuevaSombra(tema.SombraMD, ladoIndicadorRefresco/2)
	sombra.Objeto().Hide()
	disco := canvas.NewCircle(tema.Color("surface-raised", v))
	disco.StrokeColor = tema.Color("border", v)
	disco.StrokeWidth = tema.Borde
	disco.Hidden = true
	glifo := iconos.Trazo(iconos.Rotar, iconoIndicadorRefresco, "text-muted", v)
	glifo.Objeto().Hide()
	r := &rendererGestos{
		WidgetRenderer: widget.NewSimpleRenderer(w.content),
		root:           w,
		sombra:         sombra,
		disco:          disco,
		glifo:          glifo,
	}
	w.gestos.alIndicador = r.colocarIndicador
	return r
}

type rendererGestos struct {
	fyne.WidgetRenderer
	root   *raizGestos
	sombra *Sombra
	disco  *canvas.Circle
	glifo  *iconos.Dibujo
}

func (r *rendererGestos) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.root.content, r.sombra.Objeto(), r.disco, r.glifo.Objeto()}
}

func (r *rendererGestos) colocarIndicador() {
	w := r.root
	mostrar := w.gestos.capturado && w.gestos.clase == gestoActualizar && w.gestos.arrastreY > 0
	if !mostrar {
		if r.disco.Visible() {
			r.sombra.Objeto().Hide()
			r.disco.Hide()
			r.glifo.Objeto().Hide()
			canvas.Refresh(r.disco)
		}
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t := w.Size()
	avance := float32(1-math.Exp(float64(-w.gestos.arrastreY/90))) * (recorridoIndicadorRefresco + ladoIndicadorRefresco)
	pos := fyne.NewPos((t.Width-ladoIndicadorRefresco)/2, -ladoIndicadorRefresco+avance)
	tam := fyne.NewSize(ladoIndicadorRefresco, ladoIndicadorRefresco)
	r.disco.FillColor = tema.Color("surface-raised", v)
	r.disco.StrokeColor = tema.Color("border", v)
	r.disco.Move(pos)
	r.disco.Resize(tam)
	r.sombra.Ajustar(pos, tam, v)
	tinta := "text-muted"
	if w.gestos.arrastreY >= actualizarListaMovil {
		tinta = "primary-text"
	}
	r.glifo.Tenir(tinta, v)
	r.glifo.Colocar(pos.X+(ladoIndicadorRefresco-iconoIndicadorRefresco)/2, pos.Y+(ladoIndicadorRefresco-iconoIndicadorRefresco)/2, iconoIndicadorRefresco)
	r.sombra.Objeto().Show()
	r.disco.Show()
	r.glifo.Objeto().Show()
	r.disco.Refresh()
}

func (r *rendererGestos) Layout(size fyne.Size) {
	r.WidgetRenderer.Layout(size)
	r.colocarIndicador()
	w := r.root
	c := lienzoDe(w)
	if c == nil {
		return
	}
	if c == w.canvas {
		sincronizarExclusionMenuMovil(c)
		return
	}
	filter, supported := c.(mobile.TouchFilterCanvas)
	if !supported {
		return
	}
	w.canvas = c
	w.gestos.canvas = c
	filter.SetTouchFilter(w.gestos.recibir)
	sincronizarExclusionMenuMovil(c)
}

func (r *rendererGestos) Refresh() {
	r.WidgetRenderer.Refresh()
	r.colocarIndicador()
	sincronizarExclusionMenuMovil(r.root.canvas)
}

// Recompute from the live canvas content: a renderer belonging to an old app view
// must never restore Android's small menu exclusion after a dialog or detail opens.
func sincronizarExclusionMenuMovil(c fyne.Canvas) {
	if c == nil {
		return
	}
	root, ok := c.Content().(*raizGestos)
	available := ok && root.canvas == c && root.primerPlano && !root.gestos.bloqueado()
	if _, editing := c.Focused().(mobile.Keyboardable); editing {
		available = false
	}
	if available {
		a := armazonEn(root.content)
		available = a != nil && menuPorGesto(a)
	}
	actualizarExclusionGestos(c, available)
}

type claseGesto uint8

const (
	gestoPendiente claseGesto = iota
	gestoMenu
	gestoCerrarMenu
	gestoActualizar
)

type gestosMoviles struct {
	canvas            fyne.Canvas
	id                int
	inicio            fyne.Position
	hora              time.Time
	menu              *Armazon
	cierre            *Armazon
	lista             *listaActualizable
	clase             claseGesto
	revision          uint64
	arrastreY         float32
	alIndicador       func()
	activo, capturado bool
}

func (g *gestosMoviles) actualizarIndicador(dy float32) {
	nuevo := max(dy, 0)
	if g.arrastreY == nuevo {
		return
	}
	g.arrastreY = nuevo
	if g.alIndicador != nil {
		g.alIndicador()
	}
}

func (g *gestosMoviles) cancelar() {
	menu, cierre := g.menu, g.cierre
	teniaIndicador := g.arrastreY > 0
	g.activo, g.capturado = false, false
	g.menu, g.cierre, g.lista = nil, nil, nil
	g.clase = gestoPendiente
	g.arrastreY = 0
	if teniaIndicador && g.alIndicador != nil {
		g.alIndicador()
	}
	if menu != nil && menu.arrastrandoCajon {
		menu.cancelarArrastreCajon()
	}
	if cierre != nil && cierre.arrastrandoCajon {
		cierre.cancelarArrastreCajon()
	}
}

func (g *gestosMoviles) bloqueado() bool {
	return g.canvas != nil && (g.canvas.Overlays().Top() != nil || dialogoArriba(g.canvas) != nil)
}

func (g *gestosMoviles) recibir(e mobile.FilterEvent) mobile.FilterResult {
	switch e.Phase {
	case mobile.FilterBegin:
		return g.empezar(e)
	case mobile.FilterCancel:
		g.cancelar()
		return mobile.FilterPass
	case mobile.FilterMove, mobile.FilterEnd:
		if !g.activo || e.ID != g.id {
			return mobile.FilterPass
		}
		return g.seguir(e)
	}
	return mobile.FilterPass
}

func (g *gestosMoviles) empezar(e mobile.FilterEvent) mobile.FilterResult {
	sincronizarExclusionMenuMovil(g.canvas)
	g.cancelar()
	if !g.puedeEmpezar(e) {
		return mobile.FilterPass
	}
	left := bordeIzquierdoLienzo(g.canvas)
	edita, bloqueaLista := g.recolectarObjetivos(e, left)
	if edita {
		g.cancelar()
		return mobile.FilterPass
	}
	if bloqueaLista {
		g.lista = nil
	}
	if g.menu == nil && g.cierre == nil && g.lista == nil {
		return mobile.FilterPass
	}
	g.id, g.inicio, g.hora, g.activo = e.ID, e.Position, e.Time, true
	if g.lista != nil && g.lista.options.Revision != nil {
		g.revision = g.lista.options.Revision()
	}
	return mobile.FilterDefer
}

func esControlDeEscritura(o fyne.CanvasObject) bool {
	switch o.(type) {
	case *Entrada, *AreaTexto, mobile.Keyboardable:
		return true
	}
	return false
}

func bloqueaRefrescoMovil(o fyne.CanvasObject) bool {
	switch o.(type) {
	case *BotonFlotante, *Aviso:
		return true
	}
	return false
}

func puntoEnObjeto(d fyne.Driver, o fyne.CanvasObject, punto, rMin, rMax fyne.Position) bool {
	if o == nil || !o.Visible() {
		return false
	}
	pos := d.AbsolutePositionForObject(o)
	tam := o.Size()
	x0, y0 := max(pos.X, rMin.X), max(pos.Y, rMin.Y)
	x1, y1 := min(pos.X+tam.Width, rMax.X), min(pos.Y+tam.Height, rMax.Y)
	return punto.X >= x0 && punto.X < x1 && punto.Y >= y0 && punto.Y < y1
}

func recortarObjeto(d fyne.Driver, o fyne.CanvasObject, rMin, rMax fyne.Position) (fyne.Position, fyne.Position) {
	pos := d.AbsolutePositionForObject(o)
	tam := o.Size()
	return fyne.NewPos(max(pos.X, rMin.X), max(pos.Y, rMin.Y)),
		fyne.NewPos(min(pos.X+tam.Width, rMax.X), min(pos.Y+tam.Height, rMax.Y))
}

// recolectarObjetivos combina `e.Path` con un recorrido del árbol visible del lienzo:
// en Fyne `touchPath` se queda solo con el último hermano visible que cubre el punto, de
// modo que las capas hermanas a pantalla completa (`CapaFlotante` del FAB y `PilaAvisos`
// sin avisos en ese punto) eclipsaban `*Armazon` y `*listaActualizable`.
func (g *gestosMoviles) recolectarObjetivos(e mobile.FilterEvent, left float32) (edita, bloqueaLista bool) {
	for _, object := range e.Path {
		if esControlDeEscritura(object) {
			return true, true
		}
		if bloqueaRefrescoMovil(object) {
			bloqueaLista = true
		}
		g.anotarObjetivo(object, e.Position.X, left)
	}
	if g.canvas == nil || g.canvas.Content() == nil {
		return false, bloqueaLista
	}
	app := fyne.CurrentApp()
	if app == nil {
		return false, bloqueaLista
	}
	tam := g.canvas.Size()
	r := &recorridoObjetivos{
		g:    g,
		d:    app.Driver(),
		pos:  e.Position,
		left: left,
	}
	r.bloqueaLista = bloqueaLista
	r.recorrer(g.canvas.Content(), fyne.NewPos(0, 0), fyne.NewPos(tam.Width, tam.Height))
	return r.edita, r.bloqueaLista
}

type recorridoObjetivos struct {
	g                   *gestosMoviles
	d                   fyne.Driver
	pos                 fyne.Position
	left                float32
	edita, bloqueaLista bool
}

func (r *recorridoObjetivos) recorrer(o fyne.CanvasObject, rMin, rMax fyne.Position) {
	if o == nil || !o.Visible() || r.edita {
		return
	}
	switch v := o.(type) {
	case *PilaAvisos:
		r.recorrerAvisos(v, rMin, rMax)
	case *BarraSuite:
		r.recorrerBarraSuite(v, rMin, rMax)
	case *Armazon:
		r.recorrerArmazon(v, rMin, rMax)
	case *Lateral, *listaActualizable, *container.Scroll:
		r.recorrerRecortado(o, rMin, rMax)
	default:
		if hijos, contenedor := hijosDirectosGestos(o); contenedor {
			for _, h := range hijos {
				r.recorrer(h, rMin, rMax)
			}
			return
		}
		r.clasificarHoja(o, rMin, rMax)
	}
}

func (r *recorridoObjetivos) recorrerAvisos(v *PilaAvisos, rMin, rMax fyne.Position) {
	for _, h := range v.objetosAvisos() {
		if a, es := h.(*Aviso); es && puntoEnObjeto(r.d, a, r.pos, rMin, rMax) {
			r.bloqueaLista = true
		}
	}
}

func (r *recorridoObjetivos) recorrerBarraSuite(v *BarraSuite, rMin, rMax fyne.Position) {
	if v.buscador.panel != nil && v.buscador.panel.Visible() && puntoEnObjeto(r.d, v.buscador.panel, r.pos, rMin, rMax) {
		r.bloqueaLista = true
		return
	}
	if !puntoEnObjeto(r.d, v, r.pos, rMin, rMax) {
		return
	}
	if v.buscador.campo != nil && puntoEnObjeto(r.d, v.buscador.campo, r.pos, rMin, rMax) {
		r.edita, r.bloqueaLista = true, true
		return
	}
	r.bloqueaLista = true
}

func (r *recorridoObjetivos) recorrerArmazon(v *Armazon, rMin, rMax fyne.Position) {
	if !puntoEnObjeto(r.d, v, r.pos, rMin, rMax) {
		return
	}
	r.g.anotarObjetivo(v, r.pos.X, r.left)
	nMin, nMax := recortarObjeto(r.d, v, rMin, rMax)
	if v.CajonAbierto {
		r.recorrer(v.Menu, nMin, nMax)
		return
	}
	barra := v.BarraMovil
	if barra == nil && v.barra != nil {
		barra = v.barra
	}
	for _, h := range []fyne.CanvasObject{barra, v.Carril, v.Menu, v.Panel, v.Contenido} {
		r.recorrer(h, nMin, nMax)
	}
}

func (r *recorridoObjetivos) recorrerRecortado(o fyne.CanvasObject, rMin, rMax fyne.Position) {
	if !puntoEnObjeto(r.d, o, r.pos, rMin, rMax) {
		return
	}
	if l, es := o.(*listaActualizable); es && l.disponible() {
		r.g.lista = l
	}
	nMin, nMax := recortarObjeto(r.d, o, rMin, rMax)
	hijos, _ := hijosDirectosGestos(o)
	for _, h := range hijos {
		r.recorrer(h, nMin, nMax)
	}
}

func (r *recorridoObjetivos) clasificarHoja(o fyne.CanvasObject, rMin, rMax fyne.Position) {
	if !puntoEnObjeto(r.d, o, r.pos, rMin, rMax) {
		return
	}
	if esControlDeEscritura(o) {
		r.edita, r.bloqueaLista = true, true
		return
	}
	if bloqueaRefrescoMovil(o) {
		r.bloqueaLista = true
	}
}

func hijosDirectosGestos(o fyne.CanvasObject) ([]fyne.CanvasObject, bool) {
	switch v := o.(type) {
	case *raizGestos:
		return []fyne.CanvasObject{v.content}, true
	case *Armazon:
		if v.CajonAbierto {
			return []fyne.CanvasObject{v.Menu}, true
		}
		return []fyne.CanvasObject{v.Carril, v.Menu, v.Panel, v.Contenido}, true
	case *Lateral:
		return []fyne.CanvasObject{v.Cabecera, v.Cuerpo, v.Pie}, true
	case *listaActualizable:
		return []fyne.CanvasObject{v.content}, true
	case *Pista:
		return []fyne.CanvasObject{v.Hijo}, true
	case *container.Scroll:
		return []fyne.CanvasObject{v.Content}, true
	case *container.ThemeOverride:
		return []fyne.CanvasObject{v.Content}, true
	case *fyne.Container:
		return v.Objects, true
	case conHijos:
		return v.hijosDeCajon(), true
	}
	return nil, false
}

func (g *gestosMoviles) puedeEmpezar(e mobile.FilterEvent) bool {
	if g.bloqueado() || e.Position.X < 0 || e.Position.Y < 0 {
		return false
	}
	if g.canvas != nil {
		if _, editing := g.canvas.Focused().(mobile.Keyboardable); editing {
			return false
		}
	}
	return true
}

func bordeIzquierdoLienzo(c fyne.Canvas) float32 {
	if c == nil {
		return 0
	}
	position, _ := c.InteractiveArea()
	return position.X
}

func (g *gestosMoviles) anotarObjetivo(object fyne.CanvasObject, posX, left float32) {
	if a, ok := object.(*Armazon); ok {
		if posX >= left && posX <= left+bordeMenuMovil && menuPorGesto(a) {
			g.menu = a
		} else if cierreMenuPorGesto(a) {
			g.cierre = a
		}
	}
	if list, ok := object.(*listaActualizable); ok && list.disponible() {
		g.lista = list
	}
}

func menuPorGesto(a *Armazon) bool {
	return a.Visible() && a.Menu != nil && !a.CajonAbierto && !a.DetalleAbierto &&
		(a.Size().Width < Estrecho || a.Carril != nil)
}

func cierreMenuPorGesto(a *Armazon) bool {
	return a.Visible() && a.Menu != nil && a.CajonAbierto &&
		(a.Size().Width < Estrecho || a.Carril != nil)
}

func (g *gestosMoviles) seguir(e mobile.FilterEvent) mobile.FilterResult {
	if e.Phase == mobile.FilterEnd && !g.enLaMismaVista(e.Path) {
		g.cancelar()
		return mobile.FilterClaim
	}
	if g.bloqueado() || (!g.hora.IsZero() && e.Time.Sub(g.hora) > 2*time.Second) {
		return g.terminarSinAccion()
	}
	if g.listaCambiada() {
		g.cancelar()
		return mobile.FilterClaim
	}
	if !g.capturado {
		if result := g.decidir(e); result != mobile.FilterClaim {
			return result
		}
	}
	if e.Phase != mobile.FilterEnd {
		g.aplicarArrastre(e.Position)
		return mobile.FilterClaim
	}
	return g.soltar(e)
}

func (g *gestosMoviles) listaCambiada() bool {
	return g.lista != nil && g.lista.options.Revision != nil && g.lista.options.Revision() != g.revision
}

func (g *gestosMoviles) aplicarArrastre(pos fyne.Position) {
	dx, dy, _, _ := g.distancias(pos)
	switch g.clase {
	case gestoMenu:
		if g.menu != nil && menuPorGesto(g.menu) {
			g.menu.arrastrarAperturaCajon(dx)
		}
	case gestoCerrarMenu:
		if g.cierre != nil && cierreMenuPorGesto(g.cierre) {
			g.cierre.arrastrarCierreCajon(dx)
		}
	case gestoActualizar:
		if g.lista != nil && g.lista.disponible() {
			g.actualizarIndicador(dy)
		} else {
			g.actualizarIndicador(0)
		}
	default:
	}
}

func (g *gestosMoviles) sigueEnVista(path []fyne.CanvasObject, expected fyne.CanvasObject) bool {
	if expected == nil {
		return false
	}
	if enRutaTactil(path, expected) {
		return true
	}
	return g.canvas != nil && objetoVisibleEnArbol(g.canvas.Content(), expected)
}

func objetoVisibleEnArbol(o, buscado fyne.CanvasObject) bool {
	if o == nil || !o.Visible() {
		return false
	}
	if o == buscado {
		return true
	}
	hijos, _ := hijosDirectosGestos(o)
	for _, h := range hijos {
		if objetoVisibleEnArbol(h, buscado) {
			return true
		}
	}
	return false
}

func (g *gestosMoviles) enLaMismaVista(path []fyne.CanvasObject) bool {
	return (g.menu == nil || g.sigueEnVista(path, g.menu)) &&
		(g.cierre == nil || g.sigueEnVista(path, g.cierre)) &&
		(g.lista == nil || g.sigueEnVista(path, g.lista))
}

func (g *gestosMoviles) decidir(e mobile.FilterEvent) mobile.FilterResult {
	dx, dy, x, y := g.distancias(e.Position)
	switch {
	case dx >= decidirGesto && x >= dominioGesto*y && g.menu != nil && menuPorGesto(g.menu):
		g.clase, g.capturado = gestoMenu, true
	case dx <= -decidirGesto && x >= dominioGesto*y && g.cierre != nil && cierreMenuPorGesto(g.cierre):
		g.clase, g.capturado = gestoCerrarMenu, true
	case dy >= decidirGesto && y >= dominioGesto*x && g.lista != nil && g.lista.disponible():
		g.clase, g.capturado = gestoActualizar, true
		g.actualizarIndicador(dy)
	case x >= decidirGesto || y >= decidirGesto || e.Phase == mobile.FilterEnd:
		g.cancelar()
		return mobile.FilterPass
	default:
		return mobile.FilterDefer
	}
	return mobile.FilterClaim
}

func (g *gestosMoviles) distancias(position fyne.Position) (dx, dy, x, y float32) {
	dx, dy = position.X-g.inicio.X, position.Y-g.inicio.Y
	x, y = float32(math.Abs(float64(dx))), float32(math.Abs(float64(dy)))
	return dx, dy, x, y
}

func (g *gestosMoviles) soltar(e mobile.FilterEvent) mobile.FilterResult {
	dx, dy, x, y := g.distancias(e.Position)
	menu, cierre, list, kind := g.menu, g.cierre, g.lista, g.clase
	sigueMenu := g.sigueEnVista(e.Path, menu)
	sigueCierre := g.sigueEnVista(e.Path, cierre)
	sigueLista := g.sigueEnVista(e.Path, list)
	teniaIndicador := g.arrastreY > 0
	g.activo, g.capturado = false, false
	g.menu, g.cierre, g.lista = nil, nil, nil
	g.clase = gestoPendiente
	g.arrastreY = 0
	if teniaIndicador && g.alIndicador != nil {
		g.alIndicador()
	}
	switch kind {
	case gestoMenu:
		soltarAperturaMenu(menu, dx, x, y, sigueMenu)
	case gestoCerrarMenu:
		soltarCierreMenu(cierre, dx, x, y, sigueCierre)
	case gestoActualizar:
		if dy >= actualizarListaMovil && y >= x && sigueLista && list.disponible() {
			list.options.Actualizar()
		}
	default:
	}
	return mobile.FilterClaim
}

func soltarAperturaMenu(menu *Armazon, dx, x, y float32, enVista bool) {
	if menu == nil {
		return
	}
	if dx < abrirMenuMovil || x < dominioGesto*y || !enVista || !menuPorGesto(menu) {
		menu.cancelarArrastreCajon()
		return
	}
	// Use the existing button action so app-owned drawer state stays in sync.
	bar, ok := menu.barraMovil().(*BarraMovil)
	if !ok || bar.AlMenu == nil {
		menu.AbrirCajon()
		return
	}
	bar.AlMenu()
	if !menu.arrastrandoCajon {
		return
	}
	if !menu.CajonAbierto {
		menu.cancelarArrastreCajon()
		return
	}
	desde := menu.entrada
	menu.arrastrandoCajon = false
	if menu.AlCajon != nil {
		menu.AlCajon(true)
	}
	menu.animarHaciaAbierto(desde)
}

func soltarCierreMenu(cierre *Armazon, dx, x, y float32, enVista bool) {
	if cierre == nil {
		return
	}
	if -dx >= cerrarMenuMovil && x >= dominioGesto*y && enVista && cierreMenuPorGesto(cierre) {
		cierre.cerrarDesdeMando()
		return
	}
	cierre.cancelarArrastreCajon()
}

func enRutaTactil(path []fyne.CanvasObject, expected fyne.CanvasObject) bool {
	for _, object := range path {
		if object == expected {
			return true
		}
	}
	return false
}

func (g *gestosMoviles) terminarSinAccion() mobile.FilterResult {
	claimed := g.capturado
	g.cancelar()
	if claimed {
		return mobile.FilterClaim
	}
	return mobile.FilterPass
}

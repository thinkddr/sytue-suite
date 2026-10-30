// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Elegir, mover y redimensionar cajas
//
// Es la parte del escenario de SlidesCanvas.tsx que no pinta contenido: la selección
// (una caja primaria, las que se añaden con Mayús y sus grupos), el marco discontinuo
// de cada una, los dos tiradores de la primaria —la barra de mover arriba y el cuadro
// de redimensionar en la esquina—, el distintivo del paso de animación y el teclado
// del escenario (Supr, Escape, flechas y Mayús + flechas).
//
// La estructura copia la del DOM y no por gusto: en la web cada caja es un elemento con
// sus eventos y el fondo del escenario es otro. Aquí cada caja lleva un `marcoCaja`
// (se pulsa y, si no es de texto, se arrastra) y el fondo es la capa, que solo se
// pulsa. Así, en un teléfono, un arrastre que empieza en el fondo sigue siendo del
// desplazador del área, como en la web, y no de la selección.
//
// Todo va dentro del escenario escalado, así que las medidas de la web (en px del
// escenario, con el rem de 14) se multiplican por la escala `k`, igual que hace el
// `transform: scale(k)` del navegador con los tiradores.
var (
	// La barra de mover: `-top-3 h-2.5 w-10 rounded-full`, centrada.
	arribaAsaMover  = tema.Espacio(3)
	altoAsaMover    = tema.Espacio(2.5)
	anchoAsaMover   = tema.Espacio(10)
	margenAsaRedim  = tema.Espacio(1.5) // `-right-1.5 -bottom-1.5`
	ladoAsaRedim    = tema.Espacio(3)   // `h-3 w-3 rounded-sm`
	toqueAsa        = float32(16)       // el `::after { inset: -16px }` de `pointer: coarse`
	fueraDistintivo = tema.Espacio(2)   // `-top-2 -left-2`
	ladoDistintivo  = tema.Espacio(4)   // `h-4 min-w-4`
)

// ---- la selección, en el editor ----

// cajasEdit son las cajas que edita el lienzo: las de la diapositiva activa o, con
// «Editar patrón», las del patrón.
func (e *Editor) cajasEdit() []Caja { return e.m.cajas(e.objetivo()) }

// explicita es la selección tal cual: la primaria y las añadidas con Mayús.
func (e *Editor) explicita() []string {
	if e.sel == "" {
		return nil
	}
	return append([]string{e.sel}, e.extra...)
}

// efectiva es `effSelIds`: la explícita con los grupos expandidos.
func (e *Editor) efectiva() []string { return ExpandirGrupos(e.cajasEdit(), e.explicita()) }

func (e *Editor) elegida(id string) bool { return e.sel == id || slices.Contains(e.extra, id) }

// Primaria es la caja seleccionada que lleva los tiradores, o nil.
func (e *Editor) Primaria() *Caja {
	if e.sel == "" {
		return nil
	}
	return e.m.caja(e.objetivo(), e.sel)
}

// SeleccionarSolo es `selectOnly`: una caja (o ninguna, con "") y sin añadidas. Suelta
// el conector elegido: o cajas o un conector, nunca las dos cosas.
func (e *Editor) SeleccionarSolo(id string) {
	if e.escribiendo != nil && e.escribiendo.id != id {
		e.TerminarEscritura()
	}
	if e.sel == id && len(e.extra) == 0 && e.selCon == "" {
		return
	}
	e.sel, e.extra, e.selCon = id, nil, ""
	e.recomponer()
	e.refrescarCinta()
}

// SeleccionarConector es `onConnSelect`: el conector, sin cajas elegidas.
func (e *Editor) SeleccionarConector(id string) {
	e.TerminarEscritura()
	if e.selCon == id && e.sel == "" {
		return
	}
	e.sel, e.extra, e.selCon = "", nil, id
	e.recomponer()
	e.refrescarCinta()
}

// ConectorElegido es el conector elegido, o "".
func (e *Editor) ConectorElegido() string { return e.selCon }

// conectorEn es el conector que hay bajo `p` (px del escenario, sin escalar): el de más
// arriba cuyo trazo pasa a menos de la holgura. En el patrón no hay conectores.
func (e *Editor) conectorEn(p punto) string {
	if e.patron || !e.m.valida(e.activa) {
		return ""
	}
	d := e.m.Diapos[e.activa]
	rs := rutasConectores(d.Conectores, d.Cajas, float64(e.m.AltoEscenario()))
	for i := len(rs) - 1; i >= 0; i-- {
		if distancia(p, rs[i].r.puntos) <= holguraConector {
			return rs[i].c.ID
		}
	}
	return ""
}

// Alternar es `toggleSel` (Mayús + clic): la primaria no se quita, y sin primaria la
// caja pasa a serlo.
func (e *Editor) Alternar(id string) {
	switch {
	case e.sel == "":
		e.SeleccionarSolo(id)
		return
	case id == e.sel:
		return
	case slices.Contains(e.extra, id):
		e.extra = slices.DeleteFunc(slices.Clone(e.extra), func(x string) bool { return x == id })
	default:
		e.extra = append(slices.Clone(e.extra), id)
	}
	e.recomponer()
	e.refrescarCinta()
}

// BorrarSeleccion es Supr sobre el escenario: borra la selección efectiva con sus
// conectores y deja el escenario sin nada elegido.
func (e *Editor) BorrarSeleccion() {
	ids := e.efectiva()
	if len(ids) == 0 {
		return
	}
	e.cambiar(func() {
		e.m.QuitarCajas(e.objetivo(), ids)
		e.sel, e.extra = "", nil
	}, true)
}

// Empujar es una flecha sobre el escenario: mueve la selección un 1 % como bloque; con
// Mayús, redimensiona la primaria.
func (e *Editor) Empujar(dx, dy float64, redimensionar bool) {
	p := e.Primaria()
	if p == nil {
		return
	}
	if redimensionar {
		id, w, h := p.ID, p.W+dx, p.H+dy
		e.cambiar(func() { e.m.Redimensionar(e.objetivo(), id, w, h) }, true)
		return
	}
	ids := e.efectiva()
	// Topado al lienzo: en el borde no hay nada que mover ni que guardar.
	if !e.m.cabeDesplazar(e.objetivo(), ids, dx, dy) {
		return
	}
	e.cambiar(func() { e.m.Desplazar(e.objetivo(), ids, dx, dy) }, true)
}

// Organizar es `organize`: la operación de «Organizar» sobre la selección efectiva.
func (e *Editor) Organizar(op string) {
	ids := e.efectiva()
	if len(ids) == 0 {
		return
	}
	e.cambiar(func() { e.m.Organizar(e.objetivo(), ids, op, uuid()) }, true)
}

// ---- la capa ----

// capaEdicion es el fondo del escenario con todo lo que se pinta ENCIMA de las cajas
// al editar. Se pulsa (deselecciona) y se enfoca (el teclado del escenario); lo que se
// arrastra son sus hijos.
type capaEdicion struct {
	widget.BaseWidget
	e        *Editor
	enfocada bool
	mayus    bool
	// arrastre es el gesto en curso de un marco o de un tirador; nil si no hay.
	arrastre *arrastreCaja
	// arrastreCon es el de un tirador de conector, y arrastreMov el del final de una
	// trayectoria.
	arrastreCon *arrastreConector
	arrastreMov *arrastreMovimiento
}

var (
	_ fyne.Widget       = (*capaEdicion)(nil)
	_ fyne.Tappable     = (*capaEdicion)(nil)
	_ fyne.Focusable    = (*capaEdicion)(nil)
	_ desktop.Keyable   = (*capaEdicion)(nil)
	_ desktop.Mouseable = (*capaEdicion)(nil)
)

func nuevaCapaEdicion(e *Editor) *capaEdicion {
	c := &capaEdicion{e: e}
	c.ExtendBaseWidget(c)
	return c
}

// pulsarConector elige el conector bajo una pulsación en `pos` (px de la capa); dice si
// había uno. El trazo del conector va ENCIMA de las cajas y se lleva la pulsación.
func (c *capaEdicion) pulsarConector(pos fyne.Position) bool {
	k := c.e.escenario.Escala
	if k <= 0 {
		k = 1
	}
	id := c.e.conectorEn(punto{float64(pos.X / k), float64(pos.Y / k)})
	if id == "" {
		return false
	}
	c.enfocar() // que Supr y Escape lleguen al escenario
	c.e.SeleccionarConector(id)
	return true
}

// Tapped en el fondo deselecciona («clic en el fondo deselecciona»), salvo que caiga en
// un conector, que se elige.
func (c *capaEdicion) Tapped(ev *fyne.PointEvent) {
	if c.pulsarConector(ev.Position) {
		return
	}
	c.enfocar()
	c.e.SeleccionarSolo("")
}

// MouseDown y MouseUp solo toman nota de Mayús: el toque no la trae.
func (c *capaEdicion) MouseDown(ev *desktop.MouseEvent) {
	c.mayus = ev.Modifier&fyne.KeyModifierShift != 0
}

// MouseUp no hace nada: la pulsación la atiende Tapped.
func (c *capaEdicion) MouseUp(*desktop.MouseEvent) {}

func (c *capaEdicion) enfocar() {
	if lienzo := fyne.CurrentApp().Driver().CanvasForObject(c); lienzo != nil {
		lienzo.Focus(c)
	}
}

// FocusGained enseña el anillo del escenario (`sytue-focus-ring`).
func (c *capaEdicion) FocusGained() { c.enfocada = true; c.Refresh() }

// FocusLost lo quita.
func (c *capaEdicion) FocusLost() { c.enfocada = false; c.Refresh() }

// TypedRune no escribe nada: en el escenario no hay texto en edición.
func (c *capaEdicion) TypedRune(rune) {}

// KeyDown y KeyUp llevan la cuenta de Mayús, que TypedKey no dice.
func (c *capaEdicion) KeyDown(ev *fyne.KeyEvent) {
	if ev.Name == desktop.KeyShiftLeft || ev.Name == desktop.KeyShiftRight {
		c.mayus = true
	}
}

// KeyUp suelta Mayús.
func (c *capaEdicion) KeyUp(ev *fyne.KeyEvent) {
	if ev.Name == desktop.KeyShiftLeft || ev.Name == desktop.KeyShiftRight {
		c.mayus = false
	}
}

// TypedKey es `onStageKeyDown` para el escenario: Supr/Retroceso borran la selección,
// Escape la quita y las flechas mueven (o, con Mayús, redimensionan la primaria).
func (c *capaEdicion) TypedKey(ev *fyne.KeyEvent) {
	e := c.e
	if e.selCon != "" {
		// Conector elegido: Supr lo borra y Escape lo suelta.
		switch ev.Name {
		case fyne.KeyDelete, fyne.KeyBackspace:
			e.QuitarConector()
		case fyne.KeyEscape:
			e.SeleccionarSolo("")
		default:
		}
		return
	}
	if e.sel == "" {
		return
	}
	var dx, dy float64
	switch ev.Name {
	case fyne.KeyDelete, fyne.KeyBackspace:
		e.BorrarSeleccion()
		return
	case fyne.KeyEscape:
		e.SeleccionarSolo("")
		return
	case fyne.KeyReturn, fyne.KeyEnter, fyne.KeyF2:
		e.EmpezarEscritura(e.sel) // solo texto: imagen y forma no tienen modo edición
		return
	case fyne.KeyLeft:
		dx = -1
	case fyne.KeyRight:
		dx = 1
	case fyne.KeyUp:
		dy = -1
	case fyne.KeyDown:
		dy = 1
	default:
		return
	}
	e.Empujar(dx, dy, c.mayus)
}

// AccessibilityLabel es el `aria-label` del escenario: «Diapositiva n de total».
func (c *capaEdicion) AccessibilityLabel() string { return c.e.escenario.AccessibilityLabel() }

// AccessibilityRole es el `role="group"` del escenario.
func (c *capaEdicion) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

func (c *capaEdicion) CreateRenderer() fyne.WidgetRenderer {
	r := &capaRenderer{c: c, aro: ui.NuevoAro(0, ui.SinHueco, c.e.variante()).Abierto(), aroCaja: ui.NuevoAro(0, ui.SinHueco, c.e.variante()).Abierto()}
	r.rehacer()
	return r
}

type capaRenderer struct {
	c       *capaEdicion
	aro     *ui.Aro
	aroCaja *ui.Aro
	objetos []fyne.CanvasObject
}

func (r *capaRenderer) MinSize() fyne.Size           { return fyne.NewSize(1, 1) }
func (r *capaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *capaRenderer) Destroy()                     {}
func (r *capaRenderer) Layout(fyne.Size)             { r.rehacer() }

func (r *capaRenderer) Refresh() {
	r.rehacer()
	canvas.Refresh(r.c)
}

// rehacer compone la capa: el anillo del escenario, un marco por caja (en su orden en
// Z), los marcos discontinuos de la selección, los distintivos y los tiradores.
func (r *capaRenderer) rehacer() {
	c, e := r.c, r.c.e
	v := e.variante()
	k := e.escenario.Escala
	if k <= 0 {
		k = 1
	}
	r.aro.Pintar(v)
	r.aro.Colocar(c.Size())
	r.aro.Ver(c.enfocada)
	objs := append([]fyne.CanvasObject{}, r.aro.Objetos()...)
	acento := tema.Color("primary", v)
	sh := e.m.AltoEscenario()
	rectPx := func(cj Caja) (fyne.Position, fyne.Size) {
		return fyne.NewPos(float32(cj.X)/100*AnchoEscenario*k, float32(cj.Y)/100*sh*k),
			fyne.NewSize(float32(cj.W)/100*AnchoEscenario*k, float32(cj.H)/100*sh*k)
	}
	efectiva := conjunto(e.efectiva())
	var pasos map[string][]Planeada
	if !e.patron && e.activa >= 0 && e.activa < len(e.m.Diapos) {
		pasos = PorCaja(Plan(e.m.Diapos[e.activa]))
	}
	var encima []fyne.CanvasObject
	for _, cj := range e.cajasEdit() {
		pos, tam := rectPx(cj)
		m := nuevoMarcoCaja(c, cj.ID, esTexto(cj))
		m.Move(pos)
		m.Resize(tam)
		objs = append(objs, m)
		dentro := efectiva[cj.ID]
		if dentro && e.Escribiendo() != cj.ID {
			encima = append(encima, marcoDiscontinuo(pos, tam, k, acento))
		}
		if p := pasos[cj.ID]; len(p) > 0 && (dentro || e.abajo == abajoAnim) {
			encima = append(encima, distintivo(strconv.Itoa(p[0].Paso), pos, k, acento)...)
		}
	}
	objs = append(objs, encima...)
	if w := e.escribiendo; w != nil {
		if c := e.m.caja(e.objetivo(), w.id); c != nil {
			pos, tam := rectPx(*c)
			// El `sytue-focus-ring` del `contenteditable`: la caja en edición lleva el
			// anillo de foco, no el marco discontinuo.
			r.aroCaja.Pintar(v)
			r.aroCaja.ColocarEn(pos, tam)
			r.aroCaja.Ver(true)
			objs = append(objs, r.aroCaja.Objetos()...)
			objs = append(objs, w.colocar(e, pos, tam, k, e.m.Visuales(e.activa).Tinta))
		}
	}
	if p := e.Primaria(); p != nil && !e.ctx.SoloLectura && e.escribiendo == nil {
		pos, tam := rectPx(*p)
		mover := nuevaAsa(c, p.ID, false, acento, anchoAsaMover*k)
		mover.Move(pos.AddXY(tam.Width/2-anchoAsaMover*k/2, -arribaAsaMover*k))
		mover.Resize(fyne.NewSize(anchoAsaMover*k, altoAsaMover*k))
		redim := nuevaAsa(c, p.ID, true, acento, tema.RadioSM*k)
		redim.Move(pos.AddXY(tam.Width+margenAsaRedim*k-ladoAsaRedim*k, tam.Height+margenAsaRedim*k-ladoAsaRedim*k))
		redim.Resize(fyne.NewSize(ladoAsaRedim*k, ladoAsaRedim*k))
		// En un dispositivo táctil los tiradores reciben el toque 16 px alrededor: 10 px
		// no los acierta nadie con el dedo.
		if ui.EsTactil() {
			for _, a := range []*asa{mover, redim} {
				a.toque = toqueAsa * k
				a.Move(a.Position().SubtractXY(a.toque, a.toque))
				a.Resize(a.Size().AddWidthHeight(2*a.toque, 2*a.toque))
			}
		}
		objs = append(objs, mover, redim)
	}
	objs = append(objs, r.asasConector(k, acento)...)
	objs = append(objs, r.trayectorias(k, acento, efectiva)...)
	r.objetos = objs
}

// trayectorias son las de las cajas ELEGIDAS (no satura el lienzo): la polilínea
// discontinua del acento desde el centro de la caja, el círculo hueco del inicio y el
// relleno del final, que se arrastra para reorientarla. No en el patrón.
func (r *capaRenderer) trayectorias(k float32, acento color.NRGBA, elegidas map[string]bool) []fyne.CanvasObject {
	e := r.c.e
	if e.patron {
		return nil
	}
	sw, sh := float64(AnchoEscenario), float64(e.m.AltoEscenario())
	mulX, mulY := sw/100, sh/100
	var svg strings.Builder
	var asas []fyne.CanvasObject
	for _, cj := range e.cajasEdit() {
		if !elegidas[cj.ID] {
			continue
		}
		for i, a := range AnimsDe(cj) {
			if categoriaAnim[a.Tipo] != Movimiento {
				continue
			}
			pts := PuntosMovimiento(a.Tipo, a.Puntos)
			cx, cy := (cj.X+cj.W/2)*mulX, (cj.Y+cj.H/2)*mulY
			var d strings.Builder
			for j, p := range pts {
				letra := "L"
				if j == 0 {
					letra = "M"
				}
				fmt.Fprintf(&d, "%s %s %s ", letra, n2(cx+p.DX*mulX), n2(cy+p.DY*mulY))
			}
			fmt.Fprintf(&svg, `<path d="%s" fill="none" stroke="%s" stroke-width="2" stroke-dasharray="5 4"/>`+
				`<circle cx="%s" cy="%s" r="5" fill="#FFFFFF" stroke="%s" stroke-width="2"/>`,
				strings.TrimSpace(d.String()), hexDe(acento), n2(cx), n2(cy), hexDe(acento))
			fin := pts[len(pts)-1]
			if (fin.DX != 0 || fin.DY != 0) && !e.ctx.SoloLectura { // el circuito cierra en el inicio
				id, indice := cj.ID, i
				asa := nuevaAsaRedonda(acento, acento, 0,
					func(d fyne.Delta) { r.c.moverFinTrayectoria(id, indice, d) }, r.c.soltarRedonda)
				asa.colocarEn(fyne.NewPos(float32(cx+fin.DX*mulX)*k, float32(cy+fin.DY*mulY)*k), radioAsaMovimiento*k, k)
				asas = append(asas, asa)
			}
		}
	}
	if svg.Len() == 0 {
		return nil
	}
	var b strings.Builder
	cabeceraSVG(&b, sw, sh)
	b.WriteString(svg.String())
	b.WriteString(`</svg>`)
	capa := colocado(iconos.DeSVG([]byte(b.String()), fyne.NewSize(float32(sw)*k, float32(sh)*k)), fyne.NewPos(0, 0))
	return append([]fyne.CanvasObject{capa}, asas...)
}

// asasConector son los tiradores de los extremos LIBRES del conector elegido (los
// anclados siguen a su caja): un círculo de radio 6, blanco con el borde del acento.
func (r *capaRenderer) asasConector(k float32, acento color.NRGBA) []fyne.CanvasObject {
	e := r.c.e
	if e.selCon == "" || e.ctx.SoloLectura || !e.m.valida(e.activa) {
		return nil
	}
	d := e.m.Diapos[e.activa]
	var objs []fyne.CanvasObject
	for _, rd := range rutasConectores(d.Conectores, d.Cajas, float64(e.m.AltoEscenario())) {
		if rd.c.ID != e.selCon {
			continue
		}
		for _, ext := range []struct {
			final bool
			libre bool
			en    remate
		}{{false, rd.c.Desde.Caja == "", rd.r.inicio}, {true, rd.c.Hasta.Caja == "", rd.r.final}} {
			if !ext.libre {
				continue
			}
			id, final := rd.c.ID, ext.final
			a := nuevaAsaRedonda(color.NRGBA{R: 255, G: 255, B: 255, A: 255}, acento, 2*k,
				func(d fyne.Delta) { r.c.moverExtremo(id, final, d) }, r.c.soltarRedonda)
			a.colocarEn(fyne.NewPos(float32(ext.en.x)*k, float32(ext.en.y)*k), radioAsaConector*k, k)
			objs = append(objs, a)
		}
	}
	return objs
}

func esTexto(c Caja) bool { return c.Tipo == "" || c.Tipo == "text" }

// marcoDiscontinuo es el `outline: 1px dashed ACCENT` de una caja elegida: va POR
// FUERA de la caja, de 1 px del escenario.
//
// ponytail: el patrón es 3 y 3, el de un borde discontinuo fino en Chromium; no se ha
// medido trazo a trazo contra una captura.
func marcoDiscontinuo(pos fyne.Position, tam fyne.Size, k float32, c color.NRGBA) fyne.CanvasObject {
	w, h := float64(tam.Width/k)+2, float64(tam.Height/k)+2
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" width="%s" height="%s">`+
		`<rect x="0.5" y="0.5" width="%s" height="%s" fill="none" stroke="%s" stroke-width="1"`+
		` stroke-dasharray="3 3"/></svg>`, n2(w), n2(h), n2(w), n2(h), n2(w-1), n2(h-1), hexDe(c))
	d := iconos.DeSVG([]byte(svg), fyne.NewSize(float32(w)*k, float32(h)*k))
	d.Objeto().Move(pos.SubtractXY(k, k))
	return d.Objeto()
}

// distintivo es el número del paso en que arranca la caja: `text-2xs font-semibold
// text-white rounded-full` sobre el acento, en `-top-2 -left-2`.
func distintivo(paso string, pos fyne.Position, k float32, acento color.NRGBA) []fyne.CanvasObject {
	e := tema.Escalon{Tam: tema.Texto2XS.Tam * k, Linea: ladoDistintivo * k}
	ancho := max(ladoDistintivo*k, ui.Medir(paso, e, ui.SemiNegrita).Width+2*tema.Espacio(1)*k)
	fondo := canvas.NewRectangle(acento)
	fondo.CornerRadius = ladoDistintivo * k / 2
	fondo.Move(pos.SubtractXY(fueraDistintivo*k, fueraDistintivo*k))
	fondo.Resize(fyne.NewSize(ancho, ladoDistintivo*k))
	t := ui.Texto(paso, e, ui.SemiNegrita, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	ui.ColocarTexto(t, e, fondo.Position().X+(ancho-ui.Medir(paso, e, ui.SemiNegrita).Width)/2, fondo.Position().Y)
	return []fyne.CanvasObject{fondo, t}
}

// ---- el arrastre ----

// arrastreCaja es `dragRef`: qué se mueve, desde dónde y con qué topes.
type arrastreCaja struct {
	redimensionar bool
	caja          string
	dx, dy        float32 // lo arrastrado, en px de pantalla
	origs         []Caja
	min, max      [2]float64
}

// empezar es `onHandleDown`. Redimensionar afecta solo a la caja; mover, a la
// selección efectiva si la caja está en ella (o a su grupo), topada por su envolvente.
func (c *capaEdicion) empezar(id string, redimensionar bool) {
	e := c.e
	a := &arrastreCaja{redimensionar: redimensionar, caja: id}
	cs := e.cajasEdit()
	if redimensionar {
		if p := e.m.caja(e.objetivo(), id); p != nil {
			a.origs = []Caja{*p}
		}
		c.arrastre = a
		return
	}
	base := []string{id}
	if e.elegida(id) {
		base = e.explicita()
	}
	mueven := conjunto(append(ExpandirGrupos(cs, base), id))
	var rs []rect
	for _, cj := range cs {
		if mueven[cj.ID] {
			a.origs = append(a.origs, cj)
			rs = append(rs, rectDe(cj))
		}
	}
	env := envolvente(rs)
	a.min = [2]float64{-env.x, -env.y}
	a.max = [2]float64{100 - (env.x + env.w), 100 - (env.y + env.h)}
	c.arrastre = a
}

// seguir es `onHandleMove`: un solo delta topado para todo el conjunto, que conserva
// así sus posiciones relativas. Se repinta el escenario y la capa, no el carril: la
// miniatura se pone al día al soltar.
func (c *capaEdicion) seguir(d fyne.Delta) {
	a, e := c.arrastre, c.e
	if a == nil || len(a.origs) == 0 {
		return
	}
	a.dx, a.dy = a.dx+d.DX, a.dy+d.DY
	k := e.escenario.Escala
	dx := float64(a.dx / (AnchoEscenario * k) * 100)
	dy := float64(a.dy / (e.m.AltoEscenario() * k) * 100)
	if a.redimensionar {
		o := a.origs[0]
		e.m.Redimensionar(e.objetivo(), o.ID, o.W+dx, o.H+dy)
	} else {
		dx, dy = min(max(dx, a.min[0]), a.max[0]), min(max(dy, a.min[1]), a.max[1])
		for _, o := range a.origs {
			if p := e.m.caja(e.objetivo(), o.ID); p != nil {
				p.X, p.Y = o.X+dx, o.Y+dy
			}
		}
	}
	e.escenario.Refresh()
	c.Refresh()
}

// soltar es `onHandleUp`: un único cambio al soltar, que es el que marca el documento
// y programa el guardado.
func (c *capaEdicion) soltar() {
	if c.arrastre == nil {
		return
	}
	c.arrastre = nil
	c.e.cambiar(func() {}, true)
}

// ---- los hijos de la capa ----

// marcoCaja es el elemento de UNA caja: se pulsa para elegirla y, si no es de texto,
// se arrastra para moverla. El texto se mueve por su barra: su cuerpo es para escribir.
type marcoCaja struct {
	widget.BaseWidget
	capa  *capaEdicion
	id    string
	texto bool
}

var (
	_ fyne.Tappable       = (*marcoCaja)(nil)
	_ fyne.DoubleTappable = (*marcoCaja)(nil)
	_ fyne.Draggable      = (*marcoCaja)(nil)
	_ desktop.Mouseable   = (*marcoCaja)(nil)
)

// DoubleTapped es `onDblClick`: en una caja de texto, a escribir.
func (m *marcoCaja) DoubleTapped(*fyne.PointEvent) {
	if m.texto {
		m.capa.e.EmpezarEscritura(m.id)
	}
}

func nuevoMarcoCaja(c *capaEdicion, id string, texto bool) *marcoCaja {
	m := &marcoCaja{capa: c, id: id, texto: texto}
	m.ExtendBaseWidget(m)
	return m
}

// MouseDown toma nota de Mayús, que es de la pulsación y no del teclado: pulsar la caja
// quita el foco del escenario antes del Tapped, y con él cualquier KeyUp.
func (m *marcoCaja) MouseDown(ev *desktop.MouseEvent) {
	m.capa.mayus = ev.Modifier&fyne.KeyModifierShift != 0
}

// MouseUp no hace nada: la pulsación la atiende Tapped.
func (m *marcoCaja) MouseUp(*desktop.MouseEvent) {}

// Tapped elige la caja; con Mayús la añade o la quita de la selección. Un conector que
// pase por encima se lleva la pulsación.
func (m *marcoCaja) Tapped(ev *fyne.PointEvent) {
	if !m.capa.mayus && m.capa.pulsarConector(m.Position().Add(ev.Position)) {
		return
	}
	m.capa.enfocar()
	if m.capa.mayus {
		m.capa.e.Alternar(m.id)
		return
	}
	m.capa.e.SeleccionarSolo(m.id)
}

// Dragged mueve una caja que no es de texto; la elige primero si no lo estaba.
func (m *marcoCaja) Dragged(ev *fyne.DragEvent) {
	c := m.capa
	if m.texto || c.e.ctx.SoloLectura {
		if ui.EsTactil() {
			c.e.desliz.Scrolled(&fyne.ScrollEvent{PointEvent: ev.PointEvent, Scrolled: ev.Dragged})
		}
		return
	}
	if c.arrastre == nil {
		if !c.e.elegida(m.id) {
			c.e.SeleccionarSolo(m.id)
		}
		c.enfocar()
		c.empezar(m.id, false)
	}
	c.seguir(ev.Dragged)
}

// DragEnd suelta.
func (m *marcoCaja) DragEnd() { m.capa.soltar() }

// AccessibilityLabel es el `aria-label` de una caja que no es de texto (`boxLabel`).
func (m *marcoCaja) AccessibilityLabel() string {
	if p := m.capa.e.m.caja(m.capa.e.objetivo(), m.id); p != nil && !m.texto {
		return m.capa.e.t(claveDe(*p))
	}
	return ""
}

func (m *marcoCaja) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.NRGBA{}))
}

// asa es un tirador: la barra de mover o el cuadro de redimensionar.
type asa struct {
	widget.BaseWidget
	capa          *capaEdicion
	id            string
	redimensionar bool
	color         color.NRGBA
	radio         float32
	// toque es el margen de más que recibe los toques alrededor de lo pintado.
	toque float32
}

var _ fyne.Draggable = (*asa)(nil)

func nuevaAsa(c *capaEdicion, id string, redimensionar bool, col color.NRGBA, radio float32) *asa {
	a := &asa{capa: c, id: id, redimensionar: redimensionar, color: col, radio: radio}
	a.ExtendBaseWidget(a)
	return a
}

// Dragged mueve o redimensiona.
func (a *asa) Dragged(ev *fyne.DragEvent) {
	if a.capa.arrastre == nil {
		a.capa.empezar(a.id, a.redimensionar)
	}
	a.capa.seguir(ev.Dragged)
}

// DragEnd suelta.
func (a *asa) DragEnd() { a.capa.soltar() }

// Cursor es `cursor-move` en la barra.
func (a *asa) Cursor() desktop.Cursor {
	if a.redimensionar {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}

// AccessibilityLabel es el `title` del tirador.
func (a *asa) AccessibilityLabel() string {
	if a.redimensionar {
		return a.capa.e.t("slides.box.resize")
	}
	return a.capa.e.t("slides.box.move")
}

func (a *asa) CreateRenderer() fyne.WidgetRenderer {
	pintado := canvas.NewRectangle(a.color)
	pintado.CornerRadius = a.radio
	return &asaRenderer{a: a, pintado: pintado}
}

// asaRenderer pinta el tirador a su tamaño y, en un dispositivo táctil, recibe los
// toques 16 px alrededor (el `::after` de `pointer: coarse`).
type asaRenderer struct {
	a       *asa
	pintado *canvas.Rectangle
}

func (r *asaRenderer) MinSize() fyne.Size           { return fyne.NewSize(1, 1) }
func (r *asaRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.pintado} }
func (r *asaRenderer) Destroy()                     {}
func (r *asaRenderer) Refresh()                     { r.pintado.Refresh() }
func (r *asaRenderer) Layout(t fyne.Size) {
	m := r.a.toque
	r.pintado.Move(fyne.NewPos(m, m))
	r.pintado.Resize(fyne.NewSize(t.Width-2*m, t.Height-2*m))
}

// ---- tiradores redondos: extremos de conector y final de trayectoria ----

// radioAsaConector es el `r={6}` del círculo de un extremo; radioAsaMovimiento, el
// `r={7}` del final de una trayectoria; y radioInicioMovimiento el `r={5}` de su inicio.
const (
	radioAsaConector      float32 = 6
	radioAsaMovimiento    float32 = 7
	radioInicioMovimiento float32 = 5
)

// asaRedonda es un tirador circular que se arrastra: el extremo libre de un conector o
// el final de una trayectoria. Lo que hace al moverse lo dice quien lo crea.
type asaRedonda struct {
	widget.BaseWidget
	relleno, borde color.NRGBA
	grosor         float32
	toque          float32
	mover          func(fyne.Delta)
	alSoltar       func()
}

var _ fyne.Draggable = (*asaRedonda)(nil)

func nuevaAsaRedonda(relleno, borde color.NRGBA, grosor float32, mover func(fyne.Delta), alSoltar func()) *asaRedonda {
	a := &asaRedonda{relleno: relleno, borde: borde, grosor: grosor, mover: mover, alSoltar: alSoltar}
	a.ExtendBaseWidget(a)
	return a
}

// colocarEn pone el círculo de radio `radio` centrado en `centro`, con la zona de toque
// agrandada en un dispositivo táctil.
func (a *asaRedonda) colocarEn(centro fyne.Position, radio, k float32) {
	if ui.EsTactil() {
		a.toque = toqueAsa * k
	}
	m := radio + a.toque
	a.Move(centro.SubtractXY(m, m))
	a.Resize(fyne.NewSize(2*m, 2*m))
}

// Dragged mueve.
func (a *asaRedonda) Dragged(ev *fyne.DragEvent) { a.mover(ev.Dragged) }

// DragEnd suelta: el único cambio que marca el documento.
func (a *asaRedonda) DragEnd() { a.alSoltar() }

// Cursor es `cursor: move`.
func (a *asaRedonda) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (a *asaRedonda) CreateRenderer() fyne.WidgetRenderer {
	c := canvas.NewCircle(a.relleno)
	c.StrokeColor, c.StrokeWidth = a.borde, a.grosor
	return &asaRedondaRenderer{a: a, circulo: c}
}

type asaRedondaRenderer struct {
	a       *asaRedonda
	circulo *canvas.Circle
}

func (r *asaRedondaRenderer) MinSize() fyne.Size           { return fyne.NewSize(1, 1) }
func (r *asaRedondaRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.circulo} }
func (r *asaRedondaRenderer) Destroy()                     {}
func (r *asaRedondaRenderer) Refresh()                     { r.circulo.Refresh() }
func (r *asaRedondaRenderer) Layout(t fyne.Size) {
	m := r.a.toque
	r.circulo.Move(fyne.NewPos(m, m))
	r.circulo.Resize(fyne.NewSize(t.Width-2*m, t.Height-2*m))
}

// arrastreConector es el gesto de un tirador de extremo: desde dónde (en %) y cuánto.
type arrastreConector struct {
	id     string
	final  bool
	x0, y0 float64
	dx, dy float32
}

// moverExtremo es `onConnHandleMove`: el extremo sigue al puntero sin marcar el
// documento.
func (c *capaEdicion) moverExtremo(id string, final bool, d fyne.Delta) {
	e := c.e
	if c.arrastreCon == nil {
		con := e.m.conector(e.activa, id)
		if con == nil {
			return
		}
		ext := con.Desde
		if final {
			ext = con.Hasta
		}
		g := &arrastreConector{id: id, final: final}
		if ext.X != nil && ext.Y != nil {
			g.x0, g.y0 = *ext.X, *ext.Y
		}
		c.arrastreCon = g
	}
	g := c.arrastreCon
	g.dx, g.dy = g.dx+d.DX, g.dy+d.DY
	k := c.escala()
	e.m.MoverExtremo(e.activa, g.id, g.final,
		g.x0+float64(g.dx/(AnchoEscenario*k)*100), g.y0+float64(g.dy/(e.m.AltoEscenario()*k)*100))
	e.escenario.Refresh()
	c.Refresh()
}

// arrastreMovimiento es el del final de una trayectoria: sus vértices al empezar.
type arrastreMovimiento struct {
	caja   string
	indice int
	base   []Punto
	dx, dy float32
}

// moverFinTrayectoria es `onMotionEndMove`: el último vértice sigue al puntero, topado
// a ±100 %, y la trayectoria pasa a ser personalizada.
func (c *capaEdicion) moverFinTrayectoria(caja string, indice int, d fyne.Delta) {
	e := c.e
	if c.arrastreMov == nil {
		cj := e.m.caja(e.objetivo(), caja)
		if cj == nil {
			return
		}
		anims := AnimsDe(*cj)
		if indice >= len(anims) {
			return
		}
		a := anims[indice]
		c.arrastreMov = &arrastreMovimiento{caja: caja, indice: indice, base: PuntosMovimiento(a.Tipo, a.Puntos)}
	}
	g := c.arrastreMov
	g.dx, g.dy = g.dx+d.DX, g.dy+d.DY
	k := c.escala()
	pts := slices.Clone(g.base)
	n := len(pts) - 1
	pts[n] = Punto{
		DX: min(max(g.base[n].DX+float64(g.dx/(AnchoEscenario*k)*100), -100), 100),
		DY: min(max(g.base[n].DY+float64(g.dy/(e.m.AltoEscenario()*k)*100), -100), 100),
	}
	e.m.PonerPuntosMovimiento(e.objetivo(), g.caja, g.indice, pts)
	c.Refresh()
}

// soltarRedonda es `onConnHandleUp` y `onMotionEndUp`.
func (c *capaEdicion) soltarRedonda() {
	if c.arrastreCon == nil && c.arrastreMov == nil {
		return
	}
	c.arrastreCon, c.arrastreMov = nil, nil
	c.e.cambiar(func() {}, true)
}

func (c *capaEdicion) escala() float32 {
	if k := c.e.escenario.Escala; k > 0 {
		return k
	}
	return 1
}

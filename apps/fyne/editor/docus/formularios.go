// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Los CAMPOS DE FORMULARIO en el folio (`rico/formularios.go`): cómo se pintan —las fichas
// de `editor.css`— y el modo «Rellenar formulario» de la pestaña Insertar (el grupo
// «Formulario» de Insertar.tsx y la interacción de PageCanvas.tsx).
//
// Rellenando, el folio no se escribe (como el `contentEditable=false` de la web) pero los
// campos responden: la casilla se marca y se desmarca, el desplegable abre sus opciones y el
// de texto abre un campo donde escribir su valor. Con el teclado, Tab y Mayús+Tab van de un
// campo a otro y Intro o Espacio lo activan, que es el `tabindex` de la web. Cada cambio
// vuelve al HTML del propio campo y se guarda como cualquier otra edición.
//
// ponytail: aún no se insertan campos nuevos (los tres popovers de `CampoFieldPanel`): se
// leen, se pintan y se rellenan los que trae el documento.

// La caja de un campo, de `.sytue-page [data-formfield]`.
const (
	rellenoFicha      = 4    // `padding: 0 4px`
	bordeFicha        = 1    // `border: 1px solid`
	radioFicha        = 3    // `border-radius: 3px`
	interlineadoFicha = 1.35 // `line-height: 1.35`
	minFicha          = 2.5  // `min-width: 2.5em`
	ladoCasilla       = 1.1  // `width` y `height: 1.1em` de la casilla
	bajaCasilla       = 0.15 // `vertical-align: -0.15em`
	huecoFlecha       = 6    // el `margin-left: 6px` del `▾`
	anchoFlecha       = 0.5  // el `▾`, en em: se dibuja (Figtree no lo tiene)
	aroFicha          = 2    // `box-shadow: 0 0 0 2px var(--sytue-focus-ring)` al enfocar
)

// textoFicha es lo que se lee dentro del campo: su valor o, vacío, su marcador.
func textoFicha(c *rico.CampoFormulario) (texto string, marcador bool) {
	if c.Tipo == rico.CampoCasilla {
		return "", false
	}
	if c.Valor == "" {
		return c.Marcador, true
	}
	return c.Valor, false
}

// anchoFicha es lo que ocupa el campo en la línea (`box-sizing: border-box`).
func anchoFicha(c *rico.CampoFormulario, e rico.Estilo) float32 {
	if c.Tipo == rico.CampoCasilla {
		return ladoCasilla * e.Tamano
	}
	texto, _ := textoFicha(c)
	w := medidor.ancho(texto, e) + 2*(rellenoFicha+bordeFicha)
	if c.Tipo == rico.CampoDesplegable {
		w += huecoFlecha + anchoFlecha*e.Tamano
	}
	return max(w, minFicha*e.Tamano)
}

// cajaFicha es el alto y el ancho del campo, y su y en px de maqueta, a partir de la línea
// base de su texto: la casilla baja 0,15 em de la base; los demás son un `inline-block`
// de `line-height: 1.35` con su borde.
func cajaFicha(c *rico.CampoFormulario, e rico.Estilo, base float32) (y, w, h float32) {
	if c.Tipo == rico.CampoCasilla {
		lado := ladoCasilla * e.Tamano
		return base + bajaCasilla*e.Tamano - lado, lado, lado
	}
	l := interlineadoFicha * e.Tamano
	sube := (l-cajaFuente*e.Tamano)/2 + ascFuente*e.Tamano
	return base - sube - bordeFicha, anchoFicha(c, e), l + 2*bordeFicha
}

// fichaCampo es un campo ya colocado en el folio: dónde está, en px de maqueta.
type fichaCampo struct {
	campo      *rico.CampoFormulario
	lugar      rico.Lugar
	desde      int
	indice     int // el orden del campo en el documento
	x, y, w, h float32
}

// fichasCampo son los campos del folio en el orden en que se recorren con Tab: el de las
// cajas, que es el del documento.
func (f *Folio) fichasCampo() []fichaCampo {
	var out []fichaCampo
	for _, c := range f.cajas {
		if c.b.Oculta {
			continue
		}
		for _, l := range c.lineas {
			for _, p := range l.piezas {
				if p.campo == nil {
					continue
				}
				x := c.izq + l.x0 + p.x
				y, w, h := cajaFicha(p.campo, p.est, c.y+c.arribaTexto+l.y+l.base+desplazamientoVertical(p.est))
				out = append(out, fichaCampo{campo: p.campo, lugar: c.b.Lugar, desde: p.desde, indice: len(out), x: x, y: y, w: w, h: h})
			}
		}
	}
	return out
}

// pintarFicha añade el campo pintado en (x, y) de maqueta: la caja tenue con su borde (de
// latón, rellenando), el valor o el marcador, el `▾` del desplegable o el `✓` de la casilla,
// y el aro si tiene el foco del teclado.
func (f *Folio) pintarFicha(objs []fyne.CanvasObject, p pieza, x, base, ox, oy, z float32, enfocada bool) []fyne.CanvasObject {
	c, e := p.campo, p.est
	y, w, h := cajaFicha(c, e, base)
	pos := func(px, py float32) fyne.Position { return fyne.NewPos(ox+px*z, oy+py*z) }
	if enfocada {
		aro := canvas.NewRectangle(color.NRGBA{})
		aro.StrokeColor = tema.Color("focus-ring", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
		aro.StrokeWidth, aro.CornerRadius = aroFicha*z, (radioFicha+aroFicha)*z
		aro.Move(pos(x-aroFicha, y-aroFicha))
		aro.Resize(fyne.NewSize((w+2*aroFicha)*z, (h+2*aroFicha)*z))
		objs = append(objs, aro)
	}
	caja := canvas.NewRectangle(rico.FondoCampo)
	caja.StrokeColor, caja.StrokeWidth, caja.CornerRadius = rico.BordeCampo, max(bordeFicha*z, 1), radioFicha*z
	if f.rellenando {
		caja.FillColor, caja.StrokeColor = rico.FondoRellenar, rico.BordeRellenar
	}
	caja.Move(pos(x, y))
	caja.Resize(fyne.NewSize(w*z, h*z))
	objs = append(objs, caja)
	switch c.Tipo {
	case rico.CampoCasilla:
		if !c.Marcada() {
			return objs
		}
		// El `✓` en negrita de `--sytue-primary-700`, dibujado: dos trazos.
		trazo := max(e.Tamano/8*z, 1)
		corto := canvas.NewLine(rico.TintaMarca)
		corto.StrokeWidth = trazo
		corto.Position1, corto.Position2 = pos(x+0.24*w, y+0.52*h), pos(x+0.43*w, y+0.72*h)
		largo := canvas.NewLine(rico.TintaMarca)
		largo.StrokeWidth = trazo
		largo.Position1, largo.Position2 = pos(x+0.43*w, y+0.72*h), pos(x+0.78*w, y+0.3*h)
		return append(objs, corto, largo)
	case rico.CampoDesplegable:
		// El `▾`: un triángulo de trazos pegado al borde derecho, centrado en la x-altura.
		fl := anchoFlecha * e.Tamano
		fx := x + w - bordeFicha - rellenoFicha - fl
		fy := base - 0.38*e.Tamano
		for _, tr := range [][2][2]float32{{{fx, fy}, {fx + fl/2, fy + fl/2}}, {{fx + fl/2, fy + fl/2}, {fx + fl, fy}}} {
			l := canvas.NewLine(rico.TintaFlecha)
			l.StrokeWidth = max(1.5*z, 1)
			l.Position1, l.Position2 = pos(tr[0][0], tr[0][1]), pos(tr[1][0], tr[1][1])
			objs = append(objs, l)
		}
	}
	if texto, marcador := textoFicha(c); texto != "" {
		if marcador {
			e.Color = rico.TintaMarcador
		}
		t := textoContenido(texto, e, z)
		t.Move(pos(x+bordeFicha+rellenoFicha, base-ascFuente*e.Tamano))
		objs = append(objs, t)
	}
	return objs
}

// ---- el modo «Rellenar formulario» ----

// Rellenando dice si el folio está en el modo «Rellenar formulario».
func (f *Folio) Rellenando() bool { return f.rellenando }

// puedeRellenar dice si un campo se puede cambiar: el documento se entiende, el permiso lo
// deja y no se está viendo un registro de la combinación.
func (f *Folio) puedeRellenar() bool {
	return f.rellenando && f.doc != nil && f.doc.Editable() && (f.ctx == nil || !f.ctx.SoloLectura) && f.previa == nil
}

// AlternarRellenar entra en el modo «Rellenar formulario» o sale de él (`formFill`).
func (f *Folio) AlternarRellenar() {
	f.rellenando = !f.rellenando && f.doc != nil && f.doc.Editable()
	f.campoFoco = -1
	f.cerrarCampo()
	f.anc = f.cur // sin selección: lo que se hace ahora es en los campos
	f.pag.Refresh()
	f.programarFormato()
}

// campoEn es el índice (en [Folio.fichasCampo]) del campo bajo ese punto de la página, o -1.
func (f *Folio) campoEn(pos fyne.Position) int {
	z := f.escala()
	ox, oy := f.pag.origen()
	mx, my := (pos.X-ox)/z, (pos.Y-oy)/z
	for i, fc := range f.fichasCampo() {
		if mx >= fc.x && mx <= fc.x+fc.w && my >= fc.y && my <= fc.y+fc.h {
			return i
		}
	}
	return -1
}

// pulsarCampo es el clic en el modo rellenar: sobre un campo, lo activa y dice que el clic
// era suyo.
func (f *Folio) pulsarCampo(pos fyne.Position) bool {
	if !f.rellenando {
		return false
	}
	i := f.campoEn(pos)
	if i < 0 {
		return false
	}
	f.campoFoco = i
	f.activarCampo(i)
	return true
}

// activarCampo es `activate`: la casilla se marca o se desmarca, el desplegable abre sus
// opciones y el de texto, el campo donde escribir.
func (f *Folio) activarCampo(i int) {
	fs := f.fichasCampo()
	if i < 0 || i >= len(fs) || !f.puedeRellenar() {
		return
	}
	fc := fs[i]
	switch fc.campo.Tipo {
	case rico.CampoCasilla:
		v := "1"
		if fc.campo.Marcada() {
			v = "0"
		}
		f.rellenarCampo(fc.campo, v)
	case rico.CampoDesplegable:
		f.abrirOpciones(fc)
	default:
		f.abrirTexto(fc)
	}
}

// rellenarCampo guarda el valor en el campo, con su paso para deshacer, y avisa como
// cualquier edición (lo que en la web es el `input` que burbujea al folio).
func (f *Folio) rellenarCampo(c *rico.CampoFormulario, v string) {
	if !f.puedeRellenar() {
		return
	}
	foto := paso{f.doc.HTML(), f.cur, f.anc}
	if !f.doc.RellenarCampo(c, v) {
		return
	}
	f.apuntar(foto, false)
	f.reestructurado()
}

// tabCampo lleva el foco del teclado al campo siguiente (o al anterior) y dice si había
// uno; sin él, Tab sale del folio como de la última ficha con `tabindex` en la web.
func (f *Folio) tabCampo(atras bool) bool {
	n := len(f.fichasCampo())
	sig := f.campoFoco + 1
	if atras {
		sig = f.campoFoco - 1
		if f.campoFoco < 0 {
			sig = n - 1
		}
	}
	if sig < 0 || sig >= n {
		f.campoFoco = -1
		return false
	}
	f.campoFoco = sig
	f.pag.Refresh()
	return true
}

// AcceptsTab es `fyne.Tabbable`: rellenando, Tab va de campo en campo mientras quede otro.
func (p *pagina) AcceptsTab() bool {
	f := p.f
	if !f.rellenando {
		return false
	}
	n := len(f.fichasCampo())
	if f.mayus {
		if f.campoFoco == 0 || n == 0 {
			f.campoFoco = -1 // sale hacia atrás: el próximo Tab empieza por el primero
			return false
		}
		return true
	}
	if f.campoFoco+1 >= n {
		f.campoFoco = -1
		return false
	}
	return true
}

// teclaRellenar atiende las teclas del modo rellenar y dice si eran suyas: Tab y Mayús+Tab
// recorren los campos, Intro activa el enfocado; las demás no escriben en el folio.
func (f *Folio) teclaRellenar(e *fyne.KeyEvent) bool {
	if !f.rellenando {
		return false
	}
	switch e.Name {
	case fyne.KeyTab:
		f.tabCampo(f.mayus)
		return true
	case fyne.KeyReturn, fyne.KeyEnter: // Espacio llega como runa (TypedRune)
		f.activarCampo(f.campoFoco)
		return true
	case fyne.KeyBackspace, fyne.KeyDelete:
		return f.campoFoco >= 0 // no borran nada del folio con un campo enfocado
	default:
		return false
	}
}

// campoVigente es el campo de fc en el documento de AHORA. Con el panel abierto el documento
// se puede rehacer debajo (la coedición lo fusiona, deshacer lo vuelve a leer) y el campo
// leído al abrirlo ya no estaría en él: lo escrito se perdería sin decir nada. Se busca el
// mismo campo —el mismo orden, tipo y nombre— entre los de ahora (los bloques releídos son
// nodos nuevos: su [rico.Lugar] no sirve); si ya no está, el viejo, y
// [rico.Documento.RellenarCampo] no toca nada.
func (f *Folio) campoVigente(fc fichaCampo) *rico.CampoFormulario {
	if fs := f.fichasCampo(); fc.indice < len(fs) {
		if o := fs[fc.indice].campo; o.Tipo == fc.campo.Tipo && o.Nombre == fc.campo.Nombre {
			return o
		}
	}
	return fc.campo
}

// cerrarCampo cierra el panel del campo que esté abierto.
func (f *Folio) cerrarCampo() {
	if f.panelCampo != nil {
		f.panelCampo.Cerrar()
		f.panelCampo = nil
	}
}

// colocarBajoCampo pone un panel colgando del campo (`top: rect.bottom + 2`, `left:
// rect.left`), dentro de la ventana.
func (f *Folio) colocarBajoCampo(fc fichaCampo) func(ventana, tam fyne.Size) (fyne.Position, fyne.Size) {
	return func(ventana, tam fyne.Size) (fyne.Position, fyne.Size) {
		z := f.escala()
		ox, oy := f.pag.origen()
		abs := fyne.CurrentApp().Driver().AbsolutePositionForObject(f.pag)
		x := abs.X + ox + fc.x*z
		y := abs.Y + oy + (fc.y+fc.h)*z + 2
		if y+tam.Height > ventana.Height-ui.MargenVentana {
			y = max(abs.Y+oy+fc.y*z-2-tam.Height, ui.MargenVentana) // no cabe debajo: encima
		}
		x = max(ui.MargenVentana, min(x, ventana.Width-tam.Width-ui.MargenVentana))
		return fyne.NewPos(x, y), tam
	}
}

// abrirOpciones es el menú del desplegable (`data-formfield-menu`): sus opciones, con la
// elegida resaltada; la pulsada se guarda.
func (f *Folio) abrirOpciones(fc fichaCampo) {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(f.pag)
	if lienzo == nil {
		return
	}
	f.cerrarCampo()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var filas []fyne.CanvasObject
	f.opcionesCampo = nil
	for _, o := range fc.campo.Opciones {
		el := ui.NuevoElementoMenu(o, iconos.Ninguno, func() {
			f.cerrarCampo()
			f.rellenarCampo(f.campoVigente(fc), o)
			f.enfocar()
		})
		el.Denso, el.Actual = true, fc.campo.Valor == o
		f.opcionesCampo = append(f.opcionesCampo, el)
		filas = append(filas, el)
	}
	if len(filas) == 0 {
		filas = append(filas, container.New(layout.NewCustomPaddedLayout(tema.Espacio(1.5), tema.Espacio(1.5), tema.Espacio(2), tema.Espacio(2)),
			ui.NuevoParrafo(f.traducir("formfield.options.empty"), tema.TextoXS, ui.Normal, "text-muted", v)))
	}
	// `min-w-[8rem] max-h-60 overflow-auto p-1`.
	cuerpo := container.New(anchoMinimo{8 * tema.Rem}, container.New(ui.Columna{}, filas...))
	f.panelCampo = ui.MostrarEmergenteEn(lienzo, alturaMaxima{tema.Espacio(60), container.NewVScroll(cuerpo)}.envolver(),
		tema.Espacio(1), f.colocarBajoCampo(fc))
	f.panelCampo.Rotulo = f.traducir("formfield.options")
}

// abrirTexto es el campo donde se escribe el valor de un campo de texto: con su nombre
// encima y su marcador de agua; Intro (o «Aplicar») lo guarda y Escape lo deja como estaba.
// En la web el propio campo es una isla editable del folio; aquí el folio no se edita por
// dentro de una ficha, y el valor se escribe al lado, sin formato, que es lo que se guarda.
func (f *Folio) abrirTexto(fc fichaCampo) {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(f.pag)
	if lienzo == nil {
		return
	}
	f.cerrarCampo()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	entrada := ui.NuevaEntrada(fc.campo.Marcador).Con(ui.CampoSM)
	entrada.Ancho = tema.Espacio(56) // `w-56`, el de los paneles de la cinta
	entrada.PonerTexto(fc.campo.Valor)
	entrada.Rotulo = fc.campo.Nombre
	aplicar := func() {
		valor := entrada.Texto()
		f.cerrarCampo()
		f.rellenarCampo(f.campoVigente(fc), valor)
		f.enfocar()
	}
	entrada.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			aplicar()
			return true
		}
		return false
	}
	nombre := fc.campo.Nombre
	if nombre == "" {
		nombre = f.traducir("form.responses.unnamed")
	}
	boton := ui.NuevoBoton(f.traducir("action.apply"), aplicar).Con(ui.Primario, ui.TallaXS)
	panel := container.New(ui.Columna{Hueco: tema.Espacio(1.5)},
		ui.NuevoParrafo(nombre, tema.TextoXS, ui.Medio, "text", v).A(tema.Espacio(56)), entrada, container.NewHBox(boton))
	f.entradaCampo = entrada
	f.panelCampo = ui.MostrarEmergenteEn(lienzo, panel, editor.RellenoEmergenteCinta, f.colocarBajoCampo(fc))
	f.panelCampo.Rotulo = nombre
	lienzo.Focus(entrada)
}

// alturaMaxima acota el alto de su hijo (`max-h-…`): lo que no cabe se desplaza dentro.
type alturaMaxima struct {
	alto float32
	hijo fyne.CanvasObject
}

func (a alturaMaxima) envolver() fyne.CanvasObject { return container.New(a, a.hijo) }

func (a alturaMaxima) MinSize(objs []fyne.CanvasObject) fyne.Size {
	s := objs[0].MinSize()
	if sc, ok := objs[0].(*container.Scroll); ok && sc.Content != nil {
		s = sc.Content.MinSize()
	}
	return fyne.NewSize(s.Width, min(s.Height, a.alto))
}

func (a alturaMaxima) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

// ---- la cinta ----

// grupoFormulario es `FormularioGroup`: «Rellenar formulario», que se queda pulsado mientras
// dura, y «Ver respuestas».
func grupoFormulario(c *editor.Contexto, f *Folio) fyne.CanvasObject {
	rellenar := editor.NuevoBotonCinta(iconos.EdRellenar, c.Traducir("form.fill"), func() {
		f.AlternarRellenar()
		if c.Refrescar != nil {
			c.Refrescar()
		}
	})
	rellenar.Activo = f.rellenando
	rellenar.Inerte = !f.rellenando && (c.SoloLectura || !f.doc.Editable() || f.previa != nil)
	respuestas := editor.NuevoBotonEmergente(iconos.EdPrevia, c.Traducir("form.responses"), func(func()) fyne.CanvasObject {
		return panelRespuestas(c, f)
	})
	return editor.NuevoGrupo(c.Traducir("group.form"), rellenar, respuestas)
}

// AnchoRespuestas es el `w-64` de «Ver respuestas».
var AnchoRespuestas = tema.Espacio(64)

// panelRespuestas es `VerRespuestasPanel`: el nombre y el valor de cada campo, en orden, y
// «Copiar respuestas» con «Nombre: valor» por línea; sin campos, lo dice.
func panelRespuestas(c *editor.Contexto, f *Folio) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	rs := f.doc.Respuestas()
	ancho := AnchoRespuestas
	if len(rs) == 0 {
		return container.New(anchoFijo{ancho}, container.New(layout.NewCustomPaddedLayout(tema.Espacio(1.5), tema.Espacio(1.5), tema.Espacio(1), tema.Espacio(1)),
			ui.NuevoParrafo(c.Traducir("form.responses.empty"), tema.TextoXS, ui.Normal, "text-muted", v).A(ancho-tema.Espacio(2))))
	}
	sinNombre := c.Traducir("form.responses.unnamed")
	filas := make([]fyne.CanvasObject, 0, len(rs))
	for _, r := range rs {
		nombre, valor := r.Nombre, r.Valor
		if nombre == "" {
			nombre = sinNombre
		}
		if valor == "" {
			valor = "—"
		}
		fondo := canvas.NewRectangle(tema.Color("bg-subtle", v))
		fondo.CornerRadius = tema.RadioSM
		// `bg-bg-subtle flex flex-col gap-0.5 rounded-sm px-2 py-1`.
		dentro := container.New(layout.NewCustomPaddedLayout(tema.Espacio(1), tema.Espacio(1), tema.Espacio(2), tema.Espacio(2)),
			container.New(ui.Columna{Hueco: tema.Espacio(0.5)},
				ui.NuevoParrafo(nombre, tema.Texto2XS, ui.Medio, "text-muted", v).A(ancho-tema.Espacio(4)),
				ui.NuevoParrafo(valor, tema.TextoXS, ui.Normal, "text", v).A(ancho-tema.Espacio(4))))
		filas = append(filas, container.NewStack(fondo, dentro))
	}
	lista := alturaMaxima{tema.Espacio(64), container.NewVScroll(container.New(ui.Columna{Hueco: tema.Espacio(1)}, filas...))}.envolver()
	copiar := ui.NuevoBoton(c.Traducir("form.responses.copy"), func() {
		fyne.CurrentApp().Clipboard().SetContent(rico.RespuestasATexto(rs, sinNombre))
		if c.Notificar != nil {
			c.Notificar(ui.AvisoExito, c.Traducir("form.responses.copied"))
		}
	}).Con(ui.Secundario, ui.TallaSM)
	return container.New(anchoFijo{ancho}, container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, lista, copiar))
}

// esCampoFoco dice si el campo de ese bloque y esa runa es el que tiene el foco del teclado.
func (f *Folio) esCampoFoco(l rico.Lugar, desde int) bool {
	fs := f.fichasCampo()
	return f.campoFoco < len(fs) && fs[f.campoFoco].lugar == l && fs[f.campoFoco].desde == desde
}

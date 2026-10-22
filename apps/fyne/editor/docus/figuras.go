// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/editor/figuras"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los grupos «Formas» y «Gráfico» de Insertar (`ribbon/tabs/BloquesDoc.tsx`) y cómo se
// pintan sus figuras en el folio. El panel inserta si el cursor no está junto a una figura
// de su clase y, si lo está, la cambia en el acto: el `rebake` de la web.

// cmdFigura mete o cambia una figura; su valor es el HTML entero de la figura.
const cmdFigura editor.Comando = "figure"

// ordenFigura cambia la figura activa de la misma clase por la del valor o, si no hay,
// la inserta con el párrafo de escape detrás (`insertBlock`).
func (f *Folio) ordenFigura(c editor.Comando, valor string) bool {
	if c != cmdFigura {
		return false
	}
	if il := f.figuraActiva(rico.ClaseDe(valor)); il != nil {
		f.antesDeCambiar(false)
		if f.doc.PonerFigura(il, valor) {
			f.reestructurado()
		}
		return true
	}
	f.antesDeCambiar(false)
	f.borrarSeleccion()
	if l, ok := f.doc.InsertarHTML(f.doc.Bloques(), f.cur.b, f.cur.p, valor+"<p><br></p>"); ok {
		f.irA(l, 0)
	}
	f.reestructurado()
	return true
}

// cmdBloque inserta un bloque reutilizable: su HTML, ya vestido con la marca.
const cmdBloque editor.Comando = "bloque"

// ordenBloque mete el HTML del bloque en el cursor con el párrafo de escape detrás, como
// el `insertHTML` de `BloquesGroup.tsx`; entra en el deshacer como cualquier inserción.
func (f *Folio) ordenBloque(c editor.Comando, valor string) bool {
	if c != cmdBloque {
		return false
	}
	f.antesDeCambiar(false)
	f.borrarSeleccion()
	if l, ok := f.doc.InsertarHTML(f.doc.Bloques(), f.cur.b, f.cur.p, valor+"<p><br></p>"); ok {
		f.irA(l, 0)
	}
	f.reestructurado()
	return true
}

// figuraActiva es `figureFromSelection`: la figura de esa clase ([rico.Ilustracion.Clase])
// pegada al cursor, antes
// (con el cursor al principio de su bloque) o después (al final). Tras insertarla, el
// cursor queda en el párrafo de escape: la de antes.
func (f *Folio) figuraActiva(clase string) *rico.Ilustracion {
	de := func(i int) *rico.Ilustracion {
		if i < 0 || i >= len(f.bloques) || f.bloques[i].Tipo != rico.Figura {
			return nil
		}
		if il := f.bloques[i].Ilustracion; il.Clase() == clase {
			return il
		}
		return nil
	}
	if f.cur.p == 0 {
		if il := de(f.cur.b - 1); il != nil {
			return il
		}
	}
	if f.cur.p == f.largo(f.cur.b) {
		return de(f.cur.b + 1)
	}
	return nil
}

// medidaFigura es la caja de la figura en un ancho: `width: Wpx; max-width: 100%` y el
// SVG a lo ancho con `height: auto`, que guarda la proporción del viewBox.
func medidaFigura(il *rico.Ilustracion, ancho float32) (w, h float32) {
	nw, nh := float32(rico.AnchoGrafico), float32(rico.AltoGrafico)
	switch {
	case il.Forma != nil:
		nw, nh = float32(il.Forma.W), float32(il.Forma.H)
	case il.Smart != nil: // `width: 100%`
		nw, nh = ancho, float32(il.Smart.AltoSmart()/rico.AnchoSmart)*ancho
	}
	w = min(nw, ancho)
	return w, nh * w / nw
}

// escalaRaster es a cuántos píxeles por punto se rasteriza el SVG: la textura se estira a
// su caja, y a 2 se ve nítida también en una pantalla Retina.
const escalaRaster = 2

// pintarFigura pinta la figura centrada (`margin: 1em auto`): el SVG rasterizado y, por
// encima, el texto de la forma o los rótulos del gráfico, que `oksvg` no pinta.
func (f *Folio) pintarFigura(objs []fyne.CanvasObject, c caja, ox, oy, z float32, usadas map[string]*canvas.Image) []fyne.CanvasObject {
	il := c.b.Ilustracion
	w, h := medidaFigura(il, c.ancho)
	pos := fyne.NewPos(ox+(c.izq+(c.ancho-w)/2)*z, oy+c.y*z)
	var svg []byte
	var rotulos []figuras.Rotulo
	nw := float32(rico.AnchoGrafico)
	var nodos []figuras.NodoSmart
	switch {
	case il.Forma != nil:
		svg, nw = il.Forma.SVG(), float32(il.Forma.W)
	case il.Smart != nil:
		var con []figuras.ConectorSmart
		nodos, con = il.Smart.Geometria()
		nw = rico.AnchoSmart
		svg = figuras.SVGSmart(nodos, con, rico.AnchoSmart, il.Smart.AltoSmart(), figuras.Paleta[0])
		if len(nodos) == 0 && f.ctx != nil {
			// `smartart.empty`, centrado, a 16 y con la tinta al 60 %.
			rotulos = []figuras.Rotulo{{
				Texto: f.ctx.Traducir("smartart.empty"), X: rico.AnchoSmart / 2, Y: il.Smart.AltoSmart()/2 + 16*0.35,
				Tam: 16, Ancla: figuras.AnclaMedio, Color: "#6C6C6C",
			}}
		}
	default:
		svg, rotulos, _ = il.Grafico.SVG()
	}
	pw, ph := int(w*z*escalaRaster), int(h*z*escalaRaster)
	if pw > 0 && ph > 0 {
		img := f.reutilizar(usadas, "svg\x00"+string(svg)+"\x00"+strconv.Itoa(pw)+"x"+strconv.Itoa(ph), func() image.Image {
			return iconos.RasterizarSVG(svg, pw, ph)
		})
		img.Move(pos)
		img.Resize(fyne.NewSize(w*z, h*z))
		objs = append(objs, img)
	}
	k := w / nw * z // de px del viewBox a px de pantalla
	for _, r := range rotulos {
		col, _ := editor.Hex(r.Color)
		objs = append(objs, editor.RotuloEnBase(r, pos, k, col, ui.Normal))
	}
	if il.Forma != nil && il.Forma.ConTexto() {
		objs = append(objs, textoForma(*il.Forma, pos, k)...)
	}
	if il.Smart != nil {
		tinta, _ := editor.Hex(figuras.Paleta[0])
		for _, nd := range nodos {
			texto := ""
			if nd.I < len(il.Smart.Elementos) {
				texto = il.Smart.Elementos[nd.I].Texto
			}
			objs = append(objs, editor.TextoNodoSmart(nd, texto, il.Smart.Disposicion == "list", pos, k, tinta)...)
		}
	}
	return objs
}

// textoForma es la capa `foreignObject` de la forma: `padding: 6px 10px`, centrado en los
// dos ejes, `font-weight: 500`, `line-height: 1.2` y lo que no cabe, recortado.
func textoForma(fo rico.Forma, pos fyne.Position, k float32) []fyne.CanvasObject {
	if strings.TrimSpace(fo.Texto) == "" {
		return nil
	}
	tam := float32(fo.LetraTexto()) * k
	e := tema.Escalon{Tam: tam, Linea: tam * 1.2}
	cabe, alto := (float32(fo.W)-20)*k, (float32(fo.H)-12)*k
	lineas := ui.Repartir(fo.Texto, e, ui.Medio, max(cabe, 1))
	lineas = lineas[:min(len(lineas), max(1, int(alto/e.Linea)))]
	tinta, _ := editor.Hex(figuras.Paleta[0])
	y := pos.Y + (float32(fo.H)*k-float32(len(lineas))*e.Linea)/2
	fuera := make([]fyne.CanvasObject, 0, len(lineas))
	for _, l := range lineas {
		t := ui.Texto(l, e, ui.Medio, tinta)
		ui.ColocarTexto(t, e, pos.X+(float32(fo.W)*k-ui.Medir(l, e, ui.Medio).Width)/2, y)
		fuera = append(fuera, t)
		y += e.Linea
	}
	return fuera
}

// ---- los paneles ----

// Anchos de los paneles: `w-64` el de las formas, `w-80` el de los datos del gráfico y
// `w-52` el de elegir su tipo, sin el `p-2` que pone el emergente.
var (
	anchoPanelFormas     = tema.Espacio(64) - 2*tema.Espacio(2)
	anchoPanelGrafico    = tema.Espacio(80) - 2*tema.Espacio(2)
	anchoPanelTipoGraf   = tema.Espacio(52) - 2*tema.Espacio(2)
	anchoCampoMedida     = tema.Espacio(16)
	anchoRotuloFila      = tema.Espacio(12)
	anchoSelectorGrafico = float32(140)
)

// paletaRelleno es FILL_PALETTE: la de contenido y 'transparent', sin relleno.
var paletaRelleno = append(append([]string{}, figuras.Paleta...), "transparent")

// grupoFormas es `FormasDocGroup`.
func grupoFormas(c *editor.Contexto, f *Folio, inerte func(*editor.BotonCinta) *editor.BotonCinta) fyne.CanvasObject {
	b := inerte(editor.NuevoBotonEmergente(iconos.EdRect, c.Traducir("blocks.shapes"), func(func()) fyne.CanvasObject {
		return nuevoPanelFormas(c, f)
	}))
	return editor.NuevoGrupo(c.Traducir("blocks.shapes"), b)
}

type panelFormas struct {
	c        *editor.Contexto
	f        *Folio
	cfg      rico.Forma
	editando bool
	cuerpo   *fyne.Container
	ancho    *ui.Entrada
	alto     *ui.Entrada
	texto    *ui.Entrada
}

func nuevoPanelFormas(c *editor.Contexto, f *Folio) fyne.CanvasObject {
	p := &panelFormas{c: c, f: f, cfg: rico.SanearForma("rect", "", "", "", "", "")}
	if il := f.figuraActiva("shape"); il != nil {
		p.cfg, p.editando = *il.Forma, true
	}
	medida := func(rotulo string, v int, poner func(*rico.Forma, int)) *ui.Entrada {
		e := campoPanel("", anchoCampoMedida)
		e.Rotulo = rotulo
		e.PonerTexto(strconv.Itoa(v))
		e.AlCambiar(func(s string) {
			// ponytail: la web lo aplica al confirmar (`onChange`); aquí, en cuanto es una
			// medida válida, para no encoger la forma a 24 al teclear la primera cifra.
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n >= rico.MinForma && n <= rico.MaxForma {
				p.cambiar(func(fo *rico.Forma) { poner(fo, n) })
			}
		})
		return e
	}
	p.ancho = medida(c.Traducir("blocks.shape.width"), p.cfg.W, func(fo *rico.Forma, n int) { fo.W = n })
	p.alto = medida(c.Traducir("blocks.shape.height"), p.cfg.H, func(fo *rico.Forma, n int) { fo.H = n })
	p.texto = campoPanel(c.Traducir("blocks.shape.innerHint"), 0)
	p.texto.Rotulo = c.Traducir("blocks.shape.innerText")
	p.texto.PonerTexto(p.cfg.Texto)
	p.texto.AlCambiar(func(s string) { p.cambiar(func(fo *rico.Forma) { fo.Texto = s }) })
	p.cuerpo = container.New(ui.Columna{Hueco: tema.Espacio(1)})
	p.pintar()
	return container.New(anchoFijo{anchoPanelFormas}, p.cuerpo)
}

// cambiar aplica el parche a la forma activa; sin ella, no hay nada que cambiar.
func (p *panelFormas) cambiar(parche func(*rico.Forma)) {
	parche(&p.cfg)
	if p.editando {
		p.c.Mandar(cmdFigura, p.cfg.HTML(p.aria()))
	}
}

// aria es `shape.aria` con el nombre de la forma en minúsculas.
func (p *panelFormas) aria() string {
	return p.c.Traducir("shape.aria", i18n.Vars{"kind": strings.ToLower(p.nombre(p.cfg.Tipo))})
}

func (p *panelFormas) nombre(tipo string) string {
	for _, fu := range editor.FormasUI {
		if fu.ID == tipo {
			return p.c.Traducir(fu.Clave)
		}
	}
	return p.c.Traducir("shape.generic")
}

// elegir es `onShape`: cambia la forma activa o inserta una nueva, que pasa a ser la activa.
func (p *panelFormas) elegir(tipo string) {
	if p.editando {
		p.cambiar(func(fo *rico.Forma) { fo.Tipo = tipo })
	} else {
		p.cfg = rico.SanearForma(tipo, "", "", "", "", "")
		p.c.Mandar(cmdFigura, p.cfg.HTML(p.aria()))
		p.editando = true
	}
	p.pintar()
}

func (p *panelFormas) pintar() {
	c, v := p.c, tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	pista := "shape.pickHint"
	if p.editando {
		pista = "shape.editHint"
	}
	botones := make([]fyne.CanvasObject, 0, len(editor.FormasUI))
	for _, fu := range editor.FormasUI {
		clave := "shape.insert"
		if p.editando {
			clave = "shape.changeTo"
		}
		b := ui.NuevoBotonIcono(0, c.Traducir(clave, i18n.Vars{"kind": strings.ToLower(c.Traducir(fu.Clave))}), func() { p.elegir(fu.ID) })
		b.Glifo = iconos.TrazoEd(fu.Icono, iconos.TallaMD, "text-muted", v)
		b.Lado, b.TamIcono = ui.LadoIconoLG, iconos.TallaMD
		b.Pulsado = p.editando && p.cfg.Tipo == fu.ID
		botones = append(botones, b)
	}
	objs := []fyne.CanvasObject{
		ui.NuevoRotulo(c.Traducir(pista), tema.TextoXS, ui.Normal, "text-muted"),
		container.New(&envuelta{hueco: tema.Espacio(0.5), ancho: anchoPanelFormas}, botones...),
	}
	if p.editando {
		relleno := editor.NuevaMuestraCinta(c, iconos.EdResaltado, c.Traducir("shape.fill"), p.cfg.Relleno, paletaRelleno,
			func(col string) { p.cambiar(func(fo *rico.Forma) { fo.Relleno = col }) })
		trazo := editor.NuevaMuestraCinta(c, iconos.EdColorTexto, c.Traducir("border.color.label"), p.cfg.Trazo, figuras.Paleta,
			func(col string) { p.cambiar(func(fo *rico.Forma) { fo.Trazo = col }) })
		por := ui.NuevoRotulo("×", tema.TextoXS, ui.Normal, "text-muted")
		// «Estilo» y «Texto» van escritos así en la web, sin traducir.
		objs = append(objs,
			filaFigura("Estilo", relleno, trazo),
			filaFigura(c.Traducir("common.size"), p.ancho, por, p.alto),
		)
		if p.cfg.ConTexto() {
			objs = append(objs, filaFigura("Texto", p.texto))
		}
	}
	p.cuerpo.Objects = objs
	p.cuerpo.Refresh()
}

// filaFigura es `flex items-center gap-1.5` con el rótulo `w-12 text-2xs` delante; el
// último control se estira (`flex-1`).
func filaFigura(rotulo string, controles ...fyne.CanvasObject) fyne.CanvasObject {
	r := ui.NuevoRotulo(rotulo, tema.Texto2XS, ui.Normal, "text-muted")
	return container.New(&filaRotulada{ancho: anchoRotuloFila, hueco: tema.Espacio(1.5)}, append([]fyne.CanvasObject{r}, controles...)...)
}

// filaRotulada coloca el rótulo en su ancho fijo y los controles a su medida detrás, con
// el último estirado hasta el final.
type filaRotulada struct{ ancho, hueco float32 }

func (l *filaRotulada) MinSize(objs []fyne.CanvasObject) fyne.Size {
	w, h := l.ancho, float32(0)
	for _, o := range objs {
		h = max(h, o.MinSize().Height)
	}
	for _, o := range objs[1:] {
		w += l.hueco + o.MinSize().Width
	}
	return fyne.NewSize(w, h)
}

func (l *filaRotulada) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	x := float32(0)
	for i, o := range objs {
		m := o.MinSize()
		w := m.Width
		switch {
		case i == 0:
			w = l.ancho
		case i == len(objs)-1:
			w = max(m.Width, t.Width-x)
		}
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(fyne.NewSize(w, m.Height))
		x += w + l.hueco
	}
}

// grupoGrafico es `GraficoDocGroup`.
func grupoGrafico(c *editor.Contexto, f *Folio, inerte func(*editor.BotonCinta) *editor.BotonCinta) fyne.CanvasObject {
	b := inerte(editor.NuevoBotonEmergente(iconos.EdGrafico, c.Traducir("blocks.chart"), func(func()) fyne.CanvasObject {
		return nuevoPanelGrafico(c, f)
	}))
	return editor.NuevoGrupo(c.Traducir("blocks.chart"), b)
}

type panelGrafico struct {
	c        *editor.Contexto
	g        rico.Grafico
	editando bool
	cuerpo   *fyne.Container
	ancho    *anchoFijo
}

func nuevoPanelGrafico(c *editor.Contexto, f *Folio) fyne.CanvasObject {
	p := &panelGrafico{c: c, g: rico.Grafico{Tipo: "bar", Rejilla: rico.RejillaInicial()}}
	if il := f.figuraActiva("chart"); il != nil {
		p.g, p.editando = *il.Grafico, true
	}
	p.cuerpo = container.New(ui.Columna{Hueco: tema.Espacio(1.5)})
	p.ancho = &anchoFijo{anchoPanelTipoGraf}
	p.pintar()
	return container.New(p.ancho, p.cuerpo)
}

// mandar hornea el gráfico y lo aplica: el de la figura activa o uno nuevo.
func (p *panelGrafico) mandar() { p.c.Mandar(cmdFigura, p.g.HTML()) }

func (p *panelGrafico) pintar() {
	c, v := p.c, tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if !p.editando {
		p.ancho.ancho = anchoPanelTipoGraf
		objs := make([]fyne.CanvasObject, 0, 1+len(rico.TiposGrafico))
		objs = append(objs, ui.NuevoRotulo(c.Traducir("blocks.shape.chooseType"), tema.TextoXS, ui.Normal, "text-muted"))
		for _, t := range rico.TiposGrafico {
			b := editor.NuevoBotonPequeno(t.Rotulo, func() {
				p.g = rico.Grafico{Tipo: t.ID, Rejilla: rico.RejillaInicial()}
				p.mandar()
				p.editando = true
				p.pintar()
			})
			b.Glifo = iconos.TrazoEd(iconos.EdGrafico, iconos.TallaSM, "text", v)
			b.Alto = tema.TextoXS.Linea + 2*tema.Espacio(1.5) + 2*tema.Borde
			objs = append(objs, b)
		}
		p.cuerpo.Objects = objs
		p.cuerpo.Refresh()
		return
	}
	p.ancho.ancho = anchoPanelGrafico
	opciones := make([]editor.Opcion, 0, len(rico.TiposGrafico))
	for _, t := range rico.TiposGrafico {
		opciones = append(opciones, editor.Opcion{Valor: t.ID, Rotulo: t.Rotulo})
	}
	tipo := editor.NuevoSelectorCinta(c.Traducir("chart.type"), anchoSelectorGrafico, opciones, p.g.Tipo, func(t string) {
		p.g.Tipo = t
		p.mandar()
	})
	filas, cols := len(p.g.Rejilla), len(p.g.Rejilla[0])
	boton := func(i iconos.Ed, clave string, apagado bool, hacer func()) *ui.BotonIcono {
		b := ui.NuevoBotonIcono(0, c.Traducir(clave), func() { hacer(); p.mandar(); p.pintar() })
		b.Glifo = iconos.TrazoEd(i, iconos.TallaSM, "text-muted", v)
		b.Lado, b.Inerte = ui.LadoIconoSM, apagado
		return b
	}
	g := &p.g
	controles := container.New(ui.Fila{Hueco: tema.Espacio(1.5)},
		ui.NuevoRotulo(c.Traducir("common.rows"), tema.Texto2XS, ui.Normal, "text-muted"),
		boton(iconos.EdCerrar, "blocks.row.remove", filas <= 2, func() { g.Rejilla = g.Rejilla[:filas-1] }),
		boton(iconos.EdMas, "blocks.row.add", filas >= rico.MaxFilasGraf, func() { g.Rejilla = append(g.Rejilla, make([]string, cols)) }),
		ui.NuevoRotulo(c.Traducir("layout.columns"), tema.Texto2XS, ui.Normal, "text-muted"),
		boton(iconos.EdCerrar, "blocks.col.remove", cols <= 2, func() {
			for i := range g.Rejilla {
				g.Rejilla[i] = g.Rejilla[i][:cols-1]
			}
		}),
		boton(iconos.EdMas, "blocks.col.add", cols >= rico.MaxColsGraf, func() {
			for i := range g.Rejilla {
				g.Rejilla[i] = append(g.Rejilla[i], "")
			}
		}),
	)
	p.cuerpo.Objects = []fyne.CanvasObject{
		filaAjuste("Tipo", tipo), // «Tipo» va escrito así en la web
		ui.NuevoRotulo(c.Traducir("chart.data.hint"), tema.Texto2XS, ui.Normal, "text-muted"),
		p.rejilla(),
		controles,
	}
	p.cuerpo.Refresh()
}

// rejilla es la tabla de campos `h-6 w-16`, en un scroll de `max-h-56`.
func (p *panelGrafico) rejilla() fyne.CanvasObject {
	filas := make([]fyne.CanvasObject, 0, len(p.g.Rejilla))
	for r, fila := range p.g.Rejilla {
		campos := make([]fyne.CanvasObject, 0, len(fila))
		for col, v := range fila {
			e := ui.NuevaEntrada("").ConPropia(ui.CampoPropio{
				MargenX: tema.Espacio(1.5), MargenY: (tema.Espacio(6) - tema.TextoXS.Linea - 2*tema.Borde) / 2,
				Texto: tema.TextoXS, Borde: "border", Radio: tema.RadioSM,
			})
			e.Ancho = tema.Espacio(16)
			e.Rotulo = p.c.Traducir("table.rowCol", i18n.Vars{"row": r + 1, "col": col + 1})
			e.PonerTexto(v)
			e.AlCambiar(func(s string) {
				p.g.Rejilla[r][col] = s
				p.mandar()
			})
			campos = append(campos, e)
		}
		filas = append(filas, container.New(ui.Fila{Hueco: tema.Espacio(1)}, campos...))
	}
	tabla := container.New(ui.Columna{Hueco: tema.Espacio(1)}, filas...)
	return container.New(altoMax{alto: tema.Espacio(56), contenido: tabla}, container.NewScroll(tabla))
}

// altoMax es `max-h-N overflow-auto`: mide lo que su contenido hasta ese alto, y lo que
// no cabe se desplaza. De ancho, lo que le den.
type altoMax struct {
	alto      float32
	contenido fyne.CanvasObject
}

func (a altoMax) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, min(a.contenido.MinSize().Height, a.alto))
}

func (a altoMax) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

// reutilizar es la textura de la clave pintada la vez anterior, o una nueva con `crear`:
// así Fyne no vuelve a subirla en cada tecla. Dos iguales en el mismo repintado son dos
// objetos: la segunda lleva un número detrás.
func (f *Folio) reutilizar(usadas map[string]*canvas.Image, base string, crear func() image.Image) *canvas.Image {
	clave := base
	for n := 1; usadas[clave] != nil; n++ {
		clave = base + "\x00" + strconv.Itoa(n)
	}
	f.laminasMu.Lock()
	ci := f.laminas[clave]
	f.laminasMu.Unlock()
	if ci == nil {
		ci = canvas.NewImageFromImage(crear())
		ci.FillMode, ci.ScaleMode = canvas.ImageFillStretch, canvas.ImageScaleSmooth
	}
	usadas[clave] = ci
	return ci
}

// grupoSmart es `SmartArtDocGroup`.
func grupoSmart(c *editor.Contexto, f *Folio, inerte func(*editor.BotonCinta) *editor.BotonCinta) fyne.CanvasObject {
	b := inerte(editor.NuevoBotonEmergente(iconos.EdSmartLista, c.Traducir("smartart.insertOrEdit"), func(func()) fyne.CanvasObject {
		return nuevoPanelSmart(c, f)
	}))
	return editor.NuevoGrupo(c.Traducir("smartart.title"), b)
}

// anchoPanelSmart es `w-72`, sin el `p-2` del emergente.
var anchoPanelSmart = tema.Espacio(72) - 2*tema.Espacio(2)

type panelSmart struct {
	c        *editor.Contexto
	sa       rico.SmartArt
	editando bool
	cuerpo   *fyne.Container
}

func nuevoPanelSmart(c *editor.Contexto, f *Folio) fyne.CanvasObject {
	p := &panelSmart{c: c}
	if il := f.figuraActiva("smartart"); il != nil {
		p.sa, p.editando = *il.Smart, true
	}
	p.cuerpo = container.New(ui.Columna{Hueco: tema.Espacio(1.5)})
	p.pintar()
	return container.New(anchoFijo{anchoPanelSmart}, p.cuerpo)
}

// mandar hornea el diagrama y lo aplica: el de la figura activa o uno nuevo.
func (p *panelSmart) mandar() {
	c := p.c
	aria := c.Traducir("smartart.aria", i18n.Vars{"kind": c.Traducir("smartart.kind." + p.sa.Disposicion)})
	c.Mandar(cmdFigura, p.sa.HTML(aria, c.Traducir("smartart.empty")))
}

// cambiar aplica un cambio de los elementos y, si cambia cuántos hay o su orden, rehace
// la lista.
func (p *panelSmart) cambiar(hacer func(), rehacer bool) {
	hacer()
	p.mandar()
	if rehacer {
		p.pintar()
	}
}

func (p *panelSmart) pintar() {
	c, v := p.c, tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if !p.editando {
		botones := make([]fyne.CanvasObject, 0, len(editor.DisposicionesSmartUI))
		for _, d := range editor.DisposicionesSmartUI {
			b := editor.NuevoBotonPequeno(c.Traducir(d.Clave), func() {
				p.sa = rico.SmartArt{Disposicion: d.ID, Elementos: rico.ElementosIniciales(d.ID)}
				p.editando = true
				p.mandar()
				p.pintar()
			})
			b.Glifo = iconos.TrazoEd(d.Icono, iconos.TallaSM, "text", v)
			b.Alto = tema.TextoXS.Linea + 2*tema.Espacio(1.5) + 2*tema.Borde
			botones = append(botones, b)
		}
		filas := []fyne.CanvasObject{ui.NuevoRotulo(c.Traducir("smartart.chooseLayout"), tema.TextoXS, ui.Normal, "text-muted")}
		for i := 0; i < len(botones); i += 2 { // `grid grid-cols-2 gap-1`
			filas = append(filas, container.New(repartoIgual{hueco: tema.Espacio(1)}, botones[i:min(i+2, len(botones))]...))
		}
		p.cuerpo.Objects = filas
		p.cuerpo.Refresh()
		return
	}
	opciones := make([]editor.Opcion, 0, len(editor.DisposicionesSmartUI))
	for _, d := range editor.DisposicionesSmartUI {
		opciones = append(opciones, editor.Opcion{Valor: d.ID, Rotulo: c.Traducir(d.Clave)})
	}
	disp := editor.NuevoSelectorCinta(c.Traducir("smartart.layout"), anchoSelectorGrafico, opciones, p.sa.Disposicion, func(d string) {
		p.cambiar(func() { p.sa.Disposicion = rico.SanearDisposicion(d) }, true)
	})
	pista := "smartart.hint.items"
	if p.sa.Disposicion == "hierarchy" {
		pista = "smartart.hint.hierarchy"
	}
	filas := make([]fyne.CanvasObject, 0, len(p.sa.Elementos))
	for i := range p.sa.Elementos {
		filas = append(filas, p.fila(i, v))
	}
	lista := container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, filas...)
	anadir := editor.NuevoBotonPequeno(c.Traducir("smartart.addItem"), func() {
		p.cambiar(func() {
			nuevo := rico.ElementoSmart{}
			if n := len(p.sa.Elementos); n > 0 && p.sa.Disposicion == "hierarchy" {
				nuevo.Nivel = p.sa.Elementos[n-1].Nivel
			}
			p.sa.Elementos = append(p.sa.Elementos, nuevo)
		}, true)
	})
	anadir.SinBorde, anadir.Glifo = true, iconos.TrazoEd(iconos.EdMas, 14, "text-muted", v)
	p.cuerpo.Objects = []fyne.CanvasObject{
		filaAjuste(c.Traducir("smartart.layout"), disp),
		ui.NuevoRotulo(c.Traducir(pista), tema.Texto2XS, ui.Normal, "text-muted"),
		container.New(altoMax{alto: tema.Espacio(56), contenido: lista}, container.NewVScroll(lista)),
		container.New(ui.Fila{}, anadir),
	}
	p.cuerpo.Refresh()
}

// fila es un elemento del panel: sangrar y desangrar (en la jerarquía), su texto, subir,
// bajar y quitar, con los mismos límites que la web.
func (p *panelSmart) fila(i int, v tema.Variante) fyne.CanvasObject {
	c, elems := p.c, p.sa.Elementos
	n := i18n.Vars{"n": i + 1}
	boton := func(ic iconos.Ed, clave string, apagado bool, hacer func()) fyne.CanvasObject {
		b := ui.NuevoBotonIcono(0, c.Traducir(clave, n), func() { p.cambiar(hacer, true) })
		b.Glifo = iconos.TrazoEd(ic, iconos.TallaSM, "text-muted", v)
		b.Lado, b.Inerte, b.FondoEncima = ui.LadoIconoSM, apagado, "selected"
		return b
	}
	var objs []fyne.CanvasObject
	jerarquia := p.sa.Disposicion == "hierarchy"
	nivel := elems[i].Nivel
	if jerarquia {
		tope := 1
		if i > 0 {
			tope = elems[i-1].Nivel + 1
		}
		objs = append(objs,
			boton(iconos.EdChevronIzq, "item.outdent", i == 0 || nivel == 0, func() { p.sa.Elementos[i].Nivel-- }),
			boton(iconos.EdChevronDer, "item.indent", i == 0 || nivel >= tope, func() { p.sa.Elementos[i].Nivel++ }),
		)
	}
	e := campoPanel(c.Traducir("slides.smartart.itemText"), 0)
	e.Rotulo = c.Traducir("item.text", n)
	e.PonerTexto(elems[i].Texto)
	e.AlCambiar(func(s string) { p.cambiar(func() { p.sa.Elementos[i].Texto = s }, false) })
	estira := len(objs)
	objs = append(objs, e,
		boton(iconos.EdChevronArriba, "item.moveUp", i == 0, func() { elems[i-1], elems[i] = elems[i], elems[i-1] }),
		boton(iconos.EdChevronAbajo, "item.moveDown", i == len(elems)-1, func() { elems[i], elems[i+1] = elems[i+1], elems[i] }),
		boton(iconos.EdCerrar, "item.remove", len(elems) <= 1, func() { p.sa.Elementos = slices.Delete(slices.Clone(elems), i, i+1) }),
	)
	margen := float32(0)
	if jerarquia {
		margen = float32(nivel) * 12 // `margin-left: level * 12px`
	}
	return container.New(&filaEstirada{hueco: tema.Espacio(1), estira: estira, margen: margen}, objs...)
}

// filaEstirada es `flex items-center gap-N` con un hijo `flex-1`: los demás a su medida
// y ese con lo que sobre, tras su `margin-left`.
type filaEstirada struct {
	hueco  float32
	estira int
	margen float32
}

func (l *filaEstirada) MinSize(objs []fyne.CanvasObject) fyne.Size {
	w, h := l.margen+l.hueco*float32(len(objs)-1), float32(0)
	for _, o := range objs {
		m := o.MinSize()
		w, h = w+m.Width, max(h, m.Height)
	}
	return fyne.NewSize(w, h)
}

func (l *filaEstirada) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	sobra := t.Width - l.MinSize(objs).Width
	x := float32(0)
	for i, o := range objs {
		m := o.MinSize()
		w := m.Width
		if i == l.estira {
			x += l.margen
			w += max(0, sobra)
		}
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(fyne.NewSize(w, m.Height))
		x += w + l.hueco
	}
}

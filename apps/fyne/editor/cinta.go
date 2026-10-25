// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Cinta es la cinta de opciones: la tira de pestañas (32, y 44 con el dedo) y la banda de
// grupos (68, y 53 por debajo de `md`), cada una con su línea de un píxel debajo.
//
// La banda es lo de Ribbon.tsx: UNA línea que se desplaza de lado cuando no cabe —con la
// rueda, el trackpad o el dedo, y con el tabulador, que la lleva hasta el control
// enfocado— y, en escritorio, «Más» al final, que la despliega en varias líneas encima
// del lienzo. Hasta el 2026-10-16 no se desplazaba ni tenía «Más»: a 1280 los grupos
// «Ventana» y «Proteger» de Calcus caían fuera de la ventana sin forma de llegar a ellos,
// y en un teléfono la banda se repartía en tres líneas que se llevaban 368 px.
//
// La tira de pestañas es como la de la web: «Archivo» fijo a la izquierda y las pestañas
// en una fila que también se desliza. Antes solo se corría para enseñar la ACTIVA, así
// que en un teléfono las de detrás de la última visible —Revisar y Vista de Docus a 390—
// no se alcanzaban con el dedo.
type Cinta struct {
	widget.BaseWidget

	ctx      *Contexto
	registro *Registro
	// activa es el índice dentro de las pestañas VISIBLES, no del registro entero:
	// cambiar de tipo de documento cambia cuáles se ven.
	activa int

	archivo          *BotonArchivo
	envolturaArchivo *fyne.Container
	tiras            []*PestanaCinta
	tira             *ui.DesplazaLado
	cuerpo           fyne.CanvasObject
	banda            *ui.DesplazaLado

	// «Más»: su botón, el separador que lo antecede y la banda desplegada, si lo está.
	mas        *ChipCinta
	sepMas     *canvas.Rectangle
	desplegada *ui.Emergente
	// anchoDesplegada es el de la cinta al desplegar: si cambia —la ventana cambió de
	// tamaño—, se pliega, como la web (su posición es fija).
	anchoDesplegada float32
}

var _ fyne.Widget = (*Cinta)(nil)

// NuevaCinta monta la cinta con ese registro. El contexto es el del editor.
func NuevaCinta(ctx *Contexto, r *Registro) *Cinta {
	c := &Cinta{ctx: ctx, registro: r, archivo: NuevoBotonArchivo(ctx)}
	// ArchivoMenu usa un contenedor h-full items-center: el botón se centra en la tira
	// sin su border-b; el menú nace del contenedor entero.
	margen := max((AltoDeTira()-tema.Borde-MedidaBotonArchivo(c.archivo.Rotulo).Height)/2, 0)
	c.envolturaArchivo = container.New(ui.Margen{Arr: margen, Abj: margen}, c.archivo)
	c.archivo.ancla = c.envolturaArchivo
	c.tira = ui.NuevoDesplazaLado(container.NewWithoutLayout(), 0, nil)
	c.banda = ui.NuevoDesplazaLado(container.NewWithoutLayout(), 0, nil)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c.sepMas = canvas.NewRectangle(tema.Color("border", v))
	c.mas = NuevoChipCinta(ctx.Traducir("ribbon.more"), ctx.Traducir("ribbon.moreHint"), c.alternarMas)
	c.mas.Plano = true
	c.ExtendBaseWidget(c)
	c.reconstruir()
	return c
}

// AccessibilityRole la anuncia como contenedor; las pestañas se anuncian solas.
func (c *Cinta) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// Elegir abre la pestaña de ese índice visible, manteniendo una sola fila.
func (c *Cinta) Elegir(i int) {
	vis := c.registro.Visibles(c.ctx.Tipo)
	if len(vis) == 0 {
		return
	}
	c.activa = (i%len(vis) + len(vis)) % len(vis)
	// Otra pestaña: la banda empieza por el principio, como al pintarla de nuevo en la web.
	c.banda.Mostrar(0, 0)
	c.reconstruir()
	c.Refresh()
}

// Alto es lo que ocupa la cinta a ese ancho: la tira y la banda y, por debajo de `md`,
// la tira y la banda estrecha, siempre visible y sin envolver.
func (c *Cinta) Alto(ancho float32) float32 {
	if ancho >= ui.CorteMD || ancho <= 0 {
		return AltoCinta()
	}
	if c.cuerpo == nil {
		return AltoDeTira()
	}
	return AltoDeTira() + AltoBandaEstrecha
}

// Reconstruir rehace la banda con el formato de ahora. Lo llama el marco cuando el
// lienzo dice que la selección cambió: los botones de negrita y alineación se pintan
// pulsados desde `ctx.Lienzo.Formato()`.
//
// Con la banda desplegada, la pliega: se reconstruye porque se ha pulsado una acción, y
// pulsar una acción de la banda desplegada la pliega (Ribbon.tsx, `onPanelClick`). Abrir
// un panel de los suyos —una paleta, una lista— no reconstruye, y no la pliega.
func (c *Cinta) Reconstruir() {
	c.plegarMas()
	c.reconstruir()
	c.Refresh()
}

func (c *Cinta) reconstruir() {
	vis := c.registro.Visibles(c.ctx.Tipo)
	if c.activa >= len(vis) {
		c.activa = 0
	}
	c.tiras = make([]*PestanaCinta, len(vis))
	pestanas := make([]fyne.CanvasObject, len(vis))
	for i, p := range vis {
		t := NuevaPestanaCinta(c.ctx.Traducir(p.Rotulo), func() { c.Elegir(i) })
		t.Activa = i == c.activa
		t.AlMover = func(delta int, extremo bool) {
			if extremo {
				c.Elegir(delta * (len(vis) - 1))
				return
			}
			c.Elegir(i + delta)
		}
		t.AlEnfocar = func() { c.mostrarPestana(t) }
		c.tiras[i] = t
		pestanas[i] = t
	}
	c.tira.Contenido = container.New(ui.Fila{Hueco: HuecoTira}, pestanas...)
	c.cuerpo = nil
	if len(vis) > 0 && vis[c.activa].Cuerpo != nil {
		c.cuerpo = vis[c.activa].Cuerpo(c.ctx)
	}
	if c.cuerpo != nil {
		// `px-2` dentro de lo que se desplaza: el primer grupo empieza a 8 y, desplazada
		// hasta el fondo, el último acaba a 8 del borde.
		c.banda.Contenido = container.New(ui.Margen{Izq: MargenBanda, Der: MargenBanda}, c.cuerpo)
		seguirFoco(c.cuerpo, c.mostrarControl)
	}
}

// mostrarPestana desliza la tira para que esa pestaña se vea entera.
func (c *Cinta) mostrarPestana(p *PestanaCinta) {
	c.tira.Mostrar(p.Position().X, p.Size().Width)
}

// mostrarControl desliza la banda hasta el control enfocado con el tabulador.
func (c *Cinta) mostrarControl(o fyne.CanvasObject) {
	d := fyne.CurrentApp().Driver()
	x := d.AbsolutePositionForObject(o).X - d.AbsolutePositionForObject(c.banda.Contenido).X
	c.banda.Mostrar(x, o.Size().Width)
}

// seguirFoco engancha el foco de cada control de la banda para llevarlo a la vista.
func seguirFoco(o fyne.CanvasObject, mostrar func(fyne.CanvasObject)) {
	switch x := o.(type) {
	case *fyne.Container:
		for _, h := range x.Objects {
			seguirFoco(h, mostrar)
		}
	case *Grupo:
		for _, h := range x.Controles {
			seguirFoco(h, mostrar)
		}
	case interface{ pulsableDeCinta() *ui.Pulsable }:
		p := x.pulsableDeCinta()
		previo := p.AlEnfocar
		p.AlEnfocar = func() {
			if previo != nil {
				previo()
			}
			mostrar(o)
		}
	}
}

// pulsableDeCinta da la cáscara de un control de la cinta, para engancharle el foco.
func (a *activable) pulsableDeCinta() *ui.Pulsable { return &a.Pulsable }

// alternarMas despliega la banda en varias líneas sobre el lienzo, o la pliega.
//
// Lo desplegado es OTRA copia de la pestaña, hecha con el mismo cuerpo: en la web es la
// misma banda —el estado de sus controles no se duplica— y aquí cada cuerpo es una
// función del contexto, así que dos copias no pueden decir cosas distintas.
func (c *Cinta) alternarMas() {
	if c.plegarMas() || ui.EsTactil() || (c.Size().Width > 0 && c.Size().Width < ui.CorteMD) {
		return
	}
	vis := c.registro.Visibles(c.ctx.Tipo)
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(c)
	if lienzo == nil || len(vis) == 0 || vis[c.activa].Cuerpo == nil {
		return
	}
	ancho := lienzo.Size().Width
	estrecha := ancho < ui.CorteMD
	cuerpo := vis[c.activa].Cuerpo(c.ctx)
	objs := objetosDeBanda(cuerpo)
	for _, o := range objs {
		switch g := o.(type) {
		case *Grupo:
			g.Desplegado, g.AlPrincipio = true, estrecha
		case *separador:
			g.Hide() // desplegada, cada grupo lleva su rótulo y un separador al final de una línea no separa nada
		}
	}
	zona := c.anchoZonaMas()
	lineas := container.New(GruposEnLineas{Ancho: max(ancho-MargenBanda-zona, 0)}, objs...)
	cierre := NuevoChipCinta(c.mas.Texto, c.mas.Rotulo, c.alternarMas)
	cierre.Plano, cierre.Abierto = true, true
	contenido := container.New(&bandaDesplegada{zona: zona, arriba: c.arribaFila(estrecha)}, lineas, cierre)
	y := fyne.CurrentApp().Driver().AbsolutePositionForObject(c).Y + AltoDeTira() - tema.Borde
	c.desplegada = ui.MostrarEmergenteASangre(lienzo, contenido, func(ventana, tam fyne.Size) (fyne.Position, fyne.Size) {
		// `max-h-[60dvh]`, con el contenido desplazable si no cabe.
		return fyne.NewPos(0, y), fyne.NewSize(ventana.Width, min(tam.Height, ventana.Height*0.6))
	})
	c.desplegada.Rotulo = c.mas.Rotulo
	c.desplegada.AlRecogerse = func() {
		c.desplegada = nil
		c.mas.Abierto = false
		c.mas.Refresh()
	}
	c.anchoDesplegada = c.Size().Width
	c.mas.Abierto = true
	c.mas.Refresh()
}

// plegarMas pliega la banda desplegada, si lo está, y dice si lo estaba.
func (c *Cinta) plegarMas() bool {
	if c.desplegada == nil || !c.desplegada.Abierto() {
		return false
	}
	c.desplegada.Cerrar()
	return true
}

// objetosDeBanda son los grupos y separadores de un cuerpo de pestaña.
func objetosDeBanda(cuerpo fyne.CanvasObject) []fyne.CanvasObject {
	if cont, ok := cuerpo.(*fyne.Container); ok {
		return cont.Objects
	}
	return []fyne.CanvasObject{cuerpo}
}

// anchoZonaMas es lo que ocupa «Más» al final de la fila: su separador (`mx-1.5`), el
// botón y el `pr-2`.
func (c *Cinta) anchoZonaMas() float32 {
	return 2*MargenSeparador + tema.Borde + c.mas.MinSize().Width + MargenBanda
}

// arribaFila es dónde empieza la fila de controles de un grupo: `pt-1.5`, o `py-1` por
// debajo de `md`.
func (c *Cinta) arribaFila(estrecha bool) float32 {
	if estrecha {
		return ArribaGrupoEstrecho
	}
	return ArribaGrupo
}

// AltoCinta es lo que ocupa la cinta entera: tira + banda, cada una con su línea.
func AltoCinta() float32 { return AltoDeTira() + AltoBanda }

// MinSize es la medida declarada.
func (c *Cinta) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	return fyne.NewSize(0, AltoCinta())
}

// CreateRenderer compone los dos fondos, sus líneas, las pestañas y el cuerpo.
func (c *Cinta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &cintaRenderer{
		c:      c,
		fondo:  canvas.NewRectangle(tema.Color("surface", v)),
		lineaT: canvas.NewRectangle(tema.Color("border", v)),
		lineaB: canvas.NewRectangle(tema.Color("border", v)),
	}
	return r
}

type cintaRenderer struct {
	c              *Cinta
	fondo          *canvas.Rectangle
	lineaT, lineaB *canvas.Rectangle
}

func (r *cintaRenderer) MinSize() fyne.Size { return r.c.MinSize() }

func (r *cintaRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{
		r.fondo, r.lineaT, r.lineaB, r.c.envolturaArchivo, r.c.tira,
		r.c.banda, r.c.sepMas, r.c.mas,
	}
}

func (r *cintaRenderer) Destroy() {}

func (r *cintaRenderer) Layout(t fyne.Size) {
	c := r.c
	tira := AltoDeTira()
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	// La línea de la tira va DENTRO de su alto, que es lo que hace `border-b` con
	// `box-sizing: border-box`: por eso las pestañas miden 31 y no 32.
	r.lineaT.Move(fyne.NewPos(0, tira-tema.Borde))
	r.lineaT.Resize(fyne.NewSize(t.Width, tema.Borde))

	estrecha := t.Width > 0 && t.Width < ui.CorteMD
	if c.desplegada != nil && (t.Width != c.anchoDesplegada || ui.EsTactil()) {
		c.plegarMas()
	}
	// «Archivo» fijo y todas las pestañas en una tira deslizable. La banda
	// móvil permanece visible; no reserva un botón para expandirla.
	fin := t.Width - MargenTira
	ma := c.archivo.MinSize()
	c.envolturaArchivo.Move(fyne.NewPos(MargenTira, 0))
	c.envolturaArchivo.Resize(fyne.NewSize(ma.Width, tira-tema.Borde))
	x := MargenTira + ma.Width + HuecoTira
	c.tira.Move(fyne.NewPos(x, 0))
	c.tira.Resize(fyne.NewSize(max(fin-x, 0), tira-tema.Borde))
	// Tras reconstruir, las pestañas son otras y aún no están colocadas: se colocan antes de
	// llevar la activa a la vista (un Resize al mismo tamaño no recoloca nada).
	c.tira.Refresh()
	if a := c.activaTira(); a != nil {
		c.mostrarPestana(a)
	}

	c.sepMas.Hide()
	c.mas.Hide()
	if c.cuerpo == nil {
		c.banda.Hide()
		r.lineaB.Hide()
		return
	}
	c.banda.Show()
	r.lineaB.Show()
	alto := AltoBanda
	if estrecha {
		alto = AltoBandaEstrecha
	}
	r.lineaB.Move(fyne.NewPos(0, tira+alto-tema.Borde))
	r.lineaB.Resize(fyne.NewSize(t.Width, tema.Borde))
	ponerEstrecha(c.cuerpo, estrecha)
	ancho := t.Width
	// «Más» solo si algo no cabe, y OCUPA SU SITIO en la fila: la banda acaba donde él
	// empieza, a la altura de la fila de controles y con su separador delante.
	if !estrecha && !ui.EsTactil() && (c.banda.Contenido.MinSize().Width > t.Width+1 || c.desplegada != nil) {
		ancho = t.Width - c.anchoZonaMas()
		arribaSep := ArribaSeparador
		c.sepMas.Show()
		c.sepMas.Move(fyne.NewPos(ancho+MargenSeparador, tira+arribaSep))
		c.sepMas.Resize(fyne.NewSize(tema.Borde, AltoSeparadorGrupos))
		m := c.mas.MinSize()
		c.mas.Show()
		c.mas.Move(fyne.NewPos(ancho+2*MargenSeparador+tema.Borde, tira+c.arribaFila(estrecha)+(AltoFilaGrupo-m.Height)/2))
		c.mas.Resize(m)
	}
	c.banda.Move(fyne.NewPos(0, tira))
	c.banda.Resize(fyne.NewSize(max(ancho, 0), alto-tema.Borde))
}

// activaTira es la pestaña activa, si hay.
func (c *Cinta) activaTira() *PestanaCinta {
	if c.activa < len(c.tiras) {
		return c.tiras[c.activa]
	}
	return nil
}

// ponerEstrecha le dice a los grupos y separadores de la banda si la ventana está por
// debajo de `md`: alineados al principio, sin rótulo y con la fila más arriba.
func ponerEstrecha(cuerpo fyne.CanvasObject, estrecha bool) {
	for _, o := range objetosDeBanda(cuerpo) {
		switch g := o.(type) {
		case *Grupo:
			if g.AlPrincipio != estrecha {
				g.AlPrincipio = estrecha
				g.Refresh()
			}
		case *separador:
			if g.estrecho != estrecha {
				g.estrecho = estrecha
				g.Refresh()
			}
		}
	}
}

func (r *cintaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("surface", v)
	r.lineaT.FillColor = tema.Color("border", v)
	r.lineaB.FillColor = tema.Color("border", v)
	r.c.sepMas.FillColor = tema.Color("border", v)
	r.fondo.Refresh()
	r.lineaT.Refresh()
	r.lineaB.Refresh()
	r.c.sepMas.Refresh()
	// Reconstruir cambia la banda por una nueva, que hay que COLOCAR: Fyne solo llama a
	// Layout cuando cambia el tamaño, y sin esto la banda nueva se quedaba en 0 × 0 —
	// invisible— hasta que alguien redimensionara la ventana.
	r.Layout(r.c.Size())
	r.c.tira.Refresh()
	r.c.banda.Refresh()
	canvas.Refresh(r.c)
}

// FilaGrupos es el layout de la banda: los grupos uno detrás de otro, cada uno a su
// medida y con todo el alto de la banda —un grupo es `h-full` y su rótulo se pega abajo
// con `mt-auto`—. Es `Fila` con hueco cero, porque el aire de un grupo es su propio
// `px-1.5`. Nunca envuelve: lo que no cabe se alcanza desplazando la banda o con «Más».
//
// Un separador no se pinta al principio, al final ni detrás de otro separador (el
// `first:hidden last:hidden [[data-separador]+&]:hidden` de RibbonSeparator): un grupo que
// no se monta deja juntos a los dos separadores que lo rodeaban.
type FilaGrupos struct{}

var _ fyne.Layout = FilaGrupos{}

// MinSize suma los anchos de lo que se ve.
func (FilaGrupos) MinSize(objs []fyne.CanvasObject) fyne.Size {
	ajustarSeparadores(objs)
	var ancho, alto float32
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		ancho += m.Width
		alto = max(alto, m.Height)
	}
	return fyne.NewSize(ancho, alto)
}

// Layout coloca los grupos de izquierda a derecha.
func (FilaGrupos) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	ajustarSeparadores(objs)
	var x float32
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		o.Move(fyne.NewPos(x, 0))
		o.Resize(fyne.NewSize(m.Width, t.Height))
		x += m.Width
	}
}

// ajustarSeparadores esconde los separadores que sobran: el primero, el último y el que
// va detrás de otro.
func ajustarSeparadores(objs []fyne.CanvasObject) {
	for i, o := range objs {
		s, ok := o.(*separador)
		if !ok {
			continue
		}
		trasOtro := false
		if i > 0 {
			_, trasOtro = objs[i-1].(*separador)
		}
		if i == 0 || i == len(objs)-1 || trasOtro {
			s.Hide()
		} else {
			s.Show()
		}
	}
}

// AltoGrupo es lo que mide un grupo: la banda menos su línea.
func AltoGrupo() float32 { return AltoBanda - tema.Borde }

// HuecoLineasGrupos es el `gap-y-1` entre las líneas de la banda desplegada.
var HuecoLineasGrupos = tema.Espacio(1)

// GruposEnLineas es la banda desplegada con «Más» (`flex-wrap` en Ribbon.tsx): los grupos
// en líneas de `Ancho`, cada uno a su medida o, si mide más que la línea entera, a la
// medida de la línea y con sus controles repartidos (`max-w-full`).
type GruposEnLineas struct{ Ancho float32 }

var _ fyne.Layout = GruposEnLineas{}

// medida es lo que ocupa un objeto en la línea.
func (l GruposEnLineas) medida(o fyne.CanvasObject) fyne.Size {
	m := o.MinSize()
	if g, ok := o.(*Grupo); ok {
		w := min(m.Width, l.Ancho)
		return fyne.NewSize(w, AltoGrupoEn(g, w))
	}
	return fyne.NewSize(min(m.Width, l.Ancho), m.Height)
}

// lineas reparte lo visible en líneas de `Ancho`.
func (l GruposEnLineas) lineas(objs []fyne.CanvasObject) [][]fyne.CanvasObject {
	var lineas [][]fyne.CanvasObject
	var cur []fyne.CanvasObject
	var x float32
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		w := l.medida(o).Width
		if x+w > l.Ancho+0.01 && len(cur) > 0 {
			lineas = append(lineas, cur)
			cur, x = nil, 0
		}
		cur = append(cur, o)
		x += w
	}
	if len(cur) > 0 {
		lineas = append(lineas, cur)
	}
	return lineas
}

func (l GruposEnLineas) altoLinea(linea []fyne.CanvasObject) float32 {
	var h float32
	for _, o := range linea {
		h = max(h, l.medida(o).Height)
	}
	return h
}

// MinSize es el ancho dado y la suma de las líneas.
func (l GruposEnLineas) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var alto float32
	for i, linea := range l.lineas(objs) {
		if i > 0 {
			alto += HuecoLineasGrupos
		}
		alto += l.altoLinea(linea)
	}
	return fyne.NewSize(l.Ancho, alto)
}

// Layout coloca las líneas una debajo de otra.
func (l GruposEnLineas) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	var y float32
	for _, linea := range l.lineas(objs) {
		var x float32
		for _, o := range linea {
			m := l.medida(o)
			o.Move(fyne.NewPos(x, y))
			o.Resize(m)
			x += m.Width
		}
		y += l.altoLinea(linea) + HuecoLineasGrupos
	}
}

// bandaDesplegada coloca las líneas de grupos con el `px-2 py-1 pr-24` de la banda
// desplegada y, en la esquina, el «Más» que la pliega, en el MISMO sitio que el de la
// banda plegada (en la web es el mismo botón, que queda encima).
type bandaDesplegada struct {
	zona   float32 // lo que ocupa «Más» a la derecha
	arriba float32 // dónde empieza la fila de controles
}

var _ fyne.Layout = (*bandaDesplegada)(nil)

func (b *bandaDesplegada) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(MargenBanda+m.Width+b.zona, m.Height+2*tema.Espacio(1))
}

func (b *bandaDesplegada) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	lineas, mas := objs[0], objs[1]
	lineas.Move(fyne.NewPos(MargenBanda, tema.Espacio(1)))
	lineas.Resize(lineas.MinSize())
	m := mas.MinSize()
	// Lo de dentro empieza tras el borde de un píxel del emergente, que va de borde a borde
	// de la ventana y con su línea de arriba sobre la de la tira: así «Más» cae donde el de
	// la banda plegada, en x e y.
	mas.Move(fyne.NewPos(t.Width+2*tema.Borde-b.zona+2*MargenSeparador, b.arriba+(AltoFilaGrupo-m.Height)/2))
	mas.Resize(m)
}

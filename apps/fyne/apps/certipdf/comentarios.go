// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image/color"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

/*
El panel COMENTARIOS (`Comentarios` de `Paneles.tsx`): los comentarios del documento en
lista, con sus respuestas DENTRO —es lo que convierte una lista de notas en una
conversación que se sigue—, su estado de revisión y las cuatro acciones.

Los índices que se mandan al motor son los REALES de la lista del documento: filtrar la
vista no puede cambiar a qué comentario apunta responder o borrar.
*/

// tiposConocidos son los subtipos de anotación que el catálogo sabe nombrar
// (`TIPOS_CONOCIDOS`). Uno que no esté —los hay propios de un fabricante— se nombra como
// comentario, que es mejor que una clave sin traducir.
var tiposConocidos = map[string]bool{
	"Highlight": true, "Underline": true, "StrikeOut": true, "Squiggly": true, "Text": true, "Ink": true,
	"Square": true, "Circle": true, "Line": true, "Polygon": true, "FreeText": true, "Stamp": true,
}

func claveDeTipo(tipo string) string {
	if tiposConocidos[tipo] {
		return "pdf.kind." + tipo
	}
	return "pdf.kind.Text"
}

// estadosRevision son las opciones del `<select>`, en su orden. En INGLÉS, porque es lo
// que se escribe en /State y lo que lee Acrobat.
var estadosRevision = []string{"None", "Accepted", "Rejected", "Cancelled", "Completed"}

// HilosVisibles son los comentarios que abren hilo y se ven con ese filtro: los que casan
// ellos mismos o por una de sus respuestas —buscar una palabra no puede esconder el hilo
// donde se dijo—. Devuelve los índices REALES y cuántos hilos hay en total.
func HilosVisibles(anots []AnotacionLeida, busqueda string, soloAbiertos bool) (visibles []int, hilos int) {
	replicas := map[int][]int{}
	for i, a := range anots {
		if a.RespuestaA != nil {
			replicas[*a.RespuestaA] = append(replicas[*a.RespuestaA], i)
		}
	}
	q := buscar.Plegar(strings.TrimSpace(busqueda))
	casa := func(a AnotacionLeida) bool {
		if soloAbiertos && (a.Estado == "Accepted" || a.Estado == "Completed") {
			return false
		}
		return q == "" || strings.Contains(buscar.Plegar(a.Texto), q) || strings.Contains(buscar.Plegar(a.Autor), q)
	}
	for i, a := range anots {
		if a.RespuestaA != nil {
			continue
		}
		hilos++
		ok := casa(a)
		for _, j := range replicas[i] {
			ok = ok || casa(anots[j])
		}
		if ok {
			visibles = append(visibles, i)
		}
	}
	return visibles, hilos
}

// panelComentarios es el estado del panel.
type panelComentarios struct {
	l            *PanelLateral
	filtro       *ui.Entrada
	busqueda     string
	soloAbiertos bool
	respondiendo int // -1: ninguno
	replica      *ui.AreaTexto
	vista        *fyne.Container
}

func (l *PanelLateral) comentarios() fyne.CanvasObject {
	if len(l.c.Anotaciones) == 0 {
		return parrafoVacio(l.t("pdf.comments.empty", nil), anchoUtil())
	}
	if l.comentariosP == nil {
		c := &panelComentarios{l: l, respondiendo: -1}
		c.filtro = nuevoCampoPanel(l.t("pdf.comments.filter", nil), tema.Espacio(1), 0.25*tema.Rem, anchoUtil())
		c.filtro.Busqueda = true // `type="search"`
		c.filtro.AlCambiar(func(s string) { c.busqueda = s; c.rehacer() })
		c.vista = container.New(ui.Columna{})
		l.comentariosP = c
	}
	l.comentariosP.rehacer()
	return l.comentariosP.vista
}

func (c *panelComentarios) t(clave string) string { return c.l.t(clave, nil) }

func (c *panelComentarios) rehacer() {
	l := c.l
	anots := l.c.Anotaciones
	visibles, hilos := HilosVisibles(anots, c.busqueda, c.soloAbiertos)
	// `mb-1.5 flex flex-col gap-1`: el filtro, «solo sin resolver» y cuántos se ven.
	cabeza := []fyne.CanvasObject{c.filtro, nuevaCasillaNativa(c.t("pdf.comments.onlyOpen"), c.soloAbiertos, func(v bool) {
		c.soloAbiertos = v
		c.rehacer()
	})}
	if len(visibles) != hilos {
		cabeza = append(cabeza, ui.NuevoRotulo(l.t("pdf.comments.showing", i18n.Vars{
			"n": strconv.Itoa(len(visibles)), "total": strconv.Itoa(hilos),
		}), tema.Texto2XS, ui.Normal, "text-subtle"))
	}
	fichas := make([]fyne.CanvasObject, 0, len(visibles))
	for _, i := range visibles {
		fichas = append(fichas, c.ficha(i))
	}
	aplanar := botonPanel(c.t("pdf.comments.flatten"), tema.TextoXS, tema.Espacio(1.5), c.aplanar)
	aplanar.Inerte = l.trabajando()
	c.vista.Objects = []fyne.CanvasObject{
		container.New(ui.Margen{Abj: tema.Espacio(1.5)}, container.New(ui.Columna{Hueco: tema.Espacio(1)}, cabeza...)),
		container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, fichas...),
		container.New(ui.Margen{Arr: tema.Espacio(2)}, container.New(alPrincipio{}, aplanar)),
	}
	c.vista.Refresh()
}

// ficha es el `<li class="border-border rounded-md border p-2">` de un hilo.
func (c *panelComentarios) ficha(i int) fyne.CanvasObject {
	l := c.l
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	a := l.c.Anotaciones[i]
	dentro := anchoUtil() - 2*tema.Borde - 2*tema.Espacio(2)
	texto := dentro - tema.Espacio(2.5) - tema.Espacio(2)

	autor := a.Autor
	if autor == "" {
		autor = c.t("pdf.comments.anonymous")
	}
	cuerpo := a.Texto
	if cuerpo == "" {
		cuerpo = c.t(claveDeTipo(a.Tipo))
	}
	columna := []fyne.CanvasObject{
		ui.NuevoParrafo(autor+" · "+l.t("pdf.page.n", i18n.Vars{"n": a.Pagina + 1}), tema.Texto2XS, ui.Normal, "text-muted", v).A(texto),
		container.New(ui.Margen{Arr: tema.Espacio(0.5)}, ui.NuevoParrafo(cuerpo, tema.TextoXS, ui.Normal, "text", v).A(texto)),
	}
	// El estado va junto al comentario: revisar cuarenta es mirar cuáles quedan.
	if a.Estado != "" && a.Estado != "None" {
		fondo, tinta := "bg-subtle", "text-muted"
		if a.Estado == "Accepted" || a.Estado == "Completed" {
			fondo, tinta = "success-bg", "success-text"
		}
		columna = append(columna, container.New(ui.Margen{Arr: tema.Espacio(1)}, container.New(alPrincipio{},
			insigniaEstado(c.t("pdf.comments.state."+a.Estado), fondo, tinta))))
	}
	cabeza := container.New(conPunto{}, puntoDeColor(a.Color), container.New(ui.Columna{}, columna...))

	idx := i
	ir := enlaceBoton(c.t("pdf.comments.goto"), "primary-text", func() {
		if l.AlIrA != nil {
			l.AlIrA(a.Pagina + 1)
		}
	})
	borrar := enlaceBoton(c.t("pdf.comments.delete"), "danger-text", func() {
		c.aplicar(api.PDFCambio{Op: "borrar-anotacion", Campos: map[string]string{"indices": strconv.Itoa(idx)}},
			"pdf.saved.annotated", nil)
	})
	borrar.Inerte = l.trabajando()
	responder := enlaceBoton(c.t("pdf.comments.reply"), "primary-text", func() {
		if c.respondiendo == idx {
			c.respondiendo = -1
		} else {
			c.respondiendo = idx
		}
		c.replica = nil
		c.rehacer()
	})
	responder.Inerte = l.trabajando()
	estado := c.selectorEstado(a, idx)
	acciones := container.New(filaAcciones{}, ir, borrar, responder, estado)

	partes := []fyne.CanvasObject{cabeza, container.New(ui.Margen{Arr: tema.Espacio(1.5)}, acciones)}
	for _, r := range l.c.Anotaciones {
		if r.RespuestaA == nil || *r.RespuestaA != i {
			continue
		}
		partes = append(partes, container.New(ui.Margen{Arr: tema.Espacio(1.5)}, respuesta(c.nombreDe(r), r.Texto, dentro)))
	}
	if c.respondiendo == i {
		partes = append(partes, container.New(ui.Margen{Arr: tema.Espacio(1.5)}, c.formularioRespuesta(idx, dentro)))
	}
	return nuevoRecuadroDe(container.New(ui.Columna{}, partes...), anchoUtil())
}

func (c *panelComentarios) nombreDe(a AnotacionLeida) string {
	if a.Autor != "" {
		return a.Autor
	}
	return c.t("pdf.comments.anonymous")
}

// selectorEstado es el `<select>` del estado: `border-border bg-surface text-text-muted
// text-2xs rounded border px-1 py-0.5`, con la flecha del navegador.
func (c *panelComentarios) selectorEstado(a AnotacionLeida, idx int) fyne.CanvasObject {
	opciones := make([]string, len(estadosRevision))
	elegida := 0
	for k, e := range estadosRevision {
		opciones[k] = c.t("pdf.comments.state." + e)
		if e == a.Estado {
			elegida = k
		}
	}
	s := ui.NuevoSelector(opciones, func(k int) {
		// 1-based: es lo que el motor espera y lo que el usuario ve.
		c.aplicar(api.PDFCambio{Op: "marcar", Campos: map[string]string{
			"indice": strconv.Itoa(idx + 1), "estado": estadosRevision[k],
		}}, "pdf.saved.marked", nil)
	})
	s.Elegida = elegida
	s.Propio = &ui.SelectorPropio{
		Campo: ui.CampoPropio{
			MargenX: tema.Espacio(1), MargenY: tema.Espacio(0.5), Texto: tema.Texto2XS,
			Borde: "border", Radio: 0.25 * tema.Rem, Tinta: "text-muted",
		},
		Alto: ui.AltoSelectNativo2XS,
	}
	s.Inerte(c.l.trabajando())
	return s
}

// formularioRespuesta es el `<form class="mt-1.5 flex flex-col gap-1">`: dos filas de
// texto y «Enviar».
func (c *panelComentarios) formularioRespuesta(idx int, ancho float32) fyne.CanvasObject {
	if c.replica == nil {
		c.replica = ui.NuevaAreaTexto(c.t("pdf.comments.replyPlaceholder"))
		c.replica.Filas = 2
		c.replica.ConPropia(ui.CampoPropio{MargenX: tema.Espacio(1.5), MargenY: tema.Espacio(1), Texto: tema.TextoXS, Borde: "border"})
		c.replica.Radio, c.replica.Ancho = 0.25*tema.Rem, ancho
		replica := c.replica
		fyne.Do(func() {
			if lz := fyne.CurrentApp().Driver().CanvasForObject(replica); lz != nil {
				lz.Focus(replica)
			}
		})
	}
	area := c.replica
	enviar := botonPrimario(c.t("pdf.comments.send"), tema.Texto2XS, tema.Espacio(2), tema.Espacio(1), nil)
	enviar.Radio = 0.25 * tema.Rem
	enviar.AlPulsar = func() {
		texto := strings.TrimSpace(area.Texto())
		if texto == "" {
			return
		}
		c.aplicar(api.PDFCambio{Op: "responder", Campos: map[string]string{
			"indice": strconv.Itoa(idx + 1), "texto": texto,
		}}, "pdf.saved.replied", func(ok bool) {
			if ok {
				c.respondiendo, c.replica = -1, nil
			}
		})
	}
	enviar.Inerte = c.l.trabajando() || strings.TrimSpace(area.Texto()) == ""
	area.AlCambiar(func(s string) {
		inerte := c.l.trabajando() || strings.TrimSpace(s) == ""
		if enviar.Inerte != inerte {
			enviar.Inerte = inerte
			enviar.Refresh()
		}
	})
	return container.New(ui.Columna{Hueco: tema.Espacio(1)}, area, container.New(alPrincipio{}, enviar))
}

// aplanar funde las anotaciones en la página. No se deshace: se pregunta antes, con el
// diálogo de la casa.
func (c *panelComentarios) aplanar() {
	p := c.l.pant
	if p == nil {
		return
	}
	p.confirmar(c.t("pdf.comments.flatten.confirm"), c.t("pdf.comments.flatten"), func() {
		p.Aplicar(api.PDFCambio{Op: "aplanar-anotaciones", Sufijo: c.t("pdf.suffix.flattened")},
			c.t("pdf.saved.flattened"), nil)
	})
}

func (c *panelComentarios) aplicar(cambio api.PDFCambio, mensaje string, hecho func(bool)) {
	p := c.l.pant
	if p == nil {
		return
	}
	p.Aplicar(cambio, c.t(mensaje), hecho)
}

// ---- piezas de este panel ----

// puntoDeColor es el `mt-1 h-2.5 w-2.5 rounded-full` con el color de la anotación
// (`rgbAColor`: ámbar si no trae ninguno).
func puntoDeColor(c *[3]float64) fyne.CanvasObject {
	tinta := color.NRGBA{R: 0xf9, G: 0xb9, B: 0x23, A: 255}
	if c != nil {
		v := func(x float64) uint8 { return uint8(min(max(x*255+0.5, 0), 255)) }
		tinta = color.NRGBA{R: v(c[0]), G: v(c[1]), B: v(c[2]), A: 255}
	}
	circulo := canvas.NewCircle(tinta)
	lado := tema.Espacio(2.5)
	return container.New(cajaFija{tam: fyne.NewSize(lado, lado), arriba: tema.Espacio(1)}, circulo)
}

// cajaFija da a su hijo un tamaño fijo, `arriba` más abajo del origen.
type cajaFija struct {
	tam    fyne.Size
	arriba float32
}

func (c cajaFija) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(c.tam.Width, c.tam.Height+c.arriba)
}

func (c cajaFija) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	objs[0].Move(fyne.NewPos(0, c.arriba))
	objs[0].Resize(c.tam)
}

// conPunto coloca [punto, columna] con `flex items-start gap-2`.
type conPunto struct{}

func (conPunto) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(a.Width+tema.Espacio(2)+b.Width, max(a.Height, b.Height))
}

func (conPunto) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	a := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(a)
	x := a.Width + tema.Espacio(2)
	objs[1].Move(fyne.NewPos(x, 0))
	objs[1].Resize(fyne.NewSize(max(t.Width-x, 0), objs[1].MinSize().Height))
}

// filaAcciones es `flex gap-2` con el `<select>` en `ml-auto`: los enlaces a la
// izquierda, el selector pegado a la derecha, y todos al alto del más alto (`stretch`).
type filaAcciones struct{}

func (filaAcciones) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, o := range objs {
		m := o.MinSize()
		if i > 0 {
			ancho += tema.Espacio(2)
		}
		ancho += m.Width
		alto = max(alto, m.Height)
	}
	return fyne.NewSize(ancho, alto)
}

func (f filaAcciones) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	alto := f.MinSize(objs).Height
	var x float32
	for i, o := range objs {
		m := o.MinSize()
		if i == len(objs)-1 {
			o.Move(fyne.NewPos(t.Width-m.Width, 0))
			o.Resize(fyne.NewSize(m.Width, alto))
			return
		}
		o.Move(fyne.NewPos(x, 0))
		o.Resize(fyne.NewSize(m.Width, alto))
		x += m.Width + tema.Espacio(2)
	}
}

// respuesta es una réplica dentro de su hilo: `border-border ml-3 border-l pl-2`, el
// autor en `text-2xs text-text-muted` y el texto en `text-xs mt-0.5`.
func respuesta(autor, texto string, ancho float32) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	dentro := ancho - tema.Espacio(3) - tema.Borde - tema.Espacio(2)
	partes := []fyne.CanvasObject{ui.NuevoParrafo(autor, tema.Texto2XS, ui.Normal, "text-muted", v).A(dentro)}
	abajo := float32(0)
	if texto != "" {
		partes = append(partes, container.New(ui.Margen{Arr: tema.Espacio(0.5)}, ui.NuevoParrafo(texto, tema.TextoXS, ui.Normal, "text", v).A(dentro)))
	} else {
		// Una réplica sin texto deja su `<p>` vacío, y el `mt-0.5` de un bloque vacío se
		// sale de la réplica por abajo (los márgenes colapsan a través de él): la línea
		// del borde mide lo que el autor, y la ficha crece 1,75 igual. Medido: 90,5.
		abajo = tema.Espacio(0.5)
	}
	return container.New(ui.Margen{Izq: tema.Espacio(3), Abj: abajo}, container.New(conLineaIzquierda{sep: tema.Espacio(2)},
		canvas.NewRectangle(tema.Color("border", v)), container.New(ui.Columna{}, partes...)))
}

// conLineaIzquierda coloca [línea, contenido]: la línea de 1 px a la izquierda, a todo el
// alto, y el contenido `sep` más allá.
type conLineaIzquierda struct{ sep float32 }

func (c conLineaIzquierda) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[1].MinSize()
	return fyne.NewSize(m.Width+tema.Borde+c.sep, m.Height)
}

func (c conLineaIzquierda) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(tema.Borde, t.Height))
	objs[1].Move(fyne.NewPos(tema.Borde+c.sep, 0))
	objs[1].Resize(fyne.NewSize(t.Width-tema.Borde-c.sep, t.Height))
}

// insigniaEstado es `text-2xs rounded px-1.5 py-0.5` con su fondo y su tinta. No se
// pulsa: es una etiqueta.
func insigniaEstado(texto, fondo, tinta string) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f := canvas.NewRectangle(tema.Color(fondo, v))
	f.CornerRadius = 0.25 * tema.Rem
	return container.NewStack(f, container.New(ui.Margen{Izq: tema.Espacio(1.5), Arr: tema.Espacio(0.5), Der: tema.Espacio(1.5), Abj: tema.Espacio(0.5)},
		ui.NuevoRotulo(texto, tema.Texto2XS, ui.Normal, tinta)))
}

// ---- La casilla nativa ----

// casillaNativa es `<label class="text-text-muted text-2xs flex items-center gap-1.5">`
// con un `<input type="checkbox">` sin estilo: el del navegador. Chromium lo pinta de
// 13×13 con un borde de 1 px gris (#767676), esquinas de 2 y, marcado, el azul de su
// `accent-color` por defecto (#0075ff) con la marca blanca. Son los colores DEL
// NAVEGADOR y por eso no son tokens: la suite no le pone `accent-color`.
type casillaNativa struct {
	ui.Pulsable
	rotulo    string
	marcada   bool
	AlCambiar func(bool)
	// esc, hueco y tinta son los del rótulo: `text-2xs gap-1.5 text-text-muted` aquí,
	// `text-xs gap-2 text-text` en la casilla de un campo (ver [nuevaCasillaDeCampo]).
	esc   tema.Escalon
	hueco float32
	tinta string
	// ancho fija el de la `<label>` entera cuando una fila `flex` la encoge: el rótulo
	// se parte en renglones y la casilla va centrada en su alto. 0 es a su medida.
	ancho float32
}

// lineasCasilla es el rótulo repartido en lo que deja el ancho fijo, o entero.
func (c *casillaNativa) lineasCasilla() []string {
	if c.ancho <= 0 {
		return []string{c.rotulo}
	}
	return ui.Repartir(c.rotulo, c.esc, ui.Normal, c.ancho-ladoCasillaNativa-c.hueco)
}

func nuevaCasillaNativa(rotulo string, marcada bool, alCambiar func(bool)) *casillaNativa {
	c := &casillaNativa{
		rotulo: rotulo, marcada: marcada, AlCambiar: alCambiar,
		esc: tema.Texto2XS, hueco: tema.Espacio(1.5), tinta: "text-muted",
	}
	c.Nombre = rotulo
	c.AlPulsar = func() {
		c.marcada = !c.marcada
		c.Refresh()
		if c.AlCambiar != nil {
			c.AlCambiar(c.marcada)
		}
	}
	c.ExtendBaseWidget(c)
	return c
}

// ladoCasillaNativa es el de Chromium sin tamaño CSS: los Checkbox de Paneles.tsx
// pasan `class="accent-primary"`, que sin `label` sustituye el `h-4 w-4` de Form.tsx.
// El alto de la fila es la caja de línea del rótulo.
const ladoCasillaNativa float32 = 13

func (c *casillaNativa) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	alto := max(float32(len(c.lineasCasilla()))*c.esc.Linea, ladoCasillaNativa)
	if c.ancho > 0 {
		return fyne.NewSize(c.ancho, alto)
	}
	return fyne.NewSize(ladoCasillaNativa+c.hueco+ui.Medir(c.rotulo, c.esc, ui.Normal).Width, alto)
}

func (c *casillaNativa) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// La caja y el tic son la pieza de Chromium de `ui`, la misma de la tabla del ERP.
	r := &casillaNativaRenderer{
		c:     c,
		aro:   ui.NuevoAro(ui.RadioCasillaNativa, ui.SinHueco, v).Abierto(),
		pieza: ui.NuevaPiezaCasilla(ladoCasillaNativa, false, true, v),
	}
	r.Refresh()
	return r
}

type casillaNativaRenderer struct {
	c     *casillaNativa
	aro   *ui.Aro
	pieza *ui.PiezaCasilla
	// rotulos es un texto por renglón; casi siempre, uno.
	rotulos []*canvas.Text
}

func (r *casillaNativaRenderer) MinSize() fyne.Size { return r.c.MinSize() }
func (r *casillaNativaRenderer) Destroy()           {}
func (r *casillaNativaRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 0, 5+len(r.rotulos))
	objs = append(objs, r.pieza.Objetos()...)
	for _, t := range r.rotulos {
		objs = append(objs, t)
	}
	return r.aro.Antes(objs...)
}

func (r *casillaNativaRenderer) Layout(t fyne.Size) {
	y := (t.Height - ladoCasillaNativa) / 2
	r.aro.ColocarEn(fyne.NewPos(0, y), fyne.NewSize(ladoCasillaNativa, ladoCasillaNativa))
	r.pieza.Colocar(fyne.NewPos(0, y))
	// `items-center`: el bloque de renglones centrado en el alto, como la casilla.
	y0 := (t.Height - float32(len(r.rotulos))*r.c.esc.Linea) / 2
	for i, x := range r.rotulos {
		ui.ColocarTexto(x, r.c.esc, ladoCasillaNativa+r.c.hueco, y0+float32(i)*r.c.esc.Linea)
	}
}

func (r *casillaNativaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.pieza.Pintar(ui.EstadoCasilla{Marcada: r.c.marcada}, v)
	lineas := r.c.lineasCasilla()
	if len(r.rotulos) != len(lineas) {
		r.rotulos = make([]*canvas.Text, len(lineas))
		for i := range lineas {
			r.rotulos[i] = ui.Texto("", r.c.esc, ui.Normal, color.Black)
		}
	}
	for i, x := range r.rotulos {
		x.Text, x.Color = lineas[i], tema.Color(r.c.tinta, v)
		x.Refresh()
	}
	r.aro.Pintar(v)
	r.aro.Ver(r.c.Enfocado())
	r.Layout(r.c.Size())
}

var _ fyne.Widget = (*casillaNativa)(nil)

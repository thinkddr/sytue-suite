// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

/*
El PANEL LATERAL de CertiPDF (`PanelLateral` de `pdf/Paneles.tsx`): una columna de
`w-72` con su tira de pestañas y, debajo, el panel abierto.

# Qué paneles hay y cuáles NO, dicho aquí para que nadie los dé por hechos

La web tiene nueve. Los que no están tampoco tienen pestaña: una pestaña que abre un
panel vacío es fingir cobertura, y un panel a medias que escribe en el documento es
peor, porque un PDF firmado no se reescribe dos veces. Ver [panelPortado].
*/

// PanelActivo es uno de los nueve paneles de `PanelActivo` en `Paneles.tsx`.
type PanelActivo uint8

// Los nueve, en el orden de la tira de la web.
const (
	PanelPaginas PanelActivo = iota
	PanelIndice
	PanelAdjuntos
	PanelComentarios
	PanelCampos
	PanelFirmas
	PanelEditar
	PanelBuscar
	PanelAsistente
)

// panelPortado son los paneles que este paquete sabe pintar. La tira enseña SOLO
// estos: los demás no aparecen hasta que existan.
//
// Están los nueve; el guardián `TestLaTiraDelPanelEsLaDeLaWeb` obliga a que un panel
// que se quite vuelva a su lista de pendientes.
var panelPortado = map[PanelActivo]bool{
	PanelPaginas: true, PanelIndice: true, PanelAdjuntos: true, PanelComentarios: true, PanelCampos: true,
	PanelFirmas: true, PanelEditar: true, PanelBuscar: true, PanelAsistente: true,
}

// pestanaDelPanel es una fila de `pestanas` en `PanelLateral`.
type pestanaDelPanel struct {
	id    PanelActivo
	clave string
	n     int
}

// tiraDelPanel es `pestanas` de la web, con las NUEVE filas y su misma regla: Adjuntos
// solo si el documento los lleva, porque casi ningún PDF los lleva y una pestaña vacía
// permanente estorba a los que no.
func tiraDelPanel(c *Contenido) []pestanaDelPanel {
	tira := []pestanaDelPanel{
		{PanelPaginas, "pdf.panel.pages", c.Total},
		// Siempre, tenga índice o no: aquí es donde se CREA.
		{PanelIndice, "pdf.panel.outline", len(c.Indice)},
	}
	if len(c.Adjuntos) > 0 {
		tira = append(tira, pestanaDelPanel{PanelAdjuntos, "pdf.panel.attachments", len(c.Adjuntos)})
	}
	return append(tira,
		pestanaDelPanel{PanelComentarios, "pdf.panel.comments", len(c.Anotaciones)},
		pestanaDelPanel{PanelCampos, "pdf.panel.fields", len(c.Campos)},
		pestanaDelPanel{PanelFirmas, "pdf.panel.signatures", len(c.Firmas)},
		pestanaDelPanel{PanelEditar, "pdf.panel.edit", 0},
		pestanaDelPanel{PanelBuscar, "pdf.panel.search", 0},
		pestanaDelPanel{PanelAsistente, "pdf.ai.tab", 0},
	)
}

// Las medidas de `PanelLateral`, con raíz de 16 px.
var (
	// AnchoPanel es `w-72`: 288 px con la raíz de 16 de la suite.
	AnchoPanel = tema.Espacio(72)
	// rellenoPanel es el `p-2` del cuerpo.
	rellenoPanel = tema.Espacio(2)
	// pestanaPX y pestanaPY son el `px-2.5 py-2` de cada pestaña.
	pestanaPX = tema.Espacio(2.5)
	pestanaPY = tema.Espacio(2)
	// subrayadoPestana es su `border-b-2`, transparente en las que no están abiertas.
	subrayadoPestana float32 = 2
)

// AltoPestanaPanel es lo que ocupa la tira: `py-2` sobre la caja de línea de `text-2xs`
// y el subrayado, más el `border-b` de la tira: 35 px a rem16.
func AltoPestanaPanel() float32 {
	return tema.Texto2XS.Linea + 2*pestanaPY + subrayadoPestana + tema.Borde
}

// PanelLateral es la columna entera.
type PanelLateral struct {
	t  func(clave string, vars i18n.Vars) string
	c  *Contenido
	on PanelActivo
	// AlIrA lleva a una página (1-based). Lo pone la pantalla; puede ser nil.
	AlIrA func(n int)
	// PaginaActual es la que se está mirando: la miniatura que va marcada.
	PaginaActual int
	// Imagen es el dibujo de una página si ya llegó; lo que pintan las miniaturas.
	Imagen func(n int) image.Image

	miniaturas map[int]*filaMiniatura
	// pant es la pantalla, que es quien habla con el motor. Nil en las pruebas del
	// panel suelto: entonces sus acciones no hacen nada.
	pant *Pantalla
	// El estado de cada panel que lo tiene: sobrevive a cambiar de pestaña y a volver a
	// leer el documento tras un cambio, como el componente montado de la web.
	buscarP      *panelBuscar
	comentariosP *panelComentarios
	indiceP      *panelIndice
	camposP      *panelCampos
	editarP      *panelEditar
	asistenteP   *panelAsistente
	firmasP      *panelFirmas
	// bajando es el adjunto que se está guardando; errorAdjunto, el fallo del último.
	bajando, errorAdjunto string

	raiz *fyne.Container
}

// PonerContenido cambia el documento sin cerrar el panel que se estaba mirando: tras
// aplicar un cambio, la web vuelve a leer y el panel sigue donde estaba.
func (p *PanelLateral) PonerContenido(c *Contenido) {
	p.c = c
	// El veredicto de una firma es de unos bytes concretos: si el documento cambió, el
	// informe ya no vale.
	if p.firmasP != nil {
		p.firmasP.olvidar()
	}
	if p.comentariosP != nil {
		p.comentariosP.respondiendo, p.comentariosP.replica = -1, nil
	}
	if p.raiz != nil {
		p.raiz.Objects = p.componer()
		p.raiz.Refresh()
	}
}

// Recomponer vuelve a pintar el panel abierto, con su estado.
func (p *PanelLateral) Recomponer() {
	if p.raiz != nil {
		p.raiz.Objects = p.componer()
		p.raiz.Refresh()
	}
}

// trabajando es que hay una operación en vuelo: los botones que escriben se apagan.
func (p *PanelLateral) trabajando() bool { return p.pant != nil && p.pant.Trabajando }

// sinCapaDeTexto es que el documento no tiene ni una letra: un escaneo. Se sabe sin
// pedir nada más, porque el extracto de las páginas leídas —la primera siempre— viene
// vacío. Las que el motor no llegó a leer no cuentan: sin leer no es sin texto.
func (p *PanelLateral) sinCapaDeTexto() bool {
	leidas := 0
	for _, pg := range p.c.Paginas {
		if pg.Extracto == nil {
			continue
		}
		if *pg.Extracto != "" {
			return false
		}
		leidas++
	}
	return leidas > 0
}

// NuevoPanelLateral monta el panel sobre lo que devolvió `op: contenido`. Abre en
// Páginas, como la web (`useState<PanelActivo>('miniaturas')`).
func NuevoPanelLateral(t func(clave string, vars i18n.Vars) string, c *Contenido) *PanelLateral {
	return &PanelLateral{t: t, c: c, on: PanelPaginas, PaginaActual: 1}
}

// RefrescarMiniatura repinta la miniatura de la página `n` cuando llega su dibujo.
func (p *PanelLateral) RefrescarMiniatura(n int) {
	if f := p.miniaturas[n]; f != nil {
		f.Refresh()
	}
}

// RefrescarActual marca la miniatura de la página que se está mirando.
func (p *PanelLateral) RefrescarActual() {
	if p.camposP != nil && p.on == PanelCampos {
		p.camposP.alCambiarPagina()
	}
	if p.editarP != nil && p.on == PanelEditar {
		p.editarP.alCambiarPagina()
	}
	for n, f := range p.miniaturas {
		if f.actual != (n == p.PaginaActual) {
			f.actual = n == p.PaginaActual
			f.Refresh()
		}
	}
}

// Abierto es el panel que se ve.
func (p *PanelLateral) Abierto() PanelActivo { return p.on }

// Abrir cambia de panel. Uno que no está portado se ignora: no hay pestaña que lo pida.
func (p *PanelLateral) Abrir(a PanelActivo) {
	if !panelPortado[a] {
		return
	}
	p.on = a
	if p.raiz != nil {
		p.raiz.Objects = p.componer()
		p.raiz.Refresh()
	}
}

// Objeto es la columna: la tira arriba y el panel, que se desplaza, debajo.
func (p *PanelLateral) Objeto() fyne.CanvasObject {
	if p.raiz == nil {
		p.raiz = container.New(ui.Pila{Elastico: 1})
		p.raiz.Objects = p.componer()
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// El panel se superpone al PDF en estrecho: sus opciones conservan un papel
	// opaco también en traslúcido. El borde va por dentro del ancho del panel.
	fondo := canvas.NewRectangle(tema.Color("menu-surface", v))
	linea := canvas.NewRectangle(tema.Color("border", v))
	return container.New(bordeDerecho{}, fondo, linea, p.raiz)
}

func (p *PanelLateral) componer() []fyne.CanvasObject {
	pestanas := make([]fyne.CanvasObject, 0, len(panelPortado))
	for _, x := range tiraDelPanel(p.c) {
		if !panelPortado[x.id] {
			continue
		}
		id := x.id
		pestanas = append(pestanas, nuevaPestanaPanel(p.t(x.clave, nil), x.n, x.id == p.on, func() { p.Abrir(id) }))
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// SE DESLIZA, no se reparte: los nueve rótulos no caben en los 252 del panel y
	// cortarlos deja rótulos ilegibles de cuatro letras. La web enmascara la tira con su
	// `border-b`, sobre el papel opaco del panel.
	deslizante, s := deslizableCon(container.New(ui.Fila{}, pestanas...))
	tira := conMascara(container.New(conLineaAbajo{}, canvas.NewRectangle(tema.Color("border", v)), deslizante), s, "menu-surface")
	cuerpo := container.New(ui.Margen{Izq: rellenoPanel, Arr: rellenoPanel, Der: rellenoPanel, Abj: rellenoPanel},
		p.panel())
	return []fyne.CanvasObject{tira, container.NewVScroll(cuerpo)}
}

// anchoUtil es lo que le queda al contenido de un panel: la columna menos su borde y
// el `p-2` de los dos lados. 237.
func anchoUtil() float32 { return AnchoPanel - tema.Borde - 2*rellenoPanel }

func (p *PanelLateral) panel() fyne.CanvasObject {
	switch p.on {
	case PanelAdjuntos:
		return p.adjuntos()
	case PanelFirmas:
		return p.firmas()
	case PanelPaginas:
		return p.paginas()
	case PanelComentarios:
		return p.comentarios()
	case PanelBuscar:
		return p.buscar()
	case PanelCampos:
		return p.campos()
	case PanelEditar:
		return p.editar()
	case PanelAsistente:
		return p.asistente()
	case PanelIndice:
	}
	return p.indice()
}

// ---- Índice ----

// sangriaIndice es el `paddingInlineStart` de la web: `0.5rem` más `0.75rem` por nivel,
// con el nivel topado en 6 para que un documento con cuarenta niveles no se salga.
func sangriaIndice(nivel int) float32 {
	return (0.5 + float32(min(max(nivel, 0), 6))*0.75) * tema.Rem
}

// indice es `Indice`: la lista en lectura con «Editar índice» o «Crear índice» debajo,
// o el borrador en edición (ver [panelIndice]).
func (p *PanelLateral) indice() fyne.CanvasObject {
	if p.indiceP != nil && p.indiceP.borrador != nil {
		return p.indiceP.edicion()
	}
	ancho := anchoUtil() - 2*tema.Espacio(1)
	var lista fyne.CanvasObject
	if len(p.c.Indice) == 0 {
		lista = parrafoVacio(p.t("pdf.outline.empty", nil), ancho)
	} else {
		filas := make([]fyne.CanvasObject, 0, len(p.c.Indice))
		for _, m := range p.c.Indice {
			f := nuevaFilaIndice(m, ancho, p.t("pdf.outline.noTarget", nil))
			if m.Pagina >= 0 {
				n := m.Pagina + 1
				f.AlPulsar = func() {
					if p.AlIrA != nil {
						p.AlIrA(n)
					}
				}
			}
			filas = append(filas, f)
		}
		lista = container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, filas...)
	}
	clave := "pdf.outline.edit"
	if len(p.c.Indice) == 0 {
		clave = "pdf.outline.create"
	}
	editar := botonPanel(p.t(clave, nil), tema.TextoXS, tema.Espacio(1.5), p.editarIndice)
	editar.Peso, editar.FondoEncima, editar.Ancho = ui.Medio, "bg-subtle", ancho
	return container.New(ui.Margen{Izq: tema.Espacio(1), Arr: tema.Espacio(1), Der: tema.Espacio(1), Abj: tema.Espacio(1)},
		container.New(ui.Columna{Hueco: tema.Espacio(1)}, lista, container.New(ui.Margen{Arr: tema.Espacio(1)}, editar)))
}

// parrafoVacio es el `<p class="text-text-subtle p-2 text-xs">` de un panel sin nada.
func parrafoVacio(texto string, ancho float32) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	m := tema.Espacio(2)
	return container.New(ui.Margen{Izq: m, Arr: m, Der: m, Abj: m},
		ui.NuevoParrafo(texto, tema.TextoXS, ui.Normal, "text-subtle", v).A(ancho-2*m))
}

// ---- Adjuntos ----

// adjuntos es `Adjuntos`: una ficha por fichero, `gap-1.5`, cada una con «Guardar».
func (p *PanelLateral) adjuntos() fyne.CanvasObject {
	if len(p.c.Adjuntos) == 0 {
		return parrafoVacio(p.t("pdf.attachments.empty", nil), anchoUtil())
	}
	dentro := anchoUtil() - 2*tema.Borde - 2*tema.Espacio(2)
	var partes []fyne.CanvasObject
	if p.errorAdjunto != "" {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		partes = append(partes, container.New(ui.Margen{Izq: tema.Espacio(1), Der: tema.Espacio(1)},
			ui.NuevoParrafo(p.errorAdjunto, tema.Texto2XS, ui.Normal, "danger-text", v).A(anchoUtil()-2*tema.Espacio(1))))
	}
	fichas := make([]fyne.CanvasObject, 0, len(p.c.Adjuntos))
	for _, a := range p.c.Adjuntos {
		lineas := []fyne.CanvasObject{rotuloRecortado(a.Nombre, tema.TextoXS, ui.Medio, "text", dentro)}
		if a.Descripcion != "" {
			lineas = append(lineas, rotuloRecortado(a.Descripcion, tema.Texto2XS, ui.Normal, "text-subtle", dentro))
		}
		lineas = append(lineas, ui.NuevoRotulo(p.metaAdjunto(a), tema.Texto2XS, ui.Normal, "text-subtle"))
		rotulo := "pdf.attachments.save"
		if p.bajando == a.Nombre {
			rotulo = "pdf.attachments.saving"
		}
		nombre := a.Nombre
		guardar := botonPanel(p.t(rotulo, nil), tema.Texto2XS, tema.Espacio(1), func() { p.guardarAdjunto(nombre) })
		guardar.Inerte = p.bajando != ""
		lineas = append(lineas, container.New(ui.Margen{Arr: tema.Espacio(1.5)}, container.New(alPrincipio{}, guardar)))
		fichas = append(fichas, nuevoRecuadro(container.New(ui.Columna{}, lineas...)))
	}
	partes = append(partes, container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, fichas...))
	return container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, partes...)
}

// guardarAdjunto saca el fichero incrustado (`op: adjunto`) y lo deja en la carpeta de
// descargas, que es lo que hace el navegador con `descargarBlob`: sin preguntar dónde, y
// sin pisar uno que ya se llame igual.
func (p *PanelLateral) guardarAdjunto(nombre string) {
	pant := p.pant
	if pant == nil || pant.doc == nil {
		return
	}
	p.bajando, p.errorAdjunto = nombre, ""
	p.Recomponer()
	doc := pant.doc
	go func() {
		ctx, parar := context.WithTimeout(pant.ctx, plazo)
		defer parar()
		datos, err := doc.Binario(ctx, "/pdf/attachment", map[string]string{"nombre": nombre})
		if err == nil {
			var carpeta string
			if carpeta, err = ficheros.Carpeta(); err == nil {
				_, err = ficheros.Guardar(carpeta, nombre, bytes.NewReader(datos))
			}
		}
		fyne.Do(func() {
			defer pant.avisarRed()
			p.bajando = ""
			if err != nil {
				p.errorAdjunto = p.t("pdf.attachments.failed", i18n.Vars{"name": nombre})
			}
			p.Recomponer()
		})
	}()
}

// idioma es el de la pantalla que lo abrió; sin ella (las pruebas), el de respaldo.
func (p *PanelLateral) idioma() i18n.Locale {
	if p.pant == nil {
		return ""
	}
	return i18n.Locale(p.pant.Idioma)
}

// metaAdjunto es la línea `sytue-meta-dense` de la ficha: el tamaño con el
// `formatBytes` de la suite, el tipo si lo declara y la página si cuelga de una.
func (p *PanelLateral) metaAdjunto(a Adjunto) string {
	s := i18n.FormatBytes(p.idioma(), float64(a.TamanoBytes))
	if a.Tipo != "" {
		s += " · " + a.Tipo
	}
	if a.Pagina >= 0 {
		s += " · " + p.t("pdf.page.n", i18n.Vars{"n": strconv.Itoa(a.Pagina + 1)})
	}
	return s
}

// rotuloRecortado es un `truncate`: el rótulo recortado con puntos suspensivos al ancho.
// Recorta con [ui.Truncar], que es la búsqueda del catálogo, y se queda con el texto.
func rotuloRecortado(s string, e tema.Escalon, p ui.Peso, token string, ancho float32) fyne.CanvasObject {
	c := ui.Texto(s, e, p, color.Transparent)
	ui.Truncar(c, s, e, p, ancho)
	return ui.NuevoRotulo(c.Text, e, p, token)
}

// ---- piezas de este panel ----

// recuadro es el `border-border rounded-md border p-2` de las fichas de Adjuntos y
// Firmas. No se pulsa ni se enfoca: es un contenedor con su caja.
type recuadro struct {
	widget.BaseWidget
	dentro fyne.CanvasObject
}

func nuevoRecuadro(dentro fyne.CanvasObject) *recuadro {
	r := &recuadro{dentro: dentro}
	r.ExtendBaseWidget(r)
	return r
}

func medidaRecuadro(dentro fyne.CanvasObject) fyne.Size {
	m := dentro.MinSize()
	e := 2*tema.Borde + 2*tema.Espacio(2)
	return fyne.NewSize(m.Width+e, m.Height+e)
}

func (r *recuadro) MinSize() fyne.Size { r.ExtendBaseWidget(r); return medidaRecuadro(r.dentro) }

func (r *recuadro) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	x := &recuadroRenderer{r: r, caja: ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD)}
	x.Refresh()
	return x
}

type recuadroRenderer struct {
	r    *recuadro
	caja *ui.Caja
}

func (x *recuadroRenderer) MinSize() fyne.Size { return medidaRecuadro(x.r.dentro) }
func (x *recuadroRenderer) Objects() []fyne.CanvasObject {
	return append(x.caja.Objetos(), x.r.dentro)
}
func (x *recuadroRenderer) Destroy() {}
func (x *recuadroRenderer) Layout(t fyne.Size) {
	x.caja.Redimensionar(t)
	e := tema.Borde + tema.Espacio(2)
	x.r.dentro.Move(fyne.NewPos(e, e))
	x.r.dentro.Resize(fyne.NewSize(t.Width-2*e, t.Height-2*e))
}

func (x *recuadroRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	x.caja.Pintar(tema.Color("surface", v), tema.Color("border", v))
	x.Layout(x.r.Size())
}

// bordeDerecho coloca [fondo, línea, contenido]: el fondo entero, la línea de 1 px al
// borde derecho y el contenido en lo que queda. Mide [AnchoPanel] de ancho.
type bordeDerecho struct{}

func (bordeDerecho) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(AnchoPanel, objs[2].MinSize().Height)
}

func (bordeDerecho) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	objs[1].Move(fyne.NewPos(t.Width-tema.Borde, 0))
	objs[1].Resize(fyne.NewSize(tema.Borde, t.Height))
	objs[2].Move(fyne.NewPos(0, 0))
	objs[2].Resize(fyne.NewSize(t.Width-tema.Borde, t.Height))
}

// conLineaAbajo coloca [línea, contenido]: el contenido a su alto y la línea de 1 px
// debajo, a todo el ancho. Es el `border-b` de la tira.
type conLineaAbajo struct{}

func (conLineaAbajo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[1].MinSize()
	return fyne.NewSize(m.Width, m.Height+tema.Borde)
}

func (conLineaAbajo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[1].Move(fyne.NewPos(0, 0))
	objs[1].Resize(fyne.NewSize(t.Width, t.Height-tema.Borde))
	objs[0].Move(fyne.NewPos(0, t.Height-tema.Borde))
	objs[0].Resize(fyne.NewSize(t.Width, tema.Borde))
}

// conLineaArriba coloca [línea, contenido]: la línea de 1 px arriba y el contenido
// `sep` más abajo. Es el `border-t pt-2` del descargo.
type conLineaArriba struct{ sep float32 }

func (c conLineaArriba) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[1].MinSize()
	return fyne.NewSize(m.Width, m.Height+tema.Borde+c.sep)
}

func (c conLineaArriba) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, tema.Borde))
	objs[1].Move(fyne.NewPos(0, tema.Borde+c.sep))
	objs[1].Resize(fyne.NewSize(t.Width, t.Height-tema.Borde-c.sep))
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image/color"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Paridad web: la `BarraDeCuaderno` de NotesApp.tsx.

// LA BARRA DEL CUADERNO, medida en producción el 2026-09-21.
//
//	franja        min-h-12 → 42 de alto mínimo, `px-4 py-2 gap-2`, filete abajo   hoja
//	título        text-lg 15,75/22,75 peso 600                                    MEDIDO
//	filtro        21 × 182 (`max-w-52`), `px-1.5`                                 MEDIDO
//	desplegables  28 de alto, `px-3`, 11,38/15,75 peso 600, radio 6, borde 1      MEDIDO
//	iconos        24 × 24 (deshacer, rehacer, ajustes)                            MEDIDO
//	«Nueva nota»  28 de alto × 102                                                MEDIDO
//
// Los 28 y los 24 salen del catálogo sin tocar nada: `ui.TallaSM` es `h-8` (28) y
// `ui.LadoIconoSM` es 24. No hay una sola medida propia en este fichero, que es como
// tiene que ser.
const (
	// AltoBarra es el `min-h-12`. Mínimo: con la ventana estrecha el filtro baja a su
	// propia fila y la barra crece, igual que en la web.
	AltoBarra = 12 * 0.25 * tema.Rem
	// MargenBarra y HuecoBarra son `px-4 py-2` y `gap-2`.
	MargenBarra  = 4 * 0.25 * tema.Rem
	RellenoBarra = 2 * 0.25 * tema.Rem
	HuecoBarra   = 2 * 0.25 * tema.Rem
	// AnchoFiltro es `max-w-52` (13 rem): 182 medidos.
	AnchoFiltro = 52 * 0.25 * tema.Rem
	// GlifoBarra es el `size={14}` de los glifos de los desplegables, y GlifoAccion el
	// `size={16}` de los botones de icono.
	GlifoBarra  = 14
	GlifoAccion = 16
)

// Barra es la barra del cuaderno abierto: título, candado, filtro, orden, etiquetas,
// exportar, deshacer, rehacer, ajustes y «nueva nota».
//
// Como todo en este paquete es estado puro: se rellenan los campos y se pide `Objeto`.
type Barra struct {
	tr *i18n.Translator

	Cuaderno Cuaderno
	Filtro   string
	Orden    Orden
	// Etiquetas enciende el desplegable de «filtrar por etiqueta», que la web solo
	// pinta cuando hay alguna, y FiltroEstado es el estado por el que se filtra.
	Etiquetas    []api.Tag
	FiltroEstado string
	// Bloqueado es el cuaderno cifrado y cerrado; SinRed desactiva lo que escribe.
	Bloqueado bool
	SinRed    bool
	// Deshacer y Rehacer son la etiqueta de lo siguiente que se puede deshacer o
	// rehacer, vacía si no hay nada. Es lo que decide si el botón está inerte y lo que
	// va en su nombre accesible («Deshacer: mover»).
	Deshacer string
	Rehacer  string
	// Estrecha es la ventana por debajo de `sm` (640, rem de 16): el filtro baja a su
	// propia fila, entera, y «Orden», «Exportar» y «Nueva nota» se quedan en su icono
	// (`max-sm:sr-only`: el rótulo sigue siendo su nombre accesible).
	Estrecha bool

	AlFiltrar      func(string)
	AlOrdenar      func(Orden)
	AlFiltroEstado func(string)
	AlExportar     func(formato string, ver bool)
	AlDeshacer     func()
	AlRehacer      func()
	AlAjustes      func()
	// AlCompartir es «Compartir con personas» del cuaderno: solo su dueño. Nil: no sale.
	AlCompartir   func()
	AlNuevaNota   func()
	AlDesbloquear func()
	AlBloquear    func()
	// AlMenu abre un menú superpuesto. Lo pone la pantalla, que es quien tiene el
	// lienzo donde la capa se dibuja.
	AlMenu func(disparador fyne.CanvasObject, m *ui.Menu)

	// AlAbrirFiltro abre o cierra el filtro de texto de una PÁGINA (la lupa de `Acciones`):
	// lo pone la pantalla, que es quien repinta y da el foco al campo.
	AlAbrirFiltro func()

	// filtro es el campo de filtrar, y es SIEMPRE el mismo: la pantalla se repinta con
	// cada letra, y un campo nuevo en cada repintado perdía el foco al teclear la
	// primera.
	filtro *ui.Entrada
	// Filtrando es la lupa de una página abierta: con algo escrito el campo se queda
	// aunque se cierre, o no se sabría por qué faltan notas.
	Filtrando bool
}

// enPagina dice si el cuaderno se pinta como PÁGINA: un día y un cuaderno en lista llevan
// su título en la página y sus acciones al lado, sin barra encima (NotesApp.tsx, `enDia`).
// La barra se queda donde las notas ocupan todo el ancho —columnas, lienzo libre, árbol—
// y en un cuaderno cifrado, que necesita a la vista el botón del candado.
func enPagina(c Cuaderno) bool {
	return c.ID != "" && !c.Cifrado && (c.Crudo.DailyDate != "" || c.Vista == Lista)
}

// NuevaBarra crea la barra.
func NuevaBarra(tr *i18n.Translator) *Barra { return &Barra{tr: tr, Orden: OrdenManual} }

// Objeto arma la franja entera.
func (b *Barra) Objeto(v tema.Variante) fyne.CanvasObject {
	t := b.tr.T
	piezas := []fyne.CanvasObject{}

	if b.Cuaderno.Emoji != "" {
		piezas = append(piezas, ui.Texto(b.Cuaderno.Emoji, tema.TextoLG, ui.SemiNegrita,
			tema.Color("text", v)))
	}
	// El nombre CEDE (`min-w-0 truncate`): una caja de línea que recorta con elipsis.
	// En un diario el título es el día y ya lo escribe la página (`sr-only` en la web).
	nombre := b.Cuaderno.Titulo
	if b.Cuaderno.Crudo.DailyDate != "" {
		nombre = ""
	}
	piezas = append(piezas, lineaTok(nombre, tema.TextoLG, ui.SemiNegrita, "text", v))

	// El candado solo existe en un cuaderno cifrado, y dice lo CONTRARIO de su estado:
	// cerrado ofrece abrirlo y abierto ofrece cerrarlo ya.
	var candado fyne.CanvasObject
	if b.Cuaderno.Cifrado {
		clave, accion := "crypto.lockNow", b.AlBloquear
		if b.Bloqueado {
			clave, accion = "crypto.unlock", b.AlDesbloquear
		}
		bt := ui.NuevoBoton(t(clave, nil), func() { llamar0(accion) })
		bt.Variante, bt.Talla = ui.Secundario, ui.TallaSM
		bt.Icono, bt.TamIcono = iconos.Candado, GlifoBarra
		candado = bt
	}

	filtro := b.campoFiltro()

	// Los tres menús, con un icono por disparador: la barra dice qué cuaderno es y deja
	// escribir; los rótulos («Orden propio», «Filtrar por etiqueta», «Exportar») la
	// convertían en una fila de texto azul más larga que el título.
	orden := b.menuIcono(OrdenD, t("orden.label", nil)+": "+t(b.Orden.ClaveI18n(), nil), b.menuOrden)

	var etiquetas fyne.CanvasObject
	if len(b.Etiquetas) > 0 {
		e := b.menuIcono(FiltroD, t("tags.filterBy", nil), b.menuEtiquetas)
		e.Pulsado = b.FiltroEstado != ""
		etiquetas = e
	}

	// EXPORTAR ES UN DERECHO y por eso está en la barra y no escondido en los ajustes:
	// el artículo 20 del RGPD da derecho a recibir los propios datos, y un derecho que
	// hay que buscar en un modal se ejerce menos. Es el comentario de la web, literal.
	exportar := b.menuIcono(BajarD, t("export.label", nil), b.menuExportar)

	// Deshacer y rehacer solo salen cuando hay algo que deshacer o rehacer: dos flechas
	// apagadas eran dos botones que no hacían nada.
	var deshacer, rehacer fyne.CanvasObject
	if b.Deshacer != "" {
		deshacer = b.accionPropia(DeshacerD, t("history.undoOf", i18n.Vars{"action": b.Deshacer}), false, b.AlDeshacer)
	}
	if b.Rehacer != "" {
		rehacer = b.accionPropia(RehacerD, t("history.redoOf", i18n.Vars{"action": b.Rehacer}), false, b.AlRehacer)
	}

	var compartir fyne.CanvasObject
	if b.AlCompartir != nil {
		c := ui.NuevoBotonIcono(iconos.Usuarios, t("sharePeople.open", nil), func() { llamar0(b.AlCompartir) })
		c.Lado, c.TamIcono = ui.LadoIconoSM, GlifoAccion
		compartir = c
	}
	ajustes := ui.NuevoBotonIcono(iconos.Ajustes, t("notebook.settings", nil),
		func() { llamar0(b.AlAjustes) })
	ajustes.Lado, ajustes.TamIcono = ui.LadoIconoSM, GlifoAccion

	nueva := ui.NuevoBoton(b.rotulo(t("note.new", nil)), func() { llamar0(b.AlNuevaNota) })
	nueva.Talla, nueva.Icono, nueva.TamIcono = ui.TallaSM, iconos.Mas, 15
	nueva.Rotulo = t("note.new", nil)
	nueva.Inerte = b.Bloqueado || b.SinRed
	b.soloIcono(nueva)

	// El título es `mr-auto flex min-w-0 items-center gap-2`: el emoji y el nombre juntos,
	// y todo lo que sobre de la primera fila detrás de él.
	titulo := container.New(&filaElastica{hueco: HuecoBarra, elastico: len(piezas) - 1}, piezas...)
	items := sinNil([]fyne.CanvasObject{
		titulo, candado, filtro, orden, etiquetas, exportar, deshacer, rehacer, compartir, ajustes, nueva,
	})
	entera := -1
	if b.Estrecha {
		// `order-last basis-full`: el filtro, el último y en su fila.
		items = append(slices.DeleteFunc(items, func(o fyne.CanvasObject) bool { return o == filtro }), filtro)
		entera = len(items) - 1
	}
	fondo := canvas.NewRectangle(tema.Color("surface", v))
	filete := canvas.NewRectangle(tema.Color("border-subtle", v))
	flex := &barraFlex{
		entera: entera, px: MargenBarra, py: RellenoBarra, hueco: HuecoBarra,
		minAlto: AltoBarra, auto: true,
	}
	return container.New(flex, append([]fyne.CanvasObject{fondo, filete}, items...)...)
}

// soloIcono ajusta el ancho de un botón que en móvil pierde el rótulo: el rótulo va
// `sr-only` —fuera del flujo—, así que no queda ni el texto ni el `gap` que lo separaba
// del icono, y el botón mide su relleno más el glifo.
func (b *Barra) soloIcono(bt *ui.Boton) {
	if b.Estrecha {
		bt.Ancho = ui.MedidaBoton("", bt.Talla, 0).Width + bt.TamIcono
	}
}

// rotulo es el texto visible de un botón que en móvil se queda en su icono.
func (b *Barra) rotulo(s string) string {
	if b.Estrecha {
		return ""
	}
	return s
}

// campoFiltro es el campo de filtrar, creado una sola vez. El texto solo cambia
// tecleando —la web tampoco lo vacía al cambiar de cuaderno—, así que `Filtro` se lee al
// crearlo y después manda el campo.
func (b *Barra) campoFiltro() *ui.Entrada {
	if b.filtro == nil {
		b.filtro = ui.NuevaEntrada(b.tr.T("filter.placeholder", nil)).Con(ui.CampoSM)
		b.filtro.Ancho = AnchoFiltro
		b.filtro.PonerTexto(b.Filtro)
		b.filtro.AlCambiar(func(s string) { llamar1(b.AlFiltrar, s) })
	}
	return b.filtro
}

// menuIcono es un botón de icono con un glifo de esta app que abre un menú.
func (b *Barra) menuIcono(d, rotulo string, menu func() *ui.Menu) *ui.BotonIcono {
	var bt *ui.BotonIcono
	bt = b.accionPropia(d, rotulo, false, func() {
		if m := menu(); m != nil && b.AlMenu != nil {
			b.AlMenu(bt, m)
		}
	})
	return bt
}

// Acciones es la barra de una PÁGINA (`variante="dia"` de BarraDeCuaderno): en un día y en
// un cuaderno en lista no hay tira —el título lo escribe la página— y lo mismo cabe en
// tres botones al lado: el filtro tras una lupa (no en un día), exportar y «…».
func (b *Barra) Acciones(v tema.Variante) []fyne.CanvasObject {
	t := b.tr.T
	var out []fyne.CanvasObject
	switch {
	case b.Filtrando || b.Filtro != "":
		out = append(out, b.campoFiltro())
	case b.Cuaderno.Crudo.DailyDate == "":
		lupa := ui.NuevoBotonIcono(iconos.Buscar, t("filter.placeholder", nil), func() { llamar0(b.AlAbrirFiltro) })
		lupa.Lado, lupa.TamIcono = ui.LadoIconoSM, GlifoAccion
		out = append(out, lupa)
	}
	out = append(out, b.menuIcono(BajarD, t("export.label", nil), b.menuExportar))
	var mas *ui.BotonIcono
	mas = ui.NuevoBotonIcono(iconos.Puntos, t("common.more", nil), func() {
		if b.AlMenu != nil {
			b.AlMenu(mas, b.menuMas())
		}
	})
	mas.Lado, mas.TamIcono = ui.LadoIconoSM, GlifoAccion
	return append(out, mas)
}

// menuMas es el «…» de una página: deshacer y rehacer si los hay, el orden, las etiquetas
// si existen, compartir y los ajustes.
func (b *Barra) menuMas() *ui.Menu {
	t := b.tr.T
	var es []fyne.CanvasObject
	if b.Deshacer != "" {
		es = append(es, ui.NuevoElementoMenu(t("history.undoOf", i18n.Vars{"action": b.Deshacer}), iconos.Ninguno, func() { llamar0(b.AlDeshacer) }))
	}
	if b.Rehacer != "" {
		es = append(es, ui.NuevoElementoMenu(t("history.redoOf", i18n.Vars{"action": b.Rehacer}), iconos.Ninguno, func() { llamar0(b.AlRehacer) }))
	}
	if len(es) > 0 {
		es = append(es, ui.NuevoSeparadorMenu())
	}
	for _, o := range Ordenes {
		e := ui.NuevoElementoMenu(t(o.ClaveI18n(), nil), iconos.Ninguno, func() { llamar1(b.AlOrdenar, o) })
		e.Marcada = b.Orden == o
		es = append(es, e)
	}
	if len(b.Etiquetas) > 0 {
		es = append(es, ui.NuevoSeparadorMenu())
		todas := ui.NuevoElementoMenu(t("tags.filterBy", nil)+": "+t("filter.all", nil), iconos.Ninguno, func() { llamar1(b.AlFiltroEstado, "") })
		es = append(es, todas)
		for _, tag := range b.Etiquetas {
			for _, st := range tag.States {
				e := ui.NuevoElementoMenu(tag.Name+": "+st.Name, iconos.Ninguno, func() { llamar1(b.AlFiltroEstado, st.ID) })
				e.Marcada = b.FiltroEstado == st.ID
				e.PuntoColor, e.TamIcono = hexO(st.Color, color.NRGBA{}), ui.LadoPuntoMenu
				es = append(es, e)
			}
		}
	}
	es = append(es, ui.NuevoSeparadorMenu())
	if b.AlCompartir != nil {
		es = append(es, ui.NuevoElementoMenu(t("sharePeople.open", nil), iconos.Ninguno, func() { llamar0(b.AlCompartir) }))
	}
	es = append(es, ui.NuevoElementoMenu(t("notebook.settings", nil), iconos.Ninguno, func() { llamar0(b.AlAjustes) }))
	return ui.NuevoMenu(es...)
}

// accionPropia es un botón de icono de 24 con un glifo de esta app.
func (b *Barra) accionPropia(d, rotulo string, inerte bool, alPulsar func()) *ui.BotonIcono {
	bt := ui.NuevoBotonIcono(iconos.Ninguno, rotulo, func() {
		if !inerte {
			llamar0(alPulsar)
		}
	})
	bt.Glifo = iconos.TrazoDeRuta(d, GlifoAccion, tema.Color("text", tema.Claro))
	bt.Lado, bt.TamIcono = ui.LadoIconoSM, GlifoAccion
	// `disabled:opacity-40`: el glifo al 40 % y el botón sin puntero ni foco.
	bt.Inerte = inerte
	return bt
}

// menuEtiquetas es «Todas» y, separados, los estados de cada etiqueta. El elegido lleva
// la marca.
//
// ponytail: la web marca el elegido con « ✓» escrito al final del rótulo; aquí es el tic
// del menú (`Marcada`), porque Figtree no trae ese carácter y saldría la caja vacía.
//
// Delante va el punto del color del estado (`h-2.5 w-2.5 rounded-full` con el color de
// los datos), y ocupa solo lo suyo: el `icon` de MenuItem no reserva los 16 de un glifo.
func (b *Barra) menuEtiquetas() *ui.Menu {
	elementos := []fyne.CanvasObject{
		ui.NuevoElementoMenu(b.tr.T("filter.all", nil), iconos.Ninguno, func() { llamar1(b.AlFiltroEstado, "") }),
		ui.NuevoSeparadorMenu(),
	}
	for _, tag := range b.Etiquetas {
		for _, s := range tag.States {
			e := ui.NuevoElementoMenu(tag.Name+": "+s.Name, iconos.Ninguno, func() { llamar1(b.AlFiltroEstado, s.ID) })
			e.Marcada = b.FiltroEstado == s.ID
			e.PuntoColor, e.TamIcono = hexO(s.Color, color.NRGBA{}), ui.LadoPuntoMenu
			elementos = append(elementos, e)
		}
	}
	return ui.NuevoMenu(elementos...)
}

func (b *Barra) menuOrden() *ui.Menu {
	elementos := make([]fyne.CanvasObject, 0, len(Ordenes))
	for _, o := range Ordenes {
		e := ui.NuevoElementoMenu(b.tr.T(o.ClaveI18n(), nil), iconos.Ninguno, func() { llamar1(b.AlOrdenar, o) })
		e.Marcada = b.Orden == o
		elementos = append(elementos, e)
	}
	return ui.NuevoMenu(elementos...)
}

// menuExportar son los cinco formatos y, separada, la vista previa.
//
// Solo el PDF se abre para VER, y es deliberado: lo pinta el visor del navegador o del
// sistema, que es un proceso aparte. Es la misma regla que la web.
func (b *Barra) menuExportar() *ui.Menu {
	elementos := make([]fyne.CanvasObject, 0, len(FormatosExport)+2)
	// Desde la barra se BAJAN los cinco, el PDF también: verlo es la entrada de abajo.
	for _, f := range FormatosExport {
		elementos = append(elementos, ui.NuevoElementoMenu(b.tr.T("export."+f.ID, nil),
			iconos.Ninguno, func() { llamar2(b.AlExportar, f.ID, false) }))
	}
	elementos = append(elementos, ui.NuevoSeparadorMenu(),
		ui.NuevoElementoMenu(b.tr.T("export.view", nil), iconos.Ninguno,
			func() { llamar2(b.AlExportar, "pdf", true) }))
	return ui.NuevoMenu(elementos...)
}

// franja coloca el fondo, las dos filas y el filete de abajo.
//
// Es `flex flex-wrap items-center gap-2 px-4 py-2 border-b`: el título a la izquierda
// con `mr-auto` y el resto pegado a la derecha, y el filete SIN ocupar sitio en la
// altura del contenido, como un `border-b` de CSS.
type franja struct{}

var _ fyne.Layout = (*franja)(nil)

func (f *franja) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) < 4 {
		return fyne.Size{}
	}
	izq, der := objs[1].MinSize(), objs[2].MinSize()
	alto := max(izq.Height, der.Height) + 2*RellenoBarra
	return fyne.NewSize(izq.Width+der.Width+2*MargenBarra+HuecoBarra,
		max(alto, AltoBarra)+tema.Borde)
}

func (f *franja) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) < 4 {
		return
	}
	fondo, izq, der, filete := objs[0], objs[1], objs[2], objs[3]
	fondo.Move(fyne.NewPos(0, 0))
	fondo.Resize(t)
	filete.Move(fyne.NewPos(0, t.Height-tema.Borde))
	filete.Resize(fyne.NewSize(t.Width, tema.Borde))

	cuerpo := t.Height - tema.Borde
	mi, md := izq.MinSize(), der.MinSize()
	// El título CEDE y los controles NO: en un `justify-between` sin esto manda quien
	// tenga el texto más largo, y un cuaderno con nombre largo empujaba «Nueva nota»
	// fuera de la barra.
	anchoIzq := max(t.Width-2*MargenBarra-HuecoBarra-md.Width, 0)
	izq.Move(fyne.NewPos(MargenBarra, (cuerpo-mi.Height)/2))
	izq.Resize(fyne.NewSize(min(mi.Width, anchoIzq), mi.Height))
	der.Move(fyne.NewPos(max(t.Width-MargenBarra-md.Width, MargenBarra), (cuerpo-md.Height)/2))
	der.Resize(md)
}

// barraFlex es la barra del cuaderno: `flex min-h-12 flex-wrap items-center gap-2 px-4
// py-2 border-b`. Los hijos van en fila y BAJAN a la siguiente cuando no caben; en la
// primera, el título (`mr-auto`) se queda a la izquierda y lo demás a la derecha, y en las
// siguientes todo va a la izquierda, que es lo que hace el navegador. El `gap` vale en los
// dos ejes. `entera` es el hijo con `basis-full`: ocupa su fila de lado a lado.
//
// El alto depende del ancho, así que la barra lo dice con `AltoPara` y quien la apila lo
// pregunta con el ancho de verdad (ver `pilaElastica`).
//
// Es también la barra de formato del editor (`flex flex-wrap items-center gap-0.5 px-1.5
// py-1 border-b`), sin `mr-auto` y sin alto mínimo.
type barraFlex struct {
	entera        int
	px, py, hueco float32
	minAlto       float32
	auto          bool
}

// Los dos primeros hijos son el fondo y el filete; los demás, lo que va en la fila.
func (f *barraFlex) items(objs []fyne.CanvasObject) []fyne.CanvasObject {
	if len(objs) < 2 {
		return nil
	}
	return objs[2:]
}

// lineas reparte los hijos en filas dentro de `cabe`.
func (f *barraFlex) lineas(items []fyne.CanvasObject, cabe float32) [][]int {
	var out [][]int
	var fila []int
	var x float32
	for i, o := range items {
		w := o.MinSize().Width
		if i == f.entera {
			if len(fila) > 0 {
				out = append(out, fila)
			}
			out, fila, x = append(out, []int{i}), nil, 0
			continue
		}
		if len(fila) > 0 && x+f.hueco+w > cabe {
			out, fila, x = append(out, fila), nil, 0
		}
		if len(fila) > 0 {
			x += f.hueco
		}
		fila, x = append(fila, i), x+w
	}
	if len(fila) > 0 {
		out = append(out, fila)
	}
	return out
}

func altoFila(items []fyne.CanvasObject, fila []int) float32 {
	var alto float32
	for _, i := range fila {
		alto = max(alto, items[i].MinSize().Height)
	}
	return alto
}

// AltoPara es el alto de la barra con ese ancho: las filas, sus huecos, el `py-2` y el
// filete, y nunca menos que `min-h-12`.
func (f *barraFlex) AltoPara(objs []fyne.CanvasObject, ancho float32) float32 {
	if ancho <= 0 {
		ancho = 1e6 // sin colocar todavía: todo en una fila
	}
	items := f.items(objs)
	var alto float32
	for i, fila := range f.lineas(items, ancho-2*f.px) {
		if i > 0 {
			alto += f.hueco
		}
		alto += altoFila(items, fila)
	}
	return max(alto+2*f.py, f.minAlto) + tema.Borde
}

func (f *barraFlex) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho float32
	for _, o := range f.items(objs) {
		ancho = max(ancho, o.MinSize().Width)
	}
	return fyne.NewSize(ancho+2*f.px, f.AltoPara(objs, 1280))
}

func (f *barraFlex) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) < 2 {
		return
	}
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	objs[1].Move(fyne.NewPos(0, t.Height-tema.Borde))
	objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
	items := f.items(objs)
	cabe := t.Width - 2*f.px
	lineas := f.lineas(items, cabe)
	// Las filas van centradas en el alto que tenga la barra (`items-center` con
	// `min-h-12`).
	var total float32
	for i, fila := range lineas {
		if i > 0 {
			total += f.hueco
		}
		total += altoFila(items, fila)
	}
	y := max((t.Height-tema.Borde-total)/2, f.py)
	for n, fila := range lineas {
		alto := altoFila(items, fila)
		var usado float32
		for k, i := range fila {
			if k > 0 {
				usado += f.hueco
			}
			usado += items[i].MinSize().Width
		}
		x := f.px
		for k, i := range fila {
			o := items[i]
			m := o.MinSize()
			w := m.Width
			switch {
			case i == f.entera:
				w = cabe
			case f.auto && n == 0 && k == 0 && i == 0 && usado > cabe:
				// El título cede (`min-w-0 truncate`) cuando la fila no le deja sitio.
				w = max(m.Width-(usado-cabe), 0)
			}
			o.Move(fyne.NewPos(x, y+(alto-m.Height)/2))
			o.Resize(fyne.NewSize(w, m.Height))
			x += w + f.hueco
			if f.auto && n == 0 && k == 0 && i == 0 && usado < cabe {
				x += cabe - usado // `mr-auto`
			}
		}
		y += alto + f.hueco
	}
}

// altoParaAncho lo cumplen los layouts cuyo alto depende del ancho (lo que envuelve).
type altoParaAncho interface {
	AltoPara(objs []fyne.CanvasObject, ancho float32) float32
}

// altoDe es el alto que ocupa un hijo con ese ancho: el que diga su layout si envuelve,
// y si no, su mínimo.
func altoDe(o fyne.CanvasObject, ancho float32) float32 {
	if t, ok := o.(*Tarjeta); ok {
		return MedidaTarjetaNota(t, ancho).Height
	}
	if c, ok := o.(*fyne.Container); ok {
		if a, ok := c.Layout.(altoParaAncho); ok {
			return a.AltoPara(c.Objects, ancho)
		}
	}
	return ui.AltoA(o, ancho)
}

// pilaElastica es `ui.Pila` preguntando a cada hijo su alto CON el ancho de verdad: la
// barra que envuelve mide una fila o dos según la ventana, y con su mínimo a ciegas o
// sobraba una franja o se montaba encima del lienzo.
type pilaElastica struct{ elastico int }

func (p pilaElastica) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return ui.Pila{Elastico: p.elastico}.MinSize(objs)
}

func (p pilaElastica) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	var fijo float32
	for i, o := range objs {
		if i != p.elastico && o.Visible() {
			fijo += altoDe(o, t.Width)
		}
	}
	var y float32
	for i, o := range objs {
		if !o.Visible() {
			continue
		}
		alto := altoDe(o, t.Width)
		if i == p.elastico {
			alto = max(t.Height-fijo, 0)
		}
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(t.Width, alto))
		y += alto
	}
}

// sinNil quita los huecos de una lista con opcionales, que es lo que deja un `? :` de
// la web al no pintar algo.
func sinNil(objs []fyne.CanvasObject) []fyne.CanvasObject {
	out := objs[:0]
	for _, o := range objs {
		if o != nil {
			out = append(out, o)
		}
	}
	return out
}

func llamar2[A, B any](f func(A, B), a A, b B) {
	if f != nil {
		f(a, b)
	}
}

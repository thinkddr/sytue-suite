// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"cmp"
	"image/color"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// LA BARRA DE FORMATO DEL EDITOR (RichText.tsx y el cromo de `cromoDeEditor.ts`).
//
// Siete grupos, siempre en el mismo orden y separados por un pelo: historia (deshacer,
// rehacer) | bloque (el estilo del párrafo y su alineación) | formato en línea (negrita,
// cursiva, subrayado, tachado, código) | color (resaltar, color del texto) | listas y
// sangría | insertar (enlace, tabla, separador, símbolos) | herramientas (quitar formato,
// buscar y reemplazar, atajos y, si lo hay, el asistente).
//
// # UNA fila, y lo que no cabe va a «Más»
//
// Antes la barra envolvía en el escritorio (dos o tres filas en una columna estrecha) y en
// el teléfono era una fila que se desplazaba de lado, con la mitad de los botones fuera de
// la vista sin nada que lo dijera. Ahora es una fila de alto FIJO en todos los anchos: los
// botones que no caben se recogen, empezando por los de menos uso (`prioridad`), en el
// menú «Más» del final, que los ofrece con su rótulo y su estado. Así la barra mide lo
// mismo en cualquier nota y con cualquier ancho, y el cuerpo no salta al cambiar de nota
// ni al mover el cursor (el desplegable de bloque, además, mide siempre lo que su rótulo
// más largo).
//
// En un aparato táctil cada botón mide 44 × 44 (`pointer-coarse:min-h-[max(44px,2.75rem)]`
// de BOTON_DE_BARRA) y el desplegable de bloque se queda en su glifo. En el escritorio cada
// botón lleva su pista con el atajo («Negrita (Ctrl+B)»), que en un teléfono no tiene a
// qué responder.
//
// La barra NO entra en el orden de tabulación (el Tab del cuerpo sangra, como en la web):
// se llega a todo con sus atajos —la hoja de Ctrl/⌘+/ los enseña— y el menú «Más» y los
// selectores se recorren con las flechas.

// grupoBarra es uno de los siete grupos de la barra, en su orden.
type grupoBarra int

const (
	grupoHistoria grupoBarra = iota
	grupoBloque
	grupoFormato
	grupoColor
	grupoListas
	grupoInsertar
	grupoHerramientas
)

// accionBarra es un botón de la barra: dónde va, cuánto aguanta antes de irse a «Más» y
// qué hace. `pulsar` recibe de dónde colgar lo que abra (el botón o, si está recogido,
// «Más»).
type accionBarra struct {
	clave string
	grupo grupoBarra
	// prioridad: cuanto MAYOR, antes se va a «Más» cuando no cabe.
	prioridad int
	glifo     string
	icono     iconos.Icono
	rotulo    string
	// teclas es el atajo que dice la pista, ya en la forma del sistema; vacío, ninguno.
	teclas string
	pulsar func(desde fyne.CanvasObject)
	// abre dice que la acción despliega algo (un menú, una paleta): el foco vuelve al
	// cuerpo cuando eso se cierra, no al pulsar.
	abre bool
	// soloTeclado es lo que no tiene sentido sin teclado físico: la hoja de atajos.
	soloTeclado bool
	// disponible, si lo hay, dice si la acción vale en este editor (mencionar necesita
	// gente): si no, ni se ve ni va a «Más».
	disponible func() bool
}

// vale dice si la acción está disponible.
func (a accionBarra) vale() bool { return a.disponible == nil || a.disponible() }

// pista es lo que dice la ayuda del botón: el rótulo y, si lo hay, el atajo.
func (a accionBarra) pista(tr *i18n.Translator) string {
	if a.teclas == "" {
		return a.rotulo
	}
	return tr.T("editor.shortcut", i18n.Vars{"label": a.rotulo, "keys": a.teclas})
}

// Prioridades: lo que se queda a la vista en un teléfono (negrita, cursiva, tareas y
// viñetas; luego el bloque y deshacer) y lo que se va antes a «Más».
const (
	prioEsencial = iota + 1
	prioAlta
	prioMedia
	prioFormato
	prioColor
	prioInsertar
	prioHerramienta
)

// accionesBarra son las de la barra en su orden: la barra de arriba de un editor de
// páginas de hoy —deshacer y rehacer, el estilo del texto, negrita y cursiva con el resto
// del formato en «⋯», el color, las tres listas, la alineación y la sangría en un menú, lo
// que se inserta con «+» al final— y las herramientas.
func (e *EditorNota) accionesBarra() []accionBarra {
	t := func(k string) string { return e.tr.T(k, nil) }
	orden := func(o string) func(fyne.CanvasObject) {
		return func(fyne.CanvasObject) { e.Orden(o, "") }
	}
	return []accionBarra{
		{clave: "undo", grupo: grupoHistoria, prioridad: prioAlta, glifo: DeshacerD, rotulo: t("editor.undo"), teclas: ui.AtajoDelSistema("Ctrl+Z"), pulsar: orden("undo")},
		{clave: "redo", grupo: grupoHistoria, prioridad: prioMedia, glifo: RehacerD, rotulo: t("editor.redo"), teclas: ui.AtajoDelSistema("Ctrl+Y"), pulsar: orden("redo")},
		{clave: "bloque", grupo: grupoBloque, prioridad: prioAlta, rotulo: t("editor.block"), teclas: ui.AtajoDelSistema("Ctrl+Alt+0…5"), abre: true, pulsar: e.menuBloque},
		{clave: "b", grupo: grupoFormato, prioridad: prioEsencial, glifo: NegritaD, rotulo: t("editor.bold"), teclas: ui.AtajoDelSistema("Ctrl+B"), pulsar: orden("b")},
		{clave: "i", grupo: grupoFormato, prioridad: prioEsencial, glifo: CursivaD, rotulo: t("editor.italic"), teclas: ui.AtajoDelSistema("Ctrl+I"), pulsar: orden("i")},
		{clave: "formato", grupo: grupoFormato, prioridad: prioFormato, glifo: FormatoD, rotulo: t("editor.moreFormatting"), abre: true, pulsar: e.menuFormato},
		{clave: "color", grupo: grupoColor, prioridad: prioColor, glifo: PaletaD, rotulo: t("editor.textColor"), abre: true, pulsar: e.paletaColor},
		{clave: "vinetas", grupo: grupoListas, prioridad: prioEsencial, glifo: VinetasD, rotulo: t("editor.bulletList"), teclas: "-", pulsar: orden("vinetas")},
		{clave: "numeros", grupo: grupoListas, prioridad: prioMedia, glifo: NumerosD, rotulo: t("editor.numberedList"), teclas: "1.", pulsar: orden("numeros")},
		{clave: "tarea", grupo: grupoListas, prioridad: prioEsencial, glifo: TareaD, rotulo: t("editor.taskList"), teclas: "[ ]", pulsar: orden("tarea")},
		{clave: "alinear", grupo: grupoListas, prioridad: prioColor, glifo: IzquierdaD, rotulo: t("editor.align"), abre: true, pulsar: e.menuAlinear},
		{clave: "enlace", grupo: grupoInsertar, prioridad: prioMedia, glifo: EnlaceD, rotulo: t("editor.linkTool"), teclas: ui.AtajoDelSistema("Ctrl+K"), abre: true, pulsar: func(d fyne.CanvasObject) { llamar1(e.AlEnlazar, d) }},
		{clave: "mencion", grupo: grupoInsertar, prioridad: prioInsertar, glifo: ArrobaD, rotulo: t("editor.mention"), teclas: "@", pulsar: func(fyne.CanvasObject) { e.empezarCon("@") }, disponible: func() bool { return e.gente != nil }},
		{clave: "simbolos", grupo: grupoInsertar, prioridad: prioInsertar, glifo: SimboloD, rotulo: t("editor.symbols"), abre: true, pulsar: e.abrirSimbolos},
		{clave: "tabla", grupo: grupoInsertar, prioridad: prioInsertar, glifo: TablaD, rotulo: t("editor.table"), pulsar: orden("tabla")},
		{clave: "insertar", grupo: grupoInsertar, prioridad: prioEsencial, icono: iconos.Mas, rotulo: t("editor.insert"), teclas: "/", abre: true, pulsar: e.menuInsertar},
		{clave: "buscar", grupo: grupoHerramientas, prioridad: prioHerramienta, icono: iconos.Buscar, rotulo: t("editor.find"), teclas: ui.AtajoDelSistema("Ctrl+F"), abre: true, pulsar: func(fyne.CanvasObject) { e.abrirBuscador() }},
		{clave: "atajos", grupo: grupoHerramientas, prioridad: prioHerramienta, icono: iconos.Teclado, rotulo: t("editor.shortcuts"), teclas: ui.AtajoDelSistema("Ctrl+/"), abre: true, soloTeclado: true, pulsar: func(fyne.CanvasObject) { e.verAtajos() }},
	}
}

// opcionMenu es una entrada de los menús de la barra («⋯» de formato, alineación).
type opcionMenu struct {
	glifo   string
	icono   iconos.Icono
	rotulo  string
	marcada bool
	hacer   func()
}

// mostrarMenu abre un menú de la barra colgando de `disparador`. Las opciones vuelven a
// la escritura al elegirse; `nil` es un separador.
func (e *EditorNota) mostrarMenu(disparador fyne.CanvasObject, rotulo string, opciones []*opcionMenu) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	elementos := make([]fyne.CanvasObject, 0, len(opciones))
	for _, o := range opciones {
		if o == nil {
			elementos = append(elementos, ui.NuevoSeparadorMenu())
			continue
		}
		el := ui.NuevoElementoMenu(o.rotulo, o.icono, func() {
			o.hacer()
			e.reenfocar()
		})
		if o.glifo != "" {
			el.Glifo = iconos.TrazoDeRuta(o.glifo, GlifoEd, tema.Color("text-muted", v))
		}
		el.TamIcono, el.Marcada = GlifoEd, o.marcada
		elementos = append(elementos, el)
	}
	m := ui.NuevoMenu(elementos...)
	m.Rotulo, m.AlInicio = rotulo, true
	m.Mostrar(disparador)
}

// menuFormato es «⋯» del formato: lo que no es negrita ni cursiva.
func (e *EditorNota) menuFormato(disparador fyne.CanvasObject) {
	t := func(k string) string { return e.tr.T(k, nil) }
	marca := func(glifo string, icono iconos.Icono, clave, orden string) *opcionMenu {
		return &opcionMenu{glifo: glifo, icono: icono, rotulo: t(clave), marcada: e.estadoMarca(orden), hacer: func() { e.Orden(orden, "") }}
	}
	e.mostrarMenu(disparador, t("editor.moreFormatting"), []*opcionMenu{
		marca(SubrayadoD, iconos.Ninguno, "editor.underline", "u"),
		marca(TachadoD, iconos.Ninguno, "editor.strike", "s"),
		marca("", iconos.Codigo, "editor.inlineCode", "code"),
		{icono: iconos.Rotulador, rotulo: t("editor.highlight"), hacer: func() { e.Orden("mark", "") }},
		nil,
		{glifo: GomaD, rotulo: t("editor.clearFormat"), hacer: func() { e.Orden("limpiar", "") }},
	})
}

// menuAlinear es la alineación y la sangría en un menú.
func (e *EditorNota) menuAlinear(disparador fyne.CanvasObject) {
	t := func(k string) string { return e.tr.T(k, nil) }
	alinear := func(glifo, clave, valor string) *opcionMenu {
		return &opcionMenu{glifo: glifo, rotulo: t(clave), marcada: e.Activa("alinear", valor), hacer: func() { e.Orden("alinear", valor) }}
	}
	e.mostrarMenu(disparador, t("editor.align"), []*opcionMenu{
		alinear(IzquierdaD, "editor.alignLeft", "left"),
		alinear(CentroD, "editor.alignCenter", "center"),
		alinear(DerechaD, "editor.alignRight", "right"),
		nil,
		{glifo: DesangrarD, rotulo: t("editor.outdent"), hacer: func() { e.Orden("desangrar", "") }},
		{glifo: SangrarD, rotulo: t("editor.indent"), hacer: func() { e.Orden("sangrar", "") }},
	})
}

// empezarCon escribe un disparador («@», «[[») donde está el cursor: abre su menú.
func (e *EditorNota) empezarCon(s string) {
	e.escribiendo = false
	e.escribir(s)
	e.escribiendo = false
	e.reenfocar()
}

// accionIA es la del asistente, al final de las herramientas (ConAsistente).
func (e *EditorNota) accionIA() accionBarra {
	return accionBarra{
		clave: "ia", grupo: grupoHerramientas, prioridad: prioHerramienta, glifo: ChispasD,
		rotulo: e.tr.T("editor.ai", nil), abre: true, pulsar: e.menuIA,
	}
}

// itemBarra es una acción ya puesta: su botón y lo que va en la fila (el botón con su
// pista).
type itemBarra struct {
	a   accionBarra
	bt  *botonBarra
	obj fyne.CanvasObject
}

// barraEd es la barra de formato: los botones, un separador por grupo y «Más».
type barraEd struct {
	e      *EditorNota
	items  []*itemBarra
	seps   map[grupoBarra]*separadorEd
	mas    *botonBarra
	masObj fyne.CanvasObject
	fondo  *canvas.Rectangle
	filete *canvas.Rectangle
	c      *fyne.Container
	// fuera son las acciones recogidas en «Más» en la última colocación.
	fuera []*itemBarra
}

// armarBarra monta la barra con las acciones de `accionesBarra`.
func (e *EditorNota) armarBarra() *fyne.Container {
	e.botones = map[string]*botonBarra{}
	e.barraEd = e.nuevaBarraEd(e.accionesBarra())
	return e.barraEd.c
}

// nuevaBarraEd monta una barra de una fila con esas acciones y su «Más»: la de formato y
// la de tabla son la misma pieza.
func (e *EditorNota) nuevaBarraEd(acciones []accionBarra) *barraEd {
	b := &barraEd{e: e, seps: map[grupoBarra]*separadorEd{}}
	b.fondo, b.filete = canvas.NewRectangle(color.NRGBA{}), canvas.NewRectangle(color.NRGBA{})
	b.mas = nuevoBotonBarra("", iconos.Puntos, e.tr.T("editor.more", nil), nil)
	b.mas.AlPulsar = func() { b.menuMas() }
	b.masObj = b.mas.conPista(b.mas.Rotulo)
	for _, a := range acciones {
		b.poner(a)
	}
	b.c = container.New(b)
	b.rehacer()
	return b
}

// poner añade una acción con su botón.
func (b *barraEd) poner(a accionBarra) {
	e := b.e
	if a.soloTeclado && ui.EsTactil() {
		return
	}
	var bt *botonBarra
	if a.clave == "bloque" {
		e.bloqueBt = nuevoBotonBloque(a.rotulo, nil)
		e.bloqueBt.Compacto = ui.EsTactil()
		for _, x := range BloquesEd {
			e.bloqueBt.anchoRotulos = max(e.bloqueBt.anchoRotulos, ui.Medir(e.tr.T(x.Clave, nil), tema.TextoSM, ui.Normal).Width)
		}
		bt = &e.bloqueBt.botonBarra
	} else {
		bt = nuevoBotonBarra(a.glifo, a.icono, a.rotulo, nil)
	}
	bt.AlPulsar = func() {
		a.pulsar(bt)
		if !a.abre {
			e.reenfocar()
		}
	}
	e.botones[a.clave] = bt
	obj := bt.conPista(a.pista(e.tr))
	if a.clave == "bloque" {
		obj = e.bloqueBt.conPista(a.pista(e.tr))
	}
	b.items = append(b.items, &itemBarra{a: a, bt: bt, obj: obj})
	if _, hay := b.seps[a.grupo]; !hay {
		b.seps[a.grupo] = nuevoSeparadorEd()
	}
}

// rehacer pone los objetos del contenedor: fondo, filete, botones, separadores y «Más».
func (b *barraEd) rehacer() {
	objs := []fyne.CanvasObject{b.fondo, b.filete}
	for _, it := range b.items {
		objs = append(objs, it.obj)
	}
	for g := grupoHistoria; g <= grupoHerramientas; g++ {
		if s := b.seps[g]; s != nil {
			objs = append(objs, s)
		}
	}
	objs = append(objs, b.masObj)
	b.c.Objects = objs
	b.c.Refresh()
}

// medidas de la fila: el hueco entre botones y el ancho de un separador.
func anchoSeparadorEd() float32 { return tema.Borde + 2*MargenSeparadorE }

// altoFilaEd es el alto de la fila de botones: el de un botón.
func altoFilaEd() float32 { return ladoBotonEd() }

// reparto decide qué acciones se ven con `cabe` de ancho útil: todas si caben; si no,
// las de más prioridad que quepan junto a «Más». Devuelve cuáles se ven.
func (b *barraEd) reparto(cabe float32) (visibles []bool, conMas bool) {
	n := len(b.items)
	visibles = make([]bool, n)
	anchos := make([]float32, n)
	for i, it := range b.items {
		visibles[i], anchos[i] = it.a.vale(), it.obj.MinSize().Width
	}
	if b.anchoDe(anchos, visibles, false) <= cabe {
		return visibles, false
	}
	// Se recogen de una en una: primero las de más número de prioridad y, a igual
	// prioridad, las de más a la derecha.
	orden := make([]int, n)
	for i := range orden {
		orden[i] = i
	}
	slices.SortStableFunc(orden, func(x, y int) int {
		return cmp.Or(cmp.Compare(b.items[y].a.prioridad, b.items[x].a.prioridad), cmp.Compare(y, x))
	})
	for _, i := range orden {
		if b.anchoDe(anchos, visibles, true) <= cabe {
			break
		}
		visibles[i] = false
	}
	return visibles, true
}

// anchoDe es lo que mide la fila con esas acciones a la vista: los botones, un
// separador entre dos grupos con algo, los huecos y, si va, «Más».
func (b *barraEd) anchoDe(anchos []float32, visibles []bool, conMas bool) float32 {
	var ancho float32
	piezas := 0
	grupo := grupoBarra(-1)
	for i, it := range b.items {
		if !visibles[i] {
			continue
		}
		if grupo >= 0 && it.a.grupo != grupo {
			ancho += anchoSeparadorEd()
			piezas++
		}
		grupo = it.a.grupo
		ancho += anchos[i]
		piezas++
	}
	if conMas {
		ancho += b.masObj.MinSize().Width
		piezas++
	}
	if piezas > 1 {
		ancho += float32(piezas-1) * HuecoBarraEd
	}
	return ancho
}

// MinSize: «Más» y el relleno de ancho; de alto, una fila.
func (b *barraEd) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(b.masObj.MinSize().Width+2*PXBarraEd, b.AltoPara(nil, 0))
}

// AltoPara es siempre el de una fila: la barra no envuelve.
func (b *barraEd) AltoPara([]fyne.CanvasObject, float32) float32 {
	return altoFilaEd() + 2*PYBarraEd + tema.Borde
}

// Layout coloca la fila: los botones a la vista en su orden, un separador entre grupos
// y «Más» pegado a la derecha si hace falta.
func (b *barraEd) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	b.fondo.Move(fyne.NewPos(0, 0))
	b.fondo.Resize(t)
	b.filete.Move(fyne.NewPos(0, t.Height-tema.Borde))
	b.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	cabe := t.Width - 2*PXBarraEd
	visibles, conMas := b.reparto(cabe)
	for _, s := range b.seps {
		s.Hide()
	}
	alto := t.Height - tema.Borde
	x := float32(PXBarraEd)
	grupo := grupoBarra(-1)
	b.fuera = b.fuera[:0]
	for i, it := range b.items {
		if !visibles[i] {
			it.obj.Hide()
			if it.a.vale() {
				b.fuera = append(b.fuera, it)
			}
			continue
		}
		if grupo >= 0 && it.a.grupo != grupo {
			s := b.seps[it.a.grupo]
			s.Show()
			s.Move(fyne.NewPos(x, 0))
			s.Resize(fyne.NewSize(anchoSeparadorEd(), alto))
			x += anchoSeparadorEd() + HuecoBarraEd
		}
		grupo = it.a.grupo
		m := it.obj.MinSize()
		it.obj.Show()
		it.obj.Move(fyne.NewPos(x, (alto-m.Height)/2))
		it.obj.Resize(m)
		x += m.Width + HuecoBarraEd
	}
	b.masObj.Hide()
	if conMas {
		b.masObj.Show()
		m := b.masObj.MinSize()
		b.masObj.Move(fyne.NewPos(t.Width-PXBarraEd-m.Width, (alto-m.Height)/2))
		b.masObj.Resize(m)
	}
}

// disparadorDe es de dónde colgar lo que abre la acción `clave` cuando llega por su atajo:
// su botón si se ve, «Más» si está recogida y, sin barra a la vista, el cursor.
func (e *EditorNota) disparadorDe(clave string) fyne.CanvasObject {
	if b := e.botones[clave]; b != nil && e.barraEd != nil {
		for _, it := range e.barraEd.items {
			if it.bt == b && it.obj.Visible() && e.barra.Visible() {
				return b
			}
		}
		if e.barraEd.masObj.Visible() && e.barra.Visible() && !e.barra.Size().IsZero() {
			return e.barraEd.mas
		}
	}
	return e.anclaCursor()
}

// anclaCursor es un ancla en la raya del cursor, de su alto: de ahí cuelgan los paneles
// que se abren escribiendo («/» → «Símbolo…») o por un atajo sin barra a la vista.
func (e *EditorNota) anclaCursor() fyne.CanvasObject {
	x, arriba, alto := e.zona.cajaCursor()
	a := e.zona.anclaEn(fyne.NewPos(PXZonaEd+x, PYZonaEd+arriba))
	a.Resize(fyne.NewSize(1, alto))
	return a
}

// ponerAccion añade una acción a la barra ya armada (el asistente, que llega después).
func (e *EditorNota) ponerAccion(a accionBarra) {
	if e.barraEd == nil || e.botones[a.clave] != nil {
		return
	}
	e.barraEd.poner(a)
	e.barraEd.rehacer()
}

// menuMas abre «Más» con las acciones recogidas, por grupos y con su estado: la marca
// en lo que está puesto donde está el cursor, apagado lo que no se puede hacer.
func (b *barraEd) menuMas() {
	e := b.e
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var elementos []fyne.CanvasObject
	grupo := grupoBarra(-1)
	for _, it := range b.fuera {
		if grupo >= 0 && it.a.grupo != grupo {
			elementos = append(elementos, ui.NuevoSeparadorMenu())
		}
		grupo = it.a.grupo
		a, bt := it.a, it.bt
		rotulo := a.rotulo
		if a.clave == "bloque" && e.bloqueBt != nil {
			rotulo += ": " + e.bloqueBt.Texto
		}
		el := ui.NuevoElementoMenu(rotulo, iconos.Ninguno, func() {
			a.pulsar(b.mas)
			if !a.abre {
				e.reenfocar()
			}
		})
		el.Rotulo = a.pista(e.tr)
		switch {
		case a.glifo != "":
			el.Glifo = iconos.TrazoDeRuta(a.glifo, GlifoEd, tema.Color("text-muted", v))
		case a.icono != iconos.Ninguno:
			el.Icono = a.icono
		default:
			el.Glifo = iconos.TrazoDeRuta(TituloD, GlifoEd, tema.Color("text-muted", v))
		}
		el.TamIcono = GlifoEd
		el.Marcada = bt.ConEstado && bt.Activo
		el.Inerte = bt.Inerte
		elementos = append(elementos, el)
	}
	if len(elementos) == 0 {
		return
	}
	m := ui.NuevoMenu(elementos...)
	m.Rotulo = e.tr.T("editor.more", nil)
	m.Mostrar(b.mas)
}

// ---- Botones de la barra ----

// botonBarra es BOTON_DE_BARRA: 28 × 28 (44 × 44 en un aparato táctil), plano,
// `rounded-sm`, el glifo de 16 en `text-muted` que pasa a `text` al pasar con
// `bg-bg-subtle` detrás; ENCENDIDO (`aria-pressed`) con `bg-selected` y el glifo en
// `primary-text`, que se distingue del «encima» también de un vistazo; y al 40 %
// apagado.
//
// NO es enfocable, a propósito: en la web el botón hace `preventDefault` en el
// `mousedown` para que el cuerpo no pierda el foco ni la selección, y aquí eso es no
// aceptar el foco.
type botonBarra struct {
	widget.BaseWidget
	glifo  string
	icono  iconos.Icono
	Rotulo string
	// ConEstado es `aria-pressed`: Activo pinta el hundido.
	ConEstado, Activo, Inerte bool
	AlPulsar                  func()
	// Glifo y Radio cambian el glifo de 16 y el `rounded-sm` (cero: esos).
	Glifo, Radio float32
	encima       bool
	// pista es la ayuda con el atajo, en el escritorio (ver conPista).
	pista *ui.Pista
}

var (
	_ fyne.Tappable     = (*botonBarra)(nil)
	_ desktop.Hoverable = (*botonBarra)(nil)
)

func nuevoBotonBarra(glifo string, icono iconos.Icono, rotulo string, f func()) *botonBarra {
	b := &botonBarra{glifo: glifo, icono: icono, Rotulo: rotulo, AlPulsar: f}
	b.ExtendBaseWidget(b)
	return b
}

// conPista envuelve el botón con su ayuda: es lo que va en la fila. La pista NO recibe el
// puntero (el botón está encima y es él quien se lo pasa), así que el `:hover` sigue
// siendo del botón.
func (b *botonBarra) conPista(texto string) fyne.CanvasObject {
	if texto == "" {
		return b
	}
	b.pista = ui.NuevaPista(texto, b)
	return b.pista
}

func (b *botonBarra) AccessibilityLabel() string { return b.Rotulo }
func (b *botonBarra) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleButton
}

func (b *botonBarra) Tapped(*fyne.PointEvent) {
	if b.pista != nil {
		b.pista.Cerrar()
	}
	if !b.Inerte {
		llamar0(b.AlPulsar)
	}
}

func (b *botonBarra) MouseIn(ev *desktop.MouseEvent) {
	b.encima = true
	b.Refresh()
	// La pista, solo con ratón: en un teléfono no hay «encima».
	if b.pista != nil && !b.Inerte && !ui.EsTactil() {
		if !b.pista.Abierta() {
			b.pista.Portal = ui.PortalDePistas(b)
		}
		b.pista.MouseIn(ev)
	}
}
func (b *botonBarra) MouseMoved(*desktop.MouseEvent) {}
func (b *botonBarra) MouseOut() {
	b.encima = false
	b.Refresh()
	if b.pista != nil {
		b.pista.MouseOut()
	}
}

func (b *botonBarra) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonBarraRenderer{b: b, fondo: canvas.NewRectangle(color.NRGBA{})}
	r.fondo.CornerRadius = cmp.Or(b.Radio, tema.RadioSM)
	if b.glifo != "" {
		r.dibujo = iconos.TrazoDeRuta(b.glifo, b.lado(), tema.Color("text-muted", v))
	} else {
		r.dibujo = iconos.Trazo(b.icono, b.lado(), "text-muted", v)
	}
	r.Refresh()
	return r
}

type botonBarraRenderer struct {
	b      *botonBarra
	fondo  *canvas.Rectangle
	dibujo *iconos.Dibujo
}

// ladoBotonEd es el lado de un botón de la barra: 28, o 44 con el dedo.
func ladoBotonEd() float32 { return ui.SueloTactil(LadoBotonEd) }

func (r *botonBarraRenderer) MinSize() fyne.Size {
	return fyne.NewSize(ladoBotonEd(), ladoBotonEd())
}

func (r *botonBarraRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fondo, r.dibujo.Objeto()}
}
func (r *botonBarraRenderer) Destroy() {}
func (r *botonBarraRenderer) Layout(t fyne.Size) {
	// El fondo es el de 28 aunque el botón mida 44: el blanco que se pulsa crece, el
	// botón que se ve no.
	l := min(t.Width, t.Height, LadoBotonEd)
	r.fondo.Move(fyne.NewPos((t.Width-l)/2, (t.Height-l)/2))
	r.fondo.Resize(fyne.NewSize(l, l))
	g := r.b.lado()
	r.dibujo.Colocar((t.Width-g)/2, (t.Height-g)/2, g)
}

func (b *botonBarra) lado() float32 { return cmp.Or(b.Glifo, GlifoEd) }

// tintaBoton es la tinta y el fondo de un botón de la barra según su estado.
func tintaBoton(b *botonBarra, v tema.Variante) (tinta, fondo color.NRGBA) {
	tinta, fondo = tema.Color("text-muted", v), color.NRGBA{}
	switch {
	case b.Inerte:
	case b.ConEstado && b.Activo:
		tinta, fondo = tema.Color("primary-text", v), tema.Color("selected", v)
	case b.encima:
		tinta, fondo = tema.Color("text", v), tema.Color("bg-subtle", v)
	}
	if b.Inerte {
		tinta = velo(tinta, 0.4)
	}
	return tinta, fondo
}

func (r *botonBarraRenderer) Refresh() {
	tinta, fondo := tintaBoton(r.b, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.fondo.FillColor = fondo
	r.fondo.Refresh()
	r.dibujo.TenirCon(tinta)
}

// botonBloque es el desplegable de «Estilo del texto»: `w-auto gap-1 px-1.5 text-sm` con
// el glifo `heading` de 14, el rótulo (`max-w-24 truncate`) y el chevron de 12.
//
// Mide SIEMPRE lo que su rótulo más largo: con el ancho del actual, la barra entera se
// corría al pasar de «Párrafo» a «Bloque de código». Compacto (en un aparato táctil) es
// solo el glifo y el chevron: el estilo actual se ve en el menú, marcado.
type botonBloque struct {
	botonBarra
	Texto    string
	Compacto bool
	// anchoRotulos es lo que mide el rótulo más largo de los ocho.
	anchoRotulos float32
}

func nuevoBotonBloque(rotulo string, f func()) *botonBloque {
	b := &botonBloque{}
	b.Rotulo, b.AlPulsar = rotulo, f
	b.ExtendBaseWidget(b)
	return b
}

// conPista es la del botón de bloque: la pista envuelve al desplegable entero.
func (b *botonBloque) conPista(texto string) fyne.CanvasObject {
	b.pista = ui.NuevaPista(texto, b)
	return b.pista
}

func (b *botonBloque) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonBloqueRenderer{
		b:      b,
		fondo:  canvas.NewRectangle(color.NRGBA{}),
		glifo:  iconos.TrazoDeRuta(TituloD, 14, tema.Color("text-muted", v)),
		flecha: iconos.Trazo(iconos.ChevronAbajo, 12, "text-muted", v),
		texto:  ui.Texto(b.Texto, tema.TextoSM, ui.Normal, tema.Color("text-muted", v)),
	}
	r.fondo.CornerRadius = tema.RadioSM
	r.Refresh()
	return r
}

type botonBloqueRenderer struct {
	b             *botonBloque
	fondo         *canvas.Rectangle
	glifo, flecha *iconos.Dibujo
	texto         *canvas.Text
}

func (r *botonBloqueRenderer) anchoTexto() float32 {
	if r.b.Compacto {
		return 0
	}
	return min(max(r.b.anchoRotulos, ui.Medir(r.b.Texto, tema.TextoSM, ui.Normal).Width), AnchoMaxBloqueEd)
}

func (r *botonBloqueRenderer) MinSize() fyne.Size {
	hueco := float32(0.25 * tema.Rem)
	ancho := 2*PXBarraEd + 14 + hueco + 12
	if t := r.anchoTexto(); t > 0 {
		ancho += t + hueco
	}
	return fyne.NewSize(max(ancho, ladoBotonEd()), ladoBotonEd())
}

func (r *botonBloqueRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fondo, r.glifo.Objeto(), r.texto, r.flecha.Objeto()}
}
func (r *botonBloqueRenderer) Destroy() {}

func (r *botonBloqueRenderer) Layout(t fyne.Size) {
	hueco := float32(0.25 * tema.Rem)
	alto := min(t.Height, LadoBotonEd)
	r.fondo.Move(fyne.NewPos(0, (t.Height-alto)/2))
	r.fondo.Resize(fyne.NewSize(t.Width, alto))
	contenido := 14 + hueco + 12
	if a := r.anchoTexto(); a > 0 {
		contenido += a + hueco
	}
	x := max((t.Width-contenido)/2, PXBarraEd)
	if !r.b.Compacto {
		x = PXBarraEd
	}
	r.glifo.Colocar(x, (t.Height-14)/2, 14)
	x += 14 + hueco
	r.texto.Hidden = r.b.Compacto
	if !r.b.Compacto {
		ui.Truncar(r.texto, r.b.Texto, tema.TextoSM, ui.Normal, AnchoMaxBloqueEd)
		ui.ColocarTexto(r.texto, tema.TextoSM, x, (t.Height-tema.TextoSM.Linea)/2)
		x += r.anchoTexto() + hueco
	}
	r.flecha.Colocar(x, (t.Height-12)/2, 12)
}

func (r *botonBloqueRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta, fondo := tema.Color("text-muted", v), color.NRGBA{}
	if r.b.encima {
		tinta, fondo = tema.Color("text", v), tema.Color("bg-subtle", v)
	}
	r.fondo.FillColor = fondo
	r.fondo.Refresh()
	r.glifo.TenirCon(tinta)
	r.flecha.TenirCon(tinta)
	r.texto.Text, r.texto.Color = r.b.Texto, tinta
	r.texto.Refresh()
	r.Layout(r.b.Size())
}

// separadorEd es SEPARADOR_DE_BARRA: un pelo de 1 × 17,5 con `mx-1`.
type separadorEd struct{ widget.BaseWidget }

func nuevoSeparadorEd() *separadorEd {
	s := &separadorEd{}
	s.ExtendBaseWidget(s)
	return s
}

func (s *separadorEd) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return &separadorRenderer{r: canvas.NewRectangle(tema.Color("border", v))}
}

type separadorRenderer struct{ r *canvas.Rectangle }

func (s *separadorRenderer) MinSize() fyne.Size {
	return fyne.NewSize(anchoSeparadorEd(), AltoSeparadorEd)
}
func (s *separadorRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{s.r} }
func (s *separadorRenderer) Destroy()                     {}
func (s *separadorRenderer) Layout(t fyne.Size) {
	s.r.Move(fyne.NewPos(MargenSeparadorE, (t.Height-AltoSeparadorEd)/2))
	s.r.Resize(fyne.NewSize(tema.Borde, AltoSeparadorEd))
}

func (s *separadorRenderer) Refresh() {
	s.r.FillColor = tema.Color("border", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	s.r.Refresh()
}

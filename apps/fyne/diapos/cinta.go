// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Registro es la cinta de Diapos: las pestañas genéricas del cromo con «Inicio»
// sustituida por la de la presentación (`InicioSlides.tsx`) y, de la taxonomía común con
// la web, las de la presentación: «Insertar» (`insertar.go`), «Diseño» (tamaño y patrón)
// y «Presentar».
func Registro(e *Editor) *editor.Registro {
	r := editor.Genericas()
	r.Sustituir("inicio", e.cuerpoInicio)
	r.Anadir("insertar", e.cuerpoInsertar)
	r.Anadir("diseno", e.cuerpoDiseno)
	r.Anadir("presentar", e.cuerpoPresentar)
	return r
}

// Anchos de los selectores de InicioSlides, en px como los escribe la web.
const (
	anchoSelectorDisposicion float32 = 132
	anchoSelectorFuente      float32 = 128
	anchoSelectorTalla       float32 = 60
	anchoSelectorInterlinea  float32 = 60
	anchoSelectorEstilo      float32 = 104
)

// cuerpoInicio es `InicioSlidesTab`, grupo a grupo y en su orden.
//
// Los grupos de FUENTE y PÁRRAFO actúan sobre el texto de la caja que se está
// escribiendo (`exec`), y enseñan el formato de lo elegido en ella (`fmt`). Sin caja en
// edición van apagados: en la web el `exec` no tiene dónde actuar y el botón no hace
// nada, y un botón que se pulsa y no hace nada es como se concluye que la aplicación
// está rota.
func (e *Editor) cuerpoInicio(c *editor.Contexto) fyne.CanvasObject {
	ro := c.SoloLectura
	selector := func(clave string, ancho float32, ops []editor.Opcion, valor string, f func(string)) *editor.SelectorCinta {
		s := editor.NuevoSelectorCinta(c.Traducir(clave), ancho, ops, valor, f)
		s.Inerte = f == nil || ro
		return s
	}

	// Disposición: el selector hace de menú de comando; su primera opción es el rótulo
	// y vuelve a ella tras aplicar una plantilla.
	disposiciones := make([]editor.Opcion, 0, 1+len(Disposiciones))
	disposiciones = append(disposiciones, editor.Opcion{Valor: "", Rotulo: c.Traducir("slides.layout.placeholder")})
	for _, d := range Disposiciones {
		disposiciones = append(disposiciones, editor.Opcion{Valor: d.ID, Rotulo: c.Traducir(d.Clave)})
	}
	var disp *editor.SelectorCinta
	disp = selector("slides.layout", anchoSelectorDisposicion, disposiciones, "", func(v string) {
		if v != "" {
			e.AplicarDisposicion(v)
		}
		disp.Valor = ""
		disp.Refresh()
	})
	nueva := editor.NuevoBotonCinta(iconos.EdDiapoNueva, c.Traducir("slides.new"), e.NuevaDiapositiva)
	nueva.Inerte = ro

	familias := []string{"Inter", "Arial", "Georgia", "Times New Roman", "Courier New", "JetBrains Mono"}
	fuentes := make([]editor.Opcion, 0, len(familias))
	for _, f := range familias {
		fuentes = append(fuentes, editor.Opcion{Valor: f, Rotulo: f})
	}
	puntos := []string{"8", "10", "12", "14", "18", "24", "36"}
	tallas := make([]editor.Opcion, 0, len(puntos))
	for i, t := range puntos {
		tallas = append(tallas, editor.Opcion{Valor: strconv.Itoa(i + 1), Rotulo: t})
	}
	valores := []float64{1, 1.15, 1.5, 2}
	interlineas := make([]editor.Opcion, 0, len(valores))
	for _, v := range valores {
		interlineas = append(interlineas, editor.Opcion{
			Valor: strconv.FormatFloat(v, 'f', -1, 64), Rotulo: i18n.FormatNumber(e.Idioma, v),
		})
	}
	estilos := []editor.Opcion{
		{Valor: "p", Rotulo: c.Traducir("style.normal")},
		{Valor: "h1", Rotulo: c.Traducir("style.h1")},
		{Valor: "h2", Rotulo: c.Traducir("style.h2")},
		{Valor: "h3", Rotulo: c.Traducir("style.h3")},
	}

	organizar := editor.NuevoBotonCinta(iconos.EdOrganizar, c.Traducir("slides.arrange"), nil)
	organizar.AlPulsar = func() { e.abrirOrganizar(organizar) }
	organizar.Inerte = ro

	return editor.Grupos(
		editor.NuevoGrupo(c.Traducir("group.slide"), nueva, disp),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.font"), e.grupoFuente(c, fuentes, tallas, selector)...),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.paragraph"), e.grupoParrafo(c, interlineas, estilos, selector)...),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.arrange"), organizar),
	)
}

// cuerpoDiseno es `DisenoSlidesTab`: el tamaño de la presentación y su patrón. Salieron de
// «Inicio» al fijarse la taxonomía de pestañas.
func (e *Editor) cuerpoDiseno(c *editor.Contexto) fyne.CanvasObject {
	ro := c.SoloLectura
	tamano := editor.NuevoBotonCinta(iconos.EdTamanoPagina, c.Traducir("slides.size"), nil)
	tamano.AlPulsar = func() { e.abrirTamano(tamano) }
	tamano.Inerte = ro
	patron := editor.NuevoBotonCinta(iconos.EdPatron, c.Traducir("slides.master"), e.EditarPatron)
	patron.Inerte = ro
	grupos := []fyne.CanvasObject{
		editor.NuevoGrupo(c.Traducir("group.design"), tamano),
	}
	// El tema común y la marca de la organización, tras «Tamaño» y antes del patrón.
	if c.Temas != nil {
		grupos = append(grupos, c.Temas.Grupo(c)...)
	}
	grupos = append(grupos, editor.SeparadorGrupos(), editor.NuevoGrupo(c.Traducir("group.master"), patron))
	return editor.Grupos(grupos...)
}

// cuerpoPresentar es `PresentarTab`: arrancar la presentación, desde la diapositiva activa
// o con la vista del moderador.
func (e *Editor) cuerpoPresentar(c *editor.Contexto) fyne.CanvasObject {
	presentar := editor.NuevoBotonCinta(iconos.EdPresentar, c.Traducir("slides.present"), func() { e.Presentar(false) })
	moderador := editor.NuevoBotonCinta(iconos.EdCompleta, c.Traducir("slides.presenterView"), func() { e.Presentar(true) })
	botones := []fyne.CanvasObject{presentar, moderador}
	if e.Cast != nil && e.Cast.Cliente != nil {
		botones = append(botones, editor.NuevoBotonCinta(iconos.EdEmitirCast, c.Traducir("slides.cast.present"), e.PresentarConCast))
	}
	return editor.Grupos(
		editor.NuevoGrupo(c.Traducir("group.slideshow"), botones...),
	)
}

// grupoParrafo es «Párrafo»: la alineación, las listas, el interlineado y el estilo del
// bloque, sobre los bloques de lo elegido en la caja que se escribe, que enseñan los
// suyos; sin caja, apagados.
//
// Las listas no llevan viñeta ni número, como en la web: el reset de Tailwind les quita
// `list-style` y la hoja del lienzo no se lo devuelve. Se ven sangradas.
func (e *Editor) grupoParrafo(c *editor.Contexto, interlineas, estilos []editor.Opcion,
	selector func(string, float32, []editor.Opcion, string, func(string)) *editor.SelectorCinta,
) []fyne.CanvasObject {
	f := e.Formato()
	// Como en la web, encendido también sin caja abierta: allí el `exec` no tiene dónde
	// actuar y no hace nada, y la alineación de por defecto sale pulsada (`fmt.align`).
	activo := !c.SoloLectura
	boton := func(i iconos.Ed, clave string, puesto bool, cmd editor.Comando) fyne.CanvasObject {
		b := c.Boton(i, clave, puesto, cmd)
		b.Inerte = b.Inerte || !activo
		return b
	}
	estilo := "p"
	if f.Titulo == "h1" || f.Titulo == "h2" || f.Titulo == "h3" {
		estilo = f.Titulo
	}
	var enInterlinea, enEstilo func(string)
	if activo {
		enInterlinea = func(v string) {
			e.interlinea = v
			c.Mandar(editor.Interlineado, v)
		}
		enEstilo = func(v string) { c.Mandar(editor.Titulo, v) }
	}
	return []fyne.CanvasObject{
		boton(iconos.EdAlinearIzq, "align.left", f.Alineacion == editor.Izquierda, editor.AlinearIzq),
		boton(iconos.EdAlinearCentro, "align.center", f.Alineacion == editor.Centro, editor.AlinearCen),
		boton(iconos.EdAlinearDer, "align.right", f.Alineacion == editor.Derecha, editor.AlinearDer),
		boton(iconos.EdListaPuntos, "list.bullet", f.Puntos, editor.ListaPuntos),
		boton(iconos.EdListaNumeros, "list.number", f.Numeros, editor.ListaNumero),
		// El último aplicado, y «1,25» (el de `.sytue-slidebox`, que no está entre las
		// opciones) hasta que se aplique uno, como el `useState('1.25')` de la web.
		selector("paragraph.lineHeight", anchoSelectorInterlinea, interlineas, e.interlinea, enInterlinea),
		selector("paragraph.style", anchoSelectorEstilo, estilos, estilo, enEstilo),
	}
}

// grupoFuente es «Fuente»: la familia, el peldaño de tamaño, las cuatro marcas, el color,
// el resaltado y «Borrar formato», sobre lo elegido en la caja que se escribe.
//
// La familia y el tamaño enseñan los de lo elegido (o «Inter» y «12» si no son de la
// lista, como la web); el color y el resaltado, el último aplicado, porque `FormatState`
// no los expone («comportamiento clásico de suite ofimática»).
func (e *Editor) grupoFuente(c *editor.Contexto, fuentes, tallas []editor.Opcion,
	selector func(string, float32, []editor.Opcion, string, func(string)) *editor.SelectorCinta,
) []fyne.CanvasObject {
	f := e.Formato()
	activo := !c.SoloLectura // como «Párrafo»: encendido también sin caja abierta
	mandar := func(cmd editor.Comando) func(string) {
		if !activo {
			return nil
		}
		return func(v string) { c.Mandar(cmd, v) }
	}
	fuente := "Inter"
	if slices.ContainsFunc(fuentes, func(o editor.Opcion) bool { return o.Valor == f.Fuente }) {
		fuente = f.Fuente
	}
	talla := "3"
	if len(f.Tamano) == 1 && f.Tamano[0] >= '1' && f.Tamano[0] <= '7' {
		talla = f.Tamano
	}
	marca := func(i iconos.Ed, clave string, puesta bool, cmd editor.Comando) fyne.CanvasObject {
		b := c.Boton(i, clave, puesta, cmd)
		b.Inerte = b.Inerte || !activo
		return b
	}
	muestra := func(i iconos.Ed, clave string, actual *string, paleta []string, cmd editor.Comando) fyne.CanvasObject {
		m := editor.NuevaMuestraCinta(c, i, c.Traducir(clave), *actual, paleta, func(v string) {
			*actual = v
			c.Mandar(cmd, v)
		})
		m.Inerte = !activo
		return m
	}
	return []fyne.CanvasObject{
		selector("font.family", anchoSelectorFuente, fuentes, fuente, mandar(editor.Fuente)),
		selector("font.size", anchoSelectorTalla, tallas, talla, mandar(editor.Tamano)),
		marca(iconos.EdNegrita, "format.bold", f.Negrita, editor.Negrita),
		marca(iconos.EdCursiva, "format.italic", f.Cursiva, editor.Cursiva),
		marca(iconos.EdSubrayado, "format.underline", f.Subrayado, editor.Subrayado),
		marca(iconos.EdTachado, "format.strike", f.Tachado, editor.Tachado),
		muestra(iconos.EdColorTexto, "format.textColor", &e.colorTexto, nil, editor.ColorTexto),
		muestra(iconos.EdResaltado, "format.highlight", &e.resalte, editor.PaletaResaltado, editor.Resaltado),
		marca(iconos.EdSinFormato, "format.clear", false, editor.SinFormato),
	}
}

// Presentar abre el modo presentación desde la diapositiva activa, que es lo que hace
// el botón de la cinta de la web.
func (e *Editor) Presentar(moderador bool) {
	Presentar(e.m, e.activa, moderador, e.Idioma, e.ctx.T)
}

// ---- Organizar objetos ----

// opsOrganizar son ORDER_ITEMS, ALIGN_ITEMS y DIST_ITEMS, con su cabecera.
var opsOrganizar = []struct {
	cabecera string
	ops      [][2]string
}{
	{"slides.arrange.order", [][2]string{
		{"front", "slides.order.front"},
		{"forward", "slides.order.forward"},
		{"backward", "slides.order.backward"},
		{"back", "slides.order.back"},
	}},
	{"slides.arrange.align", [][2]string{
		{"align-left", "align.left"},
		{"align-hcenter", "align.hcenter"},
		{"align-right", "align.right"},
		{"align-top", "align.top"},
		{"align-vmiddle", "align.vmiddle"},
		{"align-bottom", "align.bottom"},
	}},
	{"slides.arrange.distribute", [][2]string{{"dist-h", "slides.distribute.h"}, {"dist-v", "slides.distribute.v"}}},
}

// abrirOrganizar es `OrganizarMenu`: el emergente `w-56 p-1` con los comandos de orden,
// alineación, distribución y grupos, cada uno encendido según la selección (ordenar y
// alinear con una, distribuir con tres, agrupar con dos, desagrupar si hay grupo).
func (e *Editor) abrirOrganizar(disparador *editor.BotonCinta) {
	cuantas := len(e.explicita())
	hayGrupo := false
	for _, id := range e.efectiva() {
		if c := e.m.caja(e.objetivo(), id); c != nil && c.Grupo != "" {
			hayGrupo = true
		}
	}
	var em *ui.Emergente
	item := func(op, clave string, encendido bool) fyne.CanvasObject {
		it := ui.NuevoElementoMenu(e.t(clave), iconos.Ninguno, func() {
			em.Cerrar()
			e.Organizar(op)
		})
		it.Denso, it.Inerte = true, !encendido
		return it
	}
	var filas []fyne.CanvasObject
	for k, g := range opsOrganizar {
		filas = append(filas, nuevaCabeceraMenu(e.t(g.cabecera)))
		for _, o := range g.ops {
			filas = append(filas, item(o[0], o[1], []bool{cuantas >= 1, cuantas >= 1, cuantas >= 3}[k]))
		}
	}
	filas = append(filas, ui.NuevoSeparadorMenu(),
		item("group", "slides.group", cuantas >= 2), item("ungroup", "slides.ungroup", hayGrupo))
	if cuantas < 2 {
		// `sytue-meta-dense px-2.5 pt-1.5 pb-1 leading-tight`.
		ancho := AnchoMenuOrganizar - 2*(tema.Borde+rellenoMenu) - 2*tema.Espacio(2.5)
		pista := ui.NuevoParrafo(e.t("slides.arrange.hint"), tema.Escalon{Tam: tema.Texto2XS.Tam, Linea: tema.Texto2XS.Tam * 1.25},
			ui.Normal, "text-subtle", e.variante()).A(ancho)
		filas = append(filas, container.New(&rellenoCuerpo{arriba: tema.Espacio(1.5), abajo: tema.Espacio(1), lados: tema.Espacio(2.5)}, pista))
	}
	disparador.Activo = true
	disparador.Refresh()
	em = ui.MostrarEmergente(disparador, &columnaMenu{filas: filas, ancho: AnchoMenuOrganizar}, rellenoMenu)
	em.AlRecogerse = func() { disparador.Activo = false; disparador.Refresh() }
}

// AnchoMenuOrganizar es el `w-56` del menú.
var AnchoMenuOrganizar = tema.Espacio(56)

// ---- Tamaño de diapositiva ----

// Medidas del menú de tamaño (`TamanoMenu` de InicioSlides.tsx): `w-60 p-1` y la
// cabecera `sytue-overline px-2.5 pb-0.5 pt-1.5`.
var (
	AnchoMenuTamano = tema.Espacio(60)
	rellenoMenu     = tema.Espacio(1)
)

// abrirTamano despliega el menú de tamaño: 16:9 y 4:3 se aplican al instante;
// «Personalizado» enseña ancho, alto y unidad, y se confirma con «Aplicar».
func (e *Editor) abrirTamano(disparador *editor.BotonCinta) {
	actual := "16:9"
	if t := e.m.Tamano; t != nil {
		actual = t.Preset
	}
	var em *ui.Emergente
	var filas []fyne.CanvasObject
	var rehacer func(modo string)
	rehacer = func(modo string) {
		filas = []fyne.CanvasObject{nuevaCabeceraMenu(e.t("slides.size.heading"))}
		for _, m := range []struct{ id, clave string }{
			{"16:9", "slides.size.wide"}, {"4:3", "slides.size.standard"}, {"custom", "slides.size.custom"},
		} {
			it := ui.NuevoElementoMenu(e.t(m.clave), iconos.Ninguno, func() {
				switch m.id {
				case "custom":
					rehacer("custom")
				case "4:3":
					em.Cerrar()
					e.PonerTamano(&Tamano{Preset: "4:3"})
				default:
					em.Cerrar()
					e.PonerTamano(nil)
				}
			})
			it.Denso, it.Marcada = true, m.id == modo
			filas = append(filas, it)
		}
		if modo == "custom" {
			filas = append(filas, e.tamanoPersonalizado(func() { em.Cerrar() }))
		}
		if em != nil {
			em.Cerrar()
			em = ui.MostrarEmergente(disparador, &columnaMenu{filas: filas, ancho: AnchoMenuTamano}, rellenoMenu)
			em.AlRecogerse = func() { disparador.Activo = false; disparador.Refresh() }
		}
	}
	rehacer(actual)
	disparador.Activo = true // `aria-expanded:bg-selected`
	disparador.Refresh()
	em = ui.MostrarEmergente(disparador, &columnaMenu{filas: filas, ancho: AnchoMenuTamano}, rellenoMenu)
	em.AlRecogerse = func() { disparador.Activo = false; disparador.Refresh() }
}

// tamanoPersonalizado es la parte de «Personalizado»: ancho y alto (px o cm), el botón
// de aplicar y la pista. Solo cuenta la PROPORCIÓN: las cajas van en %.
//
// ponytail: los campos y los dos botones de unidad son los de formulario de la suite
// en su talla pequeña (21 y 24 de alto) y no el `h-7` del campo de la cinta; el menú se
// usa una vez por presentación y la diferencia es de 3,5 px.
func (e *Editor) tamanoPersonalizado(cerrar func()) fyne.CanvasObject {
	w, h, unidad := "1280", "720", "px"
	if t := e.m.Tamano; t != nil && t.Preset == "custom" && t.Ancho != nil && t.Alto != nil {
		w, h, unidad = strconv.FormatFloat(*t.Ancho, 'f', -1, 64), strconv.FormatFloat(*t.Alto, 'f', -1, 64), t.Unidad
	}
	ancho := ui.NuevaEntrada(e.t("slides.size.width")).Con(ui.CampoSM)
	alto := ui.NuevaEntrada(e.t("slides.size.height")).Con(ui.CampoSM)
	ancho.PonerTexto(w)
	alto.PonerTexto(h)
	ancho.Ancho, alto.Ancho = tema.Espacio(16), tema.Espacio(16)

	var px, cm *ui.Boton
	elegirUnidad := func(u string) {
		unidad = u
		px.Variante, cm.Variante = ui.Secundario, ui.Secundario
		if u == "cm" {
			cm.Variante = ui.Primario
		} else {
			px.Variante = ui.Primario
		}
		px.Refresh()
		cm.Refresh()
	}
	px = ui.NuevoBoton("px", func() { elegirUnidad("px") }).Con(ui.Secundario, ui.TallaXS)
	cm = ui.NuevoBoton("cm", func() { elegirUnidad("cm") }).Con(ui.Secundario, ui.TallaXS)
	elegirUnidad(unidad)

	aplicar := ui.NuevoBoton(e.t("action.apply"), func() {
		nw, errW := strconv.ParseFloat(strings.Replace(ancho.Texto(), ",", ".", 1), 64)
		nh, errH := strconv.ParseFloat(strings.Replace(alto.Texto(), ",", ".", 1), 64)
		if errW != nil || errH != nil || nw <= 0 || nh <= 0 {
			return
		}
		cerrar()
		e.PonerTamano(&Tamano{Preset: "custom", Ancho: &nw, Alto: &nh, Unidad: unidad})
	}).Con(ui.Secundario, ui.TallaXS)

	etiqueta := func(clave string) fyne.CanvasObject {
		return ui.NuevoRotulo(e.t(clave), tema.TextoXS, ui.Normal, "text-muted")
	}
	pista := ui.NuevoParrafo(e.t("slides.size.hint"), tema.Escalon{Tam: tema.Texto2XS.Tam, Linea: tema.Texto2XS.Tam * 1.25},
		ui.Normal, "text-subtle", e.variante()).A(AnchoMenuTamano - 2*(tema.Borde+rellenoMenu) - 2*tema.Espacio(2.5))
	return &seccionPersonalizada{filas: [][]fyne.CanvasObject{
		{etiqueta("slides.size.width"), ancho, etiqueta("slides.size.height"), alto},
		{px, cm, nil, aplicar},
		{pista},
	}}
}

// seccionPersonalizada es `flex flex-col gap-1.5 px-2.5 pt-1.5 pb-1`, con cada fila en
// `flex items-center gap-2`; un nil en una fila empuja lo que sigue a la derecha
// (`justify-between`).
type seccionPersonalizada struct {
	widget.BaseWidget
	filas [][]fyne.CanvasObject
}

func (s *seccionPersonalizada) altoFila(f []fyne.CanvasObject) float32 {
	var a float32
	for _, o := range f {
		if o != nil {
			a = max(a, o.MinSize().Height)
		}
	}
	return a
}

func (s *seccionPersonalizada) MinSize() fyne.Size {
	s.ExtendBaseWidget(s)
	alto := tema.Espacio(1.5) + tema.Espacio(1)
	for i, f := range s.filas {
		if i > 0 {
			alto += tema.Espacio(1.5)
		}
		alto += s.altoFila(f)
	}
	return fyne.NewSize(0, alto)
}

func (s *seccionPersonalizada) CreateRenderer() fyne.WidgetRenderer {
	var objs []fyne.CanvasObject
	for _, f := range s.filas {
		for _, o := range f {
			if o != nil {
				objs = append(objs, o)
			}
		}
	}
	return &seccionRenderer{s: s, objs: objs}
}

type seccionRenderer struct {
	s    *seccionPersonalizada
	objs []fyne.CanvasObject
}

func (r *seccionRenderer) MinSize() fyne.Size           { return r.s.MinSize() }
func (r *seccionRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *seccionRenderer) Destroy()                     {}
func (r *seccionRenderer) Refresh()                     {}

func (r *seccionRenderer) Layout(t fyne.Size) {
	x0, y := tema.Espacio(2.5), tema.Espacio(1.5)
	for _, f := range r.s.filas {
		alto := r.s.altoFila(f)
		x := x0
		for i, o := range f {
			if o == nil {
				// Lo que queda va pegado a la derecha.
				resto := float32(0)
				for _, q := range f[i+1:] {
					resto += q.MinSize().Width
				}
				x = t.Width - x0 - resto
				continue
			}
			m := o.MinSize()
			o.Move(fyne.NewPos(x, y+(alto-m.Height)/2))
			o.Resize(m)
			x += m.Width + tema.Espacio(2)
		}
		y += alto + tema.Espacio(1.5)
	}
}

// columnaMenu apila las filas de un menú del editor a un ancho fijo (`w-60`, `w-56`):
// las filas se estiran a lo ancho, como un `w-full`.
type columnaMenu struct {
	widget.BaseWidget
	filas []fyne.CanvasObject
	ancho float32
}

func (c *columnaMenu) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	var alto float32
	for _, f := range c.filas {
		alto += f.MinSize().Height
	}
	return fyne.NewSize(c.ancho-2*(tema.Borde+rellenoMenu), alto)
}

func (c *columnaMenu) CreateRenderer() fyne.WidgetRenderer { return &columnaRenderer{c: c} }

type columnaRenderer struct{ c *columnaMenu }

func (r *columnaRenderer) MinSize() fyne.Size           { return r.c.MinSize() }
func (r *columnaRenderer) Objects() []fyne.CanvasObject { return r.c.filas }
func (r *columnaRenderer) Destroy()                     {}
func (r *columnaRenderer) Refresh()                     {}
func (r *columnaRenderer) Layout(t fyne.Size) {
	var y float32
	for _, f := range r.c.filas {
		a := f.MinSize().Height
		f.Move(fyne.NewPos(0, y))
		f.Resize(fyne.NewSize(t.Width, a))
		y += a
	}
}

// cabeceraMenu es MENU_HEAD: `sytue-overline px-2.5 pb-0.5 pt-1.5 text-text-subtle` —
// `text-xs` semi-negrita en versalitas con su 0,06 em de aire entre letras.
type cabeceraMenu struct {
	widget.BaseWidget
	texto string
}

func nuevaCabeceraMenu(texto string) *cabeceraMenu {
	c := &cabeceraMenu{texto: texto}
	c.ExtendBaseWidget(c)
	return c
}

func (c *cabeceraMenu) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	return fyne.NewSize(ui.MedirVersal(c.texto, tema.TextoXS, ui.SemiNegrita)+2*tema.Espacio(2.5),
		tema.Espacio(1.5)+tema.TextoXS.Linea+tema.Espacio(0.5))
}

func (c *cabeceraMenu) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &cabeceraMenuRenderer{c: c, versal: ui.NuevoVersal(c.texto, tema.TextoXS, ui.SemiNegrita,
		tema.Color("text-subtle", v))}
	return r
}

type cabeceraMenuRenderer struct {
	c      *cabeceraMenu
	versal *ui.Versal
}

func (r *cabeceraMenuRenderer) MinSize() fyne.Size           { return r.c.MinSize() }
func (r *cabeceraMenuRenderer) Objects() []fyne.CanvasObject { return r.versal.Objetos() }
func (r *cabeceraMenuRenderer) Destroy()                     {}
func (r *cabeceraMenuRenderer) Refresh() {
	r.versal.Pintar(tema.Color("text-subtle", tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
}

func (r *cabeceraMenuRenderer) Layout(fyne.Size) {
	r.versal.Colocar(tema.Espacio(2.5), tema.Espacio(1.5))
}

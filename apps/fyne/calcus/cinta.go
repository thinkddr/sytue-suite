// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Registro es la cinta de Calcus: las pestañas genéricas del cromo con «Inicio»
// sustituida por la de la hoja (`InicioSheets.tsx`) y, de la taxonomía común con la web,
// «Datos» y «Fórmulas», que solo la hoja tiene.
func Registro(lz *Lienzo) *editor.Registro {
	r := editor.Genericas()
	r.Sustituir("inicio", lz.cuerpoInicio)
	r.Anadir("datos", lz.cuerpoDatos)
	r.Anadir("formulas", lz.cuerpoFormulas)
	return r
}

// Los anchos de los selectores de InicioSheets, en px como los escribe la web.
const (
	anchoSelectorFuente  float32 = 128
	anchoSelectorTamano  float32 = 56
	anchoSelectorFormato float32 = 104
)

// Las medidas de los paneles que cuelgan de la cinta (`POPOVER_SHELL` con su ancho).
var (
	anchoPanelCelda   = tema.Espacio(56) // w-56
	anchoPanelMascara = tema.Espacio(72) // w-72
	anchoPanelTexto   = tema.Espacio(48) // w-48
	rellenoPanel      = tema.Espacio(2)  // p-2
	huecoPanel        = tema.Espacio(1)  // gap-1
	margenBloque      = tema.Espacio(2)  // mb-2 entre bloques
)

// campoCinta es FIELD_CLASS: el campo de texto de la cinta, `h-7` en `text-xs`.
var campoCinta = ui.CampoPropio{
	MargenX: tema.Espacio(1.5), MargenY: (tema.Espacio(7) - tema.TextoXS.Linea - 2*tema.Borde) / 2,
	Texto: tema.TextoXS, Borde: "border", Radio: tema.RadioSM,
}

// campoPanel es el `inputCls` de los paneles de Datos: el mismo campo a `h-6`.
var campoPanel = ui.CampoPropio{
	MargenX: tema.Espacio(1.5), MargenY: (tema.Espacio(6) - tema.TextoXS.Linea - 2*tema.Borde) / 2,
	Texto: tema.TextoXS, Borde: "border", Radio: tema.RadioSM,
}

// cuerpoInicio es `InicioSheetsTab`, grupo a grupo y en su orden.
func (lz *Lienzo) cuerpoInicio(c *editor.Contexto) fyne.CanvasObject {
	st := lz.Estado()
	f := lz.Formato()
	e := st.Estilo
	ro := c.SoloLectura
	boton := func(i iconos.Ed, clave string, activo bool, cmd editor.Comando, valor string) *editor.BotonCinta {
		b := editor.NuevoBotonCinta(i, c.Traducir(clave), func() { c.Mandar(cmd, valor) })
		b.Activo, b.Inerte = activo, ro
		return b
	}
	estilo := func(parche string) { c.Mandar(CmdEstilo, parche) }

	fuente := e.Fuente
	if fuente == "" {
		fuente = Fuentes[0]
	}
	fuentes := make([]editor.Opcion, len(Fuentes))
	for i, x := range Fuentes {
		fuentes[i] = editor.Opcion{Valor: x, Rotulo: x}
	}
	tamano := e.Tam
	if tamano == 0 {
		tamano = 11
	}
	tamanos := make([]editor.Opcion, len(TamanosFuente))
	for i, n := range TamanosFuente {
		tamanos[i] = editor.Opcion{Valor: strconv.Itoa(n), Rotulo: strconv.Itoa(n)}
	}
	selFuente := editor.NuevoSelectorCinta(c.Traducir("group.font"), anchoSelectorFuente, fuentes, fuente,
		func(v string) { estilo(fmt.Sprintf(`{"font":%q}`, v)) })
	selTamano := editor.NuevoSelectorCinta(c.Traducir("font.size"), anchoSelectorTamano, tamanos, strconv.Itoa(tamano),
		func(v string) { estilo(`{"size":` + v + `}`) })
	selFuente.Inerte, selTamano.Inerte = ro, ro

	tinta := e.Tinta
	if tinta == "" {
		tinta = editor.PaletaContenido[0]
	}
	relleno := e.Relleno
	if relleno == "" {
		relleno = "transparent"
	}
	muestraTinta := editor.NuevaMuestraCinta(c, iconos.EdColorTexto, c.Traducir("format.textColor"), tinta, nil,
		func(col string) { estilo(fmt.Sprintf(`{"color":%q}`, col)) })
	muestraRelleno := editor.NuevaMuestraCinta(c, iconos.EdResaltado, c.Traducir("format.fillColor"), relleno,
		editor.PaletaResaltado, func(col string) {
			if col == "transparent" {
				estilo(`{"fill":null}`)
				return
			}
			estilo(fmt.Sprintf(`{"fill":%q}`, col))
		})
	muestraTinta.Inerte, muestraRelleno.Inerte = ro, ro

	formatos := make([]editor.Opcion, 0, len(PresetsFormato)+1)
	for _, p := range PresetsFormato {
		formatos = append(formatos, editor.Opcion{Valor: p.ID, Rotulo: c.Traducir(p.Clave)})
	}
	if st.Numero.ID == "custom" {
		// El selector enseña la opción que coincide; con una máscara no hay ninguna y el
		// `<select>` de la web se queda en blanco. Aquí se dice qué es.
		formatos = append(formatos, editor.Opcion{Valor: "custom", Rotulo: c.Traducir("sheet.numberFormat.custom")})
	}
	selFormato := editor.NuevoSelectorCinta(c.Traducir("sheet.numberFormat"), anchoSelectorFormato, formatos, st.Numero.ID,
		func(v string) { c.Mandar(CmdFormatoNum, v) })
	selFormato.Inerte = ro

	var celda, mascara *editor.BotonCinta
	celda = editor.NuevoBotonCinta(iconos.EdTabla, c.Traducir("sheet.bordersAndFit"), func() { lz.abrirCelda(c, celda) })
	mascara = editor.NuevoBotonCinta(iconos.EdFormatoNumero, c.Traducir("sheet.numberFormat.custom"), func() { lz.abrirMascara(c, mascara) })
	mascara.Activo = st.Numero.ID == "custom"
	celda.Inerte, mascara.Inerte = ro, ro

	grupos := []fyne.CanvasObject{
		editor.NuevoGrupo(c.Traducir("group.font"),
			selFuente, selTamano,
			boton(iconos.EdNegrita, "format.bold", f.Negrita, editor.Negrita, ""),
			boton(iconos.EdCursiva, "format.italic", f.Cursiva, editor.Cursiva, ""),
			boton(iconos.EdSubrayado, "format.underline", e.Subraya, CmdEstilo, fmt.Sprintf(`{"u":%t}`, !e.Subraya)),
			boton(iconos.EdTachado, "format.strike", e.Tacha, CmdEstilo, fmt.Sprintf(`{"s":%t}`, !e.Tacha)),
			muestraTinta, muestraRelleno,
			boton(iconos.EdSinFormato, "format.clear", false, CmdQuitarEstil, ""),
		),
		editor.SeparadorGrupos(),
		// En la web los tres botones de alinear no se encienden con la alineación de la
		// celda (no llevan `active`), a diferencia de los de Docus.
		editor.NuevoGrupo(c.Traducir("group.alignment"),
			boton(iconos.EdAlinearIzq, "align.left", false, CmdAlinear, "left"),
			boton(iconos.EdAlinearCentro, "align.center", false, CmdAlinear, "center"),
			boton(iconos.EdAlinearDer, "align.right", false, CmdAlinear, "right"),
			celda,
		),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.number"),
			selFormato,
			boton(iconos.EdMasDecimales, "sheet.decimals.increase", false, CmdDecimales, "+1"),
			boton(iconos.EdMenosDecimales, "sheet.decimals.decrease", false, CmdDecimales, "-1"),
			mascara,
		),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.styles"), lz.botonCondicional(c)),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.merge"), boton(iconos.EdCombinar, "sheet.merge", false, CmdCombinar, "")),
	}
	// El tema común y la marca de la organización: la cabecera de la hoja.
	if c.Temas != nil {
		grupos = append(grupos, c.Temas.Grupo(c)...)
	}
	return editor.Grupos(grupos...)
}

// grupoBloques es el grupo «Bloques» con su separador detrás, o un hueco sin medida si el
// producto no los ofrece.
func (lz *Lienzo) grupoBloques(c *editor.Contexto) fyne.CanvasObject {
	if c.Bloques == nil {
		return container.NewWithoutLayout()
	}
	return editor.Grupos(c.Bloques.Grupo(c), editor.SeparadorGrupos())
}

// ---- los paneles de la cinta ----

// panel es un contenido de ancho fijo para un emergente: una columna de filas con el
// `mb-*` que las separa. El ancho es el de la clase (`w-56`, `w-72`…) menos su `p-2` y
// su borde, que los pone el emergente.
type panel struct {
	ancho float32
	filas []fyne.CanvasObject
	hueco float32
}

func (p panel) objeto() fyne.CanvasObject {
	return container.New(anchoFijo{ancho: p.ancho - 2*(rellenoPanel+tema.Borde), col: ui.Columna{Hueco: p.hueco}}, p.filas...)
}

// anchoFijo es una columna que declara un ancho y no el de su hijo más ancho.
type anchoFijo struct {
	ancho float32
	col   ui.Columna
}

func (a anchoFijo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(a.ancho, a.col.MinSize(objs).Height)
}

func (a anchoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) { a.col.Layout(objs, t) }

// overline es `sytue-overline text-text-subtle`: el rótulo en versalitas de un bloque.
func overline(texto string) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return ui.NuevoParrafo(ui.Versalitas(texto), tema.Texto2XS, ui.SemiNegrita, "text-subtle", v)
}

// botonPop es POP_BTN (y los botones de los paneles de la hoja que llevan sus mismas
// clases): `h-6 rounded-sm border border-border bg-surface px-2 text-xs text-text
// hover:bg-bg-subtle aria-pressed:bg-selected aria-pressed:border-transparent`, que es el
// botón pequeño del panel «Buscar» con otro alto.
func botonPop(texto string, pulsado bool, f func()) *editor.BotonPequeno {
	b := editor.NuevoBotonPequeno(texto, f)
	b.Alto, b.Pulsado = tema.Espacio(6), pulsado
	return b
}

// rejillaBotones reparte botones en `grid-cols-N gap-1`.
func rejillaBotones(columnas int, botones ...fyne.CanvasObject) fyne.CanvasObject {
	return container.New(&ui.Rejilla{Hueco: huecoPanel, Fijas: columnas}, botones...)
}

// abrirEmergente despliega un panel bajo el botón de la cinta, con el botón encendido
// mientras está abierto (`aria-expanded:bg-selected`).
func abrirEmergente(disparador *editor.BotonCinta, contenido fyne.CanvasObject) *ui.Emergente {
	disparador.Activo = true
	disparador.Refresh()
	em := ui.MostrarEmergente(disparador, contenido, rellenoPanel)
	em.AlRecogerse = func() { disparador.Activo = false; disparador.Refresh() }
	return em
}

// abrirCelda es `CellFormatMenu`: bordes, alineación vertical y ajuste de texto.
func (lz *Lienzo) abrirCelda(c *editor.Contexto, disparador *editor.BotonCinta) {
	e := lz.Estado().Estilo
	var em *ui.Emergente
	mandar := func(cmd editor.Comando, valor string) func() {
		return func() { c.Mandar(cmd, valor); em.Cerrar() }
	}
	bordes := make([]fyne.CanvasObject, 0, 4)
	for _, b := range []struct{ valor, clave string }{
		{"all", "sheet.border.all"},
		{"outer", "sheet.border.outer"},
		{"bottom", "sheet.border.bottom"},
		{"none", "sheet.border.none"},
	} {
		bordes = append(bordes, botonPop(c.Traducir(b.clave), false, mandar(CmdBordes, b.valor)))
	}
	verticales := make([]fyne.CanvasObject, 0, 3)
	for _, v := range []struct{ valor, clave string }{
		{"top", "sheet.valign.top"}, {"middle", "sheet.valign.middle"}, {"bottom", "sheet.valign.bottom"},
	} {
		parche := fmt.Sprintf(`{"valign":%q}`, v.valor)
		if e.Vertical == v.valor {
			parche = `{"valign":null}` // pulsar el que está puesto lo quita
		}
		verticales = append(verticales, botonPop(c.Traducir(v.clave), e.Vertical == v.valor, mandar(CmdEstilo, parche)))
	}
	ajustar := nuevaCasillaXS(c.Traducir("sheet.wrapText"), e.Ajustar, tema.Espacio(1.5), func(si bool) {
		if si {
			c.Mandar(CmdEstilo, `{"wrap":true}`)
		} else {
			c.Mandar(CmdEstilo, `{"wrap":null}`)
		}
	})
	em = abrirEmergente(disparador, panel{ancho: anchoPanelCelda, hueco: huecoPanel, filas: []fyne.CanvasObject{
		overline(c.Traducir("sheet.borders.heading")),
		conMargen(rejillaBotones(2, bordes...)),
		overline(c.Traducir("sheet.valignLabel")),
		conMargen(rejillaBotones(3, verticales...)),
		ajustar,
	}}.objeto())
}

// conMargen añade el `mb-2` de un bloque.
func conMargen(o fyne.CanvasObject) fyne.CanvasObject {
	return container.New(margenAbajo{margenBloque - huecoPanel}, o)
}

type margenAbajo struct{ m float32 }

func (m margenAbajo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	s := objs[0].MinSize()
	return fyne.NewSize(s.Width, s.Height+m.m)
}

func (m margenAbajo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, t.Height-m.m))
}

// mascarasEjemplo son CUSTOM_MASK_PRESETS: los patrones van sin traducir, como en la
// web —`[Rojo]` y el `"de"` de la fecha son sintaxis del motor—; solo sus rótulos.
var mascarasEjemplo = []struct{ clave, mascara string }{
	{"sheet.mask.thousands", "#,##0.00"},
	{"sheet.mask.integer", "#,##0"},
	{"sheet.mask.currency", "#,##0.00 €"},
	{"sheet.mask.percent", "0.0%"},
	{"sheet.mask.redNeg", "#,##0.00;[Rojo]-#,##0.00"},
	{"sheet.mask.parens", "#,##0.00;(#,##0.00)"},
	{"sheet.mask.kilo", `#,##0,"k"`},
	{"sheet.mask.date", "dd/mm/yyyy"},
	{"sheet.mask.longDate", `d "de" mmmm "de" yyyy`},
	{"sheet.mask.time", "hh:mm"},
	{"sheet.mask.duration", "[h]:mm:ss"},
	{"sheet.mask.threshold", "[>=100][Rojo]#,##0;#,##0"},
}

// muestrasMascara son CUSTOM_PREVIEW: positivo, negativo, cero y una fecha.
var muestrasMascara = []struct {
	clave string
	v     float64
}{{"sheet.preview.positive", 1234.567}, {"sheet.preview.negative", -1234.567}, {"sheet.preview.zero", 0}, {"sheet.mask.date", 45658}}

// abrirMascara es `CustomFmtMenu`: el campo de la máscara, los ejemplos, la vista previa
// en vivo y «Quitar»/«Aplicar».
func (lz *Lienzo) abrirMascara(c *editor.Contexto, disparador *editor.BotonCinta) {
	actual := lz.Estado().Numero
	campo := ui.NuevaEntrada(c.Traducir("sheet.mask.example")).ConPropia(campoCinta)
	if actual.ID == "custom" {
		campo.PonerTexto(actual.Mascara) // prellena con la máscara que tiene
	}
	var em *ui.Emergente
	aplicar := botonPop(c.Traducir("action.apply"), false, func() {
		if m := strings.TrimSpace(campo.Texto()); m != "" {
			c.Mandar(CmdFormatoNum, "custom:"+m)
			em.Cerrar()
		}
	})
	previa := make([]*vistaPrevia, len(muestrasMascara))
	filasPrevia := make([]fyne.CanvasObject, len(muestrasMascara))
	for i, m := range muestrasMascara {
		previa[i] = nuevaVistaPrevia(c.Traducir(m.clave), m.v)
		filasPrevia[i] = previa[i]
	}
	repintar := func(texto string) {
		t := strings.TrimSpace(texto)
		for _, p := range previa {
			p.poner(t)
		}
		aplicar.Inerte = t == ""
		aplicar.Refresh()
	}
	campo.AlCambiar(repintar)
	repintar(campo.Texto())
	ejemplos := make([]fyne.CanvasObject, len(mascarasEjemplo))
	for i, m := range mascarasEjemplo {
		ejemplos[i] = botonPop(c.Traducir(m.clave), false, func() { campo.PonerTexto(m.mascara) })
	}
	quitar := botonPop(c.Traducir("action.removeItem"), false, func() { c.Mandar(CmdFormatoNum, "general"); em.Cerrar() })
	pie := entre(quitar, aplicar)
	em = abrirEmergente(disparador, panel{ancho: anchoPanelMascara, hueco: huecoPanel, filas: []fyne.CanvasObject{
		overline(c.Traducir("sheet.formatMask")),
		conMargen(campo),
		conMargen(container.New(&ui.Rejilla{Hueco: huecoPanel, Fijas: 3}, ejemplos...)),
		overline(c.Traducir("sheet.preview")),
		conMargen(recuadro(container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, filasPrevia...))),
		pie,
	}}.objeto())
}

// vistaPrevia es una fila de la vista previa: el rótulo (`sytue-meta-dense`) y la
// muestra con la máscara, en su color si la máscara lo pone.
type vistaPrevia struct {
	*fyne.Container
	v       float64
	muestra *ui.Parrafo
}

func nuevaVistaPrevia(rotulo string, v float64) *vistaPrevia {
	tema0 := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p := &vistaPrevia{v: v, muestra: ui.NuevoParrafo("—", tema.TextoXS, ui.Normal, "text", tema0)}
	p.Container = container.New(fila{hueco: tema.Espacio(2), elastico: 0}, ui.NuevoParrafo(rotulo, tema.Texto2XS, ui.Normal, "text-subtle", tema0), p.muestra)
	return p
}

func (p *vistaPrevia) poner(mascara string) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p.muestra.Texto, p.muestra.Tinta = "—", tema.Color("text", v)
	if mascara != "" {
		texto, col := conMascara(mascara, p.v)
		if texto != "" {
			p.muestra.Texto = texto
		}
		if c, ok := editor.Hex(col); ok {
			p.muestra.Tinta = c
		}
	}
	p.muestra.Refresh()
}

// ---- Datos ----

// cuerpoDatos es `DatosTab`, grupo a grupo y en su orden.
//
// Lo que este lienzo todavía no sabe hacer va APAGADO y no escondido: la cinta mide y se
// ve como la de la web, y un botón que se pulsa y no hace nada es como se concluye que
// la aplicación está rota.
//
// ponytail: los datos externos llegan con su parte del modelo (hoy, un libro que los
// lleva se abre en solo lectura).
func (lz *Lienzo) cuerpoDatos(c *editor.Contexto) fyne.CanvasObject {
	ro := c.SoloLectura
	boton := func(i iconos.Ed, clave string, cmd editor.Comando, valor string) *editor.BotonCinta {
		b := editor.NuevoBotonCinta(i, c.Traducir(clave), func() { c.Mandar(cmd, valor) })
		b.Inerte = ro
		return b
	}
	apagado := func(i iconos.Ed, clave string) *editor.BotonCinta {
		b := editor.NuevoBotonCinta(i, c.Traducir(clave), nil)
		b.Inerte = true
		return b
	}
	var textoCols *editor.BotonCinta
	textoCols = editor.NuevoBotonCinta(iconos.EdColDer, c.Traducir("data.textToColumns"), func() { lz.abrirTextoCols(c, textoCols) })
	textoCols.Inerte = ro
	fijar := editor.NuevoBotonCinta(iconos.EdInmovilizar, c.Traducir("data.freezePanes"), lz.alternarFijar(c))
	fijar.Activo, fijar.Inerte = lz.Libro.Fijas() != (Fijas{}), ro
	// LockCellButton: bloqueada salvo `locked: false`, y el rótulo dice lo que hará.
	bloqueada := lz.Estado().Estilo.Bloqueada == nil || *lz.Estado().Estilo.Bloqueada
	claveCandado := "data.cell.lock"
	if bloqueada {
		claveCandado = "data.cell.unlock"
	}
	candado := boton(iconos.EdCandado, claveCandado, CmdEstilo, fmt.Sprintf(`{"locked":%t}`, !bloqueada))
	candado.Activo = bloqueada
	candado.Inerte = ro || lz.Estado().Protegida // con la hoja protegida no se cambia el bloqueo
	autofiltro := boton(iconos.EdFiltro, "data.autofilter", CmdAutofiltro, "")
	act := lz.botonesActualizar(c)
	autofiltro.Activo = lz.Estado().Autofiltro

	herramientas := []fyne.CanvasObject{boton(iconos.EdPapelera, "data.removeDuplicates", CmdDuplicados, ""), textoCols, lz.botonValidacion(c)}

	return editor.Grupos(
		editor.NuevoGrupo(c.Traducir("group.externalData"),
			apagado(iconos.EdBaseDatos, "data.getExternal"), apagado(iconos.EdFiltro, "data.transform"),
			act[0], act[1],
		),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.sortFilter"),
			boton(iconos.EdOrdenAsc, "data.sortAsc", CmdOrdenar, "asc"),
			boton(iconos.EdOrdenDesc, "data.sortDesc", CmdOrdenar, "desc"),
			autofiltro, lz.botonSegmentacion(c),
		),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("settings.table"), lz.botonTabla(c)),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("names.group"), lz.botonNombres(c)),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.tools"), herramientas...),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.cells"),
			boton(iconos.EdFilaArriba, "table.rowAbove", CmdInsFila, "above"),
			boton(iconos.EdFilaAbajo, "table.rowBelow", CmdInsFila, "below"),
			boton(iconos.EdColIzq, "table.colLeft", CmdInsCol, "left"),
			boton(iconos.EdColDer, "table.colRight", CmdInsCol, "right"),
			boton(iconos.EdMenos, "table.deleteRows", CmdBorrarFilas, ""),
			boton(iconos.EdCerrar, "table.deleteCols", CmdBorrarCols, ""),
		),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("slides.box.chart"), lz.botonGrafico(c)),
		editor.SeparadorGrupos(),
		lz.grupoBloques(c),
		editor.NuevoGrupo(c.Traducir("extdata.step.review"),
			lz.botonMinigrafico(c), lz.botonDinamica(c),
		),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("rail.outline.title"), lz.grupoEsquema(c)...),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.window"), fijar),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.protect"),
			candado, lz.botonProtegerHoja(c), lz.botonProtegerLibro(c),
		),
	)
}

// cuerpoFormulas es `FormulasTab`: lo que escribe o calcula con fórmulas. El asistente que
// propone la de una celda (solo con servicio detrás, como en la web: sin él sería un botón
// que siempre falla) y el análisis de hipótesis. Salieron de «Datos» al fijarse la
// taxonomía de pestañas.
func (lz *Lienzo) cuerpoFormulas(c *editor.Contexto) fyne.CanvasObject {
	hipotesis := editor.NuevoGrupo(c.Traducir("group.whatIf"), lz.grupoHipotesis(c)...)
	if b := lz.botonRelleno(c); b != nil {
		return editor.Grupos(
			editor.NuevoGrupo(c.Traducir("group.assistant"), b),
			editor.SeparadorGrupos(),
			hipotesis,
		)
	}
	return editor.Grupos(hipotesis)
}

// alternarFijar es `toggleFreeze`: si hay algo inmovilizado lo suelta; si no, inmoviliza
// por encima y a la izquierda de la esquina del rango —o la primera fila, en A1—.
func (lz *Lienzo) alternarFijar(c *editor.Contexto) func() {
	return func() {
		if lz.Libro.Fijas() != (Fijas{}) {
			c.Mandar(CmdFijar, "0,0")
			return
		}
		c1, f1, _, _ := lz.Rejilla.Rango()
		if c1 == 0 && f1 == 0 {
			c.Mandar(CmdFijar, "0,1")
			return
		}
		c.Mandar(CmdFijar, strconv.Itoa(c1)+","+strconv.Itoa(f1))
	}
}

// abrirTextoCols es `TextToColumnsMenu`: los cuatro separadores.
func (lz *Lienzo) abrirTextoCols(c *editor.Contexto, disparador *editor.BotonCinta) {
	var em *ui.Emergente
	botones := make([]fyne.CanvasObject, 0, 4)
	for _, d := range []struct{ valor, clave string }{
		{",", "data.delim.comma"}, {";", "data.delim.semicolon"}, {"\t", "data.delim.tab"}, {" ", "data.delim.space"},
	} {
		botones = append(botones, botonPop(c.Traducir(d.clave), false, func() { c.Mandar(CmdTextoCols, d.valor); em.Cerrar() }))
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	em = abrirEmergente(disparador, panel{ancho: anchoPanelTexto, hueco: tema.Espacio(1.5), filas: []fyne.CanvasObject{
		ui.NuevoParrafo(c.Traducir("data.separateBy"), tema.Texto2XS, ui.Normal, "text-subtle", v),
		rejillaBotones(2, botones...),
	}}.objeto())
}

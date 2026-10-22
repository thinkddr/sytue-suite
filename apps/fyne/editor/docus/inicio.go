// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// La pestaña «Inicio» de Docus (`ribbon/tabs/Inicio.tsx`): Historial · Fuente · Párrafo ·
// Estilos. Vive aquí y no en el cromo porque Calcus y Diapos tienen la suya
// (`InicioSheets.tsx`, `InicioSlides.tsx`), como en la web.
//
// ponytail: falta el grupo «Asistente» (IA). En la web solo sale si el servidor tiene
// servicio de IA detrás; en nativo no hay todavía cliente del asistente, y un botón que
// siempre falla es peor que no tenerlo —lo dice la propia web—.

// Fuentes son las seis de la web, en su orden; «Times» se declara «Times New Roman».
var Fuentes = []string{"Inter", "Arial", "Georgia", "Times New Roman", "Courier New", "JetBrains Mono"}

// Tallas son los siete peldaños de `fontSize` con su rótulo en pt.
var Tallas = []editor.Opcion{
	{Valor: "1", Rotulo: "8"},
	{Valor: "2", Rotulo: "10"},
	{Valor: "3", Rotulo: "12"},
	{Valor: "4", Rotulo: "14"},
	{Valor: "5", Rotulo: "18"},
	{Valor: "6", Rotulo: "24"},
	{Valor: "7", Rotulo: "36"},
}

// Interlineados son los que admite `exec('lineHeight')`.
var Interlineados = []float64{1, 1.15, 1.5, 2}

// Anchos de los selectores de la cinta (`width={128}`, `width={64}`).
const (
	AnchoSelectorFuente float32 = 128
	AnchoSelectorCorto  float32 = 64
)

// inicio es lo que la pestaña recuerda entre recomposiciones: el último color, resalte e
// interlineado aplicados, que es lo que la barrita y el selector reflejan (la web no los
// saca del formato bajo el cursor: «comportamiento clásico de suite ofimática»).
type inicio struct {
	loc                          i18n.Locale
	color, resalte, interlineado string
	interlDisposicion            string // el de «Disposición» lleva su propia memoria, como en la web
	columnas                     string // lo último aplicado en «Columnas», como el `useState` de la web
}

func nuevoInicio(loc i18n.Locale) *inicio {
	return &inicio{
		loc: loc, color: editor.PaletaContenido[0], resalte: editor.PaletaResaltado[0],
		interlineado: "1.5", interlDisposicion: "1.5",
	}
}

// registroDocus son las pestañas de la cinta de Docus: las genéricas, su «Inicio» y las
// del documento —«Insertar», «Diseño» (la disposición de página) y «Referencias»—, cada
// una en el sitio, con el rótulo y los tipos que fija la taxonomía común con la web.
func registroDocus(loc i18n.Locale) *editor.Registro {
	reg := editor.Genericas()
	in := nuevoInicio(loc)
	reg.Sustituir("inicio", in.cuerpo)
	reg.Anadir("insertar", in.cuerpoInsertar)
	reg.Anadir("diseno", in.cuerpoDisposicion)
	reg.Anadir("referencias", in.cuerpoReferencias)
	return reg
}

func (in *inicio) cuerpo(c *editor.Contexto) fyne.CanvasObject {
	f := editor.Formato{}
	if c.Lienzo != nil {
		f = c.Lienzo.Formato()
	}
	boton := c.Boton
	deshacer := boton(iconos.EdDeshacer, "action.undo", false, editor.Deshacer)
	deshacer.Inerte = deshacer.Inerte || !f.PuedeDeshacer
	rehacer := boton(iconos.EdRehacer, "action.redo", false, editor.Rehacer)
	rehacer.Inerte = rehacer.Inerte || !f.PuedeRehacer

	fuente := f.Fuente
	if !contiene(Fuentes, fuente) {
		fuente = "Inter" // indeterminado: el de por defecto, como la web
	}
	opcionesFuente := make([]editor.Opcion, len(Fuentes))
	for i, n := range Fuentes {
		opcionesFuente[i] = editor.Opcion{Valor: n, Rotulo: n}
	}
	talla := f.Tamano
	if talla == "" {
		talla = "3"
	}
	selFuente := editor.NuevoSelectorCinta(c.Traducir("font.family"), AnchoSelectorFuente, opcionesFuente, fuente,
		func(v string) { c.Mandar(editor.Fuente, v) })
	selTalla := editor.NuevoSelectorCinta(c.Traducir("font.size"), AnchoSelectorCorto, Tallas, talla,
		func(v string) { c.Mandar(editor.Tamano, v) })
	colorTexto := editor.NuevaMuestraCinta(c, iconos.EdColorTexto, c.Traducir("format.textColor"), in.color, nil,
		func(v string) { in.color = v; c.Mandar(editor.ColorTexto, v) })
	resalte := editor.NuevaMuestraCinta(c, iconos.EdResaltado, c.Traducir("format.highlight"), in.resalte,
		editor.PaletaResaltado, func(v string) { in.resalte = v; c.Mandar(editor.Resaltado, v) })

	interl := make([]editor.Opcion, len(Interlineados))
	for i, v := range Interlineados {
		interl[i] = editor.Opcion{Valor: strconv.FormatFloat(v, 'f', -1, 64), Rotulo: i18n.FormatNumber(in.loc, v)}
	}
	selInterl := editor.NuevoSelectorCinta(c.Traducir("paragraph.lineHeight"), AnchoSelectorCorto, interl, in.interlineado,
		func(v string) { in.interlineado = v; c.Mandar(editor.Interlineado, v) })
	if c.SoloLectura {
		for _, s := range []*editor.SelectorCinta{selFuente, selTalla, selInterl} {
			s.Inerte = true
		}
		colorTexto.Inerte, resalte.Inerte = true, true
	}

	titulo := f.Titulo
	if titulo == "" {
		titulo = "p" // EMPTY_FORMAT: sin cursor, «Normal» sale pulsado
	}
	estilos := make([]fyne.CanvasObject, 0, len(estilosGaleria))
	for _, e := range estilosGaleria {
		chip := nuevaFichaEstilo(e, c.Traducir(e.clave), c.Traducir("style.apply", i18n.Vars{"name": c.Traducir(e.clave)}),
			titulo == e.valor, func() { c.Mandar(editor.Titulo, e.valor) })
		chip.Inerte = c.SoloLectura
		estilos = append(estilos, chip)
	}
	grupos := []fyne.CanvasObject{
		editor.NuevoGrupo(c.Traducir("group.clipboard"), deshacer, rehacer),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.font"),
			selFuente, selTalla,
			boton(iconos.EdNegrita, "format.bold", f.Negrita, editor.Negrita),
			boton(iconos.EdCursiva, "format.italic", f.Cursiva, editor.Cursiva),
			boton(iconos.EdSubrayado, "format.underline", f.Subrayado, editor.Subrayado),
			boton(iconos.EdTachado, "format.strike", f.Tachado, editor.Tachado),
			// Sin reflejo de estado activo, como en la web: FormatState no los sigue.
			boton(iconos.EdSuperindice, "format.superscript", false, editor.Superindice),
			boton(iconos.EdSubindice, "format.subscript", false, editor.Subindice),
			colorTexto, resalte,
			boton(iconos.EdSinFormato, "format.clear", false, editor.SinFormato),
		),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.paragraph"),
			boton(iconos.EdAlinearIzq, "align.left", f.Alineacion == editor.Izquierda, editor.AlinearIzq),
			boton(iconos.EdAlinearCentro, "align.center", f.Alineacion == editor.Centro, editor.AlinearCen),
			boton(iconos.EdAlinearDer, "align.right", f.Alineacion == editor.Derecha, editor.AlinearDer),
			boton(iconos.EdJustificar, "align.justify", f.Alineacion == editor.Justificado, editor.Justificar),
			boton(iconos.EdListaPuntos, "list.bullet", f.Puntos, editor.ListaPuntos),
			boton(iconos.EdListaNumeros, "list.number", f.Numeros, editor.ListaNumero),
			boton(iconos.EdMenosSangria, "indent.decrease", false, editor.MenosSangria),
			boton(iconos.EdMasSangria, "indent.increase", false, editor.MasSangria),
			selInterl,
		),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.styles"), container.New(ui.Fila{Hueco: tema.Espacio(0.5)}, estilos...)),
	}
	// «Asistente» abre el panel del carril, y solo si hay Sytue AI detrás
	// (`ctx.aiEnabled`): sin ella, el botón sería un simulacro.
	if c.IA {
		rotulo := "ai.open"
		if c.Panel == editor.PanelIA {
			rotulo = "ai.hide"
		}
		ia := editor.NuevoBotonCinta(iconos.EdIA, c.Traducir(rotulo), func() { c.AbrirPanel(editor.PanelIA) })
		ia.Activo = c.Panel == editor.PanelIA
		grupos = append(grupos, editor.SeparadorGrupos(), editor.NuevoGrupo(c.Traducir("group.assistant"), ia))
	}
	return editor.Grupos(grupos...)
}

func contiene(s []string, x string) bool {
	for _, y := range s {
		if y == x {
			return true
		}
	}
	return false
}

// ---- la galería de estilos ----

// estiloGaleria es una ficha: el valor de `heading`, la clave del nombre y la letra de la
// previsualización «AaBb».
type estiloGaleria struct {
	valor, clave string
	esc          tema.Escalon
	peso         ui.Peso
	cursiva      bool
	token        string
}

// estilosGaleria son los cinco de `STYLE_ITEMS`, con `leading-none` (la caja de línea es
// la letra).
var estilosGaleria = []estiloGaleria{
	{"p", "style.normal", sinInterlineado(tema.TextoXS), ui.Normal, false, "text"},
	{"h1", "style.h1", sinInterlineado(tema.TextoBase), ui.Negrita, false, "text"},
	{"h2", "style.h2", sinInterlineado(tema.TextoSM), ui.SemiNegrita, false, "text"},
	{"h3", "style.h3", sinInterlineado(tema.TextoXS), ui.SemiNegrita, false, "text"},
	{"blockquote", "style.quote", sinInterlineado(tema.TextoXS), ui.Normal, true, "text-muted"},
}

func sinInterlineado(e tema.Escalon) tema.Escalon { return tema.Escalon{Tam: e.Tam, Linea: e.Tam} }

// Medidas de StyleChip: `h-11 w-[72px]`, `gap-0.5` entre la muestra y el nombre.
var (
	AnchoFichaEstilo float32 = 72
	AltoFichaEstilo          = tema.Espacio(11)
)

// fichaEstilo es StyleChip: `border-border h-11 w-[72px] rounded-sm border flex-col
// items-center justify-center gap-0.5 hover:bg-bg-subtle aria-pressed:bg-selected
// aria-pressed:border-transparent`, con «AaBb» en la letra del estilo y el nombre en
// `sytue-meta` truncado.
type fichaEstilo struct {
	botonPequeno
	e      estiloGaleria
	nombre string
	activa bool
}

func nuevaFichaEstilo(e estiloGaleria, nombre, rotulo string, activa bool, alPulsar func()) *fichaEstilo {
	f := &fichaEstilo{e: e, nombre: nombre, activa: activa}
	f.Texto, f.AlPulsar = rotulo, alPulsar
	f.ExtendBaseWidget(f)
	return f
}

func (f *fichaEstilo) MinSize() fyne.Size {
	f.ExtendBaseWidget(f)
	return fyne.NewSize(AnchoFichaEstilo, AltoFichaEstilo)
}

func (f *fichaEstilo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &fichaRenderer{
		f:    f,
		caja: ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioSM),
		aro:  ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
	}
	r.muestra = ui.Texto("AaBb", f.e.esc, f.e.peso, tema.Color(f.e.token, v))
	if f.e.cursiva {
		r.muestra.FontSource = tema.Cursiva
	}
	r.nombre = ui.Texto(f.nombre, sinInterlineado(tema.TextoXS), ui.Normal, tema.Color("text-subtle", v))
	r.objs = r.aro.Antes(append(r.caja.Objetos(), r.muestra, r.nombre)...)
	return r
}

type fichaRenderer struct {
	f               *fichaEstilo
	caja            *ui.Caja
	aro             *ui.Aro
	muestra, nombre *canvas.Text
	objs            []fyne.CanvasObject
}

func (r *fichaRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *fichaRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *fichaRenderer) Destroy()                     {}

func (r *fichaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)
	meta := sinInterlineado(tema.TextoXS)
	hueco := tema.Espacio(0.5)
	alto := r.f.e.esc.Linea + hueco + meta.Linea
	y := (t.Height - alto) / 2
	wm := ui.Medir("AaBb", r.f.e.esc, r.f.e.peso).Width
	ui.ColocarTexto(r.muestra, r.f.e.esc, (t.Width-wm)/2, y)
	// `w-full truncate px-1 text-center`.
	cabe := t.Width - 2*tema.Borde - 2*tema.Espacio(1)
	wn := ui.Truncar(r.nombre, r.f.nombre, meta, ui.Normal, cabe)
	ui.ColocarTexto(r.nombre, meta, (t.Width-wn)/2, y+r.f.e.esc.Linea+hueco)
}

func (r *fichaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde := tema.Color("surface", v), tema.Color("border", v)
	switch {
	case r.f.activa:
		// `bg-selected` es translúcido: se aplana sobre la cinta, y el borde toma el
		// mismo color (`aria-pressed:border-transparent`: no se ve).
		fondo = tokens.SobreSRGB(tema.Color("selected", v), tema.Color("surface", v))
		borde = fondo
	case r.f.Encima && !r.f.Inerte:
		fondo = tema.Color("bg-subtle", v)
	}
	r.caja.Pintar(fondo, borde)
	r.aro.Pintar(v)
	r.aro.Ver(r.f.Enfocado)
	if s := r.f.Size(); s.Width > 0 { // sin tamaño todavía: la caja con borde saldría negativa
		r.Layout(s)
	}
}

// ---- Disposición ----

// Anchos de los selectores de «Disposición» (`width={100}`, `width={88}`).
const (
	AnchoSelectorMargenes float32 = 100
	AnchoSelectorTamano   float32 = 88
)

// cuerpoDisposicion es la pestaña «Disposición» (`Disposicion.tsx`): márgenes,
// orientación, tamaño, color del papel, numeración, sangría e interlineado. Los cinco
// primeros se guardan con el documento, en la capa de la página; la numeración es de la VISTA.
func (in *inicio) cuerpoDisposicion(c *editor.Contexto) fyne.CanvasObject {
	f, _ := c.Lienzo.(*Folio)
	pag := PaginaPorDefecto()
	if f != nil {
		pag = f.Pagina()
	}
	poner := func(cambio func(*Pagina)) {
		if f == nil {
			return
		}
		p := f.Pagina()
		cambio(&p)
		f.GuardarPagina(p)
		if c.Refrescar != nil {
			c.Refrescar()
		}
	}
	margenes := []editor.Opcion{
		{Valor: "48", Rotulo: c.Traducir("layout.margins.narrow")},
		{Valor: "96", Rotulo: c.Traducir("layout.margins.normal")},
		{Valor: "128", Rotulo: c.Traducir("layout.margins.wide")},
	}
	selMargen := editor.NuevoSelectorCinta(c.Traducir("layout.margins.label"), AnchoSelectorMargenes, margenes,
		strconv.Itoa(int(pag.Margen)), func(v string) {
			n, _ := strconv.Atoi(v)
			poner(func(p *Pagina) { p.Margen, p.Margenes = float32(n), [4]float32{} })
		})
	vertical := editor.NuevoBotonCinta(iconos.EdVertical, c.Traducir("layout.orientation.portrait"),
		func() { poner(func(p *Pagina) { orientar(p, false) }) })
	vertical.Activo = !pag.Apaisada
	apaisada := editor.NuevoBotonCinta(iconos.EdApaisada, c.Traducir("layout.orientation.landscape"),
		func() { poner(func(p *Pagina) { orientar(p, true) }) })
	apaisada.Activo = pag.Apaisada
	// «A4» y «Legal» son nombres de formato; «Carta» se traduce.
	tamanos := []editor.Opcion{
		{Valor: "a4", Rotulo: c.Traducir("layout.size.a4")},
		{Valor: "carta", Rotulo: c.Traducir("layout.size.letter")},
		{Valor: "legal", Rotulo: c.Traducir("layout.size.legal")},
	}
	selTamano := editor.NuevoSelectorCinta(c.Traducir("layout.size.label"), AnchoSelectorTamano, tamanos, pag.Tamano,
		func(v string) { poner(func(p *Pagina) { p.Tamano, p.Ancho, p.Alto = v, 0, 0 }) })
	actual := "transparent"
	if pag.Papel.A > 0 {
		actual = hexDe(pag.Papel)
	}
	papel := editor.NuevaMuestraCinta(c, iconos.EdVertical, c.Traducir("layout.pageColor"), actual, editor.PaletaResaltado,
		func(v string) {
			col, _ := editor.Hex(v) // «transparent» no se lee: vuelve el blanco
			poner(func(p *Pagina) { p.Papel = col })
		})
	numerar := editor.NuevoBotonCinta(iconos.EdNotaPie, c.Traducir("layout.numbering.label"),
		func() { poner(func(p *Pagina) { p.Numerar = !p.Numerar }) })
	numerar.Activo = pag.Numerar

	interl := make([]editor.Opcion, len(Interlineados))
	for i, v := range Interlineados {
		interl[i] = editor.Opcion{Valor: strconv.FormatFloat(v, 'f', -1, 64), Rotulo: i18n.FormatNumber(in.loc, v)}
	}
	selInterl := editor.NuevoSelectorCinta(c.Traducir("paragraph.lineHeight"), AnchoSelectorCorto, interl, in.interlDisposicion,
		func(v string) { in.interlDisposicion = v; c.Mandar(editor.Interlineado, v) })
	selInterl.Inerte = c.SoloLectura
	grupos := []fyne.CanvasObject{
		editor.NuevoGrupo(c.Traducir("layout.margins"), selMargen),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("layout.orientation"), vertical, apaisada),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("layout.size"), selTamano),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("layout.pageColor"), papel),
		editor.SeparadorGrupos(),
	}
	var columnas fyne.CanvasObject
	if f != nil {
		marca, borde, cols := in.gruposDecoracion(c, f)
		grupos = append(grupos, marca, editor.SeparadorGrupos(), borde, editor.SeparadorGrupos())
		columnas = cols
	}
	grupos = append(grupos, editor.NuevoGrupo(c.Traducir("layout.numbering"), numerar), editor.SeparadorGrupos())
	if columnas != nil {
		grupos = append(grupos, columnas, editor.SeparadorGrupos())
	}
	return editor.Grupos(append(grupos,
		editor.NuevoGrupo(c.Traducir("layout.indent"),
			c.Boton(iconos.EdMenosSangria, "indent.decrease", false, editor.MenosSangria),
			c.Boton(iconos.EdMasSangria, "indent.increase", false, editor.MasSangria),
		),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("paragraph.lineHeight"), selInterl),
	)...)
}

// hexDe escribe un color como `#RRGGBB`, que es como lo guarda la paleta.
// orientar pone la página vertical o apaisada; la de un documento con su propio tamaño se gira.
func orientar(p *Pagina, apaisada bool) {
	if p.Ancho > 0 && (p.Ancho > p.Alto) != apaisada {
		p.Ancho, p.Alto = p.Alto, p.Ancho
	}
	p.Apaisada = apaisada
}

func hexDe(c color.NRGBA) string {
	const cifras = "0123456789ABCDEF"
	b := []byte{'#', 0, 0, 0, 0, 0, 0}
	for i, v := range []uint8{c.R, c.G, c.B} {
		b[1+2*i], b[2+2*i] = cifras[v>>4], cifras[v&15]
	}
	return string(b)
}

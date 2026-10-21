// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El cuerpo con formato (`RichTextEditor.tsx`)
//
// El cuerpo del redactor es el EDITOR DE NOTEPADS (`notes.EditorNota`): el mismo modelo
// de HTML, el mismo maquetador —`text-base leading-relaxed`, 19,91 de renglón, que es
// el del cuerpo de la web— y el mismo cursor, selección, deshacer y atajos (⌘/Ctrl+B,
// I, U, K). No se copia: se importa (docs/CONVENTIONS.md §13.2). Lo propio de Mail es la
// barra de la web —que es otra que la de Notepads—, su barra de tabla y su panel de
// imagen, las siete acciones del asistente de IA (las de un correo, no las de una nota),
// y lo que se hace con el HTML: sale como `multipart/alternative` (`eml.Salida.HTML`),
// con las imágenes del disco en `multipart/related`, y se guarda en el borrador.

// Medidas de la barra: `BARRA_DE_EDICION` sin fondo ni `px` (`!bg-transparent !px-0`),
// `gap-0.5 py-1` y su pelo abajo; el separador es `mx-1 h-5 w-px`; los desplegables,
// `w-20` y `w-24`. La barra de tabla es `px-1.5` y su rótulo `px-1 text-xs`.
const (
	huecoBarraFormato  float32 = 0.5 * 0.25 * tema.Rem
	pyBarraFormato     float32 = 1 * 0.25 * tema.Rem
	mxSeparadorFormato float32 = 1 * 0.25 * tema.Rem
	altoSeparador      float32 = 5 * 0.25 * tema.Rem
	anchoSelectorW20   float32 = 20 * 0.25 * tema.Rem
	anchoSelectorW24   float32 = 24 * 0.25 * tema.Rem
	glifoBarraFormato  float32 = 16
	pxBarraTabla       float32 = 1.5 * 0.25 * tema.Rem
	pxRotuloTabla      float32 = 1 * 0.25 * tema.Rem
)

// coloresTexto y resaltes son `COLORES` y `RESALTES` de la web: paletas cortas con
// contraste comprobado sobre blanco.
var (
	coloresTexto = []struct{ valor, clave string }{
		{"#2c3137", "editor.colorDefault"},
		{"#b91c1c", "editor.colorRed"},
		{"#b45309", "editor.colorAmber"},
		{"#15803d", "editor.colorGreen"},
		{"#1d4ed8", "editor.colorBlue"},
		{"#6d28d9", "editor.colorViolet"},
	}
	resaltes = []struct{ valor, clave string }{
		{"#fef08a", "editor.colorYellow"},
		{"#bbf7d0", "editor.colorGreen"},
		{"#bfdbfe", "editor.colorBlue"},
		{"#fecaca", "editor.colorRed"},
		{"transparent", "editor.colorNone"},
	}
	// estilos es `ESTILOS`: Normal, H1…H3 y el bloque de código.
	estilos = []editor.Opcion{
		{Valor: "p", Rotulo: "Normal"},
		{Valor: "h1", Rotulo: "H1"},
		{Valor: "h2", Rotulo: "H2"},
		{Valor: "h3", Rotulo: "H3"},
		{Valor: "pre", Rotulo: "</>"},
	}
	// tipografias es `TIPOGRAFIAS`: las que hay en Windows, macOS y Linux sin instalar
	// nada, para que el destinatario vea la que se eligió.
	tipografias = []editor.Opcion{
		{Valor: "Arial, Helvetica, sans-serif", Rotulo: "Arial"},
		{Valor: "Georgia, serif", Rotulo: "Georgia"},
		{Valor: `"Times New Roman", Times, serif`, Rotulo: "Times New Roman"},
		{Valor: "Verdana, Geneva, sans-serif", Rotulo: "Verdana"},
		{Valor: `"Courier New", Courier, monospace`, Rotulo: "Courier New"},
		{Valor: "Tahoma, Geneva, sans-serif", Rotulo: "Tahoma"},
	}
	// tamanos es `TAMANOS`: los siete de `fontSize` con su rótulo en puntos, que es lo que
	// se escribe (`font-size` en el `<span>`; el `<font size>` de execCommand no está en
	// el modelo).
	tamanos = []editor.Opcion{
		{Valor: "8pt", Rotulo: "8"},
		{Valor: "10pt", Rotulo: "10"},
		{Valor: "12pt", Rotulo: "12"},
		{Valor: "14pt", Rotulo: "14"},
		{Valor: "18pt", Rotulo: "18"},
		{Valor: "24pt", Rotulo: "24"},
		{Valor: "36pt", Rotulo: "36"},
	}
	// glifosTabla son los de la barra de tabla, en el orden de `notes.OrdenesTabla`.
	glifosTabla = []string{FilaEncimaD, FilaDebajoD, ColAntesD, ColDespuesD, BorrarFilaD, BorrarColD, BorrarTablaD}
)

// barraFormato es la `Toolbar` del editor: botones de `BOTON_DE_BARRA` con los glifos de
// Mail, `aria-pressed` donde la web lo consulta, y los desplegables que se aplican y
// vuelven a su rótulo (un selector que se quedara marcado mentiría al mover el cursor).
// Debajo, la barra de tabla (`tabla`), que el redactor enseña solo con el cursor en una
// celda.
type barraFormato struct {
	filaDeBarra
	ed      *notes.EditorNota
	botones []*botonOrden
	tabla   filaDeBarra
	// alEnlazar y alImagen abren el panel de enlace y el de imagen (los de la web, no las
	// herramientas de Notepads).
	alEnlazar, alImagen func()
	// ia es el botón del asistente, en su hueco; mientras contesta, el hueco lleva uno
	// apagado con la rueda (`rueda`) en el sitio del icono.
	ia    *editor.BotonCinta
	hueco *fyne.Container
	rueda *ui.Rueda
	t     *i18n.Translator
}

// botonOrden es un botón de la barra con la orden del editor que manda.
type botonOrden struct {
	*editor.BotonCinta
	orden, valor string
	conEstado    bool
}

// nuevaBarraFormato arma la barra. Con `alIA`, lleva al final el asistente, que abre su
// menú colgando del botón.
func nuevaBarraFormato(t *i18n.Translator, ed *notes.EditorNota, ventana float32, alEnlazar, alImagen func(),
	alIA func(fyne.CanvasObject),
) *barraFormato {
	b := &barraFormato{ed: ed, alEnlazar: alEnlazar, alImagen: alImagen, t: t}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	botonGlifo := func(d, rotulo, orden, valor string, conEstado bool) fyne.CanvasObject {
		bt := &botonOrden{orden: orden, valor: valor, conEstado: conEstado}
		bt.BotonCinta = editor.NuevoBotonCinta(0, rotulo, func() { b.aplicar(bt.orden, bt.valor) })
		bt.Glifo, bt.LadoGlifo = Glifo(d, glifoBarraFormato, "text-muted", v), glifoBarraFormato
		b.botones = append(b.botones, bt)
		return bt.BotonCinta
	}
	boton := func(d, clave, orden, valor string, conEstado bool) fyne.CanvasObject {
		return botonGlifo(d, t.T(clave, nil), orden, valor, conEstado)
	}
	lista := func(clave string, ancho float32, opciones []editor.Opcion, orden string) fyne.CanvasObject {
		var s *editor.SelectorCinta
		s = editor.NuevoSelectorCinta(t.T(clave, nil), ancho, append([]editor.Opcion{{Rotulo: t.T(clave, nil)}}, opciones...), "",
			func(valor string) {
				s.Valor = "" // vuelve a su rótulo, como el `<select>` de la web
				s.Refresh()
				if valor != "" {
					b.aplicar(orden, valor)
				}
			})
		return s
	}
	paleta := func(ps []struct{ valor, clave string }) []editor.Opcion {
		out := make([]editor.Opcion, 0, len(ps))
		for _, p := range ps {
			out = append(out, editor.Opcion{Valor: p.valor, Rotulo: t.T(p.clave, nil)})
		}
		return out
	}
	objs := []fyne.CanvasObject{
		boton(DeshacerD, "editor.undo", "undo", "", false), boton(RehacerD, "editor.redo", "redo", "", false), separadorDeBarra(v),
		lista("editor.font", anchoSelectorW24, tipografias, "fuente"), lista("editor.size", anchoSelectorW20, tamanos, "tamano"),
		lista("editor.style", anchoSelectorW20, estilos, "bloque"), separadorDeBarra(v),
		boton(NegritaD, "editor.bold", "b", "", true), boton(CursivaD, "editor.italic", "i", "", true),
		boton(SubrayadoD, "editor.underline", "u", "", true), boton(TachadoD, "editor.strikethrough", "s", "", true),
		lista("editor.textColor", anchoSelectorW20, paleta(coloresTexto), "color"),
		lista("editor.highlight", anchoSelectorW20, paleta(resaltes), "resaltar"), separadorDeBarra(v),
		boton(AlinearIzqD, "editor.alignLeft", "alinear", "left", true), boton(CentrarD, "editor.alignCenter", "alinear", "center", true),
		boton(AlinearDerD, "editor.alignRight", "alinear", "right", true), boton(JustificarD, "editor.alignJustify", "alinear", "justify", true),
		separadorDeBarra(v),
		boton(VinetasD, "editor.bulletList", "vinetas", "", true), boton(NumerosD, "editor.numberList", "numeros", "", true),
		boton(DesangrarD, "editor.outdent", "desangrar", "", false), boton(SangrarD, "editor.indent", "sangrar", "", false),
		boton(CitaD, "editor.quote", "bloque", "blockquote", false), separadorDeBarra(v),
		// Objetos: la tabla de 3×3 de partida, que crece con la barra de tabla.
		boton(TablaD, "editor.table", "tabla", "", false), boton(ImagenD, "editor.image", "imagen", "", false),
		boton(ReglaD, "editor.rule", "regla", "", false), boton(EnlaceD, "editor.link", "enlace", "", false),
		boton(QuitarFormatoD, "editor.clearFormat", "limpiar", "", false),
	}
	if alIA != nil {
		// EL ASISTENTE VA AL FINAL Y TRAS SEPARADOR: no es formato, reescribe lo escrito, y
		// entre negrita y cursiva se pulsaría sin querer.
		b.ia = editor.NuevoBotonCinta(0, t.T("editor.ai", nil), nil)
		b.ia.Glifo, b.ia.LadoGlifo = iconos.Trazo(iconos.IA, glifoBarraFormato, "text-muted", v), glifoBarraFormato
		b.ia.AlPulsar = func() { alIA(b.hueco) }
		b.hueco = container.NewStack(b.ia)
		objs = append(objs, separadorDeBarra(v), b.hueco)
	}
	b.filaDeBarra = nuevaFilaDeBarra(ventana, 0, objs...)

	// La barra de tabla: el rótulo, las cuatro de insertar y, tras un separador, las tres
	// de eliminar.
	filas := []fyne.CanvasObject{container.New(ui.Margen{Izq: pxRotuloTabla, Der: pxRotuloTabla},
		ui.NuevoRotulo(t.T("editor.table", nil), tema.TextoXS, ui.Normal, "text-subtle"))}
	for i, o := range notes.OrdenesTabla {
		if i == 4 {
			filas = append(filas, separadorDeBarra(v))
		}
		filas = append(filas, botonGlifo(glifosTabla[i], t.T(o.Clave, nil), o.Orden, "", false))
	}
	b.tabla = nuevaFilaDeBarra(ventana, pxBarraTabla, filas...)
	b.refrescar()
	return b
}

// corteBarraMD es `md:` (768 de ventana): desde ahí la barra se parte en renglones.
const corteBarraMD float32 = 768

// filaDeBarra es una fila de botones con su `px`: desde `md` se parte en renglones
// (`md:flex-wrap`) y por debajo es una tira que se desplaza de lado (`overflow-x-auto`).
type filaDeBarra struct {
	caja *fyne.Container
	tira *ui.TiraDeslizable
	px   float32
}

func nuevaFilaDeBarra(ventana, px float32, objs ...fyne.CanvasObject) filaDeBarra {
	f := filaDeBarra{caja: container.New(&filaBarraFormato{px: px}, objs...), px: px}
	if ventana > 0 && ventana < corteBarraMD {
		f.tira = ui.NuevaTiraDeslizable(editor.LadoBotonCinta, huecoBarraFormato, objs...)
		f.tira.AnchoPropio = true
		f.caja = container.New(ui.Margen{Izq: px, Der: px, Arr: pyBarraFormato, Abj: pyBarraFormato}, f.tira)
	}
	return f
}

// alto es lo que ocupa la fila a ese ancho, sin su pelo.
func (f filaDeBarra) alto(ancho float32) float32 {
	if f.tira != nil {
		return editor.LadoBotonCinta + 2*pyBarraFormato
	}
	return (&filaBarraFormato{px: f.px}).colocar(f.caja.Objects, ancho, false)
}

// aplicar manda la orden al editor y le devuelve el foco, que es lo que hace el
// `onMouseDown={preventDefault}` de la web: pulsar la barra no saca el cursor del cuerpo.
// Enlace e imagen abren su panel.
func (b *barraFormato) aplicar(orden, valor string) {
	switch abrir := map[string]func(){"enlace": b.alEnlazar, "imagen": b.alImagen}[orden]; {
	case abrir != nil:
		abrir()
		return
	case orden == "enlace" || orden == "imagen":
		return
	}
	b.ed.Orden(orden, valor)
	b.refrescar()
	zona, ok := b.ed.Zona().(fyne.CanvasObject)
	if !ok {
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(zona); c != nil {
		c.Focus(b.ed.Zona())
	}
}

// refrescar enciende los botones cuyo estado se consulta (`COMANDOS_CON_ESTADO`).
func (b *barraFormato) refrescar() {
	for _, bt := range b.botones {
		activo := bt.conEstado && b.ed.Activa(bt.orden, bt.valor)
		if activo != bt.Activo {
			bt.Activo = activo
			bt.Refresh()
		}
	}
}

// trabajandoIA quita el botón del asistente mientras contesta y pone en su sitio la rueda
// (`<Spinner size={16} label="editor.aiWorking">`).
func (b *barraFormato) trabajandoIA(si bool) {
	if b.hueco == nil {
		return
	}
	if b.rueda != nil {
		b.rueda.Parar()
		b.rueda = nil
	}
	b.hueco.Objects = []fyne.CanvasObject{b.ia}
	if si { // el botón `disabled` en reposo no tiene caja: se ve solo la rueda
		b.rueda = ui.NuevaRueda(glifoBarraFormato, "text-muted", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
		b.rueda.Rotulo = b.t.T("editor.aiWorking", nil)
		b.hueco.Objects = []fyne.CanvasObject{container.NewGridWrap(editor.MedidaBotonCinta(), container.NewCenter(b.rueda))}
	}
	b.hueco.Refresh()
}

// ---- El asistente de IA ----

// accionesIA son las del redactor (`AccionIA` de lib/ai.ts), en su orden; la vacía es
// el separador. No son las de Notepads: en un correo se ajusta el tono, se acorta y se
// responde.
var accionesIA = []struct{ accion, clave string }{
	{"mejorar", "editor.ai.improve"},
	{"corregir", "editor.ai.fix"},
	{"acortar", "editor.ai.shorten"},
	{"", ""},
	{"formal", "editor.ai.formal"},
	{"cercano", "editor.ai.casual"},
	{"", ""},
	{"responder", "editor.ai.reply"},
	{"traducir", "editor.ai.translate"},
}

// directivasIA son las `DIRECTIVAS` de lib/ai.ts, tal cual: todas acaban pidiendo SOLO el
// texto, o el «Claro, aquí tienes…» del modelo acaba en el correo que alguien envía.
var directivasIA = map[string]string{
	"mejorar":   "Mejora la redacción de este correo (gramática, estilo y fluidez) sin cambiar lo que dice ni su tono. Devuelve solo el texto.",
	"corregir":  "Corrige la ortografía, la gramática y la puntuación de este correo. No reescribas ni cambies el estilo. Devuelve solo el texto corregido.",
	"acortar":   "Acorta este correo dejando lo esencial y todos los datos concretos (fechas, cifras, nombres). Devuelve solo el texto.",
	"formal":    "Reescribe este correo en un tono más formal y profesional, sin añadir información que no esté. Devuelve solo el texto.",
	"cercano":   "Reescribe este correo en un tono más cercano y natural, sin perder la información. Devuelve solo el texto.",
	"responder": "Redacta una respuesta a este correo, breve y educada, en el mismo idioma. Devuelve solo el texto de la respuesta.",
	// La web no deja elegir el idioma: traduce al de por defecto (`IDIOMA_POR_DEFECTO`).
	"traducir": "Traduce este correo a inglés. Devuelve solo la traducción.",
}

// PeticionIA es `construirPeticion`: la acción con su directiva, el texto recortado a
// `MAX_CONTEXTO` y el tipo `email`.
func PeticionIA(accion, texto string) api.PeticionIA {
	r := []rune(strings.TrimSpace(texto))
	return api.PeticionIA{
		Accion: accion, Tipo: "email", Instruccion: directivasIA[accion],
		Contexto: string(r[:min(len(r), notes.MaxContextoIA)]),
	}
}

// separadorDeBarra es `SEPARADOR_DE_BARRA`.
func separadorDeBarra(v tema.Variante) fyne.CanvasObject {
	return container.New(separadorBarra{}, canvas.NewRectangle(tema.Color("border", v)))
}

// separadorBarra es `mx-1 h-5 w-px self-center`.
type separadorBarra struct{}

func (separadorBarra) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(2*mxSeparadorFormato+tema.Borde, altoSeparador)
}

func (separadorBarra) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(mxSeparadorFormato, (t.Height-altoSeparador)/2))
	objs[0].Resize(fyne.NewSize(tema.Borde, altoSeparador))
}

// filaBarraFormato es `flex flex-wrap items-center gap-0.5 py-1` con su `px`: la barra
// se parte en renglones cuando no cabe.
//
// Por debajo de `md` no se parte: es la `ui.TiraDeslizable` de filaDeBarra.
type filaBarraFormato struct{ ancho, px float32 }

func (f *filaBarraFormato) colocar(objs []fyne.CanvasObject, ancho float32, mover bool) float32 {
	lado := editor.LadoBotonCinta
	x, y := f.px, pyBarraFormato
	for _, o := range objs {
		m := o.MinSize()
		if x > f.px && ancho > 0 && x+m.Width > ancho-f.px {
			x, y = f.px, y+lado+huecoBarraFormato
		}
		if mover {
			o.Move(fyne.NewPos(x, y+(lado-m.Height)/2))
			o.Resize(m)
		}
		x += m.Width + huecoBarraFormato
	}
	return y + lado + pyBarraFormato
}

func (f *filaBarraFormato) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, f.colocar(objs, f.ancho, false))
}

func (f *filaBarraFormato) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	f.ancho = t.Width
	f.colocar(objs, t.Width, true)
}

// ---- El HTML del cuerpo ----

// HTMLDeTexto es `textToHtml` pensado para editar: un `<div>` por línea (vacía, con su
// `<br>`), que es lo que escribe un `contenteditable` al pulsar Intro. La cita de una
// respuesta y la firma entran así en el editor.
func HTMLDeTexto(texto string) string {
	if texto == "" {
		return ""
	}
	var b strings.Builder
	for _, l := range strings.Split(strings.ReplaceAll(texto, "\r\n", "\n"), "\n") {
		if l == "" {
			b.WriteString("<div><br/></div>")
			continue
		}
		b.WriteString("<div>" + htmltext.Escape(l) + "</div>")
	}
	return b.String()
}

// TextoDe es la parte de texto plano del mensaje: un renglón por bloque, las listas con
// su viñeta o su número, la cita con «> » y las celdas de una fila de tabla separadas por
// un tabulador. Sale del MODELO y no del HTML para no perder lo que el HTML no dice en
// claro (el «-- » de la firma lleva su espacio). Una imagen no tiene texto: la letra que
// ocupa en el modelo (`notes.Objeto`) no llega aquí.
func TextoDe(d *notes.Documento) string {
	lineas := make([]string, 0, len(d.Bloques))
	n := 0
	var antes notes.Celda
	for _, bl := range d.Bloques {
		prefijo := ""
		switch bl.Tipo {
		case notes.Vineta:
			prefijo = "• "
		case notes.Numerada:
			n++
			prefijo = strconv.Itoa(n) + ". "
		case notes.Cita:
			prefijo = "> "
		case notes.Parrafo, notes.Titulo, notes.Codigo, notes.Tarea, notes.Regla, notes.BloqueOpaco:
		}
		if bl.Tipo != notes.Numerada {
			n = 0
		}
		linea := prefijo + strings.ReplaceAll(bl.Texto(), string(notes.Objeto), "")
		// ponytail: una celda de varios bloques pone los de detrás en renglón propio.
		if c := bl.Celda; c.EnTabla() && len(lineas) > 0 && c.Tabla == antes.Tabla && c.Fila == antes.Fila && c.Col != antes.Col {
			lineas[len(lineas)-1] += "\t" + linea
		} else {
			lineas = append(lineas, linea)
		}
		antes = bl.Celda
	}
	return strings.Join(lineas, "\n")
}

// ---- Los paneles de enlace y de imagen ----

// Medidas del panel: `border-b flex flex-wrap items-end gap-2 px-3 py-2`, cada campo con
// su rótulo `text-xs text-muted` a `gap-1`, el texto `w-48` y la dirección `w-72`
// (`max-md:w-full`), los dos `h-8`.
const (
	pxPanelEnlace    float32 = 3 * 0.25 * tema.Rem
	pyPanelEnlace    float32 = 2 * 0.25 * tema.Rem
	huecoPanelEnlace float32 = 2 * 0.25 * tema.Rem
	anchoTextoEnlace float32 = 48 * 0.25 * tema.Rem
	anchoURLEnlace   float32 = 72 * 0.25 * tema.Rem
	cortePanelMD     float32 = 768
)

// panelEnlace es el panel de bajo la barra (`panel` de la web), que sustituye al
// `window.prompt`: el de ENLACE pide el rótulo —el texto seleccionado, si lo hay— y la
// dirección; el de IMAGEN, el fichero del equipo o, tras «URL», la dirección. Son el
// mismo panel porque son la misma pregunta. Intro lo aplica y Escape o «Cancelar» lo
// cierran.
type panelEnlace struct {
	texto, url *ui.Entrada
	caja       *fyne.Container
}

// entradaPanel es un campo del panel: `h-8` sobre el `Input` de siempre (`px-3
// text-base`), 28 de alto, con su ancho desde `md` y a lo ancho por debajo.
func entradaPanel(marca string, ancho, ventana float32) *ui.Entrada {
	h8 := ui.CampoPropio{MargenX: tema.Espacio(3), MargenY: (tema.Espacio(8) - 2*tema.Borde - tema.TextoBase.Linea) / 2, Texto: tema.TextoBase}
	e := ui.NuevaEntrada(marca).ConPropia(h8)
	if ventana >= cortePanelMD {
		e.Ancho = ancho // si no, `max-md:w-full`: cada uno en su renglón
	}
	return e
}

// campoPanel es un campo con su rótulo encima.
func campoPanel(t *i18n.Translator, clave string, o fyne.CanvasObject) fyne.CanvasObject {
	return container.New(ui.Columna{Hueco: tema.Espacio(1)}, ui.NuevoRotulo(t.T(clave, nil), tema.TextoXS, ui.Normal, "text-muted"), o)
}

// conTeclas hace de Intro aplicar y de Escape cerrar.
func conTeclas(e *ui.Entrada, ir, cerrar func()) {
	e.AlTeclear = func(k *fyne.KeyEvent) bool {
		switch k.Name {
		case fyne.KeyReturn, fyne.KeyEnter:
			ir()
			return true
		case fyne.KeyEscape:
			cerrar()
			return true
		default:
			return false
		}
	}
}

func nuevoPanelEnlace(t *i18n.Translator, rotulo string, ventana float32, aplicar func(url, rotulo string), cerrar func()) *panelEnlace {
	p := &panelEnlace{texto: entradaPanel("", anchoTextoEnlace, ventana), url: entradaPanel("https://", anchoURLEnlace, ventana)}
	p.texto.PonerTexto(rotulo)
	ir := func() { aplicar(strings.TrimSpace(p.url.Texto()), strings.TrimSpace(p.texto.Texto())) }
	conTeclas(p.url, ir, cerrar)
	aplicarBt := ui.NuevoBoton(t.T("editor.linkApply", nil), ir).Con(ui.Primario, ui.TallaSM)
	cancelar := ui.NuevoBoton(t.T("editor.linkCancel", nil), cerrar).Con(ui.Fantasma, ui.TallaSM)
	p.caja = container.New(&filaPanelEnlace{}, campoPanel(t, "editor.linkText", p.texto), campoPanel(t, "editor.linkUrl", p.url), aplicarBt, cancelar)
	return p
}

// nuevoPanelImagen es el panel de imagen: PRIMERO el fichero —lo normal, una sola
// acción— y la dirección detrás de «URL», que al pulsarlo se cambia por el campo y
// «Insertar imagen» (`alCambiar` recoloca y lleva el foco a él). Sin `delEquipo` solo se
// ofrece la dirección.
func nuevoPanelImagen(t *i18n.Translator, ventana float32, delEquipo func(), aplicar func(url string), cerrar, alCambiar func()) *panelEnlace {
	p := &panelEnlace{}
	cancelar := ui.NuevoBoton(t.T("editor.linkCancel", nil), cerrar).Con(ui.Fantasma, ui.TallaSM)
	var fichero []fyne.CanvasObject
	if delEquipo != nil {
		elegir := ui.NuevoBoton(t.T("smime.chooseFile", nil), func() { cerrar(); delEquipo() }).Con(ui.Secundario, ui.TallaSM)
		fichero = append(fichero, campoPanel(t, "editor.imageFile", elegir))
	}
	porURL := ui.NuevoBoton(t.T("editor.imageByUrl", nil), func() {
		p.url = entradaPanel("https://", anchoURLEnlace, ventana)
		ir := func() { aplicar(strings.TrimSpace(p.url.Texto())) }
		conTeclas(p.url, ir, cerrar)
		insertar := ui.NuevoBoton(t.T("editor.imageApply", nil), ir).Con(ui.Primario, ui.TallaSM)
		p.caja.Objects = slices.Concat(fichero, []fyne.CanvasObject{campoPanel(t, "editor.imageUrl", p.url), insertar, cancelar})
		alCambiar()
	}).Con(ui.Secundario, ui.TallaSM)
	p.caja = container.New(&filaPanelEnlace{}, slices.Concat(fichero, []fyne.CanvasObject{porURL, cancelar})...)
	return p
}

// URLDeImagenValida es la de `insertarImagen`: solo `https://`, lo único que el saneado
// de salida deja pasar (un `http:` no lo vería el destinatario y delataría su lectura).
func URLDeImagenValida(u string) bool { return strings.HasPrefix(strings.ToLower(u), "https://") }

// URLDeEnlaceValida es la condición de la web: `http(s)://` o `mailto:`.
func URLDeEnlaceValida(u string) bool {
	l := strings.ToLower(u)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "mailto:")
}

// filaPanelEnlace es `flex flex-wrap items-end gap-2` dentro de su `px-3 py-2`; un campo
// sin ancho propio (`w-full`) ocupa el renglón.
type filaPanelEnlace struct{}

func (f *filaPanelEnlace) colocar(objs []fyne.CanvasObject, ancho float32, mover bool) float32 {
	util := max(ancho-2*pxPanelEnlace, 0)
	x, y, alto := float32(0), pyPanelEnlace, float32(0)
	fila := make([]fyne.CanvasObject, 0, len(objs))
	cerrarFila := func() {
		for _, o := range fila {
			if mover {
				o.Move(fyne.NewPos(o.Position().X, y+alto-o.Size().Height)) // `items-end`
			}
		}
		y += alto + huecoPanelEnlace
		x, alto, fila = 0, 0, nil
	}
	for _, o := range objs {
		m := o.MinSize()
		w := m.Width
		if w <= 0 || w > util {
			w = util
		}
		if x > 0 && x+w > util {
			cerrarFila()
		}
		if mover {
			o.Move(fyne.NewPos(pxPanelEnlace+x, y))
			o.Resize(fyne.NewSize(w, m.Height))
		}
		fila = append(fila, o)
		x += w + huecoPanelEnlace
		alto = max(alto, m.Height)
	}
	cerrarFila()
	return y - huecoPanelEnlace + pyPanelEnlace
}

func (f *filaPanelEnlace) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, f.colocar(objs, 1e4, false))
}

func (f *filaPanelEnlace) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	f.colocar(objs, t.Width, true)
}

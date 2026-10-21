// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

// Cabecera es la cabecera del editor (§6.1): volver, la marca del producto con su
// filete, título editable, chip de guardado, y a la derecha compartir y descargar.
//
// La flecha de volver es la ÚNICA diferencia deliberada con la web: allí sale al Hub,
// porque la lista de Docus es una página más de un subdominio y la portada conjunta
// vive en el Hub. En nativo cada producto es su propia aplicación con su portada, y
// salir del documento es volver a ella (`ui.backToList`); ir al Hub es cambiar de app,
// y eso lo hace el sistema, no una ventana (`docs/CONVENTIONS.md` §13.6).
//
// EditorDemo remoto: 48 de alto, px-3=12 y gap-2=8; título h-7=28,
// text-sm13/peso600/radio4; botones de cabecera h-8=32, px-3 y radio4.
//
// No lleva la TopBar de la suite: es una de las dos excepciones escritas a la paridad
// (`docs/CONVENTIONS.md` §13.6) — en nativo el sistema ya pone la barra de la ventana.
type Cabecera struct {
	widget.BaseWidget

	ctx *Contexto

	volver    *BotonCinta
	marca     *iconos.Dibujo
	filete    *canvas.Rectangle
	titulo    *ui.Entrada
	compartir *ui.Boton
	descargar *ui.Boton
	menu      *ui.Menu

	// guardadoEn es cuándo pasó el chip a «Guardado», para «hace N s»; ultimo, el
	// estado anterior, y reloj si el tic de 5 s está en marcha.
	guardadoEn time.Time
	ultimo     Guardado
	reloj      bool
}

var _ fyne.Widget = (*Cabecera)(nil)

// NuevaCabecera monta la cabecera. `alVolver` es el «Volver a tus documentos».
func NuevaCabecera(ctx *Contexto, alVolver func()) *Cabecera {
	c := &Cabecera{ctx: ctx}
	c.ExtendBaseWidget(c)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c.volver = NuevoBotonCinta(iconos.EdChevronIzq, ctx.Traducir("ui.backToList"), alVolver)
	c.marca = iconos.Marca(ctx.Tipo.Producto(), LadoMarcaCabecera, v)
	c.filete = canvas.NewRectangle(tema.Color("border", v))
	c.titulo = ui.NuevaEntrada(ctx.Traducir("header.title.label")).ConPropia(ui.CampoPropio{
		// El editor común reserva 1 px a cada lado para la caja. Plano la apaga:
		// el margen 5 deja el texto en px-1.5=6; 18+2*4+2 da h-7=28.
		MargenX: tema.Espacio(1.5) - tema.Borde, MargenY: 4,
		Texto: tema.TextoSM, Peso: ui.SemiNegrita, Radio: tema.RadioSM, Plano: true, FondoActivo: "bg-subtle",
	})
	c.titulo.Radio = tema.RadioSM
	c.titulo.Ancho = AnchoTitulo
	c.titulo.AlCambiar(func(s string) {
		if s == ctx.Titulo {
			return
		}
		ctx.Titulo = s
		if ctx.AlRenombrar != nil {
			ctx.AlRenombrar(s)
		}
	})
	c.compartir = ui.NuevoBoton(ctx.Traducir("header.share"), nil).Con(ui.Primario, ui.TallaSM)
	// Fantasma y no Secundario: en la suite `secondary` es el MISMO relleno que `primary` con
	// borde, y la cabecera enseñaba dos llamadas a la acción iguales, una al lado de la otra.
	// La que importa es Compartir; descargar es un comando más (también está en «Archivo»).
	c.descargar = ui.NuevoBoton(ctx.Traducir("header.download"), c.abrirDescargas).Con(ui.Fantasma, ui.TallaSM)
	// `leadingIcon={<EditorIcon name="download" />}`: el `lg` de 18, en la tinta del botón
	// (`text-primary-text` del fantasma).
	c.descargar.Glifo = iconos.TrazoEd(iconos.EdDescargar, IconoBotonCinta, "primary-text", v)
	c.descargar.TamIcono = IconoBotonCinta
	c.Sincronizar()
	return c
}

// EnfocarTitulo lleva el foco al título: «Renombrar» del menú «Archivo»
// (`document.getElementById('editor-title-input')?.focus()`).
func (c *Cabecera) EnfocarTitulo() {
	if cv := fyne.CurrentApp().Driver().CanvasForObject(c); cv != nil {
		cv.Focus(c.titulo)
	}
}

// LadoMarcaCabecera es el `<ProductIcon size={24}>`, y AltoFileteCabecera el `h-6 w-px`
// que la separa del título. Los dos son `max-md:hidden`.
var (
	LadoMarcaCabecera  float32 = 24
	AltoFileteCabecera         = tema.Espacio(6)
)

// Sincronizar trae a la cabecera el título, el permiso y la descarga en curso del
// contexto: el documento llega DESPUÉS de montar el editor, y el campo tiene que
// enterarse. El título solo se reescribe si es OTRO: mientras se teclea, el campo ya lo
// ha copiado al contexto, y reescribirlo movería el cursor al final a cada letra.
func (c *Cabecera) Sincronizar() {
	if c.titulo.Texto() != c.ctx.Titulo {
		c.titulo.PonerTexto(c.ctx.Titulo)
	}
	c.titulo.Inerte(c.ctx.AlRenombrar == nil)
	// `loading={exporting !== null}` y `disabled={connection === 'offline'}`.
	c.descargar.Cargando = c.ctx.Exportando != ""
	// Se apagan sin red, como en la web. «Compartir» también sin gancho —un fichero
	// abierto del disco no tiene a quién compartirse—: encendido y sin hacer nada era
	// lo que pasaba en Diapos (visto en Windows), igual que la cinta (botonCompartir).
	c.descargar.Inerte = c.ctx.SinRed
	c.compartir.Inerte = c.ctx.SinRed || c.ctx.Compartir == nil
	c.compartir.AlPulsar = nil
	if f := c.ctx.Compartir; f != nil {
		c.compartir.AlPulsar = func() { f(c.compartir) }
	}
	c.descargar.Refresh()
	c.compartir.Refresh()
	c.marcarGuardado()
}

// extensiones son las de ODF y OOXML de cada tipo (`EXPORT_EXT`), que el menú de la
// cabecera escribe entre paréntesis.
func extensiones(t Tipo) (odf, ooxml string) {
	switch t {
	case Hoja:
		return ".ods", ".xlsx"
	case Presentacion:
		return ".odp", ".pptx"
	case Documento:
	}
	return ".odt", ".docx"
}

// MenuDescargas es el Dropdown de «Descargar»: los formatos en el orden de
// `EXPORT_FORMATS`, con el nombre del estándar y la extensión que le toca. El que se
// está generando dice «Generando…» y todos se apagan hasta que acabe.
func (c *Cabecera) MenuDescargas() *ui.Menu {
	odf, ooxml := extensiones(c.ctx.Tipo)
	rotulos := map[string]string{
		"pdf": "PDF", "html": "HTML", "markdown": "Markdown",
		"odf": "ODF (" + odf + ")", "ooxml": "OOXML (" + ooxml + ")",
	}
	formatos := office.Exports()
	elems := make([]fyne.CanvasObject, 0, len(formatos))
	for _, command := range formatos {
		f := command.ID
		texto := rotulos[f]
		if c.ctx.Exportando == f {
			texto = c.ctx.Traducir("header.download.generating")
		}
		e := ui.NuevoElementoMenu(texto, iconos.Ninguno, func() {
			if c.ctx.Exportar != nil {
				c.ctx.Exportar(f)
			}
		})
		e.Inerte = !command.Available(c.ctx.SinRed, c.ctx.Exportando != "") || c.ctx.Exportar == nil
		elems = append(elems, e)
	}
	m := ui.NuevoMenu(elems...)
	m.Rotulo = c.ctx.Traducir("header.download")
	return m
}

func (c *Cabecera) abrirDescargas() {
	if c.menu != nil && c.menu.Abierto() {
		c.menu.Cerrar()
		return
	}
	c.menu = c.MenuDescargas()
	c.menu.Mostrar(c.descargar)
}

// AccessibilityRole la anuncia como contenedor con nombre.
func (c *Cabecera) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// AccessibilityLabel es el `role="banner"` del editor.
func (c *Cabecera) AccessibilityLabel() string { return c.ctx.Traducir("header.label") }

// MinSize es la altura declarada; el ancho lo pone el marco.
func (c *Cabecera) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	return fyne.NewSize(0, AltoCabecera)
}

// textoGuardado es el chip de `aria-live="polite"`: clave de catálogo y token del
// punto de color, en el orden de `SaveState`.
func textoGuardado(g Guardado) (clave, token string) {
	switch g {
	case Guardando:
		return "header.save.saving", "warning"
	case Sucio:
		return "header.save.dirty", "warning"
	case FalloAlGuardar:
		return "header.save.error", "danger"
	case Limpio:
	}
	return "header.save.saved", "success"
}

// PasoRelojGuardado es la granularidad del «Guardado hace N s» de la web: 5 s.
const PasoRelojGuardado = 5 * time.Second

// textoChip es lo que dice el chip ahora, con la frase ENTERA del catálogo (en alemán el
// complemento va delante): «Guardado», «Guardado hace 15 s», «Guardado hace 2 min», o
// «Guardado en este dispositivo» sin red (`savedLabel` de EditorHeader.tsx).
func (c *Cabecera) textoChip(ahora time.Time) (texto, token string) {
	clave, token := textoGuardado(c.ctx.Guardado)
	switch {
	case c.ctx.SinRed:
		return c.ctx.Traducir("header.save.offline"), "warning"
	case c.ctx.Guardado != Limpio || c.guardadoEn.IsZero():
		return c.ctx.Traducir(clave), token
	}
	s := int(ahora.Sub(c.guardadoEn).Round(time.Second) / time.Second)
	switch {
	case s < 5:
		return c.ctx.Traducir(clave), token
	case s < 60:
		return c.ctx.Traducir("header.save.agoSeconds", i18n.Vars{"seconds": s}), token
	}
	return c.ctx.Traducir("header.save.agoMinutes", i18n.Vars{"minutes": (s + 30) / 60}), token
}

// marcarGuardado apunta cuándo pasó a «Guardado» y arranca el reloj que reescribe el
// chip cada 5 s mientras siga así. El reloj se para solo: en cuanto hay cambios o la
// cabecera ya no está en ninguna ventana.
//
// ponytail: cuenta desde el primer GUARDADO, no desde que se abre el documento (la web
// cuenta desde que monta el editor): antes de guardar no hay nada que el chip tenga que
// reescribir, así que no se arma ningún reloj. En las pruebas el reloj no pisa a nadie:
// desde el fork v2.8.1-sytue.4 su `fyne.Do` lo ejecuta la rutina de la prueba en curso.
func (c *Cabecera) marcarGuardado() {
	anterior := c.ultimo
	c.ultimo = c.ctx.Guardado
	if c.ctx.Guardado != Limpio || anterior == Limpio {
		return
	}
	c.guardadoEn = time.Now()
	if c.reloj {
		return
	}
	c.reloj = true
	var tic func()
	tic = func() {
		fyne.Do(func() {
			if fyne.CurrentApp().Driver().CanvasForObject(c) == nil || c.ctx.Guardado != Limpio {
				c.reloj = false
				return
			}
			c.Refresh()
			time.AfterFunc(PasoRelojGuardado, tic)
		})
	}
	time.AfterFunc(PasoRelojGuardado, tic)
}

// CreateRenderer compone el fondo, la línea, el chip y los controles.
func (c *Cabecera) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	clave, token := c.textoChip(time.Now())
	r := &cabeceraRenderer{
		c:      c,
		fondo:  canvas.NewRectangle(tema.Color("surface", v)),
		linea:  canvas.NewRectangle(tema.Color("border", v)),
		punto:  canvas.NewCircle(tema.Color(token, v)),
		estado: ui.Texto(clave, tema.TextoXS, ui.Normal, tema.Color("text-muted", v)),
	}
	return r
}

// LadoPuntoGuardado es el `h-1.5 w-1.5` del punto del chip.
var LadoPuntoGuardado = tema.Espacio(1.5)

// HuecoChip es el `gap-1.5` entre el punto y su texto.
var HuecoChip = tema.Espacio(1.5)

type cabeceraRenderer struct {
	c      *Cabecera
	fondo  *canvas.Rectangle
	linea  *canvas.Rectangle
	punto  *canvas.Circle
	estado *canvas.Text
}

func (r *cabeceraRenderer) MinSize() fyne.Size { return r.c.MinSize() }

func (r *cabeceraRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{
		r.fondo, r.linea, r.c.volver, r.c.marca.Objeto(), r.c.filete, r.c.titulo, r.punto, r.estado,
		r.c.compartir, r.c.descargar,
	}
}

func (r *cabeceraRenderer) Destroy() {}

func (r *cabeceraRenderer) Layout(t fyne.Size) {
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	r.linea.Move(fyne.NewPos(0, t.Height-tema.Borde))
	r.linea.Resize(fyne.NewSize(t.Width, tema.Borde))

	centrar := func(o fyne.CanvasObject, x float32) float32 {
		m := o.MinSize()
		o.Move(fyne.NewPos(x, (t.Height-tema.Borde-m.Height)/2))
		o.Resize(m)
		return x + m.Width + HuecoCabecera
	}

	x := MargenCabecera
	x = centrar(r.c.volver, x)
	// `max-md:hidden`: md es una media query, así que mide la VENTANA —que es el ancho
	// de esta banda— y con el rem inicial de 16 (768), no con el 14 de la suite.
	ancha := t.Width >= ui.CorteMD
	r.c.marca.Ver(ancha)
	if ancha {
		r.c.marca.Colocar(x, (t.Height-tema.Borde-LadoMarcaCabecera)/2, LadoMarcaCabecera)
		x += LadoMarcaCabecera + HuecoCabecera
		r.c.filete.Show()
		r.c.filete.Move(fyne.NewPos(x, (t.Height-tema.Borde-AltoFileteCabecera)/2))
		r.c.filete.Resize(fyne.NewSize(tema.Borde, AltoFileteCabecera))
		x += tema.Borde + HuecoCabecera
	} else {
		r.c.filete.Hide()
	}
	chipTexto, _ := r.c.textoChip(time.Now())
	chip := LadoPuntoGuardado + HuecoChip + ui.Medir(chipTexto, tema.TextoXS, ui.Normal).Width
	if ancha {
		// El título se ciñe a lo que pide su texto, con el `max-w-[360px]` como tope: en
		// producción mide 217,5, no 360. El suelo es el ancho de la marca de agua, que si
		// no se sale de la caja y se mete debajo del chip de guardado — se vio en la
		// captura con el documento sin título.
		texto := r.c.titulo.Texto()
		if texto == "" {
			texto = r.c.ctx.Traducir("header.title.label")
		}
		ancho := min(ui.Medir(texto, tema.TextoSM, ui.SemiNegrita).Width+
			2*tema.Espacio(1.5), AnchoTitulo)
		r.c.titulo.Ancho = max(ancho, tema.Espacio(24))
	} else {
		// En un móvil (`max-md:`) no hay hueco flexible ni «Descargar» —está en Archivo—,
		// y el título es el `flex-1` que se queda lo que dejan el chip y «Compartir», entre
		// `min-w-24` y `max-w-[40vw]`; si ni así cabe, el chip cede y se corta. Medido en
		// raíz de 16: h-7=28, px-1.5=6 y el chip no se pisa con Compartir.
		libre := t.Width - x - MargenCabecera - r.c.compartir.MinSize().Width - 2*HuecoCabecera
		r.c.titulo.Ancho = min(max(libre-chip, tema.Espacio(24)), 0.4*t.Width)
		chip = max(min(chip, libre-r.c.titulo.Ancho), 0)
	}
	x = centrar(r.c.titulo, x)

	r.punto.Move(fyne.NewPos(x, (t.Height-tema.Borde-LadoPuntoGuardado)/2))
	r.punto.Resize(fyne.NewSize(LadoPuntoGuardado, LadoPuntoGuardado))
	r.estado.Text = chipTexto
	if cabe := chip - LadoPuntoGuardado - HuecoChip; cabe > 0 {
		ui.Truncar(r.estado, chipTexto, tema.TextoXS, ui.Normal, cabe)
	}
	ui.ColocarTexto(r.estado, tema.TextoXS, x+LadoPuntoGuardado+HuecoChip,
		(t.Height-tema.Borde-tema.TextoXS.Linea)/2)

	// `flex-1` y luego los dos botones pegados a la derecha; en un móvil, «Compartir»
	// detrás del chip, y «Descargar» fuera.
	der := t.Width - MargenCabecera
	botones := []*ui.Boton{r.c.descargar, r.c.compartir}
	if !ancha {
		r.c.descargar.Hide()
		botones = botones[1:]
		der = min(der, x+chip+HuecoCabecera+r.c.compartir.MinSize().Width)
	} else {
		r.c.descargar.Show()
	}
	for _, b := range botones {
		m := b.MinSize()
		der -= m.Width
		b.Move(fyne.NewPos(der, (t.Height-tema.Borde-m.Height)/2))
		b.Resize(m)
		der -= HuecoCabecera
	}
}

func (r *cabeceraRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	texto, token := r.c.textoChip(time.Now())
	r.fondo.FillColor = tema.Color("surface", v)
	r.linea.FillColor = tema.Color("border", v)
	r.punto.FillColor = tema.Color(token, v)
	r.estado.Text = texto
	r.estado.Color = tema.Color("text-muted", v)
	r.c.filete.FillColor = tema.Color("border", v)
	if s := r.c.Size(); s.Width > 0 {
		r.Layout(s) // el chip cambia de texto con el reloj, y en un móvil se recorta
	}
	for _, o := range []fyne.CanvasObject{r.fondo, r.linea, r.punto, r.estado, r.c.filete} {
		o.Refresh()
	}
}

var _ = color.NRGBA{}

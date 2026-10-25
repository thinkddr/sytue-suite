// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La columna de la lista (`MessageList.tsx`), medida en producción a 1280×800:
//
//	panel      280 de ancho (`md:w-80`) con su filete a la derecha → 279 de interior
//	cabecera   42 (`min-h-12`), título `sytue-h4` (15,75/22,75) y el conteo a la
//	           derecha en `text-xs`; su `border-b` va DENTRO de los 42
//	búsqueda   `px-4 py-2.5` y un campo de 184 × 31,5 → la franja mide 50
//	lista      empieza en y = 148 (56 + 42 + 50) y cada fila mide 83
//
// El campo de búsqueda mide 184 y NO el ancho del panel: es el tamaño intrínseco de
// un `<input>` (el `size` por defecto), porque el `flex-1` de la web va en el
// envoltorio y no llega al control. Se copia como está: la referencia es lo que la
// web PINTA, no lo que parecía que iba a pintar.
const (
	// MargenBusqueda es el `px-4` de la franja de búsqueda y RellenoBusqueda su
	// `py-2.5`.
	MargenBusqueda  float32 = 4 * 0.25 * tema.Rem
	RellenoBusqueda float32 = 2.5 * 0.25 * tema.Rem
	// AnchoBusqueda es el ancho MEDIDO del campo (184).
	AnchoBusqueda float32 = 184
)

// AltoBusqueda es lo que ocupa la franja con su filete: 50, medido.
var AltoBusqueda = 2*RellenoBusqueda + ui.AltoCampo(ui.CampoMD) + tema.Borde

// AltoCabeceraLista es el `min-h-12` de SectionHeader: 42, medido.
var AltoCabeceraLista float32 = ui.MinCabeceraApp

// FilasEsqueleto son las ocho filas de carga que pinta la web.
const FilasEsqueleto = 8

// AltoEsqueleto es el alto de una fila del esqueleto: `h-[83px]`, la MISMA
// geometría que la fila real para que el layout no salte al llegar los datos.
//
// Fue 83 hasta que la web lo corrigió (era la cuenta de la fila con el rem de 16), y
// aquí se había copiado el 83 tal cual. Lo ata `TestElEsqueletoMideLaFila` contra el
// literal de `MessageList.tsx`, para que no vuelva a quedarse atrás.
const AltoEsqueleto float32 = 83

// Bandeja es la columna del centro: cabecera, búsqueda y la lista de conversaciones
// en sus seis estados.
type Bandeja struct {
	widget.BaseWidget

	// T y Locale son de la app: un widget no negocia idioma.
	T      *i18n.Translator
	Locale i18n.Locale
	// Ahora es el reloj con el que se escriben las fechas relativas. Se inyecta para
	// que el guardián no dependa de la hora a la que corra.
	Ahora func() time.Time

	// Titulo es el nombre de la vista: la carpeta, la etiqueta o «Resultados».
	Titulo string
	// Conversaciones es lo que se pinta cuando Estado es Listo.
	Conversaciones []Conversacion
	Estado         Estado
	// Buscando dice que lo de arriba son resultados y no una carpeta: cambia el
	// conteo y el texto del vacío.
	Buscando bool
	// Consulta es lo escrito en el campo de búsqueda.
	Consulta string
	// AbiertaID es el hilo que el lector está enseñando.
	AbiertaID string
	// Marcadas son los hilos con la casilla puesta.
	Marcadas map[string]bool
	// SoloLectura esconde las casillas y el triaje: un buzón compartido o sin red no
	// tiene acciones que confirmar en el servidor.
	SoloLectura bool
	// Acciones son las del triaje de cada fila (las monta el Buzón según la vista), y
	// Etiquetas el registro con el que se pintan sus chips.
	Acciones  *Acciones
	Etiquetas []Etiqueta
	// Lote son las acciones de la barra de selección.
	Lote *Lote
	// ConVaciar pone «Vaciar carpeta» en la cabecera (Papelera y Spam con algo dentro),
	// y AlVaciar lo atiende.
	ConVaciar bool
	AlVaciar  func()
	// HayMas dice que quedan páginas: al acercarse al final se pide la siguiente
	// (el centinela de la web). CargandoMas es esa página en camino.
	HayMas, CargandoMas bool

	AlElegir func(Conversacion)
	// AlMarcar es la casilla de una fila: su índice en el orden visible, que es con lo
	// que se arma el rango de Mayúsculas + clic.
	AlMarcar     func(c Conversacion, indice int)
	AlBuscar     func(string)
	AlRedactar   func()
	AlReintentar func()
	r            *bandejaRenderer

	// AlFinal pide la página siguiente. Se llama FUERA del reciclado de la lista
	// (`fyne.Do`): repintar la bandeja desde dentro de su propio `UpdateItem` es
	// reentrar en el layout que la está pintando.
	AlFinal func()
	// Refresco shares the mailbox request lifecycle with pull-to-refresh.
	Refresco ui.RefrescoLista
}

var _ fyne.Widget = (*Bandeja)(nil)

// NuevaBandeja crea la columna con su traductor.
func NuevaBandeja(t *i18n.Translator, l i18n.Locale) *Bandeja {
	b := &Bandeja{T: t, Locale: l, Ahora: time.Now, Marcadas: map[string]bool{}, Lote: &Lote{}}
	b.ExtendBaseWidget(b)
	return b
}

// AccessibilityLabel es el `aria-label` de la `<section>`.
func (b *Bandeja) AccessibilityLabel() string {
	return b.T.T("list.region", i18n.Vars{"folder": b.nombre()})
}

// AccessibilityRole implementa fyne.Accessible.
func (b *Bandeja) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// nombre es el título con el respaldo de la web cuando todavía no hay carpeta.
func (b *Bandeja) nombre() string {
	if b.Titulo != "" {
		return b.Titulo
	}
	return b.T.T("list.fallbackName", nil)
}

// EnfocarBusqueda es el atajo «/»: el cursor al campo de búsqueda.
func (b *Bandeja) EnfocarBusqueda() {
	if b.r == nil {
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(b); c != nil {
		c.Focus(b.r.busqueda)
	}
}

// VerFila trae a la vista la fila i: bajar con j más allá de lo visible no puede ser
// navegar a ciegas.
func (b *Bandeja) VerFila(i int) {
	if b.r != nil && i >= 0 && i < len(b.Conversaciones) {
		b.r.lista.ScrollTo(i)
	}
}

// hayMarcadas dice si alguna conversación VISIBLE tiene la casilla puesta: las marcas
// de hilos que ya no están en la lista (movidos, borrados) no cuentan.
func (b *Bandeja) hayMarcadas() bool { return b.CuantasMarcadas() > 0 }

// CuantasMarcadas es `selectedCount`: las marcadas de entre las visibles.
func (b *Bandeja) CuantasMarcadas() int {
	n := 0
	for _, c := range b.Conversaciones {
		if b.Marcadas[c.HiloID] {
			n++
		}
	}
	return n
}

// encabezado es el h1 de la vista: «Resultados» en búsqueda y la carpeta si no.
func (b *Bandeja) encabezado() string {
	if b.Buscando {
		return b.T.T("list.searchResults", nil)
	}
	return b.nombre()
}

// conteo es el texto de la derecha de la cabecera: resultados, no leídos o total, en
// ese orden de prioridad, que es el de la web.
func (b *Bandeja) conteo() string {
	n := len(b.Conversaciones)
	if b.Buscando {
		return b.T.T("list.resultCount", i18n.Vars{"count": n})
	}
	sinLeer := 0
	for _, c := range b.Conversaciones {
		if c.SinLeer {
			sinLeer++
		}
	}
	if sinLeer > 0 {
		return b.T.T("list.unreadCount", i18n.Vars{"count": sinLeer})
	}
	return b.T.T("list.conversationCount", i18n.Vars{"count": n})
}

// CreateRenderer compone cabecera, búsqueda y cuerpo.
func (b *Bandeja) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &bandejaRenderer{
		b:             b,
		fondo:         canvas.NewRectangle(tema.Color("surface", v)),
		filete:        canvas.NewRectangle(tema.Color("border", v)),
		lineaBusqueda: canvas.NewRectangle(tema.Color("border", v)),
		conteo:        ui.NuevoRotulo("", tema.TextoXS, ui.Normal, "text-subtle"),
	}
	r.vaciar = nuevoBotonVaciar(b.T.T("purge.emptyFolder", nil), func() {
		if b.AlVaciar != nil {
			b.AlVaciar()
		}
	})
	r.cabecera = ui.NuevaCabeceraApp(b.encabezado(), r.conteo)
	r.busqueda = ui.NuevaEntrada(b.T.T("list.searchPlaceholder", nil))
	r.busqueda.Icono = iconos.Buscar
	r.busqueda.Ancho = AnchoBusqueda
	r.busqueda.AlCambiar(func(s string) {
		if b.AlBuscar != nil {
			b.AlBuscar(s)
		}
	})
	// «Limpiar búsqueda»: un `Button` fantasma `sm` con el aspa de 16 como hijo (no
	// `iconOnly`: lleva su `px-3`), que solo está con algo escrito.
	r.limpiar = ui.NuevoBoton("", func() {
		r.busqueda.PonerTexto("")
		if b.AlBuscar != nil {
			b.AlBuscar("")
		}
	}).Con(ui.Fantasma, ui.TallaSM)
	r.limpiar.Rotulo, r.limpiar.Icono, r.limpiar.TamIcono = b.T.T("list.clearSearch", nil), iconos.Cerrar, 16
	r.seleccion = nuevaBarraSeleccion(b.T, b.Lote)

	var lista *widget.List
	lista = widget.NewList(
		func() int { return len(b.Conversaciones) },
		func() fyne.CanvasObject {
			f := NuevaFilaConversacion()
			// La rueda vertical sobre la tira de triaje sigue siendo de la lista.
			f.AlDesplazarV = func(e *fyne.ScrollEvent) { lista.ScrollToOffset(lista.GetScrollOffset() - e.Scrolled.DY) }
			return f
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			f, ok := o.(*FilaConversacion)
			if !ok || i < 0 || i >= len(b.Conversaciones) {
				return
			}
			b.rellenar(f, i, b.Conversaciones[i])
			if b.HayMas && !b.CargandoMas && b.AlFinal != nil && i >= len(b.Conversaciones)-casiAlFinal {
				fyne.Do(b.AlFinal)
			}
		},
	)
	r.lista = lista
	r.cuerpo = ui.EnvolverLista(lista)

	r.esqueleto = nuevoEsqueleto()
	r.vacio = ui.NuevoVacio("", "")
	b.r = r
	r.objetos = []fyne.CanvasObject{
		r.fondo, r.cabecera, r.busqueda, r.limpiar, r.lineaBusqueda, r.seleccion,
		r.cuerpo, r.esqueleto, r.vacio, r.filete,
	}
	r.Refresh()
	return r
}

// rellenar es el `UpdateItem`: pone en la fila reciclada todo lo que depende del
// idioma, que es de la app y no del widget.
func (b *Bandeja) rellenar(f *FilaConversacion, i int, c Conversacion) {
	asunto := c.Ultimo.Asunto
	if asunto == "" {
		asunto = b.T.T("common.noSubject", nil)
	}
	f.ConCasilla = !b.SoloLectura
	f.Abierta = c.HiloID == b.AbiertaID
	f.Marcada = b.Marcadas[c.HiloID]
	f.SeleccionActiva = !b.SoloLectura && b.hayMarcadas()
	f.SinLeerTexto = b.T.T("list.unreadPrefix", nil)
	f.T = b.T
	f.Acciones = nil
	if !b.SoloLectura {
		f.Acciones = b.Acciones
	}
	f.Chips = EtiquetasDe(c, b.Etiquetas)
	f.AlAbrir = func() {
		if b.AlElegir != nil {
			b.AlElegir(c)
		}
	}
	f.AlMarcar = func() {
		if b.AlMarcar != nil {
			b.AlMarcar(c, i)
		}
	}
	f.Poner(c, NombreVisible(b.T, c.Ultimo.De), asunto,
		FechaCorta(b.Locale, c.Ultimo.Recibido, b.Ahora()))
}

type bandejaRenderer struct {
	b                            *Bandeja
	fondo, filete, lineaBusqueda *canvas.Rectangle
	cabecera                     *ui.CabeceraApp
	conteo                       *ui.Rotulo
	busqueda                     *ui.Entrada
	limpiar                      *ui.Boton
	vaciar                       *botonVaciar
	seleccion                    *BarraSeleccion
	lista                        *widget.List
	cuerpo                       fyne.CanvasObject
	esqueleto                    *Esqueleto
	vacio                        *ui.Vacio
	objetos                      []fyne.CanvasObject
	// altos son las filas a las que se ha dado un alto propio (las que llevan chips),
	// para devolverlas a los 83 cuando dejan de llevarlos.
	altos map[int]float32
}

func (r *bandejaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(ui.AnchoPanelBase, AltoCabeceraLista+AltoBusqueda)
}

func (r *bandejaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *bandejaRenderer) Destroy()                     {}

func (r *bandejaRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	// El `border-e` del envoltorio: el panel mide 280 y la sección de dentro, 279.
	r.filete.Move(fyne.NewPos(t.Width-tema.Borde, 0))
	r.filete.Resize(fyne.NewSize(tema.Borde, t.Height))
	dentro := t.Width - tema.Borde

	r.cabecera.Move(fyne.NewPos(0, 0))
	r.cabecera.Resize(fyne.NewSize(dentro, AltoCabeceraLista))
	y := AltoCabeceraLista

	r.busqueda.Move(fyne.NewPos(MargenBusqueda, y+RellenoBusqueda))
	r.busqueda.Resize(fyne.NewSize(AnchoBusqueda, ui.AltoCampo(ui.CampoMD)))
	// `gap-2` detrás del campo, centrado en la franja (`items-center`).
	ml := r.limpiar.MinSize()
	r.limpiar.Move(fyne.NewPos(MargenBusqueda+AnchoBusqueda+tema.Espacio(2),
		y+(AltoBusqueda-tema.Borde-ml.Height)/2))
	r.limpiar.Resize(ml)
	r.lineaBusqueda.Move(fyne.NewPos(0, y+AltoBusqueda-tema.Borde))
	r.lineaBusqueda.Resize(fyne.NewSize(dentro, tema.Borde))
	y += AltoBusqueda
	if r.seleccion.Visible() {
		r.seleccion.Move(fyne.NewPos(0, y))
		r.seleccion.Resize(fyne.NewSize(dentro, AltoBarraSeleccion))
		y += AltoBarraSeleccion
	}

	alto := max(t.Height-y, 0)
	r.ajustarAltos(dentro)
	for _, o := range []fyne.CanvasObject{r.cuerpo, r.esqueleto} {
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(dentro, alto))
	}
	centrarVacio(r.vacio, fyne.NewPos(0, y), fyne.NewSize(dentro, alto))
}

func (r *bandejaRenderer) Refresh() {
	b := r.b
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("surface", v)
	r.filete.FillColor = tema.Color("border", v)
	r.lineaBusqueda.FillColor = tema.Color("border", v)

	r.cabecera.Titulo = b.encabezado()
	// El conteo solo se pinta con datos: durante la carga no hay nada que contar, y
	// «0 conversaciones» mientras carga es información falsa.
	hayConteo := b.Estado == Listo && len(b.Conversaciones) > 0
	r.conteo.Texto = ""
	if hayConteo {
		r.conteo.Texto = b.conteo()
	}
	verlo(r.conteo, hayConteo)
	// «Vaciar carpeta» va junto al conteo, y solo donde borrar es definitivo.
	r.cabecera.Acciones = []fyne.CanvasObject{r.conteo}
	if hayConteo && b.ConVaciar && !b.Buscando {
		r.cabecera.Acciones = append(r.cabecera.Acciones, r.vaciar)
	}
	verlo(r.limpiar, b.Consulta != "")
	marcadas := b.CuantasMarcadas()
	r.seleccion.Cuantas, r.seleccion.Todas = marcadas, marcadas > 0 && marcadas == len(b.Conversaciones)
	verlo(r.seleccion, !b.SoloLectura && marcadas > 0 && b.Estado == Listo)

	verlo(r.esqueleto, b.Estado == Cargando)
	verlo(r.cuerpo, b.Estado == Listo)
	fin := b.Estado != Cargando && b.Estado != Listo
	verlo(r.vacio, fin)
	if fin {
		r.ponerVacio()
	}

	r.lista.Refresh()
	for _, o := range r.objetos {
		o.Refresh()
	}
	r.Layout(b.Size())
}

// ajustarAltos le da a cada fila con etiquetas el alto que le piden sus chips, que es
// lo único de la fila que no mide siempre 83. `widget.List` aplica a todas el
// mínimo de la plantilla, así que la excepción se declara fila a fila.
func (r *bandejaRenderer) ajustarAltos(ancho float32) {
	b := r.b
	if ancho <= 0 {
		return
	}
	if r.altos == nil {
		r.altos = map[int]float32{}
	}
	vistas := map[int]bool{}
	for i, c := range b.Conversaciones {
		chips := EtiquetasDe(c, b.Etiquetas)
		if len(chips) == 0 {
			continue
		}
		fecha := FechaCorta(b.Locale, c.Ultimo.Recibido, b.Ahora())
		alto := AltoFilaCon(chips, AnchoTextoFila(ancho, fecha, c.ConAdjunto, c.SinLeer))
		vistas[i] = true
		if r.altos[i] != alto {
			r.altos[i] = alto
			r.lista.SetItemHeight(i, alto)
		}
	}
	for i := range r.altos {
		if !vistas[i] {
			delete(r.altos, i)
			r.lista.SetItemHeight(i, AltoFilaConversacion())
		}
	}
}

// ponerVacio escribe el estado que toca. Son CUATRO textos distintos y no uno: «no
// hay correos», «no hay resultados para “…”», «no he podido preguntar» y «no tienes
// permiso» dicen cosas distintas, y enseñar el primero cuando pasó el tercero es lo
// que hace que alguien deje de buscar un correo que sí existe.
func (r *bandejaRenderer) ponerVacio() {
	b, t := r.b, r.b.T
	var titulo, desc string
	var accion fyne.CanvasObject
	switch b.Estado {
	case Error:
		titulo = t.T("list.errorTitle", nil)
		desc = t.T("list.errorDescription", nil)
		accion = ui.NuevoBoton(t.T("common.retry", nil), b.AlReintentar).
			Con(ui.Secundario, ui.TallaSM)
	case SinConexion:
		// La web lo trae de `@sytue/shell`, que pinta el aviso de «sin conexión» en la
		// barra. Aquí no hay barra (es la exclusión de paridad escrita), así que el
		// aviso vive donde se busca el correo.
		titulo = t.T("offline.status", nil)
		desc = t.T("offline.description", nil)
	case SinPermiso:
		titulo = t.T("forbidden.title", nil)
		desc = t.T("forbidden.description", nil)
	case Vacio:
		if b.Buscando {
			titulo = t.T("list.emptySearchTitle", i18n.Vars{"query": b.Consulta})
			desc = t.T("list.emptySearchDescription", nil)
			break
		}
		titulo = t.T("list.emptyTitle", i18n.Vars{"folder": b.nombre()})
		desc = t.T("list.emptyDescription", nil)
		if !b.SoloLectura {
			boton := ui.NuevoBoton(t.T("nav.compose", nil), b.AlRedactar).Con(ui.Primario, ui.TallaSM)
			boton.Icono = iconos.Mas // `leadingIcon={<Icon name="plus" />}`
			accion = boton
		}
	case Cargando, Listo:
	}
	if nuevo := ponerEstado(r.vacio, titulo, desc, accion); nuevo != r.vacio {
		cambiar(r.objetos, r.vacio, nuevo)
		r.vacio = nuevo
	}
	// ponytail: sin ilustración. Las trece de `branding/illustrations` usan
	// `var(--sytue-primary-500)` SIN valor de reserva y eso tumba el fichero entero en
	// `oksvg`: es uno de los huecos medidos del catálogo (`docs/roadmap.md`). En
	// cuanto se sustituya el token por su hex al leer, es una línea aquí.
}

// Esqueleto son las ocho filas grises de la carga, con la geometría de la fila real.
type Esqueleto struct {
	widget.BaseWidget
}

var _ fyne.Widget = (*Esqueleto)(nil)

func nuevoEsqueleto() *Esqueleto {
	e := &Esqueleto{}
	e.ExtendBaseWidget(e)
	return e
}

// AccessibilityLabel: la web lo marca `aria-hidden` y anuncia «Cargando…» aparte. Lo
// anunciable es el estado, no los rectángulos.
func (e *Esqueleto) AccessibilityLabel() string { return "" }

// AccessibilityRole implementa fyne.Accessible.
func (e *Esqueleto) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// CreateRenderer compone las ocho filas.
func (e *Esqueleto) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &esqueletoRenderer{e: e}
	for range FilasEsqueleto {
		f := filaEsqueleto{
			filete: canvas.NewRectangle(tema.Color("border", v)),
			avatar: ui.NuevoEsqueletoRedondo(ui.MedidaAvatar(ui.AvatarMD).Width),
			l1:     ui.NuevoEsqueleto(0, 0),
			l2:     ui.NuevoEsqueleto(0, 0),
			l3:     ui.NuevoEsqueleto(0, 0),
			meta:   ui.NuevoEsqueleto(0, 0),
		}
		r.filas = append(r.filas, f)
		r.objetos = append(r.objetos, f.filete, f.avatar, f.l1, f.l2, f.l3, f.meta)
	}
	return r
}

type filaEsqueleto struct {
	filete           *canvas.Rectangle
	avatar           *ui.Esqueleto
	l1, l2, l3, meta *ui.Esqueleto
}

type esqueletoRenderer struct {
	e       *Esqueleto
	filas   []filaEsqueleto
	objetos []fyne.CanvasObject
}

func (r *esqueletoRenderer) MinSize() fyne.Size {
	return fyne.NewSize(ui.AnchoPanelBase, FilasEsqueleto*AltoEsqueleto)
}

func (r *esqueletoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *esqueletoRenderer) Destroy()                     {}

// Layout copia el marcado de la web: `h-[83px] gap-3 px-3` con el punto de 7, el
// avatar de 40 y tres barras de 14/12/12 en la columna de texto. Todo se centra en la
// caja de CONTENIDO, que es la fila menos su `border-b` (`box-sizing: border-box`).
func (r *esqueletoRenderer) Layout(t fyne.Size) {
	const (
		altoL1    float32 = 14
		altoL2    float32 = 12
		aire      float32 = 2 * 0.25 * tema.Rem // el `mt-2` entre barras
		anchoMeta float32 = 36
	)
	contenido := AltoEsqueleto - tema.Borde
	for i, f := range r.filas {
		y := float32(i) * AltoEsqueleto
		f.filete.Move(fyne.NewPos(0, y+AltoEsqueleto-tema.Borde))
		f.filete.Resize(fyne.NewSize(t.Width, tema.Borde))

		x := ui.InicioFila() + PuntoSinLeer + HuecoDelantero
		lado := ui.MedidaAvatar(ui.AvatarMD)
		f.avatar.Move(fyne.NewPos(x, y+(contenido-lado.Height)/2))
		f.avatar.Resize(lado)
		x += lado.Width + HuecoBloque

		derecha := t.Width - MargenFila
		f.meta.Move(fyne.NewPos(derecha-anchoMeta, y+(contenido-altoL2)/2))
		f.meta.Resize(fyne.NewSize(anchoMeta, altoL2))
		derecha -= anchoMeta + HuecoBloque

		util := max(derecha-x, 0)
		bloque := altoL1 + aire + altoL2 + aire + altoL2
		by := y + (contenido-bloque)/2
		for _, b := range []struct {
			r     *ui.Esqueleto
			parte float32
			alto  float32
		}{{f.l1, 0.45, altoL1}, {f.l2, 0.80, altoL2}, {f.l3, 0.65, altoL2}} {
			b.r.Move(fyne.NewPos(x, by))
			b.r.Resize(fyne.NewSize(util*b.parte, b.alto))
			by += b.alto + aire
		}
	}
}

func (r *esqueletoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// Los bloques se recolorean solos: `ui.Esqueleto` lee el tema en su Refresh. Aquí
	// solo queda el filete, que no es esqueleto sino el `divide-y` de la lista.
	for _, f := range r.filas {
		f.filete.FillColor = tema.Color("border", v)
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
}

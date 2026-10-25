// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image/color"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Contrapartida de MessageListPanel.tsx, SearchPanel.tsx, ThreadsPanel.tsx,
// DraftsPanel.tsx y ScheduledPanel.tsx dentro de la pantalla de canal declarada en
// pantalla.go.
//
// Los paneles de la derecha que son LISTAS DE MENSAJES: «Fijados» (del canal),
// «Guardados» (de quien mira, de todos los canales) y «Buscar». Comparten la banda del
// hilo (`CabeceraPanel`), el hueco de `--rooms-rhs-width` y la fila: `flex gap-3 px-4
// py-2` con el avatar, el nombre, la fecha y hora y el texto, que es un BOTÓN que salta
// al mensaje en su canal. La búsqueda añade su caja (`p-3 border-b`) y la línea de
// estado (`px-4 py-1.5 text-xs`); las otras dos, las acciones de la fila.
//
// «Hilos» (los que sigues, de todos los canales) es el cuarto: la misma hoja con otra
// fila —el #canal y la última actividad a la derecha, el texto en dos líneas y «N
// respuestas» con las no leídas— y «Cargar más» al final. «Borradores» es el quinto:
// lo escrito y no enviado, cada uno un botón que lleva a donde se escribía y su
// «Descartar» fuera de él.

// Las medidas de la HOJA.
var (
	// MargenCajaBusqueda es el `p-3` de la caja de buscar.
	MargenCajaBusqueda = tema.Espacio(3)
	// AltoEstadoBusqueda es la línea de estado: `py-1.5 text-xs`, 5,25 + 14 + 5,25.
	AltoEstadoBusqueda = 2*tema.Espacio(1.5) + tema.TextoXS.Linea
	// AireVacioLista es el `py-6` del texto de lista vacía.
	AireVacioLista = tema.Espacio(6)
	// LadoAccionFila es el `h-7 w-7` de «copiar el enlace» y «quitar».
	LadoAccionFila = tema.Espacio(7)
)

// MinConsulta es `MIN_QUERY`: con menos no se busca.
const MinConsulta = 2

// EsperaBusqueda es el `setTimeout(…, 250)` de la caja de buscar.
const EsperaBusqueda = 250 * time.Millisecond

// PanelLista es uno de los tres paneles.
type PanelLista struct {
	widget.BaseWidget

	T      *i18n.Translator
	Locale i18n.Locale

	Titulo, Subtitulo string
	// RotuloCerrar es el nombre del aspa: «Cerrar Fijados», «Cerrar la búsqueda».
	RotuloCerrar string
	// Busqueda enseña la caja de buscar y la línea de estado.
	Busqueda bool
	// Estado es Cargando, Listo o Error.
	Estado   Estado
	Mensajes []Mensaje
	// Canales es id → nombre, para el «#canal» de cada fila (guardados y búsqueda).
	Canales map[string]string
	// Vacio es el texto de la lista vacía (`panel.pinnedEmpty`…).
	Vacio string
	// RotuloQuitar es el nombre de la acción de retirar («Desfijar»); vacío, no hay.
	RotuloQuitar string

	AlElegir     func(Mensaje)
	AlQuitar     func(Mensaje)
	AlCopiar     func(Mensaje)
	AlCerrar     func()
	AlReintentar func()
	// AlBuscar recibe la consulta en cada pulsación; quien la lance hace la espera.
	AlBuscar func(string)

	// DeHilos lo hace el panel «Hilos»: pinta Hilos y no Mensajes.
	DeHilos bool
	Hilos   []ResumenHilo
	// AlCargarMas pide la página siguiente; nil, no hay más.
	AlCargarMas func()
	CargandoMas bool

	// DeProgramados lo hace el panel «Programados»: pinta Programados.
	DeProgramados bool
	Programados   []Programado
	AlEnviarYa    func(Programado)
	AlCancelarYa  func(Programado)
	// AlReprogramar mueve un programado a otra hora (`onReschedule`).
	AlReprogramar func(Programado, time.Time)

	// DeBorradores lo hace el panel «Borradores»: pinta Borradores.
	DeBorradores  bool
	Borradores    []Borrador
	AlIrABorrador func(Borrador)
	AlDescartar   func(Borrador)

	caja     *ui.Entrada
	consulta string
	fondo    *canvas.Rectangle
	filete   *canvas.Rectangle
	sepCaja  *canvas.Rectangle
	cabecera *CabeceraPanel
	estado   *canvas.Text
	cuerpo   fyne.CanvasObject
	objetos  []fyne.CanvasObject
}

var _ fyne.Widget = (*PanelLista)(nil)

// NuevoPanelLista crea el panel.
func NuevoPanelLista(t *i18n.Translator, l i18n.Locale, titulo string) *PanelLista {
	p := &PanelLista{T: t, Locale: l, Titulo: titulo, Estado: Cargando}
	p.ExtendBaseWidget(p)
	return p
}

// AccessibilityLabel es el `aria-label` del `<aside>`.
func (p *PanelLista) AccessibilityLabel() string {
	switch {
	case p.Busqueda:
		return p.T.T("search.messages", nil)
	case p.DeBorradores:
		return p.T.T("drafts.aria", nil)
	case p.DeProgramados:
		return p.T.T("scheduled.aria", nil)
	}
	return p.Titulo
}

// AccessibilityRole implementa fyne.Accessible.
func (p *PanelLista) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// Consulta es lo escrito en la caja de buscar.
func (p *PanelLista) Consulta() string { return p.consulta }

// PonerConsulta escribe en la caja de buscar y busca, que es el `initialQuery` de la
// web («in:#canal has:file » de «Archivos del canal»).
func (p *PanelLista) PonerConsulta(q string) {
	if p.caja == nil {
		p.consulta = q
		if p.AlBuscar != nil {
			p.AlBuscar(q)
		}
		return
	}
	p.caja.PonerTexto(q) // dispara AlCambiar
}

// activa dice si hay consulta bastante para buscar.
func (p *PanelLista) activa() bool { return len([]rune(strings.TrimSpace(p.consulta))) >= MinConsulta }

// CreateRenderer monta el panel.
func (p *PanelLista) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p.fondo = canvas.NewRectangle(tema.Color("surface", v))
	p.filete = canvas.NewRectangle(tema.Color("border", v))
	p.sepCaja = canvas.NewRectangle(tema.Color("border", v))
	p.estado = ui.Texto("", tema.TextoXS, ui.Normal, tema.Color("text-muted", v))
	p.cabecera = NuevaCabeceraPanel(p.Titulo, p.Subtitulo, p.RotuloCerrar, p.AlCerrar, nil)
	if p.Busqueda {
		p.caja = ui.NuevaEntrada(p.T.T("search.placeholder", nil))
		p.caja.Icono = iconos.Buscar
		p.caja.AlCambiar(func(s string) {
			p.consulta = s
			if p.AlBuscar != nil {
				p.AlBuscar(s)
			}
		})
		p.caja.AlTeclear = func(k *fyne.KeyEvent) bool {
			if k.Name == fyne.KeyEscape && p.AlCerrar != nil {
				p.AlCerrar() // `Esc cierra`
				return true
			}
			return false
		}
	}
	r := &panelListaRenderer{p: p}
	r.Refresh()
	return r
}

// Enfocar pone el cursor en la caja de buscar (`autoFocus`).
func (p *PanelLista) Enfocar() {
	if c := fyne.CurrentApp().Driver().CanvasForObject(p); c != nil && p.caja != nil {
		c.Focus(p.caja)
	}
}

// textoEstado es la línea `role="status"` de la búsqueda.
func (p *PanelLista) textoEstado() string {
	t := p.T
	switch {
	case !p.activa():
		return ""
	case p.Estado == Cargando:
		return t.T("search.busy", nil)
	case p.Estado == Error:
		return t.T("search.failed", nil)
	}
	return t.T("search.resultCount", i18n.Vars{"count": len(p.Mensajes)})
}

// construirCuerpo son las filas o el estado que toque.
func (p *PanelLista) construirCuerpo(v tema.Variante) fyne.CanvasObject {
	t := p.T
	var cosas []fyne.CanvasObject
	if !p.Busqueda {
		cosas = append(cosas, hueco(ArribaCuerpoHilo)) // el `py-2` del cuerpo
	}
	switch {
	case p.DeHilos:
		cosas = append(cosas, p.cuerpoHilos(v)...)
	case p.DeProgramados:
		cosas = append(cosas, p.cuerpoProgramados(v)...)
	case p.DeBorradores && len(p.Borradores) == 0:
		vac := ui.NuevoVacio(t.T("drafts.empty", nil), "")
		vac.Ilustracion = iconos.Pintar(iconos.VacioChat, v).A(LadoIlustracionBusqueda).Objeto()
		cosas = append(cosas, vac)
	case p.DeBorradores:
		for i, b := range p.Borradores {
			f := NuevaFilaBorrador(t, b, p.Canales[b.Canal], p.AlIrABorrador, p.AlDescartar)
			f.Ultima = i == len(p.Borradores)-1
			cosas = append(cosas, f)
		}
	case p.Busqueda && !p.activa():
		cosas = append(cosas, NuevoTextoCentrado(t.T("search.hint", nil)))
	case p.Estado == Error && p.Busqueda:
		cosas = append(cosas, NuevoAvisoHilo(t.T("search.failedRetry", nil), "", nil))
	case p.Estado == Error:
		cosas = append(cosas, NuevoAvisoHilo(t.T("error.loadFailed", nil), t.T("action.retry", nil), p.AlReintentar))
	case p.Estado == Cargando:
		e := NuevoEsqueletoHilo(t.T("state.loading", nil))
		e.Partes = []float32{0.60, 0.45, 0.52}
		cosas = append(cosas, e)
	case len(p.Mensajes) == 0 && p.Busqueda:
		vac := ui.NuevoVacio(t.T("search.noResults", nil), "")
		vac.Ilustracion = iconos.Pintar(iconos.VacioBusqueda, v).A(LadoIlustracionBusqueda).Objeto()
		cosas = append(cosas, vac)
	case len(p.Mensajes) == 0:
		cosas = append(cosas, NuevoTextoCentrado(p.Vacio))
	default:
		for _, m := range p.Mensajes {
			cosas = append(cosas, p.fila(m))
		}
	}
	cosas = append(cosas, hueco(ArribaCuerpoHilo))
	return container.NewVScroll(container.New(&columnaFluida{ancho: AnchoPanelDerecho}, cosas...))
}

// medibleA es una fila cuyo alto depende del ancho: la que parte su texto en líneas.
type medibleA interface{ altoA(ancho float32) float32 }

// columnaFluida es ui.Columna para filas que se miden a un ancho: las coloca con el
// alto que tienen AL ANCHO DE LA COLUMNA. Con ui.Columna se medían con su último
// tamaño —cero la primera vez—, y en la captura de «Hilos» un texto de dos líneas
// tapaba su «1 respuesta» con «Cargar más». Recuerda el último ancho para medirse, y
// hasta el primero supone el del panel.
type columnaFluida struct{ ancho float32 }

func (c *columnaFluida) alto(o fyne.CanvasObject, ancho float32) float32 {
	if m, ok := o.(medibleA); ok && ancho > 0 {
		return m.altoA(ancho)
	}
	return o.MinSize().Height
}

func (c *columnaFluida) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for _, o := range objs {
		if o.Visible() {
			alto += c.alto(o, c.ancho)
			ancho = max(ancho, o.MinSize().Width)
		}
	}
	return fyne.NewSize(ancho, alto)
}

func (c *columnaFluida) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	c.ancho = t.Width
	var y float32
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		h := c.alto(o, t.Width)
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(t.Width, h))
		y += h
	}
}

// LadoIlustracionBusqueda es el `size={96}` de la ilustración de «Sin resultados».
var LadoIlustracionBusqueda float32 = 96

// cuerpoProgramados son los estados de «Programados»: los de la hoja, y cada
// programado con sus dos botones.
func (p *PanelLista) cuerpoProgramados(v tema.Variante) []fyne.CanvasObject {
	t := p.T
	switch {
	case p.Estado == Error:
		return []fyne.CanvasObject{NuevoAvisoHilo(t.T("error.loadFailed", nil), t.T("action.retry", nil), p.AlReintentar)}
	case p.Estado == Cargando:
		e := NuevoEsqueletoHilo(t.T("state.loading", nil))
		e.Partes = []float32{0.60, 0.45, 0.52}
		return []fyne.CanvasObject{e}
	case len(p.Programados) == 0:
		vac := ui.NuevoVacio(t.T("scheduled.empty", nil), "")
		vac.Ilustracion = iconos.Pintar(iconos.VacioChat, v).A(LadoIlustracionBusqueda).Objeto()
		return []fyne.CanvasObject{vac}
	}
	out := make([]fyne.CanvasObject, 0, len(p.Programados))
	for i, m := range p.Programados {
		f := NuevaFilaProgramado(t, p.Locale, m, p.Canales[m.Canal], p.AlEnviarYa, p.AlCancelarYa)
		f.AlReprogramar = p.AlReprogramar
		f.Ultima = i == len(p.Programados)-1
		out = append(out, f)
	}
	return out
}

// cuerpoHilos son los estados de «Hilos»: el aviso con «Reintentar», las tres filas de
// carga, el vacío con su ilustración o los hilos con «Cargar más».
func (p *PanelLista) cuerpoHilos(v tema.Variante) []fyne.CanvasObject {
	t := p.T
	switch {
	case p.Estado == Error:
		return []fyne.CanvasObject{NuevoAvisoHilo(t.T("threads.loadFailed", nil), t.T("action.retry", nil), p.AlReintentar)}
	case p.Estado == Cargando:
		e := NuevoEsqueletoHilo(t.T("threads.loading", nil))
		e.Partes = []float32{0.60, 0.45, 0.52}
		return []fyne.CanvasObject{e}
	case len(p.Hilos) == 0:
		vac := ui.NuevoVacio(t.T("threads.empty", nil), "")
		vac.Ilustracion = iconos.Pintar(iconos.VacioChat, v).A(LadoIlustracionBusqueda).Objeto()
		return []fyne.CanvasObject{vac}
	}
	out := make([]fyne.CanvasObject, 0, len(p.Hilos)+1)
	for i := range p.Hilos {
		f := NuevaFilaLista(t, p.Locale, p.Hilos[i].Raiz, p.Canales[p.Hilos[i].Raiz.CanalID])
		f.Resumen, f.AlElegir = &p.Hilos[i], p.AlElegir
		out = append(out, f)
	}
	if p.AlCargarMas != nil {
		b := ui.NuevoBoton(t.T("action.loadMore", nil), p.AlCargarMas).Con(ui.Secundario, ui.TallaSM)
		b.Cargando = p.CargandoMas
		lado := tema.Espacio(4) // `px-4 py-2`, y el botón a todo el ancho (`w-full`)
		out = append(out, hueco(tema.Espacio(2)),
			container.NewBorder(nil, nil, sangria(lado), sangria(lado), b), hueco(tema.Espacio(2)))
	}
	return out
}

func (p *PanelLista) fila(m Mensaje) *FilaLista {
	f := NuevaFilaLista(p.T, p.Locale, m, p.Canales[m.CanalID])
	f.ConCanal = p.Busqueda || p.Canales != nil
	f.AlElegir = p.AlElegir
	if !p.Busqueda {
		f.AlCopiar = p.AlCopiar
		if p.RotuloQuitar != "" {
			f.RotuloQuitar, f.AlQuitar = p.RotuloQuitar, p.AlQuitar
		}
	}
	return f
}

type panelListaRenderer struct{ p *PanelLista }

func (r *panelListaRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, AltoCabecera) }
func (r *panelListaRenderer) Objects() []fyne.CanvasObject { return r.p.objetos }
func (r *panelListaRenderer) Destroy()                     {}

func (r *panelListaRenderer) Layout(t fyne.Size) {
	p := r.p
	p.fondo.Resize(t)
	p.filete.Move(fyne.NewPos(0, 0))
	p.filete.Resize(fyne.NewSize(tema.Borde, t.Height))
	x, ancho := tema.Borde, max(t.Width-tema.Borde, 0)
	p.cabecera.Move(fyne.NewPos(x, 0))
	p.cabecera.Resize(fyne.NewSize(ancho, AltoCabecera))
	y := AltoCabecera
	if p.Busqueda {
		m := p.caja.MinSize()
		p.caja.Move(fyne.NewPos(x+MargenCajaBusqueda, y+MargenCajaBusqueda))
		p.caja.Resize(fyne.NewSize(max(ancho-2*MargenCajaBusqueda, 0), m.Height))
		y += 2*MargenCajaBusqueda + m.Height
		p.sepCaja.Move(fyne.NewPos(x, y))
		p.sepCaja.Resize(fyne.NewSize(ancho, tema.Borde))
		y += tema.Borde
		ui.ColocarTexto(p.estado, tema.TextoXS, x+CanalonFilaHilo, y+tema.Espacio(1.5))
		y += AltoEstadoBusqueda
	}
	p.cuerpo.Move(fyne.NewPos(x, y))
	p.cuerpo.Resize(fyne.NewSize(ancho, max(t.Height-y, 0)))
}

func (r *panelListaRenderer) Refresh() {
	p := r.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p.fondo.FillColor = tema.Color("surface", v)
	p.fondo.Refresh()
	for _, f := range []*canvas.Rectangle{p.filete, p.sepCaja} {
		f.FillColor = tema.Color("border", v)
		f.Refresh()
	}
	p.cabecera.Titulo, p.cabecera.Subtitulo = p.Titulo, p.Subtitulo
	p.cabecera.Refresh()
	p.cuerpo = p.construirCuerpo(v)
	p.objetos = []fyne.CanvasObject{p.fondo, p.cuerpo, p.cabecera, p.filete}
	if p.Busqueda {
		p.estado.Text = p.textoEstado()
		p.estado.Color = tema.Color("text-muted", v)
		p.estado.Refresh()
		p.objetos = append(p.objetos, p.caja, p.sepCaja, p.estado)
	}
	r.Layout(p.Size())
}

// ---- La fila de una lista ----

// FilaLista es un mensaje de un panel: `flex gap-3 px-4 py-2`, avatar `sm` en su `w-9
// pt-0.5`, la línea de nombre (`text-sm` semi, la fecha y la hora y el «#canal» en
// `.sytue-meta`) y el texto. Toda la fila salta al mensaje; «copiar el enlace» y
// «quitar» van a la derecha (`py-2 pr-2 gap-1`) y se ven al pasar por encima, y
// siempre con el dedo.
type FilaLista struct {
	widget.BaseWidget

	T       *i18n.Translator
	Locale  i18n.Locale
	Mensaje Mensaje
	Canal   string
	// ConCanal enseña el «#canal» (o «Canal» si no se sabe cuál).
	ConCanal bool

	RotuloQuitar string
	AlElegir     func(Mensaje)
	AlCopiar     func(Mensaje)
	AlQuitar     func(Mensaje)
	// Resumen la hace fila de «Hilos»: el Mensaje es su raíz.
	Resumen *ResumenHilo

	encima                bool
	fondo                 *canvas.Rectangle
	avatar                *ui.Avatar
	nombre, cuando, canal *canvas.Text
	cuerpo                []*canvas.Text
	respuestas            *canvas.Text
	sinLeer               *ui.Contador
	copiar, quitar        *ui.BotonIcono
	objetos               []fyne.CanvasObject
}

var (
	_ fyne.Widget       = (*FilaLista)(nil)
	_ fyne.Tappable     = (*FilaLista)(nil)
	_ desktop.Hoverable = (*FilaLista)(nil)
)

// NuevaFilaLista crea la fila.
func NuevaFilaLista(t *i18n.Translator, l i18n.Locale, m Mensaje, canal string) *FilaLista {
	f := &FilaLista{T: t, Locale: l, Mensaje: m, Canal: canal}
	f.ExtendBaseWidget(f)
	return f
}

// AccessibilityLabel es «Ir al mensaje de Ana», el `aria-label` del botón de la fila.
// La de un hilo no lleva `aria-label` en la web: se lee su contenido, el nombre y las
// respuestas.
func (f *FilaLista) AccessibilityLabel() string {
	if f.Resumen != nil {
		return f.Mensaje.Autor.Nombre + ", " + f.T.T("thread.replyCount", i18n.Vars{"count": f.Mensaje.Respuestas})
	}
	return f.T.T("message.goToFrom", i18n.Vars{"name": f.Mensaje.Autor.Nombre})
}

// AccessibilityRole implementa fyne.Accessible.
func (f *FilaLista) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// Tapped salta al mensaje.
func (f *FilaLista) Tapped(*fyne.PointEvent) {
	if f.AlElegir != nil {
		f.AlElegir(f.Mensaje)
	}
}

// MouseIn es el `hover:bg-bg-subtle` de la fila, que además enseña sus acciones.
func (f *FilaLista) MouseIn(*desktop.MouseEvent) { f.encima = true; f.Refresh() }

// MouseMoved no hace nada.
func (f *FilaLista) MouseMoved(*desktop.MouseEvent) {}

// MouseOut devuelve la fila al reposo.
func (f *FilaLista) MouseOut() { f.encima = false; f.Refresh() }

// RotuloCuando es la fecha y la hora de la fila: `formatDateTime(locale, iso, { day:
// 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })`.
//
// ponytail: se compone con la fecha corta sin año y la hora de la suite, unidas por
// «, »; Intl elige el enlace por idioma. Si un idioma lo escribe distinto, se genera
// su vector en `apps/nucleo/i18n` como los otros formatos.
func RotuloCuando(l i18n.Locale, t time.Time) string {
	return i18n.FormatDayMonth(l, t) + ", " + i18n.FormatTimeDigits(l, t)
}

// RotuloProgramado es `formatSchedule`, la ficha de «Se enviará»: lo mismo con el día
// de la semana corto delante («jue, 24 sept, 09:00»).
func RotuloProgramado(l i18n.Locale, t time.Time) string {
	return i18n.FormatWeekdayDayMonthShort(l, t) + ", " + i18n.FormatTimeDigits(l, t)
}

func (f *FilaLista) rotuloCanal() string {
	if f.Canal != "" {
		return "#" + f.Canal
	}
	return f.T.T("channel.word", nil)
}

// anchoAcciones es lo que ocupan las acciones de la derecha con su `pr-2`.
func (f *FilaLista) anchoAcciones() float32 {
	n := 0
	if f.AlCopiar != nil {
		n++
	}
	if f.AlQuitar != nil {
		n++
	}
	if n == 0 {
		return 0
	}
	return float32(n)*LadoAccionFila + float32(n-1)*tema.Espacio(1) + tema.Espacio(2)
}

// cabe es el ancho de la columna del texto.
func (f *FilaLista) cabe(ancho float32) float32 {
	return max(ancho-2*CanalonFilaHilo-ColumnaAvatar-HuecoFila-f.anchoAcciones(), 0)
}

func (f *FilaLista) lineas(ancho float32) []string {
	if f.Resumen != nil {
		return f.lineasResumen(ancho)
	}
	var out []string
	for _, par := range partirEnLineas(f.Mensaje.Cuerpo) {
		if par == "" {
			out = append(out, "")
			continue
		}
		out = append(out, ui.Repartir(par, tema.TextoBase, ui.Normal, f.cabe(ancho))...)
	}
	if f.Mensaje.Cuerpo == "" {
		return nil
	}
	return out
}

// lineasResumen es el `line-clamp-2` del texto de un hilo: dos líneas como mucho, y la
// segunda acaba en «…» si queda algo.
func (f *FilaLista) lineasResumen(ancho float32) []string {
	cuerpo := strings.Join(partirEnLineas(f.Mensaje.Cuerpo), " ")
	if strings.TrimSpace(cuerpo) == "" {
		return nil
	}
	todas := ui.Repartir(cuerpo, tema.TextoBase, ui.Normal, f.cabe(ancho))
	if len(todas) <= LineasResumenHilo {
		return todas
	}
	out := slices.Clone(todas[:LineasResumenHilo])
	resto := strings.Join(todas[LineasResumenHilo-1:], " ")
	c := canvas.NewText("", nil)
	ui.Truncar(c, resto+" ", tema.TextoBase, ui.Normal, f.cabe(ancho))
	out[LineasResumenHilo-1] = c.Text
	return out
}

// LineasResumenHilo es el `line-clamp-2` del texto de una fila de «Hilos».
const LineasResumenHilo = 2

// AltoPieResumen es la línea de «N respuestas» de un hilo: `mt-1` y la pastilla de
// 18, que es más alta que el texto de `.sytue-meta`.
var AltoPieResumen = tema.Espacio(1) + max(escalonMeta.Linea, ui.AltoContador)

// MedidaFilaLista es lo que ocupa a ese ancho. Es la única cuenta.
func MedidaFilaLista(f *FilaLista, ancho float32) fyne.Size {
	alto := 2*RellenoFilaHilo + tema.TextoSM.Linea + float32(len(f.lineas(ancho)))*tema.TextoBase.Linea
	if f.Resumen != nil {
		alto += AltoPieResumen
	}
	return fyne.NewSize(ancho, max(alto, 2*RellenoFilaHilo+ui.MedidaAvatar(ui.AvatarSM).Height))
}

// CreateRenderer compone la fila.
func (f *FilaLista) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t := f.T
	f.fondo = canvas.NewRectangle(color.NRGBA{})
	f.avatar = ui.NuevoAvatar(f.Mensaje.Autor.Nombre).Con(ui.AvatarSM)
	f.nombre = ui.Texto(f.Mensaje.Autor.Nombre, tema.TextoSM, ui.SemiNegrita, tema.Color("text", v))
	f.cuando = ui.Texto(RotuloCuando(f.Locale, f.Mensaje.Creado), escalonMeta, ui.Normal, tema.Color("text-subtle", v))
	f.canal = ui.Texto(f.rotuloCanal(), escalonMeta, ui.Normal, tema.Color("text-subtle", v))
	f.objetos = []fyne.CanvasObject{f.fondo, f.avatar, f.nombre, f.cuando}
	if f.ConCanal || (f.Resumen != nil && f.Canal != "") {
		f.objetos = append(f.objetos, f.canal)
	}
	if h := f.Resumen; h != nil {
		f.cuando.Text = RotuloCuando(f.Locale, h.Actividad)
		f.respuestas = ui.Texto(t.T("thread.replyCount", i18n.Vars{"count": f.Mensaje.Respuestas}),
			escalonMeta, ui.Normal, tema.Color("text-subtle", v))
		f.objetos = append(f.objetos, f.respuestas)
		if h.SinLeer > 0 {
			f.sinLeer = ui.NuevoContador(Contador(h.SinLeer))
			f.objetos = append(f.objetos, f.sinLeer)
		}
	}
	accion := func(rotulo string, al func()) *ui.BotonIcono {
		b := ui.NuevoBotonIcono(iconos.Ninguno, rotulo, al)
		b.Lado, b.TamIcono = LadoAccionFila, iconos.TallaSM
		b.Trazo, b.FondoEncima = "text-subtle", "surface"
		return b
	}
	if f.AlCopiar != nil {
		f.copiar = accion(t.T("message.copyLinkLong", nil), func() { f.AlCopiar(f.Mensaje) })
		f.copiar.Glifo = dibujo(glifoEnlace, iconos.TallaSM, "text-subtle", v)
		f.objetos = append(f.objetos, f.copiar)
	}
	if f.AlQuitar != nil {
		f.quitar = accion(f.RotuloQuitar, func() { f.AlQuitar(f.Mensaje) })
		f.quitar.Icono = iconos.Cerrar
		f.objetos = append(f.objetos, f.quitar)
	}
	r := &filaListaRenderer{f: f}
	r.Refresh()
	return r
}

type filaListaRenderer struct{ f *FilaLista }

func (r *filaListaRenderer) MinSize() fyne.Size {
	return MedidaFilaLista(r.f, r.f.Size().Width)
}
func (r *filaListaRenderer) Objects() []fyne.CanvasObject { return r.f.objetos }
func (r *filaListaRenderer) Destroy()                     {}

func (r *filaListaRenderer) Layout(t fyne.Size) {
	f := r.f
	f.fondo.Resize(t)
	y := RellenoFilaHilo
	f.avatar.Move(fyne.NewPos(CanalonFilaHilo, y+AireAvatar))
	f.avatar.Resize(ui.MedidaAvatar(ui.AvatarSM))
	x := CanalonFilaHilo + ColumnaAvatar + HuecoFila
	cabe := f.cabe(t.Width)
	base := y + tema.TextoSM.Linea - escalonMeta.Linea
	if f.Resumen != nil {
		r.layoutResumen(t, x, y, base, cabe)
		return
	}
	nx := x + ui.Truncar(f.nombre, f.Mensaje.Autor.Nombre, tema.TextoSM, ui.SemiNegrita, cabe) + HuecoNombre
	ui.ColocarTexto(f.nombre, tema.TextoSM, x, y)
	ui.ColocarTexto(f.cuando, escalonMeta, nx, base)
	nx += ui.Medir(f.cuando.Text, escalonMeta, ui.Normal).Width + HuecoNombre
	if f.ConCanal {
		ui.Truncar(f.canal, f.rotuloCanal(), escalonMeta, ui.Normal, max(x+cabe-nx, 0))
		ui.ColocarTexto(f.canal, escalonMeta, nx, base)
	}
	y += tema.TextoSM.Linea
	r.colocarCuerpo(x, y, t.Width, "text")
	// Las acciones, a la derecha y arriba (`items-start` con `py-2`).
	ax := t.Width - tema.Espacio(2)
	for _, b := range []*ui.BotonIcono{f.quitar, f.copiar} {
		if b == nil {
			continue
		}
		ax -= LadoAccionFila
		b.Move(fyne.NewPos(ax, RellenoFilaHilo))
		b.Resize(fyne.NewSize(LadoAccionFila, LadoAccionFila))
		ax -= tema.Espacio(1)
	}
}

// layoutResumen coloca la fila de un hilo: el nombre, el #canal y, a la derecha
// (`ml-auto`), la última actividad; el texto en `text-muted`; y debajo (`mt-1 gap-2`)
// «N respuestas» y la pastilla de las no leídas, centradas en su línea.
func (r *filaListaRenderer) layoutResumen(t fyne.Size, x, y, base, cabe float32) {
	f := r.f
	ancho := ui.Medir(f.cuando.Text, escalonMeta, ui.Normal).Width
	ui.ColocarTexto(f.cuando, escalonMeta, x+cabe-ancho, base)
	libre := max(cabe-ancho-HuecoNombre, 0)
	nx := x + ui.Truncar(f.nombre, f.Mensaje.Autor.Nombre, tema.TextoSM, ui.SemiNegrita, libre) + HuecoNombre
	ui.ColocarTexto(f.nombre, tema.TextoSM, x, y)
	if f.Canal != "" {
		ui.Truncar(f.canal, "#"+f.Canal, escalonMeta, ui.Normal, max(x+libre-nx, 0))
		ui.ColocarTexto(f.canal, escalonMeta, nx, base)
	}
	y = r.colocarCuerpo(x, y+tema.TextoSM.Linea, t.Width, "text-muted")
	y += tema.Espacio(1)
	fila := max(escalonMeta.Linea, ui.AltoContador)
	ui.ColocarTexto(f.respuestas, escalonMeta, x, y+(fila-escalonMeta.Linea)/2)
	if f.sinLeer != nil {
		px := x + ui.Medir(f.respuestas.Text, escalonMeta, ui.Normal).Width + tema.Espacio(2)
		m := f.sinLeer.MinSize()
		f.sinLeer.Move(fyne.NewPos(px, y+(fila-m.Height)/2))
		f.sinLeer.Resize(m)
	}
}

// colocarCuerpo pone las líneas del texto desde `y` con esa tinta y dice dónde acaban.
func (r *filaListaRenderer) colocarCuerpo(x, y, ancho float32, token string) float32 {
	f := r.f
	lineas := f.lineas(ancho)
	tinta := tema.Color(token, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	for i, s := range lineas {
		if i >= len(f.cuerpo) {
			c := ui.Texto(s, tema.TextoBase, ui.Normal, tinta)
			f.cuerpo = append(f.cuerpo, c)
			f.objetos = append(f.objetos, c)
		}
		f.cuerpo[i].Text = s
		f.cuerpo[i].Color = tinta
		f.cuerpo[i].Show()
		ui.ColocarTexto(f.cuerpo[i], tema.TextoBase, x, y)
		y += tema.TextoBase.Linea
	}
	for i := len(lineas); i < len(f.cuerpo); i++ {
		f.cuerpo[i].Hide()
	}
	return y
}

func (r *filaListaRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo.FillColor = color.NRGBA{}
	if f.encima {
		f.fondo.FillColor = tema.Color("bg-subtle", v)
	}
	f.fondo.Refresh()
	// `opacity-0 group-hover:opacity-100 pointer-coarse:opacity-100`.
	ver := f.encima || ui.EsTactil()
	for _, b := range []*ui.BotonIcono{f.copiar, f.quitar} {
		if b == nil {
			continue
		}
		if ver {
			b.Show()
		} else {
			b.Hide()
		}
	}
	r.Layout(f.Size())
}

// ---- Un texto centrado ----

// TextoCentrado es el `<p class="text-text-muted px-4 py-6 text-center text-sm">` de
// una lista vacía o de la pista de la búsqueda.
type TextoCentrado struct {
	widget.BaseWidget
	Texto string

	lineas []*canvas.Text
}

// NuevoTextoCentrado crea el texto.
func NuevoTextoCentrado(texto string) *TextoCentrado {
	c := &TextoCentrado{Texto: texto}
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityLabel es el texto.
func (c *TextoCentrado) AccessibilityLabel() string { return c.Texto }

// AccessibilityRole implementa fyne.Accessible.
func (c *TextoCentrado) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

func (c *TextoCentrado) partes(ancho float32) []string {
	return ui.Repartir(c.Texto, tema.TextoSM, ui.Normal, max(ancho-2*CanalonFilaHilo, 0))
}

// CreateRenderer compone las líneas.
func (c *TextoCentrado) CreateRenderer() fyne.WidgetRenderer { return &textoCentradoRenderer{c: c} }

type textoCentradoRenderer struct{ c *TextoCentrado }

func (r *textoCentradoRenderer) MinSize() fyne.Size {
	n := max(len(r.c.partes(r.c.Size().Width)), 1)
	return fyne.NewSize(0, 2*AireVacioLista+float32(n)*tema.TextoSM.Linea)
}

func (r *textoCentradoRenderer) Objects() []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, len(r.c.lineas))
	for i, l := range r.c.lineas {
		out[i] = l
	}
	return out
}
func (r *textoCentradoRenderer) Destroy() {}

func (r *textoCentradoRenderer) Layout(t fyne.Size) {
	c := r.c
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	partes := c.partes(t.Width)
	for len(c.lineas) < len(partes) {
		c.lineas = append(c.lineas, ui.Texto("", tema.TextoSM, ui.Normal, tema.Color("text-muted", v)))
	}
	y := AireVacioLista
	for i, l := range c.lineas {
		if i >= len(partes) {
			l.Hide()
			continue
		}
		l.Text = partes[i]
		l.Color = tema.Color("text-muted", v)
		l.Show()
		ancho := ui.Medir(partes[i], tema.TextoSM, ui.Normal).Width
		ui.ColocarTexto(l, tema.TextoSM, (t.Width-ancho)/2, y)
		y += tema.TextoSM.Linea
	}
}

func (r *textoCentradoRenderer) Refresh() { r.Layout(r.c.Size()) }

// ---- «Ir a mensajes recientes» ----

// Medidas de la barra (ChatApp.tsx): `border-t bg-bg-subtle px-4 py-1.5` con un botón
// `w-full justify-center gap-1.5 rounded-md py-1 text-sm font-medium
// text-primary-text hover:bg-selected` y la flecha de 14. HOJA.
var (
	RellenoBarraRecientes = tema.Espacio(1.5)
	RellenoBotonRecientes = tema.Espacio(1)
	GlifoRecientes        = iconos.TallaXS
)

// glifoAbajo es la flecha de «Ir a mensajes recientes», SVG en línea en ChatApp.tsx.
const glifoAbajo = `<path d="M12 5v14M6 13l6 6 6-6" />`

// MedidaBarraRecientes es lo que ocupa: 1 + 5,25 + (3,5 + 15,75 + 3,5) + 5,25.
func MedidaBarraRecientes() fyne.Size {
	return fyne.NewSize(0, tema.Borde+2*RellenoBarraRecientes+2*RellenoBotonRecientes+tema.TextoSM.Linea)
}

// BarraRecientes es la barra que devuelve al final del canal.
type BarraRecientes struct {
	ui.Pulsable
	Rotulo string

	filete, fondo *canvas.Rectangle
	caja          *ui.Caja
	aro           *ui.Aro
	glifo         *iconos.Dibujo
	texto         *canvas.Text
}

var _ fyne.Widget = (*BarraRecientes)(nil)

// NuevaBarraRecientes crea la barra.
func NuevaBarraRecientes(rotulo string, alPulsar func()) *BarraRecientes {
	b := &BarraRecientes{Rotulo: rotulo}
	b.Nombre, b.AlPulsar = rotulo, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// CreateRenderer compone el filete, el fondo y el botón.
func (b *BarraRecientes) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b.filete = canvas.NewRectangle(tema.Color("border", v))
	b.fondo = canvas.NewRectangle(tema.Color("bg-subtle", v))
	b.caja = ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.Forma("radius-control"))
	b.aro = ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v).Abierto()
	b.glifo = dibujo(glifoAbajo, GlifoRecientes, "primary-text", v)
	b.texto = ui.Texto(b.Rotulo, tema.TextoSM, ui.Medio, tema.Color("primary-text", v))
	r := &barraRecientesRenderer{b: b}
	r.objetos = append([]fyne.CanvasObject{b.fondo, b.filete}, b.aro.Antes(b.caja.Objetos()...)...)
	r.objetos = append(r.objetos, b.glifo.Objeto(), b.texto)
	r.Refresh()
	return r
}

type barraRecientesRenderer struct {
	b       *BarraRecientes
	objetos []fyne.CanvasObject
}

func (r *barraRecientesRenderer) MinSize() fyne.Size           { return MedidaBarraRecientes() }
func (r *barraRecientesRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *barraRecientesRenderer) Destroy()                     {}

func (r *barraRecientesRenderer) Layout(t fyne.Size) {
	b := r.b
	b.fondo.Resize(t)
	b.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	pos := fyne.NewPos(CanalonHistorial, tema.Borde+RellenoBarraRecientes)
	tam := fyne.NewSize(max(t.Width-2*CanalonHistorial, 0), 2*RellenoBotonRecientes+tema.TextoSM.Linea)
	ui.MoverCaja(b.caja, pos, tam)
	b.aro.ColocarEn(pos, tam)
	ancho := GlifoRecientes + tema.Espacio(1.5) + ui.Medir(b.Rotulo, tema.TextoSM, ui.Medio).Width
	x := pos.X + (tam.Width-ancho)/2
	b.glifo.Colocar(x, pos.Y+(tam.Height-GlifoRecientes)/2, GlifoRecientes)
	ui.ColocarTexto(b.texto, tema.TextoSM, x+GlifoRecientes+tema.Espacio(1.5), pos.Y+RellenoBotonRecientes)
}

func (r *barraRecientesRenderer) Refresh() {
	b := r.b
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b.fondo.FillColor = tema.Color("bg-subtle", v)
	b.fondo.Refresh()
	b.filete.FillColor = tema.Color("border", v)
	b.filete.Refresh()
	relleno := color.NRGBA{}
	if b.Encima() {
		relleno = tokens.SobreSRGB(tema.Color("selected", v), tema.Color("bg-subtle", v))
	}
	b.caja.Pintar(relleno, color.NRGBA{})
	b.aro.Ver(b.Enfocado())
	b.aro.Pintar(v)
	r.Layout(b.Size())
}

// ---- La píldora de «cargando» ----

// PildoraCargando es `rounded-pill border bg-surface px-3 py-0.5 text-xs font-medium
// text-text-muted shadow-sm` con `role="status"`: la de «Cargando mensajes
// anteriores…» sobre la lista.
type PildoraCargando struct {
	widget.BaseWidget
	Texto string

	sombra *ui.Sombra
	caja   *ui.Caja
	texto  *canvas.Text
}

// NuevaPildoraCargando crea la píldora.
func NuevaPildoraCargando(texto string) *PildoraCargando {
	p := &PildoraCargando{Texto: texto}
	p.ExtendBaseWidget(p)
	return p
}

// AccessibilityLabel es el texto (`role="status"`).
func (p *PildoraCargando) AccessibilityLabel() string { return p.Texto }

// AccessibilityRole implementa fyne.Accessible.
func (p *PildoraCargando) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// MinSize es el texto con su `px-3 py-0.5` y el borde.
func (p *PildoraCargando) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	return fyne.NewSize(ui.Medir(p.Texto, tema.TextoXS, ui.Medio).Width+2*tema.Espacio(3)+2*tema.Borde,
		tema.TextoXS.Linea+2*tema.Espacio(0.5)+2*tema.Borde)
}

// CreateRenderer compone la sombra, la caja y el texto.
func (p *PildoraCargando) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p.sombra = ui.NuevaSombra(tema.SombraSM, tema.RadioPill)
	p.caja = ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioPill)
	p.texto = ui.Texto(p.Texto, tema.TextoXS, ui.Medio, tema.Color("text-muted", v))
	objs := append([]fyne.CanvasObject{p.sombra.Objeto()}, p.caja.Objetos()...)
	return &pildoraCargandoRenderer{p: p, objetos: append(objs, p.texto)}
}

type pildoraCargandoRenderer struct {
	p       *PildoraCargando
	objetos []fyne.CanvasObject
}

func (r *pildoraCargandoRenderer) MinSize() fyne.Size           { return r.p.MinSize() }
func (r *pildoraCargandoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pildoraCargandoRenderer) Destroy()                     {}

func (r *pildoraCargandoRenderer) Layout(t fyne.Size) {
	p := r.p
	p.sombra.Ajustar(fyne.NewPos(0, 0), t, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	p.caja.Redimensionar(t)
	ui.ColocarTexto(p.texto, tema.TextoXS, tema.Borde+tema.Espacio(3), tema.Borde+tema.Espacio(0.5))
}

func (r *pildoraCargandoRenderer) Refresh() { r.Layout(r.p.Size()) }

// ResumenHilo es un hilo de la vista «Hilos» (`ThreadSummary`): su raíz, la última
// actividad y las respuestas sin leer.
type ResumenHilo struct {
	Raiz      Mensaje
	Actividad time.Time
	SinLeer   int
}

// Borrador es un `LocalDraft`: dónde se escribía y qué.
type Borrador struct {
	Canal, Raiz, Texto string
}

// FilaBorrador es una fila de «Borradores»: `flex items-start gap-2 border-b px-2
// py-1`, el botón (`rounded-md px-2 py-2`, `hover:bg-bg-subtle`) con «#canal» en
// `text-sm font-semibold`, «en un hilo» en `.sytue-meta` y el texto en dos líneas de
// `text-sm text-muted`; y «Descartar» (`IconButton sm`, `mt-2`) fuera del botón. El
// texto va TAL CUAL, sin Markdown: es algo a medio escribir.
type FilaBorrador struct {
	ui.Pulsable

	T        *i18n.Translator
	Borrador Borrador
	Canal    string
	// Ultima quita el filete de abajo (`last:border-b-0`).
	Ultima bool

	fondo    *ui.Caja
	filete   *canvas.Rectangle
	nombre   *canvas.Text
	hilo     *canvas.Text
	cuerpo   []*canvas.Text
	aro      *ui.Aro
	descarta *ui.BotonIcono
	objetos  []fyne.CanvasObject
}

// Las medidas de la fila, en px de CSS. HOJA.
var (
	// CanalonBorrador es el `px-2` de la fila y el del botón; RellenoBorrador, el `py-1`
	// de la fila; RellenoBotonBorrador, el `py-2` del botón.
	CanalonBorrador      = tema.Espacio(2)
	RellenoBorrador      = tema.Espacio(1)
	RellenoBotonBorrador = tema.Espacio(2)
	// LadoDescartar es el `IconButton size="sm"`, `h-6 w-6`; GlifoDescartar, su `size={14}`.
	LadoDescartar  = tema.Espacio(6)
	GlifoDescartar = iconos.TallaXS
)

// NuevaFilaBorrador crea la fila. Su nombre accesible es el canal y el texto.
func NuevaFilaBorrador(t *i18n.Translator, b Borrador, canal string, alIr, alDescartar func(Borrador)) *FilaBorrador {
	f := &FilaBorrador{T: t, Borrador: b, Canal: canal}
	f.Nombre = f.rotulo() + ": " + b.Texto
	f.AlPulsar = func() {
		if alIr != nil {
			alIr(b)
		}
	}
	f.descarta = ui.NuevoBotonIcono(iconos.Ninguno, t.T("drafts.discard", nil), func() {
		if alDescartar != nil {
			alDescartar(b)
		}
	})
	f.descarta.Lado, f.descarta.TamIcono = LadoDescartar, GlifoDescartar
	f.ExtendBaseWidget(f)
	return f
}

func (f *FilaBorrador) rotulo() string {
	if f.Canal == "" {
		return f.T.T("drafts.unknownChannel", nil)
	}
	return "#" + f.Canal
}

// anchoTexto es la columna del texto dentro del botón.
func (f *FilaBorrador) anchoTexto(ancho float32) float32 {
	return max(ancho-2*CanalonBorrador-tema.Espacio(2)-LadoDescartar-2*CanalonBorrador, 0)
}

// lineas es el `line-clamp-2` del texto, con «…» si queda algo.
func (f *FilaBorrador) lineas(ancho float32) []string {
	cabe := f.anchoTexto(ancho)
	pars := partirEnLineas(f.Borrador.Texto)
	todas := make([]string, 0, len(pars))
	for _, par := range pars {
		todas = append(todas, ui.Repartir(par, tema.TextoSM, ui.Normal, cabe)...)
	}
	if len(todas) <= LineasResumenHilo {
		return todas
	}
	out := slices.Clone(todas[:LineasResumenHilo])
	c := canvas.NewText("", nil)
	ui.Truncar(c, strings.Join(todas[LineasResumenHilo-1:], " ")+" ", tema.TextoSM, ui.Normal, cabe)
	out[LineasResumenHilo-1] = c.Text
	return out
}

// MedidaFilaBorrador es lo que ocupa a ese ancho. Es la única cuenta.
func MedidaFilaBorrador(f *FilaBorrador, ancho float32) fyne.Size {
	boton := 2*RellenoBotonBorrador + tema.TextoSM.Linea + tema.Espacio(0.5) +
		float32(len(f.lineas(ancho)))*tema.TextoSM.Linea
	alto := 2*RellenoBorrador + max(boton, tema.Espacio(2)+LadoDescartar)
	if !f.Ultima {
		alto += tema.Borde
	}
	return fyne.NewSize(ancho, alto)
}

// CreateRenderer compone la fila.
func (f *FilaBorrador) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo = ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioMD)
	f.aro = ui.NuevoAro(tema.RadioMD, ui.SinHueco, v).Abierto() // `sytue-focus-ring`
	f.filete = canvas.NewRectangle(tema.Color("border", v))
	f.nombre = ui.Texto(f.rotulo(), tema.TextoSM, ui.SemiNegrita, tema.Color("text", v))
	f.descarta.Glifo = dibujo(glifoPapelera, GlifoDescartar, "text-subtle", v)
	f.objetos = append(f.fondo.Objetos(), f.aro.Objetos()...)
	f.objetos = append(f.objetos, f.filete, f.nombre, f.descarta)
	if f.Borrador.Raiz != "" {
		f.hilo = ui.Texto(f.T.T("drafts.inThread", nil), escalonMeta, ui.Normal, tema.Color("text-subtle", v))
		f.objetos = append(f.objetos, f.hilo)
	}
	r := &filaBorradorRenderer{f: f}
	r.Refresh()
	return r
}

type filaBorradorRenderer struct{ f *FilaBorrador }

func (r *filaBorradorRenderer) MinSize() fyne.Size           { return MedidaFilaBorrador(r.f, r.f.Size().Width) }
func (r *filaBorradorRenderer) Objects() []fyne.CanvasObject { return r.f.objetos }
func (r *filaBorradorRenderer) Destroy()                     {}

func (r *filaBorradorRenderer) Layout(t fyne.Size) {
	f := r.f
	x0 := CanalonBorrador
	ancho := max(t.Width-2*CanalonBorrador-tema.Espacio(2)-LadoDescartar, 0) // el botón es `flex-1`
	alto := MedidaFilaBorrador(f, t.Width).Height - 2*RellenoBorrador
	if !f.Ultima {
		alto -= tema.Borde
	}
	ui.MoverCaja(f.fondo, fyne.NewPos(x0, RellenoBorrador), fyne.NewSize(ancho, alto))
	f.aro.ColocarEn(fyne.NewPos(x0, RellenoBorrador), fyne.NewSize(ancho, alto))
	x, y := x0+CanalonBorrador, RellenoBorrador+RellenoBotonBorrador
	cabe := f.anchoTexto(t.Width)
	nx := x + ui.Truncar(f.nombre, f.rotulo(), tema.TextoSM, ui.SemiNegrita, cabe)
	ui.ColocarTexto(f.nombre, tema.TextoSM, x, y)
	if f.hilo != nil {
		ui.ColocarTexto(f.hilo, escalonMeta, nx+tema.Espacio(2), y+tema.TextoSM.Linea-escalonMeta.Linea)
	}
	y += tema.TextoSM.Linea + tema.Espacio(0.5)
	lineas := f.lineas(t.Width)
	tinta := tema.Color("text-muted", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	for i, s := range lineas {
		if i >= len(f.cuerpo) {
			c := ui.Texto(s, tema.TextoSM, ui.Normal, tinta)
			f.cuerpo = append(f.cuerpo, c)
			f.objetos = append(f.objetos, c)
		}
		f.cuerpo[i].Text, f.cuerpo[i].Color = s, tinta
		f.cuerpo[i].Show()
		ui.ColocarTexto(f.cuerpo[i], tema.TextoSM, x, y)
		y += tema.TextoSM.Linea
	}
	for i := len(lineas); i < len(f.cuerpo); i++ {
		f.cuerpo[i].Hide()
	}
	f.descarta.Move(fyne.NewPos(x0+ancho+tema.Espacio(2), RellenoBorrador+tema.Espacio(2)))
	f.descarta.Resize(fyne.NewSize(LadoDescartar, LadoDescartar))
	f.filete.Move(fyne.NewPos(0, t.Height-tema.Borde))
	f.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	f.filete.Hidden = f.Ultima
}

func (r *filaBorradorRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	if f.Encima() {
		fondo = tema.Color("bg-subtle", v)
	}
	f.fondo.Pintar(fondo, color.NRGBA{})
	f.aro.Pintar(v)
	f.aro.Ver(f.Enfocado())
	f.filete.FillColor = tema.Color("border", v)
	f.filete.Refresh()
	r.Layout(f.Size())
}

// Programado es un `ScheduledMessage`: quién, dónde, qué y cuándo sale.
type Programado struct {
	ID, Canal, Autor, Cuerpo string
	// ConAdjuntos dice que lleva ficheros: sin texto sale «Solo adjuntos». Adjuntos
	// son sus ids, que «Enviar ya» publica con él.
	ConAdjuntos bool
	Adjuntos    []string
	Cuando      time.Time
}

// FilaProgramado es una fila de «Programados»: `flex gap-3 border-b px-4 py-3`, el
// avatar `sm` en su `w-9 pt-0.5`, el nombre y el «#canal», el texto con su Markdown
// (`MessageBody`), «Se envía el …» en `text-xs font-medium text-muted` (`mt-1`) y los
// dos botones `sm` —«Enviar ya» secundario y «Cancelar» fantasma— en `mt-2 gap-2`.
//
// Delante de los botones va «Reprogramar», el `datetime-local` de la web: la fecha y la
// hora en `ui.CampoFechaHora`, que manda la nueva hora al confirmarla con Intro. La fila
// parte línea (`flex-wrap`) cuando los tres no caben, que en un teléfono es siempre.
type FilaProgramado struct {
	widget.BaseWidget

	T          *i18n.Translator
	Locale     i18n.Locale
	Programado Programado
	Canal      string
	Ultima     bool
	// AlReprogramar recibe la hora nueva; nil, sin «Reprogramar».
	AlReprogramar func(Programado, time.Time)

	filete        *canvas.Rectangle
	avatar        *ui.Avatar
	nombre, canal *canvas.Text
	cuerpo        *CuerpoRico
	adjuntos      *canvas.Text
	cuando        *canvas.Text
	enviar        *ui.Boton
	cancelar      *ui.Boton
	reprogramar   *ui.CampoFechaHora
	objetos       []fyne.CanvasObject
}

// RellenoProgramado es el `py-3` de la fila.
var RellenoProgramado = tema.Espacio(3)

// NuevaFilaProgramado crea la fila.
func NuevaFilaProgramado(t *i18n.Translator, l i18n.Locale, m Programado, canal string, alEnviar, alCancelar func(Programado)) *FilaProgramado {
	f := &FilaProgramado{T: t, Locale: l, Programado: m, Canal: canal}
	f.cancelar = ui.NuevoBoton(t.T("action.cancel", nil), func() {
		if alCancelar != nil {
			alCancelar(m)
		}
	}).Con(ui.Fantasma, ui.TallaSM)
	f.enviar = ui.NuevoBoton(t.T("scheduled.sendNow", nil), func() {
		if alEnviar != nil {
			alEnviar(m)
		}
	}).Con(ui.Secundario, ui.TallaSM)
	f.ExtendBaseWidget(f)
	return f
}

func (f *FilaProgramado) cabe(ancho float32) float32 {
	return max(ancho-2*CanalonFilaHilo-ColumnaAvatar-HuecoFila, 0)
}

func (f *FilaProgramado) altoTexto(ancho float32) float32 {
	if f.Programado.Cuerpo == "" {
		return tema.TextoSM.Linea
	}
	alto, _ := MaquetarCuerpo(f.Programado.Cuerpo, nil, f.cabe(ancho))
	return alto
}

// controles son los de la fila de abajo, en el orden del DOM.
func (f *FilaProgramado) controles() []fyne.CanvasObject {
	var out []fyne.CanvasObject
	if f.reprogramar == nil && f.AlReprogramar != nil {
		m := f.Programado
		f.reprogramar = ui.NuevoCampoFechaHora(ui.CampoSM, m.Cuando)
		f.reprogramar.AlConfirmar = func(cuando time.Time) { f.AlReprogramar(m, cuando) }
	}
	if f.reprogramar != nil {
		out = append(out, f.reprogramar.Objeto())
	}
	if f.enviar != nil {
		out = append(out, f.enviar)
	}
	return append(out, f.cancelar)
}

// altoBotones es la fila de los controles, partida en líneas si no caben.
func (f *FilaProgramado) altoBotones(ancho float32) float32 {
	return colocarEnvuelto(f.controles(), 0, 0, f.cabe(ancho), tema.Espacio(2), false)
}

// colocarEnvuelto es `flex flex-wrap items-center gap-*`: pone los objetos en fila a su
// tamaño, parte línea cuando el siguiente no cabe y centra cada uno en el alto de su
// línea. Devuelve el alto total; con `mover` falso solo mide, que es lo que hace falta
// para el alto de la fila antes de colocarla.
func colocarEnvuelto(objs []fyne.CanvasObject, x0, y0, cabe, hueco float32, mover bool) float32 {
	var lineas [][]fyne.CanvasObject
	x := float32(0)
	for _, o := range objs {
		w := medidaEnFila(o, cabe).Width
		if len(lineas) == 0 || (x > 0 && x+w > cabe) {
			lineas, x = append(lineas, nil), 0
		}
		lineas[len(lineas)-1] = append(lineas[len(lineas)-1], o)
		x += w + hueco
	}
	y := y0
	for i, l := range lineas {
		alto := float32(0)
		for _, o := range l {
			alto = max(alto, medidaEnFila(o, cabe).Height)
		}
		if mover {
			x := x0
			for _, o := range l {
				m := medidaEnFila(o, cabe)
				o.Move(fyne.NewPos(x, y+(alto-m.Height)/2))
				o.Resize(m)
				x += m.Width + hueco
			}
		}
		y += alto
		if i < len(lineas)-1 {
			y += hueco
		}
	}
	return y - y0
}

func medidaEnFila(o fyne.CanvasObject, ancho float32) fyne.Size {
	if fluido, ok := o.(interface{ medidaEnFila(float32) fyne.Size }); ok {
		return fluido.medidaEnFila(ancho)
	}
	return o.MinSize()
}

// MedidaFilaProgramado es lo que ocupa a ese ancho. Es la única cuenta.
func MedidaFilaProgramado(f *FilaProgramado, ancho float32) fyne.Size {
	alto := 2*RellenoProgramado + tema.TextoSM.Linea + f.altoTexto(ancho) +
		tema.Espacio(1) + tema.TextoXS.Linea + tema.Espacio(2) + f.altoBotones(ancho)
	if !f.Ultima {
		alto += tema.Borde
	}
	return fyne.NewSize(ancho, alto)
}

// CreateRenderer compone la fila.
func (f *FilaProgramado) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t, m := f.T, f.Programado
	f.filete = canvas.NewRectangle(tema.Color("border", v))
	f.avatar = ui.NuevoAvatar(m.Autor).Con(ui.AvatarSM)
	f.nombre = ui.Texto(m.Autor, tema.TextoSM, ui.SemiNegrita, tema.Color("text", v))
	f.canal = ui.Texto("#"+f.Canal, escalonMeta, ui.Normal, tema.Color("text-subtle", v))
	f.cuando = ui.Texto(t.T("scheduled.sendsAt", i18n.Vars{"when": RotuloCuando(f.Locale, m.Cuando)}),
		tema.TextoXS, ui.Medio, tema.Color("text-muted", v))
	f.objetos = []fyne.CanvasObject{f.filete, f.avatar, f.nombre, f.cuando}
	f.objetos = append(f.objetos, f.controles()...)
	if f.Canal != "" {
		f.objetos = append(f.objetos, f.canal)
	}
	if m.Cuerpo != "" {
		f.cuerpo = NuevoCuerpoRico(m.Cuerpo, nil) // las menciones no viajan con el programado
		f.objetos = append(f.objetos, f.cuerpo)
	} else {
		f.adjuntos = ui.Texto(t.T("message.attachmentsOnly", nil), tema.TextoSM, ui.Normal, tema.Color("text-subtle", v))
		f.adjuntos.FontSource = tema.Cursiva
		f.objetos = append(f.objetos, f.adjuntos)
	}
	return &filaProgramadoRenderer{f: f}
}

type filaProgramadoRenderer struct{ f *FilaProgramado }

func (r *filaProgramadoRenderer) MinSize() fyne.Size {
	return MedidaFilaProgramado(r.f, r.f.Size().Width)
}

func (r *filaProgramadoRenderer) Objects() []fyne.CanvasObject { return r.f.objetos }
func (r *filaProgramadoRenderer) Destroy()                     {}

func (r *filaProgramadoRenderer) Layout(t fyne.Size) {
	f := r.f
	y := RellenoProgramado
	f.avatar.Move(fyne.NewPos(CanalonFilaHilo, y+AireAvatar))
	f.avatar.Resize(ui.MedidaAvatar(ui.AvatarSM))
	x := CanalonFilaHilo + ColumnaAvatar + HuecoFila
	cabe := f.cabe(t.Width)
	nx := x + ui.Truncar(f.nombre, f.Programado.Autor, tema.TextoSM, ui.SemiNegrita, cabe) + HuecoNombre
	ui.ColocarTexto(f.nombre, tema.TextoSM, x, y)
	if f.Canal != "" {
		ui.Truncar(f.canal, "#"+f.Canal, escalonMeta, ui.Normal, max(x+cabe-nx, 0))
		ui.ColocarTexto(f.canal, escalonMeta, nx, y+tema.TextoSM.Linea-escalonMeta.Linea)
	}
	y += tema.TextoSM.Linea
	if f.cuerpo != nil {
		alto := f.altoTexto(t.Width)
		f.cuerpo.Move(fyne.NewPos(x, y))
		f.cuerpo.Resize(fyne.NewSize(cabe, alto))
		y += alto
	} else {
		ui.ColocarTexto(f.adjuntos, tema.TextoSM, x, y)
		y += tema.TextoSM.Linea
	}
	y += tema.Espacio(1)
	ui.ColocarTexto(f.cuando, tema.TextoXS, x, y)
	y += tema.TextoXS.Linea + tema.Espacio(2)
	colocarEnvuelto(f.controles(), x, y, cabe, tema.Espacio(2), true)
	f.filete.Move(fyne.NewPos(0, t.Height-tema.Borde))
	f.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	f.filete.Hidden = f.Ultima
}

func (r *filaProgramadoRenderer) Refresh() { r.Layout(r.f.Size()) }

func (f *FilaLista) altoA(ancho float32) float32      { return MedidaFilaLista(f, ancho).Height }
func (f *FilaBorrador) altoA(ancho float32) float32   { return MedidaFilaBorrador(f, ancho).Height }
func (f *FilaProgramado) altoA(ancho float32) float32 { return MedidaFilaProgramado(f, ancho).Height }

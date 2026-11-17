// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/portal/src/pages/notificaciones.astro

package hub

import (
	"image/color"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La bandeja de avisos: `NotificationsInbox.tsx` dentro de `notificaciones.astro`. Es la
// vista COMPLETA de lo que el panel de la campana solo asoma —filtros, «marcar todas» y
// las reglas de silencio—, y es adonde lleva «Ver todas» desde cualquier app de la suite.
//
// Las medidas salen de las clases con el rem de 16; las piezas que repite (cabecera,
// píldora, conmutador, estado vacío y botón) son las ya medidas en producción en las
// otras pantallas. La bandeja en sí NO se ha medido contra producción: la sonda del
// portal no la recorre todavía.
const (
	// MaxAvisosBandeja es el `limit: 100` de la carga.
	MaxAvisosBandeja = 100
	// HuecoBandeja es el `flex flex-col gap-6` entre la barra, la lista y las reglas.
	HuecoBandeja float32 = 6 * 0.25 * tema.Rem
	// HuecoBarraAvisos es el `gap-3` entre los filtros y «Marcar todas».
	HuecoBarraAvisos float32 = 3 * 0.25 * tema.Rem

	// La fila: `flex items-start gap-3 rounded-md px-1 py-3`.
	MargenXAviso float32 = 1 * 0.25 * tema.Rem
	MargenYAviso float32 = 3 * 0.25 * tema.Rem
	HuecoAviso   float32 = 3 * 0.25 * tema.Rem
	// LadoMarcaAviso es el `<ProductIcon size={18} />`, con su `mt-0.5`.
	LadoMarcaAviso float32 = 18
	// SangriaMetaAviso es el `mt-1` de la línea «Mail · hace 5 min».
	SangriaMetaAviso float32 = 1 * 0.25 * tema.Rem
	// LadoPuntoSinLeer es el `h-2 w-2` del punto. El punto va en su botón «Marcar como
	// leído» (`MarcarLeido` de @sytue/shell): `p-2` alrededor, una diana de 24 (WCAG
	// 2.5.8), y su `mt-3` lo deja a la altura del `py-3` de la fila.
	LadoPuntoSinLeer  float32 = 2 * 0.25 * tema.Rem
	RellenoPuntoAviso float32 = 2 * 0.25 * tema.Rem
	LadoBotonLeido            = LadoPuntoSinLeer + 2*RellenoPuntoAviso

	// El esqueleto: `Skeleton circle 18px` y dos barras de 14px y 12px con `gap-2`. Son
	// píxeles LITERALES en la web, no rem.
	AltoBarraEsqueleto1  float32 = 14
	AltoBarraEsqueleto2  float32 = 12
	FilasEsqueletoAvisos         = 4

	// Las reglas: `mt-2 border-t pt-6`, el `h2` de `text-base`, el párrafo con
	// `mt-1 mb-4` y los interruptores en `flex-col gap-3`.
	MargenReglas       float32 = 2 * 0.25 * tema.Rem
	RellenoReglas      float32 = 6 * 0.25 * tema.Rem
	SangriaTextoReglas float32 = 1 * 0.25 * tema.Rem
	MargenTextoReglas  float32 = 4 * 0.25 * tema.Rem
	HuecoReglas        float32 = 3 * 0.25 * tema.Rem

	// Los grupos de `PreferenciasAvisos.tsx`: `flex-col gap-6` entre familias, la
	// `legend` en `text-xs` con su `mb-3` y la rejilla `gap-x-6 gap-y-3`, a dos columnas
	// desde `sm`.
	HuecoGruposReglas   float32 = 6 * 0.25 * tema.Rem
	MargenLeyendaReglas float32 = 3 * 0.25 * tema.Rem
	HuecoColumnasReglas float32 = 6 * 0.25 * tema.Rem
	// AnchoDosColumnasReglas es el `sm:` de Tailwind (640 px) pasado al ancho de la
	// columna de contenido, que es lo que aquí se reparte.
	AnchoDosColumnasReglas float32 = 640
	// El fantasma de una regla: `Skeleton 36×20` y la etiqueta al 45 % con 14 de alto,
	// separados por `gap-2.5`. Píxeles literales en la web.
	AnchoFantasmaRegla       float32 = 36
	AltoFantasmaRegla        float32 = 20
	AltoEtiquetaFantasma     float32 = 14
	HuecoFantasmaRegla       float32 = 2.5 * 0.25 * tema.Rem
	SangriaFalloRegla        float32 = 1 * 0.25 * tema.Rem
	HuecoReintentarReglas    float32 = 3 * 0.25 * tema.Rem
	FraccionEtiquetaFantasma float32 = 0.45
)

// GrupoAvisos es una familia de orígenes bajo su `legend`.
type GrupoAvisos struct {
	// Grupo es la clave de `inbox.prefsGroup.<grupo>`.
	Grupo    string
	Origenes []string
}

// GruposDeAvisos es `ORIGENES_DE_AVISOS` de @sytue/shell (`avisos.ts`): los orígenes
// que se pueden silenciar, por familia y en el orden en que se enseñan. Antes eran seis
// productos escritos a mano y las tareas del ERP, los pedidos de Web o los directos de
// Cast avisaban sin remedio. `TestLosGruposDeAvisosSonLosDeLaWeb` compara las dos.
var GruposDeAvisos = []GrupoAvisos{
	{"communication", []string{"mail", "rooms", "calendar", "contacts"}},
	{"files", []string{"drive", "docs", "sheets", "slides", "pdf", "notes", "photos"}},
	{"business", []string{"erp", "bases"}},
	{"publishing", []string{"pages", "cast", "cloud"}},
	{"suite", []string{"one", "integrations"}},
}

// OrigenesSilenciables son todos los orígenes con interruptor, en el orden de la web.
var OrigenesSilenciables = func() []string {
	var out []string
	for _, g := range GruposDeAvisos {
		out = append(out, g.Origenes...)
	}
	return out
}()

// CargaReglas es el ciclo de las reglas, aparte del de la lista: la bandeja sirve
// aunque el Hub no sirva las preferencias.
type CargaReglas int

const (
	// ReglasListas es el cero: un `Avisos` suelto, sin Hub detrás, no espera nada.
	ReglasListas CargaReglas = iota
	ReglasCargando
	ReglasFallo
)

// OrigenFijo es el origen que sale al final, encendido y sin poder tocarse: los avisos
// de seguridad de Sytue ID, que el Hub no deja silenciar (`domain.Silenciable`) y que
// por eso no están en `GruposDeAvisos`.
const OrigenFijo = "id"

// SangriaNotaFija es el `ps-11.5` de la nota «Siempre activos…» bajo el interruptor fijo.
// HuecoNotaFija es el `gap-0.5` entre ese interruptor y su nota. Entre el último grupo
// y la fila fija va el `gap-6` de los grupos (`HuecoGruposReglas`).
const (
	SangriaNotaFija float32 = 11.5 * 0.25 * tema.Rem
	HuecoNotaFija   float32 = 0.5 * 0.25 * tema.Rem
)

// Silenciado es el `silenciado` de la web: solo cuenta la regla del ORIGEN entero (sin
// `kind`). Las de un tipo concreto las pone quien las pone, no esta pantalla.
func Silenciado(reglas []api.HubNotificationPref, origen string) bool {
	for _, p := range reglas {
		if p.Source == origen && p.Kind == "" && p.Muted {
			return true
		}
	}
	return false
}

// ConRegla sustituye la regla del origen entero por la nueva, dejando las demás: el
// reductor optimista de `silenciar`.
func ConRegla(reglas []api.HubNotificationPref, origen string, silenciar bool) []api.HubNotificationPref {
	out := make([]api.HubNotificationPref, 0, len(reglas)+1)
	for _, p := range reglas {
		if p.Source != origen || p.Kind != "" {
			out = append(out, p)
		}
	}
	return append(out, api.HubNotificationPref{Source: origen, Muted: silenciar})
}

// Visibles son los avisos que pasan los dos filtros: «sin leer» y el producto.
func Visibles(avisos []api.HubNotification, soloSinLeer bool, producto string) []api.HubNotification {
	var out []api.HubNotification
	for _, n := range avisos {
		if (!soloSinLeer || !n.Read) && (producto == "" || n.Source == producto) {
			out = append(out, n)
		}
	}
	return out
}

// Avisos es la pantalla `/notificaciones`.
type Avisos struct {
	pagina

	T *i18n.Translator
	// Ahora es la hora contra la que se dice «hace 5 min»; con el cero, la del reloj.
	Ahora time.Time

	Estado Estado
	Items  []api.HubNotification
	Reglas []api.HubNotificationPref
	// CargaReglas dice si las reglas están, llegando o sin poder llegar: sin ellas no se
	// sabe qué está callado y NO se enseñan interruptores, que saldrían encendidos.
	CargaReglas CargaReglas
	// GuardandoRegla son los orígenes con un cambio en vuelo: no admiten otro clic (dos
	// PUT cruzados dejarían el que llegue último, no el último pulsado).
	GuardandoRegla map[string]bool
	// FalloRegla son los orígenes cuyo guardado falló y volvieron a su sitio: lo dice su fila.
	FalloRegla map[string]bool
	// Organizacion es la activa: las reglas del Hub son por persona Y organización.
	Organizacion string
	// Personal: cuenta personal, sin ámbito de organización ni ERP, Bases o Web.
	Personal bool
	// AlReintentarReglas vuelve a pedirlas tras un fallo.
	AlReintentarReglas func()

	// Los dos filtros son estado de la PANTALLA, como en la web: no piden nada al Hub.
	SoloSinLeer bool
	Producto    string

	// HayMas: quedan avisos más antiguos; AlCargarMas los pide («Cargar más»).
	HayMas      bool
	AlCargarMas func()

	AlAbrir func(api.HubNotification)
	// AlMarcarLeido marca UNO como leído sin abrirlo: el punto azul, hecho botón.
	AlMarcarLeido func(api.HubNotification)
	// enfocarTras es el aviso cuya fila recibe el foco en el próximo repintado: el del
	// punto que se acaba de pulsar con el foco puesto. El botón se va al marcarlo y, sin
	// esto, el foco se quedaría en nada y el siguiente Tab empezaría desde arriba.
	enfocarTras string
	// enfocarPuesto es el puesto que esa fila ocupaba en la lista: si el aviso ya no se
	// ve —con «Sin leer», leído sale—, el foco va a la que ahora ocupa ese puesto, la
	// siguiente, o a la última si era el último.
	enfocarPuesto int
	AlMarcarTodas func()
	AlSilenciar   func(origen string, silenciar bool)
	AlReintentar  func()
}

var _ fyne.Widget = (*Avisos)(nil)

// NuevaAvisos crea la pantalla.
func NuevaAvisos(t *i18n.Translator) *Avisos {
	a := &Avisos{T: t}
	a.ExtendBaseWidget(a)
	return a
}

// Resize recuerda el ancho: el alto depende de él (los avisos se reparten en líneas).
func (a *Avisos) Resize(s fyne.Size) {
	if a.recordar(s) {
		a.BaseWidget.Resize(s)
		a.Refresh()
		return
	}
	a.BaseWidget.Resize(s)
}

// AccessibilityRole: la pantalla agrupa.
func (a *Avisos) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

func (a *Avisos) ahora() time.Time {
	if a.Ahora.IsZero() {
		return time.Now()
	}
	return a.Ahora
}

// sinLeer son los no leídos de lo cargado: es lo que cuenta la píldora de la web, no el
// contador del servidor.
func (a *Avisos) sinLeer() int {
	n := 0
	for _, x := range a.Items {
		if !x.Read {
			n++
		}
	}
	return n
}

// gruposVisibles son los grupos que esta cuenta ve: una personal no abre Sytue ERP,
// Bases ni Web y ofrecer callarlos es ruido. Un grupo que se queda sin nada, se va.
func (a *Avisos) gruposVisibles() []GrupoAvisos {
	var out []GrupoAvisos
	for _, g := range GruposDeAvisos {
		var origenes []string
		for _, o := range g.Origenes {
			if !a.Personal || !slices.Contains(SoloOrganizaciones, o) {
				origenes = append(origenes, o)
			}
		}
		if len(origenes) > 0 {
			out = append(out, GrupoAvisos{g.Grupo, origenes})
		}
	}
	return out
}

// CreateRenderer construye la pantalla. Los interruptores se crean UNA vez y
// sobreviven a los repintados: rehacerlos al pulsar uno le quitaba el foco a quien lo
// maneja con el teclado.
func (a *Avisos) CreateRenderer() fyne.WidgetRenderer {
	r := &avisosRenderer{
		a:            a,
		fondo:        ui.NuevoFondoSuite(tema.De(fyne.CurrentApp().Settings().ThemeVariant())),
		conmutadores: map[string]*ui.Conmutador{},
	}
	for _, clave := range OrigenesSilenciables {
		nombre := clave
		if p, ok := ProductoDe(clave); ok {
			nombre = p.Nombre
		} else if clave == "integrations" {
			nombre = a.T.T("inbox.prefsSource.integrations", nil)
		}
		c := ui.NuevoConmutador(nombre, func(recibir bool) {
			if a.AlSilenciar != nil {
				a.AlSilenciar(clave, !recibir) // el interruptor dice «recibir»; el Hub guarda «silenciado»
			}
		})
		r.conmutadores[clave] = c
	}
	nombreFijo := OrigenFijo
	if p, ok := ProductoDe(OrigenFijo); ok {
		nombreFijo = p.Nombre
	}
	r.fijo = ui.NuevoConmutador(nombreFijo, nil)
	r.fijo.Encendido = true
	r.fijo.Disable()
	r.rehacer()
	return r
}

type avisosRenderer struct {
	a *Avisos

	fondo     *ui.FondoSuite
	cabecera  *cabecera
	filtros   []*Pildora
	marcar    *ui.Boton
	esqueleto []fyne.CanvasObject
	carga     *ui.BloquesEsqueleto // el grupo de `esqueleto`
	aviso     *ui.Vacio
	filas     []*FilaAviso
	// leidos es el botón del punto de cada fila sin leer, a su par en `filas` (nil en
	// las leídas). Van FUERA de la fila, como en la web: hermanos, y encima.
	leidos []*BotonLeido
	// ids es el aviso de cada fila, a su par en `filas`.
	ids     []string
	filetes []*canvas.Rectangle
	mas     *ui.Boton

	fileteReglas *canvas.Rectangle
	tituloReglas *canvas.Text
	textoReglas  *ui.Parrafo
	conmutadores map[string]*ui.Conmutador
	// Lo de las reglas que cambia con su carga: los grupos visibles con su leyenda, los
	// fantasmas (dos por origen) mientras cargan, el aviso con «Reintentar» si fallan y
	// el texto de la fila cuyo guardado falló.
	grupos      []GrupoAvisos
	leyendas    []*canvas.Text
	fantasmas   map[string][2]*canvas.Rectangle
	cargaReglas *ui.BloquesEsqueleto // el grupo de los fantasmas
	falloReglas *ui.AyudaEnLinea
	reintentar  *ui.Boton
	fallosFila  map[string]*ui.Parrafo
	// fijo es el de Sytue ID: encendido e inerte, con notaFijo («Siempre activos…»).
	// Sale, como en la web, solo con las reglas ya cargadas.
	fijo     *ui.Conmutador
	notaFijo *ui.Parrafo

	objetos []fyne.CanvasObject
}

// textosAvisos son las claves de sus dos estados propios.
func textosAvisos(soloSinLeer bool) Textos {
	if soloSinLeer {
		return Textos{Vacio: "inbox.emptyUnreadTitle", VacioCuerpo: "inbox.emptyUnreadBody"}
	}
	return Textos{Vacio: "inbox.emptyTitle", VacioCuerpo: "inbox.emptyBody"}
}

func (r *avisosRenderer) rehacer() {
	a, t := r.a, r.a.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	_, ancho := CajaContenido(a.Ancho())

	r.fondo.PonerVariante(v)
	r.cabecera = nuevaCabecera(v, t.T("inbox.overline", nil), t.T("inbox.heading", nil), t.T("inbox.intro", nil))
	r.cabecera.preparar(v, ancho)
	r.hacerBarra(t)
	r.esqueleto, r.carga, r.aviso, r.filas, r.filetes, r.mas, r.leidos, r.ids = nil, nil, nil, nil, nil, nil, nil, nil

	visibles := Visibles(a.Items, a.SoloSinLeer, a.Producto)
	switch {
	case a.Estado == Cargando:
		// Las cuatro filas son UN grupo, que se anuncia con `inbox.loading` como el
		// `role="status"` de la web.
		g := ui.NuevosBloquesEsqueleto()
		for range FilasEsqueletoAvisos {
			circulo := g.Bloque(LadoMarcaAviso, LadoMarcaAviso)
			circulo.CornerRadius = LadoMarcaAviso / 2 // `circle`
			r.esqueleto = append(r.esqueleto, circulo,
				g.Bloque(0, AltoBarraEsqueleto1), g.Bloque(0, AltoBarraEsqueleto2))
		}
		r.carga = g.AMano().ConRotulo(t.T("inbox.loading", nil))
		r.filetes = Divisores(v, FilasEsqueletoAvisos)
	case a.Estado == Fallo:
		// Sin icono y con el botón en su talla de siempre: es el EmptyState de la
		// bandeja, no el `Aviso` de las otras pantallas.
		r.aviso = ui.NuevoVacio(t.T("inbox.errorTitle", nil), t.T("inbox.errorBody", nil))
		if a.AlReintentar != nil {
			r.aviso.Accion = ui.NuevoBoton(t.T("inbox.retry", nil), a.AlReintentar).Con(ui.Secundario, ui.TallaMD)
		}
	case a.Estado == SinConexion || a.Estado == Prohibido:
		r.aviso = Aviso(t, v, a.Estado, textosAvisos(false), a.AlReintentar)
	case len(visibles) == 0:
		textos := textosAvisos(a.SoloSinLeer)
		if a.HayMas {
			// Con más detrás, «nada que leer» o «estás al día» serían falsos: solo no hay
			// entre lo cargado, y «Cargar más» trae lo siguiente. Como en la web.
			textos.Vacio, textos.VacioCuerpo = "notifications.noneLoaded", ""
		}
		r.aviso = Aviso(t, v, SinNada, textos, nil)
	default:
		for puesto, n := range visibles {
			r.filas = append(r.filas, r.filaDe(n, ancho))
			r.ids = append(r.ids, n.ID)
			var b *BotonLeido
			if !n.Read {
				b = NuevoBotonLeido(t.T("inbox.markRead", nil), func() {
					if b.Enfocado() {
						a.enfocarTras, a.enfocarPuesto = n.ID, puesto
					}
					if a.AlMarcarLeido != nil {
						a.AlMarcarLeido(n)
					}
				})
			}
			r.leidos = append(r.leidos, b)
		}
		r.filetes = Divisores(v, len(r.filas))
	}
	// «Cargar más» también con un filtro que no deja ver nada: lo que busca puede estar
	// detrás. Como en la web.
	if a.Estado == Listo && a.HayMas && a.AlCargarMas != nil && len(a.Items) > 0 {
		r.mas = ui.NuevoBoton(t.T("notifications.loadMore", nil), a.AlCargarMas).Con(ui.Secundario, ui.TallaSM)
	}

	r.fileteReglas = canvas.NewRectangle(tema.Color("border", v))
	r.tituloReglas = ui.Texto(t.T("inbox.prefsTitle", nil), tema.TextoBase, ui.SemiNegrita, tema.Color("text", v))
	ambito := t.T("inbox.prefsScopePersonal", nil)
	if !a.Personal && a.Organizacion != "" {
		ambito = t.T("inbox.prefsScopeOrg", map[string]any{"org": a.Organizacion})
	}
	r.textoReglas = ui.NuevoParrafo(t.T("inbox.prefsBody", nil)+" "+ambito, tema.TextoSM, ui.Normal, "text-muted", v).
		A(min(ancho, AnchoIntro))
	r.notaFijo = ui.NuevoParrafo(t.T("inbox.prefsAlwaysOn", nil), tema.TextoSM, ui.Normal, "text-muted", v).
		A(max(0, ancho-SangriaNotaFija))
	r.rehacerReglas(v)
	r.recomponer()
}

// hacerBarra construye los filtros y «Marcar todas», que solo sale si hay algo sin leer.
func (r *avisosRenderer) hacerBarra(t *i18n.Translator) {
	a := r.a
	sinLeer := a.sinLeer()
	r.filtros = []*Pildora{
		NuevaPildora(t.T("inbox.all", nil), !a.SoloSinLeer, func() { a.SoloSinLeer = false; a.Refresh() }),
		NuevaPildora(t.T("inbox.unreadFilter", i18n.Vars{"count": sinLeer}), a.SoloSinLeer,
			func() { a.SoloSinLeer = true; a.Refresh() }),
	}
	// Los productos que HAN avisado de algo: ofrecer un filtro vacío es ruido.
	visto := map[string]bool{}
	for _, n := range a.Items {
		if visto[n.Source] {
			continue
		}
		visto[n.Source] = true
		clave := n.Source
		rotulo := clave
		p, ok := ProductoDe(clave)
		if ok {
			rotulo = p.NombreCorto()
		}
		pl := NuevaPildora(rotulo, a.Producto == clave, func() {
			if a.Producto == clave {
				a.Producto = ""
			} else {
				a.Producto = clave
			}
			a.Refresh()
		})
		if ok {
			pl.Marca, pl.ConMarca = p.Marca, true
		}
		r.filtros = append(r.filtros, pl)
	}
	r.marcar = nil
	if sinLeer > 0 {
		r.marcar = ui.NuevoBoton(t.T("inbox.markAll", nil), func() {
			if a.AlMarcarTodas != nil {
				a.AlMarcarTodas()
			}
		}).Con(ui.Secundario, ui.TallaMD)
	}
}

func (r *avisosRenderer) filaDe(n api.HubNotification, ancho float32) *FilaAviso {
	a := r.a
	origen := n.Source
	p, ok := ProductoDe(n.Source)
	if ok {
		origen = p.NombreCorto()
	}
	// El título en el idioma de la app y el cuerpo con la hora local: los mismos que la
	// campana de la barra y que la bandeja de la web.
	f := NuevaFilaAviso(ui.TituloAviso(a.T, n), ui.CuerpoAviso(a.T.Locale(), n), ui.ProcedenciaAviso(a.T, n)+origen+" · "+i18n.FormatTimeAgo(a.T.Locale(), n.CreatedAt, a.ahora()),
		func() {
			if a.AlAbrir != nil {
				a.AlAbrir(n)
			}
		})
	f.Conocido, f.Marca = ok, p.Marca
	f.SinLeer, f.Enlace, f.Ancho = !n.Read, n.URL != "", ancho
	f.RotuloPunto = a.T.T("inbox.unreadDot", nil)
	return f
}

func (r *avisosRenderer) recomponer() {
	r.objetos = []fyne.CanvasObject{r.fondo}
	r.objetos = append(r.objetos, r.cabecera.objetos()...)
	for _, p := range r.filtros {
		r.objetos = append(r.objetos, p)
	}
	if r.marcar != nil {
		r.objetos = append(r.objetos, r.marcar)
	}
	if r.carga != nil {
		r.objetos = append(r.objetos, r.carga)
	}
	if r.aviso != nil {
		r.objetos = append(r.objetos, r.aviso)
	}
	// Cada punto justo detrás de SU fila: Fyne recorre el foco en el orden de Objects(),
	// y el de la web es el del DOM, fila, punto, fila, punto (docs/design.md §2.6).
	for i, f := range r.filas {
		r.objetos = append(r.objetos, f)
		if b := r.leidos[i]; b != nil {
			r.objetos = append(r.objetos, b)
		}
	}
	for _, f := range r.filetes {
		r.objetos = append(r.objetos, f)
	}
	if r.mas != nil {
		r.objetos = append(r.objetos, r.mas)
	}
	r.objetos = append(r.objetos, r.fileteReglas, r.tituloReglas, r.textoReglas)
	if r.falloReglas != nil {
		r.objetos = append(r.objetos, r.falloReglas)
		if r.reintentar != nil {
			r.objetos = append(r.objetos, r.reintentar)
		}
		return
	}
	if r.cargaReglas != nil {
		r.objetos = append(r.objetos, r.cargaReglas)
	}
	for i, g := range r.grupos {
		r.objetos = append(r.objetos, r.leyendas[i])
		for _, o := range g.Origenes {
			if _, ok := r.fantasmas[o]; ok {
				continue
			}
			r.objetos = append(r.objetos, r.conmutadores[o])
			if p := r.fallosFila[o]; p != nil {
				r.objetos = append(r.objetos, p)
			}
		}
	}
	if r.conFijo() {
		r.objetos = append(r.objetos, r.fijo, r.notaFijo)
	}
}

// conFijo dice si sale la fila fija de Sytue ID: con las reglas cargadas, como en la web.
func (r *avisosRenderer) conFijo() bool { return r.a.CargaReglas == ReglasListas }

// rehacerReglas pone la sección de reglas en su estado: fantasmas sin nada que pulsar
// mientras cargan, el aviso sin interruptores si fallan, y si no los interruptores en
// su posición, inertes los que guardan y con su texto los que fallaron.
func (r *avisosRenderer) rehacerReglas(v tema.Variante) {
	a, t := r.a, r.a.T
	r.grupos, r.leyendas, r.fantasmas, r.falloReglas, r.reintentar, r.fallosFila = a.gruposVisibles(), nil, nil, nil, nil, nil
	r.cargaReglas = nil
	if a.CargaReglas == ReglasFallo {
		r.falloReglas = ui.NuevaAyudaEnLinea(t.T("inbox.prefsLoadErrorBody", nil)).
			ConTono(ui.AyudaPeligro).ConTitulo(t.T("inbox.prefsLoadError", nil))
		if a.AlReintentarReglas != nil {
			r.reintentar = ui.NuevoBoton(t.T("inbox.retry", nil), a.AlReintentarReglas).Con(ui.Secundario, ui.TallaSM)
		}
		return
	}
	for _, g := range r.grupos {
		r.leyendas = append(r.leyendas, ui.Texto(strings.ToUpper(t.T("inbox.prefsGroup."+g.Grupo, nil)),
			tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v)))
		for _, o := range g.Origenes {
			if a.CargaReglas == ReglasCargando {
				// Todos los fantasmas son UN grupo: laten a una.
				if r.fantasmas == nil {
					r.fantasmas = map[string][2]*canvas.Rectangle{}
					r.cargaReglas = ui.NuevosBloquesEsqueleto()
				}
				r.fantasmas[o] = [2]*canvas.Rectangle{
					r.cargaReglas.Bloque(AnchoFantasmaRegla, AltoFantasmaRegla),
					r.cargaReglas.Bloque(0, AltoEtiquetaFantasma),
				}
				continue
			}
			c := r.conmutadores[o]
			c.Encendido = !Silenciado(a.Reglas, o)
			if a.GuardandoRegla[o] {
				c.Disable()
			} else {
				c.Enable()
			}
			if a.FalloRegla[o] {
				if r.fallosFila == nil {
					r.fallosFila = map[string]*ui.Parrafo{}
				}
				r.fallosFila[o] = ui.NuevoParrafo(t.T("inbox.prefsSaveError", nil), tema.TextoXS, ui.Normal, "danger-text", v)
			}
		}
	}
	if r.cargaReglas != nil {
		r.cargaReglas.AMano()
	}
}

// anchoColumnaReglas es el de cada celda de la rejilla: dos columnas desde `sm`.
func anchoColumnaReglas(ancho float32) (float32, int) {
	if ancho >= AnchoDosColumnasReglas {
		return (ancho - HuecoColumnasReglas) / 2, 2
	}
	return ancho, 1
}

func (r *avisosRenderer) MinSize() fyne.Size { return fyne.NewSize(0, r.alto(r.a.Ancho())) }

func (r *avisosRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *avisosRenderer) Destroy()                     {}

// Refresh rehace la pantalla con los datos de ahora (ver el comentario del lanzador).
func (r *avisosRenderer) Refresh() {
	r.rehacer()
	r.Layout(r.a.Size())
	canvas.Refresh(r.a)
	r.devolverFoco()
}

// devolverFoco lleva el foco a la fila del aviso que se acaba de marcar desde su punto
// (`enfocarTras`), como `MarcarLeido` en la web. Si la fila ya no se ve —con «Sin leer»
// el aviso leído sale de la lista—, va a la del aviso siguiente, que ocupa ahora su
// puesto, o a la del anterior si era el último; como en la web.
func (r *avisosRenderer) devolverFoco() {
	id := r.a.enfocarTras
	if id == "" {
		return
	}
	r.a.enfocarTras = ""
	c := fyne.CurrentApp().Driver().CanvasForObject(r.a)
	if c == nil {
		return
	}
	for i, f := range r.filas {
		if r.ids[i] == id {
			c.Focus(f)
			return
		}
	}
	if n := len(r.filas); n > 0 {
		c.Focus(r.filas[min(r.a.enfocarPuesto, n-1)])
	}
}

// AltoFilaEsqueleto es una fila de carga: `py-3` y la columna de dos barras con `gap-2`.
func AltoFilaEsqueleto() float32 {
	return 2*MargenYAviso + max(LadoMarcaAviso, AltoBarraEsqueleto1+HuecoFiltros+AltoBarraEsqueleto2)
}

// alto es lo que ocupa la pantalla entera. Es la ÚNICA cuenta: Layout recorre lo mismo.
func (r *avisosRenderer) alto(ventana float32) float32 {
	x, ancho := CajaContenido(ventana)
	y := MargenY + r.cabecera.alto(ancho) + MargenCabeceraActividad
	y += r.barra(x, y, ancho, false) + HuecoBandeja
	y += r.altoLista(ancho) + HuecoBandeja
	return y + r.altoReglas() + MargenY
}

func (r *avisosRenderer) altoLista(ancho float32) float32 {
	var extra float32
	if r.mas != nil {
		extra = HuecoBandeja + r.mas.MinSize().Height // `flex-col gap-6` hasta el botón
	}
	switch {
	case len(r.esqueleto) > 0:
		return FilasEsqueletoAvisos*AltoFilaEsqueleto() + (FilasEsqueletoAvisos-1)*tema.Borde
	case r.aviso != nil:
		return r.aviso.MedidaVacio(ancho) + extra
	}
	var alto float32
	for i, f := range r.filas {
		if i > 0 {
			alto += tema.Borde // `divide-y`
		}
		alto += AltoFilaAviso(f, ancho)
	}
	return alto + extra
}

func (r *avisosRenderer) altoReglas() float32 {
	_, ancho := CajaContenido(r.a.Ancho())
	return r.reglas(0, 0, ancho, false)
}

// barra coloca los filtros y el botón (`flex flex-wrap items-center justify-between
// gap-3`, con los filtros en su propio `flex-wrap gap-2`) y devuelve el alto. Con `mover`
// a false solo mide: así `alto` y `Layout` no pueden discrepar.
//
// Es lo que hace el navegador: si la fila de filtros y el botón caben, van en una línea
// y centrados en vertical; si no, el botón baja a la siguiente, a la izquierda, y los
// filtros se reparten en todo el ancho.
func (r *avisosRenderer) barra(x, y, ancho float32, mover bool) float32 {
	altoPildora := MedidaPildora("").Height
	var natural float32
	for i, p := range r.filtros {
		if i > 0 {
			natural += HuecoFiltros
		}
		natural += p.MinSize().Width
	}
	var boton fyne.Size
	if r.marcar != nil {
		boton = r.marcar.MinSize()
	}
	enLinea := r.marcar == nil || natural+HuecoBarraAvisos+boton.Width <= ancho

	// Los filtros, repartidos en filas dentro del ancho.
	filas := 1
	px, py := x, float32(0)
	sitios := make([]fyne.Position, len(r.filtros))
	for i, p := range r.filtros {
		w := p.MinSize().Width
		if px > x && px+w > x+ancho {
			px, py = x, py+altoPildora+HuecoFiltros
			filas++
		}
		sitios[i] = fyne.NewPos(px, py)
		px += w + HuecoFiltros
	}
	altoNav := float32(filas)*altoPildora + float32(filas-1)*HuecoFiltros

	navY, botonPos := y, fyne.NewPos(x, y+altoNav+HuecoBarraAvisos)
	total := altoNav
	if r.marcar != nil {
		total = altoNav + HuecoBarraAvisos + boton.Height
	}
	if enLinea {
		total = max(altoNav, boton.Height)
		navY = y + (total-altoNav)/2
		botonPos = fyne.NewPos(x+ancho-boton.Width, y+(total-boton.Height)/2)
	}
	if mover {
		for i, p := range r.filtros {
			p.Move(fyne.NewPos(sitios[i].X, navY+sitios[i].Y))
			p.Resize(p.MinSize())
		}
		if r.marcar != nil {
			r.marcar.Move(botonPos)
			r.marcar.Resize(boton)
		}
	}
	return total
}

// Layout recorre lo mismo que `alto`, en el mismo orden.
func (r *avisosRenderer) Layout(t fyne.Size) {
	if t.Width <= 0 {
		return
	}
	tam := fyne.NewSize(t.Width, max(t.Height, r.alto(t.Width)))
	r.fondo.Resize(tam)
	r.carga.Cubrir(tam)
	r.cargaReglas.Cubrir(tam)
	x, ancho := CajaContenido(t.Width)
	y := r.cabecera.colocar(x) + MargenCabeceraActividad
	y += r.barra(x, y, ancho, true) + HuecoBandeja

	switch {
	case len(r.esqueleto) > 0:
		r.colocarEsqueleto(x, y, ancho)
	case r.aviso != nil:
		r.aviso.Move(fyne.NewPos(x, y))
		r.aviso.Resize(fyne.NewSize(ancho, r.aviso.MedidaVacio(ancho)))
	}
	fy := y
	for i, f := range r.filas {
		if i > 0 {
			ColocarDivisor(r.filetes[i-1], x, fy, ancho)
			fy += tema.Borde
		}
		h := AltoFilaAviso(f, ancho)
		f.Move(fyne.NewPos(x, fy))
		f.Resize(fyne.NewSize(anchoFilaAviso(f, ancho), h))
		if b := r.leidos[i]; b != nil {
			// `mt-3`, a la derecha de la fila.
			b.Move(fyne.NewPos(x+ancho-LadoBotonLeido, fy+MargenYAviso))
			b.Resize(fyne.NewSize(LadoBotonLeido, LadoBotonLeido))
		}
		fy += h
	}
	if r.mas != nil {
		if r.aviso != nil {
			fy += r.aviso.MedidaVacio(ancho)
		}
		r.mas.Move(fyne.NewPos(x, fy+HuecoBandeja))
		r.mas.Resize(r.mas.MinSize())
	}
	y += r.altoLista(ancho) + HuecoBandeja
	r.colocarReglas(x, y, ancho)
}

// colocarEsqueleto pone las cuatro filas de carga: el círculo centrado y las dos barras
// al 45 % y al 70 % de la columna de texto.
func (r *avisosRenderer) colocarEsqueleto(x, y, ancho float32) {
	columna := ancho - 2*MargenXAviso - LadoMarcaAviso - HuecoAviso
	cx := x + MargenXAviso + LadoMarcaAviso + HuecoAviso
	altoCol := AltoBarraEsqueleto1 + HuecoFiltros + AltoBarraEsqueleto2
	for i := range FilasEsqueletoAvisos {
		fy := y + float32(i)*(AltoFilaEsqueleto()+tema.Borde)
		if i > 0 {
			ColocarDivisor(r.filetes[i-1], x, fy-tema.Borde, ancho)
		}
		interior := fy + MargenYAviso + (max(LadoMarcaAviso, altoCol)-altoCol)/2
		circulo, b1, b2 := r.esqueleto[3*i], r.esqueleto[3*i+1], r.esqueleto[3*i+2]
		circulo.Move(fyne.NewPos(x+MargenXAviso, fy+MargenYAviso+(max(LadoMarcaAviso, altoCol)-LadoMarcaAviso)/2))
		circulo.Resize(fyne.NewSize(LadoMarcaAviso, LadoMarcaAviso))
		b1.Move(fyne.NewPos(cx, interior))
		b1.Resize(fyne.NewSize(columna*0.45, AltoBarraEsqueleto1))
		b2.Move(fyne.NewPos(cx, interior+AltoBarraEsqueleto1+HuecoFiltros))
		b2.Resize(fyne.NewSize(columna*0.7, AltoBarraEsqueleto2))
	}
}

func (r *avisosRenderer) colocarReglas(x, y, ancho float32) { r.reglas(x, y, ancho, true) }

// reglas coloca la sección de reglas y devuelve su alto; con `mover` a false solo mide,
// como `barra`: así `alto` y `Layout` no pueden discrepar.
func (r *avisosRenderer) reglas(x, y, ancho float32, mover bool) float32 {
	y0 := y
	y += MargenReglas
	if mover {
		r.fileteReglas.Move(fyne.NewPos(x, y))
		r.fileteReglas.Resize(fyne.NewSize(ancho, tema.Borde))
	}
	y += tema.Borde + RellenoReglas
	if mover {
		ui.ColocarTexto(r.tituloReglas, tema.TextoBase, x, y)
	}
	y += tema.TextoBase.Linea + SangriaTextoReglas
	alto := ui.AltoParrafo(r.textoReglas, r.textoReglas.Ancho)
	if mover {
		r.textoReglas.Move(fyne.NewPos(x, y))
		r.textoReglas.Resize(fyne.NewSize(r.textoReglas.Ancho, alto))
	}
	y += alto + MargenTextoReglas

	if r.falloReglas != nil {
		m := ui.MedidaAyudaEnLinea(r.falloReglas, ancho)
		if mover {
			r.falloReglas.Move(fyne.NewPos(x, y))
			r.falloReglas.Resize(m)
		}
		y += m.Height
		if r.reintentar != nil {
			b := r.reintentar.MinSize()
			if mover {
				r.reintentar.Move(fyne.NewPos(x, y+HuecoReintentarReglas))
				r.reintentar.Resize(b)
			}
			y += HuecoReintentarReglas + b.Height
		}
		return y - y0
	}

	col, columnas := anchoColumnaReglas(ancho)
	for i, g := range r.grupos {
		if i > 0 {
			y += HuecoGruposReglas
		}
		if mover {
			ui.ColocarTexto(r.leyendas[i], tema.TextoXS, x, y)
		}
		y += tema.TextoXS.Linea + MargenLeyendaReglas
		// La rejilla, fila a fila: cada fila mide lo que su celda más alta.
		for f := 0; f < len(g.Origenes); f += columnas {
			if f > 0 {
				y += HuecoReglas
			}
			var altoFila float32
			for c := 0; c < columnas && f+c < len(g.Origenes); c++ {
				cx := x + float32(c)*(col+HuecoColumnasReglas)
				altoFila = max(altoFila, r.celdaRegla(g.Origenes[f+c], cx, y, col, mover))
			}
			y += altoFila
		}
	}
	if r.conFijo() {
		y += HuecoGruposReglas
		m := r.fijo.MinSize()
		if mover {
			r.fijo.Move(fyne.NewPos(x, y))
			r.fijo.Resize(fyne.NewSize(min(m.Width, ancho), m.Height))
		}
		y += m.Height + HuecoNotaFija
		h := ui.AltoParrafo(r.notaFijo, r.notaFijo.Ancho)
		if mover {
			r.notaFijo.Move(fyne.NewPos(x+SangriaNotaFija, y))
			r.notaFijo.Resize(fyne.NewSize(r.notaFijo.Ancho, h))
		}
		y += h
	}
	return y - y0
}

// celdaRegla coloca el interruptor de un origen (o su fantasma) con, debajo, el texto
// de su guardado fallido, y devuelve lo que ocupa.
func (r *avisosRenderer) celdaRegla(o string, x, y, ancho float32, mover bool) float32 {
	if f, ok := r.fantasmas[o]; ok {
		if mover {
			f[0].Move(fyne.NewPos(x, y))
			f[0].Resize(fyne.NewSize(AnchoFantasmaRegla, AltoFantasmaRegla))
			resto := ancho - AnchoFantasmaRegla - HuecoFantasmaRegla
			f[1].Move(fyne.NewPos(x+AnchoFantasmaRegla+HuecoFantasmaRegla, y+(AltoFantasmaRegla-AltoEtiquetaFantasma)/2))
			f[1].Resize(fyne.NewSize(resto*FraccionEtiquetaFantasma, AltoEtiquetaFantasma))
		}
		return AltoFantasmaRegla
	}
	c := r.conmutadores[o]
	m := c.MinSize()
	alto := m.Height
	if mover {
		c.Move(fyne.NewPos(x, y))
		c.Resize(fyne.NewSize(min(m.Width, ancho), m.Height))
	}
	if p := r.fallosFila[o]; p != nil {
		h := ui.AltoParrafo(p, ancho)
		if mover {
			p.Move(fyne.NewPos(x, y+alto+SangriaFalloRegla))
			p.Resize(fyne.NewSize(ancho, h))
		}
		alto += SangriaFalloRegla + h
	}
	return alto
}

// ---- La fila de aviso ----

// FilaAviso es un aviso de la bandeja. A diferencia de las filas de la actividad, el
// título y el cuerpo NO se truncan: son `block` y se reparten en las líneas que hagan
// falta, así que el alto depende del ancho (AltoFilaAviso).
type FilaAviso struct {
	ui.Pulsable

	Titulo, Cuerpo, Meta string
	Marca                iconos.Producto
	Conocido             bool
	// SinLeer pone el título en `font-semibold` y deja sitio a la derecha para el botón
	// del punto, que va fuera de la fila (`BotonLeido`).
	SinLeer bool
	// Enlace es que el aviso lleva a algún sitio: en la web es un `<a>`, y si no, un
	// `<button>` que solo lo marca como leído.
	Enlace bool
	// RotuloPunto es el «Sin leer» que la fila dice para quien no ve el punto.
	RotuloPunto string
	// Ancho es el de la lista, que es el que decide en cuántas líneas cae el texto.
	Ancho float32
}

var _ fyne.Widget = (*FilaAviso)(nil)

// NuevaFilaAviso crea la fila.
func NuevaFilaAviso(titulo, cuerpo, meta string, alPulsar func()) *FilaAviso {
	f := &FilaAviso{Titulo: titulo, Cuerpo: cuerpo, Meta: meta}
	f.Nombre = titulo
	f.AlPulsar = alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// AccessibilityRole: enlace si lleva a algún sitio, botón si solo marca.
func (f *FilaAviso) AccessibilityRole() fyne.AccessibleRole {
	if f.Enlace {
		return fyne.AccessibleRoleLink
	}
	return fyne.AccessibleRoleButton
}

// AccessibilityLabel es lo que se lee de corrido en la web: título, cuerpo, origen y
// cuándo, y el «Sin leer» del punto.
func (f *FilaAviso) AccessibilityLabel() string {
	s := f.Titulo
	for _, parte := range []string{f.Cuerpo, f.Meta} {
		if parte != "" {
			s += ". " + parte
		}
	}
	if f.SinLeer && f.RotuloPunto != "" {
		s += ". " + f.RotuloPunto
	}
	return s
}

func (f *FilaAviso) peso() ui.Peso {
	if f.SinLeer {
		return ui.SemiNegrita
	}
	return ui.Medio
}

// anchoFilaAviso es lo que ocupa la fila en una lista de ese ancho: `flex-1`, menos el
// botón del punto si está sin leer.
func anchoFilaAviso(f *FilaAviso, ancho float32) float32 {
	if f.SinLeer {
		return max(ancho-LadoBotonLeido, 1)
	}
	return ancho
}

// anchoTextoAviso es lo que le queda a la columna de texto en una fila de ese ancho.
func anchoTextoAviso(ancho float32) float32 {
	return max(ancho-2*MargenXAviso-LadoMarcaAviso-HuecoAviso, 1)
}

// AltoFilaAviso es lo que ocupa una fila en una lista de ese ancho: `py-3` y lo más alto
// de la marca (con su `mt-0.5`) y la columna de texto; y, sin leer, al menos el botón
// del punto con su `mt-3`.
func AltoFilaAviso(f *FilaAviso, ancho float32) float32 {
	cabe := anchoTextoAviso(anchoFilaAviso(f, ancho))
	texto := float32(len(ui.Repartir(f.Titulo, tema.TextoSM, f.peso(), cabe))) * tema.TextoSM.Linea
	if f.Cuerpo != "" {
		texto += SangriaResumen + float32(len(ui.Repartir(f.Cuerpo, tema.TextoSM, ui.Normal, cabe)))*tema.TextoSM.Linea
	}
	texto += SangriaMetaAviso + tema.TextoXS.Linea
	alto := 2*MargenYAviso + max(texto, SangriaMarca+LadoMarcaAviso)
	if f.SinLeer {
		alto = max(alto, MargenYAviso+LadoBotonLeido)
	}
	return alto
}

// CreateRenderer compone la fila. Termina en Refresh (regla 7 de `ui.go`).
func (f *FilaAviso) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cabe := anchoTextoAviso(anchoFilaAviso(f, f.Ancho))
	r := &filaAvisoRenderer{
		f:      f,
		fondo:  canvas.NewRectangle(color.NRGBA{}),
		aro:    ui.NuevoAro(tema.RadioMD, ui.SinHueco, v),
		titulo: ui.NuevoParrafo(f.Titulo, tema.TextoSM, f.peso(), "text", v).A(cabe),
		meta:   ui.Texto(f.Meta, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
	}
	r.fondo.CornerRadius = tema.RadioMD
	r.objetos = append(r.aro.Objetos(), r.fondo)
	if f.Conocido {
		r.marca = iconos.Marca(f.Marca, LadoMarcaAviso, v)
		r.objetos = append(r.objetos, r.marca.Objeto())
	}
	r.objetos = append(r.objetos, r.titulo)
	if f.Cuerpo != "" {
		r.cuerpo = ui.NuevoParrafo(f.Cuerpo, tema.TextoSM, ui.Normal, "text-muted", v).A(cabe)
		r.objetos = append(r.objetos, r.cuerpo)
	}
	r.objetos = append(r.objetos, r.meta)
	r.Refresh()
	return r
}

// fondo es `hover:bg-chrome-hover`, como la fila de la actividad.
func (f *FilaAviso) fondo(v tema.Variante) color.NRGBA {
	if f.Encima() || f.Enfocado() {
		return tema.Color("chrome-hover", v)
	}
	return color.NRGBA{}
}

type filaAvisoRenderer struct {
	f              *FilaAviso
	fondo          *canvas.Rectangle
	aro            *ui.Aro
	marca          *iconos.Dibujo
	titulo, cuerpo *ui.Parrafo
	meta           *canvas.Text
	objetos        []fyne.CanvasObject
}

func (r *filaAvisoRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, AltoFilaAviso(r.f, r.f.Ancho))
}

func (r *filaAvisoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaAvisoRenderer) Destroy()                     {}

// Layout coloca marca y columna de texto, arriba (`items-start`).
func (r *filaAvisoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.fondo.Resize(t)
	x, y := MargenXAviso, MargenYAviso
	if r.marca != nil {
		r.marca.Colocar(x, y+SangriaMarca, LadoMarcaAviso)
	}
	x += LadoMarcaAviso + HuecoAviso
	cabe := anchoTextoAviso(t.Width)
	alto := ui.AltoParrafo(r.titulo, cabe)
	r.titulo.Move(fyne.NewPos(x, y))
	r.titulo.Resize(fyne.NewSize(cabe, alto))
	y += alto
	if r.cuerpo != nil {
		y += SangriaResumen
		alto = ui.AltoParrafo(r.cuerpo, cabe)
		r.cuerpo.Move(fyne.NewPos(x, y))
		r.cuerpo.Resize(fyne.NewSize(cabe, alto))
		y += alto
	}
	ui.Truncar(r.meta, r.f.Meta, tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(r.meta, tema.TextoXS, x, y+SangriaMetaAviso)
}

// Refresh repinta con el estado del puntero y del foco.
func (r *filaAvisoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = r.f.fondo(v)
	r.fondo.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.f.Enfocado())
	r.meta.Color = tema.Color("text-subtle", v)
	r.meta.Refresh()
}

// ---- El punto, hecho botón ----

// BotonLeido es el punto azul de un aviso sin leer como botón «Marcar como leído»: el
// `MarcarLeido` de @sytue/shell. Marca ESE aviso sin abrirlo —abrirlo navega, y «Marcar
// todas» se lleva por delante los demás—. `rounded-full p-2` con el punto de 8 en medio y
// `hover:bg-chrome-hover`.
type BotonLeido struct {
	ui.Pulsable
}

var _ fyne.Widget = (*BotonLeido)(nil)

// NuevoBotonLeido crea el botón; `rotulo` es su nombre accesible («Marcar como leído»).
func NuevoBotonLeido(rotulo string, alPulsar func()) *BotonLeido {
	b := &BotonLeido{}
	b.Nombre = rotulo
	b.AlPulsar = alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// CreateRenderer compone el círculo del puntero, el aro de foco y el punto.
func (b *BotonLeido) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonLeidoRenderer{
		b:     b,
		fondo: canvas.NewCircle(color.NRGBA{}),
		aro:   ui.NuevoAro(LadoBotonLeido/2, ui.SinHueco, v),
		punto: canvas.NewCircle(tema.Color("primary", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.fondo, r.punto)
	r.Refresh()
	return r
}

type botonLeidoRenderer struct {
	b            *BotonLeido
	fondo, punto *canvas.Circle
	aro          *ui.Aro
	objetos      []fyne.CanvasObject
}

func (r *botonLeidoRenderer) MinSize() fyne.Size {
	return fyne.NewSize(LadoBotonLeido, LadoBotonLeido)
}
func (r *botonLeidoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonLeidoRenderer) Destroy()                     {}

func (r *botonLeidoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	r.punto.Move(fyne.NewPos((t.Width-LadoPuntoSinLeer)/2, (t.Height-LadoPuntoSinLeer)/2))
	r.punto.Resize(fyne.NewSize(LadoPuntoSinLeer, LadoPuntoSinLeer))
}

// Refresh repinta con el estado del puntero y del foco.
func (r *botonLeidoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = color.NRGBA{}
	if r.b.Encima() || r.b.Enfocado() {
		r.fondo.FillColor = tema.Color("chrome-hover", v)
	}
	r.fondo.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.b.Enfocado())
	r.punto.FillColor = tema.Color("primary", v)
	r.punto.Refresh()
}

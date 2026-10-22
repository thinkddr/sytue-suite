// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// En la web: apps/frontend/apps/rooms/src/components/MeetingsPanel.tsx.
//
// El panel «Reuniones»: las salas que no cuelgan de un canal. Convocar (con invitados
// se abre el directo con ellos), entrar pegando un enlace —uno de Zoom, Meet, Teams o
// Webex se abre tal cual en el navegador— y las que están en marcha, con su enlace,
// compartir, quién está y qué se grabó.
//
// La lista es una foto al abrir, como la web (sin sondeo); se rehace al volver a abrir
// el panel, con «Reintentar» tras un fallo o con el «Reintentar» de la banda.

// MaxInvitados es el tope de la web: un directo de grupo son ocho contando a quien
// convoca.
const MaxInvitados = 7

var reUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ReunionDeEnlace es `meetingIdFromInput`: el id de lo pegado —la URL con `?rooms=` (o la
// antigua, con `meet`) o el id suelto—, o "" si no hay nada usable.
func ReunionDeEnlace(crudo string) string {
	s := strings.TrimSpace(crudo)
	if reUUID.MatchString(s) {
		return s
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" {
		return ""
	}
	for _, clave := range []string{"rooms", "meet"} {
		if id := strings.TrimSpace(u.Query().Get(clave)); reUUID.MatchString(id) {
			return id
		}
	}
	return ""
}

// EnlaceCompartir es `enlaceParaCompartir`: la URL de compartir de cada destino.
func EnlaceCompartir(destino, texto, enlace, asunto string) string {
	switch destino {
	case "whatsapp":
		return "https://wa.me/?text=" + url.QueryEscape(texto+" "+enlace)
	case "email":
		return "mailto:?subject=" + url.PathEscape(asunto) + "&body=" + url.PathEscape(texto+"\n"+enlace)
	}
	return "https://teams.microsoft.com/share?href=" + url.QueryEscape(enlace) + "&msgText=" + url.QueryEscape(texto)
}

// DuracionLegible es `humanDuration`.
func DuracionLegible(t *i18n.Translator, seg int64) string {
	if seg < 60 {
		return t.T("call.duration.underMinute", nil)
	}
	mins := (seg + 30) / 60
	if mins < 60 {
		return t.T("call.duration.minutes", i18n.Vars{"count": mins})
	}
	if mins%60 == 0 {
		return t.T("call.duration.hours", i18n.Vars{"count": mins / 60})
	}
	return t.T("call.duration.hoursMinutes", i18n.Vars{"hours": mins / 60, "minutes": mins % 60})
}

// Las medidas de MeetingsPanel.tsx. HOJA.
var (
	// CanalonReuniones es el `px-4 py-4` de cada sección.
	CanalonReuniones = tema.Espacio(4)
	// RellenoTarjetaReunion es el `p-3` de una reunión y HuecoReuniones el `gap-2` de
	// la lista.
	RellenoTarjetaReunion = tema.Espacio(3)
	HuecoReuniones        = tema.Espacio(2)
)

// HojaReuniones es el `<aside>` del panel.
type HojaReuniones struct {
	widget.BaseWidget
	T      *i18n.Translator
	Locale i18n.Locale

	Reuniones []api.RoomsMeeting
	Estado    Estado
	// Modera es `canModerate`: admin del equipo.
	Modera bool
	// DetalleDe es la reunión con el detalle abierto, con sus participantes y
	// grabaciones.
	DetalleDe       string
	Participantes   []api.RoomsMeetingParticipant
	Grabaciones     []api.RoomsMeetingRecording
	CargandoDetalle bool
	Invitados       []Persona
	// Ocupado es "create" o "join" mientras va la petición; Copiado, la reunión cuyo
	// enlace se acaba de copiar.
	Ocupado, Copiado string
	ErrorEnlace      bool
	Buscando         bool
	ErrorBuscar      string

	AlCrear  func(titulo string, invitados []Persona)
	AlUnirse func(id string)
	// AlClave recibe la clave del enlace pegado (la de una reunión suelta) antes de
	// AlUnirse, para entrar con ella.
	AlClave         func(id, clave string)
	AlAbrirExterno  func(enlace string)
	AlDetalle       func(id string)
	AlTerminar      func(id string)
	AlSacar         func(id string, p api.RoomsMeetingParticipant)
	AlCopiar        func(id string)
	AlCompartir     func(destino string, m api.RoomsMeeting)
	AlDescargar     func(id, grabacion string)
	AlBuscarPersona func(q string)
	AlReintentar    func()
	AlCerrar        func()

	titulo, invitar, enlace *ui.Entrada
	fondo, filete           *canvas.Rectangle
	cabecera                *CabeceraPanel
	cuerpo                  fyne.CanvasObject
	objetos                 []fyne.CanvasObject
}

var _ fyne.Widget = (*HojaReuniones)(nil)

// NuevaHojaReuniones crea la hoja, cargando.
func NuevaHojaReuniones(t *i18n.Translator, l i18n.Locale) *HojaReuniones {
	p := &HojaReuniones{T: t, Locale: l, Estado: Cargando}
	p.ExtendBaseWidget(p)
	p.titulo = ui.NuevaEntrada(t.T("meetings.titlePlaceholder", nil))
	p.titulo.Rotulo = t.T("meetings.titleLabel", nil)
	p.titulo.AlTeclear = alIntro(p.Crear)
	p.invitar = ui.NuevaEntrada(t.T("meetings.invitePlaceholder", nil))
	p.invitar.Rotulo = t.T("meetings.invite", nil)
	p.invitar.AlCambiar(func(string) { p.ErrorBuscar = ""; p.Refresh() })
	p.enlace = ui.NuevaEntrada(t.T("meetings.linkPlaceholder", nil))
	p.enlace.Rotulo = t.T("meetings.joinByLink", nil)
	p.enlace.AlCambiar(func(string) { p.ErrorEnlace = false; p.Refresh() })
	p.enlace.AlTeclear = alIntro(p.EntrarPorEnlace)
	return p
}

// alIntro es el `onKeyDown` de Enter de un campo.
func alIntro(f func()) func(*fyne.KeyEvent) bool {
	return func(k *fyne.KeyEvent) bool {
		if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
			return false
		}
		f()
		return true
	}
}

// Campos da los tres campos (título, invitar, enlace), para los guardianes.
func (p *HojaReuniones) Campos() (titulo, invitar, enlace *ui.Entrada) {
	return p.titulo, p.invitar, p.enlace
}

// Crear es `crear`: convoca con el título escrito y los invitados.
func (p *HojaReuniones) Crear() {
	if p.Ocupado != "" || p.AlCrear == nil {
		return
	}
	p.AlCrear(strings.TrimSpace(p.titulo.Texto()), p.Invitados)
}

// Creada vacía el formulario tras convocar.
func (p *HojaReuniones) Creada() {
	p.titulo.PonerTexto("")
	p.Invitados = nil
	p.Refresh()
}

// EntrarPorEnlace es `entrarPorEnlace`: un enlace externo se abre tal cual; uno de
// Sytue, por su id; lo demás es «no parece un enlace de reunión».
func (p *HojaReuniones) EntrarPorEnlace() {
	if p.Ocupado != "" {
		return
	}
	crudo := strings.TrimSpace(p.enlace.Texto())
	if ProveedorDeReunion(crudo) != "" {
		if p.AlAbrirExterno != nil {
			p.AlAbrirExterno(crudo)
		}
		p.enlace.PonerTexto("")
		return
	}
	id := ReunionDeEnlace(crudo)
	if id == "" {
		p.ErrorEnlace = true
		p.Refresh()
		return
	}
	if u, err := url.Parse(crudo); err == nil && p.AlClave != nil {
		p.AlClave(id, api.ClaveDeReunion(u))
	}
	if p.AlUnirse != nil {
		p.AlUnirse(id)
	}
}

// Invitar suma a alguien a los invitados, sin repetir y hasta el tope.
func (p *HojaReuniones) Invitar(per Persona) {
	p.invitar.PonerTexto("")
	for _, x := range p.Invitados {
		if x.ID == per.ID {
			return
		}
	}
	if len(p.Invitados) < MaxInvitados {
		p.Invitados = append(p.Invitados, per)
	}
	p.Refresh()
}

// AccessibilityLabel es el `aria-label` del `<aside>`.
func (p *HojaReuniones) AccessibilityLabel() string { return p.T.T("meetings.title", nil) }

// AccessibilityRole implementa fyne.Accessible.
func (p *HojaReuniones) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleContainer
}

// CreateRenderer monta el panel.
func (p *HojaReuniones) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p.fondo = canvas.NewRectangle(tema.Color("surface", v))
	p.filete = canvas.NewRectangle(tema.Color("border", v))
	// «Reintentar» en la banda, junto al aspa (`gap-0.5`): rehace la lista, que es una foto.
	reintentar := NuevoBotonBanda(p.T.T("action.retry", nil), nil, func() {
		if p.AlReintentar != nil {
			p.AlReintentar()
		}
	})
	p.cabecera = NuevaCabeceraPanel(p.T.T("meetings.title", nil), "", p.T.T("action.close", nil), p.AlCerrar, reintentar)
	p.cabecera.HuecoExtra = tema.Espacio(0.5)
	r := &hojaReunionesRenderer{p: p}
	r.Refresh()
	return r
}

// titular es un `<h3>` `text-sm font-semibold`.
func titular(s string) fyne.CanvasObject {
	return ui.NuevoRotulo(s, tema.TextoSM, ui.SemiNegrita, "text")
}

// meta es un `.sytue-meta` que se parte en líneas.
func meta(s string, v tema.Variante) *ui.Parrafo {
	return ui.NuevoParrafo(s, escalonMeta, ui.Normal, "text-muted", v)
}

// construirCuerpo son las tres secciones.
//
//nolint:gocyclo // Las secciones visibles dependen explícitamente de cada estado de reunión.
func (p *HojaReuniones) construirCuerpo(v tema.Variante) fyne.CanvasObject {
	t := p.T
	sep := func() fyne.CanvasObject { return canvas.NewRectangle(tema.Color("border-subtle", v)) }

	crearRotulo := t.T("meetings.create", nil)
	if p.Ocupado == "create" {
		crearRotulo = t.T("meetings.creating", nil)
	}
	crear := ui.NuevoBoton(crearRotulo, p.Crear).Con(ui.Primario, ui.TallaSM)
	crear.Inerte = p.Ocupado != ""
	nueva := []bloqueReunion{
		{o: titular(t.T("meetings.new", nil))},
		{o: meta(t.T("meetings.newHint", nil), v), arriba: tema.Espacio(1)},
		{o: container.New(filaFlexible{hueco: tema.Espacio(2)}, p.titulo, crear), arriba: tema.Espacio(3)},
		{o: ui.NuevoRotulo(t.T("meetings.invite", nil), tema.TextoXS, ui.Medio, "text"), arriba: tema.Espacio(3)},
		{o: p.invitar, arriba: tema.Espacio(1)},
		{o: meta(t.T("meetings.inviteHint", nil), v), arriba: tema.Espacio(1)},
	}
	if q := strings.TrimSpace(p.invitar.Texto()); PareceIdentificador(q) {
		rotulo := t.T("people.search", i18n.Vars{"query": q})
		if p.Buscando {
			rotulo = t.T("people.searching", nil)
		}
		b := NuevaFilaBuscarPersona(rotulo, func() {
			if p.AlBuscarPersona != nil {
				p.AlBuscarPersona(q)
			}
		})
		b.Inerte = p.Buscando || p.Ocupado != "" || len(p.Invitados) >= MaxInvitados
		nueva = append(nueva, bloqueReunion{o: b, arriba: tema.Espacio(1)})
		if p.ErrorBuscar != "" {
			nueva = append(nueva, bloqueReunion{o: ui.NuevoRotulo(p.ErrorBuscar, tema.TextoSM, ui.Normal, "danger-text"), arriba: tema.Espacio(1)})
		}
	}
	if len(p.Invitados) > 0 {
		chips := make([]fyne.CanvasObject, 0, len(p.Invitados))
		for _, per := range p.Invitados {
			quitar := func() {
				p.Invitados = slices.DeleteFunc(p.Invitados, func(x Persona) bool { return x.ID == per.ID })
				p.Refresh()
			}
			chips = append(chips, NuevoChipQuitable(per.Nombre, t.T("action.removePerson", i18n.Vars{"name": per.Nombre}), nil, quitar))
		}
		nueva = append(nueva, bloqueReunion{o: container.New(&filaEnvuelta{hueco: tema.Espacio(1.5)}, chips...), arriba: tema.Espacio(2)})
	}

	entrarRotulo := t.T("meetings.join", nil)
	if p.Ocupado == "join" {
		entrarRotulo = t.T("meetings.joining", nil)
	}
	entrar := ui.NuevoBoton(entrarRotulo, p.EntrarPorEnlace).Con(ui.Secundario, ui.TallaSM)
	entrar.Inerte = p.Ocupado != ""
	enlace := []bloqueReunion{
		{o: titular(t.T("meetings.joinByLink", nil))},
		{o: meta(t.T("meetings.joinByLinkHint", nil), v), arriba: tema.Espacio(1)},
		{o: container.New(filaFlexible{hueco: tema.Espacio(2)}, p.enlace, entrar), arriba: tema.Espacio(3)},
	}
	switch prov := ProveedorDeReunion(p.enlace.Texto()); {
	case p.ErrorEnlace:
		enlace = append(enlace, bloqueReunion{o: ui.NuevoRotulo(t.T("meetings.linkInvalid", nil), tema.TextoXS, ui.Normal, "danger-text"), arriba: tema.Espacio(2)})
	case prov != "":
		enlace = append(enlace, bloqueReunion{o: meta(t.T("meetings.external", i18n.Vars{"provider": nombresProveedor[prov]}), v), arriba: tema.Espacio(2)})
	}

	activas := []bloqueReunion{{o: titular(t.T("meetings.active", nil))}}
	switch {
	case p.Estado == Error:
		activas = append(activas, bloqueReunion{o: NuevoAvisoHilo(t.T("error.loadFailed", nil), t.T("action.retry", nil), p.AlReintentar), arriba: tema.Espacio(3)})
	case p.Estado == Cargando:
		e := NuevoEsqueletoHilo(t.T("state.loading", nil))
		e.Partes = []float32{0.70, 0.55}
		activas = append(activas, bloqueReunion{o: e, arriba: tema.Espacio(3)})
	case len(p.Reuniones) == 0:
		vac := ui.NuevoVacio(t.T("meetings.empty", nil), t.T("meetings.emptyHint", nil))
		vac.Ilustracion = iconos.Pintar(iconos.VacioCalendario, v).A(LadoIlustracionBusqueda).Objeto()
		activas = append(activas, bloqueReunion{o: vac, arriba: tema.Espacio(3)})
	default:
		for i, m := range p.Reuniones {
			hueco := HuecoReuniones
			if i == 0 {
				hueco = tema.Espacio(3)
			}
			activas = append(activas, bloqueReunion{o: p.tarjeta(m, v), arriba: hueco})
		}
	}
	col := &columnaReuniones{secciones: [][]bloqueReunion{nueva, enlace, activas}}
	objs := []fyne.CanvasObject{}
	for i, s := range col.secciones {
		for _, b := range s {
			objs = append(objs, b.o)
		}
		if i < len(col.secciones)-1 {
			col.seps = append(col.seps, sep())
			objs = append(objs, col.seps[i])
		}
	}
	return container.NewVScroll(container.New(col, objs...))
}

// tarjeta es una reunión en marcha: `rounded-lg border p-3`.
func (p *HojaReuniones) tarjeta(m api.RoomsMeeting, v tema.Variante) fyne.CanvasObject {
	t := p.T
	boton := func(s, token string, f func()) fyne.CanvasObject {
		return ui.NuevoEnlace(s, escalonMeta, ui.Normal, token, f)
	}
	copiar := t.T("meetings.copyLink", nil)
	if p.Copiado == m.ID {
		copiar = t.T("meetings.linkCopied", nil)
	}
	acciones := []fyne.CanvasObject{
		boton(copiar, "text-muted", func() {
			if p.AlCopiar != nil {
				p.AlCopiar(m.ID)
			}
		}),
	}
	var compartir fyne.CanvasObject
	compartir = boton(t.T("meetings.share", nil), "text-muted", func() {
		es := make([]fyne.CanvasObject, 0, 3)
		for _, d := range []string{"whatsapp", "email", "teams"} {
			es = append(es, ui.NuevoElementoMenu(t.T("meetings.share."+d, nil), iconos.Ninguno, func() {
				if p.AlCompartir != nil {
					p.AlCompartir(d, m)
				}
			}))
		}
		menu := ui.NuevoMenu(es...)
		menu.Rotulo, menu.AlInicio = t.T("meetings.share", nil), true
		menu.Mostrar(compartir)
	})
	acciones = append(acciones, compartir, boton(t.T("meetings.details", nil), "text-muted", func() {
		if p.AlDetalle != nil {
			p.AlDetalle(m.ID)
		}
	}))
	if p.Modera {
		acciones = append(acciones, boton(t.T("meetings.close", nil), "danger-text", func() {
			if p.AlTerminar != nil {
				p.AlTerminar(m.ID)
			}
		}))
	}
	cuando := ""
	if m.StartedAt != nil {
		cuando = t.T("meetings.startedAt", i18n.Vars{"time": i18n.FormatTime(p.Locale, m.StartedAt.Local())})
	}
	unirse := ui.NuevoBoton(t.T("meetings.join", nil), func() {
		if p.AlUnirse != nil {
			p.AlUnirse(m.ID)
		}
	}).Con(ui.Primario, ui.TallaSM)
	cabeza := container.New(filaFlexible{hueco: tema.Espacio(2)},
		container.New(ui.Columna{Hueco: tema.Espacio(0.5)},
			ui.NuevoRotulo(m.Titulo(), tema.TextoSM, ui.Medio, "text"),
			ui.NuevoRotulo(cuando, escalonMeta, ui.Normal, "text-muted")),
		unirse)
	partes := []fyne.CanvasObject{cabeza, container.New(&filaEnvuelta{hueco: tema.Espacio(1)}, acciones...)}
	if p.DetalleDe == m.ID {
		partes = append(partes, p.detalle(m, v))
	}
	caja := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border-subtle", v), tema.RadioLG)
	dentro := container.New(ui.Margen{Izq: RellenoTarjetaReunion, Arr: RellenoTarjetaReunion, Der: RellenoTarjetaReunion, Abj: RellenoTarjetaReunion},
		container.New(ui.Columna{Hueco: tema.Espacio(2)}, partes...))
	return container.New(enCaja{caja}, append(caja.Objetos(), dentro)...)
}

// detalle es el plegable: quién está y qué se grabó.
func (p *HojaReuniones) detalle(m api.RoomsMeeting, v tema.Variante) fyne.CanvasObject {
	t := p.T
	var filas []fyne.CanvasObject
	filas = append(filas, canvas.NewRectangle(tema.Color("border-subtle", v)))
	if p.CargandoDetalle {
		filas = append(filas, ui.NuevoEsqueleto(0, 0.875*tema.Rem))
		return container.New(ui.Columna{Hueco: tema.Espacio(3)}, filas...)
	}
	filas = append(filas, ui.NuevoRotulo(t.T("meetings.who", nil)+" · "+t.T("meetings.participants", i18n.Vars{"count": len(p.Participantes)}),
		escalonMeta, ui.SemiNegrita, "text-muted"))
	for _, per := range p.Participantes {
		nombre := ui.NuevoRotulo(nombreDe(per.Name, per.Identity), tema.TextoXS, ui.Normal, "text")
		if !p.Modera {
			filas = append(filas, nombre)
			continue
		}
		sacar := ui.NuevoBotonIcono(iconos.Cerrar, t.T("meetings.remove", nil), func() {
			if p.AlSacar != nil {
				p.AlSacar(m.ID, per)
			}
		})
		filas = append(filas, container.New(filaFlexible{hueco: tema.Espacio(2)}, nombre, sacar))
	}
	filas = append(filas, ui.NuevoRotulo(t.T("meetings.recordings", nil), escalonMeta, ui.SemiNegrita, "text-muted"))
	if len(p.Grabaciones) == 0 {
		filas = append(filas, ui.NuevoRotulo(t.T("meetings.noRecordings", nil), escalonMeta, ui.Normal, "text-muted"))
	}
	for _, g := range p.Grabaciones {
		switch g.Status {
		case "completed":
			tam := "—"
			if g.SizeBytes > 0 {
				tam = i18n.FormatBytes(t.Locale(), float64(g.SizeBytes))
			}
			s := t.T("action.download", nil) + " · " + t.T("meetings.recordingSize", i18n.Vars{"size": tam, "duration": DuracionLegible(t, g.DurationSec)})
			filas = append(filas, ui.NuevoEnlace(s, tema.TextoXS, ui.SemiNegrita, "primary-text", func() {
				if p.AlDescargar != nil {
					p.AlDescargar(m.ID, g.ID)
				}
			}))
		case "failed":
			filas = append(filas, ui.NuevoRotulo(t.T("meetings.recordingFailed", nil), tema.TextoXS, ui.Normal, "text"))
		default:
			filas = append(filas, ui.NuevoRotulo(t.T("meetings.recordingPending", nil), tema.TextoXS, ui.Normal, "text"))
		}
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(1)}, filas...)
}

type hojaReunionesRenderer struct{ p *HojaReuniones }

func (r *hojaReunionesRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, AltoCabecera) }
func (r *hojaReunionesRenderer) Objects() []fyne.CanvasObject { return r.p.objetos }
func (r *hojaReunionesRenderer) Destroy()                     {}

func (r *hojaReunionesRenderer) Layout(t fyne.Size) {
	p := r.p
	p.fondo.Resize(t)
	p.filete.Resize(fyne.NewSize(tema.Borde, t.Height))
	x, ancho := tema.Borde, max(t.Width-tema.Borde, 0)
	p.cabecera.Move(fyne.NewPos(x, 0))
	p.cabecera.Resize(fyne.NewSize(ancho, AltoCabecera))
	p.cuerpo.Move(fyne.NewPos(x, AltoCabecera))
	p.cuerpo.Resize(fyne.NewSize(ancho, max(t.Height-AltoCabecera, 0)))
}

func (r *hojaReunionesRenderer) Refresh() {
	p := r.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p.fondo.FillColor = tema.Color("surface", v)
	p.filete.FillColor = tema.Color("border", v)
	p.fondo.Refresh()
	p.filete.Refresh()
	p.cabecera.Refresh()
	p.cuerpo = p.construirCuerpo(v)
	p.objetos = []fyne.CanvasObject{p.fondo, p.cuerpo, p.cabecera, p.filete}
	r.Layout(p.Size())
}

// ---- Maquetación ----

// bloqueReunion es un hijo de una sección con su margen de arriba.
type bloqueReunion struct {
	o      fyne.CanvasObject
	arriba float32
}

// columnaReuniones apila las secciones (`px-4 py-4`, `border-b` entre ellas) midiendo
// cada hijo al ancho de la columna: un párrafo, a su número de líneas.
type columnaReuniones struct {
	secciones [][]bloqueReunion
	seps      []fyne.CanvasObject
	ancho     float32
}

func (c *columnaReuniones) altoDe(o fyne.CanvasObject, ancho float32) float32 {
	if pa, ok := o.(*ui.Parrafo); ok {
		return ui.AltoParrafo(pa, ancho)
	}
	return o.MinSize().Height
}

func (c *columnaReuniones) colocar(ancho float32, mover bool) float32 {
	dentro := max(ancho-2*CanalonReuniones, 0)
	var y float32
	for i, s := range c.secciones {
		y += CanalonReuniones
		for _, b := range s {
			y += b.arriba
			h := c.altoDe(b.o, dentro)
			if mover {
				b.o.Move(fyne.NewPos(CanalonReuniones, y))
				b.o.Resize(fyne.NewSize(dentro, h))
			}
			y += h
		}
		y += CanalonReuniones
		if i < len(c.seps) {
			if mover {
				c.seps[i].Move(fyne.NewPos(0, y))
				c.seps[i].Resize(fyne.NewSize(ancho, tema.Borde))
			}
			y += tema.Borde
		}
	}
	return y
}

func (c *columnaReuniones) MinSize([]fyne.CanvasObject) fyne.Size {
	ancho := c.ancho
	if ancho == 0 {
		ancho = AnchoPanelDerecho
	}
	return fyne.NewSize(0, c.colocar(ancho, false))
}

func (c *columnaReuniones) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	c.ancho = t.Width
	c.colocar(t.Width, true)
}

// enCaja pone la caja debajo del último hijo, a su tamaño.
type enCaja struct{ caja *ui.Caja }

func (e enCaja) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[len(objs)-1].MinSize() }

func (e enCaja) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	e.caja.Redimensionar(t)
	objs[len(objs)-1].Resize(t)
}

// filaFlexible es `flex items-center gap-2` con el primer hijo en `flex-1`: los demás a
// su ancho, a la derecha, y todos centrados en alto.
type filaFlexible struct{ hueco float32 }

func (f filaFlexible) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, o := range objs {
		m := o.MinSize()
		if i > 0 {
			ancho += f.hueco + m.Width
		}
		alto = max(alto, m.Height)
	}
	return fyne.NewSize(ancho, alto)
}

func (f filaFlexible) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	der := t.Width
	for i := len(objs) - 1; i > 0; i-- {
		m := objs[i].MinSize()
		der -= m.Width
		objs[i].Move(fyne.NewPos(der, (t.Height-m.Height)/2))
		objs[i].Resize(m)
		der -= f.hueco
	}
	if len(objs) > 0 {
		m := objs[0].MinSize()
		objs[0].Move(fyne.NewPos(0, (t.Height-m.Height)/2))
		objs[0].Resize(fyne.NewSize(max(der, 0), m.Height))
	}
}

// ---- En la Sala ----

// Reunidor es lo que el panel pide al servicio; lo cumple *api.Rooms.
type Reunidor interface {
	Meetings(ctx context.Context) ([]api.RoomsMeeting, error)
	CreateMeeting(ctx context.Context, titulo string) (api.RoomsMeeting, error)
	CloseMeeting(ctx context.Context, id string) error
	MeetingParticipants(ctx context.Context, id string) ([]api.RoomsMeetingParticipant, error)
	MeetingRecordings(ctx context.Context, id string) ([]api.RoomsMeetingRecording, error)
	RecordingURL(ctx context.Context, id, grabacion string) (string, error)
	RemoveMeetingParticipant(ctx context.Context, id, identidad string) error
	KnockMeeting(ctx context.Context, id, clave string) error
}

// EnlaceReunion es `meetingLink`: lo que se copia para convocar, con la clave de la
// reunión suelta si se conoce (sin ella solo entran quien la convocó y quien ya estuvo).
func (sa *Sala) EnlaceReunion(id string) string {
	return api.EnlaceReunion(api.ServiceURL(sa.Apex, "rooms"), id, sa.claves[id])
}

// recordarClave apunta la clave del enlace de una reunión suelta.
func (sa *Sala) recordarClave(id, clave string) {
	if clave == "" {
		return
	}
	if sa.claves == nil {
		sa.claves = map[string]string{}
	}
	sa.claves[id] = clave
}

// puertaCerrada es el aviso de una reunión bloqueada con su «Pedir entrar»: llamar
// avisa al anfitrión; si entretanto la han abierto (409), se entra sin más.
func (sa *Sala) puertaCerrada(re Reunidor, id string) {
	t := sa.P.T
	a := ui.NuevoAviso(ui.AvisoInfo, t.T("meet.roomLockedNotice", nil), "")
	a.RotuloAccion = t.T("meet.knock", nil)
	a.AlPulsarAccion = func() {
		clave := sa.claves[id]
		pedir(sa, func(ctx context.Context) (struct{}, error) { return struct{}{}, re.KnockMeeting(ctx, id, clave) },
			func(_ struct{}, err error) {
				var e *api.Error
				switch {
				case err == nil:
					sa.P.Avisar(ui.AvisoExito, t.T("meet.knocked", nil))
				case errors.As(err, &e) && e.Status == http.StatusConflict:
					sa.abrirURL(sa.EnlaceReunion(id))
					sa.cerrarDerecho()
				default:
					sa.P.Avisar(ui.AvisoPeligro, t.T("meetings.joinFailed", nil))
				}
			})
	}
	sa.P.Avisos.Notificar(a)
}

// abrirURL abre un enlace en el navegador del sistema.
func (sa *Sala) abrirURL(crudo string) {
	if u, err := url.Parse(crudo); err == nil && sa.AbrirEnlace != nil {
		_ = sa.AbrirEnlace(u)
	}
}

// abrirReuniones monta el panel y pide la lista.
//
// ponytail: la sala se abre en la web (`?rooms=`): la llamada nativa no es de este
// panel. Con invitados se abre el directo con ellos, como la web, y la llamada se
// empieza desde su «Llamar».
//
//nolint:gocognit,gocyclo // El panel conecta sus acciones con cada transición asíncrona de la reunión.
func (sa *Sala) abrirReuniones() {
	re, ok := sa.S.(Reunidor)
	if !ok {
		return
	}
	sa.CerrarHilo()
	t := sa.P.T
	h := NuevaHojaReuniones(t, sa.P.Locale)
	h.Modera = sa.adminEquipo
	h.AlCerrar = sa.cerrarDerecho
	cargar := func() {
		h.Estado = Cargando
		h.Refresh()
		pedir(sa, re.Meetings, func(ms []api.RoomsMeeting, err error) {
			h.Estado, h.Reuniones = Listo, ms
			for _, m := range ms {
				sa.recordarClave(m.ID, m.LinkKey)
			}
			if err != nil {
				h.Estado = Error
			}
			h.Refresh()
		})
	}
	h.AlReintentar = cargar
	entrar := func(id string) {
		// Con la llamada nativa se entra aquí mismo; su antesala ya sabe de puertas.
		titulo := ""
		if i := slices.IndexFunc(h.Reuniones, func(m api.RoomsMeeting) bool { return m.ID == id }); i >= 0 {
			titulo = h.Reuniones[i].Titulo()
		}
		if sa.EntrarEnReunion(id, titulo) {
			sa.cerrarDerecho()
			return
		}
		// Una sala con la puerta echada no es una avería: tiene respuesta. Quien la
		// cerró y los admin pasan; los demás, a llamar (la web lo averigua al entrar;
		// aquí lo dice la lista, que es de donde se entra).
		if i := slices.IndexFunc(h.Reuniones, func(m api.RoomsMeeting) bool { return m.ID == id }); i >= 0 {
			if m := h.Reuniones[i]; m.Locked && m.CreatedBy != sa.Yo && !sa.adminEquipo {
				sa.puertaCerrada(re, id)
				return
			}
		}
		sa.abrirURL(sa.EnlaceReunion(id))
		sa.cerrarDerecho()
	}
	h.AlUnirse, h.AlAbrirExterno, h.AlClave = entrar, sa.abrirURL, sa.recordarClave
	h.AlCrear = func(titulo string, invitados []Persona) {
		h.Ocupado = "create"
		h.Refresh()
		if len(invitados) > 0 {
			ids, nombres := make([]string, 0, len(invitados)), make([]string, 0, len(invitados))
			for _, p := range invitados {
				ids, nombres = append(ids, p.ID), append(nombres, p.Nombre)
			}
			equipo := sa.equipo
			pedir(sa, func(ctx context.Context) (api.RoomsChannel, error) {
				return sa.S.OpenDirect(ctx, equipo, ids)
			}, func(canal api.RoomsChannel, err error) {
				h.Ocupado = ""
				if err != nil {
					h.Refresh()
					sa.P.Avisar(ui.AvisoPeligro, t.T("meetings.createFailed", nil))
					return
				}
				h.Creada()
				canal.Name, canal.Peers = strings.Join(nombres, ", "), nombres
				sa.cerrarDerecho()
				sa.entrarEn(canal)
			})
			return
		}
		pedir(sa, func(ctx context.Context) (api.RoomsMeeting, error) {
			return re.CreateMeeting(ctx, titulo)
		}, func(m api.RoomsMeeting, err error) {
			h.Ocupado = ""
			if err != nil {
				h.Refresh()
				sa.P.Avisar(ui.AvisoPeligro, t.T("meetings.createFailed", nil))
				return
			}
			h.Creada()
			sa.recordarClave(m.ID, m.LinkKey)
			entrar(m.ID)
		})
	}
	h.AlCopiar = func(id string) {
		sa.Copiar(sa.EnlaceReunion(id))
		h.Copiado = id
		h.Refresh()
		sa.Esperar(2*time.Second, func() {
			if h.Copiado == id {
				h.Copiado = ""
				h.Refresh()
			}
		})
	}
	h.AlCompartir = func(destino string, m api.RoomsMeeting) {
		texto, enlace := t.T("meetings.shareText", i18n.Vars{"title": m.Titulo()}), sa.EnlaceReunion(m.ID)
		if destino == "email" {
			// El correo lo compone la app: no es una dirección de fuera (`api.AbrirFuera`
			// solo admite http y https).
			if sa.Abridor != nil {
				_ = api.AbrirCorreo(sa.Abridor, "", m.Titulo(), texto+"\n"+enlace)
			}
			return
		}
		sa.abrirURL(EnlaceCompartir(destino, texto, enlace, m.Titulo()))
	}
	h.AlDetalle = func(id string) {
		if h.DetalleDe == id {
			h.DetalleDe = ""
			h.Refresh()
			return
		}
		h.DetalleDe, h.CargandoDetalle = id, true
		h.Refresh()
		type detalle struct {
			ps []api.RoomsMeetingParticipant
			gs []api.RoomsMeetingRecording
		}
		pedir(sa, func(ctx context.Context) (detalle, error) {
			ps, err := re.MeetingParticipants(ctx, id)
			if err != nil {
				return detalle{}, err
			}
			gs, err := re.MeetingRecordings(ctx, id)
			return detalle{ps, gs}, err
		}, func(d detalle, _ error) {
			h.CargandoDetalle, h.Participantes, h.Grabaciones = false, d.ps, d.gs
			h.Refresh()
		})
	}
	h.AlTerminar = func(id string) {
		sa.confirmar(t.T("meetings.closeConfirm", nil), t.T("meetings.close", nil), true, func() {
			pedir(sa, func(ctx context.Context) (struct{}, error) {
				return struct{}{}, re.CloseMeeting(ctx, id)
			}, func(_ struct{}, err error) {
				if err != nil {
					h.Estado = Error
				} else {
					h.Reuniones = slices.DeleteFunc(h.Reuniones, func(m api.RoomsMeeting) bool { return m.ID == id })
				}
				h.Refresh()
			})
		})
	}
	h.AlSacar = func(id string, per api.RoomsMeetingParticipant) {
		nombre := nombreDe(per.Name, per.Identity)
		sa.confirmar(t.T("meetings.removeConfirm", i18n.Vars{"name": nombre}), t.T("meetings.remove", nil), true, func() {
			pedir(sa, func(ctx context.Context) (struct{}, error) {
				return struct{}{}, re.RemoveMeetingParticipant(ctx, id, per.Identity)
			}, func(_ struct{}, err error) {
				if err == nil {
					h.Participantes = slices.DeleteFunc(h.Participantes, func(x api.RoomsMeetingParticipant) bool { return x.Identity == per.Identity })
					h.Refresh()
				}
			})
		})
	}
	h.AlDescargar = func(id, grabacion string) {
		pedir(sa, func(ctx context.Context) (string, error) {
			return re.RecordingURL(ctx, id, grabacion)
		}, func(u string, err error) {
			if err == nil {
				sa.abrirURL(u)
			}
		})
	}
	h.AlBuscarPersona = func(q string) {
		h.Buscando, h.ErrorBuscar = true, ""
		h.Refresh()
		pedir(sa, func(ctx context.Context) (api.RoomsPerson, error) {
			return sa.S.ResolvePerson(ctx, q)
		}, func(per api.RoomsPerson, err error) {
			h.Buscando = false
			if err != nil {
				h.ErrorBuscar = t.T(ClaveFalloBusqueda(err), nil)
				h.Refresh()
				return
			}
			if per.UserID == sa.Yo {
				h.Refresh()
				return
			}
			nombre := per.Name
			if per.Username != "" {
				nombre = nombreDe(per.Name, per.UserID) + " · @" + per.Username
			}
			h.Invitar(Persona{ID: per.UserID, Nombre: nombreDe(nombre, per.UserID)})
		})
	}
	sa.lista, sa.miembros, sa.panel, sa.reuniones = nil, nil, PanelReuniones, h
	sa.P.Canal.Cabecera.Panel = ""
	sa.soltarHilos()
	sa.P.Lateral.Panel = PanelReuniones
	sa.P.Refrescar()
	sa.P.PonerDerecho(h)
	cargar()
}

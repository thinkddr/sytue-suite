// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"errors"
	"image/color"
	"net/http"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/text/collate"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Contrapartida de MembersPanel.tsx y lib/roster.ts dentro de la pantalla de canal
// declarada en pantalla.go.
//
// El panel de «Miembros» del canal: la lista con su buscador, el recuento, la marca de
// admin y de otra organización y, para quien puede administrar el canal, el «…» de cada
// persona (hacer o quitar admin, quitar del canal) y «Añadir personas», que cambia el
// panel a la lista del equipo con el buscador de @usuario (ADR 0040).

// Miembro es alguien del roster del canal, o del equipo.
type Miembro struct {
	ID, Nombre string
	Admin      bool
	Externo    bool
	// Presencia es la que sabe el Hub; sin ella el avatar va sin punto.
	Presencia ui.Presencia
}

// FiltrarMiembros es `filterMembers`: por nombre o por identificador, sin mayúsculas.
func FiltrarMiembros(lista []Miembro, q string) []Miembro {
	q = buscar.Plegar(strings.TrimSpace(q))
	if q == "" {
		return lista
	}
	var out []Miembro
	for _, m := range lista {
		if strings.Contains(buscar.Plegar(m.Nombre), q) || strings.Contains(buscar.Plegar(m.ID), q) {
			out = append(out, m)
		}
	}
	return out
}

// OrdenarMiembros es `byName`: el nombre sin distinguir mayúsculas ni acentos.
func OrdenarMiembros(l i18n.Locale, lista []Miembro) {
	col := collate.New(l.Tag(), collate.Loose)
	slices.SortStableFunc(lista, func(a, b Miembro) int { return col.CompareString(a.Nombre, b.Nombre) })
}

// Anadibles es `addableMembers`: la gente del equipo que aún no está en el canal.
func Anadibles(l i18n.Locale, equipo, roster []Miembro, q string) []Miembro {
	dentro := map[string]bool{}
	for _, m := range roster {
		dentro[m.ID] = true
	}
	var fuera []Miembro
	for _, m := range equipo {
		if !dentro[m.ID] {
			fuera = append(fuera, m)
		}
	}
	out := FiltrarMiembros(fuera, q)
	OrdenarMiembros(l, out)
	return out
}

// ClaveErrorMiembros es `actionMessage`: 403, 409 y 404 tienen su texto.
func ClaveErrorMiembros(err error) string {
	var e *api.Error
	if errors.As(err, &e) {
		switch e.Status {
		case http.StatusForbidden:
			return "members.errorForbidden"
		case http.StatusConflict:
			return "members.errorArchived"
		case http.StatusNotFound:
			return "members.errorGone"
		}
	}
	return "members.errorGeneric"
}

// PanelRoster es el panel de «Miembros» (PanelMiembros es su clave).
type PanelRoster struct {
	widget.BaseWidget

	T      *i18n.Translator
	Locale i18n.Locale

	NombreCanal string
	Roster      []Miembro
	Equipo      []Miembro
	Estado      Estado
	// Gestiona es `canManage`: admin del canal, o del equipo si el canal es de la casa.
	Gestiona bool
	Yo       string
	// Anadiendo es el modo «Añadir personas»; Ocupado, la persona con una acción en
	// camino; Aviso y ErrorAccion, el resultado de la última.
	Anadiendo          bool
	Ocupado            string
	Aviso, ErrorAccion string
	// Buscando y ErrorBuscar son los de «Buscar X en Sytue One».
	Buscando    bool
	ErrorBuscar string

	AlAnadir func(Miembro)
	AlQuitar func(Miembro)
	// Denunciar y bloquear (ADR 0073): de CUALQUIERA sobre cualquier otra persona, no
	// solo de quien gestiona. nil, sin esas entradas.
	AlDenunciar     func(Miembro)
	AlBloquear      func(Miembro)
	Bloqueado       func(persona string) bool
	AlRol           func(m Miembro, admin bool)
	AlBuscarPersona func(q string)
	AlCerrar        func()
	AlReintentar    func()

	caja     *ui.Entrada
	consulta string
	anadir   *ui.Boton
	fondo    *canvas.Rectangle
	filete   *canvas.Rectangle
	sep      *canvas.Rectangle
	cabecera *CabeceraPanel
	arriba   *fyne.Container
	estado   *canvas.Text
	cuerpo   fyne.CanvasObject
	objetos  []fyne.CanvasObject
}

var _ fyne.Widget = (*PanelRoster)(nil)

// NuevoPanelRoster crea el panel, cargando.
func NuevoPanelRoster(t *i18n.Translator, l i18n.Locale) *PanelRoster {
	p := &PanelRoster{T: t, Locale: l, Estado: Cargando}
	p.ExtendBaseWidget(p)
	p.nuevaCaja()
	return p
}

// nuevaCaja pone la caja de buscar del modo de ahora: cada modo tiene su marca de agua,
// y la entrada de la suite la fija al nacer.
func (p *PanelRoster) nuevaCaja() {
	marca := p.T.T("members.searchMembers", nil)
	if p.Anadiendo {
		marca = p.T.T("members.searchTeam", nil)
	}
	p.consulta = ""
	p.caja = ui.NuevaEntrada(marca)
	p.caja.Icono = iconos.Buscar
	p.caja.AlCambiar(func(s string) {
		p.consulta = s
		p.ErrorBuscar = ""
		p.Refresh()
	})
	p.caja.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name != fyne.KeyEscape {
			return false
		}
		if p.Anadiendo {
			p.CambiarModo(false)
		} else if p.AlCerrar != nil {
			p.AlCerrar()
		}
		return true
	}
}

// Caja es la de buscar.
func (p *PanelRoster) Caja() *ui.Entrada { return p.caja }

// CambiarModo es `switchMode`: vacía la búsqueda y el resultado de la última acción.
func (p *PanelRoster) CambiarModo(anadir bool) {
	p.Anadiendo, p.Aviso, p.ErrorAccion = anadir, "", ""
	p.nuevaCaja()
	p.Refresh()
	if c := fyne.CurrentApp().Driver().CanvasForObject(p); c != nil {
		c.Focus(p.caja)
	}
}

// AccessibilityLabel es el `aria-label` del `<aside>`.
func (p *PanelRoster) AccessibilityLabel() string { return p.T.T("members.aria", nil) }

// AccessibilityRole implementa fyne.Accessible.
func (p *PanelRoster) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// Visibles son las filas de ahora: el roster filtrado, o los que se pueden añadir.
func (p *PanelRoster) Visibles() []Miembro {
	if p.Anadiendo {
		return Anadibles(p.Locale, p.Equipo, p.Roster, p.consulta)
	}
	return FiltrarMiembros(p.Roster, p.consulta)
}

func (p *PanelRoster) textoEstado() (string, bool) {
	t := p.T
	switch {
	case p.ErrorAccion != "":
		return p.ErrorAccion, true
	case p.Aviso != "":
		return p.Aviso, false
	case p.Estado == Cargando:
		return t.T("members.loading", nil), false
	case p.Estado == Error:
		return t.T("members.loadFailed", nil), false
	case p.Anadiendo:
		return t.T("members.availableCount", i18n.Vars{"count": len(p.Visibles())}), false
	}
	return t.T("members.count", i18n.Vars{"count": len(p.Roster)}), false
}

func (p *PanelRoster) construirArriba() *fyne.Container {
	t := p.T
	hijos := []fyne.CanvasObject{p.caja}
	q := strings.TrimSpace(p.consulta)
	if p.Anadiendo && PareceIdentificador(q) {
		rotulo := t.T("people.search", i18n.Vars{"query": q})
		if p.Buscando {
			rotulo = t.T("people.searching", nil)
		}
		b := NuevaFilaBuscarPersona(rotulo, func() {
			if p.AlBuscarPersona != nil {
				p.AlBuscarPersona(q)
			}
		})
		b.Inerte = p.Buscando || p.Ocupado != ""
		hijos = append(hijos, b)
		if p.ErrorBuscar != "" {
			hijos = append(hijos, ui.NuevoRotulo(p.ErrorBuscar, tema.TextoSM, ui.Normal, "danger-text"))
		}
	}
	p.anadir = nil
	if p.Gestiona && !p.Anadiendo {
		p.anadir = ui.NuevoBoton(t.T("members.addPeople", nil), func() { p.CambiarModo(true) }).
			Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Mas, iconos.TallaSM)
		hijos = append(hijos, p.anadir)
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, hijos...)
}

func (p *PanelRoster) construirCuerpo() fyne.CanvasObject {
	t := p.T
	var cosas []fyne.CanvasObject
	vis := p.Visibles()
	switch {
	case p.Estado == Error:
		cosas = append(cosas, NuevoAvisoHilo(t.T("members.loadFailed", nil), t.T("action.retry", nil), p.AlReintentar))
	case p.Estado == Cargando:
		cosas = append(cosas, nuevoEsqueletoMiembros())
	case p.Anadiendo && len(vis) == 0 && PareceIdentificador(p.consulta):
	case p.Anadiendo && len(vis) == 0:
		clave := "members.teamAllIn"
		switch {
		case len(p.Equipo) == 0:
			clave = "members.teamLoadFailed"
		case strings.TrimSpace(p.consulta) != "":
			clave = "members.teamEmptyQuery"
		}
		cosas = append(cosas, NuevoTextoCentrado(t.T(clave, nil)))
	case len(vis) == 0:
		clave := "members.empty"
		if len(p.Roster) > 0 {
			clave = "members.emptyQuery"
		}
		cosas = append(cosas, NuevoTextoCentrado(t.T(clave, nil)))
	default:
		for _, m := range vis {
			cosas = append(cosas, p.fila(m))
		}
	}
	cosas = append(cosas, hueco(tema.Espacio(2)))
	return container.NewVScroll(container.New(ui.Columna{}, cosas...))
}

func (p *PanelRoster) fila(m Miembro) fyne.CanvasObject {
	t := p.T
	if p.Anadiendo {
		cola := t.T("action.add", nil)
		if p.Ocupado == m.ID {
			cola = t.T("members.adding", nil)
		}
		f := NuevaFilaMiembro(m, "", cola, nil)
		f.Nombre, f.AlPulsar, f.Inerte = m.Nombre, func() { p.AlAnadir(m) }, p.Ocupado == m.ID
		return f
	}
	yo := ""
	if m.ID == p.Yo {
		yo = t.T("members.you", nil)
	}
	var insignias []string
	if m.Externo {
		insignias = append(insignias, t.T("people.external", nil))
	}
	if m.Admin {
		insignias = append(insignias, t.T("members.admin", nil))
	}
	f := NuevaFilaMiembro(m, yo, "", insignias)
	f.Nombre = m.Nombre
	if m.ID != p.Yo && (p.Gestiona || p.AlDenunciar != nil) {
		f.Menu = func() []fyne.CanvasObject {
			var out []fyne.CanvasObject
			if p.Gestiona {
				rol := t.T("members.makeAdmin", nil)
				if m.Admin {
					rol = t.T("members.removeAdmin", nil)
				}
				quitar := ui.NuevoElementoMenu(t.T("members.removeFromChannel", nil), iconos.Ninguno, func() { p.AlQuitar(m) })
				quitar.Peligro = true
				out = append(out, ui.NuevoElementoMenu(rol, iconos.Ninguno, func() { p.AlRol(m, !m.Admin) }),
					ui.NuevoSeparadorMenu(), quitar)
			}
			if p.AlDenunciar == nil {
				return out
			}
			if len(out) > 0 {
				out = append(out, ui.NuevoSeparadorMenu())
			}
			nombre := i18n.Vars{"name": m.Nombre}
			denunciar := ui.NuevoElementoMenu(t.T("report.person", nombre), iconos.Ninguno, func() { p.AlDenunciar(m) })
			denunciar.Peligro = true
			ya := p.Bloqueado != nil && p.Bloqueado(m.ID)
			rotulo := t.T("block.block", nombre)
			if ya {
				rotulo = t.T("block.unblock", nombre)
			}
			bloquear := ui.NuevoElementoMenu(rotulo, iconos.Ninguno, func() { p.AlBloquear(m) })
			bloquear.Peligro = !ya
			return append(out, denunciar, bloquear)
		}
		f.RotuloMenu = t.T("members.actionsOn", i18n.Vars{"name": m.Nombre})
	}
	return f
}

// nuevoEsqueletoMiembros son las tres filas de «cargando»: `flex items-center gap-3
// px-4 py-2`, el cuadro de `2rem` y una barra de `0.75rem` al 55, 42 y 60 %.
func nuevoEsqueletoMiembros() fyne.CanvasObject {
	filas := make([]fyne.CanvasObject, 0, 4)
	filas = append(filas, hueco(tema.Espacio(2)))
	for _, parte := range []float32{0.55, 0.42, 0.60} {
		filas = append(filas, container.New(&filaEsqueleto{parte: parte},
			ui.NuevoEsqueleto(LadoEsqueletoAvatar, LadoEsqueletoAvatar), ui.NuevoEsqueleto(0, AltoEsqueletoTexto)))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(3)}, filas...)
}

// filaEsqueleto coloca el cuadro y la barra de una fila de «cargando».
type filaEsqueleto struct{ parte float32 }

func (f *filaEsqueleto) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, LadoEsqueletoAvatar)
}

func (f *filaEsqueleto) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	x := CanalonFilaHilo
	objs[0].Move(fyne.NewPos(x, 0))
	objs[0].Resize(fyne.NewSize(LadoEsqueletoAvatar, LadoEsqueletoAvatar))
	x += LadoEsqueletoAvatar + tema.Espacio(3)
	objs[1].Move(fyne.NewPos(x, (LadoEsqueletoAvatar-AltoEsqueletoTexto)/2))
	objs[1].Resize(fyne.NewSize(max(t.Width-x-CanalonFilaHilo, 0)*f.parte, AltoEsqueletoTexto))
}

// CreateRenderer monta el panel.
func (p *PanelRoster) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p.fondo = canvas.NewRectangle(tema.Color("surface", v))
	p.filete = canvas.NewRectangle(tema.Color("border", v))
	p.sep = canvas.NewRectangle(tema.Color("border", v))
	p.estado = ui.Texto("", tema.TextoXS, ui.Normal, tema.Color("text-muted", v))
	r := &panelRosterRenderer{p: p}
	r.Refresh()
	return r
}

type panelRosterRenderer struct{ p *PanelRoster }

func (r *panelRosterRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, AltoCabecera) }
func (r *panelRosterRenderer) Objects() []fyne.CanvasObject { return r.p.objetos }
func (r *panelRosterRenderer) Destroy()                     {}

func (r *panelRosterRenderer) Layout(t fyne.Size) {
	p := r.p
	p.fondo.Resize(t)
	p.filete.Resize(fyne.NewSize(tema.Borde, t.Height))
	x, ancho := tema.Borde, max(t.Width-tema.Borde, 0)
	p.cabecera.Move(fyne.NewPos(x, 0))
	p.cabecera.Resize(fyne.NewSize(ancho, AltoCabecera))
	y := AltoCabecera + MargenCajaBusqueda
	if p.anadir != nil && p.anadir.Ancho != ancho-2*MargenCajaBusqueda {
		p.anadir.Ancho = max(ancho-2*MargenCajaBusqueda, 0) // `w-full`
		p.anadir.Refresh()
	}
	m := p.arriba.MinSize()
	p.arriba.Move(fyne.NewPos(x+MargenCajaBusqueda, y))
	p.arriba.Resize(fyne.NewSize(max(ancho-2*MargenCajaBusqueda, 0), m.Height))
	y += m.Height + MargenCajaBusqueda
	p.sep.Move(fyne.NewPos(x, y))
	p.sep.Resize(fyne.NewSize(ancho, tema.Borde))
	y += tema.Borde
	ui.Truncar(p.estado, p.estado.Text, tema.TextoXS, ui.Normal, max(ancho-2*CanalonFilaHilo, 0))
	ui.ColocarTexto(p.estado, tema.TextoXS, x+CanalonFilaHilo, y+tema.Espacio(1.5))
	y += AltoEstadoBusqueda
	p.cuerpo.Move(fyne.NewPos(x, y))
	p.cuerpo.Resize(fyne.NewSize(ancho, max(t.Height-y, 0)))
}

func (r *panelRosterRenderer) Refresh() {
	p := r.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t := p.T
	titulo := t.T("members.title", nil)
	if p.Anadiendo {
		titulo = t.T("members.addPeople", nil)
	}
	p.cabecera = NuevaCabeceraPanel(titulo, "#"+p.NombreCanal, t.T("members.close", nil), p.AlCerrar, nil)
	if p.Anadiendo {
		p.cabecera.Volver, p.cabecera.RotuloVolver = func() { p.CambiarModo(false) }, t.T("members.back", nil)
	}
	p.arriba = p.construirArriba()
	texto, esError := p.textoEstado()
	p.estado.Text, p.estado.Color = texto, tema.Color("text-muted", v)
	if esError {
		p.estado.Color = tema.Color("danger-text", v)
	}
	p.estado.Refresh()
	p.fondo.FillColor = tema.Color("surface", v)
	p.fondo.Refresh()
	p.cuerpo = p.construirCuerpo()
	p.objetos = []fyne.CanvasObject{p.fondo, p.cuerpo, p.cabecera, p.arriba, p.sep, p.estado, p.filete}
	r.Layout(p.Size())
}

// ---- La fila ----

// FilaMiembro es una persona del panel: `flex items-center gap-3 px-4 py-1.5`, el
// avatar `sm`, el nombre `text-sm font-semibold` (con «(tú)» en `text-subtle` normal),
// las insignias y, a la derecha, el «…» o el «Añadir» de la lista del equipo.
// `hover:bg-bg-subtle`.
type FilaMiembro struct {
	ui.Pulsable
	Miembro   Miembro
	Tu        string
	Cola      string
	Insignias []string
	// Menu construye el «…»; nil, no hay.
	Menu       func() []fyne.CanvasObject
	RotuloMenu string

	fondo   *canvas.Rectangle
	avatar  *ui.Avatar
	nombre  *canvas.Text
	tu      *canvas.Text
	cola    *canvas.Text
	chapas  []*ui.Insignia
	mas     *ui.BotonIcono
	objetos []fyne.CanvasObject
}

var (
	_ fyne.Widget       = (*FilaMiembro)(nil)
	_ desktop.Hoverable = (*FilaMiembro)(nil)
)

// NuevaFilaMiembro crea la fila.
func NuevaFilaMiembro(m Miembro, tu, cola string, insignias []string) *FilaMiembro {
	f := &FilaMiembro{Miembro: m, Tu: tu, Cola: cola, Insignias: insignias}
	f.ExtendBaseWidget(f)
	return f
}

// MedidaFilaMiembro: manda el avatar de 32 con el `py-1.5`.
func MedidaFilaMiembro() fyne.Size {
	return fyne.NewSize(0, 2*tema.Espacio(1.5)+ui.MedidaAvatar(ui.AvatarSM).Height)
}

// CreateRenderer compone la fila.
func (f *FilaMiembro) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo = canvas.NewRectangle(color.NRGBA{})
	f.avatar = ui.NuevoAvatar(f.Miembro.Nombre).Con(ui.AvatarSM)
	f.avatar.Presencia = f.Miembro.Presencia
	f.nombre = ui.Texto(f.Miembro.Nombre, tema.TextoSM, ui.SemiNegrita, tema.Color("text", v))
	f.tu = ui.Texto(" "+f.Tu, tema.TextoSM, ui.Normal, tema.Color("text-subtle", v))
	f.cola = ui.Texto(f.Cola, escalonMeta, ui.Normal, tema.Color("text-subtle", v))
	f.objetos = []fyne.CanvasObject{f.fondo, f.avatar, f.nombre}
	if f.Tu != "" {
		f.objetos = append(f.objetos, f.tu)
	}
	if f.Cola != "" {
		f.objetos = append(f.objetos, f.cola)
	}
	for _, i := range f.Insignias {
		c := ui.NuevaInsignia(i, ui.InsigniaNeutra)
		f.chapas = append(f.chapas, c)
		f.objetos = append(f.objetos, c)
	}
	if f.Menu != nil {
		var menu *ui.Menu
		f.mas = ui.NuevoBotonIcono(iconos.Ninguno, f.RotuloMenu, func() {
			menu = ui.NuevoMenu(f.Menu()...)
			menu.Rotulo = f.RotuloMenu
			menu.Mostrar(f.mas)
		})
		f.mas.Glifo = dibujo(glifoMas, GlifoBarraMensaje, "text-muted", v)
		f.mas.Lado, f.mas.TamIcono = LadoBotonMensaje, GlifoBarraMensaje
		f.objetos = append(f.objetos, f.mas)
	}
	r := &filaMiembroRenderer{f: f}
	r.Refresh()
	return r
}

type filaMiembroRenderer struct{ f *FilaMiembro }

func (r *filaMiembroRenderer) MinSize() fyne.Size           { return MedidaFilaMiembro() }
func (r *filaMiembroRenderer) Objects() []fyne.CanvasObject { return r.f.objetos }
func (r *filaMiembroRenderer) Destroy()                     {}

func (r *filaMiembroRenderer) Layout(t fyne.Size) {
	f := r.f
	f.fondo.Resize(t)
	x := CanalonFilaHilo
	m := ui.MedidaAvatar(ui.AvatarSM)
	f.avatar.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
	f.avatar.Resize(m)
	x += m.Width + tema.Espacio(3)
	der := t.Width - CanalonFilaHilo
	if f.mas != nil {
		der -= LadoBotonMensaje
		f.mas.Move(fyne.NewPos(der, (t.Height-LadoBotonMensaje)/2))
		f.mas.Resize(fyne.NewSize(LadoBotonMensaje, LadoBotonMensaje))
		der -= tema.Espacio(3)
	}
	if f.Cola != "" {
		ancho := ui.Medir(f.Cola, escalonMeta, ui.Normal).Width
		der -= ancho
		ui.ColocarTexto(f.cola, escalonMeta, der, (t.Height-escalonMeta.Linea)/2)
		der -= tema.Espacio(3)
	}
	for i := len(f.chapas) - 1; i >= 0; i-- {
		mc := ui.MedidaInsignia(f.chapas[i].Texto)
		der -= mc.Width
		f.chapas[i].Move(fyne.NewPos(der, (t.Height-mc.Height)/2))
		f.chapas[i].Resize(mc)
		der -= tema.Espacio(3)
	}
	y := (t.Height - tema.TextoSM.Linea) / 2
	anchoTu := float32(0)
	if f.Tu != "" {
		anchoTu = ui.Medir(" "+f.Tu, tema.TextoSM, ui.Normal).Width
	}
	w := ui.Truncar(f.nombre, f.Miembro.Nombre, tema.TextoSM, ui.SemiNegrita, max(der-x-anchoTu, 0))
	ui.ColocarTexto(f.nombre, tema.TextoSM, x, y)
	if f.Tu != "" {
		ui.ColocarTexto(f.tu, tema.TextoSM, x+w, y)
	}
}

func (r *filaMiembroRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo.FillColor = color.NRGBA{}
	if f.Encima() && !f.Inerte {
		f.fondo.FillColor = tema.Color("bg-subtle", v)
	}
	f.fondo.Refresh()
	r.Layout(f.Size())
}

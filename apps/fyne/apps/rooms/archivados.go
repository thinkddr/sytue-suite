// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"errors"
	"image/color"
	"slices"
	"strings"

	"golang.org/x/text/collate"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Contrapartida de ArchivedChannels.tsx: la pestaña «Archivados» de «Explorar canales».
// Lista los canales archivados de quien mira; cada fila ofrece «Renombrar» —el nombre
// pasa a ser un campo: Intro guarda, Escape cancela— y «Desarchivar», que lo devuelve a
// la barra. El permiso lo decide el servicio: a quien no administra el canal le llega su
// 403 con su aviso, no un botón escondido.
//
// Las reglas son las de lib/archivados.ts: `SoloArchivados`, `ClaveErrorArchivado` y
// `NombreNuevo`.

// SoloArchivados es `soloArchivados`: los archivados, sin directos, por nombre con la
// colación del idioma, que es lo que hace `localeCompare` («área» junto a «alfa», no
// detrás de la «z»).
func SoloArchivados(l i18n.Locale, cs []api.RoomsChannel) []api.RoomsChannel {
	out := make([]api.RoomsChannel, 0, len(cs))
	for _, c := range cs {
		if c.Archived && c.Kind != "direct" {
			out = append(out, c)
		}
	}
	col := collate.New(l.Tag())
	slices.SortStableFunc(out, func(a, b api.RoomsChannel) int { return col.CompareString(a.Name, b.Name) })
	return out
}

// ClaveErrorArchivado es `claveErrorArchivado`: el aviso de un fallo al desarchivar o
// renombrar, según el estado que devolvió el servicio.
func ClaveErrorArchivado(err error) string {
	var e *api.Error
	if !errors.As(err, &e) {
		return "archived.failed"
	}
	switch e.Status {
	case 403:
		return "archived.forbidden"
	case 409:
		return "archived.nameTaken"
	case 400:
		return "archived.invalidName"
	}
	return "archived.failed"
}

// NombreNuevo es `nombreNuevo`: el nombre que se manda al renombrar, o "" si no hay
// nada que mandar (vacío o igual al de ahora).
func NombreNuevo(actual, escrito string) string {
	n := strings.TrimSpace(escrito)
	if n == actual {
		return ""
	}
	return n
}

// FilaTrasQuitar es `filaTrasQuitar`: la fila que recibe el foco cuando `quitado` sale
// de la lista (se desarchivó): la siguiente, la anterior si era la última, o "" si la
// lista se queda vacía.
func FilaTrasQuitar(ids []string, quitado string) string {
	i := slices.Index(ids, quitado)
	switch {
	case i < 0:
		return ""
	case i+1 < len(ids):
		return ids[i+1]
	case i > 0:
		return ids[i-1]
	}
	return ""
}

// Adónde va el foco tras el próximo pintado: el `focoRef` de ArchivedChannels.tsx.
const (
	focoCampo       = "campo"
	focoRenombrar   = "renombrar"
	focoDesarchivar = "desarchivar"
	focoGuardar     = "guardar"
	focoCancelar    = "cancelar"
)

type focoArchivado struct {
	// fila es el canal; "" manda el foco a la pestaña.
	fila, destino string
}

// Archivados es el cuerpo de la pestaña.
type Archivados struct {
	widget.BaseWidget

	T      *i18n.Translator
	Estado Estado
	// Canales son los archivados; Ocupado, los que van de camino al servicio, cada uno
	// por su id (con un solo hueco, la respuesta de uno reactivaba el botón de otro aún
	// en curso); y Renombrando, el que tiene el campo de nombre abierto.
	Canales     []Canal
	Ocupado     map[string]bool
	Renombrando string
	// Borrador es lo escrito en el campo de Renombrando: el estado `nombre` de la web.
	// Cada pintado rehace las filas, y sin esto la respuesta de la petición de otra fila
	// devolvía el campo al nombre original a media escritura.
	Borrador string
	// Aviso es el fallo de la última acción, ya traducido, o "".
	Aviso string

	AlDesarchivar func(Canal)
	AlRenombrar   func(Canal, string)
	AlReintentar  func()
	// Pestana es adónde va el foco si la fila que lo tenía ya no está (la lista se
	// quedó vacía o se está recargando).
	Pestana func() fyne.Focusable

	// foco es el destino pendiente: cada pintado rehace las filas, y sin esto el lienzo
	// se quedaba enfocando un campo o un botón que ya no estaba en pantalla.
	foco *focoArchivado
	// enEdicion es la fila del campo abierto, que se reutiliza entre pintados mientras
	// siga siendo la misma: rehecha, el campo nacía con el cursor al final y quien
	// corregía una letra en medio del nombre seguía escribiendo al final (en la web el
	// `<li>` lleva key y el `<input>` es el mismo nodo).
	enEdicion *FilaArchivado
	cuerpo    fyne.CanvasObject
	objetos   []fyne.CanvasObject
}

var _ fyne.Widget = (*Archivados)(nil)

// NuevosArchivados crea el cuerpo, cargando.
func NuevosArchivados(t *i18n.Translator) *Archivados {
	a := &Archivados{T: t, Estado: Cargando, Ocupado: map[string]bool{}}
	a.ExtendBaseWidget(a)
	return a
}

// Filas son las filas pintadas, para quien quiera recorrerlas.
func (a *Archivados) Filas() []*FilaArchivado {
	var out []*FilaArchivado
	if c, ok := a.cuerpo.(*fyne.Container); ok {
		for _, o := range c.Objects {
			if f, es := o.(*FilaArchivado); es {
				out = append(out, f)
			}
		}
	}
	return out
}

func (a *Archivados) construir(v tema.Variante) fyne.CanvasObject {
	t := a.T
	switch a.Estado {
	case Cargando:
		return container.NewCenter(container.New(ui.Columna{},
			hueco(AireCargaExplorar), ui.NuevaRueda(LadoRuedaExplorar, "text-subtle", v), hueco(AireCargaExplorar)))
	case Error:
		return container.NewCenter(container.New(ui.Columna{Hueco: tema.Espacio(2)},
			hueco(AireErrorExplorar-tema.Espacio(2)),
			container.NewCenter(ui.NuevoRotulo(t.T("archived.loadFailed", nil), tema.TextoSM, ui.Normal, "text-muted")),
			container.NewCenter(ui.NuevoBoton(t.T("action.retry", nil), a.AlReintentar).Con(ui.Secundario, ui.TallaSM)),
			hueco(AireErrorExplorar-tema.Espacio(2))))
	case Listo, Vacio, SinConexion, SinPermiso:
	}
	if len(a.Canales) == 0 {
		vac := ui.NuevoVacio(t.T("archived.empty", nil), "")
		vac.Ilustracion = iconos.Pintar(iconos.VacioChat, v).A(LadoIlustracionBusqueda).Objeto()
		return vac
	}
	filas := make([]fyne.CanvasObject, 0, len(a.Canales)+1)
	if a.Aviso != "" {
		// `role="alert"` en `text-xs text-danger-text`.
		filas = append(filas, ui.NuevoRotulo(a.Aviso, tema.TextoXS, ui.Normal, "danger-text"))
	}
	previa := a.enEdicion
	a.enEdicion = nil
	for _, c := range a.Canales {
		f := previa
		if a.Renombrando != c.ID || f == nil || f.Canal != c || f.Ocupado != a.Ocupado[c.ID] {
			f = NuevaFilaArchivado(t, c, a.Ocupado[c.ID], a.Renombrando == c.ID)
			if f.Renombrando {
				f.campo.PonerTextoAlFinal(a.Borrador)
				f.campo.AlCambiar(func(s string) { a.Borrador = s })
			}
		}
		if f.Renombrando {
			a.enEdicion = f
		}
		f.AlDesarchivar = a.AlDesarchivar
		f.AlRenombrar = a.AlRenombrar
		f.AlEditarNombre = func(c Canal, si bool) {
			// Abrir lleva al campo; cerrar (Intro, Escape o «Cancelar») devuelve el foco a
			// «Renombrar» de la fila, como `cancelarRenombrado`.
			a.Renombrando, a.Borrador, a.foco = "", "", &focoArchivado{fila: c.ID, destino: focoRenombrar}
			if si {
				a.Renombrando, a.Borrador, a.foco.destino = c.ID, c.Nombre, focoCampo
			}
			a.Refresh()
		}
		filas = append(filas, f)
	}
	return container.New(ui.Columna{Hueco: HuecoListaExplorar}, filas...)
}

// EnfocarTras pide que, tras el próximo pintado, el foco vaya al `destino` de la fila
// `fila` ("" es la pestaña).
func (a *Archivados) EnfocarTras(fila, destino string) {
	a.foco = &focoArchivado{fila: fila, destino: destino}
}

// recordarFoco anota, antes de rehacer las filas, a qué control de cuál apuntaba el
// lienzo, para dárselo a su gemelo en las filas nuevas.
func (a *Archivados) recordarFoco(lienzo fyne.Canvas, viejas []*FilaArchivado) {
	if a.foco != nil || lienzo == nil {
		return
	}
	enfocado := lienzo.Focused()
	for _, f := range viejas {
		if d := f.destinoDe(enfocado); d != "" {
			a.foco = &focoArchivado{fila: f.Canal.ID, destino: d}
			return
		}
	}
}

// ponerFoco es el efecto de `focoRef`: lleva el foco al destino pendiente en las filas
// recién pintadas. Si el control está inerte (su petición sigue en curso), espera al
// pintado siguiente; si la fila ya no está, va a la pestaña.
func (a *Archivados) ponerFoco(lienzo fyne.Canvas, viejas []*FilaArchivado) {
	if a.foco == nil || lienzo == nil {
		return
	}
	// Lo que apuntara a una fila vieja ya no está en pantalla: se suelta antes, para
	// que las teclas no vayan a un campo invisible mientras se espera.
	enfocado := lienzo.Focused()
	for _, f := range viejas {
		if f.destinoDe(enfocado) != "" && f != a.enEdicion {
			lienzo.Unfocus()
			break
		}
	}
	f := a.foco
	if f.fila != "" {
		for _, g := range a.Filas() {
			if g.Canal.ID != f.fila {
				continue
			}
			c := g.control(f.destino)
			if c == nil {
				a.foco = nil
				return
			}
			if d, ok := c.(fyne.Disableable); ok && d.Disabled() {
				return
			}
			a.foco = nil
			ui.Enfocar(lienzo, c)
			return
		}
	}
	a.foco = nil
	if a.Pestana != nil {
		if p := a.Pestana(); p != nil {
			ui.Enfocar(lienzo, p)
		}
	}
}

// CreateRenderer monta el cuerpo.
func (a *Archivados) CreateRenderer() fyne.WidgetRenderer {
	r := &archivadosRenderer{a: a}
	r.Refresh()
	return r
}

type archivadosRenderer struct{ a *Archivados }

func (r *archivadosRenderer) MinSize() fyne.Size           { return r.a.cuerpo.MinSize() }
func (r *archivadosRenderer) Objects() []fyne.CanvasObject { return r.a.objetos }
func (r *archivadosRenderer) Destroy()                     {}

func (r *archivadosRenderer) Layout(t fyne.Size) {
	r.a.cuerpo.Move(fyne.NewPos(0, 0))
	r.a.cuerpo.Resize(fyne.NewSize(t.Width, max(t.Height, r.a.cuerpo.MinSize().Height)))
}

func (r *archivadosRenderer) Refresh() {
	a := r.a
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(a)
	viejas := a.Filas()
	a.recordarFoco(lienzo, viejas)
	a.cuerpo = a.construir(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	a.objetos = []fyne.CanvasObject{a.cuerpo}
	r.Layout(a.Size())
	a.ponerFoco(lienzo, viejas)
}

// FilaArchivado es un canal archivado: su marca, su nombre y su propósito, y los dos
// botones; o, renombrando, el campo con «Guardar» y «Cancelar».
type FilaArchivado struct {
	widget.BaseWidget

	Canal       Canal
	Ocupado     bool
	Renombrando bool

	AlDesarchivar  func(Canal)
	AlRenombrar    func(Canal, string)
	AlEditarNombre func(Canal, bool)

	encima      bool
	fondo       *canvas.Rectangle
	marca       *iconos.Dibujo
	nombre      *canvas.Text
	detalle     *canvas.Text
	campo       *ui.Entrada
	renombrar   *ui.Boton
	desarchivar *ui.Boton
	guardar     *ui.Boton
	cancelar    *ui.Boton
	objetos     []fyne.CanvasObject
}

var _ desktop.Hoverable = (*FilaArchivado)(nil)

// NuevaFilaArchivado crea la fila.
func NuevaFilaArchivado(t *i18n.Translator, c Canal, ocupado, renombrando bool) *FilaArchivado {
	f := &FilaArchivado{Canal: c, Ocupado: ocupado, Renombrando: renombrando}
	f.ExtendBaseWidget(f)
	editar := func(si bool) {
		if f.AlEditarNombre != nil {
			f.AlEditarNombre(f.Canal, si)
		}
	}
	f.renombrar = ui.NuevoBoton(t.T("archived.rename", nil), func() { editar(true) }).Con(ui.Fantasma, ui.TallaSM)
	f.renombrar.Inerte = ocupado
	f.desarchivar = ui.NuevoBoton(t.T("archived.unarchive", nil), func() {
		if f.AlDesarchivar != nil {
			f.AlDesarchivar(f.Canal)
		}
	}).Con(ui.Secundario, ui.TallaSM)
	f.desarchivar.Cargando = ocupado
	f.campo = ui.NuevaEntrada("")
	f.campo.Rotulo = t.T("archived.renameLabel", i18n.Vars{"name": c.Nombre})
	f.campo.PonerTexto(c.Nombre)
	guardar := func() {
		texto := f.campo.Texto()
		editar(false)
		if f.AlRenombrar != nil {
			f.AlRenombrar(f.Canal, texto)
		}
	}
	f.campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		switch k.Name {
		case fyne.KeyReturn, fyne.KeyEnter:
			guardar()
			return true
		case fyne.KeyEscape:
			editar(false)
			return true
		default:
			return false
		}
	}
	f.guardar = ui.NuevoBoton(t.T("action.save", nil), guardar).Con(ui.Primario, ui.TallaSM)
	f.cancelar = ui.NuevoBoton(t.T("action.cancel", nil), func() { editar(false) }).Con(ui.Fantasma, ui.TallaSM)
	return f
}

// Campo es el de renombrar.
func (f *FilaArchivado) Campo() *ui.Entrada { return f.campo }

// destinoDe es qué control de `f` es `o`, o "".
func (f *FilaArchivado) destinoDe(o fyne.Focusable) string {
	switch {
	case o == nil:
		return ""
	case o == fyne.Focusable(f.campo):
		return focoCampo
	case o == fyne.Focusable(f.renombrar):
		return focoRenombrar
	case o == fyne.Focusable(f.desarchivar):
		return focoDesarchivar
	case o == fyne.Focusable(f.guardar):
		return focoGuardar
	case o == fyne.Focusable(f.cancelar):
		return focoCancelar
	}
	return ""
}

// control es el control de `f` que corresponde a `destino`, o nil si la fila no lo
// pinta ahora (el campo solo existe renombrando, y los botones de reposo solo fuera).
func (f *FilaArchivado) control(destino string) fyne.Focusable {
	switch destino {
	case focoCampo:
		if f.Renombrando {
			return f.campo
		}
	case focoGuardar, focoCancelar:
		if f.Renombrando {
			if destino == focoGuardar {
				return f.guardar
			}
			return f.cancelar
		}
	case focoRenombrar, focoDesarchivar:
		if !f.Renombrando {
			if destino == focoRenombrar {
				return f.renombrar
			}
			return f.desarchivar
		}
	}
	return nil
}

// Botones son «Renombrar» y «Desarchivar», o «Guardar» y «Cancelar» renombrando.
func (f *FilaArchivado) Botones() []*ui.Boton {
	if f.Renombrando {
		return []*ui.Boton{f.guardar, f.cancelar}
	}
	return []*ui.Boton{f.renombrar, f.desarchivar}
}

// MouseIn es el `hover:bg-bg-subtle` de la fila.
func (f *FilaArchivado) MouseIn(*desktop.MouseEvent) { f.encima = true; f.Refresh() }

// MouseMoved no hace nada.
func (f *FilaArchivado) MouseMoved(*desktop.MouseEvent) {}

// MouseOut devuelve la fila al reposo.
func (f *FilaArchivado) MouseOut() { f.encima = false; f.Refresh() }

// AccessibilityLabel es el nombre del canal.
func (f *FilaArchivado) AccessibilityLabel() string { return f.Canal.Nombre }

// AccessibilityRole implementa fyne.Accessible.
func (f *FilaArchivado) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleContainer
}

func (f *FilaArchivado) detalleDe() string {
	if f.Canal.Proposito != "" {
		return f.Canal.Proposito
	}
	return f.Canal.Tema
}

// CreateRenderer compone la fila.
func (f *FilaArchivado) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo = canvas.NewRectangle(color.NRGBA{})
	f.fondo.CornerRadius = tema.RadioMD
	f.marca = dibujo(glifoArchivo, iconos.TallaSM, "text-muted", v)
	f.nombre = ui.Texto(f.Canal.Nombre, tema.TextoBase, ui.Medio, tema.Color("text", v))
	f.detalle = ui.Texto(f.detalleDe(), tema.TextoXS, ui.Normal, tema.Color("text-muted", v))
	if f.Renombrando {
		f.objetos = []fyne.CanvasObject{f.fondo, f.marca.Objeto(), f.campo, f.guardar, f.cancelar}
	} else {
		f.objetos = []fyne.CanvasObject{f.fondo, f.marca.Objeto(), f.nombre, f.detalle, f.renombrar, f.desarchivar}
	}
	r := &filaArchivadoRenderer{f: f}
	r.Refresh()
	return r
}

type filaArchivadoRenderer struct{ f *FilaArchivado }

func (r *filaArchivadoRenderer) MinSize() fyne.Size {
	f := r.f
	alto := MedidaFilaExplorar(f.detalleDe() != "").Height
	if f.Renombrando {
		alto = max(alto, 2*RellenoFilaExplorar+f.campo.MinSize().Height)
	}
	return fyne.NewSize(0, alto)
}
func (r *filaArchivadoRenderer) Objects() []fyne.CanvasObject { return r.f.objetos }
func (r *filaArchivadoRenderer) Destroy()                     {}

func (r *filaArchivadoRenderer) Layout(t fyne.Size) {
	f := r.f
	f.fondo.Resize(t)
	x := CanalonFilaExplorar
	lado := iconos.TallaSM
	f.marca.Objeto().Move(fyne.NewPos(x, (t.Height-lado)/2))
	f.marca.Objeto().Resize(fyne.NewSize(lado, lado))
	x += lado + HuecoFilaExplorar
	// Los botones, de derecha a izquierda, con el `gap-3` de la fila entre ellos.
	xb := t.Width - CanalonFilaExplorar
	botones := f.Botones()
	for i := len(botones) - 1; i >= 0; i-- {
		b := botones[i]
		mb := b.MinSize()
		xb -= mb.Width
		b.Move(fyne.NewPos(xb, (t.Height-mb.Height)/2))
		b.Resize(mb)
		xb -= HuecoFilaExplorar
	}
	cabe := max(xb-x, 0)
	if f.Renombrando {
		mc := f.campo.MinSize()
		f.campo.Move(fyne.NewPos(x, (t.Height-mc.Height)/2))
		f.campo.Resize(fyne.NewSize(cabe, mc.Height))
		return
	}
	alto := tema.TextoBase.Linea
	detalle := f.detalleDe()
	if detalle != "" {
		alto += tema.TextoXS.Linea
	}
	y := (t.Height - alto) / 2
	ui.Truncar(f.nombre, f.Canal.Nombre, tema.TextoBase, ui.Medio, cabe)
	ui.ColocarTexto(f.nombre, tema.TextoBase, x, y)
	if detalle == "" {
		f.detalle.Hide()
		return
	}
	f.detalle.Show()
	ui.Truncar(f.detalle, detalle, tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(f.detalle, tema.TextoXS, x, y+tema.TextoBase.Linea)
}

func (r *filaArchivadoRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo.FillColor = color.NRGBA{}
	if f.encima {
		f.fondo.FillColor = tema.Color("bg-subtle", v)
	}
	f.fondo.Refresh()
	r.Layout(f.Size())
}

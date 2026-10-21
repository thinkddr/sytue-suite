// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Compartir un documento, como la web: el panel de «Compartir» de la cabecera y de
// Colaboración (`dialogs/ShareDialog.tsx`) —copiar el enlace, «Gestionar acceso»,
// «Hecho»— y el diálogo de «Gestionar acceso» (`components/ShareModal.tsx`) —quién lo
// ve, dar acceso a alguien, quién lo tiene y los enlaces públicos—. Es del cromo porque
// lo comparten Docus, Calcus y Diapos, y la portada de los tres.
//
// ponytail: las dos tarjetas de «Quién puede abrirlo» llevan el chevrón de
// `ui.TarjetaEleccion` y no los iconos `users` y `building` de la web: la tarjeta
// compartida no acepta otro icono todavía, y es del agente de elementos transversales.

// Compartir es lo que necesita el panel: el servicio, el documento y cómo se escriben
// sus enlaces en la web.
type Compartir struct {
	T      func(string, i18n.Vars) string
	Svc    *api.Docs
	ID     string
	Titulo string
	// Enlace es la dirección del documento en la web, la que copia «Copiar enlace».
	Enlace string
	// EnlacePublico escribe la dirección de un enlace público a partir de su token
	// (`${origin}/s/${token}`).
	EnlacePublico func(token string) string
	Avisos        *ui.PilaAvisos
	// Portapapeles es donde se copia; nil: el del sistema.
	Portapapeles fyne.Clipboard
	// Lanzar corre una petición fuera del hilo de la interfaz; nil es una rutina nueva.
	Lanzar func(func())
	// Plazo acota cada petición.
	Plazo time.Duration
	// Dir traduce la dirección que se escribe al `sub` con el que se guarda el permiso,
	// y de vuelta para enseñar con quién está compartido. Como el BFF de la web: sin él
	// la dirección iba tal cual, el acceso se resuelve contra el `sub` y no abría nada.
	Dir *api.Directorio
	// nombres son las personas de los permisos, por su `sub`.
	nombres map[string]api.Persona

	// Lo que enseña «Gestionar acceso», cuando está abierto.
	permisos  []api.DocPermission
	enlaces   []api.DocShare
	nuevo     *api.DocShare // el recién creado, el único con token
	cargando  bool
	denegado  bool
	ocupado   bool
	quien     *ui.Entrada
	papel     *ui.Selector
	cuerpo    *fyne.Container
	dialogo   *ui.Dialogo
	copiadoYa bool
	// lienzo es el hueco donde se abrió «Gestionar acceso»: decide el ancho del cuerpo
	// y si las tarjetas de alcance van en dos columnas (`sm:grid-cols-2`).
	lienzo fyne.Size
}

// errNadie es una dirección que no es de nadie de la organización.
var errNadie = errors.New("compartir: nadie de la organización tiene esa dirección")

// PapelesCompartir son los que se pueden dar desde el diálogo, en su orden.
var PapelesCompartir = []string{"viewer", "commenter", "editor"}

func (s *Compartir) t(clave string, v ...i18n.Vars) string {
	if s.T == nil {
		return clave
	}
	if len(v) == 0 {
		return s.T(clave, nil)
	}
	return s.T(clave, v[0])
}

func (s *Compartir) lanzar(f func()) {
	if s.Lanzar != nil {
		s.Lanzar(f)
		return
	}
	go f()
}

func (s *Compartir) ctx() (context.Context, context.CancelFunc) {
	plazo := s.Plazo
	if plazo <= 0 {
		plazo = 30 * time.Second
	}
	return context.WithTimeout(context.Background(), plazo)
}

func (s *Compartir) avisar(tono ui.TonoAviso, clave string) {
	if s.Avisos != nil {
		s.Avisos.Notificar(ui.NuevoAviso(tono, s.t(clave), ""))
	}
}

func (s *Compartir) portapapeles() fyne.Clipboard {
	if s.Portapapeles != nil {
		return s.Portapapeles
	}
	return fyne.CurrentApp().Clipboard()
}

// ---- el panel ----

// AnchoPanelCompartir es el `w-80` del contenido.
var AnchoPanelCompartir = tema.Espacio(80)

// Panel es el contenido del panel de «Compartir», que va en el menú colgado del botón.
// `alGestionar` abre «Gestionar acceso»; `alCerrar` recoge el panel.
func (s *Compartir) Panel(alGestionar, alCerrar func()) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	titulo := ui.NuevoRotulo(s.t("share.heading"), tema.TextoMD, ui.SemiNegrita, "text")
	desc := ui.NuevoParrafo(s.t("share.description"), tema.TextoSM, ui.Normal, "text-muted", v).A(AnchoPanelCompartir)
	enlace := ui.NuevaEntrada("")
	enlace.PonerTexto(s.Enlace)
	enlace.SoloLectura(true)
	var copiar *ui.Boton
	copiar = ui.NuevoBoton(s.t("share.copy"), func() {
		s.portapapeles().SetContent(s.Enlace)
		s.copiadoYa = true
		copiar.Texto, copiar.Icono, copiar.TamIcono = s.t("share.copied"), iconos.Tic, 14
		copiar.Refresh()
		s.avisar(ui.AvisoExito, "share.copied.toast")
	}).Con(ui.Primario, ui.TallaSM)
	fila := container.New(filaCampo{}, enlace.Con(ui.CampoSM), copiar)
	gestionar := ui.NuevoBoton(s.t("share.manage"), alGestionar).Con(ui.Secundario, ui.TallaSM)
	hecho := ui.NuevoBoton(s.t("action.done"), alCerrar).Con(ui.Fantasma, ui.TallaSM)
	pie := container.New(extremos{}, gestionar, hecho)
	// `flex w-80 flex-col gap-3`, dentro del `p-4` del Popover: el menú pone `p-1`.
	col := container.New(ui.Columna{Hueco: tema.Espacio(3)}, titulo, desc, fila, pie)
	m := tema.Espacio(4) - tema.Espacio(1)
	return container.New(layout.NewCustomPaddedLayout(m, m, m, m), col)
}

// Mostrar cuelga el panel del botón, alineado por la derecha (`align="end"`).
func (s *Compartir) Mostrar(desde fyne.CanvasObject) {
	var menu *ui.Menu
	contenido := s.Panel(func() {
		menu.Cerrar()
		if c := fyne.CurrentApp().Driver().CanvasForObject(desde); c != nil {
			s.Gestionar(c)
		}
	}, func() { menu.Cerrar() })
	menu = ui.NuevoMenu(contenido)
	menu.Rotulo = s.t("share.popover.label", i18n.Vars{"title": s.titulo()})
	menu.Mostrar(desde)
}

func (s *Compartir) titulo() string {
	if s.Titulo == "" {
		return s.t("share.untitled")
	}
	return s.Titulo
}

// ---- «Gestionar acceso» ----

// Gestionar abre el diálogo y pide quién tiene acceso.
func (s *Compartir) Gestionar(c fyne.Canvas) {
	s.nuevo, s.denegado, s.cargando = nil, false, true
	s.quien = ui.NuevaEntrada(s.t("share.personPlaceholder"))
	opciones := make([]string, len(PapelesCompartir))
	for i, p := range PapelesCompartir {
		opciones[i] = s.t("share.role." + p)
	}
	s.papel = ui.NuevoSelector(opciones, func(int) { s.pintar() })
	s.papel.Elegida = 0
	s.cuerpo = container.New(ui.Columna{})
	s.lienzo = c.Size()
	s.dialogo = ui.NuevoDialogo(s.t("share.title", i18n.Vars{"name": s.titulo()}), s.cuerpo)
	s.dialogo.RotuloCerrar = s.t("ui.close")
	s.pintar()
	s.dialogo.Mostrar(c)
	s.recargar(true)
}

// recargar vuelve a pedir permisos y enlaces. `primera` distingue la carga del diálogo
// (que dice «no se ha podido cargar» y sabe de 403) de la de después de un cambio.
func (s *Compartir) recargar(primera bool) {
	if s.Svc == nil {
		s.cargando = false
		s.pintar()
		return
	}
	s.lanzar(func() {
		ctx, cancelar := s.ctx()
		defer cancelar()
		permisos, enlaces, err := s.Svc.Sharing(ctx, s.ID)
		nombres := map[string]api.Persona{}
		for _, p := range permisos {
			if p.PrincipalKind == "user" && s.Dir != nil {
				if per, e := s.Dir.PorID(ctx, p.PrincipalID); e == nil {
					nombres[p.PrincipalID] = per
				}
			}
		}
		fyne.Do(func() {
			s.nombres = nombres
			s.cargando = false
			var e *api.Error
			switch {
			case errors.As(err, &e) && e.Status == http.StatusForbidden:
				s.denegado = true
			case err != nil && primera:
				s.avisar(ui.AvisoPeligro, "share.loadFailed")
			case err == nil:
				s.permisos, s.enlaces = permisos, enlaces
			}
			s.pintar()
		})
	})
}

// cambiar corre una petición con el candado (`busy`) y recarga; si falla, el aviso.
func (s *Compartir) cambiar(hacer func(context.Context) error, claveError string, despues func()) {
	if s.ocupado || s.Svc == nil {
		return
	}
	s.ocupado = true
	s.pintar()
	s.lanzar(func() {
		ctx, cancelar := s.ctx()
		defer cancelar()
		err := hacer(ctx)
		fyne.Do(func() {
			s.ocupado = false
			if errors.Is(err, errNadie) {
				claveError = "share.unknownPerson"
			}
			if err != nil {
				s.avisar(ui.AvisoPeligro, claveError)
				s.pintar()
				return
			}
			if despues != nil {
				despues()
			}
			s.recargar(false)
		})
	})
}

func (s *Compartir) toda() bool {
	for _, p := range s.permisos {
		if p.PrincipalKind == "tenant" {
			return true
		}
	}
	return false
}

// alcance es «Solo las personas que yo elija» o «Toda mi organización».
func (s *Compartir) alcance(todos bool) {
	s.cambiar(func(ctx context.Context) error {
		if todos {
			return s.Svc.Grant(ctx, s.ID, "tenant", "", "viewer")
		}
		return s.Svc.Revoke(ctx, s.ID, "tenant", "")
	}, "share.grantFailed", nil)
}

// papelElegido es el del selector de «Dar acceso».
func (s *Compartir) papelElegido() string {
	if s.papel == nil || s.papel.Elegida < 0 || s.papel.Elegida >= len(PapelesCompartir) {
		return "viewer"
	}
	return PapelesCompartir[s.papel.Elegida]
}

func (s *Compartir) conceder() {
	quien := trimEspacios(s.quien.Texto())
	if quien == "" {
		return
	}
	papel := s.papelElegido()
	s.cambiar(func(ctx context.Context) error {
		if s.Dir == nil || !strings.Contains(quien, "@") {
			return errNadie
		}
		per, err := s.Dir.PorCorreo(ctx, quien)
		if err != nil {
			return errNadie // 404: nadie de la organización usa esa dirección
		}
		return s.Svc.Grant(ctx, s.ID, "user", per.ID, papel)
	}, "share.grantFailed", func() { s.quien.PonerTexto("") })
}

func (s *Compartir) crearEnlace() {
	papel := s.papelElegido()
	var creado api.DocShare
	s.cambiar(func(ctx context.Context) error {
		var err error
		creado, err = s.Svc.CreateShare(ctx, s.ID, papel)
		return err
	}, "share.linkFailed", func() {
		s.nuevo = &creado
		if creado.Token != "" && s.EnlacePublico != nil {
			s.portapapeles().SetContent(s.EnlacePublico(creado.Token))
			s.avisar(ui.AvisoExito, "share.linkCopied")
		}
	})
}

func trimEspacios(x string) string {
	i, j := 0, len(x)
	for i < j && (x[i] == ' ' || x[i] == '\t' || x[i] == '\n') {
		i++
	}
	for j > i && (x[j-1] == ' ' || x[j-1] == '\t' || x[j-1] == '\n') {
		j--
	}
	return x[i:j]
}

// ancho es lo que queda dentro del diálogo `md` donde está: 560 menos su `px-6` en el
// escritorio, la pantalla menos el `px-6` en la hoja del teléfono. A 512 fijos, en 390
// «Toda mi organización» se salía 194 px del panel.
func (s *Compartir) ancho() float32 { return ui.AnchoCuerpoTalla(ui.DialogoMD, s.lienzo) }

// pintar rehace el cuerpo del diálogo con el estado de ahora.
func (s *Compartir) pintar() {
	if s.cuerpo == nil {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var hijos []fyne.CanvasObject
	switch {
	case s.cargando:
		// `flex items-center justify-center py-8` con la rueda de 22.
		hijos = []fyne.CanvasObject{container.New(layout.NewCustomPaddedLayout(tema.Espacio(8), tema.Espacio(8), 0, 0),
			container.NewCenter(ui.NuevaRueda(22, "text-muted", v)))}
	case s.denegado:
		hijos = []fyne.CanvasObject{ui.NuevoParrafo(s.t("share.onlyOwner"), tema.TextoSM, ui.Normal, "text-muted", v).A(s.ancho())}
	default:
		hijos = []fyne.CanvasObject{container.New(ui.Columna{Hueco: tema.Espacio(5)},
			s.seccionAlcance(v), s.seccionDar(v), s.seccionQuien(v), s.seccionEnlaces(v))}
	}
	s.cuerpo.Objects = hijos
	s.cuerpo.Refresh()
	if s.dialogo != nil {
		s.dialogo.Refresh()
	}
}

func encabezado(texto string) fyne.CanvasObject {
	return ui.NuevoRotulo(texto, tema.TextoSM, ui.SemiNegrita, "text")
}

func (s *Compartir) seccionAlcance(tema.Variante) fyne.CanvasObject {
	todos := s.toda()
	// `grid gap-2 sm:grid-cols-2`: por debajo de `sm`, una debajo de otra a todo el ancho.
	una := s.lienzo.Width > 0 && s.lienzo.Width < ui.CorteSM
	ancho := (s.ancho() - tema.Espacio(2)) / 2
	if una {
		ancho = s.ancho()
	}
	nombradas := ui.NuevaTarjetaEleccion(s.t("share.scope.named"), s.t("share.scope.named.desc"),
		func() { s.alcance(false) }).A(ancho)
	nombradas.Elegida, nombradas.Inerte = !todos, s.ocupado
	org := ui.NuevaTarjetaEleccion(s.t("share.scope.org"), s.t("share.scope.org.desc"),
		func() { s.alcance(true) }).A(ancho)
	org.Elegida, org.Inerte = todos, s.ocupado
	var tarjetas fyne.CanvasObject = container.New(ui.Fila{Hueco: tema.Espacio(2)}, nombradas, org)
	if una {
		tarjetas = container.New(ui.Columna{Hueco: tema.Espacio(2)}, nombradas, org)
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, encabezado(s.t("share.scope")), tarjetas)
}

func (s *Compartir) seccionDar(tema.Variante) fyne.CanvasObject {
	dar := ui.NuevoBoton(s.t("share.grant"), s.conceder).Con(ui.Primario, ui.TallaSM)
	dar.Inerte = s.ocupado || trimEspacios(s.quien.Texto()) == ""
	s.quien.AlCambiar(func(string) {
		dar.Inerte = s.ocupado || trimEspacios(s.quien.Texto()) == ""
		dar.Refresh()
	})
	fila := container.New(filaCampo{final: true}, s.quien, s.papel, dar)
	// Las pistas parten de línea al ancho del cuerpo: la del correo pedía 420 en 342.
	pista, papel := ui.NuevaPistaCampo(s.t("share.personHint")), ui.NuevaPistaCampo(s.t("share.role."+s.papelElegido()+".desc"))
	pista.Ancho, papel.Ancho = s.ancho(), s.ancho()
	return container.New(ui.Columna{Hueco: tema.Espacio(2)},
		ui.NuevoRotulo(s.t("share.addPerson"), tema.TextoSM, ui.Medio, "text"), fila, pista, papel)
}

func (s *Compartir) seccionQuien(v tema.Variante) fyne.CanvasObject {
	col := []fyne.CanvasObject{encabezado(s.t("share.whoHasAccess"))}
	if len(s.permisos) == 0 {
		col = append(col, ui.NuevoVacio(s.t("share.nobody"), s.t("share.nobodyHelp")))
		return container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, col...)
	}
	filas := make([]fyne.CanvasObject, 0, len(s.permisos))
	for _, p := range s.permisos {
		nombre := p.PrincipalID
		if per, ok := s.nombres[p.PrincipalID]; ok {
			nombre = cmp.Or(per.Nombre, per.Email)
		}
		if p.PrincipalKind == "tenant" {
			nombre = s.t("share.wholeOrg")
		}
		der := []fyne.CanvasObject{ui.NuevaInsignia(s.t("share.role."+p.Role), ui.InsigniaNeutra)}
		// Al propietario no se le quita el acceso: el documento se quedaría sin dueño.
		if p.Role != "owner" {
			quitar := ui.NuevoBoton(s.t("share.revoke"), func() {
				s.cambiar(func(ctx context.Context) error { return s.Svc.Revoke(ctx, s.ID, p.PrincipalKind, p.PrincipalID) },
					"share.revokeFailed", nil)
			}).Con(ui.Fantasma, ui.TallaSM)
			quitar.Inerte = s.ocupado
			der = append(der, quitar)
		}
		filas = append(filas, filaAcceso(nombre, tema.TextoSM, container.New(ui.Fila{Hueco: tema.Espacio(2)}, der...), v))
	}
	col = append(col, container.New(ui.Columna{Hueco: tema.Espacio(1)}, filas...))
	return container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, col...)
}

func (s *Compartir) seccionEnlaces(v tema.Variante) fyne.CanvasObject {
	col := []fyne.CanvasObject{
		encabezado(s.t("share.links")),
		ui.NuevaAyudaEnLinea(s.t("share.linksWarn")).ConTono(ui.AyudaAviso).ConTitulo(s.t("share.linksHelp")),
	}
	if s.nuevo != nil && s.nuevo.Token != "" && s.EnlacePublico != nil {
		col = append(col, avisoEnlaceNuevo(s.t("share.linkOnce"), s.EnlacePublico(s.nuevo.Token), v))
	}
	if len(s.enlaces) > 0 {
		filas := make([]fyne.CanvasObject, 0, len(s.enlaces))
		for _, e := range s.enlaces {
			texto := s.t("share.role." + e.Role)
			if e.ExpiresAt != nil {
				texto += " · " + s.t("share.expires", i18n.Vars{"date": e.ExpiresAt.Format("2006-01-02")})
			}
			quitar := ui.NuevoBoton(s.t("share.revoke"), func() {
				s.cambiar(func(ctx context.Context) error { return s.Svc.RevokeShare(ctx, s.ID, e.ID) }, "share.revokeFailed",
					func() {
						if s.nuevo != nil && s.nuevo.ID == e.ID {
							s.nuevo = nil
						}
					})
			}).Con(ui.Fantasma, ui.TallaSM)
			quitar.Inerte = s.ocupado
			filas = append(filas, filaAcceso(texto, tema.TextoXS, quitar, v))
		}
		col = append(col, container.New(ui.Columna{Hueco: tema.Espacio(1)}, filas...))
	}
	crear := ui.NuevoBoton(s.t("share.createLink"), s.crearEnlace).Con(ui.Secundario, ui.TallaSM)
	crear.Inerte = s.ocupado
	col = append(col, container.New(ui.Fila{}, crear))
	return container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, col...)
}

// ---- piezas del diálogo ----

// filaCampo es `flex items-center gap-2` (o `items-end` con `final`) con el primero
// ocupando lo que sobra (`flex-1`) y los demás a su medida.
type filaCampo struct{ final bool }

func (f filaCampo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var w, h float32
	for i, o := range objs {
		m := o.MinSize()
		if i > 0 {
			w += tema.Espacio(2)
		}
		w += m.Width
		h = max(h, m.Height)
	}
	return fyne.NewSize(w, h)
}

func (f filaCampo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	resto := t.Width
	for _, o := range objs[1:] {
		resto -= o.MinSize().Width + tema.Espacio(2)
	}
	x := float32(0)
	for i, o := range objs {
		m := o.MinSize()
		w := m.Width
		if i == 0 {
			w = max(resto, 0)
		}
		y := (t.Height - m.Height) / 2
		if f.final {
			y = t.Height - m.Height
		}
		o.Move(fyne.NewPos(x, y))
		o.Resize(fyne.NewSize(w, m.Height))
		x += w + tema.Espacio(2)
	}
}

// extremos es `flex items-center justify-between`: el primero a la izquierda y el
// último a la derecha.
type extremos struct{}

func (extremos) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var w, h float32
	for _, o := range objs {
		m := o.MinSize()
		w += m.Width
		h = max(h, m.Height)
	}
	return fyne.NewSize(w+tema.Espacio(2), h)
}

func (extremos) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	a, b := objs[0], objs[len(objs)-1]
	ma, mb := a.MinSize(), b.MinSize()
	a.Move(fyne.NewPos(0, (t.Height-ma.Height)/2))
	a.Resize(ma)
	b.Move(fyne.NewPos(t.Width-mb.Width, (t.Height-mb.Height)/2))
	b.Resize(mb)
}

// filaAcceso es el `<li class="border-border bg-surface flex items-center
// justify-between gap-3 rounded-md border px-3 py-2">` de un permiso o un enlace: el
// texto truncado a la izquierda y lo de la derecha a su medida.
func filaAcceso(texto string, e tema.Escalon, derecha fyne.CanvasObject, v tema.Variante) fyne.CanvasObject {
	token := "text"
	if e == tema.TextoXS {
		token = "text-subtle" // `sytue-meta`
	}
	return cajaConTexto(texto, e, token, "surface", "border", tema.RadioMD, tema.Espacio(3), tema.Espacio(2), derecha, v)
}

// cajaConTexto es una caja con borde que lleva un texto truncado a la izquierda y, si
// hay, algo a la derecha con `gap-3`.
func cajaConTexto(texto string, e tema.Escalon, tinta, fondo, borde string, radio, px, py float32,
	derecha fyne.CanvasObject, v tema.Variante,
) fyne.CanvasObject {
	caja := ui.NuevaCaja(tema.Color(fondo, v), tema.Color(borde, v), radio)
	t := ui.Texto(texto, e, ui.Normal, tema.Color(tinta, v))
	f := &filaLista{caja: caja, texto: t, completo: texto, e: e, px: px, py: py, derecha: derecha}
	objs := append(caja.Objetos(), t)
	if derecha != nil {
		objs = append(objs, derecha)
	}
	return container.New(f, objs...)
}

type filaLista struct {
	caja     *ui.Caja
	texto    *canvas.Text
	derecha  fyne.CanvasObject
	completo string
	e        tema.Escalon
	px, py   float32
}

func (f *filaLista) MinSize([]fyne.CanvasObject) fyne.Size {
	var d fyne.Size
	if f.derecha != nil {
		d = f.derecha.MinSize()
		d.Width += tema.Espacio(3)
	}
	return fyne.NewSize(d.Width+2*(f.px+tema.Borde), max(d.Height, f.e.Linea)+2*(f.py+tema.Borde))
}

func (f *filaLista) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	f.caja.Redimensionar(t)
	fin := t.Width - tema.Borde - f.px
	if f.derecha != nil {
		m := f.derecha.MinSize()
		fin -= m.Width
		f.derecha.Move(fyne.NewPos(fin, (t.Height-m.Height)/2))
		f.derecha.Resize(m)
		fin -= tema.Espacio(3)
	}
	x := tema.Borde + f.px
	ui.Truncar(f.texto, f.completo, f.e, ui.Normal, max(fin-x, 0))
	ui.ColocarTexto(f.texto, f.e, x, (t.Height-f.e.Linea)/2)
}

// avisoEnlaceNuevo es el recuadro del enlace recién creado, que se ve UNA vez:
// `border-warning bg-warning-bg rounded-md border p-2.5`, la frase en `text-xs
// font-semibold text-warning-text` y la dirección en un `<code class="border-border
// bg-surface mt-1.5 block truncate rounded border px-2 py-1 text-xs">`.
func avisoEnlaceNuevo(frase, direccion string, v tema.Variante) fyne.CanvasObject {
	codigo := cajaConTexto(direccion, tema.TextoXS, "text", "surface", "border", tema.RadioSM,
		tema.Espacio(2), tema.Espacio(1), nil, v)
	dentro := container.New(ui.Columna{Hueco: tema.Espacio(1.5)},
		ui.NuevoRotulo(frase, tema.TextoXS, ui.SemiNegrita, "warning-text"), codigo)
	caja := ui.NuevaCaja(tema.Color("warning-bg", v), tema.Color("warning", v), tema.RadioMD)
	return container.New(&conCaja{caja: caja, m: tema.Espacio(2.5) + tema.Borde}, append(caja.Objetos(), dentro)...)
}

// conCaja pone una caja de fondo y, dentro, el último objeto con ese margen.
type conCaja struct {
	caja *ui.Caja
	m    float32
}

func (c *conCaja) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[len(objs)-1].MinSize()
	return fyne.NewSize(m.Width+2*c.m, m.Height+2*c.m)
}

func (c *conCaja) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	c.caja.Redimensionar(t)
	o := objs[len(objs)-1]
	o.Move(fyne.NewPos(c.m, c.m))
	o.Resize(fyne.NewSize(max(t.Width-2*c.m, 0), max(t.Height-2*c.m, 0)))
}

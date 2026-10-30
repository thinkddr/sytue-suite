// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Sidebar.tsx

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// El menú lateral de la suite (Sidebar.tsx), con las medidas tomadas del DOM de
// producción (corporate.sytue.eu/drive y /mail, 1280×800, rem 14).
//
// Es la pieza más portada de todo el cromo —seis aplicaciones montan la misma— y por
// eso sus números están aquí y no repartidos por las pantallas:
//
//	menú          260 de ancho, SIN filete a la derecha (el carril es de color y ya se
//	              separa del contenido) → con el canalón, 232 de ancho útil
//	cabecera      `px-4 py-3`  (42 en Rooms; 52,5 en Mail, que mete un botón `h-9`)
//	cuerpo        `px-4 py-2`
//	sección       encabezado `pt-1 pr-1 pb-1.5 pl-2.5` y `mb-4` debajo
//	fila          33,25 × 232, `px-2.5 py-2`, `rounded-md`, texto 12,25/19,25 peso 500
//	entre filas   1,75 (`gap-0.5`)
//	pie           filete de 1 arriba (`sidebar-border`) y `px-4 py-2`
//
// # El CARRIL
//
// El menú es de color de arriba abajo (`sidebar-bg`), con texto claro. En la web todo lo
// que cuelga de `bg-sidebar` recibe reasignada la familia del cromo (appearance.css): el
// indicador y el relleno de los glifos pasan a ámbar, el foco a blanco y el botón
// principal a blanco con rótulo del color del carril. Fyne no tiene cascada, así que
// aquí cada pieza del menú pide el token del carril POR SU NOMBRE —`sidebar-indicator`,
// `sidebar-focus`, `sidebar-border`, `sidebar-primary`— y no el del cromo, que es el de
// la barra clara. Una pieza propia de una app que vaya dentro del menú hace lo mismo.
//
// El 33,25 sale de `max(caja del icono, caja de línea del texto) + 2 × py-2`, o sea
// `max(17,5, 19,25) + 14`. La caja del icono son los 17,5 de `h-5 w-5`, aunque el
// glifo que va dentro mida 18 y asome un cuarto de píxel por cada lado: en la web
// tampoco cuenta, porque `overflow` es visible.
var (
	canalonMenu = tema.Espacio(4)   // px-4
	rellenoMenu = tema.Espacio(2)   // py-2 del cuerpo y del pie
	cabeceraPY  = tema.Espacio(3)   // py-3 de la cabecera
	filaPY      = tema.Espacio(2)   // py-2
	huecoFilas  = tema.Espacio(0.5) // gap-0.5
	huecoFila   = tema.Espacio(2)   // gap-2 entre icono, rótulo y contador
	margenAbajo = tema.Espacio(4)   // mb-4 al final de cada sección
)

// enTinta es el `disabled:opacity-50` de lo que va SOBRE EL CARRIL: el 50 % compuesto
// sobre la tinta del menú, que es lo que hay detrás. `atenuar` lo compone sobre
// `surface`, que es el fondo de los controles corrientes, y aquí aclaraba en vez de
// apagar: un rótulo casi blanco al 50 % sobre blanco sigue casi blanco, cuando en la web
// se hunde hacia la tinta.
func enTinta(c color.NRGBA, inerte bool) color.NRGBA {
	if !inerte || c.A == 0 {
		return c
	}
	c.A = uint8(float32(c.A) * 0.5)
	return tokens.SobreSRGB(c, FondoCarril(tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
}

// TokenCarril es el token del color que hay DETRÁS de lo que va en el menú lateral:
// `sidebar-bg` en Carril y, en Marco —donde el menú deja ver el marco—, el del marco
// (`frame-from`; en claro y en oscuro es el mismo color, y en el traslúcido el arranque
// de su degradado, que es lo que queda bajo el menú). Es para quien compone sobre el
// carril o necesita una caja de su color: nunca para pintar el fondo del menú.
func TokenCarril() string {
	if tema.EsMarco() {
		return "frame-from"
	}
	return "sidebar-bg"
}

// FondoCarril es el color de TokenCarril en ese tema.
func FondoCarril(v tema.Variante) color.NRGBA { return tema.Color(TokenCarril(), v) }

// RellenoNav es el relleno horizontal de un ítem del menú, `px-[var(--sytue-nav-pad)]`:
// de la forma (14 en Marco, 10 en Carril). Lo comparte el rótulo de la sección, que cae
// en la misma vertical.
func RellenoNav() float32 { return tema.Forma("nav-pad") }

// BordeSobre es el borde `nombre` compuesto sobre el fondo `fondo`, que es donde lo pinta
// la web: un `rgba(tinta, .12)` sobre lo que tenga detrás. `tema.Color` da los bordes
// aplanados sobre `surface`, y eso solo acierta sobre `surface`. Medido el 2026-09-27 en
// oscuro: el pie de los diálogos y el separador de los menús, sobre `surface-raised`,
// salían #243339 donde la web pinta #2a3b44 (Δ 11); el filete de la cabecera del ERP y
// el de las columnas de Drive, sobre `bg`, #2d3b42 donde la web pinta #263035 (Δ 13).
func BordeSobre(nombre, fondo string, v tema.Variante) color.NRGBA {
	relleno := tema.Color(fondo, v)
	if relleno.A != 0xff {
		return tokens.SobreConAlfa(tokens.Crudo(nombre, v), relleno)
	}
	return tokens.SobreSRGB(tokens.Crudo(nombre, v), relleno)
}

// Medidas de las piezas de dentro de una fila.
const (
	// CajaIcono es `grid h-5 w-5`: el hueco que el icono ocupa en el layout.
	CajaIcono float32 = 1.25 * tema.Rem
	// GlifoMenu es el `size={18}` del icono que va dentro de esa caja, y el alto que
	// manda en la fila COMPACTA, que no lleva caja: ahí el glifo es el elemento más
	// alto y por eso la fila mide 28,5 y no los 26,25 que daría su texto.
	GlifoMenu float32 = 18
	// PuntoEtiqueta es `h-2.5 w-2.5`: el punto de color de una etiqueta de correo,
	// centrado en la caja del icono.
	PuntoEtiqueta float32 = 0.625 * tema.Rem
	// AltoContador es el `h-[18px] min-w-[18px]` de `<Badge count>`.
	AltoContador float32 = 18
)

// AnchoUtilMenu es el ancho que le queda a una fila dentro del menú desplegado: los
// 260 del carril menos el canalón de los dos lados: 232. Eran 231 mientras el menú llevó
// su filete a la derecha.
const AnchoUtilMenu = tema.AnchoLateral - 2*4*0.25*tema.Rem

// Lateral es el `<nav>` de Sidebar.tsx: el carril de tinta de 260 que se pliega a 64.
//
// El cuerpo desplaza como `overflow-y-auto` en la web: cabecera y pie conservan su
// sitio al girar un teléfono. Si la app ya aporta un Scroll, se reutiliza.
//
// # Este menú es de UNA aplicación
//
// Cada producto de la suite es una aplicación independiente también en escritorio y en
// móvil, así que aquí dentro solo van destinos de la app que lo monta. Una fila que
// lleve a OTRO producto —Drive abriendo un documento en Docs— no navega dentro de este
// proceso: su `AlPulsar` delega en el sistema por el camino de los deep links, igual
// que en la web sería ir a otro subdominio. La costura es justo esa función, y no hay
// nada aquí que aloje dos productos a la vez.
type Lateral struct {
	pulsable
	// sinParada: el menú no es una parada de Tab; el foco se le da por código al abrir
	// el cajón.
	sinParada

	// Cabecera es lo que va arriba con `px-4 py-3`: la marca de la app, el botón de
	// «Redactar». Puede ser nil.
	Cabecera fyne.CanvasObject
	// Cuerpo es la lista de secciones, con `px-4 py-2`. Se lleva el alto que sobra.
	Cuerpo fyne.CanvasObject
	// Pie va debajo de un filete, con `px-4 py-2`. Puede ser nil.
	Pie fyne.CanvasObject
	// Plegado es el carril estrecho de 64 (`data-collapsed`).
	Plegado bool
	// SinFondo deja ver lo que hay detrás: en la forma Marco el menú va sobre el marco,
	// que ya es de su color. Lo pone el armazón; en el cajón del teléfono no.
	SinFondo bool
}

var _ fyne.Widget = (*Lateral)(nil)

// NuevoLateral crea el menú con su cuerpo. El nombre accesible es el `aria-label` del
// `<nav>`; la app lo pasa YA TRADUCIDO, porque el idioma es suyo y no de un widget.
func NuevoLateral(nombre string, cuerpo fyne.CanvasObject) *Lateral {
	l := &Lateral{Cuerpo: cuerpo}
	l.Nombre = nombre
	l.ExtendBaseWidget(l)
	return l
}

// ConCabecera pone el botón principal del menú y LO MARCA como «sobre cromo», que es
// lo que hace `Sidebar.tsx` reasignando `--color-primary` para todo lo que cuelga de
// él: en tema claro `primary` es la misma tinta que el fondo del carril, así que sin
// esto el botón de «Redactar» es texto suelto sobre tinta hasta pasarle el ratón.
//
// Lo pone el MENÚ y no cada app a propósito: seis apps repitiendo la variante es como
// se pierde en la séptima.
//
// Y le pone el aro BLANCO del carril, que en la web también pone el menú: la cabecera de
// `Sidebar.tsx` cambia `--sytue-focus-ring` en su hijo directo con aro de botón. El
// color de foco del tema no se ve sobre el carril.
func (l *Lateral) ConCabecera(b *Boton) *Lateral {
	b.SobreCromo, b.TokenAro = true, "sidebar-focus"
	l.Cabecera = b
	return l
}

// azulSobreTinta es la otra mitad de esa reasignación: en la web `--color-primary`
// cambia para TODO lo que cuelga del `<nav>`, no solo para la cabecera. Así que cada
// botón PRINCIPAL o SECUNDARIO del cuerpo o del pie —el «Reintentar» de un error de
// carpetas, el «Mejorar el plan» junto al medidor— pasa también al primario del carril,
// blanco con el rótulo de su color. El fantasma no usa `primary` de relleno, y en la web
// tampoco cambia.
//
// Se recorre el árbol del menú: contenedores, desplazadores y las
// piezas del lateral que dicen sus hijos.
//
// ponytail: solo `Boton`; el conmutador y la casilla también pintan `primary` y no se
// marcan, porque ningún menú de la suite los lleva. Y un widget propio de una app que no
// declare sus hijos corta el recorrido, como en el cajón.
func azulSobreTinta(o fyne.CanvasObject) {
	switch v := o.(type) {
	case nil:
	case *Boton:
		if (v.Variante == Primario || v.Variante == Secundario) && !v.SobreCromo {
			v.SobreCromo = true
			v.Refresh()
		}
	case *fyne.Container:
		for _, h := range v.Objects {
			azulSobreTinta(h)
		}
	case *container.Scroll:
		azulSobreTinta(v.Content)
	case conHijos:
		for _, h := range v.hijosDeCajon() {
			azulSobreTinta(h)
		}
	}
}

// AccessibilityRole: el menú es una región, no un botón. Fyne no tiene rol de
// navegación, así que se anuncia como contenedor —pero CON su nombre, que es lo que un
// `*fyne.Container` a secas no puede dar.
func (l *Lateral) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// AnchoMenu es lo que mide el carril según esté desplegado o plegado.
func AnchoMenu(plegado bool) float32 {
	if plegado {
		return tema.LateralPlegado
	}
	return tema.AnchoLateral
}

// MedidaLateral es lo que ocupa el menú: ancho de token y, de alto, lo que suman sus
// tres franjas. En la pantalla se estira a todo lo alto; esto es su mínimo.
func MedidaLateral(l *Lateral) fyne.Size {
	alto := alto(l.Cuerpo) + 2*rellenoMenu
	if l.Cabecera != nil {
		alto += l.Cabecera.MinSize().Height + 2*cabeceraPY
	}
	if l.Pie != nil {
		alto += tema.Borde + l.Pie.MinSize().Height + 2*rellenoMenu
	}
	return fyne.NewSize(AnchoMenu(l.Plegado), alto)
}

func alto(o fyne.CanvasObject) float32 {
	if o == nil {
		return 0
	}
	return o.MinSize().Height
}

// CreateRenderer construye la geometría: el fondo de color y las franjas.
func (l *Lateral) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &lateralRenderer{
		l:     l,
		fondo: canvas.NewRectangle(tema.Color("sidebar-bg", v)),
	}
	r.fondo.Hidden = l.SinFondo
	r.objetos = []fyne.CanvasObject{r.fondo}
	for _, o := range []fyne.CanvasObject{l.Cabecera, r.cuerpo(), l.Pie} {
		if o != nil {
			r.objetos = append(r.objetos, o)
		}
	}
	if l.Pie != nil {
		r.filetePie = canvas.NewRectangle(BordeSobre("sidebar-border", TokenCarril(), v))
		r.objetos = append(r.objetos, r.filetePie)
	}
	azulSobreTinta(l)
	return r
}

type lateralRenderer struct {
	l                *Lateral
	fondo, filetePie *canvas.Rectangle
	objetos          []fyne.CanvasObject
	desplaza         *container.Scroll
}

func (r *lateralRenderer) cuerpo() fyne.CanvasObject {
	o := r.l.Cuerpo
	if o == nil {
		return nil
	}
	if s, ya := o.(*container.Scroll); ya {
		RevelarFocoEnScroll(s, s.Content)
		return s
	}
	if r.desplaza == nil {
		r.desplaza = container.NewVScroll(o)
	} else if r.desplaza.Content != o {
		r.desplaza.Content = o
		r.desplaza.Offset = fyne.Position{}
		r.desplaza.Refresh()
	}
	RevelarFocoEnScroll(r.desplaza, o)
	return r.desplaza
}

func (r *lateralRenderer) MinSize() fyne.Size           { return MedidaLateral(r.l) }
func (r *lateralRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *lateralRenderer) Destroy()                     {}

// Layout reparte el alto: cabecera y pie piden el suyo y el cuerpo se queda el resto,
// que es el `flex-1 min-h-0` de la web.
func (r *lateralRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)

	// Sin filete a la derecha: 232 de útil con los 260 del carril.
	ancho := max(t.Width-2*canalonMenu, 0)
	r.objetos = []fyne.CanvasObject{r.fondo}
	y := float32(0)
	if r.l.Cabecera != nil {
		h := r.l.Cabecera.MinSize().Height
		r.l.Cabecera.Move(fyne.NewPos(canalonMenu, y+cabeceraPY))
		r.l.Cabecera.Resize(fyne.NewSize(ancho, h))
		r.objetos = append(r.objetos, r.l.Cabecera)
		y += h + 2*cabeceraPY
	}
	abajo := t.Height
	if r.l.Pie != nil {
		if r.filetePie == nil {
			v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
			r.filetePie = canvas.NewRectangle(BordeSobre("sidebar-border", TokenCarril(), v))
		}
		h := r.l.Pie.MinSize().Height
		abajo -= h + 2*rellenoMenu
		r.filetePie.Move(fyne.NewPos(0, abajo-tema.Borde))
		r.filetePie.Resize(fyne.NewSize(t.Width, tema.Borde))
		r.l.Pie.Move(fyne.NewPos(canalonMenu, abajo+rellenoMenu))
		r.l.Pie.Resize(fyne.NewSize(ancho, h))
		abajo -= tema.Borde
	}
	if cuerpo := r.cuerpo(); cuerpo != nil {
		cuerpo.Move(fyne.NewPos(canalonMenu, y+rellenoMenu))
		cuerpo.Resize(fyne.NewSize(ancho, max(abajo-y-2*rellenoMenu, 0)))
		r.objetos = append(r.objetos, cuerpo)
		s, _ := cuerpo.(*container.Scroll)
		if RevelarFocoEnScroll(s, r.l.Cuerpo) {
			MostrarFoco(s, lienzoDe(r.l), 0)
		}
	}
	// El orden de Tab sigue cabecera, destinos y cuenta, aunque el pie se mida
	// antes para reservarle su sitio fuera del desplazamiento.
	if r.l.Pie != nil {
		r.objetos = append(r.objetos, r.l.Pie, r.filetePie)
	}
}

// Refresh repinta con los colores del tema actual, y vuelve a marcar en azul lo que se
// haya colgado después (Mail rehace su cuerpo con cada dato).
func (r *lateralRenderer) Refresh() {
	azulSobreTinta(r.l)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("sidebar-bg", v)
	r.fondo.Hidden = r.l.SinFondo
	r.fondo.Refresh()
	if r.filetePie != nil {
		r.filetePie.FillColor = BordeSobre("sidebar-border", TokenCarril(), v)
		r.filetePie.Refresh()
	}
	r.Layout(r.l.Size())
}

// SeccionLateral es SidebarSection: un rótulo en versalitas, sus filas separadas por
// 1,75, y 14 de aire debajo.
//
// El rótulo se pliega si lleva AlPlegar, y entonces gana un galón delante. Cuando está
// plegada no pinta sus filas pero SÍ conserva el `mb-4`, como la web.
type SeccionLateral struct {
	pulsable

	// Titulo es el rótulo en versalitas. Vacío, no hay encabezado.
	Titulo string
	// Acciones son los botones del encabezado, a la derecha del rótulo: la rueda de
	// «gestionar carpetas», el «＋» de una categoría nueva. Van con AccionLateral.
	Acciones []fyne.CanvasObject
	// Filas son los elementos de la sección.
	Filas []fyne.CanvasObject
	// AlPlegar convierte el rótulo en el botón que pliega. nil: la sección se ve
	// siempre y el rótulo no es pulsable.
	AlPlegar func()
	// Abierta solo cuenta con AlPlegar.
	Abierta bool
	// Plegado es el carril estrecho: sin rótulo, como en la web.
	Plegado bool

	plegador *Plegador // el rótulo pulsable, para el Tab del cajón
	// AccionesAlPasar esconde las acciones hasta que el puntero pasa por el encabezado
	// o una de ellas tiene el foco: el `opacity-0 group-hover/cat:opacity-100
	// group-focus-within/cat:opacity-100` de las categorías de Sytue Rooms. En un
	// teléfono se ven siempre (`pointer-coarse:opacity-100`). Siguen en su sitio y
	// enfocables: solo se apaga el glifo, como el `opacity-0` de la web.
	AccionesAlPasar bool
	enCabecera      bool
	// Diana es el `ring-2 ring-indicator ring-inset rounded-md` de la sección sobre la
	// que se soltaría lo que se está arrastrando (un canal o una categoría de Rooms). El
	// `mb-4` queda fuera del aro, como en la web.
	Diana bool
}

// MouseIn, MouseMoved y MouseOut siguen si el puntero está sobre el ENCABEZADO: la
// web mira el `group/cat` de su fila de título, no la sección entera.
func (s *SeccionLateral) MouseIn(e *desktop.MouseEvent) {
	s.pulsable.MouseIn(e)
	s.mirarCabecera(e)
}

func (s *SeccionLateral) MouseMoved(e *desktop.MouseEvent) { s.mirarCabecera(e) }

func (s *SeccionLateral) MouseOut() {
	s.enCabecera = false
	s.pulsable.MouseOut()
}

func (s *SeccionLateral) mirarCabecera(e *desktop.MouseEvent) {
	en := e != nil && e.Position.Y < AltoEncabezadoSeccion(s)
	if en != s.enCabecera {
		s.enCabecera = en
		s.Refresh()
	}
}

// accionesVisibles dice si las acciones se ven ahora.
func (s *SeccionLateral) accionesVisibles() bool {
	if !s.AccionesAlPasar || EsTactil() || s.enCabecera {
		return true
	}
	if p := s.plegador; p != nil && (p.encima || p.enfocado) {
		return true
	}
	for _, o := range s.Acciones {
		if a, es := o.(*AccionLateral); es && (a.encima || a.enfocado) {
			return true
		}
	}
	return false
}

var _ fyne.Widget = (*SeccionLateral)(nil)

// hijosDeCajon son los controles de la sección: el rótulo plegable, las acciones y las
// filas.
func (s *SeccionLateral) hijosDeCajon() []fyne.CanvasObject {
	out := append([]fyne.CanvasObject{}, s.Acciones...)
	if s.plegador != nil {
		out = append(out, s.plegador)
	}
	return append(out, s.Filas...)
}

// hijosDeCajon son la cabecera, el cuerpo y el pie del menú.
func (l *Lateral) hijosDeCajon() []fyne.CanvasObject {
	return []fyne.CanvasObject{l.Cabecera, l.Cuerpo, l.Pie}
}

// NuevaSeccionLateral crea una sección fija (sin plegar) con su rótulo y sus filas.
func NuevaSeccionLateral(titulo string, filas ...fyne.CanvasObject) *SeccionLateral {
	s := &SeccionLateral{Titulo: titulo, Filas: filas, Abierta: true}
	s.Nombre = titulo
	s.ExtendBaseWidget(s)
	return s
}

// SeccionLateralArrastrable es la sección cuya CABECERA se arrastra (el `draggable` de
// la fila del título de CategorySection.tsx): las categorías propias de Sytue Rooms, que
// se reordenan soltándolas sobre otra. Un arrastre que empieza en una fila no es de la
// sección y se ignora; las filas que se arrastran solas (un canal) lo reciben ellas.
type SeccionLateralArrastrable struct {
	SeccionLateral
	// AlArrastrar recibe el punto ABSOLUTO bajo el puntero; AlSoltar se llama al soltar.
	AlArrastrar func(fyne.Position)
	AlSoltar    func()

	arrastrando, ajeno bool
}

var _ fyne.Draggable = (*SeccionLateralArrastrable)(nil)

// NuevaSeccionLateralArrastrable es NuevaSeccionLateral que se arrastra por su cabecera.
func NuevaSeccionLateralArrastrable(titulo string, filas ...fyne.CanvasObject) *SeccionLateralArrastrable {
	s := &SeccionLateralArrastrable{SeccionLateral: SeccionLateral{Titulo: titulo, Filas: filas, Abierta: true}}
	s.Nombre = titulo
	s.ExtendBaseWidget(s)
	return s
}

// Dragged sigue al puntero si el arrastre empezó en la cabecera.
func (s *SeccionLateralArrastrable) Dragged(e *fyne.DragEvent) {
	if !s.arrastrando && !s.ajeno {
		inicio := e.Position.Subtract(e.Dragged)
		s.ajeno = inicio.Y >= AltoEncabezadoSeccion(&s.SeccionLateral)
		s.arrastrando = !s.ajeno
	}
	if s.arrastrando && s.AlArrastrar != nil {
		s.AlArrastrar(e.AbsolutePosition)
	}
}

// DragEnd suelta la sección donde esté el puntero.
func (s *SeccionLateralArrastrable) DragEnd() {
	if s.arrastrando && s.AlSoltar != nil {
		s.AlSoltar()
	}
	s.arrastrando, s.ajeno = false, false
}

// AccessibilityRole: una sección agrupa, no se pulsa. El rótulo plegable sí se anuncia
// como botón, y es un widget aparte.
func (s *SeccionLateral) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleContainer
}

// AltoEncabezadoSeccion es lo que ocupa la línea del rótulo con su `pt-1 pb-1.5`. Con
// acciones manda el botón (24) y no el rótulo (14), que es la razón de que un menú con
// rueda de ajustes tenga las secciones más altas que otro sin ella.
func AltoEncabezadoSeccion(s *SeccionLateral) float32 {
	if s.Titulo == "" || s.Plegado {
		return 0
	}
	h := tema.TextoXS.Linea
	if len(s.Acciones) > 0 {
		h = max(h, LadoAccionLateral)
	}
	return tema.Espacio(1) + h + tema.Espacio(1.5)
}

// MedidaSeccion es lo que ocupa la sección entera, `mb-4` incluido.
func MedidaSeccion(s *SeccionLateral) fyne.Size {
	alto := AltoEncabezadoSeccion(s) + margenAbajo
	ancho := float32(0)
	if s.AlPlegar == nil || s.Abierta {
		for i, f := range s.Filas {
			if i > 0 {
				alto += huecoFilas
			}
			m := f.MinSize()
			alto += m.Height
			ancho = max(ancho, m.Width)
		}
	}
	return fyne.NewSize(ancho, alto)
}

// CreateRenderer construye el encabezado y deja las filas tal cual.
func (s *SeccionLateral) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// El aro de la diana va DEBAJO de todo: `box-shadow inset` se pinta sobre el fondo
	// de la sección y bajo sus hijos.
	diana := canvas.NewRectangle(color.NRGBA{})
	diana.StrokeColor, diana.StrokeWidth, diana.CornerRadius = tema.Color("sidebar-indicator", v), 2, tema.RadioMD
	r := &seccionRenderer{s: s, diana: diana, objetos: []fyne.CanvasObject{diana}}
	if s.Titulo != "" && !s.Plegado {
		if s.AlPlegar != nil {
			r.plegador = nuevoPlegador(s)
			r.plegador.alCambio = s.Refresh
			s.plegador = r.plegador
			r.objetos = append(r.objetos, r.plegador)
		} else {
			r.rotulo = NuevoVersal(s.Titulo, tema.TextoXS, SemiNegrita, tema.Color("sidebar-fg-muted", v))
			r.objetos = append(r.objetos, r.rotulo.Objetos()...)
		}
		r.objetos = append(r.objetos, s.Acciones...)
	}
	r.objetos = append(r.objetos, s.Filas...)
	return r
}

type seccionRenderer struct {
	s        *SeccionLateral
	diana    *canvas.Rectangle
	rotulo   *Versal
	plegador *Plegador
	objetos  []fyne.CanvasObject
}

func (r *seccionRenderer) MinSize() fyne.Size           { return MedidaSeccion(r.s) }
func (r *seccionRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *seccionRenderer) Destroy()                     {}

// Layout coloca encabezado y filas. Las filas ocupan TODO el ancho, que es el
// `flex flex-col` con `items-stretch` de la web.
func (r *seccionRenderer) Layout(t fyne.Size) {
	s := r.s
	// El trazo de Fyne va centrado en el contorno: 2 px hacia dentro son un rectángulo
	// metido 1 px con trazo de 2.
	r.diana.Hidden = !s.Diana
	r.diana.Move(fyne.NewPos(1, 1))
	r.diana.Resize(fyne.NewSize(max(t.Width-2, 0), max(t.Height-margenAbajo-2, 0)))
	y := AltoEncabezadoSeccion(s)
	if y > 0 {
		// pt-1 pr-1 pb-1.5 pl-2.5: el rótulo cae bajo el icono de las filas.
		linea := y - tema.Espacio(1) - tema.Espacio(1.5)
		x := t.Width - tema.Espacio(1)
		for i := len(s.Acciones) - 1; i >= 0; i-- {
			m := s.Acciones[i].MinSize()
			x -= m.Width
			s.Acciones[i].Move(fyne.NewPos(x, tema.Espacio(1)+(linea-m.Height)/2))
			s.Acciones[i].Resize(m)
			x -= tema.Espacio(0.5) // gap-0.5 entre acciones
		}
		switch {
		case r.plegador != nil:
			m := r.plegador.MinSize()
			// `-ml-1 pl-1`: el galón se sale un paso a la izquierda para que la
			// primera letra caiga donde caería sin él. Y `min-h-[24px] -my-[5px]`: el
			// rótulo solo mide su línea (14) y es un botón, así que su zona pulsable
			// crece a los 24 de WCAG 2.5.8 y devuelve los 10 px con el margen; la
			// cabecera mide lo mismo que antes (web, PR #836).
			// Y `items-center`: la caja de 14 que deja el margen negativo se centra en
			// la línea, que con acciones mide 24.
			alto := max(m.Height, LadoIconoSM)
			r.plegador.Move(fyne.NewPos(RellenoNav()-tema.Espacio(1), tema.Espacio(1)+(linea-alto)/2))
			r.plegador.Resize(fyne.NewSize(min(m.Width, t.Width), alto))
		case r.rotulo != nil:
			// `flex items-center`: con acciones la línea mide 24 y el rótulo va en su
			// centro, no arriba. Salía 5 px alto en las secciones con rueda o «+».
			r.rotulo.Colocar(RellenoNav(), tema.Espacio(1)+(linea-tema.TextoXS.Linea)/2)
		}
	}
	if s.AlPlegar != nil && !s.Abierta {
		// ESCONDER, no encoger. Redimensionar a 0×0 es lo que había y no basta: un
		// `canvas.Text` a cero sigue pintando su cadena entera —el pintor no lo recorta
		// a la caja del objeto—, así que la lista de ajustes de Sytue Mail se veía
		// completa debajo del rótulo plegado. El `mb-4` se conserva igual, que lo pone
		// `MedidaSeccion` y no las filas.
		for _, f := range s.Filas {
			f.Hide()
		}
		return
	}
	for i, f := range s.Filas {
		f.Show()
		if i > 0 {
			y += huecoFilas
		}
		h := f.MinSize().Height
		f.Move(fyne.NewPos(0, y))
		f.Resize(fyne.NewSize(t.Width, h))
		y += h
	}
}

// Refresh repinta el rótulo y rehace la disposición: plegar una sección no cambia el
// tamaño que Fyne le dio, y sin el Layout las filas se quedarían donde estaban.
func (r *seccionRenderer) Refresh() {
	r.diana.StrokeColor = tema.Color("sidebar-indicator", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.diana.Refresh()
	r.Layout(r.s.Size())
	ver := r.s.accionesVisibles()
	for _, o := range r.s.Acciones {
		if a, es := o.(*AccionLateral); es {
			a.alCambio = r.s.Refresh
			if a.apagada == ver {
				a.apagada = !ver
				a.Refresh()
			}
		}
	}
	if r.rotulo == nil {
		return
	}
	r.rotulo.Pintar(tema.Color("sidebar-fg-muted", tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
}

// RamaLateral es lo que cambia en una fila del ÁRBOL del menú (NotebookTree.tsx): la
// sangría la pone el nivel en vez del canalón fijo, el relleno vertical es menor, y la
// fila activa se tiñe entera SIN la barra de cobre —la web no la pinta ahí, y con ella
// dos niveles seguidos parecían dos secciones distintas—.
//
// Los dos números no se declaran aquí porque no son del menú sino del árbol de cada
// app: Notepads usa `py-1.5` y `0,5 + (nivel−1) × 0,85 rem` para un cuaderno, y `py-1`
// con `0,75 + nivel × 0,85 rem` para la nota que cuelga de él.
type RamaLateral struct {
	// Sangria es el relleno izquierdo de este nivel.
	Sangria float32
	// RellenoY es el `py-*` de la rama.
	RellenoY float32
	// Hoja atenúa el texto y, al estar activa, deja el fondo al 60 %.
	Hoja bool
	// CajaIcono es el hueco que ocupa la marca de la izquierda. Cero: los 17,5 de
	// `h-5 w-5` del menú.
	//
	// Una rama NO siempre envuelve su glifo en esa caja: la nota que cuelga de un
	// cuaderno en Notepads lleva un `<Glyph size={12}>` suelto, y con los 17,5 la fila
	// salía 24,5 donde producción mide 21. El alto de una fila es
	// `max(caja de línea, caja del icono, cola) + 2 × py`, así que este número entra
	// directo en la medida.
	CajaIcono float32
	// Cola es lo que va pegado al borde derecho de la rama, a su medida mínima: en
	// Notepads son el distintivo de visibilidad, el contador y los dos botones de
	// acción que aparecen al pasar por encima.
	//
	// No es `Contador`, que es el `<Badge count>` de 18 del menú de Mail. Y ENTRA EN EL
	// ALTO: los dos botones de acción del árbol de cuadernos son `p-1` sobre un icono
	// de 13, o sea 20, y son EXACTAMENTE los 0,75 que separan los 29,75 que dan
	// `py-1.5` + `text-base` de los 30,5 medidos en producción el 2026-09-21. Van en el
	// layout aunque estén a `opacity-0`, que es lo que hace que la fila no cambie de
	// alto al pasar el ratón.
	Cola fyne.CanvasObject
	// ColaAlPasar es la PARTE de la cola que solo se ve al pasar por encima o al
	// enfocar la fila: el `opacity-0 group-hover:opacity-100` de la web, que envuelve
	// los botones de acción y NO el contador ni el distintivo de visibilidad.
	//
	// Tiene que ser un hijo de `Cola` —esta fila solo lo muestra y lo oculta— y la
	// medida de `Cola` tiene que contarlo AUNQUE ESTÉ OCULTO, o la fila daría un salto
	// de alto cada vez que el ratón la cruza. En la web es `opacity-0` y no
	// `display:none` por exactamente lo mismo, y de ahí salen los 0,75 de los 30,5.
	ColaAlPasar fyne.CanvasObject
	// Pliegue es el chevron de una rama con hijos, en su hueco de `LadoPliegue` a la
	// izquierda de la marca y a `gap-1.5` de ella (el `w-[13px]` de NotebookTree.tsx). Con
	// `LadoPliegue` y sin Pliegue, el hueco se queda vacío: así los nombres de un nivel
	// caen en la misma vertical tengan hijos o no. El botón NO cuenta en el alto: en la
	// web lleva `-m-1 p-1` y ocupa sus 13.
	//
	// Si es un `*BotonIcono`, pasarle el puntero no apaga el `:hover` de la fila, que es lo
	// que hace el navegador con un hijo: las acciones de la cola siguen a la vista.
	Pliegue     fyne.CanvasObject
	LadoPliegue float32
}

// FilaLateral es una fila del menú: `SidebarItem`, la fila compacta del pie o una rama
// del árbol, según cómo se declare.
type FilaLateral struct {
	pulsable

	// Rotulo es el texto de la fila y también su nombre accesible.
	Rotulo string
	// Icono es el glifo de la izquierda, que se pinta en una caja de 17,5. nil: sin
	// icono. Lo trae quien monta la pantalla: el juego de iconos no es de esta pieza.
	Icono fyne.CanvasObject
	// Punto sustituye al icono por un círculo de 8,75 centrado en la misma caja: es
	// como la web marca una etiqueta de correo, cuyo color es un DATO y no un token.
	// Alfa cero: no hay punto.
	Punto color.NRGBA
	// Contador es el `<Badge count>` de la derecha: los no leídos. Vacío, no se pinta.
	Contador string
	// Activa es `aria-current="page"`: fondo propio y barra de cobre a la izquierda.
	Activa bool
	// Plegada es el carril estrecho: solo el icono, centrado.
	Plegada bool
	// Compacta es la fila del PIE (`px-1.5 py-1.5 text-sm`): «Ver estos archivos en tu
	// ordenador», el medidor de espacio. Medida en producción: 28,5 de alto contra los
	// 33,25 de un destino de navegación, que es lo que la separa de la lista de arriba.
	Compacta bool
	// Borde es el token del filete de la fila. Vacío: sin filete, que es lo normal.
	// Lo lleva la caja de «Ir a un canal…» de Sytue Rooms, que es un BOTÓN con pinta
	// de campo —`border-sidebar-fg-muted/40`— porque lo que abre es el conmutador
	// rápido y no un filtro de la lista.
	Borde string
	// Opacidad del filete, para lo que la hoja escribe con `/N`. Cero es opaco.
	OpacidadBorde float32
	// CajaIcono sustituye el hueco del icono. Cero es el de siempre: `grid h-5 w-5`,
	// 17,5. Sytue Rooms escribe `grid h-6 w-6` en TODAS sus filas y por eso las suyas
	// miden 35 y no 33,25 —medido en producción, `rooms/canal.ancho.json`—: el hueco
	// del icono (21) pasa a mandar sobre la caja de línea del texto (19,25).
	CajaIcono float32
	// Cola es lo que va PEGADO A LA DERECHA de la fila, después del contador: hoy la
	// pastilla del atajo («Ctrl+G» en «Ir a un canal…» de Sytue Rooms, que la web
	// pinta con un `<kbd>`). nil: nada. Una fila del árbol trae la suya en `Rama`, y
	// esa manda.
	Cola fyne.CanvasObject
	// Rama convierte la fila en una del árbol. nil: la fila de siempre.
	Rama *RamaLateral
	// Tachada es el `line-through` del rótulo: la tarea hecha del árbol de Notepads.
	Tachada bool
	// Accion es el `action` de SidebarItem: el botón PROPIO de la fila —«Actualizar» la
	// carpeta que se está viendo en Sytue Mail—, HERMANO del enlace y no dentro, en
	// `absolute right-1` y centrado en alto. La fila le deja su hueco con `pr-9`, así que
	// cae en la misma columna que las acciones del encabezado de sección. No se pinta en
	// el carril plegado (`action && !collapsed`).
	Accion *AccionLateral

	// encimaHijo es que el puntero está sobre el pliegue: para la fila cuenta como
	// encima, igual que el `:hover` de un padre en la web.
	encimaHijo bool
	// titulo es la pista del carril plegado (el `title` de la web), que abre la fila.
	titulo *Pista
}

// MouseIn es el `:hover`, y en el carril plegado arranca además su `title`.
func (f *FilaLateral) MouseIn(e *desktop.MouseEvent) {
	f.pulsable.MouseIn(e)
	f.titulo.alPuntero(!f.Inerte)
}

// MouseOut devuelve la fila al reposo y recoge el `title`.
func (f *FilaLateral) MouseOut() {
	f.pulsable.MouseOut()
	f.titulo.alPuntero(false)
}

// hijosDeCajon son el pliegue, la cola y la acción de la fila.
func (f *FilaLateral) hijosDeCajon() []fyne.CanvasObject {
	out := []fyne.CanvasObject{f.Cola}
	if f.Accion != nil {
		out = append(out, f.Accion)
	}
	if f.Rama != nil {
		out = append(out, f.Rama.Pliegue, f.Rama.Cola)
	}
	return out
}

// pliegue es el hueco del chevron de una rama: su ancho con el `gap`, o cero.
func (f *FilaLateral) pliegue(m metricaFila) float32 {
	if f.Rama == nil || f.Rama.LadoPliegue == 0 {
		return 0
	}
	return f.Rama.LadoPliegue + m.hueco
}

// avisaEncima es el control que le cuenta a su fila si tiene el puntero encima.
type avisaEncima interface{ avisarEncima(func(bool)) }

// cola es lo que se pinta a la derecha del todo: la de la rama si la fila es del
// árbol, y si no la suya.
func (f *FilaLateral) cola() fyne.CanvasObject {
	if f.Rama != nil {
		return f.Rama.Cola
	}
	return f.Cola
}

var _ fyne.Widget = (*FilaLateral)(nil)

// NuevaFilaLateral crea una fila de navegación con su rótulo.
func NuevaFilaLateral(rotulo string, icono fyne.CanvasObject, alPulsar func()) *FilaLateral {
	f := &FilaLateral{Rotulo: rotulo, Icono: icono}
	f.AlPulsar = alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// AccessibilityLabel es el rótulo, que es lo que anuncia la web con su `title`.
func (f *FilaLateral) AccessibilityLabel() string { return f.Rotulo }

// metrica son los cinco números que cambian entre las tres clases de fila.
type metricaFila struct {
	px, py, hueco float32
	// pxDer es el relleno DERECHO cuando no es el mismo que el izquierdo: una rama
	// del árbol lleva `pr-1` fijo y una sangría que crece con el nivel.
	pxDer float32
	texto tema.Escalon
	peso  Peso
	// cajaIcono es el alto que el icono aporta: 17,5 con su `span`, 18 a pelo.
	cajaIcono float32
}

func (f *FilaLateral) metrica() metricaFila {
	m := f.metricaBase()
	if f.CajaIcono > 0 {
		m.cajaIcono = f.CajaIcono
	}
	if f.conAccion() {
		m.pxDer = tema.Espacio(9) // `action && !collapsed && 'pr-9'`
	}
	return m
}

// conAccion dice si la acción de la fila se pinta: no en el carril plegado.
func (f *FilaLateral) conAccion() bool { return f.Accion != nil && !f.Plegada }

// MargenAccionFila es el `right-1` de la acción de una fila.
var MargenAccionFila = tema.Espacio(1)

func (f *FilaLateral) metricaBase() metricaFila {
	switch {
	case f.Compacta:
		// `px-1.5 py-1.5 gap-2 text-sm` con un glifo de 18 SIN caja: por eso 28,5.
		return metricaFila{
			px: tema.Espacio(1.5), py: tema.Espacio(1.5), hueco: huecoFila,
			texto: tema.TextoSM, peso: Normal, cajaIcono: GlifoMenu,
		}
	case f.Rama != nil:
		// sangría · `pr-1` · `gap-1.5`, y el `py` que traiga la rama.
		caja := f.Rama.CajaIcono
		if caja == 0 {
			caja = CajaIcono
		}
		texto := tema.TextoBase
		if f.Rama.Hoja {
			texto = tema.TextoXS // la nota que cuelga va en `text-xs`
		}
		return metricaFila{
			px: f.Rama.Sangria, pxDer: tema.Espacio(1), py: f.Rama.RellenoY,
			hueco: tema.Espacio(1.5), texto: texto, peso: Normal, cajaIcono: caja,
		}
	}
	// `px-2.5 py-2 gap-2 text-base font-medium`: la fila de 33,25. El activo, con el peso
	// de «estás aquí» de la forma (`[font-weight:var(--sytue-nav-active-weight)]`).
	peso := Medio
	if f.Activa {
		peso = PesoNavActivo()
	}
	return metricaFila{
		px: RellenoNav(), pxDer: RellenoNav(), py: filaPY, hueco: huecoFila,
		texto: tema.TextoBase, peso: peso, cajaIcono: CajaIcono,
	}
}

// MedidaFilaLateral es lo que va a ocupar la fila. Es la ÚNICA cuenta: el `MinSize()`
// del renderer la llama y el guardián compara contra ella y contra los píxeles.
//
// El ancho es el mínimo; en el menú la fila se estira a los 231 de ancho útil.
// Con el dedo, también las filas compactas y plegadas conservan los 44 px de alto
// del resto de controles de navegación; el cuerpo se desplaza en vez de encogerlas.
func MedidaFilaLateral(f *FilaLateral) fyne.Size {
	m := f.metrica()
	alto, ancho := m.texto.Linea, f.pliegue(m)
	conMarca := f.Icono != nil || f.Punto.A > 0
	if conMarca {
		alto = max(alto, m.cajaIcono)
		ancho += m.cajaIcono
	}
	if f.Plegada {
		return fyne.NewSize(ancho+2*m.px, SueloTactil(alto+2*m.py))
	}
	if conMarca {
		ancho += m.hueco
	}
	if f.Contador != "" {
		alto = max(alto, AltoContador)
		ancho += m.hueco + MedidaContador(f.Contador).Width
	}
	if cola := f.cola(); cola != nil {
		c := cola.MinSize()
		alto = max(alto, c.Height)
		ancho += m.hueco + c.Width
	}
	// El rótulo, en las líneas que ocupe en el ancho útil del menú, que es fijo: la
	// fila crece solo cuando no cabe en una.
	cabe := AnchoUtilMenu - ancho - m.px - m.pxDer
	lineas := f.lineasRotulo(m, cabe)
	alto = max(alto, float32(len(lineas))*m.texto.Linea)
	// De ancho, el rótulo pide lo que ocupa pero NUNCA más que el ancho útil: es el
	// `min-w-0` de la web. Sin ese tope, un rótulo largo ensanchaba el menú entero y se
	// salía por la derecha en vez de pasar a la segunda línea.
	rotulo := Medir(f.Rotulo, m.texto, m.peso).Width
	if f.Rama == nil {
		rotulo = min(rotulo, max(cabe, 0))
	}
	ancho += rotulo
	return fyne.NewSize(ancho+m.px+m.pxDer, SueloTactil(alto+2*m.py))
}

// lineasRotulo reparte el rótulo en `cabe`. Una fila del menú va en `line-clamp-2
// break-words`: hasta DOS líneas y solo después la elipsis. Era `truncate`, y con los
// veintiséis idiomas cortaba rótulos fijos de la suite —«Buzones compartidos y
// delegación» en alemán, «Ver estos archivos en tu ordenador» en neerlandés— (web, PR
// #844). Las ramas de un árbol (Notepads) siguen en una línea, como en la web.
//
// ponytail: una palabra más larga que la fila entera se sale en vez de partirse
// (`break-words`); ningún rótulo de los veintiséis catálogos llega a eso a 231.
func (f *FilaLateral) lineasRotulo(m metricaFila, cabe float32) []string {
	if f.Rama != nil {
		return []string{recorta(f.Rotulo, m.texto, m.peso, cabe)}
	}
	if l := LineasDe(&Parrafo{Texto: f.Rotulo, Escalon: m.texto, Peso: m.peso, Lineas: 2}, cabe); len(l) > 0 {
		return l
	}
	return []string{""}
}

// colores devuelve fondo y color de texto según el estado, con los tokens de
// Sidebar.tsx uno a uno.
func (f *FilaLateral) colores(v tema.Variante) (fondo, texto color.NRGBA) {
	// En reposo, ATENUADA (`text-sidebar-fg-muted hover:text-sidebar-fg`): sobre el
	// carril todo es blanco, y si las filas fueran tan blancas como la activa, a esa solo
	// la delataría el velo. La fila compacta del pie y las ramas de un árbol no son
	// `SidebarItem` y conservan su tinta.
	texto = tema.Color("sidebar-fg", v)
	destino := f.Rama == nil && !f.Compacta
	if destino || f.Rama != nil && f.Rama.Hoja {
		texto = tema.Color("sidebar-fg-muted", v)
	}
	velo := func(c color.NRGBA, x float32) color.NRGBA {
		c.A = uint8(float32(c.A) * x)
		return c
	}
	switch {
	case f.Activa:
		// `bg-sidebar-active text-sidebar-active-fg`, que theme.css saca de los tokens de
		// la forma (`--sytue-nav-active-*`): el velo del menú con su texto, en las dos.
		fondo, texto = tema.Color("nav-active-bg", v), tema.Color("nav-active-fg", v)
		if f.Rama != nil && f.Rama.Hoja {
			fondo = velo(fondo, 0.6) // `bg-sidebar-active/60`
		}
	case f.Inerte: // `disabled:` congela el reposo y luego baja la opacidad
	case f.pulsado:
		fondo = velo(tema.Color("nav-active-bg", v), 0.6) // `active:bg-sidebar-active/60`
	case f.encima || f.encimaHijo:
		fondo = tema.Color("sidebar-hover-bg", v)
		if destino {
			texto = tema.Color("sidebar-fg", v)
		}
	}
	return fondo, texto
}

// AnchoSenal es el `before:w-[var(--sytue-nav-indicator)]` de la barra ámbar de «estás
// aquí»: de la forma. En Carril son 3 px; en Marco no la hay, porque el ítem activo ya
// es una píldora.
func AnchoSenal() float32 { return tema.Forma("nav-indicator") }

// radio es el de la caja de la fila: `rounded-nav` en un destino del menú —de la forma:
// píldora en Marco, `md` en Carril—; la fila compacta del pie y las ramas de un árbol
// siguen en `rounded-sm`.
func (f *FilaLateral) radio() float32 {
	if f.Rama == nil && !f.Compacta {
		return tema.Forma("radius-nav")
	}
	return tema.RadioSM
}

// borde es el color del filete de la fila, ya con su opacidad. Alfa cero: sin filete,
// y entonces `Caja` no pinta ni el rectángulo de fuera.
func (f *FilaLateral) borde(v tema.Variante) color.NRGBA {
	if f.Borde == "" {
		return color.NRGBA{}
	}
	c := tema.Color(f.Borde, v)
	if f.OpacidadBorde > 0 {
		c.A = uint8(float32(c.A) * f.OpacidadBorde)
	}
	return enTinta(c, f.Inerte)
}

// CreateRenderer construye la geometría. Solo primitivas y el icono que le den.
func (f *FilaLateral) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	m := f.metrica()
	fondo, texto := f.colores(v)
	r := &filaRenderer{
		f:       f,
		aro:     NuevoAroCon("sidebar-focus", f.radio(), SinHueco, v).Abierto(), // transparente en reposo
		caja:    NuevaCaja(enTinta(fondo, f.Inerte), f.borde(v), f.radio()),
		rotulo:  Texto(f.Rotulo, m.texto, m.peso, enTinta(texto, f.Inerte)),
		rotulo2: Texto("", m.texto, m.peso, enTinta(texto, f.Inerte)),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	if f.Activa && f.Rama == nil {
		r.barra = canvas.NewRectangle(tema.Color("sidebar-indicator", v))
		r.objetos = append(r.objetos, r.barra)
	}
	switch {
	case f.Icono != nil:
		r.objetos = append(r.objetos, f.Icono)
	case f.Punto.A > 0:
		r.punto = canvas.NewCircle(f.Punto)
		r.objetos = append(r.objetos, r.punto)
	}
	if f.Rama != nil && f.Rama.Pliegue != nil {
		if a, es := f.Rama.Pliegue.(avisaEncima); es {
			a.avisarEncima(func(si bool) { f.encimaHijo = si; f.Refresh() })
		}
		r.objetos = append(r.objetos, f.Rama.Pliegue)
	}
	if !f.Plegada {
		r.tacha, r.tacha2 = canvas.NewRectangle(texto), canvas.NewRectangle(texto)
		r.objetos = append(r.objetos, r.rotulo, r.rotulo2, r.tacha, r.tacha2)
		if f.Contador != "" {
			r.contador = NuevoContador(f.Contador)
			r.contador.SobreCromo = true
			r.objetos = append(r.objetos, r.contador)
		}
		if cola := f.cola(); cola != nil {
			r.objetos = append(r.objetos, cola)
		}
		if f.conAccion() {
			r.objetos = append(r.objetos, f.Accion)
		}
	}
	if f.Plegada {
		// El `title={label}` de SidebarItem: en el carril plegado solo se ve el icono, y
		// el rótulo sale al pasar el ratón. La pista va en una caja de ANCHO CERO pegada
		// al borde derecho de la fila, para que el puntero no la encuentre nunca y siga
		// siendo de la fila (el `:hover`), y la burbuja cae a la derecha del carril.
		r.titulo = NuevaPista(f.Rotulo, canvas.NewRectangle(color.NRGBA{}))
		r.titulo.Derecha = true
		f.titulo = r.titulo
		r.objetos = append(r.objetos, r.titulo)
	}
	r.fondo.hacia(enTinta(fondo, f.Inerte), r.pintarFondo)
	return r
}

type filaRenderer struct {
	f       *FilaLateral
	aro     *Aro
	caja    *Caja
	barra   *canvas.Rectangle
	punto   *canvas.Circle
	rotulo  *canvas.Text
	rotulo2 *canvas.Text // la segunda línea del rótulo, cuando no cabe en una
	// tacha y tacha2 son el `line-through` de cada línea del rótulo.
	tacha, tacha2 *canvas.Rectangle
	contador      *Contador
	objetos       []fyne.CanvasObject
	// fondo es el `transition-colors duration-150` de SidebarItem.
	fondo transicion
	// titulo es el `title` del carril plegado.
	titulo *Pista
}

func (r *filaRenderer) pintarFondo(c color.NRGBA) {
	r.caja.Pintar(c, r.f.borde(tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
}

func (r *filaRenderer) MinSize() fyne.Size           { return MedidaFilaLateral(r.f) }
func (r *filaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaRenderer) Destroy()                     {}

// Layout coloca marca, rótulo y contador dentro de la caja, todo centrado en vertical
// (`items-center`).
func (r *filaRenderer) Layout(t fyne.Size) {
	f, m := r.f, r.f.metrica()
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	if r.barra != nil {
		// «Estás aquí»: `before:top-2 before:bottom-2 before:left-0 before:w-[3px]`.
		margen := tema.Espacio(2)
		r.barra.Move(fyne.NewPos(0, margen))
		r.barra.Resize(fyne.NewSize(AnchoSenal(), max(t.Height-2*margen, 0)))
		r.barra.CornerRadius = AnchoSenal() / 2 // `rounded-full`
		r.barra.Hidden = AnchoSenal() <= 0
	}
	centrado := func(o fyne.CanvasObject, x, lado float32) {
		o.Move(fyne.NewPos(x, (t.Height-lado)/2))
		o.Resize(fyne.NewSize(lado, lado))
	}
	x := m.px
	if f.Plegada { // `justify-center`
		x = (t.Width - m.cajaIcono) / 2
	}
	if p := f.pliegue(m); p > 0 {
		if b := f.Rama.Pliegue; b != nil {
			s := b.MinSize()
			b.Move(fyne.NewPos(x+(f.Rama.LadoPliegue-s.Width)/2, (t.Height-s.Height)/2))
			b.Resize(s)
		}
		x += p
	}
	switch {
	case f.Icono != nil:
		centrado(f.Icono, x, m.cajaIcono)
		x += m.cajaIcono + m.hueco
	case f.Punto.A > 0:
		centrado(r.punto, x+(m.cajaIcono-PuntoEtiqueta)/2, PuntoEtiqueta)
		x += m.cajaIcono + m.hueco
	}
	if f.Plegada {
		if r.titulo != nil {
			r.titulo.Move(fyne.NewPos(t.Width, 0))
			r.titulo.Resize(fyne.NewSize(0, t.Height))
		}
		return
	}
	if f.conAccion() {
		s := f.Accion.MinSize()
		f.Accion.Move(fyne.NewPos(t.Width-MargenAccionFila-s.Width, (t.Height-s.Height)/2))
		f.Accion.Resize(s)
	}
	der := t.Width - m.pxDer
	if cola := f.cola(); cola != nil {
		c := cola.MinSize()
		der -= c.Width
		cola.Move(fyne.NewPos(der, (t.Height-c.Height)/2))
		cola.Resize(c)
		der -= m.hueco
	}
	if r.contador != nil {
		c := MedidaContador(f.Contador)
		der -= c.Width
		r.contador.Move(fyne.NewPos(der, (t.Height-c.Height)/2))
		r.contador.Resize(c)
		der -= m.hueco
	}
	lineas := f.lineasRotulo(m, der-x)
	y := (t.Height - float32(len(lineas))*m.texto.Linea) / 2
	r.rotulo.Text = lineas[0]
	ColocarTexto(r.rotulo, m.texto, x, y)
	r.rotulo2.Hidden = len(lineas) < 2
	if len(lineas) > 1 {
		r.rotulo2.Text = lineas[1]
		ColocarTexto(r.rotulo2, m.texto, x, y+m.texto.Linea)
	}
	tachar(r.tacha, f.Tachada, lineas[0], m, x, y)
	tachar(r.tacha2, f.Tachada && len(lineas) > 1, ultima(lineas), m, x, y+m.texto.Linea)
}

func ultima(l []string) string { return l[len(l)-1] }

// tachar pone el `line-through` sobre una línea del rótulo que empieza en (x, y). Va
// donde lo pone el navegador, en el `yStrikeoutPosition` de Figtree —0,3 em sobre la
// línea base, con 0,05 em de grueso, que a estos tamaños es un píxel—. La línea base
// sale de centrar la caja de la fuente (1,2 em, ascendente 0,95) en la caja de línea,
// que es lo que hace `ColocarTexto`.
func tachar(r *canvas.Rectangle, si bool, texto string, m metricaFila, x, y float32) {
	if !si || texto == "" {
		r.Hide()
		return
	}
	t := m.texto.Tam
	base := y + (m.texto.Linea-1.2*t)/2 + 0.95*t
	r.Move(fyne.NewPos(x, base-0.3*t))
	r.Resize(fyne.NewSize(Medir(texto, m.texto, m.peso).Width, tema.Borde))
	r.Show()
}

// Refresh repinta con los colores del estado actual sin reconstruir la geometría.
func (r *filaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f := r.f
	fondo, texto := f.colores(v)
	// El radio es de la forma, que puede cambiar con la app abierta.
	r.caja.CambiarRadio(f.radio())
	r.aro.CambiarRadio(f.radio())
	r.aro.Ver(f.enfocado)
	r.aro.Pintar(v)
	r.fondo.hacia(enTinta(fondo, f.Inerte), r.pintarFondo)
	if r.barra != nil {
		r.barra.FillColor = tema.Color("sidebar-indicator", v)
		r.barra.Refresh()
	}
	r.rotulo.Color = enTinta(texto, f.Inerte)
	// La cara sigue al estado: el activo cambia de peso con la forma.
	cara := f.metrica().peso.cara()
	r.rotulo.FontSource, r.rotulo2.FontSource = cara, cara
	// El icono de un destino sigue a la tinta del rótulo —atenuado en reposo, claro al
	// pasar y en el activo—, que es lo que hace `currentColor` en la web. Solo si es un
	// icono del menú: uno con color propio (el de un calendario, una estrella) no se toca.
	if d := iconos.De(f.Icono); d != nil && d.DelCarril() && f.Rama == nil && !f.Compacta {
		d.TenirCon(enTinta(texto, f.Inerte))
	}
	r.rotulo.Refresh()
	r.rotulo2.Color = r.rotulo.Color
	r.rotulo2.Refresh()
	if r.tacha != nil {
		r.tacha.FillColor, r.tacha2.FillColor = r.rotulo.Color, r.rotulo.Color
		r.Layout(f.Size())
	}
	if f.Rama != nil && f.Rama.ColaAlPasar != nil {
		if f.encima || f.encimaHijo || f.enfocado {
			f.Rama.ColaAlPasar.Show()
		} else {
			f.Rama.ColaAlPasar.Hide()
		}
	}
}

// LadoAccionLateral es SIDEBAR_ACTION_CLASS: `h-6 w-6 min-h-[24px] min-w-[24px]`, o
// sea 24 px FIJOS y no 1,5 rem. Hasta el 2026-09-22 la web decía 24 y medía 21 —el rem
// de la suite es 14—, y dos acciones pegadas con `gap-0.5` no cabían en la excepción de
// separación de WCAG 2.5.8; la web le puso el suelo en px de `IconButton sm` (PR #834)
// y aquí es la misma constante que aquel.
const LadoAccionLateral = LadoIconoSM

// AccionLateral es el botón de solo icono del encabezado de una sección: la rueda de
// «gestionar carpetas», el «＋» de una categoría nueva. Va sobre la tinta y sin texto,
// así que el nombre accesible es obligatorio.
type AccionLateral struct {
	pulsable

	// Icono es el glifo, a su tamaño (el `<Icon size>` de la web) y centrado en la caja de 24.
	// Lo pinta quien lo trae, con su color: para el de la hoja va Glifo.
	Icono fyne.CanvasObject
	// Glifo es el mismo icono dejando el COLOR a la acción, que es lo que hace la web:
	// `text-sidebar-fg-muted hover:text-sidebar-fg` en SIDEBAR_ACTION_CLASS, heredado por
	// `currentColor`. Gana a Icono. Ver NuevaAccionLateralCon.
	Glifo *iconos.Dibujo
	// Girando es el `animate-spin` del glifo: el «Actualizar» de Sytue Mail mientras la
	// carpeta carga («sin eso, pulsar sobre una bandeja que no cambia no se distingue de
	// un botón que no hace nada»). Con SinMovimiento se queda quieto.
	Girando bool
	// apagada es el `opacity-0` de una acción que solo se ve al pasar (ver
	// `SeccionLateral.AccionesAlPasar`): el glifo no se pinta y el botón sigue ahí.
	apagada bool
	giro    *fyne.Animation
}

// DuracionGiro es el `animate-spin` de Tailwind: `spin 1s linear infinite`.
const DuracionGiro = time.Second

// NuevaAccionLateralCon crea el botón con un glifo que tiñe ÉL, como la web: apagado en
// reposo y en `sidebar-fg` al pasar el ratón.
func NuevaAccionLateralCon(nombre string, glifo *iconos.Dibujo, alPulsar func()) *AccionLateral {
	a := NuevaAccionLateral(nombre, nil, alPulsar)
	a.Glifo = glifo
	return a
}

// icono es lo que se coloca en la caja: el glifo que tiñe la acción o el que trae su color.
func (a *AccionLateral) icono() fyne.CanvasObject {
	if a.Glifo != nil {
		return a.Glifo.Objeto()
	}
	return a.Icono
}

// girar arranca o para el `animate-spin`. Una animación por acción que gira, y solo
// mientras gira: la web no anima lo que está quieto.
func (a *AccionLateral) girar() {
	quiere := a.Girando && a.Glifo != nil && !SinMovimiento
	switch {
	case quiere && a.giro == nil:
		a.giro = fyne.NewAnimation(DuracionGiro, func(p float32) { a.Glifo.Girar(360 * p) })
		a.giro.Curve = fyne.AnimationLinear
		a.giro.RepeatCount = fyne.AnimationRepeatForever
		a.giro.Start()
	case !quiere && a.giro != nil:
		a.giro.Stop()
		a.giro = nil
		a.Glifo.Girar(0)
	}
}

var _ fyne.Widget = (*AccionLateral)(nil)

// NuevaAccionLateral crea el botón. `nombre` es su `aria-label`, ya traducido.
func NuevaAccionLateral(nombre string, icono fyne.CanvasObject, alPulsar func()) *AccionLateral {
	a := &AccionLateral{Icono: icono}
	a.Nombre, a.AlPulsar = nombre, alPulsar
	a.ExtendBaseWidget(a)
	return a
}

// MedidaAccionLateral es el cuadrado de 24 de SIDEBAR_ACTION_CLASS.
func MedidaAccionLateral() fyne.Size {
	return fyne.NewSize(LadoAccionLateral, LadoAccionLateral)
}

// CreateRenderer construye el botón: fondo, anillo y el icono centrado.
func (a *AccionLateral) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &accionRenderer{
		a:    a,
		aro:  NuevoAroCon("sidebar-focus", tema.RadioSM, SinHueco, v).Abierto(), // transparente en reposo
		caja: NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioSM),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	if a.Glifo != nil {
		// La acción tiñe el glifo entre dos tintas con una transición, así que por el
		// color no se le reconoce el carril: se le dice.
		a.Glifo.EnCarril()
	}
	if o := a.icono(); o != nil {
		r.objetos = append(r.objetos, o)
	}
	r.Refresh()
	return r
}

type accionRenderer struct {
	a       *AccionLateral
	aro     *Aro
	caja    *Caja
	objetos []fyne.CanvasObject
	// fondo y tinta son el `transition-colors` de SIDEBAR_ACTION_CLASS.
	fondo, tinta transicion
}

func (r *accionRenderer) MinSize() fyne.Size           { return MedidaAccionLateral() }
func (r *accionRenderer) Objects() []fyne.CanvasObject { return r.objetos }

// Destroy para el giro: el renderer se tira cuando la acción deja de estar en pantalla
// (Mail rehace su menú con cada dato), y una animación infinita sin nadie que la mire
// seguiría corriendo hasta que muera el proceso.
func (r *accionRenderer) Destroy() {
	if r.a.giro != nil {
		r.a.giro.Stop()
		r.a.giro = nil
	}
}

// Layout centra el icono en la caja, a SU tamaño: quien llama pasa el `<Icon size>` de
// la web (16 en Mail, Calendar y Notepads). Se forzaba a 17,5 y el glifo salía un 9 %
// más grande.
func (r *accionRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	if o := r.a.icono(); o != nil {
		m := o.MinSize()
		if m.Width == 0 || m.Height == 0 {
			m = fyne.NewSize(CajaIcono, CajaIcono)
		}
		o.Move(fyne.NewPos((t.Width-m.Width)/2, (t.Height-m.Height)/2))
		o.Resize(m)
	}
}

// Refresh repinta fondo y glifo según el puntero (`hover:bg-sidebar-hover
// hover:text-sidebar-fg`), con su transición de 150 ms.
func (r *accionRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, tinta := color.NRGBA{}, tema.Color("sidebar-fg-muted", v)
	if !r.a.Inerte && (r.a.encima || r.a.pulsado) {
		fondo, tinta = tema.Color("sidebar-hover-bg", v), tema.Color("sidebar-fg", v)
	}
	r.aro.Ver(r.a.enfocado)
	r.aro.Pintar(v)
	r.fondo.hacia(enTinta(fondo, r.a.Inerte), r.pintarFondo)
	if r.a.Glifo != nil {
		r.tinta.hacia(enTinta(tinta, r.a.Inerte), r.a.Glifo.TenirCon) // `disabled:opacity-50`
	}
	r.a.girar()
	if o := r.a.icono(); o != nil {
		if r.a.apagada {
			o.Hide()
		} else {
			o.Show()
		}
	}
}

func (r *accionRenderer) pintarFondo(c color.NRGBA) { r.caja.Pintar(c, color.NRGBA{}) }

// Plegador es el rótulo de una sección cuando se puede plegar: un galón de 14 y el
// texto en versalitas, con el galón cayendo donde caería la primera letra.
type Plegador struct {
	pulsable

	s *SeccionLateral
}

var _ fyne.Widget = (*Plegador)(nil)

func nuevoPlegador(s *SeccionLateral) *Plegador {
	p := &Plegador{s: s}
	p.Nombre, p.AlPulsar = s.Titulo, s.AlPlegar
	p.ExtendBaseWidget(p)
	return p
}

// LadoGalon es el `size={14}` del chevron del rótulo plegable.
const LadoGalon float32 = 14

// galonDe es qué chevron toca. NO es una rotación: el `-rotate-90` que la web aplica al
// plegar es, punto por punto, el `chevronRight` del mismo juego de iconos.
func galonDe(abierta bool) iconos.Icono {
	if abierta {
		return iconos.ChevronAbajo
	}
	return iconos.ChevronDerecha
}

// MedidaPlegador es el `pl-1`, el galón, el `gap-1` y el rótulo.
func MedidaPlegador(titulo string) fyne.Size {
	ancho := tema.Espacio(1) + LadoGalon + tema.Espacio(1) + MedirVersal(titulo, tema.TextoXS, SemiNegrita)
	return fyne.NewSize(ancho, max(tema.TextoXS.Linea, LadoGalon))
}

// CreateRenderer construye galón y rótulo.
func (p *Plegador) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &plegadorRenderer{
		p:      p,
		aro:    NuevoAroCon("sidebar-focus", tema.RadioSM, SinHueco, v).Abierto(), // no tiene caja
		galon:  iconos.Trazo(galonDe(p.s.Abierta), LadoGalon, "sidebar-fg-muted", v),
		rotulo: NuevoVersal(p.s.Titulo, tema.TextoXS, SemiNegrita, tema.Color("sidebar-fg-muted", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.galon.Objeto())
	r.objetos = append(r.objetos, r.rotulo.Objetos()...)
	return r
}

type plegadorRenderer struct {
	p       *Plegador
	aro     *Aro
	galon   *iconos.Dibujo
	rotulo  *Versal
	objetos []fyne.CanvasObject
}

func (r *plegadorRenderer) MinSize() fyne.Size           { return MedidaPlegador(r.p.s.Titulo) }
func (r *plegadorRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *plegadorRenderer) Destroy()                     {}

// Layout pone el galón, el `gap-1` y el rótulo. El `pl-1` compensa el `-ml-1` que le
// puso la sección.
func (r *plegadorRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	x := tema.Espacio(1)
	r.galon.Colocar(x, (t.Height-LadoGalon)/2, LadoGalon)
	x += LadoGalon + tema.Espacio(1)
	r.rotulo.Colocar(x, (t.Height-tema.TextoXS.Linea)/2)
}

// Refresh gira el galón y repinta. El galón NO rota: el `-rotate-90` de un chevron
// hacia abajo es el chevron hacia la derecha del mismo juego.
func (r *plegadorRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tono := tema.Color("sidebar-fg-muted", v)
	if r.p.encima {
		tono = tema.Color("sidebar-fg", v) // hover:text-sidebar-fg
	}
	r.aro.Ver(r.p.enfocado)
	r.aro.Pintar(v)
	r.galon.Cambiar(galonDe(r.p.s.Abierta), tono)
	r.rotulo.Pintar(tono)
}

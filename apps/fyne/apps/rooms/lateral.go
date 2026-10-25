// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El carril de canales de Sytue Rooms. No se construye ni un widget: el carril, la
// sección, la fila, la acción del encabezado y el contador son de `apps/fyne/ui`. Aquí
// solo se dice QUÉ filas hay y en qué orden, que es lo único de Rooms.
//
// # Las dos medidas que separan este carril del de las otras cinco apps
//
// Medido en `rooms/canal.ancho.json` (1280×800, rem 14), y las dos salen del mismo
// sitio:
//
//   - La fila mide **35**, no 33,25. Rooms pone `grid h-6 w-6` en el hueco del icono
//     —21— donde el resto de la suite pone `h-5 w-5` —17,5—, así que el que manda
//     sobre la caja de línea del texto (19,25) es el icono: 21 + 2×7 = 35. Lo dice
//     `ui.FilaLateral.CajaIcono`, que existe por esto.
//   - El encabezado de sección ES el botón de plegar: 22,75 × 203 con
//     `pt-1 pr-1 pb-1.5 pl-2.5`, `text-xs` semi-negrita en versalitas. Los 203 son los
//     231 útiles menos el «＋» de 21 y su `gap-1`, y eso lo reparte `ui.SeccionLateral`.
//     Desde el PR #834 de la web el «＋» es la acción del carril, de 24 px: 200.
//
// El botón «Ir a un canal…» es de HOJA salvo su borde: la medida solo trae el filete
// (`1px solid` de `sidebar-fg-muted` al 40 %) y el glifo de 14. El resto —`h-8`,
// `mx-2.5 mb-2`, `px-2.5`, `gap-2`, `text-sm`— sale de las clases.

// Medidas del carril de Rooms, en px de CSS.
var (
	// CajaIconoFila es `grid h-6 w-6`: 21. MEDIDO (la fila sale a 35).
	CajaIconoFila float32 = 1.5 * tema.Rem
	// GlifoFila es el `size={16}` del icono dentro de esa caja. MEDIDO.
	GlifoFila = iconos.TallaSM
	// AltoIrACanal es el `h-8` del botón de «Ir a un canal…». HOJA.
	AltoIrACanal = tema.Espacio(8)
	// MargenIrACanal es su `mx-2.5`, y AireIrACanal su `mb-2`. HOJA.
	MargenIrACanal = tema.Espacio(2.5)
	AireIrACanal   = tema.Espacio(2)
	// GlifoIrACanal es la lupa de ese botón. MEDIDO: 14×14.
	GlifoIrACanal = iconos.TallaXS
	// CanalonIrACanal es su `px-2.5` y HuecoIrACanal su `gap-2`. HOJA.
	CanalonIrACanal = tema.Espacio(2.5)
	HuecoIrACanal   = tema.Espacio(2)
)

// Lateral son los datos que el carril necesita. Es una estructura y no una función con
// veinte argumentos porque la app la rellena entera en cada repintado.
type Lateral struct {
	T      *i18n.Translator
	Locale i18n.Locale

	Canales    []Canal
	Categorias []Categoria
	// EstadoCanales es el de la lista: la web pinta tres esqueletos, un aviso con
	// reintento o la lista, y no lo mismo en los tres casos.
	EstadoCanales Estado
	// CanalID marca la fila activa.
	CanalID string
	// Panel es el panel lateral abierto (hilos, reuniones, borradores, apps): la
	// entrada correspondiente va con `aria-pressed`, que aquí es `Activa`.
	Panel string
	// Borradores es cuántos hay sin enviar; con cero la entrada no se pinta, igual
	// que en la web.
	Borradores int
	// HilosSinLeer es la pastilla de «Hilos».
	HilosSinLeer int
	// PuedeGestionarApps enseña «Apps e integraciones». Solo para quien las
	// administra, como `canManageIntegrations`.
	PuedeGestionarApps  bool
	TieneCifrado        bool
	TieneConversaciones bool
	// Presencia y Estado son lo que enseña la cabecera del carril: tu presencia. Con
	// el socket caído la web la pinta «Sin conexión» aunque hayas elegido otra cosa,
	// porque el Hub ya no la recibe y «En línea» al lado de «Sin conexión» se
	// contradice.
	Presencia ui.Presencia
	// EstadoPersonal es el texto de estado que la persona escribió («En una
	// reunión»): cuando lo hay, se lee EN LUGAR del nombre del estado.
	EstadoPersonal string

	AlElegirCanal func(Canal)
	AlFavorito    func(Canal)
	// AlMenuCanal abre el «…» de la fila de un canal colgando de su disparador.
	AlMenuCanal     func(Canal, fyne.CanvasObject)
	AlPlegarSeccion func(Seccion)
	// AlMenuCategoria abre el «…» de una categoría personalizada colgando de su botón.
	AlMenuCategoria func(Categoria, fyne.CanvasObject)
	// Plegadas son las secciones POR DEFECTO plegadas (Favoritos, Canales, Directos): las
	// personalizadas lo guardan en su categoría, en el servidor.
	Plegadas         map[string]bool
	AlAbrirPanel     func(string)
	AlCambiarEstado  func(disparador fyne.CanvasObject)
	AlIrACanal       func()
	AlCrearCanal     func()
	AlExplorar       func()
	AlNuevoDirecto   func()
	AlNuevaCategoria func()
	AlReintentar     func()
	Refresco         ui.RefrescoLista
	// AlSoltarCanal mueve un canal arrastrado a la sección donde se suelta
	// (`dropChannelOnGroup`); nil, las filas no se arrastran.
	AlSoltarCanal func(Canal, Seccion)
	// AlSoltarCategoria pone una categoría propia en el puesto de otra
	// (`reorderCategoryBefore`); nil, las cabeceras no se arrastran.
	AlSoltarCategoria func(arrastrada, destino Categoria)

	// dianas son las secciones de canales pintadas, para saber dónde se suelta; punto,
	// dónde va el puntero del arrastre.
	dianas []diana
	punto  fyne.Position
}

// diana es una sección del carril donde se puede soltar un canal: la pieza (`sec`, que
// pinta el aro) y el objeto puesto en el carril (`o`, que es la misma o la que la
// envuelve para arrastrarla).
type diana struct {
	s   Seccion
	sec *ui.SeccionLateral
	o   fyne.CanvasObject
}

// arrastrar sigue el puntero de un arrastre y enciende el aro (`dropActive && over`) de
// la sección de debajo si `vale` la acepta.
func (l *Lateral) arrastrar(p fyne.Position, vale func(Seccion) bool) {
	l.punto = p
	debajo, hay := l.SeccionEn(p)
	for _, x := range l.dianas {
		if on := hay && x.s.Clave == debajo.Clave && vale(x.s); x.sec.Diana != on {
			x.sec.Diana = on
			x.o.Refresh()
		}
	}
}

// soltar apaga los aros y devuelve la sección donde ha acabado el puntero.
func (l *Lateral) soltar() (Seccion, bool) {
	for _, x := range l.dianas {
		if x.sec.Diana {
			x.sec.Diana = false
			x.o.Refresh()
		}
	}
	return l.SeccionEn(l.punto)
}

// SeccionEn es la sección bajo ese punto absoluto de la ventana, si la hay.
func (l *Lateral) SeccionEn(p fyne.Position) (Seccion, bool) {
	d := fyne.CurrentApp().Driver()
	for _, x := range l.dianas {
		abs, tam := d.AbsolutePositionForObject(x.o), x.o.Size()
		if p.X >= abs.X && p.X < abs.X+tam.Width && p.Y >= abs.Y && p.Y < abs.Y+tam.Height {
			return x.s, true
		}
	}
	return Seccion{}, false
}

// Las claves de los paneles laterales, que son las que `Panel` compara.
const (
	PanelHilos       = "threads"
	PanelReuniones   = "meetings"
	PanelBorradores  = "drafts"
	PanelIntegracion = "integrations"
	PanelCifrado     = "matrix"
	PanelExternas    = "external-conversations"
)

// Construir devuelve el `ui.Lateral` ya montado.
func (l *Lateral) Construir() *ui.Lateral {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())

	secciones := l.seccionesDeCanales(v)
	cuerpo := make([]fyne.CanvasObject, 0, 2+len(secciones))
	cuerpo = append(cuerpo, l.irACanal(), l.seccionPaneles(v))
	cuerpo = append(cuerpo, secciones...)

	// `ui.Columna` sin hueco: cada sección trae su `mb-4` dentro. El cuerpo desplaza y
	// no empuja al pie, que es el `overflow-y-auto` de la web; lo decide quien monta
	// el carril, como dice `ui.Lateral`.
	c := container.NewVScroll(container.New(ui.Columna{}, cuerpo...))
	refresco := l.Refresco
	refresco.Arriba = func() bool { return c.Offset.Y <= 0.5 }
	lat := ui.NuevoLateral(l.T.T("ui.mainNav", nil), ui.RefrescarAlTirar(c, &refresco))
	lat.Cabecera = l.cabecera()
	return lat
}

// cabecera es tu presencia. NO lleva «Sytue Rooms»: el selector de apps de la barra
// superior ya lo dice y el carril lo repetía justo debajo (informado en la web el
// 2026-09-09). En nativo la marca está en el título de la ventana, que es donde el
// sistema la enseña.
//
// El disparador va con el `px-2.5` de su `div` delante: se monta en una fila con un
// hueco transparente de 8,75, que es lo que cabe en `ui.Lateral.Cabecera` sin tocar
// la pieza de las seis apps.
func (l *Lateral) cabecera() fyne.CanvasObject {
	t := l.T
	p := l.Presencia
	if p == ui.SinPresencia {
		p = ui.EnLinea // el `useState('online')` de la web: el punto no se queda sin color
	}
	estado := t.T(clavePresencia(p), nil)
	texto, nombre := estado, t.T("status.yoursIs", i18n.Vars{"state": estado})
	if l.EstadoPersonal != "" {
		texto = l.EstadoPersonal
		nombre = t.T("status.yoursWithCustom", i18n.Vars{"state": estado, "status": l.EstadoPersonal})
	}
	hueco := canvas.NewRectangle(color.NRGBA{})
	hueco.SetMinSize(fyne.NewSize(SangriaEstado, 0))
	var disparador *EstadoPropio
	disparador = NuevoEstadoPropio(p, texto, nombre, func() {
		if l.AlCambiarEstado != nil {
			l.AlCambiarEstado(disparador)
		}
	})
	return container.New(ui.Fila{}, hueco, disparador)
}

// clavePresencia es la etiqueta de tu estado, con las claves de `presence.*`.
func clavePresencia(p ui.Presencia) string {
	switch p {
	case ui.Ausente:
		return "presence.away"
	case ui.NoMolestar:
		return "presence.dnd"
	case ui.Desconectado:
		return "presence.offline"
	case ui.SinPresencia, ui.EnLinea:
	}
	return "presence.online"
}

// irACanal es la caja de arriba con su `mx-2.5` y su `mb-2`, que en la web son
// márgenes del botón y aquí dos huecos transparentes: la columna del carril estira a
// sus hijos, y el botón tiene que quedarse en sus 213,5.
func (l *Lateral) irACanal() fyne.CanvasObject {
	rotulo := l.T.T("switcher.open", nil)
	b := NuevoBotonIrACanal(rotulo, rotulo+" ("+ui.AtajoDelSistema("Ctrl+G")+")", l.AlIrACanal)
	izq := canvas.NewRectangle(color.NRGBA{})
	izq.SetMinSize(fyne.NewSize(MargenIrACanal, 0))
	abajo := canvas.NewRectangle(color.NRGBA{})
	abajo.SetMinSize(fyne.NewSize(0, AireIrACanal))
	return container.New(ui.Columna{}, container.New(ui.Fila{}, izq, b), abajo)
}

// seccionPaneles es el bloque de arriba: Hilos, Reuniones, Borradores y Apps. Son
// destinos que abren un PANEL, no un canal, y por eso van en su propia sección sin
// rótulo, como en la web.
func (l *Lateral) seccionPaneles(v tema.Variante) *ui.SeccionLateral {
	t := l.T
	s := ui.NuevaSeccionLateral("")
	s.Filas = append(s.Filas,
		l.filaPanel(PanelHilos, t.T("sidebar.threads", nil), glifoHilos, Contador(l.HilosSinLeer), l.HilosSinLeer > 0, v),
		l.filaPanel(PanelReuniones, t.T("meetings.open", nil), glifoVideo, "", false, v),
	)
	if l.Borradores > 0 {
		s.Filas = append(s.Filas, l.filaPanel(
			PanelBorradores, t.T("drafts.title", nil), glifoLapiz, Contador(l.Borradores), true, v))
	}
	if l.TieneCifrado {
		f := ui.NuevaFilaLateral(t.T("matrix.title", nil), iconos.Trazo(iconos.Candado, GlifoFila, l.tokenPanel(PanelCifrado), v).Objeto(), func() { l.AlAbrirPanel(PanelCifrado) })
		f.CajaIcono = CajaIconoFila
		s.Filas = append(s.Filas, f)
	}
	if l.TieneConversaciones {
		f := ui.NuevaFilaLateral(t.T("external.title", nil), iconos.Trazo(iconos.Rejilla, GlifoFila, l.tokenPanel(PanelExternas), v).Objeto(), func() { l.AlAbrirPanel(PanelExternas) })
		f.CajaIcono = CajaIconoFila
		s.Filas = append(s.Filas, f)
	}
	if l.PuedeGestionarApps {
		f := ui.NuevaFilaLateral(
			t.T("integrations.apps", nil),
			iconos.Trazo(iconos.Rejilla, GlifoFila, l.tokenPanel(PanelIntegracion), v).Objeto(),
			func() { l.AlAbrirPanel(PanelIntegracion) },
		)
		f.CajaIcono = CajaIconoFila
		f.Activa = l.Panel == PanelIntegracion
		s.Filas = append(s.Filas, f)
	}
	return s
}

func (l *Lateral) filaPanel(clave, rotulo, cuerpo, contador string, conContador bool,
	v tema.Variante,
) *ui.FilaLateral {
	f := ui.NuevaFilaLateral(rotulo,
		dibujo(cuerpo, GlifoFila, l.tokenPanel(clave), v).Objeto(),
		func() { l.AlAbrirPanel(clave) })
	f.CajaIcono = CajaIconoFila
	f.Activa = l.Panel == clave
	if conContador {
		f.Contador = contador
	}
	return f
}

func (l *Lateral) tokenPanel(clave string) string {
	if l.Panel == clave {
		return "sidebar-active-fg"
	}
	return "sidebar-fg"
}

// seccionesDeCanales monta las secciones en sus estados. El esqueleto, el error con
// reintento y la lista son tres pinturas distintas, no una lista vacía tres veces.
func (l *Lateral) seccionesDeCanales(v tema.Variante) []fyne.CanvasObject {
	t := l.T
	switch l.EstadoCanales {
	case Cargando:
		// Tres esqueletos con la geometría REAL de la fila, como la web: sin esto el
		// layout salta al llegar los datos.
		s := ui.NuevaSeccionLateral(t.T("sidebar.channels", nil))
		s.Filas = []fyne.CanvasObject{NuevoEsqueletoCanales(t.T("sidebar.loadingChannels", nil))}
		return []fyne.CanvasObject{s}
	case Error:
		s := ui.NuevaSeccionLateral(t.T("sidebar.channels", nil))
		s.Filas = []fyne.CanvasObject{
			ui.NuevoRotulo(t.T("sidebar.loadFailed", nil), tema.TextoXS, ui.Normal, "sidebar-fg-muted"),
			ui.NuevoBoton(t.T("action.retry", nil), l.AlReintentar).Con(ui.Secundario, ui.TallaXS),
		}
		return []fyne.CanvasObject{s}
	case Listo, Vacio, SinConexion, SinPermiso:
	}

	var out []fyne.CanvasObject
	l.dianas = l.dianas[:0]
	for _, s := range AgruparCanales(l.Canales, l.Categorias) {
		sec, o := l.seccion(s, v)
		l.dianas = append(l.dianas, diana{s, sec, o})
		out = append(out, o)
	}
	// «Nueva categoría», fuera de toda sección y con su `mb-4`, como en la web.
	nueva := ui.NuevaFilaLateral(
		t.T("sidebar.newCategory", nil),
		iconos.Trazo(iconos.Mas, GlifoFila, "sidebar-fg-muted", v).Objeto(),
		l.AlNuevaCategoria,
	)
	nueva.CajaIcono = CajaIconoFila
	return append(out, nueva)
}

// seccion es `CategorySection`: la pieza que se configura y el objeto que va en el carril
// (la misma, o la que la envuelve para que una categoría propia se arrastre por su
// cabecera y se suelte sobre otra).
//
//nolint:gocognit // Cada clase de sección lleva sus acciones, su pie y su arrastre.
func (l *Lateral) seccion(s Seccion, v tema.Variante) (*ui.SeccionLateral, fyne.CanvasObject) {
	t := l.T
	titulo := s.Titulo
	if s.ClaveTitulo != "" {
		titulo = t.T(s.ClaveTitulo, nil)
	}
	var sec *ui.SeccionLateral
	var obj fyne.CanvasObject
	if s.Categoria != nil && l.AlSoltarCategoria != nil && !ui.EsTactil() {
		k := *s.Categoria
		otra := func(d Seccion) bool { return d.Categoria != nil && d.Categoria.ID != k.ID }
		a := ui.NuevaSeccionLateralArrastrable(titulo)
		a.AlArrastrar = func(p fyne.Position) { l.arrastrar(p, otra) }
		a.AlSoltar = func() {
			if d, ok := l.soltar(); ok && otra(d) {
				l.AlSoltarCategoria(k, *d.Categoria)
			}
		}
		sec, obj = &a.SeccionLateral, a
	} else {
		sec = ui.NuevaSeccionLateral(titulo)
		obj = sec
	}
	sec.AlPlegar = func() { l.AlPlegarSeccion(s) }
	sec.Abierta = !l.Plegadas[s.Clave]
	if s.Categoria != nil {
		sec.Abierta = !s.Categoria.Plegada
		if l.AlMenuCategoria != nil {
			// Al pasar por encima (`group-hover/cat`), y siempre con el dedo.
			sec.AccionesAlPasar = true
			k := *s.Categoria
			var mas *ui.AccionLateral
			mas = ui.NuevaAccionLateralCon(t.T("category.actions", i18n.Vars{"name": titulo}),
				dibujo(glifoMas, GlifoFila, "sidebar-fg-muted", v), func() { l.AlMenuCategoria(k, mas) })
			sec.Acciones = []fyne.CanvasObject{mas}
		}
	}
	if s.Clave == ClaveCanales {
		// El «＋» de «Canales»: la puerta principal para crear uno. Siempre visible y
		// con su nombre, como el resto del carril.
		sec.Acciones = []fyne.CanvasObject{ui.NuevaAccionLateralCon(
			t.T("sidebar.createChannel", nil),
			iconos.Trazo(iconos.Mas, GlifoFila, "sidebar-fg-muted", v),
			l.AlCrearCanal,
		)}
	}
	if !sec.Abierta {
		// Una sección plegada no MONTA sus filas: `ui.SeccionLateral` las esconde, pero
		// no dárselas es más barato y evita construir cincuenta filas que nadie ve.
		return sec, obj
	}
	for _, c := range s.Canales {
		sec.Filas = append(sec.Filas, l.fila(c, v))
	}
	switch {
	case len(s.Canales) == 0 && s.Clave == ClaveCanales:
		sec.Filas = append(sec.Filas,
			ui.NuevoRotulo(t.T("sidebar.noChannelsYet", nil), tema.TextoXS, ui.Normal, "sidebar-fg-muted"))
	case len(s.Canales) == 0 && s.Categoria != nil:
		sec.Filas = append(sec.Filas,
			ui.NuevoRotulo(t.T("sidebar.dropHint", nil), tema.TextoXS, ui.Normal, "sidebar-fg-muted"))
	}
	switch s.Clave {
	case ClaveCanales:
		sec.Filas = append(sec.Filas, l.accion("sidebar.browseChannels", iconos.Buscar, l.AlExplorar, v))
	case ClaveDirectos:
		sec.Filas = append(sec.Filas, l.accion("sidebar.newMessage", iconos.Mas, l.AlNuevoDirecto, v))
	}
	return sec, obj
}

func (l *Lateral) accion(clave string, i iconos.Icono, al func(), v tema.Variante) *ui.FilaLateral {
	f := ui.NuevaFilaLateral(l.T.T(clave, nil),
		iconos.Trazo(i, GlifoFila, "sidebar-fg-muted", v).Objeto(), al)
	f.CajaIcono = CajaIconoFila
	return f
}

// fila es un canal: `ChannelRow`, con su negrita, su estrella y su pastilla fuera del
// botón. Ver `FilaCanal`.
//
// Con ratón, la de un canal de la casa se ARRASTRA a otra sección (`draggable={!c.external}`):
// una conversación de otra organización no se coloca en las categorías propias.
//
// Mientras se arrastra, la sección de debajo lleva su aro (`ring-2 ring-indicator`): un
// canal se suelta en cualquiera. El camino de teclado es el menú de la fila.
func (l *Lateral) fila(c Canal, v tema.Variante) fyne.CanvasObject {
	elegir := func() { l.AlElegirCanal(c) }
	favorito := func() {
		if l.AlFavorito != nil {
			l.AlFavorito(c)
		}
	}
	var f *FilaCanal
	var out fyne.CanvasObject
	if l.AlSoltarCanal != nil && !c.Externo && !ui.EsTactil() {
		a := NuevaFilaCanalArrastrable(l.T, c, l.marca(c, v), c.ID == l.CanalID, elegir, favorito)
		a.AlArrastrar = func(p fyne.Position) { l.arrastrar(p, func(Seccion) bool { return true }) }
		a.AlSoltar = func() {
			if s, ok := l.soltar(); ok {
				l.AlSoltarCanal(c, s)
			}
		}
		f, out = &a.FilaCanal, a
	} else {
		f = NuevaFilaCanal(l.T, c, l.marca(c, v), c.ID == l.CanalID, elegir, favorito)
		out = f
	}
	if l.AlMenuCanal != nil {
		f.AlMenu = func(d fyne.CanvasObject) { l.AlMenuCanal(c, d) }
	}
	return out
}

// marca es lo que va en la caja del icono (`channelLeading`): la «@» si hay menciones,
// el avatar en un directo, el candado en un canal privado y el «#» en uno público. El
// «#» y el candado van SIEMPRE en `sidebar-fg-muted`, esté la fila activa o no: lo que
// se enciende es el nombre, no la marca.
func (l *Lateral) marca(c Canal, v tema.Variante) fyne.CanvasObject {
	if c.Menciones > 0 {
		return NuevaMencion(l.T.T("channel.mentions", i18n.Vars{"count": c.Menciones}))
	}
	switch c.Clase {
	case Directo:
		a := ui.NuevoAvatar(c.Nombre).Con(ui.AvatarXS)
		a.Presencia = c.Presencia
		return a
	case Privado:
		return iconos.Trazo(iconos.Candado, GlifoIrACanal, "sidebar-fg-muted", v).Objeto()
	case Publico:
	}
	return NuevaAlmohadilla("sidebar-fg-muted")
}

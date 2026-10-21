// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"context"
	"image/color"
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// Los avisos de la barra: `NotificationCenter.tsx`.
//
// El panel sí va a una capa —es un menú, no un combobox: el foco se va a él a
// propósito—, así que se reutiliza `ui.Menu` entero y con él el velo que recoge el
// clic de fuera, el Escape y la sombra.
const (
	// AnchoAvisos es el `w-80 max-w-full` del CONTENIDO del panel: 280 con el rem de
	// 14; el panel, con su `p-1` y su borde, mide 290. Hasta el 2026-09-24 la web lo
	// tenía como `w-full max-w-80` y medía lo que su contenido, y aquí era el panel
	// entero el que medía 280.
	AnchoAvisos = 20 * tema.Rem
	// MaxAvisos es el `limit: 30` de la carga inicial.
	MaxAvisos = 30
	// EsperaAvisosMax es el techo de la espera creciente entre reconexiones del flujo,
	// igual que en la web: un Hub caído no debe recibir una reconexión por segundo de
	// cada aplicación abierta.
	EsperaAvisosMax = 30 * time.Second
	// MargenPanelX y MargenPanelY son el `px-3 py-2` de las bandas del panel.
	MargenPanelX = 3 * 0.25 * tema.Rem
	MargenPanelY = 2 * 0.25 * tema.Rem
)

// avisosBarra es la campana, su punto y el panel de avisos.
type avisosBarra struct {
	t   *i18n.Translator
	hub *api.Hub
	s   *sesion.Suite

	boton  *BotonIcono
	anillo *canvas.Rectangle
	punto  *canvas.Rectangle
	menu   *Menu
	// carril: la barra entera es de color (ver enCarril).
	carril bool

	mu    sync.Mutex
	items []api.HubNotification
	// anunciados son los avisos que YA han salido al sistema operativo. Existe porque
	// llegan por dos caminos —el flujo y la carga que lo repesca— y ninguno debe sonar
	// dos veces por lo mismo.
	//
	// ponytail: crece con la sesión, ~40 bytes por aviso; hacen falta decenas de miles
	// para notarlo. Si alguna vez hay una sesión así de larga, se poda por fecha.
	anunciados map[string]bool
	sinLeer    int
	// correo son los correos sin leer de Sytue Mail que cuenta el Hub; nil si no se sabe.
	correo *int
	// insignia es el último número puesto sobre el icono, para no repetirlo.
	enIcono int
	estado  estadoBarra
	parada  context.CancelFunc
	// irA lleva al destino de un aviso: su app nativa si está instalada y, si no, la
	// web (`api.AbrirURL`). Los guardianes la sustituyen para ver a dónde va.
	irA func(producto, url string)
	// alFoto es lo que hace la barra cuando Sytue ID avisa, por el mismo flujo, de que
	// la persona cambió o quitó su foto: volver a pedir la ficha (lo pone BarraSuite).
	alFoto func()
}

func nuevoAvisosBarra(t *i18n.Translator, s *sesion.Suite) *avisosBarra {
	a := &avisosBarra{t: t, s: s, estado: enReposo, anunciados: map[string]bool{}}
	a.irA = func(producto, u string) { _ = api.AbrirURL(fyne.CurrentApp(), producto, u) }
	a.boton = NuevoBotonIcono(iconos.Campana, t.T("notifications.title", nil), a.abrir).SobreBanda()
	a.boton.Trazo = "topbar-fg" // `text-topbar-fg` de TopBarButton, no el blanco entero de la banda
	a.boton.Lado = LadoControlBarra()
	a.boton.TamIcono = TamGlifoBarra
	a.anillo = canvas.NewRectangle(color.NRGBA{})
	a.punto = canvas.NewRectangle(color.NRGBA{})
	a.anillo.Hide()
	a.punto.Hide()
	a.menu = NuevoMenu()
	a.menu.Rotulo = t.T("notifications.title", nil)
	// Ancho FIJO, como `w-80` en la web: un título largo se recorta, no ensancha el panel.
	a.menu.Ancho = AnchoAvisos + 2*(rellenoPanelMenu+tema.Borde)
	return a
}

// apex es el dominio de la suite de la sesión, o el de producción sin ella.
func (a *avisosBarra) apex() string {
	if a.s == nil {
		return api.ApexProd
	}
	return a.s.Apex()
}

// org es la organización activa de la sesión, o "" sin ella: el enlace sale sin org y
// el middleware de la app de destino la añade con la de quien acabe entrando (ADR 0056).
func (a *avisosBarra) org() string {
	if a.s == nil || a.s.Sesion == nil {
		return ""
	}
	return a.s.Sesion.Organizacion()
}

func (a *avisosBarra) objetos() []fyne.CanvasObject {
	return []fyne.CanvasObject{a.boton, a.anillo, a.punto}
}

// colocar pone la campana y su punto: `top-2 right-2`, 7 px con un anillo de 2 del
// color de la barra, que es lo que lo despega del glifo.
func (a *avisosBarra) colocar(p fyne.Position, lado float32) {
	a.boton.Move(p)
	a.boton.Resize(fyne.NewSize(lado, lado))
	x := p.X + lado - MargenPuntoAviso - LadoPuntoAviso
	y := p.Y + MargenPuntoAviso
	a.punto.Move(fyne.NewPos(x, y))
	a.punto.Resize(fyne.NewSize(LadoPuntoAviso, LadoPuntoAviso))
	a.punto.CornerRadius = LadoPuntoAviso / 2
	aro := AnilloPuntoAviso
	if a.carril {
		aro = AroCarril
	}
	a.anillo.Move(fyne.NewPos(x-aro, y-aro))
	a.anillo.Resize(fyne.NewSize(LadoPuntoAviso+2*aro, LadoPuntoAviso+2*aro))
	a.anillo.CornerRadius = (LadoPuntoAviso + 2*aro) / 2
}

// enCarril pone la campana sobre la barra de color —la barra estrecha, que es carril
// entera— o la devuelve a la clara: la tinta, los velos y el foco de cada sitio.
func (a *avisosBarra) enCarril(si bool) {
	b := a.boton
	b.Trazo, b.FondoEncima, b.FondoActivo, b.FondoPulsado, b.TokenAro = "topbar-fg", "chrome-hover", "chrome-active", "chrome-active", "chrome-focus"
	if si {
		b.Trazo, b.FondoEncima, b.FondoActivo, b.FondoPulsado, b.TokenAro = "sidebar-fg", "sidebar-hover-bg", "sidebar-active-bg", "sidebar-active-bg", "sidebar-focus"
	}
	// `rounded-control` de TopBarButton: el radio del control es de la forma.
	b.Radio = tema.Forma("radius-control")
	a.carril = si
	b.Refresh()
}

func (a *avisosBarra) repintar(v tema.Variante) {
	a.mu.Lock()
	sinLeer := a.sinLeer
	a.mu.Unlock()
	a.punto.FillColor, a.anillo.FillColor = colorPuntoAviso(v, a.carril)
	ver(a.punto, sinLeer > 0)
	ver(a.anillo, sinLeer > 0)
	rotulo := a.t.T("notifications.title", nil)
	if sinLeer > 0 {
		rotulo = a.t.T("notifications.buttonLabel", i18n.Vars{"count": sinLeer})
	}
	a.boton.Rotulo = rotulo
}

// colorPuntoAviso son el punto de la campana y su anillo.
//
// Sobre la barra CLARA (Carril, en ancho) el anillo es del color de la barra —el hueco
// que despega el punto del glifo— y el punto, el rojo de texto del tema: oscuro sobre el
// cromo claro, claro sobre el oscuro.
//
// Sobre la barra DE COLOR (Marco, y Carril en estrecho) el punto lleva un ARO del texto
// del carril, como una muestra de color (`.sytue-aro-carril`): la barra puede ser
// petróleo, cobalto o el color de cualquier producto —el de Mail es un rojo—, y ningún
// rojo se despega de todos. El aro sí: el rojo de peligro contra él y él contra la barra
// pasan de 3:1 (WCAG 1.4.11) en los tres colores y los tres modos (decidido el
// 2026-10-05; lo mide TestElPuntoDeAvisoLlevaAroSobreLaBarraDeColor).
func colorPuntoAviso(v tema.Variante, carril bool) (punto, anillo color.NRGBA) {
	if carril {
		return tema.Color("danger", v), tema.Color("sidebar-fg", v)
	}
	return tema.Color("danger-text", v), tema.Color(fondoBarra, v)
}

// arrancar se apunta a la sesión y pide los avisos en cuanto haya una.
//
// Se apunta y no mira una vez: la sesión puede llegar DESPUÉS de abrir la ventana
// —desde este menú o desde otra de las doce aplicaciones—, y una campana que solo
// carga al arrancar se queda vacía para siempre en cuanto alguien entra más tarde.
func (a *avisosBarra) arrancar() {
	if a.s == nil {
		return
	}
	a.s.AlCambiar(a.sesionCambio) // siempre en el hilo de la interfaz
	a.sesionCambio(a.s.Estado())
}

// sesionCambio enciende o apaga la campana según haya sesión.
func (a *avisosBarra) sesionCambio(e sesion.Estado) {
	a.mu.Lock()
	yaEstaba := a.hub != nil
	a.mu.Unlock()
	if e.Dentro() == yaEstaba {
		return
	}
	a.parar()
	if !e.Dentro() {
		a.mu.Lock()
		a.hub, a.items, a.sinLeer, a.estado = nil, nil, 0, enReposo
		a.mu.Unlock()
		a.refrescar()
		return
	}
	ctx, cancelar := context.WithCancel(context.Background())
	a.mu.Lock()
	a.hub, a.estado, a.parada = a.s.Hub, cargando, cancelar
	a.mu.Unlock()
	go a.cargar(ctx, false)
	go a.escuchar(ctx)
}

func (a *avisosBarra) parar() {
	a.mu.Lock()
	cancelar := a.parada
	a.parada = nil
	a.mu.Unlock()
	if cancelar != nil {
		cancelar()
	}
}

// cargar pide la lista y el contador. `repesca` es la carga tras un corte del flujo:
// lo que aparece nuevo y sin leer es lo que llegó con el flujo caído, y se anuncia al
// sistema como si hubiera llegado por él (con la misma verja de una vez por aviso). Y
// si esa carga falla, se queda lo que había: un corte no vacía el panel.
func (a *avisosBarra) cargar(ctx context.Context, repesca bool) {
	hub := a.hubActual()
	if hub == nil {
		return
	}
	items, err := hub.Notifications(ctx, MaxAvisos)
	cuenta, errCuenta := hub.Contadores(ctx)
	if ctx.Err() != nil {
		return // la sesión cambió mientras tanto: esto ya no es de nadie
	}
	var nuevos []api.HubNotification
	a.mu.Lock()
	switch {
	case err != nil && !repesca:
		a.items, a.estado = nil, fallo
	case err == nil:
		for _, n := range items {
			if repesca && !n.Read && !slices.ContainsFunc(a.items, func(v api.HubNotification) bool { return v.ID == n.ID }) {
				nuevos = append(nuevos, n)
			}
		}
		a.items, a.estado = items, listo
	}
	// Si el contador falla, el panel sigue sirviendo: se queda sin punto, no sin
	// avisos. Es lo que hace la web.
	if errCuenta == nil {
		a.sinLeer, a.correo = cuenta.Unread, cuenta.MailUnread
	}
	a.mu.Unlock()
	a.refrescar()
	for _, n := range nuevos {
		a.anunciar(n)
	}
}

// escuchar mantiene el flujo SSE con espera creciente. `StreamNotifications` no vuelve
// hasta que el flujo se corta, y quien decide cuánto esperar es quien llama.
//
// Tras cada corte se vuelve a CARGAR la lista, como en la web: lo que llegó con el
// flujo caído no lo trae nadie más. Antes solo se volvía a escuchar, y con el Hub
// cortando el flujo a los 30 s (hasta el 2026-09-24) un aviso de cada dos se perdía.
func (a *avisosBarra) escuchar(ctx context.Context) {
	hub := a.hubActual()
	if hub == nil {
		return
	}
	seguidos := 0
	for primera := true; ctx.Err() == nil; primera = false {
		if !primera {
			a.cargar(ctx, true)
		}
		err := hub.Escuchar(ctx, a.llega, a.alEstado)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			seguidos = 0
		}
		espera := min(EsperaAvisosMax, time.Second*(1<<min(seguidos, 5)))
		seguidos++
		select {
		case <-ctx.Done():
			return
		case <-time.After(espera):
		}
	}
}

// alEstado atiende un evento de ESTADO del flujo, que no es un aviso ni entra en la
// campana. Hoy solo hay uno: «cambió la foto».
func (a *avisosBarra) alEstado(evento, _ string) {
	if evento == api.EventoAvatar && a.alFoto != nil {
		a.alFoto()
	}
}

// llega mete un aviso nuevo, sin repetirlo: el flujo puede reenviar lo ya cargado.
func (a *avisosBarra) llega(n api.HubNotification) {
	a.mu.Lock()
	for _, v := range a.items {
		if v.ID == n.ID {
			a.mu.Unlock()
			return
		}
	}
	a.items = append([]api.HubNotification{n}, a.items...)
	if len(a.items) > MaxAvisos {
		a.items = a.items[:MaxAvisos]
	}
	if !n.Read {
		a.sinLeer++
	}
	// ponytail: el correo nuevo suma; el que se lee en otra parte no resta hasta la
	// próxima carga, como en la web.
	if n.Type == "mail.message.received" && a.correo != nil {
		c := *a.correo + 1
		a.correo = &c
	}
	a.estado = listo
	a.mu.Unlock()
	avisarOyentes(n)
	a.refrescar()
	// Y, si es de esta aplicación y no se ha leído, se dice al SISTEMA: la ventana
	// puede estar detrás de otras o minimizada, que es cuando un aviso sirve de algo.
	a.anunciar(n)
}

// refrescar repinta la campana y, si el panel está abierto, su contenido.
func (a *avisosBarra) refrescar() {
	a.ponerInsignia()
	fyne.Do(func() {
		a.boton.Refresh()
		a.repintar(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
		a.anillo.Refresh()
		a.punto.Refresh()
		if a.menu.Abierto() {
			a.menu.Reemplazar(a.contenido()...)
		}
	})
}

func (a *avisosBarra) abrir() {
	if a.menu.Abierto() {
		a.menu.Cerrar()
		return
	}
	a.menu.Elementos = a.contenido()
	a.menu.Mostrar(a.boton)
}

// contenido es el panel entero: el título con su contador, el cuerpo según el estado y
// la salida a la bandeja completa.
func (a *avisosBarra) contenido() []fyne.CanvasObject {
	a.mu.Lock()
	estado, items, sinLeer, correo := a.estado, a.items, a.sinLeer, a.correo
	a.mu.Unlock()

	var out []fyne.CanvasObject
	// Los correos sin leer, POR ENCIMA de los avisos, como en la web: son estado y
	// el que ya esperaba no dejó aviso. Llevan a Sytue Mail.
	if correo != nil && *correo > 0 {
		f := NuevaFilaLista(a.t.T("notifications.mailUnread", i18n.Vars{"count": *correo})+" →", "")
		f.Fondo = "surface-raised"
		f.AlPulsar = func() {
			a.menu.Cerrar()
			_ = api.AbrirEn(fyne.CurrentApp(), a.apex(), "mail", a.org(), "/")
		}
		out = append(out, f)
	}
	out = append(out, a.cabecera(sinLeer))
	switch {
	case estado == enReposo: // sin sesión: no hay nada que pedir y se dice
		out = append(out, NuevoVacio(a.t.T("notifications.emptyTitle", nil), a.t.T("notifications.emptyDescription", nil)))
	case estado == cargando:
		out = append(out, esqueletosDeLista(3)...)
	case estado == fallo:
		vacio := NuevoVacio(a.t.T("notifications.failed", nil), a.t.T("error.connection", nil))
		vacio.Accion = NuevoBoton(a.t.T("action.retry", nil), func() {
			a.menu.Cerrar()
			a.mu.Lock()
			a.hub = nil // que `sesionCambio` lo vea como un encendido y vuelva a pedir
			a.mu.Unlock()
			if a.s != nil {
				a.sesionCambio(a.s.Estado())
			}
		}).Con(Secundario, TallaSM)
		out = append(out, vacio)
	case len(items) == 0:
		out = append(out, NuevoVacio(a.t.T("notifications.emptyTitle", nil), a.t.T("notifications.emptyDescription", nil)))
	default:
		for _, n := range items {
			out = append(out, a.fila(n))
		}
	}
	out = append(out, NuevoSeparadorMenu(), a.verTodas())
	return out
}

// cabecera es la banda del título con la insignia de sin leer y, si hay alguno, el
// «marcar todas como leídas». El ancho del panel lo fija `Menu.Ancho`.
func (a *avisosBarra) cabecera(sinLeer int) fyne.CanvasObject {
	fila := []fyne.CanvasObject{NuevoRotulo(a.t.T("notifications.title", nil), tema.TextoBase, SemiNegrita, "text")}
	var todas fyne.CanvasObject
	if sinLeer > 0 {
		fila = append(fila, NuevaInsignia(a.t.T("notifications.unread", i18n.Vars{"count": sinLeer}), InsigniaPrimaria))
		// «Marcar todas como leídas», con glifo como en la web: el texto no cabe junto
		// al título y la insignia en el ancho del panel.
		todas = NuevoBotonIcono(iconos.Tic, a.t.T("notifications.markAllRead", nil), func() { go a.leerTodos() })
	}
	// `px-3 pt-1.5 pb-2`: arriba 1,5 y abajo 2, no 2 y 2 (medido: 38,5 contra 36,75).
	return container.New(
		layout.NewCustomPaddedLayout(1.5*0.25*tema.Rem, MargenPanelY, MargenPanelX, MargenPanelX),
		container.NewBorder(nil, nil, nil, todas, container.New(Fila{Hueco: HuecoAccionesBarra}, fila...)))
}

// fila es un aviso: título, cuerpo y cuándo llegó, con el galón de sin leer.
func (a *avisosBarra) fila(n api.HubNotification) fyne.CanvasObject {
	f := NuevaFilaLista(TituloAviso(a.t, n), CuerpoAviso(a.t.Locale(), n))
	f.Fondo = "surface-raised"
	f.SinLeer = !n.Read
	// De OTRA organización (ADR 0053) o anterior al registro de avisos, lo dice.
	f.Meta = ProcedenciaAviso(a.t, n) + i18n.FormatTimeAgo(a.t.Locale(), n.CreatedAt, time.Now())
	// Título y cuerpo van a ListItem como nodos (`<span title>`), no como cadenas, así
	// que la web los PARTE en líneas en vez de truncarlos. Y sin `leading`: la web no
	// pone el icono del producto en la campana.
	f.Envuelve = AnchoAvisos
	f.AlPulsar = func() {
		a.menu.Cerrar()
		a.pulsar(n)
	}
	return f
}

// pulsar es lo que hace un aviso al pulsarlo, en la campana o en el sistema: se da por
// leído y se abre su destino.
func (a *avisosBarra) pulsar(n api.HubNotification) {
	go a.leido(n.ID)
	if n.URL != "" {
		// Lo que se abre desde aquí se abre EN EL SISTEMA, igual que un resultado de
		// búsqueda: el aviso puede ser de otra aplicación de la suite.
		a.irA(n.Source, n.URL)
	}
}

// leido marca el aviso y baja el contador. Falla en silencio: perder un «visto» no
// puede quitarle al usuario el aviso que acaba de pulsar.
//
// El contexto es SUYO y no el del flujo que lo trajo, a propósito: esto lo dispara un
// clic mucho después, y marcar un aviso como leído no puede cancelarse porque el SSE
// se haya reconectado por medio. Por eso `contextcheck` va callado aquí.
//
//nolint:contextcheck // el contexto del flujo no vale: esto lo lanza un clic posterior
func (a *avisosBarra) leido(id string) {
	hub := a.hubActual()
	if hub == nil {
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	if hub.MarkRead(ctx, id) != nil {
		return // sin acuse no se baja el contador: la próxima carga dirá la verdad
	}
	a.mu.Lock()
	for i := range a.items {
		if a.items[i].ID == id && !a.items[i].Read {
			a.items[i].Read = true
			a.sinLeer = max(a.sinLeer-1, 0)
		}
	}
	a.mu.Unlock()
	a.refrescar()
}

// leerTodos es «Marcar todas como leídas»: el Hub las marca en la base, y aquí se
// ponen a cero el contador y los galones.
//
//nolint:contextcheck // lo lanza un clic, no el flujo: ver leido
func (a *avisosBarra) leerTodos() {
	hub := a.hubActual()
	if hub == nil {
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	if hub.MarkAllRead(ctx) != nil {
		return
	}
	a.mu.Lock()
	for i := range a.items {
		a.items[i].Read = true
	}
	a.sinLeer = 0
	a.mu.Unlock()
	a.refrescar()
}

// hubActual es el cliente del Hub de la sesión de ahora, leído con el cerrojo: la
// sesión puede cambiar desde otra rutina mientras esto corre.
func (a *avisosBarra) hubActual() *api.Hub {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.hub
}

// verTodas es la salida a la bandeja completa, que vive en Sytue Hub: el panel enseña
// las últimas y no tiene filtros, «marcar todas» ni las reglas de silencio.
//
// Es un ENLACE en `px-3 py-2` (`text-primary-text text-sm font-semibold` con su «→»), no
// un elemento de menú con campana: así lo pinta la web.
func (a *avisosBarra) verTodas() fyne.CanvasObject {
	e := NuevoEnlace(a.t.T("notifications.viewAll", nil)+" →", tema.TextoSM, SemiNegrita, "primary-text", func() {
		a.menu.Cerrar()
		_ = api.AbrirEn(fyne.CurrentApp(), a.apex(), "one", a.org(), "/notificaciones")
	})
	return container.New(layout.NewCustomPaddedLayout(MargenPanelY, MargenPanelY, MargenPanelX, MargenPanelX),
		container.NewHBox(e))
}

// ponerInsignia lleva al icono de la app el número que toca, solo cuando cambia.
func (a *avisosBarra) ponerInsignia() {
	id := fyne.CurrentApp().Metadata().ID
	a.mu.Lock()
	n := insigniaDe(productoDeID(id), a.items, a.sinLeer)
	cambia := n != a.enIcono
	a.enIcono = n
	a.mu.Unlock()
	if cambia {
		_ = escritorio.Insignia(id, n)
	}
}

// ProcedenciaAviso es lo que va delante de cuándo llegó un aviso, con su « · »: la
// organización que lo mandó si no es la de la sesión (ADR 0053) y, si se reconstruyó
// de lo que ya había, que es anterior al registro de avisos. Es `procedenciaAviso` de
// la web (avisos.ts).
func ProcedenciaAviso(t *i18n.Translator, n api.HubNotification) string {
	var s string
	if org := api.OrgVisible(n.Organization, n.OrganizationName); org != "" {
		s += org + " · "
	}
	if n.Historic {
		s += t.T("notifications.historic", nil) + " · "
	}
	return s
}

// TituloAviso es el título de un aviso en el idioma de quien lo mira: el Hub trae
// `type` y `params` y el catálogo común tiene `notifications.kind.<type>`. Si el tipo
// no está —un emisor más nuevo que esta app— o falta una variable, sale el `title`
// que mandó el emisor, que es su respaldo. Es `tituloAviso` de la web (avisos.ts).
func TituloAviso(t *i18n.Translator, n api.HubNotification) string {
	clave := "notifications.kind." + n.Type
	vars := i18n.Vars{}
	for k, v := range n.Params {
		if v != "" {
			vars[k] = v
		}
	}
	// ponytail: «falta una variable» se ve en un `{` que queda; una variable cuyo valor
	// traiga llaves también cae al respaldo, que dice lo mismo en otro idioma.
	if s := t.T(clave, vars); n.Type != "" && s != clave && !strings.Contains(s, "{") {
		return s
	}
	return n.Title
}

// CuerpoAviso es lo que el emisor mandó (el asunto de un correo, un comentario) o, si
// no trae nada, la hora de lo que avisa en la zona de esta máquina (`params.start`:
// un recordatorio, una invitación). Es `cuerpoAviso` de la web.
func CuerpoAviso(l i18n.Locale, n api.HubNotification) string {
	if n.Body != "" {
		return n.Body
	}
	if t, err := time.Parse(time.RFC3339, n.Params["start"]); err == nil {
		return i18n.FormatDateTime(l, t.Local())
	}
	return ""
}

// EL FLUJO DEL HUB, PARA LA APP. La campana ya tiene abierto el flujo de avisos; en vez
// de que cada app abra el suyo (o sondee), cada aviso NUEVO se reparte a quien lo
// escuche. Es lo que desbloquea en vivo la franja de solo lectura de las apps sin canal
// propio: al conceder, llega `<producto>.access.granted` con `params.resource`. El
// espejo de `alLlegarAviso` de @sytue/shell.
var (
	muOyentes sync.Mutex
	oyentes   = map[int]func(api.HubNotification){}
	oyente    int
)

// AlLlegarAviso llama a `fn` (en la rutina del flujo) con cada aviso nuevo del Hub.
// Devuelve con qué dejar de escuchar.
func AlLlegarAviso(fn func(api.HubNotification)) (quitar func()) {
	muOyentes.Lock()
	defer muOyentes.Unlock()
	oyente++
	id := oyente
	oyentes[id] = fn
	return func() {
		muOyentes.Lock()
		defer muOyentes.Unlock()
		delete(oyentes, id)
	}
}

func avisarOyentes(n api.HubNotification) {
	muOyentes.Lock()
	fs := make([]func(api.HubNotification), 0, len(oyentes))
	for _, f := range oyentes {
		fs = append(fs, f)
	}
	muOyentes.Unlock()
	for _, f := range fs {
		f(n)
	}
}

// RepartirAviso reparte `n` a quien escuche con AlLlegarAviso, como si hubiera llegado
// por el flujo. Lo usan las pruebas de las apps.
func RepartirAviso(n api.HubNotification) { avisarOyentes(n) }

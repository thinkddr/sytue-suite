// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/gif"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/llamada/camara"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

// Sala es Sytue Rooms CON DATOS: habla con el servicio y rellena la Pantalla. Es el
// `ChatApp.tsx` de la web sin su marcado —el marcado ya es la Pantalla— y cada flujo
// lleva al lado el efecto de allí del que sale.
//
// # En qué hilo corre cada cosa
//
// Todo el estado se toca en el hilo de la interfaz. Lo único que sale de él es la
// llamada al servicio, que corre en `Lanzar` y vuelve con `fyne.Do`, igual que el
// buzón de Mail. Una respuesta que llega tarde —el canal de antes, cuando ya se ha
// abierto otro— se tira por la VISTA: cada apertura sube el número, y lo que vuelve
// con un número viejo no pinta. Es el `live = false` de cada efecto de la web.

// Servicio es lo que Sytue Rooms le pide al servidor. Lo cumple *api.Rooms; los
// guardianes le pasan un doble en memoria.
type Servicio interface {
	Teams(ctx context.Context) ([]api.RoomsTeam, error)
	Channels(ctx context.Context, equipo string) ([]api.RoomsChannel, error)
	Unread(ctx context.Context, equipo string) ([]api.RoomsUnread, error)
	ExternalChannels(ctx context.Context) ([]api.RoomsChannel, []api.RoomsUnread, error)
	Categories(ctx context.Context) ([]api.RoomsCategory, error)
	Messages(ctx context.Context, canal string, antes time.Time, limite int) ([]api.RoomsMessage, error)
	Post(ctx context.Context, canal, texto string) (api.RoomsMessage, error)
	PostWith(ctx context.Context, canal, texto string, o api.RoomsSend) (api.RoomsMessage, error)
	ReplyWith(ctx context.Context, raiz, texto string, o api.RoomsSend) (api.RoomsMessage, error)
	Schedule(ctx context.Context, canal, texto string, fileIDs []string, cuando time.Time) (api.RoomsScheduled, error)
	Reschedule(ctx context.Context, id string, cuando time.Time) (api.RoomsScheduled, error)
	Ack(ctx context.Context, mensaje string) (int, error)
	Readers(ctx context.Context, mensaje string) ([]api.RoomsReader, int, error)
	VoteCard(ctx context.Context, mensaje, valor string, poner bool) ([]api.RoomsCardVote, error)
	UpdateChannel(ctx context.Context, canal string, cambio api.RoomsChannelPatch) (api.RoomsChannel, error)
	LeaveChannel(ctx context.Context, canal string) error
	AddTeamMember(ctx context.Context, equipo, persona, nombre string) error
	RemoveTeamMember(ctx context.Context, equipo, persona string) error
	Commands(ctx context.Context) ([]api.RoomsCommand, error)
	ExecuteCommand(ctx context.Context, canal, linea string) (api.RoomsCommandResult, error)
	NotifyPrefs(ctx context.Context) (api.RoomsNotifyPrefs, error)
	PutNotifyPrefs(ctx context.Context, p api.RoomsNotifyPrefs) (api.RoomsNotifyPrefs, error)
	MarkRead(ctx context.Context, canal string) error
	React(ctx context.Context, mensaje, emoji string, poner bool) error
	SetFavorite(ctx context.Context, canal string, favorito bool) error
	SetNotifyLevel(ctx context.Context, canal, nivel string) error
	SetChannelCategory(ctx context.Context, canal, categoria string) error
	ArchiveChannel(ctx context.Context, canal string) error
	DeleteChannel(ctx context.Context, canal string) error
	HideDirect(ctx context.Context, canal string) error
	CreateCategory(ctx context.Context, nombre string) (api.RoomsCategory, error)
	UpdateCategory(ctx context.Context, id string, cambio api.RoomsCategoryPatch) error
	DeleteCategory(ctx context.Context, id string) error
	Drafts(ctx context.Context) ([]api.RoomsDraft, error)
	Scheduled(ctx context.Context) ([]api.RoomsScheduled, error)
	CancelScheduled(ctx context.Context, id string) error
	PutDraft(ctx context.Context, canal, raiz, texto string) error
	Edit(ctx context.Context, mensaje, texto string) (api.RoomsMessage, error)
	Delete(ctx context.Context, mensaje string) error
	Pin(ctx context.Context, mensaje string, poner bool) error
	Save(ctx context.Context, mensaje string, poner bool) error
	MarkUnread(ctx context.Context, mensaje string) error
	Pins(ctx context.Context, canal string) ([]api.RoomsMessage, error)
	Saved(ctx context.Context) ([]api.RoomsMessage, error)
	MyThreads(ctx context.Context, antes string) ([]api.RoomsThreadSummary, string, error)
	Search(ctx context.Context, q string, limite int) ([]api.RoomsMessage, error)
	CreateChannel(ctx context.Context, equipo, nombre, asunto string, privado bool) (api.RoomsChannel, error)
	Browse(ctx context.Context, equipo, q string) ([]api.RoomsChannel, error)
	OpenDirect(ctx context.Context, equipo string, personas []string) (api.RoomsChannel, error)
	TeamMembers(ctx context.Context, equipo string) ([]api.RoomsMember, error)
	ResolvePerson(ctx context.Context, consulta string) (api.RoomsPerson, error)
	ChannelMembers(ctx context.Context, canal string) ([]api.RoomsMember, error)
	AddChannelMember(ctx context.Context, canal, persona, nombre string) (api.RoomsMember, error)
	RemoveChannelMember(ctx context.Context, canal, persona string) error
	SetChannelMemberRole(ctx context.Context, canal, persona, rol string) (api.RoomsMember, error)
	Join(ctx context.Context, canal string) (api.RoomsChannel, error)
	Around(ctx context.Context, canal, mensaje string, limite int) ([]api.RoomsMessage, string, error)
	Thread(ctx context.Context, raiz string) (api.RoomsThread, error)
	Follow(ctx context.Context, raiz string, seguir bool) error
	Listen(ctx context.Context, canales []string, alLlegar func(api.RoomsEvent)) error
	Typing(ctx context.Context, canal string) error
}

var _ Servicio = (*api.Rooms)(nil)

// Los números de la web.
const (
	// PorPagina es `PAGE_SIZE` de `ChatApp.tsx`.
	PorPagina = 50
	// VidaEscribiendo es lo que dura «X está escribiendo…» sin otra trama: 5 s.
	VidaEscribiendo = 5 * time.Second
	// ReconectarBase y ReconectarTope son el respaldo exponencial del tiempo real de
	// `connectRealtime` (1 s que se dobla hasta 30 s).
	ReconectarBase = time.Second
	ReconectarTope = 30 * time.Second
)

// Sala es el estado de Sytue Rooms con datos.
type Sala struct {
	P       *Pantalla
	S       Servicio
	Cifrado *Cifrado
	// Yo es el `sub` de quien ha entrado: decide qué reacciones y qué mensajes son
	// suyos.
	Yo string
	// AdminOrg dice si quien mira administra la ORGANIZACIÓN (rol owner o admin del
	// token): le abre la cola de denuncias aunque no administre ningún equipo.
	AdminOrg bool
	// Account consent is shared with Hub/Drive; provider secrets stay in the gateway.
	CuentasApps             *api.Integraciones
	AutorizarApp            func(context.Context, string, string) (string, error)
	AutorizarAppNueva       func(context.Context, string) (string, error)
	AutorizarConversaciones func(context.Context, string, string) (string, error)
	AutorizarReunion        func(context.Context, string, string) (string, error)
	GeneracionSesion        func() string
	comprobarWhatsApp       func(string)
	cerrarConexionApps      func()
	// Apex es el dominio de la suite, del que sale el enlace a un mensaje.
	Apex string
	// Org es la organización activa de la sesión, leída VIVA del token: encabeza el
	// enlace de un mensaje (ADR 0056), como `orgActual` en la web. Lo pone el `main`
	// con `s.Sesion.Organizacion`; nil es «no la sé», y el enlace sale sin org.
	Org func() string
	// AlPonerPresencia publica la presencia en el Hub (la sesión transversal); corre
	// fuera del hilo de la interfaz.
	AlPonerPresencia func(ctx context.Context, e api.EstadoPresencia, texto string)
	// Ahora es el reloj de la caducidad del estado; los guardianes lo congelan.
	Ahora func() time.Time
	// Copiar deja un texto en el portapapeles del sistema; los guardianes lo apuntan.
	Copiar func(string)
	// Abridor invoca a otra app de la suite por su esquema (Notepads, al guardar un
	// mensaje en ella); los guardianes lo apuntan.
	Abridor api.Abridor
	// IA es Sytue AI, para «Ponme al día»; sin ella, el resumen dice que el asistente
	// no está configurado, como la web sin `SYTUE_AI_URL`.
	IA Asistente
	// Almacen guarda lo que la web deja en `localStorage`, por persona.
	Almacen Almacen
	// Presencias pregunta al Hub por la de los demás (el punto de un directo).
	Presencias Presencias
	// PrefsAviso y PonerPrefAviso son las reglas de silencio del Hub: «Activar avisos de
	// mensajes directos».
	PrefsAviso     func(ctx context.Context) ([]api.HubNotificationPref, error)
	PonerPrefAviso func(ctx context.Context, p api.HubNotificationPref) (api.HubNotificationPref, error)
	// Ficheros es Sytue Drive, donde viven los adjuntos de cada canal. ElegirFicheros es
	// el selector del sistema (BLOQUEA); Revelar y AbrirFichero, el gestor de ficheros y
	// la aplicación asociada. Los guardianes los sustituyen.
	Ficheros       Ficheros
	adjuntosEpoch  uint64
	ElegirFicheros func() ([]string, error)
	// AbrirEnlace abre un enlace en el navegador del sistema (las vistas previas).
	AbrirEnlace func(*url.URL) error
	// ListarDrive es el selector de la suite en modo Drive (`api.Elegir.Listar`).
	ListarDrive  func(ctx context.Context, carpeta, q, cursor string) (api.PaginaSuite, error)
	Revelar      func(string) error
	AbrirFichero func(string) error
	// Lanzar corre una llamada fuera del hilo de la interfaz; Esperar es el
	// temporizador. Los guardianes los hacen síncronos.
	Lanzar  func(func())
	Esperar func(time.Duration, func()) (parar func())
	// Conectar abre la llamada nativa con el SFU (`llamada.Entrar` con la tarjeta del
	// sistema). nil: las llamadas se abren en la web, como antes.
	Conectar Conector
	// Camara y Pantalla abren la cámara y la pantalla del sistema para la llamada
	// (`camara.Abrir`, `camara.AbrirPantalla`); nil, no se ofrecen.
	Camara, Pantalla func() (camara.Fuente, error)
	// MicroElegido, AltavozElegido y CamaraElegida son los dispositivos que se eligieron
	// en «Dispositivos» de la llamada; vacíos, los de por omisión.
	MicroElegido, AltavozElegido, CamaraElegida string
	// Dispositivos lista los micros, altavoces y cámaras (`altavoz.Dispositivos`,
	// `camara.Dispositivos`) y AudioDe abre la tarjeta con los elegidos; nil, «Configuración
	// de dispositivos» no se ofrece.
	Dispositivos func() (micros, altavoces, camaras []Opcion)
	AudioDe      func(micro, altavoz string) llamada.Dispositivo
	// PedirMicro pide el permiso del micrófono al abrir la antesala (Android); nil, no
	// hace falta.
	PedirMicro func() error

	ctx     context.Context
	equipo  string
	canales []api.RoomsChannel
	// retiradas invalida el deshacer de una acción cuando llega una baja más reciente.
	retiradas map[string]uint64
	pend      map[string]api.RoomsUnread
	abierto   string
	// enlace es el canal (y el mensaje) de un aviso que llegó antes que la lista.
	enlace   [2]string
	mensajes []api.RoomsMessage
	vista    int
	// escribiendo es quién escribe en cada canal (canal → sub → nombre) y el
	// temporizador que lo borra.
	escribiendo map[string]map[string]string
	// avisoEscribiendo es cuándo salió el último «escribiendo» propio.
	avisoEscribiendo time.Time
	// borradores es lo escrito y no enviado por canal o hilo; subidas, el rebote de
	// cada uno hacia el servidor; restaurando calla a la caja mientras se le pone uno.
	borradores   map[ambito]string
	subidas      map[ambito]func()
	restaurando  bool
	canalHilo    string
	caducan      map[string]func()
	pararEscucha func()
	espera       time.Duration
	// hilo es el panel del hilo, que se crea la primera vez que se abre uno;
	// raizHilo, la raíz del abierto ("" si no hay), y vistaHilo su número de vista.
	hilo      *PanelHilo
	raizHilo  string
	vistaHilo int
	// fijados y guardados son los ids fijados en el canal abierto y los que quien
	// mira ha guardado: deciden «Fijar/Desfijar» y «Guardar/Quitar de guardados».
	fijados, guardados map[string]bool
	// bloqueados es a quién tiene bloqueado quien mira (ADR 0073); mostrados, los
	// mensajes de esas personas que se ha pedido ver. Ver moderacion.go.
	bloqueados []api.RoomsBlock
	// denunciasPendientes: llegó el enlace de la cola antes de arrancar.
	denunciasPendientes bool
	// invitacionesPorEnlace: el aviso de una invitación pide la lista, no el aviso que
	// lleva a ella (COM-15).
	invitacionesPorEnlace bool
	mostrados             map[string]bool
	// separador es el mensaje ante el que va «Nuevos mensajes», o "".
	separador string
	// roster es el del canal abierto; adminEquipo, si quien mira administra el equipo.
	roster      []Miembro
	adminEquipo bool
	miembros    *PanelRoster
	menuCanal   *ui.Menu
	menuFila    *ui.Menu
	menuCat     *ui.Menu
	emojis      *SelectorEmoji
	pregunta    *ui.Dialogo
	// reuniones es el panel «Reuniones» abierto; integraciones, el diálogo de «Apps e
	// integraciones».
	reuniones     *HojaReuniones
	integraciones *CuerpoIntegraciones
	gif           *SelectorGif
	// ventanaLlamada es la ventana de la llamada en curso (una a la vez) y llamada, su
	// vista ya conectada.
	ventanaLlamada fyne.Window
	llamada        *VistaLlamada
	// arrancada es Arrancar ya hecho, y reunionPendiente la reunión de un enlace que
	// llegó antes.
	arrancada        bool
	reunionPendiente string
	// claves son las de los enlaces de las reuniones sueltas que se conocen (id →
	// clave): la de la lista, la de la que se acaba de convocar y la del enlace por el
	// que se entró. Se toca solo desde el hilo de la interfaz.
	claves map[string]string
	// canalLlamada es el canal de la llamada en curso, si lo hay: su chat.
	canalLlamada string
	// imagenesEmoji son las imágenes del catálogo de la organización ya bajadas, por
	// fichero, y pidiendoEmoji las que están en camino o no bajaron.
	imagenesEmoji map[string]image.Image
	pidiendoEmoji map[string]bool
	// gifsEmoji son los GIF del catálogo ya descodificados enteros para animarlos, y
	// pidiendoGif los que están en camino o no bajaron.
	gifsEmoji   map[string]*gif.GIF
	pidiendoGif map[string]bool
	// noLeidoAbierto enseña el contador del canal abierto: lo pide «Marcar como no
	// leído» y dura hasta cambiar de canal.
	noLeidoAbierto bool
	// lista es el panel de lista abierto (fijados, guardados o búsqueda) y panel su
	// clave; vistaLista descarta la respuesta de una búsqueda vieja.
	lista      *PanelLista
	panel      string
	vistaLista int
	// seguidos es la vista «Hilos» (`threadsList`): se carga al arrancar para la
	// pastilla del carril y el panel la recarga al abrirse. seguidosSig es el cursor
	// de la página siguiente.
	seguidos         []api.RoomsThreadSummary
	seguidosSig      string
	cargandoSeguidos bool
	// seguir son los seguimientos que el servidor ha confirmado en esta sesión
	// (`followOverrides`): mandan sobre la página de «Hilos».
	seguir     map[string]bool
	pararBusca func()
	// conmutador y crearCanal son los últimos diálogos abiertos (para los guardianes).
	pararCaducidad func()
	bloqueEstado   *BloqueEstado
	conmutador     *Conmutador
	crearCanal     *CrearCanal
	explorar       *Explorar
	directo        *NuevoDirecto
	reenviar       *Reenviar
	compartir      *Compartir
	selectorDrive  *ui.SelectorSuite
	// directosCallados es la regla del Hub que calla los avisos de los directos.
	directosCallados bool
	// presencias es lo último que dijo el Hub de cada `sub`; pararPresencia, el sondeo.
	presencias     map[string]api.Presencia
	pararPresencia func()
	// prefsAviso son las últimas guardadas y ajustesAviso el diálogo abierto.
	prefsAviso   api.RoomsNotifyPrefs
	ajustesAviso *AjustesAviso
	menuMas      *ui.Menu
	// guardandoAdjunto frena el doble clic en «Guardar en mi Drive»; locales numera las
	// fichas de lo que se sube.
	guardandoAdjunto string
	locales          int
	// miniaturas son las de los adjuntos ya pedidas (nil: pedida y sin llegar).
	miniaturas map[string]image.Image
	// enlaces son las vistas previas ya pedidas, por enlace.
	enlaces map[string]vistaEnlace
	// resumen es el último «Ponme al día» enseñado (para los guardianes).
	resumen string
	// viendoHistorial es el modo de la web tras saltar a un mensaje viejo: el final
	// del canal no se recarga solo y sale «Ir a mensajes recientes».
	viendoHistorial bool
	pararResalte    func()
	pararCifrado    func()
}

// NuevaSala cablea la Pantalla con el servicio. No pide nada: eso es Arrancar.
func NuevaSala(p *Pantalla, s Servicio, yo string) *Sala {
	sa := &Sala{
		P: p, S: s, Yo: yo,
		Lanzar:      func(f func()) { go f() },
		Esperar:     esperar,
		ctx:         context.Background(),
		pend:        map[string]api.RoomsUnread{},
		retiradas:   map[string]uint64{},
		escribiendo: map[string]map[string]string{},
		caducan:     map[string]func(){},
		fijados:     map[string]bool{},
		guardados:   map[string]bool{},
		seguir:      map[string]bool{},
		presencias:  map[string]api.Presencia{},
		miniaturas:  map[string]image.Image{},
		enlaces:     map[string]vistaEnlace{},
		borradores:  map[ambito]string{},
		subidas:     map[ambito]func(){},
		Copiar:      func(s string) { fyne.CurrentApp().Clipboard().SetContent(s) },
		// Las imágenes del catálogo de la organización.
		imagenesEmoji: map[string]image.Image{},
		pidiendoEmoji: map[string]bool{},
		gifsEmoji:     map[string]*gif.GIF{},
		pidiendoGif:   map[string]bool{},
	}
	// El registro de paquete es de ESTA Sala: su catálogo llega al arrancar y sus
	// imágenes, de su Drive.
	PonerCatalogoOrg(nil)
	catalogoOrg.imagen = sa.imagenEmoji
	if a := fyne.CurrentApp(); a != nil {
		sa.Abridor, sa.AbrirEnlace = a, func(u *url.URL) error { return api.AbrirFuera(a, u.String()) }
	}
	l := p.Lateral
	l.AlElegirCanal = sa.Elegir
	l.AlReintentar = sa.cargarCanales
	l.AlFavorito = sa.Favorito
	l.AlMenuCanal = sa.MenuFila
	l.AlPlegarSeccion = sa.PlegarSeccion
	l.AlNuevaCategoria = sa.NuevaCategoria
	l.AlMenuCategoria = sa.MenuCategoria
	p.Canal.Historial.AlReintentar = func() { sa.cargarMensajes(sa.abierto, 0) }
	p.Canal.Historial.AlReaccionar = sa.Reaccionar
	p.Canal.Historial.AlAbrirHilo = sa.AbrirHilo
	p.Canal.Historial.AlAcusar = sa.Acusar
	p.Canal.Historial.AlVotar = sa.Votar
	p.Canal.Historial.AdjuntosDe = sa.adjuntosParaFila
	p.Canal.Historial.EnlacesDe = sa.tarjetasEnlace
	p.Canal.Historial.AlCargarAnteriores = sa.CargarAnteriores
	p.Canal.Historial.Menu = sa.menuDe
	p.Canal.Historial.AlGuardarEdicion = sa.GuardarEdicion
	p.Canal.Historial.AlCancelarEdicion = sa.CancelarEdicion
	p.Canal.Compositor.AlEnviar = sa.Enviar
	p.Canal.Compositor.AlComando = sa.Comando
	p.Canal.Compositor.AlComandoRemoto = sa.ComandoRemoto
	p.Canal.Compositor.AlFormato = sa.guardarFormato
	p.Canal.Compositor.AlMas = sa.MenuMas
	p.Canal.Historial.AlEmojis = func(m Mensaje, d fyne.CanvasObject) {
		sa.AbrirEmojis(d, func(e string) { sa.Reaccionar(m, e) })
	}
	p.Canal.Compositor.AlEmoji = func() {
		c := sa.P.Canal.Compositor
		sa.AbrirEmojis(c.BotonEmoji(), c.Insertar)
	}
	p.Canal.Compositor.AlCambiarTexto = func(s string) { sa.alTeclear(ambito{canal: sa.abierto}, s) }
	p.Canal.Cabecera.AlAbrirPanel = sa.AbrirPanel
	p.Canal.Cabecera.AlAbrirMenu = sa.MenuCanal
	p.Canal.Cabecera.AlVerArchivos = sa.VerArchivos
	p.Canal.Cabecera.AlResumir = sa.PonmeAlDia
	p.Canal.Cabecera.AlEditarTema = sa.EditarTema
	p.Canal.Cabecera.AlGuardarTema = sa.GuardarTema
	p.Canal.Cabecera.AlCancelarTema = sa.CancelarTema
	p.Canal.Cabecera.AlLlamar = sa.LlamarEnCanal
	p.Canal.Historial.AlEntrarEnLlamada = func(Mensaje) { sa.LlamarEnCanal() }
	p.Canal.AlIrAlFinal = sa.IrAlFinal
	l.AlIrACanal = sa.AbrirConmutador
	l.AlCrearCanal = sa.AbrirCrearCanal
	l.AlExplorar = sa.AbrirExplorar
	l.AlNuevoDirecto = sa.AbrirNuevoDirecto
	l.AlCambiarEstado = sa.AbrirEstado
	l.AlAbrirPanel = sa.AbrirPanel
	l.AlSoltarCanal = sa.SoltarCanal
	l.AlSoltarCategoria = func(k, destino Categoria) { sa.ReordenarCategoria(k.ID, destino.ID) }
	return sa
}

func (sa *Sala) ahora() time.Time {
	if sa.Ahora != nil {
		return sa.Ahora()
	}
	return time.Now()
}

func esperar(d time.Duration, f func()) func() {
	t := time.AfterFunc(d, func() { fyne.Do(f) })
	return func() { t.Stop() }
}

// pedir hace una llamada fuera del hilo de la interfaz y vuelve a él con el
// resultado. Es la única puerta al servicio.
func pedir[T any](sa *Sala, peticion func(context.Context) (T, error), luego func(T, error)) {
	pedirEn(sa.ctx, sa, peticion, luego)
}

// pedirEn permite cancelar una búsqueda sin cerrar la sala ni duplicar la puerta
// común al servicio. El contexto debe derivarse del ciclo de vida de la sala.
func pedirEn[T any](ctx context.Context, sa *Sala, peticion func(context.Context) (T, error), luego func(T, error)) {
	sa.Lanzar(func() {
		v, err := peticion(ctx)
		fyne.Do(func() {
			if ctx == nil || ctx.Err() == nil {
				if sa.Cifrado != nil && errors.Is(err, mx.ErrUnverified) {
					sa.Cifrado.PedirVerificacion() //nolint:contextcheck // Fyne dialog actions use the current device lifecycle; this callback only renders them.
				}
				luego(v, err)
			}
		})
	})
}

// Arrancar pide los canales —que abren el primero— y abre el tiempo real.
func (sa *Sala) Arrancar(ctx context.Context) {
	sa.ctx = ctx
	sa.restaurarPreferencias()
	sa.cargarCanales() //nolint:contextcheck // el contexto viaja en sa.ctx: cargarCanales es también el «Reintentar» del carril, que no tiene uno
	sa.cargarGuardados()
	sa.cargarSeguidos()
	sa.cargarBorradores()
	sa.cargarComandos()
	sa.cargarAvisosDirectos()
	sa.cargarBloqueados()
	sa.cargarEmojisOrg()
	sa.arrancada = true
	if sa.denunciasPendientes {
		sa.denunciasPendientes = false
		sa.AbrirDenuncias()
	}
	if id := sa.reunionPendiente; id != "" {
		sa.reunionPendiente = ""
		sa.EntrarPorEnlace(id, sa.claves[id]) //nolint:contextcheck // el contexto viaja en sa.ctx, como el de cargarCanales
	}
}

// Parar cierra el tiempo real y los temporizadores. Es el `return () => …` del efecto.
func (sa *Sala) Parar() {
	sa.cancelarAdjuntos()
	if sa.pararCifrado != nil {
		sa.pararCifrado()
		sa.pararCifrado = nil
	}
	if sa.cerrarConexionApps != nil {
		sa.cerrarConexionApps()
	}
	sa.ColgarLlamada()
	if sa.pararEscucha != nil {
		sa.pararEscucha()
		sa.pararEscucha = nil
	}
	for k, parar := range sa.caducan {
		parar()
		delete(sa.caducan, k)
	}
	if sa.pararPresencia != nil {
		sa.pararPresencia()
		sa.pararPresencia = nil
	}
}

// PresenciaDe es el punto del carril que le toca a un estado del servicio; uno que no
// conozca no pinta punto. Es la tabla del menú de cuenta (`ui.PresenciaDe`).
func PresenciaDe(e api.EstadoPresencia) ui.Presencia {
	p, _ := ui.PresenciaDe(e)
	return p
}

// estadoDe traduce el error de una llamada al estado que se pinta: un 403 es «sin
// permiso», otra respuesta del servidor es «error», y una petición que no llegó a
// tener respuesta es «sin conexión».
func estadoDe(err error) Estado {
	var e *api.Error
	switch {
	case errors.As(err, &e) && e.Status == http.StatusForbidden:
		return SinPermiso
	case errors.As(err, &e), errors.Is(err, context.Canceled):
		return Error
	}
	return SinConexion
}

// ---- Canales: `listChannels` ----

// lista es lo que vuelve de pedir los canales: el equipo, los suyos y los de fuera, y
// lo pendiente de todos. Los contadores y las categorías degradan a vacío si su
// llamada falla, como en la web: los canales cargan igual.
type lista struct {
	equipo      string
	adminEquipo bool
	canales     []api.RoomsChannel
	pend        []api.RoomsUnread
	categorias  []api.RoomsCategory
}

func (sa *Sala) pedirLista(ctx context.Context) (lista, error) {
	equipos, err := sa.S.Teams(ctx)
	if err != nil {
		return lista{}, err
	}
	if len(equipos) == 0 {
		if fuera, pf, errExt := sa.S.ExternalChannels(ctx); errExt == nil && len(fuera) > 0 {
			out := lista{canales: fuera, pend: pf}
			out.categorias, _ = sa.S.Categories(ctx)
			return out, nil
		}
		return lista{}, errors.New("rooms: no teams found")
	}
	out := lista{equipo: equipos[0].ID}
	for _, e := range equipos {
		out.adminEquipo = out.adminEquipo || e.Role == "admin" // `isWorkspaceAdmin`
	}
	if out.canales, err = sa.S.Channels(ctx, out.equipo); err != nil {
		return lista{}, err
	}
	out.pend, _ = sa.S.Unread(ctx, out.equipo)
	if fuera, pf, err := sa.S.ExternalChannels(ctx); err == nil {
		out.canales = append(out.canales, fuera...)
		out.pend = append(out.pend, pf...)
	}
	out.categorias, _ = sa.S.Categories(ctx)
	return out, nil
}

func (sa *Sala) cargarCanales() {
	l := sa.P.Lateral
	l.EstadoCanales = Cargando
	sa.P.Refrescar()
	pedir(sa, sa.pedirLista, func(r lista, err error) {
		if err != nil {
			l.EstadoCanales = Error
			sa.P.Refrescar()
			return
		}
		sa.equipo, sa.canales = r.equipo, slices.DeleteFunc(r.canales, func(c api.RoomsChannel) bool { return c.Archived })
		sa.adminEquipo = r.adminEquipo
		l.PuedeGestionarApps = r.adminEquipo // `canManageIntegrations`: admin de algún equipo
		sa.ponerPendientes(r.pend)
		l.Categorias = l.Categorias[:0]
		for _, c := range r.categorias {
			l.Categorias = append(l.Categorias, Categoria{ID: c.ID, Nombre: c.Name, Orden: c.SortOrder, Plegada: c.Collapsed})
		}
		l.EstadoCanales = Listo
		sa.pintarCanales()
		sa.mirarInvitaciones()
		sa.escuchar()
		sa.refrescarPresencia()
		e := sa.enlace
		sa.enlace = [2]string{}
		if slices.ContainsFunc(sa.canales, func(c api.RoomsChannel) bool { return c.ID == e[0] }) {
			sa.IrAEnlace(e[0], e[1])
			return
		}
		// `setActiveId((prev) => prev ?? list[0]?.id ?? null)`: el primero de la lista
		// del servicio, no el primero del carril.
		if sa.abierto == "" && len(sa.canales) > 0 {
			sa.Elegir(sa.canalDe(sa.canales[0]))
			return
		}
		sa.P.Refrescar()
	})
}

func (sa *Sala) ponerPendientes(ps []api.RoomsUnread) {
	clear(sa.pend)
	for _, p := range ps {
		sa.pend[p.ChannelID] = p
	}
}

// pintarCanales pasa los canales del servicio al carril. El canal abierto no cuenta
// como pendiente: se está leyendo.
func (sa *Sala) pintarCanales() {
	l := sa.P.Lateral
	l.Canales = l.Canales[:0]
	for _, c := range sa.canales {
		l.Canales = append(l.Canales, sa.canalDe(c))
	}
}

// canalDe es `toChannel` con sus contadores.
func (sa *Sala) canalDe(c api.RoomsChannel) Canal {
	out := Canal{
		ID: c.ID, Nombre: c.Title(), Tema: c.Topic, Proposito: c.Purpose, CategoriaID: c.CategoryID,
		Orden: c.SortOrder, Favorito: c.Favorite, Silenciado: c.Muted, Externo: c.External,
	}
	switch c.Kind {
	case "direct":
		out.Clase = Directo
	case "private":
		out.Clase = Privado
	}
	sa.presenciaDe(c, &out)
	switch c.NotifyLevel {
	case "mentions":
		out.Nivel = AvisarMenciones
	case "none":
		out.Nivel = AvisarNada
	}
	if p, hay := sa.pend[c.ID]; hay && (c.ID != sa.abierto || sa.noLeidoAbierto) {
		out.SinLeer, out.Menciones = p.Unread, p.Mentions
	}
	return out
}

// refrescarPendientes vuelve a pedir los contadores, sin tocar la lista ni el canal.
func (sa *Sala) refrescarPendientes() {
	if sa.equipo == "" {
		return
	}
	pedir(sa, func(ctx context.Context) ([]api.RoomsUnread, error) {
		ps, err := sa.S.Unread(ctx, sa.equipo)
		if err != nil {
			return nil, err
		}
		if _, pf, err := sa.S.ExternalChannels(ctx); err == nil {
			ps = append(ps, pf...)
		}
		return ps, nil
	}, func(ps []api.RoomsUnread, err error) {
		if err != nil {
			return
		}
		sa.ponerPendientes(ps)
		sa.pintarCanales()
		sa.P.Refrescar()
	})
}

// Favorito conmuta la estrella de un canal: se pinta ya y se deshace si el servicio
// dice que no, que es lo que hace `toggleFavorite`.
func (sa *Sala) Favorito(c Canal) {
	poner := !c.Favorito
	sa.cambiarCanal(c.ID, func(x *api.RoomsChannel) { x.Favorite = poner }, func(x *api.RoomsChannel) { x.Favorite = !poner },
		func(ctx context.Context) error { return sa.S.SetFavorite(ctx, c.ID, poner) })
}

// cambiarCanal es el patrón de las preferencias de una fila: el cambio se ve ya, se
// pide, y si el servicio dice que no se deshace SOLO ese campo.
func (sa *Sala) cambiarCanal(id string, hacer, deshacer func(*api.RoomsChannel), peticion func(context.Context) error) {
	aplicar := func(f func(*api.RoomsChannel)) {
		if i := slices.IndexFunc(sa.canales, func(x api.RoomsChannel) bool { return x.ID == id }); i >= 0 {
			f(&sa.canales[i])
			sa.pintarCanales()
			sa.P.Refrescar()
		}
	}
	aplicar(hacer)
	pedir(sa, func(ctx context.Context) (struct{}, error) { return struct{}{}, peticion(ctx) },
		func(_ struct{}, err error) {
			if err != nil {
				aplicar(deshacer)
			}
		})
}

// nivelesAviso son los tres de `levelItem`, en el orden de Aviso, con su nombre en el
// servicio y su clave.
var nivelesAviso = [...]struct{ api, clave string }{
	AvisarTodo: {"all", "notify.levelAll"}, AvisarMenciones: {"mentions", "notify.levelMentions"},
	AvisarNada: {"none", "notify.levelNone"},
}

// MenuFila es el «…» de una fila del carril (`renderChannelRow`): favorito, el nivel
// de aviso con el tic en el vigente, mover a otra categoría —no a una conversación de
// otra organización, que no es de las tuyas—, salir y, a quien administra, archivar y
// borrar. En un directo, «Borrar chat».
func (sa *Sala) MenuFila(c Canal, disparador fyne.CanvasObject) {
	t := sa.P.T
	fav := t.T("channel.favourite", nil)
	if c.Favorito {
		fav = t.T("channel.unfavourite", nil)
	}
	es := []fyne.CanvasObject{ui.NuevoElementoMenu(fav, iconos.Ninguno, func() { sa.Favorito(c) }), ui.NuevoSeparadorMenu()}
	for n := AvisarTodo; n <= AvisarNada; n++ {
		it := ui.NuevoElementoMenu(t.T(nivelesAviso[n].clave, nil), iconos.Ninguno, func() { sa.PonerAviso(c, n) })
		it.Marcada = c.Nivel == n
		es = append(es, it)
	}
	if !c.Externo {
		cats := slices.Clone(sa.P.Lateral.Categorias)
		slices.SortStableFunc(cats, func(a, b Categoria) int {
			return cmp.Or(cmp.Compare(a.Orden, b.Orden), strings.Compare(a.Nombre, b.Nombre))
		})
		cats = slices.DeleteFunc(cats, func(k Categoria) bool { return k.ID == c.CategoriaID })
		if len(cats) > 0 || c.CategoriaID != "" {
			es = append(es, ui.NuevoSeparadorMenu())
		}
		for _, k := range cats {
			es = append(es, ui.NuevoElementoMenu(t.T("channel.moveTo", i18n.Vars{"name": k.Nombre}), iconos.Ninguno,
				func() { sa.MoverA(c, k.ID) }))
		}
		if c.CategoriaID != "" {
			es = append(es, ui.NuevoElementoMenu(t.T("channel.removeFromCategory", nil), iconos.Ninguno,
				func() { sa.MoverA(c, "") }))
		}
	}
	// Lo que la saca de tu carril, al final: salir, archivar y borrar en un canal;
	// «Borrar chat», que solo lo quita para ti, en un directo.
	es = append(es, ui.NuevoSeparadorMenu())
	if c.Clase == Directo {
		borrar := ui.NuevoElementoMenu(t.T("dm.delete", nil), iconos.Ninguno, func() { sa.PedirBorrarCanal(c) })
		borrar.Peligro = true
		es = append(es, borrar)
	} else {
		es = append(es, ui.NuevoElementoMenu(t.T("channel.leave", nil), iconos.Ninguno, func() { sa.PedirSalir(c) }))
		if sa.adminEquipo && !c.Externo {
			archivar := ui.NuevoElementoMenu(t.T("channel.archive", nil), iconos.Ninguno, func() { sa.PedirArchivar(c) })
			archivar.Peligro = true
			es = append(es, archivar)
		}
		if sa.puedeBorrar(c) {
			borrar := ui.NuevoElementoMenu(t.T("channel.delete", nil), iconos.Ninguno, func() { sa.PedirBorrarCanal(c) })
			borrar.Peligro = true
			es = append(es, borrar)
		}
	}
	m := ui.NuevoMenu(es...)
	m.Rotulo = t.T("channel.actions", i18n.Vars{"name": c.Nombre})
	sa.menuFila = m
	m.Mostrar(disparador)
}

// ---- Las categorías del carril ----

// PlegarSeccion es `toggleGroupCollapse`: una personalizada se guarda en el servidor
// (y vuelve si falla); una por defecto, en el almacén de la persona, que es el
// `localStorage` de la web.
func (sa *Sala) PlegarSeccion(s Seccion) {
	l := sa.P.Lateral
	if s.Categoria == nil {
		if l.Plegadas == nil {
			l.Plegadas = map[string]bool{}
		}
		l.Plegadas[s.Clave] = !l.Plegadas[s.Clave]
		sa.guardarPlegadas()
		sa.P.Refrescar()
		return
	}
	plegar := !s.Categoria.Plegada
	sa.cambiarCategoria(s.Categoria.ID, func(k *Categoria) { k.Plegada = plegar }, func(k *Categoria) { k.Plegada = !plegar },
		api.RoomsCategoryPatch{Collapsed: &plegar})
}

// cambiarCategoria es cambiarCanal para una categoría.
func (sa *Sala) cambiarCategoria(id string, hacer, deshacer func(*Categoria), cambio api.RoomsCategoryPatch) {
	l := sa.P.Lateral
	aplicar := func(f func(*Categoria)) {
		if i := slices.IndexFunc(l.Categorias, func(k Categoria) bool { return k.ID == id }); i >= 0 {
			f(&l.Categorias[i])
			sa.P.Refrescar()
		}
	}
	aplicar(hacer)
	pedir(sa, func(ctx context.Context) (struct{}, error) { return struct{}{}, sa.S.UpdateCategory(ctx, id, cambio) },
		func(_ struct{}, err error) {
			if err != nil {
				aplicar(deshacer)
			}
		})
}

// preguntar abre `usePrompt` con «Guardar»; lo escrito llega recortado.
func (sa *Sala) preguntar(mensaje, valor string, al func(string)) {
	t := sa.P.T
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if lienzo == nil {
		return
	}
	d := ui.Pregunta(mensaje, valor, t.T("ui.cancel", nil), t.T("ui.save", nil), t.T("ui.required", nil),
		func(s string) { al(strings.TrimSpace(s)) })
	d.RotuloCerrar = t.T("action.close", nil)
	sa.pregunta = d
	d.Mostrar(lienzo)
}

// NuevaCategoria es `createNewCategory`: el nombre por la pregunta y, si el servicio la
// crea, entra al final. Un nombre que no valga no añade nada, como en la web.
func (sa *Sala) NuevaCategoria() {
	sa.preguntar(sa.P.T.T("category.newPrompt", nil), "", func(nombre string) {
		if nombre == "" {
			return
		}
		pedir(sa, func(ctx context.Context) (api.RoomsCategory, error) { return sa.S.CreateCategory(ctx, nombre) },
			func(k api.RoomsCategory, err error) {
				if err != nil {
					return
				}
				l := sa.P.Lateral
				l.Categorias = append(l.Categorias, Categoria{ID: k.ID, Nombre: k.Name, Orden: k.SortOrder, Plegada: k.Collapsed})
				sa.P.Refrescar()
			})
	})
}

// MenuCategoria es `categoryMenu`: renombrar, subir, bajar y borrar.
func (sa *Sala) MenuCategoria(k Categoria, disparador fyne.CanvasObject) {
	t := sa.P.T
	borrar := ui.NuevoElementoMenu(t.T("category.delete", nil), iconos.Ninguno, func() { sa.PedirBorrarCategoria(k) })
	borrar.Peligro = true
	m := ui.NuevoMenu(
		ui.NuevoElementoMenu(t.T("category.rename", nil), iconos.Ninguno, func() { sa.RenombrarCategoria(k) }),
		ui.NuevoElementoMenu(t.T("category.moveUp", nil), iconos.Ninguno, func() { sa.MoverCategoria(k, -1) }),
		ui.NuevoElementoMenu(t.T("category.moveDown", nil), iconos.Ninguno, func() { sa.MoverCategoria(k, 1) }),
		ui.NuevoSeparadorMenu(), borrar)
	m.Rotulo = t.T("category.actions", i18n.Vars{"name": k.Nombre})
	sa.menuCat = m
	m.Mostrar(disparador)
}

// RenombrarCategoria es `renameCategory`.
func (sa *Sala) RenombrarCategoria(k Categoria) {
	sa.preguntar(sa.P.T.T("category.renamePrompt", nil), k.Nombre, func(nombre string) {
		if nombre == "" || nombre == k.Nombre {
			return
		}
		sa.cambiarCategoria(k.ID, func(x *Categoria) { x.Nombre = nombre }, func(x *Categoria) { x.Nombre = k.Nombre },
			api.RoomsCategoryPatch{Name: &nombre})
	})
}

// categoriasEnOrden son las categorías por su orden y, a igualdad, por nombre.
func categoriasEnOrden(ks []Categoria) []Categoria {
	orden := slices.Clone(ks)
	slices.SortStableFunc(orden, func(a, b Categoria) int {
		return cmp.Or(cmp.Compare(a.Orden, b.Orden), strings.Compare(a.Nombre, b.Nombre))
	})
	return orden
}

// MoverCategoria es `moveCategory`, el camino de teclado de reordenar: la pone en el
// puesto de la de al lado.
func (sa *Sala) MoverCategoria(k Categoria, paso int) {
	orden := categoriasEnOrden(sa.P.Lateral.Categorias)
	j := slices.IndexFunc(orden, func(x Categoria) bool { return x.ID == k.ID }) + paso
	if j < 0 || j >= len(orden) || j-paso < 0 {
		return
	}
	sa.ReordenarCategoria(k.ID, orden[j].ID)
}

// ReordenarCategoria es `reorderCategoryBefore`, lo que hace soltar una categoría sobre
// otra: la saca de su puesto, la mete en el de la de destino y renumera todas desde
// cero, guardando solo las que cambian. Si una falla, vuelve el orden de antes.
func (sa *Sala) ReordenarCategoria(arrastrada, destino string) {
	l := sa.P.Lateral
	orden := categoriasEnOrden(l.Categorias)
	i := slices.IndexFunc(orden, func(x Categoria) bool { return x.ID == arrastrada })
	j := slices.IndexFunc(orden, func(x Categoria) bool { return x.ID == destino })
	if i < 0 || j < 0 || i == j {
		return
	}
	k := orden[i]
	orden = slices.Insert(slices.Delete(orden, i, i+1), j, k)
	antes := slices.Clone(l.Categorias)
	type cambio struct {
		id    string
		orden int
	}
	var cambios []cambio
	for n := range orden {
		if orden[n].Orden != n {
			orden[n].Orden = n
			cambios = append(cambios, cambio{orden[n].ID, n})
		}
	}
	l.Categorias = orden
	sa.P.Refrescar()
	pedir(sa, func(ctx context.Context) (struct{}, error) {
		for _, c := range cambios {
			if err := sa.S.UpdateCategory(ctx, c.id, api.RoomsCategoryPatch{SortOrder: &c.orden}); err != nil {
				return struct{}{}, err
			}
		}
		return struct{}{}, nil
	}, func(_ struct{}, err error) {
		if err != nil {
			l.Categorias = antes
			sa.P.Refrescar()
		}
	})
}

// PedirBorrarCategoria es el `confirmar` de `removeCategory`.
func (sa *Sala) PedirBorrarCategoria(k Categoria) {
	t := sa.P.T
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if lienzo == nil {
		return
	}
	var d *ui.Dialogo
	cuerpo := ui.NuevoParrafo(t.T("category.confirmDelete", i18n.Vars{"name": k.Nombre}), tema.TextoSM, ui.Normal, "text",
		tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	d = ui.Confirmacion(t.T("category.delete", nil), t.T("action.cancel", nil), t.T("category.delete", nil), true,
		cuerpo, func() { d.Cerrar() }, func() {
			d.Cerrar()
			sa.BorrarCategoria(k)
		})
	d.RotuloCerrar = t.T("action.close", nil)
	sa.pregunta = d
	d.Mostrar(lienzo)
}

// BorrarCategoria es `removeCategory`: sale ya y sus canales vuelven a su sección; si
// el servicio dice que no, vuelve todo.
func (sa *Sala) BorrarCategoria(k Categoria) {
	l := sa.P.Lateral
	cats, canales := slices.Clone(l.Categorias), slices.Clone(sa.canales)
	l.Categorias = slices.DeleteFunc(l.Categorias, func(x Categoria) bool { return x.ID == k.ID })
	for i := range sa.canales {
		if sa.canales[i].CategoryID == k.ID {
			sa.canales[i].CategoryID = ""
		}
	}
	sa.pintarCanales()
	sa.P.Refrescar()
	pedir(sa, func(ctx context.Context) (struct{}, error) { return struct{}{}, sa.S.DeleteCategory(ctx, k.ID) },
		func(_ struct{}, err error) {
			if err != nil {
				l.Categorias, sa.canales = cats, canales
				sa.pintarCanales()
				sa.P.Refrescar()
			}
		})
}

// SoltarCanal es `dropChannelOnGroup`: soltado en una categoría, va a ella; en una
// sección por defecto, vuelve a la suya, y en «Favoritos» además se marca como tal.
func (sa *Sala) SoltarCanal(c Canal, s Seccion) {
	if s.Categoria != nil {
		sa.MoverA(c, s.Categoria.ID)
		return
	}
	sa.MoverA(c, "")
	if s.Clave == ClaveFavoritos && !c.Favorito {
		sa.Favorito(c)
	}
}

// PonerAviso es `setNotifyLevel`: «nada» es también silenciarla, y la fila se atenúa.
func (sa *Sala) PonerAviso(c Canal, n Aviso) {
	if c.Nivel == n {
		return
	}
	poner := func(a Aviso) func(*api.RoomsChannel) {
		return func(x *api.RoomsChannel) { x.NotifyLevel, x.Muted = nivelesAviso[a].api, a == AvisarNada }
	}
	sa.cambiarCanal(c.ID, poner(n), poner(c.Nivel),
		func(ctx context.Context) error { return sa.S.SetNotifyLevel(ctx, c.ID, nivelesAviso[n].api) })
}

// MoverA es `moveChannelToCategory`: "" la devuelve a su sección por defecto.
func (sa *Sala) MoverA(c Canal, categoria string) {
	if c.CategoriaID == categoria {
		return
	}
	sa.cambiarCanal(c.ID, func(x *api.RoomsChannel) { x.CategoryID = categoria },
		func(x *api.RoomsChannel) { x.CategoryID = c.CategoriaID },
		func(ctx context.Context) error { return sa.S.SetChannelCategory(ctx, c.ID, categoria) })
}

// PedirArchivar es el `ConfirmDialog` de archivar: destructivo para quien usa el canal,
// y dice cuál.
func (sa *Sala) PedirArchivar(c Canal) {
	t := sa.P.T
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if lienzo == nil {
		return
	}
	var d *ui.Dialogo
	cuerpo := ui.NuevoParrafo(t.T("channel.confirmArchive", i18n.Vars{"name": c.Nombre}), tema.TextoSM, ui.Normal, "text",
		tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	d = ui.Confirmacion(t.T("channel.archive", nil), t.T("action.cancel", nil), t.T("channel.archive", nil), true,
		cuerpo, func() { d.Cerrar() }, func() {
			d.Cerrar()
			sa.Archivar(c)
		})
	d.RotuloCerrar = t.T("action.close", nil)
	d.Mostrar(lienzo)
}

// Archivar es `archiveChannel`: sale del carril ya y vuelve si el servicio dice que no.
// Si era el abierto se abre el primero que quede.
//
// ponytail: la web deja la vista sin canal; aquí no hay vista sin canal con la lista
// llena, y abrir el primero es lo que hace la carga inicial.
func (sa *Sala) Archivar(c Canal) {
	antes := slices.Clone(sa.canales)
	sa.canales = slices.DeleteFunc(sa.canales, func(x api.RoomsChannel) bool { return x.ID == c.ID })
	sa.pintarCanales()
	if c.ID == sa.abierto && len(sa.P.Lateral.Canales) > 0 {
		sa.Elegir(sa.P.Lateral.Canales[0])
	}
	sa.P.Refrescar()
	pedir(sa, func(ctx context.Context) (struct{}, error) { return struct{}{}, sa.S.ArchiveChannel(ctx, c.ID) },
		func(_ struct{}, err error) {
			if err != nil {
				sa.canales = antes
				sa.pintarCanales()
				sa.P.Refrescar()
			}
		})
}

// PedirSalir es «Salir del canal» de los menús: lo mismo que /leave, tras confirmarlo.
func (sa *Sala) PedirSalir(c Canal) {
	t := sa.P.T
	sa.confirmar(t.T("channel.confirmLeave", i18n.Vars{"name": c.Nombre}), t.T("channel.leave", nil), true,
		func() { sa.SalirDe(c.ID) })
}

// puedeBorrar dice si se ofrece «Borrar canal»: a quien administra el equipo (no en
// una conversación de otra organización) o el canal —eso solo se sabe del abierto,
// por su roster—. Un directo nunca: lo suyo es «Borrar chat». La verja de verdad es
// la del servicio.
func (sa *Sala) puedeBorrar(c Canal) bool {
	if c.Clase == Directo {
		return false
	}
	if sa.adminEquipo && !c.Externo {
		return true
	}
	return c.ID == sa.abierto && slices.ContainsFunc(sa.roster, func(m Miembro) bool { return m.ID == sa.Yo && m.Admin })
}

// PedirBorrarCanal confirma «Borrar canal» (para todos) o, en un directo, «Borrar
// chat» (solo para ti), y lo hace.
func (sa *Sala) PedirBorrarCanal(c Canal) {
	t := sa.P.T
	mensaje, accion := t.T("channel.confirmDelete", i18n.Vars{"name": c.Nombre}), t.T("channel.delete", nil)
	if c.Clase == Directo {
		mensaje, accion = t.T("dm.confirmDelete", i18n.Vars{"name": c.Nombre}), t.T("dm.delete", nil)
	}
	sa.confirmar(mensaje, accion, true, func() { sa.BorrarCanal(c) })
}

// BorrarCanal borra el canal o, en un directo, lo oculta solo para ti: sale del carril
// ya y vuelve, con su aviso, si el servicio dice que no.
func (sa *Sala) BorrarCanal(c Canal) {
	directo := c.Clase == Directo
	deshacer := sa.quitarCanal(c.ID)
	pedir(sa, func(ctx context.Context) (struct{}, error) {
		if directo {
			return struct{}{}, sa.S.HideDirect(ctx, c.ID)
		}
		return struct{}{}, sa.S.DeleteChannel(ctx, c.ID)
	}, func(_ struct{}, err error) {
		if err == nil {
			sa.olvidarCanal(c.ID)
			return
		}
		deshacer()
		clave := "channel.deleteFailed"
		if directo {
			clave = "dm.deleteFailed"
		}
		sa.P.Avisar(ui.AvisoPeligro, sa.P.T.T(clave, nil))
	})
}

// ---- Mensajes: el efecto de `activeId` ----

// Elegir abre un canal: carga su final, marca el separador de no leídos con el
// contador que tenía AL ABRIRLO y lo da por leído.
func (sa *Sala) Elegir(c Canal) {
	cuantos := sa.pend[c.ID].Unread
	sa.abrir(c)
	delete(sa.pend, c.ID)
	sa.pintarCanales()
	sa.P.Refrescar()
	sa.cargarMensajes(c.ID, cuantos)
	sa.darPorLeido(c.ID)
}

// abrir es lo común a elegir un canal y a saltar a un mensaje suyo.
//
// El panel de la derecha se queda abierto al cambiar de canal, como en la web (el
// estado del panel no depende del canal); el de fijados se rehace con los del nuevo.
func (sa *Sala) abrir(c Canal) {
	cambia := c.ID != sa.abierto
	if cambia {
		sa.cancelarAdjuntos()
	}
	sa.abierto, sa.noLeidoAbierto, sa.viendoHistorial = c.ID, false, false
	if cambia && sa.panel == PanelFijados && sa.lista != nil {
		sa.lista.Subtitulo = "#" + c.Nombre
		defer sa.cargarLista()
	}
	sa.P.Canal.ViendoHistorial = false
	sa.P.Canal.Escribiendo = sa.nombresEscribiendo()
	sa.P.Canal.Historial.EditandoID = ""
	// `isTeamAdmin && !active.external`: el tema lo edita quien administra el equipo, y
	// no en una conversación de otra organización.
	cab := sa.P.Canal.Cabecera
	cab.PuedeEditarTema, cab.EditandoTema = sa.adminEquipo && c.Clase != Directo && !c.Externo, false
	sa.P.Abrir(c)
	sa.cargarFijados(c.ID)
	if cambia {
		sa.cargarRoster(c)
		sa.restaurar(ambito{canal: c.ID}, sa.P.Canal.Compositor)
	}
}

func (sa *Sala) darPorLeido(canal string) {
	pedir(sa, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, sa.S.MarkRead(ctx, canal)
	}, func(struct{}, error) {})
}

func (sa *Sala) cargarMensajes(canal string, noLeidos int) {
	if canal == "" {
		return
	}
	sa.vista++
	vista := sa.vista
	h := sa.P.Canal.Historial
	h.Estado, h.Mensajes, h.IndiceNoLeidos, h.HayAnteriores = Cargando, nil, -1, false
	sa.separador = ""
	h.Refresh()
	pedir(sa, func(ctx context.Context) ([]api.RoomsMessage, error) {
		return sa.S.Messages(ctx, canal, time.Time{}, PorPagina)
	}, func(ms []api.RoomsMessage, err error) {
		if vista != sa.vista {
			return
		}
		if err != nil {
			h.Estado = estadoDe(err)
			h.Refresh()
			return
		}
		sa.mensajes = ms
		sa.pintarMensajes()
		h.HayAnteriores = len(ms) >= PorPagina // heurística de la web: página llena, hay más
		// El separador se ancla al MENSAJE (`unreadDividerId`), no a su posición: lo que
		// entre arriba o abajo después no lo mueve.
		sa.separador = ""
		if i := IndiceSeparadorNoLeidos(noLeidos, len(sa.mensajes)); i >= 0 {
			sa.separador = sa.mensajes[i].ID
		}
		sa.pintarMensajes()
		h.Refresh()
	})
}

// RecargarMensajes recarga los mensajes del canal abierto (p. ej. al verificar cifrado).
func (sa *Sala) RecargarMensajes() {
	sa.recargarMensajes()
}

// recargarMensajes trae otra vez el final del canal abierto SIN pasar por el
// esqueleto: es lo que pide una trama del tiempo real.
//
// ponytail: la web mete el mensaje de la trama en la lista; aquí se pide la página
// entera (una GET de 50 por trama). Se cambia por el `payload` de la trama cuando el
// volumen de un canal lo note.
func (sa *Sala) recargarMensajes() {
	canal, vista := sa.abierto, sa.vista
	if canal == "" || sa.viendoHistorial {
		return // viendo historial, el final no se trae solo: para eso está la barra
	}
	pedir(sa, func(ctx context.Context) ([]api.RoomsMessage, error) {
		return sa.S.Messages(ctx, canal, time.Time{}, PorPagina)
	}, func(ms []api.RoomsMessage, err error) {
		if err != nil || vista != sa.vista {
			return
		}
		sa.mensajes = unirPaginas(sa.mensajes, ms)
		sa.pintarMensajes()
		sa.P.Canal.Historial.Refresh()
	})
}

// unirPaginas pone la página del final (`nuevos`) sobre lo que ya había: se conserva lo
// más viejo que la página, que se cargó hacia atrás, y lo demás lo manda la página
// (lo editado, lo borrado, lo nuevo).
func unirPaginas(antes, nuevos []api.RoomsMessage) []api.RoomsMessage {
	if len(nuevos) == 0 {
		return nuevos
	}
	desde := nuevos[0].CreatedAt
	out := make([]api.RoomsMessage, 0, len(antes)+len(nuevos))
	for _, m := range antes {
		if m.CreatedAt.Before(desde) {
			out = append(out, m)
		}
	}
	return append(out, nuevos...)
}

// CargarAnteriores es `loadOlder`: pide la página de antes del mensaje más viejo y la
// pone arriba sin repetir ninguno. Menos de una página es que ya no hay más; un fallo
// se reintenta solo al volver a subir.
func (sa *Sala) CargarAnteriores() {
	h := sa.P.Canal.Historial
	if sa.abierto == "" || len(sa.mensajes) == 0 || !h.HayAnteriores || h.CargandoAnteriores {
		return
	}
	canal, vista, antes := sa.abierto, sa.vista, sa.mensajes[0].CreatedAt
	h.CargandoAnteriores = true
	h.Refresh()
	pedir(sa, func(ctx context.Context) ([]api.RoomsMessage, error) {
		return sa.S.Messages(ctx, canal, antes, PorPagina)
	}, func(ms []api.RoomsMessage, err error) {
		if vista != sa.vista {
			return
		}
		h.CargandoAnteriores = false
		if err == nil {
			h.HayAnteriores = len(ms) >= PorPagina
			vistos := map[string]bool{}
			for _, m := range sa.mensajes {
				vistos[m.ID] = true
			}
			nuevos := slices.DeleteFunc(ms, func(m api.RoomsMessage) bool { return vistos[m.ID] })
			sa.mensajes = append(nuevos, sa.mensajes...)
			sa.pintarMensajes()
		}
		h.Refresh()
	})
}

func (sa *Sala) pintarMensajes() {
	h := sa.P.Canal.Historial
	h.Mensajes = h.Mensajes[:0]
	h.UltimaLlamadaID = ""
	h.IndiceNoLeidos = -1
	for i, m := range sa.mensajes {
		if m.ID == sa.separador && sa.separador != "" {
			h.IndiceNoLeidos = i
		}
		h.Mensajes = append(h.Mensajes, sa.mensajeDe(m))
		if m.Type == "call" {
			h.UltimaLlamadaID = m.ID
		}
	}
	h.Estado = Listo
	if len(h.Mensajes) == 0 {
		h.Estado = Vacio
	}
}

// mensajeDe es `toChatMessage`. Una tarjeta de llamada con el cuerpo vacío es «inició
// una llamada»; con un entero, «llamada finalizada» y el entero son sus segundos
// (`callEndedSeconds`).
func (sa *Sala) mensajeDe(m api.RoomsMessage) Mensaje {
	nombre := m.AuthorName
	if nombre == "" {
		nombre = m.AuthorSub
	}
	out := Mensaje{
		ID: m.ID, CanalID: m.ChannelID, Autor: Autor{ID: m.AuthorSub, Nombre: nombre, Bot: m.IsBot},
		Cuerpo: m.Body, Creado: m.CreatedAt.Local(), Editado: m.EditedAt != nil,
		Respuestas: m.ReplyCount,
		Siguiendo:  m.ParentID == nil && sa.sigue(m.ID),
	}
	if label := sa.seguridadMensaje(m); label != "" {
		out.Cuerpo = label + "\n" + out.Cuerpo
	}
	switch m.Priority {
	case "important":
		out.Prioridad = Importante
	case "urgent":
		out.Prioridad = Urgente
	}
	out.PideAcuse, out.Acuses, out.Acusado = m.RequiresAck, m.AckCount, m.Acked
	out.Tarjeta = tarjetaDe(m.Card, sa.Yo)
	out.Adjuntos = adjuntosDe(m.Attachments)
	if m.Type == "call" {
		out.Tipo = Llamada
		if s, err := strconv.Atoi(strings.TrimSpace(m.Body)); err == nil && s >= 0 {
			out.Terminada, out.Segundos = true, s
		}
	}
	for _, sub := range m.Mentions {
		nombre := sub // `memberNames?.[s] ?? s`
		if i := slices.IndexFunc(sa.roster, func(x Miembro) bool { return x.ID == sub }); i >= 0 {
			nombre = sa.roster[i].Nombre
		}
		out.Menciones = append(out.Menciones, nombre)
	}
	// De una persona bloqueada: plegado. Se queda quién y cuándo; el contenido no
	// se enseña hasta que se pide («Mostrar», en su menú).
	if out.Tipo != Llamada && sa.plegado(m.AuthorSub, m.ID) {
		out.Cuerpo, out.Menciones, out.Adjuntos = sa.P.T.T("block.folded", nil), nil, nil
		out.Tarjeta = nil
	}
	for _, r := range m.Reactions {
		out.Reacciones = append(out.Reacciones, Reaccion{
			Emoji: r.Emoji, Veces: r.Count, Mia: slices.Contains(r.Subs, sa.Yo),
			Quien: QuienReacciono(sa.P.T, sa.P.Locale, r.Emoji, r.Subs, sa.nombresRoster()),
		})
	}
	return out
}

// Enviar publica lo escrito en el canal abierto con sus @menciones y las opciones de
// la caja (prioridad, acuse), o lo PROGRAMA si la caja tiene hora. La caja se vacía
// YA —como la web, que no espera al servidor para limpiarla— y el mensaje entra en la
// lista cuando el servicio lo confirma; si falla, el texto vuelve a la caja para no
// perderlo.
func (sa *Sala) Enviar(texto string) {
	canal := sa.abierto
	if canal == "" || (strings.TrimSpace(texto) == "" && len(sa.P.Canal.Compositor.Subidos()) == 0) {
		return
	}
	if MencionaATodos(texto) && len(sa.roster) >= UmbralAvisoMasivo {
		sa.confirmarATodos(func() { sa.enviar(canal, texto) })
		return
	}
	sa.enviar(canal, texto)
}

// enviar es `sendBody` ya confirmado.
func (sa *Sala) enviar(canal, texto string) {
	c := sa.P.Canal.Compositor
	o := api.RoomsSend{Mentions: MencionesEn(texto, sa.roster), Priority: prioridadAPI[c.Prioridad], RequiresAck: c.Acuse}
	for _, a := range c.Subidos() {
		o.FileIDs = append(o.FileIDs, a.FileID)
	}
	cuando := c.ProgramarA
	c.Vaciar()
	if !cuando.IsZero() {
		sa.programar(canal, texto, o.FileIDs, cuando)
		return
	}
	pedir(sa, func(ctx context.Context) (api.RoomsMessage, error) {
		return sa.S.PostWith(ctx, canal, texto, o)
	}, func(m api.RoomsMessage, err error) {
		if err != nil {
			if canal == sa.abierto && sa.P.Canal.Compositor.Texto() == "" {
				sa.P.Canal.Compositor.PonerTexto(texto)
			}
			return
		}
		if canal != sa.abierto || slices.ContainsFunc(sa.mensajes, func(x api.RoomsMessage) bool { return x.ID == m.ID }) {
			return
		}
		sa.mensajes = append(sa.mensajes, m)
		sa.pintarMensajes()
		sa.P.Canal.Historial.Refresh()
	})
}

// Reaccionar pone o quita la reacción de quien mira: la suya se conmuta.
func (sa *Sala) Reaccionar(m Mensaje, emoji string) {
	poner := !slices.ContainsFunc(m.Reacciones, func(r Reaccion) bool { return r.Emoji == emoji && r.Mia })
	pedir(sa, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, sa.S.React(ctx, m.ID, emoji, poner)
	}, func(_ struct{}, err error) {
		if err == nil {
			sa.recargarMensajes()
		}
	})
}

// ---- Tiempo real: `connectRealtime` ----

// escuchar (re)abre el WebSocket suscrito a todos los canales conocidos. Cuando se
// cae, espera y vuelve a abrir con respaldo exponencial, como la web.
func (sa *Sala) escuchar() {
	if sa.pararEscucha != nil {
		sa.pararEscucha()
	}
	ids := make([]string, 0, len(sa.canales))
	for _, c := range sa.canales {
		ids = append(ids, c.ID)
	}
	ctx, cancelar := context.WithCancel(sa.ctx)
	var pararEspera func()
	sa.pararEscucha = func() {
		cancelar()
		if pararEspera != nil {
			pararEspera()
		}
	}
	var abrir func()
	abrir = func() {
		sa.Lanzar(func() {
			err := sa.S.Listen(ctx, ids, func(ev api.RoomsEvent) {
				fyne.Do(func() { sa.alLlegar(ev) })
			})
			fyne.Do(func() {
				if ctx.Err() != nil || err == nil {
					return
				}
				sa.espera = min(max(2*sa.espera, ReconectarBase), ReconectarTope)
				pararEspera = sa.Esperar(sa.espera, abrir)
			})
		})
	}
	abrir()
}

// ---- Borradores: lo escrito y no enviado, por canal o por hilo ----

// ambito es de dónde es un borrador: el canal y, si es de un hilo, su raíz.
type ambito struct{ canal, raiz string }

// EsperaBorrador es el `SYNC_DELAY_MS` de `lib/drafts.ts`: una ráfaga de tecleo es UNA
// petición al servidor, no una por pulsación.
const EsperaBorrador = 1500 * time.Millisecond

// alTeclear es lo que hace la caja en cada cambio: el borrador de su ámbito y, en la
// del canal, `onTyping` si no está en blanco.
func (sa *Sala) alTeclear(a ambito, texto string) {
	if sa.restaurando {
		return
	}
	if a.raiz == "" && strings.TrimSpace(texto) != "" {
		sa.Escribiendo()
	}
	sa.guardarBorrador(a, texto)
}

// guardarBorrador apunta lo escrito —en blanco lo quita—, pone al día la cuenta del
// carril y lo sube al servidor tras la espera. Un fallo no importa: la copia de aquí
// sigue y el siguiente guardado reintenta.
func (sa *Sala) guardarBorrador(a ambito, texto string) {
	if a.canal == "" {
		return
	}
	antes, habia := sa.borradores[a]
	if strings.TrimSpace(texto) == "" {
		if !habia {
			return
		}
		delete(sa.borradores, a)
	} else {
		if antes == texto {
			return
		}
		sa.borradores[a] = texto
	}
	sa.contarBorradores()
	if parar := sa.subidas[a]; parar != nil {
		parar()
	}
	sa.subidas[a] = sa.Esperar(EsperaBorrador, func() {
		delete(sa.subidas, a)
		sa.Lanzar(func() { _ = sa.S.PutDraft(sa.ctx, a.canal, a.raiz, texto) })
	})
}

// restaurar pone en la caja el borrador de su ámbito, o la vacía, sin que cuente como
// escribir.
func (sa *Sala) restaurar(a ambito, c *Compositor) {
	sa.restaurando = true
	defer func() { sa.restaurando = false }()
	// Las opciones de envío son de la caja de ESTE canal: la web remonta el compositor
	// al cambiar de ámbito y se van con él.
	c.QuitarOpciones()
	if texto := sa.borradores[a]; texto != "" {
		c.PonerTexto(texto)
		return
	}
	c.Vaciar()
}

// cargarBorradores trae los del servidor al arrancar: es lo que hace que lo empezado
// en otro dispositivo siga ahí. Lo que ya se haya escrito aquí manda.
func (sa *Sala) cargarBorradores() {
	pedir(sa, sa.S.Drafts, func(ds []api.RoomsDraft, err error) {
		if err != nil {
			return
		}
		for _, d := range ds {
			a := ambito{canal: d.ChannelID}
			if d.ParentID != nil {
				a.raiz = *d.ParentID
			}
			if _, hay := sa.borradores[a]; !hay && strings.TrimSpace(d.Body) != "" {
				sa.borradores[a] = d.Body
			}
		}
		sa.contarBorradores()
		if c := sa.P.Canal.Compositor; c.Texto() == "" {
			sa.restaurar(ambito{canal: sa.abierto}, c)
		}
	})
}

// contarBorradores lleva la cuenta al carril —la fila «Borradores» sale con uno— y
// repinta el panel si está abierto.
func (sa *Sala) contarBorradores() {
	if l := sa.P.Lateral; l.Borradores != len(sa.borradores) {
		l.Borradores = len(sa.borradores)
		sa.P.Refrescar()
	}
	if sa.panel == PanelBorradores && sa.lista != nil {
		sa.lista.Borradores = sa.listaBorradores()
		sa.lista.Refresh()
	}
}

// listaBorradores son los borradores para el panel, del canal por nombre y los de un
// hilo detrás de los del canal.
func (sa *Sala) listaBorradores() []Borrador {
	out := make([]Borrador, 0, len(sa.borradores))
	for a, texto := range sa.borradores {
		out = append(out, Borrador{Canal: a.canal, Raiz: a.raiz, Texto: texto})
	}
	nombres := sa.nombresDeCanal()
	slices.SortFunc(out, func(x, y Borrador) int {
		return cmp.Or(strings.Compare(nombres[x.Canal], nombres[y.Canal]), strings.Compare(x.Canal, y.Canal),
			strings.Compare(x.Raiz, y.Raiz))
	})
	return out
}

// IrABorrador es `goToDraft`: al canal y, si era de un hilo, al hilo, donde la caja
// ya trae el texto.
//
// ponytail: del borrador de un hilo solo se sabe la raíz; si no está en la ventana
// cargada del canal se pide el hilo, que es una lectura más, como en la web.
func (sa *Sala) IrABorrador(b Borrador) {
	sa.cerrarDerecho()
	if b.Canal != sa.abierto {
		if i := slices.IndexFunc(sa.P.Lateral.Canales, func(c Canal) bool { return c.ID == b.Canal }); i >= 0 {
			sa.Elegir(sa.P.Lateral.Canales[i])
		}
	}
	if b.Raiz == "" {
		return
	}
	if i := slices.IndexFunc(sa.P.Canal.Historial.Mensajes, func(m Mensaje) bool { return m.ID == b.Raiz }); i >= 0 {
		sa.AbrirHilo(sa.P.Canal.Historial.Mensajes[i])
		return
	}
	pedir(sa, func(ctx context.Context) (api.RoomsThread, error) { return sa.S.Thread(ctx, b.Raiz) },
		func(h api.RoomsThread, err error) {
			if err != nil {
				sa.P.Avisar(ui.AvisoPeligro, sa.P.T.T("thread.loadFailed", nil))
				return
			}
			sa.AbrirHilo(sa.mensajeDe(h.Root))
		})
}

// DescartarBorrador es `discardDraft`: fuera de aquí y del servidor; si era el de la
// caja abierta, la caja se vacía.
func (sa *Sala) DescartarBorrador(b Borrador) {
	a := ambito{b.Canal, b.Raiz}
	sa.guardarBorrador(a, "")
	switch {
	case b.Raiz == "" && b.Canal == sa.abierto:
		sa.restaurar(a, sa.P.Canal.Compositor)
	case b.Raiz != "" && b.Raiz == sa.raizHilo && sa.hilo != nil:
		sa.restaurar(a, sa.hilo.Compositor)
	}
}

// EsperaEscribiendo es el `3000` de `onTyping`: como mucho un aviso cada 3 s.
const EsperaEscribiendo = 3 * time.Second

// Escribiendo es `onTyping`: avisa a los demás de que se escribe en el canal abierto,
// una vez cada 3 s como mucho. Es efímero, y un fallo no importa.
func (sa *Sala) Escribiendo() {
	ahora := sa.ahora()
	if sa.abierto == "" || ahora.Sub(sa.avisoEscribiendo) < EsperaEscribiendo {
		return
	}
	sa.avisoEscribiendo = ahora
	canal := sa.abierto
	sa.Lanzar(func() { _ = sa.S.Typing(sa.ctx, canal) })
}

// alLlegar reparte una trama: lo del canal abierto recarga su final; lo de otro
// canal, los contadores; y «escribiendo» apunta el nombre durante 5 s.
func (sa *Sala) alLlegar(ev api.RoomsEvent) {
	sa.chatDeLaLlamada(ev)
	switch ev.Type {
	case api.RoomsReady:
		sa.espera = 0
	case "message", "message_edited", "message_deleted", "reaction", "call_started", "call_ended":
		if ev.Type == "message" {
			sa.subirSeguido(ev.Payload)
		}
		if sa.raizHilo != "" {
			sa.cargarHilo(false)
		}
		if ev.ChannelID == sa.abierto {
			sa.recargarMensajes()
			if ev.Type == "message" {
				sa.borrarEscribiendo(ev.ChannelID, ev.Subject)
			}
			return
		}
		if ev.Type == "message" {
			sa.refrescarPendientes()
		}
	case "channel_member_removed":
		sa.alQuitarMiembro(ev)
	case "channel_deleted":
		// Lo ha borrado quien lo administra: fuera del carril y sin texto descifrado.
		sa.quitarCanal(ev.ChannelID)
		sa.olvidarCanal(ev.ChannelID)
	case "typing":
		if ev.Subject == "" || ev.Subject == sa.Yo {
			return
		}
		nombre := ev.Name
		if nombre == "" {
			nombre = ev.Subject
		}
		if sa.escribiendo[ev.ChannelID] == nil {
			sa.escribiendo[ev.ChannelID] = map[string]string{}
		}
		sa.escribiendo[ev.ChannelID][ev.Subject] = nombre
		clave := ev.ChannelID + ":" + ev.Subject
		if parar := sa.caducan[clave]; parar != nil {
			parar()
		}
		canal, sub := ev.ChannelID, ev.Subject
		sa.caducan[clave] = sa.Esperar(VidaEscribiendo, func() { sa.borrarEscribiendo(canal, sub) })
		sa.pintarEscribiendo()
	}
}

// alQuitarMiembro cierra la conversación si la baja afecta a esta persona.
func (sa *Sala) alQuitarMiembro(ev api.RoomsEvent) {
	var member struct {
		Subject string `json:"subject"`
		Channel string `json:"channel_id"`
	}
	if json.Unmarshal(ev.Payload, &member) != nil || member.Subject == "" {
		return
	}
	if member.Subject == sa.Yo {
		channel := ev.ChannelID
		if member.Channel != "" {
			channel = member.Channel
		}
		if channel != "" {
			sa.quitarCanal(channel)
			sa.olvidarCanal(channel)
		}
	} else if ev.ChannelID == sa.abierto {
		sa.roster = slices.DeleteFunc(sa.roster, func(m Miembro) bool { return m.ID == member.Subject })
		sa.P.Canal.Cabecera.Miembros = len(sa.roster)
		sa.P.Canal.Cabecera.Refresh()
	}
}

func (sa *Sala) borrarEscribiendo(canal, sub string) {
	clave := canal + ":" + sub
	if parar := sa.caducan[clave]; parar != nil {
		parar()
		delete(sa.caducan, clave)
	}
	if _, hay := sa.escribiendo[canal][sub]; !hay {
		return
	}
	delete(sa.escribiendo[canal], sub)
	sa.pintarEscribiendo()
}

func (sa *Sala) pintarEscribiendo() {
	sa.P.Canal.Escribiendo = sa.nombresEscribiendo()
	sa.P.Canal.Refresh()
}

// nombresEscribiendo son los del canal abierto, en un orden estable.
func (sa *Sala) nombresEscribiendo() []string {
	out := make([]string, 0, len(sa.escribiendo[sa.abierto]))
	for _, n := range sa.escribiendo[sa.abierto] {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// ---- El hilo: `openThread`, `handleSendReply` y `toggleFollow` ----

// AbrirHilo abre el panel con la raíz y el esqueleto, y lo rellena con lo que conteste
// el servicio. El hilo y los demás paneles de la derecha se excluyen.
func (sa *Sala) AbrirHilo(m Mensaje) {
	if m.Enviando || m.Fallido {
		return // solo mensajes que ya están en el servidor
	}
	if sa.panel != "" {
		sa.lista, sa.panel, sa.miembros = nil, "", nil
		sa.P.Canal.Cabecera.Panel = ""
		sa.P.Canal.Cabecera.Refresh()
		sa.soltarHilos()
	}
	sa.verSeguido(m.ID)
	if sa.hilo == nil {
		h := NuevoPanelHilo(sa.P.T, sa.P.Locale)
		h.AlCerrar = sa.CerrarHilo
		h.AlReintentar = func() { sa.cargarHilo(true) }
		h.AlSeguir = sa.SeguirHilo
		h.Compositor.AlEnviar = sa.Responder
		h.Compositor.AlCambiarTexto = func(s string) { sa.alTeclear(ambito{sa.canalHilo, sa.raizHilo}, s) }
		h.Compositor.AlEmoji = func() { sa.AbrirEmojis(h.Compositor.BotonEmoji(), h.Compositor.Insertar) }
		h.Compositor.Remotos = sa.P.Canal.Compositor.Remotos
		h.Compositor.FormatoVisible = sa.P.Canal.Compositor.FormatoVisible
		h.Compositor.AlFormato = sa.guardarFormato
		sa.hilo = h
	}
	h := sa.hilo
	h.Compositor.Miembros = sa.roster
	sa.raizHilo, sa.canalHilo = m.ID, cmp.Or(m.CanalID, sa.abierto)
	sa.restaurar(ambito{sa.canalHilo, m.ID}, h.Compositor)
	h.Raiz, h.Respuestas, h.Siguiendo = m, nil, false
	h.NombreCanal = sa.P.Canal.Cabecera.Canal.Nombre
	if n := sa.nombresDeCanal()[m.CanalID]; n != "" {
		h.NombreCanal = n // uno de «Hilos» puede ser de otro canal
	}
	sa.P.PonerDerecho(h)
	sa.cargarHilo(true)
}

// cargarHilo pide el hilo abierto. Con `esqueleto`, el panel enseña la carga mientras
// tanto; sin él —una trama del tiempo real— se queda como está hasta que llegue.
func (sa *Sala) cargarHilo(esqueleto bool) {
	raiz, h := sa.raizHilo, sa.hilo
	if raiz == "" {
		return
	}
	sa.vistaHilo++
	vista := sa.vistaHilo
	if esqueleto {
		h.Estado = Cargando
		h.Refresh()
	}
	pedir(sa, func(ctx context.Context) (api.RoomsThread, error) {
		return sa.S.Thread(ctx, raiz)
	}, func(hilo api.RoomsThread, err error) {
		if vista != sa.vistaHilo || raiz != sa.raizHilo {
			return
		}
		if err != nil {
			if esqueleto {
				h.Estado = Error
				h.Refresh()
			}
			return
		}
		cambia := sa.sigue(hilo.Root.ID) != hilo.Following
		sa.seguir[hilo.Root.ID] = hilo.Following // el dato del servidor, para el canal
		if cambia {
			sa.repintarSiguiendo()
		}
		h.Raiz, h.Siguiendo, h.Estado = sa.mensajeDe(hilo.Root), hilo.Following, Listo
		h.Respuestas = h.Respuestas[:0]
		for _, r := range hilo.Replies {
			h.Respuestas = append(h.Respuestas, sa.mensajeDe(r))
		}
		h.Refresh()
	})
}

// CerrarHilo es `closeThread`.
func (sa *Sala) CerrarHilo() {
	sa.raizHilo = ""
	sa.vistaHilo++
	sa.P.PonerDerecho(nil)
}

// Responder publica en el hilo abierto. Como en el canal, la caja se vacía al momento
// y el texto vuelve si el envío falla. La cuenta de «N respuestas» del canal se pone
// al día recargando su final.
func (sa *Sala) Responder(texto string) {
	raiz := sa.raizHilo
	if raiz == "" || strings.TrimSpace(texto) == "" {
		return
	}
	if MencionaATodos(texto) && len(sa.roster) >= UmbralAvisoMasivo {
		sa.confirmarATodos(func() { sa.responder(raiz, texto) })
		return
	}
	sa.responder(raiz, texto)
}

// responder es la respuesta ya confirmada.
func (sa *Sala) responder(raiz, texto string) {
	h := sa.hilo
	h.Compositor.Vaciar()
	o := api.RoomsSend{Mentions: MencionesEn(texto, sa.roster)}
	pedir(sa, func(ctx context.Context) (api.RoomsMessage, error) {
		return sa.S.ReplyWith(ctx, raiz, texto, o)
	}, func(m api.RoomsMessage, err error) {
		if err != nil {
			if raiz == sa.raizHilo && h.Compositor.Texto() == "" {
				h.Compositor.PonerTexto(texto)
			}
			return
		}
		if raiz != sa.raizHilo {
			return
		}
		if !slices.ContainsFunc(h.Respuestas, func(x Mensaje) bool { return x.ID == m.ID }) {
			h.Respuestas = append(h.Respuestas, sa.mensajeDe(m))
			h.Raiz.Respuestas = max(h.Raiz.Respuestas, len(h.Respuestas))
			h.Refresh()
		}
		sa.recargarMensajes()
	})
}

// SeguirHilo conmuta «Seguir»: se pinta ya y se deshace si el servicio dice que no.
func (sa *Sala) SeguirHilo() {
	if sa.raizHilo != "" {
		sa.AlternarSeguir(sa.raizHilo, sa.hilo.Siguiendo)
	}
}

// sigue dice si se sigue el hilo de un mensaje, de lo que se sabe: lo confirmado en
// esta sesión y, si no, la página cargada de «Hilos».
//
// ponytail: el mensaje del canal no trae `following`, así que un hilo seguido más
// viejo que esa página no enseña «Siguiendo» hasta que se abre. Como en la web.
func (sa *Sala) sigue(id string) bool {
	if v, ok := sa.seguir[id]; ok {
		return v
	}
	return slices.ContainsFunc(sa.seguidos, func(h api.RoomsThreadSummary) bool { return h.Root.ID == id })
}

// AlternarSeguir es `toggleFollow`: se apunta ya —en el canal y en el hilo si es el
// abierto—, se pide, y «Hilos» se recarga; si el servicio dice que no, se deshace.
func (sa *Sala) AlternarSeguir(id string, siguiendo bool) {
	poner := func(v bool) {
		sa.seguir[id] = v
		if sa.raizHilo == id && sa.hilo != nil {
			sa.hilo.Siguiendo = v
			sa.hilo.Refresh()
		}
		sa.repintarSiguiendo()
	}
	poner(!siguiendo)
	pedir(sa, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, sa.S.Follow(ctx, id, !siguiendo)
	}, func(_ struct{}, err error) {
		if err != nil {
			poner(siguiendo)
			return
		}
		sa.cargarSeguidos()
	})
}

// ---- Fijados y guardados ----

// cargarFijados pide los fijados del canal abierto: el número de la chincheta de la
// cabecera y el «Fijar/Desfijar» del menú. Degrada a ninguno si falla, como la web.
func (sa *Sala) cargarFijados(canal string) {
	pedir(sa, func(ctx context.Context) ([]api.RoomsMessage, error) {
		return sa.S.Pins(ctx, canal)
	}, func(ms []api.RoomsMessage, err error) {
		if err != nil || canal != sa.abierto {
			return
		}
		clear(sa.fijados)
		for _, m := range ms {
			sa.fijados[m.ID] = true
		}
		sa.P.Canal.Cabecera.Fijados = len(ms)
		sa.P.Canal.Cabecera.Refresh()
	})
}

// cargarGuardados pide los guardados de quien mira, para el «Guardar/Quitar» del menú.
//
// ponytail: solo la primera página (el servicio pagina con `next_before`); un mensaje
// guardado hace muchas páginas saldría como «Guardar». Se pagina cuando exista el
// panel de guardados, que es quien lo necesita entero.
func (sa *Sala) cargarGuardados() {
	pedir(sa, sa.S.Saved, func(ms []api.RoomsMessage, err error) {
		if err != nil {
			return
		}
		clear(sa.guardados)
		for _, m := range ms {
			sa.guardados[m.ID] = true
		}
	})
}

// ---- El «…» de un mensaje: `menuItems` de MessageView.tsx ----

// menuDe son las entradas del menú de un mensaje, en el orden de la web: fijar,
// guardar, seguir, copiar el texto, copiar el enlace, marcar como no leído y, en los
// propios, editar y borrar tras un separador. Solo lo llevan los mensajes del canal,
// que son raíces: las respuestas del hilo no tienen «…».
//
// Tras copiar el enlace van «Guardar en Notepads» y «Reenviar…»; y «Visto por», SOLO en
// los propios: saber quién ha leído lo que escribió otro no ayuda a trabajar y sí
// convierte el chat en un panel de control sobre los compañeros.
func (sa *Sala) menuDe(m Mensaje) []fyne.CanvasObject {
	t := sa.P.T
	fijado, guardado := sa.fijados[m.ID], sa.guardados[m.ID]
	fijar, guardar := t.T("message.pin", nil), t.T("action.save", nil)
	if fijado {
		fijar = t.T("message.unpin", nil)
	}
	if guardado {
		guardar = t.T("message.unsave", nil)
	}
	// «Seguir mensaje», solo en los RAÍZ: el servicio no deja seguir una respuesta, y en
	// el canal solo hay raíces.
	seguir := t.T("message.follow", nil)
	if m.Siguiendo {
		seguir = t.T("thread.unfollowShort", nil)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// Cada entrada con su glifo de `glyphs.tsx`, en `text-muted` como el `MenuItem`.
	con := func(rotulo, cuerpo string, al func()) *ui.ElementoMenu {
		it := ui.NuevoElementoMenu(rotulo, iconos.Ninguno, al)
		it.Glifo = dibujoRelleno(cuerpo, iconos.TallaSM, "text-muted", v)
		return it
	}
	out := make([]fyne.CanvasObject, 0, 12)
	out = append(out,
		con(fijar, glifoChincheta, func() { sa.Fijar(m, !fijado) }),
		con(guardar, glifoMarcador, func() { sa.Guardar(m, !guardado) }),
		con(seguir, glifoHilos, func() { sa.AlternarSeguir(m.ID, m.Siguiendo) }),
		con(t.T("message.copyText", nil), glifoCopiar, func() {
			sa.Copiar(m.Cuerpo)
			sa.P.Avisar(ui.AvisoExito, t.T("message.copied", nil))
		}),
		con(t.T("message.copyLink", nil), glifoEnlace, func() {
			sa.Copiar(sa.enlaceA(m))
			sa.P.Avisar(ui.AvisoExito, t.T("message.linkCopied", nil))
		}),
		con(t.T("message.saveToNotes", nil), glifoNota, func() { sa.GuardarEnNotes(m) }),
		con(t.T("message.forward", nil), glifoReenviar, func() { sa.AbrirReenviar(m) }),
		con(t.T("message.markUnread", nil), glifoNoLeido, func() { sa.MarcarNoLeido(m) }),
	)
	if m.Autor.ID != sa.Yo || sa.Yo == "" {
		return append(out, sa.entradasModeracion(m)...)
	}
	out = append(out, con(t.T("message.readersTitle", nil), glifoVisto, func() { sa.AbrirLectores(m) }))
	borrar := con(t.T("action.delete", nil), glifoPapelera, func() { sa.PedirBorrar(m) })
	borrar.Peligro = true
	borrar.Glifo = dibujoRelleno(glifoPapelera, iconos.TallaSM, "danger-text", v)
	return append(out, ui.NuevoSeparadorMenu(), con(t.T("action.edit", nil), glifoLapiz, func() { sa.Editar(m) }), borrar)
}

// enlaceA es `messageLink`: la página del canal con el mensaje, en la web de Rooms, con
// la organización detrás del prefijo (ADR 0056): el enlace canónico dice a qué
// organización apunta, y quien lo reenvía comparte ESA.
func (sa *Sala) enlaceA(m Mensaje) string {
	return api.AppURL(sa.Apex, "rooms", sa.org(), "/channels/"+url.PathEscape(sa.abierto)+"?message="+url.QueryEscape(m.ID))
}

// org es la organización activa de la sesión, o "" sin sesión y en el guardián.
func (sa *Sala) org() string {
	if sa.Org == nil {
		return ""
	}
	return sa.Org()
}

// Fijar fija o desfija un mensaje: se apunta ya y se deshace EN SILENCIO si el servicio
// dice que no, como `togglePin`. El número de la cabecera sigue al conjunto.
func (sa *Sala) Fijar(m Mensaje, poner bool) {
	sa.marcar(sa.fijados, m.ID, poner)
	sa.P.Canal.Cabecera.Fijados = len(sa.fijados)
	sa.P.Canal.Cabecera.Refresh()
	pedir(sa, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, sa.S.Pin(ctx, m.ID, poner)
	}, func(_ struct{}, err error) {
		if err != nil {
			sa.marcar(sa.fijados, m.ID, !poner)
			sa.P.Canal.Cabecera.Fijados = len(sa.fijados)
			sa.P.Canal.Cabecera.Refresh()
		}
	})
}

// Guardar guarda o quita de guardados, con el mismo deshacer.
func (sa *Sala) Guardar(m Mensaje, poner bool) {
	sa.marcar(sa.guardados, m.ID, poner)
	pedir(sa, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, sa.S.Save(ctx, m.ID, poner)
	}, func(_ struct{}, err error) {
		if err != nil {
			sa.marcar(sa.guardados, m.ID, !poner)
		}
	})
}

func (sa *Sala) marcar(conjunto map[string]bool, id string, poner bool) {
	if poner {
		conjunto[id] = true
		return
	}
	delete(conjunto, id)
}

// MarcarNoLeido es `markMessageUnread`: el canal pasa a tener sin leer los mensajes
// ajenos desde ese, se pinta ya y se deshace si el servicio dice que no. Como en la
// web, el contador se ve aunque sea el canal abierto, hasta que se cambia de canal.
func (sa *Sala) MarcarNoLeido(m Mensaje) {
	canal := sa.abierto
	cuenta, desde := 0, false
	for _, x := range sa.mensajes {
		desde = desde || x.ID == m.ID
		if desde && x.AuthorSub != sa.Yo {
			cuenta++
		}
	}
	antes := sa.pend[canal]
	nuevo := antes
	nuevo.ChannelID, nuevo.Unread = canal, max(antes.Unread, max(cuenta, 1))
	sa.pend[canal], sa.noLeidoAbierto = nuevo, true
	sa.pintarCanales()
	sa.P.Refrescar()
	pedir(sa, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, sa.S.MarkUnread(ctx, m.ID)
	}, func(_ struct{}, err error) {
		if err == nil || canal != sa.abierto {
			return
		}
		sa.pend[canal] = antes
		sa.pintarCanales()
		sa.P.Refrescar()
	})
}

// ---- Editar y borrar ----

// Editar cambia el texto del mensaje por su caja de editar.
func (sa *Sala) Editar(m Mensaje) {
	h := sa.P.Canal.Historial
	h.EditandoID = m.ID
	h.Refresh()
}

// CancelarEdicion la cierra sin tocar nada.
func (sa *Sala) CancelarEdicion() {
	h := sa.P.Canal.Historial
	h.EditandoID = ""
	h.Refresh()
}

// GuardarEdicion es `saveEdit` + `handleEdit`: un texto vacío o igual al de antes no
// se manda, solo cierra la caja. El texto nuevo y «(editado)» se pintan YA y se
// deshacen si el servicio dice que no.
func (sa *Sala) GuardarEdicion(m Mensaje, texto string) {
	sa.CancelarEdicion()
	i := slices.IndexFunc(sa.mensajes, func(x api.RoomsMessage) bool { return x.ID == m.ID })
	if i < 0 || strings.TrimSpace(texto) == "" || texto == m.Cuerpo {
		return
	}
	antes := sa.mensajes[i]
	ahora := time.Now()
	sa.mensajes[i].Body, sa.mensajes[i].EditedAt = texto, &ahora
	sa.pintarMensajes()
	sa.P.Canal.Historial.Refresh()
	pedir(sa, func(ctx context.Context) (api.RoomsMessage, error) {
		return sa.S.Edit(ctx, m.ID, texto)
	}, func(nuevo api.RoomsMessage, err error) {
		j := slices.IndexFunc(sa.mensajes, func(x api.RoomsMessage) bool { return x.ID == m.ID })
		if j < 0 {
			return
		}
		if err != nil {
			sa.mensajes[j] = antes
		} else {
			sa.mensajes[j].Body, sa.mensajes[j].EditedAt = nuevo.Body, nuevo.EditedAt
		}
		sa.pintarMensajes()
		sa.P.Canal.Historial.Refresh()
	})
}

// PedirBorrar es el `ConfirmDialog` de «Borrar»: tono de peligro y el verbo en el
// botón, como en la web.
func (sa *Sala) PedirBorrar(m Mensaje) {
	t := sa.P.T
	c := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if c == nil {
		return
	}
	var d *ui.Dialogo
	cuerpo := ui.NuevoParrafo(t.T("message.confirmDelete", nil), tema.TextoSM, ui.Normal, "text",
		tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	d = ui.Confirmacion(t.T("action.delete", nil), t.T("action.cancel", nil), t.T("action.delete", nil), true,
		cuerpo, func() { d.Cerrar() }, func() {
			d.Cerrar()
			sa.Borrar(m)
		})
	d.RotuloCerrar = t.T("action.close", nil)
	d.Mostrar(c)
}

// Borrar quita el mensaje: sale de la lista cuando el servicio lo confirma.
func (sa *Sala) Borrar(m Mensaje) {
	pedir(sa, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, sa.S.Delete(ctx, m.ID)
	}, func(_ struct{}, err error) {
		if err != nil {
			return // como `handleDelete`: el mensaje se queda donde estaba
		}
		sa.mensajes = slices.DeleteFunc(sa.mensajes, func(x api.RoomsMessage) bool { return x.ID == m.ID })
		sa.pintarMensajes()
		sa.P.Canal.Historial.Refresh()
	})
}

// ---- Los paneles de lista: fijados, guardados y búsqueda ----

// AbrirPanel es el `toggle…Panel` de cada botón de la cabecera: abre su panel, o lo
// cierra si ya era el abierto. El hilo y los paneles se excluyen.
//
// «Hilos» y «Borradores» vienen del carril y abren la misma hoja.
//
// «Reuniones» es su propia hoja (reuniones.go) y «Apps», el diálogo de integraciones
// (integraciones.go), como la web.
func (sa *Sala) AbrirPanel(clave string) {
	if sa.abrirDialogo(clave) {
		return
	}
	if sa.panel == clave {
		sa.cerrarDerecho()
		return
	}
	switch clave {
	case PanelFijados, PanelGuardados, PanelBusqueda, PanelHilos, PanelBorradores, PanelProgramado:
	case PanelMiembros:
		sa.abrirMiembros()
		return
	case PanelReuniones:
		sa.cerrarDerecho()
		sa.abrirReuniones()
		return
	default:
		return
	}
	sa.CerrarHilo()
	t := sa.P.T
	var l *PanelLista
	switch clave {
	case PanelFijados:
		l = NuevoPanelLista(t, sa.P.Locale, t.T("panel.pinned", nil))
		l.Subtitulo = "#" + sa.P.Canal.Cabecera.Canal.Nombre
		l.Vacio, l.RotuloQuitar = t.T("panel.pinnedEmpty", nil), t.T("panel.unpin", nil)
		l.AlQuitar = func(m Mensaje) { sa.Fijar(m, false) }
		l.AlReintentar = sa.cargarLista
	case PanelGuardados:
		l = NuevoPanelLista(t, sa.P.Locale, t.T("panel.saved", nil))
		l.Canales = sa.nombresDeCanal()
		l.Vacio, l.RotuloQuitar = t.T("panel.savedEmpty", nil), t.T("panel.unsave", nil)
		l.AlQuitar = func(m Mensaje) { sa.Guardar(m, false) }
		l.AlReintentar = sa.cargarLista
	case PanelBusqueda:
		l = NuevoPanelLista(t, sa.P.Locale, t.T("search.title", nil))
		l.Busqueda, l.Canales, l.Estado = true, sa.nombresDeCanal(), Listo
		l.RotuloCerrar = t.T("search.close", nil)
		l.AlBuscar = sa.Buscar
	case PanelHilos:
		l = NuevoPanelLista(t, sa.P.Locale, t.T("threads.title", nil))
		l.DeHilos, l.RotuloCerrar = true, t.T("threads.close", nil)
		l.AlReintentar = sa.cargarLista
	case PanelProgramado:
		l = NuevoPanelLista(t, sa.P.Locale, t.T("scheduled.title", nil))
		l.DeProgramados, l.RotuloCerrar, l.Canales = true, t.T("scheduled.close", nil), sa.nombresDeCanal()
		l.AlReintentar, l.AlEnviarYa, l.AlCancelarYa = sa.cargarLista, sa.EnviarYa, sa.CancelarProgramado
		l.AlReprogramar = sa.Reprogramar
	case PanelBorradores:
		// Sin carga ni error: los borradores ya están aquí, no hay petición que esperar.
		l = NuevoPanelLista(t, sa.P.Locale, t.T("drafts.title", nil))
		l.DeBorradores, l.Estado, l.Canales = true, Listo, sa.nombresDeCanal()
		l.Borradores = sa.listaBorradores()
		l.AlIrABorrador, l.AlDescartar = sa.IrABorrador, sa.DescartarBorrador
	}
	if l.RotuloCerrar == "" {
		l.RotuloCerrar = t.T("action.closeNamed", i18n.Vars{"name": l.Titulo})
	}
	l.AlCerrar = sa.cerrarDerecho
	l.AlElegir = func(m Mensaje) { sa.IrAMensaje(m.CanalID, m.ID) }
	l.AlCopiar = func(m Mensaje) {
		sa.Copiar(sa.enlaceA(m))
		sa.P.Avisar(ui.AvisoExito, t.T("message.linkCopied", nil))
	}
	if clave == PanelHilos {
		l.AlElegir = sa.AbrirHilo // `onSelect={(th) => openThread(th.root)}`
	}
	sa.lista, sa.panel, sa.miembros = l, clave, nil
	sa.P.Canal.Cabecera.Panel = clave
	sa.soltarHilos()
	if clave == PanelHilos || clave == PanelBorradores {
		sa.P.Lateral.Panel = clave
		sa.P.Refrescar()
	}
	sa.P.PonerDerecho(l)
	switch clave {
	case PanelBusqueda:
		l.Enfocar()
	case PanelBorradores:
	default:
		sa.cargarLista()
	}
}

// VerArchivos es `openChannelFiles`: la búsqueda con `in:#canal has:file`.
func (sa *Sala) VerArchivos() {
	sa.cerrarDerecho()
	sa.AbrirPanel(PanelBusqueda)
	if sa.lista != nil {
		sa.lista.PonerConsulta("in:#" + sa.P.Canal.Cabecera.Canal.Nombre + " has:file ")
	}
}

// cerrarDerecho cierra el panel de la derecha que haya, lista o hilo.
func (sa *Sala) cerrarDerecho() {
	if sa.pararBusca != nil {
		sa.pararBusca()
		sa.pararBusca = nil
	}
	sa.lista, sa.panel, sa.miembros, sa.reuniones = nil, "", nil, nil
	sa.vistaLista++
	sa.P.Canal.Cabecera.Panel = ""
	sa.soltarHilos()
	sa.CerrarHilo()
}

// soltarHilos apaga «Hilos» o «Borradores» en el carril cuando su panel deja de estar
// abierto.
func (sa *Sala) soltarHilos() {
	if p := sa.P.Lateral.Panel; p == PanelHilos || p == PanelBorradores || p == PanelReuniones {
		sa.P.Lateral.Panel = ""
		sa.P.Refrescar()
	}
}

// nombresDeCanal es id → nombre de los canales conocidos: el «#canal» de cada fila.
func (sa *Sala) nombresDeCanal() map[string]string {
	out := make(map[string]string, len(sa.canales))
	for _, c := range sa.canales {
		out[c.ID] = c.Title()
	}
	return out
}

// cargarLista pide los fijados del canal o los guardados de quien mira.
func (sa *Sala) cargarLista() {
	l, clave, canal := sa.lista, sa.panel, sa.abierto
	if l == nil {
		return
	}
	switch clave {
	case PanelHilos:
		sa.cargarSeguidos()
		return
	case PanelProgramado:
		sa.cargarProgramados()
		return
	}
	sa.vistaLista++
	vista := sa.vistaLista
	l.Estado = Cargando
	l.Refresh()
	pedir(sa, func(ctx context.Context) ([]api.RoomsMessage, error) {
		if clave == PanelFijados {
			return sa.S.Pins(ctx, canal)
		}
		return sa.S.Saved(ctx)
	}, func(ms []api.RoomsMessage, err error) {
		if vista != sa.vistaLista {
			return
		}
		sa.ponerEnLista(ms, err)
	})
}

func (sa *Sala) ponerEnLista(ms []api.RoomsMessage, err error) {
	l := sa.lista
	if err != nil {
		l.Estado = Error
		l.Refresh()
		return
	}
	l.Mensajes = l.Mensajes[:0]
	for _, m := range ms {
		l.Mensajes = append(l.Mensajes, sa.mensajeDe(m))
	}
	l.Estado = Listo
	l.Refresh()
}

// ---- «Programados» ----

// cargarProgramados es `loadScheduled`.
func (sa *Sala) cargarProgramados() {
	l := sa.lista
	sa.vistaLista++
	vista := sa.vistaLista
	l.Estado = Cargando
	l.Refresh()
	pedir(sa, sa.S.Scheduled, func(ps []api.RoomsScheduled, err error) {
		if vista != sa.vistaLista || sa.lista != l {
			return
		}
		if err != nil {
			l.Estado = Error
			l.Refresh()
			return
		}
		l.Programados = l.Programados[:0]
		for _, p := range ps {
			l.Programados = append(l.Programados, Programado{
				ID: p.ID, Canal: p.ChannelID, Autor: cmp.Or(p.AuthorName, p.AuthorSub), Cuerpo: p.Body,
				ConAdjuntos: len(p.FileIDs) > 0, Adjuntos: p.FileIDs, Cuando: p.SendAt.Local(),
			})
		}
		l.Estado = Listo
		l.Refresh()
	})
}

// quitarProgramado lo saca de la lista, que es lo primero que hacen «Cancelar» y
// «Enviar ya».
func (sa *Sala) quitarProgramado(m Programado) {
	if l := sa.lista; l != nil && sa.panel == PanelProgramado {
		l.Programados = slices.DeleteFunc(l.Programados, func(x Programado) bool { return x.ID == m.ID })
		l.Refresh()
	}
}

// CancelarProgramado es `cancelScheduledMsg`: sale ya y, si el servicio falla, la
// lista se vuelve a pedir.
func (sa *Sala) CancelarProgramado(m Programado) {
	sa.quitarProgramado(m)
	pedir(sa, func(ctx context.Context) (struct{}, error) { return struct{}{}, sa.S.CancelScheduled(ctx, m.ID) },
		func(_ struct{}, err error) {
			if err != nil && sa.panel == PanelProgramado {
				sa.cargarProgramados()
			}
		})
}

// EnviarYa es `sendScheduledNow`: cancela el programado y lo publica al momento.
func (sa *Sala) EnviarYa(m Programado) {
	t := sa.P.T
	sa.quitarProgramado(m)
	pedir(sa, func(ctx context.Context) (struct{}, error) {
		if err := sa.S.CancelScheduled(ctx, m.ID); err != nil {
			return struct{}{}, err
		}
		_, err := sa.S.PostWith(ctx, m.Canal, m.Cuerpo, api.RoomsSend{FileIDs: m.Adjuntos})
		return struct{}{}, err
	}, func(_ struct{}, err error) {
		if err != nil {
			sa.P.Avisar(ui.AvisoPeligro, t.T("message.sendFailed", nil))
			if sa.panel == PanelProgramado {
				sa.cargarProgramados()
			}
			return
		}
		sa.P.Avisar(ui.AvisoExito, t.T("message.sent", nil))
		if m.Canal == sa.abierto {
			sa.recargarMensajes()
		}
	})
}

// ---- «Hilos»: los que sigues, de todos los canales ----

// paginaHilos es lo que devuelve una página de «Hilos».
type paginaHilos struct {
	hilos []api.RoomsThreadSummary
	sig   string
}

// panelHilos es el panel «Hilos» si está abierto.
func (sa *Sala) panelHilos() *PanelLista {
	if sa.panel == PanelHilos {
		return sa.lista
	}
	return nil
}

// cargarSeguidos es `loadThreads`: la primera página. Al arrancar va en silencio —es
// solo para la pastilla del carril, y un fallo no se enseña—; con el panel abierto,
// este pinta la carga y el error con «Reintentar».
func (sa *Sala) cargarSeguidos() {
	if l := sa.panelHilos(); l != nil {
		sa.vistaLista++
		l.Estado = Cargando
		l.Refresh()
	}
	vista := sa.vistaLista
	pedir(sa, func(ctx context.Context) (paginaHilos, error) {
		hs, sig, err := sa.S.MyThreads(ctx, "")
		return paginaHilos{hs, sig}, err
	}, func(pg paginaHilos, err error) {
		if vista != sa.vistaLista {
			return
		}
		if err != nil {
			if l := sa.panelHilos(); l != nil && vista == sa.vistaLista {
				l.Estado = Error
				l.Refresh()
			}
			return
		}
		sa.seguidos, sa.seguidosSig = pg.hilos, pg.sig
		sa.pintarSeguidos()
		sa.repintarSiguiendo()
	})
}

// repintarSiguiendo vuelve a pintar el canal cuando cambia QUÉ hilos se siguen: el
// «Siguiendo» de la tira y el «Seguir el hilo» del «…» salen de ahí.
func (sa *Sala) repintarSiguiendo() {
	if sa.abierto != "" {
		sa.pintarMensajes()
		sa.P.Canal.Historial.Refresh()
	}
}

// CargarMasSeguidos es `loadMoreThreads`: la página siguiente, sin repetir los que ya
// están. Un fallo deja la lista como estaba.
func (sa *Sala) CargarMasSeguidos() {
	if sa.seguidosSig == "" || sa.cargandoSeguidos {
		return
	}
	sa.cargandoSeguidos = true
	sa.pintarSeguidos()
	sig := sa.seguidosSig
	pedir(sa, func(ctx context.Context) (paginaHilos, error) {
		hs, sig, err := sa.S.MyThreads(ctx, sig)
		return paginaHilos{hs, sig}, err
	}, func(pg paginaHilos, err error) {
		sa.cargandoSeguidos = false
		if err == nil {
			for _, h := range pg.hilos {
				if !slices.ContainsFunc(sa.seguidos, func(x api.RoomsThreadSummary) bool { return x.Root.ID == h.Root.ID }) {
					sa.seguidos = append(sa.seguidos, h)
				}
			}
			sa.seguidosSig = pg.sig
		}
		sa.pintarSeguidos()
		sa.repintarSiguiendo()
	})
}

// pintarSeguidos lleva los hilos al carril —la pastilla cuenta los hilos con
// respuestas sin leer, no las respuestas— y al panel si está abierto.
//
// ponytail: como en la web, la pastilla cuenta solo la página cargada; los hilos con
// actividad reciente van primero, que son los que la tienen.
func (sa *Sala) pintarSeguidos() {
	n := 0
	for _, h := range sa.seguidos {
		if h.UnreadReplies > 0 {
			n++
		}
	}
	if sa.P.Lateral.HilosSinLeer != n {
		sa.P.Lateral.HilosSinLeer = n
		sa.P.Refrescar()
	}
	l := sa.panelHilos()
	if l == nil {
		return
	}
	hs := make([]ResumenHilo, 0, len(sa.seguidos))
	for _, h := range sa.seguidos {
		hs = append(hs, ResumenHilo{Raiz: sa.mensajeDe(h.Root), Actividad: h.LastActivity.Local(), SinLeer: h.UnreadReplies})
	}
	l.Hilos, l.Canales, l.Estado = hs, sa.nombresDeCanal(), Listo
	l.AlCargarMas, l.CargandoMas = nil, sa.cargandoSeguidos
	if sa.seguidosSig != "" {
		l.AlCargarMas = sa.CargarMasSeguidos
	}
	l.Refresh()
}

// verSeguido baja a cero las no leídas de un hilo al abrirlo: el servidor lo da por
// visto, y la pastilla se pone al día sin esperar a recargar.
func (sa *Sala) verSeguido(raiz string) {
	for i := range sa.seguidos {
		if sa.seguidos[i].Root.ID == raiz && sa.seguidos[i].UnreadReplies > 0 {
			sa.seguidos[i].UnreadReplies = 0
			sa.pintarSeguidos()
			return
		}
	}
}

// subirSeguido es «Hilos» en vivo: una respuesta a un hilo que se sigue sube su
// actividad y su cuenta, y sus no leídas si no es el hilo abierto, y lo reordena. Uno
// que no está en la página cargada no se toca.
func (sa *Sala) subirSeguido(carga json.RawMessage) {
	var m api.RoomsMessage
	if json.Unmarshal(carga, &m) != nil || m.ParentID == nil {
		return
	}
	i := slices.IndexFunc(sa.seguidos, func(h api.RoomsThreadSummary) bool { return h.Root.ID == *m.ParentID })
	if i < 0 {
		return
	}
	h := &sa.seguidos[i]
	h.LastActivity = m.CreatedAt
	h.Root.ReplyCount++
	if *m.ParentID != sa.raizHilo {
		h.UnreadReplies++
	}
	slices.SortStableFunc(sa.seguidos, func(a, b api.RoomsThreadSummary) int { return b.LastActivity.Compare(a.LastActivity) })
	sa.pintarSeguidos()
}

// Buscar es el efecto de la caja: con dos letras o más espera 250 ms sin teclear y
// pregunta; lo que vuelve de una consulta vieja no pinta.
func (sa *Sala) Buscar(q string) {
	l := sa.lista
	if sa.pararBusca != nil {
		sa.pararBusca()
		sa.pararBusca = nil
	}
	sa.vistaLista++
	vista := sa.vistaLista
	if len([]rune(strings.TrimSpace(q))) < MinConsulta {
		l.Mensajes, l.Estado = nil, Listo
		l.Refresh()
		return
	}
	l.Estado = Cargando
	l.Refresh()
	sa.pararBusca = sa.Esperar(EsperaBusqueda, func() {
		pedir(sa, func(ctx context.Context) ([]api.RoomsMessage, error) {
			return sa.S.Search(ctx, strings.TrimSpace(q), PorBusqueda)
		}, func(ms []api.RoomsMessage, err error) {
			if vista == sa.vistaLista && sa.lista == l {
				sa.ponerEnLista(ms, err)
			}
		})
	})
}

// PorBusqueda es el `limit=30` de `searchAll`.
const PorBusqueda = 30

// ---- Saltar a un mensaje: `goToMessage` ----

// IrAEnlace es la llegada por el esquema a un canal, y a un mensaje si lo trae: el aviso
// de una mención o de un mensaje directo (`hubnotify.EnlaceMensaje`). Si la lista de
// canales aún no ha llegado, espera a ella. Un canal que no es del usuario, silencio,
// como en la web.
func (sa *Sala) IrAEnlace(canal, mensaje string) {
	if len(sa.canales) == 0 {
		sa.enlace = [2]string{canal, mensaje}
		return
	}
	if mensaje != "" {
		sa.IrAMensaje(canal, mensaje)
		return
	}
	if i := slices.IndexFunc(sa.canales, func(c api.RoomsChannel) bool { return c.ID == canal }); i >= 0 {
		sa.Elegir(sa.canalDe(sa.canales[i]))
	}
}

// IrAMensaje abre el canal del mensaje con una ventana CENTRADA en él y lo resalta
// 2,5 s. No da el canal por leído (los no leídos de después siguen ahí) y deja la
// vista en «viendo historial». Si el mensaje era una respuesta, la ventana llega
// anclada en su raíz, y se abre ese hilo.
func (sa *Sala) IrAMensaje(canal, id string) {
	i := slices.IndexFunc(sa.canales, func(c api.RoomsChannel) bool { return c.ID == canal })
	if i < 0 {
		return
	}
	sa.cerrarDerecho()
	sa.abrir(sa.canalDe(sa.canales[i]))
	sa.pintarCanales()
	sa.P.Refrescar()
	sa.vista++
	vista := sa.vista
	h := sa.P.Canal.Historial
	h.Estado, h.Mensajes, h.IndiceNoLeidos = Cargando, nil, -1
	sa.separador = ""
	h.Refresh()
	pedir(sa, func(ctx context.Context) (ventana, error) {
		ms, raiz, err := sa.S.Around(ctx, canal, id, PorPagina)
		return ventana{ms, raiz}, err
	}, func(v ventana, err error) {
		if vista != sa.vista {
			return
		}
		if err != nil {
			h.Estado = estadoDe(err)
			h.Refresh()
			return
		}
		sa.mensajes = v.mensajes
		sa.P.Canal.Historial.HayAnteriores = len(v.mensajes) > 0 // ventana centrada: hay a los dos lados
		sa.viendoHistorial = true
		sa.P.Canal.ViendoHistorial = true
		sa.pintarMensajes()
		objetivo := id
		if v.raiz != "" {
			objetivo = v.raiz
		}
		sa.resaltar(objetivo)
		sa.P.Canal.Refresh()
		if j := slices.IndexFunc(sa.mensajes, func(m api.RoomsMessage) bool { return m.ID == v.raiz }); v.raiz != "" && j >= 0 {
			sa.AbrirHilo(sa.mensajeDe(sa.mensajes[j]))
		}
	})
}

type ventana struct {
	mensajes []api.RoomsMessage
	raiz     string
}

// resaltar marca el mensaje (`bg-selected ring-2 ring-primary`) y lo apaga a los
// 2,5 s, como `highlightMessage`.
func (sa *Sala) resaltar(id string) {
	h := sa.P.Canal.Historial
	if sa.pararResalte != nil {
		sa.pararResalte()
	}
	h.Resaltado = id
	h.Refresh()
	sa.pararResalte = sa.Esperar(DuracionResalte, func() {
		sa.pararResalte = nil
		if h.Resaltado == id {
			h.Resaltado = ""
			h.QuitarResalte()
		}
	})
}

// DuracionResalte es lo que dura el resalte de un mensaje enlazado.
const DuracionResalte = 2500 * time.Millisecond

// IrAlFinal es `jumpToPresent`: vuelve al final del canal y lo da por leído.
func (sa *Sala) IrAlFinal() {
	sa.viendoHistorial = false
	sa.P.Canal.ViendoHistorial = false
	sa.P.Canal.Refresh()
	sa.cargarMensajes(sa.abierto, 0)
	sa.darPorLeido(sa.abierto)
}

// ---- «Ir a un canal» (Ctrl+G) ----

// AbrirConmutador es `setSwitcherOpen(true)`: el diálogo `sm` con la caja enfocada.
// Elegir un canal lo abre y cierra el diálogo.
func (sa *Sala) AbrirConmutador() {
	c := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if c == nil {
		return
	}
	t := sa.P.T
	con := NuevoConmutador(t, sa.P.Locale, sa.P.Lateral.Canales, sa.abierto)
	con.Marca = func(canal Canal) fyne.CanvasObject {
		return sa.P.Lateral.marca(canal, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	}
	d := ui.NuevoDialogo(t.T("switcher.title", nil), con).Con(ui.DialogoSM, nil)
	d.RotuloCerrar = t.T("action.close", nil)
	d.Foco = con.Caja()
	con.AlElegir = func(canal Canal) {
		d.Cerrar()
		sa.Elegir(canal)
	}
	sa.conmutador = con
	d.Mostrar(c)
}

// Atajos engancha Ctrl+G (y ⌘G) al lienzo de la ventana, que es donde la web lo
// escucha (`document`).
func (sa *Sala) Atajos(c fyne.Canvas) {
	ui.AlAtras(c, sa.atras)
	for _, m := range []fyne.KeyModifier{fyne.KeyModifierControl, fyne.KeyModifierSuper} {
		c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyG, Modifier: m},
			func(fyne.Shortcut) { sa.AbrirConmutador() })
	}
}

// ---- «Crear canal» ----

// AbrirCrearCanal es el `CreateChannelModal`: al crear, el canal entra en el carril y
// se abre (`handleChannelCreated`); los errores del nombre vuelven al formulario.
func (sa *Sala) AbrirCrearCanal() {
	c := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if c == nil {
		return
	}
	t := sa.P.T
	f := NuevoCrearCanal(t)
	var d *ui.Dialogo
	cancelar := ui.NuevoBoton(t.T("action.cancel", nil), func() { d.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	pie := container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, f.BotonCrear())
	d = ui.NuevoDialogo(t.T("createChannel.title", nil), f).Con(ui.DialogoMD, pie)
	d.RotuloCerrar, d.Foco = t.T("action.close", nil), f.Nombre()
	f.AlCrear = func(nombre, asunto string, privado bool) {
		equipo := sa.equipo
		pedir(sa, func(ctx context.Context) (api.RoomsChannel, error) {
			return sa.S.CreateChannel(ctx, equipo, nombre, asunto, privado)
		}, func(canal api.RoomsChannel, err error) {
			f.Ocupado = false
			if err != nil {
				f.ErrorNombre, f.Fallo = errorDeCrear(t, err)
				f.Refresh()
				return
			}
			d.Cerrar()
			sa.entrarEn(canal)
		})
	}
	sa.crearCanal = f
	d.Mostrar(c)
}

// errorDeCrear reparte el fallo de crear: 409, 400 y 403 van junto al nombre; lo
// demás es el aviso genérico de debajo.
func errorDeCrear(t *i18n.Translator, err error) (string, bool) {
	var e *api.Error
	if errors.As(err, &e) {
		switch e.Status {
		case http.StatusConflict:
			return t.T("createChannel.duplicate", nil), false
		case http.StatusBadRequest:
			return t.T("createChannel.invalidName", nil), false
		case http.StatusForbidden:
			return t.T("createChannel.forbidden", nil), false
		}
	}
	return "", true
}

// entrarEn pone en el carril un canal que acaba de aparecer (creado o un directo
// abierto) y lo abre.
func (sa *Sala) entrarEn(canal api.RoomsChannel) {
	sa.sumar(canal)
	sa.Elegir(sa.canalDe(canal))
}

// sumar pone el canal en el carril y lo suma al tiempo real (`handleJoined`).
func (sa *Sala) sumar(canal api.RoomsChannel) {
	if i := slices.IndexFunc(sa.canales, func(c api.RoomsChannel) bool { return c.ID == canal.ID }); i >= 0 {
		sa.canales[i] = canal
	} else {
		sa.canales = append(sa.canales, canal)
	}
	sa.escuchar()
	sa.pintarCanales()
	sa.P.Refrescar()
}

// ---- «Explorar canales» ----

// AbrirExplorar es el `BrowseChannelsModal`: busca con 250 ms de espera (también al
// abrir, como el efecto de la web) y «Unirse» suma el canal al carril SIN abrirlo ni
// cerrar el diálogo, para poder unirse a varios seguidos.
func (sa *Sala) AbrirExplorar() {
	c := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if c == nil {
		return
	}
	t := sa.P.T
	e := NuevoExplorar(t)
	var d *ui.Dialogo
	cerrar := ui.NuevoBoton(t.T("action.close", nil), func() { d.Cerrar() }).Con(ui.Secundario, ui.TallaMD)
	d = ui.NuevoDialogo(t.T("browse.title", nil), e).Con(ui.DialogoMD, cerrar)
	d.RotuloCerrar, d.Foco = t.T("action.close", nil), e.Caja()
	vista, parar := 0, func() {}
	buscar := func(q string) {
		parar()
		vista++
		v := vista
		e.Estado = Cargando
		e.Refresh()
		parar = sa.Esperar(EsperaBusqueda, func() {
			equipo := sa.equipo
			pedir(sa, func(ctx context.Context) ([]api.RoomsChannel, error) {
				return sa.S.Browse(ctx, equipo, q)
			}, func(cs []api.RoomsChannel, err error) {
				if v != vista {
					return
				}
				e.Estado, e.Canales = Listo, e.Canales[:0]
				if err != nil {
					e.Estado = Error
				}
				for _, x := range cs {
					e.Canales = append(e.Canales, sa.canalDe(x))
				}
				e.Refresh()
			})
		})
	}
	e.AlBuscar = buscar
	e.AlReintentar = func() { buscar(e.Caja().Texto()) }
	e.AlUnirse = func(canal Canal) {
		e.Uniendo = canal.ID
		e.Refresh()
		pedir(sa, func(ctx context.Context) (api.RoomsChannel, error) {
			return sa.S.Join(ctx, canal.ID)
		}, func(unido api.RoomsChannel, err error) {
			e.Uniendo = ""
			if err == nil {
				e.Canales = slices.DeleteFunc(e.Canales, func(x Canal) bool { return x.ID == canal.ID })
				sa.sumar(unido)
			}
			e.Refresh()
		})
	}
	sa.explorar = e
	d.Mostrar(c)
	buscar("")
}

// ---- «Nuevo mensaje» ----

// AbrirNuevoDirecto es el `NewDmModal`. La gente del equipo se pide al abrirlo; al
// abrir el directo se le pone de nombre el de las personas elegidas, como
// `onOpened`, y se entra en él.
func (sa *Sala) AbrirNuevoDirecto() {
	c := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if c == nil {
		return
	}
	t := sa.P.T
	nd := NuevoNuevoDirecto(t, nil)
	var d *ui.Dialogo
	cancelar := ui.NuevoBoton(t.T("action.cancel", nil), func() { d.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	pie := container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, nd.BotonAbrir())
	d = ui.NuevoDialogo(t.T("dm.title", nil), nd).Con(ui.DialogoSM, pie)
	d.RotuloCerrar, d.Foco = t.T("action.close", nil), nd.Caja()
	equipo := sa.equipo
	pedir(sa, func(ctx context.Context) ([]api.RoomsMember, error) {
		return sa.S.TeamMembers(ctx, equipo)
	}, func(ms []api.RoomsMember, err error) {
		if err != nil {
			return // como la web: sin lista, se puede seguir buscando por @usuario
		}
		nd.Equipo = nd.Equipo[:0]
		for _, m := range ms {
			if m.Subject != sa.Yo {
				nd.Equipo = append(nd.Equipo, Persona{ID: m.Subject, Nombre: nombreDe(m.Name, m.Subject)})
			}
		}
		nd.Refresh()
	})
	nd.AlBuscarPersona = func(q string) {
		nd.Buscando, nd.ErrorBuscar = true, ""
		nd.Refresh()
		pedir(sa, func(ctx context.Context) (api.RoomsPerson, error) {
			return sa.S.ResolvePerson(ctx, q)
		}, func(p api.RoomsPerson, err error) {
			nd.Buscando = false
			if err != nil {
				nd.ErrorBuscar = t.T(ClaveFalloBusqueda(err), nil)
				nd.Refresh()
				return
			}
			nombre := p.Name
			if nombre == "" && p.Username != "" {
				nombre = "@" + p.Username
			}
			if p.UserID == sa.Yo {
				nd.Caja().PonerTexto("")
				return
			}
			nd.Encontrada(Persona{ID: p.UserID, Nombre: nombreDe(nombre, p.UserID)})
		})
	}
	nd.AlAbrir = func(elegidas []Persona) {
		ids, nombres := make([]string, 0, len(elegidas)), make([]string, 0, len(elegidas))
		for _, p := range elegidas {
			ids, nombres = append(ids, p.ID), append(nombres, p.Nombre)
		}
		pedir(sa, func(ctx context.Context) (api.RoomsChannel, error) {
			return sa.S.OpenDirect(ctx, equipo, ids)
		}, func(canal api.RoomsChannel, err error) {
			nd.Ocupado = false
			if err != nil {
				nd.Fallo = true
				nd.Refresh()
				return
			}
			canal.Name, canal.Peers = strings.Join(nombres, ", "), nombres
			d.Cerrar()
			sa.entrarEn(canal)
		})
	}
	sa.directo = nd
	d.Mostrar(c)
}

// nombreDe es el nombre, o el identificador si no lo hay.
func nombreDe(nombre, id string) string {
	if nombre != "" {
		return nombre
	}
	return id
}

// ---- Miembros ----

// cargarRoster pide el roster del canal abierto: la pila de caras de la cabecera y el
// panel de «Miembros». Un directo no lleva roster en la cabecera.
func (sa *Sala) cargarRoster(c Canal) {
	cab := sa.P.Canal.Cabecera
	sa.roster, cab.Miembros, cab.Caras = nil, 0, nil
	if c.Clase == Directo {
		return
	}
	if sa.miembros != nil {
		sa.miembros.Estado = Cargando
		sa.miembros.Refresh()
	}
	pedir(sa, func(ctx context.Context) ([]api.RoomsMember, error) {
		return sa.S.ChannelMembers(ctx, c.ID)
	}, func(ms []api.RoomsMember, err error) {
		if c.ID != sa.abierto {
			return
		}
		if err != nil {
			if sa.miembros != nil {
				sa.miembros.Estado = Error
				sa.miembros.Refresh()
			}
			return // la cabecera enseña el icono sin número, como la web
		}
		sa.roster = sa.roster[:0]
		for _, m := range ms {
			sa.roster = append(sa.roster, miembroDe(m))
		}
		OrdenarMiembros(sa.P.Locale, sa.roster)
		sa.pintarRoster()
		if sa.miembros != nil {
			sa.refrescarPresencia() // la gente nueva del panel, sin esperar al minuto
		}
	})
}

// nombresRoster es `memberNames`: sub → nombre de la gente del canal abierto.
func (sa *Sala) nombresRoster() map[string]string {
	out := make(map[string]string, len(sa.roster))
	for _, m := range sa.roster {
		out[m.ID] = m.Nombre
	}
	return out
}

func miembroDe(m api.RoomsMember) Miembro {
	return Miembro{ID: m.Subject, Nombre: nombreDe(m.Name, m.Subject), Admin: m.Role == "admin", Externo: m.External}
}

// pintarRoster lleva el roster a la cabecera y al panel.
func (sa *Sala) pintarRoster() {
	cab := sa.P.Canal.Cabecera
	cab.Miembros, cab.Caras = len(sa.roster), nil
	for _, m := range sa.roster[:min(3, len(sa.roster))] {
		cab.Caras = append(cab.Caras, m.Nombre)
	}
	cab.Refresh()
	sa.P.Canal.Compositor.Miembros = sa.roster
	if sa.hilo != nil {
		sa.hilo.Compositor.Miembros = sa.roster
	}
	// Los nombres de las menciones y de «quién reaccionó» salen del roster: con él
	// recién llegado, el historial se vuelve a escribir.
	if len(sa.mensajes) > 0 {
		sa.pintarMensajes()
		sa.P.Canal.Historial.Refresh()
	}
	if p := sa.miembros; p != nil {
		sa.presenciaRoster()
		p.Roster, p.Estado, p.Gestiona = sa.roster, Listo, sa.gestiona()
		p.Refresh()
	}
}

// gestiona es `canManageMembers`: admin del canal, o del equipo si la conversación es
// de la casa (en una de otra organización solo cuenta el rol en ese canal).
func (sa *Sala) gestiona() bool {
	externo := false
	if i := slices.IndexFunc(sa.canales, func(c api.RoomsChannel) bool { return c.ID == sa.abierto }); i >= 0 {
		externo = sa.canales[i].External
	}
	if sa.adminEquipo && !externo {
		return true
	}
	return slices.ContainsFunc(sa.roster, func(m Miembro) bool { return m.ID == sa.Yo && m.Admin })
}

// abrirMiembros es `toggleMembersPanel`: el panel con el roster ya cargado y la gente
// del equipo para «Añadir personas».
func (sa *Sala) abrirMiembros() {
	sa.CerrarHilo()
	t := sa.P.T
	p := NuevoPanelRoster(t, sa.P.Locale)
	p.NombreCanal, p.Yo, p.Roster, p.Gestiona = sa.P.Canal.Cabecera.Canal.Nombre, sa.Yo, sa.roster, sa.gestiona()
	p.Estado = Listo
	p.AlCerrar = sa.cerrarDerecho
	canal := sa.abierto
	p.AlReintentar = func() {
		if i := slices.IndexFunc(sa.canales, func(c api.RoomsChannel) bool { return c.ID == canal }); i >= 0 {
			sa.cargarRoster(sa.canalDe(sa.canales[i]))
		}
	}
	accion := func(m Miembro, llamada func(context.Context) (*api.RoomsMember, error), hecho string) {
		p.Ocupado, p.Aviso, p.ErrorAccion = m.ID, "", ""
		p.Refresh()
		pedir(sa, llamada, func(nuevo *api.RoomsMember, err error) {
			p.Ocupado = ""
			if err != nil {
				p.ErrorAccion = t.T(ClaveErrorMiembros(err), nil)
				p.Refresh()
				return
			}
			// De otra organización: es una invitación, entra al aceptar (ChatApp.tsx).
			if nuevo != nil && nuevo.Pending {
				p.Aviso = t.T("members.invited", i18n.Vars{"name": nuevo.Name})
				p.Refresh()
				return
			}
			sa.roster = slices.DeleteFunc(sa.roster, func(x Miembro) bool { return x.ID == m.ID })
			if nuevo != nil {
				sa.roster = append(sa.roster, miembroDe(*nuevo))
				OrdenarMiembros(sa.P.Locale, sa.roster)
			}
			p.Aviso = hecho
			sa.pintarRoster()
		})
	}
	p.AlAnadir = func(m Miembro) {
		accion(m, func(ctx context.Context) (*api.RoomsMember, error) {
			n, err := sa.S.AddChannelMember(ctx, canal, m.ID, m.Nombre)
			return &n, err
		}, t.T("members.added", i18n.Vars{"name": m.Nombre}))
	}
	if sa.denuncias() != nil {
		p.Bloqueado = sa.bloqueado
		p.AlDenunciar = func(m Miembro) { sa.AbrirDenuncia("", canal, m.ID, m.Nombre) }
		p.AlBloquear = func(m Miembro) { sa.AlternarBloqueo(canal, m.ID, m.Nombre, sa.bloqueado(m.ID)) }
	}
	p.AlQuitar = func(m Miembro) {
		accion(m, func(ctx context.Context) (*api.RoomsMember, error) {
			return nil, sa.S.RemoveChannelMember(ctx, canal, m.ID)
		}, t.T("members.removed", i18n.Vars{"name": m.Nombre}))
	}
	p.AlRol = func(m Miembro, admin bool) {
		rol, hecho := "member", t.T("members.adminRemoved", i18n.Vars{"name": m.Nombre})
		if admin {
			rol, hecho = "admin", t.T("members.adminGranted", i18n.Vars{"name": m.Nombre})
		}
		accion(m, func(ctx context.Context) (*api.RoomsMember, error) {
			n, err := sa.S.SetChannelMemberRole(ctx, canal, m.ID, rol)
			return &n, err
		}, hecho)
	}
	p.AlBuscarPersona = func(q string) {
		p.Buscando, p.ErrorBuscar = true, ""
		p.Refresh()
		pedir(sa, func(ctx context.Context) (api.RoomsPerson, error) {
			return sa.S.ResolvePerson(ctx, q)
		}, func(per api.RoomsPerson, err error) {
			p.Buscando = false
			if err != nil {
				p.ErrorBuscar = t.T(ClaveFalloBusqueda(err), nil)
				p.Refresh()
				return
			}
			p.Caja().PonerTexto("")
			p.AlAnadir(Miembro{ID: per.UserID, Nombre: nombreDe(per.Name, per.UserID), Externo: true})
		})
	}
	equipo := sa.equipo
	pedir(sa, func(ctx context.Context) ([]api.RoomsMember, error) {
		return sa.S.TeamMembers(ctx, equipo)
	}, func(ms []api.RoomsMember, err error) {
		if err != nil {
			return // «No se pudo cargar el equipo» sale del propio panel
		}
		p.Equipo = p.Equipo[:0]
		for _, m := range ms {
			p.Equipo = append(p.Equipo, miembroDe(m))
		}
		sa.presenciaRoster()
		p.Refresh()
	})
	sa.miembros, sa.panel = p, PanelMiembros
	sa.presenciaRoster() // lo que ya se sabe (los directos), antes de pintar
	sa.P.Canal.Cabecera.Panel = PanelMiembros
	sa.P.PonerDerecho(p)
	sa.refrescarPresencia() // la del canal, al abrir y no al siguiente minuto
}

// MenuCanal es `channelHeaderMenuItems`: el menú del nombre del canal, que en un
// teléfono es la única puerta a los paneles que la banda esconde.
//
// Tras los paneles van «Ajustes de notificación» y, para quien administra el equipo
// en un canal de la casa, «Editar el tema» tras un separador. Cada entrada con su glifo.
func (sa *Sala) MenuCanal(disparador fyne.CanvasObject) {
	if c := sa.P.Canal.Cabecera.Canal; c.Clase == Directo && c.ID != "" {
		// El menú de fila ya contiene «Borrar chat», solo para esta persona.
		sa.MenuFila(c, disparador)
		return
	}
	t := sa.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	con := func(rotulo, cuerpo string, al func()) *ui.ElementoMenu {
		it := ui.NuevoElementoMenu(rotulo, iconos.Ninguno, al)
		it.Glifo = dibujo(cuerpo, iconos.TallaSM, "text-muted", v)
		return it
	}
	m := ui.NuevoMenu(
		ui.NuevoElementoMenu(t.T("channel.members", nil), iconos.Usuarios, func() { sa.AbrirPanel(PanelMiembros) }),
		con(t.T("panel.pinned", nil), glifoChincheta, func() { sa.AbrirPanel(PanelFijados) }),
		con(t.T("channel.saved", nil), glifoMarcador, func() { sa.AbrirPanel(PanelGuardados) }),
		con(t.T("channel.scheduledMessages", nil), glifoReloj, func() { sa.AbrirPanel(PanelProgramado) }),
		ui.NuevoElementoMenu(t.T("notify.openSettings", nil), iconos.Campana, sa.AbrirAjustesAviso),
	)
	if sa.denuncias() != nil {
		m.Elementos = append(m.Elementos,
			ui.NuevoElementoMenu(t.T("block.list", nil), iconos.Ninguno, sa.AbrirBloqueados))
		// La cola, para quien administra el equipo o la organización; al resto el
		// servicio le daría una cola vacía y no se le enseña la puerta.
		if sa.adminEquipo || sa.AdminOrg {
			m.Elementos = append(m.Elementos,
				ui.NuevoElementoMenu(t.T("moderation.title", nil), iconos.Ninguno, sa.AbrirDenuncias))
		}
		m = ui.NuevoMenu(m.Elementos...)
	}
	if sa.P.Canal.Cabecera.PuedeEditarTema {
		m.Elementos = append(m.Elementos, ui.NuevoSeparadorMenu(),
			ui.NuevoElementoMenu(t.T("channel.editTopic", nil), iconos.Ninguno, sa.EditarTema))
		m = ui.NuevoMenu(m.Elementos...)
	}
	// Salir y borrar también aquí, como en la web: en un teléfono el nombre del canal
	// es el camino a todo lo del canal.
	if c := sa.P.Canal.Cabecera.Canal; c.ID != "" {
		m.Elementos = append(m.Elementos, ui.NuevoSeparadorMenu(),
			ui.NuevoElementoMenu(t.T("channel.leave", nil), iconos.Ninguno, func() { sa.PedirSalir(c) }))
		if sa.puedeBorrar(c) {
			borrar := ui.NuevoElementoMenu(t.T("channel.delete", nil), iconos.Ninguno, func() { sa.PedirBorrarCanal(c) })
			borrar.Peligro = true
			m.Elementos = append(m.Elementos, borrar)
		}
		m = ui.NuevoMenu(m.Elementos...)
	}
	m.Rotulo = t.T("channel.menuAria", i18n.Vars{"name": sa.P.Canal.Cabecera.Canal.Nombre})
	m.AlInicio = true
	sa.menuCanal = m
	m.Mostrar(disparador)
}

// Olvidar removes decrypted process state when the Sytue account/tenant changes.
func (sa *Sala) Olvidar() {
	sa.Parar()
	for key, stop := range sa.subidas {
		stop()
		delete(sa.subidas, key)
	}
	sa.cerrarDerecho()
	if sa.hilo != nil {
		sa.hilo.Compositor.Vaciar()
		sa.hilo = nil
	}
	sa.vista++
	sa.vistaHilo++
	sa.mensajes = nil
	sa.borradores = map[ambito]string{}
	sa.canales = nil
	clear(sa.retiradas)
	sa.roster = nil
	sa.equipo = ""
	sa.abierto = ""
	sa.raizHilo = ""
	sa.arrancada = false
	sa.Yo = ""
	clear(sa.miniaturas)
	clear(sa.enlaces)
	clear(sa.claves)
	sa.resumen = ""
	sa.P.Canal.Cabecera.Canal = Canal{}
	sa.P.Canal.Compositor.NombreCanal = ""
	sa.P.Canal.Historial.NombreCanal = ""
	sa.P.Canal.Compositor.Vaciar()
	sa.P.Canal.Historial.Mensajes = nil
	sa.P.Canal.Derecho = nil
	sa.P.Canal.HayCanales = false
	sa.P.Canal.EstadoCanales = Cargando
	sa.P.Lateral.Canales = nil
	sa.P.Lateral.CanalID = ""
	sa.P.Lateral.Borradores = 0
	sa.P.SinCanal()
	sa.P.PonerPagina(nil)
	sa.P.Refrescar()
}

// EventoCifrado runs on the UI thread after authenticated Matrix decryption.
// A key that arrives after the metadata receipt retries the visible placeholder.
func (sa *Sala) EventoCifrado(value mx.DecryptedEvent, err error) {
	if err != nil || value.ChannelID == "" || value.ChannelID != sa.abierto {
		return
	}
	retry := sa.P.Canal.Historial.Estado == Cargando || sa.raizHilo != ""
	for _, message := range sa.mensajes {
		if message.EncryptionError && message.MatrixEvent != nil && *message.MatrixEvent == value.Reference {
			retry = true
			break
		}
	}
	if !retry {
		return
	}
	if sa.pararCifrado != nil {
		sa.pararCifrado()
	}
	sa.pararCifrado = sa.Esperar(150*time.Millisecond, func() {
		sa.pararCifrado = nil
		if sa.ctx == nil || sa.ctx.Err() != nil || sa.abierto != value.ChannelID {
			return
		}
		sa.recargarMensajes()
		if sa.raizHilo != "" {
			sa.cargarHilo(false)
		}
	})
}

func (sa *Sala) seguridadMensaje(m api.RoomsMessage) string {
	if sa.Cifrado == nil {
		return ""
	}
	key := ""
	switch {
	case m.EncryptionError:
		key = "matrix.unreadable"
	case m.MatrixEvent == nil && m.Type != "call":
		key = "matrix.legacy"
	case m.MatrixEvent != nil && !m.AuthenticityVerified:
		key = "matrix.senderUnverified"
	case m.MatrixEvent != nil && !m.IdentityVerified:
		key = "matrix.identityUnverified"
	}
	if key == "" {
		return ""
	}
	return sa.P.T.T(key, nil)
}

// Dialogs own their visibility; list panels keep the channel sidebar selection.
func (sa *Sala) abrirDialogo(clave string) bool {
	switch clave {
	case PanelCifrado:
		if sa.Cifrado != nil {
			sa.Cifrado.AbrirPanel()
		}
	case PanelExternas:
		sa.AbrirConversacionesExternas()
	case PanelIntegracion:
		sa.AbrirIntegraciones()
	default:
		return false
	}
	return true
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"context"
	"errors"
	"io"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/llamada/camara"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/video"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// # La app viva
//
// Las pantallas son funciones de sus datos. App guarda el estado, pide al servicio y
// vuelve a construir la pantalla cuando algo cambia. Todo lo que toca el estado corre
// en el hilo de la interfaz: las llamadas van a una goroutine (`lanzar`) y vuelven con
// `hilo`, que tira la respuesta si entretanto se cambió de pantalla.

// Ruta es la pantalla en la que está la app.
type Ruta uint8

// Las siete pantallas.
const (
	RutaInicio Ruta = iota
	RutaExplorar
	RutaEmitir
	RutaSubir
	RutaVer
	RutaGestionar
	RutaCanal
	// RutaDesdeDrive es «Subir a Cast» de un vídeo de Drive (/upload?drive=<id>).
	RutaDesdeDrive
	// RutaModeracion es «Denuncias y bloqueos» (/moderation).
	RutaModeracion
)

// Sala es la llamada del directo, para ver o para emitir; la cumple *llamada.Llamada.
type Sala interface {
	Participantes() []llamada.Participante
	Camara(encendida bool) error
	Pantalla(encendida bool) error
	Micro(abierto bool) error
	Colgar()
}

// Fuente es un fichero de vídeo abierto por rangos: lo que lee el reproductor.
type Fuente interface {
	io.ReadSeeker
	io.Closer
}

// App es Sytue Cast en marcha. El cero no vale: se crea con Nueva.
type App struct {
	cli *api.Cast
	w   fyne.Window
	v   Vista
	// Widget publica la cuota ya pedida para subir, sin otra lectura de la red.
	Widget widgets.Source

	// ConSesion dice si hay alguien dentro. nil: nadie.
	ConSesion func() bool
	// Yo es el `sub` de quien ha entrado, o "": lo propio no se denuncia ni se bloquea.
	Yo func() string
	// AlEntrar abre Sytue ID.
	AlEntrar func()
	// Elegir abre el selector de ficheros del sistema y devuelve la ruta elegida.
	Elegir func() ([]string, error)
	// ElegirImagen es Elegir para el póster.
	ElegirImagen func() (string, error)
	// AbrirFichero abre un fichero local para subirlo.
	AbrirFichero func(ruta string) (io.ReadCloser, int64, error)
	// Guardar deja una descarga en la carpeta de descargas y dice dónde.
	Guardar func(nombre string, r io.Reader) (string, error)
	// WebDe compone la dirección de una página de la web de Cast (`/watch/<id>`).
	WebDe func(ruta string) string
	// AbrirMedia abre un fichero de /media para el reproductor; nil usa el cliente.
	AbrirMedia func(ctx context.Context, ruta string) (Fuente, error)
	// Audio es la tarjeta de sonido del reproductor y del directo; nil, sin sonido.
	Audio llamada.Dispositivo
	// AbrirDrive abre Sytue Drive (la app o su web): «Abrir Sytue Drive» de Subir a Cast.
	AbrirDrive func()
	// Conectar entra en la sala de LiveKit de un directo; nil, no se ve el directo.
	Conectar func(url, token string, o llamada.Opciones) (Sala, error)
	// Camara y Pantalla abren la cámara y la pantalla para emitir; nil, no se ofrecen.
	// Camara abre la elegida con CamaraElegida (en el teléfono, delantera o trasera).
	Camara, Pantalla func() (camara.Fuente, error)

	// Lo del teléfono (app_movil.go). Movil es que la app corre en un teléfono o una
	// tableta: la pantalla completa es la del reproductor y «atrás» vuelve de página.
	Movil bool
	// PantallaEncendida pide al sistema que no apague la pantalla mientras se ve o se
	// emite (`Driver.SetDisableScreenBlanking`); nil, no se pide.
	PantallaEncendida func(bool)
	// PedirMicro pide el permiso del micrófono (Android); nil, no hace falta.
	PedirMicro func() error
	// Camaras son las cámaras que hay; CamaraConcedida dice, sin preguntar, si ya hay
	// permiso para usarlas. nil: no se ofrece cambiar de cámara ni se espera al permiso.
	Camaras         func() []camara.Dispositivo
	CamaraConcedida func() bool
	// PiPMovil es la imagen dentro de imagen del sistema en el teléfono
	// (sesion.ImagenEnImagen); nil, el teléfono no la tiene y no se ofrece.
	PiPMovil PiPSistema
	// SoltarCopia borra la copia temporal que deja el selector del teléfono de un fichero
	// elegido, cuando ya no se va a subir (sesion.SoltarElegido); nil, no hay copias.
	SoltarCopia func(ruta string)

	lanzar func(func())
	// enLaInterfaz es `fyne.Do`: lleva al hilo de la interfaz lo que llega de otras
	// rutinas. Es un campo por los tests: el driver de pruebas de Fyne ejecuta `fyne.Do`
	// en la MISMA goroutine que lo llama, y lo que llega de las rutinas del reproductor
	// correría a la vez que el test pinta. El test del reproductor pone una cola que
	// vacía él.
	enLaInterfaz func(func())
	raiz         *fyne.Container
	avisos       *ui.PilaAvisos
	scroll       *container.Scroll
	ruta         Ruta
	gen          int

	buscador      *ui.Entrada
	buscaInicio   *ui.Entrada
	tituloSubida  *ui.Entrada
	tituloDirecto *ui.Entrada
	descDirecto   *ui.AreaTexto
	tituloAhora   *ui.Entrada
	tituloDrive   *ui.Entrada
	comentario    *ui.Entrada
	mensaje       *ui.Entrada
	chatLista     *container.Scroll
	gTitulo       *ui.Entrada
	gDescripcion  *ui.AreaTexto
	gPersona      *ui.Entrada
	visor         *Visor
	previa        *Previa
	// previaCam y previaPant son la cámara y la pantalla abiertas ANTES de emitir,
	// para la vista previa; al empezar se sueltan y las abre la sala.
	previaCam, previaPant camara.Fuente
	pararPrevia           chan struct{}
	reproductor           *video.Reproductor
	// pip es la ventana de imagen dentro de imagen, dueña de su reproductor mientras
	// está abierta; pendiente, la que se cerró con «volver» y espera a que la página
	// de su vídeo la recoja.
	pip, pendiente *ventanaPiP
	esperaBusqueda *time.Timer
	pararReloj     chan struct{}
	fuente         Fuente
	sala           Sala
	// emiSala es la sala en la que se EMITE desde aquí; sobrevive a cambiar de pantalla.
	emiSala         Sala
	chat            *api.CastChat
	ctxChat         context.CancelFunc
	arrastrando     bool
	ultimoEnlace    string
	ultimaInsercion string
	// posters es de dónde se pide el póster de cada vídeo visto (id → /media/…).
	posters sync.Map
	movil   estadoMovil

	ini estadoInicio
	exp estadoExplorar
	can estadoCanal
	ver estadoVer
	mod estadoModeracion
	// bloqueados son las personas que se han bloqueado en esta sesión (por su sub).
	bloqueados map[string]bool
	sub        estadoSubir
	ges        estadoGestionar
	dri        estadoDrive
	emi        estadoEmitir
}

// estadoCarga es lo común a las pantallas que piden algo al entrar.
type estadoCarga struct {
	cargando bool
	fallo    error
}

type estadoInicio struct {
	estadoCarga
	d           Inicio
	pedidos     bool
	cursor      string
	masCargando bool
}

type estadoExplorar struct {
	estadoCarga
	directos, videos []api.CastVideo
	cursor           string
	mas              bool
	pedidos          bool
}

type estadoCanal struct {
	estadoCarga
	org     string
	canal   api.CastChannel
	pedidos bool
	mas     bool
}

type estadoVer struct {
	estadoCarga
	id           string
	video        api.CastVideo
	play         *api.CastPlay
	comentarios  []api.CastComment
	cargandoCom  bool
	errorCom     string
	respondiendo *api.CastComment
	enviando     bool
	mensajes     []api.CastChatMessage
	conectado    bool
	errorChat    string
	viendo       int
	// descAbierta es el «Mostrar más» de la descripción.
	descAbierta bool
	// cursorCom es la página siguiente de comentarios; errorCargaCom, que la primera
	// no se pudo leer.
	cursorCom     string
	errorCargaCom bool
	// compartir es el diálogo abierto, para repintarlo al copiar.
	compartir *ui.Dialogo
	copiado   string
	// La calidad (calidad_ver.go): la elegida en el menú (0, «Automática»), la del
	// WebM que suena, si se está cambiando, cuándo cambió «Automática» por última vez,
	// su espera tras un cambio de tamaño y el medidor del ritmo de descarga.
	calidad, enUso int
	cambiando      bool
	ultimoAuto     time.Time
	esperaAuto     *time.Timer
	medida         *fuenteMedida
}

type estadoSubir struct {
	cuota       *api.CastQuota
	errorCuota  bool
	visibilidad string
	cola        []Envio
	siguiente   int
	cancelar    map[int]context.CancelFunc
	// preparados esperan a que se confirmen las calidades; calidades es la
	// elección, y tocadas dice si alguien la cambió (si no, va la recomendada).
	preparados []Preparado
	calidades  []int
	tocadas    bool
}

type estadoDrive struct {
	estadoCarga
	id          string
	fichero     *api.CastDriveFile
	libres      int64
	visibilidad string
	enviando    bool
	errorEnvio  string
	// estimacion es la de las calidades (solo del tamaño: aproximada).
	estimacion      *api.CastEstimate
	errorEstimacion bool
	calidades       []int
	tocadas         bool
}

type estadoGestionar struct {
	estadoCarga
	id            string
	video         api.CastVideo
	personas      []api.CastPerson
	errorPersona  string
	guardando     bool
	posterOcupado bool
	// cambiadoPintado: la última pantalla pintada tenía cambios sin guardar. Al teclear
	// solo se repinta cuando eso cambia.
	cambiadoPintado bool
	cal             estadoCalidades
}

type estadoEmitir struct {
	clave       *api.CastStreamKey
	visible     bool
	errorClave  string
	regenerando bool
	visibilidad string
	programando bool
	fuente      string
	// directos es nil mientras llegan; sinConexion, que no llegarán; estado, el de la
	// página (sin permiso para emitir).
	directos    []api.CastVideo
	sinConexion bool
	estado      Estado
	ahora       ahoraEmision
}

// ahoraEmision es «Emitir desde aquí»: lo elegido y cómo va.
type ahoraEmision struct {
	elegido                 *api.CastVideo
	visibilidad             string
	camara, pantalla, micro bool
	fase                    FaseEmision
	denegado                bool
	// video es el directo que se está emitiendo (el elegido o el recién programado).
	video api.CastVideo
}

// Nueva monta la app contra ese cliente. `w` es la ventana (diálogos, portapapeles,
// pantalla completa).
func Nueva(cli *api.Cast, v Vista, w fyne.Window) *App {
	a := &App{cli: cli, w: w, v: v, lanzar: func(f func()) { go f() }, enLaInterfaz: fyne.Do}
	a.buscador = ui.NuevaEntrada(v.Traduce("explore.search")).Con(ui.CampoSM)
	a.buscador.Icono = iconoBuscar
	a.buscador.AlCambiar(func(s string) {
		if a.ruta == RutaExplorar {
			a.pintar()
		}
	})
	a.buscaInicio = ui.NuevaEntrada(v.Traduce("home.search")).Con(ui.CampoSM)
	a.buscaInicio.Icono = iconoBuscar
	a.buscaInicio.AlCambiar(func(string) {
		if a.ruta != RutaInicio {
			return
		}
		if a.esperaBusqueda != nil {
			a.esperaBusqueda.Stop()
		}
		gen := a.gen
		a.esperaBusqueda = time.AfterFunc(250*time.Millisecond, func() { a.hilo(gen, a.pedirInicio) })
	})
	a.tituloSubida = ui.NuevaEntrada(v.Traduce("upload.titleField")).Con(ui.CampoSM)
	a.tituloDirecto = ui.NuevaEntrada("").Con(ui.CampoSM)
	a.tituloDirecto.AlCambiar(alVaciarse(a.pintarSiEmitir))
	a.descDirecto = ui.NuevaAreaTexto("")
	a.descDirecto.Con(ui.CampoSM)
	a.tituloAhora = ui.NuevaEntrada("").Con(ui.CampoSM)
	a.tituloDrive = ui.NuevaEntrada("").Con(ui.CampoSM)
	a.tituloDrive.AlCambiar(alVaciarse(func() {
		if a.ruta == RutaDesdeDrive {
			a.pintar()
		}
	}))
	a.tituloAhora.AlCambiar(alVaciarse(a.pintarSiEmitir))
	a.comentario = ui.NuevaEntrada(v.Traduce("comments.placeholder")).Con(ui.CampoSM)
	a.comentario.AlTeclear = alIntro(a.comentar)
	a.mensaje = ui.NuevaEntrada(v.Traduce("chat.placeholder")).Con(ui.CampoSM)
	a.mensaje.AlTeclear = alIntro(a.enviarChat)
	a.chatLista = container.NewVScroll(container.NewStack())
	a.gTitulo = ui.NuevaEntrada("").Con(ui.CampoSM)
	a.gTitulo.AlCambiar(func(string) { a.pintarSiGestionar() })
	a.gDescripcion = ui.NuevaAreaTexto("")
	a.gDescripcion.Con(ui.CampoSM)
	a.gDescripcion.AlCambiar(func(string) { a.pintarSiGestionar() })
	a.gPersona = ui.NuevaEntrada(v.Traduce("manage.people.placeholder")).Con(ui.CampoSM)
	a.gPersona.AlTeclear = alIntro(a.anadirPersona)
	a.sub.visibilidad = api.CastOrg
	a.sub.cancelar = map[int]context.CancelFunc{}
	a.emi.visibilidad, a.emi.fuente = api.CastOrg, "rtmp"
	a.emi.ahora = ahoraEmision{visibilidad: api.CastOrg, micro: true}
	a.scroll = container.NewVScroll(container.NewStack())
	a.raiz = container.NewStack()
	a.avisos = ui.NuevaPilaAvisos()
	if app := fyne.CurrentApp(); app != nil {
		app.Settings().AddListener(func(fyne.Settings) {
			if varianteActual() != a.v.Variante {
				fyne.Do(a.pintar)
			}
		})
	}
	return a
}

// alVaciarse llama a `f` solo cuando el campo pasa de vacío a escrito o al revés: lo
// que habilita un botón, sin repintar la página a cada tecla.
func alVaciarse(f func()) func(string) {
	vacio := true
	return func(s string) {
		if v := strings.TrimSpace(s) == ""; v != vacio {
			vacio = v
			f()
		}
	}
}

// alIntro dispara `f` con Intro en un campo de una línea.
func alIntro(f func()) func(*fyne.KeyEvent) bool {
	return func(e *fyne.KeyEvent) bool {
		if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
			f()
			return true
		}
		return false
	}
}

// Objeto es lo que se cuelga de la ventana, debajo de la barra de la suite.
func (a *App) Objeto() fyne.CanvasObject { return a.raiz }

// Ruta dice en qué pantalla está.
func (a *App) Ruta() Ruta { return a.ruta }

// Arrancar enchufa el teclado y pide la primera pantalla: Inicio con sesión, Explorar
// sin ella (la portada pública es lo que se puede ver sin entrar).
func (a *App) Arrancar() {
	if a.w != nil {
		a.w.Canvas().SetOnTypedKey(a.tecla)
		a.w.SetOnDropped(a.soltado)
		// Después de la ranura de teclas: AlAtras la envuelve.
		ui.AlAtras(a.w.Canvas(), a.atras)
	}
	a.pedirAbiertas()
	if a.gen > 0 {
		return // un enlace llegó antes y ya puso su pantalla
	}
	if a.dentro() {
		a.Ir(RutaInicio)
	} else {
		a.Ir(RutaExplorar)
	}
}

func (a *App) dentro() bool { return a.ConSesion != nil && a.ConSesion() }

// Ir cambia a una de las pantallas sin parámetro.
func (a *App) Ir(r Ruta) {
	a.apuntar(lugar{ruta: r})
	a.salirDeVer()
	a.ruta = r
	a.gen++
	a.scroll.Offset = fyne.NewPos(0, 0)
	switch r {
	case RutaInicio:
		a.pedirInicio()
		// Lo que se procesa o está en directo cambia solo: cada 20 s, como la web.
		a.sondear(20*time.Second, a.hayVivoEnInicio, a.pedirInicio)
	case RutaExplorar:
		a.pedirExplorar(false)
	case RutaEmitir:
		a.pedirEmitir()
		a.sondear(15*time.Second, a.dentro, a.pedirDirectos)
	case RutaSubir:
		a.pedirCuota()
	case RutaModeracion:
		a.pedirModeracion()
	case RutaVer, RutaGestionar, RutaCanal, RutaDesdeDrive:
		// Llevan parámetro: AbrirVideo, AbrirGestionar y AbrirCanal.
	}
	a.pintar()
}

func (a *App) irSeccion(s Seccion) {
	switch s {
	case SecInicio:
		a.Ir(RutaInicio)
	case SecExplorar:
		a.Ir(RutaExplorar)
	case SecEmitir:
		a.Ir(RutaEmitir)
	case SecSubir:
		a.Ir(RutaSubir)
	case SecModeracion:
		a.Ir(RutaModeracion)
	}
}

// hilo lleva `f` al hilo de la interfaz si la pantalla no ha cambiado.
func (a *App) hilo(gen int, f func()) {
	a.enLaInterfaz(func() {
		if gen == a.gen {
			f()
		}
	})
}

// ctx es el de una llamada al servicio: con tope.
func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 60*time.Second)
}

// ---- Pintar ----

// vistaDe decide en cuál de los estados está una pantalla.
func (a *App) vistaDe(e estadoCarga, sinDatos, vacia bool, reintentar func()) Vista {
	v := a.v
	v.AlEntrar = a.AlEntrar
	v.AlReintentar = reintentar
	switch {
	case e.fallo != nil && sinDatos:
		v.Estado = estadoDeFallo(e.fallo)
		var ae *api.Error
		if errors.As(e.fallo, &ae) && ae.Message() != "" && v.Estado == Error {
			v.Detalle = ae.Message()
		}
	case e.cargando && sinDatos:
		v.Estado = Cargando
	case vacia:
		v.Estado = Vacia
	}
	return v
}

// sinSesion es la vista de una pantalla que la necesita cuando no la hay.
func (a *App) sinSesion() Vista {
	v := a.v
	v.AlEntrar, v.Estado = a.AlEntrar, SinSesion
	return v
}

// pintar rehace la pantalla de la ruta actual.
func (a *App) pintar() {
	a.v.Variante = varianteActual()
	if a.v.Teca != nil {
		a.v.Teca.Olvidar()
	}
	var o fyne.CanvasObject
	activa := Seccion(255)
	switch a.ruta {
	case RutaInicio:
		o, activa = a.pantallaInicio(), SecInicio
	case RutaExplorar:
		o, activa = a.pantallaExplorar(), SecExplorar
	case RutaCanal:
		o = a.pantallaCanal()
	case RutaDesdeDrive:
		o, activa = a.pantallaDesdeDrive(), SecSubir
	case RutaVer:
		o = a.pantallaVer()
	case RutaSubir:
		o, activa = a.pantallaSubir(), SecSubir
	case RutaGestionar:
		o = a.pantallaGestionar()
	case RutaEmitir:
		o, activa = a.pantallaEmitir(), SecEmitir
	case RutaModeracion:
		o, activa = a.pantallaModeracion(), SecModeracion
	}
	a.scroll.Content = container.New(&a.movil.hueco, o)
	a.scroll.Refresh()
	refresco := ui.RefrescarAlTirar(a.scroll, &ui.RefrescoLista{
		Arriba:     func() bool { return a.scroll.Offset.Y <= 0.5 },
		Puede:      a.puedeRefrescarLista,
		Revision:   func() uint64 { return ui.RevisionLista(a.gen) },
		Actualizar: a.refrescarLista,
	})
	arm := ui.NuevoArmazon(container.New(&medirVentana{a: a}, refresco))
	arm.Menu = Lateral(a.v, activa, a.irSeccion)
	arm.Titulo = a.v.Traduce("app.title")
	a.raiz.Objects = []fyne.CanvasObject{arm}
	if a.ruta == RutaInicio && a.dentro() {
		// «Subir» al alcance del pulgar (MobileFab de la web, `fab` en Inicio): solo se
		// ve por debajo de 768, donde el menú es un cajón cerrado.
		fab := ui.NuevoBotonFlotante(a.v.Traduce("nav.upload"), iconos.Subir, func() { a.Ir(RutaSubir) })
		capa := container.New(ui.CapaFlotante{}, fab)
		// Con el cajón abierto no se ve: el velo del cajón tapa la página, y el botón
		// quedaba encima de los dos (visto en el emulador).
		arm.AlCajon = func(abierto bool) {
			if abierto {
				capa.Hide()
			} else {
				capa.Show()
			}
		}
		a.raiz.Objects = append(a.raiz.Objects, capa)
	}
	a.raiz.Objects = append(a.raiz.Objects, a.avisos)
	a.raiz.Refresh()
	a.actualizarEncendido()
}

// avisar es el `toast.notify` de la web: un aviso abajo que se va solo (el de peligro
// se queda hasta cerrarlo).
func (a *App) avisar(tono ui.TonoAviso, titulo string) {
	a.avisos.Notificar(ui.NuevoAviso(tono, titulo, ""))
}

// medirVentana ocupa todo y avisa a la app cuando el ancho cruza un corte: las rejillas
// cambian de columnas y hay que reconstruir.
type medirVentana struct{ a *App }

func (m *medirVentana) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (m *medirVentana) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
	a := m.a
	// La ventana es el contenido más el carril: lo que miden los cortes es la ventana.
	ventana := t.Width
	if a.w != nil {
		ventana = a.w.Canvas().Size().Width
		a.orientacion(a.w.Canvas().Size())
	}
	if ventana <= 0 || ventana == a.v.Ventana {
		return
	}
	antes := a.v.Ventana
	a.v.Ventana = ventana
	if ColumnasRejilla(ventana) != ColumnasRejilla(antes) || (ventana >= ui.CorteLG) != (antes >= ui.CorteLG) ||
		(ventana >= ui.CorteSM) != (antes >= ui.CorteSM) {
		go fyne.Do(a.pintar)
		return
	}
	go fyne.Do(a.scroll.Refresh)
}

// ---- Inicio ----

func (a *App) pedirInicio() {
	if !a.dentro() || a.cli == nil {
		return
	}
	a.ini.cargando, a.ini.fallo = true, nil
	gen, q := a.gen, strings.TrimSpace(a.buscaInicio.Texto())
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		// La búsqueda va al servicio (`q`) y solo acota lo mío, como en la web.
		mios, err := a.cli.Videos(c, api.CastMine, "", q, "", 24)
		var org, vivos, comp api.CastPage
		if err == nil {
			org, err = a.cli.Videos(c, api.CastOrgScope, api.CastStatusReady, "", "", 12)
		}
		if err == nil {
			vivos, _ = a.cli.Videos(c, api.CastOrgScope, api.CastStatusLive, "", "", 8)
			comp, _ = a.cli.Shared(c, "")
		}
		a.hilo(gen, func() {
			a.ini.cargando, a.ini.fallo, a.ini.pedidos = false, err, a.ini.pedidos || err == nil
			if err == nil {
				a.ini.d.Mios, a.ini.d.EnDirecto, a.ini.d.Compartidos = mios.Videos, vivos.Videos, comp.Videos
				a.ini.d.Organizacion = quitarDe(org.Videos, mios.Videos, vivos.Videos)
				if len(a.ini.d.Organizacion) > 8 {
					a.ini.d.Organizacion = a.ini.d.Organizacion[:8]
				}
				a.ini.cursor = mios.NextCursor
				a.recordarPosters(mios.Videos, org.Videos, vivos.Videos, comp.Videos)
			}
			a.pintar()
		})
	})
}

// masMios pide la página siguiente de «Mis vídeos».
func (a *App) masMios() {
	if a.cli == nil || a.ini.cursor == "" || a.ini.masCargando {
		return
	}
	a.ini.masCargando = true
	a.pintar()
	gen, q, cursor := a.gen, strings.TrimSpace(a.buscaInicio.Texto()), a.ini.cursor
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		p, err := a.cli.Videos(c, api.CastMine, "", q, cursor, 24)
		a.hilo(gen, func() {
			a.ini.masCargando = false
			if err == nil {
				a.ini.d.Mios = append(a.ini.d.Mios, p.Videos...)
				a.ini.cursor = p.NextCursor
				a.recordarPosters(p.Videos)
			}
			a.pintar()
		})
	})
}

// hayVivoEnInicio: algo de Inicio cambia solo (se procesa, se emite): se sondea.
func (a *App) hayVivoEnInicio() bool {
	for _, v := range a.ini.d.Mios {
		switch v.Status {
		case api.CastStatusUploading, api.CastStatusProcessing, api.CastStatusLive, api.CastStatusScheduled:
			return true
		}
	}
	return len(a.ini.d.EnDirecto) > 0
}

// sondear repite `f` cada `cada` mientras se siga en la misma pantalla y `hace`
// diga que hace falta (useSondeo de la web).
func (a *App) sondear(cada time.Duration, hace func() bool, f func()) {
	gen := a.gen
	go func() {
		t := time.NewTicker(cada)
		defer t.Stop()
		for range t.C {
			seguir := make(chan bool, 1)
			a.enLaInterfaz(func() {
				if gen != a.gen {
					seguir <- false
					return
				}
				if hace() {
					f()
				}
				seguir <- true
			})
			if !<-seguir {
				return
			}
		}
	}()
}

// quitarDe deja de `vs` los que no están en ninguna de las otras listas: lo último de
// la organización sin repetir lo mío ni lo que está en directo.
func quitarDe(vs []api.CastVideo, otras ...[]api.CastVideo) []api.CastVideo {
	visto := map[string]bool{}
	for _, l := range otras {
		for _, x := range l {
			visto[x.ID] = true
		}
	}
	var out []api.CastVideo
	for _, x := range vs {
		if !visto[x.ID] {
			out = append(out, x)
		}
	}
	return out
}

func (a *App) pantallaInicio() fyne.CanvasObject {
	if !a.dentro() {
		return PantallaInicio(a.sinSesion(), Inicio{})
	}
	d := a.ini.d
	d.Busqueda, d.Texto = a.buscaInicio, strings.TrimSpace(a.buscaInicio.Texto())
	d.HayMas, d.CargandoMas, d.AlMas = a.ini.cursor != "", a.ini.masCargando, a.masMios
	vacia := d.Texto == "" && len(d.Mios)+len(d.EnDirecto)+len(d.Compartidos)+len(d.Organizacion) == 0
	v := a.vistaDe(a.ini.estadoCarga, !a.ini.pedidos, a.ini.pedidos && vacia, a.pedirInicio)
	d.AlAbrir = a.abrir
	d.AlSubir = func() { a.Ir(RutaSubir) }
	d.AlEmitir = func() { a.Ir(RutaEmitir) }
	return PantallaInicio(v, d)
}

// ---- Explorar ----

func (a *App) pedirExplorar(mas bool) {
	if a.cli == nil {
		return
	}
	if !mas {
		a.exp = estadoExplorar{}
	}
	a.exp.cargando, a.exp.fallo = true, nil
	gen, cursor := a.gen, a.exp.cursor
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		e, err := a.cli.Explore(c, cursor, 24)
		a.hilo(gen, func() {
			a.exp.cargando, a.exp.fallo = false, err
			if err == nil {
				a.exp.pedidos = true
				if !mas {
					a.exp.directos = e.Live
				}
				a.exp.videos = append(a.exp.videos, e.Videos...)
				a.exp.cursor, a.exp.mas = e.NextCursor, e.NextCursor != ""
				a.recordarPosters(e.Live, e.Videos)
			}
			a.pintar()
		})
	})
}

func (a *App) pantallaExplorar() fyne.CanvasObject {
	e := a.exp
	vacia := len(e.directos)+len(e.videos) == 0
	v := a.vistaDe(e.estadoCarga, !e.pedidos, e.pedidos && vacia, func() { a.pedirExplorar(false) })
	return PantallaExplorar(v, Explorar{
		Directos: e.directos, Videos: e.videos, Buscador: a.buscador, Texto: a.buscador.Texto(),
		HayMas: e.mas, CargandoMas: e.cargando && e.pedidos,
		AlMas: func() { a.pedirExplorar(true) }, AlAbrir: a.abrir,
	})
}

// ---- Canal ----

// AbrirCanal enseña el canal público de una organización.
func (a *App) AbrirCanal(org string) {
	a.apuntar(lugar{ruta: RutaCanal, param: org})
	a.salirDeVer()
	a.ruta = RutaCanal
	a.gen++
	a.scroll.Offset = fyne.NewPos(0, 0)
	a.can = estadoCanal{org: org}
	a.pedirCanal(false)
	a.pintar()
}

func (a *App) pedirCanal(mas bool) {
	if a.cli == nil {
		return
	}
	a.can.cargando, a.can.fallo = true, nil
	gen, org, cursor := a.gen, a.can.org, ""
	if mas {
		cursor = a.can.canal.NextCursor
	}
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		ch, err := a.cli.Channel(c, org, cursor)
		a.hilo(gen, func() {
			a.can.cargando, a.can.fallo = false, err
			if err == nil {
				a.can.pedidos = true
				previos := a.can.canal.Videos
				a.can.canal = ch
				if mas {
					a.can.canal.Videos = slices.Concat(previos, ch.Videos)
				}
				a.can.mas = ch.NextCursor != ""
				a.recordarPosters(ch.Videos)
			}
			a.pintar()
		})
	})
}

func (a *App) pantallaCanal() fyne.CanvasObject {
	c := a.can
	v := a.vistaDe(c.estadoCarga, !c.pedidos, c.pedidos && len(c.canal.Videos) == 0, func() { a.pedirCanal(false) })
	return PantallaCanal(v, Canal{
		Org: c.org, Nombre: c.canal.Name, Videos: c.canal.Videos, HayMas: c.mas, CargandoMas: c.cargando && c.pedidos,
		AlMas: func() { a.pedirCanal(true) }, AlAbrir: a.abrir,
		AlDenunciar: func() { a.AbrirDenuncia(api.CastNewReport{Kind: api.CastReportChannel, Org: c.org}, "") },
	})
}

// ---- Los pósters ----

// recordarPosters apunta de dónde se pide el póster de cada vídeo: la clave de la caché
// es el id (la ruta lleva un token que cambia en cada respuesta).
func (a *App) recordarPosters(listas ...[]api.CastVideo) {
	for _, l := range listas {
		for _, x := range l {
			if x.PosterURL != "" {
				a.posters.Store(x.ID, x.PosterURL)
			}
		}
	}
}

// PedirPoster es la puerta de la caché de pósters a la red.
func (a *App) PedirPoster(c context.Context, id, _ string) ([]byte, error) {
	ruta, ok := a.posters.Load(id)
	if !ok || a.cli == nil {
		return nil, errors.New("cast: póster desconocido")
	}
	s, _ := ruta.(string)
	return a.cli.Poster(c, s)
}

// abrir es lo que hace una tarjeta.
func (a *App) abrir(x api.CastVideo) { a.AbrirVideo(x.ID) }

// ---- El teclado y soltar ----

// tecla recibe lo que se pulsa sin nada enfocado: en la página de un vídeo, los atajos
// del reproductor (espacio, ←/→ 5 s, F, M), como la web.
func (a *App) tecla(e *fyne.KeyEvent) {
	if a.ruta != RutaVer || (a.w != nil && a.w.Canvas().Overlays().Top() != nil) {
		return
	}
	r := a.deVer()
	switch e.Name {
	case fyne.KeySpace:
		if r != nil {
			r.Alternar()
		}
	case fyne.KeyLeft:
		if r != nil {
			r.Buscar(max(r.Posicion()-5*time.Second, 0))
		}
	case fyne.KeyRight:
		if r != nil {
			r.Buscar(r.Posicion() + 5*time.Second)
		}
	case fyne.KeyF:
		a.pantallaCompleta()
	case fyne.KeyM:
		a.alternarSilencio()
	case fyne.KeyP:
		a.alternarPiP()
	default:
		return
	}
	if a.visor != nil {
		a.visor.Rehacer()
	}
}

func (a *App) pantallaCompleta() {
	if a.Movil {
		a.alternarAmpliado()
		return
	}
	if a.w != nil {
		a.w.SetFullScreen(!a.w.FullScreen())
	}
}

func (a *App) alternarSilencio() {
	if r := a.deVer(); r != nil {
		r.Silenciar(!r.Silenciado())
	}
	if a.visor != nil {
		a.visor.Rehacer()
	}
}

// soltado: soltar ficheros encima de la ventana los pone en la cola de subida, desde
// cualquier pantalla (la zona de soltar de la web, que en la app es la ventana entera).
func (a *App) soltado(_ fyne.Position, us []fyne.URI) {
	if len(us) == 0 || !a.dentro() {
		return
	}
	rutas := make([]string, 0, len(us))
	for _, u := range us {
		rutas = append(rutas, u.Path())
	}
	if a.ruta != RutaSubir {
		a.Ir(RutaSubir)
	}
	a.anadir(rutas)
}

// esVideo mira la extensión: lo que el transcodificador sabe leer.
func esVideo(nombre string) bool {
	n := strings.ToLower(nombre)
	for _, e := range []string{".mp4", ".m4v", ".mov", ".mkv", ".webm", ".avi", ".mpg", ".mpeg", ".ogv", ".3gp"} {
		if strings.HasSuffix(n, e) {
			return true
		}
	}
	return false
}

// AbrirEnlace atiende una dirección de la web de Cast o `sytue-cast:`: `/watch/<id>`,
// `/manage/<id>`, `/c/<org>`, `/explore`, `/studio`, `/upload` y `/moderation` (a donde
// lleva el aviso de una denuncia). Dice si la entendió.
func (a *App) AbrirEnlace(u *url.URL) bool {
	partes := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(partes) > 0 && partes[0] == "cast" {
		partes = partes[1:]
	}
	if len(partes) == 0 || partes[0] == "" {
		a.Ir(RutaInicio)
		return true
	}
	// La forma canónica lleva la organización detrás del prefijo (ADR 0056).
	if len(partes) > 1 && !esRutaDeCast(partes[0]) && esRutaDeCast(partes[1]) {
		partes = partes[1:]
	}
	if r, hay := rutasSinParametro[partes[0]]; hay {
		a.Ir(r)
		return true
	}
	switch {
	case (partes[0] == "watch" || partes[0] == "embed") && len(partes) > 1:
		a.AbrirVideo(partes[1])
	case partes[0] == "manage" && len(partes) > 1:
		a.AbrirGestionar(partes[1])
	case partes[0] == "c" && len(partes) > 1:
		a.AbrirCanal(partes[1])
	case partes[0] == "upload" && u.Query().Has("drive"):
		a.AbrirDesdeDrive(u.Query().Get("drive"))
	case partes[0] == "upload":
		a.Ir(RutaSubir)
	default:
		return false
	}
	return true
}

// rutasSinParametro son las páginas que se abren solo con su nombre.
var rutasSinParametro = map[string]Ruta{"explore": RutaExplorar, "studio": RutaEmitir, "moderation": RutaModeracion}

func esRutaDeCast(s string) bool {
	switch s {
	case "watch", "embed", "manage", "c", "explore", "studio", "upload", "moderation":
		return true
	}
	return false
}

// Repintar vuelve a pedir el pintado de lo que hay, sin reconstruir: lo que llama la
// caché de pósters cuando llega uno.
func (a *App) Repintar() {
	if a.scroll.Content != nil {
		a.scroll.Content.Refresh()
	}
}

// Recargar vuelve a pedir la pantalla actual: al entrar o salir de la sesión, lo que
// había era de otra persona (o de nadie).
func (a *App) Recargar() {
	// El contador de denuncias es de quien acaba de entrar (o de nadie).
	a.v.Denuncias = 0
	a.pedirAbiertas()
	switch a.ruta {
	case RutaVer:
		a.AbrirVideo(a.ver.id)
	case RutaGestionar:
		a.AbrirGestionar(a.ges.id)
	case RutaCanal:
		a.AbrirCanal(a.can.org)
	default:
		a.Ir(a.ruta)
	}
}

// EsEnlaceDeCast dice si una dirección es de esta app: `sytue-cast:` o una página de
// la web de Cast que la app sabe enseñar.
func EsEnlaceDeCast(u *url.URL) bool {
	if u.Scheme != api.Esquema("cast") && !strings.HasPrefix(u.Host, "cast.") {
		return false
	}
	partes := strings.Split(strings.Trim(u.Path, "/"), "/")
	for _, p := range partes {
		if esRutaDeCast(p) {
			return true
		}
	}
	return len(partes) <= 1
}

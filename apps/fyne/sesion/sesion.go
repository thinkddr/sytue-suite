// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package sesion es lo TRANSVERSAL de una app nativa de la suite: quién ha entrado, con
// qué apariencia, en qué idioma y cómo aparece ante los demás.
//
// Trece binarios instalados y UN inicio de sesión: quien entra en Sytue Mail no vuelve a
// entrar en Drive. Cada `cmd/` monta esto en una línea y se lo pasa a sus clientes de
// API; el menú de cuenta enchufa aquí «quién ha iniciado sesión», «cerrar sesión», el
// tema, el idioma y la presencia.
//
// # Dónde vive cada cosa, y por qué son tres sitios (ADR 0050)
//
//   - La SESIÓN en el llavero del sistema (apps/nucleo/auth): es un secreto. Una entrada
//     para las trece apps, y por eso cerrarla en una la cierra en todas.
//   - La APARIENCIA y el IDIOMA en la carpeta compartida (apps/nucleo/ajustes): no son
//     secretos, y meterlos en el llavero haría que abrir una app pidiera permiso en
//     macOS para saber si va en oscuro, que es absurdo. Si el llavero no está, la app
//     abre igual con el tema elegido.
//   - La PRESENCIA en el servidor (api.Hub): la ven los demás. Una copia local sería una
//     segunda verdad que se queda vieja en cuanto se cambia desde la web o el teléfono.
//
// # Cómo se enteran las otras doce
//
// Un solo mecanismo para lo local: `ajustes.Vigilar` mira cada dos segundos si el
// fichero compartido ha cambiado. Con el tema, se aplica en caliente. Con la sesión, el
// fichero solo lleva una MARCA y quien la ve vuelve a preguntarle al llavero.
//
// El idioma NO se aplica en caliente: las pantallas reciben su traductor al construirse
// y rehacerlas enteras sería reabrir la app por dentro. Se aplica al siguiente arranque,
// que es lo que hace también el escritorio de cualquier sistema. Cerrar sesión sí es
// inmediato: eso no admite «en el siguiente arranque».
//
// La presencia no pasa por el fichero: se le pregunta al Hub en el mismo latido que la
// mantiene viva. El Hub NO la empuja por su flujo SSE —solo manda `ready` y
// `notification`, ver `services/hub/internal/http/stream.go`—, así que hasta que lo haga
// el retardo es el del latido.
package sesion

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/documentos"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// cadaVistazo es cada cuánto se mira si otra app ha cambiado algo. Dos lecturas de menos
// de cien bytes: al lado de lo que cuesta pintar un fotograma no se nota, y dos segundos
// es lo que tarda alguien en mirar la otra ventana después de cambiar el tema.
const cadaVistazo = 2 * time.Second

// oidcDe es el cliente de Sytue ID de la suite en `apex`, sin navegador que abrir (lo
// pone Abrir). El client_id va con el CAMINO DE VUELTA, no con el producto: el móvil
// vuelve por el esquema propio de cada app (`sytue-mobile`, migración 0071) y el
// escritorio a un servidor efímero del equipo (`sytue-desktop`, 0069). Se decide al
// abrir y no en Entrar porque el refresco lo usa igual: renovar con el client_id del
// escritorio un token emitido al móvil sale 401 cuando ya nadie está mirando. También lo
// usa el refresco sin ventana de Android (fondo_sinventana.go), que solo renueva.
func oidcDe(apex string, movil bool) *auth.OIDC {
	o := &auth.OIDC{Issuer: api.ServiceURL(apex, "id")}
	if movil {
		o.Cliente = auth.ClientIDMovil
	}
	return o
}

// Suite es lo transversal de esta app. El cero no vale: se monta con Abrir.
type Suite struct {
	// Sesion entrega un access token vigente. Es lo que se le pasa a los clientes de
	// `api`, que lo esperan como `api.Session`.
	Sesion *auth.Session
	// Hub es el cliente de Sytue Hub, que es quien sabe de presencia.
	Hub *api.Hub

	app             fyne.App
	apex            string
	oidc            *auth.OIDC
	almacen         *auth.AlmacenSesion
	reidentificando atomic.Bool

	mu               sync.Mutex
	visto            ajustes.Estado
	generacionSesion string
	// epocaSesion es la época de identidad adoptada con generacionSesion
	// (ajustes.MarcarIdentidad): solo cambia cuando cambia quién, no al renovar.
	epocaSesion string
	// widgets es lo de los widgets nativos de esta app (widgets.go).
	widgets widgetsSuite
	// despertar es el registro de esta app en el Hub para el aviso silencioso de iOS
	// (despertar.go).
	despertar         despertarSuite
	presencia         api.EstadoPresencia
	mensaje           string
	alCambiar         []func(Estado)
	alAbrirURL        []func(*url.URL)
	alBuscar          func()
	busquedaPendiente bool
	consentimiento    *autorizacionApp
	matrixConsent     *matrixAuthorization
	// alVolver y alFondo son los apuntados a la vida de la app (primerplano.go).
	alVolver, alFondo []func()
	sinDueno          []*url.URL // llegaron antes de Arrancar; los recibe cada receptor que se apunte hasta entonces
	// alFallarLocal lo pone AceptarFicheros; nil es «esta app no abre ficheros sueltos».
	alFallarLocal  func()
	ventanaLocal   fyne.Window
	confirmarLocal func(w fyne.Window, nombre string, aceptar func(recordar bool))
	// localesAntes son los ficheros de la máquina que llegaron antes de AceptarFicheros:
	// al arrancar con uno, AlAbrirFichero se apunta primero y los recibe cuando aún no
	// se sabe si esta app los abre.
	localesAntes []func()
	// propias son las rutas que la app atiende con su propio AlAbrirURL (RutaPropia):
	// AlAbrirFichero no las toma por un destino que no existe.
	propias []func(*url.URL) bool
	// noEncontrado enseña la página «no encontrada» (noencontrado.go); arrancada dice si
	// el `main` ya ha registrado a sus receptores, y con ello si un enlace sin dueño es
	// uno que esta app no reconoce.
	noEncontrado func()
	arrancada    bool
	haciaDonde   time.Duration // cada cuánto pide el Hub que se lata; 0 aún sin saberlo
	// orgs y reentrar son los dobles de Sytue ID y del navegador para probar el cambio de
	// organización (organizacion.go); nil, los de verdad.
	orgs     organizador
	reentrar func(context.Context) error
	// entrando dice que hay un Entrar esperando al navegador, y autorizando es la URL
	// que le abrió: pulsar «Iniciar sesión» otra vez la vuelve a abrir en vez de pedir
	// un segundo login, que no podría escuchar en el mismo puerto de vuelta.
	entrando    bool
	autorizando string

	tareasAlmacenMu  sync.Mutex
	tareasAlmacen    map[uint64]context.CancelCauseFunc
	tareasAlmacenSeq uint64
	tareasAlmacenWG  sync.WaitGroup
	almacenCerrando  bool

	// alCambiarSiempre son los AlCambiar de la propia suite que siguen a la sesión
	// también sin escena (el widget); los de alCambiar esperan a la escena (fondo.go).
	alCambiarSiempre []func(Estado)
	// fondo es la API de fondo del driver (fondo.go); nil, la de siempre con escena.
	fondo conductorFondo
	// sinEscena: iOS lanzó la app sin interfaz y aún no hay escena. cambioRetenido: la
	// sesión cambió mientras tanto y las apps no se han enterado.
	sinEscena, cambioRetenido bool
	// trasEscena es lo de CuandoHayaEscena que espera a la escena, en orden.
	trasEscena []func()
	// bloqueoPendiente: la plataforma puede lanzar sin escena (iOS) y el bloqueo de la
	// suite aún no se ha comprobado en este proceso; se comprueba al conectar la escena
	// (fondo.go). verificando: se está pidiendo ahora, y las URL que llegan esperan en
	// urlsRetenidas.
	bloqueoPendiente, verificando bool
	urlsRetenidas                 []*url.URL
	// retenida: con el bloqueo puesto y sin pasar, la sesión guardada no se adopta.
	retenida atomic.Bool
	// abierta se cierra al acabar Abrir; nil en las suites que las pruebas montan a mano.
	abierta chan struct{}
	// ilegibleAlAbrir: Abrir no pudo leer la sesión guardada (el llavero falló; en iOS,
	// lanzada con el teléfono bloqueado). Estar «fuera» entonces no es saber que no hay
	// nadie, y el primer aviso del widget no borra nada (iniciarWidgets).
	ilegibleAlAbrir bool
	// aplicando serializa aplicar: el vigilante y el refresco de fondo pueden llamarlo a
	// la vez, y sin esto la lectura más vieja de la sesión podía ganar (fondo.go).
	aplicando sync.Mutex
	// pausaEnFondo: en esta plataforma lo que dice que hay alguien —la presencia, el SSE
	// de la barra— se para al dejar el primer plano (iOS: la app suspendida puede volver
	// a correr para su widget). enFondo lo lleva repartir, y vuelta despierta a latir al
	// volver (primerplano.go).
	pausaEnFondo bool
	enFondo      atomic.Bool
	vuelta       chan struct{}
}

// Estado es lo que el menú de cuenta enseña.
type Estado struct {
	// Sujeto es el `sub` de quien ha entrado, o "" si no hay nadie.
	Sujeto string
	// GeneracionSesion identifica la sesión adoptada por esta app. No contiene
	// tokens; cambia en cada renovación.
	GeneracionSesion string
	// EpocaSesion es la época de identidad adoptada: cambia solo cuando cambia quién.
	// Es lo que captura el widget antes de leer (capturarWidget).
	EpocaSesion string
	// Tema es "claro", "oscuro", "traslucido" o "" (el del sistema).
	Tema string
	// Color es el color de la suite EN VIGOR: el que eligió la persona o, si no ha
	// elegido, el de su organización; sin ambos, el de cada app.
	Color string
	// Forma es la forma en vigor, igual: "carril", o "" y "marco" para la de la casa.
	Forma string
	// ColorPropio y FormaPropia son lo que eligió LA PERSONA; "" es que no ha elegido y
	// sigue a su organización. ConOrganizacion dice si hay organización a la que seguir:
	// una cuenta personal no tiene quien fije nada, y sin sesión tampoco.
	ColorPropio, FormaPropia string
	ConOrganizacion          bool
	// ColorOrg es el color que fija la organización; sin elegir, el de cada app. Es el de la
	// muestra de «Organización» en el selector.
	ColorOrg string
	// Idioma es la etiqueta elegida, o "" (el del sistema).
	Idioma string
	// Presencia es la última que dijo el Hub.
	Presencia api.EstadoPresencia
	// Mensaje es el texto de estado que la acompaña.
	Mensaje string
}

// Dentro dice si hay sesión.
func (e Estado) Dentro() bool { return e.Sujeto != "" }

// Abrir monta lo transversal y aplica el tema elegido ANTES de que se pinte nada. No
// habla con la red y no abre ningún navegador: una app tiene que poder arrancar sin
// conexión y sin sesión. Para entrar, Entrar.
func Abrir(a fyne.App, apex string) *Suite {
	if apex == "" {
		apex = api.ApexProd
	}
	prepararCasa() // en Android, antes de leer nada de disco
	ponerSoloLectura()
	estado := ajustes.Leer()
	sinGraficos(a, estado)
	tema.Elegir(estado.Tema)

	s := &Suite{app: a, apex: apex, visto: estado, abierta: make(chan struct{})}
	s.prepararFondo(conductorDe(a))
	s.engancharVida(a)
	s.oidc = oidcDe(apex, a.Driver().Device().IsMobile())
	s.oidc.Open = func(destino string) error {
		s.mu.Lock()
		s.autorizando = destino
		s.mu.Unlock()
		return abrir(a, destino)
	}
	if receptor, ok := a.(fyne.URLHandler); ok {
		receptor.SetOnOpenURL(s.recibirURL)
	} else {
		// Solo sucede con un Fyne anterior al fork de Sytue. No se impide abrir la
		// app, pero el login por esquema y los enlaces entrantes no pueden funcionar.
		slog.Warn("sesion: este Fyne no recibe URL entrante; hace falta el fork de Sytue")
	}
	// Y si esta app YA está abierta, el enlace es suyo: se le pasa y este proceso se va
	// sin pintar nada. Sale aquí, antes de que nadie cree una ventana o un cliente.
	// Solo en escritorio y solo con identificador de paquete: en móvil lo resuelve el
	// sistema, y sin identificador —las pruebas— no hay enchufe que valga.
	// El `os.Exit` está aquí y no en cada `main` a propósito: son trece binarios y
	// trece copias de esto es justo lo que §13.2 dice que no se hace. Y tiene que ser
	// un exit: la otra instancia ya se ha quedado el enlace, así que este proceso no
	// tiene nada que pintar.
	if id := a.Metadata().ID; id != "" && !a.Driver().Device().IsMobile() && reenviar(id, os.Args[1:]) {
		os.Exit(0) //nolint:revive // deep-exit deliberado: ver el comentario de arriba
	}
	// Una ruta PELADA en los argumentos —la de «Abrir con» en Linux, o la de quien lanza
	// la app a mano con un fichero— no la entrega el fork, que solo pasa lo que trae
	// esquema. Se entrega aquí, como `file://`, y espera a su receptor igual que un enlace.
	for _, arg := range os.Args[1:] {
		if u, err := url.Parse(arg); err == nil && u.Scheme != "" {
			continue // lo ha entregado ya el fork
		}
		if crudo, ok := ficheroDeArg(arg); ok {
			if u, err := url.Parse(crudo); err == nil {
				s.recibirURL(u)
			}
		}
	}
	// Con el bloqueo puesto, la sesión guardada no se abre sin que el dueño diga quién
	// es; si no lo dice, la app no abre (entrar otra vez por el navegador sería saltarse
	// el bloqueo con la sesión que el navegador tenga con Sytue ID). Va después del
	// reenvío: la copia que ya estaba abierta ya se desbloqueó.
	//
	// Donde la app puede lanzarse sin escena (iOS) no se pregunta aquí: sin interfaz no
	// hay Face ID que pedir, y con el teléfono bloqueado ni siquiera se lee si el bloqueo
	// está puesto. Se pregunta al conectar la escena (fondo.go) y, hasta entonces, con el
	// bloqueo puesto la sesión guardada no se adopta.
	pedirSytueID := false
	if s.bloqueoSinComprobar() {
		s.retenerSiBloqueada(estado)
	} else {
		pedirSytueID = pedirSytueIDAlAbrir(estado)
	}
	// En un Linux sin llavero la sesión va a un fichero cifrado (apps/nucleo/auth): se
	// dice una vez, al guardarla por primera vez, y con qué protegerla mejor.
	auth.SinLlavero = func() {
		t, err := i18n.New(i18n.Negotiate(sistema.Idioma(), estado.Idioma), i18n.Common())
		if err == nil {
			s.avisarSistema(t.T("shell.keyring.missing", nil), t.T("shell.keyring.missingBody", nil), nil)
		}
	}
	s.almacen = s.oidc.Almacen()
	// El formato anterior no guardaba emisor/cliente. Sólo producción y el
	// cliente histórico de esta plataforma pueden importar esa entrada.
	if err := s.almacen.Migrar(context.Background(), apex == api.ApexProd); err != nil {
		slog.Warn("sesion: no se pudo migrar la sesión guardada", "error", err)
	}
	guardados, marcas, err := s.leerSesion(context.Background())
	s.generacionSesion, s.epocaSesion = marcas.generacion, marcas.epoca
	s.ilegibleAlAbrir = err != nil
	if err != nil {
		// Sin llavero se puede trabajar: lo que no se puede es recordar la sesión.
		slog.Warn("sesion: el almacén seguro no se deja leer; habrá que entrar", "error", err)
	}
	if pedirSytueID {
		s.reidentificando.Store(true)
		guardados = auth.Tokens{}
		s.prepararReidentificacion(estado)
	}
	s.Sesion = auth.NewSession(guardados, auth.Compartida(s.oidc.Renew, s.cargarSesion, s.almacen.Guardar), s.almacen.Guardar)
	// El color y la forma, ya con la sesión: lo que no eligió la persona lo dice su
	// organización, y eso viene guardado con ella. Antes de que se pinte nada.
	s.elegirApariencia(estado.Ajustes)
	s.Hub = api.NewHub(api.ServiceURL(apex, "hub"), s.Sesion, nil)
	// El aviso del sistema pulsado cuando la app que lo puso ya no vivía llega aquí.
	escritorio.AlSeguir(s.seguirAviso)
	// En Linux, un aviso pulsado con la app cerrada la despierta por D-Bus: su nombre de
	// bus se coge ya, que es a quien D-Bus entrega lo que la despertó.
	if id := a.Metadata().ID; id != "" && !a.Driver().Device().IsMobile() {
		if err := escritorio.RecibirAvisos(id); err != nil {
			slog.Info("sesion: los avisos pulsados con la app cerrada no la despertarán", "error", err)
		}
	}
	// Y en el móvil, `ficheros.Elegir` abre el selector del toolkit (elegir.go).
	s.ponerSelector()
	close(s.abierta) // ya se puede pedir el bloqueo al conectar la escena (fondo.go)
	return s
}

// seguirAviso lleva a su destino un aviso del sistema pulsado cuando la app que lo puso
// ya no estaba viva —un toast viejo, un aviso de Android tras matar el proceso—: lo mismo
// que pulsarlo en la campana (`ui/barraavisos.go`), marcarlo leído y abrir su página en
// la app de su producto. El host lo pone la suite, no el aviso.
//
//nolint:contextcheck // lo lanza un clic, no hay contexto de nadie que heredar
func (s *Suite) seguirAviso(d escritorio.Destino) {
	// El destino de un aviso viejo llega por un ENLACE (`escritorio.Pulsado`), y un enlace
	// lo escribe cualquiera: un origen que no es un producto no se sigue. Sin esto,
	// `api.ServiceURL` entraba en pánico y el enlace cerraba la app.
	if !api.EsProducto(d.Origen) {
		slog.Info("sesion: el aviso pulsado no es de ningún producto de la suite", "origen", d.Origen)
		return
	}
	if d.Aviso != "" {
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), 10*time.Second)
			defer parar()
			_ = s.Hub.MarkRead(ctx, d.Aviso) // perder un «visto» no quita el destino
		}()
	}
	web := api.ServiceURL(s.apex, d.Origen) + d.Ruta
	fyne.Do(func() {
		if err := api.AbrirURL(s.app, d.Origen, web); err != nil {
			slog.Warn("sesion: el aviso pulsado no ha podido abrir su destino", "error", err)
		}
	})
}

// URL es la raíz del servicio de un producto, para construir su cliente con esta sesión:
//
//	api.NewDrive(s.URL("drive"), s.Sesion, nil)
func (s *Suite) URL(producto string) string { return api.ServiceURL(s.apex, producto) }

// Apex es el dominio de la suite (`sytue.eu`), que es lo que piden `api.AbrirEn` y
// `api.AppURL` para llevar a la WEB de otro producto. Se publica porque la alternativa
// era recortarle el subdominio a `URL("one")`, y eso deja de valer en cuanto un entorno
// no reparta los productos como subdominios del apex.
func (s *Suite) Apex() string { return s.apex }

// Organizacion es la organización activa de la sesión, o "" sin sesión: la que llevan
// los enlaces a otras apps como segmento del path (ADR 0056). Es nil-safe a propósito —
// se compone enlaces también en las pruebas, donde no hay sesión—.
func (s *Suite) Organizacion() string {
	if s.Sesion == nil {
		return ""
	}
	return s.Sesion.Organizacion()
}

// Idioma es el que hay que usar para construir las pantallas: el elegido en el menú de
// cuenta y, si no hay ninguno, el del sistema operativo.
func (s *Suite) Idioma() i18n.Locale {
	s.mu.Lock()
	elegido := s.visto.Idioma
	s.mu.Unlock()
	// `SYTUE_IDIOMA` por encima de todo: sirve para mirar una pantalla en otro idioma
	// sin cambiarle la preferencia a nadie. Estaba en `cmd/diapos` y solo ahí; aquí
	// vale para los trece binarios y se escribe una vez.
	if v := os.Getenv("SYTUE_IDIOMA"); v != "" {
		elegido = v
	}
	// El segundo argumento de Negotiate es justo esto: lo guardado manda sobre lo que
	// diga el sistema, y si no vale (o no hay), se negocia con el sistema.
	return i18n.Negotiate(sistema.Idioma(), elegido)
}

// Estado es lo que hay ahora mismo.
func (s *Suite) Estado() Estado {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.estado()
}

func (s *Suite) estado() Estado {
	color, forma := s.apariencia(s.visto.Ajustes)
	colorOrg, _ := s.Sesion.Apariencia()
	return Estado{
		Sujeto:           s.Sesion.Subject(),
		GeneracionSesion: s.generacionSesion,
		EpocaSesion:      s.epocaSesion,
		Tema:             s.visto.Tema,
		Color:            color,
		Forma:            forma,
		ColorPropio:      s.visto.Color,
		FormaPropia:      s.visto.Forma,
		ConOrganizacion:  s.Sesion.Subject() != "" && !s.Sesion.EsCuentaPersonal(),
		ColorOrg:         cmp.Or(colorOrg, tokens.ColorDeCadaApp),
		Idioma:           s.visto.Idioma,
		Presencia:        s.presencia,
		Mensaje:          s.mensaje,
	}
}

// AlCambiar apunta a quien quiera enterarse de que algo transversal ha cambiado —aquí o
// en otra de las doce apps—. Se le llama SIEMPRE en el hilo de la interfaz.
func (s *Suite) AlCambiar(f func(Estado)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alCambiar = append(s.alCambiar, f)
}

// AlAbrirURL registra quién maneja un enlace que el sistema haya abierto en esta
// aplicación. La vuelta OIDC en curso se consume antes de llamar a estos handlers:
// nunca se confunde un código de autorización con un deep link de producto.
//
// El callback se ejecuta en el hilo de la interfaz. El registro es para que el
// binario dueño de un fichero pueda recibir su ruta o identificador sin que la
// sesión sepa qué producto es.
//
// Lo que llegó ANTES de Arrancar se le entrega a CADA receptor que se apunte hasta
// entonces. No es un caso raro, es el normal: en escritorio la URL entra por la línea de
// órdenes, y Fyne la despacha dentro de Abrir —antes de arrancar, en el hilo principal,
// la ejecuta en el acto—, cuando el `main` todavía no ha tenido ocasión de llamar aquí.
// Sin esto, pulsar un documento con la app cerrada abría la app y perdía el documento.
//
// A cada uno y no solo al primero porque Diapos, Calcus, Contactos y Photos registran
// dos —el de su ruta propia y el de AlAbrirFichero— y cada receptor mira si el enlace es
// suyo: dárselo solo al primero lo perdía cuando el primero no era el suyo. Lo que llega
// después de Arrancar va a los que ya están y a nadie más.
func (s *Suite) AlAbrirURL(f func(*url.URL)) {
	if f == nil {
		return
	}
	s.mu.Lock()
	s.alAbrirURL = append(s.alAbrirURL, f)
	pendientes := append([]*url.URL(nil), s.sinDueno...)
	s.mu.Unlock()
	for _, u := range pendientes {
		f(u)
	}
}

// AlAbrirFichero es AlAbrirURL para el caso que tienen las cinco apps que abren
// documentos de otras (`apps/nucleo/api/abrir.go`): Drive lanza
// `sytue-docs:/open-from-drive?file=…&volver=…` y quien lo recibe quiere la referencia
// desarmada, no el `*url.URL`.
//
// `cargar` trae el fichero y lo ENSEÑA. Corre en su propia goroutine —habla con la red,
// y el hilo de la interfaz no se bloquea—, así que lo que toque la pantalla lo envuelve
// en `fyne.Do`. Si devuelve error —no hay sesión, el fichero ya no está, el formato no
// se entiende—, o si todavía no hay con qué cargarlo y se pasa nil, el enlace sigue a
// la MISMA página en la web, que es lo que ya hacía Drive si la app no estuviera
// instalada. Es la diferencia entre acabar en el documento y acabar en una ventana en
// blanco, que es peor que no haber registrado el esquema.
func (s *Suite) AlAbrirFichero(producto string, cargar func(context.Context, api.Apertura) error) {
	s.AlAbrirURL(func(u *url.URL) {
		if uri, ok := ficheroLocal(u); ok {
			s.abrirLocal(producto, uri, cargar)
			return
		}
		ap, ok := api.LeerApertura(producto, u)
		if !ok {
			if !esRaiz(u) && !s.esPropia(u) {
				s.NoEncontrado() // una ruta que esta app no reconoce
			}
			return
		}
		ruta := u.EscapedPath()
		if u.RawQuery != "" {
			ruta += "?" + u.RawQuery
		}
		// La página web del respaldo: la org del ENLACE manda si la trae (dice a qué
		// organización apunta) y si no, la de la sesión; y una ruta que ya lleva el
		// prefijo es ya una página de la web y va tal cual (api.WebDeApertura).
		web := api.WebDeApertura(s.apex, producto, s.Organizacion(), ruta)
		if cargar == nil {
			s.abrirEnWeb(web, nil)
			return
		}
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), plazoApertura)
			defer parar()
			if err := cargar(ctx, ap); err != nil {
				// Si el servicio dice que NO ESTÁ, la web diría lo mismo: se dice aquí.
				if noEsta(err) && s.NoEncontrado() {
					return
				}
				// Al hilo de la interfaz: abrir el navegador en móvil es un Intent o
				// `UIApplication`, y esos no se llaman desde cualquier goroutine.
				fyne.Do(func() { s.abrirEnWeb(web, err) })
			}
		}()
	})
}

// plazoApertura es cuánto se espera a que un fichero llegue y se convierta antes de
// rendirse y mandarlo a la web. Veinticinco megas por una conexión mala caben; más que
// esto, quien pulsó ya ha pensado que no funciona.
const plazoApertura = 2 * time.Minute

// abrirEnWeb es el respaldo de una apertura: la misma página, en el navegador.
func (s *Suite) abrirEnWeb(web string, motivo error) {
	if motivo != nil {
		slog.Info("sesion: el fichero no se ha podido abrir aquí; sigue en la web", "motivo", motivo)
	}
	if err := abrir(s.app, web); err != nil {
		slog.Warn("sesion: el enlace entrante no se ha podido abrir ni en la web", "error", err)
	}
}

// Importar trae de Drive el fichero de una apertura y lo convierte al modelo de la
// suite, con la sesión de esta app. Es lo que Docus, Calcus y Diapos necesitan para
// enseñarlo; está aquí para que no lo escriban tres veces.
//
// Un fichero de la máquina (`ap.Local`) se lee de disco y va al MISMO conversor: el
// servicio no guarda nada, así que abrirlo no deja ninguna copia en la organización.
func (s *Suite) Importar(ctx context.Context, ap api.Apertura) (api.Importado, error) {
	return s.importar(ctx, ap, false)
}

// ImportarGuardando es Importar para quien va a CREAR el documento con lo convertido: trae
// también los comentarios del fichero, que hay que guardar con `api.Docs.ImportComments` antes
// de sembrar (ver `api.Docs.ImportarConComentarios`).
func (s *Suite) ImportarGuardando(ctx context.Context, ap api.Apertura) (api.Importado, error) {
	return s.importar(ctx, ap, true)
}

func (s *Suite) importar(ctx context.Context, ap api.Apertura, comentarios bool) (api.Importado, error) {
	docs := api.NewDocs(s.URL("docs"), s.Sesion, nil)
	if ap.Local != "" {
		nombre, datos, err := leerLocal(ap.Local)
		if err != nil {
			return api.Importado{}, err
		}
		if comentarios {
			return docs.ImportarConComentarios(ctx, nombre, datos)
		}
		return docs.Importar(ctx, nombre, datos)
	}
	drive := api.NewDrive(s.URL("drive"), s.Sesion, nil)
	if comentarios {
		return api.ImportarDeDriveConComentarios(ctx, drive, docs, ap.Ref.ID)
	}
	return api.ImportarDeDrive(ctx, drive, docs, ap.Ref.ID)
}

// finArranque cierra el reparto de lo que llegó antes de Arrancar —cada `main` apunta a
// sus receptores antes— y dice si algo de eso no lo iba a recoger nadie: un enlace a un
// destino que esta app no tiene.
func (s *Suite) finArranque() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.arrancada = true
	llegados := s.sinDueno
	s.sinDueno = nil
	if len(s.alAbrirURL) > 0 {
		return false // ya lo recibieron, y cada uno decidió si era suyo
	}
	for _, u := range llegados {
		if !esRaiz(u) {
			return true
		}
	}
	return false
}

func (s *Suite) recibirURL(u *url.URL) {
	// Mientras se pide Face ID al conectar la escena, nada llega a nadie (fondo.go).
	s.mu.Lock()
	if s.verificando {
		s.urlsRetenidas = append(s.urlsRetenidas, u)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	// Ni la vuelta del login ni el clic de un aviso (Windows lo devuelve por enlace) son
	// de ninguna pantalla.
	if s.recibirMatrix(u) || s.recibirConsentimiento(u) || s.oidc.Vuelta(u) || escritorio.Pulsado(u) {
		return
	}
	// «Abrir en la aplicación» de la web trae la PÁGINA entera (`open?url=`): se le da a
	// los receptores como la misma página vista por el esquema, que es lo que ya leen.
	if nativo, ok := api.DesdeLaWeb(s.apex, u); ok {
		u = nativo
	}
	if s.recibirBusqueda(u) {
		return
	}
	s.mu.Lock()
	quienes := append([]func(*url.URL){}, s.alAbrirURL...)
	desconocido := len(quienes) == 0 && s.arrancada && !esRaiz(u)
	if !s.arrancada {
		s.sinDueno = append(s.sinDueno, u)
	}
	s.mu.Unlock()
	if desconocido {
		// Nadie de esta app atiende enlaces y ya no va a apuntarse nadie: el destino
		// no existe aquí.
		s.NoEncontrado()
	}
	for _, f := range quienes {
		f(u)
	}
}

// avisar llama a los apuntados en el hilo de la interfaz. Se llama SIN el cerrojo.
func (s *Suite) avisar() { s.avisarA(true) }

// avisarApps avisa solo a los AlCambiar de las apps: los que esperaban a la escena.
func (s *Suite) avisarApps() { s.avisarA(false) }

// avisarA avisa con el estado de ahora. Sin escena, los de las apps quedan retenidos
// hasta conectarEscena (fondo.go).
func (s *Suite) avisarA(suite bool) {
	s.mu.Lock()
	e := s.estado()
	var quienes []func(Estado)
	if suite {
		quienes = append(quienes, s.alCambiarSiempre...)
	}
	if s.sinEscena {
		s.cambioRetenido = true
	} else {
		quienes = append(quienes, s.alCambiar...)
	}
	s.mu.Unlock()
	fyne.Do(func() {
		for _, f := range quienes {
			f(e)
		}
	})
}

// ErrYaEntrando es lo que devuelve Entrar cuando ya había uno esperando al navegador:
// el navegador se ha vuelto a abrir con la misma autorización, y es el primero el que
// recibe la vuelta.
var ErrYaEntrando = errors.New("sesion: ya hay un inicio de sesión abierto en el navegador")

// Entrar abre Sytue ID en el navegador del sistema y BLOQUEA hasta que se vuelve. Se
// llama desde una goroutine, nunca desde Layout. Si ya hay uno esperando, vuelve a
// abrir el navegador donde estaba y devuelve ErrYaEntrando.
//
// El navegador del SISTEMA y no una vista dentro de la app (RFC 8252 §8.1): así la
// contraseña se escribe en una ventana cuyo dominio y cuyo candado se ven, y si ya hay
// sesión con Sytue ID —porque otra app entró hace un rato— se reutiliza sin volver a
// pedirla.
func (s *Suite) Entrar(ctx context.Context) error {
	var (
		nuevos auth.Tokens
		err    error
	)
	s.mu.Lock()
	if s.entrando {
		destino := s.autorizando
		s.mu.Unlock()
		if destino == "" {
			return ErrYaEntrando
		}
		if err := s.oidc.Open(destino); err != nil {
			return err
		}
		return ErrYaEntrando
	}
	s.entrando = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.entrando, s.autorizando = false, ""
		s.mu.Unlock()
	}()
	if s.app.Driver().Device().IsMobile() {
		esquema := s.app.Metadata().Custom["esquema"]
		if esquema == "" {
			return errors.New("sesion: FyneApp.toml no declara Development.esquema")
		}
		nuevos, err = s.oidc.LoginEsquema(ctx, esquema+"://auth/callback")
	} else {
		nuevos, err = s.oidc.Login(ctx)
		if err == nil {
			aparecer(s.app)
		}
	}
	if err != nil {
		// Se deja escrito AQUÍ y no en cada llamada: ocho de las trece apps la lanzan
		// en una goroutine y tiran el error (`go func() { _ = s.Entrar(ctx) }()`), así
		// que un canje rechazado por Sytue ID dejaba la app en «Cargando…» sin rastro.
		// Visto en una ventana real el 2026-09-27.
		slog.Warn("sesion: no se ha podido entrar", "error", err)
		return err
	}
	// Adoptar, no sustituir: los clientes de `api` ya tienen este puntero.
	if err := s.Sesion.AdoptarCon(ctx, nuevos); err != nil {
		return err
	}
	// Sólo un login completado desbloquea la renovación y las marcas de otras apps.
	s.reidentificando.Store(false)
	s.prepararAvisos()
	guardados, marcas, errLectura := s.leerSesion(ctx)
	s.mu.Lock()
	if errLectura == nil {
		s.Sesion.Sincronizar(guardados)
		s.generacionSesion, s.epocaSesion = marcas.generacion, marcas.epoca
	}
	s.visto = ajustes.Leer()
	// La revisión corresponde a los tokens leídos, no a una marca posterior.
	s.visto.Sesion, s.visto.Epoca = s.generacionSesion, s.epocaSesion
	visto := s.visto.Ajustes
	s.mu.Unlock()
	s.repintar(visto) // la organización de quien entra trae su color y su forma
	s.avisar()
	return nil
}

// leerSesionCompartida obtiene identidad y marca de la misma escritura. El
// cerrojo se libera antes de tocar Suite/Session para mantener el orden de locks.
func (s *Suite) leerSesionCompartida(ctx context.Context) (auth.Tokens, string, error) {
	t, marcas, err := s.leerSesion(ctx)
	return t, marcas.generacion, err
}

// marcasSesion son la marca de sesión y la época de identidad leídas con los tokens.
type marcasSesion struct{ generacion, epoca string }

// leerSesion es leerSesionCompartida con la época. Si la sesión la escribió una
// compilación que no conoce la época (la época no está confirmada con la marca), la
// confirma o la rota aquí mismo, bajo el mismo cerrojo y con la identidad de los
// tokens leídos (ADR 0078, enmienda del 2026-11-06).
//
//nolint:contextcheck // El llavero usa callbacks sin contexto y un plazo propio para D-Bus.
func (s *Suite) leerSesion(ctx context.Context) (auth.Tokens, marcasSesion, error) {
	return leerSesionGuardada(ctx, s.cargarSesion, s.almacen, !s.sinSesionPropia())
}

// leerSesionGuardada es leerSesion sin Suite: lee con `cargar` bajo el cerrojo de la
// sesión y, con `confirmar`, confirma o rota la época como ella. La usa también el
// refresco sin ventana de Android (fondo_sinventana.go).
//
//nolint:contextcheck // El llavero usa callbacks sin contexto y un plazo propio para D-Bus.
func leerSesionGuardada(
	ctx context.Context, cargar func() (auth.Tokens, error), almacen *auth.AlmacenSesion, confirmar bool,
) (auth.Tokens, marcasSesion, error) {
	soltar, err := ajustes.CerrojoSesion(ctx)
	if err != nil {
		return auth.Tokens{}, marcasSesion{}, err
	}
	defer soltar()
	guardados, err := cargar()
	if err != nil {
		return auth.Tokens{}, marcasSesion{}, err
	}
	e := ajustes.Leer()
	if e.EpocaConfirmada != e.Sesion && confirmar {
		identidad := auth.IdentidadWidgets(guardados)
		if almacen != nil {
			identidad = almacen.IdentidadWidgets(guardados)
		}
		if err := ajustes.MarcarIdentidad(identidad); err != nil {
			slog.Warn("sesion: no se ha podido confirmar la época de los widgets", "error", err)
		}
		e = ajustes.Leer()
	}
	return guardados, marcasSesion{generacion: e.Sesion, epoca: e.Epoca}, nil
}

func (s *Suite) cargarSesion() (auth.Tokens, error) {
	if s.sinSesionPropia() {
		return auth.Tokens{}, nil
	}
	if s.almacen != nil {
		return cargarAlmacen(s.almacen)
	}
	return auth.Load()
}

// cargarAlmacen es AlmacenSesion.Cargar. Las pruebas lo hacen fallar como falla el
// llavero de iOS con el teléfono bloqueado: en Linux, sin bus de sesión, auth cae al
// fichero cifrado y nunca da ese error.
var cargarAlmacen = (*auth.AlmacenSesion).Cargar

// Salir cierra la sesión en LAS TRECE apps: borra la entrada compartida del llavero y
// deja la marca que hace que las otras doce lo vean en menos de dos segundos.
//
// Y cierra de verdad, no solo aquí: el refresh token se revoca en Sytue ID —borrado de
// este equipo seguía valiendo para quien tuviera una copia— y la caché de respuestas de
// trabajar sin conexión se vacía, que es lo que hace la web al salir.
func (s *Suite) Salir() {
	if err := s.SalirConError(); err != nil {
		slog.Warn("sesion: no se ha podido completar el cierre compartido", "error", err)
	}
}

// SalirConError permite avisar y reintentar si el dispositivo no dejó retirar
// la sesión compartida. El estado local nunca readopta los tokens no borrados.
func (s *Suite) SalirConError() error {
	// Los aparatos de iOS de las quince apps de aquí, que el Hub despertaba para su
	// widget: con la sesión que aún vale, antes de revocarla (despertar.go).
	s.olvidarDespertares(context.Background(), false)
	// Y el interruptor, que es el consentimiento de quien lo encendió y no del aparato:
	// quien entre después lo encuentra apagado, y nadie lo registra sin que lo pida.
	if a := ajustes.Leer(); a.DespertarWidgets || a.DespertarDe != "" {
		if err := ajustes.Cambiar(func(a *ajustes.Ajustes) { a.DespertarWidgets, a.DespertarDe = false, "" }); err != nil {
			slog.Warn("sesion: no se ha podido apagar el despertar de los widgets", "error", err)
		}
	}
	// La contraseña del proveedor de archivos de Drive, que sigue valiendo sin sesión. La
	// primera: su token puede necesitar el refresh que se revoca justo debajo.
	s.revocarArchivos()
	// Lo guardado y no lo que haya en memoria: es el token que vale para las trece, y
	// otra app puede haberlo rotado. Se lee ANTES de olvidarlo.
	if guardados, err := s.cargarSesion(); err == nil && guardados.Refresh != "" && s.oidc != nil {
		// Aparte y con plazo: sin red se sale igual, y el token caduca solo.
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), 10*time.Second)
			defer parar()
			if err := s.oidc.Revocar(ctx, guardados.Refresh); err != nil {
				slog.Warn("sesion: el token no se ha podido revocar en Sytue ID; caducará solo", "error", err)
			}
		}()
	}
	s.retirarDriveDelSistema()
	reabrirAlmacen := s.detenerTareasAlmacenamiento()
	defer reabrirAlmacen()
	errCierre := s.Sesion.Logout()
	if errCierre != nil {
		// Hasta un login completado, ni la vigilancia ni un cambio de ajustes
		// pueden readoptar una copia que el almacén no dejó retirar.
		s.reidentificando.Store(true)
	}
	s.vaciarAlmacen()
	// Las claves recordadas de los cuadernos cifrados son de esta sesión (FYS-09).
	if err := errors.Join(auth.OlvidarSecretos("notes-"), auth.OlvidarSecretos("rooms-matrix-")); err != nil {
		slog.Warn("sesion: no se han podido olvidar las claves privadas", "error", err)
	}
	guardados, marcas, errLectura := s.leerSesion(context.Background())
	s.mu.Lock()
	if errLectura == nil {
		s.Sesion.Sincronizar(guardados)
		s.generacionSesion, s.epocaSesion = marcas.generacion, marcas.epoca
	}
	s.visto, s.presencia, s.mensaje = ajustes.Leer(), "", ""
	s.visto.Sesion, s.visto.Epoca = s.generacionSesion, s.epocaSesion
	visto := s.visto.Ajustes
	s.mu.Unlock()
	s.repintar(visto) // sin sesión no hay organización que diga color ni forma
	s.avisar()
	return errCierre
}

// retirarDriveDelSistema quita Drive del gestor de ficheros y de «Archivos» y revoca su
// contraseña de aplicación (documentos.Retirar). Revocarla pide el token, que se coge
// AQUÍ, antes de olvidar la sesión; la revocación sigue aparte, con plazo.
func (s *Suite) retirarDriveDelSistema() {
	if !documentos.Guardada() {
		return
	}
	cuentaSesion := ""
	if s.almacen != nil {
		cuentaSesion = s.almacen.Identificador()
	}
	identidad := identidadDocumentos(s.Sesion.SnapshotIdentity(), cuentaSesion)
	ctx, parar := context.WithTimeout(context.Background(), 2*time.Second)
	tok, err := s.Sesion.AccessToken(ctx)
	parar()
	if err != nil {
		tok = "" // sin token se desmonta y se borra igual; la revocación queda pendiente
	}
	cuenta := api.NewAccount(s.URL("id"), tokenFijo(tok), nil)
	go func() {
		ctx, parar := context.WithTimeout(context.Background(), 10*time.Second)
		defer parar()
		if err := documentos.RetirarIdentidad(ctx, cuenta, identidad); err != nil {
			slog.Warn("sesion: la contraseña de Drive en el sistema no se ha podido revocar; se revocará al volver a entrar", "error", err)
		}
	}()
}

// tokenFijo es un token ya pedido, para lo que sigue después de [Session.Logout].
type tokenFijo string

func (t tokenFijo) AccessToken(context.Context) (string, error) {
	if t == "" {
		return "", errors.New("sesion: sin token")
	}
	return string(t), nil
}

func (tokenFijo) Invalidate(string) {}

// Poner cambia las preferencias compartidas y las aplica. El tema se ve al momento aquí
// y en las otras doce apps abiertas; el idioma, al siguiente arranque.
func (s *Suite) Poner(cambio func(*ajustes.Ajustes)) error {
	if err := ajustes.Cambiar(cambio); err != nil {
		return err
	}
	s.aplicar(ajustes.Leer())
	return nil
}

// PonerPresencia la publica en el Hub, que es donde vive. No se guarda en la máquina.
func (s *Suite) PonerPresencia(ctx context.Context, e api.EstadoPresencia, mensaje string) error {
	cada, err := s.Hub.PonerPresencia(ctx, e, mensaje)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.presencia, s.mensaje, s.haciaDonde = e, mensaje, cada
	s.mu.Unlock()
	// Elegir «desconectada» se apunta para las trece: el Hub no lo distingue de no latir.
	if invisible := e == api.Desconectado; ajustes.Leer().Invisible != invisible {
		if err := ajustes.CambiarCon(ctx, func(a *ajustes.Ajustes) { a.Invisible = invisible }); err != nil {
			slog.Warn("sesion: no se ha podido recordar la presencia elegida", "error", err)
		}
	}
	s.avisar()
	return nil
}

// Arrancar lanza las dos goroutines de fondo y vuelve enseguida: la que mira si otra app
// ha cambiado algo, y la que mantiene viva la presencia. Mueren con ctx. La presencia,
// los avisos y el enchufe de URL esperan a que haya escena (CuandoHayaEscena): con la
// app lanzada por iOS solo para el widget, solo corren el widget y el vigilante.
func (s *Suite) Arrancar(ctx context.Context) {
	// Desde aquí, un refresco sin ventana en este proceso (Android) se lo deja a esta
	// suite, que ya sigue al servidor (fondo_sinventana.go).
	suiteViva.Store(s)
	s.iniciarWidgets(ctx)
	s.iniciarRefrescoEnFondo()
	s.iniciarDespertar() //nolint:contextcheck // vive lo que el proceso, no lo que el arranque; cada petición lleva su plazo
	s.iniciarDriveSistemaNoApple(ctx)
	s.mu.Lock()
	desde := s.visto
	s.mu.Unlock()
	if s.finArranque() {
		s.NoEncontrado()
	}
	if s.app.Driver().Device().IsMobile() {
		registrarAtras()
	}
	go ajustes.Vigilar(ctx, cadaVistazo, desde, s.aplicar)
	// Lanzada solo para el widget (iOS, sin escena), la app no está «En línea», no pide
	// permisos ni escucha enlaces: todo eso espera a que haya interfaz (fondo.go).
	s.CuandoHayaEscena(func() {
		go s.latir(ctx)
		if s.Sesion.Subject() != "" {
			s.prepararAvisos()
		}
		// El enchufe se abre AQUÍ y no en Abrir: para entonces cada app ya ha registrado
		// sus receptores, así que lo que llegue tiene a quién ir.
		if id := s.app.Metadata().ID; id != "" && !s.app.Driver().Device().IsMobile() {
			go escuchar(ctx, id, func(u *url.URL) {
				fyne.Do(func() {
					s.recibirURL(u)
					aparecer(s.app)
				})
			})
		}
	})
}

// prepararAvisos pide en Android el permiso de avisos en cuanto hay alguien dentro, que
// es cuando empieza a haber de qué avisar: sin él, Android 13+ se calla todos los avisos
// de la app sin dar ningún error. Antes de entrar no se pregunta: nadie sabría por qué.
func (s *Suite) prepararAvisos() {
	if !s.app.Driver().Device().IsMobile() {
		return
	}
	go func() {
		if err := escritorio.PrepararAvisos(s.app.Metadata().Name); err != nil {
			slog.Info("sesion: los avisos del sistema no se han podido preparar", "error", err)
		}
	}()
}

// aplicar es lo que hay que hacer cuando lo compartido cambia, venga de esta app o de
// otra. Corre en la goroutine del vigilante, y también en la del refresco de fondo y en
// Poner: de uno en uno (aplicando), y la sesión se vuelve a leer del almacén dentro,
// así que la que se adopta es la de ahora y no la de quien leyó `e` antes.
func (s *Suite) aplicar(e ajustes.Estado) {
	s.aplicando.Lock()
	defer s.aplicando.Unlock()
	s.aplicarYa(e)
}

// trasLeerAlAplicar no hace nada: las pruebas paran aquí a aplicar, entre leer la sesión y
// adoptarla (TestElVigilanteNoVaciaLaSesionTrasFaceID).
var trasLeerAlAplicar = func() {}

// aplicarYa es aplicar con `aplicando` ya tomado.
func (s *Suite) aplicarYa(e ajustes.Estado) {
	s.mu.Lock()
	// También cuando solo cambia la época: es otra app confirmando o rotando la
	// identidad de una sesión que escribió una compilación anterior.
	cambioSesion := e.Sesion != s.visto.Sesion || e.Epoca != s.visto.Epoca
	pendiente := s.bloqueoPendiente
	s.mu.Unlock()
	// Sin haber pasado el bloqueo (iOS, lanzada sin escena): si ahora se lee puesto, la
	// sesión no se adopta y la que hubiera se suelta; si ahora se lee quitado, se adopta.
	if pendiente && s.retenerSiBloqueada(e) {
		cambioSesion = true
	}
	var guardados auth.Tokens
	var marcas marcasSesion
	var err error
	if cambioSesion {
		guardados, marcas, err = s.leerSesion(context.Background())
		trasLeerAlAplicar()
	}
	s.mu.Lock()
	anterior := s.generacionSesion
	s.visto = e
	if cambioSesion && err == nil {
		s.Sesion.Sincronizar(guardados)
		s.generacionSesion, s.epocaSesion = marcas.generacion, marcas.epoca
		s.visto.Sesion, s.visto.Epoca = marcas.generacion, marcas.epoca
	}
	if cambioSesion && errors.Is(err, auth.ErrSesionNoDisponible) {
		s.Sesion.Sincronizar(auth.Tokens{})
		s.generacionSesion, s.epocaSesion = "", ""
		s.visto.Sesion, s.visto.Epoca = "", ""
	}
	cambioGeneracion := cambioSesion && s.generacionSesion != anterior
	s.mu.Unlock()
	if cambioGeneracion {
		s.cancelarTareasAlmacenamiento()
	}
	if err != nil {
		slog.Warn("sesion: el almacén seguro no se deja leer", "error", err)
	}

	s.repintar(e.Ajustes)
	s.avisar()
}

// apariencia son el color y la forma EN VIGOR: lo que eligió la persona y, si no ha
// elegido, lo de su organización, que llega con la sesión (reclamos `org_color` y
// `org_forma` del ID token). Sin ambos, el color de cada app y la forma de la casa.
// Es la misma cuenta que hace la web con sus dos cookies de organización.
func (s *Suite) apariencia(a ajustes.Ajustes) (colorSuite, forma string) {
	colorSuite, forma = a.Color, a.Forma
	if s.Sesion != nil {
		orgColor, orgForma := s.Sesion.Apariencia()
		colorSuite, forma = cmp.Or(colorSuite, orgColor), cmp.Or(forma, orgForma)
	}
	return cmp.Or(colorSuite, tokens.ColorDeCadaApp), forma
}

// elegirApariencia pone el tema, el color y la forma en vigor y dice si ha cambiado algo.
func (s *Suite) elegirApariencia(a ajustes.Ajustes) bool {
	colorSuite, forma := s.apariencia(a)
	// Los tres, siempre: con `||` un cambio de tema se saltaría los otros dos.
	cambiaTema, cambiaColor, cambiaForma := tema.Elegir(a.Tema), tema.ElegirColor(colorSuite), tema.ElegirForma(forma)
	return cambiaTema || cambiaColor || cambiaForma
}

// repintar aplica la apariencia y, si ha cambiado, refresca todos los lienzos.
// `SetTheme` no cambia la variante de Fyne —de eso ya se encarga `tema.De`—, pero sí es
// lo que hace que Fyne los refresque.
func (s *Suite) repintar(a ajustes.Ajustes) {
	if s.elegirApariencia(a) {
		fyne.Do(func() { s.app.Settings().SetTheme(tema.Sytue{}) })
	}
}

// latir mantiene viva la presencia y, de paso, se entera de si la han cambiado desde
// otra app, desde la web o desde el teléfono: son la misma vuelta al Hub.
//
// ponytail: si otra ventana la cambia, esta puede volver a latir el estado viejo una
// vez antes de enterarse —hasta un latido de desfase, ~22 s—. Se arregla de verdad
// cuando el Hub empuje la presencia por su SSE (hoy solo manda `ready` y
// `notification`); mientras tanto, un sondeo más corto solo gastaría más peticiones.
func (s *Suite) latir(ctx context.Context) {
	vuelta := s.senalVuelta()
	for {
		// En segundo plano en iOS no se late: la app suspendida vuelve a correr cuando el
		// sistema la despierta para su widget, y el latido vencido publicaría «En línea»
		// sin que nadie la mire (ADR 0078, enmienda del refresco). Se espera a volver.
		if s.latidoEnPausa() {
			select {
			case <-ctx.Done():
				return
			case <-vuelta:
			}
			continue
		}
		s.mu.Lock()
		cada := s.haciaDonde
		s.mu.Unlock()
		if cada <= 0 {
			cada = 20 * time.Second // hasta que el Hub diga el suyo
		}
		if s.Sesion.Subject() != "" {
			s.latido(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(cada):
		case <-vuelta: // de vuelta al primer plano, se late ya
		}
	}
}

// latido es una vuelta al Hub: se entera de la presencia que tiene y la vuelve a poner.
func (s *Suite) latido(ctx context.Context) {
	s.mu.Lock()
	estado, mensaje := s.presencia, s.mensaje
	s.mu.Unlock()
	// «Desconectada» es también lo que dice el Hub de quien no ha latido en un rato: solo
	// se adopta si alguien de este equipo lo eligió (ajustes.Invisible). Si no, abrir una
	// ventana es estar en línea y se publica eso.
	if quien, err := s.Hub.Presencias(ctx); err == nil && len(quien) == 1 &&
		(quien[0].Estado != api.Desconectado || ajustes.Leer().Invisible) {
		if quien[0].Estado != estado || quien[0].Mensaje != mensaje {
			s.mu.Lock()
			s.presencia, s.mensaje = quien[0].Estado, quien[0].Mensaje
			s.mu.Unlock()
			estado, mensaje = quien[0].Estado, quien[0].Mensaje
			s.avisar()
		}
	}
	if estado == "" {
		estado = api.EnLinea // abrir una ventana es estar en línea
	}
	if s.latidoEnPausa() {
		return // pasó al fondo mientras se preguntaba: no se publica
	}
	if nueva, err := s.Hub.PonerPresencia(ctx, estado, mensaje); err == nil {
		s.mu.Lock()
		s.presencia, s.haciaDonde = estado, nueva
		s.mu.Unlock()
	} else if !errors.Is(err, context.Canceled) {
		slog.Debug("sesion: no se ha podido latir la presencia", "error", err)
	}
}

// abrir lleva una URL al navegador del sistema. Solo http(s): lo que llega aquí sale del
// discovery del IdP, pero al sistema no se le pasa lo que no se ha mirado.
func abrir(a fyne.App, destino string) error {
	// La puerta es la de todas las apps (`api.AbrirFuera`); `fyne.App.OpenURL` es lo
	// mismo en los cinco destinos —`open`, `xdg-open`, `rundll32`, un Intent,
	// `UIApplication`— y ahorra el fichero por plataforma que tenía la versión de Gio.
	if err := api.AbrirFuera(a, destino); err != nil {
		return errors.New("sesion: no es una URL que abrir: " + destino)
	}
	return nil
}

// sinGraficos cierra la app con un aviso del sistema si el equipo no sabe pintar su
// ventana (`escritorio.Graficos`): sin esto se quedaba viva, invisible, con su icono en
// la bandeja y sin decir nada.
func sinGraficos(a fyne.App, e ajustes.Estado) {
	if a.Driver().Device().IsMobile() {
		return
	}
	err := escritorio.Graficos()
	if err == nil {
		return
	}
	// Sin OpenGL 2.1 del sistema, el Mesa que trae el instalador de Windows: la app se
	// abre otra vez con él y esta, que ya cargó el del sistema, se va.
	if escritorio.RelanzarConReserva() {
		slog.Info("sesion: sin OpenGL 2.1 del sistema; se abre con el de reserva", "error", err)
		os.Exit(0) //nolint:revive // deep-exit: la app sigue en el proceso relanzado
	}
	slog.Error("sesion: no se puede abrir la ventana", "error", err)
	detalle := err.Error()
	var sg *escritorio.SinGraficos
	if errors.As(err, &sg) {
		detalle = sg.Detalle
	}
	texto := detalle
	if t, e2 := i18n.New(i18n.Negotiate(sistema.Idioma(), e.Idioma), i18n.Common()); e2 == nil {
		texto = t.T("shell.graphics.unsupported", i18n.Vars{"detalle": detalle})
	}
	titulo := a.Metadata().Name
	if titulo == "" {
		titulo = "Sytue"
	}
	escritorio.AvisarSinGraficos(titulo, texto)
	// Salir aquí y no en `main`: lo comparten las trece apps y, sin ventana, seguir
	// viva es quedarse invisible con el icono en la bandeja (medido en Windows).
	os.Exit(1) //nolint:revive // deep-exit: ver arriba
}

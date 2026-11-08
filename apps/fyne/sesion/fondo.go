// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// # Lanzada sin escena: iOS despierta la app para su widget
//
// iOS puede lanzar la app SIN ninguna escena para su tarea de refresco
// (BGAppRefreshTask; ADR 0078, enmienda del 2026-11-06), y entonces corre el `main`
// entero de `cmd/<app>`. Hasta el 2026-11-07 eso habría sido abrir la app: latir «En
// línea», abrir el SSE de la barra y cargar la pantalla, todo sin que nadie la mire.
//
// Ahora, hasta que conecta la primera escena, la suite solo mantiene el widget
// (iniciarWidgets) y lo compartido (ajustes.Vigilar):
//
//   - la presencia, los avisos del sistema y el enchufe de URL esperan (Arrancar);
//   - la barra no pide avisos ni abre el SSE (ui.BarraSuite.Montar);
//   - cada `cmd/<app>` pasa sus cargas de arranque por CuandoHayaEscena, y lo fija
//     TestLosCmdCarganTrasLaEscena;
//   - los AlCambiar de las apps se RETIENEN: un cambio de sesión sin escena se les avisa
//     una vez, con el estado de entonces, al conectar la escena. Solo los de la propia
//     suite que mantienen el widget se avisan siempre (alCambiarSinEscena);
//   - el bloqueo de la suite (bloqueo.go) no se pide en Abrir, sino al conectar la
//     escena: sin interfaz no hay Face ID que pedir —y fallaría, y la app saldría en
//     cada refresco—, y con el teléfono bloqueado ni siquiera se lee si está puesto
//     (`ajustes.json` es NSFileProtectionComplete y la marca del llavero,
//     WhenUnlocked). Hasta entonces, con el bloqueo puesto la sesión guardada NO se
//     adopta: ni en Abrir, ni cuando el vigilante la lee al desbloquear, ni en el
//     refresco, que con el bloqueo puesto no hace nada.
//
// La API es la opcional del driver móvil del fork (`mobile.BackgroundDriver`, parche
// apps/fyne/upstream/0016-ios-background-refresh.patch). Se pide por aserción sobre
// el driver: con un Fyne que no la tiene (el tag anterior, el escritorio, las pruebas)
// hay escena desde el principio y todo es como antes.

// conductorFondo es mobile.BackgroundDriver del fork, con sus mismos métodos: así se
// usa sin depender del tag que la trae.
type conductorFondo interface {
	LaunchedInBackground() bool
	OnFirstScene(f func())
	ProtectedDataAvailable() bool
	OnBackgroundRefresh(every time.Duration, f func(ctx context.Context) (ok bool, retry time.Duration))
}

// conEscena es la plataforma sin lanzamientos sin escena: la escena es la app misma.
type conEscena struct{}

func (conEscena) LaunchedInBackground() bool   { return false }
func (conEscena) OnFirstScene(f func())        { f() }
func (conEscena) ProtectedDataAvailable() bool { return true }

func (conEscena) OnBackgroundRefresh(time.Duration, func(context.Context) (bool, time.Duration)) {}

// conductorDe es la API de fondo del driver de `a`, o conEscena si no la tiene.
func conductorDe(a fyne.App) conductorFondo {
	if a == nil {
		return conEscena{}
	}
	if c, ok := a.Driver().(conductorFondo); ok {
		return c
	}
	return conEscena{}
}

func (s *Suite) conductor() conductorFondo {
	if s.fondo == nil {
		return conEscena{}
	}
	return s.fondo
}

// prepararFondo es lo primero de Abrir: hasta que la plataforma conecte la escena, los
// AlCambiar de las apps quedan retenidos y el bloqueo, sin comprobar. Sin lanzamientos
// sin escena, conectarEscena corre aquí mismo y todo es como antes.
func (s *Suite) prepararFondo(c conductorFondo) {
	s.mu.Lock()
	s.fondo, s.sinEscena = c, true
	s.pausaEnFondo = s.pausaEnFondo || plataformaQueDespierta
	s.mu.Unlock()
	c.OnFirstScene(s.conectarEscena)
	s.mu.Lock()
	s.bloqueoPendiente = s.sinEscena
	s.mu.Unlock()
}

// bloqueoSinComprobar dice si este proceso aún no ha pasado el bloqueo de la suite
// porque la plataforma puede no tener interfaz con que pedirlo (iOS, hasta la escena).
func (s *Suite) bloqueoSinComprobar() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bloqueoPendiente
}

// retenerSiBloqueada, sin haber pasado el bloqueo, retiene la sesión guardada si e (o
// el llavero) dice que está puesto y la suelta si dice que no. Devuelve si ha cambiado.
// Con el teléfono bloqueado no se lee ni lo uno ni lo otro: lo que no se ha podido leer
// no suelta nada (y la sesión tampoco se deja leer, así que no se adopta igualmente).
func (s *Suite) retenerSiBloqueada(e ajustes.Estado) bool {
	puesto := bloqueoPuesto(e)
	if !puesto && !s.conductor().ProtectedDataAvailable() {
		return false
	}
	return s.retenida.Swap(puesto) != puesto
}

// sinSesionPropia: esta app no adopta la sesión guardada —espera a que el dueño entre
// con su contraseña de Sytue, o a pasar el bloqueo— aunque las demás la tengan.
func (s *Suite) sinSesionPropia() bool {
	return s.reidentificando.Load() || s.retenida.Load()
}

// conectarEscena corre en el hilo principal al conectar la primera escena. Si el
// bloqueo está sin comprobar y puesto (o la sesión retenida), lo pide ahora, en otra
// rutina —Verificar bloquea mientras el diálogo está abierto—, y hasta que acaba nada de
// lo que esperaba a la escena corre ni llega ninguna URL. Si no, suelta lo retenido.
func (s *Suite) conectarEscena() {
	s.mu.Lock()
	pendiente := s.bloqueoPendiente
	s.mu.Unlock()
	if pendiente && (s.retenida.Load() || bloqueoPuesto(ajustes.Leer())) {
		s.mu.Lock()
		s.verificando = true
		s.mu.Unlock()
		go func() {
			s.pasarBloqueoEnEscena()
			s.soltarEscena()
		}()
		return
	}
	s.mu.Lock()
	s.bloqueoPendiente = false
	s.mu.Unlock()
	s.soltarEscena()
}

// pasarBloqueoEnEscena es pedirSytueIDAlAbrir ya con interfaz y con lo que hay ahora
// en disco: si el dueño no se identifica, la app no abre; sin verificador, se entra con
// la contraseña de Sytue; verificado (o quitado), se adopta la sesión guardada.
func (s *Suite) pasarBloqueoEnEscena() {
	if s.abierta != nil {
		<-s.abierta // conectarEscena puede llegar antes de que Abrir acabe
	}
	e := ajustes.Leer()
	reidentificar := pedirSytueIDAlAbrir(e)
	// Lo que sigue adopta la sesión, como aplicar, y con su mismo cerrojo: un aplicar del
	// vigilante que la leyó retenida mientras se pedía Face ID no la vacía después.
	s.aplicando.Lock()
	defer s.aplicando.Unlock()
	s.mu.Lock()
	s.bloqueoPendiente = false
	s.mu.Unlock()
	if reidentificar {
		s.reidentificando.Store(true)
		s.retenida.Store(false)
		s.Sesion.Sincronizar(auth.Tokens{})
		s.prepararReidentificacion(e)
		s.avisar()
		return
	}
	s.retenida.Store(false)
	guardados, marcas, err := s.leerSesion(context.Background())
	if err != nil {
		slog.Warn("sesion: el almacén seguro no se deja leer; habrá que entrar", "error", err)
		return
	}
	s.mu.Lock()
	anterior := s.generacionSesion
	s.Sesion.Sincronizar(guardados)
	s.generacionSesion, s.epocaSesion = marcas.generacion, marcas.epoca
	s.visto.Sesion, s.visto.Epoca = marcas.generacion, marcas.epoca
	visto := s.visto.Ajustes
	s.mu.Unlock()
	if anterior != marcas.generacion {
		s.cancelarTareasAlmacenamiento()
	}
	s.repintar(visto)
	s.avisar()
}

// soltarEscena suelta lo retenido, en este orden: si la sesión cambió sin escena, las
// apps se enteran, una vez y con el estado de ahora; luego lo de CuandoHayaEscena, y
// luego las URL que llegaron mientras se pedía el bloqueo.
func (s *Suite) soltarEscena() {
	s.mu.Lock()
	s.sinEscena, s.verificando = false, false
	retenido := s.cambioRetenido
	s.cambioRetenido = false
	tras, urls := s.trasEscena, s.urlsRetenidas
	s.trasEscena, s.urlsRetenidas = nil, nil
	s.mu.Unlock()
	if retenido {
		s.avisarApps()
	}
	// En el hilo de la interfaz, que es el de los AlCambiar: f no corre a la vez que
	// ellos, y en el orden en que se pidió.
	for _, f := range tras {
		fyne.Do(f)
	}
	if len(urls) > 0 {
		fyne.Do(func() {
			for _, u := range urls {
				s.recibirURL(u)
			}
		})
	}
}

// LanzadoEnFondo dice si el sistema lanzó este proceso sin interfaz (iOS, para la tarea
// de refresco del widget) y aún no hay ninguna escena.
func (s *Suite) LanzadoEnFondo() bool { return s.conductor().LaunchedInBackground() }

// CuandoHayaEscena ejecuta f cuando la app tenga interfaz: ya, si la tiene (siempre,
// fuera de iOS), o al conectar la primera escena —y, con el bloqueo puesto, después de
// pasarlo—, en el hilo de la interfaz y en el orden en que se pidió. Es para las CARGAS
// de arranque de cada `cmd/<app>` —su Arrancar, entrar, pedir la ficha—: con la app
// lanzada solo para el widget no corren.
//
// Lo retenido de AlCambiar se avisa antes que esto (soltarEscena), así que f ve las
// variables que esos receptores mantienen ya al día. Eso no evita cargar dos veces: si
// la sesión entró sin escena, el AlCambiar que carga «al entrar» y f cargarían los dos.
// Un AlCambiar así no carga hasta que haya corrido f (`conEscena` en bases, erp, hub,
// contacts y drive), o compara con la identidad que ya cargó (mail, calendar).
func (s *Suite) CuandoHayaEscena(f func()) {
	if f == nil {
		return
	}
	s.mu.Lock()
	if s.sinEscena {
		s.trasEscena = append(s.trasEscena, f)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	f()
}

// vistoDistinto dice si lo compartido en disco no es lo que esta suite ha aplicado.
func (s *Suite) vistoDistinto(e ajustes.Estado) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return e != s.visto
}

// avisarSuite llama aquí mismo, sin el hilo de la interfaz, a los AlCambiar de la suite
// que siguen a la sesión sin escena (el widget). Son idempotentes: el aviso de aplicar
// que llega después por el hilo de la interfaz no vuelve a hacer nada.
func (s *Suite) avisarSuite() {
	s.mu.Lock()
	e, quienes := s.estado(), append([]func(Estado){}, s.alCambiarSiempre...)
	s.mu.Unlock()
	for _, f := range quienes {
		f(e)
	}
}

// alCambiarSinEscena es AlCambiar para lo de la propia suite que tiene que seguir a la
// sesión también sin escena: el widget.
func (s *Suite) alCambiarSinEscena(f func(Estado)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alCambiarSiempre = append(s.alCambiarSiempre, f)
}

// ponerseAlDia aplica lo que hay en disco si no es lo que esta suite ha aplicado, y dice
// si lo ha hecho. Lo lee con `aplicando` tomado: el vigilante puede estar aplicando a la
// vez otra lectura, y la que se aplica aquí es la de después de la suya.
func (s *Suite) ponerseAlDia() bool {
	s.aplicando.Lock()
	defer s.aplicando.Unlock()
	e := ajustes.Leer()
	if !s.vistoDistinto(e) {
		return false
	}
	s.aplicarYa(e) //nolint:contextcheck // el del vigilante: el llavero lleva su propio plazo
	return true
}

// Resultado es lo que RefrescarWidgetEnFondo le dice al sistema.
type Resultado int

const (
	// SinCambios: no había nada que hacer (sin widget, sin sesión, con el bloqueo de la
	// suite) o lo publicado sigue siendo lo mismo.
	SinCambios Resultado = iota
	// Nuevos: el snapshot del widget se ha vuelto a escribir (o se ha borrado porque el
	// servidor ya no acepta la sesión).
	Nuevos
	// Fallo: no se ha podido leer o escribir. Lo publicado se queda, con su antigüedad.
	Fallo
	// Aplazado: los datos protegidos no están disponibles (el dispositivo está bloqueado
	// y el App Group es NSFileProtectionComplete). No se ha leído nada.
	Aplazado
)

func (r Resultado) String() string {
	switch r {
	case Nuevos:
		return "nuevos"
	case Fallo:
		return "fallo"
	case Aplazado:
		return "aplazado"
	default:
		return "sin cambios"
	}
}

const (
	// CadaRefrescoEnFondo es el refresco que se le pide a iOS cada vez que la app pasa a
	// segundo plano y tras cada uno. Es un mínimo: iOS decide cuándo, según el uso.
	CadaRefrescoEnFondo = 30 * time.Minute
	// ReintentoAplazado es el siguiente tras un Aplazado, con la esperanza de que para
	// entonces el teléfono esté desbloqueado. iOS no lo garantiza.
	ReintentoAplazado = 5 * time.Minute
)

// iniciarRefrescoEnFondo le da a la plataforma la tarea de refresco: solo si esta app
// tiene widget que publicar.
func (s *Suite) iniciarRefrescoEnFondo() {
	if s.publicadorWidget() == nil {
		return
	}
	s.conductor().OnBackgroundRefresh(CadaRefrescoEnFondo, s.refrescoDelSistema)
}

// refrescoDelSistema traduce el Resultado a lo que pide BGAppRefreshTask: éxito, y
// cuándo pedir el siguiente.
func (s *Suite) refrescoDelSistema(ctx context.Context) (ok bool, reintento time.Duration) {
	switch s.RefrescarWidgetEnFondo(ctx) {
	case Aplazado:
		return true, ReintentoAplazado
	case Fallo:
		return false, 0
	default:
		return true, 0
	}
}

// RefrescarWidgetEnFondo es la ÚNICA entrada para todo lo que despierta la app por su
// widget (hoy, la tarea de refresco de iOS). Lee con la Fuente de la app y publica, con
// el plazo de una ronda (widgets.PlazoWidget), y nada más: ni presencia, ni SSE, ni
// esperar al hilo de la interfaz. No lee nada sin widget, sin sesión, con el bloqueo de
// la suite ni con los datos protegidos fuera de alcance.
func (s *Suite) RefrescarWidgetEnFondo(ctx context.Context) Resultado {
	pub := s.publicadorWidget()
	if pub == nil || s.app == nil || !widgets.Disponible() {
		return SinCambios
	}
	// Antes que nada: con el dispositivo bloqueado ni siquiera se puede leer si la suite
	// está bloqueada (ajustes.Leer devolvería lo de por defecto).
	if !s.conductor().ProtectedDataAvailable() {
		return Aplazado
	}
	// Con el bloqueo de la suite puesto (en los ajustes o en el llavero) no se lee: sin
	// escena no se le puede pedir al dueño que diga quién es, y esa sesión no se adopta.
	if bloqueoPuesto(ajustes.Leer()) {
		return SinCambios
	}
	// Lanzada con el teléfono bloqueado, este proceso se quedó sin sesión y con el widget
	// en pausa, y el vigilante tarda hasta un vistazo en verlo: se adopta ya lo que hay
	// en disco, y el widget lo sigue sin esperar al hilo de la interfaz.
	if s.ponerseAlDia() { //nolint:contextcheck // el del vigilante: el llavero lleva su propio plazo
		s.avisarSuite()
	}
	if !s.Estado().Dentro() {
		return SinCambios
	}
	bundle := s.app.Metadata().ID
	antes, _ := widgets.Read(bundle)
	ctx, parar := context.WithTimeout(ctx, widgets.PlazoWidget)
	defer parar()
	// Lanzada en frío, la ronda del arranque (Reanudar) ya está apuntada: se publica esa
	// y no se lee dos veces.
	return resultadoDeRonda(bundle, antes, pub.RefrescarEnFondo(ctx))
}

// resultadoDeRonda traduce lo que devolvió la ronda de un despertar a lo que se le dice
// al sistema, comparando el snapshot de `bundle` con el de antes de la ronda.
func resultadoDeRonda(bundle string, antes widgets.Snapshot, err error) Resultado {
	switch {
	case widgets.AutenticacionRechazada(err):
		return Nuevos // la ronda ha borrado el snapshot de esta app
	case errors.Is(err, widgets.ErrPrivate):
		return SinCambios // la sesión cambió o se bloqueó entre medias: nada que enseñar
	case err != nil:
		return Fallo
	}
	despues, err := widgets.Read(bundle)
	// Todo el snapshot y no solo su hora: dos rondas en el mismo segundo con datos
	// distintos llevan el mismo updated_at.
	if err != nil || reflect.DeepEqual(despues, antes) {
		return SinCambios
	}
	return Nuevos
}

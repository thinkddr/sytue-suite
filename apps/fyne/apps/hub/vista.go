// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// Federado es lo que el Hub le pide al servicio: el abanico cross-app y qué productos
// levanta esta instalación. Lo cumple `*api.Hub`; en los tests es un doble.
type Federado interface {
	RecentDe(ctx context.Context, origenes []string, limite int) (api.HubBusqueda, error)
	Search(ctx context.Context, texto string, limite int) (api.HubBusqueda, error)
	Apps(ctx context.Context) ([]string, error)
}

var _ Federado = (*api.Hub)(nil)

// Agendable es lo que la tarjeta «Hoy» necesita: los eventos del día y los correos
// sin leer. Son DOS servicios distintos y por eso es una interfaz y no `*api.Hub`: en
// la web las pide el BFF con un `Promise.all` de dos `catch`, y cada mitad falla por
// su lado.
type Agendable interface {
	Hoy(ctx context.Context, desde, hasta time.Time) (eventos []EventoDeHoy, err error)
	SinLeer(ctx context.Context) (int, error)
}

// Buzon es lo que la bandeja le pide al Hub: los avisos, su flujo en vivo, marcarlos y
// las reglas de silencio. Lo cumple `*api.Hub`; en los tests es un doble.
type Buzon interface {
	Notifications(ctx context.Context, limite int) ([]api.HubNotification, error)
	NotificationsAntes(ctx context.Context, limite int, antes api.HubNotification) ([]api.HubNotification, error)
	StreamNotifications(ctx context.Context, cada func(api.HubNotification)) error
	MarkRead(ctx context.Context, id string) error
	MarkAllRead(ctx context.Context) error
	NotificationPrefs(ctx context.Context) ([]api.HubNotificationPref, error)
	SetNotificationPref(ctx context.Context, p api.HubNotificationPref) (api.HubNotificationPref, error)
}

var _ Buzon = (*api.Hub)(nil)

// Cuenta es de dónde salen los claims de quien ha entrado: su clase de organización,
// su plan, sus roles y permisos, y la organización activa que encabeza los enlaces a
// otras apps (ADR 0056). Lo cumple `*auth.Session`.
type Cuenta interface {
	Reclamos(ctx context.Context) (auth.Reclamos, error)
	Organizacion() string
}

var _ Cuenta = (*auth.Session)(nil)

// Integrable es lo que la portada le pide a la pasarela de integraciones: las
// aplicaciones conectadas y los productos sustituidos. Lo cumple `*api.Integraciones`.
type Integrable interface {
	MisApps(ctx context.Context) ([]api.AppConectada, error)
	Sustituciones(ctx context.Context) (map[string]api.Sustituto, error)
}

var _ Integrable = (*api.Integraciones)(nil)

// Cuántos elementos pide cada pantalla. Son los de la web: el lanzador no pasa
// parámetro y el Hub acota por su cuenta; la actividad pide 20 (`limit: 20`), y la
// búsqueda enseña lo que venga diciendo cuántos hay en total.
const (
	MaxRecientes = 8
	MaxActividad = 20
	// MaxTermino es lo que la web recorta en el borde antes de llamar al Hub
	// (`buscar.astro`), aunque el Hub también valide.
	MaxTermino = 200
)

// Vista es la aplicación: la pantalla en la que se está y los datos que la alimentan.
//
// No es un widget: es el ARMAZÓN que decide qué widget se enseña, y el widget que
// devuelve `Objeto()` es el que la ventana mete en su desplazador. Se separan porque
// cada pantalla del Hub tiene su propio alto en función del ancho y su propio ciclo de
// carga, y meterlas todas en un widget con un `switch` dejaría los seis estados de las
// seis pantallas en un solo Refresh.
type Vista struct {
	// Widget captures the adopted identity before reading the existing agenda.
	Widget widgets.Source
	t      *i18n.Translator
	ahora  time.Time
	// abrir es quien delega en el sistema para abrir otra app o una dirección: la app
	// en marcha, o un doble en los tests.
	abrir  api.Abridor
	hub    Federado
	agenda Agendable
	buzon  Buzon
	cuenta Cuenta
	// alta dice si quien ha entrado terminó el alta guiada; nil, no se pregunta.
	alta              func(context.Context) (bool, error)
	integr            Integrable
	almacen           AlmacenConectable
	ventanaConexiones fyne.Window
	cuentasApps       *api.Integraciones
	autorizarApp      func(context.Context, string, string) (string, error)
	apex              string

	mu      sync.Mutex
	destino Destino
	// A destination index alone cannot detect leaving and returning to the same
	// page, or a changed product filter/account while a touch is pending.
	revisionRefresco uint64
	// pararFlujo corta el flujo SSE de la bandeja; nil es que no está escuchando.
	pararFlujo context.CancelFunc
	// ctxBandeja vive lo que la bandeja abierta; generacion sube en cada carga y
	// cargandoMas evita dos «Cargar más» a la vez: una página pedida para una lista
	// que ya no está no se pega a la de ahora.
	ctxBandeja  context.Context
	generacion  int
	cargandoMas bool

	// AlMostrar avisa de que se ha cambiado de pantalla: la ventana pone su título.
	AlMostrar func(Destino)

	lanzador        *Lanzador
	actividad       *Actividad
	buscar          *Buscar
	avisos          *Avisos
	datos           *Datos
	disp            *Dispositivos
	controlClientes *controlClientesExternos
	bajadas         *Descargas
	bienv           *Bienvenida
	pila            *fyne.Container
	// pilaAvisos son los avisos flotantes («Dirección copiada»): van por ENCIMA del
	// desplazador, no dentro, o se irían con la página al bajar.
	pilaAvisos *ui.PilaAvisos

	// exportar pide la copia de «Tus datos», y carpeta es dónde se deja: la de
	// descargas del sistema, o una temporal en los tests.
	exportar Exportador
	carpeta  func() (string, error)
	// instaladores pide al Hub la lista de `/descargas` y cada instalador.
	instaladores Descargador
	// portapapeles es el del sistema; en las pruebas, uno que falla.
	portapapeles func() fyne.Clipboard
	// orgs son mis organizaciones y el cambio de la activa (la sesión transversal).
	orgs Organizador
}

// Organizador es lo que «Tus datos» necesita para elegir de qué organización es la
// copia. Lo cumple `*sesion.Suite`: cambiar de organización es volver a entrar, y eso
// es de la sesión de las trece apps, no de esta.
type Organizador interface {
	Organizaciones(ctx context.Context) ([]api.Organizacion, error)
	CambiarOrganizacion(ctx context.Context, id string) error
}

// Descargador es lo que `/descargas` le pide al Hub. Lo cumple `*api.Hub`.
type Descargador interface {
	Descargas(ctx context.Context) (api.HubDescargas, error)
	Descargar(ctx context.Context, i api.Instalador) (io.ReadCloser, string, error)
}

var _ Descargador = (*api.Hub)(nil)

// NuevaVista monta la aplicación. `a` es el `fyne.App` en marcha, que es quien sabe
// delegar en el sistema para abrir otra aplicación.
func NuevaVista(t *i18n.Translator, a fyne.App, ahora time.Time) *Vista {
	v := &Vista{t: t, ahora: ahora, apex: api.ApexProd}
	if a != nil {
		v.abrir = a
	}
	v.lanzador = NuevoLanzador(t, ahora)
	v.lanzador.Apps = Productos
	v.lanzador.Agenda.Estado = Cargando
	v.lanzador.Reciente.Estado = Cargando
	v.lanzador.AlAbrir = v.Saltar
	v.lanzador.AlAbrirURL = v.AbrirURL
	v.lanzador.AlIr = v.Ir
	v.lanzador.AlConectarAlmacen = v.conectarAlmacen
	v.lanzador.AlConectarApps = v.conectarApp

	v.actividad = NuevaActividad(t, ahora)
	v.actividad.Estado = Cargando
	v.actividad.AlAbrirURL = v.AbrirURL
	v.actividad.AlFiltrar = v.Filtrar

	v.buscar = NuevoBuscar(t)
	v.buscar.AlAbrirURL = v.AbrirURL

	v.avisos = NuevaAvisos(t)
	v.avisos.Estado = Cargando

	v.datos = NuevosDatos(t)
	v.datos.AlDescargar = v.descargarDatos
	v.datos.AlEntrarYDescargar = v.entrarYDescargar
	// Sytue Admin es SOLO web (§13.6): «Ir a Migraciones» abre el navegador.
	v.datos.AlMigrar = func() { v.AbrirURL(api.AppURL(v.apex, "console", v.org(), "migrations")) }
	v.carpeta = ficheros.Carpeta
	v.portapapeles = portapapelesDeLaApp

	v.disp = NuevosDispositivos(t)
	v.disp.AlConectarApps = v.conectarApp
	v.controlClientes = nuevoControlClientesExternos(v)
	v.disp.AlClientesExternos = func() { v.Ir(AClientesExternos) }
	v.pilaAvisos = ui.NuevaPilaAvisos()

	v.bajadas = NuevasDescargas(t)
	v.bajadas.AlDescargar = v.descargarInstalador
	v.bajadas.AlReintentar = func() { go v.cargarDescargas() }

	v.bienv = NuevaBienvenida(t)
	v.bienv.Apex = v.apex
	v.bienv.Avisar = func(tono ui.TonoAviso, titulo, cuerpo string) {
		v.pilaAvisos.Notificar(ui.NuevoAviso(tono, titulo, cuerpo))
	}
	v.bienv.Copiar = v.copiar
	v.bienv.AbrirURL = v.AbrirURL
	v.bienv.Ir = v.Ir

	v.pila = container.NewStack(v.lanzador, v.actividad, v.buscar, v.avisos, v.datos, v.disp, v.bajadas, v.bienv, v.controlClientes.pantalla)
	v.mostrar(AInicio)
	return v
}

// Portadas son las pantallas del menú de la ventana, en su orden. Son todas las del
// portal menos el alta guiada: es el primer contacto, no un sitio al que se vuelve, y la
// web tampoco la ofrece en su navegación.
func Portadas() []Destino {
	return []Destino{AInicio, AActividad, ABuscar, AAvisos, ADatos, ADescargas, ADispositivos}
}

// mostrar enseña una pantalla y esconde las demás. Un objeto invisible no se pinta y
// tampoco recibe el puntero, que es lo que hace falta aquí.
func (v *Vista) mostrar(d Destino) {
	if v.destino == AClientesExternos && d != AClientesExternos {
		v.controlClientes.limpiar()
	}
	v.revisionRefresco++
	v.destino = d
	for i, o := range v.pila.Objects {
		if i == indice(d) {
			o.Show()
			continue
		}
		o.Hide()
	}
	v.pila.Refresh()
}

// indice es la posición de cada destino portado en la pila.
func indice(d Destino) int {
	switch d {
	case AActividad:
		return 1
	case ABuscar:
		return 2
	case AAvisos:
		return 3
	case ADatos:
		return 4
	case ADispositivos:
		return 5
	case ADescargas:
		return 6
	case ABienvenida:
		return 7
	case AClientesExternos:
		return 8
	case AInicio:
	}
	return 0
}

// Con conecta la vista con el servicio. Sin esto las pantallas se quedan en su estado
// de carga, que es lo que hace el guardián de píxeles.
func (v *Vista) Con(h Federado) *Vista {
	v.hub = h
	v.lanzador.AlReintentar = func() { go v.cargarReciente() }
	v.actividad.AlReintentar = func() { go v.cargarActividad(v.actividad.Origen) }
	v.buscar.AlBuscar = func(termino string) { go v.buscarTermino(termino) }
	v.buscar.AlReintentar = func() { go v.buscarTermino(v.buscar.Termino) }
	return v
}

// Objeto es lo que la ventana enseña.
func (v *Vista) Objeto() fyne.CanvasObject { return v.pila }

// Ir cambia de pantalla y arranca su carga si hace falta.
//
// Las ocho pantallas del portal son nativas. El alta guiada no va en el menú —es el
// primer contacto, no un sitio al que se vuelve—, pero se llega por aquí igual.
func (v *Vista) Ir(d Destino) {
	v.mu.Lock()
	v.mostrar(d)
	v.mu.Unlock()
	if v.AlMostrar != nil {
		v.AlMostrar(d)
	}
	if d == AClientesExternos {
		v.RefrescarClientesExternos()
	}
	if d == AActividad && v.hub != nil {
		go v.cargarActividad(v.actividad.Origen)
	}
	// «Tus datos» pide mis organizaciones al montarse, como `TusDatos.tsx`.
	if d == ADatos {
		go v.cargarOrganizaciones()
	}
	// El alta se lee cada vez que se entra: retoma donde se dejó, también desde
	// otra máquina o desde la web.
	if d == ABienvenida && v.bienv.Alta != nil {
		go v.bienv.Cargar(context.Background(), false)
	}
	// La lista de instaladores se pide cada vez, como la página de la web: una
	// release nueva tiene que verse al volver a entrar.
	if d == ADescargas {
		go v.cargarDescargas()
	}
	// La bandeja escucha su flujo mientras se la mira, como la isla de la web mientras
	// la página está abierta; al irse, se corta.
	if d == AAvisos {
		v.abrirBandeja()
	} else {
		v.cerrarBandeja()
	}
}

// Filtrar cambia el filtro por producto de la actividad y vuelve a pedirla al Hub,
// que es lo que hace la web recargando con `?source=`.
func (v *Vista) Filtrar(origen string) {
	v.enUI(func() { v.revisionRefresco++; v.actividad.Origen = origen })
	if v.hub != nil {
		go v.cargarActividad(origen)
	}
}

// Destino es en qué pantalla se está.
func (v *Vista) Destino() Destino {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.destino
}

// Saltar abre OTRO producto. Es una acción del sistema, no una pantalla de aquí.
func (v *Vista) Saltar(p Producto, ruta string) {
	if v.abrir == nil {
		return
	}
	_ = Abrir(v.abrir, v.apex, v.org(), p, ruta)
}

// AbrirURL abre una dirección de fuera (un resultado de la actividad, el sitio
// público). Delega en el sistema igual que el salto entre aplicaciones.
func (v *Vista) AbrirURL(crudo string) {
	if v.abrir != nil {
		_ = api.AbrirFuera(v.abrir, crudo)
	}
}

// ConAgenda conecta la tarjeta «Hoy» con sus dos fuentes.
func (v *Vista) ConAgenda(a Agendable) *Vista { v.agenda = a; return v }

// ConExportador conecta «Tus datos» con el Hub.
func (v *Vista) ConExportador(e Exportador) *Vista { v.exportar = e; return v }

// ConDriveEnSistema connects only native OS controls; no server coordinates or
// application passwords are exposed to the person using the app.
func (v *Vista) ConDriveEnSistema(activo func() bool, poner func(bool) error, abrir func() error) *Vista {
	if activo != nil {
		v.disp.DriveEnSistema = activo()
	}
	fallar := func(err error) {
		if err != nil {
			slog.Warn("hub: no se ha podido actualizar Drive en el sistema", "error", err)
			v.pilaAvisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, v.t.T("devices.native.failed", nil), ""))
		}
	}
	if poner != nil {
		v.disp.AlCambiarDrive = func(enabled bool) {
			v.mu.Lock()
			if v.disp.DriveOcupado {
				v.mu.Unlock()
				return
			}
			v.disp.DriveOcupado = true
			v.disp.Refresh()
			v.mu.Unlock()
			// El almacén espera la exclusión con login/refresh/logout. Esa espera
			// no puede detener el hilo que atiende los gestos y pinta la pantalla.
			go func() {
				err := poner(enabled)
				actual := enabled
				if activo != nil {
					actual = activo()
				}
				v.enUI(func() {
					v.disp.DriveOcupado = false
					if err == nil || activo != nil {
						v.disp.DriveEnSistema = actual
					}
					fallar(err)
				})
			}()
		}
	}
	if abrir != nil {
		v.disp.AlAbrirDrive = func() {
			go func() {
				err := abrir()
				if err != nil {
					v.enUI(func() { fallar(err) })
				}
			}()
		}
	}
	v.disp.Refresh()
	return v
}

// RefrescarDriveEnSistema mirrors a preference changed by another suite app.
func (v *Vista) RefrescarDriveEnSistema(activo bool) {
	v.disp.DriveEnSistema = activo
	v.disp.Refresh()
}

// descargarDatos pide la copia y la deja en la carpeta de descargas. La pantalla dice
// que ha empezado al pulsar, como la web, y dónde ha quedado al acabar.
func (v *Vista) descargarDatos(f api.HubFormato) {
	if v.exportar == nil {
		return
	}
	v.enUI(func() { v.datos.Descarga, v.datos.Guardada = Descargando, "" })
	go func() {
		ruta, err := v.guardarDatos(f)
		v.enUI(func() {
			if err != nil {
				v.datos.Descarga = DescargaFallida
				return
			}
			v.datos.Descarga, v.datos.Guardada = Descargada, ruta
		})
	}()
}

// guardarDatos es la descarga sin plazo: una copia de gigas tarda lo que tarda, y la
// ventana del Hub se queda en la bandeja del sistema si se cierra.
func (v *Vista) guardarDatos(f api.HubFormato) (string, error) {
	carpeta, err := v.carpeta()
	if err != nil {
		return "", err
	}
	return GuardarCopia(context.Background(), v.exportar, f, carpeta)
}

// ConAltaGuiada conecta `/bienvenida` con Sytue ID, y con la vuelta a entrar que hace
// falta al terminar para que el token lleve la organización nueva.
func (v *Vista) ConAltaGuiada(a Altero, reentrar func(context.Context) error) *Vista {
	v.bienv.Alta = a
	v.bienv.Reentrar = func(ctx context.Context) error {
		if reentrar != nil {
			if err := reentrar(ctx); err != nil {
				return err
			}
		}
		// Todo lo de la portada era de antes de tener organización.
		//nolint:contextcheck // Arrancar inicia cargas independientes de la portada.
		v.Arrancar()
		return nil
	}
	return v
}

// ConOrganizaciones conecta «Tus datos» con la sesión, para elegir organización.
func (v *Vista) ConOrganizaciones(o Organizador) *Vista { v.orgs = o; return v }

// cargarOrganizaciones pide la lista. Un fallo la deja vacía y la pantalla como estaba,
// que es lo que hace la web: sin lista no hay selector.
func (v *Vista) cargarOrganizaciones() {
	if v.orgs == nil {
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()
	orgs, err := v.orgs.Organizaciones(ctx)
	if err != nil {
		return
	}
	v.enUI(func() { v.datos.Orgs, v.datos.Org = orgs, "" })
}

// entrarYDescargar es `entrarYDescargar` de `TusDatos.tsx`: entra en la otra
// organización y, al volver, la copia empieza sola —el `?descargar=` de la web—. Sin
// plazo corto: entrar pasa por el navegador y ahí la persona tarda lo que tarda.
func (v *Vista) entrarYDescargar(org string, f api.HubFormato) {
	if v.orgs == nil {
		return
	}
	v.enUI(func() { v.datos.Descarga = SinDescarga })
	go func() {
		ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancelar()
		if err := v.orgs.CambiarOrganizacion(ctx, org); err != nil {
			v.enUI(func() { v.datos.Descarga = DescargaFallida })
			return
		}
		// Todo lo de la portada era de la otra organización.
		v.Arrancar()
		v.cargarOrganizaciones()
		fyne.Do(func() { v.descargarDatos(f) })
	}()
}

// ConDescargas conecta `/descargas` con el Hub.
func (v *Vista) ConDescargas(d Descargador) *Vista { v.instaladores = d; return v }

// cargarDescargas pide la lista. Sin Hub conectado, «no hemos podido cargar»: un
// esqueleto eterno es una pantalla que miente.
func (v *Vista) cargarDescargas() {
	if v.instaladores == nil {
		v.enUI(func() { v.bajadas.Estado = Fallo })
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()
	v.enUI(func() { v.bajadas.Estado = Cargando })
	c, err := v.instaladores.Descargas(ctx)
	v.enUI(func() { v.bajadas.Catalogo, v.bajadas.Estado = c, EstadoDe(err) })
}

// descargarInstalador baja uno a la carpeta de descargas sin plazo —son cientos de
// megas— y lo dice al empezar y al acabar, que es lo que en la web hace el gestor de
// descargas del navegador. Mientras baja, su enlace se queda inerte.
func (v *Vista) descargarInstalador(i api.Instalador) {
	if v.instaladores == nil {
		return
	}
	// Los avisos van DENTRO de enUI: con el driver de pruebas `fyne.Do` no serializa, y
	// medir texto a la vez que otra pantalla se repinta es una carrera en la fuente.
	v.enUI(func() {
		v.bajadas.Bajando[i.Fichero] = true
		v.pilaAvisos.Notificar(ui.NuevoAviso(ui.AvisoInfo, v.t.T("data.export.startedTitle", nil), ProductoDeFichero(i.Fichero)))
	})
	go func() {
		ruta, err := v.guardarInstalador(i)
		v.enUI(func() {
			delete(v.bajadas.Bajando, i.Fichero)
			if err != nil {
				v.pilaAvisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, v.t.T("downloads.saveFailedTitle", nil), v.t.T("error.connection", nil)))
				return
			}
			v.pilaAvisos.Notificar(ui.NuevoAviso(ui.AvisoExito, v.t.T("downloads.savedTitle", nil), ruta))
		})
	}()
}

func (v *Vista) guardarInstalador(i api.Instalador) (string, error) {
	carpeta, err := v.carpeta()
	if err != nil {
		return "", err
	}
	cuerpo, nombre, err := v.instaladores.Descargar(context.Background(), i)
	if err != nil {
		return "", err
	}
	defer func() { _ = cuerpo.Close() }()
	// La suma del catálogo se comprueba, como hace el actualizador: sin ella, un
	// instalador cambiado o cortado quedaba en Descargas listo para abrirse (FYS-10).
	suma := sha256.New()
	ruta, err := ficheros.Guardar(carpeta, nombre, io.TeeReader(cuerpo, suma))
	if err != nil {
		return ruta, err
	}
	if i.SHA256 == "" || !strings.EqualFold(hex.EncodeToString(suma.Sum(nil)), i.SHA256) {
		return "", errors.Join(ErrSumaInstalador, os.Remove(ruta))
	}
	return ruta, nil
}

// ErrSumaInstalador es un instalador que no es el que dice el catálogo.
var ErrSumaInstalador = errors.New("hub: el instalador no coincide con su suma")

// Avisos es la pila de avisos flotantes, que la ventana pone ENCIMA del desplazador.
func (v *Vista) Avisos() fyne.CanvasObject { return v.pilaAvisos }

// copiar lleva un valor al portapapeles y lo confirma con un aviso, como la web; si no
// ha llegado —`ui.Copiar` lo lee de vuelta—, «No se ha podido copiar», que es el
// `catch` de `navigator.clipboard.writeText`.
func (v *Vista) copiar(valor, clave string) {
	if !ui.Copiar(v.portapapeles(), valor) {
		v.pilaAvisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro,
			v.t.T("devices.copyFailTitle", nil), v.t.T("devices.copyFailBody", nil)))
		return
	}
	v.pilaAvisos.Notificar(ui.NuevoAviso(ui.AvisoExito, v.t.T(clave, nil), ""))
}

// portapapelesDeLaApp es el del sistema, o ninguno sin app en marcha.
func portapapelesDeLaApp() fyne.Clipboard {
	if a := fyne.CurrentApp(); a != nil {
		return a.Clipboard()
	}
	return nil
}

// ConAlta conecta la portada con el estado del alta guiada de Sytue ID.
func (v *Vista) ConAlta(f func(context.Context) (bool, error)) *Vista { v.alta = f; return v }

// ConCuenta conecta la portada con los claims de la sesión.
func (v *Vista) ConCuenta(c Cuenta) *Vista { v.cuenta = c; return v }

// org es la organización activa de la sesión, o "" sin sesión: el enlace sale sin ella
// y el middleware de la app de destino la añade con la de quien acabe entrando.
func (v *Vista) org() string {
	if v.cuenta == nil {
		return ""
	}
	return v.cuenta.Organizacion()
}

// Arrancar lanza las cargas de la pantalla inicial.
func (v *Vista) Arrancar() {
	v.enUI(func() {
		v.revisionRefresco++
		v.lanzador.PuedeConectarAlmacen = false
		v.lanzador.PuedeConectarApps = false
	})
	if v.hub != nil {
		go v.cargarReciente()
		go v.cargarDesplegadas()
	}
	go v.cargarAgenda()
	go v.cargarCuenta()
	go v.cargarIntegraciones()
}

// ConIntegraciones conecta la portada con la pasarela de integraciones.
func (v *Vista) ConIntegraciones(i Integrable) *Vista { v.integr = i; return v }

// cargarIntegraciones pide las dos cosas por su lado y en silencio, como la web: sin
// pasarela, la portada es la de siempre —cada producto lo sirve Sytue y no hay sección
// de conectadas—.
func (v *Vista) cargarIntegraciones() {
	if v.integr == nil {
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()
	if subs, err := v.integr.Sustituciones(ctx); err == nil {
		v.enUI(func() { v.lanzador.Sustitutos = subs })
	}
	if apps, err := v.integr.MisApps(ctx); err == nil {
		v.enUI(func() { v.lanzador.Conectadas = apps })
	}
}

// SoloOrganizaciones son los productos que una cuenta personal no abre, pague o no:
// `PRODUCTOS_SOLO_ORGANIZACION` de la web (`packages/config/src/tokens.ts`), que es la
// que apaga la tesela en el conmutador y en el lanzador del portal.
// `TestLosProductosSoloDeOrganizacionesSonLosDeLaWeb` compara las dos listas.
var SoloOrganizaciones = []string{"erp", "pages", "bases"}

// cargarCuenta decide las teselas que dependen de QUIÉN ha entrado, como el
// `index.astro` de la web: Sytue Admin solo a quien puede administrar, y los productos
// solo de organizaciones apagados en una cuenta personal. Sin sesión se queda como
// está: sin Admin y con todo lo demás.
func (v *Vista) cargarCuenta() {
	if v.cuenta == nil {
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()
	r, err := v.cuenta.Reclamos(ctx)
	if err != nil {
		return
	}
	// Primer contacto, como `index.astro`: quien todavía no pertenece a ninguna
	// organización y no ha terminado el alta va al asistente.
	// Solo se pregunta en ese caso, y si Sytue ID no contesta no se manda a ninguna
	// parte: mejor la portada que un asistente que no puede cargar.
	if r.Sub != "" && r.Tenant == "" && v.alta != nil {
		if hecha, err := v.alta(ctx); err == nil && !hecha {
			v.Ir(ABienvenida)
		}
	}
	v.cargarAccesosConexiones(ctx, r)
	v.enUI(func() {
		// El de pila: lo primero del nombre visible, como `index.astro` cuando la
		// sesión no trae `given_name`.
		if partes := strings.Fields(r.Name); len(partes) > 0 {
			v.lanzador.Nombre = partes[0]
		}
		v.lanzador.PuedeAdmin = r.PuedeAdministrar()
		apagados := make(map[string]bool, len(SoloOrganizaciones))
		for _, clave := range SoloOrganizaciones {
			apagados[clave] = r.EsCuentaPersonal()
		}
		v.lanzador.Apagados = apagados
	})
}

// cargarDesplegadas deja en la rejilla solo lo que esta instalación levanta, que es lo
// que hace la web con `/api/v1/apps`. Si el Hub no contesta se queda la suite entera:
// ofrecer una tesela de más es mejor que esconder media suite (`Presentables`).
func (v *Vista) cargarDesplegadas() {
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()
	apps, err := v.hub.Apps(ctx)
	if err != nil {
		return
	}
	v.enUI(func() { v.lanzador.Apps = Presentables(apps) })
}

// cargarAgenda pide la agenda del día y el correo sin leer. Cada mitad falla por su
// lado, como en el BFF de la web: un Mail caído no deja la agenda a oscuras.
//
// SIN fuente conectada la tarjeta queda en «la agenda no está disponible», que es
// exactamente lo que la web enseña cuando el servicio no contesta. Lo que NO puede
// quedarse es en «cargando» para siempre: un esqueleto eterno es una pantalla que
// miente.
func (v *Vista) cargarAgenda() {
	if v.agenda == nil {
		v.enUI(func() { v.lanzador.Agenda = Agenda{Estado: Fallo} })
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()

	desde, hasta := LimitesDelDia(v.ahora)
	view := v.Widget.View()
	var sinLeer int
	var errCorreo error
	var lecturas sync.WaitGroup
	lecturas.Go(func() { sinLeer, errCorreo = v.agenda.SinLeer(ctx) })
	eventos, errEventos := v.agenda.Hoy(ctx, desde, hasta)
	lecturas.Wait()
	if errCorreo == nil {
		_ = view.Publicar(ctx, widgets.Counter{Kind: "unread", Value: int64(sinLeer)})
	}
	v.enUI(func() {
		a := Agenda{Eventos: eventos, SinLeer: sinLeer, HayCorreo: errCorreo == nil}
		switch e := EstadoDe(errEventos); {
		case e != Listo:
			a.Estado = e
		case len(eventos) == 0:
			a.Estado = SinNada
		default:
			a.Estado = Listo
		}
		v.lanzador.Agenda = a
	})
}

// cargarReciente pide la actividad cross-app y deja el lanzador en el estado que
// corresponda. La agenda va por su lado y todavía no está cableada: ver `Agenda`.
func (v *Vista) cargarReciente() {
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()

	v.enUI(func() { v.lanzador.Reciente.Estado = Cargando })
	res, err := v.hub.RecentDe(ctx, nil, MaxRecientes)
	v.enUI(func() {
		v.lanzador.Reciente.Items = res.Hits
		switch e := EstadoDe(err); {
		case e != Listo:
			v.lanzador.Reciente.Estado = e
		case len(res.Hits) == 0:
			v.lanzador.Reciente.Estado = SinNada
		default:
			v.lanzador.Reciente.Estado = Listo
		}
	})
}

// cargarActividad pide la actividad, con el filtro por producto si lo hay. El filtro
// va AL HUB (`sources=<clave>`), como en la web: así los veinte que se piden son
// veinte de ese producto.
func (v *Vista) cargarActividad(origen string) {
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()

	var origenes []string
	if origen != "" {
		origenes = []string{origen}
	}
	v.enUI(func() { v.actividad.Estado = Cargando })
	res, err := v.hub.RecentDe(ctx, origenes, MaxActividad)
	v.enUI(func() {
		v.actividad.Items = res.Hits
		switch e := EstadoDe(err); {
		case e != Listo:
			v.actividad.Estado = e
		case len(v.actividad.Items) == 0:
			v.actividad.Estado = SinNada
		default:
			v.actividad.Estado = Listo
		}
	})
}

// buscarTermino lanza la búsqueda federada.
func (v *Vista) buscarTermino(termino string) {
	if termino == "" {
		v.enUI(func() { v.buscar.Termino, v.buscar.Items, v.buscar.Estado = "", nil, Listo })
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelar()

	v.enUI(func() { v.buscar.Termino, v.buscar.Estado = termino, Cargando })
	res, err := v.hub.Search(ctx, termino, MaxActividad)
	v.enUI(func() {
		v.buscar.Items, v.buscar.Total, v.buscar.Fallidos = res.Hits, res.Total, res.Fallidos()
		v.buscar.Estado = EstadoDe(err)
	})
}

// enUI ejecuta en el hilo de la interfaz y repinta.
//
// `fyne.Do` es obligatorio desde 2.6 para tocar widgets desde otra gorutina. En el
// driver de PRUEBAS del fork (v2.8.1-sytue.4) también se encola: lo ejecuta la rutina
// del test en su siguiente punto de sincronía, así que un test que espere a esto
// llama a `fyne.DoAndWait` en su bucle de espera. Los Refresh van DENTRO del cerrojo
// porque leen los mismos campos que `f` escribe, y el test los lee con él.
func (v *Vista) enUI(f func()) {
	fyne.Do(func() {
		v.mu.Lock()
		defer v.mu.Unlock()
		f()
		v.lanzador.Refresh()
		v.actividad.Refresh()
		v.buscar.Refresh()
		v.avisos.Refresh()
		v.datos.Refresh()
		v.disp.Refresh()
		if v.destino == AClientesExternos {
			v.controlClientes.pantalla.Refresh()
		}
		v.bajadas.Refresh()
	})
}

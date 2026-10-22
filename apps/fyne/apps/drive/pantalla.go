// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"errors"
	"image"

	// Los tres formatos de miniatura que el servicio sirve (JPEG, PNG y GIF).
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// SinGuardarSinConexion son las rutas de Drive que la caché «sin conexión» no guarda
// en el disco. Además de los ficheros, las unidades de equipo (el contenido de sus
// ficheros se pedía por ahí y quedaba en claro) y el almacenamiento externo (su
// estado del NAS trae el secreto del túnel) (FYS-05).
var SinGuardarSinConexion = []string{
	"/api/v1/files/", "/api/v1/batch/", "/api/v1/shared/", "/api/v1/shares",
	"/api/v1/team-drives/", "/api/v1/storage/",
}

// Pantalla es la aplicación entera: el armazón con su menú y la vista de la sección
// activa, y lo único de este paquete que habla con Sytue Drive.
//
// El estado de cada vista vive en su propio tipo (`Explorador`, `VistaPapelera`) y es
// una función pura de sus campos; aquí solo se rellenan esos campos y se vuelve a
// pintar. Es lo que hace que los guardianes de píxel no necesiten ni red ni servicio.
type Pantalla struct {
	svc *api.Drive
	tr  *i18n.Translator
	red *offline.Monitor
	win fyne.Window

	seccion Seccion
	// pedidoCompartido es lo compartido que abrir cuando llegue la lista (un aviso).
	pedidoCompartido      string
	abrirExternoPendiente bool
	// peticionDueno es la franja de quien compartió algo y recibe una petición de
	// edición (AtenderPeticion), sobre la sección que esté.
	peticionDueno peticion.Estado
	lateral       *Lateral
	explor        *Explorador
	papelera      *VistaPapelera
	sec           secciones
	avisos        *ui.PilaAvisos

	raiz *fyne.Container
	// Plazo es cuánto se espera a una petición. Se deja abierto para el guardián.
	Plazo time.Duration

	// epoca invalida las respuestas en vuelo al cambiar de carpeta, de búsqueda o de
	// sección: una página de lo que se miraba antes no se mezcla con lo de ahora (el
	// `epoch` de la web). Sube desde la rutina de la carga, de ahí que sea atómica.
	epoca atomic.Uint64
	// cursor es el de la página siguiente de la carpeta; vacío, no hay más. Solo se
	// toca en la rutina de la interfaz.
	cursor string
	// busqueda es el rebote de 300 ms del buscador.
	busqueda *time.Timer
	// ancla es el índice de la última casilla marcada sin Mayús, desde el que Mayús
	// marca un rango; -1, ninguno. Se suelta al cambiar de lista.
	ancla int
	// pidiendo son las miniaturas en vuelo, para no pedir dos veces la misma.
	pidiendo map[string]bool
	// favoritos son los ids con estrella, para que el menú diga «Añadir» o «Quitar» sin
	// pedir nada por fila. Solo se toca en la rutina de la interfaz.
	favoritos map[string]bool
	// taxonomia son las etiquetas de sensibilidad de la organización, pedidas la primera
	// vez que se clasifica algo; nil, todavía no. Solo se toca en la rutina de la interfaz.
	taxonomia []api.EtiquetaDrive
	// ocupado es el `busy` de la web: una mutación a la vez.
	ocupado atomic.Bool
	// aparte lanza trabajo fuera de la rutina de la interfaz; nil es `go f()`. El
	// guardián lo cambia por una llamada directa: el driver de pruebas ejecuta
	// `fyne.Do` en el sitio, y una rutina aparte competiría con la que mira el estado.
	aparte func(func())
	// dentro es si hay sesión, según lo último que dijo `Sesion` o `Arrancar`.
	dentro bool
	// corte es el último escalón de ventana (`sm`, `md`, `lg`, `xl`) con el que se
	// pintó: cruzar uno repinta, moverse dentro de él no.
	corte int

	// Entrar abre el inicio de sesión de la suite y BLOQUEA hasta que se vuelve. Lo
	// pone el `main` con `sesion.Suite.Entrar`; nil en el guardián, que no tiene
	// navegador.
	Entrar func() error
	// ElegirFicheros abre el selector del sistema y le pasa a `listo` lo elegido: rutas
	// del disco en escritorio, flujos del proveedor de contenido en el móvil. Lo pone el
	// `main`, que es quien tiene la ventana; nil, «Subir» no hace nada.
	ElegirFicheros func(listo func([]Entrada))
	// App es quien delega en el sistema al abrir otra aplicación; vacío es la app de
	// Fyne en marcha. Apex es el dominio de la suite; vacío es producción. Los dos se
	// dejan abiertos para el guardián, que no tiene ni sistema ni red.
	App  api.Abridor
	Apex string
	// Org es la organización activa de la sesión, leída VIVA del token: encabeza los
	// enlaces a otras apps (ADR 0056). Lo pone el `main` con `s.Sesion.Organizacion`;
	// nil (el guardián) equivale a «no la sé», y los enlaces salen sin org.
	Org func() string
	// Docs es el cliente de Sytue Docs, para «Nuevo → Documento» y la sección
	// «Documentos». Lo pone el `main`; nil, esas dos cosas no hacen nada.
	Docs *api.Docs
	// Directorio resuelve un correo a la persona de la organización con quien se
	// comparte, y de vuelta. Grupos son los de quien mira, para compartir con uno. Los
	// pone el `main`; sin ellos, el diálogo de compartir ofrece solo el enlace.
	Directorio *api.Directorio
	Grupos     func() []string
	// IA es el cliente de Sytue AI, para «Resumir». Lo pone el `main`; nil, la acción
	// no se ofrece.
	IA *api.IA
	// Integraciones es la pasarela, para «Traer mis datos». Lo pone el `main` con
	// `ConIntegraciones`; nil, el bloque no se pinta.
	integraciones *api.Integraciones
	// AutorizarApp abre consentimiento en el navegador y valida la vuelta con la sesión.
	AutorizarApp func(context.Context, string, string) (string, error)
	// AutorizarAppNueva añade otra cuenta sin sustituir credenciales ya conectadas.
	AutorizarAppNueva func(context.Context, string) (string, error)
	// AutorizarNube conecta una nube personal con el mismo consentimiento nativo y
	// devuelve su instancia. Lo pone el `main`; nil, «Conectar» no hace nada.
	AutorizarNube func(ctx context.Context, proveedor string) (string, error)
}

// ConIntegraciones monta «Traer mis datos» con la pasarela: el bloque del pie del lateral
// que trae los archivos de Google Workspace o Microsoft 365.
func (p *Pantalla) ConIntegraciones(i *api.Integraciones) {
	p.integraciones = i
	imp := ui.NuevoImportarDatos(p.tr, i, "drive")
	imp.AlCambiar = p.Repintar
	imp.AlTerminar = p.Recargar
	imp.AlPolitica = p.abrirPolitica
	p.lateral.Importar = imp
}

// NuevaPantalla monta la aplicación. `svc` puede ser nil: la pantalla se pinta igual y
// se queda en la fase que se le ponga, que es lo que hace el guardián.
func NuevaPantalla(tr *i18n.Translator, svc *api.Drive, red *offline.Monitor, win fyne.Window) *Pantalla {
	p := &Pantalla{
		svc: svc, tr: tr, red: red, win: win, Plazo: 30 * time.Second,
		avisos: ui.NuevaPilaAvisos(), favoritos: map[string]bool{},
		ancla: -1, pidiendo: map[string]bool{},
	}
	p.explor = NuevoExplorador(tr)
	p.papelera = NuevaVistaPapelera(tr)
	p.papelera.Ahora = time.Now()
	p.lateral = NuevoLateral(tr, MiUnidad, p.irA)
	p.lateral.AlAbrirExterno = func() { p.gestionarExterno() }
	p.lateral.AlAnadirAlmacen = func() { p.pedirExterno() }
	p.enganchar()
	p.montarSecciones()
	p.montarEquipos()
	p.montarNube()
	if red != nil {
		red.Subscribe(func(e offline.State) { enHilo(func() { p.conexion(e != offline.Offline) }) })
	}
	return p
}

// conexion aplica un cambio de la red: sin ella se apagan las mutaciones pero se sigue
// viendo lo ya cargado, que es el contrato offline; al volver, lo que se quedó en «sin
// conexión» se pide otra vez.
func (p *Pantalla) conexion(hay bool) {
	p.explor.Desconectado, p.papelera.Desconectado = !hay, !hay
	p.sec.recientes.Desconectado, p.sec.destacados.Desconectado, p.sec.compartido.Desconectado = !hay, !hay, !hay
	p.sec.equipos.Desconectado, p.sec.nube.Desconectado = !hay, !hay
	p.Repintar()
	if hay && (p.explor.Fase == SinConexion || p.papelera.Fase == SinConexion) && p.dentro {
		p.irA(p.seccion)
	}
}

// Objeto arma el armazón. La TOPBAR no está: existe solo en la web y en nativo la
// sustituye el cromo de la ventana (CONVENTIONS.md §13.6, excepción 1).
func (p *Pantalla) Objeto() fyne.CanvasObject {
	v := p.variante()
	vista := p.vista(v)
	if f := peticion.Franja(&p.peticionDueno, false, "", p.tr.T); f != nil {
		vista = container.New(Pila{Elastico: 1}, f, vista)
	}
	arm := ui.NuevoArmazon(vista)
	arm.Menu = p.lateral.Objeto(v)
	p.raiz = container.New(&medidorVentana{al: p.alMedir}, arm, p.avisos)
	return p.raiz
}

// medidorVentana es `container.NewStack` que además avisa del ancho que le toca.
//
// `sm:`, `md:` y los cortes de la rejilla son media queries: se resuelven contra la
// VENTANA. Sin esto el explorador nunca supo cuánto medía —`Ventana` se quedaba a
// cero— y la rejilla pintaba dos columnas en una ventana de 1280, donde la web pinta
// cinco. La raíz de la app ocupa la ventana a lo ancho, así que su ancho es el de ella.
type medidorVentana struct{ al func(float32) }

var _ fyne.Layout = (*medidorVentana)(nil)

func (m *medidorVentana) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var t fyne.Size
	for _, o := range objs {
		t = t.Max(o.MinSize())
	}
	return t
}

func (m *medidorVentana) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
	m.al(t.Width)
}

// CorteDe es el escalón de Tailwind en el que cae un ancho de ventana: 0 por debajo de
// `sm`, y 1 a 4 para `sm`, `md`, `lg` y `xl`.
func CorteDe(ancho float32) int {
	n := 0
	for _, c := range []float32{ui.CorteSM, ui.CorteMD, ui.CorteLG, ui.CorteXL} {
		if ancho >= c {
			n++
		}
	}
	return n
}

// alMedir apunta el ancho y repinta solo si cruza un corte: repintar en cada píxel de
// un redimensionado rehace el árbol entero sin cambiar nada de lo que se ve.
func (p *Pantalla) alMedir(ancho float32) {
	p.explor.Ventana, p.papelera.Ventana = ancho, ancho
	p.sec.recientes.Ventana, p.sec.destacados.Ventana = ancho, ancho
	p.sec.compartido.Ventana, p.sec.documentos.Ventana = ancho, ancho
	p.sec.equipos.Ventana, p.sec.nube.Ventana = ancho, ancho
	if c := CorteDe(ancho); c != p.corte {
		p.corte = c
		// Fuera del Layout: repintar desde dentro sustituye el árbol que se está
		// colocando.
		go enHilo(p.Repintar)
	}
}

func (p *Pantalla) variante() tema.Variante {
	return tema.De(fyne.CurrentApp().Settings().ThemeVariant())
}

// vista es la pantalla de la sección activa.
func (p *Pantalla) vista(v tema.Variante) fyne.CanvasObject {
	switch p.seccion {
	case Papelera:
		return p.papelera.Objeto(v)
	case MiUnidad:
		return p.explor.Objeto(v)
	case Recientes:
		return p.sec.recientes.Objeto(v)
	case Favoritos:
		return p.sec.destacados.Objeto(v)
	case CompartidoConmigo:
		return p.sec.compartido.Objeto(v)
	case Documentos:
		return p.sec.documentos.Objeto(v)
	case UnidadesDeEquipo:
		return p.sec.equipos.Objeto(v)
	case Nube:
		return p.sec.nube.Objeto(v)
	}
	x := ui.NuevoVacio(p.tr.T("ui.loading", nil), "")
	return centrar(x, v)
}

// Repintar rehace el árbol con el estado de ahora. Se llama SIEMPRE desde la rutina
// principal: quien venga de una goroutine pasa por `enHilo`.
func (p *Pantalla) Repintar() {
	if p.raiz == nil {
		return
	}
	v := p.variante()
	arm := ui.NuevoArmazon(p.vista(v))
	arm.Menu = p.lateral.Objeto(v)
	p.raiz.Objects[0] = arm
	p.raiz.Refresh()
}

// Recargar vuelve a pedir lo de la sección en la que se está. Lo llama el `main`
// cuando cambia la sesión: entrar en otra de las apps de la suite abre esta también.
func (p *Pantalla) Recargar() {
	p.irA(p.seccion)
	go p.cargarCuota()
}

// enSegundoPlano lanza `f` fuera de la rutina de la interfaz (ver `aparte`).
func (p *Pantalla) enSegundoPlano(f func()) {
	if p.aparte != nil {
		p.aparte(f)
		return
	}
	go f()
}

// enHilo ejecuta algo en la rutina de la interfaz.
//
// `fyne.Do` es obligatorio desde Fyne 2.6 para tocar la interfaz desde otra rutina, y
// aquí hace falta de verdad: la carga, la subida y la descarga corren en su propia
// rutina para no congelar la ventana.
func enHilo(f func()) { fyne.Do(f) }

func (p *Pantalla) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), p.Plazo)
}

// hayRed dice si el monitor de conectividad ve red. Sin monitor se asume que sí: no
// tenerlo no es estar desconectado.
func (p *Pantalla) hayRed() bool {
	return p.red == nil || p.red.State() != offline.Offline
}

// irA cambia de sección y carga lo que haga falta.
func (p *Pantalla) irA(s Seccion) {
	p.seccion = s
	p.lateral.Activa = s
	p.Repintar()
	if !p.dentro {
		return
	}
	if cargar := p.cargadorDe(s); cargar != nil {
		go cargar()
	}
}

// cargadorDe es lo que pide cada sección al entrar en ella. Se llama en la rutina de la
// interfaz, que es donde se leen la carpeta y la consulta; lo que devuelve corre aparte.
func (p *Pantalla) cargadorDe(s Seccion) func() {
	switch s {
	case Papelera:
		return p.cargarPapelera
	case MiUnidad:
		carpeta := p.carpetaActual()
		return func() { p.cargarCarpeta(carpeta) }
	case Recientes:
		return p.cargarRecientes
	case Favoritos:
		return p.cargarDestacados
	case CompartidoConmigo:
		return p.cargarCompartido
	case Documentos:
		q := strings.TrimSpace(p.sec.documentos.Consulta)
		return func() { p.cargarDocumentos(q) }
	case UnidadesDeEquipo:
		return p.cargarEquipos
	case Nube:
		return p.cargarNube
	}
	return nil
}

// carpetaActual es el ID de la carpeta que se está mirando, o "" para la raíz.
func (p *Pantalla) carpetaActual() string {
	if n := len(p.explor.Ruta); n > 0 {
		return p.explor.Ruta[n-1].ID
	}
	return ""
}

// Arrancar hace la primera carga: la carpeta raíz y el medidor de almacenamiento.
//
// Sin sesión no se pide nada: se enseña directamente «Tu sesión ha caducado» con su
// botón, que es el equivalente de la web, donde sin sesión la página redirige al
// inicio de sesión antes de pintar nada. Pedir la carpeta sin token solo serviría
// para convertir un 401 seguro en un viaje de ida y vuelta.
func (p *Pantalla) Arrancar(dentro bool) {
	if p.win != nil {
		ui.AlAtras(p.win.Canvas(), p.atras)
	}
	p.dentro = dentro
	if !dentro {
		p.explor.Fase, p.papelera.Fase = SinSesion, SinSesion
		p.sec.recientes.Fase, p.sec.destacados.Fase = SinSesion, SinSesion
		p.sec.compartido.Fase, p.sec.documentos.Fase = SinSesion, SinSesion
		p.sec.equipos.Fase, p.sec.nube.Fase = SinSesion, SinSesion
		p.Repintar()
		return
	}
	go p.cargarCarpeta(p.carpetaActual())
	go p.cargarCuota()
	go p.cargarFavoritos()
	p.cargarPie()
}

// cargarPie pide lo del lateral que depende de la sesión: el almacén propio (solo si
// se administra), las nubes personales y los orígenes de «Traer mis datos».
func (p *Pantalla) cargarPie() {
	go p.cargarExterno()
	go p.cargarNubes()
	if p.lateral.Importar != nil {
		p.lateral.Importar.Cargar()
	}
}

// abrirDispositivos es «Conectar tus dispositivos»: el asistente del portal, que ya trae
// la dirección WebDAV y los pasos por sistema. Es OTRA aplicación —el Hub—, así que se
// le pide al sistema por su esquema, con la web de respaldo (CONVENTIONS.md §13.6).
func (p *Pantalla) abrirDispositivos() {
	_ = api.AbrirEn(p.abridor(), p.apex(), "one", p.org(), "/dispositivos")
}

// Sesion avisa de que la sesión ha cambiado —aquí o en otra de las apps de la suite,
// que la comparten— y recarga solo si de verdad ha cambiado: entrar en Mail abre esta
// también, y salir en cualquiera la cierra en todas.
func (p *Pantalla) Sesion(dentro bool) {
	if dentro == p.dentro {
		return
	}
	if !dentro {
		p.lateral.Cuota, p.lateral.Externo, p.lateral.Nubes = nil, nil, nil
		p.Arrancar(false)
		return
	}
	p.dentro = true
	p.Recargar()
	p.cargarPie()
}

// cargarCarpeta pide una carpeta y pinta el resultado. Corre en su propia rutina.
//
// Vuelve SIEMPRE arriba, como la web, que al recargar vacía la lista y la vuelve a
// montar: tras subir o renombrar, lo nuevo se ve desde el principio.
func (p *Pantalla) cargarCarpeta(id string) {
	epoca := p.epoca.Add(1)
	enHilo(func() {
		p.explor.Fase = Cargando
		p.explor.EnBusqueda = false
		p.explor.HayMas, p.explor.CargandoMas = false, false
		p.cursor = ""
		p.explor.AlPrincipio()
		p.Repintar()
	})
	// Sin red también se pide: lo ya consultado lo sirve la caché del transporte
	// (`offline.Transport`), que es lo que promete el contrato offline. Si no estaba,
	// el error llega igual y la fase lo dice.
	if p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	if id == "" {
		raiz, err := p.svc.Root(ctx)
		if err != nil {
			p.fallo(epoca, err)
			return
		}
		id = raiz.ID
	}
	hijos, err := p.svc.Children(ctx, id, "")
	elementos := DeHijos(hijos)
	enHilo(func() {
		if p.epoca.Load() != epoca {
			return
		}
		p.explor.Elementos = elementos
		p.explor.Fase = FaseDe(err, p.hayRed() || err == nil, len(elementos))
		p.cursor = hijos.NextCursor
		p.explor.HayMas = err == nil && p.cursor != ""
		clear(p.explor.Seleccion)
		p.ancla = -1
		p.Repintar()
	})
}

// cargarMas pide la página siguiente y la AÑADE al final. Se llama en la rutina de la
// interfaz (desde la lista o el botón de la rejilla) y la marca de «cargando» se pone
// antes de salir de ella: la lista pinta varias filas del final de golpe y sin esa
// marca pediría la misma página varias veces (el `fetchingMore` de la web).
func (p *Pantalla) cargarMas() {
	if p.explor.CargandoMas || p.cursor == "" || p.explor.EnBusqueda || p.svc == nil {
		return
	}
	p.explor.CargandoMas = true
	carpeta, cursor, epoca := p.carpetaActual(), p.cursor, p.epoca.Load()
	go func() {
		enHilo(p.Repintar)
		ctx, cancelar := p.ctx()
		defer cancelar()
		id := carpeta
		var err error
		if id == "" {
			var raiz api.Folder
			if raiz, err = p.svc.Root(ctx); err == nil {
				id = raiz.ID
			}
		}
		var hijos api.Children
		if err == nil {
			hijos, err = p.svc.Children(ctx, id, cursor)
		}
		enHilo(func() {
			if p.epoca.Load() != epoca {
				return
			}
			p.explor.CargandoMas = false
			if err != nil {
				// No se rompe lo ya cargado: se avisa, y se reintenta al volver a
				// llegar al final.
				p.avisar(ui.AvisoPeligro, p.tr.T("files.loadMoreFailed", nil), "")
				p.Repintar()
				return
			}
			p.explor.Elementos = append(p.explor.Elementos, DeHijos(hijos)...)
			p.cursor = hijos.NextCursor
			p.explor.HayMas = p.cursor != ""
			p.Repintar()
		})
	}()
}

func (p *Pantalla) fallo(epoca uint64, err error) {
	enHilo(func() {
		if p.epoca.Load() != epoca {
			return
		}
		p.explor.Fase = FaseDe(err, p.hayRed() || err == nil, 0)
		p.Repintar()
	})
}

// cargarCuota pide el espacio usado para el medidor del carril. Un fallo lo deja sin
// medidor, que es lo que hace la web: el pie se enseña sin barra y no estorba.
func (p *Pantalla) cargarCuota() {
	if p.svc == nil || !p.hayRed() {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	q, err := p.svc.Quota(ctx)
	if err != nil {
		return
	}
	enHilo(func() {
		p.lateral.Cuota = &Cuota{Usado: q.UsedBytes, Tope: q.LimitBytes, Externo: q.ExternalStorage}
		p.Repintar()
	})
}

// cargarPapelera pide la papelera.
func (p *Pantalla) cargarPapelera() {
	enHilo(func() { p.papelera.Fase = Cargando; p.Repintar() })
	if p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	papelera, err := p.svc.Trash(ctx)
	elementos := DePapelera(papelera)
	enHilo(func() {
		p.papelera.Elementos = elementos
		p.papelera.Ahora = time.Now()
		p.papelera.Fase = FaseDe(err, p.hayRed() || err == nil, len(elementos))
		p.Repintar()
	})
}

// enganchar conecta las acciones de las vistas con el servicio.
func (p *Pantalla) enganchar() {
	e := p.explor
	e.AlAbrir = p.abrir
	e.AlNavegar = p.navegar
	e.AlReintentar = p.reintentar
	e.AlCambiarVista = func(rejilla bool) { e.Rejilla = rejilla; p.Repintar() }
	e.AlBuscar = p.buscar
	e.AlSeleccionar = p.marcar
	e.AlSeleccionarTodo = p.marcarTodo
	e.AlNuevaCarpeta = p.pedirCarpeta
	e.AlPapeleraLote = p.papeleraLote
	e.AlDescargarLote = p.descargarLote
	e.AlMoverLote = p.pedirMoverLote
	e.AlMoverA = func(origenes []Elemento, destino Elemento) { go p.moverArrastrando(origenes, destino) }
	e.Miniaturas = map[string]image.Image{}
	e.AlMiniatura = p.pedirMiniatura
	e.AlSubir = p.elegirYSubir
	e.AlCargarMas = p.cargarMas
	e.AlMenu = p.menuElemento
	e.AlNuevo = p.menuNuevo
	p.lateral.AlAbrirDispositivos = p.abrirDispositivos

	t := p.papelera
	t.AlReintentar = func() {
		if t.Fase == SinSesion {
			p.entrar()
			return
		}
		go p.cargarPapelera()
	}
	t.AlRestaurar = func(el Elemento) { go p.restaurar(el) }
	t.AlEliminar = p.pedirPurga
}

// abrir entra en una carpeta, abre el fichero en SU aplicación, o lo descarga.
//
// El orden es el de `docs/CONVENTIONS.md` §13.6: un `.docx` abre Docus, una hoja Calcus,
// una presentación Diapos, un PDF CertiPDF y una imagen SyPhotos — Drive no lleva visor
// de los formatos de las demás. Lo que no reclama nadie sigue como estaba: se baja a la
// carpeta de descargas y el aviso dice dónde ha quedado. Esta app no escribe nunca
// encima de un documento cuyo formato no entienda, que es la única forma de garantizar
// que no se corrompe.
func (p *Pantalla) abrir(el Elemento) {
	if el.Carpeta && p.explor.EnBusqueda {
		p.salirDeLaBusqueda()
		p.explor.Fase = Cargando
		p.explor.AlPrincipio()
		p.Repintar()
		go p.abrirDesdeLejos(el)
		return
	}
	if el.Carpeta {
		p.explor.Ruta = append(p.explor.Ruta, el)
		go p.cargarCarpeta(el.ID)
		return
	}
	if p.abrirEnSuApp(el) {
		return
	}
	if TipoVista(el) != "" {
		p.pedirVista(el)
		return
	}
	go p.descargar(el)
}

// salirDeLaBusqueda vacía el buscador: lo que se enseña a partir de ahora es una
// carpeta, y dejar el término escrito diría que la lista es su resultado.
func (p *Pantalla) salirDeLaBusqueda() {
	if p.busqueda != nil {
		p.busqueda.Stop()
	}
	// Primero se sale y luego se vacía el campo: vaciarlo avisa a `buscar`, que en
	// búsqueda recargaría la carpeta VIEJA mientras se resuelve la nueva.
	p.explor.EnBusqueda = false
	p.explor.Consulta = ""
	if p.explor.buscador != nil {
		p.explor.buscador.PonerTexto("")
	}
}

// abrirDesdeLejos entra en una carpeta que no cuelga de la que se estaba mirando —un
// resultado de búsqueda— con su ruta VERDADERA en las migas. Se sube de padre en padre
// hasta la raíz, que es lo que el servicio permite: la búsqueda da la ruta como texto,
// y las migas navegan por id. Antes se colgaba de la ruta vieja y las migas mentían.
func (p *Pantalla) abrirDesdeLejos(el Elemento) {
	ctx, cancelar := p.ctx()
	defer cancelar()
	ruta, err := RutaHasta(ctx, p.svc.Folder, el)
	if err != nil {
		// Sin la ruta se entra igual: mejor unas migas cortas que no abrir la carpeta.
		ruta = []Elemento{el}
	}
	enHilo(func() { p.explor.Ruta = ruta })
	p.cargarCarpeta(el.ID)
}

// RutaHasta son las carpetas desde la raíz (sin ella) hasta `el` incluida, subiendo de
// padre en padre. `pedir` es `api.Drive.Folder`; se pasa para que el guardián no
// necesite servicio.
//
// ponytail: una petición por nivel. Un árbol de diez niveles son diez viajes; si se nota,
// el servicio puede devolver los ancestros en la búsqueda.
func RutaHasta(ctx context.Context, pedir func(context.Context, string) (api.Folder, error), el Elemento) ([]Elemento, error) {
	ruta := []Elemento{el}
	actual, err := pedir(ctx, el.ID)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 64 && actual.ParentID != nil; i++ {
		padre, err := pedir(ctx, *actual.ParentID)
		if err != nil {
			return nil, err
		}
		if padre.IsRoot || padre.ParentID == nil {
			break
		}
		ruta = append([]Elemento{DeCarpeta(padre)}, ruta...)
		actual = padre
	}
	return ruta, nil
}

// abrirEnSuApp le pasa el fichero a la aplicación de su formato. Devuelve false si
// ninguna lo reclama o si el sistema no pudo abrir ni el esquema ni la web, y entonces
// quien llama sigue con la descarga de siempre en vez de quedarse callado.
//
// Lo que viaja es una REFERENCIA —el id del fichero en Drive, y su ruta para CertiPDF,
// que es lo único que su página acepta—, nunca una copia: quien guarda es la app dueña
// del formato, sobre el fichero que vive aquí con su ACL y su historial de versiones.
func (p *Pantalla) abrirEnSuApp(el Elemento) bool {
	return api.AbrirFichero(p.abridor(), p.apex(), p.org(), el.Nombre, api.Referencia{ID: el.ID, Ruta: p.rutaDe(el)})
}

// rutaDe es el camino completo del elemento en Drive.
//
// El servicio solo manda la ruta en lo que CRUZA el árbol —una búsqueda, la papelera—,
// así que en una carpeta se arma con las migas, que son las carpetas de verdad por las
// que se ha entrado y traen su nombre del servicio. La raíz no cuenta: en la web las
// rutas empiezan en `/`, no en «Mi unidad».
func (p *Pantalla) rutaDe(el Elemento) string {
	if el.Ruta != "" {
		return el.Ruta
	}
	var b strings.Builder
	for _, c := range p.explor.Ruta {
		b.WriteByte('/')
		b.WriteString(c.Nombre)
	}
	b.WriteByte('/')
	b.WriteString(el.Nombre)
	return b.String()
}

// navegar sube por las migas. -1 es la raíz.
func (p *Pantalla) navegar(nivel int) {
	if nivel < 0 {
		p.explor.Ruta = nil
	} else if nivel < len(p.explor.Ruta) {
		p.explor.Ruta = p.explor.Ruta[:nivel+1]
	}
	carpeta := p.carpetaActual()
	p.enSegundoPlano(func() { p.cargarCarpeta(carpeta) })
}

// RetrasoBusqueda es el rebote del buscador: una petición por pulsación castigaría al
// servicio y haría parpadear la lista.
const RetrasoBusqueda = 300 * time.Millisecond

// buscar consulta el nombre en TODO el árbol, no solo en esta carpeta, 300 ms después
// de la última pulsación.
//
// La selección se vacía al entrar y al salir de la búsqueda: pertenece a la lista que
// se estaba mirando, y arrastrarla dejaría la barra de lote hablando de ficheros que ya
// no están delante.
func (p *Pantalla) buscar(texto string) {
	p.explor.Consulta = texto
	clear(p.explor.Seleccion)
	if p.busqueda != nil {
		p.busqueda.Stop()
	}
	q := strings.TrimSpace(texto)
	if q == "" {
		p.epoca.Add(1) // la búsqueda que estuviera en vuelo ya no pinta nada
		if p.explor.EnBusqueda {
			go p.cargarCarpeta(p.carpetaActual())
		}
		return
	}
	p.busqueda = time.AfterFunc(RetrasoBusqueda, func() { p.ejecutarBusqueda(q) })
}

// ejecutarBusqueda pide UN término. Corre en su propia rutina, y una respuesta que
// llegue cuando ya se ha escrito otra cosa se tira: la de «inf» llegando después de la
// de «informe» pintaría los resultados de lo que ya no está escrito.
func (p *Pantalla) ejecutarBusqueda(q string) {
	epoca := p.epoca.Add(1)
	enHilo(func() {
		p.explor.Fase, p.explor.EnBusqueda = Cargando, true
		p.explor.HayMas, p.explor.CargandoMas = false, false
		p.explor.AlPrincipio()
		p.Repintar()
	})
	if p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	res, err := p.svc.Search(ctx, q)
	elementos := DeHijos(res)
	enHilo(func() {
		if p.epoca.Load() != epoca {
			return
		}
		p.explor.Elementos = elementos
		p.explor.Fase = FaseDe(err, p.hayRed(), len(elementos))
		p.Repintar()
	})
}

// reintentar es el botón de los estados de error: vuelve a entrar si la sesión
// caducó, repite la búsqueda si lo que falló fue ella, y recarga la carpeta si no.
func (p *Pantalla) reintentar() {
	switch {
	case p.explor.Fase == SinSesion:
		p.entrar()
	case p.explor.EnBusqueda:
		go p.ejecutarBusqueda(strings.TrimSpace(p.explor.Consulta))
	default:
		go p.cargarCarpeta(p.carpetaActual())
	}
}

// entrar abre el inicio de sesión de la suite. Es el «Volver a iniciar sesión» de la
// web, que recarga la página y deja que el servidor redirija.
//
// No recarga ella: al volver, la sesión avisa a todos los que la miran —esta pantalla
// incluida, por `Sesion`—, y recargar también aquí pediría todo dos veces.
func (p *Pantalla) entrar() {
	if p.Entrar == nil {
		return
	}
	// Cancelado, o cerrado el navegador: se queda donde estaba.
	go func() { _ = p.Entrar() }()
}

// Con Mayús y un ancla puesta, marca el RANGO del ancla a este (sin desmarcar nada),
// que es el `toggleSelect` de la web; el índice es el de la lista que se VE, que en
// una búsqueda no es la de la carpeta.
func (p *Pantalla) marcar(id string) {
	e := p.explor
	// La serie es la que se VE: con la franja de carpetas, primero sus teselas y después
	// la lista (`posicionEnLista` e `idsDelRango` de teselas.ts).
	enOrden := e.EnOrden()
	switch i := indiceDe(enOrden, id); {
	case id == "":
		clear(e.Seleccion)
		p.ancla = -1
	case Mayus() && p.ancla >= 0 && i >= 0:
		MarcarRango(e.Seleccion, enOrden, p.ancla, i)
	default:
		e.Seleccion[id] = !e.Seleccion[id]
		p.ancla = i
	}
	p.Repintar()
}

// MarcarRango marca los elementos entre dos índices, ambos incluidos, en cualquier orden.
func MarcarRango(sel map[string]bool, els []Elemento, a, b int) {
	lo, hi := min(a, b), max(a, b)
	for i := max(lo, 0); i <= hi && i < len(els); i++ {
		if els[i].ID != "" {
			sel[els[i].ID] = true
		}
	}
}

func indiceDe(els []Elemento, id string) int {
	for i, el := range els {
		if el.ID == id {
			return i
		}
	}
	return -1
}

// Mayus dice si la tecla Mayús está pulsada ahora mismo. Un toque de Fyne no trae los
// modificadores, pero el driver de escritorio sabe cuáles hay pulsados; en móvil no hay.
func Mayus() bool {
	d, ok := fyne.CurrentApp().Driver().(desktop.Driver)
	return ok && d.CurrentKeyModifiers()&fyne.KeyModifierShift != 0
}

// marcarTodo es el «seleccionar todo» de la cabecera: si ya está todo marcado, lo
// desmarca; si no, lo marca entero. Es lo que hace la web con su tercer estado.
func (p *Pantalla) marcarTodo() {
	todo := p.explor.TodoMarcado()
	clear(p.explor.Seleccion)
	if !todo {
		for _, el := range p.explor.Elementos {
			p.explor.Seleccion[el.ID] = true
		}
	}
	p.Repintar()
}

// seleccionados devuelve los elementos marcados, en el orden de la lista.
func (p *Pantalla) seleccionados() []Elemento {
	var out []Elemento
	for _, el := range p.explor.Elementos {
		if p.explor.Seleccion[el.ID] {
			out = append(out, el)
		}
	}
	return out
}

// pedirCarpeta abre el diálogo de «Nueva carpeta».
// Es el Modal de talla md de la web, con la etiqueta encima del campo; antes era la
// confirmación estrecha (sm) y sin etiqueta.
func (p *Pantalla) pedirCarpeta() {
	if p.win == nil || p.explor.SinRed() {
		return
	}
	carpeta := p.carpetaActual()
	d := p.dialogoCampo(p.tr.T("files.newFolder.title", nil), p.tr.T("files.newFolder.label", nil),
		p.tr.T("files.create", nil), ui.NuevaEntrada(p.tr.T("files.newFolder.placeholder", nil)),
		func(nombre string) bool { return p.crearCarpeta(carpeta, nombre) })
	d.Mostrar(p.win.Canvas())
}

// crearCarpeta crea la carpeta y dice si salió bien. Corre fuera de la interfaz.
func (p *Pantalla) crearCarpeta(padre, nombre string) bool {
	if !p.ocupar() {
		return false
	}
	defer p.soltar()
	ctx, cancelar := p.ctx()
	defer cancelar()
	if padre == "" {
		raiz, err := p.svc.Root(ctx)
		if err != nil {
			p.avisar(ui.AvisoPeligro, p.tr.T("files.newFolder.failed", nil), "")
			return false
		}
		padre = raiz.ID
	}
	if _, err := p.svc.CreateFolder(ctx, padre, nombre); err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("files.newFolder.failed", nil), "")
		return false
	}
	p.avisar(ui.AvisoExito, p.tr.T("files.newFolder.created", i18n.Vars{"name": nombre}), "")
	go p.recargarVista()
	return true
}

// papeleraLote manda la selección a la papelera, con su confirmación: es reversible,
// pero mueve varias cosas de golpe y conviene saber cuántas.
func (p *Pantalla) papeleraLote() {
	sel := p.seleccionados()
	if len(sel) == 0 || p.win == nil {
		return
	}
	cuerpo := parrafo(p.variante(), p.tr.T("files.batch.trashBody", i18n.Vars{"count": len(sel)}), 0)
	var d *ui.Dialogo
	d = ui.Confirmacion(p.tr.T("files.batch.trashTitle", nil), p.tr.T("ui.cancel", nil),
		p.tr.T("files.batch.trashCta", nil), true, cuerpo,
		func() { d.Cerrar() },
		func() { d.Cerrar(); go p.mandarAPapelera(sel) })
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	d.Mostrar(p.win.Canvas())
}

func (p *Pantalla) mandarAPapelera(sel []Elemento) {
	ctx, cancelar := p.ctx()
	defer cancelar()
	ficheros, carpetas := partir(sel)
	lote, err := p.svc.BatchTrash(ctx, ficheros, carpetas)
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("files.batch.trashFailed", nil), motivo(err))
		return
	}
	p.avisar(ui.AvisoExito, p.tr.T("files.batch.trashDone", i18n.Vars{"count": lote.Total()}), "")
	p.cargarCarpeta(p.carpetaActual())
}

// pedirPurga confirma el borrado definitivo, que es irreversible.
func (p *Pantalla) pedirPurga(el Elemento) {
	if p.win == nil {
		return
	}
	var d *ui.Dialogo
	d = p.papelera.ConfirmacionPurga(p.variante(), el,
		func() { d.Cerrar() },
		func() { d.Cerrar(); go p.purgar(el) })
	d.Mostrar(p.win.Canvas())
}

func (p *Pantalla) purgar(el Elemento) {
	p.papeleraOcupada(true)
	defer p.papeleraOcupada(false)
	ctx, cancelar := p.ctx()
	defer cancelar()
	if err := p.svc.Purge(ctx, claseDe(el), el.ID); err != nil {
		// El servicio puede negarse con un motivo (hoy: retención legal). Decirlo es
		// la diferencia entre «reintenta en vano» y «esto no depende de ti».
		if m := motivo(err); m != "" {
			p.avisar(ui.AvisoPeligro, p.tr.T("trash.purgeDenied", i18n.Vars{"name": el.Nombre}), m)
			return
		}
		p.avisar(ui.AvisoPeligro, p.tr.T("trash.purgeFailed", nil), "")
		return
	}
	p.avisar(ui.AvisoExito, p.tr.T("trash.purged", i18n.Vars{"name": el.Nombre}), "")
	p.cargarPapelera()
}

// papeleraOcupada apaga los botones de la papelera mientras hay una operación en
// curso: dos «restaurar» seguidos sobre lo mismo dan un 409 confuso.
func (p *Pantalla) papeleraOcupada(si bool) {
	enHilo(func() { p.papelera.Ocupado = si; p.Repintar() })
}

func (p *Pantalla) restaurar(el Elemento) {
	p.papeleraOcupada(true)
	defer p.papeleraOcupada(false)
	ctx, cancelar := p.ctx()
	defer cancelar()
	if err := p.svc.Restore(ctx, claseDe(el), el.ID); err != nil {
		clave := "trash.restoreFailed"
		var e *api.Error
		if errors.As(err, &e) && e.Status == http.StatusConflict {
			clave = "trash.restoreConflict"
		}
		p.avisar(ui.AvisoPeligro, p.tr.T(clave, i18n.Vars{"name": el.Nombre}), "")
		return
	}
	p.avisar(ui.AvisoExito, p.tr.T("trash.restored", i18n.Vars{"name": el.Nombre}), "")
	p.cargarPapelera()
}

// descargar baja un fichero con su progreso y avisa de dónde ha quedado.
func (p *Pantalla) descargar(el Elemento) {
	prog := NuevoProgreso(1)
	prog.Bajada = true
	prog.Empezar(0, el.Nombre)
	enHilo(func() { p.explor.Subida = prog; p.Repintar() })
	// Un reloj que refresca la barra mientras la transferencia avanza. El estado que
	// toca vive tras el cerrojo de `Progreso`, que es por lo que esto no es carrera.
	parar := p.latido()
	defer parar()

	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancelar()
	ruta, err := Descargar(ctx, p.svc, el, prog)
	enHilo(func() { p.explor.Subida = nil; p.Repintar() })
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("files.error.network", nil), motivo(err))
		return
	}
	p.avisar(ui.AvisoExito, el.Nombre, ruta)
}

// Atajo es la tecla suelta de la web: «u» sube y «n» crea una carpeta
// (`esAtajoDeApp`: sin modificadores, fuera de un campo y sin nada superpuesto; lo
// primero y lo último los comprueba quien llama). Solo en «Mi unidad» y con red, que
// es donde la web los escucha. Devuelve si la tecla era suya.
func (p *Pantalla) Atajo(r rune) bool {
	if p.seccion != MiUnidad || p.explor.SinRed() {
		return false
	}
	switch r {
	case 'u':
		p.elegirYSubir()
	case 'n':
		p.pedirCarpeta()
	default:
		return false
	}
	return true
}

// elegirYSubir es el botón «Subir»: abre el selector del sistema y sube lo elegido.
func (p *Pantalla) elegirYSubir() {
	if p.ElegirFicheros == nil || p.explor.SinRed() {
		return
	}
	p.ElegirFicheros(p.SubirEntradas)
}

// descargarLote baja la selección empaquetada en un ZIP, en streaming, a la carpeta de
// descargas. El nombre es el de la carpeta si es lo único marcado; si no, el genérico
// del servicio.
func (p *Pantalla) descargarLote() {
	sel := p.seleccionados()
	if len(sel) == 0 {
		return
	}
	solo := ""
	if len(sel) == 1 && sel[0].Carpeta {
		solo = sel[0].Nombre
	}
	ficheros, carpetas := partir(sel)
	go p.bajarZip(ficheros, carpetas, solo)
}

// bajarZip trae un ZIP del servicio y avisa de dónde ha quedado.
//
// La franja enseña el nombre del ZIP con su barra: el servicio dice cuánto suman los
// ficheros sin comprimir y la descarga cuenta lo que lleva descomprimido. Si no lo dice,
// el canal se queda vacío: una barra que avanzara sin saber el total sería inventada.
func (p *Pantalla) bajarZip(ficheros, carpetas []string, nombre string) {
	prog := NuevoProgreso(1)
	prog.Bajada = true
	prog.Empezar(0, NombreZip(nombre))
	enHilo(func() { p.explor.Subida = prog; p.Repintar() })
	parar := p.latido()
	defer parar()
	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancelar()
	ruta, err := DescargarZip(ctx, p.svc, ficheros, carpetas, nombre, prog)
	enHilo(func() {
		if p.explor.Subida == prog {
			p.explor.Subida = nil
			p.Repintar()
		}
	})
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("files.error.network", nil), motivo(err))
		return
	}
	p.avisar(ui.AvisoExito, NombreZip(nombre), ruta)
}

// partir separa una selección en los ID de ficheros y los de carpetas, que es como los
// piden los endpoints de lote.
func partir(sel []Elemento) (ficheros, carpetas []string) {
	for _, el := range sel {
		if el.Carpeta {
			carpetas = append(carpetas, el.ID)
		} else {
			ficheros = append(ficheros, el.ID)
		}
	}
	return ficheros, carpetas
}

// Subir sube los ficheros que la persona haya soltado en la ventana. Una CARPETA
// soltada se sube entera, con su árbol.
func (p *Pantalla) Subir(rutas []string) { p.SubirEntradas(Rutas(rutas)) }

// SubirEntradas sube lo elegido en el diálogo del sistema o soltado en la ventana: una
// ruta que es una carpeta se recorre entera, y un flujo del móvil sube tal cual.
//
// La lista la trae quien llama: el diálogo de fichero es del armazón nativo y no de esta
// pantalla.
func (p *Pantalla) SubirEntradas(elegidas []Entrada) {
	// Solo en «Mi unidad»: soltar ficheros sobre la papelera no significa nada, y
	// subirlos a una carpeta que no se está viendo sería peor.
	if len(elegidas) == 0 || p.explor.SinRed() || p.seccion != MiUnidad {
		cerrar(elegidas)
		return
	}
	carpeta := p.carpetaActual()
	go func() {
		var entradas []Entrada
		var rutas []string
		for _, e := range elegidas {
			if e.Flujo != nil {
				entradas = append(entradas, e)
			} else {
				rutas = append(rutas, e.Ruta)
			}
		}
		entradas = append(entradas, EntradasDe(rutas)...)
		if len(entradas) == 0 {
			return
		}
		prog := NuevoProgreso(len(entradas))
		enHilo(func() { p.explor.Subida = prog; p.Repintar() })
		parar := p.latido()
		ctx, cancelar := context.WithTimeout(context.Background(), 60*time.Minute)
		defer cancelar()
		subidos, err := SubirArbol(ctx, p.svc, carpeta, entradas, prog)
		parar()
		enHilo(func() { p.explor.Subida = nil; p.Repintar() })
		tono, titulo := resumenSubida(p.tr, subidos, len(entradas), err)
		p.avisar(tono, titulo, "")
		go p.cargarCuota()
		p.cargarCarpeta(carpeta)
	}()
}

// resumenSubida es el aviso del final, con el criterio de la web: todo bien es un
// éxito; si algo falló es un PELIGRO —no una advertencia: hay ficheros que no están
// donde la persona cree—, y si fue por la cuota se dice, porque la salida es otra
// (liberar espacio, no reintentar).
func resumenSubida(tr *i18n.Translator, subidos, total int, err error) (ui.TonoAviso, string) {
	if err == nil {
		return ui.AvisoExito, tr.T("files.upload.done", i18n.Vars{"count": subidos})
	}
	var e *api.Error
	if errors.As(err, &e) && e.Status == http.StatusInsufficientStorage {
		return ui.AvisoPeligro, tr.T("files.upload.quota", i18n.Vars{"uploaded": subidos, "count": total})
	}
	return ui.AvisoPeligro, tr.T("files.upload.partial", i18n.Vars{
		"uploaded": subidos, "count": total, "failed": total - subidos,
	})
}

// latido refresca la barra de progreso cuatro veces por segundo y devuelve cómo
// pararlo. Cuatro y no sesenta: la barra no se lee más rápido y cada repintado rehace
// el árbol de la pantalla.
func (p *Pantalla) latido() func() {
	fin := make(chan struct{})
	go func() {
		reloj := time.NewTicker(250 * time.Millisecond)
		defer reloj.Stop()
		for {
			select {
			case <-fin:
				return
			case <-reloj.C:
				enHilo(p.Repintar)
			}
		}
	}()
	return func() { close(fin) }
}

// avisar apila un aviso desde cualquier rutina.
func (p *Pantalla) avisar(tono ui.TonoAviso, titulo, descripcion string) {
	enHilo(func() { p.avisos.Notificar(ui.NuevoAviso(tono, titulo, descripcion)) })
}

// claseDe es el segmento de la API: renombrar, mover, borrar y restaurar son la misma
// petición para una carpeta y para un fichero, y solo cambia esto.
func claseDe(el Elemento) api.DriveKind {
	if el.Carpeta {
		return api.DriveFolder
	}
	return api.DriveFile
}

// motivo es lo que el servicio dijo, si dijo algo útil. No se enseña la URL ni el
// cuerpo entero: un error de API no es un mensaje para una persona.
func motivo(err error) string {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Code()
	}
	return ""
}

// pedirMiniatura trae la miniatura de una imagen para su tarjeta. Una que no llega se
// apunta como nil y no se vuelve a pedir: la tarjeta se queda con su tesela, que es lo
// que hace la web al fallar la `<img>`.
func (p *Pantalla) pedirMiniatura(el Elemento) {
	if p.svc == nil || p.pidiendo[el.ID] {
		return
	}
	p.pidiendo[el.ID] = true
	go func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		var img image.Image
		if cuerpo, err := p.svc.Thumbnail(ctx, el.ID); err == nil {
			img, _, _ = image.Decode(cuerpo)
			_ = cuerpo.Close()
		}
		enHilo(func() {
			delete(p.pidiendo, el.ID)
			p.explor.Miniaturas[el.ID] = img
			if p.explor.Rejilla {
				p.Repintar()
			}
		})
	}()
}

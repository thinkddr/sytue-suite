// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/photos/src/pages/index.astro
// paridad-web: apps/frontend/apps/photos/src/pages/favorites.astro
// paridad-web: apps/frontend/apps/photos/src/pages/albums.astro
// paridad-web: apps/frontend/apps/photos/src/pages/album.astro
// paridad-web: apps/frontend/apps/photos/src/pages/duplicates.astro
// paridad-web: apps/frontend/apps/photos/src/pages/trash.astro
// paridad-web: apps/frontend/apps/photos/src/pages/similar.astro
// paridad-web: apps/frontend/apps/photos/src/pages/nearby.astro
// paridad-web: apps/frontend/apps/photos/src/pages/shared.astro

package photos

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"weak"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/autorizacion"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La app viva
//
// Las pantallas de este paquete son funciones de sus datos (ver photos.go). App es lo
// que las anima: guarda el estado que en la web vive en los `useState` de PhotosApp.tsx
// y de sus hermanas, llama a syphotos por `apps/nucleo/api` y, cada vez que algo cambia,
// vuelve a construir la pantalla con la función que toca.
//
// Rehacer la pantalla entera es lo que hace Preact con un `setState`, y aquí cuesta poco
// porque lo caro —decodificar y remuestrear fotos— está en la Fototeca y no se repite.
// Lo que NO se rehace son los campos donde se escribe: viven en App y se vuelven a
// colgar de la pantalla nueva, para que el foco y lo escrito no se pierdan a media
// palabra.
//
// Todo lo que toca el estado corre en el hilo de la interfaz: las llamadas al servicio
// van a una goroutine (`lanzar`) y vuelven con `fyne.Do`. Así no hay cerrojos.

// Ruta es la pantalla en la que está la app: las ocho de la web que tienen sesión y el
// álbum compartido (`/p/{token}`), que no la tiene. El editor (`/edit`) no es
// navegación: ver `abrirEditor`.
type Ruta uint8

// Las nueve. Las cinco primeras van en el mismo orden que las pestañas de la nav.
const (
	RutaFotos Ruta = iota
	RutaFavoritas
	RutaAlbumes
	// RutaCompartidos es «Compartido conmigo» (`/shared`): los álbumes que otras
	// personas me han dado. Uno de ellos abierto es RutaCompartido.
	RutaCompartidos
	RutaRepetidas
	RutaPapelera
	RutaAlbum
	RutaParecidas
	RutaCerca
	// RutaPublica es el álbum compartido por enlace: se entra por AbrirPublico.
	RutaPublica
	// RutaCompartido es un álbum compartido conmigo (`/shared?album=`): se entra por
	// AbrirCompartido.
	RutaCompartido
)

// Tamaños de página y topes de la web.
const (
	// PaginaLinea es el `limite = 100` de `lineaDeTiempo`.
	PaginaLinea = 100
	// AntesDelFinal es el `rootMargin: '800px'` del centinela: la página siguiente se
	// pide una pantalla antes de llegar abajo, no cuando ya se ve el hueco.
	AntesDelFinal = 800
	// EsperaBusqueda es el `setTimeout(…, 250)` entre teclear y pedir.
	EsperaBusqueda = 250 * time.Millisecond
	// TopeParecidas es el `slice(0, 12)` de «Más como estas».
	TopeParecidas = 12
)

// App es Sytue Photos en marcha. El cero no vale: se crea con Nueva.
type App struct {
	cli *api.Photos
	w   fyne.Window
	v   Vista

	// anyadir es «Añadir fotos», el «+» flotante de la galería (`BotonFlotante` en
	// PhotosApp.tsx), en todos los anchos; capaAnyadir lo coloca y se esconde fuera de la
	// galería y sin sesión.
	anyadir     *ui.BotonFlotante
	capaAnyadir *fyne.Container

	// Drive es el cliente de Sytue Drive, donde viven los originales: subir es dejar el
	// fichero en «/Fotos» y luego indexarlo. nil: no se puede subir.
	Drive *api.Drive
	// DriveDeSubida es el Drive donde suben «Añadir fotos» y la copia, preguntado en
	// cada subida: el del servidor mientras admita escrituras y, cuando ya no (está en
	// almacenamiento privado, la bóveda de la ADR 0086), el Drive privado de la
	// persona. Lo que se sube a uno privado va cifrado y la galería del servidor no lo
	// indexa. nil: siempre [App.Drive].
	DriveDeSubida func(context.Context) (*api.Drive, error)
	// GaleriaCifrada es el Drive de la bóveda para la galería cifrada (ADR 0086), con el
	// índice que comparte con la web (`nucleo/photos/galeriacifrada`). (nil, nil): el
	// Drive de la persona no está en la bóveda. Sin `abrir`, no pide desbloquearlo:
	// [ErrDriveCifradoCerrado] si aún no lo está. nil: no hay galería cifrada.
	GaleriaCifrada func(ctx context.Context, abrir bool) (*api.Drive, error)
	// ClaveEnDispositivo dice si la clave de desbloqueo de Sytue ID está en este
	// dispositivo (con la sesión, en el almacén seguro del sistema): con ella, la copia
	// con la app cerrada puede pedir a la puerta de la bóveda que abra el Drive cifrado
	// (copia_fondo.go). Solo pregunta; la clave no sale de la sesión. nil: no.
	ClaveEnDispositivo func() bool
	// AlEntrar abre Sytue ID. Es lo que ofrece la pantalla cuando no hay sesión.
	AlEntrar func()
	// AbrirWeb abre una dirección en el navegador del sistema: el editor de la web, el
	// mapa. nil: no se ofrece.
	AbrirWeb func(url string)
	// WebDe compone la dirección de una página de la WEB de Fotos: el editor
	// (`/edit?id=`) y el álbum compartido (`/p/{token}`).
	WebDe func(ruta string) string
	// Guardar deja una descarga en la carpeta de descargas y dice dónde.
	Guardar func(nombre string, datos []byte) (string, error)
	// Elegir abre el selector de ficheros del sistema y devuelve lo elegido. nil: no
	// hay selector, y «Añadir fotos» solo admite soltar.
	Elegir func() ([]string, error)
	// ImportarVideo mete en la fototeca un VÍDEO elegido con «Añadir fotos» o soltado en
	// la ventana (Sytue Office: copiado, con su duración y su póster, FO-09); con él,
	// «Añadir fotos» y soltar admiten además los vídeos de `carrete.TipoDe`. nil, como en
	// la conectada: solo imágenes.
	ImportarVideo func(ctx context.Context, ruta string) error
	// AbrirApp abre otra app de la suite por su esquema (con la web de respaldo): la
	// vuelta del editor a la carpeta de Drive de la que se vino.
	AbrirApp func(producto, ruta string)
	// IA es el cliente de Sytue AI (`*api.IA`) o, en Sytue Office, la IA que la persona
	// configuró: describir, preguntar, ampliar una búsqueda y editar. nil: se dice lo
	// mismo que dice la web cuando no está configurada.
	IA api.Asistente
	// ExportarFototeca es «Exportar la fototeca» en la barra de la galería: Rotulo y qué
	// hace al pulsarlo. nil, como en la conectada: no se ofrece.
	ExportarFototeca *AccionExtra
	// ImportarFototeca es «Importar» en la barra de la galería (Sytue Office: del carrete,
	// de una carpeta, carpetas vigiladas; FO-04 y FO-09). nil, como en la conectada.
	ImportarFototeca *AccionExtra
	// Directorio traduce el correo que se escribe al compartir un álbum al `sub` con que
	// se guarda, y de vuelta para decir quién lo compartió. nil: no se ofrece compartir
	// con personas, y quien comparte conmigo sale como «alguien de tu organización».
	Directorio *api.Directorio

	// Galeria es la galería DEL SISTEMA (`apps/nucleo/carrete`): de donde lee la copia de
	// seguridad. nil, o sin galería en este sistema: no se ofrece la copia.
	Galeria GaleriaSistema
	// Copia es la copia de seguridad de este dispositivo, con su registro. nil: no hay.
	Copia *Copia
	// AlCambiarCopia recibe los ajustes de la copia cada vez que cambian (y al arrancar):
	// es quien le dice al sistema cuándo despertar a la app para copiar con ella cerrada.
	// nil: no hay copia con la app cerrada.
	AlCambiarCopia func(AjustesCopia)
	// FondoDisponible: la copia sigue con la app cerrada en este sistema (Android, iOS), y
	// el diálogo lo dice.
	FondoDisponible bool
	// DestinoGaleria es a qué galería del sistema llevan «Añadir a Fotos» (macOS) y
	// «Guardar en Imágenes» (Windows, Linux), en el visor y en la selección.
	DestinoGaleria DestinoGaleria
	// SeguirEnFondo pide al sistema tiempo para acabar una subida al pasar a segundo plano
	// (sistema.SeguirEnFondo). nil: no hace falta.
	SeguirEnFondo func(nombre string) (terminar func())

	// Reproductor monta, para el visor, el reproductor de un VÍDEO de la galería
	// (api.Photo.EsVideo) en vez de su póster: el objeto y con qué pararlo al pasar a
	// otra o cerrar el visor (Sytue Office, FO-09). nil, como en la conectada: un vídeo
	// se enseña como una foto.
	Reproductor func(api.Photo) (fyne.CanvasObject, func())
	// AvisoDeFoto es lo que el visor dice por su cuenta de una foto (Sytue Office: que su
	// original enlazado ya no está, FO-09); "" o nil, nada.
	AvisoDeFoto func(api.Photo) string

	// NoEncontrado enseña la página «no encontrada» de la suite y dice si había quién.
	// Es lo que contesta la web a un enlace `/p/` que no existe (un 404). nil, o si
	// devuelve false, la pantalla dice «error» como cualquier otra.
	NoEncontrado func() bool

	// lanzar ejecuta una llamada al servicio. En la app va a una goroutine; los
	// guardianes la ponen en línea para leer el estado sin carreras.
	lanzar                  func(func())
	vida                    context.Context
	cancelarVida            context.CancelFunc
	identidad               string
	identidadAdoptada       bool
	refrescando             bool
	cerrada                 bool
	reanudandoImportaciones bool
	// espera es el retardo de la búsqueda; cero en los guardianes.
	espera time.Duration

	raiz        *fyne.Container
	scroll      *container.Scroll
	buscador    *ui.Entrada
	campoAlbum  *ui.Entrada
	ruta        Ruta
	corte       int
	gen         int
	temporizado *time.Timer
	albumes     []api.Album // la lista para los selectores; nil, sin pedir
	huellas     sync.Map    // id → huella de la receta tras girar

	// arrastrando es que hay ficheros arrastrándose por encima de la ventana.
	arrastrando bool

	// avisos son los toasts de la casa (Toast.tsx): lo acabado y el «Deshacer» de la
	// papelera. El progreso sigue en línea.
	avisos *ui.PilaAvisos
	// tirando son las fotos de camino a la papelera (ver tirarSinRepetir).
	tirando map[string]bool

	g   estadoGaleria
	alb estadoAlbumes
	uno estadoAlbum
	pap estadoPapelera
	rep estadoRepetidas
	enc estadoEncontradas
	vis *estadoVisor
	ed  *estadoEditor
	peg estadoPegado
	rec estadoRecuerdos
	// cif es la galería cifrada (galeria_cifrada.go); minisCif descifra sus miniaturas
	// para las Fototecas, y cifDesviadas dice que ya se les enseñó a pedírselas.
	cif          estadoCifradas
	minisCif     *miniaturasCifradas
	cifDesviadas bool
	// anyos son los años con fotos de la vista abierta: la barra para saltar a uno.
	anyos []api.TimelineYear
	pub   estadoPublico
	cmp   estadoCompartidos
	aj    api.PhotosSettings
	cop   estadoCopia
	// terminarFondo cierra el tiempo pedido con SeguirEnFondo; nil, no se ha pedido.
	terminarFondo func()

	// cuenta es `identidad` para leerla desde otras goroutines: la descarga que guarda en
	// la galería del sistema marca la foto en el registro de la copia de esa cuenta.
	cuenta atomic.Value
}

// estadoCarga es lo común a las pantallas que piden UNA cosa al entrar.
type estadoCarga struct {
	cargando bool
	fallo    error
	aviso    string
	error    string
}

type estadoGaleria struct {
	estadoCarga
	fotos   []api.Photo
	cursor  string
	fin     bool
	texto   string
	tambien []string
	// anyo es el año al que se saltó (la primera página empieza en él); 0, desde lo
	// más reciente. Vive en la galería para que buscar —que la rehace— lo olvide.
	anyo      int
	seleccion map[string]bool
	// ancla es el id de la última marcada SIN Mayús, desde la que Mayús + clic marca el
	// tramo; "", ninguna. Va por id y no por posición porque `fotos` cambia en pleno modo
	// selección (quitar una favorita en su vista, deshacer una tirada).
	ancla     string
	subida    *[2]int
	lote      *[2]int // describiendo: hechas, total
	parar     bool
	ampliando bool
	// marcandoFavoritas es el corazón de la barra en vuelo: mientras vuelven los PUT
	// otra pulsación no lanza el mismo plan otra vez (`marcandoFavoritas` en la web).
	marcandoFavoritas bool
	// pendiente es la búsqueda que espera al temporizador: aún no se ha pedido, así que
	// cuenta como cargando y no dice «Ninguna foto coincide con…» mientras se teclea.
	pendiente bool
	// siguienteAlLlegar abre la foto siguiente en el visor en cuanto llegue la página:
	// es el `cargarMas().then(() => setAbierta(abierta + 1))` de la web.
	siguienteAlLlegar bool
}

type estadoAlbumes struct {
	estadoCarga
	lista    []api.Album
	montones [][]api.Photo
	buscando bool
	creando  bool
}

type estadoAlbum struct {
	estadoCarga
	id    string
	album api.Album
	fotos []api.Photo
}

type estadoPapelera struct {
	estadoCarga
	fotos []api.Photo
	// retencion son los días antes de la purga; 0, la papelera no caduca.
	retencion int
}

type estadoRepetidas struct {
	estadoCarga
	grupos     [][]api.Photo
	trabajando bool
}

type estadoEncontradas struct {
	estadoCarga
	ids         []string
	referencias []api.Photo
	fotos       []api.Photo
	coordenadas string
}

type estadoPublico struct {
	estadoCarga
	token string
	album api.PublicAlbum
	// teca y grande piden por el enlace, sin sesión: son otras que las de la galería
	// porque la puerta es otra (PublicVariant con el token).
	teca, grande *Fototeca
	// abierta es la foto enseñada sola; real, su tamaño real.
	abierta *api.Photo
	real    bool
	capa    *capaVisor
}

type estadoRecuerdos struct {
	lista   []api.Memory
	pedidos bool
}

// Nueva monta la app contra ese cliente. `w` es la ventana: de ella salen el lienzo de
// los diálogos y el portapapeles. Sin cliente (nil) pinta lo que tenga y no pide nada.
func Nueva(cli *api.Photos, v Vista, w fyne.Window) *App {
	a := &App{
		cli: cli, w: w, v: v, espera: EsperaBusqueda,
		lanzar:   func(f func()) { go f() },
		avisos:   ui.NuevaPilaAvisos(),
		minisCif: &miniaturasCifradas{},
	}
	a.vida, a.cancelarVida = context.WithCancel(context.Background())
	a.buscador = nuevoBuscador(v, "")
	a.buscador.AlCambiar(a.buscar)
	// Albums.tsx usa el `Input` de la casa en su talla `md`, del alto de su botón. Era un
	// `<input>` a mano, 28 px junto a un botón de 32.
	a.campoAlbum = ui.NuevaEntrada(v.Traduce("albums.title"))
	a.campoAlbum.AlTeclear = func(e *fyne.KeyEvent) bool {
		if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
			a.crearAlbum(a.campoAlbum.Texto())
			return true
		}
		return false
	}
	a.scroll = container.NewVScroll(container.NewStack())
	a.scroll.OnScrolled = func(fyne.Position) { a.quizaMas(); a.pegar() }
	a.peg.capa = container.NewWithoutLayout()
	refresco := ui.RefrescarAlTirar(a.scroll, &ui.RefrescoLista{
		Arriba:     func() bool { return a.scroll.Offset.Y <= 0.5 },
		Puede:      a.puedeRefrescarLista,
		Revision:   func() uint64 { return ui.RevisionLista(a.gen) },
		Actualizar: a.refrescarLista,
	})
	a.anyadir = ui.NuevoBotonFlotante(v.Traduce("gallery.add"), a.elegirFicheros)
	a.capaAnyadir = container.New(ui.CapaFlotante{}, a.anyadir)
	a.capaAnyadir.Hide()
	// La pila de avisos es HERMANA del contenido, no una capa (ver ui/aviso.go).
	a.raiz = container.New(&raizLayout{a: a}, refresco, a.peg.capa, a.capaAnyadir, a.avisos)
	a.corte = claseVentana(v.Ventana)
	// El tema se cambia en caliente desde el menú de cuenta (o desde otra app de la
	// suite): las pantallas de Photos se arman con colores ya resueltos, así que hay que
	// rehacerlas. Sin esto se quedaban en la variante con la que arrancó la app.
	if application := fyne.CurrentApp(); application != nil {
		reference := weak.Make(a)
		application.Settings().AddListener(func(fyne.Settings) {
			fyne.Do(func() {
				current := reference.Value()
				if current == nil || current.cerrada {
					return
				}
				if varianteActual() != current.v.Variante {
					current.pintar()
					if current.vis != nil {
						current.pintarVisor()
					}
				}
			})
		})
	}
	return a
}

// varianteActual es la de la suite ahora mismo: la elegida en el menú de cuenta o, si no,
// la del sistema.
func varianteActual() tema.Variante { return tema.De(fyne.CurrentApp().Settings().ThemeVariant()) }

// Objeto es lo que se cuelga de la ventana (debajo de la barra de la suite).
func (a *App) Objeto() fyne.CanvasObject { return a.raiz }

// Ruta dice en qué pantalla está.
func (a *App) Ruta() Ruta { return a.ruta }

// Arrancar enchufa el teclado y pide la primera pantalla.
func (a *App) Arrancar() {
	if a.cerrada {
		return
	}
	a.desviarCifradas()
	if a.w != nil {
		a.w.Canvas().SetOnTypedKey(a.teclaSuelta)
		a.w.Canvas().SetOnTypedRune(a.runaSuelta)
		ui.AlAtras(a.w.Canvas(), a.atras)
		a.w.SetOnDropped(a.soltado)
	}
	// Un enlace a un álbum compartido puede haber llegado ANTES: con la app cerrada, el
	// sistema la arranca con él y la sesión lo entrega en cuanto hay receptor.
	if a.ruta != RutaPublica && a.ruta != RutaCompartido {
		a.Ir(RutaFotos)
		a.reanudarImportaciones()
	}
	a.iniciarCopia()
}

// Ir cambia de pantalla y pide lo que esa pantalla necesita, como un `href` de la web.
func (a *App) Ir(r Ruta) {
	a.cancelarFotos()
	a.cerrarVisor()
	a.cerrarEditor()
	a.cerrarPublica()
	a.ruta = r
	a.gen++
	a.scroll.Offset = fyne.NewPos(0, 0)
	switch r {
	case RutaFotos, RutaFavoritas:
		if a.temporizado != nil {
			a.temporizado.Stop()
		}
		a.g = estadoGaleria{}
		// Los años de la otra vista (todas o favoritas) no valen: fuera hasta que lleguen.
		a.anyos = nil
		a.buscador.PonerTexto("") // dispara AlCambiar con "", que no cambia nada
		a.pintar()
		a.cargarMas()
		a.pedirRecuerdos()
		a.pedirAnyos()
		if !a.cif.pedida {
			a.cargarCifradas(false)
		}
	case RutaAlbumes:
		a.alb = estadoAlbumes{estadoCarga: estadoCarga{cargando: true}}
		a.pintar()
		a.pedirAlbumes()
		a.pedirAjustes()
		if !a.cif.pedida {
			a.cargarCifradas(false)
		}
	case RutaRepetidas:
		a.rep = estadoRepetidas{estadoCarga: estadoCarga{cargando: true}}
		a.pintar()
		a.pedirRepetidas()
		a.pedirAjustes()
	case RutaPapelera:
		a.pap = estadoPapelera{estadoCarga: estadoCarga{cargando: true}}
		a.pintar()
		a.pedirPapelera()
	case RutaCompartidos:
		a.cmp = estadoCompartidos{estadoCarga: estadoCarga{cargando: true}}
		a.pintar()
		a.pedirCompartidos()
	case RutaCompartido:
		a.pedirCompartido()
	case RutaAlbum, RutaParecidas, RutaCerca:
		// Estas tres llevan parámetro: se entra por AbrirAlbum, AbrirParecidas y
		// AbrirCerca.
		a.pintar()
	case RutaPublica:
		a.pedirPublico()
	}
}

// cancelarFotos retira las colas de la pantalla anterior, sin vaciar las cachés.
func (a *App) cancelarFotos() {
	vistas := map[*Fototeca]bool{}
	for _, f := range []*Fototeca{a.v.Teca, a.v.TecaGrande, a.pub.teca, a.pub.grande, a.cmp.teca} {
		if f != nil && !vistas[f] {
			vistas[f] = true
			f.Cancelar()
		}
	}
}

// AlFondo cancela descargas y preparaciones que ya no se ven, también al cerrar.
func (a *App) AlFondo() {
	a.cancelarFotos()
	a.AlFondoCopia()
}

// AlVolver reconstruye los rasters retirados al salir de primer plano.
func (a *App) AlVolver() {
	if a.cerrada {
		return
	}
	a.pintar()
	if a.vis != nil {
		a.pintarVisor()
	}
	if a.pub.abierta != nil {
		a.pintarPublica()
	}
	// Lo fotografiado mientras la app estaba en segundo plano.
	a.rondaCopia()
}

// irSeccion es lo que hace una pestaña de la nav.
func (a *App) irSeccion(s Seccion) { a.Ir(Ruta(s)) }

// hilo lleva `f` al hilo de la interfaz si la generación no ha cambiado: una respuesta
// que llega después de cambiar de pantalla es de otra pantalla y se tira.
func (a *App) hilo(gen int, f func()) {
	fyne.Do(func() {
		if !a.cerrada && gen == a.gen {
			f()
		}
	})
}

// ctx es el de una llamada al servicio: con tope, para que una red colgada no deje la
// rueda girando para siempre.
func ctx(vida context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(vida, 60*time.Second)
}

// ---- Pintar ----

// pintar rehace la pantalla de la ruta actual con su estado.
func (a *App) pintar() {
	a.v.Variante = varianteActual()
	if a.v.Teca != nil {
		a.v.Teca.Olvidar()
	}
	if a.v.TecaGrande != nil {
		a.v.TecaGrande.Olvidar()
	}
	if a.pub.teca != nil {
		a.pub.teca.Olvidar()
		a.pub.grande.Olvidar()
	}
	var o fyne.CanvasObject
	switch a.ruta {
	case RutaFotos, RutaFavoritas:
		o = a.pantallaGaleria()
	case RutaAlbumes:
		o = PantallaAlbumes(a.vistaDe(a.alb.estadoCarga, a.alb.lista == nil, len(a.alb.lista) == 0), a.datosAlbumes())
	case RutaAlbum:
		o = PantallaAlbum(a.vistaDe(a.uno.estadoCarga, a.uno.fotos == nil, false), a.datosAlbum())
	case RutaPapelera:
		o = PantallaPapelera(a.vistaDe(a.pap.estadoCarga, a.pap.fotos == nil, len(a.pap.fotos) == 0), a.datosPapelera())
	case RutaRepetidas:
		o = PantallaRepetidas(a.vistaDe(a.rep.estadoCarga, a.rep.grupos == nil, len(a.rep.grupos) == 0), a.datosRepetidas())
	case RutaParecidas, RutaCerca:
		o = PantallaEncontradas(a.vistaDe(a.enc.estadoCarga, a.enc.fotos == nil, len(a.enc.fotos) == 0), a.datosEncontradas())
	case RutaPublica:
		o = a.pantallaPublica()
	case RutaCompartidos:
		v := a.vistaDe(a.cmp.estadoCarga, a.cmp.lista == nil, len(a.cmp.lista) == 0)
		v.Teca = a.cmp.teca
		o = PantallaCompartidos(v, a.datosCompartidos())
	case RutaCompartido:
		o = a.pantallaCompartido()
	}
	a.ponerAnyadir()
	if a.capaAnyadir.Visible() {
		o = ui.ConReservaFlotante(o) // que el «+» no tape la última fila de fotos
	}
	a.scroll.Content = o
	a.scroll.Refresh()
	a.pegar()
	if a.vis != nil {
		a.pintarVisor()
	}
	if a.ed != nil {
		a.pintarEditor()
	}
}

// ponerAnyadir enseña el «+» de «Añadir fotos» donde la web lo pinta —la galería y sus
// favoritas, con sesión— y lo apaga mientras sube algo, como lo estaba el botón de la
// barra.
func (a *App) ponerAnyadir() {
	g := a.g
	ver := (a.ruta == RutaFotos || a.ruta == RutaFavoritas) && a.vistaDe(g.estadoCarga, false, false).Estado != SinSesion
	if ver {
		a.capaAnyadir.Show()
	} else {
		a.capaAnyadir.Hide()
	}
	if inerte := g.subida != nil || a.reanudandoImportaciones; inerte != a.anyadir.Inerte {
		a.anyadir.Inerte = inerte
		a.anyadir.Refresh()
	}
}

// vistaDe decide en cuál de los seis estados está una pantalla.
func (a *App) vistaDe(e estadoCarga, sinDatos, vacia bool) Vista {
	v := a.v
	v.AlEntrar = a.AlEntrar
	v.Desplaza = a.scroll
	v.AlReintentar = a.reintentar
	switch {
	case e.fallo != nil && sinDatos:
		v.Estado = estadoDeFallo(e.fallo)
	case e.cargando && sinDatos:
		v.Estado = Cargando
	case vacia:
		v.Estado = Vacia
	}
	return v
}

// reintentar vuelve a pedir lo que la pantalla de ahora no llegó a cargar. Las que
// llevan parámetro se reabren con el suyo; la galería pide la misma página otra vez
// sin perder la búsqueda escrita (`Ir` la borraría).
func (a *App) reintentar() {
	switch a.ruta {
	case RutaFotos, RutaFavoritas:
		a.g.fallo, a.g.error = nil, ""
		a.cargarMas()
	case RutaAlbum:
		a.AbrirAlbum(a.uno.id)
	case RutaParecidas:
		a.AbrirParecidas(a.enc.ids)
	case RutaCerca:
		if len(a.enc.ids) > 0 {
			a.AbrirCerca(a.enc.ids[0])
		}
	default:
		a.Ir(a.ruta)
	}
}

// estadoDeFallo traduce un error del servicio a uno de los estados de la pantalla.
func estadoDeFallo(err error) Estado {
	var e *api.Error
	switch {
	// Un 401 es la sesión, no el permiso: el cliente ya ha renovado y reintentado, así
	// que el servicio no acepta a esta persona y lo que toca es volver a entrar. Antes
	// salía «No tienes acceso» (el 403) y la persona buscaba un permiso que tenía.
	case errors.Is(err, autorizacion.ErrLoginRequired), errors.As(err, &e) && e.Status == 401:
		return SinSesion
	case errors.As(err, &e) && e.Status == 403:
		return SinPermiso
	case errors.As(err, &e):
		return Error
	}
	return SinConexion
}

// textoDeFallo es el `role="alert"` de un fallo que NO vacía la pantalla (una favorita
// que no se guardó, una foto que no se fue a la papelera).
func (a *App) textoDeFallo(err error) string {
	// 507: lo que se añade a un álbum de otra persona se copia a su Drive, y no cabe.
	if e := (*api.Error)(nil); errors.As(err, &e) && e.Status == http.StatusInsufficientStorage {
		return a.v.Traduce("sharedAlbum.quotaFull")
	}
	switch estadoDeFallo(err) {
	case SinConexion:
		return a.v.Traduce("offline.description")
	case SinPermiso:
		return a.v.Traduce("forbidden.description")
	default:
		return a.v.Traduce("error.connection")
	}
}

// ---- La galería ----

func (a *App) soloFavoritas() bool { return a.ruta == RutaFavoritas }

func (a *App) pantallaGaleria() fyne.CanvasObject {
	v, d := a.datosGaleria()
	a.recoger(&d)
	pantalla := PantallaGaleria(v, d)
	// El marco pone la pila de bloques como su segundo hijo: sus posiciones son las de
	// los meses dentro del desplazamiento.
	a.peg.pila = nil
	if len(pantalla.Objects) > 1 {
		a.peg.pila = pantalla.Objects[1]
	}
	return pantalla
}

// datosGaleria es lo que la galería enseña, sin montarla todavía.
func (a *App) datosGaleria() (Vista, Galeria) {
	g := a.g
	// La línea que se ve: la del servidor con las cifradas por fecha (ADR 0086).
	linea := a.lineaMezclada()
	esperando := g.cargando || g.pendiente
	v := a.vistaDe(g.estadoCarga, len(linea) == 0, !esperando && len(linea) == 0 && g.fallo == nil && g.texto == "")
	aviso := g.aviso
	if g.subida != nil {
		aviso = a.v.T.T("gallery.uploading", i18n.Vars{"done": g.subida[0], "total": g.subida[1]})
	} else if aviso == "" {
		aviso = a.avisoCopia()
	}
	if a.reanudandoImportaciones && g.subida == nil {
		aviso = a.v.Traduce("ui.loading")
	}
	consulta, ampliar := sinResultados(g.texto, esperando, len(linea), g.fallo != nil, len(g.tambien))
	d := Galeria{
		Grupos:        PorMes(linea, a.v.T),
		Buscador:      a.buscador,
		Busqueda:      g.texto,
		Anyos:         a.anyos,
		Anyo:          g.anyo,
		AlAnyo:        a.saltarAAnyo,
		SoloFavoritas: a.soloFavoritas(),
		EnSeleccion:   g.seleccion != nil,
		Seleccion:     g.seleccion,
		Aviso:         aviso,
		Arrastrando:   a.arrastrando,
		Error:         g.error,
		Cargando:      g.cargando,
		Subiendo:      g.subida != nil || a.reanudandoImportaciones,
		// Solo cuando la búsqueda literal no encuentra NADA: ampliar siempre gastaría una
		// llamada por búsqueda sin mejorar las que ya funcionan.
		Ampliar:          ampliar,
		SinResultados:    consulta,
		AlBorrarBusqueda: a.borrarBusqueda,
		Ampliando:        g.ampliando,
		AlAmpliar:        a.ampliarBusqueda,
		AlAbrir:          a.abrirDeLinea,
		AlAlternar:       a.alternar,
		AlMantener:       a.empezarSeleccion,
		AlFavorita:       a.favoritaDeLinea,
		AlIr:             a.irSeccion,
		AlAnyadir:        a.elegirFicheros,
		Copiando:         a.cop.progreso != nil && a.cop.progreso.Total > 0,
		AlComoEstas:      a.parecidasALosResultados,
		AlSeleccionar:    a.entrarEnSeleccion,
		AlSalir:          a.salirDeSeleccion,
		AlAlternarMes:    a.alternarMes,
		Afuera:           func() fyne.Scrollable { return a.scroll },
		Lote: Lote{
			Albumes:      a.albumes,
			AlNuevoAlbum: a.albumDeLaSeleccion,
			AlAnyadirA:   a.anyadirSeleccionA,
			AlDescribir:  a.describirSeleccion,
			AlParar:      func() { a.g.parar = true },
			AlParecidas:  a.parecidasALaSeleccion,
			AlDescargar:  a.descargarSeleccion,
			AlSistema:    a.guardarSeleccionEnSistema,
			AlTirar:      a.tirarSeleccion,
			AlFavoritas:  a.favoritasDeLaSeleccion,
		},
	}
	if len(g.seleccion) > 0 {
		valor, _ := planFavoritas(g.fotos, g.seleccion)
		d.Lote.QuitarFavoritas = !valor
	}
	d.Lote.MarcandoFavoritas = g.marcandoFavoritas
	if a.HayCopia() {
		d.AlCopia = a.abrirCopia
	}
	d.Exportar = a.ExportarFototeca
	d.Importar = a.ImportarFototeca
	if a.ofreceGaleria() {
		d.Lote.ClaveSistema, _ = a.DestinoGaleria.claves()
	} else {
		d.Lote.AlSistema = nil
	}
	if g.lote != nil {
		d.Lote.Describiendo = a.v.T.T("gallery.describing", i18n.Vars{"done": g.lote[0], "total": g.lote[1]})
	}
	// Solo en la de todas y sin búsqueda: un recuerdo entre los resultados de «playa»
	// no es un recuerdo, es ruido.
	d.Cifradas = a.seccionCifradas()
	if !a.soloFavoritas() && g.texto == "" && len(a.rec.lista) > 0 && v.Estado == Exito {
		d.Recuerdos = SeccionRecuerdos(a.v, a.rec.lista, time.Now(), a.abrirRecuerdo, a.ocultarRecuerdos)
	}
	return v, d
}

// cargarMas pide la página siguiente de la línea de tiempo.
func (a *App) cargarMas() {
	a.g.pendiente = false
	if a.cli == nil || a.g.fin || a.g.cargando {
		return
	}
	a.g.cargando = true
	a.pintar()
	gen, cursor, texto, solo, tambien, anyo := a.gen, a.g.cursor, a.g.texto, a.soloFavoritas(), a.g.tambien, a.g.anyo
	a.enSesion(func(vida context.Context) {
		c, fin := ctx(vida)
		defer fin()
		var (
			fotos     []api.Photo
			siguiente string
			err       error
		)
		// Sin cursor y con un año elegido, la primera página empieza en ese año.
		if cursor == "" && anyo != 0 && texto == "" {
			fotos, siguiente, err = a.cli.TimelineDesde(c, strconv.Itoa(anyo), PaginaLinea, solo)
		} else {
			fotos, siguiente, err = a.cli.Timeline(c, cursor, PaginaLinea, solo, texto, tambien...)
		}
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			a.g.cargando = false
			if err != nil {
				a.g.fallo, a.g.error = err, a.textoDeFallo(err)
				a.pintar()
				return
			}
			a.g.fallo, a.g.error = nil, ""
			a.g.fotos = append(a.g.fotos, fotos...)
			a.g.cursor, a.g.fin = siguiente, siguiente == ""
			if a.g.siguienteAlLlegar && a.vis != nil {
				a.g.siguienteAlLlegar = false
				// La lista del visor es la mezclada: la siguiente puede ser una cifrada que
				// entra ahora que la línea del servidor ha llegado a su fecha.
				a.vis.i = min(a.vis.i+1, len(a.vis.lista())-1)
				a.vis.nuevaFoto()
			}
			a.pintar()
			a.quizaMas()
		})
	})
}

// quizaMas pide otra página si el final ya está a menos de una pantalla: es lo que hace
// el IntersectionObserver del centinela, también cuando la primera página no llena la
// ventana y no hay nada que desplazar.
func (a *App) quizaMas() {
	if a.ruta != RutaFotos && a.ruta != RutaFavoritas || a.g.fin || a.g.cargando || a.g.fallo != nil {
		return
	}
	if a.scroll.Content == nil || a.scroll.Size().Height <= 0 {
		return
	}
	falta := a.scroll.Content.MinSize().Height - (a.scroll.Offset.Y + a.scroll.Size().Height)
	if a.capaAnyadir.Visible() {
		// Lo que falta es hasta la última foto, no hasta el final del hueco del «+».
		falta -= ui.ReservaFlotante
	}
	if falta < AntesDelFinal {
		a.cargarMas()
	}
}

// sinResultados es cuándo la galería dice «Ninguna foto coincide con…» y si ofrece
// ampliar (`sinResultados.ts`): una búsqueda hecha, ya cargada, sin fotos y sin fallo.
// Con los términos ya ampliados, ampliar no se vuelve a ofrecer. "" es que no toca.
func sinResultados(texto string, cargando bool, fotos int, fallo bool, tambien int) (consulta string, ampliar bool) {
	consulta = strings.TrimSpace(texto)
	if consulta == "" || cargando || fotos > 0 || fallo {
		return "", false
	}
	return consulta, tambien == 0
}

// borrarBusqueda es «Borrar búsqueda» del vacío de una búsqueda: vacía el buscador —que
// vuelve a pedir la galería entera— y le devuelve el foco, que es donde se va a escribir.
func (a *App) borrarBusqueda() {
	a.buscador.PonerTexto("")
	if a.w != nil {
		a.w.Canvas().Focus(a.buscador)
	}
}

// saltarAAnyo es pulsar un año de la barra (0 es «Recientes»): la línea empieza de nuevo,
// vacía y sin cursor, arriba del todo, y su primera página la sitúa `desde=`.
func (a *App) saltarAAnyo(anyo int) {
	if anyo == a.g.anyo && len(a.g.fotos) > 0 {
		return
	}
	a.cancelarFotos()
	a.gen++
	// La subida en marcha sigue: es de la galería entera, no de la línea que se ve, y
	// perder su marca dejaría empezar otra mientras la primera no ha acabado.
	a.g = estadoGaleria{anyo: anyo, subida: a.g.subida}
	a.scroll.Offset = fyne.NewPos(0, 0)
	a.pintar()
	a.cargarMas()
}

// pedirAnyos pide los años de la barra. Sin bloquear la galería: si falla, se queda la
// que hubiera (o ninguna).
//
// La respuesta vale mientras se siga en la MISMA vista (todas o favoritas), y no se ata
// a `gen` como la línea: buscar o saltar de año suben `gen`, y escribir en el buscador
// antes de que llegasen los años los tiraba, con lo que al borrar la búsqueda la galería
// se quedaba sin barra hasta salir y volver.
func (a *App) pedirAnyos() {
	if a.cli == nil {
		return
	}
	ruta, solo := a.ruta, a.soloFavoritas()
	a.enSesion(func(vida context.Context) {
		c, fin := ctx(vida)
		defer fin()
		anyos, err := a.cli.TimelineYears(c, solo)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		fyne.Do(func() {
			if err != nil || a.ruta != ruta {
				return
			}
			a.anyos = anyos
			a.pintar()
		})
	})
}

// buscar es el `onInput` del buscador: se empieza otra línea, vacía y sin cursor, y se
// pide al cabo de un cuarto de segundo sin teclear.
func (a *App) buscar(texto string) {
	if texto == a.g.texto {
		return
	}
	a.cancelarFotos()
	a.gen++
	gen := a.gen
	a.g = estadoGaleria{texto: texto, pendiente: a.espera > 0}
	a.pintar()
	if a.temporizado != nil {
		a.temporizado.Stop()
	}
	if a.espera <= 0 {
		a.cargarMas()
		return
	}
	a.temporizado = time.AfterFunc(a.espera, func() { a.hilo(gen, a.cargarMas) })
}

// cambiarFavorita marca o desmarca AL VOLVER la llamada, no antes: en «Favoritas»
// desmarcar saca la foto de la lista, y hacerlo optimista la haría desaparecer para
// reaparecer si la llamada falla.
func (a *App) cambiarFavorita(f api.Photo, valor bool) {
	if a.cli == nil {
		return
	}
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := ctx(vida)
		defer fin()
		err := a.cli.SetFavorite(c, f.ID, valor)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if err != nil {
				a.g.error = a.textoDeFallo(err)
				a.pintar()
				return
			}
			quitar := a.soloFavoritas() && !valor
			for i := range a.g.fotos {
				if a.g.fotos[i].ID == f.ID {
					a.g.fotos[i].Favorite = valor
				}
			}
			if quitar {
				a.g.fotos = quitarFoto(a.g.fotos, f.ID)
				a.cerrarVisor()
			}
			a.pintar()
		})
	})
}

// quitarFoto devuelve la lista sin esa foto.
func quitarFoto(l []api.Photo, id string) []api.Photo {
	out := l[:0:0]
	for _, f := range l {
		if f.ID != id {
			out = append(out, f)
		}
	}
	return out
}

// posicion es dónde está una foto en una lista, o -1.
func posicion(l []api.Photo, id string) int {
	for i, f := range l {
		if f.ID == id {
			return i
		}
	}
	return -1
}

// ---- El teclado ----

// teclaSuelta recibe lo que se pulsa sin nada enfocado: con el visor abierto son sus
// atajos, y en la galería Escape sale de la selección (la tabla de atajos.go).
func (a *App) teclaSuelta(e *fyne.KeyEvent) {
	if a.ed != nil {
		a.teclaEditor(e)
		return
	}
	if a.vis != nil {
		a.teclaVisor(e)
		return
	}
	if a.pub.capa != nil && a.ruta == RutaCompartido {
		// Las flechas pasan de foto, como en `VisorCompartido` de la web.
		if paso, es := map[fyne.KeyName]int{fyne.KeyLeft: -1, fyne.KeyRight: 1}[e.Name]; es {
			if f := a.vecinaCompartida(paso); f != nil {
				f()
			}
			return
		}
		// E Inicio y Fin, a la primera y la última, como en el de la galería.
		if a.saltoCompartido(e.Name) {
			return
		}
	}
	if a.pub.capa != nil {
		if e.Name == fyne.KeyEscape {
			a.cerrarPublica()
		}
		return
	}
	if accionDe(atajosGaleria, string(e.Name)) == atajoSalirDeSeleccion && a.g.seleccion != nil && a.enLineaDeTiempo() {
		a.salirDeSeleccion()
	}
}

// runaSuelta: «u» añade fotos (el atajo que pulsa el menú de la app de escritorio), «?»
// abre la hoja de atajos y, con el visor abierto, Espacio, «i» y «z». Solo llega sin nada
// enfocado, que es la mitad de `esAtajoDeApp`: escribir en un campo nunca dispara un
// atajo.
func (a *App) runaSuelta(r rune) {
	if a.ed != nil {
		a.runaEditor(r)
		return
	}
	if a.vis != nil {
		a.runaVisor(r)
		return
	}
	if !a.enLineaDeTiempo() || hayCapa(a.w) {
		return
	}
	switch accionDe(atajosGaleria, teclaDeRuna(r)) {
	case atajoSubir:
		if a.g.subida == nil {
			a.elegirFicheros()
		}
	case atajoBuscar:
		// El buscador de la galería, con lo escrito seleccionado para reemplazarlo.
		if a.w != nil && a.buscador != nil {
			a.w.Canvas().Focus(a.buscador)
			a.buscador.TypedShortcut(&fyne.ShortcutSelectAll{})
		}
	case atajoAyuda:
		a.verAtajos()
	default:
	}
}

// enLineaDeTiempo dice si se está en la línea de tiempo o en Favoritas: donde valen los
// atajos de la galería.
func (a *App) enLineaDeTiempo() bool { return a.ruta == RutaFotos || a.ruta == RutaFavoritas }

// hayCapa dice si hay un diálogo o un menú puesto: con una capa abierta, el teclado es
// de la capa (la otra mitad de `esAtajoDeApp`).
func hayCapa(w fyne.Window) bool {
	return w != nil && w.Canvas().Overlays().Top() != nil
}

// ---- La ventana ----

// claseVentana es en qué tramo de `sm:`/`md:`/`xl:` cae un ancho: lo único de la ventana
// que cambia cómo se construye una pantalla. Lo demás lo recolocan los layouts solos.
func claseVentana(ancho float32) int {
	switch {
	case ancho >= ui.CorteXL:
		return 3
	case ancho >= ui.CorteMD:
		return 2
	case ancho >= ui.CorteSM:
		return 1
	}
	return 0
}

// raizLayout ocupa la ventana con el desplazamiento y avisa a la app cuando el ancho
// cruza un corte, que es cuando hay que reconstruir (las teselas crecen de 9 a 11 y a
// 13rem, los rótulos de los botones aparecen).
type raizLayout struct{ a *App }

func (r *raizLayout) MinSize(objs []fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (r *raizLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
	a := r.a
	if t.Width <= 0 || t.Width == a.v.Ventana {
		return
	}
	a.v.Ventana = t.Width
	// Fuera del Layout: rehacer la pantalla aquí dentro cambiaría los hijos del
	// contenedor que se está colocando.
	if c := claseVentana(t.Width); c != a.corte {
		a.corte = c
		go fyne.Do(a.pintar)
		return
	}
	// Sin cruzar un corte basta con que el desplazamiento vuelva a preguntar el alto:
	// al ancho nuevo caben otras columnas y la galería mide otra cosa.
	go fyne.Do(func() {
		a.scroll.Refresh()
		a.quizaMas()
		// Las capas fijan el ancho al que medir sus párrafos y mandos. También se
		// actualizan entre dos anchos del mismo corte (320 ↔ 568, por ejemplo).
		if a.vis != nil {
			a.pintarVisor()
		}
		if a.ed != nil {
			a.pintarEditor()
		}
	})
}

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
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
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

	// Drive es el cliente de Sytue Drive, donde viven los originales: subir es dejar el
	// fichero en «/Fotos» y luego indexarlo. nil: no se puede subir.
	Drive *api.Drive
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
	// AbrirApp abre otra app de la suite por su esquema (con la web de respaldo): la
	// vuelta del editor a la carpeta de Drive de la que se vino.
	AbrirApp func(producto, ruta string)
	// IA es el cliente de Sytue AI: describir, preguntar, ampliar una búsqueda y editar.
	// nil: se dice lo mismo que dice la web cuando no está configurada.
	IA *api.IA
	// Directorio traduce el correo que se escribe al compartir un álbum al `sub` con que
	// se guarda, y de vuelta para decir quién lo compartió. nil: no se ofrece compartir
	// con personas, y quien comparte conmigo sale como «alguien de tu organización».
	Directorio *api.Directorio

	// NoEncontrado enseña la página «no encontrada» de la suite y dice si había quién.
	// Es lo que contesta la web a un enlace `/p/` que no existe (un 404). nil, o si
	// devuelve false, la pantalla dice «error» como cualquier otra.
	NoEncontrado func() bool

	// lanzar ejecuta una llamada al servicio. En la app va a una goroutine; los
	// guardianes la ponen en línea para leer el estado sin carreras.
	lanzar func(func())
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
	pub estadoPublico
	cmp estadoCompartidos
	aj  api.PhotosSettings
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
	fotos     []api.Photo
	cursor    string
	fin       bool
	texto     string
	tambien   []string
	seleccion map[string]bool
	subida    *[2]int
	lote      *[2]int // describiendo: hechas, total
	parar     bool
	ampliando bool
	// siguienteAlLlegar abre la foto siguiente en el visor en cuanto llegue la página:
	// es el `cargarMas().then(() => setAbierta(abierta + 1))` de la web.
	siguienteAlLlegar bool
}

type estadoAlbumes struct {
	estadoCarga
	lista    []api.Album
	montones [][]api.Photo
	buscando bool
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
		lanzar: func(f func()) { go f() },
	}
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
	a.raiz = container.New(&raizLayout{a: a}, a.scroll, a.peg.capa)
	a.corte = claseVentana(v.Ventana)
	// El tema se cambia en caliente desde el menú de cuenta (o desde otra app de la
	// suite): las pantallas de Photos se arman con colores ya resueltos, así que hay que
	// rehacerlas. Sin esto se quedaban en la variante con la que arrancó la app.
	if app := fyne.CurrentApp(); app != nil {
		app.Settings().AddListener(func(fyne.Settings) {
			if varianteActual() != a.v.Variante {
				fyne.Do(func() {
					a.pintar()
					if a.vis != nil {
						a.pintarVisor()
					}
				})
			}
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
	}
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
		a.buscador.PonerTexto("") // dispara AlCambiar con "", que no cambia nada
		a.pintar()
		a.cargarMas()
		a.pedirRecuerdos()
	case RutaAlbumes:
		a.alb = estadoAlbumes{estadoCarga: estadoCarga{cargando: true}}
		a.pintar()
		a.pedirAlbumes()
		a.pedirAjustes()
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
func (a *App) AlFondo() { a.cancelarFotos() }

// AlVolver reconstruye los rasters retirados al salir de primer plano.
func (a *App) AlVolver() {
	a.pintar()
	if a.vis != nil {
		a.pintarVisor()
	}
	if a.pub.abierta != nil {
		a.pintarPublica()
	}
}

// irSeccion es lo que hace una pestaña de la nav.
func (a *App) irSeccion(s Seccion) { a.Ir(Ruta(s)) }

// hilo lleva `f` al hilo de la interfaz si la generación no ha cambiado: una respuesta
// que llega después de cambiar de pantalla es de otra pantalla y se tira.
func (a *App) hilo(gen int, f func()) {
	fyne.Do(func() {
		if gen == a.gen {
			f()
		}
	})
}

// ctx es el de una llamada al servicio: con tope, para que una red colgada no deje la
// rueda girando para siempre.
func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 60*time.Second)
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
	a.scroll.Content = o
	a.scroll.Refresh()
	a.pegar()
	if a.vis != nil {
		a.pintarVisor()
	}
}

// vistaDe decide en cuál de los seis estados está una pantalla.
func (a *App) vistaDe(e estadoCarga, sinDatos, vacia bool) Vista {
	v := a.v
	v.AlEntrar = a.AlEntrar
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

// estadoDeFallo traduce un error del servicio a uno de los estados de la pantalla.
func estadoDeFallo(err error) Estado {
	var e *api.Error
	switch {
	// Un 401 es la sesión, no el permiso: el cliente ya ha renovado y reintentado, así
	// que el servicio no acepta a esta persona y lo que toca es volver a entrar. Antes
	// salía «No tienes acceso» (el 403) y la persona buscaba un permiso que tenía.
	case errors.Is(err, auth.ErrLoginRequired), errors.As(err, &e) && e.Status == 401:
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
	g := a.g
	v := a.vistaDe(g.estadoCarga, len(g.fotos) == 0, !g.cargando && len(g.fotos) == 0 && g.fallo == nil && g.texto == "")
	aviso := g.aviso
	if g.subida != nil {
		aviso = a.v.T.T("gallery.uploading", i18n.Vars{"done": g.subida[0], "total": g.subida[1]})
	}
	d := Galeria{
		Grupos:        PorMes(g.fotos, a.v.T),
		Buscador:      a.buscador,
		Busqueda:      g.texto,
		SoloFavoritas: a.soloFavoritas(),
		EnSeleccion:   g.seleccion != nil,
		Seleccion:     g.seleccion,
		Aviso:         aviso,
		Arrastrando:   a.arrastrando,
		Error:         g.error,
		Cargando:      g.cargando,
		Subiendo:      g.subida != nil,
		// Solo cuando la búsqueda literal no encuentra NADA: ampliar siempre gastaría una
		// llamada por búsqueda sin mejorar las que ya funcionan.
		Ampliar:       strings.TrimSpace(g.texto) != "" && !g.cargando && len(g.fotos) == 0 && len(g.tambien) == 0 && g.fallo == nil,
		Ampliando:     g.ampliando,
		AlAmpliar:     a.ampliarBusqueda,
		AlAbrir:       func(f api.Photo) { a.abrirEnGaleria(f.ID) },
		AlAlternar:    a.alternar,
		AlFavorita:    func(f api.Photo) { a.cambiarFavorita(f, !f.Favorite) },
		AlIr:          a.irSeccion,
		AlAnyadir:     a.elegirFicheros,
		AlComoEstas:   a.parecidasALosResultados,
		AlSeleccionar: a.entrarEnSeleccion,
		AlSalir:       a.salirDeSeleccion,
		AlAlternarMes: a.alternarMes,
		Lote: Lote{
			Albumes:      a.albumes,
			AlNuevoAlbum: a.albumDeLaSeleccion,
			AlAnyadirA:   a.anyadirSeleccionA,
			AlDescribir:  a.describirSeleccion,
			AlParar:      func() { a.g.parar = true },
			AlParecidas:  a.parecidasALaSeleccion,
			AlDescargar:  a.descargarSeleccion,
			AlTirar:      a.confirmarTirarSeleccion,
		},
	}
	if g.lote != nil {
		d.Lote.Describiendo = a.v.T.T("gallery.describing", i18n.Vars{"done": g.lote[0], "total": g.lote[1]})
	}
	// Solo en la de todas y sin búsqueda: un recuerdo entre los resultados de «playa»
	// no es un recuerdo, es ruido.
	if !a.soloFavoritas() && g.texto == "" && len(a.rec.lista) > 0 && v.Estado == Exito {
		d.Recuerdos = SeccionRecuerdos(a.v, a.rec.lista, time.Now(), a.abrirRecuerdo, a.ocultarRecuerdos)
	}
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

// cargarMas pide la página siguiente de la línea de tiempo.
func (a *App) cargarMas() {
	if a.cli == nil || a.g.fin || a.g.cargando {
		return
	}
	a.g.cargando = true
	a.pintar()
	gen, cursor, texto, solo, tambien := a.gen, a.g.cursor, a.g.texto, a.soloFavoritas(), a.g.tambien
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		fotos, siguiente, err := a.cli.Timeline(c, cursor, PaginaLinea, solo, texto, tambien...)
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
				a.vis.i = min(a.vis.i+1, len(a.g.fotos)-1)
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
	if falta < AntesDelFinal {
		a.cargarMas()
	}
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
	a.g = estadoGaleria{texto: texto}
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
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		err := a.cli.SetFavorite(c, f.ID, valor)
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
// atajos, y en la galería Escape sale de la selección.
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
	}
	if a.pub.capa != nil {
		if e.Name == fyne.KeyEscape {
			a.cerrarPublica()
		}
		return
	}
	if e.Name == fyne.KeyEscape && a.g.seleccion != nil && (a.ruta == RutaFotos || a.ruta == RutaFavoritas) {
		a.salirDeSeleccion()
	}
}

// runaSuelta: «u» añade fotos (el atajo que pulsa el menú de la app de escritorio) y,
// con el visor abierto, Espacio, «i» y «z». Solo llega sin nada enfocado, que es la
// mitad de `esAtajoDeApp`: escribir en un campo nunca dispara un atajo.
func (a *App) runaSuelta(r rune) {
	if a.ed != nil {
		a.runaEditor(r)
		return
	}
	if a.vis != nil {
		a.runaVisor(r)
		return
	}
	if r == 'u' && (a.ruta == RutaFotos || a.ruta == RutaFavoritas) && a.g.subida == nil && !hayCapa(a.w) {
		a.elegirFicheros()
	}
}

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
	})
}

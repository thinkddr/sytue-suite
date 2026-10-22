// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

// Sin marcador `paridad-web:` de `edit.astro` todavía: el editor abre, edita y guarda,
// pero a la cinta le faltan pestañas y al carril paneles (ver el informe de paridad).
// Declararlo portado sería fingir cobertura.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/coedicion"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// PausaTitulo es el `RENAME_MS` de `useDocument.ts`: el título se manda 800 ms después
// de la última tecla, no en cada una.
const PausaTitulo = 800 * time.Millisecond

// PausaGuardado es el `AUTOSAVE_MS`: el contenido se guarda 1,5 s después de la última
// edición. PausaReintento es el `RETRY_MS` tras un fallo de red.
const (
	PausaGuardado  = 1500 * time.Millisecond
	PausaReintento = 5 * time.Second
)

// App es Sytue Docus entero: la portada con los documentos de texto y, al abrir uno, el
// editor con el folio. Las dos son de ESTA app —en la web son `/docus/` y
// `/edit?id=` del mismo subdominio—, así que abrir un documento no lanza otra: cambia la
// pantalla de esta ventana.
type App struct {
	T     func(string, i18n.Vars) string
	loc   i18n.Locale
	Svc   *api.Docs
	Red   *offline.Monitor
	Plazo time.Duration
	// Asistir es Sytue AI (`api.IA.Asistir`), con el token de la persona; nil, sin panel
	// «Asistente». Idioma es la lengua de la sesión, en la que contesta.
	Asistir func(context.Context, api.PeticionIA) (string, error)
	// AlTitulo avisa del título de la ventana: el del documento abierto, o vacío en la
	// portada (la ventana pone entonces el nombre de la app), como Calcus y Diapos.
	AlTitulo func(string)
	Idioma   string
	// EnVivo abre el canal en vivo de la coedición; sin él, lo de los demás llega por el
	// sondeo. Las pruebas lo apagan: no hay WebSocket en su servidor falso.
	EnVivo   bool
	portada  *editor.Portada
	avisos   *ui.PilaAvisos
	raiz     *fyne.Container
	abierto  *abierto
	mu       sync.Mutex
	pausaTit *time.Timer
	// lanzar corre una petición fuera del hilo de la interfaz; las pruebas la cambian
	// por una llamada directa.
	lanzar func(func())
	// programar corre f dentro de d en el hilo de la interfaz; las pruebas lo cambian.
	programar func(time.Duration, func()) func()
	// Ventana es la de la app, para «Pantalla completa». Puede ser nil (las pruebas).
	Ventana fyne.Window
	// EnlaceDoc y EnlacePublico escriben las direcciones de la web que copia «Compartir»:
	// la del documento y la de un enlace público por su token. Las pone el binario, que
	// sabe el dominio de la organización. nil: «Compartir» no sale.
	EnlaceDoc     func(id string) string
	EnlacePublico func(token string) string
	// Directorio resuelve a quién se comparte (ver editor.Compartir.Dir).
	Directorio *api.Directorio
	// AbrirArchivo es «Abrir un archivo» del menú Archivo; lo pone el binario, que es
	// quien tiene el `cargar` de «Abrir con» y el selector del sistema. nil: no sale.
	AbrirArchivo *editor.AbrirArchivo
	// carpeta es dónde se dejan las descargas; las pruebas la cambian por una temporal.
	carpeta func() (string, error)
	// actor identifica esta sesión en el CRDT, como el `crypto.randomUUID()` de cada
	// pestaña de la web: el `dot` de cada guardado es (actor, contador).
	actor string
	// Reclamos dice quién ha entrado, para firmar las marcas del control de cambios
	// (`setTrackAuthor` de EditorApp.tsx). Lo pone el binario; nil, la reserva de la web.
	Reclamos func(context.Context) (auth.Reclamos, error)
}

// abierto es el documento que hay en el editor: su contexto y su folio.
type abierto struct {
	id    string
	ctx   *editor.Contexto
	folio *Folio
	marco *editor.Marco
	// base es el registro de contenido tal y como se leyó o se guardó por última vez:
	// si en el servidor ya no es este, otro ha guardado entretanto y no se pisa.
	base api.DocContent
	// guardado son los cambios del folio que ya están en el servidor.
	guardado  int
	guardando bool
	conflicto bool
	cuenta    uint64
	cancelar  func() // la pausa del guardado en marcha

	comentarios *comentarios // el panel, una vez abierto

	// La coedición (`coedicion.go`): la secuencia, el reloj y la seq que se han visto, las
	// ops por carácter aún sin confirmar, cuántas personas más hay y el sondeo. forzar
	// pide guardar aunque el folio no haya cambiado: tras una fusión, el registro del
	// servidor aún tiene la vista del otro.
	co          *coedicion.CoEdicion
	lamport     uint64
	seq         int64
	pendientes  []coedicion.Op
	presentes   int
	rondas      int
	sondeando   bool
	forzar      bool
	pararSondeo func()
	// El canal en vivo: cerrarlo, si está abierto (entonces no se sondea) y la última seq
	// vista, que lee la rutina del canal al reconectar.
	cerrarCanal  func()
	canalAbierto bool
	seqVista     atomic.Int64
}

// NuevaApp monta Docus. `svc` puede ser nil: la portada se pinta y no pide nada, que es
// lo que quieren los guardianes.
func NuevaApp(t func(string, i18n.Vars) string, loc i18n.Locale, svc *api.Docs) *App {
	semilla := make([]byte, 8)
	_, _ = rand.Read(semilla)
	a := &App{
		T: t, loc: loc, Svc: svc, Plazo: 30 * time.Second, EnVivo: true, avisos: ui.NuevaPilaAvisos(),
		lanzar: func(f func()) { go f() }, actor: "fyne-" + hex.EncodeToString(semilla),
	}
	a.carpeta = ficheros.Carpeta
	a.programar = func(d time.Duration, f func()) func() {
		t := time.AfterFunc(d, func() { fyne.Do(f) })
		return func() { t.Stop() }
	}
	a.portada = editor.NuevaPortada(editor.Documento, t, loc, svc)
	a.portada.Avisos = a.avisos
	a.portada.AlAbrir = a.Abrir
	a.raiz = container.NewStack(a.portada.Objeto(), a.avisos)
	return a
}

// Objeto es lo que se monta en la ventana.
func (a *App) Objeto() fyne.CanvasObject { return a.raiz }

// Portada es la portada, para quien necesite tocarla (la conectividad, el inicio de
// sesión).
func (a *App) Portada() *editor.Portada { return a.portada }

// Arrancar pide la primera página de la portada.
func (a *App) Arrancar() {
	editor.EngancharAtras(a.raiz, a.portada, func() bool { return a.abierto != nil }, a.Volver)
	a.portada.Cargar()
}

func (a *App) t(clave string) string {
	if a.T == nil {
		return clave
	}
	return a.T(clave, nil)
}

func (a *App) mostrar(o fyne.CanvasObject) {
	a.raiz.Objects[0] = o
	a.raiz.Refresh()
}

// Volver cierra el documento y enseña la portada, con la lista al día: el título o la
// fecha pueden haber cambiado mientras estaba abierto.
func (a *App) Volver() {
	a.mu.Lock()
	if a.pausaTit != nil {
		a.pausaTit.Stop()
		a.pausaTit = nil
	}
	a.mu.Unlock()
	if ab := a.abierto; ab != nil && ab.cancelar != nil {
		// Lo que quedara por guardar sale ya: salir no es perderlo.
		ab.cancelar()
		a.guardar(ab)
	}
	a.abierto = nil
	editor.DejarDeEscucharRol()
	a.titulo("")
	a.mostrar(a.portada.Objeto())
	a.portada.Cargar()
}

// titulo pasa el título a la ventana, si alguien escucha.
func (a *App) titulo(s string) {
	if a.AlTitulo != nil {
		a.AlTitulo(s)
	}
}

// recargarPorRol vuelve a abrir el documento cuando el dueño me deja editarlo: el rol
// nuevo quita la franja y enciende la cinta y el guardado, sin que nadie recargue.
func (a *App) recargarPorRol(ab *abierto) {
	if a.abierto == ab {
		a.lanzar(func() { a.cargar(ab) })
	}
}

// Abrir enseña el editor con ese documento y lo pide al servicio.
func (a *App) Abrir(d api.Document) { a.AbrirID(d.ID, d.Title) }

// AbrirID abre por identificador, con el título que ya se sepa (puede ir vacío: llega
// con el documento).
func (a *App) AbrirID(id, titulo string) {
	a.titulo(titulo)
	ctx := &editor.Contexto{
		Tipo: editor.Documento, Titulo: titulo, Estado: editor.Cargando,
		Zoom: 100, Regla: true, T: a.T, Loc: a.loc, Avisar: a.Avisar, IA: a.Asistir != nil,
	}
	folio := NuevoFolio(ctx)
	ctx.Lienzo = folio
	ctx.AjustarAlAncho = folio.AjustarAlAncho
	a.archivo(ctx)
	if w := a.Ventana; w != nil {
		ctx.PantallaCompleta = func() { w.SetFullScreen(!w.FullScreen()) }
		ctx.EnPantallaCompleta = w.FullScreen
	}
	marco := editor.NuevoMarco(ctx, registroDocus(a.loc), a.Volver)
	ab := &abierto{id: id, ctx: ctx, folio: folio, marco: marco}
	delFolio := panelesDe(folio, a.T, a.loc)
	ctx.Paneles = func(p editor.Panel) fyne.CanvasObject {
		if p == editor.PanelComentarios && a.Svc != nil {
			return a.panelComentarios(ab)
		}
		if p == editor.PanelIA && a.Asistir != nil {
			return a.panelAsistente(ab)
		}
		return delFolio(p)
	}
	a.abierto = ab
	a.mostrar(marco)
	if a.Svc == nil {
		return
	}
	a.lanzar(func() { a.cargar(ab) })
}

// archivo pone en el contexto lo que el menú Archivo ofrece en nativo: «Nuevo documento»
// (el mismo alta que «En blanco» de la portada) y «Abrir un archivo».
func (a *App) archivo(ctx *editor.Contexto) {
	if a.Svc != nil {
		ctx.Nuevo = a.portada.Crear
	}
	if a.AbrirArchivo != nil {
		ctx.Abrir = a.AbrirArchivo.Abrir
	}
}

// Avisar enseña un aviso de peligro en la pila de la ventana.
func (a *App) Avisar(titulo, texto string) {
	a.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, titulo, texto))
}

// AbrirConvertido enseña un fichero de Drive ya convertido —el enlace entrante—: sin id
// y en solo lectura, porque no es un documento de Docs y no hay dónde guardarlo.
// «Volver» lleva a la portada, como desde cualquier documento.
func (a *App) AbrirConvertido(titulo, html string) {
	o, cargar := pantallaConCarga(a.T, a.loc, a.Volver, a.archivo)
	cargar(titulo, html)
	a.abierto = nil
	a.mostrar(o)
}

// cargar pide la ficha (título y papel) y el contenido. Corre en su propia rutina.
func (a *App) cargar(ab *abierto) {
	c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
	defer cancelar()
	doc, rol, err := a.Svc.Get(c, ab.id)
	var cont api.DocContent
	if err == nil {
		cont, err = a.Svc.Content(c, ab.id, ab.ctx.Tipo.ClaveContenido())
	}
	autor := a.autor(c)
	hayRed := a.Red == nil || a.Red.State() != offline.Offline
	fyne.Do(func() {
		if a.abierto != ab {
			return // se cerró mientras llegaba
		}
		ctx := ab.ctx
		if err != nil {
			ctx.Estado = editor.EstadoDe(err, hayRed, 1)
			a.mostrar(a.pantallaFallo(ab))
			return
		}
		ab.base = cont
		ctx.Titulo = doc.Title
		a.titulo(doc.Title)
		ab.folio.Autor = autor
		// Se escribe con permiso Y si el folio entiende el documento entero; si no, se
		// lee, y la franja dice por qué. El título se deja cambiar con permiso: no toca
		// el contenido.
		entendido := ab.folio.Cargar(cont.Value)
		ab.guardado, ab.conflicto, ab.forzar = 0, false, false // lo que hay en pantalla es lo del servidor
		if ab.pararSondeo != nil {
			ab.pararSondeo() // se vuelve a leer (restaurar una versión): la secuencia es otra
		}
		if ab.cerrarCanal != nil {
			ab.cerrarCanal()
			ab.canalAbierto = false
		}
		if entendido && ctx.Tipo == editor.Documento {
			a.iniciarCoedicion(ab, cont)
		}
		ctx.SoloLectura = !api.CanWrite(rol) || !entendido
		// La salida de la franja: pedir acceso, o concederlo si soy el dueño. El canal de
		// coedición ya está abierto para un texto entendido, y su `role` llega a `enVivo`.
		editor.EngancharPeticionEdicion(ctx, a.Svc, ab.id, rol, !entendido || ctx.Tipo != editor.Documento,
			ab.marco.Recomponer, func() { a.recargarPorRol(ab) })
		if api.CanWrite(rol) {
			ctx.AlRenombrar = func(s string) { a.renombrar(ab, s) }
		}
		ctx.EnRastreo = ab.folio.EnRastreo
		ctx.Notificar = func(tono ui.TonoAviso, titulo string) { a.avisos.Notificar(ui.NuevoAviso(tono, titulo, "")) }
		if !ctx.SoloLectura {
			ab.folio.AlEditar = func() { a.programarGuardado(ab) }
			ab.folio.AlGuardarYa = func() { a.guardarYa(ab) }
			ctx.AlternarRastreo = func() {
				ab.folio.AlternarRastreo()
				ab.marco.Recomponer()
			}
			ctx.ResolverCambios = ab.folio.ResolverCambios
		}
		// Descargar y guardar versión: con cualquier papel se descarga (la web lo deja al
		// lector); una versión la guarda quien escribe (el servicio pide editor).
		ctx.Exportar = func(f string) { a.exportar(ab, f) }
		ctx.ExportarPlantilla = func(f string) { a.exportarCon(ab, f, true) }
		ctx.Imprimir = func() { a.exportar(ab, editor.FormatoImprimir) }
		if api.CanWrite(rol) {
			ctx.Bloques = &editor.BloquesDoc{
				Svc: a.portada.Svc, Lanzar: a.lanzar, Idioma: string(a.portada.Loc),
				Colocar: func(contenido string) { ctx.Mandar(cmdBloque, contenido) },
			}
			ctx.GuardarVersion = func(etiqueta string) { a.servicio().GuardarVersion(ab.id, etiqueta) }
			ctx.RestaurarVersion = func(v api.DocVersion) { a.restaurarVersion(ab, v) }
		}
		ctx.Versiones = func(listo func([]api.DocVersion)) { a.servicio().Versiones(ab.id, listo) }
		if rol == "owner" {
			ctx.Actividad = func(listo func([]api.DocActividad, error)) { a.servicio().Actividad(ab.id, listo) }
		}
		if a.EnlaceDoc != nil {
			ctx.Compartir = func(desde fyne.CanvasObject) { a.compartir(ab).Mostrar(desde) }
		}
		a.configurarComentario(ab, rol)
		ctx.SinRed = !hayRed
		ctx.Estado = editor.Listo
		ab.marco.Recomponer()
	})
}

// configurarComentario permite comentar a quien tiene ese papel, con la selección como ancla.
func (a *App) configurarComentario(ab *abierto, rol string) {
	ctx := ab.ctx
	if api.CanComment(rol) {
		// Como la web: el texto se pregunta en un diálogo y, con selección, el
		// comentario se ancla a ella con un `<mark data-comment-id>`.
		ctx.NuevoComentario = func() {
			lienzo := fyne.CurrentApp().Driver().CanvasForObject(ab.marco)
			if lienzo == nil {
				return
			}
			ui.Pregunta(a.t("comments.newPrompt"), "", a.t("ui.cancel"), a.t("ui.save"), a.t("ui.required"),
				func(cuerpo string) { a.comentarAnclado(ab, cuerpo) }).Mostrar(lienzo)
		}
	}
}

// autor es quien firma las marcas: el `name` y el `sub` de la sesión, con las reservas de
// `userFromClaims` (el correo, y si no «Usuario»). Sin sesión, el cero: el folio usa la
// reserva de la web.
func (a *App) autor(c context.Context) rico.Autor {
	if a.Reclamos == nil {
		return rico.Autor{}
	}
	r, err := a.Reclamos(c)
	if err != nil || r.Sub == "" {
		return rico.Autor{}
	}
	nombre := r.Name
	if nombre == "" {
		nombre = r.Email
	}
	if nombre == "" {
		nombre = "Usuario"
	}
	return rico.Autor{Nombre: nombre, ID: r.Sub}
}

// pantallaFallo es lo que la web pinta cuando el documento no se puede abrir: el
// editor entero cede el sitio a un EmptyState centrado sobre `surface`, con el aviso de
// 40 en `danger`, «Reintentar» y «Volver». Es un solo estado para todos los fallos, como
// en la web (`status === 'error'`).
func (a *App) pantallaFallo(ab *abierto) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	e := ui.NuevoVacio(a.t("editor.error.title"), a.t("editor.error.description"))
	e.Ilustracion = iconos.Trazo(iconos.Aviso, 40, "danger", v).Objeto()
	reintentar := ui.NuevoBoton(a.t("editor.error.retry"), func() { a.AbrirID(ab.id, ab.ctx.Titulo) }).
		Con(ui.Primario, ui.TallaSM)
	volver := ui.NuevoBoton(a.t("editor.error.back"), a.Volver).Con(ui.Secundario, ui.TallaSM)
	e.Accion = container.New(ui.Fila{Hueco: tema.Espacio(2)}, reintentar, volver)
	fondo := canvas.NewRectangle(tema.Color("surface", v))
	return container.NewStack(fondo, e)
}

// renombrar programa el cambio de título tras la pausa de la web. Un título en blanco
// no se manda (`setTitle` hace lo mismo): el documento se queda con el que tenía.
func (a *App) renombrar(ab *abierto, titulo string) {
	titulo = strings.TrimSpace(titulo)
	if titulo == "" {
		return
	}
	a.titulo(titulo)
	a.ponerGuardado(ab, editor.Sucio)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pausaTit != nil {
		a.pausaTit.Stop()
	}
	a.pausaTit = time.AfterFunc(PausaTitulo, func() { a.mandarTitulo(ab, titulo) })
}

// mandarTitulo es la petición que la pausa de renombrar deja para el final.
func (a *App) mandarTitulo(ab *abierto, titulo string) {
	c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
	defer cancelar()
	err := a.Svc.Rename(c, ab.id, titulo)
	fyne.Do(func() {
		if a.abierto != ab {
			return
		}
		switch {
		case err != nil:
			a.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, a.t("rename.error"), ""))
			a.ponerGuardado(ab, editor.FalloAlGuardar)
		case ab.folio.Cambios() == ab.guardado && !ab.guardando:
			// «Guardado» solo si tampoco queda contenido por guardar (`useDocument.ts`).
			a.ponerGuardado(ab, editor.Limpio)
		}
	})
}

// ponerGuardado cambia el chip de la cabecera, y repinta el marco solo si cambia: en cada
// tecla no hay nada nuevo que anunciar.
func (a *App) ponerGuardado(ab *abierto, g editor.Guardado) {
	if ab.ctx.Guardado != g {
		ab.ctx.Guardado = g
		ab.marco.Recomponer()
	}
}

// programarGuardado es el autoguardado: tras cada edición el chip dice «Cambios sin
// guardar» y el contenido sale [PausaGuardado] después de la última.
func (a *App) programarGuardado(ab *abierto) {
	if ab.conflicto {
		return
	}
	if !ab.guardando {
		a.ponerGuardado(ab, editor.Sucio)
	}
	if ab.cancelar != nil {
		ab.cancelar()
	}
	ab.cancelar = a.programar(PausaGuardado, func() { a.guardar(ab) })
}

// guardarYa es Ctrl/Cmd+S: adelanta el guardado programado, y el chip lo cuenta.
func (a *App) guardarYa(ab *abierto) {
	if ab.cancelar != nil {
		ab.cancelar()
		ab.cancelar = nil
	}
	a.guardar(ab)
}

// guardar manda el HTML por el camino de la web —un `set` del registro `doc:html`— con
// la comprobación de delante: si otro ha guardado entretanto, NO se pisa su trabajo; el
// chip dice que no se pudo guardar y lo escrito se queda en pantalla.
//
// autoritativo=true: es lo que manda la web cuando quien guarda no lleva la secuencia por
// carácter (`coedicion.ts`); sin la marca, un navegador abierto seguiría fusionando sobre
// un texto que ya no existe.
//
// Con la secuencia por carácter (`coedicion.go`) no hay conflicto que señalar: lo de los
// demás se fusiona. Este camino queda para un documento de antes con alguien más dentro,
// que no se puede sembrar a dos.
func (a *App) guardar(ab *abierto) {
	ab.cancelar = nil
	if a.Svc == nil || ab.guardando || ab.conflicto || (ab.folio.Cambios() == ab.guardado && !ab.forzar) {
		return
	}
	html, n := ab.folio.Serializar(), ab.folio.Cambios()
	ab.guardando, ab.forzar = true, false
	if ab.conCoedicion() {
		a.ponerGuardado(ab, editor.Guardando)
		enviar := a.loteCoedicion(ab, html, n)
		a.lanzar(func() { _ = enviar() }) //nolint:errcheck // el resultado lo recoge guardadoHecho
		return
	}
	ab.cuenta++
	dot, base := api.DocDot{Actor: a.actor, Counter: ab.cuenta}, ab.base
	a.ponerGuardado(ab, editor.Guardando)
	a.lanzar(func() {
		c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
		defer cancelar()
		res, err := a.Svc.SaveChecked(c, ab.id, ab.ctx.Tipo.ClaveContenido(), html, base, dot, true)
		fyne.Do(func() { a.guardadoHecho(ab, n, res, err) })
	})
}

// guardadoHecho recoge la respuesta del guardado en el hilo de la interfaz.
func (a *App) guardadoHecho(ab *abierto, n int, res api.DocContent, err error) {
	ab.guardando = false
	switch {
	case errors.Is(err, api.ErrDocConflicto):
		ab.conflicto = true
		a.ponerGuardado(ab, editor.FalloAlGuardar)
	case err != nil:
		a.ponerGuardado(ab, editor.FalloAlGuardar)
		if a.abierto == ab {
			ab.cancelar = a.programar(PausaReintento, func() { a.guardar(ab) })
		}
	default:
		ab.base, ab.guardado = res, n
		if ab.folio.Cambios() != n {
			// Se siguió escribiendo mientras se guardaba: lo nuevo sale en la próxima pausa.
			a.programarGuardado(ab)
			return
		}
		a.ponerGuardado(ab, editor.Limpio)
		a.fusionar(ab) // lo que llegó de los demás mientras se escribía
	}
}

// exportar es «Descargar como…»: guarda antes lo pendiente —el servicio serializa desde
// SU estado, y un autoguardado en cola exportaría una versión anterior a lo que se ve—,
// pide el formato y lo deja en la carpeta de descargas, sin pisar nada que ya esté allí.
func (a *App) exportar(ab *abierto, formato string) {
	a.exportarCon(ab, formato, false)
}

func (a *App) exportarCon(ab *abierto, formato string, plantilla bool) {
	if a.Svc == nil || ab.ctx.Exportando != "" {
		return
	}
	ab.ctx.Exportando = formato
	ab.marco.Recomponer()
	if ab.cancelar != nil {
		ab.cancelar()
		ab.cancelar = nil
	}
	sinCambios := !ab.folio.Documento().Editable() || ab.folio.Cambios() == ab.guardado || ab.conflicto
	html, n := ab.folio.Serializar(), ab.folio.Cambios()
	var enviar func() error
	if !sinCambios && !ab.ctx.SoloLectura && ab.conCoedicion() {
		enviar = a.loteCoedicion(ab, html, n)
	}
	ab.cuenta++
	dot, base := api.DocDot{Actor: a.actor, Counter: ab.cuenta}, ab.base
	antes := func() error {
		if sinCambios || ab.ctx.SoloLectura {
			return nil
		}
		if enviar != nil {
			return enviar()
		}
		c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
		defer cancelar()
		res, err := a.Svc.SaveChecked(c, ab.id, ab.ctx.Tipo.ClaveContenido(), html, base, dot, true)
		fyne.Do(func() { a.guardadoHecho(ab, n, res, err) })
		return err
	}
	descargar := a.servicio().Descargar
	if plantilla {
		descargar = a.servicio().DescargarPlantilla
	}
	descargar(ab.id, ab.ctx.Titulo, ab.ctx.Tipo, formato, antes, func() {
		ab.ctx.Exportando = ""
		ab.marco.Recomponer()
	})
}

// servicio son las acciones comunes de los editores contra el servicio de Docs.
func (a *App) servicio() editor.Servicio {
	return editor.Servicio{
		Svc: a.Svc, Plazo: a.Plazo, T: a.T, Lanzar: a.lanzar, Carpeta: a.carpeta,
		Avisar: func(tono ui.TonoAviso, titulo string) { a.avisos.Notificar(ui.NuevoAviso(tono, titulo, "")) },
	}
}

// restaurarVersion devuelve el documento a esa versión y lo vuelve a leer. No destruye
// nada: el servicio añade las operaciones que llevan hasta ella, y el estado de ahora
// queda en el historial.
func (a *App) restaurarVersion(ab *abierto, v api.DocVersion) {
	if a.Svc == nil {
		return
	}
	a.servicio().RestaurarVersion(ab.id, v, func() {
		// Lo que hubiera sin guardar se descarta a propósito: se ha pedido volver a otra
		// versión, y guardarlo después la pisaría.
		if ab.cancelar != nil {
			ab.cancelar()
			ab.cancelar = nil
		}
		a.lanzar(func() { a.cargar(ab) })
	})
}

// compartir es el panel de «Compartir» de este documento.
func (a *App) compartir(ab *abierto) *editor.Compartir {
	return &editor.Compartir{
		T: a.T, Svc: a.Svc, ID: ab.id, Titulo: ab.ctx.Titulo,
		Enlace: a.EnlaceDoc(ab.id), EnlacePublico: a.EnlacePublico,
		Avisos: a.avisos, Lanzar: a.lanzar, Plazo: a.Plazo, Dir: a.Directorio,
	}
}

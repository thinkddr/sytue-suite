// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// App es Sytue Contacts con datos: la Pantalla, que solo sabe pintarse, y lo único de
// este paquete que habla con el servicio. Es el `ContactsApp` de la web sin su JSX.
//
// Todo el estado vive aquí y la Pantalla se DERIVA de él en `volcar`: así los
// guardianes de píxel siguen pintando una Pantalla a mano, sin red.
type App struct {
	vidaMu              sync.Mutex
	vida                context.Context
	cancelarVida        context.CancelFunc
	generacion          uint64
	cerrada             bool
	trabajos            int
	trabajosTerminados  chan struct{}
	organizacionEnCurso bool
	cargaLibretas       uint64
	cargaContactos      uint64

	Pantalla *Pantalla

	svc    *api.Contacts
	avisos *ui.PilaAvisos
	raiz   *fyne.Container

	// Abridor es quien delega en el sistema al saltar a otra app (Mail, Calendar);
	// vacío es la app de Fyne en marcha. Apex es el dominio de la suite; vacío es
	// producción. Org es la organización activa de la sesión, leída VIVA del token, que
	// encabeza los enlaces a otras apps (ADR 0056); nil es «no la sé». Se dejan
	// abiertos para el guardián, que no tiene sistema.
	Abridor api.Abridor
	Apex    string
	Org     func() string
	// Almacen guarda lo que la web deja en `localStorage` por persona: el orden de la
	// libreta (orden.go). Nil: por nombre, sin guardar.
	Almacen Almacen
	// ElegirFichero abre el selector de ficheros del sistema con esas extensiones y le
	// pasa a `listo` lo leído, en el hilo de la interfaz. nil: no hay selector, y
	// «Importar» sale apagado.
	ElegirFichero func(extensiones []string, listo func(datos []byte))
	// Descargas guarda un fichero descargado. nil: la carpeta de descargas del
	// sistema; el guardián lo cambia para no escribir en el disco.
	Descargas func(nombre string, contenido []byte) error
	// Plazo es cuánto se espera a una petición.
	Plazo time.Duration
	// enFondo lanza el trabajo de red. En la app es una goroutine; el guardián lo
	// cambia por una llamada directa y así no tiene que esperar a nadie.
	enFondo func(func())
	// enFondoFila es el de las fotos que pide una fila al pintarse. Va aparte porque se
	// lanza DESDE el relleno de la lista, que no admite que se la vuelva a refrescar
	// dentro: el guardián lo encola y lo corre después.
	enFondoFila func(func())
	// enUIFoto hands completed reads back to the UI; tests can hold this handoff.
	enUIFoto func(func())

	todos []api.Contact // los de la libreta activa, ya ordenados
	// La caché pertenece al ID de contacto y a la revisión de su tarjeta.
	fotos         map[string]fotoGuardada
	fotosPedidas  map[string]peticionFoto
	secuenciaFoto uint64
	turnosFoto    chan struct{}
	// pedido es el contacto que pidió un enlace, hasta que su libreta carga; y
	// libretaPedida, su libreta si el enlace llegó antes que las libretas.
	pedido, libretaPedida string
	// nuevoPedido: la acción «Nuevo contacto» del widget llegó antes que la lista; el
	// formulario se abre al llegar (PedirNuevo).
	nuevoPedido bool
	favoritos   map[string]bool
	// destacando son los UID con su SetFavorite en vuelo (ver destacar).
	destacando map[string]bool
	marcados   map[string]bool
	// Directorio es la agenda de Sytue ID, con la que se pone al día la libreta
	// «Organización» al entrar; nil, no se toca. Yo es el `sub` de quien mira.
	Directorio *api.Directorio
	Yo         func() string
	// IA es quien rellena la ficha con el asistente «Rellenar» (Sytue AI o la IA de
	// Sytue Office, `api.Asistente`); nil, no sale.
	IA api.Asistente

	// red es el monitor de conexión (Conexion); nil, siempre hay red. escrituras son
	// los flujos que escriben que están abiertos, uno por clase, como los estados
	// `modal`, `labelModal`… de la web: sin red se cierran.
	red               *offline.Monitor
	escrituras        map[string]*ui.Dialogo
	importadorCargado bool
	// cuentas son las de `cuentasLibreta`, de la libreta que identifican.
	cuentas struct {
		listas              bool
		primero             *api.Contact
		n                   int
		dia                 time.Time
		cumples, duplicados int
	}
}

// NuevaApp monta Contacts. `svc` nil deja la pantalla sin datos, que es lo que se ve
// antes de entrar.
func NuevaApp(t *i18n.Translator, svc *api.Contacts) *App {
	a := &App{
		Pantalla:   &Pantalla{T: t},
		svc:        svc,
		avisos:     ui.NuevaPilaAvisos(),
		Plazo:      30 * time.Second,
		favoritos:  map[string]bool{},
		destacando: map[string]bool{},
		marcados:   map[string]bool{},
		escrituras: map[string]*ui.Dialogo{},
		turnosFoto: make(chan struct{}, 2),
	}
	a.enFondo, a.enFondoFila = a.lanzar, a.lanzar
	a.enUIFoto = fyne.Do
	a.fotos, a.fotosPedidas = map[string]fotoGuardada{}, map[string]peticionFoto{}
	a.enganchar()
	return a
}

// Objeto es lo que se le da a la ventana: el armazón con los avisos encima.
func (a *App) Objeto() fyne.CanvasObject {
	if a.raiz == nil {
		a.raiz = container.New(&medidorVentana{a: a}, a.Pantalla.Construir(), a.avisos)
	}
	return a.raiz
}

// medidorVentana apila la pantalla y los avisos (`container.NewStack`) y, de paso, se
// entera del ancho: los cortes `sm:`, `md:` y `lg:` de la ficha y de la lista son de
// VENTANA, y Fyne no avisa de que la ventana cambia más que colocando lo de dentro.
//
// Solo rehace la columna cuyas medidas cambian con el ancho nuevo (`huellaAncho`): al
// arrastrar el borde de la ventana entre dos cortes no se rehace nada, como el `alMedir`
// de Drive. Antes repintaba la pantalla entera a cada píxel.
type medidorVentana struct{ a *App }

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
	if p := m.a.Pantalla; t.Width > 0 && t.Width != p.Ventana {
		antes := p.huellaAncho()
		p.Ventana = t.Width
		ahora := p.huellaAncho()
		var partes Partes
		// En ventana estrecha con un contacto abierto la lista no se ve (se turna con la
		// ficha): se rehace cuando vuelva a verse, no a cada píxel (listaPendiente).
		oculta := p.Ventana < ui.Estrecho && p.DetalleAbierto
		if ahora.lista != antes.lista || (p.listaPendiente && !oculta) {
			if oculta {
				p.listaPendiente = true
			} else {
				partes |= ParteLista
			}
		}
		if ahora.ficha != antes.ficha {
			// Si solo cambia el ancho de la tarjeta, la ficha se ajusta en su sitio.
			soloTarjeta := ahora.ficha
			soloTarjeta.tarjeta = antes.ficha.tarjeta
			if soloTarjeta != antes.ficha || !p.ajustarAnchoFicha() {
				partes |= ParteFicha
			}
		}
		if partes != 0 {
			m.a.repintarPartes(partes)
		}
	}
}

// repintar rehace la pantalla con el estado de ahora. SIEMPRE en el hilo de la
// interfaz: quien venga de la red pasa por `fyne.Do`.
func (a *App) repintar() { a.repintarPartes(Todo) }

// repintarPartes es `repintar` de solo esas columnas: el buscador y las casillas tocan
// la lista; elegir un contacto, la lista (la fila abierta) y la ficha. Lo demás sigue
// en pantalla tal cual. Si la pantalla cambia de forma, se rehace entera.
func (a *App) repintarPartes(partes Partes) {
	a.volcar()
	if a.raiz == nil {
		return
	}
	if a.Pantalla.Actualizar(partes) {
		return
	}
	viejo := a.raiz.Objects[0]
	a.raiz.Objects[0] = a.Pantalla.Construir()
	a.raiz.Refresh()
	a.Pantalla.soltar([]fyne.CanvasObject{viejo})
}

func (a *App) abridor() api.Abridor {
	if a.Abridor != nil {
		return a.Abridor
	}
	return fyne.CurrentApp()
}

// org es la organización activa de la sesión, o "" sin sesión y en el guardián.
func (a *App) org() string {
	if a.Org == nil {
		return ""
	}
	return a.Org()
}

// enganchar conecta los callbacks de la Pantalla con lo que hacen.
func (a *App) enganchar() {
	p := a.Pantalla
	p.AlElegirContacto = a.elegir
	// Una tecla en el buscador solo cambia la lista: ni el menú ni la ficha se rehacen.
	p.AlBuscar = func(s string) { p.Busqueda = s; a.repintarPartes(ParteLista) }
	p.AlElegirLibreta = func(b api.AddressBook) {
		scope := a.capturar()
		a.programarEn(scope, func(scope cargaContacts) { a.abrirLibretaEn(scope, b) })
	}
	p.AlElegirEtiqueta = func(id string) {
		// Pulsar la etiqueta activa quita el filtro.
		if p.EtiquetaActiva == id {
			id = ""
		}
		p.EtiquetaActiva = id
		a.repintar()
	}
	p.AlVerDestacados = func() { p.SoloDestacados = !p.SoloDestacados; a.repintar() }
	p.AlDestacar = a.destacar
	p.AlMarcar = func(id string) {
		if a.marcados[id] {
			delete(a.marcados, id)
		} else {
			a.marcados[id] = true
		}
		a.repintarPartes(ParteLista)
	}
	p.AlMarcarTodo = a.marcarTodo
	p.AlLimpiarFiltro = func() {
		p.Busqueda, p.EtiquetaActiva, p.SoloDestacados = "", "", false
		a.repintar()
	}
	p.AlReintentar = a.IniciarCarga
	p.Refresco.Actualizar = a.recargarActiva
	p.Refresco.Puede = func() bool {
		return a.svc != nil && p.Activa != nil && !a.cerrada && a.vida != nil && a.vida.Err() == nil && !p.SinConexion &&
			(p.Estado == ui.EstadoExito || p.Estado == ui.EstadoVacia) && len(p.Lote) == 0 && !p.DetalleAbierto
	}
	p.Refresco.Revision = func() uint64 { return a.cargaContactos }
	p.AlReintentarLibretas = a.IniciarCarga
	p.AlReintentarEtiquetas = func() { a.programar(a.cargarEtiquetasEn) }
	p.AlEnviarCorreo = a.enviarCorreo
	p.AlReunion = a.reunion
	p.AlCompartir = a.compartirPorCorreo
	p.AlCopiar = a.copiar
	p.AlExportar = func(c api.Contact) { a.programar(func(scope cargaContacts) { a.exportar(scope, c) }) }
	p.AlAbrirEnlace = a.abrirEnlace
	p.AlEtiquetar = a.etiquetar
	p.AlCrearContacto = func() { a.abrirFormulario(nil) }
	p.AlEditar = func(c api.Contact) { a.abrirFormulario(&c) }
	p.AlRepintar = a.repintar
	p.AlVolver = func() {
		p.Seleccionado, p.DetalleAbierto = nil, false
		a.repintarPartes(ParteLista | ParteFicha)
	}
	p.AlCombinar = func(c api.Contact) { a.abrirFusion(c, a.candidatosDe(c)) }
	p.AlRevisarDuplicados = func() { a.verDuplicados() }
	p.AlLote = a.lote
	p.AlFotoColor = func(c api.Contact, token string) {
		if uri, err := fotoPredefinida(c.DisplayName(), token); err == nil {
			a.ponerFoto(c, uri)
		}
	}
	p.AlQuitarFoto = a.quitarFoto
	p.FotoDe = a.fotoDeFila
	p.AlSubirFoto = func(c api.Contact) {
		scope := a.capturar()
		a.fotoDeUnFicheroEn(scope, func(uri string) { a.ponerFotoEn(scope, c, uri) })
	}
	p.AlImportarVcard = func() { a.elegirEImportar([]string{".vcf"}, a.importarVCard) }
	p.AlImportarCSV = func() { a.elegirEImportar([]string{".csv"}, a.importarCSV) }
	p.AlExportarVcard = a.exportarLibreta
	p.AlExportarCSV = a.exportarCSV
	p.AlSoltarMarcados = func() { clear(a.marcados); a.repintarPartes(ParteLista) }
	p.AlCorreoATodos = a.correoATodos
	p.AlCopiarCorreos = a.copiarCorreos
	p.AlCambiarOrden = a.cambiarOrden
	p.AlExportarSeleccion = func(sel []api.Contact) { a.exportarContactos(p.t("io.exportFilename", nil), sel) }
	p.AlCorreoEtiqueta = func() {
		_, cs := a.miembrosEtiquetaActiva()
		a.correoATodos(cs)
	}
	p.AlExportarEtiqueta = func() {
		g, cs := a.miembrosEtiquetaActiva()
		a.exportarContactos(NombreExportacion(g.Name), cs)
	}
	p.MiembrosEtiqueta = func() int {
		_, cs := a.miembrosEtiquetaActiva()
		return len(cs)
	}
	p.AlVerCumpleanos = func() { a.verCumples() }
	p.AlVerPapelera = func() { a.verPapelera() }
	a.engancharEtiquetasYLibretas()
}

// lienzo es donde se abren los diálogos, o nil si la app no está montada todavía.
func (a *App) lienzo() fyne.Canvas {
	if a.raiz == nil {
		return nil
	}
	return fyne.CurrentApp().Driver().CanvasForObject(a.raiz)
}

// Atajo atiende las letras sueltas de la web (`hojaDeAtajos.ts`): las de Google Contacts.
// Lo engancha la ventana con SetOnTypedRune, que en Fyne solo llega cuando no hay nada
// enfocado —el «fuera de un campo» de `esAtajoDeApp` sale solo— o cuando lo que tiene
// el foco lo deja subir, como la fila de la lista (`FilaContacto.TypedRune`). Con algo
// abierto encima no hace nada: el teclado es de la capa.
//
//	n  nuevo contacto       j/k  siguiente / anterior   /  al buscador
//	e  editar               s    destacar                x  marcar
//	?  la hoja de atajos      #    a la papelera
//
// e, s y x actúan sobre la fila enfocada; sin ella, sobre la ficha abierta.
func (a *App) Atajo(r rune) {
	c := a.lienzo()
	if c == nil || c.Overlays().Top() != nil {
		return
	}
	p := a.Pantalla
	escribe := p.Activa != nil && p.escribe()
	switch r {
	case 'n':
		if escribe {
			a.abrirFormulario(nil)
		}
	case '?':
		a.verAtajos(c)
	case '/':
		p.EnfocarBuscador()
	case 'j', 'k':
		a.moverCursor(r == 'j')
	case 'e', 's', 'x', '#':
		if o := a.objetivoDeAtajo(); o != nil {
			a.atajoSobre(r, *o, escribe)
		}
	}
}

// atajoSobre son las letras que actúan sobre un contacto (la fila enfocada o la ficha
// abierta): editar, destacar, marcar y, como en Gmail, «#» a la papelera con el
// «Deshacer» del aviso, por el mismo lote que la barra de selección.
func (a *App) atajoSobre(r rune, o api.Contact, escribe bool) {
	p := a.Pantalla
	switch {
	case r == 'e' && escribe:
		a.abrirFormulario(&o)
	case r == 's' && p.Activa != nil && !p.SinConexion:
		// Destacar es de este equipo: también en una libreta de solo lectura.
		a.destacar(o)
	case r == 'x' && p.AlMarcar != nil:
		p.AlMarcar(o.ID)
	case r == '#' && escribe:
		a.lote(LoteAPapelera, []api.Contact{o}, nil)
	}
}

// indiceAbierto es la posición de la ficha abierta en la lista filtrada, o -1.
func (a *App) indiceAbierto() int {
	p := a.Pantalla
	if p.Seleccionado == nil {
		return -1
	}
	return slices.IndexFunc(p.Contactos, func(c api.Contact) bool { return c.ID == p.Seleccionado.ID })
}

// objetivoDeAtajo es sobre quién actúan e, s y x: la fila con el foco; si no, la ficha
// abierta, aunque el filtro la haya sacado de la lista.
func (a *App) objetivoDeAtajo() *api.Contact {
	p := a.Pantalla
	if i := p.FilaEnfocada(); i >= 0 {
		c := p.Contactos[i]
		return &c
	}
	if p.Seleccionado == nil {
		return nil
	}
	c := *p.Seleccionado
	return &c
}

// moverCursor es j/k. Desde la fila enfocada o la ficha abierta, a la siguiente o la
// anterior; sin ninguna de las dos, la primera pulsación aterriza en el cursor en vez de
// saltárselo (como Gmail). Con una ficha abierta la ficha sigue al cursor: leer una
// libreta de corrido es pulsar j una y otra vez, sin Intro entre medias.
func (a *App) moverCursor(siguiente bool) {
	p := a.Pantalla
	total := len(p.Contactos)
	if total == 0 {
		return
	}
	base, desde := p.FilaEnfocada(), true
	if base < 0 {
		base = a.indiceAbierto()
	}
	if base < 0 {
		base, desde = p.CursorFila(), false
	}
	destino := base
	switch {
	case desde && siguiente:
		destino = min(base+1, total-1)
	case desde:
		destino = max(base-1, 0)
	}
	if p.Seleccionado != nil && p.Contactos[destino].ID != p.Seleccionado.ID {
		a.elegir(p.Contactos[destino])
	}
	p.IrAFila(destino)
}

// verAtajos abre la hoja de «?» (`ui.MostrarHojaAtajos`), con los dos grupos y el orden
// de la web.
func (a *App) verAtajos(c fyne.Canvas) {
	p := a.Pantalla
	fila := func(clave string, teclas ...string) ui.AtajoDeTeclado {
		return ui.AtajoDeTeclado{Teclas: teclas, Descripcion: p.t(clave, nil)}
	}
	ui.MostrarHojaAtajos(c, p.t("shortcuts.title", nil), p.t("common.close", nil), []ui.GrupoDeAtajos{
		{Titulo: p.t("shortcuts.group.move", nil), Atajos: []ui.AtajoDeTeclado{
			fila("shortcuts.next", "j", "↓"), fila("shortcuts.prev", "k", "↑"),
			fila("shortcuts.first", p.t("shortcuts.keyHome", nil)), fila("shortcuts.last", p.t("shortcuts.keyEnd", nil)),
			fila("shortcuts.open", "↵"), fila("list.search", "/"),
		}},
		{Titulo: p.t("shortcuts.group.actions", nil), Atajos: []ui.AtajoDeTeclado{
			fila("nav.newContact", "n"), fila("shortcuts.edit", "e"), fila("shortcuts.star", "s"),
			fila("shortcuts.select", "x"), fila("batch.toTrash", "#"), fila("shortcuts.help", "?"),
		}},
	})
}

// abrirFormulario abre `ContactModal` para crear (nil) o editar un contacto, y lo
// devuelve (nil si no hay dónde abrirlo).
func (a *App) abrirFormulario(c *api.Contact) *formulario {
	lienzo, p := a.lienzo(), a.Pantalla
	if lienzo == nil || a.svc == nil || p.Activa == nil {
		return nil
	}
	scope := a.capturar()
	libreta := *p.Activa
	f := p.nuevoFormulario(c)
	if c != nil {
		f.fotoActual = a.fotoDe(*c)
	}
	if a.ElegirFichero != nil {
		f.subirFoto = func(listo func(string)) { a.fotoDeUnFicheroEn(scope, listo) }
	}
	if a.IA != nil {
		idioma := string(p.locale())
		f.alAsistir = func(texto string, listo func(api.FichaPropuesta, bool)) {
			a.programarEn(scope, func(scope cargaContacts) { a.pedirFicha(scope, texto, idioma, listo) })
		}
	}
	f.alGuardar = func(in api.ContactInput, listo func(error)) {
		fotoNueva, quitarFoto := f.fotoNueva, f.fotoQuitada && f.fotoActual != nil
		a.programarEn(scope, func(scope cargaContacts) { a.guardarContacto(scope, libreta, c, in, fotoNueva, quitarFoto, listo) })
	}
	if c != nil {
		f.alBorrar = func(listo func(error)) {
			a.programarEn(scope, func(scope cargaContacts) { a.borrarContacto(scope, libreta, *c, listo) })
		}
	}
	a.mostrarEscritura("contacto", f.dialogo(lienzo), lienzo)
	return f
}

// plazoIA es el del modo calidad del asistente: el servicio le da 120 s.
const plazoIA = 130 * time.Second

// pedirFicha es el `fetch('/api/ai', {action: 'ficha', kind: 'contact', modo:
// 'calidad'})` de AsistenteFicha, sin BFF: con el token de la persona, por el ingress que
// Sytue AI abre a las apps nativas. Sale lo pegado y nada más, igual que en la web, y el
// servicio lo manda a su modelo en Scaleway (UE) sin anonimizar. Bloquea: en segundo
// plano; `listo` llega en el hilo de la interfaz.
func (a *App) pedirFicha(scope cargaContacts, texto, idioma string, listo func(api.FichaPropuesta, bool)) {
	if !a.vigente(scope) {
		return
	}
	ctx, fin := context.WithTimeout(scope.ctx, plazoIA)
	defer fin()
	bruto, err := a.IA.Asistir(ctx, api.PeticionIA{
		Accion: "ficha", Tipo: "contact", Instruccion: api.InstruccionFicha, Contexto: texto,
		Modo: "calidad", Idioma: idioma,
	})
	var ficha api.FichaPropuesta
	ok := false
	if err != nil {
		slog.Warn("contactos: el asistente no ha contestado", "error", err)
	} else {
		ficha, ok = api.LeerFicha(bruto)
	}
	fyne.Do(func() {
		if a.vigente(scope) {
			listo(ficha, ok)
		}
	})
}

// ConImportar engancha «Trae tus contactos» (`ImportData domains={['contacts']}`, la
// pieza común `ui.ImportarDatos`) al pie del menú. Al acabar una importación la libreta
// se recarga sola, que es el `onFinished={reload}` de la web.
func (a *App) ConImportar(i *api.Integraciones) {
	d := ui.NuevoImportarDatos(a.Pantalla.T, i, "contacts")
	d.AlCambiar = a.repintar
	d.AlTerminar = a.recargarActiva
	d.AlPolitica = func() {
		_ = api.AbrirFuera(a.abridor(), ui.PoliticaDePrivacidad(a.Apex, a.Pantalla.locale()))
	}
	a.Pantalla.Importar = d
}

// ConServiciosVinculados muestra las libretas del proveedor sin importarlas.
func (a *App) ConServiciosVinculados(abrir func()) {
	a.Pantalla.AlServiciosVinculados = abrir
	a.repintar()
}

// mostrarEscritura abre un diálogo que escribe y lo apunta para cerrarlo si se va la red.
func (a *App) mostrarEscritura(clase string, d *ui.Dialogo, lienzo fyne.Canvas) {
	d.Mostrar(lienzo)
	a.escrituras[clase] = d
}

// Conexion engancha el monitor de la caché «red primero» (`sesion.Suite.Conexion`):
// es el `useConnectivity` de la web.
func (a *App) Conexion(red *offline.Monitor) {
	a.red = red
	red.Subscribe(func(e offline.State) {
		fyne.Do(func() {
			if a.abierta() {
				a.conexion(e != offline.Offline)
			}
		})
	})
}

// sinRed dice si el monitor ve la red caída.
func (a *App) sinRed() bool { return a.red != nil && a.red.State() == offline.Offline }

// conexion aplica un cambio de la red. Sin ella, Contacts se queda en SOLO LECTURA —lo
// ya cargado, o lo que la caché tenga, se sigue viendo— y, como el efecto de
// ContactsApp.tsx, se cierran los flujos que escriben y se suelta la selección. Al
// volver, lo que no pudo cargarse se pide otra vez: su panel no tiene «Reintentar».
func (a *App) conexion(hay bool) {
	p := a.Pantalla
	if p.SinConexion == !hay {
		return
	}
	p.SinConexion = !hay
	if !hay {
		for _, d := range a.escrituras {
			d.Cerrar()
		}
		clear(a.escrituras)
		clear(a.marcados)
	}
	a.repintar()
	if hay && p.Estado == ui.EstadoSinConexion {
		a.IniciarCarga()
	}
}

// estadoDeFallo es el estado de una carga que no ha llegado: sin red, el suyo.
func (a *App) estadoDeFallo() Estado {
	if a.sinRed() {
		return ui.EstadoSinConexion
	}
	return ui.EstadoError
}

// guardarContacto crea (c nil) o actualiza el contacto y, después, su foto. Bloquea:
// en segundo plano.
func (a *App) guardarContacto(scope cargaContacts, libreta api.AddressBook, c *api.Contact, in api.ContactInput,
	fotoNueva string, quitarFoto bool, listo func(error),
) {
	if !a.vigente(scope) {
		return
	}
	ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
	var err error
	var guardado api.Contact
	if c == nil {
		guardado, err = a.svc.Create(ctx, libreta.ID, in)
	} else {
		guardado, err = a.svc.Update(ctx, c.ID, in, c.ETag)
	}
	fin()
	// La foto va aparte, por su propio punto de entrada y por UID, que al crear no existe
	// hasta ahora. Si falla, la ficha ya está guardada: se avisa aparte.
	var errFoto error
	if err == nil && guardado.UID != "" && (fotoNueva != "" || quitarFoto) {
		ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
		if fotoNueva != "" {
			errFoto = a.svc.SetPhoto(ctx, guardado.UID, fotoNueva)
		} else {
			errFoto = a.svc.RemovePhoto(ctx, guardado.UID)
		}
		fin()
	}
	fyne.Do(func() {
		if !a.vigente(scope) {
			return
		}
		if errFoto != nil {
			slog.Warn("contactos: no se pudo guardar la foto", "error", errFoto)
			a.avisar(ui.AvisoPeligro, "photo.updateFailed", "")
		}
		switch {
		case err != nil:
			slog.Warn("contactos: no se pudo guardar", "error", err)
			a.avisar(ui.AvisoPeligro, "form.saveFailed", "common.tryAgainSoon")
		case c == nil:
			a.avisar(ui.AvisoExito, "form.created", "")
		default:
			a.avisar(ui.AvisoExito, "form.updated", "")
		}
		listo(err)
		if err == nil {
			a.olvidarFoto(guardado.ID)
			a.recargarEn(scope, libreta)
		}
	})
}

// borrarContacto lo manda a la papelera. Bloquea: en segundo plano.
func (a *App) borrarContacto(scope cargaContacts, libreta api.AddressBook, c api.Contact, listo func(error)) {
	if !a.vigente(scope) {
		return
	}
	ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
	err := a.svc.Delete(ctx, c.ID)
	fin()
	fyne.Do(func() {
		if !a.vigente(scope) {
			return
		}
		if err != nil {
			slog.Warn("contactos: no se pudo eliminar", "error", err)
			a.avisar(ui.AvisoPeligro, "form.deleteFailed", "common.tryAgainSoon")
		} else {
			// El borrado es SUAVE: decir solo «eliminado» hacía que la gente diera por
			// perdido lo que está a un clic en la papelera. Y lleva «Deshacer».
			a.avisarPapelera(a.Pantalla.t("form.deleted", nil), a.Pantalla.t("form.deletedHint", nil),
				[]string{api.CleanUID(c.UID)}, 1)
		}
		listo(err)
		if err == nil {
			a.recargarEn(scope, libreta)
		}
	})
}

// recargar es el `reload` de la web: suelta la selección, enseña el esqueleto y trae
// la libreta otra vez. La ficha sigue al dato fresco (ver cargarContactos).
func (a *App) recargar(b api.AddressBook) { a.recargarEn(a.capturar(), b) }

func (a *App) recargarEn(scope cargaContacts, b api.AddressBook) {
	if !a.vigente(scope) {
		return
	}
	clear(a.marcados)
	a.Pantalla.Estado = ui.EstadoCargando
	a.repintar()
	a.programarEn(scope, func(scope cargaContacts) { a.cargarContactosEn(scope, b) })
}

// Cargar trae las libretas, abre la primera y, a la vez, los destacados, que son de
// la persona y no de la libreta. Bloquea: se llama en segundo plano.
func (a *App) Cargar() { a.cargarEn(a.capturar()) }

func (a *App) cargarEn(scope cargaContacts) {
	if a.svc == nil {
		return
	}
	if !a.vigente(scope) {
		return
	}
	var sec uint64
	fyne.DoAndWait(func() {
		if !a.vigente(scope) {
			return
		}
		a.cargaLibretas++
		sec = a.cargaLibretas
		a.Pantalla.CargandoLibretas, a.Pantalla.ErrorLibretas = true, false
		// A refresh of the same account keeps its current contact and form.
		if a.Pantalla.Activa == nil {
			a.Pantalla.Estado = ui.EstadoCargando
		}
		if d := a.Pantalla.Importar; d != nil {
			if a.importadorCargado {
				d.Cargar()
			} else {
				a.importadorCargado = true
				d.CargarCon(scope.ctx)
			}
		}
		a.repintar()
	})
	ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
	libretas, err := a.libretasRefrescadas(ctx, sec > 1)
	fin()
	var abrir, refrescar *api.AddressBook
	adoptada := false
	fyne.DoAndWait(func() {
		if !a.vigente(scope) || sec != a.cargaLibretas {
			return
		}
		adoptada = true
		abrir = a.recibirLibretas(scope, sec, libretas, err)
		if err == nil && abrir == nil && a.Pantalla.Activa != nil {
			b := *a.Pantalla.Activa
			refrescar = &b
		}
	})
	if !adoptada || !a.vigente(scope) || err != nil {
		return
	}
	a.programarEn(scope, func(scope cargaContacts) { a.cargarFavoritosEn(scope) })
	if abrir != nil {
		a.abrirLibretaEn(scope, *abrir)
	} else if refrescar != nil {
		a.programarEn(scope, func(scope cargaContacts) { a.cargarEtiquetasEn(scope) })
		a.cargarContactosEn(scope, *refrescar)
	}
	// Personal contacts are useful before the optional organization import starts.
	a.programarEn(scope, func(scope cargaContacts) { a.sincronizarOrganizacionEn(scope) })
}

func (a *App) libretasRefrescadas(ctx context.Context, refresh bool) ([]api.AddressBook, error) {
	// Initial mount uses the repository already authenticated by PrivateApplication.
	if refresh {
		if err := a.svc.RefreshPrivateData(ctx); err != nil {
			return nil, err
		}
	}
	return a.svc.AddressBooks(ctx)
}

// recibirLibretas runs on the UI and preserves a selection on same-identity refresh.
func (a *App) recibirLibretas(scope cargaContacts, sec uint64, libretas []api.AddressBook, err error) *api.AddressBook {
	if !a.vigente(scope) || sec != a.cargaLibretas {
		return nil
	}
	p := a.Pantalla
	p.CargandoLibretas, p.ErrorLibretas = false, err != nil
	defer a.repintar()
	if err != nil {
		if p.Activa == nil {
			p.Estado = a.estadoDeFallo()
		}
		return nil
	}
	p.Libretas = libretas
	requested := a.libretaPedida
	a.libretaPedida = ""
	for i := range libretas {
		if requested != "" && libretas[i].ID == requested {
			return &libretas[i]
		}
	}
	if p.Activa != nil {
		for i := range libretas {
			if libretas[i].ID == p.Activa.ID {
				p.Activa = &libretas[i]
				return nil
			}
		}
	}
	if p.Activa != nil {
		p.Activa, p.Seleccionado = nil, nil
		a.todos = nil
	}
	first := libretaInicial(libretas)
	if first == nil {
		p.Estado = ui.EstadoExito
	}
	return first
}

func (a *App) sincronizarOrganizacionEn(scope cargaContacts) {
	if a.Directorio == nil || a.sinRed() || !a.vigente(scope) {
		return
	}
	a.vidaMu.Lock()
	if a.organizacionEnCurso || scope.generacion != a.generacion {
		a.vidaMu.Unlock()
		return
	}
	a.organizacionEnCurso = true
	a.vidaMu.Unlock()
	defer func() {
		a.vidaMu.Lock()
		if scope.generacion == a.generacion {
			a.organizacionEnCurso = false
		}
		a.vidaMu.Unlock()
	}()
	yo := ""
	if a.Yo != nil {
		yo = a.Yo()
	}
	ctx, fin := context.WithTimeout(scope.ctx, 4*a.Plazo)
	defer fin()
	if _, err := sincronizarOrganizacion(ctx, a.svc, a.Directorio, yo, string(a.Pantalla.locale())); err != nil {
		if ctx.Err() == nil {
			slog.Info("contactos: la libreta Organización no se ha puesto al día", "error", err)
		}
		return
	}
	libretas, err := a.svc.AddressBooks(ctx)
	if err != nil {
		return
	}
	var refrescar *api.AddressBook
	fyne.DoAndWait(func() {
		if !a.vigente(scope) {
			return
		}
		p := a.Pantalla
		p.Libretas = libretas
		// A late organization result never navigates away from the user's selection.
		if p.Activa != nil && p.Activa.Name == libretaOrganizacion {
			b := *p.Activa
			refrescar = &b
		}
		a.repintar()
	})
	if refrescar != nil && a.vigente(scope) {
		a.cargarContactosEn(scope, *refrescar)
	}
}

// Vaciar suelta todo lo cargado: al cerrar sesión, lo de otra persona no puede
// quedarse en pantalla hasta la siguiente carga.
func (a *App) Vaciar() {
	a.invalidar(false)
	a.pararImportador()
	for key, d := range a.escrituras {
		d.Cerrar()
		delete(a.escrituras, key)
	}
	a.pedido, a.libretaPedida, a.nuevoPedido = "", "", false
	p := a.Pantalla
	a.todos = nil
	clear(a.favoritos)
	clear(a.destacando)
	clear(a.marcados)
	clear(a.fotos)
	clear(a.fotosPedidas)
	p.Libretas, p.Activa, p.Etiquetas, p.Seleccionado = nil, nil, nil, nil
	p.Busqueda, p.EtiquetaActiva, p.SoloDestacados = "", "", false
	p.ImportOcupado, p.Importando, p.LoteOcupado, p.EtiquetaOcupada = false, "", false, ""
	p.CargandoLibretas, p.ErrorLibretas, p.CargandoEtiquetas, p.ErrorEtiquetas = false, false, false, false
	p.DetalleAbierto = false
	p.Estado = ui.EstadoCargando
	a.repintar()
}

// libretaInicial es la que se abre al entrar: la predeterminada, o la primera. La web
// abre `addressBooks[0]`, que el servicio ya devuelve con la predeterminada delante.
func libretaInicial(libretas []api.AddressBook) *api.AddressBook {
	for i := range libretas {
		if libretas[i].IsDefault {
			return &libretas[i]
		}
	}
	if len(libretas) > 0 {
		return &libretas[0]
	}
	return nil
}

// abrirLibreta la activa y trae sus contactos y sus etiquetas. Cambiar de libreta
// suelta la ficha, el filtro de etiqueta (las etiquetas son de cada libreta) y la
// selección.
func (a *App) abrirLibretaEn(scope cargaContacts, b api.AddressBook) {
	fyne.DoAndWait(func() {
		if !a.vigente(scope) {
			return
		}
		p := a.Pantalla
		p.Activa, p.Seleccionado, p.EtiquetaActiva = &b, nil, ""
		p.DetalleAbierto = false
		clear(a.marcados)
		p.Estado = ui.EstadoCargando
		a.repintar()
	})
	if !a.vigente(scope) {
		return
	}
	a.programarEn(scope, func(scope cargaContacts) { a.cargarEtiquetasEn(scope) })
	a.cargarContactosEn(scope, b)
}

func (a *App) cargarContactosEn(scope cargaContacts, b api.AddressBook) {
	if !a.vigente(scope) {
		return
	}
	var sec uint64
	fyne.DoAndWait(func() {
		if a.vigente(scope) {
			a.cargaContactos++
			sec = a.cargaContactos
		}
	})
	ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
	lista, err := a.svc.InBook(ctx, b.ID)
	fin()
	fyne.Do(func() {
		p := a.Pantalla
		if !a.vigente(scope) || sec != a.cargaContactos || p.Activa == nil || p.Activa.ID != b.ID {
			return
		}
		if err != nil {
			slog.Warn("contactos: no se pudo listar la libreta", "error", err)
			a.todos, p.Estado = nil, a.estadoDeFallo()
			a.repintar()
			return
		}
		ordenar(lista, p.locale(), p.PorApellido)
		a.todos, p.Estado = lista, ui.EstadoExito
		if p.Seleccionado != nil {
			p.Seleccionado = buscarPorID(lista, p.Seleccionado.ID)
		}
		a.repintar()
		a.abrirPedido()
		if a.nuevoPedido {
			a.nuevoPedido = false
			a.abrirFormulario(nil)
		}
	})
}

func (a *App) cargarEtiquetasEn(scope cargaContacts) {
	var libreta string
	// DoAndWait y no Do: lo que se lee aquí hace falta para la petición de abajo.
	fyne.DoAndWait(func() {
		if a.vigente(scope) && a.Pantalla.Activa != nil {
			libreta = a.Pantalla.Activa.ID
			a.Pantalla.CargandoEtiquetas, a.Pantalla.ErrorEtiquetas = true, false
			a.repintar()
		}
	})
	if libreta == "" {
		return
	}
	ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
	grupos, err := a.svc.Groups(ctx, libreta)
	fin()
	fyne.Do(func() {
		p := a.Pantalla
		if !a.vigente(scope) || p.Activa == nil || p.Activa.ID != libreta {
			return
		}
		p.CargandoEtiquetas, p.ErrorEtiquetas, p.Etiquetas = false, err != nil, grupos
		if err != nil {
			slog.Warn("contactos: no se pudieron listar las etiquetas", "error", err)
			p.Etiquetas = nil
		}
		a.repintar()
	})
}

// cargarFavoritos trae los destacados. Si falla, se sigue sin ellos: la web tampoco
// dice nada, porque la agenda sin estrellas sigue sirviendo.
func (a *App) cargarFavoritosEn(scope cargaContacts) {
	if !a.vigente(scope) {
		return
	}
	ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
	uids, err := a.svc.Favorites(ctx)
	fin()
	fyne.Do(func() {
		if !a.vigente(scope) {
			return
		}
		clear(a.favoritos)
		if err == nil {
			for _, u := range uids {
				a.favoritos[api.CleanUID(u)] = true
			}
		}
		a.repintar()
	})
}

// volcar deriva la Pantalla del estado: lo filtrado, el total y los destacados.
func (a *App) volcar() {
	p := a.Pantalla
	p.Total = len(a.todos)
	p.Destacados = a.favoritos
	p.Marcados = a.marcados
	p.Lote = nil // nuevo: los botones de la barra se quedan con el de antes
	for _, c := range a.todos {
		if a.marcados[c.ID] {
			p.Lote = append(p.Lote, c)
		}
	}
	var miembros []string
	if p.EtiquetaActiva != "" {
		for _, g := range p.Etiquetas {
			if g.ID == p.EtiquetaActiva {
				miembros = g.Members
			}
		}
	}
	p.Contactos = filtrar(a.todos, p.Busqueda, miembros, p.EtiquetaActiva != "", p.SoloDestacados, a.favoritos)
	// Sin filtro, la lista va por cabeceras A–Z y lo que no empieza por letra, al final:
	// el cursor y j/k recorren lo que se ve, en el orden en que se ve.
	if p.ConCabeceras() {
		p.Contactos = porInicial(p.Contactos, p.locale(), p.PorApellido)
	}
	// Los cumpleaños y los duplicados son de la libreta ENTERA y no del filtro: se
	// cuentan una vez por libreta cargada (y por día, los cumpleaños), no con cada tecla.
	p.CumpleanosPronto, p.Duplicados = a.cuentasLibreta()
	p.PuedeImportar = a.ElegirFichero != nil
	p.Foto = nil
	if p.Seleccionado != nil {
		p.Foto = a.fotoDe(*p.Seleccionado)
	}
}

// cuentasLibreta son los cumpleaños de los próximos 30 días y los grupos de duplicados
// de la libreta cargada. La libreta se SUSTITUYE al cargar (nunca se cambia en su
// sitio), así que su primer elemento y su largo la identifican.
func (a *App) cuentasLibreta() (cumples, duplicados int) {
	hoy := time.Now()
	dia := time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 0, 0, 0, 0, hoy.Location())
	var primero *api.Contact
	if len(a.todos) > 0 {
		primero = &a.todos[0]
	}
	m := &a.cuentas
	if !m.listas || m.primero != primero || m.n != len(a.todos) {
		m.listas, m.primero, m.n = true, primero, len(a.todos)
		m.duplicados = len(GruposDuplicados(a.todos))
		m.dia = time.Time{}
	}
	if !m.dia.Equal(dia) {
		m.dia, m.cumples = dia, cumpleanosEn(a.todos, hoy, 30)
	}
	return m.cumples, m.duplicados
}

// filtrar es el `filtered` de la web: destacados, etiqueta y búsqueda a la vez.
//
// La búsqueda mira nombre, correos y organización, y el teléfono por DÍGITOS: nadie
// teclea «+34 600 00 00 00» tal y como está guardado, y en una libreta importada
// conviven los dos formatos. Desde tres dígitos: con menos, «1» devolvería media
// libreta.
func filtrar(todos []api.Contact, consulta string, miembros []string, porEtiqueta, soloDestacados bool, favoritos map[string]bool) []api.Contact {
	q := buscar.Plegar(strings.TrimSpace(consulta))
	digitos := soloDigitos(q)
	porTelefono := len(digitos) >= 3
	if q == "" && !porEtiqueta && !soloDestacados {
		return todos
	}
	var fuera []api.Contact
	for _, c := range todos {
		if soloDestacados && !favoritos[api.CleanUID(c.UID)] {
			continue
		}
		if porEtiqueta && !slices.Contains(miembros, c.UID) {
			continue
		}
		if q != "" && !coincide(c, q, digitos, porTelefono) {
			continue
		}
		fuera = append(fuera, c)
	}
	return fuera
}

func coincide(c api.Contact, q, digitos string, porTelefono bool) bool {
	// Además del nombre y la organización, lo que la web saca de la vCard con
	// `textoBuscable`: apodo, cargo, nota y la ciudad de sus direcciones («el de
	// Zaragoza», «la abogada»).
	for _, s := range []string{c.DisplayName(), c.Organization, c.Nickname, c.Title, c.Note} {
		if strings.Contains(buscar.Plegar(s), q) {
			return true
		}
	}
	for _, d := range c.Addresses {
		if strings.Contains(buscar.Plegar(d.Locality), q) {
			return true
		}
	}
	for _, e := range c.Emails {
		if strings.Contains(buscar.Plegar(e.Email), q) {
			return true
		}
	}
	if porTelefono {
		for _, t := range c.Phones {
			if strings.Contains(soloDigitos(t.Phone), digitos) {
				return true
			}
		}
	}
	return false
}

func soloDigitos(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// ordenar es el `localeCompare(…, intlTag(locale))` de la web.
func buscarPorID(lista []api.Contact, id string) *api.Contact {
	for i := range lista {
		if lista[i].ID == id {
			return &lista[i]
		}
	}
	return nil
}

// elegir abre la ficha de un contacto. En ventana estrecha, además, la trae delante.
func (a *App) elegir(c api.Contact) {
	a.Pantalla.Seleccionado = &c
	// El cursor sigue a la ficha abierta (clic, cumpleaños, enlace): el tabulador entra
	// en la lista por el contacto que se está viendo, no por el primero.
	if i := a.indiceAbierto(); i >= 0 {
		a.Pantalla.Cursor = i
	}
	a.Pantalla.DetalleAbierto = true
	// La ficha y la fila abierta de la lista; el menú no depende del elegido.
	a.repintarPartes(ParteLista | ParteFicha)
	a.traerFoto(c)
}

// marcarTodo marca todo lo FILTRADO, o lo desmarca si ya lo estaba entero.
func (a *App) marcarTodo() {
	todos, _ := a.Pantalla.estadoMarcado()
	for _, c := range a.Pantalla.Contactos {
		if todos {
			delete(a.marcados, c.ID)
		} else {
			a.marcados[c.ID] = true
		}
	}
	a.repintarPartes(ParteLista)
}

// destacar pone o quita la estrella. OPTIMISTA, como la web: la estrella cambia al
// pulsar y, si el servicio dice que no, vuelve atrás y se avisa.
func (a *App) destacar(c api.Contact) {
	scope := a.capturar()
	uid := api.CleanUID(c.UID)
	// Mientras va la petición de ESE contacto, otro toque no hace nada: es el `busy` de
	// FavoriteToggle. Sin esto, un doble toque en la estrella de la fila lanzaba
	// SetFavorite(true) y SetFavorite(false) a la vez, que podían llegar desordenados y
	// dejar la pantalla y el servidor en desacuerdo hasta recargar.
	if a.destacando[uid] {
		return
	}
	a.destacando[uid] = true
	nuevo := !a.favoritos[uid]
	a.ponerFavorito(uid, nuevo)
	a.repintar()
	a.programarEn(scope, func(scope cargaContacts) {
		ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
		err := a.svc.SetFavorite(ctx, uid, nuevo)
		fin()
		if err != nil {
			slog.Warn("contactos: no se pudo destacar", "error", err)
		}
		fyne.Do(func() {
			if !a.vigente(scope) {
				return
			}
			delete(a.destacando, uid)
			if err == nil {
				return
			}
			a.ponerFavorito(uid, !nuevo)
			a.avisar(ui.AvisoPeligro, "detail.starFailed", "common.tryAgainSoon")
			a.repintar()
		})
	})
}

func (a *App) ponerFavorito(uid string, si bool) {
	if si {
		a.favoritos[uid] = true
	} else {
		delete(a.favoritos, uid)
	}
}

// avisar saca un aviso con dos claves del catálogo.
func (a *App) avisar(tono ui.TonoAviso, titulo, descripcion string) {
	d := ""
	if descripcion != "" {
		d = a.Pantalla.t(descripcion, nil)
	}
	a.avisos.Notificar(ui.NuevoAviso(tono, a.Pantalla.t(titulo, nil), d))
}

// enviarCorreo abre Sytue Mail con el destinatario puesto: el `?to=` que la web manda
// a `appUrl('mail')`. Es la app de Mail la que redacta; esta solo la invoca.
func (a *App) enviarCorreo(correo string) {
	if err := api.AbrirEn(a.abridor(), a.Apex, "mail", a.org(), "/?to="+url.QueryEscape(correo)); err != nil {
		slog.Warn("contactos: no se pudo abrir Sytue Mail", "error", err)
	}
}

// reunion abre Sytue Calendar con esta persona ya invitada y el asunto puesto: el
// `?action=create&attendees=&title=` que Calendar ya sabe leer, el mismo que usa el Hub.
func (a *App) reunion(c api.Contact, correo string) {
	q := url.Values{
		"action":    {"create"},
		"attendees": {correo},
		"title":     {a.Pantalla.t("detail.meetingTitle", i18n.Vars{"name": c.DisplayName()})},
	}
	if err := api.AbrirEn(a.abridor(), a.Apex, "calendar", a.org(), "/?"+q.Encode()); err != nil {
		slog.Warn("contactos: no se pudo abrir Sytue Calendar", "error", err)
	}
}

// copiar lo deja en el portapapeles del sistema y lo dice; si no ha llegado
// (`ui.Copiar` lo lee de vuelta), dice que no se ha podido, como el rechazo de
// `navigator.clipboard.writeText` en la web.
func (a *App) copiar(valor, aviso string) {
	if !ui.Copiar(fyne.CurrentApp().Clipboard(), valor) {
		a.avisar(ui.AvisoPeligro, "detail.copyFailed", "")
		return
	}
	a.avisar(ui.AvisoExito, aviso, "")
}

// abrirEnlace abre un `mailto:` o un `tel:` con lo que el sistema tenga para eso, que
// es lo que hace el navegador con el mismo enlace.
//
// El esquema lo pone la ficha y el valor es del contacto: se compone con sus piezas
// (`api.AbrirCorreo`, `api.AbrirTelefono`), no se le pasa al sistema la cadena tal cual.
func (a *App) abrirEnlace(destino string) {
	err := api.ErrNoSeAbre
	if correo, ok := strings.CutPrefix(destino, "mailto:"); ok {
		err = api.AbrirCorreo(a.abridor(), correo, "", "")
	} else if numero, ok := strings.CutPrefix(destino, "tel:"); ok {
		err = api.AbrirTelefono(a.abridor(), numero)
	}
	if err != nil {
		slog.Warn("contactos: no se pudo abrir el enlace", "destino", destino, "error", err)
	}
}

// exportar deja la vCard del contacto en la carpeta de descargas, con el nombre de
// fichero que pone la web (`exportFilename`).
//
// ponytail: no avisa al terminar. La web tampoco —la descarga la enseña el navegador— y
// aquí no hay clave de catálogo para «guardado en Descargas». Si hiciera falta, la
// clave se escribe en la web primero.
func (a *App) exportar(scope cargaContacts, c api.Contact) {
	a.guardarVCards(scope, NombreExportacion(c.DisplayName()), []api.Contact{c})
}

// guardarVCards deja en Descargas, con ese nombre, las vCards de esos contactos una
// detrás de otra: un `.vcf` de varias tarjetas es eso (RFC 6350), y es `downloadVcf` de
// la web. Son las del SERVICIO y no unas armadas aquí, así que un campo que esta app
// no pinta —la foto, un X-* de otro cliente— viaja igual. Bloquea: en segundo plano.
//
// Van en UNA petición por tanda (`/contacts/batch/vcard`): la web ya tiene las tarjetas
// porque lista por CardDAV, y la API REST no las da en el listado.
func (a *App) guardarVCards(scope cargaContacts, nombre string, cs []api.Contact) {
	if !a.vigente(scope) {
		return
	}
	// Una petición por tanda de TandaVCard, no una por contacto: una etiqueta de
	// trescientos eran trescientos GET seguidos, y uno que fallara cortaba los demás.
	ids := make([]string, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	var todo bytes.Buffer
	var err error
	for tanda := range slices.Chunk(ids, api.TandaVCard) {
		ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
		var tarjetas []byte
		tarjetas, err = a.svc.BatchVCard(ctx, tanda)
		fin()
		if err != nil {
			break
		}
		todo.Write(tarjetas)
	}
	if err == nil && todo.Len() == 0 && len(cs) > 0 {
		// Ninguna sigue viva (borradas desde otro dispositivo entre la selección y la
		// exportación): un .vcf vacío guardado en silencio parecería una exportación hecha.
		err = errExportacionVacia
	}
	if err == nil && a.vigente(scope) {
		err = a.guardarDescarga(nombre, todo.Bytes())
	}
	if err != nil {
		slog.Warn("contactos: no se pudo exportar", "error", err)
		fyne.Do(func() {
			if a.vigente(scope) {
				a.avisar(ui.AvisoPeligro, "common.actionFailed", "common.tryAgainSoon")
			}
		})
	}
}

// errExportacionVacia: se pidió exportar contactos y el servicio no devolvió ninguno.
var errExportacionVacia = errors.New("contactos: ninguno de los exportados sigue vivo")

// guardarDescarga es lo que el navegador hace con una descarga: dejarla en la carpeta
// de descargas del sistema.
func (a *App) guardarDescarga(nombre string, contenido []byte) error {
	if a.Descargas != nil {
		return a.Descargas(nombre, contenido)
	}
	carpeta, err := ficheros.Carpeta()
	if err == nil {
		_, err = ficheros.Guardar(carpeta, nombre, bytes.NewReader(contenido))
	}
	return err
}

// exportarLibreta es «Exportar vCard (.vcf)»: la libreta que se ve, entera.
func (a *App) exportarLibreta() {
	todos, nombre := slices.Clone(a.todos), a.Pantalla.t("io.exportFilename", nil)
	if !strings.HasSuffix(nombre, ".vcf") {
		nombre += ".vcf"
	}
	a.programar(func(scope cargaContacts) { a.guardarVCards(scope, nombre, todos) })
}

// exportarCSV es «Exportar CSV»: TODOS los contactos de la persona, de todas sus
// libretas, con el nombre de fichero que pone el servicio.
func (a *App) exportarCSV() {
	scope := a.capturar()
	a.programarEn(scope, func(scope cargaContacts) {
		ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
		nombre, csv, err := a.svc.ExportCSV(ctx)
		fin()
		if err == nil && a.vigente(scope) {
			if nombre == "" {
				nombre = strings.TrimSuffix(a.Pantalla.t("io.exportFilename", nil), ".vcf") + ".csv"
			}
			err = a.guardarDescarga(nombre, csv)
		}
		if err != nil {
			slog.Warn("contactos: no se pudo exportar el CSV", "error", err)
			fyne.Do(func() {
				if a.vigente(scope) {
					a.avisar(ui.AvisoPeligro, "common.actionFailed", "common.tryAgainSoon")
				}
			})
		}
	})
}

// NombreExportacion es `exportFilename` de la web: letras y cifras de cualquier
// alfabeto, lo demás a guiones, sin guiones en los extremos, hasta 60 y `.vcf`.
func NombreExportacion(nombre string) string {
	var b strings.Builder
	guion := false
	for _, r := range strings.TrimSpace(nombre) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			if guion && b.Len() > 0 {
				b.WriteByte('-')
			}
			guion = false
			b.WriteRune(r)
			continue
		}
		guion = true
	}
	// Se corta DESPUÉS de quitar los guiones de los extremos, como la web: un corte
	// que cae en un espacio deja el guion final, y así sale también allí.
	base := []rune(b.String())
	if len(base) > 60 {
		base = base[:60]
	}
	if len(base) == 0 {
		base = []rune("contacto") // el `|| 'contacto'` de la web
	}
	return string(base) + ".vcf"
}

// etiquetar pone o quita una etiqueta. OPTIMISTA, como la web: el cambio se ve al
// pulsar y vuelve atrás si el servicio dice que no. El PUT sustituye la pertenencia
// entera, así que se manda la lista completa de miembros.
func (a *App) etiquetar(g api.ContactGroup, c api.Contact) {
	scope := a.capturar()
	p := a.Pantalla
	if p.EtiquetaOcupada != "" {
		return
	}
	antes := slices.Clone(p.Etiquetas)
	miembros := slices.DeleteFunc(slices.Clone(g.Members), func(u string) bool { return u == c.UID })
	if len(miembros) == len(g.Members) {
		miembros = append(miembros, c.UID)
	}
	a.ponerMiembros(g.ID, miembros)
	p.EtiquetaOcupada = g.ID
	a.repintar()
	a.programarEn(scope, func(scope cargaContacts) {
		ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
		guardado, err := a.svc.RenameGroup(ctx, g.ID, g.Name, miembros)
		fin()
		fyne.Do(func() {
			if !a.vigente(scope) {
				return
			}
			p.EtiquetaOcupada = ""
			if err != nil {
				slog.Warn("contactos: no se pudo etiquetar", "error", err)
				p.Etiquetas = antes
				a.avisar(ui.AvisoPeligro, "detail.labelFailed", "common.tryAgainSoon")
			} else if guardado.ID != "" {
				a.ponerMiembros(g.ID, guardado.Members)
			}
			a.repintar()
		})
	})
}

func (a *App) ponerMiembros(id string, miembros []string) {
	gs := slices.Clone(a.Pantalla.Etiquetas)
	for i := range gs {
		if gs[i].ID == id {
			gs[i].Members = miembros
		}
	}
	a.Pantalla.Etiquetas = gs
}

// lote es `runBatch`: la acción de la barra sobre los marcados, en UNA petición (el
// servicio hace `uid = ANY`), y después la libreta, los destacados y las etiquetas otra
// vez. Mientras va, los botones de la barra se apagan (`disabled={busy}`).
func (a *App) lote(que AccionLote, sel []api.Contact, g *api.ContactGroup) {
	scope := a.capturar()
	p := a.Pantalla
	if a.svc == nil || len(sel) == 0 || p.LoteOcupado {
		return
	}
	uids := make([]string, len(sel))
	for i, c := range sel {
		uids[i] = api.CleanUID(c.UID)
	}
	n := len(sel)
	// tirados es lo que el servicio dice que mandó a la papelera: «Deshacer» compara
	// con eso, no con los marcados (un UID ajeno no se tira; uno en dos libretas, dos).
	tirados := 0
	var hacer func(context.Context) error
	var exito string
	switch que {
	case LoteDestacar, LoteQuitarDestacado:
		si := que == LoteDestacar
		hacer = func(ctx context.Context) error { return a.svc.BatchFavorite(ctx, uids, si) }
		exito = p.t("batch.unstarred", nil)
		if si {
			exito = p.t("batch.starred", i18n.Vars{"count": n})
		}
	case LoteAnadirEtiqueta, LoteQuitarEtiqueta:
		if g == nil {
			return
		}
		si, id := que == LoteAnadirEtiqueta, g.ID
		hacer = func(ctx context.Context) error { return a.svc.BatchGroupMembers(ctx, id, uids, si) }
		exito = p.t("batch.removedFrom", i18n.Vars{"label": g.Name})
		if si {
			exito = p.t("batch.addedTo", i18n.Vars{"label": g.Name})
		}
	case LoteAPapelera:
		hacer = func(ctx context.Context) (err error) {
			tirados, err = a.svc.BatchTrash(ctx, uids)
			return err
		}
		exito = p.t("batch.trashed", i18n.Vars{"count": n})
	}
	p.LoteOcupado = true
	a.repintar()
	a.programarEn(scope, func(scope cargaContacts) {
		ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
		err := hacer(ctx)
		fin()
		fyne.Do(func() {
			if !a.vigente(scope) {
				return
			}
			p.LoteOcupado = false
			if err != nil {
				slog.Warn("contactos: la acción en lote ha fallado", "error", err)
				a.avisar(ui.AvisoPeligro, "common.actionFailed", "common.tryAgainSoon")
				a.repintar()
				return
			}
			if que == LoteAPapelera {
				// Con «Deshacer»: el lote es donde más fácil se tira de más.
				a.avisarPapelera(exito, "", uids, tirados)
			} else {
				a.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, exito, ""))
			}
			a.recargarTodoEn(scope)
			a.programarEn(scope, a.cargarEtiquetasEn)
		})
	})
}

// elegirEImportar abre el selector y, con lo elegido, importa en segundo plano.
func (a *App) elegirEImportar(extensiones []string, importar func(cargaContacts, []byte)) {
	if a.ElegirFichero == nil || a.svc == nil || a.Pantalla.ImportOcupado {
		return
	}
	scope := a.capturar()
	a.ElegirFichero(extensiones, func(datos []byte) {
		if a.vigente(scope) {
			a.programarEn(scope, func(scope cargaContacts) { importar(scope, datos) })
		}
	})
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"cmp"
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"golang.org/x/text/collate"

	"git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/portapapeles"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// Buzón es Sytue Mail CON DATOS: habla con el servicio y rellena la Pantalla. Es el
// `MailApp.tsx` de la web sin su marcado —el marcado ya es la Pantalla— y cada flujo
// de aquí lleva al lado el nombre de la función de allí de la que sale.
//
// # En qué hilo corre cada cosa
//
// Todo el estado se toca en el hilo de la interfaz. Lo único que sale de él es la
// llamada al servicio, que corre en `Lanzar` y vuelve con `fyne.Do`. Así no hay
// cerrojos: un Buzón es de una ventana, y la ventana tiene un hilo.
//
// Una respuesta que llega tarde —la carpeta de antes, cuando ya se ha elegido otra—
// se tira por la VISTA: cada cambio de carpeta, etiqueta o búsqueda sube el número, y
// lo que vuelve con un número viejo no pinta. Es el `active = false` de cada efecto
// de la web.

// Servicio es lo que Sytue Mail le pide al servidor. Lo cumple *api.Mail; los
// guardianes le pasan un doble en memoria.
type Servicio interface {
	Mailboxes(ctx context.Context) ([]api.Mailbox, error)
	Messages(ctx context.Context, carpeta, cursor string, limite int) ([]api.MailMessage, string, error)
	LabelMessages(ctx context.Context, etiqueta, cursor string, limite int) ([]api.MailMessage, string, error)
	Search(ctx context.Context, q string, desde, limite int) ([]api.MailMessage, error)
	Thread(ctx context.Context, hilo string) ([]api.MailMessage, error)
	Raw(ctx context.Context, id string) (io.ReadCloser, error)
	SetFlagMany(ctx context.Context, ids []string, marca string, poner bool) error
	Labels(ctx context.Context) ([]api.MailLabel, error)
	Quota(ctx context.Context) (api.MailQuota, error)
	Changes(ctx context.Context, desde string) (api.MailCambios, error)
	// Llegadas es el aviso de correo nuevo por SSE: no vuelve hasta que se corta.
	Llegadas(ctx context.Context, alLlegar func()) error
	// El triaje: mover y etiquetar en lote, y el borrado definitivo, uno a uno porque
	// el servicio no tiene lote para eso.
	MoveMany(ctx context.Context, ids []string, carpeta string) error
	ApplyLabel(ctx context.Context, etiqueta string, ids []string, poner bool) error
	Delete(ctx context.Context, id string) error
	// La baja de una lista de correo (List-Unsubscribe): el servidor la ejecuta.
	Unsubscribe(ctx context.Context, id string) (api.MailBaja, error)
	DoUnsubscribe(ctx context.Context, id string) (string, error)
	// Escribir: firma, direcciones «De», enviar, guardar y deshacer.
	Remitente
	// El gestor de «Etiquetas».
	GestorEtiquetas
	// La autorespuesta de vacaciones.
	Vacaciones
	// El gestor de «Carpetas».
	GestorCarpetas
	// Los «Alias de correo».
	GestorAlias
	// Las «Reglas de correo».
	GestorReglas
	// La firma S/MIME: bajar el ancla y subir el certificado propio.
	Firma
	// Buzones compartidos y delegación.
	GestorCompartidos
}

var _ Servicio = (*api.Mail)(nil)

// Los números de la web.
const (
	// PorPagina es `PAGE_SIZE`: lo que se pide de una vez, en carpeta y en búsqueda.
	PorPagina = 50
	// EsperaBusqueda es el debounce de la búsqueda: no se pregunta por cada tecla.
	EsperaBusqueda = 300 * time.Millisecond
	// SondeoBase y SondeoTope son `POLL_BASE_MS` y `POLL_MAX_MS`: cada cuánto se
	// pregunta por el delta de la bandeja abierta, y hasta dónde se espacia tras
	// errores seguidos.
	SondeoBase = 10 * time.Second
	SondeoTope = 120 * time.Second
	// TopeLlegadas es lo más que se espera para reabrir el flujo de llegadas caído.
	TopeLlegadas = time.Minute
	// casiAlFinal es cuántas filas antes del final se pide la página siguiente. La web
	// observa un centinela con `rootMargin: 200px`, que a 75,75 por fila son 2,6 filas.
	casiAlFinal = 3
)

// ordenRoles es `ROLE_ORDER`: las carpetas del sistema van primero y en este orden.
var ordenRoles = []string{"inbox", "drafts", "sent", "archive", "junk", "trash"}

// Buzon es el estado de Sytue Mail con datos.
type Buzon struct {
	P *Pantalla
	S Servicio
	// Widget captures the adopted identity before the existing mailbox fetch.
	Widget widgets.Source
	// Lanzar corre una llamada al servicio fuera del hilo de la interfaz. Los
	// guardianes la hacen síncrona: así no dependen de cuándo acabe una goroutine.
	Lanzar func(func())
	// Esperar es el temporizador del debounce y del sondeo. Los guardianes lo
	// sustituyen para no dormir.
	Esperar func(time.Duration, func()) (parar func())
	// Fondo corre lo que vive mientras la ventana está abierta —el flujo de
	// llegadas—. Los guardianes lo sustituyen para no dejar nada vivo.
	Fondo func(func())
	// Hub resuelve los intents entre apps («Crear evento») y Abridor abre el enlace en
	// la app que toca. Sin cualquiera de los dos, el botón no se ofrece.
	Hub     Intents
	Abridor api.Abridor
	// Drive guarda un adjunto como fichero de Drive, por un acto explícito. Sin él, el
	// botón «Guardar en Drive» no se ofrece.
	Drive Guardador
	// Explorador navega Drive para «Adjuntar de Drive», y URLDrive es su dirección
	// pública, con la que se escribe el enlace. Llegan con ConDrive.
	Explorador ExploradorDrive
	URLDrive   string
	// Contactos sugiere direcciones al escribir en Para, CC y CCO. Sin él, se escriben
	// a mano. Libreta es «Elegir de Contactos», y URLContactos, la de la app.
	Contactos    Buscador
	Libreta      Libreta
	URLContactos string
	// ElegirFicheros es el panel de abrir del sistema («Adjuntar del equipo»). BLOQUEA:
	// se llama desde `pedir`. Los guardianes lo sustituyen.
	ElegirFicheros func() ([]string, error)
	// ElegirImagenes es el mismo panel filtrado a imágenes, para la imagen del cuerpo.
	ElegirImagenes func() ([]string, error)
	// Asistir es Sytue AI (`api.IA.Asistir`): con él, el redactor lleva el asistente al
	// final de su barra. Sin él (sin servicio), no sale.
	Asistir func(context.Context, api.PeticionIA) (string, error)
	// LeerHTML es el HTML del portapapeles del sistema, para pegar con formato. Los
	// guardianes lo sustituyen.
	LeerHTML func() (string, error)
	// Entrar vuelve a iniciar sesión en Sytue ID (`sesion.Suite.Entrar`). BLOQUEA hasta
	// volver del navegador. Sin él, una sesión caducada se dice pero no se ofrece botón.
	Entrar              func(context.Context) error
	CorreoExterno       CorreoExterno
	AutorizarCorreo     func(context.Context, string, string, ...string) (string, error)
	cerrarCorreoExterno func()
	correoAbierto       *correoExterno

	ctx         context.Context
	cancelar    context.CancelFunc
	ciclo       uint64
	identidad   string
	redacciones map[string]redaccionSuspendida

	carpetas   []api.Mailbox
	etiquetas  []api.MailLabel
	carpetaID  string
	etiquetaID string
	// compartidos son los buzones ajenos con acceso; compartido, el que se ve (nil,
	// el propio) y carpetaComp, su Recibidos.
	// peticion es la franja del buzón compartido abierto; buzonDelAviso, el del aviso
	// de una petición que se atiende; reabrir, el que se vuelve a abrir al llegar la
	// lista (compartidos.go).
	peticion      peticion.Estado
	buzonDelAviso string
	reabrir       string
	compartidos   []api.MailShared
	compartido    *api.MailShared
	carpetaComp   string
	// mensajes es la carpeta o la etiqueta, y resultados la búsqueda. Se guardan los
	// sobres del SERVICIO y no las conversaciones: el triaje, el delta y la
	// paginación trabajan por mensaje, y plegarlos es barato.
	mensajes   []api.MailMessage
	resultados []api.MailMessage
	cursor     string
	pidiendo   bool // hay una página siguiente en camino
	consulta   string
	// buscaMas es `searchHasMore`: la última página de resultados vino llena.
	buscaMas bool
	// hiloID es la conversación abierta, hilo su versión COMPLETA (atraviesa
	// carpetas) y cuerpos lo que ya se ha leído del .eml, por id de mensaje.
	hiloID  string
	hilo    []api.MailMessage
	cuerpos map[string]Cuerpo

	// cursorAtajo es la fila donde se quedó j/k: tras archivar, la siguiente ocupa su
	// sitio y j/k sigue desde ahí.
	cursorAtajo int
	// ancla es la fila del último clic en una casilla: de ahí sale el rango de
	// Mayúsculas + clic. -1 es ninguna.
	ancla int

	// La identidad del redactor —firma y direcciones «De»— y el redactor abierto.
	firma, principal string
	alias            []string
	redaccion        *redaccion
	// URLCuenta es «Clientes de correo»: dónde se crean las contraseñas de aplicación,
	// en Sytue ID (`${idBaseUrl}/account#app-passwords`). Vacía, no se ofrece.
	URLCuenta string

	vista        int
	hiloPedido   string // el de un aviso que llegó antes que la primera lista
	pararBusca   func()
	pararSondeo  func()
	estadoDelta  string
	esperaSondeo time.Duration
	// deltaEnCurso dice que hay un delta en camino, y otraVuelta, que llegó correo
	// mientras tanto: al volver se pregunta otra vez sin esperar.
	deltaEnCurso, otraVuelta bool
	segundoPlano             bool
}

// Cuerpo es lo que se enseña de un mensaje abierto.
type Cuerpo struct {
	Texto string
	// ConFormato dice que el correo traía HTML: lo que se ve es texto y no su maqueta,
	// y el lector lo avisa con `reader.plainTextOnly`.
	ConFormato bool
	// Fallo dice que el .eml no se pudo bajar: se queda el extracto del sobre.
	Fallo bool
	// Adjuntos son las partes que no son el cuerpo, con sus bytes: salen del mismo
	// .eml, así que descargarlos o guardarlos en Drive no pide nada más al servidor.
	Adjuntos []eml.Adjunto
}

// NuevoBuzon cablea la Pantalla con el servicio. No pide nada: eso es Arrancar.
func NuevoBuzon(p *Pantalla, s Servicio) *Buzon {
	b := &Buzon{
		P: p, S: s,
		Lanzar:         func(f func()) { go f() },
		Fondo:          func(f func()) { go f() },
		ElegirFicheros: ficheros.Elegir,
		ElegirImagenes: func() ([]string, error) { return ficheros.ElegirImagenes(p.T.T("editor.image", nil)) },
		LeerHTML:       portapapeles.HTML,
		Esperar:        esperar,
		ctx:            context.Background(),
		cuerpos:        map[string]Cuerpo{},
		ancla:          -1,
	}
	p.Menu.AlElegirCarpeta = b.ElegirCarpeta
	p.Menu.AlElegirEtiqueta = b.ElegirEtiqueta
	p.Menu.AlElegirCompartido = b.ElegirCompartido
	p.Menu.AlReintentarCarpetas = b.cargarCarpetas
	p.Menu.AlRefrescar = b.Refrescar
	p.Menu.AlGestionarEtiquetas = func() { b.P.CerrarCajon(); b.verEtiquetas() }
	p.Menu.AlGestionarCarpetas = func() { b.P.CerrarCajon(); b.verCarpetas() }
	p.Menu.AlPlegarAjustes = func() {
		b.P.Menu.AjustesAbiertos = !b.P.Menu.AjustesAbiertos
		b.P.Refrescar()
	}
	p.Bandeja.AlElegir = b.Abrir
	p.Bandeja.AlBuscar = b.Buscar
	p.Bandeja.AlReintentar = b.Reintentar
	p.Bandeja.Refresco.Actualizar = b.Reintentar
	p.Bandeja.Refresco.Puede = func() bool {
		return b.S != nil && b.ctx != nil && b.ctx.Err() == nil && !b.segundoPlano &&
			p.Bandeja.Estado != Cargando && p.Bandeja.Estado != SinPermiso &&
			!p.Bandeja.CargandoMas && p.Bandeja.CuantasMarcadas() == 0
	}
	p.Bandeja.Refresco.Revision = func() uint64 { return ui.RevisionLista(b.vista) }
	p.Bandeja.AlFinal = b.MasPagina
	p.Bandeja.AlMarcar = b.marcarFila
	p.Bandeja.AlVaciar = b.pedirVaciado
	p.Lector.AlDesplegar = b.pedirCuerpo
	p.Lector.Cuerpo = b.cuerpo
	p.Lector.AlDarseDeBaja = b.darseDeBaja
	p.Menu.AlRedactar, p.Bandeja.AlRedactar, p.Lector.AlRedactar = b.Redactar, b.Redactar, b.Redactar
	p.Lector.AlResponder = b.responder
	p.Menu.AlAbrir = b.abrirAjuste
	p.AlVolver = func() { b.cerrarHilo(); b.P.Refrescar() }
	p.Lector.AlDescargarAdjunto = b.descargarAdjunto
	p.Lector.AlOriginal = b.descargarOriginal
	return b
}

// ConDrive engancha Drive: con él aparece «Guardar en Drive» junto a cada adjunto.
//
// Si además sabe navegar (*api.Drive sí), el redactor ofrece «Adjuntar de Drive».
func (b *Buzon) ConDrive(d Guardador) *Buzon {
	b.Drive, b.Explorador = d, nil
	b.P.Lector.AlGuardarEnDrive = nil
	if d != nil {
		b.P.Lector.AlGuardarEnDrive = b.guardarEnDrive
	}
	if e, ok := d.(ExploradorDrive); ok {
		b.Explorador = e
	}
	return b
}

// ConImportar engancha «Trae tu correo» (`ImportData domains={['mail','contacts']}`,
// la pieza común `ui.ImportarDatos`) al pie del menú: aparece si la organización tiene
// conectado Google o Microsoft 365, y al acabar una importación la lista se recarga
// sola. alPolitica abre la política de privacidad del DataFlow.
func (b *Buzon) ConImportar(i *api.Integraciones, alPolitica func()) *Buzon {
	d := ui.NuevoImportarDatos(b.P.T, i, "mail", "contacts")
	d.AlPolitica = alPolitica
	d.AlTerminar = b.Refrescar
	d.AlCambiar = b.P.Refrescar
	b.P.Menu.Importar = d
	return b
}

// ConHub engancha el Hub y el abridor de apps: con ellos aparece «Crear evento».
func (b *Buzon) ConHub(h Intents, a api.Abridor) *Buzon {
	b.Hub, b.Abridor = h, a
	b.P.Lector.AlCrearEvento = nil
	if h != nil && a != nil {
		b.P.Lector.AlCrearEvento = b.crearEvento
	}
	return b
}

func esperar(d time.Duration, f func()) func() {
	t := time.AfterFunc(d, func() { fyne.Do(f) })
	return func() { t.Stop() }
}

// pedir hace una llamada fuera del hilo de la interfaz y vuelve a él con el
// resultado. Es la única puerta al servicio: todo lo que tarda pasa por aquí.
func pedir[T any](b *Buzon, llamada func(context.Context) (T, error), luego func(T, error)) {
	ctx, ciclo := b.ctx, b.ciclo
	b.Lanzar(func() {
		if ctx.Err() != nil {
			return
		}
		v, err := llamada(ctx)
		fyne.Do(func() {
			if ctx.Err() == nil && ciclo == b.ciclo {
				luego(v, err)
			}
		})
	})
}

// Arrancar pide lo que la pantalla necesita para empezar: carpetas —que eligen
// Recibidos y cargan su lista—, etiquetas y espacio.
func (b *Buzon) Arrancar(ctx context.Context) {
	if b.cancelar != nil {
		b.cancelar()
	}
	ctx, b.cancelar = context.WithCancel(ctx)
	b.ctx = ctx
	ciclo := b.ciclo + 1
	b.ciclo = ciclo
	//nolint:contextcheck // Estas cargas toman el contexto de vida guardado en b.ctx.
	b.cargarCarpetas()
	//nolint:contextcheck // Estas cargas toman el contexto de vida guardado en b.ctx.
	b.cargarEtiquetas()
	//nolint:contextcheck // Estas cargas toman el contexto de vida guardado en b.ctx.
	b.cargarCuota()
	//nolint:contextcheck // La firma y los alias usan la misma vida guardada en b.ctx mediante pedir.
	b.cargarIdentidad()
	//nolint:contextcheck // Estas cargas toman el contexto de vida guardado en b.ctx.
	b.cargarCompartidos()
	b.sondear()
	if d := b.P.Menu.Importar; d != nil {
		d.CargarCon(ctx)
	}
	b.Fondo(func() {
		api.Escuchar(ctx, TopeLlegadas, func(ctx context.Context) error {
			return b.S.Llegadas(ctx, func() {
				fyne.Do(func() {
					if ctx.Err() == nil && ciclo == b.ciclo {
						//nolint:contextcheck // El evento usa la vida del buzón guardada en b.ctx, ya contrastada con este SSE.
						b.llegada()
					}
				})
			})
		})
	})
}

// ---- Carpetas, etiquetas y espacio ----

// ordenarCarpetas es `sortMailboxes`: las del sistema por su rol y las de la persona
// por nombre con el orden del idioma (la Ä alemana y la Ñ no se colocan igual).
func ordenarCarpetas(l i18n.Locale, cs []api.Mailbox) []api.Mailbox {
	pos := func(m api.Mailbox) int {
		if i := slices.Index(ordenRoles, m.Role); i >= 0 && m.Role != "" {
			return i
		}
		return 99
	}
	col := collate.New(l.Tag())
	out := slices.Clone(cs)
	slices.SortStableFunc(out, func(a, c api.Mailbox) int {
		if d := cmp.Compare(pos(a), pos(c)); d != 0 {
			return d
		}
		return col.CompareString(a.Name, c.Name)
	})
	return out
}

func (b *Buzon) cargarCarpetas() {
	b.P.Menu.EstadoCarpetas = Cargando
	b.P.Refrescar()
	pedir(b, b.carpetasParaWidget(), func(cs []api.Mailbox, err error) {
		if err != nil {
			b.P.Menu.EstadoCarpetas = Error
			// La lista TAMBIÉN lo dice: en un teléfono el menú es un cajón cerrado y
			// su aviso no se ve (medido en la web el 2026-09-09 a 390 px).
			b.P.Bandeja.Estado = estadoDe(err)
			b.P.Refrescar()
			return
		}
		b.ponerCarpetas(cs)
		if b.carpetaID == "" && b.etiquetaID == "" {
			if i := slices.IndexFunc(b.carpetas, func(m api.Mailbox) bool { return m.Role == "inbox" }); i >= 0 {
				b.ElegirCarpeta(carpetaDe(b.carpetas[i]))
				return
			}
			if len(b.carpetas) > 0 {
				b.ElegirCarpeta(carpetaDe(b.carpetas[0]))
				return
			}
		}
		b.P.Refrescar()
	})
}

// ponerCarpetas deja las carpetas en el menú y el contador de Recibidos donde el
// sistema lo enseñe (el Dock en macOS): `ponerBadge` de la web.
func (b *Buzon) ponerCarpetas(cs []api.Mailbox) {
	b.carpetas = ordenarCarpetas(b.P.Locale, cs)
	b.P.Menu.Carpetas = b.P.Menu.Carpetas[:0]
	for _, c := range b.carpetas {
		b.P.Menu.Carpetas = append(b.P.Menu.Carpetas, carpetaDe(c))
	}
	b.P.Menu.EstadoCarpetas = Listo
	if len(b.carpetas) == 0 {
		b.P.Menu.EstadoCarpetas = Vacio
	}
	b.ponerAcciones() // los destinos del triaje salen de las carpetas
}

// refrescarContadores es `refreshMailboxCounts`: vuelve a pedir las carpetas para
// que el número del lateral diga la verdad, sin tocar la selección ni la lista.
// `luego` corre al volver, haya ido bien o no: el gestor de carpetas se repinta ahí.
func (b *Buzon) refrescarContadores(luego ...func()) {
	pedir(b, b.carpetasParaWidget(), func(cs []api.Mailbox, err error) {
		if err == nil {
			b.ponerCarpetas(cs)
			b.P.Refrescar()
		}
		for _, f := range luego {
			f()
		}
	})
}

// carpetasParaWidget reuses the existing complete inventory and server counter;
// it never infers unread messages from the current page or starts another fetch.
func (b *Buzon) carpetasParaWidget() func(context.Context) ([]api.Mailbox, error) {
	view := b.Widget.View()
	return func(ctx context.Context) ([]api.Mailbox, error) {
		mailboxes, err := b.S.Mailboxes(ctx)
		if err == nil {
			_ = view.Publicar(ctx, widgets.Counter{Kind: "unread", Value: int64(api.MailInboxUnread(mailboxes))})
		}
		return mailboxes, err
	}
}

func (b *Buzon) cargarEtiquetas() {
	pedir(b, b.S.Labels, func(es []api.MailLabel, err error) {
		if err != nil {
			return // best-effort, como en la web: sin etiquetas la bandeja sigue
		}
		b.etiquetas = es
		registro := make([]Etiqueta, 0, len(es))
		for _, e := range es {
			registro = append(registro, Etiqueta{ID: e.ID, Nombre: e.Name, Color: e.Color, SinLeer: e.UnreadEmails})
		}
		// Un solo registro para las tres: el menú, los chips de la lista y los menús
		// «Etiquetar» del lector.
		b.P.Menu.Etiquetas, b.P.Bandeja.Etiquetas, b.P.Acciones.Etiquetas = registro, registro, registro
		b.P.Refrescar()
	})
}

// cargarCuota alimenta el medidor del pie: `StorageMeter.tsx`.
func (b *Buzon) cargarCuota() {
	pedir(b, b.S.Quota, func(q api.MailQuota, err error) {
		m := b.P.Menu
		if err != nil {
			m.EstadoCuota = Error
		} else {
			m.EstadoCuota, m.CuotaFraccion, m.CuotaPorcentaje, m.CuotaUso, m.CuotaNota = Cuota(b.P.T, b.P.Locale, q)
		}
		b.P.Refrescar()
	})
}

// ---- La vista: carpeta, etiqueta o búsqueda ----

// ElegirCarpeta abre una carpeta. Borra la búsqueda y la etiqueta, como el `onClick`
// de un SidebarItem de carpeta.
func (b *Buzon) ElegirCarpeta(c Carpeta) {
	b.P.CerrarCajon()
	b.dejarCompartido()
	b.carpetaID, b.etiquetaID = c.ID, ""
	b.P.Menu.CarpetaID, b.P.Menu.EtiquetaID = c.ID, ""
	b.P.Bandeja.Titulo = RotuloCarpeta(b.P.T, c.Nombre, c.Rol)
	b.limpiarBusqueda()
	b.cargarLista()
}

// ElegirEtiqueta abre la vista de una etiqueta, que cruza carpetas.
//
// Suelta la carpeta, como el `setSelectedMailbox(null)` de la web: con una etiqueta
// no se está EN ninguna carpeta, así que el triaje ofrece todas y el hilo esconde lo
// de Papelera y Spam.
func (b *Buzon) ElegirEtiqueta(e Etiqueta) {
	b.P.CerrarCajon()
	b.dejarCompartido()
	b.etiquetaID, b.carpetaID = e.ID, ""
	b.P.Menu.EtiquetaID, b.P.Menu.CarpetaID = e.ID, ""
	b.P.Bandeja.Titulo = e.Nombre
	b.limpiarBusqueda()
	b.cargarLista()
}

func (b *Buzon) limpiarBusqueda() {
	if b.pararBusca != nil {
		b.pararBusca()
	}
	b.consulta, b.resultados = "", nil
	b.P.Bandeja.Buscando, b.P.Bandeja.Consulta = false, ""
}

// cargarLista pide la primera página de la vista. Es el efecto de `[viewKind, viewId,
// listTry]`: cambiar de vista suelta la conversación abierta y la selección.
func (b *Buzon) cargarLista() {
	b.vista++
	vista := b.vista
	b.cerrarHilo()
	b.mensajes, b.cursor, b.pidiendo = nil, "", false
	b.P.Bandeja.Marcadas, b.ancla = map[string]bool{}, -1
	b.cursorAtajo = 0 // otra lista: j empieza por arriba
	b.P.Bandeja.Estado = Cargando
	b.P.Bandeja.Conversaciones = nil
	b.P.Refrescar()
	b.estadoDelta = "" // otra vista, otro arranque del delta

	carpeta, etiqueta, comp := b.carpetaID, b.etiquetaID, b.compartido
	pedir(b, func(ctx context.Context) (pagina, error) {
		if comp != nil {
			return b.paginaCompartida(ctx, comp.AccountID, "", "")
		}
		return b.pagina(ctx, carpeta, etiqueta, "")
	}, func(p pagina, err error) {
		if vista != b.vista {
			return
		}
		if err != nil {
			b.P.Bandeja.Estado = estadoDe(err)
			b.P.Refrescar()
			return
		}
		if comp != nil {
			b.carpetaComp = p.carpeta
		}
		b.mensajes, b.cursor = p.mensajes, p.siguiente
		b.pintarLista()
		if h := b.hiloPedido; h != "" {
			b.hiloPedido = ""
			b.AbrirHilo(h)
		}
	})
}

// AbrirHilo abre una conversación por su id: la de un aviso del Hub
// (`sytue-mail:/mail/?thread=`). Como en la web (`hiloPedido`), espera a la primera
// lista, porque cargarla suelta la conversación abierta.
func (b *Buzon) AbrirHilo(id string) {
	if b.P.Bandeja.Estado == Cargando {
		b.hiloPedido = id
		return
	}
	for _, c := range b.P.Bandeja.Conversaciones {
		if c.HiloID == id {
			b.Abrir(c)
			return
		}
	}
	b.Abrir(Conversacion{HiloID: id}) // fuera de esta carpeta: el lector pide el hilo
}

type pagina struct {
	mensajes  []api.MailMessage
	siguiente string
	// carpeta es la del buzón compartido, que la primera página tiene que buscar.
	carpeta string
}

func (b *Buzon) pagina(ctx context.Context, carpeta, etiqueta, cursor string) (pagina, error) {
	var (
		p   pagina
		err error
	)
	if etiqueta != "" {
		p.mensajes, p.siguiente, err = b.S.LabelMessages(ctx, etiqueta, cursor, PorPagina)
	} else {
		p.mensajes, p.siguiente, err = b.S.Messages(ctx, carpeta, cursor, PorPagina)
	}
	return p, err
}

// Reintentar es el botón de los estados de error: lo que falló fueron las carpetas,
// la búsqueda o la lista, y se reintenta eso.
func (b *Buzon) Reintentar() {
	switch {
	case b.P.Menu.EstadoCarpetas == Error:
		b.cargarCarpetas()
	case b.consulta != "":
		b.buscarYa(b.consulta)
	default:
		b.cargarLista()
	}
}

// Refrescar es el botón de actualizar de la carpeta activa: vuelve a pedir la lista.
func (b *Buzon) Refrescar() { b.cargarLista() }

// MasPagina es el scroll infinito: `handleLoadMore`. No hace nada si ya hay una en
// camino o no quedan. En búsqueda pagina por posición (lo ya recibido), en carpeta y
// etiqueta por cursor.
func (b *Buzon) MasPagina() {
	if b.consulta != "" {
		b.masResultados()
		return
	}
	if b.pidiendo || b.cursor == "" {
		return
	}
	b.pidiendo = true
	b.P.Bandeja.CargandoMas = true
	b.P.Bandeja.Refresh()
	vista, carpeta, etiqueta, cursor, comp, carpetaComp := b.vista, b.carpetaID, b.etiquetaID, b.cursor, b.compartido, b.carpetaComp
	pedir(b, func(ctx context.Context) (pagina, error) {
		if comp != nil {
			return b.paginaCompartida(ctx, comp.AccountID, carpetaComp, cursor)
		}
		return b.pagina(ctx, carpeta, etiqueta, cursor)
	}, func(p pagina, err error) {
		if vista != b.vista {
			return
		}
		b.pidiendo = false
		b.P.Bandeja.CargandoMas = false
		if err != nil {
			b.cursor = "" // la web deja de pedir: el botón de reintentar ya es la lista entera
			b.P.Bandeja.Refresh()
			return
		}
		b.mensajes = sinRepetir(b.mensajes, p.mensajes)
		b.cursor = p.siguiente
		b.pintarLista()
	})
}

// masResultados es la rama de búsqueda de `handleLoadMore`: la página que sigue a lo
// ya recibido; una más corta que PorPagina es la última, y un fallo deja de pedir.
func (b *Buzon) masResultados() {
	if b.pidiendo || !b.buscaMas {
		return
	}
	b.pidiendo = true
	b.P.Bandeja.CargandoMas = true
	b.P.Bandeja.Refresh()
	vista, q, desde := b.vista, b.consulta, len(b.resultados)
	pedir(b, func(ctx context.Context) ([]api.MailMessage, error) {
		return b.S.Search(ctx, q, desde, PorPagina)
	}, func(ms []api.MailMessage, err error) {
		if vista != b.vista {
			return
		}
		b.pidiendo = false
		b.P.Bandeja.CargandoMas = false
		b.buscaMas = err == nil && len(ms) == PorPagina
		if err != nil {
			b.P.Bandeja.Refresh()
			return
		}
		b.resultados = sinRepetir(b.resultados, ms)
		b.pintarLista()
	})
}

// sinRepetir es `appendUnique`: dos páginas pueden solaparse si entra correo entre una
// y otra, y un mensaje no puede salir dos veces.
func sinRepetir(antes, nuevos []api.MailMessage) []api.MailMessage {
	vistos := make(map[string]bool, len(antes))
	for _, m := range antes {
		vistos[m.ID] = true
	}
	out := slices.Clone(antes)
	for _, m := range nuevos {
		if !vistos[m.ID] {
			out = append(out, m)
		}
	}
	return out
}

// Buscar es el campo de la lista: vacío vuelve a la carpeta, y lo demás pregunta
// cuando se deja de teclear 300 ms.
func (b *Buzon) Buscar(q string) {
	if b.pararBusca != nil {
		b.pararBusca()
		b.pararBusca = nil
	}
	b.P.Bandeja.Consulta = q
	if strings.TrimSpace(q) == "" {
		b.consulta, b.resultados = "", nil
		b.P.Bandeja.Buscando = false
		b.vista++
		b.pintarLista()
		return
	}
	b.pararBusca = b.Esperar(EsperaBusqueda, func() { b.buscarYa(q) })
}

func (b *Buzon) buscarYa(q string) {
	q = strings.TrimSpace(q)
	b.vista++
	vista := b.vista
	b.consulta = q
	b.cerrarHilo()
	b.P.Bandeja.Buscando = true
	b.P.Bandeja.Estado = Cargando
	b.P.Refrescar()
	b.buscaMas, b.pidiendo = false, false
	pedir(b, func(ctx context.Context) ([]api.MailMessage, error) {
		return b.S.Search(ctx, q, 0, PorPagina)
	}, func(ms []api.MailMessage, err error) {
		if vista != b.vista {
			return
		}
		if err != nil {
			// Una búsqueda que FALLA no es una búsqueda sin resultados: decir «no hay
			// nada» da por cierto que ese correo no existe.
			b.resultados = nil
			b.P.Bandeja.Estado = estadoDe(err)
			b.P.Refrescar()
			return
		}
		b.resultados = ms
		b.buscaMas = len(ms) == PorPagina
		b.pintarLista()
	})
}

// visibles es lo que la lista enseña: los resultados en búsqueda y la vista si no.
func (b *Buzon) visibles() []api.MailMessage {
	if b.consulta != "" {
		return b.resultados
	}
	return b.mensajes
}

// pintarLista pliega los sobres en conversaciones y los pasa a la bandeja.
func (b *Buzon) pintarLista() {
	vis := b.visibles()
	mensajes := make([]Mensaje, 0, len(vis))
	for _, m := range vis {
		mensajes = append(mensajes, MensajeDe(m))
	}
	b.P.Bandeja.Conversaciones = AgruparPorHilo(mensajes)
	b.P.Bandeja.HayMas = b.cursor != ""
	if b.consulta != "" {
		b.P.Bandeja.HayMas = b.buscaMas
	}
	b.ponerAcciones()
	b.P.Bandeja.Estado = Listo
	if len(mensajes) == 0 {
		b.P.Bandeja.Estado = Vacio
	}
	b.pintarHilo()
	b.P.Refrescar()
}

// ---- La conversación abierta ----

// Abrir es `handleSelectConversation`: abre el hilo, lo marca leído en UNA llamada y
// pide la conversación completa y el cuerpo del último mensaje. En Borradores no abre
// el lector: reabre el redactor para seguir escribiendo.
func (b *Buzon) Abrir(c Conversacion) {
	if b.rolVista() == "drafts" {
		b.abrirBorrador(c.Ultimo)
		return
	}
	b.hiloID = c.HiloID
	b.hilo = nil
	b.P.Bandeja.AbiertaID = c.HiloID
	b.P.Lector.Estado = Listo
	b.pintarHilo()
	b.P.DetalleAbierto(true)
	b.P.Refrescar()

	var sinLeer []string
	for _, m := range c.Mensajes {
		if !m.Leido {
			sinLeer = append(sinLeer, m.ID)
		}
	}
	if len(sinLeer) > 0 && !b.P.Bandeja.SoloLectura {
		b.marcar(sinLeer, api.MailFlagSeen, true)
		pedir(b, func(ctx context.Context) (struct{}, error) {
			return struct{}{}, b.S.SetFlagMany(ctx, sinLeer, api.MailFlagSeen, true)
		}, func(_ struct{}, err error) {
			if err != nil {
				b.marcar(sinLeer, api.MailFlagSeen, false)
				b.pintarLista()
				return
			}
			// Y SE REFRESCAN LOS CONTADORES: el parche es optimista y el número del
			// lateral sale de las carpetas. La web los pedía al mover y no al leer.
			b.refrescarContadores()
		})
	}
	// En un buzón compartido no se pide el hilo: sería de otra cuenta y el servicio,
	// que responde por la propia, lo devolvería vacío.
	if !strings.HasPrefix(c.HiloID, "msg:") && b.compartido == nil {
		hiloID := c.HiloID
		pedir(b, func(ctx context.Context) ([]api.MailMessage, error) {
			return b.S.Thread(ctx, hiloID)
		}, func(ms []api.MailMessage, err error) {
			if err != nil || hiloID != b.hiloID || len(ms) == 0 {
				return // degrada a los mensajes de la carpeta, sin error visible
			}
			b.hilo = ms
			b.pintarHilo()
			b.P.Lector.Refresh()
			// El que se despliega es el último del hilo ENTERO, que puede no estar en
			// la carpeta: tu respuesta desde Enviados. Y la baja se pregunta por ESE.
			if u := b.P.Lector.Conversacion; len(u.Mensajes) > 0 {
				b.pedirCuerpo(u.Mensajes[0].ID)
				if len(c.Mensajes) == 0 || u.Mensajes[0].ID != c.Mensajes[0].ID {
					b.consultarBaja(u.Mensajes[0].ID)
				}
			}
		})
	}
	if len(c.Mensajes) > 0 {
		b.pedirCuerpo(c.Mensajes[0].ID)
		b.consultarBaja(c.Mensajes[0].ID)
	}
}

// cerrarHilo suelta la conversación abierta: la lista vuelve a mandar en estrecho.
func (b *Buzon) cerrarHilo() {
	b.hiloID, b.hilo = "", nil
	b.P.Bandeja.AbiertaID = ""
	b.P.Lector.Conversacion = Conversacion{}
	b.P.Lector.Estado = Vacio
	b.P.Lector.Baja, b.P.Lector.DandoDeBaja = false, false
	b.P.DetalleAbierto(false)
}

// ocultosEnHilo es `hiddenInThread`: lo tirado a la papelera o al spam no reaparece
// dentro de un hilo de Recibidos, salvo estando EN esa carpeta.
func (b *Buzon) ocultosEnHilo() map[string]bool {
	ocultos := map[string]bool{}
	for _, m := range b.carpetas {
		if (m.Role == "trash" || m.Role == "junk") && m.ID != b.carpetaID {
			ocultos[m.ID] = true
		}
	}
	return ocultos
}

// pintarHilo pone en el lector la conversación abierta: la de la lista y, si ya ha
// llegado, fundida con el hilo completo (`mergeThread`).
func (b *Buzon) pintarHilo() {
	if b.hiloID == "" {
		return
	}
	var c Conversacion
	for _, x := range b.P.Bandeja.Conversaciones {
		if x.HiloID == b.hiloID {
			c = x
			break
		}
	}
	if len(b.hilo) > 0 {
		c = FundirHilo(c, b.hilo, b.ocultosEnHilo())
	}
	if len(c.Mensajes) == 0 {
		return // el hilo ya no está a la vista (movido, borrado): se queda el anterior
	}
	b.P.Lector.Conversacion = c
}

// FundirHilo es `mergeThread` + `conversacionAbierta`: los mensajes del hilo entero,
// con la versión LOCAL de los que la lista ya tiene (lleva el triaje optimista) y sin
// los que viven en carpetas ocultas. Si la lista no tenía el hilo —se llegó por
// enlace—, se arma solo con el completo.
func FundirHilo(c Conversacion, hilo []api.MailMessage, ocultos map[string]bool) Conversacion {
	local := make(map[string]Mensaje, len(c.Mensajes))
	for _, m := range c.Mensajes {
		local[m.ID] = m
	}
	var ms []Mensaje
	for _, m := range hilo {
		if l, ya := local[m.ID]; ya {
			ms = append(ms, l)
			continue
		}
		if !ocultos[m.MailboxID] {
			ms = append(ms, MensajeDe(m))
		}
	}
	if len(ms) == 0 {
		return c
	}
	slices.SortStableFunc(ms, func(a, x Mensaje) int { return x.Recibido.Compare(a.Recibido) })
	f := AgruparPorHilo(ms)[0]
	f.HiloID = cmp.Or(c.HiloID, f.HiloID)
	return f
}

// marcar aplica una marca a unos mensajes en las dos listas, sin esperar al servidor.
func (b *Buzon) marcar(ids []string, marca string, poner bool) {
	cual := make(map[string]bool, len(ids))
	for _, id := range ids {
		cual[id] = true
	}
	for _, lista := range [][]api.MailMessage{b.mensajes, b.resultados, b.hilo} {
		for i := range lista {
			if cual[lista[i].ID] {
				lista[i].Keywords = conMarca(lista[i].Keywords, marca, poner)
			}
		}
	}
	b.pintarLista()
}

func conMarca(ks []string, marca string, poner bool) []string {
	ks = slices.DeleteFunc(slices.Clone(ks), func(k string) bool { return strings.EqualFold(k, marca) })
	if poner {
		ks = append(ks, marca)
	}
	return ks
}

// pedirCuerpo baja el .eml de un mensaje y se queda con su texto. Una vez por mensaje:
// al plegar y volver a desplegar no se pide otra vez.
func (b *Buzon) pedirCuerpo(id string) {
	if _, ya := b.cuerpos[id]; ya || id == "" {
		return
	}
	pedir(b, func(ctx context.Context) (eml.Mensaje, error) {
		r, err := b.S.Raw(ctx, id)
		if err != nil {
			return eml.Mensaje{}, err
		}
		defer r.Close()
		return eml.Leer(r)
	}, func(m eml.Mensaje, err error) {
		b.cuerpos[id] = CuerpoDe(m, err)
		b.P.Lector.Refresh()
	})
}

// CuerpoDe elige qué se enseña de un .eml leído. El TEXTO primero y el HTML pasado a
// texto después: aquí no hay visor de HTML (ver `TarjetaMensaje`), y el texto que el
// remitente escribió dice más que el que sale de quitarle las etiquetas a su maqueta.
func CuerpoDe(m eml.Mensaje, err error) Cuerpo {
	conFormato := strings.TrimSpace(m.HTML) != ""
	if texto := strings.TrimSpace(m.Texto); texto != "" {
		return Cuerpo{Texto: strings.ReplaceAll(texto, "\r\n", "\n"), ConFormato: conFormato, Adjuntos: m.Adjuntos}
	}
	if conFormato {
		return Cuerpo{Texto: htmltext.Text(m.HTML), ConFormato: true, Adjuntos: m.Adjuntos}
	}
	return Cuerpo{Fallo: err != nil, Adjuntos: m.Adjuntos}
}

// cuerpo es lo que el lector pregunta por cada tarjeta.
func (b *Buzon) cuerpo(id string) (Cuerpo, bool) {
	c, ya := b.cuerpos[id]
	return c, ya
}

// ---- El delta de la bandeja abierta ----

// sondear es el refresco en vivo: pregunta por el delta cada diez segundos y aplica
// SOLO lo cambiado, sin recargar la bandeja. Tras errores seguidos se espacia hasta
// dos minutos, y con una página cortada (`has_more`) vuelve a preguntar ya. El aviso
// de llegada (llegada) lo adelanta: el sondeo queda de respaldo si el flujo se cae.
func (b *Buzon) sondear() {
	if b.segundoPlano || b.ctx.Err() != nil {
		return
	}
	if b.esperaSondeo == 0 {
		b.esperaSondeo = SondeoBase
	}
	b.pararSondeo = b.Esperar(b.esperaSondeo, b.vueltaDelta)
}

// llegada es el `llegada` del flujo: pregunta por el delta YA, sin esperar a la vuelta
// del sondeo. Si ya hay uno en camino no se lanza otro —serían dos cadenas de
// sondeo—: se pide otra vuelta en cuanto acabe.
func (b *Buzon) llegada() {
	if b.segundoPlano || b.ctx.Err() != nil {
		return
	}
	if b.deltaEnCurso {
		b.otraVuelta = true
		return
	}
	if b.pararSondeo != nil {
		b.pararSondeo()
	}
	b.vueltaDelta()
}

func (b *Buzon) vueltaDelta() {
	if b.segundoPlano || b.ctx.Err() != nil {
		return
	}
	if b.consulta != "" || (b.carpetaID == "" && b.etiquetaID == "") {
		b.sondear() // en búsqueda no aplica: es una vista de paso
		return
	}
	vista, desde := b.vista, b.estadoDelta
	b.deltaEnCurso = true
	pedir(b, func(ctx context.Context) (api.MailCambios, error) {
		return b.S.Changes(ctx, desde)
	}, func(c api.MailCambios, err error) {
		b.deltaEnCurso = false
		if b.otraVuelta && !b.segundoPlano {
			b.otraVuelta = false
			defer func() { b.pararSondeo(); b.vueltaDelta() }()
		}
		if err != nil {
			b.esperaSondeo = min(b.esperaSondeo*2, SondeoTope)
			b.sondear()
			return
		}
		b.esperaSondeo = SondeoBase
		if vista == b.vista {
			arrancado := b.estadoDelta != ""
			b.estadoDelta = c.State
			if arrancado && (len(c.Changed) > 0 || len(c.Destroyed) > 0) {
				b.aplicarDelta(c)
			}
		}
		if c.HasMore {
			b.esperaSondeo = time.Millisecond // drena la ráfaga sin esperar al intervalo
		}
		b.sondear()
	})
}

// aplicarDelta es el `apply` del sondeo: quita lo borrado, pone o quita lo cambiado
// según pertenezca a la vista, y reordena del más nuevo al más viejo.
func (b *Buzon) aplicarDelta(c api.MailCambios) {
	b.mensajes = AplicarDelta(b.mensajes, c, b.carpetaID, b.etiquetaID)
	b.pintarLista()
	b.refrescarContadores()
}

// AplicarDelta es la cuenta pura del sondeo, aparte para que el guardián la mire.
func AplicarDelta(ms []api.MailMessage, c api.MailCambios, carpeta, etiqueta string) []api.MailMessage {
	fuera := make(map[string]bool, len(c.Destroyed))
	for _, id := range c.Destroyed {
		fuera[id] = true
	}
	porID := map[string]api.MailMessage{}
	for _, m := range ms {
		if !fuera[m.ID] {
			porID[m.ID] = m
		}
	}
	for _, m := range c.Changed {
		if fuera[m.ID] {
			continue
		}
		dentro := m.MailboxID == carpeta
		if etiqueta != "" {
			dentro = slices.Contains(m.Keywords, etiqueta)
		}
		if dentro {
			porID[m.ID] = m
		} else {
			delete(porID, m.ID) // salió de esta vista: movido a otra carpeta
		}
	}
	out := make([]api.MailMessage, 0, len(porID))
	for _, m := range porID {
		out = append(out, m)
	}
	slices.SortFunc(out, func(a, x api.MailMessage) int {
		if d := x.ReceivedAt.Compare(a.ReceivedAt); d != 0 {
			return d
		}
		return strings.Compare(x.ID, a.ID)
	})
	return out
}

// Parar suelta los temporizadores: la ventana se cierra.
func (b *Buzon) Parar() {
	b.ciclo++
	if b.cancelar != nil {
		b.cancelar()
	}
	if d := b.P.Menu.Importar; d != nil {
		d.Parar()
	}
	if b.cerrarCorreoExterno != nil {
		b.cerrarCorreoExterno()
	}
	for _, f := range []func(){b.pararBusca, b.pararSondeo} {
		if f != nil {
			f()
		}
	}
	if b.redaccion != nil && b.redaccion.pararAuto != nil {
		b.redaccion.pararAuto()
	}
}

// ---- Traducciones del servicio a la pantalla ----

// MensajeDe traduce un sobre del servicio a lo que pinta la pantalla. Es
// `toEmailSummary` de `lib/mailrest.ts`: las marcas de IMAP pasan a ser estado y el
// resto de keywords son ids de etiqueta.
func MensajeDe(m api.MailMessage) Mensaje {
	x := Mensaje{
		ID: m.ID, MessageID: m.MessageID, HiloID: m.ThreadID, CarpetaID: m.MailboxID,
		Asunto: m.Subject, Extracto: m.Preview, Recibido: m.ReceivedAt,
		Leido: m.Has(api.MailFlagSeen), Destacado: m.Has(api.MailFlagFlagged),
		ConAdjunto: m.HasAttachment, EnHilo: m.ThreadCount,
	}
	if m.From != "" {
		x.De = []Direccion{{Nombre: m.FromName, Correo: m.From}}
	}
	for _, d := range m.To {
		x.Para = append(x.Para, Direccion{Correo: d})
	}
	for _, d := range m.Cc {
		x.CC = append(x.CC, Direccion{Correo: d})
	}
	for _, k := range m.Keywords {
		if !strings.HasPrefix(k, `\`) && !strings.HasPrefix(k, "$") {
			if x.Etiquetas == nil {
				x.Etiquetas = map[string]bool{}
			}
			x.Etiquetas[k] = true
		}
	}
	return x
}

func carpetaDe(m api.Mailbox) Carpeta {
	return Carpeta{ID: m.ID, Nombre: m.Name, Rol: m.Role, SinLeer: m.UnreadEmails, Total: m.TotalEmails}
}

// estadoDe decide qué estado vacío dice la verdad sobre un error: un 403 es falta de
// permiso, un error que no es una respuesta del servidor es que no hay red, y lo
// demás es un fallo con reintento.
func estadoDe(err error) Estado {
	var e *api.Error
	switch {
	case errors.As(err, &e) && e.Status == http.StatusForbidden:
		return SinPermiso
	case errors.As(err, &e):
		return Error
	case errors.Is(err, context.Canceled):
		return Error
	}
	return SinConexion
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain define los tipos y reglas de negocio de Sytue Hub, el backend de
// integración seamless de la suite (shell backend): búsqueda federada,
// notificaciones unificadas, presencia e intents cross-app. Independiente de
// transporte (HTTP/SSE) y de almacenamiento (PostgreSQL) y de los conectores.
//
// Clean-room: modelo y protocolos propios (REST + SSE sobre RFC 8895/EventSource,
// LISTEN/NOTIFY de PostgreSQL); sin código de terceros AGPL (CONVENTIONS §0.2).
package domain

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
)

// Errores de dominio. La capa HTTP los traduce a *httpx.Error.
var (
	ErrNotFound      = errors.New("hub: recurso no encontrado")
	ErrInvalidInput  = errors.New("hub: entrada inválida")
	ErrForbidden     = errors.New("hub: no autorizado para esta acción")
	ErrUnknownSource = errors.New("hub: origen de búsqueda desconocido")
	ErrUnknownIntent = errors.New("hub: intent no soportado")
	// ErrTooMany: el emisor ya ha dejado a esa persona el tope de avisos de la hora.
	ErrTooMany = errors.New("hub: demasiados avisos para ese destinatario")
	// ErrGone: la ruta de mantenimiento ya cerró (HistoricoCierre).
	ErrGone = errors.New("hub: esa operación de mantenimiento ya no existe")
)

// Source identifica un servicio de la suite consultable por la búsqueda federada
// y referenciable por las notificaciones/intents.
type Source string

const (
	SourceDrive    Source = "drive"
	SourceMail     Source = "mail"
	SourceCalendar Source = "calendar"
	SourceContacts Source = "contacts"
	SourceChat     Source = "chat"
	SourceRooms    Source = "rooms"
	// SourceNotes es Sytue Notepads: cuadernos y notas. Solo federa lo que su dueño
	// ha dejado en el índice (cada cuaderno y cada nota llevan su interruptor).
	SourceNotes Source = "notes"
	// SourceERP es SyERP: clientes, facturas, compras, inventario y gestión.
	SourceERP Source = "erp"
	// SourceIntegrations agrupa los SaaS CONECTADOS vía el Sytue Integration Gateway
	// (Microsoft 365, Google Workspace…): su contenido se federa en la búsqueda
	// unificada a través de un único conector que consulta el gateway.
	SourceIntegrations Source = "integrations"
	// SourceCloud es Sytue Cloud: avisos del ciclo de vida de los despliegues
	// (listo/fallido) para quien los lanzó. No federa búsqueda: es solo
	// notificación — ver services/cloud/internal/service/notify.go.
	SourceCloud Source = "cloud"
	// SourcePages es Sytue Web: federa la búsqueda de los sitios de la organización y deja
	// avisos de pedidos, formularios, desistimientos, dominios y exportaciones (tipos `pages.*`).
	SourcePages Source = "pages"
	// SourceBases es Sytue Bases (tablas relacionales, ADR 0068): federa la búsqueda de
	// las bases a las que la persona tiene acceso. No deja avisos.
	SourceBases Source = "bases"
	// SourceDocs es Sytue Docus/Calcus/Diapos/certiDocs: aviso de «compartido
	// contigo». No federa búsqueda: solo notificación.
	SourceDocs Source = "docs"
)

// AllSources es el conjunto de orígenes federados por defecto.
func AllSources() []Source {
	return []Source{SourceDrive, SourceMail, SourceCalendar, SourceContacts, SourceChat, SourceRooms, SourceNotes, SourceERP, SourcePages, SourceBases, SourceIntegrations, SourceCloud, SourceDocs}
}

// Orígenes que solo avisan: no se buscan en ellos, pero su aviso tiene que abrir SU app
// nativa. Un documento de Calcus que llega como `docs` lo abre Docus; con su origen, lo
// abre la hoja de cálculo. Son las claves de producto de la web (`ProductKey`).
const (
	SourceSheets Source = "sheets"
	SourceSlides Source = "slides"
	SourcePDF    Source = "pdf"
	SourcePhotos Source = "photos"
	// SourceID es Sytue ID: los avisos de seguridad de la propia cuenta (un inicio de
	// sesión nuevo, la contraseña o la verificación en dos pasos que cambian).
	SourceID Source = "id"
	// SourceOne es la suite como tal: «hay una versión nueva de la app», que deja la
	// app de escritorio para quien la tiene instalada y abre la página de descargas.
	SourceOne Source = "one"
	// SourceCast es Sytue Cast: «está en directo» (services/cast/internal/directo).
	// Faltaba, y el Hub devolvía ErrUnknownSource: el aviso se perdía sin error visible.
	SourceCast Source = "cast"
)

// NotificationSources son los orígenes que pueden dejar un aviso.
func NotificationSources() []Source {
	return append(AllSources(), SourceSheets, SourceSlides, SourcePDF, SourcePhotos, SourceID, SourceOne, SourceCast)
}

// ValidNotificationSource indica si s puede dejar un aviso.
func ValidNotificationSource(s Source) bool {
	switch s {
	case SourceSheets, SourceSlides, SourcePDF, SourcePhotos, SourceID, SourceOne, SourceCast:
		return true
	default:
		return ValidSource(s)
	}
}

// ValidSource indica si s es un origen conocido.
func ValidSource(s Source) bool {
	switch s {
	case SourceDrive, SourceMail, SourceCalendar, SourceContacts, SourceChat, SourceRooms, SourceNotes, SourceERP, SourcePages, SourceBases, SourceIntegrations, SourceCloud, SourceDocs:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Búsqueda federada
// ---------------------------------------------------------------------------

// SearchQuery son los parámetros normalizados de una búsqueda federada.
type SearchQuery struct {
	// Text es el término de búsqueda (no vacío tras recortar).
	Text string
	// Sources restringe los orígenes a consultar (vacío = todos).
	Sources []Source
	// Limit es el máximo de resultados por origen.
	Limit int
	// Offset descarta los N primeros resultados del AGREGADO ya ordenado. Es lo que
	// permite pasar de página: sin él, una búsqueda con muchos resultados enseñaba el
	// primer puñado y no había forma de ver el resto.
	//
	// Se pagina sobre el agregado y no por origen a propósito: el orden que le importa
	// al usuario es el de relevancia entre TODOS los orígenes, y un cursor por origen
	// no puede reproducirlo. El coste es pedir Limit+Offset a cada uno.
	Offset int
	// From y To acotan por fecha del recurso (su Timestamp). Nil = sin acotar. El
	// filtro se aplica en el Hub, no en cada origen: los conectores hablan APIs
	// distintas y no todas admiten rango, así que hacerlo aquí lo garantiza para todos.
	From *time.Time
	To   *time.Time
	// Kinds restringe por tipo de entidad dentro del origen (file, message, event…).
	// Vacío = todos. Es el filtro que pide quien busca «solo ficheros».
	Kinds []string
}

// SearchHit es un resultado de búsqueda normalizado, común a todos los orígenes.
// Cada conector traduce su modelo propio a esta forma canónica para el shell.
type SearchHit struct {
	// Source es el servicio del que procede el resultado.
	Source Source `json:"source"`
	// Kind es el tipo de entidad dentro del origen (file, message, event, ...).
	Kind string `json:"kind"`
	// ID es el identificador del recurso en su origen.
	ID string `json:"id"`
	// Title es el texto principal a mostrar.
	Title string `json:"title"`
	// Snippet es un fragmento de contexto (cuerpo, descripción), opcional.
	Snippet string `json:"snippet,omitempty"`
	// SizeBytes es el tamaño de un fichero. Lo formatea el cliente en el idioma de
	// quien busca: el Snippet de Drive lo dice en texto («11.7 KiB»), que salía igual en
	// español que en inglés y con otras unidades que el resto de la suite. El Snippet se
	// queda para los clientes que aún no lo leen.
	SizeBytes *int64 `json:"size_bytes,omitempty"`
	// URL es el deep-link a la superficie de cara al usuario.
	URL string `json:"url"`
	// Icon es una pista de icono para el shell (mime/category), opcional.
	Icon string `json:"icon,omitempty"`
	// Timestamp es la fecha relevante del recurso (modificación/creación).
	Timestamp *time.Time `json:"timestamp,omitempty"`
	// Score es una relevancia 0..1 para ordenar resultados entre orígenes.
	Score float64 `json:"score,omitempty"`
}

// SourceResult agrupa los resultados de un único origen, con su latencia y un
// posible error parcial (la búsqueda federada degrada con elegancia: un origen
// caído no rompe el resto).
type SourceResult struct {
	Source Source      `json:"source"`
	Hits   []SearchHit `json:"hits"`
	Took   int64       `json:"took_ms"`
	Error  string      `json:"error,omitempty"`
}

// HasError indica si el origen devolvió un error parcial.
func (r SourceResult) HasError() bool { return r.Error != "" }

// SearchResponse es la respuesta agregada de la búsqueda federada.
type SearchResponse struct {
	Query   string         `json:"query"`
	Hits    []SearchHit    `json:"hits"`
	Sources []SourceResult `json:"sources"`
	TookMS  int64          `json:"took_ms"`
	// Total es cuántos resultados hay en el agregado ANTES de recortar la página. Sin
	// él, la interfaz no puede decir «23 resultados» ni saber si ofrecer «ver más».
	Total int `json:"total"`
	// HasMore indica si quedan resultados más allá de esta página.
	HasMore bool `json:"has_more,omitempty"`
	// Interpreted es lo que se entendió de la frase cuando la búsqueda pidió
	// interpretarla (`ia=1`). Ausente = se buscó literal, que es lo de siempre.
	//
	// SE DEVUELVE PARA QUE SE PUEDA ENSEÑAR Y QUITAR. Una búsqueda que ha
	// filtrado por un rango de fechas que el usuario no ha visto devuelve cero
	// resultados sin explicar por qué, y quien busca concluye que no hay nada.
	Interpreted *Interpreted `json:"interpreted,omitempty"`
}

// Interpreted es el desglose de lo que la IA entendió de la consulta.
type Interpreted struct {
	// Text es el término literal que quedó tras sacar los filtros.
	Text    string     `json:"text"`
	Sources []Source   `json:"sources,omitempty"`
	Kinds   []string   `json:"kinds,omitempty"`
	From    *time.Time `json:"from,omitempty"`
	To      *time.Time `json:"to,omitempty"`
}

// ---------------------------------------------------------------------------
// Notificaciones unificadas
// ---------------------------------------------------------------------------

// NotificationPref silencia los avisos de un origen, o de un tipo concreto dentro de
// él. Se guardan solo las EXCEPCIONES: lo que el usuario no ha tocado llega. Así una
// app nueva no nace muda, que es lo que pasaría con una matriz de opt-in.
type NotificationPref struct {
	Source Source `json:"source"`
	// Kind es el tipo de aviso dentro del origen. Vacío = todo el origen.
	Kind  string `json:"kind,omitempty"`
	Muted bool   `json:"muted"`
}

// Muted decide si un aviso se entrega, aplicando la regla MÁS ESPECÍFICA: una que
// nombra origen+tipo gana sobre la que solo nombra el origen. Sin reglas, se entrega.
func Muted(prefs []NotificationPref, source Source, kind string) bool {
	deOrigen := false
	for _, p := range prefs {
		if p.Source != source {
			continue
		}
		// La regla exacta (origen+tipo) manda: se responde con ella sin mirar más.
		if kind != "" && p.Kind == kind {
			return p.Muted
		}
		if p.Kind == "" {
			deOrigen = p.Muted
		}
	}
	return deOrigen
}

// NotificationLevel categoriza la prioridad de una notificación.
type NotificationLevel string

const (
	LevelInfo    NotificationLevel = "info"
	LevelSuccess NotificationLevel = "success"
	LevelWarning NotificationLevel = "warning"
	LevelError   NotificationLevel = "error"
)

// ValidLevel valida un nivel de notificación.
func ValidLevel(l NotificationLevel) bool {
	switch l {
	case LevelInfo, LevelSuccess, LevelWarning, LevelError:
		return true
	default:
		return false
	}
}

// Notification es un aviso unificado entregado al usuario en el shell. Los
// servicios de la suite las emiten; Hub las almacena, agrega y reparte por SSE.
type Notification struct {
	ID      uuid.UUID         `json:"id"`
	Tenant  string            `json:"-"`
	Subject string            `json:"-"`
	Source  Source            `json:"source"`
	Type    string            `json:"type"`
	Level   NotificationLevel `json:"level"`
	Title   string            `json:"title"`
	Body    string            `json:"body,omitempty"`
	URL     string            `json:"url,omitempty"`
	// Params son las variables del título (quién, qué) para que cada cliente lo
	// componga en SU idioma con la clave `notifications.kind.<type>`. Title queda
	// como el texto de respaldo, en el idioma del emisor.
	Params    map[string]string `json:"params,omitempty"`
	Read      bool              `json:"read"`
	CreatedAt time.Time         `json:"created_at"`
	ReadAt    *time.Time        `json:"read_at,omitempty"`
	// Organization es la organización que lo mandó cuando NO es la de la sesión que
	// lo lee (ADR 0053): la campana lo dice, para que un aviso de fuera no pase por
	// uno de la propia organización. Vacía en los de la propia.
	Organization string `json:"organization,omitempty"`
	// OrganizationName es el nombre visible de Organization («Acme Ingeniería S.L.»),
	// si Sytue ID lo da; sin él, la campana enseña el identificador.
	OrganizationName string `json:"organization_name,omitempty"`
	// Historic es un aviso RECONSTRUIDO de algo que pasó antes de que existiera el
	// registro de avisos (`avisos-historicos`): la campana lo marca como tal.
	Historic bool `json:"historic,omitempty"`
}

// Antes es el cursor de la lista de avisos: los anteriores a (En, ID). Con la fecha sola,
// dos avisos del mismo instante en el corte de página se perdían (los reconstruidos
// traen instantes repetidos). ID vacío = todos los de antes de En.
type Antes struct {
	En time.Time
	ID uuid.UUID
}

// VistoDesde es el aviso tal y como lo ve una sesión de la organización `tenant`.
func (n Notification) VistoDesde(tenant string) Notification {
	if n.Tenant != "" && n.Tenant != tenant {
		n.Organization = n.Tenant
	} else {
		n.Organization = ""
	}
	return n
}

// Destinatarios son los buzones de avisos de una persona: su `sub` y sus direcciones
// (la de entrar, `email`, y la de la organización, `mailbox`), en la forma de
// `hubnotify.PorCorreo`. Vienen en el token que firma Sytue ID. Con TODOS se leen sus
// avisos de la organización del token; entre organizaciones, solo con los
// personales (BuzonesPersonales).
func Destinatarios(sub, email, mailbox string) []string {
	if strings.TrimSpace(sub) == "" {
		return nil // sin identidad no hay buzones
	}
	out := []string{sub}
	for _, e := range []string{email, mailbox} {
		if strings.TrimSpace(e) == "" {
			continue
		}
		if d := hubnotify.PorCorreo(e); !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}

// NewNotification son los datos para crear una notificación.
type NewNotification struct {
	Subject string
	Source  Source
	Type    string
	Level   NotificationLevel
	Title   string
	Body    string
	URL     string
	Params  map[string]string
}

// Topes de Params: son variables de un título, no un segundo cuerpo.
const (
	maxParams     = 8
	maxParamClave = 32
	maxParamRunas = 300
)

// Validate normaliza y valida los datos de una nueva notificación.
func (n *NewNotification) Validate() error {
	n.Subject = strings.TrimSpace(n.Subject)
	n.Title = strings.TrimSpace(n.Title)
	if n.Subject == "" || n.Title == "" {
		return ErrInvalidInput
	}
	if utf8.RuneCountInString(n.Title) > 200 || len(n.Body) > 4000 {
		return ErrInvalidInput
	}
	if !ValidNotificationSource(n.Source) {
		return ErrUnknownSource
	}
	if n.Type == "" || len(n.Type) > 100 {
		return ErrInvalidInput
	}
	// URL: allowlist de esquema http/https. El shell abre la notificación con
	// window.location.assign(n.url) en el origen de la Sytue Shell; sin esta verja,
	// un emisor con notifications:write (servicio interno / Console — confused
	// deputy) podría inyectar javascript:/data: (XSS almacenado) o un https ajeno
	// (open-redirect/phishing). Validar el esquema en el límite de confianza cierra
	// el sink para TODOS los emisores. rel=noopener + allowlist en el sink del shell
	// quedan como defensa en profundidad (follow-up de front).
	n.URL = strings.TrimSpace(n.URL)
	if n.URL != "" {
		if len(n.URL) > 2048 {
			return ErrInvalidInput
		}
		u, err := url.Parse(n.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return ErrInvalidInput
		}
	}
	if n.Level == "" {
		n.Level = LevelInfo
	}
	if !ValidLevel(n.Level) {
		return ErrInvalidInput
	}
	if len(n.Params) > maxParams {
		return ErrInvalidInput
	}
	for k, v := range n.Params {
		if k == "" || len(k) > maxParamClave || utf8.RuneCountInString(v) > maxParamRunas {
			return ErrInvalidInput
		}
	}
	return nil
}

// HistoricoHasta es el techo de los avisos reconstruidos: nada de lo que ocurra desde
// entonces puede entrar por la ruta histórica, aunque el registro de su tipo siga sin
// estrenar. El registro empezó a guardar avisos el 2026-09-25 (medido en producción);
// el día siguiente cierra el hueco con margen y deja la ruta sin uso para lo demás.
var HistoricoHasta = time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)

// HistoricoCierre es cuándo deja de existir la ruta histórica (410), por el reloj del
// Hub: hubnotify.CierreReconstruccion, la misma fecha que el directorio de Sytue ID.
var HistoricoCierre = hubnotify.CierreReconstruccion

// TopeHistoricosPorPersona es cuántos avisos reconstruidos puede tener una persona,
// sumando TODAS las organizaciones (el Hub no sabe cuáles existen, y por organización
// se rodearía inventando nombres): la ruta no tiene el tope por hora de la de siempre
// (sus fechas son viejas), y sin este se podría llenar la campana de cualquiera.
const TopeHistoricosPorPersona = 2000

// VentanaHistorica es hasta dónde atrás acepta el Hub un aviso reconstruido: la misma
// ventana que aplica el comando (hubnotify.VentanaHistorica), aquí para que no dependa
// de quien llama.
const VentanaHistorica = hubnotify.VentanaHistorica

// MargenHistorico se quita al primer aviso en vivo de cada tipo: ese aviso se guarda
// un poco DESPUÉS del elemento que lo causa, y sin margen ese elemento se reconstruiría
// y saldría dos veces.
const MargenHistorico = 10 * time.Minute

// Resultados de un aviso histórico, para el recuento del comando que los reconstruye.
const (
	HistoricoCreado     = "creado"
	HistoricoRepetido   = "repetido"   // ya estaba: la clave de origen es única
	HistoricoSilenciado = "silenciado" // su destinatario calla ese origen o tipo
	HistoricoPosterior  = "posterior"  // de cuando el registro ya funcionaba
	HistoricoOlvidado   = "olvidado"   // a una dirección que Sytue ID retiró (ADR 0053)
	HistoricoAntiguo    = "antiguo"    // más viejo que VentanaHistorica
	HistoricoTope       = "tope"       // su destinatario ya tiene TopeHistoricosPorPersona
)

// NewHistorico es un aviso reconstruido: el de siempre, con la fecha en que ocurrió
// y la clave del elemento del que sale.
type NewHistorico struct {
	NewNotification
	Clave string
	Fecha time.Time
}

// Validate valida el aviso y su clave y fecha. La fecha no se compara aquí con el
// registro en vivo: eso lo hace el servicio, que ve la base.
func (h *NewHistorico) Validate() error {
	if err := h.NewNotification.Validate(); err != nil {
		return err
	}
	h.Clave = strings.TrimSpace(h.Clave)
	if h.Clave == "" || len(h.Clave) > 200 || !utf8.ValidString(h.Clave) || h.Fecha.IsZero() {
		return ErrInvalidInput
	}
	return nil
}

// ---------------------------------------------------------------------------
// Presencia
// ---------------------------------------------------------------------------

// PresenceState es el estado de presencia de un usuario.
type PresenceState string

const (
	StateOnline  PresenceState = "online"
	StateAway    PresenceState = "away"
	StateBusy    PresenceState = "busy"
	StateOffline PresenceState = "offline"
)

// ValidState valida un estado de presencia declarable por el cliente.
func ValidState(s PresenceState) bool {
	switch s {
	case StateOnline, StateAway, StateBusy:
		return true
	default:
		return false
	}
}

// Presence es la presencia agregada de un usuario en la suite.
type Presence struct {
	Subject  string        `json:"subject"`
	State    PresenceState `json:"state"`
	Status   string        `json:"status,omitempty"`
	LastSeen time.Time     `json:"last_seen"`
}

// ---------------------------------------------------------------------------
// Intents cross-app
// ---------------------------------------------------------------------------

// IntentType identifica una acción cross-app que devuelve deep-links.
type IntentType string

const (
	// IntentAttachDriveToMail abre un borrador de Mail con ficheros de Drive
	// adjuntos (por referencia).
	IntentAttachDriveToMail IntentType = "attach-drive-to-mail"
	// IntentCreateEventFromMail abre el compositor de Calendar prerellenado
	// desde un correo.
	IntentCreateEventFromMail IntentType = "create-event-from-mail"
	// IntentShareToChat abre Chat con un recurso (fichero/evento/correo)
	// preparado para compartir en un canal.
	IntentShareToChat IntentType = "share-to-chat"
	// IntentCallChannel abre Sytue Rooms (servido en el host de Chat) e inicia la
	// videollamada del canal indicado (Chat+Meet unificados).
	IntentCallChannel IntentType = "call-channel"
)

// ValidIntent valida un tipo de intent soportado.
func ValidIntent(t IntentType) bool {
	switch t {
	case IntentAttachDriveToMail, IntentCreateEventFromMail, IntentShareToChat, IntentCallChannel:
		return true
	default:
		return false
	}
}

// IntentRequest es la petición de un intent cross-app. Los parámetros concretos
// se validan por tipo en el paquete intents.
type IntentRequest struct {
	Type   IntentType        `json:"type"`
	Params map[string]string `json:"params"`
	// IDs son identificadores múltiples (p. ej. varios ficheros a adjuntar).
	IDs []string `json:"ids,omitempty"`
}

// IntentResult es la respuesta de un intent: un deep-link primario y enlaces
// auxiliares opcionales, más etiquetas para que el shell renderice la acción.
type IntentResult struct {
	Type IntentType `json:"type"`
	// URL es el deep-link de cara al usuario que materializa el intent.
	URL string `json:"url"`
	// Target es la superficie destino (mail, calendar, chat, ...).
	Target Source `json:"target"`
	// Label es el texto sugerido para el botón/acción en el shell.
	Label string `json:"label"`
	// Links son deep-links adicionales relevantes (origen del recurso, etc.).
	Links []DeepLink `json:"links,omitempty"`
}

// DeepLink es un enlace nombrado a una superficie de la suite.
type DeepLink struct {
	Rel    string `json:"rel"`
	Source Source `json:"source"`
	URL    string `json:"url"`
}

// BuzonesPersonales son los buzones que identifican a la persona en TODA la suite, y
// con los que la campana enseña sus avisos de CUALQUIER organización (ADR 0053): el
// `sub`, que Sytue ID emite único, y sus direcciones principales en TODAS sus
// organizaciones activas (`mailbox` y `mailboxes`), que Sytue ID asigna en un dominio
// verificado, no se repiten entre organizaciones y deriva en cada emisión del token.
//
// El `email` de entrar NO está, ni verificado: `email_verified` lo pone a true el alta
// que hace una administración, SAML o una federación, sin que nadie haya probado que
// la dirección es suya. Sus avisos los sigue viendo en la organización que se los mandó.
func BuzonesPersonales(sub string, mailboxes ...string) []string {
	if strings.TrimSpace(sub) == "" {
		return nil
	}
	out := []string{sub}
	for _, m := range mailboxes {
		if strings.TrimSpace(m) != "" {
			if b := hubnotify.PorCorreo(m); !slices.Contains(out, b) {
				out = append(out, b)
			}
		}
	}
	return out
}

// Persona es lo que el handler saca del token para leer avisos de todas sus
// organizaciones: sus buzones personales y cuándo se emitió el token.
type Persona struct {
	Buzones []string
	Emitido time.Time
}

type claveBuzonesPersonales struct{}

// ConBuzonesPersonales deja en el contexto los buzones personales de quien llama y la
// hora de su token; el store los usa para `app.destinatarios`. Solo lo ponen los
// handlers, con el token.
func ConBuzonesPersonales(ctx context.Context, b []string, emitido time.Time) context.Context {
	return context.WithValue(ctx, claveBuzonesPersonales{}, Persona{Buzones: b, Emitido: emitido})
}

// BuzonesPersonalesDe es lo que puso ConBuzonesPersonales, o nada.
func BuzonesPersonalesDe(ctx context.Context) Persona {
	p, _ := ctx.Value(claveBuzonesPersonales{}).(Persona)
	return p
}

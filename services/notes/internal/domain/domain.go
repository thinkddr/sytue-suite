// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain define el modelo y las reglas de negocio de Sytue Notepads:
// cuadernos anidados, notas heterogéneas dentro de ellos, etiquetas con estado y
// enlaces a objetos de la suite. Independiente de transporte (HTTP) y de
// almacenamiento (PostgreSQL).
//
// Clean-room: modelo propio. La inspiración funcional —contenedores anidados de
// notas de distinto tipo, etiquetas con estado, filtro— son ideas y no tienen
// copyright; no se ha consultado ni portado código de terceros (CONVENTIONS §12).
package domain

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Errores de dominio. La capa HTTP los traduce a *httpx.Error.
var (
	ErrNotFound     = errors.New("notes: recurso no encontrado")
	ErrInvalidInput = errors.New("notes: entrada inválida")
	ErrForbidden    = errors.New("notes: no autorizado para esta acción")
	// ErrSoloDueno: mover, publicar, esconder o cifrar un cuaderno es de su dueño o de
	// quien administra la organización. Aparte de ErrForbidden porque quien lo recibe
	// SÍ ve el cuaderno: un 404 le diría que no existe.
	ErrSoloDueno = errors.New("notes: solo el dueño del cuaderno o quien administra la organización")
	// ErrEncrypted se devuelve cuando se intenta escribir texto en claro en un
	// cuaderno cifrado (o al revés). No es un fallo de permisos: es que el
	// contenido no puede viajar de esa forma.
	ErrEncrypted = errors.New("notes: el cuaderno está cifrado; el contenido debe llegar cifrado")
	// ErrFormatoNoReconocido: el fichero que se intenta leer no es un volcado de
	// Sytue Notepads. Se distingue de ErrVersionNoSoportada a propósito: «esto no
	// es lo que crees» y «esto es más nuevo que yo» piden cosas distintas a quien
	// lo intenta, y `.onex` es además la extensión del índice de un cuaderno de
	// OneNote, así que ese caso va a darse.
	ErrFormatoNoReconocido = errors.New("notes: el fichero no es un volcado de Sytue Notepads")
	// ErrVersionNoSoportada: es un volcado, pero de una versión que esta
	// instalación no sabe leer.
	ErrVersionNoSoportada = errors.New("notes: el volcado es de una versión más nueva")
)

// ---------------------------------------------------------------------------
// Visibilidad
// ---------------------------------------------------------------------------

// Visibility es quién puede ver un cuaderno o una nota. Es la única perilla de
// privacidad del producto: de ella salen tanto el permiso de lectura como el
// valor POR DEFECTO de la indexación.
type Visibility string

const (
	// VisPrivate — solo su dueño, y NUNCA en el índice de búsqueda.
	VisPrivate Visibility = "private"
	// VisPersonal — solo su dueño, pero buscable por él.
	VisPersonal Visibility = "personal"
	// VisOrganization — cualquier miembro de la organización activa.
	VisOrganization Visibility = "organization"
	// VisPublic — cualquiera con el enlace, sin sesión y en solo lectura.
	VisPublic Visibility = "public"
)

// ValidVisibility indica si v es una visibilidad conocida.
func ValidVisibility(v Visibility) bool {
	switch v {
	case VisPrivate, VisPersonal, VisOrganization, VisPublic:
		return true
	default:
		return false
	}
}

// DefaultIndexable es si una visibilidad entra en el índice mientras nadie diga
// lo contrario. Privada = no, el resto = sí. Es la configuración más protectora
// que sigue siendo útil (RGPD art. 25, CONVENTIONS §0.7).
func DefaultIndexable(v Visibility) bool { return v != VisPrivate }

// ---------------------------------------------------------------------------
// Cuadernos
// ---------------------------------------------------------------------------

// Layout es la disposición del lienzo de un cuaderno.
type Layout string

const (
	// LayoutFree coloca cada nota en las coordenadas donde se soltó (mapa mental).
	// LayoutTree es la disposición POR DEFECTO: el árbol de notas a la izquierda y
	// la nota elegida a la derecha, como Mail con sus hilos y como el BasKet
	// clásico. Las notas se anidan por `group_id`; las carpetas son los cuadernos.
	LayoutTree Layout = "tree"
	LayoutFree Layout = "free"
	// LayoutColumns reparte las notas en columnas verticales.
	LayoutColumns Layout = "columns"
	// LayoutList apila las notas en una sola columna.
	LayoutList Layout = "list"
)

// ValidLayout indica si l es una disposición conocida.
func ValidLayout(l Layout) bool {
	switch l {
	case LayoutTree, LayoutFree, LayoutColumns, LayoutList:
		return true
	default:
		return false
	}
}

// CryptoParams son los parámetros de derivación de clave de un cuaderno cifrado.
// El servidor los guarda y los sirve, pero NO participa en el cifrado: la clave se
// deriva en el navegador a partir de la contraseña, que nunca sale de él.
type CryptoParams struct {
	// V versiona el esquema (hoy 1).
	V int `json:"v"`
	// KDF es la función de derivación (hoy "PBKDF2-SHA256", la que trae WebCrypto).
	KDF string `json:"kdf"`
	// Salt es la sal en base64. Iterations, el coste.
	Salt       string `json:"salt"`
	Iterations int    `json:"iterations"`
	// Check es un criptograma de verificación: permite decirle a quien escribe
	// «esa contraseña no es» sin descifrar ninguna nota ni guardar la clave.
	Check string `json:"check"`
}

// MinKDFIterations es el coste mínimo aceptado para PBKDF2-SHA256. Por debajo, la
// contraseña se rompe por fuerza bruta con el criptograma en la mano, así que un
// cliente que proponga menos se rechaza en vez de guardar cifrado de adorno.
const MinKDFIterations = 210_000

// Validate comprueba los parámetros de derivación que propone el cliente.
func (c CryptoParams) Validate() error {
	if c.V != 1 || c.KDF != "PBKDF2-SHA256" {
		return ErrInvalidInput
	}
	if c.Iterations < MinKDFIterations || c.Iterations > 10_000_000 {
		return ErrInvalidInput
	}
	if l := len(c.Salt); l < 16 || l > 256 {
		return ErrInvalidInput
	}
	if l := len(c.Check); l == 0 || l > 1024 {
		return ErrInvalidInput
	}
	return nil
}

// Notebook es un contenedor de notas. Los cuadernos anidan (ParentID) y forman el
// árbol de la barra lateral.
type Notebook struct {
	// DailyDate identifica el día civil del diario, independientemente de su título.
	DailyDate string `json:"daily_date,omitempty"`
	// ArchivedAt saca el cuaderno del árbol y de la búsqueda sin borrarlo (0015).
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	ID         uuid.UUID  `json:"id"`
	Tenant     string     `json:"-"`
	OwnerSub   string     `json:"owner_sub"`
	ParentID   *uuid.UUID `json:"parent_id,omitempty"`
	Title      string     `json:"title"`
	Emoji      string     `json:"emoji,omitempty"`
	Color      string     `json:"color"`
	Layout     Layout     `json:"layout"`
	Visibility Visibility `json:"visibility"`
	Indexable  bool       `json:"indexable"`
	Encrypted  bool       `json:"encrypted"`
	// Crypto solo viaja con Encrypted; sin él el navegador no puede derivar la clave.
	Crypto *CryptoParams `json:"crypto,omitempty"`
	// RoomID vincula el cuaderno a una sala de Sytue Rooms (lienzo de la sala).
	RoomID string `json:"room_id,omitempty"`
	// PublicToken es el secreto del enlace público. Solo se sirve a su DUEÑO: para
	// el resto es una credencial de lectura y no tiene por qué verla.
	PublicToken string    `json:"public_token,omitempty"`
	Position    int       `json:"position"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// NoteCount lo rellena el listado del árbol (no se persiste).
	NoteCount int `json:"note_count"`
	// ReadOnly y EditRequested los rellena el listado del árbol: quien pregunta lo lee
	// pero no escribe sus notas (un cuaderno público de otra persona) y, si es así, si
	// ya ha pedido hacerlo (la franja de solo lectura). En negativo a propósito: un
	// cliente que no conozca el campo sigue tratándolo como escribible, como antes.
	ReadOnly      bool `json:"read_only,omitempty"`
	EditRequested bool `json:"edit_requested,omitempty"`
}

// maxTitle acota el título de un cuaderno o nota (en runas, no en bytes: un
// título en cirílico no puede valer la mitad que uno en latín).
const (
	maxNotebookTitle = 200
	maxNoteTitle     = 400
	maxContent       = 200_000
	maxCipher        = MaxEncryptedNoteCipher
	maxEmoji         = 16
)

// NotebookInput son los campos que el cliente puede fijar de un cuaderno.
type NotebookInput struct {
	ParentID   *uuid.UUID    `json:"parent_id"`
	Title      string        `json:"title"`
	Emoji      string        `json:"emoji"`
	Color      string        `json:"color"`
	Layout     Layout        `json:"layout"`
	Visibility Visibility    `json:"visibility"`
	Indexable  *bool         `json:"indexable"`
	Encrypted  bool          `json:"encrypted"`
	Crypto     *CryptoParams `json:"crypto"`
	RoomID     string        `json:"room_id"`
	Position   *int          `json:"position"`
}

// Normalize recorta, completa los valores por defecto y valida. Devuelve el input
// ya saneado: lo que salga de aquí es lo único que llega al store.
func (in *NotebookInput) Normalize() error {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > maxNotebookTitle {
		return ErrInvalidInput
	}
	in.Emoji = strings.TrimSpace(in.Emoji)
	if utf8.RuneCountInString(in.Emoji) > maxEmoji {
		return ErrInvalidInput
	}
	if in.Color == "" {
		in.Color = "#86198f"
	}
	if !ValidHexColor(in.Color) {
		return ErrInvalidInput
	}
	if in.Layout == "" {
		in.Layout = LayoutTree
	}
	if !ValidLayout(in.Layout) {
		return ErrInvalidInput
	}
	if in.Visibility == "" {
		in.Visibility = VisPersonal
	}
	if !ValidVisibility(in.Visibility) {
		return ErrInvalidInput
	}
	in.RoomID = strings.TrimSpace(in.RoomID)
	if len(in.RoomID) > 200 {
		return ErrInvalidInput
	}
	if in.Encrypted {
		if in.Crypto == nil {
			return ErrInvalidInput
		}
		if err := in.Crypto.Validate(); err != nil {
			return err
		}
		// Cifrado y en el índice a la vez no existe: el índice guarda el texto en
		// claro, que es justo lo que el cifrado impide que tengamos.
		no := false
		in.Indexable = &no
	} else {
		in.Crypto = nil
	}
	if in.Indexable == nil {
		d := DefaultIndexable(in.Visibility)
		in.Indexable = &d
	}
	return nil
}

// ---------------------------------------------------------------------------
// Notas
// ---------------------------------------------------------------------------

// Kind es el tipo de una nota: decide cómo se pinta y qué hay en Data.
type Kind string

const (
	// KindText es una nota de texto enriquecido (el tipo por defecto).
	KindText Kind = "text"
	// KindTodo es una nota con casilla: tarea que se marca hecha.
	KindTodo Kind = "todo"
	// KindImage es una imagen (subida a Drive; Data lleva su URL y tamaño).
	KindImage Kind = "image"
	// KindFile es un fichero adjunto.
	KindFile Kind = "file"
	// KindLink es un enlace externo con su vista previa.
	KindLink Kind = "link"
	// KindColor es una muestra de color (paleta).
	KindColor Kind = "color"
	// KindLauncher es un acceso directo a una acción/URL de la suite.
	KindLauncher Kind = "launcher"
	// KindCrossref es una referencia a otro cuaderno o nota de Sytue Notepads.
	KindCrossref Kind = "crossref"
	// KindDoc incrusta un documento de Sytue Docs con su visor.
	KindDoc Kind = "doc"
	// KindDrive enlaza un fichero de Sytue Drive elegido con el selector.
	KindDrive Kind = "drive"
	// KindEvent enlaza un evento de Sytue Calendar (referencia bidireccional).
	KindEvent Kind = "event"
	// KindRoom enlaza una sala o un mensaje de Sytue Rooms.
	KindRoom Kind = "room"
)

// ValidKind indica si k es un tipo de nota conocido.
func ValidKind(k Kind) bool {
	switch k {
	case KindText, KindTodo, KindImage, KindFile, KindLink, KindColor,
		KindLauncher, KindCrossref, KindDoc, KindDrive, KindEvent, KindRoom:
		return true
	default:
		return false
	}
}

// LinkKindFor traduce el tipo de nota al tipo de destino de note_links, y dice si
// esa nota debe materializar un enlace consultable en sentido inverso. Es lo que
// hace que un evento sepa qué notas cuelgan de él sin recorrer ningún jsonb.
func LinkKindFor(k Kind) (string, bool) {
	switch k {
	case KindEvent:
		return TargetCalendarEvent, true
	case KindDoc:
		return TargetDocsDocument, true
	case KindDrive:
		return TargetDriveFile, true
	case KindRoom:
		return TargetRoomChannel, true
	case KindCrossref:
		return TargetNote, true
	default:
		return "", false
	}
}

// Tipos de destino de un enlace (columna target_kind).
const (
	TargetCalendarEvent = "calendar_event"
	TargetRoomChannel   = "room_channel"
	TargetRoomMessage   = "room_message"
	TargetDocsDocument  = "docs_document"
	TargetDriveFile     = "drive_file"
	TargetNotebook      = "notebook"
	TargetNote          = "note"
)

// ValidTargetKind indica si k es un destino de enlace conocido.
func ValidTargetKind(k string) bool {
	switch k {
	case TargetCalendarEvent, TargetRoomChannel, TargetRoomMessage,
		TargetDocsDocument, TargetDriveFile, TargetNotebook, TargetNote:
		return true
	default:
		return false
	}
}

// Note es una nota dentro de un cuaderno.
type Note struct {
	ID         uuid.UUID `json:"id"`
	Tenant     string    `json:"-"`
	NotebookID uuid.UUID `json:"notebook_id"`
	OwnerSub   string    `json:"owner_sub"`
	Kind       Kind      `json:"kind"`
	Title      string    `json:"title,omitempty"`
	// Content es HTML ya saneado (allowlist), sea cual sea el tipo. Vacío en una nota
	// cifrada. Lo guardado antes de sanear todos los tipos puede no estarlo: quien lo
	// vuelque como HTML lo sanea al salir (cuerpoHTMLDeNota, cuerpoDeNota).
	Content string `json:"content,omitempty"`
	// ContentCipher es el criptograma que produjo el navegador ("encv1:<b64>").
	ContentCipher string          `json:"content_cipher,omitempty"`
	Data          json.RawMessage `json:"data,omitempty"`
	// DataJSON is a lossless read representation used only by bounded note pages.
	DataJSON  string     `json:"data_json,omitempty"`
	X         int        `json:"x"`
	Y         int        `json:"y"`
	Col       int        `json:"col"`
	Position  int        `json:"position"`
	Width     int        `json:"width"`
	GroupID   *uuid.UUID `json:"group_id,omitempty"`
	Collapsed bool       `json:"collapsed"`
	Checked   bool       `json:"checked"`
	Pinned    bool       `json:"pinned"`
	// CheckedAt es cuándo se completó; CancelledAt, cuándo se canceló (tachada y fuera
	// del traslado). Nunca las dos a la vez.
	CheckedAt   *time.Time `json:"checked_at,omitempty"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`
	// Visibility/Indexable vacíos = se heredan del cuaderno.
	Visibility Visibility  `json:"visibility,omitempty"`
	Indexable  *bool       `json:"indexable,omitempty"`
	States     []NoteState `json:"states,omitempty"`
	Links      []NoteLink  `json:"links,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
	// RepeatedInto solo viaja en la respuesta de editar una tarea: la siguiente que
	// creó al marcarla hecha, o la que retiró al desmarcarla. Con ella, el cliente
	// vuelve a pedir las notas y los recordatorios, sean suyos o de otra persona.
	RepeatedInto *uuid.UUID `json:"repeated_into,omitempty"`
}

// NoteState es un estado de etiqueta marcado en una nota (con lo necesario para
// pintarlo sin una segunda consulta).
type NoteState struct {
	TagID   uuid.UUID `json:"tag_id"`
	TagName string    `json:"tag_name"`
	StateID uuid.UUID `json:"state_id"`
	Name    string    `json:"name"`
	Emoji   string    `json:"emoji,omitempty"`
	Color   string    `json:"color"`
}

// NoteLink es un enlace de una nota a un objeto de la suite.
type NoteLink struct {
	TargetKind string `json:"target_kind"`
	TargetID   string `json:"target_id"`
	Label      string `json:"label,omitempty"`
	URL        string `json:"url,omitempty"`
}

// MaxClientID es el largo máximo de `data.client_id`: un UUID cabe con holgura.
const MaxClientID = 64

// NoteInput son los campos que el cliente puede fijar de una nota.
type NoteInput struct {
	// CipherVersion is a write capability, never persisted or returned as note data.
	CipherVersion  int             `json:"cipher_version,omitempty"`
	ExpectedCrypto json.RawMessage `json:"expected_crypto,omitempty"`
	Kind           Kind            `json:"kind"`
	Title          string          `json:"title"`
	Content        string          `json:"content"`
	ContentCipher  string          `json:"content_cipher"`
	Data           json.RawMessage `json:"data"`
	X              *int            `json:"x"`
	Y              *int            `json:"y"`
	Col            *int            `json:"col"`
	Position       *int            `json:"position"`
	Width          *int            `json:"width"`
	GroupID        *uuid.UUID      `json:"group_id"`
	Collapsed      *bool           `json:"collapsed"`
	Checked        *bool           `json:"checked"`
	// Cancelled cancela (true) o reactiva (false) una tarea; nil la deja como está.
	Cancelled  *bool      `json:"cancelled"`
	Pinned     *bool      `json:"pinned"`
	Visibility Visibility `json:"visibility"`
	Indexable  *bool      `json:"indexable"`
	Links      []NoteLink `json:"links"`
}

// hechaOCancelada hace cumplir que hecha y cancelada se excluyen: marcar una quita la
// otra, y pedir las dos es un error.
func (in *NoteInput) hechaOCancelada() error {
	si := func(b *bool) bool { return b != nil && *b }
	switch {
	case si(in.Checked) && si(in.Cancelled):
		return ErrInvalidInput
	case si(in.Checked):
		in.Cancelled = new(bool)
	case si(in.Cancelled):
		in.Checked = new(bool)
	}
	return nil
}

// ClientID es el `data.client_id` de la nota, o "" si no lo trae. Normalize ya ha
// comprobado que, si está, es una cadena de hasta MaxClientID bytes.
func (in *NoteInput) ClientID() string {
	var d struct {
		ClientID string `json:"client_id"`
	}
	_ = json.Unmarshal(in.Data, &d)
	return d.ClientID
}

// validaData comprueba los campos de `data` que el servicio interpreta.
func validaData(data json.RawMessage) error {
	// `data.url` ES UN HREF. Va a `<a href>` —y a `<img src>`— en el navegador de
	// quien vea la nota, que en un cuaderno de organización o publicado es otra
	// persona, y hasta ahora solo se validaba el de `links[]`. Un `javascript:`
	// aquí era un clic ajeno con la sesión de quien lo daba. Misma verja que los
	// enlaces: http/https con host. Un `data` que no sea objeto no tiene `url`.
	var d struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(data, &d) == nil && d.URL != "" && !ValidWebURL(d.URL) {
		return ErrInvalidInput
	}
	// `data.client_id` es la marca con que un cliente reintenta una captura hecha sin
	// conexión (ver Store.CreateNote). Se lee aparte del `url`: con los dos en el mismo
	// struct, un `client_id` numérico hacía fallar el Unmarshal entero y se saltaba la
	// verja del `url`.
	var c struct {
		ClientID any `json:"client_id"`
	}
	if json.Unmarshal(data, &c) == nil && c.ClientID != nil {
		if s, ok := c.ClientID.(string); !ok || s == "" || len(s) > MaxClientID {
			return ErrInvalidInput
		}
	}
	return nil
}

// Normalize sanea y valida la nota que llega del cliente contra el cuaderno que la
// aloja. `encrypted` decide qué forma puede tener el contenido: en un cuaderno
// cifrado el texto en claro se RECHAZA, no se guarda «por si acaso».
//
// El HTML de una nota, del tipo que sea, se sanea aquí (allowlist), no en el navegador: el
// contenido de un cuaderno de organización lo pinta el navegador de OTRA persona,
// así que el saneado tiene que estar en el lado que no se puede saltar.
func (in *NoteInput) Normalize(encrypted bool) error {
	if in.Kind == "" {
		in.Kind = KindText
	}
	if !ValidKind(in.Kind) {
		return ErrInvalidInput
	}
	if err := in.hechaOCancelada(); err != nil {
		return err
	}
	if err := in.normalizaContenido(encrypted); err != nil {
		return err
	}
	if len(in.Data) == 0 || (encrypted && string(in.Data) == "null") {
		in.Data = json.RawMessage(`{}`)
	}
	// Adjuntos o datos de la nota: hasta 5 MB por nota para adjuntos guardados solo en la nota.
	if len(in.Data) > 5*1024*1024 || !json.Valid(in.Data) {
		return ErrInvalidInput
	}
	if err := validaData(in.Data); err != nil {
		return err
	}
	if in.Width != nil && (*in.Width < 80 || *in.Width > 1200) {
		return ErrInvalidInput
	}
	if in.Visibility != "" && !ValidVisibility(in.Visibility) {
		return ErrInvalidInput
	}
	// Una nota cifrada no puede pedir estar en el índice, venga de donde venga.
	if encrypted {
		if in.CipherVersion != EncryptedNoteWriteVersion {
			return ErrEncryptedClientUpgrade
		}
		if !EncryptedPublicData(in.Data, false) {
			return ErrInvalidInput
		}
		no := false
		in.Indexable = &no
	}
	return in.normalizaEnlaces()
}

// normalizaContenido deja el cuerpo de la nota como toca según venga cifrada o
// en claro. Sale de `Normalize` porque son las dos ramas más largas y su tamaño
// escondía el resto de comprobaciones, que son de una línea cada una.
func (in *NoteInput) normalizaContenido(encrypted bool) error {
	in.ContentCipher = strings.TrimSpace(in.ContentCipher)
	if encrypted {
		if in.ContentCipher == "" || !strings.HasPrefix(in.ContentCipher, "encv1:") {
			return ErrEncrypted
		}
		if len(in.ContentCipher) > maxCipher {
			return ErrInvalidInput
		}
		// En claro no queda NADA: ni el título, que suele ser lo más revelador.
		in.Title = ""
		in.Content = ""
		return nil
	}
	if in.ContentCipher != "" {
		return ErrInvalidInput
	}
	in.Title = strings.TrimSpace(in.Title)
	if utf8.RuneCountInString(in.Title) > maxNoteTitle {
		return ErrInvalidInput
	}
	if len(in.Content) > maxContent {
		return ErrInvalidInput
	}
	// TODOS los tipos, no solo texto y tarea: las exportaciones, la búsqueda y los
	// avisos tratan `Content` como HTML sea cual sea el tipo, y el tipo se puede cambiar
	// después. Con una sola regla no hay tipo por el que colar HTML sin sanear.
	in.Content = SanitizeHTML(in.Content)
	return nil
}

// normalizaEnlaces recorta y valida los enlaces salientes de la nota.
func (in *NoteInput) normalizaEnlaces() error {
	for i := range in.Links {
		l := &in.Links[i]
		l.TargetID = strings.TrimSpace(l.TargetID)
		l.Label = strings.TrimSpace(l.Label)
		l.URL = strings.TrimSpace(l.URL)
		if !ValidTargetKind(l.TargetKind) || l.TargetID == "" || len(l.TargetID) > 400 {
			return ErrInvalidInput
		}
		if utf8.RuneCountInString(l.Label) > 400 {
			return ErrInvalidInput
		}
		if l.URL != "" && !ValidWebURL(l.URL) {
			return ErrInvalidInput
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Etiquetas con estado
// ---------------------------------------------------------------------------

// Tag es una etiqueta: un eje ("Prioridad") con estados excluyentes.
type Tag struct {
	ID       uuid.UUID  `json:"id"`
	Tenant   string     `json:"-"`
	OwnerSub string     `json:"-"`
	Name     string     `json:"name"`
	Position int        `json:"position"`
	States   []TagState `json:"states"`
}

// TagState es uno de los valores de una etiqueta.
type TagState struct {
	ID       uuid.UUID `json:"id"`
	TagID    uuid.UUID `json:"tag_id"`
	Name     string    `json:"name"`
	Emoji    string    `json:"emoji,omitempty"`
	Color    string    `json:"color"`
	Shortcut string    `json:"shortcut,omitempty"`
	Position int       `json:"position"`
}

// TagInput son los campos que el cliente fija de una etiqueta y sus estados.
type TagInput struct {
	Name     string          `json:"name"`
	Position *int            `json:"position"`
	States   []TagStateInput `json:"states"`
}

// TagStateInput es un estado propuesto por el cliente. ID vacío = estado nuevo.
type TagStateInput struct {
	ID       *uuid.UUID `json:"id"`
	Name     string     `json:"name"`
	Emoji    string     `json:"emoji"`
	Color    string     `json:"color"`
	Shortcut string     `json:"shortcut"`
}

// Normalize sanea y valida una etiqueta con sus estados.
func (in *TagInput) Normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 60 {
		return ErrInvalidInput
	}
	if len(in.States) == 0 || len(in.States) > 20 {
		return ErrInvalidInput
	}
	for i := range in.States {
		s := &in.States[i]
		s.Name = strings.TrimSpace(s.Name)
		s.Emoji = strings.TrimSpace(s.Emoji)
		s.Shortcut = strings.TrimSpace(s.Shortcut)
		if s.Name == "" || utf8.RuneCountInString(s.Name) > 60 {
			return ErrInvalidInput
		}
		if utf8.RuneCountInString(s.Emoji) > maxEmoji {
			return ErrInvalidInput
		}
		if utf8.RuneCountInString(s.Shortcut) > 1 {
			return ErrInvalidInput
		}
		if s.Color == "" {
			s.Color = "#565b62"
		}
		if !ValidHexColor(s.Color) {
			return ErrInvalidInput
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Validadores compartidos
// ---------------------------------------------------------------------------

// ValidHexColor acepta exactamente #RRGGBB. Sin nombres ni funciones CSS: el
// color acaba en un atributo `style`, y ahí la lista corta es la defensa.
func ValidHexColor(v string) bool {
	if len(v) != 7 || v[0] != '#' {
		return false
	}
	for _, c := range v[1:] {
		if !isHex(c) {
			return false
		}
	}
	return true
}

func isHex(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// ValidWebURL acepta http/https con host. Es la misma verja que aplica el Hub a
// las URL de notificación: sin ella, `javascript:`/`data:` en un enlace de una
// nota compartida sería XSS almacenado en el navegador de quien la abra.
func ValidWebURL(raw string) bool {
	if raw == "" || len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// --- Auditoría -------------------------------------------------------------

// Acciones que se registran en la traza. Son las que NO se pueden deshacer y las
// que amplían quién puede ver algo; escribir una nota no entra, porque una traza
// que lo registra todo no la lee nadie.
const (
	// AuditNotebookPublish — el cuaderno pasa a `public`: token nuevo y lectura
	// SIN SESIÓN para cualquiera con el enlace. Es la única superficie del
	// servicio que sale de la organización.
	AuditNotebookPublish = "notebook.publish"
	// AuditNotebookUnpublish — deja de estarlo. Sin este par, un cuaderno que
	// estuvo publicado no se distingue de uno que nunca lo estuvo, y la ventana
	// de exposición no se puede reconstruir.
	AuditNotebookUnpublish = "notebook.unpublish"
	// AuditNotebookDelete — a la papelera con toda su rama.
	AuditNotebookDelete = "notebook.delete"
	// AuditNotePurge — borrado definitivo. La papelera tiene vuelta atrás; esto no.
	AuditNotePurge = "note.purge"
	// AuditNotebookExport — alguien se llevó un cuaderno entero fuera del sistema.
	// Se anota por el mismo motivo que publicar: es contenido que SALE, y en un
	// cuaderno de organización quien exporta puede no ser quien lo escribió. Es
	// además la acción que una fuga por la puerta de delante deja detrás.
	AuditNotebookExport = "notebook.export"
	// AuditNotebookImport — entró un cuaderno entero desde un fichero. Es la
	// vuelta de exportar y se anota por la razón simétrica: contenido que ENTRA
	// sin pasar por la pantalla, y en cantidad. Un cuaderno de mil notas que
	// aparece de golpe en una organización tiene que poder explicarse.
	AuditNotebookImport = "notebook.import"
	// AuditLegalHoldCreate — se preserva el contenido de alguien (o de toda la
	// organización) frente al recolector. Es acceso privilegiado de un admin sobre
	// datos ajenos, así que se anota SIEMPRE, aunque no cambie ni una nota.
	AuditLegalHoldCreate = "legalhold.create"
	// AuditLegalHoldRelease — se levanta. Importa tanto o más que ponerlo: es el
	// instante a partir del cual el contenido vuelve a ser destruible, y si algo
	// se perdió después habrá que poder decir quién abrió esa puerta y cuándo.
	AuditLegalHoldRelease = "legalhold.release"
)

// LegalHold es una retención legal: mientras esté activo (ReleasedAt nulo), el
// recolector de papelera NO destruye el contenido que cubre.
//
// Mismas columnas que el `legal_holds` de Drive y Mail. Ver la migración 0006
// para la semántica: la preservación es DINÁMICA —se evalúa en el punto de
// borrado, no sella cada nota— y `Subject` vacío significa TODA la organización.
type LegalHold struct {
	ID         uuid.UUID  `json:"id"`
	Tenant     string     `json:"-"`
	Subject    string     `json:"subject,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	CaseRef    string     `json:"case_ref,omitempty"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	ReleasedBy string     `json:"released_by,omitempty"`
}

// Activo dice si el hold sigue preservando. Se pregunta así y no comparando
// contra nil por ahí suelto: es la definición de «activo» y vive en un sitio.
func (h LegalHold) Activo() bool { return h.ReleasedAt == nil }

// AuditEntry es una línea de la traza. Mismas columnas que el `audit_log` de
// Drive, Mail y services/id: la Console lee los cuatro con un solo contrato.
type AuditEntry struct {
	ID         int64           `json:"id"`
	OccurredAt time.Time       `json:"occurred_at"`
	Tenant     string          `json:"-"`
	Actor      string          `json:"actor"`
	Action     string          `json:"action"`
	Target     string          `json:"target,omitempty"`
	Detail     json.RawMessage `json:"detail,omitempty"`
}

// Los dos niveles con que se comparte un cuaderno o una nota con una persona (0010).
const (
	PermisoLeer   = "read"
	PermisoEditar = "write"
)

// Persona es alguien con quien su dueño ha compartido un cuaderno o una nota: su `sub`,
// si puede editarlo y quién y cuándo se lo dio. El correo y el nombre no se guardan
// aquí —cambian—; los pone quien pinta la lista, con el directorio.
type Persona struct {
	Sujeto       string    `json:"subject"`
	Permiso      string    `json:"permission"`
	ConcedidoPor string    `json:"granted_by"`
	Creado       time.Time `json:"created_at"`
}

// NotaCompartida es una nota de «Compartido conmigo»: la nota, el nivel con que se la
// dieron a quien la lee y quién.
type NotaCompartida struct {
	Note
	Permiso      string `json:"permission"`
	ConcedidoPor string `json:"shared_by"`
}

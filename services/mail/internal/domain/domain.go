// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain define los tipos y reglas de negocio de Sytue Mail,
// independientes de transporte (SMTP/IMAP/JMAP) y de almacenamiento (PG/S3).
//
// Clean-room: modelo de datos propio sobre los estándares abiertos del correo
// (RFC 5321 SMTP, RFC 5322 IMF, RFC 3501/9051 IMAP, RFC 8620/8621 JMAP,
// RFC 7208 SPF, RFC 6376 DKIM, RFC 7489 DMARC). No se porta código de terceros
// AGPL; se implementa desde las especificaciones y librerías permisivas (MIT).
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Errores de dominio. La capa de transporte los traduce a su forma (httpx.Error
// en JMAP, código SMTP en MX/submission).
var (
	ErrNotFound        = errors.New("mail: recurso no encontrado")
	ErrMailboxExists   = errors.New("mail: el buzón ya existe")
	ErrLabelExists     = errors.New("mail: la etiqueta ya existe")
	ErrMailboxNotEmpty = errors.New("mail: el buzón no está vacío")
	ErrInvalidAddress  = errors.New("mail: dirección de correo inválida")
	ErrMessageTooLarge = errors.New("mail: mensaje demasiado grande")
	// ErrFromNoAutorizado: la cabecera `From:` del mensaje que se quiere enviar no
	// es una dirección (principal o alias) de la cuenta que lo envía, o no hay
	// exactamente una.
	ErrFromNoAutorizado = errors.New("mail: el From de la cabecera no es una dirección de la cuenta que envía")
	ErrTooManyRcpts     = errors.New("mail: demasiados destinatarios")
	ErrRelayDenied      = errors.New("mail: relay no permitido")
	ErrAuthRequired     = errors.New("mail: autenticación requerida")
	ErrNoSuchUser       = errors.New("mail: destinatario local desconocido")
	ErrQuotaExceeded    = errors.New("mail: cuota de buzón superada")
	ErrSpecialMailbox   = errors.New("mail: buzón de sistema no modificable")
	ErrInvalidFilter    = errors.New("mail: regla de filtrado inválida")
	// ErrScheduleGone: un envío diferido (programado/deshacer) ya no es
	// cancelable/editable porque el worker lo liberó, ya se envió o no es del
	// usuario. Cancelar/editar tras la ventana devuelve esto (409), no un 404,
	// para no confundir «ya salió» con «nunca existió».
	ErrScheduleGone = errors.New("mail: envío diferido ya liberado o inexistente")
)

// Account es una cuenta de correo, vinculada 1:1 a una identidad de Sytue ID
// (sub) dentro de un tenant. La dirección principal es address; pueden existir
// alias adicionales (tabla aliases).
type Account struct {
	ID          uuid.UUID `json:"id"`
	Tenant      string    `json:"-"`
	Subject     string    `json:"-"` // sub de Sytue ID (clave de autorización JMAP)
	Address     string    `json:"address"`
	DisplayName string    `json:"display_name,omitempty"`
	QuotaBytes  int64     `json:"quota_bytes"`
	UsedBytes   int64     `json:"used_bytes"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SpecialUse identifica el rol de un buzón (RFC 6154 SPECIAL-USE / JMAP role).
type SpecialUse string

const (
	RoleInbox   SpecialUse = "inbox"
	RoleSent    SpecialUse = "sent"
	RoleDrafts  SpecialUse = "drafts"
	RoleTrash   SpecialUse = "trash"
	RoleJunk    SpecialUse = "junk"
	RoleArchive SpecialUse = "archive"
	RoleNone    SpecialUse = ""
)

// Mailbox es una carpeta de correo (folder IMAP / Mailbox JMAP).
type Mailbox struct {
	ID        uuid.UUID  `json:"id"`
	Tenant    string     `json:"-"`
	AccountID uuid.UUID  `json:"-"`
	ParentID  *uuid.UUID `json:"parent_id,omitempty"`
	Name      string     `json:"name"`
	Role      SpecialUse `json:"role,omitempty"`
	// UIDValidity y UIDNext implementan la semántica IMAP de UIDs estables.
	UIDValidity uint32    `json:"-"`
	UIDNext     uint32    `json:"-"`
	TotalCount  int64     `json:"total_emails"`
	UnreadCount int64     `json:"unread_emails"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Flags estándar de mensaje (IMAP \Seen, \Flagged, ...; keywords JMAP).
const (
	FlagSeen     = "\\Seen"
	FlagFlagged  = "\\Flagged"
	FlagDraft    = "\\Draft"
	FlagAnswered = "\\Answered"
	FlagDeleted  = "\\Deleted"
)

// Message es el sobre/metadato de un correo. El cuerpo MIME crudo y los
// adjuntos viven en S3 bajo BlobKey; aquí solo cabeceras indexables y estado.
type Message struct {
	ID        uuid.UUID `json:"id"`
	Tenant    string    `json:"-"`
	AccountID uuid.UUID `json:"-"`
	MailboxID uuid.UUID `json:"mailbox_id"`
	// UID es el identificador IMAP estable dentro del buzón (asignado al insertar).
	UID uint32 `json:"-"`
	// ThreadID agrupa mensajes de una conversación (RFC 5322 References/In-Reply-To).
	ThreadID uuid.UUID `json:"thread_id"`
	// ThreadCount es cuántos mensajes vivos tiene ESE hilo en toda la cuenta, no
	// solo en esta carpeta: es lo que abre el lector al pulsar la fila. Lo rellena
	// el listado de bandeja (agregado en la misma consulta, sin N+1); el resto de
	// lecturas lo dejan a cero y por eso es omitempty.
	ThreadCount int `json:"thread_count,omitempty"`
	// MessageID es el Message-ID RFC 5322 (único, para dedupe y threading).
	MessageID string `json:"message_id"`
	InReplyTo string `json:"in_reply_to,omitempty"`

	Subject  string   `json:"subject"`
	FromAddr string   `json:"from"`
	FromName string   `json:"from_name,omitempty"`
	To       []string `json:"to"`
	Cc       []string `json:"cc,omitempty"`
	Bcc      []string `json:"bcc,omitempty"`
	ReplyTo  []string `json:"reply_to,omitempty"`

	// Snippet es un extracto de texto plano para vistas de lista (sin abrir S3).
	Snippet string `json:"preview,omitempty"`
	// BlobKey es la clave S3 del .eml crudo (relativa al prefijo de tenant).
	BlobKey   string   `json:"-"`
	SizeBytes int64    `json:"size"`
	Flags     []string `json:"keywords,omitempty"`
	HasAttach bool     `json:"has_attachment"`

	// Resultados de autenticación del remitente (RFC 8601), para correo entrante.
	SPFResult   AuthResult `json:"-"`
	DKIMResult  AuthResult `json:"-"`
	DMARCResult AuthResult `json:"-"`

	ReceivedAt time.Time `json:"received_at"`
	SentAt     time.Time `json:"date"`
}

// HasFlag indica si el mensaje tiene un flag/keyword.
func (m Message) HasFlag(flag string) bool {
	for _, f := range m.Flags {
		if strings.EqualFold(f, flag) {
			return true
		}
	}
	return false
}

// AuthResult es el veredicto de un mecanismo de autenticación de remitente.
type AuthResult string

const (
	AuthNone      AuthResult = ""
	AuthPass      AuthResult = "pass"
	AuthFail      AuthResult = "fail"
	AuthSoftFail  AuthResult = "softfail"
	AuthNeutral   AuthResult = "neutral"
	AuthTempError AuthResult = "temperror"
	AuthPermError AuthResult = "permerror"
)

// OutboundState es el estado de un mensaje en la cola de salida.
type OutboundState string

const (
	OutQueued    OutboundState = "queued"
	OutSending   OutboundState = "sending"
	OutSent      OutboundState = "sent"
	OutDeferred  OutboundState = "deferred"
	OutBounced   OutboundState = "bounced"
	OutCancelled OutboundState = "cancelled"
)

// Hold marca una fila de la cola como «retenida» (submission diferida que el
// worker libera al vencer next_retry_at) en vez de una entrega inmediata.
const (
	HoldNone      = ""          // entrega inmediata / reintento (comportamiento clásico)
	HoldScheduled = "scheduled" // programado por el usuario (vista «Programados»)
	HoldUndo      = "undo"      // ventana de deshacer (transitorio; no se lista)
)

// OutboundMessage es una unidad de la cola de salida (sobre PostgreSQL; sin
// Redis/Valkey). Un mensaje genera un OutboundMessage por destinatario-dominio
// para reintento independiente.
type OutboundMessage struct {
	ID         uuid.UUID     `json:"id"`
	Tenant     string        `json:"-"`
	AccountID  uuid.UUID     `json:"-"`
	MessageID  uuid.UUID     `json:"message_id"`
	From       string        `json:"from"`       // envelope MAIL FROM (return-path)
	Recipients []string      `json:"recipients"` // envelope RCPT TO de este lote
	BlobKey    string        `json:"-"`          // .eml firmado en S3
	State      OutboundState `json:"state"`
	Attempts   int           `json:"attempts"`
	LastError  string        `json:"last_error,omitempty"`
	// NextRetryAt es también el instante de LIBERACIÓN de un diferido: mientras
	// Hold != "", es la hora a la que el worker publicará el mensaje.
	NextRetryAt time.Time `json:"next_retry_at"`
	// Hold != "" marca una submission diferida (programado/deshacer); Subject se
	// desnormaliza para pintar la lista de programados sin bajar el .eml.
	Hold      string    `json:"hold,omitempty"`
	Subject   string    `json:"subject,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Address es una dirección de correo parseada (local@domain).
type Address struct {
	Name   string
	Local  string
	Domain string
}

// String reconstruye la dirección sin display name.
func (a Address) String() string { return a.Local + "@" + a.Domain }

// ParseAddress parsea y normaliza una dirección de correo de forma estricta.
// Clean-room sobre RFC 5321 §4.1.2 (forma del addr-spec). No acepta display
// names ni rutas de origen; para eso usar el parser de cabeceras (go-message).
func ParseAddress(s string) (Address, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	at := strings.LastIndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return Address{}, ErrInvalidAddress
	}
	local, domain := s[:at], strings.ToLower(s[at+1:])
	if len(local) > 64 || len(domain) > 255 {
		return Address{}, ErrInvalidAddress
	}
	if strings.ContainsAny(local, " \t\r\n") || strings.ContainsAny(domain, " \t\r\n/") {
		return Address{}, ErrInvalidAddress
	}
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return Address{}, ErrInvalidAddress
	}
	return Address{Local: local, Domain: domain}, nil
}

// ValidMailboxName valida el nombre de un buzón (no vacío, sin control ni
// separador de jerarquía interno; la jerarquía se modela con ParentID).
func ValidMailboxName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 {
		return false
	}
	if strings.ContainsAny(name, "/\x00") {
		return false
	}
	for _, r := range name {
		if r < 0x20 {
			return false
		}
	}
	return true
}

// Label es una etiqueta de usuario (estilo Gmail). Se modela como KEYWORD
// JMAP/IMAP: el token que ocupa en messages.flags es su id en texto (uuid), de
// modo que un mensaje lleva N etiquetas (multi-pertenencia natural del array de
// flags) y renombrar/recolorear NO reescribe los mensajes. Esta struct solo
// aporta los metadatos que un keyword desnudo no tiene: nombre, color y dueño.
type Label struct {
	ID        uuid.UUID `json:"id"`
	Tenant    string    `json:"-"`
	AccountID uuid.UUID `json:"-"`
	Name      string    `json:"name"`
	Color     string    `json:"color,omitempty"`
	// Contadores para el badge del sidebar (los rellena ListLabels con un JOIN).
	TotalCount  int64     `json:"total_emails"`
	UnreadCount int64     `json:"unread_emails"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Keyword es el token que la etiqueta ocupa en messages.flags (keyword JMAP /
// flag IMAP): su id en texto. Estable (rename/recolor no lo cambian) y válido
// como atom IMAP (solo [0-9a-f-]) y keyword JMAP.
func (l Label) Keyword() string { return l.ID.String() }

// LabelColors es la paleta sobria de etiquetas (Graphite & Brass): TOKENS, no CSS
// crudo, para alinear con la identidad, evitar inyección de estilo y que el color
// no sea el único indicador (el nombre siempre acompaña). Vacío = sin color.
var LabelColors = []string{"slate", "brass", "sage", "clay", "blue", "plum", "teal", "rose"}

// ValidLabelColor acepta el vacío (neutro) o un token de la paleta.
func ValidLabelColor(c string) bool {
	if c == "" {
		return true
	}
	for _, v := range LabelColors {
		if v == c {
			return true
		}
	}
	return false
}

// ValidLabelName valida el nombre de una etiqueta: no vacío, ≤128, sin control ni
// NUL. Admite '/' para permitir jerarquía por convención de nombre (estilo Gmail
// «Trabajo/Proyectos»), que el cliente puede pintar anidada; el backend las trata
// planas (el modelo de keyword no anida).
func ValidLabelName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// DefaultMailboxes es el conjunto de buzones de sistema creados al provisionar
// una cuenta. El orden importa (Inbox primero).
var DefaultMailboxes = []struct {
	Name string
	Role SpecialUse
}{
	{"Inbox", RoleInbox},
	{"Sent", RoleSent},
	{"Drafts", RoleDrafts},
	{"Trash", RoleTrash},
	{"Junk", RoleJunk},
	{"Archive", RoleArchive},
}

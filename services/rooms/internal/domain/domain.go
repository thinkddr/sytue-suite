// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain define los tipos y reglas de negocio de Sytue Rooms,
// independientes de transporte (HTTP/WebSocket) y de almacenamiento (PostgreSQL).
//
// Modelo: un tenant tiene equipos (teams); cada team tiene canales (channels)
// públicos o privados, con miembros (members). En cada canal se publican mensajes
// (messages), que pueden iniciar hilos (threads) mediante parent_id, llevar
// reacciones (reactions), menciones a usuarios y adjuntos de Sytue Drive.
//
// Clean-room: modelo y protocolo propios sobre WebSocket (RFC 6455) y REST; sin
// código de terceros AGPL (Mattermost queda fuera, CONVENTIONS §0.2).
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Errores de dominio. La capa HTTP los traduce a *httpx.Error.
var (
	ErrNotFound     = errors.New("chat: recurso no encontrado")
	ErrConflict     = errors.New("chat: conflicto (nombre o estado)")
	ErrInvalidName  = errors.New("chat: nombre inválido")
	ErrInvalidInput = errors.New("chat: entrada inválida")
	ErrForbidden    = errors.New("chat: no eres miembro o tu rol no lo permite")
	ErrNotMember    = errors.New("chat: no eres miembro del recurso")
	ErrEmptyMessage = errors.New("chat: el mensaje no puede estar vacío")
	// ErrTooManyLookups: Sytue ID frenó las búsquedas de personas de quien llama
	// (su límite por persona). Se devuelve como 429, no como «no existe».
	ErrTooManyLookups = errors.New("chat: demasiadas búsquedas de personas seguidas")
	// ErrLastAdmin: el último admin de un canal privado con más miembros no puede
	// irse sin nombrar antes a otro. Envuelve ErrConflict (409).
	ErrLastAdmin = fmt.Errorf("%w: último admin del canal", ErrConflict)
)

// ChannelKind es la visibilidad de un canal.
type ChannelKind string

const (
	// ChannelPublic es visible para cualquier miembro del team.
	ChannelPublic ChannelKind = "public"
	// ChannelPrivate solo es visible para sus miembros.
	ChannelPrivate ChannelKind = "private"
	// ChannelDirect es una conversación directa (DM) entre dos o más usuarios.
	ChannelDirect ChannelKind = "direct"
)

// ValidChannelKind valida un tipo de canal.
func ValidChannelKind(k ChannelKind) bool {
	return k == ChannelPublic || k == ChannelPrivate || k == ChannelDirect
}

// MemberRole es el rol de un usuario dentro de un team o canal.
type MemberRole string

const (
	// RoleAdmin gestiona el recurso (renombrar, archivar, gestionar miembros).
	RoleAdmin MemberRole = "admin"
	// RoleMember participa con permisos normales.
	RoleMember MemberRole = "member"
)

// ValidMemberRole valida un rol de pertenencia.
func ValidMemberRole(r MemberRole) bool { return r == RoleAdmin || r == RoleMember }

// Tipos de mensaje. El vacío se normaliza a MessageTypeUser (mensaje de usuario).
const (
	// MessageTypeUser es un mensaje normal escrito por un usuario.
	MessageTypeUser = "user"
	// MessageTypeCall es una tarjeta de sistema que anota el inicio de una
	// videollamada de Sytue Rooms en el timeline del canal (Ola 6). No lleva
	// autoría de texto: el cuerpo puede ir vacío y el cliente pinta la tarjeta.
	MessageTypeCall = "call"
)

// ValidMessageType valida el tipo de un mensaje. El vacío es válido (se normaliza
// a "user" al persistir).
func ValidMessageType(t string) bool {
	return t == "" || t == MessageTypeUser || t == MessageTypeCall
}

// Prioridad de un mensaje (Ola 12). El vacío se normaliza a PriorityNormal. El
// cliente pinta un badge de acento con TEXTO (nunca solo color, WCAG AA).
const (
	// PriorityNormal es un mensaje sin realce (por defecto).
	PriorityNormal = "normal"
	// PriorityImportant marca el mensaje como importante.
	PriorityImportant = "important"
	// PriorityUrgent marca el mensaje como urgente.
	PriorityUrgent = "urgent"
)

// ValidPriority valida la prioridad de un mensaje. El vacío es válido (se
// normaliza a "normal" al persistir).
func ValidPriority(p string) bool {
	return p == "" || p == PriorityNormal || p == PriorityImportant || p == PriorityUrgent
}

// Nivel de notificación por canal y usuario (Ola 13). notify_level es la fuente de
// verdad de la política de aviso; el atenuado del sidebar (Channel.Muted) se deriva
// como NotifyLevelNone. El vacío se normaliza a NotifyLevelAll.
const (
	// NotifyLevelAll avisa de todos los mensajes del canal (por defecto).
	NotifyLevelAll = "all"
	// NotifyLevelMentions avisa solo de @menciones (a ti o al canal) y palabras clave.
	NotifyLevelMentions = "mentions"
	// NotifyLevelNone no avisa; además atenúa el canal en el sidebar (Muted derivado).
	NotifyLevelNone = "none"
)

// ValidNotifyLevel valida el nivel de notificación de un canal. El vacío es válido
// (se normaliza a "all" al persistir).
func ValidNotifyLevel(l string) bool {
	return l == "" || l == NotifyLevelAll || l == NotifyLevelMentions || l == NotifyLevelNone
}

// CanManage indica si el rol puede administrar el recurso.
func (r MemberRole) CanManage() bool { return r == RoleAdmin }

// Team es un espacio de trabajo de mensajería dentro de un tenant.
type Team struct {
	ID          uuid.UUID `json:"id"`
	Tenant      string    `json:"-"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedBy   string    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Role es el rol de QUIEN PREGUNTA en este team ('admin' o 'member'), no una
	// propiedad del team. Solo lo rellena ListTeams, que ya une con team_members para
	// filtrar; las demás consultas lo dejan vacío y por eso es omitempty. Dice quién
	// administra ESE team (sus canales y miembros). NO quién administra el espacio:
	// eso es el rol owner/admin de la organización en Sytue ID
	// (store.esAdminDeLaOrganizacion).
	Role string `json:"role,omitempty"`
}

// Channel es un canal de mensajería dentro de un team.
type Channel struct {
	ID     uuid.UUID `json:"id"`
	Tenant string    `json:"-"`
	TeamID uuid.UUID `json:"team_id"`
	Slug   string    `json:"slug"`
	Name   string    `json:"name"`
	Topic  string    `json:"topic,omitempty"`
	// Purpose es «de qué va este canal»: el texto que se lee al EXPLORAR canales,
	// distinto del tema (que es el estado del día bajo el nombre). Slack y Mattermost
	// los mantienen separados por esa razón.
	Purpose     string      `json:"purpose,omitempty"`
	Kind        ChannelKind `json:"kind"`
	Archived    bool        `json:"archived"`
	CreatedBy   string      `json:"created_by,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	LastMessage *time.Time  `json:"last_message_at,omitempty"`
	// Favorite y Muted son preferencias per-usuario (de la propia membresía),
	// pobladas solo al listar los canales del usuario (ListChannels); en lecturas
	// genéricas quedan en false. Muted se DERIVA de NotifyLevel (Muted ⟺ NotifyLevel
	// == "none"): es el atenuado visual del sidebar, no una fuente de verdad aparte.
	Favorite bool `json:"favorite,omitempty"`
	Muted    bool `json:"muted,omitempty"`
	// NotifyLevel es la política de aviso per-usuario del canal: "all" (por defecto),
	// "mentions" (solo @menciones/palabras clave) o "none" (nada). Fuente de verdad de
	// la notificación; se puebla solo al listar los canales del usuario (ListChannels).
	NotifyLevel string `json:"notify_level,omitempty"`
	// Peers son los nombres a mostrar de los OTROS miembros de un DM (directo),
	// resueltos per-visor y poblados solo por ListChannels. Permite al cliente
	// distinguir un DM de grupo (varios) de un 1:1 y componer la cabecera. Vacío en
	// el resto de canales.
	Peers []string `json:"peers,omitempty"`
	// PeerSubs son los SUBJECTS de esos mismos OTROS miembros del DM, ALINEADOS índice
	// a índice con Peers (misma ORDER BY en el agregado de ListChannels). El cliente los
	// usa como clave estable de presencia y de acuses de lectura, en vez de casar por
	// nombre (frágil ante homónimos). Poblado solo por ListChannels; vacío en el resto.
	PeerSubs []string `json:"peer_subs,omitempty"`
	// Invitados son las personas de OTRA organización a las que abrir este DM acaba de
	// invitar (mig 0041). Solo lo rellena OpenGroupDirect al CREAR la conversación: es a
	// quien se avisa en la campana (COM-15). No sale en el JSON.
	Invitados []string `json:"-"`
	// CategoryID es la categoría personalizada (per-usuario) del sidebar a la que el
	// usuario asignó el canal; nil = categoría por defecto según kind (Favoritos si
	// favorite, DMs si direct, si no Canales). SortOrder es su orden dentro de la
	// categoría. Ambos se pueblan solo al listar los canales del usuario (Ola 11).
	CategoryID *uuid.UUID `json:"category_id,omitempty"`
	SortOrder  int        `json:"sort_order,omitempty"`
	// External marca, en la lista de quien la ve, una conversación que vive en OTRA
	// organización y en la que esa persona está como externa (ADR 0040). Solo lo
	// pone ListExternalChannels; el cliente la pinta aparte.
	External bool `json:"external,omitempty"`
}

// Category es un grupo personalizado y colapsable de canales en el sidebar de un
// usuario (Ola 11, estilo Mattermost/Slack). Es estado PER-USUARIO (subject): agrupa y
// ordena la barra lateral de ese usuario sin afectar a los demás. Los canales se asignan
// a una categoría en su fila de pertenencia (channel_members.category_id); NULL cae a la
// categoría por defecto según kind.
type Category struct {
	ID        uuid.UUID `json:"id"`
	Tenant    string    `json:"-"`
	Subject   string    `json:"-"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	Collapsed bool      `json:"collapsed"`
	CreatedAt time.Time `json:"created_at"`
}

// Member es la pertenencia de un usuario a un team o canal.
type Member struct {
	Subject  string     `json:"subject"`
	Name     string     `json:"name,omitempty"`
	Role     MemberRole `json:"role"`
	JoinedAt time.Time  `json:"joined_at"`
	// External: es de otra organización (ADR 0040). Solo en el roster de un canal.
	External bool `json:"external,omitempty"`
	// Pending: es una invitación, aún sin aceptar (mig 0041): no es miembro todavía.
	Pending bool `json:"pending,omitempty"`
}

// Invitacion es la de una persona de OTRA organización a un canal, un DM o un grupo
// (mig 0041): hasta que la acepta no es miembro de nada. Lo único que enseña del canal
// es su nombre y su clase, y quién invita.
type Invitacion struct {
	Organizacion string `json:"organization"`
	// NombreOrganizacion es el nombre visible de Organizacion, si Sytue ID lo da.
	NombreOrganizacion  string      `json:"organization_name,omitempty"`
	ChannelID           uuid.UUID   `json:"channel_id"`
	Nombre              string      `json:"name"`
	Clase               ChannelKind `json:"kind"`
	InvitadoPor         string      `json:"invited_by"`
	NombreDeQuienInvita string      `json:"invited_by_name"`
	Creada              time.Time   `json:"created_at"`
}

// Attachment referencia un fichero de Sytue Drive adjuntado a un mensaje.
type Attachment struct {
	// FileID es el id del fichero en Sytue Drive.
	FileID uuid.UUID `json:"file_id"`
	// Name y ContentType se denormalizan para mostrar sin llamar a Drive.
	Name        string `json:"name,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
}

// Message es un mensaje publicado en un canal. Si ParentID no es nil, es una
// respuesta dentro de un hilo cuya raíz es ParentID.
type Message struct {
	ID          uuid.UUID       `json:"id"`
	Tenant      string          `json:"-"`
	ChannelID   uuid.UUID       `json:"channel_id"`
	ParentID    *uuid.UUID      `json:"parent_id,omitempty"`
	AuthorSub   string          `json:"author_sub"`
	AuthorName  string          `json:"author_name,omitempty"`
	Body        string          `json:"body"`
	MatrixEvent *MatrixEventRef `json:"matrix_event,omitempty"`
	Encryption  string          `json:"encryption"`
	// Type distingue un mensaje normal ("user") de una tarjeta de sistema como la
	// de una llamada iniciada ("call"). Vacío se trata como "user".
	Type        string       `json:"type,omitempty"`
	Mentions    []string     `json:"mentions,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Reactions   []Reaction   `json:"reactions,omitempty"`
	// ReplyCount es el número de respuestas en el hilo (solo en mensajes raíz).
	ReplyCount int        `json:"reply_count,omitempty"`
	EditedAt   *time.Time `json:"edited_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	// Priority es "normal" (por defecto), "important" o "urgente". Vacío = "normal".
	Priority string `json:"priority,omitempty"`
	// RequiresAck marca que el autor pide acuse de recibo explícito a los lectores.
	RequiresAck bool `json:"requires_ack,omitempty"`
	// AckCount es el nº de lectores que han confirmado (agregado, sin N+1); Acked
	// indica si el VISOR de esta lectura ya confirmó. Solo relevantes si RequiresAck.
	AckCount int  `json:"ack_count,omitempty"`
	Acked    bool `json:"acked,omitempty"`
	// IsBot marca los mensajes publicados por una integración (webhook entrante,
	// respuesta de webhook saliente o bot). Se DERIVA del prefijo del author_sub
	// (hook:/bot:) al escanear —sin columna ni migración— y alimenta el badge «BOT»
	// del frontend. Ver domain.IsBotAuthor / Ola 15.
	IsBot bool `json:"is_bot,omitempty"`
	// Card es la tarjeta estructurada del mensaje (encuesta o tarea); nil en un
	// mensaje normal. Su definición se persiste en messages.card y su estado
	// (Card.Votes) se agrega al leer desde message_card_votes, sin N+1. Ver cards.go.
	Card *Card `json:"card,omitempty"`
	// MentionsAll marca que el mensaje menciona a TODO el canal (@channel/@here/@all).
	// Persistido en messages.mentions_all (mig. 0003). Se SERIALIZA en el JSON (y viaja
	// en el frame WS) para que el cliente suba el badge de @menciones al instante al
	// llegar el mensaje, sin esperar al recálculo de UnreadByChannel.
	MentionsAll bool `json:"mentions_all,omitempty"`
}

// ScheduledMessage es un mensaje redactado para publicarse en un instante futuro
// (SendAt). Un worker lo publica al vencer y borra la fila (Ola 12). ParentID no
// nil = respuesta en hilo programada.
type ScheduledMessage struct {
	ID         uuid.UUID   `json:"id"`
	Tenant     string      `json:"-"`
	ChannelID  uuid.UUID   `json:"channel_id"`
	ParentID   *uuid.UUID  `json:"parent_id,omitempty"`
	AuthorSub  string      `json:"author_sub"`
	AuthorName string      `json:"author_name,omitempty"`
	Body       string      `json:"body"`
	FileIDs    []uuid.UUID `json:"file_ids,omitempty"`
	SendAt     time.Time   `json:"send_at"`
	CreatedAt  time.Time   `json:"created_at"`
}

// Draft es un borrador de redacción del usuario: el texto que escribió y no envió
// en el composer de un canal (ParentID nil) o de un hilo (ParentID = raíz). Es estado
// PER-USUARIO y se sincroniza entre dispositivos: lo redactado en el portátil sigue
// ahí en el móvil. El cliente conserva además una copia en localStorage para
// restaurarlo sin esperar a la red; esta es la fuente de verdad.
type Draft struct {
	ChannelID uuid.UUID  `json:"channel_id"`
	ParentID  *uuid.UUID `json:"parent_id,omitempty"`
	Body      string     `json:"body"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Reaction agrega los usuarios que reaccionaron con un emoji a un mensaje.
type Reaction struct {
	Emoji string   `json:"emoji"`
	Count int      `json:"count"`
	Subs  []string `json:"subs,omitempty"`
}

// ThreadSummary resume un hilo SEGUIDO para la vista global «Hilos» (Ola 9): el
// mensaje raíz (su ReplyCount es el total de respuestas), la última actividad del
// hilo (created_at de la última respuesta) y el número de respuestas que el usuario
// aún no ha leído (created_at > su last_viewed_at, de otros autores).
type ThreadSummary struct {
	Root          Message   `json:"root"`
	LastActivity  time.Time `json:"last_activity"`
	UnreadReplies int       `json:"unread_replies"`
}

// ReadState marca hasta dónde ha leído un usuario un canal.
type ReadState struct {
	ChannelID     uuid.UUID  `json:"channel_id"`
	Subject       string     `json:"-"`
	LastReadAt    time.Time  `json:"last_read_at"`
	LastMessageID *uuid.UUID `json:"last_read_message_id,omitempty"`
	Unread        int        `json:"unread"`
	// Mentions es el subconjunto de Unread que menciona a este usuario (@sub):
	// la señal de "te han mencionado aquí" que el shell resalta aparte del badge
	// de no leídos. Reusa el índice GIN messages_mentions_gin.
	Mentions int `json:"mentions"`
}

// Receipt es el acuse de lectura de OTRO miembro de un DM (Ola 16 · «Visto»):
// hasta cuándo ha leído el canal (LastReadAt, derivado de read_state; nil si nunca
// lo abrió). El cliente marca «Visto» en TU último mensaje cuando LastReadAt >= su
// created_at. Solo se calcula para DMs (Mattermost no hace receipts en canales
// grandes, para no pagar el coste). Name es el nombre a mostrar del miembro.
type Receipt struct {
	Subject    string     `json:"subject"`
	Name       string     `json:"name,omitempty"`
	LastReadAt *time.Time `json:"last_read_at,omitempty"`
}

// CustomEmoji es un emoji personalizado del tenant (Ola 14): liga un :nombre:
// (shortcode) a una imagen que vive en Sytue Drive (FileID). ContentType se
// denormaliza para filtrar por tipo sin volver a llamar a Drive; la imagen se
// sirve por el proxy de adjuntos (nosniff). El catálogo es acotado por tenant.
type CustomEmoji struct {
	Name        string    `json:"name"`
	FileID      uuid.UUID `json:"file_id"`
	ContentType string    `json:"content_type,omitempty"`
	CreatedBy   string    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// emojiNameRE valida un shortcode de emoji personalizado: minúsculas/dígitos/_/-,
// 1..64 (mismo invariante que el CHECK de la migración 0013).
var emojiNameRE = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// ValidEmojiName valida el nombre (shortcode) de un emoji personalizado.
func ValidEmojiName(name string) bool { return emojiNameRE.MatchString(name) }

// NotifyPrefs son las preferencias de notificación PER-USUARIO (Ola 13): palabras
// clave que disparan un aviso además de las @menciones, y la ventana «No molestar»
// (DND) en minutos del día [0..1439]. DNDStart==DNDEnd = DND desactivado; DNDStart >
// DNDEnd cruza medianoche. El cliente decide el aviso: compara la hora LOCAL con la
// ventana y busca las palabras clave (guardadas ya en minúsculas) en el mensaje.
type NotifyPrefs struct {
	Keywords []string `json:"keywords"`
	DNDStart int      `json:"dnd_start"`
	DNDEnd   int      `json:"dnd_end"`
}

// Topes de las preferencias de notificación (evitan abuso a escala).
const (
	maxNotifyKeywords   = 50
	maxNotifyKeywordLen = 64
	minutesPerDay       = 1440
)

// NormalizeNotifyPrefs sanea las preferencias antes de persistir: recorta y deduplica
// las palabras clave (sin vacías, en minúsculas, acotadas en número y longitud) y ciñe
// la ventana DND a [0..1439]. Pura y testeable.
func NormalizeNotifyPrefs(p NotifyPrefs) NotifyPrefs {
	seen := make(map[string]struct{}, len(p.Keywords))
	out := make([]string, 0, len(p.Keywords))
	for _, k := range p.Keywords {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" || utf8.RuneCountInString(k) > maxNotifyKeywordLen {
			continue
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
		if len(out) >= maxNotifyKeywords {
			break
		}
	}
	return NotifyPrefs{Keywords: out, DNDStart: clampMinute(p.DNDStart), DNDEnd: clampMinute(p.DNDEnd)}
}

// clampMinute ciñe un minuto del día al rango válido [0..1439].
func clampMinute(m int) int {
	if m < 0 {
		return 0
	}
	if m >= minutesPerDay {
		return minutesPerDay - 1
	}
	return m
}

// ---------------------------------------------------------------------------
// Validación
// ---------------------------------------------------------------------------

// ValidName valida un nombre legible (team/canal): no vacío, sin control, <=100.
func ValidName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return false
	}
	for _, r := range name {
		if r < 0x20 {
			return false
		}
	}
	return true
}

// slugChars define los caracteres válidos de un slug (minúsculas, dígitos, '-').
func validSlugRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
}

// ValidSlug valida un slug de team/canal (1..64, minúsculas/dígitos/guiones).
func ValidSlug(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	if s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for _, r := range s {
		if !validSlugRune(r) {
			return false
		}
	}
	return true
}

// Slugify deriva un slug válido a partir de un nombre legible.
func Slugify(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case validSlugRune(r) && r != '-':
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '_' || r == '-' || r == '.':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 64 {
		out = strings.Trim(out[:64], "-")
	}
	return out
}

// ValidEmoji valida un identificador de reacción (shortcode :smile: o emoji
// unicode corto). Se acota el tamaño para evitar abuso.
func ValidEmoji(e string) bool {
	e = strings.TrimSpace(e)
	if e == "" || len(e) > 64 {
		return false
	}
	for _, r := range e {
		if r < 0x20 {
			return false
		}
	}
	return true
}

// channelMentionRE detecta los tokens de mención a todo el canal (@channel,
// @here, @all) como palabra: precedidos de inicio o de un carácter no-palabra
// (para no disparar con un correo como foo@channel.com) y seguidos de fin o de
// otro no-palabra (para no disparar con @allison o @channels).
var channelMentionRE = regexp.MustCompile(`(^|[^\w@])@(channel|here|all)($|\W)`)

// HasChannelMention indica si el cuerpo menciona a todo el canal (@channel /
// @all) o a los conectados (@here). Es la señal para marcar el mensaje como
// "menciona a todo el canal" (columna messages.mentions_all) y que el contador
// de menciones de cada usuario lo cuente por consulta indexada, en lugar de
// insertar una mención por miembro: a escala (cientos de miles) eso evita un
// fan-out de N filas por mensaje.
//
// ponytail: @here == @channel hasta que exista presencia; filtrar por online
// cuando aterrice la ola de presencia.
func HasChannelMention(body string) bool {
	return channelMentionRE.MatchString(body)
}

// NormalizeBody recorta el cuerpo de un mensaje y valida su longitud en runas.
func NormalizeBody(body string, maxBytes int) (string, error) {
	body = strings.TrimRight(body, " \t\r\n")
	if strings.TrimSpace(body) == "" {
		return "", ErrEmptyMessage
	}
	if len(body) > maxBytes {
		return "", ErrInvalidInput
	}
	return body, nil
}

// ---------------------------------------------------------------------------
// Denuncias y bloqueos (ADR 0073)
// ---------------------------------------------------------------------------

// ErrBlocked: entre las dos personas de un mensaje directo hay un bloqueo. El
// error es EL MISMO lo haya puesto quien escribe o la otra persona: quién bloqueó
// a quién no se le dice a nadie.
var ErrBlocked = errors.New("chat: hay un bloqueo entre las dos personas")

// Motivos de una denuncia: lista corta y CERRADA (la misma en el CHECK de la
// tabla). El texto libre va aparte, en la nota.
var reportReasons = map[string]bool{"harassment": true, "sexual": true, "violence": true, "spam": true, "other": true}

// ValidReportReason dice si el motivo es uno de los de la lista.
func ValidReportReason(r string) bool { return reportReasons[r] }

// ReportNoteMaxRunes acota la nota de quien denuncia.
const ReportNoteMaxRunes = 500

// Estados de una denuncia. Los tres últimos son, a la vez, la acción que la cerró.
const (
	ReportOpen           = "open"
	ReportDismissed      = "dismissed"
	ReportMessageDeleted = "message_deleted"
	ReportMemberRemoved  = "member_removed"
)

// Report es una denuncia vista por quien modera. NO lleva quién denunció: para
// actuar no hace falta, y es lo que impide que le llegue al denunciado.
type Report struct {
	ID          uuid.UUID   `json:"id"`
	ChannelID   uuid.UUID   `json:"channel_id"`
	ChannelName string      `json:"channel_name"`
	ChannelKind ChannelKind `json:"channel_kind"`
	TargetSub   string      `json:"target_sub"`
	TargetName  string      `json:"target_name"`
	MessageID   *uuid.UUID  `json:"message_id,omitempty"`
	// MessageBody es el texto del mensaje denunciado LEÍDO EN VIVO: vacío si la
	// denuncia es a la persona o si el mensaje ya se borró.
	MessageBody string     `json:"message_body,omitempty"`
	MessageAt   *time.Time `json:"message_at,omitempty"`
	Reason      string     `json:"reason"`
	Note        string     `json:"note,omitempty"`
	Status      string     `json:"status"`
	ResolvedBy  string     `json:"resolved_by,omitempty"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	// ResolvedFrom dice desde qué cola se resolvió: "org" o "platform" (el
	// operador de la plataforma). Vacío en las resueltas antes de la 0039.
	ResolvedFrom string    `json:"resolved_from,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	// Tenant es la organización que aloja la conversación. Solo la rellena la
	// cola del operador, que es de todas.
	Tenant string `json:"tenant,omitempty"`
}

// Block es una persona bloqueada, para la lista de ajustes de quien bloquea.
type Block struct {
	Subject   string    `json:"subject"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain define los tipos y reglas de negocio de Sytue Rooms,
// independientes de transporte (HTTP), de almacenamiento (PostgreSQL) y del SFU
// de medios (LiveKit). Una sala de Sytue Rooms orquesta una room de LiveKit: el
// estado canónico (quién puede entrar, historial, grabaciones) vive en
// PostgreSQL; los medios WebRTC los conmuta LiveKit.
//
// Clean-room: modelo propio sobre estándares abiertos (WebRTC, JWT — RFC 7519);
// LiveKit (Apache-2.0) es la única app de terceros, integrada por su API/JWT
// públicos, sin portar su código.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Errores de dominio. La capa HTTP los traduce a *httpx.Error.
var (
	ErrNotFound    = errors.New("meet: recurso no encontrado")
	ErrConflict    = errors.New("meet: conflicto con el estado actual")
	ErrInvalidName = errors.New("meet: nombre o parámetro inválido")
	ErrRoomClosed  = errors.New("meet: la sala está cerrada")
	// ErrRoomLocked: la sala está cerrada a nuevas entradas. Es distinto de
	// ErrRoomClosed —la reunión sigue viva, solo que con la puerta echada— y por
	// eso lleva su propio error: la interfaz tiene que poder decir «el anfitrión
	// ha bloqueado la sala» y no «la reunión ha terminado», que es mentira.
	ErrRoomLocked = errors.New("meet: la sala está bloqueada")
	// ErrRoomOpen: se llamó a la puerta de una sala que NO está cerrada. Es un
	// error y no un no-op silencioso: quien lo recibe puede entrar directamente, y
	// tratarlo como «apuntado, espera» dejaría a alguien aguardando a que le abran
	// una puerta que estaba abierta.
	ErrRoomOpen      = errors.New("meet: la sala no está bloqueada")
	ErrRoomFull      = errors.New("meet: la sala está completa")
	ErrPermission    = errors.New("meet: permiso insuficiente")
	ErrInvalidSecret = errors.New("meet: firma de webhook inválida")
)

// RoomStatus es el ciclo de vida de una sala.
type RoomStatus string

const (
	// RoomScheduled: creada pero aún no iniciada en LiveKit.
	RoomScheduled RoomStatus = "scheduled"
	// RoomActive: hay (o ha habido) una room viva en LiveKit.
	RoomActive RoomStatus = "active"
	// RoomEnded: la sala se cerró (room_finished o cierre explícito).
	RoomEnded RoomStatus = "ended"
)

// Valid indica si el estado es uno de los permitidos.
func (s RoomStatus) Valid() bool {
	switch s {
	case RoomScheduled, RoomActive, RoomEnded:
		return true
	default:
		return false
	}
}

// Role es el rol del participante en una sala, que determina sus grants de
// LiveKit (publicar/suscribir/moderar).
type Role string

const (
	// RoleHost: anfitrión/moderador (puede publicar, suscribir y administrar).
	RoleHost Role = "host"
	// RoleSpeaker: puede publicar y suscribir.
	RoleSpeaker Role = "speaker"
	// RoleViewer: solo suscribir (sala/ponencia de solo escucha).
	RoleViewer Role = "viewer"
)

// ValidRole valida un rol de participante.
func ValidRole(r Role) bool {
	return r == RoleHost || r == RoleSpeaker || r == RoleViewer
}

// CanPublish indica si el rol puede publicar audio/vídeo.
func (r Role) CanPublish() bool { return r == RoleHost || r == RoleSpeaker }

// IsAdmin indica si el rol puede administrar la room en LiveKit (mutear, kick).
func (r Role) IsAdmin() bool { return r == RoleHost }

// Room es una sala de Sytue Rooms (estado canónico en PostgreSQL).
type Room struct {
	ID     uuid.UUID `json:"id"`
	Tenant string    `json:"-"`
	// Name es el identificador de la room en LiveKit (único por tenant, estable).
	Name string `json:"name"`
	// Title es el nombre legible mostrado a los usuarios.
	Title string `json:"title"`
	// Status es el ciclo de vida (scheduled|active|ended).
	Status RoomStatus `json:"status"`
	// CreatedBy es el sub del usuario que creó la sala.
	CreatedBy string `json:"created_by,omitempty"`
	// MaxParticipants limita el aforo (0 = límite por defecto del servicio).
	MaxParticipants int `json:"max_participants"`
	// EmptyTimeoutSeconds es el tiempo que LiveKit mantiene la sala vacía.
	EmptyTimeoutSeconds int `json:"empty_timeout_seconds"`
	// LiveKitSID es el identificador de la room asignado por LiveKit (si activa).
	LiveKitSID string `json:"livekit_sid,omitempty"`
	// Metadata es JSON arbitrario (p. ej. intent de Chat/Calendar de origen).
	Metadata map[string]any `json:"metadata,omitempty"`
	// Locked cierra la puerta: nadie NUEVO entra hasta reabrirla. El anfitrión y
	// los admin sí pasan —si no, quien la cerró se quedaría fuera al recargar—.
	// Expulsar a un intruso no sirve de nada si puede volver en tres segundos.
	Locked bool `json:"locked,omitempty"`
	// LinkKey es la clave del enlace de una reunión suelta (mig 0040): con ella entra
	// quien no la convocó ni estuvo dentro. Solo la recibe quien ya puede entrar.
	LinkKey string `json:"link_key,omitempty"`
	// StartedAt/EndedAt marcan el ciclo real en LiveKit (vía webhooks).
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// IsJoinable indica si se puede entrar a la sala (no cerrada).
func (r Room) IsJoinable() bool { return r.Status != RoomEnded }

// Participant es la presencia de un usuario en una sala, derivada de los
// webhooks de LiveKit (participant_joined/left) y conservada como historial.
type Participant struct {
	ID     uuid.UUID `json:"id"`
	Tenant string    `json:"-"`
	RoomID uuid.UUID `json:"room_id"`
	// Identity es la identidad del participante en LiveKit (= sub de Sytue ID).
	Identity string `json:"identity"`
	// Name es el nombre legible.
	Name string `json:"name,omitempty"`
	// Role con el que entró (grants minteados en el JWT).
	Role Role `json:"role"`
	// LiveKitSID es el SID del participante en LiveKit.
	LiveKitSID string     `json:"livekit_sid,omitempty"`
	JoinedAt   time.Time  `json:"joined_at"`
	LeftAt     *time.Time `json:"left_at,omitempty"`
}

// IsPresent indica si el participante sigue conectado (sin left_at).
func (p Participant) IsPresent() bool { return p.LeftAt == nil }

// RecordingStatus es el estado de una grabación (egress de LiveKit).
type RecordingStatus string

const (
	RecordingStarted   RecordingStatus = "started"
	RecordingCompleted RecordingStatus = "completed"
	RecordingFailed    RecordingStatus = "failed"
)

// Recording es el metadato de una grabación de sala (egress de LiveKit hacia S3
// gestionado por Sytue Drive/Storage; aquí solo guardamos el metadato).
type Recording struct {
	ID       uuid.UUID       `json:"id"`
	Tenant   string          `json:"-"`
	RoomID   uuid.UUID       `json:"room_id"`
	EgressID string          `json:"egress_id"`
	Status   RecordingStatus `json:"status"`
	// StorageKey es la clave del fichero resultante (en el bucket de Meet/Drive).
	// NUNCA se serializa: es la clave del objeto en el bucket y no tiene ningún uso
	// en el cliente (la descarga va por una ruta autorizada del servicio). Se exponía
	// en el JSON de /rooms/{id}/recordings, que además no tenía verja de acceso.
	// Mismo criterio que FileVersion.StorageKey en Sytue Drive.
	StorageKey  string     `json:"-"`
	SizeBytes   int64      `json:"size_bytes,omitempty"`
	DurationSec int64      `json:"duration_sec,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
}

// Admission es una llamada a la puerta de una sala bloqueada.
//
// `GrantedAt` nulo significa esperando. No se borra al admitir: quien entra,
// sale y vuelve tendría que volver a llamar, que sería la sala de espera más
// molesta posible.
type Admission struct {
	ID       uuid.UUID `json:"id"`
	Tenant   string    `json:"-"`
	RoomID   uuid.UUID `json:"room_id"`
	Identity string    `json:"identity"`
	// Name es cómo se presenta, para que quien abre sepa a quién. Es dato de
	// PRESENTACIÓN: quien autoriza es Identity, que sale del token.
	Name        string     `json:"name,omitempty"`
	RequestedAt time.Time  `json:"requested_at"`
	GrantedAt   *time.Time `json:"granted_at,omitempty"`
}

// Esperando indica que la llamada sigue sin respuesta.
func (a Admission) Esperando() bool { return a.GrantedAt == nil }

// SourceKind identifica la app de origen que solicita crear una sala (intents
// cross-app del Hub: crear sala desde un evento de Calendar o desde un canal de
// Chat). El estado canónico del enlace (evento/canal → sala) viaja en la
// metadata de la sala para que el Hub y las apps de origen la reconcilien.
type SourceKind string

const (
	// SourceCalendar: la sala se crea desde un evento de Sytue Calendar.
	SourceCalendar SourceKind = "calendar"
	// SourceChat: la sala se crea desde un canal/hilo de Sytue Rooms.
	SourceChat SourceKind = "chat"
	// SourceDocument: la sala se crea desde un documento de Sytue Docs (Docus, Calcus
	// o Diapos). RefID es el id del documento y Title su título: nada más del
	// documento viaja a Rooms, y quién puede entrar lo decide siempre su ACL en Docs.
	SourceDocument SourceKind = "document"
	// SourceDirect: creación directa desde la app de Meet (sin origen cross-app).
	SourceDirect SourceKind = "direct"
)

// ValidSourceKind valida el origen de una sala.
func ValidSourceKind(k SourceKind) bool {
	return k == SourceCalendar || k == SourceChat || k == SourceDocument || k == SourceDirect
}

// IsCrossAppKind indica si el tipo de origen proviene de otra app (calendar/chat).
func (k SourceKind) IsCrossAppKind() bool {
	return k == SourceCalendar || k == SourceChat || k == SourceDocument
}

// RoomSource describe el origen cross-app de una sala (evento de Calendar o
// canal de Chat) para propagar el intent y permitir el enlace bidireccional
// (la app de origen guarda la URL de la sala; Meet guarda la referencia de
// origen en la metadata). RefID es el ID opaco del evento/canal en su servicio.
type RoomSource struct {
	Kind  SourceKind `json:"kind"`
	RefID string     `json:"ref_id"`
	// Title opcional del recurso de origen (asunto del evento, nombre del canal).
	Title string `json:"title,omitempty"`
}

// SourceDeMetadata recupera el origen guardado en la metadata de una sala.
//
// Hace falta al CERRARLA: el evento de cierre tiene que decir a qué canal
// pertenecía la llamada, o el aviso de «llamada terminada» no llega a ningún
// sitio y el banner se queda colgado hasta que alguien recarga.
//
// La metadata es JSON libre —viene de la base de datos y la escribió una versión
// anterior—, así que cada campo se comprueba antes de usarlo. Lo que no cuadre
// devuelve un origen vacío, que `Valid` rechaza.
func SourceDeMetadata(m map[string]any) RoomSource {
	crudo, ok := m["source"].(map[string]any)
	if !ok {
		return RoomSource{}
	}
	kind, _ := crudo["kind"].(string)
	refID, _ := crudo["ref_id"].(string)
	title, _ := crudo["title"].(string)
	return RoomSource{Kind: SourceKind(kind), RefID: refID, Title: title}
}

// Valid indica si el origen está bien formado. Un origen directo no necesita
// RefID; los orígenes calendar/chat sí requieren la referencia.
func (s RoomSource) Valid() bool {
	if !ValidSourceKind(s.Kind) {
		return false
	}
	if s.Kind == SourceDirect {
		return true
	}
	ref := strings.TrimSpace(s.RefID)
	if s.Kind == SourceDocument {
		// El id de un documento de Docs es un UUID: lo demás nunca será uno.
		_, err := uuid.Parse(ref)
		return err == nil
	}
	return ref != "" && len(ref) <= 256
}

// IsCrossApp indica si el origen proviene de otra app (no directo).
func (s RoomSource) IsCrossApp() bool { return s.Kind.IsCrossAppKind() }

// ValidTitle valida el título legible de una sala.
func ValidTitle(title string) bool {
	title = strings.TrimSpace(title)
	return title != "" && len(title) <= 200
}

// ValidRoomName valida el nombre técnico de la room en LiveKit: minúsculas,
// dígitos y guiones, longitud razonable (compatible con un identificador URL).
func ValidRoomName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 {
		return false
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if !ok {
			return false
		}
	}
	return true
}

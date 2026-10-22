// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain define los tipos y reglas de negocio de Sytue Calendar,
// independientes de transporte (HTTP/CalDAV) y de almacenamiento (PostgreSQL).
//
// Clean-room: modelo propio sobre los estandares abiertos iCalendar (RFC 5545),
// CalDAV (RFC 4791), scheduling CalDAV (RFC 6638) e iTIP (RFC 5546). Implementa
// la semantica desde las especificaciones publicas; no deriva de codigo de
// terceros AGPL.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// Errores de dominio. La capa HTTP/CalDAV los traduce a su error de transporte.
var (
	ErrNotFound           = errors.New("calendar: recurso no encontrado")
	ErrConflict           = errors.New("calendar: conflicto de unicidad")
	ErrInvalidName        = errors.New("calendar: nombre invalido")
	ErrInvalidEvent       = errors.New("calendar: evento invalido")
	ErrInvalidRange       = errors.New("calendar: rango temporal invalido")
	ErrInvalidRRULE       = errors.New("calendar: regla de recurrencia (RRULE) invalida")
	ErrPreconditionFailed = errors.New("calendar: precondicion ETag fallida")
	ErrDefaultImmutable   = errors.New("calendar: el calendario por defecto no se puede borrar")
	// ErrDemasiadosInvitados: se pide invitar a más gente de la que admite un evento.
	ErrDemasiadosInvitados = errors.New("calendar: demasiados invitados para un solo evento")
	// ErrCupoInvitaciones: la persona ha agotado sus correos de invitación de esta hora.
	ErrCupoInvitaciones = errors.New("calendar: demasiadas invitaciones por correo en la última hora")
)

// Component es el tipo de componente iCalendar de un objeto de calendario.
type Component string

const (
	CompEvent Component = "VEVENT"
)

// EventStatus es el STATUS de un VEVENT (RFC 5545 3.8.1.11).
type EventStatus string

const (
	StatusConfirmed EventStatus = "CONFIRMED"
	StatusTentative EventStatus = "TENTATIVE"
	StatusCancelled EventStatus = "CANCELLED"
)

// ValidStatus valida un STATUS.
func ValidStatus(s EventStatus) bool {
	switch s {
	case StatusConfirmed, StatusTentative, StatusCancelled:
		return true
	default:
		return false
	}
}

// Classification es el CLASS (RFC 5545 3.8.1.3).
type Classification string

const (
	ClassPublic       Classification = "PUBLIC"
	ClassPrivate      Classification = "PRIVATE"
	ClassConfidential Classification = "CONFIDENTIAL"
)

// ValidClassification valida un CLASS.
func ValidClassification(c Classification) bool {
	switch c {
	case ClassPublic, ClassPrivate, ClassConfidential:
		return true
	default:
		return false
	}
}

// Transparency es el TRANSP (RFC 5545 3.8.2.7): determina si el evento cuenta
// para la disponibilidad (free/busy).
type Transparency string

const (
	TranspOpaque      Transparency = "OPAQUE"
	TranspTransparent Transparency = "TRANSPARENT"
)

// ValidTransparency valida un TRANSP.
func ValidTransparency(t Transparency) bool {
	return t == TranspOpaque || t == TranspTransparent
}

// SanitizeColor valida un color de evento para uso SEGURO en iCalendar y CSS:
// admite un hex #RRGGBB (o #RRGGBBAA de clientes Apple) o un nombre CSS3 (solo
// letras), y descarta cualquier otro valor. Asi ningun color puede inyectar
// lineas ni parametros en el ICS (no lleva ':', ';', CR/LF ni caracteres de
// control). "" desactiva el color (el evento hereda el del calendario).
func SanitizeColor(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if isHexColor(v) {
		return strings.ToUpper(v)
	}
	if isCSSColorName(v) {
		return v
	}
	return ""
}

// SanitizeConferenceURL valida la URL de videollamada de un evento para uso SEGURO en
// iCalendar. Solo admite http(s) absolutas: un valor con ':' de mas, ';', CR/LF o
// caracteres de control podria cerrar la propiedad CONFERENCE e inyectar lineas nuevas
// en el ICS que sirve CalDAV — y ese ICS lo consumen clientes de terceros. Tambien
// acota la longitud, porque una linea desmedida rompe el plegado de RFC 5545.
// "" desactiva la videollamada del evento.
func SanitizeConferenceURL(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 512 {
		return ""
	}
	if !strings.HasPrefix(v, "https://") && !strings.HasPrefix(v, "http://") {
		return ""
	}
	// Ni saltos de linea ni controles: son la via de inyeccion en un formato de lineas.
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	// El ';' separa parametros de la propiedad y la ',' separa valores: una URL que los
	// lleve sin escapar partiria CONFERENCE en algo que no es.
	if strings.ContainsAny(v, ";,") {
		return ""
	}
	return v
}

func isHexColor(v string) bool {
	if v == "" || v[0] != '#' || (len(v) != 7 && len(v) != 9) {
		return false
	}
	for _, c := range v[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

func isCSSColorName(v string) bool {
	if v == "" || len(v) > 40 {
		return false
	}
	for _, c := range v {
		if c > unicode.MaxASCII || !unicode.IsLetter(c) {
			return false
		}
	}
	return true
}

// Calendar es una coleccion CalDAV (RFC 4791) propiedad de un usuario.
type Calendar struct {
	ID          uuid.UUID `json:"id"`
	Tenant      string    `json:"-"`
	OwnerSub    string    `json:"owner_sub"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Description string    `json:"description,omitempty"`
	Color       string    `json:"color"`
	Timezone    string    `json:"timezone"`
	Components  []string  `json:"components"`
	IsDefault   bool      `json:"is_default"`
	// ReadOnly marca el calendario como no editable por el usuario (una
	// suscripción ICS externa): el store rechaza sus escrituras con ErrReadOnly.
	ReadOnly bool `json:"read_only"`
	// Subscription, si no es nil, es el estado de sincronización de la suscripción
	// ICS externa que respalda este calendario (implica ReadOnly).
	Subscription *Subscription `json:"subscription,omitempty"`
	CTag         uuid.UUID     `json:"ctag"`
	SyncToken    int64         `json:"sync_token"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// CalendarObject es el recurso CalDAV (RFC 4791): el iCalendar serializado de un
// VEVENT (y sus overrides) compartiendo UID, mas los metadatos de sincronizacion.
type CalendarObject struct {
	ID           uuid.UUID `json:"id"`
	Tenant       string    `json:"-"`
	CalendarID   uuid.UUID `json:"calendar_id"`
	UID          string    `json:"uid"`
	ResourceName string    `json:"resource_name"`
	Component    Component `json:"component"`
	Data         string    `json:"-"`
	ETag         uuid.UUID `json:"etag"`
	SizeBytes    int       `json:"size_bytes"`
	SequenceNo   int       `json:"sequence_no"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// EventInput son los datos normalizados de un evento (maestro u override) y su
// recurso CalDAV asociado (objeto + iCalendar serializado). Es el valor que las
// capas de transporte (HTTP/CalDAV) y el store intercambian al escribir.
type EventInput struct {
	CalendarID     uuid.UUID
	UID            string
	ResourceName   string
	ICalData       string
	Summary        string
	Description    string
	Location       string
	Organizer      string
	DTStart        time.Time
	DTEnd          time.Time
	TZID           string
	AllDay         bool
	Status         EventStatus
	Classification Classification
	Transparency   Transparency
	// Color es el color del evento (hex #RRGGBB de la paleta de la suite o nombre
	// CSS3; RFC 7986 COLOR / X-APPLE-CALENDAR-COLOR). Vacio => hereda el del
	// calendario. Saneado (SanitizeColor) antes de persistir/serializar.
	Color string
	// ConferenceURL es la URL de la videollamada del evento (RFC 7986 CONFERENCE).
	// Vacia => el evento no tiene videollamada. Saneada (SanitizeConferenceURL).
	ConferenceURL string
	RRule         string
	RDate         []time.Time
	ExDate        []time.Time
	RecurrenceEnd *time.Time
	RecurrenceID  *time.Time
	SequenceNo    int
	Attendees     []Attendee
}

// Event es la proyeccion normalizada de un VEVENT (maestro u override).
type Event struct {
	ID             uuid.UUID      `json:"id"`
	Tenant         string         `json:"-"`
	CalendarID     uuid.UUID      `json:"calendar_id"`
	ObjectID       uuid.UUID      `json:"object_id"`
	UID            string         `json:"uid"`
	Summary        string         `json:"summary"`
	Description    string         `json:"description,omitempty"`
	Location       string         `json:"location,omitempty"`
	Organizer      string         `json:"organizer,omitempty"`
	DTStart        time.Time      `json:"dtstart"`
	DTEnd          time.Time      `json:"dtend"`
	TZID           string         `json:"tzid"`
	AllDay         bool           `json:"all_day"`
	Status         EventStatus    `json:"status"`
	Classification Classification `json:"classification"`
	Transparency   Transparency   `json:"transp"`
	Color          string         `json:"color,omitempty"`
	ConferenceURL  string         `json:"conference_url,omitempty"`
	RRule          string         `json:"rrule,omitempty"`
	RDate          []time.Time    `json:"rdate,omitempty"`
	ExDate         []time.Time    `json:"exdate,omitempty"`
	RecurrenceEnd  *time.Time     `json:"recurrence_end,omitempty"`
	RecurrenceID   *time.Time     `json:"recurrence_id,omitempty"`
	SequenceNo     int            `json:"sequence_no"`
	Attendees      []Attendee     `json:"attendees,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// Duration devuelve la duracion del evento.
func (e Event) Duration() time.Duration { return e.DTEnd.Sub(e.DTStart) }

// IsRecurring indica si el evento define una serie (RRULE o RDATE).
func (e Event) IsRecurring() bool { return e.RRule != "" || len(e.RDate) > 0 }

// CountsAsBusy indica si la instancia ocupa tiempo a efectos de free/busy:
// confirmada/tentativa y OPAQUE.
func (e Event) CountsAsBusy() bool {
	return e.Status != StatusCancelled && e.Transparency == TranspOpaque
}

// AttendeeRole es el ROLE del asistente (RFC 5545 3.2.16).
type AttendeeRole string

const (
	RoleChair    AttendeeRole = "CHAIR"
	RoleRequired AttendeeRole = "REQ-PARTICIPANT"
	RoleOptional AttendeeRole = "OPT-PARTICIPANT"
	RoleNonPart  AttendeeRole = "NON-PARTICIPANT"
)

// ParticipationStatus es el PARTSTAT (RFC 5545 3.2.12).
type ParticipationStatus string

const (
	PartNeedsAction ParticipationStatus = "NEEDS-ACTION"
	PartAccepted    ParticipationStatus = "ACCEPTED"
	PartDeclined    ParticipationStatus = "DECLINED"
	PartTentative   ParticipationStatus = "TENTATIVE"
	PartDelegated   ParticipationStatus = "DELEGATED"
)

// ValidPartStat valida un PARTSTAT.
func ValidPartStat(p ParticipationStatus) bool {
	switch p {
	case PartNeedsAction, PartAccepted, PartDeclined, PartTentative, PartDelegated:
		return true
	default:
		return false
	}
}

// Attendee es un ATTENDEE de un evento (RFC 5545 3.8.4.1).
type Attendee struct {
	ID         uuid.UUID           `json:"id,omitempty"`
	Email      string              `json:"email"`
	CommonName string              `json:"common_name,omitempty"`
	Role       AttendeeRole        `json:"role"`
	PartStat   ParticipationStatus `json:"partstat"`
	CUType     string              `json:"cutype,omitempty"`
	RSVP       bool                `json:"rsvp"`
}

// SchedulingMethod es el METHOD iTIP (RFC 5546 1.4).
type SchedulingMethod string

const (
	MethodRequest        SchedulingMethod = "REQUEST"
	MethodReply          SchedulingMethod = "REPLY"
	MethodCancel         SchedulingMethod = "CANCEL"
	MethodAdd            SchedulingMethod = "ADD"
	MethodRefresh        SchedulingMethod = "REFRESH"
	MethodCounter        SchedulingMethod = "COUNTER"
	MethodDeclineCounter SchedulingMethod = "DECLINECOUNTER"
)

// ValidMethod valida un METHOD de scheduling.
func ValidMethod(m SchedulingMethod) bool {
	switch m {
	case MethodRequest, MethodReply, MethodCancel, MethodAdd,
		MethodRefresh, MethodCounter, MethodDeclineCounter:
		return true
	default:
		return false
	}
}

// InvitationStatus es el estado de procesamiento de una invitacion.
type InvitationStatus string

const (
	InvPending   InvitationStatus = "pending"
	InvAccepted  InvitationStatus = "accepted"
	InvDeclined  InvitationStatus = "declined"
	InvTentative InvitationStatus = "tentative"
	InvDelivered InvitationStatus = "delivered"
	InvCancelled InvitationStatus = "cancelled"
)

// Invitation es un mensaje de scheduling iTIP entregado a un destinatario.
type Invitation struct {
	ID           uuid.UUID        `json:"id"`
	Tenant       string           `json:"-"`
	EventID      *uuid.UUID       `json:"event_id,omitempty"`
	UID          string           `json:"uid"`
	Method       SchedulingMethod `json:"method"`
	Organizer    string           `json:"organizer,omitempty"`
	Recipient    string           `json:"recipient"`
	RecipientSub string           `json:"recipient_sub,omitempty"`
	SequenceNo   int              `json:"sequence_no"`
	Data         string           `json:"-"`
	Status       InvitationStatus `json:"status"`
	CreatedAt    time.Time        `json:"created_at"`
	RespondedAt  *time.Time       `json:"responded_at,omitempty"`
	// Organization es la organización que convoca cuando NO es la de quien la lee
	// (COM-15): su bandeja la dice. Summary y Start salen entonces del iCalendar de la
	// invitación, porque su evento no se lee desde fuera. Vacíos en las propias.
	Organization string `json:"organization,omitempty"`
	// OrganizationName es su nombre visible, si Sytue ID lo da.
	OrganizationName string     `json:"organization_name,omitempty"`
	Summary          string     `json:"summary,omitempty"`
	Start            *time.Time `json:"start,omitempty"`
}

// FreeBusyType es el FBTYPE de un periodo de disponibilidad (RFC 5545 3.2.9).
type FreeBusyType string

const (
	FBBusy            FreeBusyType = "BUSY"
	FBBusyTentative   FreeBusyType = "BUSY-TENTATIVE"
	FBBusyUnavailable FreeBusyType = "BUSY-UNAVAILABLE"
	FBFree            FreeBusyType = "FREE"
)

// FreeBusyPeriod es un intervalo ocupado/libre devuelto por una consulta de
// disponibilidad (FREEBUSY, RFC 5545 3.8.2.6).
type FreeBusyPeriod struct {
	Start time.Time    `json:"start"`
	End   time.Time    `json:"end"`
	Type  FreeBusyType `json:"type"`
}

// ValidName valida el nombre de coleccion (segmento de ruta CalDAV): no vacio,
// sin separadores ni control, longitud razonable.
func ValidName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 {
		return false
	}
	if name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return false
	}
	for _, r := range name {
		if r < 0x20 {
			return false
		}
	}
	return true
}

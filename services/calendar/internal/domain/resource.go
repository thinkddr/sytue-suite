// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Recursos reservables (salas, equipos) con el patrón resource-calendar de
// Exchange/Google: cada recurso tiene su propio calendario y, al reservarlo, se
// crea un evento (booking) en ese calendario. El recurso AUTO-ACEPTA si está libre
// en la franja o RECHAZA si está ocupado (anti doble-reserva). Este fichero define
// el modelo y la lógica de conflicto PURA (sin transporte ni almacenamiento):
// reutiliza la expansión de recurrencias (RRULE/RDATE/EXDATE + TZ/DST) y el motor
// free/busy del dominio, de modo que la comprobación es correcta con series y
// cambios de hora. La atomicidad (bloqueo anti carrera) vive en el store.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Errores de dominio de recursos. La capa HTTP los traduce a su transporte.
var (
	// ErrInvalidResource: datos de recurso inválidos (tipo/nombre/aforo). Mapea a 400.
	ErrInvalidResource = errors.New("calendar: datos de recurso inválidos")
	// ErrResourceInactive: recurso desactivado; no admite reservas. Mapea a 409.
	ErrResourceInactive = errors.New("calendar: el recurso está desactivado")
	// ErrNotManager: el usuario no gestiona el recurso (editar/borrar/aprobar). 403.
	ErrNotManager = errors.New("calendar: no eres gestor de este recurso")
)

// ResourceKind es el tipo de recurso reservable.
type ResourceKind string

const (
	// ResourceRoom es una sala (con aforo y ubicación).
	ResourceRoom ResourceKind = "room"
	// ResourceEquipment es un equipo (proyector, coche, portátil…).
	ResourceEquipment ResourceKind = "equipment"
)

// ValidResourceKind valida el tipo de recurso.
func ValidResourceKind(k ResourceKind) bool {
	return k == ResourceRoom || k == ResourceEquipment
}

// Resource es un recurso reservable del tenant (sala o equipo). Cada recurso tiene
// un calendario propio (CalendarID) donde viven sus reservas: consultar su
// disponibilidad = free/busy de ese calendario. AutoAccept activa la aceptación
// automática (libre→acepta / ocupado→rechaza); Managers son los subs que pueden
// editarlo/borrarlo (además de created_by y de los admin/owner del tenant).
type Resource struct {
	ID         uuid.UUID    `json:"id"`
	Tenant     string       `json:"-"`
	CalendarID uuid.UUID    `json:"calendar_id"`
	Kind       ResourceKind `json:"kind"`
	Name       string       `json:"name"`
	Capacity   int          `json:"capacity"`
	Location   string       `json:"location,omitempty"`
	// Email es la identidad de calendario del recurso (mailto), para invitarlo como
	// asistente en el evento origen (ATTENDEE;CUTYPE=ROOM/RESOURCE). Opcional.
	Email      string    `json:"email,omitempty"`
	AutoAccept bool      `json:"auto_accept"`
	Managers   []string  `json:"managers"`
	Active     bool      `json:"active"`
	CreatedBy  string    `json:"created_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// IsManager indica si el sub dado puede gestionar (editar/borrar/aprobar) el
// recurso: es uno de sus gestores o quien lo creó. Los admin/owner del tenant se
// añaden aparte en la capa de servicio (isAdmin), donde se conoce el rol del JWT.
func (r Resource) IsManager(sub string) bool {
	if sub == "" {
		return false
	}
	if sub == r.CreatedBy {
		return true
	}
	for _, m := range r.Managers {
		if m == sub {
			return true
		}
	}
	return false
}

// ValidResource valida los datos de un recurso en el borde de confianza: tipo
// conocido, nombre no vacío/acotado, aforo no negativo y correo (si se da) válido.
func ValidResource(kind ResourceKind, name string, capacity int, email string) bool {
	if !ValidResourceKind(kind) {
		return false
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return false
	}
	if capacity < 0 {
		return false
	}
	if email != "" && !ValidEmail(email) {
		return false
	}
	return true
}

// ResourceConflict comprueba si una reserva (candidate) solaparía alguna reserva
// existente del recurso dentro de [from, to). Es el CORAZÓN del anti doble-reserva:
//   - Expande AMBOS lados con ExpandEvents (RRULE/RDATE/EXDATE respetando TZID y los
//     cambios DST) y fusiona los periodos ocupados con el motor free/busy, de modo
//     que series recurrentes y solapes a través de husos se detectan correctamente.
//   - Excluye de «existing» las reservas con el MISMO UID que la candidata: reprogramar
//     una reserva ya hecha (mover la hora del evento) no debe chocar consigo misma.
//
// Devuelve el primer periodo ocupado que solapa y true; o el cero y false si toda
// la reserva cabe libre. La atomicidad frente a reservas SIMULTÁNEAS (que dos no
// ganen a la vez) la garantiza el store con un bloqueo de fila del recurso; esta
// función es la comprobación determinista que corre bajo ese bloqueo.
func ResourceConflict(existing []Event, candidate Event, from, to time.Time) (FreeBusyPeriod, bool) {
	others := make([]Event, 0, len(existing))
	for _, e := range existing {
		if e.UID == candidate.UID {
			continue // reprograma su propia reserva: no cuenta como conflicto
		}
		others = append(others, e)
	}
	busy := FreeBusy(ExpandEvents(others, from, to), from, to)
	if len(busy) == 0 {
		return FreeBusyPeriod{}, false
	}
	for _, inst := range ExpandEvents([]Event{candidate}, from, to) {
		for _, b := range busy {
			if overlaps(inst.Start, inst.End, b.Start, b.End) {
				return b, true
			}
		}
	}
	return FreeBusyPeriod{}, false
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//nolint:tagliatelle // Google Calendar usa estas claves camelCase fijas (conferenceData).
package agenda

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/emersion/go-ical"
)

// El ENLACE DE LA REUNIÓN creado por el proveedor del calendario (A-13). En un evento de
// un calendario de Google, «Añadir Google Meet» pide a Google Calendar que le cree su sala
// al subirlo (`conferenceData.createRequest` con `conferenceDataVersion=1`; no pide otro
// permiso que el del calendario); en uno de Microsoft, «Reunión de Teams», que Graph le
// cree la reunión (`isOnlineMeeting` con `onlineMeetingProvider: teamsForBusiness`).
//
// El enlace no existe hasta que el proveedor lo crea, así que el formulario no lo puede
// pegar: Calendar marca el evento con [PropReunion] (en el `.ics` de este equipo), la
// subida lo traduce a la petición de la API, y lo que el proveedor contesta —el evento
// con su enlace en CONFERENCE y ya sin la marca— vuelve aquí en la misma vuelta (el
// Sincronizador guarda lo que devuelve el remoto tras subir, si devuelve algo). La marca
// nunca sale de este equipo: a Google y a Graph se sube su JSON, no el `.ics`.

// PropReunion es la marca de un evento que pide a su proveedor el enlace de la reunión,
// con ReunionMeet o ReunionTeams.
const PropReunion = "X-SYTUE-REUNION"

// Lo que pide PropReunion.
const (
	ReunionMeet  = "google-meet"
	ReunionTeams = "teams"
)

// pideReunion dice si un VEVENT pide la reunión de ese tipo y aún no tiene enlace.
func pideReunion(ev ical.Event, tipo string) bool {
	return strings.EqualFold(strings.TrimSpace(textoDe(ev, PropReunion)), tipo) && strings.TrimSpace(textoDe(ev, "CONFERENCE")) == ""
}

// idPeticionReunion es el `requestId` de Google para la sala de un evento: siempre el
// mismo para el mismo evento, así que subirlo otra vez (un reintento) no crea otra.
func idPeticionReunion(uid string) string {
	h := sha256.Sum256([]byte(uid + "\x00" + ReunionMeet))
	return hex.EncodeToString(h[:16])
}

// conferenciaGoogle es `conferenceData` de un evento de Google: lo que se pide al subir.
type conferenciaGoogle struct {
	CreateRequest *peticionConferencia `json:"createRequest,omitempty"`
}

type peticionConferencia struct {
	RequestID string `json:"requestId"`
	Solucion  struct {
		Type string `json:"type"`
	} `json:"conferenceSolutionKey"`
}

func peticionMeet(uid string) *conferenciaGoogle {
	p := &peticionConferencia{RequestID: idPeticionReunion(uid)}
	p.Solucion.Type = "hangoutsMeet"
	return &conferenciaGoogle{CreateRequest: p}
}

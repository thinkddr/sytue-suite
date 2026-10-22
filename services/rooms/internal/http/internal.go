// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"net/http"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// ---------------------------------------------------------------------------
// Endpoint INTERNO service-to-service (NO público): POST /internal/call-ended.
//
// Sytue Rooms lo llama al recibir room_finished de LiveKit para una sala ligada a un
// canal de chat, y así publicar la tarjeta de sistema «Llamada finalizada · duración».
// No lleva JWT de usuario: el secreto compartido (Authorization: Bearer <secreto>,
// cfg.InternalSecret) ES la autorización, comparado en tiempo CONSTANTE. El tenant viaja
// en el cuerpo (Meet lo conoce por la metadata de la sala) y se fija en el contexto para
// que RLS aísle; el acceso de escritura lo valida createMessageTx sobre el autor. Fuera
// del grupo /api/v1: no hereda su RequireAuth. Mutación por POST (sin cookies ⇒ sin CSRF).
// ---------------------------------------------------------------------------

// internalAuth exige el secreto compartido en Authorization: Bearer. Comparación en
// tiempo constante (subtle) para no filtrar el secreto por temporización. Un secreto
// vacío nunca debería llegar aquí (el router no monta la ruta si no está configurado),
// pero se rechaza igualmente por defensa en profundidad.
func (ih *intHandlers) internalAuth(next http.Handler) http.Handler {
	return httpx.SecretoInterno(ih.internalSecret)(next)
}

// ---------------------------------------------------------------------------
// Endpoint INTERNO service-to-service: GET /internal/channel-access
//
// Sytue Drive lo consulta para gatear los ADJUNTOS de un canal (ámbito de canal,
// mig 0014 de drive): los bytes viven en Drive, pero el roster vive AQUÍ, así que
// Chat es la única fuente de verdad de «¿puede este usuario leer este canal?». Se
// pregunta en CADA descarga/miniatura/subida, de modo que salir del canal (o del
// equipo, en un privado) revoca el acceso al instante y sin ACL replicada.
//
// Responde 204 si puede leer, 404 si no (o si el canal no existe: NO se distingue,
// para no filtrar existencia). Reusa la MISMA verja que leer el canal
// (store.GetChannel → requireChannelAccess), así que un adjunto nunca es más
// visible que su mensaje. Autorización: el secreto compartido (internalAuth).
// ---------------------------------------------------------------------------

// ChannelAccess responde si un sujeto puede LEER un canal. Idempotente y sin
// cuerpo: GET con tenant/channel/subject en la query.
func (ih *intHandlers) ChannelAccess(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	t, err := tenancy.New(q.Get("tenant"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid tenant."))
		return
	}
	channelID, err := uuid.Parse(q.Get("channel"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid channel."))
		return
	}
	subject := q.Get("subject")
	if subject == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("subject is required."))
		return
	}
	// GetChannel aplica requireChannelAccess: público ⇒ miembro del equipo; privado ⇒
	// miembro del canal Y del equipo; directo ⇒ participante. Cualquier negativa
	// (ErrForbidden/ErrNotFound) sale como 404 indistinguible.
	if _, err := ih.svc.GetChannel(tenancy.Into(r.Context(), t), channelID, subject); err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

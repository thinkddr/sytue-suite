// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// «Llamar desde el documento» avisa por el Hub a quien colabora en él (ADR 0070).
//
// La lista de personas con acceso NO sale de Docs: lo pide Sytue Rooms al crear la sala
// del documento, con el token de quien llama, y es Docs quien la recorre y deja los
// avisos. Rooms sigue sin saber quién puede ver el documento, que es lo que el ADR
// quería evitar copiar.

// maxAvisosDeLlamada acota a cuántas personas avisa una llamada: es un aviso de «hay
// alguien», no una convocatoria; un documento compartido con cientos no los despierta.
const maxAvisosDeLlamada = 50

// ventanaAvisoLlamada es cuánto tiempo calla el documento tras avisar de una llamada.
// Una llamada que se cuelga y se vuelve a abrir no repite el aviso a todo el mundo.
const ventanaAvisoLlamada = 10 * time.Minute

// avisosDeLlamada recuerda cuándo avisó cada documento.
//
// ponytail: en memoria y por réplica; con N réplicas el peor caso son N avisos por
// ventana. Si se notara, la misma clave con SumarTTL en Dragonfly.
type avisosDeLlamada struct {
	mu      sync.Mutex
	ultimos map[string]time.Time
	ahora   func() time.Time
}

// toca dice si el documento puede avisar ahora y, si puede, lo apunta.
func (a *avisosDeLlamada) toca(doc string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	ahora := time.Now()
	if a.ahora != nil {
		ahora = a.ahora()
	}
	if a.ultimos == nil {
		a.ultimos = map[string]time.Time{}
	}
	if t, ok := a.ultimos[doc]; ok && ahora.Sub(t) < ventanaAvisoLlamada {
		return false
	}
	// Barrido de lo caducado cuando crece: el mapa no puede ser una fuga.
	if len(a.ultimos) > 10_000 {
		for k, t := range a.ultimos {
			if ahora.Sub(t) >= ventanaAvisoLlamada {
				delete(a.ultimos, k)
			}
		}
	}
	a.ultimos[doc] = ahora
	return true
}

// destinatariosDeLlamada son a quién avisar de una llamada en el documento: su dueño y
// las PERSONAS con permiso propio, sin quien llama. A un grupo o a toda la organización
// no hay buzón concreto al que avisar (como «compartido contigo»), y una concesión a una
// dirección escrita tal cual no abre nada.
func destinatariosDeLlamada(doc *domain.Document, perms []domain.Permission, quien string) []string {
	vistos := map[string]bool{quien: true, "": true}
	var out []string
	add := func(sub string) {
		if vistos[sub] || strings.Contains(sub, "@") || len(out) >= maxAvisosDeLlamada {
			return
		}
		vistos[sub] = true
		out = append(out, sub)
	}
	add(doc.OwnerSub)
	for _, p := range perms {
		if p.PrincipalKind == domain.PrincipalUser {
			add(p.PrincipalID)
		}
	}
	return out
}

// tituloLlamada es el título de respaldo del aviso (los clientes lo componen con
// `notifications.kind.docs.call` y sus `params`).
func tituloLlamada(doc *domain.Document) string { return "Llamada en «" + doc.Title + "»" }

// avisarLlamada — POST /api/v1/documents/{id}/call-notice. Lo llama Sytue Rooms al
// crear la sala de un documento, con el token de quien la abre: basta con poder verlo.
// Responde 202 siempre que pueda verlo, haya aviso o no (sin Hub, en la ventana del
// anterior o sin nadie a quien avisar): el aviso es el extra, la llamada es el hecho.
func (h *Handler) avisarLlamada(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	doc, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleViewer)
	if err != nil {
		fail(w, r, err)
		return
	}
	if h.hubNotifier == nil || !h.avisosLlamada.toca(doc.ID) {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	perms, err := h.store.ListPermissions(r.Context(), doc.ID)
	if err != nil {
		fail(w, r, err)
		return
	}
	for _, sub := range destinatariosDeLlamada(doc, perms, subj.Sub) {
		h.avisar(r.Context(), sub, doc, "docs.call", tituloLlamada(doc), "")
	}
	w.WriteHeader(http.StatusAccepted)
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package collab

import (
	"sync"
	"time"
)

// PresenceTTL es la ventana tras la que una conexión sin heartbeat se considera
// ausente. El cliente renueva cada ~10 s (ver editor/state/collab.ts): 3× de
// margen absorbe el throttling de temporizadores en pestañas en segundo plano.
const PresenceTTL = 30 * time.Second

// PresenceRegistry mantiene la presencia efímera por documento a partir de
// heartbeats HTTP (PUT .../presence). Es el ÚNICO canal de presencia
// autenticable desde el navegador: el WS exige `Accept: text/event-stream` para
// aceptar el token en query (platform/pkg/authn/middleware.go) y la API
// WebSocket del navegador no puede fijar esa cabecera → 401. A diferencia del
// Hub (presencia por conexión WS, hoy inalcanzable desde el navegador), aquí
// cuenta el lector pasivo: abrir el documento = heartbeat = presente.
//
// Estado en memoria por proceso. Con varias réplicas cada una vería solo a los
// suyos; basta para la señal de avatares (no es estado durable).
type PresenceRegistry struct {
	mu   sync.Mutex
	docs map[string]map[string]conn // docID → connID → última señal
}

// conn es la última señal vista de una conexión (pestaña) en un documento.
type conn struct {
	sub  string
	seen time.Time
}

// NewPresenceRegistry construye un registro vacío.
func NewPresenceRegistry() *PresenceRegistry {
	return &PresenceRegistry{docs: make(map[string]map[string]conn)}
}

// Member es una conexión presente en un documento.
type Member struct {
	// Conn identifica la pestaña/conexión (el actor CRDT del cliente): permite
	// que dos pestañas del mismo usuario cuenten como dos presencias y que cada
	// una se excluya a sí misma al pintar avatares.
	Conn string `json:"conn"`
	// Sub es el usuario, derivado del token (el cliente NO lo fija).
	Sub string `json:"sub"`
}

// Touch registra o renueva (docID, connID, sub) y devuelve los miembros vivos,
// purgando de paso los expirados de esa sala.
func (p *PresenceRegistry) Touch(docID, connID, sub string) []Member {
	return p.touchAt(docID, connID, sub, time.Now())
}

// touchAt es Touch con el reloj inyectado (pruebas deterministas del TTL).
func (p *PresenceRegistry) touchAt(docID, connID, sub string, now time.Time) []Member {
	p.mu.Lock()
	defer p.mu.Unlock()

	room := p.docs[docID]
	if room == nil {
		room = make(map[string]conn)
		p.docs[docID] = room
	}
	room[connID] = conn{sub: sub, seen: now}

	out := make([]Member, 0, len(room))
	for id, c := range room {
		if now.Sub(c.seen) > PresenceTTL {
			delete(room, id)
			continue
		}
		out = append(out, Member{Conn: id, Sub: c.sub})
	}
	// ponytail: una sala cuyos miembros expiran sin más heartbeats queda hasta
	// el próximo Touch de ese doc o el reinicio del proceso (unos bytes por doc
	// abandonado). Barrido en background como el Hub si la memoria importara.
	return out
}

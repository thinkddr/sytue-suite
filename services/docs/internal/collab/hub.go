// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package collab

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// room es la sala de un documento: el conjunto de clientes conectados a esta réplica.
type room struct {
	mu       sync.RWMutex
	clients  map[*Client]struct{}
	cancelFn func() // Cancela la suscripción a DragonflyDB cuando la sala queda vacía en esta réplica
}

func newRoom() *room { return &room{clients: make(map[*Client]struct{})} }

// Hub multiplexa salas por documento (clave = document_id).
type Hub struct {
	mu     sync.RWMutex
	rooms  map[string]*room
	relay  *cache.Client
	origin string
	// reautorizarCada es cada cuánto una conexión abierta vuelve a preguntar por su
	// rol: lo que no avisa al quitarse —un enlace que caduca, un permiso de grupo o de
	// toda la organización que se retira— también corta el canal (ADR 0052). Va en el
	// Hub y no en una variable de paquete para que un test la acorte en SU hub: la
	// variable la leían las conexiones que un test anterior dejaba vivas (el servidor
	// de httptest no espera a los websockets) y -race lo daba como carrera.
	reautorizarCada time.Duration
}

// NewHub construye un Hub. relay puede ser nil o un cliente de caché deshabilitado.
func NewHub(relay *cache.Client) *Hub {
	return &Hub{
		rooms:  make(map[string]*room),
		relay:  relay,
		origin: uuid.NewString(),

		reautorizarCada: time.Minute,
	}
}

// Message es el sobre para pub/sub de DragonflyDB.
type Message struct {
	Origin string               `json:"origin"`
	Ops    []domain.SequencedOp `json:"ops,omitempty"`
	Cursor *Presence            `json:"cursor,omitempty"`
	Rol    *CambioRol           `json:"rol,omitempty"`
}

// CambioRol es el nuevo rol de una persona sobre el documento.
type CambioRol struct {
	Sub  string      `json:"sub"`
	Role domain.Role `json:"role"`
}

// join añade un cliente a la sala del documento (creándola si no existe).
func (h *Hub) join(ctx context.Context, docID string, c *Client) {
	h.mu.Lock()
	r, ok := h.rooms[docID]
	first := false
	if !ok {
		r = newRoom()
		h.rooms[docID] = r
		first = true
	}
	r.mu.Lock()
	r.clients[c] = struct{}{}
	r.mu.Unlock()
	h.mu.Unlock()

	// Si es el primer cliente local de esta réplica en la sala, nos suscribimos al canal de Dragonfly
	if first && h.relay != nil {
		h.subscribeDragonfly(ctx, docID, r)
	}
}

// leave elimina un cliente y limpia la sala vacía.
func (h *Hub) leave(docID string, c *Client) {
	h.mu.Lock()
	r, ok := h.rooms[docID]
	if !ok {
		h.mu.Unlock()
		return
	}
	r.mu.Lock()
	delete(r.clients, c)
	empty := len(r.clients) == 0
	r.mu.Unlock()

	if empty {
		delete(h.rooms, docID)
		if r.cancelFn != nil {
			r.cancelFn()
		}
	}
	h.mu.Unlock()
}

// subscribeDragonfly suscribe esta réplica al canal pub/sub de Dragonfly del documento.
func (h *Hub) subscribeDragonfly(ctx context.Context, docID string, r *room) {
	channel := "docs:collab:room:" + docID
	ch, unsubscribe, err := h.relay.Subscribe(ctx, channel)
	if err != nil {
		log.From(ctx).Error("docs: subscribe a Dragonfly falló", "doc", docID, "error", err)
		return
	}
	r.cancelFn = unsubscribe

	go func() {
		for payload := range ch {
			var msg Message
			if err := json.Unmarshal(payload, &msg); err != nil {
				continue
			}
			// Descartar el eco propio
			if msg.Origin == h.origin {
				continue
			}
			if len(msg.Ops) > 0 {
				h.broadcastLocal(docID, ServerMessage{Type: MsgOps, Ops: msg.Ops}, nil)
			}
			if msg.Cursor != nil {
				h.broadcastLocal(docID, ServerMessage{Type: MsgPresence, Cursor: msg.Cursor}, nil)
			}
			if msg.Rol != nil {
				h.cambiarRolLocal(docID, *msg.Rol)
			}
		}
	}()
}

// Presence devuelve los subs presentes en un documento (sin duplicados).
func (h *Hub) Presence(docID string) []string {
	h.mu.RLock()
	r := h.rooms[docID]
	h.mu.RUnlock()
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := make(map[string]struct{}, len(r.clients))
	out := make([]string, 0, len(r.clients))
	for c := range r.clients {
		if _, ok := seen[c.subject.Sub]; ok {
			continue
		}
		seen[c.subject.Sub] = struct{}{}
		out = append(out, c.subject.Sub)
	}
	return out
}

// PublishOps publica operaciones a clientes locales y a Dragonfly para otras réplicas.
func (h *Hub) PublishOps(ctx context.Context, docID string, ops []domain.SequencedOp, except *Client) {
	h.broadcastLocal(docID, ServerMessage{Type: MsgOps, Ops: ops}, except)

	if h.relay == nil {
		return
	}
	payload, err := json.Marshal(Message{
		Origin: h.origin,
		Ops:    ops,
	})
	if err != nil {
		return
	}
	channel := "docs:collab:room:" + docID
	pctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = h.relay.Publish(pctx, channel, payload)
}

// PublishCursor publica la presencia/cursor de un usuario a locales y a Dragonfly.
func (h *Hub) PublishCursor(ctx context.Context, docID string, p *Presence, except *Client) {
	h.broadcastLocal(docID, ServerMessage{Type: MsgPresence, Cursor: p}, except)

	if h.relay == nil {
		return
	}
	payload, err := json.Marshal(Message{
		Origin: h.origin,
		Cursor: p,
	})
	if err != nil {
		return
	}
	channel := "docs:collab:room:" + docID
	pctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = h.relay.Publish(pctx, channel, payload)
}

// PublishRole avisa a las conexiones de UNA persona de que su rol ha cambiado, en
// esta réplica y en las demás. La conexión deja de rechazar sus ops (o empieza a
// hacerlo) y el cliente desbloquea su franja de solo lectura sin recargar.
func (h *Hub) PublishRole(ctx context.Context, docID, sub string, role domain.Role) {
	if h == nil {
		return
	}
	cambio := CambioRol{Sub: sub, Role: role}
	h.cambiarRolLocal(docID, cambio)
	if h.relay == nil {
		return
	}
	payload, err := json.Marshal(Message{Origin: h.origin, Rol: &cambio})
	if err != nil {
		return
	}
	pctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = h.relay.Publish(pctx, "docs:collab:room:"+docID, payload)
}

func (h *Hub) cambiarRolLocal(docID string, cambio CambioRol) {
	h.mu.RLock()
	r := h.rooms[docID]
	h.mu.RUnlock()
	if r == nil {
		return
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for c := range r.clients {
		if c.subject.Sub == cambio.Sub {
			c.role.Store(cambio.Role)
			if !cambio.Role.CanRead() {
				// Sin acceso (permiso quitado, enlace revocado: ADR 0052) no basta con
				// dejar de escribir: el canal le seguiría trayendo los cambios. Se cierra;
				// si puede entrar por otro lado, al reconectar se autoriza de nuevo.
				go func() { _ = c.conn.Close(websocket.StatusPolicyViolation, "access revoked") }()
				continue
			}
			c.enqueue(ServerMessage{Type: MsgRole, Role: string(cambio.Role)})
		}
	}
}

// Broadcast envía un mensaje local (se mantiene para compatibilidad y envío local directo).
func (h *Hub) Broadcast(docID string, msg ServerMessage, except *Client) {
	h.broadcastLocal(docID, msg, except)
}

// broadcastLocal difunde localmente en memoria de este proceso.
func (h *Hub) broadcastLocal(docID string, msg ServerMessage, except *Client) {
	h.mu.RLock()
	r := h.rooms[docID]
	h.mu.RUnlock()
	if r == nil {
		return
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for c := range r.clients {
		if c != except {
			c.enqueue(msg)
		}
	}
}

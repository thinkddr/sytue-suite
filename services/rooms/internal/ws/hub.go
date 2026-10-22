// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ws

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// roomKey identifica una sala de difusión (aislada por tenant + canal).
type roomKey struct {
	tenant  string
	channel uuid.UUID
}

// room es el conjunto de clientes suscritos a un canal.
type room struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
}

func newRoom() *room { return &room{clients: make(map[*Client]struct{})} }

// Hub multiplexa salas por (tenant, canal). Es un broadcaster en memoria por
// proceso; el fan-out cross-réplica de los eventos DURABLES llega vía Deliver
// (lo invoca el relay del outbox sobre LISTEN/NOTIFY) y el de los EFÍMEROS
// (typing/presencia) vía el relay pub/sub de Dragonfly (StartRelay).
type Hub struct {
	mu    sync.RWMutex
	rooms map[roomKey]*room

	// relay es el pub/sub de Dragonfly para el fan-out cross-réplica de los
	// frames efímeros. nil (tests) o cliente deshabilitado (sin DRAGONFLY_URL)
	// => degradación: los efímeros quedan intra-réplica, como antes.
	relay *cache.Client
	// origin identifica este proceso para descartar el eco de sus propios
	// publish (cada réplica también recibe lo que publica).
	origin string
}

// NewHub construye un Hub vacío. relay puede ser nil o un cliente deshabilitado.
func NewHub(relay *cache.Client) *Hub {
	return &Hub{rooms: make(map[roomKey]*room), relay: relay, origin: uuid.NewString()}
}

// subscribe une un cliente a la sala (tenant, canal).
func (h *Hub) subscribe(tenant string, channel uuid.UUID, c *Client) {
	key := roomKey{tenant, channel}
	h.mu.Lock()
	r, ok := h.rooms[key]
	if !ok {
		r = newRoom()
		h.rooms[key] = r
	}
	h.mu.Unlock()

	r.mu.Lock()
	r.clients[c] = struct{}{}
	r.mu.Unlock()
}

// unsubscribe elimina un cliente de la sala y limpia salas vacías.
func (h *Hub) unsubscribe(tenant string, channel uuid.UUID, c *Client) {
	key := roomKey{tenant, channel}
	h.mu.RLock()
	r := h.rooms[key]
	h.mu.RUnlock()
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.clients, c)
	empty := len(r.clients) == 0
	r.mu.Unlock()

	if empty {
		h.mu.Lock()
		if rr, ok := h.rooms[key]; ok {
			rr.mu.RLock()
			n := len(rr.clients)
			rr.mu.RUnlock()
			if n == 0 {
				delete(h.rooms, key)
			}
		}
		h.mu.Unlock()
	}
}

// Expulsar da de baja en la sala las conexiones de esa persona: quien sale (o es
// expulsado) de un canal deja de recibirlo sin esperar a que cierre el socket (COM-08).
// Si el canal le sigue siendo legible (público), le basta volver a suscribirse: la
// suscripción se autoriza otra vez.
func (h *Hub) Expulsar(tenant string, channel uuid.UUID, subject string) {
	h.mu.RLock()
	r := h.rooms[roomKey{tenant, channel}]
	h.mu.RUnlock()
	if r == nil {
		return
	}
	var fuera []*Client
	r.mu.RLock()
	for c := range r.clients {
		if c.subject == subject {
			fuera = append(fuera, c)
		}
	}
	r.mu.RUnlock()
	for _, c := range fuera {
		c.mu.Lock()
		delete(c.subs, channel)
		c.mu.Unlock()
		h.unsubscribe(tenant, channel, c)
	}
}

// Deliver difunde un frame a todos los clientes suscritos a (tenant, canal),
// opcionalmente excluyendo a un cliente (típicamente el emisor). El envío es no
// bloqueante: un cliente lento se cierra para no degradar la sala.
func (h *Hub) Deliver(tenant string, channel uuid.UUID, frame ServerFrame, except *Client) {
	h.mu.RLock()
	r := h.rooms[roomKey{tenant, channel}]
	h.mu.RUnlock()
	if r == nil {
		return
	}
	r.mu.RLock()
	targets := make([]*Client, 0, len(r.clients))
	for c := range r.clients {
		if c != except {
			targets = append(targets, c)
		}
	}
	r.mu.RUnlock()

	for _, c := range targets {
		c.enqueue(frame)
	}
}

// ephemeralChannel es el canal pub/sub de Dragonfly compartido por todas las
// réplicas de chat para los frames efímeros (typing/presencia). Los mensajes
// persistentes NO pasan por aquí: siguen en el outbox + LISTEN/NOTIFY.
const ephemeralChannel = "chat:ephemeral"

// ephemeralMsg es el sobre publicado en Dragonfly: qué frame entregar, en qué
// salas (tenant + canales) y desde qué réplica (origin, para deduplicar el eco).
type ephemeralMsg struct {
	Origin   string      `json:"origin"`
	Tenant   string      `json:"tenant"`
	Channels []uuid.UUID `json:"channels"`
	Frame    ServerFrame `json:"frame"`
}

// publishEphemeral difunde un frame efímero a las demás réplicas vía Dragonfly.
// Best-effort: sin relay, o con Dragonfly caído, es no-op y el frame queda
// intra-réplica (degradación aceptada; es estado reconstruible).
func (h *Hub) publishEphemeral(ctx context.Context, tenant string, channels []uuid.UUID, frame ServerFrame) {
	if h.relay == nil || len(channels) == 0 {
		return
	}
	payload, err := json.Marshal(ephemeralMsg{Origin: h.origin, Tenant: tenant, Channels: channels, Frame: frame})
	if err != nil {
		return
	}
	// Cota dura de latencia: el publish corre síncrono en el readLoop del
	// cliente; con Dragonfly caído a media ejecución, los reintentos de dial de
	// go-redis estancarían cada frame varios segundos. 1 s corta eso sin afectar
	// al camino sano (<1 ms).
	pctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := h.relay.Publish(pctx, ephemeralChannel, payload); err != nil {
		// Debug, no Warn: typing es hot-path y con Dragonfly caído esto se
		// emitiría por cada frame.
		log.From(ctx).Debug("ws: publish efímero falló; frame solo intra-réplica", "error", err)
	}
}

// StartRelay suscribe esta réplica (una sola vez, al arrancar) al canal efímero
// y reenvía lo publicado por otras réplicas a los clientes locales. La goroutine
// termina al cancelar ctx. Con relay nil devuelve nil sin hacer nada; con
// cliente deshabilitado la suscripción nunca entrega (no-op inocuo).
// StartRelay suscribe esta réplica al canal efímero y reenvía lo publicado por otras
// réplicas a los clientes locales. Ante desconexiones o fallos en Dragonfly, reintenta
// la suscripción en bucle con backoff exponencial. La goroutine termina al cancelar ctx.
// Con relay nil devuelve nil sin hacer nada.
func (h *Hub) StartRelay(ctx context.Context) error {
	if h.relay == nil {
		return nil
	}
	go h.relayLoop(ctx)
	return nil
}

// relayLoop gestiona el bucle de suscripción y reconexión ante desconexiones.
func (h *Hub) relayLoop(ctx context.Context) {
	backoff := 1 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		frames, cancel, err := h.relay.Subscribe(ctx, ephemeralChannel)
		if err != nil {
			log.From(ctx).Warn("ws: error al suscribirse al relay de chat efímero, reintentando...", "error", err, "backoff", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = min(backoff*2, 30*time.Second)
				continue
			}
		}
		backoff = 1 * time.Second // restablecer backoff si la suscripción tiene éxito

		stop := make(chan struct{})
		go h.readRelayFrames(ctx, frames, cancel, stop)

		select {
		case <-ctx.Done():
			return
		case <-stop:
			log.From(ctx).Warn("ws: la suscripción al relay de chat se cerró inesperadamente, reconectando...")
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond): // pausa mínima antes de reconectar
			}
		}
	}
}

// readRelayFrames drena los payloads recibidos y los entrega localmente.
func (h *Hub) readRelayFrames(ctx context.Context, frames <-chan []byte, cancel func(), stop chan struct{}) {
	defer close(stop)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case payload, ok := <-frames:
			if !ok {
				return
			}
			h.deliverRemote(payload)
		}
	}
}

// deliverRemote entrega a los clientes locales un frame efímero publicado por
// otra réplica, descartando el eco propio (mismo origin) y payloads malformados.
func (h *Hub) deliverRemote(payload []byte) {
	var m ephemeralMsg
	if err := json.Unmarshal(payload, &m); err != nil || m.Origin == h.origin {
		return
	}
	for _, ch := range m.Channels {
		frame := m.Frame
		cid := ch
		frame.ChannelID = &cid
		h.Deliver(m.Tenant, ch, frame, nil) // el emisor está en otra réplica: sin excepciones
	}
}

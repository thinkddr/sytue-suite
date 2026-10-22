// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ws

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/log"
)

const (
	// sendBuffer es la cola de salida por cliente; si se llena, el cliente es lento.
	sendBuffer = 128
	// writeTimeout limita cada escritura al socket.
	writeTimeout = 10 * time.Second
	// pingInterval mantiene viva la conexión y detecta pares muertos.
	pingInterval = 30 * time.Second
	// maxMessageBytes acota el tamaño de un frame entrante (anti-abuso).
	maxMessageBytes = 1 << 16 // 64 KiB
	// maxSubscriptions limita los canales a los que un cliente puede suscribirse.
	maxSubscriptions = 500
)

// Authorizer comprueba que un usuario puede acceder a un canal (lectura) y dice en
// qué organización: la de su conexión o, en una conversación de otra organización
// donde está como persona de fuera, la anfitriona (ADR 0040). Lo implementa la
// capa de servicio; se inyecta para desacoplar ws de store.
type Authorizer interface {
	CanAccessChannel(ctx context.Context, tenant string, channelID uuid.UUID, subject string) (string, bool)
}

// Client representa una conexión WebSocket de un usuario.
type Client struct {
	hub     *Hub
	auth    Authorizer
	conn    *websocket.Conn
	tenant  string
	subject string
	name    string

	send chan ServerFrame

	mu sync.Mutex
	// subs guarda, por canal, la organización de su sala: la de la conexión salvo
	// en las conversaciones de otra organización, que se difunden en la suya.
	subs map[uuid.UUID]string
}

// NewClient construye un cliente para una conexión WS ya aceptada.
func NewClient(hub *Hub, auth Authorizer, conn *websocket.Conn, tenant, subject, name string) *Client {
	return &Client{
		hub:     hub,
		auth:    auth,
		conn:    conn,
		tenant:  tenant,
		subject: subject,
		name:    name,
		send:    make(chan ServerFrame, sendBuffer),
		subs:    make(map[uuid.UUID]string),
	}
}

// Subject devuelve el sub del usuario de la conexión.
func (c *Client) Subject() string { return c.subject }

// enqueue encola un frame de salida sin bloquear; si la cola está llena cierra
// la conexión (cliente lento) para no degradar la sala.
func (c *Client) enqueue(f ServerFrame) {
	select {
	case c.send <- f:
	default:
		_ = c.conn.Close(websocket.StatusPolicyViolation, "cliente lento")
	}
}

// Serve gestiona el ciclo de vida de la conexión: bucle de escritura + lectura.
func (c *Client) Serve(ctx context.Context) {
	defer c.cleanup()

	c.enqueue(ServerFrame{Type: ServerReady, Subject: c.subject})

	writerDone := make(chan struct{})
	go c.writeLoop(ctx, writerDone)

	c.readLoop(ctx)

	_ = c.conn.Close(websocket.StatusNormalClosure, "")
	<-writerDone
}

// cleanup retira al cliente de todas sus salas.
func (c *Client) cleanup() {
	c.mu.Lock()
	subs := c.subs
	c.subs = map[uuid.UUID]string{}
	c.mu.Unlock()
	for id, tenant := range subs {
		c.hub.unsubscribe(tenant, id, c)
	}
}

// writeLoop drena la cola de salida y emite pings periódicos.
func (c *Client) writeLoop(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case f, ok := <-c.send:
			if !ok {
				return
			}
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := wsjson.Write(wctx, c.conn, f)
			cancel()
			if err != nil {
				return
			}
		case <-ticker.C:
			pctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

// readLoop procesa los frames entrantes hasta el cierre de la conexión.
func (c *Client) readLoop(ctx context.Context) {
	c.conn.SetReadLimit(maxMessageBytes)
	for {
		var frame ClientFrame
		if err := wsjson.Read(ctx, c.conn, &frame); err != nil {
			if websocket.CloseStatus(err) == -1 && !errors.Is(err, context.Canceled) {
				log.From(ctx).Debug("ws: lectura finalizada", "error", err)
			}
			return
		}
		c.handle(ctx, frame)
	}
}

// handle despacha un frame del cliente.
func (c *Client) handle(ctx context.Context, f ClientFrame) {
	switch f.Type {
	case ClientSubscribe:
		c.onSubscribe(ctx, f.ChannelID)
	case ClientUnsubscribe:
		c.onUnsubscribe(f.ChannelID)
	case ClientTyping:
		c.onTyping(ctx, f.ChannelID)
	case ClientPresence:
		c.onPresence(ctx, f)
	case ClientPing:
		c.enqueue(ServerFrame{Type: ServerPong})
	}
}

// subscribed indica si el cliente está suscrito a un canal.
func (c *Client) subscribed(channelID uuid.UUID) bool {
	_, ok := c.salaDe(channelID)
	return ok
}

// salaDe devuelve la organización de la sala a la que el cliente está suscrito.
func (c *Client) salaDe(channelID uuid.UUID) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.subs[channelID]
	return t, ok
}

// onSubscribe autoriza y suscribe al cliente a la difusión de un canal.
func (c *Client) onSubscribe(ctx context.Context, channelID uuid.UUID) {
	if channelID == uuid.Nil {
		c.enqueue(errorFrame("bad_request", "channel_id obligatorio."))
		return
	}
	c.mu.Lock()
	if len(c.subs) >= maxSubscriptions {
		c.mu.Unlock()
		c.enqueue(errorFrame("too_many_subscriptions", "Demasiados canales suscritos."))
		return
	}
	_, already := c.subs[channelID]
	c.mu.Unlock()
	if already {
		return
	}
	tenant, ok := c.auth.CanAccessChannel(ctx, c.tenant, channelID, c.subject)
	if !ok {
		c.enqueue(errorFrame("forbidden", "No tienes acceso a ese canal."))
		return
	}
	c.mu.Lock()
	c.subs[channelID] = tenant
	c.mu.Unlock()
	c.hub.subscribe(tenant, channelID, c)
}

// onUnsubscribe retira la suscripción a un canal.
func (c *Client) onUnsubscribe(channelID uuid.UUID) {
	c.mu.Lock()
	tenant, ok := c.subs[channelID]
	delete(c.subs, channelID)
	c.mu.Unlock()
	if ok {
		c.hub.unsubscribe(tenant, channelID, c)
	}
}

// onTyping difunde el indicador de escritura a la sala (efímero, no persiste):
// entrega local inmediata + publish a las demás réplicas vía Dragonfly.
func (c *Client) onTyping(ctx context.Context, channelID uuid.UUID) {
	tenant, ok := c.salaDe(channelID)
	if channelID == uuid.Nil || !ok {
		return
	}
	cid := channelID
	frame := ServerFrame{
		Type:      ServerTyping,
		ChannelID: &cid,
		Subject:   c.subject,
		Name:      c.name,
	}
	c.hub.Deliver(tenant, channelID, frame, c)
	c.hub.publishEphemeral(ctx, tenant, []uuid.UUID{channelID}, frame)
}

// onPresence difunde el cambio de presencia a todas las salas del cliente
// (local + demás réplicas vía Dragonfly).
func (c *Client) onPresence(ctx context.Context, f ClientFrame) {
	status := f.Status
	if status != "away" {
		status = "online"
	}
	// Agrupados por la organización de su sala: la presencia en una conversación
	// de otra organización se difunde en la suya.
	c.mu.Lock()
	porTenant := map[string][]uuid.UUID{}
	for id, tenant := range c.subs {
		porTenant[tenant] = append(porTenant[tenant], id)
	}
	c.mu.Unlock()
	base := ServerFrame{Type: ServerPresence, Subject: c.subject, Name: c.name, Status: status}
	for tenant, subs := range porTenant {
		for _, id := range subs {
			cid := id
			frame := base
			frame.ChannelID = &cid
			c.hub.Deliver(tenant, id, frame, c)
		}
		// Un solo publish por organización con TODOS sus canales (no uno por canal):
		// el fan-out por sala lo hace cada réplica receptora sobre sus clientes locales.
		c.hub.publishEphemeral(ctx, tenant, subs, base)
	}
}

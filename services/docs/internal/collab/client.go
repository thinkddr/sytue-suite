// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package collab

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

const (
	// sendBuffer es la cola de salida por cliente; si se llena, el cliente es lento.
	sendBuffer = 64
	// writeTimeout limita cada escritura al socket.
	writeTimeout = 10 * time.Second
	// pingInterval mantiene viva la conexión y detecta pares muertos.
	pingInterval = 30 * time.Second
	// maxMessageBytes acota el tamaño de un mensaje entrante (anti-abuso).
	maxMessageBytes = 1 << 20 // 1 MiB
)

// Client representa una conexión WebSocket de un colaborador a un documento.
type Client struct {
	hub     *Hub
	conn    *websocket.Conn
	docID   string
	subject domain.Subject
	// role cambia en vivo cuando el dueño concede o quita un permiso (PublishRole),
	// desde otra goroutine que la del bucle de lectura que lo consulta.
	role atomic.Value // domain.Role
	name string

	send chan ServerMessage
	// outbound buffer de presencia (último cursor anunciado).
	cursor json.RawMessage
	// Solo el readLoop lo escribe: un cliente incompatible no puede mandar ops
	// después de un hello rechazado omitiendo el campo version.
	incompatible bool
}

// enqueue encola un mensaje de salida sin bloquear; si la cola está llena cierra
// la conexión (cliente lento), evitando back-pressure sobre el broadcaster.
func (c *Client) enqueue(msg ServerMessage) {
	msg.Version = ProtocolVersion
	select {
	case c.send <- msg:
	default:
		// Cola saturada: cerrar para no degradar a toda la sala.
		_ = c.conn.Close(websocket.StatusPolicyViolation, "cliente lento")
	}
}

// Handler orquesta el cableado del servicio en la capa de transporte (lo
// implementa internal/http). Desacopla collab de http para evitar ciclos.
type Handler interface {
	// ApplyOps secuencia y persiste operaciones (autorización incluida).
	ApplyOps(ctx context.Context, subj domain.Subject, docID string, ops []domain.Op) ([]domain.SequencedOp, int64, error)
	// OpsSince devuelve operaciones posteriores a sinceSeq para el replay.
	OpsSince(ctx context.Context, docID string, sinceSeq int64, limit int) ([]domain.SequencedOp, error)
	// Rol es el rol VIGENTE de la persona sobre el documento; domain.ErrForbidden o
	// domain.ErrNotFound si ya no puede entrar.
	Rol(ctx context.Context, subj domain.Subject, docID string) (domain.Role, error)
}

// Serve gestiona el ciclo de vida de un cliente: lo une a la sala, lanza el
// bucle de escritura y procesa los mensajes entrantes hasta el cierre.
func (c *Client) Serve(ctx context.Context, h Handler, initialSeq int64) {
	c.hub.join(ctx, c.docID, c)
	defer func() {
		c.hub.leave(c.docID, c)
		c.broadcastPresence()
	}()

	// Bienvenida + anuncio de presencia a la sala.
	c.enqueue(ServerMessage{Type: MsgWelcome, Role: string(c.rol()), Seq: initialSeq})
	c.broadcastPresence()

	writerDone := make(chan struct{})
	go c.writeLoop(ctx, writerDone)
	rctx, pararReautorizar := context.WithCancel(ctx)
	defer pararReautorizar()
	go c.reautorizar(rctx, h)

	c.readLoop(ctx, h)

	_ = c.conn.Close(websocket.StatusNormalClosure, "")
	<-writerDone
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
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := wsjson.Write(wctx, c.conn, msg)
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

// readLoop procesa los mensajes entrantes hasta que la conexión se cierra.
func (c *Client) readLoop(ctx context.Context, h Handler) {
	c.conn.SetReadLimit(maxMessageBytes)
	for {
		var msg ClientMessage
		err := wsjson.Read(ctx, c.conn, &msg)
		if err != nil {
			status := websocket.CloseStatus(err)
			if status == -1 && !errors.Is(err, context.Canceled) {
				log.From(ctx).Debug("collab: lectura WS finalizada", "error", err)
			}
			return
		}
		c.handle(ctx, h, msg)
	}
}

// handle despacha un mensaje del cliente.
func (c *Client) handle(ctx context.Context, h Handler, msg ClientMessage) {
	if c.incompatible || !compatibleVersion(msg.Version) {
		c.incompatible = true
		c.enqueue(errorMessage("protocol_version", "Unsupported collaboration protocol version."))
		return
	}
	switch msg.Type {
	case MsgHello:
		// Replay: enviar operaciones posteriores al last_seq del cliente.
		ops, err := h.OpsSince(ctx, c.docID, msg.LastSeq, 0)
		if err != nil {
			c.enqueue(errorMessage("replay_failed", "No se pudo sincronizar el documento."))
			return
		}
		if len(ops) > 0 {
			c.enqueue(ServerMessage{Type: MsgOps, Ops: ops})
		}

	case MsgCData:
		if !c.rol().CanWrite() {
			c.enqueue(errorMessage("forbidden", "Tu rol no permite editar."))
			return
		}
		if len(msg.Ops) == 0 {
			return
		}
		seqd, newSeq, err := h.ApplyOps(domain.ConProteccion(ctx, msg.Proteccion), c.subject, c.docID, msg.Ops)
		if err != nil {
			switch {
			case errors.Is(err, domain.ErrProtegido):
				c.enqueue(errorMessage("protected", "Esta hoja o este libro están protegidos."))
			case errors.Is(err, domain.ErrClaveProteccion):
				c.enqueue(errorMessage("protection_password", "La contraseña de protección no es correcta."))
			default:
				c.enqueue(errorMessage("apply_failed", "No se pudieron aplicar los cambios."))
			}
			return
		}
		// Confirmar al emisor (con seq) y difundir al resto.
		c.enqueue(ServerMessage{Type: MsgAck, Seq: newSeq, Ops: seqd})
		c.hub.PublishOps(ctx, c.docID, seqd, c)

	case MsgCursor:
		if !c.rol().CanRead() {
			return
		}
		c.cursor = msg.Cursor
		c.hub.PublishCursor(ctx, c.docID, &Presence{Sub: c.subject.Sub, Name: c.name, Cursor: msg.Cursor}, c)

	case MsgPing:
		// keep-alive de aplicación; no requiere respuesta (el ping WS lo cubre).
	}
}

// broadcastPresence anuncia la lista actual de participantes a la sala.
func (c *Client) broadcastPresence() {
	subs := c.hub.Presence(c.docID)
	pres := make([]Presence, 0, len(subs))
	for _, s := range subs {
		pres = append(pres, Presence{Sub: s})
	}
	c.hub.Broadcast(c.docID, ServerMessage{Type: MsgPresence, Presence: pres}, nil)
}

// NewClient construye un cliente de colaboración para una conexión aceptada.
func NewClient(hub *Hub, conn *websocket.Conn, docID string, subj domain.Subject, role domain.Role, name string) *Client {
	c := &Client{
		hub:     hub,
		conn:    conn,
		docID:   docID,
		subject: subj,
		name:    name,
		send:    make(chan ServerMessage, sendBuffer),
	}
	c.role.Store(role)
	return c
}

// rol es el rol vigente de la conexión.
func (c *Client) rol() domain.Role { r, _ := c.role.Load().(domain.Role); return r }

// reautorizar vuelve a preguntar por el rol cada Hub.reautorizarCada. Si ya no puede leer,
// cierra el canal (si puede entrar por otro lado, al reconectar se autoriza de nuevo);
// si cambió, se lo dice. Un fallo que no sea «no puede» (la base caída) no corta nada.
func (c *Client) reautorizar(ctx context.Context, h Handler) {
	t := time.NewTicker(c.hub.reautorizarCada)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rol, err := h.Rol(ctx, c.subject, c.docID)
			if errors.Is(err, domain.ErrForbidden) || errors.Is(err, domain.ErrNotFound) || (err == nil && !rol.CanRead()) {
				_ = c.conn.Close(websocket.StatusPolicyViolation, "access revoked")
				return
			}
			if err == nil && rol != c.rol() {
				c.role.Store(rol)
				c.enqueue(ServerMessage{Type: MsgRole, Role: string(rol)})
			}
		}
	}
}

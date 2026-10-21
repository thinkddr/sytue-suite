// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package push avisa al webmail de que ha llegado un correo, en el momento.
//
// LO QUE FALTABA. Sytue Mail no tenía NINGÚN transporte de tiempo real: ni SSE,
// ni WebSocket, ni siquiera un sondeo. La bandeja solo se refrescaba al volver a
// la pestaña (`visibilitychange`), así que con Mail abierto delante un correo
// nuevo no aparecía. En un cliente de correo eso es lo primero que se nota.
//
// POR QUÉ PostgreSQL Y NO Dragonfly. La llegada de un correo es un hecho durable
// que ya se escribe en una transacción, y `platform/pkg/cache` exige que los
// servicios funcionen sin Dragonfly —«solo pierden la capa efímera»—. Colgar de
// él algo que el usuario ve haría que se degradara en silencio. Además el hub de
// notificaciones ya resuelve exactamente esto con LISTEN/NOTIFY, así que esto es
// su hermano, no un invento nuevo.
//
// POR QUÉ NO EL DISPATCHER DEL OUTBOX. Ese reparte at-least-once: reclama cada
// evento para UNA réplica. Aquí hace falta lo contrario —que TODAS se enteren,
// porque cada una tiene sus propios clientes conectados—, y eso es justo lo que
// da un canal `pg_notify`: PostgreSQL lo entrega a todas las sesiones que
// escuchan.
package push

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// pgChannel es el canal LISTEN/NOTIFY por el que viajan las llegadas.
const pgChannel = "sytue_mail_llegadas"

// Llegada es lo que ve el cliente. Lleva lo justo para pintar la fila sin volver
// a pedir nada, y NADA del cuerpo: por este canal no viaja contenido del correo.
type Llegada struct {
	AccountID uuid.UUID `json:"account_id"`
	MailboxID uuid.UUID `json:"mailbox_id"`
	MessageID uuid.UUID `json:"message_id"`
	Subject   string    `json:"subject"`
	From      string    `json:"from"`
}

// sobre es lo que va por el NOTIFY: el destinatario y la llegada.
type sobre struct {
	Tenant  string  `json:"tenant"`
	Llegada Llegada `json:"llegada"`
}

// Publicar emite el aviso. Se llama DESPUÉS de que el mensaje esté guardado —al
// lado del evento de outbox que ya había—, así que nunca se anuncia un correo
// que no exista; y si el aviso se pierde no se pierde nada, porque el mensaje ya
// está en PostgreSQL y aparece al refrescar.
//
// `pg_notify` va por parámetros: no hay inyección posible por el asunto ni por
// el remitente, que son texto de fuera.
func Publicar(ctx context.Context, q pg.Querier, tenant string, l Llegada) error {
	b, err := json.Marshal(sobre{Tenant: tenant, Llegada: l})
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, "SELECT pg_notify($1, $2)", pgChannel, string(b))
	return err
}

// clave identifica a quién va dirigida una llegada.
type clave struct {
	tenant string
	cuenta uuid.UUID
}

type suscriptor struct {
	id uuid.UUID
	ch chan Llegada
}

// Hub reparte las llegadas a los clientes conectados A ESTA réplica. Las demás
// tienen el suyo y reciben el mismo NOTIFY, así que no hay que coordinar nada.
type Hub struct {
	mu   sync.RWMutex
	subs map[clave]map[uuid.UUID]*suscriptor
	buf  int
}

// NewHub construye el repartidor. `buffer` es lo que aguanta un cliente lento
// antes de que se le empiecen a descartar avisos.
func NewHub(buffer int) *Hub {
	if buffer <= 0 {
		buffer = 16
	}
	return &Hub{subs: make(map[clave]map[uuid.UUID]*suscriptor), buf: buffer}
}

// Subscribe registra un cliente y devuelve su canal y cómo darse de baja.
func (h *Hub) Subscribe(tenant string, cuenta uuid.UUID) (<-chan Llegada, func()) {
	s := &suscriptor{id: uuid.New(), ch: make(chan Llegada, h.buf)}
	k := clave{tenant: tenant, cuenta: cuenta}
	h.mu.Lock()
	if h.subs[k] == nil {
		h.subs[k] = make(map[uuid.UUID]*suscriptor)
	}
	h.subs[k][s.id] = s
	h.mu.Unlock()

	var una sync.Once
	return s.ch, func() {
		una.Do(func() {
			h.mu.Lock()
			if m, ok := h.subs[k]; ok {
				delete(m, s.id)
				if len(m) == 0 {
					delete(h.subs, k)
				}
			}
			h.mu.Unlock()
			close(s.ch)
		})
	}
}

// Dispatch entrega a todos los clientes de esa cuenta. NO BLOQUEA: si el buzón
// de uno está lleno se le descarta el aviso, porque el mensaje sigue en
// PostgreSQL y lo verá al refrescar. Bloquear aquí pararía el relay entero por
// culpa de un cliente lento.
func (h *Hub) Dispatch(tenant string, l Llegada) {
	k := clave{tenant: tenant, cuenta: l.AccountID}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subs[k] {
		select {
		case s.ch <- l:
		default:
		}
	}
}

// Suscritos son las conexiones vivas (métricas y tests).
func (h *Hub) Suscritos() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for _, m := range h.subs {
		n += len(m)
	}
	return n
}

// Relay escucha el canal y reparte al Hub de ESTA réplica.
type Relay struct {
	pool *pg.Pool
	hub  *Hub
}

// NewRelay construye el relay.
func NewRelay(pool *pg.Pool, hub *Hub) *Relay { return &Relay{pool: pool, hub: hub} }

// Run bloquea escuchando. Reabre con espera creciente si se pierde la conexión;
// devuelve nil al cancelarse el contexto.
func (r *Relay) Run(ctx context.Context) error {
	espera := time.Second
	for {
		if err := r.escuchar(ctx); err != nil {
			if ctx.Err() != nil {
				return nil //nolint:nilerr // igual que arriba: apagar no es fallar.
			}
			log.From(ctx).Warn("mail push: se perdió la escucha; reintentando", "error", err, "espera", espera)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(espera):
			}
			if espera < 30*time.Second {
				espera *= 2
			}
			continue
		}
		return nil
	}
}

func (r *Relay) escuchar(ctx context.Context) error {
	// Conexión DIRECTA: LISTEN es estado de sesión y no sobrevive a PgBouncer en
	// pool_mode=transaction. Es el mismo motivo que documenta el relay del hub.
	conn, err := r.pool.AcquireDirect(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{pgChannel}.Sanitize()); err != nil {
		return err
	}
	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil //nolint:nilerr // cancelación del contexto = apagado ordenado, no un fallo.
			}
			return err
		}
		var s sobre
		if err := json.Unmarshal([]byte(n.Payload), &s); err != nil {
			log.From(ctx).Warn("mail push: aviso ilegible", "error", err)
			continue
		}
		r.hub.Dispatch(s.Tenant, s.Llegada)
	}
}

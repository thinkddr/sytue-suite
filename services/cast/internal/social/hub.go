// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package social

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// sala identifica el chat de un vídeo. El id ya es único en toda la instalación;
// el tenant va en la clave para que el aislamiento por organización sea visible
// y no una consecuencia de que los UUID no choquen.
type sala struct {
	tenant string
	video  uuid.UUID
}

// Hub reparte las tramas del chat entre las conexiones de cada vídeo.
//
// Cast corre con varias réplicas y cada conexión vive en una. Con `publicar`
// (Servicio.Relevar), lo que una réplica difunde —un mensaje, un borrado, un
// bloqueo recién puesto— le llega a las demás por LISTEN/NOTIFY de PostgreSQL,
// con su origen para descartar el eco. El recuento de espectadores no se releva:
// es el de cada réplica.
type Hub struct {
	mu    sync.RWMutex
	salas map[sala]map[*Cliente]struct{}
	// origen identifica a esta réplica en el relevo; publicar es nil sin relevo.
	origen   string
	publicar func(tipo string, carga any)
}

// Lo que viaja entre réplicas.
const (
	relevoTrama   = "cast.chat.trama"
	relevoBloqueo = "cast.chat.bloqueo"
)

// canalDelChat es el canal de LISTEN/NOTIFY del relevo.
const canalDelChat events.Channel = "cast_chat"

type tramaRelevada struct {
	Origen string    `json:"o"`
	Tenant string    `json:"t"`
	Video  uuid.UUID `json:"v"`
	Trama  Trama     `json:"f"`
}

type bloqueoRelevado struct {
	Origen string `json:"o"`
	Quien  string `json:"q"`
	Sub    string `json:"s"`
	Puesto bool   `json:"p"`
}

// NewHub construye un hub vacío.
func NewHub() *Hub {
	return &Hub{salas: make(map[sala]map[*Cliente]struct{})}
}

// alta une la conexión a la sala y devuelve cuántas hay.
func (h *Hub) alta(tenant string, video uuid.UUID, c *Cliente) int {
	k := sala{tenant, video}
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.salas[k]
	if !ok {
		s = make(map[*Cliente]struct{})
		h.salas[k] = s
	}
	s[c] = struct{}{}
	return len(s)
}

// baja retira la conexión y devuelve cuántas quedan; borra la sala vacía.
func (h *Hub) baja(tenant string, video uuid.UUID, c *Cliente) int {
	k := sala{tenant, video}
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.salas[k]
	delete(s, c)
	if len(s) == 0 {
		delete(h.salas, k)
	}
	return len(s)
}

// Espectadores es cuántas conexiones tiene el chat del vídeo en esta réplica.
func (h *Hub) Espectadores(tenant string, video uuid.UUID) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.salas[sala{tenant, video}])
}

// Difundir manda la trama a todas las conexiones del vídeo, en esta réplica y en
// las demás. No bloquea: la cola de cada conexión es finita y quien no la drena
// se cierra (ver Cliente.encolar).
func (h *Hub) Difundir(tenant string, video uuid.UUID, t Trama) {
	h.entregar(tenant, video, t)
	if h.publicar != nil && t.Type != TramaEspectadores {
		h.publicar(relevoTrama, tramaRelevada{Origen: h.origen, Tenant: tenant, Video: video, Trama: t})
	}
}

// entregar manda la trama a las conexiones del vídeo en ESTA réplica.
func (h *Hub) entregar(tenant string, video uuid.UUID, t Trama) {
	h.mu.RLock()
	destinos := make([]*Cliente, 0, len(h.salas[sala{tenant, video}]))
	for c := range h.salas[sala{tenant, video}] {
		destinos = append(destinos, c)
	}
	h.mu.RUnlock()
	for _, c := range destinos {
		// Un mensaje de alguien a quien esta conexión tiene bloqueado no le llega.
		if t.Type == TramaMensaje && t.Author != nil && c.bloquea(t.Author.Sub) {
			continue
		}
		c.encolar(t)
	}
}

// bloqueo pone al día las conexiones abiertas de `quien` cuando bloquea o
// desbloquea a alguien —en esta réplica y en las demás—: si no, el cambio no
// valdría hasta reconectar.
func (h *Hub) bloqueo(quien, sub string, puesto bool) {
	h.aplicarBloqueo(quien, sub, puesto)
	if h.publicar != nil {
		h.publicar(relevoBloqueo, bloqueoRelevado{Origen: h.origen, Quien: quien, Sub: sub, Puesto: puesto})
	}
}

// recibir aplica lo que otra réplica difundió. Lo propio se descarta: ya se
// aplicó al difundirlo.
func (h *Hub) recibir(n events.Notification) {
	switch n.Type {
	case relevoTrama:
		var r tramaRelevada
		if json.Unmarshal([]byte(n.Data), &r) == nil && r.Origen != h.origen {
			h.entregar(r.Tenant, r.Video, r.Trama)
		}
	case relevoBloqueo:
		var r bloqueoRelevado
		if json.Unmarshal([]byte(n.Data), &r) == nil && r.Origen != h.origen {
			h.aplicarBloqueo(r.Quien, r.Sub, r.Puesto)
		}
	}
}

func (h *Hub) aplicarBloqueo(quien, sub string, puesto bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.salas {
		for c := range s {
			if c.quien.Sub == quien {
				c.bloquear(sub, puesto)
			}
		}
	}
}

// Relevar enchufa el relevo entre réplicas: lo que este proceso difunda sale por
// NOTIFY, y lo que difundan las demás llega a sus conexiones. Se llama una vez,
// antes de servir; escucha hasta que termine el contexto. Si el NOTIFY falla, la
// trama se queda en esta réplica, como antes de existir el relevo.
func (s *Servicio) Relevar(ctx context.Context) {
	h := s.hub
	h.origen = uuid.NewString()
	h.publicar = func(tipo string, carga any) {
		b, err := json.Marshal(carga)
		if err != nil {
			return
		}
		envio, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if err := events.Notificar(envio, s.store.pool, events.Notification{Channel: canalDelChat, Type: tipo, Data: string(b)}); err != nil {
			log.From(ctx).Warn("cast/chat: no se pudo relevar a las demás réplicas", "tipo", tipo, "error", err)
		}
	}
	sub := events.NewSubscriber(s.store.pool, func(_ context.Context, _ events.Channel, n events.Notification) {
		h.recibir(n)
	}, canalDelChat)
	go func() {
		if err := sub.Run(ctx); err != nil {
			log.From(ctx).Error("cast/chat: el relevo entre réplicas se ha parado", "error", err)
		}
	}()
}

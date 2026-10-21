// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notify

import (
	"sync"

	"github.com/google/uuid"
)

// subKey identifica de forma única el buzón de un usuario (tenant + subject).
type subKey struct {
	tenant  string
	subject string
}

// Evento es lo que viaja por una suscripción SSE: el nombre del evento
// (`notification`, o uno de estado como `avatar`) y lo que va en su `data`.
type Evento struct {
	Nombre string
	Datos  any
}

// subscriber es una suscripción SSE viva: un canal por el que se entregan los
// eventos destinados al usuario.
type subscriber struct {
	id uuid.UUID
	ch chan Evento
}

// Hub abanica notificaciones a las suscripciones SSE locales del proceso. Es el
// destino del Relay (LISTEN/NOTIFY): un proceso = un relay = un Hub. Con varias
// réplicas, cada una escucha el mismo canal Postgres y atiende a SUS clientes.
type Hub struct {
	mu   sync.RWMutex
	subs map[subKey]map[uuid.UUID]*subscriber
	// porPersona son las mismas suscripciones por buzón, sin la organización: un
	// AVISO es de la persona (ADR 0053) y le llega en la sesión de cualquiera de sus
	// organizaciones. Los eventos de estado siguen por (tenant, subject).
	porPersona map[string]map[uuid.UUID]*subscriber
	bufSize    int
}

// NewHub construye un Hub. bufferSize es la capacidad del canal por suscriptor
// (si se llena, se descartan eventos para no bloquear el relay).
func NewHub(bufferSize int) *Hub {
	if bufferSize <= 0 {
		bufferSize = 32
	}
	return &Hub{
		subs:       make(map[subKey]map[uuid.UUID]*subscriber),
		porPersona: make(map[string]map[uuid.UUID]*subscriber),
		bufSize:    bufferSize,
	}
}

// Subscribe registra una nueva suscripción SSE para los buzones (tenant,subject) de
// una persona —su `sub` y sus direcciones— y devuelve su canal de lectura y una
// función de baja (idempotente) a llamar al cerrar. `personales` son los que además
// reciben sus AVISOS de cualquier organización (domain.BuzonesPersonales): el `sub` y
// el `mailbox`, no el correo de entrar.
func (h *Hub) Subscribe(tenant string, personales []string, subjects ...string) (<-chan Evento, func()) {
	s := &subscriber{id: uuid.New(), ch: make(chan Evento, h.bufSize)}

	h.mu.Lock()
	for _, subject := range subjects {
		key := subKey{tenant: tenant, subject: subject}
		if h.subs[key] == nil {
			h.subs[key] = make(map[uuid.UUID]*subscriber)
		}
		h.subs[key][s.id] = s
	}
	for _, subject := range personales {
		if h.porPersona[subject] == nil {
			h.porPersona[subject] = make(map[uuid.UUID]*subscriber)
		}
		h.porPersona[subject][s.id] = s
	}
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			for _, subject := range subjects {
				key := subKey{tenant: tenant, subject: subject}
				if m, ok := h.subs[key]; ok {
					delete(m, s.id)
					if len(m) == 0 {
						delete(h.subs, key)
					}
				}
			}
			for _, subject := range personales {
				if m, ok := h.porPersona[subject]; ok {
					delete(m, s.id)
					if len(m) == 0 {
						delete(h.porPersona, subject)
					}
				}
			}
			h.mu.Unlock()
			close(s.ch)
		})
	}
	return s.ch, cancel
}

// Dispatch entrega un evento a todas las suscripciones del destinatario.
// La entrega es no bloqueante: si el buzón de un cliente está lleno, se descarta
// el evento para ese cliente (el estado durable sigue en PostgreSQL, así que el
// cliente lo recupera al reconectar/listar).
//
// Un aviso (`notification`) va a la PERSONA, esté en la organización que esté; un
// evento de estado, solo a la organización en la que ocurrió.
func (h *Hub) Dispatch(tenant, subject string, n Evento) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	// Un aviso a un buzón personal, a todas sus sesiones; uno al correo de entrar (que
	// no es personal: ADR 0053), solo en su organización. Sin repetir a nadie.
	vistos := map[uuid.UUID]bool{}
	entregar := func(m map[uuid.UUID]*subscriber) {
		for id, s := range m {
			if vistos[id] {
				continue
			}
			vistos[id] = true
			select {
			case s.ch <- n:
			default:
			}
		}
	}
	entregar(h.subs[subKey{tenant: tenant, subject: subject}])
	if n.Nombre == "notification" {
		entregar(h.porPersona[subject])
	}
}

// SubscriberCount devuelve el número de suscripciones vivas (métricas/tests).
func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	vivas := map[uuid.UUID]bool{} // una suscripción está en tantas claves como buzones tenga
	for _, m := range h.subs {
		for id := range m {
			vivas[id] = true
		}
	}
	return len(vivas)
}

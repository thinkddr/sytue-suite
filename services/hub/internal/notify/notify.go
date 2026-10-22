// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package notify implementa las notificaciones unificadas de Sytue Hub: creación
// durable en PostgreSQL y reparto en tiempo real por SSE sobre LISTEN/NOTIFY (sin
// Redis/Valkey). Un único relay escucha el canal Postgres del proceso y abanica a
// las suscripciones SSE locales, filtrando por destinatario (tenant+subject).
//
// Clean-room: SSE (WHATWG EventSource / text/event-stream) y LISTEN/NOTIFY de
// PostgreSQL; sin código de terceros AGPL.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

// pgChannel es el canal PostgreSQL LISTEN/NOTIFY de notificaciones de Hub.
const pgChannel = "sytue_hub_notifications"

// wireEnvelope es el sobre que viaja por el NOTIFY: identifica al destinatario y
// transporta la notificación completa para que el relay no reconsulte la BD.
//
// Con `Event` es un evento de ESTADO (ver PublishEvent): no hay notificación, y lo
// que viaja es su nombre y sus datos.
type wireEnvelope struct {
	Tenant       string              `json:"tenant"`
	Subject      string              `json:"subject"`
	Notification domain.Notification `json:"notification"`
	Event        string              `json:"event,omitempty"`
	Data         json.RawMessage     `json:"data,omitempty"`
}

// evento es lo que el sobre entrega al flujo SSE.
func (e wireEnvelope) evento() Evento {
	if e.Event != "" {
		return Evento{Nombre: e.Event, Datos: e.Data}
	}
	return Evento{Nombre: "notification", Datos: e.Notification}
}

// TopeAvisosPorHora es cuántos avisos puede dejar una organización a un mismo
// destinatario en una hora (ADR 0053).
const TopeAvisosPorHora = 200

// TopeAvisosTotalPorHora es el total, de cualquier tipo, a un mismo destinatario.
const TopeAvisosTotalPorHora = 1000

// TopeAvisosPersonaPorHora es el total a un mismo destinatario desde TODAS las
// organizaciones, existan o no: el tope que no se rodea inventando nombres.
const TopeAvisosPersonaPorHora = 2000

// Service crea notificaciones y dispara su reparto en tiempo real.
type Service struct {
	store *store.Store
	pool  *pg.Pool
	hub   *Hub
	// topePersona es TopeAvisosPersonaPorHora; ConTopePersona lo baja en las pruebas.
	topePersona int
}

// ConTopePersona cambia el tope por persona y hora (para probarlo sin crear 2000 avisos).
func (s *Service) ConTopePersona(n int) *Service {
	s.topePersona = n
	return s
}

// NewService construye el servicio de notificaciones. El Hub es el abanicador
// local de suscripciones SSE.
func NewService(st *store.Store, hub *Hub) *Service {
	return &Service{store: st, pool: st.Pool(), hub: hub, topePersona: TopeAvisosPersonaPorHora}
}

// Create persiste una notificación y emite el NOTIFY de tiempo real en la MISMA
// transacción (el aviso solo viaja si la escritura hace commit).
//
// Antes de nada consulta las preferencias del destinatario: un aviso silenciado NO se
// persiste. Filtrar al ENTREGAR (y no al leer) es lo que hace que el contador de no
// leídos diga la verdad — guardarlo y ocultarlo dejaría el badge contando cosas que el
// usuario decidió no recibir. Se devuelve una notificación vacía sin error: para quien
// publica, «silenciado» no es un fallo.
func (s *Service) Create(ctx context.Context, rec domain.NewNotification) (domain.Notification, error) {
	if err := rec.Validate(); err != nil {
		return domain.Notification{}, err
	}
	t := tenancy.MustFrom(ctx)
	if rec.Subject != "" {
		// Las reglas de la PERSONA, de todas sus organizaciones (ADR 0053): basta con
		// que lo calle en una.
		porOrg, err := s.store.PrefsDeLaPersona(ctx, rec.Subject)
		if err != nil {
			// Leer preferencias no puede impedir un aviso: ante la duda, se entrega.
			log.From(ctx).Warn("hub: leer preferencias de notificación", "error", err)
		}
		for _, prefs := range porOrg {
			if domain.Muted(prefs, rec.Source, rec.Type) {
				return domain.Notification{}, nil
			}
		}
	}
	var n domain.Notification
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Primero el tope por PERSONA en todas las organizaciones: el Hub no sabe qué
		// organizaciones existen, así que un emisor con el secreto podría inventar
		// nombres para repartir avisos y rodear el tope por organización. Toma además
		// el cerrojo del destinatario, que serializa los recuentos de abajo.
		// Precio aceptado: quien tenga el secreto puede agotar en una hora el tope de
		// alguien en todas sus organizaciones; antes se quedaba en la suya. El secreto
		// solo lo tienen los servicios, y las rutas son solo desde dentro.
		if hechos, e := store.DeLaPersona(ctx, tx, rec.Subject, false); e != nil {
			return e
		} else if hechos >= s.topePersona {
			return domain.ErrTooMany
		}
		// Un tope por emisor, destinatario y tipo (ADR 0053): un aviso llega a la campana
		// de la persona en todas sus organizaciones, y una organización no puede
		// llenársela. ponytail: una hora fija y un número, contados antes de insertar bajo
		// el cerrojo del destinatario; si alguien legítimo lo roza, subirlo.
		if hechos, e := store.AvisosRecientes(ctx, tx, t.Slug, rec.Subject, rec.Source, rec.Type); e != nil {
			return e
		} else if hechos >= TopeAvisosPorHora {
			return domain.ErrTooMany
		}
		// Y uno total: el tipo es texto libre, y variándolo no se esquiva el tope.
		if hechos, e := store.AvisosRecientes(ctx, tx, t.Slug, rec.Subject, "", ""); e != nil {
			return e
		} else if hechos >= TopeAvisosTotalPorHora {
			return domain.ErrTooMany
		}
		var e error
		n, e = store.InsertNotificationTx(ctx, tx, t.Slug, rec)
		if e != nil {
			return e
		}
		return publishTx(ctx, tx, wireEnvelope{Tenant: t.Slug, Subject: n.Subject, Notification: n})
	})
	return n, err
}

// CreateHistorico guarda un aviso RECONSTRUIDO (ruta de mantenimiento, ver
// domain.NewHistorico) y devuelve qué pasó con él (domain.Historico*). A diferencia
// de Create: no emite el NOTIFY —ni tiempo real, ni push, ni nada que suene—, nace
// leído, lleva su fecha real y su clave de origen, y solo entra si es ANTERIOR al
// primer aviso en vivo de su tipo en la organización y a domain.HistoricoHasta. Las
// preferencias de silencio se respetan igual que en Create.
func (s *Service) CreateHistorico(ctx context.Context, h domain.NewHistorico) (string, error) {
	if !time.Now().Before(domain.HistoricoCierre) {
		return "", domain.ErrGone
	}
	if err := h.Validate(); err != nil {
		return "", err
	}
	t := tenancy.MustFrom(ctx)
	porOrg, err := s.store.PrefsDeLaPersona(ctx, h.Subject)
	if err != nil {
		// Aquí no hay prisa ni nadie esperando el aviso: ante la duda, no se reconstruye.
		return "", err
	}
	for _, prefs := range porOrg {
		if domain.Muted(prefs, h.Source, h.Type) {
			return domain.HistoricoSilenciado, nil
		}
	}
	resultado := domain.HistoricoRepetido
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Una dirección retirada no recibe nada de antes: sería dárselo a quien la
		// hereda (OlvidarBuzon borra lo que tenía; esto no puede resucitarlo).
		if olvidado, e := store.BuzonOlvidado(ctx, tx, h.Subject); e != nil {
			return e
		} else if olvidado {
			resultado = domain.HistoricoOlvidado
			return nil
		}
		techo := domain.HistoricoHasta
		if vivo, e := store.PrimerAvisoEnVivo(ctx, tx, t.Slug, h.Type); e != nil {
			return e
		} else if vivo != nil && vivo.Add(-domain.MargenHistorico).Before(techo) {
			techo = vivo.Add(-domain.MargenHistorico)
		}
		if !h.Fecha.Before(techo) || h.Fecha.After(time.Now()) {
			resultado = domain.HistoricoPosterior
			return nil
		}
		if h.Fecha.Before(time.Now().Add(-domain.VentanaHistorica)) {
			resultado = domain.HistoricoAntiguo
			return nil
		}
		// Un resultado y no un error: relanzar no lo cambia, y no debe tumbar el comando.
		if hechos, e := store.DeLaPersona(ctx, tx, h.Subject, true); e != nil {
			return e
		} else if hechos >= domain.TopeHistoricosPorPersona {
			resultado = domain.HistoricoTope
			return nil
		}
		creado, e := store.InsertHistoricoTx(ctx, tx, t.Slug, h)
		if creado {
			resultado = domain.HistoricoCreado
		}
		return e
	})
	return resultado, err
}

// publishTx emite pg_notify del sobre en la transacción dada (atómico con la
// inserción). pg_notify es seguro frente a inyección (parámetros).
func publishTx(ctx context.Context, q pg.Querier, env wireEnvelope) error {
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, "SELECT pg_notify($1, $2)", pgChannel, string(b))
	return err
}

// PublishEvent reparte un evento de ESTADO a las pantallas abiertas de una persona
// —«cambió su foto»— sin guardarlo: no sale en la campana, no cuenta como no leído
// y no pasa por las preferencias, que son de avisos. Viaja por el mismo NOTIFY que
// los avisos para que llegue a los clientes de TODAS las réplicas, no solo de esta.
func (s *Service) PublishEvent(ctx context.Context, tenant, subject, event string, data json.RawMessage) error {
	return publishTx(ctx, s.pool, wireEnvelope{Tenant: tenant, Subject: subject, Event: event, Data: data})
}

// UnreadCount delega en el store (lo usa el handshake SSE para el badge inicial).
func (s *Service) UnreadCount(ctx context.Context, subjects []string) (int, error) {
	return s.store.UnreadCount(ctx, subjects)
}

// Relay escucha el canal PostgreSQL de notificaciones y abanica cada sobre a las
// suscripciones SSE locales cuyo (tenant,subject) coincide. Bloquea hasta que se
// cancele el contexto; reconecta con backoff (lo gestiona events.Subscriber).
type Relay struct {
	pool *pg.Pool
	hub  *Hub
}

// NewRelay construye el relay de tiempo real.
func NewRelay(pool *pg.Pool, hub *Hub) *Relay { return &Relay{pool: pool, hub: hub} }

// Run bloquea escuchando LISTEN y entregando al Hub. Reabre la conexión con
// backoff si se pierde; devuelve nil al cancelarse el contexto.
func (r *Relay) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // cancelación del contexto = apagado ordenado, no un fallo.
		}
		err := r.listen(ctx)
		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}
		log.From(ctx).Warn("notify relay: conexión LISTEN perdida, reintentando",
			"error", err, "backoff", backoff.String())
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// listen adquiere una conexión dedicada, ejecuta LISTEN y entrega cada NOTIFY al
// Hub hasta que la conexión se pierda o el contexto se cancele.
func (r *Relay) listen(ctx context.Context) error {
	// Conexión DIRECTA a PostgreSQL: LISTEN es estado de sesión y no funciona a
	// través de PgBouncer en pool_mode=transaction (prod). En dev es el pool normal.
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
				return nil
			}
			return err
		}
		var env wireEnvelope
		if err := json.Unmarshal([]byte(n.Payload), &env); err != nil {
			log.From(ctx).Warn("notify relay: payload NOTIFY inválido", "error", err)
			continue
		}
		r.hub.Dispatch(env.Tenant, env.Subject, env.evento())
	}
}

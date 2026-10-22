// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package auditsink entrega la auditoría de cada organización a SU SIEM (Splunk HEC,
// Microsoft Sentinel, Elastic, Datadog…).
//
// La auditoría ya se podía sacar tirando de ella (CSV/JSON bajo demanda), pero un SIEM
// no funciona así: espera RECIBIR los eventos para correlacionarlos con el resto de la
// seguridad del cliente. Sin push, esa correlación llega tarde o no llega.
//
// Cómo entrega, y por qué así:
//
//   - En LOTES desde un cursor monótono (`audit_log.id`), en orden ascendente. El SIEM
//     tiene que ver los eventos en el orden en que ocurrieron, y el id desempata sin
//     ambigüedad dos eventos del mismo instante.
//   - El cursor solo avanza tras un 2xx. Si el SIEM está caído, el lote se reintenta en
//     el siguiente tick: es preferible repetir un evento a perderlo (at-least-once), y
//     los SIEM deduplican por el id que va en cada registro.
//   - El cuerpo va FIRMADO con HMAC-SHA256 (cabecera `X-Sytue-Signature`), para que el
//     receptor pueda comprobar que el lote viene de Sytue y no ha sido manipulado.
//   - El cliente HTTP es el SSRF-safe de la plataforma: la URL la escribe un admin, pero
//     un admin comprometido no puede convertir esto en un escáner de la red interna.
package auditsink

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

const (
	// batchSize acota cuántos eventos van en un POST. Un lote grande reduce viajes pero
	// aumenta lo que se repite cuando falla: 200 es el término medio que usan los HEC.
	batchSize = 200
	// maxBatchesPerTick evita que una organización con un atasco de meses monopolice el
	// despachador: se ponen al día en varios ticks en vez de en una ráfaga.
	maxBatchesPerTick = 10
	// httpTimeout por POST. Un SIEM lento no puede bloquear a los demás.
	httpTimeout = 15 * time.Second
)

// Store es lo que el despachador necesita de la persistencia.
type Store interface {
	ListEnabledAuditSinks(ctx context.Context) ([]store.AuditSink, error)
	AuditSince(ctx context.Context, orgID string, afterID int64, limit int) ([]store.AuditRow, error)
	MarkAuditSent(ctx context.Context, orgID string, lastID int64) error
	MarkAuditSinkError(ctx context.Context, orgID, msg string) error
}

// Dispatcher empuja la auditoría pendiente de cada organización a su SIEM.
type Dispatcher struct {
	store  Store
	client *http.Client
}

// New construye el despachador con un cliente HTTP SSRF-safe: la URL la escribe un
// administrador, pero uno comprometido no puede convertir esto en un escáner de la red
// interna (el cliente resuelve y bloquea rangos privados en CADA envío, también si el
// DNS cambia después de guardar la configuración).
func New(st Store) *Dispatcher {
	return &Dispatcher{store: st, client: ssrfhttp.NewClient(httpTimeout, 2)}
}

// newWithClient existe para los tests, que necesitan hablar con un httptest.Server en
// 127.0.0.1 — justo lo que el cliente de producción bloquea, y con razón.
func newWithClient(st Store, c *http.Client) *Dispatcher {
	return &Dispatcher{store: st, client: c}
}

// event es la forma de un registro en el lote. Plana y con nombres explícitos: la
// consume un SIEM ajeno, así que se prefiere legible a compacto.
type event struct {
	ID         int64           `json:"id"`
	OccurredAt time.Time       `json:"occurred_at"`
	OrgID      string          `json:"org_id"`
	ActorID    string          `json:"actor_id,omitempty"`
	ActorEmail string          `json:"actor_email,omitempty"`
	Action     string          `json:"action"`
	Target     string          `json:"target,omitempty"`
	IP         string          `json:"ip,omitempty"`
	Detail     json.RawMessage `json:"detail,omitempty"`
}

type batch struct {
	Source string  `json:"source"`
	OrgID  string  `json:"org_id"`
	Events []event `json:"events"`
}

// DispatchAll recorre los sinks activos y entrega lo pendiente de cada uno. Un fallo en
// una organización NO detiene a las demás: se anota en su sink y se sigue.
func (d *Dispatcher) DispatchAll(ctx context.Context) error {
	sinks, err := d.store.ListEnabledAuditSinks(ctx)
	if err != nil {
		return err
	}
	for _, s := range sinks {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := d.dispatchOne(ctx, s); err != nil {
			// El error ya quedó anotado en el sink; aquí solo se evita cortar el barrido.
			_ = d.store.MarkAuditSinkError(ctx, s.OrgID, err.Error())
		}
	}
	return nil
}

// dispatchOne vacía la cola de una organización, en varios lotes si hace falta.
func (d *Dispatcher) dispatchOne(ctx context.Context, s store.AuditSink) error {
	cursor := s.LastSentID
	for i := 0; i < maxBatchesPerTick; i++ {
		rows, err := d.store.AuditSince(ctx, s.OrgID, cursor, batchSize)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil // al día
		}
		if err := d.post(ctx, s, rows); err != nil {
			return err
		}
		cursor = rows[len(rows)-1].ID
		if err := d.store.MarkAuditSent(ctx, s.OrgID, cursor); err != nil {
			return err
		}
		if len(rows) < batchSize {
			return nil
		}
	}
	return nil
}

// post entrega un lote y exige 2xx. Cualquier otra cosa es un error que NO avanza el
// cursor: el lote se repetirá, que es lo correcto para una pista de auditoría.
func (d *Dispatcher) post(ctx context.Context, s store.AuditSink, rows []store.AuditRow) error {
	body, err := json.Marshal(toBatch(s.OrgID, rows))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Sytue-ID-AuditSink/1.0")
	if s.AuthHeader != "" {
		req.Header.Set("Authorization", s.AuthHeader)
	}
	if s.SigningSecret != "" {
		req.Header.Set("X-Sytue-Signature", "sha256="+sign(body, s.SigningSecret))
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	// Se drena un poco del cuerpo para poder reutilizar la conexión y, de paso, dar un
	// error con la respuesta del SIEM: «400» a secas no le dice nada al administrador.
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("el SIEM respondió %d: %s", resp.StatusCode, bytes.TrimSpace(snippet))
	}
	return nil
}

func toBatch(orgID string, rows []store.AuditRow) batch {
	evs := make([]event, 0, len(rows))
	for _, r := range rows {
		evs = append(evs, event{
			ID: r.ID, OccurredAt: r.OccurredAt, OrgID: orgID,
			ActorID: r.ActorID, ActorEmail: r.ActorEmail,
			Action: r.Action, Target: r.Target, IP: r.IP,
			Detail: json.RawMessage(r.Detail),
		})
	}
	return batch{Source: "sytue-id", OrgID: orgID, Events: evs}
}

// sign devuelve el HMAC-SHA256 del cuerpo en hexadecimal.
func sign(body []byte, secret string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

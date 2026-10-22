// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package hubnotify provee un cliente HTTP ligero y reutilizable para que los servicios
// de la suite emitan notificaciones unificadas a Sytue Hub (POST /internal/notifications).
package hubnotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
)

// Notification describe una notificación unificada a crear en el Hub.
type Notification struct {
	Tenant  string `json:"tenant"`
	Subject string `json:"subject"`
	Source  string `json:"source"`
	Type    string `json:"type"`
	Level   string `json:"level,omitempty"` // info, success, warning, error (defecto: info)
	Title   string `json:"title"`
	Body    string `json:"body,omitempty"`
	URL     string `json:"url,omitempty"`
	// Params son las variables del título —`actor`, `name`…— con las que cada
	// cliente lo compone en el idioma de quien lo lee, con la clave
	// `notifications.kind.<Type>` del catálogo común. Title sigue siendo
	// obligatorio: es el respaldo de un cliente que no conozca el tipo.
	Params map[string]string `json:"params,omitempty"`
}

// PorCorreo es el destinatario de un aviso para quien el emisor solo conoce por su
// dirección: un invitado de Calendar, una persona con quien se comparte escribiendo su
// correo. El Hub se lo enseña a quien tenga esa dirección en su token (`email` o
// `mailbox`) dentro de la misma organización. Es la forma de avisar sin preguntarle
// antes a Sytue ID quién es, que desde un servicio no se puede sin la sesión de nadie.
func PorCorreo(email string) string {
	return "email:" + strings.ToLower(strings.TrimSpace(email))
}

// Compartido es el aviso de «compartido contigo» a una persona: el mismo para todos los
// que comparten algo con alguien (Drive, Photos, Notepads). `tipo` es el de su catálogo
// (`drive.share.created`, `photos.shared`, `notes.shared`) y `actor` quien comparte, por
// su nombre: sin él, el título de respaldo no nombra a nadie y los clientes componen el
// suyo sin `actor`.
func Compartido(tenant, para, source, tipo, nombre, actor, enlace string) Notification {
	title := fmt.Sprintf("«%s» compartido contigo", nombre)
	params := map[string]string{"name": nombre}
	if actor != "" {
		title = fmt.Sprintf("%s ha compartido «%s» contigo", actor, nombre)
		params["actor"] = actor
	}
	return Notification{
		Tenant: tenant, Subject: para, Source: source, Type: tipo, Level: "info",
		Title: title, URL: enlace, Params: params,
	}
}

// MaxTituloRunas es lo que el Hub admite de título: un nombre de fichero, un asunto o
// el título de un documento largos, con su «ha compartido «…» contigo», lo pasaban, y
// el Hub contestaba 400 y el aviso se perdía.
const MaxTituloRunas = 200

// RecortarTitulo deja el título en MaxTituloRunas, con «…» si se corta.
func RecortarTitulo(t string) string {
	if utf8.RuneCountInString(t) <= MaxTituloRunas {
		return t
	}
	return validation.TruncateRunes(t, MaxTituloRunas-1) + "…"
}

// Client envía notificaciones al Sytue Hub por HTTP interno.
type Client struct {
	baseURL string
	secret  string
	hc      *http.Client
}

// New construye un cliente de notificaciones hacia el Hub.
// Devuelve nil si baseURL está vacía (servicio Hub deshabilitado o no configurado).
func New(baseURL, secret string) *Client {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return nil
	}
	return &Client{
		baseURL: base,
		secret:  strings.TrimSpace(secret),
		hc:      &http.Client{Timeout: 4 * time.Second},
	}
}

// Send entrega la notificación al Hub por el endpoint interno /internal/notifications.
// Si c es nil, es una no-op y devuelve nil.
func (c *Client) Send(ctx context.Context, n Notification) error {
	if c == nil {
		return nil
	}
	if strings.TrimSpace(n.Tenant) == "" || strings.TrimSpace(n.Subject) == "" || strings.TrimSpace(n.Title) == "" {
		return fmt.Errorf("hubnotify: tenant, subject y title son obligatorios")
	}
	if n.Level == "" {
		n.Level = "info"
	}
	n.Title = RecortarTitulo(n.Title)
	return c.post(ctx, "/internal/notifications", n)
}

// EventoAvatar es el evento de estado que manda Sytue ID cuando alguien cambia o
// quita su foto. Datos: {"version": "<avatar_version>"}, vacía si ya no tiene foto.
// Las barras de la suite (web y Fyne) lo escuchan en el flujo de la campana y
// vuelven a pedir la foto sin recargar.
const EventoAvatar = "avatar"

// Evento es un cambio de ESTADO para las pantallas abiertas de una persona, no un
// aviso: el Hub lo reparte por el flujo SSE con su propio nombre (`event: avatar`)
// y no lo guarda, así que ni sale en la campana ni cuenta como no leído. Quien no
// tenga nada abierto no lo recibe, y no hace falta: al abrir, la pantalla ya lee
// el estado bueno.
type Evento struct {
	Tenant  string `json:"tenant"`
	Subject string `json:"subject"`
	Event   string `json:"event"`
	Data    any    `json:"data,omitempty"`
}

// SendEvent entrega un evento de estado por /internal/events. Si c es nil, no-op.
func (c *Client) SendEvent(ctx context.Context, e Evento) error {
	if c == nil {
		return nil
	}
	if strings.TrimSpace(e.Tenant) == "" || strings.TrimSpace(e.Subject) == "" || strings.TrimSpace(e.Event) == "" {
		return fmt.Errorf("hubnotify: tenant, subject y event son obligatorios")
	}
	return c.post(ctx, "/internal/events", e)
}

// SendEventAsync es SendEvent sin esperar, como SendAsync.
func (c *Client) SendEventAsync(ctx context.Context, e Evento) {
	c.async(ctx, func(ctx context.Context) error { return c.SendEvent(ctx, e) }, "event", e.Event)
}

// OlvidarBuzon pide al Hub que borre, en todas las organizaciones, los avisos a una
// dirección que Sytue ID acaba de retirar: si se reasigna, quien la reciba no hereda
// los de quien la tuvo (ADR 0053).
func (c *Client) OlvidarBuzon(ctx context.Context, direccion string) error {
	if c == nil {
		return nil
	}
	return c.post(ctx, "/internal/notifications/forget", map[string]string{"address": direccion})
}

func (c *Client) post(ctx context.Context, ruta string, v any) error {
	return c.postLee(ctx, ruta, v, nil)
}

// postLee es post leyendo la respuesta en `out` (nil = descartarla).
func (c *Client) postLee(ctx context.Context, ruta string, v, out any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("hubnotify: serializar payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+ruta, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("hubnotify: crear petición: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("hubnotify: error de red enviando a hub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("hubnotify: hub respondió HTTP %d", resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("hubnotify: leer respuesta: %w", err)
		}
	}
	return nil
}

// SendAsync lanza Send en una goroutine desacoplada con timeout para no bloquear
// el hilo principal si la llamada es best-effort.
func (c *Client) SendAsync(ctx context.Context, n Notification) {
	c.async(ctx, func(ctx context.Context) error { return c.Send(ctx, n) }, "source", n.Source, "type", n.Type)
}

func (c *Client) async(ctx context.Context, enviar func(context.Context) error, attrs ...any) {
	if c == nil {
		return
	}
	//nolint:gosec // contexto desacoplado deliberadamente para no abortar el envío si la petición HTTP termina antes
	go func() {
		bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := enviar(bgCtx); err != nil {
			log.From(bgCtx).Warn("hubnotify: envío asíncrono fallido", append([]any{"error", err}, attrs...)...)
		}
	}()
}

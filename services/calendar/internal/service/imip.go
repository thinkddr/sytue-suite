// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-ical"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/correo"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// Invitaciones por correo (iMIP, RFC 6047) a quien NO es de la suite. Dentro de la
// suite la invitación ya llega a la bandeja de planificación y a la campana del Hub;
// una dirección de fuera no tiene ninguna de las dos, y sin esto se quedaba sin
// enterarse. Sale por el relé de Sytue Mail con su REQUEST (o su CANCEL) y su
// ORGANIZER, así que el REPLY del cliente de quien la recibe vuelve al buzón de quien
// organiza.

// EnviarCorreo manda un correo iMIP a UN destinatario. Es `correo.EnviarCalendario`
// con el buzón de Calendar; el guardián lo cambia por uno que apunta.
type EnviarCorreo func(ctx context.Context, para, asunto, cuerpo string, ics []byte, metodo string) error

// SetCorreo engancha el envío iMIP. Sin `enviar`, no sale ningún correo.
func (s *Service) SetCorreo(enviar EnviarCorreo) { s.correo = enviar }

// SetDominiosInternos fija los dominios de correo de la suite: los que Sytue Mail tiene
// dados de alta (VigilarDominios). Se puede llamar en caliente.
func (s *Service) SetDominiosInternos(ds []string) { s.internos.Store(&ds) }

// dominiosInternos son los de Mail más el apex de PublicURL, que es de la suite aunque
// Mail aún no haya contestado: así, sin Mail, nadie de dentro recibe un correo de más
// por ser del dominio principal.
func (s *Service) dominiosInternos() []string {
	var ds []string
	if p := s.internos.Load(); p != nil {
		ds = *p
	}
	if apex := apexDe(s.cfg.PublicURL); apex != "" {
		ds = append(slices.Clip(ds), apex)
	}
	return ds
}

// DominiosDeMail pide a Sytue Mail sus dominios de correo (`GET /internal/domains`,
// con el secreto de la suite).
func DominiosDeMail(ctx context.Context, base, secreto string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/internal/domains", http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+secreto)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendar: dominios de Mail: HTTP %d", resp.StatusCode)
	}
	var cuerpo struct {
		Domains []string `json:"domains"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&cuerpo); err != nil {
		return nil, fmt.Errorf("calendar: dominios de Mail: %w", err)
	}
	return cuerpo.Domains, nil
}

// VigilarDominios pide los dominios a Mail ahora y luego cada `cada`, hasta que se
// cancele `ctx`. Si Mail no contesta, se queda con los últimos que dio: un dominio que
// se da de alta tarda como mucho `cada` en contar como de dentro.
func (s *Service) VigilarDominios(ctx context.Context, base, secreto string, cada time.Duration) {
	t := time.NewTicker(cada)
	defer t.Stop()
	for {
		c, cancelar := context.WithTimeout(ctx, 10*time.Second)
		ds, err := DominiosDeMail(c, base, secreto)
		cancelar()
		if err != nil {
			log.From(ctx).Warn("calendar: no se pudieron leer los dominios de Sytue Mail", "error", err)
		} else {
			s.SetDominiosInternos(ds)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// CorreoDesdeConfig es el envío por el buzón de la configuración, o nil si no hay.
func CorreoDesdeConfig(c correo.Config) EnviarCorreo {
	if !c.Activo() {
		return nil
	}
	return func(ctx context.Context, para, asunto, cuerpo string, ics []byte, metodo string) error {
		return correo.EnviarCalendario(ctx, c, para, asunto, cuerpo, ics, metodo)
	}
}

// apexDe es el dominio de la suite a partir de la URL pública del servicio:
// `https://calendar.sytue.eu` → `sytue.eu`.
func apexDe(publica string) string {
	u, err := url.Parse(publica)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if i := strings.Index(host, "."); i > 0 && strings.Count(host, ".") >= 2 {
		return host[i+1:]
	}
	return host
}

// EsDeFuera dice si una dirección no es de la suite: su dominio no es ninguno de los
// internos ni un subdominio suyo.
func EsDeFuera(email string, internos []string) bool {
	_, dom, ok := strings.Cut(strings.ToLower(strings.TrimSpace(email)), "@")
	if !ok || dom == "" {
		return false
	}
	for _, d := range internos {
		d = strings.ToLower(strings.TrimSpace(d))
		if d != "" && (dom == d || strings.HasSuffix(dom, "."+d)) {
			return false
		}
	}
	return true
}

// asuntoIMIP es el asunto del correo: el de los clientes de calendario («Invitación:»,
// «Cancelado:»), que es lo que la persona reconoce en su bandeja.
//
// ponytail: en castellano, como los avisos que el servicio deja en el Hub; el cliente
// de quien lo recibe pinta la invitación con su propio idioma a partir del iCalendar.
func asuntoIMIP(ev domain.Event, metodo domain.SchedulingMethod) (asunto, cuerpo string) {
	cuando := ev.DTStart.UTC().Format("02/01/2006 15:04") + " UTC"
	if ev.AllDay {
		cuando = ev.DTStart.UTC().Format("02/01/2006")
	}
	org := cleanAddr(ev.Organizer)
	if metodo == domain.MethodCancel {
		return "Cancelado: " + ev.Summary, fmt.Sprintf("%s ha cancelado «%s» (%s).", org, ev.Summary, cuando)
	}
	return "Invitación: " + ev.Summary, fmt.Sprintf("%s te invita a «%s» (%s).\nContesta desde tu calendario: la respuesta le llega a quien organiza.", org, ev.Summary, cuando)
}

// maxCorreosAFueraPorHora es cuántos correos de invitación a direcciones de fuera manda
// una persona en una hora. Salen del remitente de avisos de la plataforma, que comparten
// Drive, Notepads y la recuperación de cuentas: sin tope, una cuenta cualquiera lo usaba
// para escribir a quien quisiera con el asunto que quisiera (COM-07).
const maxCorreosAFueraPorHora = 100

// cupoHora reparte ese tope por persona, en horas de reloj.
//
// ponytail: en memoria y por réplica (el chart corre de 2 a 8), así que el tope real es
// ese múltiplo y se olvida al reiniciar. cache.SumarTTL si hace falta exacto.
type cupoHora struct {
	mu    sync.Mutex
	hora  time.Time
	usado map[string]int
}

// libre son los correos que a la persona le quedan esta hora.
func (c *cupoHora) libre(quien string, ahora time.Time) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if h := ahora.Truncate(time.Hour); !h.Equal(c.hora) {
		c.hora, c.usado = h, map[string]int{}
	}
	return max(maxCorreosAFueraPorHora-c.usado[quien], 0)
}

// tomar aparta n correos del cupo de la persona, todos o ninguno: media invitación
// repartida es peor que ninguna.
func (c *cupoHora) tomar(quien string, n int, ahora time.Time) bool {
	if n > c.libre(quien, ahora) {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.usado[quien] += n
	return true
}

// deFuera son las direcciones de los asistentes que no son de la suite, sin repetir y
// sin quien organiza: las que reciben la invitación por correo.
func (s *Service) deFuera(org string, asistentes []domain.Attendee) []string {
	internos := s.dominiosInternos()
	var fuera []string
	for _, a := range asistentes {
		para := strings.ToLower(strings.TrimSpace(a.Email))
		if para == "" || strings.EqualFold(para, org) || !EsDeFuera(para, internos) || slices.Contains(fuera, para) {
			continue
		}
		fuera = append(fuera, para)
	}
	return fuera
}

// mandarIMIP manda la invitación (o su cancelación) por correo a cada asistente de
// fuera. Best effort y en segundo plano: un relé caído no puede frenar el guardado.
//
// Una sola goroutine por entrega, con los correos en fila: antes era una goroutine y
// una conexión SMTP por asistente, y el cuerpo de una petición admitía miles.
func (s *Service) mandarIMIP(ctx context.Context, ev domain.Event, metodo domain.SchedulingMethod, ics string) {
	if s.correo == nil || (metodo != domain.MethodRequest && metodo != domain.MethodCancel) {
		return
	}
	fuera := s.deFuera(cleanAddr(ev.Organizer), ev.Attendees)
	if len(fuera) == 0 {
		return
	}
	lg := log.From(ctx)
	if c, _ := authn.FromContext(ctx); !s.cupoCorreo.tomar(c.Subject, len(fuera), time.Now()) {
		lg.Warn("calendar: tope de invitaciones por correo de esta hora; no salen", "uid", ev.UID, "destinatarios", len(fuera))
		return
	}
	asunto, cuerpo := asuntoIMIP(ev, metodo)
	enviar := s.correo
	go func() {
		for _, para := range fuera {
			c, cancelar := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
			err := enviar(c, para, asunto, cuerpo, []byte(ics), string(metodo))
			cancelar()
			if err != nil {
				lg.Warn("calendar: invitación por correo", "uid", ev.UID, "error", err)
			}
		}
	}()
}

// Las respuestas que vuelven por correo: el cliente de quien recibió la invitación
// manda un REPLY al ORGANIZER, que es un buzón de Sytue Mail, y Mail lo pasa aquí
// (`POST /internal/imip/reply`) con la dirección que lo firma. Contestar es lo mismo que
// hace la bandeja con alguien de dentro: la invitación queda respondida, el PARTSTAT del
// asistente cambia y el organizador recibe su REPLY en la bandeja.

// ErrRespuestaAjena es un REPLY en el que quien contesta no es quien lo firma: un
// correo no puede cambiar la respuesta de otra persona.
var ErrRespuestaAjena = errors.New("calendar: el REPLY no es de quien lo manda")

// partstatAInvitacion es la inversa de `invStatusToPartStat` del store.
func partstatAInvitacion(p string) (domain.InvitationStatus, bool) {
	switch strings.ToUpper(strings.TrimSpace(p)) {
	case string(domain.PartAccepted):
		return domain.InvAccepted, true
	case string(domain.PartDeclined):
		return domain.InvDeclined, true
	case string(domain.PartTentative):
		return domain.InvTentative, true
	}
	return "", false
}

// LeerRespuesta saca de un iCalendar METHOD:REPLY el UID y la respuesta de cada
// asistente que trae (normalmente uno). ok=false si no es un REPLY legible.
func LeerRespuesta(ics string) (uid string, respuestas map[string]domain.InvitationStatus, ok bool) {
	cal, err := ical.NewDecoder(strings.NewReader(ics)).Decode()
	if err != nil {
		return "", nil, false
	}
	if m := cal.Props.Get(ical.PropMethod); m == nil || !strings.EqualFold(strings.TrimSpace(m.Value), "REPLY") {
		return "", nil, false
	}
	respuestas = map[string]domain.InvitationStatus{}
	for _, c := range cal.Children {
		if c.Name != ical.CompEvent {
			continue
		}
		if p := c.Props.Get(ical.PropUID); p != nil && uid == "" {
			uid = strings.TrimSpace(p.Value)
		}
		for _, a := range c.Props[ical.PropAttendee] {
			if st, ok := partstatAInvitacion(a.Params.Get(ical.ParamParticipationStatus)); ok {
				respuestas[strings.ToLower(cleanAddr(a.Value))] = st
			}
		}
	}
	return uid, respuestas, uid != "" && len(respuestas) > 0
}

// RecibirRespuesta aplica un REPLY que ha llegado por correo firmado por `remitente`.
// Solo cuenta la respuesta del propio remitente, y solo si tiene una invitación
// pendiente de ese evento: nada de lo que diga el correo sobre otras personas.
func (s *Service) RecibirRespuesta(ctx context.Context, remitente, ics string) error {
	uid, respuestas, ok := LeerRespuesta(ics)
	if !ok {
		return domain.ErrInvalidEvent
	}
	remitente = strings.ToLower(strings.TrimSpace(remitente))
	estado, ok := respuestas[remitente]
	if !ok || remitente == "" {
		return ErrRespuestaAjena
	}
	id, err := s.store.InvitacionPendiente(ctx, uid, remitente)
	if err != nil {
		return err
	}
	inv, err := s.store.RespondInvitation(ctx, id, estado, []string{remitente}, "")
	if err != nil {
		return err
	}
	if inv.EventID != nil && inv.Organizer != "" {
		if ev, err := s.store.GetEvent(ctx, *inv.EventID); err == nil {
			s.deliverReply(ctx, ev, inv.Recipient)
		}
	}
	s.publish(ctx, "calendar.invitation.responded", map[string]any{
		"invitation_id": inv.ID, "status": inv.Status, "via": "imip",
	})
	return nil
}

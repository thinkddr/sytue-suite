// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

// Posponer y silenciar una conversación (D-MAIL-07): el servicio saca el hilo de
// Recibidos hasta una fecha y lo devuelve solo, sin leer; y un hilo silenciado manda
// sus respuestas nuevas al Archivo. Es del servicio: en el modo privado los hilos los
// guarda cifrados el núcleo y nadie los despertaría, así que ahí todo es
// ErrPrivateMailOperation y la app no lo ofrece (MailPospone).

// MailSnooze es una conversación pospuesta: vuelve a Recibidos en `Until`.
type MailSnooze struct {
	ThreadID string    `json:"thread_id"`
	Until    time.Time `json:"until"`
}

// MailEstadoHilo es lo que el lector pinta del hilo además de sus mensajes.
type MailEstadoHilo struct {
	Muted        bool       `json:"muted"`
	SnoozedUntil *time.Time `json:"snoozed_until,omitempty"`
}

var (
	// ErrSnoozeInvalido: la fecha no cae entre un minuto y un año desde ahora.
	ErrSnoozeInvalido = errors.New("mail: fecha de posponer fuera de rango")
	// ErrNadaQuePosponer: el hilo no tiene nada en Recibidos (ni estaba pospuesto).
	ErrNadaQuePosponer = errors.New("mail: nada que posponer en Recibidos")
)

// MailPospone dice si esta cuenta pospone y silencia: el servicio sí, el modo privado
// no.
func (m *Mail) MailPospone() bool { return m.data == nil }

// Snoozes lista las conversaciones pospuestas, la que vuelve antes primero.
func (m *Mail) Snoozes(ctx context.Context) ([]MailSnooze, error) {
	if m.data != nil {
		return nil, nil
	}
	r, err := JSON[struct {
		Snoozes []MailSnooze `json:"snoozes"`
	}](ctx, m.c, Request{Path: "/snoozes"})
	return r.Snoozes, err
}

// SnoozeThread pospone el hilo hasta `hasta` (o le cambia la fecha si ya lo estaba).
func (m *Mail) SnoozeThread(ctx context.Context, hilo string, hasta time.Time) (MailSnooze, error) {
	if m.data != nil {
		return MailSnooze{}, ErrPrivateMailOperation
	}
	r, err := m.peticion(http.MethodPut, "/threads/"+url.PathEscape(hilo)+"/snooze",
		map[string]any{"until": hasta.UTC().Format(time.RFC3339)})
	if err != nil {
		return MailSnooze{}, err
	}
	sn, err := JSON[MailSnooze](ctx, m.c, r)
	var deAPI *Error
	if errors.As(err, &deAPI) {
		switch {
		case deAPI.Code() == "nothing_to_snooze":
			return MailSnooze{}, ErrNadaQuePosponer
		case deAPI.Code() == "invalid_snooze" || deAPI.Status == http.StatusBadRequest:
			return MailSnooze{}, ErrSnoozeInvalido
		}
	}
	return sn, err
}

// UnsnoozeThread devuelve ya a Recibidos una conversación pospuesta.
func (m *Mail) UnsnoozeThread(ctx context.Context, hilo string) error {
	if m.data != nil {
		return ErrPrivateMailOperation
	}
	return m.sinCuerpo(ctx, http.MethodDelete, "/threads/"+url.PathEscape(hilo)+"/snooze", nil)
}

// MuteThread silencia la conversación (lo que tenga en Recibidos se archiva) o deja de
// silenciarla. Devuelve cuántos mensajes ha archivado.
func (m *Mail) MuteThread(ctx context.Context, hilo string, silenciar bool) (int, error) {
	if m.data != nil {
		return 0, ErrPrivateMailOperation
	}
	r, err := m.peticion(http.MethodPut, "/threads/"+url.PathEscape(hilo)+"/mute", map[string]any{"muted": silenciar})
	if err != nil {
		return 0, err
	}
	v, err := JSON[struct {
		Archived int `json:"archived"`
	}](ctx, m.c, r)
	return v.Archived, err
}

// ThreadState es el estado del hilo (silenciado, pospuesto hasta…), que el servicio
// devuelve junto a sus mensajes. En el modo privado no hay: el de por defecto.
func (m *Mail) ThreadState(ctx context.Context, hilo string) (MailEstadoHilo, error) {
	if m.data != nil {
		return MailEstadoHilo{}, nil
	}
	return JSON[MailEstadoHilo](ctx, m.c, Request{Path: "/threads/" + url.PathEscape(hilo) + "/messages"})
}

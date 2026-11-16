// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Snooze es una conversación pospuesta (migración 0036): sus mensajes de Recibidos
// esperan en el buzón de rol `snoozed` hasta `Until`, y entonces vuelven a la
// Entrada sin leer. Es del HILO y no del mensaje, como en Gmail: lo que se aparta
// es la conversación entera.
type Snooze struct {
	ThreadID uuid.UUID `json:"thread_id"`
	Until    time.Time `json:"until"`
}

// ThreadState es lo que la vista de conversación necesita saber del hilo además de
// sus mensajes: si está silenciado y hasta cuándo está pospuesto.
type ThreadState struct {
	Muted        bool       `json:"muted"`
	SnoozedUntil *time.Time `json:"snoozed_until,omitempty"`
}

const (
	// SnoozeMinimo descarta lo que vencería antes del siguiente barrido del
	// trabajador: posponer «hasta ahora mismo» sería un ida y vuelta sin sentido.
	SnoozeMinimo = time.Minute
	// SnoozeMaximo acota a un año: más allá ya no es posponer sino olvidar, y una
	// fecha a siglos vista suele ser un año mal tecleado.
	SnoozeMaximo = 366 * 24 * time.Hour
)

var (
	// ErrInvalidSnooze: la fecha no cae entre SnoozeMinimo y SnoozeMaximo desde ahora.
	ErrInvalidSnooze = errors.New("mail: fecha de posponer fuera de rango")
	// ErrNothingToSnooze: el hilo no tiene nada en Recibidos ni estaba pospuesto.
	// Posponer saca de la Entrada; lo que no está en ella no tiene de dónde salir.
	ErrNothingToSnooze = errors.New("mail: nada que posponer en Recibidos")
)

// ValidarSnooze comprueba que `until` es una fecha a la que se puede posponer
// visto desde `now`.
func ValidarSnooze(until, now time.Time) error {
	if until.IsZero() {
		return ErrInvalidSnooze
	}
	d := until.Sub(now)
	if d < SnoozeMinimo || d > SnoozeMaximo {
		return ErrInvalidSnooze
	}
	return nil
}

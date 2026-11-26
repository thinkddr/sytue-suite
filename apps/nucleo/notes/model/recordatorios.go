// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"strings"
	"time"

	"github.com/teambition/rrule-go"
)

// NormalizarRecordatorio son las reglas de un recordatorio de nota, las MISMAS en el
// servicio (`domain.ReminderInput.Normalize`) y en Sytue Office: una zona que exista
// (vacía, UTC), una sola forma de repetir (RRULE o «a los N días», no las dos) y una RRULE
// calculable que no repita más de una vez al día; el vencimiento, en UTC y al minuto.
// Devuelve los valores ya limpios, o ErrInvalidInput.
func NormalizarRecordatorio(regla, zona string, vence time.Time, despuesDias int) (string, string, time.Time, error) {
	regla = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(regla), "RRULE:"))
	zona = strings.TrimSpace(zona)
	if zona == "" {
		zona = "UTC"
	}
	if vence.IsZero() || len(zona) > 64 || despuesDias < 0 || despuesDias > 365 ||
		(regla != "" && despuesDias > 0) || len(regla) > 200 {
		return regla, zona, vence, ErrInvalidInput
	}
	if _, err := time.LoadLocation(zona); err != nil {
		return regla, zona, vence, ErrInvalidInput
	}
	if regla != "" {
		if strings.ContainsAny(regla, "\r\n") || strings.Contains(strings.ToUpper(regla), "DTSTART") {
			return regla, zona, vence, ErrInvalidInput
		}
		opt, err := rrule.StrToROption(regla)
		if err != nil || opt.Freq > rrule.DAILY || len(opt.Byhour) > 0 || len(opt.Byminute) > 0 || len(opt.Bysecond) > 0 {
			return regla, zona, vence, ErrInvalidInput
		}
	}
	return regla, zona, vence.UTC().Truncate(time.Minute), nil
}

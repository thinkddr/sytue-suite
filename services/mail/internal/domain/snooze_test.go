// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"testing"
	"time"
)

func TestValidarSnooze(t *testing.T) {
	now := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	casos := []struct {
		nombre string
		until  time.Time
		ok     bool
	}{
		{"mañana", now.Add(24 * time.Hour), true},
		{"justo el mínimo", now.Add(SnoozeMinimo), true},
		{"justo el máximo", now.Add(SnoozeMaximo), true},
		{"sin fecha", time.Time{}, false},
		{"en el pasado", now.Add(-time.Hour), false},
		{"ahora mismo", now.Add(10 * time.Second), false},
		{"más de un año", now.Add(SnoozeMaximo + time.Minute), false},
	}
	for _, c := range casos {
		err := ValidarSnooze(c.until, now)
		if c.ok && err != nil {
			t.Errorf("%s: err=%v, quería que valiera", c.nombre, err)
		}
		if !c.ok && !errors.Is(err, ErrInvalidSnooze) {
			t.Errorf("%s: err=%v, quería ErrInvalidSnooze", c.nombre, err)
		}
	}
}

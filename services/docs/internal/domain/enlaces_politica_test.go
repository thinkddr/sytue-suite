// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"testing"
	"time"
)

func TestCaducidadDeEnlace(t *testing.T) {
	ahora := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	dias := func(n int) *time.Time { v := ahora.Add(time.Duration(n) * 24 * time.Hour); return &v }
	casos := []struct {
		nombre  string
		pedida  *time.Time
		max     int
		quiere  *time.Time
		quiereE error
	}{
		{"sin fecha, techo por defecto: caduca en el techo", nil, 365, dias(365), nil},
		{"sin fecha, sin techo: no caduca", nil, 0, nil, nil},
		{"dentro del techo", dias(30), 365, dias(30), nil},
		{"justo en el techo", dias(365), 365, dias(365), nil},
		{"un día más allá", dias(366), 365, nil, ErrCaducidadExcedida},
		{"muy lejos", dias(36500), 7, nil, ErrCaducidadExcedida},
		{"lejos pero sin techo", dias(36500), 0, dias(36500), nil},
		{"pasada", dias(-1), 365, nil, ErrCaducidadPasada},
		{"ahora mismo", dias(0), 365, nil, ErrCaducidadPasada},
		{"pasada sin techo", dias(-1), 0, nil, ErrCaducidadPasada},
	}
	for _, c := range casos {
		got, err := CaducidadDeEnlace(ahora, c.pedida, c.max)
		if !errors.Is(err, c.quiereE) {
			t.Errorf("%s: err = %v, se esperaba %v", c.nombre, err, c.quiereE)
			continue
		}
		if (got == nil) != (c.quiere == nil) || (got != nil && !got.Equal(*c.quiere)) {
			t.Errorf("%s: caducidad = %v, se esperaba %v", c.nombre, got, c.quiere)
		}
	}
}

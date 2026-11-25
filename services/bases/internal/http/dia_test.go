// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package baseshttp

import (
	"net/url"
	"testing"
	"time"
)

// `?today=` es el día de quien pide en su huso: vale el de ayer, hoy y mañana del
// servidor (los husos llegan a ±14 h) y nada más lejos, que ya no es un huso sino otra
// fecha colándose como «hoy».
func TestDiaDeQuienPide(t *testing.T) {
	dia := func(d time.Duration) string { return time.Now().UTC().Add(d).Format(time.DateOnly) }
	casos := []struct {
		valor string
		vale  bool
	}{
		{"", true},
		{dia(0), true},
		{dia(-24 * time.Hour), true},
		{dia(24 * time.Hour), true},
		{dia(-72 * time.Hour), false},
		{dia(72 * time.Hour), false},
		{"2026-13-01", false},
		{"ayer", false},
	}
	for _, c := range casos {
		d, ok := diaDeQuienPide(url.Values{"today": {c.valor}})
		if ok != c.vale {
			t.Errorf("today=%q: ok = %v, quería %v", c.valor, ok, c.vale)
		}
		if ok && c.valor != "" && d.Format(time.DateOnly) != c.valor {
			t.Errorf("today=%q: leído %s", c.valor, d.Format(time.DateOnly))
		}
	}
	// Sin `today`, el alta usa el día del servidor.
	if hoy, ok := hoyDe(url.Values{}); !ok || hoy.IsZero() {
		t.Errorf("sin today: %v %v", hoy, ok)
	}
}

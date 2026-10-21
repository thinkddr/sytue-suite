// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain_test

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

func TestParseSearchQuery(t *testing.T) {
	t.Parallel()
	fecha := func(s string) time.Time {
		d, err := time.Parse(time.DateOnly, s)
		if err != nil {
			t.Fatalf("fecha de prueba inválida %q: %v", s, err)
		}
		return d
	}

	casos := []struct {
		nombre string
		q      string
		quiero domain.SearchQuery
	}{
		{
			nombre: "sin operadores queda igual que entró",
			q:      "factura de marzo",
			quiero: domain.SearchQuery{Text: "factura de marzo"},
		},
		{
			nombre: "remitente y adjunto",
			q:      "from:ana has:attachment presupuesto",
			quiero: domain.SearchQuery{Text: "presupuesto", From: "ana", HasAttachment: true},
		},
		{
			nombre: "destinatario, asunto entrecomillado y estado",
			q:      `to:equipo@sytue.eu subject:"informe anual" is:unread`,
			quiero: domain.SearchQuery{To: "equipo@sytue.eu", Subject: "informe anual", Unseen: true},
		},
		{
			nombre: "rango de fechas",
			q:      "after:2026-01-31 before:2026-03-01 nóminas",
			quiero: domain.SearchQuery{Text: "nóminas", After: fecha("2026-01-31"), Before: fecha("2026-03-01")},
		},
		{
			nombre: "el operador admite mayúsculas",
			q:      "From:Ana IS:starred",
			quiero: domain.SearchQuery{From: "Ana", Flagged: true},
		},
		// Lo importante de los tres siguientes: NADA se traga en silencio. Un
		// operador que no existe o con un valor que no se entiende vuelve al texto,
		// porque quien lo escribió estaba buscando esas palabras.
		{
			nombre: "operador desconocido vuelve al texto",
			q:      "nota:reunión",
			quiero: domain.SearchQuery{Text: "nota:reunión"},
		},
		{
			nombre: "valor no entendido vuelve al texto",
			q:      "is:importante has:foto before:ayer",
			quiero: domain.SearchQuery{Text: "is:importante has:foto before:ayer"},
		},
		{
			nombre: "entre comillas es texto literal aunque lleve dos puntos",
			q:      `"from:ana"`,
			quiero: domain.SearchQuery{Text: `"from:ana"`},
		},
		{
			nombre: "operador sin valor no filtra nada",
			q:      "from: hola",
			quiero: domain.SearchQuery{Text: "from: hola"},
		},
		{
			nombre: "comilla sin cerrar no descarta lo escrito",
			q:      `subject:"sin cerrar`,
			quiero: domain.SearchQuery{Subject: "sin cerrar"},
		},
		{
			nombre: "consulta vacía",
			q:      "   ",
			quiero: domain.SearchQuery{},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			got := domain.ParseSearchQuery(c.q)
			if got != c.quiero {
				t.Errorf("ParseSearchQuery(%q)\n  got  %+v\n  want %+v", c.q, got, c.quiero)
			}
		})
	}
}

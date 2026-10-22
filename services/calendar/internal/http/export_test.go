// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

func TestExportEntregaUnICSPorCalendario(t *testing.T) {
	cal := uuid.New()
	f := &fakeService{
		calendars:   []domain.Calendar{{ID: cal, Name: "trabajo", DisplayName: "Trabajo"}},
		objects:     []domain.CalendarObject{{CalendarID: cal, Data: "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:x\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"}},
		rangeEvents: []domain.Event{{UID: "x", Summary: "Reunión", CalendarID: cal}},
	}
	h := &Handlers{svc: f}
	rec := httptest.NewRecorder()
	h.Export(rec, req("GET", "/api/v1/export?format=csv", "", &authn.Claims{Subject: "u1", Tenant: "acme"}, nil))

	partes := map[string]string{}
	for parte, err := range portabilidad.Leer(rec.Result()) {
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(parte)
		partes[parte.Ruta] = string(b)
	}
	if !strings.Contains(partes["calendars/Trabajo.ics"], "UID:x") {
		t.Fatalf("ICS: %v", partes)
	}
	if !strings.Contains(partes["events.csv"], "Reunión") || !strings.Contains(partes["calendars.csv"], "Trabajo") {
		t.Fatalf("tablas: %v", partes)
	}
	// Exportar es llevárselo todo: el rango pedido no deja fuera ni lo viejo ni lo futuro.
	if f.gotFrom.Year() > 1 || f.gotTo.Year() < 9999 {
		t.Fatalf("rango: %v → %v", f.gotFrom, f.gotTo)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendar_test

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Una extensión X- en el VEVENT principal de un evento: se pone, sobrevive a una edición
// del formulario (no es de la API), se quita, y lo que no es una X- ni un valor de una
// línea no entra.
func TestExtensionDeUnEvento(t *testing.T) {
	repo, cerrar := abrirLocal(t, t.TempDir())
	defer cerrar()
	ctx := t.Context()
	cal, err := repo.CreateCalendar(ctx, api.CalendarInput{Name: "casa", DisplayName: "Casa", Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	inicio := time.Date(2026, 12, 2, 9, 0, 0, 0, time.UTC)
	ev, err := repo.CreateEvent(ctx, cal.ID, api.EventInput{Summary: "Revisión", Start: inicio, End: inicio.Add(time.Hour), TZID: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	datos := func() string {
		t.Helper()
		os, err := repo.ObjetosCrudos(ctx, cal.ID)
		if err != nil || len(os) != 1 {
			t.Fatal(os, err)
		}
		return os[0].Datos
	}
	if err := repo.PonerExtension(ctx, ev.ID, "x-sytue-reunion", "google-meet"); err != nil {
		t.Fatal(err)
	}
	if d := datos(); !strings.Contains(d, "X-SYTUE-REUNION:google-meet") || !strings.Contains(d, "SUMMARY:Revisión") {
		t.Fatalf("puesta: %s", d)
	}
	// El formulario guarda otra vez el evento: la extensión se conserva.
	in := ev.Input()
	in.Summary = "Revisión trimestral"
	if _, err := repo.UpdateEvent(ctx, ev.ID, in); err != nil {
		t.Fatal(err)
	}
	if d := datos(); !strings.Contains(d, "X-SYTUE-REUNION:google-meet") || !strings.Contains(d, "SUMMARY:Revisión trimestral") {
		t.Fatalf("tras editar: %s", d)
	}
	if err := repo.PonerExtension(ctx, ev.ID, "X-SYTUE-REUNION", ""); err != nil {
		t.Fatal(err)
	}
	if d := datos(); strings.Contains(d, "X-SYTUE-REUNION") {
		t.Fatalf("quitada: %s", d)
	}
	for _, mal := range [][2]string{{"SUMMARY", "x"}, {"X-", "x"}, {"X-UNA;COSA", "x"}, {"X-BIEN", "dos\r\nlineas"}} {
		if err := repo.PonerExtension(ctx, ev.ID, mal[0], mal[1]); err == nil {
			t.Errorf("%q=%q entró", mal[0], mal[1])
		}
	}
	if err := repo.PonerExtension(ctx, "no-existe", "X-SYTUE-REUNION", "teams"); err == nil {
		t.Error("en un evento que no existe")
	}
}

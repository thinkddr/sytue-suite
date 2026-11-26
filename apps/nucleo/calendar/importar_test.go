// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendar_test

import (
	"context"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// abrirLocal abre el repositorio de Calendar, sin tocarlo, sobre el puerto en claro de
// Sytue Office (apps/nucleo/local/almacen), como hace `cmd-local/calendar`.
func abrirLocal(t *testing.T, dir string) (*calendar.Repository, func()) {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), dir, "calendar", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := calendar.New(rec.Config)
	if err != nil {
		rec.Cerrar()
		t.Fatal(err)
	}
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		t.Fatal("preparar:", err)
	}
	return repo, func() { repo.Close(); rec.Cerrar() }
}

const icsDePrueba = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\n" +
	"BEGIN:VEVENT\r\nUID:semanal@prueba\r\nDTSTAMP:20260101T000000Z\r\nDTSTART:20260105T090000Z\r\nDTEND:20260105T100000Z\r\nSUMMARY:Reunión semanal\r\nRRULE:FREQ=WEEKLY;COUNT=4\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:suelto@prueba\r\nDTSTAMP:20260101T000000Z\r\nDTSTART:20260107T120000Z\r\nDTEND:20260107T130000Z\r\nSUMMARY:Comida\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestCalendarLocalSobreviveAReabrirEImportaICS(t *testing.T) {
	dir := t.TempDir()
	repo, cerrar := abrirLocal(t, dir)
	ctx := t.Context()
	cal, err := repo.CreateCalendar(ctx, api.CalendarInput{Name: "casa", DisplayName: "Casa", Timezone: "Europe/Madrid"})
	if err != nil {
		t.Fatal(err)
	}
	inicio := time.Date(2026, 1, 20, 18, 0, 0, 0, time.UTC)
	if _, err := repo.CreateEvent(ctx, cal.ID, api.EventInput{Summary: "Dentista", Start: inicio, End: inicio.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	res, err := repo.ImportICS(ctx, cal.ID, []byte(icsDePrueba))
	if err != nil || res.Importados != 2 || res.Omitidos != 0 || res.Fallidos != 0 {
		t.Fatal("importar", res, err)
	}
	cerrar()

	repo2, cerrar2 := abrirLocal(t, dir)
	defer cerrar2()
	cals, err := repo2.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	encontrado := false
	for _, c := range cals {
		encontrado = encontrado || c.ID == cal.ID
	}
	if !encontrado {
		t.Fatal("el calendario no sobrevive a reabrir", cals)
	}
	desde, hasta := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	inst, err := repo2.Instances(ctx, cal.ID, desde, hasta)
	if err != nil {
		t.Fatal(err)
	}
	// 1 creado + 4 de la serie semanal + 1 suelto.
	if len(inst) != 6 {
		t.Fatalf("instancias tras reabrir = %d, se esperaban 6: %+v", len(inst), inst)
	}
	// Importar otra vez el mismo fichero no duplica.
	res, err = repo2.ImportICS(ctx, cal.ID, []byte(icsDePrueba))
	if err != nil || res.Importados != 0 || res.Omitidos != 2 {
		t.Fatal("reimportar", res, err)
	}
	if _, err := repo2.ImportICS(ctx, cal.ID, []byte("esto no es un calendario")); err == nil {
		t.Fatal("un fichero ilegible debería fallar")
	}
}

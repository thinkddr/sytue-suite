// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// Los recordatorios que se ponen por REST acaban en la cola de avisos
// (`/reminders/due`) igual que los que pone la web por CalDAV, y editarlos no se lleva
// por delante el aviso por correo que puso otro cliente.
func TestRecordatoriosPorRESTLlegaALaColaYConservaLosAjenos(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour}, st, nil)
	ctx := ctxDe(ownerSub, ownerEmail)
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: ownerSub, Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}

	inicio := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)
	in := nuevoEvento(cal.ID, "rec-1")
	in.DTStart, in.DTEnd = inicio, inicio.Add(time.Hour)
	in.Reminders = &[]int{15}
	ev, err := svc.CreateEvent(ctx, in)
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if got, err := svc.EventReminders(ctx, ev); err != nil || !reflect.DeepEqual(got, []int{15}) {
		t.Fatalf("EventReminders tras crear = %v, %v; quiero [15]", got, err)
	}
	vencidos, err := svc.DueReminders(ctx, ownerSub, inicio, nil, 50)
	if err != nil || len(vencidos) != 1 || !vencidos[0].TriggerAt.Equal(inicio.Add(-15*time.Minute)) {
		t.Fatalf("la cola no ve el recordatorio creado por REST: %v %+v", err, vencidos)
	}

	// Otro cliente CalDAV añade un aviso por correo al mismo evento.
	obj, err := st.GetObjectByUID(ctx, cal.ID, "rec-1")
	if err != nil {
		t.Fatal(err)
	}
	conCorreo := strings.Replace(obj.Data, "END:VEVENT",
		"BEGIN:VALARM\r\nACTION:EMAIL\r\nTRIGGER:-PT30M\r\nEND:VALARM\r\nEND:VEVENT", 1)
	if _, err := svc.PutObject(ctx, cal.ID, conCorreo, obj.ETag, false); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	// Editar por REST cambiando el recordatorio: el de correo se queda.
	in.Reminders = &[]int{60}
	ev, err = svc.UpdateEvent(ctx, in)
	if err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	if got, _ := svc.EventReminders(ctx, ev); !reflect.DeepEqual(got, []int{60}) {
		t.Fatalf("EventReminders tras editar = %v; quiero [60]", got)
	}
	vencidos, err = svc.DueReminders(ctx, ownerSub, inicio, nil, 50)
	if err != nil || len(vencidos) != 2 {
		t.Fatalf("la cola tras editar: %v %+v (quiero el de 60 min y el de correo)", err, vencidos)
	}
	acciones := map[string]time.Time{}
	for _, v := range vencidos {
		acciones[v.Action] = v.TriggerAt
	}
	if !acciones["DISPLAY"].Equal(inicio.Add(-time.Hour)) || !acciones["EMAIL"].Equal(inicio.Add(-30*time.Minute)) {
		t.Fatalf("disparos inesperados: %+v", vencidos)
	}

	// Un cliente que no sabe de recordatorios (nil) no toca ninguno.
	in.Reminders = nil
	in.Summary = "otro título"
	if ev, err = svc.UpdateEvent(ctx, in); err != nil {
		t.Fatalf("UpdateEvent sin recordatorios: %v", err)
	}
	if got, _ := svc.EventReminders(ctx, ev); !reflect.DeepEqual(got, []int{60}) {
		t.Fatalf("editar sin recordatorios se los ha llevado: %v", got)
	}

	// Fuera de rango: 400, no un recurso con basura.
	in.Reminders = &[]int{-5}
	if _, err := svc.UpdateEvent(ctx, in); !service.IsValidationError(err) {
		t.Fatalf("un recordatorio negativo debería ser un error de validación: %v", err)
	}
}

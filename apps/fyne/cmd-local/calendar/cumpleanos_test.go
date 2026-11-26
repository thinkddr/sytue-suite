// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/lector"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

func abrir[T almacen.Inicializable](t *testing.T, dir, producto string, nuevo func(*almacen.Recursos) (T, error)) T {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), dir, producto, "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rec.Cerrar)
	repo, err := nuevo(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		t.Fatal(err)
	}
	return repo
}

// El calendario de cumpleaños sale de Contacts, de solo lectura, con la edad cuando se
// sabe el año y el 29 de febrero el 28 en los años que no son bisiestos.
func TestCalendarioDeCumpleanos(t *testing.T) {
	ctx := t.Context()
	dirContactos := t.TempDir()
	cts := abrir(t, dirContactos, "contacts", func(r *almacen.Recursos) (*contacts.Repository, error) { return contacts.New(r.Config) })
	t.Cleanup(cts.Close)
	cal := abrir(t, t.TempDir(), "calendar", func(r *almacen.Recursos) (*privateCalendar.Repository, error) { return privateCalendar.New(r.Config) })
	t.Cleanup(cal.Close)

	datos := &conCumpleanos{
		Repository: cal, lector: lector.Nuevo(dirContactos, "instalacion-1"),
		nombre: func() string { return "Cumpleaños" },
		titulo: func(n string, edad int) string { return fmt.Sprintf("%s %d", n, edad) },
	}
	if l, _ := datos.List(ctx); len(l) != 0 {
		t.Fatal("sin cumpleaños no hay calendario", l)
	}
	lb, err := cts.CreateAddressBook(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cts.Create(ctx, lb.ID, api.ContactInput{FormattedName: "Ada", Birthday: "1815-12-10"}); err != nil {
		t.Fatal(err)
	}
	if _, err := cts.Create(ctx, lb.ID, api.ContactInput{FormattedName: "Bisiesta", BirthdayNoYear: "--02-29"}); err != nil {
		t.Fatal(err)
	}
	datos.leidos = time.Time{} // sin esperar a que caduque lo leído
	l, err := datos.List(ctx)
	if err != nil || len(l) != 1 || l[0].ID != idCumpleanos || !l[0].ReadOnly {
		t.Fatal("el calendario de cumpleaños", l, err)
	}
	svc, err := api.NewCalendars("", nil, nil).WithPrivateData(datos)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := svc.Instances(ctx, idCumpleanos, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(inst) != 2 {
		t.Fatal("ocurrencias", inst, err)
	}
	if !inst[0].Start.Equal(time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)) || !inst[0].Event.AllDay || inst[0].Event.Summary != "Bisiesta 0" {
		t.Fatal("el 29 de febrero cae el 28 en 2026", inst[0])
	}
	if inst[1].Event.Summary != "Ada 211" {
		t.Fatal("con la edad", inst[1].Event.Summary)
	}
	if ev, err := svc.GetEvent(ctx, inst[1].Event.ID); err != nil || ev.Summary == "" {
		t.Fatal("la vista rápida lo pide por id", ev, err)
	}
	if _, err := svc.CreateEvent(ctx, idCumpleanos, api.EventInput{Summary: "x"}); err == nil {
		t.Fatal("en el calendario de cumpleaños no se escribe")
	}
}

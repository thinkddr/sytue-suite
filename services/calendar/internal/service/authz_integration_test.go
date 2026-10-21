// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Verja de autorización del servicio de Calendar, contra un PostgreSQL real.
//
// Por qué existe: hasta ahora esta capa no tenía NINGÚN test. Los de la capa HTTP
// usan un fake que SUSTITUYE al servicio, así que comprueban el mapeo de un error
// a su código HTTP —que ErrReadOnly sale como 403— pero nunca ejecutan la lógica
// que decide si ese error toca. Y el store prueba `CalendarAccess` aisladamente,
// que responde «qué permiso tiene este usuario», no «esta operación preguntó».
// Entre ambas quedaba sin cubrir justo lo que importa: que cada escritura pase por
// la verja. Un método nuevo que olvide llamarla compila, pasa los tests de HTTP y
// pasa los del store.
//
// Se prueban las tres respuestas y su diferencia:
//   - dueño y writer escriben;
//   - reader lee pero al escribir recibe ErrReadOnly (403);
//   - un extraño recibe ErrNotFound (404) hasta para leer — nunca ErrReadOnly,
//     porque un 403 confirmaría que el calendario existe y eso permite enumerar.
package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

const (
	ownerSub   = "owner-1"
	ownerEmail = "owner@sytue.eu"
	readerSub  = "reader-1"
	readerMail = "reader@sytue.eu"
	writerSub  = "writer-1"
	writerMail = "writer@sytue.eu"
	extranoSub = "extrano-1"
	extranoEml = "extrano@sytue.eu"
)

// ctxDe monta el contexto tal y como lo recibe el servicio en producción: el
// tenant que fija el middleware y las claims del token, de donde `caller()` saca
// el sub y el email con los que se resuelve el acceso.
func ctxDe(sub, email string) context.Context {
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	return authn.Into(ctx, authn.Claims{Subject: sub, Email: email})
}

func nuevoEvento(calID uuid.UUID, uid string) service.EventInput {
	inicio := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	return service.EventInput{
		CalendarID: calID,
		UID:        uid,
		Summary:    "reunión",
		Organizer:  ownerEmail,
		DTStart:    inicio,
		DTEnd:      inicio.Add(time.Hour),
	}
}

// montar deja un calendario del dueño con un evento dentro, compartido con un
// reader y un writer. Devuelve el servicio, el id del calendario y el del evento.
func montar(t *testing.T) (*service.Service, uuid.UUID, uuid.UUID) {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour}, st, nil)

	ctx := ctxDe(ownerSub, ownerEmail)
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{
		OwnerSub: ownerSub, Name: "trabajo", DisplayName: "Trabajo",
	})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	ev, err := svc.CreateEvent(ctx, nuevoEvento(cal.ID, "uid-base"))
	if err != nil {
		t.Fatalf("CreateEvent del dueño: %v", err)
	}
	if _, err := st.CreateCalendarShare(ctx, cal.ID, ownerSub, readerMail, domain.PermReader); err != nil {
		t.Fatalf("compartir con el reader: %v", err)
	}
	if _, err := st.CreateCalendarShare(ctx, cal.ID, ownerSub, writerMail, domain.PermWriter); err != nil {
		t.Fatalf("compartir con el writer: %v", err)
	}
	return svc, cal.ID, ev.ID
}

func TestVerjaEscrituraReaderNoEscribe(t *testing.T) {
	svc, calID, evID := montar(t)
	ctx := ctxDe(readerSub, readerMail)

	// Crear.
	if _, err := svc.CreateEvent(ctx, nuevoEvento(calID, "uid-del-reader")); !errors.Is(err, domain.ErrReadOnly) {
		t.Errorf("CreateEvent de un reader = %v, quería ErrReadOnly", err)
	}
	// Editar un evento que SÍ ve.
	in := nuevoEvento(calID, "uid-base")
	in.Summary = "secuestrado"
	if _, err := svc.UpdateEvent(ctx, in); !errors.Is(err, domain.ErrReadOnly) {
		t.Errorf("UpdateEvent de un reader = %v, quería ErrReadOnly", err)
	}
	// Borrar.
	if err := svc.DeleteEvent(ctx, evID); !errors.Is(err, domain.ErrReadOnly) {
		t.Errorf("DeleteEvent de un reader = %v, quería ErrReadOnly", err)
	}
}

func TestVerjaLecturaReaderSiLee(t *testing.T) {
	svc, calID, evID := montar(t)
	ctx := ctxDe(readerSub, readerMail)

	desde := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	insts, err := svc.ListInstances(ctx, calID, desde, desde.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("ListInstances de un reader: %v", err)
	}
	if len(insts) == 0 {
		t.Error("el reader no ve ninguna instancia del calendario que le han compartido")
	}
	if _, err := svc.GetEvent(ctx, evID); err != nil {
		t.Errorf("GetEvent de un reader: %v", err)
	}
}

func TestVerjaEscrituraWriterSiEscribe(t *testing.T) {
	svc, calID, _ := montar(t)
	ctx := ctxDe(writerSub, writerMail)

	propio, err := svc.CreateEvent(ctx, nuevoEvento(calID, "uid-del-writer"))
	if err != nil {
		t.Fatalf("CreateEvent de un writer: %v", err)
	}
	in := nuevoEvento(calID, "uid-base")
	in.Summary = "editado por el writer"
	if _, err := svc.UpdateEvent(ctx, in); err != nil {
		t.Errorf("UpdateEvent de un writer: %v", err)
	}
	// OJO con el id que se usa aquí: el identificador estable de un evento es su
	// UID dentro del calendario (modelo CalDAV), no `events.id`. `UpdateEvent`
	// reemplaza la fila del evento maestro —DELETE + INSERT, store/events.go:142—
	// así que editar CAMBIA ese id. Por eso se borra el evento que este writer
	// acaba de crear y no el de `montar`, cuyo id ya no vale tras la edición de
	// arriba: usarlo daba ErrNotFound y parecía un fallo de la verja.
	if err := svc.DeleteEvent(ctx, propio.ID); err != nil {
		t.Errorf("DeleteEvent de un writer: %v", err)
	}
}

// El extraño es el caso que más importa: NADA, ni siquiera leer, y siempre con
// ErrNotFound. Si aquí saliera ErrReadOnly se estaría confirmando la existencia
// del calendario a quien no debería saber que existe.
func TestVerjaExtranoNiLeeNiEscribeYNuncaSabeQueExiste(t *testing.T) {
	svc, calID, evID := montar(t)
	ctx := ctxDe(extranoSub, extranoEml)

	desde := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if _, err := svc.ListInstances(ctx, calID, desde, desde.Add(48*time.Hour)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ListInstances de un extraño = %v, quería ErrNotFound", err)
	}
	if _, err := svc.GetEvent(ctx, evID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetEvent de un extraño = %v, quería ErrNotFound", err)
	}
	if _, err := svc.CreateEvent(ctx, nuevoEvento(calID, "uid-del-extrano")); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("CreateEvent de un extraño = %v, quería ErrNotFound (no ErrReadOnly: filtraría existencia)", err)
	}
	if err := svc.DeleteEvent(ctx, evID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("DeleteEvent de un extraño = %v, quería ErrNotFound", err)
	}
}

// Revocar el share corta el acceso de verdad, no solo el listado.
func TestVerjaRevocarCortaElAcceso(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour}, st, nil)

	ctxOwner := ctxDe(ownerSub, ownerEmail)
	cal, err := st.CreateCalendar(ctxOwner, store.NewCalendar{
		OwnerSub: ownerSub, Name: "trabajo", DisplayName: "Trabajo",
	})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	sh, err := st.CreateCalendarShare(ctxOwner, cal.ID, ownerSub, writerMail, domain.PermWriter)
	if err != nil {
		t.Fatalf("compartir: %v", err)
	}

	ctxWriter := ctxDe(writerSub, writerMail)
	if _, err := svc.CreateEvent(ctxWriter, nuevoEvento(cal.ID, "uid-antes")); err != nil {
		t.Fatalf("el writer debería poder crear antes de la revocación: %v", err)
	}

	if err := st.RevokeCalendarShare(ctxOwner, cal.ID, sh.ID, ownerSub); err != nil {
		t.Fatalf("revocar: %v", err)
	}

	if _, err := svc.CreateEvent(ctxWriter, nuevoEvento(cal.ID, "uid-despues")); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("tras revocar, CreateEvent = %v, quería ErrNotFound", err)
	}
	desde := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if _, err := svc.ListInstances(ctxWriter, cal.ID, desde, desde.Add(48*time.Hour)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("tras revocar, ListInstances = %v, quería ErrNotFound", err)
	}
}

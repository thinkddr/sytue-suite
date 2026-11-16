// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Posponer y silenciar conversaciones (migración 0036): el hilo sale de Recibidos al
// buzón de pospuestos y vuelve al vencer (sin leer), a mano o con una respuesta
// nueva; silenciado, archiva lo que había y manda al Archivo lo que llegue. Todo
// del dueño y de su organización.
package store_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// buzonDe devuelve el buzón de un rol o falla el test.
func buzonDe(t *testing.T, ctx context.Context, st *store.Store, acc domain.Account, role domain.SpecialUse) uuid.UUID {
	t.Helper()
	mb, err := st.MailboxByRole(ctx, acc.ID, role)
	if err != nil {
		t.Fatalf("MailboxByRole(%s): %v", role, err)
	}
	return mb.ID
}

// dondeEsta devuelve el buzón de cada mensaje del hilo, por id.
func dondeEsta(t *testing.T, ctx context.Context, st *store.Store, acc domain.Account, threadID uuid.UUID) map[uuid.UUID]domain.Message {
	t.Helper()
	msgs, err := st.ListThreadMessages(ctx, acc.ID, threadID, 0)
	if err != nil {
		t.Fatalf("ListThreadMessages: %v", err)
	}
	out := map[uuid.UUID]domain.Message{}
	for _, m := range msgs {
		out[m.ID] = m
	}
	return out
}

func TestSnoozeSacaDeRecibidosYVuelveAlVencerSinLeer(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ada := seedAccount(ctx, t, st, "ada@sytue.eu")
	inbox := buzonDe(t, ctx, st, ada, domain.RoleInbox)
	archive := buzonDe(t, ctx, st, ada, domain.RoleArchive)

	primero := seedMessage(ctx, t, st, ada, inbox, "Presupuesto", "eva@fuera.eu", []string{domain.FlagSeen})
	respuesta := seedReply(ctx, t, st, ada, inbox, "Re: Presupuesto", "eva@fuera.eu", primero)
	if err := st.SetFlags(ctx, ada.ID, respuesta.ID, domain.FlagSeen, true); err != nil {
		t.Fatalf("SetFlags: %v", err)
	}
	// Lo archivado del hilo no se pospone: posponer saca de la Entrada y nada más.
	viejo := seedReply(ctx, t, st, ada, archive, "Re: Presupuesto", "ada@sytue.eu", primero)

	until := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	sn, err := st.SnoozeThread(ctx, ada.ID, primero.ThreadID, until)
	if err != nil {
		t.Fatalf("SnoozeThread: %v", err)
	}
	if sn.ThreadID != primero.ThreadID || !sn.Until.Equal(until) {
		t.Fatalf("snooze = %+v", sn)
	}
	snoozed := buzonDe(t, ctx, st, ada, domain.RoleSnoozed)
	hilo := dondeEsta(t, ctx, st, ada, primero.ThreadID)
	if hilo[primero.ID].MailboxID != snoozed || hilo[respuesta.ID].MailboxID != snoozed {
		t.Fatalf("lo de Recibidos no está en pospuestos: %+v", hilo)
	}
	if hilo[viejo.ID].MailboxID != archive {
		t.Fatalf("lo archivado se movió")
	}
	if lista, _ := st.ListSnoozes(ctx, ada.ID); len(lista) != 1 || lista[0].ThreadID != primero.ThreadID {
		t.Fatalf("ListSnoozes = %+v", lista)
	}
	if est, err := st.ThreadState(ctx, ada.ID, primero.ThreadID); err != nil || est.SnoozedUntil == nil || !est.SnoozedUntil.Equal(until) {
		t.Fatalf("ThreadState = %+v, %v", est, err)
	}

	// Antes de la fecha no despierta nada.
	if d, err := st.DespertarPospuestos(context.Background(), time.Now(), 10); err != nil || len(d) != 0 {
		t.Fatalf("DespertarPospuestos antes de tiempo = %+v, %v", d, err)
	}
	// Al vencer vuelve a Recibidos, y lo más reciente sin leer.
	d, err := st.DespertarPospuestos(context.Background(), until.Add(time.Second), 10)
	if err != nil || len(d) != 1 || d[0].Tenant != "acme" || d[0].Subject != "sub-ada@sytue.eu" {
		t.Fatalf("DespertarPospuestos = %+v, %v", d, err)
	}
	hilo = dondeEsta(t, ctx, st, ada, primero.ThreadID)
	if hilo[primero.ID].MailboxID != inbox || hilo[respuesta.ID].MailboxID != inbox {
		t.Fatalf("no volvió a Recibidos: %+v", hilo)
	}
	if slices.Contains(hilo[respuesta.ID].Flags, domain.FlagSeen) {
		t.Fatalf("lo más reciente vuelve leído: %v", hilo[respuesta.ID].Flags)
	}
	if !slices.Contains(hilo[primero.ID].Flags, domain.FlagSeen) {
		t.Fatalf("lo anterior perdió el leído: %v", hilo[primero.ID].Flags)
	}
	if lista, _ := st.ListSnoozes(ctx, ada.ID); len(lista) != 0 {
		t.Fatalf("la posposición sigue: %+v", lista)
	}
	// Ya despierto, no se despierta dos veces.
	if d, _ := st.DespertarPospuestos(context.Background(), until.Add(time.Hour), 10); len(d) != 0 {
		t.Fatalf("despertó dos veces: %+v", d)
	}
}

// idsDeLaCarpeta recorre la carpeta entera página a página (de `pagina` en
// `pagina`), con el cursor keyset que da el servicio: la fecha de orden y el id del
// último de cada página.
func idsDeLaCarpeta(t *testing.T, ctx context.Context, st *store.Store, mbox uuid.UUID, pagina int) []uuid.UUID {
	t.Helper()
	var out []uuid.UUID
	var antes time.Time
	antesID := uuid.Nil
	for vuelta := 0; vuelta < 50; vuelta++ {
		msgs, err := st.ListMessages(ctx, mbox, pagina, antes, antesID)
		if err != nil {
			t.Fatalf("ListMessages: %v", err)
		}
		for _, m := range msgs {
			out = append(out, m.ID)
		}
		if len(msgs) < pagina {
			return out
		}
		ultimo := msgs[len(msgs)-1]
		antes, antesID = ultimo.FechaDeOrden(), ultimo.ID
	}
	t.Fatalf("la paginación no acaba")
	return nil
}

// Como en Gmail: lo que vuelve de «Pospuestos» al vencer sube ARRIBA de Recibidos,
// ordenado por la hora de vuelta, y no a su sitio cronológico (varias páginas más
// abajo, donde nadie lo ve). La paginación por cursor sigue sin repetir ni saltarse
// nada, y lo que se trae a mano antes de tiempo vuelve a su sitio.
func TestLoPospuestoVuelveArribaDeRecibidosAlVencer(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ada := seedAccount(ctx, t, st, "ada@sytue.eu")
	inbox := buzonDe(t, ctx, st, ada, domain.RoleInbox)

	base := time.Now().Add(-10 * 24 * time.Hour).UTC().Truncate(time.Second)
	viejo := seedMsgAt(ctx, t, st, ada, inbox, "Factura de marzo", "eva@fuera.eu", base)
	manual := seedMsgAt(ctx, t, st, ada, inbox, "Lo traigo a mano", "eva@fuera.eu", base.Add(time.Hour))
	var recientes []domain.Message
	for i := range 5 {
		recientes = append(recientes, seedMsgAt(ctx, t, st, ada, inbox, "Nuevo", "luis@fuera.eu", base.Add(time.Duration(i+2)*24*time.Hour)))
	}

	until := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	for _, m := range []domain.Message{viejo, manual} {
		if _, err := st.SnoozeThread(ctx, ada.ID, m.ThreadID, until); err != nil {
			t.Fatalf("SnoozeThread: %v", err)
		}
	}
	// El primero vuelve al vencer (el trabajador pasa unos segundos tarde); el
	// segundo lo trae la persona antes de tiempo.
	if d, err := st.DespertarPospuestos(context.Background(), until.Add(30*time.Second), 1); err != nil || len(d) != 1 {
		t.Fatalf("DespertarPospuestos = %+v, %v", d, err)
	}
	vuelto, traido := viejo.ID, manual.ID
	if est, _ := st.ThreadState(ctx, ada.ID, viejo.ThreadID); est.SnoozedUntil != nil {
		// Con límite 1 despierta el que vence antes; los dos vencen a la vez y el
		// orden entre ellos no está fijado: se ajusta lo esperado al que despertó.
		vuelto, traido = manual.ID, viejo.ID
	}
	var traidoHilo uuid.UUID
	for _, m := range []domain.Message{viejo, manual} {
		if m.ID == traido {
			traidoHilo = m.ThreadID
		}
	}
	if err := st.UnsnoozeThread(ctx, ada.ID, traidoHilo); err != nil {
		t.Fatalf("UnsnoozeThread: %v", err)
	}

	// El vuelto encabeza la carpeta, sin leer y con la hora de vuelta (la del
	// posponer, no la del barrido); el traído a mano sigue en su sitio.
	primera, err := st.ListMessages(ctx, inbox, 3, time.Time{}, uuid.Nil)
	if err != nil || len(primera) != 3 {
		t.Fatalf("ListMessages = %d, %v", len(primera), err)
	}
	if primera[0].ID != vuelto {
		t.Fatalf("lo vuelto de Pospuestos no encabeza Recibidos: %v", primera[0].Subject)
	}
	if primera[0].UnsnoozedAt == nil || !primera[0].UnsnoozedAt.Equal(until) {
		t.Fatalf("unsnoozed_at = %v, quiero %v", primera[0].UnsnoozedAt, until)
	}
	if !primera[0].ReceivedAt.Equal(viejo.ReceivedAt) && !primera[0].ReceivedAt.Equal(manual.ReceivedAt) {
		t.Fatalf("la fecha de llegada cambió: %v", primera[0].ReceivedAt)
	}
	if slices.Contains(primera[0].Flags, domain.FlagSeen) {
		t.Fatalf("vuelve leído")
	}
	if primera[1].ID != recientes[4].ID {
		t.Fatalf("tras lo vuelto no sigue lo más reciente: %v", primera[1].Subject)
	}

	// Recorrida a páginas de 2, la carpeta sale entera, sin repetidos, y en orden:
	// lo vuelto, lo reciente de más nuevo a más viejo, y al fondo lo traído a mano y
	// lo más antiguo.
	quiero := []uuid.UUID{vuelto}
	for i := len(recientes) - 1; i >= 0; i-- {
		quiero = append(quiero, recientes[i].ID)
	}
	quiero = append(quiero, traido)
	if got := idsDeLaCarpeta(t, ctx, st, inbox, 2); !slices.Equal(got, quiero) {
		t.Fatalf("orden paginado = %v, quiero %v", got, quiero)
	}

	// La búsqueda ordena igual que la lista.
	res, err := st.SearchMessages(ctx, ada.ID, store.SearchFilter{Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) == 0 || res[0].ID != vuelto {
		t.Fatalf("la búsqueda no pone arriba lo vuelto")
	}

	// Posponerlo otra vez borra la vuelta: si después se trae a mano, va a su sitio.
	var vueltoHilo uuid.UUID
	for _, m := range []domain.Message{viejo, manual} {
		if m.ID == vuelto {
			vueltoHilo = m.ThreadID
		}
	}
	if _, err := st.SnoozeThread(ctx, ada.ID, vueltoHilo, until.Add(time.Hour)); err != nil {
		t.Fatalf("SnoozeThread otra vez: %v", err)
	}
	if err := st.UnsnoozeThread(ctx, ada.ID, vueltoHilo); err != nil {
		t.Fatalf("UnsnoozeThread: %v", err)
	}
	if got := idsDeLaCarpeta(t, ctx, st, inbox, 50); got[0] != recientes[4].ID {
		t.Fatalf("traído a mano tras posponerlo otra vez, sigue arriba")
	}
}

func TestSnoozeCambiarFechaQuitarYNadaQuePosponer(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ada := seedAccount(ctx, t, st, "ada@sytue.eu")
	inbox := buzonDe(t, ctx, st, ada, domain.RoleInbox)
	archive := buzonDe(t, ctx, st, ada, domain.RoleArchive)

	m := seedMessage(ctx, t, st, ada, inbox, "Hola", "eva@fuera.eu", nil)
	if _, err := st.SnoozeThread(ctx, ada.ID, m.ThreadID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SnoozeThread: %v", err)
	}
	// Otra vez, desde pospuestos: solo cambia la fecha.
	nueva := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	if sn, err := st.SnoozeThread(ctx, ada.ID, m.ThreadID, nueva); err != nil || !sn.Until.Equal(nueva) {
		t.Fatalf("cambiar fecha = %+v, %v", sn, err)
	}
	if err := st.UnsnoozeThread(ctx, ada.ID, m.ThreadID); err != nil {
		t.Fatalf("UnsnoozeThread: %v", err)
	}
	// Archivado a mano desde «Pospuestos», el hilo ya no está en espera: el lector no
	// debe decir «Pospuesto hasta…» de un hilo que vive en el Archivo.
	otro := seedMessage(ctx, t, st, ada, inbox, "Otro", "eva@fuera.eu", nil)
	if _, err := st.SnoozeThread(ctx, ada.ID, otro.ThreadID, nueva); err != nil {
		t.Fatalf("SnoozeThread: %v", err)
	}
	if est, _ := st.ThreadState(ctx, ada.ID, otro.ThreadID); est.SnoozedUntil == nil {
		t.Fatalf("recién pospuesto, ThreadState no lo dice")
	}
	if _, err := st.MoveMessages(ctx, ada.ID, []uuid.UUID{otro.ID}, archive); err != nil {
		t.Fatalf("MoveMessages: %v", err)
	}
	if est, err := st.ThreadState(ctx, ada.ID, otro.ThreadID); err != nil || est.SnoozedUntil != nil {
		t.Fatalf("archivado desde Pospuestos sigue pospuesto: %+v, %v", est, err)
	}
	if got := dondeEsta(t, ctx, st, ada, m.ThreadID)[m.ID]; got.MailboxID != inbox {
		t.Fatalf("quitar la posposición no lo devolvió a Recibidos")
	}
	if err := st.UnsnoozeThread(ctx, ada.ID, m.ThreadID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("quitar dos veces: err = %v", err)
	}

	// Un hilo sin nada en Recibidos no se pospone.
	archivado := seedMessage(ctx, t, st, ada, archive, "Viejo", "eva@fuera.eu", nil)
	if _, err := st.SnoozeThread(ctx, ada.ID, archivado.ThreadID, time.Now().Add(time.Hour)); !errors.Is(err, domain.ErrNothingToSnooze) {
		t.Fatalf("posponer sin nada en Recibidos: err = %v", err)
	}
	if _, err := st.SnoozeThread(ctx, ada.ID, m.ThreadID, time.Now().Add(-time.Hour)); !errors.Is(err, domain.ErrInvalidSnooze) {
		t.Fatalf("posponer al pasado: err = %v", err)
	}
}

// Una carpeta propia llamada «Snoozed» no se adopta: el buzón de pospuestos se crea
// con otro nombre y la carpeta queda como estaba.
func TestSnoozeNoAdoptaUnaCarpetaPropiaConElMismoNombre(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ada := seedAccount(ctx, t, st, "ada@sytue.eu")
	inbox := buzonDe(t, ctx, st, ada, domain.RoleInbox)
	propia, err := st.CreateMailbox(ctx, ada.ID, nil, "snoozed")
	if err != nil {
		t.Fatalf("CreateMailbox: %v", err)
	}
	m := seedMessage(ctx, t, st, ada, inbox, "Hola", "eva@fuera.eu", nil)
	if _, err := st.SnoozeThread(ctx, ada.ID, m.ThreadID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SnoozeThread: %v", err)
	}
	snoozed := buzonDe(t, ctx, st, ada, domain.RoleSnoozed)
	if snoozed == propia.ID {
		t.Fatalf("adoptó la carpeta propia")
	}
}

func TestUnaRespuestaNuevaDevuelveElHiloPospuesto(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ada := seedAccount(ctx, t, st, "ada@sytue.eu")
	inbox := buzonDe(t, ctx, st, ada, domain.RoleInbox)

	m := seedMessage(ctx, t, st, ada, inbox, "Reunión", "eva@fuera.eu", nil)
	if _, err := st.SnoozeThread(ctx, ada.ID, m.ThreadID, time.Now().Add(24*time.Hour)); err != nil {
		t.Fatalf("SnoozeThread: %v", err)
	}
	r := &domain.Message{
		AccountID: ada.ID, MailboxID: inbox, Subject: "Re: Reunión", FromAddr: "eva@fuera.eu",
		MessageID: uuid.NewString() + "@fuera.eu", InReplyTo: m.MessageID,
		BlobKey: "messages/x.eml", SizeBytes: 10, ReceivedAt: time.Now().UTC().Add(time.Minute),
	}
	silenciado, err := st.InsertMessageEntrante(ctx, r)
	if err != nil || silenciado {
		t.Fatalf("InsertMessageEntrante = %v, %v", silenciado, err)
	}
	hilo := dondeEsta(t, ctx, st, ada, m.ThreadID)
	if hilo[m.ID].MailboxID != inbox || hilo[r.ID].MailboxID != inbox {
		t.Fatalf("la respuesta no trajo el hilo de vuelta: %+v", hilo)
	}
	if lista, _ := st.ListSnoozes(ctx, ada.ID); len(lista) != 0 {
		t.Fatalf("la posposición sigue tras la respuesta: %+v", lista)
	}
}

func TestMuteArchivaYMandaAlArchivoLoNuevo(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ada := seedAccount(ctx, t, st, "ada@sytue.eu")
	inbox := buzonDe(t, ctx, st, ada, domain.RoleInbox)
	archive := buzonDe(t, ctx, st, ada, domain.RoleArchive)

	m := seedMessage(ctx, t, st, ada, inbox, "Cadena", "eva@fuera.eu", nil)
	n, err := st.MuteThread(ctx, ada.ID, m.ThreadID, true)
	if err != nil || n != 1 {
		t.Fatalf("MuteThread = %d, %v; quiero 1 archivado", n, err)
	}
	if got := dondeEsta(t, ctx, st, ada, m.ThreadID)[m.ID]; got.MailboxID != archive {
		t.Fatalf("silenciar no archivó")
	}
	if est, _ := st.ThreadState(ctx, ada.ID, m.ThreadID); !est.Muted {
		t.Fatalf("ThreadState no dice silenciado")
	}

	nueva := func() *domain.Message {
		return &domain.Message{
			AccountID: ada.ID, MailboxID: inbox, Subject: "Re: Cadena", FromAddr: "eva@fuera.eu",
			MessageID: uuid.NewString() + "@fuera.eu", InReplyTo: m.MessageID,
			BlobKey: "messages/y.eml", SizeBytes: 10,
		}
	}
	r := nueva()
	silenciado, err := st.InsertMessageEntrante(ctx, r)
	if err != nil || !silenciado || r.MailboxID != archive {
		t.Fatalf("respuesta a silenciado: silenciado=%v buzón=%v err=%v", silenciado, r.MailboxID, err)
	}

	// Sin silenciar, lo siguiente entra en Recibidos como siempre.
	if _, err := st.MuteThread(ctx, ada.ID, m.ThreadID, false); err != nil {
		t.Fatalf("MuteThread(false): %v", err)
	}
	r = nueva()
	if silenciado, err := st.InsertMessageEntrante(ctx, r); err != nil || silenciado || r.MailboxID != inbox {
		t.Fatalf("tras dejar de silenciar: silenciado=%v buzón=%v err=%v", silenciado, r.MailboxID, err)
	}
}

func TestSnoozeYMuteSonDelDuenoYDeSuOrganizacion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	acme := tenantCtx("acme")
	otra := tenantCtx("otra")
	ada := seedAccount(acme, t, st, "ada@sytue.eu")
	bob := seedAccount(acme, t, st, "bob@sytue.eu")
	eva := seedAccount(otra, t, st, "eva@otra.eu")
	m := seedMessage(acme, t, st, ada, buzonDe(t, acme, st, ada, domain.RoleInbox), "Mío", "x@fuera.eu", nil)
	until := time.Now().Add(time.Hour)

	if _, err := st.SnoozeThread(acme, bob.ID, m.ThreadID, until); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("posponer el hilo de otra cuenta: err = %v", err)
	}
	if _, err := st.SnoozeThread(otra, eva.ID, m.ThreadID, until); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("posponer el hilo de otra organización: err = %v", err)
	}
	if _, err := st.MuteThread(acme, bob.ID, m.ThreadID, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("silenciar el hilo de otra cuenta: err = %v", err)
	}
	if _, err := st.ThreadState(acme, bob.ID, m.ThreadID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("estado del hilo de otra cuenta: err = %v", err)
	}
	if _, err := st.SnoozeThread(acme, ada.ID, m.ThreadID, until); err != nil {
		t.Fatalf("SnoozeThread: %v", err)
	}
	if err := st.UnsnoozeThread(acme, bob.ID, m.ThreadID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("quitar la posposición de otra cuenta: err = %v", err)
	}
	if lista, _ := st.ListSnoozes(acme, bob.ID); len(lista) != 0 {
		t.Fatalf("Bob ve las posposiciones de Ada: %+v", lista)
	}
	if lista, _ := st.ListSnoozes(otra, ada.ID); len(lista) != 0 {
		t.Fatalf("otra organización ve las posposiciones de Ada: %+v", lista)
	}
}

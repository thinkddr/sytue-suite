// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

type tareas struct {
	t   *testing.T
	st  *store.Store
	ctx context.Context
}

func (x tareas) crear(nb uuid.UUID, sub, titulo string, padre *uuid.UUID) domain.Note {
	x.t.Helper()
	in := domain.NoteInput{Kind: domain.KindTodo, Title: titulo, GroupID: padre}
	if err := in.Normalize(false); err != nil {
		x.t.Fatal(err)
	}
	n, err := x.st.CreateNote(x.ctx, nb, sub, in)
	if err != nil {
		x.t.Fatal(err)
	}
	return n
}

// marcar completa (hecha) o cancela (!hecha) una tarea por el PUT de siempre.
func (x tareas) marcar(n domain.Note, hecha bool) domain.Note {
	x.t.Helper()
	in := domain.NoteInput{Kind: n.Kind, Title: n.Title, Checked: siBool(hecha), Cancelled: siBool(!hecha)}
	if err := in.Normalize(false); err != nil {
		x.t.Fatal(err)
	}
	got, err := x.st.UpdateNote(x.ctx, n.ID, n.OwnerSub, in)
	if err != nil {
		x.t.Fatal(err)
	}
	return got
}

func (x tareas) en(nb uuid.UUID) map[uuid.UUID]domain.Note {
	x.t.Helper()
	ns, err := x.st.ListNotes(x.ctx, nb, "ada")
	if err != nil {
		x.t.Fatal(err)
	}
	out := map[uuid.UUID]domain.Note{}
	for _, n := range ns {
		out[n.ID] = n
	}
	return out
}

func nuevasTareas(t *testing.T) tareas {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	return tareas{t: t, st: store.New(h.Pool), ctx: tenantCtx("acme")}
}

func (x tareas) dia(fecha string, carry bool) domain.Notebook {
	x.t.Helper()
	nb, err := x.st.DailyNotebook(x.ctx, "ada", fecha, fecha, carry)
	if err != nil {
		x.t.Fatal(err)
	}
	return nb
}

// Un subárbol en el que UN nodo está compartido no se mueve entero: trasladarlo lo
// enseñaría en un cuaderno donde nadie lo había compartido, y moverlo a medias
// rompería el grupo.
func TestTrasladoConSubarbolNoMueveLoCompartido(t *testing.T) {
	x := nuevasTareas(t)
	ayer := x.dia("2026-09-30", false)
	padre := x.crear(ayer.ID, "ada", "Padre", nil)
	hija := x.crear(ayer.ID, "ada", "Hija", &padre.ID)
	nieta := x.crear(ayer.ID, "ada", "Nieta", &hija.ID)
	otro := x.crear(ayer.ID, "ada", "Otro padre", nil)
	compartida := x.crear(ayer.ID, "ada", "Hija compartida", &otro.ID)
	if _, _, err := x.st.CompartirNota(x.ctx, compartida.ID, "ada", "bob", false); err != nil {
		t.Fatal(err)
	}
	hoy := x.dia("2026-10-01", true)
	llegaron := x.en(hoy.ID)
	if len(llegaron) != 3 || llegaron[nieta.ID].GroupID == nil || *llegaron[nieta.ID].GroupID != hija.ID ||
		llegaron[padre.ID].GroupID != nil {
		t.Fatalf("el subárbol limpio no llegó entero: %+v", llegaron)
	}
	quedan := x.en(ayer.ID)
	if len(quedan) != 2 || quedan[otro.ID].ID == uuid.Nil || quedan[compartida.ID].ID == uuid.Nil {
		t.Fatalf("el subárbol con una hija compartida se movió: %+v", quedan)
	}
}

// Cancelar saca la tarea del traslado; completar el padre sube a los hijos
// pendientes al primer nivel del día nuevo; y cada traslado suma uno al contador.
func TestTrasladoCanceladasPadresHechosYContador(t *testing.T) {
	x := nuevasTareas(t)
	d1 := x.dia("2026-09-29", false)
	cancelada := x.marcar(x.crear(d1.ID, "ada", "Cancelada", nil), false)
	if cancelada.CancelledAt == nil || cancelada.Checked || cancelada.CheckedAt != nil {
		t.Fatalf("cancelar no deja la marca: %+v", cancelada)
	}
	padre := x.crear(d1.ID, "ada", "Padre", nil)
	hija := x.crear(d1.ID, "ada", "Hija pendiente", &padre.ID)
	padre = x.marcar(padre, true)
	if padre.CheckedAt == nil || padre.CancelledAt != nil {
		t.Fatalf("completar no guarda la fecha: %+v", padre)
	}
	suelta := x.crear(d1.ID, "ada", "Suelta", nil)

	d2 := x.dia("2026-09-30", true)
	llegaron := x.en(d2.ID)
	if len(llegaron) != 2 || llegaron[hija.ID].GroupID != nil || llegaron[suelta.ID].ID == uuid.Nil {
		t.Fatalf("esperaba la suelta y la hija subida a primer nivel: %+v", llegaron)
	}
	quedan := x.en(d1.ID)
	if quedan[cancelada.ID].ID == uuid.Nil || quedan[padre.ID].ID == uuid.Nil {
		t.Fatalf("la cancelada o el padre hecho se movieron: %+v", quedan)
	}

	x.dia("2026-10-01", true)
	var data struct {
		From  string `json:"carried_from"`
		Count int    `json:"carry_count"`
	}
	hoy := x.en(x.dia("2026-10-01", false).ID)
	if err := json.Unmarshal(hoy[suelta.ID].Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Count != 2 || data.From != "2026-09-29" {
		t.Fatalf("contador o día de origen mal tras dos traslados: %+v", data)
	}
}

// GET /tasks no enseña tareas cifradas ni las privadas de otra persona, y cada
// estado devuelve solo lo suyo.
func TestListTasksVerjasYEstados(t *testing.T) {
	x := nuevasTareas(t)
	mio, err := x.st.CreateNotebook(x.ctx, "ada", entradaCuaderno("Lista", domain.VisOrganization))
	if err != nil {
		t.Fatal(err)
	}
	abierta := x.crear(mio.ID, "ada", "Abierta", nil)
	hecha := x.marcar(x.crear(mio.ID, "ada", "Hecha", nil), true)
	cancelada := x.marcar(x.crear(mio.ID, "ada", "Cancelada", nil), false)
	// Bob escribe en el cuaderno de organización de Ada una tarea privada suya.
	in := domain.NoteInput{Kind: domain.KindTodo, Title: "Privada de Bob", Visibility: domain.VisPrivate}
	if err := in.Normalize(false); err != nil {
		t.Fatal(err)
	}
	if _, err := x.st.CreateNote(x.ctx, mio.ID, "bob", in); err != nil {
		t.Fatal(err)
	}
	cif := domain.NotebookInput{Title: "Cifrado", Encrypted: true, Crypto: &domain.CryptoParams{
		V: 1, KDF: "PBKDF2-SHA256", Salt: "c2FsLWRlLXBydWViYS0xMjM0", Iterations: domain.MinKDFIterations, Check: "encv1:AAAA",
	}}
	if err := cif.Normalize(); err != nil {
		t.Fatal(err)
	}
	nbc, err := x.st.CreateNotebook(x.ctx, "ada", cif)
	if err != nil {
		t.Fatal(err)
	}
	cryptoJSON, _ := json.Marshal(nbc.Crypto)
	if _, err := x.st.CreateNote(x.ctx, nbc.ID, "ada", domain.NoteInput{CipherVersion: domain.EncryptedNoteWriteVersion, ExpectedCrypto: cryptoJSON, Kind: domain.KindTodo, ContentCipher: "encv1:AAAA"}); err != nil {
		t.Fatal(err)
	}

	for estado, quiero := range map[string]uuid.UUID{
		store.TareaAbierta: abierta.ID, store.TareaHecha: hecha.ID, store.TareaCancelada: cancelada.ID,
	} {
		ts, err := x.st.ListTasks(x.ctx, "ada", estado, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(ts) != 1 || ts[0].ID != quiero || ts[0].NotebookTitle != "Lista" {
			t.Errorf("%s: %+v", estado, ts)
		}
	}
	// Bob no ve como suyas las tareas de Ada, ni Ada la privada de Bob (arriba).
	ts, err := x.st.ListTasks(x.ctx, "bob", store.TareaAbierta, nil, nil)
	if err != nil || len(ts) != 1 || ts[0].Title != "Privada de Bob" {
		t.Errorf("bob: %+v %v", ts, err)
	}
	if _, err := x.st.ListTasks(x.ctx, "ada", "todas", nil, nil); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("estado desconocido: %v", err)
	}
}

// Posponer lleva la tarea con sus hijas al día elegido, y una compartida no se mueve.
func TestPosponerConSubarbol(t *testing.T) {
	x := nuevasTareas(t)
	hoy := x.dia("2026-10-01", false)
	padre := x.crear(hoy.ID, "ada", "Padre", nil)
	hija := x.crear(hoy.ID, "ada", "Hija", &padre.ID)
	manana, err := x.st.PostponeTask(x.ctx, hija.ID, "ada", "2026-10-02", "Mañana")
	if err != nil || manana.DailyDate != "2026-10-02" {
		t.Fatalf("posponer la hija: %+v %v", manana, err)
	}
	if n := x.en(manana.ID)[hija.ID]; n.ID == uuid.Nil || n.GroupID != nil {
		t.Fatalf("la hija pospuesta no llegó a primer nivel: %+v", n)
	}
	x.crear(hoy.ID, "ada", "Nieta", &padre.ID)
	if _, err := x.st.PostponeTask(x.ctx, padre.ID, "ada", "2026-10-05", "Lunes"); err != nil {
		t.Fatal(err)
	}
	if len(x.en(hoy.ID)) != 0 {
		t.Fatal("el padre se pospuso sin su subárbol")
	}
	sola := x.crear(hoy.ID, "ada", "Compartida", nil)
	if _, _, err := x.st.CompartirNota(x.ctx, sola.ID, "ada", "bob", false); err != nil {
		t.Fatal(err)
	}
	if _, err := x.st.PostponeTask(x.ctx, sola.ID, "ada", "2026-10-02", "Mañana"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("una compartida se pospuso: %v", err)
	}
	if _, err := x.st.PostponeTask(x.ctx, sola.ID, "bob", "2026-10-02", "Mañana"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra persona pospuso una tarea ajena: %v", err)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

func recordatorio(t *testing.T, cuando time.Time, rrule string, despues int) domain.ReminderInput {
	t.Helper()
	in := domain.ReminderInput{DueAt: cuando, RRule: rrule, AfterDays: despues, TZID: "Europe/Madrid"}
	if err := in.Normalize(); err != nil {
		t.Fatal(err)
	}
	return in
}

// Lo vencido avisa UNA vez y a quien lo puso; lo que no toca —en la papelera, en una
// nota que ya no se lee, una tarea hecha— no avisa y no vuelve; una nota cifrada avisa
// sin título; y la nota que repite por calendario se reprograma en vez de apagarse.
func TestRecordatoriosVencenYAvisanUnaVez(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ahora := time.Now().UTC().Truncate(time.Minute)
	antes := ahora.Add(-time.Hour)

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Casa", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	nota := func(titulo string, tipo domain.Kind) domain.Note {
		t.Helper()
		n, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: titulo, Kind: tipo})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	poner := func(n domain.Note, sujeto, rrule string) {
		t.Helper()
		if _, err := st.SetReminder(ctx, n.ID, sujeto, recordatorio(t, antes, rrule, 0)); err != nil {
			t.Fatalf("poner en %q: %v", n.Title, err)
		}
	}
	simple, papelera, diaria, hecha := nota("Llamar al fontanero", domain.KindText),
		nota("Tirada", domain.KindText), nota("Regar", domain.KindText), nota("Hecha", domain.KindTodo)
	compartida := nota("Para Bob", domain.KindText)
	for _, n := range []domain.Note{simple, papelera, hecha} {
		poner(n, "ada", "")
	}
	poner(diaria, "ada", "FREQ=DAILY")
	if _, err := st.UpdateNote(ctx, hecha.ID, "ada", domain.NoteInput{Title: "Hecha", Checked: siBool(true)}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteNote(ctx, papelera.ID, "ada"); err != nil {
		t.Fatal(err)
	}
	// Bob la lee porque se la compartieron; luego se la quitan: ya no se le avisa.
	if _, err := st.SetReminder(ctx, compartida.ID, "bob", recordatorio(t, antes, "", 0)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("poner un recordatorio en una nota que no se lee: %v", err)
	}
	if _, _, err := st.CompartirNota(ctx, compartida.ID, "ada", "bob", false); err != nil {
		t.Fatal(err)
	}
	poner(compartida, "bob", "")
	// Un cuaderno cifrado: el título no sale nunca en el aviso.
	cif := domain.NotebookInput{Title: "Cifrado", Encrypted: true, Crypto: &domain.CryptoParams{
		V: 1, KDF: "PBKDF2-SHA256", Salt: "c2FsLWRlLXBydWViYS0xMjM0", Iterations: domain.MinKDFIterations, Check: "encv1:AAAA",
	}}
	if err := cif.Normalize(); err != nil {
		t.Fatal(err)
	}
	legacyBook := cif
	legacyBook.Encrypted, legacyBook.Crypto = false, nil
	nbc, err := st.CreateNotebook(ctx, "ada", legacyBook)
	if err != nil {
		t.Fatal(err)
	}
	// A legacy note created before encryption was enabled must not leak its title.
	secreta, err := st.CreateNote(ctx, nbc.ID, "ada", domain.NoteInput{Title: "Secreto"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateNotebook(ctx, nbc.ID, "ada", cif); err != nil {
		t.Fatal(err)
	}
	poner(secreta, "ada", "")
	// Otra organización no se ve con la sesión de esta, pero el worker sí la avisa.
	nbo, err := st.CreateNotebook(tenantCtx("otra"), "eva", entradaCuaderno("Suyo", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	ajena, err := st.CreateNote(tenantCtx("otra"), nbo.ID, "eva", domain.NoteInput{Title: "De otra"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetReminder(tenantCtx("otra"), ajena.ID, "eva", recordatorio(t, antes, "", 0)); err != nil {
		t.Fatal(err)
	}

	mias, err := st.ListReminders(ctx, "ada")
	if err != nil || len(mias) != 4 {
		t.Fatalf("mis recordatorios: %d %v (sin la nota en la papelera tenían que ser 4)", len(mias), err)
	}

	reclamar := func() map[uuid.UUID]store.RecordatorioVencido {
		t.Helper()
		vistos := map[uuid.UUID]store.RecordatorioVencido{}
		if _, err := st.ReclamarRecordatorios(ctx, ahora, 100, func(v store.RecordatorioVencido) {
			if _, ya := vistos[v.NoteID]; ya {
				t.Errorf("%q se avisa dos veces en la misma vuelta", v.Title)
			}
			vistos[v.NoteID] = v
		}); err != nil {
			t.Fatal(err)
		}
		return vistos
	}
	vistos := reclamar()
	quiero := map[uuid.UUID]string{simple.ID: "ada", diaria.ID: "ada", compartida.ID: "bob", secreta.ID: "ada", ajena.ID: "eva"}
	if len(vistos) != len(quiero) {
		t.Errorf("se avisa de %d y tocaban %d: %+v", len(vistos), len(quiero), vistos)
	}
	for id, quien := range quiero {
		if v, ok := vistos[id]; !ok || v.OwnerSub != quien {
			t.Errorf("la nota %v tenía que avisar a %q: %+v", id, quien, v)
		}
	}
	if v := vistos[secreta.ID]; v.Title != "" {
		t.Errorf("el aviso de una nota cifrada lleva su título %q", v.Title)
	}
	if v := vistos[simple.ID]; v.Title != "Llamar al fontanero" || v.NotebookID != nb.ID {
		t.Errorf("el aviso no dice qué nota es: %+v", v)
	}

	// Segunda vuelta: nada, salvo que ya toque la diaria (no: se fue a mañana).
	if otra := reclamar(); len(otra) != 0 {
		t.Errorf("la segunda vuelta vuelve a avisar: %+v", otra)
	}
	mias, err = st.ListReminders(ctx, "ada")
	if err != nil || len(mias) != 1 || mias[0].NoteID != diaria.ID || !mias[0].DueAt.After(ahora) {
		t.Fatalf("tras avisar solo sigue pendiente la diaria, reprogramada: %+v %v", mias, err)
	}

	// A Bob le quitan la nota antes de que suene: se apaga sin avisar.
	if _, err := st.SetReminder(ctx, compartida.ID, "bob", recordatorio(t, antes, "", 0)); err != nil {
		t.Fatal(err)
	}
	if err := st.DejarDeCompartirNota(ctx, compartida.ID, "ada", "bob"); err != nil {
		t.Fatal(err)
	}
	if otra := reclamar(); len(otra) != 0 {
		t.Errorf("avisa a quien ya no lee la nota: %+v", otra)
	}
	if bobs, _ := st.ListReminders(ctx, "bob"); len(bobs) != 0 {
		t.Errorf("el recordatorio de quien ya no la lee sigue pendiente: %+v", bobs)
	}
}

// Marcar hecha una tarea que repite deja la siguiente sin marcar, con el recordatorio
// en su próximo vencimiento; la hecha se queda hecha y sin él. Dos veces no crea dos.
func TestTareaQueRepiteCreaLaSiguienteAlCompletarse(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Rutinas", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	crear := func(titulo string) domain.Note {
		t.Helper()
		n, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: titulo, Kind: domain.KindTodo})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	madrid, _ := time.LoadLocation("Europe/Madrid")
	vence := time.Date(2026, 10, 1, 9, 0, 0, 0, madrid)
	completada := time.Date(2026, 10, 1, 18, 0, 0, 0, madrid)
	regar, gimnasio, una := crear("Regar"), crear("Gimnasio"), crear("Solo una vez")
	for n, in := range map[uuid.UUID]domain.ReminderInput{
		regar.ID:    recordatorio(t, vence, "FREQ=DAILY", 0),
		gimnasio.ID: recordatorio(t, vence, "", 2),
		una.ID:      recordatorio(t, vence, "", 0),
	} {
		if _, err := st.SetReminder(ctx, n, "ada", in); err != nil {
			t.Fatal(err)
		}
	}
	// Sin marcar, no hay nada que repetir.
	if sig, err := st.RepetirAlCompletar(ctx, regar.ID, completada); err != nil || sig != nil {
		t.Fatalf("una tarea sin hacer crea la siguiente: %+v %v", sig, err)
	}
	// Suenan las tres antes de hacerlas. La que no repite se apaga; las que repiten
	// siguen en «mis recordatorios» hasta completarse, que es donde se ve que repiten.
	if _, err := st.ReclamarRecordatorios(ctx, completada, 100, func(store.RecordatorioVencido) {}); err != nil {
		t.Fatal(err)
	}
	if sonadas, err := st.ListReminders(ctx, "ada"); err != nil || len(sonadas) != 2 {
		t.Fatalf("tras sonar siguen las dos que repiten: %+v %v", sonadas, err)
	}
	quiero := map[uuid.UUID]time.Time{
		regar.ID:    time.Date(2026, 10, 2, 9, 0, 0, 0, madrid),
		gimnasio.ID: time.Date(2026, 10, 3, 9, 0, 0, 0, madrid),
	}
	for _, n := range []domain.Note{regar, gimnasio, una} {
		if _, err := st.UpdateNote(ctx, n.ID, "ada", domain.NoteInput{Title: n.Title, Checked: siBool(true)}); err != nil {
			t.Fatal(err)
		}
		sig, err := st.RepetirAlCompletar(ctx, n.ID, completada)
		if err != nil {
			t.Fatal(err)
		}
		cuando, repite := quiero[n.ID]
		if !repite {
			if sig != nil {
				t.Errorf("%q no repite y crea %+v", n.Title, sig)
			}
			continue
		}
		if sig == nil || sig.ID == n.ID || sig.Checked || sig.Title != n.Title || sig.Kind != domain.KindTodo {
			t.Fatalf("%q: la siguiente es %+v", n.Title, sig)
		}
		otra, err := st.RepetirAlCompletar(ctx, n.ID, completada)
		if err != nil || otra != nil {
			t.Errorf("%q: completar dos veces crea otra siguiente: %+v %v", n.Title, otra, err)
		}
		quiero[sig.ID] = cuando
		delete(quiero, n.ID)
	}
	mias, err := st.ListReminders(ctx, "ada")
	if err != nil {
		t.Fatal(err)
	}
	// Las dos siguientes; la de «solo una vez» ya sonó, y las hechas no conservan nada.
	if len(mias) != 2 {
		t.Fatalf("recordatorios tras completar: %+v", mias)
	}
	for _, r := range mias {
		if cuando, ok := quiero[r.NoteID]; !ok || !r.DueAt.Equal(cuando) {
			t.Errorf("la siguiente de %v vence %v, y tocaba %v", r.NoteID, r.DueAt.In(madrid), cuando)
		}
	}
	notas, err := st.ListNotes(ctx, nb.ID, "ada")
	if err != nil || len(notas) != 5 {
		t.Fatalf("en el cuaderno: %d notas %v (tres hechas y dos siguientes)", len(notas), err)
	}
}

// Lo que compartió quien ya no escribe en el cuaderno deja de valer también para los
// recordatorios: Alice escribe en el cuaderno de Bob y comparte su nota con Carol;
// cuando Bob le quita la edición, Carol ya no la ve en «Compartido conmigo», así que
// tampoco se le avisa (con su título en claro) ni puede ponerle uno. Mientras vale, el
// aviso sabe que Carol solo la tiene por compartida.
func TestRecordatorioDeLoQueCompartioQuienYaNoEscribe(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ahora := time.Now().UTC().Truncate(time.Minute)
	antes := ahora.Add(-time.Hour)

	nb, err := st.CreateNotebook(ctx, "bob", entradaCuaderno("De Bob", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CompartirCuaderno(ctx, nb.ID, "bob", "alice", true); err != nil {
		t.Fatal(err)
	}
	nota, err := st.CreateNote(ctx, nb.ID, "alice", domain.NoteInput{Title: "Presupuesto"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CompartirNota(ctx, nota.ID, "alice", "carol", false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetReminder(ctx, nota.ID, "carol", recordatorio(t, antes, "", 0)); err != nil {
		t.Fatal(err)
	}
	reclamar := func() []store.RecordatorioVencido {
		t.Helper()
		var out []store.RecordatorioVencido
		if _, err := st.ReclamarRecordatorios(ctx, ahora, 100, func(v store.RecordatorioVencido) { out = append(out, v) }); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if v := reclamar(); len(v) != 1 || v[0].OwnerSub != "carol" || !v[0].SoloCompartida {
		t.Fatalf("a Carol se le avisa por compartida: %+v", v)
	}
	if _, err := st.SetReminder(ctx, nota.ID, "carol", recordatorio(t, antes, "", 0)); err != nil {
		t.Fatal(err)
	}
	if err := st.DejarDeCompartirCuaderno(ctx, nb.ID, "bob", "alice"); err != nil {
		t.Fatal(err)
	}
	if v := reclamar(); len(v) != 0 {
		t.Errorf("avisa de lo que compartió quien ya no escribe: %+v", v)
	}
	if rs, _ := st.ListReminders(ctx, "carol"); len(rs) != 0 {
		t.Errorf("sigue pendiente: %+v", rs)
	}
	if _, err := st.SetReminder(ctx, nota.ID, "carol", recordatorio(t, antes, "", 0)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("poner un recordatorio en lo que ya no vale: %v", err)
	}
}

// La siguiente de una tarea que repite se comparte con las mismas personas, y la crea
// solo un recordatorio de quien puede ESCRIBIR la nota. El de quien solo la lee se
// reprograma en la misma, sin escribir una tarea nueva en un cuaderno ajeno, y sigue
// sonando.
func TestRepetirRespetaQuienEscribe(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	madrid, _ := time.LoadLocation("Europe/Madrid")
	vence := time.Date(2026, 10, 1, 9, 0, 0, 0, madrid)
	hecha := time.Date(2026, 10, 1, 18, 0, 0, 0, madrid)

	nb, err := st.CreateNotebook(ctx, "alice", entradaCuaderno("Casa", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	completar := func(id uuid.UUID, quien string) *domain.Note {
		t.Helper()
		n, antes, err := st.ActualizarNota(ctx, id, quien, domain.NoteInput{Title: "x", Checked: siBool(true)})
		if err != nil || antes.Checked || !n.Checked {
			t.Fatalf("completar: %+v %v %v", n, antes, err)
		}
		sig, err := st.RepetirAlCompletar(ctx, id, hecha)
		if err != nil {
			t.Fatal(err)
		}
		return sig
	}

	// Carol la tiene compartida con edición: la completa y la siguiente sigue siendo suya.
	tarea, err := st.CreateNote(ctx, nb.ID, "alice", domain.NoteInput{Title: "Riego", Kind: domain.KindTodo})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CompartirNota(ctx, tarea.ID, "alice", "carol", true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetReminder(ctx, tarea.ID, "carol", recordatorio(t, vence, "FREQ=WEEKLY", 0)); err != nil {
		t.Fatal(err)
	}
	sig := completar(tarea.ID, "carol")
	if sig == nil {
		t.Fatal("el recordatorio de quien escribe por compartida no crea la siguiente")
	}
	comp, err := st.NotasCompartidas(ctx, "carol")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(comp, func(c domain.NotaCompartida) bool { return c.ID == sig.ID }) {
		t.Errorf("la siguiente no se comparte con Carol: %+v", comp)
	}
	if rs, _ := st.ListReminders(ctx, "carol"); len(rs) != 1 || rs[0].NoteID != sig.ID {
		t.Errorf("el recordatorio de Carol no se muda a la siguiente: %+v", rs)
	}

	// Dan solo lee el cuaderno: su «cada día» no crea tareas.
	if _, _, err := st.CompartirCuaderno(ctx, nb.ID, "alice", "dan", false); err != nil {
		t.Fatal(err)
	}
	otra, err := st.CreateNote(ctx, nb.ID, "alice", domain.NoteInput{Title: "Basura", Kind: domain.KindTodo})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetReminder(ctx, otra.ID, "dan", recordatorio(t, vence, "FREQ=DAILY", 0)); err != nil {
		t.Fatal(err)
	}
	antesDe, _ := st.ListNotes(ctx, nb.ID, "alice")
	if s := completar(otra.ID, "alice"); s != nil {
		t.Fatalf("el recordatorio de quien solo lee crea %+v", s)
	}
	if despues, _ := st.ListNotes(ctx, nb.ID, "alice"); len(despues) != len(antesDe) {
		t.Errorf("aparece una tarea que nadie pidió: %d → %d", len(antesDe), len(despues))
	}
	manana := time.Date(2026, 10, 2, 9, 0, 0, 0, madrid)
	rs, _ := st.ListReminders(ctx, "dan")
	if len(rs) != 1 || rs[0].NoteID != otra.ID || !rs[0].DueAt.Equal(manana) {
		t.Fatalf("el de Dan se reprograma en la misma: %+v", rs)
	}
	var avisos []store.RecordatorioVencido
	if _, err := st.ReclamarRecordatorios(ctx, manana.Add(time.Minute), 100, func(v store.RecordatorioVencido) {
		if v.OwnerSub == "dan" {
			avisos = append(avisos, v)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if len(avisos) != 1 || avisos[0].SoloCompartida {
		t.Errorf("el de Dan no suena en la tarea hecha: %+v", avisos)
	}
	if rs, _ := st.ListReminders(ctx, "dan"); len(rs) != 1 || !rs[0].DueAt.After(manana) {
		t.Errorf("tras sonar no se reprograma: %+v", rs)
	}
}

// Repetir es cosa del PASO a hecha: reenviar `checked: true` no crea otra. Y desmarcarla
// lo deshace: la siguiente intacta va a la papelera y el recordatorio vuelve con su
// vencimiento. Si alguien ya la tocó, se queda, y la original no duplica el recordatorio.
func TestDesmarcarDeshaceLaRepeticion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	madrid, _ := time.LoadLocation("Europe/Madrid")
	vence := time.Now().Add(48 * time.Hour).In(madrid).Truncate(time.Minute)

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Rutinas", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	tarea, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "Regar", Kind: domain.KindTodo})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetReminder(ctx, tarea.ID, "ada", recordatorio(t, vence, "FREQ=DAILY", 0)); err != nil {
		t.Fatal(err)
	}
	marcar := func(id uuid.UUID, titulo string, m bool) bool {
		t.Helper()
		_, antes, err := st.ActualizarNota(ctx, id, "ada", domain.NoteInput{Title: titulo, Checked: siBool(m)})
		if err != nil {
			t.Fatal(err)
		}
		return antes.Checked
	}
	cuantas := func() int {
		t.Helper()
		ns, err := st.ListNotes(ctx, nb.ID, "ada")
		if err != nil {
			t.Fatal(err)
		}
		return len(ns)
	}

	if marcar(tarea.ID, "Regar", true) {
		t.Fatal("antes de marcarla no estaba marcada")
	}
	sig, err := st.RepetirAlCompletar(ctx, tarea.ID, time.Now())
	if err != nil || sig == nil {
		t.Fatalf("no crea la siguiente: %v", err)
	}
	if !marcar(tarea.ID, "Regar las plantas", true) {
		t.Error("cambiar el texto de una hecha se lee como el paso a hecha")
	}
	// Desmarcar: la siguiente, intacta, se va; el recordatorio vuelve tal cual.
	if !marcar(tarea.ID, "Regar las plantas", false) {
		t.Fatal("desmarcar no ve que estaba marcada")
	}
	retirada, err := st.DeshacerRepeticion(ctx, tarea.ID)
	if err != nil || retirada == nil || *retirada != sig.ID {
		t.Fatalf("no retira la siguiente: %v %v", retirada, err)
	}
	if n := cuantas(); n != 1 {
		t.Errorf("tras deshacer quedan %d tareas", n)
	}
	rs, _ := st.ListReminders(ctx, "ada")
	if len(rs) != 1 || rs[0].NoteID != tarea.ID || !rs[0].DueAt.Equal(vence) {
		t.Fatalf("el recordatorio no vuelve con su vencimiento: %+v", rs)
	}

	// Otra vez, y ahora alguien toca la siguiente: desmarcar la original no la quita.
	marcar(tarea.ID, "Regar las plantas", true)
	sig, err = st.RepetirAlCompletar(ctx, tarea.ID, time.Now())
	if err != nil || sig == nil {
		t.Fatalf("no crea la siguiente al volver a marcar: %v", err)
	}
	marcar(sig.ID, "Regar las plantas del balcón", false)
	marcar(tarea.ID, "Regar las plantas", false)
	if retirada, err := st.DeshacerRepeticion(ctx, tarea.ID); err != nil || retirada != nil {
		t.Errorf("retira una siguiente que ya tocaron: %v %v", retirada, err)
	}
	if n := cuantas(); n != 2 {
		t.Errorf("con la siguiente tocada tenían que quedar 2 tareas y hay %d", n)
	}
	if rs, _ := st.ListReminders(ctx, "ada"); len(rs) != 1 || rs[0].NoteID != sig.ID {
		t.Errorf("el recordatorio se duplica o no se queda en la siguiente: %+v", rs)
	}
}

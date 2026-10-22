// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// Un cuaderno PERSONAL de Ada compartido con Bob: Bob lo lee —y solo lo que no es
// privado de Ada—, no escribe hasta que Ada le da edición, nunca cambia el cuaderno en sí
// ni lo comparte, y al dejar de compartirlo deja de verlo. Carla, de la misma
// organización, no lo ve en ningún momento.
func TestCompartirUnCuadernoConUnaPersona(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Diario", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "Lunes"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "Secreto", Visibility: domain.VisPrivate}); err != nil {
		t.Fatal(err)
	}
	ve := func(quien string) (visto, soloLectura bool) {
		t.Helper()
		ls, err := st.ListNotebooks(ctx, quien)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range ls {
			if x.ID == nb.ID {
				return true, x.ReadOnly
			}
		}
		return false, false
	}
	if v, _ := ve("bob"); v {
		t.Fatal("Bob ve el cuaderno personal de Ada antes de que se lo compartan")
	}

	if _, _, err := st.CompartirCuaderno(ctx, nb.ID, "ada", "ada", false); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("compartir con una misma: %v", err)
	}
	if _, _, err := st.CompartirCuaderno(ctx, nb.ID, "bob", "carla", false); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Bob comparte un cuaderno que no es suyo: %v", err)
	}
	nueva, titulo, err := st.CompartirCuaderno(ctx, nb.ID, "ada", "bob", false)
	if err != nil || !nueva || titulo != "Diario" {
		t.Fatalf("compartir para leer: nueva=%v título=%q err=%v", nueva, titulo, err)
	}
	if v, ro := ve("bob"); !v || !ro {
		t.Fatalf("compartido para leer: visto=%v soloLectura=%v", v, ro)
	}
	if v, _ := ve("carla"); v {
		t.Fatal("Carla ve un cuaderno compartido solo con Bob")
	}
	if _, err := st.GetNotebook(ctx, nb.ID, "bob"); err != nil {
		t.Errorf("Bob no abre el cuaderno compartido: %v", err)
	}
	notas, err := st.ListNotes(ctx, nb.ID, "bob")
	if err != nil || len(notas) != 1 || notas[0].Title != "Lunes" {
		t.Fatalf("Bob tiene que leer «Lunes» y no la nota privada de Ada: %+v %v", notas, err)
	}
	if _, err := st.CreateNote(ctx, nb.ID, "bob", domain.NoteInput{Title: "x"}); err == nil {
		t.Error("Bob escribe con el cuaderno compartido solo para leer")
	}
	if _, err := st.PersonasDeCuaderno(ctx, nb.ID, "bob"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Bob ve con quién está compartido: %v", err)
	}
	ps, err := st.PersonasDeCuaderno(ctx, nb.ID, "ada")
	if err != nil || len(ps) != 1 || ps[0].Sujeto != "bob" || ps[0].Permiso != domain.PermisoLeer || ps[0].ConcedidoPor != "ada" {
		t.Fatalf("personas: %+v %v", ps, err)
	}

	// Subir a edición no es compartir de nuevo: no se vuelve a avisar.
	nueva, _, err = st.CompartirCuaderno(ctx, nb.ID, "ada", "bob", true)
	if err != nil || nueva {
		t.Fatalf("dar edición: nueva=%v err=%v", nueva, err)
	}
	if v, ro := ve("bob"); !v || ro {
		t.Fatalf("con edición: visto=%v soloLectura=%v", v, ro)
	}
	if _, err := st.CreateNote(ctx, nb.ID, "bob", domain.NoteInput{Title: "de Bob"}); err != nil {
		t.Errorf("con edición, Bob no escribe: %v", err)
	}
	if _, err := st.UpdateNotebook(ctx, nb.ID, "bob", entradaCuaderno("Mío", domain.VisOrganization)); err == nil {
		t.Error("con edición, Bob cambia el cuaderno en sí (y podría abrirlo a toda la organización)")
	}
	if ps, _ := st.PersonasDeCuaderno(ctx, nb.ID, "ada"); len(ps) != 1 || ps[0].Permiso != domain.PermisoEditar {
		t.Fatalf("una persona está en UN nivel: %+v", ps)
	}

	if err := st.DejarDeCompartirCuaderno(ctx, nb.ID, "bob", "bob"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Bob se revoca a sí mismo sin ser el dueño: %v", err)
	}
	if err := st.DejarDeCompartirCuaderno(ctx, nb.ID, "ada", "bob"); err != nil {
		t.Fatal(err)
	}
	if v, _ := ve("bob"); v {
		t.Fatal("dejado de compartir, Bob sigue viendo el cuaderno")
	}
	if _, err := st.CreateNote(ctx, nb.ID, "bob", domain.NoteInput{Title: "y"}); err == nil {
		t.Error("dejado de compartir, Bob sigue escribiendo")
	}
	if err := st.DejarDeCompartirCuaderno(ctx, nb.ID, "ada", "bob"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("revocar lo que ya no está: %v", err)
	}
}

// Una nota de Ada compartida con Bob: le sale en «Compartido conmigo» y no le abre el
// cuaderno; con edición cambia su contenido y nada más —ni la visibilidad, ni las
// otras notas—; al retirarla, desaparece. Carla nunca la ve.
func TestCompartirUnaNotaConUnaPersona(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Trabajo", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	nota, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "Presupuesto", Content: "<p>1000</p>", Visibility: domain.VisPrivate})
	if err != nil {
		t.Fatal(err)
	}
	otra, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "Otra"})
	if err != nil {
		t.Fatal(err)
	}
	compartidas := func(quien string) []domain.NotaCompartida {
		t.Helper()
		ns, err := st.NotasCompartidas(ctx, quien)
		if err != nil {
			t.Fatal(err)
		}
		return ns
	}

	if _, _, err := st.CompartirNota(ctx, nota.ID, "bob", "carla", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Bob comparte una nota que no escribió: %v", err)
	}
	nueva, titulo, err := st.CompartirNota(ctx, nota.ID, "ada", "bob", false)
	if err != nil || !nueva || titulo != "Presupuesto" {
		t.Fatalf("compartir la nota: nueva=%v título=%q err=%v", nueva, titulo, err)
	}
	ns := compartidas("bob")
	if len(ns) != 1 || ns[0].ID != nota.ID || ns[0].Permiso != domain.PermisoLeer || ns[0].ConcedidoPor != "ada" {
		t.Fatalf("«Compartido conmigo» de Bob: %+v", ns)
	}
	if len(compartidas("carla")) != 0 {
		t.Fatal("Carla ve una nota compartida solo con Bob")
	}
	if _, err := st.GetNotebook(ctx, nb.ID, "bob"); err == nil {
		t.Error("compartir una nota le abre a Bob el cuaderno entero")
	}
	if _, err := st.UpdateNote(ctx, nota.ID, "bob", domain.NoteInput{Title: "Mía"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Bob edita una nota compartida solo para leer: %v", err)
	}

	if nueva, _, err := st.CompartirNota(ctx, nota.ID, "ada", "bob", true); err != nil || nueva {
		t.Fatalf("dar edición: nueva=%v err=%v", nueva, err)
	}
	cambiada, err := st.UpdateNote(ctx, nota.ID, "bob", domain.NoteInput{Title: "Presupuesto 2027", Content: "<p>1200</p>", Visibility: domain.VisOrganization})
	if err != nil {
		t.Fatalf("con edición, Bob no cambia la nota: %v", err)
	}
	if cambiada.Title != "Presupuesto 2027" || cambiada.Visibility != domain.VisPrivate {
		t.Errorf("Bob cambia el título pero NO la visibilidad: %+v", cambiada)
	}
	if _, err := st.UpdateNote(ctx, otra.ID, "bob", domain.NoteInput{Title: "x"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("la edición de una nota le deja escribir otra del cuaderno: %v", err)
	}
	if err := st.DeleteNote(ctx, nota.ID, "bob"); err == nil {
		t.Error("con edición de la nota, Bob la manda a la papelera")
	}

	if _, err := st.PersonasDeNota(ctx, nota.ID, "bob"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Bob ve con quién está compartida: %v", err)
	}
	if err := st.DejarDeCompartirNota(ctx, nota.ID, "ada", "bob"); err != nil {
		t.Fatal(err)
	}
	if len(compartidas("bob")) != 0 {
		t.Fatal("retirada, la nota le sigue saliendo a Bob")
	}
	if _, err := st.UpdateNote(ctx, nota.ID, "bob", domain.NoteInput{Title: "z"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("retirada, Bob sigue editando: %v", err)
	}

	// En la papelera tampoco sale: quien la recibió no ve lo que su dueña tiró.
	if _, _, err := st.CompartirNota(ctx, otra.ID, "ada", "bob", false); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteNote(ctx, otra.ID, "ada"); err != nil {
		t.Fatal(err)
	}
	if len(compartidas("bob")) != 0 {
		t.Error("una nota en la papelera de Ada le sale a Bob en «Compartido conmigo»")
	}
}

// Una nota de un cuaderno cifrado no se comparte suelta: quien la recibe no tendría
// el cuaderno con que descifrarla.
func TestUnaNotaCifradaNoSeComparteSuelta(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	in := domain.NotebookInput{Title: "Cifrado", Encrypted: true, Crypto: &domain.CryptoParams{
		V: 1, KDF: "PBKDF2-SHA256", Salt: "c2FsLWRlLXBydWViYS0xMjM0", Iterations: domain.MinKDFIterations, Check: "encv1:AAAA",
	}}
	if err := in.Normalize(); err != nil {
		t.Fatal(err)
	}
	nb, err := st.CreateNotebook(ctx, "ada", in)
	if err != nil {
		t.Fatal(err)
	}
	cryptoJSON, _ := json.Marshal(nb.Crypto)
	nota, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{CipherVersion: domain.EncryptedNoteWriteVersion, ExpectedCrypto: cryptoJSON, ContentCipher: "encv1:AAAA"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CompartirNota(ctx, nota.ID, "ada", "bob", false); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("compartir una nota cifrada: %v", err)
	}
	// El cuaderno entero sí: quien lo recibe lo abre con la contraseña, como hoy uno
	// de la organización.
	if _, _, err := st.CompartirCuaderno(ctx, nb.ID, "ada", "bob", false); err != nil {
		t.Errorf("compartir el cuaderno cifrado: %v", err)
	}
}

// Un subcuaderno compartido sale en la RAÍZ del árbol de quien lo recibe: su padre no lo
// ve, y los clientes arman el árbol desde la raíz.
func TestUnSubcuadernoCompartidoSaleEnLaRaiz(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	padre, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Trabajo", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	in := entradaCuaderno("Viajes", domain.VisPersonal)
	in.ParentID = &padre.ID
	hijo, err := st.CreateNotebook(ctx, "ada", in)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CompartirCuaderno(ctx, hijo.ID, "ada", "bob", false); err != nil {
		t.Fatal(err)
	}
	ls, err := st.ListNotebooks(ctx, "bob")
	if err != nil || len(ls) != 1 || ls[0].ID != hijo.ID || ls[0].ParentID != nil {
		t.Fatalf("el árbol de Bob: %+v %v", ls, err)
	}
	if ls, _ := st.ListNotebooks(ctx, "ada"); len(ls) != 2 || (ls[1].ParentID == nil && ls[0].ParentID == nil) {
		t.Fatalf("el árbol de Ada pierde su anidado: %+v", ls)
	}
}

// Lo que una persona compartió de un cuaderno en el que ya no escribe deja de valer, y no
// puede volver a compartirlo: Ada escribe en el cuaderno de organización de Carla, lo
// comparte con Bob, y Carla lo hace personal.
func TestLoCompartidoPorQuienPierdeElCuadernoDejaDeValer(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	nb, err := st.CreateNotebook(ctx, "carla", entradaCuaderno("Equipo", domain.VisOrganization))
	if err != nil {
		t.Fatal(err)
	}
	nota, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "De Ada"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CompartirNota(ctx, nota.ID, "ada", "bob", true); err != nil {
		t.Fatal(err)
	}
	if ns, _ := st.NotasCompartidas(ctx, "bob"); len(ns) != 1 {
		t.Fatalf("Bob no ve la nota de Ada: %+v", ns)
	}
	if _, err := st.UpdateNotebook(ctx, nb.ID, "carla", entradaCuaderno("Equipo", domain.VisPersonal)); err != nil {
		t.Fatal(err)
	}
	if ns, _ := st.NotasCompartidas(ctx, "bob"); len(ns) != 0 {
		t.Errorf("Ada ya no escribe en el cuaderno y Bob sigue viendo su nota: %+v", ns)
	}
	if _, err := st.UpdateNote(ctx, nota.ID, "bob", domain.NoteInput{Title: "x"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Bob sigue editándola: %v", err)
	}
	if _, _, err := st.CompartirNota(ctx, nota.ID, "ada", "dani", false); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Ada reparte una nota de un cuaderno en el que ya no escribe: %v", err)
	}
}

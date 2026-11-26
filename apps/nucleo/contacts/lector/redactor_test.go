// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package lector_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/lector"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// La libreta del redactor de Mail (M-13): antes Mail tenía su propia copia de esta
// libreta en memoria (`apps/fyne/local/correo.Contactos`); ahora usa Libreta, con Search y
// All, y estas pruebas vinieron con ella.

// libretaVariada abre Contacts (su repositorio, escribiendo) en dir con gente variada:
// acentos, varias libretas, organización, apodo, nota, varios correos y teléfonos, uno
// en la papelera y una libreta entera en la papelera.
func libretaVariada(t *testing.T, dir string) (*contacts.Repository, string) {
	t.Helper()
	ctx := t.Context()
	rec, err := almacen.Abrir(context.Background(), dir, "contacts", "persona")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rec.Cerrar)
	repo, err := contacts.New(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repo.Close)
	if err := almacen.Preparar(ctx, repo); err != nil {
		t.Fatal(err)
	}
	personal, err := repo.CreateAddressBook(ctx, "Contactos")
	if err != nil {
		t.Fatal(err)
	}
	trabajo, err := repo.CreateAddressBook(ctx, "Trabajo")
	if err != nil {
		t.Fatal(err)
	}
	vieja, err := repo.CreateAddressBook(ctx, "Vieja")
	if err != nil {
		t.Fatal(err)
	}
	crear := func(libro string, in api.ContactInput) api.Contact {
		t.Helper()
		c, err := repo.Create(ctx, libro, in)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	crear(personal.ID, api.ContactInput{FormattedName: "Luis Pérez", GivenName: "Luis", FamilyName: "Pérez", Emails: []api.ContactEmail{{Email: "luis@ejemplo.eu"}, {Email: "LUIS.perez@trabajo.es"}}})
	crear(personal.ID, api.ContactInput{FormattedName: "José Ñúñez", Nickname: "Pepe", Phones: []api.ContactPhone{{Phone: "+34 600 111 222"}}})
	crear(personal.ID, api.ContactInput{FormattedName: "Marta Gil", Note: "Conocida en el congreso de Ávila", Emails: []api.ContactEmail{{Email: "marta@ejemplo.eu"}}})
	crear(trabajo.ID, api.ContactInput{FormattedName: "Ana Acme", Organization: "ACME Ibérica", Emails: []api.ContactEmail{{Email: "ana@acme.example"}}})
	crear(trabajo.ID, api.ContactInput{FormattedName: "Ángela Ruiz", Organization: "Acme", Phones: []api.ContactPhone{{Phone: "600 333 444"}}})
	borrado := crear(personal.ID, api.ContactInput{FormattedName: "Luisa Borrada", Emails: []api.ContactEmail{{Email: "luisa@ejemplo.eu"}}})
	crear(vieja.ID, api.ContactInput{FormattedName: "Luis Antiguo", Emails: []api.ContactEmail{{Email: "antiguo@ejemplo.eu"}}})
	if err := repo.Delete(ctx, borrado.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteAddressBook(ctx, vieja.ID); err != nil {
		t.Fatal(err)
	}
	return repo, personal.ID
}

func idsDe(cs []api.Contact) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return out
}

// TestLaLibretaBuscaYPaginaComoContacts: la libreta en memoria, la del redactor de Mail
// (M-13), busca y pagina EXACTAMENTE como `Search` y `All` del repositorio de Contacts,
// en el mismo orden (acentos, mayúsculas, correos, teléfonos, apodo,
// nota, organización, papelera, libretas borradas, topes y desplazamientos).
func TestLaLibretaBuscaYPaginaComoContacts(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, _ := libretaVariada(t, dir)
	c := lector.Nuevo(dir, "persona").Libreta()

	for _, q := range []string{
		"luis", "LUIS", "pérez", "perez", "jose", "JOSÉ", "ñu", "nun", "pepe", "acme", "ACME ibérica",
		"@ejemplo", "trabajo.es", "600", "+34", "avila", "congreso", "a", "  marta  ", "luisa", "antiguo", "zzz", "", "   ",
	} {
		for _, limite := range []int{8, 1, 2, 0, -3, 50, 501} {
			quiere, errQuiere := repo.Search(ctx, q, limite)
			tiene, errTiene := c.Search(ctx, q, limite)
			if (errQuiere != nil) != (errTiene != nil) {
				t.Errorf("buscar %q (%d): error %v ≠ %v", q, limite, errTiene, errQuiere)
				continue
			}
			if !slices.Equal(idsDe(tiene), idsDe(quiere)) {
				t.Errorf("buscar %q (%d): %v ≠ %v", q, limite, idsDe(tiene), idsDe(quiere))
			}
		}
	}
	for _, p := range [][2]int{{500, 0}, {2, 0}, {2, 2}, {2, 4}, {2, 6}, {0, 0}, {-1, 1}, {501, 0}, {3, 1000}, {3, -2}} {
		quiere, sigQuiere, errQuiere := repo.All(ctx, p[0], p[1])
		tiene, sigTiene, errTiene := c.All(ctx, p[0], p[1])
		if errQuiere != nil || errTiene != nil || !slices.Equal(idsDe(tiene), idsDe(quiere)) || sigTiene != sigQuiere {
			t.Errorf("página %v: %v %d %v ≠ %v %d %v", p, idsDe(tiene), sigTiene, errTiene, idsDe(quiere), sigQuiere, errQuiere)
		}
	}
	if todos, _, _ := c.All(ctx, 500, 0); len(todos) != 5 {
		t.Errorf("la libreta viva son cinco (sin papelera ni la libreta borrada): %v", idsDe(todos))
	}
}

// TestLaLibretaNoEsperaALaRelectura: cuando Contacts cambia, autocompletar no espera a
// releer la libreta (con 20.000 contactos son segundos): contesta en el acto con lo que
// ya había, relee detrás UNA vez aunque lleguen muchas letras, y lo nuevo aparece al
// acabar. La primera lectura, en cambio, sí se espera (no hay nada que enseñar), y
// Precargar la adelanta.
func TestLaLibretaNoEsperaALaRelectura(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, libro := libretaVariada(t, dir)
	c := lector.Nuevo(dir, "persona").Libreta()
	c.Precargar()
	lector.EsperarRelectura(c)
	if cs, err := c.Search(ctx, "marta", 8); err != nil || len(cs) != 1 {
		t.Fatalf("primera lectura: %+v %v", cs, err)
	}
	if _, err := repo.Create(ctx, libro, api.ContactInput{FormattedName: "Marta Gil Segunda"}); err != nil {
		t.Fatal(err)
	}
	suelta := make(chan struct{})
	lecturas := lector.FrenarLectura(t, suelta)
	lector.Caducar(c)
	for range 5 {
		listo := make(chan []api.Contact, 1)
		go func() {
			cs, _ := c.Search(ctx, "marta", 8)
			listo <- cs
		}()
		select {
		case cs := <-listo:
			if len(cs) != 1 {
				t.Fatalf("mientras se relee: %+v", cs)
			}
		case <-time.After(5 * time.Second):
			close(suelta)
			t.Fatal("autocompletar espera a que se relea la libreta")
		}
		lector.Caducar(c)
	}
	close(suelta)
	lector.EsperarRelectura(c)
	// Contado al acabar: la rutina de la relectura puede no haber empezado aún.
	if n := lecturas.Load(); n != 1 {
		t.Errorf("relecturas por cinco letras: %d", n)
	}
	if cs, _ := c.Search(ctx, "marta", 8); len(cs) != 2 {
		t.Fatalf("lo nuevo no aparece: %+v", cs)
	}
}

// TestLaLibretaDeContactsEnSoloLectura: con Contacts ABIERTA (su almacén abierto y
// escribiendo), la libreta de Mail lee sus contactos sin tocar su diario ni copiarlo; lo que Contacts
// añade después aparece al refrescar; sin Contacts en el equipo, no hay contactos ni error.
func TestLaLibretaDeContactsEnSoloLectura(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, libro := libretaVariada(t, dir)
	antes, err := os.ReadFile(filepath.Join(dir, "registros.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	ficherosAntes, _ := os.ReadDir(dir)

	c := lector.Nuevo(dir, "persona").Libreta()
	cs, err := c.Search(ctx, "luis", 8)
	if err != nil || len(cs) != 1 || cs[0].FormattedName != "Luis Pérez" {
		t.Fatalf("buscar: %+v %v", cs, err)
	}
	if despues, _ := os.ReadFile(filepath.Join(dir, "registros.jsonl")); !bytes.Equal(despues, antes) {
		t.Fatal("Mail escribió en el diario de Contacts")
	}
	if ficherosDespues, _ := os.ReadDir(dir); len(ficherosDespues) != len(ficherosAntes) {
		t.Fatalf("Mail dejó algo en la carpeta de Contacts: %d → %d", len(ficherosAntes), len(ficherosDespues))
	}

	if _, err := repo.Create(ctx, libro, api.ContactInput{FormattedName: "Marta Gil Segunda", Emails: []api.ContactEmail{{Email: "marta2@ejemplo.eu"}}}); err != nil {
		t.Fatal(err)
	}
	if cs, _ := c.Search(ctx, "marta", 8); len(cs) != 1 {
		t.Fatalf("antes del refresco se usa lo leído: %+v", cs)
	}
	lector.Caducar(c) // ya ha pasado el refresco
	// La relectura va detrás: mientras, se contesta con lo que ya había.
	if cs, _ := c.Search(ctx, "marta", 8); len(cs) != 1 {
		t.Fatalf("mientras se relee, lo leído: %+v", cs)
	}
	lector.EsperarRelectura(c)
	if cs, _ := c.Search(ctx, "marta", 8); len(cs) != 2 {
		t.Fatalf("lo nuevo de Contacts no aparece: %+v", cs)
	}
	// Y Contacts sigue escribiendo con normalidad.
	if _, err := repo.Create(ctx, libro, api.ContactInput{FormattedName: "Otro más"}); err != nil {
		t.Fatal("Contacts sigue escribiendo:", err)
	}

	vacio := lector.Nuevo(t.TempDir(), "persona").Libreta()
	if cs, err := vacio.Search(ctx, "x", 8); err != nil || cs != nil {
		t.Fatalf("sin Contacts: %v %v", cs, err)
	}
	if cs, n, err := vacio.All(ctx, 500, 0); err != nil || cs != nil || n != 0 {
		t.Fatalf("sin Contacts, la libreta: %v %d %v", cs, n, err)
	}
}

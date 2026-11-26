// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contacts_test

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/internal/libretaprueba"
)

// Varias consultas a la vez que se escribe (con -race): ninguna ve una libreta a medias
// ni toca la guardada, y al final la guardada es la releída.
func TestLaLibretaGuardadaConVariasGorutinas(t *testing.T) {
	ctx := t.Context()
	rec := almacenDe(t, t.TempDir())
	repo := abrirCon(t, rec.Config)
	libro, err := repo.CreateAddressBook(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	const altas = 20
	hecho := make(chan struct{})
	fallos := make(chan error, 8)
	var lectores sync.WaitGroup
	for i := range 4 {
		lectores.Go(func() {
			for {
				select {
				case <-hecho:
					return
				default:
				}
				todos, _, err := repo.All(ctx, 500, 0)
				if err == nil && len(todos) > altas {
					err = fmt.Errorf("más contactos de los que hay: %d", len(todos))
				}
				if err == nil && len(todos) > 0 {
					// Cambiar lo leído no cambia la libreta de nadie.
					todos[0].FormattedName = "tocado"
					if len(todos[0].Emails) > 0 {
						todos[0].Emails[0].Email = "tocado"
					}
					_, err = repo.Contact(ctx, todos[len(todos)-1].ID)
				}
				if err == nil && i%2 == 0 {
					_, err = repo.Search(ctx, "a", 10)
				}
				if err != nil {
					fallos <- err
					return
				}
			}
		})
	}
	r := rand.New(rand.NewPCG(3, 4))
	for i := range altas {
		if _, err := repo.Create(ctx, libro.ID, entradaDe(libretaprueba.Realista(r, i))); err != nil {
			t.Fatal(err)
		}
	}
	close(hecho)
	lectores.Wait()
	close(fallos)
	for err := range fallos {
		t.Fatal(err)
	}
	otro := rec.Config
	otro.Journal = otroDiario(t)
	if got, want := leerVistas(t, repo), leerVistas(t, abrirCon(t, otro)); !reflect.DeepEqual(got, want) || len(got.Contactos) != altas {
		t.Fatalf("la libreta guardada no es la releída: %d contra %d", len(got.Contactos), len(want.Contactos))
	}
}

// vistasDe son las tres vistas de la libreta de repo.
type vistasDe struct {
	Libros    []contacts.BookView
	Contactos []contacts.ContactView
	Grupos    []contacts.GroupView
}

func leerVistas(t *testing.T, repo *contacts.Repository) vistasDe {
	t.Helper()
	foto, err := repo.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var v vistasDe
	if v.Libros, err = foto.Books(); err != nil {
		t.Fatal(err)
	}
	if v.Contactos, err = foto.Contacts(); err != nil {
		t.Fatal(err)
	}
	if v.Grupos, err = foto.Groups(); err != nil {
		t.Fatal(err)
	}
	return v
}

// Tras cualquier escritura, la libreta guardada (la candidata de la escritura o la
// releída) es la misma, campo a campo, que la que lee de cero otro repositorio sobre el
// mismo almacén: altas, cambios, papelera, favoritos (también de un UID que está en otra
// libreta), etiquetas, fotos, borrar para siempre e importar.
func TestLaLibretaGuardadaEsLaMismaQueReleerla(t *testing.T) {
	ctx := t.Context()
	rec := almacenDe(t, t.TempDir())
	repo := abrirCon(t, rec.Config)
	comprobar := func(paso string) {
		t.Helper()
		otro := rec.Config
		otro.Journal = otroDiario(t)
		de := abrirCon(t, otro)
		if got, want := leerVistas(t, repo), leerVistas(t, de); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: la libreta guardada no es la releída\nguardada: %+v\nreleída:  %+v", paso, got, want)
		}
		de.Close()
	}
	uno, err := repo.CreateAddressBook(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	dos, err := repo.CreateAddressBook(ctx, "Trabajo")
	if err != nil {
		t.Fatal(err)
	}
	comprobar("libretas")
	r := rand.New(rand.NewPCG(11, 25))
	gente := make([]api.Contact, 0, 12)
	for i := range 12 {
		libro := uno.ID
		if i%3 == 0 {
			libro = dos.ID
		}
		c, err := repo.Create(ctx, libro, entradaDe(libretaprueba.Realista(r, i)))
		if err != nil {
			t.Fatal(err)
		}
		gente = append(gente, c)
	}
	comprobar("altas")

	// Un favorito, y una tarjeta con su mismo UID en la otra libreta: sale destacada.
	if err := repo.SetFavorite(ctx, gente[1].UID, true); err != nil {
		t.Fatal(err)
	}
	comprobar("favorito")
	tarjeta := fmt.Sprintf("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:%s\r\nFN:Gemela\r\nEND:VCARD\r\n", gente[1].UID)
	if _, err := repo.PonerTarjetaCruda(ctx, dos.ID, "", "", tarjeta); err != nil {
		t.Fatal(err)
	}
	comprobar("alta con el UID de un favorito")

	cambio := gente[1].Input()
	cambio.Note = "Editado tras destacarlo"
	if _, err := repo.Update(ctx, gente[1].ID, cambio, gente[1].ETag); err != nil {
		t.Fatal(err)
	}
	comprobar("editar un favorito")

	cambio = gente[3].Input()
	cambio.Emails = append([]api.ContactEmail{{Email: "nuevo@ejemplo.org", IsPrimary: true}}, cambio.Emails...)
	cambio.Phones = nil
	if _, err := repo.Update(ctx, gente[3].ID, cambio, gente[3].ETag); err != nil {
		t.Fatal(err)
	}
	comprobar("cambiar correos y teléfonos")

	if err := repo.Delete(ctx, gente[2].ID); err != nil {
		t.Fatal(err)
	}
	comprobar("a la papelera")
	cambio = gente[2].Input()
	cambio.Title = "En la papelera"
	if _, err := repo.Update(ctx, gente[2].ID, cambio, gente[2].ETag); err == nil {
		t.Fatal("se editó un contacto de la papelera")
	}
	if err := repo.Restore(ctx, gente[2].ID); err != nil {
		t.Fatal(err)
	}
	comprobar("de vuelta de la papelera")
	vuelto, err := repo.Contact(ctx, gente[2].ID)
	if err != nil {
		t.Fatal(err)
	}
	cambio = vuelto.Input()
	cambio.Title = "De vuelta"
	if _, err := repo.Update(ctx, vuelto.ID, cambio, vuelto.ETag); err != nil {
		t.Fatal(err)
	}
	comprobar("editar uno que volvió de la papelera")

	grupo, err := repo.CreateGroup(ctx, uno.ID, "Amigos")
	if err != nil {
		t.Fatal(err)
	}
	var miembros []string
	for _, c := range gente[3:7] {
		if c.AddressBookID == uno.ID {
			miembros = append(miembros, c.UID)
		}
	}
	if _, err := repo.RenameGroup(ctx, grupo.ID, "Amigos de siempre", miembros); err != nil {
		t.Fatal(err)
	}
	comprobar("etiqueta")

	if err := repo.Delete(ctx, gente[4].ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Purge(ctx, gente[4].ID); err != nil {
		t.Fatal(err)
	}
	comprobar("borrar para siempre")
	if err := repo.BatchTrash(ctx, []string{gente[5].UID, gente[6].UID}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.BatchRestore(ctx, []string{gente[5].UID}); err != nil {
		t.Fatal(err)
	}
	comprobar("papelera en lote")

	foto := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	if err := repo.SetPhoto(ctx, gente[7].UID, foto); err != nil {
		t.Fatal(err)
	}
	comprobar("foto")

	gemelos := make([]card.Contact, 5)
	for i := range gemelos {
		gemelos[i] = libretaprueba.Realista(r, 100+i)
	}
	csv := csvDeGente(t, gemelos)
	if hechos, _, err := repo.ImportCSV(ctx, uno.ID, csv); err != nil || hechos != 5 {
		t.Fatal(hechos, err)
	}
	if hechos, saltados, err := repo.ImportCSV(ctx, uno.ID, csv); err != nil || hechos != 0 || saltados != 5 {
		t.Fatal("reimportar:", hechos, saltados, err)
	}
	comprobar("importar CSV")

	if err := repo.DeleteGroup(ctx, grupo.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteAddressBook(ctx, dos.ID); err != nil {
		t.Fatal(err)
	}
	comprobar("borrar etiqueta y libreta")
}

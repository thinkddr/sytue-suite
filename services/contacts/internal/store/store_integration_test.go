// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integracion de Sytue Contacts contra un PostgreSQL real (efimero,
// via platform/pkg/pgtest sobre testcontainers-go). Detras del build-tag
// `integration`: no corren en el build/CI por defecto (sin Docker). Verifican:
//   - que las migraciones del servicio aplican (esquema + RLS + trigger de busqueda),
//   - CRUD basico del store (libreta + contacto),
//   - AISLAMIENTO por tenant (RLS): datos de un tenant no visibles para otro.
package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/contacts/db"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

// tenantCtx devuelve un contexto con el tenant fijado (lo consume RLS).
func tenantCtx(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

// sampleContact construye una UpsertContact valida para una libreta y UID dados.
func sampleContact(bookID uuid.UUID, uid, fn string) store.UpsertContact {
	return store.UpsertContact{
		AddressBookID: bookID,
		Contact: domain.Contact{
			UID:           uid,
			FormattedName: fn,
			GivenName:     "Ada",
			FamilyName:    "Lovelace",
			Organization:  "Sytue",
			Emails: []domain.Email{
				{Email: "ada@sytue.eu", Type: domain.EmailWork, IsPrimary: true},
			},
			Phones: []domain.Phone{
				{Phone: "+34600000000", Type: domain.PhoneCell},
			},
		},
		CardData: "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:" + uid + "\r\nFN:" + fn + "\r\nEND:VCARD\r\n",
	}
}

// TestMigrationsApply verifica que las migraciones embebidas aplican limpiamente
// contra un PostgreSQL real y que las tablas clave llevan RLS ENABLE+FORCE.
func TestMigrationsApply(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()

	for _, tbl := range []string{
		"address_books", "card_objects", "contacts",
		"contact_emails", "contact_phones", "contact_groups", "contact_group_members",
	} {
		var relrowsecurity, relforcerowsecurity bool
		err := h.Pool.QueryRow(ctx,
			`SELECT relrowsecurity, relforcerowsecurity
			 FROM pg_class WHERE relname = $1 AND relkind = 'r'`, tbl).
			Scan(&relrowsecurity, &relforcerowsecurity)
		if err != nil {
			t.Fatalf("tabla %q no encontrada tras migrar: %v", tbl, err)
		}
		if !relrowsecurity || !relforcerowsecurity {
			t.Fatalf("tabla %q sin RLS ENABLE+FORCE (enable=%v force=%v)", tbl, relrowsecurity, relforcerowsecurity)
		}
	}
}

// TestAddressBookContactCRUD ejercita el ciclo crear/leer/borrar de una libreta
// y un contacto, a traves del store (que fija app.tenant por RLS).
func TestAddressBookContactCRUD(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	// Crear libreta.
	book, err := st.CreateAddressBook(ctx, store.NewAddressBook{
		OwnerSub:    "user-1",
		Name:        "trabajo",
		DisplayName: "Trabajo",
	})
	if err != nil {
		t.Fatalf("CreateAddressBook: %v", err)
	}
	if book.ID == uuid.Nil || book.Name != "trabajo" {
		t.Fatalf("libreta inesperada: %+v", book)
	}

	// Leer libreta.
	gotBook, err := st.GetAddressBook(ctx, "user-1", book.ID)
	if err != nil {
		t.Fatalf("GetAddressBook: %v", err)
	}
	if gotBook.ID != book.ID {
		t.Fatalf("GetAddressBook devolvio id %s, esperado %s", gotBook.ID, book.ID)
	}

	// Crear contacto (crea tarjeta + proyeccion + emails/telefonos).
	c, err := st.CreateContact(ctx, "user-1", sampleContact(book.ID, "c-uid-1", "Ada Lovelace"))
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	if c.UID != "c-uid-1" || c.ETag == uuid.Nil {
		t.Fatalf("contacto inesperado: %+v", c)
	}
	if len(c.Emails) != 1 || c.PrimaryEmail() != "ada@sytue.eu" {
		t.Fatalf("emails del contacto inesperados: %+v", c.Emails)
	}

	// Leer contacto.
	gotC, err := st.GetContact(ctx, "user-1", c.ID)
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	if gotC.FormattedName != "Ada Lovelace" {
		t.Fatalf("GetContact FN inesperado: %q", gotC.FormattedName)
	}

	// Busqueda full-text (verifica el trigger contacts_search_update).
	found, err := st.SearchContacts(ctx, "user-1", "Lovelace", nil, 10)
	if err != nil {
		t.Fatalf("SearchContacts: %v", err)
	}
	if len(found) != 1 || found[0].ID != c.ID {
		t.Fatalf("SearchContacts no encontro el contacto: %+v", found)
	}

	// Busqueda por SUBCADENA/infijo (0002 pg_trgm): "ovela" no es un token FTS
	// de "Lovelace", solo lo encuentra el ILIKE '%q%' sobre search_text.
	byInfix, err := st.SearchContacts(ctx, "user-1", "ovela", nil, 10)
	if err != nil {
		t.Fatalf("SearchContacts infijo: %v", err)
	}
	if len(byInfix) != 1 || byInfix[0].ID != c.ID {
		t.Fatalf("SearchContacts por subcadena no encontro el contacto (trigram): %+v", byInfix)
	}

	// Sin acentos ni mayúsculas, en los dos sentidos: por el FTS (palabra entera) y
	// por los trigramas (trozo). «Müller» se guarda con diéresis y se busca sin ella.
	muller := sampleContact(book.ID, "c-uid-2", "Jürgen Müller")
	muller.Contact.FamilyName, muller.Contact.GivenName, muller.Contact.Organization = "Müller", "Jürgen", "Café Pequeño"
	m, err := st.CreateContact(ctx, "user-1", muller)
	if err != nil {
		t.Fatalf("CreateContact Müller: %v", err)
	}
	for _, q := range []string{"muller", "MULLER", "Müller", "jurgen", "cafe pequeno", "pequeño", "ulle"} {
		got, err := st.SearchContacts(ctx, "user-1", q, nil, 10)
		if err != nil {
			t.Fatalf("SearchContacts(%q): %v", q, err)
		}
		if len(got) != 1 || got[0].ID != m.ID {
			t.Fatalf("SearchContacts(%q) = %+v, quiero solo a Müller", q, got)
		}
	}
	// Al revés: el texto sin acento se encuentra buscando con él.
	if got, err := st.SearchContacts(ctx, "user-1", "Lóvelace", nil, 10); err != nil || len(got) != 1 || got[0].ID != c.ID {
		t.Fatalf("SearchContacts(Lóvelace) = %+v, %v", got, err)
	}

	// Borrar contacto (soft) y confirmar.
	if err := st.DeleteContact(ctx, "user-1", c.ID); err != nil {
		t.Fatalf("DeleteContact: %v", err)
	}
	if _, err := st.GetContact(ctx, "user-1", c.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetContact tras borrar: esperaba ErrNotFound, obtuve %v", err)
	}

	// Borrar libreta y confirmar.
	if err := st.DeleteAddressBook(ctx, "user-1", book.ID); err != nil {
		t.Fatalf("DeleteAddressBook: %v", err)
	}
	if _, err := st.GetAddressBook(ctx, "user-1", book.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetAddressBook tras borrar: esperaba ErrNotFound, obtuve %v", err)
	}
}

// TestOwnerIsolation es la prueba nuclear del owner-scoping DENTRO de un mismo
// tenant (la RLS solo aisla tenants): un usuario autenticado NO ve, edita,
// borra ni encuentra por busqueda los contactos/libretas/grupos privados de un
// companero, aunque conozca sus IDs. Recurso ajeno => ErrNotFound.
func TestOwnerIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	book, err := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "alice", Name: "personal"})
	if err != nil {
		t.Fatalf("CreateAddressBook: %v", err)
	}
	c, err := st.CreateContact(ctx, "alice", sampleContact(book.ID, "iso-1", "Ada Lovelace"))
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	g, err := st.CreateGroup(ctx, "alice", store.UpsertGroup{
		AddressBookID: book.ID,
		Group:         domain.Group{UID: "iso-g", Name: "Privado"},
		CardData:      "BEGIN:VCARD\r\nVERSION:4.0\r\nKIND:group\r\nUID:iso-g\r\nFN:Privado\r\nEND:VCARD\r\n",
	})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	// mallory (mismo tenant) no alcanza NADA de alice por ID.
	if _, err := st.GetContact(ctx, "mallory", c.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("mallory leyo contacto ajeno: %v", err)
	}
	if _, err := st.UpdateContact(ctx, "mallory", c.ID, sampleContact(book.ID, "iso-1", "Pwned"), nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("mallory edito contacto ajeno: %v", err)
	}
	if err := st.DeleteContact(ctx, "mallory", c.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("mallory borro contacto ajeno: %v", err)
	}
	if _, err := st.GetAddressBook(ctx, "mallory", book.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("mallory leyo libreta ajena: %v", err)
	}
	if _, err := st.ListContacts(ctx, "mallory", book.ID, 0); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("mallory listo libreta ajena: %v", err)
	}
	if err := st.DeleteAddressBook(ctx, "mallory", book.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("mallory borro libreta ajena: %v", err)
	}
	if _, err := st.GetGroup(ctx, "mallory", g.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("mallory leyo grupo ajeno: %v", err)
	}
	if err := st.DeleteGroup(ctx, "mallory", g.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("mallory borro grupo ajeno: %v", err)
	}
	if _, err := st.CreateContact(ctx, "mallory", sampleContact(book.ID, "iso-2", "Intruso")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("mallory creo en libreta ajena: %v", err)
	}
	// La busqueda de mallory NO devuelve los contactos privados de alice.
	if found, err := st.SearchContacts(ctx, "mallory", "Lovelace", nil, 10); err != nil || len(found) != 0 {
		t.Fatalf("mallory busco y vio contactos ajenos: %v %+v", err, found)
	}

	// alice sigue viendo y buscando lo suyo (el gate no rompe al dueno).
	if _, err := st.GetContact(ctx, "alice", c.ID); err != nil {
		t.Fatalf("alice perdio su contacto: %v", err)
	}
	if found, _ := st.SearchContacts(ctx, "alice", "Lovelace", nil, 10); len(found) != 1 {
		t.Fatalf("alice no encuentra su propio contacto: %+v", found)
	}
}

// TestTenantIsolation es la prueba nuclear de RLS (CONVENTIONS §0.7/§10): los
// datos de un tenant NO son visibles ni borrables desde otro tenant, aunque se
// conozca el ID de la fila.
func TestTenantIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)

	ctxA := tenantCtx("tenant-a")
	ctxB := tenantCtx("tenant-b")

	// Tenant A crea libreta y contacto.
	bookA, err := st.CreateAddressBook(ctxA, store.NewAddressBook{OwnerSub: "u-a", Name: "personal-a"})
	if err != nil {
		t.Fatalf("A CreateAddressBook: %v", err)
	}
	cA, err := st.CreateContact(ctxA, "u-a", sampleContact(bookA.ID, "c-a", "Contacto A"))
	if err != nil {
		t.Fatalf("A CreateContact: %v", err)
	}

	// Tenant B NO ve la libreta de A por su ID (mismo owner_sub: aisla la RLS).
	if _, err := st.GetAddressBook(ctxB, "u-a", bookA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B ve la libreta de A (err=%v)", err)
	}
	// Tenant B NO ve el contacto de A por su ID.
	if _, err := st.GetContact(ctxB, "u-a", cA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B ve el contacto de A (err=%v)", err)
	}
	// Tenant B NO borra el contacto de A.
	if err := st.DeleteContact(ctxB, "u-a", cA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B borra el contacto de A (err=%v)", err)
	}
	// La busqueda de B no encuentra datos de A.
	found, err := st.SearchContacts(ctxB, "u-a", "Lovelace", nil, 10)
	if err != nil {
		t.Fatalf("B SearchContacts: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("aislamiento roto: B busca y ve contactos de A: %+v", found)
	}

	// A sigue viendo lo suyo intacto.
	if _, err := st.GetContact(ctxA, "u-a", cA.ID); err != nil {
		t.Fatalf("A perdio su propio contacto: %v", err)
	}

	// B crea su libreta con el mismo owner/name; no colisiona con A.
	bookB, err := st.CreateAddressBook(ctxB, store.NewAddressBook{OwnerSub: "u-a", Name: "personal-a"})
	if err != nil {
		t.Fatalf("B CreateAddressBook: %v", err)
	}
	if bookB.ID == bookA.ID {
		t.Fatalf("B reutilizo el ID de A: %s", bookB.ID)
	}

	// Cada tenant lista solo sus libretas.
	listA, err := st.ListAddressBooks(ctxA, "u-a")
	if err != nil {
		t.Fatalf("A ListAddressBooks: %v", err)
	}
	if len(listA) != 1 || listA[0].ID != bookA.ID {
		t.Fatalf("A ve libretas ajenas: %+v", listA)
	}
	listB, err := st.ListAddressBooks(ctxB, "u-a")
	if err != nil {
		t.Fatalf("B ListAddressBooks: %v", err)
	}
	if len(listB) != 1 || listB[0].ID != bookB.ID {
		t.Fatalf("B ve libretas ajenas: %+v", listB)
	}
}

// TestFavorites ejercita destacar/quitar destacado, el filtro, el oculta/revive
// con el borrado suave y el aislamiento por dueño.
func TestFavorites(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	book, err := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner", Name: "personal"})
	if err != nil {
		t.Fatalf("CreateAddressBook: %v", err)
	}
	c, err := st.CreateContact(ctx, "owner", sampleContact(book.ID, "fav-1", "Ada"))
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}

	// Destacar por UID (owner) y comprobar el filtro.
	if err := st.SetFavorite(ctx, "owner", c.UID, true); err != nil {
		t.Fatalf("SetFavorite on: %v", err)
	}
	favs, err := st.ListFavoriteUIDs(ctx, "owner")
	if err != nil {
		t.Fatalf("ListFavoriteUIDs: %v", err)
	}
	if len(favs) != 1 || favs[0] != "fav-1" {
		t.Fatalf("favoritos = %v, want [fav-1]", favs)
	}

	// Otro dueño NO puede destacar el contacto ajeno (owner-scoped).
	if err := st.SetFavorite(ctx, "intruso", c.UID, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("intruso destacó contacto ajeno: %v", err)
	}

	// El UID con prefijo urn:uuid: (como lo sirve CardDAV) resuelve al mismo
	// contacto: SetFavorite normaliza con CleanUID (defensa del desajuste
	// vCard/proyección).
	if err := st.SetFavorite(ctx, "owner", "urn:uuid:"+c.UID, false); err != nil {
		t.Fatalf("SetFavorite con UID prefijado: %v", err)
	}
	if favs, _ := st.ListFavoriteUIDs(ctx, "owner"); len(favs) != 0 {
		t.Fatalf("el UID prefijado no desmarcó el mismo contacto: %v", favs)
	}
	if err := st.SetFavorite(ctx, "owner", c.UID, true); err != nil { // re-destacar para el resto del test
		t.Fatalf("re-SetFavorite: %v", err)
	}

	// Borrado suave: el favorito se OCULTA del filtro.
	if err := st.DeleteContact(ctx, "owner", c.ID); err != nil {
		t.Fatalf("DeleteContact: %v", err)
	}
	favs, _ = st.ListFavoriteUIDs(ctx, "owner")
	if len(favs) != 0 {
		t.Fatalf("un favorito en la papelera no debe listarse: %v", favs)
	}

	// Restaurar lo REVIVE (el flag sigue puesto).
	if err := st.RestoreContact(ctx, "owner", c.ID); err != nil {
		t.Fatalf("RestoreContact: %v", err)
	}
	favs, _ = st.ListFavoriteUIDs(ctx, "owner")
	if len(favs) != 1 {
		t.Fatalf("el favorito no revivió al restaurar: %v", favs)
	}

	// Quitar destacado.
	if err := st.SetFavorite(ctx, "owner", c.UID, false); err != nil {
		t.Fatalf("SetFavorite off: %v", err)
	}
	favs, _ = st.ListFavoriteUIDs(ctx, "owner")
	if len(favs) != 0 {
		t.Fatalf("favoritos tras quitar = %v, want []", favs)
	}
}

// TestTrashLifecycle ejercita listar la papelera, restaurar y purgar, todo
// owner-scoped.
func TestTrashLifecycle(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	book, err := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner", Name: "personal"})
	if err != nil {
		t.Fatalf("CreateAddressBook: %v", err)
	}
	c, err := st.CreateContact(ctx, "owner", sampleContact(book.ID, "trash-1", "Grace Hopper"))
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}

	// Vivo: la papelera está vacía.
	if items, _ := st.ListTrash(ctx, "owner", 0); len(items) != 0 {
		t.Fatalf("papelera no vacía con contacto vivo: %+v", items)
	}

	// Borrado suave -> aparece en la papelera con su nombre y correo principal.
	if err := st.DeleteContact(ctx, "owner", c.ID); err != nil {
		t.Fatalf("DeleteContact: %v", err)
	}
	items, err := st.ListTrash(ctx, "owner", 0)
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(items) != 1 || items[0].ID != c.ID || items[0].FormattedName != "Grace Hopper" {
		t.Fatalf("papelera inesperada: %+v", items)
	}
	if items[0].PrimaryEmail != "ada@sytue.eu" {
		t.Errorf("correo principal en papelera = %q", items[0].PrimaryEmail)
	}

	// Otro dueño NO ve ni purga la papelera ajena.
	if other, _ := st.ListTrash(ctx, "intruso", 0); len(other) != 0 {
		t.Fatalf("intruso ve papelera ajena: %+v", other)
	}
	if err := st.PurgeContact(ctx, "intruso", c.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("intruso purgó contacto ajeno: %v", err)
	}

	// Restaurar -> fuera de la papelera y de nuevo vivo.
	if err := st.RestoreContact(ctx, "owner", c.ID); err != nil {
		t.Fatalf("RestoreContact: %v", err)
	}
	if items, _ := st.ListTrash(ctx, "owner", 0); len(items) != 0 {
		t.Fatalf("papelera no vacía tras restaurar: %+v", items)
	}
	if _, err := st.GetContact(ctx, "owner", c.ID); err != nil {
		t.Fatalf("contacto restaurado no está vivo: %v", err)
	}

	// Borrar de nuevo y PURGAR (borrado en duro): desaparece del todo.
	if err := st.DeleteContact(ctx, "owner", c.ID); err != nil {
		t.Fatalf("DeleteContact 2: %v", err)
	}
	if err := st.PurgeContact(ctx, "owner", c.ID); err != nil {
		t.Fatalf("PurgeContact: %v", err)
	}
	if items, _ := st.ListTrash(ctx, "owner", 0); len(items) != 0 {
		t.Fatalf("papelera no vacía tras purgar: %+v", items)
	}
	if _, err := st.GetContact(ctx, "owner", c.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("contacto purgado sigue accesible: %v", err)
	}
	// La restauración de algo purgado ya no encuentra nada.
	if err := st.RestoreContact(ctx, "owner", c.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("restaurar un purgado debe ser ErrNotFound: %v", err)
	}
}

// TestImportExportHelpers ejercita el dedupe (ContactEmailSet) y la fuente del
// export (ListOwnerContacts, que decodifica la vCard con direcciones incluidas).
func TestImportExportHelpers(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	book, err := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner", Name: "personal"})
	if err != nil {
		t.Fatalf("CreateAddressBook: %v", err)
	}

	// Contacto con dirección: se codifica la vCard como en el handler REST.
	c := domain.Contact{
		UID:           "imp-1",
		FormattedName: "Ada Lovelace",
		GivenName:     "Ada",
		FamilyName:    "Lovelace",
		Emails:        []domain.Email{{Email: "Ada@Sytue.eu", Type: domain.EmailWork, IsPrimary: true}},
		Addresses:     []domain.Address{{Street: "Calle Uno 1", Locality: "Madrid", Country: "España", Type: domain.AddressHome, IsPrimary: true}},
	}
	card, err := domain.EncodeContact(c, "")
	if err != nil {
		t.Fatalf("EncodeContact: %v", err)
	}
	if _, err := st.CreateContact(ctx, "owner", store.UpsertContact{
		AddressBookID: book.ID, Contact: c, CardData: card, ResourceName: domain.ResourceName(c.UID),
	}); err != nil {
		t.Fatalf("CreateContact: %v", err)
	}

	// Dedupe: el correo existente se devuelve en minúsculas.
	set, err := st.ContactEmailSet(ctx, book.ID)
	if err != nil {
		t.Fatalf("ContactEmailSet: %v", err)
	}
	if _, ok := set["ada@sytue.eu"]; !ok {
		t.Fatalf("ContactEmailSet no contiene el correo en minúsculas: %v", set)
	}

	// Export: la vCard se decodifica con su dirección (que la proyección no guarda).
	owned, err := st.ListOwnerContacts(ctx, "owner")
	if err != nil {
		t.Fatalf("ListOwnerContacts: %v", err)
	}
	if len(owned) != 1 || len(owned[0].Addresses) != 1 || owned[0].Addresses[0].Locality != "Madrid" {
		t.Fatalf("ListOwnerContacts sin la dirección decodificada: %+v", owned)
	}
	// Aislamiento por dueño.
	if other, _ := st.ListOwnerContacts(ctx, "intruso"); len(other) != 0 {
		t.Fatalf("intruso exporta contactos ajenos: %+v", other)
	}
}

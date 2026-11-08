// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integracion (PostgreSQL real via pgtest) de las acciones EN LOTE
// (multiseleccion) y de la FOTO de contacto (PHOTO embebida en la vCard). Verifican
// el `uid = ANY` en una sola consulta, el aislamiento por dueno (UID ajenos
// omitidos) y el round-trip de la foto por el vCard.
package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/contacts/db"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

// pngDataURI es un PNG 1x1 válido para probar la foto embebida.
const pngDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func sampleGroup(bookID uuid.UUID, uid, name string) store.UpsertGroup {
	g := domain.Group{UID: uid, Name: name}
	card, _ := domain.EncodeGroup(g, "")
	return store.UpsertGroup{AddressBookID: bookID, Group: g, CardData: card, ResourceName: domain.ResourceName(uid)}
}

// TestBatchTrashOwnerScoped verifica el envío a papelera EN LOTE por `uid = ANY`,
// con los UID ajenos omitidos y sin tocar los no seleccionados.
func TestBatchTrashOwnerScoped(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	book, err := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner", Name: "personal"})
	if err != nil {
		t.Fatalf("CreateAddressBook: %v", err)
	}
	for _, u := range []string{"b1", "b2", "b3"} {
		if _, err := st.CreateContact(ctx, "owner", sampleContact(book.ID, u, "C "+u)); err != nil {
			t.Fatalf("CreateContact %s: %v", u, err)
		}
	}
	// Contacto de OTRO dueño (mismo tenant).
	book2, err := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner2", Name: "personal2"})
	if err != nil {
		t.Fatalf("CreateAddressBook owner2: %v", err)
	}
	if _, err := st.CreateContact(ctx, "owner2", sampleContact(book2.ID, "x1", "Ajeno")); err != nil {
		t.Fatalf("CreateContact x1: %v", err)
	}

	// Lote: b1, b2 (propios) + x1 (ajeno) + inexistente -> solo 2 a la papelera.
	n, err := st.TrashContacts(ctx, "owner", []string{"b1", "b2", "x1", "nope"})
	if err != nil {
		t.Fatalf("TrashContacts: %v", err)
	}
	if n != 2 {
		t.Fatalf("trashed = %d, want 2 (ajenos/inexistentes omitidos)", n)
	}
	items, _ := st.ListTrash(ctx, "owner", 0)
	if len(items) != 2 {
		t.Fatalf("papelera del dueño = %d, want 2", len(items))
	}
	// x1 (ajeno) NO fue tocado: sigue vivo para owner2.
	if other, _ := st.ListTrash(ctx, "owner2", 0); len(other) != 0 {
		t.Fatalf("el contacto ajeno acabó en papelera: %+v", other)
	}
	// b3 (no seleccionado) sigue vivo.
	if live, _ := st.ListContacts(ctx, "owner", book.ID, 0); len(live) != 1 || live[0].UID != "b3" {
		t.Fatalf("contactos vivos = %+v, want solo b3", live)
	}

	// Un intruso no envía a la papelera contactos ajenos (0 afectados).
	if n, _ := st.TrashContacts(ctx, "intruso", []string{"b3"}); n != 0 {
		t.Fatalf("intruso envió a papelera ajena: n=%d", n)
	}
	// UID con prefijo urn:uuid: resuelve igual (CleanUID).
	if n, _ := st.TrashContacts(ctx, "owner", []string{"urn:uuid:b3"}); n != 1 {
		t.Fatalf("UID prefijado no resolvió: n=%d", n)
	}
}

// TestBatchFavoritesOwnerScoped verifica destacar/quitar EN LOTE por `uid = ANY`.
func TestBatchFavoritesOwnerScoped(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	book, _ := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner", Name: "personal"})
	for _, u := range []string{"f1", "f2"} {
		if _, err := st.CreateContact(ctx, "owner", sampleContact(book.ID, u, "C "+u)); err != nil {
			t.Fatalf("CreateContact %s: %v", u, err)
		}
	}
	book2, _ := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner2", Name: "p2"})
	if _, err := st.CreateContact(ctx, "owner2", sampleContact(book2.ID, "x1", "Ajeno")); err != nil {
		t.Fatalf("CreateContact x1: %v", err)
	}

	n, err := st.SetFavorites(ctx, "owner", []string{"f1", "f2", "x1"}, true)
	if err != nil {
		t.Fatalf("SetFavorites: %v", err)
	}
	if n != 2 {
		t.Fatalf("updated = %d, want 2 (ajeno omitido)", n)
	}
	if favs, _ := st.ListFavoriteUIDs(ctx, "owner"); len(favs) != 2 {
		t.Fatalf("favoritos = %v, want 2", favs)
	}
	if n, _ := st.SetFavorites(ctx, "owner", []string{"f1"}, false); n != 1 {
		t.Fatalf("quitar destacado en lote: n=%d, want 1", n)
	}
	if favs, _ := st.ListFavoriteUIDs(ctx, "owner"); len(favs) != 1 || favs[0] != "f2" {
		t.Fatalf("favoritos tras quitar = %v, want [f2]", favs)
	}
}

// TestBatchGroupMembers verifica añadir/quitar EN LOTE la pertenencia a un grupo,
// con la vCard MEMBER sincronizada y owner-scoped.
func TestBatchGroupMembers(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	book, _ := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner", Name: "personal"})
	for _, u := range []string{"g1", "g2"} {
		if _, err := st.CreateContact(ctx, "owner", sampleContact(book.ID, u, "C "+u)); err != nil {
			t.Fatalf("CreateContact %s: %v", u, err)
		}
	}
	grp, err := st.CreateGroup(ctx, "owner", sampleGroup(book.ID, "grp-1", "Familia"))
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	// Añadir g1, g2 en lote.
	out, err := st.ModifyGroupMembers(ctx, "owner", grp.ID, []string{"g1", "g2"}, true)
	if err != nil {
		t.Fatalf("ModifyGroupMembers add: %v", err)
	}
	if len(out.Members) != 2 {
		t.Fatalf("miembros = %v, want 2", out.Members)
	}
	// La vCard del grupo (card_objects.data) refleja los MEMBER.
	card, err := st.GetCardObjectByResource(ctx, book.ID, domain.ResourceName("grp-1"))
	if err != nil {
		t.Fatalf("GetCardObjectByResource: %v", err)
	}
	if !strings.Contains(card.Data, "g1") || !strings.Contains(card.Data, "g2") {
		t.Fatalf("la vCard del grupo no sincronizó los MEMBER: %q", card.Data)
	}

	// Añadir de nuevo g1 (idempotente): sigue habiendo 2.
	if out, _ := st.ModifyGroupMembers(ctx, "owner", grp.ID, []string{"g1"}, true); len(out.Members) != 2 {
		t.Fatalf("add duplicado cambió el conteo: %v", out.Members)
	}

	// Quitar g1 en lote.
	out, err = st.ModifyGroupMembers(ctx, "owner", grp.ID, []string{"g1"}, false)
	if err != nil {
		t.Fatalf("ModifyGroupMembers remove: %v", err)
	}
	if len(out.Members) != 1 || out.Members[0] != "g2" {
		t.Fatalf("miembros tras quitar = %v, want [g2]", out.Members)
	}

	// Owner-scoped: un intruso no modifica el grupo ajeno.
	if _, err := st.ModifyGroupMembers(ctx, "intruso", grp.ID, []string{"g2"}, false); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("intruso modificó grupo ajeno: %v", err)
	}
}

// TestContactPhotoRoundTrip verifica set / round-trip / quitar de la foto embebida
// y el aislamiento por dueño.
func TestContactPhotoRoundTrip(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	book, _ := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner", Name: "personal"})
	c, err := st.CreateContact(ctx, "owner", sampleContact(book.ID, "ph-1", "Ada"))
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}

	// Sin foto todavía.
	if _, _, err := st.GetContactPhoto(ctx, "owner", c.UID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetContactPhoto sin foto: %v", err)
	}

	// Definir la foto (embebida en la vCard).
	if err := st.SetContactPhoto(ctx, "owner", c.UID, pngDataURI); err != nil {
		t.Fatalf("SetContactPhoto: %v", err)
	}
	got, etag, err := st.GetContactPhoto(ctx, "owner", c.UID)
	if err != nil {
		t.Fatalf("GetContactPhoto: %v", err)
	}
	if got != pngDataURI {
		t.Errorf("data URI no round-trip")
	}
	if etag == uuid.Nil {
		t.Error("etag nulo tras definir foto")
	}
	// Round-trip por la vCard almacenada (card_objects.data).
	card, _ := st.GetCardObjectByResource(ctx, book.ID, domain.ResourceName(c.UID))
	if !domain.CardHasEmbeddedPhoto(card.Data) {
		t.Fatalf("la vCard no conserva la PHOTO embebida")
	}

	// Aislamiento por dueño: un intruso no ve ni define la foto ajena.
	if _, _, err := st.GetContactPhoto(ctx, "intruso", c.UID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("intruso vio foto ajena: %v", err)
	}
	if err := st.SetContactPhoto(ctx, "intruso", c.UID, pngDataURI); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("intruso definió foto ajena: %v", err)
	}

	// Quitar la foto.
	if err := st.RemoveContactPhoto(ctx, "owner", c.UID); err != nil {
		t.Fatalf("RemoveContactPhoto: %v", err)
	}
	if _, _, err := st.GetContactPhoto(ctx, "owner", c.UID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("la foto no se quitó: %v", err)
	}
}

// TestHasPhotoEnElListado: el listado dice qué contactos tienen foto, con la regla de
// `hasEmbeddedPhoto` de la web —la PHOTO embebida, plegada o con grupo, sí; una URL,
// no— y sigue a la tarjeta por cualquier camino que la escriba.
func TestHasPhotoEnElListado(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	book, _ := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner", Name: "personal"})

	conFoto := func() map[string]bool {
		t.Helper()
		lista, err := st.ListContacts(ctx, "owner", book.ID, 0)
		if err != nil {
			t.Fatalf("ListContacts: %v", err)
		}
		m := map[string]bool{}
		for _, c := range lista {
			m[c.UID] = c.HasPhoto
		}
		return m
	}

	c, err := st.CreateContact(ctx, "owner", sampleContact(book.ID, "hp-1", "Ada"))
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	if conFoto()[c.UID] {
		t.Fatal("recién creado, sin foto, sale con foto")
	}
	if err := st.SetContactPhoto(ctx, "owner", c.UID, pngDataURI); err != nil {
		t.Fatalf("SetContactPhoto: %v", err)
	}
	if !conFoto()[c.UID] {
		t.Fatal("con la foto puesta, el listado no lo dice")
	}
	if got, _ := st.GetContact(ctx, "owner", c.ID); !got.HasPhoto {
		t.Fatal("GetContact no dice que tiene foto")
	}
	if err := st.RemoveContactPhoto(ctx, "owner", c.UID); err != nil {
		t.Fatalf("RemoveContactPhoto: %v", err)
	}
	if conFoto()[c.UID] {
		t.Fatal("quitada la foto, el listado sigue diciendo que la tiene")
	}

	// Tarjetas escritas tal cual, como llegan por CardDAV.
	tarjetas := map[string]struct {
		datos string
		foto  bool
	}{
		// Plegada justo antes del valor (RFC 6350 §3.2).
		"hp-plegada": {"BEGIN:VCARD\r\nVERSION:4.0\r\nUID:hp-plegada\r\nFN:Plegada\r\nPHOTO;MEDIATYPE=image/png:\r\n data:image/png;base64,iVBORw0KGgo=\r\nEND:VCARD\r\n", true},
		"hp-grupo":   {"BEGIN:VCARD\r\nVERSION:4.0\r\nUID:hp-grupo\r\nFN:Grupo\r\nitem1.photo:DATA:IMAGE/JPEG;base64,/9j/4AAQ\r\nEND:VCARD\r\n", true},
		"hp-url":     {"BEGIN:VCARD\r\nVERSION:4.0\r\nUID:hp-url\r\nFN:Url\r\nPHOTO:https://ejemplo.es/ana.jpg\r\nEND:VCARD\r\n", false},
		"hp-nota":    {"BEGIN:VCARD\r\nVERSION:4.0\r\nUID:hp-nota\r\nFN:Nota\r\nNOTE:la PHOTO:data:image/png va aparte\r\nEND:VCARD\r\n", false},
	}
	for uid, tc := range tarjetas {
		in := sampleContact(book.ID, uid, uid)
		in.CardData = tc.datos
		if _, err := st.CreateContact(ctx, "owner", in); err != nil {
			t.Fatalf("CreateContact %s: %v", uid, err)
		}
	}
	vistos := conFoto()
	for uid, tc := range tarjetas {
		if vistos[uid] != tc.foto {
			t.Errorf("%s: has_photo = %v, want %v", uid, vistos[uid], tc.foto)
		}
	}
}

// TestBatchRestoreUltimoDeCadaUID verifica el «Deshacer» del lote: UNA operación que
// devuelve lo último que se tiró de cada UID (las dos copias de un UID que vive en dos
// libretas, no una copia más vieja), sin ajenos, sin chocar con un vivo y con un
// recuento comparable al de TrashContacts.
func TestBatchRestoreUltimoDeCadaUID(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	bookA, err := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner", Name: "a"})
	if err != nil {
		t.Fatalf("CreateAddressBook a: %v", err)
	}
	bookB, err := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner", Name: "b"})
	if err != nil {
		t.Fatalf("CreateAddressBook b: %v", err)
	}
	for _, c := range []struct {
		book uuid.UUID
		uid  string
	}{{bookA.ID, "d1"}, {bookA.ID, "d2"}, {bookA.ID, "d3"}, {bookB.ID, "d1"}, {bookB.ID, "d3"}} {
		if _, err := st.CreateContact(ctx, "owner", sampleContact(c.book, c.uid, "C "+c.uid)); err != nil {
			t.Fatalf("CreateContact %s: %v", c.uid, err)
		}
	}
	ajeno, err := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner2", Name: "x"})
	if err != nil {
		t.Fatalf("CreateAddressBook owner2: %v", err)
	}
	if _, err := st.CreateContact(ctx, "owner2", sampleContact(ajeno.ID, "x1", "Ajeno")); err != nil {
		t.Fatalf("CreateContact x1: %v", err)
	}
	if _, err := st.TrashContacts(ctx, "owner2", []string{"x1"}); err != nil {
		t.Fatalf("TrashContacts owner2: %v", err)
	}

	// d1 vive en dos libretas: el lote tira tres filas y deshacer devuelve las tres.
	trashed, err := st.TrashContacts(ctx, "owner", []string{"d1", "d2"})
	if err != nil || trashed != 3 {
		t.Fatalf("TrashContacts = %d, %v; want 3", trashed, err)
	}
	if n, _ := st.RestoreContacts(ctx, "intruso", []string{"d1", "d2"}); n != 0 {
		t.Fatalf("un intruso restauró %d contactos ajenos", n)
	}
	n, err := st.RestoreContacts(ctx, "owner", []string{"d1", "urn:uuid:d2", "x1", "nope"})
	if err != nil || n != trashed {
		t.Fatalf("RestoreContacts = %d, %v; want %d (ni ajenos ni inexistentes)", n, err, trashed)
	}
	if items, _ := st.ListTrash(ctx, "owner", 0); len(items) != 0 {
		t.Fatalf("papelera tras deshacer = %+v, want vacía", items)
	}
	if other, _ := st.ListTrash(ctx, "owner2", 0); len(other) != 1 {
		t.Fatalf("el contacto ajeno salió de su papelera: %+v", other)
	}

	// d3: se tiran las dos copias, se recupera una a mano y se vuelve a tirar.
	// Deshacer ese segundo borrado devuelve esa copia, no la otra (que es de antes).
	if n, _ := st.TrashContacts(ctx, "owner", []string{"d3"}); n != 2 {
		t.Fatalf("TrashContacts d3 = %d, want 2", n)
	}
	items, _ := st.ListTrash(ctx, "owner", 0)
	if len(items) != 2 {
		t.Fatalf("papelera con d3 = %+v, want 2", items)
	}
	if err := st.RestoreContact(ctx, "owner", items[0].ID); err != nil {
		t.Fatalf("RestoreContact: %v", err)
	}
	vivoEn := map[uuid.UUID]bool{}
	for _, b := range []uuid.UUID{bookA.ID, bookB.ID} {
		live, _ := st.ListContacts(ctx, "owner", b, 0)
		for _, c := range live {
			if c.UID == "d3" {
				vivoEn[b] = true
			}
		}
	}
	if len(vivoEn) != 1 {
		t.Fatalf("d3 debía estar vivo en una sola libreta: %v", vivoEn)
	}
	if n, _ := st.TrashContacts(ctx, "owner", []string{"d3"}); n != 1 {
		t.Fatalf("segundo TrashContacts d3 = %d, want 1", n)
	}
	if n, _ := st.RestoreContacts(ctx, "owner", []string{"d3"}); n != 1 {
		t.Fatalf("RestoreContacts d3 = %d, want 1 (solo el último borrado)", n)
	}
	for b := range vivoEn {
		live, _ := st.ListContacts(ctx, "owner", b, 0)
		hay := false
		for _, c := range live {
			hay = hay || c.UID == "d3"
		}
		if !hay {
			t.Fatalf("volvió la copia vieja de d3, no la del último borrado")
		}
	}
	if items, _ := st.ListTrash(ctx, "owner", 0); len(items) != 1 {
		t.Fatalf("la copia vieja de d3 debía seguir en la papelera: %+v", items)
	}

	// Un UID que ya tiene un vivo en su libreta no vuelve (chocaría) y no cuenta.
	if n, _ := st.TrashContacts(ctx, "owner", []string{"d2"}); n != 1 {
		t.Fatalf("TrashContacts d2 = %d, want 1", n)
	}
	if _, err := st.CreateContact(ctx, "owner", sampleContact(bookA.ID, "d2", "Otro d2")); err != nil {
		t.Fatalf("CreateContact d2 nuevo: %v", err)
	}
	if n, err := st.RestoreContacts(ctx, "owner", []string{"d2"}); err != nil || n != 0 {
		t.Fatalf("RestoreContacts con choque = %d, %v; want 0 sin error", n, err)
	}
}

// TestBatchRestoreNoReviveLoYaRecuperado: si en los diez segundos del aviso la persona
// recupera a mano desde la Papelera lo que acaba de tirar, «Deshacer» no puede revivir
// en su lugar una copia más vieja del mismo UID que tiró otro día en otra libreta.
func TestBatchRestoreNoReviveLoYaRecuperado(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	bookA, err := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner", Name: "a"})
	if err != nil {
		t.Fatalf("CreateAddressBook a: %v", err)
	}
	bookB, err := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner", Name: "b"})
	if err != nil {
		t.Fatalf("CreateAddressBook b: %v", err)
	}
	// Ayer: d1 en B, a la papelera.
	if _, err := st.CreateContact(ctx, "owner", sampleContact(bookB.ID, "d1", "Copia de ayer")); err != nil {
		t.Fatalf("CreateContact d1 en B: %v", err)
	}
	if n, _ := st.TrashContacts(ctx, "owner", []string{"d1"}); n != 1 {
		t.Fatalf("TrashContacts d1 (ayer) = %d, want 1", n)
	}
	// Hoy: d1 en A, a la papelera desde el lote, y recuperado a mano.
	if _, err := st.CreateContact(ctx, "owner", sampleContact(bookA.ID, "d1", "Copia de hoy")); err != nil {
		t.Fatalf("CreateContact d1 en A: %v", err)
	}
	if n, _ := st.TrashContacts(ctx, "owner", []string{"d1"}); n != 1 {
		t.Fatalf("TrashContacts d1 (hoy) = %d, want 1", n)
	}
	items, _ := st.ListTrash(ctx, "owner", 0)
	if len(items) != 2 || items[0].FormattedName != "Copia de hoy" {
		t.Fatalf("papelera = %+v, want la de hoy primero", items)
	}
	if err := st.RestoreContact(ctx, "owner", items[0].ID); err != nil {
		t.Fatalf("RestoreContact: %v", err)
	}

	// «Deshacer» del aviso: no hay nada suyo que deshacer.
	if n, err := st.RestoreContacts(ctx, "owner", []string{"d1"}); err != nil || n != 0 {
		t.Fatalf("RestoreContacts tras recuperar a mano = %d, %v; want 0", n, err)
	}
	if live, _ := st.ListContacts(ctx, "owner", bookB.ID, 0); len(live) != 0 {
		t.Fatalf("volvió la copia de ayer de B, que nadie pidió: %+v", live)
	}
	if items, _ := st.ListTrash(ctx, "owner", 0); len(items) != 1 || items[0].FormattedName != "Copia de ayer" {
		t.Fatalf("la copia de ayer debía seguir en la papelera: %+v", items)
	}
}

// TestEachCardByIDSoloLasVivasDelDuenoEnOrden: la exportación de una selección trae, en
// UNA consulta, la vCard guardada de cada contacto vivo del dueño, en el orden pedido;
// ni ajenos, ni borrados.
func TestEachCardByIDSoloLasVivasDelDuenoEnOrden(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	book, err := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner", Name: "a"})
	if err != nil {
		t.Fatalf("CreateAddressBook: %v", err)
	}
	ids := map[string]uuid.UUID{}
	for _, u := range []string{"e1", "e2", "e3"} {
		c, err := st.CreateContact(ctx, "owner", sampleContact(book.ID, u, "C "+u))
		if err != nil {
			t.Fatalf("CreateContact %s: %v", u, err)
		}
		ids[u] = c.ID
	}
	ajena, _ := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "owner2", Name: "x"})
	x, err := st.CreateContact(ctx, "owner2", sampleContact(ajena.ID, "x1", "Ajeno"))
	if err != nil {
		t.Fatalf("CreateContact x1: %v", err)
	}
	if _, err := st.TrashContacts(ctx, "owner", []string{"e2"}); err != nil {
		t.Fatalf("TrashContacts: %v", err)
	}

	var cards []string
	err = st.EachCardByID(ctx, "owner", []uuid.UUID{ids["e3"], x.ID, ids["e2"], ids["e1"], uuid.New()}, func(c string) error {
		cards = append(cards, c)
		return nil
	})
	if err != nil {
		t.Fatalf("EachCardByID: %v", err)
	}
	if len(cards) != 2 || !strings.Contains(cards[0], "UID:e3") || !strings.Contains(cards[1], "UID:e1") {
		t.Fatalf("EachCardByID = %q; want e3 y e1, en ese orden", cards)
	}

	// Un error de quien recibe para la lectura y se devuelve.
	parar := errors.New("parar")
	n := 0
	err = st.EachCardByID(ctx, "owner", []uuid.UUID{ids["e3"], ids["e1"]}, func(string) error { n++; return parar })
	if !errors.Is(err, parar) || n != 1 {
		t.Fatalf("EachCardByID con fn que falla: err=%v tras %d tarjetas; want parar tras 1", err, n)
	}
}

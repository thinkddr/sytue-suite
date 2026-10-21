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

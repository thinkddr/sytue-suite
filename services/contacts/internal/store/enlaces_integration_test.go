// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/contacts/db"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

// Los sitios web, las relaciones y el aniversario (migración 0008) se guardan, se leen
// por la ficha y por la página del listado, y se quitan al no reenviarlos.
func TestWebRelacionesYAniversarioSeGuardanYSeLeen(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	book, err := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "user-1", Name: "personal"})
	if err != nil {
		t.Fatal(err)
	}
	boda := time.Date(2012, 6, 9, 0, 0, 0, 0, time.UTC)
	in := sampleContact(book.ID, "c-web", "Ada Lovelace")
	in.Contact.URLs = []domain.URL{{URL: "https://ada.example", Type: "work"}}
	in.Contact.Related = []domain.Related{{Name: "William King", Type: "spouse"}, {Name: "Mary"}}
	in.Contact.Anniversary = &boda
	c, err := st.CreateContact(ctx, "user-1", in)
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	comprobar := func(donde string, got domain.Contact) {
		t.Helper()
		if !reflect.DeepEqual(got.URLs, in.Contact.URLs) || !reflect.DeepEqual(got.Related, in.Contact.Related) {
			t.Errorf("%s: %+v / %+v", donde, got.URLs, got.Related)
		}
		if got.Anniversary == nil || !got.Anniversary.Equal(boda) {
			t.Errorf("%s: aniversario %v", donde, got.Anniversary)
		}
	}
	got, err := st.GetContact(ctx, "user-1", c.ID)
	if err != nil {
		t.Fatal(err)
	}
	comprobar("GetContact", got)
	page, err := st.ListContactsPage(ctx, "user-1", book.ID, nil, 10)
	if err != nil || len(page.Contacts) != 1 {
		t.Fatalf("ListContactsPage: %+v, %v", page.Contacts, err)
	}
	comprobar("ListContactsPage", page.Contacts[0])

	in.Contact.URLs, in.Contact.Related, in.Contact.Anniversary = nil, nil, nil
	got, err = st.UpdateContact(ctx, "user-1", c.ID, in, nil)
	if err != nil || len(got.URLs) != 0 || len(got.Related) != 0 || got.Anniversary != nil {
		t.Fatalf("UpdateContact: %+v, %v", got, err)
	}
	// Sin nada, la columna es '[]' y no el `null` de JSON.
	var urls string
	if err := h.PoolSinRLS(t).QueryRow(context.Background(),
		`SELECT urls::text FROM contacts WHERE id = $1`, c.ID).Scan(&urls); err != nil || urls != "[]" {
		t.Errorf("urls = %q, %v", urls, err)
	}
}

// La 0008 rellena los contactos que ya estaban leyendo su tarjeta, con la misma regla
// que el servicio: lo que este proyecta, la migración también, y lo que oculta, igual.
func TestMigracion0008RellenaLosEnlacesQueYaEstaban(t *testing.T) {
	antes := fstest.MapFS{}
	err := fs.WalkDir(db.Migrations, db.MigrationsDir, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.Contains(ruta, "/0008_") {
			return err
		}
		datos, err := fs.ReadFile(db.Migrations, ruta)
		antes[ruta] = &fstest.MapFile{Data: datos}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	subida, err := fs.ReadFile(db.Migrations, db.MigrationsDir+"/0008_web_relaciones_aniversario.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	h := pgtest.New(t, antes, db.MigrationsDir)
	sql := h.PoolSinRLS(t)
	ctx := context.Background()

	var libro string
	if err := sql.QueryRow(ctx, `INSERT INTO address_books (tenant, owner_sub, name) VALUES ('acme', 'u', 'l') RETURNING id`).Scan(&libro); err != nil {
		t.Fatal(err)
	}
	casos := []struct {
		uid, lineas string
		urls        string // el jsonb que debe quedar
		related     string
		aniversario string
	}{
		{"web", "URL;TYPE=work:https://acme.example/equipo\r\nitem1.URL:http://ada.example", `[{"url": "https://acme.example/equipo", "type": "work"}, {"url": "http://ada.example", "type": "other"}]`, "[]", ""},
		{"apple", "URL;TYPE=HOME,pref:https://casa.example", `[{"url": "https://casa.example", "type": "home"}]`, "[]", ""},
		{"ocultos", "URL:ftp://viejo.example\r\nURL:www.sin-esquema.example\r\nRELATED:urn:uuid:1234", "[]", "[]", ""},
		{"personas", "RELATED;TYPE=spouse;VALUE=text:Ana\\, la de Lugo\r\nRELATED;VALUE=text:Pepe\r\nRELATED;TYPE=friend:Luis", "[]", `[{"name": "Ana, la de Lugo", "type": "spouse"}, {"name": "Pepe"}, {"name": "Luis", "type": "friend"}]`, ""},
		{"boda", "ANNIVERSARY:20120609", "[]", "[]", "2012-06-09"},
		{"boda-ext", "ANNIVERSARY;VALUE=date:2001-02-28", "[]", "[]", "2001-02-28"},
		{"parcial", "ANNIVERSARY:--0609", "[]", "[]", ""},
		{"imposible", "ANNIVERSARY:2001-02-29", "[]", "[]", ""},
		{"plegado", "URL:https://plegado.exam\r\n ple", `[{"url": "https://plegado.example", "type": "other"}]`, "[]", ""},
		{"ninguno", "NOTE:nada", "[]", "[]", ""},
	}
	for _, c := range casos {
		datos := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:" + c.uid + "\r\n" + c.lineas + "\r\nEND:VCARD\r\n"
		var objeto string
		if err := sql.QueryRow(ctx, `INSERT INTO card_objects (tenant, address_book_id, uid, resource_name, data)
			VALUES ('acme', $1, $2, $2 || '.vcf', $3) RETURNING id`, libro, c.uid, datos).Scan(&objeto); err != nil {
			t.Fatal(err)
		}
		if _, err := sql.Exec(ctx, `INSERT INTO contacts (tenant, address_book_id, object_id, uid, formatted_name)
			VALUES ('acme', $1, $2, $3, $3)`, libro, objeto, c.uid); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := sql.Exec(ctx, string(subida)); err != nil {
		t.Fatalf("0008: %v", err)
	}

	for _, c := range casos {
		var urls, related, aniversario, datos string
		if err := sql.QueryRow(ctx, `SELECT c.urls::text, c.related::text, coalesce(c.anniversary::text, ''), o.data
			FROM contacts c JOIN card_objects o ON o.id = c.object_id WHERE c.uid = $1`, c.uid).
			Scan(&urls, &related, &aniversario, &datos); err != nil {
			t.Fatal(err)
		}
		if urls != c.urls || related != c.related || aniversario != c.aniversario {
			t.Errorf("%s: %s / %s / %q; quiero %s / %s / %q", c.uid, urls, related, aniversario, c.urls, c.related, c.aniversario)
		}
		// La misma tarjeta, leída por el servicio, dice lo mismo que la migración.
		d, err := domain.DecodeContact(datos)
		if err != nil {
			t.Fatal(err)
		}
		var mURLs []domain.URL
		var mRelated []domain.Related
		if err := json.Unmarshal([]byte(urls), &mURLs); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(related), &mRelated); err != nil {
			t.Fatal(err)
		}
		if len(mURLs) != len(d.URLs) || (len(d.URLs) > 0 && !reflect.DeepEqual(mURLs, d.URLs)) {
			t.Errorf("%s: el servicio lee %+v y la migración %+v", c.uid, d.URLs, mURLs)
		}
		if len(mRelated) != len(d.Related) || (len(d.Related) > 0 && !reflect.DeepEqual(mRelated, d.Related)) {
			t.Errorf("%s: el servicio lee %+v y la migración %+v", c.uid, d.Related, mRelated)
		}
		var dAniv string
		if d.Anniversary != nil {
			dAniv = d.Anniversary.Format(time.DateOnly)
		}
		if dAniv != aniversario {
			t.Errorf("%s: el servicio lee el aniversario %q y la migración %q", c.uid, dAniv, aniversario)
		}
	}
	var forzada bool
	if err := sql.QueryRow(ctx, `SELECT relforcerowsecurity FROM pg_class WHERE relname = 'card_objects'`).Scan(&forzada); err != nil || !forzada {
		t.Errorf("la 0008 deja card_objects sin FORCE ROW LEVEL SECURITY (%v)", err)
	}
}

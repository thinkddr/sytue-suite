// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/contacts/db"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

// El cumpleaños sin año (migración 0007) se guarda, se lee por los dos caminos de
// lectura (ficha y página del listado) y se cambia por una fecha completa.
func TestCumpleanosSinAnoSeGuardaYSeLee(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	book, err := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "user-1", Name: "personal"})
	if err != nil {
		t.Fatal(err)
	}
	in := sampleContact(book.ID, "c-cumple", "Ada Lovelace")
	in.Contact.BirthdayNoYear = "--12-10"
	c, err := st.CreateContact(ctx, "user-1", in)
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	if got, err := st.GetContact(ctx, "user-1", c.ID); err != nil || got.BirthdayNoYear != "--12-10" || got.Birthday != nil {
		t.Fatalf("GetContact: %q / %v, %v", got.BirthdayNoYear, got.Birthday, err)
	}
	page, err := st.ListContactsPage(ctx, "user-1", book.ID, nil, 10)
	if err != nil || len(page.Contacts) != 1 || page.Contacts[0].BirthdayNoYear != "--12-10" {
		t.Fatalf("ListContactsPage: %+v, %v", page.Contacts, err)
	}

	completo := time.Date(1815, 12, 10, 0, 0, 0, 0, time.UTC)
	in.Contact.Birthday, in.Contact.BirthdayNoYear = &completo, ""
	got, err := st.UpdateContact(ctx, "user-1", c.ID, in, nil)
	if err != nil || got.BirthdayNoYear != "" || got.Birthday == nil || got.Birthday.Year() != 1815 {
		t.Fatalf("UpdateContact: %q / %v, %v", got.BirthdayNoYear, got.Birthday, err)
	}

	// La base rechaza lo que el servicio no escribiría nunca.
	if _, err := h.PoolSinRLS(t).Exec(context.Background(),
		`UPDATE contacts SET birthday_no_year = '--13-01' WHERE id = $1`, c.ID); err == nil {
		t.Error("la columna admite «--13-01»")
	}
}

// La 0007 rellena los contactos que ya estaban leyendo su tarjeta: el «--MMDD» que la
// proyección vieja descartaba y el año de relleno de Apple que tomaba por real. Lo
// demás —una fecha de verdad, un texto, un 31 de febrero— no se toca.
func TestMigracion0007RellenaLosCumpleanosQueYaEstaban(t *testing.T) {
	antes := fstest.MapFS{}
	err := fs.WalkDir(db.Migrations, db.MigrationsDir, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.Contains(ruta, "/0007_") {
			return err
		}
		datos, err := fs.ReadFile(db.Migrations, ruta)
		antes[ruta] = &fstest.MapFile{Data: datos}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	subida, err := fs.ReadFile(db.Migrations, db.MigrationsDir+"/0007_cumple_sin_ano.up.sql")
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
		uid, bday, proyectado string // proyectado: el `birthday` que dejó la proyección vieja
		completo, sinAno      string
	}{
		{"vcard4", "BDAY:--0215", "", "", "--02-15"},
		{"extendido", "item1.BDAY;VALUE=date:--12-31", "", "", "--12-31"},
		{"plegado", "BDAY:--02\r\n 29", "", "", "--02-29"},
		{"apple", "BDAY;X-APPLE-OMIT-YEAR=1604:1604-12-10", "1604-12-10", "", "--12-10"},
		{"completo", "BDAY:1990-05-04", "1990-05-04", "1990-05-04", ""},
		{"imposible", "BDAY:--0231", "", "", ""},
		{"texto", "BDAY:primavera", "", "", ""},
		{"ninguno", "NOTE:sin cumpleaños", "", "", ""},
	}
	for _, c := range casos {
		datos := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:" + c.uid + "\r\n" + c.bday + "\r\nEND:VCARD\r\n"
		var objeto string
		if err := sql.QueryRow(ctx, `INSERT INTO card_objects (tenant, address_book_id, uid, resource_name, data)
			VALUES ('acme', $1, $2, $2 || '.vcf', $3) RETURNING id`, libro, c.uid, datos).Scan(&objeto); err != nil {
			t.Fatal(err)
		}
		if _, err := sql.Exec(ctx, `INSERT INTO contacts (tenant, address_book_id, object_id, uid, formatted_name, birthday)
			VALUES ('acme', $1, $2, $3, $3, NULLIF($4, '')::date)`, libro, objeto, c.uid, c.proyectado); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := sql.Exec(ctx, string(subida)); err != nil {
		t.Fatalf("0007: %v", err)
	}

	for _, c := range casos {
		var completo, sinAno string
		if err := sql.QueryRow(ctx, `SELECT coalesce(birthday::text, ''), birthday_no_year FROM contacts WHERE uid = $1`, c.uid).
			Scan(&completo, &sinAno); err != nil {
			t.Fatal(err)
		}
		if completo != c.completo || sinAno != c.sinAno {
			t.Errorf("%s: %q / %q; quiero %q / %q", c.uid, completo, sinAno, c.completo, c.sinAno)
		}
		// La misma tarjeta, leída por el servicio, dice lo mismo que la migración.
		var datos string
		if err := sql.QueryRow(ctx, `SELECT data FROM card_objects WHERE uid = $1`, c.uid).Scan(&datos); err != nil {
			t.Fatal(err)
		}
		if d, err := domain.DecodeContact(datos); err == nil && d.BirthdayNoYear != c.sinAno {
			t.Errorf("%s: el servicio lee %q y la migración %q", c.uid, d.BirthdayNoYear, sinAno)
		}
	}
	var forzada bool
	if err := sql.QueryRow(ctx, `SELECT relforcerowsecurity FROM pg_class WHERE relname = 'contacts'`).Scan(&forzada); err != nil || !forzada {
		t.Errorf("la 0007 deja contacts sin FORCE ROW LEVEL SECURITY (%v)", err)
	}
}

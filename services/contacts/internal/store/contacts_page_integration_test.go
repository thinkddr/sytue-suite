// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package store_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/contacts/db"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

func TestContactPageNoTruncaNiCruzaPersonaOTenant(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	book, err := st.CreateAddressBook(ctx, store.NewAddressBook{OwnerSub: "alice", Name: "personal", DisplayName: "Personal"})
	if err != nil {
		t.Fatal(err)
	}
	for i := range 601 {
		in := sampleContact(book.ID, fmt.Sprintf("uid-%04d", i), fmt.Sprintf("Nombre %04d", i))
		in.Contact.FamilyName = []string{"Álvaro", "I", "İ", "z"}[i%4]
		if _, err := st.CreateContact(ctx, "alice", in); err != nil {
			t.Fatal(err)
		}
	}
	started := time.Now()
	legacy, err := st.ListContacts(ctx, "alice", book.ID, 500)
	if err != nil || len(legacy) != 500 {
		t.Fatalf("base legacy=%d err=%v", len(legacy), err)
	}
	var after *store.ContactCursor
	seen := map[string]bool{}
	pages := 0
	for {
		page, err := st.ListContactsPage(ctx, "alice", book.ID, after, 100)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, c := range page.Contacts {
			if seen[c.ID.String()] {
				t.Fatalf("duplicado %s", c.ID)
			}
			seen[c.ID.String()] = true
			if len(c.Emails) != 1 || len(c.Phones) != 1 {
				t.Fatal("canales incompletos")
			}
		}
		if page.Next == nil {
			break
		}
		after = page.Next
		for _, other := range []struct{ tenant, subject string }{{"acme", "bob"}, {"otra", "alice"}} {
			if _, err := st.ListContactsPage(tenantCtx(other.tenant), other.subject, book.ID, after, 100); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("cruzó %v: %v", other, err)
			}
		}
	}
	if len(seen) != 601 || pages != 7 {
		t.Fatalf("n=%d pages=%d", len(seen), pages)
	}
	t.Logf("601 contactos,7paginas,sin duplicados,canales completos: %s", time.Since(started))
}

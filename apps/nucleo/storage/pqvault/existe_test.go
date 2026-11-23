// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pqvault

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// HasRecords no crea la bóveda, distingue «sin bóveda», «vacía» y «con datos», y
// ante un fallo del servidor devuelve el fallo, nunca «vacía».
func TestHasRecordsNeverCreatesAndNeverGuesses(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "persona-1", Product: "drive"}
	fake, cfg := newFake(t, scope)

	has, err := HasRecords(t.Context(), cfg.Server, scope)
	if err != nil || has {
		t.Fatalf("sin bóveda no hay registros: %v %v", has, err)
	}
	if len(fake.Resets()) != 0 {
		t.Fatal("preguntar no puede crear la bóveda")
	}

	vault, err := Open(t.Context(), withKey(cfg, unlockKey(3)))
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	if has, err := HasRecords(t.Context(), cfg.Server, scope); err != nil || has {
		t.Fatalf("una bóveda recién creada está vacía: %v %v", has, err)
	}
	if _, err := vault.Put(t.Context(), "txn_1", record(scope, "obj_1", "rev_1", []byte(`{}`))); err != nil {
		t.Fatal(err)
	}
	if has, err := HasRecords(t.Context(), cfg.Server, scope); err != nil || !has {
		t.Fatalf("con un registro, la bóveda tiene datos: %v %v", has, err)
	}

	fake.FailList = true
	if has, err := HasRecords(t.Context(), cfg.Server, scope); err == nil || has {
		t.Fatal("un fallo del servidor no es una respuesta")
	}
	if _, err := HasRecords(t.Context(), nil, scope); err == nil {
		t.Fatal("sin servidor no se elige")
	}
}

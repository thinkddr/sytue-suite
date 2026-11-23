// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package store_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

func claveDePrueba(c byte) string { return strings.Repeat(string("0123456789abcdef"[c%16]), 64) }

// La decisión se toma sobre la activa de ESE momento y cada generación retira la
// anterior: el historial queda entero y solo hay una activa por bóveda.
func TestRegisterVaultKeyGeneracionesHistorialYUnaSolaActiva(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, nil, domain.User{Email: "ana@acme.test", EmailVerified: true, Name: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	registrar := func(clave string, motivo string) (store.VaultKeyRow, error) {
		return st.RegisterVaultKey(ctx, u.ID, "acme", "mail", func(actual *store.VaultKeyRow) (store.VaultKeyChange, error) {
			if actual != nil && actual.KeyID == clave {
				return store.VaultKeyChange{}, store.ErrVaultKeyUnchanged
			}
			prev := ""
			if actual != nil {
				prev = actual.KeyID
			}
			return store.VaultKeyChange{KeyID: clave, PreviousKeyID: prev, PublicKey: []byte(`{"k":"` + clave[:4] + `"}`), Reason: motivo}, nil
		})
	}
	if act, err := st.ActiveVaultKey(ctx, u.ID, "acme", "mail"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sin registrar: %+v, %v", act, err)
	}
	uno, err := registrar(claveDePrueba(1), "alta")
	if err != nil || uno.Generation != 1 || uno.Reason != "alta" || uno.PreviousKeyID != "" {
		t.Fatalf("alta: %+v, %v", uno, err)
	}
	// Repetir la activa no crea otra generación.
	if igual, err := registrar(claveDePrueba(1), "alta"); err != nil || igual.Generation != 1 {
		t.Fatalf("repetida: %+v, %v", igual, err)
	}
	dos, err := registrar(claveDePrueba(2), "vaciado")
	if err != nil || dos.Generation != 2 || dos.PreviousKeyID != claveDePrueba(1) {
		t.Fatalf("vaciado: %+v, %v", dos, err)
	}
	hist, err := st.VaultKeys(ctx, u.ID, "acme", "mail")
	if err != nil || len(hist) != 2 || hist[0].Generation != 2 || hist[0].RetiredAt != nil || hist[1].RetiredAt == nil {
		t.Fatalf("historial: %+v, %v", hist, err)
	}
	// Otra bóveda de la misma persona (otro producto, otra organización) es aparte.
	if otra, err := st.ActiveVaultKey(ctx, u.ID, "acme", "drive"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra bóveda: %+v, %v", otra, err)
	}
	// La base rechaza lo que la decisión no debería producir nunca.
	if _, err := st.RegisterVaultKey(ctx, u.ID, "acme", "notes", func(*store.VaultKeyRow) (store.VaultKeyChange, error) {
		return store.VaultKeyChange{KeyID: "no-hex", PublicKey: []byte(`{}`), Reason: "alta"}, nil
	}); err == nil {
		t.Fatal("key_id malformado aceptado")
	}
	if _, err := st.RegisterVaultKey(ctx, u.ID, "acme", "notes", func(*store.VaultKeyRow) (store.VaultKeyChange, error) {
		return store.VaultKeyChange{KeyID: claveDePrueba(3), PublicKey: []byte(`{}`), Reason: "vaciado"}, nil
	}); err == nil {
		t.Fatal("generación 1 con motivo de vaciado aceptada")
	}
	// Un error de la decisión no deja nada a medias.
	errNo := errors.New("no")
	if _, err := st.RegisterVaultKey(ctx, u.ID, "acme", "mail", func(*store.VaultKeyRow) (store.VaultKeyChange, error) {
		return store.VaultKeyChange{}, errNo
	}); !errors.Is(err, errNo) {
		t.Fatalf("decisión que falla: %v", err)
	}
	if act, _ := st.ActiveVaultKey(ctx, u.ID, "acme", "mail"); act == nil || act.Generation != 2 {
		t.Fatalf("tras una decisión fallida la activa cambió: %+v", act)
	}
}

// Dos dispositivos que registran a la vez: uno gana, el otro decide sobre la clave
// que acaba de quedar activa (no sobre la de antes). Nunca hay dos activas.
func TestRegisterVaultKeySerializaLaBoveda(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, nil, domain.User{Email: "eva@acme.test", EmailVerified: true, Name: "Eva"})
	if err != nil {
		t.Fatal(err)
	}
	const n = 8
	var wg sync.WaitGroup
	vistas := make([]int64, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = st.RegisterVaultKey(ctx, u.ID, "acme", "mail", func(actual *store.VaultKeyRow) (store.VaultKeyChange, error) {
				prev, motivo := "", "alta"
				if actual != nil {
					prev, motivo = actual.KeyID, "continuidad"
					vistas[i] = actual.Generation
				}
				return store.VaultKeyChange{KeyID: claveDePrueba(byte(i + 1)), PreviousKeyID: prev, PublicKey: []byte(`{}`), Reason: motivo}, nil
			})
		}()
	}
	wg.Wait()
	hist, err := st.VaultKeys(ctx, u.ID, "acme", "mail")
	if err != nil || len(hist) != n {
		t.Fatalf("historial: %d filas, %v; quiero %d", len(hist), err, n)
	}
	activas := 0
	for i, f := range hist {
		if f.RetiredAt == nil {
			activas++
		}
		if f.Generation != int64(n-i) {
			t.Fatalf("generaciones con hueco: %+v", hist)
		}
		if i+1 < len(hist) && f.PreviousKeyID != hist[i+1].KeyID {
			t.Fatalf("la generación %d no encadena con la %d", f.Generation, hist[i+1].Generation)
		}
	}
	if activas != 1 {
		t.Fatalf("%d activas; quiero 1", activas)
	}
}

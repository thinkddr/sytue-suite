// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del segundo factor: cifrado en reposo del secreto TOTP y
// ciclo de vida de los códigos de recuperación (lote, conteo, consumo de un solo
// uso, borrado y regeneración). Contra un PostgreSQL real (pgtest, build-tag
// `integration`).
package store_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// TestTOTPSecretEncryptedAtRest verifica que, con cifrado configurado (como en
// prod), el secreto TOTP se guarda cifrado (sobre "encv1:", sin el texto plano) y
// se descifra al leerlo.
func TestTOTPSecretEncryptedAtRest(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	key := make([]byte, keycrypto.KeyLen)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	cipher, err := keycrypto.New(key)
	if err != nil {
		t.Fatalf("keycrypto.New: %v", err)
	}
	st := store.NewWithCipher(h.Pool, cipher)
	ctx := context.Background()

	u := seedUser(t, ctx, st, "totp-enc@sytue.eu")
	secret := []byte("0123456789abcdefghij") // 20 bytes, como un secreto TOTP real
	if err := st.UpsertTOTPSecret(ctx, nil, u.ID, secret, true); err != nil {
		t.Fatalf("UpsertTOTPSecret: %v", err)
	}

	// La columna cruda NO contiene el secreto en claro y sí el sobre cifrado.
	var raw []byte
	if err := st.DB().QueryRow(ctx, `SELECT secret FROM totp_secrets WHERE user_id = $1`, u.ID).Scan(&raw); err != nil {
		t.Fatalf("SELECT secret: %v", err)
	}
	if !bytes.HasPrefix(raw, []byte("encv1:")) {
		t.Fatalf("el secreto no está cifrado en reposo: %q", raw)
	}
	if bytes.Contains(raw, secret) {
		t.Fatal("el secreto en claro aparece en la columna cifrada")
	}

	// Y round-trip: al leerlo se descifra al valor original.
	got, err := st.GetTOTPSecret(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetTOTPSecret: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatalf("secreto descifrado = %q, quiero %q", got, secret)
	}
}

// TestRecoveryCodesLifecycle cubre el lote (10), el conteo, el consumo de un solo
// uso (el segundo intento del mismo código no consume), el borrado y la
// regeneración (invalida los anteriores).
func TestRecoveryCodesLifecycle(t *testing.T) {
	st, ctx := newStore(t)
	u := seedUser(t, ctx, st, "rc@sytue.eu")

	_, hashes, err := domain.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("NewRecoveryCodes: %v", err)
	}
	if len(hashes) != 10 {
		t.Fatalf("se esperaban 10 códigos, hay %d", len(hashes))
	}
	if err := st.InsertRecoveryCodes(ctx, nil, u.ID, hashes); err != nil {
		t.Fatalf("InsertRecoveryCodes: %v", err)
	}
	if n, _ := st.CountRecoveryCodes(ctx, u.ID); n != 10 {
		t.Fatalf("conteo tras insertar = %d, quiero 10", n)
	}

	// Un solo uso: consumir devuelve true la 1ª vez y false la 2ª (ya no existe).
	used, err := st.ConsumeRecoveryCode(ctx, nil, u.ID, hashes[0])
	if err != nil || !used {
		t.Fatalf("primer consumo: used=%v err=%v, quiero true/nil", used, err)
	}
	used, err = st.ConsumeRecoveryCode(ctx, nil, u.ID, hashes[0])
	if err != nil || used {
		t.Fatalf("segundo consumo (reuso): used=%v err=%v, quiero false/nil", used, err)
	}
	if n, _ := st.CountRecoveryCodes(ctx, u.ID); n != 9 {
		t.Fatalf("conteo tras un consumo = %d, quiero 9", n)
	}

	// Regenerar reemplaza el lote: los códigos viejos dejan de consumirse.
	_, newHashes, err := domain.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("NewRecoveryCodes(2): %v", err)
	}
	if err := st.InsertRecoveryCodes(ctx, nil, u.ID, newHashes); err != nil {
		t.Fatalf("InsertRecoveryCodes(2): %v", err)
	}
	if used, _ := st.ConsumeRecoveryCode(ctx, nil, u.ID, hashes[1]); used {
		t.Fatal("un código del lote anterior no debería consumirse tras regenerar")
	}
	if used, _ := st.ConsumeRecoveryCode(ctx, nil, u.ID, newHashes[0]); !used {
		t.Fatal("un código del lote nuevo debería consumirse")
	}

	// DeleteRecoveryCodes deja el conteo a cero (al desactivar el TOTP).
	if err := st.DeleteRecoveryCodes(ctx, nil, u.ID); err != nil {
		t.Fatalf("DeleteRecoveryCodes: %v", err)
	}
	if n, _ := st.CountRecoveryCodes(ctx, u.ID); n != 0 {
		t.Fatalf("conteo tras borrar = %d, quiero 0", n)
	}
}

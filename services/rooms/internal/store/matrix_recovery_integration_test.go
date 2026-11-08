// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// La clave custodiada (ADR 0081) va cifrada, es de la persona en cualquier
// organización, se sustituye tras un restablecimiento y se va con la cuenta.
func TestMatrixRecoveryKeyIsSealedPerPersonAndGoesWithTheAccount(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	cipher, err := keycrypto.New(bytes.Repeat([]byte{9}, keycrypto.KeyLen))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(h.Pool, cipher)
	acme, globex := tenantCtx("acme"), tenantCtx("globex")
	const first = "EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP"
	const second = "EsSz ygLv VP1b xF1C v7kE eBQx MxDP buG5 w25T L3b6 hfyG Kkrd"

	if _, err := st.MatrixRecoveryKey(acme, "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sin clave: %v; quiero ErrNotFound", err)
	}
	if err := st.PutMatrixRecoveryKey(acme, "alice", first); err != nil {
		t.Fatal(err)
	}
	// La misma cuenta de Matrix desde otra organización ve la misma clave.
	if got, err := st.MatrixRecoveryKey(globex, "alice"); err != nil || got != first {
		t.Fatalf("desde otra organización = %q, %v", got, err)
	}
	if _, err := st.MatrixRecoveryKey(acme, "bob"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra persona: %v; quiero ErrNotFound", err)
	}
	var sealed string
	if err := h.Pool.InMaintenanceTx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT sealed FROM matrix_recovery_keys WHERE subject='alice'`).Scan(&sealed)
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, "encv1:") || strings.Contains(sealed, "EsT3") {
		t.Fatalf("la clave está en claro en la base: %q", sealed)
	}
	if err := st.PutMatrixRecoveryKey(acme, "alice", second); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.MatrixRecoveryKey(acme, "alice"); got != second {
		t.Fatalf("tras restablecer = %q; quiero la nueva", got)
	}
	if err := st.ApplyMatrixAccountAccess(acme, "alice", "deleted", 1, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := st.MatrixRecoveryKey(acme, "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tras borrar la cuenta: %v; quiero ErrNotFound", err)
	}
	// Sin KEK no se guarda nada en claro.
	if err := store.New(h.Pool, nil).PutMatrixRecoveryKey(acme, "bob", first); !errors.Is(err, store.ErrNoCipher) {
		t.Fatalf("sin KEK: %v; quiero ErrNoCipher", err)
	}
}

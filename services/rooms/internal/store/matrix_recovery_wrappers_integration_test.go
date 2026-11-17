// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// Los envoltorios (ADR 0082) van por cuenta de Matrix: Rooms y cada cuenta aislada
// tienen los suyos; guardar uno retira la copia con KEK de ADR 0081, una clave
// nueva retira los envoltorios de la vieja, y todo se va con la cuenta.
func TestLosEnvoltoriosVanPorCuentaYRetiranLaCustodiaConKEK(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	cipher, err := keycrypto.New(bytes.Repeat([]byte{9}, keycrypto.KeyLen))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(h.Pool, cipher)
	acme := tenantCtx("acme")
	rooms := store.MatrixRecoveryAccount{}
	drive := store.MatrixRecoveryAccount{Tenant: "acme", Product: "drive"}
	wrapper := func(keyID, tag string) []byte {
		return []byte(fmt.Sprintf(`{"v":1,"kind":"password","key_id":%q,"tag":%q}`, keyID, tag))
	}

	if err := st.PutMatrixRecoveryKey(acme, "alice", "EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP"); err != nil {
		t.Fatal(err)
	}
	if rows, legacy, err := st.MatrixRecoveryWrappers(acme, "alice", rooms); err != nil || len(rows) != 0 || !legacy {
		t.Fatalf("antes: %d envoltorios, legacy=%v, %v", len(rows), legacy, err)
	}
	// Un envoltorio de la cuenta aislada no toca la de Rooms ni su copia con KEK.
	if _, err := st.PutMatrixRecoveryWrapper(acme, "alice", drive, "password", "", "k1", wrapper("k1", "drive")); err != nil {
		t.Fatal(err)
	}
	if rows, legacy, _ := st.MatrixRecoveryWrappers(acme, "alice", rooms); len(rows) != 0 || !legacy {
		t.Fatalf("Rooms tras guardar el de Drive: %d envoltorios, legacy=%v", len(rows), legacy)
	}
	if rows, legacy, _ := st.MatrixRecoveryWrappers(acme, "alice", drive); len(rows) != 1 || legacy {
		t.Fatalf("Drive: %d envoltorios, legacy=%v", len(rows), legacy)
	}
	if rows, _, _ := st.MatrixRecoveryWrappers(tenantCtx("globex"), "alice", store.MatrixRecoveryAccount{Tenant: "globex", Product: "drive"}); len(rows) != 0 {
		t.Fatal("la cuenta aislada de otra organización ve los envoltorios de acme")
	}
	if rows, _, _ := st.MatrixRecoveryWrappers(acme, "bob", drive); len(rows) != 0 {
		t.Fatal("otra persona ve los envoltorios de alice")
	}
	// El de Rooms retira la copia con KEK en la misma transacción.
	if _, err := st.PutMatrixRecoveryWrapper(acme, "alice", rooms, "password", "", "r1", wrapper("r1", "a")); err != nil {
		t.Fatal(err)
	}
	if _, err := st.MatrixRecoveryKey(acme, "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("la copia con KEK sigue ahí: %v", err)
	}
	if _, err := st.PutMatrixRecoveryWrapper(acme, "alice", rooms, "passkey", "cred", "r1", wrapper("r1", "b")); err != nil {
		t.Fatal(err)
	}
	if rows, legacy, _ := st.MatrixRecoveryWrappers(acme, "alice", rooms); len(rows) != 2 || legacy {
		t.Fatalf("Rooms: %d envoltorios, legacy=%v; quiero 2 y false", len(rows), legacy)
	}
	// Restablecer: la clave nueva retira los envoltorios de la vieja.
	if _, err := st.PutMatrixRecoveryWrapper(acme, "alice", rooms, "password", "", "r2", wrapper("r2", "c")); err != nil {
		t.Fatal(err)
	}
	if rows, _, _ := st.MatrixRecoveryWrappers(acme, "alice", rooms); len(rows) != 1 || !bytes.Contains(rows[0].Wrapper, []byte(`"r2"`)) {
		t.Fatalf("tras rotar: %+v", rows)
	}
	// Tope por cuenta.
	for i := range store.MaxMatrixRecoveryWrappers {
		_, err = st.PutMatrixRecoveryWrapper(acme, "alice", rooms, "passkey", fmt.Sprint("c", i), "r2", wrapper("r2", "x"))
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("pasado el tope: %v; quiero ErrConflict", err)
	}
	// En claro no hay nada que no hubiera mandado el dispositivo; y solo en mantenimiento.
	var visible int
	if err := h.Pool.InTenantTx(acme, func(tx pgx.Tx) error {
		return tx.QueryRow(acme, `SELECT count(*) FROM matrix_recovery_wrappers`).Scan(&visible)
	}); err != nil || visible != 0 {
		t.Fatalf("fuera de mantenimiento se ven %d filas (%v)", visible, err)
	}
	if err := st.ApplyMatrixAccountAccess(acme, "alice", "deleted", 1, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	for _, account := range []store.MatrixRecoveryAccount{rooms, drive} {
		if rows, _, _ := st.MatrixRecoveryWrappers(acme, "alice", account); len(rows) != 0 {
			t.Fatalf("tras borrar la cuenta quedan envoltorios en %+v", account)
		}
	}
}

// El envoltorio `sytue-id` (ADR 0084) es uno por cuenta, como el de contraseña,
// y convive con los de la 0082 de la misma clave: la transición los deja ahí y el
// cliente usa el nuevo. Una passkey sin credencial o uno `sytue-id` con ella no
// caben en la tabla.
func TestElEnvoltorioSytueIDEsUnoPorCuentaYConviveConLosDeLa0082(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	cipher, err := keycrypto.New(bytes.Repeat([]byte{9}, keycrypto.KeyLen))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(h.Pool, cipher)
	acme := tenantCtx("acme")
	rooms := store.MatrixRecoveryAccount{}
	wrapper := func(kind, tag string) []byte {
		return []byte(fmt.Sprintf(`{"v":1,"kind":%q,"key_id":"r1","tag":%q}`, kind, tag))
	}
	for _, w := range []struct{ kind, cred string }{{"password", ""}, {"passkey", "cred"}, {"sytue-id", ""}} {
		if _, err := st.PutMatrixRecoveryWrapper(acme, "alice", rooms, w.kind, w.cred, "r1", wrapper(w.kind, "a")); err != nil {
			t.Fatalf("%s: %v", w.kind, err)
		}
	}
	if _, err := st.PutMatrixRecoveryWrapper(acme, "alice", rooms, "sytue-id", "", "r1", wrapper("sytue-id", "b")); err != nil {
		t.Fatal(err)
	}
	rows, _, err := st.MatrixRecoveryWrappers(acme, "alice", rooms)
	if err != nil || len(rows) != 3 {
		t.Fatalf("%d envoltorios (%v); quiero 3: el segundo sytue-id sustituye al primero", len(rows), err)
	}
	for _, bad := range []struct{ kind, cred string }{{"sytue-id", "cred"}, {"passkey", ""}} {
		if _, err := st.PutMatrixRecoveryWrapper(acme, "alice", rooms, bad.kind, bad.cred, "r1", wrapper(bad.kind, "x")); err == nil {
			t.Fatalf("%+v aceptado", bad)
		}
	}
	// Y la tabla lo impide aunque alguien se salte el almacén.
	err = h.Pool.InMaintenanceTx(acme, func(tx pgx.Tx) error {
		_, err := tx.Exec(acme, `INSERT INTO matrix_recovery_wrappers(subject,kind,credential_id,key_id,wrapper) VALUES('bob','sytue-id','cred','r1','{}')`)
		return err
	})
	if err == nil {
		t.Fatal("la tabla acepta un sytue-id con credencial")
	}
}

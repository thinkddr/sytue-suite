// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// Ninguna clave nueva entra en una custodia que el operador pueda leer (ADR 0082).
func TestLaCustodiaConKEKNoAdmiteClavesNuevas(t *testing.T) {
	err := (&Service{}).StoreMatrixRecoveryKey(context.Background(), "sub", "EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP")
	if !errors.Is(err, domain.ErrRecoveryEscrowRetired) {
		t.Fatalf("StoreMatrixRecoveryKey = %v; quiero ErrRecoveryEscrowRetired", err)
	}
}

// Rooms y la identidad heredada comparten fila; la aislada va por organización y
// producto. Una cuenta aislada nunca puede caer en la fila de Rooms.
func TestCadaCuentaDeMatrixTieneSusEnvoltorios(t *testing.T) {
	acme, err := tenancy.New("acme")
	if err != nil {
		t.Fatal(err)
	}
	ctx := tenancy.Into(context.Background(), acme)
	for _, tc := range []struct {
		in   MatrixRecoveryAccount
		want store.MatrixRecoveryAccount
		err  bool
	}{
		{in: MatrixRecoveryAccount{}},
		{in: MatrixRecoveryAccount{Product: "drive"}},
		{in: MatrixRecoveryAccount{Product: "drive", IdentityVersion: storage.LegacyIdentity}},
		{in: MatrixRecoveryAccount{Product: "drive", IdentityVersion: storage.IsolatedIdentity}, want: store.MatrixRecoveryAccount{Tenant: "acme", Product: "drive"}},
		{in: MatrixRecoveryAccount{IdentityVersion: storage.IsolatedIdentity}, err: true},
		{in: MatrixRecoveryAccount{Product: "nada", IdentityVersion: storage.IsolatedIdentity}, err: true},
		{in: MatrixRecoveryAccount{Product: "drive", IdentityVersion: 7}, err: true},
	} {
		got, err := tc.in.resolve(ctx)
		if (err != nil) != tc.err || got != tc.want {
			t.Errorf("resolve(%+v) = %+v, %v; quiero %+v (error %v)", tc.in, got, err, tc.want, tc.err)
		}
	}
}

// Lo que no tiene la forma (p. ej. un Argon2id barato) ni llega a mirar quién pide.
func TestUnEnvoltorioDebilSeRechazaAntesDeNada(t *testing.T) {
	good, err := recoverywrap.WrapPassword("x", "@sytue_a:x", "kid", "EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP")
	if err != nil {
		t.Fatal(err)
	}
	weak := good
	weak.KDF.Memory = 1024
	if _, err := (&Service{}).PutMatrixRecoveryWrapper(context.Background(), "sub", MatrixRecoveryAccount{}, weak); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("envoltorio débil: %v; quiero ErrInvalidInput", err)
	}
	// El bueno pasa la forma y se para en la identidad (aquí no hay Matrix).
	if _, err := (&Service{}).PutMatrixRecoveryWrapper(context.Background(), "sub", MatrixRecoveryAccount{}, good); !errors.Is(err, domain.ErrMatrixUnavailable) {
		t.Fatalf("envoltorio bueno: %v; quiero ErrMatrixUnavailable", err)
	}
}

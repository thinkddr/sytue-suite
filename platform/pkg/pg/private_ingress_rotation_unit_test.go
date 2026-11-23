// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
)

func TestRotacionDeLaClaveDeEntradaSoloConUnaBovedaPosterior(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	boveda := func(gen int64) matrixproof.Authority {
		return matrixproof.Authority{Version: 1, Scope: scope, UserID: vaultproof.UserID(scope), RootKey: strings.Repeat("A", 43), Generation: gen, Status: "active"}
	}
	matrix := boveda(6)
	matrix.UserID = storage.IsolatedMatrixUserPrefix(scope) + "matrix.test"
	str := func(s string) *string { return &s }
	num := func(n int64) *int64 { return &n }
	deMatrix := keyPlanAuthority{generation: 1, user: str(matrix.UserID), authority: num(5)}
	deBoveda := keyPlanAuthority{generation: 2, user: str(vaultproof.UserID(scope)), authority: num(6)}
	antigua := keyPlanAuthority{generation: 1}
	for nombre, c := range map[string]struct {
		actual    keyPlanAuthority
		siguiente int64
		autoridad matrixproof.Authority
		quiero    bool
	}{
		"traspaso de Matrix a la bóveda":     {deMatrix, 2, boveda(6), true},
		"plan de antes, sucedido por bóveda": {antigua, 2, boveda(1), true},
		"vaciado de la bóveda":               {deBoveda, 3, boveda(7), true},
		"la misma bóveda otra vez":           {deBoveda, 3, boveda(6), false},
		"una bóveda anterior":                {deBoveda, 3, boveda(5), false},
		"Matrix no rota":                     {deMatrix, 2, matrix, false},
		"plan de antes, Matrix":              {antigua, 2, matrix, false},
		"saltarse una generación":            {deMatrix, 3, boveda(6), false},
		"sobre un plan revocado":             {keyPlanAuthority{generation: 1, revoked: true}, 2, boveda(6), false},
		"autoridad revocada":                 {deMatrix, 2, func() matrixproof.Authority { a := boveda(6); a.Status = "revoked"; return a }(), false},
	} {
		if got := rotationAllowed(scope, c.actual, c.siguiente, c.autoridad); got != c.quiero {
			t.Errorf("%s: %v; quiero %v", nombre, got, c.quiero)
		}
	}
}

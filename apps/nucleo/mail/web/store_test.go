// SPDX-License-Identifier: Apache-2.0
package web

import (
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

func TestElAlmacenLoEligeElAnfitrionYTieneQueSerElDeLaAutoridad(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	root := strings.Repeat("A", 43)
	boveda := matrixproof.Authority{Version: 1, Scope: scope, UserID: vaultproof.UserID(scope), RootKey: root, Generation: 1, Status: "active"}
	matrix := boveda
	matrix.UserID = storage.IsolatedMatrixUserPrefix(scope) + "matrix.test"
	if err := storeMatchesAuthority(StoreVault, boveda); err != nil {
		t.Fatal("la bóveda no abre con su autoridad", err)
	}
	if err := storeMatchesAuthority(StoreMatrix, matrix); err != nil {
		t.Fatal("Matrix no abre con la suya", err)
	}
	if err := storeMatchesAuthority(StoreVault, matrix); !errors.Is(err, webport.ErrRejected) {
		t.Fatal("la bóveda abrió un ámbito de Matrix", err)
	}
	if err := storeMatchesAuthority(StoreMatrix, boveda); !errors.Is(err, webport.ErrRejected) {
		t.Fatal("Matrix abrió un ámbito de la bóveda", err)
	}
}

func TestLaBovedaNoAceptaPuertosDeMatrixNiUnAlmacenDesconocido(t *testing.T) {
	manager, err := NewManager(webport.NewSources(), webport.NewPolicies())
	if err != nil {
		t.Fatal(err)
	}
	port := &boundaryPort{id: strings.Repeat("a", 64)}
	reader := &boundaryPort{id: strings.Repeat("b", 64)}
	for name, open := range map[string]func() error{
		"bóveda con origen de Matrix": func() error {
			_, err := manager.OpenStore(t.Context(), Ports{Isolated: port, LegacyReader: reader}, StoreVault)
			return err
		},
		"bóveda con escritor de Matrix": func() error {
			_, err := manager.OpenStore(t.Context(), Ports{Isolated: port, MigrationWriter: reader}, StoreVault)
			return err
		},
		"almacén desconocido": func() error {
			_, err := manager.OpenStore(t.Context(), Ports{Isolated: port}, Store(7))
			return err
		},
	} {
		if err := open(); !errors.Is(err, webport.ErrRejected) || port.current != 0 || reader.current != 0 {
			t.Fatal(name, err, port.current, reader.current)
		}
	}
}

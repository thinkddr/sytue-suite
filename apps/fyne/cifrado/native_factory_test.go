// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestEveryMobileNativeProductUsesBoundNativeLoginInsteadOfLoopback(t *testing.T) {
	sentinel := errors.New("desktop-only loopback")
	for _, product := range storage.Products() {
		panel := &Panel{Login: func(context.Context, mx.Config) (*mx.Client, error) { return nil, sentinel }}
		if err := configureNativePanel(nil, product, "", true, panel); err != nil {
			t.Fatal(product, err)
		}
		if _, err := panel.Login(t.Context(), mx.Config{}); !errors.Is(err, ErrPrivateResources) {
			t.Fatal("mobile product kept loopback or unbound native login", product, err)
		}
		if panel.nativeRecovery != (product == "drive") {
			t.Fatal("unrelated product adopted Drive consumer recovery", product)
		}
	}
}

func TestOwnDesktopNativeProductKeepsMaintainedLoopbackAndRejectsForeignConsumer(t *testing.T) {
	sentinel := errors.New("desktop loopback")
	panel := &Panel{Login: func(context.Context, mx.Config) (*mx.Client, error) { return nil, sentinel }}
	if err := configureNativePanel(nil, "mail", "", false, panel); err != nil {
		t.Fatal(err)
	}
	if _, err := panel.Login(t.Context(), mx.Config{}); !errors.Is(err, sentinel) {
		t.Fatal("own desktop login changed unexpectedly", err)
	}
	if err := configureNativePanel(nil, "mail", "pdf", true, panel); !errors.Is(err, ErrPrivateResources) {
		t.Fatal("Mail accepted a PDF consumer", err)
	}
}

// Todos los productos de almacenamiento guardan su clave envuelta en Rooms desde
// un solo sitio (ADR 0082), con la sesión vinculada a la aplicación: sin ella no
// se pide nada al servicio.
func TestEveryStorageProductWrapsItsRecoveryKeyInRooms(t *testing.T) {
	for _, product := range storage.Products() {
		panel := &Panel{}
		if err := configureNativePanel(&sesion.Suite{}, product, "", false, panel); err != nil {
			t.Fatal(product, err)
		}
		if _, ok := panel.Custodio.(custodiaDeSesion); !ok {
			t.Fatalf("%s: sin custodia de la clave envuelta (%T)", product, panel.Custodio)
		}
		if _, err := panel.Custodio.MatrixRecoveryWrappers(t.Context(), api.MatrixRecoveryAccount{}); !errors.Is(err, ErrPrivateResources) {
			t.Fatalf("%s: la custodia habló con Rooms sin la sesión de la aplicación: %v", product, err)
		}
	}
	// Una custodia ya puesta (p. ej. en pruebas) no se sustituye.
	own := &servicioFalso{}
	panel := &Panel{Custodio: own}
	if err := configureNativePanel(&sesion.Suite{}, "notes", "", false, panel); err != nil || panel.Custodio != own {
		t.Fatal("se sustituyó la custodia del panel", err)
	}
}

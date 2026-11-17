// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/nube"
)

func TestSesionVinculadaNoReabreUnPresentadorCerradoAunqueVuelvaLaMismaCuenta(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	app := test.NewApp()
	defer app.Quit()
	ajustes.MarcarSesion()
	tokens := auth.Tokens{Access: "synthetic-a", Refresh: "synthetic-r", Expiry: time.Now().Add(time.Hour), Issuer: "https://id.example", Cliente: "native", Subject: "owner", Organizacion: "org"}
	s := &Suite{app: app, Sesion: auth.NewSession(tokens, nil, nil), generacionSesion: ajustes.Leer().Sesion}
	vida, cancel := context.WithCancel(t.Context())
	defer cancel()
	bound := s.SesionVinculada(vida)
	if _, err := bound.AccessToken(t.Context()); err != nil {
		t.Fatal(err)
	}
	other := tokens
	other.Subject = "other"
	s.Sesion.Sincronizar(other)
	if _, err := bound.AccessToken(t.Context()); !errors.Is(err, nube.ErrOtraCuenta) {
		t.Fatal("bound presenter adopted another identity", err)
	}
	cancel()
	s.Sesion.Sincronizar(tokens)
	if _, err := bound.AccessToken(context.Background()); !errors.Is(err, nube.ErrOtraCuenta) {
		t.Fatal("closed presenter readopted the same account", err)
	}
}

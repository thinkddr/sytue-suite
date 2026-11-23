// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"testing"

	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/office/guardardrive"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type cuentaOficina struct {
	scope storage.Scope
	err   error
}

func (c cuentaOficina) Context() context.Context           { return context.Background() }
func (c cuentaOficina) PrivateStorageScope() storage.Scope { return c.scope }
func (cuentaOficina) HTTP() api.Session                    { return nil }
func (c cuentaOficina) Check(context.Context) error        { return c.err }

type repoOficina struct{ guardardrive.Repositorio }

// El Drive de la bóveda de la ofimática solo se da a la persona que lo abrió; sin
// Drive de la bóveda en el dispositivo se dice, y nunca se pide desbloquear nada
// a una cuenta que ya no es la de la sesión.
func TestDriveOficinaBoveda(t *testing.T) {
	ana := cuentaOficina{scope: storage.Scope{Tenant: "acme", Subject: "ana", Product: "docs"}}
	luis := cuentaOficina{scope: storage.Scope{Tenant: "acme", Subject: "luis", Product: "docs"}}
	d := &DriveOficina{slot: container.NewStack(), fallo: ErrPrivateResources, dispatch: func(f func()) { f() }}
	repo := &repoOficina{}
	d.acceso.Store(&accesoOficina{repo: repo, cuenta: cuentaOficina{scope: storage.Scope{Tenant: "acme", Subject: "ana", Product: "drive"}}})

	if r, err := d.boveda(ana)(t.Context()); err != nil || r != repo {
		t.Fatalf("la de ana = %v, %v", r, err)
	}
	if _, err := d.boveda(luis)(t.Context()); !errors.Is(err, guardardrive.ErrBovedaNoDisponible) {
		t.Fatalf("la de luis = %v", err)
	}
	fuera := errors.New("la sesión ya es de otra persona")
	if _, err := d.boveda(cuentaOficina{scope: ana.scope, err: fuera})(t.Context()); !errors.Is(err, fuera) {
		t.Fatalf("con la cuenta caducada = %v", err)
	}
	if d.pedido || len(d.slot.Objects) != 0 {
		t.Fatal("se pidió desbloquear sin poder")
	}
}

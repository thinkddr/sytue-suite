// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package directo

import (
	"context"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/cast/db"
)

// Con un rol que se salta la RLS (como el del despliegue que entrega a los
// servicios el superusuario de PostgreSQL), la clave de emisión de una persona en
// una organización no se lee ni se rota desde otra: lo sostiene el SQL.
func TestSinRLSLaClaveDeEmisionEsDeSuOrganizacion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := h.PoolSinRLS(t)
	d := &datos{pool: pool}

	// La misma persona en dos organizaciones, con una clave en cada una.
	for _, c := range []Clave{
		{Tenant: "acme", OwnerSub: "ana", OwnerName: "Ana", Key: "clave-acme", IngressID: "in-acme"},
		{Tenant: "globex", OwnerSub: "ana", OwnerName: "Ana", Key: "clave-globex", IngressID: "in-globex"},
	} {
		if _, _, err := d.guardarClave(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	for tenant, quiere := range map[string]string{"acme": "clave-acme", "globex": "clave-globex"} {
		c, err := d.clave(ctx, tenant, "ana")
		if err != nil || c.Key != quiere || c.Tenant != tenant {
			t.Errorf("clave de ana en %s: %q de %q (%v), esperaba %q", tenant, c.Key, c.Tenant, err, quiere)
		}
	}
	if _, err := d.rotarClave(ctx, Clave{Tenant: "acme", OwnerSub: "ana", OwnerName: "Ana", Key: "nueva-acme", IngressID: "in-acme-2"}); err != nil {
		t.Fatal(err)
	}
	if c, err := d.clave(ctx, "globex", "ana"); err != nil || c.Key != "clave-globex" {
		t.Errorf("rotar la clave en acme cambió la de globex: %q (%v)", c.Key, err)
	}
	if c, err := d.clave(ctx, "acme", "ana"); err != nil || c.Key != "nueva-acme" {
		t.Errorf("la clave de acme no rotó: %q (%v)", c.Key, err)
	}
}

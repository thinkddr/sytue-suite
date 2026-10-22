// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// La apariencia por defecto de una organización —su color (0107) y su forma (0108)—
// con las migraciones aplicadas de verdad: una organización nace con los valores
// predeterminados, que no viajan en el token, y lo que fija el administrador se lee de vuelta.
package store_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

func TestLaAparienciaDeLaOrganizacionTrasLasMigraciones(t *testing.T) {
	st, ctx := newStore(t)
	org, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "apariencia", Name: "Apariencia"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	if org.Color != domain.OrgColorDefault || org.Forma != domain.OrgFormaDefault {
		t.Fatalf("nace con color %q y forma %q, quiero los predeterminados", org.Color, org.Forma)
	}
	if org.ColorClaim() != "" || org.FormaClaim() != "" {
		t.Errorf("los valores predeterminados no viajan en el token: %q, %q", org.ColorClaim(), org.FormaClaim())
	}

	for eje, valor := range map[string]string{"color": "cobalto", "forma": "carril"} {
		if err := st.GuardarApariencia(ctx, nil, org.ID, eje, valor); err != nil {
			t.Fatalf("GuardarApariencia(%s): %v", eje, err)
		}
	}
	leida, err := st.GetOrgByID(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetOrgByID: %v", err)
	}
	if leida.ColorClaim() != "cobalto" || leida.FormaClaim() != "carril" {
		t.Errorf("se lee color %q y forma %q, quiero cobalto y carril", leida.Color, leida.Forma)
	}

	// Un eje que no existe no escribe nada: el nombre de la columna no llega a la consulta.
	if err := st.GuardarApariencia(ctx, nil, org.ID, "slug", "x"); err == nil {
		t.Error("GuardarApariencia aceptó un eje desconocido")
	}
}

// La 0115 distingue el petróleo heredado de la elección de un administrador por
// su registro de auditoría. Reaplica el SQL REAL contra Postgres para cubrir tanto
// el defecto de las nuevas cuentas como las que ya estaban en producción.
func TestElNuevoColorPorDefectoConservaLasEleccionesAuditadas(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	dueno, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dueno.Close(ctx) }()
	aplicar := func(nombre string) {
		t.Helper()
		sql, err := db.Migrations.ReadFile("migrations/0115_color_de_cada_app_por_defecto." + nombre + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := dueno.Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	aplicar("down") // las tres cuentas nacen con el defecto anterior
	st := store.New(h.Pool)
	casos := []struct {
		slug, elegido, accion, quiero string
	}{
		{"heredado", "", "org.branding.forma", "app"},
		{"petroleo-explicito", "petroleo", "org.branding.color", "petroleo"},
		{"cobalto-explicito", "cobalto", "org.branding.color", "cobalto"},
	}
	for _, c := range casos {
		org, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: c.slug, Name: c.slug})
		if err != nil {
			t.Fatal(err)
		}
		if c.elegido != "" {
			if err := st.GuardarApariencia(ctx, nil, org.ID, "color", c.elegido); err != nil {
				t.Fatal(err)
			}
		}
		if err := st.Audit(ctx, nil, store.AuditEntry{OrgID: org.ID, Action: c.accion, Target: c.elegido}); err != nil {
			t.Fatal(err)
		}
	}
	aplicar("up")
	for _, c := range casos {
		org, err := st.GetOrgBySlug(ctx, c.slug)
		if err != nil || org.Color != c.quiero {
			t.Errorf("%s: color=%q, err=%v; quiero %q", c.slug, org.Color, err, c.quiero)
		}
	}
	nueva, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "nueva", Name: "Nueva"})
	if err != nil || nueva.Color != "app" {
		t.Errorf("tras migrar: color=%q, err=%v; quiero app", nueva.Color, err)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	domain "git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/vectores"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// TestVectoresDorados recorre en Postgres los mismos escenarios que el ERP de Sytue
// Office recorre en SQLite (apps/nucleo/local/erp/vectores_test.go): misma
// numeración, mismos asientos, mismos saldos (PLAN-EDICION-LOCAL §6.13, X-08).
func TestVectoresDorados(t *testing.T) {
	escenarios, err := vectores.Cargar()
	if err != nil || len(escenarios) == 0 {
		t.Fatalf("vectores: %v %d", err, len(escenarios))
	}
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	c := cat(t)
	for _, esc := range escenarios {
		t.Run(esc.Nombre, func(t *testing.T) {
			ctx := tenantCtx("vectores-" + esc.Nombre)
			ids := map[string]string{}
			modelos := map[string]string{}
			for i, p := range esc.Pasos {
				switch {
				case p.Crear != "":
					cuerpo, err := json.Marshal(vectores.Resolver(p.Data, ids))
					if err != nil {
						t.Fatal(err)
					}
					r := crear(ctx, t, st, modelo(t, c, p.Crear), string(cuerpo))
					if p.Ref != "" && r.Ref != p.Ref {
						t.Fatalf("paso %d: referencia %q, se esperaba %q", i, r.Ref, p.Ref)
					}
					if p.Como != "" {
						ids[p.Como], modelos[p.Como] = r.ID, p.Crear
					}
				case p.Asentar != "":
					m, id := modelo(t, c, modelos[p.Asentar]), ids[p.Asentar]
					a, err := st.Asentar(ctx, m, id, "ada")
					switch p.Error {
					case "ya_asentado":
						if !errors.Is(err, store.ErrYaAsentado) {
							t.Fatalf("paso %d: %v, se esperaba ya asentado", i, err)
						}
						continue
					case "datos":
						if !errors.Is(err, domain.ErrDatos) {
							t.Fatalf("paso %d: %v, se esperaba un error de datos", i, err)
						}
						continue
					}
					if err != nil {
						t.Fatalf("paso %d: asentar %s: %v", i, p.Asentar, err)
					}
					if a.Ref != p.Ref || a.Diario != p.Diario || len(a.Apuntes) != len(p.Apuntes) {
						t.Fatalf("paso %d: asiento %+v", i, a)
					}
					for j, q := range p.Apuntes {
						ap := a.Apuntes[j]
						if ap.Cuenta != q.Cuenta || ap.Debe != q.Debe || ap.Haber != q.Haber || ap.Tercero != q.TerceroID(ids) {
							t.Fatalf("paso %d: apunte %d = %+v, se esperaba %+v", i, j, ap, q)
						}
					}
					if r, err := st.Obtener(ctx, m.Clave, id); err != nil || r.Estado != p.Estado {
						t.Fatalf("paso %d: estado: %v %q", i, err, r.Estado)
					}
				}
			}
			saldos, err := st.Balance(ctx, time.Time{})
			if err != nil || len(saldos) != len(esc.Balance) {
				t.Fatalf("balance: %v %+v", err, saldos)
			}
			for i, q := range esc.Balance {
				s := saldos[i]
				if s.Cuenta != q.Cuenta || s.Debe != q.Debe || s.Haber != q.Haber || s.Saldo != q.Saldo {
					t.Fatalf("balance %d = %+v, se esperaba %+v", i, s, q)
				}
			}
		})
	}
}
